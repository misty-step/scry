---
name: shape
description: |
  Shape a raw idea into something buildable. Product + technical exploration.
  Spec, design, critique, plan. Output is a context packet.
  Use when: "shape this", "write a spec", "design this feature",
  "plan this", "spec out", "context packet", "technical design".
  Trigger: /shape, /spec, /plan, /cp.
argument-hint: "[idea|issue|backlog-item] [--spec-only] [--design-only]"
---

# /shape

Shape a raw idea into a buildable, file-backed context packet for scry. The
packet is the artifact `/deliver` and `/implement` consume. It is not a chat
summary and it is never written after implementation as justification.

Scry is an agentic Anki killer built around concept-level FSRS, Convex as the
source of truth, eval-backed generation, and Willow as the review experience at
`/`. A valid shape protects that product thesis before it protects any proposed
feature.

## Required Reads

Always read:

- `.spellbook/repo-brief.md` for the product spine, known debts, and handoff
  gate.
- The requested `backlog.d/<id>-*.md` item and any existing
  `backlog.d/<id>-*.ctx.md` packet.
- Nearby context packets in `backlog.d/*.ctx.md` when matching style or
  deciding packet granularity.

Read by domain:

- `vision.md` for north-star product language.
- `convex/schema.ts` before shaping data, lifecycle, review, IQC, generation,
  subscription, or analytics changes.
- `docs/guides/convex-bandwidth.md` before shaping Convex queries, scans, bulk
  mutations, or dashboard/library flows.
- `evals/promptfoo.yaml`, `convex/lib/promptTemplates.ts`, and
  `convex/lib/generationContracts.ts` before shaping prompt, generation,
  grading, content-type, or LLM quality changes.
- `convex/fsrs/engine.ts` and `convex/fsrs/conceptScheduler.ts` before shaping
  scheduling or review-selection behavior.

If context gathering would exceed a short bounded read, dispatch fresh-context
exploration agents when available: one product/code mapper and one prior-art or
design critic. Keep their output tied to concrete files.

## Non-Negotiable Gates

### 1. Product gate before code

Do not write or green-light implementation until the product direction is
locked. Start with the user outcome, not the requested mechanism:

- How does this improve the core loop: generate content, preserve atomic
  concepts, review due knowledge, record feedback, improve future retrieval?
- Does it strengthen the "agentic Anki killer" thesis or add another surface to
  maintain?
- Does it preserve pure FSRS: no daily caps, no comfort-mode shortcuts, no
  ease buttons that bypass the curve, no "FSRS but better" scheduling?
- Is this an IQC/library problem, a review-session problem, a generation
  quality problem, or a schema/API problem?

If the idea fights pure FSRS or hides learning debt, reject or reframe it before
solution design.

### 2. Alternatives before convergence

Every non-trivial shape must diverge before choosing. Include at least three
structurally distinct options:

- Minimal viable: smallest backend-first path that proves the behavior.
- Product-complete: the best durable version for Willow, IQC, or generation.
- Inversion/deletion: remove a surface, move responsibility to IQC, encode it
  as an eval, or decide not to build.

For each option, state how it fails differently. Do not converge until the user
ratifies the product direction and the technical direction. Recommend one; do
not merely list choices.

### 3. Backend-first Convex

For any behavior that touches persisted state, the shape sequence is:

1. `convex/schema.ts` contract and migration path.
2. Convex query/mutation/action changes with indexes and bounded reads.
3. Generated API/type readiness.
4. UI wiring.

Runtime Convex paths must not use unbounded `.collect()`. Shape queries around
indexes, `.take()`, pagination, batch sizes, and truncation signals. Destructive
actions need reverses: `archive`/`unarchive`, `softDelete`/`restore`; hard
delete requires explicit confirmation UX.

### 4. Eval-backed generation

Generation changes are not done because the prompt "looks better." A shape that
changes prompts, generation contracts, grading, content types, or model output
must name:

- Prompt/template files touched, usually `convex/lib/promptTemplates.ts` and
  `evals/prompts/**`.
- Contract schemas touched, usually `convex/lib/generationContracts.ts`.
- Promptfoo config and cases, usually `evals/promptfoo.yaml` or a new sibling
  config when variables/output shape differ.
