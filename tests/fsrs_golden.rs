use scry::learning::{self, Card, State};
use serde::Deserialize;

#[derive(Deserialize)]
struct Golden {
    source: String,
    scheduler: String,
    weights: Vec<f64>,
    cases: Vec<Case>,
}

#[derive(Deserialize)]
struct Case {
    name: String,
    transitions: Vec<Transition>,
}

#[derive(Deserialize)]
struct Transition {
    before: Card,
    rating: u8,
    now_ms: i64,
    after: Card,
    probe_ms: i64,
    retrievability: f64,
}

fn float_agrees(actual: f64, expected: f64) -> bool {
    actual.is_finite()
        && expected.is_finite()
        && (actual - expected).abs() <= 1e-12 * expected.abs().max(1.0)
}

/// These full-card expected values came from the actual pinned Go dependency,
/// not a second copy of the Rust implementation or merely expected intervals.
#[test]
fn all_persisted_fields_match_pinned_go_scheduler() {
    let golden: Golden = serde_json::from_str(include_str!("fixtures/fsrs-golden.json")).unwrap();
    assert!(golden.source.contains("go-fsrs/v4@v4.0.0"));
    assert_eq!(golden.scheduler, learning::SCHEDULER);
    assert_eq!(golden.weights.len(), 21);
    assert_eq!(golden.weights[20], 0.1542);
    let mut steps = 0;
    let mut saw_learning = false;
    let mut saw_relearning = false;
    let mut saw_review = false;
    let mut saw_ceiling = false;
    for case in golden.cases {
        for (index, transition) in case.transitions.into_iter().enumerate() {
            let actual =
                learning::schedule(&transition.before, transition.rating, transition.now_ms)
                    .unwrap_or_else(|e| panic!("{} step {index}: {e}", case.name));
            let expected = transition.after;
            let context = format!("{} step {index}", case.name);
            assert_eq!(actual.due_ms, expected.due_ms, "{context}: due");
            assert_eq!(actual.state, expected.state, "{context}: state");
            assert_eq!(actual.reps, expected.reps, "{context}: reps");
            assert_eq!(actual.lapses, expected.lapses, "{context}: lapses");
            assert_eq!(
                actual.scheduled_days, expected.scheduled_days,
                "{context}: interval"
            );
            assert_eq!(
                actual.last_review_ms, expected.last_review_ms,
                "{context}: last review"
            );
            assert_eq!(
                actual.remaining_steps, expected.remaining_steps,
                "{context}: remaining steps"
            );
            assert!(
                float_agrees(actual.stability, expected.stability),
                "{context}: stability {} != {}",
                actual.stability,
                expected.stability
            );
            assert!(
                float_agrees(actual.difficulty, expected.difficulty),
                "{context}: difficulty {} != {}",
                actual.difficulty,
                expected.difficulty
            );
            assert!(
                float_agrees(
                    learning::retrievability(&actual, transition.probe_ms),
                    transition.retrievability
                ),
                "{context}: retrievability"
            );
            // Persist/restore the complete card without losing counters or
            // learning/relearning state; storage uses this same representation.
            let restored: Card =
                serde_json::from_str(&serde_json::to_string(&actual).unwrap()).unwrap();
            assert_eq!(restored, actual, "{context}: JSON round trip");
            saw_learning |= actual.state == State::Learning;
            saw_relearning |= actual.state == State::Relearning;
            saw_review |= actual.state == State::Review;
            saw_ceiling |= actual.scheduled_days == 36_501;
            steps += 1;
        }
    }
    assert_eq!(steps, 260);
    assert!(saw_learning && saw_relearning && saw_review && saw_ceiling);
}

#[test]
fn published_trajectory_and_algorithm_identity_remain_exact() {
    let mut now_ms = 1_669_725_000_000;
    let mut card = learning::new_card(now_ms);
    let ratings = [3, 3, 3, 3, 3, 3, 1, 1, 3, 3, 3, 3, 3];
    let days = [0, 2, 11, 46, 163, 498, 0, 0, 2, 4, 7, 12, 21];
    for (rating, expected) in ratings.into_iter().zip(days) {
        card = learning::schedule(&card, rating, now_ms).unwrap();
        assert_eq!(card.scheduled_days, expected);
        assert!(card.due_ms > now_ms);
        now_ms = card.due_ms;
    }
    assert_eq!(learning::event_algorithm("exact-v1"), learning::ALGORITHM);
    assert_eq!(
        learning::event_algorithm(learning::SEMANTIC_POLICY),
        format!("{};grading=semantic-v1", learning::SCHEDULER)
    );
}

#[test]
fn invalid_and_unearned_transitions_are_rejected() {
    let now = 1_790_000_000_000;
    let new = learning::new_card(now);
    for rating in [0, 2, 4, 255] {
        assert!(learning::schedule(&new, rating, now).is_err());
    }
    let learned = learning::schedule(&new, 3, now).unwrap();
    assert!(learning::schedule(&learned, 3, now - 1).is_err());
    for stability in [f64::NAN, f64::INFINITY, -1.0, 0.0, 0.0009] {
        let mut invalid = learned.clone();
        invalid.stability = stability;
        assert!(learning::schedule(&invalid, 3, now).is_err());
    }
    for difficulty in [f64::NAN, f64::INFINITY, 0.99] {
        let mut invalid = learned.clone();
        invalid.difficulty = difficulty;
        assert!(learning::schedule(&invalid, 3, now).is_err());
    }
    assert_eq!(learning::retrievability(&new, now), -1.0);
    assert!(learning::schedule(&learning::new_card(i64::MAX), 3, i64::MAX).is_err());
    assert!(serde_json::from_str::<State>("\"unsupported\"").is_err());
}

#[test]
fn utc_calendar_scheduling_and_elapsed_day_estimate_keep_distinct_conventions() {
    let before_midnight = chrono::DateTime::parse_from_rfc3339("2024-02-28T23:55:00Z")
        .unwrap()
        .timestamp_millis();
    let card =
        learning::schedule(&learning::new_card(before_midnight), 3, before_midnight).unwrap();
    let after_midnight = before_midnight + 10 * 60_000;
    assert_eq!(learning::retrievability(&card, after_midnight), 1.0);
    let midnight_next = learning::schedule(&card, 3, after_midnight).unwrap();
    let mut same_day_card = card.clone();
    same_day_card.last_review_ms = Some(after_midnight - 60_000);
    let same_day_next = learning::schedule(&same_day_card, 3, after_midnight).unwrap();
    assert!(midnight_next.stability > same_day_next.stability);
}
