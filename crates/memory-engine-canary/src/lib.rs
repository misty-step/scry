//! Bounded Canary observability adapter for memory-engine.
//!
//! Errors and closed performance aggregates share one bounded worker queue.
//! Request paths never perform Canary I/O. Error events go to
//! `POST /api/v1/errors`; performance batches go to `POST /api/v1/events`
//! and are mirrored to stdout as explicitly non-authoritative debug evidence.
//!
//! Admission, drain, and delivery are different boundaries: reporting only
//! admits work locally; a successful flush/shutdown returns cumulative observed
//! delivery, including losses. Only HTTP 2xx at the ingest endpoint is acceptance,
//! not proof of durable storage or timeline readback. Redirects are not followed.
//! Transport failures, timeouts, HTTP 408/429 and 5xx get one immediate retry;
//! other rejections are final. Each attempt is bounded to two seconds. An unknown
//! response can therefore cause a duplicate retry; this is not exactly-once I/O.

mod performance;

use std::{
    fmt,
    sync::{
        atomic::{AtomicBool, AtomicU64, Ordering},
        mpsc::{self, Receiver, RecvTimeoutError, SyncSender, TrySendError},
        Arc, Mutex,
    },
    thread,
    time::{Duration, Instant},
};

use memory_engine_performance::{Namespace, Observation};

use performance::{current_minute, namespace_index, Aggregator};
pub use performance::{
    read_performance_timeline, DeliveryAccounting, PerformanceBatch, PerformanceError,
    PerformanceReadback, ReadbackConfig, DEBUG_AUTHORITY, PERFORMANCE_BATCH_SCHEMA,
    PERFORMANCE_EVENT_NAME,
};

/// Environment variable naming the Canary base endpoint.
pub const ENDPOINT_ENV: &str = "CANARY_ENDPOINT";
/// Environment variable holding the ingest API key.
pub const API_KEY_ENV: &str = "CANARY_API_KEY";
const MAX_QUEUE_DEPTH: usize = 128;
const REQUEST_TIMEOUT: Duration = Duration::from_secs(2);
const RETRY_ATTEMPTS: usize = 2;
const WORKER_IDLE_POLL: Duration = Duration::from_millis(100);

/// Reporter configuration, normally loaded from the environment.
#[derive(Clone, Debug)]
pub struct CanaryConfig {
    /// Base URL, for example `https://canary.example.com`.
    pub endpoint: String,
    /// Ingest-scoped Canary API key.
    pub api_key: String,
    /// Stable service name.
    pub service: String,
    /// Deployment environment label.
    pub environment: String,
}

impl CanaryConfig {
    /// Build a configuration. Missing endpoint or key disables reporting.
    #[must_use]
    pub fn from_parts(endpoint: Option<String>, api_key: Option<String>) -> Option<Self> {
        let endpoint = endpoint?.trim_end_matches('/').to_owned();
        let api_key = api_key?;
        if endpoint.is_empty() || api_key.is_empty() {
            return None;
        }
        Some(Self {
            endpoint,
            api_key,
            service: "memory-engine-api".to_owned(),
            environment: "production".to_owned(),
        })
    }

    /// Load `CANARY_ENDPOINT` and `CANARY_API_KEY` from the process environment.
    #[must_use]
    pub fn from_env() -> Option<Self> {
        Self::from_parts(
            std::env::var(ENDPOINT_ENV).ok(),
            std::env::var(API_KEY_ENV).ok(),
        )
    }
}

/// Error severity accepted by Canary.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum Severity {
    /// Non-fatal informational event.
    Info,
    /// Degraded behavior that needs attention.
    Warning,
    /// Failed behavior.
    Error,
}

impl Severity {
    const fn as_str(self) -> &'static str {
        match self {
            Self::Info => "info",
            Self::Warning => "warning",
            Self::Error => "error",
        }
    }
}

/// Structured error event submitted to Canary.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ErrorEvent {
    /// Stable, bounded error classification.
    pub error_class: String,
    /// Human-readable error message.
    pub message: String,
    /// Event severity.
    pub severity: Severity,
    /// Optional structured context. Callers must not include secrets or learner content.
    pub context: Option<serde_json::Value>,
    /// Stable fingerprint components used by Canary grouping.
    pub fingerprint: Vec<String>,
}

/// A cheap process-liveness observation sent through Canary's check-in route.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CheckInEvent {
    /// Stable monitor slug registered in Canary.
    pub monitor: String,
    /// Canary check-in status.
    pub status: CheckInStatus,
    /// Short bounded summary.
    pub summary: String,
    /// Monitor expiry requested from Canary.
    pub ttl_ms: u64,
    /// Optional structured, content-free monitor context.
    pub context: Option<serde_json::Value>,
}

