//! Transactional Resend transport. A message id proves provider acceptance only,
//! never mailbox delivery or a human read. Durable receipts, not Resend's
//! 24-hour idempotency window, prevent blind replay of uncertain sends.
//!
//! Provider contract (retrieved 2026-09-06):
//! <https://resend.com/docs/api-reference/emails/send-email>
//! <https://resend.com/docs/api-reference/introduction>
//! <https://resend.com/docs/api-reference/errors>
//! <https://resend.com/docs/dashboard/emails/idempotency-keys>

use std::time::Duration;

use futures_util::{
    future::{select, Either},
    pin_mut, StreamExt,
};
use memory_engine_api_state::normalize_email;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use worker::{
    AbortController, Delay, Env, Fetch, Headers, Method, Request, RequestInit, RequestRedirect,
    Response, Url,
};

use crate::{auth, database::Database, now_ms, AppResult, Failure};

const MAX_REQUEST_BYTES: usize = 64 * 1024;
const MAX_RESPONSE_BYTES: usize = 16 * 1024;
const REQUEST_TIMEOUT: Duration = Duration::from_secs(10);

#[derive(Clone, Debug, Eq, PartialEq)]
pub(crate) enum Acceptance {
    Provider { message_id: String },
    LocalOutbox,
}

// Keep the durable payload encoding stable across transport changes: existing
// receipt hashes and the inspectable local outbox are not Resend wire payloads.
#[derive(Serialize)]
struct Payload<'a> {
    from: &'a Value,
    to: &'a str,
    subject: &'a str,
    text: &'a str,
}

#[derive(Serialize)]
struct ResendPayload<'a> {
    from: &'a str,
    to: [&'a str; 1],
    subject: &'a str,
    text: &'a str,
}

#[derive(Deserialize)]
struct ResendResponse {
    id: String,
}

