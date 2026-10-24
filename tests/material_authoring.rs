#[path = "support/materials.rs"]
#[allow(dead_code)]
mod support;

use scry::{
    engine,
    materials::{Material, MaterialProvenance, MaterialRevision},
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence::{self, Archive},
};
use std::collections::BTreeMap;
use support::NOW;

type Fields = BTreeMap<String, String>;

fn empty() -> App {
    App::new("synthetic-authoring-csrf-at-least-32-characters".into())
}
fn fields(operation: &str, values: &[(&str, &str)]) -> Fields {
    let mut fields = BTreeMap::from([("operation_id".into(), operation.into())]);
    fields.extend(
        values
            .iter()
            .map(|(key, value)| ((*key).into(), (*value).into())),
    );
    fields
}
fn create_fields(operation: &str) -> Fields {
    fields(
        operation,
        &[
            ("name", "  Café concepts  "),
            ("title", "  Reference title  "),
            (
                "text",
                "  Supplied café 💡\r\n\tLiteral https://example.invalid/source\n",
            ),
        ],
    )
}
fn call(app: &mut App, path: &str, fields: &Fields, now: i64) -> AppResult<MutationResult> {
    engine::mutate_as(
        app,
        path,
        fields,
        now,
        fields
            .get("operation_id")
            .map(String::as_str)
            .unwrap_or("missing-operation-seed"),
        MutationActor::Browser,
    )
}
fn create(app: &mut App, operation: &str) -> (String, String, String) {
    let result = call(app, "/concepts/create", &create_fields(operation), NOW).unwrap();
    let concept_id = result.entity_id.unwrap();
    let material = app
        .materials
        .items
        .values()
        .find(|m| m.concept_id == concept_id)
        .unwrap();
    (
        concept_id,
        material.id.clone(),
        material.current_revision_id.clone(),
    )
}
fn refused(app: &mut App, path: &str, fields: &Fields, now: i64, status: u16) {
    let before = app.clone();
    let before_bytes = serde_json::to_vec(app).unwrap();
    assert_eq!(call(app, path, fields, now).unwrap_err().status, status);
    assert_eq!(*app, before, "Refusal changed App for {path}");
    assert_eq!(serde_json::to_vec(app).unwrap(), before_bytes);
}
fn assert_roundtrip(app: &App, photos: Vec<persistence::ArchivedPhoto>) {
    engine::validate_app(app).unwrap();
    let rows = persistence::rows(app).unwrap();
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), *app);
    let bytes = Archive::new(app.clone(), photos, NOW + 5_000)
        .encode()
        .unwrap();
    let restored = Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(restored.app, *app);
    assert_eq!(restored.encode().unwrap(), bytes);
}

#[test]
fn create_is_atomic_exact_owner_authorship_without_learning_or_external_work() {
    let mut app = empty();
    let mut input = create_fields("authoring-create-0001");
    input.insert("summary".into(), "  Optional summary\r\n".into());
    input.insert("csrf".into(), app.csrf.clone());
    let response =
        engine::mutate(&mut app, "/concepts/create", &input, NOW, "create-seed").unwrap();
    let concept = &app.concepts[response.entity_id.as_ref().unwrap()];
    assert_eq!(response.redirect, format!("/concepts/{}", concept.id));
    assert!(response.data.is_none() && response.message.is_none());
    assert_eq!(concept.origin, Some(ConceptOrigin::OwnerAuthored));
    assert_eq!(concept.name, input["name"]);
    assert_eq!(concept.summary, input["summary"]);
    assert!(
        concept.goal_id.is_none() && concept.notes.is_empty() && concept.prerequisites.is_empty()
    );
    assert!(concept.introduced_ms.is_none() && concept.exposure_ms.is_none());
    assert!(!concept.already_known && !concept.archived);
    assert_eq!(
        (app.materials.items.len(), app.materials.revisions.len()),
        (1, 1)
    );
    let material = app.materials.items.values().next().unwrap();
    let revision = &app.materials.revisions[&material.current_revision_id];
    assert_eq!(material.concept_id, concept.id);
    assert_eq!(revision.material_id, material.id);
    assert_eq!(revision.provenance, MaterialProvenance::OwnerSupplied);
    assert_eq!(
        (revision.ordinal, revision.predecessor_id.as_ref()),
        (1, None)
    );
    assert_eq!(revision.title.as_bytes(), input["title"].as_bytes());
    assert_eq!(revision.text.as_bytes(), input["text"].as_bytes());
    assert_eq!((material.created_ms, revision.created_ms), (NOW, NOW));
    assert!(app.materials.capability);
    assert_eq!((app.operations.len(), app.revision), (1, 1));
    assert_eq!(app.operations[&input["operation_id"]].result, response);
    let mut only_authorship = app.clone();
    only_authorship.concepts.clear();
    only_authorship.materials = Default::default();
    only_authorship.operations.clear();
    only_authorship.revision = 0;
    assert_eq!(
        only_authorship,
        empty(),
        "Authoring must not create learning/provider state"
    );
    assert_roundtrip(&app, vec![]);
}

