//! Synchronous, transaction-owning Durable Object `SQLite` boundary.

use serde::{de::DeserializeOwned, Deserialize};
use serde_json::Value;
use worker::{
    js_sys::{Function, Reflect},
    wasm_bindgen::{closure::ScopedClosure, JsCast, JsValue},
    SqlStorage, SqlStorageValue,
};

use crate::{AppResult, Failure};

// Exclude only platform-owned tables: Cloudflare KV/alarm metadata and
// Miniflare's named object identity. Other tables must match the application schema.
pub(crate) const APPLICATION_TABLES_SQL: &str = "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT GLOB 'sqlite_*' AND name NOT IN ('__cf_kv', '_cf_METADATA', '__miniflare_do_name') ORDER BY name";

#[derive(Clone)]
pub struct Database {
    sql: SqlStorage,
    storage: JsValue,
}

impl Database {
    pub fn new(sql: SqlStorage, storage: JsValue) -> Self {
        Self { sql, storage }
    }

    pub fn query<T: DeserializeOwned>(&self, sql: &str, params: &[Value]) -> AppResult<Vec<T>> {
        let cursor = self.sql.exec(sql, bindings(params)?)?;
        // Never let a cursor escape to an await: even read-only cursors lose their
        // snapshot across an event-loop turn in workerd.
        cursor
            .next::<T>()
            .map(|row| row.map_err(Into::into))
            .collect()
    }

    pub fn execute(&self, sql: &str, params: &[Value]) -> AppResult<()> {
        let cursor = self.sql.exec(sql, bindings(params)?)?;
        // Fully step RETURNING and multi-statement cursors before reporting success.
        for row in cursor.raw() {
            row?;
        }
        Ok(())
    }

    pub fn transaction<T>(&self, operation: impl FnOnce() -> AppResult<T>) -> AppResult<T> {
        let transaction = Reflect::get(&self.storage, &JsValue::from_str("transactionSync"))
            .map_err(|_| Failure::internal("SQLite transaction API unavailable"))?
            .dyn_into::<Function>()
            .map_err(|_| Failure::internal("SQLite transaction API unavailable"))?;
        let mut operation = Some(operation);
        let mut result = None;
        let invocation = {
            let mut callback = || -> Result<(), JsValue> {
                result = Some(match operation.take() {
                    Some(operation) => operation(),
                    None => Err(Failure::internal("SQLite transaction callback repeated")),
                });
                if result.as_ref().is_some_and(Result::is_ok) {
                    Ok(())
                } else {
                    // Returning Err makes wasm-bindgen throw only after Rust locals
                    // are dropped. transactionSync sees a throw and rolls back; the
                    // original classified Rust error stays outside the JS boundary.
                    Err(JsValue::from_str("Scry transaction aborted"))
                }
            };
            // The scope is synchronous, cannot escape, and is invalidated before
            // captured stack state is read again. A panic also throws/rolls back;
            // interrupted operation state is never reused by this boundary.
            let closure: ScopedClosure<'_, dyn FnMut() -> Result<(), JsValue>> =
                ScopedClosure::borrow_mut_assert_unwind_safe(&mut callback);
            transaction.call1(&self.storage, closure.as_js_value())
        };
        match (invocation, result) {
            (_, Some(Err(error))) => Err(error),
            (Ok(_), Some(Ok(value))) => Ok(value),
            (Err(_), _) => Err(Failure::internal("SQLite transaction failed")),
            (Ok(_), None) => Err(Failure::internal("SQLite transaction callback not invoked")),
        }
    }
}

fn bindings(params: &[Value]) -> AppResult<Vec<SqlStorageValue>> {
    params
        .iter()
        .map(|value| match value {
            Value::Null => Ok(SqlStorageValue::Null),
            Value::Bool(value) => Ok(SqlStorageValue::Integer(i64::from(*value))),
            Value::Number(value) => {
                if let Some(value) = value.as_i64() {
                    SqlStorageValue::try_from_i64(value)
                        .map_err(|_| Failure::bad_request("SQL integer exceeds exact range"))
                } else if value.is_u64() {
                    Err(Failure::bad_request("SQL integer exceeds exact range"))
                } else {
                    value
                        .as_f64()
                        .filter(|value| value.is_finite())
                        .map(SqlStorageValue::Float)
                        .ok_or_else(|| Failure::bad_request("Invalid SQL number"))
                }
            }
            Value::String(value) => Ok(SqlStorageValue::String(value.clone())),
            Value::Array(_) | Value::Object(_) => {
                Ok(SqlStorageValue::String(serde_json::to_string(value)?))
            }
        })
        .collect()
}

const MIGRATIONS: &[(i64, &str, &str)] = &[
    (1, "001-auth", include_str!("../migrations/001-auth.sql")),
    (2, "002-study", include_str!("../migrations/002-study.sql")),
    (3, "003-jobs", include_str!("../migrations/003-jobs.sql")),
    (
        4,
        "004-recovery",
        include_str!("../migrations/004-recovery.sql"),
    ),
];

#[derive(Deserialize)]
struct Migration {
    version: i64,
    name: String,
}

#[derive(Deserialize)]
struct TableName {
    name: String,
}

/// Apply a forward-only, contiguous ledger. An unversioned application schema
/// is not a fresh database and is never silently adopted.
pub fn initialize(db: &Database) -> AppResult<()> {
    db.transaction(|| {
        let tables: Vec<TableName> = db.query(
            APPLICATION_TABLES_SQL,
            &[],
        )?;
        if !tables.iter().any(|table| table.name == "memory_engine_schema_migrations") {
            if !tables.is_empty() {
                return Err(Failure::internal("Unversioned SQLite schema"));
            }
            db.execute(
                "CREATE TABLE memory_engine_schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, applied_at_ms INTEGER NOT NULL)",
                &[],
            )?;
        }
        let ledger: Vec<Migration> = db.query(
            "SELECT version, name FROM memory_engine_schema_migrations ORDER BY version",
            &[],
        )?;
        if ledger.len() > MIGRATIONS.len()
            || ledger.iter().zip(MIGRATIONS).any(|(applied, expected)| {
                applied.version != expected.0 || applied.name != expected.1
            })
        {
            return Err(Failure::internal("Unknown or noncontiguous SQLite schema version"));
        }
        for (version, name, sql) in &MIGRATIONS[ledger.len()..] {
            db.execute(sql, &[])?;
            db.execute(
                "INSERT INTO memory_engine_schema_migrations (version, name, applied_at_ms) VALUES (?, ?, ?)",
                &[Value::from(*version), Value::from(*name), Value::from(crate::now_ms())],
            )?;
        }
        Ok(())
    })
}
