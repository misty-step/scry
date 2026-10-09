# Rust Cloudflare replacement candidate

The operator instructed a full redesign/rewrite on 2026-10-07. This document
owns the replacement's architecture, build, and proposed release procedure.
[The operational runbook](runbook.md) still owns deployed state: the documented
Go/SQLite Container at `scry.study` has not been replaced by source changes.
Its exact source remains in Git at `c6ba395`; compatible artifacts and private
configuration remain independent recovery material. Old Rust Workers,
Worker/R2 data, frozen Postgres, and the retained VM stay separate and paused.

The candidate is a ground-up implementation, not a resurrection of the retired
Rust stack or an import of old learning records. A green gate, local workerd,
authored fixture, screenshot, or merge is not activation approval, model-quality
acceptance, or physical-phone approval.

## Architecture and ownership

A plain Rust/WASM Cloudflare Worker validates the Access JWT signature, exact
issuer/audience/owner subject, expiration, and canonical Host/origin. It forwards
through a separate internal capability to one named `LearningSpace` Durable
Object. The object's SQLite transactions atomically own application records,
immutable presentations/events, full pinned FSRS cards, exact operation receipts,
leases, allowance reservations, and held review results. External model/Jev calls
happen after ownership and allowance commit; fresh state is checked before
publication. Only this singleton can write learning or claim paid work.

R2 stores private input photos and complete versioned recovery archives. Each
archive includes a committed application snapshot plus every referenced photo's
bytes, metadata, and checksum. Success requires exact size/checksum/full-byte
remote readback and archive validation. Daily and pre-release snapshots with
30-day retention target RPO 24 hours and RTO 60 minutes; those objectives are
not guarantees. Durable Object storage and local R2 alone are not independent
remote-recovery proof.

The candidate bounds a complete archive to 16 MiB, including base64 photo
bytes, and each SQLite record to 1 MiB. Photos are at most 4 MiB individually.
Create and content edits reject excess archive growth before committing and
retain the draft; archive sizing happens before reading photo bodies. Paid
completion receipts remain durable even at a capacity boundary. A later capacity
condition can prevent a complete backup and must be resolved by the operator;
it never becomes a successful backup or silent data deletion.

Rust renders escaped HTML and serves self-hosted fonts/styles and a small vanilla
controller. Core POST forms work without JavaScript. The controller only presents
and reconciles, preserves one frozen unresolved answer, pauses offline, avoids
browser storage, and hides private pages on denied access or pagehide. Protected
notes, names, goal intent/title, historical answers, edit, export, and photos
require assistance before they can coexist with an unaided current attempt.

The scheduler port retains the Go adapter's original algorithm identity and full
state. The 260 reference trajectories verify that bounded compatibility. They do
not establish personalized retention, model calibration, mastery, or efficacy.
The accepted product and evidence contracts remain in [SPEC](../SPEC.md),
[stories](../USER_STORIES.md), [design](../DESIGN.md), and [QA](qa/system.md).

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
Automatic deployment from master is intentionally absent: CI prepares a candidate
and preserves `ci`, `foundation`, `story-walk`, and review requirements.

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

The application allows $3.50 per rolling day and reserves $0.50 per preparation
against shared generation/meaning/critic use. Unknown sent outcomes keep their
reservation. The provider's existing $25/week key cap is a separate control.
Live-provider use requires the current session's integration authority and a
bounded exercise; do not clone integrations into a recovery preview.

## Reviewable release procedure

1. Bind the accepted replacement scope and exact committed source to the PR/work
   record. Complete required source gates and current Worker story receipts.
   Preserve honest gaps: representative useful material/holdout quality,
   physical phone, and production ingress cannot be inferred from fixtures.
2. Freeze the smoke-tested Worker modules/assets, checksums, source archive,
   lockfiles, compatible configuration, and proof. Prepare a reviewed release
   configuration pointing to those exact bytes with build steps disabled;
   rebuilding during activation invalidates the proof.
3. In a new private preview, verify anonymous/forged denial, exact owner JWT
   claims/signature, alternate-host read-only redirects, origin/CSRF, access
   loss/history return, and the actual Create/reference/practice/refinement
   surface. Keep provider exercises bounded and provenance honest.
4. Verify a complete remote backup by full readback. Independently retrieve it
   with retained recovery authority and restore into an unused namespace using
   compatible frozen assets/configuration. Inspect immutable content/history/card
   equality, all photo bytes, expected key/CSRF changes, paused work, private HTTPS,
   and UI. Measure time/data loss and name
   omitted provisioning, DNS, or configuration steps.
5. Prepare the activation and rollback plan around one writer. Retain the current
   Go Container's exact artifact/private configuration and a fresh pre-release
   remote snapshot. No live-data import or destructive historical-store action
   is implied; any such requirement needs separate scope and migration proof.
6. Present the concrete artifact, configuration, ingress/recovery receipts,
   remaining acceptance limits, and writer plan for **explicit operator activation
   approval**. Source approval or a merge is not that approval. Do not publish,
   redirect production, restart a predecessor, or retire a live writer first.
7. Once activation is actually authorized, use the reviewed frozen configuration
   and tested modules, identify the resulting Worker version, and exercise actual
   owner/anonymous ingress, persistence, live UI, and remote readback. Record
   those observations in the operational runbook. Retire only the agreed writer;
   keep historical stores/artifacts independently recoverable.

No automatic deployment credential enters the build/test workflow. The former
Go Container deploy job is removed rather than pointed at replacement source.
The deployment/recovery scripts under `deploy/cloudflare-hosting/`, retained
exe.dev VM scripts, and old data tooling remain specific to their corresponding
historical/live-Go environment.

## Independent fresh-space restore

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
