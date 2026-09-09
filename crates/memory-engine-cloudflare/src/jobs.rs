//! Durable admission, leases, accounting and publication. No in-memory worker
//! readiness flag or detached task is authoritative: alarms re-read this ledger.
use futures_util::future::join_all;
use memory_engine_api_state::{GenerationJob, JobStatus, StudyViewResponse};
use memory_engine_generation::{
    enforce_content_policy, generation_repair_rejections, run_beta_generation,
    run_beta_generation_with_provider, BetaGenerationRequest, BetaGenerationStore, ProviderFailure,
    ProviderUsage, SourceAuthorizationContext,
};
use memory_engine_persistence::{GenerationRunUsage, SourceDocument};
use memory_engine_study::BetaStudySession;
use serde::Deserialize;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use worker::Env;

use crate::{
    database::Database, now_ms, provider, random_id, store::SqlStudyStore, AppResult, Failure,
};

const MAX_QUEUE_ACCOUNT: i64 = 8;
const MAX_QUEUE_GLOBAL: i64 = 64;
const MAX_RUNNING: i64 = 4;
const MAX_ATTEMPTS: u32 = 3;
const LEASE_MS: i64 = 180_000;
const BUDGET_WINDOW_MS: i64 = 86_400_000;
// Admission bounds are conservative accounted spend, not a model price quote.
// Reported charges always replace the allowance, even when larger; missing
// usage, interrupted Fetch and eviction retain the full per-call allowance.
const REQUEST_RESERVATION: i64 = 100_000;
const GENERATION_RESERVATION: i64 = 2 * REQUEST_RESERVATION;
const ACCOUNT_BUDGET: i64 = 1_000_000;
const GLOBAL_BUDGET: i64 = 10_000_000;
const BUDGET_RETRY_MS: i64 = 60_000;

#[derive(Clone, Deserialize)]
struct JobRow {
    account_id: String,
    job_id: String,
    source_id: String,
    title: String,
    status: JobStatus,
    card_count: usize,
    attempts: u32,
    error: Option<String>,
    model_key: String,
    created_at_ms: i64,
    updated_at_ms: i64,
    retry_at_ms: Option<i64>,
    lease_expires_at_ms: Option<i64>,
    lease_token: Option<String>,
    reserved_cost_usd_micros: i64,
    source_fingerprint: Option<String>,
    retryable: u8,
}

impl JobRow {
    fn wire(self) -> GenerationJob {
        GenerationJob {
            id: self.job_id,
            account_id: self.account_id,
            source_id: self.source_id,
            title: self.title,
            status: self.status,
            card_count: self.card_count,
            attempts: self.attempts,
            retryable: self.retryable != 0
                && self.attempts < MAX_ATTEMPTS
                && self.status != JobStatus::Succeeded,
            error: self.error,
            created_at: self.created_at_ms,
            updated_at: self.updated_at_ms,
            retry_at: self.retry_at_ms,
            lease_expires_at: self.lease_expires_at_ms,
        }
    }
}

struct Claim {
    row: JobRow,
    source: SourceDocument,
    fingerprint: String,
    token: String,
    run_id: String,
    local: bool,
}

#[derive(Deserialize)]
struct Amount {
    amount: i64,
}

fn amount(db: &Database, sql: &str, parameters: &[Value]) -> AppResult<i64> {
    db.query::<Amount>(sql, parameters)?
        .into_iter()
        .next()
        .map(|row| row.amount)
        .ok_or_else(|| Failure::internal("The generation ledger returned no aggregate."))
}

fn job_row(db: &Database, account_id: &str, job_id: &str) -> AppResult<JobRow> {
    db.query(
        "SELECT * FROM memory_engine_generation_jobs WHERE account_id = ? AND job_id = ?",
        &[json!(account_id), json!(job_id)],
    )?
    .into_iter()
    .next()
    .ok_or_else(|| Failure::not_found("Generation job not found."))
}

pub fn get(db: &Database, account_id: &str, job_id: &str) -> AppResult<GenerationJob> {
    Ok(job_row(db, account_id, job_id)?.wire())
}

pub fn list(db: &Database, account_id: &str) -> AppResult<Vec<GenerationJob>> {
    // Retain the durable ledger for recovery/accounting; bound only the learner
    // history projection. Never drop an in-flight job from this projection.
    let rows: Vec<JobRow> = db.query(
        "SELECT * FROM memory_engine_generation_jobs WHERE account_id = ? AND
         (status IN ('queued','running','retry') OR job_id IN (
           SELECT job_id FROM memory_engine_generation_jobs WHERE account_id = ?
           AND status IN ('succeeded','failed') ORDER BY created_at_ms DESC, job_id DESC LIMIT 50))
         ORDER BY created_at_ms DESC, job_id DESC",
        &[json!(account_id), json!(account_id)],
    )?;
    Ok(rows.into_iter().map(JobRow::wire).collect())
}

