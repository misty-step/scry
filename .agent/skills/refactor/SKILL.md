---
name: refactor
description: |
  Branch-aware simplification workflow for scry. Use it to delete dead surface
  area, extract the four known monoliths into agent-ready modules, or rationalize
  root topology without changing product behavior or public Convex APIs.
  Use when: "refactor this", "simplify this diff", "clean this up",
  "reduce complexity", "pay down tech debt", "make this easier to maintain",
  "extract this safely", "split the review chat", "split concepts",
  "split aiGeneration", "split iqc".
  Trigger: /refactor.
argument-hint: "[--base <branch>] [--scope <path>] [--report-only] [--apply]"
---

# /refactor

Refactor scry by removing complexity, not relocating it. The live complexity
map is narrow and concrete:

1. deletion-first simplification, proven by `backlog.d/001-delete-dead-code.md`
2. extract-only decomposition from `backlog.d/007-agent-ready-architecture.md`
3. root topology cleanup from `docs/architecture/root-topology-inventory.md`

Anything outside those lanes needs explicit evidence that it removes real
maintenance cost now. No speculative abstraction, no "while we are here"
rewrites, and no public Convex API changes.

## Repo Gate

Repo brief gate statement, cited verbatim:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Use `pnpm` only. Do not run `pnpm build:local`, `pnpm build:prod`,
`pnpm convex:deploy`, `./scripts/deploy-production.sh`, production migration
scripts, or anything that changes non-local Convex/Vercel state without
explicit operator approval.

## Branch-Aware Routing

Detect the current branch and base before analysis:

```bash
git rev-parse --abbrev-ref HEAD
git symbolic-ref --short refs/remotes/origin/HEAD | sed 's#^origin/##'
```

Fallback base order is `main`, then `master`. `--base <branch>` overrides
detection. If the current branch is detached, the base is ambiguous, or the
branch cannot be compared safely, stop and require `--base <branch>`.

- **Feature Branch Mode:** current branch differs from the primary branch.
  Simplify only the diff from `<base>...HEAD`, or the explicit `--scope`.
- **Primary Branch Mode:** current branch is the primary branch. Default to
  report/backlog shaping. Apply code only with `--apply`, and only for one
  bounded, low-risk deletion or extraction slice.
- **Backlog-aware branches:** if the branch name matches
  `<type>/<id>-<slug>`, read `backlog.d/<id>-*.md` and matching `.ctx.md`
  before deciding the refactor target.

## Non-Negotiables

- Deletion comes before extraction. Extraction comes before abstraction.
- `convex/schema.ts`, `convex/_generated/**`, and exported Convex functions are
  API contracts. Do not move, rename, or change public query/mutation/action
  signatures as part of a refactor.
- Convex-decorated exports must stay at their current file paths unless the
  task explicitly approves an API migration. Moving an `internalMutation`,
  `internalQuery`, or `internalAction` changes the generated `internal.*` path.
- BACKLOG-007 is extract-only: preserve behavior, keep wrappers thin, move
  private helpers into focused modules, and add unit coverage for newly
  exported pure functions.
- No comfort-mode FSRS changes, daily limits, generated "ease" shortcuts, or
  algorithmic tweaks. Refactoring must preserve the review curve.
- Backend-before-frontend still applies. For Convex-backed UI changes, update
  schema/query/mutation/action code first, validate types, then wire React.
- Do not combine topology cleanup with product refactors unless the branch or
  backlog item explicitly owns both.

## Scry Complexity Map

| Area | Evidence | Allowed refactor | Stable contract |
| --- | --- | --- | --- |
| Dead product surface | `backlog.d/001-delete-dead-code.md` deleted dev routes, legacy review flow, orphan tests, and unused imports | Delete unused routes/components/tests/libs outright | The agentic review experience remains the product; do not migrate logic from deleted legacy surfaces |
| `components/agent/review-chat.tsx` | 1389 lines; mixed review session orchestration, UI cards, message rendering, stats heatmap, callback tangle | Extract `use-review-session`, `active-session`, `action-panel-card`, `pending-feedback-card`, `chat-message`, `review-stats-panel`, and `review-chat-types` | `ReviewChat` remains the imported public component; behavior and review UX stay unchanged |
| `convex/concepts.ts` | 1295 lines; CRUD, FSRS scheduling, phrasing management, lifecycle state machines | Extract private helpers into `convex/lib/conceptLifecycle.ts`, `convex/lib/conceptReview.ts`, and `convex/lib/conceptScoring.ts` | All exported query/mutation/internal wrappers and `internal.concepts.*` paths stay in `convex/concepts.ts` |
| `convex/aiGeneration.ts` | 1202 lines; pipeline prep, validation, tracing, error handling, orchestration | Extract pure pipeline logic to `convex/lib/generationPipeline.ts`, tracing to `convex/lib/generationTracing.ts`, and failure handling to `convex/lib/generationErrorHandler.ts` | `processJob` and `generatePhrasingsForConcept` stay in `convex/aiGeneration.ts` with unchanged signatures |
| `convex/iqc.ts` | 827 lines; merge candidate scoring, prompt construction, snapshotting, action-card orchestration | Extract pure helpers/schemas to `convex/lib/iqcHelpers.ts` and adjudication/fetch helpers to `convex/lib/iqcAdjudication.ts` | `scanAndPropose`, action-card mutations/queries, and `internal.iqc.*` paths stay in `convex/iqc.ts` |
| Root topology | `docs/architecture/root-topology-inventory.md` names duplicate config surfaces and root doc sprawl | Resolve one duplicate config or one root-doc move per slice, with reference updates | Keep runtime source dirs and root workflow anchors stable unless the slice explicitly changes policy |

