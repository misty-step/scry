# Convex Testing & Coverage Expectations

## Coverage guardrails
- Global floor: lines/statements 27%, functions 26%, branches 22% (Vitest + Codecov).
- Critical paths (Vitest `thresholds`):
  - `convex/**/*.ts`: ≥25% lines & functions (ratchets upward only).
  - `lib/payment/**/*.ts`: ≥80% lines & functions.
  - `lib/auth/**/*.ts`: ≥80% lines & functions.
- Aim to raise Convex coverage toward 60% overall; never lower thresholds.

## How to run
- Unit/integration: `pnpm test --run` or target a file: `pnpm test tests/convex/generationJobs.logic.test.ts`.
- Coverage: `pnpm test:coverage` (respects per-path thresholds).
- Type/lint sanity before CI hooks: `pnpm tsc --noEmit && pnpm eslint .`.

## Test isolation rules
- No network/LLM calls in unit suites. Use stubs from `tests/helpers` (`createMockDb`, `createMockCtx`, `createLoggerStub`, `createSchedulerStub`).
- Mock Sentry/analytics globally (see `vitest.setup.ts`); avoid real telemetry.
- Keep Convex handlers invoked via `as any` on `._handler` only in tests—do not export production-only APIs.

## Tips for Convex modules
- Prefer pure helpers for validation/normalization; export under `__test` when needed.
- Use fixed timestamps (`fixedNow` in helpers) to avoid flake.
- Pagination helpers: use `createQueryChain` or local paginate fakes instead of real Convex runtime.

## Common pitfalls
- esbuild binary mismatches can block Vitest; run `pnpm install esbuild@latest` if startup errors mention versions.
- Keep test data minimal to hold coverage runtime <5s.
