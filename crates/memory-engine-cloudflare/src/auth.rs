//! Invite-only auth, stable native session identity, and fenced reminders.
//! All identity comes from stored credentials. Only the public Worker's
//! overwritten x-scry-client-ip / x-scry-edge-scheme headers are trusted.

use std::{fmt::Write as _, net::IpAddr};

use hmac::{Hmac, KeyInit, Mac};
use memory_engine_api_render::{
    render_app_shell, render_auth_recovery, render_entry_recovery, render_entry_requested,
    render_return_notification_confirmation, render_return_notification_disabled,
    render_return_notification_recovery,
};
use memory_engine_api_state::{
    normalize_email, AccountCreated, AppAccount, CreateAccountRequest,
    ScheduledReturnNotificationReport, WaitlistEntry,
};
use memory_engine_study::BetaStudySession;
use serde::{de::DeserializeOwned, Deserialize};
use serde_json::json;
use sha2::{Digest, Sha256};
use subtle::ConstantTimeEq;
use worker::{Env, Method, Request, Response};

use crate::{
    database::Database, mail, now_ms, random_id, store::SqlStudyStore, AppResult, Failure,
};

const SESSION_MS: i64 = 90 * 24 * 60 * 60_000;
const MAGIC_TTL_MS: i64 = 30 * 60_000;
const RATE_WINDOW_MS: i64 = 15 * 60_000;
const REMINDER_INTERVAL_MS: i64 = 24 * 60 * 60_000;
const UNSUBSCRIBE_TTL_MS: i64 = 7 * 24 * 60 * 60_000;
const CLAIM_TTL_MS: i64 = 5 * 60_000;
const REMINDER_CHECK_MS: i64 = 60_000;
const BODY_LIMIT: usize = 16 * 1024;
const SECURE_COOKIE: &str = "__Host-memory_engine_session";
const LOCAL_COOKIE: &str = "memory_engine_session";

#[derive(Deserialize)]
struct AccountId {
    account_id: String,
}
#[derive(Deserialize)]
struct BrowserSession {
    account_id: String,
    session_token_hash: String,
    csrf_token_hash: String,
    expires_at_ms: i64,
}
#[derive(Deserialize)]
struct Challenge {
    email_normalized: String,
}
#[derive(Deserialize)]
struct RateLimit {
    window_start_ms: i64,
    attempt_count: i64,
}
#[derive(Deserialize)]
struct WaitlistRow {
    email_normalized: String,
    source: String,
    created_at_ms: i64,
    updated_at_ms: i64,
    invited_at_ms: Option<i64>,
}
impl From<WaitlistRow> for WaitlistEntry {
    fn from(row: WaitlistRow) -> Self {
        Self {
            email: row.email_normalized,
            source: row.source,
            created_at_ms: row.created_at_ms,
            updated_at_ms: row.updated_at_ms,
            invited_at_ms: row.invited_at_ms,
        }
    }
}

pub(crate) fn secret_hash(value: &str) -> String {
    encode_hex(&Sha256::digest(value.as_bytes()))
}

pub(crate) fn encode_hex(bytes: &[u8]) -> String {
    let mut result = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        let _ = write!(result, "{byte:02x}");
    }
    result
}

fn decode_hex(value: &str) -> Option<Vec<u8>> {
    if !value.is_ascii() || !value.len().is_multiple_of(2) {
        return None;
    }
    (0..value.len())
        .step_by(2)
        .map(|index| u8::from_str_radix(&value[index..index + 2], 16).ok())
        .collect()
}

fn same_secret(left: &str, right: &str) -> bool {
    let left = Sha256::digest(left.as_bytes());
    let right = Sha256::digest(right.as_bytes());
    bool::from(left.as_slice().ct_eq(right.as_slice()))
}

fn same_hash(left: &str, right: &str) -> bool {
    bool::from(left.as_bytes().ct_eq(right.as_bytes()))
}

fn account_id_for(email: &str) -> String {
    let stable = email.bytes().fold(0xcbf2_9ce4_8422_2325_u64, |hash, byte| {
        (hash ^ u64::from(byte)).wrapping_mul(0x0000_0100_0000_01b3)
    });
    format!("acct_{stable:016x}")
}

fn csrf_for(session_token_hash: &str) -> String {
    format!(
        "csrf_{}",
        secret_hash(&format!("csrf:{session_token_hash}"))
    )
}

fn account_email(email: &str) -> AppResult<String> {
    normalize_email(email)
        .filter(|email| {
            email.len() <= 320
                && !email
                    .chars()
                    .any(|ch| ch.is_control() || ch.is_whitespace())
                && !email.contains(['<', '>', ',', ';'])
        })
        .ok_or_else(|| Failure::bad_request("Account email must contain one @ and a domain."))
}

fn configured_invite(env: &Env, email: &str) -> bool {
    mail::variable(env, "MEMORY_ENGINE_AUTH_ALLOWED_EMAILS").is_some_and(|list| {
        list.split(',')
            .filter_map(normalize_email)
            .any(|allowed| allowed == email)
    })
}

fn admitted(db: &Database, env: &Env, email: &str) -> AppResult<bool> {
    if configured_invite(env, email) {
        return Ok(true);
    }
    Ok(!db.query::<WaitlistRow>(
        "SELECT * FROM memory_engine_waitlist_entries WHERE email_normalized = ? AND invited_at_ms IS NOT NULL",
        &[json!(email)],
    )?.is_empty())
}

fn missing_session() -> Failure {
    Failure::new(401, "Session token is required.")
}
fn forbidden_account() -> Failure {
    Failure::forbidden("Session token does not match account.")
}

pub fn require_admin(req: &Request, env: &Env) -> AppResult<()> {
    let supplied = req.headers().get("x-admin-token")?.unwrap_or_default();
    require_secret(
        env,
        "MEMORY_ENGINE_ADMIN_TOKEN",
        &supplied,
        "Admin token is not authorized.",
    )
}

fn require_secret(env: &Env, name: &str, supplied: &str, message: &str) -> AppResult<()> {
    if mail::variable(env, name)
        .as_deref()
        .is_some_and(|configured| {
            !configured.trim().is_empty()
                && !supplied.is_empty()
                && same_secret(configured.trim(), supplied)
        })
    {
        Ok(())
    } else {
        Err(Failure::forbidden(message))
    }
}

fn api_token(req: &Request) -> AppResult<String> {
    let explicit = req.headers().get("x-session-token")?;
    let token = if let Some(token) = explicit
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
    {
        Some(token.to_owned())
    } else {
        req.headers().get("authorization")?.and_then(|value| {
            let (scheme, token) = value.trim().split_once(char::is_whitespace)?;
            (scheme.eq_ignore_ascii_case("bearer") && !token.trim().is_empty())
                .then(|| token.trim().to_owned())
        })
    };
    token
        .filter(|token| {
            token.len() <= 512
                && !(token.len() == 64 && token.bytes().all(|byte| byte.is_ascii_hexdigit()))
        })
        .ok_or_else(missing_session)
}

