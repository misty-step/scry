# Delete Dead Code

Priority: high
Status: done
Estimate: S

## Goal

Remove ~4,000 lines of unused and duplicate code — dev-only routes and the legacy review flow — so the codebase reflects the product it actually is.

## Non-Goals

- Rewriting anything (pure deletion)
- Changing the agentic review experience
- Migrating any logic from legacy to agentic (if it's not in /agent already, it's not needed)

## Oracle

- [x] `/app/lab/` directory deleted (UnifiedLabClient 684 lines, ConfigManagerPage 471 lines)
- [x] `/app/evolve/` directory deleted (EvolveDashboard)
- [x] `/app/design-lab/` directory deleted (LandingTuner)
- [x] `/app/test-error/` directory deleted (167 lines)
- [x] Legacy ReviewFlow components deleted: `components/review-flow.tsx` (119 lines), `components/review/session-context.tsx` (476 lines), and all supporting review components (~2,430 lines total)
- [x] `components/review-flow.test.tsx.skip` and `components/generation-modal.test.tsx.skip` deleted (tests for dead code are dead tests)
- [x] `/app/page.tsx` redirects to `/agent` (or renders the agentic review directly)
- [x] All imports of deleted components removed — no dead imports, no unused exports
- [x] `pnpm build:local` succeeds with zero errors
- [x] `pnpm test` passes (delete tests that reference deleted components)

## What Was Built

PR: https://github.com/misty-step/scry/pull/307

89 files changed, 20 insertions, 13,249 deletions. Deleted all dev-only routes,
legacy review flow, orphaned hooks/libs/types, and dead loading skeletons.
Root route redirects to `/agent`. Review bench found and fixed: no-op callback
prop in navbar, dead `app/loading.tsx`, and entirely unused `loading-skeletons.tsx`.

## Notes

**What's being deleted and why:**

Dev-only routes (blocked in production anyway, ~2,000 lines):
- `/lab` — Genesis Laboratory config testing
- `/lab/configs` — Config manager
- `/evolve` — Prompt evolution dashboard
- `/design-lab` — Landing page tuner
- `/test-error` — Sentry test page

Legacy review flow (~2,400 lines):
- The app has TWO review experiences: legacy card-based ReviewFlow at `/` and agentic chat at `/agent`
- The agentic experience is the product. The legacy flow is dead weight.
- 15+ components in `components/review/` that are only used by the legacy flow

**Sequence:**
1. Delete dev-only route directories
2. Delete legacy review components
3. Make `/` route to agentic experience
4. Clean up dead imports and tests
5. Verify build + tests pass
