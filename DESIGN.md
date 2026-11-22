# Scry: design

Scry is for someone who wants what matters to stay in their head, for about five minutes a day. It should feel like a precise, warm instrument: closer to Warp and Supermemory than to a book or a game. [PRODUCT.md](PRODUCT.md) owns what Scry does. This file owns how it looks and feels.

Accepted by Phaedrus on 2026-11-17 (chat with Draper, shaped under [MIS-1210](https://linear.app/misty-step/issue/MIS-1210)). Session record, candidates and reasons: [design/sessions/2026-11-17-scry-design](design/sessions/2026-11-17-scry-design/decisions.md). The reference boards there illustrate the direction. The tokens and rules below are what bind.

## Principles

- **Words first.** Every state is said in words. Color, highlight and strike back the words up and never carry the meaning alone.
- **One thing at a time.** Each screen has one job and one main action.
- **Quiet everywhere, one hard moment.** Pages are calm and exact. The moment a miss is judged is the one place the type gets loud.
- **Keyboard first, touch equal.** Navigation and study actions have dedicated keys. Everything else is reachable by Tab and Enter. Every action has a 44 px touch target.
- **Never:** streaks, confetti, mascots, badges, points, glows, violet gradients, gradient text, shadows under cards, icon-card grids, stock imagery, eyebrow labels over every heading, or the Inter typeface.

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

The accent appears at most once as a filled control per screen. Mode follows the system setting, with a toggle in Settings. Contrast: body and meta text meet 4.5:1 in both modes; ghost is never used for text the learner must read.

## Shape, space and texture

- Surfaces are square (radius 0). Controls and fields are 4 px. Nothing is a pill.
- 1 px hairlines separate things; no shadows, no gradients, no blur.
- Spacing steps: 4, 8, 16, 24, 32, 48, 64, 96 px. Section padding is 56 px on desktop, 40 px on phones.
- Reading pages have a measure of at most 68 characters. Inbox, draft and study scrollback rows are exempt. The page frame is at most 1120 px.
- The study session sits on a faint 22 px dot grid (ink-ghost at about 6%) with short tick marks along its left and right edges. No other screen has a texture.

## Motion

- Fades and short moves of 150–250 ms, ease-out. Nothing loops and nothing waits on an animation.
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

Shown when the library is empty and reachable later from the library. Title "Start with something worth knowing." Below, a hairline grid of typographic tiles: no cards, no images. Each tile holds the set's title (Title), a mono item count, one line of description and a "+ Add" mono action. States: default; hover (sunken fill); added (accent hairline outline and the tag "Added"); adding (the tag reads "Adding…"). After the first add, the main action becomes "Start studying".

### Study session

The center of Scry. The session reads like scrollback:

- Answered items recede above the current one in ink-muted, one line each, with a mono verdict tag at the right edge ("right", "close", "missed"). The newest right answer keeps its one-line why underneath until the next answer arrives, then shrinks to one line like the rest.
- The current prompt sits in the middle at Display size, with the answer line under it and a block caret.
- Upcoming items fade below in ink-ghost, at most three.
- The left margin holds the idea's name and the count ("4 / 11") in mono. The bottom right holds the key hints: "enter to check · space to start/stop speaking (empty answer)".

States: typing; checking (instant; no spinner unless a check takes over 400 ms, then the tag reads "checking" in mono); judged (see below); session end.

### Voice answer

Space or the single round mic control starts listening. Words appear on the answer line after listening stops, set exactly like typed text; transcription is requested once, not repeatedly while speaking. While listening, the answer line stays empty (or keeps any text already typed), its hairline gently swells, and a mono label reads "listening". States: listening, heard (text settles; Enter checks), couldn't hear ("Didn't catch that. Try again or type it."; also for a recording stopped at once, too short to hold a word, or silent), microphone blocked (one line telling the learner how to allow it).

### The answer judgment

Instant, under the answer. The one-line why is the fixed sentence of the criterion the grader applied (accepted by Phaedrus: no extra model call on the hot path, so judging stays fast and cheap).

- **Right:** a green mono "right" tag and the one-line why in body text. The item moves up into the scrollback.
- **Close:** the learner's answer is set plainly at the prompt's size (no automatic highlight of close parts), the tag reads "close", the why is the criterion sentence, and the right answer follows.
- **Missed:** the moment the type gets loud. The learner's answer is set at Judgment size and struck with a missed-red line that draws across; the right answer is set beside it at the same size in ink. If the pair cannot fit side by side without wrapping either answer, stack the answers in that order, left-aligned with a 16 px gap, just as on phones. Each stacked answer may wrap within the available width; never shrink the Judgment type to fit. Below, the tag "missed" and the one-line why. Keys: Enter for next.
- **Why? (close and missed only):** under the verdict, a quiet bordered "Why?" control (44 px, never the accent; reached by Tab and Enter, or a tap). Nothing is explained until it is pressed. While waiting it reads "explaining" in mono. The explanation then appears in place as body text at the reading measure: one or two sentences, at most 280 characters, saying what differed between the learner's answer and the right one. It never changes the tag, the why, the schedule or the history. If no explanation can be had (provider error or timeout, none configured, allowance reached), the criterion sentence is shown with a meta line "A fuller explanation isn't available right now." Right answers and exact texts have no "Why?"; an exact text's word diff already says what differed.
- **Exact texts** (texts the learner marked exact) are judged word for word in place: correct words stay plain ink, missing words are inserted and highlighted, wrong words are struck in ink-muted with the right word set right after them. A mono label above names the text and line ("Exact text · Nicene Creed · line 1"). The why counts the differences in words ("One word missing, one swapped."). Actions: "Again" (text button) and "Next" (accent).

### Session end

The scrollback clears to one statement at Display size: "Done." Under it, one line: "11 reviewed, 2 to revisit tomorrow." Under that, a quiet text link: "Learn something new". No charts, scores or celebration.

### Library and concept links

Two panes split by a hairline on wide screens; stacked on phones.

- **Left:** a numbered outline of ideas in mono numbers and text titles, like a table of contents. The open idea has an accent rule at its left edge; sub-ideas nest one level.
- **Right:** the idea page. Title, a one-line summary, then its links: a small diagram of the idea and its related ideas, drawn as thin accent lines to hairline-boxed labels (never a glowing node graph), each label a link. Then "Items", ruled rows of questions, each with a mono due label ("due today", "in 6 days").

States: empty library (the starter shelf), an idea with no links (the diagram is omitted, not drawn empty), search results (same rows, matched words highlighted).

### Inbox

Suggestions from the learner's agents. Header "Inbox" with a mono count ("7 suggestions") and "Approve all" as a text action at the right. Dense ruled rows: the suggested question in body text, under it a mono source line ("from reading agent · The Federalist Papers"), the idea it would join at the right, then approve (accent, 4 px square control) and dismiss (quiet). The focused row is raised and shows "A approve · D dismiss". No icon sidebar. States: empty ("Nothing waiting."), approving (the row collapses upward), approved all.

### Learn something new, and a book's core ideas

One prompt field at Display size: "What do you want to know?" with a mono hint underneath ("A topic, a question, or a book's title"). The result arrives as a draft outline: ideas with their items, laid out like the inbox, each approvable on its own or all at once. Nothing enters the library until approved. States: writing (a mono "drafting" line with the ideas appearing as they are ready), draft ready, nothing useful found (said plainly, with a suggestion to narrow the topic), allowance reached (says so and links to Billing).

### Billing and allowance

A ledger page. Top: the plan and price in one sentence. The allowance as one sentence ("You've used 62% of this month's allowance. It resets on December 1.") with a thin 2 px meter under it, accent on a line-colored track. "Top up" is one button with its price on it ("Top up · $5"); when four top-ups are used this UTC month it is disabled with "Monthly $20 top-up limit reached. Resets <date> UTC." A top-up still being confirmed says so and that it is charged at most once. Below, a ruled history of charges and top-ups with dates and amounts in tabular figures. Cancel is a plain text link with a one-step confirmation that states the end date. States: on the no-card trial (its end date and days left, "nothing is charged", "Add a card · nothing charged today" as the one accent button, the allowance as a share of the trial's $0.50, "Top-ups need a card on file." in place of the Top up button, and no Cancel link: there is nothing to cancel), trial allowance reached ("Add a card to keep going."), no-card trial ended ("Add a card · $5 a month"), on trial with a card (trial end date and the first charge), active, allowance near its limit (the sentence says so; no color alarm), allowance reached, card failed (the reason and "Update card"), ended (no allowance sentence and nothing said to be spendable; leftover top-up credit is said to be kept for when the plan restarts, matching Study and Learn, which need an active plan).

## Keys

Outside text fields: g s Study, g l Library, g i Inbox, g n Learn, g b Billing; / focuses search where shown. Escape leaves a text field so navigation keys can be used without changing the answer. In study, Enter checks the answer or continues to Next; Space starts or stops listening (on an empty answer line, or outside fields); A repeats an exact text when Again is available. Focused controls retain their normal Enter and Space actions. Hints use Micro in the existing footer and study hint line.

Top up, cancel, + Add, item rows, Settings and similar actions use Tab and Enter; they do not need dedicated shortcuts. Existing inbox and draft row keys remain: J/K or arrows move, A approves, D dismisses.

## Phones

Everything stacks into one column. The study session keeps the scrollback with two receded items above and one below. Judgment size drops to 72 px. Key hints are hidden; the mic and Check are thumb-reachable at the bottom. Nothing scrolls sideways.
