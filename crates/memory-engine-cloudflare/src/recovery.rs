//! Administrator-only, portable raw-row recovery. No SQL supplied by a client is executed.
//!
//! The import format deliberately stores JSON payload columns as JSON *text* inside
//! canonical NDJSON rows. `PostgreSQL`'s original payload bytes, integers, SQL IDs,
//! timestamps, hashes, and nulls survive without model deserialization or backfill.
use std::collections::{BTreeMap, BTreeSet};

use futures_util::StreamExt;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use worker::{Bucket, Env, Method, Request, Response};

use crate::{auth, database::Database, now_ms, random_id, AppResult, Failure};

const BUNDLE_SCHEMA: &str = "memory_engine.cloudflare_import.v1";
const FINGERPRINT_SCHEMA: &str = "memory_engine.cloudflare_fingerprint.v1";
const MANIFEST_SCHEMA: &str = "memory_engine.cloudflare_backup.v1";
const STORAGE_VERSION: i64 = 4;
const MAX_BUNDLE_BYTES: usize = 16 * 1024 * 1024;
const MAX_ROW_BYTES: usize = 1024 * 1024;
const MAX_ROWS: usize = 100_000;
const CHUNK_BYTES: usize = 256 * 1024;
const MAX_MANIFEST_BYTES: usize = 128 * 1024;
const MAX_BACKUPS: usize = 512;
const MAX_R2_KEYS: usize = MAX_BACKUPS * (MAX_BUNDLE_BYTES / CHUNK_BYTES + 1);
const DAY_MS: i64 = 86_400_000;
const MANIFEST_PREFIX: &str = "scry/recovery/v1/manifests/";
const CHUNK_PREFIX: &str = "scry/recovery/v1/chunks/";
const PROVENANCE: &str = "memory_engine_recovery_imports";
const REVISIONS: &str = "memory_engine_source_revisions";
const SEQUENCE_TABLES: [&str; 2] = ["memory_engine_attempts", "memory_engine_waitlist_audit_log"];

// Dependency order, not a client-selected identifier list. Schema8/9 columns are
// pinned below; current SQLite columns are checked against the migration-owned
// catalog. New tables require an explicit recovery cutover here.
const TABLES: &[&str] = &[
    "memory_engine_accounts",
    "memory_engine_rate_limits",
    "memory_engine_auth_challenges",
    "memory_engine_api_sessions",
    "memory_engine_browser_sessions",
    "memory_engine_return_notification_preferences",
    "memory_engine_return_notification_schedule",
    "memory_engine_waitlist_entries",
    "memory_engine_waitlist_audit_log",
    "memory_engine_mail_receipts",
    "memory_engine_mail_outbox",
    "memory_engine_source_documents",
    "memory_engine_reference_spans",
    "memory_engine_generation_runs",
    "memory_engine_concept_reference_notes",
    "memory_engine_generated_prompt_drafts",
    "memory_engine_review_units",
    "memory_engine_schedules",
    "memory_engine_attempts",
    "memory_engine_applied_reviews",
    "memory_engine_content_feedback",
    "memory_engine_remediation_packs",
    "memory_engine_review_exposures",
    "memory_engine_generation_jobs",
    "memory_engine_generation_job_attempts",
    REVISIONS,
    "memory_engine_model_operations",
    PROVENANCE,
];

#[derive(Clone, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Source {
    engine: String,
    schema_version: i64,
    schema_name: String,
    instance: String,
    captured_at_ms: i64,
    migrations: Vec<Value>,
    sequences: Vec<Sequence>,
    // Preserved as provenance, never activated on the separate restore object.
    recovery_state: Option<Value>,
}

#[derive(Clone, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Sequence {
    table: String,
    value: i64,
    is_called: bool,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct TableData {
    name: String,
    columns: Vec<String>,
    rows: usize,
    sha256: String,
    data: String,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Bundle {
    schema: String,
    source: Source,
    tables: Vec<TableData>,
    fingerprint: Option<String>,
}

#[derive(Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
struct TableReceipt {
    name: String,
    rows: usize,
    sha256: String,
}

#[derive(Clone, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Fingerprint {
    schema: String,
    storage_schema_version: i64,
    schema_versions: Vec<i64>,
    sha256: String,
    rows: usize,
    account_count: usize,
    account_ids_sha256: String,
    tables: Vec<TableReceipt>,
}

#[derive(Clone, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Chunk {
    index: usize,
    bytes: usize,
    sha256: String,
}

#[derive(Clone, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Manifest {
    schema: String,
    complete: bool,
    backup_id: String,
    source_instance: String,
    created_at_ms: i64,
    bytes: usize,
    bundle_sha256: String,
    fingerprint: Fingerprint,
    chunks: Vec<Chunk>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct RestoreRequest {
    backup_id: String,
    target: String,
    manifest_sha256: String,
    bundle_sha256: String,
    fingerprint_sha256: String,
    account_ids_sha256: String,
    account_count: usize,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct RetentionRequest {
    target: String,
    days: u32,
    keep: usize,
    confirm: bool,
}

#[derive(Deserialize)]
struct Column {
    name: String,
    #[serde(rename = "type")]
    kind: String,
    notnull: i64,
    pk: i64,
}

fn digest(bytes: &[u8]) -> String {
    crate::auth::encode_hex(&Sha256::digest(bytes))
}

fn is_hash(value: &str) -> bool {
    value.len() == 64
        && value
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
}

fn is_target(value: &str) -> bool {
    value == "app"
        || value.strip_prefix("restore-").is_some_and(|suffix| {
            (8..=64).contains(&suffix.len())
                && suffix
                    .bytes()
                    .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
        })
}

fn identifier(value: &str) -> AppResult<String> {
    if value.is_empty()
        || !value
            .bytes()
            .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'_')
    {
        return Err(Failure::bad_request("unknown storage identifier"));
    }
    Ok(format!("\"{value}\""))
}

fn object_name(req: &Request) -> AppResult<String> {
    let name = req
        .headers()
        .get("x-scry-object-name")?
        .ok_or_else(|| Failure::forbidden("trusted recovery object identity required"))?;
    if !is_target(&name) {
        return Err(Failure::forbidden("invalid recovery object identity"));
    }
    Ok(name)
}

fn confirm_target(req: &Request, actual: &str) -> AppResult<()> {
    if req.headers().get("x-scry-confirm-target")?.as_deref() != Some(actual) {
        return Err(Failure::conflict(
            "explicit recovery target does not match this object",
        ));
    }
    Ok(())
}

async fn bounded_body(req: &mut Request, maximum: usize) -> AppResult<Vec<u8>> {
    if req.headers().get("content-encoding")?.is_some() {
        return Err(Failure::bad_request(
            "compressed recovery requests are not accepted",
        ));
    }
    if let Some(length) = req.headers().get("content-length")? {
        let length: usize = length
            .parse()
            .map_err(|_| Failure::bad_request("invalid content length"))?;
        if length > maximum {
            return Err(Failure::new(
                413,
                "recovery request exceeds the explicit size bound",
            ));
        }
    }
    let mut stream = req.stream()?;
    let collect = async {
        let mut bytes = Vec::new();
        while let Some(chunk) = stream.next().await {
            let chunk = chunk?;
            if chunk.len() > maximum.saturating_sub(bytes.len()) {
                return Err(Failure::new(
                    413,
                    "recovery request exceeds the explicit size bound",
                ));
            }
            bytes.extend_from_slice(&chunk);
        }
        Ok(bytes)
    };
    let deadline = worker::Delay::from(std::time::Duration::from_secs(30));
    futures_util::pin_mut!(collect, deadline);
    match futures_util::future::select(collect, deadline).await {
        futures_util::future::Either::Left((result, _)) => result,
        futures_util::future::Either::Right(_) => {
            Err(Failure::new(408, "recovery body deadline exceeded"))
        }
    }
}

fn parse<T: serde::de::DeserializeOwned>(bytes: &[u8]) -> AppResult<T> {
    serde_json::from_slice(bytes).map_err(|_| Failure::bad_request("invalid recovery document"))
}

