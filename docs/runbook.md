# Scry production runbook

## Current authority

Scry is one private Go process and SQLite database on **`scry-app.exe.xyz`**.
The canonical application is **https://scry.study**. The operator approved the
phone flow and daily-backup policy. `www.scry.study` and `scry-app.exe.xyz`
redirect reads to that origin; mutations on aliases are rejected, not replayed.
The old Worker is not the live learner store. `scry-dev.exe.xyz` remains an
isolated development/recovery environment, not a second production writer; its
restored rehearsal service is stopped/disabled.

| Boundary | Authority |
| --- | --- |
| Application | `cmd/scry`, `internal/`, one unprivileged `scry.service` on `scry-app` |
| Live data | `/var/lib/scry/scry.sqlite` and SQLite sidecars on `scry-app` |
| Browser identity | Private exe.dev ingress, exact stable owner UserID, trusted proxy peer, Host/origin and CSRF checks |
| Model requests | Scry-only `scry-model` exe integration; OpenRouter key remains outside the VM |
| Recovery | Local consistent archives plus append/read-only `scry-go-backups` Worker and private `scry-go-recovery` R2 bucket |
| DNS | Existing DigitalOcean `scry.study` zone; exe.dev handles custom-domain TLS and private ingress |
| Old runtime | Production and staging Rust Workers paused, cron triggers removed; native Postgres service disabled, recovery backups retained |

The active release is `mis48-e0274bc916de`, from committed revision
`e0274bc916de8bf686160d4068fdb6f3befa540f`. The staged, smoke-tested binary's
SHA-256 is `773911fc83ae07e09774cb7c32ae3e71914435e3b95d6913c6a37d10e587863d`.
Protected activation verified a pre-release remote backup and preserved the
acknowledged learning export exactly. The actual process is active/enabled as
`scry`, bound to loopback port 8080; the deployed checksum matches that artifact.

There is no public signup, magic-link mail, separate frontend, public service
session, maintained legacy CLI/MCP contract, or second application database.
The backend listener is `127.0.0.1:8080`. Do not open it publicly or trust an
identity header from an arbitrary peer. `deploy/scry.env.example` documents the
configuration; populated files and provider capabilities are private.

## Build once, exercise, stage those bytes

```sh
bun run ci
bun run ci:full -- --out target/ci-release --require-committed
```

The full gate uses pinned Dagger tooling. It verifies frozen source provenance,
Go formatting/tests/vet, browser/gateway JavaScript and deployment shell syntax,
retained historical recovery contracts, redacted Gitleaks, and the exact static
Linux amd64 binary against synthetic loopback state. The output includes the
binary, `SHA256SUMS`, source inventory/archive, `proof.json`, and smoke evidence.
See [the QA contract](qa/system.md) for the claims each check establishes.

Use an unused output path. `--require-committed` rejects dirty or untracked
source; a `worktree-...` label is not committed release proof. Preserve the
artifact outside the VM. Do not rebuild between smoke and activation, substitute
an artifact from a different revision, or bypass the pre-push/hosted gate.

Transfer the exact tested binary and reviewed `deploy/` scripts to a private
staging directory on the intended VM. Supply a complete private environment
only for first installation. From that directory, with operator-approved
administrative authority:

```sh
sudo bash deploy/install.sh --binary ./scry --release RELEASE --env /private/scry.env
sudo bash deploy/activate.sh --release RELEASE
```

For later releases omit `--env`: installation verifies immutable bytes and
stages only. It does not start, enable, or replace the active service. Environment
changes are a separate explicit operation, never a side effect of installing a
binary. IDs are immutable; do not reuse one for different bytes.

Activation serializes deployment, drains/stops the service, requires a completed
off-VM/readback-verified backup when live data exists, checks schema compatibility
read-only, switches the immutable release link, and checks both the process
executable and `/readyz`. Startup/rollback failure is not success. The previous
binary is restored only if it independently accepts the current schema. Never
restore an old database over acknowledged live writes as a release rollback.

### Schema 2 foundation release boundary

