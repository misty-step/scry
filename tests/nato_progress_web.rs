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
fn page(app: &App) -> String {
    web::render(app, "/", &BTreeMap::new(), NOW, None)
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
    assert!(html.contains("Your progress"));
    assert!(html.contains("0 reviews"));
    assert!(!html.contains("0%"));
}
#[test]
fn progress_during_recall_never_leaks_words_and_cannot_mutate_state() {
    let mut app = App::new("synthetic-progress-web-csrf-32chars".into());
    act(&mut app, "/practice/nato/start", &[], "start");
    let before = app.clone();
    let html = page(&app);
    for word in ["Alfa", "Bravo", "Charlie", "Zulu", "Alphabet reference"] {
        assert!(!html.contains(word), "{word}");
    }
    assert!(
        html.contains("id=\"progress-A\" value=\"A\" aria-label=\"Letter A progress\" checked")
    );
    assert!(html.contains("Due times are schedule suggestions"));
    assert!(html.contains("data-metric=\"position\">On screen now"));
    assert!(html.contains("data-metric=\"position\">Up next"));
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
    assert!(result.contains("data-metric=\"position\">Again in 26 letters"));
    assert!(result.contains("Correct, "));
    act(
        &mut app,
        "/practice/next",
        &[("after", p.id), ("study_id", p.study_id)],
        "next",
    );
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
        "/practice/answer",
        &[("prompt_id", p.id), ("answer", "Alfa".into())],
        "answer",
    );
    let html = page(&app);
    assert!(html.contains("1 review"));
    assert!(html.contains("1 with help"));
    assert_eq!(html.matches("data-metric=\"accuracy\">—").count(), 26);
    assert!(html.contains("With help, "));
    assert!(!html.contains("data-metric=\"accuracy\">0%"));
}
