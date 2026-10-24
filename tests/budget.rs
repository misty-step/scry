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
fn month_rollover_unknown_transport_and_late_provider_settlement_are_durable() {
    let mut app = fresh();
    capture(&mut app, 1);
    let work = claim(&mut app, 1, MAX).unwrap();
    engine::fail_work(&mut app, &work, "Synthetic lost response", true, NOW + 1).unwrap();
    let later = NOW + DAY_MS * 40;
    let mut app = restart(&app);
    assert_eq!(budget::committed(&app, later).unwrap(), MAX);
    // Only the existing provider-result receipt path can settle a charge. A late
    // result cannot publish stale learning, but its actual expense is retained.
    let raw = "{\"usage\":{\"cost\":0.000123}}".to_owned();
    assert!(
        engine::finish_work(
            &mut app,
            &work,
            raw.clone(),
            budget::MODEL.into(),
            Some(123),
            later
        )
        .is_err()
    );
    let saved = restart(&app);
    assert_eq!(budget::committed(&app, later).unwrap(), 123);
    engine::finish_work(
        &mut app,
        &work,
        raw.clone(),
        budget::MODEL.into(),
        Some(123),
        later + 1,
    )
    .unwrap();
    assert_eq!(app, saved);
    assert!(
        engine::finish_work(
            &mut app,
            &work,
            raw,
            budget::MODEL.into(),
            Some(0),
            later + 1
        )
        .is_err()
    );
    assert_eq!(app, saved);
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
fn no_caller_can_release_an_archived_or_restarted_unknown_hold() {
    use scry::owner_token::{MutationActor, OwnerToken};
    let mut app = fresh();
    capture(&mut app, 1);
    let work = claim(&mut app, 1, MAX).unwrap();
    let goal = app.jobs[&work.id].goal_id.clone();
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "budget-archive-operation-001".into()),
    ]);
    engine::mutate(
        &mut app,
        &format!("/goals/{goal}/archive"),
        &fields,
        NOW + 1,
        "archive",
    )
    .unwrap();
    let token = OwnerToken::issue(
        "synthetic paid caller",
        None,
        true,
        NOW,
        "0123456789abcdef0123456789abcdef0123456789abcdef",
    )
    .unwrap();
    app.owner_tokens
        .insert(token.info.id.clone(), token.clone());
    let mut app = restart(&app);
    let saved = app.clone();
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        (
            "operation_id".into(),
            "budget-false-reconciliation-001".into(),
        ),
        ("spend_id".into(), work.lease.clone()),
        ("cost_micros".into(), "0".into()),
        ("verified".into(), "provider_billing".into()),
        ("reference".into(), "made-up-receipt-001".into()),
    ]);
    for actor in [MutationActor::Browser, MutationActor::Token(&token.info)] {
        assert!(
            engine::mutate_as(
                &mut app,
                "/spend/reconcile",
                &fields,
                NOW + 2,
                "attestation",
                actor
            )
            .is_err()
        );
        assert_eq!(app, saved);
    }
    assert_eq!(budget::committed(&app, NOW + DAY_MS * 40).unwrap(), MAX);
    let archive = persistence::Archive::new(app.clone(), vec![], NOW + 3);
    assert_eq!(archive.format, "scry-rust-durable-sqlite-budget-v1");
    let bytes = archive.encode().unwrap();
    let restored = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(restored.app, app);
}

#[test]
fn exact_quote_fits_the_retained_daily_cap_at_claim_and_before_send() {
    for extra in [0, 1] {
        let mut app = fresh();
        for n in 1..=2 {
            capture(&mut app, n);
            let work = claim(&mut app, n, 10_000_000).unwrap();
            let cost = (DAILY_ALLOWANCE - 2 * MAX) / 2 + if n == 1 { extra } else { 0 };
            let _ = engine::finish_work(
                &mut app,
                &work,
                "{}".into(),
                budget::MODEL.into(),
                Some(cost),
                NOW + 1,
            );
        }
        capture(&mut app, 3);
        let third = claim(&mut app, 3, 10_000_000).unwrap();
        engine::fail_work(&mut app, &third, "Synthetic unknown", true, NOW + 1).unwrap();
        capture(&mut app, 4);
        let fourth = claim(&mut app, 4, 10_000_000);
        assert_eq!(fourth.is_some(), extra == 0);
        if let Some(work) = fourth {
            assert_eq!(app.spent(NOW), DAILY_ALLOWANCE);
            budget::authorize_send(&app, &work, &policy(10_000_000), NOW).unwrap();
        }
        assert!(app.spent(NOW) <= DAILY_ALLOWANCE);
        restart(&app);
    }
    // A pre-deployment legacy request can settle above its old fixed reserve
    // while a new bounded claim waits. Recheck the daily limit before sending.
    let mut app = fresh();
    capture(&mut app, 10);
    let legacy = engine::claim_work(&mut app, NOW, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", &config())
        .unwrap()
        .unwrap();
    capture(&mut app, 11);
    let pending = claim(&mut app, 11, 10_000_000).unwrap();
    let _ = engine::finish_work(
        &mut app,
        &legacy,
        "{}".into(),
        budget::MODEL.into(),
        Some(3_000_000),
        NOW + 1,
    );
    restart(&app);
    assert!(budget::authorize_send(&app, &pending, &policy(10_000_000), NOW + 1).is_err());
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
