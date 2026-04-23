# Fix Bun Test Oracle

Priority: high
Status: ready
Estimate: S

## Goal

Make Scry's top-level test oracle truthful. `bun test` currently fails in unrelated frontend and Vitest-specific paths, so operators cannot use it as a reliable repo gate.

## Non-Goals

- No broad test rewrite.
- No migration away from Vitest.
- No attempt to fix every currently failing test in one ticket.

## Oracle

- [ ] `package.json` no longer implies `bun test` is the primary repo test gate when the suite requires Vitest/jsdom semantics
- [ ] One canonical test command is documented and wired for operators and CI
- [ ] Repo-level failures under that canonical command are either fixed or explicitly split into follow-up backlog items
- [ ] The failure modes found on `2026-04-18` are captured:
  - React Testing Library tests failing under Bun with `document is not defined`
  - Vitest mocks failing under Bun with `vi.mocked is not a function`

## Notes

Observed while wiring `memory-engine` canary ticket 10. The FSRS seam passed under the native Scry runner:

- `corepack pnpm@10.12.1 tsc --noEmit`
- `corepack pnpm@10.12.1 exec vitest --run tests/convex/memory-engine-adapter.test.ts tests/convex/fsrs.test.ts convex/fsrs/conceptScheduler.test.ts`

But `bun test` failed in unrelated app and AI-generation paths, so the broken oracle should be fixed at the repo level rather than copied into downstream tickets.