fn has_account(db: &Database, id: &str) -> AppResult<bool> {
    Ok(!db
        .query::<AccountId>(
            "SELECT account_id FROM memory_engine_accounts WHERE account_id = ?",
            &[json!(id)],
        )?
        .is_empty())
}

pub fn api_account(
    req: &Request,
    db: &Database,
    _env: &Env,
    expected_account_id: &str,
) -> AppResult<String> {
    let token = api_token(req)?;
    if !db.query::<AccountId>(
        "SELECT account_id FROM memory_engine_api_sessions WHERE account_id = ? AND session_token_hash = ?
         AND revoked_at_ms IS NULL AND expires_at_ms > ?",
        &[json!(expected_account_id), json!(secret_hash(&token)), json!(now_ms())],
    )?.is_empty() {
        return Ok(expected_account_id.to_owned());
    }
    if has_account(db, expected_account_id)? {
        Err(forbidden_account())
    } else {
        Err(Failure::not_found("Account not found."))
    }
}

fn cookie_value<'a>(cookies: &'a str, name: &str) -> Option<&'a str> {
    cookies.split(';').find_map(|cookie| {
        let (key, value) = cookie.trim().split_once('=')?;
        (key == name && !value.trim().is_empty()).then_some(value.trim())
    })
}

pub fn browser_session_cookie_present(req: &Request) -> bool {
    req.headers()
        .get("cookie")
        .ok()
        .flatten()
        .is_some_and(|cookies| {
            cookies.split(';').any(|cookie| {
                cookie
                    .trim()
                    .split_once('=')
                    .is_some_and(|(name, _)| name == SECURE_COOKIE || name == LOCAL_COOKIE)
            })
        })
}

fn secure_request(req: &Request) -> bool {
    req.headers()
        .get("x-scry-edge-scheme")
        .ok()
        .flatten()
        .as_deref()
        != Some("http")
}

fn local_environment(env: &Env) -> bool {
    matches!(
        mail::variable(env, "MEMORY_ENGINE_ENVIRONMENT").as_deref(),
        Some("local" | "development" | "test")
    )
}

fn browser_id(req: &Request, env: &Env) -> AppResult<String> {
    let cookies = req.headers().get("cookie")?.ok_or_else(missing_session)?;
    let id = cookie_value(&cookies, SECURE_COOKIE)
        .or_else(|| {
            (!secure_request(req) && local_environment(env))
                .then(|| cookie_value(&cookies, LOCAL_COOKIE))
                .flatten()
        })
        .ok_or_else(missing_session)?;
    if id.len() > 512
        || id
            .bytes()
            .any(|byte| !byte.is_ascii_alphanumeric() && byte != b'_' && byte != b'-')
    {
        return Err(missing_session());
    }
    Ok(id.to_owned())
}

pub fn browser_account(
    req: &Request,
    db: &Database,
    env: &Env,
    csrf: Option<&str>,
) -> AppResult<AppAccount> {
    if csrf.is_none() && !matches!(req.method(), Method::Get | Method::Head) {
        return Err(Failure::forbidden(
            "CSRF token is required for a browser mutation.",
        ));
    }
    if !secure_request(req) && !local_environment(env) {
        return Err(Failure::forbidden(
            "A secure browser connection is required.",
        ));
    }
    let session_id = browser_id(req, env)?;
    let id_hash = secret_hash(&session_id);
    let now = now_ms();
    db.transaction(|| {
        let mut session = db.query::<BrowserSession>(
            "SELECT account_id, session_token_hash, csrf_token_hash, expires_at_ms
             FROM memory_engine_browser_sessions WHERE session_id_hash = ?
             AND revoked_at_ms IS NULL AND expires_at_ms > ?",
            &[json!(id_hash), json!(now)],
        )?.pop().ok_or_else(missing_session)?;
        if let Some(csrf) = csrf {
            if csrf.is_empty() || !same_hash(&session.csrf_token_hash, &secret_hash(csrf)) {
                return Err(Failure::forbidden("CSRF token does not match session."));
            }
        }
        if db.query::<AccountId>(
            "SELECT account_id FROM memory_engine_api_sessions WHERE account_id = ? AND session_token_hash = ?
             AND revoked_at_ms IS NULL AND expires_at_ms > ?",
            &[json!(session.account_id), json!(session.session_token_hash), json!(now)],
        )?.is_empty() {
            return Err(forbidden_account());
        }
        // Preserve native 90-day sliding lifetime with the same half-life write throttle.
        if session.expires_at_ms.saturating_sub(now) < SESSION_MS / 2 {
            session.expires_at_ms = now.saturating_add(SESSION_MS);
            db.execute(
                "UPDATE memory_engine_browser_sessions SET expires_at_ms = ? WHERE session_id_hash = ?
                 AND account_id = ? AND session_token_hash = ? AND revoked_at_ms IS NULL AND expires_at_ms > ?",
                &[json!(session.expires_at_ms), json!(id_hash), json!(session.account_id), json!(session.session_token_hash), json!(now)],
            )?;
            db.execute(
                "UPDATE memory_engine_api_sessions SET expires_at_ms = ? WHERE account_id = ?
                 AND session_token_hash = ? AND revoked_at_ms IS NULL AND expires_at_ms > ?",
                &[json!(session.expires_at_ms), json!(session.account_id), json!(session.session_token_hash), json!(now)],
            )?;
        }
        let csrf = csrf.map_or_else(|| csrf_for(&session.session_token_hash), str::to_owned);
        Ok(app_account(session_id, session.account_id, session.session_token_hash, csrf, session.expires_at_ms, now))
    })
}

fn app_account(
    id: String,
    account: String,
    token_hash: String,
    csrf: String,
    expires: i64,
    now: i64,
) -> AppAccount {
    AppAccount::from_session(
        id,
        account,
        token_hash,
        csrf,
        expires,
        u64::try_from(expires.saturating_sub(now).max(0) / 1000).unwrap_or(0),
    )
}

pub fn refresh_browser_cookie(
    req: &Request,
    account: &AppAccount,
    response: &mut Response,
) -> AppResult<()> {
    let secure = secure_request(req);
    let name = if secure { SECURE_COOKIE } else { LOCAL_COOKIE };
    let id = account.browser_session_id();
    if id.is_empty()
        || !id
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || byte == b'_' || byte == b'-')
    {
        return Err(Failure::internal("Browser session identifier is invalid."));
    }
    // Recompute against the clock after rendering/mail awaits; never outlive the row.
    let max_age = u64::try_from(account.expires_at_ms().saturating_sub(now_ms()).max(0) / 1000)
        .unwrap_or(0)
        .min(account.cookie_max_age_seconds());
    response.headers_mut().append(
        "set-cookie",
        &format!(
            "{name}={id}; HttpOnly;{} SameSite={}; Path=/; Max-Age={max_age}",
            if secure { " Secure;" } else { "" },
            if secure { "None" } else { "Lax" },
        ),
    )?;
    Ok(())
}

