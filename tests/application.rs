use scry::{
    engine::{self, WorkConfig},
    generation,
    model::*,
    persistence,
};
use serde_json::json;
use std::collections::BTreeMap;

const NOW: i64 = 1_791_410_400_000;
fn fresh() -> App {
    App::new("test-owner-csrf-at-least-32-characters".into())
}
fn fixture() -> App {
    let mut a = fresh();
    engine::seed_fixture(&mut a, NOW).unwrap();
    for q in a
        .questions
        .values_mut()
        .filter(|q| q.content().kind == "choice")
    {
        q.card.due_ms = NOW + DAY_MS;
    }
    a.occurrence = None;
    engine::ensure_occurrence(&mut a, NOW, "recall-fixture").unwrap();
    a
}
fn fields(a: &App, operation: &str, extra: &[(&str, &str)]) -> BTreeMap<String, String> {
    let mut f = BTreeMap::from([
        ("csrf".into(), a.csrf.clone()),
        ("operation_id".into(), operation.into()),
    ]);
    if let Some(o) = &a.occurrence {
        f.insert("occurrence_id".into(), o.id.clone());
    }
    for (k, v) in extra {
        f.insert((*k).into(), (*v).into());
    }
    f
}
fn apply(a: &mut App, path: &str, operation: &str, extra: &[(&str, &str)]) {
    let f = fields(a, operation, extra);
    engine::mutate(a, path, &f, NOW, operation).unwrap();
}
fn config() -> WorkConfig {
    WorkConfig {
        generation: true,
        jev: true,
        critic: false,
        model: "test-generation".into(),
        jev_model: "test-jev".into(),
    }
}
fn no_models() -> WorkConfig {
    WorkConfig {
        generation: false,
        jev: false,
        critic: false,
        ..config()
    }
}

