mod common;

use base64::{Engine as _, engine::general_purpose::STANDARD};
use scry::{
    model::App,
    persistence::{self, Archive},
};
use serde_json::json;

fn retained_app(bytes: &[u8]) -> App {
    // Exercise the serialized boundary: an old build silently drops unknown
    // fields. The new archive identity must prevent accepting such a restore.
    let mut app =
        serde_json::to_value(App::new("synthetic-csrf-at-least-24-characters".into())).unwrap();
    let hash = persistence::sha256(bytes);
    let chunks = bytes
        .chunks(48 * 1024)
        .enumerate()
        .map(|(index, chunk)| (format!("{hash}/{index:08}"), json!(STANDARD.encode(chunk))))
        .collect::<serde_json::Map<_, _>>();
    app["revival_go"] = json!(chunks);
    serde_json::from_value(app).unwrap()
}

/// A recorded earlier notebook that also retains the Go bytes `bytes`.
fn earlier_with_custody(name: &str, bytes: &[u8]) -> (App, i64) {
    let (mut app, now) = common::earlier(name);
    let hash = persistence::sha256(bytes);
    app.revival_go = bytes
        .chunks(48 * 1024)
        .enumerate()
        .map(|(index, chunk)| (format!("{hash}/{index:08}"), STANDARD.encode(chunk)))
        .collect();
    (app, now)
}

#[test]
fn retained_go_bytes_survive_rows_and_complete_archives_without_becoming_learning() {
    let bytes = [
        b"SYNTHETIC_OPAQUE_GO_RECOVERY\0".as_slice(),
        &vec![0xff; 180_000],
    ]
    .concat();
    let app = retained_app(&bytes);
    let value = serde_json::to_value(&app).unwrap();
    assert_eq!(value["revival_go"].as_object().unwrap().len(), 4);
    assert!(app.goals.is_empty() && app.events.is_empty() && app.jobs.is_empty());
    let rows = persistence::rows(&app).unwrap();
    assert_eq!(
        rows.keys()
            .filter(|key| key.starts_with("revival_go/"))
            .count(),
        4
    );
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), app);
    let archive = Archive::new(app.clone(), vec![], 1_790_000_000_000);
    assert_eq!(archive.format, "scry-rust-durable-sqlite-go-custody-v1");
    let encoded = archive.encode().unwrap();
    assert_eq!(
        Archive::decode(&encoded, &persistence::sha256(&encoded))
            .unwrap()
            .app,
        app
    );
    let mut downgraded = archive;
    downgraded.format = persistence::ARCHIVE_FORMAT.into();
    assert!(
        downgraded.encode().is_err(),
        "Never label custody with the old lossy archive identity"
    );
}

#[test]
fn ordinary_old_archives_and_rows_remain_compatible_without_custody() {
    let app = App::new("synthetic-csrf-at-least-24-characters".into());
    let mut value = serde_json::to_value(&app).unwrap();
    value.as_object_mut().unwrap().remove("revival_go");
    let old: App = serde_json::from_value(value.clone()).unwrap();
    assert_eq!(old, app);
    let archive = Archive::new(old, vec![], 1_790_000_000_000);
    assert_eq!(archive.format, persistence::ARCHIVE_FORMAT);
    let mut rows = persistence::rows(&app).unwrap();
    rows.insert("meta".into(), value.to_string());
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), app);
    assert!(archive.encode().is_ok());
}

#[test]
fn malformed_or_changed_retained_bytes_fail_before_restore() {
    let original = serde_json::to_value(retained_app(&vec![42; 100_000])).unwrap();
    let chunks = original["revival_go"].as_object().unwrap();
    let keys = chunks.keys().cloned().collect::<Vec<_>>();
    for mode in [
        "changed",
        "gap",
        "mixed-hash",
        "short-middle",
        "empty",
        "invalid-base64",
        "noncanonical",
    ] {
        let mut value = original.clone();
        let map = value["revival_go"].as_object_mut().unwrap();
        match mode {
            "changed" => {
                map.insert(keys[1].clone(), json!(STANDARD.encode([41; 48 * 1024])));
            }
            "gap" => {
                map.remove(&keys[0]);
            }
            "mixed-hash" => {
                let data = map.remove(&keys[1]).unwrap();
                map.insert(format!("{}/00000001", "0".repeat(64)), data);
            }
            "short-middle" => {
                map.insert(keys[0].clone(), json!(STANDARD.encode([42])));
            }
            "empty" => {
                map.insert(keys[2].clone(), json!(""));
            }
            "invalid-base64" => {
                map.insert(keys[2].clone(), json!("not base64!"));
            }
            "noncanonical" => {
                map.insert(keys[2].clone(), json!("Kh=="));
            }
            _ => unreachable!(),
        }
        let app: App = serde_json::from_value(value).unwrap();
        let archive = Archive::new(app, vec![], 1_790_000_000_000);
        assert!(archive.encode().is_err(), "Accepted {mode}");
    }
}

#[test]
fn retained_custody_counts_toward_existing_complete_archive_capacity() {
    let app = retained_app(&vec![42; 13 * 1024 * 1024]);
    assert!(
        Archive::new(app, vec![], 1_790_000_000_000)
            .encode()
            .is_err()
    );
}

