use base64::{Engine as _, engine::general_purpose::STANDARD};
use scry::{
    engine,
    model::*,
    persistence::{self, Archive},
    spend::{self, SpendBy},
};
use serde_json::json;

fn notebook() -> App {
    let mut app = App::new("synthetic-csrf-at-least-24-characters".into());
    engine::seed_fixture(&mut app, 1_791_410_400_000).unwrap();
    app.spend.push(serde_json::from_value(json!({
        "id":"synthetic-spend", "work_id":"fixture-job", "created_ms":1_791_410_400_000_i64,
        "reserved_micros":RESERVATION, "cost_micros":123, "status":"known", "model":"retained-model"
    })).unwrap());
    app
}

#[test]
fn historical_attribution_and_custody_roundtrip_without_invention() {
    for model in [None, Some(""), Some("retained-model")] {
        let mut app = notebook();
        app.spend[0].model = model.map(str::to_owned);
        let custody = b"SYNTHETIC_OPAQUE_RETAINED_BYTES\0\xff";
        app.revival_go.insert(
            format!("{}/00000000", persistence::sha256(custody)),
            STANDARD.encode(custody),
        );
        engine::validate_app(&app).unwrap();
        let rows = persistence::rows(&app).unwrap();
        let historical = rows.get("spend/0000000000000000").unwrap();
        assert!(!historical.contains("purpose"));
        if model.is_none() {
            assert!(
                serde_json::from_str::<serde_json::Value>(historical)
                    .unwrap()
                    .get("model")
                    .is_none()
            );
        }
        let loaded = persistence::app_from_rows(&rows).unwrap();
        assert_eq!(persistence::rows(&loaded).unwrap(), rows);
        let archive = Archive::new(loaded, vec![], 1_791_410_400_000);
        assert_eq!(archive.format, "scry-rust-durable-sqlite-go-custody-v1");
        let bytes = archive.encode().unwrap();
        let restored = Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
        assert_eq!(restored.encode().unwrap(), bytes);
        assert_eq!(persistence::rows(&restored.app).unwrap(), rows);
        let before = restored.app.clone();
        let report = spend::report(&restored.app, SpendBy::Purpose).unwrap();
        assert_eq!(report.entries[0].purpose, "unattributed");
        assert_eq!(
            report.entries[0].model,
            model.filter(|v| !v.is_empty()).unwrap_or("unattributed")
        );
        assert_eq!(report.entries[0].cost_micros, Some(123));
        assert_eq!(report.totals.known_cost_micros, 123);
        assert_eq!(restored.app, before);
    }
}

#[test]
fn attributed_archives_require_compatible_reader_and_preserve_all_cost_states() {
    let mut app = notebook();
    app.spend[0].purpose = Some("historical.custom".into());
    let mut unknown = app.spend[0].clone();
    unknown.id = "synthetic-unknown".into();
    unknown.status = "unknown".into();
    unknown.cost_micros = None;
    app.spend.push(unknown);
    let archive = Archive::new(app.clone(), vec![], 1_791_410_400_000);
    assert_eq!(
        archive.format,
        "scry-rust-durable-sqlite-attributed-spend-v1"
    );
    let bytes = archive.encode().unwrap();
    let restored = Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(restored.app, app);
    assert_eq!(restored.encode().unwrap(), bytes);
    assert_eq!(
        persistence::rows(&restored.app).unwrap(),
        persistence::rows(&app).unwrap()
    );
    let report = spend::report(&restored.app, SpendBy::Purpose).unwrap();
    assert_eq!(report.groups.len(), 1);
    assert_eq!(report.totals.reserved_micros, 2 * RESERVATION);
    assert_eq!(report.totals.known_cost_micros, 123);
    assert_eq!(report.totals.unknown_held_micros, RESERVATION);
    assert_eq!(report.entries[1].cost_micros, None);
    assert_eq!(app.spent(1_791_410_400_001), 123 + RESERVATION);
    let mut downgraded = archive;
    downgraded.format = persistence::ARCHIVE_FORMAT.into();
    assert!(downgraded.encode().is_err());
    let mut inconsistent = app;
    inconsistent.spend[1].status = "known".into();
    assert!(
        Archive::new(inconsistent, vec![], 1_791_410_400_000)
            .encode()
            .is_err()
    );
}

#[test]
fn reporting_never_saturates_totals_or_changes_the_rolling_allowance() {
    let mut app = notebook();
    app.spend[0].cost_micros = Some(u64::MAX);
    let mut next = app.spend[0].clone();
    next.id = "overflow".into();
    app.spend.push(next);
    assert_eq!(
        spend::report(&app, SpendBy::Model).err().unwrap().status,
        422
    );
    let mut app = notebook();
    let created = app.spend[0].created_ms;
    assert_eq!(app.spent(created + DAY_MS - 1), 123);
    assert_eq!(app.spent(created + DAY_MS), 0);
    app.spend[0].cost_micros = Some(0);
    assert_eq!(
        spend::report(&app, SpendBy::Day).unwrap().entries[0].cost_micros,
        Some(0)
    );
    assert!(SpendBy::parse("invalid").is_err());
}