#[test]
fn add_is_independent_even_for_duplicate_title_and_text_and_revision_is_immutable() {
    let mut app = empty();
    let (concept, first_material, first_revision) = create(&mut app, "authoring-duplicate-create");
    let original = app.materials.revisions[&first_revision].clone();
    let addition = fields(
        "authoring-duplicate-add",
        &[
            ("concept_id", &concept),
            ("title", &original.title),
            ("text", &original.text),
        ],
    );
    let added = call(&mut app, "/materials/add", &addition, NOW + 1).unwrap();
    let second_material = added.entity_id.unwrap();
    assert_ne!(first_material, second_material);
    assert_eq!(app.materials.items.len(), 2);
    assert_eq!(app.materials.revisions.len(), 2);
    let second_revision = &app.materials.items[&second_material].current_revision_id;
    assert_ne!(&first_revision, second_revision);
    assert_eq!(app.materials.revisions[second_revision].text, original.text);
    let revision_fields = fields(
        "authoring-revise-0001",
        &[
            ("material_id", &first_material),
            ("expected_revision_id", &first_revision),
            ("title", "  Revised title  "),
            ("text", "  New\r\n\twording: e\u{301} ☕\n\n"),
        ],
    );
    let revised = call(&mut app, "/materials/revise", &revision_fields, NOW + 2).unwrap();
    let current_id = revised.entity_id.as_ref().unwrap();
    let current = &app.materials.revisions[current_id];
    assert_eq!(
        current.predecessor_id.as_deref(),
        Some(first_revision.as_str())
    );
    assert_eq!((current.ordinal, current.created_ms), (2, NOW + 2));
    assert_eq!(current.text, revision_fields["text"]);
    assert_eq!(
        &app.materials.items[&first_material].current_revision_id,
        current_id
    );
    assert_eq!(app.materials.revisions[&first_revision], original);
    assert_eq!(app.materials.items[&first_material].created_ms, NOW);
    assert_eq!(revised.redirect, format!("/concepts/{concept}"));
    assert!(revised.data.is_none());
    assert_roundtrip(&app, vec![]);
}

