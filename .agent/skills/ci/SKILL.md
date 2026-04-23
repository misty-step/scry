---
name: ci
description: |
  Run and repair scry's actual quality gates. The ship gate is GitHub
  Actions Quality Checks merge-gate, with pnpm local parity and Lefthook
  as defense in depth. This repo does not use Dagger; never scaffold it
  or describe it as canonical here. Bounded self-heal may fix lint,
  formatting, stale generated artifacts, and obvious focus markers, but
  must not lower coverage, weaken TypeScript, disable tests, or bypass
  audit.
  Use when: "run ci", "check ci", "fix ci", "audit ci", "is ci passing",
  "run the gates", "why is ci failing", "strengthen ci", "tighten ci",
  "ci is red", "gates failing".
  Trigger: /ci, /gates.
argument-hint: "[--audit-only|--run-only]"
---

# /ci

Confidence in scry's gates, not generic pipeline theater. This repo's
quality contract is pnpm + GitHub Actions + Lefthook. There is no Dagger
pipeline in scry; do not scaffold Dagger, ask for Dagger, or treat missing
Dagger as a gap.

Stops at a trustworthy green gate. Does not review semantics (use
`/code-review`), does not shape work (use `/shape`), does not deploy, and
does not run forbidden deploy-coupled commands.

## Modes

- Default: audit live config, run the local parity gate, add required
  checks by touched surface, self-heal bounded failures, then report.
- `--audit-only`: inspect config and report gate coverage; do not run
  checks or edit files.
- `--run-only`: skip the audit and run the appropriate gates for the
  current diff.

## Load-Bearing Gate

Quote this exactly when naming the ship gate:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Meaning:

- **Authoritative ship gate:** `.github/workflows/ci.yml` job
  `merge-gate` in workflow `Quality Checks`. It depends on `quality` and
  `test` and fails on either failure or cancellation.
- **Local parity:** run `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`
  before claiming green. `pnpm typecheck` is the CI script alias for
  `tsc --noEmit`; local parity uses the explicit command from the repo
  brief.
- **Additive checks:** run `pnpm test:contract` when `convex/**` changes,
  `pnpm build` when package/build/workflow/deploy surfaces change, and
  `pnpm audit --audit-level=critical` when `package.json` or lockfile
  changes.
- **Hooks:** Lefthook pre-commit and pre-push are defense in depth, not
  replacements for the ship gate.
- **Advisory workflows:** security, preview smoke, and nightly E2E
  provide signal outside the ship gate. Treat failures seriously, but do
  not redefine the ship gate around them.

## Source Files To Read First

Read these before auditing or changing CI behavior:

- `.spellbook/repo-brief.md`
- `package.json`
- `.github/workflows/ci.yml`
- `.github/workflows/security.yml`
- `.github/workflows/preview-smoke-test.yml`
- `.github/workflows/nightly-e2e.yml`
- `lefthook.yml`
- `vitest.config.ts`

If these conflict with docs or README, live config and the repo brief win.

## What Exists Today

- `packageManager` is `pnpm@10.12.1`; Node in CI is `20.20.0`, with
  engines requiring Node `>=20.19.0`.
- CI installs with `pnpm install --frozen-lockfile`.
- `Quality Checks / quality` runs Node version verification, `pnpm lint`,
  `pnpm typecheck`, `pnpm audit --audit-level=critical`, and a `git grep`
  guard against `(describe|test|it).only` in test files.
- `Quality Checks / test` runs `pnpm test:ci`, then requires both
  `coverage/coverage-summary.json` and `coverage/coverage-final.json`.
- Vitest uses `happy-dom`, V8 coverage, `verbose` reporter, sequential
  workers, and 75% thresholds for lines, functions, branches, and
  statements across `lib/**`, `convex/**`, and `hooks/**`.
- Pre-commit runs TruffleHog on staged files, topology hygiene,
  `pnpm exec tsc --noEmit` for TypeScript changes, and lint-staged
  (`prettier --write`, `eslint --fix` for JS/TS).
- Pre-push runs `./scripts/test-changed-batches.sh` and
  `pnpm test:contract`.
- `Security Audit` runs Gitleaks and Trivy, uploads SARIF best-effort,
  and runs `pnpm audit --audit-level=high` with `continue-on-error`.
- `Preview Deployment Smoke Test` waits for a Vercel preview, checks
  `/api/health`, and runs `pnpm test:e2e:smoke` when the preview is
  reachable. It can skip when preview deployment protection blocks access.
- `Nightly E2E + Coverage` is scheduled/manual. It runs `pnpm
  test:coverage` and full Playwright against `https://scry.vercel.app`.

## Process

### Phase 1 - Audit

Skip only with `--run-only`.

1. Read the source files above.
2. Check whether the current branch changes gate surfaces:
   `package.json`, `pnpm-lock.yaml`, `next.config.ts`, `vercel.json`,
   `.github/workflows/**`, `lefthook.yml`, `vitest.config.ts`, `convex/**`,
   test files, or generated Convex API types.
3. Verify the ship gate still matches the quoted repo-brief contract.
4. Verify coverage thresholds remain at least the current 75/75/75/75
   floor and that CI still requires coverage JSON artifacts.
