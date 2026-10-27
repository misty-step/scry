# One question and honest judgment (concept notebook)

The concept notebook offers one question at a time. The learner answers by
choice or recall, reads a held, honest result, and chooses Next deliberately.
Help and corrections stay visible and immutable. Stories: US-002, US-003,
US-007, US-008, US-011. Intent: [S01/S02](../SPEC.md#s01--open-into-useful-learning),
[S05](../SPEC.md#s05--repair-a-bad-learning-moment) and
[S06](../SPEC.md#s06--survive-interruptions-without-losing-truth).

## Sub-features

- `notebook-empty` shows "Follow your curiosity." in a fresh notebook.
- `notebook-question` presents one question; a choice tap or `#answer` plus Check my answer submits it.
- `notebook-result` holds the result across refresh until Next question.
- `notebook-assist` offers Show me, Look it up, repair or archive from More, behind the assistance gate.
- `notebook-correct` retains historical learner corrections in `/history`; new self-rating writes return 410. Unavailable native grading keeps the answer saved and ungraded.
- `notebook-resume` keeps the concept practice stream at `/notebook`, independent of native Review.

## How to get to it (user POV)

- Open review Home (`/`) and choose Resume earlier practice when native grading is available, or read saved earlier answers in `/notebook`.
  Use Start practicing if an introduction is shown.
- In an empty isolated notebook, the authored-demo action seeds three questions.

## Driving it with verify-scry helpers

Preconditions:

- Built Worker (`worker-build --release --locked`) and CLI (`cargo build --locked --bin scry`).
- Fresh synthetic space; never a live notebook.

- **Journey.** Run `.agents/skills/verify-scry/scripts/local-journeys.mjs`
  (`--width 1280` for desktop). `notebook.empty` sees the empty heading.
  `notebook.concept` chooses Explore an authored demo, taps `.choice`, waits for
  `[data-state="result"]`, reloads and still sees it, chooses Next question,
  then opens Map → goal → concept. `notebook.legacy-link` reopens `/notebook`
  while an exact Study remains at `/recall`. `state.backend-down` stops the run-owned
  Worker and answers. No result may appear, and a failure must be visible.
- **Story walk.** `qa/walk --stories "US-002 US-003 US-007 US-008 US-011"`
  covers each criterion. `qa/browser-recovery.mjs` (inside `qa/walk`) covers no-JS forms,
  keyboard, response loss, offline, competing tabs, access loss and layouts.
- **Proof.** Screenshots `notebook-*-390.png` and `checks.jsonl` under
  the `<stamp>-local-journeys` evidence directory.

## Gotchas

- The authored demo refuses a notebook with existing learning state. Start a
  fresh runtime; do not delete state to reuse one.
- Refresh and Back must not manufacture an answer. Assert the held result after
  reload, not just after the click.
- The fixture is authored material, not provider-quality proof.
- On production, any GET may freeze the owner's next question, so drive this
  flow only locally.
