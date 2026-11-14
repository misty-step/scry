//! The progress inspector is read-only metadata, including during recall.
use scry::{engine, model::*, web};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
fn act(app: &mut App, path: &str, extras: &[(&str, String)], tag: &str) {
    let mut fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), format!("progress-web-{tag}")),
        ("revision".into(), app.revision.to_string()),
    ]);
    fields.extend(extras.iter().map(|(k, v)| (k.to_string(), v.clone())));
    engine::mutate(app, path, &fields, NOW, tag).unwrap();
}
fn try_from_memory(app: &mut App, tag: &str) {
    let id = app.practice.current.as_ref().unwrap().prompt.id.clone();
    act(app, "/practice/quiz", &[("prompt_id", id)], tag);
}
fn page(app: &App) -> String {
    web::render(
        app,
        "/",
        &BTreeMap::from([("practice".into(), "nato".into())]),
        NOW,
        None,
    )
}
#[test]
fn empty_progress_has_all_letters_and_no_invented_accuracy() {
    let app = App::new("synthetic-progress-web-csrf-32chars".into());
    let html = page(&app);
    assert_eq!(
        html.matches("class=\"progress-radio visually-hidden\"")
            .count(),
        26
    );
    assert_eq!(html.matches("data-metric=\"accuracy\">—").count(), 26);
    assert_eq!(
        html.matches("data-metric=\"due\">Not scheduled").count(),
        26
    );
    assert!(html.contains("<span>Progress</span>"));
    assert!(html.contains("0 reviews"));
    assert!(!html.contains("0%"));
}
#[test]
fn progress_during_recall_never_leaks_words_and_cannot_mutate_state() {
    let mut app = App::new("synthetic-progress-web-csrf-32chars".into());
    act(&mut app, "/practice/nato/start", &[], "start");
    // A new letter is studied with its word, then quizzed with nothing shown.
    let study = page(&app);
    assert!(study.contains("Alfa") && study.contains("Try from memory"));
    assert!(!study.contains("id=\"answer\""));
    for word in ["Bravo", "Charlie", "Zulu", "Alphabet reference"] {
        assert!(!study.contains(word), "{word}");
    }
    try_from_memory(&mut app, "quiz-a");
    let before = app.clone();
    let html = page(&app);
    for word in ["Alfa", "Bravo", "Charlie", "Zulu", "Alphabet reference"] {
        assert!(!html.contains(word), "{word}");
    }
    assert!(
        html.contains("id=\"progress-A\" value=\"A\" aria-label=\"Letter A progress\" checked")
    );
    assert!(html.contains("Practice selects due reviews and new letters"));
    assert!(html.contains("data-metric=\"position\">On screen now"));
    assert!(!html.contains("Up next"));
    assert!(!html.contains("<form method=\"get\""));
    assert_eq!(app, before);
    let p = app.practice.current.as_ref().unwrap().prompt.clone();
    act(
        &mut app,
        "/practice/answer",
        &[("prompt_id", p.id.clone()), ("answer", "Alfa".into())],
        "answer",
    );
    let result = page(&app);
    assert!(result.contains("1 review"));
    assert!(result.contains("data-metric=\"accuracy\">100%"));
    assert!(result.contains("1 of 1 answers"));
    assert!(result.contains("data-metric=\"position\">Answer saved"));
    assert!(result.contains("Correct, "));
    act(
        &mut app,
        "/practice/next",
        &[("after", p.id), ("study_id", p.study_id)],
        "next",
    );
    let study = page(&app);
    assert!(study.contains("Letter B") && study.contains("Bravo"));
    assert!(!study.contains("Alfa"));
    try_from_memory(&mut app, "quiz-b");
    let next = page(&app);
    assert!(next.contains("Letter B"));
    assert!(!next.contains("Alfa"));
    assert!(!next.contains("Bravo"));
    assert!(next.contains("data-metric=\"accuracy\">100%"));
    let restored: App = serde_json::from_str(&serde_json::to_string(&app).unwrap()).unwrap();
    assert_eq!(page(&restored), next);
}
#[test]
fn reveal_and_helped_answer_are_not_an_unaided_failure() {
    let mut app = App::new("synthetic-progress-web-csrf-32chars".into());
    act(&mut app, "/practice/nato/start", &[], "start");
    try_from_memory(&mut app, "first-quiz");
    let p = app.practice.current.as_ref().unwrap().prompt.clone();
    act(
        &mut app,
        "/practice/reveal",
        &[("prompt_id", p.id.clone())],
        "reveal",
    );
    assert!(page(&app).contains("0 reviews"));
    act(
        &mut app,
        "/practice/quiz",
        &[("prompt_id", p.id.clone())],
        "quiz",
    );
    let quiz_id = app.practice.current.as_ref().unwrap().prompt.id.clone();
    act(
        &mut app,
        "/practice/answer",
        &[("prompt_id", quiz_id), ("answer", "Alfa".into())],
        "answer",
    );
    let html = page(&app);
    assert!(html.contains("1 review"));
    assert!(html.contains("1 with help"));
    assert_eq!(html.matches("data-metric=\"accuracy\">—").count(), 26);
    assert!(html.contains("With help, "));
    assert!(!html.contains("data-metric=\"accuracy\">0%"));
}