## Feature Branch Mode

Goal: simplify the branch before merge while preserving its stated behavior.

1. Map the delta:
   ```bash
   git diff --stat <base>...HEAD
   git diff --name-only <base>...HEAD
   git diff <base>...HEAD -- <scope-if-any>
   ```
2. Classify every candidate as deletion, extract-only decomposition, topology
   cleanup, or unsafe API migration. Drop unsafe API migrations unless the task
   explicitly asked for them.
3. Prefer the smallest change that removes the most code or state:
   deletion, then duplicated-helper consolidation, then extract-only module
   splits, then naming clarification. Abstraction is last and requires at least
   two live call sites plus a named invariant.
4. If the branch already touches one BACKLOG-007 hotspot, keep the refactor
   inside that hotspot. Do not start a second monolith extraction in the same
   branch unless the backlog item requires it.
5. Execute one bounded refactor unless the user requested a broader pass.
   Existing tests should pass without modification; add tests only for newly
   exported pure helpers or behavior that was previously unobserved.

## Primary Branch Mode

Goal: identify or execute one safe simplification slice from the repo map.

Default output is a report or shaped `backlog.d/` item, not code. With
`--apply`, make only one bounded change:

- pure deletion of unused surface area with import cleanup
- one BACKLOG-007 extraction slice that leaves public wrappers in place
- one root topology slice with documented canonical-source decision

Before recommending a new abstraction, prove deletion or extraction cannot solve
the problem. If the proposal needs a general framework, schema-driven UI system,
or cross-cutting orchestration layer, stop and shape it as a design item instead
of implementing it under `/refactor`.

## Extraction Playbooks

### Review Chat

Keep `components/agent/review-chat.tsx` as a thin `ReviewChat` shell. Extract
stateful review behavior into `components/agent/hooks/use-review-session.ts`;
extract render-only pieces into sibling components. Keep `/` as the review home
and preserve the current Willow review UX. Do not smuggle in BACKLOG-008
renderSpec work unless the branch owns that ticket.

### Concepts

Keep all Convex-decorated exports in `convex/concepts.ts`, including public
queries/mutations and internal wrappers. Move private lifecycle, review, and
scoring helpers into `convex/lib/*` modules with explicit `ctx` and `userId`
parameters. Preserve FSRS scheduling behavior and bounded Convex reads.

### AI Generation

Keep the internal actions in `convex/aiGeneration.ts`. Move pure preparation,
deduplication, validation, error classification, and conflict scoring into
`convex/lib/generationPipeline.ts`; move Langfuse trace helpers separately from
job failure handling. Existing imports may be temporarily re-exported only to
avoid churn, not to create a new public API.

### IQC

Keep action-card queries/mutations and scheduler-referenced internals in
`convex/iqc.ts`. Move tokenization, similarity scoring, merge payload schemas,
prompt construction, snapshots, and stat deltas into pure helpers. Move
adjudication/fetch helpers only as dependency-injected async functions, not as
new Convex functions.

### Root Topology

Use the topology inventory as a cleanup backlog, not as license for broad root
reshuffling. One slice resolves one concern: for example Lighthouse config,
Prettier config, non-root-critical docs, or generated artifact drift. Update
scripts/docs that name the moved or canonicalized path, then run the gate
required for build/config/workflow surfaces.

## Verification

Run the narrowest relevant tests first, then satisfy the repo brief gate
statement quoted above before declaring an applied refactor complete.

- For React extraction: run the relevant component/hook tests, then
  `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`.
- For `convex/**`: run focused Convex tests plus `pnpm test:contract`, then
  `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`.
- For build/config/workflow/dependency surfaces: add `pnpm build`; add
  `pnpm audit --audit-level=critical` for dependency or lockfile changes.
- For report-only mode: do not edit files; include the exact verification that
  would be required to apply the recommendation.

## Required Output

```markdown
## Refactor Report
Mode: feature-branch | primary-branch
Target: <branch, backlog id, or scope>

### Complexity Removed
<deletions, extracted responsibilities, or state reductions>

### Public Contracts Preserved
<Convex/API/UI contracts checked>

### Applied Change Or Recommendation
<what changed, or the backlog item/report produced>

### Verification
<commands run and results, using the repo gate statement>

### Residual Risk
<remaining risk, owner, and why it was not part of this slice>
```

## Gotchas

- Moving a Convex-decorated export is an API change, even if TypeScript still
  compiles locally.
- Splitting one tangled file into several tangled files is not simplification.
- "Reusable" code with one caller is speculative abstraction. Delete or inline
  until a second live caller proves the shape.
- Tests for dead code are dead tests. Delete them with the surface they cover.
- README and older docs can drift. Use `package.json`, `.github/workflows/**`,
  `lefthook.yml`, and `.spellbook/repo-brief.md` as the gate/source hierarchy.
- Root cleanup is not product cleanup. Do not mix topology moves with review,
  FSRS, generation, or IQC behavior changes.
