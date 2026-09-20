# Critic evidence — 2026-09-20 (stage 1, increment 1)

First proven vertical: the `practice-review` walk against a fresh isolated
synthetic candidate (`candidate up` → `human --goal practice-review`).

- `receipt.json` — exit 0: US-002.1 pass, US-002.2 pass, US-002.2-next pass;
  no findings; coverage exercised x3. **Candidate-bound**: the receipt cites
  the tested handle (`cand-59869625646e`), checkout revision `ad272f5`, the
  served binary's own revision `binary_revision: ad272f5` (verified against
  the binary the candidate served), the built `binary_sha256`, and
  `source_state: clean`. The handle's `source_state_source: source-checkout`
  records that a default build's state comes from the checkout (== build
  source); a `--binary` handle's state comes from the binary's own buildinfo
  (`vcs.modified` → clean/dirty, or `unknown` for stamp-only exports) and
  never from the walking checkout. A walk whose handle revision does not
  match the checkout HEAD, whose binary was replaced, or whose binary
  revision is undeterminable fails closed (exit 2, receipt) instead of
  producing a false claim.
- `test-output.txt` — raw `node --test scripts/critics/tests/` output
  (65 pass / 0 fail / 0 skipped) with a real browser; the browser cases
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
- `candidate up --binary <built from a modified tree at the checkout rev>` →
  handle records `source_state: dirty` (`source_state_source: buildinfo-vcs`)
  from the binary's own `vcs.modified`; the bound walk receipt carries
  `dirty`, never `clean`
- `candidate up --binary <stamped/-trimpath export at the checkout rev>` →
  handle records `source_state: unknown` (a stamp carries no tree state); the
  walk still binds, labeled `unknown`
- a `/readyz` that accepts but never answers → each readiness attempt is
  bounded by a request timeout; the attempt loop advances instead of stalling

Reproduce:

    node scripts/critics/run.mjs candidate up --dir target/critics/c --port 18080 --out target/critics/c/candidate.json
    node scripts/critics/run.mjs human --goal practice-review --candidate http://127.0.0.1:18080 --handle target/critics/c/candidate.json --out target/critics/run
    node --test --test-timeout=180000 scripts/critics/tests/

Scope note: this is criterion-bound checking (aria/DOM discovery + authored
fixture + bounded script), not the full goal-driven human critic. See
`../critics.md` "Portability and seams" for the honest limitations.
