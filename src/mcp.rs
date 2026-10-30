//! Bounded MCP 2025-11-25 stdio adapter, not another grading or content engine.
//! Only supplied content and native practice are exposed. No prompts, sampling,
//! token-management, generation, self-grading, arbitrary HTTP or filesystem tools.
mod schema;

use crate::{
    library::{Library, MappingInput, VersionInput},
    owner_client,
    persistence::sha256,
};
use reqwest::{Url, blocking::Client};
use serde::{Deserialize, de::DeserializeOwned};
use serde_json::{Value, json};
use std::{
    collections::BTreeSet,
    io::{BufRead, Write},
    path::PathBuf,
};

pub const PROTOCOL_VERSION: &str = "2025-11-25";
pub const MAX_MESSAGE_BYTES: usize = 4 * 1024 * 1024;
const MAX_SESSION_REQUESTS: usize = 10_000;
const INSTRUCTIONS: &str = "Scry stores externally authored material and grades raw answers natively. Search before import; validation is local, not remote admission. Retain every mutation's operation_id and exact arguments before sending. Never replace an unknown mutation with a new ID. Reconcile the same operation and payload BEFORE Next; older answer-bearing replays can be fenced by a newer unanswered unaided Prompt. Reveal records assistance. No generation or caller-supplied grades. Hosted Owner API admission is not established by this local adapter.";

#[derive(Default)]
enum Phase {
    #[default]
    New,
    Initialized,
    Ready,
}

struct Session {
    phase: Phase,
    ids: BTreeSet<String>,
    config: PathBuf,
    client: Client,
    /// Freeze origin + principal capability without retaining/logging the token.
    login_fingerprint: Option<String>,
    /// Only a digest, never the raw answer or credential. Session-local guard;
    /// clients retain the frozen operation durably across process restarts.
    pending: Option<String>,
}

