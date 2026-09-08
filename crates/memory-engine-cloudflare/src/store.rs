//! Account-scoped `SQLite` persistence for the shared learning and generation kernel.

use std::collections::BTreeSet;

use memory_engine_core::{
    defer_queue_availability, QueueCandidate, Rating, ReviewUnitId, ReviewUnitLifecycle,
    ScheduleState, Verdict,
};
use memory_engine_generation::BetaGenerationStore;
use memory_engine_persistence::{
    applied_review_key, assert_attempt_contract, assert_concept_reference_note_contract,
    assert_draft_contract, assert_non_blank, normalized_concept_key, promoted_review_unit,
    replace_prompt_answer, replace_prompt_text, review_occurrence_key, transition_learner_draft,
    AppliedReviewReceipt, BetaReviewUnitRecord, BetaStoreError, BetaStoreSnapshot,
    ConceptReferenceNote, GeneratedPromptDraft, GenerationRun, LearnerDraftDecisionInput,
    ReferenceSpan, RemediationPackRecord, ReviewExposure, SourceDocument, SourcePermission,
};
use memory_engine_service::{
    content_feedback_replay_matches, ContentFeedback, ContentFeedbackStore, MemoryServiceStore,
    ServiceAttemptRecord,
};
use memory_engine_study::BetaStudyStore;
use serde::{de::DeserializeOwned, Deserialize, Serialize};
use serde_json::{json, Value};

use crate::{database::Database, AppResult, Failure};

#[derive(Clone)]
pub struct SqlStudyStore {
    db: Database,
    account_id: String,
}

#[derive(Deserialize)]
struct JsonRow {
    value: String,
}

#[derive(Deserialize)]
struct Present {
    present: i64,
}

#[derive(Deserialize)]
struct QueueRow {
    queue: String,
    state: Option<String>,
    snoozed_until: Option<i64>,
}

#[derive(Deserialize)]
struct LeaseRow {
    job_id: String,
    reservation_cost_usd_micros: i64,
}

impl SqlStudyStore {
    pub fn new(db: Database, account_id: impl Into<String>) -> Self {
        Self {
            db,
            account_id: account_id.into(),
        }
    }

    fn write_transaction<T>(&self, operation: impl FnOnce() -> AppResult<T>) -> AppResult<T> {
        self.db.transaction(|| {
            nonblank(&self.account_id, "Account id")?;
            operation()
        })
    }

    fn json_rows<T: DeserializeOwned>(&self, sql: &str, params: &[Value]) -> AppResult<Vec<T>> {
        self.db
            .query::<JsonRow>(sql, params)?
            .into_iter()
            .map(|row| serde_json::from_str(&row.value).map_err(Into::into))
            .collect()
    }

    fn json_row<T: DeserializeOwned>(&self, sql: &str, params: &[Value]) -> AppResult<Option<T>> {
        let rows = self.json_rows(sql, params)?;
        if rows.len() > 1 {
            return Err(Failure::internal("Nonunique study record"));
        }
        Ok(rows.into_iter().next())
    }

    fn exists(&self, sql: &str, params: &[Value]) -> AppResult<bool> {
        Ok(self
            .db
            .query::<Present>(sql, params)?
            .iter()
            .any(|row| row.present != 0))
    }

    pub fn source_document(&self, id: &str) -> AppResult<Option<SourceDocument>> {
        self.json_row("SELECT document AS value FROM memory_engine_source_documents WHERE account_id = ? AND source_document_id = ?", &[json!(self.account_id), json!(id)])
    }

    pub fn source_documents(&self) -> AppResult<Vec<SourceDocument>> {
        self.json_rows("SELECT document AS value FROM memory_engine_source_documents WHERE account_id = ? ORDER BY created_at_ms, source_document_id", &[json!(self.account_id)])
    }

    fn require_source(&self, id: &str) -> AppResult<SourceDocument> {
        self.source_document(id)?
            .ok_or_else(|| Failure::not_found("Unknown source document"))
    }

    fn draft(&self, id: &str) -> AppResult<Option<GeneratedPromptDraft>> {
        self.json_row("SELECT draft AS value FROM memory_engine_generated_prompt_drafts WHERE account_id = ? AND draft_id = ?", &[json!(self.account_id), json!(id)])
    }

    fn require_draft(&self, id: &str) -> AppResult<GeneratedPromptDraft> {
        self.draft(id)?
            .ok_or_else(|| Failure::not_found("Unknown generated draft"))
    }

    fn review_unit(&self, id: &ReviewUnitId) -> AppResult<BetaReviewUnitRecord> {
        self.json_row("SELECT record AS value FROM memory_engine_review_units WHERE account_id = ? AND review_unit_id = ?", &[json!(self.account_id), json!(id.as_str())])?
            .ok_or_else(|| Failure::not_found("Unknown review unit"))
    }

    fn generation_run(&self, id: &str) -> AppResult<Option<GenerationRun>> {
        self.json_row("SELECT run AS value FROM memory_engine_generation_runs WHERE account_id = ? AND generation_run_id = ?", &[json!(self.account_id), json!(id)])
    }

    pub fn export_snapshot(&self) -> AppResult<BetaStoreSnapshot> {
        self.read_snapshot(false)
    }

