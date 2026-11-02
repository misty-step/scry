//! Authored lessons use the native mutation and persistence boundaries.
use scry::{engine, model::*, persistence, study};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
fn app() -> App {
    App::new("synthetic-lesson-csrf-at-least-32".into())
}
fn fields(a: &App, extra: &[(&str, &str)]) -> BTreeMap<String, String> {
    let mut f = BTreeMap::from([
        ("csrf".into(), a.csrf.clone()),
        ("revision".into(), a.revision.to_string()),
        (
            "operation_id".into(),
            format!("lesson-operation-{:016}", a.revision),
        ),
    ]);
    for (k, v) in extra {
        f.insert((*k).into(), (*v).into());
    }
    f
}
fn mutate(a: &mut App, path: &str, extra: &[(&str, &str)]) -> MutationResult {
    let f = fields(a, extra);
    let seed = format!("lesson-seed-{}", a.revision);
    let result = engine::mutate(a, path, &f, NOW, &seed).unwrap();
    engine::validate_app(a).unwrap();
    result
}
fn answer(a: &mut App, text: &str) {
    if a.practice.current.as_ref().unwrap().source_visible() {
        let id = a.practice.current.as_ref().unwrap().prompt.id.clone();
        mutate(a, "/practice/quiz", &[("prompt_id", &id)]);
    }
    let id = a.practice.current.as_ref().unwrap().prompt.id.clone();
    mutate(
        a,
        "/practice/answer",
        &[("prompt_id", &id), ("answer", text)],
    );
}
fn next(a: &mut App, mode: &str) {
    let id = a.practice.current.as_ref().unwrap().prompt.id.clone();
    mutate(
        a,
        "/practice/lesson/next",
        &[("after", &id), ("mode", mode)],
    );
}
#[test]
fn starter_is_idempotent_and_recovery_preserves_all_evidence() {
    let mut a = app();
    let f = fields(&a, &[("starter", "rosary-joyful")]);
    let first = engine::mutate(&mut a, "/practice/lesson/start", &f, NOW, "lesson-start").unwrap();
    let saved = a.clone();
    assert_eq!(
        engine::mutate(&mut a, "/practice/lesson/start", &f, NOW, "retry").unwrap(),
        first
    );
    assert_eq!(a, saved);
    assert_eq!(a.practice.cards.len(), 5);
    assert!(matches!(
        a.practice.current.as_ref().unwrap().prompt.scope,
        study::Scope::Whole
    ));
    assert!(!a.practice.current.as_ref().unwrap().prompt.assisted);
    answer(
        &mut a,
        "The Annunciation\nVisitation\nBirth of Jesus\nPresentation\nFinding of Jesus in the Temple",
    );
    let page = scry::web::render(&a, "/recall", &BTreeMap::new(), NOW, None);
    assert!(page.contains("1 answer</span>"));
    assert!(!page.contains("1 answers</span>"));
    let attempt = a.practice.attempts.values().next().unwrap();
    assert_eq!(format!("{:?}", attempt.grade.verdict), "Good");
    assert!(!attempt.assisted);
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&a).unwrap()).unwrap(),
        a
    );
}
#[test]
fn source_study_is_ungraded_and_memory_phrase_and_whole_challenges_are_separate() {
    let mut a = app();
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "st-michael-en-lasance-1911")],
    );
    assert_eq!(a.practice.cards.len(), 6);
    let p = a.practice.current.as_ref().unwrap().clone();
    assert!(p.prompt.assisted);
    assert!(matches!(
        p.prompt.scope,
        study::Scope::Unit { position: 1, .. }
    ));
    let text = a.library.versions[&p.prompt.version_id].sections[0].units[0]
        .text
        .clone();
    let schedules = a
        .practice
        .cards
        .iter()
        .map(|(id, c)| (id.clone(), c.state.schedule.clone()))
        .collect::<BTreeMap<_, _>>();
    assert_eq!(p.phase, Some(study::StudyPhase::Study));
    assert!(a.practice.attempts.is_empty());
    let source_id = p.prompt.id.clone();
    mutate(&mut a, "/practice/quiz", &[("prompt_id", &source_id)]);
    assert!(a.practice.attempts.is_empty());
    for (id, schedule) in &schedules {
        assert_eq!(&a.practice.cards[id].state.schedule, schedule);
    }
    answer(&mut a, &text);
    let attempt = a.practice.attempts.values().next().unwrap();
    assert!(!attempt.assisted);
    assert!(attempt.effects.iter().all(|e| e.scheduled));
    for (id, schedule) in schedules {
        if id != p.targets[0].card_id {
            assert_eq!(a.practice.cards[&id].state.schedule, schedule);
        }
    }
    next(&mut a, "join");
    assert!(matches!(
        a.practice.current.as_ref().unwrap().prompt.scope,
        study::Scope::Section { .. }
    ));
    let joined = a.practice.current.as_ref().unwrap().clone();
    let joined_text = joined
        .targets
        .iter()
        .map(|target| {
            a.library.versions[&joined.prompt.version_id]
                .sections
                .iter()
                .flat_map(|s| s.units.iter())
                .nth(target.unit_index)
                .unwrap()
                .text
                .as_str()
        })
        .collect::<String>();
    answer(&mut a, &joined_text);
    next(&mut a, "whole");
    let p = a.practice.current.as_ref().unwrap();
    assert!(matches!(p.prompt.scope, study::Scope::Whole));
    assert!(!p.prompt.assisted);
    assert_eq!(p.targets.len(), 6);
    let full = a.library.versions[&p.prompt.version_id].text.clone();
    answer(&mut a, &full);
    assert_eq!(a.practice.attempts.len(), 3);
    next(&mut a, "phrases");
    assert!(matches!(
        a.practice.current.as_ref().unwrap().prompt.scope,
        study::Scope::Unit { position: 1, .. }
    ));
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&a).unwrap()).unwrap(),
        a
    );
}
#[test]
fn protected_nato_cannot_be_replaced_but_answered_switch_preserves_history() {
    let mut a = app();
    mutate(&mut a, "/practice/nato/start", &[]);
    let saved = a.clone();
    let f = fields(&a, &[("starter", "rosary-joyful")]);
    assert_eq!(
        engine::mutate(&mut a, "/practice/lesson/start", &f, NOW, "switch")
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(a, saved);
    answer(&mut a, "Alfa");
    let before = a.practice.clone();
    let after = a.practice.current.as_ref().unwrap().prompt.id.clone();
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "rosary-joyful"), ("after", &after)],
    );
    for (id, attempt) in before.attempts {
        assert_eq!(a.practice.attempts[&id], attempt);
    }
    for (id, card) in before.cards {
        assert_eq!(a.practice.cards[&id], card);
    }
    for (id, s) in before.studies {
        assert_eq!(a.practice.studies[&id], s);
    }
}
#[test]
fn client_grades_invalid_modes_and_stale_results_are_atomic() {
    let mut a = app();
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "rosary-joyful")],
    );
    for (path, extra, status) in [
        (
            "/practice/lesson/start",
            vec![("starter", "rosary-joyful"), ("grade", "Good")],
            422,
        ),
        (
            "/practice/lesson/next",
            vec![("after", "stale"), ("mode", "whole")],
            409,
        ),
        ("/practice/answer", vec![("grade", "Good")], 422),
    ] {
        let before = a.clone();
        let mut f = fields(&a, &extra);
        if path == "/practice/answer" {
            f.insert(
                "prompt_id".into(),
                a.practice.current.as_ref().unwrap().prompt.id.clone(),
            );
            f.insert("answer".into(), "Annunciation".into());
        }
        assert_eq!(
            engine::mutate(&mut a, path, &f, NOW, "invalid")
                .unwrap_err()
                .status,
            status
        );
        assert_eq!(a, before);
    }
    answer(
        &mut a,
        "Annunciation\nNativity\nPresentation\nFinding in the Temple",
    );
    assert_eq!(
        format!(
            "{:?}",
            a.practice.attempts.values().next().unwrap().grade.verdict
        ),
        "Again"
    );
    let before = a.clone();
    let id = a.practice.current.as_ref().unwrap().prompt.id.clone();
    let f = fields(&a, &[("after", &id), ("mode", "self-grade")]);
    assert_eq!(
        engine::mutate(&mut a, "/practice/lesson/next", &f, NOW, "invalid")
            .unwrap_err()
            .status,
        422
    );
    assert_eq!(a, before);
}

