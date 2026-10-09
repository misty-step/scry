use scry::{engine, model::App, web};
use std::collections::{BTreeMap, BTreeSet};

const NOW: i64 = 1_800_000_000_000;
fn query(nonce: &str) -> BTreeMap<String, String> {
    BTreeMap::from([("__nonce".into(), nonce.into())])
}
fn fixture() -> App {
    let mut app = App::new("01234567890123456789012345678901".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    app
}

#[test]
fn answer_and_related_reference_are_absent_before_assistance() {
    let mut app = fixture();
    let occurrence = app.occurrence.as_mut().unwrap();
    occurrence.phase = "question".into();
    occurrence.assisted = false;
    occurrence.presentation.kind = "recall".into();
    occurrence.presentation.answer = "PRIVATE_EXPECTED_ANSWER".into();
    occurrence.presentation.explanation = "PRIVATE_EXPLANATION".into();
    let concept_id = occurrence.concept_id.clone();
    let question_id = occurrence.question_id.clone();
    let concept = app.concepts.get_mut(&concept_id).unwrap();
    concept.name = "PRIVATE_CONCEPT_NAME".into();
    concept.summary = "PRIVATE_CONCEPT_SUMMARY".into();
    concept.notes[0].text = "PRIVATE_NOTE".into();
    let goal_id = concept.goal_id.clone();
    let goal = app.goals.get_mut(&goal_id).unwrap();
    goal.title = "PRIVATE_GOAL_TITLE".into();
    goal.intent = "PRIVATE_GOAL_INTENT".into();
    let current = app
        .questions
        .get_mut(&question_id)
        .unwrap()
        .versions
        .last_mut()
        .unwrap();
    current.answer = "PRIVATE_EXPECTED_ANSWER".into();
    current.explanation = "PRIVATE_EXPLANATION".into();
    for path in [
        "/".to_string(),
        "/map".into(),
        "/history".into(),
        "/settings".into(),
        format!("/goals/{goal_id}"),
        format!("/concepts/{concept_id}"),
        format!("/questions/{question_id}/edit"),
    ] {
        let html = web::render(&app, &path, &query("nonce-one"), NOW, None);
        for secret in [
            "PRIVATE_EXPECTED_ANSWER",
            "PRIVATE_EXPLANATION",
            "PRIVATE_CONCEPT_NAME",
            "PRIVATE_CONCEPT_SUMMARY",
            "PRIVATE_NOTE",
            "PRIVATE_GOAL_TITLE",
            "PRIVATE_GOAL_INTENT",
        ] {
            assert!(!html.contains(secret), "{secret} leaked on {path}");
        }
        assert!(!html.contains("style=\""));
        assert!(!html.contains("<script>"));
    }
    app.occurrence.as_mut().unwrap().assisted = true;
    let page = web::render(
        &app,
        &format!("/concepts/{concept_id}"),
        &query("nonce-two"),
        NOW,
        None,
    );
    assert!(page.contains("PRIVATE_NOTE"));
    assert!(page.contains("PRIVATE_EXPECTED_ANSWER"));
}

#[test]
fn pending_check_keeps_reference_private_without_offering_an_invalid_help_mutation() {
    let mut app = fixture();
    let initial = app.occurrence.as_ref().unwrap().clone();
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "history-before-pending-check".into()),
        ("occurrence_id".into(), initial.id),
        ("answer".into(), initial.presentation.answer),
    ]);
    engine::mutate(&mut app, "/review/answer", &fields, NOW, "history").unwrap();
    let historical = app.events.last_mut().unwrap();
    historical.presentation.prompt = "PENDING_PRIVATE_HISTORY_PROMPT".into();
    historical.presentation.answer = "PENDING_PRIVATE_ANSWER".into();
    historical.presentation.explanation = "PENDING_PRIVATE_EXPLANATION".into();
    historical.answer = "PENDING_PRIVATE_ANSWER".into();
    let occurrence = app.occurrence.as_mut().unwrap();
    occurrence.phase = "checking".into();
    occurrence.assisted = false;
    occurrence.presentation.answer = "PENDING_PRIVATE_ANSWER".into();
    occurrence.presentation.explanation = "PENDING_PRIVATE_EXPLANATION".into();
    let concept_id = occurrence.concept_id.clone();
    let question_id = occurrence.question_id.clone();
    let concept = app.concepts.get_mut(&concept_id).unwrap();
    concept.name = "PENDING_PRIVATE_CONCEPT".into();
    concept.summary = "PENDING_PRIVATE_SUMMARY".into();
    concept.notes[0].text = "PENDING_PRIVATE_NOTE".into();
    let goal_id = concept.goal_id.clone();
    let goal = app.goals.get_mut(&goal_id).unwrap();
    goal.title = "PENDING_PRIVATE_GOAL".into();
    goal.intent = "PENDING_PRIVATE_INTENT".into();
    for path in [
        "/".to_string(),
        "/map".into(),
        "/history".into(),
        "/settings".into(),
        format!("/goals/{goal_id}"),
        format!("/concepts/{concept_id}"),
        format!("/questions/{question_id}/edit"),
    ] {
        let html = web::render(&app, &path, &query("checking"), NOW, None);
        for secret in [
            "PENDING_PRIVATE_ANSWER",
            "PENDING_PRIVATE_EXPLANATION",
            "PENDING_PRIVATE_CONCEPT",
            "PENDING_PRIVATE_SUMMARY",
            "PENDING_PRIVATE_NOTE",
            "PENDING_PRIVATE_GOAL",
            "PENDING_PRIVATE_INTENT",
            "PENDING_PRIVATE_HISTORY_PROMPT",
        ] {
            assert!(
                !html.contains(secret),
                "{secret} leaked while checking on {path}"
            );
        }
        if path.starts_with("/goals/")
            || path.starts_with("/concepts/")
            || path.starts_with("/questions/")
        {
            assert!(html.contains("Return to your saved answer"));
            assert!(!html.contains("action=\"/review/help\""));
            assert!(!html.contains("Look it up"));
        }
        if path == "/history" {
            assert!(html.contains("An earlier attempt is hidden"));
        }
    }
}

