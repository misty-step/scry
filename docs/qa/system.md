# Scry verification

The production application is Go/SQLite/HTMX behind Cloudflare Access and Worker
`scry-app-host`, which forwards to one singleton Container. Choose proof for the
changed behavior; a build, fixture, browser observation, live provider result,
and physical-phone acceptance establish different claims.
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
   nor another live request reverses the historical rejection of MIS-59.
   Authorization on 2026-09-23 adopts v5 design, not production activation.

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
| Semantic grading / short-v1 / overrides | `go test ./internal/learning ./internal/store ./internal/semantic ./internal/web` | Exact/variant local, Jev short accept/reject/unsure thresholds and injection/identity fences, rubric semantic-v1, saved answer → self-check or failed Retry, authority in history, one-tap correction with immutable original event and schedule, interruption/exact replay. Bounded Jev holdout quality is separate evidence. |
| Generation, research, and notes | `go test ./internal/generation ./internal/store` | Topic Exa search vs pasted text no-search; Link chosen-page fetch, Photo transcription, absent Exa key, exact quotes/citations, zero-document fallback, sequential plan/questions, immutable note levels, interruption and bounded spending. Live Exa/model quality requires separate authorization. |
| Prepublication critic (US-004) | `go test ./internal/learning ./internal/semantic ./internal/store ./internal/generation` | Candidate persistence before HTTP, one send lease, shared allowance, interrupted unknown spend, critic-only automatic/manual retry, hard veto and skipped publication. Run the opt-in bounded live control test separately; report false accepts/rejects/abstentions, raw requests/responses, model and cost. No production activation or broad accuracy claim. |
| HTTP/browser | `go test ./internal/web` | Actual browser interaction against the changed surface, not DOM-click substitution |
| Recovery | `go test ./internal/recovery` | Completed remote checksum readback and independent restored-service proof |
| Deployment | Current shell/config syntax and exact-binary gate | Actual Worker version, Access-owner/anonymous denial, Container readiness after restore, and remote backup readback after an approved rollout |
| Historical recovery | `bun run test:recovery` | Use the corresponding historical store/tooling; never reinterpret old formats as Go snapshots |

Release smoke compatibility: `seed-fixture` prints JSON with `source` and
`model: "authored-test-fixture"`; first `GET /` contains an HTML form whose
action is `/review/answer`. `Accept: application/json` on `GET /` and
`POST /review/answer` returns `{"review":...,"csrf":...,"operation_id":...}`;
every referenced `/assets/...` response is byte-identical to
`internal/web/assets/...`. These are synthetic compatibility checks, not
real-browser, private-ingress or provider proof.

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
qa_root="${XDG_CACHE_HOME:-$HOME/.cache}/tmp" &&
mkdir -p "$qa_root" &&
qa_dir=$(mktemp -d "$qa_root/scry-qa.XXXXXXXX") &&
printf 'Disposable QA directory: %s\n' "$qa_dir" &&
go build -mod=readonly -o "$qa_dir/scry" ./cmd/scry &&
env -i PATH=/usr/bin:/bin HOME="$qa_dir" LANG=C.UTF-8 \
  "$qa_dir/scry" seed-fixture --db "$qa_dir/synthetic.sqlite" &&
env -i PATH=/usr/bin:/bin HOME="$qa_dir" LANG=C.UTF-8 \
  SCRY_BACKUP_DIR="$qa_dir/backups" \
  "$qa_dir/scry" serve --dev --db "$qa_dir/synthetic.sqlite" --addr 127.0.0.1:8080
