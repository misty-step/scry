//! Content-free browser receipts and bounded Cloudflare runtime logs.
//!
//! Only explicitly allowlisted fields reach the runtime logger. Isolate-local
//! aggregation is best effort; logging is not remote ingest acceptance or
//! evidence of durable telemetry storage.

use std::{cell::RefCell, collections::VecDeque};

use memory_engine_performance::{
    Action, Boot, CompletionMarker, CompletionPhase, Namespace, Navigation, Observation, Outcome,
    ReviewAction, Snapshot, Viewport, Window, MAX_AGGREGATE_BATCHES_PER_MINUTE,
    REQUEST_UI_MAX_DURATION_MS,
};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use worker::{Env, Method, Request, Response};

use crate::{auth, database::Database, now_ms, web, AppResult, Failure};

const MAX_BROWSER_RECEIPT_BYTES: usize = 4096;
const MAX_PENDING_SERIES: usize = 40;
const MAX_PENDING_WINDOWS: usize = 2;
const MAX_BATCH_ATTRIBUTES_BYTES: usize = 8_000;

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

/// Logs a validated, content-free receipt before acknowledging it. The response
/// describes runtime logging only, never external delivery or durable readback.
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
    // Account, cookie, CSRF, source, answer and feedback never enter the log.
    // Only the explicitly reconstructed allowlist above is emitted.
    worker::console_log!("{}", attributes);
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
    let mut response = Response::empty()?.with_status(202);
    response
        .headers_mut()
        .set("x-scry-telemetry-delivery", "runtime_logged")?;
    response
        .headers_mut()
        .set("x-scry-telemetry-attempts", "0")?;
    auth::refresh_browser_cookie(req, &account, &mut response)?;
    Ok(Some(web::no_store(response)?))
}

#[derive(Default, Serialize)]
struct Accounting {
    batches_dropped: u64,
    observations_dropped: u64,
    observations_invalid: u64,
    series_dropped: u64,
}

impl Accounting {
    fn is_empty(&self) -> bool {
        self.batches_dropped == 0
            && self.observations_dropped == 0
            && self.observations_invalid == 0
            && self.series_dropped == 0
    }
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
}

impl Aggregate {
    fn new() -> Self {
        Self {
            boot: Boot::new(u64::try_from(now_ms()).unwrap_or_default()),
            pending: VecDeque::new(),
            accounting: std::array::from_fn(|_| Accounting::default()),
            export_minute: current_minute(),
            exports: 0,
        }
    }

    fn record(&mut self, observation: Observation) {
        let minute = current_minute();
        let namespace = observation.marker().namespace();
        while self.pending.front().is_some_and(|window| {
            window.minute.saturating_add(MAX_PENDING_WINDOWS as u64) <= minute
        }) {
            if let Some(window) = self.pending.pop_front() {
                let accounting = &mut self.accounting[namespace_index(window.namespace)];
                accounting.batches_dropped = accounting.batches_dropped.saturating_add(1);
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
                        accounting.batches_dropped = accounting.batches_dropped.saturating_add(1);
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

    fn take_batch(&mut self) -> Option<String> {
        let minute = current_minute();
        if self.export_minute != minute {
            self.export_minute = minute;
            self.exports = 0;
        }
        if self.exports >= MAX_AGGREGATE_BATCHES_PER_MINUTE {
            return None;
        }
        let mut window = self.pending.pop_front().or_else(|| {
            // Rejected or evicted observations remain visible even if their
            // namespace produces no further valid observations.
            Namespace::all()
                .iter()
                .copied()
                .find(|namespace| !self.accounting[namespace_index(*namespace)].is_empty())
                .map(|namespace| PendingWindow {
                    minute,
                    namespace,
                    snapshots: Vec::new(),
                })
        })?;
        let mut accounting =
            std::mem::take(&mut self.accounting[namespace_index(window.namespace)]);
        loop {
            let attributes = json!({
                "schema": "memory_engine.performance_batch.v1", "authority": "non_authoritative_debug",
                "namespace": window.namespace, "window": {"start_minute": window.minute},
                "delivery": accounting, "snapshots": window.snapshots,
            });
            let encoded = attributes.to_string();
            if encoded.len() <= MAX_BATCH_ATTRIBUTES_BYTES {
                self.exports += 1;
                return Some(encoded);
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

/// Emits at most three bounded aggregate records per minute. There is no
/// asynchronous work or external transport to coordinate.
pub fn flush() {
    while let Some(batch) = aggregate(Aggregate::take_batch) {
        worker::console_log!("{}", batch);
    }
}

/// Records the observed actor/recovery health, not an external monitor check-in.
pub fn report_health(healthy: bool, backup_age_ms: Option<i64>) {
    worker::console_log!(
        "{}",
        json!({
            "schema": "memory_engine.worker_health.v1",
            "status": if healthy { "healthy" } else { "degraded" },
            "backupAgeMs": backup_age_ms,
        })
    );
}