#[test]
fn replay_conflict_and_stale_revision_race_never_partially_write() {
    let mut app = empty();
    let create_input = create_fields("authoring-replay-create");
    let created = call(&mut app, "/concepts/create", &create_input, NOW).unwrap();
    let concept = created.entity_id.as_ref().unwrap().clone();
    let material = app.materials.items.values().next().unwrap().clone();
    let add_input = fields(
        "authoring-replay-add",
        &[
            ("concept_id", &concept),
            ("title", "Duplicate"),
            ("text", "Duplicate"),
        ],
    );
    let added = call(&mut app, "/materials/add", &add_input, NOW + 1).unwrap();
    let revise_input = fields(
        "authoring-replay-revise",
        &[
            ("material_id", &material.id),
            ("expected_revision_id", &material.current_revision_id),
            ("title", "Revision"),
            ("text", "Revision"),
        ],
    );
    let revised = call(&mut app, "/materials/revise", &revise_input, NOW + 2).unwrap();
    let committed = app.clone();
    for (path, input, expected) in [
        ("/concepts/create", &create_input, &created),
        ("/materials/add", &add_input, &added),
        ("/materials/revise", &revise_input, &revised),
    ] {
        let mut refreshed = input.clone();
        refreshed.insert("csrf".into(), "refreshed-session".into());
        let replay = engine::mutate_as(
            &mut app,
            path,
            &refreshed,
            NOW - 10,
            "different-seed",
            MutationActor::Browser,
        )
        .unwrap();
        assert_eq!(&replay, expected);
        assert_eq!(app, committed);
        refreshed.insert("text".into(), "Changed operation payload".into());
        refused(&mut app, path, &refreshed, NOW + 4, 409);
    }
    let mut stale = revise_input.clone();
    stale.insert("operation_id".into(), "authoring-racing-revise".into());
    refused(&mut app, "/materials/revise", &stale, NOW + 3, 409);
    let mut backward = stale.clone();
    backward.insert("expected_revision_id".into(), revised.entity_id.unwrap());
    refused(&mut app, "/materials/revise", &backward, NOW + 1, 409);
    call(&mut app, "/materials/revise", &backward, NOW + 2).unwrap();
    assert_eq!(
        app.materials.revisions.len(),
        committed.materials.revisions.len() + 1
    );
    assert_roundtrip(&app, vec![]);
}

#[test]
fn fields_and_utf8_byte_limits_are_strict_without_storage_normalization() {
    for (field, value) in [
        ("name", "".into()),
        ("name", " \t\r\n".into()),
        ("name", "é".repeat(101)),
        ("title", "💡".repeat(51)),
        ("title", "".into()),
        ("text", "".into()),
        ("text", "\r\n\t ".into()),
        ("text", "x\0y".into()),
        ("text", "x\u{7f}y".into()),
        ("text", "é".repeat(TEXT_LIMIT / 2 + 1)),
        ("summary", "é".repeat(1001)),
        ("summary", "  ".into()),
        ("source_url", "https://example.invalid".into()),
        ("goal_id", "fake".into()),
    ] {
        let mut app = empty();
        let mut input = create_fields("authoring-invalid-field");
        input.insert(field.into(), value);
        refused(&mut app, "/concepts/create", &input, NOW, 422);
    }
    for missing in ["operation_id", "name", "title", "text"] {
        let mut app = empty();
        let mut input = create_fields("authoring-missing-field");
        input.remove(missing);
        refused(&mut app, "/concepts/create", &input, NOW, 422);
    }
    let mut app = empty();
    let input = fields(
        "authoring-exact-limits",
        &[
            ("name", &"é".repeat(100)),
            ("title", &"💡".repeat(50)),
            ("summary", &"é".repeat(1000)),
            (
                "text",
                &format!("{}\r\n\t\n", "é".repeat((TEXT_LIMIT - 4) / 2)),
            ),
        ],
    );
    call(&mut app, "/concepts/create", &input, NOW).unwrap();
    assert_eq!(
        app.materials.revisions.values().next().unwrap().text.len(),
        TEXT_LIMIT
    );
    assert_eq!(
        app.materials.revisions.values().next().unwrap().text,
        input["text"]
    );
    let (concept, material, revision) = create(&mut app, "authoring-strict-existing");
    for (path, mut input) in [
        (
            "/materials/add",
            fields(
                "authoring-add-unknown",
                &[("concept_id", &concept), ("title", "T"), ("text", "T")],
            ),
        ),
        (
            "/materials/revise",
            fields(
                "authoring-revise-unknown",
                &[
                    ("material_id", &material),
                    ("expected_revision_id", &revision),
                    ("title", "T"),
                    ("text", "T"),
                ],
            ),
        ),
    ] {
        input.insert("name".into(), "No new field".into());
        refused(&mut app, path, &input, NOW, 422);
    }
    assert_roundtrip(&app, vec![]);
}

