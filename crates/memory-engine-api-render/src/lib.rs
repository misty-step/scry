#![cfg_attr(not(test), deny(clippy::expect_used, clippy::unwrap_used))]

//! Server-rendered HTML for the Memory Engine HTTP API.

mod render;

#[cfg(test)]
mod design_preview;

pub const LEDGER_CSS: &str = include_str!("../assets/ledger.css");
pub const LITERATA_WOFF2: &[u8] = include_bytes!("../assets/fonts/literata-latin-variable.woff2");
pub const MANROPE_WOFF2: &[u8] = include_bytes!("../assets/fonts/manrope-latin-variable.woff2");
pub const FONT_LICENSE: &str = include_str!("../assets/fonts/OFL.txt");
pub const PWA_ICON_192: &[u8] = include_bytes!("../assets/icons/icon-192.png");
pub const PWA_ICON_512: &[u8] = include_bytes!("../assets/icons/icon-512.png");
pub const APPLE_TOUCH_ICON: &[u8] = include_bytes!("../assets/icons/apple-touch-icon.png");
pub const FAVICON: &[u8] = include_bytes!("../assets/icons/favicon.png");

pub use render::{
    is_generating_notice, render_account_page, render_analytics_page, render_app_shell,
    render_auth_recovery, render_capture_waiting_page, render_content_feedback_recovery_html,
    render_content_feedback_result_html, render_create_page, render_edit_review_html,
    render_entry_recovery, render_entry_requested, render_library_page, render_reference_page,
    render_return_notification_confirmation, render_return_notification_disabled,
    render_return_notification_recovery, render_submit_action_result_html, render_submit_recovery,
    AnalyticsConceptFilter, AnalyticsConceptSort, AnalyticsViewOptions, ContentFeedbackRecovery,
    SKIP_CONFIRM_NOTICE, SNOOZE_CONCEPT_CONFIRM_NOTICE, SNOOZE_CONFIRM_NOTICE,
};
