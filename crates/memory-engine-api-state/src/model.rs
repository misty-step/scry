//! Runtime-neutral API data shared by native and Worker adapters and rendering.
//!
//! The crate root remains the public API. These types own the wire contract;
//! storage, transport, scheduling, and error reporting belong to each runtime.

use http::StatusCode;
pub use memory_engine_persistence::SourcePermission;
use memory_engine_service::ContentFeedbackVerdict;
use memory_engine_study::{
    BetaStudyConceptProgress, BetaStudyCurrent, BetaStudyDraftRow, BetaStudySummary, BetaStudyView,
    LibrarySourceRow,
};
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ReturnNotificationPreference {
    pub email: String,
    pub enabled: bool,
    pub last_sent_at_ms: Option<i64>,
    #[serde(default)]
    pub unsubscribe_nonce: String,
    #[serde(default)]
    pub claim_id: Option<String>,
    #[serde(default)]
    pub claim_expires_at_ms: Option<i64>,
    #[serde(default)]
    pub pending_delivery_key: Option<String>,
    #[serde(default)]
    pub pending_due_count: Option<usize>,
    #[serde(default)]
    pub pending_unsubscribe_expires_at_ms: Option<i64>,
    #[serde(default)]
    pub retry_attempts: u32,
    #[serde(default)]
    pub next_retry_at_ms: Option<i64>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ScheduledReturnNotificationReport {
    pub examined: usize,
    pub due: usize,
    pub sent: usize,
    pub skipped: usize,
    pub failed: usize,
    pub truncated: bool,
    pub started_at_ms: i64,
    pub finished_at_ms: i64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SchedulerHealth {
    pub enabled: bool,
    pub running: bool,
    pub last_run_at_ms: Option<i64>,
    pub last_success_at_ms: Option<i64>,
    pub failure_count: u64,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct MagicLinkRequest {
    pub debug_link: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct CreateAccountRequest {
    pub email: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AccountCreated {
    pub account_id: String,
    pub session_token: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct CreateSourceRequest {
    #[serde(default)]
    pub title: String,
    pub body: String,
    #[serde(default = "default_source_permission")]
    pub permission: SourcePermission,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SourceRecord {
    pub source_id: String,
    pub title: String,
    pub body: String,
    pub permission: SourcePermission,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub project_key: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ttl_expires_at: Option<i64>,
}

fn default_source_permission() -> SourcePermission {
    SourcePermission::ModelEligible
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct CreateProjectDeckRequest {
    pub project_key: String,
    pub title: String,
    pub body: String,
    pub ttl_expires_at: Option<i64>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct InvalidateProjectDeckRequest {
    pub event: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProjectDeckRecord {
    pub deck_id: String,
    pub project_key: String,
    pub source: SourceRecord,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SourceList {
    pub sources: Vec<SourceRecord>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct SubmitReviewRequest {
    pub answer: String,
    pub response_time_ms: u32,
    pub idempotency_key: String,
}

/// Blocking Postgres phases observed for one browser review submission.
///
/// `None` means the phase did not run. Callers must not coerce an absent
/// phase to zero because auth, validation, and connection failures stop at
/// different boundaries.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct SubmitReviewTimings {
    connect_ms: Option<u64>,
    connect_count: Option<u64>,
    operation_ms: Option<u64>,
    statement_count: Option<u64>,
}

impl SubmitReviewTimings {
    #[must_use]
    pub const fn postgres_connect_ms(self) -> Option<u64> {
        self.connect_ms
    }
    #[must_use]
    pub const fn postgres_connect_count(self) -> Option<u64> {
        self.connect_count
    }

    #[must_use]
    pub const fn postgres_operation_ms(self) -> Option<u64> {
        self.operation_ms
    }

    #[must_use]
    pub const fn postgres_statement_count(self) -> Option<u64> {
        self.statement_count
    }

    #[cfg(feature = "native")]
    pub(crate) fn record_postgres_connect(&mut self, duration_ms: u64) {
        self.connect_ms = Some(
            self.connect_ms
                .unwrap_or_default()
                .saturating_add(duration_ms),
        );
        self.connect_count = Some(self.connect_count.unwrap_or_default().saturating_add(1));
    }

    #[cfg(feature = "native")]
    pub(crate) fn record_postgres_operation(&mut self, duration_ms: u64) {
        self.operation_ms = Some(
            self.operation_ms
                .unwrap_or_default()
                .saturating_add(duration_ms),
        );
    }

    #[cfg(feature = "native")]
    pub(crate) fn record_postgres_statement_count(&mut self, count: u64) {
        self.statement_count = Some(
            self.statement_count
                .unwrap_or_default()
                .saturating_add(count),
        );
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum SubmitPerformanceOutcome {
    Succeeded,
    ClientRejected,
    ServerFailed,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum SubmitViewport {
    Mobile,
    Tablet,
    Desktop,
}

/// A correlated browser completion. Only observed phases are present; DOM
/// swaps and full-page navigation remain distinct in receipts and aggregates.
#[derive(Clone, Copy, Debug)]
pub struct BrowserSubmitReceipt<'a> {
    pub request_id: &'a str,
    pub trace_id: &'a str,
    pub navigation: memory_engine_performance::Navigation,
    pub tap_to_ack_ms: u64,
    pub request_to_response_ms: Option<u64>,
    pub transfer_ms: Option<u64>,
    pub navigation_ms: Option<u64>,
    pub dom_swap_ms: Option<u64>,
    pub graded_visible_ms: u64,
    pub viewport: SubmitViewport,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct ContentFeedbackRequest {
    pub verdict: ContentFeedbackVerdict,
    pub rationale: Option<String>,
    pub idempotency_key: String,
    pub supersedes_id: Option<String>,
}

#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StudyViewResponse {
    pub drafts: Vec<BetaStudyDraftRow>,
    /// Active review inventory, separate from historical generated drafts.
    /// Older saved graded receipts predate this projection.
    #[serde(default)]
    pub queue: Vec<memory_engine_study::BetaStudyQueueRow>,
    pub current: Option<BetaStudyCurrent>,
    pub concept_progress: Vec<BetaStudyConceptProgress>,
    pub summary: BetaStudySummary,
    pub due_count: usize,
    #[serde(default)]
    pub generation_notices: Vec<String>,
    /// Per-source active-card inventory for the Library view
    /// (memory-engine-087).
    #[serde(default)]
    pub library: Vec<LibrarySourceRow>,
}

impl StudyViewResponse {
    #[must_use]
    pub fn from_view(view: BetaStudyView) -> Self {
        Self {
            drafts: view.drafts,
            queue: view.queue,
            current: view.current,
            concept_progress: view.concept_progress,
            summary: view.summary,
            due_count: view.due_count,
            generation_notices: view.generation_notices,
            library: view.library,
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HealthResponse {
    pub status: &'static str,
    pub service: &'static str,
    pub return_notification_scheduler: SchedulerHealth,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ReadinessResponse {
    pub status: &'static str,
    pub service: &'static str,
    pub worker_started: bool,
    pub postgres: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ApiError {
    pub error: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AppAccount {
    pub(crate) browser_session_id: String,
    pub(crate) account_id: String,
    pub(crate) session_token: String,
    pub(crate) csrf_token: String,
    /// Absolute browser-session expiry in epoch milliseconds.
    pub(crate) expires_at_ms: i64,
    /// Remaining cookie lifetime in seconds, computed with the same clock that
    /// authored `expires_at_ms` so Set-Cookie Max-Age never drifts ahead of the
    /// server row.
    pub(crate) cookie_max_age_seconds: u64,
}

impl AppAccount {
    /// Construct render data from a session already authenticated by its runtime.
    ///
    /// This does not mint or validate credentials. The caller supplies the
    /// durable session's expiry and the cookie lifetime derived from that same
    /// clock; no native storage or runtime is consulted.
    #[must_use]
    pub fn from_session(
        browser_session_id: String,
        account_id: String,
        session_token: String,
        csrf_token: String,
        expires_at_ms: i64,
        cookie_max_age_seconds: u64,
    ) -> Self {
        Self {
            browser_session_id,
            account_id,
            session_token,
            csrf_token,
            expires_at_ms,
            cookie_max_age_seconds,
        }
    }

    #[must_use]
    pub fn browser_session_id(&self) -> &str {
        &self.browser_session_id
    }

    #[must_use]
    pub fn account_id(&self) -> &str {
        &self.account_id
    }

    #[must_use]
    pub fn session_token(&self) -> &str {
        &self.session_token
    }

    #[must_use]
    pub fn csrf_token(&self) -> &str {
        &self.csrf_token
    }

    #[must_use]
    pub fn expires_at_ms(&self) -> i64 {
        self.expires_at_ms
    }

    #[must_use]
    pub fn cookie_max_age_seconds(&self) -> u64 {
        self.cookie_max_age_seconds
    }
}

#[derive(Debug)]
pub struct ApiFailure {
    pub(crate) status: StatusCode,
    pub message: String,
}

impl ApiFailure {
    #[must_use]
    pub fn status(&self) -> StatusCode {
        self.status
    }

    #[must_use]
    pub fn bad_request(message: &'static str) -> Self {
        Self {
            status: StatusCode::BAD_REQUEST,
            message: message.to_owned(),
        }
    }

    #[must_use]
    pub fn unknown_account() -> Self {
        Self {
            status: StatusCode::NOT_FOUND,
            message: "Account not found.".to_owned(),
        }
    }

    #[must_use]
    pub fn not_found(message: &'static str) -> Self {
        Self {
            status: StatusCode::NOT_FOUND,
            message: message.to_owned(),
        }
    }

    #[must_use]
    pub fn conflict(message: &'static str) -> Self {
        Self {
            status: StatusCode::CONFLICT,
            message: message.to_owned(),
        }
    }

    #[must_use]
    pub fn conflict_message(message: String) -> Self {
        Self {
            status: StatusCode::CONFLICT,
            message,
        }
    }

    #[must_use]
    pub fn missing_session() -> Self {
        Self {
            status: StatusCode::UNAUTHORIZED,
            message: "Session token is required.".to_owned(),
        }
    }

    #[must_use]
    pub fn forbidden_account() -> Self {
        Self {
            status: StatusCode::FORBIDDEN,
            message: "Session token does not match account.".to_owned(),
        }
    }

    #[must_use]
    pub fn forbidden(message: &'static str) -> Self {
        Self {
            status: StatusCode::FORBIDDEN,
            message: message.to_owned(),
        }
    }

    #[must_use]
    pub fn too_many_requests(message: &'static str) -> Self {
        Self {
            status: StatusCode::TOO_MANY_REQUESTS,
            message: message.to_owned(),
        }
    }

    #[must_use]
    pub fn service_unavailable(message: String) -> Self {
        Self {
            status: StatusCode::SERVICE_UNAVAILABLE,
            message,
        }
    }

    #[must_use]
    pub fn internal(message: String) -> Self {
        #[cfg(feature = "native")]
        crate::native::report_internal_error(&message);
        Self {
            status: StatusCode::INTERNAL_SERVER_ERROR,
            message,
        }
    }

    #[must_use]
    pub fn payload_too_large(message: &'static str) -> Self {
        Self {
            status: StatusCode::PAYLOAD_TOO_LARGE,
            message: message.to_owned(),
        }
    }

    #[must_use]
    pub fn is_session_expired(&self) -> bool {
        self.status == StatusCode::UNAUTHORIZED
    }

    #[must_use]
    pub fn is_magic_link_recovery(&self) -> bool {
        self.status == StatusCode::FORBIDDEN && self.message == "Magic link is invalid or expired."
    }

    /// True when this failure came from validating or acting on a
    /// return-notification (due-count reminder) unsubscribe token: signature
    /// mismatch, malformed payload, unknown/rotated nonce, or a token whose
    /// `expires_at_ms` has passed. Runtime unsubscribe handlers use
    /// [`ApiFailure::forbidden`] with a message that starts with this exact
    /// prefix, so it is a safe, precise discriminator distinct from every
    /// other `Forbidden` failure in the app surface (e.g.
    /// `forbidden_account`).
    #[must_use]
    pub fn is_return_notification_link_invalid(&self) -> bool {
        self.status == StatusCode::FORBIDDEN && self.message.starts_with("That unsubscribe link")
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum JobStatus {
    Queued,
    Running,
    Retry,
    Succeeded,
    Failed,
}

impl JobStatus {
    #[must_use]
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Queued => "queued",
            Self::Running => "running",
            Self::Retry => "retry",
            Self::Succeeded => "succeeded",
            Self::Failed => "failed",
        }
    }

    /// Succeeded/failed jobs no longer change on their own, so they are the
    /// prunable history (see `MAX_TERMINAL_JOBS_PER_ACCOUNT`). This also governs
    /// crash-restore: a *non*-terminal job is reset to a retryable failure on
    /// restart, since no worker owns it after the restart (see `load_jobs`).
    #[must_use]
    pub fn is_terminal(self) -> bool {
        matches!(self, Self::Succeeded | Self::Failed)
    }
}

/// One generation job. The account/source ids drive the worker; the rest is
/// learner-facing status surfaced in the activity log and over SSE.
#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GenerationJob {
    pub id: String,
    /// Authorization + store routing for the worker; never serialized to the UI.
    #[serde(skip)]
    pub account_id: String,
    #[serde(skip)]
    pub source_id: String,
    pub title: String,
    pub status: JobStatus,
    /// Number of scheduled review cards created by this job. Trust-gated
    /// generation leaves this at zero until a learner keeps or edits a draft.
    pub card_count: usize,
    pub attempts: u32,
    /// False once the bounded attempt budget is exhausted. This is sent over
    /// SSE so the browser never advertises a retry that the API must reject.
    pub retryable: bool,
    pub error: Option<String>,
    pub created_at: i64,
    pub updated_at: i64,
    #[serde(skip)]
    pub retry_at: Option<i64>,
    #[serde(skip)]
    pub lease_expires_at: Option<i64>,
}

/// One waitlist row: normalized email, an audit trail of when it joined and
/// last changed, the first-run surface it came from, and whether an operator
/// has since transitioned it to invited. No account, session, or generation
/// state is ever attached to this record.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WaitlistEntry {
    pub email: String,
    pub created_at_ms: i64,
    pub updated_at_ms: i64,
    /// Where the join happened, e.g. `"first-run"`. Submitting the form is
    /// the consent action; this field is the source half of the
    /// "consent/source metadata" the card asks for.
    pub source: String,
    pub invited_at_ms: Option<i64>,
}

#[must_use]
pub fn normalize_email(email: &str) -> Option<String> {
    let trimmed = email.trim().to_ascii_lowercase();
    let (local, domain) = trimmed.split_once('@')?;
    if local.is_empty()
        || domain.is_empty()
        || domain.contains('@')
        || !domain.split('.').all(|part| !part.is_empty())
        || !domain.contains('.')
    {
        return None;
    }

    Some(trimmed)
}

pub fn csrf_token(value: Option<&String>) -> &str {
    value.map(String::as_str).map(str::trim).unwrap_or_default()
}

pub const APP_SESSION_COOKIE_NAME: &str = "__Host-memory_engine_session";
pub const APP_SESSION_INSECURE_COOKIE_NAME: &str = "memory_engine_session";
pub const APP_SESSION_MAX_AGE_SECONDS: u64 = 60 * 60 * 24 * 90;
pub const APP_ACCOUNT_RATE_LIMIT_MAX_ATTEMPTS: u32 = 5;
pub const MAX_SOURCE_BODY_BYTES: usize = 256 * 1024;
pub const MAX_SOURCE_TITLE_BYTES: usize = 512;
/// Same policy shape as the magic-link request limiter: five attempts per
/// window, keyed by normalized email and by client IP.
pub const WAITLIST_RATE_LIMIT_MAX_ATTEMPTS: u32 = 5;
// 30 minutes: links travel through email, where spam checks and device
// switches routinely burn ten minutes. Found in dogfood: a link expired
// before the operator could click it.
pub const AUTH_CHALLENGE_TTL_MS: i64 = 30 * 60 * 1_000;
/// At most one due-count reminder per account per day, apart from the
/// one-time confirmation sent immediately after an explicit opt-in.
pub const RETURN_NOTIFICATION_INTERVAL_MS: i64 = 24 * 60 * 60 * 1_000;
pub const RETURN_NOTIFICATION_UNSUBSCRIBE_TTL_MS: i64 = 7 * 24 * 60 * 60 * 1_000;

#[must_use]
pub fn app_session_max_age_ms() -> i64 {
    i64::try_from(APP_SESSION_MAX_AGE_SECONDS)
        .unwrap_or(i64::MAX)
        .saturating_mul(1_000)
}
