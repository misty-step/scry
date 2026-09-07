# Scry production runbook — Cloudflare

## Destination versus current live truth

The production destination is `memory-engine-cloudflare`: Rust Wasm on Cloudflare
Workers, one SQLite-backed `Scry` Durable Object as the invite-beta transaction
owner, alarms for leased generation/reminders, private R2 recovery, and Resend
over Worker Fetch. Pure learning policy and the shared Rust renderer remain
inside the existing Rust capability system. There is no production D1, KV
consistency cache, Queue, container, external Postgres, or native runtime in
this destination architecture.

**A repository change is not a completed cutover.** The approved interim origin
is `https://scry.misty-step.workers.dev`; registrar/custom-domain access is not
a prerequisite. The native DigitalOcean application and its host-local
PostgreSQL database (reported schema 8) remain authoritative until Main records
the source writer barrier, final import/readback, recovery, activation, and
public proof. Old references to Neon are not a direction to switch back.
Keep the old host, ingress, credentials, local dumps, and backup work intact
until Main explicitly changes or retires them. Isolated staging receipts exist
for `https://scry-staging.misty-step.workers.dev` (2026-09-07); they do not
move production. Production commands below remain a planned sequence until
Main records the source barrier, final import, recovery, activation, and
public proof. A reachable Worker hostname alone is not cutover.

| Surface | Staging | Production |
|---|---|---|
| Account | Misty Step, `b069014f6a46558ea9146fb6c4ff8f6c` | same account |
| Worker | `scry-staging` | `scry` |
| Approved interim origin | `https://scry-staging.misty-step.workers.dev` | `https://scry.misty-step.workers.dev` |
| `SCRY` binding | private SQLite class `Scry`, staging namespace | private SQLite class `Scry`, production namespace |
| `RECOVERY` binding | private `scry-staging-recovery` R2 bucket | private `scry-recovery` R2 bucket |
| Mail transport | Resend HTTPS API with approved staging identities | Resend HTTPS API with approved production sender |

`wrangler.jsonc` contains explicit staging/production bindings and the first
class migration `{tag: "v1", new_sqlite_classes: ["Scry"]}`. It intentionally has
**no custom-domain routes**. Never add `scry.study`/`www.scry.study` routes before
Main's authoritative DNS proof and complete migration plan. The release guard
also rejects custom routes; the eventual DNS cutover is a separate reviewed
source/configuration change, not a hidden CLI override.

Every new actor starts paused: `learner_traffic_enabled INTEGER DEFAULT 0` in
object-local recovery control is excluded from the authoritative fingerprint.
`GET /internal/runtime` reports `{maintenance: bool}`. Authenticated
`POST /internal/runtime` accepts `{enabled: bool, expectedFingerprint: string}`
and atomically changes primary traffic only when the current fingerprint
matches and no application/background/import operation is still in flight.
Busy operations or stale fingerprints return 409; do not retry blindly.
`/healthz`, static assets, and authenticated internal recovery/schema/runtime
routes remain usable while paused. `/readyz` and learner routes return 503;
jobs, reminders, and automatic recovery alarms stay suppressed. `restore-*`
objects remain paused and never become a second learner-writing instance.
Activation wakes background work only after the transaction commits.
This is not `MEMORY_ENGINE_MAINTENANCE` environment control: never toggle a
Worker secret, change configuration, or deploy another version to change traffic.

## One executable build and release path

All supported build/dev/release operations go through
`scripts/scry-cloudflare`, exposed by the Bun commands below. Direct
`wrangler deploy`, dashboard edits, native host installers, and implicit
"previous version" rollback are not release paths. A merge does not deploy.

Prerequisites: Rust 1.94.0, Node 22+, Python 3.11+, Bun, and Git. Dagger plus a
container engine are required for the ship gate. Operators additionally need
`gh` access to this repository's protection/check-run metadata and authenticated
Wrangler access to the specified Cloudflare account. Credentials stay in
private environment files, the operator's authenticated CLI, or secret stores;
never put values in command arguments, logs, or checked-in configuration.

```sh
bun run worker:tools
bun run worker:build --out target/cloudflare/candidate
bun run worker:smoke --artifact target/cloudflare/candidate \
  --receipt target/cloudflare/candidate-workerd-proof.json
bun run dev:isolated --artifact target/cloudflare/candidate --port 8787
```

Tool installation has one fixed mode: `npm ci` from `package-lock.json`, the
Wasm target for Rust 1.94.0, `cargo install --locked` of **worker-build 0.8.5**
and **wasm-bindgen-cli 0.2.125**. Wrangler is **4.129.0** and esbuild **0.28.1**.
The worker-build release supports wasm-bindgen 0.2.122–0.2.125; this repository
requires exactly 0.2.125 in Cargo.lock and the CLI, not an independently updated
bindgen binary. Its optimizer is the worker-build-pinned wasm-opt 130.

Compilation snapshots exact source bytes into a separate directory, then runs
`worker-build` with `--release --locked --no-default-features` for
`wasm32-unknown-unknown`. Only that target receives
`--cfg getrandom_backend="wasm_js"`; inherited Rust/compiler/provider overrides
are not carried into the build. esbuild and wasm-bindgen binaries are explicitly
selected and version/hash recorded. `Dockerfile` is the same pinned build/QA
substrate with an artifact-only final stage; it is not a deployed server image.

