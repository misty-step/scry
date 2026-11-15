# The earlier concept notebook as history (Earlier practice)

`/notebook` shows what the earlier concept notebook holds: the question that
was held, or the answer that was saved, exactly as it was left. It is history,
not practice. Nothing there can be answered, revealed, retried, corrected or
dismissed, and no page read writes to it. Walk journeys: US-002, US-003, US-007,
US-008, US-011. Intent: [SPEC § Earlier concept notebook](../SPEC.md#earlier-concept-notebook),
[Grading](../SPEC.md#grading) and
[Drafts and interruptions](../SPEC.md#drafts-and-interruptions).

## Sub-features

- `notebook-empty` (`data-state="earlier-empty"`) says nothing is held, or that
  nothing is saved. A saved generation request whose preparation stopped still
  leads the page as before (`preparing`, `preparation-paused`, `preparation-stopped`).
- `notebook-unanswered` (`earlier-unanswered`) shows a question that was never
  answered, with the choices that were presented as text and "Not answered."
- `notebook-ungraded` (`earlier-ungraded`) shows a saved answer with no Verdict
  and one fixed line for the stored status of its check (`data-check`): queued,
  sent, failed, unknown, paused, inconclusive, superseded or none.
- `notebook-result` (`earlier-result`) shows a held Verdict with its Authority,
  or "Your Correction", and the answer pair.
- `notebook-refusal`: every `/review/*` action and `/concepts/{id}/practice`
  answers 410 and writes nothing. Self-grading routes keep their own 410.

## How to get to it (user POV)

- Menu → Earlier practice. Map and History are one tap from there; a Concept's
  page shows its saved answers with no gate.
- In an empty isolated notebook, the authored-demo action seeds three questions
  and holds the first one, unanswered.

## Driving it with verify-scry helpers

Preconditions:

- Built Worker (`worker-build --release --locked`) and CLI (`cargo build --locked --bin scry`).
- Fresh synthetic space; never a live notebook.

- **Stored states.** `tests/fixtures/earlier-notebook.json` holds the notebook in
  every state an existing Owner's notebook can be in. `qa/earlier-fixture.mjs`
  loads one state into an unused Durable Object; no HTTP route can create these
  states any more.
- **Journey.** Run `.agents/skills/verify-scry/scripts/local-journeys.mjs`
  (`--width 1280` for desktop). `notebook.empty` sees "Earlier practice" with
  nothing saved. `notebook.concept` chooses Explore an authored demo, sees the
  held question as "Not answered." with no control, reloads and still sees it,
  then opens Map → goal → concept. `notebook.legacy-link` reopens `/notebook`
  while an exact Study remains at `/recall`. `state.backend-down` stops the
  run-owned Worker and answers a Study Prompt. No result may appear, and a
  failure must be visible.
- **Story walk.** `qa/walk --stories "US-002 US-003 US-007 US-008 US-011"`
  covers each criterion. `qa/browser-recovery.mjs` (inside `qa/walk`) covers
  no-JS forms, response loss, offline, competing tabs, access loss and
  background on a Study answer, and the layouts and refusals of Earlier practice.
- **Worker proof.** `node --test qa/earlier-notebook.test.mjs`: nothing is sent
  to Jev, a read writes nothing, the export is the stored notebook, retired
  routes answer 410, and Study beside it leaves every earlier row as stored.
- **Proof.** Screenshots `notebook-*-390.png` and `checks.jsonl` under
  the `<stamp>-local-journeys` evidence directory.

## Gotchas

- The authored demo refuses a notebook with existing learning state. Start a
  fresh runtime; do not delete state to reuse one.
- A GET or HEAD selects nothing, records no exposure, recovers no work and
  moves no alarm. Only the first-ever read of an unused space writes (the empty
  notebook), and a bearer read of `whoami`, tokens or the Library stamps that
  token's last use.
- A check saved as queued or sent stays so for good: the alarm, recovery,
  restore and resuming paused judgment never read it. Its message stays in the
  export; the page shows a fixed label.
- The fixture is authored material, not provider-quality proof.
