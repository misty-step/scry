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
        "/api/v1/concepts/create" => Some("/concepts/create"),
        "/api/v1/materials/add" => Some("/materials/add"),
        "/api/v1/materials/revise" => Some("/materials/revise"),
        "/api/v1/learn" => Some("/practice/learn"),
        "/api/v1/studies/pause" => Some("/practice/pause"),
        "/api/v1/studies/resume" => Some("/practice/resume"),
        "/api/v1/studies/drop" => Some("/practice/drop"),
        "/api/v1/next" => Some("/practice/next"),
        "/api/v1/answer" => Some("/practice/answer"),
        "/api/v1/reveal" => Some("/practice/reveal"),
        "/api/v1/correct" => Some("/practice/correct"),
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
        sections: None,
    };
    fields.insert(
        "data".into(),
        serde_json::to_string(&input).map_err(|_| AppError::new(422, "Invalid supplied text."))?,
    );
    Ok(())
}

/// New reference reads never initialize a stream, record exposure, or write token metadata.
pub fn material_read_path(path: &str) -> bool {
    path == "/api/v1/concepts"
        || path.starts_with("/api/v1/concepts/")
        || path == "/api/v1/materials"
        || path.starts_with("/api/v1/materials/")
}

pub fn material_read(app: &App, path: &str) -> AppResult<serde_json::Value> {
    use serde_json::json;
    if app.practice.protected() {
        return Err(AppError::new(
            409,
            "Finish your Prompt or record assistance before reading Materials.",
        ));
    }
    let check = |id: &str| -> AppResult<()> {
        if app.protected_concept(id) {
            Err(AppError::new(
                409,
                "Record assistance before reading this Concept.",
            ))
        } else {
            Ok(())
        }
    };
    let parts: Vec<_> = path.trim_matches('/').split('/').collect();
    match parts.as_slice() {
        ["api", "v1", "concepts"] => {
            if app.protected_goal().is_some() {
                return Err(AppError::new(
                    409,
                    "Record assistance before listing Concepts.",
                ));
            }
            Ok(json!({"concepts": app.concepts.values().collect::<Vec<_>>()}))
        }
        ["api", "v1", "concepts", id] | ["api", "v1", "concepts", id, "materials"] => {
            let concept = app
                .concepts
                .get(*id)
                .ok_or_else(|| AppError::new(404, "Concept not found."))?;
            check(id)?;
            let materials = crate::materials::for_concept(app, concept);
            if parts.len() == 4 {
                Ok(json!({"concept":concept,"materials":materials}))
            } else {
                Ok(json!({"materials":materials}))
            }
        }
        ["api", "v1", "materials", id]
        | ["api", "v1", "materials", id, "revisions"]
        | ["api", "v1", "materials", id, "revisions", _] => {
            let material = app
                .materials
                .items
                .get(*id)
                .ok_or_else(|| AppError::new(404, "Material not found."))?;
            check(&material.concept_id)?;
            if let Some(revision_id) = parts.get(5) {
                let revision = app
                    .materials
                    .revisions
                    .get(*revision_id)
                    .filter(|r| r.material_id == material.id)
                    .ok_or_else(|| AppError::new(404, "Revision not found for this Material."))?;
                Ok(json!({"material":material,"revision":revision}))
            } else {
                let mut revisions: Vec<_> = app
                    .materials
                    .revisions
                    .values()
                    .filter(|r| r.material_id == material.id)
                    .collect();
                revisions.sort_by_key(|r| r.ordinal);
                Ok(json!({"material":material,"revisions":revisions}))
            }
        }
        _ => Err(AppError::new(404, "Concept or Material action not found.")),
    }
}
