use scry::learning::{self, Attempt, QuestionEvidence, State};

fn observation(at_ms: i64, rating: u8, assisted: bool, outcome: &str) -> Attempt {
    Attempt {
        at_ms,
        rating,
        assisted,
        outcome: outcome.into(),
    }
}

#[test]
fn observations_distinguish_exposure_help_and_cold_recall_without_mutations() {
    let now = 1_790_000_000_000;
    let mut question = QuestionEvidence {
        card: learning::new_card(now),
        level: "recall".into(),
        attempts: vec![],
    };
    let state = learning::compute_concept_state(&[question.clone()], now);
    assert_eq!(state.status, "new");
    assert_eq!(state.recall, -1.0);
    assert_eq!(state.brightness, 0);
    assert!(state.next_due_ms.is_none());
    question.attempts = vec![
        observation(now + 5, 3, false, "correct"),
        observation(now + 1, 0, true, "warm_correct"),
        observation(now + 4, 1, false, "wrong"),
        observation(now + 2, 1, true, "revealed"),
        observation(now + 3, 0, false, "known_already"),
    ];
    let before = question.card.clone();
    let state = learning::compute_concept_state(&[question.clone()], now + 10);
    assert_eq!(state.status, "learning");
    assert_eq!(state.recall, -1.0);
    assert_eq!(state.unaided, 1);
    assert_eq!(state.helped, 2);
    assert_eq!(state.missed, 1);
    assert_eq!(state.tally, ["h", "h", "m", "u"]);
    assert_eq!(state.last_at_ms, Some(now + 5));
    assert_eq!(question.card, before);
}

#[test]
fn solid_and_fading_are_recall_estimates_and_latest_help_prevents_solid() {
    let now = 1_790_000_000_000;
    let mut card = learning::new_card(now);
    card.state = State::Review;
    card.last_review_ms = Some(now);
    card.stability = 30.0;
    card.difficulty = 5.0;
    card.due_ms = now + 30 * 86_400_000;
    let mut question = QuestionEvidence {
        card,
        level: "explain".into(),
        attempts: vec![observation(now, 3, false, "correct")],
    };
    let solid = learning::compute_concept_state(&[question.clone()], now);
    assert_eq!(solid.status, "solid");
    assert_eq!(solid.recall, 1.0);
    assert_eq!(solid.brightness, 5);
    assert_eq!(solid.next_due_ms, Some(question.card.due_ms));
    question
        .attempts
        .push(observation(now + 1, 0, true, "warm_correct"));
    let helped = learning::compute_concept_state(&[question.clone()], now + 1);
    assert_eq!(helped.status, "learning");
    assert_eq!(helped.brightness, 4);
    let fading = learning::compute_concept_state(&[question], now + 120 * 86_400_000);
    assert_eq!(fading.status, "fading");
    assert!(fading.recall < 0.80);
    assert!(fading.brightness <= 2);
}

#[test]
fn levels_weight_estimates_and_tally_caps_observations_without_losing_counts() {
    let now = 1_790_000_000_000;
    let mut base = learning::new_card(now);
    base.state = State::Review;
    base.last_review_ms = Some(now - 20 * 86_400_000);
    base.stability = 2.0;
    base.difficulty = 5.0;
    let first = QuestionEvidence {
        card: base.clone(),
        level: "recognize".into(),
        attempts: (0..20)
            .map(|i| observation(now + i, 3, false, "correct"))
            .collect(),
    };
    let mut second = first.clone();
    second.card.stability = 50.0;
    second.level = "explain".into();
    second.attempts.clear();
    let expected = (learning::retrievability(&first.card, now)
        + 3.0 * learning::retrievability(&second.card, now))
        / 4.0;
    let state = learning::compute_concept_state(&[first, second], now);
    assert!((state.recall - expected).abs() < 1e-12);
    assert_eq!(state.tally.len(), 12);
    assert_eq!(state.unaided, 20);
    assert_eq!(learning::new_concept_budget("light"), 3);
    assert_eq!(learning::new_concept_budget("steady"), 6);
    assert_eq!(learning::new_concept_budget("intense"), 12);
    assert_eq!(learning::new_concept_budget("unknown"), 6);
    assert_eq!(learning::level_rank("recognize"), 0);
    assert_eq!(learning::level_rank("recall"), 1);
    assert_eq!(learning::level_rank("explain"), 2);
    assert_eq!(learning::level_rank("apply"), 3);
}
