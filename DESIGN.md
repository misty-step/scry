# Scry design system

Status: the binding visual and interaction contract for production surfaces in
`crates/memory-engine-api-render`. The operator’s current authorization to
implement and ship the comprehensive PWA redesign supersedes the July 2026
Ledger aesthetic lock. Existing route, wire, storage, environment, and DOM
integration identifiers are not product naming and remain stable.

## Direction: a reading-first study instrument

Scry is for understanding and recalling material the learner cares about. It
should feel like opening a well-set reading page with a few reliable study
controls, not administering a database or checking a score dashboard.

The initial design plan paired a cool reading ground, a single left-aligned
content column, serif study text, and a quiet thumb-reachable dock. Reviewing
that plan against the frontend-design guidance removed the generic dashboard
hero count, repeated rounded-card grid, uppercase labels, monospace metadata,
and decorative motion. The revised emphasis is the material itself: an actual
question, an actual source passage, or the next deliberate study action.

```text
Review                          Add
Scry       Due      Add / More   Scry              Review

The current question            Anything you want to learn
                                A word, phrase, or essay
Answer / I don’t know yet        Learn this

Feedback, then Next question     Generation opens review
```

Left-align content and controls. Center the reading column in the viewport,
not the text within it. Long material gets vertical space, not extra columns.
Structure reflects content: saved Sources, Quiz answer choices, feedback,
and a dedicated Study note are different surfaces, not identical
cards with different labels.

## Product vocabulary

- **Quiz**: a question and answer used in deliberate retrieval practice. An
  easier Quiz is still a Quiz, not an automatically approved remediation pack.
- **Study note**: readable material and source context supporting the same Quiz.
- **Source**: saved material or a topic from which study material is prepared.
- **Concept**: the exact normalized learning concept, not an entire Source.
- **Progress**: the learner-facing navigation label for `/app/analytics`.

Use sentence case and plain action verbs. Do not expose internal card, deck,
workspace, ledger, or provider vocabulary as the primary product model.
Provider/model provenance may appear in a purposeful technical disclosure.
Never promise perfect memory, fabricated recall scores, or automatic mastery.

## Color and type

The stable stylesheet is `assets/ledger.css`, served at `/static/ledger.css`.
The legacy `--lg-*` token prefix is an internal compatibility identifier.

| Role | Light | Dark |
|---|---|---|
| Reading ground (`--lg-paper`) | `#F4F8FA` | `#0F2430` |
| Tidal surface (`--lg-paper-2`) | `#E3EFF3` | `#1C3948` |
| Field (`--lg-field`) | `#FFFFFF` | `#142F3E` |
| Petrol ink (`--lg-ink`) | `#153746` | `#E8F3F6` |
| Secondary ink (`--lg-ink-2`) | `#466574` | `#B3CDD8` |
| Action (`--lg-accent`) | `#256581` | `#91C5DB` |
| Correct (`--lg-pine`) | `#28664E` | `#8FD2B1` |
| Try again (`--lg-clay`) | `#983F49` | `#F1A8B0` |
| Close (`--lg-ochre`) | `#805719` | `#E2C28C` |
| Assisted / Revealed (`--lg-slate`) | `#555B8B` | `#BEC3ED` |

Dark mode follows `prefers-color-scheme` and is deep blue, not warm-black or
acid-neon. Primary, secondary, semantic, and control text must meet WCAG AA in
both modes. Controls have a stronger boundary token than decorative dividers.
State must be conveyed in words, not color alone. Wrong MCQ choices retain
readable contrast; do not lower text opacity to simulate dimming.

**Literata**, variable normal 200–900, is for actual Quiz questions, answer
choices, explanations, captured passages, and Study note reading. Body reading
uses 18px with approximately 1.85 line-height; questions use a responsive
23–29px scale and 1.55 line-height. Keep reading below about 65 characters per
line. Preserve paragraph breaks and wrap long tokens without clipping them.

