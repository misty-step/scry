mod common;

use scry::{model::App, web};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
fn render(app: &App, path: &str) -> String {
    web::render(app, path, &BTreeMap::new(), NOW, None)
}

#[test]
fn review_home_is_clear_and_does_not_masquerade_as_an_empty_text_library() {
    let app = App::new("synthetic-concept-home-csrf-value".into());
    let home = render(&app, "/");
    assert!(home.contains("data-state=\"mixed-home\""));
    assert!(home.contains("action=\"/practice/review/start\""));
    assert!(home.contains("action=\"/practice/lesson/start\""));
    assert!(home.contains("href=\"/library\">Library</a>"));
    assert!(!home.contains("Find a text") && !home.contains("library-query"));
    assert!(!home.contains("<h1>Your Library</h1>"));
    assert!(render(&app, "/library").contains("data-state=\"library\""));
}

#[test]
fn home_neither_shows_nor_replaces_the_held_earlier_question() {
    let (app, now) = common::earlier("question-cold-choice");
    assert_eq!(now, NOW);
    let before = app.clone();
    let home = render(&app, "/");
    assert!(home.contains("data-state=\"mixed-home\""));
    assert!(home.contains("href=\"/notebook\">Earlier practice</a>"));
    assert!(!home.contains("data-state=\"question\""));
    assert!(!home.contains(&app.occurrence.as_ref().unwrap().presentation.answer));
    assert_eq!(app, before);
    // Earlier practice shows it as it was left: unanswered, with no way to answer.
    let earlier = render(&app, "/notebook");
    assert!(earlier.contains("data-state=\"earlier-unanswered\""));
    assert!(!earlier.contains("data-state=\"question\"") && !earlier.contains("<form"));
    assert_eq!(app, before);
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
    assert!(unavailable.contains("Native content generation is retired"));
    assert!(!unavailable.contains("action=\"/create\""));
    assert!(!unavailable.contains("type=\"file\""));
    let ready = web::render(
        &app,
        "/create",
        &BTreeMap::from([("__preparation_ready".into(), "true".into())]),
        NOW,
        None,
    );
    assert!(!ready.contains("action=\"/create\""));
    assert!(!ready.contains("Prepare material"));
    assert!(ready.contains("native answer grading"));
    assert!(ready.contains("Your saved material and history remain available"));
    assert!(!ready.contains("video/"));
}

#[test]
fn related_concepts_reuse_both_directions_of_the_existing_prerequisite_graph() {
    let (mut app, _) = common::earlier("question-cold-choice");
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
    for heading in ["Notes", "Recorded answers", "Resources", "Related"] {
        assert!(first.contains(&format!("<h2>{heading}</h2>")), "{heading}");
    }
    assert!(first.contains("Used by"));
    assert!(first.contains(&format!("href=\"/concepts/{}\"", dependent.id)));
    assert!(second.contains("Builds on"));
    assert!(second.contains(&format!("href=\"/concepts/{}\"", prerequisite.id)));
}

#[test]
fn archived_goal_concepts_leave_active_navigation_but_keep_their_reference() {
    // The demo's Goal, archived after its last answer was graded.
    let (app, _) = common::earlier("result-then-archived");
    assert!(app.goals.values().all(|g| g.archived));
    let before = app.clone();
    let home = render(&app, "/");
    assert!(home.contains("action=\"/practice/review/start\""));
    assert!(home.contains("Lessons"));
    for c in app.concepts.values() {
        assert!(!home.contains(&format!("href=\"/concepts/{}\"", c.id)));
        let detail = render(&app, &format!("/concepts/{}", c.id));
        assert!(detail.contains(&c.name));
        assert!(detail.contains("Archived: retained for reference."));
        assert!(!detail.contains("Practice this idea"));
        // Its links to the other archived Concepts stay, each labelled archived.
        let related: Vec<_> = app
            .concepts
            .values()
            .filter(|o| c.prerequisites.contains(&o.id) || o.prerequisites.contains(&c.id))
            .collect();
        assert_eq!(detail.contains("<h2>Related</h2>"), !related.is_empty());
        for o in related {
            assert!(detail.contains(&format!(
                "href=\"/concepts/{}\">{}</a><span class=\"fineprint related-archived\">Archived</span>",
                o.id, o.name
            )));
        }
    }
    assert_eq!(
        app, before,
        "Rendering retained references must not change records"
    );
}