fn source(db: &Database, account_id: &str, source_id: &str) -> AppResult<SourceDocument> {
    let source = SqlStudyStore::new(db.clone(), account_id)
        .source_document(source_id)?
        .ok_or_else(|| Failure::not_found("Source document not found."))?;
    if source.archived_at.is_some() {
        return Err(Failure::conflict(
            "Archived sources cannot generate study material.",
        ));
    }
    if source
        .body
        .as_deref()
        .is_none_or(|body| body.trim().is_empty())
    {
        return Err(Failure::bad_request("The source has no text to study."));
    }
    if source
        .title
        .len()
        .saturating_add(source.body.as_deref().map_or(0, str::len))
        > memory_engine_openrouter::content::MAX_SOURCE_BYTES
    {
        return Err(Failure::new(
            413,
            "The source exceeds 64 KiB. Split it into smaller captures.",
        ));
    }
    Ok(source)
}

fn fingerprint(db: &Database, account_id: &str, source: &SourceDocument) -> AppResult<String> {
    let revision = amount(db, "SELECT revision AS amount FROM memory_engine_source_revisions WHERE account_id = ? AND source_id = ?", &[json!(account_id), json!(source.id)])?;
    let mut hash = Sha256::new();
    hash.update(revision.to_be_bytes());
    hash.update(serde_json::to_vec(source)?);
    Ok(crate::auth::encode_hex(&hash.finalize()))
}

pub(crate) fn authorization_fingerprint(
    db: &Database,
    account_id: &str,
    authorization: &SourceAuthorizationContext,
) -> AppResult<String> {
    if authorization.sources().is_empty() {
        return Err(Failure::forbidden(
            "This item has no source context authorized for model generation.",
        ));
    }
    let mut hash = Sha256::new();
    for authorized in authorization.sources() {
        let current = source(db, account_id, authorized.id())?;
        let current_authorization =
            SourceAuthorizationContext::from_sources(std::slice::from_ref(&current))
                .map_err(|_| Failure::forbidden("The source authorization changed."))?;
        if current_authorization.sources().first() != Some(authorized) {
            return Err(Failure::conflict(
                "The source authorization changed while material was being prepared.",
            ));
        }
        hash.update(fingerprint(db, account_id, &current)?.as_bytes());
    }
    Ok(crate::auth::encode_hex(&hash.finalize()))
}

fn queued_count(db: &Database, account_id: Option<&str>) -> AppResult<i64> {
    amount(db,
        "SELECT (SELECT COUNT(*) FROM memory_engine_generation_jobs WHERE status IN ('queued','running','retry') AND (? IS NULL OR account_id = ?))
         + (SELECT COUNT(*) FROM memory_engine_model_operations WHERE status = 'running' AND (? IS NULL OR account_id = ?)) AS amount",
        &[json!(account_id), json!(account_id), json!(account_id), json!(account_id)])
}

fn running_count(db: &Database, account_id: Option<&str>) -> AppResult<i64> {
    amount(db,
        "SELECT (SELECT COUNT(*) FROM memory_engine_generation_jobs WHERE status = 'running' AND (? IS NULL OR account_id = ?))
         + (SELECT COUNT(*) FROM memory_engine_model_operations WHERE status = 'running' AND (? IS NULL OR account_id = ?)) AS amount",
        &[json!(account_id), json!(account_id), json!(account_id), json!(account_id)])
}

fn budget_used(db: &Database, account_id: Option<&str>) -> AppResult<i64> {
    amount(db,
        "WITH charges AS (
           SELECT account_id, cost_usd_micros + reserved_cost_usd_micros AS charge
           FROM memory_engine_generation_job_attempts WHERE started_at_ms >= ? OR status = 'running'
           UNION ALL
           SELECT job.account_id, CASE WHEN EXISTS (SELECT 1 FROM memory_engine_generation_job_attempts attempt
                  WHERE attempt.account_id = job.account_id AND attempt.job_id = job.job_id)
             THEN job.legacy_cost_usd_micros + CASE WHEN job.status IN ('queued','retry') THEN job.reserved_cost_usd_micros ELSE 0 END
             ELSE job.cost_usd_micros + CASE WHEN job.status IN ('queued','running','retry') THEN job.reserved_cost_usd_micros ELSE 0 END END
           FROM memory_engine_generation_jobs job WHERE job.updated_at_ms >= ? OR job.status IN ('queued','running','retry')
           UNION ALL
           SELECT account_id, cost_usd_micros + reserved_cost_usd_micros FROM memory_engine_model_operations
           WHERE started_at_ms >= ? OR status = 'running')
         SELECT COALESCE(SUM(charge), 0) AS amount FROM charges WHERE ? IS NULL OR account_id = ?",
        &[json!(now_ms() - BUDGET_WINDOW_MS), json!(now_ms() - BUDGET_WINDOW_MS), json!(now_ms() - BUDGET_WINDOW_MS), json!(account_id), json!(account_id)])
}

fn admit_budget(db: &Database, account_id: &str, additional: i64) -> AppResult<()> {
    if additional <= 0 {
        return Ok(());
    }
    if budget_used(db, Some(account_id))?.saturating_add(additional) > ACCOUNT_BUDGET {
        return Err(Failure::new(
            429,
            "This account's 24-hour generation allowance is reserved or spent. Try again later.",
        ));
    }
    if budget_used(db, None)?.saturating_add(additional) > GLOBAL_BUDGET {
        return Err(Failure::new(
            429,
            "The service's 24-hour generation allowance is reserved or spent. Try again later.",
        ));
    }
    Ok(())
}

