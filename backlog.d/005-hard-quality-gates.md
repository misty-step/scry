# Hard Quality Gates

Priority: high
Status: ready
Estimate: M

## Goal

Make evals and tests actually block merges. Convert the existing quality infrastructure from informational dashboards into hard gates that prevent regressions.

## Non-Goals

- Achieving 100% coverage
- Adding new CI workflows (fix what exists)
- Resolving all 55 audit vulnerabilities (triage, not zero)

## Oracle

- [ ] `prompt-eval.yml` is a **required status check** — remove `continue-on-error: true` (lines 63, 78); evals must pass to merge
- [ ] ESLint rules `react-hooks/set-state-in-effect`, `react-hooks/purity`, `react-hooks/refs`, `react-hooks/preserve-manual-memoization` promoted from `warn` to `error` in `eslint.config.mjs:36-39`
- [ ] `pnpm audit --audit-level=high` in CI (currently `--audit-level=critical` at `ci.yml:38`) — or each suppressed vulnerability documented
- [ ] `ci-bun.yml` either deleted or promoted to primary (no parallel divergent pipelines)
- [ ] `@typescript-eslint/no-explicit-any` enforced as `error` in production code
- [ ] All CI checks on PRs are either required or deleted — no noise

## Notes

**Current state:**
- `prompt-eval.yml` runs evals but `continue-on-error: true` means failures are invisible
- 4 React hooks rules downgraded to warn
- 55 vulnerabilities (6 low, 24 moderate, 25 high) — CI only fails on critical
- Two parallel CI pipelines risk divergence

**Depends on:** Items 003-004 (evals must exist before they can gate PRs)