fn response(value: &impl Serialize) -> AppResult<Response> {
    let mut response = Response::from_json(value)?;
    response.headers_mut().set("cache-control", "no-store")?;
    Ok(response)
}

fn bytes_response(bytes: Vec<u8>) -> AppResult<Response> {
    let hash = digest(&bytes);
    let mut response = Response::from_bytes(bytes)?;
    response
        .headers_mut()
        .set("content-type", "application/json; charset=utf-8")?;
    response.headers_mut().set("cache-control", "no-store")?;
    response.headers_mut().set("x-scry-content-sha256", &hash)?;
    Ok(response)
}

fn column_catalog(db: &Database, table: &str) -> AppResult<Vec<Column>> {
    let mut columns =
        db.query::<Column>(&format!("PRAGMA table_info({})", identifier(table)?), &[])?;
    if columns.is_empty() {
        return Err(Failure::conflict("required recovery table is missing"));
    }
    columns.sort_by(|left, right| left.name.cmp(&right.name));
    Ok(columns)
}

fn check_catalog(db: &Database) -> AppResult<()> {
    let rows = db.query::<Value>(crate::database::APPLICATION_TABLES_SQL, &[])?;
    let mut expected: BTreeSet<&str> = TABLES.iter().copied().collect();
    expected.insert("memory_engine_schema_migrations");
    expected.insert("memory_engine_recovery_state");
    let actual: BTreeSet<&str> = rows.iter().filter_map(|row| row["name"].as_str()).collect();
    if actual != expected {
        return Err(Failure::conflict(
            "unknown or missing authoritative storage table",
        ));
    }
    let ledger = db.query::<Value>(
        "SELECT version, name FROM memory_engine_schema_migrations ORDER BY version",
        &[],
    )?;
    let names = ["001-auth", "002-study", "003-jobs", "004-recovery"];
    if ledger.len() != names.len()
        || ledger
            .iter()
            .zip(names)
            .zip(1..=STORAGE_VERSION)
            .any(|((row, name), version)| {
                row["version"].as_i64() != Some(version) || row["name"].as_str() != Some(name)
            })
    {
        return Err(Failure::conflict(
            "unsupported SQLite recovery schema ledger",
        ));
    }
    Ok(())
}

fn count(db: &Database, table: &str) -> AppResult<usize> {
    let rows = db.query::<Value>(
        &format!("SELECT COUNT(*) AS n FROM {}", identifier(table)?),
        &[],
    )?;
    rows.first()
        .and_then(|row| row["n"].as_u64())
        .and_then(|n| usize::try_from(n).ok())
        .ok_or_else(|| Failure::internal("invalid recovery row count"))
}

fn ensure_empty(db: &Database) -> AppResult<()> {
    check_catalog(db)?;
    for table in TABLES {
        if count(db, table)? != 0 {
            return Err(Failure::conflict("recovery destination is not empty"));
        }
    }
    if !db
        .query::<Value>("SELECT name FROM sqlite_sequence WHERE seq > 0", &[])?
        .is_empty()
    {
        return Err(Failure::conflict(
            "recovery destination has prior sequence history",
        ));
    }
    Ok(())
}

fn scalar(value: &Value, column: &Column) -> AppResult<()> {
    let valid = match value {
        Value::Null => column.notnull == 0 && column.pk == 0,
        Value::String(_) => column.kind == "TEXT",
        Value::Number(number) => {
            column.kind == "INTEGER"
                && number
                    .as_i64()
                    .is_some_and(|n| (-9_007_199_254_740_991..=9_007_199_254_740_991).contains(&n))
        }
        _ => false,
    };
    if !valid {
        return Err(Failure::bad_request(
            "recovery row violates its scalar column contract",
        ));
    }
    Ok(())
}

fn export_table(
    db: &Database,
    table: &str,
    requested_columns: Option<&[String]>,
    allowance: usize,
) -> AppResult<TableData> {
    let catalog = column_catalog(db, table)?;
    let columns: Vec<String> = requested_columns.map_or_else(
        || catalog.iter().map(|column| column.name.clone()).collect(),
        <[String]>::to_vec,
    );
    if columns
        .iter()
        .any(|name| !catalog.iter().any(|column| column.name == *name))
    {
        return Err(Failure::bad_request("unknown recovery column"));
    }
    let quoted: Vec<String> = columns
        .iter()
        .map(|name| identifier(name))
        .collect::<AppResult<_>>()?;
    let mut primary: Vec<&Column> = catalog.iter().filter(|column| column.pk > 0).collect();
    primary.sort_by_key(|column| column.pk);
    if primary.is_empty() {
        return Err(Failure::conflict("recovery requires a stable primary key"));
    }
    let order = primary
        .iter()
        .map(|column| identifier(&column.name))
        .collect::<AppResult<Vec<_>>>()?
        .join(",");
    let table_name = identifier(table)?;
    let row_count = count(db, table)?;
    if row_count > MAX_ROWS {
        return Err(Failure::new(
            413,
            "recovery row count exceeds the explicit bound",
        ));
    }
    check_export_size(db, &table_name, &quoted, allowance)?;
    if primary.iter().any(|column| !columns.contains(&column.name)) {
        return Err(Failure::conflict(
            "recovery projection omitted a primary key",
        ));
    }
    let first_sql = format!(
        "SELECT {} FROM {table_name} ORDER BY {order} LIMIT 8",
        quoted.join(",")
    );
    let placeholders = vec!["?"; primary.len()].join(",");
    let following_sql = format!(
        "SELECT {} FROM {table_name} WHERE ({order}) > ({placeholders}) ORDER BY {order} LIMIT 8",
        quoted.join(",")
    );
    let mut after: Option<Vec<Value>> = None;
    let mut data = String::new();
    let mut rows = 0;
    while rows < row_count {
        let page = if let Some(after) = &after {
            db.query::<BTreeMap<String, Value>>(&following_sql, after)?
        } else {
            db.query::<BTreeMap<String, Value>>(&first_sql, &[])?
        };
        if page.is_empty() {
            return Err(Failure::conflict("recovery snapshot changed during export"));
        }
        after = page
            .last()
            .map(|row| {
                primary
                    .iter()
                    .map(|column| {
                        row.get(&column.name)
                            .cloned()
                            .ok_or_else(|| Failure::internal("missing recovery primary key"))
                    })
                    .collect::<AppResult<Vec<_>>>()
            })
            .transpose()?;
        for row in page {
            let line = encode_export_row(&row, &columns)?;
            if line.len() > MAX_ROW_BYTES || line.len() + 1 > allowance.saturating_sub(data.len()) {
                return Err(Failure::new(
                    413,
                    "recovery data exceeds the explicit size bound",
                ));
            }
            data.push_str(&line);
            data.push('\n');
            rows += 1;
        }
    }
    Ok(TableData {
        name: table.to_owned(),
        columns,
        rows,
        sha256: digest(data.as_bytes()),
        data,
    })
}

fn check_export_size(
    db: &Database,
    table_name: &str,
    columns: &[String],
    allowance: usize,
) -> AppResult<()> {
    // Check per-row cell size before moving values across the JS/Wasm boundary.
    let size_expression = columns
        .iter()
        .map(|column| format!("COALESCE(length(CAST({column} AS BLOB)),0)"))
        .collect::<Vec<_>>()
        .join("+");
    let sizes = db.query::<Value>(&format!("SELECT COALESCE(MAX({size_expression}),0) AS largest, COALESCE(SUM({size_expression}),0) AS total FROM {table_name}"), &[])?;
    let sizes = sizes
        .first()
        .ok_or_else(|| Failure::internal("missing recovery size result"))?;
    let largest = sizes["largest"]
        .as_u64()
        .and_then(|size| usize::try_from(size).ok());
    let total = sizes["total"]
        .as_u64()
        .and_then(|size| usize::try_from(size).ok());
    if largest.is_none_or(|size| size > MAX_ROW_BYTES) || total.is_none_or(|size| size > allowance)
    {
        return Err(Failure::new(
            413,
            "recovery data exceeds the explicit size bound",
        ));
    }
    Ok(())
}

