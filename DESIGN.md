# Scry: the curiosity notebook

The Rust rewrite treats Scry as a private notebook that grows through use.
Create begins with curiosity, not an import workflow or an assessment. A useful
explanation and practice come first; answers and explicit feedback shape what
comes next. [VISION](VISION.md), [USER_STORIES](USER_STORIES.md), and
[S11 in SPEC](SPEC.md#s11--create-useful-material-from-intent-us-013) govern the
experience. This design is implementation direction, not deployment or
physical-phone acceptance evidence.

## A quiet page with a clear next action

Warm paper, dark botanical ink, and cobalt replace the previous cool card
surfaces. The page has generous margins, editorial reading typography, simple
hairlines, and small square radii. It should feel like a place to think, rather
than a dashboard to operate. There is no progress ring, streak, accuracy badge,
notification feed, or always-visible due count.

The masthead contains the Scry wordmark and exactly two destinations: Create
and Map. The wordmark returns to the held question or result. History and
Settings live in the quiet footer. The reading column is at most 680 px;
question practice narrows to 650 px. The outer page frame is 1080 px.

Scry's mark is four connected points inside a fine circle. One point opens into
a small star. It suggests curiosity becoming connected understanding. The Map
repeats the language as one night-sky constellation per goal. The constellation
is decorative; the ordinary linked concept list carries all useful information.
Prerequisites determine horizontal depth. Actual observed practice determines
brightness. Generated connections remain proposed relationships, not mastery.

## Color and typography

| Role | Day | Night |
| --- | --- | --- |
| Paper | `#f5f2ea` | `#191e1d` |
| Writing surface | `#fcfaf5` | `#212725` |
| Primary ink | `#242721` | `#e9e8dd` |
| Secondary ink | `#565b51` | `#c0c4b8` |
| Quiet ink | `#687063` | `#a0a89a` |
| Hairline | `#deddd2` | `#363d36` |
| Control boundary | `#b7bcae` | `#596453` |
| Cobalt action | `#344cc4` | `#a5b3ff` |
| Recall | `#286749` | `#98c9a6` |
| Miss | `#a64031` | `#efac97` |
| Helped practice | `#74538a` | `#cdb3e0` |
| Constellation | `#232c2b` | `#111916` |

State is always stated in words. Color reinforces its meaning. Cobalt means an
action the learner can take; green, red pencil, and violet describe observed
practice. The night scheme follows the system preference.

Literata sets titles, questions, notes, explanations, and written answers.
Atkinson Hyperlegible Next sets navigation, controls, provenance, and practical
copy. Both are self-hosted variable WOFF2 fonts, copied with their SIL Open Font
License to [assets/fonts](assets/fonts/). No third-party font request occurs.
Literata italic supplies the held result's quiet verdict. Desktop page titles
reach 64 px, mobile titles 44 px, and questions scale between 30 and 52 px.
Reading prose uses 16–19 px and generous line height. Long text wraps instead
of breaking the viewport.

## Create and the useful first result

Create asks, “What do you want to learn?” One large writing surface accepts a
word, phrase, ramble, pasted material, or dictated text. There is no mode picker,
profile questionnaire, or quiz before preparation. An optional disclosure
accepts a photo. The privacy note states that private words prepare material and
pasted links do not trigger browsing. Text and photo limits are explained;
validation retains the editable draft.

The first result is a saved reference: a plain explanation for each teachable
idea, relevant examples and distinctions, provenance, and linked practice.
Captured intent remains inspectable under “Your original request.” General
knowledge says so; a request is never presented as a factual citation.

Preparation gets a saved receipt. A pending result never claims to be ready.
Existing practice remains available while preparation runs. Partial material is
labeled partial. A failed or uncertain outcome preserves the request, explains
what is known, and offers a deliberate bounded new attempt. Paid work is never
silently resent because a browser poll or refresh occurs.

A goal's refinement section asks what would make the material more useful. A
concept asks what is still unclear. Saved feedback is inspectable alongside its
reference. Refinement uses attempts and feedback while preserving existing
notes, wording, and learning history.

## One question, one action, a held result

Returning study goes straight to the current question or a short prerequisite
introduction. The question is the page's single h1. A choice tap submits that
choice directly. Recall has one serif field and Check my answer. More contains
Show me, Look it up, content repair, and archive. Keyboard numbers select
choices; Enter submits written recall on fine pointers, Shift+Enter keeps a
newline, and Enter or Space chooses Next outside a focused control.

A new idea has a saved note and two deliberate observations: Start practicing
and I know this already. Reading and either observation are exposure, not
unaided recall. Assisted practice says so.

The answer and explanation are absent from HTML before grading, self-check,
or recorded assistance. That fence covers the current concept's name and
notes, the goal's title and original intent, edit fields, and earlier attempts
that could reveal the answer. Opening a reference first presents an assistance
gate; its POST durably records help before returning answer-bearing material.

A held result keeps the prompt, submitted answer, expected answer, explanation,
and decision authority. It advances only on Next. Automatic misses offer I was
right; automatic correct results offer Count as a miss in More. Corrections
remain separate records and keep assistance attached to the original attempt.
Uncertain checks present a self-check without inventing a grade. Failed checks
also offer an explicit retry. The learner check retains whether the submitted
attempt had help; reading the exposed answer makes the next practice helped.

On a phone, unanswered study stretches enough to put answer controls in the
thumb zone. The result's Next and grade correction, self-check decisions, and
introduction action stay at the bottom as the learner reads. Desktop controls
follow the prose naturally. All core touch targets are at least 44 px, and
primary/choice controls are larger. Narrow 320 px layouts retain ordinary
flow without horizontal scrolling.

## References, repair, and honest records

Map groups goals as readable sections, active before paused, focused first.
Each has its constellation, concept states, Focus and Pause, and a reference
link. Concepts expose current notes, prerequisites, practice, and authored
questions. Recall is explicitly a scheduling estimate, never a certainty or a
measure of understanding. Unaided, helped, and missed attempts have separate
lifetime counts and an accessible recent tally. The estimate comes from the
same pinned learning policy used by the durable scheduler.

Edits carry the current content version and preserve historical presentations.
A prepared fix is a saved draft the learner can inspect, alter, and deliberately
accept. It does not replace a question merely because model preparation
finished. Faulty-material feedback can request a deliberate schedule reset;
it remains distinct from changing an answer's grade. Archive stops future
selection and retains history.

History preserves the question as presented, original answer, authority,
assistance, and later correction. Settings offers the 3/6/12 new-idea rhythm,
rolling preparation allowance and uncertain reservations, verified backup
status, deliberate backup, portable export, and the font colophon. Recovery
attention is stated plainly. An isolated synthetic preview carries a clear
badge and alone offers an authored fixture; production never offers fixture
seeding or presents authored material as model-quality proof.

## Browser, privacy, and access

Every mutation is an ordinary same-origin POST with CSRF and a unique
cryptographic-render-nonce-bound operation ID. Core study, creation, reference,
and repair work without JavaScript. The external vanilla controller enhances
those forms; it owns no durable learning state and stores no private content in
browser storage.

A successful response reconciles the rendered page. A definite validation
rejection retains the learner's draft. If a response is lost, one frozen payload
and operation stay unresolved, with explicit Reconcile this attempt and Open
saved progress actions. Reconciliation sends the identical operation; it cannot
invent a second answer. Offline study pauses. Reconnecting does not send a
queued mutation. Only one unresolved mutation is permitted.

Preparation polling is read-only and pauses while a field is being edited or
an answer is unresolved. It never replaces an unanswered question or initiates
paid work. A late response cannot revert a newer rendered stage.

Access denial removes the private page. Pagehide removes private content from
the retained browser page; restoration from back/forward cache reloads through
the access boundary. The server must also enforce private, no-store responses.
All user and model text is escaped. The policy uses external scripts/styles,
self-hosted fonts, and no inline executable markup.

## Accessibility, motion, and evidence

Each page has one h1, semantic landmarks, a focused skip link, native forms,
visible focus, text labels, and honest polite status announcements. Decorative
SVG is hidden from assistive technology. Ordinary links and disclosures expose
all reference and maintenance controls. Forced colors retain control borders
and focus. Reduced motion gets no transitions or animations. Other motion is
limited to a pressed control and a short arrival after a learner action;
there is no ambient animation or automatic advance.

[The Rust renderer](src/web.rs), [styles](assets/app.css), and
[presentation controller](assets/app.js) implement this system.
[Web rendering tests](tests/web_render.rs) verify the protection fence,
escaping, operation identity, native form contract, synthetic boundaries,
held results, and versioned repair. Browser acceptance must separately exercise
320/390/1280 px layouts in both schemes, keyboard and actual pointer actions,
o-JavaScript forms, held feedback, assistance, competing tabs, offline pauses,
response loss, preparation recovery, and access loss. A rendered screenshot or
synthetic pass establishes appearance or mechanics, not usefulness, physical
phone acceptance, production activation, or learning gains.
