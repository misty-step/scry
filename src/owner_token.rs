//! Owner credentials. Secrets exist only in the issuance response, never in App.
use crate::{model::*, persistence::sha256};
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct TokenInfo {
    pub id: String,
    pub name: String,
    pub created_ms: i64,
    pub last_used_ms: Option<i64>,
    pub expires_ms: Option<i64>,
    pub paid: bool,
    pub revoked_ms: Option<i64>,
}
#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct OwnerToken {
    #[serde(flatten)]
    pub info: TokenInfo,
    pub verifier: String,
}

pub fn issuance_secret(seed: &str) -> String {
    let id = &sha256(format!("owner-token-id:{seed}").as_bytes())[..32];
    format!("scry_ot_{id}_{seed}")
}
pub fn token_id(secret: &str) -> Option<&str> {
    let rest = secret.strip_prefix("scry_ot_")?;
    let (id, entropy) = rest.split_once('_')?;
    (id.len() == 32
        && entropy.len() == 48
        && id
            .bytes()
            .chain(entropy.bytes())
            .all(|b| b.is_ascii_hexdigit() && !b.is_ascii_uppercase()))
    .then_some(id)
}
impl OwnerToken {
    pub fn issue(
        name: &str,
        expires_ms: Option<i64>,
        paid: bool,
        now: i64,
        seed: &str,
    ) -> AppResult<Self> {
        if name.trim().is_empty()
            || name.len() > 120
            || name.chars().any(char::is_control)
            || expires_ms.is_some_and(|t| t <= now)
        {
            return Err(AppError::new(
                422,
                "Use a name and a future expiry, if any.",
            ));
        }
        let secret = issuance_secret(seed);
        let id =
            token_id(&secret).ok_or_else(|| AppError::new(500, "Token entropy unavailable."))?;
        Ok(Self {
            info: TokenInfo {
                id: id.into(),
                name: name.trim().into(),
                created_ms: now,
                last_used_ms: None,
                expires_ms,
                paid,
                revoked_ms: None,
            },
            verifier: sha256(secret.as_bytes()),
        })
    }
    pub fn active(&self, now: i64) -> bool {
        self.info.revoked_ms.is_none() && self.info.expires_ms.is_none_or(|t| now < t)
    }
    pub fn verifies(&self, secret: &str, now: i64) -> bool {
        let candidate = sha256(secret.as_bytes());
        let difference = candidate
            .bytes()
            .zip(self.verifier.bytes())
            .fold(0u8, |acc, (a, b)| acc | (a ^ b));
        self.verifier.len() == candidate.len()
            && difference == 0
            && self.active(now)
            && token_id(secret) == Some(self.info.id.as_str())
    }
}

#[derive(Clone, Copy)]
pub enum MutationActor<'a> {
    Browser,
    Token(&'a TokenInfo),
}
impl MutationActor<'_> {
    pub fn token_id(self) -> Option<String> {
        match self {
            Self::Browser => None,
            Self::Token(info) => Some(info.id.clone()),
        }
    }
    pub fn paid(self) -> bool {
        match self {
            Self::Browser => true,
            Self::Token(info) => info.paid,
        }
    }
}
pub fn paid_refusal() -> AppError {
    AppError::new(403, "This Owner token has no paid permission.")
}
