---
name: deliver
description: |
  scry inner-loop composer. Takes one file-driven backlog item from
  backlog.d/ to merge-ready code by composing /shape -> /implement ->
  {/code-review + /ci + /refactor + /qa as applicable}. Stops at
  merge-ready. Does not push, merge, deploy, archive backlog tickets, or
  inject backlog-closing trailers; /ship owns closure.
  Use when: building one shaped backlog item, "deliver this", "make it
  merge-ready", driving one scry ticket through implementation, review,
  verification, and evidence.
  Trigger: /deliver.
argument-hint: "[backlog-id|backlog-file|issue-id] [--resume <ulid>] [--abandon <ulid>] [--state-dir <path>]"
---

# /deliver

One scry backlog item becomes merge-ready code. Delivered is not shipped:
`/deliver` stops with a clean branch, a receipt, and a terse operator brief.
`/ship` owns pushing, PR landing, backlog archival, backlog trailers, and
post-merge reflection.

## Non-Negotiable Role

- Work exactly one backlog item from `backlog.d/` or one explicit GitHub issue.
- Compose phase skills; do not inline `/shape`, `/implement`, `/code-review`,
  `/ci`, `/refactor`, or `/qa` logic.
- Stop at merge-ready. Never run `git push`, `gh pr merge`, deployment commands,
  production migrations, ticket archival, or `git mv backlog.d/<id>-*.md
  backlog.d/_done/`.
- Do not add `Closes-backlog:`, `Ships-backlog:`, or `Refs-backlog:` trailers.
  `/ship` injects trailers and archives the backlog file during landing.
- Fail loud. A red phase is a red delivery, not a "best effort" pass.

## Scry Ship Gate

The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

For delivery, "merge-ready" means the local parity command and every relevant
additive check above passed, or the receipt records the exact failure and exits
non-zero. Use `pnpm` only. `bun` CI files and migration docs are parity evidence,
not permission to switch the workflow.

Lefthook is defense in depth, not the gate. Pre-commit runs TruffleHog, topology
hygiene, `pnpm exec tsc --noEmit`, and lint-staged. Pre-push runs
`scripts/test-changed-batches.sh` and `pnpm test:contract`, but `/deliver` still
does not push.

## Composition

```text
/deliver [backlog-id|backlog-file|issue-id] [--resume <ulid>] [--state-dir <path>]
    |
    v
  pick one backlog.d item if no arg
    |
    v
  /shape        -> executable .ctx.md packet with goal, constraints, oracle, sequence
    |
    v
  /implement    -> TDD build on feature branch
    |
    v
  CLEAN LOOP, max 3 iterations
    - /code-review: concrete blocking findings, not style commentary
    - /ci: scry gate parity and additive checks
    - /refactor: diff-aware simplification only where risk or duplication is real
    - /qa: only for user-facing review/library/generation surfaces
    |
    v
  receipt.json + operator brief; stop
```

## Ticket Intake

scry backlog is file-driven:

- Active backlog items are `backlog.d/<id>-<slug>.md`.
- Context packets are `backlog.d/<id>-<slug>.ctx.md`.
- `_done/` is archival state and belongs to `/ship`, not `/deliver`.
- A shaped packet is executable only when it names the spec, constraints, file
  inventory, implementation sequence, oracle, and verification.
- Existing `## What Was Built` means the item has already been delivered or
  shipped; stop and route to `/groom tidy` instead of re-delivering stale work.

If no target is supplied, pick the highest-priority active item that is not
`Status: done` and has enough oracle detail to execute. If a `.ctx.md` packet is
missing or stale, run `/shape` first. Do not improvise around a weak oracle.

## Scry Delivery Rules

- **Backend-first Convex.** For Convex work, implement schema, query, mutation,
  action, helper, and contract changes under `convex/` first. Validate generated
  API/type readiness before wiring React. Run `pnpm test:contract` when
  `convex/**` changes.
- **Pure FSRS.** Do not add daily limits, artificial review caps, comfort-mode
  buttons, "easy" shortcuts, or algorithmic optimizations that change FSRS
  scheduling. The interval decides.
- **Bounded Convex reads.** Runtime queries use indexes, `.take()`, pagination,
  and truncation signals. No unbounded `.collect()` in runtime paths.
- **Mutation reversibility.** Destructive UX needs a reverse path:
  archive/unarchive, softDelete/restore. Hard delete requires explicit
  confirmation UX.
- **Deploy and migration commands require explicit approval.** Never run
  `pnpm build:local`, `pnpm build:prod`, `pnpm convex:deploy`,
  `./scripts/deploy-production.sh`, production migration scripts, or commands
  that mutate non-local Convex/Vercel state unless the operator explicitly
  approves that exact command.
- **Source of truth.** Trust `package.json`, `.github/workflows/ci.yml`, and
  `lefthook.yml` over README or older docs. `CLAUDE.md` is secondary; docs are
  advisory unless verified against live config.

## Branch and Backlog Boundaries

`/implement` owns branch creation. The scry branch contract is:

```text
<type>/<id>-<slug>
```

where `<type>` is one of `feat`, `fix`, `chore`, `refactor`, `docs`, `test`,
or `perf`, and `<id>` is the bare numeric backlog ID. This branch name is the
backlog claim that `/ship` later reads.

`/deliver` may reference the backlog ID in the receipt and brief, but it must
not close it. Trailer-based closure is a `/ship` concern:
`Closes-backlog:`, `Ships-backlog:`, `Refs-backlog:`, and movement into
`backlog.d/_done/` happen only there.

## Phase Routing

