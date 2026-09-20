# Critic evidence — 2026-09-20 (stage 1, increment 1)

First proven vertical: the `practice-review` walk against a fresh isolated
synthetic candidate (`candidate up` → `human --goal practice-review`).

- `receipt.json` — exit 0: US-002.1 pass, US-002.2 pass, US-002.2-next pass;
  no findings; coverage exercised x3. **Candidate-bound**: the receipt cites
  the tested handle (`cand-b678b6e5006a`), checkout revision `db04ef5`, the
  served binary's own revision `binary_revision: db04ef5` (verified against
  the binary the candidate served), the built `binary_sha256`, and
  `source_state: clean`. A walk whose handle revision does not match the
  checkout HEAD, whose binary was replaced, or whose binary revision is
  undeterminable fails closed (exit 2, receipt) instead of producing a false
  claim.
- `test-output.txt` — raw `node --test scripts/critics/tests/` output
  (60 pass / 0 fail / 0 skipped) with a real browser; the browser cases
  exercise the fixtures for real and report visible skips (never passes)
  when the environment cannot run them.

Screenshots from the run stay outside Git (target/critics/evidence-run/shots/):
`ungraded.png`, `feedback.png` (Correct feedback + Next), `next.png` (advanced).

Negative proof (reproducible, not committed):
- production target `https://scry.study` → exit 2 before any browser launches
- unreachable candidate → exit 2 with a blocked receipt (walk-execution unverified)
- browser launch failure → exit 2 with a blocked receipt
- occupied port on `candidate up` → exit 2, no handle written
- `candidate up --binary <built from another revision>` → exit 2, no handle
  written (`--binary was built from revision X, but the checkout HEAD is Y;
  refusing to record a handle that would mislabel the served artifact`)
- `candidate up --binary <revision undeterminable>` → handle records
  `binary_revision: null`; the walk binding refuses it (exit 2, blocked
  receipt with `walk-execution: unverified`)
- a `/readyz` that accepts but never answers → each readiness attempt is
  bounded by a request timeout; the attempt loop advances instead of stalling

Reproduce:

    node scripts/critics/run.mjs candidate up --dir target/critics/c --port 18080 --out target/critics/c/candidate.json
    node scripts/critics/run.mjs human --goal practice-review --candidate http://127.0.0.1:18080 --handle target/critics/c/candidate.json --out target/critics/run
    node --test --test-timeout=180000 scripts/critics/tests/

Scope note: this is criterion-bound checking (aria/DOM discovery + authored
fixture + bounded script), not the full goal-driven human critic. See
`../critics.md` "Portability and seams" for the honest limitations.