fn admit_queue(db: &Database, account_id: &str) -> AppResult<()> {
    if queued_count(db, Some(account_id))? >= MAX_QUEUE_ACCOUNT {
        return Err(Failure::new(
            429,
            "This account already has eight generation operations waiting or running.",
        ));
    }
    if queued_count(db, None)? >= MAX_QUEUE_GLOBAL {
        return Err(Failure::new(
            429,
            "The generation queue is full. Your saved source is safe; try again later.",
        ));
    }
    Ok(())
}

pub fn enqueue(
    db: &Database,
    env: &Env,
    account_id: &str,
    source_id: &str,
) -> AppResult<(GenerationJob, bool)> {
    db.transaction(|| {
    expire_stale(db)?;
    let existing: Vec<JobRow> = db.query(
        "SELECT * FROM memory_engine_generation_jobs WHERE account_id = ? AND source_id = ? AND status IN ('queued','running','retry') LIMIT 1",
        &[json!(account_id), json!(source_id)])?;
    if let Some(job) = existing.into_iter().next() { return Ok((job.wire(), true)); }
    let source = source(db, account_id, source_id)?;
    let local = provider::local_generation(&source);
    let model = if local { provider::LOCAL_MODEL.to_owned() } else { provider::model_name(env)? };
    let reservation = if local { 0 } else { GENERATION_RESERVATION };
    admit_queue(db, account_id)?;
    admit_budget(db, account_id, reservation)?;
    let id = random_id("job-")?;
    let fingerprint = fingerprint(db, account_id, &source)?;
    let now = now_ms();
    db.execute(
        "INSERT INTO memory_engine_generation_jobs (account_id, job_id, source_id, title, status, model_key, created_at_ms, updated_at_ms, reserved_cost_usd_micros, source_fingerprint)
         VALUES (?, ?, ?, ?, 'queued', ?, ?, ?, ?, ?)",
        &[json!(account_id), json!(id), json!(source_id), json!(source.title), json!(model), json!(now), json!(now), json!(reservation), json!(fingerprint)])?;
    Ok((job_row(db, account_id, &id)?.wire(), false))
})
}

pub fn retry(
    db: &Database,
    _env: &Env,
    account_id: &str,
    job_id: &str,
) -> AppResult<GenerationJob> {
    db.transaction(|| {
    expire_stale(db)?;
    let job = job_row(db, account_id, job_id)?;
    if job.status != JobStatus::Failed || job.retryable == 0 || job.attempts >= MAX_ATTEMPTS {
        return Err(Failure::conflict("This job cannot be retried. Create a new generation if you changed the source."));
    }
    let source = source(db, account_id, &job.source_id)?;
    let fingerprint = fingerprint(db, account_id, &source)?;
    if job.source_fingerprint.as_ref().is_some_and(|old| *old != fingerprint) {
        return Err(Failure::conflict("The source changed. Start a new generation for its current content and permission."));
    }
    if amount(db, "SELECT COUNT(*) AS amount FROM memory_engine_generation_jobs WHERE account_id = ? AND source_id = ? AND status IN ('queued','running','retry')", &[json!(account_id), json!(job.source_id)])? > 0 {
        return Err(Failure::conflict("A generation for this source is already waiting or running."));
    }
    admit_queue(db, account_id)?;
    let reservation = if provider::local_generation(&source) { 0 } else { GENERATION_RESERVATION };
    admit_budget(db, account_id, reservation)?;
    db.execute("UPDATE memory_engine_generation_jobs SET status = 'queued', error = NULL, retry_at_ms = NULL, updated_at_ms = ?, reserved_cost_usd_micros = ?, source_fingerprint = ? WHERE account_id = ? AND job_id = ?",
        &[json!(now_ms()), json!(reservation), json!(fingerprint), json!(account_id), json!(job_id)])?;
    Ok(job_row(db, account_id, job_id)?.wire())
})
}

fn fail_unclaimed(db: &Database, job: &JobRow, message: &str) -> AppResult<()> {
    db.execute("UPDATE memory_engine_generation_jobs SET status = 'failed', retryable = 0, error = ?, retry_at_ms = NULL, reserved_cost_usd_micros = 0, updated_at_ms = ? WHERE account_id = ? AND job_id = ? AND status IN ('queued','retry')",
        &[json!(message), json!(now_ms()), json!(job.account_id), json!(job.job_id)])
}

