# Bun Migration Guide

> Developer guide for using Bun as the package manager in scry.

## Status

| Phase | Status | Notes |
|-------|--------|-------|
| Phase 1: Root topology cleanup | ✅ Complete | See #271 |
| Phase 2: Bun compatibility spike | ✅ Complete | See #272 |
| Phase 3: CI/ops cutover | 🔄 In Progress | See #273 |
| Phase 4: Full cutover | ⏳ Blocked | Waiting for Phase 3 validation |

## Current State

Bun is **available as an alternative** to pnpm. Both package managers work, but pnpm remains the default for CI and production deploys until Phase 4.

## Quick Reference

| Task | pnpm | Bun |
|------|------|-----|
| Install dependencies | `pnpm install` | `bun install` |
| Deterministic install | `pnpm install --frozen-lockfile` | `bun install --frozen-lockfile` |
| Dev server | `pnpm dev` | `bun run dev` |
| Lint | `pnpm lint` | `bun run lint` |
| Typecheck | `pnpm typecheck` | `bun run typecheck` |
| Test | `pnpm test:ci` | `bun run test:ci` |
| QA gate | `pnpm qa` | `bun run qa` |
| QA smoke | `pnpm qa:dogfood:local` | `bun run qa:dogfood:local` |
| Security | `pnpm audit` | `bun audit` |

## Important Differences

### Linting

Next.js 16 has removed the `next lint` command. We've updated the `lint` script in `package.json` to use ESLint directly.

```bash
# ❌ Don't use (fails with "Invalid project directory")
bunx next lint

# ✅ Use instead
bun run lint
# OR
bunx eslint .
```

### Lockfiles

- **pnpm**: Uses `pnpm-lock.yaml` (Primary source of truth for production)
- **Bun**: Uses `bun.lock` (Tracked for CI deterministic validation)

**Do not update `bun.lock` unless intentionally changing dependencies.** CI uses `bun install --frozen-lockfile`.

## Running Parity Checks

Verify Bun compatibility locally:

```bash
bun run spike:bun:matrix
```

This runs the full parity matrix and generates a report at `/tmp/bun-parity-*.md`.

## CI Behavior

- **ci.yml**: Runs on pnpm (production CI)
- **ci-bun.yml**: Runs on Bun (parallel validation, non-blocking)

The Bun CI workflow runs on feature branches only. It will be promoted to production CI after a 2-week validation period.

## Rollback

If Bun causes issues, see: [Bun Rollback Runbook](/docs/operations/BUN_ROLLBACK_RUNBOOK.md)

## Troubleshooting

### "Invalid project directory provided"

If you see this error when running `next lint`, it's because Next.js 16 no longer includes the `lint` command. Use `bun run lint` (or `pnpm lint`) instead, as we have updated the package script to use ESLint directly.

### Installation issues

If `bun install` fails:

1. Clear the cache: `bun pm cache rm`
2. Remove node_modules: `rm -rf node_modules`
3. Reinstall: `bun install`

## Migration Timeline

- **Week 1-2**: Parallel CI validation (current)
- **Week 3**: Decision gate - go/no-go for full cutover
- **Week 4**: Full cutover (if approved)

## Questions?

- Issue: #273
- Discussion: #270