The immutable artifact directory contains:

- `worker/index.js` and `worker/index_bg.wasm` — the actual deployed module bytes.
- `wrangler.json` — self-contained no-bundle configuration for both environments.
- `source.json` — full source path/content/mode hashes.
- `manifest.json` — revision, source SHA-256, toolchain and tool hashes, target,
  dependency flags, module/configuration checksums, and ordered SQLite migration
  version/name/content hashes. The manifest's SHA-256 is the **bundle hash**.

No artifact or receipt is overwritten. Select a new output path for each
candidate. Local dirty-source builds are useful for development, but cannot be
released: every remote operation rechecks the artifact's complete source map
against its Git revision, requires that revision on protected `origin/master`,
requires administrator-enforced `ci` protection and a successful current GitHub
Actions `ci` check for that exact revision, and requires a matching real
workerd proof (traffic operations retain the current verified receipt's proof
binding). There is no stub, dirty-release, skipped-guard, or arbitrary native
install mode. Run old-artifact rollback tooling from that artifact's recorded
reviewed source revision; the script checks its own source hash too.

### Isolated local workerd

`dev:isolated` starts the exact verified bundle, not `cargo run`. A unique local
Worker name, loopback port, inspector port, private home, SQLite/R2 state, and
private runtime config separate it from every other dev session. `--local`
disables remote bindings. Model/Cloudflare credentials are not inherited.
`--state NEW_DIRECTORY` retains only that explicit state after stopping; without
it, the unique temporary state is removed. Ctrl-C or termination stops the owned
process group. A later run uses new state; nothing resets a deployed object.
First launch reads the private actor's fingerprint and performs authenticated
`POST /internal/runtime` with `enabled: true` and that exact
`expectedFingerprint`, then observes `/readyz`. Restarts read the persisted
runtime state and require it to remain active without another mutation.

The local browser accepts `dev@example.test` (or explicit `--email`). Local auth
uses exactly `MEMORY_ENGINE_ENVIRONMENT=development`,
`MEMORY_ENGINE_MAIL_MODE=local-outbox`, and
`MEMORY_ENGINE_AUTH_EXPOSE_DEBUG_LINKS=true`. `/app/account` shows the local
magic link. Admin-only `GET /internal/mail/outbox` returns staged messages with
`mode: "local-outbox"` and `sent: false`; the private runtime configuration has
the ephemeral admin token. This is **not sent-mail or inbox-delivery proof**.
The outbox/debug path is unavailable in staging/production mail mode.

The finite smoke runs actual public assets (including `/static/app.js`) and
readiness, anonymous rejection, allowlisted service-session issuance,
alarm-driven LocalOnly structured generation, explicit draft keep, two real
accounts' isolation, durable review resume, and assisted grading across restart.
Idempotent replay must preserve attempts, schedules, exposure, and receipt hashes.
The smoke finishes with the admin schema fingerprint.
A failed smoke retains a private workerd log beside
the requested receipt; it does not manufacture a passing proof.

### Gates

```sh
bun run ci:local
bun run ci:full
# Export Dagger's exact bundle and its workerd proof when needed:
dagger call worker --source=. --git-sha="$(git rev-parse HEAD)" \
  export --path=target/cloudflare/dagger-proof
```

The fast gate retains the browser contract, consequential private-env/old-host
recovery checks, Rust formatting/tests/Clippy/rustdoc, and the file action-latency
budget, then invokes the real Wasm/workerd gate. Dagger repeats those boundaries,
adds Postgres 16 with `MEMORY_ENGINE_POSTGRES_TEST_URL` for native reference
contracts, retains Postgres latency receipts, and runs Gitleaks. Neither gate
receives deployment credentials or substitutes a synthetic HTTP server.

The native API remains a compatibility/test surface. Passing native Cargo tests
alone does not prove the Worker. Passing workerd alone does not prove browser
latency, model quality, mail delivery, migration, or production recovery.

## Account provisioning — operator only

These are required operations, **not a record that they have been executed**:

1. Confirm the authenticated account is the Misty Step account above. Provision
   Workers/SQLite Durable Object and R2 access, appropriate billing/limits, and
   least-privilege deployment authority. Do not copy a token into the repo.
2. Create the two named private R2 buckets before release. Do not enable public
   bucket access, attach a public domain, or share staging/production storage.

   ```sh
   npx --no-install wrangler r2 bucket create scry-staging-recovery
   npx --no-install wrangler r2 bucket create scry-recovery
   npx --no-install wrangler r2 bucket info scry-staging-recovery --json
   npx --no-install wrangler r2 bucket info scry-recovery --json
   ```

3. Retain the operator-approved **Resend** sender/domain and its existing verified
   DNS configuration. Confirm the sender and API key may send to the approved
   recipient; do not change registrar authority or mail DNS as a hosting side
   effect. There is no `EMAIL` binding or Cloudflare Email Service onboarding
   dependency. A successful Resend API response records **provider acceptance
   only**, not mailbox receipt, Inbox/Spam placement, or authentication results.
4. Prepare separate owner-owned mode-0600 literal `KEY=value` secret files for
   staging and production. No shell expansion is supported. Bootstrap passes
   that private file to Wrangler's documented `--secrets-file`. Use the same
   environment's admin token in the separate `--admin-env` file. Preserve
   imported auth/session/reminder signing identities; do not rotate them merely
   because hosting changed.

