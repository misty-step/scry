use base64::{Engine as _, engine::general_purpose::STANDARD};
use scry::{
    engine::{self, Work, WorkConfig},
    generation::{self, Batch},
    model::*,
    persistence,
};
use serde_json::{Value, json};
use std::collections::BTreeMap;

const NOW: i64 = 1_791_410_400_000;
fn config() -> WorkConfig {
    WorkConfig {
        generation: true,
        jev: true,
        critic: true,
        model: "configured-generator".into(),
        jev_model: "configured-critic".into(),
    }
}
fn act(
    app: &mut App,
    path: &str,
    operation: &str,
    extra: &[(&str, &str)],
) -> BTreeMap<String, String> {
    let mut fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), operation.into()),
    ]);
    for (key, value) in extra {
        fields.insert((*key).into(), (*value).into());
    }
    engine::mutate(app, path, &fields, NOW, operation).unwrap();
    fields
}
fn prepared() -> (App, Batch, Work) {
    let mut fixture = App::new("critic-authored-fixture-csrf-32-characters".into());
    engine::seed_fixture(&mut fixture, NOW).unwrap();
    let mut batch: Batch = serde_json::from_str(
        fixture
            .jobs
            .values()
            .next()
            .unwrap()
            .candidate_json
            .as_deref()
            .unwrap(),
    )
    .unwrap();
    batch.concepts.truncate(1);
    let mut app = App::new("critic-reuse-owner-csrf-32-characters".into());
    act(
        &mut app,
        "/create",
        "critic-capture-op001",
        &[("intent", "HTTP caching")],
    );
    let work = engine::claim_work(&mut app, NOW, "critic-generator-lease01", &config())
        .unwrap()
        .unwrap();
    let raw = json!({"choices":[{"message":{"content":serde_json::to_string(&batch).unwrap()}}]})
        .to_string();
    engine::finish_work(
        &mut app,
        &work,
        raw,
        "actual-generator".into(),
        Some(10_000),
        NOW + 1,
    )
    .unwrap();
    let critic = engine::claim_work(&mut app, NOW + 2, "critic-first-lease001", &config())
        .unwrap()
        .unwrap();
    (app, batch, critic)
}
fn response(request: &Value) -> Value {
    let answers: serde_json::Map<String, Value> = request["questions"]
        .as_object()
        .unwrap()
        .keys()
        .map(|key| (key.clone(), json!({"type":"noul","noul":0.01})))
        .collect();
    json!({"answers":answers})
}
fn archive(app: &App) {
    let bytes = persistence::Archive::new(app.clone(), vec![], NOW + 20)
        .encode()
        .unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        *app
    );
}

