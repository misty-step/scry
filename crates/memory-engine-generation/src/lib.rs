//! Beta content generation behind a provider boundary.
//!
//! This crate owns the [`DraftProvider`] boundary, deterministic providers
//! (structured-block parsing and a CI-safe fake model), draft validation,
//! cited reference creation, and generation-run bookkeeping. Model-backed
//! providers implement [`DraftProvider`] from their own boundary crates; this
//! crate never talks to a network.
//!
//! Every provider's output passes the same trust gate before persistence:
//! claimed source quotes and answer support are checked, model-expanded facts
//! are labeled honestly, duplicates and defective retrieval shapes are rejected,
//! and exercises require worked solutions. Acceptance is not proof of truth.

mod provider;

use std::{collections::BTreeSet, error::Error, fmt};

pub use provider::{
    classify_learning_intent, enforce_content_policy, BridgeMaterial, BridgeMaterialProvider,
    BridgeMaterialRequest, DraftCandidate, DraftProvider, DraftRejection, FakeModelProvider,
    FallbackProvider, LearningIntent, LearningIntentClassification, ProviderDrafts,
    ProviderFailure, ProviderFailureKind, ProviderUsage, ReferenceNoteDraft, ReferenceNoteProvider,
    ReferenceNoteRequest, ReviewPerformanceContext, SourceAuthorizationContext,
    SourceAuthorizationError, StructuredBlockProvider,
};

use memory_engine_core::{
    ExactPrompt, ExactPromptKind, ProgressionMetadata, Prompt, ReviewUnitId, ReviewUnitLifecycle,
};
use memory_engine_persistence::{
    BetaPersistenceStore, BetaReviewUnitRecord, BetaStoreError, BetaStoreSnapshot,
    ConceptReferenceNote, GeneratedLearningActivityKind, GeneratedPromptDraft,
    GeneratedPromptModel, GeneratedPromptValidation, GeneratedPromptValidationStatus,
    GenerationRun, GenerationRunUsage, PersistedQueueCandidate, ReferenceSpan,
    RemediationPackRecord, RemediationPackStatus, SourceDocument, SourceDocumentKind,
    SourcePermission, SourcePermissionReceipt,
};

/// The captured input retained for lineage of model-expanded knowledge. This
/// seed is not evidence for the generated answer; labels and critique notes
/// explicitly distinguish it from a verified source quotation.
fn knowledge_seed(source: &SourceDocument) -> &str {
    source
        .body
        .as_deref()
        .map(str::trim)
        .filter(|body| !body.is_empty())
        .unwrap_or_else(|| source.title.trim())
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BetaGenerationRequest {
    pub run_id: String,
    pub source_document_ids: Vec<String>,
    pub parent_review_unit_id: Option<ReviewUnitId>,
    pub started_at: i64,
    pub completed_at: Option<i64>,
    pub default_due: i64,
    pub model: Option<GeneratedPromptModel>,
    /// Keep queued output unscheduled until the worker lease fence publishes it.
    pub pending: bool,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BetaGenerationResult {
    pub run_id: String,
    pub draft_ids: Vec<String>,
    pub accepted_draft_ids: Vec<String>,
    pub rejected_draft_ids: Vec<String>,
    pub validation_failures: Vec<String>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BridgeGenerationRequest {
    pub run_id: String,
    pub parent_review_unit_id: ReviewUnitId,
    pub started_at: i64,
    pub completed_at: Option<i64>,
    pub default_due: i64,
    pub model: Option<GeneratedPromptModel>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BridgeGenerationResult {
    pub run_id: String,
    pub concept_key: String,
    pub reference_note_created: bool,
    pub accepted_draft_ids: Vec<String>,
    pub rejected_draft_ids: Vec<String>,
    pub validation_failures: Vec<String>,
}

/// Request to generate one automatic remediation pack for a failed parent
/// review unit's exact attempt.
///
/// Distinct from [`BridgeGenerationRequest`] (the explicit, pre-answer
/// learner escape hatch): remediation packs are triggered by the boundary
/// after a wrong, close, or revealed answer, are keyed by `attempt_id` for
/// idempotency, and never supersede the parent.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RemediationPackGenerationRequest {
    pub run_id: String,
    pub parent_review_unit_id: ReviewUnitId,
    pub attempt_id: String,
    pub started_at: i64,
    pub completed_at: Option<i64>,
    pub default_due: i64,
    pub model: Option<GeneratedPromptModel>,
}

/// Outcome of one remediation pack generation attempt.
///
/// `pack.status` is [`RemediationPackStatus::Active`] when at least one
/// draft was accepted, or [`RemediationPackStatus::Rejected`] when provider
/// output produced none — a data outcome, not an error, so a caller can
/// grade the learner's answer without the parent ever being buried.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RemediationPackGenerationResult {
    pub pack: RemediationPackRecord,
    pub accepted_draft_ids: Vec<String>,
    pub rejected_draft_ids: Vec<String>,
    pub validation_failures: Vec<String>,
}

#[derive(Debug, PartialEq, Eq)]
pub enum BetaGenerationError<E = BetaStoreError> {
    Store(E),
    UnknownSourceDocument(String),
    ArchivedSourceDocument(String),
    LocalOnlySource(String),
    UnknownReviewUnit(ReviewUnitId),
    SourceDocumentHasNoTextBody(String),
    ProviderFailure(String),
}

impl<E> fmt::Display for BetaGenerationError<E>
where
    E: fmt::Display,
{
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Store(error) => write!(formatter, "store error: {error}"),
            Self::UnknownSourceDocument(id) => write!(formatter, "Unknown source document: {id}"),
            Self::ArchivedSourceDocument(id) => write!(formatter, "Archived source document: {id}"),
            Self::LocalOnlySource(id) => write!(
                formatter,
                "Local-only source {id} cannot be sent to the model provider."
            ),
            Self::UnknownReviewUnit(id) => write!(formatter, "Unknown review unit: {id}"),
            Self::SourceDocumentHasNoTextBody(id) => {
                write!(formatter, "Source document has no text body: {id}")
            }
            Self::ProviderFailure(message) => formatter.write_str(message),
        }
    }
}

impl<E> Error for BetaGenerationError<E>
where
    E: Error + 'static,
{
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        match self {
            Self::Store(error) => Some(error),
            Self::UnknownSourceDocument(_)
            | Self::ArchivedSourceDocument(_)
            | Self::LocalOnlySource(_)
            | Self::UnknownReviewUnit(_)
            | Self::SourceDocumentHasNoTextBody(_)
            | Self::ProviderFailure(_) => None,
        }
    }
}

impl From<BetaStoreError> for BetaGenerationError<BetaStoreError> {
    fn from(error: BetaStoreError) -> Self {
        Self::Store(error)
    }
}

pub trait BetaGenerationStore {
    type Error;

    /// Read the current beta-store snapshot used to select source inputs and
    /// detect duplicate generated drafts.
    ///
    /// # Errors
    ///
    /// Returns the store error when snapshot reconstruction fails.
    fn snapshot(&self) -> Result<BetaStoreSnapshot, Self::Error>;

    /// Save a run receipt and atomically enroll accepted output when it completes.
    ///
    /// # Errors
    ///
    /// Returns the store error when the run is rejected or cannot be persisted.
    fn save_generation_run(&mut self, run: GenerationRun) -> Result<GenerationRun, Self::Error>;

    /// Save cited source evidence for a generated draft.
    ///
    /// # Errors
    ///
    /// Returns the store error when the reference is rejected or cannot be
    /// persisted.
    fn save_reference_span(
        &mut self,
        reference: ReferenceSpan,
    ) -> Result<ReferenceSpan, Self::Error>;

    /// Save provider-generated concept reference material.
    ///
    /// # Errors
    ///
    /// Returns the store error when the note is rejected or cannot be persisted.
    fn save_concept_reference_note(
        &mut self,
        note: ConceptReferenceNote,
    ) -> Result<ConceptReferenceNote, Self::Error>;

    /// Save a generated prompt draft.
    ///
    /// # Errors
    ///
    /// Returns the store error when the draft is rejected or cannot be persisted.
    fn save_generated_prompt_draft(
        &mut self,
        draft: GeneratedPromptDraft,
    ) -> Result<GeneratedPromptDraft, Self::Error>;

    /// Save or replace a boundary-owned remediation pack record.
    ///
    /// # Errors
    ///
    /// Returns the store error when the pack cannot be persisted.
    fn save_remediation_pack(
        &mut self,
        pack: RemediationPackRecord,
    ) -> Result<RemediationPackRecord, Self::Error>;

    /// Remove pending output when a durable worker lease loses its commit fence.
    ///
    /// # Errors
    ///
    /// Returns the store error when rollback cannot be persisted.
    fn discard_generation_run(&mut self, _run_id: &str) -> Result<(), Self::Error> {
        Ok(())
    }

    /// Atomically fence a run and remove all stale output when the lease is lost.
    ///
    /// # Errors
    /// Returns the store error when finalization cannot be persisted.
    fn finalize_generation_run(
        &mut self,
        _run_id: &str,
        _generation_attempt: i32,
        _lease_token: &str,
        _now_ms: i64,
        _lease_valid: bool,
    ) -> Result<bool, Self::Error> {
        Ok(true)
    }
}

impl BetaGenerationStore for BetaPersistenceStore {
    type Error = BetaStoreError;

    fn snapshot(&self) -> Result<BetaStoreSnapshot, Self::Error> {
        Ok(BetaPersistenceStore::snapshot(self))
    }

    fn save_generation_run(&mut self, run: GenerationRun) -> Result<GenerationRun, Self::Error> {
        BetaPersistenceStore::save_generation_run(self, run)
    }

    fn save_reference_span(
        &mut self,
        reference: ReferenceSpan,
    ) -> Result<ReferenceSpan, Self::Error> {
        BetaPersistenceStore::save_reference_span(self, reference)
    }

    fn save_concept_reference_note(
        &mut self,
        note: ConceptReferenceNote,
    ) -> Result<ConceptReferenceNote, Self::Error> {
        BetaPersistenceStore::save_concept_reference_note(self, note)
    }

    fn save_generated_prompt_draft(
        &mut self,
        draft: GeneratedPromptDraft,
    ) -> Result<GeneratedPromptDraft, Self::Error> {
        BetaPersistenceStore::save_generated_prompt_draft(self, draft)
    }

    fn save_remediation_pack(
        &mut self,
        pack: RemediationPackRecord,
    ) -> Result<RemediationPackRecord, Self::Error> {
        BetaPersistenceStore::save_remediation_pack(self, pack)
    }

    fn discard_generation_run(&mut self, run_id: &str) -> Result<(), Self::Error> {
        BetaPersistenceStore::discard_generation_run(self, run_id)
    }

    fn finalize_generation_run(
        &mut self,
        run_id: &str,
        _generation_attempt: i32,
        _lease_token: &str,
        now_ms: i64,
        lease_valid: bool,
    ) -> Result<bool, Self::Error> {
        BetaPersistenceStore::finalize_generation_run(self, run_id, now_ms, lease_valid)
    }
}

