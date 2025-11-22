1. **Executive Summary**  
- Backend Convex coverage is ~13% vs overall 24%; high-risk modules (aiGeneration, generationJobs, questionsInteractions, migrations helpers) lack regression nets.  
- Goal: raise Convex coverage to ≥30% and lock per-path thresholds so critical code can’t regress silently.  
- Strategy: deterministic unit seams (stubbed Convex ctx + fixture factories) around pure logic and state transitions; avoid live Convex/LLM calls.  
- Success = Convex coverage ≥30%, critical mutation paths exercised, vitest per-path thresholds enforced in CI, suites remain <5s total added runtime.

2. **User Context & Outcomes**  
- Who: backend maintainers, QA, on-call.  
- Pain: regressions in AI pipeline, scheduling, migrations surface late.  
- Outcome: safe refactors; CI blocks drops on Convex/payment/auth; tests document invariants for new contributors.

3. **Requirements**  
- Functional  
  - aiGeneration: cover `prepareConceptIdeas`, `prepareGeneratedPhrasings`, `classifyError`, conflict-score branch, fallback concept path, embedding failure warning path (mocked action).  
  - generationJobs: prompt length bounds, concurrent job cap, rate-limit gating, cancel states, progress/advancePendingConcept state machine, failJob/completeJob duration handling.  
  - questionsInteractions: ownership guard, FSRS init vs repeat, session context composition, stats patch deltas, nextReview/scheduledDays propagation.  
  - migrations helpers: difficulty/topic removal pagination, dry-run vs real, failure aggregation, createdAt/sessionId backfills (missing context, already-migrated skips).  
- Non-Functional  
  - Tests deterministic, no network/LLM calls, no real Convex server; runtime <5s; fixtures minimal, no PII.  
  - Maintain deep modules: expose test seams without leaking Convex internals; avoid pass-through helpers.  
  - Keep logging noise low in test runs; guard against env mutation.  
- Infrastructure  
  - Update `vitest.config.ts` thresholds per path: `convex/**/*.ts` ≥30 lines/funcs; `lib/payment/**` ≥80; `lib/auth/**` ≥80; retain global floor.  
  - Respect Lefthook: tests runnable via `pnpm test --run` and `pnpm test:coverage`; ensure new suites pass `pnpm tsc --noEmit`.  
  - Observability parity: keep Sentry calls mocked/disabled in tests; use structured logger stubs.  
  - CI ready: no new secrets, no additional env vars beyond defaults.

4. **Architecture Decision**  
- Chosen: Pure-unit seam with stubbed Convex ctx + fixture factories.  
  - Deep modules: isolate prep/classification functions; wrap Convex ctx access behind tiny helpers to mock cheaply.  
  - Avoid spinning Convex dev server or LLMs; use deterministic fixtures for jobs, concepts, scheduler outputs.  
- Alternatives (weighted: value 40, simplicity 30, explicitness 20, risk 10)  
  - A) Pure-unit seams (selected): Value 9, Simplicity 9, Explicitness 8, Risk 7 → 8.7/10.  
  - B) Integration tests via Convex dev runtime: Value 9, Simplicity 5, Explicitness 7, Risk 6 → 7.0/10 (slower, flakier, env heavy).  
  - C) End-to-end API tests hitting Next+Convex: Value 8, Simplicity 4, Explicitness 6, Risk 5 → 6.2/10 (costly, overlaps Playwright).  
- Layering:  
  - Fixtures layer owns data shapes (Doc<'questions'>, job snapshots).  
  - Test seams expose small APIs (e.g., `__test.prepareConceptIdeas`, ctx factory).  
  - Mocks layer owns provider/scheduler stubs; no test reaches network.

5. **Data & API Contracts**  
- Convex ctx stub: `{ db: { get: fn, insert: fn, patch: fn, query: fn }, scheduler?: { runAfter: fn } }`; mutations invoked via `._handler` when needed.  
- Generation job fixture: `{ userId: Id<'users'>, prompt: string, status: 'pending'|'processing'|'failed'|'completed', phase, pendingConceptIds: Id<'concepts'>[], questionsGenerated: number, questionsSaved: number }`.  
- Concept phrasing batch fixture: array of `{ question, explanation, type, options, correctAnswer }` with target count = `TARGET_PHRASINGS_PER_CONCEPT`.  
- Migration result contract: `{ status: 'completed'|'partial'|'failed', dryRun: boolean, stats: T, failures?: {recordId,error}[] }` mirrored in tests for difficulty/topic/backfill helpers.  
- Scheduler stub must return `{ dbFields, nextReview?, scheduledDays?, state? }` to mirror `IScheduler.scheduleNextReview` contract.

6. **Implementation Phases**  
- Phase 1 (MVP, ~0.5d): Add vitest per-path thresholds; build fixture builders + logger/scheduler/db stubs; write aiGeneration prep/error-classification tests; cover conflict-score edge.  
- Phase 2 (Core, ~1d): Add generationJobs state-machine tests (progress, cancellation, cleanup summaries); questionsInteractions success/failure paths incl. missing ownership and FSRS init/repeat; ensure session context saved.  
- Phase 3 (Hardening, ~0.5d): Migration helpers dry-run vs real, pagination continuation, partial failures; embed warnings path; add coverage assertions to CI docs; tidy docs in `docs/guides/testing-convex.md` if present.  
- Future: Property-based fuzz for phrasing filters; optional Convex runtime smoke via mock adapter; add Codecov status check tightening once stable.

7. **Testing & Observability**  
- Unit: Vitest suites for each module; use fixed timestamps to kill flake; snapshot minimal objects only when stable.  
- Integration-lite: call Convex mutations via `._handler` with stub ctx where cheap (recordInteraction).  
- Coverage: run `pnpm test:coverage`; ensure reports include convex paths; monitor added LCOV size.  
- Error tracking: Mock Sentry in tests; assert logger called for fatal paths without asserting exact string (focus on fields).  
- Logging: use stub logger that records payloads; ensure no console spam in CI.  
- Performance: target added test runtime <5s; parallel-friendly.  
- Analytics/Web Vitals: no new events; tests ensure `trackEvent` mocks not throwing when absent.

8. **Risks & Mitigations**  
- Brittle fixtures vs schema drift → build fixture factories anchored to `_generated/dataModel` types.  
- Over-mocking hides integration bugs → select a few `._handler` invocations to hit patch/insert logic.  
- Flakiness from timestamps/random IDs → fix time, seed randomness, avoid `Date.now()` in assertions.  
- Thresholds blocking legacy low-coverage paths → scope thresholds to critical globs only; document waivers.  
- Test noise from Sentry/console → provide noop mocks in setup.

9. **Open Questions / Assumptions**  
- Desired runtime budget in CI for new tests (<5s assumed)?  
- Is it acceptable to expose test-only exports (e.g., `__test` objects) in convex modules to avoid brittle `as any`?  
- Should migration helper tests cover `convex/migrations/*.ts` backups or only active helpers?  
- Are we allowed to refactor small helper seams (e.g., extract conflict-score calc) to deepen modules for testability?  
- Any paid API keys present in CI that we must avoid touching (LLM, Clerk)? assumption: none used in unit tests.  
- Target date for enforcing new thresholds? assume immediate once merged.  
- Any priority ordering among the four modules (aiGeneration vs migrations) for first coverage bump?
