# Scry: design

Scry is for someone who wants what matters to stay in their head, for about five minutes a day. It should feel like a precise, warm instrument: closer to Warp and Supermemory than to a book or a game. [PRODUCT.md](PRODUCT.md) owns what Scry does. This file owns how it looks and feels.

Accepted by Phaedrus on 2026-11-17 (chat with Draper, shaped under [MIS-1210](https://linear.app/misty-step/issue/MIS-1210)). Session record, candidates and reasons: [design/sessions/2026-11-17-scry-design](design/sessions/2026-11-17-scry-design/decisions.md). The reference boards there illustrate the direction. The tokens and rules below are what bind.

## Principles

- **Words first.** Every state is said in words. Color, highlight and strike back the words up and never carry the meaning alone.
- **One thing at a time.** Each screen has one job and one main action.
- **Quiet everywhere, one hard moment.** Pages are calm and exact. The moment a miss is judged is the one place the type gets loud.
- **Keyboard first, touch equal.** Every action has a key and a 44 px touch target.
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
| close | `#EEF17C` fill, ink text | `#EEF17C` at 30% fill, ink text | Highlighter mark behind the words that were close or missing |
| missed | `#C23B26` | `#FF7A66` | The strike through a wrong answer and the "missed" tag |

The accent appears at most once as a filled control per screen. Mode follows the system setting, with a toggle in Settings. Contrast: body and meta text meet 4.5:1 in both modes; ghost is never used for text the learner must read.

## Shape, space and texture

- Surfaces are square (radius 0). Controls and fields are 4 px. Nothing is a pill.
- 1 px hairlines separate things; no shadows, no gradients, no blur.
- Spacing steps: 4, 8, 16, 24, 32, 48, 64, 96 px. Section padding is 56 px on desktop, 40 px on phones.
- Reading measure is at most 68 characters. The page frame is at most 1120 px.
- The study session sits on a faint 22 px dot grid (ink-ghost at about 6%) with short tick marks along its left and right edges. No other screen has a texture.

## Motion

- Fades and short moves of 150–250 ms, ease-out. Nothing loops and nothing waits on an animation.
- Two drawn lines: the strike through a missed answer draws across once (200 ms), and an idea's link lines draw in once when its page opens.
- Under reduced motion, everything appears in its final state.

## Wordmark

"scry" in Geist Mono, lowercase, ink. No glyph for now.

## Key screens and states

### Sign-up and trial

One centered column. Email and password, then the card step, which says in plain words: "7-day trial. You won't be charged until <date>. Cancel any time before then in Billing." The trial end date appears here and in Billing, never as a banner elsewhere. States: empty, typing, error in words under the field (for example "That email already has an account."), and card declined with the reason the processor gives.

### Starter shelf

Shown when the library is empty and reachable later from the library. Title "Start with something worth knowing." Below, a hairline grid of typographic tiles: no cards, no images. Each tile holds the set's title (Title), a mono item count, one line of description and a "+ Add" mono action. States: default; hover (sunken fill); added (accent hairline outline and the tag "Added"); adding (the tag reads "Adding…"). After the first add, the main action becomes "Start studying".

### Study session

The center of Scry. The session reads like scrollback:

- Answered items recede above the current one in ink-muted, one line each, with a mono verdict tag at the right edge ("right", "close", "missed").
- The current prompt sits in the middle at Display size, with the answer line under it and a block caret.
- Upcoming items fade below in ink-ghost, at most three.
- The left margin holds the idea's name and the count ("4 / 11") in mono. The bottom right holds the key hints: "enter to check · space to speak".

States: typing; checking (instant; no spinner unless a check takes over 400 ms, then the tag reads "checking" in mono); judged (see below); session end.

### Voice answer

Space or the single round mic control starts listening. Words appear on the answer line after listening stops, set exactly like typed text; transcription is requested once, not repeatedly while speaking. While listening, the answer line stays empty (or keeps any text already typed), its hairline gently swells, and a mono label reads "listening". States: listening, heard (text settles; Enter checks), couldn't hear ("Didn't catch that. Try again or type it."), microphone blocked (one line telling the learner how to allow it).

### The answer judgment

Instant, under the answer.

- **Right:** a green mono "right" tag and the one-line why in body text. The item moves up into the scrollback.
- **Close:** the parts that were close are marked with the highlighter, the tag reads "close", and the why says what differed.
- **Missed:** the moment the type gets loud. The learner's answer is set at Judgment size and struck with a missed-red line that draws across; the right answer is set beside it at the same size in ink. Below, the tag "missed" and the one-line why. Keys: Enter for next.
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

A ledger page. Top: the plan and price in one sentence. The allowance as one sentence ("You've used 62% of this month's allowance. It resets on December 1.") with a thin 2 px meter under it, accent on a line-colored track. "Top up" is one button with its price on it ("Top up · $5"). Below, a ruled history of charges and top-ups with dates and amounts in tabular figures. Cancel is a plain text link with a one-step confirmation that states the end date. States: on trial (trial end date and the first charge), active, allowance near its limit (the sentence says so; no color alarm), allowance reached, card failed (the reason and "Update card").

## Phones

Everything stacks into one column. The study session keeps the scrollback with two receded items above and one below. Judgment size drops to 72 px. Key hints are hidden; the mic and Check are thumb-reachable at the bottom. Nothing scrolls sideways.