**Manrope**, variable normal 200–800, is for headings, controls, labels, and
supporting UI. Page headings use a responsive 28–40px scale; the entry headline
may reach 48px. Body UI is 16px; support is 13–15px. Labels are readable sentence
case, never tracked-out uppercase. Counts use tabular numerals only where
alignment helps, not a separate monospace visual system.

Both variable Latin WOFF2 fonts are self-hosted. Fallbacks include Charter,
Iowan Old Style, Georgia, Segoe UI, and platform sans-serif. There are no remote
font requests or runtime font dependencies.

- `/static/fonts/literata-latin-variable.woff2`
- `/static/fonts/manrope-latin-variable.woff2`
- `/static/fonts/OFL.txt` carries the font copyright notices and complete license.
- Source files and their shared SIL Open Font License are in
  `crates/memory-engine-api-render/assets/fonts/`.
- The renderer exports `LITERATA_WOFF2`, `MANROPE_WOFF2`, and `FONT_LICENSE`
  for static serving.

The original Scry monogram uses light lettering on a solid petrol ground,
with its content inside the maskable safe area. Its SVG source and 512px,
192px, 180px Apple touch, and 32px favicon PNGs are in
`crates/memory-engine-api-render/assets/icons/`. Raster bytes are exported as
`PWA_ICON_512`, `PWA_ICON_192`, `APPLE_TOUCH_ICON`, and `FAVICON`; serving paths
and the web manifest stay owned by the application boundary.

## Layout, access, and motion

Standing pages use a maximum 48rem column; Quiz and Study note use 44rem.
Mobile gutters are 16–20px. All flex/grid children can shrink. Source sections
are block-flow content, permission controls take a full line, labels wrap, and
nested disclosures stay within the available width. The Library must reflow
at **320px and 390px**, including long Source titles, open permission/removal
controls, MCQ drafts, and generation failures. `overflow-x: hidden` or `clip`
on the viewport is not a layout fix.

Every interactive target is at least 44px high, including compact controls,
navigation, retries, account actions, and disclosure summaries. Answer choices
are at least 64px high and the deliberate Continue control is at least 56px.
Use native buttons, forms, links, labels, and details/summary semantics. Keep
one page-level heading and a visible keyboard focus ring. The skip link targets
`#me-main`; heading and verdict focus can move programmatically without
requiring a mouse. Announcements are polite and atomic where the changed
result needs to be read together.

Review is the default destination and has no standing-view dock, competing
analytics, or draft triage. The question takes the available viewport and
answer controls sit within thumb reach. Add and More remain available in the
header. Secondary pages use a quiet Review/Add dock.
Disclosures expand in the document instead of a clipped floating action grid.
The account menu is bounded to the viewport.

There is no ambient animation, loading pulse, celebratory drawing, or timed
advance. Pending feedback is immediate and still. Small user-triggered color
transitions may last 120ms. `prefers-reduced-motion` removes animation and
transitions. Forced-color mode retains selected and accepted-answer outlines.

## Surface contract

| Surface | Primary job and required next action |
|---|---|
| Signed out | Explain Source → Quiz / Study note → practice; request a magic link with an explicit invite/waitlist explanation. |
| Request received | Explain the invited-email or waitlist outcome without disclosing account eligibility; use the newest link or return to start. |
| Recovery | Render the real error safely; provide the correct retry/link action rather than a dead end. Never print a secret. |
| Home, due | Open the actual next question; preserve an existing graded result until Next. |
| Home, new | Show the capture field directly. No Start review or Create navigation gate. |
| Home, caught up | State nothing is due and let the learner add more or leave. |
| Create | One field accepts a word, phrase, or essay; Learn this saves and generates. Model use is explicit. |
| Capture waiting | Follow one durable job; keep failed input recoverable and show an authorized retry. Only successful completion navigates to review. |
| Library | Secondary Source management, published inventory, permission/removal, and generation activity. No approval inbox. |
| Quiz management | Optional edits/removal preserve identity, attempts, and schedules; provenance remains available. |
| Quiz | Actual question and one-tap choices or a labelled response field. “I don’t know yet” saves assisted practice in one intent. |
| Graded Quiz | Hold the canonical verdict, accepted answer, concise feedback, and Next question. Details stays collapsed. |
| Study note | A dedicated reading surface for `current.reference_text`, escaped and preserving line breaks, returning to the same Quiz. State absence honestly. |
| Progress | Filterable, bounded Concept evidence list with real recall history and pagination. Distinguish untried from struggling; no invented score. |
| Account / reminders | Browser sign-out scope is explicit; service sessions remain separate. Reminder actions stay native protected forms behind a Home disclosure. |

