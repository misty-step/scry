# Context Packet: Agent-Ready Architecture

## Spec

**Goal:** Split 4 monoliths (concepts.ts 1295L, aiGeneration.ts 1202L, iqc.ts 827L, review-chat.tsx 1389L) to <400 lines each. Extract, don't rewrite. Zero public API changes.

### What stays vs. what moves

**concepts.ts (1295 -> ~350):** Keeps all `mutation`/`query`/`internalMutation`/`internalQuery` wrappers (thin shells that delegate). Loses all private helper functions and lifecycle state machine logic.

**aiGeneration.ts (1202 -> ~320):** Keeps `processJob` and `generatePhrasingsForConcept` (the two `internalAction` exports) as orchestration shells. Loses pure validation/preparation functions, error classification, Langfuse tracing setup, and conflict scoring.

**iqc.ts (827 -> ~350):** Keeps `scanAndPropose` (internalAction), `applyActionCard`/`getOpenCards`/`rejectActionCard` (mutations/queries), and the 4 internal mutation stubs. Loses all pure helper functions (similarity, merge prompt building, payload construction, snapshot).

**review-chat.tsx (1389 -> ~350):** Keeps the `ReviewChat` component as a thin orchestrator. Loses `ActiveSession`, `ReviewStatsPanel`, `ActionPanelCard`, `PendingFeedbackCard`, `ChatMessage`, heatmap utilities, and the `useReviewSession` hook (extracted from the callback tangle).

---

## Decomposition Map

### concepts.ts -> Extracted Modules

#### 1. `convex/lib/conceptLifecycle.ts` (~180 lines)
Functions moving (with current line ranges):
- `updatePhrasingsBatched` (L64-97) — batched phrasing update utility
- `archiveConceptDoc` (L1176-1205) — archive logic
- `unarchiveConceptDoc` (L1207-1232) — unarchive logic
- `softDeleteConceptDoc` (L1234-1263) — soft delete logic
- `restoreConceptDoc` (L1265-1290) — restore logic
- `applyConceptBulkAction` (L1156-1174) — bulk action dispatch

Signature: all take `(ctx: MutationCtx, userId: Id<'users'>, concept: ConceptDoc)` and return `Promise<boolean>`. No Convex decorators needed -- pure helper functions operating on a passed-in `ctx`.

#### 2. `convex/lib/conceptReview.ts` (~120 lines)
Functions moving:
- `getDueHandler` (L180-249) — due concept selection + prioritization
- `selectActivePhrasing` (L468-504) — phrasing selection for review
- `getConceptsDueCountHandler` (L268-275) — due count from userStats
- `recordInteractionHandler` (L319-399) — FSRS scheduling + interaction recording
- `findActiveGenerationJob` (L919-945) — active job lookup

Signature: all take explicit `(ctx: QueryCtx | MutationCtx, userId, ...)`. No Convex decorators.

#### 3. `convex/lib/conceptScoring.ts` (~40 lines)
Functions moving:
- Conflict score recalculation logic duplicated in `archivePhrasing` (L768-773) and `unarchivePhrasing` (L837-843)

Extract a shared `recalculateConceptScores(activePhrasings, targetPhrasingsPerConcept)` pure function that returns `{ phrasingCount, thinScore, conflictScore }`.

**What remains in concepts.ts (~350 lines):**
- All `export const` mutations/queries: `createMany`, `getDue`, `getDueInternal`, `getConceptsDueCount`, `getConceptsDueCountInternal`, `getReviewDashboard`, `recordInteraction`, `recordInteractionInternal`, `recordFeedback`, `getConceptById` (internalQuery), `applyPhrasingGenerationUpdate` (internalMutation), `listForLibrary`, `getDetail`, `setCanonicalPhrasing`, `archivePhrasing`, `unarchivePhrasing`, `requestPhrasingGeneration`, `updateConcept`, `updatePhrasing`, `archiveConcept`, `unarchiveConcept`, `softDeleteConcept`, `restoreConcept`, `runBulkAction`
- Each becomes a thin shell: auth check + delegate to extracted helper
- Type aliases (`ConceptDoc`, `PhrasingDoc`, `SelectionResult`, constants)
- `__test` export

