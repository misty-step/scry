//! Content-free browser receipts and bounded Worker Fetch delivery.
//!
//! Recording, scheduling a flush, receiving HTTP 2xx, and durable Canary
//! readback are different boundaries. Only the Fetch response counts as ingest
//! acceptance. Isolate-local aggregate admission is deliberately best effort.

use std::{cell::RefCell, collections::VecDeque, time::Duration};

use futures_util::future::{select, Either};
use memory_engine_performance::{
    Action, Boot, CompletionMarker, CompletionPhase, Namespace, Navigation, Observation, Outcome,
    ReviewAction, Snapshot, Viewport, Window, MAX_AGGREGATE_BATCHES_PER_MINUTE,
    REQUEST_UI_MAX_DURATION_MS,
};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use worker::{
    AbortController, Delay, Env, Fetch, Headers, Method, Request, RequestInit, RequestRedirect,
    Response, Url,
};

use crate::{auth, database::Database, now_ms, web, AppResult, Failure};

const MAX_BROWSER_RECEIPT_BYTES: usize = 4096;
const MAX_PENDING_SERIES: usize = 40;
const MAX_PENDING_WINDOWS: usize = 2;
const MAX_BATCH_ATTRIBUTES_BYTES: usize = 8_000;
const FETCH_TIMEOUT: Duration = Duration::from_secs(2);

#[derive(Clone, Copy, Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum DeliveryState {
    Disabled,
    Admitted,
    Accepted,
    Rejected,
    TimedOut,
    TransportFailed,
}

/// Actual observed delivery, never a successful `waitUntil` scheduling receipt.
#[derive(Clone, Debug, Serialize)]
pub struct DeliveryReport {
    pub state: DeliveryState,
    pub attempts: u32,
    pub accepted_requests: u64,
    pub dropped_requests: u64,
    pub accepted_observations: u64,
    pub dropped_observations: u64,
    pub pending_observations: u64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider_status: Option<u16>,
}

impl DeliveryReport {
    fn pending(state: DeliveryState, count: u64) -> Self {
        Self {
            state,
            attempts: 0,
            accepted_requests: 0,
            dropped_requests: 0,
            accepted_observations: 0,
            dropped_observations: 0,
            pending_observations: count,
            provider_status: None,
        }
    }

    fn label(&self) -> &'static str {
        match self.state {
            DeliveryState::Disabled => "disabled",
            DeliveryState::Admitted => "admitted",
            DeliveryState::Accepted => "provider_accepted",
            DeliveryState::Rejected => "provider_rejected",
            DeliveryState::TimedOut => "timed_out",
            DeliveryState::TransportFailed => "transport_failed",
        }
    }
}

#[derive(Clone, Copy, Deserialize)]
#[serde(rename_all = "snake_case")]
enum BrowserViewport {
    Mobile,
    Tablet,
    Desktop,
}

impl BrowserViewport {
    fn performance(self) -> Viewport {
        match self {
            Self::Mobile => Viewport::Mobile,
            Self::Tablet => Viewport::Tablet,
            Self::Desktop => Viewport::Desktop,
        }
    }

    fn label(self) -> &'static str {
        match self {
            Self::Mobile => "mobile",
            Self::Tablet => "tablet",
            Self::Desktop => "desktop",
        }
    }
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct BrowserSubmitPerformance {
    schema: String,
    csrf_token: Option<String>,
    request_id: String,
    trace_id: String,
    navigation: Navigation,
    tap_to_ack_ms: u64,
    request_to_response_ms: Option<u64>,
    transfer_ms: Option<u64>,
    navigation_ms: Option<u64>,
    dom_swap_ms: Option<u64>,
    graded_visible_ms: u64,
    viewport: BrowserViewport,
}

pub(crate) fn strict_opaque_id(value: &str, prefix: &str) -> bool {
    value.len() == prefix.len() + 32
        && value.starts_with(prefix)
        && value.as_bytes()[prefix.len()..]
            .iter()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(byte))
}

