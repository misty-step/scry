use scry::{
    engine,
    materials::*,
    model::*,
    persistence::{self, Archive},
};
use std::collections::BTreeMap;

const NOW: i64 = 1_791_410_400_000;

/// A typed domain constructor for this compatibility fixture, not a production
/// creation path or a bypass of archive/row validation.
fn fixture() -> App {
    let mut app = App::new("synthetic-material-csrf-at-least-32-characters".into());
    app.concepts.insert(
        "concept-authored".into(),
        Concept {
            id: "concept-authored".into(),
            goal_id: None,
            origin: Some(ConceptOrigin::OwnerAuthored),
            name: "Conditional requests".into(),
            summary: String::new(),
            notes: vec![],
            prerequisites: vec![],
            introduced_ms: None,
            exposure_ms: None,
            already_known: false,
            archived: false,
        },
    );
    app.materials.capability = true;
    for (id, texts) in [
        (
            "material-a",
            vec!["  Original\r\n\ttext: café 💡\n", "Revised supplied text"],
        ),
        ("material-b", vec!["A second independent reference."]),
    ] {
        let mut predecessor_id = None;
        for (index, text) in texts.iter().enumerate() {
            let revision_id = format!("{id}-revision-{}", index + 1);
            app.materials.revisions.insert(
                revision_id.clone(),
                MaterialRevision {
                    id: revision_id.clone(),
                    material_id: id.into(),
                    predecessor_id,
                    ordinal: index as u64 + 1,
                    title: format!("Reference {id}"),
                    text: (*text).into(),
                    provenance: MaterialProvenance::OwnerSupplied,
                    created_ms: NOW + index as i64,
                },
            );
            predecessor_id = Some(revision_id);
        }
        app.materials.items.insert(
            id.into(),
            Material {
                id: id.into(),
                concept_id: "concept-authored".into(),
                current_revision_id: predecessor_id.unwrap(),
                created_ms: NOW,
            },
        );
    }
    engine::validate_app(&app).unwrap();
    app
}

fn rejects(app: App) {
    assert!(engine::validate_app(&app).is_err());
    assert!(persistence::rows(&app).is_err());
    assert!(Archive::new(app, vec![], NOW).encode().is_err());
}

#[test]
fn authored_concept_is_not_a_goal_or_a_learning_observation() {
    let app = fixture();
    assert!(app.goals.is_empty() && app.jobs.is_empty() && app.questions.is_empty());
    assert!(app.events.is_empty() && app.spend.is_empty() && app.assessments.is_empty());
    assert!(app.practice.is_empty() && app.library.is_empty());
    let before = serde_json::to_vec(&app).unwrap();
    let materials = for_concept(&app, &app.concepts["concept-authored"]);
    assert_eq!(materials.len(), 2);
    assert_eq!(serde_json::to_vec(&app).unwrap(), before);
    assert_eq!(
        app.materials.revisions["material-a-revision-1"].text,
        "  Original\r\n\ttext: café 💡\n"
    );
    let rows = persistence::rows(&app).unwrap();
    assert_eq!(rows[CAPABILITY_ROW], "true");
    assert!(
        !serde_json::from_str::<serde_json::Value>(&rows["meta"])
            .unwrap()
            .as_object()
            .unwrap()
            .contains_key("materials")
    );
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), app);
    let archive = Archive::new(app.clone(), vec![], NOW);
    assert_eq!(archive.format, ARCHIVE_FORMAT);
    let bytes = archive.encode().unwrap();
    assert_eq!(
        Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        app
    );
}

