//! HTTP transport only: learning decisions stay in `BetaStudySession` and SQL stores.

use std::{collections::BTreeMap, fmt::Write as _, time::Duration};

use futures_util::{stream, StreamExt};
use memory_engine_api_render::{
    self as render, AnalyticsConceptFilter, AnalyticsConceptSort, AnalyticsViewOptions,
    ContentFeedbackRecovery,
};
use memory_engine_api_state::{
    AppAccount, ContentFeedbackRequest, CreateProjectDeckRequest, CreateSourceRequest,
    GenerationJob, InvalidateProjectDeckRequest, JobStatus, ProjectDeckRecord, SourceList,
    SourceRecord, StudyViewResponse, SubmitReviewRequest, MAX_SOURCE_BODY_BYTES,
    MAX_SOURCE_TITLE_BYTES,
};
use memory_engine_core::ReviewUnitId;
use memory_engine_generation::BetaGenerationError;
use memory_engine_persistence::SourcePermission;
use memory_engine_service::{
    record_content_feedback, ContentFeedback, ContentFeedbackError, ContentFeedbackVerdict,
    RecordContentFeedbackCommand, ServiceError,
};
use memory_engine_study::{
    infer_capture_title, BetaStudyError, BetaStudySession, BetaStudySourceInput, BetaStudyView,
};
use serde::{de::DeserializeOwned, Deserialize, Serialize};
use serde_json::{json, Value};
use worker::{Delay, Env, Method, Request, Response};

use crate::{
    auth, database::Database, jobs, now_ms, provider, store::SqlStudyStore, telemetry, AppResult,
    Failure,
};

type Study = BetaStudySession<SqlStudyStore>;
type StudyResult = Result<BetaStudyView, BetaStudyError<Failure>>;
const MAX_REQUEST_BYTES: usize = 2 * 1024 * 1024;
const MAX_RESPONSE_TIME_MS: u32 = 600_000;

/// All assets are embedded in the release; no runtime filesystem or third-party fonts.
pub fn static_response(req: &Request) -> AppResult<Option<Response>> {
    if !matches!(req.method(), Method::Get | Method::Head) {
        return Ok(None);
    }
    let path = req.path();
    let (body, content_type, cache): (&[u8], &str, &str) = match path.as_str() {
        "/static/ledger.css" => (
            render::LEDGER_CSS.as_bytes(),
            "text/css; charset=utf-8",
            "public, max-age=3600",
        ),
        "/static/app.js" => (
            include_bytes!("../../memory-engine-api/assets/app.js"),
            "text/javascript; charset=utf-8",
            "public, max-age=3600",
        ),
        "/sw.js" => (
            include_bytes!("../../memory-engine-api/assets/service-worker.js"),
            "text/javascript; charset=utf-8",
            "no-cache",
        ),
        "/offline.html" => (
            include_bytes!("../../memory-engine-api/assets/offline.html"),
            "text/html; charset=utf-8",
            "public, max-age=3600",
        ),
        "/static/fonts/manrope-latin-variable.woff2" => {
            (render::MANROPE_WOFF2, "font/woff2", "public, max-age=86400")
        }
        "/static/fonts/literata-latin-variable.woff2" => (
            render::LITERATA_WOFF2,
            "font/woff2",
            "public, max-age=86400",
        ),
        "/static/fonts/OFL.txt" => (
            render::FONT_LICENSE.as_bytes(),
            "text/plain; charset=utf-8",
            "public, max-age=86400",
        ),
        "/favicon.png" => (render::FAVICON, "image/png", "public, max-age=3600"),
        "/apple-touch-icon.png" => (
            render::APPLE_TOUCH_ICON,
            "image/png",
            "public, max-age=3600",
        ),
        "/icon-192.png" => (render::PWA_ICON_192, "image/png", "public, max-age=3600"),
        "/icon-512.png" => (render::PWA_ICON_512, "image/png", "public, max-age=3600"),
        "/static/icons/scry.svg" => (
            include_bytes!("../../memory-engine-api-render/assets/icons/scry.svg"),
            "image/svg+xml",
            "public, max-age=3600",
        ),
        "/manifest.webmanifest" => (
            MANIFEST.as_bytes(),
            "application/manifest+json",
            "public, max-age=3600",
        ),
        "/v1/openapi.json" => (
            include_bytes!("../../../docs/api/openapi.v1.json"),
            "application/json",
            "public, max-age=3600",
        ),
        _ => return Ok(None),
    };
    let mut response = if req.method() == Method::Head {
        Response::empty()?
    } else {
        Response::from_bytes(body.to_vec())?
    };
    response.headers_mut().set("content-type", content_type)?;
    response
        .headers_mut()
        .set("content-length", &body.len().to_string())?;
    response.headers_mut().set("cache-control", cache)?;
    response
        .headers_mut()
        .set("x-content-type-options", "nosniff")?;
    if path == "/sw.js" {
        response.headers_mut().set("service-worker-allowed", "/")?;
    }
    Ok(Some(response))
}

const MANIFEST: &str = r##"{
  "name":"Scry","short_name":"Scry","description":"Remember everything",
  "start_url":"/","scope":"/","display":"standalone",
  "background_color":"#f4f8fa","theme_color":"#f4f8fa",
  "icons":[
    {"src":"/icon-192.png","sizes":"192x192","type":"image/png","purpose":"any maskable"},
    {"src":"/icon-512.png","sizes":"512x512","type":"image/png","purpose":"any maskable"}
  ]
}"##;

/// Auth owns its routes; every other response shares the same no-store boundary.
pub async fn handle(req: &mut Request, db: &Database, env: &Env) -> AppResult<Response> {
    let result = dispatch(req, db, env).await;
    let response = match result {
        Ok(response) => response,
        Err(error) if req.path() == "/" || req.path().starts_with("/app/") => {
            if req.path() == "/app/performance/submit" {
                json_response(&json!({"error": error.message}), error.status)?
            } else {
                browser_failure(&error)?
            }
        }
        Err(error) => json_response(&json!({"error": error.message}), error.status)?,
    };
    let response = if req.method() == Method::Head {
        Response::empty()?
            .with_status(response.status_code())
            .with_headers(response.headers().clone())
    } else {
        response
    };
    no_store(response)
}

async fn dispatch(req: &mut Request, db: &Database, env: &Env) -> AppResult<Response> {
    if let Some(response) = auth::handle(req, db, env).await? {
        return Ok(response);
    }
    if let Some(response) = telemetry::handle(req, db, env).await? {
        return Ok(response);
    }
    let path = req.path();
    if path == "/healthz" || path == "/readyz" {
        require_method(req, &Method::Get)?;
        db.query::<Value>("SELECT 1 AS ready", &[])?;
        return json_response(
            &json!({
                "status": if path == "/readyz" { "ready" } else { "ok" },
                "service": "memory-engine-api", "storage": "durable-object-sqlite",
                "sqlite": true, "postgres": false,
                "schedulerMode": "durable_alarm"
            }),
            200,
        );
    }
    if path == "/" || path.starts_with("/app/") {
        return browser_route(req, db, env).await;
    }
    // The unversioned compatibility surface remains mounted: external consumers
    // cannot be exhaustively proven absent from the repository alone.
    if path.starts_with("/v1/accounts/") || path.starts_with("/accounts/") {
        return api_route(req, db, env).await;
    }
    Err(Failure::not_found("Route not found."))
}

