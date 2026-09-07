-- Recovery control is object-local, never restored over an object's identity or alarm.
CREATE TABLE memory_engine_recovery_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    learner_traffic_enabled INTEGER NOT NULL DEFAULT 0 CHECK (learner_traffic_enabled IN (0, 1)),
    next_backup_at_ms INTEGER NOT NULL,
    lease_token TEXT,
    lease_expires_at_ms INTEGER,
    last_backup_id TEXT,
    last_backup_at_ms INTEGER
);
INSERT INTO memory_engine_recovery_state (singleton, next_backup_at_ms) VALUES (1, 0);

-- Original migration ledger, sequence high-water marks, and table receipts survive
-- the PostgreSQL cutover without being confused with the SQLite migration ledger.
-- This is provenance, not duplicated learner payload or an inferred history.
CREATE TABLE memory_engine_recovery_imports (
    import_id TEXT PRIMARY KEY,
    source_sha256 TEXT NOT NULL CHECK (length(source_sha256) = 64),
    source_metadata TEXT NOT NULL CHECK (json_valid(source_metadata)),
    source_tables TEXT NOT NULL CHECK (json_valid(source_tables)),
    destination_sha256 TEXT NOT NULL CHECK (length(destination_sha256) = 64),
    imported_at_ms INTEGER NOT NULL,
    target TEXT NOT NULL
);
