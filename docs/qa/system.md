# Scry verification

The current application is Go/SQLite/HTMX on private exe.dev ingress. Choose
proof for the changed behavior; a build, a fixture, a browser observation, a
live provider result, and physical-phone acceptance establish different things.
[VISION](../../VISION.md) owns product intent, [SPEC](../../SPEC.md) owns
acceptance, and [the runbook](../runbook.md) owns deployment/recovery procedures.
Linear owns current work ownership, status, and pause; a procedure or old receipt
does not grant permission to execute it.

## Choose proof before running checks

Start with the authorized changed surface and the claim to establish, not the
largest available gate:

1. Read the owning criterion/design direction and existing
   [dated evidence](#evidence-and-historical-material). Reuse evidence whose
   source/artifact, environment, and exercised behavior still cover the claim;
   name any changed boundary that invalidates it rather than repeating unrelated
   paid, browser, or recovery work.
2. For prose-only design changes, review meaning, source links, authority, and
   the proposed journey/stop boundary. Keep operator direction distinct from
   unaccepted proposals and current commands. Native document readback is proof
   of those edits; tests, models, VMs, and deployment are not needed by default.
3. For authorized runtime changes, choose the [focused proof](#focused-checks)
   below; use the exact-binary gate for a release claim. Neither a green gate
   nor another live request resolves the operator's
   [September 12 foundations rejection](../design/concept-centered-study.md#operator-findings-and-direction).
   Reuse functional evidence without calling the experience useful or approved.

## Repository gate and release artifact

```sh
bun run ci
bun run ci:full -- --out target/ci-release --require-committed
```

`ci` and `ci:local` run `scripts/scry-ci local` with host tools. `ci:full` uses
pinned Dagger tooling and the same shared gate over a frozen source snapshot.
The host requires Go 1.27.1 Linux/amd64, Node 22+, and Gitleaks 8.30.1. Use the
full gate when those tools are not available locally. Both gates check:

- Go formatting, tests, and vet with dependency updates forbidden.
- Syntax of browser/gateway JavaScript and current deployment shell scripts.
- Retained historical backup/recovery contracts, separately from the Go app.
- Redacted Gitleaks over admitted source, excluding ignored local secrets/state.
- The exact Linux amd64, CGO-disabled binary: real HTML and embedded assets,
  grading, exact retry, persisted feedback after restart, consistent backup,
  isolated restore, and full restored learning-state equality.

The artifact contains the tested binary, checksums, revision, source inventory
and archive, proof, build information, and smoke logs/exports. No second build
follows smoke. Synthetic smoke has loopback development identity and no remote
model or backup authority. It does not prove remote backup or live AI quality.

Committed source receives its Git revision. Dirty/untracked source is labeled
`worktree-...`; hosted `--require-committed` refuses it. Destinations are
exclusive-create. A successful process launch is not release evidence: inspect
`proof.json`, the checksums, and `scry version`, then stage only those bytes.

## Focused checks

| Changed area | Useful existing checks | Additional real proof |
| --- | --- | --- |
| Design/prose only | Meaning, relative-link and authority review; native readback | Operator design agreement when sought; no runtime claim or default test/model/VM exercise |
| Learning/SQLite review | `go test ./internal/learning ./internal/store` | Exact retry/restart and durable event/schedule agreement |
| Semantic grading | `go test ./internal/learning ./internal/store ./internal/semantic ./internal/web` | Synthetic authored semantic quiz: pending → correct, incomplete cue fenced as assistance, failed check with saved answer/retry/reveal, exact-operation replay, and stale finalization; live Jev quality is separate bounded evidence |
| Generation | `go test ./internal/generation` | One bounded, authorized live request on representative material; inspect provenance, coverage, rejections, and actual spend |
| HTTP/browser | `go test ./internal/web` | Actual browser interaction against the changed surface, not DOM-click substitution |
| Recovery | `go test ./internal/recovery` | Completed remote checksum readback and independent restored-service proof |
| Deployment | Current shell syntax plus exact-binary gate | Actual unprivileged process/readiness after protected activation |
| Historical recovery | `bun run test:recovery` | Use the corresponding historical store/tooling; never reinterpret old formats as Go snapshots |

Do not add permanent tests for wiring or source text. Keep regressions for
observable boundaries, races, precedence, and failure transitions. Mock only
external boundaries; repository-owned storage and learning collaborators stay
real.

## Critics

Scheduled and exploratory critic passes start at `scripts/critics/` with run
instructions in [critics.md](critics.md). A pass needs a code postcondition plus
a real observation; Jev output is advisory only. Mutating passes run only
against isolated synthetic data. Receipts bind the source revision, environment,
story/criterion ids, and observed result.

## Browser and private ingress

### Local authored fixture

From the repository root, use a POSIX shell, `mktemp`, `env`, and the Go toolchain
selected by `go.mod` (Go 1.27.1). The first build may download that toolchain and
Go modules; this is not a provider call. No Bun, frontend build, model key, exe
login, or production environment file is needed for this local recipe.

Build once into a new private temporary directory, seed its unused database,
then run those bytes in the foreground. Run the whole block in one shell; the
`&&` chain stops if allocation, build, or seeding fails.

```sh
qa_dir=$(mktemp -d /tmp/scry-qa.XXXXXXXX) &&
printf 'Disposable QA directory: %s\n' "$qa_dir" &&
go build -mod=readonly -o "$qa_dir/scry" ./cmd/scry &&
env -i PATH=/usr/bin:/bin HOME="$qa_dir" LANG=C.UTF-8 \
  "$qa_dir/scry" seed-fixture --db "$qa_dir/synthetic.sqlite" &&
env -i PATH=/usr/bin:/bin HOME="$qa_dir" LANG=C.UTF-8 \
  SCRY_BACKUP_DIR="$qa_dir/backups" \
  "$qa_dir/scry" serve --dev --db "$qa_dir/synthetic.sqlite" --addr 127.0.0.1:8080
```

`seed-fixture` publishes an authored DNS/TLS bundle without calling a model or
fetching its reference link. Its JSON reports `synthetic: true`, model
`authored-test-fixture`, and `provider_cost_micros: 0`; it includes authored
content, so reading it is not a cold-recall observation. One TLS recall uses a
two-idea semantic rubric, one missing-idea cue, and one contradiction/feedback
pair for local UI smoke. With the isolated recipe's empty semantic endpoint, its
non-exact answer safely shows the failed/ungraded recovery state without a live
request. The command refuses an existing database or any SQLite sidecar (`-wal`,
`-shm`, `-journal`), never replaces them, and does not import production data.
the exact-binary smoke in `scripts/lib/scry_smoke.py`, reached through the
repository gate above.

`--dev` supplies loopback development identity, **not** capability isolation:
`serve` still reads inherited `SCRY_*` configuration and starts generation and
recovery workers. The `env -i` allowlist above removes that inheritance,
including model endpoints/keys and remote-backup URLs/tokens. Do not source a
production environment file or add live integrations. With no model endpoint,
new generation jobs become saved, zero-cost configuration failures rather than
producing questions. Recovery still creates local snapshots in the disposable
`backups` directory at startup and on its interval; none are off-VM proof.

Wait for `Scry ready`, then open **http://127.0.0.1:8080/** in a local browser.
The fixture supplies real persisted materials and assessments for the embedded
review UI. To select the same recall path used by smoke, open the fixture
goal's `/goals/<goal.id>/plan` path using `goal.id` from the seed JSON. Set
**Time for this goal plan** to `3600`, **New assessments per day** to `100`,
**Learning focus** to **Deliberately choose more practice**, enter a synthetic
QA reason, and save. Return to learning to exercise answer → held feedback →
Next. These are disposable QA pacing values, not recommended learning settings.
If port 8080 is occupied, choose another unused loopback port in `--addr` and
the browser URL; do not stop an unrelated service.

Stop with **Ctrl-C** in the serving terminal and wait for the process to exit
before manipulating files. To resume saved state, rerun only the final
`env -i ... serve` command in that shell, then refresh the browser (the
development session secret is regenerated). To reset, rerun the whole block
for a fresh directory, not `seed-fixture` against the old database. Keep the
printed directory for evidence or remove only that verified disposable
directory after stopping; never delete `data/` or an existing SQLite database
to make seeding succeed.

This exercises local authored-data UI and persistence, not live generation,
provider quality/spend controls, private HTTPS ingress, owner authorization,
physical-phone touch, or independent recovery. Use the separately authorized
proof below for those claims.

### Browser interaction and private ingress

Use an actual browser. Check answer → held feedback → deliberate Next, capture
and saved generation status, library/correction, and interrupted access. Exercise
real pointer/keyboard events and inspect the screen. A DOM `.click()` bypass is
not evidence of working touch. If a browser harness stalls, diagnose or replace
that isolated browser rather than count a bypass as product proof.

Keep pending, committed, rejected, and unknown outcomes distinguishable. Lose a
response after commit and retry the identical operation: one event/schedule
transition, same feedback. Background/reconnect must preserve an unsaved draft;
privacy/access failure must hide content until access is revalidated.

For capture bounds, use native typing/paste for both ASCII and multibyte input,
not DOM value assignment that bypasses browser limits. Verify exact draft bytes
before submission and after a 422, no source/job on rejection, and successful
exact storage at 32,768 bytes. Opening an ungraded dispute must neither reveal
answers nor require assistance; reset-off must preserve its event and schedule.
After archiving, held feedback describes the recorded schedule and the empty
screen explains that archived questions are unavailable.

On the deployed private origin, use an approved exe login or separately scoped
VM token; do not put tokens in URLs, argv, captures, or public receipts. Verify
anonymous/forged access is denied, exact owner access succeeds, stale/cross-site
mutations are rejected, and alternate hosts cannot replay writes. Account or
provider-access loss must not expose private history/cache on return. VM API
authority is independent of browser logout; never claim logout revokes a token.

`/healthz` and `/readyz` return plain text `ok` and `ready`. They prove liveness
and database readiness, not fresh off-VM recovery or question quality. Inspect
Settings for the last completed backup and visible stale/error state. There is
no current `/statusz` contract or public service-session API.

## Foundation detour (MIS-59)

Use a fresh isolated schema-2 app and clearly labeled synthetic authored content
for local mechanics. The changed route family is `POST /review/foundation`,
`GET /foundations`, and `GET/POST /foundations/{id}`. The Library links to saved
foundations; requests do not invoke Reveal. Since MIS-157 the review interface
does not surface a Too advanced control, so walk the detour by posting to
`POST /review/foundation` directly and viewing saved foundations from the
Library. No test posts `POST /review/foundation`; the commands below cover the
store and generation methods, and the web Foundation tests cover the
`/foundations/{id}` and `/foundations` routes, so the direct request and its
queued job stay part of this manual walk. Relevant focused commands:

```sh
go test ./internal/store -run 'Foundation|PopulatedV1' -count=1
go test ./internal/generation -run Foundation -count=1
go test ./internal/web -run Foundation -count=1
go test ./internal/recovery
```

Walk through a retained Calvin-cycle-style question, including a typed draft:
submit the foundation request (the review UI no longer shows a Too advanced
button, MIS-157) → pending/failed saved job → ordinary review still available →
explicit bounded retry → saved explanation → ordered diagram/reference →
warm foundation practice → held feedback/Next → deliberate identical-target
return. Save the original presentation ID and exact prompt, not merely a similar
question. Restart the app; revisit from Library and reuse without another job.
Use native browser pointer/keyboard events. If a headless harness loses focus,
restore browser focus rather than bypassing the DOM with `.click()`; record
any focus emulation separately from real-device acceptance.

Drop a POST response AFTER commit, observe unknown status, and retry the identical
operation ID/payload. Expect one read/practice observation and current committed
progress, not a replayed transition. Test a stale bridge revision and an old tab
after newer review advancement. It must not overwrite the newer target. Check
that request-only creates no negative event, read/practice changes no FSRS card,
and the exposed target cannot report cold success. Complete warm A and B with
cold C due: C must appear next, not an A/B loop. A sole warm target must remain
unavailable after Next, reload and restart; selection, preview, counts, empty
state, next availability and Library must agree on completion plus 24 hours.
Compare all FSRS bytes before/after warm work. A deliberate reset or newer real
review must override old consumption; explicit Reveal retains assisted/Again.
A paused unknown foundation job retains allowance and never auto-retries.
Do not confuse retrying a saved browser operation after response loss with
resending paid provider work. For an unknown provider outcome, keep the job and
reservation intact and hand off through the runbook's
[generation/spending boundary](../runbook.md#generation-and-spending); no fresh
request or manual charge release is authorized by a QA interruption.

Repeat the foundation request on the same occurrence with an edited draft; reload/restart
must retain the latest acknowledgment. Replay an older exact operation and send
a stale new operation after bridge advancement: neither may revert draft or
progress. Clear the draft and ask for help; required answer validation must not
block help, while an empty Check answer still must not submit.
Include an older ungraded answer: it must not hide a later saved draft or an
acknowledged clearing. A newer ungraded submission must become the editable text
again, while its immutable prior attempt remains visible separately.

During a later cold occurrence of the same quiz version, GET an old graded
bridge with Accept: application/json. Before acknowledgment, its target's answer,
explanation, evidence, variants, submitted answer and saved draft, plus old warm
feedback, must be withheld, not merely its materials. Compare immutable history
and original snapshots before/after that GET. A synthetic future occurrence can
exercise this boundary, but is not evidence of elapsed-time retention.

Inspect exact material/unit versions and separate coverage roles/provenance.
Rendered markup must stay inert; references must be safe URL-only pointers,
never fetched/verified source claims. Exercise denied owner/peer/Host and CSRF
on the new reads/mutations, with no-store/CSP and session-loss behavior.

For migration, use a populated v1 DB containing old presentations, assistance,
reviews, full FSRS cards, corrections, content versions, receipts, queued and
unknown paid work. Read-only candidate `check` must leave v1 bytes unchanged;
startup migration must preserve every old row and leave historical knowledge
unmapped. Reject incomplete/forged/newer schemas before readiness or claims.
Compare all foundation export sections after a complete backup/unused-path
restore and exercise the restored service, not only its tables. Account separately
for intentional restored-job pauses and new service-start backup receipts.
Prove the retained v1 snapshot/compatible-binary recovery path described in the
runbook; never overwrite a live DB to demonstrate rollback.

Local provider-boundary fixtures prove serialization, publication, failure,
reservation and retry mechanics ONLY. The
[September 11 rollout receipt](foundation-rollout-20260911.json), particularly
`authorized_live_followup`, records bounded selected-provider generation,
native private-browser interaction, and off-VM restore of the new records
through private UI on the exact deployed binary. Its earlier blockers and
historical PASS/UNVERIFIED fields remain dated observations, not current status.
The operator subsequently rejected the foundations experience; see
[current product direction](../../VISION.md) and the
[design assessment](../design/concept-centered-study.md#operator-findings-and-direction).
Technical success does not reverse that assessment or authorize further work.
This slice does not test or claim MIS-60–63's unimplemented planner/estimator
behavior.

## Independent recovery

Use the approved daily/pre-release, 30-day new-app retention policy and
RPO 24h/RTO 60m targets. Recover a completed R2 archive using independently
retained operator capability and compatible binary/configuration. Restore only
into an unused path on an isolated environment. Do not copy live integrations
or duplicate scheduler ownership into a preview.

Compare the recovered export to the snapshot's acknowledged state. Keep
uncertain jobs paused. When claiming service recovery, also activate the
unprivileged service, exercise private HTTPS and the actual UI, and record
elapsed time and excluded steps. An existing VM, copied files, or successful
SQLite integrity check alone is not a complete disaster-recovery proof.

`--allow-local-backup` is only for an explicitly synthetic rehearsal. It does
not establish production remote-backup success. Never use it for live cutover.

## Evidence and historical material

[The earlier Go acceptance receipt](personal-go-acceptance-20260909.json) records
bounded live generation, trusted touch, interrupted-response/access recovery,
and independent data restoration. Its original observation predates subsequent
operator phone approval and full service recovery; do not rewrite history into
an unobserved claim.

[The cutover receipt](personal-go-cutover-20260910.json) reconciles all 32
criteria, names the immutable browser/release artifacts, and records corrected
capture/dispute/availability paths, live generation, and private activation.
S09.2 remains unverified for a different exe account. Global logout kept private
content hidden; history return encountered an upstream authentication redirect
loop, while fresh navigation reached sign-in. Do not substitute a VM token,
local owner-header fixture, or global logout for a second-account observation.
The receipt's pending S04.2/AI1 review describes its date, not the latest
foundations assessment. [Current acceptance](../../SPEC.md) distinguishes that
negative operator assessment from general generated-material usefulness, which
is not established. The receipt enumerates all seven candidate quality
contracts, including unmeasured p95 and full text-zoom/accessibility coverage;
functional checks do not silently pass them.

[The September 11 foundation rollout receipt](foundation-rollout-20260911.json)
adds exact-binary activation and bounded live provider/browser/new-record
recovery evidence. Read `authorized_live_followup` alongside its explicitly
retained earlier observations. Both temporary VM-scoped QA tokens were revoked;
that exercise supplies evidence, not reusable credentials or fresh authority.

Dated Rust/Cloudflare, beta, dogfood, performance, and generation receipts remain
historical evidence. Their old commands require the corresponding historical
revision, not the current Go tree. `fixtures/legacy-generation` preserves the
old generation corpus; science/research remain useful without implying parity.
The retired Worker monitor is disabled, not repointed to an incompatible Go
health contract. Native backup monitoring belongs to retained recovery.

Every report names the source/artifact, environment, data provenance, actions,
observed result, and limitations. Distinguish PASS, FAIL, and UNVERIFIED. Owner
phone acceptance is separate from automation; neither establishes sustained
use, learning efficacy, inbox delivery, an availability SLA, or unmeasured
provisioning/DNS recovery time.