#[test]
fn original_go_schedule_epoch_is_explicit_preserved_and_fail_closed() {
    let (mut app, now) =
        earlier_with_custody("question-cold-choice", b"synthetic-original-go-recovery");
    let old = app.questions.keys().next().unwrap().clone();
    let mut question = app.questions.remove(&old).unwrap();
    question.id = format!("legacy-import-quiz-{old}");
    question.schedule_version = 1;
    let id = question.id.clone();
    app.questions.insert(id.clone(), question);
    let archive = |app| Archive::new(app, vec![], now + 1);
    assert!(archive(app.clone()).encode().is_err());
    app.revival_schedule_origins.insert(id.clone(), 1);
    let valid = archive(app.clone());
    assert_eq!(valid.format, "scry-rust-durable-sqlite-go-projection-v1");
    assert!(valid.encode().is_ok());
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&app).unwrap()).unwrap(),
        app
    );
    for mode in [
        "no-custody",
        "orphan",
        "wrong-origin",
        "gap",
        "wrong-identity",
    ] {
        let mut changed = app.clone();
        match mode {
            "no-custody" => changed.revival_go.clear(),
            "orphan" => {
                changed.revival_schedule_origins.insert("missing".into(), 1);
            }
            "wrong-origin" => {
                changed.revival_schedule_origins.insert(id.clone(), 2);
            }
            "gap" => changed.questions.get_mut(&id).unwrap().schedule_version = 2,
            _ => {}
        }
        let mut invalid = archive(changed);
        if mode == "wrong-identity" {
            invalid.format = "scry-rust-durable-sqlite-go-custody-v1".into();
        }
        assert!(invalid.encode().is_err(), "Accepted {mode}");
    }
}

#[test]
fn imported_helped_observations_do_not_invent_scheduler_writes() {
    // One answer recorded after help: rating 0, its first schedule epoch. The
    // held explanation and its queued check are not part of this history.
    let (mut app, now) = earlier_with_custody(
        "checking-queued-explain",
        b"synthetic-original-go-helped-history",
    );
    app.occurrence = None;
    app.assessments.clear();
    assert_eq!(app.events.len(), 1);
    let old = app.events[0].question_id.clone();
    assert_eq!(app.events[0].result.rating, 0);
    assert_eq!(app.questions[&old].schedule_version, 1);
    let mut question = app.questions.remove(&old).unwrap();
    let id = format!("legacy-import-quiz-{old}");
    question.id = id.clone();
    app.questions.insert(id.clone(), question);
    app.revival_schedule_origins.insert(id.clone(), 1);
    app.occurrence = None;
    let event = &mut app.events[0];
    event.question_id = id.clone();
    event.id = "legacy-import-event-first".into();
    event.result.event_id = event.id.clone();
    event.occurrence_id = "legacy-import-presentation-first".into();
    let mut second = event.clone();
    second.id = "legacy-import-event-second".into();
    second.result.event_id = second.id.clone();
    second.occurrence_id = "legacy-import-presentation-second".into();
    app.events.push(second);
    let archive = |app| Archive::new(app, vec![], now + 1);
    assert!(archive(app.clone()).encode().is_ok());
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&app).unwrap()).unwrap(),
        app
    );
    for mode in [
        "origin",
        "custody",
        "marker",
        "assistance",
        "card",
        "epoch",
        "duplicate",
    ] {
        let mut bad = app.clone();
        match mode {
            "origin" => bad.revival_schedule_origins.clear(),
            "custody" => bad.revival_go.clear(),
            "marker" => bad.events[1].occurrence_id = "ordinary-occurrence".into(),
            "assistance" => bad.events[1].assisted = false,
            "card" => {
                bad.events[1].before_card.due_ms += 1;
                bad.events[1].after_card.due_ms += 1;
                bad.events[1].result.due_ms += 1;
            }
            "epoch" => bad.events[1].schedule_version += 1,
            "duplicate" => bad.events.push(bad.events[0].clone()),
            _ => unreachable!(),
        }
        assert!(archive(bad).encode().is_err(), "Accepted {mode}");
    }
    // New Rust warm events still advance their own concurrency epoch.
    let mut rust = app.events[1].clone();
    rust.id = "ordinary-rust-event".into();
    rust.result.event_id = rust.id.clone();
    rust.occurrence_id = "ordinary-rust-occurrence".into();
    rust.schedule_version = 2;
    app.events.push(rust);
    app.questions.get_mut(&id).unwrap().schedule_version = 2;
    assert!(archive(app.clone()).encode().is_ok());
    app.questions.get_mut(&id).unwrap().schedule_version = 1;
    assert!(archive(app).encode().is_err());
}

#[test]
fn historical_ungraded_attempts_are_not_grades_and_keep_their_original_epoch() {
    // An attempt the Go application saved awaiting a judgment: an ungraded
    // observation on its original schedule epoch, kept with its Go bytes.
    let (mut app, now) = common::earlier("go-retained-self-check");
    let event = &app.events[0];
    assert_eq!(app.events.len(), 1);
    assert_eq!(
        (event.result.authority.as_str(), event.result.rating),
        ("ungraded", 0)
    );
    assert_eq!(app.revival_schedule_origins[&event.question_id], 1);
    assert!(!app.revival_go.is_empty());
    app.occurrence = None;
    let archive = |app| Archive::new(app, vec![], now + 1);
    archive(app.clone()).encode().unwrap();
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&app).unwrap()).unwrap(),
        app
    );
    for mode in [
        "marker",
        "custody",
        "authority",
        "card",
        "epoch",
        "answer",
        "kind",
    ] {
        let mut bad = app.clone();
        match mode {
            "marker" => bad.events[0].id = "ordinary-event".into(),
            "custody" => bad.revival_go.clear(),
            "authority" => bad.events[0].result.authority = "exact".into(),
            "card" => bad.events[0].after_card.due_ms += 1,
            "epoch" => bad.events[0].schedule_version += 1,
            "answer" => bad.events[0].answer = bad.events[0].presentation.answer.clone(),
            "kind" => bad.events[0].presentation.kind = "choice".into(),
            _ => unreachable!(),
        }
        assert!(archive(bad).encode().is_err(), "Accepted {mode}");
    }
}
