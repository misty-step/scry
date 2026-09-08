//! HTTP client over the deployed `memory-engine-api` v1 contract.
//!
//! A Bearer-token `ureq` client against the same v1 routes
//! `memory-engine-review`'s `ReviewClient` and `memory-engine-contract`'s
//! `ContractClient` use, kept deliberately thin. It adds no new server
//! surface — every method below maps to one existing v1 route. Generation
//! goes through the durable job queue (`POST .../generation-jobs`, `GET
//! .../generation-jobs/{id}`) exclusively: the legacy synchronous `POST
//! .../generate` route is refused outright once a deployment has
//! `MEMORY_ENGINE_POSTGRES_URL` set (`ApiFailure::conflict`, HTTP 409 —
//! `registry.rs::generate_source`), which is every production deployment.

use std::{thread, time::Duration};

use serde::{de::DeserializeOwned, Deserialize, Serialize};
use serde_json::json;

const MAX_RESPONSE_BYTES: u64 = 2 * 1024 * 1024;
const REQUEST_TIMEOUT: Duration = Duration::from_secs(30);
/// Bounded poll for a queued generation job: 40 attempts at 500ms is a 20s
/// ceiling, comfortably above the ~1s-per-poll the production receipt
/// (`docs/qa/103-machine-generation-receipt-2026-07-17.md`) observed for a
/// one-card source, while still failing loudly instead of hanging an agent
/// call forever on a stuck job.
const GENERATION_POLL_MAX_ATTEMPTS: u32 = 40;
const GENERATION_POLL_INTERVAL: Duration = Duration::from_millis(500);

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SourceRecord {
    pub source_id: String,
    pub title: String,
    pub body: String,
    #[serde(default)]
    pub project_key: Option<String>,
    #[serde(default)]
    pub ttl_expires_at: Option<i64>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SourceList {
    pub sources: Vec<SourceRecord>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ProjectDeckRecord {
    pub deck_id: String,
    pub project_key: String,
    pub source: SourceRecord,
}

/// A queued generation job (`GenerationJob` in the `OpenAPI` contract).
/// `status` is one of `queued`, `running`, `retry`, `succeeded`, `failed` —
/// kept as a plain `String` rather than a closed enum so an added status
/// value degrades to a visible string instead of a deserialize failure that
/// would take down the whole poll.
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct GenerationJob {
    pub id: String,
    pub source_id: String,
    pub title: String,
    pub status: String,
    pub card_count: usize,
    pub attempts: u32,
    pub retryable: bool,
    pub error: Option<String>,
    pub created_at: i64,
    pub updated_at: i64,
}

impl GenerationJob {
    #[must_use]
    pub fn is_terminal(&self) -> bool {
        matches!(self.status.as_str(), "succeeded" | "failed")
    }
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct EnqueuedGenerationJob {
    #[serde(flatten)]
    pub job: GenerationJob,
    /// `true` when this call joined a job already in flight for the same
    /// source rather than starting a new one.
    pub coalesced: bool,
}

/// Durable generation receipt, including failures after material was saved.
/// A succeeded job has already published its validated quizzes for review.
#[derive(Clone, Debug)]
pub enum GenerationOutcome {
    Succeeded {
        job: GenerationJob,
        coalesced: bool,
    },
    Failed {
        job: GenerationJob,
        coalesced: bool,
    },
    /// The job keeps running server-side; inspect its id rather than saving
    /// the same material again.
    TimedOut {
        job: GenerationJob,
        coalesced: bool,
    },
    /// The source was saved, but enqueueing could not be confirmed.
    AdmissionFailed {
        error: String,
    },
    /// The last confirmed job is retained when a status request fails.
    PollFailed {
        job: GenerationJob,
        coalesced: bool,
        error: String,
    },
}

/// One generated quiz (`StudyDraft` on the wire). Publication preserves the
/// validation and provenance fields without inventing a learner decision.
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DraftRow {
    pub id: String,
    pub review_unit_id: String,
    pub activity_kind: String,
    pub activity_stage: String,
    pub prompt: String,
    pub validation_status: String,
    #[serde(default)]
    pub validation_reasons: Vec<String>,
    pub worked_solution: Option<String>,
    pub approved: bool,
    #[serde(default)]
    pub learner_decision: Option<serde_json::Value>,
    #[serde(default)]
    pub source_spans: Vec<serde_json::Value>,
    #[serde(default)]
    pub provenance: Option<serde_json::Value>,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StudyView {
    #[serde(default)]
    pub drafts: Vec<DraftRow>,
    pub queue: Vec<StudyQueueRow>,
    pub current: Option<StudyCurrent>,
    #[serde(default)]
    pub concept_progress: Vec<ConceptProgress>,
    #[serde(default)]
    pub summary: StudySummary,
    pub due_count: usize,
    #[serde(default)]
    pub generation_notices: Vec<String>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StudyQueueRow {
    pub review_unit_id: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StudyCurrent {
    pub review_unit_id: String,
    pub concept_key: Option<String>,
    #[serde(default)]
    pub prompt_id: String,
    #[serde(default)]
    pub activity_kind: String,
    #[serde(default)]
    pub activity_stage: String,
    pub prompt: String,
    #[serde(default)]
    pub choices: Vec<String>,
    #[serde(default)]
    pub revision_expected_answer: String,
    pub expected_answer: Option<String>,
    #[serde(default)]
    pub reference_text: Option<String>,
    #[serde(default)]
    pub worked_solution: Option<String>,
    pub grade: Option<StudyGrade>,
    #[serde(default)]
    pub review_state: Option<ReviewState>,
    #[serde(default)]
    pub schedule_change: Option<ScheduleChange>,
    #[serde(default)]
    pub feedback: Option<StudyFeedback>,
    #[serde(default)]
    pub content_feedback_head_id: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StudyGrade {
    pub verdict: String,
    pub rating: u8,
    pub is_correct: bool,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ReviewState {
    pub due: i64,
    pub reps: u32,
    pub lapses: u32,
    pub state: u8,
    pub last_review: Option<i64>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ScheduleChange {
    pub before: Option<ReviewState>,
    pub after: ReviewState,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StudyFeedback {
    pub verdict: String,
    pub expected_answer: String,
    pub item_history: StudyItemHistory,
    pub concept_progress: Option<ConceptProgress>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StudyItemHistory {
    pub attempts: u32,
    pub correct: u32,
    pub success_rate: String,
    pub trend: String,
    pub last_seen: Option<i64>,
    pub last_seen_summary: String,
    pub last_response_time_ms: Option<u32>,
    pub average_response_time_ms: Option<u32>,
    pub response_time_trend: String,
    pub stage: String,
    pub next_review: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ConceptProgress {
    pub concept_key: String,
    pub concept_label: String,
    pub attempts: u32,
    pub correct: u32,
    pub success_rate: String,
    pub trend: String,
    pub average_response_time_ms: Option<u32>,
    pub response_time_trend: String,
    pub health: String,
    pub summary: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StudySummary {
    #[serde(default)]
    pub source_count: usize,
    #[serde(default)]
    pub accepted_draft_count: usize,
    #[serde(default)]
    pub approved_review_unit_count: usize,
    #[serde(default)]
    pub attempt_count: u32,
    #[serde(default)]
    pub last_outcome: Option<String>,
    #[serde(default)]
    pub next_review_unit_id: Option<String>,
}

/// A recorded content-feedback verdict (`kept`/`dropped`) on one review
/// unit — the "declare this generated card good or bad" signal, distinct
/// from grading an answer.
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ContentFeedback {
    pub id: String,
    pub review_unit_id: String,
    pub verdict: String,
    pub rationale: Option<String>,
    pub source: String,
    pub account_id: String,
    pub occurred_at: i64,
    pub supersedes_id: Option<String>,
}

/// Thin Bearer-token client over the v1 API, scoped to one account.
pub struct MemoryEngineClient {
    agent: ureq::Agent,
    base_url: String,
    account_id: String,
    session_token: String,
}

impl MemoryEngineClient {
    #[must_use]
    pub fn new(base_url: String, account_id: String, session_token: String) -> Self {
        let agent = ureq::Agent::config_builder()
            .timeout_global(Some(REQUEST_TIMEOUT))
            // Read the response body ourselves on 4xx/5xx instead of losing
            // it to a bare `StatusCode(u16)` error: the API's `ApiError`
            // body (`{"error": "..."}`) is the safe, agent-actionable
            // message ("Generation queue is full for this account...",
            // "Direct synchronous generation is disabled in production...")
            // that a raw status code discards.
            .http_status_as_error(false)
            .build()
            .into();
        Self {
            agent,
            base_url,
            account_id,
            session_token,
        }
    }

    #[must_use]
    pub fn account_id(&self) -> &str {
        &self.account_id
    }

    /// Save text with an inferred title and generate reviewable quizzes.
    /// Uses the same source capture and durable queue as the other clients.
    ///
    /// # Errors
    ///
    /// Returns an error only when input is empty or saving fails. Later
    /// failures retain the saved source in the returned receipt.
    pub fn learn(&self, input: &str) -> Result<(SourceRecord, GenerationOutcome), String> {
        let input = input.trim();
        if input.is_empty() {
            return Err("learning input must not be empty".to_owned());
        }
        let source: SourceRecord = self.post_json(
            &format!("/v1/accounts/{}/sources", self.account_id),
            &json!({ "body": input }),
        )?;
        let outcome = self.generate_saved_source(&source.source_id);
        Ok((source, outcome))
    }

    /// Save a project-scoped deck and generate its quizzes on the durable
    /// queue. Successful generation publishes validated quizzes directly.
    ///
    /// # Errors
    ///
    /// Returns an error only when saving the deck fails. Admission and
    /// polling failures retain the saved deck and last confirmed job.
    pub fn create_deck(
        &self,
        project_key: &str,
        title: &str,
        body: &str,
        ttl_expires_at: Option<i64>,
    ) -> Result<(ProjectDeckRecord, GenerationOutcome), String> {
        let mut request = json!({
            "projectKey": project_key,
            "title": title,
            "body": body,
        });
        if let Some(ttl) = ttl_expires_at {
            request["ttlExpiresAt"] = json!(ttl);
        }
        let deck: ProjectDeckRecord = self.post_json(
            &format!("/v1/accounts/{}/project-decks", self.account_id),
            &request,
        )?;

        let outcome = self.generate_saved_source(&deck.source.source_id);

        Ok((deck, outcome))
    }

    /// Enqueue (or join an already-in-flight) generation job for a saved
    /// source. Composes `POST .../generation-jobs` — the durable queue every
    /// production deployment requires.
    ///
    /// # Errors
    ///
    /// Returns an error when the request fails, including the declared
    /// rejection/unavailable messages the queue returns (budget exhausted,
    /// queue full, generation temporarily disabled).
    pub fn enqueue_generation_job(&self, source_id: &str) -> Result<EnqueuedGenerationJob, String> {
        self.post_empty(&format!(
            "/v1/accounts/{}/sources/{source_id}/generation-jobs",
            self.account_id
        ))
    }

    /// Fetch one generation job's current status.
    ///
    /// # Errors
    ///
    /// Returns an error when the request fails (including "not found" for
    /// an unknown or cross-account job id).
    pub fn generation_job(&self, job_id: &str) -> Result<GenerationJob, String> {
        self.get(&format!(
            "/v1/accounts/{}/generation-jobs/{job_id}",
            self.account_id
        ))
    }

    fn generate_saved_source(&self, source_id: &str) -> GenerationOutcome {
        match self.enqueue_generation_job(source_id) {
            Ok(enqueued) => self.poll_generation_job(enqueued.job, enqueued.coalesced),
            Err(error) => GenerationOutcome::AdmissionFailed { error },
        }
    }

    /// Poll the durable job without fetching unrelated account-wide content.
    fn poll_generation_job(&self, mut job: GenerationJob, coalesced: bool) -> GenerationOutcome {
        for attempt in 0..GENERATION_POLL_MAX_ATTEMPTS {
            if job.is_terminal() {
                break;
            }
            if attempt + 1 == GENERATION_POLL_MAX_ATTEMPTS {
                return GenerationOutcome::TimedOut { job, coalesced };
            }
            thread::sleep(GENERATION_POLL_INTERVAL);
            match self.generation_job(&job.id) {
                Ok(updated) => job = updated,
                Err(error) => {
                    return GenerationOutcome::PollFailed {
                        job,
                        coalesced,
                        error,
                    };
                }
            }
        }

        match job.status.as_str() {
            "succeeded" => GenerationOutcome::Succeeded { job, coalesced },
            "failed" => GenerationOutcome::Failed { job, coalesced },
            _ => GenerationOutcome::TimedOut { job, coalesced },
        }
    }

    /// List active published quizzes with stable ids, validation and provenance.
    ///
    /// # Errors
    ///
    /// Returns an error when the underlying study-view request fails.
    pub fn quizzes(&self) -> Result<Vec<DraftRow>, String> {
        let view: StudyView = self.get(&format!("/v1/accounts/{}/review/next", self.account_id))?;
        let active_ids = view
            .queue
            .iter()
            .map(|row| row.review_unit_id.as_str())
            .collect::<std::collections::HashSet<_>>();
        Ok(view
            .drafts
            .into_iter()
            .filter(|draft| {
                draft.activity_kind == "quiz"
                    && draft.approved
                    && active_ids.contains(draft.review_unit_id.as_str())
            })
            .collect())
    }

    /// List saved sources that belong to a project deck (`project_key` set),
    /// optionally filtered to one `project_key`.
    ///
    /// # Errors
    ///
    /// Returns an error when the list request fails.
    pub fn list_decks(&self, project_key: Option<&str>) -> Result<Vec<SourceRecord>, String> {
        let list: SourceList = self.get(&format!("/v1/accounts/{}/sources", self.account_id))?;
        Ok(list
            .sources
            .into_iter()
            .filter(|source| {
                source.project_key.is_some()
                    && project_key.is_none_or(|key| source.project_key.as_deref() == Some(key))
            })
            .collect())
    }

    /// Retire every card generated from a project deck.
    ///
    /// # Errors
    ///
    /// Returns an error when the invalidate request fails.
    pub fn invalidate_deck(&self, deck_id: &str, event: &str) -> Result<StudyView, String> {
        self.post_json(
            &format!(
                "/v1/accounts/{}/project-decks/{deck_id}/invalidate",
                self.account_id
            ),
            &json!({ "event": event }),
        )
    }

    /// Update one published quiz's wording without changing its schedule.
    ///
    /// # Errors
    /// Returns an error when the request fails.
    pub fn edit_quiz(
        &self,
        draft_id: &str,
        prompt: &str,
        expected_answer: &str,
    ) -> Result<StudyView, String> {
        self.post_json(
            &format!("/v1/accounts/{}/drafts/{draft_id}/edit", self.account_id),
            &json!({
                "prompt": prompt,
                "expectedAnswer": expected_answer,
            }),
        )
    }

    /// Remove one generated quiz from future review.
    ///
    /// # Errors
    /// Returns an error when the request fails.
    pub fn remove_quiz(&self, draft_id: &str) -> Result<StudyView, String> {
        self.post_empty(&format!(
            "/v1/accounts/{}/drafts/{draft_id}/reject",
            self.account_id
        ))
    }

    /// Fetch the next due review card.
    ///
    /// # Errors
    ///
    /// Returns an error when the request fails.
    pub fn next_review(&self) -> Result<StudyView, String> {
        self.post_empty(&format!("/v1/accounts/{}/review/next", self.account_id))
    }

    /// Submit a graded answer for one review card.
    ///
    /// # Errors
    ///
    /// Returns an error when the request fails.
    pub fn submit_review(
        &self,
        review_unit_id: &str,
        answer: &str,
        response_time_ms: u32,
        idempotency_key: &str,
    ) -> Result<StudyView, String> {
        self.post_json(
            &format!(
                "/v1/accounts/{}/review/{review_unit_id}/submit",
                self.account_id
            ),
            &json!({
                "answer": answer,
                "responseTimeMs": response_time_ms,
                "idempotencyKey": idempotency_key,
            }),
        )
    }

    /// Reveal the current card's expected answer without grading it.
    ///
    /// # Errors
    ///
    /// Returns an error when the request fails.
    pub fn reveal_review(&self, review_unit_id: &str) -> Result<StudyView, String> {
        self.post_empty(&format!(
            "/v1/accounts/{}/review/{review_unit_id}/reveal",
            self.account_id
        ))
    }

    /// Declared remediation: request extra reference material for the
    /// current card instead of grading it now.
    ///
    /// # Errors
    ///
    /// Returns an error when the request fails.
    pub fn learn_more(&self, review_unit_id: &str) -> Result<StudyView, String> {
        self.post_empty(&format!(
            "/v1/accounts/{}/review/{review_unit_id}/reference",
            self.account_id
        ))
    }

    /// Declared remediation: skip the current card, leaving its schedule
    /// untouched, and advance to the next due card.
    ///
    /// # Errors
    ///
    /// Returns an error when the request fails.
    pub fn skip_review(&self, review_unit_id: &str) -> Result<StudyView, String> {
        self.post_empty(&format!(
            "/v1/accounts/{}/review/{review_unit_id}/skip",
            self.account_id
        ))
    }

    /// Declared remediation: push just this card later in the due queue.
    ///
    /// # Errors
    ///
    /// Returns an error when the request fails.
    pub fn snooze_review(&self, review_unit_id: &str) -> Result<StudyView, String> {
        self.post_empty(&format!(
            "/v1/accounts/{}/review/{review_unit_id}/snooze",
            self.account_id
        ))
    }

    /// Declared remediation: push every card for this card's concept later
    /// in the due queue.
    ///
    /// # Errors
    ///
    /// Returns an error when the request fails.
    pub fn snooze_concept_review(&self, review_unit_id: &str) -> Result<StudyView, String> {
        self.post_empty(&format!(
            "/v1/accounts/{}/review/{review_unit_id}/snooze-concept",
            self.account_id
        ))
    }

    /// Declared remediation: request bridge (scaffold) material for a card
    /// the learner is consistently missing.
    ///
    /// # Errors
    ///
    /// Returns an error when the request fails.
    pub fn bridge_review(&self, review_unit_id: &str) -> Result<StudyView, String> {
        self.post_empty(&format!(
            "/v1/accounts/{}/review/{review_unit_id}/bridge",
            self.account_id
        ))
    }

    /// Record a `kept`/`dropped` content-feedback verdict on one review
    /// unit's generated content — distinct from grading an answer.
    ///
    /// # Errors
    ///
    /// Returns an error when the request fails, including a declared
    /// conflict (HTTP 409) when `idempotency_key`/`supersedes_id` no longer
    /// match the current feedback revision.
    pub fn content_feedback(
        &self,
        review_unit_id: &str,
        verdict: &str,
        rationale: Option<&str>,
        idempotency_key: &str,
        supersedes_id: Option<&str>,
    ) -> Result<ContentFeedback, String> {
        let mut request = json!({
            "verdict": verdict,
            "idempotencyKey": idempotency_key,
        });
        if let Some(rationale) = rationale {
            request["rationale"] = json!(rationale);
        }
        if let Some(supersedes_id) = supersedes_id {
            request["supersedesId"] = json!(supersedes_id);
        }
        self.post_json(
            &format!(
                "/v1/accounts/{}/review/{review_unit_id}/content-feedback",
                self.account_id
            ),
            &request,
        )
    }

    fn get<T: DeserializeOwned>(&self, path: &str) -> Result<T, String> {
        let mut response = self
            .agent
            .get(endpoint(&self.base_url, path))
            .header("Authorization", &self.authorization())
            .call()
            .map_err(|error| transport_failure(path, &error))?;
        read_json(&mut response, path)
    }

    fn post_empty<T: DeserializeOwned>(&self, path: &str) -> Result<T, String> {
        let mut response = self
            .agent
            .post(endpoint(&self.base_url, path))
            .header("Authorization", &self.authorization())
            .send_empty()
            .map_err(|error| transport_failure(path, &error))?;
        read_json(&mut response, path)
    }

    fn post_json<T: DeserializeOwned>(
        &self,
        path: &str,
        body: &serde_json::Value,
    ) -> Result<T, String> {
        let mut response = self
            .agent
            .post(endpoint(&self.base_url, path))
            .header("Authorization", &self.authorization())
            .send_json(body)
            .map_err(|error| transport_failure(path, &error))?;
        read_json(&mut response, path)
    }

    fn authorization(&self) -> String {
        format!("Bearer {}", self.session_token)
    }
}

#[derive(Deserialize)]
struct ApiError {
    error: String,
}

/// Deserialize a successful response, or surface the server's safe
/// `{"error": "..."}` message on a non-2xx status instead of a bare status
/// code (the agent built with `http_status_as_error(false)` above never
/// turns a 4xx/5xx into a transport error, so every call reaches here).
fn read_json<T: DeserializeOwned>(
    response: &mut ureq::http::Response<ureq::Body>,
    action: &str,
) -> Result<T, String> {
    let status = response.status();
    if status.is_success() {
        return response
            .body_mut()
            .with_config()
            .limit(MAX_RESPONSE_BYTES)
            .read_json()
            .map_err(|error| format!("{action} returned unreadable JSON: {error}"));
    }

    let body: Result<ApiError, _> = response
        .body_mut()
        .with_config()
        .limit(MAX_RESPONSE_BYTES)
        .read_json();
    match body {
        Ok(ApiError { error }) => Err(format!("{action} failed: {error} (HTTP {status})")),
        Err(_) => Err(format!("{action} failed with HTTP {status}")),
    }
}

fn transport_failure(action: &str, error: &ureq::Error) -> String {
    format!("{action} transport failed: {error}")
}

fn endpoint(base_url: &str, path: &str) -> String {
    format!("{}{}", base_url.trim_end_matches('/'), path)
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use axum::{
        extract::{Request, State},
        middleware::{self, Next},
        response::Response,
    };

    use super::*;

    type RequestLog = Arc<Mutex<Vec<(String, String)>>>;

    fn unique_suffix() -> String {
        use std::sync::atomic::{AtomicU64, Ordering};
        static COUNTER: AtomicU64 = AtomicU64::new(0);

        let millis = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map_or(0, |duration| duration.as_millis());
        let counter = COUNTER.fetch_add(1, Ordering::Relaxed);
        format!("{}-{millis}-{counter}", std::process::id())
    }

    /// A local `ApiState` with `email` pre-allowlisted — every test server
    /// needs credentials provisioned up front now that anonymous account
    /// creation is deny-by-default (`AuthConfig::default()`).
    fn provisioned_state(email: &str) -> memory_engine_api::ApiState {
        let store_root =
            std::env::temp_dir().join(format!("memory-engine-mcp-client-{}", unique_suffix()));
        memory_engine_api::ApiState::new(
            memory_engine_api::AccountRegistry::with_store_root(&store_root).with_auth_config(
                memory_engine_api::AuthConfig::allow_emails([email.to_owned()])
                    .with_anonymous_account_creation(true),
            ),
        )
    }

    /// Real authenticated API with a running durable worker and request capture.
    /// The capture protects the production-only queue contract.
    async fn spawn_local_api_with_capture(
        email: &str,
    ) -> (
        String,
        tokio::task::JoinHandle<()>,
        RequestLog,
        String,
        String,
    ) {
        let state = provisioned_state(email);
        let created = state
            .create_account(email)
            .expect("pre-provision test account");
        state.start_worker();

        let requests: RequestLog = Arc::new(Mutex::new(Vec::new()));
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind local API listener");
        let address = listener.local_addr().expect("local address");
        let router = memory_engine_api::router(state).layer(middleware::from_fn_with_state(
            requests.clone(),
            capture_request,
        ));
        let handle = tokio::spawn(async move {
            axum::serve(listener, router)
                .await
                .expect("serve local API");
        });
        (
            format!("http://{address}"),
            handle,
            requests,
            created.account_id,
            created.session_token,
        )
    }

    async fn capture_request(State(log): State<RequestLog>, req: Request, next: Next) -> Response {
        log.lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .push((req.method().to_string(), req.uri().path().to_owned()));
        next.run(req).await
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn learning_publishes_quizzes_that_can_be_edited_and_removed_directly() {
        let (base_url, server, _requests, account_id, session_token) =
            spawn_local_api_with_capture("mcp-client-lifecycle-test@example.com").await;
        let client = MemoryEngineClient::new(base_url, account_id, session_token);
        let body = "Concept: NATO letter B\nActivity: quiz\nStage: recognition-3\n\
            Question: What is the NATO phonetic alphabet word for B?\nAnswer: BRAVO\n\
            Distractors: BAKER, BOSTON\n\
            Reference: The NATO phonetic alphabet word for B is BRAVO.";
        let (source, outcome) = client.learn(body).expect("learn material");
        let GenerationOutcome::Succeeded { job, .. } = outcome else {
            panic!("learning must publish the validated fixture: {outcome:?}");
        };
        assert_eq!(job.source_id, source.source_id);
        assert_eq!(job.card_count, 1);
        let quizzes = client.quizzes().expect("published quizzes");
        let quiz = &quizzes[0];
        assert!(quiz.learner_decision.is_none());
        assert_eq!(quiz.validation_status, "accepted");
        let due = client.next_review().expect("review after learning");
        assert_eq!(due.due_count, 1);
        assert_eq!(
            due.current.expect("published quiz").review_unit_id,
            quiz.review_unit_id
        );

        let graded = client
            .submit_review(&quiz.review_unit_id, "BRAVO", 5000, "before-edit")
            .expect("grade the published quiz");
        let graded_current = graded.current.as_ref().expect("held graded quiz");
        let graded_schedule = graded_current
            .review_state
            .as_ref()
            .expect("saved schedule");
        for prompt in [
            "Which NATO word represents B?",
            "B is represented by which NATO word?",
        ] {
            let edited = client
                .edit_quiz(&quiz.id, prompt, "BRAVO")
                .expect("edit the published quiz repeatedly");
            assert_eq!(edited.due_count, 0, "editing must not reset its schedule");
            assert_eq!(edited.summary.attempt_count, 1);
            let updated = client.quizzes().expect("edited quizzes");
            let updated = updated
                .iter()
                .find(|row| row.id == quiz.id)
                .expect("edited quiz");
            assert_eq!(updated.prompt, prompt);
            assert_eq!(updated.review_unit_id, quiz.review_unit_id);
            let held: StudyView = client
                .get(&format!("/v1/accounts/{}/review/next", client.account_id))
                .expect("reading inventory must not consume the held grade");
            let held_current = held.current.expect("held review after management");
            assert_eq!(held_current.review_unit_id, quiz.review_unit_id);
            assert_eq!(
                held_current.grade.expect("preserved grade").verdict,
                "correct"
            );
            let held_schedule = held_current.review_state.expect("preserved schedule");
            assert_eq!(held_schedule.due, graded_schedule.due);
            assert_eq!(held_schedule.reps, graded_schedule.reps);
        }
        let removed = client.remove_quiz(&quiz.id).expect("remove the quiz");
        assert_eq!(removed.due_count, 0);
        assert_eq!(removed.summary.attempt_count, 1);
        assert!(client.quizzes().expect("remaining quizzes").is_empty());
        client
            .edit_quiz(&quiz.id, "Must not resurrect removed content", "BRAVO")
            .expect_err("removal is terminal");
        assert_eq!(
            client.next_review().expect("after rejected edit").due_count,
            0
        );
        server.abort();
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn create_deck_enqueues_and_polls_without_ever_requesting_generate() {
        let (base_url, server, requests, account_id, session_token) =
            spawn_local_api_with_capture("mcp-client-generate-guard-test@example.com").await;
        let client = MemoryEngineClient::new(base_url, account_id, session_token);

        let deck_body = "Concept: NATO letter A\nActivity: quiz\nStage: recognition-3\n\
            Question: What is the NATO phonetic alphabet word for A?\nAnswer: ALFA\n\
            Distractors: ABLE, ADAM\n\
            Reference: The NATO phonetic alphabet word for A is ALFA.";

        let (_deck, outcome) = client
            .create_deck("nato-onboarding", "NATO letter A fixture", deck_body, None)
            .expect("create_deck reaches a terminal outcome against a real worker");
        let GenerationOutcome::Succeeded { job, .. } = &outcome else {
            panic!(
                "the job must reach succeeded against a real running worker, not time out: {outcome:?}"
            );
        };
        assert_eq!(job.card_count, 1);
        let due = client.next_review().expect("review after deck creation");
        assert_eq!(due.due_count, 1);
        assert_eq!(
            due.current.expect("automatically published quiz").prompt,
            "What is the NATO phonetic alphabet word for A?"
        );

        server.abort();

        let captured = requests
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        assert!(
            !captured.iter().any(|(_, path)| path.ends_with("/generate")),
            "create_deck must never request the legacy synchronous /generate route; captured: {captured:?}"
        );
        assert!(
            captured
                .iter()
                .any(|(method, path)| method == "POST" && path.ends_with("/generation-jobs")),
            "create_deck must enqueue on the durable generation-jobs queue; captured: {captured:?}"
        );
        assert!(
            captured
                .iter()
                .any(|(method, path)| method == "GET" && path.contains("/generation-jobs/")),
            "create_deck must poll the enqueued job's status; captured: {captured:?}"
        );
    }

    async fn spawn_api_with_generation_fault(
        email: &str,
        fail_poll: bool,
    ) -> (MemoryEngineClient, tokio::task::JoinHandle<()>) {
        let state = provisioned_state(email);
        let created = state.create_account(email).expect("provision account");
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind API");
        let address = listener.local_addr().expect("local address");
        let router = memory_engine_api::router(state).layer(middleware::from_fn(
            move |req: Request, next: Next| async move {
                use axum::response::IntoResponse;
                let fail = if fail_poll {
                    req.method() == "GET" && req.uri().path().contains("/generation-jobs/")
                } else {
                    req.method() == "POST" && req.uri().path().ends_with("/generation-jobs")
                };
                if fail {
                    return (
                        axum::http::StatusCode::SERVICE_UNAVAILABLE,
                        axum::Json(json!({"error": "Generation temporarily unavailable."})),
                    )
                        .into_response();
                }
                next.run(req).await
            },
        ));
        let server = tokio::spawn(async move {
            axum::serve(listener, router).await.expect("serve API");
        });
        (
            MemoryEngineClient::new(
                format!("http://{address}"),
                created.account_id,
                created.session_token,
            ),
            server,
        )
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn learn_returns_the_saved_source_when_generation_admission_fails() {
        let (client, server) =
            spawn_api_with_generation_fault("mcp-admission-test@example.com", false).await;
        let response = crate::call_tool(&client, "learn", &json!({"input": "spaced repetition"}))
            .expect("saved-material receipt");
        assert_eq!(response["isError"], true);
        let payload: serde_json::Value =
            serde_json::from_str(response["content"][0]["text"].as_str().expect("receipt"))
                .expect("receipt JSON");
        assert_eq!(payload["generation"]["status"], "admission_failed");
        let error = payload["generation"]["error"].as_str().expect("safe error");
        assert!(error.contains("Generation temporarily unavailable."));
        assert!(error.contains("503"));
        let saved: SourceList = client
            .get(&format!("/v1/accounts/{}/sources", client.account_id))
            .expect("saved sources after failed admission");
        assert_eq!(saved.sources.len(), 1);
        assert_eq!(saved.sources[0].source_id, payload["source"]["sourceId"]);
        assert_eq!(saved.sources[0].body, "spaced repetition");
        server.abort();
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn polling_failure_retains_the_durable_job_and_saved_deck() {
        let (client, server) =
            spawn_api_with_generation_fault("mcp-poll-test@example.com", true).await;
        let (deck, outcome) = client
            .create_deck("example-project", "Saved deck", "spaced repetition", None)
            .expect("saved deck receipt");
        let GenerationOutcome::PollFailed { job, error, .. } = outcome else {
            panic!("polling must fail with a durable receipt: {outcome:?}");
        };
        assert_eq!(job.source_id, deck.source.source_id);
        assert!(error.contains("503"));
        let joined = client
            .enqueue_generation_job(&deck.source.source_id)
            .expect("join the existing job, without saving again");
        assert!(joined.coalesced);
        assert_eq!(joined.job.id, job.id);
        let saved = client
            .list_decks(Some("example-project"))
            .expect("saved deck");
        assert_eq!(saved[0].source_id, deck.source.source_id);
        server.abort();
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn capture_and_generation_remain_scoped_to_the_credential_owner() {
        let root = std::env::temp_dir().join(format!("mcp-tenant-{}", unique_suffix()));
        let state = memory_engine_api::ApiState::new(
            memory_engine_api::AccountRegistry::with_store_root(root).with_auth_config(
                memory_engine_api::AuthConfig::allow_emails([
                    "mcp-owner@example.com".to_owned(),
                    "mcp-other@example.com".to_owned(),
                ])
                .with_anonymous_account_creation(true),
            ),
        );
        let owner = state
            .create_account("mcp-owner@example.com")
            .expect("provision owner");
        let other = state
            .create_account("mcp-other@example.com")
            .expect("provision other account");
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind API");
        let base_url = format!("http://{}", listener.local_addr().expect("address"));
        let server = tokio::spawn(async move {
            axum::serve(listener, memory_engine_api::router(state))
                .await
                .expect("serve API");
        });
        let client = MemoryEngineClient::new(
            base_url.clone(),
            owner.account_id.clone(),
            owner.session_token,
        );
        let source: SourceRecord = client
            .post_json(
                &format!("/v1/accounts/{}/sources", owner.account_id),
                &json!({"body": "Private spaced repetition notes."}),
            )
            .expect("capture owned material");
        let enqueued = client
            .enqueue_generation_job(&source.source_id)
            .expect("enqueue owned material");
        let mismatched = MemoryEngineClient::new(
            base_url.clone(),
            owner.account_id.clone(),
            other.session_token.clone(),
        );
        assert!(mismatched
            .learn("must not be saved in the owner's account")
            .expect_err("cross-account capture denied")
            .contains("403"));
        let other_client = MemoryEngineClient::new(base_url, other.account_id, other.session_token);
        assert!(other_client
            .enqueue_generation_job(&source.source_id)
            .expect_err("another account's source is hidden")
            .contains("404"));
        assert!(other_client
            .generation_job(&enqueued.job.id)
            .expect_err("another account's job is hidden")
            .contains("404"));
        let saved: SourceList = client
            .get(&format!("/v1/accounts/{}/sources", owner.account_id))
            .expect("owner's material");
        assert_eq!(saved.sources, vec![source]);
        server.abort();
    }
}
