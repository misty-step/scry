use scry::{api, engine, model::*, persistence, study, web};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
const TEXT: &str = "A patient garden grows beside the water.\n";
fn app() -> App {
    App::new("synthetic-phone-library-csrf-32chars".into())
}
fn fields(app: &App, op: &str, values: &[(&str, &str)]) -> BTreeMap<String, String> {
    let mut f = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), op.into()),
        ("revision".into(), app.revision.to_string()),
    ]);
    for (k, v) in values {
        f.insert((*k).into(), (*v).into());
    }
    f
}
fn action(app: &mut App, path: &str, op: &str, values: &[(&str, &str)]) -> MutationResult {
    let f = fields(app, op, values);
    let result = engine::mutate(app, path, &f, NOW, op).unwrap();
    engine::validate_app(app).unwrap();
    result
}
fn add(app: &mut App, op: &str) -> (MutationResult, BTreeMap<String, String>) {
    let mut f = fields(
        app,
        op,
        &[
            ("title", "Garden <&>"),
            ("text", TEXT),
            ("name", "Authored edition"),
            ("source", "Synthetic author, not a fetched source"),
        ],
    );
    api::library_form_fields(&mut f).unwrap();
    let result = engine::mutate(app, "/library/version/add", &f, NOW, op).unwrap();
    engine::validate_app(app).unwrap();
    (result, f)
}
fn page(app: &App, path: &str) -> String {
    web::render(
        app,
        path,
        &BTreeMap::from([("__nonce".into(), "synthetic-render-nonce".into())]),
        NOW,
        None,
    )
}

#[test]
fn phone_library_entry_supplied_text_and_same_study_use_existing_transitions() {
    let mut app = app();
    let empty = page(&app, "/");
    assert!(empty.contains("data-state=\"library\""));
    assert!(!empty.contains("Make it understandable"));
    assert!(empty.contains("href=\"/notebook\">Understanding</a>"));
    let masthead = empty
        .split("<nav aria-label=\"Main navigation\">")
        .nth(1)
        .unwrap()
        .split("</nav>")
        .next()
        .unwrap();
    assert!(masthead.contains(">Library</a>"));
    assert!(!masthead.contains(">Create</a>") && !masthead.contains(">Map</a>"));
    let form = page(&app, "/library/add");
    for expected in [
        "method=\"post\" action=\"/library/version/add\"",
        "name=\"csrf\"",
        "name=\"operation_id\"",
        "name=\"title\"",
        "name=\"text\"",
        "name=\"source\"",
    ] {
        assert!(form.contains(expected), "{expected}");
    }
    let (saved, f) = add(&mut app, "phone-library-save-0001");
    let vid = saved.entity_id.clone().unwrap();
    let detail_path = format!("/library/versions/{vid}");
    assert_eq!(
        web::mutation_redirect(&app, "/library/version/add", &saved.redirect),
        detail_path
    );
    let before = app.clone();
    assert_eq!(
        engine::mutate(&mut app, "/library/version/add", &f, NOW, "different-seed").unwrap(),
        saved
    );
    assert_eq!(
        app, before,
        "Lost response reuses the complete operation receipt"
    );
    assert_eq!(
        add(&mut app, "phone-library-save-0002").0.entity_id,
        Some(vid.clone())
    );
    assert_eq!(app.library.works.len(), 1);
    assert_eq!(app.library.versions.len(), 1);
    let version = &app.library.versions[&vid];
    assert_eq!(version.rights, scry::library::Rights::PrivateOnly);
    assert_eq!(
        version
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .map(|u| u.text.as_str())
            .collect::<String>(),
        TEXT
    );
    let detail = page(&app, &detail_path);
    for expected in [
        "Garden &lt;&amp;&gt;",
        "Authored edition",
        "Synthetic author",
        "Private only",
        TEXT,
        "action=\"/practice/learn\"",
    ] {
        assert!(detail.contains(expected), "{expected}");
    }
    let learned = action(
        &mut app,
        "/practice/learn",
        "phone-library-learn-01",
        &[("version_id", &vid)],
    );
    assert_eq!(
        action(
            &mut app,
            "/practice/learn",
            "phone-library-learn-02",
            &[("version_id", &vid)]
        )
        .entity_id,
        learned.entity_id
    );
    assert_eq!(app.practice.studies.len(), 1);
    study::ensure_stream(&mut app, NOW, "first-prompt").unwrap();
    assert!(page(&app, "/").contains("data-state=\"exact-prompt\""));
    let pid = app.practice.current.as_ref().unwrap().prompt.id.clone();
    action(
        &mut app,
        "/practice/answer",
        "phone-library-answer01",
        &[("prompt_id", &pid), ("answer", TEXT)],
    );
    let held = app.practice.current.clone();
    let restored = persistence::app_from_rows(&persistence::rows(&app).unwrap()).unwrap();
    assert_eq!(restored.practice.current, held);
    assert!(page(&restored, "/").contains("data-state=\"exact-result\""));
    assert_eq!(app.jobs.len(), 0);
    assert_eq!(app.assessments.len(), 0);
    assert_eq!(app.spend.len(), 0);
}