fn expire_stale(db: &Database) -> AppResult<()> {
    #[derive(Deserialize)]
    struct PendingRun {
        account_id: String,
        job_id: String,
        attempt: u32,
        generation_run_id: String,
        run: String,
    }
    let now = now_ms();
    // Older native workers persisted pending drafts before terminal publication.
    // Preserve their partial usage receipt, then remove only unpublished output
    // before replay; otherwise it would poison the shared duplicate gate.
    let pending: Vec<PendingRun> = db.query(
        "SELECT attempt.account_id, attempt.job_id, attempt.attempt, attempt.generation_run_id, run.run
         FROM memory_engine_generation_job_attempts attempt
         JOIN memory_engine_generation_jobs job ON job.account_id = attempt.account_id AND job.job_id = attempt.job_id
           AND job.lease_token = attempt.lease_token
         JOIN memory_engine_generation_runs run ON run.account_id = attempt.account_id AND run.generation_run_id = attempt.generation_run_id
         WHERE job.status = 'running' AND attempt.status = 'running' AND (job.lease_expires_at_ms IS NULL OR job.lease_expires_at_ms <= ?)
           AND (json_extract(run.run, '$.completedAt') IS NULL OR json_extract(run.run, '$.completedAt') = -9223372036854775808)",
        &[json!(now)])?;
    for pending in pending {
        let run: memory_engine_persistence::GenerationRun = serde_json::from_str(&pending.run)?;
        if let Some(usage) = run.usage {
            db.execute("UPDATE memory_engine_generation_job_attempts SET usage_json = ?, cost_usd_micros = MAX(cost_usd_micros, ?) WHERE account_id = ? AND job_id = ? AND attempt = ? AND usage_json IS NULL",
                &[json!(serde_json::to_string(&usage)?), json!(usage.cost_usd_micros.unwrap_or(0).max(0)), json!(pending.account_id), json!(pending.job_id), json!(pending.attempt)])?;
        }
        SqlStudyStore::new(db.clone(), &pending.account_id)
            .discard_generation_run(&pending.generation_run_id)?;
    }
    // Preserve already-accounted calls, conservatively charge only the still
    // reserved calls, and never re-label uncertainty as a provider price.
    db.execute(
        "UPDATE memory_engine_generation_job_attempts SET status = 'stale', cost_usd_micros = cost_usd_micros + reserved_cost_usd_micros,
         cost_unknown = CASE WHEN reserved_cost_usd_micros > 0 THEN 1 ELSE cost_unknown END, reserved_cost_usd_micros = 0,
         error = 'The generation lease expired before publication.', completed_at_ms = ?, updated_at_ms = ?
         WHERE status = 'running' AND EXISTS (SELECT 1 FROM memory_engine_generation_jobs job
           WHERE job.account_id = memory_engine_generation_job_attempts.account_id AND job.job_id = memory_engine_generation_job_attempts.job_id
           AND job.status = 'running' AND job.lease_token = memory_engine_generation_job_attempts.lease_token
           AND (job.lease_expires_at_ms IS NULL OR job.lease_expires_at_ms <= ?))",
        &[json!(now), json!(now), json!(now)])?;
    db.execute(
        "UPDATE memory_engine_generation_jobs SET status = CASE WHEN attempts < ? THEN 'retry' ELSE 'failed' END,
         retryable = CASE WHEN attempts < ? THEN 1 ELSE 0 END, error = 'The generation lease expired; durable recovery will retry when permitted.',
         retry_at_ms = CASE WHEN attempts < ? THEN ? ELSE NULL END, lease_owner = NULL, lease_token = NULL, lease_expires_at_ms = NULL,
         cost_usd_micros = CASE WHEN NOT EXISTS (SELECT 1 FROM memory_engine_generation_job_attempts attempt
           WHERE attempt.account_id = memory_engine_generation_jobs.account_id AND attempt.job_id = memory_engine_generation_jobs.job_id)
           THEN cost_usd_micros + reserved_cost_usd_micros ELSE cost_usd_micros END,
         reserved_cost_usd_micros = 0, updated_at_ms = ?
         WHERE status = 'running' AND (lease_expires_at_ms IS NULL OR lease_expires_at_ms <= ?)",
        &[json!(MAX_ATTEMPTS), json!(MAX_ATTEMPTS), json!(MAX_ATTEMPTS), json!(now), json!(now), json!(now)])?;
    db.execute(
        "UPDATE memory_engine_model_operations SET status = 'stale', cost_usd_micros = cost_usd_micros + reserved_cost_usd_micros,
         cost_unknown = CASE WHEN reserved_cost_usd_micros > 0 THEN 1 ELSE cost_unknown END, reserved_cost_usd_micros = 0,
         error = 'The model operation expired before publication.', completed_at_ms = ? WHERE status = 'running' AND lease_expires_at_ms <= ?",
        &[json!(now), json!(now)])
}

