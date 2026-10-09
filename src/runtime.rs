//! Native Rust/WASM Cloudflare entrypoint and singleton SQLite Durable Object.
//! Authentication and all public-to-object trust are enforced here. No model
//! call is performed while a learning transaction is open.
use crate::{engine, model::*, persistence, web};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use js_sys::{Array, Function, Promise, Reflect, Uint8Array};
use serde::Deserialize;
use serde_json::{Value, json};
use std::{
    cell::RefCell,
    collections::{BTreeMap, BTreeSet},
};
use wasm_bindgen::{JsCast, JsValue, closure::Closure};
use wasm_bindgen_futures::JsFuture;
use worker::*;

const MAX_HTTP_BODY: usize = PHOTO_LIMIT + TEXT_LIMIT + 16 * 1024;
const MODEL_RESPONSE_LIMIT: usize = 256 * 1024;
const MODEL_TIMEOUT_MS: i64 = 55_000;
const RECOVERY_MS: i64 = 90_000;
const INTERNAL_HEADER: &str = "x-scry-internal-key";
thread_local! {
    // Public keys only, scoped to an issuer and refreshed after five minutes.
    static ACCESS_KEYS: RefCell<BTreeMap<String, (i64, Value)>> = const { RefCell::new(BTreeMap::new()) };
}
fn js_json(value: &Value) -> Result<JsValue> {
    js_sys::JSON::parse(&serde_json::to_string(value)?).map_err(Error::from)
}

fn now() -> i64 {
    js_sys::Date::now() as i64
}
fn random_id() -> Result<String> {
    let crypto = Reflect::get(&js_sys::global(), &"crypto".into())?;
    let array = Uint8Array::new_with_length(24);
    call(&crypto, "getRandomValues", &[array.clone().into()])?;
    Ok(hex::encode(array.to_vec()))
}
fn call(target: &JsValue, method: &str, args: &[JsValue]) -> Result<JsValue> {
    let f: Function = Reflect::get(target, &method.into())?.dyn_into()?;
    let a = Array::new();
    for arg in args {
        a.push(arg);
    }
    f.apply(target, &a).map_err(Error::from)
}
fn variable(env: &Env, name: &str) -> String {
    env.var(name).map(|v| v.to_string()).unwrap_or_default()
}
fn secret(env: &Env, name: &str) -> String {
    env.secret(name).map(|v| v.to_string()).unwrap_or_default()
}
fn development(env: &Env) -> bool {
    variable(env, "SCRY_ENV") == "development"
}
fn isolated(env: &Env) -> bool {
    development(env) || variable(env, "SCRY_ENV") == "restore"
}
fn internal_key(env: &Env) -> String {
    secret(env, "INTERNAL_KEY")
}
fn space_name(env: &Env) -> Result<String> {
    let name = variable(env, "SPACE_NAME");
    if name.is_empty()
        || name.len() > 128
        || !name
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || "-_".contains(c))
    {
        return Err(Error::RustError(
            "A fresh explicit learning-space name is required.".into(),
        ));
    }
    Ok(name)
}
fn provider_secrets_present(env: &Env) -> bool {
    ["OPENROUTER_API_KEY", "JEV_API_KEY", "OPERATOR_KEY"]
        .iter()
        .any(|key| !secret(env, key).is_empty())
}
fn app_error(e: AppError) -> Result<Response> {
    private(Response::error(e.message, e.status)?)
}
fn view_query(env: &Env, fields: &BTreeMap<String, String>) -> Result<BTreeMap<String, String>> {
    let mut query: BTreeMap<_, _> = fields
        .iter()
        .filter(|(k, _)| !k.starts_with("__"))
        .map(|(k, v)| (k.clone(), v.clone()))
        .collect();
    query.insert("__nonce".into(), random_id()?);
    if development(env) {
        query.insert("__synthetic".into(), "true".into());
    }
    Ok(query)
}
fn observe_read(app: &mut App, path: &str, timestamp: i64) -> bool {
    let mut concepts = BTreeSet::new();
    if path == "/export" && app.protected_goal().is_none() {
        concepts.extend(app.concepts.keys().cloned());
    } else if path == "/history" {
        concepts.extend(
            app.events
                .iter()
                .filter(|event| !app.protected_concept(&event.concept_id))
                .map(|event| event.concept_id.clone()),
        );
    } else if let Some(id) = path.strip_prefix("/concepts/") {
        if !app.protected_concept(id) {
            concepts.insert(id.into());
        }
    } else if let Some(id) = path
        .strip_prefix("/goals/")
        .or_else(|| path.strip_prefix("/photos/"))
    {
        if app.protected_goal() != Some(id)
            && let Some(goal) = app.goals.get(id)
        {
            concepts.extend(goal.concept_ids.iter().filter_map(|id| {
                app.concepts
                    .get(id)
                    .filter(|concept| !concept.archived && !app.protected_concept(id))
                    .map(|concept| concept.id.clone())
            }));
        }
    } else if let Some(id) = path
        .strip_prefix("/questions/")
        .and_then(|id| id.strip_suffix("/edit"))
        && let Some(question) = app.questions.get(id)
        && !app.protected_concept(&question.concept_id)
    {
        concepts.insert(question.concept_id.clone());
    }
    let mut changed = false;
    for id in concepts {
        if let Some(concept) = app.concepts.get_mut(&id)
            && concept.exposure_ms != Some(timestamp)
        {
            // This affects future presentations only. The occurrence's saved
            // assistance and submitted answer remain unchanged while checking.
            concept.exposure_ms = Some(timestamp);
            changed = true;
        }
    }
    changed
}
fn render_error(
    env: &Env,
    app: &App,
    path: &str,
    fields: &BTreeMap<String, String>,
    e: AppError,
) -> Result<Response> {
    let view_path = if matches!(path, "/create" | "/add") || path.ends_with("/edit") {
        path
    } else {
        "/"
    };
    let query = view_query(env, fields)?;
    private(
        Response::from_html(web::render(app, view_path, &query, now(), Some(&e.message)))?
            .with_status(e.status),
    )
}