fn encode_export_row(row: &BTreeMap<String, Value>, columns: &[String]) -> AppResult<String> {
    let values: Vec<&Value> = columns
        .iter()
        .map(|column| {
            row.get(column)
                .ok_or_else(|| Failure::internal("missing recovery column value"))
        })
        .collect::<AppResult<_>>()?;
    Ok(serde_json::to_string(&values)?)
}

fn sequences(db: &Database) -> AppResult<Vec<Sequence>> {
    let rows = db.query::<Value>("SELECT name, seq FROM sqlite_sequence ORDER BY name", &[])?;
    if rows.iter().any(|row| {
        !row["name"]
            .as_str()
            .is_some_and(|name| SEQUENCE_TABLES.contains(&name))
    }) {
        return Err(Failure::conflict("unknown SQLite sequence"));
    }
    SEQUENCE_TABLES
        .iter()
        .map(|table| {
            let value = rows
                .iter()
                .find(|row| row["name"].as_str() == Some(table))
                .map_or(Some(0), |row| row["seq"].as_i64())
                .ok_or_else(|| Failure::conflict("invalid SQLite sequence"))?;
            Ok(Sequence {
                table: (*table).to_owned(),
                value,
                is_called: value > 0,
            })
        })
        .collect()
}

fn table_receipts(tables: &[TableData], include_provenance: bool) -> Vec<TableReceipt> {
    let mut receipts: Vec<_> = tables
        .iter()
        .filter(|table| include_provenance || table.name != PROVENANCE)
        .map(|table| TableReceipt {
            name: table.name.clone(),
            rows: table.rows,
            sha256: table.sha256.clone(),
        })
        .collect();
    receipts.sort_by(|left, right| left.name.cmp(&right.name));
    receipts
}

fn account_identity(tables: &[TableData]) -> AppResult<(usize, String)> {
    let table = tables
        .iter()
        .find(|table| table.name == "memory_engine_accounts")
        .ok_or_else(|| Failure::bad_request("account table is missing"))?;
    let index = table
        .columns
        .iter()
        .position(|column| column == "account_id")
        .ok_or_else(|| Failure::bad_request("account identity column is missing"))?;
    let mut ids = Vec::new();
    for line in table.data.lines() {
        let row: Vec<Value> = parse(line.as_bytes())?;
        let id = row
            .get(index)
            .and_then(Value::as_str)
            .ok_or_else(|| Failure::bad_request("invalid account identity"))?;
        if id.is_empty()
            || id.len() > 128
            || !id
                .bytes()
                .all(|b| b.is_ascii_alphanumeric() || b"._:-".contains(&b))
        {
            return Err(Failure::bad_request("unsupported account identity"));
        }
        ids.push(id.to_owned());
    }
    ids.sort();
    if ids.windows(2).any(|pair| pair[0] == pair[1]) {
        return Err(Failure::bad_request("duplicate account identity"));
    }
    Ok((ids.len(), digest(&serde_json::to_vec(&ids)?)))
}

fn fingerprint(tables: &[TableData], sequence_rows: &[Sequence]) -> AppResult<Fingerprint> {
    let tables_receipt = table_receipts(tables, false);
    let mut hash = Sha256::new();
    // Version 5 changes enrollment, not the authoritative raw-row envelope.
    // Keep the v4 digest domain so old backups verify byte-for-byte in quarantine.
    hash.update(format!("{FINGERPRINT_SCHEMA}\n4\n").as_bytes());
    for table in &tables_receipt {
        hash.update(format!("{}\t{}\t{}\n", table.name, table.rows, table.sha256).as_bytes());
    }
    let mut sequence_rows: Vec<_> = sequence_rows.iter().collect();
    sequence_rows.sort_by(|left, right| left.table.cmp(&right.table));
    for sequence in sequence_rows {
        hash.update(
            format!(
                "{}\t{}\t{}\n",
                sequence.table, sequence.value, sequence.is_called
            )
            .as_bytes(),
        );
    }
    let (account_count, account_ids_sha256) = account_identity(tables)?;
    Ok(Fingerprint {
        schema: FINGERPRINT_SCHEMA.to_owned(),
        storage_schema_version: STORAGE_VERSION,
        schema_versions: (1..=STORAGE_VERSION).collect(),
        sha256: crate::auth::encode_hex(&hash.finalize()),
        rows: tables_receipt.iter().map(|table| table.rows).sum(),
        account_count,
        account_ids_sha256,
        tables: tables_receipt,
    })
}

fn snapshot(db: &Database, instance: &str) -> AppResult<(Bundle, Fingerprint)> {
    db.transaction(|| {
        check_catalog(db)?;
        let mut tables = Vec::new();
        let mut bytes = 0;
        let mut rows = 0;
        for table in TABLES {
            let data = export_table(db, table, None, MAX_BUNDLE_BYTES.saturating_sub(bytes))?;
            bytes += data.data.len();
            rows += data.rows;
            if rows > MAX_ROWS {
                return Err(Failure::new(413, "recovery row count exceeds the explicit bound"));
            }
            tables.push(data);
        }
        let sequence_rows = sequences(db)?;
        let fingerprint = fingerprint(&tables, &sequence_rows)?;
        let source = Source {
            engine: "cloudflare-sqlite".to_owned(), schema_version: STORAGE_VERSION,
            schema_name: "main".to_owned(), instance: instance.to_owned(), captured_at_ms: now_ms(),
            migrations: db.query("SELECT version, name, applied_at_ms FROM memory_engine_schema_migrations ORDER BY version", &[])?,
            sequences: sequence_rows,
            recovery_state: Some(db.query::<Value>("SELECT singleton, learner_traffic_enabled, next_backup_at_ms, lease_token, lease_expires_at_ms, last_backup_id, last_backup_at_ms FROM memory_engine_recovery_state WHERE singleton = 1", &[])?
                .into_iter().next().ok_or_else(|| Failure::conflict("recovery control row is missing"))?),
        };
        Ok((Bundle { schema: BUNDLE_SCHEMA.to_owned(), source, tables, fingerprint: Some(fingerprint.sha256.clone()) }, fingerprint))
    })
}

pub(crate) fn fingerprint_sha256(db: &Database) -> AppResult<String> {
    snapshot(db, "app").map(|(_, fingerprint)| fingerprint.sha256)
}

fn encode_bundle(bundle: &Bundle) -> AppResult<Vec<u8>> {
    let bytes = serde_json::to_vec(bundle)?;
    if bytes.len() > MAX_BUNDLE_BYTES {
        return Err(Failure::new(
            413,
            "encoded recovery bundle exceeds 16 MiB; nothing was truncated",
        ));
    }
    Ok(bytes)
}

