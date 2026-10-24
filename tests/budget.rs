use scry::{
    budget::{self, Policy},
    engine::{self, Work, WorkConfig},
    model::*,
    persistence,
};
use std::collections::BTreeMap;
const NOW: i64 = 1_792_800_000_000;
const MAX: u64 = 1_071_576;
fn policy(cap: u64) -> Policy {
    Policy {
        contract: budget::CONTRACT.into(),
        monthly_micros: cap,
        request_micros: MAX,
    }
}
fn config() -> WorkConfig {
    WorkConfig {
        generation: true,
        jev: true,
        critic: false,
        model: budget::MODEL.into(),
        jev_model: "unbounded-jev".into(),
    }
}
fn fresh() -> App {
    App::new("synthetic-csrf-at-least-32-characters".into())
}
fn capture(app: &mut App, n: u64) {
    let f = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), format!("budget-capture-{n:016}")),
        ("intent".into(), format!("Learn cache validation topic {n}")),
    ]);
    engine::mutate(app, "/create", &f, NOW, &format!("capture-{n}")).unwrap();
}
fn claim(app: &mut App, n: u64, cap: u64) -> Option<Work> {
    budget::claim_work(app, NOW, &format!("{n:032x}"), &config(), Ok(policy(cap))).unwrap()
}
fn restart(app: &App) -> App {
    engine::validate_app(app).unwrap();
    let restarted = persistence::app_from_rows(&persistence::rows(app).unwrap()).unwrap();
    engine::validate_app(&restarted).unwrap();
    assert_eq!(&restarted, app);
    restarted
}

#[test]
fn exact_month_and_request_bound_reserve_before_send_and_refuse_one_micro_over() {
    let mut app = fresh();
    capture(&mut app, 1);
    let work = claim(&mut app, 1, MAX).unwrap();
    assert_eq!(app.spend[0].reserved_micros, MAX);
    assert_eq!(work.request["provider"]["allow_fallbacks"], false);
    assert_eq!(
        work.request["provider"]["only"],
        serde_json::json!(["openai"])
    );
    let mut app = restart(&app);
    budget::authorize_send(&app, &work, &policy(MAX), NOW).unwrap();
    capture(&mut app, 2);
    assert!(claim(&mut app, 2, MAX).is_none());
    assert_eq!(app.spend.len(), 1);
    assert_eq!(budget::committed(&app, NOW).unwrap(), MAX);
    let mut below = fresh();
    capture(&mut below, 3);
    let mut limit = policy(MAX);
    limit.request_micros = MAX - 1;
    assert!(
        budget::claim_work(
            &mut below,
            NOW,
            "33333333333333333333333333333333",
            &config(),
            Ok(limit)
        )
        .unwrap()
        .is_none()
    );
    assert!(below.spend.is_empty());
    restart(&below);
}

#[test]
fn disabled_invalid_policy_and_unsupported_models_never_admit_a_send() {
    for raw in [
        "",
        "{}",
        "null",
        "{\"contract\":\"unknown\",\"monthly_micros\":100,\"request_micros\":10}",
    ] {
        let mut app = fresh();
        capture(&mut app, 1);
        assert!(
            budget::claim_work(
                &mut app,
                NOW,
                "44444444444444444444444444444444",
                &config(),
                Policy::parse(raw)
            )
            .unwrap()
            .is_none()
        );
        assert!(app.spend.is_empty());
        restart(&app);
    }
    let mut app = fresh();
    capture(&mut app, 1);
    let mut cfg = config();
    cfg.model = "unreviewed-provider-model".into();
    assert!(
        budget::claim_work(
            &mut app,
            NOW,
            "55555555555555555555555555555555",
            &cfg,
            Ok(policy(MAX))
        )
        .unwrap()
        .is_none()
    );
    assert!(app.spend.is_empty());
}

#[test]
fn concurrent_claims_share_one_remaining_budget_and_replay_cannot_resend() {
    let mut app = fresh();
    capture(&mut app, 1);
    capture(&mut app, 2);
    let work = claim(&mut app, 1, MAX).unwrap();
    // The runtime serializes these synchronous transitions under its revision
    // fence. A second claim sees the first persisted hold, not a stale balance.
    let mut app = restart(&app);
    assert!(claim(&mut app, 2, MAX).is_none());
    assert_eq!(app.spend.len(), 1);
    engine::fail_work(&mut app, &work, "Synthetic cancelled send", false, NOW + 1).unwrap();
    assert_eq!(budget::committed(&app, NOW + DAY_MS * 40).unwrap(), MAX);
    assert!(budget::authorize_send(&app, &work, &policy(MAX), NOW + 2).is_err());
    restart(&app);
}

