use base64::{Engine as _, engine::general_purpose::STANDARD};
use scry::{
    engine,
    model::App,
    persistence::{self, Archive},
};
use serde_json::{Value, json};
use std::collections::BTreeMap;

fn imported() -> App {
    let bytes = b"SYNTHETIC_PRIVATE_ORIGINAL_GO_CUSTODY";
    let hash = persistence::sha256(bytes);
    let mut value =
        serde_json::to_value(App::new("synthetic-csrf-at-least-32-characters".into())).unwrap();
    value["revival_go"] = json!({format!("{hash}/00000000"): STANDARD.encode(bytes)});
    value["operations"] = json!({"old": {
        "payload_hash": "f".repeat(64),
        "result": {"redirect": "/history", "entity_id": null,
            "message": "legacy-import: original receipt retained in the Go archive."}
    }});
    serde_json::from_value(value).unwrap()
}

#[test]
fn retained_receipt_keys_and_hashes_roundtrip_without_rekeying() {
    let mut app = imported();
    let receipt = app.operations.remove("old").unwrap();
    for key in [
        "old".to_string(),
        "old/receipt:1".into(),
        "révision\tancienne".into(),
        "x".repeat(200),
    ] {
        app.operations.insert(key, receipt.clone());
    }
    let archive = Archive::new(app.clone(), vec![], 1_790_000_000_000);
    assert!(archive.diagnose().unwrap().is_empty());
    let bytes = archive.encode().unwrap();
    assert_eq!(
        Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        app
    );
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&app).unwrap()).unwrap(),
        app
    );
}

#[test]
fn imported_receipt_allowance_requires_custody_exact_marker_and_valid_hash() {
    let baseline = serde_json::to_value(imported()).unwrap();
    for mode in [
        "custody", "marker", "redirect", "entity", "hash", "blank", "oversize", "control",
    ] {
        let mut value = baseline.clone();
        match mode {
            "custody" => value["revival_go"] = json!({}),
            "marker" => value["operations"]["old"]["result"]["message"] = Value::Null,
            "redirect" => value["operations"]["old"]["result"]["redirect"] = json!("/"),
            "entity" => value["operations"]["old"]["result"]["entity_id"] = json!("missing"),
            "hash" => value["operations"]["old"]["payload_hash"] = json!("invalid"),
            _ => {
                let key = match mode {
                    "blank" => " ".to_string(),
                    "oversize" => "x".repeat(201),
                    _ => "bad\0id".into(),
                };
                let receipt = value["operations"]
                    .as_object_mut()
                    .unwrap()
                    .remove("old")
                    .unwrap();
                value["operations"] = json!({key: receipt});
            }
        }
        let app: App = serde_json::from_value(value).unwrap();
        assert_eq!(
            engine::diagnose_app(&app).unwrap()["an-invalid-exactoperation-receipt"],
            1,
            "{mode}"
        );
        assert!(
            Archive::new(app, vec![], 1_790_000_000_000)
                .encode()
                .is_err(),
            "{mode}"
        );
    }
}

#[test]
fn imported_receipt_shape_never_allows_a_new_non_native_request_identity() {
    let mut app = imported();
    for operation in [
        "old".to_string(),
        "old/receipt:1".into(),
        "révision\tancienne".into(),
        "x".repeat(200),
    ] {
        let fields = BTreeMap::from([
            ("csrf".into(), app.csrf.clone()),
            ("operation_id".into(), operation),
            ("intent".into(), "synthetic intent".into()),
        ]);
        let before = app.clone();
        assert_eq!(
            engine::mutate(
                &mut app,
                "/create",
                &fields,
                1_790_000_000_000,
                "synthetic-id"
            )
            .unwrap_err()
            .status,
            422
        );
        assert_eq!(app, before);
    }
}
