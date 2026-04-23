---
name: implement
description: |
  Atomic TDD build skill. Takes a context packet (shaped ticket) and
  produces code + tests on a feature branch. Red → Green → Refactor.
  Does not shape, review, QA, or ship — single concern: spec to green tests.
  Use when: "implement this spec", "build this", "TDD this", "code this up",
  "write the code for this ticket", after /shape has produced a context packet.
  Trigger: /implement, /build (alias).
argument-hint: "[context-packet-path|ticket-id]"
---

# /implement

Spec in, red-green-refactor out. For scry, that means a shaped
`backlog.d/` packet becomes code + behavior tests on a backlog-claim
branch, with Convex truth established before UI sparkle and FSRS left pure.

## Invariants

- **Branch claim is mandatory.** Create the branch with
  `git checkout -b <type>/<id>-<slug>` where `<type>` is one of
  `feat|fix|chore|refactor|docs|test|perf` and `<id>` is the bare numeric
  backlog ID. The branch name is the backlog claim; `/ship` reads it to
  preserve backlog trailers. Do not use free-form names or `cx/` prefixes.
- **pnpm only.** `packageManager` is `pnpm@10.12.1`; Bun artifacts are
  migration-spike evidence, not permission to change the default.
- Trust the context packet. Do not reshape, re-prioritize, or widen scope.
- If the packet is incomplete, **fail loudly**. Do not invent the spec from a
  title or from model memory.
- TDD is the default: failing behavior test first, minimum production code,
  local refactor, then verification. Skip only for config, generated code, UI
  layout, or pure exploration, and say so in the commit message.
- Mock at the boundary only. External SDKs/services, network, clock, random,
  and filesystem seams may be mocked. Internal scry modules, Convex helpers,
  FSRS logic, validators, and owned utilities must be exercised directly or
  through realistic fakes.
- Backend-first Convex flow is mandatory: schema/query/mutation/action first,
  generated API/type readiness second, UI wiring last.
- Pure FSRS is non-negotiable. Do not add daily limits, comfort-mode
  shortcuts, "easy" affordances that alter the curve, or algorithmic
  optimizations to the `ts-fsrs`/scry FSRS behavior.
- Destructive mutations need reverse pairs: `archive`/`unarchive`,
  `softDelete`/`restore`. `hardDelete` requires explicit confirmation UX and
  must be called out in the packet/oracle.
- Never run deploy-coupled commands without explicit operator approval:
  `pnpm build:local`, `pnpm build:prod`, `pnpm convex:deploy`,
  `./scripts/deploy-production.sh`, production migration scripts, or anything
  that mutates non-local Convex/Vercel state.

## Contract

**Input.** A context packet: goal, non-goals, constraints, repo anchors,
oracle (executable preferred), implementation sequence. Resolution order:

1. Explicit path argument (`/implement backlog.d/033-foo.md`)
2. Backlog item ID (`/implement 033`) → resolves via `backlog.d/<id>-*.md`
3. Last `/shape` output in the current session
4. **No packet found → stop.** Do not guess the spec from a title.

