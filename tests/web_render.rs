use scry::{
    engine,
    learning::new_card,
    model::{App, Concept, Content, Goal, Job, Note, Occurrence, Question},
    web,
};
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
/// Text a reader can see: markup, attributes and hidden fields removed.
fn visible(html: &str) -> String {
    let mut text = String::new();
    let mut in_tag = false;
    for ch in html.chars() {
        match ch {
            '<' => in_tag = true,
            '>' => {
                in_tag = false;
                text.push(' ');
            }
            _ if !in_tag => text.push(ch),
            _ => {}
        }
    }
    text
}

const QUIZ: &str = "d02c4e9a7f1b3d5e8a6c0b2f4e1d9a7c";
const LEGACY_GOAL: &str = "legacy-import-goal-6b2e81f0c94a4d7e9a3155c2d8e0f417";
const LEGACY_PROMPT: &str = "Which two rivers meet at Sacramento?";
const LEGACY_GOAL_TITLE: &str = "Sacramento history and the 1862 flood";

/// The revival projection's shapes (revival/project.mjs): an imported quiz
/// without an idea of its own, and imported requests that never finished.
fn revived() -> App {
    let mut app = fixture();
    let concept = format!("legacy-import-quiz-concept-{QUIZ}");
    let question = format!("legacy-import-quiz-{QUIZ}");
    let content = Content {
        version: 1,
        prompt: LEGACY_PROMPT.into(),
        kind: "recall".into(),
        answer: "The Sacramento and the American".into(),
        variants: vec![],
        choices: vec![],
        explanation: "The American River joins the Sacramento just north of downtown.".into(),
        basis: "general".into(),
        quotes: vec![],
        required_ideas: vec![],
        contradictions: vec![],
        model: "google/gemini-3.7-flash".into(),
        created_ms: NOW - 1000,
    };
    let goal = |id: &str, title: &str, status: &str, concepts: Vec<String>, job: &str| Goal {
        id: id.into(),
        title: title.into(),
        intent: title.into(),
        created_ms: NOW - 1000,
        revision: 1,
        status: status.into(),
        paused: false,
        focused: false,
        archived: false,
        concept_ids: concepts,
        job_id: job.into(),
        photo: None,
        transcript: None,
    };
    let job = |id: &str, goal: &str, status: &str, error: Option<&str>| Job {
        id: id.into(),
        goal_id: goal.into(),
        question_id: None,
        question_version: None,
        source_revision: 1,
        kind: "create".into(),
        status: status.into(),
        created_ms: NOW - 1000,
        lease: None,
        started_ms: None,
        attempt: 1,
        error: error.map(String::from),
        raw_response: None,
        model: Some("legacy-import".into()),
        feedback_ids: vec![],
        rejected: vec![],
        candidate_json: None,
        critic_json: None,
        critic_cache_json: None,
        critic_model: None,
        critic_parent_id: None,
        critic_inherited_keys: vec![],
    };
    let ready_job = "legacy-import-job-5f0e9d8c7b6a49382716a5b4c3d2e1f0";
    app.goals.insert(
        LEGACY_GOAL.into(),
        goal(
            LEGACY_GOAL,
            LEGACY_GOAL_TITLE,
            "ready",
            vec![concept.clone()],
            ready_job,
        ),
    );
    app.jobs
        .insert(ready_job.into(), job(ready_job, LEGACY_GOAL, "ready", None));
    app.concepts.insert(
        concept.clone(),
        Concept {
            id: concept.clone(),
            goal_id: LEGACY_GOAL.into(),
            name: format!("Legacy import — quiz {QUIZ}"),
            summary: "Legacy import: reference assembled mechanically from the original quiz, not new learning evidence.".into(),
            notes: vec![Note {
                text: format!(
                    "Legacy import\n\n{}\n\n{}\n\n{}",
                    content.prompt, content.answer, content.explanation
                ),
                basis: "general".into(),
                quotes: vec![],
                created_ms: NOW - 1000,
                model: "legacy-import".into(),
            }],
            prerequisites: vec![],
            introduced_ms: None,
            exposure_ms: None,
            already_known: false,
            archived: false,
        },
    );
    app.questions.insert(
        question.clone(),
        Question::new(question, LEGACY_GOAL.into(), concept, content, NOW - 1000),
    );
    // Original preparations that never produced material: paused, canceled
    // (superseded), failed, partial and completed-without-questions.
    for (n, status) in ["paused", "superseded", "failed", "ready", "partial"]
        .iter()
        .enumerate()
    {
        let gid = format!("legacy-import-goal-0000000000000000000000000000000{n}");
        let jid = format!("legacy-import-job-0000000000000000000000000000000{n}");
        app.goals.insert(
            gid.clone(),
            goal(
                &gid,
                &format!("Unfinished request {n}"),
                "paused",
                vec![],
                &jid,
            ),
        );
        app.jobs.insert(
            jid.clone(),
            job(
                &jid,
                &gid,
                status,
                Some("legacy-import: historical work is paused; original receipts remain in the Go archive."),
            ),
        );
    }
    app
}
fn legacy_occurrence(app: &App, phase: &str) -> Occurrence {
    let question = &app.questions[&format!("legacy-import-quiz-{QUIZ}")];
    Occurrence {
        id: "legacy-occurrence".into(),
        question_id: question.id.clone(),
        concept_id: question.concept_id.clone(),
        presentation: question.content().clone(),
        phase: phase.into(),
        assisted: false,
        before_card: new_card(NOW),
        schedule_version: 0,
        created_ms: NOW,
        answer: None,
        assessment_id: None,
        result: None,
    }
}

