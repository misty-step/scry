use scry::{
    api, engine,
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence,
};
use serde_json::{Value, json};
const NOW: i64 = 1_800_000_000_000;
fn app() -> App {
    let mut app = App::new("synthetic-csrf-at-least-24-chars".into());
    let token = OwnerToken::issue(
        "Library builder",
        None,
        false,
        NOW,
        "abcdef0123456789abcdef0123456789abcdef0123456789",
    )
    .unwrap();
    app.owner_tokens.insert(token.info.id.clone(), token);
    app
}
fn draft(title: &str, text: &str) -> Value {
    json!({"title":title,"kind":"text","text":text,"name":"Synthetic wording"})
}
fn mutate(app: &mut App, path: &str, op: &str, mut payload: Value) -> AppResult<String> {
    payload["operation_id"] = json!(op);
    let fields = api::library_fields(payload)?;
    let token = app.owner_tokens.values().next().unwrap().info.clone();
    let path = api::mutation_path(path).unwrap();
    Ok(
        engine::mutate_as(app, path, &fields, NOW, op, MutationActor::Token(&token))?
            .entity_id
            .unwrap(),
    )
}
fn add(app: &mut App, op: &str, data: Value) -> String {
    mutate(app, "/api/v1/library/versions", op, json!({"data":data})).unwrap()
}
#[test]
fn library_search_dedupe_segmentation_and_shared_receipts() {
    let mut app = app();
    assert!(app.library.search("apostles creed").empty);
    let mut data = draft(
        "Apostles' Creed",
        "Synthetic line one,\r\nsecond clause.\n\nLast line.\n",
    );
    data["aliases"] = json!(["Symbolum Apostolorum"]);
    let id = add(&mut app, "library-initial-operation", data.clone());
    let version = &app.library.versions[&id];
    assert_eq!(
        version
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .map(|u| u.text.as_str())
            .collect::<String>(),
        data["text"].as_str().unwrap()
    );
    assert_eq!(version.provenance, scry::library::Provenance::Supplied);
    assert_eq!(version.rights, scry::library::Rights::PrivateOnly);
    for query in ["APOSTLES CREED", "Symbolum Apostolorum", "synthetic line"] {
        assert_eq!(app.library.search(query).matches.len(), 1);
    }
    let snapshot = app.clone();
    assert_eq!(add(&mut app, "library-initial-operation", data.clone()), id);
    assert_eq!(snapshot, app, "Identical operation replay changes nothing.");
    let mut changed = data.clone();
    changed["context"] = json!("changed");
    assert_eq!(
        mutate(
            &mut app,
            "/api/v1/library/versions",
            "library-initial-operation",
            json!({"data":changed})
        )
        .unwrap_err()
        .status,
        409
    );
    for (n, text) in [
        "Synthetic line one,\r\nsecond clause.\n\nLast line.\n",
        "SYNTHETIC LINE ONE SECOND CLAUSE LAST LINE",
        "syntheticlineonesecondclauselastline",
    ]
    .iter()
    .enumerate()
    {
        data["text"] = json!(text);
        assert_eq!(
            add(
                &mut app,
                &format!("library-dedupe-operation-{n}"),
                data.clone()
            ),
            id
        );
        assert_eq!(app.library, snapshot.library);
    }
    let mut second = draft(
        "Symbolum Apostolorum",
        "Synthetic second wording; unchanged meaning is not assumed.\n",
    );
    second["name"] = json!("Another supplied wording");
    let sibling = add(&mut app, "library-alias-operation", second);
    assert_eq!(app.library.works.len(), 1);
    assert_eq!(app.library.versions.len(), 2);
    assert_eq!(
        app.library.versions[&id].work_id,
        app.library.versions[&sibling].work_id
    );
    let before = app.clone();
    let mut bad = draft("New Work", "Untouched bytes.\n");
    bad["sections"] = json!([{"heading":"Bad","units":["Untouched bytes."]}]);
    assert_eq!(
        mutate(
            &mut app,
            "/api/v1/library/versions",
            "library-bad-join-operation",
            json!({"data":bad})
        )
        .unwrap_err()
        .status,
        422
    );
    assert_eq!(
        app, before,
        "Bad join leaves every record and receipt unchanged."
    );
    assert!(app.jobs.is_empty() && app.assessments.is_empty() && app.spend.is_empty());
    assert!(
        app.operations.values().all(
            |r| r.acting_token.as_deref() == app.owner_tokens.keys().next().map(String::as_str)
        )
    );
    engine::validate_app(&app).unwrap();
}
#[test]
fn supersede_retains_original_and_study_reference_refuses_deletion() {
    let mut app = app();
    let old = add(
        &mut app,
        "library-original-operation",
        draft("Private text", "Original synthetic wording.\n"),
    );
    let original = app.library.versions[&old].clone();
    let work = original.work_id.clone();
    // This exercises the durable reference seam only. #317 must create real
    // Studies/Attempts and maintain these references, including after drop.
    app.study_version_refs
        .insert("future-study-reference-fixture".into(), old.clone());
    let refs = app.study_version_refs.clone();
    let new=mutate(&mut app,"/api/v1/library/versions/supersede","library-correction-operation",json!({"version_id":old,"revision":1,"data":draft("Private text","Corrected synthetic wording.\n")})).unwrap();
    assert_ne!(new, old);
    assert_eq!(app.library.versions[&old], original);
    assert_eq!(app.study_version_refs, refs);
    assert!(app.library.show_version(&old).unwrap().relations.iter().any(|r|matches!(r,scry::library::Relation::Supersedes{version_id,previous_id} if version_id==&new&&previous_id==&old)));
    let before = app.clone();
    for (path, payload) in [
        (
            "/api/v1/library/versions/delete",
            json!({"version_id":old,"revision":2}),
        ),
        (
            "/api/v1/library/works/delete",
            json!({"work_id":work,"revision":2}),
        ),
    ] {
        let error = mutate(&mut app, path, "library-used-delete-operation", payload).unwrap_err();
        assert_eq!(error.status, 409);
        assert!(error.message.contains("Study uses"));
        assert_eq!(app, before);
    }
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&app).unwrap()).unwrap(),
        app
    );
    let archive = persistence::Archive::new(app.clone(), vec![], NOW);
    let bytes = archive.encode().unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        app
    );
    let mut broken = app.clone();
    broken.library.versions.get_mut(&old).unwrap().sections[0].units[0]
        .text
        .push('!');
    assert!(engine::validate_app(&broken).is_err());
    assert!(engine::validate_app(&app).is_ok());
}
#[test]
fn unused_crud_fences_names_unicode_and_library_export() {
    let mut app = app();
    let id = add(
        &mut app,
        "library-unicode-operation",
        draft("Ｓｔｒａße’s Prayer", "Café synthetic text."),
    );
    let id2 = add(
        &mut app,
        "library-unicode-dedupe",
        draft("STRASSES PRAYER", "Cafe\u{301} SYNTHETIC TEXT!"),
    );
    assert_eq!(id, id2);
    let work = app.library.versions[&id].work_id.clone();
    mutate(&mut app,"/api/v1/library/works/edit","library-work-edit-operation",json!({"data":{"work_id":work,"revision":1,"title":"Edited title","aliases":["Private alias"],"context":"Private context"}})).unwrap();
    assert_eq!(app.library.search("private alias").matches.len(), 1);
    let before = app.clone();
    assert_eq!(
        mutate(
            &mut app,
            "/api/v1/library/versions/delete",
            "library-stale-delete-operation",
            json!({"version_id":id,"revision":1})
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(before, app);
    let export = api::library_read(&app, "/api/v1/library/export", "").unwrap();
    assert_eq!(
        export
            .as_object()
            .unwrap()
            .keys()
            .map(String::as_str)
            .collect::<Vec<_>>(),
        vec!["relations", "versions", "works"]
    );
    mutate(
        &mut app,
        "/api/v1/library/versions/delete",
        "library-unused-delete-operation",
        json!({"version_id":id,"revision":2}),
    )
    .unwrap();
    assert!(app.library.show_version(&id).is_err());
    assert!(app.library.show_work(&work).unwrap().versions.is_empty());
    mutate(
        &mut app,
        "/api/v1/library/works/delete",
        "library-unused-work-delete",
        json!({"work_id":work,"revision":3}),
    )
    .unwrap();
    assert!(app.library.is_empty());
    let archive = persistence::Archive::new(app.clone(), vec![], NOW);
    assert_eq!(archive.format, "scry-rust-durable-sqlite-library-v1");
    let bytes = archive.encode().unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        app
    );
    assert_eq!(
        add(
            &mut app,
            "library-unicode-operation",
            draft("Ｓｔｒａße’s Prayer", "Café synthetic text.")
        ),
        id
    );
    assert!(
        app.library.is_empty(),
        "Replaying a deleted add returns its receipt without resurrecting content."
    );
    let a = add(
        &mut app,
        "library-name-a-operation",
        draft("Name A", "Distinct text A."),
    );
    let b = add(
        &mut app,
        "library-name-b-operation",
        draft("Name B", "Distinct text B."),
    );
    let wa = app.library.versions[&a].work_id.clone();
    let wb = app.library.versions[&b].work_id.clone();
    let before = app.clone();
    assert_eq!(
        mutate(
            &mut app,
            "/api/v1/library/works/edit",
            "library-name-collision-op",
            json!({"data":{"work_id":wa,"revision":1,"aliases":["Name B"]}})
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, before);
    let mut collision = draft("Name A", "Distinct text C.");
    collision["aliases"] = json!(["Name B"]);
    assert_eq!(
        mutate(
            &mut app,
            "/api/v1/library/versions",
            "library-ambiguous-operation",
            json!({"data":collision})
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, before);
    assert!(app.library.works.contains_key(&wb));
}
#[test]
fn empty_library_preserves_legacy_application_rows_and_archive_shape() {
    let app = App::new("synthetic-csrf-at-least-24-chars".into());
    let value = serde_json::to_value(&app).unwrap();
    assert!(value.get("library").is_none());
    assert!(value.get("study_version_refs").is_none());
    let rows = persistence::rows(&app).unwrap();
    assert!(
        rows.keys()
            .all(|k| !k.starts_with("library_") && !k.starts_with("study_version_refs/"))
    );
    assert_eq!(
        persistence::rows(&persistence::app_from_rows(&rows).unwrap()).unwrap(),
        rows
    );
    let archive = persistence::Archive::new(app.clone(), vec![], NOW);
    let bytes = archive.encode().unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .encode()
            .unwrap(),
        bytes
    );
}