#[test]
fn phone_library_search_validation_and_understanding_remain_honest() {
    let mut app = app();
    let (saved, _) = add(&mut app, "phone-library-save-0001");
    let vid = saved.entity_id.unwrap();
    let wid = app.library.versions[&vid].work_id.clone();
    let data = serde_json::json!({"work_id":wid,"revision":app.library.works[&wid].revision,"aliases":["Waterside verse"]}).to_string();
    action(
        &mut app,
        "/library/work/edit",
        "phone-library-alias-01",
        &[("data", &data)],
    );
    for query in ["Garden", "Waterside verse", "A patient garden"] {
        let html = web::render(
            &app,
            "/library",
            &BTreeMap::from([("q".into(), query.into())]),
            NOW,
            None,
        );
        assert!(html.contains(&format!("href=\"/library/versions/{vid}\"")));
        assert!(html.contains("Authored edition"));
    }
    let miss = web::render(
        &app,
        "/library",
        &BTreeMap::from([("q".into(), "No such text".into())]),
        NOW,
        None,
    );
    assert!(miss.contains("Nothing matches."));
    assert!(!miss.contains(&vid));
    let before = app.clone();
    let mut bad = fields(
        &app,
        "phone-library-bad-0001",
        &[("title", " "), ("text", TEXT)],
    );
    assert_eq!(api::library_form_fields(&mut bad).unwrap_err().status, 422);
    assert_eq!(app, before);
    let draft = web::render(
        &app,
        web::mutation_error_path(&app, "/library/version/add"),
        &bad,
        NOW,
        Some("Add a title."),
    );
    assert!(draft.contains(TEXT));
    assert!(draft.contains("Add a title."));
    let mut oversized = fields(&app, "phone-library-bad-0002", &[("title", "Title")]);
    oversized.insert("text".into(), "é".repeat(scry::library::TEXT_BYTES));
    assert_eq!(
        api::library_form_fields(&mut oversized).unwrap_err().status,
        422
    );
    assert!(page(&app, "/notebook").contains("href=\"/create\">Create</a>"));
    assert!(page(&app, "/notebook").contains("href=\"/map\">Map</a>"));
}

#[test]
fn phone_library_never_renders_protected_text_before_durable_help() {
    let mut app = app();
    let (saved, _) = add(&mut app, "phone-library-save-0001");
    let vid = saved.entity_id.unwrap();
    action(
        &mut app,
        "/practice/learn",
        "phone-library-learn-01",
        &[("version_id", &vid)],
    );
    study::ensure_stream(&mut app, NOW, "initial-prompt").unwrap();
    let cid = app.practice.current.as_ref().unwrap().targets[0]
        .card_id
        .clone();
    for i in 0..6 {
        let pid = app.practice.current.as_ref().unwrap().prompt.id.clone();
        action(
            &mut app,
            "/practice/answer",
            &format!("phone-library-fade-answer-{i}"),
            &[("prompt_id", &pid), ("answer", TEXT)],
        );
        action(
            &mut app,
            "/practice/next",
            &format!("phone-library-fade-next-{i}"),
            &[("after", &pid), ("card_id", &cid)],
        );
    }
    assert!(app.practice.protected());
    let before = app.clone();
    let detail = format!("/library/versions/{vid}");
    for path in ["/library", "/library/add", &detail] {
        let html = page(&app, path);
        assert!(html.contains("data-state=\"library-gate\""));
        for secret in [
            TEXT,
            "Authored edition",
            "Synthetic author",
            "Garden &lt;&amp;&gt;",
        ] {
            assert!(!html.contains(secret), "{path} leaked {secret}");
        }
        assert!(html.contains("action=\"/practice/reveal\""));
        assert!(html.contains("Open Library with help"));
    }
    assert_eq!(app, before);
    let pid = app.practice.current.as_ref().unwrap().prompt.id.clone();
    let f = fields(
        &app,
        "phone-library-help-0001",
        &[("prompt_id", &pid), ("return_to", &detail)],
    );
    engine::mutate(&mut app, "/practice/reveal", &f, NOW, "help").unwrap();
    assert!(!app.practice.protected());
    assert!(page(&app, &detail).contains(TEXT));
    assert!(
        app.practice
            .attempts
            .values()
            .any(|a| a.disclosure && a.assisted)
    );
    assert_eq!(
        app.practice.cards[&cid].state,
        before.practice.cards[&cid].state
    );
    engine::mutate(
        &mut app,
        "/practice/answer",
        &fields(
            &before,
            "phone-library-answer02",
            &[("prompt_id", &pid), ("answer", TEXT)],
        ),
        NOW,
        "answer",
    )
    .unwrap();
    action(
        &mut app,
        "/practice/next",
        "phone-library-next-002",
        &[("after", &pid), ("card_id", &cid)],
    );
    assert!(app.practice.protected());
    assert_eq!(
        engine::mutate(&mut app, "/practice/reveal", &f, NOW, "replayed-help")
            .unwrap_err()
            .status,
        409
    );
    for unsafe_path in [
        "//evil.invalid/library",
        "https://evil.invalid/library",
        "/library/../export",
        "/library?next=https://evil.invalid",
        "/library/versions/not-an-id",
        "/export",
    ] {
        assert!(!web::library_destination(unsafe_path));
    }
    for safe in ["/library", "/library/add", &detail] {
        assert!(web::library_destination(safe));
    }
}
