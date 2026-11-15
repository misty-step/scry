//! SPEC § Experience: every core action is a native form, Save draft included.
//! SPEC § Invariants 1: nothing a client sends beyond an action's own fields
//! is accepted, for every Study control alike.
use scry::{engine, model::App, persistence, web};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;

fn call(app: &mut App, path: &str, op: &str, pairs: &[(&str, &str)]) -> scry::model::AppResult<()> {
    let mut fields = BTreeMap::from([
        ("csrf".to_string(), app.csrf.clone()),
        ("operation_id".to_string(), format!("drafts-controls-{op}")),
    ]);
    fields.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())));
    engine::mutate(app, path, &fields, NOW, op).map(|_| ())
}

/// Focused NATO practice at its first Quiz.
fn quizzed() -> App {
    let mut app = App::new("synthetic-csrf-at-least-24-characters".into());
    call(&mut app, "/practice/nato/start", "start-000001", &[]).unwrap();
    let pid = prompt(&app);
    call(
        &mut app,
        "/practice/quiz",
        "quiz-0000001",
        &[("prompt_id", &pid)],
    )
    .unwrap();
    app
}

fn prompt(app: &App) -> String {
    app.practice.current.as_ref().unwrap().prompt.id.clone()
}

/// What the browser posts when Save draft is pressed with JavaScript off: the
/// answer form's own fields, with no checkpoint generation.
fn native(app: &App, answer: &str) -> Vec<(&'static str, String)> {
    vec![
        ("revision", app.revision.to_string()),
        ("prompt_id", prompt(app)),
        ("answer", answer.to_string()),
    ]
}

