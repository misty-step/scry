# Scry Product Vision

Status: prompt-first product intent reaffirmed on 2026-10-02. Create starts
with what the learner wants to understand, not a required import or setup
questionnaire. This is the governing goal, not a newly invented product.

The [stories](USER_STORIES.md) connect that goal to [Create acceptance in
SPEC](SPEC.md#s11--create-useful-material-from-intent-us-013). The
[runbook](docs/runbook.md) alone owns deployed state and release evidence.
Approved intent, implemented behavior, and production acceptance are distinct.

## The product

Scry is a private, prompt-first learning app for one person. Choose Create,
enter a word, phrase, or a long dictated ramble about what you want to learn,
and receive useful tailored study material: an understandable reference and
practice connected to teachable concepts. Useful material comes before a
mandatory questionnaire or diagnostic quiz. Source text, links, and photos
can support the request; importing material is not the required front door.

The first result is a useful starting point, not a claim to know the learner.
Actual attempts, expressed needs, and feedback refine subsequent material and
practice. “Quiz-first” describes retrieval-led ongoing study, not a quiz gate
before creation or a restriction to disposable question cards. Notes remain
revisitable; reading, revealing, and assisted work never masquerade as unaided
recall. Generation may take time or fail; saved input and honest pending,
partial, failed, and uncertain states matter more than an instant-result promise.

The desired outcome is understanding and later recall, not collection size,
engagement time, an accuracy badge, or a general learning platform. The operator
reported no active users at the rewrite decision; fixture and deployment success
are not evidence of sustained use or learning gains.

## Principles

1. Curiosity to useful material with no compulsory setup; then one question,
   one answer action, held result, deliberate Next. The current Go masthead
   uses Add and Map; Create is the approved journey, not a shipped label change.
2. Concepts are the navigable unit. A goal is what I want to know; a note teaches
   one idea at a chosen depth; questions test it. Related ideas and prerequisites
   are explicit, but their generated links are claims, not proven mastery.
3. My input keeps its provenance and privacy. A topic or request is intent,
   not factual evidence. Current Add text is private and never sent to web
   search or opened as a link by inference. A photo is transcribed. Retained
   explicit Topic/Link callers have their bounded research paths. A source
   quote must match the source; a web quote must match a saved excerpt and cite
   it; otherwise label it “General knowledge.”
4. Grade honestly both ways. The exact key and authored variants resolve locally;
   every other short answer uses one bounded Jev check; explain-level prose retains
   its authored rubric. Close/unsure/failed checks invite self-check, and an
   automatic grade can be corrected with one tap. Every grade names its
   authority. Never accept a different fact through broad text similarity.
5. Introduce unseen concepts in prerequisite order and vary useful practice;
   distinguish exposure, helped practice, misses, and unaided attempts.
   Personalize from those observations and explicit feedback, not learning-style
   labels or invented mastery. The Map shows a labeled recall estimate.
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

The retained [concept-centered round](docs/design/concept-centered-study.md)
and [learning-science synthesis](docs/science/README.md) explain concepts,
references, retrieval, spacing, and their evidence limits. Their dated proposals
and earlier interface rounds remain provenance, not competing Create contracts.

Returning study opens into a useful question or a clear creation invitation.
Creation yields an initial explanation/reference and practice without first
requiring a learner profile. Before a new concept's first question, offer a
short note and an honest “I know this already” observation. If it remains
confusing, its note is one tap away after grading without consuming the
occurrence. Preparation can take time without hiding existing study. When
caught up, show what is next rather than inventing work.

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

**Implemented runtime:** Go/SQLite renders private HTML/HTMX with a small
presentation controller in one writer behind Cloudflare Access and
`scry-app-host`. The Go Add screen accepts private text or an optional photo;
that capture path is not end-to-end proof of the approved Create/refinement
contract. The audience remains the operator, primarily on a phone.

**Approved future direction:** a ground-up Rust-on-Cloudflare rewrite. It is
not shipped, not the retired Rust stack restored, and not permission to deploy
an obsolete imported Go build. Storage, speech integration, and replacement
activation are not selected by this documentation.

**Recovery remains binding:** Container storage restores from private R2;
daily/pre-release backups and 30-day retention target RPO 24 hours and RTO
60 minutes without guaranteeing either. Hosted Scry has no local-only backup
exception. Old Rust stores and the retained exe.dev VM remain recovery material,
never simultaneous writers.

No public signup, billing, collaboration, universal import, course generation,
chat product, manual graph editor, or offline mutation queue is required. The
foundation detour was rejected on 2026-09-12 and its routes are retired in v5;
old rows survive the additive migration but do not appear as newly generated
concepts. No live schema migration or activation is authorized by this document.
[QA](docs/qa/system.md) separates synthetic, private-browser, provider, and
independent-restore evidence.