| Secret/configuration | Worker contract |
|---|---|
| `MEMORY_ENGINE_ADMIN_TOKEN` | operator service-session, waitlist, migration/recovery authorization |
| `MEMORY_ENGINE_AUTH_ALLOWED_EMAILS` | explicit invite allowlist; keep addresses out of public receipts |
| `MEMORY_ENGINE_RETURN_UNSUBSCRIBE_SECRET` | stable HMAC secret for existing signed unsubscribe continuity |
| `MEMORY_ENGINE_MAIL_FROM` | replyable approved sender on the existing Resend-verified sending domain |
| `RESEND_API_KEY` | environment's Resend send authority; required for deployed mail |
| `OPENROUTER_API_KEY` | model-backed generation through Worker Fetch, never a local smoke requirement |
| `MEMORY_ENGINE_ENVIRONMENT` | checked-in `staging` or `production`; both enforce production auth/mail safety |
| `MEMORY_ENGINE_MAIL_MODE` | checked-in `resend`; only other mode is explicit local/development/test `local-outbox` |
| `MEMORY_ENGINE_PUBLIC_BASE_URL` | checked-in environment-specific workers.dev origin until reviewed DNS cutover |
| `MEMORY_ENGINE_GENERATION_MODEL` | optional reviewed override; provider default remains `google/gemini-3.7-flash` |

`MEMORY_ENGINE_POSTGRES_URL`, filesystem store/outbox paths, mailer commands,
native ports, systemd, and Caddy are not Worker runtime inputs. Keep old-host
secret/configuration files intact for the authoritative native service.
Bootstrap supplies the initial secrets in its one exact-byte deployment.
Do not update secrets between bootstrap and activation: traffic control uses
application state so the verified immutable version remains unchanged.

## Bootstrap, activate, upload, promote, pause, rollback

Use new private receipt filenames and keep the full artifact with its proof.
The following shell variables name **paths**, not credentials:

```sh
artifact=target/cloudflare/candidate
proof=target/cloudflare/candidate-workerd-proof.json
staging_admin=/private/scry-staging-admin.env
production_admin=/private/scry-production-admin.env
```

### First class/Worker creation only

Cloudflare cannot apply a Durable Object class lifecycle migration with
`versions upload`; initial `v1` requires `deploy`. The repo's explicit bootstrap
allows this only when Wrangler positively reports the missing Worker error
**10007**. Authentication failures, timeouts, existing scripts, and an unknown
status are not evidence of absence. It still enforces reviewed source and the
exact workerd proof, and it never rebuilds.

```sh
bun run release:bootstrap --environment staging --artifact "$artifact" \
  --verification "$proof" --admin-env "$staging_admin" \
  --secrets-file /private/scry-staging-secrets.env \
  --receipt target/cloudflare/staging-initial.json

# Rehearse migration/recovery separately; only approved nonproduction learners
# use staging app. A restore-* object is never activated.
staging_fingerprint="$(bun run ops:data fingerprint \
  --base https://scry-staging.misty-step.workers.dev \
  --admin-env "$staging_admin" --target app | jq -er '.sha256')"
bun run release:traffic --environment staging --artifact "$artifact" \
  --current target/cloudflare/staging-initial.json --admin-env "$staging_admin" \
  --expected-fingerprint "$staging_fingerprint" --enable \
  --receipt target/cloudflare/staging-activated.json

# Only after activated staging's migration/recovery/product evidence is accepted:
bun run release:bootstrap --environment production --artifact "$artifact" \
  --verification "$proof" --admin-env "$production_admin" \
  --secrets-file /private/scry-production-secrets.env \
  --staging-receipt target/cloudflare/staging-activated.json \
  --receipt target/cloudflare/production-initial.json
```

Bootstrap requires the new primary actor to report `maintenance: true`, public
health/static assets to work, authenticated SQLite schema access to succeed,
and readiness/learner requests to return 503. Its verified receipt is explicitly
paused and has `public_smoke: "not-run-paused"`; it cannot authorize production.
`staging-activated.json` must bind `maintenance: false`, `readiness: "ready"`,
and `public_smoke: "passed"` to the exact bundle and still-active staging
version before production bootstrap/upload/promotion accepts it.
Production bootstrap still leaves the destination paused for the final import
and recovery drill. It never stops native writers, migrates data, changes DNS,
or authorizes production activation.

### Explicit primary traffic control

Only `release:traffic` changes the primary actor's learner/background activity.
It requires the exact artifact and current verified deployment/traffic receipt,
private admin env, reviewed authoritative fingerprint, explicit `--enable` or
`--disable`, and a new receipt path. Source/CI/review protection remains enforced.
Intent is saved before the application mutation; the result records the
fingerprint, prior/final lifecycle state, schema proof, and unchanged immutable
Worker version. No upload, deployment, compilation, or secret write occurs.
If already in the requested state, it verifies without repeating the mutation.

**Production enable is Main-only**, after the independently executed stopped
source writer/sender barrier, final primary import, independent readback, and
accepted recovery proof below. `--source-quiesced` is an explicit declaration,
not a command that stops anything. The root additionally requires verified
PostgreSQL primary-import provenance and atomically matches the supplied state
fingerprint. Main must review full private import/restore receipts; a flag or
matching hash is not a substitute for that evidence.