fn pairs<'a>(fields: &'a [(&'static str, String)]) -> Vec<(&'static str, &'a str)> {
    fields.iter().map(|(k, v)| (*k, v.as_str())).collect()
}

#[test]
fn save_draft_and_discard_work_as_native_forms() {
    let mut app = quizzed();
    let page = web::render(&app, "/", &BTreeMap::new(), NOW, None);
    assert!(
        page.contains("formaction=\"/practice/draft\" data-checkpoint-save value=\"Save draft\"")
    );
    let attempts = app.practice.attempts.len();
    let cards = app.practice.cards.clone();
    let form = native(&app, "Al");
    call(&mut app, "/practice/draft", "native-save1", &pairs(&form)).unwrap();
    let draft = app.practice.answer_checkpoint.as_ref().unwrap();
    assert_eq!(draft.generation, 1);
    assert_eq!(draft.draft.as_ref().unwrap().answer, "Al");
    assert_eq!(
        app.practice.attempts.len(),
        attempts,
        "Saving never grades."
    );
    assert_eq!(app.practice.cards, cards, "Saving never schedules.");
    // The same press, replayed, returns the committed result and writes nothing.
    let saved = persistence::rows(&app).unwrap();
    call(&mut app, "/practice/draft", "native-save1", &pairs(&form)).unwrap();
    assert_eq!(persistence::rows(&app).unwrap(), saved);
    // The reloaded page holds the saved text and offers Discard.
    let page = web::render(&app, "/", &BTreeMap::new(), NOW, None);
    assert!(page.contains("value=\"Al\""));
    assert!(page.contains("data-checkpoint-present=\"true\""));
    // A native discard sends the field's text as well; it is ignored.
    let form = native(&app, "Alf");
    call(
        &mut app,
        "/practice/draft/discard",
        "native-discard",
        &pairs(&form),
    )
    .unwrap();
    let checkpoint = app.practice.answer_checkpoint.as_ref().unwrap();
    assert!(checkpoint.draft.is_none());
    assert_eq!(checkpoint.generation, 2);
    engine::validate_app(&app).unwrap();
}

#[test]
fn a_stale_native_page_cannot_overwrite_a_newer_draft() {
    let mut app = quizzed();
    let stale = native(&app, "loser");
    // Another tab saves first.
    let pid = prompt(&app);
    call(
        &mut app,
        "/practice/draft",
        "winner-save1",
        &[
            ("prompt_id", &pid),
            ("generation", "0"),
            ("answer", "winner"),
        ],
    )
    .unwrap();
    let before = persistence::rows(&app).unwrap();
    let refused = call(&mut app, "/practice/draft", "stale-save01", &pairs(&stale)).unwrap_err();
    assert_eq!(refused.status, 409);
    let refused = call(
        &mut app,
        "/practice/draft/discard",
        "stale-discard",
        &pairs(&stale),
    )
    .unwrap_err();
    assert_eq!(refused.status, 409);
    assert_eq!(persistence::rows(&app).unwrap(), before);
    // Without a generation or a revision there is nothing to compare against.
    let refused = call(
        &mut app,
        "/practice/draft",
        "no-revision1",
        &[("prompt_id", &pid), ("answer", "x")],
    )
    .unwrap_err();
    assert_eq!(refused.status, 422);
    // An enhanced discard still names only the draft and its generation.
    let refused = call(
        &mut app,
        "/practice/draft/discard",
        "enhanced-discard",
        &[("prompt_id", &pid), ("generation", "1"), ("answer", "x")],
    )
    .unwrap_err();
    assert_eq!(refused.status, 422);
    assert_eq!(persistence::rows(&app).unwrap(), before);
}

#[test]
fn a_refused_native_save_keeps_the_typed_text_on_the_page() {
    let app = quizzed();
    let pid = prompt(&app);
    let query = BTreeMap::from([
        ("__typed_prompt".to_string(), pid),
        ("__typed".to_string(), "Alf typed".to_string()),
    ]);
    let page = web::render(
        &app,
        "/",
        &query,
        NOW,
        Some("Let's return to what is saved."),
    );
    assert!(page.contains("value=\"Alf typed\""));
    // Text typed for another Prompt is never shown.
    let other = BTreeMap::from([
        ("__typed_prompt".to_string(), "0".repeat(32)),
        ("__typed".to_string(), "Alf typed".to_string()),
    ]);
    assert!(!web::render(&app, "/", &other, NOW, None).contains("Alf typed"));
}

#[test]
fn every_study_control_refuses_unknown_fields_and_returns_to_its_version() {
    let mut app = quizzed();
    let (sid, vid) = {
        let s = app.practice.studies.values().next().unwrap();
        (s.id.clone(), s.version_id.clone())
    };
    let before = persistence::rows(&app).unwrap();
    for path in ["/practice/pause", "/practice/resume", "/practice/drop"] {
        for extra in [
            ("grade", "Good"),
            ("status", "active"),
            ("pace", "light"),
            ("schedule", "now"),
        ] {
            let op = format!("{}-{}", path.trim_start_matches("/practice/"), extra.0);
            let refused = call(&mut app, path, &op, &[("study_id", &sid), extra]).unwrap_err();
            assert_eq!(refused.status, 422, "{path} with {}", extra.0);
            assert_eq!(persistence::rows(&app).unwrap(), before, "{path}");
        }
    }
    let refused = call(
        &mut app,
        "/practice/pace",
        "pace-extra-1",
        &[("study_id", &sid), ("pace", "light"), ("grade", "Good")],
    )
    .unwrap_err();
    assert_eq!(refused.status, 422);
    for (n, path) in [
        "/practice/pause",
        "/practice/resume",
        "/practice/pace",
        "/practice/drop",
    ]
    .iter()
    .enumerate()
    {
        let pace = [("study_id", sid.as_str()), ("pace", "light")];
        let fields = if *path == "/practice/pace" {
            &pace[..]
        } else {
            &pace[..1]
        };
        call(&mut app, path, &format!("control-{n:04}"), fields).unwrap();
        assert_eq!(
            web::study_control_page(&app, path, Some(&sid)).as_deref(),
            Some(format!("/library/versions/{vid}").as_str())
        );
    }
    assert_eq!(
        web::study_control_page(&app, "/practice/answer", Some(&sid)),
        None
    );
}

/// SPEC § Experience names the answer action **Check**, and Terms call a
/// text's Unit a phrase; practice screens use those words.
#[test]
fn practice_screens_use_spec_words() {
    let mut app = App::new("synthetic-csrf-at-least-24-characters".into());
    call(
        &mut app,
        "/practice/lesson/start",
        "words-lesson",
        &[("starter", "st-michael-en")],
    )
    .unwrap();
    let pid = prompt(&app);
    call(
        &mut app,
        "/practice/quiz",
        "words-quiz01",
        &[("prompt_id", &pid)],
    )
    .unwrap();
    let lesson = web::render(&app, "/recall", &BTreeMap::new(), NOW, None);
    assert!(lesson.contains("<button class=\"button primary\">Check</button>"));
    assert!(!lesson.contains("Check answer"));
    // Study's mix carries no "All subjects" line on its Prompt.
    let mut mixed = App::new("synthetic-csrf-at-least-24-characters".into());
    call(&mut mixed, "/practice/review/start", "words-mix0001", &[]).unwrap();
    let pid = prompt(&mixed);
    call(
        &mut mixed,
        "/practice/quiz",
        "words-mixquiz",
        &[("prompt_id", &pid)],
    )
    .unwrap();
    let page = web::render(&mixed, "/", &BTreeMap::new(), NOW, None);
    assert!(page.contains("data-study-phase=\"quiz\""));
    assert!(!page.contains("All subjects"));
    let missing = web::render(&app, "/no-such-page", &BTreeMap::new(), NOW, None);
    assert!(missing.contains(">Back to practice →</a>"));
    let earlier = web::render(&app, "/notebook", &BTreeMap::new(), NOW, None);
    assert!(earlier.contains("<a href=\"/\">Practice</a><a href=\"/map\">"));
}