enum ResendFailure {
    Rejected(&'static str),
    Unknown(&'static str),
}

#[derive(Deserialize, Serialize)]
struct Receipt {
    delivery_key: String,
    kind: String,
    recipient_hash: String,
    payload_hash: String,
    state: String,
    message_id: Option<String>,
    error_code: Option<String>,
    attempt_count: i64,
    started_at_ms: i64,
    updated_at_ms: i64,
    accepted_at_ms: Option<i64>,
    reconciled_at_ms: Option<i64>,
    reconciliation_evidence_sha256: Option<String>,
}

pub(crate) fn variable(env: &Env, key: &str) -> Option<String> {
    env.var(key).ok().map(|value| value.to_string())
}

pub(crate) fn required_variable(env: &Env, key: &str) -> AppResult<String> {
    variable(env, key)
        .filter(|value| !value.trim().is_empty())
        .ok_or_else(|| Failure::new(503, format!("Required configuration {key} is unavailable.")))
}

pub(crate) fn local_outbox(env: &Env) -> AppResult<bool> {
    let local = matches!(
        variable(env, "MEMORY_ENGINE_ENVIRONMENT").as_deref(),
        Some("local" | "development" | "test")
    );
    let mode = variable(env, "MEMORY_ENGINE_MAIL_MODE");
    let outbox = match mode.as_deref().unwrap_or("resend") {
        "resend" => false,
        "local-outbox" if local => true,
        _ => {
            return Err(Failure::new(
                503,
                "Mail mode is not authorized for this environment.",
            ))
        }
    };
    if variable(env, "MEMORY_ENGINE_AUTH_EXPOSE_DEBUG_LINKS").as_deref() == Some("true") && !outbox
    {
        return Err(Failure::new(
            503,
            "Debug sign-in links require explicit local-outbox mode.",
        ));
    }
    Ok(outbox)
}

pub(crate) fn debug_links_enabled(env: &Env) -> AppResult<bool> {
    Ok(local_outbox(env)?
        && variable(env, "MEMORY_ENGINE_AUTH_EXPOSE_DEBUG_LINKS").as_deref() == Some("true"))
}

pub(crate) fn public_url(env: &Env, path: &str) -> AppResult<String> {
    let local = local_outbox(env)?;
    let raw = required_variable(env, "MEMORY_ENGINE_PUBLIC_BASE_URL")?;
    let base = Url::parse(raw.trim())
        .map_err(|_| Failure::new(503, "Public mail URL is not configured correctly."))?;
    let loopback = matches!(
        base.host_str(),
        Some("localhost" | "127.0.0.1" | "[::1]" | "::1")
    );
    if !(base.scheme() == "https" || (local && loopback && base.scheme() == "http"))
        || base.host_str().is_none()
        || !base.username().is_empty()
        || base.password().is_some()
        || base.query().is_some()
        || base.fragment().is_some()
        || base.path() != "/"
        || !path.starts_with('/')
        || path.starts_with("//")
    {
        return Err(Failure::new(
            503,
            "Public mail URL is not configured correctly.",
        ));
    }
    base.join(path)
        .map(|url| url.to_string())
        .map_err(|_| Failure::internal("Unable to construct transactional mail URL."))
}

fn sender(env: &Env) -> AppResult<(String, Value)> {
    let value = required_variable(env, "MEMORY_ENGINE_MAIL_FROM")?;
    let value = value.trim();
    if value.chars().any(char::is_control) || value.len() > 512 {
        return Err(Failure::new(
            503,
            "Transactional mail sender is not configured correctly.",
        ));
    }
    let (name, address) = if let Some((name, suffix)) = value.split_once('<') {
        let address = suffix.strip_suffix('>').ok_or_else(|| {
            Failure::new(
                503,
                "Transactional mail sender is not configured correctly.",
            )
        })?;
        (Some(name.trim()), address.trim())
    } else {
        (None, value)
    };
    let email = transport_email(address).map_err(|_| {
        Failure::new(
            503,
            "Transactional mail sender is not configured correctly.",
        )
    })?;
    Ok(match name {
        Some(name) if !name.is_empty() && !name.contains(['<', '>']) => (
            format!("{name} <{email}>"),
            json!({ "email": email, "name": name }),
        ),
        Some(_) => {
            return Err(Failure::new(
                503,
                "Transactional mail sender is not configured correctly.",
            ))
        }
        None => (email.clone(), Value::String(email)),
    })
}

fn transport_email(email: &str) -> AppResult<String> {
    if email.len() > 320
        || email.chars().any(|c| c.is_control() || c.is_whitespace())
        || email.contains(['<', '>', ',', ';'])
    {
        return Err(Failure::bad_request(
            "Email address cannot be used for transactional mail.",
        ));
    }
    normalize_email(email)
        .ok_or_else(|| Failure::bad_request("Email must contain one @ and a domain."))
}

fn resend_request(env: &Env, payload: String, delivery_key: &str) -> AppResult<Request> {
    let configuration_error = || {
        Failure::new(
            503,
            "Transactional mail request is not configured correctly.",
        )
    };
    if payload.len() > MAX_REQUEST_BYTES {
        return Err(Failure::new(
            503,
            "Transactional mail payload exceeds the size limit.",
        ));
    }
    if delivery_key.is_empty()
        || delivery_key.len() > 256
        || !delivery_key.bytes().all(|byte| byte.is_ascii_graphic())
    {
        return Err(Failure::internal(
            "Transactional mail delivery key is invalid.",
        ));
    }
    let key = env
        .secret("RESEND_API_KEY")
        .map_err(|_| Failure::new(503, "Required configuration RESEND_API_KEY is unavailable."))?
        .to_string();
    if key.is_empty() || key.len() > 512 || !key.bytes().all(|byte| byte.is_ascii_graphic()) {
        return Err(Failure::new(
            503,
            "Transactional mail provider credential is not configured correctly.",
        ));
    }
    let mut endpoint = if let Some(raw) = variable(env, "RESEND_API_URL") {
        let local = matches!(
            variable(env, "MEMORY_ENGINE_ENVIRONMENT").as_deref(),
            Some("local" | "development" | "test")
        );
        let endpoint = Url::parse(&raw).map_err(|_| configuration_error())?;
        let loopback = matches!(
            endpoint.host_str(),
            Some("localhost" | "127.0.0.1" | "[::1]" | "::1")
        );
        if !local
            || !loopback
            || !matches!(endpoint.scheme(), "http" | "https")
            || !endpoint.username().is_empty()
            || endpoint.password().is_some()
            || endpoint.query().is_some()
            || endpoint.fragment().is_some()
            || endpoint.path() != "/"
        {
            return Err(Failure::new(
                503,
                "Transactional mail endpoint override requires an explicit local loopback origin.",
            ));
        }
        endpoint
    } else {
        Url::parse("https://api.resend.com").map_err(|_| configuration_error())?
    };
    endpoint.set_path("/emails");
    let headers = Headers::new();
    headers
        .set("content-type", "application/json")
        .map_err(|_| configuration_error())?;
    headers
        .set("authorization", &format!("Bearer {key}"))
        .map_err(|_| configuration_error())?;
    headers
        .set("idempotency-key", delivery_key)
        .map_err(|_| configuration_error())?;
    // Resend requires an explicit User-Agent for direct HTTP clients.
    headers
        .set("user-agent", "scry-worker")
        .map_err(|_| configuration_error())?;
    Request::new_with_init(
        endpoint.as_str(),
        &RequestInit {
            method: Method::Post,
            headers,
            body: Some(payload.into()),
            // Inspect non-success responses without forwarding credentials.
            redirect: RequestRedirect::Manual,
            ..RequestInit::default()
        },
    )
    .map_err(|_| configuration_error())
}

pub(crate) async fn magic_link(
    db: &Database,
    env: &Env,
    email: &str,
    link: &str,
    delivery_key: &str,
) -> AppResult<Acceptance> {
    let url = public_url(env, link)?;
    let text = format!(
        "Sign in to Scry: {url}\n\nThis single-use link expires in 30 minutes. If you did not request it, ignore this email."
    );
    Delivery::prepare(
        env,
        "magic_link",
        email,
        "Your Scry sign-in link",
        &text,
        delivery_key,
    )?
    .send(db, env, true)
    .await
}

pub(crate) async fn due_count(
    db: &Database,
    env: &Env,
    email: &str,
    count: usize,
    unsubscribe_link: &str,
    delivery_key: &str,
    new_delivery: bool,
) -> AppResult<Acceptance> {
    let home = public_url(env, "/")?;
    let unsubscribe = public_url(env, unsubscribe_link)?;
    let subject = if count == 0 {
        "Your Scry reminders are on".to_owned()
    } else {
        format!(
            "You have {count} Scry review{}",
            if count == 1 { "" } else { "s" }
        )
    };
    let text = format!(
        "Your Scry due-count reminders are on. You have {count} review(s) waiting. Return when it suits you: {home}\n\nReminders stay to at most one per day. To turn them off, visit: {unsubscribe}"
    );
    Delivery::prepare(env, "due_count", email, &subject, &text, delivery_key)?
        .send(db, env, new_delivery)
        .await
}

// A prepared delivery owns its normalized envelope and stable receipt identity
// through reservation and transport; the provider wire encoding stays separate.
struct Delivery<'a> {
    kind: &'a str,
    key: &'a str,
    local: bool,
    from: String,
    recipient: String,
    subject: &'a str,
    text: &'a str,
    payload: String,
    payload_hash: String,
    recipient_hash: String,
}

impl<'a> Delivery<'a> {
    fn prepare(
        env: &Env,
        kind: &'a str,
        email: &str,
        subject: &'a str,
        text: &'a str,
        key: &'a str,
    ) -> AppResult<Self> {
        let local = local_outbox(env)?;
        let recipient = transport_email(email)?;
        let (from, receipt_from) = sender(env)?;
        if from
            .len()
            .saturating_add(recipient.len())
            .saturating_add(subject.len())
            .saturating_add(text.len())
            > MAX_REQUEST_BYTES
        {
            return Err(Failure::new(
                503,
                "Transactional mail payload exceeds the size limit.",
            ));
        }
        let payload = serde_json::to_string(&Payload {
            from: &receipt_from,
            to: &recipient,
            subject,
            text,
        })?;
        if payload.len() > MAX_REQUEST_BYTES {
            return Err(Failure::new(
                503,
                "Transactional mail payload exceeds the size limit.",
            ));
        }
        let payload_hash = auth::secret_hash(&payload);
        let recipient_hash = auth::secret_hash(&recipient);
        Ok(Self {
            kind,
            key,
            local,
            from,
            recipient,
            subject,
            text,
            payload,
            payload_hash,
            recipient_hash,
        })
    }

