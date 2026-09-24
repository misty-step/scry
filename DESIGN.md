# Scry design system: Ink notebook

Adopted on 2026-09-24 after the operator judged the earlier "Scrying glass"
interface atrocious (MIS-162). The audit, the six concepts, the three clickable
finalists, and the decision are in
[docs/design/redesign-2026-09-24.md](docs/design/redesign-2026-09-24.md).
[Stories](USER_STORIES.md) define learner progress and
[SPEC](SPEC.md#experience-contract) owns behavior. This document specifies how
the interface looks and moves. It does not claim a production rollout.

## Direction

Scry is a study notebook. By day it is blue-black ink on cool paper, and by
night paper-colored text on blue-black ink. The palette has fixed roles:

- **Cobalt:** the learner's own action, such as Next, Check, and Add to Scry.
- **Green tick:** recall.
- **Red pencil:** a miss.
- **Violet:** a shown answer.
- **Gold:** concept strength.

Each goal's **star chart** on the Map is the one bold element. It is always a
small window of night sky, in either scheme. Everything else stays quiet.

The frame is a masthead with the wordmark and exactly two destinations, Add
and Map. History and Settings sit in the footer. Reading content sits in a
single column no wider than 40rem.

On a phone, the Stream places the question at the top and the answer controls
in the bottom third, within thumb reach. On a result, Next and "I was right"
stick to the bottom edge. On screens 48rem and wider, controls follow the
question directly and nothing is pinned. Never turn review into a dashboard,
a card grid, or a tab bar.

## Tokens

Both schemes are defined in [`app.css`](internal/web/assets/app.css). Dark
values apply under `prefers-color-scheme: dark`.

| Role | Day | Night | Use |
| --- | --- | --- | --- |
| `--paper` | `#F5F6F8` | `#0E1220` | Page ground |
| `--sheet` | `#FFFFFF` | `#161B2C` | Raised surface: choices, pairs, panels |
| `--sunk` | `#ECEEF3` | `#1B2135` | Quiet fill: receipts, icon wells |
| `--rule` / `--rule-2` | `#DDE1E9` / `#B9C0CE` | `#262D44` / `#3B4462` | Hairlines / control borders |
| `--ink` / `--ink-2` / `--ink-3` | `#131A2C` / `#434C63` / `#5C657D` | `#E9EBF3` / `#B1B7CB` / `#8E95AC` | Text ranks |
| `--accent` | `#2B44D6` | `#93A4FF` | Primary action fill, focus ring |
| `--accent-ink` | `#2338B5` | `#A9B6FF` | Links, current destination |
| `--correct` | `#13693F` | `#6DD39C` | Recall |
| `--miss` | `#AE251D` | `#FF8F87` | Miss, destructive |
| `--helped` | `#5A45AE` | `#C3B3FF` | Shown, helped practice |
| `--star` | `#B8861B` | `#F2C45B` | Strength dots |
| `--sky` | `#121A33` | `#0A0F1F` | Star-chart ground |

Each color has a `*-soft` fill for seals and notices. Radii are 10, 14, and
18 px: small, controls, and panels. Pills are fully round and are used only for
chips, badges, and small actions. Text and control pairs meet WCAG AA in both
schemes. State is always written in words; color only reinforces it.

## Type

- **Literata** (variable, with optical size and weight axes): questions,
  headings, notes, explanations, and typed answers.
- **Atkinson Hyperlegible Next** (variable weight): interface text, labels,
  and controls. It was chosen for letterforms that stay distinct at small
  sizes.

Both are self-hosted Latin-subset WOFF2 files in
[`internal/web/assets/fonts/`](internal/web/assets/fonts/). `OFL.txt` records
their copyright and the SIL Open Font License. No remote font is fetched.

| Style | Setting |
| --- | --- |
| Question | Literata 470, `clamp(1.625rem, 1.2rem + 2vw, 2.25rem)` / 1.2 on phones, up to 2.5rem on desktop |
| Page title | Literata 520, 2rem / 1.15 |
| Section title | Literata 560, 1.25rem |
| Verdict | Literata italic 600, 1.75rem |
| Reading | Literata, 1.0625rem / 1.65, at most 36rem |
| Interface | Atkinson, 1.0625rem body; 0.9375rem hints; 0.8125rem pair labels |

Use sentence case with no tracked capitals. Headings balance their lines, and
long unbroken strings such as URLs wrap anywhere.

## Components and named states

- **Masthead:** a lens wordmark (a magnifier with a gold star) and "scry" in
  Literata italic. It links to Add and Map. The current destination gets an
  accent-soft pill. The app icon uses the same lens: paper color and gold on
  the sky tile. It was checked at 16 px on white, dark, and grey browser tabs.
- **Stream receipts:**
  - Each preparation in progress is one compact row showing its title, stage,
    and ready counts, marked with a static accent dot.
  - Stopped preparations collapse into a single red-pencil disclosure, such as
    "2 things you added couldn't be prepared", which lists each one's link.
  - A receipt never pushes the question below the fold.
- **Question:**
  - An optional concept chip sits above the question, showing strength dots
    and the concept name. It is inert before grading and links to the Concept
    page after.
  - The question itself is the page's h1.
  - Choices are full-width sheets with a quiet key label (1 to 4) that matches
    the keyboard shortcut. A tap submits the choice. A pressed choice shows
    cobalt.
  - Recall is one serif textarea with an inset mic button (shown only when
    speech recognition is available), a hint, and Check. Check reads "Show me"
    while the field is empty.
- **More:** a centered "More" disclosure opens a single list with rows in this
  order: Show me, Look it up, Count as a miss (after an automatic correct),
  Fix this question, Edit, and a red "Archive this question" last.
- **Result:** a seal and verdict: a green tick with "Correct.", a red cross
  with "Not quite.", or a violet eye with "Shown.". The authority line names
  who decided (Scry matched, Scry checked the meaning, you checked it, or you
  changed it). A two-row pair follows: "You answered" (struck through in red
  pencil on a miss) above "Answer". Then the explanation in reading type,
  citation chips, Next, and on an automatic miss only, a quiet "I was right"
  beneath Next. Nothing advances on its own.
- **Self-check** uses a neutral balance seal, the reason in plain words, the
  same pair, and a split "I was right" / "Not quite". A failed check adds a
  quiet "Retry check". **Checking** shows the saved answer and says it can be
  refreshed.
- **Intro:** a "A new idea" kicker with a gold star, the concept name, its
  summary, and the note as a cobalt-ruled reading column with its provenance.
  "Got it" is primary and sticky on phones; "I know this already" is quiet.
- **Empty states:** a round mark, a heading, one sentence, and actions.
  - First run: "What are you curious about?" with Add something.
  - Caught up: "You're caught up." with when the next question is due, Add
    something, Open your map, and Check again.
  - Gate: "Look it up?" explains that opening the page counts the question as
    shown.
  - Errors and not-found use the same shape.
- **Add:** the page asks "What do you want to learn?". There are four mode
  cards, and each card names its privacy consequence:
  - Topic: Scry searches the web.
  - My text: private, never searched.
  - Link: Scry reads that one page.
  - Photo: Scry reads the words in it.

  A serif textarea with an inset mic follows. The photo field appears only
  while Photo is chosen (via `:has`; without it the field stays visible).
  Add to Scry is last.
- **Map:** each goal is a section with:
  - its title,
  - In focus and Paused badges and its counts,
  - the star chart,
  - the concept list: strength dots, name, and status word,
  - pill actions (Focus on this or Remove focus, Pause or Resume) and
    Original input.

  Captures that stopped before any idea was mapped are listed together under
  "Couldn't prepare". Earlier unplaced material has its own list.
- **Star chart:** decorative (`aria-hidden`) and laid out on the server by
  prerequisite depth, so prerequisites sit left of the ideas that need them.
  Star brightness and radius follow concept strength, and the two strongest
  levels get a soft halo. Labels are clipped to fit their column. With a
  single row, labels alternate above and below the stars.
- **Concept:**
  - the strength and status kicker, the name, and the summary;
  - a practice panel with the tally and the counts. Tally marks: filled green
    for recalled, violet outline for with help, red slash for missed.
  - "Estimated recall now: N%. This is an estimate, not a certainty.";
  - Practice this;
  - the note with its provenance, citations, and "What this draws on";
  - connected ideas as strength chips;
  - questions, each with a disclosed answer and Edit;
  - More questions;
  - a quiet archive.
- **Source:**
  - its mode kicker, excerpt title, and saved time;
  - a preparation panel. Each step shows a check, alert, or dot, its status,
    and a learner-language summary. Internal check identifiers are
    parenthesized in stored errors; the summary removes those asides and keeps
    the recorded text under Details.
  - the photo, the original text, what Scry read, and questions.
- **Edit:** fields grouped as Question, Answers, and Explanation. Draft,
  drafting, and stopped-fix notices sit above the form. Fix and archive are
  quiet disclosures.
- **History:** the question leads each entry, followed by a meta line with a
  verdict tag, the answer style ("Pick an answer" or "From memory"), the time,
  and the authority. "See this attempt" reuses the result pair.
- **Settings:** the pace as three radio cards with their daily counts, the
  record as number tiles, the week's spend, backup attention when stale, and
  the font and license notes.
- **Request state:** an inverted toast pinned under the masthead, so it never
  covers the bottom answer zone. It carries retry and recovery actions.

## Interaction and motion

[`app.js`](internal/web/assets/app.js) is presentation only. It does these
things and nothing more:

- toggles the recall button's copy;
- submits on Enter (Shift+Enter inserts a newline on fine pointers);
- maps keys 1 to 6 to choices, and Space or Enter to Next when focus is not in
  a field;
- reduces photos to at most 1600 px as JPEG at quality 0.82;
- preselects only the private modes (My text when non-link text is pasted,
  Photo when a file is chosen).

Topic and Link are chosen by the learner alone. The browser never owns
durable learning state or an offline queue.

All motion sits under `prefers-reduced-motion: no-preference`, so reduced
motion has none, and every motion answers a learner action:

- A result or new stage surfaces over 220ms (opacity and an 8px rise,
  `cubic-bezier(.2,.7,.2,1)`).
- The outgoing stage lifts over 140ms.
- A pressed button scales to 0.985.
- The More menu surfaces over 160ms.
- The chip's strength dots glow once after a correct answer.

There is no ambient animation, loading pulse, confetti, swipe grading, or
timed advance.

## Copy, access, and trust

- **Study copy:** plain sentences. Provenance reads "From your material", "From
  the web", or "General knowledge". Interface copy says "idea"; stories and
  code say "concept".
- **No engineering words** in learner copy: job, lease, schema, token, micros,
  FSRS, check identifiers, or model names.
- **Spend** appears in Settings and in a stopped step's Details, never in the
  question flow.
- **No hidden answers:** answers, explanations, and evidence are absent from
  visible and accessible markup before grading or self-check.
- **Untrusted text** is never rendered as HTML.

Accessibility:

- Use native buttons, links, radios, and disclosures. Touch targets are at
  least 44 px; choices and Next are larger.
- Each page has one h1 and a skip link (hidden until focused). The focus ring
  is visible in both schemes and in forced colors.
- The tally uses an ordered list with visually hidden words. Result
  announcements are polite and atomic.
- The CSP disallows inline styles, so SVG uses attributes and classes.
- Layouts hold at 320, 390, and desktop widths. Keyboard, touch, and no-JS
  forms keep equivalent core behavior.

## Implementation and validation

- [`internal/web/templates/`](internal/web/templates/) owns escaped markup, and
  `icons.html` owns the inline icon set.
- `goalChart` in [`concepts.go`](internal/web/concepts.go) owns the chart
  layout.
- `app.css` owns tokens, type, layout, and motion.

Several hooks are relied on by other code and must be kept:

- `form.answer-form`
- `fieldset.choice-fieldset` with `.choice` buttons
- `form[data-next]`
- `details.overflow`
- the `role=status` result
- `section.empty-stage`
- `data-state`
- `class="feedback feedback-correct"`
- `svg.constellation`

The browser critic and the web tests read them.

To verify, inspect real pages on an isolated exe.dev VM against the synthetic
fixture:

- **Widths and schemes:** 390 px and 1280 px in both schemes, as full pages
  and phone first screens.
- **Answer paths:** choice and recall answers, a miss followed by an override,
  and a reveal.
- **Self-check:** the unsure and failed cases.
- **Other Stream states:** intro, receipts (in progress and stopped), caught
  up, first run, and the gate.
- **Pages:** Map with focus and pause, Concept, Source for each mode, Edit
  with a fix draft, History, Dispute, Settings, and not-found.
- **Transient states:** the offline toast.

Then run:

- `bun ~/.local/bin/design-check internal/web/templates`
- axe-core on the captured states
- `go test ./internal/web`

A screenshot proves appearance only, not operator acceptance.
