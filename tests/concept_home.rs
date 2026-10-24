use scry::{engine, model::App, web};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
fn render(app: &App, path: &str) -> String {
    web::render(app, path, &BTreeMap::new(), NOW, None)
}

#[test]
fn concept_home_is_clear_and_does_not_masquerade_as_an_empty_text_library() {
    let app = App::new("synthetic-concept-home-csrf-value".into());
    let home = render(&app, "/");
    assert!(home.contains("data-state=\"concept-home\""));
    assert!(home.contains("<h1>What do you want to learn?</h1>"));
    assert!(home.contains("href=\"/create\">Add material</a>"));
    assert!(home.contains("href=\"/recall\">Text practice</a>"));
    assert!(!home.contains("Find a text") && !home.contains("library-query"));
    assert!(!home.contains("<h1>Your Library</h1>"));
    assert!(render(&app, "/library").contains("data-state=\"library\""));
}

#[test]
fn home_resumes_without_exposing_or_replacing_the_concept_occurrence() {
    let mut app = App::new("synthetic-concept-home-csrf-value".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    engine::ensure_occurrence(&mut app, NOW, "home-practice").unwrap();
    let before = app.clone();
    let home = render(&app, "/");
    assert!(home.contains("data-state=\"concept-home\""));
    assert!(home.contains("href=\"/notebook\">Resume practice</a>"));
    assert!(!home.contains("data-state=\"question\""));
    assert!(!home.contains(&app.occurrence.as_ref().unwrap().presentation.answer));
    assert_eq!(app, before);
    assert!(render(&app, "/notebook").contains("data-state=\"question\""));
    assert_eq!(
        web::mutation_redirect(&app, "/practice/learn", "/"),
        "/recall"
    );
    assert_eq!(
        web::mutation_redirect(&app, "/review/answer", "/"),
        "/notebook"
    );
}

#[test]
fn unavailable_preparation_does_not_offer_a_fake_submit_or_supported_formats() {
    let app = App::new("synthetic-concept-home-csrf-value".into());
    let unavailable = render(&app, "/create");
    assert!(unavailable.contains("Preparation is unavailable"));
    assert!(!unavailable.contains("action=\"/create\""));
    assert!(!unavailable.contains("type=\"file\""));
    let ready = web::render(
        &app,
        "/create",
        &BTreeMap::from([("__preparation_ready".into(), "true".into())]),
        NOW,
        None,
    );
    assert!(ready.contains("action=\"/create\""));
    assert!(ready.contains("Prepare material"));
    assert!(ready.contains("paid AI"));
    assert!(ready.contains("pasted links are not opened"));
    assert!(!ready.contains("video/"));
}

#[test]
fn related_concepts_reuse_both_directions_of_the_existing_prerequisite_graph() {
    let mut app = App::new("synthetic-concept-home-csrf-value".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    app.occurrence = None;
    let prerequisite = app
        .concepts
        .values()
        .find(|c| c.prerequisites.is_empty())
        .unwrap();
    let dependent = app
        .concepts
        .values()
        .find(|c| !c.prerequisites.is_empty())
        .unwrap();
    let first = render(&app, &format!("/concepts/{}", prerequisite.id));
    let second = render(&app, &format!("/concepts/{}", dependent.id));
    for heading in ["Study", "Practice", "Resources", "Related"] {
        assert!(first.contains(&format!("<h2>{heading}</h2>")), "{heading}");
    }
    assert!(first.contains("Used by"));
    assert!(first.contains(&format!("href=\"/concepts/{}\"", dependent.id)));
    assert!(second.contains("Builds on"));
    assert!(second.contains(&format!("href=\"/concepts/{}\"", prerequisite.id)));
}
