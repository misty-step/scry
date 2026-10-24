use scry::{engine::{self, WorkConfig}, model::*, persistence};
use std::collections::BTreeMap;

#[test]
fn unknown_provider_charge_remains_held_after_day_and_restart() {
    let now = 1_792_800_000_000;
    let mut app = App::new("synthetic-csrf-at-least-32-characters".into());
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "budget-baseline-capture-0001".into()),
        ("intent".into(), "Learn conditional cache validation".into()),
    ]);
    engine::mutate(&mut app, "/create", &fields, now, "budget-baseline-seed").unwrap();
    let config = WorkConfig { generation:true, jev:false, critic:false,
        model:"synthetic-model".into(), jev_model:String::new() };
    let work = engine::claim_work(&mut app, now, "00112233445566778899aabbccddeeff", &config).unwrap().unwrap();
    engine::fail_work(&mut app, &work, "Synthetic interrupted transport", true, now + 1).unwrap();
    engine::validate_app(&app).unwrap();
    let restarted = persistence::app_from_rows(&persistence::rows(&app).unwrap()).unwrap();
    engine::validate_app(&restarted).unwrap();
    assert_eq!(restarted.spent(now + DAY_MS + 1), RESERVATION,
        "An unresolved provider charge must not become free after 24 hours.");
}