fn postgres_columns(table: &str) -> Option<Vec<String>> {
    let columns = match table {
        "memory_engine_accounts" => "account_id created_at_ms active_graded_review",
        "memory_engine_rate_limits" => "rate_limit_key window_start_ms attempt_count",
        "memory_engine_auth_challenges" => "challenge_hash email_normalized expires_at_ms consumed_at_ms",
        "memory_engine_api_sessions" => "session_token_hash account_id created_at_ms expires_at_ms revoked_at_ms",
        "memory_engine_browser_sessions" => "session_id_hash account_id session_token_hash csrf_token_hash created_at_ms expires_at_ms revoked_at_ms",
        "memory_engine_return_notification_preferences" => "account_id email_normalized enabled last_sent_at_ms unsubscribe_nonce updated_at_ms claim_id claim_expires_at_ms pending_delivery_key pending_due_count pending_unsubscribe_expires_at_ms retry_attempts next_retry_at_ms",
        "memory_engine_waitlist_entries" => "email_normalized source created_at_ms updated_at_ms invited_at_ms",
        "memory_engine_waitlist_audit_log" => "audit_id email_normalized event occurred_at_ms",
        "memory_engine_source_documents" => "account_id source_document_id document created_at_ms",
        "memory_engine_reference_spans" => "account_id reference_span_id source_document_id span created_at_ms",
        "memory_engine_generation_runs" => "account_id generation_run_id run started_at_ms",
        "memory_engine_concept_reference_notes" => "account_id concept_key note updated_at_ms",
        "memory_engine_generated_prompt_drafts" => "account_id draft_id review_unit_id draft created_at_ms",
        "memory_engine_review_units" => "account_id review_unit_id record created_at_ms archived_at_ms",
        "memory_engine_schedules" => "account_id review_unit_id state updated_at_ms",
        "memory_engine_attempts" => "account_id attempt_id review_unit_id prompt_id idempotency_key attempt occurred_at_ms",
        "memory_engine_applied_reviews" => "account_id receipt_key review_unit_id attempt expected_prior_schedule_state schedule_state applied_at_ms",
        "memory_engine_content_feedback" => "account_id feedback_id review_unit_id feedback occurred_at_ms",
        "memory_engine_remediation_packs" => "account_id pack_id pack attempt_id created_at_ms",
        "memory_engine_review_exposures" => "account_id review_unit_id occurrence_key exposure revealed_at_ms",
        "memory_engine_generation_jobs" => "account_id job_id source_id title status card_count attempts error model_key cost_usd_micros created_at_ms updated_at_ms retry_at_ms lease_owner lease_expires_at_ms lease_token reserved_cost_usd_micros",
        "memory_engine_generation_job_attempts" => "account_id job_id attempt lease_token status generation_run_id reservation_cost_usd_micros reserved_cost_usd_micros cost_usd_micros error started_at_ms completed_at_ms updated_at_ms",
        _ => return None,
    };
    let mut columns: Vec<_> = columns.split_whitespace().map(str::to_owned).collect();
    columns.sort();
    Some(columns)
}

fn validate_bundle(
    db: &Database,
    bundle: &Bundle,
    expected_count: usize,
    expected_ids: &str,
) -> AppResult<()> {
    let postgres = bundle.source.engine == "postgresql";
    if bundle.schema != BUNDLE_SCHEMA
        || bundle.source.captured_at_ms < 0
        || !(postgres && matches!(bundle.source.schema_version, 8..=10)
            || bundle.source.engine == "cloudflare-sqlite" && bundle.source.schema_version == 4)
    {
        return Err(Failure::bad_request("unsupported recovery source schema"));
    }
    if !is_hash(expected_ids) || expected_count > MAX_ROWS {
        return Err(Failure::bad_request(
            "explicit expected account identity fingerprint required",
        ));
    }
    if !postgres && (bundle.source.schema_name != "main" || !is_target(&bundle.source.instance)) {
        return Err(Failure::bad_request("invalid source object identity"));
    }
    if postgres {
        identifier(&bundle.source.schema_name)?;
    }
    validate_source_state(db, &bundle.source, postgres)?;
    validate_source_ledger(&bundle.source, postgres)?;
    let expected: BTreeSet<&str> = TABLES
        .iter()
        .copied()
        .filter(|table| {
            !postgres
                || postgres_columns(table).is_some()
                    && (*table != "memory_engine_review_exposures"
                        || bundle.source.schema_version >= 9)
        })
        .collect();
    let actual: BTreeSet<&str> = bundle
        .tables
        .iter()
        .map(|table| table.name.as_str())
        .collect();
    if actual != expected || actual.len() != bundle.tables.len() {
        return Err(Failure::bad_request(
            "unknown, missing, or duplicated recovery table",
        ));
    }
    let mut total_rows = 0;
    let mut total_bytes = 0;
    for table in &bundle.tables {
        validate_table_data(db, table, postgres)?;
        total_rows += table.rows;
        total_bytes += table.data.len();
        if total_rows > MAX_ROWS || total_bytes > MAX_BUNDLE_BYTES {
            return Err(Failure::new(
                413,
                "recovery bundle exceeds the explicit bound",
            ));
        }
    }
    validate_source_sequences(&bundle.source.sequences, postgres)?;
    let (account_count, ids_hash) = account_identity(&bundle.tables)?;
    if account_count != expected_count || ids_hash != expected_ids || postgres && account_count != 1
    {
        return Err(Failure::conflict(
            "source account identity does not match the explicit migration scope",
        ));
    }
    if !postgres
        && bundle.fingerprint.as_deref()
            != Some(
                fingerprint(&bundle.tables, &bundle.source.sequences)?
                    .sha256
                    .as_str(),
            )
        || postgres && bundle.fingerprint.is_some()
    {
        return Err(Failure::bad_request("source fingerprint mismatch"));
    }
    Ok(())
}

fn validate_source_state(db: &Database, source: &Source, postgres: bool) -> AppResult<()> {
    if postgres {
        if source.recovery_state.is_some() {
            return Err(Failure::bad_request(
                "unexpected PostgreSQL recovery control state",
            ));
        }
    } else {
        let state = source
            .recovery_state
            .as_ref()
            .and_then(Value::as_object)
            .ok_or_else(|| Failure::bad_request("source recovery control state is missing"))?;
        let columns = column_catalog(db, "memory_engine_recovery_state")?;
        if state.len() != columns.len() || state.get("singleton").and_then(Value::as_i64) != Some(1)
        {
            return Err(Failure::bad_request(
                "unknown source recovery control state",
            ));
        }
        for column in &columns {
            scalar(
                state.get(&column.name).ok_or_else(|| {
                    Failure::bad_request("source recovery control column is missing")
                })?,
                column,
            )?;
        }
    }
    Ok(())
}

fn validate_source_ledger(source: &Source, postgres: bool) -> AppResult<()> {
    if i64::try_from(source.migrations.len()).ok() != Some(source.schema_version) {
        return Err(Failure::bad_request("incomplete source migration ledger"));
    }
    let names = ["001-auth", "002-study", "003-jobs", "004-recovery"];
    for (index, (migration, version)) in source
        .migrations
        .iter()
        .zip(1..=source.schema_version)
        .enumerate()
    {
        let row = migration
            .as_object()
            .ok_or_else(|| Failure::bad_request("invalid source migration row"))?;
        if row.len() != if postgres { 2 } else { 3 }
            || migration["version"].as_i64() != Some(version)
            || migration["applied_at_ms"].as_i64().is_none()
            || !postgres && migration["name"].as_str() != names.get(index).copied()
        {
            return Err(Failure::bad_request("unknown source migration ledger"));
        }
    }
    Ok(())
}

fn validate_table_data(db: &Database, table: &TableData, postgres: bool) -> AppResult<()> {
    let catalog = column_catalog(db, &table.name)?;
    let expected_columns = if postgres {
        postgres_columns(&table.name)
            .ok_or_else(|| Failure::bad_request("unknown PostgreSQL table"))?
    } else {
        catalog.iter().map(|column| column.name.clone()).collect()
    };
    if table.columns != expected_columns
        || !is_hash(&table.sha256)
        || digest(table.data.as_bytes()) != table.sha256
        || !table.data.is_empty() && !table.data.ends_with('\n')
    {
        return Err(Failure::bad_request(
            "recovery table schema or checksum mismatch",
        ));
    }
    let mut rows = 0;
    for line in table.data.lines() {
        if line.len() > MAX_ROW_BYTES {
            return Err(Failure::new(
                413,
                "recovery row exceeds the explicit size bound",
            ));
        }
        let values: Vec<Value> = parse(line.as_bytes())?;
        if values.len() != table.columns.len() || serde_json::to_string(&values)? != line {
            return Err(Failure::bad_request(
                "recovery row is not canonical or complete",
            ));
        }
        for (name, value) in table.columns.iter().zip(&values) {
            let column = catalog
                .iter()
                .find(|column| column.name == *name)
                .ok_or_else(|| Failure::bad_request("source column has no destination mapping"))?;
            scalar(value, column)?;
        }
        rows += 1;
    }
    if rows != table.rows {
        return Err(Failure::bad_request("recovery table row count mismatch"));
    }
    Ok(())
}

