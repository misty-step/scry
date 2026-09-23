# Scry concept-centered specification

Status: v5 direction authorized on 2026-09-23 by the operator: “Execute this
reimagining in full” (MIS-162). This is an implementation contract, not a
production activation receipt. Live schema migration needs separate release-time
approval. The current deployed state and compatible artifacts remain owned by
[the runbook](docs/runbook.md); previously approved phone-flow and recovery proof
are not v5 acceptance.

[VISION](VISION.md) owns intent; [USER_STORIES](USER_STORIES.md) owns the learner
stories; [Direction A](DESIGN.md) owns visual decisions; the earlier Rust
extraction strategy remains historical in [its migration record](docs/rust-migration.md).

## Authority and open decisions

**Confirmed by the operator:** the rewrite began with no active users and an
experience the operator wanted to replace. Go, SQLite, HTMX, and exe.dev are the
accepted direction: smooth, simple, aesthetically intentional, and enhanced by
AI content generation. Full execution was authorized. The initial real-phone
flow was subsequently approved, followed by the daily recovery policy below.

**2026-09-23 decision (MIS-162):** after rejecting the MIS-59 foundations
experience on 2026-09-12, the operator explicitly authorized the complete
concept-centered redesign. The accepted design is one question, Add and Map,
concepts with durable notes, selected capture modes, honest short-answer
checking/self-check/grade override, and the “Scrying glass” visual system.
US-001–003 change intent under this explicit authorization; US-005–012 add
failable contracts. The old foundations data/history survive v5 migration but
its UI and routes retire. Production activation is separately gated.

**Spending decision:** the existing Scry personal (exe.dev) provider key limit
was raised to $25/week on 2026-09-23; the application permits $3.50 per rolling
24 hours with $0.50 conservative generation reservations. Jev assessments
still reserve from that same allowance; an unknown sent request keeps its
reservation. Topic capture may use Exa web search; pasted text never does.
The [private acceptance receipt](docs/qa/personal-go-acceptance-20260909.json)
records native touch, live generation, and independent-VM data restoration at
that earlier observation. Subsequent phone approval and full restored-service
activation are separate evidence: the recovered private HTTPS export matched
exactly and the restored Review UI rendered. Approximately 117 seconds elapsed
through private HTTPS on an existing recovery VM; provisioning and DNS recovery
were not timed, and the synthetic rehearsal received no production integrations.

**Open acceptance:** useful material, short-v1 holdout quality, real browser
interaction, independent recovery and live provider outcomes need evidence for
v5. A passing fixture or prior phone report does not establish them. The
earlier S09.2 different-account history-return observation remains unverified.
Old stores remain preserved separately; import or deletion needs a new decision.

Resolve these decisions as the relevant slice becomes ready. Record the decision
and reason here; remove obsolete alternatives rather than retaining two designs.

| Decision | Adopted choice | Basis / remaining evidence |
| --- | --- | --- |
| D1: personal material and failures | Build around what the operator wants to remember, not old QA datasets | The accepted candidate used photosynthesis, HTTP caching, and DNS examples. Specific future learning goals and sustained usefulness come from real use, not invented frustrations. |
| D2: response grading | Choice and exact/variant recall resolve locally; flexible short recall uses bounded Jev `short-v1`; explain-level prose retains rubric `semantic-v1`. Close/unsure/failed checks offer self-check, and automatic grades allow one-tap correction. Every recorded grade names exact, Jev, learner, or reveal authority. | Operator authorized 2026-09-23; short-v1 accepts p≥0.85 with identity≤0.35 and injection≤0.20, rejects p≥0.90 with injection≤0.20, otherwise self-check. No liberal similarity or model call inside SQL; holdout quality remains to prove. |
| D3: assistance and correction | Reveal, answer-bearing cues and self-check exposure remain honest; immutable original grade plus separate override adjusts the current schedule without rewriting history. | Exact operation replay is durable; reading/intro is not cold success. |
| D4: experience approval | One question and one answer action, retained feedback and deliberate Next; Add/Map masthead and Direction A visual system | v5 phone/usefulness requires new observation; earlier approved flow is not blanket acceptance. |
| D5: recovery / spend | Daily/pre-release off-VM backups, 30-day retention, RPO 24h/RTO 60m targets; provider key $25/week, application $3.50/rolling day, $0.50 generation reservation | Limit raised on 2026-09-23 for Scry personal (exe.dev). Unknown cost retains reservation, not free retry; backup targets are not guarantees. |
| D6: replacement boundary | Fresh target data, no legacy API parity; preserve historical stores and backups separately | Operator explicitly directed preservation. Both old Workers are paused; native Postgres remains disabled with recovery backups active. No old data import or deletion. |

Unresolved decisions block only work that depends on them. They do not require
planning every future feature before testing the core experience.

