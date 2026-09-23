# Scry production runbook

## Current authority

This runbook owns deployment/recovery procedures, not product acceptance or
permission to execute them. [VISION](../VISION.md) and [SPEC](../SPEC.md) own
current intent and acceptance; Linear owns current owner, status, and pause.
The operator rejected the foundations UI on 2026-09-12; on 2026-09-23 they
authorized the concept-centered v5 implementation (MIS-162). This is not
authorization to migrate live data or activate production. The [design study](design/concept-centered-study.md#outcome-2026-09-23)
records that change; current deployed receipts below remain v4 observations.

As of September 22, the canonical application is **https://scry.study** on
Cloudflare Worker `scry-app-host` and singleton Container `scry-app-container`.
Cloudflare Access gates `scry.study`, `www.scry.study`, and `scry.mistystep.io`
with one audience and one exact owner-email policy. The Worker checks the
owner subject before forwarding to nginx; nginx injects the preserved Scry
owner ID. The two alternate hosts redirect reads only and reject mutations.

The Access team domain is `misty-step-pantry.cloudflareaccess.com`, the shared
Cloudflare Access login/JWKS authority—not a Pantry application Worker or Scry
origin. The canonical application origin remains `https://scry.study` after
the Access handoff.

The old `scry-app.exe.xyz` VM service is stopped and disabled with its schema-3
database intact.
Do not start it while the target can write schema 4. The old Rust Worker is not
the live learner store. `scry-dev.exe.xyz` remains an isolated recovery instance.

| Boundary | Authority |
| --- | --- |
| Application | `cmd/scry`, `internal/`, one Cloudflare Container behind `scry-app-host` |
| Live data | Container SQLite at `/var/lib/scry/data/scry.sqlite`; one writer |
| Browser identity | Cloudflare Access owner policy and immutable subject guard, then exact app owner ID, loopback ingress, Host/origin and CSRF checks |
| Model requests | Existing Scry-only OpenRouter key; direct HTTPS chat completions from the Container |
| Recovery | Local consistent archives plus append/read-only `scry-go-backups` Worker and private `scry-go-recovery` R2 bucket |
| DNS | Cloudflare authoritative for `scry.study`; three Worker custom domains, Cloudflare TLS and Access |
| Old runtime | Production and staging Rust Workers paused, cron triggers removed; native Postgres service disabled, recovery backups retained |

### September 23 automatic meaning recall release

Production Worker `scry-app-host` serves version
`49affb5b-a787-4009-9038-16e80f4efa75`, deployed from master
`b9836cf8fd9bc078110c8a906e066664c4f79f38`
([PR 179](https://github.com/misty-step/scry/pull/179)). This release changes
the image. The container application is at version 2 with image digest
`sha256:0a59a1b59ee48d0cf2128672315debbdf552cb291d2af7cdbf4216097e3dbe32`.
It carries the committed-gate binary `scry b9836cf`, SHA-256
`4c3a326e2f87c84fc3b645bd05a89a6f95500d9b8ad59f21f6aa03afbc661470`. The
same digest first passed two synthetic staging cold-wake cycles. Planned
replacement order:

1. Read back the newest R2 snapshot independently.
2. Deploy Worker-only with the one-minute proof window. The old instance
   stopped after a verified backup.
3. Read back that snapshot. Only the backup ledger changed.
4. Restore a private copy with the new binary. Both the new binary and the
   pinned `a36e796` rollback binary passed `check`, with equal export counts.
5. Deploy the committed 24-hour configuration with the pinned new image.

The rollout also stopped the instance started by the first cold wake. It wrote
a verified final backup first. A second planned cold wake restored
`scry-20260923T153256.061901797Z-8574070ca6a3df05f7117ba4de81757f.scry-backup.zip`
on the new image. Its start log showed semantic mode on. Readiness returned
`ready` twice. The owner root returned 403 to the temporary service identity,
and a wrong probe token returned 401. The temporary policy and token were
deleted, and the owner policy did not change. The Worker bindings and the
`0 */12 * * *` schedule did not change.

Rollback stays schema 4. Deploy the previous image digest (below) with the
same planned replacement. Do not restore a pre-release snapshot over newer
writes.

### September 23 recovery closeout state (superseded)

Production Worker `scry-app-host` serves version
`c077459c-72a4-4b6a-b94f-1fda7f5c9a28`, deployed Worker-only from master
`70db046091fd7bc5000b1d662b648d7249ab0c47` (PRs
[174](https://github.com/misty-step/scry/pull/174),
[175](https://github.com/misty-step/scry/pull/175) and
[176](https://github.com/misty-step/scry/pull/176)) with
`--containers-rollout=none --keep-vars --strict`. The singleton Container
instance and the pinned image digest below did not change. The image inputs
(`cmd/`, `internal/`, `go.mod`, `go.sum`, and the hosting `Dockerfile`,
`entrypoint.sh` and `nginx.conf`) are unchanged since `a36e796`, so this Worker
and the pinned image were a matched pair until the image release above. A deploy without
`--containers-rollout=none` may roll out a new Container version and replace
the live instance; treat it as a planned replacement under
[Recovery policy and observation](#recovery-policy-and-observation).
After this deploy, a temporary non-identity Access probe returned `ready`
twice, owner root returned 403, and a wrong probe token returned 401. The
instance stayed running. The temporary policy and token were deleted, and the
owner policy was unchanged. The newest verified snapshot and the observed daily
cycle are in the recovery section.

### September 22 hosting cutover state

Protected PR [168](https://github.com/misty-step/scry/pull/168) merged at
`e1a981d3bfff518d96dbcc4306c135fb62a96901`. Its tree equals the tested
`a36e796e241cdc3612e7b7a2bd422d32263dbdba` integration tree; the merge
SHA is source provenance, not a rebuild of the staged binary. The complete
candidate also includes Jev product PRs 170/171 and the schema-4 migration;
the narrow final nginx fix alone did not change application behavior. Hosted
master CI run `35781708916` passed. The production
binary SHA-256 is `294b61aeefbb16a11a39c6d5467527c1067fefd3dfccdd2a7f2f8445c3651e75`;
the pinned image digest is `sha256:1b165833050cc194ff09146d4c312540ac9b7afd83fad9ab637d3cf706376209`.
The production Worker version at cutover is
`0920a47f-fe9a-432f-b465-e32f21483102`. The production Container uses
restore-required mode, the recovered data class, and a 24-hour idle window.
The final VM snapshot `scry-20260922T210233.992750553Z-5bf7ed2a33dd3452b2ae76f9a4c054be.scry-backup.zip`
was read back independently from R2 with SHA-256
`6b664324fed9039d922a6b27b76129af6e7139e9f612014fbdbfbaaf625d34b7`.
The target restored it and migrated schema 3 to 4. The target's remote snapshot
`scry-20260922T212025.301631528Z-c1cd0425e5a4677b4b0842168a9b70e2.scry-backup.zip`
was read back and checked at schema 4. Every source table's historical columns
retained its rows except the expected appended backup ledger. A subsequent cold
wake restored that exact target snapshot. The source service remains disabled;
verify both `systemctl is-enabled` and `systemctl is-active` before any recovery.

A credential rotation ahead of the running container briefly caused one failed
idle-stop backup. No owner session had written to the target. The target
restored its last verified schema-4 snapshot, then used the corrected gateway
credential to publish
`scry-20260922T212025.301631528Z-c1cd0425e5a4677b4b0842168a9b70e2.scry-backup.zip`.
Confirm the newest remote key and checksum before recovery, since later daily
or shutdown snapshots can supersede this receipt. A failed final backup is not
proof that a newer acknowledged write survived.

Production Access without credentials redirects to login. A temporary service
identity could only reach the readiness probe; owner root returned 403 and a
wrong probe token returned 401. The temporary policy and token were deleted;
the owner-only policy remained. In response to the production-phone checklist
for entry, retained material, one review and deliberate Next, the principal
reported "the phone works." This satisfies the human-input condition as an
owner-reported phone pass, not independent observation of a fresh Cloudflare
Access challenge or of each UI step. Do not ask for a repeat login. The
machine-owned alias authentication/mutation probes remain separate.
An unnecessary new OpenRouter key was briefly issued at $1/day instead of the
existing Scry key's $0.25/week cap; it made no paid calls and was reduced and
disabled. A temporary replacement app secret and generated probe/backup values
were exposed in a diagnostic transcript. The source signing secret and original
Scry provider key were restored in the prepared target configuration; generated
probe/backup values were rotated and a corrected cold restore and remote backup
succeeded. Do not reuse any exposed generated value. See the private sanitized
closeout readback for what provider/runtime evidence does and does not prove.
The semantic endpoint was unset on first activation. Committed production
configuration now sets the Jev Decisions route (see
[Semantic assessments on Cloudflare](#semantic-assessments-on-cloudflare)); it
reaches the application only when a new Container instance starts.
Keep one production writer. The old `mis157-76202e78aef2` binary cannot read
target schema 4. If the target fails after writes, preserve its state and use
the compatible pinned artifact with an independently checked fresh snapshot
restored to an unused path. Do not point the old binary at schema 4 or overwrite
either live database.

The earlier VM release observed on September 11 was `mis59-4e13cafef7c3`, compiled from committed revision
`4e13cafef7c3e7eac1c6f6fbd1fa8e5bfbc087df`. The retained, smoke-tested binary's
SHA-256 is `79da084c9ea9a024b45b508861d72b82a99bf2e54570f2fc3f468d1c900fdc08`.
September 11 protected activation required the previous binary's fresh off-VM
backup and matching readback, then migrated schema 1 to 2. Every pre-existing
learning/history/schedule/operation/job export section remained equal; new
backup receipts are accounted separately. The actual process was active/enabled
as `scry`, bound to loopback port 8080, with that executable checksum and readiness.
The prior `mis48-e0274bc916de` binary is retained but cannot run the upgraded DB.
Evidence-only documentation commits do not relabel or replace the compiled binary.

See [the protected rollout and authorized live QA receipt](qa/foundation-rollout-20260911.json).
After the initial sign-in blocker, the operator authorized temporary VM-scoped
QA tokens for `scry-app` and `scry-dev`. One native-browser foundation request
used `google/gemini-3.7-flash`: 11.855 seconds, $0.006600 settled, one $0.20
reservation, no retry or outstanding reservation. Saved instruction, a six-step
diagram and one agent-exercised warm question survived reload, deliberate return
to the exact original occurrence, and later Library/repeated-help reuse without
another call. All original presentations, review events and FSRS state remained
identical; this was functional QA, not learner performance.

Authenticated changed routes succeeded; anonymous/forged requests were gated,
authenticated invalid-CSRF/cross-site writes returned 403 and alias writes 409,
with every exported state section unchanged. Both temporary credentials were
revoked through exe authority; each prior token then returned 401 and its
fingerprint was absent from the authority list. No login/global-logout or
different-account acceptance is implied.

The September 11 machine review found substantive instruction/diagram, but warm
practice repeated the original multiple-choice answer and four options rather
than isolating a simpler prerequisite. Appropriate practice and usefulness were
not accepted; the optional reference path was absent, not invented or fetched.
The later operator rejection is recorded in the
[design assessment](design/concept-centered-study.md#operator-findings-and-direction),
not an outstanding blanket request to rerun QA or obtain phone sign-off. Saved
material is at canonical `https://scry.study/foundations` under normal private
login. The revoked QA tokens and earlier permission are not authority for a new
exercise.

There is no public signup, application-owned magic-link mail, separate frontend, public service
session, maintained legacy CLI/MCP contract, or second application database.
The target backend listens at `127.0.0.1:8081` behind nginx; the retained VM
listened at `127.0.0.1:8080`. Do not open either publicly or trust an identity
header from an arbitrary peer. `deploy/scry.env.example` documents the
configuration; populated files and provider capabilities are private.

## Schema v5 release boundary

The v5 candidate transactionally migrates complete schema 4 to 5, preserving
foundation-origin rows, old source/quiz/review/FSRS/spend history and pausing
old foundation jobs; it adds concept relations, goals, immutable notes,
documents, capture images, evidence, grade overrides, preferences and a concept
index for reuse.
The retired foundation routes/UI do not erase their data. Migration is
irreversible: after `user_version=5` a v4 binary cannot read the upgraded
database. There is no down-migration.

**Activation checklist (separate explicit approval required):**

1. Retain the compatible v4 binary and a complete independently checksummed
   pre-release v4 R2 snapshot; verify old-binary compatibility on an unused
   copy. Confirm only one production writer and account for all acknowledged
   writes before replacement.
2. Run the v5 candidate's read-only `check --db PATH` on another copy, then
   rehearse migration on a separate isolated copy. Compare immutable
   learning/history/schedules/content/spend, concept backfill and restored
   service; leave uncertain paid work paused.
3. Bind the exact committed smoke-tested v5 binary, source revision and
   reviewed private config. Check `SCRY_MODEL`, Jev Decisions and optional
   Exa key/HTTPS endpoint, $25/week provider cap, $3.50 rolling-day allowance,
   $0.50 reservation, and app-only secret forwarding without logging values.
4. Read back and checksum the newest off-VM pre-release archive before any
   Container replacement. Obtain explicit operator approval for the
   irreversible live schema migration and planned replacement.
5. After activation, verify actual binary/schema/readiness, owner-only ingress
   and CSRF, capture modes, held Stream result/self-check/override, intro,
   Map/Concept notes, share target, backup status and independent remote
   readback. Record what remains unverified; a local fixture cannot prove
   live provider quality, production access, or recovered service.

If v5 activation fails after migration, preserve the v5 database and its
post-snapshot writes; stop rather than launch v4 against v5. A rollback to v4
requires **restoring the preserved v4 snapshot into an unused path with the
previous v4 binary**, separately verifying its service and data, quantifying
any acknowledged writes absent from that snapshot, and obtaining explicit
operator approval for the resulting loss before switching a single writer.
Never overwrite a live database, clone a live writer, or call a code-only
downgrade a rollback. A v5-compatible fix can instead be activated after
independent proof. If the activation response is lost, inspect the actual
process, schema, readiness, remote snapshot and artifact checksums before
deciding; source HEAD and a missing response prove neither outcome.

`check --db PATH` is the intended read-only compatibility probe. `export`
opens a store and may migrate; do not run it on the untouched pre-upgrade DB
as an inspection shortcut. `--allow-local-backup` is synthetic rehearsal only.

## Release provenance and retained VM procedure (not the current Cloudflare deploy)

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

The following install/activate steps describe the retained exe.dev VM release
path; they are **not** Cloudflare deployment or permission to restart its
schema-3 writer. Transfer the exact tested binary and reviewed `deploy/`
scripts to a private staging directory on the intended VM. Supply a complete
private environment only for first installation. From that directory, with
operator-approved administrative authority:

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

## Retained exe.dev VM configuration (historical; do not run at cutover)

On the retained VM, the service ran as `scry`, not root. Its configuration is
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

For the retained VM, browser login/logout was owned by exe.dev. A separately
scoped VM API token is independent of browser login and is not revoked by
logging out. Revoke temporary tokens through their actual authority. Do not
print their values in receipts.
Current production identity is the single three-domain Cloudflare Access
application plus Worker owner-subject guard, not an exe.dev session or QA token.

## Generation and spending

The ignored workstation `.env` retains the dedicated Scry-only provider key
privately (mode `0600`), never copied wholesale into production. On 2026-09-23,
the existing key **“Scry personal (exe.dev)”** was raised to a **$25/week**
provider limit; that cap covers content generation and Jev checks and is not
proof either path is active. Keep provider management credentials out of the
application. The app's separate allowance is **$3.50 per rolling 24 hours**
(`SCRY_GENERATION_DAILY_BUDGET_MICROS=3500000`) with **$0.50 reserved per
generation attempt** (`SCRY_GENERATION_RESERVATION_MICROS=500000`).
Provider weekly and app rolling-day windows differ. Unknown sent costs retain
the reservation; neither window is an invented per-token price.

Generation uses `SCRY_MODEL_ENDPOINT`, `SCRY_MODEL_API_KEY`, and `SCRY_MODEL`.
Meaning-sensitive recall separately uses `SCRY_SEMANTIC_ENDPOINT`,
`SCRY_SEMANTIC_API_KEY` (empty reuses `SCRY_MODEL_API_KEY`), and
`SCRY_SEMANTIC_MODEL` (default `typesafe/jev-1.13`), and
`SCRY_SEMANTIC_RESERVATION_MICROS` (default 2000, USD micros reserved per check
from the same rolling 24-hour allowance as generation). An empty semantic
endpoint means no Decisions request is sent, nothing is reserved, and the
saved answer remains ungraded. A configured endpoint must be a complete HTTPS
URL without embedded credentials, query, or fragment, because every request
carries the bearer key and private learner text; plaintext HTTP is accepted
only for a loopback gateway, and the service refuses to start otherwise. Before
production activation, prove that the configured private integration forwards
`POST /api/alpha/decisions`; generation access alone does not prove that route.

`SCRY_EXA_API_KEY` is an optional, bounded private Container secret. Without
it, Topic research completes with zero documents at zero cost and plans from
labeled general knowledge; a Link capture fails recoverably at zero cost,
because its page cannot be read.
`SCRY_EXA_ENDPOINT` is optional and defaults to `https://api.exa.ai`; configured
production endpoint must be HTTPS with no embedded credentials, query or
fragment. Only an explicit **Topic** capture issues Exa search. **Link** fetches
its chosen URL through Exa contents. Pasted **My text** is never searched;
**Photo** transcribes before planning. Exa documents, published date, provider,
quotes, and citations remain inspectable on Source/Concept. Search/fetch costs
share the app's bounded allowance and unknown outcomes are not assumed free.
`SCRY_MODEL` remains one explicit model identifier, not automatic routing.
Never put the Exa key in a plain Worker var, backup exec environment, image,
flags, or logs. [Hosting configuration](../deploy/cloudflare-hosting/README.md#private-configuration)
describes the app-only forwarding boundary.

### Semantic assessments on Cloudflare

Production `wrangler.jsonc` sets three plain vars: `SCRY_SEMANTIC_ENDPOINT`
(`https://openrouter.ai/api/alpha/decisions`), `SCRY_SEMANTIC_MODEL`
(`typesafe/jev-1.13`), and `SCRY_SEMANTIC_RESERVATION_MICROS` (`2000`).
`SCRY_SEMANTIC_API_KEY` is not set, so the application reuses
`SCRY_MODEL_API_KEY`, the dedicated Scry provider key with its $25/week
provider cap, shared with generation. Staging and `mistystep-prod` set no
semantic var and therefore send nothing; the v5 app's `short-v1` checks use
the same Decisions endpoint only for flexible short recall.

`appEnvVars` forwards all four names, always: empty strings when the endpoint
is unset. It refuses to start the Container when the configuration is partial
or unsafe: any semantic setting without an endpoint; a non-HTTPS, credentialed,
query, or fragment URL; a path other than `/api/alpha/decisions` (so a chat
completions URL cannot stand in); an empty, whitespace, or `openrouter/auto`
model; a reservation that is not a positive integer no larger than the daily
allowance; or no usable key. `backupExecEnv` stays limited to the five
`SCRY_BACKUP_*` settings.

A Container receives `envVars` only when it starts. A Worker-only deploy with
`--containers-rollout=none` stores the new vars but the running instance keeps
its old environment. Enabling, disabling, or changing semantic settings is
therefore a planned instance replacement: confirm the newest R2 key and
checksum with an independent readback, let the instance stop through the
verified-backup idle path (not a force stop), read the new archive back, then
cold-start the instance. To disable, remove the three vars and repeat the same
replacement; the image and schema need no change.
Learner answers sent for semantic assessment are private provider-bound text:
state contains only prompt, expected answer, variants, rubric, and learner answer,
not identity or review history.

Saved capture creates a durable bounded generation job. Semantic Check instead
stages a durable assessment, calls the model outside SQL with an eight-second
limit, then re-fences the occurrence/content/schedule in a second transaction.
Each assessment gets exactly one send lease (30 seconds). Before the request
leaves the process, the reservation is checked and recorded against the shared
allowance in the same transaction. A duplicate submit during a live lease is
refused; an exact replay reconciles the durable state and never resends; a
lease that lapses without a result becomes a definite failure with its
reservation retained as unknown spend. Once a request has left the process,
its judgment or failure is settled under a bounded context detached from the
learner's connection, so a dropped request cannot turn a transmitted result
into an unknown outcome. The no-endpoint path is the only case that reserves
nothing and marks the failure as a provable no-send, because nothing left the
process. Returned semantic cost is rounded
up to micro-dollars, stored per assessment, and replaces the reservation in the
rolling 24-hour allowance; missing provider cost stays unknown, never zero.
Failures keep the answer ungraded for a deliberate new operation (retry) or
reveal. Inspect the recorded state and reconcile explicitly; the app never
auto-retries a paid unknown outcome.

The frozen `semantic-v1` policy applies only Correct. Incomplete and incorrect
are recorded as shadow classes (`decision` with `applied=0`) for evaluation and
render as ungraded. Enabling a class is a code change with holdout evidence.

Source-backed quizzes keep exact supplied evidence. Topic expansions are
labeled as model knowledge. Structural/provenance checks are not independent
fact-checking. Rejected candidates and incomplete coverage remain visible as
partial/failure, not invented completion. The private acceptance receipt
records representative live generation; it is not a longitudinal efficacy claim.

### Prepublication critic

The same semantic endpoint, model, key, and reservation configure the content
critic. A configured endpoint requires a positive semantic reservation.
No endpoint records `skipped`, reserves nothing, and retains existing generation
behavior. A pending batch cannot bypass criticism if configuration later disappears.
This code path is not evidence of production activation. The critic's evaluation
corpus was agent-authored, not human-validated, and it once accepted an
answer-leaking candidate that independent generation validation rejected.
Treat critic acceptance as one check, not proof of quality.

The worker saves at most 60 validated candidates per critic batch (ordinary
concept questions at most 36); exact-text and complete-set tasks up to 60
units are judged whole. Oversize batches fail rather than silently truncate.
Each candidate receives an independent defect battery. The frozen `critic-v1`
threshold is 0.80 for hard defects; explanation value never vetoes.
Any missing or malformed judgment blocks publication of the batch until retry.
Accepted candidates publish only after transactional revalidation. Rejected
candidates retain their reasons; an entirely rejected batch publishes nothing.

Unavailable criticism preserves candidates with `critic_status=pending`.
The next automatic claim runs only criticism, not generation. A new critic
assessment reserves the semantic amount, never another generation reservation.
Three total job attempts bound automatic work. Explicit Retry resumes the same
batch up to five total attempts and reuses judged candidates. Restores remain
paused until explicit retry. Fully rejected batches require revised input.
Each assessment row remains single-send forever. New attempts use new rows;
unknown prior outcomes remain charged, rather than becoming free retries.
Expired orphan leases become failed during allowance reconciliation.

Inspect `jobs.candidates_json`, `critic_status`, and exported `content_assessments`
for candidate content, requests, responses, reasons, model, and spend. Job cost
readouts include critic usage. `Store.ContentHistory` retains candidate attempts;
review history remains learner events. Export omits content-assessment lease tokens.
The bounded live control test is opt-in and uses synthetic/public text only.
Record its source revision, returned model, raw receipts, errors, and cost.
Do not treat a small control set as broad accuracy or activation proof.

The MIS-59 foundation job/retry UI is retired in v5. Migrated foundation
records and old paid outcomes remain historical; any uncertain work stays
paused with its reservation. Do not use old `/foundations` routes or attempt
to revive paid work under the new chain. Inspect the original record and
obtain separate authority before any manual cost reconciliation.

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

`SCRY_BACKUP_INTERVAL=24h` and `SCRY_BACKUP_KEEP=30` govern in-process cadence
and local completed-snapshot count. Production Worker `scry-app-host` also has
a native UTC cron at 00:00 and 12:00. While the singleton Container is running,
the cron executes the existing `scry backup --require-remote` command in that
instance, checks its success receipt against R2 metadata, and logs the result.
A failed invocation fails the scheduled event; it does not claim a remote
snapshot. The event's error carries the exit code and a redacted last stderr
line, never stdout. The exec does not inherit the container start
environment; it receives only the five `SCRY_BACKUP_*` settings. If the
Container is stopped, the cron checks the newest R2 object
without waking it. An object older than 24 hours is reported as `idle_stale`;
that age alone does not show data loss when the writer has been stopped.
Before a planned idle stop, the Container runs a remote-verified backup and
defers the stop if this fails. Other stops attempt the entrypoint's final
backup, but host failure or a force stop may still lose recent writes. A
scheduled run and a successful idle stop do not guarantee the RPO objective.
Check the latest remote archive and exact readback before every planned
instance replacement. Time-based remote retention is a separate R2
lifecycle policy; a count of local snapshots is not proof of 30-day off-VM
retention. The app/gateway cannot delete, overwrite, or change bucket policy.
Use only the dedicated new-app bucket for this lifecycle policy, never the
historical recovery buckets. Keep independently recoverable gateway capability,
compatible release artifact, and required configuration outside the VM.
The dedicated bucket's enabled `scry-go-recovery-retention` rule was read back
with an object age of 2,592,000 seconds (30 days). This is configured retention,
not an observation of a month of successful backups. Committed `appEnvVars`
forwards `SCRY_BACKUP_INTERVAL=24h` and `SCRY_BACKUP_KEEP=30` to the
container; `Manager.Run` attempts a snapshot at startup and every 24 hours
**while the process runs**. The 24-hour idle window can end that loop, so the
Worker cron and the idle stop carry the daily policy: a running Container gets
a remote-verified backup at the next 00:00 or 12:00 UTC run, and an idle
Container stops only after one more verified backup. A stopped Container has
no writer, so the cron reports its newest R2 object and never wakes it.
Failures are visible in Workers Logs as failed scheduled events and
`[scry-recovery] idle stop deferred` entries; there is no external alert. A
host failure or force stop can still lose writes made after the last verified
archive.

Observed September 22-23, 2026 (single runs, not a guarantee): the 00:00 UTC
production cron wrote
`scry-20260923T000047.310173185Z-8c4def576aa81711cdc00f428e799874.scry-backup.zip`
(SHA-256 `bb1dff3639e527b597dfb3ad950f7d0931251e691f8dafd12b0ce60c5f263565`,
485,090 bytes) without replacing the running Container. An independent R2
readback matched its size and checksum, passed SQLite `integrity_check` at
schema 4, and found more review events than the pre-phone cutover snapshot.
On synthetic staging, a fresh instance idle-stopped only after a verified
backup, and the next cron reported `idle` without a wake. Confirm the newest
key and checksum again before any planned instance replacement.

`GET /healthz` returns `ok`; `GET /readyz` returns `ready`. These are plaintext
liveness/readiness checks, not backup freshness. Settings exposes the last
completed backup and error/staleness state. Inspect the service journal without
printing content or credentials. No current Go external mail/availability
monitor is claimed; the retired Worker monitor is disabled, not repointed to
an incompatible response format. Native backup monitoring remains separate.

Manual app commands are `scry version`, `check --db PATH`, `export --db PATH`,
and `backup --db PATH --require-remote`. For retained VM procedures, use the
service's private environment and unprivileged account through systemd, not a
shell-sourced secret file; do not start that writer now. Export contains private
learning content; protect it like the DB.

## Retained VM independent restore drill (not current target recovery)

For a **new isolated VM** drill, retrieve a completed `.scry-backup.zip` from
private off-VM recovery using independently retained authority, and verify its
SHA-256. Keep a compatible reviewed binary and configuration independently of
the lost VM. On an isolated
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

For Cloudflare production schema 4, use the pinned digest-compatible artifact,
the newest independently checksummed R2 snapshot, and restore-required boot in
an unused target; do not run the retained VM's schema-3 binary or overwrite a
live database. Preserve the current target before any config rollout. The
cutover receipt proved a target-written snapshot and cold restore, not a future
RTO or physical-phone session.

Activate separately when ready. Synthetic rehearsals without remote backup
capability may explicitly use `--allow-local-backup`; real recovery must restore
the approved production recovery capability before normal operation. Prove the
actual unprivileged process, private HTTPS, export equality, and UI; record
elapsed time and omitted steps. Do not clone active scheduler ownership. Stop
and disable the rehearsal service afterwards.

The September 11 drill independently downloaded the fresh schema-2 archive
`9d057372818f59ec83f866c5caddbf632b3c8ee4e57a370b14b8313d57aaef08` using retained
off-VM recovery authority and restored it to the unused
`/var/lib/scry/mis59-rehearsal-20260911.sqlite` on `scry-dev`. Every snapshot export
section matched, including all history/accounting. The exact MIS-59 binary ran
as an isolated bounded transient `scry` service and passed executable/checksum
and loopback readiness checks; one local startup-backup receipt was separate.
No production integrations or scheduler ownership were attached. The transient
service was stopped/collected; the existing service remains stopped/disabled.
Restore through process verification took about 71 seconds on the existing VM.
Provisioning, DNS, login recovery and authenticated private UI were not measured;
private ingress returned sign-in. Foundation tables were empty because real
generation access was unavailable: this is not off-VM proof for newly generated
foundation records or a complete end-to-end RTO claim.

A later authorized September 11 drill retrieved archive
`0570ca1a312b14506e50d0b5ceafb5acd4d4c7b4845330c8e9272437f890816b`
(329,316 bytes) with the independently retained recovery authority and exact
authenticated readback. It contains one foundation request/bundle/bridge, three
materials, two units, six links and seven interactions. Restore into unused
`/var/lib/scry/mis59-liveqa-20260911.sqlite` matched every snapshot export section,
including all history, operations and spend; no uncertain jobs needed changes.
The exact binary ran as an isolated transient `scry` service, passed readiness,
and rendered the saved instruction through private HTTPS using the dev-scoped
QA token. Service state stayed equal except one isolated local startup-backup
receipt. Restore through private UI observation took 58.33 seconds on the
existing VM, excluding provisioning, DNS and operator-login recovery; this is
not a complete RTO claim. The transient service was stopped/collected, MainPID
zero and no port-8080 listener; normal dev service remains stopped/disabled and
its original DB metadata/current-release link remained unchanged. No production
integration or recurring recovery writer was attached. Both QA tokens were
revoked and denied afterward; production was observed on the same exact release.

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

- The September 22 production-cutover sanitized reports
  (`REPORT.md`, `03-production-access-proof.json`,
  `04-production-remote-snapshot-proof.json`, and `CLOSEOUT-READBACK.md`) live in
  the private operator reports directory, not this public repository. The
  machine-owned Access probes are not instructions for a human to manufacture
  authenticated HTTP mutations. The principal's phone report is human
  acceptance, not an automated/fresh-login proof or Jev feature activation.
- [Earlier private Go acceptance](qa/personal-go-acceptance-20260909.json): live
  generation, trusted touch, interrupted access/response recovery, data restore.
- [September 9–10 cutover acceptance](qa/personal-go-cutover-20260910.json):
  criterion-level browser corrections, generation, canonical activation, and
  explicit remaining identity-switch evidence.
- [September 11 foundation rollout](qa/foundation-rollout-20260911.json):
  protected schema-2 activation plus `authorized_live_followup` provider/browser
  and new-record recovery evidence; earlier blockers remain historical.
- Current exported `proof.json`: source-bound exact-binary gate; synthetic and
  without production model/recovery capability.
- Dated September 8 Rust receipts remain historical. Do not reinterpret them
  as Go, new-domain, or current-monitor evidence.

The September 9 phone approval establishes the operator's initial experience
acceptance, not acceptance of the later rejected foundations flow. It does not
establish sustained use, delayed recall, availability, or unmeasured
DNS/provisioning recovery time.

Global exe sign-out was exercised without an independent VM token. Private
content stayed absent on history return, but that return encountered an upstream
authentication redirect loop; a fresh canonical navigation reached sign-in.
Switching to a different exe account remains unverified in that receipt.
Do not broaden trust or assume logout covers S09.2; Linear owns current work
status. [SPEC](../SPEC.md) owns current S04.2/AI1 acceptance, including the later
negative foundations assessment rather than merely pending usefulness review.
The receipt records candidate performance/accessibility budgets separately;
initial phone-flow approval is not a measured p95 or general AI-quality claim.
