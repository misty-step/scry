# Synthetic cue-ladder and quiz-first history

`cue-ladder-legacy.json` was captured by native `engine::mutate` operations on
unmodified source `fb9c5d281f84dbd6c1d3541c23d2d16f0963af97`, the last source
on which a Study started by `learn` climbed a cue ladder, a mapping pair opened
as a Quiz and a Library-started ordered list was drilled item by item. It
contains no production data, credentials or external-provider output.

Each state holds the complete recovery archive as that source encoded it
(`archive`, its `sha256` and `format`), the time of its last operation
(`now_ms`) and how many of its group's `requests` had run. A held state also
holds `answer`: the exact native request that answered its Prompt on that
source, and the checksum of the complete archive that source produced from it.
`held_first_letters` also holds its stored `rows`.

## `ladder`: Studies started by `learn` and driven by `next`

One notebook with three Studies, none of them a lesson and none marked
`source_quiz`. Its archives use `scry-rust-durable-sqlite-study-scheduled-v2`.

- **Synthetic Canticle**, a text of four phrases in two Sections. Phrase 1
  climbed read, first letters and first letter to an unaided scheduled answer,
  then faded once more on early practice: its Card is at `section_heading`.
  Phrase 2 climbed to its first unaided scheduled answer: `previous_line`.
  Phrase 3 reached first letters and was then missed beside them:
  `first_letters`. Phrase 4 was never introduced.
- **Synthetic harbour lamps**, a mapping of three pairs. Pair 1 was answered
  correctly and pair 2 with another pair's value, both as Quizzes with no study
  first. Pair 3 was never introduced.
- **Five synthetic landmarks**, an ordered list. Item 1 was read and typed once
  as a single item: `read`. Items 2 to 5 were never introduced.

Three states branch from that history, each holding one unanswered Prompt with
no phase:

1. `held_first_letters`: phrase 3 beside its first letters, `t p r b l a n`.
2. `held_list_item`: list item 1 beside its own text.
3. `held_mapping_key`: pair 3 as a Quiz from its key, never studied.

## `quiz_first`: app Studies before mappings and lists were studied first

One notebook driven through the app's routes, in order. Its archives use
`scry-rust-durable-sqlite-study-phase-v3`.

1. Start NATO practice and answer A, which opened as a Quiz.
2. Start the Safe soothing lesson (`baby_held_quiz`): its first pair is held as
   a Quiz, never studied. It is then answered.
3. Start the Joyful Mysteries (`rosary_held_quiz`): the whole set is held as a
   Quiz, never studied. It is then answered (`rosary_answered`).
4. Return to NATO (`nato_held_quiz`): B is held as a Quiz, never studied.

## What the regressions prove

`tests/study_then_quiz.rs` decodes every archive, validates it, re-encodes it
to identical bytes, loads and rewrites the stored rows unchanged and replays the
recorded operations. It submits each recorded `answer` and requires the
resulting archive checksum to equal the one the earlier source produced, so a
Prompt held from these records is recorded exactly as it would have been. It
then requires every later Prompt to be source study or a Quiz, a Card's stored
cue level never to choose a presentation, and the item-drilled list to be
recalled whole. `tests/study_quiz.rs` and `tests/study.rs` reuse the same
notebook for archive format and receipt-replay fences.

Every state passed `engine::validate_app` at capture time. This fixture proves
synthetic historical compatibility, not deployed ingress, physical-phone
behavior, provider usefulness or learning efficacy.
