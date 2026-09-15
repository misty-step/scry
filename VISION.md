# Scry Product Vision

Status: adopted personal Go/SQLite/HTMX product direction. The operator approved
the initial phone flow on 2026-09-09. [SPEC.md](SPEC.md) owns stories and
acceptance; [the runbook](docs/runbook.md) owns deployed origins and recovery.

## The Product

Today Scry is a personal, quiz-first learning app. It turns something I want to know
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

## Approved Next Direction

The approved next direction is a proactively managed personal learning map.
Chosen goals define the knowledge to cover comprehensively, not just concepts
already found in generated cards. Quizzes serve retrieval/practice and provide
observations alongside durable instructional and reference material. This
direction remains unimplemented, but on 2026-09-10 the operator explicitly
authorized specification, full implementation, and protected shipping. New
[KC01–KC19 acceptance criteria](SPEC.md#knowledge-centered-implementation-acceptance)
govern that work; the initial S01–S10 meanings and historical receipts are
unchanged. Existing spend, private-access, live-history preservation, recovery,
and deployment safeguards still apply; this is not evidence of a shipped engine.

Foundations remain represented for beginners and experts. Evidence and uncertain,
task-dependent estimates guide exposure, not deletion of basics or a permanent
expert flag. Missing map coverage, unknown knowledge, and observed gaps are
different problems. Independently meaningful atomic targets and composition/
application both matter. Foundation completeness, demonstrated capability, and
future recall probability are distinct, not one score that can hide a basic gap.

The operator reported introductory biology material that was too advanced; this
is a design driver, not a revised QA result. Generate or reuse durable bridge
material and reference resources proactively at capture and when gaps emerge.
Goals and available time govern useful instruction, probes, retrieval, and
composition; library presence does not make every item immediately due. The
[approved design](SPEC.md#approved-knowledge-centered-design-unimplemented)
owns the data relationships, evidence limits, and direct-review baseline.

Instruction, reference text, structured diagrams, and assessment material are
durable resources. Actual external article/video references retain provenance;
a reference is not an invented transcript, fetched source, or generated video.
The map supports advanced and lateral suggestions as well as bridges, aligned
with chosen goals rather than silently adding new ones. Goals can be organized
into multi-scale learning horizons—daily retention sweeps, Pomodoro acquisition
sessions, and quarterly immersion campaigns—allowing deep focus without dropping
foundational maintenance. One interaction can inform several uncertain estimates
without becoming several reviews. Current
quiz-owned FSRS remains the direct-review baseline; cross-item selection has
explicit policy provenance and must earn effectiveness claims through later
evidence, not functional checks.

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

The next direction preserves one learning moment and deliberate Next on the
phone. Proactive planning belongs backstage; meaningful changes to learning
focus, material, or schedules must be visible and reversible without rewriting
history. No chat surface is required. Chat may later be another way to use the
same knowledge and evidence, not a prerequisite for the learning loop.

From a Too advanced action, useful foundation instruction and practice should
lead to a deliberate return to the retained target. Pacing and resumable bridge/
plan state keep that detour useful; model work must not block ordinary review or
blur the difference between pending and saved work when connectivity fails.

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

The implemented initial scope is capture, generation, review, correction, library,
learning history, private access, and recovery. Public signup, waitlists,
billing, collaboration, generalized import, full course generation, a chat
product, and an offline synchronization engine are not initial requirements.
These initial exclusions do not reject the approved knowledge-centered direction
above or authorize multi-user infrastructure, historical imports, or operational
changes.

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
