# Concepts, references, and goals

Concept Home (`/`) is the primary destination. The Map shows goals and connections. Reference sits
behind a visible assistance gate, and he can focus, pause, resume and pace his
material. Stories: US-001, US-006, US-009, US-010. Intent:
[S07](../SPEC.md#s07--manage-what-i-am-learning-us-001us-009us-010),
[S08](../SPEC.md#s08--return-to-worthwhile-practice-us-009us-011) and
[US-006](../SPEC.md#us-006-understand-a-questions-concept).

## Sub-features

- `concept-home` explains an empty start and links saved concepts or Resume practice without selecting an occurrence.
- `map-list` shows goals and their concepts at `/map`, in an accessible list.
- `map-goal` opens a goal (`/goals/{id}`) with focus, pause, resume and Practice.
- `map-concept` opens a concept (`/concepts/{id}`) with its reference.
- `material-floor` reads restored owner-authored Concepts and independent supplied Material revisions without enabling authoring (SPEC CM1/CM2).
- `map-gate` puts "Look it up" before reference for the current question's concept.
- `map-pace` sets Settings → "How many new ideas in a day?" (3/6/12).

## How to get to it (user POV)

- Masthead Concepts → a concept; View connections → Map → a goal. Follow displayed
  links, never invented IDs. More → Look it up from a question reaches the gate.

## Driving it with verify-scry helpers

Preconditions:

- A run-owned synthetic notebook seeded with the authored demo ([study.md](study.md)).

- **Journey.** `.agents/skills/verify-scry/scripts/local-journeys.mjs` →
  `notebook.concept` opens Map, then `a[href^="/goals/"]`, then the first
  `main a[href^="/concepts/"]`, and accepts `[data-state="concept"]` or
  `[data-state="gate"]`. Evidence: `notebook-concept-390.png`.
- **Story walk.** `qa/walk --stories "US-001 US-006 US-009 US-010"`. US-001 also
  runs the retained-Go migration check described in [QA](../docs/qa/system.md).
- **Proof.** Inspect desktop and phone navigation, not the decorative constellation or a process exit.

- **Compatibility proof.** `tests/materials.rs`, `tests/materials_history.rs` and
  `node --test qa/materials.test.mjs` cover SPEC CM1/CM2: typed synthetic complete
  history, read-only home/detail, compiled CLI, built Worker SQLite/R2, fresh
  restore, first unrelated write, reserved-path refusal before replay, and
  checksum-pinned old-reader refusal. `qa/materials-browser.mjs` retains native
  disclosure and Home/detail/reload pixels across phone/desktop light/dark and
  100%/200% text in `target/materials-browser/`. This does not enable authoring or certify independently retrieved hosted recovery.

## Gotchas

- Opening `/history`, `/goals/*` or `/concepts/*` marks exposure on Goal-backed
  Concepts for future presentations. Reading an owner-authored Concept does not
  fabricate legacy exposure. It is a write, so never browse the owner's concepts on production.
- Do not use historical live stores as fixtures.
- Source: `src/learning.rs`, `src/model.rs`, `src/engine.rs`, `src/web.rs`, `assets/*`.
