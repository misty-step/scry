# Pi Local Workflow for scry

This repository uses a lean local Pi foundation focused on evidence, CI parity, and backend-first Convex delivery.

## Recommended target
Use `build` for day-to-day work in this repo.

## Core loop (explore -> design -> implement -> review)
1. Discover context
   - `/discover <goal>`
2. Design minimal plan
   - `/design <goal>`
3. Deliver change
   - `/deliver <goal>`
4. Final review
   - `/review <diff-or-pr-context>`

## Pipeline routing
- `scry-delivery-v2` (default for product work)
- `scry-convex-delivery-v1` (Convex schema/query/mutation heavy work)
- `scry-foundation-v2` (Pi/config/process foundation changes)

## Verification baseline (must mirror CI)
Run for meaningful code changes:
- `pnpm lint`
- `pnpm tsc --noEmit`
- `pnpm test:ci`

Add when applicable:
- `pnpm test:contract` for `convex/**` changes
- `pnpm build` for dependency/build/workflow/config changes
- `pnpm audit --audit-level=critical` for dependency or lockfile changes

Never run deploy-coupled commands without explicit approval:
- `pnpm build:local`
- `pnpm build:prod`
- `pnpm convex:deploy`
- `./scripts/deploy-production.sh`

## 72h accretive experiment (SEAM-lite)
Use `scry-seam-retrospective-v1` after completed tasks for 72 hours to test whether one evidence-backed lesson per task reduces repeated mistakes.

Track:
- repeated CI-parity misses
- repeated Convex bandwidth violations
- time overhead per task

## Lessons (local)
- (empty by default) Add only reviewer-approved, repo-specific lessons from `scry-seam-retrospective-v1`.