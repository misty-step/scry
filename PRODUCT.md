# Scry

Scry gets what's worth knowing into your head and keeps it there, for the fewest minutes a day. It is built for Phaedrus and usable by anyone.

## User stories

- As a newcomer, I can find Scry, sign up and sign in through Clerk with email and password, and be studying within a minute: the 7-day trial starts at once, with no card up front.
- As a learner on the trial, I always know how many days are left, and I can add a card any time in Billing or on the card step to keep Scry after it; adding one during the trial charges nothing until the trial ends. When the trial ends or its allowance runs out, Scry says so calmly and shows me where to add a card.
- As a newcomer, I must verify my email before I can study or use billing.
- As a returning learner who forgot my password, I can reset it by email and sign in with my new password.
- As a new user with an empty library, I can pick from about 30 excellent starter sets (kitchen numbers, the NATO alphabet, the Creeds, core ideas in every major subject, vocabulary, mental models) and start right away.
- As a learner, I can ask Scry for material on any topic and get it, shaped to me.
- As a learner, I can turn a favorite book into a set of its core ideas.
- As a learner whose agents do work for me, my agents can add material to my library or suggest items from my conversations and reading. Suggestions wait in an inbox until I approve them with one tap.
- As a learner, I open Scry, get what's due, answer by typing or speaking, and I'm done in about 5 minutes.
- As a learner, I never grade myself. Every non-exact answer is graded correct, almost correct or incorrect on understanding, not case, punctuation or trivialities, by a decisions-model grader. Every answer is judged instantly with a one-line reason why: the criterion the grader applied. When an answer is almost correct or incorrect, I can ask "Why?" and get a short explanation of what differed; asking never changes the grade. Texts I mark exact ignore case, punctuation and apostrophes, then are judged word for word. Near misses show a word-by-word diff.
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