fn private(mut response: Response) -> Result<Response> {
    let fresh: Headers = response.headers().entries().collect();
    response = response.with_headers(fresh);
    let headers = response.headers_mut();
    headers.set("Cache-Control", "private, no-store, max-age=0")?;
    headers.set("Pragma", "no-cache")?;
    headers.set("Vary", "Cf-Access-Jwt-Assertion")?;
    headers.set("X-Content-Type-Options", "nosniff")?;
    // Native same-origin forms need the browser's exact Origin value for CSRF
    // enforcement. Same-origin keeps it while withholding external referrers.
    headers.set("Referrer-Policy", "same-origin")?;
    headers.set("X-Frame-Options", "DENY")?;
    headers.set(
        "Permissions-Policy",
        "camera=(), microphone=(self), geolocation=()",
    )?;
    headers.set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'")?;
    Ok(response)
}
fn redirect(location: &str) -> Result<Response> {
    let mut r = Response::empty()?.with_status(303);
    r.headers_mut().set("Location", location)?;
    private(r)
}
fn loopback(url: &Url) -> bool {
    url.scheme() == "http"
        && matches!(
            url.host_str(),
            Some("127.0.0.1" | "localhost" | "[::1]" | "::1")
        )
}
fn configured_origin(env: &Env) -> Result<Url> {
    let raw = variable(env, "CANONICAL_ORIGIN");
    let url = Url::parse(&raw)
        .map_err(|_| Error::RustError("Private ingress is not configured.".into()))?;
    if url.path() != "/"
        || url.query().is_some()
        || url.fragment().is_some()
        || !url.username().is_empty()
        || url.password().is_some()
        || (!development(env)
            && !(variable(env, "SCRY_ENV") == "restore" && loopback(&url))
            && url.scheme() != "https")
    {
        return Err(Error::RustError(
            "Private ingress is not configured.".into(),
        ));
    }
    Ok(url)
}

#[event(fetch)]
pub async fn fetch(req: Request, env: Env, _ctx: Context) -> Result<Response> {
    // Liveness carries no data, authentication, provider, or recovery claims.
    if req.path() == "/healthz" && req.method() == Method::Get {
        return private(Response::ok("ok")?);
    }
    let url = req.url()?;
    let canonical = match configured_origin(&env) {
        Ok(v) => v,
        Err(_) => return private(Response::error("Private ingress is not configured.", 503)?),
    };
    let request_origin = url.origin().ascii_serialization();
    let canonical_origin = canonical.origin().ascii_serialization();
    let host = req.headers().get("Host")?.unwrap_or_default();
    let url_host = url.host_str().unwrap_or_default();
    let expected_host = match url.port() {
        Some(p) => format!("{url_host}:{p}"),
        None => url_host.to_string(),
    };
    if host != expected_host {
        return private(Response::error("Invalid host.", 421)?);
    }
    let method = req.method();
    let read = matches!(method, Method::Get | Method::Head);
    if request_origin != canonical_origin {
        let allowed = variable(&env, "ALTERNATE_HOSTS")
            .split(',')
            .any(|x| !x.trim().is_empty() && x.trim() == url_host);
        if allowed && read && url.scheme() == "https" {
            let location = format!(
                "{}{}{}",
                canonical_origin,
                url.path(),
                url.query().map(|q| format!("?{q}")).unwrap_or_default()
            );
            return redirect(&location);
        }
        return private(Response::error("Use the canonical origin.", 421)?);
    }
    if req.path().starts_with("/__internal") {
        return private(Response::error("Not found.", 404)?);
    }
    let mode = variable(&env, "SCRY_ENV");
    if development(&env) {
        if !loopback(&url)
            || provider_secrets_present(&env)
            || !variable(&env, "ACCESS_TEAM").is_empty()
        {
            return private(Response::error(
                "Development requires loopback and isolated capabilities.",
                503,
            )?);
        }
    } else if mode == "restore" {
        if provider_secrets_present(&env)
            || req.path() != "/operator/restore"
            || method != Method::Post
        {
            return private(Response::error(
                "Restore is an isolated operator-only environment.",
                403,
            )?);
        }
        let token = secret(&env, "RESTORE_TOKEN");
        if token.len() < 32
            || req.headers().get("x-scry-restore-token")?.as_deref() != Some(token.as_str())
        {
            return private(Response::error("Restore authorization required.", 403)?);
        }
    } else {
        if mode != "production" && mode != "preview" {
            return private(Response::error("Private ingress is not configured.", 503)?);
        }
        if validate_access(&req, &env).await.is_err() {
            return private(Response::error("Owner authorization required.", 401)?);
        }
    }
    if !read && mode != "restore" {
        let origin = req.headers().get("Origin")?.unwrap_or_default();
        let cross_site = req
            .headers()
            .get("Sec-Fetch-Site")?
            .is_some_and(|v| v == "cross-site");
        if origin != canonical_origin || cross_site {
            return private(Response::error(
                "Reload from the private application to submit.",
                403,
            )?);
        }
    }
    let key = internal_key(&env);
    if key.len() < 32 {
        return private(Response::error("Private storage is not configured.", 503)?);
    }
    let body = if read {
        None
    } else {
        let max = if mode == "restore" {
            persistence::MAX_ARCHIVE_BYTES
        } else {
            MAX_HTTP_BODY
        };
        if req
            .headers()
            .get("Content-Length")?
            .and_then(|n| n.parse::<usize>().ok())
            .is_some_and(|n| n > max)
        {
            return private(Response::error("Input is too large.", 413)?);
        }
        let bytes = match bounded_request_body(&req, max).await {
            Ok(bytes) => bytes,
            Err(_) => {
                return private(Response::error(
                    "Input is too large or could not be read safely.",
                    413,
                )?);
            }
        };
        Some(Uint8Array::from(bytes.as_slice()).into())
    };
    // Construct new headers instead of forwarding identity supplied by a client.
    let headers = Headers::new();
    for name in [
        "Content-Type",
        "Origin",
        "Sec-Fetch-Site",
        "Accept",
        "x-scry-operator-key",
        "x-scry-restore-token",
        "x-scry-sha256",
    ] {
        if let Some(value) = req.headers().get(name)? {
            headers.set(name, &value)?;
        }
    }
    headers.set(INTERNAL_HEADER, &key)?;
    headers.set("x-scry-owner", "private-owner")?;
    let mut init = RequestInit::new();
    init.with_method(method)
        .with_headers(headers)
        .with_body(body)
        .with_redirect(RequestRedirect::Manual);
    let trusted = Request::new_with_init(url.as_ref(), &init)?;
    let stub = env
        .durable_object("LEARNING_SPACE")?
        .get_by_name(&space_name(&env)?)?;
    private(stub.fetch_with_request(trusted).await?)
}

