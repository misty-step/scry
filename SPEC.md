# Scry prompt-first, concept-centered specification

Status: prompt-first Create intent reaffirmed on 2026-10-02; full redesign and
Rust-on-Cloudflare rewrite instructed on 2026-10-07. Its behavioral acceptance
remains [S11](#s11--create-useful-material-from-intent-us-013), together with the
accepted learning/history/privacy/recovery contracts below. Active replacement
code is Rust/WASM Worker + singleton SQLite Durable Object + private R2.
[The operational runbook](docs/runbook.md) still owns the deployed Go Container;
source, fixtures, and green gates do not activate its replacement.

[VISION](VISION.md) owns intent; [USER_STORIES](USER_STORIES.md) owns the learner
stories; [DESIGN](DESIGN.md) owns visual decisions; the earlier Rust
extraction strategy remains historical in [its migration record](docs/rust-migration.md).

## Authority and open decisions

**Current implementation authority (2026-10-07):** the operator asked to fully
redesign and rewrite the app as Rust and Cloudflare. The active replacement is a
ground-up Rust/WASM Worker, one SQLite-backed Durable Object, private R2 complete
archives, and escaped server-rendered HTML with a vanilla controller. It starts
in a fresh namespace and does not restore the retired Rust architecture or import
historical data. Create produces useful tailored reference/practice first, then
refines through actual attempts and feedback. No speech engine is selected.

**Production remains separately governed:** the Go/SQLite Container behind
`scry-app-host` remains the documented live runtime. Its exact source is retained
in Git at `c6ba395`; historical artifacts/stores remain recovery material. The
rewrite instruction authorizes implementation, not live migration, replacement
activation, reactivation of frozen writers, or new paid integration capability.
Concrete committed-source/target, private ingress, remote recovery, provider,
and phone evidence precede an explicitly approved release.

**2026-09-23 decision (MIS-162):** after rejecting the MIS-59 foundations
experience on 2026-09-12, the operator explicitly authorized the complete
concept-centered redesign. The accepted design is one question, historically Add and Map,
concepts with durable notes, selected capture modes, honest short-answer
checking/self-check/grade override, and a visual system (the 2026-09-24
“Ink notebook” redesign replaced the first “Scrying glass” direction).
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
interaction, independent recovery and live provider outcomes need fresh evidence
for the Rust replacement. A passing fixture or prior phone report does not establish them. The
earlier S09.2 different-account history-return observation remains unverified.
Old stores remain preserved separately; import or deletion needs a new decision.

Resolve these decisions as the relevant slice becomes ready. Record the decision
and reason here; remove obsolete alternatives rather than retaining two designs.

| Decision | Adopted choice | Basis / remaining evidence |
| --- | --- | --- |
| D1: personal material and failures | Build around what the operator wants to remember, not old QA datasets | The accepted candidate used photosynthesis, HTTP caching, and DNS examples. Specific future learning goals and sustained usefulness come from real use, not invented frustrations. |
| D2: response grading | Choice and exact-key/variant recall resolve locally; every other short recall answer uses bounded Jev `short-v1`; explain-level prose retains rubric `semantic-v1`. Close/unsure/failed checks offer self-check, and automatic grades allow one-tap correction. Every recorded grade names exact, Jev, learner, or reveal authority. | Operator authorized 2026-09-23; short-v1 accepts p≥0.85 with identity≤0.35 and injection≤0.20, rejects p≥0.90 with injection≤0.20, otherwise self-check. No liberal similarity or model call inside SQL; holdout quality remains to prove. |
| D3: assistance and correction | Reveal, answer-bearing cues and self-check exposure remain honest; immutable original grade plus separate override adjusts the current schedule without rewriting history. | Exact operation replay is durable; reading/intro is not cold success. |
| D4: experience approval | One question and one answer action, retained feedback and deliberate Next; Create/Map masthead and the [DESIGN](DESIGN.md) visual system | v5 phone/usefulness requires new observation; earlier approved flow is not blanket acceptance. |
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
| Work | Glass in scope `misty-step/scry`, linked to story/criterion IDs and source revision; legacy MIS/Linear links are provenance | What bounded change is someone doing now? |
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

## User stories and acceptance criteria

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

## Rust replacement architecture and durable boundaries

The operator's 2026-10-07 instruction selects the active ground-up replacement.
This section describes current replacement source, with acceptance limits stated
separately. The deployed Go Container is still governed by the operational
runbook; it has not been replaced by this source revision.

### One application and one state authority

```text
Private phone/browser — escaped HTML, self-hosted assets, vanilla controller
       |
Cloudflare Access → Rust/WASM Worker (JWT signature + exact issuer/aud/subject)
       |
internal capability → singleton LearningSpace Durable Object
       |                          |
SQLite durable transactions    bounded Model / Jev HTTPS
       |
complete JSON + photo archive → private R2 → exact remote readback
```

The replacement is a plain Rust Cloudflare Worker, not a Container, restored
retired Worker stack, or multi-application engine. One named SQLite Durable
Object owns all learning writes and background transmission ownership. R2 owns
private photo assets and complete recovery archives, not a second learning
writer. Active source has no Go application dependency, frontend framework,
HTMX requirement, frontend build, vector database, event bus, or generic agent
orchestration layer. Worker-build compiles Rust/WASM and its Worker loader.

`src/learning.rs` is pure scheduling, grading, and concept evidence.
`src/model.rs` / `src/engine.rs` own typed records and domain transitions.
`src/persistence.rs` owns validated row and archive encoding.
`src/generation.rs` owns bounded serialized model requests and validated material.
`src/runtime.rs` owns Access ingress, canonical origin, CSRF, SQLite durability,
external effects, alarms, private assets, and recovery. `src/web.rs` / `assets/`
own escaped HTML, self-hosted fonts, and presentation. The public product
surface remains private; no legacy API/CLI/MCP compatibility is inherited.

### The browser/server boundary

The browser does not own a grade, schedule, durable draft, or publication.
Ordinary same-origin POST forms preserve core behavior without JavaScript.
Every form has CSRF and an operation identity derived from a cryptographically
fresh server render nonce plus occurrence/revision/action/form identity.
The external vanilla controller enhances those forms and reads server-rendered
HTML. All untrusted model/user text is escaped; no inline executable source,
remote font, localStorage history, or offline mutation queue is required.

The first response can be visibly pending, but only one browser mutation can
remain unresolved. A lost response may follow a committed write: freeze its
exact payload and reconcile with the same operation. Changed payload with that
identity conflicts. A definite validation rejection retains editable input;
offline study pauses. Reconnect never automatically sends an answer.

Read-only preparation polling pauses for active/dirty forms, unknown answers,
or hidden/offline pages. A late response cannot replace a newer stage. Polling
neither retries paid work nor advances a question. Results remain until Next.
An unanswered occurrence and expected answer are not prefetched into hidden
markup. A note, goal title/intent, leaking concept name, old attempt, edit,
photo, or export cannot bypass the current assistance fence. The assistance
POST must commit before answer-bearing reference is served.

Private responses use no-store and a strict external-script/style policy.
Access denial removes the private DOM; pagehide removes retained private pages,
and back/forward restoration revalidates. Native keyboard and pointer paths,
320/390/1280 px, both color schemes, no-JS forms, and actual phone acceptance
need real observation independently of render tests.

### Domain state and transactions

Saved goals contain learning intent and optional photo/transcript. Concepts
contain prerequisite links and append-only notes. Questions retain immutable
content versions and full current FSRS cards. An occurrence snapshots exactly
the presented content, schedule version, answer, assistance, and held result.
Immutable events, separate overrides, operation receipts, jobs, assessment
leases, allowance reservations, explicit feedback, preferences, and backup
status complete the durable state.

A short SQLite transaction commits the complete domain transition:

```text
validate owner/origin/CSRF and operation identity
  identical committed operation -> return its saved result
  changed payload or stale occurrence/content/schedule -> conflict
  resolve exact/choice grade or stage one saved bounded assessment
  save immutable event + full card + held result + operation receipt atomically
commit before returning success
```

The singleton persists typed rows in its SQLite Durable Object using a
revision-fenced synchronous transaction. No external call happens inside it.
The runtime saves transmission ownership and allowance before an await, then
loads fresh state and checks lease, source/content revision, occurrence,
schedule version, and lifecycle before accepting an external completion.
A stale completion cannot publish or rewrite an event.

The Rust scheduler ports the pinned Go FSRS adapter and its complete state:
due time, stability, difficulty, scheduled days, repetition/lapse counters,
state, last-review time, and remaining steps. `learning::SCHEDULER` and
`learning::ALGORITHM` preserve the original Go identity strings. Events append
`exact-v1`, `short-v1`, `semantic-v1`, or `learner-v1` authority as appropriate.
`tests/fsrs_golden.rs` compares 260 full-state trajectories against the exact
Go reference, including miss/relearning and equivalent-time replay. That proof
is bounded scheduler compatibility, not retired-engine parity, personalized
retention, certified understanding, or learning gains.

The target uses a fresh schema/namespace. US-001 criterion 1 continues to require
Go v4→v5 foundation-row preservation in the retained Go runtime. That migration
and its tests remain intact; the fresh Rust namespace never performs the upgrade
or imports the live database. The old Go source remains
in Git at `c6ba395` with its operational artifacts, and frozen Rust/Postgres/
Worker stores remain separate recovery material. Foundation routes stay retired.

Private routes are `/` (study), `/create` (`/add` alias), `/map`,
`/goals/{id}` (reference/input/preparation/refinement), `/concepts/{id}`,
`/questions/{id}/edit`, `/history`, `/settings`, `/gate`, `/photos/{goal_id}`,
and complete `/export`. Review POST routes are intro, answer, reveal, help,
self, retry, override, and next. Goal routes pause, focus, archive, retry, and
refine; concepts practice, archive, and feedback; questions edit, archive,
dispute, and fix. Preparation, practice, and maintenance retain version and
operation fences. Operator backup/isolated restore capabilities are separate
from browser-owner authorization.

### Generation and learning policy

Create saves private intent before bounded preparation. A word, phrase, or long
ramble can produce a useful general-knowledge starting point; no required
profile, assessment, source import, or approval inbox intervenes. An optional
photo is validated and transcribed before its explanation and practice are
prepared. Text/photo/caption byte bounds remain 32 KiB / 4 MiB / 1 KiB.
Pasted URLs remain private intent: the replacement does not infer research,
open them, or restore a legacy public Topic/Link API.

Generation produces a bounded batch of teachable concepts, clear notes,
prerequisites, and useful practice variation. Claims attributed to supplied
material quote that saved text exactly; general knowledge is labeled and carries
no fabricated evidence. A learning request is not factual support. Model/user
text is serialized data and rendered inert. Validation rejects oversize or
unsupported content without silent truncation, retains failure/candidates and
usage, and labels partial useful output honestly.

Refinement explicitly uses saved feedback, actual attempts, confusion,
corrections, and existing concepts. Existing notes append versions; published
questions retain attribution and history. A prepared fix is a saved draft for
its exact question/version. It does not overwrite content upon completion.
The learner's versioned Save chooses it; an edit or newer source/content
revision invalidates stale work. Changing answer/rubric-bearing wording removes
an invalidated semantic rubric rather than pretending it still applies.

Choice, exact key, and authored recall variants resolve locally. All other short
answers stage `short-v1`: independently bounded verdict, exact-identity, and
injection judgments. Accept requires p≥0.85, identity≤0.35, injection≤0.20;
reject requires p≥0.90 and injection≤0.20. Missing/malformed/close judgments
stay ungraded. Explain prose retains independently authored required ideas and
`semantic-v1`; incomplete/incorrect shadow classes do not become automatic
misses. Saved uncertain answers offer learner self-check; failed checks offer
an explicit new assessment with no identical-row resend.

Each recorded result names exact, Jev, learner, or reveal authority. Automatic
unaided grades allow one immutable correction and a consistent current card;
original attempts remain unchanged. Assistance/intro exposure and deliberate
extra practice do not fabricate unaided success or move the FSRS schedule as
cold recall. Self-check retains the submitted attempt’s assistance snapshot and
records answer exposure for subsequent practice before applying learner authority. Question feedback/dispute and a deliberate schedule reset remain
separate from a grade override.

Selection introduces unseen ideas prerequisite-first, interleaves eligible due
practice, prioritizes focused new concepts, excludes paused goals' new material,
and keeps their due practice. Light/steady/intense cap new concepts at 3/6/12
per rolling day. Concept new/learning/solid/fading and recall/brightness derive
from actual evidence under the pinned pure policy. Read/know observations,
helped attempts, unaided successes, and misses remain distinct. An estimate is
never a guarantee or mastery score.

### Prepublication content critic: US-004

Validated candidates and attribution are saved before any critic request.
A batch contains at most 60 candidates; larger output fails rather than claiming
complete coverage. When configured, the critic independently judges each
applicable hard defect: unsupported source claim, contradicting evidence,
changed qualification, missing context, ambiguous/indefensible answer, leaked
answer, overlapping choice, misaligned rubric, or adversarial content.
Source, choice, and rubric-specific defects apply only to those types.

`critic-v1` freezes the hard threshold at 0.80. Any applicable hard judgment at
or above it rejects that candidate. Teaching-value score can rank only, never
veto. Missing/mixed-type/malformed judgments stay ungraded and cannot publish.
These thresholds are product policy, not demonstrated calibration or efficacy.
Without a configured critic, skipped criticism is durably recorded without
reserving extra allowance or pretending independent checking happened.

Each assessment has one transmission ownership and an atomic reservation in the
shared generation/meaning allowance. Calls happen outside SQLite. Known usage
settles reservation; an interrupted or unknown sent outcome retains it and
requires deliberate resolution. Candidates, raw responses, model attribution,
rejection reasons, and known/unknown usage remain in export/history.

Critic failure retains candidates and judgments. A retry checks only unresolved
criticism with new send ownership, reuses already judged candidates, and never
regenerates the saved batch or silently resends an assessment. Publication
recomputes decisions from saved judgments and rechecks source revision,
candidate identity, lifecycle, and job ownership atomically. Mixed accepted/
rejected output is partial; no valid accepted output cannot be called ready.
Restored uncertain work stays paused. Live critic controls and representative
human review remain distinct from deterministic boundary tests.

### Private Cloudflare target and recovery

The replacement Worker verifies the Access JWT RS256 signature against issuer
keys, issuer, configured audience, time claims, and exact immutable owner
subject. Cloudflare's email policy alone is insufficient. It accepts the
canonical origin; alternate hosts redirect reads only and reject mutations.
An internal capability authenticates forwarding to the singleton. CSRF,
operation identity, and same-origin checks protect browser writes. Development
identity is loopback-only and refuses live provider/operator capabilities.

`src/runtime.rs` persists complete snapshot metadata and referenced photo bytes
as a versioned JSON archive in private R2. A backup is verified only after exact
remote size/checksum/full-byte readback and archive validation. Daily alarms and
pre-release/operator backup preserve the approved 30-day retention policy.
A failed upload/readback stays visibly failed in Settings while ordinary review
remains available. R2 durable storage alone is not independent recovery proof.

Restore requires an unused isolated Durable Object namespace, compatible
Worker/assets/configuration retained independently, an integrity-checked complete
archive, and a separate scoped restore capability. It verifies all content and
photo bytes, clears live backup receipts, and keeps uncertain jobs/assessments
paused with no inherited paid integrations. Never overwrite acknowledged live
writes or reactivate a historical writer during rehearsal. Daily/pre-release
snapshots target RPO 24 hours and RTO 60 minutes; measure actual private-service
recovery and name untested provisioning/DNS/configuration steps.

The deployed Go Container's origins, retained SQLite backup gateway, shutdown,
restore, and activation procedures remain in `docs/runbook.md` and
`deploy/cloudflare-hosting/README.md`. They are historical/current-production
operational authority, not the replacement build or acceptance surface.

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

## Delivery slices and work provenance

[Glass](https://mirrodin.tail5f5eb4.ts.net) in scope `misty-step/scry` owns
current work state; [README](README.md#start-or-resume-work) owns the work route.
Migration reconciliation and writer repointing belong to the off-Linear work,
not this specification. Imported descriptions require reconciliation with
current intent before execution, not automatic permission to ship an old build.

The table below records the earlier Go delivery sequence, not the approved
Rust replacement's implementation plan. Legacy
[MIS-48](https://linear.app/misty-step/issue/MIS-48/execute-the-personal-go-sqlite-htmx-scry-rewrite)
and MIS-162 retain that provenance; MIS-42 was Estate's separate inventory repair.

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
commands, docs and dependencies coherently. Preserve historical source/artifacts
and current-production recovery, without keeping two supported application
writers or relabeling historical proof as replacement acceptance. Activation
remains a separate approved operational action.

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

Executable replacement verification uses `cargo test --locked`,
`cargo clippy --all-targets -- -D warnings`,
`cargo check --locked --target wasm32-unknown-unknown`,
`worker-build --release --locked`, affected real Worker/browser scenarios, and
the source-bound exact-artifact smoke. `bun run ci` is the source-snapshot gate;
worktree evidence is development-only and release output requires committed
source. Old Go and retired Rust receipts do not transfer. Specification-only
changes need semantic/link/traceability review, not default model runs or a
simulated product acceptance.

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
- [Concept-centered round](docs/design/concept-centered-study.md) and
  [September interface rounds](docs/design/redesign-2026-09-24.md): retained
  decisions/proposals, not a competing prompt-first front door.
- [Learning reference pack](docs/research/learning-science-references.md) and
  [AI learning experiments](docs/research/ai-learning-design-brainstorm.md):
  historical research and ticket sketches, not mandatory import/approval steps.
- [Generation research](docs/research/prose-to-quiz-generation.md) and
  [generation receipt](docs/evals/frictionless-generation-20260908.json): task
  fit, grounding, finite sets, misleading aggregate quality and bounded costs.
- [Review receipt](docs/qa/frictionless-review-20260908.json): held feedback,
  reveal persistence, displayed-choice grading, and honest empty generation.
- [Feedback research](docs/research/feedback-flywheel.md): attribute corrections
  to the actual content/generation rather than an unexplained negative signal.

Historical Go technical references, retained for provenance rather than
replacement-runtime proof:

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
