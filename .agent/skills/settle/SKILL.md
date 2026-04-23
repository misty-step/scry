---
name: settle
description: |
  scry polish loop. Take a feature branch with code and tests and iterate
  CI -> code-review -> refactor -> QA until the branch is lean, green,
  and merge-ready under scry's GitHub Actions merge-gate and local parity
  checks. Stops at merge-ready. Does not merge. Does not archive backlog
  tickets. Hands off to /ship, never /land.
  Use when: "polish this", "get this merge-ready", "unblock", "clean up",
  "address reviews", "fix CI", "make this shippable".
  Trigger: /settle, /pr-fix, /pr-polish.
argument-hint: "[PR-number|branch-name]"
---

# /settle

Take a scry feature branch from "works" to merge-ready. Iterate
`/ci` -> `/code-review` -> `/refactor` -> `/qa` until the same pass is
green, reviewed, simplified, and manually/automatically QA-clean. Then
stop and hand the operator to `/ship`.

`/settle` is not a landing command. It does not merge, does not move
`backlog.d/` files to `_done/`, does not archive tickets, does not deploy,
and does not invoke `/reflect`. `/ship` owns merge, backlog closure, and
reflection.

## Gate Contract

Every settle run is anchored to the repo brief's gate statement:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Implications:

- Use `pnpm`, never npm, yarn, or bun, for settle verification.
- Hosted CI truth is `.github/workflows/ci.yml` job
  `Quality Checks / merge-gate`.