    fn read_snapshot(&self, review_only: bool) -> AppResult<BetaStoreSnapshot> {
        self.db.transaction(|| {
            // One consistent snapshot without workerd's compound-SELECT limit,
            // discriminator strings, or a second account-wide row buffer.
            let params = [json!(self.account_id), json!(review_only)];
            let account = &params[..1];
            Ok(BetaStoreSnapshot {
                source_documents: self.json_rows(
                    "SELECT CASE WHEN ?2 THEN json_set(document, '$.body', '') ELSE document END AS value
                     FROM memory_engine_source_documents WHERE account_id = ?1
                     ORDER BY created_at_ms, source_document_id", &params)?,
                reference_spans: self.json_rows(
                    "SELECT span AS value FROM memory_engine_reference_spans WHERE account_id = ?1
                     ORDER BY created_at_ms, reference_span_id", account)?,
                generated_prompt_drafts: self.json_rows(
                    "SELECT draft AS value FROM memory_engine_generated_prompt_drafts WHERE account_id = ?1
                     ORDER BY created_at_ms, draft_id", account)?,
                review_units: self.json_rows(
                    "SELECT record AS value FROM memory_engine_review_units WHERE account_id = ?1
                     ORDER BY created_at_ms, review_unit_id", account)?,
                schedules: self.json_rows(
                    "SELECT json_object('reviewUnitId', review_unit_id, 'state', json(state)) AS value
                     FROM memory_engine_schedules WHERE account_id = ?1
                     ORDER BY updated_at_ms, review_unit_id", account)?,
                attempts: self.json_rows(
                    "SELECT CASE WHEN ?2 THEN json_set(json_remove(attempt, '$.gradedPrompt'),
                         '$.submittedAnswer', '', '$.grade',
                         CASE WHEN json_type(attempt, '$.grade') = 'object'
                             THEN json_set(json_extract(attempt, '$.grade'), '$.submittedAnswer', '',
                                 '$.expectedAnswer', '', '$.feedback', '', '$.criterionResults', json('[]'))
                             ELSE NULL END)
                     ELSE attempt END AS value
                     FROM memory_engine_attempts WHERE account_id = ?1
                     ORDER BY occurred_at_ms, attempt_id", &params)?,
                generation_runs: self.json_rows(
                    "SELECT run AS value FROM memory_engine_generation_runs WHERE account_id = ?1
                     ORDER BY started_at_ms, generation_run_id", account)?,
                content_feedback: self.json_rows(
                    "SELECT feedback AS value FROM memory_engine_content_feedback AS candidate
                     WHERE account_id = ?1 AND (NOT ?2 OR NOT EXISTS (
                         SELECT 1 FROM memory_engine_content_feedback AS child
                         WHERE child.account_id = candidate.account_id
                             AND json_extract(child.feedback, '$.supersedesId') = candidate.feedback_id))
                     ORDER BY occurred_at_ms, feedback_id", &params)?,
                applied_reviews: if review_only { Vec::new() } else { self.json_rows(
                    "SELECT json_object('key', receipt_key, 'attempt', json(attempt),
                         'expectedPriorScheduleState', json(expected_prior_schedule_state),
                         'scheduleState', json(schedule_state)) AS value
                     FROM memory_engine_applied_reviews WHERE account_id = ?1
                     ORDER BY applied_at_ms, receipt_key", account)? },
                concept_reference_notes: if review_only { Vec::new() } else { self.json_rows(
                    "SELECT note AS value FROM memory_engine_concept_reference_notes WHERE account_id = ?1
                     ORDER BY updated_at_ms, concept_key", account)? },
                remediation_packs: self.json_rows(
                    "SELECT pack AS value FROM memory_engine_remediation_packs WHERE account_id = ?1
                     ORDER BY created_at_ms, pack_id", account)?,
                review_exposures: if review_only { Vec::new() } else { self.json_rows(
                    "SELECT exposure AS value FROM memory_engine_review_exposures WHERE account_id = ?1
                     ORDER BY revealed_at_ms, review_unit_id, occurrence_key", account)? },
                ..BetaStoreSnapshot::default()
            })
        })
    }

    pub(crate) fn publish_generated_content(&self, run_id: Option<&str>) -> AppResult<()> {
        self.write_transaction(|| {
            let account = &[json!(self.account_id)];
            let mut snapshot = BetaStoreSnapshot {
                source_documents: self.json_rows("SELECT json_set(document, '$.body', '') AS value FROM memory_engine_source_documents WHERE account_id = ?", account)?,
                reference_spans: self.json_rows("SELECT span AS value FROM memory_engine_reference_spans WHERE account_id = ?", account)?,
                concept_reference_notes: self.json_rows("SELECT note AS value FROM memory_engine_concept_reference_notes WHERE account_id = ?", account)?,
                generated_prompt_drafts: self.json_rows("SELECT draft AS value FROM memory_engine_generated_prompt_drafts WHERE account_id = ?", account)?,
                review_units: self.json_rows("SELECT record AS value FROM memory_engine_review_units WHERE account_id = ?", account)?,
                remediation_packs: self.json_rows("SELECT pack AS value FROM memory_engine_remediation_packs WHERE account_id = ?", account)?,
                generation_runs: self.json_rows(
                    "SELECT run AS value FROM memory_engine_generation_runs AS generation
                     WHERE account_id = ?1
                       AND COALESCE(json_extract(run, '$.status'), '') NOT IN ('pending', 'superseded', 'cancelled', 'failed')
                       AND NOT EXISTS (SELECT 1 FROM memory_engine_generation_job_attempts AS attempt
                           WHERE attempt.account_id = generation.account_id
                             AND attempt.generation_run_id = generation.generation_run_id
                             AND attempt.status <> 'succeeded')", account)?,
                ..BetaStoreSnapshot::default()
            };
            loop {
                let units = memory_engine_persistence::review_units_for_publication(&snapshot, run_id)
                    .map_err(|error| policy_error(&error))?;
                if units.is_empty() { break; }
                for unit in &units {
                    self.insert_published_unit(unit)?;
                }
                snapshot.review_units.extend(units);
                if run_id.is_some() { break; }
            }
            Ok(())
        })
    }

    fn insert_published_unit(&self, unit: &BetaReviewUnitRecord) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_review_units (account_id, review_unit_id, record, created_at_ms, archived_at_ms) VALUES (?, ?, ?, ?, NULL) ON CONFLICT (account_id, review_unit_id) DO NOTHING",
            &[json!(self.account_id), json!(unit.review_unit_id.as_str()), encoded(unit)?, json!(unit.created_at)])
    }

    fn provenance(
        &self,
        sources: &[String],
        references: &[String],
        note_key: Option<&str>,
    ) -> AppResult<BetaStoreSnapshot> {
        let mut snapshot = BetaStoreSnapshot::default();
        for id in sources {
            let source = self.json_row(
                "SELECT json_set(document, '$.body', '') AS value FROM memory_engine_source_documents WHERE account_id = ? AND source_document_id = ?",
                &[json!(self.account_id), json!(id)],
            )?.ok_or_else(|| Failure::not_found("Unknown source document"))?;
            snapshot.source_documents.push(source);
        }
        for id in references {
            let reference = self.json_row(
                "SELECT span AS value FROM memory_engine_reference_spans WHERE account_id = ? AND reference_span_id = ?",
                &[json!(self.account_id), json!(id)],
            )?.ok_or_else(|| Failure::not_found("Unknown reference span"))?;
            snapshot.reference_spans.push(reference);
        }
        if let Some(key) = note_key {
            let note = self.json_row(
                "SELECT note AS value FROM memory_engine_concept_reference_notes WHERE account_id = ? AND concept_key = ?",
                &[json!(self.account_id), json!(key)],
            )?.ok_or_else(|| Failure::not_found("Unknown concept reference note"))?;
            snapshot.concept_reference_notes.push(note);
        }
        Ok(snapshot)
    }

    fn put_source(&self, document: &SourceDocument) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_source_documents (account_id, source_document_id, document, created_at_ms) VALUES (?, ?, ?, ?) ON CONFLICT (account_id, source_document_id) DO UPDATE SET document = excluded.document, created_at_ms = excluded.created_at_ms",
            &[json!(self.account_id), json!(document.id), encoded(document)?, json!(document.created_at)])
    }

    fn put_reference(&self, span: &ReferenceSpan) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_reference_spans (account_id, reference_span_id, source_document_id, span, created_at_ms) VALUES (?, ?, ?, ?, ?) ON CONFLICT (account_id, reference_span_id) DO UPDATE SET source_document_id = excluded.source_document_id, span = excluded.span, created_at_ms = excluded.created_at_ms",
            &[json!(self.account_id), json!(span.id), json!(span.source_document_id), encoded(span)?, json!(span.created_at)])
    }

    fn put_note(&self, note: &ConceptReferenceNote) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_concept_reference_notes (account_id, concept_key, note, updated_at_ms) VALUES (?, ?, ?, ?) ON CONFLICT (account_id, concept_key) DO UPDATE SET note = excluded.note, updated_at_ms = excluded.updated_at_ms",
            &[json!(self.account_id), json!(note.concept_key), encoded(note)?, json!(note.updated_at)])
    }

    fn put_run(&self, run: &GenerationRun) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_generation_runs (account_id, generation_run_id, run, started_at_ms) VALUES (?, ?, ?, ?) ON CONFLICT (account_id, generation_run_id) DO UPDATE SET run = excluded.run, started_at_ms = excluded.started_at_ms",
            &[json!(self.account_id), json!(run.id), encoded(run)?, json!(run.started_at)])
    }

    fn put_draft(&self, draft: &GeneratedPromptDraft) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_generated_prompt_drafts (account_id, draft_id, review_unit_id, draft, created_at_ms) VALUES (?, ?, ?, ?, ?) ON CONFLICT (account_id, draft_id) DO UPDATE SET review_unit_id = excluded.review_unit_id, draft = excluded.draft, created_at_ms = excluded.created_at_ms",
            &[json!(self.account_id), json!(draft.id), json!(draft.review_unit_id.as_str()), encoded(draft)?, json!(draft.created_at)])
    }

    fn put_unit(&self, unit: &BetaReviewUnitRecord) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_review_units (account_id, review_unit_id, record, created_at_ms, archived_at_ms) VALUES (?, ?, ?, ?, ?) ON CONFLICT (account_id, review_unit_id) DO UPDATE SET record = excluded.record, created_at_ms = excluded.created_at_ms, archived_at_ms = excluded.archived_at_ms",
            &[json!(self.account_id), json!(unit.review_unit_id.as_str()), encoded(unit)?, json!(unit.created_at), json!(unit.archived_at)])
    }