#[test]
fn explicit_critic_retry_sends_only_unfinished_batteries_and_preserves_attempts() {
    let (mut app, batch, work) = prepared();
    let mut partial = response(&work.request);
    partial["answers"]["c0_q0_prompt_leaks_answer"]["noul"] = json!(0.90);
    partial["answers"]
        .as_object_mut()
        .unwrap()
        .remove("c0_q1_no_defensible_answer");
    partial["answers"]["c0_q1_adversarial_content"] = json!({"noul":0.01,"choice":"accept"});
    let raw = partial.to_string();
    engine::finish_work(
        &mut app,
        &work,
        raw.clone(),
        "actual-critic-first".into(),
        Some(12_000),
        NOW + 3,
    )
    .unwrap();
    assert_eq!(app.jobs[&work.id].status, "failed");
    assert!(
        app.jobs[&work.id]
            .rejected
            .iter()
            .any(|reason| reason.contains("prompt_leaks_answer"))
    );
    assert!(app.questions.is_empty());
    let cache =
        generation::read_critic_cache(&batch, app.jobs[&work.id].critic_cache_json.as_deref())
            .unwrap();
    assert_eq!(cache.answers["c0_q0_prompt_leaks_answer"], 0.90);
    assert!(!cache.answers.keys().any(|key| key.starts_with("c0_q1_")));
    let old_job = app.jobs[&work.id].clone();
    let old_spend = app.spend.clone();
    archive(&app);
    let goal = app.goals.keys().next().unwrap().clone();
    let path = format!("/goals/{goal}/retry");
    let fields = act(&mut app, &path, "critic-retry-op0001", &[]);
    let after_retry = app.clone();
    engine::mutate(&mut app, &path, &fields, NOW + 4, "irrelevant-new-seed").unwrap();
    assert_eq!(app, after_retry);
    let retry = engine::claim_work(&mut app, NOW + 5, "critic-remainder-lease1", &config())
        .unwrap()
        .unwrap();
    assert_eq!(retry.kind, "critic");
    assert!(
        retry.request["questions"]
            .as_object()
            .unwrap()
            .keys()
            .all(|key| key.starts_with("c0_q1_"))
    );
    assert!(retry.request["state"]["candidate"]["concepts"][0]["questions"][0].is_null());
    let mut remainder = response(&retry.request);
    // An unsolicited replacement cannot remove the previously judged veto.
    remainder["answers"]["c0_q0_prompt_leaks_answer"] = json!({"type":"noul","noul":0.0});
    engine::finish_work(
        &mut app,
        &retry,
        remainder.to_string(),
        "actual-critic-remainder".into(),
        Some(7_000),
        NOW + 6,
    )
    .unwrap();
    assert_eq!(app.jobs[&retry.id].status, "partial");
    assert_eq!(app.questions.len(), 1);
    assert_eq!(
        app.questions.values().next().unwrap().content().prompt,
        batch.concepts[0].questions[1].prompt
    );
    assert_eq!(app.jobs[&work.id], old_job);
    assert_eq!(&app.spend[..old_spend.len()], old_spend.as_slice());
    assert_eq!(
        app.jobs[&work.id].critic_json.as_deref(),
        Some(raw.as_str())
    );
    assert_eq!(
        app.jobs[&retry.id].critic_model.as_deref(),
        Some("actual-critic-remainder")
    );
    assert_eq!(
        app.spend.last().unwrap().response_model.as_deref(),
        Some("actual-critic-remainder")
    );
    let cache =
        generation::read_critic_cache(&batch, app.jobs[&retry.id].critic_cache_json.as_deref())
            .unwrap();
    assert_eq!(cache.answers["c0_q0_prompt_leaks_answer"], 0.90);
    archive(&app);
    let mut forged = app.clone();
    let mut cache = cache.clone();
    cache
        .answers
        .insert("c0_q0_prompt_leaks_answer".into(), 0.0);
    forged.jobs.get_mut(&retry.id).unwrap().critic_cache_json =
        Some(serde_json::to_string(&cache).unwrap());
    assert!(engine::validate_app(&forged).is_err());
}

#[test]
fn fully_judged_rejection_is_reused_without_new_spend_or_score_fishing() {
    let (mut app, _, work) = prepared();
    let mut rejected = response(&work.request);
    for qi in 0..2 {
        rejected["answers"][format!("c0_q{qi}_no_defensible_answer")]["noul"] = json!(0.90);
    }
    engine::finish_work(
        &mut app,
        &work,
        rejected.to_string(),
        "actual-critic".into(),
        Some(12_000),
        NOW + 3,
    )
    .unwrap();
    assert_eq!(app.jobs[&work.id].status, "failed");
    assert!(
        app.jobs[&work.id]
            .rejected
            .iter()
            .any(|reason| reason.contains("no_defensible_answer"))
    );
    let old_spend = app.spend.clone();
    let goal = app.goals.keys().next().unwrap().clone();
    act(
        &mut app,
        &format!("/goals/{goal}/retry"),
        "critic-no-fishing01",
        &[],
    );
    assert!(
        engine::claim_work(&mut app, NOW + 4, "unused-no-fishing-lease", &config())
            .unwrap()
            .is_none()
    );
    assert_eq!(app.spend, old_spend);
    assert!(app.questions.is_empty());
    assert_eq!(app.jobs[&app.goals[&goal].job_id].status, "failed");
    archive(&app);
}

