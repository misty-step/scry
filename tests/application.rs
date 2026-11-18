mod common;

use scry::{
    engine::{self, WorkConfig},
    generation,
    model::*,
    persistence,
};
use serde_json::json;
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
fn fresh() -> App {
    App::new("test-owner-csrf-at-least-32-characters".into())
}
/// The retired authored demo as it left the notebook: one published Goal and
/// its choice question held unanswered.
fn fixture() -> App {
    let (a, now) = common::earlier("question-cold-choice");
    assert_eq!(now, NOW);
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
            model: Some("test".into()),
            purpose: None,
            response_hash: None,
            response_model: None,
            raw_response: None,
            response_bytes: None,
            response_outcome: None,
        });
    }
    let goal = a.goals.keys().next().unwrap().clone();
    apply(
        &mut a,
        &format!("/goals/{goal}/refine"),
        "semantic-budget-0001",
        &[("feedback", "Use a concrete example of validation.")],
    );
    assert!(
        engine::claim_work(&mut a, NOW, "no-budget-lease-01", &config())
            .unwrap()
            .is_none()
    );
    assert_eq!(a.spend.len(), 7);
    assert_eq!(a.jobs[&a.goals[&goal].job_id].status, "failed");
    assert_eq!(a.spent(NOW + DAY_MS + 1), 0);
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
fn full_text_refinement_keeps_existing_material_when_generation_or_critic_is_partial() {
    for critic_veto in [false, true] {
        let mut a = fixture();
        let goal = a.goals.keys().next().unwrap().clone();
        let original_questions = serde_json::to_value(&a.questions).unwrap();
        let original_concepts = serde_json::to_value(&a.concepts).unwrap();
        let mut batch: generation::Batch = serde_json::from_str(
            a.jobs
                .values()
                .next()
                .unwrap()
                .candidate_json
                .as_ref()
                .unwrap(),
        )
        .unwrap();
        batch.complete = critic_veto;
        apply(
            &mut a,
            &format!("/goals/{goal}/refine"),
            "full-text-refine-0001",
            &[(
                "feedback",
                "I need to memorize the full text line by line, not just a few examples.",
            )],
        );
        let mut cfg = config();
        cfg.critic = true;
        let work = engine::claim_work(&mut a, NOW, "full-text-generator-lease", &cfg)
            .unwrap()
            .unwrap();
        let raw =
            json!({"choices":[{"message":{"content":serde_json::to_string(&batch).unwrap()}}]})
                .to_string();
        engine::finish_work(
            &mut a,
            &work,
            raw,
            "test-generator".into(),
            Some(1),
            NOW + 1,
        )
        .unwrap();
        if critic_veto {
            let mut vetoed = false;
            for i in 0..100 {
                let Some(check) = engine::claim_work(
                    &mut a,
                    NOW + 2 + i,
                    &format!("full-text-critic-lease-{i}"),
                    &cfg,
                )
                .unwrap() else {
                    break;
                };
                assert_eq!(check.kind, "critic");
                let answers: serde_json::Map<String, serde_json::Value> =
                    check.request["questions"]
                        .as_object()
                        .unwrap()
                        .keys()
                        .map(|key| {
                            let probability = if !vetoed && key.ends_with("prompt_leaks_answer") {
                                vetoed = true;
                                0.90
                            } else {
                                0.01
                            };
                            (key.clone(), json!({"type":"noul","noul":probability}))
                        })
                        .collect();
                engine::finish_work(
                    &mut a,
                    &check,
                    json!({"answers":answers}).to_string(),
                    "test-critic".into(),
                    Some(1),
                    NOW + 3 + i,
                )
                .unwrap();
            }
            assert!(vetoed);
        }
        assert_eq!(a.jobs[&work.id].status, "failed");
        assert!(a.jobs[&work.id].raw_response.is_some());
        assert_eq!(
            serde_json::to_value(&a.questions).unwrap(),
            original_questions
        );
        assert_eq!(
            serde_json::to_value(&a.concepts).unwrap(),
            original_concepts
        );
        assert!(a.events.is_empty());
        assert!(a.spend.iter().all(|s| s.status == "known"));
        let saved = persistence::Archive::new(a, vec![], NOW).encode().unwrap();
        assert_eq!(
            persistence::Archive::decode(&saved, &persistence::sha256(&saved))
                .unwrap()
                .encode()
                .unwrap(),
            saved
        );
    }
}

#[test]
fn restored_state_keeps_paid_work_paused_and_held_feedback() {
    // A non-exact answer whose check was sent, with its reservation.
    let (mut a, now) = common::earlier("checking-sent");
    assert!(now >= NOW);
    let sent = a.clone();
    assert_eq!(sent.occurrence.as_ref().unwrap().phase, "checking");
    engine::pause_restored(&mut a, NOW + 1).unwrap();
    assert!(a.restored_paused);
    assert_eq!(a.spend, sent.spend);
    // The earlier notebook is restored as it was saved: the sent check, the
    // held answer and the Concepts are not rewritten.
    assert_eq!(a.assessments, sent.assessments);
    assert_eq!(a.occurrence, sent.occurrence);
    assert_eq!(a.concepts, sent.concepts);
    // Neither the alarm's claim nor the historical one sends from it.
    for later in [NOW + 500_000, NOW + 10 * 365 * DAY_MS] {
        assert!(!engine::recover_checks(&mut a, later));
        assert!(
            engine::claim_check_work(&mut a, later, "cannot-send-check", &config())
                .unwrap()
                .is_none()
        );
    }
    assert!(
        engine::claim_work(&mut a, NOW + 500_000, "cannot-send-lease", &config())
            .unwrap()
            .is_none()
    );
    let before = a.clone();
    let resume = fields(&a, "resume-restored-notebook", &[]);
    engine::mutate(&mut a, "/settings/resume", &resume, NOW + 2, "resume").unwrap();
    assert!(!a.restored_paused);
    assert_eq!(a.jobs, before.jobs);
    assert_eq!(a.assessments, before.assessments);
    assert_eq!(a.spend, before.spend);
    assert_eq!(a.occurrence, before.occurrence);
    assert_eq!(a.questions, before.questions);
    let resumed = a.clone();
    engine::mutate(&mut a, "/settings/resume", &resume, NOW + 3, "replay").unwrap();
    assert_eq!(a, resumed);
    let again = fields(&a, "resume-already-resumed", &[]);
    assert_eq!(
        engine::mutate(&mut a, "/settings/resume", &again, NOW + 4, "again")
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(a, resumed);
    // Resumed, the alarm still recovers and claims nothing of the saved check.
    assert!(!engine::recover_checks(&mut a, NOW + 10 * 365 * DAY_MS));
    assert!(
        engine::claim_check_work(
            &mut a,
            NOW + 10 * 365 * DAY_MS,
            "never-resend-check",
            &config()
        )
        .unwrap()
        .is_none()
    );
    assert_eq!(a, resumed);
    assert!(
        engine::claim_work(&mut a, NOW + 5, "never-resend-history", &config())
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
    // An answer revealed, held as its result.
    let (a, _) = common::earlier("result-revealed");
    engine::validate_app(&a).unwrap();
    let value = serde_json::to_value(&a).unwrap();
    let copy: App = serde_json::from_value(value).unwrap();
    assert_eq!(a, copy);
    let rows = persistence::rows(&a).unwrap();
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), a);
    assert_eq!(generation::RESPONSE_LIMIT, 256 * 1024);
}
