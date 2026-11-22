# Scry: design

Scry is for someone who wants what matters to stay in their head, for about five minutes a day. It should feel like a precise, warm instrument: closer to Warp and Supermemory than to a book or a game. [PRODUCT.md](PRODUCT.md) owns what Scry does. This file owns how it looks and feels.

Accepted by Phaedrus on 2026-11-17 (chat with Draper, shaped under [MIS-1210](https://linear.app/misty-step/issue/MIS-1210)). Session record, candidates and reasons: [design/sessions/2026-11-17-scry-design](design/sessions/2026-11-17-scry-design/decisions.md). The reference boards there illustrate the direction. The tokens and rules below are what bind.

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
| Body | 16–17 px / 1.55 | 400 | Reading, rows, the one-line why |
| Meta | 13 px / 1.4, mono | 400 | Labels, counts, sources, keys |
| Micro | 11 px / 1.4, mono, +0.04em | 400 | Keyboard hints only |

Answers the learner types are set in Text at the prompt's size, so their words carry the same weight as the question. Mono labels are sentence case or short uppercase tags; never long uppercase sentences.

## Icons

One family of inline SVG line icons, drawn on a 24 px grid: `stroke="currentColor"`, stroke width 1.75, round caps and joins, no fill, `aria-hidden="true"`. They are served inline in the page (no icon font, no sprite request, no emoji) and rendered at 16 px in the navigation and inline labels, 18 px in buttons and verdicts, and 20 px on starter tiles. An icon takes the color of the text it sits beside and always has that text next to it, so it never carries meaning alone.

- **Navigation:** every destination has one: Study (a stack of cards), Library (books on a shelf), Inbox (a tray), Create (a sparkle), Billing (a card), Settings (sliders).
- **Starter tiles:** each set has an icon for its subject in ink-muted beside its title (a radio mast for the NATO phonetic alphabet, layers for mental models, a ruler for kitchen numbers, a globe for geography, a beaker for chemistry, an atom for physics, a scroll for the Creeds, and so on). A tile stays a typographic hairline tile; the icon is a glyph on the title line, not a picture or a filled badge.
- **Counts:** an item count is a compact card-stack icon and the number in tabular figures ("26 cards").
- **Actions and verdicts:** Add (plus), Added (check), Start studying and Next card (arrow), Ask Why? (question mark), Edit card (pencil); Correct (check in a circle), Almost Correct (a tilde in a circle), Incorrect (a cross in a circle).

## Color

Ink steps are the base ink at fixed opacity, so both modes keep the same hierarchy.

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| page | `#FFFFFF` | `#12101A` | Page ground |
| raised | `#FFFFFF` | `#1A1522` | Panels, hovered rows |
| sunken | `#F4F4F6` | `#241E2F` | Fields, wells, the shelf hover |
| ink | `#0B1015` | `#FFFFFF` | Primary text |
| ink-body | ink at 75% | ink at 75% | Reading text |
| ink-muted | ink at 60% | ink at 60% | Labels, sources, receded items |
| ink-ghost | ink at 20% | ink at 22% | Upcoming items, disabled, the dot grid |
| line | ink at 12% | ink at 16% | Hairlines and rules |
| accent | `#0562EF` | `#4D8DFF` | The one main action, anything due, concept links, focus ring |
| right | `#1E7F4E` | `#5FD39A` | The "right" tag |
| close | `#EEF17C` fill, ink text | `#EEF17C` at 30% fill, ink text | Highlighter mark behind missing words in an exact text, and matched words in search |
| missed | `#C23B26` | `#FF7A66` | The strike through a wrong answer and the "missed" tag |

The accent appears at most once as a filled control per screen. Controls come in three weights: the one filled accent action, bordered secondary buttons (ink text, a ghost hairline that turns ink on hover), and quiet text links. Hover and focus are crisp, never soft: a fill or border change in 150 ms and a 2 px accent focus ring; a pressed button moves down 1 px. Mode follows the system setting, with a toggle in Settings. Contrast: body and meta text meet 4.5:1 in both modes; ghost is never used for text the learner must read.

## Shape, space and texture

- Surfaces are square (radius 0). Controls and fields are 4 px. Nothing is a pill.
- 1 px hairlines separate things; no shadows, no gradients, no blur.
- Spacing steps: 4, 8, 16, 24, 32, 48, 64, 96 px. Section padding is 56 px on desktop, 40 px on phones.
- Reading pages have a measure of at most 68 characters. Inbox, draft and study scrollback rows are exempt. The page frame is at most 1120 px.
- The study session sits on a faint 22 px dot grid (ink-ghost at about 6%) with short tick marks along its left and right edges. No other screen has a texture.

## Motion

- Fades and short moves of 150–250 ms, ease-out. Nothing waits on an animation, and nothing loops except the two processing marks shown while a card or billing form is being confirmed (a 14 px spinner and a 2 px progress line; see "Card and billing forms: processing").
- Two drawn lines: the strike through a missed answer draws across once (200 ms), and an idea's link lines draw in once when its page opens.
- Under reduced motion, everything appears in its final state.

## Wordmark

"scry" in Geist Mono, lowercase, ink. No glyph for now.

## Key screens and states

### Sign-up and trial

One centered column. Email and password, and the 7-day trial starts at once, with no card: the newcomer lands on the starter shelf (Study once the library has items). The sign-up page says "7-day trial · no card needed" and the landing page "7 days free · no card needed · then $5 a month if you keep it". States: empty, typing, error in words under the field (for example "That email already has an account.").

During a no-card trial the top bar carries one quiet line after the wordmark, in micro mono text inside a hairline frame, never a banner or a color alarm: "Trial · 6 days left · Upgrade" ("last day" on the final day; "Trial ended · Add a card" after it), where only "Upgrade" (or "Add a card") is accent-colored, and the whole line links to the card step. It is not shown on bare pages (sign-up, sign-in, the card step) or once a card is on file.

The card step (/signup/card, or Billing's "Add a card") says in plain words, inside the trial: "Nothing is charged today: your trial still ends on <date>, when your card is first charged $5. Cancel any time before then in Billing." with the button "Add card · nothing charged today"; after the trial (or with under an hour left): "Your 7-day trial ended on <date>. Add a card to keep studying: $5 today, and then monthly." with "Start · $5 a month". States: card declined with the reason the processor gives, and the trial carries on unchanged.

### Starter shelf

Shown when the library is empty and reachable later from the library. Title "Pick a deck and start recalling in seconds." with one mono line under it ("30 starter sets · add one or more to begin"). Below, a hairline grid of typographic tiles: no pictures, no filled cards. Each tile holds the set's subject icon and title (Title) on one line, a mono item count with the card-stack icon ("26 cards"), one line of description and a bordered "+ Add" action.

Order: the first row is broad and instantly usable (NATO phonetic alphabet, Mental models, Kitchen numbers, World geography), then everyday knowledge and the core ideas of each subject. The Creeds and Passages worth keeping (word-for-word texts) close the shelf; they are never in the first row.

States: default; hover (sunken fill); adding (the button reads "Adding…" and is disabled); added (accent hairline outline and the accent tag "Added" with a check). Adding is confirmed in place: the page does not reload and the scroll position does not move. After the first add a bar pinned to the bottom of the window appears, without scrolling the page: the count ("2 sets added · 50 cards") and the screen's one filled accent action, "Start studying". Without JavaScript the same form posts and the page returns to the tile that was added.

### Study session

The center of Scry. The session reads like scrollback:

- Answered items recede above the current one in ink-muted, one line each, with a mono verdict tag at the right edge ("correct", "almost correct", "incorrect").
- The current prompt sits in the middle at Display size, with the answer line under it and a block caret.
- Upcoming items fade below in ink-ghost, at most three.
- The left margin says where the learner is, in mono: the Study icon and name with the purpose line "Recall what's due today", then the spoiler-safe place and the count ("4 / 11"). The bottom right holds the key hints: "enter to check · space to start/stop speaking (empty answer)".

**No spoilers.** While a card waits for its answer, the place line never shows an idea's own title. It shows the card's topic (the top-level set or idea) and a neutral set number: "NATO phonetic alphabet · Set 1". Every part of that line is checked against the answer to this card and to every card of the same idea: a topic that shares a word with one of them is dropped ("Set 1", or "Current set" when there is no set number), and a set number that happens to be an answer word ("Set 3" beside "3 teaspoons") gives way to the topic alone. If nothing safe is left, the line is empty and only the count shows. Once the answer is submitted, the review shows the real place ("NATO phonetic alphabet · Letters A–I"). Starter sets also name their sections neutrally ("Letters A–I", never "Alfa to India"). The "coming up" list follows the same rule: a cue that quotes the answer still owed (the next line of an exact text quotes the line before it) is held back until the answer is in.

States: typing; checking (instant; no spinner unless a check takes over 400 ms, then the tag reads "checking" in mono); post-answer review (see below); session end; empty library.

**Empty library.** Study with no cards is its own screen, never a silent redirect: "No cards to study yet." at Display size, one line ("Your library is empty. Add a starter set or create your own cards, then come back here to recall them."), and two actions: "Pick a starter set" (accent) and "Create cards with AI" (bordered).

### Voice answer

Space or the single round mic control starts listening. Words appear on the answer line after listening stops, set exactly like typed text; transcription is requested once, not repeatedly while speaking. While listening, the answer line stays empty (or keeps any text already typed), its hairline gently swells, and a mono label reads "listening". States: listening, heard (text settles; Enter checks), couldn't hear ("Didn't catch that. Try again or type it."; also for a recording stopped at once, too short to hold a word, or silent), microphone blocked (one line telling the learner how to allow it).

### Post-answer review

Every submitted answer, whatever its verdict, stops here before the next card. Scry never advances on its own: the learner reads the review and presses "Next card". The judgment is instant, and the one-line why is the fixed sentence of the criterion the grader applied (accepted by Phaedrus: no extra model call on the hot path, so judging stays fast and cheap).

The review reads top to bottom:

1. **The question**, receded to Title size.
2. **Your answer against the correct answer.**
   - *Correct and Almost Correct:* two labelled columns at Title size, "Your answer" and "Correct answer" (stacked on phones). Nothing is highlighted automatically.
   - *Incorrect:* the moment the type gets loud. The learner's answer is set at Judgment size and struck with a missed-red line that draws across; the correct answer is set beside it at the same size in ink. If the pair cannot fit side by side without wrapping either answer, stack the answers in that order, left-aligned with a 16 px gap, just as on phones. Each stacked answer may wrap within the available width; never shrink the Judgment type to fit.
   - *Exact texts* are compared word for word in place: correct words stay plain ink, missing words are inserted and highlighted, wrong words are struck in ink-muted with the right word set right after them. A mono label above names the text and line ("Exact text · Nicene Creed · line 1"). The why counts the differences in words ("One word missing, one swapped.").
3. **The verdict badge and the one-line why.** The badge is a bordered 4 px label with an icon and the word, in the verdict's color: "Correct" (check, right green), "Almost Correct" (tilde, ink on the close highlighter fill), "Incorrect" (cross, missed red). The word always says it; color and icon only back it up.
4. **Ask Why?** A bordered secondary control (44 px, never the accent; reached by Tab and Enter, or a tap) on every model-graded answer: correct, almost correct and incorrect. Nothing is explained until it is pressed. While waiting it reads "explaining" in mono. The explanation then appears in place as body text at the reading measure: one or two sentences, at most 280 characters, saying what differed (or, for a correct answer, why it holds). It never changes the verdict, the why, the schedule or the history. If no explanation can be had (provider error or timeout, none configured, allowance reached), the criterion sentence is shown with a meta line "A fuller explanation isn't available right now." Exact texts have no "Ask Why?"; the word diff already says what differed.
5. **This card's schedule**, a ruled row of mono facts read from the stored schedule and history, never invented: "Next review" (date and relative time: "Nov 24 · in 2 days"), "Interval" ("2 days"), "Stability" ("3.7 days"), "Difficulty" ("5.2 / 10") and "Reviews" (the total and the split: "3 · 2 correct · 0 almost · 1 incorrect"). Facts only: no streaks, flames, points or praise. After a practice attempt at an exact text a meta line says the schedule did not move.
6. **Edit card.** A bordered secondary toggle that opens two fields in place, "Question" and "Correct answer", with "Save card". Saving keeps the learner in the review: the question and correct answer on screen update, and a mono "Saved" appears beside the button. An empty question or answer is refused in words under the fields. A meta line says "Edits apply from the next review. This grade and its schedule stay as they are." Without JavaScript the same form posts and the review returns with "Saved".
7. **Next card**, the screen's one filled accent action ("Finish session" on the last card), with an arrow. For an exact text, "Again" sits beside it as a text button (a practice attempt that does not move the schedule).

Keys: Enter or Space for Next card whenever focus is not in a field or on another control; A for Again where offered. Hint line: "enter or space for next card".

### Session end

The scrollback clears to one statement at Display size: "Done." Under it, one line: "11 reviewed, 2 to revisit tomorrow." Under that, a quiet text link: "Create new cards". No charts, scores or celebration.

### Page headers

Every top-level destination opens with its name and one plain line saying what it is for; no slogans.

| Page | Purpose line |
| --- | --- |
| Study | Recall what's due today |
| Library | Every card you have, grouped by idea. |
| Inbox | Cards suggested by external scripts or tools wait here until you approve them. |
| Create | Generate cards with AI |
| Billing | Your plan, what you have used and what you have been charged. |
| Settings | Appearance, your study profile, API tokens and your data. |

The navigation reads Study, Library, Inbox, Create, Billing, Settings, each with its icon. "Create" is the page at /learn (it was labelled "Learn"); Study and Create never share an icon or a verb, so recalling and making cards cannot be confused.

### Library and concept links

A slim header (name and purpose line), then two panes split by a hairline on wide screens; stacked on phones.

- **Left:** a numbered outline of ideas in mono numbers and text titles, like a table of contents. The open idea has an accent rule at its left edge; sub-ideas nest one level.
- **Right:** the idea page. Title, a one-line summary, then its links: a small diagram of the idea and its related ideas, drawn as thin accent lines to hairline-boxed labels (never a glowing node graph), each label a link. Then "Items", ruled rows of questions, each with a mono due label ("due today", "in 6 days").

States: empty library (the starter shelf), an idea with no links (the diagram is omitted, not drawn empty), search results (same rows, matched words highlighted).

### Inbox

Cards suggested through the learner's API tokens. Header "Inbox" with a mono count ("7 suggestions"), the purpose line "Cards suggested by external scripts or tools wait here until you approve them." and "Approve all" as a text action at the right. Dense ruled rows: the suggested question in body text, under it a mono source line ("from reading agent · The Federalist Papers"), the idea it would join at the right, then approve (accent, 4 px square control) and dismiss (quiet). The focused row is raised and shows "A approve · D dismiss". No icon sidebar. States: empty ("Nothing waiting." and one line pointing to API Tokens in Settings), approving (the row collapses upward), approved all.

### Create, and a book's core ideas

Header "Create" with its sparkle icon and the purpose line "Generate cards with AI". Then one prompt field at Display size: "What do you want to know?" with a mono hint underneath ("A topic, a question, or a book's title"). The result arrives as a draft outline: ideas with their items, laid out like the inbox, each approvable on its own or all at once. Nothing enters the library until approved. States: writing (a mono "drafting" line with the ideas appearing as they are ready), draft ready, nothing useful found (said plainly, with a suggestion to narrow the topic), allowance reached (says so and links to Billing).

### Billing and allowance

A ledger page. Top: the plan and price in one sentence. The allowance as one sentence ("You've used 62% of this month's allowance. It resets on December 1.") with a thin 2 px meter under it, accent on a line-colored track. "Top up" is one button with its price on it ("Top up · $5"); when four top-ups are used this UTC month it is disabled with "Monthly $20 top-up limit reached. Resets <date> UTC." A top-up still being confirmed says so and that it is charged at most once. Below, a ruled history of charges and top-ups with dates and amounts in tabular figures. Cancel is a plain text link with a one-step confirmation that states the end date. States: on the no-card trial (its end date and days left, "nothing is charged", "Add a card · nothing charged today" as the one accent button, the allowance as a share of the trial's $0.50, "Top-ups need a card on file." in place of the Top up button, and no Cancel link: there is nothing to cancel), trial allowance reached ("Add a card to keep going."), no-card trial ended ("Add a card · $5 a month"), on trial with a card (trial end date and the first charge), active, allowance near its limit (the sentence says so; no color alarm), allowance reached, card failed (the reason and "Update card"), ended (no allowance sentence and nothing said to be spendable; leftover top-up credit is said to be kept for when the plan restarts, matching Study and Learn, which need an active plan).

### Card and billing forms: processing

Submitting the card step (/signup/card, /billing/card) or Top up never leaves an ambiguous pause. At once: the button is disabled and marked busy (so a second press does nothing), a status line appears under it with a small spinner and plain words ("Confirming your card with Stripe…" on the card step, or "Confirming your card…" where the test-mode processor stands in for Stripe; "Updating billing…" on a top-up), and a 2 px progress line runs under that. The fields stay visible but dimmed. If the processor or Scry answers with an error, the form comes back exactly as it was, with the reason in words. A request that was stopped and never answered gives the form back after 30 seconds. Under reduced motion the spinner and line are still.

### Settings

A reading page of plain sections: Appearance; "Study profile" ("Background and goals used to tailor AI-generated cards."); "API Tokens" ("Programmatic access to your account. A token lets your own scripts or tools suggest cards through the Scry API; suggestions wait in your Inbox until you approve them."), with one field, "Token name", and "Create token" (no other options); Recently removed; Your data; Account.

## Keys

Outside text fields: g s Study, g l Library, g i Inbox, g n Create, g b Billing; / focuses search where shown. Escape leaves a text field so navigation keys can be used without changing the answer. In study, Enter checks the answer; in the post-answer review Enter or Space continues to the next card (not while typing in the Edit card fields); while a card waits, Space starts or stops listening (on an empty answer line, or outside fields); A repeats an exact text when Again is available. Focused controls retain their normal Enter and Space actions. Hints use Micro in the existing footer and study hint line.

Top up, cancel, + Add, item rows, Settings and similar actions use Tab and Enter; they do not need dedicated shortcuts. Existing inbox and draft row keys remain: J/K or arrows move, A approves, D dismisses.

## Phones

Everything stacks into one column. The study session keeps the scrollback with two receded items above and one below. Judgment size drops to 72 px. Key hints are hidden; the mic and Check, and Next card in the review, are thumb-reachable at the bottom. The starter shelf is one column with its "Start studying" bar pinned to the bottom. Nothing scrolls sideways at 390 px.