#[test]
fn imported_material_shows_its_own_words_not_import_labels_or_identifiers() {
    let mut app = revived();
    let concept = format!("legacy-import-quiz-concept-{QUIZ}");
    let routes = [
        "/".to_string(),
        "/map".into(),
        "/history".into(),
        "/settings".into(),
        format!("/goals/{LEGACY_GOAL}"),
        format!("/concepts/{concept}"),
        "/goals/legacy-import-goal-00000000000000000000000000000000".into(),
        "/goals/legacy-import-goal-00000000000000000000000000000002".into(),
    ];
    let check = |app: &App, path: &str| {
        let text = visible(&web::render(app, path, &query("legacy"), NOW, None));
        assert!(
            !text.to_lowercase().contains("legacy"),
            "import label on {path}"
        );
        assert!(!text.contains(QUIZ), "quiz identifier on {path}");
        assert!(!text.contains("00000000000"), "identifier on {path}");
        assert!(
            !text.contains("Preparation is in progress"),
            "stopped import shown as in progress on {path}"
        );
        text
    };
    for path in &routes {
        check(&app, path);
    }
    let map = check(&app, "/map");
    assert!(map.contains(LEGACY_PROMPT));
    assert!(map.contains("Preparation paused"));
    let reference = check(&app, &format!("/concepts/{concept}"));
    assert_eq!(reference.matches(LEGACY_PROMPT).count(), 2); // title + practice list
    assert!(reference.contains("The Sacramento and the American"));
    assert!(reference.contains("joins the Sacramento just north of downtown"));
    let stopped = check(
        &app,
        "/goals/legacy-import-goal-00000000000000000000000000000002",
    );
    assert!(stopped.contains("came from your earlier notebook"));
    // Every view of a stopped import agrees that nothing is being prepared.
    for n in 0..5 {
        let page = check(
            &app,
            &format!("/goals/legacy-import-goal-0000000000000000000000000000000{n}"),
        );
        assert!(page.contains("This request didn't finish."), "goal {n}");
        for claim in [
            "taking shape",
            "Scry is preparing",
            "A useful part is ready",
        ] {
            assert!(!page.contains(claim), "goal {n} claims {claim}");
        }
    }
    let home = check(&app, "/");
    for words in [
        "Preparation is paused",
        "Preparation stopped",
        "Preparation needs your attention",
        "No practice was prepared",
    ] {
        assert!(home.contains(words), "{words} missing from receipts");
    }
    assert!(!home.contains("Some material is ready"));
    let mut idle = app.clone();
    idle.occurrence = None;
    assert!(!check(&idle, "/").contains("taking shape"));

    // The new-idea introduction names the idea by its question, once.
    app.occurrence = Some(legacy_occurrence(&app, "intro"));
    let intro = check(&app, "/");
    assert_eq!(intro.matches(LEGACY_PROMPT).count(), 1);
    assert!(intro.contains("The Sacramento and the American"));

    // Above its held result, the chip names and links the goal instead of
    // repeating the question.
    let mut result = legacy_occurrence(&app, "result");
    result.answer = Some("Sacramento and American".into());
    result.result = Some(scry::model::ReviewResult {
        event_id: "legacy-result".into(),
        outcome: "correct".into(),
        authority: "jev".into(),
        rating: 3,
        reviewed_ms: NOW,
        due_ms: NOW + 86_400_000,
        corrected: false,
    });
    app.occurrence = Some(result);
    let html = web::render(&app, "/", &query("result"), NOW, None);
    let held = check(&app, "/");
    assert!(html.contains(&format!("href=\"/goals/{LEGACY_GOAL}\"")));
    assert!(held.contains(LEGACY_GOAL_TITLE));
    assert_eq!(held.matches(LEGACY_PROMPT).count(), 1);

    // While its question waits for an unaided answer, the imported idea and
    // goal stay behind the same fence as generated material.
    app.occurrence = Some(legacy_occurrence(&app, "question"));
    let fenced = check(&app, "/map");
    assert!(!fenced.contains(LEGACY_PROMPT));
    assert!(!fenced.contains(LEGACY_GOAL_TITLE));
    assert!(fenced.contains("Current idea"));
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