fn claim(db: &Database, env: &Env, target: Option<(&str, &str)>) -> AppResult<Option<Claim>> {
    db.transaction(|| {
        expire_stale(db)?;
        if running_count(db, None)? >= MAX_RUNNING { return Ok(None); }
        let rows: Vec<JobRow> = db.query(
            "SELECT * FROM memory_engine_generation_jobs WHERE (status = 'queued' OR (status = 'retry' AND COALESCE(retry_at_ms, 0) <= ?))
             AND (? IS NULL OR (account_id = ? AND job_id = ?)) ORDER BY created_at_ms, account_id, job_id LIMIT 64",
            &[json!(now_ms()), json!(target.map(|value| value.0)), json!(target.map(|value| value.0)), json!(target.map(|value| value.1))])?;
        for mut row in rows {
            if running_count(db, Some(&row.account_id))? != 0 { continue; }
            if row.attempts >= MAX_ATTEMPTS { fail_unclaimed(db, &row, "The maximum generation attempts were exhausted.")?; continue; }
            let source = match source(db, &row.account_id, &row.source_id) {
                Ok(source) => source,
                Err(failure) if failure.status < 500 => { fail_unclaimed(db, &row, &failure.message)?; continue; }
                Err(failure) => return Err(failure),
            };
            let fingerprint = fingerprint(db, &row.account_id, &source)?;
            if row.source_fingerprint.as_ref().is_some_and(|old| *old != fingerprint) {
                fail_unclaimed(db, &row, "The source changed or its permission was revoked. Create a new generation for the current source.")?;
                continue;
            }
            let local = provider::local_generation(&source);
            let reservation = if local { 0 } else { GENERATION_RESERVATION };
            // A schema8 job may have used 'deterministic' as an incidental
            // missing-env default. Resolve it to the real configured model if
            // its prose requires Fetch, never to the fake fixture provider.
            if !local && row.model_key == provider::LOCAL_MODEL { row.model_key = provider::model_name(env)?; }
            if let Err(failure) = admit_budget(db, &row.account_id, reservation.saturating_sub(row.reserved_cost_usd_micros)) {
                if failure.status != 429 { return Err(failure); }
                db.execute("UPDATE memory_engine_generation_jobs SET status = 'retry', retry_at_ms = ?, error = ?, updated_at_ms = ? WHERE account_id = ? AND job_id = ?",
                    &[json!(now_ms() + BUDGET_RETRY_MS), json!(failure.message), json!(now_ms()), json!(row.account_id), json!(row.job_id)])?;
                continue;
            }
            let token = random_id("lease-")?;
            let now = now_ms();
            row.attempts += 1;
            let run_id = format!("job:{}:attempt:{}", row.job_id, row.attempts);
            // Preserve charged legacy jobs that predate per-attempt receipts.
            // Once this claim creates an attempt, aggregate accounting must not
            // silently stop counting that pre-ledger spend.
            db.execute("UPDATE memory_engine_generation_jobs SET legacy_cost_usd_micros = MAX(legacy_cost_usd_micros, cost_usd_micros) WHERE account_id = ? AND job_id = ? AND NOT EXISTS (SELECT 1 FROM memory_engine_generation_job_attempts attempt WHERE attempt.account_id = memory_engine_generation_jobs.account_id AND attempt.job_id = memory_engine_generation_jobs.job_id)",
                &[json!(row.account_id), json!(row.job_id)])?;
            db.execute("UPDATE memory_engine_generation_jobs SET status = 'running', attempts = ?, error = NULL, retry_at_ms = NULL, lease_owner = 'cloudflare-alarm', lease_token = ?, lease_expires_at_ms = ?, updated_at_ms = ?, reserved_cost_usd_micros = 0, source_fingerprint = ?, model_key = ? WHERE account_id = ? AND job_id = ?",
                &[json!(row.attempts), json!(token), json!(now + LEASE_MS), json!(now), json!(fingerprint), json!(row.model_key), json!(row.account_id), json!(row.job_id)])?;
            db.execute("INSERT INTO memory_engine_generation_job_attempts (account_id, job_id, attempt, lease_token, status, generation_run_id, reservation_cost_usd_micros, reserved_cost_usd_micros, started_at_ms, updated_at_ms) VALUES (?, ?, ?, ?, 'running', ?, ?, ?, ?, ?)",
                &[json!(row.account_id), json!(row.job_id), json!(row.attempts), json!(token), json!(run_id), json!(reservation), json!(reservation), json!(now), json!(now)])?;
            row.status = JobStatus::Running;
            row.updated_at_ms = now;
            row.lease_expires_at_ms = Some(now + LEASE_MS);
            row.lease_token = Some(token.clone());
            row.reserved_cost_usd_micros = reservation;
            return Ok(Some(Claim { row, source, fingerprint, token, run_id, local }));
        }
        Ok(None)
    })
}

fn check_claim(db: &Database, claim: &Claim) -> AppResult<()> {
    let current = job_row(db, &claim.row.account_id, &claim.row.job_id)?;
    if current.status != JobStatus::Running
        || current.attempts != claim.row.attempts
        || current.lease_token.as_deref() != Some(claim.token.as_str())
        || current
            .lease_expires_at_ms
            .is_none_or(|expiry| expiry <= now_ms())
    {
        return Err(Failure::conflict(
            "The generation lease is no longer owned by this attempt.",
        ));
    }
    let source = source(db, &claim.row.account_id, &claim.row.source_id)?;
    if fingerprint(db, &claim.row.account_id, &source)? != claim.fingerprint {
        return Err(Failure::conflict("The source changed or its permission was revoked before generation could be published."));
    }
    Ok(())
}

#[derive(Default)]
struct UsageSummary {
    usage: Option<GenerationRunUsage>,
    accounted: i64,
    uncertain: bool,
    calls: i64,
}

