# Rust Cloudflare replacement candidate

The operator instructed a full redesign/rewrite on 2026-10-07. This document
owns the replacement's build and proposed release procedure.
[The runbook's dated serving observation](runbook.md#rust-production-serving--observed-2026-10-20)
owns deployed state. The retained Go source remains in Git at `c6ba395`;
compatible artifacts and private configuration remain independent recovery
material. Old Rust Workers, Worker/R2 data, frozen Postgres, and the retained VM
stay separate and paused.

The candidate is a ground-up implementation, not a resurrection of the retired
Rust stack. [ADR 006](adr/006-one-way-revival-custody.md) owns the subsequently
authorized one-way Go revival and Cloudflare-only custody exception; it does
not provide an implemented converter or learner-data acceptance. A green gate,
local workerd, authored fixture, screenshot, or merge alone does not prove
production readiness, model quality, or physical-phone acceptance.

## Architecture and ownership

[ADR 004](adr/004-rust-cloudflare-candidate.md) owns candidate architecture,
module boundaries, transaction/ingress mechanisms and scheduler compatibility.
[SPEC](../SPEC.md) alone owns stories, behavior and policy, including capacity
and recovery. [DESIGN](../DESIGN.md) owns visuals; [QA](qa/system.md) owns proof.
The archive implementation sizes before reading photo bodies; this procedure
does not independently redefine SPEC's limits.

## Build and isolated development

Tooling is pinned by `rust-toolchain.toml`, Cargo.lock, package-lock.json, and
`.exe/setup.sh`: Rust 1.98.1 + `wasm32-unknown-unknown`, worker-build 0.8.7,
Node 22.22.0, and Playwright 1.63.0. VM setup uses official rustup and verified
Node/Bun/Gitleaks archives; it does not read production configuration.

```sh
cargo test --locked
cargo clippy --all-targets -- -D warnings
cargo check --locked --target wasm32-unknown-unknown
worker-build --release --locked
bun run ci
bun run dev
```

`bun run dev` / `bun run rust-dev` create a fresh private run directory and a
loopback workerd session with local SQLite/R2, explicit synthetic identity, and
no inherited provider/deployment/operator capabilities. Repository private env
files are not loaded. The authored-demo action refuses existing state and does
not call a provider. Stop the run-owned process and retain its evidence before
removing only its temporary directory.

The source-snapshot gate exports the exact tested Worker modules/assets,
checksums, source inventory/archive, and proof. Release output must use committed
source with `--require-committed`; worktree output is development evidence. Use
the bytes exercised by the smoke and browser, without rebuilding at deployment.
Push/PR CI prepares a candidate with functional Rust/WASM checks and browser
story walks. The same `ci.yml` workflow can promote an already-verified master
artifact without rebuilding; it never changes the serving domains automatically.

## Fresh namespace and private configuration

[wrangler.toml](../wrangler.toml) is a deliberately unconfigured template. Its
Worker, migration, space name, and R2 names identify a new target; they do not
bind current recovery stores or provision an approved production origin. Closed
configuration fails before private content is served.

An approved preview/release configuration must select a fresh Worker/Durable
Object namespace, singleton `SPACE_NAME`, separate private `SCRY_ASSETS` and
`SCRY_BACKUPS` buckets, canonical HTTPS origin, Access application/team/audience,
and exact owner subject. Keep preview identity/state/buckets separate from the
release. No legacy namespace, live database, old bucket, or historical cron is
reactivated, renamed, or copied as a shortcut. No Container is provisioned for
this new target.

Public vars name the mode/origin, Access issuer/audience, alternate read-only
hosts, namespace, and explicit models/endpoints. Secret bindings are
`OWNER_SUBJECT`, `INTERNAL_KEY`, `OPENROUTER_API_KEY`, `JEV_API_KEY`, and separately
scoped `OPERATOR_KEY` / `RESTORE_TOKEN` only where needed. Use secret bindings or
interactive secret input; never embed values in source, argv, logs, env examples,
images, Worker modules, or public receipts. `.dev.vars.example` contains only
safe synthetic values. Production `.env` and historical systemd configuration
are not files to source or copy wholesale.

### Private runtime errors

The Rust fetch entrypoint reports returned 5xx responses and unexpected Rust
request errors through the existing Scry Sentry envelope transport in
`deploy/cloudflare-hosting/telemetry.mjs`. It sends only fixed categories and
deployment identity, not requests, raw errors, bodies or owner identity. Bind
native `SCRY_VERSION` version metadata and set `SENTRY_ENVIRONMENT` to
`production` or `synthetic-staging`; absent/invalid telemetry configuration
disables sending without changing study. Rust events use logger `scry.rust`,
boundary `rust-worker` and release `scry-rust@<Worker version UUID>`, distinct
from retained Go-edge events. The existing Sentry workflow owns alert routing.
No new Sentry credential, failure endpoint or scheduler is introduced.

`node --test qa/rust-telemetry.test.mjs` exercises the exact compiled native
entrypoint against a controlled HTTP sink, including real one-minute recovery.
Retrieve a controlled hosted synthetic event and its owned intake separately
before claiming delivery; local tests, Go-edge events and source merges do not
prove Rust production reporting, uptime/cron health or owner access.

### Dedicated QA learner

Ordinary `preview` and `production` may admit the approved `scry-qa` Access
service token to its own fixed notebook. Bind that token's exact client ID as
Worker secret `SCRY_QA_ACCESS_CLIENT_ID`; do not bind its client secret to the
Worker. Missing/empty binding denies all QA service access. Owner subject,
`SPACE_NAME`, internal/operational keys and existing owner routes stay unchanged.
Do not configure the owner's space as `scry-qa`: a collision refuses QA access.

The authorized provisioner creates token `scry-qa`, adds its own Service Auth
policy on the ordinary preview and production Access apps for the whole app,
and retains the pair in `misty-step/SCRY_QA_ACCESS_CLIENT_ID` and
`misty-step/SCRY_QA_ACCESS_CLIENT_SECRET` in pass. Provisioning is an explicit
credential/Access write, not performed by a deployment or test. Use that pair
only in a dedicated QA browser/request context, never in a URL, capture or
shared browser. The existing `synthetic-preview` principal is different and
does not authorize ordinary preview or production.

The dedicated QA account may also sign in as a person. Plain var `QA_EMAIL`
(`scry-qa@agentmail.to` in `deploy/rust-production.toml`, empty elsewhere) maps
a signed Access identity token, with non-empty `sub` and exactly that `email`,
to the same QA principal. The match is byte for byte, and the var must be one
lowercase address; anything else maps nothing. The owner subject wins over the
email, and a `scry-qa` owner space refuses it like the service token. The QA
account's Access policy membership is a separate Access/auth change.

QA requests verify the same signature, issuer, audience, time and CSRF fences.
The signed service identity selects only SQLite object `scry-qa`, photo prefix
`photos/scry-qa/`, and backup prefix `snapshots/scry-qa/`. QA has its own durable
jobs, history and alarms, but no paid provider capability or separate allowance.
Generation, photo transcription, meaning checking and criticism remain disabled
for QA even when owner providers are configured, including after runtime restart.
Captures remain saved with an honest unavailable-preparation state; local study
and learner self-check do not require a provider. See SPEC S09.4 before planning
a real-provider QA journey: it requires an explicitly approved allocation and
implementation, not just an Access token or provider binding.
Settings backup operates on QA only. Operator routes and the owner's machine
backup route are forbidden to QA even with operational headers. `/healthz`
remains the existing safe owner-backup freshness probe, not QA notebook access.
Removing the QA ID binding denies subsequent service requests without deleting
either notebook. Access revocation is separately owned by the provisioner.

`node --test qa/runtime.test.mjs` exercises both ordinary modes against the
compiled WASM, including cross-principal reads/writes, same-operation photo
keys, complete backup isolation, revocation/recovery and alarms after runtime
restart. This is not hosted Access or authenticated phone-use evidence. After
provisioning, exercise the real phone-size journey on ordinary preview first,
then production, using only authored QA material and recording video.

The application allows $3.50 per rolling day and reserves $0.50 per preparation
against shared generation/meaning/critic use. Unknown sent outcomes keep their
reservation. The provider's existing $25/week key cap is a separate control.
Live-provider use requires the current session's integration authority and a
bounded exercise; do not clone integrations into a recovery preview.

## Reviewable release procedure

### Ordinary code-only shipping

The owner's 2026-10-29 simplification decision replaces the generated-release
PR detour with one path:

    reviewed source PR -> protected merge -> exact-source CI build/tests/walk
    -> same tested artifact -> deploy -> live module/storage/route readback

`ci:full` builds the Worker once and runs its runtime tests and `qa/walk --all`
against those bytes. The `story-walk` status name is a lightweight dependency
on that same job, retained for existing branch protection; it does not build or
walk again. Build/test receives no deployment credential. Promotion serializes
only actual production work under `scry-rust-production`, never a fixed wait.
A failed deployment does not invalidate a successful build job's immutable
artifact. Manual retry requires its exact source, artifact ID and digest:

```sh
gh workflow run ci.yml -R misty-step/scry --ref master \
  -f mode=promote -f expected_head=FULL_MASTER_SHA \
  -f artifact_id=VERIFIED_MASTER_ARTIFACT_ID \
  -f artifact_sha256=VERIFIED_ZIP_SHA256
```

There is no automatic version/changelog PR, tag requirement, or second build.
Historical tags, published artifacts and `CHANGELOG.md` stay intact. Intentional
version/notes updates are ordinary reviewed source changes; the exact source
revision remains the serving identity even between version updates.

Known availability limit accepted for this scoped simplification: Actions keeps
these artifacts for 14 days. Both candidate admission and live-source module
verification require an unexpired exact CI artifact; a sufficiently old live
artifact therefore blocks promotion. This fails closed and is not recovery
failure or data loss. Durable artifact provenance remains deferred/STOP; this
change adds no release-store fallback, unsigned sidecar, or alternate admission
path to bypass the exact successful CI artifact requirement.

[`scripts/scry-promote.py`](../scripts/scry-promote.py) retrieves the artifact
from a completed successful exact-source master **CI job**, checks transport
and every archived file checksum, and invokes native Wrangler with no build.
It refuses moved source, divergent/manual rollback history, split traffic,
unrecognized provenance, or drift in private storage and serving edges.
Secret-only native versions retain their existing chronological provenance
checks; an opaque etag alone never establishes bundle identity.

Before ordinary upload, [`scripts/scry-cloud-backup.py`](../scripts/scry-cloud-backup.py)
requires unchanged persisted data vocabulary, App validation/grade-replay
dependencies (including engine, generation and Unicode tables), archive/SQL write
methods and locked dependencies between the actually serving source and candidate. This is
conservative: a shared schema number alone is not compatible rollback proof.
The existing populated-history, archive roundtrip, restart and predecessor
refusal tests remain in the same full CI gate. The prior native version and
source are recorded as the code rollback target on unchanged bindings.

The backup gate reads **metadata only** from the existing private R2 bucket.
It requires a complete bounded listing and a native-shaped snapshot no older
than 24 hours, with a nonzero bounded size and native ETag. The qualified native
code validates the full App/history/photos before its single R2 upload; R2
provides [strong object/list consistency](https://developers.cloudflare.com/r2/reference/consistency/).
This is honestly `snapshot_metadata_verified`, **not** full readback, successful
persisted backup status/alarm, or independent restore verification. No archive
body, snapshot key, ETag, or learner hash is published or downloaded. A fresh
backup is not triggered on every code patch; the existing 24-hour RPO is the
explicit ordinary-rollout risk allowance. Missing/stale metadata stops upload.

The same tested `index.js` and WASM are read back from the serving Worker and
checked byte-for-byte. Namespace IDs, private buckets, space, routes, domains,
preview disablement and single-version 100% traffic are checked again. The
receipt records exact deployed source and compatible previous native version;
it does not pretend control-plane readback is owner sign-in or learner UI proof.
Existing live private-ingress/phone observations remain separate and honest.

Compatible rollback deploys a retained code artifact onto the **same current
data**, never a snapshot rewind. A failed post-upload check records attempted
upload without success; inspect actual serving state before retry or rollback.
An ordinary same-source recheck still reports `recovery_verified=false`; it does
not erase or close a failed maintenance receipt, cleanup incident, or existing
failure issue. It creates no new incident-clearance approval gate.
A rollback/divergent history still requires explicit operator disposition.
Never import, erase, rename or rebind historical/current learner stores to make
a rollout pass. Independent review and protected exact-head CI remain required.

### Initial custody transitions and recovery maintenance

A custody-format change is not an ordinary code-only release. It requires a
fresh complete backup, independently retrieved fresh-namespace restore with
compatible frozen assets, preserved immutable history/photos, and a separately
proven compatible rollback artifact before activation. Destructive migration,
new credentials, new spend or private-data scope requires separate authority.
The historical one-way revival still requires ADR 006's canonical reconciliation;
source merge and synthetic tests do not close its acceptance gaps.

In particular, the old v0.8.1 / `62a7716` Worker deliberately refuses NATO
mapping archives. It is a refusal control, **not** a rollback after mappings
have been saved. The first NATO custody transition therefore cannot silently
use the ordinary unchanged-custody path. Later same-custody releases can use
their actual mapping-capable prior live version without repeating a hosted
restore for every patch.

The explicit `mode=recover` path below remains available for admitted
maintenance. It is not invoked automatically by normal source merges. The
former Go Container deploy job remains retired; historical tooling and stores
keep their original scope.

### Canonical-origin promotion

For an origin change, first read the runbook's
[dated preparation and outstanding proof](runbook.md#canonical-origin-change--prepared-2026-10-20).
Use the release procedure above with the reviewed production configuration.
Require its unchanged bindings and serving-edge readback. No domain attachment,
Access-policy change, or new capability follows from changing origin vars.

The daily trigger uses a separate checkout. Keep that checkout on the serving
origin until promotion succeeds. Pulling the new origin before promotion sends
backup POSTs to an alternate host, which returns 421. Leaving the old checkout
after promotion has the same result. Immediately after successful promotion,
run on Mirrodin as the existing timer user:

```sh
git -C "$HOME/.local/share/scry-backup" pull --ff-only origin master
systemctl --user start scry-daily-backup.service
systemctl --user show scry-daily-backup.service -p Result -p ExecMainStatus
journalctl --user -u scry-daily-backup.service -n 10 --no-pager
```

Require `Result=success`, `ExecMainStatus=0`, and
`Scry production snapshot verified.` Independently confirm the new private R2
snapshot and Sentry `ok` check-in. Observe the next natural timer execution too;
a manual run does not prove scheduled delivery. Record the serving origin,
checkout revision and both backup observations in the runbook. If a compatible
rollout changes the origin again, align the trigger checkout with the serving
configuration and repeat this verification. Never replay a rejected POST through
an alternate-host redirect.

## Cloud-only release recovery gate

This is the explicit custody-transition/recovery-maintenance path, not the
ordinary deployment prerequisite. `mode=recover` independently restores a
complete pinned archive before exact production code upload, then checks
persisted recurrence and cleanup. It stays inside Cloudflare, never copying
learner archives into Actions. Ordinary `/healthz`, upload metadata and green
source CI are not an independent restore receipt.

One explicitly authorized operation supplies `recovery_approval` as workflow
input, bound to the **exact tested source SHA**. Native repository permission to
dispatch the workflow is required; there is no dependence on the retired release
bot's actor name. Repository, master ref, manual event, first attempt, exact
source, expiry and cost bounds remain checked. This changes no GitHub permission
or credential. Expiry, cost and observation
windows are operational input, not application source or generated version
metadata. Missing, expired or wrong-source authority refuses before billable
work. Source changes never renew the original $20 logical-operation approval.

A content-free private R2 claim consumes the approval ID once under production
concurrency; GET/PUT/readback is **not** cross-writer CAS. Existing/conflicting or
uncertain claims stop, never automatically retry the drill. Cleanup verifies
and retains the less-than-1-KiB consumed tombstone: it does not delete active
admission and does not hold production until expiry. The tiny bounded storage
record prevents replay; actual temporary Workers, namespaces, objects, bucket
and schedules must still be removed and verified immediately. Missing or
changed tombstones remain a reported incident, not success or permission to
retry. Receipts upload as soon as cleanup finishes, including on failure.

Before billable listing or mutations, the gate requires the existing Standard
usage model and explicit bounded cost admission. Provider keys, retention
candidates, unknown live modules/source, or incomplete listings stop. It selects
exactly one fresh archive from a fixed, reviewed prior observation window; that
metadata selects input but proves neither integrity nor recurrence. Complete
independent restore/readback must pass before the deployment callback can run.
The gate does not schedule another production backup or require Cron analytics.
Historical run-owned schedule cleanup remains supported: any unresolved Cron
is a production cleanup incident, not storage-only residue, until absence is
verified.

A separately approved temporary service binding lets the same private checker
make one fixed `GET /healthz` after exact deployed-module verification. The
expected archive hash is computed and retained inside Cloudflare, passed only
in a header, never a URL. The existing health route returns only a boolean proof
of matching successful fresh backup status and a persisted future alarm due no
later than the existing 24-hour cadence. It rereads the revision/status around
the alarm read and refuses races without writes, rearming or job recovery.
Ordinary health behavior is unchanged. No Access credential, bypass, public
route or production binding is added. The temporary service is not permission
for another route, caller-selected request, or background operation.

A post-upload proof or cleanup failure is explicitly deployed/upload-attempted
but unverified, never success through a same-source retry shortcut. Existing
compatible-code rollback safety and 30-day retention remain unchanged. A persisted alarm proves
current scheduling, not future delivery; the next natural daily run remains a
separate observation.

Recovery uses separate temporary private Workers/DOs/R2, no public route,
provider, owner or QA binding, and short-lived internal/restore/transfer keys.
An isolated adapter delegates to exact frozen candidate `index.js`/WASM bytes;
its different entrypoint supplies only authenticated content-free size checks
and bounded recovery admission, never a production wrapper. The bridge reads a
size/ETag-pinned snapshot inside Cloudflare, restores once, takes one complete
backup, and strictly compares application/photo data with only documented
restore changes. Rehearsal admission retains the native **16 MiB complete archive**
envelope, including embedded photos, while independently limiting serialized
**App state to 1 MiB and 1,000 recursive entries** in both bridge and target
before native restore writes. Photo bodies go only to R2; the App/SQL storage
bound is unchanged. This does not change the production archive format.
Oversized data is a stop, not permission to raise the budget.

Cost admission assumes an existing Workers Standard account, no new subscription
or free allowance, and rounds every meter independently: the bounded one-drill
and first-storage-period cumulative quote is **$19.235**, within the original
**$20** cap ($0.765 headroom), not a new allowance per failed attempt. The
170-million SQL-read envelope includes the prior bounded production bootstrap
scans and one isolated continuation; it does not assert measured usage.
The driver limits control-plane requests (128 including cleanup), private preview
requests (5), CPU (30,000 ms per invocation), subrequests (100), and admission
time (15 minutes). Already-admitted I/O can outlive capability expiry; the finite
call count and 72-hour margin before the **verified actual account billing-period
boundary** bound its compute billing. Calendar month-end is not that boundary.
An explicitly reviewed continuation also pins its prior accounting start; the
same verified period must cover that usage, not grant another rounded allowance.
The original accounting start through expiry plus drain must fit the reviewed
96-hour cumulative window; admission also compares the full meter quote against
the approved cap. Earlier failed isolated setups invoked no Worker or restore.
Missing billing read permission or ambiguous period metadata stops before the
claim or billable listing; no permission change is implied.
No target/bridge alarm or Cron can continue after that drain. Failed cleanup
therefore leaves only temporary storage, conservatively **$0.215/month** within
the approved **$0.25/month**. Expiry is not deletion.

The fresh SQL namespace accepts at most 1,018 partition/chunk rows and two later
meta versions. Charging UTF-16, JSON re-escaping, keys and every obsolete payload
version gives at most 12 MiB serialized payload. For each allowed SQLite page
size, the tested B-tree/overflow/pointer-map calculation plus initial/schema/KV
allowance is at most 27,161,600 bytes; transient rebalancing adds at most 143,360,
still below the adapter's 32 MiB check. This bounds stored SQL data, not unknown
provider replicas; hosted size checks corroborate rather than replace the
pre-write bound. See [SQLite file format](https://www.sqlite.org/fileformat.html),
[DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/),
and [R2 billing-unit rounding](https://developers.cloudflare.com/r2/pricing/#billable-unit-rounding).

Temporary resource identities are verified before cleanup. Cleanup failures
block success and retain a content-free receipt naming only this run's resources;
they do not authorize deleting an uncertain or historical store. The existing
Actions failure issue links the run, without learner content, credentials,
private snapshot keys or capability URLs. Proof of this limited hosted restore
does not establish owner UI, physical-phone acceptance or continuous monitoring.

## Independent fresh-space restore

### Mirrodin-triggered daily snapshot

The repo-owned [`scry-daily-backup.timer`](../deploy/mirrodin/scry-daily-backup.timer)
runs daily at **09:30 UTC** with `Persistent=true` to catch up after downtime.
Its [service](../deploy/mirrodin/scry-daily-backup.service) runs
[`scripts/scry-daily-backup.py`](../scripts/scry-daily-backup.py) from the dedicated
checkout `~/.local/share/scry-backup` on Mirrodin. Keeping the script and
`deploy/rust-production.toml` in one checkout avoids separate copies drifting;
each run derives its destination from that checkout's `[vars].CANONICAL_ORIGIN`.
An origin promotion does not update the checkout. Pull it immediately after
successful promotion and verify a snapshot through the
[canonical-origin handoff](#canonical-origin-promotion). No Cloudflare Cron
Trigger is used.

The script needs Python 3.11 or newer and local `pass` with noninteractive GPG
access for the timer's user. It reads `misty-step/SCRY_ACCESS_CLIENT_ID` and
`misty-step/SCRY_ACCESS_CLIENT_SECRET` for the existing `scry-machine` Access
token, plus `operations/scry/production-20261016/backup-trigger` for the existing
Worker `BACKUP_TRIGGER_KEY`. It strips trailing newlines and sends those values
only as `CF-Access-Client-Id`, `CF-Access-Client-Secret`, and `x-scry-backup-key`
headers to canonical `POST /internal/backup`. Redirects are refused. Never put
capabilities in argv, URLs, source or receipts. This principal cannot read
learner routes or act as the owner. Keep synthetic and production keys separate.

Each execution sends Sentry native cron `in_progress`, then `ok` only after
snapshot verification or `error` on failure, using one check-in ID. The script
reuses the public DSN in `deploy/cloudflare-hosting/telemetry.mjs`; there is no
new credential, SDK, watcher, or journal reader. Reports contain only execution
ID, fixed monitor/environment/status, script SHA-256 release, duration, and the
schedule. No origin, backup credentials, raw error, or response body is sent.

The first start check-in upserts `scry-mirrodin-daily-backup` in Sentry project
`scry`, environment `production`, with `30 9 * * *` / UTC, a 15-minute check-in
margin, three-minute maximum runtime, and one-failure/one-recovery thresholds.
Once registered, Sentry detects a missing start after **09:45 UTC** even if the
timer never fires or Mirrodin is down; a killed/hung execution times out there
without a local failure hook. It uses the existing Scry production issues and
monitor failures workflow to `#ops`. Both check-ins have a two-second timeout,
refuse redirects, and are never retried. Reporting failure does not prevent the
backup attempt, but prints only `Scry backup monitoring unavailable.` and makes
the service exit non-zero. Ingest acceptance alone is not owner-delivery proof.

#### Install or update after merge

The conductor installs from master and disables the old Hermes job after merge;
the implementation PR does neither. As the Mirrodin user with the existing pass
access, create the dedicated checkout once:

```sh
mkdir -p "$HOME/.local/share"
git clone --branch master --single-branch https://github.com/misty-step/scry.git "$HOME/.local/share/scry-backup"
```

For installation and each update, refresh the clean checkout and copy both units:

```sh
git -C "$HOME/.local/share/scry-backup" pull --ff-only origin master
install -Dm644 "$HOME/.local/share/scry-backup/deploy/mirrodin/scry-daily-backup.service" "$HOME/.config/systemd/user/scry-daily-backup.service"
install -Dm644 "$HOME/.local/share/scry-backup/deploy/mirrodin/scry-daily-backup.timer" "$HOME/.config/systemd/user/scry-daily-backup.timer"
systemctl --user daemon-reload
systemctl --user start scry-daily-backup.service
systemctl --user show scry-daily-backup.service -p Result -p ExecMainStatus
journalctl --user -u scry-daily-backup.service -n 10 --no-pager
```

Require `Result=success`, `ExecMainStatus=0`, and the journal line
`Scry production snapshot verified.` Confirm `GET /healthz` is 200 using the
same machine token and `User-Agent: ScryDailyBackup/1.0`, and independently confirm a new snapshot object in private
R2 `scry-rust-backups` without logging its contents. Then enable the timer:

```sh
systemctl --user enable --now scry-daily-backup.timer
systemctl --user list-timers scry-daily-backup.timer
loginctl show-user "$USER" -p Linger
```

Confirm user lingering is enabled so the timer survives logout. If needed, the
conductor enables it with `loginctl enable-linger "$USER"`.

Before retiring Hermes, read back the Sentry monitor's production schedule,
thresholds, successful check-in, and enabled existing `#ops` workflow. Retain a
controlled failed-run notification in that channel and independently observe a
missed check-in alert before calling missed-run delivery proven. Use only a
scratch checkout with an invalid origin and synthetic backup credentials for a
failure drill, never production with bad credentials. A manual success registers
the schedule; without an accepted first upsert there is no remote missed-run
coverage. If monitor upsert or existing routing is unavailable, keep Hermes and
escalate the exact missing configuration/credential instead of creating one.

Only after installation, snapshot verification, and owner-alert proof, the
conductor disables archived Hermes cron `37ca5bbdd570` if reachable; otherwise
records its reachability limit on #309. Disable it before #301's canonical
promotion, since its hard-coded origin will then be rejected. The implementation
PR does not install units or disable Hermes.
Failure exits non-zero with a fixed reason in the journal, without upstream
bodies, URLs, error details, or credentials. Missing noninteractive pass access
must be resolved before relying on the timer.

#### Stop or roll back the daily trigger

Reverting source does not stop an installed timer. As its Mirrodin user:

```sh
systemctl --user disable --now scry-daily-backup.timer
systemctl --user stop scry-daily-backup.service
rm -f "$HOME/.config/systemd/user/scry-daily-backup.service" "$HOME/.config/systemd/user/scry-daily-backup.timer"
systemctl --user daemon-reload
```

Also pause the `scry-mirrodin-daily-backup` production monitor in Sentry during
an intentional stop so it does not report expected missed runs. Retain the
monitor's history. To resume, install the reviewed master units as above,
re-enable the monitor, verify a successful check-in, and enable the timer.
Coordinate an authorized replacement daily trigger before leaving this stopped;
do not silently restore Hermes across an origin change.

Missing or wrong keys return 403; non-POST returns 405. Success returns 200 only
after the same complete R2 verification used by the operator backup. Failure
returns 503 `backup_failed` without private diagnostics. Record an actual natural
timer execution plus independent remote archive readback before calling daily
delivery proven. `/healthz` returns 503 `backup_stale` before a verified backup or
after SPEC's freshness limit; storage/configuration failure returns 503
`backup_unavailable`. Health does not create a learning record or start work.

### Restore procedure

These are source contracts for the replacement candidate. Local emulator tests
exercise them with controlled capabilities. Actual independently retrieved remote
recovery and an approved writer transition still need their own evidence.

For a pre-release snapshot, `POST /operator/backup` requires normal exact-owner
Cloudflare Access authentication and canonical Host/Origin, plus a separate
`OPERATOR_KEY` of at least 32 bytes in `x-scry-operator-key`. Success means the
complete versioned JSON/photo archive was validated and read back byte for byte
from private R2. Independently retrieve that archive and its checksum using
retained recovery authority. Do not infer remote success from a local emulator
or a checksum alone.

Prepare an unused Worker/Durable Object namespace, private R2 bindings, and a
fresh `SPACE_NAME` with the exact compatible frozen modules. Set
`SCRY_ENV=restore`, a canonical HTTPS origin and `INTERNAL_KEY` of at least
32 bytes, plus a separately scoped `RESTORE_TOKEN` of at least 32 bytes. This mode
accepts only `POST /operator/restore`; browser access, ordinary learning routes
and scheduled external work are unavailable. Configure no provider/operator keys
and no live integrations. The restore route uses its scoped token rather than
Cloudflare Access.

Send the complete archive as the request body with `x-scry-restore-token` and
`x-scry-sha256` containing its exact SHA-256. The runtime requires canonical Host,
a truly unused object, the compatible schema/algorithm and deep references, and
a complete archive no larger than 16 MiB including base64 photos. It validates
every referenced photo and reads each restored R2 object back fully. A used space
returns 409; restore never overwrites it. Prepare private request configuration
files outside the checkout with mode 0600 so capabilities stay out of argv,
logs and receipts:

```sh
curl --fail-with-body --config /secure/scry/restore-http.conf \
  --data-binary @/secure/scry/retrieved-archive.json \
  "$SCRY_RECOVERY_ORIGIN/operator/restore"
```

The protected HTTP configuration supplies POST, the token/checksum headers and
content type. Keep the archive itself private. Do not print the configuration,
use verbose request logging, or copy operator env files into the restored Worker.

Restore preserves content versions, attempts, corrections, resets and full cards.
It creates a new CSRF token, remaps photos to fresh private R2 keys, resets the
backup status and sets `restored_paused=true`. Unsent queued/candidate jobs become
paused; sent generation or critic requests become unknown with allowance retained.
Unfinished assessments expose the saved answer through learner self-check:
unsent assessments pause and sent outcomes remain uncertain/accounted. It never
replays historical paid requests. Inspect these expected changes explicitly when
comparing exports. No automatic external transmission follows successful restore.

After recovery review and explicit approval, prepare normal `preview` or
`production` private configuration for that same recovered namespace and space.
Verify owner/anonymous ingress, immutable records, photo bytes and the actual UI
before enabling external work. Keep retained writer/store ownership separate.
An ordinary login, restore success or deployment alone cannot resume paid work.

### Resume future external work

`POST /operator/resume-work` is accepted only in approved preview/production
configuration. It requires normal exact-owner RS256 Cloudflare Access,
canonical Host and Origin, and a separately bound `OPERATOR_KEY` of at least
32 bytes in `x-scry-operator-key`. Use a protected request configuration carrying
the owner session and operator header:

```sh
curl --fail-with-body --config /secure/scry/resume-http.conf \
  "$SCRY_RECOVERY_ORIGIN/operator/resume-work"
```

This POST atomically clears only the restored pause flag and arms future work.
Historical jobs, assessments and operation receipts remain paused/unknown;
their reservations are preserved. Any retry is a separate deliberate application
action within the existing allowance and attempt limits. An empty object or one
whose pause flag is already clear returns 409; repeating resume cannot silently
requeue work. The endpoint is unavailable in restore or synthetic development.
Record the reviewed namespace/configuration, approved resumption, response and
subsequent bounded observation without recording tokens.

A restored export and compatible Worker module establish data compatibility.
Hosted recovery additionally requires private activation, owner/anonymous access,
UI, fresh writes, and a verified new snapshot. Report those claims separately.
The historical Go SQLite/Worker/Postgres formats are not this JSON+photo archive;
use their exact compatible tools and retain their original names and provenance.


### Isolated recovery privacy readback (2026-10-28)

Cloudflare's script-settings schema permits `observability: null`; Wrangler
normalizes empty observability to disabled. This is not a provider guarantee
that every platform diagnostic is absent. The isolated recovery driver accepts
that exact null shape only after an acknowledged explicit disable PATCH and an
HTTP 200 canonical read with `logpush: false`, no configured tail consumers, and
a matching privacy-reviewed source fingerprint (wrappers, Rust, telemetry and
dependencies; release-number-only changes are ignored). Enabled, missing or
malformed states still refuse invocation. Receipts retain null and label the
assurance `audited-no-learner-content-emissions`, not “provider logs proven off.”

The same documented `ScriptSetting` contract permits a nullable consumer list.
After explicitly PATCHing `tail_consumers: []` on the temporary Worker, the
driver interprets null, absent and empty-array readbacks as no configured tail
consumers; nonempty lists and malformed values refuse invocation. This is a
supported API-contract inference, not evidence that the earlier failed run
actually returned null. Its receipt only establishes failure before the first
privacy observation and therefore before any preview or restore invocation.

The isolated adapter never invokes normal Worker fetch/scheduled handlers. Its
native Durable Object errors are caught; archive parsing and reconciliation
errors become fixed codes, response bodies are cancelled, and internal URLs are
fixed. No learner-content console emission was identified in that audited path.
Platform request metadata is a different claim and is not declared absent.
Namespace identity is retained before privacy checks so refusal still allows
verified cleanup of the operation's own namespace.

Contract evidence: [nullable ScriptSetting](https://github.com/cloudflare/cloudflare-typescript/blob/6dbe1fc45b41f102e5edcd3630d6fd3e803552ce/src/resources/workers/scripts/scripts.ts#L856-L865),
[Wrangler normalization](https://github.com/cloudflare/workers-sdk/blob/6e3924dc1a02cdfa14c2bce5f92f0f3c5fb4193f/packages/deploy-helpers/src/deploy/helpers/config-diffs.ts#L337-L364),
and [disabled-state tests](https://github.com/cloudflare/workers-sdk/blob/6e3924dc1a02cdfa14c2bce5f92f0f3c5fb4193f/packages/wrangler/src/__tests__/deploy/get-remote-config-diff.test.ts#L583-L643).