#[test]
fn externally_authored_text_uses_same_lesson_without_changing_pace_or_version() {
    let mut a = app();
    let lines = (1..=7)
        .map(|i| format!("Supplied phrase {i}.\n"))
        .collect::<Vec<_>>();
    let data=serde_json::json!({"title":"Supplied synthetic text","kind":"text","text":lines.concat(),"name":"Synthetic edition · en","sources":["https://example.com/synthetic"],"sections":[{"heading":"Authored meaning group","units":lines}]}).to_string();
    let vid = mutate(&mut a, "/library/version/add", &[("data", &data)])
        .entity_id
        .unwrap();
    let original = a.library.versions[&vid].clone();
    assert!(
        serde_json::to_value(&original)
            .unwrap()
            .get("ordered_aliases")
            .is_none(),
        "Empty extension must not alter old serialized shape."
    );
    let sid = mutate(
        &mut a,
        "/practice/learn",
        &[("version_id", &vid), ("pace", "light")],
    )
    .entity_id
    .unwrap();
    mutate(&mut a, "/practice/lesson/start", &[("version_id", &vid)]);
    assert_eq!(
        a.practice.cards.len(),
        7,
        "Deliberate lesson is not blocked on daily new-unit cap."
    );
    assert_eq!(a.practice.studies[&sid].pace, study::Pace::Light);
    assert_eq!(
        a.library.versions[&vid], original,
        "Lesson does not rewrite authored Version/hash."
    );
    let full = original.text.clone();
    for turn in 0..14 {
        let i = turn % 7;
        let p = a.practice.current.as_ref().unwrap();
        assert!(matches!(p.prompt.scope,study::Scope::Unit{position,..} if position==i+1));
        let text = original.sections[0].units[i].text.clone();
        answer(&mut a, &text);
        next(&mut a, "phrases");
    }
    assert_eq!(
        a.practice.current.as_ref().unwrap().prompt.cue_level,
        study::Cue::WholeTitle,
        "Subsequent memory quizzes hide source even when historical card cue state is Read."
    );
    answer(&mut a, &original.sections[0].units[0].text);
    next(&mut a, "whole");
    answer(&mut a, &full);
    assert_eq!(a.library.versions[&vid], original);
    let mut explicit: serde_json::Value = serde_json::from_str(&data).unwrap();
    explicit["ordered_aliases"] = serde_json::json!({});
    assert_eq!(
        mutate(
            &mut a,
            "/library/version/add",
            &[("data", &explicit.to_string())]
        )
        .entity_id
        .as_deref(),
        Some(vid.as_str()),
        "Empty aliases reimport retains immutable identity."
    );
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&a).unwrap()).unwrap(),
        a
    );
}

