fn artifact(name: &str) -> std::path::PathBuf {
    let root = std::path::PathBuf::from("target/text-repair");
    std::fs::create_dir_all(&root).unwrap();
    root.join(name)
}
use scry::{engine, model::App, persistence};
use serde_json::Value;
fn mutation_fields(
    app: &App,
    operation: &str,
    extra: &[(&str, String)],
) -> std::collections::BTreeMap<String, String> {
    let mut fields = std::collections::BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("revision".into(), app.revision.to_string()),
        ("operation_id".into(), operation.into()),
    ]);
    fields.extend(extra.iter().map(|(k, v)| (k.to_string(), v.clone())));
    fields
}
fn checkpoint_app() -> App {
    let fixture: Value =
        serde_json::from_str(include_str!("fixtures/split-unit-legacy.json")).unwrap();
    let mut app: App = serde_json::from_value(fixture["quiz"].clone()).unwrap();
    let pid = app.practice.current.as_ref().unwrap().prompt.id.clone();
    let fields = mutation_fields(
        &app,
        "synthetic-save-draft-0001",
        &[
            ("prompt_id", pid),
            ("generation", "0".into()),
            ("answer", "Preserve café\n3.14 exactly.".into()),
        ],
    );
    engine::mutate(
        &mut app,
        "/practice/draft",
        &fields,
        1800000000000,
        "save-draft",
    )
    .unwrap();
    engine::validate_app(&app).unwrap();
    app
}
#[test]
fn repaired_copy_preserves_checkpoint_and_roundtrips() {
    let mut app = checkpoint_app();
    let before = app.clone();
    let original = app.library.versions.keys().next().unwrap().clone();
    let v = app.library.versions[&original].clone();
    let revision = app.library.works[&v.work_id].revision;
    let fp = scry::text_repair::preview(&app.library, &original).unwrap();
    let fields = mutation_fields(
        &app,
        "candidate-repair-0001",
        &[
            ("version_id", original.clone()),
            ("revision", revision.to_string()),
            ("fingerprint", fp),
        ],
    );
    let result = engine::mutate(
        &mut app,
        "/library/version/repair",
        &fields,
        1800000000001,
        "candidate-repair",
    )
    .unwrap();
    engine::validate_app(&app).unwrap();
    assert_eq!(result.redirect, "/recall");
    assert!(result.data.is_none());
    assert!(result.message.is_none());
    let copy = result.entity_id.clone().unwrap();
    assert_ne!(copy, original);
    assert_eq!(app.library.versions[&copy].text, v.text);
    assert_eq!(app.library.versions[&original], v);
    assert_eq!(app.practice, before.practice);
    assert_eq!(app.study_version_refs, before.study_version_refs);
    assert_eq!(app.revision, before.revision + 1);
    assert_eq!(app.library.works[&v.work_id].revision, revision + 1);
    let mut expected = before.clone();
    expected.library = app.library.clone();
    expected.operations = app.operations.clone();
    expected.revision += 1;
    assert_eq!(app, expected);
    let rows = persistence::rows(&app).unwrap();
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), app);
    let replay = engine::mutate(
        &mut app,
        "/library/version/repair",
        &fields,
        1800000000002,
        "unused-replay-seed",
    )
    .unwrap();
    assert_eq!(
        serde_json::to_value(replay).unwrap(),
        serde_json::to_value(result).unwrap()
    );
    assert_eq!(persistence::rows(&app).unwrap(), rows);
    let bytes = persistence::Archive::new(app.clone(), vec![], 1800000000002)
        .encode()
        .unwrap();
    let archive = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(archive.app, app);
    assert_eq!(archive.format, "scry-rust-durable-sqlite-text-repair-v1");
    std::fs::write(
        artifact("repaired-rows.json"),
        serde_json::to_vec(&rows).unwrap(),
    )
    .unwrap();
    std::fs::write(artifact("repaired-archive.json"), &bytes).unwrap();
    let mut stripped = rows.clone();
    stripped.remove("text_repair_compatibility/v1");
    assert!(persistence::app_from_rows(&stripped).is_err());
    let mut second = fields.clone();
    second.insert("operation_id".into(), "candidate-repair-0002".into());
    let saved = app.clone();
    assert!(
        engine::mutate(
            &mut app,
            "/library/version/repair",
            &second,
            1800000000003,
            "stale-second"
        )
        .is_err()
    );
    assert_eq!(app, saved);
    second.insert(
        "revision".into(),
        app.library.works[&v.work_id].revision.to_string(),
    );
    second.insert("fingerprint".into(), "bogus".into());
    assert!(
        engine::mutate(
            &mut app,
            "/library/version/repair",
            &second,
            1800000000003,
            "bogus-second"
        )
        .is_err()
    );
    assert_eq!(app, saved);
    second.insert(
        "fingerprint".into(),
        scry::text_repair::preview(&app.library, &original).unwrap(),
    );
    let result = engine::mutate(
        &mut app,
        "/library/version/repair",
        &second,
        1800000000003,
        "second-repair",
    )
    .unwrap();
    assert_eq!(result.entity_id.unwrap(), copy);
    assert_eq!(app.library, archive.app.library);
    assert_eq!(app.practice, before.practice);
}

