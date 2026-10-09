use scry::{engine, learning, model::*, persistence};
use std::collections::BTreeMap;

const NOW: i64 = 1_791_410_400_000;

fn fixture() -> App {
    let mut app = App::new("schedule-history-owner-csrf-32-characters".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    for question in app
        .questions
        .values_mut()
        .filter(|q| q.content().kind == "choice")
    {
        question.card.due_ms = NOW + DAY_MS;
    }
    app.occurrence = None;
    engine::ensure_occurrence(&mut app, NOW, "schedule-recall-fixture").unwrap();
    app
}

fn fields(app: &App, operation: &str, extra: &[(&str, &str)]) -> BTreeMap<String, String> {
    let mut fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), operation.into()),
    ]);
    if let Some(o) = &app.occurrence {
        fields.insert("occurrence_id".into(), o.id.clone());
    }
    for (key, value) in extra {
        fields.insert((*key).into(), (*value).into());
    }
    fields
}

fn mutate(app: &mut App, path: &str, operation: &str, now: i64, extra: &[(&str, &str)]) {
    let fields = fields(app, operation, extra);
    engine::mutate(app, path, &fields, now, operation).unwrap();
}

fn reviewed() -> App {
    let mut app = fixture();
    let answer = app.occurrence.as_ref().unwrap().presentation.answer.clone();
    mutate(
        &mut app,
        "/review/answer",
        "history-answer-0001",
        NOW,
        &[("answer", &answer)],
    );
    app
}

fn reset_fields(app: &App, operation: &str, question_id: &str) -> BTreeMap<String, String> {
    let question = &app.questions[question_id];
    let content_version = question.content().version.to_string();
    let schedule_version = question.schedule_version.to_string();
    fields(
        app,
        operation,
        &[
            (
                "text",
                "This wording was confusing; start this question again.",
            ),
            ("reset", "true"),
            ("content_version", &content_version),
            ("schedule_version", &schedule_version),
        ],
    )
}

#[test]
fn reset_history_is_immutable_atomic_exactly_replayed_and_self_contained() {
    let mut app = reviewed();
    let question_id = app.events[0].question_id.clone();
    mutate(
        &mut app,
        "/review/override",
        "history-override-001",
        NOW + 1,
        &[("correct", "false")],
    );
    let original = app.events[0].clone();
    let correction = app.overrides[0].clone();
    let before_card = app.questions[&question_id].card.clone();
    let fields = reset_fields(&app, "history-reset-00001", &question_id);
    let path = format!("/questions/{question_id}/dispute");
    let receipt = engine::mutate(&mut app, &path, &fields, NOW + 2, "history-reset-00001").unwrap();
    assert_eq!(app.resets.len(), 1);
    let reset = &app.resets[0];
    assert_eq!(reset.before_card, before_card);
    assert_eq!(reset.after_card, learning::new_card(NOW + 2));
    assert_eq!(reset.schedule_version_before, 2);
    assert_eq!(reset.schedule_version_after, 3);
    assert_eq!(reset.content_version, 1);
    assert!(
        app.feedback
            .iter()
            .any(|f| f.id == reset.feedback_id && f.kind == "dispute")
    );
    assert_eq!(app.events[0], original);
    assert_eq!(app.overrides[0], correction);
    let saved = app.clone();
    assert_eq!(
        engine::mutate(&mut app, &path, &fields, NOW + 5, "new-random-seed").unwrap(),
        receipt
    );
    assert_eq!(app, saved);
    engine::validate_app(&app).unwrap();
    let rows = persistence::rows(&app).unwrap();
    assert!(rows.keys().any(|key| key.starts_with("resets/")));
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), app);
    let bytes = persistence::Archive::new(app.clone(), vec![], NOW + 5)
        .encode()
        .unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        app
    );
    // Continue with a real later occurrence; a reset cannot rewrite the old
    // grade, and the next review follows the reset's new-card state.
    for q in app.questions.values_mut().filter(|q| q.id != question_id) {
        q.archived = true;
    }
    mutate(
        &mut app,
        "/review/next",
        "history-after-reset1",
        NOW + DAY_MS + 3,
        &[],
    );
    assert_eq!(app.occurrence.as_ref().unwrap().question_id, question_id);
    let answer = app.occurrence.as_ref().unwrap().presentation.answer.clone();
    mutate(
        &mut app,
        "/review/answer",
        "history-reset-answer",
        NOW + DAY_MS + 4,
        &[("answer", &answer)],
    );
    assert_eq!(app.events[1].before_card, learning::new_card(NOW + 2));
    assert_eq!(app.events[1].schedule_version, 4);
    engine::validate_app(&app).unwrap();
}

