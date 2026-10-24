#[path = "support/materials.rs"]
mod support;

use base64::{Engine as _, engine::general_purpose::STANDARD};
use scry::{
    engine, materials,
    model::{App, RESERVATION},
    persistence,
};
use std::collections::BTreeMap;
use support::*;

fn assert_original_rows(before: &BTreeMap<String, String>, after: &BTreeMap<String, String>) {
    for (key, bytes) in before {
        if key != "meta" {
            assert_eq!(after.get(key), Some(bytes), "Historical row changed: {key}");
        }
    }
}

fn assert_custody(app: &App, expected: &[u8]) {
    let retained: Vec<u8> = app
        .revival_go
        .values()
        .flat_map(|value| STANDARD.decode(value).unwrap())
        .collect();
    assert_eq!(retained, expected);
}

#[test]
fn typed_fixture_covers_both_history_models_before_adding_authored_materials() {
    let fixture = material_fixture().unwrap();
    let old = &fixture.baseline;
    assert!(old.materials.is_empty());
    assert!(!old.goals.is_empty());
    assert!(
        old.concepts
            .values()
            .all(|c| c.goal_id.is_some() && c.origin.is_none() && c.notes.len() == 2)
    );
    assert!(old.questions.values().any(|q| q.versions.len() == 2));
    assert_eq!(
        (old.events.len(), old.overrides.len(), old.resets.len()),
        (1, 1, 1)
    );
    assert_eq!(old.library.versions.len(), 2);
    assert_eq!(old.practice.studies.len(), 1);
    assert_eq!(old.practice.attempts.len(), 7);
    assert_eq!(old.practice.corrections.len(), 1);
    assert_eq!(old.spend.len(), 2);
    assert_eq!(old.spend[0].cost_micros, Some(12_345));
    assert!(old.spend[0].raw_response.is_some());
    assert_eq!(old.spend[1].status, "unknown");
    assert_eq!(old.spend[1].cost_micros, None);
    assert_eq!(old.spend[1].reserved_micros, RESERVATION);
    assert_eq!(old.spent(NOW + 500), 12_345 + RESERVATION);
    assert_eq!(fixture.photos.len(), 1);
    assert_eq!(old.goals.values().filter(|g| g.photo.is_some()).count(), 1);
    assert!(
        old.jobs
            .values()
            .all(|j| matches!(j.status.as_str(), "ready" | "failed" | "unknown"))
    );
    assert!(old.assessments.is_empty());
    let token = old.owner_tokens.values().next().unwrap();
    assert!(!token.info.paid && token.info.expires_ms.is_none());
    assert!(token.verifies(&fixture.owner_token_secret, NOW + 2_000));
    assert_eq!(old.revival_go.len(), 3);
    assert_eq!(old.revival_schedule_origins[REVIVAL_QUESTION_ID], 1);
    assert_eq!(old.questions[REVIVAL_QUESTION_ID].schedule_version, 1);
    assert_custody(old, &fixture.revival_bytes);
    for concept in old.concepts.values() {
        let row: serde_json::Value =
            serde_json::from_str(&fixture.baseline_rows[&format!("concepts/{}", concept.id)])
                .unwrap();
        assert!(row["goal_id"].is_string());
        assert!(
            row.get("origin").is_none(),
            "Do not invent generated provenance"
        );
    }
    engine::validate_app(old).unwrap();
    engine::validate_app(&fixture.app).unwrap();
}