| Phase | Skill | scry-specific ownership | Skip when |
|---|---|---|---|
| Shape | `/shape` | Produce or refresh the `.ctx.md` packet and executable oracle | Packet is current, concrete, and already executable |
| Implement | `/implement` | TDD red -> green -> refactor on the feature branch | Never |
| Review | `/code-review` | Blocking risks in Convex contracts, FSRS semantics, bandwidth, auth, review UI, evals, and tests | Never |
| CI | `/ci` | Run the scry gate parity plus additive checks | Never |
| Refactor | `/refactor` | Remove accidental complexity introduced by this diff | Trivial single-concern diffs with no review/CI pressure |
| QA | `/qa` | Browser check for `/`, `/concepts`, generation modal, action inbox, settings, or other touched UI | Pure backend/lib/eval changes |

Each phase writes its own evidence. `/deliver` records pointers in the receipt;
it does not create version-controlled evidence bundles.

## Verification Matrix

Always run:

```bash
pnpm lint && pnpm tsc --noEmit && pnpm test:ci
```

Add checks by touched surface:

| Surface touched | Additional check |
|---|---|
| `convex/**` | `pnpm test:contract` |
| `package.json`, lockfile | `pnpm audit --audit-level=critical` |
| `.github/workflows/**`, `next.config.ts`, `vercel.json`, build config, dependencies | `pnpm build` |
| `evals/**`, prompt templates, generation behavior | relevant promptfoo command, usually `pnpm eval` or the shaped eval command |
| User-facing review/library/generation UI | `/qa` browser evidence plus targeted tests |

Do not substitute `pnpm qa` for the gate unless the receipt also shows the
required gate commands above. `package.json` defines both, but GitHub protects
the `merge-gate`.

## Clean Loop

The clean loop runs at most three times. Dirty means any of:

- `/code-review` returns a blocking finding.
- `/ci` fails any required or additive gate.
- `/qa` finds a P0 or P1 in a user-facing path.
- The implementation violates backend-first Convex, pure FSRS, bandwidth,
  mutation reversibility, or approval-gated command boundaries.
- Review completed but no verdict/evidence points at the current HEAD.

Iteration 1 or 2 can continue after fixes. Iteration 3 exits `20` with
`status: clean_loop_exhausted` and a concrete remaining-work list. Do not invent
a fourth loop.

If implementation proves the shape wrong, stop and return to `/shape`; do not
patch around a bad oracle.

## Contract

`/deliver` communicates through `<state-dir>/receipt.json`, exit code, and a
short operator brief. Callers must not parse stdout.

| Exit | Meaning | Receipt `status` |
|---|---|---|
| 0 | Merge-ready by the scry gate | `merge_ready` |
| 10 | Phase handler hard-failed | `phase_failed` |
| 20 | Clean loop exhausted after 3 iterations | `clean_loop_exhausted` |
| 30 | Operator/SIGINT abort | `aborted` |
| 40 | Invalid args or missing phase skill | `phase_failed` |
| 41 | Target was already delivered or shipped | `phase_failed` |

The receipt must include:

- backlog target: ID, markdown path, ctx path if present
- branch and HEAD SHA
- phase receipts and evidence pointers
- commands run, with exit codes
- checks intentionally skipped and the reason
- residual risks and remaining work
- confirmation that no push, merge, deploy, migration, ticket archive, or
  backlog trailer injection happened

## Resume and Durability

State is filesystem-backed and resumable:

- Default root: `<worktree-root>/.spellbook/deliver/<ulid>/`.
- Override: `--state-dir <path>`, usually supplied by `/flywheel`.
- After each phase, rewrite `state.json` atomically.
- `--resume <ulid>` loads state, skips completed phases, and re-enters at
  `current_phase`.
- `--abandon <ulid>` removes delivery state and leaves the branch untouched.

Do not depend on conversation memory. If the session dies, `state.json`,
phase receipts, and git state must be enough to resume.

## Operator Brief

End every run with a tight brief, not a changelog. It should state:

- backlog item worked and outcome
- user/product value once shipped
- developer/operator value now
- key design choice and rejected alternative
- verification run and residual risk
- whether `/ship` can take over or what blocks it

When `/deliver` runs inside `/flywheel`, keep the same facts but let
`/flywheel` own the cycle-level summary.

## Gotchas

- `BACKLOG-001` and `BACKLOG-002` already contain `## What Was Built`; do not
  redeliver them as fresh work.
- `BACKLOG-003` and `BACKLOG-004` are eval-first work. Do not change generation
  behavior before establishing the eval floor called out by the oracle.
- `BACKLOG-006` touches question types. Preserve schema, generation contracts,
  review UI, grading, and eval coverage together.
- `BACKLOG-007` is extraction-only. Do not change public Convex API signatures
  or rewrite behavior while splitting monoliths.
- `BACKLOG-008` calls for a thin Zod-validated renderSpec registry, not a
  general LLM-generated UI framework.

## Non-Goals

- Shipping or landing work
- Pushing branches
- Merging PRs
- Deploying Convex or Vercel
- Running production migrations
- Archiving backlog files
- Injecting backlog trailers
- Multi-ticket batching

## Related

- Producer: `/shape` writes executable `.ctx.md` packets.
- Builder: `/implement` owns branch and code changes.
- Cleaners: `/code-review`, `/ci`, `/refactor`, `/qa`.
- Consumer: `/flywheel` may call `/deliver` and read `receipt.json`.
- Closure: `/ship` pushes, lands, archives backlog, injects trailers, and runs
  post-merge reflection.