fn validate_source_sequences(sequences: &[Sequence], postgres: bool) -> AppResult<()> {
    let sequence_names: BTreeSet<&str> = sequences
        .iter()
        .map(|sequence| sequence.table.as_str())
        .collect();
    if sequence_names != SEQUENCE_TABLES.into_iter().collect()
        || sequences.len() != SEQUENCE_TABLES.len()
        || sequences.iter().any(|sequence| {
            sequence.value < 0
                || sequence.value > 9_007_199_254_740_991
                || sequence.is_called && sequence.value == 0
                || postgres && sequence.value == 0
                || !postgres && !sequence.is_called && sequence.value != 0
        })
    {
        return Err(Failure::bad_request(
            "unsupported or incomplete source sequence state",
        ));
    }
    Ok(())
}

fn normalize_legacy_holds(db: &Database, postgres: bool) -> AppResult<Vec<Value>> {
    let accounts = db.query::<Value>("SELECT account_id, active_graded_review FROM memory_engine_accounts WHERE active_graded_review IS NOT NULL", &[])?;
    let mut transformations = Vec::new();
    for account in accounts {
        let original = account["active_graded_review"]
            .as_str()
            .ok_or_else(|| Failure::conflict("invalid persisted graded review"))?;
        let mut hold: Value = parse(original.as_bytes())?;
        let object = hold
            .as_object()
            .ok_or_else(|| Failure::conflict("invalid persisted graded review"))?;
        match object.get("state").and_then(Value::as_str) {
            Some("consumed") if object.len() == 2 => {
                if object
                    .get("idempotency_key")
                    .and_then(Value::as_str)
                    .is_some()
                {
                    continue;
                }
                let key = object
                    .get("idempotencyKey")
                    .and_then(Value::as_str)
                    .filter(|_| postgres)
                    .ok_or_else(|| Failure::conflict("unknown consumed review representation"))?
                    .to_owned();
                hold = json!({"state": "consumed", "idempotency_key": key});
                let normalized = serde_json::to_string(&hold)?;
                db.execute("UPDATE memory_engine_accounts SET active_graded_review = ? WHERE account_id = ?", &[
                    json!(normalized), account["account_id"].clone(),
                ])?;
                transformations.push(
                    json!({"table": "memory_engine_accounts", "accountId": account["account_id"],
                    "column": "active_graded_review", "operation": "consumed_idempotency_key",
                    "original": original, "originalSha256": digest(original.as_bytes()),
                    "replacementSha256": digest(normalized.as_bytes())}),
                );
            }
            Some("active")
                if object.len() == 4
                    && object
                        .get("review_unit_id")
                        .and_then(Value::as_str)
                        .is_some()
                    && object
                        .get("idempotency_key")
                        .and_then(Value::as_str)
                        .is_some()
                    && object.get("view").is_some_and(Value::is_object) =>
            {
                let _: memory_engine_api_state::StudyViewResponse =
                    serde_json::from_value(hold["view"].clone())
                        .map_err(|_| Failure::conflict("persisted graded view is not readable"))?;
            }
            _ => {
                return Err(Failure::conflict(
                    "unknown persisted graded review representation",
                ))
            }
        }
    }
    Ok(transformations)
}

fn import(
    db: &Database,
    bundle: &Bundle,
    source_sha256: &str,
    target: &str,
    expected_count: usize,
    expected_ids: &str,
) -> AppResult<Value> {
    check_catalog(db)?;
    validate_bundle(db, bundle, expected_count, expected_ids)?;
    db.transaction(|| {
        ensure_empty(db)?;
        restore_import_rows(db, &bundle.tables)?;
        restore_import_sequences(
            db,
            &bundle.source.sequences,
            bundle.source.engine == "postgresql",
        )?;
        if !db.query::<Value>("PRAGMA foreign_key_check", &[])?.is_empty() {
            return Err(Failure::conflict("import has unresolved foreign keys"));
        }
        // Read every source projection back, not merely a row count or an INSERT
        // success. This catches trigger rewrites, serialization loss, and bad mappings.
        for table in &bundle.tables {
            let actual = export_table(db, &table.name, Some(&table.columns), MAX_BUNDLE_BYTES)?;
            if actual.rows != table.rows || actual.sha256 != table.sha256 {
                return Err(Failure::conflict("import readback differs from source rows"));
            }
        }
        // Normalize the one known native SQL/serde tombstone mismatch only after
        // exact raw readback. Preserve the original private bytes in provenance.
        let transformations = normalize_legacy_holds(db, bundle.source.engine == "postgresql")?;
        let source_metadata = json!({"source": bundle.source, "transforms": transformations});
        let mut tables = Vec::new();
        let mut bytes = 0;
        for table in TABLES {
            let data = export_table(db, table, None, MAX_BUNDLE_BYTES.saturating_sub(bytes))?;
            bytes += data.data.len();
            tables.push(data);
        }
        let actual = fingerprint(&tables, &sequences(db)?)?;
        if bundle.source.engine == "cloudflare-sqlite" && bundle.fingerprint.as_deref() != Some(actual.sha256.as_str()) {
            return Err(Failure::conflict("restored authoritative fingerprint differs"));
        }
        db.execute("INSERT INTO memory_engine_recovery_imports (import_id, source_sha256, source_metadata, source_tables, destination_sha256, imported_at_ms, target) VALUES (?, ?, ?, ?, ?, ?, ?)", &[
            json!(format!("import_{source_sha256}")), json!(source_sha256), json!(serde_json::to_string(&source_metadata)?),
            json!(serde_json::to_string(&table_receipts(&bundle.tables, true))?), json!(actual.sha256), json!(now_ms()), json!(target),
        ])?;
        Ok(json!({"schema": "memory_engine.cloudflare_import_receipt.v1", "sourceSha256": source_sha256,
            "sourceSchemaVersion": bundle.source.schema_version, "sourceTables": table_receipts(&bundle.tables, true),
            "fingerprint": actual, "transformedRows": transformations.len(), "verified": true}))
    })
}

fn restore_import_rows(db: &Database, tables: &[TableData]) -> AppResult<()> {
    for name in TABLES {
        let Some(table) = tables.iter().find(|table| table.name == *name) else {
            continue;
        };
        if *name == REVISIONS {
            // Only rows generated by this transaction's preceding source INSERTs.
            // The destination was empty; restoring the source's fencing epochs is exact.
            db.execute("DELETE FROM memory_engine_source_revisions", &[])?;
        }
        let columns = table
            .columns
            .iter()
            .map(|name| identifier(name))
            .collect::<AppResult<Vec<_>>>()?
            .join(",");
        let placeholders = vec!["?"; table.columns.len()].join(",");
        let sql = format!(
            "INSERT INTO {} ({columns}) VALUES ({placeholders})",
            identifier(name)?
        );
        for line in table.data.lines() {
            let values: Vec<Value> = parse(line.as_bytes())?;
            db.execute(&sql, &values).map_err(|_| {
                Failure::conflict("import violates authoritative storage invariants")
            })?;
        }
    }
    Ok(())
}

fn restore_import_sequences(
    db: &Database,
    sequences: &[Sequence],
    postgres: bool,
) -> AppResult<()> {
    for sequence in sequences {
        // An uncalled PostgreSQL sequence stores its next value. SQLite stores
        // the allocated high-water mark, including zero for an unused sequence.
        let high_water = if postgres && !sequence.is_called {
            sequence.value - 1
        } else {
            sequence.value
        };
        let current = db.query::<Value>(
            "SELECT seq FROM sqlite_sequence WHERE name = ?",
            &[json!(sequence.table)],
        )?;
        if current
            .first()
            .and_then(|row| row["seq"].as_i64())
            .unwrap_or(0)
            > high_water
        {
            return Err(Failure::conflict(
                "source sequence is behind its imported rows",
            ));
        }
        db.execute(
            "DELETE FROM sqlite_sequence WHERE name = ?",
            &[json!(sequence.table)],
        )?;
        if high_water > 0 {
            db.execute(
                "INSERT INTO sqlite_sequence (name, seq) VALUES (?, ?)",
                &[json!(sequence.table), json!(high_water)],
            )?;
        }
    }
    Ok(())
}

