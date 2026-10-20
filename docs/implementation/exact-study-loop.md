# Study loop implementation (#317)

Fixed parent: `17ae38bf07f5d062e06ba3065642f149e5b323a0` (#342).
SPEC US-016/017 and US-007 own behavior; ADR004 owns the durable authority.

Existing access paths: immutable Library Versions contain ordered Sections/Units;
`study_version_refs` already prevents deletion. `engine::mutate_as` clones, checks
operation payload receipts, attributes tokens and increments revision; runtime
commits its delta with the existing SQLite revision fence. `learning::schedule`
is the pinned Again/Good adapter and `grading::grade` is the only exact grader.
Concurrent clients share this App, so no second writer or mutation queue is added.

`study` owns typed Study status/Pace, Card direction/Cue state, a frozen current
presentation, immutable Attempts with per-Unit effects, and immutable Corrections.
Callers: CLI HTTP → API adapter → engine receipt path → Study transition;
read-only views use the spend slice's noninitializing SQL read and never save.
A Prompt contains identifiers and rendered permitted context/cue, not a Version
or hidden expected answer. Answer-bearing cues are saved as assistance before
returning; reveal saves assistance before disclosure. Attempts bind Version ID
and normalized hash, exact Unit IDs, actual presented Cue, raw input, exact-v2
Authority/Diff, acting token and before/after schedule/Cue effects.

Reversible selection choices: due introduced Cards first, in due/Study/reading
order; then one new Unit in Study creation/reading order. A cued Card remains due
while it learns the Ladder. Explicit `next --card` requests extra practice.
New introductions use a Study-local rolling 24h count. Scheduled reviews use UTC
calendar day, matching the pinned adapter's scheduling convention. An unrevealed
Good counts toward two consecutive successes at the presented rung, including
Read/FirstLetters/FirstLetter practice. Explicit Reveal breaks the success run
at every rung, even when the answer is Good. Each Attempt preserves `revealed`
separately from cue assistance; answer, Correction reconstruction and archive
validation use that same fact. Fading resets the pair. Free Again drops one rung
and resets the pair; revealed Again resets the pair without a lapse.
Section/title rungs select a Unit by its public ordinal; explicit `--recite`
selects all Units in a named Section or the whole Version, with heading/title
alone. Recitations require their Units already introduced, never bypassing Pace.
Actual recitation Cue is recorded even if a Card's ordinary Cue differs.

`next` without `after` returns the existing presentation/held result. Deliberate
`next --after PROMPT_ID` alone releases feedback; selecting a different scope
while one is held conflicts. Answer and correction callers bind that frozen ID.
Correction follows US-007: only the held Attempt, one override per Unit, no later
Card mutation; recompute from the original before state/time and preserved
assistance/same-day facts. Original Attempt/Authority remain byte-identical.
Explicit resume can reactivate a dropped Study without replacing its history.
History/export/Library reads refuse while a protected exact Prompt could disclose
its answer, instead of recording help as a side effect of a read.
The shared engine receipt boundary also refuses replay of expected text, an
Attempt reference, or an old answer-bearing Prompt/context while a new Prompt
is protected. This check precedes both the saved result and resolved practice
response. Safe metadata/Correction receipts and the current permitted Prompt
still reconcile, and a changed payload still conflicts. No receipt is rewritten.

Empty Study state is omitted from old JSON/rows. Populated records use a distinct
archive format requiring this compatible writer/restore; no historical data is
imported or rewritten. Cold framework SQLite-file allocation remains distinct
from application row writes and is owned by parent #342's ruling.
Pre-fix populated `study-v1` Attempts lack an explicit Reveal fact, so this writer
refuses them rather than guessing from generic assistance. There is no populated
data migration or older-writer rollback claim. New populated archives require a
compatible forward writer that retains Reveal history and the settled fade rule.