#[test]
fn deliberate_leave_records_no_attempt_and_rejects_stale_answers() {
    let mut a = app();
    mutate(&mut a, "/practice/nato/start", &[]);
    let old = a.practice.current.as_ref().unwrap().prompt.id.clone();
    let original = a.practice.clone();
    let before = a.clone();
    let f = fields(&a, &[("starter", "rosary-joyful"), ("leave", "stale")]);
    assert_eq!(
        engine::mutate(&mut a, "/practice/lesson/start", &f, NOW, "bad-leave")
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(a, before);
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "rosary-joyful"), ("leave", &old)],
    );
    assert!(a.practice.attempts.is_empty());
    for (id, card) in original.cards {
        assert_eq!(a.practice.cards[&id], card);
    }
    let before = a.clone();
    let f = fields(&a, &[("prompt_id", &old), ("answer", "Alfa")]);
    assert_eq!(
        engine::mutate(&mut a, "/practice/answer", &f, NOW, "late-answer")
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(a, before);
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&a).unwrap()).unwrap(),
        a
    );
}

#[test]
fn lesson_sources_link_only_absolute_http_and_escape_other_citations() {
    let mut a = app();
    let mut input = scry::lessons::input("st-michael-en-lasance-1911").unwrap();
    input.sources = vec![
        "https://example.org/prayer?a=1&b=2".into(),
        "http://example.org/edition".into(),
        "Lasance <1911>, p. 213".into(),
        "javascript:alert(1)".into(),
        "data:text/html,<script>alert(1)</script>".into(),
        "https:///missing-host".into(),
    ];
    let id = a.library.add(input, "source-render", None).unwrap();
    mutate(&mut a, "/practice/lesson/start", &[("version_id", &id)]);
    let page = scry::web::render(&a, "/recall", &BTreeMap::new(), NOW, None);
    assert!(page.contains("href=\"https://example.org/prayer?a=1&amp;b=2\""));
    assert!(page.contains("href=\"http://example.org/edition\""));
    assert!(page.contains("<li>Lasance &lt;1911&gt;, p. 213</li>"));
    assert!(page.contains("<li>javascript:alert(1)</li>"));
    assert!(page.contains("<li>data:text/html,&lt;script&gt;alert(1)&lt;/script&gt;</li>"));
    assert!(!page.contains("href=\"javascript:"));
    assert!(!page.contains("href=\"data:"));
    assert!(!page.contains("href=\"https:///missing-host"));
}