#[test]
fn authored_concept_and_materials_only_add_rows_without_rewriting_old_history() {
    let fixture = material_fixture().unwrap();
    assert_original_rows(&fixture.baseline_rows, &fixture.material_rows);
    assert_eq!(
        fixture.material_rows.len(),
        fixture.baseline_rows.len() + 10
    );
    let authored = &fixture.app.concepts[AUTHOR_CONCEPT_ID];
    assert!(authored.goal_id.is_none());
    assert!(authored.notes.is_empty());
    assert!(authored.prerequisites.is_empty());
    assert_eq!(fixture.app.materials.items.len(), 2);
    assert_eq!(fixture.app.materials.revisions.len(), 3);
    for (path, entity) in [
        ("/concepts/create", AUTHOR_CONCEPT_ID),
        ("/materials/add", SECOND_MATERIAL_ID),
        ("/materials/revise", SECOND_REVISION_ID),
    ] {
        let receipts: Vec<_> = fixture
            .app
            .operations
            .values()
            .filter(|receipt| receipt.path.as_deref() == Some(path))
            .collect();
        assert_eq!(receipts.len(), 1);
        assert_eq!(receipts[0].result.entity_id.as_deref(), Some(entity));
        assert_eq!(
            receipts[0].result.redirect,
            format!("/concepts/{AUTHOR_CONCEPT_ID}")
        );
        assert!(receipts[0].result.data.is_none());
        assert!(receipts[0].acting_token.is_some());
    }
    assert_eq!(
        fixture.app.materials.revisions[FIRST_REVISION_ID]
            .text
            .as_bytes(),
        FIRST_TEXT.as_bytes()
    );
    assert_eq!(
        fixture.app.materials.revisions[SECOND_REVISION_ID]
            .text
            .as_bytes(),
        SECOND_TEXT.as_bytes()
    );
    assert_eq!(
        fixture.app.materials.revisions[THIRD_REVISION_ID]
            .text
            .as_bytes(),
        THIRD_TEXT.as_bytes()
    );
    assert_eq!(
        fixture.app.materials.items[FIRST_MATERIAL_ID].current_revision_id,
        SECOND_REVISION_ID
    );
    assert_eq!(
        fixture.app.materials.items[SECOND_MATERIAL_ID].current_revision_id,
        THIRD_REVISION_ID
    );
    let mut old_meta: serde_json::Value =
        serde_json::from_str(&fixture.baseline_rows["meta"]).unwrap();
    let new_meta: serde_json::Value = serde_json::from_str(&fixture.material_rows["meta"]).unwrap();
    old_meta["revision"] = new_meta["revision"].clone();
    assert_eq!(
        old_meta, new_meta,
        "The compatibility marker has its own row"
    );
    assert_custody(&fixture.app, &fixture.revival_bytes);
}

#[test]
fn materials_and_complete_history_survive_rows_archive_and_unrelated_write() {
    let fixture = material_fixture().unwrap();
    let mut loaded = persistence::app_from_rows(&fixture.material_rows).unwrap();
    assert_eq!(loaded, fixture.app);
    let archive = persistence::Archive::new(loaded.clone(), fixture.photos.clone(), NOW + 1_500);
    assert_eq!(archive.format, materials::ARCHIVE_FORMAT);
    let bytes = archive.encode().unwrap();
    let restored = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(restored.encode().unwrap(), bytes);
    assert_eq!(restored.app, loaded);
    assert_eq!(
        persistence::rows(&restored.app).unwrap(),
        fixture.material_rows
    );
    unrelated_settings_mutation(&mut loaded).unwrap();
    assert_eq!(loaded.preferences.pace, "light");
    assert_eq!(loaded.materials, fixture.app.materials);
    let written = persistence::rows(&loaded).unwrap();
    assert_original_rows(&fixture.material_rows, &written);
    assert_custody(&loaded, &fixture.revival_bytes);
    let reopened = persistence::app_from_rows(&written).unwrap();
    assert_eq!(reopened, loaded);
    let bytes = persistence::Archive::new(reopened, fixture.photos.clone(), NOW + 2_001)
        .encode()
        .unwrap();
    let again = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(again.app, loaded);
    assert_eq!(again.encode().unwrap(), bytes);
    let saved = loaded.clone();
    unrelated_settings_mutation(&mut loaded).unwrap();
    assert_eq!(
        loaded, saved,
        "The unrelated mutation still replays exactly"
    );
    engine::validate_app(&loaded).unwrap();
}

#[test]
fn legacy_note_projection_and_supplied_revision_views_do_not_migrate_history() {
    let fixture = material_fixture().unwrap();
    let app = &fixture.app;
    for concept in fixture.baseline.concepts.values() {
        let views = materials::for_concept(app, &app.concepts[&concept.id]);
        assert_eq!(views.len(), 1, "One Note chain is one Material view");
        match &views[0] {
            materials::MaterialView::LegacyNote {
                concept_id,
                revisions,
            } => {
                assert_eq!(*concept_id, concept.id);
                assert_eq!(*revisions, concept.notes.as_slice());
            }
            _ => panic!("Legacy Notes must remain a borrowed historical projection"),
        }
    }
    let views = materials::for_concept(app, &app.concepts[AUTHOR_CONCEPT_ID]);
    assert_eq!(views.len(), 2);
    let revision_counts: Vec<_> = views
        .iter()
        .map(|view| match view {
            materials::MaterialView::Supplied { revisions, .. } => revisions.len(),
            _ => panic!("Authored Concepts must not fabricate generated Notes"),
        })
        .collect();
    assert_eq!(revision_counts, vec![2, 1]);
    assert_eq!(persistence::rows(app).unwrap(), fixture.material_rows);
}

