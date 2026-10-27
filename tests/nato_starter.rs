//! Explicit bundled-content import is the same shared Library mutation, never a read side effect.
use scry::{api, engine, library, model::*, persistence, study, web};
use serde_json::json;
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
fn app() -> App {
    App::new("synthetic-nato-starter-csrf-32-chars".into())
}
fn fields(app: &App, op: &str) -> BTreeMap<String, String> {
    BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), op.into()),
        ("revision".into(), app.revision.to_string()),
        ("starter".into(), "nato-v1".into()),
    ])
}
fn page(app: &App, path: &str) -> String {
    web::render(
        app,
        path,
        &BTreeMap::from([("__nonce".into(), "synthetic-starter-render".into())]),
        NOW,
        None,
    )
}
fn form_fields(html: &str, action: &str) -> BTreeMap<String, String> {
    let form = html
        .split("<form")
        .find(|s| s.starts_with(&format!(" method=\"post\" action=\"{action}\"")))
        .unwrap()
        .split("</form>")
        .next()
        .unwrap();
    form.split("<input")
        .skip(1)
        .filter_map(|input| {
            let input = input.split('>').next().unwrap();
            if !input.contains("type=\"hidden\"") {
                return None;
            }
            let attr = |name: &str| {
                input
                    .split(&format!("{name}=\""))
                    .nth(1)
                    .unwrap()
                    .split('"')
                    .next()
                    .unwrap()
                    .to_string()
            };
            Some((attr("name"), attr("value")))
        })
        .collect()
}
#[test]
fn library_starter_native_form_imports_canonical_content_only_on_explicit_action() {
    assert_eq!(
        persistence::sha256(include_bytes!("../examples/nato-alphabet.json")),
        "620bf6ccc5f1ae807e6d744a00f1c5a63651a31a45c9ee6fa7d3c1f0ff9d5369",
        "Changed starter content requires a new selector, not silent nato-v1 drift"
    );
    let mut app = app();
    let before = app.clone();
    let html = page(&app, "/library");
    assert!(html.contains("Add NATO alphabet"));
    assert!(!html.contains("Alfa"));
    assert!(!html.contains("Bravo"));
    assert_eq!(app, before);
    let form = form_fields(&html, "/library/mapping/add");
    assert_eq!(form["starter"], "nato-v1");
    assert!(form.contains_key("revision"));
    assert!(!form.contains_key("data"));
    let result =
        engine::mutate(&mut app, "/library/mapping/add", &form, NOW, "starter-form").unwrap();
    engine::validate_app(&app).unwrap();
    let vid = result.entity_id.as_ref().unwrap();
    let v = &app.library.versions[vid];
    assert_eq!(v.kind, library::Kind::KeyedMapping);
    assert!(v.spelling_strict);
    assert_eq!(v.sections.iter().flat_map(|s| &s.units).count(), 26);
    assert!(v.text.contains("A\tAlfa\n"));
    assert!(v.text.contains("J\tJuliett\n"));
    assert!(v.text.contains("W\tWhiskey\n"));
    assert!(v.text.contains("X\tX-ray\n"));
    assert!(app.practice.is_empty());
    assert!(app.jobs.is_empty() && app.spend.is_empty());
    let destination = format!("/library/versions/{vid}");
    assert_eq!(
        web::mutation_redirect(&app, "/library/mapping/add", &result.redirect),
        destination
    );
    assert!(page(&app, &destination).contains("action=\"/practice/learn\""));
    let saved = app.clone();
    assert_eq!(
        engine::mutate(&mut app, "/library/mapping/add", &form, NOW, "repeat").unwrap(),
        result
    );
    assert_eq!(app, saved);
    let bytes = persistence::Archive::new(app.clone(), vec![], NOW)
        .encode()
        .unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        app
    );
}
#[test]
fn starter_selector_is_allowlisted_exclusive_and_cannot_override_supplied_words() {
    for (key, value) in [
        ("starter", "nato-v2"),
        ("starter", "../examples/nato-alphabet.json"),
        ("data", "{}"),
        ("answer", "Alpha"),
        ("generate", "true"),
        ("grade", "Good"),
    ] {
        let mut app = app();
        let mut f = fields(&app, "starter-invalid-0001");
        f.insert(key.into(), value.into());
        let before = app.clone();
        assert_eq!(
            engine::mutate(&mut app, "/library/mapping/add", &f, NOW, "invalid")
                .unwrap_err()
                .status,
            422
        );
        assert_eq!(app, before);
    }
    let mut app = app();
    let mut f = fields(&app, "starter-no-csrf-0001");
    f.remove("csrf");
    let before = app.clone();
    assert_eq!(
        engine::mutate(&mut app, "/library/mapping/add", &f, NOW, "invalid")
            .unwrap_err()
            .status,
        403
    );
    assert_eq!(app, before);
}
#[test]
fn repeated_starter_and_agent_manifest_preserve_same_study_and_schedule() {
    let mut app = app();
    let f = fields(&app, "starter-initial-0001");
    let vid = engine::mutate(&mut app, "/library/mapping/add", &f, NOW, "starter")
        .unwrap()
        .entity_id
        .unwrap();
    let mut call = |path: &str, data: serde_json::Value, seed: &str| {
        let mut f = api::library_fields(data).unwrap();
        f.insert("csrf".into(), app.csrf.clone());
        engine::mutate(&mut app, path, &f, NOW, seed).unwrap()
    };
    call(
        "/practice/learn",
        json!({"operation_id":"starter-learn-0001","version_id":vid}),
        "learn",
    );
    call(
        "/practice/next",
        json!({"operation_id":"starter-next-0001"}),
        "next",
    );
    let pid = app.practice.current.as_ref().unwrap().prompt.id.clone();
    let f=api::fields(json!({"csrf":app.csrf,"operation_id":"starter-answer-0001","prompt_id":pid,"answer":"Alfa"})).unwrap();
    let result = engine::mutate(&mut app, "/practice/answer", &f, NOW, "answer").unwrap();
    assert_eq!(
        study::wire_response(&app, &result).unwrap()["attempt"]["grade"]["verdict"],
        "Good"
    );
    let practice = app.practice.clone();
    let library = app.library.clone();
    let f = fields(&app, "starter-second-0001");
    assert_eq!(
        engine::mutate(&mut app, "/library/mapping/add", &f, NOW, "second")
            .unwrap()
            .entity_id
            .as_deref(),
        Some(vid.as_str())
    );
    let manifest: serde_json::Value =
        serde_json::from_str(include_str!("../examples/nato-alphabet.json")).unwrap();
    let mut f =
        api::library_fields(json!({"operation_id":"starter-agent-0001","data":manifest})).unwrap();
    f.insert("csrf".into(), app.csrf.clone());
    assert_eq!(
        engine::mutate(&mut app, "/library/mapping/add", &f, NOW, "agent")
            .unwrap()
            .entity_id
            .as_deref(),
        Some(vid.as_str())
    );
    assert_eq!(app.library, library);
    assert_eq!(app.practice, practice);
    engine::validate_app(&app).unwrap();
}
