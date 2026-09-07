//! Scry's Cloudflare runtime. One `SQLite` actor owns the invite-beta transaction
//! domain; the public Worker owns ingress provenance and static delivery.

use std::{cell::Cell, fmt::Write as _, time::Duration};

use database::Database;
use js_sys::Reflect;
use serde::Deserialize;
use serde_json::json;
use wasm_bindgen::JsValue;
use worker::{durable_object, event, DurableObject, Env, Method, Request, Response, State};

mod auth;
mod database;
mod error;
mod jobs;
mod mail;
mod provider;
mod recovery;
mod store;
mod telemetry;
mod web;

pub use error::{AppResult, Failure};

const PRIMARY_OBJECT: &str = "app";
const SAFETY_ALARM_DELAY: Duration = Duration::from_secs(60);
const MAX_BACKUP_AGE_MS: i64 = 90_000_000;

#[must_use]
pub fn now_ms() -> i64 {
    #[cfg(target_arch = "wasm32")]
    {
        i64::try_from(worker::Date::now().as_millis()).unwrap_or(i64::MAX)
    }
    #[cfg(not(target_arch = "wasm32"))]
    {
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map_or(0, |elapsed| {
                i64::try_from(elapsed.as_millis()).unwrap_or(i64::MAX)
            })
    }
}

/// Generates opaque identities without platform-local counters or clock entropy.
///
/// # Errors
/// Fails closed if the runtime cannot obtain cryptographic randomness.
pub fn random_id(prefix: &str) -> AppResult<String> {
    let mut bytes = [0_u8; 16];
    getrandom::fill(&mut bytes)
        .map_err(|_| Failure::internal("Secure randomness is unavailable."))?;
    let mut result = String::with_capacity(prefix.len() + 32);
    result.push_str(prefix);
    for byte in bytes {
        write!(result, "{byte:02x}")
            .map_err(|_| Failure::internal("An opaque identity could not be encoded."))?;
    }
    Ok(result)
}

#[event(fetch)]
async fn fetch(req: Request, env: Env, _ctx: worker::Context) -> worker::Result<Response> {
    let path = req.path();
    let result = ingress(req, env).await;
    finish_response(result, &path)
}

async fn ingress(req: Request, env: Env) -> AppResult<Response> {
    let url = req.url()?;
    if url.host_str() == Some("www.scry.study") {
        let mut canonical = url;
        canonical
            .set_host(Some("scry.study"))
            .map_err(|_| Failure::internal("The canonical origin is invalid."))?;
        let mut response = Response::empty()?.with_status(308);
        response.headers_mut().set("Location", canonical.as_str())?;
        response.headers_mut().set("Cache-Control", "no-store")?;
        return Ok(response);
    }
    let path = req.path();
    if path.starts_with("/__worker/") {
        return Err(Failure::not_found("Route not found."));
    }
    let requested_target = req.headers().get("x-scry-recovery-target")?;
    let target = if let Some(target) = requested_target {
        if !recovery_route(&path) {
            return Err(Failure::forbidden(
                "An isolated target is only available for recovery operations.",
            ));
        }
        auth::require_admin(&req, &env)?;
        if !valid_target(&target) {
            return Err(Failure::bad_request("Recovery target is invalid."));
        }
        target
    } else {
        PRIMARY_OBJECT.to_owned()
    };
    if let Some(response) = web::static_response(&req)? {
        return Ok(response);
    }
    let client_ip = req
        .headers()
        .get("CF-Connecting-IP")?
        .filter(|value| value.parse::<std::net::IpAddr>().is_ok())
        .unwrap_or_else(|| "unknown".to_owned());
    let scheme = url.scheme().to_owned();
    let mut forwarded = req.clone_mut()?;
    let headers = forwarded.headers_mut()?;
    for name in [
        "forwarded",
        "x-forwarded-for",
        "x-forwarded-proto",
        "x-real-ip",
        "x-scry-recovery-target",
    ] {
        headers.delete(name)?;
    }
    headers.set("x-scry-client-ip", &client_ip)?;
    headers.set("x-scry-edge-scheme", &scheme)?;
    headers.set("x-scry-object-name", &target)?;
    let namespace = env.durable_object("SCRY")?;
    let response = namespace
        .id_from_name(&target)?
        .get_stub()?
        .fetch_with_request(forwarded)
        .await?;
    // Fetch response headers are immutable. Own only the header map; forward
    // the body stream without buffering or cloning it.
    let headers = response.headers().clone();
    Ok(response.with_headers(headers))
}

fn recovery_route(path: &str) -> bool {
    path.starts_with("/internal/migration/") || path.starts_with("/internal/recovery/")
}