    fn put_schedule(
        &self,
        id: &ReviewUnitId,
        state: &ScheduleState,
        updated_at: i64,
    ) -> AppResult<()> {
        finite_schedule(state)?;
        self.db.execute("INSERT INTO memory_engine_schedules (account_id, review_unit_id, state, updated_at_ms) VALUES (?, ?, ?, ?) ON CONFLICT (account_id, review_unit_id) DO UPDATE SET state = excluded.state, updated_at_ms = excluded.updated_at_ms",
            &[json!(self.account_id), json!(id.as_str()), encoded(state)?, json!(updated_at)])
    }

    fn insert_attempt(&self, attempt: &ServiceAttemptRecord) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_attempts (account_id, review_unit_id, prompt_id, idempotency_key, attempt, occurred_at_ms) VALUES (?, ?, ?, ?, ?, ?)",
            &[json!(self.account_id), json!(attempt.review_unit_id.as_str()), json!(attempt.prompt_id), json!(attempt.idempotency_key), encoded(attempt)?, json!(attempt.occurred_at)])
    }

    fn insert_receipt(&self, receipt: &AppliedReviewReceipt) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_applied_reviews (account_id, receipt_key, review_unit_id, attempt, expected_prior_schedule_state, schedule_state, applied_at_ms) VALUES (?, ?, ?, ?, ?, ?, ?)",
            &[json!(self.account_id), json!(receipt.key), json!(receipt.attempt.review_unit_id.as_str()), encoded(&receipt.attempt)?, receipt.expected_prior_schedule_state.as_ref().map(encoded).transpose()?.unwrap_or(Value::Null), encoded(&receipt.schedule_state)?, json!(receipt.attempt.occurred_at)])
    }

    fn insert_feedback(&self, feedback: &ContentFeedback) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_content_feedback (account_id, feedback_id, review_unit_id, feedback, occurred_at_ms) VALUES (?, ?, ?, ?, ?)",
            &[json!(self.account_id), json!(feedback.id), json!(feedback.review_unit_id.as_str()), encoded(feedback)?, json!(feedback.occurred_at)])
    }

    fn put_pack(&self, pack: &RemediationPackRecord) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_remediation_packs (account_id, pack_id, pack, attempt_id, created_at_ms) VALUES (?, ?, ?, ?, ?) ON CONFLICT (account_id, pack_id) DO UPDATE SET pack = excluded.pack, attempt_id = excluded.attempt_id, created_at_ms = excluded.created_at_ms",
            &[json!(self.account_id), json!(pack.id), encoded(pack)?, json!(pack.attempt_id), json!(pack.created_at)])
    }

    fn insert_exposure(&self, exposure: &ReviewExposure) -> AppResult<()> {
        self.db.execute("INSERT INTO memory_engine_review_exposures (account_id, review_unit_id, occurrence_key, exposure, revealed_at_ms) VALUES (?, ?, ?, ?, ?) ON CONFLICT (account_id, review_unit_id, occurrence_key) DO NOTHING",
            &[json!(self.account_id), json!(exposure.review_unit_id.as_str()), json!(review_occurrence_key(exposure.prior_schedule_state.as_ref())), encoded(exposure)?, json!(exposure.revealed_at)])
    }

    pub fn active_graded_review(&self) -> AppResult<Option<Value>> {
        self.json_row("SELECT active_graded_review AS value FROM memory_engine_accounts WHERE account_id = ? AND active_graded_review IS NOT NULL", &[json!(self.account_id)])
    }

    pub fn save_active_graded_review(
        &mut self,
        active_review: &Value,
        idempotency_key: &str,
        recovering: bool,
    ) -> AppResult<bool> {
        nonblank(idempotency_key, "Review idempotency key")?;
        if active_review.get("idempotency_key").and_then(Value::as_str) != Some(idempotency_key) {
            return Err(Failure::bad_request("Active review idempotency mismatch"));
        }
        self.write_transaction(|| self.exists(
            "UPDATE memory_engine_accounts SET active_graded_review = ?2 WHERE account_id = ?1
                AND ((?4 AND active_graded_review IS NULL) OR (NOT ?4 AND (
                    active_graded_review IS NULL
                    OR (json_extract(active_graded_review, '$.state') = 'consumed' AND json_extract(active_graded_review, '$.idempotency_key') IS NOT ?3)
                    OR (coalesce(json_extract(active_graded_review, '$.state'), 'active') = 'active' AND json_extract(active_graded_review, '$.idempotency_key') = ?3))))
                RETURNING 1 AS present",
            &[json!(self.account_id), encoded(active_review)?, json!(idempotency_key), json!(recovering)],
        ))
    }

    pub fn consume_active_graded_review(&mut self, idempotency_key: &str) -> AppResult<bool> {
        nonblank(idempotency_key, "Review idempotency key")?;
        self.write_transaction(|| self.exists(
            "UPDATE memory_engine_accounts SET active_graded_review = json_object('state', 'consumed', 'idempotency_key', ?2) WHERE account_id = ?1 AND (active_graded_review IS NULL OR json_extract(active_graded_review, '$.idempotency_key') = ?2) RETURNING 1 AS present",
            &[json!(self.account_id), json!(idempotency_key)],
        ))
    }

    pub fn applied_review_idempotency_key_exists(&self, idempotency_key: &str) -> AppResult<bool> {
        self.exists("SELECT 1 AS present FROM memory_engine_applied_reviews WHERE account_id = ? AND receipt_key = ?", &[json!(self.account_id), json!(format!("idempotency:{idempotency_key}"))])
    }

    pub fn latest_applied_review_idempotency_key(&self) -> AppResult<Option<String>> {
        let rows: Vec<JsonRow> = self.db.query("SELECT json_extract(attempt, '$.idempotencyKey') AS value FROM memory_engine_applied_reviews WHERE account_id = ? AND json_extract(attempt, '$.idempotencyKey') IS NOT NULL ORDER BY applied_at_ms DESC, receipt_key DESC LIMIT 1", &[json!(self.account_id)])?;
        Ok(rows.into_iter().next().map(|row| row.value))
    }

    pub fn content_feedback_head(&self, id: &ReviewUnitId) -> AppResult<Option<ContentFeedback>> {
        self.json_row("SELECT candidate.feedback AS value FROM memory_engine_content_feedback AS candidate WHERE candidate.account_id = ?1 AND candidate.review_unit_id = ?2 AND NOT EXISTS (SELECT 1 FROM memory_engine_content_feedback AS child WHERE child.account_id = candidate.account_id AND child.review_unit_id = candidate.review_unit_id AND json_extract(child.feedback, '$.supersedesId') = candidate.feedback_id) ORDER BY candidate.occurred_at_ms DESC, candidate.feedback_id DESC LIMIT 1", &[json!(self.account_id), json!(id.as_str())])
    }

    fn decide_draft(
        &self,
        id: &str,
        input: &LearnerDraftDecisionInput<'_>,
        decided_at: i64,
    ) -> AppResult<(GeneratedPromptDraft, Option<BetaReviewUnitRecord>)> {
        self.write_transaction(|| {
            let draft = self.require_draft(id)?;
            let run = draft.generation_run_id.as_deref().map(|id| self.generation_run(id)).transpose()?.flatten();
            let transitioned = transition_learner_draft(&draft, run.as_ref(), input, decided_at)
                .map_err(|error| policy_error(&error))?;
            let unchanged = transitioned == draft;
            let draft = transitioned;
            let rejecting = matches!(input, LearnerDraftDecisionInput::Reject);
            if unchanged {
                let unit = if rejecting { None } else { Some(self.review_unit(&draft.review_unit_id)?) };
                return Ok((draft, unit));
            }
            self.put_draft(&draft)?;
            if rejecting {
                self.db.execute("UPDATE memory_engine_review_units SET archived_at_ms = ?3, record = json_set(record, '$.archivedAt', ?3) WHERE account_id = ?1 AND json_extract(record, '$.generatedPromptDraftId') = ?2",
                    &[json!(self.account_id), json!(draft.id), json!(decided_at)])?;
            } else if matches!(input, LearnerDraftDecisionInput::Edit { .. }) {
                self.db.execute("UPDATE memory_engine_review_units SET record = json_set(record, '$.prompt', json(?3)) WHERE account_id = ?1 AND json_extract(record, '$.generatedPromptDraftId') = ?2",
                    &[json!(self.account_id), json!(draft.id), encoded(&draft.prompt)?])?;
            }
            if rejecting {
                return Ok((draft, None));
            }
            let unit = promoted_review_unit(&draft);
            self.insert_published_unit(&unit)?;
            let persisted = self.review_unit(&unit.review_unit_id)?;
            Ok((draft, Some(persisted)))
        })
    }

    fn lease(
        &self,
        run_id: &str,
        attempt: i32,
        token: &str,
        now: i64,
    ) -> AppResult<Option<LeaseRow>> {
        let rows = self.db.query("SELECT attempt.job_id, COALESCE(attempt.cost_usd_micros, attempt.reservation_cost_usd_micros) AS reservation_cost_usd_micros FROM memory_engine_generation_job_attempts AS attempt JOIN memory_engine_generation_jobs AS job ON job.account_id = attempt.account_id AND job.job_id = attempt.job_id WHERE attempt.account_id = ?1 AND attempt.generation_run_id = ?2 AND attempt.attempt = ?3 AND attempt.lease_token = ?4 AND attempt.status = 'running' AND job.status = 'running' AND job.attempts = attempt.attempt AND job.lease_token = attempt.lease_token AND job.lease_expires_at_ms > ?5 LIMIT 1",
            &[json!(self.account_id), json!(run_id), json!(attempt), json!(token), json!(now)])?;
        Ok(rows.into_iter().next())
    }
}

