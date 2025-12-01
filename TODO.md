## Planning Report
**Spec**: DESIGN.md (Concept/Phrasing-Only Atomic Cutover)  
**Tasks Generated**: 12  
**Total Estimate**: 11h  
**Critical Path**: 7h (Tasks 1 → 2 → 3 → 4 → 5 → 6)

### Task Summary
| Phase | Tasks | Estimate | Dependencies |
| --- | --- | --- | --- |
| Backend schema & data | 4 | 4h | None |
| Backend logic | 3 | 2.5h | Schema drop |
| Frontend | 3 | 2.5h | Schema/logic updates |
| Tests & docs | 2 | 2h | Backend & frontend |

### Critical Path
1. Backend schema removal (1h) →  
2. Delete Convex question modules (1h) →  
3. Generation jobs/AI pipeline refactor (1h) →  
4. Embeddings service cleanup (1h) →  
5. Review flow hook/component rewrite (1.5h) →  
6. Test/doc updates (1.5h)  
Total: ~7h

---

## TODO

- [x] Backend: Drop legacy tables and fields from schema
  ```
  Files:
  - convex/schema.ts
  Goal: Remove questions/questionEmbeddings/quizResults tables; remove questionId + related indexes from interactions; replace question counters in generationJobs with phrasing counters.
  Approach:
  1) Delete questions, questionEmbeddings, quizResults definitions.  
  2) interactions: remove questionId fields/indexes; keep conceptId/phrasingId indexes.  
  3) generationJobs: remove questionIds/questionsGenerated/questionsSaved; add phrasingGenerated/phrasingSaved (or rename), keep conceptIds/pendingConceptIds.
  Success Criteria:
  - Schema compiles (`npx convex dev` dry load).  
  - No references to questions/questionEmbeddings/quizResults remain.  
  Tests: `npx convex dev` schema check.
  Estimate: 1h
  ```

- [x] Backend: Delete Convex question modules and migrations
  ```
  Files:
  - convex/questions*.ts (6 files)
  - convex/migrations.ts
  - convex/migrations/ directory
  Goal: Remove legacy question CRUD/bulk/library/related/migration code per TASK manifest.
  Approach:
  1) Delete listed files/directories.  
  2) Ensure imports elsewhere are removed/updated (grep for api.questions*).  
  Success Criteria:
  - No build/type errors from missing modules.  
  - `rg "questions"` only returns historical docs/strings, not code paths.  
  Tests: `pnpm lint` (type import failures surface).
  Estimate: 1h
  Depends: Backend schema removal
  ```

- [x] Backend: Refactor generationJobs to concept/phrasing-only
  ```
  Files:
  - convex/generationJobs.ts
  - convex/aiGeneration.ts
  - convex/concepts.ts (requestPhrasingGeneration init job payload)
  Goal: Remove questionIds usage; track phrasing counts; completeJob/fail/updateProgress align with new fields.
  Approach:
  1) Rename counters to phrasingGenerated/phrasingSaved; adjust job inserts/patches.  
  2) aiGeneration Stage B: stop building questionIds; use conceptIds and phrasing counts in telemetry.  
  3) Adjust generation-task UI data expectations later (frontend task).  
  Success Criteria:
  - Job lifecycle runs without referencing questions; typecheck passes.  
  - Tracking events use phrasing/concept counts.  
  Tests: Update/run `tests/convex/generationJobs.test.ts`; targeted `pnpm test tests/convex/generationJobs.test.ts`.
  Estimate: 1h
  Depends: Backend schema removal
  ```

- [x] Backend: Clean embeddings service to concepts/phrasings only
  ```
  Files:
  - convex/embeddings.ts
  - convex/lib/embeddingHelpers.ts (delete)
  Goal: Remove question embedding paths and helper module; ensure search/sync only use concept/phrasings.
  Approach:
  1) Delete embeddingHelpers and imports.  
  2) Strip question search/backfill branches; keep concept/phrasing limits.  
  3) Adjust vector search to phrasings (or concepts) only; update constants.  
  Success Criteria:
  - No questionId/Id<'questions'> remains.  
  - Embedding cron still compiles with concept/phrasing flows.  
  Tests: Update/run `tests/convex/embeddings.test.ts`.
  Estimate: 1h
  Depends: Backend schema removal
  ```

- [x] Backend: Review pipeline cleanup (concepts.ts interactions)
  ```
  Files:
  - convex/concepts.ts
  Goal: Remove legacyQuestion lookups/patches; interactions insert uses conceptId/phrasingId only; drop orphanedQuestions count.
  Approach:
  1) getDue: delete legacyQuestion query/return; ensure candidate loop unaffected.  
  2) recordInteraction: remove legacyQuestion patch/return.  
  3) getConceptsDueCount: remove orphaned question scan.  
  Success Criteria:
  - getDue response matches DESIGN (no legacyQuestionId).  
  - recordInteraction returns conceptId/phrasingId/nextReview/scheduledDays/state only.  
  Tests: Update `hooks/use-review-flow.test.ts` fixtures; `tests/convex/fsrs-soft-delete.test.ts` if referencing legacyQuestion.
  Estimate: 45m
  Depends: Backend schema removal
  ```

