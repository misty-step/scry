use scry::{api, engine, model::*, web};
use std::collections::BTreeMap;

const NOW: i64 = 1_791_410_400_000;
fn fields(operation: &str, entries: &[(&str, &str)]) -> BTreeMap<String, String> {
    let mut values: BTreeMap<_, _> = entries
        .iter()
        .map(|(k, v)| ((*k).into(), (*v).into()))
        .collect();
    values.insert("operation_id".into(), operation.into());
    values.insert(
        "csrf".into(),
        "synthetic-material-ui-csrf-at-least-32".into(),
    );
    values
}
fn authored() -> (App, String, String) {
    let mut app = App::new("synthetic-material-ui-csrf-at-least-32".into());
    let values = fields(
        "material-ui-create-operation",
        &[
            ("name", "Conditional requests"),
            ("title", "First <reference>"),
            (
                "text",
                "\n\tOriginal café\r\nhttps://literal.invalid/path\n",
            ),
        ],
    );
    let result = engine::mutate(&mut app, "/concepts/create", &values, NOW, "ui-create").unwrap();
    let concept = result.entity_id.unwrap();
    let material = app.materials.items.keys().next().unwrap().clone();
    (app, concept, material)
}
#[test]
fn versioned_reads_expose_retained_graph_without_writes_or_wrong_material_scope() {
    let (mut app, concept, first) = authored();
    let result = engine::mutate(
        &mut app,
        "/materials/add",
        &fields(
            "material-ui-add-operation",
            &[
                ("concept_id", &concept),
                ("title", "Second"),
                ("text", "Independent"),
            ],
        ),
        NOW + 1,
        "ui-add",
    )
    .unwrap();
    let second = result.entity_id.unwrap();
    let revision = app.materials.items[&first].current_revision_id.clone();
    let before = app.clone();
    let concepts = api::material_read(&app, "/api/v1/concepts").unwrap();
    assert_eq!(concepts["concepts"].as_array().unwrap().len(), 1);
    let detail = api::material_read(&app, &format!("/api/v1/concepts/{concept}")).unwrap();
    assert_eq!(detail["materials"].as_array().unwrap().len(), 2);
    for path in [
        format!("/api/v1/materials/{first}"),
        format!("/api/v1/materials/{first}/revisions"),
        format!("/api/v1/materials/{first}/revisions/{revision}"),
        format!("/api/v1/concepts/{concept}/materials"),
    ] {
        assert!(api::material_read(&app, &path).is_ok());
    }
    assert_eq!(
        api::material_read(
            &app,
            &format!("/api/v1/materials/{second}/revisions/{revision}")
        )
        .unwrap_err()
        .status,
        404
    );
    assert_eq!(app, before);
}
#[test]
fn native_create_is_free_explicit_and_error_draft_preserves_operation_and_wording() {
    let app = App::new("synthetic-material-ui-csrf-at-least-32".into());
    let home = web::render(&app, "/", &BTreeMap::new(), NOW, None);
    assert!(home.contains("href=\"/concepts/new\"") && home.contains("Create concept"));
    assert!(home.contains("Prepare material with AI"));
    assert!(!home.contains("Start practice"));
    let mut query = BTreeMap::from([
        ("name".into(), "<draft name>".into()),
        ("title".into(), "<draft title>".into()),
        (
            "text".into(),
            "\nDraft\r\n</textarea><script>bad</script>".into(),
        ),
        (
            "__draft_operation".into(),
            "material-error-draft-operation".into(),
        ),
    ]);
    for paused in [false, true] {
        let mut app = app.clone();
        app.restored_paused = paused;
        let html = web::render(&app, "/concepts/create", &query, NOW, Some("Invalid title"));
        assert!(html.contains("action=\"/concepts/create\""));
        assert!(html.contains("name=\"operation_id\" value=\"material-error-draft-operation\""));
        assert!(!html.contains("name=\"revision\""));
        assert!(
            html.contains("&lt;draft name&gt;") && html.contains("&lt;/textarea&gt;&lt;script&gt;")
        );
        assert!(
            html.contains(">Save</button>")
                && html.contains(">Cancel</a>")
                && html.contains("← Back")
        );
        assert!(!html.contains("type=\"file\"") && !html.contains("<script>bad"));
    }
    query.clear();
    let html = web::render(&app, "/concepts/new", &query, NOW, None);
    assert!(html.contains("name=\"text\"") && html.contains("No AI or paid work"));
}
#[test]
fn detail_history_map_and_stale_revision_editor_use_same_real_material_ids() {
    let (mut app, concept, material) = authored();
    let previous = app.materials.items[&material].current_revision_id.clone();
    engine::mutate(
        &mut app,
        "/materials/revise",
        &fields(
            "material-ui-revise-operation",
            &[
                ("material_id", &material),
                ("expected_revision_id", &previous),
                ("title", "Revised"),
                ("text", "New exact wording"),
            ],
        ),
        NOW + 1,
        "ui-revise",
    )
    .unwrap();
    let before = app.clone();
    let html = web::render(
        &app,
        &format!("/concepts/{concept}"),
        &BTreeMap::new(),
        NOW,
        None,
    );
    assert!(html.contains("<h2>Materials</h2>") && html.contains("Earlier revisions"));
    assert!(html.contains(">Add study material</a>"));
    assert!(html.contains("No quizzes/questions yet.") && !html.contains("Practice this idea"));
    assert!(html.contains(&format!("/materials/{material}/edit")));
    assert!(!html.contains("Saved explanation"));
    let older = web::render(
        &app,
        &format!("/materials/{material}/revisions/{previous}"),
        &BTreeMap::new(),
        NOW,
        None,
    );
    assert!(older.contains("Original café") && older.contains("Revision history"));
    let query = BTreeMap::from([
        ("material_id".into(), material.clone()),
        ("expected_revision_id".into(), previous.clone()),
        ("title".into(), "Unsent revision".into()),
        ("text".into(), "Unsent text".into()),
    ]);
    let stale = web::render(&app, "/materials/revise", &query, NOW, Some("Conflict"));
    assert!(stale.contains(&format!(
        "name=\"expected_revision_id\" value=\"{previous}\""
    )));
    assert!(stale.contains("Unsent text"));
    assert!(
        web::render(&app, "/map", &BTreeMap::new(), NOW, None)
            .contains(&format!("/concepts/{concept}"))
    );
    assert_eq!(app, before);
}
#[test]
fn current_concept_reference_fence_covers_api_materials_and_old_revisions() {
    let mut app = App::new("synthetic-material-ui-csrf-at-least-32".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    engine::ensure_occurrence(&mut app, NOW, "protected-ui").unwrap();
    let concept = app.occurrence.as_ref().unwrap().concept_id.clone();
    app.occurrence.as_mut().unwrap().assisted = true;
    let material = engine::mutate(
        &mut app,
        "/materials/add",
        &fields(
            "material-ui-protected-add",
            &[
                ("concept_id", &concept),
                ("title", "PRIVATE_TITLE"),
                ("text", "PRIVATE_MATERIAL_TEXT"),
            ],
        ),
        NOW + 1,
        "protected-material",
    )
    .unwrap()
    .entity_id
    .unwrap();
    app.occurrence.as_mut().unwrap().assisted = false;
    let revision = app.materials.items[&material].current_revision_id.clone();
    let before = app.clone();
    for path in [
        format!("/api/v1/concepts/{concept}"),
        format!("/api/v1/concepts/{concept}/materials"),
        format!("/api/v1/materials/{material}"),
        format!("/api/v1/materials/{material}/revisions/{revision}"),
    ] {
        assert_eq!(api::material_read(&app, &path).unwrap_err().status, 409);
    }
    for path in [
        format!("/concepts/{concept}"),
        format!("/materials/{material}"),
        format!("/materials/{material}/edit"),
        format!("/materials/{material}/revisions/{revision}"),
    ] {
        let html = web::render(&app, &path, &BTreeMap::new(), NOW, None);
        assert!(!html.contains("PRIVATE_TITLE") && !html.contains("PRIVATE_MATERIAL_TEXT"));
        assert!(html.contains("data-state=\"gate\""));
    }
    assert_eq!(app, before);
}
