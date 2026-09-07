-- Keep the PostgreSQL study row envelope intact for lossless raw migration.
-- JSON payloads are authoritative; indexes never replace historical evidence.
CREATE TABLE memory_engine_source_documents (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    source_document_id TEXT NOT NULL,
    document TEXT NOT NULL CHECK (json_valid(document)),
    created_at_ms INTEGER NOT NULL,
    PRIMARY KEY (account_id, source_document_id),
    CHECK (json_extract(document, '$.id') IS source_document_id)
);
CREATE TABLE memory_engine_reference_spans (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    reference_span_id TEXT NOT NULL,
    source_document_id TEXT NOT NULL,
    span TEXT NOT NULL CHECK (json_valid(span)),
    created_at_ms INTEGER NOT NULL,
    PRIMARY KEY (account_id, reference_span_id),
    FOREIGN KEY (account_id, source_document_id)
        REFERENCES memory_engine_source_documents(account_id, source_document_id) ON DELETE CASCADE,
    CHECK (json_extract(span, '$.id') IS reference_span_id),
    CHECK (json_extract(span, '$.sourceDocumentId') IS source_document_id)
);
CREATE TABLE memory_engine_generation_runs (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    generation_run_id TEXT NOT NULL,
    run TEXT NOT NULL CHECK (json_valid(run)),
    started_at_ms INTEGER NOT NULL,
    PRIMARY KEY (account_id, generation_run_id),
    CHECK (json_extract(run, '$.id') IS generation_run_id)
);
CREATE TABLE memory_engine_concept_reference_notes (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    concept_key TEXT NOT NULL,
    note TEXT NOT NULL CHECK (json_valid(note)),
    updated_at_ms INTEGER NOT NULL,
    PRIMARY KEY (account_id, concept_key),
    CHECK (json_extract(note, '$.conceptKey') IS concept_key)
);
CREATE TABLE memory_engine_generated_prompt_drafts (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    draft_id TEXT NOT NULL,
    review_unit_id TEXT NOT NULL,
    draft TEXT NOT NULL CHECK (json_valid(draft)),
    created_at_ms INTEGER NOT NULL,
    PRIMARY KEY (account_id, draft_id),
    CHECK (json_extract(draft, '$.id') IS draft_id),
    CHECK (json_extract(draft, '$.reviewUnitId') IS review_unit_id)
);
CREATE INDEX memory_engine_drafts_run_idx
    ON memory_engine_generated_prompt_drafts(account_id, json_extract(draft, '$.generationRunId'));
CREATE INDEX memory_engine_drafts_review_idx
    ON memory_engine_generated_prompt_drafts(account_id, review_unit_id);
CREATE TABLE memory_engine_review_units (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    review_unit_id TEXT NOT NULL,
    record TEXT NOT NULL CHECK (json_valid(record)),
    created_at_ms INTEGER NOT NULL,
    archived_at_ms INTEGER,
    PRIMARY KEY (account_id, review_unit_id),
    CHECK (json_extract(record, '$.reviewUnitId') IS review_unit_id),
    CHECK (json_extract(record, '$.archivedAt') IS archived_at_ms)
);
CREATE INDEX memory_engine_review_units_active_idx
    ON memory_engine_review_units(account_id, archived_at_ms, created_at_ms, review_unit_id);
CREATE INDEX memory_engine_review_units_draft_idx
    ON memory_engine_review_units(account_id, json_extract(record, '$.generatedPromptDraftId'));