#[test]
fn paid_completion_receipts_are_immutable_including_unknown_cost_and_oversize() {
    let (mut app, _, work) = prepared();
    let raw = response(&work.request).to_string();
    engine::finish_work(
        &mut app,
        &work,
        raw.clone(),
        "actual-critic".into(),
        None,
        NOW + 3,
    )
    .unwrap();
    let saved = app.clone();
    engine::finish_work(
        &mut app,
        &work,
        raw.clone(),
        "actual-critic".into(),
        None,
        NOW + 4,
    )
    .unwrap();
    assert_eq!(app, saved);
    assert!(
        engine::finish_work(
            &mut app,
            &work,
            raw.clone(),
            "changed-model".into(),
            None,
            NOW + 5
        )
        .is_err()
    );
    assert_eq!(app, saved);
    assert!(
        engine::finish_work(
            &mut app,
            &work,
            raw,
            "actual-critic".into(),
            Some(0),
            NOW + 5
        )
        .is_err()
    );
    assert_eq!(app, saved);
    let (mut app, _, work) = prepared();
    let raw = "é".repeat(generation::RESPONSE_LIMIT);
    assert_eq!(
        engine::finish_work(
            &mut app,
            &work,
            raw.clone(),
            "actual-oversize-critic".into(),
            Some(91_000),
            NOW + 3
        )
        .unwrap_err()
        .status,
        422
    );
    assert_eq!(app.jobs[&work.id].status, "failed");
    assert_eq!(
        app.jobs[&work.id].critic_json.as_ref().unwrap().len(),
        generation::RESPONSE_LIMIT
    );
    assert_eq!(app.spend.last().unwrap().cost_micros, Some(91_000));
    assert_eq!(app.spend.last().unwrap().status, "known");
    assert_eq!(
        app.spend.last().unwrap().response_hash.as_deref(),
        Some(persistence::sha256(raw.as_bytes()).as_str())
    );
    let saved = app.clone();
    engine::finish_work(
        &mut app,
        &work,
        raw,
        "actual-oversize-critic".into(),
        Some(91_000),
        NOW + 4,
    )
    .unwrap();
    assert_eq!(app, saved);
    archive(&app);
}

#[test]
fn huge_known_cost_saturates_the_allowance_and_blocks_a_later_claim() {
    let (mut app, _, work) = prepared();
    let raw = response(&work.request).to_string();
    engine::finish_work(
        &mut app,
        &work,
        raw,
        "actual-critic".into(),
        Some(u64::MAX),
        NOW + 3,
    )
    .unwrap();
    assert_eq!(app.spent(NOW + 4), u64::MAX);
    let goal = app.goals.keys().next().unwrap().clone();
    act(
        &mut app,
        &format!("/goals/{goal}/refine"),
        "huge-cost-refine001",
        &[("feedback", "Provide another useful cache example.")],
    );
    assert!(
        engine::claim_work(&mut app, NOW + 4, "blocked-huge-cost-lease", &config())
            .unwrap()
            .is_none()
    );
    assert_eq!(app.spend.len(), 2);
    archive(&app);
}

#[test]
fn photo_transcription_generation_and_critic_keep_each_exact_paid_response() {
    let (_, batch, _) = prepared();
    let bytes = STANDARD.decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j56cAAAAASUVORK5CYII=").unwrap();
    let photo = Photo {
        key: "photos/source-photo-exact".into(),
        mime: "image/png".into(),
        size: bytes.len(),
        sha256: persistence::sha256(&bytes),
    };
    let mut app = App::new("photo-outcomes-owner-csrf-32-characters".into());
    act(
        &mut app,
        "/create",
        "photo-paid-create01",
        &[
            ("intent", "Explain HTTP caching"),
            ("__photo", &serde_json::to_string(&photo).unwrap()),
        ],
    );
    let work = engine::claim_work(&mut app, NOW, "photo-transcribe-lease", &config())
        .unwrap()
        .unwrap();
    assert_eq!(work.kind, "transcribe");
    let raw = json!({"choices":[{"message":{"content":json!({"text":"An ETag identifies a representation."}).to_string()}}]}).to_string();
    engine::finish_work(
        &mut app,
        &work,
        raw.clone(),
        "actual-transcriber".into(),
        Some(13_000),
        NOW + 1,
    )
    .unwrap();
    let transcript_receipt = app.spend[0].clone();
    assert_eq!(
        transcript_receipt.raw_response.as_deref(),
        Some(raw.as_str())
    );
    assert_eq!(
        transcript_receipt.response_model.as_deref(),
        Some("actual-transcriber")
    );
    assert_eq!(transcript_receipt.response_bytes, Some(raw.len()));
    let work = engine::claim_work(&mut app, NOW + 2, "photo-generation-lease", &config())
        .unwrap()
        .unwrap();
    assert_eq!(work.kind, "generation");
    let raw = json!({"choices":[{"message":{"content":serde_json::to_string(&batch).unwrap()}}]})
        .to_string();
    engine::finish_work(
        &mut app,
        &work,
        raw.clone(),
        "actual-generator".into(),
        Some(15_000),
        NOW + 3,
    )
    .unwrap();
    assert_eq!(
        app.jobs[&work.id].raw_response.as_deref(),
        Some(raw.as_str())
    );
    assert_eq!(app.spend[0], transcript_receipt);
    let work = engine::claim_work(&mut app, NOW + 4, "photo-critic-lease01", &config())
        .unwrap()
        .unwrap();
    let raw = response(&work.request).to_string();
    engine::finish_work(
        &mut app,
        &work,
        raw,
        "actual-critic".into(),
        Some(11_000),
        NOW + 5,
    )
    .unwrap();
    assert_eq!(app.spend[0], transcript_receipt);
    assert_eq!(app.spend.len(), 3);
    engine::validate_app(&app).unwrap();
    let rows = persistence::rows(&app).unwrap();
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), app);
    let bytes = persistence::Archive::new(
        app.clone(),
        vec![persistence::ArchivedPhoto { photo, bytes }],
        NOW + 6,
    )
    .encode()
    .unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        app
    );
    let mut corrupt = app.clone();
    corrupt.spend[0].raw_response = Some("Altered transcript provider receipt.".into());
    assert!(engine::validate_app(&corrupt).is_err());
    let mut corrupt = app.clone();
    corrupt.spend[0].response_bytes = Some(0);
    assert!(engine::validate_app(&corrupt).is_err());
    let mut corrupt = app;
    corrupt.jobs.get_mut(&work.id).unwrap().critic_model = Some("unpaid-replacement-model".into());
    assert!(engine::validate_app(&corrupt).is_err());
}

