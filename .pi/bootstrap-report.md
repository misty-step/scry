# Pi Local Bootstrap Report (scry)

Generated: 2026-02-26
Scope: repo-local `.pi/` foundation refresh for scry (pnpm, Next.js, React, Convex, Tailwind v4, TypeScript, Vitest).

## Repo Signals (evidence-backed)
- Branch-protected CI reality is `quality` + `test`; commands map to `pnpm lint`, `pnpm tsc --noEmit`, `pnpm audit --audit-level=critical`, and `pnpm test:ci` (`.github/workflows/ci.yml`).
- Deploy-coupled scripts exist (`build:local`, `build:prod`, `convex:deploy`) and remain explicit opt-in only (`package.json`, `CLAUDE.md`).
- Convex guardrails are explicit: ban unbounded `.collect()`, require index + bounded reads (`docs/guides/convex-bandwidth.md`).
- FSRS doctrine is non-negotiable: no comfort features and no daily limits (`README.md`, `CLAUDE.md`).

## Lane Evidence Highlights
- [context-bridge] surfaced context drift risk (malformed root `AGENTS.md`, conflicting advisory docs); this foundation enforces explicit source-of-truth ordering in persona + agents.
- [workflow-critic] identified prior worker/CI mismatch and silent-risk workflow hazards (dual lefthook config, Cerberus skip behavior, nightly prod E2E exposure); this foundation aligns worker checks with protected CI and keeps residual risks explicit.
- [implementation-critic] flagged phantom `memory_context` conditionals and brittle prompt routing; prompts now use concrete local warmup (`.pi/persona.md`, `.pi/state/session-handoff.json`) and deterministic pipeline selection.
- [docs-research] reinforced stack specifics (React 19, Tailwind v4, Next.js caching defaults); persona captures these as constraints where they affect implementation choices.
- [ambition-pass] proposed self-evolving memory loops; implemented here as a bounded experimental pipeline (`scry-seam-retrospective-v1`) with explicit kill criteria.

## Adopt / Bridge / Ignore
### Adopt
- Existing planner/worker/reviewer triad and local prompt entry points.
- Backend-first Convex workflow and Pure FSRS invariants from `CLAUDE.md`.
- CI-parity verification baseline from `.github/workflows/ci.yml`.

### Bridge
- Convert broad/stale context into deterministic source-of-truth policy inside `.pi/persona.md`.
- Route delivery through explicit repo pipelines (`scry-delivery-v2`, `scry-convex-delivery-v1`).
- Convert reflective ambition into a bounded 72h experiment (`scry-seam-retrospective-v1`).

### Ignore (baseline)
- Unscoped memory macros and implicit runtime conditionals.
- Non-blocking advisory workflows (lighthouse/nightly/preview) as default local merge gates.
- Any deploy/migration automation without explicit operator approval.

## Safety and Quality Controls
- Default local verification mirrors branch-protected CI gates.
- Convex changes require contract-test reinforcement.
- Reviewer explicitly checks Convex bandwidth, FSRS doctrine, and operational safety.
- Teams/pipelines reference only local agents (`planner`, `worker`, `reviewer`) for auditable execution.

## Single Highest-Leverage Addition
- Idea: `scry-seam-retrospective-v1` — a constrained, evidence-backed guardrail accretion loop that turns one resolved failure into one reusable local lesson.
- Source lane: ambition-pass (self-evolving memory concept), constrained by implementation-critic (drift/noise controls).
- Why now: lane evidence shows repeated context drift and silent failure modes; a bounded learning loop compounds repo-specific intelligence without adding rigid scripts.
- 72h validation experiment:
  1. Run the pipeline after each completed delivery task for 72 hours.
  2. Require reviewer approval for every lesson before it is kept.
  3. Measure duplicate mistake rate (CI parity misses, Convex bandwidth violations, backend-first sequencing errors) and median overhead per task.
  4. Success threshold: >=30% reduction in repeated mistakes with <=10 minutes overhead per task.
- Kill criteria:
  - >20% of generated lessons are rejected as noise or duplicates, or
  - median overhead exceeds 15 minutes/task, or
  - any proposed lesson weakens safety or quality gates.

## Quality Gate Scorecard
- Gate pass: yes
- Ambition score: 90/100 (pass)
  - novelty: 5/5
  - feasibility: 5/5
  - evidence: 5/5
  - rollbackability: 3/5
- Consensus score: 96/100 (pass)
