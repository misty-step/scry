# Exact text practice

The owner studies a Library Version source, then explicitly quizzes from memory. He gets an
exact verdict and a word-by-word Diff, and on his phone the Stream opens
straight into the next due Unit. Behavior authority:
[US-016](../SPEC.md#us-016-learn-a-version-unit-by-unit),
[US-017](../SPEC.md#us-017-get-an-exact-verdict-and-a-word-by-word-diff),
[US-018](../SPEC.md#us-018-practise-exact-recall-on-my-phone) and
[US-007](../SPEC.md#us-007-correct-a-grade-in-one-tap).

## Sub-features

- `study-learn` creates one Study per Version (`scry learn`).
- `study-prompt` shows source without an input, then Try from memory selects a hidden-source quiz with one field and one submit control. Historical phase-less API cues remain compatible.
- `study-verdict` gives Good or Again with a Diff of correct, slipped, wrong, missing and extra words, held until Next.
- `study-assist` records Show me or a cue as an assisted Attempt before showing it.
- `study-correct` preserves immutable historical Corrections; new learner-supplied Verdicts return 410 under the [retirement amendment](../SPEC.md#review-first-entry-and-retired-native-generation-2026-10-27-406).
- `study-caught-up` says when the next Card is due instead of inventing practice.
- `study-cli` covers the same actions through `scry next`, `answer`, `due`, `history` and `export`.
- `study-history` reads saved Attempts and appended Corrections from the footer's
  History link, with frozen wording, Diff, Authority and per-Unit schedule effects.

## How to get to it (user POV)

- Phone or desktop: choose Review (`/recall`). Before a Study, Library offers search or supplied
  text and Learn; afterwards the current Prompt or held result opens directly.
  Review Home is `/`; saved earlier practice remains separate at `/notebook`.
- Terminal: `scry learn VERSION`, then `scry next`, `scry quiz PROMPT` to leave source study, and `scry answer PROMPT --text-stdin`
  (see the [command reference](../docs/owner-api.md#text-study-and-practice)).

## Driving it with verify-scry helpers

Preconditions:

- Built Worker and CLI, as in [study.md](study.md#driving-it-with-verify-scry-helpers).
- A Version added through the [Library](library.md) in the same run-owned space.

- **Phone journey.** Run `.agents/skills/verify-scry/scripts/local-journeys.mjs`.
  `study.phone` runs `scry learn` and opens `/recall` at 390px touch. It requires
  `[data-state="exact-prompt"]` with one textarea, fills "Your answer", and
  chooses Check my answer. It captures `[data-state="exact-result"]` on arrival
  before scrolling or reloading, then requires the same held result on reload. `[data-verdict]` and every
  `[data-diff-kind]` must equal `scry next --json`'s Attempt grade. Next then
  advances, with no horizontal overflow.
- **Interactive fixture.** `node qa/phone-serve.mjs PORT` prints a loopback URL
  whose second Unit is already in Free recall. Drive it with Playwright or the
  T3 preview; start another fixture for Show me before answering.
- **First study.** `qa/phone-library.mjs` walks empty Library → supplied Version
  → Learn → source study → Try from memory → native quiz → held result, with CLI readback, dedupe, protected navigation, native
  forms and interruption checks under US-018.6.
- **History.** `qa/phone-history.mjs`, called by the existing phone Stream walks,
  exercises native disclosures, saved CLI/export parity, original Verdicts and
  Corrections, reveal versus empty answers, frozen Versions, dropped Studies,
  protected navigation and private return. `tests/study.rs` covers rendering
  from real engine operations; `qa/study.mjs` retains SQLite/restart/R2 parity.
  Exact Prompt protection gates all historical wording; a protected legacy
  question gates exact Attempts and preserves its per-concept history fence.
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

## Forward keyed mappings (NATO first)

Externally authored mapping manifests enter through `scry mapping add --file`
and the shared Library API. Learn the returned Version and open Text practice.
The first Prompt shows only the key, with one typed answer and Check my answer.
Results show the canonical value and native exact-v2 Diff, held until Next.
Mapping results have no self-rating or Owner Correction. Show me records durable
assistance and leaves the schedule unchanged, including a later correct answer.

`tests/keyed_mapping.rs`, `tests/keyed_mapping_library.rs` and
`qa/keyed-mapping.test.mjs` own the bounded US-021 forward subset. The canonical
`examples/nato-alphabet.json` is supplied content, not generated runtime output.
Reverse directions, choice scaffolding, random-start sets, year tolerance and
multimedia are not implemented by this subset; hosted #335 remains unverified.


## Authored lessons (LESSON-01–06)

The `/lessons` shelf and inline review disclosure start a bundled Rosary/prayer
or supplied Version lesson without replacing existing history. This is not the
stopped generic phone recitation chooser. Source: `src/lessons.rs`,
`assets/lessons.json`, native lesson mutations in `src/study.rs`, ordered alias
policy in `src/ordered.rs`, and lesson rendering in `src/web.rs`.

Verification: `tests/lessons.rs` covers deliberate switching, stale fences,
assisted clauses, joined/whole challenges, supplied content, unchanged Pace,
idempotent starters and recovery. `tests/ordered.rs` covers alignment, aliases,
duplicates, ambiguity and immutable identity. `qa/lessons.mjs` is called by the
existing keyed-mapping Worker/browser gate: real native forms, CRLF, actual
phone/desktop/dark/no-JavaScript flows, draft preservation and source-bound images.
Local source tests and synthetic screenshots are not authenticated live acceptance.

## Empty-normalized text boundaries

`tests/empty_text_units.rs` and the `emptyTextJourney` in
`qa/study.test.mjs` exercise US-015/016/017 and LESSON-04: import a leading
newline or ellipsis, source → quiz → empty answer, and restore pre-fix
synthetic source/quiz/result/whole-text history before trying new actions.
The runtime receipt is `target/empty-text-units-runtime.json`; inspect its
SQLite no-write refusal and replay results alongside the native archive tests.
Automatic segmentation keeps every source byte but attaches separators to a
word-bearing Unit. Explicit empty-normalized boundaries are refused. Retained
Versions with such boundaries remain readable and restorable, but are excluded
from new practice in every scope. A normalized reimport returns the original
Version, not repaired boundaries. Original grades, schedules and exact receipts
remain historical evidence; no production content was inspected or migrated.

Quarantined prompts show an unavailable state, a copyable acknowledged draft,
and an explicit fenced action to study other material. Library and the lesson
shelf do not offer a new start for affected Versions. `emptyTextWalk`, included
in the existing keyed-mapping browser gate, covers this recovery at 320px native
no-JavaScript and 1280px enhanced layouts without creating an Attempt.

## Split-word text boundaries

`tests/split_text_units.rs` and `splitTextJourney` in `qa/study.test.mjs`
exercise US-015/016/017 and LESSON-04 with decimal, ellipsis, adjacent-punctuation
and Unicode text. New automatic segmentation preserves every source byte and
keeps native normalized words together. Explicit boundaries are accepted only
when normalizing all joined Units yields the same word vector as normalizing
the individual Units, including cuts between Sections. This does not change
`exact-v2`, its pinned Unicode rules, Diff spans, or accepted answer aliases.

The runtime receipt is `target/split-text-units-runtime.json`. The pre-fix
synthetic fixture retains the false Again for an exact whole-text answer,
including its schedule effect. Saved Versions remain unchanged and restorable;
unsafe ones use the same unavailable-practice fence as empty Units. Reads,
archives and exact committed receipts retain their historical meaning. Ordinary
reimport still deduplicates to the saved Version and is not a boundary repair.
No repair successor or production migration is included.

`splitTextWalk`, in the existing keyed-mapping browser gate, checks the accurate
unavailable reason, acknowledged draft, reload and fenced switch to other
material at 320px native/no-JavaScript and 1280px enhanced layouts. These
synthetic checks do not establish physical-phone or authenticated live acceptance.