pub(crate) fn no_store(mut response: Response) -> AppResult<Response> {
    response.headers_mut().set("cache-control", "no-store")?;
    response
        .headers_mut()
        .set("referrer-policy", "no-referrer")?;
    response
        .headers_mut()
        .set("x-content-type-options", "nosniff")?;
    response.headers_mut().set("x-frame-options", "DENY")?;
    Ok(response)
}

pub(crate) fn json_response(value: &impl Serialize, status: u16) -> AppResult<Response> {
    Ok(Response::from_json(value)?.with_status(status))
}

fn html_response(html: String, status: u16) -> AppResult<Response> {
    Ok(Response::from_html(html)?.with_status(status))
}

fn browser_response(
    req: &Request,
    account: &AppAccount,
    mut response: Response,
) -> AppResult<Response> {
    auth::refresh_browser_cookie(req, account, &mut response)?;
    no_store(response)
}

fn browser_failure(error: &Failure) -> AppResult<Response> {
    let html = if error.status == 401 {
        render::render_auth_recovery(
            "Your session expired",
            "Your study data is safe. Sign in again to continue where you left off.",
        )
    } else if error.status == 403 {
        render::render_auth_recovery("Request not authorized", "This request could not be authorized. Reload the app or sign in again before retrying.")
    } else {
        render::render_submit_recovery("Request not completed", &error.message)
    };
    html_response(html, error.status)
}

fn require_method(req: &Request, method: &Method) -> AppResult<()> {
    if req.method() == *method || (*method == Method::Get && req.method() == Method::Head) {
        Ok(())
    } else {
        Err(Failure::new(405, "Method not allowed."))
    }
}

/// Bound the stream before deserialization, including requests without Content-Length.
pub(crate) async fn body_bytes(req: &mut Request, limit: usize) -> AppResult<Vec<u8>> {
    if req.headers().get("content-length")?.is_some_and(|length| {
        length
            .parse::<usize>()
            .map_or(true, |length| length > limit)
    }) {
        return Err(Failure::new(413, "Request body is too large."));
    }
    let receive = async {
        let mut stream = req
            .stream()
            .map_err(|_| Failure::bad_request("Request body is missing."))?;
        let mut bytes = Vec::new();
        while let Some(chunk) = stream.next().await {
            let chunk =
                chunk.map_err(|_| Failure::bad_request("Request body could not be read."))?;
            if bytes.len().saturating_add(chunk.len()) > limit {
                return Err(Failure::new(413, "Request body is too large."));
            }
            bytes.extend_from_slice(&chunk);
        }
        Ok(bytes)
    };
    let deadline = Delay::from(Duration::from_secs(30));
    futures_util::pin_mut!(receive, deadline);
    match futures_util::future::select(receive, deadline).await {
        futures_util::future::Either::Left((result, _)) => result,
        futures_util::future::Either::Right(_) => {
            Err(Failure::new(408, "Request body deadline exceeded."))
        }
    }
}

pub(crate) async fn json_body<T: DeserializeOwned>(
    req: &mut Request,
    limit: usize,
) -> AppResult<T> {
    let content_type = req.headers().get("content-type")?.unwrap_or_default();
    let media_type = content_type.split(';').next().unwrap_or_default().trim();
    if media_type != "application/json"
        && !(media_type.starts_with("application/") && media_type.ends_with("+json"))
    {
        return Err(Failure::new(
            415,
            "Expected an application/json request body.",
        ));
    }
    let bytes = body_bytes(req, limit).await?;
    serde_json::from_slice(&bytes).map_err(|error| {
        let status = if error.is_data() { 422 } else { 400 };
        Failure::new(status, "JSON request body is invalid.")
    })
}

#[derive(Default)]
struct Form(BTreeMap<String, String>);

impl Form {
    async fn read(req: &mut Request) -> AppResult<Self> {
        let content_type = req.headers().get("content-type")?.unwrap_or_default();
        if content_type.split(';').next().unwrap_or_default().trim()
            != "application/x-www-form-urlencoded"
        {
            return Err(Failure::new(415, "Expected a form-encoded request body."));
        }
        let pairs: Vec<(String, String)> =
            serde_urlencoded::from_bytes(&body_bytes(req, MAX_REQUEST_BYTES).await?)
                .map_err(|_| Failure::new(422, "Form fields could not be read."))?;
        let mut fields = BTreeMap::new();
        for (key, value) in pairs {
            if fields.insert(key, value).is_some() {
                return Err(Failure::new(422, "Form fields must not be repeated."));
            }
        }
        Ok(Self(fields))
    }

    fn get(&self, key: &str) -> Option<&str> {
        self.0.get(key).map(String::as_str)
    }

    fn required(&self, key: &str) -> AppResult<&str> {
        self.get(key)
            .ok_or_else(|| Failure::new(422, "A required form field is missing."))
    }

    fn csrf(&self) -> &str {
        self.get("csrfToken").unwrap_or_default().trim()
    }

