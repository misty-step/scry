# Exact text practice

The owner learns a Library Version unit by unit with fading cues. He gets an
exact verdict and a word-by-word Diff, and on his phone the Stream opens
straight into the next due Unit. Behavior authority:
[US-016](../SPEC.md#us-016-learn-a-version-unit-by-unit),
[US-017](../SPEC.md#us-017-get-an-exact-verdict-and-a-word-by-word-diff),
[US-018](../SPEC.md#us-018-practise-exact-recall-on-my-phone) and
[US-007](../SPEC.md#us-007-correct-a-grade-in-one-tap).

## Sub-features

- `study-learn` creates one Study per Version (`scry learn`).
- `study-prompt` shows the Work title, the previous Unit and the cue for the Card's level, with one field and one submit control.
- `study-verdict` gives Good or Again with a Diff of correct, slipped, wrong, missing and extra words, held until Next.
- `study-assist` records Show me or a cue as an assisted Attempt before showing it.
- `study-correct` records I was right or Count as a miss as an immutable Correction.
- `study-caught-up` says when the next Card is due instead of inventing practice.
- `study-recitation` chooses a whole text or Section without opening its wording.
- `study-cli` covers the same actions through `scry next`, `answer`, `due`, `history` and `export`.

## How to get to it (user POV)

- Phone or desktop: open `/`. Before a Study, Library offers search or supplied
  text and Learn; afterwards the current Prompt or held result opens directly.
  The understanding notebook remains at `/notebook`, linked quietly as Understanding.
- After a held result or when caught up, open Recite a text and choose Whole text
  or a Section. A pending Prompt must be answered first; introduction and
  capacity refusals leave saved progress intact.
- Terminal: `scry learn VERSION`, then `scry next` and `scry answer PROMPT --text-stdin`
  (see the [command reference](../docs/owner-api.md#text-study-and-practice)).

## Driving it with verify-scry helpers

Preconditions:

- Built Worker and CLI, as in [study.md](study.md#driving-it-with-verify-scry-helpers).
- A Version added through the [Library](library.md) in the same run-owned space.

- **Phone journey.** Run `.agents/skills/verify-scry/scripts/local-journeys.mjs`.
  `study.phone` runs `scry learn` and opens `/` at 390px touch. It requires
  `[data-state="exact-prompt"]` with one textarea, fills "Your answer", and
  chooses Check my answer. It captures `[data-state="exact-result"]` on arrival
  before scrolling or reloading, then requires the same held result on reload. `[data-verdict]` and every
  `[data-diff-kind]` must equal `scry next --json`'s Attempt grade. Next then
  advances, with no horizontal overflow.
- **Interactive fixture.** `node qa/phone-serve.mjs PORT` prints a loopback URL
  whose second Unit is already in Free recall. Drive it with Playwright or the
  T3 preview; start another fixture for Show me before answering.
- **First study.** `qa/phone-library.mjs` walks empty Library → supplied Version
  → Learn → held result, with CLI readback, dedupe, protected navigation, native
  forms and interruption checks under US-018.6.
- **Story walk.** `qa/walk --stories "US-016 US-017 US-018"` covers every
  criterion. The CLI-only US-016/017 walks need no browser. US-018 retains light
  and dark at 320/390/1280px, 100/200% text at 320/390px, keyboard and no-JS forms. Native Full answer
  retains complete wording and ordered Diff without a learning event. It checks
  phone/CLI parity on a restored frozen Prompt, response loss, stale tabs,
  offline pause and private DOM removal. `node --test qa/study.test.mjs` reuses
  the CLI journey. See the [builder receipt](../docs/qa/evidence/scry-319/2026-10-20-recovery-links/README.md).
- **Proof.** `study-prompt-390.png`, `study-diff-390.png`, `study-next-390.png`
  and `cli-transcript.json` under the `<stamp>-local-journeys` evidence directory.

## Gotchas

- A new Card starts with an answer-bearing cue, so its first typed Attempt is
  assisted and leaves the schedule unchanged ("Assisted practice · the schedule
  stayed unchanged"). Free recall needs the fading in `qa/phone-stream.mjs`.
- Capture the result arrival before scrolling or reloading: verdict and the
  first changed word/replacement must be unobscured. Native Full answer holds
  the complete ordered Diff; the actions follow in ordinary flow.
  `qa/feedback-arrival.mjs` retains viewport geometry and hit-test evidence,
  while the existing full-page captures retain expanded completeness.
- The fixture is authored synthetic wording under Creed metadata. It establishes
  no private canonical wording or physical-phone acceptance; emulation is not hardware.
- On production, `/` freezes the next Prompt. Never drive exact practice there;
  hosted API admission (#335) and independent hosted restore remain unverified.
- Source: `src/study.rs`, `src/engine.rs`, `src/api.rs`, `src/runtime.rs`,
  `src/web.rs`, `src/bin/scry.rs`, `assets/app.js`; grading and scheduling in
  `src/grading` and `src/learning.rs`.
