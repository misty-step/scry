# Synthetic Recitation over phrases not yet introduced

`partial-recitation-legacy.json` was captured by native `engine::mutate`
operations on unmodified source `f87db8aa4a6f93e7b3f461dfe9ff88ab01d4a751`,
the last source on which `/practice/lesson/next` accepted a `whole` or `join`
Recitation for a text Study whose Units were not all introduced. It contains no
production data, credentials or external-provider output.

Each state is the complete notebook (`App`) as that source held it in memory.
None is a recovery archive: that source already refused to encode all four
(`422 Invalid frozen Study Prompt.`), which is the defect.

Both branches start the same way: add **Synthetic Canticle** (eight phrases in
two Sections of four), start Study's mix, which starts its Study as a lesson
and introduces phrase 1 only, try it from memory and answer it exactly.

- `whole_held`: `lesson/next` with `mode=whole` then held a whole-Version
  Recitation over phrase 1 alone.
- `whole_answered`: that Prompt answered exactly, recording its Attempt.
- `join_held`: `lesson/next` with `mode=join` instead held a Recitation of the
  Section "Opening" over phrase 1 alone.
- `join_answered`: that Prompt answered exactly, recording its Attempt.

The fixed application refuses to hold either Prompt and refuses to answer or
reveal a held one. It does not rewrite or remove a recorded Attempt, and the
validation rule that refuses these states is unchanged: the two `answered`
states stay refused by recovery validation.
