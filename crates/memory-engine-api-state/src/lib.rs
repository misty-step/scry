#![cfg_attr(not(test), deny(clippy::expect_used, clippy::unwrap_used))]

//! Shared API data with optional native auth, storage, and job adapters.
//!
//! Renderers and Worker clients disable default features to use the same public
//! models without Postgres, native HTTP, threads, or a Tokio runtime. The default
//! `native` feature retains the complete local API implementation.

pub mod model;
pub use model::*;

#[cfg(feature = "native")]
mod native;
#[cfg(feature = "native")]
pub use native::{
    browser_session_cookie_header_for_request, browser_session_cookie_present,
    html_with_browser_session, html_with_browser_session_for_request,
    html_with_cleared_browser_session, html_with_cleared_browser_session_for_request,
    init_error_reporting, read_session_token, report_health_check_in,
    report_submit_browser_performance, report_submit_server_performance, request_is_secure,
    shutdown_error_reporting, start_health_reporting_loop, AccountRegistry, ApiState, AuthConfig,
    AuthLinkDelivery, EnqueueOutcome, JobBroadcast, JobQueue, OpenRouterConfig,
    ReturnNotificationSchedulerConfig, SchedulerHandle, StudyStorage,
};