fn backup_id(value: &str) -> AppResult<()> {
    let Some((time, hash)) = value.split_once('-') else {
        return Err(Failure::bad_request("invalid backup identifier"));
    };
    if !(1..=16).contains(&time.len())
        || !time.bytes().all(|byte| byte.is_ascii_digit())
        || !is_hash(hash)
    {
        return Err(Failure::bad_request("invalid backup identifier"));
    }
    Ok(())
}

fn manifest_key(id: &str) -> String {
    format!("{MANIFEST_PREFIX}{id}.json")
}
fn chunk_key(id: &str, index: usize) -> String {
    format!("{CHUNK_PREFIX}{id}/{index:04}")
}

async fn r2_bytes(bucket: &Bucket, key: &str, maximum: usize) -> AppResult<Vec<u8>> {
    let object = bucket
        .get(key)
        .execute()
        .await?
        .ok_or_else(|| Failure::not_found("recovery object is missing"))?;
    let expected = usize::try_from(object.size())
        .ok()
        .filter(|size| *size <= maximum)
        .ok_or_else(|| Failure::new(413, "R2 recovery object exceeds the explicit bound"))?;
    let bytes = object
        .body()
        .ok_or_else(|| Failure::conflict("recovery object has no body"))?
        .bytes()
        .await?;
    if bytes.len() != expected {
        return Err(Failure::conflict("R2 recovery object length mismatch"));
    }
    Ok(bytes)
}

fn validate_manifest(manifest: &Manifest, id: &str) -> AppResult<()> {
    backup_id(id)?;
    if manifest.schema != MANIFEST_SCHEMA
        || !manifest.complete
        || manifest.backup_id != id
        || !is_target(&manifest.source_instance)
        || manifest.created_at_ms < 0
        || id != format!("{}-{}", manifest.created_at_ms, manifest.bundle_sha256)
        || manifest.bytes == 0
        || manifest.bytes > MAX_BUNDLE_BYTES
        || !is_hash(&manifest.bundle_sha256)
        || manifest.chunks.len() != manifest.bytes.div_ceil(CHUNK_BYTES)
        || manifest.fingerprint.schema != FINGERPRINT_SCHEMA
        || manifest.fingerprint.storage_schema_version != 4
        || manifest.fingerprint.schema_versions
            != (1..=manifest.fingerprint.storage_schema_version).collect::<Vec<_>>()
        || !is_hash(&manifest.fingerprint.sha256)
        || !is_hash(&manifest.fingerprint.account_ids_sha256)
    {
        return Err(Failure::conflict("invalid or incomplete recovery manifest"));
    }
    let expected_tables: BTreeSet<&str> = TABLES
        .iter()
        .copied()
        .filter(|table| *table != PROVENANCE)
        .collect();
    let actual_tables: BTreeSet<&str> = manifest
        .fingerprint
        .tables
        .iter()
        .map(|table| table.name.as_str())
        .collect();
    if actual_tables != expected_tables
        || actual_tables.len() != manifest.fingerprint.tables.len()
        || manifest.fingerprint.rows > MAX_ROWS
        || manifest.fingerprint.account_count > MAX_ROWS
        || manifest
            .fingerprint
            .tables
            .iter()
            .any(|table| table.rows > MAX_ROWS || !is_hash(&table.sha256))
        || manifest
            .fingerprint
            .tables
            .iter()
            .map(|table| table.rows)
            .sum::<usize>()
            != manifest.fingerprint.rows
    {
        return Err(Failure::conflict(
            "recovery manifest table coverage mismatch",
        ));
    }
    for (index, chunk) in manifest.chunks.iter().enumerate() {
        if chunk.index != index
            || chunk.bytes != (manifest.bytes - index * CHUNK_BYTES).min(CHUNK_BYTES)
            || !is_hash(&chunk.sha256)
        {
            return Err(Failure::conflict(
                "recovery manifest chunk coverage mismatch",
            ));
        }
    }
    Ok(())
}

async fn load_manifest(bucket: &Bucket, id: &str) -> AppResult<(Manifest, String)> {
    backup_id(id)?;
    let bytes = r2_bytes(bucket, &manifest_key(id), MAX_MANIFEST_BYTES).await?;
    let manifest: Manifest = parse(&bytes)?;
    validate_manifest(&manifest, id)?;
    Ok((manifest, digest(&bytes)))
}

fn backup_receipt(manifest: &Manifest, manifest_sha256: &str) -> Value {
    json!({"schema": "memory_engine.cloudflare_backup_receipt.v1", "backupId": manifest.backup_id,
        "manifestSha256": manifest_sha256, "bundleSha256": manifest.bundle_sha256,
        "fingerprintSha256": manifest.fingerprint.sha256, "accountIdsSha256": manifest.fingerprint.account_ids_sha256,
        "accountCount": manifest.fingerprint.account_count, "bytes": manifest.bytes,
        "rows": manifest.fingerprint.rows, "chunks": manifest.chunks.len(), "createdAtMs": manifest.created_at_ms})
}

async fn backup(db: &Database, env: &Env, instance: &str) -> AppResult<Value> {
    if instance != "app" {
        return Err(Failure::forbidden(
            "only the primary object publishes recovery backups",
        ));
    }
    let token = random_id("backup_")?;
    let now = now_ms();
    db.transaction(|| {
        let rows = db.query::<Value>("SELECT lease_expires_at_ms FROM memory_engine_recovery_state WHERE singleton = 1", &[])?;
        if rows.first().and_then(|row| row["lease_expires_at_ms"].as_i64()).is_some_and(|expires| expires > now) {
            return Err(Failure::conflict("a recovery backup is already leased"));
        }
        db.execute("UPDATE memory_engine_recovery_state SET lease_token = ?, lease_expires_at_ms = ?, next_backup_at_ms = ? WHERE singleton = 1", &[json!(token), json!(now + 900_000), json!(now + 900_000)])
    })?;
    let result = async {
        let (manifest, manifest_sha256) = publish_backup(db, env, instance, now).await?;
        db.execute("UPDATE memory_engine_recovery_state SET last_backup_id = ?, last_backup_at_ms = ? WHERE singleton = 1 AND lease_token = ?", &[json!(manifest.backup_id), json!(now), json!(token)])?;
        Ok(backup_receipt(&manifest, &manifest_sha256))
    }.await;
    db.execute("UPDATE memory_engine_recovery_state SET lease_token = NULL, lease_expires_at_ms = NULL, next_backup_at_ms = ? WHERE singleton = 1 AND lease_token = ?", &[
        json!(now_ms() + if result.is_ok() { DAY_MS } else { 300_000 }), json!(token),
    ])?;
    result
}

async fn publish_backup(
    db: &Database,
    env: &Env,
    instance: &str,
    captured_at: i64,
) -> AppResult<(Manifest, String)> {
    let bucket = env.bucket("RECOVERY")?;
    let limit = u32::try_from(MAX_BACKUPS)
        .map_err(|_| Failure::internal("recovery backup limit exceeds the R2 range"))?;
    let inventory = bucket
        .list()
        .prefix(MANIFEST_PREFIX)
        .limit(limit)
        .execute()
        .await?;
    if inventory.truncated() || inventory.objects().len() >= MAX_BACKUPS {
        return Err(Failure::conflict(
            "run retention before publishing additional recovery backups",
        ));
    }
    let (bundle, fingerprint) = snapshot(db, instance)?;
    let bytes = encode_bundle(&bundle)?;
    drop(bundle);
    let bundle_sha256 = digest(&bytes);
    let id = format!("{captured_at}-{bundle_sha256}");
    let chunks = write_backup_chunks(&bucket, &id, &bytes).await?;
    let manifest = Manifest {
        schema: MANIFEST_SCHEMA.to_owned(),
        complete: true,
        backup_id: id,
        source_instance: instance.to_owned(),
        created_at_ms: captured_at,
        bytes: bytes.len(),
        bundle_sha256,
        fingerprint,
        chunks,
    };
    drop(bytes);
    let manifest_sha256 = write_backup_manifest(&bucket, &manifest).await?;
    Ok((manifest, manifest_sha256))
}

