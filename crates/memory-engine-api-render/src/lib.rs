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

// Compile-time FNV-1a cache tags, not integrity checks: no dependency,
// per-render hashing, or manual version to forget when CSS or JS changes.
pub(crate) const LEDGER_CSS_VERSION: u64 = asset_content_tag(LEDGER_CSS.as_bytes());
pub(crate) const APP_JS_VERSION: u64 =
    asset_content_tag(include_bytes!("../../memory-engine-api/assets/app.js"));

const fn asset_content_tag(bytes: &[u8]) -> u64 {
    let mut hash = 0xcbf2_9ce4_8422_2325_u64;
    let mut index = 0;
    while index < bytes.len() {
        hash ^= bytes[index] as u64;
        hash = hash.wrapping_mul(0x0000_0100_0000_01b3);
        index += 1;
    }
    hash
}

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