    fn reserve(&self, db: &Database) -> AppResult<Option<Acceptance>> {
        db.transaction(|| {
            let receipt = db.query::<Receipt>(
                "SELECT * FROM memory_engine_mail_receipts WHERE delivery_key = ?",
                &[json!(self.key)],
            )?.pop();
            if let Some(receipt) = receipt {
                if receipt.state == "sending" || receipt.state == "unknown" {
                    return Err(uncertain_acceptance());
                }
                if (receipt.state != "prepared" && receipt.payload_hash != self.payload_hash)
                    || receipt.recipient_hash != self.recipient_hash || receipt.kind != self.kind
                {
                    return Err(Failure::conflict("Mail delivery key was already used for a different message."));
                }
                match receipt.state.as_str() {
                    "accepted" => return Ok(Some(Acceptance::Provider {
                        message_id: receipt.message_id.filter(|value| !value.is_empty())
                            .ok_or_else(|| Failure::internal("Accepted mail receipt has no provider message id."))?,
                    })),
                    "local_outbox" if self.local => return Ok(Some(Acceptance::LocalOutbox)),
                    "local_outbox" => return Err(Failure::conflict("A local outbox receipt cannot become a production send.")),
                    "prepared" | "rejected" => (),
                    _ => return Err(Failure::internal("Mail receipt has an unsupported state.")),
                }
            }
            let now = now_ms();
            db.execute(
                "INSERT INTO memory_engine_mail_receipts
                 (delivery_key, kind, recipient_hash, payload_hash, state, attempt_count, started_at_ms, updated_at_ms)
                 VALUES (?, ?, ?, ?, ?, 1, ?, ?)
                 ON CONFLICT(delivery_key) DO UPDATE SET state = excluded.state, error_code = NULL,
                 message_id = NULL, accepted_at_ms = NULL, payload_hash = excluded.payload_hash,
                 attempt_count = memory_engine_mail_receipts.attempt_count + 1,
                 started_at_ms = excluded.started_at_ms, updated_at_ms = excluded.updated_at_ms",
                &[json!(self.key), json!(self.kind), json!(self.recipient_hash), json!(self.payload_hash),
                  json!(if self.local { "local_outbox" } else { "sending" }), json!(now), json!(now)],
            )?;
            if self.local {
                db.execute(
                    "INSERT INTO memory_engine_mail_outbox(delivery_key, payload, created_at_ms) VALUES (?, ?, ?)",
                    &[json!(self.key), json!(self.payload), json!(now)],
                )?;
                return Ok(Some(Acceptance::LocalOutbox));
            }
            Ok(None)
        })
    }