fn clear_cookies(response: &mut Response) -> AppResult<()> {
    for (name, secure) in [(SECURE_COOKIE, true), (LOCAL_COOKIE, false)] {
        response.headers_mut().append(
            "set-cookie",
            &format!(
                "{name}=; HttpOnly;{} SameSite={}; Path=/; Max-Age=0",
                if secure { " Secure;" } else { "" },
                if secure { "None" } else { "Lax" },
            ),
        )?;
    }
    Ok(())
}

fn client_key(req: &Request) -> String {
    req.headers()
        .get("x-scry-client-ip")
        .ok()
        .flatten()
        .and_then(|value| value.trim().parse::<IpAddr>().ok())
        .map_or_else(|| "unknown".to_owned(), |address| address.to_string())
}

fn rate_limit(db: &Database, keys: &[String], maximum: i64, message: &str) -> AppResult<()> {
    let now = now_ms();
    db.transaction(|| {
        let mut writes = Vec::with_capacity(keys.len());
        for key in keys {
            let previous = db.query::<RateLimit>(
                "SELECT window_start_ms, attempt_count FROM memory_engine_rate_limits WHERE rate_limit_key = ?",
                &[json!(key)],
            )?.pop().filter(|row| row.window_start_ms > now.saturating_sub(RATE_WINDOW_MS));
            let (start, count) = previous.map_or((now, 0), |row| (row.window_start_ms, row.attempt_count));
            if count >= maximum { return Err(Failure::new(429, message)); }
            writes.push((key, start, count + 1));
        }
        for (key, start, count) in writes {
            db.execute(
                "INSERT INTO memory_engine_rate_limits(rate_limit_key, window_start_ms, attempt_count)
                 VALUES (?, ?, ?) ON CONFLICT(rate_limit_key) DO UPDATE SET
                 window_start_ms = excluded.window_start_ms, attempt_count = excluded.attempt_count",
                &[json!(key), json!(start), json!(count)],
            )?;
        }
        Ok(())
    })
}

async fn request_access(
    req: &Request,
    db: &Database,
    env: &Env,
    email: &str,
) -> AppResult<Option<String>> {
    let email = account_email(email);
    let mut keys = vec![format!("app-account-ip:{}", client_key(req))];
    if let Ok(email) = &email {
        keys.push(format!("app-account-email:{email}"));
    }
    rate_limit(db, &keys, 5, "Too many sign-in attempts. Try again later.")?;
    let email = email?;
    let debug = mail::debug_links_enabled(env)?;
    if !admitted(db, env, &email)? {
        db.transaction(|| {
            let now = now_ms();
            db.execute(
                "INSERT INTO memory_engine_waitlist_entries(email_normalized, source, created_at_ms, updated_at_ms)
                 VALUES (?, 'first-run', ?, ?) ON CONFLICT(email_normalized) DO UPDATE SET updated_at_ms = excluded.updated_at_ms",
                &[json!(email), json!(now), json!(now)],
            )?;
            audit_waitlist(db, &email, "joined", now)
        })?;
        return Ok(None);
    }
    let token = random_id("magic_")?;
    let hash = secret_hash(&token);
    db.execute(
        "INSERT INTO memory_engine_auth_challenges(challenge_hash, email_normalized, expires_at_ms) VALUES (?, ?, ?)",
        &[json!(hash), json!(email), json!(now_ms().saturating_add(MAGIC_TTL_MS))],
    )?;
    let link = format!("/app/login/verify?token={token}");
    // A failed/uncertain send is an error, never a production debug-link escape hatch.
    mail::magic_link(db, env, &email, &link, &format!("magic-link:{hash}")).await?;
    Ok(debug.then_some(link))
}

fn mint_api(db: &Database, account_id: &str, token: &str, now: i64) -> AppResult<()> {
    db.execute("INSERT INTO memory_engine_accounts(account_id, created_at_ms) VALUES (?, ?) ON CONFLICT(account_id) DO NOTHING",
        &[json!(account_id), json!(now)])?;
    db.execute(
        "INSERT INTO memory_engine_api_sessions(session_token_hash, account_id, created_at_ms, expires_at_ms)
         VALUES (?, ?, ?, ?)",
        &[json!(secret_hash(token)), json!(account_id), json!(now), json!(now.saturating_add(SESSION_MS))],
    )
}

fn mint_browser(db: &Database, account_id: &str, token: &str, now: i64) -> AppResult<AppAccount> {
    let id = random_id("browser_")?;
    let token_hash = secret_hash(token);
    let csrf = csrf_for(&token_hash);
    let expires = now.saturating_add(SESSION_MS);
    db.execute(
        "INSERT INTO memory_engine_browser_sessions(session_id_hash, account_id, session_token_hash,
         csrf_token_hash, created_at_ms, expires_at_ms) VALUES (?, ?, ?, ?, ?, ?)",
        &[json!(secret_hash(&id)), json!(account_id), json!(token_hash), json!(secret_hash(&csrf)), json!(now), json!(expires)],
    )?;
    Ok(app_account(
        id,
        account_id.to_owned(),
        token_hash,
        csrf,
        expires,
        now,
    ))
}

fn verify_magic(req: &Request, db: &Database, env: &Env, token: &str) -> AppResult<AppAccount> {
    let hash = secret_hash(token.trim());
    rate_limit(
        db,
        &[
            format!("magic-link-verify-token:{hash}"),
            format!("magic-link-verify-ip:{}", client_key(req)),
        ],
        10,
        "Too many verification attempts. Try again later.",
    )?;
    // Return a nested Result so denied admission consumes the challenge, but
    // a storage failure rolls back consumption and all session/account writes.
    db.transaction(|| {
        let now = now_ms();
        let challenge = db.query::<Challenge>(
            "UPDATE memory_engine_auth_challenges SET consumed_at_ms = ? WHERE challenge_hash = ?
             AND consumed_at_ms IS NULL AND expires_at_ms > ? RETURNING email_normalized",
            &[json!(now), json!(hash), json!(now)],
        )?.pop();
        let Some(challenge) = challenge else {
            return Ok(Err(Failure::forbidden("Magic link is invalid or expired.")));
        };
        if !admitted(db, env, &challenge.email_normalized)? {
            return Ok(Err(Failure::forbidden("This invite is no longer active.")));
        }
        let id = account_id_for(&challenge.email_normalized);
        let token = random_id("sess_")?;
        mint_api(db, &id, &token, now)?;
        Ok(Ok(mint_browser(db, &id, &token, now)?))
    })?
}

fn issue_service(db: &Database, env: &Env, email: &str) -> AppResult<AccountCreated> {
    let email = account_email(email)?;
    // Operator issuance preserves the native static-allowlist gate, not merely a waitlist row.
    if !configured_invite(env, &email) {
        return Err(Failure::forbidden("This email is not allowed to register."));
    }
    let account_id = account_id_for(&email);
    let session_token = random_id("sess_")?;
    db.transaction(|| mint_api(db, &account_id, &session_token, now_ms()))?;
    Ok(AccountCreated {
        account_id,
        session_token,
    })
}