```sh
# This receipt is captured by the final import procedure below, not invented.
production_fingerprint="$(jq -er \
  'select(.schema == "memory_engine.cloudflare_import_receipt.v1" and .verified == true) | .fingerprint.sha256' \
  /private/scry-final-import-receipt.json)"
bun run release:traffic --environment production --artifact "$artifact" \
  --current target/cloudflare/production-initial.json --admin-env "$production_admin" \
  --expected-fingerprint "$production_fingerprint" --source-quiesced --enable \
  --receipt target/cloudflare/production-activated.json

# Pause later using a freshly reviewed current-state fingerprint, not the old
# import hash after legitimate learner writes. Source quiescence is not needed
# to pause; it must still hold when production is re-enabled.
pause_fingerprint="$(bun run ops:data fingerprint \
  --base https://scry.misty-step.workers.dev \
  --admin-env "$production_admin" --target app | jq -er '.sha256')"
bun run release:traffic --environment production --artifact "$artifact" \
  --current target/cloudflare/production-activated.json --admin-env "$production_admin" \
  --expected-fingerprint "$pause_fingerprint" --disable \
  --receipt target/cloudflare/production-paused.json
```

The pause receipt verifies 503 readiness/learner fences and the same
authoritative fingerprint after the transaction. Active background work may
legitimately change the fingerprint after activation. A 409 or uncertain HTTP
result leaves a `failed-requires-inspection` receipt; read current lifecycle,
state, and immutable version before a new explicit attempt. The command never
blindly retries. Reactivation uses the pause receipt as `--current`, a freshly
reviewed fingerprint, `--enable`, and production's still-true
`--source-quiesced` declaration.

### Ordinary code changes

Upload identical verified bytes to staging, then explicitly promote that
returned immutable version. `--current` must identify the currently verified
100% deployment; a concurrent/manual change causes refusal.

```sh
bun run release:upload --environment staging --artifact "$artifact" \
  --verification "$proof" --admin-env "$staging_admin" \
  --current target/cloudflare/staging-current.json \
  --receipt target/cloudflare/staging-upload.json

bun run release:promote --environment staging --artifact "$artifact" \
  --verification "$proof" --admin-env "$staging_admin" \
  --current target/cloudflare/staging-current.json \
  --version-receipt target/cloudflare/staging-upload.json \
  --receipt target/cloudflare/staging-promoted.json

bun run release:upload --environment production --artifact "$artifact" \
  --verification "$proof" --admin-env "$production_admin" \
  --current target/cloudflare/production-current.json \
  --staging-receipt target/cloudflare/staging-promoted.json \
  --receipt target/cloudflare/production-upload.json

bun run release:promote --environment production --artifact "$artifact" \
  --verification "$proof" --admin-env "$production_admin" \
  --current target/cloudflare/production-current.json \
  --version-receipt target/cloudflare/production-upload.json \
  --staging-receipt target/cloudflare/staging-promoted.json \
  --receipt target/cloudflare/production-promoted.json
```

`*-current.json` above names a retained deployment/traffic receipt, not a
mutable pointer. A `failed-requires-inspection` code-deployment receipt may be
used explicitly after inspection: upload and promotion still require the
recorded version to be active, its bundle/schema identity to match, and the
full live schema/runtime preflight to succeed before any version mutation.
The failed receipt alone never proves recovery. Use the actual filename; no
`latest` alias or symlink is trusted. The new verified receipt becomes the next
operation's explicit `--current`.

Internally, upload uses Wrangler `versions upload --no-bundle`; promotion uses
`versions deploy VERSION_ID@100 --yes`. The bundle hash is written into the
Cloudflare version message and checked through `versions view --json`.
`deployments status --json` verifies the active version. Upload checks that it
did not alter the active deployment. Production requires a previously verified
staging receipt for **the same bundle hash and version**, with staging activated
and `public_smoke` passed; it does not rebuild "equivalent" Wasm for production.
Code releases do not mutate the durable traffic flag. Promotion checks it before
and after deployment; rollback reads and reports the recovered state without
requiring the outgoing application to respond.

Every mutation writes an intent journal first. Code releases retain private
Wrangler structured output; traffic operations retain only safe application
intent/result metadata. Completed receipts record environment/account, reviewed
revision/source/bundle/schema hashes, CI check-run id, workerd proof hash,
immutable version, previous version, and deployment id when returned. Failures
record `failed-requires-inspection` and any returned version rather than
silently retrying activation or fabricating success. Inspect a failed operation
and live versions before taking another action.
Release and traffic commands hold one private operator-local lock across both
environments and all worktrees; promotion/rollback also recheck the active
version immediately before switching traffic. Cloudflare's deployment API does
not offer compare-and-swap. Keep one authoritative operator host/user and do not
deploy concurrently from another host or the dashboard: the local lock is not
a distributed control-plane lock.
The operator HTTP clients identify themselves as `scry-cloudflare` and
`scry-cloudflare-data`. Cloudflare ingress can reject Python's generic default
user-agent with 403/1010 before a request reaches the application; do not
misdiagnose that response as an application credential failure or weaken auth.