/// Generate deterministic beta drafts from structured source blocks.
///
/// Equivalent to [`run_beta_generation_with_provider`] with
/// [`StructuredBlockProvider`]; kept as the zero-configuration entry point.
///
/// # Errors
///
/// Returns [`BetaGenerationError`] when requested source documents are missing,
/// have no text body, or when the beta store rejects a generated entity.
pub fn run_beta_generation<S>(
    store: &mut S,
    request: BetaGenerationRequest,
) -> Result<BetaGenerationResult, BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    run_beta_generation_internal(store, &StructuredBlockProvider, request, false)
}

/// Generate beta drafts from the given provider's candidates.
///
/// Provider transport failures do not abort the run: they are recorded as
/// human-readable validation failures on the persisted run so the study UI
/// can explain zero-draft outcomes.
///
/// # Errors
///
/// Returns [`BetaGenerationError`] when requested source documents are missing,
/// have no text body, or when the beta store rejects a generated entity.
pub fn run_beta_generation_with_provider<S>(
    store: &mut S,
    provider: &dyn DraftProvider,
    request: BetaGenerationRequest,
) -> Result<BetaGenerationResult, BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    run_beta_generation_internal(store, provider, request, true)
}

fn run_beta_generation_internal<S>(
    store: &mut S,
    provider: &dyn DraftProvider,
    request: BetaGenerationRequest,
    enforce_source_permission: bool,
) -> Result<BetaGenerationResult, BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    let snapshot = store.snapshot().map_err(BetaGenerationError::Store)?;
    let sources = load_sources::<S::Error>(&snapshot, &request.source_document_ids)?;
    ensure_sources_not_archived(&sources)?;
    if enforce_source_permission {
        ensure_model_eligible(&sources)?;
    }
    if let Some(run) = snapshot.generation_runs.iter().find(|run| {
        run.id == request.run_id && memory_engine_persistence::generation_run_is_published(run)
    }) {
        let drafts = snapshot
            .generated_prompt_drafts
            .iter()
            .filter(|draft| run.draft_ids.contains(&draft.id))
            .collect::<Vec<_>>();
        return Ok(BetaGenerationResult {
            run_id: run.id.clone(),
            draft_ids: run.draft_ids.clone(),
            accepted_draft_ids: drafts
                .iter()
                .filter(|draft| {
                    draft.validation.status == GeneratedPromptValidationStatus::Accepted
                })
                .map(|draft| draft.id.clone())
                .collect(),
            rejected_draft_ids: drafts
                .iter()
                .filter(|draft| {
                    draft.validation.status == GeneratedPromptValidationStatus::Rejected
                })
                .map(|draft| draft.id.clone())
                .collect(),
            validation_failures: run.validation_failures.clone(),
        });
    }
    let model = request.model.clone().unwrap_or_else(|| provider.model());
    let mut validation_failures = Vec::new();
    let mut draft_ids = Vec::new();
    let mut accepted_draft_ids = Vec::new();
    let mut rejected_draft_ids = Vec::new();
    let mut usage: Option<GenerationRunUsage> = None;
    // Tracks the distinct models that actually produced drafts so the run
    // header is honest: one producer names it, several (a composite running
    // different sub-providers across sources) fall back to the declared model
    // rather than last-write-wins. Per-draft model stamping stays exact.
    let mut producing_models: Vec<GeneratedPromptModel> = Vec::new();

    save_generation_run_progress(
        store,
        &request,
        &model,
        RunProgress::Started,
        source_permission_receipts(&sources),
    )?;
    let mut seen_signatures =
        existing_accepted_candidate_signatures(&snapshot, Some(&request.run_id));
    for source in &sources {
        let source_usage = process_generation_source(
            store,
            provider,
            &request,
            source,
            &mut seen_signatures,
            &mut validation_failures,
            &mut draft_ids,
            &mut accepted_draft_ids,
            &mut rejected_draft_ids,
            &mut producing_models,
        )?;
        usage = merge_usage(usage, source_usage.drafts);
        usage = merge_usage(usage, source_usage.repair);
        save_generation_run_progress(
            store,
            &request,
            &model,
            RunProgress::InProgress {
                draft_ids: draft_ids.clone(),
                validation_failures: validation_failures.clone(),
                usage: usage.clone(),
            },
            source_permission_receipts(&sources),
        )?;
    }

    let run_model = match producing_models.as_slice() {
        [single] => single.clone(),
        _ => model.clone(),
    };
    save_generation_run_progress(
        store,
        &request,
        &run_model,
        RunProgress::Completed {
            draft_ids: draft_ids.clone(),
            validation_failures: validation_failures.clone(),
            usage,
        },
        source_permission_receipts(&sources),
    )?;

    Ok(BetaGenerationResult {
        run_id: request.run_id,
        draft_ids,
        accepted_draft_ids,
        rejected_draft_ids,
        validation_failures,
    })
}

/// Draft-generation and repair usage for one source, including paid failures.
struct SourceGenerationUsage {
    drafts: Option<ProviderUsage>,
    repair: Option<ProviderUsage>,
}

/// Runs generation and one bounded repair for a source. A failed provider
/// response still returns its reported usage so rejected/truncated paid output
/// remains in the run receipt.
#[allow(clippy::too_many_arguments)]
fn process_generation_source<S>(
    store: &mut S,
    provider: &dyn DraftProvider,
    request: &BetaGenerationRequest,
    source: &SourceDocument,
    seen_signatures: &mut Vec<CandidateSignature>,
    validation_failures: &mut Vec<String>,
    draft_ids: &mut Vec<String>,
    accepted_draft_ids: &mut Vec<String>,
    rejected_draft_ids: &mut Vec<String>,
    producing_models: &mut Vec<GeneratedPromptModel>,
) -> Result<SourceGenerationUsage, BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    let drafts = match provider.generate_drafts(source) {
        Ok(drafts) => drafts,
        Err(failure) => {
            validation_failures.push(format!("{}: {failure}", source.id));
            return Ok(SourceGenerationUsage {
                drafts: failure.usage().cloned(),
                repair: None,
            });
        }
    };
    let drafts = enforce_content_policy(source, drafts);
    validation_failures.extend(drafts.failures);
    // Stamp drafts with the provider that actually generated them, not the
    // composite's declared identity. A caller override still wins.
    let source_model = request.model.clone().unwrap_or(drafts.model);
    if !drafts.candidates.is_empty() && !producing_models.contains(&source_model) {
        producing_models.push(source_model.clone());
    }

    // A claimed quote must verify; a quote-free card is model-expanded
    // knowledge, not a factual claim supported by the captured topic seed.
    let mut source_rejections = Vec::new();
    let mut max_candidate_index = 0;
    let learning_intent = drafts.learning_intent;
    persist_candidates(
        store,
        source,
        drafts.candidates,
        &mut PersistCandidatesContext {
            request,
            source_model: &source_model,
            seen_signatures,
            validation_failures,
            draft_ids,
            accepted_draft_ids,
            rejected_draft_ids,
            source_rejections: &mut source_rejections,
            max_candidate_index: &mut max_candidate_index,
            learning_intent,
        },
    )?;

    let repair_usage = try_repair_source(
        store,
        provider,
        source,
        request,
        &source_rejections,
        seen_signatures,
        validation_failures,
        draft_ids,
        accepted_draft_ids,
        rejected_draft_ids,
        producing_models,
        &mut max_candidate_index,
    )?;

    Ok(SourceGenerationUsage {
        drafts: drafts.usage,
        repair: repair_usage,
    })
}

fn load_sources<E>(
    snapshot: &BetaStoreSnapshot,
    source_document_ids: &[String],
) -> Result<Vec<SourceDocument>, BetaGenerationError<E>> {
    source_document_ids
        .iter()
        .map(|source_document_id| require_source(&snapshot.source_documents, source_document_id))
        .collect()
}

#[allow(clippy::too_many_arguments)]
fn try_repair_source<S>(
    store: &mut S,
    provider: &dyn DraftProvider,
    source: &SourceDocument,
    request: &BetaGenerationRequest,
    source_rejections: &[DraftRejection],
    seen_signatures: &mut Vec<CandidateSignature>,
    validation_failures: &mut Vec<String>,
    draft_ids: &mut Vec<String>,
    accepted_draft_ids: &mut Vec<String>,
    rejected_draft_ids: &mut Vec<String>,
    producing_models: &mut Vec<GeneratedPromptModel>,
    max_candidate_index: &mut usize,
) -> Result<Option<ProviderUsage>, BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    if source_rejections.is_empty() {
        return Ok(None);
    }
    let repair_rejections =
        &source_rejections[..source_rejections.len().min(MAX_REPAIR_REJECTIONS)];

    let repair = match provider.repair_drafts(source, repair_rejections) {
        Ok(Some(repair)) => repair,
        Ok(None) => return Ok(None),
        Err(failure) => {
            validation_failures.push(format!("{} repair: {failure}", source.id));
            return Ok(failure.usage().cloned());
        }
    };
    let repair = enforce_content_policy(source, repair);
    validation_failures.extend(repair.failures);
    let usage = repair.usage;
    let learning_intent = repair.learning_intent;
    let repair_model = request.model.clone().unwrap_or(repair.model);
    if !repair.candidates.is_empty() && !producing_models.contains(&repair_model) {
        producing_models.push(repair_model.clone());
    }
    let mut repair_candidates = repair
        .candidates
        .into_iter()
        .take(repair_rejections.len())
        .collect::<Vec<_>>();
    for (offset, candidate) in repair_candidates.iter_mut().enumerate() {
        candidate.index = *max_candidate_index + offset + 1;
    }
    let mut repair_rejections = Vec::new();
    persist_candidates(
        store,
        source,
        repair_candidates,
        &mut PersistCandidatesContext {
            request,
            source_model: &repair_model,
            seen_signatures,
            validation_failures,
            draft_ids,
            accepted_draft_ids,
            rejected_draft_ids,
            source_rejections: &mut repair_rejections,
            max_candidate_index,
            learning_intent,
        },
    )?;

    Ok(usage)
}

fn save_generation_run_progress<S>(
    store: &mut S,
    request: &BetaGenerationRequest,
    model: &GeneratedPromptModel,
    progress: RunProgress,
    source_permissions: Vec<SourcePermissionReceipt>,
) -> Result<(), BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    store
        .save_generation_run(run_receipt(request, model, progress, source_permissions))
        .map(|_| ())
        .map_err(BetaGenerationError::Store)
}

struct PersistCandidatesContext<'a> {
    request: &'a BetaGenerationRequest,
    source_model: &'a GeneratedPromptModel,
    seen_signatures: &'a mut Vec<CandidateSignature>,
    validation_failures: &'a mut Vec<String>,
    draft_ids: &'a mut Vec<String>,
    accepted_draft_ids: &'a mut Vec<String>,
    rejected_draft_ids: &'a mut Vec<String>,
    source_rejections: &'a mut Vec<DraftRejection>,
    max_candidate_index: &'a mut usize,
    learning_intent: Option<LearningIntent>,
}

