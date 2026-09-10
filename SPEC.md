# Scry Rewrite Specification

Status: implemented private Go/SQLite/HTMX application; initial phone flow and
daily recovery policy approved by the operator on 2026-09-09. The active app is
at `https://scry.study`, with append-only R2 recovery through `scry-go-backups`.
The committed source-bound Go release and canonical-domain cutover are active.
Alternate-host reads redirect; mutations are rejected rather than replayed.
The old production/staging Workers remain paused with no cron triggers and
final verified recovery copies. Historical data is preserved, not imported or
deleted. [Criterion-level evidence](docs/qa/personal-go-cutover-20260910.json)
records the cutover, corrected browser paths, and open S04.2/S09.2 evidence.

[Product direction](VISION.md) is upstream. Historical extraction strategy
remains in Git and [the Rust migration record](docs/rust-migration.md).

## Authority and open decisions

**Confirmed by the operator:** the rewrite began with no active users and an
experience the operator wanted to replace. Go, SQLite, HTMX, and exe.dev are the
accepted direction: smooth, simple, aesthetically intentional, and enhanced by
AI content generation. Full execution was authorized. The initial real-phone
flow was subsequently approved, followed by the daily recovery policy below.

**Implemented and accepted defaults:** tap-choice and short cued recall; reveal
marks the occurrence assisted; one question stage with held feedback and
deliberate Next; daily and pre-release off-VM snapshots with 30-day new-app
retention; RPO 24 hours / RTO 60 minutes as targets, not guarantees; $1/day
generation ceiling with $0.20 conservative reservation; fresh target data.

Private generation now uses a Scry-only OpenRouter key, retained in ignored
`.env` and supplied to the application through the private exe integration.
The provider adds a $1/day UTC limit alongside the application's rolling
24-hour allowance. Session-access checks are bounded and can be retried in
place after reconnecting without discarding unsaved input. The
[private acceptance receipt](docs/qa/personal-go-acceptance-20260909.json)
records native touch, live generation, and independent-VM data restoration at
that earlier observation. Subsequent phone approval and full restored-service
activation are separate evidence: the recovered private HTTPS export matched
exactly and the restored Review UI rendered. Approximately 117 seconds elapsed
through private HTTPS on an existing recovery VM; provisioning and DNS recovery
were not timed, and the synthetic rehearsal received no production integrations.

**Still unverified:** operator review of generated-material usefulness (S04.2)
and switching to another exe account with private-history return (S09.2).
Global sign-out hid private content, but history return hit an upstream
authentication redirect loop; fresh navigation reached sign-in. MIS-48 remains
open. Proposed p95/accessibility budgets and held-out AI acceptance are recorded
separately in the receipt, not inferred from phone-flow approval. Future material
quality and longitudinal learning outcomes are not established by the examples.
Old data remains preserved separately; import or deletion needs a new decision.

Resolve these decisions as the relevant slice becomes ready. Record the decision
and reason here; remove obsolete alternatives rather than retaining two designs.

| Decision | Adopted choice | Basis / remaining evidence |
| --- | --- | --- |
| D1: personal material and failures | Build around what the operator wants to remember, not old QA datasets | The accepted candidate used photosynthesis, HTTP caching, and DNS examples. Specific future learning goals and sustained usefulness come from real use, not invented frustrations. |
| D2: initial response styles | Tap-choice and short cued recall; no model grading in the fast path | Initial phone flow approved. Changes to styles or typing burden need renewed review. |
| D3: assistance and correction | Answer-bearing help marks the occurrence assisted; disputes are explicit, not automatic successes | Durable reveal, edit/archive, and dispute behavior exercised; historical events remain unchanged. |
| D4: experience approval | One question stage, persistent feedback, deliberate Next | Operator approved the real-phone flow. Swipe is neither required nor an implicit grade. |
| D5: recovery / spend | Daily and pre-release off-VM backups; 30-day new-app retention; RPO 24h / RTO 60m targets; bounded generation spend | Operator selected daily backups. Separate-VM data restore, full unprivileged service activation, private HTTPS/export equality, and restored UI passed. Provisioning/DNS outage recovery and an availability SLA are not claimed. |
| D6: replacement boundary | Fresh target data, no legacy API parity; preserve historical stores and backups separately | Operator explicitly directed preservation. Both old Workers are paused; native Postgres remains disabled with recovery backups active. No old data import or deletion. |

Unresolved decisions block only work that depends on them. They do not require
planning every future feature before testing the core experience.

## The specification spine

| Layer | Durable owner | Question answered |
| --- | --- | --- |
| Product | VISION.md | Who is this for, what outcome matters, what is out of scope? |
| Behavior | Stories and quality contracts below | What must the learner be able to do, including failure and interruption? |
| Design | Architecture and decision sections below | What is the smallest system that can deliver those behaviors? |
| Work | Accepted Linear issues linked to story/criterion IDs and a source revision | What bounded change is someone doing now? |
| Evidence | Revision-bound QA receipt linked from the issue/PR | What was actually exercised, observed, and not verified? |

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

Implemented surface:

```text
Review                         Add / Library

One question or recall cue
Only the context needed to answer

Answer choices or a short response
I don't know yet

After answering, in the same stage:
Result + concise explanation
Next                         Fix / Inspect
```

- Maintain spatial continuity between question and feedback. Feedback remains
  until Next; do not auto-dismiss it or require chasing a moving control.
- Keep controls reachable one-handed. Support buttons and keyboard before an
  optional gesture; swiping while reading must not submit or erase an answer.
- Keep generation, account machinery, and library maintenance out of review.
- End honestly when nothing useful is ready. Extra practice or new material is
  deliberate, not an infinite feed engineered to extend a session.
- Prefer a few excellent prompt forms to a universal activity framework. A
  finite list, exact wording, and a causal explanation need different quizzes.
- No large dashboard cards, decorative metrics, confetti, or fake progress as
  substitutes for clarity. Motion explains a user action and yields to reduced
  motion settings. Question length may require scrolling; small screens must
  not clip content merely to imitate a fixed-height feed.

The initial phone experience is approved: a white reading surface, deep ink,
cobalt actions, restrained feedback, local system typography, and a prominent
question stage rather than a branded dashboard. `internal/web/assets/app.css`
owns exact visual tokens. Approval is not a permanent freeze; review subsequent
changes against the brief with real questions and actual phone interaction.

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
- **S01.3:** Returning from Add/Library preserves an unfinished occurrence or
  held result. Navigation and status reads do not consume it.

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

Proof: actual browser interactions plus deterministic learning/SQLite boundary
checks for ambiguous answers and reveal/submit races.

### S03 — Add something without configuring it

As the learner, I want to add a word, goal, phrase, or pasted material in one
place so that capture is faster than making my own flashcards.

- **S03.1:** One field accepts the supported inputs without mandatory title,
  deck, taxonomy, or prompt-type decisions. The saved input remains inspectable.
- **S03.2:** Capture returns a durable saved/job state. Leaving the page or
  restarting the app does not lose an acknowledged capture. Retrying an
  ambiguous submission does not create duplicate generation work.
- **S03.3:** Oversize/unsupported input gets a specific remedy without silent
  truncation. Generation failure preserves the input and offers retry/edit;
  zero usable questions is not reported as ready.

Proof: browser capture/navigation, interrupted response, process restart, and
honest invalid/empty/provider-failure outcomes.

### S04 — Receive useful, trustworthy generated material

As the learner, I want AI to produce material worth reviewing so that I do not
become a full-time editor of generated cards.

- **S04.1:** Topic expansion is identified as generated knowledge, not proof
  supplied by the topic word. Source-based content retains inspectable source
  evidence and does not invent quotations or strengthen qualified claims.
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

Proof: network interruption/reordering, two browser tabs, and real restart with
SQLite state inspected through consumer-visible history/results.

### S07 — Find and manage what I am learning

As the learner, I want a small searchable library so that I can find, inspect,
edit, and stop reviewing material without managing a database.

- **S07.1:** Search finds source/quiz text and opens the relevant material,
  provenance and future review state without exposing implementation vocabulary.
- **S07.2:** Archive stops future selection after refresh/restart and invalidates
  pending publication for that source. It preserves historical evidence and is
  labeled archive, not permanent erasure.
- **S07.3:** Personal export includes source text, current content, review history,
  and schedule/version information in a documented portable form. Export is
  read-only and does not alter the review session. Full erasure/backup retention
  semantics are a separate explicit decision, not an archive side effect.

Proof: browser search/edit/archive and an inspected export across a restart.

### S08 — Return to worthwhile practice

As the learner, I want Scry to choose useful future reviews and show honest
progress so that returning improves recall rather than just my activity count.

- **S08.1:** The same documented algorithm version, state, rating and time give
  the same next schedule. A miss/help leads to the declared relearning policy;
  a refresh, snooze, flag, or animation never creates a learning event.
- **S08.2:** Selection respects due/availability state, avoids repeatedly serving
  removed/superseded material, and uses meaningful variation rather than random
  mixing for its own sake. An empty queue has an exit, not a progression dead end.
- **S08.3:** History distinguishes attempts, assistance, recognition, and disputed
  outcomes. Due counts, streaks, predicted retention, and same-session success
  are not labeled proof of knowledge or improved long-term retention.

Proof: versioned reference scheduling trajectories, next-day/resume scenarios,
and operator use. Efficacy claims require separate delayed unaided recall data.

### S09 — Keep the application private

As the owner, I want easy private access from my phone and computer without
maintaining a signup product or exposing my learning material.

- **S09.1:** Approved exe identity can access the app; missing, unapproved, or
  forged identity cannot read/export material or mutate it through any exposed
  hostname, backend listener, or alternate port. Verify the actual ingress trust
  boundary before accepting personal data.
- **S09.2:** Cross-site mutation and user/model script or HTMX-attribute injection
  are rejected or rendered inert. Back/history/cache after sign-out or identity
  change does not expose a previous private session. Prove exe logout/account
  switching behavior rather than assume header presence solves session lifecycle.
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

## Implemented architecture

### One application and one state authority

