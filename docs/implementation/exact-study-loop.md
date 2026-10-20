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
calendar day, matching the pinned adapter's scheduling convention. Every Good
counts toward two consecutive successes at the presented rung, including assisted
practice; fading resets that pair. Free Again drops one rung and resets the pair.
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

Empty Study state is omitted from old JSON/rows. Populated records use a distinct
archive format requiring this compatible writer/restore; no historical data is
imported or rewritten. Cold framework SQLite-file allocation remains distinct
from application row writes and is owned by parent #342's ruling.
