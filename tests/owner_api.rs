use scry::{
    engine,
    model::*,
    owner_token::{self, MutationActor, OwnerToken},
    persistence,
};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
const SEED: &str = "abcdef0123456789abcdef0123456789abcdef0123456789";
fn fields(pairs: &[(&str, &str)]) -> BTreeMap<String, String> {
    pairs
        .iter()
        .map(|(k, v)| (k.to_string(), v.to_string()))
        .collect()
}
#[test]
fn owner_tokens_show_once_replay_conflict_revoke_and_old_archives() {
    let mut app = App::new("synthetic-csrf-at-least-24-chars".into());
    let request = fields(&[
        ("csrf", &app.csrf),
        ("operation_id", "owner-token-create-operation"),
        ("name", "build agent"),
    ]);
    let created = engine::mutate(&mut app, "/tokens/create", &request, NOW, SEED).unwrap();
    let id = created.entity_id.unwrap();
    let secret = owner_token::issuance_secret(SEED);
    let token = app.owner_tokens[&id].clone();
    assert!(token.verifies(&secret, NOW));
    assert!(!token.info.paid);
    let stored = serde_json::to_string(&app).unwrap();
    assert!(!stored.contains(&secret));
    assert!(!stored.contains(SEED));
    let committed = app.clone();
    engine::mutate(
        &mut app,
        "/tokens/create",
        &request,
        NOW + 1,
        "different-seed",
    )
    .unwrap();
    assert_eq!(app, committed);
    let mut changed = request.clone();
    changed.insert("name".into(), "other".into());
    assert_eq!(
        engine::mutate(&mut app, "/tokens/create", &changed, NOW, SEED)
            .unwrap_err()
            .status,
        409
    );
    let request = fields(&[
        ("operation_id", "revoke-other-token-operation"),
        ("token_id", &id),
    ]);
    engine::mutate_as(
        &mut app,
        "/tokens/revoke",
        &request,
        NOW + 2,
        SEED,
        MutationActor::Token(&token.info),
    )
    .unwrap();
    assert!(!app.owner_tokens[&id].verifies(&secret, NOW + 3));
    assert_eq!(
        app.operations["revoke-other-token-operation"]
            .acting_token
            .as_deref(),
        Some(id.as_str())
    );
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&app).unwrap()).unwrap(),
        app
    );
    engine::validate_app(&app).unwrap();
    let expiry_request = fields(&[
        ("csrf", &app.csrf),
        ("operation_id", "owner-token-utc-expiry-operation"),
        ("name", "paid build"),
        ("expires_at", "2028-01-01T00:00"),
        ("paid", "true"),
    ]);
    let issued = engine::mutate(
        &mut app,
        "/tokens/create",
        &expiry_request,
        NOW,
        "1234567890abcdef1234567890abcdef1234567890abcdef",
    )
    .unwrap();
    let dated = &app.owner_tokens[&issued.entity_id.unwrap()].info;
    assert_eq!(dated.expires_ms, Some(1_830_297_600_000));
    assert!(dated.paid);

    let mut old = persistence::rows(&App::new("synthetic-csrf-at-least-24-chars".into())).unwrap();
    let mut meta: serde_json::Value = serde_json::from_str(&old["meta"]).unwrap();
    meta.as_object_mut().unwrap().remove("owner_tokens");
    old.insert("meta".into(), meta.to_string());
    assert!(
        persistence::app_from_rows(&old)
            .unwrap()
            .owner_tokens
            .is_empty()
    );
    let recovered = persistence::app_from_rows(&old).unwrap();
    assert_eq!(
        persistence::rows(&recovered).unwrap(),
        old,
        "A legacy notebook without tokens retains exactly its storage rows."
    );
    assert!(
        serde_json::to_value(&recovered)
            .unwrap()
            .get("owner_tokens")
            .is_none(),
        "Historical archive application shape stays unchanged until a token is created."
    );
}
#[test]
fn token_expiry_and_paid_refusal_precede_reservation_and_transmission() {
    let mut app = App::new("synthetic-csrf-at-least-24-chars".into());
    let token = OwnerToken::issue("qa", Some(NOW + 100), false, NOW, SEED).unwrap();
    let secret = owner_token::issuance_secret(SEED);
    assert!(token.verifies(&secret, NOW + 99));
    assert!(!token.verifies(&secret, NOW + 100));
    app.owner_tokens
        .insert(token.info.id.clone(), token.clone());
    let request = fields(&[
        ("operation_id", "unpaid-create-operation-id"),
        ("intent", "synthetic private input"),
    ]);
    let before = app.clone();
    assert_eq!(
        engine::mutate_as(
            &mut app,
            "/create",
            &request,
            NOW,
            SEED,
            MutationActor::Token(&token.info)
        )
        .unwrap_err()
        .status,
        403
    );
    assert_eq!(app, before);
    assert!(app.spend.is_empty());
    let mut paid = token.clone();
    paid.info.paid = true;
    app.owner_tokens.insert(paid.info.id.clone(), paid.clone());
    engine::mutate_as(
        &mut app,
        "/create",
        &request,
        NOW,
        SEED,
        MutationActor::Token(&paid.info),
    )
    .unwrap();
    let job = app.jobs.values().next().unwrap().clone();
    assert_eq!(job.acting_token, Some(paid.info.id.clone()));
    app.owner_tokens
        .get_mut(&paid.info.id)
        .unwrap()
        .info
        .revoked_ms = Some(NOW + 1);
    assert_eq!(
        engine::authorize_work(&app, &job.id, NOW + 2)
            .unwrap_err()
            .status,
        403
    );
    let config = engine::WorkConfig {
        generation: true,
        jev: false,
        critic: false,
        model: "synthetic".into(),
        jev_model: String::new(),
    };
    assert!(
        engine::claim_work(&mut app, NOW + 2, "lease", &config)
            .unwrap()
            .is_none()
    );
    assert!(app.spend.is_empty());
}
#[test]
fn web_and_api_share_receipts_and_cookie_csrf() {
    let mut app = App::new("synthetic-csrf-at-least-24-chars".into());
    let token = OwnerToken::issue("agent", None, false, NOW, SEED).unwrap();
    app.owner_tokens
        .insert(token.info.id.clone(), token.clone());
    let mut request = fields(&[
        ("operation_id", "same-web-api-operation-id"),
        ("pace", "light"),
    ]);
    let result = engine::mutate_as(
        &mut app,
        "/settings",
        &request,
        NOW,
        SEED,
        MutationActor::Token(&token.info),
    )
    .unwrap();
    let committed = app.clone();
    request.insert("csrf".into(), app.csrf.clone());
    assert_eq!(
        engine::mutate(&mut app, "/settings", &request, NOW + 1, SEED).unwrap(),
        result
    );
    assert_eq!(app, committed);
    request.insert("csrf".into(), "wrong".into());
    assert_eq!(
        engine::mutate(&mut app, "/settings", &request, NOW, SEED)
            .unwrap_err()
            .status,
        403
    );
    assert_eq!(app, committed);
}
