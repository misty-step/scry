## Planning Report

**Spec**: TASK.md (Convex coverage uplift)  
**Tasks Generated**: 9  
**Total Estimate**: 7h 45m  
**Critical Path**: 5h 45m

### Task Summary

| Phase | Tasks | Estimate | Dependencies |
|-------|-------|----------|--------------|
| Setup | 2 | 1h 15m | None |
| Core  | 4 | 4h 30m | Setup |
| Migrations | 1 | 45m | Core (fixtures) |
| Quality | 2 | 1h 15m | Core |

**Critical Path**: T1 → T2 → T3 → T4 → T5 (5h 45m)

---

## TODO

- [x] T1: Add per-path coverage thresholds  
  ```
  Files: vitest.config.ts  
  Goal: Enforce coverage floors for convex, lib/payment, lib/auth without lowering globals.  
  Approach:
    1. Add thresholds object with globs per TASK.md (convex 30 lines/funcs; payment/auth 80).  
    2. Keep existing global thresholds unchanged.  
    3. Ensure include/exclude unchanged to keep report scope.  
  Success Criteria:
    - Coverage config shows new globs with specified numbers.
    - `pnpm test --run` loads config without errors.  
  Tests: `pnpm test --run somefile.test.ts` (config parse) or `pnpm test:coverage` dry run if time.  
  Estimate: 20m  
  ```

- [x] T2: Add shared test fixtures and stubs for Convex modules  
  ```
  Files: tests/helpers/convexFixtures.ts (new), tests/helpers/loggerStub.ts (new), tests/helpers/schedulerStub.ts (new), vitest.setup.ts (imports)  
  Goal: Provide reusable ctx/db/logger/scheduler stubs and data builders to keep suites deterministic.  
  Approach:
    1. Create factory for mock Convex ctx with get/insert/patch/query, configurable returns.  
    2. Add builder helpers for questions, concepts, generation jobs with fixed timestamps.  
    3. Add no-op logger capturing calls; add scheduler stub with injectable return values.  
    4. Wire helpers in vitest.setup.ts exports for easy import path.  
  Success Criteria:
    - Helpers imported by tests without TS errors.  
    - Factories support overriding fields per call.  
  Tests: Typecheck via `pnpm tsc --noEmit`; small sanity test in one suite once added.  
  Estimate: 55m  
  ```

- [x] T3: Cover aiGeneration prep + error classification logic  
  ```
  Files: tests/convex/aiGeneration.prep.test.ts (new), convex/aiGeneration.ts (optional test-only export block)  
  Goal: Exercise prepareConceptIdeas, prepareGeneratedPhrasings, calculateConflictScore, classifyError branches incl. fallback, duplicates, option limits, correct answer mismatch.  
  Approach:
    1. If needed, add `__test` exports for pure helpers (no behavior change).  
    2. Write focused unit cases with fixed input arrays and existingQuestions list; assert outputs + stats.  
    3. Add classifyError cases for schema, rate limit, api key, network, default.  
  Success Criteria:
    - Tests pass and push Convex coverage upward.  
    - No network/LLM calls in tests.  
  Tests: `pnpm test tests/convex/aiGeneration.prep.test.ts`  
  Estimate: 1h 15m  
  Depends: T2 (fixtures for stats if used)  
  ```

- [ ] T4: Cover generationJobs state machine & limits  
  ```
  Files: tests/convex/generationJobs.logic.test.ts (new), convex/generationJobs.ts (optional `__test` export for defaults/constants)  
  Goal: Validate prompt length bounds, concurrent job cap, rate limit, cancel states, updateProgress/advancePendingConcept transitions, complete/fail durations.  
  Approach:
    1. Use fixture ctx with in-memory maps; simulate createJob with varying prompts and ip addresses.  
    2. Test advancePendingConcept: pendingConceptIds shrink, phase flips to finalizing when empty.  
    3. Test cancelJob ownership and status restrictions.  
  Success Criteria:
    - All branches under test without hitting Convex runtime.  
    - Added tests deterministic and under 1s runtime.  
  Tests: `pnpm test tests/convex/generationJobs.logic.test.ts`  
  Estimate: 1h 15m  
  Depends: T2  
  ```