fn valid_target(target: &str) -> bool {
    target == PRIMARY_OBJECT
        || target.strip_prefix("restore-").is_some_and(|suffix| {
            (8..=64).contains(&suffix.len())
                && suffix
                    .bytes()
                    .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
        })
}

fn maintenance(db: &Database) -> AppResult<bool> {
    #[derive(Deserialize)]
    struct Runtime {
        learner_traffic_enabled: u8,
    }
    db.query::<Runtime>(
        "SELECT learner_traffic_enabled FROM memory_engine_recovery_state WHERE singleton = 1",
        &[],
    )?
    .pop()
    .map(|runtime| runtime.learner_traffic_enabled != 1)
    .ok_or_else(|| Failure::internal("Runtime control is unavailable."))
}

// Pausing must not return while an already admitted request can still commit.
// SQLite controls admission; this guard covers only live awaited operations.
struct Activity<'a>(&'a Cell<usize>);

impl Drop for Activity<'_> {
    fn drop(&mut self) {
        self.0.set(self.0.get() - 1);
    }
}

/// Recovery actors use the same schema and bytes, but cannot serve learners or
/// execute migrated generation/reminder work. Their identity is runtime-owned.
#[durable_object]
pub struct Scry {
    state: State,
    env: Env,
    raw_storage: Result<JsValue, JsValue>,
    initialized: Cell<bool>,
    in_flight: Cell<usize>,
}

impl DurableObject for Scry {
    fn new(state: State, env: Env) -> Self {
        let inner = state._inner();
        let raw_storage = Reflect::get(inner.as_ref(), &JsValue::from_str("storage"));
        Self {
            state: State::from(inner),
            env,
            raw_storage,
            initialized: Cell::new(false),
            in_flight: Cell::new(0),
        }
    }

    async fn fetch(&self, mut req: Request) -> worker::Result<Response> {
        let path = req.path();
        let result = self.handle(&mut req).await;
        finish_response(result, &path)
    }

    async fn alarm(&self) -> worker::Result<Response> {
        if self
            .object_name()
            .map_err(|failure| worker_failure(failure.status))?
            != PRIMARY_OBJECT
            || maintenance(
                &self
                    .database()
                    .map_err(|failure| worker_failure(failure.status))?,
            )
            .map_err(|failure| worker_failure(failure.status))?
        {
            self.state.storage().delete_alarm().await?;
            return Response::empty();
        }
        self.run_background()
            .await
            .map_err(|failure| worker_failure(failure.status))?;
        Response::empty()
    }
}

impl Scry {
    fn object_name(&self) -> AppResult<String> {
        self.state
            .id()
            .name()
            .filter(|name| valid_target(name))
            .ok_or_else(|| {
                Failure::forbidden("This application object has no authorized identity.")
            })
    }

    fn database(&self) -> AppResult<Database> {
        let raw_storage = self
            .raw_storage
            .as_ref()
            .map_err(|_| Failure::internal("Durable storage is unavailable."))?
            .clone();
        let database = Database::new(self.state.storage().sql(), raw_storage);
        if !self.initialized.get() {
            database::initialize(&database)?;
            self.initialized.set(true);
        }
        Ok(database)
    }