5. Verify hooks remain defense in depth: pre-commit catches secrets,
   topology, typecheck, lint/format; pre-push catches changed batches and
   Convex contracts.
6. If a gate is missing because the live config drifted, restore the
   repo-brief contract directly unless the change is clearly intentional
   and user-approved.

Audit output should distinguish:

- `ship-gate`: `.github/workflows/ci.yml` `Quality Checks` `merge-gate`
- `local-parity`: commands run locally before claiming green
- `hooks`: Lefthook pre-commit/pre-push checks
- `additive`: checks required by touched surfaces
- `advisory`: security, preview smoke, nightly E2E

### Phase 2 - Run

Run from the repo root with pnpm.

Default local parity:

```bash
pnpm lint && pnpm tsc --noEmit && pnpm test:ci
```

Additive by diff:

```bash
pnpm test:contract
```

Run when `convex/**` changes.

```bash
pnpm build
```

Run when dependencies, build config, deploy config, or workflow surfaces
change, including `package.json`, `pnpm-lock.yaml`, `next.config.ts`,
`vercel.json`, and `.github/workflows/**`.

```bash
pnpm audit --audit-level=critical
```

Run when `package.json` or `pnpm-lock.yaml` changes, and never bypass it.

Optional diagnostic checks:

```bash
./scripts/test-changed-batches.sh
pnpm test:e2e:smoke
pnpm test:e2e:full
```

Use changed-batches to mirror pre-push locally. Use Playwright only when
the task needs browser confidence or a workflow failure points there; E2E
is not the default ship gate.

Do not run these without explicit operator approval:

```bash
pnpm build:local
pnpm build:prod
pnpm convex:deploy
./scripts/deploy-production.sh
```

Also do not run production migration scripts or commands that mutate
non-local Convex/Vercel state.

### Phase 3 - Self-Heal

Fix bounded, mechanical failures; stop and diagnose contract failures.

Self-healable:

- Format/lint drift. Run targeted `prettier --write`, `eslint --fix`, or
  `pnpm format` when the blast radius is acceptable, then rerun `pnpm lint`.
- Focused tests. Remove accidental `.only` markers while preserving the
  test body, then rerun the relevant test and the `.only` grep.
- Stale lockfile after an intentional dependency edit. Run `pnpm install`
  to update `pnpm-lock.yaml`, then run `pnpm audit --audit-level=critical`.
- Stale generated Convex API types after Convex work, when the repo's
  normal local generation command is already established for the task.
- Obvious typos/import mistakes introduced by the current diff.

Not self-healable without a real code fix:

- Lowering Vitest coverage thresholds below 75% or expanding exclusions to
  hide untested runtime code.
- Weakening TypeScript strictness, adding `any`/casts to silence a real
  contract error, or changing `tsconfig` to pass.
- Deleting, skipping, or loosening a failing test to make CI green.
- Adding `continue-on-error`, `|| true`, `--passWithNoTests`, audit ignore
  flags, or other bypasses to required gates.
- Downgrading `pnpm audit --audit-level=critical` or ignoring an audit
  failure. Fix the dependency graph or escalate with the advisory excerpt.
- Running deploy-coupled commands as verification.

Bound retries to three self-heal attempts per gate. After that, stop and
report the failing command, excerpt, files implicated, and likely root
cause.

### Phase 4 - Verify

After any edit, rerun the smallest failing gate first, then the full local
parity command. Add the additive checks required by touched surfaces. Do
not claim green from a partial run unless the output explicitly says it is
partial.

## Output

Keep reports short and operational:

```markdown
## /ci Report
Ship gate: GitHub Actions `Quality Checks` `merge-gate`
Local parity: pnpm lint && pnpm tsc --noEmit && pnpm test:ci
Additive checks: pnpm test:contract (convex changed), pnpm audit --audit-level=critical (lockfile changed)
Hooks: Lefthook pre-commit/pre-push are defense in depth, not replacements
Advisory: security/preview/nightly not run
Self-heal: removed accidental test.only; reran affected test
Final: green
```

For red:

```markdown
## /ci Report - RED
Gate: pnpm test:ci
Failure: tests/scheduler.test.ts > schedules due concepts
Excerpt: expected due count 3, got 2
Classification: behavior failure, not self-healable
Next action: fix scheduler contract or update the test with explicit product approval
```

## Anti-Patterns

- Calling Dagger canonical, scaffolding Dagger, or treating missing Dagger
  as a CI gap.
- Reporting only hook success as ship-ready. Hooks are defense in depth;
  the ship gate remains `Quality Checks` `merge-gate`.
- Treating preview smoke, nightly E2E, or security workflow behavior as
  the merge gate. They are advisory unless branch protection changes and
  the repo brief is updated.
- Claiming local parity after `pnpm qa` alone. `pnpm qa` is close, but the
  repo brief names the explicit local parity command.
- Bypassing audit because another workflow marks audit advisory.
- Lowering quality gates, broadening coverage exclusions, or suppressing
  test/type failures to get green.
- Running production deploy or migration commands during CI verification.