Required packet fields (hard gate — missing any = stop):
- `goal` (one sentence, testable)
- `oracle` (how we know it's done, ideally executable commands)
- `implementation sequence` (ordered steps, or explicit "single chunk")

See `references/context-packet.md` for the full shape.

**Output.**
- Code + tests on a backlog-claim branch:
  `git checkout -b <type>/<id>-<slug>`
- All packet oracle commands green, plus the relevant scry verification
  commands below
- Working tree clean (no debug prints, no scratch files)
- Commits in repo convention — one logical unit per commit
- Final message: branch ref, commits, oracle checklist, verification commands,
  residual risks

**Stops at:** green tests + clean tree. Does not run `/code-review`,
`/qa`, `/ci`, or open a PR.

## Gate Statement

Every implementation should know the ship gate even when `/ci` or `/settle`
runs it later. Cite this exactly when handing off gate status:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Lefthook adds defense in depth: pre-commit runs TruffleHog,
`scripts/tooling/topology-hygiene-check.sh`, `pnpm exec tsc --noEmit`, and
lint-staged; pre-push runs `scripts/test-changed-batches.sh` and
`pnpm test:contract`. Hooks do not replace the gate.

## Workflow

### 1. Load and validate packet

Resolve the packet (order above). Parse required fields. If any are missing
or vague ("add feature X" with no oracle), stop with:

> Packet incomplete: missing <field>. Run /shape first.

Do not try to fill in the gaps. Shape is a different skill's judgment.

Extract the bare numeric backlog ID from the packet path or argument and verify
the packet's requested work maps to one of the allowed branch types:
`feat|fix|chore|refactor|docs|test|perf`. If it does not, stop and ask for the
type; do not silently pick one.

### 2. Create the backlog-claim branch

Run `git status --short` first and preserve unrelated user/worker changes.
Then create the claim branch from the current base:

```bash
git checkout -b <type>/<id>-<slug>
```

Examples:

```bash
git checkout -b feat/033-review-artifact-render-spec
git checkout -b fix/146-due-query-bandwidth-regression
```

Builders never commit to `main`/`master` or a shared worker branch. If work was
started on the wrong branch, stop, create the correct branch, and move only the
owned implementation changes.

### 3. Red: write the behavior test first

Pick the narrowest test that proves the packet's first behavior. One behavior
per test. Prefer repo-native surfaces:

- Pure logic and React behavior: Vitest 4 with `happy-dom` from
  `vitest.config.ts`; targeted command is `pnpm test -- <path-or-pattern>` or
  `pnpm exec vitest --run <path-or-pattern>`.
- Convex contract/API behavior: extend the relevant Convex-facing test and run
  `pnpm test:contract` when `convex/**` changes.
- Review UI or route behavior that needs a browser: Playwright lives in
  `tests/e2e`; use `pnpm test:e2e:smoke` for smoke coverage or
  `pnpm test:e2e:full` when the packet demands it. Local Playwright starts
  `pnpm dev`; CI points at `https://scry.vercel.app`.
- Prompt or generation behavior: use the packet's eval/oracle; if it touches
  promptfoo surfaces, include `pnpm eval` only when explicitly required because
  provider access may be environment-bound.

Boundary mocks are allowed for Clerk, Stripe, AI providers, Sentry, PostHog,
network, time, random, and browser APIs. Red flags in scry tests:
`vi.mock("../...")`, `vi.mock("@/lib/...")`, stubbing `convex/**`, stubbing
FSRS modules, or replacing an owned validator with a mock. Use the real module
or build a fake at the I/O seam.

Commit only after the test has failed for the right reason, or keep the red
state uncommitted until green if that is the repo's current branch hygiene.

### 4. Green: implement the smallest correct change

Follow the packet sequence exactly. For Convex work, use this order:

1. Update `convex/schema.ts` and indexes first. Schema removals use the
   optional-field migration pattern: make optional, dry-run/backfill, verify
   diagnostics, then remove.
2. Implement Convex queries, mutations, actions, and library logic. Use
   indexes and bounded reads from `docs/guides/convex-bandwidth.md`: no
   unbounded `.collect()` in runtime paths, use `.take()` or `.paginate()`,
   batch writes at 200 or fewer docs, and return truncation signals when data
   is capped.
3. Validate generated API/type readiness with `pnpm exec tsc --noEmit` or the
   packet's codegen/typecheck oracle. Do not hand-edit `convex/_generated/**`.
4. Wire Next.js/React UI only after the backend contract is real.

For FSRS/review work, keep scheduling concept-level. `concepts.fsrs` in
`convex/schema.ts` is the source of truth for stability, difficulty,
`nextReview`, reps, lapses, state, and scheduled interval data. Tests should
assert retrieval/scheduling behavior, not implementation shape, and must not
hide overdue learning debt behind daily caps.

For destructive behavior, implement the reverse in the same packet unless the
packet explicitly scopes it out and files the follow-up. `archive` without
`unarchive`, `softDelete` without `restore`, or a `hardDelete` path without
confirmation UX is incomplete.

### 5. Refactor locally

Refactor only the code touched for the behavior you just made green. In scry,
that usually means deleting duplicated path logic, extracting a pure helper
from a hot file, or tightening a Convex boundary. It does not mean redesigning
`review-chat.tsx`, `convex/concepts.ts`, `convex/aiGeneration.ts`, or
`convex/iqc.ts` unless the packet is specifically the agent-ready extraction
work.

### 6. Dispatch builders only when ownership partitions cleanly

Spawn a **builder** sub-agent (general-purpose) with:
- The full context packet
- The executable oracle
- The branch claim (`<type>/<id>-<slug>`)
- The scry TDD and no-internal-mocks mandate
- The relevant Convex/FSRS/bandwidth constraints
- File ownership (if the packet decomposes into disjoint chunks, spawn
  multiple builders in parallel — one per chunk, each with subset of oracle)

**Builder prompt must include:**
> You MUST write a failing test before production code. RED → GREEN →
> REFACTOR → COMMIT. Use pnpm only. Do not mock internal scry modules. For
> Convex work, implement schema/query/mutation/action before UI and keep reads
> indexed and bounded. Exceptions: config files, generated code, UI layout.
> Document any skipped-TDD step inline in the commit message.

See `references/tdd-loop.md` for the full cycle and skip rules.

Only parallelize when file ownership and oracle criteria partition cleanly.
Shared files, generated API surfaces, schema changes, and review scheduling
paths stay serial.

### 7. Verify exit conditions

Before exiting, confirm:
- [ ] Every packet oracle command exits 0 (run them, don't trust a builder)
- [ ] Targeted tests for the changed behavior exit 0
- [ ] `pnpm lint` exits 0 for code changes unless a narrower packet oracle
      explicitly defers lint to `/ci`
- [ ] `pnpm exec tsc --noEmit` or `pnpm typecheck` exits 0 after TypeScript or
      Convex API changes
- [ ] `pnpm test:ci` exits 0 before declaring local parity; if skipped for a
      narrow handoff, state that it was not run
- [ ] `pnpm test:contract` exits 0 when `convex/**` changed
- [ ] `pnpm build` exits 0 when `package.json`, lockfile, `next.config.ts`,
      `vercel.json`, or `.github/workflows/**` changed
- [ ] `pnpm audit --audit-level=critical` exits 0 when dependencies or lockfile
      changed
- [ ] `git status` clean (no untracked debug files)
- [ ] No `TODO`/`FIXME`/`console.log` added that isn't in the spec
- [ ] Commits are logically atomic (one concern per commit)

If any check fails, dispatch a builder sub-agent to fix. Max 2 fix loops,
then escalate.

### 8. Hand off

Output: feature branch name, commit list, oracle checklist (which commands
pass), residual risks. Do not run review, do not merge, do not push unless
the packet explicitly says so.

## Scoping Judgment (what the model must decide)

- **Test granularity.** One behavior per test. If you can't name the
  behavior in one short sentence, the test is too big.
- **When to skip TDD.** Config, generated code, UI layout, pure
  exploration. Document the skip in the commit. Everything else: test first.
- **When to escalate.** Builder loops on the same test failure 3+ times,
  the oracle contradicts the constraints, or the spec requires behavior
  that violates an invariant. Stop and report, don't power through.
- **Parallelism.** Only parallelize when file ownership is disjoint and
  oracle criteria partition cleanly. Shared files → serial builders.
- **Refactor depth.** The refactor step in TDD is local — improve the
  code you just wrote. Broader refactors are `/refactor`'s job, not yours.
- **Convex bandwidth.** A query that can grow with user data must use an index
  and bounded `.take()`/`.paginate()`. If you need all rows, you need an
  explicit batch plan and an oracle for truncation or completion.
- **Domain purity.** A feature that makes review feel easier by lying about
  due concepts is not product polish; it violates scry's memory contract.

## What /implement does NOT do

- Pick tickets (caller's job, or `/deliver` / `/flywheel`)
- Shape or re-shape specs (→ `/shape`)
- Code review (→ `/code-review`)
- QA against the running app (→ `/qa`)
- CI gates / lint (→ `/ci`)
- Simplification passes beyond TDD refactor (→ `/refactor`)
- Ship, merge, deploy (→ human, or `/settle`)
- Deploy Convex or Vercel, run production migrations, or execute forbidden
  deploy-coupled scripts

## Stopping Conditions

Stop with a loud report if:
- Packet is incomplete or ambiguous
- Oracle is unverifiable (prose-only checkboxes with no executable form —
  write one, or stop)
- Builder fails the same test 3+ times after targeted fix attempts
- Spec contradicts itself or violates a stated invariant
- Tests hit an external dependency that isn't available
- The work would require daily review caps, FSRS curve changes, or a destructive
  mutation without its reverse
- Convex implementation would require an unbounded runtime `.collect()` and no
  bounded/indexed design is present in the packet

**Not** stopping conditions: spec is hard, unfamiliar codebase, initial
tests red. Those are the job.

## Gotchas

- **Reshaping inside /implement.** If the spec is wrong, stop. Don't
  silently rewrite the oracle to match what you built.
- **Wrong branch claim.** `feat/foo` and `cx/feat/033-foo` break the backlog
  closure loop. The invariant is exactly `git checkout -b <type>/<id>-<slug>`.
- **Declaring victory with partial oracle.** "Most tests pass" is not
  green. Every oracle command exits 0, or you're not done.
- **Silent catch-and-return.** New code that swallows exceptions and
  returns fallbacks is hiding bugs. Fail loud. Test the failure mode.
- **Testing implementation, not behavior.** Tests that assert the
  structure of the code break on every refactor. Test what the code
  does from the outside.
- **Internal mocks.** Mocking `convex/fsrs/**`, `convex/lib/**`, `hooks/**`, or
  `@/lib/**` makes tests prove wiring, not behavior. Use the real module or a
  fake at the external seam.
- **Frontend before Convex.** UI compiled against imagined queries is fiction.
  Make schema/functions real, verify types, then wire React.
- **Bandwidth regressions.** `.collect()` followed by JavaScript filtering over
  `concepts`, `phrasings`, `interactions`, `generationJobs`, or IQC/action-card
  tables is not acceptable in runtime code.
- **FSRS comfort features.** Daily limits, hiding overdue concepts, or adding
  non-FSRS ease shortcuts are product regressions even when tests pass.
- **One-way mutations.** Archive, delete, and restore semantics are paired
  stories. Do not leave a scroll burned without a council.
- **Accidental deploy.** `pnpm build` is allowed when required; `pnpm
  build:local`, `pnpm build:prod`, `pnpm convex:deploy`, and
  `./scripts/deploy-production.sh` are not routine verification.
- **Committing debug noise.** `console.log`, `print("here")`, commented-out
  code. The tree must be clean before exit.
- **Skipping TDD without documenting.** Config and generated code are
  fine exceptions; silently skipping because "it was simpler" is not.
- **Parallelizing coupled builders.** Two builders editing files that
  import each other = merge pain and lost work. Partition by file
  ownership before parallel dispatch.
- **Branch drift.** Forgetting to create the feature branch and
  committing to the current branch. Always `git checkout -b` first.
- **Scope creep from builders.** Builder adds "while I'm here"
  improvements. The spec is the constraint — raise a blocker, don't
  silently expand the diff.
- **Trusting self-reported success.** Builders say "all tests pass."
  Verify by running the oracle yourself. Agents lie (accidentally).