impl UsageSummary {
    fn add(&mut self, usage: Option<&ProviderUsage>) {
        let Some(usage) = usage else {
            return;
        };
        self.calls += 1;
        self.accounted = self.accounted.saturating_add(
            usage
                .cost_usd_micros
                .filter(|cost| *cost >= 0)
                .unwrap_or(REQUEST_RESERVATION),
        );
        self.uncertain |= usage.cost_usd_micros.is_none_or(|cost| cost < 0);
        let total = self.usage.get_or_insert(GenerationRunUsage {
            input_tokens: 0,
            output_tokens: 0,
            cost_usd_micros: Some(0),
            latency_ms: 0,
        });
        total.input_tokens = total.input_tokens.saturating_add(usage.input_tokens);
        total.output_tokens = total.output_tokens.saturating_add(usage.output_tokens);
        total.latency_ms = total.latency_ms.saturating_add(usage.latency_ms);
        total.cost_usd_micros = if self.uncertain {
            None
        } else {
            Some(self.accounted)
        };
    }
}

fn account_attempt(
    db: &Database,
    claim: &Claim,
    usage: &UsageSummary,
    complete: bool,
) -> AppResult<()> {
    let remaining = if complete || claim.local {
        0
    } else {
        GENERATION_RESERVATION.saturating_sub(usage.calls * REQUEST_RESERVATION)
    };
    db.execute("UPDATE memory_engine_generation_job_attempts SET cost_usd_micros = ?, reserved_cost_usd_micros = ?, cost_unknown = ?, usage_json = ?, updated_at_ms = ? WHERE account_id = ? AND job_id = ? AND attempt = ? AND lease_token = ? AND status = 'running'",
        &[json!(usage.accounted), json!(remaining), json!(u8::from(usage.uncertain)), json!(usage.usage.as_ref().map(serde_json::to_string).transpose()?), json!(now_ms()), json!(claim.row.account_id), json!(claim.row.job_id), json!(claim.row.attempts), json!(claim.token)])
}

fn finish_claim(
    db: &Database,
    claim: &Claim,
    succeeded: bool,
    retryable: bool,
    message: Option<&str>,
) -> AppResult<()> {
    // All callers publish under a synchronous transaction. The token/attempt
    // predicates also make late accounting/finalization harmless after reclaim.
    let may_retry = !succeeded && retryable && claim.row.attempts < MAX_ATTEMPTS;
    let status = if succeeded {
        "succeeded"
    } else if may_retry {
        "retry"
    } else {
        "failed"
    };
    let retry_at = may_retry.then(|| now_ms() + 1_000 * i64::from(claim.row.attempts));
    db.execute("UPDATE memory_engine_generation_jobs SET status = ?, retryable = ?, error = ?, retry_at_ms = ?, card_count = 0, lease_owner = NULL, lease_token = NULL, lease_expires_at_ms = NULL, reserved_cost_usd_micros = 0, updated_at_ms = ?, cost_usd_micros = legacy_cost_usd_micros + (SELECT COALESCE(SUM(cost_usd_micros), 0) FROM memory_engine_generation_job_attempts WHERE account_id = ? AND job_id = ?) WHERE account_id = ? AND job_id = ? AND attempts = ? AND lease_token = ? AND status = 'running'",
        &[json!(status), json!(u8::from(retryable && claim.row.attempts < MAX_ATTEMPTS && !succeeded)), json!(message), json!(retry_at), json!(now_ms()), json!(claim.row.account_id), json!(claim.row.job_id), json!(claim.row.account_id), json!(claim.row.job_id), json!(claim.row.attempts), json!(claim.token)])?;
    db.execute("UPDATE memory_engine_generation_job_attempts SET status = ?, reserved_cost_usd_micros = 0, error = ?, completed_at_ms = ?, updated_at_ms = ? WHERE account_id = ? AND job_id = ? AND attempt = ? AND lease_token = ? AND status = 'running'",
        &[json!(if succeeded { "succeeded" } else { "failed" }), json!(message), json!(now_ms()), json!(now_ms()), json!(claim.row.account_id), json!(claim.row.job_id), json!(claim.row.attempts), json!(claim.token)])
}

