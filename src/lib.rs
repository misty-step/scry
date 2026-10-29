pub mod api;
pub mod engine;
pub mod generation;
pub mod grading;
pub mod learning;
pub mod library;
#[cfg(not(target_arch = "wasm32"))]
pub mod mcp;
pub mod model;
pub mod nato_progress;
#[cfg(not(target_arch = "wasm32"))]
pub mod owner_client;
pub mod owner_token;
pub mod persistence;
pub mod spend;
pub mod study;
pub mod web;

#[cfg(target_arch = "wasm32")]
mod runtime;

pub mod lessons;
pub mod ordered;