#[test]
fn every_revision_must_belong_to_one_complete_linear_material_chain() {
    for defect in 0..12 {
        let mut app = fixture();
        match defect {
            0 => {
                app.materials
                    .items
                    .get_mut("material-a")
                    .unwrap()
                    .current_revision_id = "missing".into()
            }
            1 => {
                app.materials
                    .items
                    .get_mut("material-a")
                    .unwrap()
                    .current_revision_id = "material-b-revision-1".into()
            }
            2 => {
                app.materials
                    .revisions
                    .get_mut("material-a-revision-2")
                    .unwrap()
                    .predecessor_id = Some("material-b-revision-1".into())
            }
            3 => {
                app.materials
                    .revisions
                    .get_mut("material-a-revision-2")
                    .unwrap()
                    .ordinal = 3
            }
            4 => {
                app.materials
                    .revisions
                    .get_mut("material-a-revision-1")
                    .unwrap()
                    .ordinal = 0
            }
            5 => {
                app.materials
                    .revisions
                    .get_mut("material-a-revision-1")
                    .unwrap()
                    .predecessor_id = Some("material-a-revision-2".into())
            }
            6 => {
                app.materials
                    .revisions
                    .get_mut("material-a-revision-2")
                    .unwrap()
                    .created_ms = NOW - 1
            }
            7 => {
                app.materials
                    .items
                    .get_mut("material-a")
                    .unwrap()
                    .created_ms = NOW - 1
            }
            8 => {
                app.materials
                    .items
                    .get_mut("material-a")
                    .unwrap()
                    .concept_id = "missing".into()
            }
            9 => {
                app.materials
                    .revisions
                    .get_mut("material-a-revision-2")
                    .unwrap()
                    .material_id = "missing".into()
            }
            10 => {
                let mut branch = app.materials.revisions["material-a-revision-2"].clone();
                branch.id = "branch".into();
                app.materials.revisions.insert(branch.id.clone(), branch);
            }
            11 => {
                app.materials
                    .items
                    .get_mut("material-a")
                    .unwrap()
                    .current_revision_id = "material-a-revision-1".into()
            }
            _ => unreachable!(),
        }
        rejects(app);
    }
}

#[test]
fn authored_origin_does_not_relax_generated_contracts_or_invent_evidence() {
    for defect in 0..9 {
        let mut app = fixture();
        let c = app.concepts.get_mut("concept-authored").unwrap();
        match defect {
            0 => c.origin = None,
            1 => c.goal_id = Some("fake-goal".into()),
            2 => c.introduced_ms = Some(NOW),
            3 => c.exposure_ms = Some(NOW),
            4 => c.already_known = true,
            5 => c.prerequisites.push("fake-prerequisite".into()),
            6 => app.materials.items.clear(),
            7 => app.materials.capability = false,
            8 => c.name.clear(),
            _ => unreachable!(),
        }
        rejects(app);
    }
    let mut generated = App::new("synthetic-legacy-csrf-at-least-32-characters".into());
    engine::seed_fixture(&mut generated, NOW).unwrap();
    let before = persistence::rows(&generated).unwrap();
    let concept = generated.concepts.values_mut().next().unwrap();
    concept.notes.push(concept.notes[0].clone());
    let concept_id = concept.id.clone();
    let views = for_concept(&generated, &generated.concepts[&concept_id]);
    assert!(
        matches!(views.as_slice(), [MaterialView::LegacyNote { revisions, .. }] if revisions.len() == 2)
    );
    assert!(generated.materials.is_empty());
    assert!(!before.keys().any(|k| k.starts_with("material")));
    let c = generated.concepts.get_mut(&concept_id).unwrap();
    c.notes.clear();
    assert!(
        engine::validate_app(&generated).is_err(),
        "Legacy notes remain mandatory."
    );
}

