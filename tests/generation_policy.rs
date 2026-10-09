use scry::generation::{self, Batch, GeneratedConcept, GeneratedQuestion};
use serde_json::{Value, json};
use std::collections::BTreeMap;

fn question(index: usize) -> GeneratedQuestion {
    GeneratedQuestion {
        prompt: format!("Which HTTP header identifies a representation in example {index}?"),
        kind: "recall".into(),
        answer: "ETag".into(),
        variants: vec![],
        choices: vec![],
        explanation: "The header is an opaque identifier; a conditional request compares it to the current representation.".into(),
        basis: "general".into(),
        quotes: vec![],
        required_ideas: vec![],
        contradictions: vec![],
    }
}

fn concept(index: usize, count: usize) -> GeneratedConcept {
    GeneratedConcept {
        key: format!("cache{index}"),
        name: format!("HTTP validators {index}"),
        summary: "A validator lets a cache check whether its stored response is current.".into(),
        note: "A cache can send its saved validator in a conditional request. The server can answer that the representation is unchanged, avoiding retransmission.".into(),
        basis: "general".into(),
        quotes: vec![],
        prerequisites: vec![],
        questions: (0..count).map(|i|question(index*10+i)).collect(),
    }
}

fn batch() -> Batch {
    Batch {
        title: "Understanding HTTP caching".into(),
        complete: true,
        concepts: vec![concept(0, 3)],
    }
}

fn critic_response(batch: &Batch) -> Value {
    let request = generation::critic_request(batch, "synthetic-test");
    let answers: serde_json::Map<String, Value> = request["questions"]
        .as_object()
        .unwrap()
        .keys()
        .map(|key| (key.clone(), json!({"noul":0.01,"type":"noul"})))
        .collect();
    json!({"answers":answers})
}

#[test]
fn evidence_quotes_must_match_saved_input_exactly_and_general_claims_have_no_quotes() {
    let source = "An ETag identifies a representation.";
    let mut sourced = batch();
    sourced.concepts[0].basis = "source".into();
    sourced.concepts[0].quotes = vec![source.into()];
    for question in &mut sourced.concepts[0].questions {
        question.basis = "source".into();
        question.quotes = vec![source.into()];
    }
    assert!(generation::validate_batch(sourced.clone(), source).is_ok());
    let mut invented = sourced.clone();
    invented.concepts[0].quotes[0] = "An ETag identifies any and all representations.".into();
    assert!(generation::validate_batch(invented, source).is_err());
    let mut wrong_question = sourced.clone();
    wrong_question.concepts[0].questions[0].quotes[0] =
        "An etag identifies a representation.".into();
    let (partial, rejected) = generation::validate_batch(wrong_question, source).unwrap();
    assert_eq!(partial.concepts[0].questions.len(), 2);
    assert!(!partial.complete);
    assert!(!rejected.is_empty());
    let mut false_provenance = question(0);
    false_provenance.quotes = vec![source.into()];
    assert!(generation::validate_question(&false_provenance, source).is_err());
    false_provenance.basis = "source".into();
    assert!(generation::validate_question(&false_provenance, source).is_ok());
    false_provenance.quotes.clear();
    assert!(generation::validate_question(&false_provenance, source).is_err());
}

#[test]
fn normal_partial_output_is_honest_but_oversized_and_cyclic_output_fails() {
    let mut partial = batch();
    partial.concepts[0].questions[0].prompt = "The answer is ETag, which header is this?".into();
    let (valid, rejected) = generation::validate_batch(partial, "HTTP caching").unwrap();
    assert_eq!(valid.concepts[0].questions.len(), 2);
    assert!(!valid.complete);
    assert!(!rejected.is_empty());
    let large = Batch {
        concepts: (0..8).map(|i| concept(i, 8)).collect(),
        ..batch()
    };
    let error = generation::validate_batch(large.clone(), "HTTP caching").unwrap_err();
    assert!(error.message.contains("60-question"));
    assert!(generation::apply_critic(large, "{}").is_err());
    let mut cyclic = Batch {
        concepts: vec![concept(0, 1), concept(1, 1)],
        ..batch()
    };
    cyclic.concepts[0].prerequisites = vec!["cache1".into()];
    cyclic.concepts[1].prerequisites = vec!["cache0".into()];
    assert!(
        generation::validate_batch(cyclic, "HTTP caching")
            .unwrap_err()
            .message
            .contains("circular")
    );
}