---

### aiGeneration.ts -> Extracted Modules

#### 1. `convex/lib/generationPipeline.ts` (~100 lines)
Functions moving:
- `GenerationPipelineError` class (L60-68) — custom error
- `classifyError` (L244-278) — error classification
- `calculateConflictScore` (L234-239) — conflict scoring
- `prepareConceptIdeas` (L79-160) — concept idea normalization/dedup
- `prepareGeneratedPhrasings` (L170-232) — phrasing validation/normalization
- Type exports: `ConceptPreparationStats`, `PreparedPhrasing` type (L162-168)

All are already pure functions with zero Convex runtime dependency. The `__test` export in aiGeneration.ts currently wraps `classifyError` and `calculateConflictScore` -- these become direct exports from the new module.

#### 2. `convex/lib/generationTracing.ts` (~80 lines)
Functions moving:
- Langfuse trace initialization pattern (repeated at L391-412 and L836-858)
- Langfuse span/generation creation helpers
- Trace completion and error update patterns (L631-638, L700-712, L1086-1133, L1186-1197)

Extract: `initGenerationTrace(config)`, `traceSpanForGeneration(trace, name, input, metadata)`, `finalizeTrace(trace, output)`, `finalizeTraceWithError(trace, error)`.

#### 3. `convex/lib/generationErrorHandler.ts` (~60 lines)
Functions moving:
- User-friendly error message mapping (L655-667, L1143-1154)
- Error logging pattern for Stage A (L669-688) and Stage B (L1172-1183)
- Job failure + analytics tracking pattern (L683-698, L1156-1171)

Extract: `handleGenerationFailure(ctx, { jobId, error, metadata, trace, startTime })`.

**What remains in aiGeneration.ts (~320 lines):**
- `processJob` internalAction (Stage A orchestration shell)
- `generatePhrasingsForConcept` internalAction (Stage B orchestration shell)
- Logger setup (L42-56)
- Each action becomes: init provider -> delegate to helpers -> handle result

---

### iqc.ts -> Extracted Modules

#### 1. `convex/lib/iqcHelpers.ts` (~120 lines)
Functions moving:
- `buildProposalKey` (L722-725) — deterministic pair key
- `computeTitleSimilarity` (L727-744) — Jaccard similarity
- `tokenizeTitle` (L746-754) — title tokenization
- `shouldConsiderMerge` (L756-773) — merge threshold logic
- `buildMergePayload` (L775-801) — payload construction
- `buildMergePrompt` (L695-720) — LLM prompt builder
- `snapshotConcept` (L803-813) — concept snapshot
- `accumulateStatDelta` (L815-827) — stat accumulation
- `mergeDecisionSchema` (L35-40) + `mergeActionPayloadSchema` (L44-77) — Zod schemas
- `IQC_SCAN_CONFIG` (L22-33) — configuration constants
- Type exports: `MergeCandidate` (L81-87), `MergeDecision` (L42)

All are already pure functions and Zod schemas. Zero Convex runtime dependency.

#### 2. `convex/lib/iqcAdjudication.ts` (~40 lines)
Functions moving:
- `adjudicateMergeCandidate` (L677-693) — LLM merge decision
- `fetchNeighborConcepts` (L647-675) — vector search + hydration

These require `ActionCtx` and `LanguageModel` as parameters but are not Convex-decorated. They are plain async functions passed their dependencies.

**What remains in iqc.ts (~350 lines):**
- `scanAndPropose` (internalAction) — orchestration shell
- `applyActionCard` (mutation) — merge execution
- `getOpenCards` (query) — card listing
- `rejectActionCard` (mutation) — card rejection
- 4 internal mutation stubs: `getRecentConceptSamples`, `getOpenActionCardsForUser`, `insertActionCard`, `getConceptById`
- Logger setup