fn encoded(value: &impl Serialize) -> AppResult<Value> {
    Ok(Value::String(serde_json::to_string(value)?))
}

fn nonblank(value: &str, label: &'static str) -> AppResult<()> {
    assert_non_blank(value, label).map_err(|error| policy_error(&error))
}

fn policy_error(error: &BetaStoreError) -> Failure {
    match error {
        BetaStoreError::UnknownSourceDocument(_)
        | BetaStoreError::UnknownReferenceSpan(_)
        | BetaStoreError::UnknownConceptReferenceNote(_)
        | BetaStoreError::UnknownReviewUnit(_)
        | BetaStoreError::UnknownGeneratedPromptDraft(_) => {
            Failure::not_found("Unknown study record")
        }
        BetaStoreError::LearnerDraftDecisionAlreadyRecorded(_)
        | BetaStoreError::MissingGenerationRunForAcceptedDraft
        | BetaStoreError::DuplicateAppliedReview(_)
        | BetaStoreError::StaleScheduleWrite(_)
        | BetaStoreError::SourceDocumentArchived(_)
        | BetaStoreError::ReviewUnitArchived(_) => Failure::conflict("Study state changed"),
        BetaStoreError::Blank { label } => Failure::bad_request(format!("{label} cannot be blank")),
        BetaStoreError::InvalidBooleanAnswer => {
            Failure::bad_request("Expected answer must be true or false")
        }
        _ => Failure::bad_request("Invalid study record"),
    }
}