#[test]
fn author_and_model_text_is_escaped_in_every_context() {
    let mut app = fixture();
    app.occurrence.as_mut().unwrap().presentation.prompt =
        "<img src=x onerror=alert(1)> & \"test\"".into();
    let html = web::render(
        &app,
        "/",
        &query("nonce"),
        NOW,
        Some("<script>private()</script>"),
    );
    assert!(html.contains("&lt;img src=x onerror=alert(1)&gt; &amp; &quot;test&quot;"));
    assert!(!html.contains("<img src=x"));
    assert!(!html.contains("<script>private"));
    assert!(html.contains("&lt;script&gt;private()&lt;/script&gt;"));
}

#[test]
fn every_form_has_a_unique_nonce_bound_operation_and_native_fallback() {
    let app = fixture();
    let first = web::render(&app, "/", &query("first-request"), NOW, None);
    let second = web::render(&app, "/", &query("second-request"), NOW, None);
    let ids = |html: &str| {
        html.split("name=\"operation_id\" value=\"")
            .skip(1)
            .map(|part| part.split('"').next().unwrap().to_string())
            .collect::<Vec<_>>()
    };
    let first_ids = ids(&first);
    let second_ids = ids(&second);
    assert!(!first_ids.is_empty());
    assert_eq!(
        first_ids.len(),
        first_ids.iter().collect::<BTreeSet<_>>().len()
    );
    assert!(first_ids.iter().all(|id| !second_ids.contains(id)));
    assert_eq!(
        first.matches("<form ").count(),
        first.matches("method=\"post\"").count()
    );
    assert_eq!(
        first.matches("<form ").count(),
        first.matches("name=\"csrf\"").count()
    );
    assert!(first.contains("form method=\"post\" action=\"/review/answer\""));
    assert_eq!(first.matches("<h1").count(), 1);
}

