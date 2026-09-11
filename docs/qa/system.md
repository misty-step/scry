# Scry verification

The current application is Go/SQLite/HTMX on private exe.dev ingress. Choose
proof for the changed behavior; a build, a fixture, a browser observation, a
live provider result, and physical-phone acceptance establish different things.
`SPEC.md` owns acceptance. The runbook owns live authority and recovery.

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
| Learning/SQLite review | `go test ./internal/learning ./internal/store` | Exact retry/restart and durable event/schedule agreement |
| Generation | `go test ./internal/generation` | One bounded, authorized live request on representative material; inspect provenance, coverage, rejections, and actual spend |
| HTTP/browser | `go test ./internal/web` | Actual browser interaction against the changed surface, not DOM-click substitution |
| Recovery | `go test ./internal/recovery` | Completed remote checksum readback and independent restored-service proof |
| Deployment | Current shell syntax plus exact-binary gate | Actual unprivileged process/readiness after protected activation |
| Historical recovery | `bun run test:recovery` | Use the corresponding historical store/tooling; never reinterpret old formats as Go snapshots |

Do not add permanent tests for wiring or source text. Keep regressions for
observable boundaries, races, precedence, and failure transitions. Mock only
external boundaries; repository-owned storage and learning collaborators stay
real.

## Browser and private ingress

Run an isolated development app with synthetic data:

```sh
go run ./cmd/scry serve --dev --db data/scry.sqlite --addr 127.0.0.1:8080
```

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
foundations; requests do not invoke Reveal. Relevant focused commands:

```sh
go test ./internal/store -run 'Foundation|PopulatedV1' -count=1
go test ./internal/generation -run Foundation -count=1
go test ./internal/web -run Foundation -count=1
go test ./internal/recovery
```

Walk through a retained Calvin-cycle-style question, including a typed draft:
Too advanced → pending/failed saved job → ordinary review still available →
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

Repeat Too advanced on the same occurrence with an edited draft; reload/restart
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
reservation and retry mechanics ONLY. Live selected-provider material quality,
operator usefulness, actual private ingress/phone access, independent review,
final exact-binary gate, fresh off-VM backup and protected activation remain
UNVERIFIED until their separate authorized exercises. This slice does not test
or claim MIS-60–63's unimplemented planner/estimator behavior.

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
S04.2/AI1 also await operator material-usefulness review. The receipt enumerates
all seven candidate quality contracts, including unmeasured p95 and full
text-zoom/accessibility coverage; functional checks do not silently pass them.

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