---

### review-chat.tsx -> Extracted Components/Hooks

#### 1. `components/agent/hooks/use-review-session.ts` (~200 lines)
Extracted from the `ReviewChat` component (L161-694):
- All `useState` declarations (L163-178)
- All `useCallback` handlers: `loadNextQuestion` (L230-261), `handleStart` (L263-277), `handleSendChat` (L279-326), `handleAnswer` (L328-385), `handleRetryPendingFeedback` (L387-397), `executeReschedule` (L399-470), `handleKeyDown` (L472-480), `buildSuggestionPrompt` (L482-505), `handleSuggestionChip` (L507-601)
- `appendArtifact` callback (L206-216)
- Chat pending detection effect (L218-228)
- All Convex mutation/query bindings (L181-187)
- Messages subscription (L189-193)

Returns a typed `ReviewSessionState` object consumed by the thin `ReviewChat` shell.

#### 2. `components/agent/active-session.tsx` (~250 lines)
Extracted component:
- `ActiveSession` function component (L697-1054) — the split-layout session view
- Timeline item computation (L779-802)
- Message visibility logic (L752-766)

#### 3. `components/agent/action-panel-card.tsx` (~90 lines)
Extracted component:
- `ActionPanelCard` (L1068-1153) — weak areas / rescheduled / notice cards

#### 4. `components/agent/pending-feedback-card.tsx` (~50 lines)
Extracted component:
- `PendingFeedbackCard` (L1155-1212) — submission pending/failed state

#### 5. `components/agent/chat-message.tsx` (~50 lines)
Extracted component:
- `ChatMessage` (L1214-1253) — user/assistant message rendering

#### 6. `components/agent/review-stats-panel.tsx` (~140 lines)
Extracted component:
- `ReviewStatsPanel` (L1287-1389) — start screen stats panel
- `generateHeatmapCells` (L1265-1285) — heatmap utility
- `HEATMAP_LEVELS` constant (L1257-1263)

#### 7. `components/agent/review-chat-types.ts` (~70 lines)
Extracted type definitions:
- All interface/type declarations (L30-159): `SuggestionChip`, `ChatIntent`, `SendChatOptions`, `ReviewFeedbackData`, `ReviewFeedbackState`, `ActionPanelState`, `PendingFeedbackState`, `ActiveQuestionState`, `RescheduleTarget`, `ActionReplyState`, `RescheduleMutationResult`, `WeakAreasMutationResult`, `ArtifactEntry`
- Constants: `MAX_VISIBLE_CHAT_MESSAGES`, `MAX_ARTIFACT_ENTRIES`, `SUGGESTION_CHIPS`

**What remains in review-chat.tsx (~120 lines):**
- `ReviewChat` component: pre-session screen (start button + stats panel) + delegates to `ActiveSession` via `useReviewSession` hook
- `formatActionDate` utility (L1058-1066, or move to types file)

---

## Import Rewiring

### concepts.ts extractions

| File | Current import | New import |
|------|---------------|------------|
| `tests/convex/concepts.archive.test.ts` | `from '@/convex/concepts'` | No change (still exports `unarchivePhrasing`) |
| `tests/convex/concepts.test.ts` | `import * as conceptsModule` | No change (`__test` still exported) |
| `tests/convex/concepts.detail.test.ts` | `from '@/convex/concepts'` | No change (public exports stay) |
| `tests/convex/concepts.update.test.ts` | `from '@/convex/concepts'` | No change |
| `tests/convex/coverage-regressions.test.ts` | `from '@/convex/concepts'` | No change |

No external files import the private helpers being extracted. All `internal.concepts.*` references remain valid since the Convex-decorated functions stay in `concepts.ts`.

### aiGeneration.ts extractions