async fn validate_access(req: &Request, env: &Env) -> Result<()> {
    let team = variable(env, "ACCESS_TEAM");
    let aud = variable(env, "ACCESS_AUDIENCE");
    let owner = secret(env, "OWNER_SUBJECT");
    if team.is_empty()
        || aud.is_empty()
        || owner.is_empty()
        || !team.ends_with(".cloudflareaccess.com")
        || !team
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '.')
    {
        return Err(Error::RustError("Access is not configured.".into()));
    }
    let token = req
        .headers()
        .get("Cf-Access-Jwt-Assertion")?
        .unwrap_or_default();
    if token.len() > 16 * 1024 {
        return Err(Error::RustError("Invalid token.".into()));
    }
    let parts: Vec<_> = token.split('.').collect();
    if parts.len() != 3 {
        return Err(Error::RustError("Invalid token.".into()));
    }
    let decode = |part: &str| {
        base64::engine::general_purpose::URL_SAFE_NO_PAD
            .decode(part)
            .map_err(|_| Error::RustError("Invalid token.".into()))
    };
    let header: Value = serde_json::from_slice(&decode(parts[0])?)?;
    let claims: Value = serde_json::from_slice(&decode(parts[1])?)?;
    let issuer = format!("https://{team}");
    let kid = header
        .get("kid")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let seconds = now() / 1000;
    let valid_aud = claims.get("aud").is_some_and(|v| {
        v.as_str() == Some(aud.as_str())
            || v.as_array()
                .is_some_and(|a| a.iter().any(|x| x.as_str() == Some(aud.as_str())))
    });
    if header.get("alg").and_then(Value::as_str) != Some("RS256")
        || header
            .get("crit")
            .is_some_and(|v| !v.as_array().is_some_and(Vec::is_empty))
        || kid.is_empty()
        || kid.len() > 256
        || claims.get("iss").and_then(Value::as_str) != Some(issuer.as_str())
        || claims.get("sub").and_then(Value::as_str) != Some(owner.as_str())
        || !valid_aud
        || !claims
            .get("exp")
            .and_then(Value::as_i64)
            .is_some_and(|n| n > seconds)
        || claims
            .get("nbf")
            .is_some_and(|v| !v.as_i64().is_some_and(|n| n <= seconds))
        || claims
            .get("iat")
            .is_some_and(|v| !v.as_i64().is_some_and(|n| n <= seconds + 30))
    {
        return Err(Error::RustError("Invalid token.".into()));
    }
    let cached = ACCESS_KEYS
        .with(|cache| cache.borrow().get(&issuer).cloned())
        .filter(|(fetched, keys)| {
            now() - fetched < 300_000
                && keys
                    .get("keys")
                    .and_then(Value::as_array)
                    .is_some_and(|keys| {
                        keys.iter()
                            .any(|key| key.get("kid").and_then(Value::as_str) == Some(kid))
                    })
        })
        .map(|(_, keys)| keys);
    let jwks = match cached {
        Some(keys) => keys,
        None => {
            let request = Request::new_with_init(
                &format!("{issuer}/cdn-cgi/access/certs"),
                RequestInit::new().with_redirect(RequestRedirect::Manual),
            )?;
            let mut response = timed_fetch(request, 10_000).await?;
            if response.status_code() != 200 {
                return Err(Error::RustError("Access keys unavailable.".into()));
            }
            let bytes = bounded_response_body(&mut response, 64 * 1024).await?;
            let keys: Value = serde_json::from_slice(&bytes)?;
            ACCESS_KEYS.with(|cache| {
                let mut cache = cache.borrow_mut();
                if cache.len() >= 4 {
                    cache.clear();
                }
                cache.insert(issuer.clone(), (now(), keys.clone()));
            });
            keys
        }
    };
    let key = jwks
        .get("keys")
        .and_then(Value::as_array)
        .and_then(|keys| {
            keys.iter().find(|key| {
                key.get("kid").and_then(Value::as_str) == Some(kid)
                    && key.get("kty").and_then(Value::as_str) == Some("RSA")
                    && key
                        .get("alg")
                        .is_none_or(|alg| alg.as_str() == Some("RS256"))
                    && key.get("use").is_none_or(|u| u.as_str() == Some("sig"))
            })
        })
        .ok_or_else(|| Error::RustError("Unknown Access key.".into()))?;
    let crypto = Reflect::get(&js_sys::global(), &"crypto".into())?;
    let subtle = Reflect::get(&crypto, &"subtle".into())?;
    let algorithm = js_json(&json!({"name":"RSASSA-PKCS1-v1_5","hash":"SHA-256"}))?;
    let imported = JsFuture::from(
        call(
            &subtle,
            "importKey",
            &[
                "jwk".into(),
                js_json(key)?,
                algorithm.clone(),
                false.into(),
                Array::of1(&"verify".into()).into(),
            ],
        )?
        .dyn_into::<Promise>()?,
    )
    .await?;
    let signature = decode(parts[2])?;
    let input = format!("{}.{}", parts[0], parts[1]);
    let verified = JsFuture::from(
        call(
            &subtle,
            "verify",
            &[
                algorithm,
                imported,
                Uint8Array::from(signature.as_slice()).into(),
                Uint8Array::from(input.as_bytes()).into(),
            ],
        )?
        .dyn_into::<Promise>()?,
    )
    .await?;
    if verified.as_bool() != Some(true) {
        return Err(Error::RustError("Invalid signature.".into()));
    }
    Ok(())
}

async fn bounded_stream(stream: JsValue, max: usize) -> Result<Vec<u8>> {
    if stream.is_null() || stream.is_undefined() {
        return Ok(vec![]);
    }
    let reader = call(&stream, "getReader", &[])?;
    let mut bytes = Vec::new();
    loop {
        let chunk = JsFuture::from(call(&reader, "read", &[])?.dyn_into::<Promise>()?).await?;
        if Reflect::get(&chunk, &"done".into())?.as_bool() == Some(true) {
            break;
        }
        let data = Uint8Array::new(&Reflect::get(&chunk, &"value".into())?);
        if bytes.len() + data.length() as usize > max {
            let _ = call(&reader, "cancel", &[]);
            return Err(Error::RustError(
                "Input exceeds its bounded size limit.".into(),
            ));
        }
        bytes.extend_from_slice(&data.to_vec());
    }
    Ok(bytes)
}
async fn bounded_request_body(req: &Request, max: usize) -> Result<Vec<u8>> {
    bounded_stream(Reflect::get(req.inner().as_ref(), &"body".into())?, max).await
}
async fn bounded_response_body(response: &mut Response, max: usize) -> Result<Vec<u8>> {
    match response.body() {
        ResponseBody::Empty => Ok(vec![]),
        ResponseBody::Body(bytes) if bytes.len() <= max => Ok(bytes.clone()),
        ResponseBody::Body(_) => Err(Error::RustError(
            "Response exceeds its bounded size limit.".into(),
        )),
        ResponseBody::Stream(stream) => bounded_stream(stream.clone().into(), max).await,
    }
}
async fn timed_fetch(req: Request, timeout_ms: i64) -> Result<Response> {
    let constructor = Reflect::get(&js_sys::global(), &"AbortSignal".into())?;
    let raw = call(
        &constructor,
        "timeout",
        &[JsValue::from_f64(timeout_ms as f64)],
    )?;
    let signal: worker::web_sys::AbortSignal = raw.dyn_into()?;
    Fetch::Request(req).send_with_signal(&signal.into()).await
}