The MIS-59 candidate reads known complete schemas 1 and 2. Candidate
`check --db PATH` is a read-only migratability preflight: it validates the full
schema, integrity, and foreign keys, without creating a file, claiming jobs, or
upgrading v1. Startup `store.Open` atomically adds the schema-2 foundation tables
before traffic/claims. No historical v1 rows, snapshots, schedules, corrections,
operation receipts, or accounting are rewritten.

Keep the existing activation guard: old-binary check and verified remote
pre-release backup, then candidate read-only check, then switch/start. Do not
bypass it to migrate. After schema 2 commits, a schema-1 binary is deliberately
incompatible; the failure trap must not restart it against the upgraded DB.
There is no down-migration. A rollback is either a reviewed schema-2-compatible
binary or an explicitly authorized snapshot-recovery operation into an UNUSED
path with the snapshot's compatible binary. Preserve the upgraded database and
post-snapshot writes; quantify and obtain acknowledgment of any loss before
switching a restored instance into service. Never overwrite the live DB or allow
two writers.

For a rehearsal, preserve a populated v1 snapshot and its exact compatible
binary, run candidate `check` and verify the v1 schema/data remain unchanged,
start the candidate on an isolated copy, compare every historical export section,
and verify the old binary now refuses schema 2. Separately restore the preserved
snapshot into another unused path with its compatible binary and exercise its
service. This proves a recovery path, not production activation or fresh off-VM
backup.

`--allow-local-backup` exists only for explicitly synthetic rehearsals. It is
not a production bypass. Initial installation with no database has nothing to
back up; existing SQLite sidecars without the main database fail closed.

## Private configuration and domain changes

The service runs as `scry`, not root. Production configuration is
`/etc/scry/scry.env`, root-owned mode `0600`, parsed by systemd as an
`EnvironmentFile`. It is not executable shell: do not source it, place secrets
in argv, or copy production capabilities into a preview.

- `SCRY_BASE_URL` is the one approved HTTPS application origin, without a path.
- `SCRY_REDIRECT_HOSTS` lists explicit alternate hosts. Reads redirect to the
  canonical origin; mutations are rejected, never forwarded/replayed.
- `SCRY_OWNER_ID` is the exact stable exe owner identity, not an email or label.
- `SCRY_TRUSTED_PROXY_IPS` contains observed exact ingress peers, not public CIDRs
  or values inferred from `X-Forwarded-*`. Re-prove the peer before changing it.
- Preserve the application signing secret across normal releases. A fresh
  recovery environment needs its own appropriately scoped identity/configuration.

Register approved custom domains with exe.dev and follow its returned DNS
requirements. Keep the VM private. The application must accept the intended
Host and trusted ingress before DNS changes. Prove certificate, private login,
anonymous denial, canonical reads, and mutation rejection on aliases. Only then
retire old Caddy ingress blocks that specifically served Scry; leave unrelated
sites untouched. Never proxy the private app through the old public Worker or
accept forged owner headers as a shortcut.

Browser login/logout is owned by exe.dev. A separately scoped VM API token is
independent of browser login and is not revoked by logging out. Revoke temporary
tokens through their actual authority. Do not print their values in receipts.

## Generation and spending

The ignored workstation `.env` retains `OPENROUTER_API_KEY` with mode `0600`.
The Scry-only key has a $1/day UTC provider limit. The app receives it through
`scry-model`; no provider management key belongs in the VM. The application
uses a $1 rolling 24-hour allowance with $0.20 conservative reservations per
attempt. Provider and app windows differ; neither is an invented token price.

Review/grading never waits on a model. Saved capture creates a durable bounded
job. Leases and recorded operation identities prevent stale publication or
retries from manufacturing duplicate work. Unknown provider outcomes retain
their reservation; do not release spend or resend merely because a request
lost its response. Inspect the recorded job/error and reconcile explicitly.