fn persist_candidates<S>(
    store: &mut S,
    source: &SourceDocument,
    candidates: Vec<DraftCandidate>,
    context: &mut PersistCandidatesContext<'_>,
) -> Result<(), BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    for candidate in candidates {
        *context.max_candidate_index = (*context.max_candidate_index).max(candidate.index);
        // Preserve source lineage in both lanes without laundering a captured
        // topic seed into evidence for a generated factual claim.
        let cited = candidate
            .evidence
            .as_deref()
            .map(str::trim)
            .filter(|quote| !normalize_for_match(quote).is_empty());
        let grounded = cited.is_some();
        let evidence: &str = cited.unwrap_or_else(|| knowledge_seed(source));

        let signature = CandidateSignature::from_candidate(&candidate);
        let duplicate = context
            .seen_signatures
            .iter()
            .any(|seen| seen.duplicates(&signature));
        if duplicate {
            let reason = "Duplicate-ish generated draft".to_owned();
            context
                .validation_failures
                .push(format!("{} block {}: {reason}", source.id, candidate.index));
            context
                .source_rejections
                .push(candidate_rejection(&candidate, vec![reason]));
            continue;
        }

        let draft = persist_candidate(
            store,
            source,
            &candidate,
            evidence,
            &PersistParams {
                request: context.request,
                model: context.source_model,
                duplicate: false,
                grounded,
                quote_verified: !grounded || source_contains_quote(source, evidence),
                learning_intent: context.learning_intent,
            },
        )?;

        context.draft_ids.push(draft.id.clone());
        if draft.validation.status == GeneratedPromptValidationStatus::Accepted {
            context.seen_signatures.push(signature);
            context.accepted_draft_ids.push(draft.id);
        } else {
            context.source_rejections.push(candidate_rejection(
                &candidate,
                draft.validation.reasons.clone(),
            ));
            context.rejected_draft_ids.push(draft.id);
        }
    }

    Ok(())
}

/// Preview the shared trust gate before an asynchronous, single repair pass.
/// `drafts` must already have passed [`enforce_content_policy`]. No draft, run,
/// or learner decision is persisted; commit must run the gate again against
/// the current store after the asynchronous boundary.
#[must_use]
pub fn generation_repair_rejections(
    snapshot: &BetaStoreSnapshot,
    source: &SourceDocument,
    drafts: &ProviderDrafts,
) -> Vec<DraftRejection> {
    let mut seen = existing_accepted_candidate_signatures(snapshot, None);
    let mut rejections = Vec::new();
    for candidate in &drafts.candidates {
        let signature = CandidateSignature::from_candidate(candidate);
        let reasons = if seen.iter().any(|prior| prior.duplicates(&signature)) {
            vec!["Duplicate-ish generated draft".to_owned()]
        } else {
            let quote = candidate
                .evidence
                .as_deref()
                .map(str::trim)
                .filter(|quote| !normalize_for_match(quote).is_empty());
            source_candidate_reasons(
                candidate,
                false,
                quote.is_some(),
                quote.is_none_or(|quote| source_contains_quote(source, quote)),
            )
        };
        if reasons.is_empty() {
            seen.push(signature);
        } else if rejections.len() < MAX_REPAIR_REJECTIONS {
            rejections.push(candidate_rejection(candidate, reasons));
        }
    }
    rejections
}

fn candidate_rejection(candidate: &DraftCandidate, reasons: Vec<String>) -> DraftRejection {
    DraftRejection {
        index: candidate.index,
        concept: candidate.concept.clone(),
        question: candidate.question.clone(),
        answer: candidate.answer.clone(),
        reasons,
    }
}

/// Generate bridge material for one kept parent review unit.
///
/// Bridge generation is concept-backed rather than source-backed: if the
/// concept has no cached reference note, the provider writes one first; easier
/// bridge drafts cite that concept note and enter the queue with the supplied
/// due timestamp. The caller owns deferring the parent item.
///
/// # Errors
///
/// Returns [`BetaGenerationError`] when the parent is unknown or store writes
/// fail.
pub fn run_bridge_generation_with_provider<S>(
    store: &mut S,
    provider: &dyn BridgeMaterialProvider,
    request: BridgeGenerationRequest,
) -> Result<BridgeGenerationResult, BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    run_bridge_generation_internal(store, provider, request, true)
}

/// Generate bridge material with the deterministic local provider path.
///
/// # Errors
///
/// Returns [`BetaGenerationError`] when the parent is unknown or store writes
/// fail.
pub fn run_bridge_generation<S>(
    store: &mut S,
    request: BridgeGenerationRequest,
) -> Result<BridgeGenerationResult, BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    run_bridge_generation_internal(store, &FakeModelProvider, request, false)
}

fn run_bridge_generation_internal<S>(
    store: &mut S,
    provider: &dyn BridgeMaterialProvider,
    request: BridgeGenerationRequest,
    enforce_source_permission: bool,
) -> Result<BridgeGenerationResult, BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    let snapshot = store.snapshot().map_err(BetaGenerationError::Store)?;
    let context = bridge_generation_context::<S::Error>(&snapshot, &request.parent_review_unit_id)?;
    let (source_document_ids, source_permissions, authorization) =
        source_authorization_for_parent::<S::Error>(
            &snapshot,
            &context.parent,
            enforce_source_permission,
        )?;
    if let Some(run) = snapshot.generation_runs.iter().find(|run| {
        run.id == request.run_id && memory_engine_persistence::generation_run_is_published(run)
    }) {
        let mut drafts = BridgeDraftPersistence {
            draft_ids: Vec::new(),
            accepted_draft_ids: Vec::new(),
            accepted_review_unit_ids: Vec::new(),
            rejected_draft_ids: Vec::new(),
            validation_failures: run.validation_failures.clone(),
        };
        for draft in snapshot
            .generated_prompt_drafts
            .iter()
            .filter(|draft| run.draft_ids.contains(&draft.id))
        {
            record_bridge_draft(&mut drafts, draft);
        }
        return Ok(BridgeGenerationResult {
            run_id: run.id.clone(),
            concept_key: context.concept_key,
            reference_note_created: false,
            accepted_draft_ids: drafts.accepted_draft_ids,
            rejected_draft_ids: drafts.rejected_draft_ids,
            validation_failures: run.validation_failures.clone(),
        });
    }
    let provider_request = bridge_material_request(&snapshot, &context, authorization);
    let material = provider
        .generate_bridge_material(&provider_request)
        .map_err(|failure| BetaGenerationError::ProviderFailure(failure.to_string()))?;
    let model = request.model.as_ref().unwrap_or(&material.model);
    let (note, note_body, reference_note_created) =
        bridge_reference_note(&context, &material, model, request.started_at);

    let run_request = bridge_run_request(&request, &source_document_ids);
    store
        .save_generation_run(run_receipt(
            &run_request,
            model,
            RunProgress::Started,
            source_permissions.clone(),
        ))
        .map_err(BetaGenerationError::Store)?;
    store
        .save_concept_reference_note(note)
        .map_err(BetaGenerationError::Store)?;

    let bridge_drafts = save_bridge_drafts(
        store,
        &snapshot,
        material.candidates,
        &BridgeDraftBaseContext {
            run_id: &request.run_id,
            concept_key: &context.concept_key,
            reference_note_body: &note_body,
            model,
            due: request.default_due,
            created_at: request.started_at,
            parent_review_unit_id: &context.parent.review_unit_id,
            parent_progression: context.parent.queue.progression.as_ref(),
            parent_stage_order: context.parent_stage_order,
            domain_key: "bridge",
            supersede_parent: true,
            remediation_pack_id: None,
            source_document_ids: &source_document_ids,
            source_key: context.parent.queue.source_key.as_ref(),
        },
    )?;

    store
        .save_generation_run(run_receipt(
            &run_request,
            model,
            RunProgress::Completed {
                draft_ids: bridge_drafts.draft_ids,
                validation_failures: bridge_drafts.validation_failures.clone(),
                usage: material.usage.as_ref().map(provider_usage_to_run_usage),
            },
            source_permissions,
        ))
        .map_err(BetaGenerationError::Store)?;

    if bridge_drafts.accepted_draft_ids.is_empty() {
        return Err(BetaGenerationError::ProviderFailure(format!(
            "Bridge material produced no accepted drafts: {}",
            bridge_drafts.validation_failures.join("; ")
        )));
    }

    Ok(BridgeGenerationResult {
        run_id: request.run_id,
        concept_key: context.concept_key,
        reference_note_created,
        accepted_draft_ids: bridge_drafts.accepted_draft_ids,
        rejected_draft_ids: bridge_drafts.rejected_draft_ids,
        validation_failures: bridge_drafts.validation_failures,
    })
}

/// Generate one automatic remediation pack for a failed parent review
/// unit's exact attempt.
///
/// Idempotent by `request.attempt_id`: a duplicate request, retry, or
/// process restart naming the same attempt returns the existing pack
/// instead of generating a second one. Reuses the same provider boundary,
/// concept-note caching, duplicate detection, and draft-ladder enforcement
/// as [`run_bridge_generation_with_provider`] (the explicit, pre-answer
/// learner escape hatch), but never supersedes the parent — deferring and
/// returning it is the caller's job via `snooze_review_unit_until`, so
/// mastery can never bury it.
///
/// Zero accepted drafts is a data outcome, not an error: the returned pack
/// carries [`RemediationPackStatus::Rejected`] and the caller must not
/// defer the parent.
///
/// # Errors
///
/// Returns [`BetaGenerationError`] when the parent is unknown or store
/// reads or writes fail.
pub fn run_remediation_pack_generation_with_provider<S>(
    store: &mut S,
    provider: &dyn BridgeMaterialProvider,
    request: &RemediationPackGenerationRequest,
) -> Result<RemediationPackGenerationResult, BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    let snapshot = store.snapshot().map_err(BetaGenerationError::Store)?;
    if let Some(existing) = snapshot.remediation_packs.iter().find(|pack| {
        pack.attempt_id == request.attempt_id
            && snapshot.generation_runs.iter().any(|run| {
                run.id == request.run_id
                    && memory_engine_persistence::generation_run_is_published(run)
            })
    }) {
        return Ok(RemediationPackGenerationResult {
            pack: existing.clone(),
            accepted_draft_ids: Vec::new(),
            rejected_draft_ids: Vec::new(),
            validation_failures: Vec::new(),
        });
    }

    let pack_id = format!("remediation-pack:{}", request.attempt_id);
    let context = bridge_generation_context::<S::Error>(&snapshot, &request.parent_review_unit_id)?;
    let (source_document_ids, source_permissions, authorization) =
        source_authorization_for_parent::<S::Error>(&snapshot, &context.parent, true)?;
    let provider_request = bridge_material_request(&snapshot, &context, authorization);
    let material = provider
        .generate_bridge_material(&provider_request)
        .map_err(|failure| BetaGenerationError::ProviderFailure(failure.to_string()))?;
    let model = request.model.as_ref().unwrap_or(&material.model);
    let (note, note_body, _) =
        bridge_reference_note(&context, &material, model, request.started_at);

    let run_request = remediation_pack_run_request(request, &source_document_ids);
    store
        .save_generation_run(run_receipt(
            &run_request,
            model,
            RunProgress::Started,
            source_permissions.clone(),
        ))
        .map_err(BetaGenerationError::Store)?;
    store
        .save_concept_reference_note(note)
        .map_err(BetaGenerationError::Store)?;

    let pack_drafts = save_bridge_drafts(
        store,
        &snapshot,
        material.candidates,
        &BridgeDraftBaseContext {
            run_id: &request.run_id,
            concept_key: &context.concept_key,
            reference_note_body: &note_body,
            model,
            due: request.default_due,
            created_at: request.started_at,
            parent_review_unit_id: &context.parent.review_unit_id,
            parent_progression: context.parent.queue.progression.as_ref(),
            parent_stage_order: context.parent_stage_order,
            domain_key: "remediation",
            supersede_parent: false,
            remediation_pack_id: Some(&pack_id),
            source_document_ids: &source_document_ids,
            source_key: context.parent.queue.source_key.as_ref(),
        },
    )?;

    let status = if pack_drafts.accepted_draft_ids.is_empty() {
        RemediationPackStatus::Rejected
    } else {
        RemediationPackStatus::Active
    };
    let pack = store
        .save_remediation_pack(RemediationPackRecord {
            id: pack_id,
            parent_review_unit_id: request.parent_review_unit_id.clone(),
            attempt_id: request.attempt_id.clone(),
            concept_key: context.concept_key.clone(),
            review_unit_ids: pack_drafts.accepted_review_unit_ids.clone(),
            status,
            created_at: request.started_at,
            resolved_at: if status == RemediationPackStatus::Rejected {
                Some(request.started_at)
            } else {
                None
            },
        })
        .map_err(BetaGenerationError::Store)?;
    store
        .save_generation_run(run_receipt(
            &run_request,
            model,
            RunProgress::Completed {
                draft_ids: pack_drafts.draft_ids,
                validation_failures: pack_drafts.validation_failures.clone(),
                usage: material.usage.as_ref().map(provider_usage_to_run_usage),
            },
            source_permissions,
        ))
        .map_err(BetaGenerationError::Store)?;

    Ok(RemediationPackGenerationResult {
        pack,
        accepted_draft_ids: pack_drafts.accepted_draft_ids,
        rejected_draft_ids: pack_drafts.rejected_draft_ids,
        validation_failures: pack_drafts.validation_failures,
    })
}