fn valid_browser_receipt(event: &BrowserSubmitPerformance) -> bool {
    if event.schema != "memory_engine.browser_submit.v2"
        || !strict_opaque_id(&event.request_id, "req_")
        || !strict_opaque_id(&event.trace_id, "trace_")
        || !matches!(event.navigation, Navigation::InPlace | Navigation::FullPage)
        || event.graded_visible_ms > REQUEST_UI_MAX_DURATION_MS
        || event.tap_to_ack_ms > event.graded_visible_ms
    {
        return false;
    }
    let phases = [
        event.request_to_response_ms,
        event.transfer_ms,
        event.navigation_ms,
        event.dom_swap_ms,
    ];
    if phases
        .iter()
        .flatten()
        .any(|phase| *phase > event.graded_visible_ms)
        || event
            .request_to_response_ms
            .is_some_and(|phase| event.tap_to_ack_ms > phase)
    {
        return false;
    }
    if matches!(event.navigation, Navigation::InPlace) && event.navigation_ms.is_some()
        || matches!(event.navigation, Navigation::FullPage) && event.dom_swap_ms.is_some()
    {
        return false;
    }
    let observed_sum: u64 = phases.iter().flatten().sum();
    let earliest_response = if event.request_to_response_ms.is_none() {
        event.tap_to_ack_ms
    } else {
        0
    };
    if observed_sum + earliest_response > event.graded_visible_ms + 4 {
        return false;
    }
    if let (Navigation::FullPage, Some(response), Some(transfer), Some(navigation)) = (
        event.navigation,
        event.request_to_response_ms,
        event.transfer_ms,
        event.navigation_ms,
    ) {
        return event
            .graded_visible_ms
            .abs_diff(response + transfer + navigation)
            <= 4;
    }
    true
}

/// A valid browser receipt is sent to the configured provider here, not merely
/// admitted to an unobserved background future. The learner's answer has already
/// committed, so a telemetry outage cannot turn grading into a failed action.
pub async fn handle(req: &mut Request, db: &Database, env: &Env) -> AppResult<Option<Response>> {
    if req.path() != "/app/performance/submit" {
        return Ok(None);
    }
    if req.method() != Method::Post {
        return Err(Failure::new(405, "Method not allowed."));
    }
    let raw: Value = web::json_body(req, MAX_BROWSER_RECEIPT_BYTES).await?;
    let csrf = raw
        .get("csrfToken")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let account = auth::browser_account(req, db, env, Some(csrf))?;
    let event: BrowserSubmitPerformance = serde_json::from_value(raw)
        .map_err(|_| Failure::bad_request("Browser performance receipt is invalid."))?;
    if !valid_browser_receipt(&event)
        || event.csrf_token.as_deref().map(str::trim) != Some(account.csrf_token())
    {
        return Err(Failure::bad_request(
            "Browser performance receipt is invalid.",
        ));
    }
    let mut attributes = json!({
        "schema": "memory_engine.browser_submit_durations.v2",
        "request_id": event.request_id,
        "trace_id": event.trace_id,
        "viewport": event.viewport.label(),
        "navigation": event.navigation,
        "tap_to_ack_ms": event.tap_to_ack_ms,
        "graded_visible_ms": event.graded_visible_ms,
    });
    for (name, phase) in [
        ("request_to_response_ms", event.request_to_response_ms),
        ("transfer_ms", event.transfer_ms),
        ("navigation_ms", event.navigation_ms),
        ("dom_swap_ms", event.dom_swap_ms),
    ] {
        if let Some(duration) = phase {
            attributes[name] = Value::from(duration);
        }
    }
    // The raw account, cookie, CSRF, source, answer and feedback never leave this
    // adapter. Only the explicitly reconstructed allowlist above is submitted.
    let body = event_body(
        "memory_engine.browser_submit_durations.v2",
        "Observed browser review completion",
        attributes,
    );
    let report = send_event(env, &body, 1).await;
    let accepted = matches!(report.state, DeliveryState::Accepted);
    if accepted {
        for (phase, duration) in [
            (CompletionPhase::ImmediateAck, event.tap_to_ack_ms),
            (
                CompletionPhase::VisibleAfterTwoAnimationFrames,
                event.graded_visible_ms,
            ),
        ] {
            if let Ok(marker) = CompletionMarker::browser(
                Action::Review(ReviewAction::Submit),
                phase,
                Outcome::Succeeded,
                event.navigation,
                event.viewport.performance(),
            ) {
                if let Ok(observation) = marker.observation(duration) {
                    record_observation(observation);
                }
            }
        }
    }
    let mut response = if accepted {
        Response::empty()?.with_status(204)
    } else {
        web::json_response(
            &json!({"error": "Browser performance receipt was not accepted by telemetry ingest."}),
            503,
        )?
    };
    response
        .headers_mut()
        .set("x-scry-telemetry-delivery", report.label())?;
    response
        .headers_mut()
        .set("x-scry-telemetry-attempts", &report.attempts.to_string())?;
    auth::refresh_browser_cookie(req, &account, &mut response)?;
    log_delivery("browser_receipt", &report);
    Ok(Some(web::no_store(response)?))
}