CREATE TABLE memory_engine_schedules (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    review_unit_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK (json_valid(state)),
    updated_at_ms INTEGER NOT NULL,
    PRIMARY KEY (account_id, review_unit_id),
    FOREIGN KEY (account_id, review_unit_id)
        REFERENCES memory_engine_review_units(account_id, review_unit_id) ON DELETE CASCADE
);
CREATE TABLE memory_engine_attempts (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    attempt_id INTEGER PRIMARY KEY AUTOINCREMENT,
    review_unit_id TEXT NOT NULL,
    prompt_id TEXT,
    idempotency_key TEXT,
    attempt TEXT NOT NULL CHECK (json_valid(attempt)),
    occurred_at_ms INTEGER NOT NULL,
    FOREIGN KEY (account_id, review_unit_id)
        REFERENCES memory_engine_review_units(account_id, review_unit_id) ON DELETE CASCADE,
    CHECK (json_extract(attempt, '$.reviewUnitId') IS review_unit_id),
    CHECK (json_extract(attempt, '$.promptId') IS prompt_id),
    CHECK (json_extract(attempt, '$.idempotencyKey') IS idempotency_key)
);
CREATE INDEX memory_engine_attempts_account_review_idx
    ON memory_engine_attempts(account_id, review_unit_id, occurred_at_ms);
CREATE TABLE memory_engine_applied_reviews (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    receipt_key TEXT NOT NULL,
    review_unit_id TEXT NOT NULL,
    attempt TEXT NOT NULL CHECK (json_valid(attempt)),
    expected_prior_schedule_state TEXT CHECK (expected_prior_schedule_state IS NULL OR json_valid(expected_prior_schedule_state)),
    schedule_state TEXT NOT NULL CHECK (json_valid(schedule_state)),
    applied_at_ms INTEGER NOT NULL,
    PRIMARY KEY (account_id, receipt_key),
    FOREIGN KEY (account_id, review_unit_id)
        REFERENCES memory_engine_review_units(account_id, review_unit_id) ON DELETE CASCADE,
    CHECK (json_extract(attempt, '$.reviewUnitId') IS review_unit_id)
);
CREATE INDEX memory_engine_applied_reviews_unit_idx
    ON memory_engine_applied_reviews(account_id, review_unit_id, applied_at_ms);
CREATE TABLE memory_engine_content_feedback (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    feedback_id TEXT NOT NULL,
    review_unit_id TEXT NOT NULL,
    feedback TEXT NOT NULL CHECK (json_valid(feedback)),
    occurred_at_ms INTEGER NOT NULL,
    PRIMARY KEY (account_id, feedback_id),
    FOREIGN KEY (account_id, review_unit_id)
        REFERENCES memory_engine_review_units(account_id, review_unit_id) ON DELETE CASCADE,
    CHECK (json_extract(feedback, '$.id') IS feedback_id),
    CHECK (json_extract(feedback, '$.accountId') IS account_id),
    CHECK (json_extract(feedback, '$.reviewUnitId') IS review_unit_id)
);
CREATE INDEX memory_engine_content_feedback_account_review_idx
    ON memory_engine_content_feedback(account_id, review_unit_id, occurred_at_ms);
CREATE INDEX memory_engine_content_feedback_parent_idx
    ON memory_engine_content_feedback(account_id, json_extract(feedback, '$.supersedesId'));
CREATE TABLE memory_engine_remediation_packs (
    account_id TEXT NOT NULL REFERENCES memory_engine_accounts(account_id) ON DELETE CASCADE,
    pack_id TEXT NOT NULL,
    pack TEXT NOT NULL CHECK (json_valid(pack)),
    attempt_id TEXT NOT NULL,
    created_at_ms INTEGER NOT NULL,
    PRIMARY KEY (account_id, pack_id),
    CHECK (json_extract(pack, '$.id') IS pack_id),
    CHECK (json_extract(pack, '$.attemptId') IS attempt_id)
);
CREATE INDEX memory_engine_remediation_packs_attempt_idx
    ON memory_engine_remediation_packs(account_id, attempt_id);
CREATE TABLE memory_engine_review_exposures (
    account_id TEXT NOT NULL,
    review_unit_id TEXT NOT NULL,
    occurrence_key TEXT NOT NULL,
    exposure TEXT NOT NULL CHECK (json_valid(exposure)),
    revealed_at_ms INTEGER NOT NULL,
    PRIMARY KEY (account_id, review_unit_id, occurrence_key),
    FOREIGN KEY (account_id, review_unit_id)
        REFERENCES memory_engine_review_units(account_id, review_unit_id) ON DELETE CASCADE,
    CHECK (json_extract(exposure, '$.reviewUnitId') IS review_unit_id)
);
