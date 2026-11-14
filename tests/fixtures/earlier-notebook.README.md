# The earlier concept notebook, in every stored state

`earlier-notebook.json` holds the earlier concept notebook in each state an
existing notebook can be in. It was written by the builders in
`tests/earlier_notebook.rs`, run on unmodified source
`27067b31cc132e3161d895d4e3a9ef4c7eb92c7a`: source that still serves, schedules
and grades that notebook, so its engine can still produce every state.

It contains no production data, credentials or external-provider output. Every
Goal, answer, response and identifier in it is synthetic and comes from that
test file or from `engine::seed_fixture`.

## What each state holds

For each named state:

- `archive`: the complete encoded recovery archive, exactly as
  `persistence::Archive::encode` wrote it. It is JSON text, stored as one string.
- `sha256` and `bytes`: the hash and length of those archive bytes.
- `format`: the archive format name.
- `rows`: every stored row, name to exact value, as `persistence::rows` wrote them.
- `now_ms`: the clock when the state was saved; the archive's `created_ms`.

Three format names occur: `scry-rust-durable-sqlite-v1`,
`scry-rust-durable-sqlite-attributed-spend-v1` and
`scry-rust-durable-sqlite-go-projection-v1`.

## The states

No question held:

- `never-practised`: a Goal prepared from supplied material and never read.
  No Attempt, no held question, every question due.
- `empty-history`: three answers given and dismissed. Nothing is held and one
  question is still due.

A question held, unanswered:

- `intro`: a new Concept waiting to be introduced.
- `question-cold`: a recall question, unaided.
- `question-cold-choice`: a multiple-choice question, unaided.
- `question-helped`: the same recall question after its reference was opened.

An answer saved and not graded:

- `checking-queued`: queued for Jev by an Owner token with paid permission.
- `checking-queued-explain`: an explanation, helped by its introduction, queued
  under the semantic policy.
- `checking-sent`: sent, with its lease, start time and one unknown reservation.
- `self-check-failed`: the response could not be read; its cost is known.
- `self-check-failed-unsent`: Jev was not configured; nothing was reserved.
- `self-check-failed-send`: the send failed; the reservation stays unknown.
- `self-check-rejected`: Jev refused; the refusal is kept as received and the
  reservation stays unknown.
- `self-check-unknown`: sent, with no outcome inside the recovery time.
- `self-check-unsure`: Jev could not decide.
- `self-check-paused`: the notebook was restored while the check was queued.
- `restored-sent`: the notebook was restored while the check was sent.
- `queued-while-paused`: answered in a restored notebook; queued, never sent.
- `three-checks`: three failed checks, the limit for one answer.
- `go-retained-self-check`: an attempt the earlier Go application held before
  answer checks existed: an ungraded observation and no check record.

A Verdict held:

- `result-held`: an unaided exact answer.
- `result-jev-correct` and `result-jev-wrong`: Jev's Verdict on a recall answer.
- `result-helped`: an exact answer after the reference was opened.
- `result-corrected`: Jev's rejection, then the Owner's Correction.
- `result-revealed`: the answer shown without an attempt.
- `result-then-archived`: a held result whose Goal was archived afterwards.
- `result-then-edited`: a held result whose question was edited afterwards; it
  keeps the wording that was answered.

Everything else a notebook can keep:

- `history-rich`: four Goals. One is in focus; one is paused; one came from a
  photo, has a transcription and is archived; one never finished preparing.
  It holds exact, wrong, helped and revealed answers; explanations Jev accepted
  under the semantic policy; a Correction; a dispute with its schedule reset;
  feedback on a Concept; a question with two versions and a check superseded by
  that edit; a fix draft that was never applied; an archived question and an
  archived Concept; a Concept marked already known; notes and questions that
  quote the supplied material; the pace preference set to `light`; and one
  archived photo.

## How the states were built

Every state except one is reached through public engine entry points, with a
fixed clock and deterministic identifiers: `engine::seed_fixture`,
`engine::mutate`, `engine::mutate_as`, `engine::ensure_occurrence` (what
reading a page does), `engine::claim_work`, `engine::claim_grading_work`,
`engine::finish_work`, `engine::fail_work`, `engine::fail_received_work`,
`engine::recover_grading` and `engine::pause_for_restore`.

Two things are set on the stored structures directly:

- `go-retained-self-check`. No engine path writes a retained Go attempt, so its
  question identity, schedule epoch, ungraded observation, held self-check and
  retained Go bytes are assigned in the builder. Full validation accepts it.
- The Owner token in `checking-queued`. See the next section.

## Nothing shaped like a credential

An Owner token is identified by 32 lowercase hexadecimal characters that
issuance derives from a hash, and validation requires that shape. Masking the
identifier after the fact would make `archive` and `sha256` unverifiable, so the
builder does not issue a token. It stores one whose identifier is 32 zeros and
whose verifier is 64 zeros. No secret exists that matches them.

Nothing in the fixture is masked. `the_fixture_holds_nothing_shaped_like_a_credential`
checks every value under a field or row named for a token, key, secret or
authorization.

Other identifiers (Goals, Concepts, questions, Attempts, checks) are the
engine's own 32-character hashes of fixed seeds. Hashes of payloads, responses
and the photo are SHA-256 of synthetic content.

## What the tests prove

- `rebuilding_every_state_gives_the_committed_fixture` runs the builders on the
  current source and requires every value to equal the fixture.
- `every_state_round_trips_byte_for_byte` reads only the fixture. Each archive
  decodes against its hash and encodes again to the same bytes, hash, length
  and format name; each set of rows becomes the same notebook and the same
  rows again.
- `every_state_passes_full_validation` reads only the fixture.
- `every_state_holds_what_its_name_says` reads only the fixture.
- `the_fixture_holds_nothing_shaped_like_a_credential` reads only the fixture.

## Regenerating, and the end of the builders

To rewrite the fixture, check out the source named above and run

    SCRY_WRITE_EARLIER_NOTEBOOK_FIXTURE=1 cargo test --locked --test earlier_notebook

with a `CARGO_TARGET_DIR` of its own. Two checkouts that share a target
directory share compiled artifacts, so the test could link a later library and
the fixture would prove nothing.

Later source stops serving, scheduling and grading the earlier concept notebook
and then deletes the engine's concept-practice code. No route can create these
states after that. The builders and the rebuilding test are removed with that
code, and this fixture becomes the only source of these states. The other four
tests stay: they need only the fixture.