#[test]
fn mistaken_continuation_gets_clear_feedback_and_a_fenced_guided_retry() {
    let mut a = app();
    let data = serde_json::json!({"title":"Synthetic phrase lesson","kind":"text","name":"Synthetic English","text":"First little phrase. Second little phrase.","sections":[{"heading":"Pair","units":["First little phrase. ","Second little phrase."]}]}).to_string();
    let vid = mutate(&mut a, "/library/version/add", &[("data", &data)])
        .entity_id
        .unwrap();
    mutate(&mut a, "/practice/lesson/start", &[("version_id", &vid)]);
    let first = a.practice.current.as_ref().unwrap().clone();
    let prompt = scry::web::render(&a, "/recall", &BTreeMap::new(), NOW, None);
    assert!(prompt.contains("Try from memory"));
    assert!(!prompt.contains("action=\"/practice/answer\""));
    assert!(prompt.contains("Phrase 1 / 2"));
    assert!(prompt.contains("Synthetic English"));
    answer(&mut a, "Second little phrase.");
    let previous_attempts = a.practice.attempts.clone();
    let result = scry::web::render(&a, "/recall", &BTreeMap::new(), NOW, None);
    assert!(result.contains("Not quite"));
    assert!(!result.contains("Practised with help"));
    let target = result
        .split("class=\"lesson-target\"")
        .nth(1)
        .unwrap()
        .split("</div>")
        .next()
        .unwrap();
    assert!(target.contains("First little phrase."));
    assert!(!target.contains("Second little phrase."));
    assert!(result.contains("Try this phrase again"));
    assert!(result.contains("Continue to next phrase"));
    let answered_id = a.practice.current.as_ref().unwrap().prompt.id.clone();
    let retry = fields(&a, &[("after", &answered_id), ("mode", "retry")]);
    let r = engine::mutate(&mut a, "/practice/lesson/next", &retry, NOW, "retry-one").unwrap();
    engine::validate_app(&a).unwrap();
    let saved = a.clone();
    assert_eq!(
        engine::mutate(&mut a, "/practice/lesson/next", &retry, NOW, "duplicate").unwrap(),
        r
    );
    assert_eq!(a, saved);
    assert_eq!(a.practice.attempts, previous_attempts);
    let second = a.practice.current.as_ref().unwrap();
    assert_ne!(second.prompt.id, first.prompt.id);
    assert_eq!(second.prompt.scope, first.prompt.scope);
    assert_eq!(second.prompt.unit_ids, first.prompt.unit_ids);
    assert_eq!(second.prompt.cue_level, study::Cue::Read);
    assert!(second.prompt.assisted);
    let stale = fields(&a, &[("after", &first.prompt.id), ("mode", "phrases")]);
    assert_eq!(
        engine::mutate(&mut a, "/practice/lesson/next", &stale, NOW, "stale")
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(a, saved);
    answer(&mut a, "First little phrase.");
    let attempt = a.practice.attempts[&a
        .practice
        .current
        .as_ref()
        .unwrap()
        .attempt_id
        .clone()
        .unwrap()]
        .clone();
    assert!(attempt.assisted);
    assert!(attempt.effects.iter().all(|e| !e.scheduled));
    let correct = scry::web::render(&a, "/recall", &BTreeMap::new(), NOW, None);
    assert!(correct.contains("That matches · with help"));
    next(&mut a, "phrases");
    assert!(matches!(
        a.practice.current.as_ref().unwrap().prompt.scope,
        study::Scope::Unit { position: 2, .. }
    ));
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&a).unwrap()).unwrap(),
        a
    );
}