#[test]
fn tampered_preview_and_lineage_refuse_without_mutation() {
    let mut app = checkpoint_app();
    let original = app.library.versions.keys().next().unwrap().clone();
    let before = app.clone();
    let revision = app.library.works[&app.library.versions[&original].work_id].revision;
    let mut fields = mutation_fields(
        &app,
        "candidate-tamper-0001",
        &[
            ("version_id", original.clone()),
            ("revision", revision.to_string()),
            ("fingerprint", "bad-fingerprint".into()),
        ],
    );
    assert!(
        engine::mutate(
            &mut app,
            "/library/version/repair",
            &fields,
            1800000000001,
            "tamper"
        )
        .is_err()
    );
    assert_eq!(app, before);
    fields.insert(
        "fingerprint".into(),
        scry::text_repair::preview(&app.library, &original).unwrap(),
    );
    engine::mutate(
        &mut app,
        "/library/version/repair",
        &fields,
        1800000000001,
        "repair",
    )
    .unwrap();
    let mut tampered = app.clone();
    tampered.library.repairs.get_mut(&original).unwrap().policy = "future-policy".into();
    assert!(engine::validate_app(&tampered).is_err());
    let mut tampered = app.clone();
    let copy = app.library.repairs[&original].copy.clone();
    tampered
        .library
        .versions
        .get_mut(&copy)
        .unwrap()
        .text
        .push(' ');
    assert!(engine::validate_app(&tampered).is_err());
    assert!(scry::text_repair::preview(&app.library, &copy).is_err());
}
fn repair_request(app: &App) -> std::collections::BTreeMap<String, String> {
    let id = app.library.versions.keys().next().unwrap();
    let v = &app.library.versions[id];
    mutation_fields(
        app,
        "candidate-refusal-001",
        &[
            ("version_id", id.clone()),
            (
                "revision",
                app.library.works[&v.work_id].revision.to_string(),
            ),
            (
                "fingerprint",
                scry::text_repair::preview(&app.library, id).unwrap(),
            ),
        ],
    )
}
#[test]
fn runtime_admission_is_explicitly_closed() {
    assert_eq!(
        engine::admit_current_mutation("/library/version/repair")
            .unwrap_err()
            .status,
        410
    );
}
#[test]
fn full_app_precommit_refuses_without_changes() {
    let mut app = checkpoint_app();
    app.study_version_refs
        .insert("bad-reference".into(), "missing-version".into());
    let before = app.clone();
    let f = repair_request(&app);
    assert!(
        engine::mutate(
            &mut app,
            "/library/version/repair",
            &f,
            1800000000001,
            "refusal"
        )
        .is_err()
    );
    assert_eq!(app, before);
}
fn filler(n: usize) -> scry::model::Receipt {
    scry::model::Receipt {
        acting_token: None,
        path: None,
        created_ms: None,
        payload_hash: "a".repeat(64),
        result: scry::model::MutationResult {
            data: None,
            redirect: "/".into(),
            entity_id: None,
            message: Some("x".repeat(n)),
        },
    }
}
#[test]
fn row_capacity_precommit_refuses_without_changes() {
    let mut app = checkpoint_app();
    app.operations.insert(
        "synthetic-oversize-row".into(),
        filler(persistence::MAX_RECORD_BYTES),
    );
    engine::validate_app(&app).unwrap();
    let before = app.clone();
    let f = repair_request(&app);
    assert_eq!(
        engine::mutate(
            &mut app,
            "/library/version/repair",
            &f,
            1800000000001,
            "refusal"
        )
        .unwrap_err()
        .status,
        413
    );
    assert_eq!(app, before);
}
#[test]
fn crossing_archive_capacity_refuses_without_changes() {
    let mut app = checkpoint_app();
    for i in 0..18 {
        app.operations
            .insert(format!("synthetic-fill-{i:04}"), filler(900000));
    }
    app.operations
        .insert("synthetic-tail-0000".into(), filler(0));
    let size = persistence::archive_size(&app).unwrap();
    let remain = persistence::MAX_ARCHIVE_BYTES - size - 100;
    assert!(remain < 900000);
    app.operations
        .get_mut("synthetic-tail-0000")
        .unwrap()
        .result
        .message = Some("x".repeat(remain));
    assert_eq!(
        persistence::archive_size(&app).unwrap(),
        persistence::MAX_ARCHIVE_BYTES - 100
    );
    engine::validate_app(&app).unwrap();
    assert!(persistence::rows(&app).is_ok());
    let before = app.clone();
    let f = repair_request(&app);
    assert_eq!(
        engine::mutate(
            &mut app,
            "/library/version/repair",
            &f,
            1800000000001,
            "refusal"
        )
        .unwrap_err()
        .status,
        413
    );
    assert_eq!(app, before);
}
#[test]
fn pinned_partition_goldens() {
    for (text, expected) in [
        ("Use 3.14 for pi.", vec!["Use 3.14 for pi."]),
        ("Alpha,beta.", vec!["Alpha,beta."]),
        ("\n...Alpha, beta.", vec!["\n...Alpha,", " beta."]),
        ("Straße,Äpfel.", vec!["Straße,Äpfel."]),
        ("\r\nAlpha.\r\n", vec!["\r\nAlpha.\r\n"]),
    ] {
        let p = scry::text_repair_policy_v1::segments(text);
        let units = p
            .iter()
            .flat_map(|s| &s.units)
            .map(String::as_str)
            .collect::<Vec<_>>();
        assert_eq!(units, expected);
        assert_eq!(units.concat(), text);
        scry::text_repair_policy_v1::check_units(units.into_iter()).unwrap();
    }
    assert!(
        scry::text_repair_policy_v1::check_units(["Use 3.", "14 for pi."].into_iter()).is_err()
    );
    assert!(scry::text_repair_policy_v1::check_units(["...", "Alpha"].into_iter()).is_err());
}

