# Scry Product Vision

Status: concept-centered v5 direction authorized by the operator on 2026-09-23:
“Execute this reimagining in full” (MIS-162). This supersedes the rejected
foundations experience, not its preserved data or the earlier phone-flow proof.
[Stories](USER_STORIES.md) own the learner contract; [SPEC](SPEC.md) owns behavior;
[the runbook](docs/runbook.md) owns deployed state and release approval. This
source change does **not** authorize production activation or claim that v5 is live.

## The product

Scry is a private, quiz-first way to learn what matters to one person. Add a
topic, your own text, a link, or a photo. Scry makes a goal, maps a few teachable
concepts, writes notes that can be revisited, and asks questions worth returning
to. Recall attempts and explicit learner judgments shape future practice; a
note read or revealed answer never masquerades as independent recall.

The desired outcome is understanding and later recall, not collection size,
engagement time, an accuracy badge, or a general learning platform. The operator
reported no active users at the rewrite decision; fixture and deployment success
are not evidence of sustained use or learning gains.

## Principles

1. One question, one answer action, held result, deliberate Next. Add and Map
   stay in the masthead; maintenance is available without overwhelming study.
2. Concepts are the navigable unit. A goal is what I want to know; a note teaches
   one idea at a chosen depth; questions test it. Related ideas and prerequisites
   are explicit, but their generated links are claims, not proven mastery.
3. My input keeps its provenance. Only an explicit Topic choice starts Exa web
   search; a Link fetches its chosen page, a Photo is transcribed, and pasted
   text is never sent to search. A source quote must match the source; a web
   quote must match a saved excerpt and cite it; otherwise label it “General
   knowledge.”
4. Grade honestly both ways. The exact key and authored variants resolve locally;
   every other short answer uses one bounded Jev check; explain-level prose retains
   its authored rubric. Close/unsure/failed checks invite self-check, and an
   automatic grade can be corrected with one tap. Every grade names its
   authority. Never accept a different fact through broad text similarity.
5. Introduce unseen concepts in prerequisite order; distinguish exposure,
   helped practice, misses, and unaided attempts. The Map shows status and a
   labeled recall estimate, not a declaration of knowledge.
6. Keep history, privacy, bounded spending, and recovery true even through a
   refresh, competing tab, outage, failed generation, or deployment. Paid work
   and unknown usage are not retried invisibly.

## Experience direction

The operator's smoothness bar is immediacy and continuity, not a compulsive feed.
The [Ink notebook design](DESIGN.md) (2026-09-24, replacing the rejected
“Scrying glass”) treats Scry as a study notebook. It has a reading serif for
questions and notes, and a legibility sans for controls. On a phone the
controls sit in the thumb zone, and each goal's star chart is the one
ornament. Color marks the learner's action, recall, and misses. Day and night
schemes both apply, and motion happens only in response to a learner action.
It is a design decision, not a claim that the earlier phone approval covered
this experience.

Open into one useful question or a clear Add invitation. Before a new concept's
first question, offer a short note and an honest “I know this already” observation.
If the concept remains confusing, its note is one tap away after grading; navigating there must not consume the occurrence.
Prepare work can take time without hiding the existing Stream. When caught up,
show when something is next and permit adding more, rather than inventing work.

## What must remain true

- Answer, recorded outcome, schedule, and idempotent operation agree durably.
- A reveal, cue, or note read cannot become an unaided success; immutable past
  presentations remain inspectable even after edits and grade overrides.
- Generated content quotes exactly, renders as escaped text, and does not claim
  source support from a topic word or fabricated citation.
- Captured input survives failure; uncertain paid calls retain reservations and
  require deliberate resolution. The provider key is capped at $25/week; the
  application allows $3.50 per rolling day and conservatively reserves $0.50
  per generation attempt.
- A short smoke run establishes mechanics, not usefulness, model calibration,
  delayed recall, or production activation.

## Scope and runtime

The audience is the operator, primarily on a phone. Go/SQLite renders private
HTML/HTMX with a small presentation controller in one writer behind Cloudflare
Access and `scry-app-host`. Container storage restores from private R2; the
approved daily/pre-release backup and 30-day retention targets RPO 24 hours and
RTO 60 minutes without guaranteeing either. The old Rust stores and retained
exe.dev VM remain historical recovery material, never simultaneous writers.

No public signup, billing, collaboration, universal import, course generation,
chat product, manual graph editor, or offline mutation queue is required. The
foundation detour was rejected on 2026-09-12 and its routes are retired in v5;
old rows survive the additive migration but do not appear as newly generated
concepts. Further activation of schema v5 on live data needs separate explicit
release approval. [QA](docs/qa/system.md) separates synthetic, private-browser,
provider, and independent-restore evidence.