## The specification spine

| Layer | Durable owner | Question answered |
| --- | --- | --- |
| Product | VISION.md | Who is this for, what outcome matters, what is out of scope? |
| Stories | USER_STORIES.md plus this file's criteria | What must the learner be able to do, including failure and interruption? |
| Behavior / criteria | This file (binding ids: S##.##) | What exactly must be accepted for each story? |
| Design | Architecture and decision sections below | What is the smallest system that can deliver those behaviors? |
| Work | Linear, linked to story/criterion IDs and a source revision | What bounded change is someone doing now? |
| Verification | docs/qa/system.md plus docs/qa/critics.md critic receipts | What was actually exercised, observed, and not verified? |

**Contract status.** Implemented behavior lives in code, tests, and dated
receipts; accepted goals live in [VISION.md](VISION.md) and the accepted
criteria in this file; proposals live in [design notes](docs/design/) marked as
proposals; unknowns live in the Open acceptance list above. No other document is
a second authority for these states.

This is not a waterfall. A cheap experience or technical spike can change a
story or design before implementation. Later failures feed back into the spec.
Do not build a specification compiler, ticket generator, or agent framework to
operate this process. Two living documents, ordinary issues, and concise receipts
are sufficient until repeated work proves otherwise.

Use stable IDs below. Editing a criterion's meaning requires recording the
reason and approving the new spec revision; do not quietly weaken it to fit code.
Retire IDs explicitly rather than assigning their old meaning to a new behavior.

## Experience contract

The first screen is the learning moment, not a dashboard. Learning effort is
intentional; interface effort is not. An ordinary session should not require
choosing a deck, configuring a scheduler, approving drafts, reading telemetry,
or understanding the implementation.

The Stream opens into a question, a concept introduction, a preparing receipt,
or an honest first-run/caught-up state; never an administration dashboard.
Concepts are teachable ideas, goals express intent, and notes are durable
reference material at standard, simpler, or deeper level. The Map is the place
to search, inspect goals/concepts, focus or pause, and see a labeled estimate
from real observations, not a certified mastery score.

```text
scry                                  + Add   Map
concept chip (inert before grading)
One question or first-time concept introduction
One answer control OR Got it / I know this already

checking → self-check (if unsure/close/failed) → held result
Correct. / Not quite. / Shown.
expected answer · explanation · citations
Next
I was right (quiet, automatic miss only)
```

- One choice tap submits, or a recall field uses Check (empty → Show me).
  `question`, `checking`, `self-check`, `result`, `intro`, `preparing`,
  `empty-first-run`, `caught-up`, and `conflict/error` are distinct states.
  Feedback stays until deliberate Next; no auto-dismiss, swipe grade, or
  ungraded answer/explanation preloading.
- Self-check is learner authority on close exact-form near misses, Jev unsure,
  or failed check. Failed check also offers Retry check using a new deliberate
  operation, not a duplicate send. An automatic miss offers quiet “I was right”
  under Next; automatic correct offers “Count as a miss” in More. Immutable
  attempt and correction both remain in history.
- Intro presents a standard note before an unseen concept's first question;
  “I know this already” records an observation without scoring a cold answer.
  While a question awaits an unaided answer, every concept it assesses or
  contrasts opens behind the Look it up gate, and text or titles drawn from its
  own capture (source text, goal title, preparation receipts, search hits) are
  replaced on every route until assistance is recorded; a concept page links to
  its capture but never carries the capture's text. Navigate to a concept page
  without consuming a review occurrence. A note request preserves the current
  standard note until its new level is ready.
- Add requires explicit Topic / My text / Link / Photo mode. Topic alone may
  start Exa search; Link fetches its chosen page; Photo transcribes; pasted
  text goes straight to planning, never web search. Only private modes may be
  preselected (pasted text, a chosen photo); Topic and Link are never inferred
  from length or URL shape, and research refuses any other mode before a
  request leaves. Captures and preparing failures remain inspectable on Source
  and Map. Share-target prefill via `/add?text=&url=&title=` remains editable
  and chooses no mode.
- Map starts with search, shows goal sections and an accessible concept list
  alongside a decorative constellation. Concept pages show status, separate
  unaided/helped/missed tally, labeled recall estimate, notes, related concepts,
  questions, provenance, and Practice. Status words and focus/pause never
  depend on color alone.
- Keep private browser controller presentation-only; without JS, forms and
  required mode radios remain functional. Design tokens, responsive behavior,
  reduced motion, and accessible targets live in [DESIGN](DESIGN.md).

## User stories and acceptance criteria

These are the accepted implementation criteria. Their source/environment-bound
**PASS**, **FAIL**, and **UNVERIFIED** observations belong to the
[acceptance receipt](docs/qa/personal-go-cutover-20260910.json), not this prose.
IDs are specification identifiers, not invented Linear issues. Quality contracts
apply across stories; the owning issue links here rather than copying the spec.

### S01 — Open into useful learning

As the learner, I want opening Scry to put me into a useful learning moment so
that I can start without organizing the app first.

- **S01.1:** With eligible material, opening review shows a question and an
  actionable response control without an intermediate dashboard/deck choice.
- **S01.2:** With no material, the screen offers capture. With material but none
  currently due/eligible, it explains that state and offers a deliberate next
  action; it does not invent due work or silently reset schedules.
- **S01.3:** Returning from Add/Map/Concept preserves an unfinished occurrence
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
- **S02.5 (US-003/US-008):** Content versions distinguish exact, flexible
  short recall, and explain-level prose. Choices/exact/variants resolve locally;
  exact-form case/whitespace near misses ask for self-check. Flexible recall
  stages one bounded Jev `short-v1` battery and accepts only with accept
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

### S03 — Add something without configuring it (US-005)

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

### S07 — Find and manage what I am learning (US-001/US-009/US-010)

As the learner, I want a searchable Map and concept pages so I can inspect,
focus, pause, and revisit the material I care about.

- **S07.1:** Search finds concept, note, question, and source text with linked
  destinations; question hits never reveal answer snippets. Map lists active
  goals before paused goals, omits archived goals, shows a readable status/due
  list beside its decorative constellation, and does not mutate learning on
  navigation.
- **S07.2:** Pausing/focusing a goal changes selection without erasing notes or
  history. Archiving source/concept stops future selection and invalidates stale
  publication while retaining old content and evidence.
- **S07.3:** Personal export includes source text, current content, review history,
  and schedule/version information in a documented portable form. Export is
  read-only and does not alter the review session. Full erasure/backup retention
  semantics are a separate explicit decision, not an archive side effect.

Proof: browser search/edit/archive and an inspected export across a restart.

### S08 — Return to worthwhile practice (US-009/US-011/US-012)

As the learner, I want Scry to introduce prerequisites first and choose
appropriate review from honest evidence, without mistaking exposure for recall.

- **S08.1:** The pinned FSRS algorithm, state, rating and time yield the same
  next schedule. A miss/help follows its conservative policy; read, know,
  practice, and confusion observations remain separate from graded attempts.
- **S08.2:** Selection v2 introduces unseen concepts prerequisite-first,
  interleaves eligible due practice, honors focused/paused goals, and caps new
  concepts per rolling day at 3/6/12 for light/steady/intense pace. One recorded
  confusion is not a grade; two for one pair can queue a contrast question.
- **S08.3:** Concept state distinguishes new/learning/solid/fading and tallies
  unaided/helped/missed events. Predicted recall is explicitly an estimate;
  navigation, animation, or a note read never manufactures a learning event.

Proof: prerequisite/pace/confusion selection, concept evidence and next-day
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
- **S10.2:** Backup produces a completed, integrity-checked SQLite snapshot and
  required assets/configuration metadata outside the live VM. Interrupted copy
  or upload is not advertised as a usable backup; stale backup status is visible.
- **S10.3:** A fresh isolated environment can restore using off-VM recovery
  material and a compatible binary. Restored uncertain jobs stay paused until
  reconciled; they do not silently regenerate or send again. Measure actual data
  loss and restore time against the approved D5 policy before calling this safe.

Proof: process interruption, concurrent review/backup, incomplete upload, and a
real fresh-environment restore. Persistent disk or VM cloning is insufficient.

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
| OPS1 | Privacy and recovery are release requirements | S09/S10 proof against the actual exe application and off-VM recovery path; D5 recovery/spend choices approved before relying on real personal data |

A small smoke sample can establish a functioning path, not a production p95 or
population retention claim. Record the sampling plan before a performance run;
repeatable browser emulation is not evidence of the owner's physical-phone feel.
The owner approves aesthetics and usefulness; QA agents can surface defects and
measure contracts but cannot manufacture that approval.

## V5 architecture and durable boundaries

### One application and one state authority

```text
Phone browser — HTML/HTMX, embedded assets, small presentation controller
       |
Cloudflare Access → Worker scry-app-host (exact subject) → singleton Container
       |
nginx (strip client authority; inject owner) → Go/SQLite (one writer)
       |                       |                   |
SQLite WAL           Model / Jev HTTPS      Exa HTTPS (selected captures)
       |
consistent snapshot → append/read gateway → private R2
```

The Go module uses net/http, html/template, embedded assets/migrations,
`internal/learning` pure policy, `internal/store` short SQLite transactions,
`internal/generation` and `internal/semantic` bounded external calls, and
`internal/web` private routes. No React/frontend build, second writer, vector
database, event bus, or generic AI orchestration layer. Use existing
Cloudflare Worker/Container ingress, not an app Worker database.

HTMX handles HTML forms, fragments, ordinary navigation and bounded job polling.
Initial rich content is escaped text/controlled formatting, not model-generated
HTML. Treat user/source/model text as untrusted data, never template source or
unchecked template.HTML. Pin and embed browser dependencies; do not require a
third-party CDN to open the app.

### The browser/server boundary

Pure fragment roundtrips cannot beat real network latency. A small review-only
browser controller owns immediate visual feedback, focus, gesture cancellation,
transitions, and a bounded in-memory prefetch. Go owns grading, scheduling,
publication, presentation identity, and durable progress.

A candidate next fragment may arrive with a committed result or a read-only
prefetch. It carries a version/occurrence token, not authority to advance a
schedule. Fetching it does not count as exposure, an answer, or a paid job.
Do not preload an answer/explanation into the visible or accessibility tree
before the learner answers or asks for help. Revealing answer-bearing content
must be fenced durably before it can coexist with a cold-recall success; do not
make local reveal instantaneous by removing that guarantee.

A transition may be provisional, visibly pending, with one unresolved mutation.
A stale/failed transition reconciles to server truth. A lost response is an
unknown outcome, not proof the server rolled back; retry the same operation ID.
Do not let a second answer disappear into an offline queue. This bound may feel
slow on a poor connection: the real-phone spike decides whether the tradeoff is
acceptable, not an assertion that optimistic HTML is automatically smooth.

Ordinary forms retain core functionality without the enhancement. Disable HTMX
localStorage history snapshots on private review screens, use appropriate
private/no-store response policies, and explicitly handle 409/422/error swaps.
Aborting an HTMX request does not undo a committed server transaction.

### Domain state and transactions

The essential records are sources and revisions; quizzes and content versions;
presentations/assistance state; immutable review events and operation receipts;
current schedules; durable generation jobs/attempt accounting; owner settings.
This is a conceptual model, not a mandate for one table or package per noun.

Review transaction:

```text
authorize owner and validate operation/presentation
begin short write transaction
  identical prior operation -> return its result
  same ID / different payload or stale occurrence -> conflict
  validate content version and assistance state
  grade and compute next schedule with explicit algorithm version/time
  save event + schedule + resumable result/receipt atomically
commit
return committed result, optionally with next prompt preview
```

Use sql.Tx consistently; do not mix pooled DB calls into a transaction. WAL
stays on local persistent disk, not R2 or a network filesystem. Configure foreign
keys, bounded busy handling and deliberate connection limits on every relevant
connection. Begin with serialized short writes and FULL synchronous durability;
weaken acknowledged-write durability only through an explicit decision. Close
read cursors promptly. No model HTTP or streaming inside a SQL transaction.

`modernc.org/sqlite` v1.58.0 is the selected pure-Go driver; the release gate
exercises a CGO-disabled Linux amd64 binary. Preserve transaction, cancellation,
backup, engine-fix, and target-build requirements on upgrades. The embedded
SQLite engine must include the official WAL-reset corruption fix (3.51.3 or a
documented fixed backport); a Go module version alone is not that evidence.

`go-fsrs/v4` v4.0.0 is pinned behind the pure adapter in `internal/learning`.
The constructor can silently fall back for invalid parameters, so the adapter
validates pinned configuration first. Versioned reference trajectories, including
misses/relearning and equivalent-time replay, are exercised by the Go gate.
`internal/learning.Scheduler` identifies the unchanged scheduler policy on
every schedule card; `internal/learning.Algorithm` is that identity plus the
default `exact-v1` grading and remains byte-identical to pre-Jev history.
Each event names `exact-v1`, `short-v1`, `semantic-v1`, or `learner-v1` as
appropriate; a `grade_overrides` row retains a separate correction and before/
after schedule. Old events remain immutable. No old-engine parity, personalized
retention, or learning-efficacy claim is inherited.

Schema v5 adds goals and goal-concept membership; generated/learner concepts
with status and source origin; `requires`, `part_of`, and symmetric
`confused_with` relations; immutable per-level notes and source documents;
private bounded capture images; question concept roles/level/answer form/
citations; evidence observations; grade overrides; preferences and search
index. One primary concept is required for each new question. Old foundation
rows and links remain with historical origin but stay out of Map/Stream.
Migration v4→v5 is additive and transactional, with `user_version=5`, full
schema/reference validation and FTS rebuild. Once migrated a v4 binary cannot
open the live DB; see [release boundary](docs/runbook.md#schema-v5-release-boundary).

The private HTTP surface is Stream `/`, Capture `/add`, Map `/map`, Concept
`/concepts/{id}` (note, questions, practice and
archive actions), Goal `/goals/{id}` (pause/resume/focus), Source
`/sources/{id}` (input/documents/image/retry/archive), and History/Settings.
Review writes use `/review/answer`, `/review/reveal`, `/review/next`,
`/review/self`, `/review/override`, and `/review/intro`, each with CSRF and
idempotent operation handling. Question fix/edit/archive and source export
retain their authorized routes. Foundation routes and `/library` are removed.

### Generation and learning policy

Capture creates one goal/source and a sequential chain per mode:
`topic → research (Exa search) → plan → questions`,
`link → research (Exa contents) → plan → questions`,
`photo → transcribe → plan → questions`,
`text → plan → questions`. Topic research with zero documents proceeds as
general knowledge, with no invented citation. Link research needs the chosen
page; a missing Exa key or unreadable page fails the preparation with a
recoverable message (paste the text as My text, or retry), never a plan about a
URL nobody read. The content client sends a bounded search
or chosen-link fetch only for those explicit modes. A plan produces 1–12 atomic
concepts with a standard note and justified relations; reuse an existing active
concept only when it is the same idea, not merely adjacent. Dedupe judgments use
Jev only for candidate matches, with p≥0.80 to reuse; absent endpoint skips
dedupe. Ordinary goals should aim for 3–10 concepts, and exact/complete-set
tasks retain their unit ordering contract.

Questions ascend recognize → recall → explain/apply (2–3 per concept where
appropriate). Exact answer form is for exact identity/wording; otherwise use
flexible short recall. Explain prose alone gets required-idea rubric. Choice
distractors can identify the contrasting concept. `questions` can be requested
for one concept, `note` for simpler/deeper level, `fix` for a specified quiz
version, and after two observations of the same confusion `contrast` for the
pair. A `fix` only suggests: its corrected question waits beside the current
one on the question's edit page and changes nothing until the learner uses it
(then it is written as the next version under the same fences as a manual
edit) or keeps theirs; a later edit or suggestion supersedes an undecided one.
A fix belongs to its question: the capture's chain lists it, but it never
becomes the capture's status, receipt, or Try again. A stopped fix is reported
on the question with its accounted use; a failed request is moot after an
edit, while one paused by a restore stays paused and reported until the
learner asks again, which is its explicit retry.
Only one live job per source; busy requests return conflict. Completing
each stage atomically publishes validated content and enqueues its successor.
Validation is per item: an invalid concept or question is dropped (with any
relation naming it) and the valid remainder publishes as a partial stage whose
note names each failed check; a stage fails only when nothing valid remains.
Exact-text and complete-set tasks stay all-or-nothing. Model output is
normalized where no honesty is lost (goal cut to 120 characters, levels sorted,
unsupported or prompt-copied recall variants removed, fill-in coverage cleared).
When search excerpts are supplied, notes and questions ground in them.

Jobs claim durable attempt/lease ownership before Exa, model, or Jev HTTP
outside SQL; publication rechecks owner, source revision, and source lifecycle.
Published quotes are byte-exact substrings of the learner's material or a saved
search excerpt: a model quote that matches only after whitespace, quote-mark,
dash, or ellipsis normalization is replaced by the original text, and an
unmatched quote is dropped. An item left without evidence is dropped for the
learner's own material, or labeled General knowledge (no evidence, no citation)
for a topic. Web quotes cite the saved result that contains them; topic notes
carry no evidence. Untrusted input stays serialized data, not instructions or
HTML. Candidate critic US-004 remains prepublication, with shared spend
accounting. Known cost settles reservations; unknown sent outcomes stay charged
and paused, never silently retried.

Selection v2 uses prerequisite-first unseen intros, due reviews, focused goals,
light/steady/intense rolling-day new-concept caps (3/6/12), and concept state
`new|learning|solid|fading` from graded and non-graded observations. Recall
probability/brightness are estimates, never mastery. Reveal/reading stays
assisted or observational, and `internal/learning.Algorithm` remains pinned.

### Prepublication content critic: US-004

The configured semantic endpoint checks validated generation candidates.
The absolute batch bound is 60 for exact-text and complete-set tasks; ordinary
concept question jobs produce at most 36 (12 concepts × 3). No silent
truncation: a larger batch fails validation rather than claiming complete
coverage. Candidates, generator attribution, and usage persist before
criticism. With no endpoint, `critic_status=skipped` preserves existing
publication behavior. Historical foundation rows are not candidates.

`BuildCriticRequest` sends only candidate prompt, answer, explanation, choices,
rubric, basis, and evidence. Each applicable defect receives an independent
Noul judgment. Hard defects cover unsupported source answers, contradictory
evidence, changed qualifications, missing context, ambiguous answers, indefensible
answers, leaked answers, overlapping choices, misaligned rubrics, and adversarial
content. Source support applies only to source-basis candidates. Choice overlap
and rubric alignment apply only to their corresponding quiz types. The leakage
check also covers authored rubric cues. Required ideas remain exclusive to
explain-level authored prose. The validator rejects rubrics on choice,
exact-text, flexible short-recall, and complete-set output; source-basis
ideas must be supported by the candidate's evidence without strengthening
qualifications. Generated rubrics never leak hints. Under `semantic-v1`,
incomplete and incorrect remain ungraded; rubric alignment is judged in the
same bounded critic call. Contrast candidates use the same critic fence.

The code-owned `critic-v1` policy freezes the hard threshold at 0.80.
Any hard judgment at or above that threshold rejects the candidate.
Explanation restatement supplies a teaching-value score only; it never rejects.
The worker preserves source order rather than sorting ordered learning material.
Missing, mixed-type, or invalid judgments remain ungraded and cannot publish.
These initial thresholds do not establish calibrated accuracy or learning gains.

Each `content_assessments` row permits exactly one transmission under a 30-second
lease. The store reserves the semantic amount against the shared daily allowance
before sending. Known costs replace reservations; unknown outcomes retain them.
Expired leases become failed, including rows abandoned by terminal jobs.
Candidate storage extends job ownership for the bounded serial critic battery.
The provider call remains outside SQL and uses the existing eight-second limit.

Unavailable criticism leaves candidates saved with `critic_status=pending`.
Automatic retries stay within three total job attempts. Explicit retries may
resume the same unpublished batch up to five total attempts. They never repeat
generation or charge its reservation. Judged candidates are reused unchanged;
unjudged candidates receive new assessment rows on a new job attempt.
Restored work remains paused until explicit retry. Fully rejected batches need
revised input, not repeated criticism to search for a passing judgment.

`Store.CompleteJob` recomputes decisions from stored response judgments and
rechecks candidate identity, source revision, validation, and ownership.
It publishes accepted candidates only. Mixed batches become partial; fully
rejected batches fail without publishing. Export includes candidates, attempts,
policy identity, raw requests/responses, reasons, and usage. `ContentHistory`
returns content attempts separately from immutable learner review history.

Proof: pure-policy boundaries; durable lease, spend, crash, retry, and publication
tests; real HTTP worker integration; and bounded live public/synthetic controls.
Live controls report false accepts, false rejects, abstentions, model, and cost.
They do not establish broad publication quality or authorize activation.


Use durable job status with bounded HTMX polling. SSE is optional only after
proving useful incremental delivery through the Cloudflare Worker/Container
path; streaming and proxy timeout behavior remain unverified by this research.

### Cloudflare production hosting and recovery

Production requests to `scry.study` pass through Cloudflare Access and Worker
`scry-app-host`. Access's owner-email policy is not sufficient by itself: the
Worker verifies the JWT issuer, audience, and exact immutable owner subject
before forwarding to one `ScryContainer` Durable Object instance. nginx strips
client-provided identity/authority headers, injects the fixed application
owner ID, and proxies to Go on loopback. Go still checks canonical Host, the
trusted ingress peer, exact owner ID, its signed app session, and CSRF on
writes. Keep the backend private; a caller-supplied identity header is never
authentication.

Build and preview in isolated environments with synthetic data. exe.dev may
host development or recovery work, but it is not the production app origin.
Keep one production Container writer; no HA or automatic failover claim. Never
clone live jobs, model credentials, or recovery authority into previews.

Container storage is ephemeral. A cold start restores the newest complete
SQLite snapshot from private R2 through the append/read-only `scry-go-backups`
Worker. The production Worker runs `backup --require-remote` at 00:00 and
12:00 UTC and validates the remote receipt; graceful stop attempts an
additional remote backup. A crash, forced stop, or failed egress can lose writes
newer than the last verified snapshot. R2 is recovery, not the live database;
the application cannot delete snapshots or alter bucket policy. Archives use
SQLite-consistent backup, integrity/checksum and schema/binary metadata, unique
keys, and exact remote readback. Never copy only the live `.db` while WAL
writes continue; include separately stored assets in recovery.

Restore into an unused isolated path and reconcile uncertain jobs before
enabling external effects. A recent snapshot alone is not proof of successful
service recovery. Backup failures should be visible without disabling ordinary
review. D5 owns data-loss tolerance, retention and recovery time; no provider
SLA is asserted here.

## Foundation detour: MIS-59 (historical)

The operator rejected the saved-foundations interaction on 2026-09-12.
Its earlier technical evidence remains dated history in
[the design study](docs/design/concept-centered-study.md) and
[the rollout receipt](docs/qa/foundation-rollout-20260911.json).
The 2026-09-23 authorized concept-centered design replaces its routes and UI
without deleting original rows, changing immutable reviews, or claiming the
old usability trial succeeded. The v5 migration preserves historical
foundation-origin data, hidden from current Map/Stream; an older binary cannot
run after migration. It does not authorize production activation.

## Delivery slices and Linear mapping

MIS-48 owns the accepted rewrite and its execution evidence. The Scry project
was empty at the initial September 9 assessment; no speculative backlog was
generated. MIS-42 remains Estate's separate inventory repair, not this rewrite.

The first deliverable is an experience to judge, not a repository scaffold.
Ticket only the next ready slice and its real dependencies. A primary story
usually has one owning ticket; split when an independently useful boundary or
risk justifies it, and link all acceptance obligations back to the story.
Small technical prerequisites are allowed, but completed plumbing is not a
completed learner story. Deferred stories remain in the specification, not an
automatically manufactured backlog.

| Slice | Story coverage | Inspectable outcome / decision |
| --- | --- | --- |
| Experience and feasibility | S01, S02, S06; UX1-UX5; S09 boundary probe | Private synthetic-content prototype on the actual phone and exe HTTPS: press, answer, held feedback, Next, slow network, restart and stale tab. Decide D2-D4 and whether the browser/server split feels good before porting breadth. Auth/driver/scheduler probes are bounded risks, not a platform project. |
| First durable personal loop | S01, S02, S06, S08, S09; S10 baseline | Authenticated review of a small deliberately authored set, real SQLite history/schedules, next-day resume, and an independent restore before trusting real personal data. This proves review, not AI generation. |
| Capture to useful quiz | S03, S04; AI1 | A real selected input becomes usable, inspectable material through the real provider within approved spend; failure remains recoverable. No fixture fallback masquerades as generation. |
| Repair and ownership | S05, S07; remaining S08/S10 criteria | Correct bad content, preserve history, find/archive/export, operate and restore the actual complete data set. |
| Personal acceptance | All accepted stories and quality contracts | The operator can use their own material repeatedly without engineering intervention and approves the experience; report remaining failures/unverified claims rather than calling a demo adoption. |

The named research prototype may use clearly identified authored/synthetic
material. It is not a fake provider or a production feature completion claim.
Privacy and restore precede trusting personal data, not a hardening task postponed
until after launch. At replacement time, update repository instructions, runtime
commands, docs and dependencies in one coherent cutover; do not bypass the old
Rust gates or leave both runtimes as permanent supported products accidentally.

### Ticket contract

A ready ticket contains:

- User outcome and story/criterion IDs, linked to the authoritative spec revision.
- Exact scope and non-goals; relevant architecture decision and real dependencies.
- Concrete examples/fixtures, failure scenarios, and the observable acceptance
  oracle. State the actual surface: local logic, browser, provider, exe, recovery.
- File/interface ownership for concurrent work, with shared contracts agreed
  before edits. Parallel agents own independent slices, not the same mutation.
- Authorized environment/data/capabilities/spend and the evidence needed to close.

The issue owns execution state: current owner, accepted scope and open choices,
pause/blocker, and next permitted action. On interruption or handoff, retain the
selected checkout/worktree and committed base, identify relevant uncommitted
work, and link the exact tested revision/artifact and its evidence limitations.
PRs own change explanation and evidence links. Keep durable decisions in their
owning spec/design sections and link them; neither issue nor PR becomes a second
spec or a pasted session transcript. An In Progress status or earlier approval
does not override a later operator pause. Fixing a technical dependency can close
its ticket without closing the parent story; its missing criteria remain explicit.

## Agent engineering and independent QA

Use roles when the work needs them, not a permanent agent bureaucracy:

1. **Product owner + specification agent:** turn real goals/examples into a
   falsifiable story and resolve consequential choices. The human owns taste,
   priorities and acceptable risk. An agent does not infer those from passing tests.
2. **Designer/implementer:** choose the smallest vertical change, build it, and
   run the relevant checks. Request a spec change when requirements conflict;
   do not silently choose an easier behavior or expand into a reusable platform.
3. **Independent QA agent:** read the approved spec/criteria and environment
   instructions before reading the implementer's narrative. Exercise the actual
   surface and negative cases; inspect code afterward to diagnose failures.
4. **Integrator/operator:** reconcile evidence, accept or reject, merge/deploy
   within authority, and exercise the deployed slice. A merge is not a deployment.

One person/agent may perform several roles on a small change, but a meaningful
behavioral slice should have an independent acceptance pass. Do not require
multiple agents for a typo or spin up fleets merely to follow this diagram.
Parallelism belongs in independent implementation/review ownership, not in
passing a half-specified task sequentially between many agents.

### What QA evaluates

- **Contract correctness:** tests for real behavior/invariants, especially
  grading ambiguity, assisted recall, duplicate/stale submissions, jobs and
  atomic SQLite writes. Prefer real repository collaborators; fake only external
  boundaries needed for controlled failure. No tests of source wording/wiring.
- **Experience:** actual browser and real-phone interaction, long content,
  keyboard/focus/reduced motion, network delay, return/resume. Screenshots prove
  layout; a short recording and timing trace prove transitions better.
- **AI quality:** real selected provider, representative/held-out inputs, human
  keep/revise/reject and error reasons. Judge scores supplement, not replace,
  factual checks and operator usefulness. Label topic knowledge honestly.
- **Operations:** private deployed ingress, actual restart/release, job recovery,
  and independent restore. Local fixtures cannot certify these.
- **Product outcome:** repeated voluntary use and later unaided recall. Usage is
  not efficacy; a seven-day personal-use observation is a useful proposed review
  point, not a statistically valid retention study or a required daily streak.

Executable Go verification uses formatting, Go tests/vet, affected real
browser/runtime scenarios, and the source-bound exact-binary release smoke.
`bun run ci` is the host gate; `bun run ci:full` supplies pinned Dagger tooling.
The retired Rust application no longer owns current build or release commands.
Specification-only changes need semantic/link/traceability review, not model
runs, provisioned VMs, or simulated product acceptance.

### Evidence receipt and completion

Use a concise ordinary record, not a new QA platform. It identifies:

- Application revision/artifact and spec revision; story/criterion IDs.
- Environment/origin, device/browser, network profile, data provenance and
  authorized capabilities; sensitive values stay out of screenshots/logs.
- Scenario/actions, expected observation, actual observation, and evidence link.
- Per-criterion **PASS**, **FAIL**, or **UNVERIFIED**, with reasons and limitations.
- Coverage not exercised, outstanding decisions, and operator experience approval
  recorded separately from automated correctness.

Current implementation can fail a candidate criterion without creating an
accepted bug ticket automatically. An accepted story is done only when every
required criterion has adequate evidence on its required surface. UNVERIFIED is
not PASS. Fixture success, an HTTP 200, a green build, or a plausible screenshot
cannot substitute for the missing observation. QA cannot change acceptance
criteria to agree with implementation; proposed changes return to the owner.

An evidence failure loops to diagnosis and a bounded fix. A product objection
loops to the spec/design. No amount of implementation correctness obligates the
operator to accept an experience they do not want to use.

## Learning carried forward and sources

Keep atomic learning events, durable assisted-recall state, immutable historical
content, honest generation/provenance, bounded paid work, and restore proof.
Do not inherit the multi-app engine extraction, crate topology, five-face parity,
beta signup, mandatory activity ladder, legacy schemas, or deployment ceremony.

Repository evidence is historical observation, not a rerun of the target app:

- [Learning-science synthesis](docs/science/README.md): retrieval/spacing and the
  distinction between researched principles and product policy.
- [Generation research](docs/research/prose-to-quiz-generation.md) and
  [generation receipt](docs/evals/frictionless-generation-20260908.json): task
  fit, grounding, finite sets, misleading aggregate quality and bounded costs.
- [Review receipt](docs/qa/frictionless-review-20260908.json): held feedback,
  reveal persistence, displayed-choice grading, and honest empty generation.
- [Feedback research](docs/research/feedback-flywheel.md): attribute corrections
  to the actual content/generation rather than an unexplained negative signal.

Primary technical references, consulted for feasibility rather than runtime proof:

- [Go SQL transactions](https://go.dev/doc/database/execute-transactions),
  [connection management](https://go.dev/doc/database/manage-connections), and
  [template security](https://github.com/golang/go/blob/master/src/html/template/doc.go).
- [SQLite WAL and reset-bug advisory](https://sqlite.org/wal.html),
  [backup API](https://sqlite.org/backup.html), and
  [VACUUM INTO](https://sqlite.org/lang_vacuum.html#vacuuminto).
- [HTMX](https://htmx.org/docs/), [request synchronization](https://htmx.org/attributes/hx-sync/),
  and [preload](https://htmx.org/extensions/preload/): browser coordination is not
  server transactionality or an offline sync protocol.
- Former exe.dev VM references: [private HTTPS](https://exe.dev/docs/proxy.md),
  [identity](https://exe.dev/docs/login-with-exe.md),
  [machine tokens](https://exe.dev/docs/https-tokens-for-vms.md), and
  [migration/listener guidance](https://exe.dev/docs/migrating-to-exe.md).
  These document the retained VM workflow, not current production ingress.
- [Pinned Go FSRS release](https://github.com/open-spaced-repetition/go-fsrs/releases/tag/v4.0.0)
  and [selected pure-Go SQLite driver](https://gitlab.com/cznic/sqlite).
- [R2 consistency](https://developers.cloudflare.com/r2/reference/consistency/)
  and [S3 compatibility](https://developers.cloudflare.com/r2/api/s3/api/):
  unique completed snapshots, not assumptions about object lock or replication.
