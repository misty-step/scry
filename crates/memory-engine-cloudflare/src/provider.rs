//! Worker Fetch owns transport only. The portable `OpenRouter` contract owns
//! prompts/parsers; shared generation/study owns trust, caching, and decisions.
use std::{cell::RefCell, time::Duration};

use futures_util::{
    future::{select, Either},
    pin_mut, StreamExt,
};
use memory_engine_api_state::StudyViewResponse;
use memory_engine_generation::{
    BetaGenerationError, BridgeMaterial, BridgeMaterialProvider, BridgeMaterialRequest,
    DraftProvider, DraftRejection, ProviderDrafts, ProviderFailure, ProviderUsage,
    ReferenceNoteDraft, ReferenceNoteProvider, ReferenceNoteRequest, StructuredBlockProvider,
};
use memory_engine_openrouter::{
    content, API_KEY_ENV, BASE_URL_ENV, DEFAULT_BASE_URL, DEFAULT_MODEL, MODEL_ENV,
};
use memory_engine_persistence::{GeneratedPromptModel, SourceDocument, SourcePermission};
use memory_engine_study::{BetaStudyError, BetaStudySession};
use worker::{
    AbortController, Delay, Env, Fetch, Headers, Method, Request, RequestInit, RequestRedirect,
};

use crate::{database::Database, jobs, now_ms, store::SqlStudyStore, AppResult, Failure};

pub(crate) const REQUEST_TIMEOUT_MS: u64 = 60_000;
pub(crate) const LOCAL_MODEL: &str = "deterministic";

pub(crate) fn model_name(env: &Env) -> AppResult<String> {
    let name = env
        .var(MODEL_ENV)
        .map_or_else(|_| DEFAULT_MODEL.to_owned(), |value| value.to_string());
    if name.trim().is_empty() || name.len() > 200 {
        return Err(Failure::internal(
            "Generation model configuration is invalid.",
        ));
    }
    Ok(name)
}

pub(crate) fn model_identity(name: &str) -> GeneratedPromptModel {
    GeneratedPromptModel {
        provider: "openrouter".to_owned(),
        name: name.to_owned(),
        version: content::PromptVariant::Principled.label().to_owned(),
    }
}

/// `LocalOnly` and explicitly authored Question/Answer blocks are deterministic.
/// Ordinary prose is never routed to the fixture model when Fetch is unavailable.
pub(crate) fn local_generation(source: &SourceDocument) -> bool {
    source.permission == SourcePermission::LocalOnly
        || StructuredBlockProvider
            .generate_drafts(source)
            .is_ok_and(|drafts| !drafts.candidates.is_empty())
}

pub(crate) async fn drafts(
    env: &Env,
    source: &SourceDocument,
    model: &str,
) -> Result<ProviderDrafts, ProviderFailure> {
    let request = content::quiz_request(
        content::PromptVariant::Principled,
        content::DEFAULT_MAX_DRAFTS,
        source,
    )?;
    let response = complete(env, model, &request).await?;
    content::parse_drafts_response(
        response,
        source,
        model_identity(model),
        content::DEFAULT_MAX_DRAFTS,
    )
}

pub(crate) async fn repair(
    env: &Env,
    source: &SourceDocument,
    model: &str,
    rejections: &[DraftRejection],
) -> Result<Option<ProviderDrafts>, ProviderFailure> {
    let Some(request) = content::repair_request(
        content::PromptVariant::Principled,
        content::DEFAULT_MAX_DRAFTS,
        source,
        rejections,
    )?
    else {
        return Ok(None);
    };
    let response = complete(env, model, &request).await?;
    content::parse_drafts_response(
        response,
        source,
        model_identity(model),
        rejections.len().min(content::MAX_REPAIR_DRAFTS),
    )
    .map(Some)
}

