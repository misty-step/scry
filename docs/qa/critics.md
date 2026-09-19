# Scry critics — run instructions

Run the critic via `scripts/critics/run.mjs` (node, no new npm deps).

## Subcommands

### candidate up|down

Manage an isolated candidate for testing.

**candidate up**
```sh
node scripts/critics/run.mjs candidate up --dir target/critics/candidate --port 18080 --out target/critics/candidate.json
```

Builds `scry` binary, seeds a fresh SQLite DB with `seed-fixture`, serves on loopback,
polls `/healthz` and `/readyz`, writes a JSON handle.

Exit codes: 0 success, 2 build/seed/serve failure or dir not fresh.

**candidate down**
```sh
node scripts/critics/run.mjs candidate down --dir target/critics/candidate --out target/critics/candidate.json
```

Stops the recorded PID after verifying it matches the binary path. Updates handle with `stopped_at`.

### human --goal practice-review

Drive a browser walk against the candidate.

```sh
node scripts/critics/run.mjs human --goal practice-review --candidate http://127.0.0.1:18080 --out target/critics/run1
```

Opens the review surface, discovers the live state via accessibility + DOM scans,
answers (tap for choice, type+submit for recall), observes held feedback, presses Next.
Captures screenshots: `ungraded`, `feedback`, `next`. Emits receipt JSON.

Resume-safe: a persistent candidate may carry a leftover graded state; the walk
advances via Next (bounded) before starting. Recall answers use the authored
fixture answer map (see `cmd/scry/main.go` seed-fixture); an unclear match falls
back to the reveal path ("I don't know yet") to reach a graded state. Budgets
(steps/time/screenshots) are enforced. A blocked run (no browser, unreachable
candidate, unsafe target) still writes a receipt with `walk-execution: unverified`.

Exit codes:
- 0: all selected postconditions met, no findings, coverage > 0
- 1: finding(s) with evidence
- 2: blocked/usage/environment failure (e.g. unreachable candidate, Playwright unavailable)
- 3: unverified/needs escalation (navigation blockage, two-method disagreement)

## Isolation + safety rules

- **Mutating modes** (human) run ONLY against loopback candidates.
- **Production origins** (`scry.study`, `www.scry.study`) are refused → exit 2.
- **Non-loopback** requires `--allow-origin` whose value must NOT be a production origin → exit 2.
- **Dry-run** defaults for any filing (none in this increment).
- **No cron, no schedules, no deployment.**
- **Budget limits**: max 24 steps, 120s timeout, 12 screenshots (configurable).

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