fn remediation_pack_run_request(
    request: &RemediationPackGenerationRequest,
    source_document_ids: &[String],
) -> BetaGenerationRequest {
    BetaGenerationRequest {
        run_id: request.run_id.clone(),
        source_document_ids: source_document_ids.to_vec(),
        parent_review_unit_id: Some(request.parent_review_unit_id.clone()),
        started_at: request.started_at,
        completed_at: request.completed_at,
        default_due: request.default_due,
        model: request.model.clone(),
        pending: false,
    }
}

struct BridgeGenerationContext {
    parent: BetaReviewUnitRecord,
    concept_key: String,
    concept_label: String,
    cached_note: Option<ConceptReferenceNote>,
    parent_stage_order: u32,
}

/// Resolve a parent review unit's source documents into the trio every
/// bridge/remediation generation path needs: the source ids to stamp on the
/// generated run, their permission receipts, and the [`SourceAuthorizationContext`]
/// an arbitrary model provider must respect. `enforce` gates both requiring
/// provenance to exist and rejecting a `LocalOnly` source before any provider
/// call, matching the `_with_provider` (production) path; the deterministic
/// local path passes `false`.
fn source_authorization_for_parent<E>(
    snapshot: &BetaStoreSnapshot,
    parent: &BetaReviewUnitRecord,
    enforce: bool,
) -> Result<
    (
        Vec<String>,
        Vec<SourcePermissionReceipt>,
        SourceAuthorizationContext,
    ),
    BetaGenerationError<E>,
> {
    let source_documents = parent_source_documents::<E>(snapshot, parent, enforce)?;
    let source_document_ids = source_documents
        .iter()
        .map(|source| source.id.clone())
        .collect::<Vec<_>>();
    let source_permissions = source_permission_receipts(&source_documents);
    let authorization = SourceAuthorizationContext::from_sources(&source_documents).map_err(
        |error| match error {
            SourceAuthorizationError::ArchivedSourceDocument(id) => {
                BetaGenerationError::ArchivedSourceDocument(id)
            }
        },
    )?;
    if enforce {
        ensure_model_eligible_receipts(&source_permissions)?;
    }
    Ok((source_document_ids, source_permissions, authorization))
}

fn bridge_generation_context<E>(
    snapshot: &BetaStoreSnapshot,
    parent_review_unit_id: &ReviewUnitId,
) -> Result<BridgeGenerationContext, BetaGenerationError<E>> {
    let parent = snapshot
        .review_units
        .iter()
        .find(|unit| unit.review_unit_id == *parent_review_unit_id)
        .cloned()
        .ok_or_else(|| BetaGenerationError::UnknownReviewUnit(parent_review_unit_id.clone()))?;
    let parent_draft = snapshot
        .generated_prompt_drafts
        .iter()
        .find(|draft| draft.review_unit_id == parent.review_unit_id);
    let concept_key = parent
        .queue
        .concept_key
        .clone()
        .or_else(|| parent_draft.and_then(|draft| draft.queue.concept_key.clone()))
        .unwrap_or_else(|| parent.review_unit_id.as_str().to_owned());
    let cached_note = snapshot
        .concept_reference_notes
        .iter()
        .find(|note| note.concept_key == concept_key)
        .cloned();
    let parent_stage_order = parent
        .queue
        .progression
        .as_ref()
        .map_or(1, |progression| progression.stage_order);

    Ok(BridgeGenerationContext {
        parent,
        concept_label: concept_key.replace('-', " "),
        concept_key,
        cached_note,
        parent_stage_order,
    })
}

fn bridge_material_request(
    snapshot: &BetaStoreSnapshot,
    context: &BridgeGenerationContext,
    authorization: SourceAuthorizationContext,
) -> BridgeMaterialRequest {
    BridgeMaterialRequest::new(
        context.concept_key.clone(),
        context.concept_label.clone(),
        context.parent.review_unit_id.clone(),
        prompt_text(&context.parent.prompt),
        expected_answer(&context.parent.prompt),
        context.parent_stage_order,
        context.cached_note.as_ref().map(|note| note.body.clone()),
        recent_performance_for_concept(snapshot, &context.concept_key),
        authorization,
    )
}

/// Prepare actual source context and the cached concept note for an
/// asynchronous manual Bridge call, without invoking a provider or writing.
///
/// # Errors
/// Rejects unknown parents and missing, archived, or local-only provenance.
pub fn prepare_bridge_material<E>(
    snapshot: &BetaStoreSnapshot,
    parent_review_unit_id: &ReviewUnitId,
) -> Result<BridgeMaterialRequest, BetaGenerationError<E>> {
    let context = bridge_generation_context::<E>(snapshot, parent_review_unit_id)?;
    let (_, _, authorization) =
        source_authorization_for_parent::<E>(snapshot, &context.parent, true)?;
    Ok(bridge_material_request(snapshot, &context, authorization))
}

fn bridge_reference_note(
    context: &BridgeGenerationContext,
    material: &BridgeMaterial,
    model: &GeneratedPromptModel,
    created_at: i64,
) -> (ConceptReferenceNote, String, bool) {
    let created = context.cached_note.is_none();
    let body = context.cached_note.as_ref().map_or_else(
        || material.reference_note.body.clone(),
        |note| note.body.clone(),
    );
    let note = context
        .cached_note
        .clone()
        .unwrap_or_else(|| ConceptReferenceNote {
            concept_key: context.concept_key.clone(),
            title: material.reference_note.title.clone(),
            body: material.reference_note.body.clone(),
            model: model.clone(),
            created_at,
            updated_at: created_at,
        });

    (note, body, created)
}

struct BridgeDraftBaseContext<'a> {
    run_id: &'a str,
    concept_key: &'a str,
    reference_note_body: &'a str,
    model: &'a GeneratedPromptModel,
    due: i64,
    created_at: i64,
    parent_review_unit_id: &'a ReviewUnitId,
    parent_progression: Option<&'a ProgressionMetadata>,
    parent_stage_order: u32,
    domain_key: &'a str,
    supersede_parent: bool,
    remediation_pack_id: Option<&'a str>,
    source_document_ids: &'a [String],
    source_key: Option<&'a String>,
}

struct BridgeDraftPersistence {
    draft_ids: Vec<String>,
    accepted_draft_ids: Vec<String>,
    accepted_review_unit_ids: Vec<ReviewUnitId>,
    rejected_draft_ids: Vec<String>,
    validation_failures: Vec<String>,
}

fn save_bridge_drafts<S>(
    store: &mut S,
    snapshot: &BetaStoreSnapshot,
    candidates: Vec<DraftCandidate>,
    base: &BridgeDraftBaseContext<'_>,
) -> Result<BridgeDraftPersistence, BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    let mut result = BridgeDraftPersistence {
        draft_ids: Vec::new(),
        accepted_draft_ids: Vec::new(),
        accepted_review_unit_ids: Vec::new(),
        rejected_draft_ids: Vec::new(),
        validation_failures: Vec::new(),
    };
    let mut seen_signatures =
        existing_draft_signatures(snapshot.generated_prompt_drafts.iter().filter(|draft| {
            draft.generation_run_id.as_deref() != Some(base.run_id)
                && snapshot.generation_runs.iter().any(|run| {
                    draft.generation_run_id.as_deref() == Some(run.id.as_str())
                        && memory_engine_persistence::generation_run_is_published(run)
                })
        }));
    seen_signatures.extend(existing_review_unit_signatures(&snapshot.review_units));
    let mut drafts = Vec::new();

    for candidate in candidates {
        let signature = candidate_signature(&candidate);
        let duplicate = seen_signatures.contains(&signature);
        seen_signatures.insert(signature);
        let easier = stage_order(&candidate.activity_stage, &candidate.activity_kind)
            < base.parent_stage_order;
        let draft = bridge_draft(
            &candidate,
            &BridgeDraftContext {
                run_id: base.run_id,
                concept_key: base.concept_key,
                reference_note_body: base.reference_note_body,
                model: base.model,
                due: base.due,
                created_at: base.created_at,
                duplicate,
                easier,
                parent_review_unit_id: base.parent_review_unit_id,
                parent_progression: base.parent_progression,
                domain_key: base.domain_key,
                supersede_parent: base.supersede_parent,
                remediation_pack_id: base.remediation_pack_id,
                source_document_ids: base.source_document_ids,
                source_key: base.source_key,
            },
        );
        drafts.push(draft);
    }

    enforce_bridge_pack_ladder(&mut drafts, base.parent_stage_order);
    for draft in drafts {
        record_bridge_draft(&mut result, &draft);
        store
            .save_generated_prompt_draft(draft)
            .map_err(BetaGenerationError::Store)?;
    }

    Ok(result)
}

fn enforce_bridge_pack_ladder(drafts: &mut [GeneratedPromptDraft], parent_stage_order: u32) {
    let accepted_stage_orders = drafts
        .iter()
        .filter(|draft| draft.validation.status == GeneratedPromptValidationStatus::Accepted)
        .map(|draft| stage_order(&draft.activity_stage, &draft.activity_kind))
        .collect::<BTreeSet<_>>();
    let accepted_count = drafts
        .iter()
        .filter(|draft| draft.validation.status == GeneratedPromptValidationStatus::Accepted)
        .count();
    if accepted_count <= 1 || accepted_stage_orders.len() >= 2 || parent_stage_order <= 1 {
        return;
    }

    for draft in drafts
        .iter_mut()
        .filter(|draft| draft.validation.status == GeneratedPromptValidationStatus::Accepted)
    {
        draft.validation.status = GeneratedPromptValidationStatus::Rejected;
        draft
            .validation
            .reasons
            .push("Bridge drafts must form a scaffold ladder".to_owned());
        draft
            .critique_notes
            .push("Rejected: Bridge drafts must form a scaffold ladder".to_owned());
    }
}