#[test]
fn complete_and_verbatim_requests_never_publish_partial_material() {
    for intent in [
        "Prepare the complete set",
        "Give every item",
        "Teach exact wording verbatim",
    ] {
        let mut incomplete = batch();
        incomplete.complete = false;
        assert!(generation::validate_batch(incomplete, intent).is_err());
        let mut invalid = batch();
        invalid.concepts[0].questions[0].answer = "".into();
        assert!(generation::validate_batch(invalid, intent).is_err());
        let original = batch();
        let mut response = critic_response(&original);
        response["answers"]["c0_q0_prompt_leaks_answer"]["noul"] = json!(0.80);
        let (critic_partial, _) =
            generation::apply_critic(original, &response.to_string()).unwrap();
        assert!(generation::validate_batch(critic_partial, intent).is_err());
    }
}

#[test]
fn critic_uses_independent_applicable_defects_and_teaching_only_ranks() {
    let mut candidate = batch();
    candidate.concepts[0].questions[1].kind = "choice".into();
    candidate.concepts[0].questions[1].choices = vec!["ETag".into(), "Date".into(), "Via".into()];
    candidate.concepts[0].questions[1].basis = "source".into();
    candidate.concepts[0].questions[1].quotes = vec!["An ETag identifies a representation.".into()];
    candidate.concepts[0].questions[2].kind = "explain".into();
    candidate.concepts[0].questions[2].required_ideas =
        vec!["Conditional requests compare the validator to the current representation.".into()];
    let request = generation::critic_request(&candidate, "synthetic-test");
    let keys = request["questions"].as_object().unwrap();
    assert_eq!(keys.len(), 28);
    assert!(keys.contains_key("c0_q0_no_defensible_answer"));
    assert!(!keys.contains_key("c0_q0_unsupported_by_evidence"));
    assert!(!keys.contains_key("c0_q0_mcq_options_overlap"));
    assert!(keys.contains_key("c0_q1_unsupported_by_evidence"));
    assert!(keys.contains_key("c0_q1_mcq_options_overlap"));
    assert!(keys.contains_key("c0_q2_rubric_misaligned"));
    let mut response = critic_response(&candidate);
    response["answers"]["c0_q0_explanation_restates_without_teaching"]["noul"] = json!(1.0);
    let (accepted, rejected) =
        generation::apply_critic(candidate.clone(), &response.to_string()).unwrap();
    assert_eq!(accepted.concepts[0].questions.len(), 3);
    assert!(rejected.is_empty());
    response["answers"]["c0_q1_unsupported_by_evidence"]["noul"] = json!(0.80);
    let (partial, rejected) = generation::apply_critic(candidate, &response.to_string()).unwrap();
    assert_eq!(partial.concepts[0].questions.len(), 2);
    assert!(!partial.complete);
    assert!(
        rejected
            .iter()
            .any(|reason| reason.contains("unsupported_by_evidence"))
    );
}

#[test]
fn critic_missing_extra_mixed_or_invalid_judgments_cannot_publish() {
    let candidate = batch();
    let key = "c0_q0_no_defensible_answer";
    let mut missing = critic_response(&candidate);
    missing["answers"].as_object_mut().unwrap().remove(key);
    assert!(generation::apply_critic(candidate.clone(), &missing.to_string()).is_err());
    let mut extra = critic_response(&candidate);
    extra["answers"]["unexpected"] = json!({"noul":0.01});
    assert!(generation::apply_critic(candidate.clone(), &extra.to_string()).is_err());
    for judgment in [
        json!({"noul":0.01,"choice":"accept"}),
        json!({"noul":0.01,"type":"choice"}),
        json!({"noul":0.01,"score":0.9}),
        json!({"noul":0.01,"probabilities":{"accept":0.9}}),
        json!({"noul":1.01}),
        json!({"noul":null}),
    ] {
        let mut response = critic_response(&candidate);
        response["answers"][key] = judgment;
        assert!(generation::apply_critic(candidate.clone(), &response.to_string()).is_err());
    }
    let mut notes_rejected = critic_response(&candidate);
    notes_rejected["answers"]["c0_note"]["noul"] = json!(0.80);
    assert!(generation::apply_critic(candidate, &notes_rejected.to_string()).is_err());
}

#[test]
fn refinement_keeps_existing_prerequisites_and_rejects_combined_cycles() {
    let existing = BTreeMap::from([("saved-idea".into(), Vec::new())]);
    let mut candidate = batch();
    candidate.concepts[0].prerequisites = vec!["saved-idea".into()];
    let (valid, _) =
        generation::validate_batch_with_existing(candidate.clone(), "HTTP caching", &existing)
            .unwrap();
    assert_eq!(valid.concepts[0].prerequisites, ["saved-idea"]);
    assert!(generation::validate_batch(candidate.clone(), "HTTP caching").is_err());
    let cyclic_existing = BTreeMap::from([("saved-idea".into(), vec!["cache0".into()])]);
    assert!(
        generation::validate_batch_with_existing(candidate, "HTTP caching", &cyclic_existing)
            .is_err()
    );
    let mut self_cycle = batch();
    self_cycle.concepts[0].prerequisites = vec!["cache0".into()];
    assert!(generation::validate_batch(self_cycle, "HTTP caching").is_err());
    let mut update = batch();
    update.concepts[0].key = "saved-idea".into();
    update.concepts[0].prerequisites = vec![];
    assert!(generation::validate_batch_with_existing(update, "HTTP caching", &existing).is_ok());
}

