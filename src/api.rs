//! Shared wire types and canonical route adapters; domain behavior stays in engine.
use crate::{model::*, owner_token::TokenInfo};
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

#[derive(Serialize, Deserialize)]
pub struct Identity {
    pub owner: bool,
    pub token: Option<TokenInfo>,
}
#[derive(Serialize, Deserialize)]
pub struct MutationResponse {
    pub result: MutationResult,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub practice: Option<serde_json::Value>,
    pub replayed: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub token: Option<TokenInfo>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub secret: Option<String>,
}
pub fn mutation_path(path: &str) -> Option<&str> {
    match path {
        "/api/v1/lessons/start" => Some("/practice/lesson/start"),
        "/api/v1/lessons/next" => Some("/practice/lesson/next"),
        "/api/v1/learn" => Some("/practice/learn"),
        "/api/v1/studies/pause" => Some("/practice/pause"),
        "/api/v1/studies/resume" => Some("/practice/resume"),
        "/api/v1/studies/drop" => Some("/practice/drop"),
        "/api/v1/next" => Some("/practice/next"),
        "/api/v1/answer" => Some("/practice/answer"),
        "/api/v1/reveal" => Some("/practice/reveal"),
        "/api/v1/correct" => Some("/practice/correct"),
        "/api/v1/library/mappings" => Some("/library/mapping/add"),
        "/api/v1/library/versions" => Some("/library/version/add"),
        "/api/v1/library/versions/supersede" => Some("/library/version/supersede"),
        "/api/v1/library/versions/delete" => Some("/library/version/delete"),
        "/api/v1/library/works/edit" => Some("/library/work/edit"),
        "/api/v1/library/works/delete" => Some("/library/work/delete"),
        "/api/v1/tokens" => Some("/tokens/create"),
        "/api/v1/tokens/revoke" => Some("/tokens/revoke"),
        "/api/v1/settings" => Some("/settings"),
        "/api/v1/create" => Some("/create"),
        _ => None,
    }
}
pub fn fields(value: serde_json::Value) -> AppResult<BTreeMap<String, String>> {
    let object = value
        .as_object()
        .ok_or_else(|| AppError::new(422, "Use a JSON object."))?;
    object
        .iter()
        .map(|(key, value)| {
            if key.starts_with("__") {
                return Err(AppError::new(422, "Invalid private field."));
            }
            let value = match value {
                serde_json::Value::String(s) => s.clone(),
                serde_json::Value::Bool(b) => b.to_string(),
                serde_json::Value::Number(n) => n.to_string(),
                _ => return Err(AppError::new(422, "Use scalar JSON fields.")),
            };
            Ok((key.clone(), value))
        })
        .collect()
}

/// Only Library draft metadata is nested. Other API field rules stay unchanged.
pub fn library_fields(mut value: serde_json::Value) -> AppResult<BTreeMap<String, String>> {
    if let Some(data) = value.get_mut("data") {
        if !data.is_object() {
            return Err(AppError::new(422, "Use a Library data object."));
        }
        *data = serde_json::Value::String(data.to_string());
    }
    fields(value)
}

pub fn library_read(app: &App, path: &str, query: &str) -> AppResult<serde_json::Value> {
    let library = &app.library;
    let value = if path == "/api/v1/library/search" {
        serde_json::to_value(library.search(query))
    } else if path == "/api/v1/library/export" {
        serde_json::to_value(library)
    } else if let Some(id) = path.strip_prefix("/api/v1/library/works/") {
        serde_json::to_value(library.show_work(id)?)
    } else if let Some(id) = path.strip_prefix("/api/v1/library/versions/") {
        serde_json::to_value(library.show_version(id)?)
    } else {
        return Err(AppError::new(404, "Library action not found."));
    };
    value.map_err(|_| AppError::new(500, "Cannot encode Library."))
}

/// Browser supplied-text fields become the same Library input as the API/CLI.
/// No segmentation, dedupe, validation or durable writes live in this adapter.
pub fn library_form_fields(fields: &mut BTreeMap<String, String>) -> AppResult<()> {
    let value = |key| fields.get(key).cloned().unwrap_or_default();
    let title = value("title");
    let text = value("text");
    if title.trim().is_empty() || title.len() > 240 {
        return Err(AppError::new(
            422,
            "Add a title of up to 240 bytes. Your text is still here.",
        ));
    }
    if text.trim().is_empty() || text.len() > crate::library::TEXT_BYTES {
        return Err(AppError::new(
            422,
            "Add text of up to 256 KiB. Your draft is still here.",
        ));
    }
    let source = value("source");
    let input = crate::library::VersionInput {
        title,
        text,
        name: value("name"),
        kind: crate::library::Kind::Text,
        sources: if source.trim().is_empty() {
            vec![]
        } else {
            vec![source]
        },
        aliases: vec![],
        context: String::new(),
        rights: crate::library::Rights::PrivateOnly,
        spelling_strict: false,
        ordered_aliases: Default::default(),
        sections: None,
    };
    fields.insert(
        "data".into(),
        serde_json::to_string(&input).map_err(|_| AppError::new(422, "Invalid supplied text."))?,
    );
    Ok(())
}