fn active(unit: &BetaReviewUnitRecord) -> AppResult<()> {
    if unit.archived_at.is_some() {
        Err(Failure::conflict("Review unit is archived"))
    } else {
        Ok(())
    }
}

impl BetaStudyStore for SqlStudyStore {
    fn review_snapshot(&self) -> AppResult<BetaStoreSnapshot> {
        self.read_snapshot(true)
    }

    fn applied_review(
        &self,
        review_unit_id: &str,
        idempotency_key: &str,
    ) -> AppResult<Option<AppliedReviewReceipt>> {
        self.json_row(
            "SELECT json_object('key', receipt_key, 'attempt', json(attempt), 'expectedPriorScheduleState', json(expected_prior_schedule_state), 'scheduleState', json(schedule_state)) AS value FROM memory_engine_applied_reviews WHERE account_id = ? AND review_unit_id = ? AND receipt_key = ?",
            &[json!(self.account_id), json!(review_unit_id), json!(format!("idempotency:{idempotency_key}"))],
        )
    }

    fn reveal_review_occurrence(
        &mut self,
        review_unit_id: &ReviewUnitId,
        prior_schedule: Option<ScheduleState>,
        revealed_at: i64,
    ) -> AppResult<()> {
        self.write_transaction(|| {
            self.review_unit(review_unit_id)?;
            if self.read_schedule_state(review_unit_id)? != prior_schedule {
                return Err(Failure::conflict("Review occurrence changed"));
            }
            self.insert_exposure(&ReviewExposure {
                review_unit_id: review_unit_id.clone(),
                prior_schedule_state: prior_schedule,
                revealed_at,
            })
        })
    }

    fn save_source_document(&mut self, document: SourceDocument) -> AppResult<SourceDocument> {
        nonblank(&document.id, "Source document id")?;
        nonblank(&document.title, "Source document title")?;
        self.write_transaction(|| {
            self.put_source(&document)?;
            Ok(document)
        })
    }

    fn archive_source_document(
        &mut self,
        source_document_id: &str,
        archived_at: i64,
    ) -> AppResult<SourceDocument> {
        self.write_transaction(|| {
            let mut source = self.require_source(source_document_id)?;
            source.archived_at = Some(archived_at);
            self.put_source(&source)?;
            Ok(source)
        })
    }

    fn update_source_document_permission(
        &mut self,
        source_document_id: &str,
        permission: SourcePermission,
    ) -> AppResult<SourceDocument> {
        self.write_transaction(|| {
            let mut source = self.require_source(source_document_id)?;
            if source.archived_at.is_some() {
                return Err(Failure::conflict("Source document is archived"));
            }
            source.permission = permission;
            self.put_source(&source)?;
            Ok(source)
        })
    }

    fn keep_generated_prompt_draft(
        &mut self,
        draft_id: &str,
        decided_at: i64,
    ) -> AppResult<BetaReviewUnitRecord> {
        self.decide_draft(draft_id, &LearnerDraftDecisionInput::Keep, decided_at)?
            .1
            .ok_or_else(|| Failure::internal("Kept draft has no review unit"))
    }

    fn edit_and_keep_generated_prompt_draft(
        &mut self,
        draft_id: &str,
        prompt_text: &str,
        expected_answer: &str,
        choices: &[String],
        decided_at: i64,
    ) -> AppResult<BetaReviewUnitRecord> {
        self.decide_draft(
            draft_id,
            &LearnerDraftDecisionInput::Edit {
                prompt_text,
                expected_answer,
                choices,
            },
            decided_at,
        )?
        .1
        .ok_or_else(|| Failure::internal("Kept draft has no review unit"))
    }

    fn reject_generated_prompt_draft(
        &mut self,
        draft_id: &str,
        decided_at: i64,
    ) -> AppResult<GeneratedPromptDraft> {
        Ok(self
            .decide_draft(draft_id, &LearnerDraftDecisionInput::Reject, decided_at)?
            .0)
    }

    fn update_review_unit_prompt_text(
        &mut self,
        review_unit_id: &ReviewUnitId,
        prompt_text: &str,
        expected_answer: &str,
    ) -> AppResult<BetaReviewUnitRecord> {
        nonblank(prompt_text, "Review unit prompt")?;
        nonblank(expected_answer, "Review unit expected answer")?;
        self.write_transaction(|| {
            let mut unit = self.review_unit(review_unit_id)?;
            active(&unit)?;
            replace_prompt_text(&mut unit.prompt, prompt_text);
            replace_prompt_answer(&mut unit.prompt, expected_answer)
                .map_err(|error| policy_error(&error))?;
            if let Some(id) = &unit.generated_prompt_draft_id {
                let mut draft = self.require_draft(id)?;
                replace_prompt_text(&mut draft.prompt, prompt_text);
                replace_prompt_answer(&mut draft.prompt, expected_answer)
                    .map_err(|error| policy_error(&error))?;
                if !draft
                    .critique_notes
                    .iter()
                    .any(|note| note == "Learner edited kept wording.")
                {
                    draft
                        .critique_notes
                        .push("Learner edited kept wording.".to_owned());
                }
                self.put_draft(&draft)?;
            }
            self.put_unit(&unit)?;
            Ok(unit)
        })
    }

