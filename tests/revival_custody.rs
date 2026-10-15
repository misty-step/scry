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
