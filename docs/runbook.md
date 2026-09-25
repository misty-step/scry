# Scry production runbook

## Current authority

This runbook owns deployment/recovery procedures, not product acceptance or
permission to execute them. [VISION](../VISION.md) and [SPEC](../SPEC.md) own
current intent and acceptance; Linear owns current owner, status, and pause.
The operator rejected the foundations UI on 2026-09-12; on 2026-09-23 they
authorized the concept-centered v5 implementation (MIS-162), and on 2026-09-24
approved its release ("go for it"). The release completed steps 1–4 of the
[schema v5 activation checklist](#schema-v5-release-boundary).
Step 5 is partial: binary, schema, readiness, ingress denial and remote readback
were verified, but no owner-authenticated flow was (see below). The
[design study](design/concept-centered-study.md#outcome-2026-09-23)
records the product change.

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

### September 24 phone sign-in lifetime

The production Cloudflare Access application `Scry production owner` on
`scry.study` and its sole `Allow Scry production owner` policy both had a
one-hour session duration. On September 24, the operator chose 30 days; both
were changed to `720h` through the Access API and read back. The policy still
allows only the same owner email, the application still selects only the existing
One-time PIN identity provider, its three domains and audience are unchanged,
and the Worker still requires the exact owner subject. The account-wide
Cloudflare Access session remains 24 hours; an unexpired application cookie
allows access for its own lifetime without relying on that shared token.

On a Safari home-screen installation, request the code and **type the PIN into
that installation's Access page**. Opening the email's login link in Chrome
does not transfer Chrome's cookies to the Safari home-screen app. Existing
one-hour tokens are not lengthened in place: sign in once inside the installed
app to obtain a new token. The app's separate signed form/CSRF cookie remains
12 hours and is renewed on an authorized read; an old open form may need a
reload before posting, but that should not require a new Access email.
The policy and anonymous redirect were checked live; a new owner token and
physical-phone persistence have not been verified without the owner's phone.

### September 24–25 owner-login 1101 incident

After the owner entered a PIN inside the phone PWA, the page returned 1101.
Workers Logs for `scry-app-host` show `[scry-container] error: Error: Network
connection lost.` at 23:44:47 UTC and repeated `container lifecycle is stopped;
refusing a competing start` on fetches at 23:52–23:53 UTC. This is after the
Worker's Access JWT and exact-subject check, not a PIN or session-duration
rejection. The platform still reported the singleton as running while the
Container SDK's stored lifecycle state said `stopped`; `wrangler containers
info` reported one active but zero healthy instances, and SSH returned HTTP
400. Rolling Access back to one hour would not address this exception, so
the 30-day owner-only policy was left intact.

The 00:00 UTC scheduled backup executed in the existing instance and reported
`backed_up`: `scry-20260925T000041.039650444Z-e2e1671859ebaebae8a3df1d0bb36302.scry-backup.zip`,
648,930 bytes, SHA-256
`ff39cca4b04c907e4d51429fd0627b75f156aec70c3add4048cf11f53e119890`.
An independent R2 GET matched that digest. The Worker lifecycle now joins the
already-running process when the SDK says `stopped`; it passes the newest
snapshot key in case the process exits before readiness, rather than risking
an empty-key restart. The regression failed before and passed after the fix;
all 54 Cloudflare-hosting tests passed. Worker-only deployment
`75f06840-55f8-4326-a1e2-509f496d433b` used
`--containers-rollout=none`: singleton instance
`7ecab3eee8035c56aeaba32584ba11eec1b85280f572a6daa85e51849d23f575`
remained on image version 7. Anonymous ingress still redirects to Access.
Owner-authenticated phone entry after this deployment remains to be observed;
neither a successful backup nor an anonymous redirect proves it.

### September 24 single grading rule release (current)

The Worker serves `75f06840-55f8-4326-a1e2-509f496d433b` with the
stale-lifecycle recovery fix, the 24-hour idle configuration, and the rotated
probe token. The container application remains at version 7 with image
`sha256:ac4bba4d2ba0d7c6d010c5d1c4993ab4fe6d03fec3524c647eb2a6871d6d2168`. It
carries committed-gate binary `scry f9667eb`
([PR 190](https://github.com/misty-step/scry/pull/190)), SHA-256
`0ae6b941174cb7f2c55fb380773f55bd7c76c8ec06695f04e851a97a1d7d6b31`.

The exact/flexible answer form is gone. Locally, only the exact key or an
authored variant is correct and a wrong choice is a miss. Every other recall
answer goes to Jev `short-v1`, with unchanged thresholds.

Before activation, a held-out live check ran through the production builder
and policy with 14 cases:
- **10 exact-form misses** (spelling, year, symbol, symbol case, verbatim
  line, number, status code, precision): none was accepted. 8 were rejected,
  and 2 spelling slips went to self-check. Identity risk was 0.97–0.98.
- **4 same-meaning answers:** 2 were accepted ("CO2", "the mitochondrion").
  2 went to self-check ("paris" at identity 0.50, "in 1989" at 0.40), which
  errs on the conservative side, not toward a false success.

Rollout:
1. Quiesced with the one-minute window.
2. The final backup
   `scry-20260924T190414.558669067Z-fbac8c29db711d846a31b60786f10dfc.scry-backup.zip`
   (SHA-256 `7ad40d07…`) read back and passed `check`.
3. Deployed the image. The first probe came after the image propagated, at
   application version 7.
4. At 19:23:40 the new instance restored that snapshot on revision `f9667eb`.

Readiness returned `ready` and health `ok`. The owner root returned 403, a
wrong probe token 401, and the temporary Access policy and token were deleted.

### September 24 grading release (superseded)

The Worker serves `411db3f6-4cf1-425e-82cc-a356425dfec5`, the 24-hour
configuration plus a rotated probe token. The container application is at
version 6 with image
`sha256:a73f981c4bcd8460f84aa17178e465cc44d646b554ed3a960ec480425ad0611f`. It
carries committed-gate binary `scry 124e8f2`, SHA-256
`9fbb5b414f7aad4b5f79c22c486b6a88901a445523fce10d664efa61d7a3d02d`.

This release changes grading, not the schema. It fixes a legacy recall
question (one with no authored answer form) that sent "water" for the key
"Water" to self-check. Under this release:
- A legacy or `flexible` recall answer that doesn't match the key goes to the
  Jev `short-v1` check. All 11 production recall questions are legacy.
- An authored `exact` form keeps the local rule.

PRs [187](https://github.com/misty-step/scry/pull/187) and
[188](https://github.com/misty-step/scry/pull/188) made the change. Live Jev
on the reported prompt judged "water" and "H2O" correct and "carbon dioxide"
wrong.

The rollout order followed the MIS-164 rule:
1. Quiesced with the one-minute window.
2. The final backup `scry-20260924T173455…` read back (SHA-256 `68d3d425…`) and
   passed `check`.
3. Deployed `f5cfcb7` (#187), then built and deployed `124e8f2` (#188) while the
   instance stayed inactive.
4. A readiness probe at 17:43:27 cold-started an instance that restored
   `scry-20260924T173455…` on the stale `f5cfcb7` revision, because `124e8f2`
   had not yet propagated. The rollout began shutting that instance down at
   17:43:41; its final backup `scry-20260924T174342…` (key named when the
   snapshot started) was uploaded and verified at 17:43:47, and the Worker
   logged the stop at 17:43:51. At 17:46:15 the next
   instance restored that backup on revision `124e8f2`, and its listener was
   ready at 17:46:16.

Only probe traffic reached `f5cfcb7`. CI now falls back to the GHCR mirror of
the pinned Dagger engine when `registry.dagger.io` fails.

### September 24 Ink notebook interface release (superseded)

This release changes only the interface. The schema is unchanged at 5. The
Worker serves version `9dfb05ff-e829-4258-8ef6-51437760aa95`: the committed
24-hour configuration, deployed as `986d5831…`, plus a probe-token rotation.
The container application is at version 4 with image digest
`sha256:b287bf39e914d80c583002f2964952832c1d53993ce0f304567da7b78eb0bbc0`.
It carries the committed-gate binary `scry 0877687` (PR
[185](https://github.com/misty-step/scry/pull/185)), SHA-256
`81b6d1a84809630ec0abe512da8b7bf1b9ef06180f7918b2000b7f70d2b83ea4`.

The binary was built by `ci:full --require-committed` on an isolated exe.dev
VM. `registry.dagger.io` was returning HTTP 500 at the time, so the pinned
engine `v0.21.6` ran from its `ghcr.io/dagger/engine` mirror.

After the rollout, readiness returned `ready` and health `ok` to a temporary
service identity. The owner root returned 403, a wrong probe token 401, and an
anonymous request was redirected to Access login. The temporary policy and
token were deleted, and the probe token was rotated.

**Rollout race ([MIS-164](https://linear.app/misty-step/issue/MIS-164)).** This
deploy went straight to `--containers-rollout=immediate` without first
quiescing with the one-minute window. The new instance fetched
`scry-20260924T133048.201650106Z-8f378963aa56b9b9c258dc10d40f9bbd.scry-backup.zip`
at 17:14:04.8. The outgoing instance's final backup
`scry-20260924T171400.753013739Z-6a4e374099cf208e85f12e10f4cd744d.scry-backup.zip`
finished uploading 3 seconds later. An independent export diff of the two
snapshots showed that only the backup ledger changed, so no learning write was
lost.

Until MIS-164 adds a structural guard, every image rollout must quiesce first:

1. Deploy the Worker only with the one-minute window.
2. Confirm the instance is `inactive`.
3. Read back its final backup.
4. Only then deploy the image.

Rollback is the previous image `sha256:9e461ab8…fe73` (binary `cdd8212`),
which is schema-5 compatible. Use the same quiesced replacement.

### September 24 concept-centered v5 release

Receipt: [v5-release-20260924.json](qa/v5-release-20260924.json).

Production Worker `scry-app-host` serves version
`0df3cd48-689b-4200-9c92-cc9e62744dd3`. That version is the probe-token
rotation on top of `4786a6c7-cdc7-472f-972e-77e29e2258c9`, the committed
24-hour configuration from master `cdd8212b2bf84c6c23f96fc19ec036d2ad598944`
([PR 182](https://github.com/misty-step/scry/pull/182)). The container
application is at version 3 with image digest
`sha256:9e461ab8d82783c303b46f1b7c23f13ba4c991bd139ed8f6f7dde59ab710fe73`.
It carries the committed-gate binary `scry cdd8212`, SHA-256
`fdb628f92afd6243652c2c150a7a9f6919774716b622b4ece9b66471b6d1c259`, built by
`ci:full --require-committed` on an isolated exe.dev VM. `SCRY_EXA_API_KEY`
is now a production secret binding; the model, semantic, and budget settings
match the committed configuration ($3.50 rolling day, $0.50 reservation). The
provider key's weekly limit read back as $25. The image went straight to
production without a staging cold wake. `Dockerfile`, `entrypoint.sh`, and
`nginx.conf` are unchanged since `b9836cf`.

What was done, in order:

1. Read back the 12:00 UTC snapshot
   `scry-20260924T120041.335564850Z-2587444dece0c8d52f12216e32da8f1e.scry-backup.zip`
   (SHA-256 `5f284af45ed99a18b6eb8713f058ceac4756c3465c0bd2b264164c2b87566053`).
   The v4 binary's `check` passed on a copy.
2. Deployed the Worker only, with the one-minute window, then woke the
   instance with a readiness probe. The v4 instance stopped after verified
   backups; the last was
   `scry-20260924T132001.273153794Z-ad32f0cd61e81a2a909737dea1647a5f.scry-backup.zip`
   (SHA-256 `345c398c3ab884ba0cce350cfd5c8e0a2d551ec714b1da487e131d1b3528c559`).
   Compared with 12:00, only the backup ledger changed.
3. Rehearsed on the VM in unused paths. Both binaries restored the snapshot and
   passed `check`. Separately, on an untouched v4 restore of the rollback-point
   snapshot, v5's read-only `check` reported compatible and left the file
   byte-identical at `user_version=4`. The v5 restore migrated it to `user_version=5`. Every v4
   export section kept every row. The only additions were code-owned catalog
   ids (`dedupe-v1`, `short-v1`, `learner-v1`) and new v5 sections. The
   migration created 13 goals, one per source; the 6 unarchived sources show
   as unmapped on the Map. Old jobs stayed paused. `serve` on the migrated copy
   returned `ready`, and every page (Stream, Add, Map, History, Settings,
   sources, question edits) rendered without template errors.
4. Deployed the image with `--containers-rollout=immediate`. A probe had
   already cold-started a v4 instance, so the rollout stopped it after a
   verified final backup,
   `scry-20260924T132341.741963310Z-6697e710d45033856c844e0a54156288.scry-backup.zip`
   (SHA-256 `4cf452367b9e6232360d316e086d7d10382bcf438b54078b9bc86de6bf672609`).
   That is the last v4 snapshot and the rollback point. It differs from the
   rehearsed snapshot only by one ledger row, and its own rehearsal passed.
   v5 restored and migrated it; the start log shows revision `cdd8212` with
   semantic mode on.
5. Cycled the instance once through the one-minute window. It stopped after
   the verified v5 backup
   `scry-20260924T132931.946091435Z-0202f65a1db001a01a8d0cf474d38fe5.scry-backup.zip`
   (SHA-256 `338c34998aab5bf21df6b0931c4cc14b62e9673c908969cba03372be970b3e29`,
   624,354 bytes). An independent R2 readback passed v5 `check` at schema 5.
   Its export equals the rehearsed migration except the ledger and the
   generated goal ids. The v4 binary refuses it.
6. Redeployed the committed 24-hour configuration. The next cold wake restored
   that v5 snapshot on revision `cdd8212`. Readiness returned `ready`, health
   returned `ok`, the owner root returned 403 to the temporary service
   identity, a wrong probe token returned 401, and an anonymous request
   redirected to Access login. The temporary policy and service token were
   then deleted; the owner-only policy is unchanged. The probe token was
   rotated to a value no one retained.

All requests in the release window were operator probes, so no owner writes
are missing from the rollback point. A rollback to v4 follows the
[failure procedure](#schema-v5-release-boundary): restore that v4 snapshot
into an unused path with the retained `b9836cf` binary (SHA-256
`4c3a326e…1470`), quantify any v5 writes it lacks, and get operator approval.
The previous image digest `sha256:0a59a1b5…3be32` cannot read the v5
snapshots now newest in R2.

Not verified by this release: owner-authenticated flows on production (the
capture modes, a held Stream result with self-check and override, intro,
Map/Concept notes, share target, and the Settings backup status); a live Topic
request using the new Exa binding; a physical-phone pass; and the full
146-shot visual matrix on this revision. The exact binary passed these flows
on the VM and locally with synthetic data and the production model shape, but
that does not prove live production behavior.

### September 23 automatic meaning recall release (superseded)

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

That schema-4 rollback no longer applies. The v4 image cannot read the v5
snapshots now in R2; follow the September 24 rollback path above.

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

v5 was released on 2026-09-24 (see
[the release record](#september-24-concept-centered-v5-release)). The failure
and rollback rules below still apply to v5 production. The v5 release
transactionally migrates complete schema 4 to 5, preserving
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
