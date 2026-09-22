# Scry production runbook

## Current authority

This runbook owns deployment/recovery procedures, not product acceptance or
permission to execute them. [VISION](../VISION.md) and [SPEC](../SPEC.md) own
current intent and acceptance; Linear owns current owner, status, and pause.
The [September 12 operator assessment](design/concept-centered-study.md#operator-findings-and-direction)
rejects the foundations experience and calls for design reset, not another
technical gate. Earlier phone-flow approval does not cover that experience.

As of September 22, the canonical application is **https://scry.study** on
Cloudflare Worker `scry-app-host` and singleton Container `scry-app-container`.
Cloudflare Access gates `scry.study`, `www.scry.study`, and `scry.mistystep.io`
with one audience and one exact owner-email policy. The Worker checks the
owner subject before forwarding to nginx; nginx injects the preserved Scry
owner ID. The two alternate hosts redirect reads only and reject mutations.
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
The semantic endpoint is unset on first activation. Schema-4 Jev code and
rubric/content paths exist, but live semantic grading and criticism are not
enabled or proven. The product program owns endpoint compatibility, capped
provider access, representative behavior, and acceptance before enablement.
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

### Schema 2 foundation release boundary

The active MIS-59 binary reads known complete schemas 1 and 2. Candidate
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

If activation fails or its response is lost, do not infer the running release
from source HEAD, the `current` link, or a missing response alone. The
[failure trap](../deploy/activate.sh) leaves the service stopped when no verified
compatible rollback is available; preserve that boundary rather than forcing
the old binary to start. In the existing work record, hand off candidate/prior
release IDs and checksums, the pre-release archive/readback reference, schema
check result, and last observed process/readiness state, separating unknowns
from observations. The current work owner retains the stopped-release decision.
Resume compatible activation or the [isolated restore procedure](#independent-restore)
only within existing explicit authority; obtain a new operator decision when
recovery or potential data loss exceeds it. Procedures do not extend that grant.

For an authorized compatibility inspection, use the intended binary's existing
`check --db PATH`, not `export`. The CLI's
[`export`](../cmd/scry/main.go) opens through
[`store.Open`](../internal/store/store.go): it can create a database, migrate
schema 1 to 2, and set WAL mode. It is not a read-only schema probe and must not
be used to investigate an untouched pre-upgrade DB.

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

The ignored workstation `.env` retains `OPENROUTER_API_KEY` with mode `0600`.
The existing dedicated Scry key has a $0.25/week provider limit with a weekly
reset. The Cloudflare target is configured to use that same key directly; the
stopped VM used `scry-model`. A redundant newly issued key is disabled with
zero usage and is not the authorized additional allowance.
No provider management key belongs in either application runtime. The
application uses a $1 rolling 24-hour allowance with $0.20 conservative
reservations per generation attempt. Provider and app windows differ; neither is
an invented token price.

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
This code path is not evidence of production activation.

The worker saves up to twelve validated candidates before criticism. Larger
configured batches publish only that bounded selection and report partial work.
Each candidate gets an independent defect battery. The frozen `critic-v1`
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

On interruption, hand off the saved foundation/job identity, last recorded
status/error, known cost versus retained reservation, and any provider receipt
reference through the existing work record (private content stays private).
The current work owner needs a provider outcome and explicit operator authority
before selecting any manual reconciliation; if either is missing, leave the
job paused and reservation intact. The
[current CLI](../cmd/scry/main.go) supplies no reconciliation command, and the
[foundation retry control](../internal/store/foundation.go) rejects unknown-cost
work. Do not turn “reconcile explicitly” into an invented command or database edit.

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

`SCRY_BACKUP_INTERVAL=24h` and `SCRY_BACKUP_KEEP=30` govern in-process cadence
and local completed-snapshot count. Production Worker `scry-app-host` also has
a native UTC cron at 00:00 and 12:00. While the singleton Container is running,
the cron executes the existing `scry backup --require-remote` command in that
instance, checks its success receipt against R2 metadata, and logs the result.
A failed invocation fails the scheduled event; it does not claim a remote
snapshot. If the Container is stopped, the cron checks the newest R2 object
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
for the deployed source forwards `SCRY_BACKUP_INTERVAL=24h` and
`SCRY_BACKUP_KEEP=30` to the container; `Manager.Run` attempts a snapshot at
startup and every 24 hours **while the process runs**. The Worker has no cron
trigger, and the container's 24-hour idle sleep can stop that loop; therefore
the configured interval and the verified cutover snapshot do **not** prove a
wall-clock daily backup on an idle instance. A fresh off-VM backup must be
confirmed before planned release/rollout, and backup freshness monitored; a
separately authorized durable daily wake/schedule is needed for a literal
calendar-day backup guarantee. Do not silently create a cron or wake the owner
app just to turn this documentation claim green.

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