/// Canary check-in state.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum CheckInStatus {
    Alive,
    Started,
    Completed,
    Failed,
}

impl CheckInStatus {
    const fn as_str(self) -> &'static str {
        match self {
            Self::Alive => "alive",
            Self::Started => "started",
            Self::Completed => "completed",
            Self::Failed => "failed",
        }
    }
}

/// Observed transport outcome for all work covered by a completed drain.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum DeliveryOutcome {
    /// No ingest request was accepted and no loss was observed.
    Empty,
    /// At least one ingest request received HTTP 2xx, with no observed loss.
    Accepted,
    /// At least one request or performance observation was lost.
    Dropped,
}

/// Content-free reason the final ingest attempt was not accepted.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum DeliveryFailure {
    /// The ingest endpoint returned a non-2xx status.
    Rejected(u16),
    /// The per-attempt network deadline expired; acceptance is unknown.
    TimedOut,
    /// No successful HTTP response was observed; acceptance is unknown.
    Transport,
}

impl DeliveryFailure {
    const fn retryable(self) -> bool {
        match self {
            Self::Rejected(status) => matches!(status, 408 | 429 | 500..=599),
            Self::TimedOut | Self::Transport => true,
        }
    }
}

impl fmt::Display for DeliveryFailure {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Rejected(status) => write!(formatter, "ingest rejected HTTP {status}"),
            Self::TimedOut => formatter.write_str("ingest attempt timed out"),
            Self::Transport => formatter.write_str("ingest transport failed"),
        }
    }
}

/// Cumulative evidence from reporter creation through a completed drain.
///
/// Counts never reset at a flush, so a later acceptance cannot erase earlier
/// loss. Only performance observations carry observation counts; error events
/// and check-ins contribute request counts after worker admission. Queue
/// admission alone is never counted as acceptance.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct DeliveryReport {
    requests_accepted: u64,
    requests_dropped: u64,
    retries: u64,
    observations_accepted: u64,
    observations_dropped: u64,
    last_failure: Option<DeliveryFailure>,
}

impl DeliveryReport {
    #[must_use]
    pub const fn outcome(&self) -> DeliveryOutcome {
        if self.requests_dropped > 0 || self.observations_dropped > 0 {
            DeliveryOutcome::Dropped
        } else if self.requests_accepted > 0 {
            DeliveryOutcome::Accepted
        } else {
            DeliveryOutcome::Empty
        }
    }

    #[must_use]
    pub const fn requests_accepted(&self) -> u64 {
        self.requests_accepted
    }

    #[must_use]
    pub const fn requests_dropped(&self) -> u64 {
        self.requests_dropped
    }

    #[must_use]
    pub const fn retries(&self) -> u64 {
        self.retries
    }

    #[must_use]
    pub const fn observations_accepted(&self) -> u64 {
        self.observations_accepted
    }

    /// Includes queue rejection, aggregate validation/bounds and failed batches.
    #[must_use]
    pub const fn observations_dropped(&self) -> u64 {
        self.observations_dropped
    }

    #[must_use]
    pub const fn last_failure(&self) -> Option<DeliveryFailure> {
        self.last_failure
    }

    fn note_request(&mut self, delivery: RequestDelivery) {
        self.retries = self
            .retries
            .saturating_add(delivery.attempts.saturating_sub(1) as u64);
        match delivery.result {
            Ok(()) => self.requests_accepted = self.requests_accepted.saturating_add(1),
            Err(failure) => {
                self.requests_dropped = self.requests_dropped.saturating_add(1);
                self.last_failure = Some(failure);
            }
        }
    }
}

/// The requested drain boundary was not observed.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum DrainError {
    /// Work may still complete in the background; this is not a delivery result.
    DeadlineExceeded,
    /// Flush was requested after shutdown closed admission.
    Closed,
    /// The worker could not start, disconnected, or terminated unexpectedly.
    WorkerUnavailable,
}

impl fmt::Display for DrainError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(match self {
            Self::DeadlineExceeded => "Canary drain deadline exceeded; delivery is unconfirmed",
            Self::Closed => "Canary reporter is closed",
            Self::WorkerUnavailable => "Canary reporter worker is unavailable",
        })
    }
}

impl std::error::Error for DrainError {}

#[derive(Clone, Copy)]
struct RequestDelivery {
    result: Result<(), DeliveryFailure>,
    attempts: usize,
}

#[derive(Clone, Debug)]
enum Command {
    Error(ErrorEvent),
    CheckIn(CheckInEvent),
    Performance(Observation),
    Flush(mpsc::Sender<DeliveryReport>),
    Shutdown,
}