Source-backed quizzes keep exact supplied evidence. Topic expansions are
labeled as model knowledge. Structural/provenance checks are not independent
fact-checking. Rejected candidates and incomplete coverage remain visible as
partial/failure, not invented completion. The private acceptance receipt
records representative live generation; it is not a longitudinal efficacy claim.

Too advanced foundation requests share the existing serial worker and spending
authority. Reuse is exact source revision plus quiz ID/version; saved material
does not become a scheduled quiz. Inspect `/foundations` for saved progress,
instruction, safe URL-only references, ordered diagrams and warm practice.
Producer/prompt attribution is retained; nothing is fetched from reference URLs.
Known-cost failed foundation work can retry within the SAME job's three attempts.
Expired/unknown foundation outcomes and restored jobs stay paused; prior
reservations remain accounted. There is no automated provider reconciliation or
UI action to discard an unknown charge. Keep ordinary review available, inspect
the provider outcome, and obtain separate operator authority before any manual
reconciliation. Source/quiz revision changes or archive prevent stale publication.

Warm target completion consumes practice availability for 24 hours from its
persisted rating-zero assisted completion, only while content and schedule
versions still match. This is not an FSRS change. Library shows availability
separately from scheduled due; queue counts and next availability use that same
fence across restarts. A deliberate reset or newer real review supersedes it.
Repeated help saves the latest acknowledged draft using the observed bridge
revision; stale new requests reload rather than overwriting saved progress.
Old bridge JSON withholds historical answer-bearing fields during a later cold
occurrence until exposure is acknowledged; immutable past records remain intact.

## Recovery policy and observation

The operator selected **daily and pre-release off-VM backups**, **30-day
new-app retention**, **RPO 24 hours**, and **RTO 60 minutes**. These are objectives,
not an SLA. Historical Worker/Postgres archives are outside this retention
policy and remain preserved separately.

The recovery manager attempts a backup at startup and then at the configured
interval. A consistent SQLite snapshot plus embedded assets/configuration
metadata is packaged into a unique completed archive. The app uploads the
exact bytes and requires an authenticated SHA-256-matching GET readback before
marking remote success. An interrupted or corrupt upload is not a backup.

`SCRY_BACKUP_INTERVAL=24h` and `SCRY_BACKUP_KEEP=30` govern current cadence and
local completed-snapshot count. Time-based remote retention is a separate R2
lifecycle policy; a count of local snapshots is not proof of 30-day off-VM
retention. The app/gateway cannot delete, overwrite, or change bucket policy.
Use only the dedicated new-app bucket for this lifecycle policy, never the
historical recovery buckets. Keep independently recoverable gateway capability,
compatible release artifact, and required configuration outside the VM.
The dedicated bucket's enabled `scry-go-recovery-retention` rule was read back
with an object age of 2,592,000 seconds (30 days). This is configured retention,
not an observation of a month of successful backups.

`GET /healthz` returns `ok`; `GET /readyz` returns `ready`. These are plaintext
liveness/readiness checks, not backup freshness. Settings exposes the last
completed backup and error/staleness state. Inspect the service journal without
printing content or credentials. No current Go external mail/availability
monitor is claimed; the retired Worker monitor is disabled, not repointed to
an incompatible response format. Native backup monitoring remains separate.

Manual app commands are `scry version`, `check --db PATH`, `export --db PATH`,
and `backup --db PATH --require-remote`. Run live maintenance with the service's
private environment and unprivileged account through systemd, not a shell-sourced
secret file. Export contains private learning content; protect it like the DB.

## Independent restore

Retrieve a completed `.scry-backup.zip` from private off-VM recovery using
independently retained authority, and verify its SHA-256. Keep a compatible
reviewed binary and configuration independently of the lost VM. On an isolated
VM, stage that binary and a fresh private environment, with no production model
or backup integrations attached. Stop the application before restoring its
canonical database path.

```sh
sudo bash deploy/install.sh --binary ./scry --release RELEASE --env /private/recovery.env
sudo bash deploy/restore.sh --release RELEASE --snapshot /private/recovery.scry-backup.zip --destination /var/lib/scry/scry.sqlite
```