    fn archive_review_unit(
        &mut self,
        review_unit_id: &ReviewUnitId,
        archived_at: i64,
    ) -> AppResult<BetaReviewUnitRecord> {
        self.write_transaction(|| {
            let mut unit = self.review_unit(review_unit_id)?;
            unit.archived_at = Some(archived_at);
            self.put_unit(&unit)?;
            Ok(unit)
        })
    }

    fn snooze_review_unit_until(
        &mut self,
        review_unit_id: &ReviewUnitId,
        snoozed_until: i64,
    ) -> AppResult<BetaReviewUnitRecord> {
        self.write_transaction(|| {
            let mut unit = self.review_unit(review_unit_id)?;
            active(&unit)?;
            // Availability is boundary metadata. Never rewrite FSRS or occurrence.
            unit.snoozed_until = Some(snoozed_until);
            self.put_unit(&unit)?;
            Ok(unit)
        })
    }

    fn snooze_review_units_for_concept_until(
        &mut self,
        concept_key: &str,
        snoozed_until: i64,
    ) -> AppResult<Vec<BetaReviewUnitRecord>> {
        let concept_key = normalized_concept_key(concept_key);
        nonblank(&concept_key, "Concept key")?;
        self.write_transaction(|| {
            #[derive(Deserialize)]
            struct ConceptRow { review_unit_id: String, concept_key: Option<String> }
            let rows: Vec<ConceptRow> = self.db.query(
                "SELECT review_unit_id, json_extract(record, '$.queue.conceptKey') AS concept_key FROM memory_engine_review_units WHERE account_id = ? AND archived_at_ms IS NULL ORDER BY created_at_ms, review_unit_id",
                &[json!(self.account_id)],
            )?;
            let mut units = Vec::new();
            for row in rows {
                if row.concept_key.as_deref().is_some_and(|key| normalized_concept_key(key) == concept_key) {
                    let mut unit = self.review_unit(&ReviewUnitId::new(row.review_unit_id))?;
                    unit.snoozed_until = Some(snoozed_until);
                    self.put_unit(&unit)?;
                    units.push(unit);
                }
            }
            Ok(units)
        })
    }

    fn set_review_unit_lifecycle(
        &mut self,
        review_unit_id: &ReviewUnitId,
        lifecycle: ReviewUnitLifecycle,
    ) -> AppResult<BetaReviewUnitRecord> {
        self.write_transaction(|| {
            let mut unit = self.review_unit(review_unit_id)?;
            active(&unit)?;
            unit.queue.lifecycle = lifecycle;
            self.put_unit(&unit)?;
            Ok(unit)
        })
    }
}

impl MemoryServiceStore for SqlStudyStore {
    type Error = Failure;

    fn record_attempt(&mut self, attempt: ServiceAttemptRecord) -> AppResult<()> {
        self.write_transaction(|| {
            let context = BetaStoreSnapshot {
                review_units: vec![self.review_unit(&attempt.review_unit_id)?],
                ..BetaStoreSnapshot::default()
            };
            assert_attempt_contract(&context, &attempt).map_err(|error| policy_error(&error))?;
            self.insert_attempt(&attempt)
        })
    }

    fn read_schedule_state(
        &self,
        review_unit_id: &ReviewUnitId,
    ) -> AppResult<Option<ScheduleState>> {
        self.json_row("SELECT state AS value FROM memory_engine_schedules WHERE account_id = ? AND review_unit_id = ?", &[json!(self.account_id), json!(review_unit_id.as_str())])
    }

    fn review_was_revealed(
        &self,
        review_unit_id: &ReviewUnitId,
        prior_schedule: Option<&ScheduleState>,
    ) -> AppResult<bool> {
        self.exists("SELECT 1 AS present FROM memory_engine_review_exposures WHERE account_id = ? AND review_unit_id = ? AND occurrence_key = ?",
            &[json!(self.account_id), json!(review_unit_id.as_str()), json!(review_occurrence_key(prior_schedule))])
    }

    fn apply_review(
        &mut self,
        review_unit_id: &ReviewUnitId,
        attempt: ServiceAttemptRecord,
        schedule_state: ScheduleState,
        expected_prior_schedule_state: Option<ScheduleState>,
    ) -> AppResult<()> {
        if review_unit_id != &attempt.review_unit_id {
            return Err(Failure::bad_request("Review unit mismatch"));
        }
        if schedule_state.last_review != Some(attempt.occurred_at) {
            return Err(Failure::bad_request("Schedule last review mismatch"));
        }
        self.write_transaction(|| {
            let context = BetaStoreSnapshot { review_units: vec![self.review_unit(review_unit_id)?], ..BetaStoreSnapshot::default() };
            assert_attempt_contract(&context, &attempt).map_err(|error| policy_error(&error))?;
            let key = applied_review_key(&attempt);
            if self.exists("SELECT 1 AS present FROM memory_engine_applied_reviews WHERE account_id = ? AND receipt_key = ?", &[json!(self.account_id), json!(key)])? {
                return Err(Failure::conflict("Review already applied"));
            }
            if self.read_schedule_state(review_unit_id)? != expected_prior_schedule_state {
                return Err(Failure::conflict("Review occurrence changed"));
            }
            // This second assistance read shares the commit transaction: a reveal
            // between the service's grading read and this write cannot earn recall.
            if self.review_was_revealed(review_unit_id, expected_prior_schedule_state.as_ref())?
                && !attempt.grade.as_ref().is_some_and(|grade| grade.verdict == Verdict::Revealed && grade.rating == Rating::Again && !grade.is_correct)
            {
                return Err(Failure::conflict("Review occurrence was revealed"));
            }
            self.insert_attempt(&attempt)?;
            self.put_schedule(review_unit_id, &schedule_state, attempt.occurred_at)?;
            self.insert_receipt(&AppliedReviewReceipt { key, attempt, expected_prior_schedule_state, schedule_state })
        })
    }

    fn list_queue_candidates(&self) -> AppResult<Vec<QueueCandidate>> {
        let rows: Vec<QueueRow> = self.db.query(
            "SELECT json_extract(unit.record, '$.queue') AS queue, schedule.state, json_extract(unit.record, '$.snoozedUntil') AS snoozed_until FROM memory_engine_review_units AS unit LEFT JOIN memory_engine_schedules AS schedule ON schedule.account_id = unit.account_id AND schedule.review_unit_id = unit.review_unit_id WHERE unit.account_id = ? AND unit.archived_at_ms IS NULL ORDER BY unit.created_at_ms, unit.review_unit_id",
            &[json!(self.account_id)],
        )?;
        rows.into_iter()
            .map(|row| {
                let queue: memory_engine_persistence::PersistedQueueCandidate =
                    serde_json::from_str(&row.queue)?;
                let state = row.state.as_deref().map(serde_json::from_str).transpose()?;
                let candidate = queue.with_schedule(state);
                Ok(match row.snoozed_until {
                    Some(until) => defer_queue_availability(&candidate, until),
                    None => candidate,
                })
            })
            .collect()
    }
}