fn completion_request(
    env: &Env,
    model: &str,
    content_request: &content::ContentRequest,
) -> Result<Request, ProviderFailure> {
    let payload = content_request.payload(model)?.to_string();
    let key = env
        .secret(API_KEY_ENV)
        .map_err(|_| {
            ProviderFailure::new("The model provider is not configured. Your saved source is safe.")
        })?
        .to_string();
    if key.trim().is_empty() {
        return Err(ProviderFailure::new(
            "The model provider is not configured. Your saved source is safe.",
        ));
    }
    let base = env
        .var(BASE_URL_ENV)
        .map_or_else(|_| DEFAULT_BASE_URL.to_owned(), |value| value.to_string());
    let mut url = worker::Url::parse(&base).map_err(|_| {
        ProviderFailure::new("The model provider endpoint configuration is invalid.")
    })?;
    // An explicit local endpoint is useful for the same real transport smoke.
    // It is allowed only in the private development environment, never by input.
    let development = env
        .var("MEMORY_ENGINE_ENVIRONMENT")
        .is_ok_and(|value| matches!(value.to_string().as_str(), "local" | "development" | "test"));
    let loopback = matches!(url.host_str(), Some("localhost" | "127.0.0.1" | "[::1]"));
    if !(url.scheme() == "https" || development && loopback && url.scheme() == "http")
        || !url.username().is_empty()
        || url.password().is_some()
        || url.query().is_some()
        || url.fragment().is_some()
    {
        return Err(ProviderFailure::new(
            "The model provider endpoint configuration is invalid.",
        ));
    }
    url.set_path(&format!(
        "{}/chat/completions",
        url.path().trim_end_matches('/')
    ));
    let headers = Headers::new();
    headers
        .set("content-type", "application/json")
        .map_err(transport_configuration)?;
    headers
        .set("authorization", &format!("Bearer {key}"))
        .map_err(transport_configuration)?;
    Request::new_with_init(
        url.as_str(),
        &RequestInit {
            method: Method::Post,
            headers,
            body: Some(payload.into()),
            // Inspect non-success responses without forwarding credentials.
            redirect: RequestRedirect::Manual,
            ..RequestInit::default()
        },
    )
    .map_err(transport_configuration)
}

/// One bounded request, no implicit retries, redirect, model, or endpoint fallback.
/// A durable job owns transient retry admission; manual calls report real failure.
async fn complete(
    env: &Env,
    model: &str,
    content_request: &content::ContentRequest,
) -> Result<content::StructuredResponse, ProviderFailure> {
    let request = completion_request(env, model, content_request)?;
    let controller = AbortController::default();
    let signal = controller.signal();
    let started = now_ms();
    let fetch = async {
        let mut response = Fetch::Request(request)
            .send_with_signal(&signal)
            .await
            .map_err(|_| ProviderFailure::transient("The model provider could not be reached."))?;
        let status = response.status_code();
        if !(200..300).contains(&status) {
            return Err(match status {
                408 | 429 | 500..=599 => ProviderFailure::transient("The model provider is temporarily unavailable. Your saved source is safe."),
                401 | 403 => ProviderFailure::new("The model provider could not authorize this generation. Your saved source is safe."),
                _ => ProviderFailure::new("The model provider rejected this generation request. Your saved source is safe."),
            });
        }
        let content_length = response
            .headers()
            .get("content-length")
            .ok()
            .flatten()
            .and_then(|value| value.parse::<u64>().ok());
        if content_length.is_some_and(|length| length > content::MAX_RESPONSE_BYTES) {
            return Err(ProviderFailure::new(
                "The model provider response exceeded the size limit.",
            ));
        }
        let mut body = Vec::new();
        let mut stream = response.stream().map_err(|_| {
            ProviderFailure::transient("The model provider response could not be read.")
        })?;
        while let Some(chunk) = stream.next().await {
            let chunk = chunk.map_err(|_| {
                ProviderFailure::transient("The model provider response was interrupted.")
            })?;
            if body.len().saturating_add(chunk.len()) as u64 > content::MAX_RESPONSE_BYTES {
                return Err(ProviderFailure::new(
                    "The model provider response exceeded the size limit.",
                ));
            }
            body.extend_from_slice(&chunk);
        }
        let raw = String::from_utf8(body)
            .map_err(|_| ProviderFailure::new("The model provider response could not be read."))?;
        content::parse_completion(&raw, elapsed_ms(started))
    };
    let deadline = Delay::from(Duration::from_millis(REQUEST_TIMEOUT_MS));
    pin_mut!(fetch, deadline);
    let result = match select(fetch, deadline).await {
        Either::Left((result, _)) => result,
        Either::Right(((), _)) => Err(ProviderFailure::transient(
            "The model provider timed out. Your saved source is safe.",
        )),
    };
    // Cancel a rejected/oversize stream and enforce the same total deadline on
    // headers plus body. No provider body/token is logged or surfaced.
    controller.abort();
    result.map_err(|failure| {
        let usage = failure.usage().cloned().unwrap_or(ProviderUsage {
            input_tokens: 0,
            output_tokens: 0,
            cost_usd_micros: None,
            latency_ms: elapsed_ms(started),
        });
        failure.with_usage(Some(usage))
    })
}

fn transport_configuration(_: worker::Error) -> ProviderFailure {
    ProviderFailure::new("The model provider request configuration is invalid.")
}

fn elapsed_ms(started: i64) -> u64 {
    u64::try_from(now_ms().saturating_sub(started)).unwrap_or_default()
}

/// Materialized real network output, consumed once by the synchronous shared
/// trust runner. It never invokes a failing provider to discover parameters.
pub(crate) struct PreparedDrafts {
    source: SourceDocument,
    model: GeneratedPromptModel,
    first: RefCell<Option<Result<ProviderDrafts, ProviderFailure>>>,
    repair: RefCell<Option<Result<Option<ProviderDrafts>, ProviderFailure>>>,
}