fn audit_waitlist(db: &Database, email: &str, event: &str, now: i64) -> AppResult<()> {
    db.execute("INSERT INTO memory_engine_waitlist_audit_log(email_normalized, event, occurred_at_ms) VALUES (?, ?, ?)",
        &[json!(email), json!(event), json!(now)])
}

fn waitlist(db: &Database) -> AppResult<Vec<WaitlistEntry>> {
    Ok(db
        .query::<WaitlistRow>(
            "SELECT * FROM memory_engine_waitlist_entries ORDER BY email_normalized",
            &[],
        )?
        .into_iter()
        .map(Into::into)
        .collect())
}

fn invite_waitlist(db: &Database, email: &str) -> AppResult<WaitlistEntry> {
    let email = account_email(email)?;
    db.transaction(|| {
        let mut entry = db.query::<WaitlistRow>("SELECT * FROM memory_engine_waitlist_entries WHERE email_normalized = ?", &[json!(email)])?
            .pop().ok_or_else(|| Failure::not_found("Waitlist entry not found."))?;
        if entry.invited_at_ms.is_none() {
            let now = now_ms();
            db.execute("UPDATE memory_engine_waitlist_entries SET invited_at_ms = ? WHERE email_normalized = ?", &[json!(now), json!(email)])?;
            audit_waitlist(db, &email, "invited", now)?;
            entry.invited_at_ms = Some(now);
        }
        Ok(entry.into())
    })
}

fn delete_waitlist(db: &Database, email: &str) -> AppResult<()> {
    let email = account_email(email)?;
    db.transaction(|| {
        if db.query::<Challenge>("DELETE FROM memory_engine_waitlist_entries WHERE email_normalized = ? RETURNING email_normalized", &[json!(email)])?.is_empty() {
            return Err(Failure::not_found("Waitlist entry not found."));
        }
        let now = now_ms();
        db.execute("UPDATE memory_engine_auth_challenges SET consumed_at_ms = ? WHERE email_normalized = ? AND consumed_at_ms IS NULL", &[json!(now), json!(email)])?;
        audit_waitlist(db, &email, "deleted", now)
    })
}

fn csv_field(value: &str) -> String {
    let mut value = value.to_owned();
    if value.starts_with(['=', '+', '-', '@']) {
        value.insert(0, '\'');
    }
    if value.contains([',', '"', '\n', '\r']) {
        format!("\"{}\"", value.replace('"', "\"\""))
    } else {
        value
    }
}

#[derive(Deserialize)]
struct Preference {
    email_normalized: String,
    enabled: i64,
    last_sent_at_ms: Option<i64>,
    unsubscribe_nonce: String,
    claim_expires_at_ms: Option<i64>,
    pending_delivery_key: Option<String>,
    pending_due_count: Option<usize>,
    pending_unsubscribe_expires_at_ms: Option<i64>,
    next_retry_at_ms: Option<i64>,
}

fn preference(db: &Database, id: &str) -> AppResult<Option<Preference>> {
    Ok(db
        .query::<Preference>(
            "SELECT email_normalized, enabled, last_sent_at_ms, unsubscribe_nonce,
             claim_expires_at_ms, pending_delivery_key, pending_due_count,
             pending_unsubscribe_expires_at_ms, next_retry_at_ms
             FROM memory_engine_return_notification_preferences WHERE account_id = ?",
            &[json!(id)],
        )?
        .pop())
}

fn set_preference(
    db: &Database,
    env: &Env,
    account: &AppAccount,
    email: Option<&str>,
    enabled: bool,
) -> AppResult<bool> {
    db.transaction(|| {
        let old = preference(db, account.account_id())?;
        let email = if enabled {
            let email = account_email(email.ok_or_else(|| Failure::bad_request("Reminder email must contain one @ and a domain."))?)?;
            if !admitted(db, env, &email)? || account_id_for(&email) != account.account_id() {
                return Err(Failure::forbidden("That reminder email must belong to the authenticated account."));
            }
            email
        } else if let Some(old) = &old { old.email_normalized.clone() } else { return Ok(false); };
        // Repeated opt-in is idempotent: no repeated confirmations or daily-limit reset.
        let unchanged = old.as_ref().is_some_and(|old| (old.enabled != 0) == enabled && old.email_normalized == email);
        if unchanged { return Ok(false); }
        let nonce = random_id("unsubscribe_nonce_")?;
        let now = now_ms();
        db.execute(
            "INSERT INTO memory_engine_return_notification_preferences
             (account_id, email_normalized, enabled, last_sent_at_ms, unsubscribe_nonce, updated_at_ms)
             VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(account_id) DO UPDATE SET
             email_normalized = excluded.email_normalized, enabled = excluded.enabled,
             last_sent_at_ms = excluded.last_sent_at_ms, unsubscribe_nonce = excluded.unsubscribe_nonce,
             updated_at_ms = excluded.updated_at_ms, claim_id = NULL, claim_expires_at_ms = NULL,
             pending_delivery_key = NULL, pending_due_count = NULL, pending_unsubscribe_expires_at_ms = NULL,
             retry_attempts = 0, next_retry_at_ms = NULL",
            &[json!(account.account_id()), json!(email), json!(i64::from(enabled)),
              json!(old.as_ref().and_then(|old| old.last_sent_at_ms)), json!(nonce), json!(now)],
        )?;
        schedule_check(db, account.account_id(), now)?;
        Ok(enabled)
    })
}

fn unsubscribe_secret(env: &Env) -> AppResult<String> {
    mail::required_variable(env, "MEMORY_ENGINE_RETURN_UNSUBSCRIBE_SECRET")
}

fn signed_unsubscribe(
    env: &Env,
    account_id: &str,
    email: &str,
    nonce: &str,
    expires: i64,
) -> AppResult<String> {
    let secret = unsubscribe_secret(env)?;
    let payload = format!("v2\n{account_id}\n{email}\n{nonce}\n{expires}");
    let mut mac = <Hmac<Sha256> as KeyInit>::new_from_slice(secret.as_bytes())
        .map_err(|_| Failure::internal("Unsubscribe signing key is invalid."))?;
    mac.update(payload.as_bytes());
    Ok(format!(
        "{}.{}",
        encode_hex(payload.as_bytes()),
        encode_hex(&mac.finalize().into_bytes())
    ))
}

fn invalid_unsubscribe() -> Failure {
    Failure::forbidden("That unsubscribe link is invalid or expired.")
}