- Baseline capture expectations: pass rate, rubric scores, latency, and any
  credentials required to run evals.

If the eval cannot be written, the generation behavior is not shaped.

### 5. Schema-driven review UI

Review UI shapes should bias toward typed, deterministic backend output and a
thin renderer. The current direction is a Zod-validated `renderSpec`
discriminated union plus an exhaustive component registry, not a general
LLM-generated UI framework and not another hardcoded `if/else` artifact chain.

When shaping review artifacts, include the backend data shape, the frontend
schema, registry entries, fallback behavior, and tests that prove adding a new
artifact requires only the schema variant, component, and registry entry.

### 6. File-backed oracles

The output of `/shape` is a context packet on disk, normally
`backlog.d/<id>-<slug>.ctx.md`. Oracles must be executable commands where
possible, with observable/manual checks only for UX that scripts cannot decide.
See `references/executable-oracles.md`.

## Problem Diamond

Use this before solution design.

1. **Name the memory outcome.** Tie the request to durable recall, generation
   quality, IQC cleanliness, review flow, or operational safety.
2. **Five-whys the framing.** If the request says "add X," ask whether the real
   outcome is better scheduling, clearer review artifacts, fewer dead routes,
   stronger evals, or a simpler schema.
3. **Classify the workstream.** Pick the primary lane: pure FSRS/review,
   Convex data model, generation/evals, schema-driven UI, IQC/library, quality
   gate, or architecture extraction.
4. **Surface invariant conflicts.** Explicitly call out FSRS purity, Convex
   bandwidth, backend-first sequencing, destructive reversibility, and eval
   requirements.
5. **Lock product direction.** Ask one question at a time until the user
   ratifies the outcome and non-goals.

Do not enter the solution diamond while the product gate is unresolved.

## Solution Diamond

1. **Map current state.** Cite concrete files, line ranges when useful, current
   types/schema, current tests/evals, and known debts from the repo brief.
2. **Diverge.** Produce the alternatives above. For each, include touched
   layers, acceptance oracle, risks, and why it might be wrong.
3. **Critique.** For M+ effort, use the design bench when available:
   Ousterhout for module depth and hidden coupling, Carmack for shippability,
   Grug for complexity. For prompt/generation or external architecture, get a
   genuinely heterogeneous second voice via `/research`, web research, Gemini,
   or Thinktank when available.
4. **Converge.** Recommend one design and state the rejected alternatives.
5. **Ratify.** The user disposes. If they choose a different path, update the
   packet rather than silently absorbing the change.

## Context Packet Format

Mirror the existing `backlog.d/*.ctx.md` style: concrete inventories, tables
when useful, exact file paths, explicit not-touched lists, implementation
sequence, verification, and risks. Do not force every section when irrelevant,
but every packet needs a spec, constraints, sequence, and oracle.

