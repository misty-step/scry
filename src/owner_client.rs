//! Native Owner API transport shared by the CLI and stdio MCP adapter.
//! Errors are fixed vocabulary: never reflect upstream bodies, URLs or credentials.
use crate::owner_token::token_id;
use reqwest::{Url, blocking::Client, redirect::Policy};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::{
    fs,
    io::Read,
    os::unix::fs::PermissionsExt,
    path::{Path, PathBuf},
    time::Duration,
};

pub type Result<T> = std::result::Result<T, &'static str>;

#[derive(Serialize, Deserialize)]
pub struct Login {
    pub origin: String,
    pub token: String,
}

pub fn config_path() -> Result<PathBuf> {
    if let Some(dir) = std::env::var_os("XDG_CONFIG_HOME") {
        return Ok(PathBuf::from(dir).join("scry/login.json"));
    }
    Ok(
        PathBuf::from(std::env::var_os("HOME").ok_or("Set HOME or use --config.")?)
            .join(".config/scry/login.json"),
    )
}

pub fn origin(input: &str) -> Result<String> {
    let url = Url::parse(input).map_err(|_| "Use an HTTPS origin.")?;
    let local = url.scheme() == "http"
        && matches!(url.host_str(), Some("127.0.0.1" | "localhost" | "[::1]"));
    if !(url.scheme() == "https" || local)
        || url.path() != "/"
        || url.query().is_some()
        || url.fragment().is_some()
        || !url.username().is_empty()
        || url.password().is_some()
        || url.host_str().is_none()
    {
        return Err("Use an HTTPS origin (HTTP is allowed only on loopback).");
    }
    Ok(url.origin().ascii_serialization())
}

pub fn load(path: &Path) -> Result<Login> {
    let metadata = fs::symlink_metadata(path).map_err(|_| "Log in with scry login first.")?;
    if !metadata.is_file() || metadata.permissions().mode() & 0o077 != 0 {
        return Err("Credential file must be private (chmod 600).");
    }
    // Credentials are small. A corrupt file must not consume unbounded memory.
    let mut bytes = Vec::new();
    fs::File::open(path)
        .and_then(|file| file.take(8193).read_to_end(&mut bytes))
        .map_err(|_| "Cannot read login.")?;
    if bytes.len() > 8192 {
        return Err("Invalid login file.");
    }
    let login: Login = serde_json::from_slice(&bytes).map_err(|_| "Invalid login file.")?;
    if origin(&login.origin)? != login.origin || token_id(&login.token).is_none() {
        return Err("Invalid login file.");
    }
    Ok(login)
}

pub fn client() -> Result<Client> {
    Client::builder()
        .redirect(Policy::none())
        .retry(reqwest::retry::never())
        .timeout(Duration::from_secs(30))
        .build()
        .map_err(|_| "HTTP client unavailable.")
}

#[derive(Clone, Copy, Debug, Serialize)]
pub struct ApiError {
    pub code: &'static str,
    pub message: &'static str,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub status: Option<u16>,
    /// A mutation may already have committed. Reconcile the same ID and payload.
    pub outcome_unknown: bool,
}

fn reflects_token(value: &Value, token: &str) -> bool {
    match value {
        Value::String(value) => value.contains(token),
        Value::Array(values) => values.iter().any(|value| reflects_token(value, token)),
        Value::Object(values) => values
            .iter()
            .any(|(key, value)| key.contains(token) || reflects_token(value, token)),
        _ => false,
    }
}

pub fn request_detailed(
    client: &Client,
    login: &Login,
    path: &str,
    payload: Option<&Value>,
) -> std::result::Result<Value, ApiError> {
    let uncertain = |code, message| ApiError {
        code,
        message,
        status: None,
        outcome_unknown: payload.is_some(),
    };
    let url = format!("{}{path}", login.origin);
    let request = match payload {
        Some(payload) => client.post(url).json(payload),
        None => client.get(url),
    };
    let response = request.bearer_auth(&login.token).send().map_err(|_| {
        uncertain(
            "transport_error",
            "HTTP request failed; retry mutations with the same operation ID.",
        )
    })?;
    if !response.status().is_success() {
        let status = response.status().as_u16();
        let (code, message) = match status {
            401 => ("unauthorized", "Owner authorization required."),
            403 => ("forbidden", "Request refused."),
            409 => ("conflict", "Operation conflict."),
            422 => ("validation_failed", "API request failed."),
            300..=399 => ("redirect_refused", "API request failed."),
            _ => ("api_error", "API request failed."),
        };
        // A proxy failure is not evidence that the upstream mutation did not commit.
        return Err(ApiError {
            code,
            message,
            status: Some(status),
            outcome_unknown: payload.is_some() && !(400..500).contains(&status),
        });
    }
    let mut bytes = Vec::new();
    response
        .take(16 * 1024 * 1024 + 1)
        .read_to_end(&mut bytes)
        .map_err(|_| uncertain("invalid_response", "Cannot read API response."))?;
    if bytes.len() > 16 * 1024 * 1024 {
        return Err(uncertain("invalid_response", "API response too large."));
    }
    let value: Value = serde_json::from_slice(&bytes)
        .map_err(|_| uncertain("invalid_response", "API returned invalid JSON."))?;
    // Inspect decoded strings too: JSON escapes must not bypass secret refusal.
    if !login.token.is_empty() && reflects_token(&value, &login.token) {
        return Err(uncertain(
            "invalid_response",
            "API returned an unexpected response.",
        ));
    }
    Ok(value)
}

/// Preserve the CLI's existing JSON/error vocabulary while sharing all transport rules.
pub fn request(
    client: &Client,
    login: &Login,
    path: &str,
    payload: Option<&Value>,
) -> Result<Value> {
    request_detailed(client, login, path, payload).map_err(|error| error.message)
}