#[durable_object]
pub struct LearningSpace {
    state: State,
    env: Env,
}
impl DurableObject for LearningSpace {
    fn new(state: State, env: Env) -> Self {
        Self { state, env }
    }
    async fn fetch(&self, mut req: Request) -> Result<Response> {
        let key = internal_key(&self.env);
        if key.len() < 32
            || req.headers().get(INTERNAL_HEADER)?.as_deref() != Some(key.as_str())
            || req.headers().get("x-scry-owner")?.as_deref() != Some("private-owner")
        {
            return private(Response::error("Internal authorization required.", 403)?);
        }
        let path = req.path();
        if path == "/operator/restore" {
            return self.restore(req).await;
        }
        if path == "/__internal/daily" {
            self.load()?;
            self.backup(false).await?;
            self.arm().await?;
            return private(Response::ok("Snapshot verified.")?);
        }
        let url = req.url()?;
        if matches!(req.method(), Method::Get | Method::Head) {
            if path == "/readyz" {
                self.load()?;
                return private(Response::ok("ready")?);
            }
            if let Some((mime, bytes)) = web::asset(&path) {
                let mut response = Response::from_bytes(bytes.to_vec())?;
                response.headers_mut().set("Content-Type", mime)?;
                return private(response);
            }
            let mut app = self.load()?;
            let previous = app.clone();
            engine::recover(&mut app, now());
            if let Err(e) = engine::ensure_occurrence(&mut app, now(), &random_id()?) {
                return app_error(e);
            }
            if app != previous {
                self.save(&mut app, Some(previous.revision))?;
            }
            if app.protected_goal().is_some()
                && (path == "/export"
                    || path
                        .strip_prefix("/photos/")
                        .is_some_and(|id| app.protected_goal() == Some(id)))
            {
                let mut gate_query = BTreeMap::new();
                gate_query.insert("return_to".into(), path.clone());
                let gate_query = view_query(&self.env, &gate_query)?;
                return private(Response::from_html(web::render(
                    &app,
                    "/gate",
                    &gate_query,
                    now(),
                    None,
                ))?);
            }
            if path == "/export" {
                // Includes referenced assets, never just the database metadata.
                let archive = self
                    .archive(app)
                    .await
                    .map_err(|_| Error::RustError("Complete export is unavailable.".into()))?;
                let bytes = archive.encode().map_err(to_worker_error)?;
                drop(archive);
                // Assets await outside SQLite. Recheck the latest attempt
                // before returning answers from the earlier committed snapshot.
                let mut latest = self.load()?;
                if latest.protected_goal().is_some() {
                    let mut query = BTreeMap::new();
                    query.insert("return_to".into(), path.clone());
                    return private(Response::from_html(web::render(
                        &latest,
                        "/gate",
                        &view_query(&self.env, &query)?,
                        now(),
                        None,
                    ))?);
                }
                if req.method() == Method::Get {
                    let before = latest.revision;
                    if observe_read(&mut latest, &path, now()) {
                        self.save(&mut latest, Some(before))?;
                    }
                }
                let hash = persistence::sha256(&bytes);
                let mut response = Response::from_bytes(bytes)?;
                response
                    .headers_mut()
                    .set("Content-Type", "application/json; charset=utf-8")?;
                response.headers_mut().set(
                    "Content-Disposition",
                    "attachment; filename=scry-complete-export.json",
                )?;
                response.headers_mut().set("X-Scry-Sha256", &hash)?;
                return private(response);
            }
            if let Some(id) = path.strip_prefix("/photos/") {
                if let Some(photo) = app.goals.get(id).and_then(|g| g.photo.as_ref()) {
                    let bucket = self.env.bucket("SCRY_ASSETS")?;
                    if let Some(object) = bucket.get(&photo.key).execute().await? {
                        if object.size() != photo.size as u64 || photo.size > PHOTO_LIMIT {
                            return private(Response::error(
                                "The saved photo failed integrity verification.",
                                503,
                            )?);
                        }
                        let Some(body) = object.body() else {
                            return private(Response::error("Photo not found.", 404)?);
                        };
                        let bytes = body.bytes().await?;
                        if bytes.len() != photo.size || persistence::sha256(&bytes) != photo.sha256
                        {
                            return private(Response::error(
                                "The saved photo failed integrity verification.",
                                503,
                            )?);
                        }
                        let mut latest = self.load()?;
                        if latest.protected_goal() == Some(id) {
                            let mut query = BTreeMap::new();
                            query.insert("return_to".into(), path.clone());
                            return private(Response::from_html(web::render(
                                &latest,
                                "/gate",
                                &view_query(&self.env, &query)?,
                                now(),
                                None,
                            ))?);
                        }
                        if req.method() == Method::Get {
                            let before = latest.revision;
                            if observe_read(&mut latest, &path, now()) {
                                self.save(&mut latest, Some(before))?;
                            }
                        }
                        let mut response = Response::from_bytes(bytes)?;
                        response.headers_mut().set("Content-Type", &photo.mime)?;
                        return private(response);
                    }
                }
                return private(Response::error("Photo not found.", 404)?);
            }
            let external_query: BTreeMap<String, String> = url
                .query_pairs()
                .map(|(k, v)| (k.into_owned(), v.into_owned()))
                .collect();
            let query = view_query(&self.env, &external_query)?;
            self.arm().await?;
            // Alarm installation can yield to another owner request. Render
            // and mark only the latest allowed reference content after it.
            let mut app = self.load()?;
            if req.method() == Method::Get {
                let before = app.revision;
                if observe_read(&mut app, &path, now()) {
                    self.save(&mut app, Some(before))?;
                }
            }
            return private(Response::from_html(web::render(
                &app,
                &path,
                &query,
                now(),
                None,
            ))?);
        }
        if req.method() != Method::Post {
            return private(Response::error("Method not allowed.", 405)?);
        }
        if path == "/operator/resume-work" {
            if !matches!(
                variable(&self.env, "SCRY_ENV").as_str(),
                "preview" | "production"
            ) {
                return private(Response::error(
                    "Use approved private recovery configuration.",
                    403,
                )?);
            }
            let expected = secret(&self.env, "OPERATOR_KEY");
            if expected.len() < 32
                || req.headers().get("x-scry-operator-key")?.as_deref() != Some(expected.as_str())
            {
                return private(Response::error("Operator authorization required.", 403)?);
            }
            if self.stored_rows()?.is_empty() {
                return private(Response::error(
                    "There is no restored learning space to resume.",
                    409,
                )?);
            }
            let mut app = self.load()?;
            if !app.restored_paused {
                return private(Response::error(
                    "This space is not awaiting recovery review.",
                    409,
                )?);
            }
            let before = app.revision;
            app.restored_paused = false;
            self.save(&mut app, Some(before))?;
            self.arm().await?;
            return private(Response::ok(
                "Future external work is enabled. Historical uncertain work remains paused.",
            )?);
        }
        if path == "/operator/backup" {
            let expected = secret(&self.env, "OPERATOR_KEY");
            if expected.len() < 32
                || req.headers().get("x-scry-operator-key")?.as_deref() != Some(expected.as_str())
            {
                return private(Response::error("Operator authorization required.", 403)?);
            }
            self.backup(true).await?;
            self.arm().await?;
            return private(Response::ok(
                "Pre-release snapshot verified by complete readback.",
            )?);
        }
        let content_type = req.headers().get("Content-Type")?.unwrap_or_default();
        if !content_type.starts_with("application/x-www-form-urlencoded")
            && !content_type.starts_with("multipart/form-data;")
        {
            return private(Response::error("Use an application form to submit.", 415)?);
        }
        let form = req.form_data().await?;
        let raw_form: JsValue = form.into();
        let raw_keys = call(&raw_form, "keys", &[])?;
        let keys = js_sys::try_iter(&raw_keys)?
            .ok_or_else(|| Error::RustError("Invalid form.".into()))?
            .map(|key| {
                key.and_then(|key| {
                    key.as_string()
                        .ok_or_else(|| JsValue::from_str("Invalid form field."))
                })
            })
            .collect::<std::result::Result<Vec<String>, JsValue>>()?;
        let form = FormData::from(raw_form);
        let mut fields = BTreeMap::new();
        for name in keys {
            if name.starts_with("__") {
                return private(Response::error("Invalid private form field.", 422)?);
            }
            if let Some(FormEntry::Field(value)) = form.get(&name) {
                if form.get_all(&name).is_some_and(|values| values.len() != 1) {
                    return private(Response::error("Duplicate form field.", 422)?);
                }
                fields.insert(name, value);
            }
        }
        let mut app = self.load()?;
        let before = app.revision;
        if fields.get("csrf").map(String::as_str) != Some(app.csrf.as_str()) {
            return private(Response::error("Reload before submitting.", 403)?);
        }
        if path == "/__fixture" {
            if !development(&self.env) || !app.goals.is_empty() || !app.events.is_empty() {
                return private(Response::error(
                    "Authored fixtures require an unused isolated development space.",
                    409,
                )?);
            }
            if let Err(e) = engine::seed_fixture(&mut app, now()) {
                return app_error(e);
            }
            self.save(&mut app, Some(before))?;
            self.arm().await?;
            return redirect("/");
        }
        let mut photo_key = None;
        if let Some(FormEntry::File(file)) = form.get("photo")
            && file.size() > 0
        {
            if path != "/create" || file.size() > PHOTO_LIMIT {
                return render_error(
                    &self.env,
                    &app,
                    &path,
                    &fields,
                    AppError::new(
                        422,
                        "Choose a JPEG, PNG, or WebP photo up to 4 MiB. Your caption is still editable.",
                    ),
                );
            }
            let text = fields
                .get("intent")
                .or_else(|| fields.get("text"))
                .map(String::as_str)
                .unwrap_or_default();
            if text.len() > 1024 {
                return render_error(
                    &self.env,
                    &app,
                    &path,
                    &fields,
                    AppError::new(
                        422,
                        "Keep a photo caption within 1 KiB. Your text is still editable; attach the photo again after revising it.",
                    ),
                );
            }
            let op = fields.get("operation_id").cloned().unwrap_or_default();
            if op.len() < 16
                || op.len() > 128
                || !op
                    .chars()
                    .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_')
            {
                return private(Response::error("Reload before submitting.", 422)?);
            }
            let bytes = file.bytes().await?;
            let Some(mime) = persistence::image_mime(&bytes) else {
                return render_error(
                    &self.env,
                    &app,
                    &path,
                    &fields,
                    AppError::new(
                        422,
                        "Choose a JPEG, PNG, or WebP photo. Your caption is still editable.",
                    ),
                );
            };
            let hash = persistence::sha256(&bytes);
            let key = format!("photos/{}/{op}-{hash}", space_name(&self.env)?);
            let photo = Photo {
                key: key.clone(),
                mime: mime.into(),
                size: bytes.len(),
                sha256: hash,
            };
            self.env
                .bucket("SCRY_ASSETS")?
                .put(&key, bytes)
                .execute()
                .await?;
            fields.insert("__photo".into(), serde_json::to_string(&photo)?);
            photo_key = Some(key);
            // R2 is awaited outside SQLite; reload and validate the current revision.
            app = self.load()?;
        }
        let before = app.revision;
        let result = engine::mutate(&mut app, &path, &fields, now(), &random_id()?);
        match result {
            Err(e) => {
                if let Some(key) = photo_key {
                    let latest = self.load()?;
                    if !latest
                        .goals
                        .values()
                        .any(|g| g.photo.as_ref().is_some_and(|p| p.key == key))
                    {
                        let _ = self.env.bucket("SCRY_ASSETS")?.delete(&key).await;
                    }
                }
                render_error(&self.env, &app, &path, &fields, e)
            }
            Ok(result) => {
                // New captured or edited content must fit a complete recovery
                // archive. Paid completions still retain their raw/ledger
                // outcome through the separate durable work path.
                if app.revision != before
                    && (matches!(path.as_str(), "/create" | "/add") || path.ends_with("/edit"))
                    && let Err(e) = persistence::archive_size(&app)
                {
                    let retained = self.load()?;
                    if let Some(key) = photo_key
                        && !retained
                            .goals
                            .values()
                            .any(|g| g.photo.as_ref().is_some_and(|p| p.key == key))
                    {
                        let _ = self.env.bucket("SCRY_ASSETS")?.delete(&key).await;
                    }
                    return render_error(&self.env, &retained, &path, &fields, e);
                }
                self.save(&mut app, Some(before))?;
                if path == "/settings/backup" {
                    self.backup(false).await?;
                }
                self.arm().await?;
                redirect(&result.redirect)
            }
        }
    }
    async fn alarm(&self) -> Result<Response> {
        // The recovery alarm is installed before any paid send. An evicted or
        // timed-out send becomes uncertain once, never another invisible send.
        let mut app = self.load()?;
        let before = app.revision;
        engine::recover(&mut app, now());
        if app.revision != before {
            self.save(&mut app, Some(before))?;
        }
        if app
            .backup
            .completed_ms
            .is_none_or(|last| last <= now() - DAY_MS)
        {
            let _ = self.backup(false).await;
        }
        let mut app = self.load()?;
        let before = app.revision;
        if !app.restored_paused {
            let config = self.work_config();
            let lease = random_id()?;
            match engine::claim_work(&mut app, now(), &lease, &config) {
                Err(e) => return app_error(e),
                Ok(work) => {
                    if app.revision != before {
                        self.save(&mut app, Some(before))?;
                    }
                    if let Some(work) = work {
                        set_alarm(&self.state.storage(), now() + RECOVERY_MS).await?;
                        self.run_work(work).await?;
                    }
                }
            }
        }
        self.arm().await?;
        private(Response::ok("ok")?)
    }
}

