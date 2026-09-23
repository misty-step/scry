# Scry design system — Direction A, “Scrying glass”

Adopted for the concept-centered v5 experience (MIS-162, operator authorization
2026-09-23). [Stories](USER_STORIES.md) define learner progress and
[SPEC](SPEC.md#experience-contract) owns behavior. The retired Rust renderer's
Ledger design is historical Git material, not the Go UI contract. This document
specifies the new visual direction; it does not claim a production rollout.

## Composition and visual hierarchy

Dark-first, with an automatic Daylight scheme via `prefers-color-scheme: light`.
Set `color-scheme: dark light` and matching theme-color values. A single centered
40rem column has generous top space and left-aligned reading content. On Stream,
the question dominates; answer controls sit low, full-width, within thumb reach.
The masthead is an italic Fraunces “scry” wordmark, a small ember scrying mark,
and exactly two destinations: `+` Add and Map. Do not turn review into a metric
dashboard, card grid, or five-destination navigation bar.

| CSS role | Dark | Daylight |
| --- | --- | --- |
| `--ground` | `#0B0D10` | `#F4F1EA` |
| `--ground-2` | `#12151A` | `#FBF9F4` |
| `--ground-3` | `#191D24` | `#FFFFFF` |
| `--line` | `#262C36` | `#DDD6C8` |
| `--line-strong` | `#3B4351` | `#B7AE9C` |
| `--ink` | `#EDE7DB` | `#17181C` |
| `--ink-2` | `#B9B2A5` | `#4A4841` |
| `--ink-3` | `#8C867B` | `#66625A` |
| `--ember` | `#FF8A3D` | `#B8440F` |
| `--on-ember` | `#160B04` | `#FFFFFF` |
| `--ember-soft` | `rgba(255,138,61,.14)` | `rgba(184,68,15,.12)` |
| `--correct` | `#86D7AE` | `#1E6B47` |
| `--miss` | `#F3A6A0` | `#A33A32` |
| `--helped` | `#B8B6F2` | `#4B479C` |
| `--pending` | `#9FB7D9` | `#34557F` |

All text/control pairs must meet WCAG AA in each scheme. State is always written
in words, never color alone. A semantic-colored label on a surface must still
meet contrast; do not make incorrect options illegible by reducing opacity.

## Type and assets

Self-host variable **Fraunces** for questions, verdicts, note/concept titles,
and reading text, with optical sizing and `"SOFT" 40, "WONK" 0` settings.
Question: `clamp(1.75rem, 1.15rem + 2.6vw, 2.6rem)` / 1.16, weight about 430.
Reading: 1.125rem / 1.62. Self-host variable **Instrument Sans** for UI,
labels, and metadata at 1rem, with tabular numerals where they align. Use
sentence case; no tracked uppercase labels. Preserve paragraph breaks and wrap
long tokens. Use font fallbacks while assets load. Variable WOFF2 files live in
[`internal/web/assets/fonts/`](internal/web/assets/fonts/) with the actual font
copyright notices and SIL Open Font License in `OFL.txt`; no remote font fetch.
The wordmark, SVG star, and icons are local embedded assets.

## Components and named states

- **Stream:** concept chip above question, with star brightness class `b0..b5`.
  Inert before grading; linked to its Concept page afterward. Render just the
  answer-bearing control for that question. Choice buttons submit on tap; recall
  offers one field and Check, changing to Show me when empty. More holds Show me,
  Look it up (ungraded and safely gated), edit, archive, and correction actions.
- **Question states:** choice; recall; checking; self-check (`close`, `unsure`,
  `failed` with Retry check); result-correct; result-miss-automatic;
  result-overridden; result-shown; result-self. Before grading/self-check do not
  include expected answer, explanation, or evidence in visible or accessible
  markup. Self-check shows answer/explanation and asks the learner to judge.
- **Result:** Fraunces italic verdict (“Correct.”, “Not quite.”, “Shown.”), bold
  answer, readable explanation, modest citation links, a full-width ember Next.
  The automatic miss alone gets a quiet “I was right” beneath Next; automatic
  correct offers “Count as a miss” in More. No automatic transition. Show the
  authority of a grade in detail without making the verdict sound certain when
  the learner chose it.
- **Intro:** a standard note for an unseen concept before its questions; “Got
  it” primary and “I know this already” quiet. The latter is an observation, not
  a passing answer.
- **Preparing receipts:** compact stage/status above the Stream or its empty
  state. Error shows a usable remedy and preserves captured input. First-run
  empty offers Add; caught-up shows a next-time hint and Add; conflict/unknown
  result reconciles the same operation instead of asserting success.
- **Capture:** one input and visible required Topic / My text / Link / Photo
  choice. Suggest a mode on input; never hide or lock the chosen mode. URL and
  share-target text/title prefill are editable. A photo is reduced to at most
  1600px JPEG near quality .82 by browser JS where available; server limits
  still govern. Mic controls appear only when native SpeechRecognition is
  supported; never make them prerequisites for capture or recall.
- **Map:** each goal section includes title, a decorative
  aria-hidden SVG constellation (brightness stars, faint prerequisite lines),
  and an accessible list of concept name, status word, and due hint. Focus/pause
  are deliberate goal actions. Archived goals stay out of active browsing.
- **Concept:** title and status, tally marks (filled unaided, hollow helped,
  slash missed), estimate explicitly labeled “Estimated recall now 88%” rather
  than truth, the current note with citations, related concepts, gated
  question details, Practice this, and quiet archive. Source shows captured
  input, documents, provenance, and recoverable preparation status.

## Interaction and motion

The browser controller in [`internal/web/assets/app.js`](internal/web/assets/app.js)
may toggle recall button copy, submit Enter (Shift+Enter inserts a newline on
non-touch), accept keys 1–6 for choices, and use Space/Enter for Next only when
focus is not in a field. Code may preselect only private modes: pasting
non-link text selects My text and choosing a file selects Photo, unless the
learner has already chosen. Topic and Link send material to web research, so
they are selected by the learner alone, never inferred from length or a URL
shape, and a share-target prefill chooses no mode. With JavaScript disabled, a
required unselected radio makes the learner choose.
HTMX swaps may add/remove motion classes; browser JS owns presentation only,
never durable learning state or offline mutation queues.

All motion must be under `prefers-reduced-motion: no-preference`; reduced-motion
has none. On a user-triggered result, surface once over 200ms with
`cubic-bezier(.2,.7,.2,1)`, opacity 0→1, blur 6px→0, translateY 8px→0.
Choice press scales .985 for 80ms; outgoing card fades/lifts 140ms; concept chip
may glow once for 600ms after a correct answer. No ambient animation, loading
pulse, confetti, swipe grading, or timed advance.

## Copy, access, and trust

Use plain sentences in study. Provenance labels are “From your material”,
“From the web”, and “General knowledge”; a topic word is never cited as evidence.
No provider/money disclaimer in a question flow: spend belongs in Settings
(this week) and failed receipts. No engineering words (job, lease, schema,
token, micros, FSRS, model name) in learner-facing study copy. Distinguish
uncertain checking, known failure, preparation in progress, and actually caught
up; do not call missing work success. Preserve the typed answer after a 422 or
network interruption. Never render untrusted material as HTML.

Use native buttons, links, radio labels, form actions, and focus order; 44px
minimum touch targets (larger for answer/Next). Provide one page heading, a
skip link, a visible focus ring in both schemes and forced colors, readable
semantic labels, and polite atomic result announcements. Respect 320px, 390px,
and desktop; long notes and disclosures flow vertically without clipped text.
Focus must not jump on a pending result; a completed result can receive focus.
CSP disallows inline styles; SVG uses attributes/classes, not `style=`.
Keyboard, touch, and no-JS forms must preserve equivalent core behavior.

## Implementation and validation

[`internal/web/templates/`](internal/web/templates/) owns escaped markup for
Stream, Capture, Map, Concept, Source, History and Settings;
[`internal/web/assets/app.css`](internal/web/assets/app.css) owns both token
sets, type and reduced-motion behavior; `app.js` adds progressive enhancements.
The manifest/icons/fonts live in the same embedded assets tree. Server routes,
CSRF, private caching, and answer-exposure gates are not client-only effects.

Inspect actual pages at 320px, 390px and desktop in both schemes, with actual
choice/recall answers, self-check and correction, long notes, empty/preparing/
error states, keyboard focus, reduced motion, and JS off. Exercise a real
browser on an isolated exe.dev QA VM and inspect screens rather than equating
source assertions with layout proof. Run `bun ~/.local/bin/design-check
internal/web/templates` if that CLI exists; otherwise review template hierarchy,
copy, accessibility, and link targets manually and record the limitation. QA is
not a substitute for operator acceptance or release authorization.

Document structure (direction, tokens, typography/license, surfaces/states,
interaction, accessibility, implementation and verification) was reviewed
manually on 2026-09-23; the `design-md` CLI was unavailable. This does not
stand in for the browser or template design check above.