    async fn send(self, db: &Database, env: &Env, new_delivery: bool) -> AppResult<Acceptance> {
        #[derive(Deserialize)]
        struct Attempt {
            attempt_count: i64,
        }
        if !new_delivery {
            // A migrated/old pending key without a receipt might already have been
            // accepted by the prior transport. Record uncertainty, not a new send.
            db.execute(
                "INSERT INTO memory_engine_mail_receipts
                 (delivery_key, kind, recipient_hash, payload_hash, state, error_code,
                  attempt_count, started_at_ms, updated_at_ms)
                 VALUES (?, ?, ?, ?, 'unknown', 'MISSING_PRIOR_ACCEPTANCE_RECEIPT', 0, ?, ?)
                 ON CONFLICT(delivery_key) DO NOTHING",
                &[
                    json!(self.key),
                    json!(self.kind),
                    json!(self.recipient_hash),
                    json!(self.payload_hash),
                    json!(now_ms()),
                    json!(now_ms()),
                ],
            )?;
        }
        // Resolve all configuration and construct the request before reserving a send.
        let request = if self.local {
            None
        } else {
            let payload = serde_json::to_string(&ResendPayload {
                from: &self.from,
                to: [&self.recipient],
                subject: self.subject,
                text: self.text,
            })?;
            Some(resend_request(env, payload, self.key)?)
        };
        if let Some(acceptance) = self.reserve(db)? {
            return Ok(acceptance);
        }
        let attempt = db.query::<Attempt>(
            "SELECT attempt_count FROM memory_engine_mail_receipts WHERE delivery_key = ? AND state = 'sending'",
            &[json!(self.key)],
        )?.pop().ok_or_else(uncertain_acceptance)?.attempt_count;
        let request =
            request.ok_or_else(|| Failure::internal("Transactional mail request is missing."))?;
        match resend(request).await {
            Ok(message_id) => {
                let recorded = db.query::<serde::de::IgnoredAny>(
                    "UPDATE memory_engine_mail_receipts SET state = 'accepted', message_id = ?,
                     accepted_at_ms = ?, updated_at_ms = ?, error_code = NULL
                     WHERE delivery_key = ? AND state = 'sending' AND attempt_count = ? RETURNING delivery_key",
                    &[json!(message_id), json!(now_ms()), json!(now_ms()), json!(self.key), json!(attempt)],
                )?;
                if recorded.is_empty() {
                    return Err(uncertain_acceptance());
                }
                Ok(Acceptance::Provider { message_id })
            }
            Err(error) => {
                let (state, code) = match error {
                    ResendFailure::Rejected(code) => ("rejected", code),
                    ResendFailure::Unknown(code) => ("unknown", code),
                };
                record_failure(db, self.key, attempt, state, code)?;
                if state == "rejected" {
                    Err(Failure::new(503, "The transactional email provider did not accept this message. Please try again later."))
                } else {
                    Err(uncertain_acceptance())
                }
            }
        }
    }
}

async fn resend(request: Request) -> Result<String, ResendFailure> {
    let controller = AbortController::default();
    let signal = controller.signal();
    let fetch = async {
        let mut response = Fetch::Request(request)
            .send_with_signal(&signal)
            .await
            .map_err(|_| ResendFailure::Unknown("UNCONFIRMED_TRANSPORT_ERROR"))?;
        let status = response.status_code();
        // These statuses prove non-acceptance. In particular, 409 does not:
        // an earlier request with this key may already be accepted or in flight.
        let rejection = match status {
            400 => Some("RESEND_HTTP_400"),
            401 => Some("RESEND_HTTP_401"),
            403 => Some("RESEND_HTTP_403"),
            404 => Some("RESEND_HTTP_404"),
            405 => Some("RESEND_HTTP_405"),
            413 => Some("RESEND_HTTP_413"),
            415 => Some("RESEND_HTTP_415"),
            422 => Some("RESEND_HTTP_422"),
            429 => Some("RESEND_HTTP_429"),
            _ => None,
        };
        if let Some(code) = rejection {
            return Err(ResendFailure::Rejected(code));
        }
        if !(200..300).contains(&status) {
            return Err(ResendFailure::Unknown("UNCONFIRMED_PROVIDER_ERROR"));
        }
        let content_length = response
            .headers()
            .get("content-length")
            .ok()
            .flatten()
            .and_then(|value| value.parse::<u64>().ok());
        if content_length.is_some_and(|length| length > MAX_RESPONSE_BYTES as u64) {
            return Err(ResendFailure::Unknown("RESPONSE_TOO_LARGE"));
        }
        let mut body = Vec::new();
        let mut stream = response
            .stream()
            .map_err(|_| ResendFailure::Unknown("UNCONFIRMED_RESPONSE"))?;
        while let Some(chunk) = stream.next().await {
            let chunk = chunk.map_err(|_| ResendFailure::Unknown("UNCONFIRMED_RESPONSE"))?;
            if body.len().saturating_add(chunk.len()) > MAX_RESPONSE_BYTES {
                return Err(ResendFailure::Unknown("RESPONSE_TOO_LARGE"));
            }
            body.extend_from_slice(&chunk);
        }
        let response: ResendResponse = serde_json::from_slice(&body)
            .map_err(|_| ResendFailure::Unknown("UNCONFIRMED_RESPONSE"))?;
        if response.id.is_empty()
            || response.id.len() > 128
            || !response
                .id
                .bytes()
                .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'_' | b'-'))
        {
            return Err(ResendFailure::Unknown("UNCONFIRMED_RESPONSE"));
        }
        Ok(response.id)
    };
    let deadline = Delay::from(REQUEST_TIMEOUT);
    pin_mut!(fetch, deadline);
    let result = match select(fetch, deadline).await {
        Either::Left((result, _)) => result,
        Either::Right(((), _)) => Err(ResendFailure::Unknown("UNCONFIRMED_TIMEOUT")),
    };
    // One deadline covers headers and streaming body. Cancel unread/error bodies
    // as well; neither provider response text nor message content enters diagnostics.
    controller.abort();
    result
}

