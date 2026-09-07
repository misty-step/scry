-- Keep PostgreSQL's private routing/attempt ledger intact for lossless import.
-- Cost columns are accounted micro-USD, not a claim that unknown cost was zero.
CREATE TABLE memory_engine_generation_jobs (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    job_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'retry', 'succeeded', 'failed')),
    card_count INTEGER NOT NULL DEFAULT 0 CHECK (card_count >= 0),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    error TEXT,
    model_key TEXT NOT NULL,
    cost_usd_micros INTEGER NOT NULL DEFAULT 0 CHECK (cost_usd_micros >= 0),
    legacy_cost_usd_micros INTEGER NOT NULL DEFAULT 0 CHECK (legacy_cost_usd_micros >= 0),
    created_at_ms INTEGER NOT NULL,
    updated_at_ms INTEGER NOT NULL,
    retry_at_ms INTEGER,
    lease_owner TEXT,
    lease_expires_at_ms INTEGER,
    lease_token TEXT,
    reserved_cost_usd_micros INTEGER NOT NULL DEFAULT 0 CHECK (reserved_cost_usd_micros >= 0),
    source_fingerprint TEXT,
    retryable INTEGER NOT NULL DEFAULT 1 CHECK (retryable IN (0, 1)),
    PRIMARY KEY (account_id, job_id)
);
CREATE INDEX memory_engine_generation_jobs_queue_idx
    ON memory_engine_generation_jobs(status, retry_at_ms, created_at_ms);
CREATE INDEX memory_engine_generation_jobs_account_idx
    ON memory_engine_generation_jobs(account_id, created_at_ms DESC);
CREATE UNIQUE INDEX memory_engine_generation_jobs_active_source_idx
    ON memory_engine_generation_jobs(account_id, source_id)
    WHERE status IN ('queued', 'running', 'retry');

CREATE TABLE memory_engine_generation_job_attempts (
    account_id TEXT NOT NULL,
    job_id TEXT NOT NULL,
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    lease_token TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'stale')),
    generation_run_id TEXT,
    reservation_cost_usd_micros INTEGER NOT NULL DEFAULT 0 CHECK (reservation_cost_usd_micros >= 0),
    reserved_cost_usd_micros INTEGER NOT NULL DEFAULT 0 CHECK (reserved_cost_usd_micros >= 0),
    cost_usd_micros INTEGER NOT NULL DEFAULT 0 CHECK (cost_usd_micros >= 0),
    error TEXT,
    started_at_ms INTEGER NOT NULL,
    completed_at_ms INTEGER,
    updated_at_ms INTEGER NOT NULL,
    usage_json TEXT CHECK (usage_json IS NULL OR json_valid(usage_json)),
    cost_unknown INTEGER NOT NULL DEFAULT 0 CHECK (cost_unknown IN (0, 1)),
    PRIMARY KEY (account_id, job_id, attempt),
    UNIQUE (account_id, job_id, lease_token),
    FOREIGN KEY (account_id, job_id) REFERENCES memory_engine_generation_jobs(account_id, job_id) ON DELETE CASCADE
);
CREATE INDEX memory_engine_generation_job_attempts_budget_idx
    ON memory_engine_generation_job_attempts(account_id, started_at_ms);
CREATE INDEX memory_engine_generation_job_attempts_global_budget_idx
    ON memory_engine_generation_job_attempts(started_at_ms);

-- A monotonic revision prevents an ABA permission change (revoke, reallow)
-- from restoring an old request's right to publish. Deleted ids retain a tombstone.
CREATE TABLE memory_engine_source_revisions (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    source_id TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    PRIMARY KEY (account_id, source_id)
);
INSERT INTO memory_engine_source_revisions (account_id, source_id, revision)
    SELECT account_id, source_document_id, 1 FROM memory_engine_source_documents;
CREATE TRIGGER memory_engine_source_revision_insert AFTER INSERT ON memory_engine_source_documents BEGIN
    INSERT INTO memory_engine_source_revisions (account_id, source_id, revision)
    VALUES (NEW.account_id, NEW.source_document_id, 1)
    ON CONFLICT (account_id, source_id) DO UPDATE SET revision = revision + 1;
END;
CREATE TRIGGER memory_engine_source_revision_update AFTER UPDATE OF document ON memory_engine_source_documents
WHEN NEW.document IS NOT OLD.document BEGIN
    UPDATE memory_engine_source_revisions SET revision = revision + 1
    WHERE account_id = NEW.account_id AND source_id = NEW.source_document_id;
END;
CREATE TRIGGER memory_engine_source_revision_delete AFTER DELETE ON memory_engine_source_documents BEGIN
    UPDATE memory_engine_source_revisions SET revision = revision + 1
    WHERE account_id = OLD.account_id AND source_id = OLD.source_document_id;
END;

-- Interactive reference/Bridge calls share admission and uncertain-cost bounds
-- with queued generation. A Durable Object alarm settles abandoned reservations.
CREATE TABLE memory_engine_model_operations (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    operation_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('reference', 'bridge')),
    review_unit_id TEXT NOT NULL,
    model_key TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'stale')),
    reservation_cost_usd_micros INTEGER NOT NULL CHECK (reservation_cost_usd_micros > 0),
    reserved_cost_usd_micros INTEGER NOT NULL CHECK (reserved_cost_usd_micros >= 0),
    cost_usd_micros INTEGER NOT NULL DEFAULT 0 CHECK (cost_usd_micros >= 0),
    cost_unknown INTEGER NOT NULL DEFAULT 0 CHECK (cost_unknown IN (0, 1)),
    usage_json TEXT CHECK (usage_json IS NULL OR json_valid(usage_json)),
    error TEXT,
    started_at_ms INTEGER NOT NULL,
    completed_at_ms INTEGER,
    lease_expires_at_ms INTEGER NOT NULL,
    PRIMARY KEY (account_id, operation_id)
);
CREATE INDEX memory_engine_model_operations_budget_idx ON memory_engine_model_operations(account_id, started_at_ms);
CREATE INDEX memory_engine_model_operations_expiry_idx ON memory_engine_model_operations(status, lease_expires_at_ms);
CREATE UNIQUE INDEX memory_engine_model_operations_active_account_idx
    ON memory_engine_model_operations(account_id) WHERE status = 'running';