    fn capture(&self) -> CreateSourceRequest {
        let body = self
            .get("capture")
            .or_else(|| self.get("body"))
            .unwrap_or_default()
            .to_owned();
        let title = self
            .get("title")
            .filter(|title| !title.trim().is_empty())
            .map_or_else(|| infer_capture_title(&body), str::to_owned);
        CreateSourceRequest {
            title,
            body,
            permission: SourcePermission::ModelEligible,
        }
    }
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct GenerationJobResource {
    id: String,
    source_id: String,
    title: String,
    status: JobStatus,
    card_count: usize,
    attempts: u32,
    retryable: bool,
    error: Option<String>,
    created_at: i64,
    updated_at: i64,
}

impl From<GenerationJob> for GenerationJobResource {
    fn from(job: GenerationJob) -> Self {
        Self {
            id: job.id,
            source_id: job.source_id,
            title: job.title,
            status: job.status,
            card_count: job.card_count,
            attempts: job.attempts,
            retryable: job.retryable,
            error: job.error,
            created_at: job.created_at,
            updated_at: job.updated_at,
        }
    }
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct EnqueuedGenerationJobResource {
    #[serde(flatten)]
    job: GenerationJobResource,
    coalesced: bool,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct EditDraftRequest {
    prompt: String,
    expected_answer: String,
    #[serde(default)]
    choices: Vec<String>,
}

#[derive(Deserialize)]
struct UpdateSourcePermissionRequest {
    permission: SourcePermission,
}

async fn api_route(req: &mut Request, db: &Database, env: &Env) -> AppResult<Response> {
    let path = req.path();
    let versioned = path.starts_with("/v1/");
    let normalized = path.strip_prefix("/v1").unwrap_or(&path);
    let segments = normalized
        .split('/')
        .skip(1)
        .map(decode_segment)
        .collect::<AppResult<Vec<_>>>()?;
    let parts = segments.iter().map(String::as_str).collect::<Vec<_>>();
    let ["accounts", account_id, rest @ ..] = parts.as_slice() else {
        return Err(Failure::not_found("Route not found."));
    };
    // Parse a body before authorization, then authorize immediately before its
    // synchronous mutation. Awaiting an upload must not retain a revoked grant.
    match rest {
        ["sources" | "project-decks" | "generation-jobs", ..] => {
            api_source_route(req, db, env, account_id, rest).await
        }
        _ => api_study_route(req, db, env, account_id, rest, versioned).await,
    }
}

async fn api_source_route(
    req: &mut Request,
    db: &Database,
    env: &Env,
    account_id: &str,
    rest: &[&str],
) -> AppResult<Response> {
    match rest {
        ["sources"] if matches!(req.method(), Method::Get | Method::Head) => {
            auth::api_account(req, db, env, account_id)?;
            json_response(
                &SourceList {
                    sources: sources(db, account_id)?,
                },
                200,
            )
        }
        ["sources"] => {
            require_method(req, &Method::Post)?;
            let request: CreateSourceRequest = json_body(req, MAX_REQUEST_BYTES).await?;
            auth::api_account(req, db, env, account_id)?;
            json_response(&save_source(db, account_id, &request)?, 201)
        }
        ["sources", source_id] if req.method() == Method::Delete => {
            auth::api_account(req, db, env, account_id)?;
            archive_source(db, account_id, source_id)?;
            Ok(Response::empty()?.with_status(204))
        }
        ["sources", source_id] => {
            require_method(req, &Method::Patch)?;
            let request: UpdateSourcePermissionRequest = json_body(req, MAX_REQUEST_BYTES).await?;
            auth::api_account(req, db, env, account_id)?;
            change_permission(db, account_id, source_id, request.permission)?;
            Ok(Response::empty()?.with_status(204))
        }
        ["project-decks"] => {
            require_method(req, &Method::Post)?;
            let request: CreateProjectDeckRequest = json_body(req, MAX_REQUEST_BYTES).await?;
            auth::api_account(req, db, env, account_id)?;
            json_response(&create_project_deck(db, account_id, &request)?, 201)
        }
        ["project-decks", deck_id, "invalidate"] => {
            require_method(req, &Method::Post)?;
            let request: InvalidateProjectDeckRequest = json_body(req, MAX_REQUEST_BYTES).await?;
            auth::api_account(req, db, env, account_id)?;
            required_text(&request.event, "Invalidation event")?;
            let view = with_study(db, account_id, false, |study| {
                if !sources(db, account_id)?
                    .iter()
                    .any(|source| source.source_id == *deck_id && source.project_key.is_some())
                {
                    return Err(Failure::not_found("Project deck not found."));
                }
                study
                    .invalidate_project_deck(deck_id, now_ms())
                    .map(StudyViewResponse::from_view)
                    .map_err(study_failure)
            })?;
            json_response(&view, 200)
        }
        ["sources", source_id, "generate"] => {
            require_method(req, &Method::Post)?;
            auth::api_account(req, db, env, account_id)?;
            json_response(
                &jobs::generate_now(db, env, account_id, source_id).await?,
                200,
            )
        }
        ["sources", source_id, "generation-jobs"] => {
            require_method(req, &Method::Post)?;
            auth::api_account(req, db, env, account_id)?;
            let (job, coalesced) = jobs::enqueue(db, env, account_id, source_id)?;
            json_response(
                &EnqueuedGenerationJobResource {
                    job: job.into(),
                    coalesced,
                },
                if coalesced { 200 } else { 202 },
            )
        }
        ["generation-jobs", job_id] => {
            require_method(req, &Method::Get)?;
            auth::api_account(req, db, env, account_id)?;
            json_response(
                &GenerationJobResource::from(jobs::get(db, account_id, job_id)?),
                200,
            )
        }
        _ => Err(Failure::not_found("Route not found.")),
    }
}

async fn api_study_route(
    req: &mut Request,
    db: &Database,
    env: &Env,
    account_id: &str,
    rest: &[&str],
    versioned: bool,
) -> AppResult<Response> {
    match rest {
        ["drafts", draft_id, action @ ("keep" | "edit" | "reject")] => {
            require_method(req, &Method::Post)?;
            let edit: Option<EditDraftRequest> = if *action == "edit" {
                Some(json_body(req, MAX_REQUEST_BYTES).await?)
            } else {
                None
            };
            auth::api_account(req, db, env, account_id)?;
            json_response(
                &draft_action(db, account_id, draft_id, action, edit.as_ref())?,
                200,
            )
        }
        ["review", "next"] => {
            require_method(
                req,
                if versioned {
                    &Method::Post
                } else {
                    &Method::Get
                },
            )?;
            auth::api_account(req, db, env, account_id)?;
            json_response(&next_review(db, account_id)?, 200)
        }
        ["review", review_unit_id, "submit"] => {
            require_method(req, &Method::Post)?;
            let request: SubmitReviewRequest = json_body(req, MAX_REQUEST_BYTES).await?;
            auth::api_account(req, db, env, account_id)?;
            json_response(
                &submit_review(db, account_id, review_unit_id, &request)?,
                200,
            )
        }
        ["review", review_unit_id, "content-feedback"] => {
            require_method(req, &Method::Post)?;
            let request: ContentFeedbackRequest = json_body(req, MAX_REQUEST_BYTES).await?;
            auth::api_account(req, db, env, account_id)?;
            json_response(&feedback(db, account_id, review_unit_id, request)?, 200)
        }
        ["review", review_unit_id, "reference"] => {
            require_method(req, &Method::Post)?;
            auth::api_account(req, db, env, account_id)?;
            json_response(
                &provider::reference(db, env, account_id, review_unit_id).await?,
                200,
            )
        }
        ["review", review_unit_id, "bridge"] => {
            require_method(req, &Method::Post)?;
            auth::api_account(req, db, env, account_id)?;
            json_response(
                &provider::bridge(db, env, account_id, review_unit_id).await?,
                200,
            )
        }
        ["review", review_unit_id, action @ ("reveal" | "skip" | "snooze" | "snooze-concept")] => {
            require_method(req, &Method::Post)?;
            auth::api_account(req, db, env, account_id)?;
            json_response(&review_action(db, account_id, review_unit_id, action)?, 200)
        }
        _ => Err(Failure::not_found("Route not found.")),
    }
}

fn decode_segment(segment: &str) -> AppResult<String> {
    if !segment.contains('%') {
        return Ok(segment.to_owned());
    }
    let mut decoded = Vec::with_capacity(segment.len());
    let mut input = segment.as_bytes().iter().copied();
    while let Some(byte) = input.next() {
        if byte == b'%' {
            let high = input
                .next()
                .and_then(|value| char::from(value).to_digit(16));
            let low = input
                .next()
                .and_then(|value| char::from(value).to_digit(16));
            match (high, low) {
                (Some(high), Some(low)) => decoded.push(
                    u8::try_from(high * 16 + low)
                        .map_err(|_| Failure::bad_request("Path encoding is invalid."))?,
                ),
                _ => return Err(Failure::bad_request("Path encoding is invalid.")),
            }
        } else {
            decoded.push(byte);
        }
    }
    String::from_utf8(decoded).map_err(|_| Failure::bad_request("Path encoding is invalid."))
}

fn required_text<'a>(text: &'a str, label: &str) -> AppResult<&'a str> {
    let text = text.trim();
    if text.is_empty() {
        return Err(Failure::bad_request(format!("{label} must not be blank.")));
    }
    if label.contains("body") && text.len() > MAX_SOURCE_BODY_BYTES {
        return Err(Failure::new(
            413,
            "Source body exceeds the 256 KiB generation limit.",
        ));
    }
    if label.contains("title") && text.len() > MAX_SOURCE_TITLE_BYTES {
        return Err(Failure::new(
            413,
            "Source title exceeds the 512 byte limit.",
        ));
    }
    Ok(text)
}

fn stable_id(prefix: &str, fields: &[&str]) -> String {
    let hash = fields
        .iter()
        .flat_map(|field| field.bytes())
        .fold(0xcbf2_9ce4_8422_2325_u64, |hash, byte| {
            (hash ^ u64::from(byte)).wrapping_mul(0x0000_0100_0000_01b3)
        });
    format!("{prefix}_{hash:016x}")
}

fn sources(db: &Database, account_id: &str) -> AppResult<Vec<SourceRecord>> {
    Ok(SqlStudyStore::new(db.clone(), account_id)
        .source_documents()?
        .into_iter()
        .filter(|source| source.archived_at.is_none())
        .map(|source| SourceRecord {
            source_id: source.id,
            title: source.title,
            body: source.body.unwrap_or_default(),
            permission: source.permission,
            project_key: source.project_key,
            ttl_expires_at: source.ttl_expires_at,
        })
        .collect())
}

fn save_source(
    db: &Database,
    account_id: &str,
    request: &CreateSourceRequest,
) -> AppResult<SourceRecord> {
    let title = required_text(&request.title, "Source title")?;
    let body = required_text(&request.body, "Source body")?;
    let source = SourceRecord {
        source_id: stable_id("src", &[account_id, title, body]),
        title: title.to_owned(),
        body: body.to_owned(),
        permission: request.permission.clone(),
        project_key: None,
        ttl_expires_at: None,
    };
    persist_source(db, account_id, &source)?;
    Ok(source)
}

fn create_project_deck(
    db: &Database,
    account_id: &str,
    request: &CreateProjectDeckRequest,
) -> AppResult<ProjectDeckRecord> {
    let project_key = required_text(&request.project_key, "Project key")?;
    let title = required_text(&request.title, "Project deck title")?;
    let body = required_text(&request.body, "Project deck body")?;
    let source = SourceRecord {
        source_id: stable_id("deck", &[account_id, project_key, title, body]),
        title: title.to_owned(),
        body: body.to_owned(),
        permission: SourcePermission::ModelEligible,
        project_key: Some(project_key.to_owned()),
        ttl_expires_at: request.ttl_expires_at,
    };
    persist_source(db, account_id, &source)?;
    Ok(ProjectDeckRecord {
        deck_id: source.source_id.clone(),
        project_key: project_key.to_owned(),
        source,
    })
}

fn persist_source(db: &Database, account_id: &str, source: &SourceRecord) -> AppResult<()> {
    with_study(db, account_id, false, |study| {
        study
            .add_source(BetaStudySourceInput {
                id: source.source_id.clone(),
                title: source.title.clone(),
                body: source.body.clone(),
                project_key: source.project_key.clone(),
                ttl_expires_at: source.ttl_expires_at,
                permission: source.permission.clone(),
            })
            .map(drop)
            .map_err(study_failure)
    })
}

fn with_study<T>(
    db: &Database,
    account_id: &str,
    focused: bool,
    operation: impl FnOnce(&mut Study) -> AppResult<T>,
) -> AppResult<T> {
    db.transaction(|| {
        let store = SqlStudyStore::new(db.clone(), account_id);
        let mut study = if focused {
            Study::for_review(store, now_ms)
        } else {
            Study::from_store(store, now_ms)
        };
        operation(&mut study)
    })
}

pub(crate) fn study_failure(error: BetaStudyError<Failure>) -> Failure {
    match error {
        BetaStudyError::Store(error)
        | BetaStudyError::Service(ServiceError::Store(error))
        | BetaStudyError::Generation(BetaGenerationError::Store(error)) => error,
        BetaStudyError::NoActiveReviewUnit
        | BetaStudyError::Generation(BetaGenerationError::UnknownReviewUnit(_)) => {
            Failure::not_found("Review unit not found.")
        }
        BetaStudyError::NoConceptKey => {
            Failure::bad_request("The active review unit must have a nonblank concept key.")
        }
        BetaStudyError::Generation(
            BetaGenerationError::UnknownSourceDocument(_)
            | BetaGenerationError::ArchivedSourceDocument(_),
        ) => Failure::not_found("Source not found."),
        BetaStudyError::Generation(BetaGenerationError::LocalOnlySource(_)) => {
            Failure::forbidden("Local-only source cannot be sent to a model provider.")
        }
        BetaStudyError::Generation(BetaGenerationError::SourceDocumentHasNoTextBody(_)) => {
            Failure::bad_request("Source document has no text body.")
        }
        BetaStudyError::Generation(BetaGenerationError::ProviderFailure(_)) => {
            Failure::new(502, "Model generation could not be completed.")
        }
        BetaStudyError::UnknownReferenceSpan(_) => {
            Failure::conflict("The source reference is no longer available.")
        }
        BetaStudyError::Service(ServiceError::Scheduler(_)) => {
            Failure::internal("Review scheduling failed.")
        }
    }
}

fn archive_source(
    db: &Database,
    account_id: &str,
    source_id: &str,
) -> AppResult<(StudyViewResponse, usize)> {
    with_study(db, account_id, false, |study| {
        study
            .archive_source(source_id)
            .map(|(view, count)| (StudyViewResponse::from_view(view), count))
            .map_err(study_failure)
    })
}

fn change_permission(
    db: &Database,
    account_id: &str,
    source_id: &str,
    permission: SourcePermission,
) -> AppResult<()> {
    with_study(db, account_id, false, |study| {
        study
            .update_source_permission(source_id, permission)
            .map(drop)
            .map_err(study_failure)
    })
}

fn draft_action(
    db: &Database,
    account_id: &str,
    draft_id: &str,
    action: &str,
    edit: Option<&EditDraftRequest>,
) -> AppResult<StudyViewResponse> {
    with_study(db, account_id, false, |study| {
        let result = match action {
            "keep" => study.keep_draft(draft_id),
            "reject" => study.reject_draft(draft_id),
            "edit" => {
                let edit =
                    edit.ok_or_else(|| Failure::bad_request("Draft edit fields are missing."))?;
                let prompt = required_text(&edit.prompt, "Learner prompt")?;
                let answer = required_text(&edit.expected_answer, "Learner expected answer")?;
                study.edit_and_keep_draft(draft_id, prompt, answer, &edit.choices)
            }
            _ => return Err(Failure::not_found("Draft action not found.")),
        };
        result
            .map(StudyViewResponse::from_view)
            .map_err(study_failure)
    })
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase", tag = "state")]
enum ActiveGradedReview {
    Active {
        review_unit_id: String,
        idempotency_key: String,
        view: Box<StudyViewResponse>,
    },
    Consumed {
        idempotency_key: String,
    },
}

fn graded_hold(store: &SqlStudyStore) -> AppResult<Option<ActiveGradedReview>> {
    store
        .active_graded_review()?
        .map(serde_json::from_value)
        .transpose()
        .map_err(|_| Failure::internal("Saved review state could not be read."))
}

fn save_graded_hold(
    store: &mut SqlStudyStore,
    review_unit_id: &str,
    idempotency_key: &str,
    view: &StudyViewResponse,
    recovering: bool,
) -> AppResult<()> {
    let value = json!({"state": "active", "review_unit_id": review_unit_id,
        "idempotency_key": idempotency_key, "view": view});
    if !store.save_active_graded_review(&value, idempotency_key, recovering)? {
        return Err(Failure::not_found("Review unit not found."));
    }
    Ok(())
}

fn next_review(db: &Database, account_id: &str) -> AppResult<StudyViewResponse> {
    db.transaction(|| {
        let store = SqlStudyStore::new(db.clone(), account_id);
        let idempotency_key = match graded_hold(&store)? {
            Some(ActiveGradedReview::Active {
                idempotency_key, ..
            }) => Some(idempotency_key),
            Some(ActiveGradedReview::Consumed { .. }) => None,
            None => store.latest_applied_review_idempotency_key()?,
        };
        let mut study = Study::for_review(store, now_ms);
        let view = study
            .start()
            .map(StudyViewResponse::from_view)
            .map_err(study_failure)?;
        if let Some(key) = idempotency_key {
            if !study.into_store().consume_active_graded_review(&key)? {
                return Err(Failure::conflict(
                    "The active review changed before Continue completed.",
                ));
            }
        }
        Ok(view)
    })
}

fn resume_review(
    db: &Database,
    account_id: &str,
    review_unit_id: &str,
) -> AppResult<StudyViewResponse> {
    db.transaction(|| {
        let store = SqlStudyStore::new(db.clone(), account_id);
        if let Some(ActiveGradedReview::Active {
            review_unit_id: saved_id,
            view,
            ..
        }) = graded_hold(&store)?
        {
            if saved_id == review_unit_id {
                return Ok(*view);
            }
            return Err(Failure::conflict(
                "Continue the open graded review before opening another quiz.",
            ));
        }
        Study::for_review(store, now_ms)
            .resume_review(review_unit_id)
            .map(StudyViewResponse::from_view)
            .map_err(study_failure)
    })
}

fn submit_review(
    db: &Database,
    account_id: &str,
    review_unit_id: &str,
    request: &SubmitReviewRequest,
) -> AppResult<StudyViewResponse> {
    let answer = required_text(&request.answer, "Review answer")?;
    let key = required_text(&request.idempotency_key, "Idempotency key")?;
    if request.response_time_ms == 0 {
        return Err(Failure::bad_request(
            "Review response time must be a positive integer.",
        ));
    }
    db.transaction(|| {
        let mut store = SqlStudyStore::new(db.clone(), account_id);
        let saved = graded_hold(&store)?;
        if store.applied_review_idempotency_key_exists(key)? {
            return match saved {
                Some(ActiveGradedReview::Active {
                    review_unit_id: saved_id,
                    idempotency_key,
                    view,
                }) if saved_id == review_unit_id && idempotency_key == key => Ok(*view),
                None => {
                    let mut study = Study::for_review(store, now_ms);
                    if !study
                        .restore_graded_review(review_unit_id, key)
                        .map_err(study_failure)?
                    {
                        return Err(Failure::not_found("Review unit not found."));
                    }
                    let view = study
                        .view()
                        .map(StudyViewResponse::from_view)
                        .map_err(study_failure)?;
                    store = study.into_store();
                    save_graded_hold(&mut store, review_unit_id, key, &view, true)?;
                    Ok(view)
                }
                _ => Err(Failure::not_found("Review unit not found.")),
            };
        }
        if matches!(saved, Some(ActiveGradedReview::Active { .. })) {
            return Err(Failure::conflict(
                "Continue the graded review before submitting another answer.",
            ));
        }
        let mut study = Study::for_review(store, now_ms);
        study.resume_review(review_unit_id).map_err(study_failure)?;
        let view = study
            .submit_answer_with_idempotency_key(answer, request.response_time_ms, Some(key))
            .map(StudyViewResponse::from_view)
            .map_err(study_failure)?;
        save_graded_hold(&mut study.into_store(), review_unit_id, key, &view, false)?;
        Ok(view)
    })
}

fn review_action(
    db: &Database,
    account_id: &str,
    review_unit_id: &str,
    action: &str,
) -> AppResult<StudyViewResponse> {
    with_study(db, account_id, true, |study| {
        if let Some(ActiveGradedReview::Active {
            review_unit_id: saved_id,
            view,
            ..
        }) = graded_hold(&SqlStudyStore::new(db.clone(), account_id))?
        {
            if saved_id == review_unit_id && action == "reveal" {
                return Ok(*view);
            }
            return Err(Failure::conflict(
                "Continue the graded review before changing the review queue.",
            ));
        }
        study.resume_review(review_unit_id).map_err(study_failure)?;
        let result: StudyResult = match action {
            "reveal" => study.reveal(),
            "skip" => study.skip_current(),
            "snooze" => study.snooze_current(),
            "snooze-concept" => study.snooze_current_concept(),
            "delete" => study.archive_current(),
            _ => return Err(Failure::not_found("Review action not found.")),
        };
        result
            .map(StudyViewResponse::from_view)
            .map_err(study_failure)
    })
}

fn edit_review(
    db: &Database,
    account_id: &str,
    review_unit_id: &str,
    prompt: &str,
    answer: &str,
) -> AppResult<StudyViewResponse> {
    let prompt = required_text(prompt, "Review unit prompt")?;
    let answer = required_text(answer, "Review unit expected answer")?;
    db.transaction(|| {
        let store = SqlStudyStore::new(db.clone(), account_id);
        let hold = graded_hold(&store)?;
        let mut study = Study::for_review(store, now_ms);
        let graded_key = if let Some(ActiveGradedReview::Active {
            review_unit_id: saved_id,
            idempotency_key,
            ..
        }) = hold
        {
            if saved_id != review_unit_id {
                return Err(Failure::conflict(
                    "Continue the graded review before editing another quiz.",
                ));
            }
            if !study
                .restore_graded_review(review_unit_id, &idempotency_key)
                .map_err(study_failure)?
            {
                return Err(Failure::not_found("Graded review is no longer available."));
            }
            Some(idempotency_key)
        } else {
            study.resume_review(review_unit_id).map_err(study_failure)?;
            None
        };
        let view = study
            .edit_current_prompt(prompt, answer)
            .map(StudyViewResponse::from_view)
            .map_err(study_failure)?;
        if let Some(key) = graded_key {
            // Update the editable definition, never the historical graded prompt.
            save_graded_hold(&mut study.into_store(), review_unit_id, &key, &view, false)?;
        }
        Ok(view)
    })
}

fn feedback(
    db: &Database,
    account_id: &str,
    review_unit_id: &str,
    request: ContentFeedbackRequest,
) -> AppResult<ContentFeedback> {
    let feedback_id = required_text(&request.idempotency_key, "Idempotency key")?.to_owned();
    db.transaction(|| {
        record_content_feedback(
            &mut SqlStudyStore::new(db.clone(), account_id),
            RecordContentFeedbackCommand {
                feedback_id,
                review_unit_id: ReviewUnitId::new(review_unit_id),
                verdict: request.verdict,
                rationale: request.rationale,
                account_id: account_id.to_owned(),
                occurred_at: now_ms(),
                supersedes_id: request.supersedes_id,
            },
        )
        .map_err(|error| match error {
            ContentFeedbackError::Store(error) => error,
            ContentFeedbackError::BlankFeedbackId => {
                Failure::bad_request("Idempotency key must not be blank.")
            }
            ContentFeedbackError::BlankAccountId => {
                Failure::forbidden("Account authorization is required.")
            }
        })
    })
}

fn study_view(db: &Database, account_id: &str) -> AppResult<StudyViewResponse> {
    with_study(db, account_id, false, |study| {
        study
            .view()
            .map(StudyViewResponse::from_view)
            .map_err(study_failure)
    })
}

/// Shared with auth after a successful sign-in or preference change.
pub fn account_page(
    db: &Database,
    account: &AppAccount,
    notice: Option<&str>,
) -> AppResult<String> {
    let view = study_view(db, account.account_id())?;
    let live_jobs = if notice.is_some_and(|notice| notice.contains("Generating")) {
        jobs::list(db, account.account_id())?
    } else {
        Vec::new()
    };
    Ok(render::render_account_page(
        account,
        Some(&view),
        &live_jobs,
        notice,
    ))
}

fn library_page(
    db: &Database,
    account: &AppAccount,
    view: Option<&StudyViewResponse>,
    notice: Option<&str>,
) -> AppResult<String> {
    let fetched = if view.is_none() {
        Some(study_view(db, account.account_id())?)
    } else {
        None
    };
    let sources = sources(db, account.account_id())?;
    let jobs = jobs::list(db, account.account_id())?;
    Ok(render::render_library_page(
        account,
        &sources,
        view.or(fetched.as_ref()),
        &jobs,
        notice,
    ))
}

fn create_page(db: &Database, account: &AppAccount, notice: Option<&str>) -> AppResult<String> {
    let view = study_view(db, account.account_id())?;
    let jobs = jobs::list(db, account.account_id())?;
    Ok(render::render_create_page(
        account,
        Some(&view),
        &jobs,
        notice,
    ))
}

fn action_result(
    db: &Database,
    account: &AppAccount,
    result: AppResult<StudyViewResponse>,
    notice: Option<&str>,
) -> AppResult<Response> {
    match result {
        Ok(view) => html_response(
            render::render_account_page(account, Some(&view), &[], notice),
            200,
        ),
        Err(error) => {
            // Never describe a failed database read as an empty account.
            let html = match account_page(db, account, Some(&error.message)) {
                Ok(html) => html,
                Err(_) => render::render_submit_recovery("Request not completed", &error.message),
            };
            html_response(html, error.status)
        }
    }
}

async fn browser_route(req: &mut Request, db: &Database, env: &Env) -> AppResult<Response> {
    let path = req.path();
    if matches!(req.method(), Method::Get | Method::Head) {
        return browser_read(req, db, env, &path);
    }
    require_method(req, &Method::Post)?;
    if path == "/app/submit" {
        return browser_submit(req, db, env).await;
    }
    let form = Form::read(req).await?;
    let account = auth::browser_account(req, db, env, Some(form.csrf()))?;
    let response = match path.as_str() {
        "/app/start" | "/app/source" => {
            let result = save_source(db, account.account_id(), &form.capture());
            let (status, notice) = match result {
                Ok(_) => (
                    200,
                    "Capture saved. Create review when you are ready.".to_owned(),
                ),
                Err(error) => (error.status, error.message),
            };
            html_response(library_page(db, &account, None, Some(&notice))?, status)?
        }
        "/app/capture" => capture_source(req, db, env, &account, &form)?,
        "/app/generate" => {
            let result = jobs::enqueue(db, env, account.account_id(), form.required("sourceId")?);
            let (status, notice) = match result {
                Ok((_, false)) => (200, "Generating. Watch the activity log.".to_owned()),
                Ok((_, true)) => (200, "Already generating this source.".to_owned()),
                Err(error) => (error.status, error.message),
            };
            html_response(library_page(db, &account, None, Some(&notice))?, status)?
        }
        "/app/source/archive" => {
            let result = archive_source(db, account.account_id(), form.required("sourceId")?);
            match result {
                Ok((view, count)) => {
                    let noun = if count == 1 { "card" } else { "cards" };
                    let notice = format!("Source removed. {count} {noun} retired.");
                    html_response(library_page(db, &account, Some(&view), Some(&notice))?, 200)?
                }
                Err(error) => html_response(
                    library_page(db, &account, None, Some(&error.message))?,
                    error.status,
                )?,
            }
        }
        "/app/source/permission" => {
            let permission =
                serde_json::from_value(Value::String(form.required("permission")?.to_owned()))
                    .map_err(|_| Failure::new(422, "Source permission is invalid."))?;
            let (status, notice) = match change_permission(
                db,
                account.account_id(),
                form.required("sourceId")?,
                permission,
            ) {
                Ok(()) => (200, "Source permission updated.".to_owned()),
                Err(error) => (error.status, error.message),
            };
            html_response(library_page(db, &account, None, Some(&notice))?, status)?
        }
        "/app/jobs/retry" => {
            let (status, notice) =
                match jobs::retry(db, env, account.account_id(), form.required("jobId")?) {
                    Ok(_) => (
                        200,
                        "Retrying. Generating again in the background.".to_owned(),
                    ),
                    Err(error) => (error.status, error.message),
                };
            html_response(library_page(db, &account, None, Some(&notice))?, status)?
        }
        _ => browser_study_action(db, env, &account, &form, &path).await?,
    };
    browser_response(req, &account, response)
}

async fn browser_study_action(
    db: &Database,
    env: &Env,
    account: &AppAccount,
    form: &Form,
    path: &str,
) -> AppResult<Response> {
    match path {
        "/app/draft/keep" | "/app/draft/edit" | "/app/draft/reject" => {
            let action = path.rsplit('/').next().unwrap_or_default();
            browser_draft_action(db, account, form, action)
        }
        "/app/next" => browser_next_review(db, account),
        "/app/resume" => action_result(
            db,
            account,
            resume_review(db, account.account_id(), form.required("reviewUnitId")?),
            None,
        ),
        "/app/reference" => match provider::reference(
            db,
            env,
            account.account_id(),
            form.required("reviewUnitId")?,
        )
        .await
        {
            Ok(view) => html_response(render::render_reference_page(account, &view), 200),
            Err(error) => action_result(db, account, Err(error), None),
        },
        "/app/bridge" => match provider::bridge(
            db,
            env,
            account.account_id(),
            form.required("reviewUnitId")?,
        )
        .await
        {
            Ok(view) => html_response(library_page(db, account, Some(&view), None)?, 200),
            Err(error) => action_result(db, account, Err(error), None),
        },
        "/app/reveal" | "/app/skip" | "/app/snooze" | "/app/snooze-concept" | "/app/delete" => {
            let action = path.strip_prefix("/app/").unwrap_or_default();
            let notice = match action {
                "skip" => Some(render::SKIP_CONFIRM_NOTICE),
                "snooze" => Some(render::SNOOZE_CONFIRM_NOTICE),
                "snooze-concept" => Some(render::SNOOZE_CONCEPT_CONFIRM_NOTICE),
                _ => None,
            };
            action_result(
                db,
                account,
                review_action(
                    db,
                    account.account_id(),
                    form.required("reviewUnitId")?,
                    action,
                ),
                notice,
            )
        }
        "/app/edit" => {
            let view = resume_review(db, account.account_id(), form.required("reviewUnitId")?)?;
            let jobs = jobs::list(db, account.account_id())?;
            html_response(
                render::render_edit_review_html(account, &view, &jobs, None),
                200,
            )
        }
        "/app/edit/save" => {
            let result = edit_review(
                db,
                account.account_id(),
                form.required("reviewUnitId")?,
                form.required("prompt")?,
                form.required("expectedAnswer")?,
            );
            action_result(db, account, result, None)
        }
        "/app/content-feedback" => browser_feedback(db, account, form),
        _ => Err(Failure::not_found("Route not found.")),
    }
}

fn browser_draft_action(
    db: &Database,
    account: &AppAccount,
    form: &Form,
    action: &str,
) -> AppResult<Response> {
    let edit = if action == "edit" {
        Some(EditDraftRequest {
            prompt: form.required("prompt")?.to_owned(),
            expected_answer: form.required("expectedAnswer")?.to_owned(),
            choices: form
                .get("choices")
                .unwrap_or_default()
                .lines()
                .map(str::trim)
                .filter(|choice| !choice.is_empty())
                .map(str::to_owned)
                .collect(),
        })
    } else {
        None
    };
    action_result(
        db,
        account,
        draft_action(
            db,
            account.account_id(),
            form.required("draftId")?,
            action,
            edit.as_ref(),
        ),
        None,
    )
}

fn browser_next_review(db: &Database, account: &AppAccount) -> AppResult<Response> {
    let started = now_ms();
    let request_id = crate::random_id("req_")?;
    let result = next_review(db, account.account_id());
    let operation_ms = elapsed_ms(started);
    let rendered = now_ms();
    let mut response = action_result(db, account, result, None)?;
    timing_headers(
        &mut response,
        &request_id,
        None,
        elapsed_ms(started),
        Some(operation_ms),
        elapsed_ms(rendered),
    )?;
    Ok(response)
}

fn browser_read(req: &Request, db: &Database, env: &Env, path: &str) -> AppResult<Response> {
    let known = matches!(
        path,
        "/" | "/app/create"
            | "/app/library"
            | "/app/analytics"
            | "/app/jobs/events"
            | "/app/capture"
    ) || path.starts_with("/app/jobs/");
    if !known {
        return Err(Failure::not_found("Route not found."));
    }
    let account = match auth::browser_account(req, db, env, None) {
        Ok(account) => account,
        Err(error)
            if error.status == 401
                && !auth::browser_session_cookie_present(req)
                && matches!(
                    path,
                    "/" | "/app/create" | "/app/library" | "/app/analytics"
                ) =>
        {
            return html_response(render::render_app_shell(None, &[], None, &[], None), 200);
        }
        Err(error) => return Err(error),
    };
    let query: BTreeMap<String, String> = req.url()?.query_pairs().into_owned().collect();
    let response = match path {
        "/" => html_response(account_page(db, &account, None)?, 200)?,
        "/app/create" => html_response(create_page(db, &account, None)?, 200)?,
        "/app/library" => html_response(library_page(db, &account, None, None)?, 200)?,
        "/app/analytics" => {
            let view = study_view(db, account.account_id())?;
            let options = analytics_options(&query)?;
            html_response(render::render_analytics_page(&account, &view, options), 200)?
        }
        "/app/jobs/events" if req.method() == Method::Head => {
            let mut response = Response::empty()?;
            response
                .headers_mut()
                .set("content-type", "text/event-stream")?;
            response
        }
        "/app/jobs/events" => jobs_events(req, db, env, &account)?,
        "/app/capture" => {
            let job_id = query
                .get("jobId")
                .ok_or_else(|| Failure::bad_request("Generation job id is required."))?;
            let job = jobs::get(db, account.account_id(), job_id)?;
            html_response(render::render_capture_waiting_page(&account, &job), 200)?
        }
        _ => {
            let job_id = path
                .strip_prefix("/app/jobs/")
                .ok_or_else(|| Failure::not_found("Route not found."))?;
            let job = jobs::get(db, account.account_id(), &decode_segment(job_id)?)?;
            json_response(&GenerationJobResource::from(job), 200)?
        }
    };
    browser_response(req, &account, response)
}

fn analytics_options(query: &BTreeMap<String, String>) -> AppResult<AnalyticsViewOptions> {
    let page = query
        .get("page")
        .map(|page| page.parse::<usize>())
        .transpose()
        .map_err(|_| Failure::bad_request("Analytics page is invalid."))?
        .unwrap_or(1);
    Ok(AnalyticsViewOptions {
        filter: match query.get("filter").map(String::as_str) {
            Some("at-risk") => AnalyticsConceptFilter::AtRisk,
            Some("struggling") => AnalyticsConceptFilter::Struggling,
            Some("mixed") => AnalyticsConceptFilter::Mixed,
            Some("solid") => AnalyticsConceptFilter::Solid,
            Some("untried") => AnalyticsConceptFilter::Untried,
            _ => AnalyticsConceptFilter::All,
        },
        sort: match query.get("sort").map(String::as_str) {
            Some("name") => AnalyticsConceptSort::Name,
            Some("success") => AnalyticsConceptSort::Success,
            _ => AnalyticsConceptSort::Health,
        },
        page,
    })
}

fn capture_source(
    req: &Request,
    db: &Database,
    env: &Env,
    account: &AppAccount,
    form: &Form,
) -> AppResult<Response> {
    let source = match save_source(db, account.account_id(), &form.capture()) {
        Ok(source) => source,
        Err(error) => {
            return html_response(
                create_page(db, account, Some(&error.message))?,
                error.status,
            )
        }
    };
    match jobs::enqueue(db, env, account.account_id(), &source.source_id) {
        Ok((job, _)) => {
            let mut response = Response::empty()?.with_status(303);
            // POST/Redirect/GET: reload follows this durable job, never a new
            // capture. Only its dedicated waiting render opts into navigation.
            let mut location = req.url()?;
            location.set_path("/app/capture");
            location.set_query(None);
            location.query_pairs_mut().append_pair("jobId", &job.id);
            let relative = format!(
                "{}?{}",
                location.path(),
                location.query().unwrap_or_default()
            );
            response.headers_mut().set("location", &relative)?;
            Ok(response)
        }
        Err(error) => {
            let notice = format!(
                "Your capture was saved, but generation did not start. {}",
                error.message
            );
            html_response(
                library_page(db, account, None, Some(&notice))?,
                error.status,
            )
        }
    }
}

/// Finite SSE snapshots read live SQL, not a process-local broadcast queue.
/// On reconnect the browser receives terminal jobs too, so there is no lost
/// completion race between a capture response and opening `EventSource`.
fn jobs_events(
    req: &Request,
    db: &Database,
    env: &Env,
    account: &AppAccount,
) -> AppResult<Response> {
    let db = db.clone();
    let env = env.clone();
    let request = req.clone()?;
    let account_id = account.account_id().to_owned();
    let state = (
        db,
        env,
        request,
        account_id,
        BTreeMap::<String, (JobStatus, i64)>::new(),
        0_u8,
    );
    let events = stream::unfold(
        state,
        |(db, env, request, account_id, mut seen, round)| async move {
            if round == 25 {
                return None;
            }
            if round != 0 {
                Delay::from(Duration::from_secs(1)).await;
            }
            // Recheck expiry, revocation and invitation before every private frame.
            if auth::browser_account(&request, &db, &env, None)
                .map_or(true, |account| account.account_id() != account_id)
            {
                return None;
            }
            let Ok(jobs) = jobs::list(&db, &account_id) else {
                return None;
            };
            let mut frame = String::new();
            if round == 0 {
                frame.push_str("retry: 1000\n\n");
            }
            for job in jobs {
                let revision = (job.status, job.updated_at);
                if seen.get(&job.id) == Some(&revision) {
                    continue;
                }
                seen.insert(job.id.clone(), revision);
                let Ok(payload) = serde_json::to_string(&job) else {
                    return None;
                };
                frame.push_str("event: job\ndata: ");
                frame.push_str(&payload);
                frame.push_str("\n\n");
            }
            if frame.is_empty() {
                frame.push_str(": ping\n\n");
            }
            Some((
                Ok::<Vec<u8>, worker::Error>(frame.into_bytes()),
                (db, env, request, account_id, seen, round + 1),
            ))
        },
    );
    let mut response = Response::from_stream(events)?;
    response
        .headers_mut()
        .set("content-type", "text/event-stream")?;
    response.headers_mut().set("x-accel-buffering", "no")?;
    no_store(response)
}

fn browser_feedback(db: &Database, account: &AppAccount, form: &Form) -> AppResult<Response> {
    let review_unit_id = form.required("reviewUnitId")?;
    let verdict: ContentFeedbackVerdict =
        serde_json::from_value(Value::String(form.required("verdict")?.to_owned()))
            .map_err(|_| Failure::new(422, "Feedback verdict is invalid."))?;
    let request = ContentFeedbackRequest {
        verdict,
        rationale: form.get("rationale").map(str::to_owned),
        idempotency_key: form.required("idempotencyKey")?.to_owned(),
        supersedes_id: form.get("supersedesId").map(str::to_owned),
    };
    // Grading hold and feedback update are one transaction. A Continue in
    // another tab cannot attach feedback to an already-consumed graded view.
    let result = db.transaction(|| {
        let mut store = SqlStudyStore::new(db.clone(), account.account_id());
        let Some(ActiveGradedReview::Active {
            review_unit_id: saved_id,
            idempotency_key,
            mut view,
        }) = graded_hold(&store)?
        else {
            return Err(Failure::not_found("Graded review is no longer active."));
        };
        if saved_id != review_unit_id
            || view
                .current
                .as_ref()
                .is_none_or(|current| current.grade.is_none())
        {
            return Err(Failure::not_found("Graded review is no longer active."));
        }
        let recorded = feedback(db, account.account_id(), review_unit_id, request.clone())?;
        if let Some(current) = view.current.as_mut() {
            current.content_feedback_head_id = Some(recorded.id);
        }
        save_graded_hold(&mut store, review_unit_id, &idempotency_key, &view, false)?;
        Ok(*view)
    });
    match result {
        Ok(view) => html_response(
            render::render_content_feedback_result_html(
                account,
                &view,
                &[],
                "Saved. This card will help improve future generation.",
            ),
            200,
        ),
        Err(error) if error.status == 404 => browser_failure(&error),
        Err(error) => feedback_recovery(db, account, review_unit_id, request, error),
    }
}

fn feedback_recovery(
    db: &Database,
    account: &AppAccount,
    review_unit_id: &str,
    request: ContentFeedbackRequest,
    error: Failure,
) -> AppResult<Response> {
    let mut status = error.status;
    let mut message = error.message;
    let mut idempotency_key = request.idempotency_key;
    let mut supersedes_id = request.supersedes_id;
    if status == 409 && !idempotency_key.trim().is_empty() {
        match SqlStudyStore::new(db.clone(), account.account_id())
            .content_feedback_head(&ReviewUnitId::new(review_unit_id))
        {
            Ok(head) => {
                idempotency_key = crate::random_id("feedback-retry-")?;
                supersedes_id = head.map(|feedback| feedback.id);
            }
            Err(error) => {
                status = error.status;
                "Feedback was not saved, and the latest revision could not be loaded. Retry when storage is available.".clone_into(&mut message);
            }
        }
    } else if idempotency_key.trim().is_empty() {
        idempotency_key = crate::random_id("feedback-retry-")?;
    }
    let due = study_view(db, account.account_id())?.due_count;
    html_response(
        render::render_content_feedback_recovery_html(
            account,
            due,
            &ContentFeedbackRecovery {
                review_unit_id,
                verdict: match request.verdict {
                    ContentFeedbackVerdict::Kept => "kept",
                    ContentFeedbackVerdict::Dropped => "dropped",
                },
                rationale: request.rationale.as_deref(),
                idempotency_key: &idempotency_key,
                supersedes_id: supersedes_id.as_deref(),
                message: &message,
            },
        ),
        status,
    )
}

pub(crate) fn elapsed_ms(started: i64) -> u64 {
    u64::try_from(now_ms().saturating_sub(started)).unwrap_or_default()
}

fn sanitize_response_time(raw: Option<&str>) -> u32 {
    raw.and_then(|value| value.trim().parse::<u32>().ok())
        .filter(|elapsed| *elapsed > 0)
        .map_or(MAX_RESPONSE_TIME_MS, |elapsed| {
            elapsed.min(MAX_RESPONSE_TIME_MS)
        })
}

fn timing_headers(
    response: &mut Response,
    request_id: &str,
    trace_id: Option<&str>,
    total_ms: u64,
    sql_ms: Option<u64>,
    render_ms: u64,
) -> AppResult<()> {
    let total_ms = total_ms.max(sql_ms.unwrap_or_default().saturating_add(render_ms));
    let mut timing = format!(r#"request;desc="{request_id}""#);
    if let Some(trace_id) = trace_id {
        let _ = write!(timing, r#", handoff;desc="{trace_id}""#);
    }
    let _ = write!(timing, ", total;dur={total_ms}");
    if let Some(sql_ms) = sql_ms {
        let _ = write!(timing, ", sql;dur={sql_ms}");
    }
    let _ = write!(timing, ", render;dur={render_ms}");
    // Do not claim Postgres connection/statement phases on SQLite or invent
    // absent phases. Zero is the observed coarse Worker clock delta; it is not
    // evidence that a synchronous CPU/SQL phase took less than a millisecond.
    response.headers_mut().set("x-request-id", request_id)?;
    response.headers_mut().set("server-timing", &timing)?;
    Ok(())
}

async fn browser_submit(req: &mut Request, db: &Database, env: &Env) -> AppResult<Response> {
    let started = now_ms();
    let request_id = crate::random_id("req_")?;
    let form = Form::read(req).await;
    let mut trace_id = None;
    let mut sql_ms = None;
    let rendered;
    let mut response = match form {
        Err(error) => {
            rendered = now_ms();
            html_response(
                render::render_submit_recovery("Review not submitted", &error.message),
                error.status,
            )?
        }
        Ok(form) => {
            trace_id = form
                .get("performanceTraceId")
                .filter(|id| telemetry::strict_opaque_id(id, "trace_"))
                .map(str::to_owned);
            let sql_started = now_ms();
            let account = auth::browser_account(req, db, env, Some(form.csrf()));
            match account {
                Err(error) => {
                    sql_ms = Some(elapsed_ms(sql_started));
                    rendered = now_ms();
                    browser_failure(&error)?
                }
                Ok(account) => {
                    let result = (|| {
                        submit_review(
                            db,
                            account.account_id(),
                            form.required("reviewUnitId")?,
                            &SubmitReviewRequest {
                                answer: form.required("answer")?.to_owned(),
                                response_time_ms: sanitize_response_time(
                                    form.get("responseTimeMs"),
                                ),
                                idempotency_key: form.required("idempotencyKey")?.to_owned(),
                            },
                        )
                    })();
                    let (view, status, notice) = match result {
                        Ok(view) => (Some(view), 200, None),
                        Err(error) => (
                            study_view(db, account.account_id()).ok(),
                            error.status,
                            Some(error.message),
                        ),
                    };
                    sql_ms = Some(elapsed_ms(sql_started));
                    rendered = now_ms();
                    let html = if view.is_some() {
                        render::render_submit_action_result_html(
                            &account,
                            view.as_ref(),
                            &[],
                            notice.as_deref(),
                            &request_id,
                            trace_id.as_deref(),
                        )
                    } else {
                        render::render_submit_recovery(
                            "Review not submitted",
                            notice
                                .as_deref()
                                .unwrap_or("Study state could not be loaded."),
                        )
                    };
                    browser_response(req, &account, html_response(html, status)?)?
                }
            }
        }
    };
    let render_ms = elapsed_ms(rendered);
    let total_ms = elapsed_ms(started).max(sql_ms.unwrap_or_default().saturating_add(render_ms));
    timing_headers(
        &mut response,
        &request_id,
        trace_id.as_deref(),
        total_ms,
        sql_ms,
        render_ms,
    )?;
    // Admission into the bounded in-isolate aggregate is not remote delivery.
    // Main's request context drains this after the response with waitUntil.
    telemetry::record_submit_server(total_ms, response.status_code());
    no_store(response)
}
