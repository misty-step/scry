//! Fail-closed paid-send admission. Spend remains the only money ledger.
use crate::{
    engine::{self, Work, WorkConfig},
    model::*,
};
use chrono::Datelike;
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};

// This is a reviewed provider contract, not a production configuration. No
// policy is supplied by default. See docs/spend.md for its activation gates.
pub const CONTRACT: &str = "openrouter-gpt41-mini-text-2026-10-24";
pub const MODEL: &str = "openai/gpt-4.1-mini";
pub const ENDPOINT: &str = "https://openrouter.ai/api/v1/chat/completions";
const CONTEXT_TOKENS: u64 = 1_047_576;
const OUTPUT_TOKENS: u64 = 12_000;
// Exact integer USD/million ceilings: one token costs at most this many
// microdollars. Cache reads cost less; this contract has no cache-write fee.
const INPUT_PRICE: u64 = 1;
const OUTPUT_PRICE: u64 = 2;
const CONTRACT_END_MS: i64 = 1_795_996_800_000; // 2026-11-30 00:00 UTC

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
pub struct Policy {
    pub contract: String,
    pub monthly_micros: u64,
    pub request_micros: u64,
}
impl Policy {
    pub fn parse(raw: &str) -> AppResult<Self> {
        serde_json::from_str(raw)
            .map_err(|_| refusal("Paid AI is off: an explicit valid budget policy is required."))
    }
    pub fn validate(&self, now: i64) -> AppResult<()> {
        if self.contract != CONTRACT
            || now >= CONTRACT_END_MS
            || self.monthly_micros == 0
            || self.request_micros == 0
            || self.request_micros > self.monthly_micros
        {
            return Err(refusal(
                "Paid AI is off: the budget or reviewed provider contract is unavailable.",
            ));
        }
        Ok(())
    }
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
pub struct Quote {
    pub policy: Policy,
    pub month: String,
    pub request_hash: String,
    pub maximum_micros: u64,
}
fn refusal(message: &str) -> AppError {
    AppError::new(429, message)
}
fn month(now: i64) -> AppResult<String> {
    let date = chrono::DateTime::from_timestamp_millis(now)
        .ok_or_else(|| refusal("The budget period cannot be determined."))?;
    Ok(format!("{:04}-{:02}", date.year(), date.month()))
}
fn hash(request: &Value) -> String {
    hex::encode(Sha256::digest(
        serde_json::to_vec(request).expect("JSON request"),
    ))
}
pub fn breach(spend: &Spend) -> bool {
    spend.quote.as_ref().is_some_and(|quote| {
        spend
            .cost_micros
            .is_some_and(|cost| cost > quote.maximum_micros)
    })
}
pub fn committed(app: &App, now: i64) -> AppResult<u64> {
    let period = month(now)?;
    let mut total = 0_u64;
    for spend in &app.spend {
        if breach(spend) {
            return Err(refusal(
                "Paid AI stopped: a provider charge breached its bound. Inspect Spend.",
            ));
        }
        let cost = spend.cost_micros;
        // Unknown charges never age out, including across month boundaries.
        if cost.is_none()
            || month(spend.created_ms)? == period
            || spend.created_ms > now
            || spend
                .settled_ms
                .is_some_and(|settled| month(settled).is_ok_and(|m| m == period) || settled > now)
        {
            total = total
                .checked_add(cost.unwrap_or(spend.reserved_micros))
                .ok_or_else(|| refusal("Paid AI stopped: spend exceeds the accounting range."))?;
        }
    }
    Ok(total)
}

fn bound(work: &mut Work, policy: &Policy, now: i64) -> AppResult<Quote> {
    policy.validate(now)?;
    if work.kind != "generation" || work.photo_key.is_some() || work.request["model"] != MODEL {
        return Err(refusal(
            "Paid AI is off for this request: no reviewed total-cost bound exists.",
        ));
    }
    let object = work
        .request
        .as_object()
        .ok_or_else(|| refusal("Invalid paid request."))?;
    if object.keys().any(|key| {
        !matches!(
            key.as_str(),
            "model" | "temperature" | "max_tokens" | "response_format" | "messages"
        )
    }) || work.request["max_tokens"] != OUTPUT_TOKENS
        || work.request["response_format"] != json!({"type":"json_object"})
        || work.request["messages"].as_array().is_none_or(|messages| {
            messages.is_empty()
                || messages.iter().any(|message| {
                    message.as_object().is_none_or(|m| m.len() != 2)
                        || !matches!(message["role"].as_str(), Some("system" | "user"))
                        || !message["content"].is_string()
                })
        })
    {
        return Err(refusal(
            "Paid AI is off: this request has an unbounded billable dimension.",
        ));
    }
    // Reserve the model's ENTIRE hard context, not a guessed tokenizer count.
    // max_tokens is mandatory; no tool, plugin, modality, tier or routing extras.
    let maximum_micros = CONTEXT_TOKENS
        .checked_mul(INPUT_PRICE)
        .and_then(|input| {
            OUTPUT_TOKENS
                .checked_mul(OUTPUT_PRICE)
                .and_then(|output| input.checked_add(output))
        })
        .ok_or_else(|| refusal("The request bound exceeds the accounting range."))?;
    if maximum_micros > policy.request_micros {
        return Err(refusal(
            "The bounded request exceeds the configured per-request budget.",
        ));
    }
    work.request["provider"] = json!({"only":["openai"],"allow_fallbacks":false,"require_parameters":true,
        "max_price":{"prompt":INPUT_PRICE,"completion":OUTPUT_PRICE,"request":0,"image":0}});
    work.request["stream"] = json!(false);
    Ok(Quote {
        policy: policy.clone(),
        month: month(now)?,
        request_hash: hash(&work.request),
        maximum_micros,
    })
}

/// Stage the existing domain claim, then admit its exact wire request and ledger
/// together. The runtime MUST persist this result before awaiting or transmitting.
pub fn claim_work(
    app: &mut App,
    now: i64,
    lease: &str,
    config: &WorkConfig,
    policy: AppResult<Policy>,
) -> AppResult<Option<Work>> {
    if app.spend.iter().any(|spend| spend.id == lease) {
        return Err(AppError::conflict());
    }
    let mut next = app.clone();
    let Some(mut work) = engine::claim_work(&mut next, now, lease, config)? else {
        *app = next;
        return Ok(None);
    };
    let admitted = policy.and_then(|policy| {
        let quote = bound(&mut work, &policy, now)?;
        if app.spent(now).checked_add(quote.maximum_micros).is_none_or(|total| total > DAILY_ALLOWANCE) {
            return Err(refusal("The exact request bound exceeds the remaining rolling-day allowance. Your input is saved."));
        }
        if committed(app, now)?.checked_add(quote.maximum_micros).is_none_or(|total| total > policy.monthly_micros) {
            return Err(refusal("The monthly AI budget is held or spent. Your input is saved. Unresolved provider charges remain reserved."));
        }
        Ok(quote)
    });
    match admitted {
        Ok(quote) => {
            let spend = next
                .spend
                .iter_mut()
                .find(|spend| spend.id == lease)
                .ok_or_else(AppError::conflict)?;
            spend.reserved_micros = quote.maximum_micros;
            spend.quote = Some(quote);
            *app = next;
            Ok(Some(work))
        }
        Err(error) => {
            // No send was admitted. Keep the visible failed job, not a fictitious
            // paid obligation. Previously persisted obligations are untouched.
            engine::fail_work(&mut next, &work, &error.message, false, now)?;
            next.spend.retain(|spend| spend.id != lease);
            *app = next;
            Ok(None)
        }
    }
}

/// Recheck after every await and immediately before the first network send.
pub fn authorize_send(app: &App, work: &Work, policy: &Policy, now: i64) -> AppResult<()> {
    policy.validate(now)?;
    engine::authorize_work(app, &work.id, now)?;
    let spend = app
        .spend
        .iter()
        .find(|spend| spend.id == work.lease && spend.work_id == work.id)
        .ok_or_else(AppError::conflict)?;
    let quote = spend.quote.as_ref().ok_or_else(AppError::conflict)?;
    let job = app.jobs.get(&work.id).ok_or_else(AppError::conflict)?;
    let goal = app.goals.get(&job.goal_id).ok_or_else(AppError::conflict)?;
    if app.restored_paused
        || &quote.policy != policy
        || quote.month != month(now)?
        || quote.request_hash != hash(&work.request)
        || spend.cost_micros.is_some()
        || job.status != "sent"
        || job.lease.as_deref() != Some(&work.lease)
        || job
            .started_ms
            .is_none_or(|started| now < started || now - started >= 90_000)
        || goal.archived
        || goal.revision != job.source_revision
        || job.question_id.as_ref().is_some_and(|id| {
            app.questions
                .get(id)
                .is_none_or(|q| q.archived || Some(q.content().version) != job.question_version)
        })
        || app.spent(now) > DAILY_ALLOWANCE
        || committed(app, now)? > policy.monthly_micros
    {
        return Err(refusal(
            "Paid AI was cancelled before send: its lease, source or budget changed.",
        ));
    }
    Ok(())
}

pub fn validate_spend(spend: &Spend) -> bool {
    let quote_valid = spend.quote.as_ref().is_none_or(|quote| {
        quote.policy.validate(spend.created_ms).is_ok()
            && quote.month == month(spend.created_ms).unwrap_or_default()
            && quote.maximum_micros == CONTEXT_TOKENS * INPUT_PRICE + OUTPUT_TOKENS * OUTPUT_PRICE
            && quote.maximum_micros == spend.reserved_micros
            && quote.maximum_micros <= quote.policy.request_micros
            && quote.request_hash.len() == 64
            && quote.request_hash.bytes().all(|b| b.is_ascii_hexdigit())
            && spend.model.as_deref() == Some(MODEL)
            && spend.purpose.as_deref() == Some("generate.concepts")
    });
    quote_valid
        && spend
            .settled_ms
            .is_none_or(|settled| spend.cost_micros.is_some() && settled >= spend.created_ms)
}