A deployment receipt's `status: verified` proves the active 100% version,
health/static assets, authenticated SQLite schema, and its explicit
`runtime_proof`. A paused receipt proves the learner/readiness fence, not
public learner readiness. An active receipt additionally proves `public_smoke`
(public app/readiness and auth rejection). Neither declares that live migration,
restored history, inbox placement, paid generation, UI timing, or cutover passed.

### Explicit code rollback

Keep the old bundle, its workerd proof, and its previously verified deployed
receipt. From its recorded reviewed source/tooling revision:

```sh
bun run release:rollback --environment production \
  --artifact target/cloudflare/previous-bundle \
  --verification target/cloudflare/previous-workerd-proof.json \
  --admin-env "$production_admin" \
  --current target/cloudflare/production-promoted.json \
  --version-receipt target/cloudflare/previous-production-verified.json \
  --receipt target/cloudflare/production-rollback.json
```

Rollback names an exact previously verified version and uses the same 100%
version deployment operation; never an inferred predecessor. `--current` may
be the inspected `.failed.json` receipt from a failed deployment, provided its
version/hash still matches the live Cloudflare control plane. The outgoing app
does not have to pass the schema or public probes that rollback is repairing.
Current and target receipts must have identical ordered SQLite migration-content
hashes. The recovered version must then pass live schema and runtime verification;
its receipt reports whether learner traffic is active or paused. Unknown/newer,
gapped, or changed schema definitions fail closed. Class lifecycle changes are
also refused by this release path. There is no automatic SQL downgrade.

Cloudflare rollback changes code, **not bound resource data**. A class lifecycle
migration blocks rollback to earlier classes, and removed bindings/buckets can
make old versions unusable. Schema-changing recovery therefore requires the
separate-empty-object restore/data procedure below and a separately reviewed
traffic cutover. Never restore a database underneath live writers or treat an
old native schema-8 binary as rollback for a migrated SQLite object.

## Full data migration and R2 recovery

The owner is `scripts/scry-cloudflare-data` and the Worker recovery module.
Every API command explicitly selects an origin, a private admin env file, and
`--target app` or a separate `restore-[a-z0-9-]{8,64}` object. The Worker overwrites
trusted internal object/identity headers. There is no arbitrary SQL endpoint.
HTTP credentials never follow redirects; HTTPS is required except loopback.
Root routing must keep `restore-*` targets administrator-only and suppress their
generation/reminder/recovery alarms. Only `app` may publish backups or perform
retention; recovery does not grant a second live learner-writing instance.

The portable bundle schema is `memory_engine.cloudflare_import.v1`. Current
bounds are 16 MiB/bundle, 1 MiB/row, and 100,000 rows. PostgreSQL export reads
through existing host `psql`/SSH using a **repeatable-read READ ONLY transaction**,
with catalog/schema checks and no remote file writes or native store startup.
It accepts the known complete schema-8 ledger (and explicitly recognized
schema-9 exposures when present), fails on unknown/missing tables, columns,
types/nullability, keys, RLS, sequences, or accounts, and never invokes
`PostgresStudyStore::connect`.
This beta migration requires exactly one explicitly known source account.

All raw accounts, auth challenges, browser/API session hashes, CSRF hashes,
waitlist/audit rows, reminder preferences/claims, source/reference bodies,
generation runs/drafts/jobs/attempts/reservations, notes, review units/schedules,
attempts/idempotency receipts, feedback, remediation, and available exposure
history are part of the export. JSON payloads remain JSON text, preserving
source content and integer/timestamp/hash/null values. Import is transactional
and empty-target-only; known format conversion and new target bookkeeping are
explicit, with source hashes/counts and destination fingerprint evidence.
No historical learning/exposure evidence is invented.

Schema 8 exports all 21 application tables, eight migration-ledger rows, and both
sequence high-water states; recognized schema 9 also exports actual exposures.
The one known consumed-hold key `idempotencyKey` is normalized to
`idempotency_key` only after exact raw-source readback, with original private
bytes and before/after hashes retained in provenance. PostgreSQL booleans map
to SQLite 0/1. No exposure is backfilled into schema 8's new empty exposure table.
Imported pending reminder deliveries without a corresponding receipt remain
unknown rather than being blindly resent. Full SQLite export includes all
28 application/provenance tables and original ledger/sequence/recovery control
state; source control/lease state is kept as provenance, not activated on a
restored object. Platform-internal SQL-inaccessible `__cf_kv` is not application
state. These conversion and suppression invariants still require Main's real
catalog, data/session, workerd, and restored-object proof.

### Staging rehearsal and final export

Use a private directory with new mode-0600 bundle filenames. The expected
account id is supplied from operator knowledge, not inferred from a newly
created test account. The native env file path below remains on the old host;
the command does not copy or print its credentials.
`--sudo --database-user scry` reads the root-owned environment first, then drops
the exporter to the existing `scry` OS/database user for Unix-socket peer auth.
It does not grant a new role, alter authentication, or write host files.

```sh
bun run ops:data pg-export --ssh public-apps --sudo --database-user scry \
  --env-file /etc/public-apps/scry.env --schema public \
  --expected-account-id "$expected_account_id" \
  --output /private/scry-precutover.json

bun run ops:data inspect --input /private/scry-precutover.json \
  --expected-account-id "$expected_account_id"

# Rehearsal into a separate empty object; it does not claim a stopped source:
bun run ops:data import --base https://scry-staging.misty-step.workers.dev \
  --admin-env "$staging_admin" --target restore-rehearsal-20260906 \
  --input /private/scry-precutover.json --expected-account-id "$expected_account_id"
```

