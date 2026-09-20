# Scry critics — run instructions

Run the critic via `scripts/critics/run.mjs` (node, no new npm deps).

## Subcommands

### candidate up|down

Manage an isolated candidate for testing.

**candidate up**
```sh
node scripts/critics/run.mjs candidate up --dir target/critics/candidate --port 18080 --out target/critics/candidate.json
```

Builds `scry` binary (or reuses `--binary <path>`), seeds a fresh SQLite DB with
`seed-fixture`, serves on loopback, polls `/readyz` while verifying the child
process is alive (each readiness attempt is bounded by a request timeout, so a
server that accepts but never answers cannot stall the attempt loop), then
writes a JSON handle. A port that is already in use is refused, and no handle
is ever written for a process that is not serving. The handle carries `id`,
`binary_sha256`, `revision`, `binary_revision`, and `source_state`
(clean/dirty/unknown) to identify the artifact it serves.

`revision` is the walking checkout; `binary_revision` is the revision the
served binary itself was built from. For the default build both are the
checkout revision by construction. For `--binary` the binary's own evidence
decides: buildinfo `vcs.revision` for plain builds, the `main.revision` stamp
(buildinfo `-ldflags`, or the binary's `version` output for gate exports built
with `-trimpath`). A determinable revision that differs from the checkout (or
cannot be compared to one) refuses the command (exit 2, no handle written).
When the revision cannot be determined, the handle records
`binary_revision: null` (`binary_revision_source: null`) and the walk binding
refuses the handle — the checkout revision is never asserted as the supplied
binary's provenance.

Exit codes: 0 success, 2 build/seed/serve failure, busy port, foreign binary
provenance, or dir not fresh.

**candidate down**
```sh
node scripts/critics/run.mjs candidate down --dir target/critics/candidate --out target/critics/candidate.json
```

Stops the recorded PID after verifying it matches the binary path. Updates handle with `stopped_at`.

### human --goal practice-review

Drive a browser walk against the candidate.

```sh
node scripts/critics/run.mjs human --goal practice-review --candidate http://127.0.0.1:18080 --handle target/critics/candidate/candidate.json --out target/critics/run1
```

Opens the review surface, discovers the live state via accessibility + DOM scans,
answers (tap for choice, type+submit for recall), observes held feedback, presses Next.
Captures screenshots: `ungraded`, `feedback`, `next`. Emits receipt JSON.

`--goal` is required and validated (`practice-review` is the only supported goal).

**Candidate binding (receipts cite the tested artifact):** pass `--handle` with
the candidate's handle file. The receipt then records the handle's `revision`,
`binary_sha256`, `handle_id` and `source_state` (`candidate.bound: true`). The
walk fails closed (exit 2, blocked receipt) when the handle is unreadable,
lacks identity fields, describes a different origin than the walk target, or
its revision does not match the walking checkout HEAD.

For a loopback handle the walk also verifies the serving artifact before
accepting the binding: the recorded PID must be alive, its command line must
reference the recorded binary (relative paths resolve against the repo root),
the binary's sha256 must equal the recorded digest, and the handle must record
the binary's own revision (`binary_revision`), equal to the revision the
receipt would claim. A stopped candidate, a port re-used by another server, a
replaced binary, or a binary whose revision is undeterminable (`null` or
missing) or differs from the claimed revision fails closed (exit 2, blocked
receipt with `walk-execution: unverified`); the receipt keeps the declared
handle identity and records the rejection in `observed`.
Non-loopback handles (`--allow-origin`) are not process-verified — `candidate
up` only ever writes loopback handles.

Without `--handle` the receipt is explicitly unbound (`bound: false`,
`revision: null`) and cannot be cited as revision-bound evidence.

Resume-safe: a persistent candidate may carry a leftover graded state; the walk
advances via Next (bounded) before starting. Recall answers use the authored
fixture answer map (see `cmd/scry/main.go` seed-fixture); an unclear match falls
back to the reveal path ("I don't know yet") to reach a graded state. Budgets
(steps/time/screenshots) are enforced (`--max-steps`, `--timeout`,
`--max-screenshots`); a limit stop is blocked (exit 2) with a receipt that
records the configured limits and used counters. A blocked run (no browser,
unreachable candidate, unsafe target, browser launch failure, rejected binding)
still writes a receipt with `walk-execution: unverified`.

Exit codes:
- 0: all selected postconditions met, no findings, coverage > 0
- 1: finding(s) with evidence
- 2: blocked/usage/environment failure (e.g. unreachable candidate, Playwright unavailable)
- 3: unverified/needs escalation (navigation blockage, two-method disagreement)

## Isolation + safety rules

- **Mutating modes** (human) run ONLY against loopback candidates.
- **Production origins** (`scry.study`, `www.scry.study`) are refused → exit 2.
- **Non-loopback** requires `--allow-origin` whose value must match the target origin exactly; production origins are always refused → exit 2.
- **Dry-run** defaults for any filing (none in this increment).
- **No cron, no schedules, no deployment.**
- **Budget limits**: max 24 steps, 120s timeout, 12 screenshots; configurable with `--max-steps`, `--timeout`, `--max-screenshots`.

## Exit codes (summary)