#[test]
fn old_fields_and_marker_only_state_are_explicit_and_byte_stable() {
    let mut app = App::new("synthetic-old-csrf-at-least-32-characters".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    let old = persistence::rows(&app).unwrap();
    let bytes = Archive::new(app.clone(), vec![], NOW).encode().unwrap();
    assert!(
        !String::from_utf8(bytes.clone())
            .unwrap()
            .contains("\"materials\"")
    );
    for c in app.concepts.values() {
        let json = serde_json::to_value(c).unwrap();
        assert!(json["goal_id"].is_string());
        assert!(json.get("origin").is_none());
    }
    assert_eq!(
        persistence::rows(&persistence::app_from_rows(&old).unwrap()).unwrap(),
        old
    );
    assert_eq!(
        Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .encode()
            .unwrap(),
        bytes
    );
    app.materials.capability = true;
    let marked = persistence::rows(&app).unwrap();
    assert_eq!(marked.len(), old.len() + 1);
    for (key, value) in &old {
        assert_eq!(&marked[key], value);
    }
    assert_eq!(
        Archive::new(app.clone(), vec![], NOW).format,
        ARCHIVE_FORMAT
    );
    assert_eq!(persistence::app_from_rows(&marked).unwrap(), app);
    let mut bad = marked.clone();
    bad.insert(CAPABILITY_ROW.into(), "false".into());
    assert!(persistence::app_from_rows(&bad).is_err());
    let mut data_without_marker = persistence::rows(&fixture()).unwrap();
    data_without_marker.remove(CAPABILITY_ROW);
    assert!(persistence::app_from_rows(&data_without_marker).is_err());
    let mut downgrade = Archive::new(app, vec![], NOW);
    downgrade.format = persistence::ARCHIVE_FORMAT.into();
    assert!(downgrade.encode().is_err());
}

#[test]
fn exact_utf8_bounds_and_complete_archive_capacity_are_enforced() {
    let mut app = fixture();
    app.materials
        .revisions
        .get_mut("material-a-revision-1")
        .unwrap()
        .text = "💡".repeat(TEXT_LIMIT / 4);
    engine::validate_app(&app).unwrap();
    assert!(
        persistence::rows(&app)
            .unwrap()
            .values()
            .all(|r| r.len() <= persistence::MAX_RECORD_BYTES)
    );
    let mut oversized = app.clone();
    oversized
        .materials
        .revisions
        .get_mut("material-a-revision-1")
        .unwrap()
        .text
        .push('x');
    rejects(oversized);
    for text in ["", " \n\t", "with\0nul"] {
        let mut invalid = fixture();
        invalid
            .materials
            .revisions
            .get_mut("material-a-revision-1")
            .unwrap()
            .text = text.into();
        rejects(invalid);
    }
    for index in 0..520 {
        let id = format!("large-{index}");
        let rid = format!("{id}-revision");
        app.materials.items.insert(
            id.clone(),
            Material {
                id: id.clone(),
                concept_id: "concept-authored".into(),
                current_revision_id: rid.clone(),
                created_ms: NOW,
            },
        );
        app.materials.revisions.insert(
            rid.clone(),
            MaterialRevision {
                id: rid,
                material_id: id,
                predecessor_id: None,
                ordinal: 1,
                title: "Bounded reference".into(),
                text: "x".repeat(TEXT_LIMIT),
                provenance: MaterialProvenance::OwnerSupplied,
                created_ms: NOW,
            },
        );
    }
    engine::validate_app(&app).unwrap();
    assert_eq!(persistence::archive_size(&app).unwrap_err().status, 413);
    assert_eq!(
        Archive::new(app, vec![], NOW).encode().unwrap_err().status,
        413
    );
}

#[test]
fn no_normal_authoring_or_legacy_mutation_of_authored_concepts_is_enabled() {
    let mut app = fixture();
    let before = app.clone();
    assert_eq!(engine::seed_fixture(&mut app, NOW).unwrap_err().status, 409);
    assert_eq!(
        app, before,
        "The demo cannot introduce restored authored Concepts."
    );
    for path in [
        "/concepts/create",
        "/materials/add",
        "/materials/revise",
        "/concepts/concept-authored/archive",
        "/concepts/concept-authored/feedback",
        "/concepts/concept-authored/practice",
    ] {
        let before = app.clone();
        let fields = BTreeMap::from([
            ("csrf".into(), app.csrf.clone()),
            ("operation_id".into(), "authored-refusal-operation".into()),
        ]);
        assert!(engine::mutate(&mut app, path, &fields, NOW, "refuse").is_err());
        assert_eq!(app, before);
    }
    for path in [
        "/api/v1/concepts",
        "/api/v1/materials",
        "/api/v1/materials/revise",
    ] {
        assert_eq!(scry::api::mutation_path(path), None);
    }
}

#[test]
fn simulated_future_receipts_require_the_exact_retained_entity_and_owner() {
    let mut app = fixture();
    // This models acknowledged Phase B writes. Phase A has no writer for them.
    for (operation, path, entity) in [
        (
            "synthetic-concept-create",
            "/concepts/create",
            "concept-authored",
        ),
        ("synthetic-material-add", "/materials/add", "material-b"),
        (
            "synthetic-material-revise",
            "/materials/revise",
            "material-a-revision-2",
        ),
    ] {
        app.operations.insert(
            operation.into(),
            Receipt {
                payload_hash: persistence::sha256(operation.as_bytes()),
                acting_token: None,
                path: Some(path.into()),
                created_ms: Some(NOW + 2),
                result: MutationResult {
                    data: None,
                    redirect: "/concepts/concept-authored".into(),
                    entity_id: Some(entity.into()),
                    message: None,
                },
            },
        );
    }
    engine::validate_app(&app).unwrap();
    let rows = persistence::rows(&app).unwrap();
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), app);
    for defect in 0..8 {
        let mut invalid = app.clone();
        let receipt = invalid
            .operations
            .get_mut("synthetic-material-revise")
            .unwrap();
        match defect {
            0 => receipt.result.entity_id = Some("missing".into()),
            1 => receipt.result.entity_id = Some("material-a".into()),
            2 => receipt.result.entity_id = Some("material-a-revision-1".into()),
            3 => receipt.result.redirect = "/concepts/wrong-owner".into(),
            4 => receipt.created_ms = Some(NOW - 1),
            5 => receipt.result.data = Some(serde_json::json!({"unknown":"shape"})),
            6 => receipt.payload_hash = "not-a-hash".into(),
            7 => invalid.materials.capability = false,
            _ => unreachable!(),
        }
        rejects(invalid);
    }
    let mut changed = app.clone();
    changed
        .operations
        .get_mut("synthetic-material-revise")
        .unwrap()
        .path = Some("/materials/not-reserved".into());
    rejects(changed);
}