struct ReporterInner {
    sender: SyncSender<Command>,
    performance_drops: Arc<[AtomicU64; 3]>,
    closed: AtomicBool,
    worker: Mutex<WorkerState>,
}

struct WorkerState {
    handle: Option<thread::JoinHandle<DeliveryReport>>,
    shutdown_queued: bool,
    completed: Option<Result<DeliveryReport, DrainError>>,
}

/// Non-blocking reporter backed by one bounded process worker.
#[derive(Clone)]
pub struct CanaryReporter {
    inner: Arc<ReporterInner>,
}

impl CanaryReporter {
    /// Construct a reporter and its single bounded worker.
    #[must_use]
    pub fn new(config: CanaryConfig) -> Self {
        let (sender, receiver) = mpsc::sync_channel(MAX_QUEUE_DEPTH);
        let performance_drops = Arc::new(std::array::from_fn(|_| AtomicU64::new(0)));
        let worker_drops = Arc::clone(&performance_drops);
        let worker = thread::Builder::new()
            .name("memory-engine-canary".to_owned())
            .spawn(move || worker_loop(&config, &receiver, worker_drops.as_ref()))
            .ok();
        Self {
            inner: Arc::new(ReporterInner {
                sender,
                performance_drops,
                closed: AtomicBool::new(false),
                worker: Mutex::new(WorkerState {
                    completed: worker
                        .is_none()
                        .then_some(Err(DrainError::WorkerUnavailable)),
                    handle: worker,
                    shutdown_queued: false,
                }),
            }),
        }
    }

    /// Queue an error without blocking the caller. Saturated or closed queues
    /// drop the event rather than affecting application behavior.
    pub fn report(&self, event: &ErrorEvent) {
        self.try_send(Command::Error(event.clone()), None);
    }

    /// Queue a check-in without blocking the caller.
    pub fn check_in(&self, event: &CheckInEvent) {
        self.try_send(Command::CheckIn(event.clone()), None);
    }

    /// Queue one validated performance observation without blocking the caller.
    ///
    /// Returns `true` for local queue admission only, never backend acceptance.
    /// A `false` result is counted as a drop in the observation's trusted
    /// namespace. Shutdown closes admission; producers should stop beforehand.
    #[must_use]
    pub fn report_performance(&self, observation: Observation) -> bool {
        let namespace = observation.marker().namespace();
        self.try_send(Command::Performance(observation), Some(namespace))
    }

    /// Drain queued work and close the active minute, bounded by `deadline`.
    ///
    /// `Ok(report)` means work ahead of this barrier settled, including failed
    /// delivery. Inspect [`DeliveryReport::outcome`] for transport acceptance.
    /// This is an operator boundary, not a request-path operation. A timed-out
    /// flush may still execute; it does not cancel queued work or network I/O.
    ///
    /// # Errors
    ///
    /// Returns [`DrainError`] if the barrier cannot be observed before the
    /// deadline, admission is closed, or the worker is unavailable.
    pub fn flush(&self, deadline: Duration) -> Result<DeliveryReport, DrainError> {
        if self.inner.closed.load(Ordering::Acquire) {
            return Err(DrainError::Closed);
        }
        let started = Instant::now();
        let (acknowledge, received) = mpsc::channel();
        send_control_until(
            &self.inner.sender,
            Command::Flush(acknowledge),
            started,
            deadline,
        )?;
        let remaining = deadline.saturating_sub(started.elapsed());
        received
            .recv_timeout(remaining)
            .map_err(|error| match error {
                RecvTimeoutError::Timeout => DrainError::DeadlineExceeded,
                RecvTimeoutError::Disconnected => DrainError::WorkerUnavailable,
            })
    }

