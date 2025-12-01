## Architecture Overview
**Selected Approach**: Concept/Phrasing-Only Atomic Cutover  
**Rationale**: Data already migrated; dual models create hidden coupling across Convex, hooks, analytics, and UI. Atomic removal minimizes code surface, drops ~5k LOC, and restores single mental model.

**Core Modules**
- Schema & Data Model – single concepts/phrasings/interaction model; drop questions/questionEmbeddings/quizResults.
- Review Pipeline – `convex/concepts.ts` + `interactions` table; no legacy question fallbacks.
- Generation Jobs & AI Pipeline – `convex/generationJobs.ts` + `aiGeneration.ts`; track conceptIds only.
- Embeddings Service – `convex/embeddings.ts`; concept/phrasings vectors only; delete question helpers.
- Frontend Review Experience – hooks and components consume phrasing IDs only; true random option shuffle.
- Test & Fixture Layer – remove question fixtures/tests; align analytics docs.

**Data Flow**: User → GenerationJob (concept synthesis + phrasing generation) → concepts/phrasings tables → Review selector (`concepts.getDue`) → Review UI → `interactions` writes + FSRS update → stats/analytics.

**Key Decisions**
1. Hard delete questions stack (schema + Convex modules + UI) to collapse vocabulary to concept/phrasing.
2. Embed only concept/phrasings; remove `questionEmbeddings` table + helpers to simplify search pipeline.
3. Remove deterministic option seeding; use cryptographic/random shuffle to match requirement of “true randomness”.
4. Keep single deployment; no feature flags or shims.

## Module: Schema & Data Model
Responsibility: authoritative storage; enforce single vocabulary.

Public interface (Convex schema slices):
```ts
// interactions: remove questionId + legacy indexes
{ userId, conceptId, phrasingId, userAnswer, isCorrect, attemptedAt, sessionId?, timeSpent?, context? }

// generationJobs: conceptIds only; remove questionIds/questionsGenerated/questionsSaved (rename to phrasing counts)
{ userId, prompt, status, phase, phrasingGenerated, phrasingSaved, estimatedTotal?, topic?, conceptIds, pendingConceptIds?, durationMs?, errorMessage?, errorCode?, retryable?, timestamps... }
```

Internal implementation:
- Delete tables: `questions`, `questionEmbeddings`, `quizResults`.
- Drop `convex/migrations.ts` + `convex/migrations/` directory (already completed; no data migration).
- interactions indexes: keep `by_user`, `by_user_session`, `by_user_concept`, `by_user_phrasing`, `by_concept`; drop question-based indexes.
- generationJobs fields: rename usage to `phrasingGenerated/phrasingSaved`; keep status/phase enums unchanged.

Dependencies: Convex runtime; `convex/schemaVersion.ts` if present. Used by all backend modules.

Error handling: schema compile-time; add runtime guards in affected queries to ensure conceptId/phrasingId exist.

## Module: Review Pipeline (convex/concepts.ts + interactions)
Responsibility: select due phrasing, record answer, update FSRS + stats.

Public interface:
```ts
query getDue(): {
  concept: Doc<'concepts'>;
  phrasing: Doc<'phrasings'>;
  selectionReason: string | null;
  phrasingStats: { index: number; total: number } | null;
  interactions: Doc<'interactions'>[];
  serverTime: number;
}

query getConceptsDueCount(): { conceptsDue: number }

mutation recordInteraction(args: {
  conceptId: Id<'concepts'>;
  phrasingId: Id<'phrasings'>;
  userAnswer: string;
  isCorrect: boolean;
  timeSpent?: number;
  sessionId?: string;
}): {
  conceptId: Id<'concepts'>;
  phrasingId: Id<'phrasings'>;
  nextReview: number;
  scheduledDays: number;
  newState: FsrsState;
}
```

Internal implementation:
- Remove all `legacyQuestionId` lookups, patches, or returns.
- `prioritizeConcepts` and `selectActivePhrasing` stay; ensure candidate selection doesn’t query questions table.
- `getConceptsDueCount`: drop orphanedQuestions scan.
- FSRS update remains concept-level; interactions store conceptId+phrasingId only.

Dependencies: `fsrs` engine, `updateStatsCounters`, `buildInteractionContext`, `selectActivePhrasing`.

Error handling: unauthorized → throw; missing concept/phrasing → throw; validation of phrasing/concept ownership.

## Module: Generation Jobs & AI Pipeline
Responsibility: create jobs, run Stage A (concept synthesis) + Stage B (phrasing generation), track progress.

Public interface (convex/generationJobs.ts):
- `createJob(prompt, ipAddress?)` → { jobId }
- `getRecentJobs`, `getJobById`
- internal mutations: `setConceptWork`, `advancePendingConcept`, `completeJob`, `failJob`, `cleanup`

Internal implementation changes:
- Replace question counters with phrasing counters: `phrasingGenerated/phrasingSaved`.
- Remove `questionIds` everywhere; `completeJob` accepts conceptIds only; UI displays conceptIds count.
- aiGeneration Stage B: stop building `questionIds`; log phrasing counts; tracking events use `phrasingCount`.
- Rate limiting unchanged (`rateLimit.ts`).

