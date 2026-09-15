# Scry Product Vision

Status: adopted personal Go/SQLite/HTMX product direction. The operator approved
the initial phone flow on 2026-09-09. [SPEC.md](SPEC.md) owns stories and
acceptance; [the runbook](docs/runbook.md) owns deployed origins and recovery.

## The Product

Scry is a personal, quiz-first learning app. It turns something I want to know
into useful questions and brings those questions back when reviewing them is
worthwhile. AI helps create and improve the material; my attempts drive the
learning history and next review.

The desired outcome is knowledge I can recall, not a larger card collection,
a longer session, or a reusable learning platform. The interface should take
almost no effort to operate while leaving room for the effort of remembering.

At the rewrite decision, the operator reported no active users, including the
operator. That is the adoption baseline. Phone acceptance, generated fixtures,
passing checks, and historical production receipts are not evidence of sustained
use or learning gains.

## Experience Direction

The operator's bar is **TikTok-level smoothness**, with a simple interface and
intentional aesthetics. Borrow immediacy, clear focus, and continuity, not
engagement-maximizing mechanics.

The accepted initial experience has one learning moment at a time:

1. Open directly into something useful to review, or one clear way to add it.
2. Add a word, a goal, or pasted material without configuring a project or deck.
3. Receive useful AI-generated quizzes with honest progress and recoverable
   failure, rather than manage a generation pipeline.
4. Answer or ask for help. Understand the result without losing the question.
5. Continue deliberately; return later without losing committed progress.
6. Fix, remove, or inspect material without making maintenance the main product.

The initial phone flow is approved, not a permanent freeze on design. Changes
to question styles, gestures, visual direction, or ambiguous-answer behavior
should follow real use and renewed operator review.

## What Must Remain True

- Generated knowledge is not falsely presented as a quotation or verified fact.
- Revealed or otherwise answer-assisted work is not recorded as unaided recall.
- An answer, its recorded result, and its schedule change agree durably.
- Retried requests do not manufacture additional learning events.
- Content correction does not silently rewrite what was previously presented.
- Failed generation does not lose captured input, invent success, or spend
  without a bound.
- Learning material remains private and recoverable.
- UI responsiveness, content quality, voluntary use, and delayed recall are
  different claims requiring different evidence.

Carry these lessons forward without treating the old implementation's exact
policies, public APIs, crate boundaries, or fixtures as a parity requirement.

## Scope

The initial audience is the operator. The phone web experience is primary.
Agent capture or a CLI can be added when an actual personal workflow needs it;
five independent product faces are not a launch obligation.

The current scope is capture, generation, review, correction, library,
learning history, private access, and recovery. Public signup, waitlists,
billing, collaboration, generalized import, full course generation, a chat
product, and an offline synchronization engine are not initial requirements.
Their exclusion is not a permanent ban if later use justifies them.

## Technical Direction and Current Runtime

One Go application owns SQLite on persistent disk, server-rendered HTML with
HTMX, and a small browser-side interaction layer on exe.dev. Cloudflare provides
private off-VM R2 recovery through a narrow append/read gateway; it is not the
interactive application runtime.

The old Rust application and five-face implementation are retired. Its Worker
stores, frozen native Postgres, backups, and compatible historical source remain
recovery material, not alternative live writers. Current operation and proof
belong in [the runbook](docs/runbook.md) and [QA guide](docs/qa/system.md).

No old QA data is imported and unused APIs have no compatibility requirement.
The operator chose to preserve old data and historical recovery separately.
New-app snapshots use the approved daily/pre-release, 30-day retention policy.

## Specification and Proof

[SPEC.md](SPEC.md) owns accepted user stories, stable acceptance identifiers,
experience budgets, architecture decisions, delivery slices, and the independent
QA contract. Linear owns accepted work, priorities, and execution state, not a
second copy of product truth.

The first product decision is whether the operator wants to keep using the
actual experience. A polished demo and green tests cannot answer that. Repeated
personal use and later unassisted recall should inform the next revision; do
not promise efficacy from a scheduler setting or a short smoke run.