#[test]
fn supplied_english_is_distinct_and_does_not_rewrite_the_historical_lesson() {
    let mut a = app();
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "st-michael-en-lasance-1911")],
    );
    let historical = a.practice.current.as_ref().unwrap().clone();
    let old_version = a.library.versions[&historical.prompt.version_id].clone();
    answer(&mut a, &old_version.sections[0].units[0].text);
    let old_attempts = a.practice.attempts.clone();
    let answered_id = a.practice.current.as_ref().unwrap().prompt.id.clone();
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "st-michael-en"), ("after", &answered_id)],
    );
    let p = a.practice.current.as_ref().unwrap();
    let new_version = &a.library.versions[&p.prompt.version_id];
    assert_ne!(new_version.id, old_version.id);
    assert_eq!(a.library.versions[&old_version.id], old_version);
    assert_eq!(a.practice.attempts, old_attempts);
    let page = scry::web::render(&a, "/recall", &BTreeMap::new(), NOW, None);
    assert!(page.contains("English · supplied wording"));
    assert!(
        !page.contains("This historical English wording begins"),
        "Shared Work context must not masquerade as this Version’s provenance."
    );
    assert!(
        a.library.works[&old_version.work_id]
            .context
            .contains("This historical English wording begins"),
        "Stored historical Work context is preserved, not rewritten."
    );
    let expected = [
        "st michael the archangel defend us in battle",
        "be our protection against the wickedness and snares of the devil",
        "may god rebuke him we humbly pray",
        "and do thou o prince of the heavenly host",
        "by the power of god",
        "cast into hell satan and all the evil spirits",
        "who prowl about the world seeking the ruin of souls",
        "amen",
    ];
    let actual = new_version
        .sections
        .iter()
        .flat_map(|s| &s.units)
        .map(|u| scry::grading::normalized_words(&u.text).join(" "))
        .collect::<Vec<_>>();
    assert_eq!(actual, expected);
    assert_eq!(
        new_version
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .map(|u| u.text.as_str())
            .collect::<String>(),
        new_version.text
    );
    assert!(p.prompt.assisted);
    assert_eq!(p.targets[0].before.schedule.reps, 0);
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&a).unwrap()).unwrap(),
        a
    );
}

#[test]
fn retry_rejects_non_phrase_scopes_without_mutation() {
    let mut a = app();
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "st-michael-en")],
    );
    answer(&mut a, "Synthetic wrong answer");
    next(&mut a, "whole");
    answer(&mut a, "Synthetic wrong answer");
    let before = a.clone();
    let id = a.practice.current.as_ref().unwrap().prompt.id.clone();
    let f = fields(&a, &[("after", &id), ("mode", "retry")]);
    assert_eq!(
        engine::mutate(&mut a, "/practice/lesson/next", &f, NOW, "invalid")
            .unwrap_err()
            .status,
        422
    );
    assert_eq!(a, before);
}

#[test]
fn chooser_groups_topics_and_identifies_current_without_restarting_it() {
    let mut a = app();
    let before = a.clone();
    let shelf = scry::web::render(&a, "/lessons", &BTreeMap::new(), NOW, None);
    assert_eq!(shelf.matches("class=\"lesson-group\"").count(), 4);
    assert_eq!(
        shelf.matches("action=\"/practice/lesson/start\"").count(),
        11
    );
    assert!(shelf.contains("Historical English"));
    assert!(shelf.contains("Lasance · 1911"));
    assert!(!shelf.contains("Annunciation"));
    assert!(!shelf.contains("defend us in battle"));
    assert_eq!(a, before, "Browsing creates no content or evidence.");
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "st-michael-en")],
    );
    let before = a.clone();
    let inline = scry::web::render(&a, "/", &BTreeMap::new(), NOW, None);
    let picker = inline.split("class=\"lesson-picker\"").nth(1).unwrap();
    assert!(picker.contains("data-current-lesson=\"st-michael-en\""));
    assert!(!picker.contains("name=\"starter\" value=\"st-michael-en\""));
    assert!(picker.contains("data-picker-close hidden"));
    assert!(picker.contains("Switching leaves this question ungraded."));
    assert_eq!(a, before);
    answer(&mut a, "St Michael the Archangel, defend us in battle");
    let result = scry::web::render(&a, "/lessons", &BTreeMap::new(), NOW, None);
    assert!(result.contains("href=\"/recall\">Back to practice"));
    assert!(
        !result.contains("name=\"starter\" value=\"st-michael-en\""),
        "Returning to held feedback must not invoke lesson start."
    );
}