impl ContentFeedbackStore for SqlStudyStore {
    type Error = Failure;

    fn record_content_feedback(&mut self, feedback: ContentFeedback) -> AppResult<ContentFeedback> {
        if feedback.account_id != self.account_id {
            return Err(Failure::forbidden("Feedback account mismatch"));
        }
        nonblank(&feedback.id, "Feedback id")?;
        if let Some(rationale) = &feedback.rationale {
            nonblank(rationale, "Feedback rationale")?;
        }
        self.write_transaction(|| {
            self.review_unit(&feedback.review_unit_id)?;
            if let Some(existing) = self.json_row::<ContentFeedback>(
                "SELECT feedback AS value FROM memory_engine_content_feedback WHERE account_id = ? AND feedback_id = ?",
                &[json!(self.account_id), json!(feedback.id)],
            )? {
                if content_feedback_replay_matches(&existing, &feedback) {
                    return Ok(existing);
                }
                return Err(Failure::conflict("Feedback idempotency key already used"));
            }
            if let Some(parent) = &feedback.supersedes_id {
                let prior = self.json_row::<ContentFeedback>(
                    "SELECT feedback AS value FROM memory_engine_content_feedback WHERE account_id = ? AND feedback_id = ?",
                    &[json!(self.account_id), json!(parent)],
                )?.ok_or_else(|| Failure::not_found("Unknown superseded feedback"))?;
                if prior.review_unit_id != feedback.review_unit_id {
                    return Err(Failure::bad_request("Feedback supersedes another review unit"));
                }
            }
            let head = self.content_feedback_head(&feedback.review_unit_id)?;
            if head.as_ref().map(|feedback| feedback.id.as_str()) != feedback.supersedes_id.as_deref() {
                return Err(Failure::conflict("Feedback revision changed"));
            }
            self.insert_feedback(&feedback)?;
            Ok(feedback)
        })
    }
}

impl BetaGenerationStore for SqlStudyStore {
    type Error = Failure;

    fn snapshot(&self) -> AppResult<BetaStoreSnapshot> {
        self.export_snapshot()
    }

    fn save_generation_run(&mut self, run: GenerationRun) -> AppResult<GenerationRun> {
        nonblank(&run.id, "Generation run id")?;
        self.write_transaction(|| {
            if let Some(existing) = self.generation_run(&run.id)? {
                if memory_engine_persistence::generation_run_is_published(&existing) {
                    return Ok(existing);
                }
            }
            let mut run = run;
            if self.exists("SELECT 1 AS present FROM memory_engine_generation_job_attempts WHERE account_id = ? AND generation_run_id = ? LIMIT 1",
                &[json!(self.account_id), json!(run.id)])?
            {
                run.completed_at = Some(i64::MIN);
            }
            for id in &run.source_document_ids {
                self.require_source(id)?;
            }
            self.put_run(&run)?;
            if memory_engine_persistence::generation_run_is_published(&run) {
                self.publish_generated_content(Some(&run.id))?;
            }
            Ok(run)
        })
    }

    fn save_reference_span(&mut self, reference: ReferenceSpan) -> AppResult<ReferenceSpan> {
        nonblank(&reference.id, "Reference span id")?;
        nonblank(&reference.text, "Reference span text")?;
        self.write_transaction(|| {
            self.require_source(&reference.source_document_id)?;
            self.put_reference(&reference)?;
            Ok(reference)
        })
    }

    fn save_concept_reference_note(
        &mut self,
        note: ConceptReferenceNote,
    ) -> AppResult<ConceptReferenceNote> {
        assert_concept_reference_note_contract(&note).map_err(|error| policy_error(&error))?;
        self.write_transaction(|| {
            self.put_note(&note)?;
            Ok(note)
        })
    }

    fn save_generated_prompt_draft(
        &mut self,
        draft: GeneratedPromptDraft,
    ) -> AppResult<GeneratedPromptDraft> {
        self.write_transaction(|| {
            let context = self.provenance(
                &draft.source_document_ids,
                &draft.reference_span_ids,
                draft.concept_reference_note_key.as_deref(),
            )?;
            assert_draft_contract(&context, &draft).map_err(|error| policy_error(&error))?;
            if let Some(existing) = self.draft(&draft.id)? {
                if existing.learner_decision.is_some()
                    || self.exists("SELECT 1 AS present FROM memory_engine_review_units WHERE account_id = ? AND json_extract(record, '$.generatedPromptDraftId') = ? LIMIT 1",
                        &[json!(self.account_id), json!(draft.id)])?
                {
                    return Ok(existing);
                }
            }
            self.put_draft(&draft)?;
            Ok(draft)
        })
    }

    fn save_remediation_pack(
        &mut self,
        pack: RemediationPackRecord,
    ) -> AppResult<RemediationPackRecord> {
        nonblank(&pack.id, "Remediation pack id")?;
        nonblank(&pack.attempt_id, "Remediation attempt id")?;
        self.write_transaction(|| {
            self.review_unit(&pack.parent_review_unit_id)?;
            for id in &pack.review_unit_ids {
                if !self.exists("SELECT 1 AS present FROM memory_engine_generated_prompt_drafts WHERE account_id = ? AND review_unit_id = ? LIMIT 1",
                    &[json!(self.account_id), json!(id.as_str())])?
                {
                    self.review_unit(id)?;
                }
            }
            self.put_pack(&pack)?;
            Ok(pack)
        })
    }

    fn discard_generation_run(&mut self, run_id: &str) -> AppResult<()> {
        // Rollback is allowed after a lease is lost.
        self.db.transaction(|| self.remove_run_output(run_id))
    }