fn record_bridge_draft(result: &mut BridgeDraftPersistence, draft: &GeneratedPromptDraft) {
    if draft.validation.status == GeneratedPromptValidationStatus::Accepted {
        result.accepted_draft_ids.push(draft.id.clone());
        result
            .accepted_review_unit_ids
            .push(draft.review_unit_id.clone());
    } else {
        result
            .validation_failures
            .extend(draft.validation.reasons.clone());
        result.rejected_draft_ids.push(draft.id.clone());
    }
    result.draft_ids.push(draft.id.clone());
}

fn bridge_run_request(
    request: &BridgeGenerationRequest,
    source_document_ids: &[String],
) -> BetaGenerationRequest {
    BetaGenerationRequest {
        run_id: request.run_id.clone(),
        source_document_ids: source_document_ids.to_vec(),
        parent_review_unit_id: Some(request.parent_review_unit_id.clone()),
        started_at: request.started_at,
        completed_at: request.completed_at,
        default_due: request.default_due,
        model: request.model.clone(),
        pending: false,
    }
}

struct PersistParams<'a> {
    request: &'a BetaGenerationRequest,
    model: &'a GeneratedPromptModel,
    duplicate: bool,
    grounded: bool,
    quote_verified: bool,
    learning_intent: Option<LearningIntent>,
}

/// Save one candidate's reference span and draft, returning the stored draft.
fn persist_candidate<S>(
    store: &mut S,
    source: &SourceDocument,
    candidate: &DraftCandidate,
    evidence: &str,
    params: &PersistParams<'_>,
) -> Result<GeneratedPromptDraft, BetaGenerationError<S::Error>>
where
    S: BetaGenerationStore,
{
    // A source-supported card cites a checked quote. A model-expanded card
    // retains its seed for lineage, explicitly labeled as non-evidence.
    let label = if params.grounded {
        format!("{} source evidence", candidate.concept)
    } else {
        format!("{} model-expanded input (not evidence)", candidate.concept)
    };
    let reference_span = store
        .save_reference_span(ReferenceSpan {
            id: generated_id(&params.request.run_id, "ref", &source.id, candidate),
            source_document_id: source.id.clone(),
            label,
            text: evidence.to_owned(),
            locator: format!("block:{}", candidate.index),
            created_at: params.request.started_at,
        })
        .map_err(BetaGenerationError::Store)?;

    store
        .save_generated_prompt_draft(build_draft(
            source,
            candidate,
            &DraftContext {
                run_id: &params.request.run_id,
                reference_span_id: &reference_span.id,
                model: params.model,
                due: params.request.default_due,
                created_at: params.request.started_at,
                duplicate: params.duplicate,
                grounded: params.grounded,
                quote_verified: params.quote_verified,
                learning_intent: params.learning_intent,
            },
        ))
        .map_err(BetaGenerationError::Store)
}

enum RunProgress {
    Started,
    InProgress {
        draft_ids: Vec<String>,
        validation_failures: Vec<String>,
        usage: Option<GenerationRunUsage>,
    },
    Completed {
        draft_ids: Vec<String>,
        validation_failures: Vec<String>,
        usage: Option<GenerationRunUsage>,
    },
}

fn run_receipt(
    request: &BetaGenerationRequest,
    model: &GeneratedPromptModel,
    progress: RunProgress,
    source_permissions: Vec<SourcePermissionReceipt>,
) -> GenerationRun {
    let (draft_ids, completed_at, validation_failures, usage) = match progress {
        RunProgress::Started => (
            Vec::new(),
            request.pending.then_some(i64::MIN),
            Vec::new(),
            None,
        ),
        RunProgress::InProgress {
            draft_ids,
            validation_failures,
            usage,
        } => (
            draft_ids,
            request.pending.then_some(i64::MIN),
            validation_failures,
            usage,
        ),
        RunProgress::Completed {
            draft_ids,
            validation_failures,
            usage,
        } => (
            draft_ids,
            Some(if request.pending {
                i64::MIN
            } else {
                request.completed_at.unwrap_or(request.started_at)
            }),
            validation_failures,
            usage,
        ),
    };

    GenerationRun {
        id: request.run_id.clone(),
        source_document_ids: request.source_document_ids.clone(),
        parent_review_unit_id: request.parent_review_unit_id.clone(),
        draft_ids,
        provider: model.provider.clone(),
        model: model.name.clone(),
        started_at: request.started_at,
        completed_at,
        validation_failures,
        usage,
        source_permissions,
        prompt_version: model.version.clone(),
    }
}

fn source_permission_receipts(sources: &[SourceDocument]) -> Vec<SourcePermissionReceipt> {
    sources
        .iter()
        .map(|source| SourcePermissionReceipt {
            source_document_id: source.id.clone(),
            permission: source.permission.clone(),
            consented: source.permission == SourcePermission::ModelEligible,
        })
        .collect()
}

fn ensure_model_eligible<E>(sources: &[SourceDocument]) -> Result<(), BetaGenerationError<E>> {
    ensure_sources_not_archived(sources)?;
    ensure_model_eligible_receipts(&source_permission_receipts(sources))
}

fn ensure_sources_not_archived<E>(
    sources: &[SourceDocument],
) -> Result<(), BetaGenerationError<E>> {
    if let Some(source) = sources.iter().find(|source| source.archived_at.is_some()) {
        return Err(BetaGenerationError::ArchivedSourceDocument(
            source.id.clone(),
        ));
    }
    Ok(())
}

fn ensure_model_eligible_receipts<E>(
    receipts: &[SourcePermissionReceipt],
) -> Result<(), BetaGenerationError<E>> {
    if let Some(receipt) = receipts
        .iter()
        .find(|receipt| receipt.permission == SourcePermission::LocalOnly)
    {
        return Err(BetaGenerationError::LocalOnlySource(
            receipt.source_document_id.clone(),
        ));
    }

    Ok(())
}

fn parent_source_documents<E>(
    snapshot: &BetaStoreSnapshot,
    parent: &BetaReviewUnitRecord,
    require_provenance: bool,
) -> Result<Vec<SourceDocument>, BetaGenerationError<E>> {
    let draft = snapshot
        .generated_prompt_drafts
        .iter()
        .find(|draft| draft.review_unit_id == parent.review_unit_id);
    let mut source_ids = draft
        .map(|draft| draft.source_document_ids.clone())
        .unwrap_or_default();
    if let Some(source_key) = parent.queue.source_key.as_ref() {
        if !source_ids.iter().any(|id| id == source_key) {
            source_ids.push(source_key.clone());
        }
    }
    if require_provenance && source_ids.is_empty() {
        return Err(BetaGenerationError::UnknownSourceDocument(
            parent.review_unit_id.to_string(),
        ));
    }
    let mut sources = Vec::with_capacity(source_ids.len());
    for source_id in &source_ids {
        let source = snapshot
            .source_documents
            .iter()
            .find(|source| &source.id == source_id)
            .ok_or_else(|| BetaGenerationError::UnknownSourceDocument(source_id.clone()))?;
        sources.push(source.clone());
    }
    ensure_sources_not_archived(&sources)?;
    Ok(sources)
}

struct DraftContext<'a> {
    run_id: &'a str,
    reference_span_id: &'a str,
    model: &'a GeneratedPromptModel,
    due: i64,
    created_at: i64,
    duplicate: bool,
    grounded: bool,
    quote_verified: bool,
    learning_intent: Option<LearningIntent>,
}

fn merge_usage(
    total: Option<GenerationRunUsage>,
    addition: Option<ProviderUsage>,
) -> Option<GenerationRunUsage> {
    let Some(addition) = addition else {
        return total;
    };
    let Some(total) = total else {
        return Some(provider_usage_to_run_usage(&addition));
    };
    Some(GenerationRunUsage {
        input_tokens: total.input_tokens.saturating_add(addition.input_tokens),
        output_tokens: total.output_tokens.saturating_add(addition.output_tokens),
        cost_usd_micros: total
            .cost_usd_micros
            .zip(addition.cost_usd_micros)
            .map(|(left, right)| left.saturating_add(right)),
        latency_ms: total.latency_ms.saturating_add(addition.latency_ms),
    })
}

fn provider_usage_to_run_usage(usage: &ProviderUsage) -> GenerationRunUsage {
    GenerationRunUsage {
        input_tokens: usage.input_tokens,
        output_tokens: usage.output_tokens,
        cost_usd_micros: usage.cost_usd_micros,
        latency_ms: usage.latency_ms,
    }
}

fn source_contains_quote(source: &SourceDocument, quote: &str) -> bool {
    evidence_quote_matches(source.body.as_deref().unwrap_or_default(), quote)
}

/// Minimum number of words an evidence quote must carry to count as proof.
///
/// A single-word quote like "the" substring-matches almost any prose, so it
/// proves nothing about a fabricated answer; requiring a short phrase closes
/// that hole. Terse captures are the exception: when the entire source is one
/// or two words, the source can support itself exactly.
const MIN_EVIDENCE_WORDS: usize = 3;
const MAX_REPAIR_REJECTIONS: usize = 4;

/// Whether an evidence quote is substantive and appears in the source text
/// after folding case, punctuation, and whitespace.
///
/// Cheap models paraphrase formatting even when quoting faithfully, so exact
/// matching would reject grounded drafts — hence the normalization. A quote
/// shorter than the minimum evidence-word threshold is accepted only when it
/// equals the complete normalized source text, which lets a one-word capture be
/// studied without treating tiny substrings as proof. This is the same predicate
/// the generation trust gate applies, exported so eval judges score exactly what
/// production enforces.
#[must_use]
pub fn evidence_quote_matches(source_text: &str, quote: &str) -> bool {
    let quote = normalize_for_match(quote);
    if quote.is_empty() {
        return false;
    }

    let source = normalize_for_match(source_text);
    if quote.split(' ').filter(|word| !word.is_empty()).count() < MIN_EVIDENCE_WORDS {
        return quote == source;
    }

    token_sequence_present(&source, &quote)
}

fn token_sequence_present(text: &str, needle: &str) -> bool {
    !needle.is_empty()
        && text.match_indices(needle).any(|(start, _)| {
            (start == 0 || text.as_bytes()[start - 1] == b' ')
                && (start + needle.len() == text.len()
                    || text.as_bytes()[start + needle.len()] == b' ')
        })
}