For final primary import, **Main alone** first stops all old writers and
senders: public learner mutations, native generation workers, reminder
schedulers, machine writers, and any background job capable of changing state.
The destination must remain paused; primary import rejects an active actor.
Capture a fresh final read-only export under that barrier. `--source-quiesced`
is a required assertion of the independently executed barrier, **not a command
that stops anything**. Never assert it just to bypass the refusal.

```sh
# Capture the actual JSON import result in a new private file; never overwrite.
(
  umask 077
  set -C
  bun run ops:data import --base https://scry.misty-step.workers.dev \
    --admin-env "$production_admin" --target app --source-quiesced \
    --input /private/scry-final-quiesced.json --expected-account-id "$expected_account_id" \
    > /private/scry-final-import-receipt.json
)

bun run ops:data fingerprint --base https://scry.misty-step.workers.dev \
  --admin-env "$production_admin" --target app
```

Import requires the reviewed source checksum and exact expected account
fingerprint. `app` must be empty; there is no destructive replace flag. Preserve
the import receipt, every table's count/hash comparison, imported session-record
proof, and full history/active-review continuity evidence. Compare the independent
fingerprint readback with the import receipt before recovery and activation.
A successful transport or matching account count alone is insufficient.

### Back up, retrieve, restore into a separate object

When active, the main object's alarm performs daily R2 backup and retention
(30 days, keep at least 7 complete backups per source). While paused, use the
authenticated manual backup/retrieve/restore commands for pre-activation proof.
R2 uses private complete manifests and
256 KiB checksummed chunks. The complete marker is written only after all
chunks; partial uploads are not considered recoverable backups. Inventory is
bounded and fails closed rather than silently truncating. Retention removes
complete markers before chunks and gives incomplete chunks a seven-day grace.

```sh
bun run ops:data backup --base https://scry-staging.misty-step.workers.dev \
  --admin-env "$staging_admin" --target app \
  --receipt /private/scry-staging-backup.json

bun run ops:data retrieve --base https://scry-staging.misty-step.workers.dev \
  --admin-env "$staging_admin" --target app \
  --receipt /private/scry-staging-backup.json \
  --output /private/scry-staging-retrieved.json

bun run ops:data restore --base https://scry-staging.misty-step.workers.dev \
  --admin-env "$staging_admin" --target restore-drill-20260906 \
  --receipt /private/scry-staging-backup.json

bun run ops:data fingerprint --base https://scry-staging.misty-step.workers.dev \
  --admin-env "$staging_admin" --target restore-drill-20260906
```

Restore verifies manifest, every chunk, complete bundle, account identity, and
content fingerprint, commits to a **separate empty** `restore-*` object, then
performs an independent request-boundary fingerprint readback. It never
replaces `app`, automatically selects the restored object for learner traffic,
or shares buckets between environments. The `restore-drill-20260906` name above
is a command template: each drill needs a new empty `restore-*` object, never
reuse of an occupied restore. The executed 2026-09-07 staging drill used
`restore-qa-20260907` and must not be reused. For production, run the same
commands with the production origin/admin file and a new production
receipt/restore name. Use the `export` command for an additional private
SQLite portable bundle; `list` exposes complete backup receipts without
content. Manual retention is explicitly destructive and requires
`--target app --days 30 --keep 7 --confirm`.

Do not configure a coarse bucket lifecycle rule that can delete required
chunks before their complete manifest. Durable Object PITR is an additional
Cloudflare recovery option; it is not a substitute for the portable R2 restore
drill. Record the actual platform recovery point and operator procedure before
claiming PITR proof. Staging R2 backup/retrieve/restore into separate object
`restore-qa-20260907` was executed 2026-09-07 against live QA state (1 account,
51 rows; backup/retrieve/restore fingerprints `3da26c5b9ca7e51305e79b3779f48570fec07a89bafe9950fa3fab93e1d5f982`).
It did not replace `app`. No production restore, PITR action, or production
data drill has been executed.

## Auth, learner, generation, and delivery proof

Human auth remains invite-gated magic links with independent sliding browser
sessions and CSRF-checked mutations. Machine faces use
`POST /v1/service-sessions` with `x-admin-token` and JSON `{ "email": ... }`,
returning `{accountId, sessionToken}` to a private consuming-client credential
file. The static allowlist applies even to an admin-issued session. Never read
mail archives to authenticate machines. Account-scoped DELETE
`/v1/accounts/{account_id}/service-sessions/current` and `/all` retain their
explicit revocation semantics.
Browser-session, challenge, and CSRF records remain preserved, but browser
cookies are host-scoped and cannot move from `scry.study` to workers.dev.
Users must perform a fresh magic-link sign-in on the new origin. Machine
clients may retain their imported bearer token while explicitly changing their
base URL. Do not weaken cookie/CSRF boundaries to simulate seamless host migration.

Waitlist `/internal/waitlist`, `/export`, `/invite`, and `/delete` remain
admin-only. The public Worker strips caller-supplied trusted headers and derives
trusted client identity from the Cloudflare request. Caller-controlled forwarded
IP headers must never choose an auth quota. An unavailable/unknown identity
still fails closed rather than disabling rate limits.