```

`seed-fixture` walks the real preparation chain with authored content and no
provider or web call: research finds nothing, the plan names two concepts
(DNS address records, TLS certificate trust) with standard notes, and three
questions link to them. A synthetic learner then reads each intro, so the
Stream opens on a question awaiting an answer. Its JSON is
`{"model":"authored-test-fixture","source":"<source id>"}`; the content is
authored, so reading it is not a cold-recall observation. With no semantic
endpoint, a non-exact answer safely reaches a failed/self-check recovery path
without a live request. The command refuses an existing database or any SQLite
sidecar (`-wal`, `-shm`, `-journal`); it never imports production data.

For exact-binary synthetic smoke, use `scripts/lib/scry_smoke.py` through
the repository gate above; this local recipe does not replace it.

`--dev` supplies loopback development identity, **not** capability isolation:
`serve` still reads inherited `SCRY_*` configuration and starts generation and
recovery workers. The `env -i` allowlist above removes that inheritance,
including model endpoints/keys and remote-backup URLs/tokens. Do not source a
production environment file or add live integrations. With no model endpoint,
new generation jobs become saved, zero-cost configuration failures rather than
producing questions. Recovery still creates local snapshots in the disposable
`backups` directory at startup and on its interval; none are off-VM proof.

Do **not** open a local browser on the workstation. Run browser QA against the
isolated service from an **exe.dev VM** using an actual browser there; keep
synthetic fixture state and integration credentials isolated. After `Scry ready`,
open the service's private QA URL and exercise question → held feedback → Next.
The seed JSON supplies `source` and `model`, not a goal identifier or a
planning route. Use `/map`, `/concepts/{id}`, and `/sources/{id}` only when
those identifiers have been created in the isolated fixture. The recipe
above remains a server-only loopback smoke; it is not browser acceptance. If
port 8080 is occupied, choose another unused loopback port without stopping
an unrelated service.

Stop with **Ctrl-C** in the serving terminal and wait for the process to exit
before manipulating files. To resume saved state, rerun only the final
`env -i ... serve` command in that shell, then refresh the VM browser
(the development session secret is regenerated). To reset, rerun the whole
block for a fresh directory, not `seed-fixture` against the old database. Keep the
printed directory for evidence or remove only that verified disposable
directory after stopping; never delete `data/` or an existing SQLite database
to make seeding succeed.

This loopback recipe exercises authored data, HTTP and persistence, not browser
interaction, live generation, provider quality/spend controls, private HTTPS
ingress, owner authorization, physical-phone touch, or independent recovery.
Use separately authorized proof for those claims.

### Browser interaction and private ingress

Use an actual browser on an isolated exe.dev VM. Check answer → held feedback
→ deliberate Next, capture and saved preparation status, Map/correction, and
interrupted access. Exercise real pointer/keyboard events and inspect the
screen. A DOM `.click()` bypass is
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

## Concept-centered v5 journeys (MIS-162)

The MIS-59 foundations walk is historical; its routes are retired in v5. Its
[dated rollout receipt](foundation-rollout-20260911.json) does not validate the
new experience. Exercise the following with synthetic inputs and real browser
pointer/keyboard actions on an isolated exe.dev VM. Do not run a local browser
or headless Chromium on the workstation.

1. Capture each visible mode: Topic with configured Exa search, Topic without
   an Exa key (general-knowledge fallback), My text with **no web request**,
   Link fetching only the chosen page, and Photo with transcription. Inspect
   Source input/documents, provenance, failed/retry states, and Map preparing
   receipts; submit invalid and oversize input without creating a source.
2. On Stream cover choice/recall, checking, self-check close/unsure/failed
   (including Retry check), result correct/automatic miss/overridden/shown/
   learner, intro, preparing, first-run/caught-up and conflict/error. Verify no
   answer-bearing fields leak to ungraded HTML/JSON. Confirm held result and
   deliberate Next after reload or response loss.
3. Correct an automatic miss with “I was right”; correct an automatic success
   with “Count as a miss”. Repeat the exact operation ID and check one override,
   immutable prior event, authority, and consistent schedule. Check stale
   operation rejection, assisted reveal, and rubric explain-level behavior.
4. A new concept intro shows the standard note before its first question.
   “I know this already” records an observation, not a cold review. Open Map,
   search across concept/note/question/source, focus/pause a goal, and inspect
   prerequisite-first order, status words and estimates. Open a Concept page:
   note levels (request Simpler/Deeper, pending and completed), citations,
   related ideas, practice and gated question details. Two recorded confusions
   should produce one targeted contrast preparation.
5. Open Add via `/add?text=&url=&title=` as a share target; the prefilled fields
   remain editable and **mode choice remains explicit**. Exercise without JS,
   with reduced motion, both color schemes, keyboard focus, 320px/390px and
   desktop. Verify assets actually served match embedded bytes.

These journeys prove changed mechanics only when actually observed. Model
quality, Exa live retrieval, provider spend, protected origin, physical phone,
and independently restored service require separate bounded, authorized proof.

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

[The MIS-162 v5 receipt](concept-study-20260923.json) binds the concept-centered
build (`757acdf`) to its committed-source gate, a 146-shot synthetic state matrix
on an isolated VM, and a bounded live run in which all four capture modes
published through real generation, Exa and Jev. It lists what remains
unverified, including the production-data migration rehearsal and activation.

[The MIS-162 advisory receipt](concept-study-advisories-20260923.json) binds
the follow-up (`bc35504`): fixes as learner-chosen suggestions that belong to
their question, whole-capture withholding while a question waits for an
unaided answer, and 17 named states on the exact gate binary with one live fix.

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