/// A conservative lexical support floor, not a semantic entailment oracle.
/// Blocks unrelated quotes, unsupported quantities, and topic seeds cited as
/// evidence. Legitimate paraphrases can be rejected and repaired with source
/// wording; human-calibrated evals still judge factual correctness.
#[must_use]
pub fn answer_has_evidence_support(evidence: &str, answer: &str) -> bool {
    let evidence = normalize_for_match(evidence);
    let answer = normalize_for_match(answer);
    if answer.is_empty() || evidence.is_empty() {
        return false;
    }
    if token_sequence_present(&evidence, &answer) {
        return true;
    }
    let words: Vec<_> = answer
        .split_whitespace()
        .filter(|word| {
            !matches!(
                *word,
                "a" | "an"
                    | "the"
                    | "is"
                    | "are"
                    | "was"
                    | "were"
                    | "be"
                    | "been"
                    | "of"
                    | "to"
                    | "and"
                    | "or"
                    | "in"
                    | "on"
                    | "at"
                    | "for"
                    | "from"
                    | "by"
                    | "with"
                    | "that"
                    | "this"
                    | "it"
                    | "its"
                    | "they"
                    | "their"
                    | "as"
            )
        })
        .collect();
    if words.is_empty() {
        return false;
    }
    if words
        .iter()
        .any(|word| word.chars().any(char::is_numeric) && !token_sequence_present(&evidence, word))
    {
        return false;
    }
    let supported = words
        .iter()
        .filter(|word| token_sequence_present(&evidence, word))
        .count();
    supported.saturating_mul(5) >= words.len().saturating_mul(3)
}

fn normalize_for_match(text: &str) -> String {
    let mut normalized = String::with_capacity(text.len());
    let mut previous_space = true;
    for character in text.chars().flat_map(char::to_lowercase) {
        if character.is_alphanumeric() {
            normalized.push(character);
            previous_space = false;
        } else if !previous_space {
            normalized.push(' ');
            previous_space = true;
        }
    }
    while normalized.ends_with(' ') {
        normalized.pop();
    }

    normalized
}

fn require_source<E>(
    sources: &[SourceDocument],
    source_document_id: &str,
) -> Result<SourceDocument, BetaGenerationError<E>> {
    let source = sources
        .iter()
        .find(|candidate| candidate.id == source_document_id)
        .cloned()
        .ok_or_else(|| BetaGenerationError::UnknownSourceDocument(source_document_id.to_owned()))?;
    if source
        .body
        .as_deref()
        .is_none_or(|body| body.trim().is_empty())
    {
        return Err(BetaGenerationError::SourceDocumentHasNoTextBody(
            source_document_id.to_owned(),
        ));
    }

    Ok(source)
}

fn build_draft(
    source: &SourceDocument,
    candidate: &DraftCandidate,
    context: &DraftContext<'_>,
) -> GeneratedPromptDraft {
    let unit_id = ReviewUnitId::new(generated_id(
        "generated",
        activity_kind_slug(&candidate.activity_kind),
        &source.id,
        candidate,
    ));
    let reasons = source_candidate_reasons(
        candidate,
        context.duplicate,
        context.grounded,
        context.quote_verified,
    );
    let status = if reasons.is_empty() {
        GeneratedPromptValidationStatus::Accepted
    } else {
        GeneratedPromptValidationStatus::Rejected
    };
    let critique_notes = if status == GeneratedPromptValidationStatus::Accepted {
        if context.grounded {
            vec![format!("Source quotation verified in {}; answer support checked lexically, not independently fact-checked.", context.reference_span_id)]
        } else {
            vec![format!("Model-expanded from input \"{}\"; the captured seed is not evidence for this answer.", source.title)]
        }
    } else {
        reasons
            .iter()
            .map(|reason| format!("Rejected: {reason}"))
            .collect()
    };

    GeneratedPromptDraft {
        learner_decision: None,
        id: generated_id(context.run_id, "draft", &source.id, candidate),
        source_document_ids: vec![source.id.clone()],
        reference_span_ids: vec![context.reference_span_id.to_owned()],
        concept_reference_note_key: None,
        generation_run_id: Some(context.run_id.to_owned()),
        review_unit_id: unit_id.clone(),
        prompt_id: format!("{unit_id}-prompt"),
        prompt: build_prompt(candidate, &unit_id, context.learning_intent),
        queue: PersistedQueueCandidate {
            review_unit_id: unit_id.clone(),
            due: context.due,
            lifecycle: ReviewUnitLifecycle::active().with_ttl_expires_at(source.ttl_expires_at),
            progression: Some(ProgressionMetadata {
                progression_group: Some(slug(&candidate.concept)),
                stage_order: stage_order(&candidate.activity_stage, &candidate.activity_kind),
                requires: Vec::new(),
                supersedes: Vec::new(),
            }),
            concept_key: Some(slug(&candidate.concept)),
            source_key: Some(source.id.clone()),
            domain_key: Some(source_kind_key(&source.kind).to_owned()),
        },
        activity_kind: candidate.activity_kind.clone(),
        activity_stage: candidate.activity_stage.clone(),
        worked_solution: candidate.worked_solution.clone(),
        model: context.model.clone(),
        validation: GeneratedPromptValidation { status, reasons },
        critique_notes,
        remediation_pack_id: None,
        created_at: context.created_at,
    }
}

struct BridgeDraftContext<'a> {
    run_id: &'a str,
    concept_key: &'a str,
    reference_note_body: &'a str,
    model: &'a GeneratedPromptModel,
    due: i64,
    created_at: i64,
    duplicate: bool,
    easier: bool,
    parent_review_unit_id: &'a ReviewUnitId,
    parent_progression: Option<&'a ProgressionMetadata>,
    domain_key: &'a str,
    supersede_parent: bool,
    remediation_pack_id: Option<&'a str>,
    source_document_ids: &'a [String],
    source_key: Option<&'a String>,
}

fn bridge_draft(
    candidate: &DraftCandidate,
    context: &BridgeDraftContext<'_>,
) -> GeneratedPromptDraft {
    // Remediation members are scoped by pack lineage (`remediation-pack:{attempt_id}`),
    // not the constant "remediation" domain key, so a later distinct failed
    // attempt against the same parent mints genuinely new review-unit ids
    // instead of colliding with an earlier, already-resolved pack's members.
    // Bridge material (no pack id) keeps its original domain-scoped id.
    let unit_id_prefix = context.remediation_pack_id.unwrap_or(context.domain_key);
    let unit_id = ReviewUnitId::new(bridge_generated_id(
        unit_id_prefix,
        activity_kind_slug(&candidate.activity_kind),
        context.parent_review_unit_id,
        candidate,
    ));
    let reasons = bridge_validation_reasons(candidate, context);
    let status = if reasons.is_empty() {
        GeneratedPromptValidationStatus::Accepted
    } else {
        GeneratedPromptValidationStatus::Rejected
    };
    let critique_notes = if status == GeneratedPromptValidationStatus::Accepted {
        vec![format!(
            "Study material derived from concept reference note {}; model explanation is not independent source evidence.",
            context.concept_key
        )]
    } else {
        reasons
            .iter()
            .map(|reason| format!("Rejected: {reason}"))
            .collect()
    };
    let stage_order = stage_order(&candidate.activity_stage, &candidate.activity_kind);
    let parent_group = context
        .parent_progression
        .and_then(|progression| progression.progression_group.clone())
        .unwrap_or_else(|| context.concept_key.to_owned());

    GeneratedPromptDraft {
        learner_decision: None,
        id: bridge_generated_id(
            context.run_id,
            "draft",
            context.parent_review_unit_id,
            candidate,
        ),
        source_document_ids: context.source_document_ids.to_vec(),
        reference_span_ids: Vec::new(),
        concept_reference_note_key: Some(context.concept_key.to_owned()),
        generation_run_id: Some(context.run_id.to_owned()),
        review_unit_id: unit_id.clone(),
        prompt_id: format!("{unit_id}-prompt"),
        // Bridge material is an easier derivative, not the original source's
        // verbatim intent; keep its exercise tolerant unless a future typed
        // bridge contract explicitly opts into recitation.
        prompt: build_prompt(candidate, &unit_id, None),
        queue: PersistedQueueCandidate {
            review_unit_id: unit_id.clone(),
            due: context.due.saturating_add(i64::from(stage_order)),
            lifecycle: ReviewUnitLifecycle::active(),
            progression: Some(ProgressionMetadata {
                progression_group: Some(parent_group),
                stage_order,
                requires: Vec::new(),
                supersedes: if context.supersede_parent {
                    vec![context.parent_review_unit_id.clone()]
                } else {
                    Vec::new()
                },
            }),
            concept_key: Some(context.concept_key.to_owned()),
            source_key: context
                .source_key
                .cloned()
                .or_else(|| context.source_document_ids.first().cloned()),
            domain_key: Some(context.domain_key.to_owned()),
        },
        activity_kind: candidate.activity_kind.clone(),
        activity_stage: candidate.activity_stage.clone(),
        worked_solution: candidate.worked_solution.clone(),
        model: context.model.clone(),
        validation: GeneratedPromptValidation { status, reasons },
        critique_notes,
        remediation_pack_id: context.remediation_pack_id.map(str::to_owned),
        created_at: context.created_at,
    }
}

fn bridge_validation_reasons(
    candidate: &DraftCandidate,
    context: &BridgeDraftContext<'_>,
) -> Vec<String> {
    let mut reasons = Vec::new();
    if candidate.unsupported {
        reasons.push("Unsupported by bridge context".to_owned());
    }
    if context.reference_note_body.trim().is_empty() {
        reasons.push("Bridge drafts require a concept reference note".to_owned());
    }
    if !context.easier {
        reasons.push("Bridge drafts must target a lower progression stage".to_owned());
    }
    if context.duplicate {
        reasons.push("Duplicate-ish generated draft".to_owned());
    }
    reasons.extend(candidate_quality_reasons(candidate));

    reasons
}

fn build_prompt(
    candidate: &DraftCandidate,
    review_unit_id: &ReviewUnitId,
    learning_intent: Option<LearningIntent>,
) -> Prompt {
    if candidate.activity_kind == GeneratedLearningActivityKind::Quiz
        && candidate.distractors.len() >= 2
    {
        return Prompt::Mcq {
            review_unit_id: review_unit_id.clone(),
            prompt: candidate.question.clone(),
            choices: unique(
                std::iter::once(candidate.answer.clone())
                    .chain(candidate.distractors.clone())
                    .collect(),
            ),
            correct_choice: candidate.answer.clone(),
        };
    }

    Prompt::Exact(ExactPrompt {
        kind: match learning_intent {
            Some(LearningIntent::VerbatimMemorization) => ExactPromptKind::Recitation,
            Some(
                LearningIntent::EnumerableSet
                | LearningIntent::ConceptUnderstanding
                | LearningIntent::FactRecall
                | LearningIntent::ProcedureProcess,
            )
            | None => ExactPromptKind::ShortAnswer,
        },
        review_unit_id: review_unit_id.clone(),
        prompt: candidate.question.clone(),
        accepted_answers: vec![candidate.answer.clone()],
        equivalence_groups: Vec::new(),
        ignored_tokens: vec![
            ".".to_owned(),
            ",".to_owned(),
            ";".to_owned(),
            ":".to_owned(),
        ],
    })
}