#[derive(Default, Serialize)]
struct Accounting {
    batches_sent: u64,
    batches_retried: u64,
    batches_dropped: u64,
    observations_dropped: u64,
    observations_invalid: u64,
    series_dropped: u64,
}

struct PendingWindow {
    minute: u64,
    namespace: Namespace,
    snapshots: Vec<Snapshot>,
}

struct Aggregate {
    boot: Boot,
    pending: VecDeque<PendingWindow>,
    accounting: [Accounting; 3],
    export_minute: u64,
    exports: u8,
    flushing: bool,
}

impl Aggregate {
    fn new() -> Self {
        Self {
            boot: Boot::new(u64::try_from(now_ms()).unwrap_or_default()),
            pending: VecDeque::new(),
            accounting: std::array::from_fn(|_| Accounting::default()),
            export_minute: current_minute(),
            exports: 0,
            flushing: false,
        }
    }

    fn count(&self) -> u64 {
        self.pending
            .iter()
            .flat_map(|window| &window.snapshots)
            .map(Snapshot::count)
            .sum()
    }

    fn record(&mut self, observation: Observation) {
        let minute = current_minute();
        let namespace = observation.marker().namespace();
        while self.pending.front().is_some_and(|window| {
            window.minute.saturating_add(MAX_PENDING_WINDOWS as u64) <= minute
        }) {
            if let Some(window) = self.pending.pop_front() {
                let accounting = &mut self.accounting[namespace_index(window.namespace)];
                accounting.series_dropped = accounting
                    .series_dropped
                    .saturating_add(window.snapshots.len() as u64);
                accounting.observations_dropped = accounting
                    .observations_dropped
                    .saturating_add(window.snapshots.iter().map(Snapshot::count).sum::<u64>());
            }
        }
        let index = self
            .pending
            .iter()
            .position(|window| window.minute == minute && window.namespace == namespace)
            .unwrap_or_else(|| {
                // Even an unexpected backwards clock step cannot grow the queue.
                if self.pending.len() == MAX_PENDING_WINDOWS * 3 {
                    if let Some(expired) = self.pending.pop_front() {
                        let accounting = &mut self.accounting[namespace_index(expired.namespace)];
                        accounting.series_dropped = accounting
                            .series_dropped
                            .saturating_add(expired.snapshots.len() as u64);
                        accounting.observations_dropped =
                            accounting.observations_dropped.saturating_add(
                                expired.snapshots.iter().map(Snapshot::count).sum::<u64>(),
                            );
                    }
                }
                self.pending.push_back(PendingWindow {
                    minute,
                    namespace,
                    snapshots: Vec::new(),
                });
                self.pending.len() - 1
            });
        let window = &mut self.pending[index];
        if let Some(snapshot) = window
            .snapshots
            .iter_mut()
            .find(|snapshot| snapshot.marker() == observation.marker())
        {
            if snapshot.record(observation).is_err() {
                self.accounting[namespace_index(namespace)].observations_invalid += 1;
            }
        } else if window.snapshots.len() == MAX_PENDING_SERIES {
            self.accounting[namespace_index(namespace)].observations_dropped += 1;
        } else {
            match Snapshot::new(observation.marker(), Window::new(minute), self.boot) {
                Ok(mut snapshot) => {
                    if snapshot.record(observation).is_ok() {
                        window.snapshots.push(snapshot);
                    } else {
                        self.accounting[namespace_index(namespace)].observations_invalid += 1;
                    }
                }
                Err(_) => self.accounting[namespace_index(namespace)].observations_invalid += 1,
            }
        }
    }