#[test]
fn typed_fixture_constructor_refuses_duplicates_without_partial_changes() {
    let fixture = material_fixture().unwrap();
    let mut app = fixture.app.clone();
    assert_eq!(add_authored_materials(&mut app).unwrap_err().status, 409);
    assert_eq!(app, fixture.app);
    assert_eq!(persistence::rows(&app).unwrap(), fixture.material_rows);
}

#[test]
fn legacy_concept_material_fixture_is_independent_of_authored_concept_schema() {
    let fixture = material_fixture().unwrap();
    let mut app = fixture.baseline.clone();
    add_material_to_legacy_concept(&mut app).unwrap();
    assert!(
        app.concepts
            .values()
            .all(|concept| concept.goal_id.is_some() && concept.origin.is_none())
    );
    assert_eq!(app.concepts, fixture.baseline.concepts);
    assert_eq!(app.materials.items.len(), 1);
    assert_eq!(app.materials.revisions.len(), 1);
    let rows = persistence::rows(&app).unwrap();
    assert_original_rows(&fixture.baseline_rows, &rows);
    assert_eq!(rows.len(), fixture.baseline_rows.len() + 4);
    let archive = persistence::Archive::new(app.clone(), fixture.photos, NOW + 1_500);
    let bytes = archive.encode().unwrap();
    let restored = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(restored.app, app);
    assert_eq!(persistence::rows(&restored.app).unwrap(), rows);
}

#[test]
fn restore_pause_preserves_history_and_does_not_restart_its_revision_epoch() {
    let fixture = material_fixture().unwrap();
    let mut restored = fixture.app.clone();
    engine::pause_for_restore(&mut restored, NOW + 1_501).unwrap();
    restored.backup = Default::default();
    assert_eq!(restored.revision, fixture.app.revision + 1);
    engine::validate_app(&restored).unwrap();
    assert_original_rows(
        &fixture.material_rows,
        &persistence::rows(&restored).unwrap(),
    );
    let archive = persistence::Archive::new(restored.clone(), fixture.photos, NOW + 1_502);
    assert!(archive.encode().is_ok());
    restored.revision = 1;
    assert!(
        engine::validate_app(&restored).is_err(),
        "Restoring must not place historical sequences in the future"
    );
}