fn validation_reasons(
    candidate: &DraftCandidate,
    duplicate: bool,
    grounded: bool,
    quote_verified: bool,
) -> Vec<String> {
    let mut reasons = Vec::new();
    // A generator-flagged "unsupported" card is rejected in BOTH lanes: the flag
    // is a quality red flag, not a provenance claim, so it must bite even for a
    // quote-free world-knowledge card (otherwise omitting the quote would launder
    // an explicitly-flagged-bad answer into the queue).
    if candidate.unsupported {
        reasons.push("Unsupported by cited source material".to_owned());
    }
    // The quote check applies only to a card that CLAIMS a source quote: it must
    // verify, or the card is rejected — a fabricated citation is the
    // anti-hallucination failure this catches. A world-knowledge card has no
    // quote to verify; the source seed is lineage, not proof of factual correctness.
    if grounded && !quote_verified {
        reasons.push("Evidence quote not found in cited source".to_owned());
    }
    if duplicate {
        reasons.push("Duplicate-ish generated draft".to_owned());
    }
    reasons.extend(candidate_quality_reasons(candidate));

    reasons
}

fn source_candidate_reasons(
    candidate: &DraftCandidate,
    duplicate: bool,
    grounded: bool,
    quote_verified: bool,
) -> Vec<String> {
    let mut reasons = validation_reasons(candidate, duplicate, grounded, quote_verified);
    if grounded
        && quote_verified
        && candidate
            .evidence
            .as_deref()
            .is_some_and(|quote| !answer_has_evidence_support(quote, &candidate.answer))
    {
        reasons.push("Cited quote does not support the answer".to_owned());
    }
    reasons
}

/// Provider-independent retrieval quality gate, shared with Bridge and evals.
/// Mechanical defects are rejected; plausibility and deeper factual entailment
/// remain explicit human-calibrated quality judgments.
#[must_use]
pub fn candidate_quality_reasons(candidate: &DraftCandidate) -> Vec<String> {
    let mut reasons = Vec::new();
    if candidate.concept.trim().is_empty()
        || candidate.concept.split_whitespace().count() > 12
        || candidate.concept.len() > 160
        || candidate.question.trim().is_empty()
        || candidate.question.len() > 1_024
        || candidate.answer.trim().is_empty()
        || candidate.answer.len() > 2_048
        || candidate
            .worked_solution
            .as_ref()
            .is_some_and(|text| text.len() > 8_000)
        || candidate
            .evidence
            .as_ref()
            .is_some_and(|text| text.len() > 8_000)
    {
        reasons.push("Draft fields are empty or exceed the bounded content size".to_owned());
    }
    if references_source_artifact(&candidate.question) {
        reasons.push("Question references the source instead of standing alone".to_owned());
    }
    if token_sequence_present(
        &normalize_for_match(&candidate.question),
        &normalize_for_match(&candidate.answer),
    ) {
        reasons.push("Question gives away the answer".to_owned());
    }
    if candidate.activity_kind == GeneratedLearningActivityKind::Exercise
        && candidate
            .worked_solution
            .as_deref()
            .is_none_or(|text| text.trim().is_empty())
    {
        reasons.push("Exercises require a worked solution".to_owned());
    }
    reasons.extend(mcq_quality_reasons(candidate));
    reasons
}

/// A card must stand alone — the learner sees only the question, never the
/// material it was generated from. A question that points back at the study
/// artifact ("the subject of the source text", "according to the passage") is
/// unanswerable once the source is gone, and is exactly the meta-card the
/// generation prompt is told to avoid. The model usually obeys; this gate
/// rejects the ones that slip through so the repair pass regenerates a
/// self-contained card. Phrases mirror the prompt's own forbidden list and are
/// multi-word to keep a legitimate bare "text"/"source" from tripping it.
///
/// Exported so the generation bench eval scores self-reference with the exact
/// same definition the runtime gate enforces. A private copy in the bench crate
/// had silently drifted to a shorter phrase list, which let the eval pass cards
/// (e.g. "the list above") that the runtime gate actually rejects.
#[must_use]
pub fn references_source_artifact(question: &str) -> bool {
    const ARTIFACT_PHRASES: &[&str] = &[
        "source text",
        "the source material",
        "subject of the source",
        "subject of the text",
        "the passage",
        "the excerpt",
        "the list above",
        "the text above",
        "the table above",
        "the article above",
        "the document above",
        "the reading above",
        "according to the source",
        "according to the passage",
        "according to the text",
        "according to the article",
        "in the passage",
    ];
    let normalized = normalize_for_match(question);
    ARTIFACT_PHRASES
        .iter()
        .any(|phrase| normalized.contains(phrase))
}

fn mcq_quality_reasons(candidate: &DraftCandidate) -> Vec<String> {
    if candidate.activity_kind != GeneratedLearningActivityKind::Quiz
        || candidate.distractors.is_empty()
    {
        return Vec::new();
    }

    let mut reasons = Vec::new();
    if compound_question(&candidate.question) {
        reasons.push("MCQ question tests multiple atoms".to_owned());
    }

    if !(2..=3).contains(&candidate.distractors.len()) {
        reasons.push("MCQ requires 2-3 plausible distractors".to_owned());
    }
    let mut seen = BTreeSet::new();
    let answer = normalize_for_match(&candidate.answer);
    for distractor in &candidate.distractors {
        let normalized = normalize_for_match(distractor);
        if normalized.is_empty() {
            reasons.push("MCQ contains an empty distractor".to_owned());
            continue;
        }
        if normalized == answer {
            reasons.push("MCQ distractor duplicates the correct answer".to_owned());
        }
        if !seen.insert(normalized.clone()) {
            reasons.push("MCQ repeats a distractor".to_owned());
        }
        if matches!(
            normalized.as_str(),
            "all of the above" | "none of the above"
        ) {
            reasons.push("MCQ uses a catch-all distractor".to_owned());
        }
    }
    let options: Vec<_> = std::iter::once(candidate.answer.as_str())
        .chain(candidate.distractors.iter().map(String::as_str))
        .collect();
    for (index, option) in options.iter().enumerate() {
        if let Some((low, high)) = numeric_interval(option) {
            if options[..index]
                .iter()
                .filter_map(|other| numeric_interval(other))
                .any(|(other_low, other_high)| low <= other_high && other_low <= high)
            {
                reasons.push("MCQ options contain overlapping numeric ranges".to_owned());
            }
        }
    }
    let question = normalize_for_match(&candidate.question);
    if let Some(letter) = question
        .split_once("letter ")
        .and_then(|(_, rest)| rest.split_whitespace().next())
        .filter(|token| token.len() == 1 && token.as_bytes()[0].is_ascii_alphabetic())
    {
        if options
            .iter()
            .any(|option| !option.trim().to_ascii_lowercase().starts_with(letter))
        {
            reasons.push("MCQ distractors expose the answer through its keyed initial".to_owned());
        }
    }

    reasons.sort();
    reasons.dedup();
    reasons
}

fn numeric_interval(option: &str) -> Option<(f64, f64)> {
    let (left, right) = option
        .split_once(" to ")
        .or_else(|| option.split_once('–'))
        .or_else(|| option.split_once('—'))
        .or_else(|| {
            option.trim_start_matches('-').find('-').map(|index| {
                let index = index + usize::from(option.starts_with('-'));
                (&option[..index], &option[index + 1..])
            })
        })?;
    let low = left.trim().parse::<f64>().ok()?;
    let high = right.split_whitespace().next()?.parse::<f64>().ok()?;
    (low.is_finite() && high.is_finite() && low <= high).then_some((low, high))
}

fn compound_question(question: &str) -> bool {
    let normalized = normalize_for_match(question);
    (normalized.contains(" and ")
        || normalized.contains(" plus ")
        || normalized.contains(" along with "))
        && (normalized.contains("what ")
            || normalized.contains("which ")
            || normalized.contains("why ")
            || normalized.contains("how "))
}

fn existing_accepted_candidate_signatures(
    snapshot: &BetaStoreSnapshot,
    excluded_run: Option<&str>,
) -> Vec<CandidateSignature> {
    snapshot
        .generated_prompt_drafts
        .iter()
        .filter(|draft| {
            draft.validation.status == GeneratedPromptValidationStatus::Accepted
                && draft.generation_run_id.as_deref() != excluded_run
                && snapshot.generation_runs.iter().any(|run| {
                    draft.generation_run_id.as_deref() == Some(run.id.as_str())
                        && memory_engine_persistence::generation_run_is_published(run)
                })
        })
        .map(CandidateSignature::from_draft)
        .collect()
}

fn existing_draft_signatures<'a>(
    drafts: impl Iterator<Item = &'a GeneratedPromptDraft>,
) -> BTreeSet<String> {
    drafts
        .map(|draft| {
            [
                draft.queue.concept_key.clone().unwrap_or_default(),
                prompt_text(&draft.prompt).to_lowercase(),
                expected_answer(&draft.prompt).to_lowercase(),
            ]
            .join("\0")
        })
        .collect()
}

fn existing_review_unit_signatures(review_units: &[BetaReviewUnitRecord]) -> BTreeSet<String> {
    review_units
        .iter()
        .map(|unit| {
            [
                unit.queue.concept_key.clone().unwrap_or_default(),
                prompt_text(&unit.prompt).to_lowercase(),
                expected_answer(&unit.prompt).to_lowercase(),
            ]
            .join("\0")
        })
        .collect()
}

