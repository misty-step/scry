# Scry prompt-first, concept-centered specification

This is the single behavioral reference for rebuilding Scry: a private learning
notebook for one person, from curiosity to useful reference and practice, then
refinement through real answers and feedback. QA runs against this file's US,
S, and quality-contract IDs. They describe accepted intent, not implementation
or deployment success. Never weaken a criterion to fit code; change intent
through review, and retire identifiers explicitly rather than reusing them.

[VISION](VISION.md) explains the goal; [DESIGN](DESIGN.md) owns visual decisions;
[ADRs](docs/adr/README.md) own technical decisions; [QA](docs/qa/system.md) owns
verification procedures and evidence limits. [README](README.md#start-or-resume-work)
is the work entrypoint. Dated receipts and proposals are not competing requirements.

## Scope and acceptance status

Prompt-first Create was reaffirmed on 2026-10-02; the 2026-10-07 rewrite uses
these contracts without inheriting historical runtime acceptance. The current
Rust candidate ordinarily starts fresh; the separately authorized one-way
revival in D6 is the sole exception. US-001.1's Go schema 4→5 preservation and
US-005/S03's explicit-mode capture remain historical Go obligations at Git
`c6ba395`; US-012 is retired with its ID reserved. Old Go and retired Rust/preview
receipts do not certify the candidate, and a green gate does not activate it.
[The runbook](docs/runbook.md) owns operational records; serving provenance and
revival remain separately governed, not inferred from this specification.

Useful real material, short-v1 holdout quality, physical-phone experience,
deployed private ingress (including S09.2 account/history return), live provider
outcomes, and independent recovery still require their own evidence. The
existing rewrite acceptance stays open. No historical-store import, deletion,
writer reactivation, new paid capability, or deployment permission follows here.

No public signup, billing, collaboration, universal import, course/chat product,
manual graph editor, CLI/MCP parity, compulsory foundation detour, or invisible
offline mutation queue is required. Dictated input is text, not an assumed
speech engine or automatic recording integration. A mandatory activity ladder
is excluded from scope.

## Adopted behavioral policies

The D identifiers are retained for existing references; implementation choices
belong in ADRs, not this table.

| ID | Binding policy |
| --- | --- |
| D1 | Build around what the learner wants to remember, not old QA datasets. Sustained usefulness needs real use. |
| D2 | Honest grading and uncertainty follow S02 and US-003/US-008; policy adoption alone is not holdout calibration. |
| D3 | Assistance and immutable corrections follow S02/S05 and US-007; exposure is not cold recall. |
| D4 | One question, one answer action, held feedback and deliberate Next; Create/Map masthead. Visual decisions live in DESIGN; earlier phone approval is not blanket acceptance. |
| D5 | Daily/pre-release off-machine complete backups, 30-day retention, RPO 24h/RTO 60m targets (not guarantees). Provider key cap $25/week; application $3.50 per rolling 24h; conservative generation reservation $0.50. Generation, Jev and criticism share the allowance. Unknown sent cost retains its reservation and requires deliberate resolution, not free retry. |
| D6 | No legacy API parity. Ordinary candidates start fresh; the authorized 2026-10-15 revival alone imports the final consistent Go dataset once, with preserved learning history and media. Preserve historical stores, source and recovery artifacts separately; no general import, deletion or simultaneous historical writer is implied. [ADR 006](docs/adr/006-one-way-revival-custody.md) owns custody and cutover decisions, not an acceptance receipt. |

Health must report non-200 with reason `backup_stale` if no verified remote
snapshot exists or the last one is older than 26 hours. A separately authorized
machine trigger may request a backup, but cannot read learner data or act as
the owner. Trigger success acknowledges complete remote verification, not just
an attempted upload. Failed backups never advance the last verified time.

### Private runtime failure reporting

As the learner, I want an unexpected failure at the operated application
boundary to reach the existing Sentry/owned-triage route without exposing my
notebook or making study depend on the telemetry service.

- Unexpected request exceptions and upstream server failures identify the
  environment and deployed release; ordinary access denials are not incidents.
- Reports contain only a fixed failure category and deployment identity, never
  learner content, URLs, headers, credentials, raw errors or response bodies.
- Reporting is bounded and asynchronous. An unavailable or rejecting sink does
  not delay or change the application response; later reporting can recover.
- A controlled synthetic failure, its remote event and owned recovery are
  retrieved before claiming coverage. This does not certify uptime or cron health.

## Experience contract

The first screen is the learning moment, not a dashboard. Learning effort is
intentional; interface effort is not. An ordinary session should not require
choosing a deck, configuring a scheduler, approving drafts, reading telemetry,
or understanding the implementation.

The Stream opens into a question, a concept introduction, a preparing receipt,
or an honest first-run/caught-up state; never an administration dashboard.
Concepts are teachable ideas, goals express intent, and notes are durable
reference material, one current note per concept. The Map is the place
to inspect goals/concepts, focus or pause, and see a labeled estimate
from real observations, not a certified mastery score.

Creation precedes the study loop when starting a new interest: prompt → saved
preparation → useful reference and practice → refinement from real interaction
([S11](#s11--create-useful-material-from-intent-us-013)). A returning learner can
continue retrieval-led study directly. “Quiz-first” is not a compulsory pretest,
material-import workflow, or mastery profile before generation.

```text
scry                                    Create   Map
concept chip (inert before grading)
One question or first-time concept introduction
One answer control OR Start practicing / I know this already

checking → self-check (if unsure/close/failed) → held result
Correct. / Not quite. / Shown.
expected answer · explanation · citations
Next
I was right (quiet, automatic miss only)
```

- One choice tap submits, or a recall field uses Check my answer; More offers Show me.
  `question`, `checking`, `self-check`, `result`, `intro`, `preparing`,
  `empty-first-run`, `caught-up`, and `conflict/error` are distinct states.
  Feedback stays until deliberate Next; no auto-dismiss, swipe grade, or
  ungraded answer/explanation preloading.
- Self-check is learner authority on Jev unsure or a failed or unavailable
  check. Failed check also offers Retry check using a new deliberate
  operation, not a duplicate send. An automatic miss offers quiet “I was right”
  under Next; automatic correct offers “Count as a miss” in More. Immutable
  attempt and correction both remain in history.
- Intro presents a standard note before an unseen concept's first question;
  “I know this already” records an observation without scoring a cold answer.
  While a question awaits an unaided answer, every concept it assesses opens
  behind the Look it up gate, and text or titles drawn from its own capture
  (source text, goal title, preparation receipts) are replaced on every route
  until assistance is recorded; a concept page links to
  its capture but never carries the capture's text. Navigate to a concept page
  without consuming a review occurrence.
- Create has one private intent field and optional photo (US-013/S11). Text,
  including URLs, never infers public search or link reading; photos transcribe
  before preparation. Saved goals expose captured intent, useful reference,
  practice, refinement, and honest failure. `/add` is an alias; text/URL prefill
  remains editable and private. Historical explicit Topic/Link callers belong
  to the archived Go revision, not a replacement public API.

- Map shows goal sections and an accessible concept list
  alongside a decorative constellation. Concept pages show status, separate
  unaided/helped/missed tally, labeled recall estimate, notes, related concepts,
  questions, provenance, and Practice. Status words and focus/pause never
  depend on color alone.
- Keep private browser controller presentation-only; without JS, forms and
  capture controls remain functional. Design tokens, responsive behavior,
  reduced motion, and accessible targets live in [DESIGN](DESIGN.md).

## End-to-end acceptance criteria

These are accepted behavioral criteria, not claims that each is implemented.
Dated **PASS**, **FAIL**, and **UNVERIFIED** observations belong to their
source/environment-bound receipts, including the [earlier Go acceptance
receipt](docs/qa/personal-go-cutover-20260910.json), not this prose. IDs are
specification identifiers, not tracker-generated issues; owning items link here.

### S01 — Open into useful learning

As the learner, I want opening Scry to put me into a useful learning moment so
that I can start without organizing the app first.

- **S01.1:** With eligible material, opening review shows a question and an
  actionable response control without an intermediate dashboard/deck choice.
- **S01.2:** With no material, the screen offers capture. With material but none
  currently due/eligible, it explains that state and offers a deliberate next
  action; it does not invent due work or silently reset schedules.
- **S01.3:** Returning from Create/Map/Concept preserves an unfinished occurrence
  or held result. Navigation and status reads do not consume it.

Proof: browser journeys through empty, due, exhausted, and returning states.

### S02 — Answer, understand, and continue

As the learner, I want a clear recall attempt and useful feedback so that each
interaction teaches me something without taking away control.

- **S02.1:** A selected option refers to the exact displayed choice, including
  after order changes. Supported typed variants pass without accepting a
  meaning-changing answer merely through broad string normalization.
- **S02.2:** An answer produces the submitted/expected response, understandable
  outcome and concise explanation. The question and feedback remain until
  explicit Next. An unjudgeable response stays visibly ungraded rather than
  being confidently labeled wrong or silently advancing its schedule.
- **S02.3:** Reveal or answer-bearing help marks the occurrence assisted before
  it can count as a success; refresh, another tab, or a later correct answer
  cannot turn that occurrence into cold recall. Recognition and cued recall
  remain distinguishable in the learning record.
- **S02.4:** Next advances deliberately; Back, refresh, canceled gestures, and
  read-only requests do not fabricate answers. If no next item is ready, the
  learner sees an honest end/pending state.
- **S02.5 (US-003/US-008):** Content versions distinguish short recall and
  explain-level prose; there is no answer-form setting. Choices, the exact key,
  and authored variants resolve locally. Every other short recall answer stages
  one bounded Jev `short-v1` battery, whose identity judgment reads from the
  prompt whether an exact value or form is required, and accepts only with accept
  probability ≥0.85, identity risk ≤0.35, injection risk ≤0.20, or rejects
  only with reject probability ≥0.90 and injection risk ≤0.20. Otherwise it
  stays ungraded for learner self-check. Explain-level authored required-idea
  rubrics retain `semantic-v1`: only independently supported correct judgments
  become success; incomplete/incorrect shadow classes stay ungraded.
- **S02.6 (US-002/US-007):** A close, unsure, or failed check preserves the
  answer, opens self-check with expected answer/explanation, and offers Retry
  check on failure. A graded result names `exact`, `jev`, `learner`, or `reveal`
  authority. Automatic misses offer “I was right” and automatic correct grades
  offer “Count as a miss”; one immutable correction adjusts the current
  schedule without rewriting the original attempt. Assistance and answer
  exposure remain fenced before display; neither a self-check nor a note read
  becomes an unassisted success by accident.

Proof: actual browser interactions plus deterministic learning/SQLite boundary
checks for ambiguous answers, semantic policy thresholds, pending/failure
recovery, assistance cues, and reveal/submit races.

Historical Go first-screen cutover (US-013, 2026-09-28): `/add` now has one text field
and an optional photo, with no mode chooser. Text, including a standalone
URL or shared URL, is saved as private My text and never authorizes web
research. A selected photo uses the existing photo path and caption limit.
The mode-based flow below describes retained explicit-mode captures and
legacy callers; this screen does not claim link reading, topic search, a
new interpretation step, or a new spending contract.

### S03 — Add something without configuring it (US-005)

Historical Go explicit-mode callers only, retained at Git `c6ba395`. This is
not a replacement compatibility API or the Create screen contract; US-013 and
S11 govern the Rust front door. The exact historical criteria remain recorded.

As the learner, I want to add a topic, text, link, or photo with an explicit
choice of what it is, so I do not have to create a deck or card template.

- **S03.1:** Capture requires a visible Topic / My text / Link / Photo choice,
  one input or photo, and no mandatory taxonomy/title. Share-target prefill
  remains editable. The source and its goal persist under one operation ID.
- **S03.2:** Topic alone may trigger Exa search when configured; Link fetches
  only its chosen page; Photo transcribes before planning; pasted text never
  goes to web search. Zero research documents is valid, with topic knowledge
  labeled honestly. Preparation stages remain inspectable after leaving/restart.
- **S03.3:** Unsupported/oversize input is rejected without truncation or
  creating a source. Failed work preserves input, permits bounded retry and
  never reports zero usable questions as ready.

Proof: browser capture/navigation, interrupted response, process restart, and
honest invalid/empty/provider-failure outcomes.

### S04 — Receive useful, trustworthy generated material

As the learner, I want AI to produce material worth reviewing so that I do not
become a full-time editor of generated cards.

- **S04.1:** Source-basis material quotes exact saved source/page/transcript
  text; web-basis material quotes exact saved Exa search excerpts and cites their
  URLs/documents. Topic-basis material carries no fabricated evidence and is
  labeled “General knowledge”; a topic word is not a factual source.
- **S04.2:** Published prompts are answerable, standalone where appropriate,
  non-leaking, and aligned with the requested learning task. MCQs have one
  defensible answer and useful non-overlapping distractors. Explicit complete
  sets or exact-text tasks are not quietly sampled, reordered, or paraphrased
  while claiming completion.
- **S04.3:** Useful validated quizzes become reviewable without a mandatory
  approval inbox. A partial result is labeled partial; paid retries, rejected
  output, and unknown usage are not hidden behind a success count.
- **S04.4:** Slow, rate-limited, or failed model work leaves existing review
  usable. Attempts and spend are bounded; stale or repeated job completion
  cannot duplicate publication or resurrect edited/removed source material.

Proof: a human-reviewed representative content set plus real provider calls for
model quality; controlled external-boundary failures for job behavior. Valid
JSON, deterministic fixtures, or an LLM judge alone cannot prove usefulness.

### S05 — Repair a bad learning moment

As the learner, I want to correct or dismiss a bad question without being
punished for it so that I can trust the app and keep learning.

- **S05.1:** From the question/result I can flag a problem and correct future
  content or stop its future selection without navigating an administration UI.
- **S05.2:** Content edits preserve the old presented wording, choices, answer,
  generation attribution, and recorded result. Future occurrences use the new
  version; changing content does not silently rewrite review history.
- **S05.3:** A grading dispute is distinct from learner failure and from a new
  successful recall. Retain the original event, mark it
  disputed, and offer an explicit recorded schedule reset for faulty material;
  do not silently recompute all subsequent history or assert that the learner
  knew the answer. This is the adopted D3 policy.

Proof: edit/flag after grading, inspect old and new occurrences, and confirm
that lifecycle/correction actions do not manufacture recall evidence.

### S06 — Survive interruptions without losing truth

As the learner, I want to resume after a refresh, lost connection, or app restart
so that I do not have to remember which answers actually saved.

- **S06.1:** Lose an answer response after commit, then retry/reload: exactly
  one review and schedule transition exist and the same result is recoverable.
  Reusing an operation ID with a different payload is an explicit conflict.
- **S06.2:** Competing tabs or stale question versions cannot overwrite a newer
  occurrence. Late HTML/prefetch responses cannot revert the current view.
- **S06.3:** Pending is distinguishable from saved. A definite rejection and an
  unknown network outcome offer safe recovery without losing the in-page
  answer. Offline review pauses; there is no invisible queue of uncommitted
  answers. Only one unresolved browser mutation is permitted.
- **S06.4:** Semantic submission first saves one pending assessment and immutable
  operation receipt, calls the model outside SQL, then rechecks current
  occurrence, content version, schedule version, ungraded state, and lease
  ownership before finalization. Exactly one send lease exists per assessment:
  a concurrent duplicate is refused, a replay reconciles the durable state and
  never resends, and an interrupted send becomes a definite failure with its
  reservation retained as unknown spend. A competing operation conflicts, and
  stale work is superseded without an event or schedule change.

Proof: network interruption/reordering, two browser tabs, semantic timeout and
stale-finalization cases, and real restart with SQLite state inspected through
consumer-visible history/results.

### S07 — Manage what I am learning (US-001/US-009/US-010)

As the learner, I want a Map and concept pages so I can inspect, focus, pause,
and revisit the material I care about.

- **S07.1:** Map lists active goals before paused goals, omits archived
  goals, shows a readable status/due list beside its decorative constellation,
  and does not mutate learning on navigation.
- **S07.2:** Pausing/focusing a goal changes selection without erasing notes or
  history. Archiving source/concept stops future selection and invalidates stale
  publication while retaining old content and evidence.
- **S07.3:** Personal export includes source text, current content, review history,
  and schedule/version information in a documented portable form. Export is
  read-only and does not alter the review session. Full erasure/backup retention
  semantics are a separate explicit decision, not an archive side effect.

Proof: browser edit/archive and an inspected export across a restart.

### S08 — Return to worthwhile practice (US-009/US-011)

As the learner, I want Scry to introduce prerequisites first and choose
appropriate review from honest evidence, without mistaking exposure for recall.

- **S08.1:** The pinned FSRS algorithm, state, rating and time yield the same
  next schedule. A miss/help follows its conservative policy; read, know,
  and practice observations remain separate from graded attempts.
- **S08.2:** Selection v2 introduces unseen concepts prerequisite-first,
  interleaves eligible due practice, honors focused/paused goals, and caps new
  concepts per rolling day at 3/6/12 for light/steady/intense pace.
- **S08.3:** Concept state distinguishes new/learning/solid/fading and tallies
  unaided/helped/missed events. Predicted recall is explicitly an estimate;
  navigation, animation, or a note read never manufactures a learning event.

Proof: prerequisite/pace selection, concept evidence and next-day
resume in focused learning/store checks and browser use. Efficacy claims
require separate delayed unaided recall data.

### S09 — Keep the application private

As the owner, I want easy private access from my phone and computer without
maintaining a signup product or exposing my learning material.

- **S09.1:** Only the approved owner identity can access the app; missing,
  unapproved, or forged identity cannot read/export material or mutate it
  through any exposed hostname, backend listener, or alternate port. Verify
  the actual ingress trust boundary before accepting personal data.
- **S09.2:** Cross-site mutation and user/model script or HTMX-attribute injection
  are rejected or rendered inert. Back/history/cache after sign-out or identity
  change does not expose a previous private session. Prove upstream logout and
  account-switching behavior rather than assume header presence solves session
  lifecycle.
- **S09.3:** Build/preview agents have synthetic data and no production database,
  model, backup, or deployment authority by default. If machine access is added,
  it uses an explicitly scoped/revocable app credential, not VM management/root
  access or a browser token copied into a client.

Proof: actual private HTTPS access, negative ingress/CSRF/content tests, and
inspection of the deployed capability boundary. A local auth stub is not proof.

### S10 — Keep my work recoverable

As the owner, I want a restart, bad release, or lost VM not to destroy my learning
history or silently restart paid work.

- **S10.1:** Restart preserves acknowledged captures/reviews and exposes durable
  jobs as recoverable or failed, not forgotten. Incompatible migration fails
  before serving ordinary traffic or claiming jobs.
- **S10.2:** Backup produces a completed, integrity-checked complete application snapshot and
  required assets/configuration metadata outside the live VM. Interrupted copy
  or upload is not advertised as a usable backup; stale backup status is visible.
- **S10.3:** A fresh isolated environment can restore using off-VM recovery
  material and a compatible binary. Restored uncertain jobs stay paused until
  reconciled; they do not silently regenerate or send again. Measure actual data
  loss and restore time against the approved D5 policy before calling this safe.

Proof: process interruption, concurrent review/backup, incomplete upload, and a
real fresh-environment restore. Persistent disk or VM cloning is insufficient.

### S11 — Create useful material from intent (US-013)

Approved product acceptance, reaffirmed 2026-10-02; not an implementation receipt.
As the learner, I want to express what I want to learn and receive useful
tailored material first, then improve it through use rather than setup.

- **S11.1:** From Create, a short topic word, a phrase, and a long rambling
  dictated request are each valid learning intent. For example, “photosynthesis”,
  “why HTTP caches go stale”, and a transcript mixing current knowledge,
  questions, and a desired practical use do not require imported factual text,
  a mode chooser, learner profile, or diagnostic quiz before preparation.
  Dictated input means accepting its text; no speech engine, browser vendor,
  automatic recording, or microphone integration is selected here. Unsupported
  or oversize input is rejected honestly without silent truncation or loss of
  the editable draft; established 32 KiB text / 4 MiB photo / 1 KiB caption bounds remain unchanged.
- **S11.2:** The first completed result contains useful study/reference material
  and practice attached to teachable concepts: an understandable explanation
  with relevant examples or distinctions, and answerable questions with
  explanatory feedback. It responds to the expressed intent, not merely its
  keywords. A bare topic still receives a useful starting point with assumptions
  made visible. Optional clarification can refine it afterward; no compulsory
  questionnaire or draft-approval inbox stands between curiosity and useful work.
- **S11.3:** Supplied material may support the prompt but is not mandatory.
  Keep captured intent and optional evidence inspectable. A topic/request is
  not a factual citation: generated general knowledge is labeled; source/web
  claims obey S04.1's exact evidence rules. Private text, including dictated
  text and pasted URLs, does not authorize inferred web search or link reading.
- **S11.4:** Saved input and preparation state survive leaving or restarting.
  Pending is visibly pending, not ready or an instantaneous-completion promise;
  existing study stays usable while work runs. Partial usable output is labeled
  partial. Failed/empty output preserves input and offers the existing bounded
  recovery path, never a fabricated success. An unknown sent outcome retains
  accounted usage and requires deliberate reconciliation, not an invisible
  resend. Replayed capture/publication does not duplicate saved work.
- **S11.5:** Subsequent material and practice respond to actual answers,
  expressed confusion, content/grade corrections, and learner feedback. The
  evidence for refinement remains inspectable; an initial guess, reading,
  navigation, reveal, or “I know this already” is not a genuine unaided attempt
  or certified mastery. Preserve immutable presentations/attempts and grading
  authority. Refinement neither silently mutates the pinned scheduler policy
  nor reinstates retired US-012's contrast-generation mechanism by implication.
- **S11.6:** Durable concept notes remain reusable beyond one answer. Practice
  uses useful variations, mechanisms, distinctions, and application where the
  intent calls for them, rather than repeated wording masquerading as breadth.
  Prerequisite introductions, assistance fences, honest estimates, held feedback,
  and deliberate Next remain binding under S02/S08; this front door deletes none
  of those contracts.
- **S11.7:** Privacy, shared bounded spending, exact retry/history, and hosted
  recovery under S06/S09/S10 and D5 remain binding through creation/refinement
  and any future runtime replacement. No new cap, local-only production backup
  exception, historical-store import, or deployment permission follows from S11.

Proof required for product completion: human review of real word/phrase/dictated
inputs through authorized generation, inspectable reference/practice output,
and later interaction that demonstrably refines material without manufacturing
learning history. Exercise slow, partial, failed, and uncertain work separately.
Historical Go Add walks prove their capture/privacy boundary only; source documentation, fixture
success, and older phone receipts do not establish S11 acceptance.

## Cross-cutting quality contracts

These are candidate budgets to approve through the experience slice, not
measurements of the current or proposed app. Keep latency, generation quality,
reliability, usability, and learning outcomes separate.

| ID | Proposed contract | Measurement / acceptance |
| --- | --- | --- |
| UX1 | Input acknowledgement p95 <100 ms | Pointer/key action to visible pressed/pending response on the owner's actual phone, including an induced 300 ms RTT profile; no network dependency for acknowledgement |
| UX2 | Deterministic graded feedback p95 <300 ms | Submit to committed, visible result, on a declared <=100 ms RTT profile; include DB/render/browser time, not just handler time; no claim that AI semantic grading meets this budget |
| UX3 | Prefetched Next p95 <150 ms; existing first review p95 <2 s; capture to first usable quiz p95 <20 s | Report warm/cold state, payload sizes, network/device, sample count, failures/timeouts and measurement endpoints separately; pending UI is not completion, and failed jobs remain in the outcome report |
| UX4 | Usable at 320 CSS px, with keyboard and 200% text zoom | No horizontal clipping; visible focus, labeled controls, accessible result announcements, >=44 CSS px primary targets, adequate contrast, reduced motion; test long questions and explanations, not only short fixtures |
| UX5 | Smooth but honest continuity | At most one speculative next prompt, no speculative success/mastery; canceled gestures and stale responses do not submit; loss/conflict recovers without an invisible offline queue |
| AI1 | Useful material rather than parser success | Operator-approved real-input set includes topics, qualified passages, complete sets, and ambiguous/adversarial inputs; record keep/revise/reject with reasons, coverage, factual/provenance defects, latency and spend; hold out examples from prompt tuning |
| OPS1 | Privacy and recovery are release requirements | S09/S10 proof against the actual private Cloudflare application and remote recovery path; D5 recovery/spend choices approved before relying on real personal data |

A small smoke sample can establish a functioning path, not a production p95 or
population retention claim. Record the sampling plan before a performance run;
repeatable browser emulation is not evidence of the owner's physical-phone feel.
The owner approves aesthetics and usefulness; QA agents can surface defects and
measure contracts but cannot manufacture that approval.

## Detailed behavioral boundaries

These requirements retain behavior previously mixed into the architecture
section. Their implementation is recorded in [ADR 004](docs/adr/004-rust-cloudflare-candidate.md).

### Interruption, privacy, and assistance

- Only one browser mutation may remain unresolved. Reconcile a lost response
  with its exact frozen operation/payload; changed payload conflicts. Definite
  rejection retains editable input; reconnect never automatically sends answers.
- Read-only preparation polling pauses for active/dirty forms, unknown answers,
  or hidden/offline pages. Late responses cannot replace newer stages, retry
  paid work, advance questions, or dismiss held results.
- A current answer, note, goal title/intent, leaking concept name, old attempt,
  edit, photo, or export cannot bypass assistance. Record assistance durably
  before displaying answer-bearing reference. Self-check preserves the original
  attempt's assistance snapshot and records exposure for subsequent practice.
- Private pages are not cached. Denied access removes private content;
  leaving a page removes retained private views, and history return revalidates
  access. Untrusted user/model text remains inert. Core forms work without JS.
- Alternate hostnames may redirect reads only, never replay mutations.
  Browser-owner access does not grant backup or isolated-restore capability.

### Content, practice, and criticism

- Saved goals retain intent and optional photo/transcript; concepts retain
  prerequisites and append-only notes. Occurrences preserve presented wording,
  choices, content/schedule versions, answer, assistance, and held result.
  An acknowledged answer, immutable event, complete schedule, held result, and
  exact operation receipt must agree durably before success is shown.
- A generated fix is a saved draft for its exact question/version, not an
  automatic overwrite. Versioned Save chooses it; later edits/source changes
  invalidate stale work and remove any invalidated answer-bearing rubric.
- Deliberate extra practice, intro/read/know observations, helped attempts,
  and unaided grades remain distinct. Pausing excludes new material but keeps
  due practice; focusing prioritizes new concepts. Scheduler compatibility
  preserves full due/stability/difficulty/interval/repetition/lapse/state/
  last-review/remaining-step state and pinned algorithm identity, not efficacy.
- When configured, US-004's critic independently judges each applicable hard
  defect: unsupported source claims, contradictions,
  changed qualifications, missing context, ambiguous/indefensible answers,
  leaked answers, overlapping choices, misaligned rubrics, and adversarial
  content. Source/choice/rubric defects apply only to those content types.
  Mixed-type or incomplete judgments cannot publish. Thresholds are policy,
  not demonstrated calibration. Critic retries check only unresolved criticism,
  retain prior judgments, and never regenerate the saved batch or fish for scores.
  Mixed accepted/rejected output is partial; no accepted output is not ready.
  Generated output above US-004's 60-candidate bound fails with the input kept;
  it is never sampled, truncated, or presented as complete.

### Capacity and recovery

- The candidate bounds a complete archive to 16 MiB including base64 photo bytes,
  and each stored record to 1 MiB. Existing individual input bounds remain in
  S11/US-013. Reject capture/edit growth beyond archive capacity before committing
  and retain the draft. Paid completion receipts remain durable at capacity;
  capacity failure cannot become backup success or silent deletion.
- A completed backup includes every referenced photo, metadata and checksum,
  and passes exact remote size/checksum/full-byte readback and archive validation.
  Failed upload/readback stays visibly failed in Settings while study remains
  available. Durable storage alone is not independent recovery proof.
- Restore uses an unused isolated space, independently retained compatible
  application/assets/configuration, and an integrity-checked complete archive.
  Validate all content/photo bytes, clear inherited live-backup receipts, and
  keep uncertain jobs/assessments paused without paid integrations. Never overwrite
  acknowledged live writes or reactivate a historical writer during rehearsal.
  Measure private-service recovery and name untested provisioning/DNS/configuration
  steps against D5; copying a live store is not a complete recovery demonstration.
- Before the one-way revival import, an authorized read-only dry run traverses
  the complete preserved dataset inside private Cloudflare. It writes nothing
  and reports only fixed refusal class names and their counts, never content or
  identifiers. An incomplete traversal is a failure, not a complete report.
  Further shape-specific repairs follow that complete report as one batch;
  repeated first-refusal imports are not the diagnostic workflow.

## User stories

The original US IDs and numbered criteria below remain binding alongside the
S criteria above. Overlapping IDs express the same contract at different journey
scales, not competing authorities. Policy names (FSRS, Jev, short-v1, semantic-v1,
critic-v1) retain their accepted compatibility meaning; technical decisions and
verification commands live outside this behavioral reference.

| Story | Related end-to-end criteria | Scope |
| --- | --- | --- |
| US-001 | S05, S07 | Current concepts/history; criterion 1 is retained Go migration only |
| US-002 | S01, S02 | Current study loop |
| US-003 | S02, S06 | Current grading |
| US-004 | S04, S06 | Current prepublication criticism |
| US-005 | S03 | Superseded front door; historical Go explicit-mode callers only |
| US-006 | S02, S04, S07 | Current concept reference |
| US-007 | S02, S05, S06 | Current grade correction |
| US-008 | S02 | Current short-answer policy |
| US-009 | S07, S08 | Current Map/evidence |
| US-010 | S07, S08 | Current focus, pause and pace |
| US-011 | S08 | Current prerequisites |
| US-012 | S11.5 excludes reinstatement | Retired; ID reserved |
| US-013 | S11, S04, S06, S09, S10 | Current Create and refinement |

## US-001 Find and reuse saved understanding

Statement: When I explore what I am learning, I want saved concepts and notes revisitable, so that I can connect prerequisites across questions without losing earlier study.

Criteria:
1. WHEN schema 4 upgrades to schema 5, THE SYSTEM SHALL preserve foundation rows and relations as historical data; foundation-origin concepts SHALL NOT appear as newly generated concepts in the Map or Stream.
2. WHEN I inspect a concept page, THE SYSTEM SHALL show its current notes, questions, prerequisites, and linked sources while preserving old presented wording and review history.

Criterion 1 remains binding on the retained Go/SQLite runtime and its schema
4→5 migration, preserved at Git `c6ba395` with compatible recovery artifacts.
The Rust candidate never performs that Go schema upgrade or deletes that store;
D6's separately authorized one-way revival is not the historical migration or
API parity. Criterion 2 applies to the current candidate. Neither proves serving state.

No-gos: no manual graph editor, no automatic replanner.

## US-002 Stay with one question through its result

Statement: When I review, I want one question and one way to answer, followed by the result and Next, so I can keep studying without navigating a dashboard.

Criteria:
1. WHEN a question is ungraded, THE SYSTEM SHALL show it as the dominant heading with one answer control (choice buttons that submit by tap, or one recall field and submit), and SHALL keep the answer and explanation hidden until grading or self-check.
2. WHEN an answer resolves, THE SYSTEM SHALL retain the question, result, expected answer, and explanation until I choose Next; a choice tap SHALL NOT require another submit. WHEN a check is pending, THE SYSTEM SHALL show checking without inventing a grade.
3. WHEN an automatic check reports a miss, THE SYSTEM SHALL show a quiet "I was right" link beneath Next; WHEN an automatic check reports correct, THE SYSTEM SHALL offer "Count as a miss" in More. These controls SHALL NOT auto-advance.
4. THE SYSTEM SHALL show exactly two masthead destinations, Create and Map, beside the wordmark; other maintenance actions SHALL remain in More, not an always-visible navigation bar or inspection dashboard.
5. WHEN review is empty or caught up, THE SYSTEM SHALL offer Create or an honest next-time indication, not an invented due question or foundation detour.

No-gos: no auto-advance, due-count chrome, foundation routes, or hidden answer preloading.

## US-003 Get an honest answer judgment

Statement: When I answer in my own words, I want correct meaning recognized where appropriate and uncertainty handed back to me, so my history reflects what I actually knew.

Criteria:
1. WHEN a choice answer, the exact key, or an authored variant matches, THE SYSTEM SHALL resolve it locally without a network check; THE SYSTEM SHALL NOT award, mark wrong, or send to self-check any other recall answer locally, however close or far it is.
2. WHEN a recall answer does not match the key or an authored variant, THE SYSTEM SHALL stage one bounded Jev `short-v1` check outside SQL; it SHALL accept only at accept probability ≥0.85, exact-identity risk ≤0.35, injection risk ≤0.20, or reject only at reject probability ≥0.90 and injection risk ≤0.20. Other outcomes SHALL remain ungraded for self-check.
3. WHEN explain-level prose has an authored rubric, THE SYSTEM SHALL retain the `semantic-v1` required-ideas policy; incomplete and incorrect shadow classes SHALL remain ungraded, and answer-bearing cues SHALL count as assistance before display.
4. WHEN a check is close, unsure, unavailable, malformed, or fails, THE SYSTEM SHALL preserve the answer and offer a learner self-check; a failed check SHALL also offer Retry check without sending the identical paid assessment twice.
5. WHEN a result is recorded, THE SYSTEM SHALL name its authority as exact, Jev, learner, or reveal; it SHALL preserve original attempts and grade corrections across restart without silently rewriting history.

No-gos: no liberal string-similarity grading, model call inside a SQL transaction, learner rubric authoring, or change to pinned scheduler identity.

## US-004 Check generated candidates before publication

Statement: I want generated quizzes checked for critical defects before they
enter review. A failed check must retain candidates and paid usage.

Criteria:
1. WHEN the critic is configured, THE SYSTEM SHALL save validated candidates
   before any critic request. EACH batch SHALL contain at most 60 candidates.
2. THE critic-v1 policy SHALL reject any applicable hard defect at probability
   0.80 or higher. Missing or malformed judgments SHALL remain ungraded.
   Explanation teaching value SHALL rank only, never reject.
3. EACH assessment SHALL have one transmission lease and an atomic reservation
   against the shared generation and semantic allowance. Interrupted sends SHALL
   retain unknown usage. A retry SHALL use a new assessment, never resend a row.
4. WHEN a critic fails, THE SYSTEM SHALL preserve candidates and retry only the
   critic. Judged candidates SHALL be reused. Publication SHALL recheck the saved
   judgments, source revision, and job ownership in one transaction.
5. WHEN no critic endpoint is configured, THE SYSTEM SHALL record skipped
   criticism without reserving allowance or changing existing publication.
6. Export and content history SHALL retain candidate attempts, defect reasons,
   model attribution, raw responses, and known or unknown usage.

No-gos: no generator-v4 change, pairwise duplicate checks,
practice-coverage UI, production activation, or general accuracy claim.

## US-005 Capture a goal in one step

Superseded by US-013 for the Go Add contract on 2026-09-28. The Rust replacement
uses prompt-first Create. Explicit-mode criteria remain historical compatibility
for the archived Go revision and captures; no replacement Topic/Link API or
inferred browsing follows from them.

Statement: When I have something to learn, I want to add it in one step with its actual source type, so useful material can be prepared without a project setup.

Criteria:
1. WHEN a retained explicit-mode caller adds a Topic, My text, Link, or Photo, THE SYSTEM SHALL require that caller's mode and save exactly one source and goal for an identical operation ID; this SHALL NOT require a mode chooser on Add/Create.
2. WHEN I choose Topic, THE SYSTEM SHALL search through Exa only if configured; WHEN I paste My text, THE SYSTEM SHALL NOT send it to web search. A Link SHALL fetch only the chosen URL, and a Photo SHALL transcribe before planning.
3. IF Topic research has no documents, THEN THE SYSTEM SHALL proceed with labeled general knowledge; IF a Link page cannot be read, THEN THE SYSTEM SHALL fail that preparation recoverably instead of planning from the URL; IF a photo is unsupported or too large, THEN THE SYSTEM SHALL reject it without creating a source.
4. WHEN generation fails, THE SYSTEM SHALL preserve captured material and show a recoverable failed preparation state rather than report ready.

No-gos: no automatic search of pasted private text, account signup, or duplicate capture on replay.

## US-006 Understand a question's concept

Statement: When a question assumes an idea I do not understand, I want a note for that concept one tap away, so studying can be more than repeating the answer.

Criteria:
1. WHEN a newly generated question publishes, THE SYSTEM SHALL link exactly one primary concept and an existing standard note.
2. WHEN I open a concept from a graded question, THE SYSTEM SHALL show its note and linked questions without changing the current review occurrence.
3. WHEN a note quotes my material or a web excerpt, THE SYSTEM SHALL keep exact evidence and inspectable provenance; general-knowledge notes SHALL be labeled and SHALL NOT claim quoted source support.

No-gos: no untrusted markup execution, fabricated citations, or reading counted as unaided recall.

## US-007 Correct a grade in one tap

Statement: When a check gets my answer wrong, I want to correct it immediately, so my future practice does not inherit a false result.

Criteria:
1. WHEN an automatic miss is shown, THE SYSTEM SHALL offer "I was right" beneath Next; WHEN an automatic correct result is shown, THE SYSTEM SHALL offer "Count as a miss" in More.
2. WHEN I correct a grade, THE SYSTEM SHALL record one immutable override, replace the current schedule consistently, and retain the original attempt and authority in history.
3. WHEN the same override operation is retried, THE SYSTEM SHALL return the committed correction without a second schedule change; a stale correction SHALL conflict.

No-gos: no rewriting the original attempt or pretending the correction was a new cold recall.

## US-008 Answer short questions in my own words

Statement: When a short answer means the same thing in different words, I want it to count without accepting a different fact as correct.

Criteria:
1. WHEN a short response does not match the key or an authored variant locally, THE SYSTEM SHALL use the `short-v1` Jev battery with verdict, exact-identity, and injection judgments.
2. WHEN accept probability is at least 0.85 and identity risk at most 0.35 and injection risk at most 0.20, THE SYSTEM SHALL accept; WHEN reject probability is at least 0.90 and injection risk at most 0.20, THE SYSTEM SHALL reject; otherwise it SHALL ask me to self-check.
3. WHEN a check fails or its result cannot be trusted, THE SYSTEM SHALL keep my answer, name the learner's judgment as authority if I self-check, and SHALL NOT silently award success.

No-gos: no liberal similarity rule, hidden automatic retry, or claim that an untested holdout passed; policy adoption requires holdout evidence before live activation.

## US-009 See what I know

Statement: When I return to study, I want to see where my concepts stand, so I can choose what needs attention without mistaking an estimate for a fact.

Criteria:
1. WHEN I open Map, THE SYSTEM SHALL show active and paused goals with concept status, due indication, and an accessible list alongside any decorative constellation.
2. WHEN I inspect a concept, THE SYSTEM SHALL distinguish new, learning, solid, and fading, show unaided/helped/missed observations separately, and label predicted recall explicitly as an estimate.

No-gos: no guaranteed mastery score, color-only meaning, or synthetic review events from navigation.

## US-010 Say what I want to know

Statement: When my interests change, I want to pause or focus a learning goal, so the next material reflects what I choose.

Criteria:
1. WHEN capture succeeds, THE SYSTEM SHALL create one goal tied to the source; focus SHALL prioritize new concepts belonging to that goal.
2. WHEN I pause a goal, THE SYSTEM SHALL exclude its new material from ordinary selection and retain its notes/history; WHEN I resume it, THE SYSTEM SHALL make eligible material selectable again.
3. WHEN I change pace among light, steady, and intense, THE SYSTEM SHALL cap new concepts in a rolling day at 3, 6, and 12 respectively without fabricating due work.

No-gos: no invisible goal deletion or goal settings as a gate before capture.

## US-011 Meet prerequisites first

Statement: When one idea depends on another, I want a useful introduction and simpler starting point before its questions, so I am not repeatedly tested on something I have not encountered.

Criteria:
1. WHEN a goal's concepts have prerequisites, THE SYSTEM SHALL order unseen introductions prerequisite-first and show the standard note before that concept's first question.
2. WHEN I choose "I know this already" on the intro, THE SYSTEM SHALL record that observation without scoring a cold review or skipping later due questions.

No-gos: no compulsory foundation detour or reading counted as a graded success.

## US-012 Practice a real confusion (retired)

Retired on 2026-09-23 by the operator's simplification decision (MIS-162): Scry
no longer generates contrast questions from recorded confusions. The id stays
reserved.

## US-013 Create from what I want to learn

Statement: When curiosity starts as a word, phrase, or long dictated ramble,
I want Create to produce useful tailored study/reference and practice material
without a setup questionnaire, so I can start learning and refine it through
actual practice and feedback.

Acceptance: [S11.1–S11.7](#s11--create-useful-material-from-intent-us-013)
bind the approved Create journey, including useful first material, honest
generation states, and refinement without fabricated mastery. The criteria below
preserve the capture/privacy boundary adopted on 2026-09-28 in the redesigned
Create flow. The implementation accepts dictated text; it does not select a
speech engine or add automatic recording. Current tests and synthetic browser
walks establish only the cases they actually exercise.

Criteria:
1. WHEN I submit text or one supported photo from Create, THE SYSTEM SHALL save
   one source and goal without a mode chooser, enforce the existing 32 KiB
   text, 4 MiB photo and 1 KiB caption limits, and return the saved source for
   an identical operation.
2. WHEN I paste or share text containing a URL, THE SYSTEM SHALL keep it
   private text and SHALL NOT start web research by inference; invalid input
   SHALL leave a usable Create field without creating a source.

No-gos: mandatory import, onboarding questionnaire or diagnostic quiz before
useful material; inferred public search; assumed speech engine or recording
integration; source-only docs treated as runtime or deployment acceptance.