- Local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`.
- Add `pnpm test:contract` whenever the branch changes `convex/**`.
- Add `pnpm build` whenever the branch changes dependencies, build config,
  workflow surfaces, `package.json`, lockfile, `next.config.ts`,
  `vercel.json`, or `.github/workflows/**`.
- Add `pnpm audit --audit-level=critical` whenever the branch changes
  dependencies or lockfile.

## Scry Invariants To Protect

- Backend before frontend for Convex work: schema, query, mutation/action,
  generated API/type readiness, then UI wiring.
- Pure FSRS is non-negotiable: no daily limits, no comfort-mode shortcuts,
  no "FSRS but better" scheduling changes.
- Convex runtime reads must be indexed and bounded with `.take()` or
  pagination; no unbounded `.collect()` in runtime paths.
- Destructive actions need a reverse: `archive`/`unarchive`,
  `softDelete`/`restore`; hard delete requires explicit confirmation UX.
- `/` is the review home for Willow and due concepts. `/agent` redirects
  to `/`; do not revive dead primary navigation while polishing.
- Valid tracker references are only `backlog.d/` files and GitHub issues.
  Any other tracker vocabulary is stale-source signal to delete from the
  branch, not a reference to preserve.

## Approval Boundaries

Never run these during `/settle` without explicit operator approval:

- `pnpm build:local`
- `pnpm build:prod`
- `pnpm convex:deploy`
- `./scripts/deploy-production.sh`
- Production migration scripts
- Any command that changes non-local Convex, Vercel, Stripe, Clerk, or
  other production state

`pnpm build` is allowed when its additive-check condition applies. It is a
local Next build, not a deploy. `pnpm build:local` and `pnpm build:prod`
deploy Convex as part of the script and remain approval-gated.

## Prerequisites

Assert at start; refuse with a clear reason on any miss.

- On a feature branch, not `master`, `main`, or the protected default.
- Branch has commits beyond the base branch.
- Working tree is clean, or dirty only with operator-acknowledged work that
  belongs to this branch.
- No rebase, merge, or cherry-pick in progress.
- No unresolved conflict markers.
- Branch tracker refs, if present, point only at `backlog.d/<id>-*.md` or
  GitHub issues.

Detect base with the remote default branch when available, otherwise use
`master` if it exists, then `main`:

```sh
git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null | sed 's#^origin/##'
```

## Mode Detection

- **PR mode**: `$ARGUMENTS` is a PR number, or `gh pr view` succeeds for
  the current branch. Use full PR review/comment bodies; do not rely on
  truncated previews. Check remote checks with `gh pr checks`.
- **Local mode**: no PR exists for the branch. Rely on local `/ci`,
  `/code-review`, `/refactor`, and `/qa`.

Mode changes evidence sources, not the loop or exit criteria.

## The Polish Loop

Run the loop steps in order. If any step produces a code, test, doc, or
config change, commit or leave the intended branch changes explicit, then
return to step 2 (`/ci`). The loop exits only when CI, code review,
refactor, QA, and hindsight pass clean in the same iteration.

### 1. Assert preconditions

Record:

- Current branch and base branch.
- PR number, if any.
- Tracker refs found in branch name, commits, PR body, or changed backlog
  files.
- Changed surfaces: `convex/**`, dependencies/lockfile, build/config,
  workflows, review UI, Stripe/Clerk/API routes, prompts/evals.
- Approval-gated commands that would be relevant but are not authorized.

If branch naming references a backlog ticket, keep the bare numeric ID
compatible with `/ship` trailers (`Closes-backlog:`, `Ships-backlog:`,
`Refs-backlog:`). Do not invent alternate tracker syntax.

### 2. CI

Invoke `/ci` and require it to enforce the gate contract above. If working
directly, run the local parity command:

```sh
pnpm lint && pnpm tsc --noEmit && pnpm test:ci
```

Then run additive checks when their surfaces changed:

```sh
pnpm test:contract
pnpm build
pnpm audit --audit-level=critical
```

Classify red checks:

- **Mechanical**: lint formatting, import drift, stale generated types,
  obvious typo, or lockfile inconsistency. Fix directly if bounded.
- **Convex contract/schema**: fix backend-first. Do not patch UI around a
  missing query, mutation, action, index, or generated API mismatch.
- **FSRS behavior**: protect the pure algorithm; failing tests are evidence
  to repair implementation, not permission to relax intervals or add caps.
- **Logic/architecture**: dispatch a bounded builder with the exact failing
  command, excerpt, file:line, and allowed scope.

In PR mode, also wait for `gh pr checks` to reach terminal success. A
pending check is not green.

### 3. Code Review

Invoke `/code-review` with a must-ship lens and scry's invariants:

- Backend-first Convex flow and generated API readiness.
- Bounded Convex queries and no runtime unbounded `.collect()`.
- Pure FSRS, no comfort shortcuts, no daily due caps.
- Destructive mutation reversibility.
- Tests that exercise behavior and do not mock internal repo modules.
- Hotspot discipline around `components/agent/review-chat.tsx`,
  `convex/concepts.ts`, `convex/aiGeneration.ts`, and `convex/iqc.ts`:
  extract or simplify when touched, but do not rewrite unrelated surfaces.

Synthesize the verdict:

- `ship` or `conditional` with no blockers: proceed to refactor.
- `dont-ship` or blockers: fix each blocker, then return to CI.

In PR mode, read every PR review, inline comment, and check annotation in
full. Disposition remains lead-model judgment:

- **Fix** if the concern is valid and in scope.
- **Defer** only if it is truly outside this branch; file or cite a
  concrete `backlog.d/` ticket or GitHub issue.
- **Reject** only after steelmanning the concern and citing the specific
  invariant, test, or code path that makes the suggestion wrong.

Do not write "pre-existing, not introduced" as an excuse for a broken path
in touched code. If it is actually broken and you touched the area, fix it
or file the concrete tracker item before proceeding.

### 4. Refactor

Invoke `/refactor` against the detected base. Treat it as a scry-specific
simplification pass, not generalized cleanup.

Mandatory refactor pressure:

- Net branch diff is large enough that review risk is non-trivial.
- The branch touches known hot files from the repo brief.
- The branch adds pass-through layers, temporal decomposition, duplicate
  validation, or hidden coupling between Convex and UI state.
- Tests depend on internal mocks instead of behavior at boundaries.

Refactor priorities:

- Delete dead or duplicate code before adding abstractions.
- Keep Convex modules deep and interfaces simple.
- Extract from hot files without changing public API when the ticket is
  architectural debt.
- Preserve generated API boundaries; do not hand-edit `convex/_generated/**`.

If `/refactor` applies changes, return to CI.

### 5. QA

Invoke `/qa` after refactor is clean. QA is where scry proves the branch is
usable, not merely typed.

Minimum QA asks:

- For review-surface changes, exercise `/` as Willow's review home and
  verify due concepts, feedback recording, and relevant artifact rendering.
- For Convex changes, verify the mutation/query/action path that the UI or
  tests rely on; include `pnpm test:contract`.
- For generation, IQC, prompt, or eval changes, run the narrow prompt/eval
  or test path the branch owns before broader gates.
- For Stripe, Clerk, webhook, or API route changes, test against local or
  mocked external boundaries only; do not touch production state.
- For dogfood flows when applicable, prefer `pnpm qa:dogfood:local` against
  a local app. `pnpm qa:dogfood` may target configured non-local URLs; ask
  before using it if the target is unclear.

If QA finds a bug, fix it and return to CI.

### 6. Hindsight Self-Review

Read the full branch diff one last time after CI, code-review, refactor,
and QA are clean:

```sh
git diff "$(git merge-base HEAD "$BASE_BRANCH")"...HEAD
```

Ask: **Would I approve this for scry's memory-science product and Convex
backend contract?** Look for:

- Any shortcut that weakens FSRS or hides due learning debt.
- Any frontend affordance whose backend mutation/query contract is missing.
- Any unbounded Convex runtime read.
- Any destructive mutation without a reverse path.
- Any test that mocks internal collaborators.
- Any stale TODO, debug artifact, commented-out code, or tracker reference
  outside `backlog.d/` and GitHub issues.
- Any required additive check skipped because the changed surface was
  under-classified.

If anything non-trivial surfaces, fix it and return to CI.

### 7. Verdict Ref Check

If `scripts/lib/verdicts.sh` exists, confirm the current verdict for this
branch is fresh and not `dont-ship`:

```sh
source scripts/lib/verdicts.sh
verdict_validate "$(git rev-parse --abbrev-ref HEAD)"
```

A stale verdict means changes landed after review; return to code review.
A `dont-ship` verdict means exit criteria are not met.

## Exit Criteria

Report **merge-ready** only when all gates pass in the same iteration:

- `/ci` is green, including local parity, required additive checks, and PR
  checks if in PR mode.
- `/code-review` verdict is `ship` or `conditional`, with no open blockers.
- `/refactor` ran and produced no further changes this iteration.
- `/qa` ran for the branch's changed surfaces and produced no follow-ups.
- Hindsight self-review is clean.
- Verdict ref is fresh and not `dont-ship`, if this repo has verdict refs.
- Tracker refs are limited to `backlog.d/` and GitHub issues.

Safety cap: max 6 iterations. If the loop has not converged by the sixth
pass, stop and emit a structured diagnosis with the failing gate, evidence,
likely root cause, and the human decision needed.

On clean exit, emit:

```text
/settle complete - merge-ready.

Iterations: 3
CI:          green (Quality Checks / merge-gate + local parity)
Additive:    pnpm test:contract for convex/**; pnpm build not required
Code-review: ship (0 blockers)
Refactor:    no further simplification found
QA:          clean on / review flow
Self-review: clean
Trackers:    backlog.d/123-example.md, GitHub #456

Next: run /ship to merge, archive backlog tickets, and reflect.
```

## What /settle Does Not Do

- Does not merge. `/ship` performs the merge.
- Does not archive backlog tickets or move `backlog.d/*` to
  `backlog.d/_done/`. `/ship` owns closure.
- Does not run `/reflect`. `/ship` invokes reflection with bounded scope.
- Does not deploy Convex or Vercel.
- Does not run approval-gated production or migration commands.
- Does not push unless the operator separately invokes `/yeet` or directs a
  push.
- Does not hand off to `/land`; that command is retired for this repo loop.

## PR Mode vs Local Mode

| Concern | PR mode | Local mode |
|---|---|---|
| Detection | `$ARGUMENTS` is a PR number, or `gh pr view` succeeds | no PR for branch |
| CI signal | `/ci`, local parity as needed, and `gh pr checks` | `/ci` and local parity |
| Review input | Full PR reviews/comments/check annotations plus `/code-review` | `/code-review` |
| Refactor | `/refactor` against base | `/refactor` against base |
| QA | `/qa` plus PR-specific evidence when available | `/qa` with local evidence |
| Closure | Hand to `/ship` | Hand to `/ship` |

Both modes run the same polish loop. PR mode adds remote checks and full
review-comment disposition.

## Refuse Conditions

Stop instead of looping when:

- On `master`, `main`, or the protected default branch.
- Branch has no commits beyond base.
- Rebase, merge, or cherry-pick is in progress.
- Working tree has unresolved conflict markers.
- Tracker refs point outside `backlog.d/` or GitHub issues.
- Required verification would need an approval-gated deploy/migration
  command and the operator has not approved it.
- Safety cap hit.
- The operator asks `/settle` to merge, archive, deploy, reflect, or hand
  off to `/land`.

## Interactions

- Invoked by `/flywheel` as the polish stage after `/yeet`.
- Invokes `/ci`, `/code-review`, `/refactor`, and `/qa`.
- Dispatches bounded builders for specific failures; keeps reviewer
  disposition and merge-readiness judgment on the lead model.
- Hands off to `/ship` for merge, backlog archive, and reflection.
- Works with `/yeet` for branch push discipline, but remains independent of
  pushing.

## Gotchas

- **`pnpm typecheck` vs local parity.** CI runs `pnpm typecheck`; the repo
  brief's local parity spells this as `pnpm tsc --noEmit`. Treat both as
  the same TypeScript gate in their respective contexts.
- **`pnpm build` vs deploy builds.** `pnpm build` is the additive local
  build check. `pnpm build:local` and `pnpm build:prod` deploy Convex and
  require approval.
- **Convex UI patching.** If generated API types or a query/mutation are
  wrong, fix Convex first. UI guards around missing backend truth are
  symptom patches.
- **Pending checks.** Pending GitHub checks are not green. Wait for terminal
  success.
- **Post-refactor drift.** Any refactor or QA fix invalidates prior CI and
  review evidence. Return to CI.
- **Tracker drift.** Only `backlog.d/` and GitHub issues are valid. Retired
  tracker references must not survive in settle output or branch metadata.
- **Merge temptation.** A clean `/settle` report means "run `/ship` next",
  not "merge now".