#[test]
fn absent_critic_records_skipped_check_and_publishes_without_an_extra_reservation() {
    let (_, batch, _) = prepared();
    let mut app = App::new("skipped-critic-owner-csrf-32-characters".into());
    act(
        &mut app,
        "/create",
        "skipped-create-op01",
        &[("intent", "HTTP caching")],
    );
    let mut cfg = config();
    cfg.critic = false;
    let work = engine::claim_work(&mut app, NOW, "skipped-generator-lease", &cfg)
        .unwrap()
        .unwrap();
    let raw = json!({"choices":[{"message":{"content":serde_json::to_string(&batch).unwrap()}}]})
        .to_string();
    engine::finish_work(
        &mut app,
        &work,
        raw,
        "actual-generator".into(),
        Some(10_000),
        NOW + 1,
    )
    .unwrap();
    let paid = app.spend.clone();
    assert!(
        engine::claim_work(&mut app, NOW + 2, "unused-skipped-lease", &cfg)
            .unwrap()
            .is_none()
    );
    assert_eq!(app.spend, paid);
    assert_eq!(
        app.jobs[&work.id].critic_json.as_deref(),
        Some("{\"status\":\"skipped\"}")
    );
    assert_eq!(app.jobs[&work.id].status, "ready");
    assert_eq!(app.questions.len(), 2);
    archive(&app);
}

#[test]
fn rejected_http_response_keeps_paid_receipt_but_never_interprets_success_shaped_body() {
    let (mut app, batch, work) = prepared();
    let raw = response(&work.request).to_string();
    engine::fail_received_work(
        &mut app,
        &work,
        NOW + 3,
        raw.clone(),
        "actual-refused-critic".into(),
        Some(8_000),
        "The provider refused this request (HTTP429).",
    )
    .unwrap();
    assert!(app.questions.is_empty());
    assert_eq!(app.jobs[&work.id].status, "failed");
    assert!(
        generation::read_critic_cache(&batch, app.jobs[&work.id].critic_cache_json.as_deref())
            .unwrap()
            .answers
            .is_empty()
    );
    let receipt = app.spend.last().unwrap();
    assert_eq!(receipt.raw_response.as_deref(), Some(raw.as_str()));
    assert_eq!(receipt.cost_micros, Some(8_000));
    assert_eq!(receipt.response_outcome.as_deref(), Some("rejected"));
    assert_eq!(
        receipt.response_model.as_deref(),
        Some("actual-refused-critic")
    );
    archive(&app);
    let saved = app.clone();
    engine::fail_received_work(
        &mut app,
        &work,
        NOW + 4,
        raw.clone(),
        "actual-refused-critic".into(),
        Some(8_000),
        "Retrying the same refused receipt",
    )
    .unwrap();
    assert_eq!(app, saved);
    assert!(
        engine::finish_work(
            &mut app,
            &work,
            raw,
            "actual-refused-critic".into(),
            Some(8_000),
            NOW + 4
        )
        .is_err()
    );
    assert_eq!(app, saved);
    let goal = app.goals.keys().next().unwrap().clone();
    act(
        &mut app,
        &format!("/goals/{goal}/retry"),
        "http-refusal-retry01",
        &[],
    );
    let retry = engine::claim_work(&mut app, NOW + 5, "http-refusal-new-lease", &config())
        .unwrap()
        .unwrap();
    assert_eq!(retry.request["questions"], work.request["questions"]);
}

