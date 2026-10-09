use scry::{
    engine::{self, WorkConfig},
    model::*,
    persistence,
};
use serde_json::json;
use std::collections::BTreeMap;
const NOW: i64 = 1_791_410_400_000;
fn cfg() -> WorkConfig {
    WorkConfig {
        generation: true,
        jev: true,
        critic: false,
        model: "synthetic-generator".into(),
        jev_model: "synthetic-jev".into(),
    }
}
fn fixture() -> App {
    let mut a = App::new("paid-outcome-owner-csrf-32-characters".into());
    engine::seed_fixture(&mut a, NOW).unwrap();
    for q in a
        .questions
        .values_mut()
        .filter(|q| q.content().kind == "choice")
    {
        q.card.due_ms = NOW + DAY_MS;
    }
    a.occurrence = None;
    engine::ensure_occurrence(&mut a, NOW, "paid-recall").unwrap();
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
fn accepted() -> String {
    json!({"answers":{"verdict":{"choice":"accept","probabilities":{"accept":0.99,"reject":0.005,"unsure":0.005}},"identity":{"noul":0.0},"injection":{"noul":0.0}}}).to_string()
}
#[test]
fn stale_paid_assessment_retains_exact_raw_and_known_cost_without_learning_write() {
    let mut a = fixture();
    act(
        &mut a,
        "/review/answer",
        "stale-paid-answer01",
        &[("answer", "nonexact authored response")],
        NOW,
    );
    let work = engine::claim_work(&mut a, NOW, "stale-paid-lease01", &cfg())
        .unwrap()
        .unwrap();
    let goal = a.goals.keys().next().unwrap().clone();
    act(
        &mut a,
        &format!("/goals/{goal}/archive"),
        "stale-goal-archive01",
        &[],
        NOW + 1,
    );
    let raw = accepted();
    assert_eq!(
        engine::finish_work(
            &mut a,
            &work,
            raw.clone(),
            "synthetic-jev".into(),
            Some(19_500),
            NOW + 2
        )
        .unwrap_err()
        .status,
        409
    );
    assert!(a.events.is_empty());
    assert_eq!(
        a.assessments[&work.id].raw_response.as_deref(),
        Some(raw.as_str())
    );
    assert_eq!(a.assessments[&work.id].status, "superseded");
    assert_eq!(a.spend.last().unwrap().cost_micros, Some(19_500));
    assert_eq!(a.spend.last().unwrap().status, "known");
    let bytes = persistence::Archive::new(a.clone(), vec![], NOW + 3)
        .encode()
        .unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        a
    );
}
#[test]
fn changed_question_blocks_fix_before_paid_send() {
    let mut a = fixture();
    let qid = a.occurrence.as_ref().unwrap().question_id.clone();
    let q = a.questions[&qid].clone();
    act(&mut a, "/review/help", "stale-fix-help-0001", &[], NOW);
    act(
        &mut a,
        &format!("/questions/{qid}/fix"),
        "stale-fix-request01",
        &[("content_version", "1")],
        NOW,
    );
    act(
        &mut a,
        &format!("/questions/{qid}/edit"),
        "stale-fix-edit-0001",
        &[
            ("content_version", "1"),
            ("prompt", &q.content().prompt),
            ("kind", &q.content().kind),
            ("answer", &q.content().answer),
            ("basis", "general"),
            ("quotes", ""),
            (
                "explanation",
                "A new authored explanation supersedes the draft source.",
            ),
        ],
        NOW + 1,
    );
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
    act(
        &mut a,
        "/review/answer",
        "rejected-send-00001",
        &[("answer", "nonexact test answer")],
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
