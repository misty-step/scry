# Concepts, references, and goals

The earlier concept notebook's Goals, Concepts, notes and saved questions stay
readable: the Map shows Goals and connections, a Goal's page its reference and
original request, a Concept's page its note, saved questions with their answers
and its observed counts. Nothing stands before them and reading them writes
nothing. Walk journeys: US-001, US-006, US-009, US-010. Intent:
[SPEC § Earlier concept notebook](../SPEC.md#earlier-concept-notebook).

## Sub-features

- `concept-home` (`/`) is Study; it never shows or selects an earlier question.
- `map-list` shows goals and their concepts at `/map`, in an accessible list.
- `map-goal` opens a goal (`/goals/{id}`) with its reference and original request.
- `map-concept` opens a concept (`/concepts/{id}`) with its reference, saved
  questions and answers, and prerequisite links both ways.
- `map-pace` sets Settings → "New items per day" (3/6/12), with focus and pause
  on the Map. These only ever chose what the earlier notebook served next.

## How to get to it (user POV)

- Menu → Earlier practice → Map → a goal → a concept. Follow displayed links,
  never invented IDs. The concept chip above a held question opens its Concept.

## Driving it with verify-scry helpers

Preconditions:

- A run-owned synthetic notebook: the authored demo, or a stored state loaded
  with `qa/earlier-fixture.mjs` ([study.md](study.md)).

- **Journey.** `.agents/skills/verify-scry/scripts/local-journeys.mjs` →
  `notebook.concept` opens Map, then `a[href^="/goals/"]`, then the first
  `main a[href^="/concepts/"]`, and accepts only `[data-state="concept"]`.
  Evidence: `notebook-concept-390.png`.
- **Story walk.** `qa/walk --stories "US-001 US-006 US-009 US-010"`. US-001 also
  runs the retained-Go migration check described in [QA](../docs/qa/system.md).
- **Proof.** Inspect desktop and phone navigation, not the decorative constellation or a process exit.

## Gotchas

- Opening `/history`, `/goals/*`, `/concepts/*`, `/photos/*` or `/export` is a
  plain read. It marks no exposure and changes no row.
- There is no assistance gate. A held earlier question is not a Prompt awaiting
  an answer and protects nothing; an open Study Prompt still does.
- Archiving a Goal, Concept or question, editing a question and flagging faulty
  material are still accepted writes. Archive, edit and schedule reset drop a
  held unanswered or ungraded earlier question, as they always did.
- Do not use historical live stores as fixtures.
- Source: `src/learning.rs`, `src/model.rs`, `src/engine.rs`, `src/web.rs`, `assets/*`.
