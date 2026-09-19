# Critic evidence — 2026-09-19 (stage 1, increment 1)

First proven vertical: the `practice-review` walk against a fresh isolated
synthetic candidate (`candidate up` → `human --goal practice-review`).

- `receipt.json` — exit 0: US-002.1 pass, US-002.2 pass, US-002.2-next pass;
  no findings; coverage exercised x3; revision d55c694.
- `test-output.txt` — raw `node --test scripts/critics/tests/` output
  (23 pass / 0 fail / 0 skipped).

Screenshots from the run stay outside Git (target/critics/fresh2/run/shots/):
`ungraded.png`, `feedback.png` (Correct feedback + Next), `next.png` (advanced).

Negative proof (reproducible, not committed):
- production target `https://scry.study` → exit 2 before any browser launches
- unreachable candidate → exit 2 with a blocked receipt (walk-execution unverified)

Reproduce:

    node scripts/critics/run.mjs candidate up --dir target/critics/c --port 18080 --out target/critics/c/candidate.json
    node scripts/critics/run.mjs human --goal practice-review --candidate http://127.0.0.1:18080 --out target/critics/run
    node --test scripts/critics/tests/

Scope note: this is criterion-bound checking (aria/DOM discovery + authored
fixture + bounded script), not the full goal-driven human critic. See
`../critics.md` "Portability and seams" for the honest limitations.
