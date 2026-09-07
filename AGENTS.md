# Scry

Scry is the quiz-first consumer product: **Remember everything** across a
phone-first PWA, CLI, skill, MCP, and API over one capability system.
`README.md` is the current entrypoint; `VISION.md` is optional product context,
not a lock or a mandatory first read.

## Product and runtime

- All faces share typed capabilities and learning/grading semantics.
- Beta access is invite-gated with a visible waitlist and magic-link sign-in;
  there is no OAuth path. Machine faces use operator-gated service sessions.
  Public signup waits for bounded cost, privacy, reliability, and Stripe proof.
- The selected deployment direction and activation, recovery, and traffic
  procedure live in `docs/runbook.md`. Its source-writer barrier and observed
  cutover evidence determine runtime status; this instruction file is not a
  live host inventory.
- Crate names, Postgres identifiers, wire and telemetry literals, and
  `MEMORY_ENGINE_*` environment variables retain the old name. Renaming a
  storage, network, deployment, or compatibility boundary requires explicit
  current scope and migration proof.

## Architecture and boundaries

- `crates/memory-engine-core` is framework-free and persistence-free. Boundary
  crates own storage, ingestion, sessions, UI, identity, analytics, and model
  clients. Consumers own persistence of scheduler output.
- `.dagger/src/index.ts` is the only retained TypeScript surface. Browser JS
  under `crates/memory-engine-api/assets/` is the deliberate plain-JS exception;
  it has no build step.

## Sources of truth

- Work from the operator's current request. Check current code and overlapping
  work. Linear owns selected work and priorities; historical issues are context,
  not automatic intake. Record sanitized conclusions and exact evidence links
  in the work item and PR; close only when the stated outcome is verified.
- `package.json` scripts, `docs/qa/system.md`, and `docs/evals.md` own current
  verification and authorized model comparison. `docs/runbook.md` owns operating
  procedures; observed runtime/release records remain proof, not copied status
  prose. SLICE docs and exemplars are historical extraction context, not delivery oracles.
- Treat tests, types, code, docs, and lore as evidence to reconcile. Current
  requested behavior and observed state are authoritative.

## Runtime contracts

- The scheduler receives `ScheduleState` and returns the next state; consumers
  own persistence. `ScheduleState` is JSON-safe snake_case with
  `state: 0 | 1 | 2 | 3` and `last_review: number | null`. `ReviewUnitId` is
  opaque; the kernel does not infer concept or phrasing meaning.
- Prompt enum and grader dispatch changes require exhaustive Rust matches and
  grader tests in the same change. `Grader::grade()` returns one `GradeResult`
  with `rating` populated by the injected rating policy.
- Verdicts are `correct`, `close`, `wrong`, or `revealed`; other names map to
  these four and need a spec update.
- Do not add runtime dependencies without shaped scope and docs. Do not lower
  gates, bypass hooks, or claim unrun canaries as proof.
- No TypeScript runtime or tests belong outside `.dagger/`; the QA crate enforces
  this by extension, while the browser-JS exception above remains permitted.

## Gates and proof

- Use the current `package.json` scripts and `docs/qa/system.md` for the exact
  fast and ship-parity gates. Choose verification for the changed contract;
  prose-only edits need content and reference checks, not unrelated runtime or
  model runs. Live model comparison requires explicit spending authorization
  and follows `docs/evals.md`. Never weaken a required behavior gate, bypass
  hooks, or claim unexercised production proof.
- Test observable behavior with real repo-owned collaborators; mock only
  external boundaries such as network, clock, and model providers.