async fn run_claimed(db: &Database, env: &Env, claim: Claim) -> AppResult<()> {
    let mut usage = UsageSummary::default();
    let mut transient = false;
    let mut provider_message = None;
    let prepared = if claim.local {
        None
    } else {
        if let Err(failure) = check_claim(db, &claim) {
            if failure.status >= 500 {
                return Err(failure);
            }
            account_attempt(db, &claim, &usage, true)?;
            db.transaction(|| finish_claim(db, &claim, false, false, Some(&failure.message)))?;
            return Ok(());
        }
        let first = provider::drafts(env, &claim.source, &claim.row.model_key)
            .await
            .map(|drafts| enforce_content_policy(&claim.source, drafts));
        match &first {
            Ok(drafts) => usage.add(drafts.usage.as_ref()),
            Err(failure) => {
                usage.add(failure.usage());
                transient = failure.is_transient();
                provider_message = Some(failure.to_string());
            }
        }
        account_attempt(db, &claim, &usage, false)?;
        let repair = if let Ok(first) = &first {
            match check_claim(db, &claim) {
                Ok(()) => {
                    let snapshot = BetaGenerationStore::snapshot(&SqlStudyStore::new(
                        db.clone(),
                        &claim.row.account_id,
                    ))?;
                    let rejections = generation_repair_rejections(&snapshot, &claim.source, first);
                    if rejections.is_empty() {
                        Ok(None)
                    } else {
                        provider::repair(env, &claim.source, &claim.row.model_key, &rejections)
                            .await
                    }
                }
                Err(failure) => {
                    if failure.status >= 500 {
                        return Err(failure);
                    }
                    account_attempt(db, &claim, &usage, true)?;
                    db.transaction(|| {
                        finish_claim(db, &claim, false, false, Some(&failure.message))
                    })?;
                    return Ok(());
                }
            }
        } else {
            Ok(None)
        };
        match &repair {
            Ok(Some(drafts)) => usage.add(drafts.usage.as_ref()),
            Err(failure) => {
                usage.add(failure.usage());
                transient |= failure.is_transient();
                provider_message = Some(failure.to_string());
            }
            Ok(None) => {}
        }
        Some(provider::PreparedDrafts::new(
            claim.source.clone(),
            provider::model_identity(&claim.row.model_key),
            first,
            repair,
        ))
    };
    account_attempt(db, &claim, &usage, true)?;
    let publication = publish_claim(
        db,
        &claim,
        prepared.as_ref(),
        &usage,
        transient,
        provider_message.as_deref(),
    );
    if let Err(failure) = publication {
        if failure.status >= 500 {
            return Err(failure);
        }
        db.transaction(|| finish_claim(db, &claim, false, false, Some(&failure.message)))?;
    }
    Ok(())
}

fn publish_claim(
    db: &Database,
    claim: &Claim,
    prepared: Option<&provider::PreparedDrafts>,
    usage: &UsageSummary,
    transient: bool,
    provider_message: Option<&str>,
) -> AppResult<()> {
    db.transaction(|| {
        check_claim(db, claim)?;
        let mut store = SqlStudyStore::new(db.clone(), &claim.row.account_id);
        let request = BetaGenerationRequest {
            run_id: claim.run_id.clone(), source_document_ids: vec![claim.row.source_id.clone()],
            parent_review_unit_id: None, started_at: claim.row.updated_at_ms, completed_at: Some(now_ms()),
            default_due: now_ms(), model: None, pending: true,
        };
        let result = match prepared {
            Some(provider) => run_beta_generation_with_provider(&mut store, provider, request),
            None => run_beta_generation(&mut store, request),
        }.map_err(provider::generation_failure)?;
        // A repair already paid for may be unnecessary after another learner
        // action changes duplicate state. Its charge still belongs to the run.
        if let Some(usage) = &usage.usage {
            db.execute("UPDATE memory_engine_generation_runs SET run = json_set(run, '$.usage', json(?)) WHERE account_id = ? AND generation_run_id = ?",
                &[json!(serde_json::to_string(usage)?), json!(claim.row.account_id), json!(claim.run_id)])?;
        }
        let succeeded = !result.accepted_draft_ids.is_empty();
        check_claim(db, claim)?;
        if succeeded {
            if !store.finalize_generation_run(&claim.run_id, i32::try_from(claim.row.attempts)
                .map_err(|_| Failure::internal("Invalid generation attempt"))?, &claim.token, now_ms(), true)?
            {
                return Err(Failure::conflict("Generation publication lost its lease"));
            }
            return Ok(());
        }
        let error = if succeeded { None } else { Some(provider_message.unwrap_or("No drafts passed the quality checks. Review the generation notices and revise this source before generating again.")) };
        finish_claim(db, claim, succeeded, transient, error)
    })
}

pub async fn run_due(db: &Database, env: &Env) -> AppResult<()> {
    let mut claims = Vec::new();
    for _ in 0..MAX_RUNNING {
        let Some(claim) = claim(db, env, None)? else {
            break;
        };
        claims.push(claim);
    }
    let outcomes = join_all(claims.into_iter().map(|claim| run_claimed(db, env, claim))).await;
    // Drain every claimed future before returning an error, so one account's
    // persistence failure cannot cancel another account's already-paid request.
    for outcome in outcomes {
        outcome?;
    }
    Ok(())
}

pub async fn generate_now(
    db: &Database,
    env: &Env,
    account_id: &str,
    source_id: &str,
) -> AppResult<StudyViewResponse> {
    let (job, _) = enqueue(db, env, account_id, source_id)?;
    if let Some(claim) = claim(db, env, Some((account_id, &job.id)))? {
        run_claimed(db, env, claim).await?;
    } else {
        let current = get(db, account_id, &job.id)?;
        if !current.status.is_terminal() {
            return Err(Failure::conflict("Generation is already running or waiting for capacity. Its durable job can be followed in Activity."));
        }
    }
    let session = BetaStudySession::from_store(SqlStudyStore::new(db.clone(), account_id), now_ms);
    let mut view = StudyViewResponse::from_view(session.view().map_err(provider::study_failure)?);
    if let Some(error) = get(db, account_id, &job.id)?.error {
        if !view.generation_notices.contains(&error) {
            view.generation_notices.push(error);
        }
    }
    Ok(view)
}

