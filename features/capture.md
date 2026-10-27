# Retired generation and saved preparation history

Native content generation and self-grading are retired under the
[2026-10-27 amendment](../SPEC.md#review-first-entry-and-retired-native-generation-2026-10-27-406).
Existing requests, drafts, material, Attempts, Corrections and provider/spend
history remain preserved. [S10](../SPEC.md#s10--keep-my-work-recoverable) still
owns complete recovery.

## Sub-features

- Legacy `/create` and `/add` explain retirement and keep an unsent text draft
  copyable. They never offer preparation or photo-upload controls.
- New generation, retry, refinement and question-generation submissions refuse
  before jobs, assets, receipts, reservations or provider calls.
- Existing generation jobs remain historical: reads, alarms and meaning-check
  resumption cannot send or publish them.
- Native answer grading continues. Unsupported earlier answers remain saved and
  ungraded rather than requesting a learner-supplied Verdict.

## How to get to it (user POV)

Home → native review or Start with NATO. Library contains supplied content.
Earlier notes and concepts remain a secondary destination when saved material
exists. Legacy links remain safe to read; they do not reactivate preparation.

## Driving it with verify-scry helpers

`tests/generation_retirement.rs` exercises current admission, grading-only
claims, preserved historical jobs and read-only Home/ungraded answers.
`qa/generation-retirement.test.mjs` exercises the built Worker refusal boundary,
including multipart requests and API calls, checking unchanged SQLite/R2 state.
Historical generation policy fixtures remain compatibility proof, not evidence
of an active generation feature. The 2026-10-27 local actual-Worker runtime
suite passed 40/40, including retirement refusal, native grading, isolation and
historical recovery; native tests passed 194 with one pre-existing ignored.
These are synthetic worktree receipts, not release acceptance. Browser proof,
independent review and exact-head protected CI are required before release.

## Gotchas

- A missing provider is not the retirement policy: refusal is unconditional.
- Retiring self-grading never deletes or relabels old learner-authored results.
- Restoring/resuming meaning checks cannot resume historical generation jobs.
- Source changes do not establish deployment, physical-phone acceptance or
  independent hosted recovery.