| File | Current import | New import |
|------|---------------|------------|
| `convex/evals/runner.ts` | `from '../aiGeneration'` | `from '../lib/generationPipeline'` |
| `tests/convex/aiGeneration.test.ts` | `from '../../convex/aiGeneration'` | `from '../../convex/lib/generationPipeline'` |
| `tests/convex/aiGeneration.prep.test.ts` | `from '@/convex/aiGeneration'` | `from '@/convex/lib/generationPipeline'` |
| `tests/convex/aiGeneration.process.test.ts` | `from '@/convex/aiGeneration'` | Keep for `processJob`/`generatePhrasingsForConcept`; add new import for extracted pure fns |

**Re-export option:** To minimize churn, `aiGeneration.ts` can re-export from `generationPipeline.ts`:
```ts
export { prepareConceptIdeas, prepareGeneratedPhrasings } from './lib/generationPipeline';
```
This preserves all existing imports. Remove the re-exports in a follow-up cleanup.

### iqc.ts extractions

| File | Current import | New import |
|------|---------------|------------|
| `convex/iqc.test.ts` | `from './iqc'` | `from './lib/iqcHelpers'` for pure functions |

**Re-export option:** Same as aiGeneration -- `iqc.ts` can re-export the pure helpers to minimize test churn.

### review-chat.tsx extractions

| File | Current import | New import |
|------|---------------|------------|
| `app/agent/page.tsx` | `from '@/components/agent/review-chat'` | No change (`ReviewChat` stays) |

All extracted components are internal to the `components/agent/` directory. No external import rewiring needed.

---

## Convex Constraints

### Internal functions that MUST stay in their original file

Convex's `internal.*` API surface is derived from file paths. An `internalMutation` exported from `convex/iqc.ts` is referenced as `internal.iqc.functionName`. Moving it to another file changes the path and breaks all callers.

**concepts.ts -- must stay:**
- `createMany` (internalMutation) -- called by `aiGeneration.ts` L576
- `getConceptById` (internalQuery) -- called by `aiGeneration.ts` L764
- `applyPhrasingGenerationUpdate` (internalMutation) -- called by `aiGeneration.ts` L1045
- `getDueInternal` (internalQuery) -- called by `reviewAgent.ts` L53, `reviewStreaming.ts` L98
- `getConceptsDueCountInternal` (internalQuery) -- called by `reviewAgent.ts` L120
- `recordInteractionInternal` (internalMutation) -- called by `reviewAgent.ts` L87, `reviewStreaming.ts` L120

**aiGeneration.ts -- must stay:**
- `processJob` (internalAction) -- called by `generationJobs.ts` L69 via scheduler
- `generatePhrasingsForConcept` (internalAction) -- called by `concepts.ts` L910 via scheduler, self-scheduled at L609-617

**iqc.ts -- must stay:**
- `scanAndPropose` (internalAction) -- called by `cron.ts` L47
- `getRecentConceptSamples` (internalMutation) -- called by self at L120
- `getOpenActionCardsForUser` (internalMutation) -- called by self at L177
- `insertActionCard` (internalMutation) -- called by self at L280
- `getConceptById` (internalMutation) -- called by self at L668

### Scheduler calls that constrain extraction

| Caller | Target | Reference |
|--------|--------|-----------|
| `concepts.ts` L910 | `internal.aiGeneration.generatePhrasingsForConcept` | File path locked |
| `aiGeneration.ts` L609 | `internal.aiGeneration.generatePhrasingsForConcept` | Self-reference, file path locked |
| `generationJobs.ts` L69 | `internal.aiGeneration.processJob` | File path locked |
| `cron.ts` L47 | `internal.iqc.scanAndPropose` | File path locked |

**None of these are affected by the proposed extraction** because we only move private helper functions, not the Convex-decorated exports.

---

## Test Strategy