    fn activity(&self) -> Activity<'_> {
        self.in_flight.set(self.in_flight.get() + 1);
        Activity(&self.in_flight)
    }

    async fn runtime(&self, req: &mut Request, db: &Database) -> AppResult<Response> {
        auth::require_admin(req, &self.env)?;
        if req.method() == Method::Post {
            #[derive(Deserialize)]
            #[serde(rename_all = "camelCase", deny_unknown_fields)]
            struct Traffic {
                enabled: bool,
                expected_fingerprint: String,
            }
            let traffic: Traffic = web::json_body(req, 4096).await?;
            db.transaction(|| {
                if self.in_flight.get() != 0 {
                    return Err(Failure::conflict("An admitted operation is still running. Wait for it to finish before changing traffic."));
                }
                if traffic.expected_fingerprint.len() != 64
                    || traffic.expected_fingerprint != recovery::fingerprint_sha256(db)? {
                    return Err(Failure::conflict("Durable state changed. Inspect a fresh fingerprint before changing traffic."));
                }
                if traffic.enabled && mail::variable(&self.env, "MEMORY_ENGINE_ENVIRONMENT").as_deref() == Some("production")
                    && db.query::<serde::de::IgnoredAny>(
                        "SELECT import_id FROM memory_engine_recovery_imports WHERE target = 'app' AND json_extract(source_metadata, '$.source.engine') = 'postgresql' LIMIT 1", &[],
                    )?.is_empty() {
                    return Err(Failure::conflict("Production traffic requires verified imported learner state."));
                }
                db.execute("UPDATE memory_engine_recovery_state SET learner_traffic_enabled = ? WHERE singleton = 1", &[json!(u8::from(traffic.enabled))])
            })?;
            if traffic.enabled {
                self.arm_safety_alarm().await?;
                self.arm_next_alarm(db).await?;
            } else {
                self.state.storage().delete_alarm().await?;
            }
        } else if req.method() != Method::Get {
            return Err(Failure::new(405, "Method not allowed."));
        }
        Ok(Response::from_json(
            &json!({"maintenance": maintenance(db)?}),
        )?)
    }

    async fn handle(&self, req: &mut Request) -> AppResult<Response> {
        let object_name = self.object_name()?;
        if req.headers().get("x-scry-object-name")?.as_deref() != Some(&object_name) {
            return Err(Failure::forbidden(
                "Application object identity does not match ingress.",
            ));
        }
        let path = req.path();
        if object_name != PRIMARY_OBJECT {
            auth::require_admin(req, &self.env)?;
            if !recovery_route(&path) {
                return Err(Failure::forbidden(
                    "Recovery objects cannot serve application traffic.",
                ));
            }
            let db = self.database()?;
            return recovery::handle(req, &db, &self.env)
                .await?
                .ok_or_else(|| Failure::not_found("Recovery route not found."));
        }
        let db = self.database()?;
        if path == "/internal/runtime" {
            return self.runtime(req, &db).await;
        }
        if path == "/statusz" {
            if req.method() != Method::Get {
                return Err(Failure::new(405, "Method not allowed."));
            }
            // A witness must not wake jobs or repair the backup it is checking.
            let paused = maintenance(&db)?;
            let backup_age = backup_age_ms(&db)?;
            let healthy =
                !paused && backup_age.is_some_and(|age| (0..=MAX_BACKUP_AGE_MS).contains(&age));
            return Ok(Response::from_json(&json!({
                "schema": "memory_engine.runtime_health.v1",
                "status": if healthy { "healthy" } else { "degraded" },
                "maintenance": paused,
                "backupAgeMs": backup_age
            }))?
            .with_status(if healthy { 200 } else { 503 }));
        }
        if path == "/__worker/scheduled" {
            let paused = maintenance(&db)?;
            if !paused {
                self.run_background().await?;
            }
            return Ok(Response::from_json(&json!({
                "maintenance": paused,
                "backupAgeMs": backup_age_ms(&db)?
            }))?);
        }
        if recovery_route(&path) {
            if path == "/internal/migration/import" && !maintenance(&db)? {
                return Err(Failure::conflict(
                    "Pause primary traffic before importing learner state.",
                ));
            }
            let _activity = self.activity();
            return recovery::handle(req, &db, &self.env)
                .await?
                .ok_or_else(|| Failure::not_found("Recovery route not found."));
        }
        let paused = maintenance(&db)?;
        if paused
            && path != "/healthz"
            && !(req.method() == Method::Get && path.starts_with("/internal/"))
        {
            let mut response = Response::from_json(&json!({
                "error": "Scry is paused for a verified data cutover.", "maintenance": true
            }))?
            .with_status(503);
            response.headers_mut().set("Retry-After", "60")?;
            return Ok(response);
        }
        if path == "/healthz" || path == "/readyz" {
            return web::handle(req, &db, &self.env).await;
        }
        let _activity = self.activity();
        // Persist a recovery wake BEFORE any body/network await can enqueue or
        // claim work; request cancellation must not strand a committed job.
        if !paused {
            self.arm_safety_alarm().await?;
        }
        let result = web::handle(req, &db, &self.env).await;
        if !paused {
            self.arm_next_alarm(&db).await?;
        }
        telemetry::flush();
        result
    }

    async fn arm_safety_alarm(&self) -> AppResult<()> {
        let storage = self.state.storage();
        let current = storage.get_alarm().await?;
        if current.is_none_or(|deadline| deadline > now_ms().saturating_add(60_000)) {
            storage.set_alarm(SAFETY_ALARM_DELAY).await?;
        }
        Ok(())
    }

    async fn arm_next_alarm(&self, db: &Database) -> AppResult<()> {
        let storage = self.state.storage();
        let current = storage.get_alarm().await?;
        // Query after the storage await; another request may have enqueued while
        // this request was awaiting an external provider.
        let next = [
            jobs::next_run_at(db)?,
            auth::next_reminder_at(db)?,
            recovery::next_run_at(db)?,
        ]
        .into_iter()
        .flatten()
        .min();
        match next {
            Some(next) => {
                let delay_ms =
                    u64::try_from(next.saturating_sub(now_ms()).max(1_000)).unwrap_or(u64::MAX);
                let deadline = now_ms().saturating_add(i64::try_from(delay_ms).unwrap_or(i64::MAX));
                if current.is_none_or(|current| current > deadline) {
                    storage.set_alarm(Duration::from_millis(delay_ms)).await?;
                }
            }
            None if current.is_some() => {
                storage.delete_alarm().await?;
            }
            None => (),
        }
        Ok(())
    }

    async fn run_background(&self) -> AppResult<()> {
        let db = self.database()?;
        if maintenance(&db)? {
            return Ok(());
        }
        let _activity = self.activity();
        self.arm_safety_alarm().await?;
        let generation = jobs::run_due(&db, &self.env).await;
        let reminders = auth::run_reminders(&db, &self.env).await;
        let backups = recovery::run_due(&db, &self.env).await;
        self.arm_next_alarm(&db).await?;
        telemetry::flush();
        generation?;
        reminders?;
        backups?;
        Ok(())
    }
}