#[test]
fn month_rollover_unknown_transport_and_manual_resolution_are_durable_and_immutable() {
    let mut app = fresh();
    capture(&mut app, 1);
    let work = claim(&mut app, 1, MAX).unwrap();
    assert!(
        budget::reconcile(&mut app, &work.lease, 0, "provider-receipt-001", NOW + 1).is_err(),
        "Cannot free an in-flight reservation."
    );
    engine::fail_work(
        &mut app,
        &work,
        "Synthetic transport uncertainty",
        true,
        NOW + 1,
    )
    .unwrap();
    let later = NOW + DAY_MS * 40;
    let mut app = restart(&app);
    assert_eq!(budget::committed(&app, later).unwrap(), MAX);
    budget::reconcile(&mut app, &work.lease, 123, "provider-receipt-001", later).unwrap();
    assert_eq!(
        app.spend[0].cost_micros, None,
        "The original provider receipt is not rewritten."
    );
    let app = restart(&app);
    assert_eq!(budget::committed(&app, later).unwrap(), 123);
    let mut replay = app.clone();
    budget::reconcile(
        &mut replay,
        &work.lease,
        123,
        "provider-receipt-001",
        later + 1,
    )
    .unwrap();
    assert_eq!(replay, app);
    assert!(
        budget::reconcile(
            &mut replay,
            &work.lease,
            0,
            "provider-receipt-002",
            later + 1
        )
        .is_err()
    );
    assert_eq!(replay, app);
}

#[test]
fn late_overquote_cost_is_retained_and_blocks_future_months() {
    let mut app = fresh();
    capture(&mut app, 1);
    let work = claim(&mut app, 1, MAX * 2).unwrap();
    let raw = "{}".to_owned();
    let _ = engine::finish_work(
        &mut app,
        &work,
        raw.clone(),
        budget::MODEL.into(),
        Some(MAX + 1),
        NOW + 1,
    );
    assert_eq!(app.spend[0].cost_micros, Some(MAX + 1));
    assert!(budget::breach(&app.spend[0]));
    let mut app = restart(&app);
    assert!(budget::committed(&app, NOW + DAY_MS * 60).is_err());
    let saved = app.clone();
    engine::finish_work(
        &mut app,
        &work,
        raw,
        budget::MODEL.into(),
        Some(MAX + 1),
        NOW + 2,
    )
    .unwrap();
    assert_eq!(app, saved);
    assert!(budget::authorize_send(&app, &work, &policy(MAX * 2), NOW + 2).is_err());
}

#[test]
fn pre_send_rechecks_policy_request_source_lease_and_clock() {
    let mut app = fresh();
    capture(&mut app, 1);
    let work = claim(&mut app, 1, MAX * 2).unwrap();
    let mut changed = work.clone();
    changed.request["tools"] = serde_json::json!([]);
    assert!(budget::authorize_send(&app, &changed, &policy(MAX * 2), NOW).is_err());
    assert!(budget::authorize_send(&app, &work, &policy(MAX), NOW).is_err());
    assert!(budget::authorize_send(&app, &work, &policy(MAX * 2), NOW + 90_000).is_err());
    let goal = app.jobs[&work.id].goal_id.clone();
    app.goals.get_mut(&goal).unwrap().archived = true;
    assert!(budget::authorize_send(&app, &work, &policy(MAX * 2), NOW + 1).is_err());
}

#[test]
fn reconciliation_mutation_replay_and_archive_format_keep_original_receipt() {
    let mut app = fresh();
    capture(&mut app, 1);
    let work = claim(&mut app, 1, MAX).unwrap();
    engine::fail_work(&mut app, &work, "Synthetic unknown", true, NOW + 1).unwrap();
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        (
            "operation_id".into(),
            "budget-resolution-operation-001".into(),
        ),
        ("spend_id".into(), work.lease.clone()),
        ("cost_micros".into(), "0".into()),
        ("reference".into(), "synthetic-provider-bill-001".into()),
        ("verified".into(), "provider_billing".into()),
    ]);
    engine::mutate(&mut app, "/spend/reconcile", &fields, NOW + 2, "reconcile").unwrap();
    let saved = restart(&app);
    engine::mutate(
        &mut app,
        "/spend/reconcile",
        &fields,
        NOW + 3,
        "different-seed",
    )
    .unwrap();
    assert_eq!(app, saved);
    assert_eq!(app.spend[0].cost_micros, None);
    assert_eq!(budget::committed(&app, NOW + 3).unwrap(), 0);
    let archive = persistence::Archive::new(app.clone(), vec![], NOW + 3);
    assert_eq!(archive.format, "scry-rust-durable-sqlite-budget-v1");
    let mut wrong = fields;
    wrong.insert("cost_micros".into(), "1".into());
    assert_eq!(
        engine::mutate(
            &mut app,
            "/spend/reconcile",
            &wrong,
            NOW + 4,
            "different-seed"
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, saved);
}

#[test]
fn missing_cost_is_not_free_and_accounting_overflow_refuses() {
    let mut app = fresh();
    capture(&mut app, 1);
    let work = claim(&mut app, 1, MAX * 2).unwrap();
    let _ = engine::finish_work(
        &mut app,
        &work,
        "{}".into(),
        budget::MODEL.into(),
        None,
        NOW + 1,
    );
    let mut app = restart(&app);
    assert_eq!(budget::committed(&app, NOW + DAY_MS * 40).unwrap(), MAX);
    // A validated legacy ledger can contain large measured costs; never wrap.
    app.spend[0].quote = None;
    app.spend[0].reserved_micros = RESERVATION;
    app.spend[0].cost_micros = Some(u64::MAX);
    app.spend[0].status = "known".into();
    let mut second = app.spend[0].clone();
    second.id = "ffffffffffffffffffffffffffffffff".into();
    second.cost_micros = Some(1);
    app.spend.push(second);
    restart(&app);
    assert!(budget::committed(&app, NOW).is_err());
}
