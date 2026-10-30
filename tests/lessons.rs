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
fn phrase_practice_is_assisted_and_whole_challenge_is_separate() {
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
    answer(&mut a, &text);
    let attempt = a.practice.attempts.values().next().unwrap();
    assert!(attempt.assisted);
    assert!(attempt.effects.iter().all(|e| !e.scheduled));
    for (id, schedule) in schedules {
        assert_eq!(a.practice.cards[&id].state.schedule, schedule);
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
        study::Cue::FirstLetters,
        "Two successful rounds fade phrase cues, not global mastery."
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
