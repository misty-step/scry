---
name: flywheel
description: |
  scry outer-loop shipping orchestrator. Picks the next backlog item,
  composes /shape, /implement, /yeet, /settle, /ship, and /monitor,
  then loops. Closure (backlog archive, trailers, /reflect, and harness
  routing) lives in /ship; flywheel does not invoke /reflect directly.
  Use when: "flywheel", "run the outer loop", "next N items",
  "overnight queue", "cycle".
  Trigger: /flywheel.
argument-hint: "[--max-cycles N]"
---

# /flywheel

You are scry's outer loop. Preserve the mandatory composition exactly:

pick -> `/shape` -> `/implement` -> `/yeet` -> `/settle` -> `/ship` ->
`/monitor` -> loop.

Do not substitute `/deliver` for the explicit phases here. The point of
this skill is orchestration discipline around scry's backlog, not a
shortcut around the leaf skills.

## Source Of Truth

Read `.spellbook/repo-brief.md` and `backlog.d/*.md` before the first
pick in a run. If they disagree, prefer the repo brief for repo-wide
invariants and the specific backlog file for ticket-local acceptance
criteria. If either conflicts with live `package.json`, `.github/workflows/**`,
or `lefthook.yml`, stop and let `/shape` reconcile the conflict before
implementation.

Every gate-adjacent phase cites this repo-brief statement verbatim:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Never run forbidden deploy/build commands as part of the loop without
explicit operator approval: `pnpm build:local`, `pnpm build:prod`,
`pnpm convex:deploy`, `./scripts/deploy-production.sh`, production
migration scripts, or anything that changes non-local Convex/Vercel
state.

## Pick Order

Pick from `backlog.d/`, not vibes. Continue an `in-progress` item before
starting a new ready item unless ownership or git state says another
worker owns it. Skip `done` items unless the backlog file is stale; stale
entries are fixed through the normal `/ship` closure path, not by
flywheel archiving them.

Current scry priority shape:

- High-priority eval and content-type work comes first: phrasing
  generation evals, content-type eval suite, hard quality gates, then
  cloze and short-answer completeness. Respect declared dependencies:
  `003` -> `004` -> `005` -> `006`.
- Agent-ready architecture follows once gates are hard. It is extraction,
  not rewrite: split `convex/concepts.ts`, `convex/aiGeneration.ts`,
  `convex/iqc.ts`, and `components/agent/review-chat.tsx` into focused
  modules without public API changes.
- Generative UI foundation follows content types and review-chat
  decomposition. Prefer scry's thin Zod-validated `renderSpec` registry
  over a general-purpose LLM-generated UI framework.
- Product-surface cleanup remains important, but do not trample another
  worker's `in-progress` ownership. If the item is open and unowned,
  finish it before starting a new high-priority chain.

When the next item is ambiguous, ask `/shape` to ratify the pick with the
backlog dependency graph and current git state. Flywheel does not invent
new scoring systems, locks, queues, or ticket state machines.

## Scry Guardrails

Every cycle protects the product doctrine:

- Pure FSRS is non-negotiable. Do not pick or accept work that adds daily
  caps, artificial review limits, comfort-mode shortcuts, or "ease"
  affordances that change the scheduling curve.
- Convex backend precedes UI. Schema, query, mutation, generated API
  readiness, and contract tests come before React wiring.
- Convex bandwidth is a ship concern. Runtime paths need indexes and
  bounded `.take()` or pagination; unbounded `.collect()` is not accepted
  without a bounded diagnostic/offline justification.
- Destructive mutations need reverses: `archive`/`unarchive`,
  `softDelete`/`restore`; hard delete requires explicit confirmation UX.
- Keep `/` as the review home and Willow's review chat as the product
  center. `/tasks` and `/action-inbox` may exist, but they are not primary
  navigation unless a shaped ticket explicitly changes that.

## Phase Contract

For each cycle:

1. **Pick.** Select one backlog item and verify it is not already shipped
   in git or owned by another worker. Record only the selected item in the
   handoff to `/shape`.
2. **`/shape`.** Ensure the ticket has a concrete oracle, dependency
   order, test plan, FSRS/Convex implications, and the exact ship gate
   statement above.
3. **`/implement`.** Build only the shaped ticket. For Convex work,
   backend-first sequencing is mandatory. For architecture work, extract
   without changing public APIs.
4. **`/yeet`.** Run adversarial review against behavior, tests, FSRS
   purity, Convex bandwidth, and ticket oracle. Fix findings through the
   normal implementation loop.
5. **`/settle`.** Stabilize the diff, receipts, and git state. No new
   scope is added here.
6. **`/ship`.** Owns closure: final gate evidence, squash/merge flow,
   backlog archive, trailers, `/reflect`, and harness routing. Flywheel
   never invokes `/reflect` directly and never archives tickets itself.
7. **`/monitor`.** Check post-ship signals appropriate to the item. For
   deploy-no-op work, monitor still records that there was nothing to
   observe beyond CI/repo state.
8. **Loop.** Re-read the backlog summary before the next pick; do not
   rely on stale memory from the prior cycle.

## Collision Rules

- One worktree runs one flywheel. Parallel flywheels need separate
  worktrees and disjoint backlog ownership.
- If a worker edits another skill, harness bridge, setting, marker,
  `AGENTS.md`, or app code outside the selected ticket, stop and route
  through the owning workflow. Flywheel does not clean up foreign edits.
- If `/ship` produces reflect/harness changes, they route according to
  `/ship` policy and never land on the product branch by flywheel action.

## Non-Goals

- No direct implementation inside flywheel.
- No direct `/reflect` invocation.
- No backlog archive, trailer editing, squash-merge, or harness routing
  inside flywheel; `/ship` owns closure.
- No custom cycle state machine, event enum, lock service, pick scorer,
  or semantic orchestration layer.