async fn write_backup_chunks(bucket: &Bucket, id: &str, bytes: &[u8]) -> AppResult<Vec<Chunk>> {
    let mut chunks = Vec::new();
    for (index, bytes) in bytes.chunks(CHUNK_BYTES).enumerate() {
        let checksum = Sha256::digest(bytes);
        let sha256 = crate::auth::encode_hex(&checksum);
        let key = chunk_key(id, index);
        if bucket
            .put(&key, bytes.to_vec())
            .sha256(checksum.to_vec())
            .execute()
            .await?
            .is_none()
        {
            return Err(Failure::conflict("recovery chunk was not stored"));
        }
        let stored = r2_bytes(bucket, &key, CHUNK_BYTES).await?;
        if stored.len() != bytes.len() || digest(&stored) != sha256 {
            return Err(Failure::conflict(
                "recovery chunk readback checksum mismatch",
            ));
        }
        chunks.push(Chunk {
            index,
            bytes: bytes.len(),
            sha256,
        });
    }
    Ok(chunks)
}

async fn write_backup_manifest(bucket: &Bucket, manifest: &Manifest) -> AppResult<String> {
    let bytes = serde_json::to_vec(manifest)?;
    if bytes.len() > MAX_MANIFEST_BYTES {
        return Err(Failure::new(
            413,
            "recovery manifest exceeds the explicit bound",
        ));
    }
    let checksum = Sha256::digest(&bytes);
    let sha256 = crate::auth::encode_hex(&checksum);
    // The complete marker is published LAST, after every chunk has been read
    // back and verified. An interrupted backup cannot masquerade as complete.
    if bucket
        .put(manifest_key(&manifest.backup_id), bytes)
        .sha256(checksum.to_vec())
        .execute()
        .await?
        .is_none()
    {
        return Err(Failure::conflict("recovery manifest was not stored"));
    }
    let (_, stored_hash) = load_manifest(bucket, &manifest.backup_id).await?;
    if stored_hash != sha256 {
        return Err(Failure::conflict(
            "recovery manifest readback checksum mismatch",
        ));
    }
    Ok(sha256)
}

async fn restore(
    db: &Database,
    env: &Env,
    actual: &str,
    request: &RestoreRequest,
) -> AppResult<Value> {
    if request.target != actual || !actual.starts_with("restore-") || !is_target(actual) {
        return Err(Failure::forbidden(
            "portable restore requires an explicit separate restore-* object",
        ));
    }
    ensure_empty(db)?;
    let bucket = env.bucket("RECOVERY")?;
    let (manifest, manifest_hash) = load_manifest(&bucket, &request.backup_id).await?;
    if manifest.source_instance == actual
        || manifest_hash != request.manifest_sha256
        || manifest.bundle_sha256 != request.bundle_sha256
        || manifest.fingerprint.sha256 != request.fingerprint_sha256
        || manifest.fingerprint.account_ids_sha256 != request.account_ids_sha256
        || manifest.fingerprint.account_count != request.account_count
    {
        return Err(Failure::conflict(
            "restore receipt or destination does not match the selected backup",
        ));
    }
    let mut bytes = Vec::with_capacity(manifest.bytes);
    for chunk in &manifest.chunks {
        let data = r2_bytes(
            &bucket,
            &chunk_key(&manifest.backup_id, chunk.index),
            CHUNK_BYTES,
        )
        .await?;
        if data.len() != chunk.bytes || digest(&data) != chunk.sha256 {
            return Err(Failure::conflict("restore chunk is corrupt or incomplete"));
        }
        bytes.extend_from_slice(&data);
    }
    if bytes.len() != manifest.bytes || digest(&bytes) != manifest.bundle_sha256 {
        return Err(Failure::conflict("restored bundle checksum mismatch"));
    }
    let bundle: Bundle = parse(&bytes)?;
    drop(bytes);
    if bundle.source.engine != "cloudflare-sqlite"
        || bundle.source.instance != manifest.source_instance
        || bundle.source.schema_version != manifest.fingerprint.storage_schema_version
        || bundle.fingerprint.as_deref() != Some(manifest.fingerprint.sha256.as_str())
    {
        return Err(Failure::conflict(
            "restored bundle is routed to the wrong source manifest",
        ));
    }
    import(
        db,
        &bundle,
        &manifest.bundle_sha256,
        actual,
        request.account_count,
        &request.account_ids_sha256,
    )
}

async fn all_manifests(bucket: &Bucket) -> AppResult<Vec<(Manifest, String)>> {
    let mut manifests = Vec::new();
    let mut cursor = None;
    loop {
        let mut list = bucket.list().prefix(MANIFEST_PREFIX).limit(100);
        if let Some(value) = cursor {
            list = list.cursor(value);
        }
        let page = list.execute().await?;
        for object in page.objects() {
            if manifests.len() >= MAX_BACKUPS {
                return Err(Failure::new(
                    413,
                    "recovery manifest inventory exceeds the explicit bound",
                ));
            }
            let key = object.key();
            let id = key
                .strip_prefix(MANIFEST_PREFIX)
                .and_then(|name| name.strip_suffix(".json"))
                .ok_or_else(|| Failure::conflict("unknown recovery manifest object"))?;
            manifests.push(load_manifest(bucket, id).await?);
        }
        if !page.truncated() {
            break;
        }
        cursor = Some(
            page.cursor()
                .ok_or_else(|| Failure::conflict("R2 manifest listing is incomplete"))?,
        );
    }
    manifests.sort_by(|left, right| {
        right
            .0
            .created_at_ms
            .cmp(&left.0.created_at_ms)
            .then_with(|| right.0.backup_id.cmp(&left.0.backup_id))
    });
    Ok(manifests)
}

async fn retention(env: &Env, days: u32, keep: usize) -> AppResult<Value> {
    if !(7..=365).contains(&days) || !(2..=MAX_BACKUPS).contains(&keep) {
        return Err(Failure::bad_request(
            "retention requires 7..365 days and 2..512 retained complete backups",
        ));
    }
    let bucket = env.bucket("RECOVERY")?;
    let manifests = all_manifests(&bucket).await?;
    let complete_ids: BTreeSet<&str> = manifests
        .iter()
        .map(|(manifest, _)| manifest.backup_id.as_str())
        .collect();
    let mut per_source = BTreeMap::<&str, usize>::new();
    let threshold = now_ms() - i64::from(days) * DAY_MS;
    let mut expired = Vec::new();
    for (manifest, _) in &manifests {
        let position = per_source.entry(&manifest.source_instance).or_default();
        *position += 1;
        if *position > keep && manifest.created_at_ms < threshold {
            expired.push(manifest);
        }
    }
    // Inventory before deleting. Incomplete uploads get a seven-day grace; a
    // retention failure never quietly claims an incomplete listing was pruned.
    let orphan_keys = orphaned_chunk_keys(&bucket, &complete_ids).await?;
    // Remove all selected complete markers before any chunks. Batch R2 deletes
    // so the explicit 512-manifest bound also stays within subrequest limits.
    let expired_markers: Vec<_> = expired
        .iter()
        .map(|manifest| manifest_key(&manifest.backup_id))
        .collect();
    for batch in expired_markers.chunks(1000) {
        bucket.delete_multiple(batch.to_vec()).await?;
    }
    let expired_chunks: Vec<_> = expired
        .iter()
        .flat_map(|manifest| {
            manifest
                .chunks
                .iter()
                .map(|chunk| chunk_key(&manifest.backup_id, chunk.index))
        })
        .collect();
    for batch in expired_chunks.chunks(1000) {
        bucket.delete_multiple(batch.to_vec()).await?;
    }
    for batch in orphan_keys.chunks(1000) {
        bucket.delete_multiple(batch.to_vec()).await?;
    }
    Ok(
        json!({"schema": "memory_engine.cloudflare_retention_receipt.v1", "deletedBackups": expired.len(),
        "deletedOrphanChunks": orphan_keys.len(), "retainedBackups": manifests.len() - expired.len(), "days": days, "keep": keep}),
    )
}