#[test]
fn preview_fingerprint_golden() {
    let app = checkpoint_app();
    let id = app.library.versions.keys().next().unwrap();
    assert_eq!(
        scry::text_repair::preview(&app.library, id).unwrap(),
        "85bcaec67a8a10d39e7c3fa2e6506d53ee15cd814973bb10a1c2e6af01b345c7"
    );
}
#[test]
fn missing_source_work_is_an_error_not_a_panic() {
    let mut app = checkpoint_app();
    let f = repair_request(&app);
    let id = app.library.versions.keys().next().unwrap().clone();
    app.library.works.clear();
    let before = app.clone();
    assert!(scry::text_repair::preview(&app.library, &id).is_err());
    assert!(
        engine::mutate(
            &mut app,
            "/library/version/repair",
            &f,
            1800000000001,
            "missing-work"
        )
        .is_err()
    );
    assert_eq!(app, before);
}
#[test]
fn lineage_views_and_versioned_library_exports() {
    let mut app = checkpoint_app();
    let original = app.library.versions.keys().next().unwrap().clone();
    let work = app.library.versions[&original].work_id.clone();
    let ordinary = scry::api::library_read(&app, "/api/v1/library/export", "").unwrap();
    assert_eq!(ordinary, serde_json::to_value(&app.library).unwrap());
    let oldbytes = serde_json::to_vec(&ordinary).unwrap();
    assert_eq!(
        scry::text_repair::decode_library_export(&oldbytes).unwrap(),
        app.library
    );
    std::fs::write(artifact("ordinary-library.json"), oldbytes).unwrap();
    assert!(
        serde_json::to_value(app.library.show_work(&work).unwrap())
            .unwrap()
            .get("repairs")
            .is_none()
    );
    let fields = repair_request(&app);
    let copy = engine::mutate(
        &mut app,
        "/library/version/repair",
        &fields,
        1800000000001,
        "view-repair",
    )
    .unwrap()
    .entity_id
    .unwrap();
    let before = app.clone();
    for path in [
        format!("/api/v1/library/works/{work}"),
        format!("/api/v1/library/versions/{original}"),
        format!("/api/v1/library/versions/{copy}"),
    ] {
        let v = scry::api::library_read(&app, &path, "").unwrap();
        assert_eq!(v["repairs"][0]["original"], original);
        assert_eq!(v["repairs"][0]["copy"], copy);
    }
    let export = scry::api::library_read(&app, "/api/v1/library/export", "").unwrap();
    assert_eq!(export["format"], scry::text_repair::LIBRARY_FORMAT);
    assert!(export.get("works").is_none());
    assert_eq!(export.as_object().unwrap().len(), 3);
    let bytes = serde_json::to_vec(&export).unwrap();
    assert_eq!(
        scry::text_repair::decode_library_export(&bytes).unwrap(),
        app.library
    );
    std::fs::write(artifact("repaired-library.json"), &bytes).unwrap();
    assert!(
        scry::text_repair::decode_library_export(&serde_json::to_vec(&app.library).unwrap())
            .is_err()
    );
    let mut unsupported = export.clone();
    unsupported["format"] = serde_json::json!("future-library-format");
    assert!(
        scry::text_repair::decode_library_export(&serde_json::to_vec(&unsupported).unwrap())
            .is_err()
    );
    assert_eq!(app, before);
}
fn historical(fixture: &str, state: &str) -> App {
    let source = match fixture {
        "empty" => include_str!("fixtures/empty-unit-legacy.json"),
        "split" => include_str!("fixtures/split-unit-legacy.json"),
        _ => panic!("unknown synthetic fixture"),
    };
    let f: Value = serde_json::from_str(source).unwrap();
    let app: App = serde_json::from_value(f[state].clone()).unwrap();
    engine::validate_app(&app).unwrap();
    app
}
fn assert_repair_preserves(mut app: App, label: &str) {
    engine::validate_app(&app).unwrap();
    let before = app.clone();
    let rows = persistence::rows(&app).unwrap();
    let practice = serde_json::to_vec(&app.practice).unwrap();
    let id = app.library.versions.keys().next().unwrap().clone();
    let work = app.library.versions[&id].work_id.clone();
    let f = repair_request(&app);
    let result = engine::mutate(
        &mut app,
        "/library/version/repair",
        &f,
        1800000000100,
        label,
    )
    .unwrap();
    engine::validate_app(&app).unwrap();
    assert_ne!(result.entity_id.as_deref(), Some(id.as_str()));
    let mut expected = before.clone();
    expected.library = app.library.clone();
    expected.operations = app.operations.clone();
    expected.revision += 1;
    assert_eq!(app, expected, "{label}");
    assert_eq!(
        serde_json::to_vec(&app.practice).unwrap(),
        practice,
        "{label}"
    );
    assert_eq!(app.operations.len(), before.operations.len() + 1, "{label}");
    assert_eq!(
        app.library.works.len(),
        before.library.works.len(),
        "{label}"
    );
    assert_eq!(
        app.library.versions.len(),
        before.library.versions.len() + 1,
        "{label}"
    );
    assert_eq!(
        app.library.repairs.len(),
        before.library.repairs.len() + 1,
        "{label}"
    );
    assert_eq!(
        app.library.relations.len(),
        before.library.relations.len() + 1,
        "{label}"
    );
    let mut expected_work = before.library.works[&work].clone();
    expected_work.revision += 1;
    assert_eq!(app.library.works[&work], expected_work, "{label}");
    let after = persistence::rows(&app).unwrap();
    for (k, v) in &rows {
        if k != "meta" && k != &format!("library_works/{work}") {
            assert_eq!(after.get(k), Some(v), "{label}: changed {k}");
        }
    }
    let archive = persistence::Archive::new(app.clone(), vec![], 1800000000100);
    let bytes = archive.encode().unwrap();
    let restored = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(restored.app, app);
    assert_eq!(persistence::app_from_rows(&after).unwrap(), app);
    engine::mutate(
        &mut app,
        "/library/version/repair",
        &f,
        1800000000200,
        "replay-unused",
    )
    .unwrap();
    assert_eq!(persistence::rows(&app).unwrap(), after);
}
#[test]
fn historical_empty_and_split_state_matrix() {
    let mut report = Vec::new();
    for fixture in ["empty", "split"] {
        for state in [
            "unstarted",
            "source",
            "quiz",
            "answered",
            "whole_quiz",
            "whole_answered",
        ] {
            if fixture == "empty" && state == "whole_answered" {
                continue;
            }
            let app = historical(fixture, state);
            let label = format!("{fixture}-{state}");
            report.push(serde_json::json!({"case":label,"attempts":app.practice.attempts.len(),"scheduled_effects":app.practice.attempts.values().flat_map(|a|&a.effects).filter(|e|e.scheduled).count(),"held_result":app.practice.current.as_ref().and_then(|p|p.attempt_id.as_ref()).is_some(),"status":"pass"}));
            assert_repair_preserves(app, &label);
        }
    }
    std::fs::write(
        artifact("historical-matrix.json"),
        serde_json::to_vec_pretty(&report).unwrap(),
    )
    .unwrap();
}
#[test]
fn paused_dropped_and_checkpoint_matrix() {
    for fixture in ["empty", "split"] {
        for action in ["pause", "drop"] {
            let mut app = historical(fixture, "answered");
            let sid = app.practice.studies.keys().next().unwrap().clone();
            let f = mutation_fields(&app, "synthetic-state-0001", &[("study_id", sid.clone())]);
            engine::mutate(
                &mut app,
                &format!("/practice/{action}"),
                &f,
                1800000000050,
                "state-change",
            )
            .unwrap();
            engine::validate_app(&app).unwrap();
            assert_eq!(
                app.practice.studies[&sid].status,
                if action == "pause" {
                    scry::study::Status::Paused
                } else {
                    scry::study::Status::Dropped
                }
            );
            assert_repair_preserves(app, &format!("{fixture}-{action}"));
        }
    }
    assert_repair_preserves(checkpoint_app(), "saved-unicode-draft");
    let mut app = checkpoint_app();
    let pid = app.practice.current.as_ref().unwrap().prompt.id.clone();
    let generation = app.practice.answer_checkpoint.as_ref().unwrap().generation;
    let f = mutation_fields(
        &app,
        "synthetic-discard-0001",
        &[("prompt_id", pid), ("generation", generation.to_string())],
    );
    engine::mutate(
        &mut app,
        "/practice/draft/discard",
        &f,
        1800000000050,
        "discard",
    )
    .unwrap();
    assert!(
        app.practice
            .answer_checkpoint
            .as_ref()
            .unwrap()
            .draft
            .is_none()
    );
    assert_repair_preserves(app, "checkpoint-without-draft");
}
#[test]
fn retained_synthetic_corrections_and_schedule_matrix() {
    // Historical-only fixture seam. Runtime admission continues to return 410.
    assert_eq!(
        engine::admit_current_mutation("/practice/correct")
            .unwrap_err()
            .status,
        410
    );
    for fixture in ["empty", "split"] {
        let mut app = historical(fixture, "answered");
        let p = app.practice.current.as_ref().unwrap();
        let aid = p.attempt_id.as_ref().unwrap().clone();
        let f = mutation_fields(
            &app,
            "synthetic-correction-0001",
            &[
                ("prompt_id", p.prompt.id.clone()),
                ("attempt_id", aid),
                ("verdict", "Again".into()),
            ],
        );
        engine::mutate(
            &mut app,
            "/practice/correct",
            &f,
            1800000000050,
            "historical-correction",
        )
        .unwrap();
        engine::validate_app(&app).unwrap();
        assert_eq!(app.practice.corrections.len(), 1);
        assert_repair_preserves(app, &format!("{fixture}-correction"));
    }
}
#[test]
fn malformed_export_and_archive_matrix() {
    let mut app = checkpoint_app();
    let f = repair_request(&app);
    let original = app.library.versions.keys().next().unwrap().clone();
    let copy = engine::mutate(
        &mut app,
        "/library/version/repair",
        &f,
        1800000000001,
        "malformed-matrix",
    )
    .unwrap()
    .entity_id
    .unwrap();
    let before = app.clone();
    let export = scry::text_repair::export_library(&app.library).unwrap();
    let mut cases = Vec::new();
    let mut v = export.clone();
    v["policy"] = serde_json::json!("unsupported-policy");
    cases.push(("policy mismatch", v));
    let mut v = export.clone();
    v["library"].as_object_mut().unwrap().remove("repairs");
    cases.push(("stripped lineage", v));
    for id in [&original, &copy] {
        let mut v = export.clone();
        v["library"]["versions"].as_object_mut().unwrap().remove(id);
        cases.push(("missing endpoint", v));
    }
    let mut v = export.clone();
    v["library"]["repairs"][&original]
        .as_object_mut()
        .unwrap()
        .remove("fingerprint");
    cases.push(("stripped fingerprint", v));
    for (label, value) in cases {
        assert!(
            scry::text_repair::decode_library_export(&serde_json::to_vec(&value).unwrap()).is_err(),
            "{label}"
        );
    }
    let archive = serde_json::to_value(persistence::Archive::new(
        app.clone(),
        vec![],
        1800000000002,
    ))
    .unwrap();
    for change in ["strip", "original", "copy", "policy"] {
        let mut value = archive.clone();
        match change {
            "strip" => {
                value["app"]["library"]
                    .as_object_mut()
                    .unwrap()
                    .remove("repairs");
            }
            "original" | "copy" => {
                value["app"]["library"]["versions"]
                    .as_object_mut()
                    .unwrap()
                    .remove(if change == "original" {
                        &original
                    } else {
                        &copy
                    });
            }
            _ => {
                value["app"]["library"]["repairs"][&original]["policy"] =
                    serde_json::json!("unsupported-policy");
            }
        }
        let bytes = serde_json::to_vec(&value).unwrap();
        assert!(
            persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).is_err(),
            "{change}"
        );
    }
    assert_eq!(app, before);
}
fn native_step(app: &mut App, path: &str, extra: &[(&str, String)]) -> scry::model::MutationResult {
    let operation = format!("repair-learning-step-{:08}", app.revision);
    let f = mutation_fields(app, &operation, extra);
    let result = engine::mutate(app, path, &f, 1800000001000, &operation).unwrap();
    engine::validate_app(app).unwrap();
    result
}
fn answer_current_exact(app: &mut App) {
    let p = app.practice.current.as_ref().unwrap().prompt.clone();
    let v = &app.library.versions[&p.version_id];
    let text = v
        .sections
        .iter()
        .flat_map(|s| &s.units)
        .filter(|u| p.unit_ids.contains(&u.id))
        .map(|u| u.text.as_str())
        .collect::<String>();
    native_step(
        app,
        "/practice/answer",
        &[("prompt_id", p.id), ("answer", text)],
    );
    let a = &app.practice.attempts[app
        .practice
        .current
        .as_ref()
        .unwrap()
        .attempt_id
        .as_ref()
        .unwrap()];
    assert_eq!(a.grade.verdict, scry::grading::Verdict::Good);
    assert!(
        a.grade
            .units
            .iter()
            .all(|u| u.verdict == scry::grading::Verdict::Good)
    );
    assert_eq!(a.grade.authority.name, "exact-v2");
}
#[test]
fn explicit_enrollment_grades_copy_units_sections_whole_original_stays_quarantined() {
    for fixture in ["empty", "split"] {
        for scope in ["unit", "section", "whole"] {
            let mut app = historical(fixture, "unstarted");
            let original = app.library.versions.keys().next().unwrap().clone();
            let original_version = app.library.versions[&original].clone();
            let f = repair_request(&app);
            let copy = engine::mutate(
                &mut app,
                "/library/version/repair",
                &f,
                1800000000100,
                "repair-for-learning",
            )
            .unwrap()
            .entity_id
            .unwrap();
            assert!(app.practice.studies.is_empty());
            assert!(app.practice.cards.is_empty());
            assert!(app.practice.current.is_none());
            for path in [
                "/practice/learn",
                "/practice/study/start",
                "/practice/lesson/start",
            ] {
                let before = app.clone();
                let f = mutation_fields(
                    &app,
                    "quarantined-original-0001",
                    &[("version_id", original.clone())],
                );
                assert_eq!(
                    engine::mutate(&mut app, path, &f, 1800000000200, "refuse-original")
                        .unwrap_err()
                        .status,
                    422
                );
                assert_eq!(app, before);
            }
            let sid = native_step(&mut app, "/practice/learn", &[("version_id", copy.clone())])
                .entity_id
                .unwrap();
            assert_eq!(app.practice.studies.len(), 1);
            assert_eq!(app.practice.studies[&sid].version_id, copy);
            native_step(&mut app, "/practice/next", &[("study_id", sid.clone())]);
            assert_eq!(
                app.practice.current.as_ref().unwrap().prompt.version_id,
                copy
            );
            answer_current_exact(&mut app);
            if scope != "unit" {
                let after = app.practice.current.as_ref().unwrap().prompt.id.clone();
                let mut fields = vec![
                    ("after", after),
                    ("study_id", sid),
                    ("recite", scope.into()),
                ];
                if scope == "section" {
                    fields.push((
                        "section_id",
                        app.library.versions[&copy].sections[0].id.clone(),
                    ));
                }
                native_step(&mut app, "/practice/next", &fields);
                assert_eq!(
                    app.practice.current.as_ref().unwrap().prompt.version_id,
                    copy
                );
                answer_current_exact(&mut app);
            }
            assert_eq!(app.library.versions[&original], original_version);
            assert!(app.practice.studies.values().all(|s| s.version_id == copy));
            assert!(app.practice.attempts.values().all(|a| a.version_id == copy));
        }
    }
}

