//! Optional, explicitly configured private learner. Configuration is not signup.
//! No email aliases, client-selected space names or shared owner history.
use crate::persistence;

pub const QA_SPACE_NAME: &str = "scry-qa";

/// The principal's immutable Access subject determines a stable private object.
/// Missing, malformed, owner-colliding or reserved-space configuration is inert.
pub fn space_name(subject: &str, owner_subject: &str, owner_space: &str) -> Option<String> {
    if subject.is_empty()
        || subject.len() > 128
        || subject == owner_subject
        || !subject
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
    {
        return None;
    }
    let name = format!("scry-learner-{}", persistence::sha256(subject.as_bytes()));
    (name != owner_space && name != QA_SPACE_NAME).then_some(name)
}

/// This selects a candidate only; the runtime still verifies the ordinary JWT
/// signature/issuer/audience/lifetime before admitting any request. Empty
/// service-identity subjects can never match.
pub fn matches(subject: &str, configured: &str, owner: &str, owner_space: &str) -> bool {
    subject == configured && space_name(configured, owner, owner_space).is_some()
}

/// Learners can manage their own learning, never operator or owner-token APIs.
pub fn owner_only_path(path: &str) -> bool {
    path == "/operator"
        || path.starts_with("/operator/")
        || path == "/tokens"
        || path.starts_with("/tokens/")
        || path == "/internal/backup"
        || path.starts_with("/__internal/")
        || path.starts_with("/api/v1/")
}