#[test]
fn collisions_and_invalid_complete_candidate_or_receipt_cannot_commit() {
    let mut app = empty();
    let first = create_fields("authoring-collision-first");
    engine::mutate_as(
        &mut app,
        "/concepts/create",
        &first,
        NOW,
        "same-seed",
        MutationActor::Browser,
    )
    .unwrap();
    let before = app.clone();
    let second = create_fields("authoring-collision-second");
    assert_eq!(
        engine::mutate_as(
            &mut app,
            "/concepts/create",
            &second,
            NOW + 1,
            "same-seed",
            MutationActor::Browser
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, before);
    let concept = app.concepts.keys().next().unwrap().clone();
    let addition = fields(
        "authoring-collision-add",
        &[("concept_id", &concept), ("title", "T"), ("text", "T")],
    );
    assert_eq!(
        engine::mutate_as(
            &mut app,
            "/materials/add",
            &addition,
            NOW + 1,
            "same-seed",
            MutationActor::Browser
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, before);
    let token = OwnerToken::issue(
        "Not retained",
        None,
        false,
        NOW,
        "abcdef0123456789abcdef0123456789abcdef0123456789",
    )
    .unwrap();
    assert_eq!(
        engine::mutate_as(
            &mut app,
            "/concepts/create",
            &second,
            NOW,
            "new-seed",
            MutationActor::Token(&token.info)
        )
        .unwrap_err()
        .status,
        422
    );
    assert_eq!(
        app, before,
        "The receipt's missing acting token must prevent commit"
    );
    app.csrf = "invalid".into();
    refused(&mut app, "/concepts/create", &second, NOW, 422);
    app = before;
    app.revision = u64::MAX;
    refused(&mut app, "/concepts/create", &second, NOW, 409);
}

#[test]
fn unpaid_restored_authoring_preserves_every_old_row_and_complete_history() {
    let fixture = support::material_fixture().unwrap();
    let mut app = fixture.baseline;
    engine::pause_for_restore(&mut app, NOW + 1_501).unwrap();
    assert!(app.restored_paused);
    let before = app.clone();
    let before_rows = persistence::rows(&before).unwrap();
    let token = app.owner_tokens.values().next().unwrap().info.clone();
    assert!(!token.paid);
    let mut operations = vec![];
    let first = create_fields("authoring-unpaid-create");
    let created = engine::mutate_as(
        &mut app,
        "/concepts/create",
        &first,
        NOW + 2_000,
        "unpaid-create",
        MutationActor::Token(&token),
    )
    .unwrap();
    operations.push(first["operation_id"].clone());
    let concept = created.entity_id.unwrap();
    let material = app
        .materials
        .items
        .values()
        .find(|m| m.concept_id == concept)
        .unwrap()
        .clone();
    let generated = before.concepts.keys().next().unwrap();
    for (path, input) in [
        (
            "/materials/add",
            fields(
                "authoring-unpaid-add",
                &[
                    ("concept_id", generated),
                    ("title", "Legacy reference"),
                    ("text", "No copied Notes"),
                ],
            ),
        ),
        (
            "/materials/revise",
            fields(
                "authoring-unpaid-revise",
                &[
                    ("material_id", &material.id),
                    ("expected_revision_id", &material.current_revision_id),
                    ("title", "New revision"),
                    ("text", "Exact\r\n\twording\n"),
                ],
            ),
        ),
    ] {
        engine::mutate_as(
            &mut app,
            path,
            &input,
            NOW + 2_001,
            &input["operation_id"],
            MutationActor::Token(&token),
        )
        .unwrap();
        operations.push(input["operation_id"].clone());
    }
    assert!(app.restored_paused);
    let after_rows = persistence::rows(&app).unwrap();
    for (key, bytes) in &before_rows {
        if key != "meta" {
            assert_eq!(
                after_rows.get(key),
                Some(bytes),
                "Rewrote historical row {key}"
            );
        }
    }
    for operation in operations {
        assert_eq!(
            app.operations[&operation].acting_token.as_deref(),
            Some(token.id.as_str())
        );
        assert!(app.operations[&operation].result.data.is_none());
    }
    let mut old_projection = app.clone();
    old_projection.materials = before.materials.clone();
    old_projection.concepts = before.concepts.clone();
    old_projection.operations = before.operations.clone();
    old_projection.revision = before.revision;
    assert_eq!(old_projection, before);
    assert_roundtrip(&app, fixture.photos);
}

#[test]
fn exact_protection_blocks_new_authoring_and_old_authoring_receipts_without_reveal() {
    let fixture = support::material_fixture().unwrap();
    let mut app = fixture.baseline;
    let create_input = create_fields("authoring-protected-create");
    let created = call(&mut app, "/concepts/create", &create_input, NOW + 1_600).unwrap();
    let concept = created.entity_id.unwrap();
    let first = app
        .materials
        .items
        .values()
        .find(|m| m.concept_id == concept)
        .unwrap()
        .clone();
    let add_input = fields(
        "authoring-protected-add",
        &[("concept_id", &concept), ("title", "T"), ("text", "T")],
    );
    call(&mut app, "/materials/add", &add_input, NOW + 1_601).unwrap();
    let revise_input = fields(
        "authoring-protected-revise",
        &[
            ("material_id", &first.id),
            ("expected_revision_id", &first.current_revision_id),
            ("title", "R"),
            ("text", "R"),
        ],
    );
    call(&mut app, "/materials/revise", &revise_input, NOW + 1_602).unwrap();
    let current = app.practice.current.as_ref().unwrap();
    let next = fields(
        "authoring-protected-next",
        &[
            ("after", &current.prompt.id),
            ("card_id", &current.targets[0].card_id),
        ],
    );
    call(&mut app, "/practice/next", &next, NOW + 2_000).unwrap();
    assert!(app.practice.protected());
    engine::validate_app(&app).unwrap();
    for (path, input) in [
        ("/concepts/create", create_input),
        ("/materials/add", add_input),
        ("/materials/revise", revise_input),
    ] {
        refused(&mut app, path, &input, NOW + 2_001, 409);
        let mut fresh = input;
        fresh.insert(
            "operation_id".into(),
            format!("{}-fresh", fresh["operation_id"]),
        );
        refused(&mut app, path, &fresh, NOW + 2_002, 409);
    }
    assert!(!app.practice.current.as_ref().unwrap().revealed);
    assert!(!app.practice.current.as_ref().unwrap().prompt.assisted);
}

#[test]
fn current_concept_protection_is_targeted_and_rechecked_before_replay() {
    let mut app = empty();
    engine::seed_fixture(&mut app, NOW).unwrap();
    let target = app.occurrence.as_ref().unwrap().concept_id.clone();
    assert!(app.protected_concept(&target));
    let input = fields(
        "authoring-target-add",
        &[
            ("concept_id", &target),
            ("title", "Reference"),
            ("text", "Reference"),
        ],
    );
    refused(&mut app, "/materials/add", &input, NOW + 1, 409);
    // The current question does not block authoring an unrelated Concept.
    let (other, other_material, other_revision) = create(&mut app, "authoring-unrelated-create");
    assert_ne!(other, target);
    let other_input = fields(
        "authoring-unrelated-revise",
        &[
            ("material_id", &other_material),
            ("expected_revision_id", &other_revision),
            ("title", "Unrelated"),
            ("text", "Unrelated"),
        ],
    );
    call(&mut app, "/materials/revise", &other_input, NOW + 1).unwrap();
    // Save references outside the protected occurrence, then restore the same
    // valid frozen occurrence to model a later protected question in another tab.
    let occurrence = app.occurrence.take();
    let added = call(&mut app, "/materials/add", &input, NOW + 2).unwrap();
    let material = added.entity_id.unwrap();
    let revision = app.materials.items[&material].current_revision_id.clone();
    let revise = fields(
        "authoring-target-revise",
        &[
            ("material_id", &material),
            ("expected_revision_id", &revision),
            ("title", "Revised"),
            ("text", "Revised"),
        ],
    );
    call(&mut app, "/materials/revise", &revise, NOW + 3).unwrap();
    app.occurrence = occurrence;
    engine::validate_app(&app).unwrap();
    for (path, input) in [("/materials/add", input), ("/materials/revise", revise)] {
        refused(&mut app, path, &input, NOW + 4, 409);
        let mut fresh = input;
        fresh.insert(
            "operation_id".into(),
            format!("{}-new", fresh["operation_id"]),
        );
        refused(&mut app, path, &fresh, NOW + 4, 409);
    }
    assert!(!app.occurrence.as_ref().unwrap().assisted);
}

#[test]
fn complete_archive_and_row_bounds_refuse_without_partial_authorship() {
    let mut app = empty();
    engine::seed_fixture(&mut app, NOW).unwrap();
    let concept = app.concepts.values_mut().next().unwrap();
    let mut note = concept.notes[0].clone();
    note.text = "x".repeat(16_000);
    concept.notes = vec![note; 70];
    engine::validate_app(&app).unwrap();
    refused(
        &mut app,
        "/concepts/create",
        &create_fields("authoring-row-bound"),
        NOW,
        413,
    );

    let mut app = empty();
    let (concept, _, _) = create(&mut app, "authoring-archive-base");
    let input = fields(
        "authoring-archive-overflow",
        &[("concept_id", &concept), ("title", "T"), ("text", "T")],
    );
    let mut sample = app.clone();
    call(&mut sample, "/materials/add", &input, NOW).unwrap();
    sample.operations.remove(&input["operation_id"]);
    let domain_growth =
        persistence::archive_size(&sample).unwrap() - persistence::archive_size(&app).unwrap();
    // Valid, bounded rows can still exceed the complete recovery envelope.
    for n in 0..510 {
        let id = format!("synthetic-bounded-material-{n}");
        let revision_id = format!("synthetic-bounded-revision-{n}");
        app.materials.items.insert(
            id.clone(),
            Material {
                id: id.clone(),
                concept_id: concept.clone(),
                current_revision_id: revision_id.clone(),
                created_ms: NOW,
            },
        );
        app.materials.revisions.insert(
            revision_id.clone(),
            MaterialRevision {
                id: revision_id,
                material_id: id,
                predecessor_id: None,
                ordinal: 1,
                title: "T".into(),
                text: "x".repeat(TEXT_LIMIT),
                provenance: MaterialProvenance::OwnerSupplied,
                created_ms: NOW,
            },
        );
    }
    engine::validate_app(&app).unwrap();
    assert!(persistence::rows(&app).is_ok());
    // Leave room for the new domain records, but not their receipt. Checking
    // bounds before adding the receipt would incorrectly acknowledge this write.
    let unbounded = serde_json::to_vec(&Archive::new(app.clone(), vec![], i64::MIN))
        .unwrap()
        .len();
    let mut trim = unbounded + domain_growth - persistence::MAX_ARCHIVE_BYTES;
    for revision in app.materials.revisions.values_mut().rev() {
        let remove = trim.min(revision.text.len() - 1);
        revision.text.truncate(revision.text.len() - remove);
        trim -= remove;
        if trim == 0 {
            break;
        }
    }
    assert_eq!(trim, 0);
    assert_eq!(
        persistence::archive_size(&app).unwrap(),
        persistence::MAX_ARCHIVE_BYTES - domain_growth
    );
    refused(&mut app, "/materials/add", &input, NOW, 413);
}
