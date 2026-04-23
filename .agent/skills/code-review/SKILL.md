---
name: code-review
description: |
  Parallel multi-agent code review for scry. Launch reviewer team, synthesize
  findings, auto-fix blocking issues, loop until clean.
  Use when: "review this", "code review", "is this ready to ship",
  "check this code", "review my changes".
  Trigger: /code-review, /review, /critique.
argument-hint: "[branch|diff|files]"
---

# /code-review

scry code review is a Tamiyo-grade archive check: review the diff, preserve
pure FSRS, protect Convex bandwidth, enforce backend-before-frontend, and cite
the real ship gate. You are the marshal. Read the diff, select reviewers,
dispatch every independent review tier in parallel, synthesize concrete
file:line findings, fix blockers, and loop until clean.

## Load-Bearing Gate

Cite this repo-brief gate statement consistently in every final review:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Do not substitute `pnpm qa`, README guidance, old context notes, or advisory
workflows for that gate. If the diff changes scripts, workflows, hooks,
dependencies, or verification language, review it against this exact contract.

Forbidden without explicit operator approval: `pnpm build:local`,
`pnpm build:prod`, `pnpm convex:deploy`, `./scripts/deploy-production.sh`,
production migration scripts, or anything that changes non-local Convex/Vercel
state. A review may flag these paths; it must not run them as routine
verification.

## Required Context

Before dispatching reviewers, read these anchors enough to brief them:

- `.spellbook/repo-brief.md` and `AGENTS.md`
- `docs/guides/convex-bandwidth.md`
- `docs/adr/0001-optimize-bandwidth-for-large-collections.md`
- `backlog.d/007-agent-ready-architecture.md`
- `vitest.config.ts` and `package.json`

Use live config as source of truth: `package.json`, `.github/workflows/**`, and
`lefthook.yml` outrank docs. Treat `.claude/context.md`, `README`, and older
provider notes as historical unless verified in code.

## Marshal Protocol

1. **Read the diff.** Resolve base to `main` or `master` unless the user
   supplied one, then inspect `git diff $BASE...HEAD` and
   `git diff --name-only $BASE...HEAD`. Classify scry surfaces: Convex schema,
   queries, mutations, FSRS scheduling, review UI, AI generation/IQC,
   tests/evals, dependencies, deploy scripts, migrations, and hot monoliths.

2. **Select internal reviewers deterministically.** Do not hand-pick the core
   bench. Run the algorithm in `references/bench-map.yaml`:
   `git diff --name-only $BASE...HEAD`, start from `default`, union `add`
   agents for matching globs, de-dupe, cap at 5, keep `critic` pinned. Read
   `references/bench-map.md` and `references/internal-bench.md`. Manual
   ad-hoc additions are allowed only for a concrete scry risk not captured by
   the map; document the swap in the synthesis.

3. **Give every reviewer the scry packet.** Each prompt must include:
   - The exact diff scope and base.
   - The gate statement from **Load-Bearing Gate**.
   - Pure FSRS is non-negotiable: no daily limits, comfort-mode shortcuts,
     artificial caps, or "FSRS but better" scheduling changes.
   - Convex runtime reads must be bounded: no unbounded `.collect()` on growing
     tables, no client-side filtering/sorting after full scans, use indexes
     plus `.take()` or `.paginate()`, and return truncation signals when capping.
   - Backend-first Convex sequencing: schema/query/mutation in `convex/`,
     generated API/type readiness, then UI wiring.
   - Destructive mutations need reverses: `archive`/`unarchive`,
     `softDelete`/`restore`; hard delete requires explicit confirmation UX.
   - Tests must avoid internal mocks. Boundary mocks are fine for external
     services, clocks, random, and network. Internal modules need real
     implementations or contract-honoring fakes.
   - Hot monoliths are known debt: `components/agent/review-chat.tsx`,
     `convex/concepts.ts`, `convex/aiGeneration.ts`, and `convex/iqc.ts`.
     Diffs touching them should extract focused modules without public API
     changes, not grow the monolith or rewrite business logic.
   - Deployment/migration safety: no routine non-local deploys, no irreversible
     schema removal without optional-field migration sequencing, and no scripts
     that blur local verification with production state changes.

