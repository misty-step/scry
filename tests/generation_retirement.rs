//! Current runtime policy is deliberately separate from historical generation tests.
use scry::{
    engine::{self, WorkConfig},
    model::*,
    web,
};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
fn config() -> WorkConfig {
    WorkConfig {
        generation: true,
        jev: true,
        critic: true,
        model: "synthetic-generator".into(),
        jev_model: "synthetic-grader".into(),
    }
}
fn fields(app: &App, extra: &[(&str, &str)]) -> BTreeMap<String, String> {
    let mut f = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "historical-operation-001".into()),
    ]);
    for (k, v) in extra {
        f.insert((*k).into(), (*v).into());
    }
    f
}
#[test]
fn current_generation_admission_refuses_every_old_entry_not_grading_or_external_authoring() {
    for path in [
        "/create",
        "/add",
        "/goals/old/retry",
        "/goals/old/refine",
        "/questions/old/fix",
    ] {
        let e = engine::admit_current_mutation(path).unwrap_err();
        assert_eq!(e.status, 410);
        assert!(e.message.contains("retired"));
    }
    for path in ["/review/self", "/review/override", "/practice/correct"] {
        let error = engine::admit_current_mutation(path).unwrap_err();
        assert_eq!(error.status, 410);
        assert!(error.message.contains("Self-grading is retired"));
    }
    for path in [
        "/practice/answer",
        "/practice/reveal",
        "/library/mapping/add",
        "/library/version/add",
        "/review/answer",
        "/review/retry",
        "/settings/resume",
        "/questions/old/edit",
    ] {
        engine::admit_current_mutation(path).unwrap();
    }
}
#[test]
fn current_worker_never_claims_or_recovers_historical_generation_jobs() {
    let mut app = App::new("retirement-synthetic-csrf-at-least-24".into());
    let f = fields(&app, &[("intent", "Historical saved request")]);
    engine::mutate(&mut app, "/create", &f, NOW, "old-create").unwrap();
    let unchanged = |app: &App| {
        engine::validate_app(app).unwrap();
        let mut current = app.clone();
        assert!(
            engine::claim_grading_work(&mut current, NOW + 100_000, "unused-lease", &config())
                .unwrap()
                .is_none()
        );
        assert!(!engine::recover_grading(&mut current, NOW + 100_000));
        assert_eq!(
            &current, app,
            "Historical job, receipt, content and spend bytes must be unchanged"
        );
    };
    unchanged(&app); // queued
    let generated = engine::claim_work(&mut app, NOW, "historical-generation-lease", &config())
        .unwrap()
        .unwrap();
    unchanged(&app); // sent, including an unresolved reservation
    let mut fixture = App::new("retirement-authored-batch-csrf-24".into());
    engine::seed_fixture(&mut fixture, NOW).unwrap();
    let batch = fixture
        .jobs
        .values()
        .next()
        .unwrap()
        .candidate_json
        .as_ref()
        .unwrap();
    let raw = serde_json::json!({"choices":[{"message":{"content":batch}}]}).to_string();
    engine::finish_work(
        &mut app,
        &generated,
        raw,
        "synthetic-generator".into(),
        Some(10_000),
        NOW + 1,
    )
    .unwrap();
    unchanged(&app); // candidates cannot be published or criticized by the current worker
    let critic = engine::claim_work(&mut app, NOW + 2, "historical-critic-lease", &config())
        .unwrap()
        .unwrap();
    unchanged(&app); // critic-sent
    engine::fail_work(
        &mut app,
        &critic,
        "Synthetic unknown transport",
        true,
        NOW + 3,
    )
    .unwrap();
    unchanged(&app); // unknown outcome and reservation retained
    engine::pause_for_restore(&mut app, NOW + 4).unwrap();
    unchanged(&app);
    let historical_jobs = app.jobs.clone();
    engine::resume_work(&mut app).unwrap();
    unchanged(&app);
    assert_eq!(
        app.jobs, historical_jobs,
        "Resuming assessments never reactivates historical generation"
    );
}
#[test]
fn native_and_existing_answer_assessment_are_not_generation() {
    let mut app = App::new("retirement-synthetic-csrf-at-least-24".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    for q in app
        .questions
        .values_mut()
        .filter(|q| q.content().kind == "choice")
    {
        q.card.due_ms = NOW + DAY_MS;
    }
    app.occurrence = None;
    engine::ensure_occurrence(&mut app, NOW, "answer-check").unwrap();
    let occurrence = app.occurrence.as_ref().unwrap().id.clone();
    let f = fields(
        &app,
        &[
            ("occurrence_id", &occurrence),
            ("answer", "A nonexact synthetic answer"),
        ],
    );
    engine::admit_current_mutation("/review/answer").unwrap();
    engine::mutate(&mut app, "/review/answer", &f, NOW, "answer-check").unwrap();
    let jobs = app.jobs.clone();
    let work = engine::claim_grading_work(&mut app, NOW, "assessment-lease", &config())
        .unwrap()
        .unwrap();
    assert_eq!(work.kind, "assessment");
    assert_eq!(app.jobs, jobs);
    assert!(
        app.spend
            .iter()
            .all(|s| s.purpose.as_deref() == Some("grade.jev"))
    );
}
#[test]
fn review_home_never_consumes_or_exposes_interrupted_legacy_practice() {
    let mut app = App::new("retirement-synthetic-csrf-at-least-24".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    engine::ensure_occurrence(&mut app, NOW, "interrupted").unwrap();
    let before = app.clone();
    let html = web::render(&app, "/", &BTreeMap::new(), NOW, None);
    assert!(html.contains("data-state=\"review-home\""));
    assert!(html.contains("/library#nato-starter"));
    assert!(html.contains("Resume earlier practice"));
    assert!(!html.contains("href=\"/create\"") && !html.contains("Add material"));
    assert!(!html.contains(&app.occurrence.as_ref().unwrap().presentation.answer));
    assert_eq!(app, before);
    for ready in ["true", "false"] {
        let q = BTreeMap::from([
            ("__preparation_ready".into(), ready.into()),
            ("intent".into(), "Unsent <draft>".into()),
        ]);
        let html = web::render(&app, "/create", &q, NOW, None);
        assert!(html.contains("Native content generation is retired"));
        assert!(html.contains("Unsent &lt;draft&gt;"));
        assert!(!html.contains("action=\"/create\"") && !html.contains("type=\"file\""));
    }
}

#[test]
fn legacy_ungraded_answer_is_readable_without_self_rating_or_fake_result() {
    let mut app = App::new("retirement-synthetic-csrf-at-least-24".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    for question in app
        .questions
        .values_mut()
        .filter(|q| q.content().kind == "choice")
    {
        question.card.due_ms = NOW + DAY_MS;
    }
    app.occurrence = None;
    engine::ensure_occurrence(&mut app, NOW, "saved-unsupported-answer").unwrap();
    let occurrence = app.occurrence.as_ref().unwrap().id.clone();
    let answer = fields(
        &app,
        &[
            ("occurrence_id", &occurrence),
            ("answer", "Saved earlier answer"),
        ],
    );
    engine::mutate(&mut app, "/review/answer", &answer, NOW, "saved-answer").unwrap();
    let mut unavailable = config();
    unavailable.jev = false;
    assert!(
        engine::claim_grading_work(&mut app, NOW, "unavailable-grader", &unavailable)
            .unwrap()
            .is_none()
    );
    assert_eq!(app.occurrence.as_ref().unwrap().phase, "self-check");
    engine::validate_app(&app).unwrap();
    let before = app.clone();
    let html = web::render(&app, "/notebook", &BTreeMap::new(), NOW, None);
    assert!(html.contains("Your answer is saved, not graded."));
    assert!(html.contains("Saved earlier answer"));
    for route in ["/review/self", "/review/override", "/practice/correct"] {
        assert!(!html.contains(&format!("action=\"{route}\"")));
    }
    let home = web::render(&app, "/", &BTreeMap::new(), NOW, None);
    assert!(home.contains("Read saved earlier answer"));
    assert!(!home.contains("Resume earlier practice"));
    assert_eq!(app, before);
}