Dependencies: `aiGeneration.ts`, `phrasings.insertGenerated`, `concepts.applyPhrasingGenerationUpdate`, `embeddings.generateEmbedding`.

Error handling: propagate `GenerationPipelineError` codes; ensure fail/complete use new counters.

## Module: Embeddings Service
Responsibility: embedding generation + sync for concept/phrasings; remove question embeddings.

Public interface:
- `internal.embeddings.generateEmbedding(text: string): number[]`
- `internal.embeddings.searchConcepts/phrases` (if present) should operate on concept/phrasings only.
- Cron sync: process concepts/phrasings; drop question backfill logic.

Internal implementation:
- Delete `convex/lib/embeddingHelpers.ts`; inline or replace with concept/phrasing helpers.
- Remove `questionEmbeddings` references, indexes, and backfill functions (`collectQuestionEmbeddings`, `getQuestionsWithoutEmbeddings`, etc.).
- Adjust limits constants to only `conceptLimit`/`phrasingLimit`.
- Search: ensure vector search queries `phrasings` (or `concepts`) with matching filter fields; remove hybrid question text search branch.

Dependencies: Google AI embeddings; `createConceptsLogger`; `chunkArray`.

Error handling: keep API key diagnostics; rate limit classification; add guard for missing concept/phrasing docs during sync.

## Module: Frontend Review Experience
Responsibility: render review UI with phrasing-only data.

Components/Hooks:
- `hooks/use-review-flow.ts`: drop `legacyQuestionId`; state carries `phrasingId` only; telemetry question counts based on phrasing.
- `components/review-flow.tsx`: remove legacy edit/delete paths; concept/phrasing archive/edit only; context removal.
- `components/review-question-display.tsx` → rename `review-phrasing-display.tsx`; prop `questionId` becomes `phrasingId` (optional) only for analytics.
- Delete `hooks/use-question-mutations.ts`, `components/question-edit-modal.tsx`, `contexts/current-question-context.tsx`, `types/questions.ts`, `/app/library` feature.
- `hooks/use-shuffled-options.ts`: switch to true random shuffle.
- `lib/utils/shuffle.ts`: remove seeded helpers; single `shuffle(options: T[]): T[]` using `crypto.getRandomValues` fallback to `Math.random`.

Dependencies: `useConceptActions`, `useUnifiedEdit`, `useQuizInteractions` (ensure they don’t expect question IDs).

Error handling: toasts remain; remove question-specific error text.

## Core Algorithms (Pseudocode)
### getDue (phrasing-only)
1. nowMs = Date.now(); pull due concepts via `by_user_next_review` with phrasingCount > 0.
2. If none, pull earliest `state === 'new'` concepts (limit MAX_CONCEPT_CANDIDATES).
3. prioritized = prioritizeConcepts(candidates, now);
4. For each concept in prioritized:
   - selection = selectActivePhrasing(concept, userId); if none → continue.
   - interactions = latest N interactions by `phrasingId`.
   - return { concept, phrasing: selection.phrasing, selectionReason, phrasingStats, interactions, serverTime: nowMs }.
5. If none found → return null.

### recordInteraction
1. Verify user owns concept & phrasing; ensure phrasing.conceptId === conceptId.
2. scheduleResult = scheduleConceptReview(concept, isCorrect, { now });
3. Insert interaction { conceptId, phrasingId, userAnswer, isCorrect, timeSpent, sessionId, context }.
4. Patch phrasing stats (attemptCount/correctCount/lastAttemptedAt).
5. Patch concept.fsrs + updatedAt; update userStats via `updateStatsCounters`.
6. return { conceptId, phrasingId, nextReview: scheduleResult.nextReview, scheduledDays, newState }.

### generationJobs.completeJob
1. savedCount = conceptIds.length (or phrasingSaved counter).
2. patch job: status=completed, conceptIds, pendingConceptIds=[], phrasingSaved=savedCount, durationMs, completedAt=now.

### option shuffle (true random)
```ts
export function shuffle<T>(items: T[]): T[] {
  const result = [...items];
  const rand = crypto?.getRandomValues
    ? () => crypto.getRandomValues(new Uint32Array(1))[0] / 2 ** 32
    : Math.random;
  for (let i = result.length - 1; i > 0; i--) {
    const j = Math.floor(rand() * (i + 1));
    [result[i], result[j]] = [result[j], result[i]];
  }
  return result;
}
// useShuffledOptions(options): return useMemo(() => shuffle(options), [options]);
```