fn candidate_signature(candidate: &DraftCandidate) -> String {
    [
        slug(&candidate.concept),
        candidate.question.to_lowercase(),
        candidate.answer.to_lowercase(),
    ]
    .join("\0")
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct CandidateSignature {
    concept: String,
    question: String,
    answer: String,
}

impl CandidateSignature {
    fn from_candidate(candidate: &DraftCandidate) -> Self {
        Self {
            concept: slug(&candidate.concept),
            question: normalize_for_match(&candidate.question),
            answer: normalize_for_match(&candidate.answer),
        }
    }

    fn from_draft(draft: &GeneratedPromptDraft) -> Self {
        Self {
            concept: draft.queue.concept_key.clone().unwrap_or_default(),
            question: normalize_for_match(&prompt_text(&draft.prompt)),
            answer: normalize_for_match(&expected_answer(&draft.prompt)),
        }
    }

    fn duplicates(&self, other: &Self) -> bool {
        self.concept == other.concept
            && self.answer == other.answer
            && question_similarity(&self.question, &other.question) >= 0.8
    }
}

/// Cheap production duplicate predicate shared with the deterministic eval
/// harness so bench duplicate rates match runtime acceptance.
#[must_use]
pub fn candidates_duplicateish(left: &DraftCandidate, right: &DraftCandidate) -> bool {
    CandidateSignature::from_candidate(left).duplicates(&CandidateSignature::from_candidate(right))
}

fn question_similarity(left: &str, right: &str) -> f64 {
    let left = left.split_whitespace().collect::<BTreeSet<_>>();
    let right = right.split_whitespace().collect::<BTreeSet<_>>();
    let union = left.union(&right).count();
    if union == 0 {
        return 1.0;
    }
    #[allow(clippy::cast_precision_loss)]
    {
        left.intersection(&right).count() as f64 / union as f64
    }
}

fn recent_performance_for_concept(
    snapshot: &BetaStoreSnapshot,
    concept_key: &str,
) -> Vec<ReviewPerformanceContext> {
    snapshot
        .attempts
        .iter()
        .rev()
        .filter(|attempt| {
            snapshot
                .review_units
                .iter()
                .find(|unit| unit.review_unit_id == attempt.review_unit_id)
                .and_then(|unit| unit.queue.concept_key.as_ref())
                .is_some_and(|key| key == concept_key)
        })
        .take(5)
        .map(|attempt| ReviewPerformanceContext {
            review_unit_id: attempt.review_unit_id.to_string(),
            submitted_answer: attempt.submitted_answer.clone(),
            verdict: attempt
                .grade
                .as_ref()
                .map(|grade| format!("{:?}", grade.verdict).to_lowercase()),
        })
        .collect()
}

fn prompt_text(prompt: &Prompt) -> String {
    match prompt {
        Prompt::Mcq { prompt, .. }
        | Prompt::Boolean { prompt, .. }
        | Prompt::Exact(ExactPrompt { prompt, .. }) => prompt.clone(),
    }
}

fn expected_answer(prompt: &Prompt) -> String {
    match prompt {
        Prompt::Mcq { correct_choice, .. } => correct_choice.clone(),
        Prompt::Boolean { correct_answer, .. } => correct_answer.to_string(),
        Prompt::Exact(prompt) => prompt.accepted_answers.first().cloned().unwrap_or_default(),
    }
}

fn stage_order(stage: &str, activity_kind: &GeneratedLearningActivityKind) -> u32 {
    let normalized = stage.to_lowercase();
    if normalized.contains("bridge") && normalized.contains("recognition") {
        return 0;
    }
    if normalized.contains("bridge") && normalized.contains("cued") {
        return 1;
    }
    if normalized.contains("recognition") {
        return 1;
    }
    if normalized.contains("cued") {
        return 2;
    }
    if normalized.contains("free") {
        return 3;
    }
    if normalized.contains("composition") {
        return 4;
    }
    if activity_kind == &GeneratedLearningActivityKind::Exercise {
        5
    } else {
        1
    }
}

fn generated_id(prefix: &str, kind: &str, source_id: &str, candidate: &DraftCandidate) -> String {
    [
        prefix.to_owned(),
        kind.to_owned(),
        slug(source_id),
        candidate.index.to_string(),
        slug(&candidate.concept),
    ]
    .join("-")
}

fn bridge_generated_id(
    prefix: &str,
    kind: &str,
    parent_review_unit_id: &ReviewUnitId,
    candidate: &DraftCandidate,
) -> String {
    [
        prefix.to_owned(),
        kind.to_owned(),
        slug(parent_review_unit_id.as_str()),
        candidate.index.to_string(),
        slug(&candidate.concept),
    ]
    .join("-")
}

fn slug(value: &str) -> String {
    let mut slugged = String::new();
    let mut previous_dash = false;
    for character in value.chars().flat_map(char::to_lowercase) {
        if character.is_ascii_alphanumeric() {
            slugged.push(character);
            previous_dash = false;
        } else if !previous_dash && !slugged.is_empty() {
            slugged.push('-');
            previous_dash = true;
        }
    }
    while slugged.ends_with('-') {
        slugged.pop();
    }
    if slugged.is_empty() {
        "generated".to_owned()
    } else {
        slugged
    }
}

fn unique(values: Vec<String>) -> Vec<String> {
    let mut seen = BTreeSet::new();
    let mut result = Vec::new();
    for value in values {
        if seen.insert(value.to_lowercase()) {
            result.push(value);
        }
    }
    result
}

fn activity_kind_slug(kind: &GeneratedLearningActivityKind) -> &'static str {
    match kind {
        GeneratedLearningActivityKind::Quiz => "quiz",
        GeneratedLearningActivityKind::Exercise => "exercise",
    }
}

fn source_kind_key(kind: &SourceDocumentKind) -> &'static str {
    match kind {
        SourceDocumentKind::Text => "text",
        SourceDocumentKind::Link => "link",
        SourceDocumentKind::File => "file",
        SourceDocumentKind::Image => "image",
        SourceDocumentKind::VideoTranscript => "video-transcript",
    }
}

#[cfg(test)]
mod tests {
    use memory_engine_persistence::GeneratedLearningActivityKind;

    use super::{evidence_quote_matches, validation_reasons, DraftCandidate};

    const SOURCE: &str = "Photosynthesis occurs in the chloroplast and converts \
                          light into chemical energy stored as glucose.";

    fn quiz(question: &str, answer: &str, distractors: &[&str]) -> DraftCandidate {
        DraftCandidate {
            index: 1,
            concept: "Test concept".to_owned(),
            question: question.to_owned(),
            answer: answer.to_owned(),
            evidence: Some("Photosynthesis occurs in the chloroplast".to_owned()),
            distractors: distractors
                .iter()
                .map(|value| (*value).to_owned())
                .collect(),
            worked_solution: None,
            activity_kind: GeneratedLearningActivityKind::Quiz,
            activity_stage: "recognition".to_owned(),
            unsupported: false,
        }
    }

    #[test]
    fn substantive_verbatim_quote_matches() {
        assert!(evidence_quote_matches(SOURCE, "occurs in the chloroplast"));
    }

    #[test]
    fn match_tolerates_case_and_punctuation() {
        assert!(evidence_quote_matches(
            SOURCE,
            "OCCURS, in the! Chloroplast"
        ));
    }

    #[test]
    fn trivial_one_or_two_word_quote_is_rejected() {
        // The fabrication B1 closes: a real source word that proves nothing.
        assert!(!evidence_quote_matches(SOURCE, "the"));
        assert!(!evidence_quote_matches(SOURCE, "in the"));
    }

    #[test]
    fn terse_quote_matches_only_when_it_is_the_complete_source() {
        assert!(evidence_quote_matches("Mitochondria", "mitochondria"));
        assert!(evidence_quote_matches("NATO alphabet", "NATO alphabet"));
        assert!(!evidence_quote_matches(
            "Mitochondria generate ATP.",
            "mitochondria"
        ));
    }

    #[test]
    fn three_word_quote_is_the_floor() {
        assert!(evidence_quote_matches(SOURCE, "into chemical energy"));
    }

    #[test]
    fn empty_quote_is_rejected() {
        assert!(!evidence_quote_matches(SOURCE, ""));
        assert!(!evidence_quote_matches(SOURCE, "   "));
    }

    #[test]
    fn substantive_quote_absent_from_source_is_rejected() {
        assert!(!evidence_quote_matches(
            SOURCE,
            "stored as fructose molecules"
        ));
    }

    #[test]
    fn mcq_quality_rejects_compound_questions() {
        let candidate = quiz(
            "What do mitochondria generate and what process do they regulate?",
            "ATP and apoptosis",
            &["Glucose and mitosis", "RNA and transcription"],
        );

        let reasons = validation_reasons(&candidate, false, true, true);

        assert!(reasons.contains(&"MCQ question tests multiple atoms".to_owned()));
    }

    #[test]
    fn mcq_quality_rejects_answer_duplicate_distractors() {
        let candidate = quiz(
            "Where does photosynthesis occur?",
            "chloroplast",
            &["chloroplast", "mitochondrion"],
        );

        let reasons = validation_reasons(&candidate, false, true, true);

        assert!(reasons.contains(&"MCQ distractor duplicates the correct answer".to_owned()));
    }

    #[test]
    fn self_referential_meta_question_is_rejected() {
        // The exact garbage meta-card observed in dogfood: it asks about the
        // study artifact rather than the subject, so it is unanswerable once the
        // source is gone. The gate must reject it (the repair pass then writes a
        // standalone card) even though it carries a valid quote and no flags.
        let candidate = quiz(
            "What is the name of the phonetic alphabet presented as the subject of the source text?",
            "NATO phonetic alphabet",
            &["ICAO spelling alphabet", "ITU phonetic alphabet"],
        );

        let reasons = validation_reasons(&candidate, false, true, true);

        assert!(
            reasons
                .contains(&"Question references the source instead of standing alone".to_owned()),
            "self-referential meta-question must be rejected: {reasons:?}"
        );
    }

    #[test]
    fn standalone_question_naming_its_subject_is_accepted() {
        // The well-formed sibling: names its subject, mentions no artifact. It
        // must pass cleanly so the gate does not trip on a legitimate card that
        // merely contains words like "text" or "source" in other contexts.
        let candidate = quiz(
            "In the NATO phonetic alphabet, what code word represents the letter A?",
            "Alfa",
            &["Apple", "Alpha-One"],
        );

        let reasons = validation_reasons(&candidate, false, true, true);

        assert!(
            reasons.is_empty(),
            "a self-contained card must not be rejected: {reasons:?}"
        );
    }

    #[test]
    fn a_quote_cannot_match_only_a_prefix_of_a_source_word() {
        assert!(!evidence_quote_matches(
            "In the chloroplasts, light is absorbed.",
            "in the chloroplast"
        ));
        assert!(evidence_quote_matches(
            "In the chloroplast, light is absorbed.",
            "in the chloroplast"
        ));
    }

    #[test]
    fn unrelated_quotes_and_unsupported_numbers_fail_answer_support() {
        assert!(!super::answer_has_evidence_support(
            "NATO phonetic alphabet",
            "Alfa"
        ));
        assert!(!super::answer_has_evidence_support(
            "The pilot followed 24 adults.",
            "240 adults"
        ));
        assert!(super::answer_has_evidence_support(
            "The pilot followed 24 adults.",
            "24 adults"
        ));
        assert!(super::answer_has_evidence_support(
            "The no-cache directive allows storage but requires revalidation.",
            "It requires revalidation"
        ));
    }

    #[test]
    fn overlapping_numeric_mcq_options_are_rejected_but_disjoint_ranges_are_usable() {
        let invalid = quiz(
            "Which temperature interval is specified for the process?",
            "10 to 20",
            &["15 to 25", "30 to 40"],
        );
        let valid = quiz(
            "Which temperature interval is specified for the process?",
            "10 to 20",
            &["21 to 30", "31 to 40"],
        );
        assert!(!super::candidate_quality_reasons(&invalid).is_empty());
        assert!(super::candidate_quality_reasons(&valid).is_empty());
    }

    #[test]
    fn a_distractor_cannot_repeat_or_reveal_the_keyed_initial_answer() {
        let repeated = quiz(
            "Which cell structure carries out photosynthesis?",
            "chloroplast",
            &["mitochondrion", "Mitochondrion!"],
        );
        let clue = quiz(
            "In the NATO alphabet, which code word represents the letter A?",
            "Alfa",
            &["Bravo", "Charlie"],
        );
        let sound = quiz(
            "In the NATO alphabet, which code word represents the letter A?",
            "Alfa",
            &["Atlas", "Aster"],
        );
        assert!(!super::candidate_quality_reasons(&repeated).is_empty());
        assert!(!super::candidate_quality_reasons(&clue).is_empty());
        assert!(super::candidate_quality_reasons(&sound).is_empty());
    }

    #[test]
    fn recall_questions_must_not_embed_the_correct_answer() {
        let leaked = quiz(
            "Name the organelle chloroplast that carries out photosynthesis.",
            "chloroplast",
            &[],
        );
        let clean = quiz(
            "Which organelle carries out photosynthesis in plants?",
            "chloroplast",
            &[],
        );
        assert!(!super::candidate_quality_reasons(&leaked).is_empty());
        assert!(super::candidate_quality_reasons(&clean).is_empty());
    }
}