The destination must be unused, including SQLite sidecars. Restore checks the
archive checksum, schema, and integrity, publishes without replacing an existing
path, and pauses all nonterminal jobs. It does not start a service or replay
uncertain paid work. Compare recovered export/acknowledged history, inspect
those jobs, and reconcile only after checking provider outcomes.

Schema-2 exports/backups include foundation request-to-target bindings, bundle
and material/unit versions, coverage roles/provenance, bridge revision/progress,
saved target drafts, and immutable read/practice/return observations. Compare
these sections along with ALL pre-existing history and spend, not just quiz
counts. The archive contains the whole SQLite state; no remote reference content
or external diagram assets exist. A schema-2 candidate may restore a schema-1
archive into an unused path and migrate it before publication; use the retained
schema-1 binary instead when rehearsing recovery to the pre-upgrade state.

Activate separately when ready. Synthetic rehearsals without remote backup
capability may explicitly use `--allow-local-backup`; real recovery must restore
the approved production recovery capability before normal operation. Prove the
actual unprivileged process, private HTTPS, export equality, and UI; record
elapsed time and omitted steps. Do not clone active scheduler ownership. Stop
and disable the rehearsal service afterwards.

The September 9 drill restored the independently retrieved R2 archive on
`scry-dev`, activated the unprivileged service, matched the private HTTPS export,
and rendered the restored phone UI. Restore/activation/private HTTPS took about
117 seconds on the already-provisioned VM. VM provisioning and DNS restoration
were not timed. The rehearsal service was then stopped and disabled.

## Historical stores: preserve, do not reactivate

Both old Workers were paused using guarded runtime control with unchanged data
fingerprints. Final production/staging R2 backups were retrieved and verified;
cron triggers were removed. The old `production-health.yml` workflow is disabled
and `SCRY_MONITOR_ENABLED=false`. No old learning data was imported or deleted.
Native `scry.service` on `public-apps` remains disabled; its separate off-host
backup timer remains active. Do not restart any old writer as rollback.

Historical Worker names are `scry` and `scry-staging`; historical buckets are
`scry-recovery` and `scry-staging-recovery`. These are not `scry-go-backups` or
`scry-go-recovery`. Keep identifiers, schemas, credentials, and recovery tools
separate; a Go restore never accepts an old export by renaming its file.

The reviewed old source is Git revision
`37a5a3d3089365c6a1144256e8a8eb01f0096c84`. Its old runbook and full application
remain in Git history and the privately retained source archive. Current
`bin/`, `etc/`, `scripts/scry-cloudflare-data`, and their explicit recovery
contracts retain old recovery capability, not current product/deployment paths.
Private cutover archives and compatible binaries live under the operator's
`~/.local/share/scry/ops/`; keep independent copies where recovery requires them.

## Evidence

- [Earlier private Go acceptance](qa/personal-go-acceptance-20260909.json): live
  generation, trusted touch, interrupted access/response recovery, data restore.
- [September 9–10 cutover acceptance](qa/personal-go-cutover-20260910.json):
  criterion-level browser corrections, generation, canonical activation, and
  explicit remaining identity-switch evidence.
- Current exported `proof.json`: source-bound exact-binary gate; synthetic and
  without production model/recovery capability.
- Dated September 8 Rust receipts remain historical. Do not reinterpret them
  as Go, new-domain, or current-monitor evidence.

Phone approval establishes the operator's initial experience acceptance. It
does not establish sustained use, delayed recall, availability, or unmeasured
DNS/provisioning recovery time.

Global exe sign-out was exercised without an independent VM token. Private
content stayed absent on history return, but that return encountered an upstream
authentication redirect loop; a fresh canonical navigation reached sign-in.
Switching to a different exe account remains unverified. Keep S09.2/MIS-48 open
for that observation rather than broadening trust or assuming logout covers it.
Operator material-usefulness acceptance also remains open under S04.2/AI1.
The receipt records candidate performance/accessibility budgets separately;
initial phone-flow approval is not a measured p95 or general AI-quality claim.
