# Concepts, references, and goals

Everything saved in the earlier concept notebook is reachable from the Menu by
following links, read-only: the Map shows every Goal, archived ones after the
live ones and labelled; a Goal's page its Concepts (archived ones labelled), its
original request, transcription and photo link; a Concept's page every note in
order with its provenance, every saved question (archived ones labelled), its
observed counts and its prerequisite links both ways; and a saved question's
page its whole record. Nothing stands before them, reading them writes nothing,
and no page offers a write. Walk journeys: US-001, US-006, US-009, US-010. Intent:
[SPEC § Earlier concept notebook](../SPEC.md#earlier-concept-notebook).

## Sub-features

- `concept-home` (`/`) is Study; it never shows or selects an earlier question.
- `map-list` shows goals and their concepts at `/map`, in an accessible list.
  Each Goal shows its stored "In focus", "Paused" and "Archived" as text; each
  Concept the number of answers recorded for it, and "Archived" when it is.
  Every star of the constellation is drawn alike; a Goal with no live Concept
  draws none.
- `map-goal` opens a goal (`/goals/{id}`) with its reference and original request.
- `map-concept` opens a concept (`/concepts/{id}`) with every note, saved
  questions and answers, observed counts of recalled, helped and missed
  answers (of its live questions), and prerequisite links both ways; links to
  archived Concepts are labelled.
- `map-question` (`/questions/{id}/edit`, the URL the edit form had) shows a
  saved question's record, read-only: the current version; each earlier
  version and any fix draft never applied, in native disclosures; every
  recorded answer (`#attempt-{event}`) with its result, Authority, the version
  it was given to, any Correction (`#correction-{id}`) and saved check
  (`#check-{id}`, a fixed label for its status, never its stored message);
  answers saved with no Verdict (`#saved-{occurrence}`); and any schedule reset
  (`#reset-{id}`) with the feedback that caused it.
- The pace (Settings "New items per day"), focus, pause, archive, edit,
  feedback and dispute are retired: their POSTs answer 410 before anything is
  written, and the stored pace, focus and pause stay in export unchanged.

## How to get to it (user POV)

- Menu → Earlier practice → Map → a goal → a concept → "The saved question".
  Follow displayed links, never invented IDs. The concept chip above a held
  question opens its Concept. `tests/earlier_reachable.rs` and
  `qa/earlier-notebook.test.mjs` crawl these links from the Menu and assert
  every stored thing is on a page reached.

## Driving it with verify-scry helpers

Preconditions:

- A run-owned synthetic notebook holding a stored state loaded with
  `qa/earlier-fixture.mjs` ([study.md](study.md)). The authored demo is retired.

- **Journey.** `.agents/skills/verify-scry/scripts/local-journeys.mjs` →
  `notebook.concept` opens Map, then `a[href^="/goals/"]`, then the first
  `main a[href^="/concepts/"]`, and accepts only `[data-state="concept"]`.
  Evidence: `notebook-concept-390.png`.
- **Story walk.** `qa/walk --stories "US-001 US-006 US-009 US-010"`. US-001
  crawls every page from the Menu at 390 px and asserts every stored thing is
  reached. US-010
  proves nothing is rewritten or deleted: no control, every retired POST 410,
  rows identical, stored flags shown as stored.
- **Proof.** Inspect desktop and phone navigation, not the decorative constellation or a process exit.

## Gotchas

- Opening `/history`, `/goals/*`, `/concepts/*`, `/photos/*` or `/export` is a
  plain read. It marks no exposure and changes no row.
- There is no assistance gate. A held earlier question is not a Prompt awaiting
  an answer and protects nothing; an open Study Prompt still does.
- Every write to it is refused by the Worker's admission
  (`engine::admit_current_mutation`), 410 with one message, in the browser and
  the API (`POST /api/v1/settings`): any three-part `/goals|concepts|questions/{id}/{action}`,
  the exact path `/settings`, and `/__fixture`. A replay of a committed
  operation is refused too. `/settings/backup`, `/settings/resume` and
  `/settings/judgment` are not affected. No engine function is left behind the
  old routes: the stored states come only from
  `tests/fixtures/earlier-notebook.json`.
- History (`/history`) lists each Event as before but does not yet link to its
  question or list answers saved without a Verdict: its bytes are pinned by
  `tests/judgment_off.rs` and `tests/fixtures/judgment-off.json`.
- Do not use historical live stores as fixtures.
- Source: `src/learning.rs`, `src/model.rs`, `src/engine.rs`, `src/web.rs`, `assets/*`.
