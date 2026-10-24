# Scry specification: concepts first

## What it does

Scry helps its owner understand concepts, study reference material, and practise
what he knows. Concepts are the primary learning destination. Exact text
memorization remains a distinct activity with its existing Library, Studies,
fading cues, word-by-word verdicts, and memory schedule.

## What it enables

The owner can move from a concept to its explanation, saved questions, original
material, and related concepts, or resume practice without losing an answer.
The longer-term direction includes many independent materials and quizzes per
concept, varied source formats, and evidence-based transfer between related
concepts. Those capabilities are not implied by the current interface.

This is the single behavioral reference for Scry, for one owner. Words follow
[GLOSSARY](GLOSSARY.md). QA runs against this file's US,
S, and quality-contract IDs. They describe accepted intent, not implementation
or deployment success. Never weaken a criterion to fit code; change intent
through review, and retire identifiers explicitly rather than reusing them.

[VISION](VISION.md) explains the goal; [GLOSSARY](GLOSSARY.md) owns the words;
[DESIGN](DESIGN.md) owns visual decisions;
[ADRs](docs/adr/README.md) own technical decisions; [QA](docs/qa/system.md) owns
verification procedures and evidence limits. [README](README.md#start-or-resume-work)
is the work entrypoint. Dated receipts and proposals are not competing requirements.

## Scope and acceptance status

On 2026-10-24 the owner made concepts the primary primitive and rejected an
empty saved-text Library as the front door ([#388](https://github.com/misty-step/scry/issues/388)).
Home (`/`) introduces concepts; Phase B adds the explicit manual Create concept path. It does not select
or consume either practice stream. Resume practice opens the existing concept
occurrence at `/notebook`; Text practice opens exact recall at `/recall`, with
Library secondary there. Both streams retain their current questions, protected
answers, held results, and histories. Concept pages organize existing facts as
Study, Practice, Resources, and Related (Builds on / Used by). This priority
amendment does not import or rewrite data, activate paid work, add source
formats, create independent material/quiz collections, or infer mastery across
relationships. Preparation readiness is not spend authorization or a hard
monthly budget guarantee; production paid generation stays off.

On 2026-10-20 the owner made exact recall Scry's first purpose
([#296](https://github.com/misty-step/scry/issues/296)); see
[Exact recall first](#exact-recall-first-2026-10-20) for what that retired and
narrowed. The 2026-10-24 amendment supersedes that navigation priority, not its
exact-recall behavior. Prompt-first Create (2026-10-02) governs concept preparation. The 2026-10-07 rewrite uses
these contracts without inheriting historical runtime acceptance. The revived
notebook from D6 is archived read-only and exact recall starts fresh. US-001.1's Go schema 4→5 preservation and
US-005/S03's explicit-mode capture remain historical Go obligations at Git
`c6ba395`; US-012 is retired with its ID reserved. Old Go and retired Rust/preview
receipts do not certify the candidate, and a green gate does not activate it.
[The runbook](docs/runbook.md) owns operational records; serving provenance and
revival remain separately governed, not inferred from this specification.

Useful real material, physical-phone experience,
deployed private ingress (including S09.2 account/history return), live provider
outcomes, and independent recovery still require their own evidence. The
existing rewrite acceptance stays open. No historical-store import, deletion,
writer reactivation, new paid capability, or deployment permission follows here.

Scry has one owner. Accounts or tokens for anyone else, public signup or
anonymous reading, a public Library, billing, collaboration, universal import,
a course/chat product, a manual graph editor, MCP, a compulsory foundation
detour, and an invisible offline mutation queue are out of scope. Every
exact-recall action is available through the API and the `scry` CLI (AG1).
Dictated input is text, not an assumed speech engine or automatic recording
integration; it is graded as typed. A mandatory activity ladder is excluded;
the exact-recall Ladder is a cue scaffold, not a required activity sequence.

### Concept Material compatibility floor (Phase A)

Phase A of [#124](https://github.com/misty-step/scry/issues/124), tracked in
[#393](https://github.com/misty-step/scry/issues/393), establishes recovery and
read compatibility before authoring is enabled. It accepts an explicitly
owner-authored Concept with no Goal, at least one independently addressable
Material, and immutable revisions of exact supplied text. It invents no
preparation, questions, prerequisites, learning observations or provider spend.
Existing generated Concepts retain their Goal, Note history, prerequisite and
question validation. Their Note chain is one read-only Material projection,
not a copied or split collection.

**CM1: compatible retention.** A complete archive or store containing these
records must retain every Material, revision, acknowledged operation and old
history across read, fresh-space restore, and the first unrelated ordinary
write. New-format state, including a retained capability marker alone, must
not be silently accepted by a reader that would discard it. The retained floor
artifact must be a functioning compatible writer before Phase B authoring is
activated. Refusal by an older artifact alone is not rollback, and an older
snapshot must never overwrite accepted writes.

**CM2: honest read-only floor.** Home and Concept detail can show compatible
restored Concepts and all supplied Material revisions without inventing a
legacy Note or exposure. Existing Goal-backed exposure and assistance fences
remain unchanged, including for a supplied Material attached to a generated
Concept. This phase exposes no normal authoring API, CLI command or UI form;
no new source formats, quizzes, graph prediction or paid capability follows.

### Supplied Concept authoring (Phase B)

Phase B of #124, tracked in [#400](https://github.com/misty-step/scry/issues/400),
uses the released CM1/CM2 compatibility contract. Its activation is dependent on
qualification, publication and promotion of that floor, not merely a source
merge. The floor's exact published artifact is the compatible rollback writer.

**CM3: supplied authoring.** From empty Home, Create concept saves a named Concept
and its first supplied-text Material atomically. A Concept can have multiple
independent Materials; matching names, titles or text never deduplicate them.
Add study material appends another Material. Revise material requires the expected
current revision and appends immutable wording; stale revisions conflict.
Names/titles are nonempty and at most 200 UTF-8 bytes; optional summary is at
most 2,000; text is nonempty and at most 32,768. Exact supplied UTF-8, CRLF,
tabs and trailing newlines survive. Unknown fields are refused. Full graph,
row and complete-archive bounds apply to the candidate including its receipt
before commit, with no partial Concept, Material, revision or operation.

**CM4: honest free surfaces.** These create/read/add/revise actions share the
engine through native forms, versioned API and CLI. Existing Owner auth,
principal, CSRF and exact operation replay apply; only POST can mutate. Reads
of new Material routes and authored Concepts use snapshots, without stream
selection or exposure. Current Concept and exact-Prompt protection covers
all reference revisions, JSON and replays; no read or authoring action Reveals
an answer. Validation, stale edits and unknown responses keep the draft until
a deliberate Save, Cancel, Back, reload or same-operation reconciliation.

Manual Create concept remains available when AI is off, on an unpaid token,
and while external work is restored-paused. Prepare material with AI is a
separate existing path. No Goal, Job, Question, Event, schedule, provider call,
reservation or spend is invented. With no real questions the page says
“No quizzes/questions yet” and offers no Practice action. Generated Concepts
retain their existing Note/Goal/refinement behavior; the old Note chain stays
one read-only Saved explanation. The Map includes authored Concepts without
fabricating relatedness or mastery. URLs remain literal text, not fetched
sources. Deletion, quiz authoring, relations, prediction and multimodal
source ingestion are out of scope.

Qualification uses real engine/API-created state, the complete retained
historical baseline, zero-egress API/compiled CLI/native browser journeys,
SQLite restart and complete R2 fresh restore. The published floor must preserve
accepted Phase B records and receipts on its first unrelated ordinary write;
pre-floor refusal is a separate negative control. All existing native,
historical, runtime and browser gates remain required. No down-conversion or
old snapshot may replace accepted writes.

### Exact recall first (2026-10-20)

Historical priority decision: navigation priority is amended above on 2026-10-24;
the exact-recall behavior and the other dated decisions below remain binding.

The owner confirmed this direction on 2026-10-20 (#296, with his changes: a
generation agent with tools, one owner, Roman Missal 2011 wording, the slip
default, owner tokens, an archived revived notebook, no spend decision yet,
and cuts to unrequested gates). US-014 to US-021 add the exact-recall
stories. These existing contracts are retired or narrowed as of that date:

| Contract | Change on 2026-10-20 |
| --- | --- |
| Prompt-first front door (Experience contract, S11, US-013) | Narrowed: governs the understanding notebook's Create. Exact recall starts by searching the Library (US-015) and creating with the generation agent (US-019). |
| No inferred web search (S11.3, US-013.2, Create bullet) | Narrowed: still binds Create. A request to create a Work authorizes the generation agent to search the web for that named Work (US-019). |
| Local grading forbidden beyond exact key (S02.5, US-003.1, US-008) | Narrowed: govern answers outside exact recall. Exact-recall answers are graded locally by `exact-v2` (US-017); its one enumerated Slip tolerance is not the liberal similarity those stories exclude. |
| US-008 holdout evidence before live activation | Retired: Jev is off the exact-recall path and the owner did not ask for this gate. |
| AI1 operator-approved real-input set | Retired: usefulness is judged by the owner's use and Self-checks (US-019). |
| US-004 critic-v1 gate and 60-candidate bound | Narrowed: concept generation only. Library Versions publish on deterministic Self-checks (US-019), whatever their Unit count. |
| Prerequisite-first concepts and 3/6/12 new concepts (S08.2, US-010.3, US-011) | Narrowed: concept goals only. Studies introduce Units in reading order at the same Pace numbers (US-016). |
| Owner identity (S09.1) | Extended: an unexpired, unrevoked Owner token is owner identity (US-014). |
| Bespoke QA identity (S09.4) | Retired: machine access uses Owner tokens (S09.3, US-014) or synthetic environments; removal of the QA service identity is a ticket. A verification-only QA sign-in to the isolated `scry-qa` space remains (2026-10-21, S09.4). |
| One-way revival (D6, revival dry-run boundary) | Retired for new work: the revived notebook is archived read-only, nothing deleted, and exact recall starts in a fresh space. |
| Masthead navigation (US-002.4) | Library is the primary destination; the wordmark returns to practice. Understanding retains the old Create/Map paths quietly. |
| `exact-v1` trim equality | Narrowed: history keeps its attribution; new exact-recall Attempts use `exact-v2`. |

## Adopted behavioral policies

The D identifiers are retained for existing references; implementation choices
belong in ADRs, not this table.

| ID | Binding policy |
| --- | --- |
| D1 | Build around what the learner wants to remember, not old QA datasets. Sustained usefulness needs real use. |
| D2 | Honest grading and uncertainty follow S02 and US-003/US-008 outside exact recall; exact-recall answers follow US-017's deterministic `exact-v2`. |
| D3 | Assistance and immutable corrections follow S02/S05 and US-007; exposure is not cold recall. |
| D4 | One question, one answer action, held feedback and deliberate Next; concept-first navigation and explicit Text practice access. Visual decisions live in DESIGN; earlier phone approval is not blanket acceptance. |
| D5 | Daily/pre-release off-machine complete backups, 30-day retention, RPO 24h/RTO 60m targets (not guarantees). Provider key cap $25/week; application $3.50 per rolling 24h; conservative generation reservation $0.50. Generation, Jev and criticism share the allowance. Unknown sent cost retains its reservation and requires deliberate resolution, not free retry. Exact-recall practice makes no provider call. Enabling production generation for the generation agent, and its caps, is a separate owner spend decision that arrives with US-019; until he makes it these numbers stand and production generation stays off. |
| D6 | Amended 2026-10-20: the revived notebook is archived read-only with nothing deleted, its final verified backup retained and no writer attached; exact recall starts in a fresh space. The rest of this row is historical. No legacy API parity. Ordinary candidates start fresh; the authorized 2026-10-15 revival alone imports the final consistent Go dataset once, with preserved learning history and media. Preserve historical stores, source and recovery artifacts separately; no general import, deletion or simultaneous historical writer is implied. [ADR 006](docs/adr/006-one-way-revival-custody.md) owns custody and cutover decisions, not an acceptance receipt. |

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

Home introduces concepts, with a clear learning purpose and Create concept rather
than an empty text search. It offers Resume practice when an occurrence exists,
without showing its answer or selecting another one. The concept Stream at
`/notebook` retains its question, introduction, preparing receipt and first-run
states. The exact-recall Stream at `/recall` opens into the current Prompt or held result, an honest
caught-up state, or Library entry when no Study remains active (US-018). Neither practice stream opens an administration dashboard.
Concepts are teachable ideas. Generated goals express preparation intent, and
their original Note chain is one read-only Saved explanation. Independently
supplied Materials have their own immutable wording history (CM3/CM4). The Map is the place
to inspect goals/concepts, focus or pause, and see a labeled estimate
from real observations, not a certified mastery score.

Creation precedes the study loop when starting a new interest: prompt → saved
preparation → useful reference and practice → refinement from real interaction
([S11](#s11--create-useful-material-from-intent-us-013)). A returning learner can
continue retrieval-led study directly. “Quiz-first” is not a compulsory pretest,
material-import workflow, or mastery profile before generation.

```text
scry                                    Concepts   Create concept
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
- Create has one private intent field and optional photo (US-013/S11). It
  prepares concepts; exact-recall Works are found or created
  through US-015/US-019. Text,
  including URLs, never infers public search or link reading; photos transcribe
  before preparation. Saved goals expose captured intent, useful reference,
  practice, refinement, and honest failure. `/add` is an alias; text/URL prefill
  remains editable and private when preparation is available. When preparation
  is unavailable, say so before submission, omit preparation controls, and retain
  any unsent text visibly. Do not imply saving a free draft or support for URL
  fetching or videos. Readiness does not authorize paid work. Historical explicit Topic/Link callers belong
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
  action; it does not invent due work or silently reset schedules. Stopped
  preparation is not described as running: the Stream links directly to the
  saved request and its recovery options instead of asking the learner to wait.
  Imported requests that never finished remain recoverable on their own page
  and the Map, not as the Stream's next learning action.
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
  stays ungraded for learner self-check. Exact-recall answers are outside this
  criterion and graded by `exact-v2` (US-017). Explain-level authored required-idea
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

S04 governs concept material. Versions made by the generation agent follow
US-019 (narrowed 2026-10-20).

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
  concepts per rolling day at 3/6/12 for light/steady/intense pace. Narrowed
  2026-10-20 to concept goals; Studies follow US-016.
- **S08.3:** Concept state distinguishes new/learning/solid/fading and tallies
  unaided/helped/missed events. Predicted recall is explicitly an estimate;
  navigation, animation, or a note read never manufactures a learning event.

Proof: prerequisite/pace selection, concept evidence and next-day
resume in focused learning/store checks and browser use. Efficacy claims
require separate delayed unaided recall data.

### S09 — Keep the application private

As the owner, I want easy private access from my phone and computer without
maintaining a signup product or exposing my learning material.

- **S09.1:** Only the approved owner identity, through Access or an unexpired,
  unrevoked Owner token (US-014), can access the owner's notebook; missing,
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
- **S09.4:** Retired 2026-10-20; the identifier stays reserved. The bespoke QA
  service identity and notebook are no longer a contract: machine access uses
  Owner tokens without paid work (US-014) or synthetic environments (S09.3).
  The former text is at Git `cb93333`. On 2026-10-21 (harness verification
  policy, phaedrus 2026-10-21) a verification-only QA sign-in remains: the
  dedicated AgentMail account (`QA_EMAIL`), admitted by an exact-email Access
  policy and signed in through the Owner's identity provider, reaches only the
  isolated `scry-qa` space, with Owner capabilities there. It never reaches the
  Owner's notebook, operator routes or backup. It is a verification access
  rule, not a learner-facing contract.

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
- **S10.4:** While restored background work is paused, preparation and meaning
  checks say so and link to Settings, not indefinite progress or ineffective
  retries. The notebook's own learner can deliberately choose Resume preparation
  there, without an operator key. This enables queued and future work only;
  historical paused/unknown jobs, assessments, reservations and learning history
  remain unchanged. Same-origin/session protection and exact-operation replay
  apply. A new resume after resumption conflicts without changing state.

Proof: process interruption, concurrent review/backup, incomplete upload, and a
real fresh-environment restore. Persistent disk or VM cloning is insufficient.

### S11 — Create useful material from intent (US-013)

Approved product acceptance, reaffirmed 2026-10-02; not an implementation receipt.
Amended 2026-10-24: S11 governs concept preparation, reached through Prepare material with AI
from concept Home. S11.3's search boundary does not restrict US-019.
If preparation is unavailable, the interface states that before accepting a
request; it never enables a provider or claims a free save-only draft.
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

### S12 — Inspect the changes in this build

As the learner, I can follow Changelog from the private application's navigation
to useful release notes included in the serving build, not newer unpublished
work. The page names the semantic version and full source revision, works in
light and dark themes on phone and desktop, and does not read or change my
learning state. It retains the application's private access boundary. Health
responses identify that same version and revision without exposing notebook data.

## Cross-cutting quality contracts

These are candidate budgets to approve through the experience slice, not
measurements of the current or proposed app. Keep latency, generation quality,
reliability, usability, and learning outcomes separate.

| ID | Proposed contract | Measurement / acceptance |
| --- | --- | --- |
| UX1 | Input acknowledgement p95 <100 ms | Pointer/key action to visible pressed/pending response on the owner's actual phone, including an induced 300 ms RTT profile; no network dependency for acknowledgement |
| UX2 | Deterministic graded feedback p95 <300 ms, including `exact-v2` | Submit to committed, visible result, on a declared <=100 ms RTT profile; include DB/render/browser time, not just handler time; no claim that AI semantic grading meets this budget |
| UX3 | Prefetched Next p95 <150 ms; existing first review p95 <2 s; capture to first usable quiz p95 <20 s | Report warm/cold state, payload sizes, network/device, sample count, failures/timeouts and measurement endpoints separately; pending UI is not completion, and failed jobs remain in the outcome report |
| UX4 | Usable at 320 CSS px, with keyboard and 200% text zoom | No horizontal clipping; visible focus, labeled controls, accessible result announcements, >=44 CSS px primary targets, adequate contrast, reduced motion; test long questions and explanations, not only short fixtures |
| UX5 | Smooth but honest continuity | At most one speculative next prompt, no speculative success/mastery; canceled gestures and stale responses do not submit; loss/conflict recovers without an invisible offline queue |
| AI1 | Retired 2026-10-20 | The operator-approved real-input set is no longer required; usefulness is judged by the owner's use and US-019's Self-checks |
| AG1 | Agent-drivable | Every exact-recall create, read, update and delete action is available through the versioned API and the `scry` CLI with machine-readable output, driven by an Owner token (US-014). The phone UI calls the same behavior; no exact-recall action is web-only |
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
- Concept generation only (narrowed 2026-10-20; Library Versions follow
  US-019): when configured, US-004's critic independently judges each applicable hard
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
- Historical (D6 amended 2026-10-20): before the one-way revival import, an authorized read-only dry run traverses
  the complete preserved dataset inside private Cloudflare. It writes nothing
  and reports only fixed refusal class names and their counts, never content or
  identifiers. An incomplete traversal is a failure, not a complete report.
  Further shape-specific repairs follow that complete report as one batch;
  repeated first-refusal imports are not the diagnostic workflow.
  Before writing the production store, rehearse the complete import with the
  same retained dataset and candidate on disposable private hosted storage.
  Native archive/history/scheduler refusals must be included, not only the Go
  projection checks. An incomplete native traversal is failure; never report it
  as an empty refusal table or retry production to discover the next defect.

## User stories

The original US IDs and numbered criteria below remain binding alongside the
S criteria above, as narrowed in [Exact recall first](#exact-recall-first-2026-10-20).
A story whose evidence line reads `Evidence: pending` is accepted intent not yet
built; the ticket that implements it replaces that line with its walks and tests.

US-014 to US-021, added 2026-10-20, govern the exact-recall activity for its owner
([#296](https://github.com/misty-step/scry/issues/296)). In them "I" is the
owner, whether he acts on his phone or through an agent holding an Owner token.
Every criterion holds through the API and the `scry` CLI (AG1); US-018 adds the
phone. S06 retry/conflict, S09 privacy and S10 recovery apply unchanged. Overlapping IDs express the same contract at different journey
scales, not competing authorities. Policy names (FSRS, Jev, short-v1, semantic-v1,
critic-v1) retain their accepted compatibility meaning; technical decisions and
verification commands live outside this behavioral reference.

| Story | Related end-to-end criteria | Scope |
| --- | --- | --- |
| US-001 | S05, S07 | Current concepts/history; criterion 1 is retained Go migration only |
| US-002 | S01, S02 | Current study loop |
| US-003 | S02, S06 | Current grading outside exact recall |
| US-004 | S04, S06 | Current prepublication criticism, concept generation only |
| US-005 | S03 | Superseded front door; historical Go explicit-mode callers only |
| US-006 | S02, S04, S07 | Current concept reference |
| US-007 | S02, S05, S06 | Current grade correction |
| US-008 | S02 | Current short-answer policy outside exact recall |
| US-009 | S07, S08 | Current Map/evidence |
| US-010 | S07, S08 | Current focus, pause and pace for concept goals |
| US-011 | S08 | Current prerequisites for concept goals |
| US-012 | S11.5 excludes reinstatement | Retired; ID reserved |
| US-013 | S11, S04, S06, S09, S10 | Create and refinement for the understanding notebook |
| US-014 | S06, S09, AG1 | Exact recall: Owner tokens, API and CLI (see criterion walks) |
| US-015 | S07, AG1 | Exact recall: Library, find before create (evidence pending) |
| US-016 | S01, S02, S06, S08 | Exact recall: Study, Ladder and schedule (see criterion walks) |
| US-017 | S02, UX2 | Exact recall: `exact-v2` verdict and Diff (see criterion walks) |
| US-018 | S01, S02, UX4 | Exact recall on the phone (see criterion walks) |
| US-019 | S04, S06, D5 | Exact recall: generation agent (evidence pending) |
| US-020 | S02, S08 | Exact recall: ordered lists (evidence pending) |
| US-021 | S02, S08 | Exact recall: keyed mappings (evidence pending) |

## US-001 Find and reuse saved understanding

Statement: When I explore what I am learning, I want saved concepts and notes revisitable, so that I can connect prerequisites across questions without losing earlier study.

Criteria:
1. WHEN schema 4 upgrades to schema 5, THE SYSTEM SHALL preserve foundation rows and relations as historical data; foundation-origin concepts SHALL NOT appear as newly generated concepts in the Map or Stream.
2. WHEN I inspect a concept page, THE SYSTEM SHALL show its current notes under Study, saved questions and actions under Practice, original material under Resources, and prerequisite links in both directions under Related (Builds on and Used by) while preserving old presented wording and review history.

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
4. THE SYSTEM SHALL keep Concepts and Create concept in the masthead, with the wordmark returning to concept Home. Home SHALL NOT select either practice stream or reveal a protected answer; it SHALL offer Resume practice for a saved concept occurrence at `/notebook`. Quiet Text practice SHALL retain the exact stream at `/recall` and its Library access, including current Prompts and held results. Maintenance SHALL NOT become an always-visible inspection dashboard.
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

Narrowed 2026-10-20: US-003 governs answers outside exact recall. Exact-recall
answers are graded locally by `exact-v2` (US-017), whose single enumerated Slip
tolerance is shown in every Diff and is not liberal similarity.

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

Narrowed 2026-10-20: US-004 applies to concept generation. Versions from the
generation agent publish on deterministic Self-checks (US-019), and the
60-candidate bound does not limit a Version's Units.

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

No-gos: no liberal similarity rule, hidden automatic retry, or claim that an untested holdout passed.

Narrowed 2026-10-20: US-008 governs short answers outside exact recall. Its
former requirement of holdout evidence before live activation is retired.

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

Narrowed 2026-10-20: US-010 applies to concept goals. Studies pause and set
Pace under US-016.

## US-011 Meet prerequisites first

Statement: When one idea depends on another, I want a useful introduction and simpler starting point before its questions, so I am not repeatedly tested on something I have not encountered.

Criteria:
1. WHEN a goal's concepts have prerequisites, THE SYSTEM SHALL order unseen introductions prerequisite-first and show the standard note before that concept's first question.
2. WHEN I choose "I know this already" on the intro, THE SYSTEM SHALL record that observation without scoring a cold review or skipping later due questions.

No-gos: no compulsory foundation detour or reading counted as a graded success.

Narrowed 2026-10-20: US-011 applies to concept goals. Studies introduce Units
in reading order (US-016).

## US-012 Practice a real confusion (retired)

Retired on 2026-09-23 by the operator's simplification decision (MIS-162): Scry
no longer generates contrast questions from recorded confusions. The id stays
reserved.

## US-013 Create from what I want to learn

Statement: When curiosity starts as a word, phrase, or long dictated ramble,
I want Create to produce useful tailored study/reference and practice material
without a setup questionnaire, so I can start learning and refine it through
actual practice and feedback.

Amended 2026-10-24: US-013 is concept preparation, reached through Prepare material with AI
from concept Home. When unavailable, the page explains that honestly and
omits submission controls; no free draft or unsupported format is promised.

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
   SHALL leave a usable Create field without creating a source when preparation is ready, or visibly retain unsent text when it is unavailable.

No-gos: mandatory import, onboarding questionnaire or diagnostic quiz before
useful material; inferred public search; assumed speech engine or recording
integration; source-only docs treated as runtime or deployment acceptance.

## US-014 Drive Scry with an owner token

Statement: When I or an agent I trust work from a terminal, I want every Scry
action available through an API and the `scry` CLI with a revocable owner
token, so I can add, practise and inspect without a browser.

Criteria:
1. WHEN I, signed in through Access, create an Owner token, THE SYSTEM SHALL show its secret once, keep only a verifier, and afterwards list it by name, creation time, last use and expiry without the secret.
2. WHEN a request presents an unexpired, unrevoked Owner token, THE SYSTEM SHALL act as the Owner for every API action, including creating, listing and revoking Owner tokens.
3. IF a request presents a missing, malformed, expired or revoked token, THE SYSTEM SHALL refuse it before reading or changing any state, with the same refusal whether or not the token ever existed.
4. WHEN an Owner token is revoked, THE SYSTEM SHALL refuse that token's next request.
5. WHEN an API mutation repeats a committed operation ID with the same payload, THE SYSTEM SHALL return the committed result without a second change; IF the payload differs, THE SYSTEM SHALL refuse it as a conflict.
6. WHEN a token-authenticated mutation commits, THE SYSTEM SHALL record which Owner token acted, visible in history and export.
7. IF a token without paid permission asks for paid work, THE SYSTEM SHALL refuse before reserving any allowance; a new token SHALL lack paid permission unless created with it.
8. WHEN a CLI command runs with `--json`, THE SYSTEM SHALL print machine-readable output and exit non-zero on any refusal or failure.

No-gos: no accounts or tokens for anyone but the owner, no signup, no anonymous
or public reading, no token kept in browser storage, no relaxed CSRF for
browser sessions, no MCP server.

Evidence: `qa/walk --stories "US-014"` (eight criterion walks),
`qa/owner-api.test.mjs` (built Worker, SQLite and compiled CLI),
`tests/owner_api.rs` (shared receipts, paid authorization and archive compatibility).
Synthetic local proof; deployed private ingress and physical phone remain unverified.

## US-015 Find a Work before creating it

Statement: When I want to learn something by heart, I want Scry to look in my
Library first and keep each Work and Version once, so I build on what is
already there instead of collecting duplicates.

Criteria:
1. WHEN I search by a title, alias or opening words, THE SYSTEM SHALL return matching Works with their Versions, and SHALL say plainly when nothing matches.
2. WHEN I add a Version under a title or alias of an existing Work, THE SYSTEM SHALL attach it to that Work instead of creating another Work.
3. WHEN I add a Version whose text, ignoring case, punctuation and spacing, equals an existing Version's, THE SYSTEM SHALL return the existing Version and create nothing.
4. WHEN I add a Version, THE SYSTEM SHALL store its Kind, ordered Sections and Units, Provenance and Rights status; IF its Units joined in order do not reproduce its text exactly, THE SYSTEM SHALL refuse it and store nothing.
5. WHEN I correct a Version's wording, THE SYSTEM SHALL create a new Version that supersedes the old one and SHALL leave the old Version and every Attempt against it unchanged.
6. WHEN I show a Work or Version, THE SYSTEM SHALL show its Versions, Units, Provenance, Rights status and Relations without changing any Study.
7. WHEN I delete a Work or Version that no Study uses, THE SYSTEM SHALL remove it; IF a Study uses it, THE SYSTEM SHALL refuse and keep it.
8. WHEN the Library is exported on its own, THE SYSTEM SHALL include no Study, Card, Attempt, request or token.

No-gos: no public or anonymous reading, no sharing yet, no meaning-based
(embedding) search, no editing a Version in place.

Evidence: `qa/walk --stories "US-015"` (eight criterion walks),
`qa/library.test.mjs` (compiled CLI → built isolated Worker → SQLite/local R2),
`tests/library.rs` (invariants, receipts, archive compatibility and Study-reference refusal).
US-015.7's used-Study case is native reference-seam proof only, as #316 permits;
real Study/Attempt preservation and refusal need #317. Supplied text is synthetic;
the owner's full Roman Missal 2011 input, hosted ingress and recovery remain unverified.

## US-016 Learn a Version unit by unit

Statement: When I choose a Version to learn, I want Scry to bring its Units to
me in reading order with cues that fade as I improve, so I can recite it word
for word in the fewest minutes a day.

Criteria:
1. WHEN I learn a Version, THE SYSTEM SHALL create one Study for it; learning the same Version again SHALL return that Study.
2. WHEN a Study introduces new Units, THE SYSTEM SHALL introduce them in reading order, at most 3, 6 or 12 per rolling day for light, steady or intense Pace.
3. WHEN I ask for the next Prompt, THE SYSTEM SHALL show the Work title, the previous Unit as context and the cue for the Card's Cue level, and nothing more of the Unit than that level allows.
4. WHEN I ask to recite a Section or a whole Version, THE SYSTEM SHALL prompt from its heading or title alone and treat the answer as a Recitation.
5. WHEN I answer with an answer-bearing cue, THE SYSTEM SHALL record the Attempt as assisted and leave the Card's schedule unchanged.
6. WHEN I answer in Free recall, THE SYSTEM SHALL schedule the Card with the pinned memory model from the Verdict at most once per Card per day; later same-day Attempts SHALL be recorded as extra practice without changing the schedule.
7. WHEN I succeed twice in a row at a Card's Cue level, THE SYSTEM SHALL fade its cue one level; WHEN a Free recall Attempt is Again, THE SYSTEM SHALL drop the Card one Cue level for relearning.
8. WHEN I ask what is due, THE SYSTEM SHALL list due Cards and upcoming due times without presenting, consuming or changing any Card.
9. WHEN I pause a Study, THE SYSTEM SHALL stop introducing its new Units while keeping its due Cards and history; WHEN I drop a Study, THE SYSTEM SHALL stop presenting its Cards and keep its Attempts.
10. WHEN I read history or export, THE SYSTEM SHALL include each Study, its Cards with schedules and Cue levels, every Attempt with its Diff, Verdict and Authority, and the Versions they reference.
11. WHILE I practise exact recall, THE SYSTEM SHALL make no provider call and record no spend.

No-gos: no separate Cards per Cue level, no prerequisite ordering, no
concept selection, no streaks or due-count chrome.

Evidence: `qa/walk --stories "US-016"`, `qa/study.test.mjs`, and `tests/study.rs` exercise the compiled CLI, built Worker, SQLite, and local R2 with authored synthetic text. Phone, private canonical wording, hosted admission, and independent hosted recovery remain unverified.

## US-017 Get an exact verdict and a word-by-word diff

Statement: When I recite a Unit or a whole text, I want an instant verdict on
every word and a diff of what I got wrong, so I know exactly what to fix and
my schedule reflects what I actually recalled.

Criteria:
1. WHEN I submit an exact-recall answer, THE SYSTEM SHALL grade it locally without a network call, ignoring case, punctuation, line breaks and quote or dash style.
2. WHEN every expected word is present, in order, with nothing extra, THE SYSTEM SHALL return Good; WHEN any word is wrong, missing, extra or out of order, THE SYSTEM SHALL return Again.
3. WHEN a typed word differs from an expected word of at least five letters by one inserted, deleted, substituted or swapped letter, and is not another word of the Version, THE SYSTEM SHALL mark it a Slip and count it correct.
4. IF the Version is Spelling-strict, THE SYSTEM SHALL count every Slip as a wrong word.
5. WHEN a Verdict is shown, THE SYSTEM SHALL show the Diff marking each word correct, slipped, wrong, missing or extra, with the expected text, and keep both until I choose Next.
6. WHEN an Again answer matches another Version of the same Work more closely than the studied one, THE SYSTEM SHALL name that Version beside the Diff.
7. WHEN a Recitation covers several Units, THE SYSTEM SHALL attribute each edit to its Unit and record a Verdict for each Unit.
8. WHEN an answer arrives as dictated text, THE SYSTEM SHALL grade it exactly as typed text.
9. EACH exact-recall Verdict SHALL name `exact-v2` and its grader version as Authority, and the same Version, answer and grader version SHALL always give the same Verdict and Diff.
10. WHEN I correct an exact-recall Verdict, THE SYSTEM SHALL record one immutable Correction as US-007 does and keep the original Attempt.

No-gos: no model judge, no tolerance beyond the Slip, no grading of punctuation
or case, no voice-specific tolerance yet, no meaning-level variants inside a
Version ("Holy Ghost" and "Holy Spirit" are different Versions).

Evidence: `qa/walk --stories "US-017"`, `qa/study.test.mjs`, and `tests/study.rs` exercise the compiled CLI, built Worker, SQLite, and local R2 with authored synthetic text. Phone, private canonical wording, hosted admission, and independent hosted recovery remain unverified.

## US-018 Practise exact recall on my phone

Statement: When I have a few minutes with my phone, I want Scry to open
straight into my current practice, and to find or add a text and begin learning
it there, so practice fits into my day without a terminal.

Criteria:
1. WHEN I open Text practice on my phone with exact-recall Cards due, THE SYSTEM SHALL show the next Prompt in the Stream with one answer field and one submit control.
2. WHEN I submit, THE SYSTEM SHALL show the Verdict and Diff the API gives for that Attempt and hold them until I choose Next.
3. WHEN I ask for a cue or to see the Unit before answering, THE SYSTEM SHALL record the Attempt as assisted before showing it.
4. WHEN no exact-recall Card is due, THE SYSTEM SHALL say when the next one is due instead of inventing practice.
5. WHEN an automatic Verdict is shown, THE SYSTEM SHALL offer the US-007 corrections without auto-advancing.
6. WHEN I open Text practice with no current exact-recall Study, THE SYSTEM SHALL offer Library entry on my phone: search by title, alias or opening words with an honest miss, inspect matching Work/Version metadata, add supplied text with optional edition/source, and Learn a chosen Version. Supplied text SHALL retain US-015 segmentation, deduplication and private rights by default; repeated Learn SHALL return the same Study (US-016.1). Current Prompts and held results SHALL remain directly reachable. Library content SHALL remain behind the deliberate assistance boundary while a protected Prompt is unanswered, and the supplied-text path SHALL call no provider.

No-gos: no web-only exact-recall action, no
microphone integration, no swipe grading.

Evidence: `qa/walk --stories "US-018"`, `qa/phone-stream.mjs`, `qa/phone-library.mjs`, `tests/phone_library.rs`, and `tests/study.rs` cover first-study Library entry and the phone Stream, shared CLI Verdict and Diff, held results, assistance, next due time, and Corrections using authored synthetic wording in light and dark. Emulation is not physical-phone acceptance. Private canonical wording, deployed ingress, and owner acceptance remain unverified.

## US-019 Create a Version with the generation agent

Statement: When something I want to learn is not in my Library, I want to
name it and have an agent with tools produce a checked Version, so I can start
learning without typing the text myself.

Criteria:
1. WHEN I ask to create something, THE SYSTEM SHALL search the Library first and start a Generation run only if nothing matches or I ask for another Version.
2. WHEN a Generation run drafts a Version, THE SYSTEM SHALL let the Generation agent choose between model memory and web search, and SHALL record in the Version's Provenance which it used, with any sources cited.
3. WHEN a draft passes its Self-checks (Units join to its text exactly, and any declared count matches, such as 26 NATO letters), THE SYSTEM SHALL add it to the Library through US-015 without critic-v1 or another model judge.
4. IF a Self-check fails, or the run fails or is cancelled, THE SYSTEM SHALL keep the run with its draft, tool calls, Self-check results and cost, leave it unpublished, and allow a deliberate new run.
5. EACH Generation run SHALL reserve against the shared allowance before sending and stop at its cost bound; an unknown sent cost SHALL keep its reservation until deliberately resolved (D5).
6. WHEN the same create operation is replayed, THE SYSTEM SHALL return the existing run without spending again.
7. EACH Spend ledger entry SHALL record its purpose and model; WHEN I ask for spend, THE SYSTEM SHALL report it by day, purpose and model, and the cost of each run.
8. IF the owner has not enabled production generation, THE SYSTEM SHALL refuse a Generation run before reserving allowance, and supplied Versions (US-015) SHALL still work.

No-gos: no rule that texts must come from sources or from two sources, no ban on
model memory, no Study or Attempt data sent to the agent, no unbounded run, no
discarded run.

Evidence: pending

## US-020 Learn an ordered list

Statement: When order is the content, such as the Mysteries of the Rosary, the
books of the New Testament or the English monarchs since 1066, I want to
practise each item from the one before it, so I can say the whole list in order.

Criteria:
1. WHEN I study an ordered list, THE SYSTEM SHALL make one Card per item and prompt each item from the previous item and its position.
2. WHEN an item has authored aliases, THE SYSTEM SHALL accept an alias as correct and show the Version's own form.
3. WHEN a Card has not yet reached Free recall, THE SYSTEM SHALL offer a choice of the next item and record that Attempt as assisted.
4. WHEN I recite the whole list, THE SYSTEM SHALL grade order by alignment and give each item its own Verdict.
5. WHEN a list is part of a larger Work (the Glorious Mysteries in the Rosary), THE SYSTEM SHALL record and show that Relation on both Works.

No-gos: no reordering or sampling of a complete set, no relations editor.

Evidence: pending

## US-021 Learn a keyed mapping

Statement: When the content is pairs, such as the NATO alphabet, the Rosary day
schedule or the first seven ecumenical councils with their years, I want to
practise each pair in the directions worth practising, so I can answer from
either side.

Criteria:
1. WHEN I study a keyed mapping, THE SYSTEM SHALL make one Card per pair and declared direction.
2. WHEN the value is a year, THE SYSTEM SHALL accept it with or without AD or CE and grade it as an exact number.
3. WHEN a Spelling-strict mapping is answered (NATO "Alfa", "Juliett"), THE SYSTEM SHALL count any misspelling as wrong and show the official form.
4. WHEN a Card has not yet reached Free recall, THE SYSTEM SHALL offer a choice of values and record that Attempt as assisted.
5. WHEN I recite the whole set from a random start, THE SYSTEM SHALL give each pair its own Verdict.
6. WHEN a mapping relates to another Work (the Rosary day schedule to the Mysteries of the Rosary), THE SYSTEM SHALL record and show that Relation on both Works.

No-gos: no tolerance on years unless the Version declares a range, no
relations editor.

Evidence: pending
