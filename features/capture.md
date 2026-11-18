# Retired generation and saved preparation history

Native content generation and self-grading are [non-goals](../SPEC.md#purpose-and-non-goals);
[SPEC § Retired generation records](../SPEC.md#retired-generation-records) owns what stays readable.
Existing requests, drafts, material, Attempts, Corrections and provider/spend
history remain preserved. [SPEC § Backup and recovery](../SPEC.md#backup-and-recovery)
owns complete recovery.

## Sub-features

- Legacy `/create` and `/add` explain retirement and keep an unsent text draft
  copyable. They never offer preparation or photo-upload controls.
- New generation, retry, refinement and question-generation submissions refuse
  before jobs, assets, receipts, reservations or provider calls.
- Existing generation jobs remain historical: reads, alarms and meaning-check
  resumption cannot send or publish them.
- `goal-generation-records` (`/goals/{id}`, section "Generation records"): one
  read-only entry per job (`#job-{id}`) with its kind, date, attempt, recorded
  status and model attribution, its recorded outcome and defect reasons, then
  closed native disclosures holding the saved candidates, critic result, kept
  critic results and raw response exactly as stored; each request's usage
  (`#usage-{id}`) shows purpose, model, known cost or the counted unknown
  reservation, and its raw response. Import markers are not shown as models.
- Native answer grading continues. Unsupported earlier answers remain saved and
  ungraded rather than requesting a learner-supplied Verdict.

## How to get to it (user POV)

Home → native review or Start with NATO. Library contains supplied content.
Earlier notes and concepts remain a secondary destination when saved material
exists. Legacy links remain safe to read; they do not reactivate preparation.

## Driving it with verify-scry helpers

`tests/generation_retirement.rs` exercises current admission, grading-only
claims, preserved historical jobs and read-only Home/ungraded answers.
`tests/earlier_reachable.rs` and `qa/earlier-notebook.test.mjs` crawl from the
Menu and find every stored job and usage entry on its Goal's page, inert.
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
