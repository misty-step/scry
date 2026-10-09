# One question and honest judgment

Stories: US-002, US-003, US-007, US-008, US-011

Source: src/learning.rs, src/engine.rs, src/generation.rs, src/web.rs, src/runtime.rs, assets/app.js

## Sub-features

One question or prerequisite intro, exact/meaning checks, assistance/self-check,
held feedback, deliberate Next, immutable correction, and exact response replay.
## How to get to it (user POV)

Open `/` directly into a question, prerequisite introduction, or honest empty
state. Choose one `.choice` inside `fieldset.choice-fieldset`, or type into
`#answer` in `form.answer-form`. Choice taps submit directly. Introductions have
Start practicing and I know this already, both observations rather than cold
review. The current question survives navigation and refresh.

Choice/exact-key/authored-variant grading is local. Other recall answers stage
one bounded independent Jev `short-v1` check; authored explain prose uses
`semantic-v1`. Close, unsure, malformed, or failed checks preserve the answer
and offer self-check; failed checks offer an explicit new check attempt. The
held `role=status` result names authority and keeps expected answer/explanation
until `form[data-next]`. Nothing auto-advances.

`details.overflow` has Show me, Look it up, repair, and archive. Automatic misses
offer I was right below Next; automatic correct grades offer Count as a miss.
Corrections preserve original attempts and assistance. `/history` keeps the
wording/result as presented and identifies later correction. Reference and edit
pages cannot preload protected answers while an attempt remains unaided.

## Driving it

`cargo test --locked --test learning_policy --test fsrs_golden --test application --test web_render`
covers policy, 260 pinned scheduler trajectories, exact-operation replay,
immutable corrections, and HTML fences. A real local Worker browser exercises
answer → held result → Next, refresh, response loss/reconcile, offline drafts,
keyboard, pointer, self-check, and access loss. No-JS forms use the same durable
POST contract. The controller keeps only one unresolved frozen payload and does
not store private history or queue mutations offline.

The authored fixture is HTTP caching material in a fresh synthetic namespace;
no provider endpoint yields honest failed/self-check handling. Neither fixture
answers nor scheduler compatibility establish real Jev holdout quality, useful
learning, private ingress, or physical-phone acceptance.

## Gotchas

Synthetic data proves mechanics only. Follow the current QA recipe, retain
source-bound real observations, and keep protected reference/answer material
behind durable assistance. Never attach live integrations or import historical
stores to make a preview appear complete.