- [x] Frontend: Delete library route and question UI
  ```
  Files:
  - app/library/** (delete)
  - components/question-edit-modal.tsx (delete)
  - components/question-history.tsx (delete if present)
  - hooks/use-question-mutations.ts + test (delete)
  - contexts/current-question-context.tsx (delete)
  - types/questions.ts (delete)
  - lib/strip-generated-questions.ts (delete if exists)
  Goal: Remove legacy question UI surface and hooks.
  Approach:
  1) Delete directories/files.  
  2) Fix import fallout in remaining components/hooks (review-flow).  
  Success Criteria:
  - No imports from deleted modules remain.  
  - /library 404s by absence.  
  Tests: `pnpm lint` (type/import), `pnpm test` targeted failing suites.
  Estimate: 1h
  Depends: Backend schema removal
  ```

- [x] Frontend: Review display rename + phrasing-only props
  ```
  Files:
  - components/review-question-display.tsx → components/review-phrasing-display.tsx
  - components/index.ts (export update)
  - import sites (review-flow, review-mode, tests)
  Goal: Align naming and props to phrasingId; drop questionId union.
  Approach:
  1) Rename file and component; prop `questionId` → `phrasingId` optional.  
  2) Update imports/exports.  
  3) Adjust usages to pass phrasingId.  
  Success Criteria:
  - Build passes; no lingering review-question-display imports.  
  Tests: Update component tests if present; run `pnpm test components/review-flow.test.tsx`.
  Estimate: 45m
  Depends: Review flow hook update
  ```

- [x] Frontend: Review flow hook/component to remove legacyQuestionId
  ```
  Files:
  - hooks/use-review-flow.ts
  - components/review-flow.tsx
  - components/review/review-mode.tsx
  Goal: Make review flow phrasing-only; remove legacy question edit/delete paths and context usage.
  Approach:
  1) use-review-flow state/actions: drop legacyQuestionId; adjust payload typing; ensure lock/timeout logic unchanged.  
  2) review-flow component: remove question context/setCurrentQuestion logic; delete legacy edit/delete handlers; ensure archive/edit operate on concept/phrasing hooks only.  
  3) review-mode/other consumers: update prop names if any.  
  Success Criteria:
  - Typecheck passes without Id<'questions'>.  
  - UI still supports answer, archive, edit phrasing/concept.  
  Tests: Update `hooks/use-review-flow.test.ts`, `components/review-flow.test.tsx`, `tests/api-contract.test.ts` expectations.
  Estimate: 1.5h
  Depends: Backend review pipeline cleanup
  ```

- [x] Frontend: True random option shuffle
  ```
  Files:
  - lib/utils/shuffle.ts
  - hooks/use-shuffled-options.ts
  Goal: Replace seeded shuffle with true random per TASK requirement.
  Approach:
  1) Implement non-seeded shuffle using crypto.getRandomValues fallback to Math.random.  
  2) use-shuffled-options: drop questionId/userId seed; memoize on options only.  
  Success Criteria:
  - Options order differs across renders statistically; no seed params.  
  Tests: Unit test to assert shuffle uses provided mock random; remove seed-based expectations.
  Estimate: 45m
  Depends: Frontend review flow hook/component update
  ```

- [x] Frontend: Generation task card job fields
  ```
  Files:
  - components/generation-task-card.tsx
  Goal: Display conceptIds / phrasing counts instead of questionIds/questionsSaved.
  Approach:
  1) Update props/data mapping to new job shape (phrasingSaved/Generated).  
  2) Adjust copy to “phrasing(s)” and concept count.  
  Success Criteria:
  - Component renders with new fields; no runtime access to questionIds.  
  Tests: Update related tests/snapshots if exist.
  Estimate: 30m
  Depends: Generation jobs refactor
  ```

- [x] Tests & fixtures cleanup
  ```
  Files:
  - tests/convex/questionsLibrary.test.ts (delete)
  - tests/convex/questionsInteractions.record.test.ts (delete)
  - tests/convex/migrations.test.ts (delete if exists)
  - tests/convex/generationJobs.test.ts (update)
  - tests/convex/embeddings.test.ts (update)
  - tests/convex/coverage-regressions.test.ts & bandwidth-regressions.test.ts (update fixtures)
  - lib/test-utils/fixtures.ts, lib/test-utils/largeFixtures.ts (remove question data)
  - tests/api-contract.test.ts (drop question APIs)
  Goal: Align test suite to concept/phrasing-only world.
  Approach:
  1) Delete question-specific tests.  
  2) Update remaining fixtures/types to remove questionId.  
  3) Run targeted vitest suites, fix failures.  
  Success Criteria:
  - `pnpm test` passes.  
  - No test references Id<'questions'>.  
  Estimate: 1.5h
  Depends: Backend & frontend updates
  ```

- [x] Docs & analytics terminology update
  ```
  Files:
  - docs/analytics-events.md
  - lib/analytics.ts (property types if needed)
  Goal: Replace questionId/questionCount with phrasingId/conceptId phrasingCount; drop question CRUD events.
  Approach:
  1) Update event tables/examples to new fields.  
  2) Ensure analytics types match code after hook changes.  
  Success Criteria:
  - Docs reflect current events; no question vocabulary.  
  Tests: Typecheck analytics types; optional lint on docs spelling.  
  Estimate: 30m
  Depends: Review flow & generation job changes
  ```

---

## Guardrails
- Run `pnpm lint`, `pnpm test`, `pnpm build`, and `npx convex dev` schema check before PR.
- Out of scope: adding new analytics events, new UI features, or refactoring concepts.ts beyond question removal.