#[test]
fn new_record_identity_and_storage_bounds_are_not_bypassed() {
    let mut app = fixture();
    app.materials.items.get_mut("material-a").unwrap().id = "concept-authored".into();
    rejects(app);
    let mut app = fixture();
    app.materials
        .revisions
        .get_mut("material-a-revision-1")
        .unwrap()
        .id = "material-a".into();
    rejects(app);
    let mut rows = persistence::rows(&fixture()).unwrap();
    rows.insert("material_capability/v2".into(), "true".into());
    assert!(persistence::app_from_rows(&rows).is_err());
    let mut rows = persistence::rows(&fixture()).unwrap();
    let key = "material_revisions/material-a-revision-1";
    let mut revision: serde_json::Value = serde_json::from_str(&rows[key]).unwrap();
    revision["unrecognized_provenance"] = serde_json::json!("verified source");
    rows.insert(key.into(), revision.to_string());
    assert!(persistence::app_from_rows(&rows).is_err());

    let mut generated = App::new("synthetic-row-bound-csrf-at-least-32-characters".into());
    engine::seed_fixture(&mut generated, NOW).unwrap();
    generated.materials.capability = true;
    let concept = generated.concepts.values_mut().next().unwrap();
    let mut note = concept.notes[0].clone();
    note.text = "x".repeat(16_000);
    concept.notes = vec![note; 70];
    engine::validate_app(&generated).unwrap();
    assert_eq!(persistence::rows(&generated).unwrap_err().status, 413);
}