fn rpc_error(id: Value, code: i32, message: &'static str) -> Value {
    json!({"jsonrpc":"2.0","id":id,"error":{"code":code,"message":message}})
}
fn result(value: Value, failed: bool) -> Value {
    json!({"content":[{"type":"text","text":value.to_string()}],"structuredContent":value,"isError":failed})
}
fn tool_error(code: &'static str, message: &'static str) -> Value {
    result(
        json!({"error":{"code":code,"message":message,"outcome_unknown":false}}),
        true,
    )
}
type InputResult<T> = std::result::Result<T, &'static str>;
fn parse<T: DeserializeOwned>(value: Value) -> InputResult<T> {
    serde_json::from_value(value)
        .map_err(|_| "Use the tool's inputSchema; unknown fields are refused.")
}
fn operation(value: &str) -> InputResult<()> {
    if !(16..=128).contains(&value.len())
        || !value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
    {
        return Err("operation_id must be 16–128 ASCII letters, digits, hyphens or underscores.");
    }
    Ok(())
}
fn identity(value: &str) -> InputResult<()> {
    if value.len() != 32 || !value.bytes().all(|b| b.is_ascii_hexdigit()) {
        return Err("Use a 32-character entity ID returned by Scry.");
    }
    Ok(())
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Empty {}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Search {
    query: String,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Show {
    kind: String,
    id: String,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Manifest {
    format: String,
    data: Value,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Import {
    operation_id: String,
    format: String,
    data: Value,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Learn {
    operation_id: String,
    version_id: String,
    pace: Option<String>,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct LessonStart {
    operation_id: String,
    starter: Option<String>,
    version_id: Option<String>,
    after: Option<String>,
    leave: Option<String>,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct LessonNext {
    operation_id: String,
    after: String,
    mode: Option<String>,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Next {
    operation_id: String,
    after: Option<String>,
    study_id: Option<String>,
    card_id: Option<String>,
    recite: Option<String>,
    section_id: Option<String>,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Answer {
    operation_id: String,
    prompt_id: String,
    answer: String,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Reveal {
    operation_id: String,
    prompt_id: String,
}

/// The same domain validator used by import, in a disposable empty Library.
/// This deliberately proves no remote deduplication, authorization or capacity.
fn validate(format: &str, data: Value) -> InputResult<Value> {
    let input = match format {
        "version" => parse::<VersionInput>(data)?,
        "mapping" => parse::<MappingInput>(data)?.into_version()
            .map_err(|_| "Invalid mapping: use unique keys, direction forward, and nonempty key/value pairs without control characters or surrounding whitespace.")?,
        _ => return Err("format must be version or mapping."),
    };
    let mut library = Library::default();
    let id = library.add(input, "local-mcp-validation", None)
        .map_err(|_| "Invalid Library draft: check metadata and byte/count limits; Sections must join to text exactly.")?;
    let version = &library.versions[&id];
    Ok(
        json!({"valid":true,"scope":"local_structure_only","remote_admission_checked":false,
        "kind":version.kind,"text_bytes":version.text.len(),"sections":version.sections.len(),
        "units":version.sections.iter().map(|s| s.units.len()).sum::<usize>()}),
    )
}

enum Call {
    Local(Value),
    Api {
        path: String,
        payload: Option<Value>,
    },
}
fn read(path: impl Into<String>) -> Call {
    Call::Api {
        path: path.into(),
        payload: None,
    }
}
fn write(path: &str, payload: Value) -> Call {
    Call::Api {
        path: path.into(),
        payload: Some(payload),
    }
}
fn prepare(name: &str, args: Value) -> InputResult<Call> {
    match name {
        "scry_capabilities" => {
            parse::<Empty>(args)?;
            Ok(Call::Local(
                json!({"adapter":"scry-mcp","protocol_version":PROTOCOL_VERSION,
                "formats":["version","mapping"],"grading":"native_only","generation":false,
                "self_grading":false,"multimedia":false,"mapping_direction":"forward",
                "remote_capabilities_checked":false,"hosted_admission":"unverified",
                "instructions":INSTRUCTIONS,"max_message_bytes":MAX_MESSAGE_BYTES,
                "mutation_ids":"required; preserve across retries and restarts",
                "cancellation":"admitted synchronous HTTP calls cannot be cancelled; no automatic retry"}),
            ))
        }
        "scry_schema" => {
            parse::<Empty>(args)?;
            Ok(Call::Local(schema::manifests()))
        }
        "scry_search" => {
            let input: Search = parse(args)?;
            if input.query.len() > 4096 {
                return Err("query must be at most 4096 UTF-8 bytes.");
            }
            let mut url = Url::parse("https://scry.invalid/api/v1/library/search").unwrap();
            url.query_pairs_mut().append_pair("q", &input.query);
            Ok(read(format!(
                "{}?{}",
                url.path(),
                url.query().unwrap_or_default()
            )))
        }
        "scry_show" => {
            let input: Show = parse(args)?;
            identity(&input.id)?;
            let resource = match input.kind.as_str() {
                "work" => "works",
                "version" => "versions",
                _ => return Err("kind must be work or version."),
            };
            Ok(read(format!("/api/v1/library/{resource}/{}", input.id)))
        }
        "scry_validate" => {
            let input: Manifest = parse(args)?;
            Ok(Call::Local(validate(&input.format, input.data)?))
        }
        "scry_import" => {
            let input: Import = parse(args)?;
            operation(&input.operation_id)?;
            validate(&input.format, input.data.clone())?;
            let path = if input.format == "mapping" {
                "/api/v1/library/mappings"
            } else {
                "/api/v1/library/versions"
            };
            Ok(write(
                path,
                json!({"operation_id":input.operation_id,"data":input.data}),
            ))
        }
        "scry_learn" => {
            let input: Learn = parse(args)?;
            operation(&input.operation_id)?;
            identity(&input.version_id)?;
            let mut payload =
                json!({"operation_id":input.operation_id,"version_id":input.version_id});
            if let Some(pace) = input.pace {
                if !matches!(pace.as_str(), "light" | "steady" | "intense") {
                    return Err("pace must be light, steady or intense.");
                }
                payload["pace"] = json!(pace);
            }
            Ok(write("/api/v1/learn", payload))
        }
        "scry_lesson_start" => {
            let input: LessonStart = parse(args)?;
            operation(&input.operation_id)?;
            if input.starter.is_some() == input.version_id.is_some()
                || input.after.is_some() && input.leave.is_some()
            {
                return Err("Choose exactly one starter or version_id.");
            }
            if let Some(starter) = &input.starter
                && (starter.is_empty()
                    || starter.len() > 128
                    || !starter
                        .bytes()
                        .all(|b| b.is_ascii_alphanumeric() || b == b'-'))
            {
                return Err("Use a starter ID of up to 128 ASCII letters, digits or hyphens.");
            }
            for value in [&input.version_id, &input.after, &input.leave]
                .into_iter()
                .flatten()
            {
                identity(value)?;
            }
            let mut payload = json!({"operation_id":input.operation_id});
            for (key, value) in [
                ("starter", input.starter),
                ("version_id", input.version_id),
                ("after", input.after),
                ("leave", input.leave),
            ] {
                if let Some(value) = value {
                    payload[key] = json!(value);
                }
            }
            Ok(write("/api/v1/lessons/start", payload))
        }
        "scry_lesson_next" => {
            let input: LessonNext = parse(args)?;
            operation(&input.operation_id)?;
            identity(&input.after)?;
            let mut payload = json!({"operation_id":input.operation_id,"after":input.after});
            if let Some(mode) = input.mode {
                if !matches!(
                    mode.as_str(),
                    "phrases" | "retry" | "join" | "whole" | "middle"
                ) {
                    return Err("Use lesson mode phrases, retry, join, whole or middle.");
                }
                payload["mode"] = json!(mode);
            }
            Ok(write("/api/v1/lessons/next", payload))
        }
        "scry_due" => {
            parse::<Empty>(args)?;
            Ok(read("/api/v1/due"))
        }
        "scry_next" => {
            let input: Next = parse(args)?;
            operation(&input.operation_id)?;
            for id in [
                &input.after,
                &input.study_id,
                &input.card_id,
                &input.section_id,
            ]
            .into_iter()
            .flatten()
            {
                identity(id)?;
            }
            if input.card_id.is_some() && input.recite.is_some()
                || input
                    .recite
                    .as_deref()
                    .is_some_and(|s| !matches!(s, "whole" | "section"))
                || (input.recite.as_deref() == Some("section")) != input.section_id.is_some()
            {
                return Err(
                    "Use card_id or recite whole|section; section_id is required only for recite section.",
                );
            }
            let mut payload = json!({"operation_id":input.operation_id});
            for (field, value) in [
                ("after", input.after),
                ("study_id", input.study_id),
                ("card_id", input.card_id),
                ("recite", input.recite),
                ("section_id", input.section_id),
            ] {
                if let Some(value) = value {
                    payload[field] = json!(value);
                }
            }
            Ok(write("/api/v1/next", payload))
        }
        "scry_answer" => {
            let input: Answer = parse(args)?;
            operation(&input.operation_id)?;
            identity(&input.prompt_id)?;
            if input.answer.len() > crate::library::TEXT_BYTES {
                return Err("answer must be at most 256 KiB of UTF-8; an empty answer is a miss.");
            }
            Ok(write(
                "/api/v1/answer",
                json!({"operation_id":input.operation_id,"prompt_id":input.prompt_id,"answer":input.answer}),
            ))
        }
        "scry_reveal" => {
            let input: Reveal = parse(args)?;
            operation(&input.operation_id)?;
            identity(&input.prompt_id)?;
            Ok(write(
                "/api/v1/reveal",
                json!({"operation_id":input.operation_id,"prompt_id":input.prompt_id}),
            ))
        }
        _ => unreachable!("tool name checked before prepare"),
    }
}

impl Session {
    fn call(&mut self, name: &str, args: Value) -> Value {
        let call = match prepare(name, args) {
            Ok(call) => call,
            Err(message) => return tool_error("invalid_arguments", message),
        };
        let (path, payload) = match call {
            Call::Local(value) => return result(value, false),
            Call::Api { path, payload } => (path, payload),
        };
        let fingerprint = payload
            .as_ref()
            .map(|value| sha256(format!("{path}\n{value}").as_bytes()));
        if fingerprint.is_some()
            && self
                .pending
                .as_ref()
                .is_some_and(|pending| Some(pending) != fingerprint.as_ref())
        {
            return result(
                json!({"error":{"code":"reconciliation_required","message":"Reconcile the unknown mutation with its original operation_id and exact payload before another mutation, especially Next.","outcome_unknown":true}}),
                true,
            );
        }
        let login = match owner_client::load(&self.config) {
            Ok(login) => login,
            Err(message) => {
                return result(
                    json!({"error":{
                        "code":"login_required","message":message,
                        "outcome_unknown":self.pending.is_some() && fingerprint.is_some()
                    }}),
                    true,
                );
            }
        };
        let login_fingerprint = sha256(format!("{}\n{}", login.origin, login.token).as_bytes());
        if self
            .login_fingerprint
            .as_ref()
            .is_some_and(|saved| saved != &login_fingerprint)
        {
            return result(
                json!({"error":{
                    "code":"login_changed",
                    "message":"The login origin or credential changed. Restore the original login and reconcile pending operations with the original principal before starting another MCP process.",
                    "outcome_unknown":self.pending.is_some()
                }}),
                true,
            );
        }
        self.login_fingerprint.get_or_insert(login_fingerprint);
        match owner_client::request_detailed(&self.client, &login, &path, payload.as_ref()) {
            Ok(value) => {
                // A 2xx proxy object is not a committed receipt. Reuse the native
                // wire type before clearing an unknown mutation; do not recreate
                // the engine's response schema here.
                let receipt = payload.is_none()
                    || serde_json::from_value::<crate::api::MutationResponse>(value.clone())
                        .is_ok();
                if !value.is_object()
                    || value.get("error").is_some()
                    || value.get("secret").is_some()
                    || !receipt
                {
                    if fingerprint.is_some() {
                        self.pending = fingerprint;
                    }
                    return result(
                        json!({"error":{"code":"invalid_response","message":"API returned an unexpected response.","outcome_unknown":payload.is_some()}}),
                        true,
                    );
                }
                if fingerprint.is_some() {
                    self.pending = None;
                }
                result(value, false)
            }
            Err(mut error) => {
                // Refusal of this retry cannot settle the earlier unknown write.
                error.outcome_unknown |= self.pending.is_some() && fingerprint.is_some();
                if error.outcome_unknown {
                    self.pending = fingerprint;
                }
                let mut value = json!({"error":error});
                if error.code == "conflict" {
                    value["error"]["recovery"] = json!(
                        "Keep the original ID and payload. A newer unanswered unaided Prompt fences older answer-bearing results. If another client advanced, finish that Prompt through the Owner API/CLI, then retry the original operation here. An unresolved mutation still blocks other MCP mutations. Do not invent a new ID to bypass a conflict."
                    );
                } else if error.outcome_unknown {
                    value["error"]["recovery"] = json!(
                        "Retry only the same operation_id and exact payload before Next. No retry was performed by this adapter."
                    );
                }
                result(value, true)
            }
        }
    }

    fn message(&mut self, value: Value) -> Option<Value> {
        let Some(object) = value.as_object() else {
            return Some(rpc_error(Value::Null, -32600, "Invalid request."));
        };
        let valid_id = object
            .get("id")
            .filter(|v| v.as_str().is_some_and(|s| s.len() <= 256) || v.is_i64() || v.is_u64());
        let id = valid_id.cloned().unwrap_or(Value::Null);
        if object.get("jsonrpc").and_then(Value::as_str) != Some("2.0")
            || !object.get("method").is_some_and(Value::is_string)
            || object.contains_key("result")
            || object.contains_key("error")
            || (object.contains_key("id") && valid_id.is_none())
        {
            return Some(rpc_error(id, -32600, "Invalid request."));
        }
        let method = object["method"].as_str().unwrap();
        let params = object.get("params").cloned().unwrap_or(json!({}));
        if !object.contains_key("id") {
            if method == "notifications/initialized"
                && params.is_object()
                && matches!(self.phase, Phase::Initialized)
            {
                self.phase = Phase::Ready;
            }
            // Cancellation is optional. The admitted blocking HTTP call cannot be
            // cancelled and completes within 30s; never log the supplied reason.
            // Unknown/completed cancellation and all other notifications are ignored.
            return None;
        }
        if self.ids.len() >= MAX_SESSION_REQUESTS {
            return Some(rpc_error(
                id,
                -32000,
                "Session request limit reached; reconnect.",
            ));
        }
        if !self.ids.insert(id.to_string()) {
            return Some(rpc_error(
                id,
                -32600,
                "Request IDs must be unique within a session.",
            ));
        }
        let outcome = if !params.is_object() {
            Err((-32602, "Use object params."))
        } else if method == "ping" {
            Ok(json!({}))
        } else if method == "initialize" {
            if !matches!(self.phase, Phase::New) {
                Err((-32600, "Already initialized."))
            } else if !params.get("protocolVersion").is_some_and(Value::is_string)
                || !params.get("capabilities").is_some_and(Value::is_object)
                || !params
                    .get("clientInfo")
                    .and_then(|c| c.get("name"))
                    .is_some_and(Value::is_string)
                || !params
                    .get("clientInfo")
                    .and_then(|c| c.get("version"))
                    .is_some_and(Value::is_string)
            {
                Err((-32602, "Invalid initialize parameters."))
            } else {
                self.phase = Phase::Initialized;
                Ok(
                    json!({"protocolVersion":PROTOCOL_VERSION,"capabilities":{"tools":{}},
                    "serverInfo":{"name":"scry-mcp","version":env!("CARGO_PKG_VERSION")},"instructions":INSTRUCTIONS}),
                )
            }
        } else if !matches!(self.phase, Phase::Ready) {
            Err((
                -32000,
                "Initialize and send notifications/initialized first.",
            ))
        } else if method == "tools/list" {
            if params.get("cursor").is_some() {
                Err((-32602, "This tool list has no pagination cursor."))
            } else {
                Ok(json!({"tools":schema::tools()}))
            }
        } else if method == "tools/call" {
            if params.get("task").is_some() {
                Err((-32602, "Task augmentation is not supported."))
            } else if let Some(name) = params
                .get("name")
                .and_then(Value::as_str)
                .filter(|name| schema::NAMES.contains(name))
            {
                Ok(self.call(name, params.get("arguments").cloned().unwrap_or(json!({}))))
            } else {
                Err((-32602, "Unknown tool."))
            }
        } else {
            Err((-32601, "Method not found."))
        };
        Some(match outcome {
            Ok(value) => json!({"jsonrpc":"2.0","id":id,"result":value}),
            Err((code, message)) => rpc_error(id, code, message),
        })
    }
}

enum Frame {
    End,
    Message(Vec<u8>),
    Invalid,
}
/// Read one newline-delimited frame with a hard allocation bound, draining an
/// oversized frame before accepting another. Never echo malformed input.
fn frame(input: &mut impl BufRead) -> std::io::Result<Frame> {
    let mut bytes = Vec::new();
    let mut oversized = false;
    loop {
        let buffer = input.fill_buf()?;
        if buffer.is_empty() {
            return Ok(if bytes.is_empty() && !oversized {
                Frame::End
            } else {
                Frame::Invalid
            });
        }
        let newline = buffer.iter().position(|b| *b == b'\n');
        let take = newline.map_or(buffer.len(), |i| i + 1);
        if !oversized {
            if bytes.len() + take > MAX_MESSAGE_BYTES {
                oversized = true;
                bytes.clear();
            } else {
                bytes.extend_from_slice(&buffer[..take]);
            }
        }
        input.consume(take);
        if newline.is_some() {
            return Ok(if oversized {
                Frame::Invalid
            } else {
                Frame::Message(bytes)
            });
        }
    }
}

pub fn serve(config: PathBuf) -> owner_client::Result<()> {
    let mut session = Session {
        phase: Phase::New,
        ids: BTreeSet::new(),
        config,
        client: owner_client::client()?,
        login_fingerprint: None,
        pending: None,
    };
    let stdin = std::io::stdin();
    let stdout = std::io::stdout();
    let mut input = stdin.lock();
    let mut output = stdout.lock();
    loop {
        let response = match frame(&mut input).map_err(|_| "Cannot read MCP input.")? {
            Frame::End => return Ok(()),
            Frame::Invalid => Some(rpc_error(
                Value::Null,
                -32700,
                "Invalid or oversized JSON frame.",
            )),
            Frame::Message(bytes) => match serde_json::from_slice(&bytes) {
                Ok(value) => session.message(value),
                Err(_) => Some(rpc_error(Value::Null, -32700, "Invalid JSON.")),
            },
        };
        if let Some(response) = response {
            writeln!(output, "{response}")
                .and_then(|_| output.flush())
                .map_err(|_| "Cannot write MCP output.")?;
        }
    }
}

#[cfg(test)]
mod lesson_tests {
    use super::*;
    #[test]
    fn lesson_tools_share_native_routes_and_reject_self_grades() {
        let op = "lesson-mcp-operation-001";
        for (name, args, path) in [
            (
                "scry_lesson_start",
                json!({"operation_id":op,"starter":"rosary-joyful"}),
                "/api/v1/lessons/start",
            ),
            (
                "scry_lesson_next",
                json!({"operation_id":op,"after":"a".repeat(32),"mode":"whole"}),
                "/api/v1/lessons/next",
            ),
            (
                "scry_lesson_next",
                json!({"operation_id":op,"after":"a".repeat(32),"mode":"retry"}),
                "/api/v1/lessons/next",
            ),
        ] {
            match prepare(name, args).unwrap() {
                Call::Api {
                    path: actual,
                    payload,
                } => {
                    assert_eq!(actual, path);
                    assert_eq!(payload.unwrap()["operation_id"], op);
                }
                _ => panic!("not API"),
            }
        }
        assert!(
            prepare(
                "scry_lesson_start",
                json!({"operation_id":op,"starter":"rosary-joyful","grade":"Good"})
            )
            .is_err()
        );
        assert!(
            prepare(
                "scry_lesson_start",
                json!({"operation_id":op,"starter":"rosary-joyful","version_id":"a".repeat(32)})
            )
            .is_err()
        );
        assert!(
            prepare(
                "scry_lesson_next",
                json!({"operation_id":op,"after":"a".repeat(32),"mode":"automatic"})
            )
            .is_err()
        );
        assert!(
            schema::tools()
                .iter()
                .any(|t| t["name"] == "scry_lesson_start")
        );
    }
}