#[test]
fn provider_requests_use_string_instructions_and_supported_decision_heads() {
    let mut app = scry::model::App::new("request-shape-owner-csrf-32-characters".into());
    scry::engine::seed_fixture(&mut app, 1_791_410_400_000).unwrap();
    let mut occurrence = app.occurrence.unwrap();
    occurrence.presentation.kind = "recall".into();
    let short = generation::assessment_request(&occurrence, "the saved tag", "synthetic-jev");
    occurrence.presentation.kind = "explain".into();
    occurrence.presentation.required_ideas =
        vec!["Validators preserve \"opaque\" identity.\nNever infer meaning.".into()];
    occurrence.presentation.contradictions =
        vec!["A tag always specifies a modification date.".into()];
    let explain =
        generation::assessment_request(&occurrence, "the server compares it", "synthetic-jev");
    assert!(
        explain["questions"]["idea_0"]["instructions"]
            .as_str()
            .unwrap()
            .contains(&serde_json::to_string(&occurrence.presentation.required_ideas[0]).unwrap())
    );
    for request in [
        short,
        explain,
        generation::critic_request(&batch(), "synthetic-jev"),
    ] {
        for question in request["questions"].as_object().unwrap().values() {
            assert!(question["instructions"].is_string());
            let criteria = question["criteria"].as_object().unwrap();
            match question["type"].as_str().unwrap() {
                "noul" => {
                    assert_eq!(criteria.len(), 2);
                    assert!(criteria.contains_key("true") && criteria.contains_key("false"));
                }
                "choice" => assert!(criteria.len() >= 2),
                other => panic!("unsupported decision head {other}"),
            }
        }
    }
}

#[test]
fn critic_cache_reuses_only_whole_batteries_and_never_overwrites_a_saved_veto() {
    let candidate = batch();
    let empty = generation::read_critic_cache(&candidate, None).unwrap();
    let mut response = critic_response(&candidate);
    response["answers"]["c0_q0_prompt_leaks_answer"]["noul"] = json!(0.90);
    response["answers"]
        .as_object_mut()
        .unwrap()
        .remove("c0_q1_no_defensible_answer");
    response["answers"]["c0_q2_adversarial_content"] = json!({"type":"choice","noul":0.01});
    response["answers"]["unrequested"] = json!({"noul":0.01});
    let cache = generation::merge_critic_cache(&candidate, &empty, &response.to_string()).unwrap();
    assert!(cache.answers.contains_key("c0_note"));
    assert_eq!(cache.answers["c0_q0_prompt_leaks_answer"], 0.90);
    assert!(
        !cache
            .answers
            .keys()
            .any(|key| key.starts_with("c0_q1_") || key.starts_with("c0_q2_"))
    );
    assert!(generation::apply_critic_cache(candidate.clone(), &cache).is_err());
    let request =
        generation::critic_request_remaining(&candidate, "synthetic-jev", &cache).unwrap();
    assert!(
        request["questions"]
            .as_object()
            .unwrap()
            .keys()
            .all(|key| key.starts_with("c0_q1_") || key.starts_with("c0_q2_"))
    );
    assert!(request["state"]["candidate"]["concepts"][0]["questions"][0].is_null());
    assert!(request["state"]["candidate"]["concepts"][0]["note"].is_null());
    let replacement = critic_response(&candidate);
    let merged =
        generation::merge_critic_cache(&candidate, &cache, &replacement.to_string()).unwrap();
    assert_eq!(merged.answers["c0_q0_prompt_leaks_answer"], 0.90);
    let (accepted, rejected) = generation::apply_critic_cache(candidate.clone(), &merged).unwrap();
    assert_eq!(accepted.concepts[0].questions.len(), 2);
    assert!(
        rejected
            .iter()
            .any(|reason| reason.contains("prompt_leaks_answer"))
    );
    let mut wrong_candidate = candidate.clone();
    wrong_candidate.concepts[0].questions[0].answer = "Date".into();
    assert!(generation::validate_critic_cache(&wrong_candidate, &merged).is_err());
    let mut partial_cache = merged;
    partial_cache.answers.remove("c0_q0_no_defensible_answer");
    assert!(generation::validate_critic_cache(&candidate, &partial_cache).is_err());
}