#[test]
fn repaired_multi_section_scope_and_negative_grading_matrix() {
    use scry::{
        grading::Verdict,
        library::{Unit, VersionInput},
        study::Scope,
    };
    let mut base = historical("split", "unstarted");
    base.library = Default::default();
    let text = "Use 3.14 for pi. Keep blue stars.\nCount green moons. Remember red suns.";
    let input: VersionInput = serde_json::from_value(serde_json::json!({"title":"Multi-section repair","kind":"text","text":text,"sections":null})).unwrap();
    let original = base
        .library
        .add(input, "multi-section-original", None)
        .unwrap();
    // Synthetic historical unsafe cut; source bytes and normalized hash remain unchanged.
    let v = base.library.versions.get_mut(&original).unwrap();
    let first = v.sections[0].units.remove(0);
    assert_eq!(first.text, "Use 3.14 for pi.");
    v.sections[0].units.insert(
        0,
        Unit {
            id: first.id.clone(),
            text: "Use 3.".into(),
        },
    );
    v.sections[0].units.insert(
        1,
        Unit {
            id: "abcdef1234567890abcdef1234567890".into(),
            text: "14 for pi.".into(),
        },
    );
    engine::validate_app(&base).unwrap();
    let original_snapshot = base.library.versions[&original].clone();
    let fields = repair_request(&base);
    let copy = engine::mutate(
        &mut base,
        "/library/version/repair",
        &fields,
        1800000000100,
        "multi-section-repair",
    )
    .unwrap()
    .entity_id
    .unwrap();
    let version = base.library.versions[&copy].clone();
    assert_eq!(version.text, text);
    assert_eq!(version.sections.len(), 2);
    assert_eq!(
        version
            .sections
            .iter()
            .map(|s| s.units.len())
            .collect::<Vec<_>>(),
        vec![2, 2]
    );
    assert!(base.practice.studies.is_empty());
    let sid = native_step(
        &mut base,
        "/practice/learn",
        &[("version_id", copy.clone())],
    )
    .entity_id
    .unwrap();
    native_step(&mut base, "/practice/next", &[("study_id", sid.clone())]);
    for scope in ["unit", "section", "whole"] {
        let mut ready = base.clone();
        if scope != "unit" {
            let mut seen = std::collections::BTreeSet::new();
            for _ in 0..32 {
                let p = ready.practice.current.as_ref().unwrap().prompt.clone();
                let flat = version
                    .sections
                    .iter()
                    .flat_map(|s| &s.units)
                    .collect::<Vec<_>>();
                let index = flat
                    .iter()
                    .position(|u| p.unit_ids == vec![u.id.clone()])
                    .unwrap();
                assert_eq!(
                    p.scope,
                    Scope::Unit {
                        unit_id: flat[index].id.clone(),
                        position: index + 1
                    }
                );
                seen.insert(index);
                answer_current_exact(&mut ready);
                if seen.len() == 4 {
                    break;
                }
                native_step(
                    &mut ready,
                    "/practice/next",
                    &[("after", p.id), ("study_id", sid.clone())],
                );
            }
            assert_eq!(seen.len(), 4);
            let after = ready.practice.current.as_ref().unwrap().prompt.id.clone();
            let mut f = vec![
                ("after", after),
                ("study_id", sid.clone()),
                ("recite", scope.into()),
            ];
            if scope == "section" {
                f.push(("section_id", version.sections[1].id.clone()));
            }
            native_step(&mut ready, "/practice/next", &f);
        }
        let prompt = ready.practice.current.as_ref().unwrap().prompt.clone();
        let expected = match scope {
            "unit" => {
                assert_eq!(
                    prompt.scope,
                    Scope::Unit {
                        unit_id: version.sections[0].units[0].id.clone(),
                        position: 1
                    }
                );
                vec![version.sections[0].units[0].clone()]
            }
            "section" => {
                assert_eq!(
                    prompt.scope,
                    Scope::Section {
                        section_id: version.sections[1].id.clone()
                    }
                );
                version.sections[1].units.clone()
            }
            _ => {
                assert_eq!(prompt.scope, Scope::Whole);
                version
                    .sections
                    .iter()
                    .flat_map(|s| s.units.clone())
                    .collect::<Vec<_>>()
            }
        };
        assert_eq!(prompt.version_id, copy);
        assert_eq!(
            prompt.unit_ids,
            expected.iter().map(|u| u.id.clone()).collect::<Vec<_>>()
        );
        let exact = expected.iter().map(|u| u.text.as_str()).collect::<String>();
        let omission = if expected.len() > 1 {
            expected[..expected.len() - 1]
                .iter()
                .map(|u| u.text.as_str())
                .collect::<String>()
        } else {
            "Use for pi.".into()
        };
        let crossed = if expected.len() > 1 {
            expected
                .iter()
                .rev()
                .map(|u| u.text.as_str())
                .collect::<String>()
        } else {
            version.sections[0].units[1].text.clone()
        };
        for (kind, answer, want) in [
            ("exact", exact, Verdict::Good),
            ("omission", omission, Verdict::Again),
            ("cross-boundary", crossed, Verdict::Again),
        ] {
            let mut app = ready.clone();
            native_step(
                &mut app,
                "/practice/answer",
                &[("prompt_id", prompt.id.clone()), ("answer", answer)],
            );
            let attempt = &app.practice.attempts[app
                .practice
                .current
                .as_ref()
                .unwrap()
                .attempt_id
                .as_ref()
                .unwrap()];
            assert_eq!(attempt.grade.verdict, want, "{scope}/{kind}");
            assert_eq!(attempt.grade.units.len(), expected.len(), "{scope}/{kind}");
            assert_eq!(
                attempt
                    .grade
                    .units
                    .iter()
                    .map(|u| u.unit_index)
                    .collect::<Vec<_>>(),
                expected
                    .iter()
                    .map(|u| version
                        .sections
                        .iter()
                        .flat_map(|s| &s.units)
                        .position(|v| v.id == u.id)
                        .unwrap())
                    .collect::<Vec<_>>()
            );
            if want == Verdict::Good {
                assert!(
                    attempt
                        .grade
                        .units
                        .iter()
                        .all(|u| u.verdict == Verdict::Good)
                );
            } else {
                assert!(
                    attempt
                        .grade
                        .units
                        .iter()
                        .any(|u| u.verdict == Verdict::Again)
                );
            }
            assert_eq!(app.library.versions[&original], original_snapshot);
            assert!(app.practice.attempts.values().all(|a| a.version_id == copy));
        }
    }
}