| code | meaning |
|------|---------|
| 0 | all checks pass, coverage > 0 |
| 1 | findings present |
| 2 | blocked/usage/environment |
| 3 | unverified/needs escalation |

## Receipt schema

See `scripts/critics/lib/receipt.mjs` — `scry-critic-receipt-v1`.

Key invariants:
- `pass` check requires `evidence.notes.length >= 1` and `evidence.screenshots` references existing files.
- `findings[].fingerprint` = `fingerprint(story, criterion, mechanism)` — deterministic dedupe identity (persona and prose never included).
- `no_findings` = true iff `findings.length === 0`.
- `coverage.exercised` lists all check ids; pass checks must be exercised.
- Zero coverage → exit 2.
- `candidate.bound: true` requires a 40/64-hex `revision`, a 64-hex
  `binary_sha256`, and a `handle` reference; `candidate.bound: false` may claim
  neither revision nor digest.
- `candidate.binary_revision` (when present) is 40/64-hex or null; an unbound
  candidate must not claim one.
- `run.budget` records the configured limits plus used `steps`/`screenshots`.

## Browser requirement (tests and CI)

- The walker and the browser test cases need Playwright plus a Chromium binary
  (`CHROMIUM_PATH` or the system paths; module via `NODE_PATH` or
  `SCRY_CRITICS_PLAYWRIGHT`). A missing browser is a blocked run with a
  receipt — never a pass.
- Test suite: browser cases skip visibly when the browser is unavailable. With
  `SCRY_CRITICS_REQUIRE_BROWSER=1` they fail instead of skipping. The CI gate
  sets this flag and provisions Debian Chromium plus the pinned Playwright
  module (`playwright@1.63.0`), so the required check exercises the browser
  paths for real.
- Containers running as root: Chromium launches with `--no-sandbox`
  automatically (or explicitly with `SCRY_CRITICS_NO_SANDBOX=1`).

## Unverified vs missing rule

Navigation blockage (e.g. no Next control visible) remains `unverified`.
Classify `missing` or `broken` only when **two independent observation methods agree**:
- Accessibility tree scan (Playwright `ariaSnapshot`)
- Raw DOM scan (visible interactive elements)

Both must independently conclude the expected control is absent → `missing`.
If only one method sees the absence or the scan is incomplete → `unverified`.

## Dedupe identity rule

Finding identity = `story + criterion + mechanism` (normalized).
Never use persona, prose, run id, or variable text.
Same story+criterion+mechanism collapses across differing prose.
Different mechanism does not collapse.

## Advisory

Jev advisory only. No shell-command execution hooks.

## Limits

- No production mutation
- No cron/schedules
- All runs bounded by budget limits

## Proof artifacts

Verified end-to-end on 2026-09-19 against a fresh isolated synthetic candidate
(`candidate up` → `human --goal practice-review`):

- `target/critics/fresh2/run/receipt.json` — exit 0: US-002.1 pass, US-002.2 pass, US-002.2-next pass
- `target/critics/fresh2/run/shots/{ungraded,feedback,next}.png`
- `target/critics/test-output.txt` — full suite output (23 pass / 0 fail / 0 skipped)
- `target/critics/blocked-check/receipt.json` — blocked-path receipt (unreachable candidate)

Committed copy: `docs/qa/critics/evidence/` (receipt + raw test output).
Screenshots stay outside Git; attach or archive them with the run.

## Portability and seams (stage-1 handoff)

This is Scry's first critic increment, proven only against Scry. Do not copy the
whole tree to another project; separate the reusable core from the
project-specific parts.

Reusable candidates (not yet proven on a second project):
- lifecycle and exit semantics (0 pass / 1 finding / 2 blocked / 3 unverified)
- receipt validation rules (a pass requires evidence and exercised coverage)
- finding identity: story/criterion + normalized mechanism (no persona, no prose)
- budgets (steps/time/screenshots) and fail-closed stops
- origin safety rules (mutating passes only on isolated synthetic targets)
- deterministic dedupe and merge across runs

Scry-specific parts (replace per project):
- goals/journeys and behavior oracles (practice-review walk, US-002 criteria)
- candidate commands (Go build, seed-fixture, loopback dev identity, serve)
- discovery semantics tied to this product's DOM/ARIA and states
- fixtures, seed data, and provider-failure scenarios
- test/release/health/rollback commands and access rules

Onboarding checklist for a new project (keep it small):
1. Read the project's USER_STORIES.md/spec; pick one or two criteria a check can
   fail on.
2. Provide an isolated synthetic environment command. Never a production target.
3. Define one goal journey and its observable postconditions.
4. Wire the receipt validator and budgets; keep the exit-code semantics.
5. Prove one real pass and one real negative (blocked environment, unsafe
   target, missing evidence) before scheduling anything.

Candidate reusable parts for misty-step/harness (extract only after a second
project's real use validates the boundary):
- receipt validator + exit-code vocabulary
- criterion+mechanism dedupe
- budget/fail-closed guard pattern

Limitations (honest):
- Criterion-bound checking today, not a fully goal-driven human critic.
- Single-project proof; no cross-project extraction yet.
- Screenshots stay outside Git; receipts carry paths, not pixels.
- No triage filing integration yet (dry-run only); no cron activation.
- Browser cases need Playwright + Chromium; a missing browser is a blocked run
  with a receipt, never a pass.
