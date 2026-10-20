# verify-scry end-to-end run — 2026-10-20

The first full run of [verify-scry](../../../../../.agents/skills/verify-scry/SKILL.md)
followed its Drive section in order, from 22:04 to 22:05 UTC on host `mirrodin`.
The source was master `4358124` plus this change's helpers, the same bytes as the
PR. [run-transcript.txt](run-transcript.txt) has every command, PASS/FAIL/UNPROVEN
line and exit code. Each directory holds a helper's `checks.jsonl`, `summary.json`
and screenshots.

## Targets and data

- Production https://scry.mistystep.io, Worker `scry-rust-production`, deployment
  `d40a58cf` (code `69435af`, `OWNER_SUBJECT` written 20:53Z). Read-only:
  Cloudflare API reads, refused requests, and the isolated `scry-qa` service identity.
  No owner page was loaded and nothing was written.
- Local: built WASM Worker in disposable workerd SQLite/R2 with no egress, the
  compiled `scry` CLI, and Playwright Chromium at 390px touch and 1280px.
  Authored synthetic text only.

## Results

| Feature | Result | Evidence |
| --- | --- | --- |
| Doctor: owner identity configuration | PASS 9, UNPROVEN `owner.subject` (secret unreadable) | [doctor-prod](2026-10-20T220415Z-doctor-prod/checks.jsonl) |
| Doctor: local Worker | PASS | [doctor-local](2026-10-20T220420Z-doctor-local/checks.jsonl) |
| Owner sign-in, signed-out → Access → Misty Step "Welcome back" | PASS at 390 and 1280 | [390](2026-10-20T220423Z-owner-signin/signin-idp-390.png), [1280](2026-10-20T220541Z-owner-signin/signin-idp-1280.png), [hops](2026-10-20T220423Z-owner-signin/navigation.txt) |
| **Owner sign-in as phaedrus@mistystep.io** | **UNPROVEN: needs his Misty Step password in pass** | [checks](2026-10-20T220423Z-owner-signin/checks.jsonl) |
| Signed out, expired/forged session, wrong identity, alternate host, 390px | PASS | [prod-states](2026-10-20T220439Z-prod-states/checks.jsonl), [QA view 390](2026-10-20T220439Z-prod-states/qa-identity-create-390.png) |
| Hosted Owner-token admission (#335) | UNPROVEN (uncommissioned) | same |
| Empty notebook and legacy concept flow | PASS | [empty](2026-10-20T220444Z-local-journeys/notebook-empty-390.png), [held result](2026-10-20T220444Z-local-journeys/notebook-held-result-390.png), [concept](2026-10-20T220444Z-local-journeys/notebook-concept-390.png) |
| Owner token → CLI login → revoke | PASS | [shown once (masked)](2026-10-20T220444Z-local-journeys/token-shown-once-390.png), [list](2026-10-20T220444Z-local-journeys/token-list-390.png), [CLI](2026-10-20T220444Z-local-journeys/cli-transcript.json) |
| Library find and keep | PASS | [CLI](2026-10-20T220444Z-local-journeys/cli-transcript.json) |
| Exact Study on the phone Stream | PASS | [prompt](2026-10-20T220444Z-local-journeys/study-prompt-390.png), [arrival](2026-10-20T220444Z-local-journeys/study-result-arrival-390.png), [Diff](2026-10-20T220444Z-local-journeys/study-diff-390.png), [next](2026-10-20T220444Z-local-journeys/study-next-390.png) |
| Backend down | PASS (local) | [390](2026-10-20T220444Z-local-journeys/backend-down-390.png) |

The 1280px local run is in [2026-10-20T220509Z-local-journeys](2026-10-20T220509Z-local-journeys/checks.jsonl).
The first 1280px sign-in attempt hit a transient host `net::ERR_NETWORK_CHANGED`
([kept](2026-10-20T220433Z-owner-signin/checks.jsonl)); the retry is logged at the end of the transcript.

## Observations

- On a 390×844 phone, the sticky Next / I was right bar covers the first row of
  the word-by-word Diff when the result arrives
  ([arrival](2026-10-20T220444Z-local-journeys/study-result-arrival-390.png)).
  "Again." is visible, but which words were wrong is not until the learner
  scrolls. At 1280×844 the Diff starts below the fold. US-018.2's held
  Verdict and Diff are present and equal to the CLI's; this is a hesitation, not a criterion failure.
- The owner's Access account recorded One-time PIN logins at 21:49Z and 21:56Z
  (account-wide last-seen identity) with no Scry login logged. This run did not cause them.

## Limits

Owner admission (`OWNER_SUBJECT` = his `user_uuid`) is not proven. Neither is
his own browser load after the 20:53Z re-point; see the runbook. Emulated
widths are not physical-phone acceptance. Synthetic text proves no canonical
wording, provider usefulness, hosted CLI admission or hosted recovery.