/// The isolated runtime gate consumes the same typed fixture as native tests.
/// Invoke explicitly with SCRY_MATERIAL_FIXTURE_OUTPUT set to a run-owned path.
#[test]
#[ignore = "writes the requested synthetic runtime fixture"]
fn export_material_fixture() {
    let output = std::env::var_os("SCRY_MATERIAL_FIXTURE_OUTPUT")
        .expect("Set SCRY_MATERIAL_FIXTURE_OUTPUT to the run-owned fixture path");
    let fixture = material_fixture().unwrap();
    let archive =
        persistence::Archive::new(fixture.app.clone(), fixture.photos.clone(), NOW + 1_500);
    let baseline_archive =
        persistence::Archive::new(fixture.baseline.clone(), fixture.photos.clone(), NOW + 500);
    let archive_bytes = archive.encode().unwrap();
    let baseline_bytes = baseline_archive.encode().unwrap();
    let mut marker_app = fixture.baseline.clone();
    marker_app.materials.capability = true;
    let marker_archive =
        persistence::Archive::new(marker_app.clone(), fixture.photos.clone(), NOW + 500);
    let marker_bytes = marker_archive.encode().unwrap();
    let mut legacy_material_app = fixture.baseline.clone();
    add_material_to_legacy_concept(&mut legacy_material_app).unwrap();
    let legacy_material_archive = persistence::Archive::new(
        legacy_material_app.clone(),
        fixture.photos.clone(),
        NOW + 1_500,
    );
    let legacy_material_bytes = legacy_material_archive.encode().unwrap();
    let mut restored_app = fixture.app.clone();
    restored_app.csrf = "synthetic-restored-material-csrf-32-characters".into();
    engine::pause_for_restore(&mut restored_app, NOW + 1_501).unwrap();
    restored_app.backup = Default::default();
    engine::validate_app(&restored_app).unwrap();
    let restored_rows = persistence::rows(&restored_app).unwrap();
    let envelope = serde_json::json!({
        "archive": serde_json::from_slice::<serde_json::Value>(&archive_bytes).unwrap(),
        "baseline_archive": serde_json::from_slice::<serde_json::Value>(&baseline_bytes).unwrap(),
        "marker_archive": serde_json::from_slice::<serde_json::Value>(&marker_bytes).unwrap(),
        "marker_rows": persistence::rows(&marker_app).unwrap(),
        "legacy_material_archive": serde_json::from_slice::<serde_json::Value>(&legacy_material_bytes).unwrap(),
        "legacy_material_rows": persistence::rows(&legacy_material_app).unwrap(),
        "restored_app": restored_app,
        "restored_rows": restored_rows,
        "rows": fixture.material_rows,
        "baseline_rows": fixture.baseline_rows,
        "owner_token_secret": fixture.owner_token_secret,
        "ids": {
            "authored_concept": AUTHOR_CONCEPT_ID,
            "owner_token": fixture.app.owner_tokens.keys().next().unwrap(),
            "legacy_concept": legacy_material_app.materials.items[LEGACY_MATERIAL_ID].concept_id,
            "legacy_material": LEGACY_MATERIAL_ID,
            "legacy_revision": LEGACY_REVISION_ID,
            "materials": [FIRST_MATERIAL_ID, SECOND_MATERIAL_ID],
            "revisions": [FIRST_REVISION_ID, SECOND_REVISION_ID, THIRD_REVISION_ID],
            "generated_concepts": fixture.baseline.concepts.keys().collect::<Vec<_>>(),
            "exact_versions": fixture.baseline.library.versions.keys().collect::<Vec<_>>(),
            "exact_studies": fixture.baseline.practice.studies.keys().collect::<Vec<_>>()
        }
    });
    std::fs::write(output, serde_json::to_vec(&envelope).unwrap()).unwrap();
}

#[test]
fn retained_material_receipts_replay_safely_after_phase_b_enablement() {
    let fixture = material_fixture().unwrap();
    let cases = [
        ("synthetic-future-concept-create", "/concepts/create"),
        ("synthetic-future-material-add", "/materials/add"),
        ("synthetic-future-material-revise", "/materials/revise"),
    ];
    for (index, (operation, path)) in cases.into_iter().enumerate() {
        let receipt = &fixture.app.operations[operation];
        let payload = BTreeMap::from([
            ("operation_id".to_string(), operation.to_string()),
            ("synthetic_fixture".to_string(), "future-writer".to_string()),
            (
                "entity_id".to_string(),
                receipt.result.entity_id.clone().unwrap(),
            ),
        ]);
        assert_eq!(
            receipt.payload_hash,
            persistence::sha256(&serde_json::to_vec(&(path, &payload)).unwrap())
        );
        for variant in 0..3 {
            let mut fields = payload.clone();
            if variant == 1 {
                fields.insert("entity_id".into(), "changed-payload".into());
            } else if variant == 2 {
                fields.insert(
                    "operation_id".into(),
                    format!("fresh-enabled-authoring-{index}"),
                );
            }
            fields.insert("csrf".into(), fixture.app.csrf.clone());
            let mut candidate = fixture.app.clone();
            let result = engine::mutate(
                &mut candidate,
                path,
                &fields,
                NOW + 3_000,
                "enabled-authoring",
            );
            match variant {
                0 => assert_eq!(result.unwrap(), receipt.result),
                1 => assert_eq!(result.unwrap_err().status, 409),
                _ => assert_eq!(result.unwrap_err().status, 422),
            }
            assert_eq!(candidate, fixture.app);
            assert_eq!(
                persistence::rows(&candidate).unwrap(),
                fixture.material_rows
            );
        }
    }
    // Ordinary existing operations still replay without duplicate domain changes.
    let mut ordinary = fixture.app.clone();
    unrelated_settings_mutation(&mut ordinary).unwrap();
    let saved = ordinary.clone();
    unrelated_settings_mutation(&mut ordinary).unwrap();
    assert_eq!(ordinary, saved);
}
