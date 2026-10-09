# Scry verification

The active replacement source is Rust/WASM on a plain Cloudflare Worker with a
singleton SQLite Durable Object and private R2. The production Go/SQLite
Container remains the deployed runtime until an approved replacement;
[the operational runbook](../runbook.md) owns that state. [VISION](../../VISION.md)
owns intent, [SPEC](../../SPEC.md) owns acceptance, and
[README](../../README.md#start-or-resume-work) identifies work ownership.

A native test, a workerd fixture, a browser interaction, live-provider material,
private deployed ingress, physical-phone acceptance, and independent recovery
establish different claims. The ground-up rewrite needs its own evidence; old
Go/retired-Worker receipts are historical observations, not transferred passes.

## Choose proof before running checks

Read the affected story/criterion and design decision, then choose the smallest
proof that exercises the changed boundary. Reuse valid receipts when their
source, environment, and behavior still apply. Documentation-only work needs
meaning, link, authority, and traceability review; a source edit alone does not
justify a model call or deployment.

Preserve all accepted behavior: prompt-first useful material, honest grading,
assistance, immutable history, bounded unknown usage, and recoverability. QA
cannot change a criterion to match the implementation. A build or fixture does
not certify usefulness or authorize activation.

## Repository gate and artifact

```sh
cargo test --locked
cargo clippy --all-targets -- -D warnings
cargo check --locked --target wasm32-unknown-unknown
worker-build --release --locked
bun run ci
```

`ci` / `ci:local` run [scripts/scry-ci](../../scripts/scry-ci) over a frozen
non-ignored source snapshot. Native checks use the lockfile and preserve the
pinned scheduler identities; WASM compilation and a real local Worker smoke
exercise the replacement target. JavaScript syntax, historical recovery tools,
secret scanning, source inventory, and exact-artifact checks remain separate
parts of the gate. No production/model/deployment credentials belong in it.

For release evidence, require committed source with `--require-committed` and
an unused output destination. Inspect the source inventory, archive, checksums,
proof, and smoke result. Worktree-labeled output is development evidence. The
WASM and Worker loader/assets exercised must be the bytes staged later; never
rebuild between proof and activation. A passing source gate is not a deployment
receipt or a guarantee that remote recovery is usable.

## Focused checks

| Changed boundary | Existing Rust checks | Additional observation |
| --- | --- | --- |
| Pure grading / scheduler | `tests/learning_policy.rs`, `tests/fsrs_golden.rs` | Full 260-trajectory comparison protects pinned Go FSRS state, not learning efficacy |
| Concept evidence / pace / selection | `tests/concept_policy.rs`, `tests/application.rs` | Prerequisite intros, focused new concepts, paused new material, due work, 3/6/12 rolling-day caps, and separate exposure/help/unaided observations |
| Durable review / retry | `tests/application.rs` | Real workerd SQLite, refresh/restart, concurrent tabs, response loss after commit, identical-operation replay, and one event/schedule transition |
| Meaning checks / override | `tests/learning_policy.rs`, `tests/application.rs` | Exact/variant local resolution; independent short-v1 and rubric judgments; unsure/failed self-check; immutable authority and corrections; bounded real holdout quality separately |
| Generation / refinement | `tests/generation_policy.rs`, `tests/application.rs` | Actual word/phrase/ramble → useful reference + practice, exact evidence, optional photo transcription, genuine attempt/feedback refinement, slow/partial/failed/unknown outcomes |
| Prepublication critic US-004 | `tests/learning_policy.rs`, `tests/generation_policy.rs`, `tests/application.rs` | Saved candidates before external checks, independent hard judgments, one send lease, shared allowance, unknown usage, critic-only new attempt, judgment reuse, and fenced publication |
| HTML / assistance fence | `tests/web_render.rs` | Real pointer/keyboard, native forms without JS, hidden current answer/reference across navigation, held result, content draft/version fences, access loss |
| Persistence / archives | `src/persistence.rs` unit tests, `tests/application.rs` | Real SQLite/R2 archive with every referenced photo, full-byte remote readback, independent fresh-space restore, paused uncertain work |
| Private Worker ingress | WASM check and runtime review | Actual owner JWT signature/issuer/audience/sub, anonymous/forged denial, Host/origin/CSRF, alternate-host write rejection, logout/account return |
| Historical recovery | `bun run test:recovery` | Corresponding historical format/artifact only; do not reinterpret old snapshots as rewrite archives |

Write regression tests for actual boundaries, precedence, races, and recovery
transitions. Mock external effects when needed for controlled failures; exercise
repository-owned learning and engine policy directly. Do not use tests of
source wording as runtime acceptance.

## Browser and private ingress

### Local authored fixture

```sh
bun run dev
```

[scripts/rust-dev](../../scripts/rust-dev) builds the WASM Worker, creates its
own private temporary configuration/state, starts loopback workerd with SQLite
Durable Object and local R2, and removes inherited credentials with `env -i`.
It does not source `.env`, `.dev.vars`, or historical operator configuration.
The printed run directory identifies ownership; use an unused port via
`SCRY_DEV_PORT` without stopping an unrelated service.

Begin with the empty notebook. The isolated preview alone offers “Explore an
authored demo,” a deliberate fixture action that refuses existing learning
state. The fixture has HTTP cache freshness/validation explanations and three
questions. It is authored data, not a provider call, real cold recall, or an AI
quality result. Requests made without model configuration remain saved with
honest zero-cost configuration failure. No production store is imported.

For browser work in T3 Code, first inspect the collaborative preview status and
open it if needed. Use its native pointer/keyboard tools and snapshots. If those
tools are unavailable, run browser QA on the isolated exe.dev VM as the
repository browser/story runner instructs. Do not silently replace a stalled
harness with DOM `.click()` or mark unobserved cases passed. The Worker story
walk must produce a fresh revision/environment/story-bound receipt; historical
Go walks and their selectors are not rewrite proof.

Stop the run-owned process before cleanup. Preserve evidence and remove only
its verified temporary directory. To reset, start a new run/namespace; never
erase existing SQLite state, R2 buckets, or historical stores to make a fixture
pass. The local R2 emulator proves archive mechanics, not remote off-machine
recovery.

### Core journeys

1. Create: a word, a phrase, and a long dictated-text request are valid without
   setup or a mode chooser. Capture saves one goal/operation; pasted URLs remain
   private intent. Optional supported photos transcribe before preparation.
   Invalid/oversize input retains the draft and creates no goal/job/photo
   reference. Check ASCII and multibyte byte bounds, not only character limits.
2. Preparation: leave and refresh a saved request; distinguish pending, partial,
   failed, missing provider, exhausted allowance, and unknown sent outcome. A
   poll is read-only. It neither sends paid work again nor replaces an editable
   draft or current unanswered question. Existing study stays usable.
3. Study: choice tap or typed recall → saved checking when needed → self-check
   on uncertainty or a held graded result → deliberate Next. Refresh and Back
   do not manufacture an answer. Reading intro or choosing “I know this already”
   records exposure, not an unaided event.
4. Assistance: before unaided grading, inspect root, Map, goals, concepts,
   History, edit, export/photo, and alternate navigation. No current expected
   answer, explanation, note, goal intent/title, or leaking concept name appears
   before an assistance POST. Help stays attached through refresh/another tab.
5. Corrections: automatic miss → I was right; automatic correct → Count as a
   miss. Replay the same operation and inspect one immutable correction,
   unchanged original event, and one consistent current schedule. A stale
   correction conflicts. Repair drafts require deliberate versioned Save;
   disputes and deliberate schedule reset remain distinct from grade changes.
6. Map/reference: prerequisite-first introductions, focus on new material,
   pause/resume, pace caps, accessible states/list, saved notes, labeled recall
   estimate, and separate unaided/helped/missed tally. Navigation never changes
   the durable review occurrence or invents evidence.
7. Resilience: lose a response after commit, reconcile the same frozen operation,
   and inspect exactly one event/result. Keep an in-page draft offline; reconnect
   does not auto-send. Test competing tabs and a late response. Access denial
   removes private DOM; pagehide/back-forward return revalidates access.
8. Appearance/access: 320/390/1280 px, both color schemes, text zoom, reduced
   motion, keyboard focus, no-JavaScript forms, real pointer events, and long
   reference/error text. A screenshot proves appearance, not phone acceptance.

### Real-provider and private production proof

Use integration capabilities only within the session's authorized scope. A
bounded provider run names the model, material, raw result/usage, human review,
keep/revise/reject reasons, and remaining uncertainty. S11 acceptance needs
useful real word/phrase/dictated inputs and later refinement from real attempts
and feedback; valid JSON or an LLM critic alone cannot establish usefulness.
US-004 controls report false accepts/rejects/abstentions separately from broad
publication quality. Unknown paid usage remains reserved and inspectable.

Use only the approved Cloudflare Access owner login for deployed private
browser proof. Never substitute VM/root authority or copied browser tokens for
that boundary. Verify anonymous and forged denial, exact owner access,
cross-site/stale writes, read-only alternate-host redirects, and private content
on logout/account/history return. Keep credentials out of URLs, argv, logs,
screenshots, and committed receipts. Access logout, app mutation state, provider
keys, and restore/operator capabilities are separate authorities.

`/healthz` and `/readyz` establish liveness/readiness, not model quality,
remote-backup freshness, or private-browser acceptance. Settings exposes the
last verified backup and errors. No public signup/API/status compatibility is
inherited from the retired applications.

## Independent recovery

The rewrite archive is complete versioned JSON plus all referenced photo bytes
and their checksums, generated from one committed application snapshot. Its R2
upload is successful only after exact remote size/checksum/full-byte readback
and archive validation. Daily/pre-release backups and 30-day retention target
RPO 24 hours and RTO 60 minutes; report measurements and exclusions plainly.

Retrieve a completed archive independently with compatible Worker assets and
configuration retained outside live storage. Restore into an unused isolated
Durable Object namespace with a separately scoped restore capability and no
model/operator integrations. Verify the schema, metadata, photo contents, and
export equality to the acknowledged snapshot. Unknown jobs and assessments stay
paused; no automatic transmission follows restore.

When claiming service recovery, also measure private activation, owner/anonymous
access, the actual UI, and readback after fresh writes. Provisioning, DNS,
configuration recovery, and elapsed time not exercised remain explicit gaps.
Local R2, a checksum, copied files, or a reused live namespace alone is not
independent hosted recovery evidence. Never overwrite the live application or
reactivate historical writers during rehearsal.

## Evidence and historical material

Reports bind source/artifact and specification revision, environment/origin,
synthetic/real provenance, browser/device, authorized capabilities, exact
story/criterion, actions, expected observation, actual result, and limitations.
Use PASS, FAIL, or UNVERIFIED. Owner experience/physical-phone acceptance is a
separate observation. Do not claim efficacy, availability, mail delivery, or
unmeasured recovery from synthetic mechanics.

The September Go concept-study, private-acceptance, cutover, foundation,
advisory, and simplification receipts remain intact in this directory. Their
exact compatible source and artifacts are historical evidence. The old Go
source at `c6ba395`, frozen Rust/Cloudflare/Postgres snapshots, historical
scripts, and retained backups keep their original formats. Nothing in the
rewrite imports them or transfers their acceptance to the new namespace.

[Critic instructions](critics.md) describe advisory/review workflows. Adapted
Worker passes need their own actual observation and source-bound receipt;
mutating passes remain isolated and synthetic. A named story closes only when
all its required surfaces have adequate evidence; UNVERIFIED is not PASS.