    fn take_batch(&mut self) -> Option<(Value, u64, Namespace)> {
        let minute = current_minute();
        if self.export_minute != minute {
            self.export_minute = minute;
            self.exports = 0;
        }
        if self.exports >= MAX_AGGREGATE_BATCHES_PER_MINUTE {
            return None;
        }
        let mut window = self.pending.pop_front()?;
        let mut accounting =
            std::mem::take(&mut self.accounting[namespace_index(window.namespace)]);
        loop {
            let attributes = json!({
                "schema": "memory_engine.performance_batch.v1", "authority": "non_authoritative_debug",
                "namespace": window.namespace, "window": {"start_minute": window.minute},
                "delivery": accounting, "snapshots": window.snapshots,
            });
            if serde_json::to_vec(&attributes)
                .is_ok_and(|encoded| encoded.len() <= MAX_BATCH_ATTRIBUTES_BYTES)
            {
                self.exports += 1;
                let count = window.snapshots.iter().map(Snapshot::count).sum();
                return Some((
                    event_body(
                        "memory_engine.performance.snapshot",
                        "Bounded memory-engine performance aggregate",
                        attributes,
                    ),
                    count,
                    window.namespace,
                ));
            }
            let snapshot = window.snapshots.pop()?;
            accounting.series_dropped = accounting.series_dropped.saturating_add(1);
            accounting.observations_dropped = accounting
                .observations_dropped
                .saturating_add(snapshot.count());
        }
    }
}

thread_local! {
    static AGGREGATE: RefCell<Option<Aggregate>> = const { RefCell::new(None) };
}

fn aggregate<T>(operation: impl FnOnce(&mut Aggregate) -> T) -> T {
    AGGREGATE.with(|state| operation(state.borrow_mut().get_or_insert_with(Aggregate::new)))
}

fn namespace_index(namespace: Namespace) -> usize {
    match namespace {
        Namespace::Browser => 0,
        Namespace::Server => 1,
        Namespace::Job => 2,
    }
}

fn current_minute() -> u64 {
    u64::try_from(now_ms()).unwrap_or_default() / 60_000
}

pub fn record_observation(observation: Observation) {
    aggregate(|aggregate| aggregate.record(observation));
}

/// Runs no network I/O and cannot turn a committed grade into an HTTP failure.
pub fn record_submit_server(duration_ms: u64, status: u16) {
    let outcome = if (200..300).contains(&status) {
        Outcome::Succeeded
    } else if status >= 500 {
        Outcome::ServerFailed
    } else {
        Outcome::ClientRejected
    };
    let observation = CompletionMarker::server(
        Action::Review(ReviewAction::Submit),
        CompletionPhase::ImmediateAck,
        outcome,
    )
    .ok()
    .and_then(|marker| marker.observation(duration_ms).ok());
    if let Some(observation) = observation {
        record_observation(observation);
    } else {
        aggregate(|aggregate| {
            aggregate.accounting[namespace_index(Namespace::Server)].observations_invalid += 1;
        });
    }
}

struct FlushGuard;

impl Drop for FlushGuard {
    fn drop(&mut self) {
        // A cancelled waitUntil must not permanently latch the sender.
        aggregate(|aggregate| aggregate.flushing = false);
    }
}

