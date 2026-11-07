# Synthetic split-word history

`split-unit-legacy.json` was captured by native `engine::mutate` operations on
unmodified source `7446077314dade8d2df7802bc50778cb7c84da39`.
It contains no production data, credentials, or external-provider output.

The supplied Version is `Use 3.14 for pi.` with automatic segmentation. The
historical importer creates Units `Use 3.` and `14 for pi.`. The capture retains
these successive, natively validated states:

1. Add Version (`unstarted`).
2. Start the supplied lesson (`source`).
3. Try from memory (`quiz`).
4. Submit the exact first phrase, `Use 3.` (`answered`): Good.
5. Select the whole-text challenge (`whole_quiz`).
6. Submit the exact original text, `Use 3.14 for pi.` (`whole_answered`): Again.
   This captures the historical false negative, not a desirable grading result.

`requests` contains the exact native mutation fields for receipt replay, and
`source_commit` identifies the capture source. Every state passed
`engine::validate_app` at capture time. Native regressions validate and
round-trip each state through rows and complete archives, replay each recorded
operation, and assert byte-stable archived output and unchanged stored effects.

The fixed application refuses new practice on unsafe boundaries without
rewriting old Units, Attempts, grades, receipts or schedules. The old whole-text
Again remains visible in history. Dedupe is not a repair operation. Existing
source, quiz, held result and acknowledged draft states retain their original
bytes until an explicitly fenced switch to other material.

This fixture proves synthetic historical compatibility, not deployed ingress,
physical-phone behavior, provider usefulness or learning efficacy.