pub fn next_run_at(db: &Database) -> AppResult<Option<i64>> {
    #[derive(Deserialize)]
    struct Next {
        next_at: Option<i64>,
    }
    let ready = running_count(db, None)? < MAX_RUNNING;
    let rows: Vec<Next> = db.query(
        "SELECT MIN(next_at) AS next_at FROM (
           SELECT CASE WHEN status = 'running' THEN COALESCE(lease_expires_at_ms, ?)
             WHEN status = 'retry' THEN COALESCE(retry_at_ms, ?) ELSE ? END AS next_at
           FROM memory_engine_generation_jobs job WHERE status = 'running' OR
             (status IN ('queued','retry') AND ? = 1 AND NOT EXISTS (
                SELECT 1 FROM memory_engine_generation_jobs running WHERE running.account_id = job.account_id AND running.status = 'running')
              AND NOT EXISTS (SELECT 1 FROM memory_engine_model_operations operation WHERE operation.account_id = job.account_id AND operation.status = 'running'))
           UNION ALL SELECT lease_expires_at_ms AS next_at FROM memory_engine_model_operations WHERE status = 'running')",
        &[json!(now_ms()), json!(now_ms()), json!(now_ms()), json!(u8::from(ready))])?;
    Ok(rows.into_iter().next().and_then(|row| row.next_at))
}

pub(crate) fn reserve_operation(
    db: &Database,
    account_id: &str,
    kind: &str,
    review_unit_id: &str,
    model: &str,
) -> AppResult<String> {
    db.transaction(|| {
        expire_stale(db)?;
        admit_queue(db, account_id)?;
        if running_count(db, Some(account_id))? > 0 || running_count(db, None)? >= MAX_RUNNING {
            return Err(Failure::new(429, "Another generation is running. Please try this study action again when it finishes."));
        }
        admit_budget(db, account_id, REQUEST_RESERVATION)?;
        let id = random_id("model-")?;
        let now = now_ms();
        db.execute("INSERT INTO memory_engine_model_operations (account_id, operation_id, kind, review_unit_id, model_key, status, reservation_cost_usd_micros, reserved_cost_usd_micros, started_at_ms, lease_expires_at_ms) VALUES (?, ?, ?, ?, ?, 'running', ?, ?, ?, ?)",
            &[json!(account_id), json!(id), json!(kind), json!(review_unit_id), json!(model), json!(REQUEST_RESERVATION), json!(REQUEST_RESERVATION), json!(now), json!(now + LEASE_MS)])?;
        Ok(id)
    })
}

pub(crate) fn account_operation(
    db: &Database,
    account_id: &str,
    operation: &str,
    usage: Option<&ProviderUsage>,
    failure: Option<&ProviderFailure>,
) -> AppResult<()> {
    // No usage means no Fetch was dispatched (configuration/prepare rejection).
    // Every dispatched request has a usage receipt, including transport errors.
    let cost = usage.map_or(0, |usage| {
        usage
            .cost_usd_micros
            .filter(|cost| *cost >= 0)
            .unwrap_or(REQUEST_RESERVATION)
    });
    let unknown = usage.is_some_and(|usage| usage.cost_usd_micros.is_none_or(|cost| cost < 0));
    let receipt = usage.map(|usage| GenerationRunUsage {
        input_tokens: usage.input_tokens,
        output_tokens: usage.output_tokens,
        cost_usd_micros: usage.cost_usd_micros,
        latency_ms: usage.latency_ms,
    });
    db.execute("UPDATE memory_engine_model_operations SET cost_usd_micros = ?, cost_unknown = ?, usage_json = ?, reserved_cost_usd_micros = 0, error = ?, status = CASE WHEN ? = 1 THEN 'failed' ELSE status END, completed_at_ms = CASE WHEN ? = 1 THEN ? ELSE completed_at_ms END WHERE account_id = ? AND operation_id = ? AND status = 'running' AND lease_expires_at_ms > ?",
        &[json!(cost), json!(u8::from(unknown)), json!(receipt.as_ref().map(serde_json::to_string).transpose()?), json!(failure.map(ToString::to_string)), json!(u8::from(failure.is_some())), json!(u8::from(failure.is_some())), json!(now_ms()), json!(account_id), json!(operation), json!(now_ms())])
}

pub(crate) fn check_operation(db: &Database, account_id: &str, operation: &str) -> AppResult<()> {
    if amount(db, "SELECT COUNT(*) AS amount FROM memory_engine_model_operations WHERE account_id = ? AND operation_id = ? AND status = 'running' AND lease_expires_at_ms > ?", &[json!(account_id), json!(operation), json!(now_ms())])? != 1 {
        return Err(Failure::conflict("This model operation no longer owns its publication lease."));
    }
    Ok(())
}

pub(crate) fn finish_operation(
    db: &Database,
    account_id: &str,
    operation: &str,
    failure: Option<&str>,
) -> AppResult<()> {
    db.execute("UPDATE memory_engine_model_operations SET status = ?, error = ?, reserved_cost_usd_micros = 0, completed_at_ms = ? WHERE account_id = ? AND operation_id = ? AND status = 'running'",
        &[json!(if failure.is_some() { "failed" } else { "succeeded" }), json!(failure), json!(now_ms()), json!(account_id), json!(operation)])
}