/// Main schedules this with its Worker/DO waitUntil context. Concurrent drains
/// coalesce; there are at most three bounded aggregate batches per minute.
/// Pending observations in the returned report have NOT been accepted remotely.
pub async fn flush(env: &Env) -> DeliveryReport {
    let pending = aggregate(|aggregate| aggregate.count());
    if pending == 0 {
        return DeliveryReport::pending(DeliveryState::Admitted, 0);
    }
    if configuration(env, "/api/v1/events").is_none() {
        return DeliveryReport::pending(DeliveryState::Disabled, pending);
    }
    let acquired = aggregate(|aggregate| {
        if aggregate.flushing {
            false
        } else {
            aggregate.flushing = true;
            true
        }
    });
    if !acquired {
        return DeliveryReport::pending(DeliveryState::Admitted, pending);
    }
    let _guard = FlushGuard;
    let mut summary = DeliveryReport::pending(DeliveryState::Admitted, pending);
    loop {
        let batch = aggregate(Aggregate::take_batch);
        let Some((body, count, namespace)) = batch else {
            break;
        };
        let delivery = send_event(env, &body, count).await;
        aggregate(|aggregate| {
            let accounting = &mut aggregate.accounting[namespace_index(namespace)];
            accounting.batches_sent = accounting
                .batches_sent
                .saturating_add(delivery.accepted_requests);
            accounting.batches_retried = accounting
                .batches_retried
                .saturating_add(u64::from(delivery.attempts.saturating_sub(1)));
            accounting.batches_dropped = accounting
                .batches_dropped
                .saturating_add(delivery.dropped_requests);
            accounting.observations_dropped = accounting
                .observations_dropped
                .saturating_add(delivery.dropped_observations);
        });
        summary.state = delivery.state;
        summary.attempts += delivery.attempts;
        summary.accepted_requests += delivery.accepted_requests;
        summary.dropped_requests += delivery.dropped_requests;
        summary.accepted_observations += delivery.accepted_observations;
        summary.dropped_observations += delivery.dropped_observations;
        summary.provider_status = delivery.provider_status;
    }
    summary.pending_observations = aggregate(|aggregate| aggregate.count());
    if summary.dropped_requests > 0 {
        summary.state = DeliveryState::Rejected;
    } else if summary.pending_observations > 0 {
        summary.state = DeliveryState::Admitted;
    }
    log_delivery("aggregate", &summary);
    summary
}

struct Config {
    endpoint: String,
    api_key: String,
}

fn configuration(env: &Env, route: &str) -> Option<Config> {
    let endpoint = crate::mail::variable(env, "CANARY_ENDPOINT")?;
    let key = env.secret("CANARY_API_KEY").ok()?.to_string();
    let mut url = Url::parse(endpoint.trim_end_matches('/')).ok()?;
    if url.scheme() != "https"
        || url.host_str().is_none()
        || !url.username().is_empty()
        || url.password().is_some()
        || url.query().is_some()
        || url.fragment().is_some()
        || key.trim().is_empty()
    {
        return None;
    }
    let path = format!("{}{route}", url.path().trim_end_matches('/'));
    url.set_path(&path);
    Some(Config {
        endpoint: url.to_string(),
        api_key: key,
    })
}

fn event_body(name: &str, summary: &str, attributes: Value) -> Value {
    let mut body = json!({"service": "memory-engine-api", "name": name, "summary": summary,
        "severity": "info", "sampling_policy": "unsampled",
        "retention_class": "standard", "privacy_policy": "redacted"});
    body["attributes"] = attributes;
    body
}

async fn send_event(env: &Env, body: &Value, observations: u64) -> DeliveryReport {
    send_payload(env, "/api/v1/events", body, observations).await
}