4. **Dispatch all three tiers in parallel:**

   | Tier | What | How |
   |------|------|-----|
   | Internal bench | 3-5 Explore sub-agents with philosophy lenses | Agent tool, scry packet plus tailored lens |
   | Thinktank review | 10 agents, 8 model providers | `thinktank review`; see `references/thinktank-review.md` |
   | Cross-harness | Codex + Gemini CLIs, skipping whichever you are | See `references/cross-harness.md` |

   Thinktank-specific rule: wait for the process to exit, or for
   `trace/summary.json` to reach `complete` or `degraded` with a
   `run_completed` event in `trace/events.jsonl`, before consuming the run.
   Mid-run output directories are not final artifacts.

5. **Synthesize.** Collect all outputs. Deduplicate across tiers. Rank by
   severity: blocking correctness/security/data-loss/regression risks first,
   then important architecture/testing/operability risks, then advisory style.
   Findings must be concrete, fixable, and cited with file:line references.

6. **Verdict.** If no blocking findings remain, verdict is **Ship**. If
   blocking findings exist, enter the fix loop. A review that has not evaluated
   the gate statement and the scry packet is incomplete, even if tests pass.

## Scry Blocking Lenses

Flag these as blocking or conditional unless the diff proves they are harmless:

- **Pure FSRS drift:** daily review caps, "easy" affordances that bypass the
  curve, manual rescheduling shortcuts, comfort limits, or algorithmic
  optimizations outside the pure FSRS contract.
- **Unbounded Convex reads:** runtime `.collect()` on growing tables,
  `.collect().filter()`, `.collect().sort().slice()`, bulk `Promise.all` over
  unbounded query results, missing indexes, missing `.take()`/pagination, or
  capped responses that hide truncation.
- **Frontend-first Convex usage:** UI calls or generated API references for
  schema/query/mutation behavior not implemented in `convex/`, or stale
  generated types treated as truth over generated API readiness.
- **Irreversible mutation semantics:** destructive UX without reverse mutation,
  hard delete without explicit confirmation, schema removals without the
  optional-field migration path, or mutations that cannot be audited/restored.
- **Internal mocks:** `vi.mock("./...")`, `vi.mock("../...")`,
  `jest.mock("@/owned-module")`, internal `__mocks__/`, or stubs that assert
  wiring instead of behavior. Replace with real owned modules or realistic fakes.
- **Hot monolith growth:** changes that add unrelated responsibility to
  `review-chat.tsx`, `concepts.ts`, `aiGeneration.ts`, or `iqc.ts` rather than
  extracting pure functions/hooks/modules with zero public API change.
- **Deployment/migration safety:** scripts or docs that make production deploys,
  Convex deploys, or non-local migrations look routine; migrations without
  dry-run/backfill/diagnostic verification; dependency changes without audit.

## Reviewer Prompt Templates

Use these as starting points; tailor file paths and line ranges to the diff.

**Critic baseline:**

```text
Role: scry release critic.
Objective: Review $BASE...HEAD for blocking correctness, data-loss, security,
and ship-gate failures.
Scope: Changed files: <list>. Focus only on this diff.
Scry anchors: cite the gate statement exactly; preserve pure FSRS; enforce
bounded Convex reads; backend-before-frontend; reversible destructive mutations;
no internal mocks; hot monoliths should shrink or stay stable; no routine
production deploy/migration commands.
Output: Verdict Ship/Conditional/Don't Ship. Findings first, each with
file:line, severity, and a concrete fix. No style-only comments.
Boundaries: Read-only. Do not edit files or run forbidden deploy commands.
```

**Ousterhout architecture:**