    /// Close admission, drain queued work, and wait for the worker to stop.
    ///
    /// The deadline bounds this caller's wait, not cancellation of a running
    /// request. On timeout the worker may continue; another shutdown call can
    /// finish enqueueing shutdown or retrieve the same final cumulative report.
    /// A completed drain is not necessarily successful delivery. Quiesce
    /// producers before shutdown to put all admitted work ahead of the barrier.
    ///
    /// # Errors
    ///
    /// Returns [`DrainError::DeadlineExceeded`] if shutdown is not complete in
    /// time, or [`DrainError::WorkerUnavailable`] for worker failure.
    pub fn shutdown(&self, deadline: Duration) -> Result<DeliveryReport, DrainError> {
        let started = Instant::now();
        self.inner.closed.store(true, Ordering::Release);
        loop {
            {
                let mut state = self
                    .inner
                    .worker
                    .lock()
                    .map_err(|_| DrainError::WorkerUnavailable)?;
                if let Some(completed) = state.completed {
                    return completed;
                }
                if state
                    .handle
                    .as_ref()
                    .is_some_and(thread::JoinHandle::is_finished)
                {
                    let completed = state
                        .handle
                        .take()
                        .ok_or(DrainError::WorkerUnavailable)?
                        .join()
                        .map_err(|_| DrainError::WorkerUnavailable);
                    state.completed = Some(completed);
                    return completed;
                }
                if !state.shutdown_queued {
                    match self.inner.sender.try_send(Command::Shutdown) {
                        Ok(()) => state.shutdown_queued = true,
                        Err(TrySendError::Full(_)) => {}
                        Err(TrySendError::Disconnected(_)) => {
                            state.completed = Some(Err(DrainError::WorkerUnavailable));
                            return Err(DrainError::WorkerUnavailable);
                        }
                    }
                }
            }
            wait_for_control(started, deadline)?;
        }
    }

    fn try_send(&self, command: Command, namespace: Option<Namespace>) -> bool {
        if self.inner.closed.load(Ordering::Acquire) {
            self.note_performance_drop(namespace);
            return false;
        }
        match self.inner.sender.try_send(command) {
            Ok(()) => true,
            Err(TrySendError::Full(_) | TrySendError::Disconnected(_)) => {
                self.note_performance_drop(namespace);
                false
            }
        }
    }

    fn note_performance_drop(&self, namespace: Option<Namespace>) {
        if let Some(namespace) = namespace {
            self.inner.performance_drops[namespace_index(namespace)]
                .fetch_add(1, Ordering::Relaxed);
        }
    }
}

fn send_control_until(
    sender: &SyncSender<Command>,
    mut command: Command,
    started: Instant,
    deadline: Duration,
) -> Result<(), DrainError> {
    loop {
        match sender.try_send(command) {
            Ok(()) => return Ok(()),
            Err(TrySendError::Full(returned)) => {
                command = returned;
                wait_for_control(started, deadline)?;
            }
            Err(TrySendError::Disconnected(_)) => return Err(DrainError::WorkerUnavailable),
        }
    }
}

fn wait_for_control(started: Instant, deadline: Duration) -> Result<(), DrainError> {
    let remaining = deadline.saturating_sub(started.elapsed());
    if remaining.is_zero() {
        return Err(DrainError::DeadlineExceeded);
    }
    thread::sleep(remaining.min(Duration::from_millis(5)));
    Ok(())
}

fn worker_loop(
    config: &CanaryConfig,
    receiver: &Receiver<Command>,
    performance_drops: &[AtomicU64; 3],
) -> DeliveryReport {
    let agent: ureq::Agent = ureq::Agent::config_builder()
        .timeout_global(Some(REQUEST_TIMEOUT))
        .http_status_as_error(false)
        .max_redirects(0)
        .build()
        .into();
    let mut aggregator = Aggregator::new();
    let mut delivery = DeliveryReport::default();
    loop {
        match receiver.recv_timeout(WORKER_IDLE_POLL) {
            Ok(Command::Error(event)) => delivery.note_request(send_error(&agent, config, &event)),
            Ok(Command::CheckIn(event)) => {
                delivery.note_request(send_check_in(&agent, config, &event));
            }
            Ok(Command::Performance(observation)) => {
                let minute = current_minute();
                if aggregator
                    .active_minute()
                    .is_some_and(|active| active != minute)
                {
                    flush_performance(
                        &agent,
                        config,
                        &mut aggregator,
                        performance_drops,
                        &mut delivery,
                    );
                }
                aggregator.record(minute, observation);
            }
            Ok(Command::Flush(acknowledge)) => {
                flush_performance(
                    &agent,
                    config,
                    &mut aggregator,
                    performance_drops,
                    &mut delivery,
                );
                let _ = acknowledge.send(delivery);
            }
            Ok(Command::Shutdown) | Err(RecvTimeoutError::Disconnected) => {
                flush_performance(
                    &agent,
                    config,
                    &mut aggregator,
                    performance_drops,
                    &mut delivery,
                );
                return delivery;
            }
            Err(RecvTimeoutError::Timeout) => {
                if aggregator
                    .active_minute()
                    .is_some_and(|active| active != current_minute())
                {
                    flush_performance(
                        &agent,
                        config,
                        &mut aggregator,
                        performance_drops,
                        &mut delivery,
                    );
                }
            }
        }
    }
}