async fn send_payload(env: &Env, route: &str, body: &Value, observations: u64) -> DeliveryReport {
    let Some(config) = configuration(env, route) else {
        let mut report = DeliveryReport::pending(DeliveryState::Disabled, 0);
        report.dropped_observations = observations;
        return report;
    };
    let Ok(encoded) = serde_json::to_string(body) else {
        return failed_delivery(DeliveryState::Rejected, 0, None, observations);
    };
    let mut attempts = 0;
    loop {
        attempts += 1;
        let result = fetch_attempt(&config, &encoded).await;
        match result {
            Ok(status) if (200..300).contains(&status) => {
                return DeliveryReport {
                    state: DeliveryState::Accepted,
                    attempts,
                    accepted_requests: 1,
                    dropped_requests: 0,
                    accepted_observations: observations,
                    dropped_observations: 0,
                    pending_observations: 0,
                    provider_status: Some(status),
                }
            }
            Ok(status) if attempts < 2 && (matches!(status, 408 | 429) || status >= 500) => {}
            Ok(status) => {
                return failed_delivery(
                    DeliveryState::Rejected,
                    attempts,
                    Some(status),
                    observations,
                )
            }
            Err(_) if attempts < 2 => {}
            Err(state) => return failed_delivery(state, attempts, None, observations),
        }
    }
}

/// Cron observes the actor and its last verified R2 backup, not just ingress
/// liveness. The separate monitor cannot mask the native service during cutover.
pub async fn report_health(env: &Env, healthy: bool, backup_age_ms: Option<i64>) -> DeliveryReport {
    let production =
        crate::mail::variable(env, "MEMORY_ENGINE_ENVIRONMENT").as_deref() == Some("production");
    let body = json!({
        "monitor": if production { "memory-engine-api-cloudflare" } else { "memory-engine-api-cloudflare-staging" },
        "status": if healthy { "alive" } else { "failed" },
        "summary": if healthy { "Scry runtime and recovery healthy" } else { "Scry runtime or recovery degraded" },
        "ttl_ms": 180_000,
        "context": { "source": "memory-engine-api", "storage": "durable-object-sqlite", "backupAgeMs": backup_age_ms }
    });
    let report = send_payload(env, "/api/v1/check-ins", &body, 0).await;
    log_delivery("health_check_in", &report);
    report
}

fn failed_delivery(
    state: DeliveryState,
    attempts: u32,
    provider_status: Option<u16>,
    observations: u64,
) -> DeliveryReport {
    DeliveryReport {
        state,
        attempts,
        accepted_requests: 0,
        dropped_requests: 1,
        accepted_observations: 0,
        dropped_observations: observations,
        pending_observations: 0,
        provider_status,
    }
}

async fn fetch_attempt(config: &Config, body: &str) -> Result<u16, DeliveryState> {
    let headers = Headers::new();
    headers
        .set("content-type", "application/json")
        .map_err(|_| DeliveryState::TransportFailed)?;
    headers
        .set("authorization", &format!("Bearer {}", config.api_key))
        .map_err(|_| DeliveryState::TransportFailed)?;
    let mut init = RequestInit::new();
    init.with_method(Method::Post)
        .with_headers(headers)
        .with_redirect(RequestRedirect::Manual)
        .with_body(Some(body.into()));
    let request = Request::new_with_init(&config.endpoint, &init)
        .map_err(|_| DeliveryState::TransportFailed)?;
    let abort = AbortController::default();
    let signal = abort.signal();
    let fetch = Fetch::Request(request);
    let completed = select(
        Box::pin(fetch.send_with_signal(&signal)),
        Box::pin(Delay::from(FETCH_TIMEOUT)),
    )
    .await;
    match completed {
        Either::Left((result, _)) => result
            .map(|response| response.status_code())
            .map_err(|_| DeliveryState::TransportFailed),
        Either::Right(_) => {
            abort.abort();
            Err(DeliveryState::TimedOut)
        }
    }
}

fn log_delivery(kind: &str, report: &DeliveryReport) {
    // Closed labels and numeric counters only; no request, account, source,
    // provider response body, endpoint, cookie or token can enter this receipt.
    worker::console_log!(
        "{}",
        json!({"schema": "memory_engine.worker_delivery.v1", "kind": kind,
        "authority": "observed_transport", "delivery": report})
    );
}