impl PreparedDrafts {
    pub(crate) fn new(
        source: SourceDocument,
        model: GeneratedPromptModel,
        first: Result<ProviderDrafts, ProviderFailure>,
        repair: Result<Option<ProviderDrafts>, ProviderFailure>,
    ) -> Self {
        Self {
            source,
            model,
            first: RefCell::new(Some(first)),
            repair: RefCell::new(Some(repair)),
        }
    }
}

impl DraftProvider for PreparedDrafts {
    fn model(&self) -> GeneratedPromptModel {
        self.model.clone()
    }
    fn generate_drafts(&self, source: &SourceDocument) -> Result<ProviderDrafts, ProviderFailure> {
        if *source != self.source {
            return Err(ProviderFailure::new(
                "The source changed before generation could be published.",
            ));
        }
        self.first
            .borrow_mut()
            .take()
            .ok_or_else(|| ProviderFailure::new("Generation output was already consumed."))?
    }
    fn repair_drafts(
        &self,
        source: &SourceDocument,
        _: &[DraftRejection],
    ) -> Result<Option<ProviderDrafts>, ProviderFailure> {
        if *source != self.source {
            return Err(ProviderFailure::new(
                "The source changed before generation could be published.",
            ));
        }
        self.repair.borrow_mut().take().unwrap_or(Ok(None))
    }
}

struct PreparedBridge {
    request: BridgeMaterialRequest,
    material: RefCell<Option<BridgeMaterial>>,
    model: GeneratedPromptModel,
}

impl ReferenceNoteProvider for PreparedBridge {
    fn model(&self) -> GeneratedPromptModel {
        self.model.clone()
    }
    fn explain_concept(
        &self,
        request: &ReferenceNoteRequest,
    ) -> Result<ReferenceNoteDraft, ProviderFailure> {
        if request.concept_key != self.request.concept_key
            || request.authorization() != self.request.authorization()
        {
            return Err(ProviderFailure::new(
                "The concept context changed before publication.",
            ));
        }
        self.material
            .borrow()
            .as_ref()
            .map(|material| material.reference_note.clone())
            .ok_or_else(|| ProviderFailure::new("The Bridge output was already consumed."))
    }
}

impl BridgeMaterialProvider for PreparedBridge {
    fn generate_bridge_material(
        &self,
        request: &BridgeMaterialRequest,
    ) -> Result<BridgeMaterial, ProviderFailure> {
        if *request != self.request {
            return Err(ProviderFailure::new(
                "The Bridge context changed before publication. Please try again.",
            ));
        }
        self.material
            .borrow_mut()
            .take()
            .ok_or_else(|| ProviderFailure::new("The Bridge output was already consumed."))
    }
}

pub(crate) fn study_failure(error: BetaStudyError<Failure>) -> Failure {
    match error {
        BetaStudyError::Store(failure) => failure,
        BetaStudyError::Generation(error) => generation_failure(error),
        BetaStudyError::NoActiveReviewUnit => {
            Failure::not_found("This review item is no longer available.")
        }
        _ => Failure::conflict("The study state changed. Reload this review before trying again."),
    }
}

pub(crate) fn generation_failure(error: BetaGenerationError<Failure>) -> Failure {
    match error {
        BetaGenerationError::Store(failure) => failure,
        BetaGenerationError::UnknownSourceDocument(_)
        | BetaGenerationError::UnknownReviewUnit(_) => {
            Failure::not_found("The study material was not found.")
        }
        BetaGenerationError::ArchivedSourceDocument(_)
        | BetaGenerationError::LocalOnlySource(_) => {
            Failure::forbidden("The source is not authorized for model generation.")
        }
        BetaGenerationError::SourceDocumentHasNoTextBody(_) => {
            Failure::bad_request("The source has no text to study.")
        }
        BetaGenerationError::ProviderFailure(message) => Failure::new(422, message),
    }
}

fn review_session(
    db: &Database,
    account_id: &str,
    review_unit_id: &str,
    restore_hold: bool,
) -> AppResult<BetaStudySession<SqlStudyStore>> {
    let store = SqlStudyStore::new(db.clone(), account_id);
    let hold = if restore_hold {
        store.active_graded_review()?
    } else {
        None
    };
    let mut session = BetaStudySession::from_store(store, now_ms);
    if let Some(hold) = hold.filter(|hold| {
        hold.get("state").and_then(serde_json::Value::as_str) == Some("active")
            && hold
                .get("review_unit_id")
                .and_then(serde_json::Value::as_str)
                == Some(review_unit_id)
    }) {
        if let Some(key) = hold
            .get("idempotency_key")
            .and_then(serde_json::Value::as_str)
        {
            if session
                .restore_graded_review(review_unit_id, key)
                .map_err(study_failure)?
            {
                return Ok(session);
            }
        }
    }
    session
        .resume_review(review_unit_id)
        .map_err(study_failure)?;
    Ok(session)
}