## Learning-loop invariants

- **One tap answers an MCQ.** The exact choice value submits; no separate confirm
  and no letter-guessing interface.
- Free response is type, then submit. Browser timing starts from actual
  presentation; a missing time stays missing rather than becoming a fake fast
  recall measurement.
- A revealed answer is assisted practice for that occurrence. Submitting after
  reveal must produce `Revealed` with the conservative scheduling result; do
  not fabricate exposure for historical attempts.
- The four visible verdict literals remain **Correct**, **Close**, **Try again**,
  and **Revealed**. The graded page holds indefinitely. **No auto-advance.**
- Only deliberate Continue advances the review. Quiz-quality feedback saves in
  place, with its stable idempotency and superseding identifiers preserved.
- The verdict and accepted answer stay visible. Details contains the original
  choice recap, schedule horizon, Concept progress, recall history, the Study
  note entry, and Quiz-quality controls. No dossier is shown before grading.
- More offers Study note, Skip, Snooze quiz, exact Concept snooze, manual Bridge,
  Edit, Create, and confirmed Delete. Each touch-visible description tells the
  truth about scope: Skip is later in this session; Snooze is until tomorrow;
  Concept snooze is the exact Concept only; Source removal affects every Quiz
  generated from that Source.
- Bridge generates genuinely easier quizzes through the same quality gates and
  automatic publication path. No learner admission ceremony is required.
- Study notes reuse durable source-backed material. Never invent quotes or
  render untrusted material as HTML. `render_reference_page(account, view)`
  consumes resolved data, not a loader. Its return form is POST `/app/resume`
  with `csrfToken` and the same `reviewUnitId`; it does not call Continue.
  Preserve model-expanded/source-informed provenance labels and critique text
  supplied in the note. A captured topic seed is not evidence for generated
  claims. Keep span labels visible and use neutral Source context headings,
  never label all generated input as verified evidence.

## Integration and verification

Preserve `.ae-view`, `footer.ae-bar`, `.me-due`, `.me-verdict`, `form.me-next`,
answer-form actions, CSRF fields, response-time fields, and idempotency fields.
Fresh answer/result views include an empty `[data-review-status]` live region.
`.me-verdict` is programmatically focusable. Generation retains `#me-jobs`,
`.me-job`, `data-job-id`, `data-status`, and the title/meta/retry hooks.
`render_capture_waiting_page(account, job)` adds one
`[data-generation-job-id][data-terminal-url="/app/library"]` container and a
`[data-generation-status]` live region. Only a terminal event for that exact
job may navigate automatically. Library has editable draft/source forms and
must not receive this terminal-navigation hook. The waiting renderer does not
load account state or invent a due count when the job provides none.

Home and Quiz render from an already resolved study view; they do not load the
Source catalog. A live generation notice may consult jobs to avoid stale
status. Do not introduce loaders for display-only concerns.

Behavior/security/accessibility tests protect the form and study invariants,
not incidental wording, exact CSS token strings, or the old aesthetic lock.
Visual acceptance is the actual PWA at 320px, 390px, and desktop in both color
schemes, including open disclosures, long content, focus, pending/error
feedback, grading, and same-Quiz Study note return. Check with JavaScript both
available and unavailable; normal forms must remain usable. Rendering a
preview or passing a source-string assertion is not proof of mobile reflow.