#[test]
fn stale_reset_cannot_erase_a_newer_review_or_content_revision() {
    let mut app = reviewed();
    let question_id = app.events[0].question_id.clone();
    let stale = reset_fields(&app, "history-stale-reset1", &question_id);
    mutate(
        &mut app,
        "/review/override",
        "history-new-override",
        NOW + 1,
        &[("correct", "false")],
    );
    let before = app.clone();
    assert_eq!(
        engine::mutate(
            &mut app,
            &format!("/questions/{question_id}/dispute"),
            &stale,
            NOW + 2,
            "stale-reset"
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, before);
    let stale = reset_fields(&app, "history-stale-reset2", &question_id);
    let question = app.questions.get_mut(&question_id).unwrap();
    let mut edited = question.content().clone();
    edited.version += 1;
    edited.prompt = "Which HTTP directive sets the freshness lifetime in seconds?".into();
    question.versions.push(edited);
    let before = app.clone();
    assert_eq!(
        engine::mutate(
            &mut app,
            &format!("/questions/{question_id}/dispute"),
            &stale,
            NOW + 3,
            "stale-content-reset"
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, before);
}

#[test]
fn missing_or_tampered_schedule_history_cannot_become_restore_authority() {
    let mut app = reviewed();
    let question_id = app.events[0].question_id.clone();
    let fields = reset_fields(&app, "history-reset-00002", &question_id);
    engine::mutate(
        &mut app,
        &format!("/questions/{question_id}/dispute"),
        &fields,
        NOW + 2,
        "history-reset-00002",
    )
    .unwrap();
    engine::validate_app(&app).unwrap();
    let mut variants = Vec::new();
    let mut corrupt = app.clone();
    corrupt.resets.clear();
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.resets[0].before_card.reps += 1;
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.resets[0].after_card.due_ms += 1;
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.resets[0].feedback_id = "missing-dispute".into();
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.resets[0].content_version = 99;
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.resets[0].source_revision = 99;
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.questions.get_mut(&question_id).unwrap().card.due_ms += 1;
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt
        .questions
        .get_mut(&question_id)
        .unwrap()
        .schedule_version += 1;
    variants.push(corrupt);
    for corrupt in variants {
        assert!(engine::validate_app(&corrupt).is_err());
        assert!(
            persistence::Archive::new(corrupt, vec![], NOW + 5)
                .encode()
                .is_err()
        );
    }
}

#[test]
fn due_reviews_remain_available_while_paused_and_another_goal_is_focused() {
    let mut app = reviewed();
    let question_id = app.events[0].question_id.clone();
    let goal_id = app.questions[&question_id].goal_id.clone();
    for q in app.questions.values_mut().filter(|q| q.id != question_id) {
        q.archived = true;
    }
    mutate(
        &mut app,
        &format!("/goals/{goal_id}/pause"),
        "history-pause-goal01",
        NOW + 1,
        &[],
    );
    mutate(
        &mut app,
        "/create",
        "history-create-focus",
        NOW + 2,
        &[("intent", "Learn TLS")],
    );
    let other_goal = app
        .goals
        .values()
        .find(|goal| goal.id != goal_id)
        .unwrap()
        .id
        .clone();
    mutate(
        &mut app,
        &format!("/goals/{other_goal}/focus"),
        "history-focus-other1",
        NOW + 3,
        &[],
    );
    let due = app.questions[&question_id].card.due_ms;
    mutate(&mut app, "/review/next", "history-due-paused01", due, &[]);
    assert_eq!(app.occurrence.as_ref().unwrap().question_id, question_id);
    assert!(!app.questions[&question_id].card.is_new());
    assert!(app.goals[&goal_id].paused && app.goals[&other_goal].focused);
    engine::validate_app(&app).unwrap();
}

#[test]
fn assistance_is_pinned_to_occurrence_even_when_24_hours_elapse_before_answer() {
    let mut app = fixture();
    let concept_id = app.occurrence.as_ref().unwrap().concept_id.clone();
    app.concepts.get_mut(&concept_id).unwrap().exposure_ms = Some(NOW);
    app.occurrence = None;
    engine::ensure_occurrence(&mut app, NOW + DAY_MS - 1, "history-warm-pin").unwrap();
    assert!(app.occurrence.as_ref().unwrap().assisted);
    let before = app.occurrence.as_ref().unwrap().before_card.clone();
    let answer = app.occurrence.as_ref().unwrap().presentation.answer.clone();
    mutate(
        &mut app,
        "/review/answer",
        "history-warm-expired",
        NOW + DAY_MS + 60_000,
        &[("answer", &answer)],
    );
    assert_eq!(app.events[0].result.outcome, "warm_correct");
    assert_eq!(app.events[0].result.rating, 0);
    assert_eq!(app.events[0].after_card, before);
    engine::validate_app(&app).unwrap();
}

#[test]
fn self_check_preserves_original_attempt_and_records_exposure_for_later_occurrences() {
    let mut app = fixture();
    let question_id = app.occurrence.as_ref().unwrap().question_id.clone();
    let concept_id = app.occurrence.as_ref().unwrap().concept_id.clone();
    for q in app.questions.values_mut().filter(|q| q.id != question_id) {
        q.archived = true;
    }
    mutate(
        &mut app,
        "/review/answer",
        "history-learner-answer",
        NOW,
        &[("answer", "an answer in my own words")],
    );
    let config = engine::WorkConfig {
        generation: false,
        jev: false,
        critic: false,
        model: "synthetic-generator".into(),
        jev_model: "synthetic-jev".into(),
    };
    assert!(
        engine::claim_work(&mut app, NOW + 1, "history-no-assessor", &config)
            .unwrap()
            .is_none()
    );
    assert_eq!(app.concepts[&concept_id].exposure_ms, Some(NOW + 1));
    mutate(
        &mut app,
        "/review/self",
        "history-self-check01",
        NOW + 2,
        &[("correct", "true")],
    );
    assert!(!app.events[0].assisted);
    assert_eq!(app.events[0].result.rating, 3);
    let due = app.questions[&question_id].card.due_ms;
    mutate(&mut app, "/review/next", "history-after-self01", due, &[]);
    assert!(app.occurrence.as_ref().unwrap().assisted);
    let before = app.occurrence.as_ref().unwrap().before_card.clone();
    let answer = app.occurrence.as_ref().unwrap().presentation.answer.clone();
    mutate(
        &mut app,
        "/review/answer",
        "history-warm-afterself",
        due + 1,
        &[("answer", &answer)],
    );
    assert_eq!(app.events[1].result.rating, 0);
    assert_eq!(app.events[1].after_card, before);
    engine::validate_app(&app).unwrap();
}

#[test]
fn revealing_records_future_help_instead_of_an_immediate_cold_success() {
    let mut app = fixture();
    let question_id = app.occurrence.as_ref().unwrap().question_id.clone();
    let concept_id = app.occurrence.as_ref().unwrap().concept_id.clone();
    for q in app.questions.values_mut().filter(|q| q.id != question_id) {
        q.archived = true;
    }
    mutate(&mut app, "/review/reveal", "history-reveal-00001", NOW, &[]);
    assert_eq!(app.events[0].result.rating, 1);
    assert_eq!(app.concepts[&concept_id].exposure_ms, Some(NOW));
    let due = app.questions[&question_id].card.due_ms;
    mutate(&mut app, "/review/next", "history-after-reveal", due, &[]);
    assert!(app.occurrence.as_ref().unwrap().assisted);
    let before = app.occurrence.as_ref().unwrap().before_card.clone();
    let answer = app.occurrence.as_ref().unwrap().presentation.answer.clone();
    mutate(
        &mut app,
        "/review/answer",
        "history-revealed-help",
        due + 1,
        &[("answer", &answer)],
    );
    assert_eq!(app.events[1].result.outcome, "warm_correct");
    assert_eq!(app.events[1].result.rating, 0);
    assert_eq!(app.events[1].after_card, before);
    engine::validate_app(&app).unwrap();
}

#[test]
fn pausing_excludes_unseen_questions_even_after_the_concept_intro() {
    let mut app = fixture();
    let goal_id = app.goals.values().next().unwrap().id.clone();
    assert!(app.concepts.values().all(|c| c.introduced_ms.is_some()));
    assert!(app.questions.values().all(|q| q.card.is_new()));
    mutate(
        &mut app,
        &format!("/goals/{goal_id}/pause"),
        "history-pause-unseen",
        NOW,
        &[],
    );
    app.occurrence = None;
    engine::ensure_occurrence(&mut app, NOW + 1, "history-no-new-when-paused").unwrap();
    assert!(app.occurrence.is_none());
    engine::validate_app(&app).unwrap();
}
