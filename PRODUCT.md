# Scry

Scry gets what's worth knowing into your head and keeps it there, for the fewest minutes a day. It is built for Phaedrus and usable by anyone.

## User stories

- As a newcomer, I can find Scry, sign up and sign in through Clerk with email and password, and be studying within a minute: the 7-day trial starts at once, with no card up front.
- As a learner on the trial, I always know how many days are left, and I can add a card any time in Billing or on the card step to keep Scry after it; adding one during the trial charges nothing until the trial ends. When the trial ends or its allowance runs out, Scry says so calmly and shows me where to add a card.
- As a newcomer, I must verify my email before I can study or use billing.
- As a returning learner who forgot my password, I can reset it by email and sign in with my new password.
- As a new user with an empty library, I can pick from about 30 excellent starter sets and start right away. The shelf leads with sets anyone can enjoy at once (the NATO phonetic alphabet, mental models, kitchen numbers, world geography), then core ideas in every major subject and vocabulary; the Creeds and the other word-for-word texts sit lower on the shelf. Adding a set confirms in place: the page does not reload or jump, and a "Start studying" bar appears.
- As a learner with an empty library, pressing Study never bounces me somewhere else without a word: Study says there are no cards yet and offers two ways forward, pick a starter set or create cards with AI.
- As a learner, I can tell Study from Create at a glance: Study is active recall of what is due; Create generates new cards with AI. In Create I can ask Scry for material on any topic and get it, shaped to my study profile.
- As a learner, I can turn a favorite book into a set of its core ideas.
- As a learner with my own scripts or tools, I can create an API token in Settings so they can suggest cards from my conversations and reading. Suggestions wait in an inbox until I approve them with one tap. New tokens always go through the inbox; a token made earlier that may add straight to the library keeps working until I revoke it.
- As a learner, I open Scry, get what's due, answer by typing or speaking, and I'm done in about 5 minutes.
- As a learner, nothing on the Study screen gives the answer away before I answer: while a card is waiting, the screen names only its topic and a neutral set number ("NATO phonetic alphabet · Set 1"), never an idea title or any label that holds a word of the answer to this card or to the cards beside it.
- As a learner, every answer I submit (correct, almost correct, incorrect, or an exact text) stops on a review of that answer before the next card: what I wrote beside the card's correct answer, a verdict in words (Correct, Almost Correct, Incorrect), the one-line reason, and "Next card" when I am ready. Scry never skips ahead on its own.
- As a learner, in that review I see this card's real schedule, read from what Scry has stored: when it is next due (date and relative time), its current interval, its stability and difficulty, and how often I have reviewed it and with what verdicts. These are facts, not scores: no streaks, points or celebration.
- As a learner, I can fix a card on the spot: "Edit card" in the review lets me change the question or the correct answer without leaving the session. The edit is saved to my library at once and applies from the next review; the grade just given and its schedule do not change.
- As a learner, I never grade myself. Every non-exact answer is graded correct, almost correct or incorrect on understanding, not case, punctuation or trivialities, by a decisions-model grader. Every answer is judged instantly with a one-line reason why: the criterion the grader applied. After any model-graded answer I can press "Ask Why?" and get a short explanation: what differed when it was almost correct or incorrect, or why it holds when it was correct. Asking never changes the grade or the schedule, and if no explanation can be had the reason is shown instead. Texts I mark exact ignore case, punctuation and apostrophes, then are judged word for word. Near misses show a word-by-word diff.
- As a learner, I can see how ideas connect, and each item belongs to an idea.
- As a learner, my library and history are mine alone, and nobody else can see them.
- As a paying user, I know what I pay and what my allowance covers, and I can top up or cancel any time.

- As an operator, production errors and health are watched continuously, and an agent is alerted, triages and proposes or ships the fix. A fault drill proves this works.

## Business rules

- No free tier. There's a 7-day interactive trial with no card required up front, guarded by a strict capped AI usage allowance (judging, speaking, explanations and drafting share it; no top-ups without a card). Access ends when the 7 days do. Upgrade any time in Billing or on the card step to the $5 a month plan: with a card added during the trial, the first charge is when it ends; in the trial's last hour or after it, the plan starts and is charged at once, with no second trial. Scry never charges today unless the page the learner submitted said so: if the trial reached its last hour (or ended) after the page was loaded, nothing is charged and the learner is asked to confirm the charge first. A declined card never shortens the trial.
- Prices cover costs plus a small margin, only to protect against abuse and runaway costs: a small monthly base with a capped allowance, and top-ups beyond it. Top-ups cost $5 each, up to $20 per person per calendar month (UTC); the limit resets at the start of each UTC month. One press of Top up is one charge, however often the form is resubmitted.
- Phaedrus is the first customer, with no special powers in the product.
- Reliable from day one: outages are noticed and fixed without anyone being asked.

## Feel

Calm, precise and warm, in the style of Warp and Supermemory. No game mechanics. Dark and light modes. The details live in DESIGN.md.

## Not now

Sharing, public decks, classrooms, streaks, importing from links, images or video, and predicting performance.