#[test]
fn restored_authored_materials_are_readable_without_offering_fake_learning_actions() {
    let mut app = fixture();
    let revision = app
        .materials
        .revisions
        .get_mut("material-a-revision-2")
        .unwrap();
    revision.title = "Supplied <title> & quotation".into();
    revision.text = "  <script>not executable</script>\r\n\tExact café 💡\n".into();
    engine::validate_app(&app).unwrap();
    let before = persistence::rows(&app).unwrap();
    let query = BTreeMap::new();
    let home = scry::web::render(&app, "/", &query, NOW, None);
    assert!(home.contains("Your concepts") && home.contains("Study material"));
    assert!(home.contains("href=\"/concepts/concept-authored\""));
    assert!(!home.contains("Start practice") && !home.contains("Resume practice"));
    let page = scry::web::render(&app, "/concepts/concept-authored", &query, NOW, None);
    assert!(page.contains("aria-label=\"Study materials\""));
    assert_eq!(page.matches("data-material-id=").count(), 2);
    assert_eq!(page.matches("data-material-revision-id=").count(), 3);
    for revision in app.materials.revisions.values() {
        assert!(page.contains(&scry::web::escape(&revision.text)));
        assert!(page.contains(&scry::web::escape(&revision.title)));
    }
    assert!(
        page.find("material-a-revision-2").unwrap() < page.find("material-a-revision-1").unwrap()
    );
    assert!(page.contains("Earlier revisions") && page.contains("No saved questions"));
    assert!(!page.contains("<script>not executable"));
    assert!(!page.contains("Saved explanation") && !page.contains("No note is available"));
    for action in ["practice", "feedback", "archive"] {
        assert!(!page.contains(&format!("action=\"/concepts/concept-authored/{action}\"")));
    }
    assert!(!page.contains("Estimated recall") && !page.contains("Not enough unaided"));
    assert_eq!(persistence::rows(&app).unwrap(), before);
}

#[test]
fn supplied_material_on_a_generated_concept_keeps_the_existing_assistance_fence() {
    let mut app = App::new("synthetic-material-fence-csrf-at-least-32-characters".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    engine::ensure_occurrence(&mut app, NOW, "material-fence").unwrap();
    let concept_id = app.occurrence.as_ref().unwrap().concept_id.clone();
    app.materials = fixture().materials;
    for material in app.materials.items.values_mut() {
        material.concept_id = concept_id.clone();
    }
    engine::validate_app(&app).unwrap();
    let before = app.clone();
    let path = format!("/concepts/{concept_id}");
    let gated = scry::web::render(&app, &path, &BTreeMap::new(), NOW, None);
    assert!(gated.contains("data-state=\"gate\""));
    assert!(!gated.contains("data-material-id=") && !gated.contains("Revised supplied text"));
    assert_eq!(app, before);
    // Deliberate assistance, not a read bypass, permits both legacy and new reference.
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "look-at-material".into()),
        (
            "occurrence_id".into(),
            app.occurrence.as_ref().unwrap().id.clone(),
        ),
    ]);
    engine::mutate(&mut app, "/review/help", &fields, NOW + 1, "material-help").unwrap();
    let allowed = scry::web::render(&app, &path, &BTreeMap::new(), NOW + 1, None);
    assert_eq!(
        allowed.matches("aria-label=\"Saved explanation\"").count(),
        1
    );
    assert_eq!(allowed.matches("data-material-id=").count(), 2);
    assert!(
        allowed.contains("Practice this idea")
            && allowed.contains("Original material &amp; preparation")
    );
    assert!(allowed.contains(&format!("action=\"/concepts/{concept_id}/feedback\"")));
}

#[test]
fn a_missing_legacy_goal_cannot_sneak_through_the_optional_field_row_decoder() {
    let mut app = App::new("synthetic-missing-goal-csrf-at-least-32-characters".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    let original = persistence::rows(&app).unwrap();
    let concept_id = app.concepts.keys().next().unwrap().clone();
    let key = format!("concepts/{concept_id}");
    for null_instead_of_absent in [false, true] {
        let mut rows = original.clone();
        let mut record: serde_json::Value = serde_json::from_str(&rows[&key]).unwrap();
        if null_instead_of_absent {
            record["goal_id"] = serde_json::Value::Null;
        } else {
            record.as_object_mut().unwrap().remove("goal_id");
        }
        rows.insert(key.clone(), record.to_string());
        assert!(persistence::app_from_rows(&rows).is_err());
    }
    app.concepts.get_mut(&concept_id).unwrap().goal_id = None;
    assert!(requires_capability(&app));
    rejects(app);
}