### `convex/lib/conceptLifecycle.ts`
- **What:** archive/unarchive/softDelete/restore logic, batched phrasing updates
- **How:** Unit test with a mock `MutationCtx` (mock `ctx.db.query`, `ctx.db.patch`, `ctx.db.delete`). Verify correct patches applied, stats deltas computed, idempotency (e.g., archive already-archived returns false).
- **Existing tests:** `tests/convex/concepts.archive.test.ts` already covers archive/unarchive behavior -- these should continue to pass unchanged.

### `convex/lib/conceptReview.ts`
- **What:** getDueHandler priority selection, selectActivePhrasing, recordInteractionHandler
- **How:** Unit test with mock `QueryCtx`/`MutationCtx`. Test prioritization ordering, empty-queue fallback to new concepts, phrasing selection reason logic.
- **Existing tests:** `tests/convex/concepts.test.ts` covers `prioritizeConcepts` (already in conceptHelpers).

### `convex/lib/conceptScoring.ts`
- **What:** `recalculateConceptScores` pure function
- **How:** Pure unit test -- input array of phrasing questions, verify output `{ phrasingCount, thinScore, conflictScore }`.

### `convex/lib/generationPipeline.ts`
- **What:** `prepareConceptIdeas`, `prepareGeneratedPhrasings`, `classifyError`, `calculateConflictScore`
- **How:** Already pure functions. Existing tests at `tests/convex/aiGeneration.test.ts` and `tests/convex/aiGeneration.prep.test.ts` cover these. Update import paths (or use re-exports).

### `convex/lib/generationTracing.ts`
- **What:** Langfuse trace helpers
- **How:** Unit test with mock Langfuse client. Verify span creation, score attachment, error finalization. Low priority -- telemetry wrappers.

### `convex/lib/generationErrorHandler.ts`
- **What:** `handleGenerationFailure` orchestrator
- **How:** Unit test with mock `ctx.runMutation`. Verify correct error code mapping, job failure mutation called, analytics tracked.

### `convex/lib/iqcHelpers.ts`
- **What:** All pure IQC functions
- **How:** Already tested in `convex/iqc.test.ts`. Update import paths (or use re-exports). These are pure functions -- no Convex runtime needed.

### `convex/lib/iqcAdjudication.ts`
- **What:** `adjudicateMergeCandidate`, `fetchNeighborConcepts`
- **How:** Unit test with mock `ActionCtx` and mock `LanguageModel`. Verify prompt construction, response parsing, neighbor filtering.

### `components/agent/hooks/use-review-session.ts`
- **What:** All state management and callback logic
- **How:** React Testing Library with `renderHook`. Mock Convex mutations/queries. Test state transitions: start -> question loaded -> answer submitted -> feedback shown -> next question.

### Extracted sub-components (`active-session.tsx`, `action-panel-card.tsx`, etc.)
- **What:** Presentational rendering
- **How:** Snapshot/render tests. Verify props -> rendered output. These are presentational and low risk.

---

## Implementation Sequence

### Phase 1: Pure function extractions (independent, parallelizable)

These three extractions have zero dependency on each other and can be done simultaneously by separate agents:

**1a.** Extract `convex/lib/generationPipeline.ts` from `aiGeneration.ts`
- Move pure functions + types
- Add re-exports in `aiGeneration.ts` for backward compat
- Update `convex/evals/runner.ts` import
- Run existing tests to verify

**1b.** Extract `convex/lib/iqcHelpers.ts` from `iqc.ts`
- Move pure functions + schemas + config constants
- Add re-exports in `iqc.ts`
- Update `convex/iqc.test.ts` imports
- Run existing tests to verify

**1c.** Extract `components/agent/review-chat-types.ts` from `review-chat.tsx`
- Move all type definitions and constants
- No behavioral change

### Phase 2: Stateful helper extractions (independent, parallelizable)

**2a.** Extract `convex/lib/conceptLifecycle.ts` from `concepts.ts`
- Move lifecycle state machine functions
- Update `concepts.ts` to import and delegate
- Run `tests/convex/concepts.archive.test.ts` to verify

