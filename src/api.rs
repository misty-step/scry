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
    pub replayed: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub token: Option<TokenInfo>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub secret: Option<String>,
}
pub fn mutation_path(path: &str) -> Option<&str> {
    match path {
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
