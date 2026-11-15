//! Current runtime policy is deliberately separate from historical generation tests.
mod common;

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
    // The earlier concept notebook is history: nothing presents, answers,
    // checks, dismisses, archives, edits, flags, focuses, pauses or paces it,
    // and no demo seeds one.
    for path in [
        "/review/intro",
        "/review/answer",
        "/review/reveal",
        "/review/help",
        "/review/retry",
        "/review/next",
        "/review/anything-else",
        "/concepts/old/practice",
        "/goals/old/pause",
        "/goals/old/focus",
        "/goals/old/archive",
        "/concepts/old/archive",
        "/concepts/old/feedback",
        "/questions/old/edit",
        "/questions/old/archive",
        "/questions/old/dispute",
        "/settings",
        "/__fixture",
    ] {
        let error = engine::admit_current_mutation(path).unwrap_err();
        assert_eq!(error.status, 410, "{path}");
        assert_eq!(error.message, engine::EARLIER_NOTEBOOK_RETIRED, "{path}");
    }
    for path in [
        "/practice/answer",
        "/practice/reveal",
        "/practice/check",
        "/library/mapping/add",
        "/library/version/add",
        "/settings/resume",
        "/settings/judgment",
        "/settings/backup",
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
        // What the alarm runs: recovery of Checks, then a Check claim.
        let mut current = app.clone();
        assert!(!engine::recover_checks(&mut current, NOW + 100_000));
        assert!(
            engine::claim_check_work(&mut current, NOW + 100_000, "unused-lease", &config())
                .unwrap()
                .is_none()
        );
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
    // The authored candidates the retired demo published.
    let (fixture, _) = common::earlier("question-cold-choice");
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
    engine::pause_restored(&mut app, NOW + 4).unwrap();
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
fn review_home_never_consumes_or_exposes_interrupted_legacy_practice() {
    let (app, now) = common::earlier("question-cold-choice");
    assert_eq!(now, NOW);
    let before = app.clone();
    let html = web::render(&app, "/", &BTreeMap::new(), NOW, None);
    assert!(html.contains("data-state=\"mixed-home\""));
    assert!(html.contains("/practice/lesson/start"));
    assert!(html.contains("Earlier practice"));
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
    // A saved answer whose check closed unsent: Jev was not configured.
    let (app, now) = common::earlier("self-check-failed-unsent");
    let saved = app.occurrence.as_ref().unwrap().answer.clone().unwrap();
    assert_eq!(app.occurrence.as_ref().unwrap().phase, "self-check");
    engine::validate_app(&app).unwrap();
    let before = app.clone();
    let html = web::render(&app, "/notebook", &BTreeMap::new(), now, None);
    assert!(html.contains("Your answer is saved, not graded."));
    assert!(html.contains(&web::escape(&saved)));
    for route in ["/review/self", "/review/override", "/practice/correct"] {
        assert!(!html.contains(&format!("action=\"{route}\"")));
    }
    let home = web::render(&app, "/", &BTreeMap::new(), now, None);
    assert!(home.contains("href=\"/notebook\">Earlier practice</a>"));
    assert!(!home.contains("Read saved earlier answer"));
    assert_eq!(app, before);
}