fn verify_unsubscribe(env: &Env, token: &str) -> AppResult<(String, String, String)> {
    if token.len() > 4096 {
        return Err(invalid_unsubscribe());
    }
    let (payload, signature) = token
        .trim()
        .split_once('.')
        .ok_or_else(invalid_unsubscribe)?;
    let payload = decode_hex(payload).ok_or_else(invalid_unsubscribe)?;
    let signature = decode_hex(signature).ok_or_else(invalid_unsubscribe)?;
    let secret = unsubscribe_secret(env)?;
    let mut mac = <Hmac<Sha256> as KeyInit>::new_from_slice(secret.as_bytes())
        .map_err(|_| Failure::internal("Unsubscribe signing key is invalid."))?;
    mac.update(&payload);
    mac.verify_slice(&signature)
        .map_err(|_| invalid_unsubscribe())?;
    let payload = String::from_utf8(payload).map_err(|_| invalid_unsubscribe())?;
    let mut fields = payload.split('\n');
    if fields.next() != Some("v2") {
        return Err(invalid_unsubscribe());
    }
    let account = fields
        .next()
        .filter(|value| !value.is_empty())
        .ok_or_else(invalid_unsubscribe)?;
    let email = fields
        .next()
        .filter(|value| !value.is_empty())
        .ok_or_else(invalid_unsubscribe)?;
    let nonce = fields
        .next()
        .filter(|value| !value.is_empty())
        .ok_or_else(invalid_unsubscribe)?;
    let expires = fields
        .next()
        .and_then(|value| value.parse::<i64>().ok())
        .ok_or_else(invalid_unsubscribe)?;
    if fields.next().is_some() || expires <= now_ms() {
        return Err(invalid_unsubscribe());
    }
    Ok((account.to_owned(), email.to_owned(), nonce.to_owned()))
}

fn validate_unsubscribe(db: &Database, env: &Env, token: &str) -> AppResult<()> {
    let (id, email, nonce) = verify_unsubscribe(env, token)?;
    let row = preference(db, &id)?.ok_or_else(invalid_unsubscribe)?;
    if row.email_normalized != email || !same_hash(&row.unsubscribe_nonce, &nonce) {
        return Err(Failure::forbidden(
            "That unsubscribe link is not for this reminder account.",
        ));
    }
    Ok(())
}

fn disable_unsubscribe(db: &Database, env: &Env, token: &str) -> AppResult<()> {
    let (id, email, nonce) = verify_unsubscribe(env, token)?;
    let next_nonce = random_id("unsubscribe_nonce_")?;
    if db.query::<AccountId>(
        "UPDATE memory_engine_return_notification_preferences SET enabled = 0, unsubscribe_nonce = ?,
         claim_id = NULL, claim_expires_at_ms = NULL, pending_delivery_key = NULL, pending_due_count = NULL,
         pending_unsubscribe_expires_at_ms = NULL, retry_attempts = 0, next_retry_at_ms = NULL, updated_at_ms = ?
         WHERE account_id = ? AND email_normalized = ? AND unsubscribe_nonce = ? AND enabled = 1 RETURNING account_id",
        &[json!(next_nonce), json!(now_ms()), json!(id), json!(email), json!(nonce)],
    )?.is_empty() { return Err(invalid_unsubscribe()); }
    Ok(())
}