```text
Role: scry architecture reviewer.
Objective: Find shallow modules, hidden coupling, stale public API contracts,
and monolith growth in $BASE...HEAD.
Scope: Pay special attention to Convex API boundaries, generated API/type
readiness, and BACKLOG-007 hot files.
Output: Blocking architecture findings with file:line and simpler interface
recommendations. Prefer deletion/extraction over new configuration.
Boundaries: Read-only. Do not request rewrites unrelated to changed code.
```

**Beck testing:**

```text
Role: scry TDD reviewer.
Objective: Verify changed behavior is covered by behavior tests without
internal mocks.
Scope: Apply vitest.config.ts rules: happy-dom, coverage over lib/convex/hooks,
single-worker stability, and known Convex exclusions. For convex/** changes,
expect pnpm test:contract. For dependency/config/workflow changes, expect
build/audit checks from the gate statement.
Output: Missing or brittle tests, internal mock violations, focused test
hazards, and exact verification commands needed.
Boundaries: Read-only. Boundary mocks for external services are acceptable.
```

**Carmack shippability/perf:**

```text
Role: scry shippability reviewer.
Objective: Review whether the diff will work for large Anki-scale collections
and production operation without cleverness.
Scope: Convex query shape, FSRS scheduling path, review UI latency, deployment
scripts, migrations, and dependency changes.
Output: Concrete runtime risks with file:line, expected failure mode, and direct
fix. Treat unbounded reads and production-state side effects as blockers.
Boundaries: Read-only. Do not run production deploy or migration commands.
```

**Grug simplification:**

```text
Role: scry complexity hunter.
Objective: Find code that makes the memory archive harder to preserve than the
feature requires.
Scope: New abstractions, branching, compatibility shims, duplicated UI/backend
flows, and growth in the four hot monoliths.
Output: Findings where deleting or extracting code reduces risk without
changing behavior. No taste comments.
Boundaries: Read-only. Keep the review tied to the diff.
```

**A11y/UI when user-facing routes/components change:**

```text
Role: scry review-surface accessibility reviewer.
Objective: Verify changed React/Next.js UI remains usable for the Willow review
flow and other changed surfaces.
Scope: .tsx/.jsx/app/components changes in $BASE...HEAD. Check keyboard flow,
focus management, labels, disabled states, responsive behavior, and hardcoded
review artifact rendering.
Output: User-facing blockers with file:line and live verification notes.
Boundaries: Read-only unless the marshal later assigns a fix builder.
```

## Fix Loop

For each blocking finding, spawn a builder sub-agent with the exact file:line,
the failing invariant, and the smallest strategic fix. Builders must preserve
scry semantics: pure FSRS, bounded Convex reads, backend-first Convex flow,
reversible destructive mutations, no internal mocks, and no public API changes
for BACKLOG-007-style extraction unless the user explicitly requested them.

After fixes land, re-dispatch all three review tiers. Full re-review, not a
spot-check. Loop until no blocking findings remain. Max 3 iterations; escalate
to the user if still blocked.

## Live Verification

Trigger live verification when the diff touches user-facing surfaces:
`.tsx`, `.jsx`, `app/**`, `components/**`, review UI, auth/subscription UI, or
API routes that feed the UI.

At least one reviewer must exercise the affected route/component where feasible.
For scry, `/` is the review home and `/agent` redirects to `/`; do not assume
stale navigation from old docs. Ship is blocked until live verification passes
or the review explains why it is not applicable.

Skip live verification for pure docs, tests-only, backend-only Convex changes
with no user-facing surface, and config-only changes.

## Verification Commands

Use `pnpm`, not Bun or npm. Match the gate statement:

```bash
pnpm lint
pnpm tsc --noEmit
pnpm test:ci
```

Add checks by diff surface:

```bash
pnpm test:contract          # when convex/** changes
pnpm build                  # when package, lockfile, Next/Vercel, or workflow surfaces change
pnpm audit --audit-level=critical  # when dependencies or lockfile change
```

