# Synthetic empty-Unit history

`empty-unit-legacy.json` was captured by native `engine::mutate` operations on
unmodified source `fc3378f9e03e99a5b7bfbcbc7168ededefb7301c`.
It contains no production data, credentials, or external-provider output.

The supplied Version is `\nAlpha beta.` with explicit Units `\n` and
`Alpha beta.` in one Section. The capture retains these successive states:

1. Add Version (`unstarted`).
2. Start the supplied lesson (`source`).
3. Try from memory (`quiz`).
4. Submit an empty answer (`answered`). This captures the pre-fix erroneous
   unaided Good and its actual schedule effect, not a desirable grading result.
5. Select the whole-text challenge (`whole_quiz`), whose scope contains both
   an empty-normalized Unit and a meaningful Unit.

`requests` contains the exact native mutation fields for receipt replay. Each
state passed `engine::validate_app` at capture time. Native regressions validate
and round-trip the original state through rows and complete archives. The
Worker journey wraps each saved App in the existing phase-v3 archive envelope and
uses the real restore endpoint in a fresh synthetic space before attempting
current review actions. No test setter bypasses an action under test.

The fixed application retains historical grades and effects, including the bad
old result, without treating them as permission for a new Attempt. It does not
migrate Units, rewrite receipts, repair through dedupe, or erase evidence.
