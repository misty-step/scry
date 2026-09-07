-- PostgreSQL schema8 auth columns retain their names, units, and hash meanings.
CREATE TABLE memory_engine_accounts (
    account_id TEXT PRIMARY KEY,
    created_at_ms INTEGER NOT NULL,
    active_graded_review TEXT CHECK (active_graded_review IS NULL OR json_valid(active_graded_review))
);

CREATE TABLE memory_engine_rate_limits (
    rate_limit_key TEXT PRIMARY KEY,
    window_start_ms INTEGER NOT NULL,
    attempt_count INTEGER NOT NULL CHECK (attempt_count >= 0)
);
CREATE INDEX memory_engine_rate_limits_window_idx ON memory_engine_rate_limits(window_start_ms);

CREATE TABLE memory_engine_auth_challenges (
    challenge_hash TEXT PRIMARY KEY,
    email_normalized TEXT NOT NULL,
    expires_at_ms INTEGER NOT NULL,
    consumed_at_ms INTEGER
);
CREATE INDEX memory_engine_auth_challenges_email_idx
    ON memory_engine_auth_challenges(email_normalized, consumed_at_ms);

CREATE TABLE memory_engine_api_sessions (
    session_token_hash TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    created_at_ms INTEGER NOT NULL,
    expires_at_ms INTEGER NOT NULL,
    revoked_at_ms INTEGER
);
CREATE INDEX memory_engine_api_sessions_account_idx
    ON memory_engine_api_sessions(account_id, expires_at_ms);

CREATE TABLE memory_engine_browser_sessions (
    session_id_hash TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    session_token_hash TEXT NOT NULL,
    csrf_token_hash TEXT NOT NULL,
    created_at_ms INTEGER NOT NULL,
    expires_at_ms INTEGER NOT NULL,
    revoked_at_ms INTEGER
);
CREATE INDEX memory_engine_browser_sessions_account_idx
    ON memory_engine_browser_sessions(account_id, expires_at_ms);
CREATE INDEX memory_engine_browser_sessions_token_idx
    ON memory_engine_browser_sessions(account_id, session_token_hash, revoked_at_ms);

CREATE TABLE memory_engine_return_notification_preferences (
    account_id TEXT PRIMARY KEY REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    email_normalized TEXT NOT NULL,
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    -- Historical name: transport acceptance/completion, NOT mailbox delivery or readback.
    last_sent_at_ms INTEGER,
    unsubscribe_nonce TEXT NOT NULL DEFAULT '',
    updated_at_ms INTEGER NOT NULL,
    claim_id TEXT,
    claim_expires_at_ms INTEGER,
    pending_delivery_key TEXT,
    pending_due_count INTEGER,
    pending_unsubscribe_expires_at_ms INTEGER,
    retry_attempts INTEGER NOT NULL DEFAULT 0,
    next_retry_at_ms INTEGER
);
CREATE TABLE memory_engine_return_notification_schedule (
    account_id TEXT PRIMARY KEY REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    next_check_at_ms INTEGER NOT NULL
);

CREATE TABLE memory_engine_waitlist_entries (
    email_normalized TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    created_at_ms INTEGER NOT NULL,
    updated_at_ms INTEGER NOT NULL,
    invited_at_ms INTEGER
);
CREATE TABLE memory_engine_waitlist_audit_log (
    audit_id INTEGER PRIMARY KEY AUTOINCREMENT,
    email_normalized TEXT NOT NULL,
    event TEXT NOT NULL CHECK (event IN ('joined', 'invited', 'deleted')),
    occurred_at_ms INTEGER NOT NULL
);
CREATE INDEX memory_engine_waitlist_audit_log_email_idx
    ON memory_engine_waitlist_audit_log(email_normalized, occurred_at_ms);

-- No production body, link, raw auth credential, or recipient is persisted here.
-- A send interrupted before its acceptance receipt is uncertain, never blindly replayed.
CREATE TABLE memory_engine_mail_receipts (
    delivery_key TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('magic_link', 'due_count')),
    recipient_hash TEXT NOT NULL,
    -- Empty only for a proven pre-send configuration failure (state = prepared).
    payload_hash TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('prepared', 'sending', 'accepted', 'rejected', 'unknown', 'local_outbox')),
    message_id TEXT,
    error_code TEXT,
    attempt_count INTEGER NOT NULL,
    started_at_ms INTEGER NOT NULL,
    updated_at_ms INTEGER NOT NULL,
    accepted_at_ms INTEGER,
    reconciled_at_ms INTEGER,
    reconciliation_evidence_sha256 TEXT
);
CREATE INDEX memory_engine_mail_receipts_updated_idx
    ON memory_engine_mail_receipts(updated_at_ms, delivery_key);
CREATE INDEX memory_engine_mail_receipts_recipient_idx
    ON memory_engine_mail_receipts(recipient_hash, kind, started_at_ms);

-- Explicit local-outbox mode only. The HTTP readout is local-mode AND admin gated.
CREATE TABLE memory_engine_mail_outbox (
    delivery_key TEXT PRIMARY KEY REFERENCES memory_engine_mail_receipts(delivery_key) ON DELETE CASCADE,
    payload TEXT NOT NULL CHECK (json_valid(payload)),
    created_at_ms INTEGER NOT NULL
);