Never lower coverage thresholds, lint strictness, type strictness, hook checks,
or CI requirements to make a review pass. Focused tests (`.only`) are blocked by
the ship gate and should be reported if introduced.

## Plausible-but-Wrong Patterns

LLMs optimize for plausible scry-shaped code. Reviewers must hunt for:

- FSRS module names with non-FSRS scheduling behavior.
- Convex code that passes small fixtures but explodes for 10,000+ concepts.
- UI affordances that feel helpful but hide real due learning debt.
- Generated API imports that compile locally while the backend contract is
  missing, stale, or untested.
- Tests that mock owned modules and prove only that mocks were called.
- Extraction diffs that rename monolith chunks without reducing coupling.
- Migration/deploy scripts that conflate local verification with production
  state changes.

## Simplification Pass

After review passes, if the diff is large or touches a hot monolith:

- Look for code that can be deleted.
- Collapse single-use abstractions and pass-through layers.
- Prefer extracted pure helpers/hooks over new orchestration frameworks.
- Preserve public Convex API signatures unless the diff explicitly changes the
  contract and tests cover the migration.
- Keep every changed line traceable to the requested behavior or review fix.

## Review Scoring

After the final verdict, append one JSON line to `.groom/review-scores.ndjson`
in the target project root (create `.groom/` if needed):

```json
{"date":"2026-04-23","pr":42,"correctness":8,"depth":7,"simplicity":9,"craft":8,"verdict":"ship","providers":["claude","thinktank","codex","gemini"]}
```

- Scores (1-10) reflect cross-provider consensus, not any single reviewer.
- `pr` is the PR number, or `null` when reviewing a branch without a PR.
- `verdict`: `"ship"`, `"conditional"`, or `"dont-ship"`.
- `providers`: which review tiers contributed.
- This file is committed to git when review artifacts are part of the branch.

## Verdict Ref (git-native review proof)

After scoring, record the verdict as a git ref if `scripts/lib/verdicts.sh`
exists in this repo:

```bash
source scripts/lib/verdicts.sh
verdict_write "<branch>" '{"branch":"<branch>","base":"<base>","verdict":"<ship|conditional|dont-ship>","reviewers":[...],"scores":{...},"sha":"<HEAD-sha>","date":"<ISO-8601>"}'
```

- Write on every review, not just `ship`; `dont-ship` blocks landing.
- `sha` must be `git rev-parse HEAD` at the time of review. New commits make
  the verdict stale.
- Verdict refs live under `refs/verdicts/<branch>` and sync via `git push/fetch`.
- Also write `.evidence/<branch>/<date>/verdict.json` for browsability.
- The escape hatch (`SPELLBOOK_NO_REVIEW=1`) belongs to callers, never inside
  `/code-review`.

Skip this step if `scripts/lib/verdicts.sh` does not exist.

## Gotchas

- **Self-review leniency:** reviewers must be separate sub-agents or external
  tiers, not the builder grading itself.
- **Review scope drift:** review `$BASE...HEAD`, not the whole repo. Use the
  required anchors only to understand invariants.
- **Gate drift:** always cite the GitHub Actions `Quality Checks` `merge-gate`
  statement above; do not invent a new "required checks" list.
- **Skipping tiers:** internal bench alone is same-model groupthink. Thinktank
  and cross-harness runs provide real model/harness diversity. If a tier fails,
  record the failure and continue with the others.
- **Misreading Thinktank:** `review.md`, `summary.md`, and `agents/*.md` may not
  exist until late. Watch stderr progress or `trace/summary.json`, not only the
  directory listing.
- **Treating all concerns equally:** style preferences do not block shipping.
  Pure FSRS drift, unbounded Convex reads, unsafe destructive mutations, broken
  gate semantics, and production-state side effects do.
- **README drift:** README can lag; package scripts, workflows, lefthook,
  repo brief, and AGENTS.md decide.
