//! `OpenRouter` content contract with an optional native HTTP adapter.
//!
//! [`content`] contains portable authorization, bounded request construction,
//! strict schemas, completion parsing, and semantic content gates. Worker Fetch
//! clients disable default features and use exactly the same contract.
//!
//! The default `native` feature exposes the blocking `OpenRouterProvider`
//! adapter used by local tools and offline HTTP regressions.

pub mod content;
pub use content::{PromptVariant, StructuredResponse};
#[cfg(feature = "native")]
mod native;
#[cfg(feature = "native")]
pub use native::{OpenRouterConfig, OpenRouterProvider};

/// Environment variable holding the `OpenRouter` API key.
pub const API_KEY_ENV: &str = "OPENROUTER_API_KEY";
/// One-run bearer capability for the trusted hosted-eval provider proxy.
pub const PROXY_TOKEN_ENV: &str = "OPENROUTER_PROXY_TOKEN";
/// Environment variable overriding the generation model id.
pub const MODEL_ENV: &str = "MEMORY_ENGINE_GENERATION_MODEL";
/// Default model. Operator ruling 2026-08-17: `google/gemini-3.7-flash`.
/// Override with `MEMORY_ENGINE_GENERATION_MODEL`. Historical model comparisons
/// remain in `docs/evals/generation-field-2026-06-11.md`; they are not current
/// endpoint-availability or pricing guarantees.
pub const DEFAULT_MODEL: &str = "google/gemini-3.7-flash";
/// Default `OpenRouter` API endpoint; transports may override it explicitly.
pub const DEFAULT_BASE_URL: &str = "https://openrouter.ai/api/v1";
/// Trusted hosted evaluation may replace the upstream with a local provider
/// proxy. The proxy owns the real key; target code receives only a one-run
/// capability token as `OPENROUTER_PROXY_TOKEN`.
pub const BASE_URL_ENV: &str = "OPENROUTER_BASE_URL";
/// Unix socket for the trusted hosted-eval proxy. The target gets a bounded
/// capability token, never the provider key or general network access.
pub const PROXY_SOCKET_ENV: &str = "OPENROUTER_PROXY_SOCKET";