All five faces retain `/v1` wire identifiers and learning behavior. Preferred
generation is `POST /v1/accounts/{account_id}/sources/{source_id}/generation-jobs`,
then account-scoped `GET /v1/accounts/{account_id}/generation-jobs/{job_id}` until
terminal status. Coalesced admission returns the existing exact job; generated
drafts do not schedule reviews until explicit learner keep/edit. The synchronous
`/sources/{source_id}/generate` route remains a compatibility capability; do not
copy the old host-specific claim that every production call returns 409.

Generation uses Worker Fetch against OpenRouter with the shared Rust trust
runner, not a fake provider or an unproved Workers AI substitution. Current
provider controls include source 64 KiB, response 1 MiB, 60-second requests,
180-second leases, three attempts, per-account/global active limits 8/64 and
running limits 1/4. Admission reserves $0.10 for each potentially dispatched
unknown-cost call (initial+repair $0.20), with rolling 24-hour account/global
limits $1/$10. Actual over-allowance cost is charged and blocks further starts;
unknown costs keep their reservation, never a fictitious zero. LocalOnly
sources must not reach a model. Cached Notes and manual learner-requested
Bridge remain explicit, with learner approval before scheduling.

Before public cutover, retain separate actual evidence for:

1. Imported session/challenge/CSRF records and machine sessions, fresh workers.dev
   browser sign-in, allowlist recheck, one-use magic links, CSRF rejection,
   logout-current/all, and signed unsubscribe.
2. Real account/source/history/graded-hold continuity and account isolation.
3. Browser Capture → waiting → Library draft decisions → Study → reveal/grade →
   Next, with exact-job terminal reconciliation that does not navigate away
   from editable Library. Verify the actual mobile surface, not just HTML text.
4. Real queued generation, lease/replay behavior, provider usage/cost accounting,
   and the current generation quality acceptance bar.
5. Resend provider acceptance recorded separately from Inbox/Spam placement and
   SPF/DKIM/DMARC results, a successfully consumed fresh magic link, replay
   rejection, and an actual due-count reminder to an approved account with
   genuinely due material. Resend acceptance, outbox rows, debug links, page
   renders, and `/healthz` are not inbox-delivery proof.
6. p95 acknowledgement <100 ms, graded-visible <300 ms, and first quiz <20 s
   measured on the delivered browser contract, not inferred from native timing.
7. Complete backup retrieval, separate-object restore, and equality of the
   appropriate private fingerprint/count/hash receipts.

### Isolated staging evidence (2026-09-07)

These receipts are for `scry-staging` only. Native `https://scry.study`
remained `/readyz` 200 and authoritative. Production Worker traffic was not
enabled.

- Immutable version `cb483442-eac6-468e-8fdd-81d52b2f486c` from
  `f876bf80848159bfc4bf2eaf448e2318f42ab59b`, bundle
  `b98c307a9aa07e52b850fab13e9ee0eea9693fa7c978f041f0252324b4561fa1`,
  activated with `release:traffic --enable`.
- Deployed script settings: `redact_query_string: true`,
  `logs.invocation_logs: false`.
- Fresh AgentMail magic-link: Resend `last_event: delivered`, inbox
  `received`/`unread`, SPF/DKIM/DMARC pass, consumed on phone-sized
  `workers.dev`, replay HTTP 403. CSRF missing/mismatch both 403. Current
  browser logout returned the public sign-in form. Logout-all and signed
  unsubscribe were not exercised.
- Capture of a public network-reference source produced grounded Library
  drafts (library DOM 17698 ms). Explicit keep scheduled study. Reveal
  graded `Revealed` (assisted). Unassisted MX answer graded `Correct`.
  Delivered browser contract: `tapToAckMs` 0, `gradedVisibleMs` 176,
  `viewport: mobile`. Keeping a due draft opened study rather than remaining
  on editable Library; remaining drafts stayed inspectable on Home/Library.
- Succeeded generation job `google/gemini-3.7-flash`,
  `cost_usd_micros` 11216. Operator-gated `POST /v1/service-sessions` 201
  for the QA email (not mail-derived). That token could not read the native
  production account id on this object. Native production history lives in
  the separate rehearsal object, not staging `app`.
- Reminders enabled with 1 due quiz. Inbox received
  `You have 1 Scry review` with SPF/DKIM/DMARC pass. Confirmation copy
  correctly said mailbox delivery was not yet verified; the later due-count
  message is the inbox proof.
- CLI, MCP, and skill faces were not live-exercised against staging.


## Health observations and operator alerts

The Worker logs bounded, content-free browser, action and recovery observations
through Cloudflare runtime logging. A browser receipt's HTTP 202 and
`x-scry-telemetry-delivery: runtime_logged` mean that the runtime logged it;
`attempts: 0` means no external delivery was attempted. They do not prove
provider ingest, durable telemetry storage, or remote readback. Canary is
retired and is not a deployment credential or monitoring dependency.
Automatic invocation logs are explicitly disabled in every deployment
environment: they include request URLs, which can carry magic-link credentials.
Application logs remain enabled, with `redact_query_string: true` so their
platform request metadata cannot retain query credentials either. Verify both
settings through the deployed script-settings API before a mail or browser-auth
drill; do not rely on the platform's redaction default (false).

