# Scry: design

Scry is for someone who wants what matters to stay in their head, for about five minutes a day. It should feel like a precise, warm instrument: closer to Warp and Supermemory than to a book or a game. [PRODUCT.md](PRODUCT.md) owns what Scry does. This file owns how it looks and feels.

Accepted by Phaedrus on 2026-11-17 (chat with Draper, shaped under [MIS-1210](https://linear.app/misty-step/issue/MIS-1210)), and refined with Scry's custom brand mark and craft pass on 2026-11-23. The tokens and rules below are what bind.

## Principles

- **Words first.** Every state is said in words. Color, highlight and strike back the words up and never carry the meaning alone.
- **One thing at a time.** Each screen has one job and one main action.
- **Quiet everywhere, one hard moment.** Pages are calm and exact. The moment a miss is judged is the one place the type gets loud.
- **Keyboard first, touch equal.** Navigation and study actions have dedicated keys. Everything else is reachable by Tab and Enter. Every action has a 44 px touch target.
- **Simple is not plain.** Calm does not mean bare: every destination says what it is for in one plain line, and small line icons (see Icons) help the eye find Study, Create and each starter set. Icons sit beside words and never replace them.
- **Never:** streaks, confetti, mascots, achievement badges, points, glows, violet gradients, gradient text, shadows under cards, filled or colored icon tiles, emoji as interface, stock imagery, eyebrow labels over every heading, or the Inter typeface. Copy is concrete: no "unlock", "elevate", "seamless" or "effortless".

## Type

| Role | Face | Use |
| --- | --- | --- |
| Text | Geist | Prompts, answers, reading, titles, buttons |
| Mono | Geist Mono | Labels, counts, keys, sources, due dates, verdict tags, the wordmark |

Both are SIL Open Font License faces, served from Scry itself. No third face. Hierarchy comes from size and weight.

| Step | Size / line height | Weight | Use |
| --- | --- | --- | --- |
| Judgment | 96–160 px / 1.0 (fluid; 72 px on phones) | 600 | The struck answer and the right answer on a miss |
| Display | 40–48 px / 1.1 (32 px on phones) | 500 | The current prompt, page titles |
| Title | 24 px / 1.25 | 500 | Idea titles, set titles |
| Answer | 22 px / 1.5 (18 px on phones) | 400 | The learner's answer as it is typed or dictated in Study |
| Body | 16–17 px / 1.55 | 400 | Reading, rows, the one-line why |
| Meta | 13 px / 1.4, mono | 400 | Labels, counts, sources, keys |
| Micro | 11 px / 1.4, mono, +0.04em | 400 | Keyboard hints only |

Answers the learner types or dictates are set in Text at Answer size in a multiline field (see Study session: the answer field), large enough to carry weight under the prompt and small enough to write several sentences or a paragraph in comfortably; the review then sets them at Title or Judgment size. Mono labels are sentence case or short uppercase tags; never long uppercase sentences.

## Icons

One family of inline SVG line icons, drawn on a 24 px grid: `stroke="currentColor"`, stroke width 1.75, round caps and joins, no fill, `aria-hidden="true"`. They are served inline in the page (no icon font, no sprite request, no emoji) and rendered at 16 px in the navigation and inline labels, 18 px in buttons and verdicts, and 20 px on starter tiles. An icon takes the color of the text it sits beside and always has that text next to it, so it never carries meaning alone.

- **Navigation:** every destination has one: Study (a stack of cards), Library (books on a shelf), Create (a sparkle), Billing (a card), Settings (sliders).
- **Starter tiles:** each set has an icon for its subject in ink-muted beside its title (a radio mast for the NATO phonetic alphabet, layers for mental models, a ruler for kitchen numbers, a globe for geography, a beaker for chemistry, an atom for physics, a scroll for the Creeds, and so on). A tile stays a typographic hairline tile; the icon is a glyph on the title line, not a picture or a filled badge.
- **Counts:** an item count is a compact card-stack icon and the number in tabular figures ("26 cards").
- **Actions and verdicts:** Add (plus), Added (check), Start studying and Next card (arrow), Ask Why? (question mark), Edit card (pencil), Snooze (a crescent moon), Delete (a bin); Correct (check in a circle), Almost Correct (a tilde in a circle), Incorrect (a cross in a circle).
- **Bridge cards:** Repeated miss and Build bridge cards (a bridge: a deck on two piers over an arch), Held by N bridges (a pause mark in a circle), Release hold (a play mark in a circle), Remove a drafted bridge (a cross). 16 px beside their words, in the words' color.
- **Schedule facts:** Next review (calendar), Interval (a span between two stops), Stability (an anchor), Difficulty (a gauge), Reviews (a clock with a turning arrow). Rendered at 14 px in ink-muted beside their mono label.
- **How Scry works:** the landing page's four rows each have a 20 px icon beside the Title: a clock (the five-minute daily loop), a check in a circle (judged on understanding, and exact texts), layered books (starter sets, Markdown capture, imports), a lock (private, exports, API tokens).

## Color

Ink steps are the base ink at fixed opacity, so both modes keep the same hierarchy.

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| page | `#FFFFFF` | `#12101A` | Page ground |
| raised | `#FFFFFF` | `#1A1522` | Panels, hovered rows |
| sunken | `#F4F4F6` | `#241E2F` | Fields, wells, the shelf hover |
| ink | `#0B1015` | `#FFFFFF` | Primary text |
| ink-body | ink at 75% | ink at 75% | Reading text |
| ink-muted | ink at 60% | ink at 60% | Labels, sources, receded items, upcoming items |
| ink-ghost | ink at 20% | ink at 22% | Control hairlines, disabled, the dot grid; never visible text |
| line | ink at 12% | ink at 16% | Hairlines and rules |
| accent | `#0562EF` | `#4D8DFF` | The one main action, anything due, concept links, focus ring |
| right | `#1E7F4E` | `#5FD39A` | The "right" tag |
| close | `#EEF17C` fill, ink text | `#EEF17C` at 30% fill, ink text | Highlighter mark behind missing words in an exact text, and matched words in search |
| missed | `#C23B26` | `#FF7A66` | The strike through a wrong answer and the "missed" tag |

The accent appears at most once as a filled control per screen. Controls come in three weights: the one filled accent action, bordered secondary buttons (ink text, a ghost hairline that turns ink on hover), and quiet text links. Text inputs carry a 1 px `line` hairline border at rest (turning `ink` on focus) so field boundaries remain legible in both modes. Hover and focus are crisp, never soft: a fill or border change in 150 ms and a 2 px accent focus ring; a pressed button moves down 1 px. The focus ring is for the keyboard only (`:focus-visible` after Tab or arrow navigation): a control Scry focuses by script after a tap, a click or typing in a field ("Next card" when a review opens) shows no ring, and no control ever wears two rings at once (a ring around an accent fill, or a ring beside a focus border). Without script the browser's own `:focus-visible` rule applies. Hover fills apply only on hover-capable pointers (`@media (hover: hover)`) so tapped controls never stick highlighted on touch devices. Mode follows the system setting, with a toggle in Settings. Contrast: every visible text, the upcoming items included, meets 4.5:1 in both modes; ghost opacity is never used for visible text.

## Shape, space and texture

- Surfaces are square (radius 0). Controls and fields are 4 px. Nothing is a pill.
- 1 px hairlines separate things; no shadows, no gradients, no blur.
- Spacing steps: 4, 8, 16, 24, 32, 48, 64, 96 px. Section padding is 56 px on desktop, 40 px on phones.
- Reading pages have a measure of at most 68 characters. Draft and study scrollback rows are exempt. The page frame is at most 1120 px.
- Only the study session and the landing hero have texture: a faint 22 px dot grid (ink-ghost at 6–8%) with calibrated ruler ticks along the left and right edges; the hero adds a `sunken` drafting band in a 1 px `line` frame. Never a gradient, glow or shadow; text on it meets 4.5:1 in both modes.

## Motion

- Fades and short moves of 150–250 ms, ease-out. Nothing waits on an animation, and nothing loops except the landing Study Specimen (the one ambient demonstration; see Landing) and the two processing marks shown while a card or billing form is being confirmed (a 14 px spinner and a 2 px progress line; see "Card and billing forms: processing").
- Two drawn lines: the strike through a missed answer draws across once (200 ms), and an idea's link lines draw in once when its page opens.
- Under reduced motion, everything appears in its final state (the Specimen rests on Judged).

## Brand mark and wordmark

Scry pairs its lowercase Geist Mono wordmark (`scry`, ink) with a dedicated custom geometric **scrying lens** mark drawn on the same 24 px stroke grid (`stroke-width: 1.75`, round caps and joins): four crisp corner viewfinder brackets framing a centered circular lens ring in ink, with a single solid focal point (`r="2.25"`) in `accent` at the exact center. The mark sits 20 px square beside `scry` (8 px gap) in the top bar and serves as Scry's standalone app icon (adaptive light/dark vector mark and crisp dark-ground `#12101A` icon with white aperture and `#4D8DFF` focal point for home-screen launch and social link previews).

## Key screens and states

### Landing

A two-column desktop hero (`minmax(0, 1fr) minmax(0, 1.08fr)`, 48 px gap; one column at 760 px and below) on the drafting band:

- **Left:** Headline ("Keep what's worth knowing."), one plain lede naming active recall and spaced repetition, one filled accent action ("Start the 7-day trial") beside a quiet secondary "Sign in", and the trial terms line ("7 days free · no card needed · then $5 a month if you keep it", keeping "$5 a month" unbroken).
- **Right:** the **Study Specimen**: `raised` (`page` in light), 1 px `line` border, radius 0, padding 24–28 px (16–20 px on phones); pure type, hairlines and line icons. Mono step controls (`1 · Prompt`, `2 · Answer`, `3 · Judged`; 44 px tall, reached by Tab, current one underlined) pick a stage: *Prompt* (place "Study · Mental models · Set 1", "4 / 11", the prompt "Hanlon's razor", the answer field); *Answer* (a typed answer settles in the field); *Judged* ("Your answer" beside "Correct answer", the "Correct" badge with its 16 px icon, the one-line why, the schedule). The server renders Judged, so without script and under reduced motion it is complete and still. With script it moves on every 3.5–4.5 s (150–250 ms ease-out cross-fade), pausing on hover, focus or a chosen step. Stages share one fixed-height grid cell, so nothing below shifts.
- **Specimen schedule:** Next review "in 3 days", Interval "3 days", Stability "3.4 days", Difficulty "4.9 / 10" in a flush 2×2 grid (`repeat(2, minmax(0, 1fr))` at every width, no gap) of cells sharing 1 px `line` rules, padded 12–16 px on every side. Each cell: its 14 px icon and Meta label on one line (`white-space: nowrap`) over the value in ink, tabular figures, at least 12 px clear of every rule and border at 1440×900 and 390×844 in both modes. The post-answer review keeps its own table.

Below the hero, four ruled rows (each with its icon) explain the daily ritual, AI judgment of understanding alongside word-for-word exact texts, the 30 starter sets, Markdown capture and deck imports, and private ownership (naming external scripts or tools rather than unexplained "agents"). Public pages end with a quiet hairline footer naming Misty Step and Scry's plain privacy commitment.

### Sign-up and trial

One centered column. Email and password, and the 7-day trial starts at once, with no card: the newcomer lands on the starter shelf (Study once the library has items). Both Clerk and local sign-up pages keep `"7-day trial · no card needed"` visible above the heading, and the landing page says `"7 days free · no card needed · then $5 a month if you keep it"`. Sign-in clarifies that `"Forgot password"` appears on the password step after entering an email. Embedded Clerk auth panels inherit Scry's light and dark tokens (`page` / `raised` surface, 1 px `line` border, 0 px card radius, 4 px controls, Geist face, 16 px inputs, zero drop shadows or gradients) so dark mode never flashes a bright white card. States: empty, typing, error in words under the field (for example `"That email already has an account."`).

During a no-card trial the top bar carries one quiet line after the wordmark, in micro mono text inside a hairline frame, never a banner or a color alarm: "Trial · 6 days left · Upgrade" ("last day" on the final day; "Trial ended · Add a card" after it), where only "Upgrade" (or "Add a card") is accent-colored, and the whole line links to the card step. It is not shown on bare pages (sign-up, sign-in, the card step) or once a card is on file.

The card step (/signup/card, or Billing's "Add a card") says in plain words, inside the trial: "Nothing is charged today: your trial still ends on <date>, when your card is first charged $5. Cancel any time before then in Billing." with the button "Add card · nothing charged today"; after the trial (or with under an hour left): "Your 7-day trial ended on <date>. Add a card to keep studying: $5 today, and then monthly." with "Start · $5 a month". States: card declined with the reason the processor gives, and the trial carries on unchanged.

### Starter shelf

Shown when the library is empty and reachable later from the library. Title "Pick a deck and start recalling in seconds." with one mono line under it ("30 starter sets · add one or more to begin"). Below, a hairline grid of typographic tiles: no pictures, no filled cards. Each tile holds the set's subject icon and title (Title) on one line, a mono item count with the card-stack icon ("26 cards"), one line of description and a bordered "+ Add" action.

Order: the first row is broad and instantly usable (NATO phonetic alphabet, Mental models, Kitchen numbers, World geography), then everyday knowledge and the core ideas of each subject. The Creeds and Passages worth keeping (word-for-word texts) close the shelf; they are never in the first row.

States: default; hover (sunken fill on hover-capable pointers); adding (the button reads "Adding…" and is disabled); added (accent hairline outline and the accent tag "Added" with a check). Adding is confirmed in place: the page does not reload and the scroll position does not move. After the first add a bar pinned to the bottom of the window appears (with bottom safe-area padding on phones), without scrolling the page: the count ("2 sets added · 50 cards") and the screen's one filled accent action, "Start studying". Without JavaScript the same form posts and the page returns to the tile that was added.

### Study session

The center of Scry. The session reads like scrollback:

- Answered items recede above the current one in ink-muted, one line each, with a mono verdict tag at the right edge ("correct", "almost correct", "incorrect").
- The current prompt sits in the middle at Display size, with the answer field under it (see below).
- Upcoming items sit below at Meta size in ink-muted, at most three (only while a card waits; the post-answer review hides both the answered rows and the upcoming list): secondary by their size, and still readable (4.5:1 or better in both modes; never ghost opacity).
- The left margin (`196 px` on desktop, aligned cleanly to the top of the active prompt/review flow rather than floating mid-height) says where the learner is, in mono: the Study icon and name with the purpose line "Recall what's due today", then the spoiler-safe place and the count ("4 / 11", counted across every deck in the session). The waiting screen fits comfortably inside a standard desktop viewport (`1440×900`) without unintended vertical scroll. The bottom right holds the key hints, with the answer instructions associated with the field for assistive technology: "Enter to check · Shift+Enter for new line · Space to start/stop speaking (empty answer)" on desktop, "Return to check" on phones (see Phones).

**Interleaved session ([S111-1]).** Study has no deck picker: `/study` and the Study navigation open straight onto the first card of one session across every active deck (each top-level idea or starter set). Session capacity is the five-minute budget: 20 s per card and 30 s per exact-text line, so at most 15 cards (fewer with exact lines), never fewer than one, chosen from at most 200 due candidates. Due reviews get the session slots first, by lowest FSRS retrievability then earliest due, so 15 or more ordinary due cards fill the session with no new card; new cards fill only the room left, at most 10 a day across both kinds of session. From that pool, alternate decks whenever possible, preferring due reviews to new cards from another deck; repeat the last deck only if there is no eligible alternative. The lines of an exact text keep their order. The deck tag is the card's spoiler-safe place line in the rail (on phones, above the prompt), in Meta mono ink-muted: it changes with each card, quietly, with no color per deck, no badge and no transition beyond the card's own. A focused session started from Library's "Study this idea" adds one Meta line under the count, "Focused · <topic>" (spoiler-safe like the place line), with a quiet text link "Study all decks" back to `/study`; the interleaved session never shows such a line.

**The answer field ([S111-2]).** One auto-expanding multiline `<textarea>` (`name="answer"`, `rows="3"` in the markup and set to one row by script, labelled "Your answer" for screen readers), never a one-line `<input>`. It is set in Text at Answer size (22 px, 18 px on phones; never under 16 px, so phones do not zoom) with a line height of 1.5, at a reading measure: full column width up to `max-width: 64ch` (within 60–68ch). It sits in a subtle editorial frame: a 1 px `line` hairline border at the 4 px control radius on the `page` ground, padded 12 px 16 px, turning `ink` on focus (no ring beside the border; see Color), with the native caret in `accent` (`caret-color`) instead of a drawn caret. Words wrap naturally (`white-space: pre-wrap`, `overflow-wrap: anywhere`, so a long unbroken string or URL breaks rather than widening the field); `overflow-x: hidden` and `resize: none`, so nothing ever scrolls or clips sideways at 1440×900 or 390×844 in either mode. It grows row by row: on every input, paste, dictated insert, width change (resize, rotation) and page restore, script sets its height from its content (`height: auto`, then `scrollHeight` plus the border), with no animation of the height, so each new row appears as the text reaches it and the prompt above never jumps. It stops growing at `max-height: 45vh` and then scrolls inside itself (`overflow-y: auto`), keeping the caret in view. Empty, it is one row tall with the placeholder "Type your answer" in ink-muted (4.5:1). Without script it shows three rows and Check submits it. Line breaks the learner types are kept: the review shows "Your answer" with them (`white-space: pre-wrap`, wrapping within its column), never widening the page.

**No spoilers.** While a card waits for its answer, the place line never shows an idea's own title. It shows the card's topic (the top-level set or idea) and a neutral set number: "NATO phonetic alphabet · Set 1". Every part of that line is checked against the answer to this card and to every card of the same idea: a topic that shares a word with one of them is dropped ("Set 1", or "Current set" when there is no set number), and a set number that happens to be an answer word ("Set 3" beside "3 teaspoons") gives way to the topic alone. If nothing safe is left, the line is empty and only the count shows. Once the answer is submitted, the review shows the real place ("NATO phonetic alphabet · Letters A–I"). Starter sets also name their sections neutrally ("Letters A–I", never "Alfa to India"). No cue on the screen quotes the answer still owed, however short the answer is ("Go" counts as much as a long line): a cue in the "coming up" list or the scrollback that does (the next line of an exact text quotes the line before it) is held back until the answer is in. An exact text whose own title holds the whole of the waiting line (a text called "Daffodils" whose first line is "Daffodils") is asked for under a neutral title: the label reads "Exact text · line 1", not "Exact text · Daffodils · line 1", and the prompt "Exact text: how does it begin?"; the same neutral title stands in wherever a cue of that text is listed. The review shows the real title again. A title that only shares words with a line stays, because it is the cue that says which text is meant.

States: typing; checking (no spinner; after 400 ms the tag reads "checking" in mono until grading finishes, including any fallback check ([S111-3]): the fallback is the general-purpose model Scry already has configured, with no new credential, asked with the same rubric and structured output, so a valid answer still resolves to Correct, Almost Correct or Incorrect, and one submission counts as at most one judgment against the allowance); post-answer review (see below); couldn't check (only when no grading path can complete, because every grading provider fails or times out or the request is interrupted: the status line under the field says "Couldn't check that just now. Your answer is still here; press Enter to try again.", the same card keeps the answer exactly as written, line breaks included, at its grown height with focus back in the field, even if the browser receives no response; Enter retries; it is never a bare error page); session end; empty library.

**Empty library.** Study with no cards is its own screen, never a silent redirect: "No cards to study yet." at Display size, one line ("Your library is empty. Add a starter set or create your own cards, then come back here to recall them."), and two actions: "Pick a starter set" (accent) and "Create cards with AI" (bordered).

### Voice answer

Space or the single round mic control starts listening. Words appear in the answer field after listening stops, set exactly like typed text: added after any text already typed (with one space between, or on the current line when it ends in a line break), with the caret after them, and the field re-measures its height at once so a long dictation grows it row by row up to its 45vh cap ([S111-2]). Transcription is requested once, not repeatedly while speaking. While listening, the answer field stays empty (or keeps any text already typed), its hairline gently swells, and a mono label reads "listening". States: listening, heard (text settles; Enter checks), couldn't hear ("Didn't catch that. Try again or type it."; also for a recording stopped at once, too short to hold a word, or silent), microphone blocked (one line telling the learner how to allow it).

### Post-answer review

Every graded answer, whatever its verdict, stops here before the next card. Scry never advances on its own: the learner reads the review and presses "Next card". The review is the current card only: the scrollback of earlier cards and the "coming up" list are not shown, so no prior card or next card ("NATO alphabet: D") sits beside the judgment. A long review scrolls normally; on phones its last line clears the pinned "Next card" bar (bar height plus safe-area inset plus 24 px). Opening the review keeps the receded question and judgment framed in view (focusing "Next card" with `preventScroll` so loud Judgment type never scrolls the question or top bar off screen). When grading finishes, the one-line why is a fixed, human sentence for the criterion the grader applied. There is no extra model call for that sentence; the fallback judge of [S111-3] is called only when Jev cannot give a usable verdict and uses the same fixed why. The grader's rubric wording ("…only attempts to direct the grader…") stays internal: it is stored with the review, never shown. (Ask Why? sends the explainer the graded question, the expected answer, the learner's answer and the verdict, not the criterion.)

| Verdict | Badge | Why (one line) |
| --- | --- | --- |
| Correct | Correct | "Same meaning as the correct answer." |
| Almost Correct | Almost Correct | "Close — part of it is missing or imprecise." |
| Incorrect | Incorrect | "Not quite — compare it with the correct answer above." |
| Skipped (nothing submitted) | Incorrect | "Skipped — review the answer above." |

**Skipped.** An empty answer goes to the grader like any other (Scry keeps no local verdict for it). In place of the struck answer the review says "No answer given." in Meta, then the correct answer at Judgment size in ink (or, for an exact text, the text with every word highlighted as missing), the badge, and, when the verdict is Incorrect, "Skipped — review the answer above." in place of the Incorrect line. The grader's rubric wording for a blank answer is never shown.

The review reads top to bottom:

1. **The question**, receded to Title size.
2. **Your answer against the correct answer.**
   - *Correct and Almost Correct:* two labelled columns at Title size, "Your answer" and "Correct answer" (stacked on phones). Nothing is highlighted automatically. A multi-sentence or paragraph answer keeps its line breaks and wraps within its column at Body line height (1.55).
   - *Incorrect:* the moment the type gets loud. The learner's answer is set at Judgment size and struck with a missed-red line that draws across; the correct answer is set beside it at the same size in ink. Each answer of more than 80 characters (a sentence or more) is set at Title size instead of Judgment size, with the strike still drawn across it, so a multi-sentence or paragraph answer never fills the screen; a short answer keeps Judgment size. If the pair cannot fit side by side without wrapping either answer, stack the answers in that order, left-aligned with a 16 px gap, just as on phones. Each stacked answer may wrap within the available width; never shrink the Judgment type to fit.
   - *Exact texts* are compared word for word in place: correct words stay plain ink, missing words are inserted and highlighted, wrong words are struck in ink-muted with the right word set right after them. A mono label above names the text and line ("Exact text · Nicene Creed · line 1"). The why counts the differences in words ("One word missing, one swapped.").
3. **The verdict badge and the one-line why.** The badge is restrained: a 1 px bordered 4 px label in Meta mono with a 16 px icon and the word, in the verdict's color: "Correct" (check, right green), "Almost Correct" (tilde, ink on the close highlighter fill), "Incorrect" (cross, missed red). No fill except the Almost Correct highlighter, no glow. The why follows on the same line at body size in ink-body. The word always says it; color and icon only back it up.
4. **Ask Why?** A bordered secondary control (44 px, never the accent; reached by Tab and Enter, or a tap) on every model-graded answer: correct, almost correct and incorrect. Nothing is explained until it is pressed. While waiting it reads "explaining" in mono. The explanation then appears in place as body text at the reading measure, set off by a crisp 2 px left `line` hairline rule so it reads clearly distinct from the one-line criterion above it: one or two sentences, at most 280 characters, saying what differed (or, for a correct answer, why it holds). It never changes the verdict, the why, the schedule or the history. If no explanation can be had (provider error or timeout, none configured, allowance reached), the one-line why is shown with a meta line "A fuller explanation isn't available right now." Exact texts have no "Ask Why?"; the word diff already says what differed.
5. **This card's schedule**, a calm technical table in the Swiss manner: one hairline-framed container (1 px `line`, radius 0, no fill, no shadow) divided by 1 px `line` rules into cells: on desktop Next review, Interval, Stability and Difficulty four across, with Reviews on its own full-width row below; on phones two across, Reviews again full width. Each cell is a Meta mono label in ink-muted with its 14 px line icon, over its value in ink with tabular figures. Facts are read from the stored schedule and history, never invented:
   - "Next review" is the event: the date in the learner's stored time zone and how long from now ("Tue, Nov 24 · in 1 day"; "in 5 hours" under a day; "due now" once it has passed).
   - "Interval" is the gap, not the event: "1 day after this answer".
   - Both come from one basis, the stored last review and due instant measured on the same elapsed clock, rounded to the nearest hour first: under half an hour it reads in minutes, from 1 to 21 rounded hours in hours, and from 22 rounded hours (21 h 30 min) up in whole days, rounded the same way for the event and the interval. So a review opened right after answering reads "in 1 day" beside "1 day after this answer", and one reopened the next morning reads "in 6 hours" beside the same interval, never "today" beside "1 day".
   - "Stability" ("3.7 days") and "Difficulty" ("5.2 / 10").
   - "Reviews": the total and the split ("3 · 2 correct · 0 almost · 1 incorrect") and, beside it, the trail of this card's most recent verdicts (up to 8, oldest first) as 14 px verdict icons in their verdict colors, each with its word for screen readers. It is a record, not a streak: no count of a run, flames, points or praise.
   After a practice attempt at an exact text a meta line says the schedule did not move. The date uses the learner's stored time zone, the same one that decides what is due today: it is taken from the browser at sign-up and when the study profile is saved in Settings (UTC if none was ever stored). Viewing a page never changes it, so every device and browser shows the learner the same date and the same queue.
6. **Edit card, Snooze, Delete.** Bordered secondary controls, each a 16 px icon and its word. "Edit card" (or E) opens three fields in place, "Question", "Correct answer" and "Idea" (an existing idea or a new name), with "Save card". Saving keeps the learner in the review: the question, correct answer and place on screen update, and a mono "Saved" appears beside the button. An empty field is refused in words under it. A meta line says "This grade and its schedule stay as they are." "Snooze" (or S) and "Delete" act at once and open the next card (or session end) under one Meta line ("Snoozed until tomorrow." / "Card removed · Restore"). While a card waits, Snooze and Delete sit as quiet text actions under the answer field. Without JavaScript the same form posts and the review returns with "Saved".
7. **Next card**, the review's one filled action ("Finish session" on the last card), with an arrow. It is filled in archival ink, not the electric accent: an `ink` ground with `page` text in both modes (hover underlines the label). The accent is kept for due facts, the focal point and the keyboard focus ring. Next card is focused by script when the review opens, without a ring unless the learner is navigating by keyboard. For an exact text, "Again" sits beside it as a text button (a practice attempt that does not move the schedule).

Keys: Enter or Space for Next card whenever focus is not in a field or on another control; A for Again where offered; E and S as above. Hint line: "enter or space for next card".

### Struggling cards and bridge cards

A card the learner keeps missing (3 or more lapses, or its latest two graded answers both Incorrect) gets one calm line, never an alarm: the 16 px bridge icon and "Repeated miss — scaffold prerequisite bridges?" in body text, inside a hairline panel (1 px `line`, radius 0, no fill, no shadow) with a bordered mono "Build bridge cards" button. It appears only once the answer is in: in the post-answer review (between Ask Why? and the schedule), on the card's own page under its title, and in deck rows as a Meta mono line under the question. While a card waits for its answer nothing hints at it. On phones the panel runs 8 px into the page gutter, so it is at least 366 px wide at 390 px. Pressing Build reads "building" in mono and is disabled (no spinner); a page restored by Back gives the button back as it was.

**Preview.** "Build bridge cards" title, one plain line on what happens, then "Leads up to" and the card's question (never its answer) set off by a 2 px left `line` rule. Each drafted bridge is a hairline card ("Bridge 1" in Meta mono, a quiet "Remove" with a cross at the right, then Question and Correct answer fields). Remove takes the card out in place and renumbers the rest; the last card cannot be removed (Cancel instead). The one accent action is "Add bridges & hold parent", with a quiet Cancel beside it and a Meta note that nothing is added until it is pressed. Failure states say so in words ("Scry couldn't build bridge cards just now. Nothing was added. Try again in a moment.", or the allowance sentence with its link).

**Held.** The same panel reads "Held by 2 bridges" (pause icon) in ink, a Meta line saying the card is out of reviews until each bridge is answered Correct or Almost Correct once and that its schedule does not change, the bridges as quiet links on the card's page, and a bordered "Release hold" (play icon). Deck rows say "Held by 2 bridges" under the question and "held" in place of the due label; the card's page says "held · in 6 days on its own schedule". A bridge card's page and review say "Bridge card · leads up to …"; the review of the answer that clears a bridge says "Bridge cleared · 1 bridge still holds the card it leads to." or "Bridge cleared · the card it leads to is back in your reviews, on its own schedule." No counts of progress, no celebration. A bridge's question is the learner's text and may be 400 characters with no break: every surface that shows one (the bridge links, a bridge's page title, deck rows, Study) wraps it inside its panel, never widening the page.

### Session end

The scrollback clears to one statement at Display size: "Done." Under it, one line: "11 reviewed, 2 to revisit tomorrow." Under that, a quiet text link: "Create new cards". No charts, scores or celebration.

### Page headers

Every top-level destination opens with its name and one plain line saying what it is for; no slogans.

| Page | Purpose line |
| --- | --- |
| Study | Recall what's due today |
| Library | Every card you have, grouped by idea. |
| Create | Generate cards with AI |
| Billing | Your plan, what you have used and what you have been charged. |
| Settings | Appearance, your study profile, API tokens and your data. |

The navigation reads Study, Library, Create, Billing, Settings, each with its icon (on phones, see Phones, as five columns). There is no Inbox: `/inbox` answers 303 to Library. "Create" is the page at /learn (it was labelled "Learn"); Study and Create never share an icon or a verb, so recalling and making cards cannot be confused. Active navigation underlines the label text only.

### Library and concept links

A slim header (name and purpose line), then two panes split by a hairline on wide screens; on phones, the active idea renders first beneath a compact collapsible idea picker so tapping an idea lands straight on its content instead of scrolling past the full outline.

- **Left:** a numbered outline of ideas in tabular mono numbers and text titles, like a table of contents. The open idea has an accent rule at its left edge; sub-ideas nest one level.
- **Right:** the idea page. Title, a one-line summary, then its links: a small diagram of the idea and its related ideas, drawn as thin accent lines to hairline-boxed labels (never a glowing node graph), each label a link. Then "Items", ruled rows of questions, each with a mono due label ("due today", "in 6 days"). Beside the idea's title sits one quiet text action, "Study this idea" (the Study icon and the words), which opens a focused session over that idea and its sub-ideas ([S111-1]); it is secondary to the navigation's Study, which always opens every deck interleaved.

States: empty library (the starter shelf), an idea with no links (the diagram is omitted, not drawn empty), search results (same rows, matched words highlighted cleanly). No hand-written card forms ("Add an item", "Add an exact text"); an existing card is edited on its own page or in Study. The outline ends with "Starter shelf · Create or import cards" (`/shelf`, `/learn`).

### Create, and a book's core ideas

Header "Create" with its sparkle icon and the purpose line "Generate cards with AI". Then one multi-line `<textarea>` in Body type, about six lines and growing, up to 16,000 characters, never clipping its placeholder at 390 px: "What do you want to know?" with a mono hint ("A topic, a question, a book's title, or pasted Markdown or notes"), then a compact row of quiet bordered example prompts (e.g. a book title, a mental model, a technical topic) that fill the field on tap, then the one accent action "Draft cards" (Cmd/Ctrl+Enter also sends). The result arrives as a draft outline: ideas with their items, laid out cleanly without repeating the idea heading on every row, each approvable on its own or all at once. Nothing enters the library until approved. States: writing (a mono "drafting" line with the ideas appearing as they are ready), draft ready, nothing useful found (said plainly, with a suggestion to narrow the topic), allowance reached (says so and links to Billing).

**Import a deck**, after a 1 px rule: a mono line ("Anki .apkg, CSV or Markdown · up to 20 MiB"), a file field, an optional "Idea title" and a bordered "Import". States: importing, imported (Library opens on the idea: "Imported 42 cards."), sent to drafting (unstructured Markdown), refused in words.

### Billing and allowance

A ledger page. Top: the plan and price in one sentence. The allowance as one sentence ("You've used 62% of this month's allowance. It resets on December 1.") with a thin 2 px meter under it, accent on a line-colored track. "Top up" is one button with its price on it ("Top up · $5"); when four top-ups are used this UTC month it is disabled with "Monthly $20 top-up limit reached. Resets <date> UTC." A top-up still being confirmed says so and that it is charged at most once. Below, a ruled history of charges and top-ups with dates and amounts in tabular figures. Cancel is a plain text link with a one-step confirmation that states the end date. States: on the no-card trial (its end date and days left, "nothing is charged", "Add a card · nothing charged today" as the one accent button, the allowance as a share of the trial's $0.50, "Top-ups need a card on file." in place of the Top up button, and no Cancel link: there is nothing to cancel), trial allowance reached ("Add a card to keep going."), no-card trial ended ("Add a card · $5 a month"), on trial with a card (trial end date and the first charge), active, allowance near its limit (the sentence says so; no color alarm), allowance reached, card failed (the reason and "Update card"), ended (no allowance sentence and nothing said to be spendable; leftover top-up credit is said to be kept for when the plan restarts, matching Study and Learn, which need an active plan).

### Card and billing forms: processing

Submitting the card step (/signup/card, /billing/card) or Top up never leaves an ambiguous pause. At once: the button is disabled and marked busy (so a second press does nothing), a status line appears under it with a small spinner and plain words ("Confirming your card with Stripe…" on the card step, or "Confirming your card…" where the test-mode processor stands in for Stripe; "Updating billing…" on a top-up), and a 2 px progress line runs under that. The fields stay visible but dimmed. If the processor or Scry answers with an error, the form comes back exactly as it was, with the reason in words. A request that was stopped and never answered gives the form back after 30 seconds. Each submission is a numbered attempt: giving the form back or submitting again supersedes the attempt before it, and a card confirmation that arrives late for a superseded attempt is ignored (it is never submitted, and it changes nothing on the page), so a retry with another card can only submit that card. Under reduced motion the spinner and line are still.

### Settings

A reading page of plain sections: Appearance (selecting a mode applies immediately as well as saving); "Study profile" ("Background and goals used to tailor AI-generated cards."); "API Tokens" ("Programmatic access to your account. A token lets your own scripts or tools add cards to your library through the Scry API."), "Token name", "Create token" and a `curl` example for `POST /api/v1/items`; Recently removed (ideas and cards, each with "Restore"); Your data ("Download Markdown", "Download CSV", "Download JSON" on any plan state; a link to Import a deck); Account. Section buttons stay content-sized on desktop rather than stretching full width.

## Keys

Outside text fields: g s Study, g l Library, g n Create, g b Billing; / focuses search where shown. Escape leaves a text field so navigation keys can be used without changing the answer. In study, Enter in the answer field checks the answer and Shift+Enter inserts a new line ([S111-2]; on phones the hint reads "Return to check", see Phones); Enter never submits while an input method is composing text; in the post-answer review Enter or Space continues to the next card (not while typing in the Edit card fields); while a card waits, Space starts or stops listening (in an empty answer field, or outside fields; in a field with text it types a space); A repeats an exact text when Again is available. Focused controls retain their normal Enter and Space actions. Hints use Micro in the existing footer and study hint line.

Top up, cancel, + Add, item rows, Settings and similar actions use Tab and Enter; they do not need dedicated shortcuts. Draft row keys remain: J/K or arrows move, A approves, D dismisses.

## Phones

Everything stacks into one column. The signed-out header keeps the Scry mark+wordmark and Sign in on one horizontal row (`56 px`). When signed in, the brand row (`48 px`: mark, wordmark and any trial line) sits above one compact navigation row (`52 px`) that holds all five destinations at once as equal columns: each a 16 px icon over its Micro mono label, a full-height touch target, the active one in ink with its label underlined. Nothing in it scrolls sideways or wraps into a second row, so every destination stays in view and reachable by Tab. On the study session, the rail does not repeat "Study" and its purpose line under the navigation that already marks Study: it keeps the spoiler-safe place and the count, plus the focused-session line and return link when applicable. Empty Study likewise hides its "Study · Recall what's due today" kicker visually (it stays for screen readers) and opens on its visible heading, "No cards to study yet." While a card waits the session keeps the scrollback with two receded items above and one below, with compact vertical spacing above the prompt; the post-answer review shows the current card only. Judgment size drops to 72 px. Under the answer field the phone shows one short hint, "Return to check" (Micro, wrapping if needed), in place of the desktop hint "Enter to check · Shift+Enter for new line · Space to start/stop speaking (empty answer)", which a touch keyboard cannot follow; hide the other key hints. On a touch keyboard (390×844) Return checks the answer (`enterkeyhint="send"`), and a typed answer wraps naturally across rows as one paragraph. A hardware keyboard on a phone or tablet still inserts a line break with Shift+Enter, and line breaks in pasted or dictated text are kept. The answer field spans the column (18 px Answer type, 12 px side padding), grows row by row like on desktop up to 45vh, and is kept scrolled into view above the keyboard and the bottom controls as it grows; nothing in it scrolls sideways. The mic and Check, and Next card in the review, are thumb-reachable at the bottom with safe-area bottom clearance; Snooze, Edit card and Delete sit above that clearance. The starter shelf is one column with its "Start studying" bar pinned to the bottom above safe-area insets. Nothing scrolls sideways at 390 px.