#[test]
fn recovery_renderer_is_metadata_only_and_requires_internal_gate() {
    let mut app = checkpoint_app();
    let original = app.library.versions.keys().next().unwrap().clone();
    let text = app.library.versions[&original].text.clone();
    let ordinary = std::collections::BTreeMap::new();
    assert!(
        !scry::web::render(&app, "/recall", &ordinary, 1800000000000, None)
            .contains("data-repair-link")
    );
    let query = std::collections::BTreeMap::from([
        ("__repair_enabled".into(), "true".into()),
        ("version_id".into(), original.clone()),
    ]);
    let before = app.clone();
    let page = scry::web::render(&app, "/library/repair", &query, 1800000000000, None);
    assert!(page.contains("data-repair-form"));
    assert!(page.contains("data-repair-draft"));
    assert!(page.contains("generation 1"));
    assert!(!page.contains(&text));
    assert!(!page.contains("Preserve café"));
    assert_eq!(app, before);
    let fields = repair_request(&app);
    let copy = engine::mutate(
        &mut app,
        "/library/version/repair",
        &fields,
        1800000000100,
        "renderer-repair",
    )
    .unwrap()
    .entity_id
    .unwrap();
    let page = scry::web::render(&app, "/library/repair", &query, 1800000000200, None);
    assert!(page.contains(&copy));
    assert!(page.contains("Enroll copy without switching"));
    assert!(!page.contains("data-repair-create"));
    assert!(!page.contains(&text));
    assert_eq!(app.practice, before.practice);
}