**2b.** Extract `convex/lib/conceptReview.ts` from `concepts.ts`
- Move query/mutation handler functions
- Update `concepts.ts` to import and delegate
- Run all concepts tests to verify

**2c.** Extract `convex/lib/iqcAdjudication.ts` from `iqc.ts`
- Move `adjudicateMergeCandidate` and `fetchNeighborConcepts`
- Update `iqc.ts` to import

**2d.** Extract `convex/lib/generationTracing.ts` and `convex/lib/generationErrorHandler.ts` from `aiGeneration.ts`
- Move tracing and error handling patterns
- Update `aiGeneration.ts` orchestration shells

### Phase 3: Frontend extractions (sequential within, parallel to Phase 2)

**3a.** Extract `components/agent/hooks/use-review-session.ts`
- Extract all state + callbacks from `ReviewChat`
- `ReviewChat` becomes thin shell consuming hook

**3b.** Extract sub-components (parallelizable after 3a):
- `active-session.tsx`
- `action-panel-card.tsx`
- `pending-feedback-card.tsx`
- `chat-message.tsx`
- `review-stats-panel.tsx`

### Phase 4: Scoring extraction + test coverage + cleanup

**4a.** Extract `convex/lib/conceptScoring.ts` (shared conflict/thin score recalculation)

**4b.** Write new unit tests for extracted modules that previously had no coverage

**4c.** Remove re-exports from `aiGeneration.ts` and `iqc.ts` (update all import paths to point directly at lib modules)

**4d.** Remove coverage exclusions from `vitest.config.ts` for:
- `convex/concepts.ts` (L100)
- `convex/iqc.ts` (L102)

(Note: `aiGeneration.ts` is not currently excluded, so no change needed there.)

---

## Risks

### Convex internal function references that break if moved
- **Mitigated:** No Convex-decorated function (mutation/query/action/internal*) is being moved. Only private helper functions move to `convex/lib/`. The `internal.*` API surface is unchanged.

### Circular dependency risks
- **`conceptLifecycle.ts` -> `userStatsHelpers.ts`:** Safe. `userStatsHelpers` is a leaf module.
- **`conceptReview.ts` -> `conceptHelpers.ts`, `conceptFsrsHelpers.ts`, `interactionContext.ts`:** Safe. All are leaf modules.
- **`generationPipeline.ts` -> `generationContracts.ts`:** Safe. Already a separate module.
- **`iqcHelpers.ts` -> `conceptFsrsHelpers.ts`, `userStatsHelpers.ts`:** Safe. Leaf modules.
- **`iqcAdjudication.ts` -> `iqcHelpers.ts`:** Safe. One-way dependency.
- **Risk: `conceptReview.ts` importing from `fsrs.ts`:** Already exists in `concepts.ts`. No new cycle introduced.
- **Risk: `generationErrorHandler.ts` needing `internal.generationJobs.failJob`:** This function calls `ctx.runMutation(internal.generationJobs.failJob, ...)`. The helper would need `ctx` and `internal` passed as arguments, or remain inline in the action. **Recommendation:** Keep the `ctx.runMutation(internal.generationJobs.failJob)` call inline in the action shells; only extract the error classification and message mapping to the helper.

### Test import breakage
- **Mitigated by re-export strategy:** Phase 1 adds re-exports so existing test imports work unchanged. Phase 4c removes re-exports and updates imports as a separate, low-risk step.

### Frontend hook extraction complexity
- **Risk:** The `useReviewSession` hook has 10+ interdependent callbacks with closure-captured state. Incorrect extraction could break callback dependency chains.
- **Mitigation:** Extract as a single hook first (don't split into sub-hooks). Test with `renderHook` before splitting further. The `ActiveSession` component extraction is mechanical (copy-paste the function + its prop types).

### File count growth
- This adds ~12 new files. Justified by the 4:1 reduction in per-file complexity and the fact that `convex/lib/` already has 18 modules following this exact pattern.