## File Organization
```
convex/
  schema.ts                 // remove questions/questionEmbeddings/quizResults; update interactions & generationJobs
  concepts.ts               // strip legacyQuestion usage
  embeddings.ts             // concept/phrasings only
  generationJobs.ts         // conceptId counters only
  aiGeneration.ts           // remove questionIds, use phrasing counts
  phrasings.ts              // ensure no question deps
  migrations/               // delete dir
  migrations.ts             // delete file

app/
  library/                  // delete entire route

components/
  review-phrasing-display.tsx (renamed from review-question-display.tsx)
  review-flow.tsx           // phrasing-only actions
  generation-task-card.tsx  // conceptIds display
  question-history.tsx, question-edit-modal.tsx // delete

hooks/
  use-review-flow.ts        // remove legacyQuestionId
  use-shuffled-options.ts   // true random
  use-question-mutations.ts // delete

lib/
  utils/shuffle.ts          // replace with non-seeded shuffle
  strip-generated-questions.ts // delete if present

types/
  concepts.ts               // add SimplePhrasing if needed
  questions.ts              // delete

tests/
  convex/questions*.test.ts // delete
  convex/generationJobs.test.ts // update to conceptIds
  convex/embeddings.test.ts // remove question cases
  convex/coverage-regressions.test.ts, bandwidth-regressions.test.ts // drop question fixtures
  api-contract.test.ts      // remove questions API expectations
  fixtures under lib/test-utils // remove question data

docs/
  analytics-events.md       // rename questionId → phrasingId/conceptId; job counters phrasingCount
```

## Integration Points
- **Database**: Schema drop will break old deployments; ensure single deploy with `npx convex dev` locally before Next build. No migration scripts needed (data already migrated).
- **External services**: Google AI embeddings unchanged; Clerk auth unchanged.
- **Environment**: No new env vars. Keep `GOOGLE_AI_API_KEY`, `NEXT_PUBLIC_CONVEX_URL`.
- **Observability**: Keep Sentry instrumentation; update logger messages to remove “question” vocabulary; ensure telemetry events (conceptsLogger) use concept/phrasing IDs.
- **Analytics**: Update event names/properties in `lib/analytics.ts` + docs to use phrasing/concept counts. Remove question CRUD events.
- **CI/Quality gates**: Lefthook pre-commit (tsc + lint); pre-push changed-tests; CI runs lint + vitest. Ensure new files pass formatting (prettier) and type checks.

## State Management
- Client state: `useReviewFlow` stores current phrasing; remove current-question context. Option shuffle uses pure function (memoized on options array).
- Server state: FSRS stored on concept.fsrs; interactions link phrasingId; generation job progress tracked via conceptIds + phrasing counters.
- Cache/invalidations: Convex reactive queries auto-update; ensure deleted tables don’t leave dangling subscriptions.
- Concurrency: `getDue` relies on lockId logic in hook; backend stays stateless; no per-question lock after deletion.

## Error Handling Strategy
- Validation: throw on missing concept/phrasing or ownership mismatches.
- Analytics: still track Review Session Started/Completed/Abandoned with phrasing counts; guard against undefined data on unmount.
- API failures: keep toast messaging; update wording to “phrasing/concept”.
- Embeddings: classify API_KEY/RATE_LIMIT/NETWORK; retries unchanged.

## Testing Strategy
- Unit: `use-review-flow`, `use-shuffled-options` (randomness still deterministic in tests via mock random), `generationJobs` counters, `embeddings` concept/phrasing sync.
- Integration: Convex queries/mutations (recordInteraction, getDue) using Convex test harness; ensure interactions schema change covered.
- E2E: Review happy path (load phrasing, answer, archive), generation job list, /library 404 expectation.
- Coverage targets: critical paths (schema/query/mutation/generation) 90%+; other logic 80%+; UI hooks 70%+.
- Quality gates: run `pnpm lint`, `pnpm test`, `pnpm build`. Update/skip removed tests accordingly.

## Performance & Security Notes
- Performance: removing legacy question lookups cuts `getDue` queries; ensure no unbounded `.collect()` remains. Embedding sync limited to concept/phrasings only, reducing bandwidth.
- Security: interactions now only accept conceptId/phrasingId (reduces unauthorized cross-table writes). Option shuffle uses `crypto.getRandomValues` where available; fallback still non-deterministic.
- Observability: add Sentry breadcrumbs for generation job state transitions and review errors; maintain correlationId where present.

## Alternative Architectures Considered
| Option | Simplicity (40%) | Module Depth (30%) | Explicitness (20%) | Robustness (10%) | Weighted Score | Verdict |
| --- | --- | --- | --- | --- | --- | --- |
| Atomic removal (selected) | 10 | 9 | 9 | 8 | **9.2** | Wins: least surface, no flags |
| Feature-flag fallback | 6 | 5 | 6 | 7 | 6.1 | Adds flag plumbing, retains dual terms |
| Phased 3-deploy (soft-delete → hide → drop) | 5 | 6 | 6 | 8 | 6.0 | More migrations/tests for no data benefit |

Trigger to revisit: If production finds latent question-only data we need read-only adapter; otherwise stay atomic.

## ADR Creation
TASK does not mark ADR required. Given irreversible schema drop is already approved in PRD, skip ADR. If leadership wants paper trail, create `docs/adr/ADR-XXXX-remove-questions-table.md` referencing this DESIGN.

## Open Questions / Assumptions
- Assumption: All question data migrated; no support cases needing read-only access.
- Assumption: No third-party webhook/analytics consumer depends on questionId payloads; if exists, update alongside analytics schema.
- Confirm: Any feature flags pointing at `/library`? Remove or adjust routing guards if present.