#[test]
fn exact_answer_event_schedule_and_receipt_are_one_transition() {
    let mut a = fixture();
    let expected = a.occurrence.as_ref().unwrap().presentation.answer.clone();
    let f = fields(&a, "answer-operation-0001", &[("answer", &expected)]);
    let r = engine::mutate(&mut a, "/review/answer", &f, NOW, "answer-1").unwrap();
    assert_eq!(a.events.len(), 1);
    assert_eq!(a.events[0].result.outcome, "correct");
    assert_eq!(a.events[0].result.authority, "exact");
    let saved = a.clone();
    assert_eq!(
        engine::mutate(
            &mut a,
            "/review/answer",
            &f,
            NOW + 5000,
            "different-random-id"
        )
        .unwrap(),
        r
    );
    assert_eq!(a, saved);
    let mut conflicting = f.clone();
    conflicting.insert("answer".into(), "a different answer".into());
    assert_eq!(
        engine::mutate(&mut a, "/review/answer", &conflicting, NOW, "answer-2")
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(a, saved);
    let q = &a.questions[&a.events[0].question_id];
    assert_eq!(q.card, a.events[0].after_card);
    assert_eq!(q.schedule_version, 1);
    let rows = persistence::rows(&a).unwrap();
    let mut restarted = persistence::app_from_rows(&rows).unwrap();
    engine::ensure_occurrence(&mut restarted, NOW + 1000, "restart").unwrap();
    assert_eq!(restarted, a);
}

#[test]
fn failures_do_not_partially_capture_and_csrf_is_required() {
    let mut a = fresh();
    let before = a.clone();
    let too_large = "x".repeat(TEXT_LIMIT + 1);
    let f = fields(&a, "capture-operation-01", &[("intent", &too_large)]);
    assert_eq!(
        engine::mutate(&mut a, "/create", &f, NOW, "capture")
            .unwrap_err()
            .status,
        422
    );
    assert_eq!(a, before);
    let mut f = fields(&a, "capture-operation-02", &[("intent", "HTTP caching")]);
    f.insert("csrf".into(), "forged".into());
    assert_eq!(
        engine::mutate(&mut a, "/create", &f, NOW, "capture")
            .unwrap_err()
            .status,
        403
    );
    assert_eq!(a, before);
}

#[test]
fn capture_word_phrase_dictated_and_url_stay_private_and_exact() {
    for input in [
        "photosynthesis",
        "why HTTP caches go stale",
        "I know how requests work, but I get confused about stale content. I want to learn when to use no-cache and what a 304 means, so I can debug my web app.",
        "https://private.example/notes?q=secret",
    ] {
        let mut a = fresh();
        let f = fields(&a, "capture-operation-03", &[("intent", input)]);
        let r = engine::mutate(&mut a, "/create", &f, NOW, "capture").unwrap();
        assert_eq!(a.goals.len(), 1);
        assert_eq!(a.jobs.len(), 1);
        assert_eq!(a.goals.values().next().unwrap().intent, input);
        let w = engine::claim_work(&mut a, NOW, "send-lease-0000001", &config())
            .unwrap()
            .unwrap();
        assert_eq!(w.kind, "generation");
        assert!(w.request.get("messages").is_some());
        assert!(w.request.get("search").is_none());
        assert_eq!(
            engine::mutate(&mut a, "/create", &f, NOW + 2000, "capture-replay").unwrap(),
            r
        );
        assert_eq!(a.goals.len(), 1);
    }
}

#[test]
fn look_up_is_durable_and_cannot_become_unaided_success() {
    let mut a = fixture();
    let before = a.occurrence.as_ref().unwrap().before_card.clone();
    assert!(a.protected_goal().is_some());
    apply(&mut a, "/review/help", "assistance-operation1", &[]);
    assert!(a.protected_goal().is_none());
    let mut b = persistence::app_from_rows(&persistence::rows(&a).unwrap()).unwrap();
    let answer = b.occurrence.as_ref().unwrap().presentation.answer.clone();
    apply(
        &mut b,
        "/review/answer",
        "assisted-answer-0001",
        &[("answer", &answer)],
    );
    let e = &b.events[0];
    assert!(e.assisted);
    assert_eq!(e.result.rating, 0);
    assert_eq!(e.result.outcome, "warm_correct");
    assert_eq!(e.after_card, before);
}

#[test]
fn reveal_is_held_and_next_is_deliberate() {
    let mut a = fixture();
    let oid = a.occurrence.as_ref().unwrap().id.clone();
    apply(&mut a, "/review/reveal", "reveal-operation-001", &[]);
    assert_eq!(a.events[0].result.outcome, "revealed");
    assert_eq!(a.events[0].result.rating, 1);
    engine::ensure_occurrence(&mut a, NOW + 100, "read-only").unwrap();
    assert_eq!(a.occurrence.as_ref().unwrap().id, oid);
    apply(&mut a, "/review/next", "next-operation-0001", &[]);
    assert_ne!(a.occurrence.as_ref().unwrap().id, oid);
    assert_eq!(a.events.len(), 1);
}

#[test]
fn missing_assessor_keeps_answer_and_learner_authority() {
    let mut a = fixture();
    apply(
        &mut a,
        "/review/answer",
        "semantic-answer-0001",
        &[("answer", "an attempt in my own words")],
    );
    assert_eq!(a.occurrence.as_ref().unwrap().phase, "checking");
    assert!(a.events.is_empty());
    assert!(
        engine::claim_work(&mut a, NOW, "missing-assessor-01", &no_models())
            .unwrap()
            .is_none()
    );
    assert_eq!(a.occurrence.as_ref().unwrap().phase, "self-check");
    assert_eq!(
        a.occurrence.as_ref().unwrap().answer.as_deref(),
        Some("an attempt in my own words")
    );
    assert!(a.spend.is_empty());
    apply(
        &mut a,
        "/review/self",
        "self-check-operation1",
        &[("correct", "true")],
    );
    assert_eq!(a.events[0].result.authority, "learner");
    assert!(!a.events[0].assisted);
    assert_eq!(a.events[0].result.rating, 3);
}

#[test]
fn assessment_send_once_timeout_unknown_reservation_and_explicit_retry() {
    let mut a = fixture();
    apply(
        &mut a,
        "/review/answer",
        "semantic-answer-0002",
        &[("answer", "caching directives")],
    );
    let work = engine::claim_work(&mut a, NOW, "semantic-lease-001", &config())
        .unwrap()
        .unwrap();
    assert_eq!(work.kind, "assessment");
    assert_eq!(a.spent(NOW), RESERVATION);
    assert!(
        engine::claim_work(&mut a, NOW + 1000, "second-lease-001", &config())
            .unwrap()
            .is_none()
    );
    assert!(engine::recover(&mut a, NOW + 90_001));
    assert_eq!(a.occurrence.as_ref().unwrap().phase, "self-check");
    assert_eq!(a.spend[0].status, "unknown");
    assert_eq!(a.spent(NOW), RESERVATION);
    assert!(a.events.is_empty());
    let raw=serde_json::to_string(&json!({"answers":{"verdict":{"choice":"accept","probabilities":{"accept":0.99,"reject":0.005,"unsure":0.005}},"identity":{"noul":0.0},"injection":{"noul":0.0}}})).unwrap();
    assert_eq!(
        engine::finish_work(&mut a, &work, raw, "jev".into(), Some(12_000), NOW + 90_002)
            .unwrap_err()
            .status,
        409
    );
    assert!(a.events.is_empty());
    apply(&mut a, "/review/retry", "explicit-retry-0001", &[]);
    let next = engine::claim_work(&mut a, NOW + 91_000, "new-lease-explicit", &config())
        .unwrap()
        .unwrap();
    assert_ne!(next.id, work.id);
    assert_eq!(a.spend.len(), 2);
}

#[test]
fn semantic_verdict_fenced_and_grade_override_keeps_original() {
    let mut a = fixture();
    apply(
        &mut a,
        "/review/answer",
        "semantic-answer-0003",
        &[("answer", "wrong answer")],
    );
    let work = engine::claim_work(&mut a, NOW, "semantic-lease-002", &config())
        .unwrap()
        .unwrap();
    let raw=json!({"answers":{"verdict":{"choice":"reject","probabilities":{"accept":0.005,"reject":0.99,"unsure":0.005}},"identity":{"noul":0.0},"injection":{"noul":0.0}}}).to_string();
    engine::finish_work(&mut a, &work, raw, "jev".into(), Some(12_000), NOW + 100).unwrap();
    assert_eq!(a.events[0].result.outcome, "wrong");
    let original = a.events[0].clone();
    apply(
        &mut a,
        "/review/override",
        "grade-override-0001",
        &[("correct", "true")],
    );
    assert_eq!(a.events[0], original);
    assert_eq!(a.overrides.len(), 1);
    assert!(
        a.occurrence
            .as_ref()
            .unwrap()
            .result
            .as_ref()
            .unwrap()
            .corrected
    );
    let fields = fields(&a, "grade-override-0002", &[("correct", "false")]);
    assert_eq!(
        engine::mutate(&mut a, "/review/override", &fields, NOW, "again")
            .unwrap_err()
            .status,
        409
    );
}

#[test]
fn allowance_is_shared_and_unknown_cost_is_never_free() {
    let mut a = fixture();
    for i in 0..7 {
        a.spend.push(Spend {
            id: format!("prior-{i}"),
            work_id: format!("work-{i}"),
            created_ms: NOW,
            reserved_micros: RESERVATION,
            cost_micros: None,
            status: "unknown".into(),
            model: "test".into(),
            response_hash: None,
            response_model: None,
            raw_response: None,
            response_bytes: None,
            response_outcome: None,
        });
    }
    apply(
        &mut a,
        "/review/answer",
        "semantic-budget-0001",
        &[("answer", "cache control")],
    );
    assert!(
        engine::claim_work(&mut a, NOW, "no-budget-lease-01", &config())
            .unwrap()
            .is_none()
    );
    assert_eq!(a.spend.len(), 7);
    assert_eq!(a.occurrence.as_ref().unwrap().phase, "self-check");
    assert_eq!(a.spent(NOW + DAY_MS + 1), 0);
}

#[test]
fn content_edits_leave_historical_presentations_immutable() {
    let mut a = fixture();
    apply(&mut a, "/review/reveal", "reveal-before-edit1", &[]);
    let qid = a.events[0].question_id.clone();
    let old = a.events[0].clone();
    let c = a.questions[&qid].content().clone();
    let extra = [
        ("content_version", "1"),
        (
            "prompt",
            "Which directive controls a stored response's freshness lifetime?",
        ),
        ("kind", c.kind.as_str()),
        ("answer", c.answer.as_str()),
        ("variants", ""),
        ("choices", ""),
        ("explanation", c.explanation.as_str()),
        ("basis", "general"),
        ("quotes", ""),
    ];
    apply(
        &mut a,
        &format!("/questions/{qid}/edit"),
        "content-edit-operation1",
        &extra,
    );
    assert_eq!(a.events[0], old);
    assert_eq!(a.questions[&qid].versions.len(), 2);
    assert_eq!(
        a.occurrence.as_ref().unwrap().presentation,
        old.presentation
    );
}

#[test]
fn competing_occurrences_and_archive_cannot_publish_stale_answer() {
    let mut a = fixture();
    let old = fields(&a, "late-answer-operation1", &[("answer", "max-age")]);
    apply(&mut a, "/review/reveal", "reveal-other-tab-01", &[]);
    apply(&mut a, "/review/next", "next-other-tab-0001", &[]);
    let before = a.clone();
    assert_eq!(
        engine::mutate(&mut a, "/review/answer", &old, NOW, "stale")
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(a, before);
}

#[test]
fn refinement_uses_real_feedback_and_never_rewrites_attempts() {
    let mut a = fixture();
    let goal = a.goals.keys().next().unwrap().clone();
    apply(
        &mut a,
        &format!("/goals/{goal}/refine"),
        "refine-operation-001",
        &[(
            "feedback",
            "Help me distinguish freshness from validation using a concrete example.",
        )],
    );
    let work = engine::claim_work(&mut a, NOW, "refine-lease-00001", &config())
        .unwrap()
        .unwrap();
    let user = work.request["messages"][1]["content"].as_str().unwrap();
    assert!(user.contains("Help me distinguish"));
    assert!(user.contains("observed_attempts"));
    assert!(a.events.is_empty());
}

#[test]
fn restored_state_keeps_paid_work_paused_and_held_feedback() {
    let mut a = fixture();
    apply(
        &mut a,
        "/review/answer",
        "restore-assess-0001",
        &[("answer", "cache headers")],
    );
    engine::claim_work(&mut a, NOW, "restore-lease-001", &config())
        .unwrap()
        .unwrap();
    let spend = a.spend.clone();
    engine::pause_for_restore(&mut a, NOW + 1);
    assert!(a.restored_paused);
    assert_eq!(a.spend, spend);
    assert_eq!(a.occurrence.as_ref().unwrap().phase, "self-check");
    assert!(
        engine::claim_work(&mut a, NOW + 500_000, "cannot-send-lease", &config())
            .unwrap()
            .is_none()
    );
}

#[test]
fn candidates_are_saved_before_critic_and_critic_retry_reuses_generation() {
    let mut a = fresh();
    apply(
        &mut a,
        "/create",
        "candidate-capture001",
        &[("intent", "HTTP caching")],
    );
    let mut cfg = config();
    cfg.critic = true;
    let work = engine::claim_work(&mut a, NOW, "generator-lease001", &cfg)
        .unwrap()
        .unwrap();
    let fixture = fixture();
    let batch = fixture
        .jobs
        .values()
        .next()
        .unwrap()
        .candidate_json
        .clone()
        .unwrap();
    let raw = json!({"choices":[{"message":{"content":batch}}]}).to_string();
    engine::finish_work(
        &mut a,
        &work,
        raw,
        "selected-model".into(),
        Some(50_000),
        NOW + 1,
    )
    .unwrap();
    assert!(a.questions.is_empty());
    assert_eq!(a.jobs[&work.id].status, "candidates");
    let critic = engine::claim_work(&mut a, NOW + 2, "critic-lease00001", &cfg)
        .unwrap()
        .unwrap();
    assert_eq!(critic.kind, "critic");
    engine::fail_work(&mut a, &critic, "Unavailable", true, NOW + 3).unwrap();
    let goal = a.goals.keys().next().unwrap().clone();
    apply(
        &mut a,
        &format!("/goals/{goal}/retry"),
        "retry-critic-00001",
        &[],
    );
    let retry = engine::claim_work(&mut a, NOW + 4, "retry-critic-lease", &cfg)
        .unwrap()
        .unwrap();
    assert_eq!(retry.kind, "critic");
    assert_ne!(retry.id, work.id);
}

#[test]
fn missing_generation_never_fabricates_content() {
    let mut a = fresh();
    apply(
        &mut a,
        "/create",
        "missing-capture001",
        &[("intent", "photosynthesis")],
    );
    assert!(
        engine::claim_work(&mut a, NOW, "missing-model-001", &no_models())
            .unwrap()
            .is_none()
    );
    assert!(a.questions.is_empty());
    assert!(a.concepts.is_empty());
    assert!(a.spend.is_empty());
    assert_eq!(a.goals.values().next().unwrap().status, "failed");
}

#[test]
fn export_readback_roundtrip_preserves_complete_schedule_and_evidence() {
    let mut a = fixture();
    apply(&mut a, "/review/reveal", "export-reveal00001", &[]);
    engine::validate_app(&a).unwrap();
    let value = serde_json::to_value(&a).unwrap();
    let copy: App = serde_json::from_value(value).unwrap();
    assert_eq!(a, copy);
    let rows = persistence::rows(&a).unwrap();
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), a);
    assert_eq!(generation::RESPONSE_LIMIT, 256 * 1024);
}