`GET /healthz`, `/readyz`, and `/statusz` are observations: none wakes background
work. The first two witness liveness and readiness. `/statusz` uses
`memory_engine.runtime_health.v1` and returns 200 only when traffic is active
and `backupAgeMs` is an integer in `0..=90000000` (25 hours). A paused actor,
missing backup, future timestamp, or stale backup returns 503. Backup freshness
does not establish that a restore was rehearsed; retain the separate restore
proof.

`bun run ops:monitor -- --environment production --state-file PATH
--receipt-file PATH` runs `scripts/scry-monitor` against the canonical
workers.dev origin. Use `staging` for the isolated environment. Supply only
`RESEND_API_KEY`, `MEMORY_ENGINE_MAIL_FROM`, and `MEMORY_ENGINE_ALERT_TO` through
the private process environment. The recipient must be the approved operator;
no learner, admin, Cloudflare, or model credentials belong in this process.
The CLI checks mail configuration even when health is good.

`.github/workflows/production-health.yml` owns external observation and alerts.
Its protected `production-monitor` environment permits only `master` and stores
those three mail-only secrets. Scheduled runs require the repository variable
`SCRY_MONITOR_ENABLED=true`; leave it false until Main proves live cutover.
An explicit master-branch dispatch can run before automatic monitoring is
enabled. Its optional unique `delivery_drill` label sends a clearly identified
operator drill, not a simulated production incident.

The first healthy observation sends no mail. Failure opens one incident;
continued failure does not resend an accepted notification. Recovery sends one
notification. Pending messages are saved before transport; unconfirmed
acceptance retains the same payload and idempotency key for retry. A Resend ID
records provider acceptance only, never inbox placement. The provider's
idempotency window is bounded; expired pending requests fail rather than silently
becoming a new send.

GitHub's nominal five-minute cron can be delayed, dropped, or disabled. Its
per-environment cache can expire or disappear; it is not durable alert history,
and losing it can repeat an incident notification. The workflow uploads
sanitized check/provider-acceptance receipts even on failure. This is best-effort
monitoring, not an availability or delivery SLA. Prove actual dispatch and
provider acceptance before enabling automatic runs; record inbox placement
separately if observed.

## Interim-origin cutover and old-host retirement

Main alone owns the live account, source write barrier, final data movement,
actual production activation, browser/machine proof, and old-host routing or
retirement. The authorized free destination is
`https://scry.misty-step.workers.dev`; a registrar transfer, custom domain, or
DNS authority is **not** an activation prerequisite for that origin.
After exact source/hash/version readback, final imported-state comparison,
and accepted recovery proof, activate through `release:traffic` and exercise
that actual HTTPS origin with a fresh browser sign-in and imported machine
sessions. Record timestamps and evidence; no checklist item is satisfied by
this runbook's existence.

Old `scry.study`/`www.scry.study` ingress must not keep sending learner writes
to the frozen native database. Main chooses and proves any temporary
reverse-proxy/redirect behavior, including existing signed links and client
methods, then retires it only after destination continuity is accepted.
A later direct custom-domain route needs separately reviewed DNS authority,
TLS/canonical-host, cookie, and client proof; it is not part of the free-origin
prerequisite or an implicit CLI override.

Before any new destination writes, a failed migration can leave the old host
as the authoritative source under the controlled barrier. **After Worker
writes are accepted, blindly routing traffic back to the frozen old database
would discard learner history.** Recovery then needs a deliberate data-aware
plan; code rollback cannot merge divergent native and SQLite histories.

Keep the pre-session host backup work: `bin/scry-pg-dump`,
`bin/scry-backup-offhost`, `bin/retention-preflight.py`, the installer and
backup timers/alerts, isolated native restore tooling, and their private
configuration/dumps. Off-host backup provisioning or freshness must be proven,
not assumed; an absent/stale destination is a failure. The newly created native
release installer, rollback launcher, Caddy fragments, and runtime unit were
removed from this repo because they are no longer a deployment path; **nothing
was removed from the live host**. Shared private-env helpers and consequential
recovery tests remain until safe host retirement.

Stopping native writers at the source barrier is distinct from deleting the
old service. Only after production continuity and recovery/delivery proof are
accepted may Main retire the old mailer, runtime, reverse proxy/redirect,
ingress, credentials, and hosting. Preserve required private historical
snapshots and receipts according to the recovery/retention decision. Do not
reinstall the old runtime, point back to Neon, delete old dumps, or claim a
completed Cloudflare migration prematurely.

## Source references

- [Wrangler Worker commands](https://developers.cloudflare.com/workers/wrangler/commands/workers/)
- [Wrangler configuration](https://developers.cloudflare.com/workers/wrangler/configuration/)
- [R2 provisioning commands](https://developers.cloudflare.com/workers/wrangler/commands/r2/)
- [Durable Objects and version deployment](https://developers.cloudflare.com/workers/versions-and-deployments/gradual-deployments/with-durable-objects/)
- [Rollback resource/schema limitations](https://developers.cloudflare.com/workers/versions-and-deployments/rollbacks/)
- [Resend send-email API and acceptance response](https://resend.com/docs/api-reference/emails/send-email)
- [Resend email events and delivery status](https://resend.com/docs/webhooks/event-types)
- [worker-build 0.8.5 compatibility source](https://github.com/cloudflare/workers-rs/blob/v0.8.5/worker-build/src/versions.rs)