fn send_error(agent: &ureq::Agent, config: &CanaryConfig, event: &ErrorEvent) -> RequestDelivery {
    let body = serde_json::json!({
        "service": config.service,
        "environment": config.environment,
        "error_class": event.error_class,
        "message": event.message,
        "severity": event.severity.as_str(),
        "context": event.context,
        "fingerprint": event.fingerprint,
    });
    let url = format!("{}/api/v1/errors", config.endpoint);
    let authorization = format!("Bearer {}", config.api_key);
    send_json_with_retry(agent, &url, &authorization, &body)
}

fn send_check_in(
    agent: &ureq::Agent,
    config: &CanaryConfig,
    event: &CheckInEvent,
) -> RequestDelivery {
    let body = serde_json::json!({
        "monitor": event.monitor,
        "status": event.status.as_str(),
        "summary": event.summary,
        "ttl_ms": event.ttl_ms,
        "context": event.context,
    });
    let url = format!("{}/api/v1/check-ins", config.endpoint);
    let authorization = format!("Bearer {}", config.api_key);
    send_json_with_retry(agent, &url, &authorization, &body)
}

fn flush_performance(
    agent: &ureq::Agent,
    config: &CanaryConfig,
    aggregator: &mut Aggregator,
    performance_drops: &[AtomicU64; 3],
    delivery: &mut DeliveryReport,
) {
    for namespace in Namespace::all() {
        let dropped = performance_drops[namespace_index(*namespace)].swap(0, Ordering::AcqRel);
        aggregator.note_queue_drop(*namespace, dropped);
    }
    for batch in aggregator.take_batches() {
        let (request, body) = send_performance_with_retry(agent, config, &batch);
        println!("{body}");
        delivery.note_request(request);
        let observations = batch.snapshots().iter().fold(0_u64, |count, snapshot| {
            count.saturating_add(snapshot.count())
        });
        if request.result.is_ok() {
            delivery.observations_accepted =
                delivery.observations_accepted.saturating_add(observations);
        } else {
            aggregator.note_batch_drop(
                batch.namespace(),
                request.attempts.saturating_sub(1) as u64,
                observations,
            );
        }
    }
    delivery.observations_dropped = aggregator.observations_dropped();
}

fn send_performance_with_retry(
    agent: &ureq::Agent,
    config: &CanaryConfig,
    batch: &PerformanceBatch,
) -> (RequestDelivery, serde_json::Value) {
    let url = format!("{}/api/v1/events", config.endpoint);
    let authorization = format!("Bearer {}", config.api_key);
    let mut attempt = 1;
    loop {
        let body = performance_body(config, batch, attempt, true);
        let result = send_json(agent, &url, &authorization, &body);
        if result.is_ok() {
            return (
                RequestDelivery {
                    result,
                    attempts: attempt,
                },
                body,
            );
        }
        if attempt == RETRY_ATTEMPTS || result.is_err_and(|failure| !failure.retryable()) {
            return (
                RequestDelivery {
                    result,
                    attempts: attempt,
                },
                performance_body(config, batch, attempt, false),
            );
        }
        attempt += 1;
    }
}

fn performance_body(
    config: &CanaryConfig,
    batch: &PerformanceBatch,
    attempts: usize,
    sent: bool,
) -> serde_json::Value {
    serde_json::json!({
        "service": config.service,
        "name": PERFORMANCE_EVENT_NAME,
        "summary": "Bounded memory-engine performance aggregate",
        "severity": "info",
        "attributes": batch.attributes_for_attempt(attempts, sent),
        "sampling_policy": "unsampled",
        "retention_class": "standard",
        "privacy_policy": "redacted",
    })
}

fn send_json_with_retry(
    agent: &ureq::Agent,
    url: &str,
    authorization: &str,
    body: &serde_json::Value,
) -> RequestDelivery {
    let mut attempts = 1;
    loop {
        let result = send_json(agent, url, authorization, body);
        if result.is_ok()
            || attempts == RETRY_ATTEMPTS
            || result.is_err_and(|failure| !failure.retryable())
        {
            return RequestDelivery { result, attempts };
        }
        attempts += 1;
    }
}

fn send_json(
    agent: &ureq::Agent,
    url: &str,
    authorization: &str,
    body: &serde_json::Value,
) -> Result<(), DeliveryFailure> {
    match agent
        .post(url)
        .header("Authorization", authorization)
        .send_json(body)
    {
        Ok(response) if response.status().is_success() => Ok(()),
        Ok(response) => Err(DeliveryFailure::Rejected(response.status().as_u16())),
        Err(ureq::Error::Timeout(_)) => Err(DeliveryFailure::TimedOut),
        Err(_) => Err(DeliveryFailure::Transport),
    }
}