```markdown
# Context Packet: <Title>

## Spec

<Product outcome, chosen approach, and key decisions. Name the user-visible
behavior and the durable invariant it protects.>

## Product Gate

- Outcome: <memory/review/generation/IQC/safety outcome>
- Non-goals: <scope that agents must not drift into>
- Pure FSRS check: <why this does not add caps, comfort shortcuts, or curve
  changes>
- Alternatives considered: <minimal, product-complete, inversion/deletion>
- Chosen direction: <recommendation ratified by user>

## Current State

- `<file>` - <current role, relevant line/function/component>
- `<file>` - <current test/eval/contract>

## Design Decisions

### <Decision>

<Decision plus rationale. Include rejected alternatives and why they fail.>

## Backend / Schema Plan

<Required for Convex-backed work. Include schema/index/migration/query/mutation
details. State "N/A" only when the work truly has no persisted state.>

## Eval / Oracle Plan

<Required for generation, grading, prompt, or quality changes. Include promptfoo
cases and baseline capture.>

## UI / Review Plan

<Required for review UI. Prefer renderSpec/schema/registry over hardcoded
artifact branches.>

## Implementation Handoff Gate

Quote this repo-brief gate statement exactly in every implementation handoff:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Forbidden without explicit operator approval: `pnpm build:local`,
`pnpm build:prod`, `pnpm convex:deploy`, `./scripts/deploy-production.sh`, and
production migration scripts.

## Implementation Sequence

1. <First coherent chunk. Backend/schema before UI when applicable.>
2. <Second chunk. Keep parallelizable work explicit.>
3. <Verification and cleanup chunk.>

## Verification

Commands that must exit 0:
- `pnpm lint`
- `pnpm tsc --noEmit`
- `pnpm test:ci`
- `<targeted command>` - <why this proves the changed behavior>

Add when applicable:
- `pnpm test:contract` - required for `convex/**`.
- `pnpm build` - required for build/config/workflow/dependency surfaces.
- `pnpm audit --audit-level=critical` - required for dependency or lockfile
  changes.
- `pnpm eval` or `npx promptfoo eval -c <config>` - required for prompt/eval
  changes when credentials are available.

Observable outcomes:
- <manual or browser-visible result that scripts cannot decide>

## Risks

- <Risk> - <mitigation or rollback>

## Files Touched

| File | Change |
|---|---|
| `<path>` | <planned change> |

## Not Touched

| File | Reason |
|---|---|
| `<path>` | <why this is intentionally out of scope> |
```

If the shape cannot provide a file-backed oracle, go back to the product gate.

## Domain Checklists

### Pure FSRS and review scheduling

- Anchor to `convex/fsrs/engine.ts`, `convex/fsrs/conceptScheduler.ts`, and
  concept-level `fsrs` fields in `convex/schema.ts`.
- Preserve `Rating.Good` for correct and `Rating.Again` for incorrect unless
  the shape is explicitly about scheduler semantics and has FSRS evidence.
- No daily limits, artificial due caps, comfort modes, or ease affordances.
- Due-count truth beats user comfort. If 300 concepts are due, the product must
  surface that debt honestly.

### Convex data and bandwidth

- `convex/schema.ts` is an API contract. Removals require optional-field
  migration: make optional, backfill/verify, then remove.
- Every query shape names its index and bound: `.take(n)`, `.paginate()`, batch
  size, or explicit small-table justification.
- Runtime `.collect()` over user-growth tables is a blocker.
- Return truncation or sampling signals when capping large results.

### Generation and evals

- Prompt changes must include promptfoo assertions, not only prose rubrics.
- Structural assertions come first: JSON validity, required fields, enum values,
  option/correct-answer consistency, latency budget.
- Quality rubrics mirror production scoring where possible: standalone clarity,
  distractor quality, explanation value, difficulty calibration.
- Content-type changes must cover concept synthesis and phrasing generation
  unless the shape explicitly isolates one stage.

### Schema-driven review UI

- Prefer deterministic backend tool results carrying a validated render spec.
- Use Zod discriminated unions and an exhaustive registry; TypeScript should
  fail when a new component variant is not registered.
- Unknown or invalid specs need a safe fallback, not a crashed review session.
- Do not import a broad generative UI framework unless the shape proves the
  current thin registry cannot express the requirement.

### Agent-ready architecture

- Extraction shapes move private helpers first and keep Convex-decorated
  `query`, `mutation`, `action`, `internalQuery`, `internalMutation`, and
  `internalAction` exports at their file paths unless the shape includes a
  complete internal API migration.
- Preserve public API imports with re-exports only as a temporary migration
  step and name when they are removed.
- Split monoliths by responsibility, not by chronology.

## Gotchas

- **Generic packet:** If the packet could apply to any Next.js + Convex app, it
  is not shaped for scry. Name Willow, concepts, phrasings, IQC, FSRS, promptfoo,
  renderSpec, or the exact files that make this repo different.
- **Code before product:** A technical plan without product ratification is not
  ready for `/implement`.
- **Alternatives in costume:** Three UI variants for the same backend design are
  one option. Diverge at responsibility boundaries.
- **Eval omitted:** Any prompt/generation change without promptfoo coverage is
  a guess.
- **Bandwidth handwave:** "Use pagination" is not enough. Name the index,
  page size, cap, and truncation signal.
- **Forbidden local build:** Existing old packets may mention
  `pnpm build:local`; new packets should use `pnpm build` unless the operator
  explicitly approves deploy-coupled commands.
- **Chat-only spec:** A shape that is not written to `backlog.d/*.ctx.md` is not
  durable enough for handoff.