#[event(scheduled)]
async fn scheduled(_event: worker::ScheduledEvent, env: Env, ctx: worker::ScheduleContext) {
    ctx.wait_until(async move {
        let result = scheduled_wake(&env).await;
        match result {
            Ok(health) if !health.maintenance => {
                let healthy = health
                    .backup_age_ms
                    .is_some_and(|age| (0..=MAX_BACKUP_AGE_MS).contains(&age));
                telemetry::report_health(healthy, health.backup_age_ms);
            }
            Ok(_) => (),
            Err(failure) => {
                worker::console_error!(
                    "{{\"event\":\"scry.scheduler.failed\",\"status\":{}}}",
                    failure.status
                );
                telemetry::report_health(false, None);
            }
        }
    });
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Health {
    maintenance: bool,
    backup_age_ms: Option<i64>,
}
async fn scheduled_wake(env: &Env) -> AppResult<Health> {
    let mut request = Request::new("https://scry.internal/__worker/scheduled", Method::Post)?;
    request
        .headers_mut()?
        .set("x-scry-object-name", PRIMARY_OBJECT)?;
    let namespace = env.durable_object("SCRY")?;
    let mut response = namespace
        .id_from_name(PRIMARY_OBJECT)?
        .get_stub()?
        .fetch_with_request(request)
        .await?;
    if !(200..300).contains(&response.status_code()) {
        return Err(Failure::new(
            response.status_code(),
            "The durable scheduler did not complete.",
        ));
    }
    Ok(response.json().await?)
}

fn backup_age_ms(db: &Database) -> AppResult<Option<i64>> {
    #[derive(Deserialize)]
    struct Backup {
        last_backup_at_ms: Option<i64>,
    }
    Ok(db
        .query::<Backup>(
            "SELECT last_backup_at_ms FROM memory_engine_recovery_state WHERE singleton = 1",
            &[],
        )?
        .pop()
        .and_then(|row| row.last_backup_at_ms)
        .and_then(|at| now_ms().checked_sub(at))
        .filter(|age| *age >= 0))
}

fn worker_failure(status: u16) -> worker::Error {
    // Do not let an exception include learner or transport payloads.
    worker::Error::RustError(format!("Scry operation failed (status {status})."))
}

fn finish_response(result: AppResult<Response>, path: &str) -> worker::Result<Response> {
    let mut response = match result {
        Ok(response) => response,
        Err(failure) => {
            worker::console_error!(
                "{{\"event\":\"scry.request.failed\",\"status\":{}}}",
                failure.status
            );
            if path.starts_with("/v1/")
                || path.starts_with("/accounts")
                || path.starts_with("/internal/")
                || path == "/readyz"
                || path == "/healthz"
                || path == "/statusz"
            {
                Response::from_json(&json!({"error": failure.message}))?.with_status(failure.status)
            } else {
                Response::from_html(memory_engine_api_render::render_entry_recovery(
                    "The action could not finish",
                    &failure.message,
                ))?
                .with_status(failure.status)
            }
        }
    };
    if !path.starts_with("/static/")
        && !matches!(
            path,
            "/sw.js"
                | "/offline.html"
                | "/manifest.webmanifest"
                | "/favicon.png"
                | "/icon-192.png"
                | "/icon-512.png"
                | "/apple-touch-icon.png"
        )
    {
        response.headers_mut().set("Cache-Control", "no-store")?;
    }
    response
        .headers_mut()
        .set("X-Content-Type-Options", "nosniff")?;
    response
        .headers_mut()
        .set("Referrer-Policy", "no-referrer")?;
    Ok(response)
}