fn schedule_check(db: &Database, id: &str, at: i64) -> AppResult<()> {
    db.execute("INSERT INTO memory_engine_return_notification_schedule(account_id, next_check_at_ms) VALUES (?, ?)
        ON CONFLICT(account_id) DO UPDATE SET next_check_at_ms = excluded.next_check_at_ms", &[json!(id), json!(at)])
}

struct ReminderClaim {
    account_id: String,
    email: String,
    claim_id: String,
    delivery_key: String,
    due_count: usize,
    nonce: String,
    unsubscribe_expires: i64,
    new_delivery: bool,
}

fn claim_reminder(
    db: &Database,
    id: &str,
    due_count: usize,
    force: bool,
) -> AppResult<Option<ReminderClaim>> {
    db.transaction(|| {
        let Some(row) = preference(db, id)? else { return Ok(None); };
        let now = now_ms();
        if row.enabled == 0 || row.claim_expires_at_ms.is_some_and(|expires| expires > now) { return Ok(None); }
        let new_delivery = row.pending_delivery_key.is_none();
        if !new_delivery && row.next_retry_at_ms.is_some_and(|retry| retry > now) { return Ok(None); }
        if new_delivery && ((!force && due_count == 0 && row.last_sent_at_ms.is_some())
            || row.last_sent_at_ms.is_some_and(|sent| sent > now.saturating_sub(REMINDER_INTERVAL_MS)))
        { return Ok(None); }
        if new_delivery && !db.query::<serde::de::IgnoredAny>(
            "SELECT delivery_key FROM memory_engine_mail_receipts WHERE recipient_hash = ? AND kind = 'due_count'
             AND (state IN ('sending', 'unknown') OR (state = 'accepted' AND accepted_at_ms > ?)
                  OR (state = 'local_outbox' AND started_at_ms > ?)) LIMIT 1",
            &[json!(secret_hash(&row.email_normalized)), json!(now.saturating_sub(REMINDER_INTERVAL_MS)),
              json!(now.saturating_sub(REMINDER_INTERVAL_MS))],
        )?.is_empty() {
            // Disable/re-enable must not evade an outstanding send or the daily
            // limit by discarding a preference's current delivery key.
            return Ok(None);
        }
        let claim_id = random_id("return_claim_")?;
        let delivery_key = match row.pending_delivery_key {
            Some(key) => key,
            None => random_id(&format!("return-notification:{id}:"))?,
        };
        let nonce = if row.unsubscribe_nonce.is_empty() { random_id("unsubscribe_nonce_")? } else { row.unsubscribe_nonce };
        let due_count = row.pending_due_count.unwrap_or(due_count);
        let unsubscribe_expires = row.pending_unsubscribe_expires_at_ms.unwrap_or(now.saturating_add(UNSUBSCRIBE_TTL_MS));
        db.execute(
            "UPDATE memory_engine_return_notification_preferences SET claim_id = ?, claim_expires_at_ms = ?,
             pending_delivery_key = ?, pending_due_count = ?, pending_unsubscribe_expires_at_ms = ?,
             unsubscribe_nonce = ?, updated_at_ms = ? WHERE account_id = ? AND enabled = 1",
            &[json!(claim_id), json!(now.saturating_add(CLAIM_TTL_MS)), json!(delivery_key), json!(due_count),
              json!(unsubscribe_expires), json!(nonce), json!(now), json!(id)],
        )?;
        Ok(Some(ReminderClaim { account_id: id.to_owned(), email: row.email_normalized, claim_id,
            delivery_key, due_count, nonce, unsubscribe_expires, new_delivery }))
    })
}

fn release_reminder(db: &Database, claim: &ReminderClaim) -> AppResult<()> {
    db.transaction(|| {
        let now = now_ms();
        // A known pre-send failure retains the same confirmation intent (even
        // at zero due reviews). Imported pending keys never get this proof.
        if claim.new_delivery {
            db.execute(
                "INSERT INTO memory_engine_mail_receipts
                 (delivery_key, kind, recipient_hash, payload_hash, state, error_code,
                  attempt_count, started_at_ms, updated_at_ms)
                 VALUES (?, 'due_count', ?, '', 'prepared', 'NOT_ATTEMPTED', 0, ?, ?)
                 ON CONFLICT(delivery_key) DO NOTHING",
                &[json!(claim.delivery_key), json!(secret_hash(&claim.email)), json!(now), json!(now)],
            )?;
        }
        db.execute("UPDATE memory_engine_return_notification_preferences SET claim_id = NULL, claim_expires_at_ms = NULL,
            retry_attempts = MIN(retry_attempts + 1, 2147483647),
            next_retry_at_ms = ? + MIN(21600000, 60000 * (1 << MIN(retry_attempts, 9)))
            WHERE account_id = ? AND claim_id = ?", &[json!(now), json!(claim.account_id), json!(claim.claim_id)])?;
        schedule_check(db, &claim.account_id, now.saturating_add(REMINDER_CHECK_MS))
    })
}

async fn send_reminder(
    db: &Database,
    env: &Env,
    id: &str,
    count: usize,
    force: bool,
) -> AppResult<Option<mail::Acceptance>> {
    // Validate the stable signing secret before authoring any delivery intent.
    unsubscribe_secret(env)?;
    let Some(claim) = claim_reminder(db, id, count, force)? else {
        return Ok(None);
    };
    let token = signed_unsubscribe(
        env,
        id,
        &claim.email,
        &claim.nonce,
        claim.unsubscribe_expires,
    )?;
    let result = mail::due_count(
        db,
        env,
        &claim.email,
        claim.due_count,
        &format!("/app/return-notifications?token={token}"),
        &claim.delivery_key,
        claim.new_delivery,
    )
    .await;
    let acceptance = match result {
        Ok(acceptance) => acceptance,
        Err(error) => {
            release_reminder(db, &claim)?;
            return Err(error);
        }
    };
    let completed = db.query::<AccountId>(
        "UPDATE memory_engine_return_notification_preferences SET last_sent_at_ms = ?, claim_id = NULL,
         claim_expires_at_ms = NULL, pending_delivery_key = NULL, pending_due_count = NULL,
         pending_unsubscribe_expires_at_ms = NULL, retry_attempts = 0, next_retry_at_ms = NULL, updated_at_ms = ?
         WHERE account_id = ? AND claim_id = ? AND enabled = 1 AND unsubscribe_nonce = ? RETURNING account_id",
        &[json!(now_ms()), json!(now_ms()), json!(id), json!(claim.claim_id), json!(claim.nonce)],
    )?;
    Ok((!completed.is_empty()).then_some(acceptance))
}

const REMINDER_READY: &str = "SELECT p.account_id,
    MAX(COALESCE(s.next_check_at_ms, 0), COALESCE(p.claim_expires_at_ms, 0),
        CASE WHEN p.pending_delivery_key IS NOT NULL THEN COALESCE(p.next_retry_at_ms, 0)
        ELSE COALESCE(p.last_sent_at_ms + 86400000, 0) END) AS ready_at
    FROM memory_engine_return_notification_preferences p
    LEFT JOIN memory_engine_return_notification_schedule s ON s.account_id = p.account_id
    LEFT JOIN memory_engine_mail_receipts m ON m.delivery_key = p.pending_delivery_key
    WHERE p.enabled = 1 AND (m.state IS NULL OR m.state NOT IN ('sending', 'unknown'))";

pub fn next_reminder_at(db: &Database) -> AppResult<Option<i64>> {
    #[derive(Deserialize)]
    struct Next {
        next_at: Option<i64>,
    }
    Ok(db
        .query::<Next>(
            &format!("SELECT MIN(ready_at) AS next_at FROM ({REMINDER_READY})"),
            &[],
        )?
        .pop()
        .and_then(|row| row.next_at)
        .map(|at| at.max(now_ms())))
}

async fn reminder_report(db: &Database, env: &Env) -> AppResult<ScheduledReturnNotificationReport> {
    let batch = mail::variable(env, "MEMORY_ENGINE_RETURN_NOTIFICATION_BATCH_SIZE")
        .and_then(|value| value.parse::<usize>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(100)
        .min(1000);
    let started = now_ms();
    let mut rows = db.query::<AccountId>(
        &format!("SELECT account_id FROM ({REMINDER_READY}) WHERE ready_at <= ? ORDER BY ready_at, account_id LIMIT ?"),
        &[json!(started), json!(batch + 1)],
    )?;
    let mut report = ScheduledReturnNotificationReport {
        started_at_ms: started,
        examined: rows.len().min(batch),
        truncated: rows.len() > batch,
        ..Default::default()
    };
    rows.truncate(batch);
    for row in rows {
        // Fairness/no-due retry time is independent from last provider acceptance.
        schedule_check(
            db,
            &row.account_id,
            now_ms().saturating_add(REMINDER_CHECK_MS),
        )?;
        let session = BetaStudySession::for_review(
            SqlStudyStore::new(db.clone(), row.account_id.clone()),
            now_ms,
        );
        let Ok(view) = session.view() else {
            report.failed += 1;
            continue;
        };
        if view.due_count > 0 {
            report.due += 1;
        }
        match send_reminder(db, env, &row.account_id, view.due_count, false).await {
            Ok(Some(mail::Acceptance::Provider { .. })) => report.sent += 1,
            Ok(Some(mail::Acceptance::LocalOutbox) | None) => report.skipped += 1,
            Err(_) => report.failed += 1,
        }
    }
    report.finished_at_ms = now_ms();
    Ok(report)
}

pub async fn run_reminders(db: &Database, env: &Env) -> AppResult<()> {
    let report = reminder_report(db, env).await?;
    worker::console_log!(
        "reminder_scheduler examined={} due={} accepted={} skipped={} failed={} truncated={}",
        report.examined,
        report.due,
        report.sent,
        report.skipped,
        report.failed,
        report.truncated
    );
    if report.failed > 0 {
        Err(Failure::new(
            503,
            "Some scheduled reminders could not be accepted.",
        ))
    } else {
        Ok(())
    }
}

pub(crate) async fn json_body<T: DeserializeOwned>(req: &mut Request) -> AppResult<T> {
    let content_type = req.headers().get("content-type")?.unwrap_or_default();
    let mime = content_type.split(';').next().unwrap_or_default().trim();
    let is_json = mime.split_once('/').is_some_and(|(kind, subtype)| {
        kind.eq_ignore_ascii_case("application")
            && (subtype.eq_ignore_ascii_case("json")
                || subtype
                    .rsplit_once('+')
                    .is_some_and(|(_, suffix)| suffix.eq_ignore_ascii_case("json")))
    });
    if !is_json {
        return Err(Failure::new(415, "Request body must be JSON."));
    }
    serde_json::from_slice(&crate::web::body_bytes(req, BODY_LIMIT).await?).map_err(|error| {
        Failure::new(
            if error.is_data() { 422 } else { 400 },
            "Request body must be JSON with the required fields.",
        )
    })
}

async fn form_body<T: DeserializeOwned>(req: &mut Request) -> AppResult<T> {
    if req
        .headers()
        .get("content-type")?
        .unwrap_or_default()
        .split(';')
        .next()
        .map(str::trim)
        != Some("application/x-www-form-urlencoded")
    {
        return Err(Failure::new(
            415,
            "Request body must be an encoded browser form.",
        ));
    }
    serde_urlencoded::from_bytes(&crate::web::body_bytes(req, BODY_LIMIT).await?)
        .map_err(|_| Failure::new(422, "Browser form fields are missing or invalid."))
}

fn token_query(req: &Request) -> AppResult<Option<String>> {
    let url = req.url()?;
    let mut tokens = url.query_pairs().filter(|(key, _)| key == "token");
    let token = tokens.next().map(|(_, value)| value.into_owned());
    if tokens.next().is_some() {
        return Err(Failure::bad_request("Only one token may be supplied."));
    }
    Ok(token)
}

#[derive(Deserialize)]
struct EmailForm {
    email: String,
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct AccountAction {
    csrf_token: Option<String>,
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ReminderForm {
    csrf_token: Option<String>,
    unsubscribe_token: Option<String>,
    reminder_email: Option<String>,
    enabled: Option<String>,
}

fn html(body: String, status: u16) -> AppResult<Response> {
    Ok(Response::from_html(body)?.with_status(status))
}

fn account_response(
    req: &Request,
    db: &Database,
    account: &AppAccount,
    notice: Option<&str>,
) -> AppResult<Response> {
    let mut response = html(crate::web::account_page(db, account, notice)?, 200)?;
    refresh_browser_cookie(req, account, &mut response)?;
    Ok(response)
}

fn browser_failure(error: &Failure) -> AppResult<Response> {
    let title = match error.status {
        401 | 403 => "Sign in again to continue",
        429 => "Please try again later",
        _ => "We couldn’t complete that request",
    };
    html(render_auth_recovery(title, &error.message), error.status)
}

fn entry_failure(error: &Failure) -> AppResult<Response> {
    let (status, title, message) = match error.status {
        400 => (
            400,
            "Check the email address",
            "That email address is not valid. Check it and try again.",
        ),
        429 => (
            429,
            "Please try again later",
            "We’re taking a short break from requests. Wait a little while, then try again.",
        ),
        _ => (
            503,
            "We couldn’t complete that request",
            "We couldn’t finish that request. Please try again shortly.",
        ),
    };
    html(render_entry_recovery(title, message), status)
}

async fn browser_route(
    req: &mut Request,
    db: &Database,
    env: &Env,
    path: &str,
) -> AppResult<Response> {
    if !secure_request(req) && !local_environment(env) {
        return Err(Failure::forbidden(
            "A secure browser connection is required.",
        ));
    }
    match path {
        "/app/account" => {
            let form: EmailForm = form_body(req).await?;
            match request_access(req, db, env, &form.email).await {
                Ok(debug_link) => {
                    let mut response = html(render_entry_requested(debug_link.as_deref()), 200)?;
                    if mail::local_outbox(env)? {
                        response
                            .headers_mut()
                            .set("x-scry-mail-mode", "local-outbox")?;
                    }
                    Ok(response)
                }
                Err(error) => entry_failure(&error),
            }
        }
        "/app/login/verify" => {
            let token = token_query(req)?
                .ok_or_else(|| Failure::bad_request("A sign-in token is required."))?;
            match verify_magic(req, db, env, &token) {
                Ok(account) => {
                    let mut response = Response::empty()?.with_status(303);
                    response.headers_mut().set("location", "/")?;
                    refresh_browser_cookie(req, &account, &mut response)?;
                    Ok(response)
                }
                Err(error) if error.status == 403 => html(render_auth_recovery("Sign-in link expired",
                    "That link is no longer valid. Request a fresh link and return to your study space."), 403),
                Err(error) => browser_failure(&error),
            }
        }
        "/app/logout" | "/app/logout-all" => {
            let form: AccountAction = form_body(req).await?;
            let account = browser_account(
                req,
                db,
                env,
                Some(form.csrf_token.as_deref().unwrap_or_default()),
            )?;
            if path == "/app/logout-all" {
                db.execute("UPDATE memory_engine_browser_sessions SET revoked_at_ms = ? WHERE account_id = ? AND revoked_at_ms IS NULL",
                    &[json!(now_ms()), json!(account.account_id())])?;
            } else {
                db.execute("UPDATE memory_engine_browser_sessions SET revoked_at_ms = ? WHERE account_id = ? AND session_id_hash = ? AND revoked_at_ms IS NULL",
                    &[json!(now_ms()), json!(account.account_id()), json!(secret_hash(account.browser_session_id()))])?;
            }
            let mut response = html(render_app_shell(None, &[], None, &[], None), 200)?;
            clear_cookies(&mut response)?;
            Ok(response)
        }
        "/app/return-notifications" if matches!(req.method(), Method::Get | Method::Head) => {
            let token = token_query(req)?;
            if let Some(token) = token {
                return match validate_unsubscribe(db, env, &token) {
                    Ok(()) => html(render_return_notification_confirmation(&token), 200),
                    Err(error) => html(
                        render_return_notification_recovery(
                            "That reminder link needs a refresh",
                            &error.message,
                        ),
                        error.status,
                    ),
                };
            }
            let account = browser_account(req, db, env, None)?;
            account_response(req, db, &account, None)
        }
        "/app/return-notifications" => update_reminders(req, db, env).await,
        _ => Err(Failure::not_found("Route not found.")),
    }
}

async fn update_reminders(req: &mut Request, db: &Database, env: &Env) -> AppResult<Response> {
    let form: ReminderForm = form_body(req).await?;
    if let Some(token) = form
        .unsubscribe_token
        .as_deref()
        .filter(|token| !token.trim().is_empty())
    {
        return match disable_unsubscribe(db, env, token) {
            Ok(()) => html(render_return_notification_disabled(), 200),
            Err(error) => html(
                render_return_notification_recovery(
                    "That reminder link needs a refresh",
                    &error.message,
                ),
                error.status,
            ),
        };
    }
    let account = browser_account(
        req,
        db,
        env,
        Some(form.csrf_token.as_deref().unwrap_or_default()),
    )?;
    let enabled = form.enabled.as_deref() == Some("on");
    let confirmation =
        match set_preference(db, env, &account, form.reminder_email.as_deref(), enabled) {
            Ok(changed) => changed,
            Err(error) => return account_response(req, db, &account, Some(&error.message)),
        };
    let mut notice = if enabled {
        "Due-count reminders are on. Reminders stay to at most one per day."
    } else {
        "Due-count reminders are off."
    };
    if confirmation {
        let session = BetaStudySession::for_review(
            SqlStudyStore::new(db.clone(), account.account_id()),
            now_ms,
        );
        let count = session
            .view()
            .map_err(|_| Failure::new(503, "Unable to read the reminder due count."))?
            .due_count;
        match send_reminder(db, env, account.account_id(), count, true).await {
            Ok(Some(mail::Acceptance::Provider { .. })) => notice = "Due-count reminders are on. The email provider accepted your confirmation; mailbox delivery is not yet verified.",
            Ok(Some(mail::Acceptance::LocalOutbox)) => notice = "Due-count reminders are on. The confirmation is in the private local outbox; no email was sent.",
            Ok(None) => (),
            Err(error) => return account_response(req, db, &account, Some(&error.message)),
        }
    }
    account_response(req, db, &account, Some(notice))
}

pub async fn handle(req: &mut Request, db: &Database, env: &Env) -> AppResult<Option<Response>> {
    if let Some(response) = mail::handle(req, db, env).await? {
        return Ok(Some(finish_response(req, response)?));
    }
    let path = req.path();
    let expected = match path.as_str() {
        "/app/account" | "/app/logout" | "/app/logout-all" => Some(Method::Post),
        "/app/login/verify" => Some(Method::Get),
        "/app/return-notifications" => None,
        _ => {
            let response = api_route(req, db, env, &path).await?;
            return response
                .map(|response| finish_response(req, response))
                .transpose();
        }
    };
    let method_ok = expected.as_ref().map_or_else(
        || matches!(req.method(), Method::Get | Method::Head | Method::Post),
        |method| req.method() == *method || *method == Method::Get && req.method() == Method::Head,
    );
    let result = if method_ok {
        browser_route(req, db, env, &path).await
    } else {
        Err(Failure::new(405, "Method not allowed."))
    };
    let response = match result {
        Ok(response) => response,
        Err(error) => browser_failure(&error)?,
    };
    Ok(Some(finish_response(req, response)?))
}

fn finish_response(req: &Request, mut response: Response) -> AppResult<Response> {
    response.headers_mut().set("cache-control", "no-store")?;
    response
        .headers_mut()
        .set("referrer-policy", "no-referrer")?;
    if req.method() == Method::Head {
        return Ok(Response::empty()?
            .with_status(response.status_code())
            .with_headers(response.headers().clone()));
    }
    Ok(response)
}

async fn api_route(
    req: &mut Request,
    db: &Database,
    env: &Env,
    path: &str,
) -> AppResult<Option<Response>> {
    if matches!(path, "/v1/accounts" | "/accounts" | "/v1/service-sessions") {
        if req.method() != Method::Post {
            return Err(Failure::new(405, "Method not allowed."));
        }
        let request: CreateAccountRequest = if path == "/v1/service-sessions" {
            require_admin(req, env)?; // Native machine issuance authenticates before parsing.
            serde_json::from_slice(&crate::web::body_bytes(req, BODY_LIMIT).await?).map_err(
                |_| Failure::bad_request("Request body must be JSON with an email field."),
            )?
        } else {
            let request: CreateAccountRequest = json_body(req).await?;
            account_email(&request.email)?;
            require_admin(req, env)?;
            request
        };
        return Ok(Some(
            Response::from_json(&issue_service(db, env, &request.email)?)?.with_status(201),
        ));
    }
    let segments: Vec<_> = path.split('/').filter(|part| !part.is_empty()).collect();
    if let ["v1", "accounts", id, "service-sessions", scope @ ("current" | "all")] =
        segments.as_slice()
    {
        if req.method() != Method::Delete {
            return Err(Failure::new(405, "Method not allowed."));
        }
        db.transaction(|| {
            api_account(req, db, env, id)?;
            let now = now_ms();
            if *scope == "current" {
                db.execute("UPDATE memory_engine_api_sessions SET revoked_at_ms = ? WHERE account_id = ? AND session_token_hash = ? AND revoked_at_ms IS NULL",
                    &[json!(now), json!(id), json!(secret_hash(&api_token(req)?))])?;
            } else {
                // Machine revoke-all cannot sign out a live browser-backed session.
                db.execute("UPDATE memory_engine_api_sessions SET revoked_at_ms = ? WHERE account_id = ? AND revoked_at_ms IS NULL
                    AND session_token_hash NOT IN (SELECT session_token_hash FROM memory_engine_browser_sessions WHERE account_id = ? AND revoked_at_ms IS NULL)",
                    &[json!(now), json!(id), json!(id)])?;
            }
            Ok(())
        })?;
        return Ok(Some(Response::empty()?.with_status(204)));
    }
    if path == "/internal/scheduler/return-notifications" {
        if req.method() != Method::Post {
            return Err(Failure::new(405, "Method not allowed."));
        }
        let token = req.headers().get("x-scheduler-token")?.unwrap_or_default();
        require_secret(
            env,
            "MEMORY_ENGINE_RETURN_NOTIFICATION_MANUAL_TOKEN",
            &token,
            "Scheduled reminder trigger is not authorized.",
        )?;
        let mut response = Response::from_json(&reminder_report(db, env).await?)?;
        response
            .headers_mut()
            .set("x-scry-mail-evidence", "provider-acceptance-only")?;
        return Ok(Some(response));
    }
    if !matches!(
        path,
        "/internal/waitlist"
            | "/internal/waitlist/export"
            | "/internal/waitlist/invite"
            | "/internal/waitlist/delete"
    ) {
        return Ok(None);
    }
    Ok(Some(waitlist_route(req, db, env, path).await?))
}

async fn waitlist_route(
    req: &mut Request,
    db: &Database,
    env: &Env,
    path: &str,
) -> AppResult<Response> {
    require_admin(req, env)?;
    let read = matches!(path, "/internal/waitlist" | "/internal/waitlist/export");
    let allowed = if read {
        matches!(req.method(), Method::Get | Method::Head)
    } else {
        req.method() == Method::Post
    };
    if !allowed {
        return Err(Failure::new(405, "Method not allowed."));
    }
    let response = match path {
        "/internal/waitlist" => Response::from_json(&waitlist(db)?)?,
        "/internal/waitlist/export" => {
            let mut csv = String::from("email,createdAtMs,updatedAtMs,source,invitedAtMs\n");
            for row in waitlist(db)? {
                let _ = writeln!(
                    csv,
                    "{},{},{},{},{}",
                    csv_field(&row.email),
                    row.created_at_ms,
                    row.updated_at_ms,
                    csv_field(&row.source),
                    row.invited_at_ms
                        .map_or_else(String::new, |value| value.to_string())
                );
            }
            let mut response = Response::ok(csv)?;
            response
                .headers_mut()
                .set("content-type", "text/csv; charset=utf-8")?;
            response
        }
        "/internal/waitlist/invite" => {
            let request: EmailForm = json_body(req).await?;
            Response::from_json(&invite_waitlist(db, &request.email)?)?
        }
        _ => {
            let request: EmailForm = json_body(req).await?;
            delete_waitlist(db, &request.email)?;
            Response::from_json(&json!({ "deleted": true }))?
        }
    };
    Ok(response)
}