fn to_worker_error(e: AppError) -> Error {
    Error::RustError(e.message)
}

#[derive(Deserialize)]
struct StoredRow {
    key: String,
    value: String,
}
impl LearningSpace {
    fn stored_rows(&self) -> Result<BTreeMap<String, String>> {
        let sql = self.state.storage().sql();
        sql.exec("CREATE TABLE IF NOT EXISTS records (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID", None)?;
        let records: Vec<StoredRow> = sql
            .exec("SELECT key, value FROM records ORDER BY key", None)?
            .to_array()?;
        Ok(records
            .into_iter()
            .map(|row| (row.key, row.value))
            .collect())
    }
    fn load(&self) -> Result<App> {
        let rows = self.stored_rows()?;
        if rows.is_empty() {
            let mut app = App::new(random_id()?);
            self.save(&mut app, None)?;
            return Ok(app);
        }
        persistence::app_from_rows(&rows).map_err(to_worker_error)
    }
    fn save(&self, app: &mut App, expected: Option<u64>) -> Result<()> {
        app.revision = expected
            .unwrap_or(0)
            .checked_add(1)
            .ok_or_else(|| Error::RustError("State revision exhausted.".into()))?;
        let next = persistence::rows(app).map_err(to_worker_error)?;
        let previous = self.stored_rows()?;
        let actual = previous
            .get("meta")
            .map(|meta| serde_json::from_str::<Value>(meta))
            .transpose()?
            .and_then(|value| value.get("revision").and_then(Value::as_u64));
        if actual != expected {
            return Err(to_worker_error(AppError::conflict()));
        }
        let sql = self.state.storage().sql();
        let storage = self.state.storage();
        let raw: JsValue = storage.as_raw().into();
        let callback = Closure::once(move || -> std::result::Result<JsValue, JsValue> {
            for key in previous.keys().filter(|key| !next.contains_key(*key)) {
                sql.exec(
                    "DELETE FROM records WHERE key = ?",
                    Some(vec![key.as_str().into()]),
                )
                .map_err(JsValue::from)?;
            }
            for (key, value) in &next {
                if previous.get(key) != Some(value) {
                    sql.exec("INSERT INTO records (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", Some(vec![key.as_str().into(), value.as_str().into()])).map_err(JsValue::from)?;
                }
            }
            Ok(JsValue::UNDEFINED)
        });
        // The closure is wholly synchronous. Cloudflare rolls every changed
        // record back if any statement throws; no request/provider await enters.
        call(&raw, "transactionSync", &[callback.as_ref().clone()])?;
        Ok(())
    }
    fn work_config(&self) -> engine::WorkConfig {
        let disabled = isolated(&self.env);
        let model = variable(&self.env, "OPENROUTER_MODEL");
        let jev_model = variable(&self.env, "JEV_MODEL");
        let generation =
            !disabled && !secret(&self.env, "OPENROUTER_API_KEY").is_empty() && !model.is_empty();
        let jev = !disabled
            && !secret(&self.env, "JEV_API_KEY").is_empty()
            && !jev_model.is_empty()
            && valid_provider_url(&variable(&self.env, "JEV_URL"));
        engine::WorkConfig {
            generation,
            jev,
            critic: jev,
            model,
            jev_model,
        }
    }
    async fn arm(&self) -> Result<()> {
        if variable(&self.env, "SCRY_ENV") == "restore" {
            return Ok(());
        }
        let app = self.load()?;
        let timestamp = now();
        let due = app
            .backup
            .completed_ms
            .map(|t| t + DAY_MS)
            .unwrap_or(timestamp + DAY_MS);
        let mut alarm = if app.backup.error.is_some() {
            timestamp + 30 * 60_000
        } else {
            due.max(timestamp + 1000)
        };
        if !app.restored_paused {
            for job in app.jobs.values() {
                if matches!(job.status.as_str(), "queued" | "candidates") {
                    alarm = alarm.min(timestamp + 1000);
                }
                if matches!(job.status.as_str(), "sent" | "critic-sent") {
                    alarm = alarm.min(job.started_ms.unwrap_or(timestamp) + RECOVERY_MS);
                }
            }
            for assessment in app.assessments.values() {
                if assessment.status == "queued" {
                    alarm = alarm.min(timestamp + 1000);
                }
                if assessment.status == "sent" {
                    alarm = alarm.min(assessment.started_ms.unwrap_or(timestamp) + RECOVERY_MS);
                }
            }
        }
        set_alarm(&self.state.storage(), alarm.max(timestamp + 1000)).await
    }
    async fn run_work(&self, work: engine::Work) -> Result<()> {
        let result = self.transmit(&work).await;
        let mut app = self.load()?;
        let before = app.revision;
        match result {
            Ok((raw, model, cost, rejection)) => {
                // The pure engine rechecks lease, source/content/schedule
                // revision and persists the raw response before publication.
                // Even a fenced stale response settles and retains its paid
                // outcome in the engine. Persist that state before returning.
                if let Some(message) = rejection {
                    let _ = engine::fail_received_work(
                        &mut app,
                        &work,
                        now(),
                        raw,
                        model,
                        cost,
                        &message,
                    );
                } else {
                    let _ = engine::finish_work(&mut app, &work, raw, model, cost, now());
                }
            }
            Err((message, unknown)) => {
                // Source ownership may have changed while HTTP was in flight.
                // Its existing reservation remains; a stale failure must not
                // erase a receipt or prevent saving other reconciliation.
                let _ = engine::fail_work(&mut app, &work, &message, unknown, now());
            }
        }
        self.save(&mut app, Some(before))?;
        Ok(())
    }
    async fn transmit(
        &self,
        work: &engine::Work,
    ) -> std::result::Result<(String, String, Option<u64>, Option<String>), (String, bool)> {
        if isolated(&self.env) {
            return Err((
                "External work is disabled in this isolated environment.".into(),
                false,
            ));
        }
        let chat = work.kind == "generation" || work.kind == "transcribe";
        let endpoint = if chat {
            "https://openrouter.ai/api/v1/chat/completions".into()
        } else {
            variable(&self.env, "JEV_URL")
        };
        if !valid_provider_url(&endpoint) {
            return Err((
                "The model endpoint is not securely configured.".into(),
                false,
            ));
        }
        let key = secret(
            &self.env,
            if chat {
                "OPENROUTER_API_KEY"
            } else {
                "JEV_API_KEY"
            },
        );
        let model = variable(
            &self.env,
            if chat {
                "OPENROUTER_MODEL"
            } else {
                "JEV_MODEL"
            },
        );
        if key.is_empty() || model.is_empty() {
            return Err(("The model is not configured.".into(), false));
        }
        let mut request = work.request.clone();
        if let Some(photo_key) = &work.photo_key {
            let bucket = self
                .env
                .bucket("SCRY_ASSETS")
                .map_err(|_| ("Photo storage is unavailable.".into(), false))?;
            let photo = self
                .load()
                .map_err(|_| ("Saved state is unavailable.".into(), false))?
                .goals
                .values()
                .filter_map(|g| g.photo.clone())
                .find(|p| &p.key == photo_key)
                .ok_or_else(|| ("The saved photo is missing.".into(), false))?;
            let object = bucket
                .get(photo_key)
                .execute()
                .await
                .map_err(|_| ("The saved photo could not be read.".into(), false))?
                .ok_or_else(|| ("The saved photo is missing.".into(), false))?;
            if object.size() != photo.size as u64 || photo.size > PHOTO_LIMIT {
                return Err((
                    "The saved photo failed integrity verification.".into(),
                    false,
                ));
            }
            let bytes = object
                .body()
                .ok_or_else(|| ("The saved photo has no data.".into(), false))?
                .bytes()
                .await
                .map_err(|_| ("The saved photo could not be read.".into(), false))?;
            if bytes.len() > PHOTO_LIMIT
                || bytes.len() != photo.size
                || persistence::sha256(&bytes) != photo.sha256
            {
                return Err((
                    "The saved photo failed integrity verification.".into(),
                    false,
                ));
            }
            let messages = request
                .get_mut("messages")
                .and_then(Value::as_array_mut)
                .ok_or_else(|| ("The transcription request is invalid.".into(), false))?;
            let user = messages
                .iter_mut()
                .rev()
                .find(|v| v.get("role").and_then(Value::as_str) == Some("user"))
                .ok_or_else(|| ("The transcription request is invalid.".into(), false))?;
            let text = user
                .get("content")
                .and_then(Value::as_str)
                .map(str::to_string)
                .or_else(|| {
                    user.get("content").and_then(Value::as_array).map(|parts| {
                        parts
                            .iter()
                            .filter_map(|p| p.get("text").and_then(Value::as_str))
                            .collect::<Vec<_>>()
                            .join("\n")
                    })
                })
                .unwrap_or_else(|| "Transcribe this photo accurately.".into());
            user["content"] = json!([{"type":"text","text":text},{"type":"image_url","image_url":{"url":format!("data:{};base64,{}",photo.mime,STANDARD.encode(bytes))}}]);
        }
        let headers = Headers::new();
        headers
            .set("Authorization", &format!("Bearer {key}"))
            .map_err(|_| ("The provider request is invalid.".into(), false))?;
        headers
            .set("Content-Type", "application/json")
            .map_err(|_| ("The provider request is invalid.".into(), false))?;
        headers
            .set("Accept", "application/json")
            .map_err(|_| ("The provider request is invalid.".into(), false))?;
        headers
            .set("X-OpenRouter-Title", "Scry")
            .map_err(|_| ("The provider request is invalid.".into(), false))?;
        let body = serde_json::to_string(&request)
            .map_err(|_| ("The provider request is invalid.".into(), false))?;
        let mut init = RequestInit::new();
        init.with_method(Method::Post)
            .with_headers(headers)
            .with_redirect(RequestRedirect::Manual)
            .with_body(Some(body.into()));
        let req = Request::new_with_init(&endpoint, &init)
            .map_err(|_| ("The provider request is invalid.".into(), false))?;
        // From this point, a network failure has an unknown paid outcome.
        let mut response = timed_fetch(req, MODEL_TIMEOUT_MS).await.map_err(|_| {
            (
                "The provider response was interrupted; its paid outcome is unknown.".into(),
                true,
            )
        })?;
        let bytes = bounded_response_body(&mut response, MODEL_RESPONSE_LIMIT)
            .await
            .map_err(|_| {
                (
                    "The provider response could not be safely read; its paid outcome is unknown."
                        .into(),
                    true,
                )
            })?;
        let rejection = (!(200..300).contains(&response.status_code())).then(|| {
            format!(
                "The provider rejected the request ({}).",
                response.status_code()
            )
        });
        let raw = String::from_utf8(bytes).map_err(|_| {
            (
                "The provider response is malformed; its paid usage is unknown.".into(),
                true,
            )
        })?;
        // A received, bounded malformed response is still a paid outcome.
        // Preserve its bytes through the engine receipt before honest parsing
        // failure; it must not disappear into a transport-only error.
        let parsed = serde_json::from_str::<Value>(&raw).ok();
        let returned_model = parsed
            .as_ref()
            .and_then(|parsed| parsed.get("model"))
            .and_then(Value::as_str)
            .filter(|m| !m.is_empty() && m.len() <= 256)
            .unwrap_or(&model)
            .to_string();
        let cost = parsed
            .as_ref()
            .and_then(|parsed| parsed.get("usage"))
            .and_then(|v| v.get("cost"))
            .and_then(|v| {
                v.as_f64()
                    .or_else(|| v.as_str().and_then(|s| s.parse().ok()))
            })
            .filter(|n| n.is_finite() && *n >= 0.)
            // Retain over-budget measured expense. Rust's saturating float
            // conversion conservatively blocks further work even for a value
            // larger than the integer ledger can represent.
            .map(|dollars| (dollars * 1_000_000.).ceil() as u64);
        Ok((raw, returned_model, cost, rejection))
    }
    async fn archive(&self, app: App) -> Result<persistence::Archive> {
        // Enforce the full JSON/base64 footprint before fetching any asset
        // bodies. Counting only after loading every photo could exhaust the
        // isolate before returning the bounded-capacity error.
        persistence::archive_size(&app).map_err(to_worker_error)?;
        let bucket = self.env.bucket("SCRY_ASSETS")?;
        let mut photos = Vec::new();
        for photo in persistence::required_photos(&app)
            .map_err(to_worker_error)?
            .into_values()
        {
            let object = bucket.get(&photo.key).execute().await?.ok_or_else(|| {
                Error::RustError("A required photo is missing; the snapshot is incomplete.".into())
            })?;
            if object.size() != photo.size as u64 {
                return Err(Error::RustError(
                    "A required photo has invalid size.".into(),
                ));
            }
            let bytes = object
                .body()
                .ok_or_else(|| Error::RustError("A required photo is unreadable.".into()))?
                .bytes()
                .await?;
            photos.push(persistence::ArchivedPhoto {
                photo: photo.clone(),
                bytes,
            });
        }
        Ok(persistence::Archive::new(app, photos, now()))
    }
    async fn backup(&self, pre_release: bool) -> Result<()> {
        let operation = async {
            let app = self.load()?;
            let archive = self.archive(app).await?;
            let bytes = archive.encode().map_err(to_worker_error)?;
            drop(archive);
            let expected_size = bytes.len();
            let hash = persistence::sha256(&bytes);
            let prefix = format!("snapshots/{}/", space_name(&self.env)?);
            let key = format!(
                "{prefix}{}-{}-{}.json",
                now(),
                if pre_release { "pre-release" } else { "daily" },
                random_id()?
            );
            let bucket = self.env.bucket("SCRY_BACKUPS")?;
            bucket.put(&key, bytes).execute().await?;
            let object = bucket
                .get(&key)
                .execute()
                .await?
                .ok_or_else(|| Error::RustError("Snapshot readback was missing.".into()))?;
            if object.size() != expected_size as u64 {
                return Err(Error::RustError("Snapshot readback size differed.".into()));
            }
            let readback = object
                .body()
                .ok_or_else(|| Error::RustError("Snapshot readback was empty.".into()))?
                .bytes()
                .await?;
            if readback.len() != expected_size || persistence::sha256(&readback) != hash {
                return Err(Error::RustError(
                    "Snapshot readback failed integrity verification.".into(),
                ));
            }
            persistence::Archive::decode(&readback, &hash).map_err(to_worker_error)?;
            let mut latest = self.load()?;
            let before = latest.revision;
            latest.backup = BackupStatus {
                completed_ms: Some(now()),
                key: Some(key),
                sha256: Some(hash),
                error: None,
            };
            self.save(&mut latest, Some(before))?;
            self.prune_backups(&prefix).await?;
            Ok(())
        }
        .await;
        if operation.is_err() {
            let mut app = self.load()?;
            let before = app.revision;
            app.backup.error = Some("The complete snapshot could not be verified. Review remains available; inspect operator recovery configuration.".into());
            self.save(&mut app, Some(before))?;
        }
        operation
    }
    async fn prune_backups(&self, prefix: &str) -> Result<()> {
        let bucket = self.env.bucket("SCRY_BACKUPS")?;
        let mut cursor = None;
        loop {
            let mut builder = bucket.list().prefix(prefix).limit(1000);
            if let Some(c) = cursor {
                builder = builder.cursor(c);
            }
            let list = builder.execute().await?;
            for object in list.objects() {
                if object.uploaded().as_millis() as i64 <= now() - 30 * DAY_MS {
                    bucket.delete(object.key()).await?;
                }
            }
            if !list.truncated() {
                break;
            }
            cursor = list.cursor();
            if cursor.is_none() {
                break;
            }
        }
        Ok(())
    }
    async fn restore(&self, req: Request) -> Result<Response> {
        if variable(&self.env, "SCRY_ENV") != "restore" || provider_secrets_present(&self.env) {
            return private(Response::error(
                "Use a fresh isolated restore environment.",
                403,
            )?);
        }
        let expected = secret(&self.env, "RESTORE_TOKEN");
        if expected.len() < 32
            || req.headers().get("x-scry-restore-token")?.as_deref() != Some(expected.as_str())
        {
            return private(Response::error("Restore authorization required.", 403)?);
        }
        if !self.stored_rows()?.is_empty() {
            return private(Response::error(
                "Restore requires an unused learning-space object.",
                409,
            )?);
        }
        let hash = req.headers().get("x-scry-sha256")?.unwrap_or_default();
        let bytes = bounded_request_body(&req, persistence::MAX_ARCHIVE_BYTES).await?;
        let mut archive = match persistence::Archive::decode(&bytes, &hash) {
            Ok(a) => a,
            Err(e) => return app_error(e),
        };
        let mut renamed = BTreeMap::new();
        let bucket = self.env.bucket("SCRY_ASSETS")?;
        for photo in &archive.photos {
            let key = format!(
                "photos/{}/{}-{}",
                space_name(&self.env)?,
                photo.photo.sha256,
                random_id()?
            );
            bucket.put(&key, photo.bytes.clone()).execute().await?;
            let object =
                bucket.get(&key).execute().await?.ok_or_else(|| {
                    Error::RustError("Restored photo readback is missing.".into())
                })?;
            let restored = object
                .body()
                .ok_or_else(|| Error::RustError("Restored photo readback is empty.".into()))?
                .bytes()
                .await?;
            if restored != photo.bytes {
                return private(Response::error("Restored photo readback differed.", 422)?);
            }
            renamed.insert(photo.photo.key.clone(), key);
        }
        for goal in archive.app.goals.values_mut() {
            if let Some(photo) = &mut goal.photo {
                photo.key = renamed.get(&photo.key).cloned().ok_or_else(|| {
                    Error::RustError("Restored photo manifest is incomplete.".into())
                })?;
            }
        }
        archive.app.csrf = random_id()?;
        engine::pause_for_restore(&mut archive.app, now());
        archive.app.backup = BackupStatus::default();
        // Check again after R2 awaits: a concurrent restore cannot overwrite it.
        if !self.stored_rows()?.is_empty() {
            return private(Response::error(
                "The restore target is no longer unused.",
                409,
            )?);
        }
        self.save(&mut archive.app, None)?;
        self.state.storage().sync().await?;
        private(Response::ok(
            "Complete archive restored into a fresh learning space. External work remains paused.",
        )?)
    }
}
fn valid_provider_url(raw: &str) -> bool {
    Url::parse(raw).is_ok_and(|url| {
        url.scheme() == "https"
            && url.host_str().is_some()
            && url.username().is_empty()
            && url.password().is_none()
            && url.query().is_none()
            && url.fragment().is_none()
    })
}
async fn set_alarm(storage: &Storage, millis: i64) -> Result<()> {
    storage
        .set_alarm(ScheduledTime::new(js_sys::Date::new(&JsValue::from_f64(
            millis as f64,
        ))))
        .await
}

#[event(scheduled)]
pub async fn scheduled(_event: ScheduledEvent, env: Env, ctx: ScheduleContext) {
    if variable(&env, "SCRY_ENV") == "restore" {
        return;
    }
    ctx.wait_until(async move {
        let result = async {
            let key = internal_key(&env);
            if key.len() < 32 {
                return Err(Error::RustError(
                    "Backup authorization is not configured.".into(),
                ));
            }
            let headers = Headers::new();
            headers.set(INTERNAL_HEADER, &key)?;
            headers.set("x-scry-owner", "private-owner")?;
            let mut init = RequestInit::new();
            init.with_method(Method::Post).with_headers(headers);
            let req = Request::new_with_init("https://internal.invalid/__internal/daily", &init)?;
            env.durable_object("LEARNING_SPACE")?
                .get_by_name(&space_name(&env)?)?
                .fetch_with_request(req)
                .await?;
            Ok::<(), Error>(())
        }
        .await;
        if result.is_err() {
            console_error!("The scheduled complete snapshot could not be verified.");
        }
    });
}
