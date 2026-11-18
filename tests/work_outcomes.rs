mod common;

use scry::{
    engine::{self, WorkConfig},
    model::*,
};
use serde_json::json;
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
fn cfg() -> WorkConfig {
    WorkConfig {
        generation: true,
        jev: true,
        critic: false,
        model: "synthetic-generator".into(),
        jev_model: "synthetic-jev".into(),
    }
}
/// The retired authored demo's published Goal, with no question held.
fn fixture() -> App {
    let (mut a, now) = common::earlier("question-cold-choice");
    assert_eq!(now, NOW);
    a.occurrence = None;
    a
}
fn act(a: &mut App, path: &str, op: &str, extra: &[(&str, &str)], now: i64) {
    let mut f = BTreeMap::from([
        ("csrf".into(), a.csrf.clone()),
        ("operation_id".into(), op.into()),
    ]);
    if let Some(o) = &a.occurrence {
        f.insert("occurrence_id".into(), o.id.clone());
    }
    for (k, v) in extra {
        f.insert((*k).into(), (*v).into());
    }
    engine::mutate(a, path, &f, now, op).unwrap();
}
#[test]
fn changed_question_blocks_fix_before_paid_send() {
    let mut a = fixture();
    let qid = a.questions.keys().next().unwrap().clone();
    act(
        &mut a,
        &format!("/questions/{qid}/fix"),
        "stale-fix-request01",
        &[("content_version", "1")],
        NOW,
    );
    // The question gains a version after the fix was requested, as an edit
    // once saved one.
    let q = a.questions.get_mut(&qid).unwrap();
    let mut edited = q.content().clone();
    edited.version += 1;
    edited.explanation = "A new authored explanation supersedes the draft source.".into();
    edited.created_ms = NOW + 1;
    q.versions.push(edited);
    assert!(
        engine::claim_work(&mut a, NOW + 2, "stale-fix-no-send01", &cfg())
            .unwrap()
            .is_none()
    );
    assert!(a.spend.is_empty());
    assert!(
        a.jobs
            .values()
            .any(|j| j.kind == "fix" && j.status == "superseded")
    );
}
#[test]
fn refinement_retry_is_bounded_but_new_feedback_starts_fresh_attempt() {
    let mut a = fixture();
    let goal = a.goals.keys().next().unwrap().clone();
    act(
        &mut a,
        &format!("/goals/{goal}/refine"),
        "bounded-refine-0001",
        &[("feedback", "Explain with a more concrete example.")],
        NOW,
    );
    for n in 1..=3 {
        let w = engine::claim_work(
            &mut a,
            NOW + n,
            &format!("bounded-paid-lease{n:02}"),
            &cfg(),
        )
        .unwrap()
        .unwrap();
        assert_eq!(a.jobs[&w.id].attempt, n as u8);
        engine::fail_work(&mut a, &w, "Unknown outcome", true, NOW + n).unwrap();
        if n < 3 {
            act(
                &mut a,
                &format!("/goals/{goal}/retry"),
                &format!("bounded-retry-op-{n:02}"),
                &[],
                NOW + n + 1,
            );
        }
    }
    let before = a.clone();
    let fields = BTreeMap::from([
        ("csrf".into(), a.csrf.clone()),
        ("operation_id".into(), "bounded-retry-four01".into()),
    ]);
    assert_eq!(
        engine::mutate(
            &mut a,
            &format!("/goals/{goal}/retry"),
            &fields,
            NOW + 10,
            "fourth"
        )
        .unwrap_err()
        .status,
        422
    );
    assert_eq!(a, before);
    act(
        &mut a,
        &format!("/goals/{goal}/refine"),
        "new-feedback-000001",
        &[(
            "feedback",
            "Use browser refresh and a changed image as the new example.",
        )],
        NOW + 11,
    );
    assert_eq!(a.jobs[&a.goals[&goal].job_id].attempt, 1);
    assert_eq!(a.spend.len(), 3);
    assert!(
        a.spend
            .iter()
            .all(|s| s.status == "unknown" && s.cost_micros.is_none())
    );
}
#[test]
fn rejected_transport_never_claims_a_free_paid_outcome() {
    let mut a = fixture();
    let goal = a.goals.keys().next().unwrap().clone();
    act(
        &mut a,
        &format!("/goals/{goal}/refine"),
        "rejected-send-00001",
        &[("feedback", "Explain with a more concrete example.")],
        NOW,
    );
    let w = engine::claim_work(&mut a, NOW, "rejected-lease-0001", &cfg())
        .unwrap()
        .unwrap();
    engine::fail_work(
        &mut a,
        &w,
        "Provider rejected request; cost absent",
        false,
        NOW + 1,
    )
    .unwrap();
    assert_eq!(a.spend[0].status, "unknown");
    assert_eq!(a.spend[0].cost_micros, None);
    assert_eq!(a.spend[0].reserved_micros, RESERVATION);
}

#[test]
fn skipped_critic_publishes_saved_candidates_without_a_second_reservation() {
    let authored = fixture()
        .jobs
        .values()
        .next()
        .unwrap()
        .candidate_json
        .clone()
        .unwrap();
    let mut a = App::new("skipped-critic-owner-csrf-32-characters".into());
    act(
        &mut a,
        "/create",
        "skipped-create-0001",
        &[("intent", "Understand HTTP caching")],
        NOW,
    );
    let work = engine::claim_work(&mut a, NOW, "skipped-generator-lease", &cfg())
        .unwrap()
        .unwrap();
    let raw = json!({"choices":[{"message":{"content":authored}}]}).to_string();
    engine::finish_work(
        &mut a,
        &work,
        raw,
        "authored-generator".into(),
        Some(12_000),
        NOW + 1,
    )
    .unwrap();
    assert_eq!(a.jobs[&work.id].status, "candidates");
    assert!(a.questions.is_empty());
    let prior = a.spend.clone();
    assert!(
        engine::claim_work(&mut a, NOW + 2, "must-not-be-reserved", &cfg())
            .unwrap()
            .is_none()
    );
    assert_eq!(a.spend, prior);
    assert_eq!(
        a.jobs[&work.id].critic_json.as_deref(),
        Some("{\"status\":\"skipped\"}")
    );
    assert_eq!(a.jobs[&work.id].status, "ready");
    assert_eq!(a.questions.len(), 3);
    assert_eq!(a.concepts.len(), 2);
    engine::validate_app(&a).unwrap();
}
