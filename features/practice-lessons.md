# Choose and practise lessons

Choose a topic visually, select its edition or mystery set, and practise with
native feedback. [SPEC](../SPEC.md#visual-practice-chooser-2026-11-01) owns acceptance.

## Sub-features

- The topic shelf groups NATO, Saint Michael editions and Rosary mystery sets.
- Current-session return preserves the prompt and typed draft.
- With JavaScript enabled, Save draft stores acknowledged text for the current unanswered prompt in the notebook. Anyone with access to that notebook can read it.
- With JavaScript enabled, an enhanced in-page switch away from a nonempty unsubmitted answer asks for confirmation. A confirmed switch keeps the answer read-only while its outcome is unresolved.
- Prayer practice separates source study from hidden phrase, joined and whole-prayer quizzes.
- A mistaken phrase shows expected wording and retries the same phrase.
- Rosary sequence practice identifies missing or misplaced mysteries.

## How to get to it (user POV)

Open Choose what to practise during Review, or open `/lessons` directly.
Pick Saint Michael and the intended language/edition, or a named Rosary set.
The supplied eight-line English edition and historical English edition remain
separate. Return goes to the held current session, not a promised cross-lesson resume.

## Driving it with verify-scry helpers

`node --test qa/keyed-mapping.test.mjs` calls `lessonJourney` and `lessonWalk`
from `qa/lessons.mjs`. Source study has no answer field; Try from memory is a fenced native transition.
The browser journey starts a set through its native form,
types a partial answer into `#answer`, opens/closes `.lesson-picker > summary`,
and verifies the draft and frozen prompt are unchanged. It selects the prayer,
types the next line by mistake, verifies the full expected current line, retries,
and advances to phrase 2 only after the correct phrase. It also covers whole
challenge, native/no-JS controls and offline/uncertain switch recovery.

Keep `lessons-runtime.json`, `browser.json`, and `lesson-*` screenshots from
`target/keyed-mapping/`. Inspect the 320/390px chooser and desktop screenshots,
including the long historical-edition label, current state and keyboard focus.

## Gotchas

Switching to a different lesson starts its current supported learning flow; do
not label it resume without evidence. Browsing must not import or schedule.
Starting another topic must not record an unfinished draft as an attempt. Only
text acknowledged by Save draft can be relied on as durable. A lost response may
have committed; reconcile the same operation. Unsaved edits can be lost on reload,
Back/Forward, ordinary navigation, tab close, eviction or crash. Automatic draft
persistence remains an open part of STUDY-04.
External content with the same title is not a built-in edition: identity comes
from server data. Protected reference metadata must not leak into the chooser.
Scripted browser success is not the user's acceptance of clarity.
