# Concepts, references, and goals

The earlier concept notebook's Goals, Concepts, notes and saved questions stay
readable: the Map shows Goals and connections, a Goal's page its reference and
original request, a Concept's page its note, saved questions with their answers
and its observed counts, and a saved question's page its wording, choices,
answer and explanation. Nothing stands before them, reading them writes nothing,
and no page offers a write. Walk journeys: US-001, US-006, US-009, US-010. Intent:
[SPEC § Earlier concept notebook](../SPEC.md#earlier-concept-notebook).

## Sub-features

- `concept-home` (`/`) is Study; it never shows or selects an earlier question.
- `map-list` shows goals and their concepts at `/map`, in an accessible list.
  Each Goal shows its stored "In focus" and "Paused" as text; each Concept the
  number of answers recorded for it. Every star of the constellation is drawn
  alike.
- `map-goal` opens a goal (`/goals/{id}`) with its reference and original request.
- `map-concept` opens a concept (`/concepts/{id}`) with its reference, saved
  questions and answers, observed counts of recalled, helped and missed
  answers, and prerequisite links both ways.
- `map-question` (`/questions/{id}/edit`, the URL the edit form had) shows a
  saved question as stored, read-only.
- The pace (Settings "New items per day"), focus, pause, archive, edit,
  feedback and dispute are retired: their POSTs answer 410 before anything is
  written, and the stored pace, focus and pause stay in export unchanged.

## How to get to it (user POV)

- Menu → Earlier practice → Map → a goal → a concept. Follow displayed links,
  never invented IDs. The concept chip above a held question opens its Concept.

## Driving it with verify-scry helpers

Preconditions:

- A run-owned synthetic notebook holding a stored state loaded with
  `qa/earlier-fixture.mjs` ([study.md](study.md)). The authored demo is retired.

- **Journey.** `.agents/skills/verify-scry/scripts/local-journeys.mjs` →
  `notebook.concept` opens Map, then `a[href^="/goals/"]`, then the first
  `main a[href^="/concepts/"]`, and accepts only `[data-state="concept"]`.
  Evidence: `notebook-concept-390.png`.
- **Story walk.** `qa/walk --stories "US-001 US-006 US-009 US-010"`. US-010
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
  `/settings/judgment` are not affected. The engine functions behind the old
  routes still run when a test calls them directly.
- Do not use historical live stores as fixtures.
- Source: `src/learning.rs`, `src/model.rs`, `src/engine.rs`, `src/web.rs`, `assets/*`.