pub async fn reference(
    db: &Database,
    env: &Env,
    account_id: &str,
    review_unit_id: &str,
) -> AppResult<StudyViewResponse> {
    let (request, cached) = db.transaction(|| {
        let mut session = review_session(db, account_id, review_unit_id, true)?;
        let request = session.prepare_reference_note().map_err(study_failure)?;
        let view = if request.is_none() {
            Some(StudyViewResponse::from_view(
                session.view().map_err(study_failure)?,
            ))
        } else {
            None
        };
        Ok((request, view))
    })?;
    if let Some(view) = cached {
        return Ok(view);
    }
    let request =
        request.ok_or_else(|| Failure::internal("Reference preparation was incomplete."))?;
    let content_request =
        content::reference_request(&request).map_err(|failure| provider_failure(&failure))?;
    let fingerprint = jobs::authorization_fingerprint(db, account_id, request.authorization())?;
    let model = model_name(env)?;
    let operation = jobs::reserve_operation(db, account_id, "reference", review_unit_id, &model)?;
    let result = complete(env, &model, &content_request)
        .await
        .and_then(|response| content::parse_reference_response(response, &request));
    let usage = match &result {
        Ok((_, usage)) => usage.as_ref(),
        Err(failure) => failure.usage(),
    };
    jobs::account_operation(db, account_id, &operation, usage, result.as_ref().err())?;
    let (note, _) = result.map_err(|failure| provider_failure(&failure))?;
    let committed = db.transaction(|| {
        jobs::check_operation(db, account_id, &operation)?;
        if jobs::authorization_fingerprint(db, account_id, request.authorization())? != fingerprint
        {
            return Err(Failure::conflict(
                "The source changed while its reference was being prepared. Please try again.",
            ));
        }
        let mut session = review_session(db, account_id, review_unit_id, true)?;
        let view = session
            .commit_reference_note(&request, note, model_identity(&model))
            .map_err(study_failure)?;
        jobs::finish_operation(db, account_id, &operation, None)?;
        Ok(StudyViewResponse::from_view(view))
    });
    if let Err(failure) = &committed {
        jobs::finish_operation(db, account_id, &operation, Some(failure.message.as_str()))?;
    }
    committed
}

pub async fn bridge(
    db: &Database,
    env: &Env,
    account_id: &str,
    review_unit_id: &str,
) -> AppResult<StudyViewResponse> {
    // Manual-only: no grade hook runs this path, and no candidate is auto-kept.
    let request = review_session(db, account_id, review_unit_id, false)?
        .prepare_bridge_material()
        .map_err(study_failure)?;
    let content_request =
        content::bridge_request(&request).map_err(|failure| provider_failure(&failure))?;
    let fingerprint = jobs::authorization_fingerprint(db, account_id, request.authorization())?;
    let model = model_name(env)?;
    let operation = jobs::reserve_operation(db, account_id, "bridge", review_unit_id, &model)?;
    let result = complete(env, &model, &content_request)
        .await
        .and_then(|response| {
            content::parse_bridge_response(response, &request, model_identity(&model))
        });
    let usage = match &result {
        Ok(material) => material.usage.as_ref(),
        Err(failure) => failure.usage(),
    };
    jobs::account_operation(db, account_id, &operation, usage, result.as_ref().err())?;
    let material = result.map_err(|failure| provider_failure(&failure))?;
    let committed = db.transaction(|| {
        jobs::check_operation(db, account_id, &operation)?;
        if jobs::authorization_fingerprint(db, account_id, request.authorization())? != fingerprint
        {
            return Err(Failure::conflict(
                "The source changed while Bridge was being prepared. Please try again.",
            ));
        }
        let mut session = review_session(db, account_id, review_unit_id, false)?;
        let provider = PreparedBridge {
            request,
            material: RefCell::new(Some(material)),
            model: model_identity(&model),
        };
        let view = session
            .generate_bridge_material_with_provider(&provider)
            .map_err(study_failure)?;
        jobs::finish_operation(db, account_id, &operation, None)?;
        Ok(StudyViewResponse::from_view(view))
    });
    if let Err(failure) = &committed {
        jobs::finish_operation(db, account_id, &operation, Some(failure.message.as_str()))?;
    }
    committed
}

fn provider_failure(failure: &ProviderFailure) -> Failure {
    Failure::new(
        if failure.is_transient() { 503 } else { 422 },
        failure.to_string(),
    )
}