```text
Phone browser
  HTML + pinned HTMX + small review interaction controller
       |
exe private HTTPS / owner identity
       |
One Go binary, supervised by systemd
  HTTP/rendering + learning policy + model-job loop
       |                         |
Local SQLite (WAL)          Model provider over HTTPS
       |
Consistent snapshot -> append/read gateway -> private Cloudflare R2
```

One Go module uses ordinary internal packages: net/http, html/template,
embedded templates/assets/migrations, SQLite, a small deterministic learning
package, and one model HTTP boundary. There is no service framework.
Use one implementation of each workflow. Interfaces should isolate real external
boundaries, not mirror every table with a repository/service/controller stack.
No React runtime, client build step, Redis, Postgres, event bus, vector database,
application Worker/Durable Object, or orchestration platform is required.

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
`internal/learning.Algorithm` identifies policy. No old-engine parity or
personalized-retention claim is inherited.

### Generation and learning policy

One bounded worker loop claims SQL jobs and persists attempt/lease ownership
before making provider calls outside a transaction. Its timeout is shorter than
the lease or the lease is renewed. Completion rechecks the claim and source
revision/lifecycle, then atomically publishes validated content and final state.
Expired claims are recoverable after restart; stale workers cannot publish.
Use explicit queue/input/output/retry limits and account for failed or uncertain
paid calls within the approved spending ceiling.

Local publication can be idempotent; remote model billing is not magically
exactly once. If the process dies after provider acceptance, retry may spend
again. Use provider idempotency/result lookup only when actually supported;
otherwise preserve uncertainty and use a conservative bounded/manual-retry policy.

Choose one model/provider by results on D1 material. Preserve input, model/prompt
version, generated content and corrections for diagnosis; do not build a generic
AI platform. Topic expansion and source-grounded generation have different
provenance claims. Initial grading stays deterministic for supported formats;
semantic model grading is a distinct, slower product decision, not a hidden
network call in the fast review path.

Start with reusable explanations and an optional simpler question after a miss
only when genuinely helpful. A universal prerequisite graph, recursive tutor,
fixed activity ladder, automatic curriculum, and personalization optimizer do
not follow from needing spaced review.

Use durable job status with bounded HTMX polling. SSE is optional only after
proving useful incremental delivery through exe's actual proxy; streaming and
proxy timeout behavior were not established by this specification research.

### exe.dev and the small Cloudflare role

Build and preview on an isolated exe development workspace with synthetic data.
Deploy the reviewed binary to the application environment without giving coding
agents live data or production authority by default. Do not clone an active
scheduler, credentials, or attached integrations into a second writing owner.
A single production instance is sufficient; no HA or automatic failover claim.

Exe's private HTTPS and exact stable owner UserID form the identity boundary.
Keep the backend private/loopback as supported; before use, prove that spoofed
identity headers cannot enter through alternate routes. Header presence alone
is not authentication. Application ownership checks and CSRF protection remain.
Do not add Cloudflare Access in front of exe auth without a reason to maintain
two access systems. The runbook owns approved domains and observed ingress.

systemd owns restart/startup. On shutdown stop job claims, drain HTTP with a
bound, join/cancel the job worker, then close the database. Mutable data and
secrets live outside immutable release directories. Migrations finish before
readiness; a prior binary is a rollback option only if compatible with the
current schema. Do not run the build directly over the active release.

Cloudflare R2 owns private off-VM archives, not a live SQLite filesystem or a
second primary. The append/read-only recovery gateway provides the application
no delete or bucket-policy authority. Recovery uses completed VACUUM INTO,
checks integrity/checksum and schema/binary metadata, uploads under a unique key,
and marks off-VM completion only after exact remote readback. Never copy only the
live .db while WAL writes continue. Include separately stored assets in recovery.

Recovery credentials/material must remain available without the lost VM. Restore
into a fresh isolated environment and reconcile uncertain jobs before enabling
external effects. VM persistence, VM copying, R2 availability and a recent
backup timestamp each fall short of a tested restore. Backup failures should
be visible without disabling ordinary review. D5 owns data-loss tolerance,
retention and recovery time; no provider SLA is asserted here.

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

The issue owns execution state. PRs own change explanation and evidence links.
Neither becomes a second spec. Fixing a technical dependency can close its ticket
without closing the parent story; its missing criteria remain explicit.

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
- [exe private HTTPS](https://exe.dev/docs/proxy.md),
  [identity](https://exe.dev/docs/login-with-exe.md),
  [machine tokens](https://exe.dev/docs/https-tokens-for-vms.md), and
  [migration/listener guidance](https://exe.dev/docs/migrating-to-exe.md).
- [Pinned Go FSRS release](https://github.com/open-spaced-repetition/go-fsrs/releases/tag/v4.0.0)
  and [selected pure-Go SQLite driver](https://gitlab.com/cznic/sqlite).
- [R2 consistency](https://developers.cloudflare.com/r2/reference/consistency/)
  and [S3 compatibility](https://developers.cloudflare.com/r2/api/s3/api/):
  unique completed snapshots, not assumptions about object lock or replication.