- [ ] T5: Cover questionsInteractions mutation behavior  
  ```
  Files: tests/convex/questionsInteractions.record.test.ts (new), convex/questionsInteractions.ts (add `__test` export if needed)  
  Goal: Ensure ownership check, FSRS init vs repeat paths, session context composition, stats patch, nextReview/scheduledDays propagation.  
  Approach:
    1. Mock requireUserFromClerk & getScheduler via vi.fn in test.  
    2. Build ctx with get returning question owned/unowned; assert error on unauthorized.  
    3. Verify insert payload includes sessionId/timeSpent and context fields; patch called with deltas.  
  Success Criteria:
    - Both init and repeat branches covered.  
    - Session context includes fsrsState, scheduledDays, nextReview.  
  Tests: `pnpm test tests/convex/questionsInteractions.record.test.ts`  
  Estimate: 1h 5m  
  Depends: T2  
  ```

- [ ] T6: Cover migration helpers (difficulty/topic/backfill)  
  ```
  Files: tests/convex/migrations.helpers.test.ts (new), convex/migrations.ts (export test hooks if needed)  
  Goal: Validate dry-run vs real paths, pagination continuation, already-migrated skips, failure aggregation, missing session context path for backfillInteractionSessionId.  
  Approach:
    1. Extract or expose internal helpers under `__test`.  
    2. Use fixtures to simulate paginated batches; inject failures to assert partial status.  
    3. Assert replace/patch not called in dry-run.  
  Success Criteria:
    - Tests cover success, partial, failure status calculations and counters.  
    - No reliance on backup migration files.  
  Tests: `pnpm test tests/convex/migrations.helpers.test.ts`  
  Estimate: 45m  
  Depends: T2  
  ```

- [ ] T7: Sentry/logger/test setup hardening  
  ```
  Files: vitest.setup.ts, instrumentation.ts (if guard needed), tests/helpers/loggerStub.ts (reuse)  
  Goal: Ensure tests don’t emit Sentry/network; provide noop/mock for captureException/trackEvent and console suppression toggle.  
  Approach:
    1. In vitest.setup.ts, vi.mock Sentry modules and analytics helpers.  
    2. Add silent console for noisy modules in test env if necessary.  
  Success Criteria:
    - Running new suites produces no Sentry/network attempts.  
    - Logger stub available across tests.  
  Tests: any added suite; confirm no console spam.  
  Estimate: 30m  
  Depends: T2 (logger stub)  
  ```

- [ ] T8: Update docs with coverage expectations  
  ```
  Files: docs/guides/testing-convex.md (or new), BACKLOG.md (if doc missing)  
  Goal: Document new thresholds, how to run Convex unit suites, and no-network rule.  
  Approach:
    1. Add short section on coverage goals and commands (`pnpm test:coverage`).  
    2. Note test-only exports and when acceptable.  
  Success Criteria:
    - Doc present and referenced paths accurate.  
  Tests: n/a (doc review)  
  Estimate: 20m  
  Depends: T1-T7 (final values)  
  ```

- [ ] T9: Quality gate verification  
  ```
  Files: n/a (commands)  
  Goal: Validate config + suites after changes.  
  Approach:
    1. Run `pnpm test:coverage` to confirm thresholds met and runtime acceptable.  
    2. Run `pnpm lint` and `pnpm tsc --noEmit`.  
  Success Criteria:
    - Commands pass locally; capture runtime for CI note.  
  Tests: the commands themselves.  
  Estimate: 30m  
  Depends: T1-T8  
  ```

---

## Boundaries / Not Doing
- No Convex dev server or live LLM/API calls in tests.  
- No Playwright/E2E additions.  
- No schema changes or data migrations beyond test coverage.  
- Backup migration files remain untouched.

## Risks & Mitigations (quick)
- Fixture drift vs dataModel → tie builders to types, update when schema moves.  
- Thresholds flake due to LCOV size → keep tests fast, avoid snapshot bloat.  
- Over-mocking hides integration bugs → limited `._handler` invocation in T5 ensures patch/insert paths executed.

---

**Next**: `/prompts:execute` to start T1.  