async fn orphaned_chunk_keys(
    bucket: &Bucket,
    complete_ids: &BTreeSet<&str>,
) -> AppResult<Vec<String>> {
    let mut orphan_keys = Vec::new();
    let mut cursor = None;
    let mut objects_seen = 0;
    loop {
        let mut list = bucket.list().prefix(CHUNK_PREFIX).limit(1000);
        if let Some(value) = cursor {
            list = list.cursor(value);
        }
        let page = list.execute().await?;
        for object in page.objects() {
            objects_seen += 1;
            if objects_seen > MAX_R2_KEYS {
                return Err(Failure::new(
                    413,
                    "recovery object inventory exceeds the explicit bound",
                ));
            }
            let key = object.key();
            let suffix = key
                .strip_prefix(CHUNK_PREFIX)
                .ok_or_else(|| Failure::conflict("invalid recovery chunk prefix"))?;
            let (id, index) = suffix
                .split_once('/')
                .ok_or_else(|| Failure::conflict("unknown recovery chunk object"))?;
            backup_id(id)?;
            if index.len() != 4 || !index.bytes().all(|byte| byte.is_ascii_digit()) {
                return Err(Failure::conflict("unknown recovery chunk object"));
            }
            if !complete_ids.contains(id) {
                let cutoff = u64::try_from(now_ms() - 7 * DAY_MS).map_err(|_| {
                    Failure::internal("recovery retention cutoff precedes the Unix epoch")
                })?;
                if object.uploaded().as_millis() < cutoff {
                    orphan_keys.push(key);
                }
            }
        }
        if !page.truncated() {
            break;
        }
        cursor = Some(
            page.cursor()
                .ok_or_else(|| Failure::conflict("R2 chunk listing is incomplete"))?,
        );
    }
    Ok(orphan_keys)
}

/// Called only for the primary application's private alarm by the DO owner.
pub async fn run_due(db: &Database, env: &Env) -> AppResult<()> {
    if next_run_at(db)?.is_some_and(|next| next <= now_ms()) {
        if count(db, "memory_engine_accounts")? == 0 {
            db.execute(
                "UPDATE memory_engine_recovery_state SET next_backup_at_ms = ? WHERE singleton = 1",
                &[json!(now_ms() + DAY_MS)],
            )?;
        } else {
            backup(db, env, "app").await?;
            retention(env, 30, 7).await?;
        }
    }
    Ok(())
}

pub fn next_run_at(db: &Database) -> AppResult<Option<i64>> {
    let rows = db.query::<Value>("SELECT MAX(next_backup_at_ms, COALESCE(lease_expires_at_ms, 0)) AS next FROM memory_engine_recovery_state WHERE singleton = 1", &[])?;
    Ok(rows.first().and_then(|row| row["next"].as_i64()))
}

/// All recovery routes authenticate before reading a body or touching SQLite/R2.
pub async fn handle(req: &mut Request, db: &Database, env: &Env) -> AppResult<Option<Response>> {
    let path = req.path();
    if !path.starts_with("/internal/migration/") && !path.starts_with("/internal/recovery/") {
        return Ok(None);
    }
    auth::require_admin(req, env)?;
    let actual = object_name(req)?;
    let response = match (req.method(), path.as_str()) {
        (Method::Get, "/internal/migration/fingerprint") => {
            let (_, fingerprint) = snapshot(db, &actual)?;
            response(&fingerprint)?
        }
        (Method::Get, "/internal/migration/export") => {
            let (bundle, _) = snapshot(db, &actual)?;
            bytes_response(encode_bundle(&bundle)?)?
        }
        (Method::Post, "/internal/migration/import") => import_request(req, db, &actual).await?,
        (Method::Post, "/internal/recovery/backup") => {
            confirm_target(req, &actual)?;
            response(&backup(db, env, &actual).await?)?
        }
        (Method::Get, "/internal/recovery/list") => {
            let bucket = env.bucket("RECOVERY")?;
            let manifests = all_manifests(&bucket).await?;
            response(
                &json!({"schema": "memory_engine.cloudflare_backup_list.v1", "backups": manifests.iter().map(|(manifest, hash)| backup_receipt(manifest, hash)).collect::<Vec<_>>()}),
            )?
        }
        (Method::Get, "/internal/recovery/manifest" | "/internal/recovery/chunk") => {
            recovery_object(req, env, &path).await?
        }
        (Method::Post, "/internal/recovery/restore") => {
            confirm_target(req, &actual)?;
            let body: RestoreRequest = parse(&bounded_body(req, 16 * 1024).await?)?;
            response(&restore(db, env, &actual, &body).await?)?
        }
        (Method::Post, "/internal/recovery/retention") => {
            confirm_target(req, &actual)?;
            let body: RetentionRequest = parse(&bounded_body(req, 4096).await?)?;
            if actual != "app" || body.target != actual || !body.confirm {
                return Err(Failure::forbidden(
                    "retention requires explicit primary-object confirmation",
                ));
            }
            response(&retention(env, body.days, body.keep).await?)?
        }
        _ => return Err(Failure::not_found("unknown recovery route or method")),
    };
    Ok(Some(response))
}

async fn import_request(req: &mut Request, db: &Database, actual: &str) -> AppResult<Response> {
    confirm_target(req, actual)?;
    let expected_sha = req
        .headers()
        .get("x-scry-content-sha256")?
        .filter(|value| is_hash(value))
        .ok_or_else(|| Failure::bad_request("expected bundle checksum required"))?;
    let expected_ids = req
        .headers()
        .get("x-scry-account-ids-sha256")?
        .ok_or_else(|| Failure::bad_request("expected account fingerprint required"))?;
    let expected_count: usize = req
        .headers()
        .get("x-scry-account-count")?
        .ok_or_else(|| Failure::bad_request("expected account count required"))?
        .parse()
        .map_err(|_| Failure::bad_request("invalid expected account count"))?;
    let bytes = bounded_body(req, MAX_BUNDLE_BYTES).await?;
    if digest(&bytes) != expected_sha {
        return Err(Failure::bad_request("bundle transport checksum mismatch"));
    }
    let bundle: Bundle = parse(&bytes)?;
    drop(bytes);
    if bundle.source.engine == "cloudflare-sqlite"
        && (!actual.starts_with("restore-") || bundle.source.instance == actual)
    {
        return Err(Failure::forbidden(
            "SQLite import requires a separate empty restore-* object",
        ));
    }
    if bundle.source.engine == "postgresql"
        && actual == "app"
        && req.headers().get("x-scry-source-quiesced")?.as_deref() != Some("true")
    {
        return Err(Failure::conflict(
            "primary PostgreSQL cutover requires an explicit stopped-writer barrier",
        ));
    }
    response(&import(
        db,
        &bundle,
        &expected_sha,
        actual,
        expected_count,
        &expected_ids,
    )?)
}

async fn recovery_object(req: &Request, env: &Env, path: &str) -> AppResult<Response> {
    let url = req.url()?;
    let query: BTreeMap<_, _> = url
        .query_pairs()
        .map(|(key, value)| (key.into_owned(), value.into_owned()))
        .collect();
    let id = query
        .get("backupId")
        .ok_or_else(|| Failure::bad_request("backupId required"))?;
    let bucket = env.bucket("RECOVERY")?;
    let (manifest, _) = load_manifest(&bucket, id).await?;
    if path.ends_with("/manifest") {
        bytes_response(serde_json::to_vec(&manifest)?)
    } else {
        let index: usize = query
            .get("index")
            .ok_or_else(|| Failure::bad_request("chunk index required"))?
            .parse()
            .map_err(|_| Failure::bad_request("invalid chunk index"))?;
        let chunk = manifest
            .chunks
            .get(index)
            .ok_or_else(|| Failure::not_found("chunk index is outside the complete manifest"))?;
        let bytes = r2_bytes(&bucket, &chunk_key(id, index), CHUNK_BYTES).await?;
        if bytes.len() != chunk.bytes || digest(&bytes) != chunk.sha256 {
            return Err(Failure::conflict("recovery chunk checksum mismatch"));
        }
        let mut response = bytes_response(bytes)?;
        response
            .headers_mut()
            .set("content-type", "application/octet-stream")?;
        Ok(response)
    }
}