    fn finalize_generation_run(
        &mut self,
        run_id: &str,
        generation_attempt: i32,
        lease_token: &str,
        now_ms: i64,
        _lease_valid: bool,
    ) -> AppResult<bool> {
        self.db.transaction(|| {
            let Some(mut run) = self.generation_run(run_id)? else { return Ok(false) };
            // A replay of the terminal, exactly identified provider attempt may
            // observe cleared job lease fields. Never delete its committed run.
            if run.completed_at.is_some_and(|at| at != i64::MIN)
                && self.exists("SELECT 1 AS present FROM memory_engine_generation_job_attempts WHERE account_id = ? AND generation_run_id = ? AND attempt = ? AND lease_token = ? AND status = 'succeeded' LIMIT 1",
                    &[json!(self.account_id), json!(run_id), json!(generation_attempt), json!(lease_token)])?
            {
                return Ok(true);
            }
            let Some(lease) = self.lease(run_id, generation_attempt, lease_token, now_ms)? else {
                self.remove_run_output(run_id)?;
                return Ok(false);
            };
            for id in &run.source_document_ids {
                let source = self.require_source(id)?;
                if source.archived_at.is_some()
                    || !run.source_permissions.iter().any(|receipt| receipt.source_document_id == *id && receipt.permission == source.permission
                        && (receipt.consented || source.permission == SourcePermission::LocalOnly))
                {
                    return Err(Failure::conflict("Generation source permission changed"));
                }
            }
            run.completed_at = Some(now_ms);
            let reported_cost = run.usage.as_ref().and_then(|usage| usage.cost_usd_micros);
            let accounted_cost = reported_cost.unwrap_or(lease.reservation_cost_usd_micros);
            if accounted_cost < 0 {
                return Err(Failure::bad_request("Invalid generation cost"));
            }
            self.put_run(&run)?;
            let attempt_updated = self.exists(
                "UPDATE memory_engine_generation_job_attempts SET status = 'succeeded', cost_usd_micros = ?5, reserved_cost_usd_micros = 0, cost_unknown = ?6, usage_json = ?7, error = NULL, completed_at_ms = ?8, updated_at_ms = ?8 WHERE account_id = ?1 AND job_id = ?2 AND attempt = ?3 AND lease_token = ?4 AND status = 'running' RETURNING 1 AS present",
                &[json!(self.account_id), json!(lease.job_id), json!(generation_attempt), json!(lease_token), json!(accounted_cost), json!(reported_cost.is_none()), run.usage.as_ref().map(encoded).transpose()?.unwrap_or(Value::Null), json!(now_ms)],
            )?;
            let job_updated = self.exists(
                "UPDATE memory_engine_generation_jobs SET status = 'succeeded', card_count = (SELECT count(*) FROM memory_engine_generated_prompt_drafts WHERE account_id = ?1 AND json_extract(draft, '$.generationRunId') = ?7 AND json_extract(draft, '$.validation.status') = 'accepted'), cost_usd_micros = legacy_cost_usd_micros + (SELECT COALESCE(SUM(cost_usd_micros), 0) FROM memory_engine_generation_job_attempts WHERE account_id = ?1 AND job_id = ?2), reserved_cost_usd_micros = 0, retryable = 0, error = NULL, retry_at_ms = NULL, lease_owner = NULL, lease_expires_at_ms = NULL, lease_token = NULL, updated_at_ms = ?6 WHERE account_id = ?1 AND job_id = ?2 AND attempts = ?3 AND lease_token = ?4 AND status = 'running' RETURNING 1 AS present",
                &[json!(self.account_id), json!(lease.job_id), json!(generation_attempt), json!(lease_token), json!(accounted_cost), json!(now_ms), json!(run_id)],
            )?;
            if !attempt_updated || !job_updated {
                return Err(Failure::conflict("Generation finalization lost lease"));
            }
            self.publish_generated_content(Some(run_id))?;
            Ok(true)
        })
    }
}

impl SqlStudyStore {
    fn remove_run_output(&self, run_id: &str) -> AppResult<()> {
        if self
            .generation_run(run_id)?
            .as_ref()
            .is_some_and(memory_engine_persistence::generation_run_is_published)
        {
            return Ok(());
        }
        let drafts: Vec<GeneratedPromptDraft> = self.json_rows(
            "SELECT draft AS value FROM memory_engine_generated_prompt_drafts AS draft WHERE account_id = ?1 AND json_extract(draft, '$.generationRunId') = ?2 AND json_extract(draft, '$.learnerDecision') IS NULL AND NOT EXISTS (SELECT 1 FROM memory_engine_review_units AS unit WHERE unit.account_id = draft.account_id AND json_extract(unit.record, '$.generatedPromptDraftId') = draft.draft_id)",
            &[json!(self.account_id), json!(run_id)],
        )?;
        let mut references = BTreeSet::new();
        let mut packs = BTreeSet::new();
        for draft in drafts {
            references.extend(draft.reference_span_ids);
            packs.extend(draft.remediation_pack_id);
            self.db.execute("DELETE FROM memory_engine_generated_prompt_drafts WHERE account_id = ? AND draft_id = ?", &[json!(self.account_id), json!(draft.id)])?;
        }
        if !self.exists("SELECT 1 AS present FROM memory_engine_generated_prompt_drafts WHERE account_id = ? AND json_extract(draft, '$.generationRunId') = ? LIMIT 1", &[json!(self.account_id), json!(run_id)])? {
            self.db.execute("DELETE FROM memory_engine_generation_runs WHERE account_id = ? AND generation_run_id = ?", &[json!(self.account_id), json!(run_id)])?;
        }
        for pack in packs {
            self.db.execute(
                "DELETE FROM memory_engine_remediation_packs WHERE account_id = ?1 AND pack_id = ?2 AND NOT EXISTS (SELECT 1 FROM memory_engine_generated_prompt_drafts WHERE account_id = ?1 AND json_extract(draft, '$.remediationPackId') = ?2) AND NOT EXISTS (SELECT 1 FROM memory_engine_review_units WHERE account_id = ?1 AND json_extract(record, '$.remediationPackId') = ?2)",
                &[json!(self.account_id), json!(pack)],
            )?;
        }
        for reference in references {
            self.db.execute(
                "DELETE FROM memory_engine_reference_spans WHERE account_id = ?1 AND reference_span_id = ?2 AND NOT EXISTS (SELECT 1 FROM memory_engine_generated_prompt_drafts AS draft, json_each(draft.draft, '$.referenceSpanIds') AS ref WHERE draft.account_id = ?1 AND ref.value = ?2) AND NOT EXISTS (SELECT 1 FROM memory_engine_review_units AS unit, json_each(unit.record, '$.referenceSpanIds') AS ref WHERE unit.account_id = ?1 AND ref.value = ?2)",
                &[json!(self.account_id), json!(reference)],
            )?;
        }
        Ok(())
    }
}

fn finite_schedule(state: &ScheduleState) -> AppResult<()> {
    if state.stability.is_finite() && state.difficulty.is_finite() {
        Ok(())
    } else {
        Err(Failure::bad_request("Invalid schedule state"))
    }
}