#[test]
fn a_late_parent_response_cannot_change_a_retry_snapshot_or_break_archive_integrity() {
    let (mut app, batch, work) = prepared();
    engine::fail_work(
        &mut app,
        &work,
        "The transmitted outcome is unknown.",
        true,
        NOW + 3,
    )
    .unwrap();
    let goal = app.goals.keys().next().unwrap().clone();
    act(
        &mut app,
        &format!("/goals/{goal}/retry"),
        "late-parent-retry01",
        &[],
    );
    let retry = engine::claim_work(&mut app, NOW + 4, "late-parent-child-lease", &config())
        .unwrap()
        .unwrap();
    assert!(app.jobs[&retry.id].critic_inherited_keys.is_empty());
    let raw = response(&work.request).to_string();
    assert_eq!(
        engine::finish_work(
            &mut app,
            &work,
            raw,
            "actual-late-parent".into(),
            Some(9_000),
            NOW + 5
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app.jobs[&work.id].status, "superseded");
    assert!(
        !generation::read_critic_cache(&batch, app.jobs[&work.id].critic_cache_json.as_deref())
            .unwrap()
            .answers
            .is_empty()
    );
    assert!(
        generation::read_critic_cache(&batch, app.jobs[&retry.id].critic_cache_json.as_deref())
            .unwrap()
            .answers
            .is_empty()
    );
    assert!(app.questions.is_empty());
    archive(&app);
    let parent = app.jobs[&work.id].clone();
    let mut raw = response(&retry.request);
    raw["answers"]["c0_q0_prompt_leaks_answer"]["noul"] = json!(0.90);
    engine::finish_work(
        &mut app,
        &retry,
        raw.to_string(),
        "actual-child-critic".into(),
        Some(8_000),
        NOW + 6,
    )
    .unwrap();
    assert_eq!(app.questions.len(), 1);
    assert_eq!(app.jobs[&work.id], parent);
    assert_eq!(
        generation::read_critic_cache(&batch, app.jobs[&retry.id].critic_cache_json.as_deref())
            .unwrap()
            .answers["c0_q0_prompt_leaks_answer"],
        0.90
    );
    archive(&app);
}

#[test]
fn a_rejected_success_shaped_assessment_stays_ungraded_and_preserves_the_saved_answer() {
    let mut app = App::new("rejected-assessment-csrf-32-characters".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    for q in app
        .questions
        .values_mut()
        .filter(|q| q.content().kind == "choice")
    {
        q.card.due_ms = NOW + DAY_MS;
    }
    app.occurrence = None;
    engine::ensure_occurrence(&mut app, NOW, "refused-assessment-cold").unwrap();
    let occurrence = app.occurrence.as_ref().unwrap().id.clone();
    act(
        &mut app,
        "/review/answer",
        "refused-answer-op001",
        &[
            ("occurrence_id", &occurrence),
            ("answer", "a nonexact synthetic answer"),
        ],
    );
    let question = app.questions[&app.occurrence.as_ref().unwrap().question_id].clone();
    let work = engine::claim_work(&mut app, NOW + 1, "refused-assessment-lease", &config())
        .unwrap()
        .unwrap();
    assert_eq!(work.kind, "assessment");
    let raw = json!({"answers":{"verdict":{"choice":"accept","probabilities":{"accept":0.99,"reject":0.005,"unsure":0.005}},"identity":{"noul":0.0},"injection":{"noul":0.0}}}).to_string();
    engine::fail_received_work(
        &mut app,
        &work,
        NOW + 2,
        raw.clone(),
        "actual-refused-jev".into(),
        None,
        "The provider refused this check (HTTP429).",
    )
    .unwrap();
    assert!(app.events.is_empty());
    assert_eq!(app.questions[&question.id], question);
    let occurrence = app.occurrence.as_ref().unwrap();
    assert_eq!(occurrence.phase, "self-check");
    assert_eq!(
        occurrence.answer.as_deref(),
        Some("a nonexact synthetic answer")
    );
    assert!(!occurrence.assisted);
    assert_eq!(app.assessments[&work.id].status, "failed");
    assert_eq!(
        app.assessments[&work.id].raw_response.as_deref(),
        Some(raw.as_str())
    );
    assert_eq!(
        app.spend.last().unwrap().response_outcome.as_deref(),
        Some("rejected")
    );
    assert_eq!(app.spend.last().unwrap().cost_micros, None);
    archive(&app);
}