#[test]
fn recovery_without_current_prompt_has_no_mutation_controls() {
    let mut app = historical("split", "unstarted");
    let original = app.library.versions.keys().next().unwrap().clone();
    let query = std::collections::BTreeMap::from([
        ("__repair_enabled".into(), "true".into()),
        ("version_id".into(), original),
    ]);
    for repaired in [false, true] {
        if repaired {
            let fields = repair_request(&app);
            engine::mutate(
                &mut app,
                "/library/version/repair",
                &fields,
                1800000000100,
                "unstarted-renderer",
            )
            .unwrap();
        }
        let before = app.clone();
        let html = scry::web::render(&app, "/library/repair", &query, 1800000000200, None);
        assert!(html.contains("data-state=\"repair-no-held-prompt\""));
        assert!(html.contains("href=\"/library\" data-repair-close"));
        assert!(!html.contains("data-repair-form"));
        assert!(!html.contains("data-repair-enroll"));
        assert!(!html.contains("Your current prompt has not switched"));
        assert_eq!(app, before);
        assert!(app.practice.current.is_none());
        assert!(app.practice.studies.is_empty());
    }
}

#[test]
fn repair_different_version_preserves_held_prompt_and_draft() {
    use scry::library::{Unit, VersionInput};
    let mut app = checkpoint_app();
    let held = app.practice.clone();
    let input:VersionInput=serde_json::from_value(serde_json::json!({"title":"Other synthetic unsafe text","kind":"text","text":"Balance 6.28 meters. Keep this second text.","sections":null})).unwrap();
    let other = app
        .library
        .add(input, "other-unsafe-original", None)
        .unwrap();
    let version = app.library.versions.get_mut(&other).unwrap();
    let first = version.sections[0].units.remove(0);
    version.sections[0].units.insert(
        0,
        Unit {
            id: first.id,
            text: "Balance 6.".into(),
        },
    );
    version.sections[0].units.insert(
        1,
        Unit {
            id: "abcdef1234567890abcdef1234567891".into(),
            text: "28 meters.".into(),
        },
    );
    engine::validate_app(&app).unwrap();
    std::fs::write(
        artifact("different-held-archive.json"),
        persistence::Archive::new(app.clone(), vec![], 1800000000002)
            .encode()
            .unwrap(),
    )
    .unwrap();
    let query = std::collections::BTreeMap::from([
        ("__repair_enabled".into(), "true".into()),
        ("version_id".into(), other.clone()),
    ]);
    let page = scry::web::render(&app, "/library/repair", &query, 1800000000003, None);
    assert!(page.contains("data-repair-other-prompt"));
    assert!(page.contains(&held.current.as_ref().unwrap().prompt.version_id));
    assert!(!page.contains("Balance 6.28"));
    let version = app.library.versions[&other].clone();
    let revision = app.library.works[&version.work_id].revision;
    let fields = mutation_fields(
        &app,
        "different-held-repair-001",
        &[
            ("version_id", other.clone()),
            ("revision", revision.to_string()),
            (
                "fingerprint",
                scry::text_repair::preview(&app.library, &other).unwrap(),
            ),
        ],
    );
    let copy = engine::mutate(
        &mut app,
        "/library/version/repair",
        &fields,
        1800000000004,
        "different-held-repair",
    )
    .unwrap()
    .entity_id
    .unwrap();
    assert_eq!(app.practice, held);
    assert_eq!(app.library.versions[&other], version);
    let current = app.practice.current.clone();
    let draft = app.practice.answer_checkpoint.clone();
    native_step(&mut app, "/practice/learn", &[("version_id", copy)]);
    assert_eq!(app.practice.current, current);
    assert_eq!(app.practice.answer_checkpoint, draft);
    let page = scry::web::render(&app, "/library/repair", &query, 1800000000005, None);
    assert!(page.contains("data-repair-other-prompt"));
    assert!(page.contains("Your current prompt and its saved answer are unchanged."));
}