#[test]
fn chooser_does_not_confuse_same_named_supplied_content_or_leak_its_metadata() {
    let mut a = app();
    let mut input = scry::lessons::input("st-michael-en").unwrap();
    input.text = "Distinct supplied content.\n".into();
    input.sections = Some(vec![scry::library::SectionInput {
        heading: "Supplied".into(),
        units: vec![input.text.clone()],
    }]);
    let vid = a.library.add(input, "same-name-supplied", None).unwrap();
    let shelf = scry::web::render(&a, "/lessons", &BTreeMap::new(), NOW, None);
    assert!(shelf.contains("Your other texts"));
    assert!(shelf.contains(&format!("name=\"version_id\" value=\"{vid}\"")));
    assert!(
        !scry::lessons::catalog()
            .iter()
            .any(|l| l.matches(&a.library.versions[&vid]))
    );
    a.library.works.values_mut().next().unwrap().title = "PRIVATE ANSWER BEARING TITLE".into();
    mutate(&mut a, "/practice/nato/start", &[]);
    let protected = scry::web::render(&a, "/lessons", &BTreeMap::new(), NOW, None);
    assert!(!protected.contains("PRIVATE ANSWER BEARING TITLE"));
    assert!(!protected.contains(&vid));
    assert!(!protected.contains("Your other texts"));
    assert!(protected.contains("data-current-lesson=\"nato\""));
}

#[test]
fn chooser_returns_to_generic_exact_study_without_entering_nato_home() {
    let mut a = app();
    let data = serde_json::json!({"title":"Supplied text", "kind":"text", "text":"A synthetic line.\n", "name":"Private edition"}).to_string();
    let vid = mutate(&mut a, "/library/version/add", &[("data", &data)])
        .entity_id
        .unwrap();
    mutate(&mut a, "/practice/learn", &[("version_id", &vid)]);
    study::ensure_stream(&mut a, NOW, "generic-current").unwrap();
    let before = a.clone();
    let picker = scry::web::render(&a, "/lessons", &BTreeMap::new(), NOW, None);
    assert!(picker.contains("href=\"/recall\">Back to practice"));
    let returned = scry::web::render(&a, "/recall", &BTreeMap::new(), NOW, None);
    assert!(returned.contains("data-state=\"exact-prompt\""));
    assert_eq!(a, before);
}

#[test]
fn simplified_practice_has_one_identity_and_secondary_tools_without_losing_actions() {
    let mut a = app();
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "st-michael-en")],
    );
    let before = a.clone();
    let page = scry::web::render(&a, "/recall", &BTreeMap::new(), NOW, None);
    assert_eq!(page.matches("Phrase 1 / 8").count(), 1);
    assert_eq!(page.matches("<h1>").count(), 1);
    assert!(page.contains("class=\"practice-tools\""));
    assert!(page.contains("Try from memory"));
    assert!(page.contains("data-study-phase=\"study\""));
    assert!(!page.contains("action=\"/practice/answer\""));
    assert!(!page.contains("Copy the phrase"));
    assert!(!page.contains("Text practice</span>"));
    assert!(!page.contains("<footer"));
    assert_eq!(a, before);
    answer(&mut a, "wrong");
    let result = scry::web::render(&a, "/recall", &BTreeMap::new(), NOW, None);
    assert!(result.contains("Try this phrase again"));
    assert!(result.contains("name=\"mode\" value=\"whole\""));
    assert!(result.contains("name=\"mode\" value=\"join\""));
    assert!(result.contains("<summary>More</summary>"));
}