fn record_failure(
    db: &Database,
    key: &str,
    attempt: i64,
    state: &str,
    code: &str,
) -> AppResult<()> {
    let changed = db.query::<serde::de::IgnoredAny>(
        "UPDATE memory_engine_mail_receipts SET state = ?, error_code = ?, updated_at_ms = ?
         WHERE delivery_key = ? AND state = 'sending' AND attempt_count = ? RETURNING delivery_key",
        &[
            json!(state),
            json!(code),
            json!(now_ms()),
            json!(key),
            json!(attempt),
        ],
    )?;
    if changed.is_empty() {
        Err(uncertain_acceptance())
    } else {
        Ok(())
    }
}

fn uncertain_acceptance() -> Failure {
    Failure::new(503, "Email acceptance could not be confirmed. Operator reconciliation is required before this message can be retried.")
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Reconciliation {
    delivery_key: String,
    accepted: bool,
    message_id: Option<String>,
    evidence_sha256: String,
}

/// Readout is explicitly acceptance-only; reconciliation records externally
/// obtained operator evidence, not an invented provider readback API.
pub(crate) async fn handle(
    req: &mut Request,
    db: &Database,
    env: &Env,
) -> AppResult<Option<Response>> {
    let path = req.path();
    if !matches!(
        path.as_str(),
        "/internal/mail/outbox" | "/internal/mail/receipts" | "/internal/mail/reconcile"
    ) {
        return Ok(None);
    }
    auth::require_admin(req, env)?;
    if path == "/internal/mail/reconcile" {
        if req.method() != worker::Method::Post {
            return Err(Failure::new(405, "Method not allowed."));
        }
        let body: Reconciliation = auth::json_body(req).await?;
        if body.delivery_key.len() > 256
            || body.delivery_key.is_empty()
            || body.evidence_sha256.len() != 64
            || !body
                .evidence_sha256
                .bytes()
                .all(|byte| byte.is_ascii_hexdigit())
            || body.accepted
                && body.message_id.as_deref().is_none_or(|id| {
                    id.is_empty() || id.len() > 512 || id.chars().any(char::is_control)
                })
        {
            return Err(Failure::bad_request(
                "A delivery key, SHA-256 readback evidence, and accepted message id are required.",
            ));
        }
        db.transaction(|| {
            let receipt = db.query::<Receipt>("SELECT * FROM memory_engine_mail_receipts WHERE delivery_key = ?", &[json!(body.delivery_key)])?
                .pop().ok_or_else(|| Failure::not_found("Mail receipt not found."))?;
            if !matches!(receipt.state.as_str(), "sending" | "unknown")
                || receipt.state == "sending" && receipt.started_at_ms.saturating_add(5 * 60_000) > now_ms()
            {
                return Err(Failure::conflict("Only an uncertain or expired in-flight send can be reconciled."));
            }
            let now = now_ms();
            db.execute(
                "UPDATE memory_engine_mail_receipts SET state = ?, message_id = ?, accepted_at_ms = ?,
                 error_code = ?, updated_at_ms = ?, reconciled_at_ms = ?, reconciliation_evidence_sha256 = ?
                 WHERE delivery_key = ?",
                &[json!(if body.accepted { "accepted" } else { "rejected" }), json!(body.message_id),
                  if body.accepted { json!(now) } else { Value::Null }, json!("OPERATOR_READBACK"),
                  json!(now), json!(now), json!(body.evidence_sha256), json!(body.delivery_key)],
            )?;
            db.execute(
                "UPDATE memory_engine_return_notification_preferences SET claim_id = NULL,
                 claim_expires_at_ms = NULL, next_retry_at_ms = ? WHERE pending_delivery_key = ?",
                &[json!(now), json!(body.delivery_key)],
            )?;
            Ok(())
        })?;
        return Ok(Some(Response::from_json(
            &json!({ "reconciled": true, "deliveryVerified": false, "readVerified": false }),
        )?));
    }
    if !matches!(req.method(), worker::Method::Get | worker::Method::Head) {
        return Err(Failure::new(405, "Method not allowed."));
    }
    let url = req.url()?;
    let before = url
        .query_pairs()
        .find(|(name, _)| name == "before")
        .map(|(_, value)| value.into_owned());
    if path == "/internal/mail/outbox" {
        #[derive(Deserialize, Serialize)]
        struct OutboxRow {
            delivery_key: String,
            payload: String,
            created_at_ms: i64,
        }
        if !local_outbox(env)? {
            return Err(Failure::not_found("Local mail outbox is not available."));
        }
        let rows = db.query::<OutboxRow>(
            "SELECT delivery_key, payload, created_at_ms FROM memory_engine_mail_outbox
             WHERE (? IS NULL OR delivery_key < ?) ORDER BY delivery_key DESC LIMIT 100",
            &[json!(before), json!(before)],
        )?;
        return Ok(Some(Response::from_json(
            &json!({ "mode": "local-outbox", "sent": false, "messages": rows }),
        )?));
    }
    let rows = db.query::<Receipt>(
        "SELECT * FROM memory_engine_mail_receipts WHERE (? IS NULL OR delivery_key < ?)
         ORDER BY delivery_key DESC LIMIT 100",
        &[json!(before), json!(before)],
    )?;
    Ok(Some(Response::from_json(
        &json!({ "deliveryVerified": false, "readVerified": false, "receipts": rows }),
    )?))
}