#[test]
fn synthetic_fixture_is_never_advertised_in_the_private_application() {
    let app = App::new("01234567890123456789012345678901".into());
    let private = web::render(&app, "/", &query("private"), NOW, None);
    assert!(!private.contains("/__fixture"));
    assert!(!private.contains("Synthetic notebook"));
    let mut synthetic_query = query("synthetic");
    synthetic_query.insert("__synthetic".into(), "true".into());
    let synthetic = web::render(&app, "/", &synthetic_query, NOW, None);
    assert!(synthetic.contains("/__fixture"));
    assert!(synthetic.contains("Synthetic notebook"));
}

#[test]
fn edits_carry_content_identity_and_preserve_rejected_drafts() {
    let mut app = fixture();
    app.occurrence.as_mut().unwrap().assisted = true;
    let question_id = app.occurrence.as_ref().unwrap().question_id.clone();
    let mut fields = query("edit");
    fields.insert("prompt".into(), "A corrected question <draft>".into());
    let html = web::render(
        &app,
        &format!("/questions/{question_id}/edit"),
        &fields,
        NOW,
        None,
    );
    assert!(html.contains("A corrected question &lt;draft&gt;"));
    for field in ["content_version", "kind", "basis", "quotes", "use_draft"] {
        assert!(
            html.contains(&format!("name=\"{field}\"")),
            "Missing edit contract field {field}"
        );
    }
}

#[test]
fn held_result_names_authority_and_requires_deliberate_next() {
    let mut app = fixture();
    let o = app.occurrence.as_ref().unwrap().clone();
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "web-result-answer-operation".into()),
        ("occurrence_id".into(), o.id),
        ("answer".into(), o.presentation.answer),
    ]);
    engine::mutate(&mut app, "/review/answer", &fields, NOW, "result").unwrap();
    let html = web::render(&app, "/", &query("result"), NOW, None);
    assert!(html.contains("data-state=\"result\""));
    assert!(html.contains("data-next"));
    assert!(html.contains("Matched the saved answer"));
    assert!(html.contains("The answer"));
    assert!(!html.contains("http-equiv=\"refresh\""));
    assert!(web::asset("/assets/fonts/Literata.woff2").is_some());
    assert!(web::asset("/assets/../../.env").is_none());
}

#[test]
fn assisted_results_do_not_offer_automatic_grade_overrides() {
    let mut app = fixture();
    app.occurrence.as_mut().unwrap().assisted = true;
    let o = app.occurrence.as_ref().unwrap().clone();
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        (
            "operation_id".into(),
            "web-assisted-answer-operation".into(),
        ),
        ("occurrence_id".into(), o.id),
        ("answer".into(), o.presentation.answer),
    ]);
    engine::mutate(&mut app, "/review/answer", &fields, NOW, "assisted").unwrap();
    let html = web::render(&app, "/", &query("assisted"), NOW, None);
    assert!(html.contains("recorded as helped practice"));
    assert!(!html.contains("/review/override"));
    assert!(!html.contains("Count as a miss"));
}

#[test]
fn repair_dispute_form_fences_a_deliberate_schedule_reset() {
    let app = fixture();
    let html = web::render(&app, "/", &query("dispute"), NOW, None);
    let form = html
        .split("/dispute\"")
        .nth(1)
        .unwrap()
        .split("</form>")
        .next()
        .unwrap();
    assert!(form.contains("name=\"content_version\""));
    assert!(form.contains("name=\"schedule_version\""));
    assert!(form.contains("name=\"reset\" value=\"true\""));
    assert!(form.contains("name=\"text\""));
    assert!(!form.contains(" checked"));
}
