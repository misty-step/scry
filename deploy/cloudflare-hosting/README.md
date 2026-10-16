# Scry Cloudflare hosting (exe.dev retirement)

Standing policy: `daybook/meetings/decisions/2026-09-18-hosting-and-autonomy.md`
— public/private product Scry runs on Cloudflare, not exe.dev. Private ingress
uses Cloudflare Access; the container does not accept public identity headers.

## Shape

```text
scry.study (Cloudflare edge)
  └─ Worker scry-app-host (index.mjs)
       ├─ Cloudflare Access JWT gate (jose + Access JWKS)
       ├─ immutable owner-subject check
       ├─ probe token: GET/HEAD /healthz and /readyz only
       └─ Durable Object ScryContainer (container.mjs)
            └─ one container instance (Dockerfile)
                 ├─ nginx :8080 — strips client authority headers and sets
                 │  the fixed X-ExeDev-UserID for the application
                 └─ scry serve 127.0.0.1:8081 (exact release binary)
                      ├─ cold start: restore latest complete R2 snapshot
                      └─ graceful stop: attempt final `backup --require-remote`
```

One named container instance preserves the application's single-writer contract.
Its disk is ephemeral. The private R2 sink (`scry-go-recovery`, reached through
`scry-go-backups`) is the durable store. A graceful-stop backup is only an
opportunity: crash, SIGKILL, host loss, or failed egress can lose writes newer
than the last remotely verified snapshot.

## Files

- `Dockerfile` — Alpine, nginx, and the exact release binary. The binary is not
  committed; copy the gate-produced artifact to `deploy/cloudflare-hosting/scry`
  immediately before deployment. That path is ignored by Git.
- `entrypoint.sh` — fail-closed restore, application-listener readiness, serve,
  and graceful-stop backup.
- `nginx.conf` — local ingress template. It strips client authority headers and
  injects the fixed application owner identity.
- `index.mjs` / `auth.mjs` — Access JWT, immutable owner subject, and probe gate.
- `container.mjs` — singleton Container Durable Object and R2 snapshot selection.
- `runtime-env.mjs` — complete bounded environment validation.
- `wrangler.jsonc` — isolated `staging`, pre-cutover `mistystep-prod`, and final
  `production` environments. Named-environment bindings are intentionally
  repeated because Wrangler does not inherit them.

## Private configuration

Set secret values with `npx wrangler secret put ... --env <environment>` and
provide values through stdin. Never place them in flags, source, logs, or the
container image.

Worker-only secrets:

- `SCRY_ACCESS_OWNER_SUB` — the one immutable Cloudflare Access user UUID. The
  Access policy's email allow-list is not sufficient by itself; a JWT with the
  same email and a different `sub` remains denied.

- `SCRY_PROBE_TOKEN` — grants only GET/HEAD access to `/healthz` and `/readyz`
  after the outer Access application has admitted the request.

The `SCRY_ACCESS_TEAM_DOMAIN` values in `wrangler.jsonc` name the shared
Cloudflare Access team `misty-step-pantry.cloudflareaccess.com`. This is the
JWT issuer/JWKS authority and may appear during the Access login handoff; it is
not a Pantry Worker or Scry application origin. Production requests still use
`https://scry.study`; successful authentication should return to that app
origin.

Container secrets:

- `SCRY_OWNER_ID` and `SCRY_SECRET` — fixed application identity and signing
  secret. Staging uses synthetic values, never values copied from production.
- `SCRY_BACKUP_REMOTE_TOKEN` — the isolated container's append/read capability;
  set the same value as `SCRY_CONTAINER_BACKUP_TOKEN` on the matching backup
  gateway Worker.
- `SCRY_MODEL_API_KEY` — the Scry-only provider credential for recovered/live
  environments, shared with Jev when its dedicated key is empty.
- `SCRY_EXA_API_KEY` — optional private Exa credential for explicit Topic web
  research and chosen Link contents. Never put it in a plain Wrangler var.
  Absent key means Topic research yields zero documents and planning continues
  on labeled general knowledge, while Link captures fail recoverably; it never
  authorizes another provider or search of pasted text.

The Access application audience is not confidential, but it is deployment-specific;
inject `SCRY_ACCESS_AUD` as a Worker secret binding after creating each Access
application rather than committing an environment identifier. An absent audience
or owner subject fails closed.

## Generation and Exa contract

Synthetic staging disables all four model settings together. Recovered/live
hosting uses Scry's configurable HTTPS chat-completions path:
`SCRY_MODEL_ENDPOINT`, `SCRY_MODEL_API_KEY`, one explicit `SCRY_MODEL`, and
`SCRY_MODEL_PROVIDER` (`openrouter` or `openai`). The Scry personal (exe.dev)
provider key limit was raised on 2026-09-23 to **$25/week**. The separate
application allowance is **$3.50 per rolling day** (`3500000` micros), with
**$0.50** conservative reservation (`500000` micros) per generation attempt.
Known costs replace reservations; unknown sent outcomes retain them.

`runtime-env.mjs` forwards optional `SCRY_EXA_API_KEY` (32–4096 characters,
without control characters) and `SCRY_EXA_ENDPOINT` (optional HTTPS origin)
only to the app Container, never scheduled `backup` exec. An empty endpoint
uses `https://api.exa.ai` inside generation. Topic alone initiates web search;
Link retrieves the selected page, Photo transcribes, and pasted My text is
never searched. Keep key values in private secret bindings; the endpoint may
be a reviewed plain var if a different HTTPS origin is required. No redirect
or implicit search fallback broadens the private-data boundary.

## Semantic contract

Only `production` sets the Jev Decisions vars: `SCRY_SEMANTIC_ENDPOINT`
(an HTTPS URL ending in `/api/alpha/decisions`), one explicit
`SCRY_SEMANTIC_MODEL`, and a positive `SCRY_SEMANTIC_RESERVATION_MICROS` no
larger than the daily allowance. An empty `SCRY_SEMANTIC_API_KEY` reuses
`SCRY_MODEL_API_KEY`. With no endpoint, semantic/short checks do not send.
Partial or unsafe settings fail closed. These settings serve short-v1 flexible
recall, rubric semantic-v1 prose, dedupe, and the prepublication critic; no
configuration switch licenses liberal similarity grading.
New values reach the application only in a new Container instance; see
[the runbook](../../docs/runbook.md#schema-v5-release-boundary).

## Exact-binary isolated staging

No production data or live model credential belongs in this flow.

1. Commit the candidate and run the committed-source gate:

   ```sh
   bun run ci:full -- --out target/ci-release --require-committed
   ```

   Inspect `proof.json`, checksums, revision, and `scry version`. Copy that same
   tested binary to `deploy/cloudflare-hosting/scry`; never rebuild it.
2. Create the dedicated `scry-go-recovery-staging` bucket and deploy
   `deploy/backup-gateway` with `--env staging`. Give it only the task-owned
   `SCRY_CONTAINER_BACKUP_TOKEN`.
3. With the exact binary, create a fresh authored synthetic database and publish
   one `backup --require-remote` snapshot to the staging gateway. Retain the
   reported immutable key, archive SHA-256, remote readback, schema, and binary
   identity.
4. Create the staging Access application for
   `scry-app-host-staging.misty-step.workers.dev`, allow the intended account,
   and configure its audience plus the exact immutable owner subject.
5. Set synthetic application, probe, and backup secrets; leave model settings
   absent. Deploy only `npx wrangler deploy --env staging`.
6. Prove anonymous denial, exact-owner ingress, restored application state,
   canonical-host behavior, remote backup readback, and a real cold wake after
   the one-minute staging idle window. Bind the receipt to the committed source,
   gate artifact SHA-256, Worker version, and container image digest.

Retain the staging resource identifiers and exact cleanup handles with the
evidence. Do not delete evidence fixtures as part of review acceptance.

## Production boundary

### Go edge runtime errors

`telemetry.mjs` observes unexpected fetch exceptions and upstream 5xx responses
at this operated Go edge. Its fixed, allowlisted Sentry envelope deliberately
uses no automatic request/error collectors: learner content, raw errors and
credentials never enter its interface. The public DSN belongs to the existing
Scry project. Native version metadata identifies the exact deployed release.
Reporting is background-only, with a two-second deadline, no retries and at
most one event per minute per isolate; it is not uptime/cron coverage or a
global traffic quota. Sentry's existing first-seen workflow owns alert routing.

Use Node 22.22.0 and the root lockfile on the authorized build host:

```sh
npm ci --ignore-scripts --no-audit --no-fund
node --test deploy/cloudflare-hosting/*.test.mjs
./node_modules/.bin/wrangler deploy --config deploy/cloudflare-hosting/wrangler.jsonc --env production --dry-run --containers-rollout none --outdir "$PWD/target/edge-release"
node qa/edge-runtime.mjs
```

The `Go edge errors` CI workflow exercises these exact module bytes locally,
then in two run-scoped disposable Workers with a synthetic upstream. It reads
back hosted bytes and success/denial/failure/recovery, then deletes both Workers.
The upstream has a Container-enabled namespace: native version promotion,
distinct-code rollback and re-promotion must retain both its versioned attachment
and runtime `ctx.container`. No application/image is created or instance started.
Retrieve the matching Sentry event and approved-intake notification separately;
a hosted 503 alone is not delivery evidence. No owner session or learner data
is used. On master, CI promotes the same tested module only to the still-Go
edge, preserving all existing bindings, images, routes, schedules and data.
Version upload copies the live Container attachment separately from DO bindings
and reads it back before promotion; rollback must preserve that attachment too.
It refuses a newer/divergent source or changed cutover boundary. Failed release
readback restores only its own still-active prior code. The previous UUID in
the production receipt is also the compatible code-only rollback target:

```sh
gh workflow run go-edge.yml -R misty-step/scry --ref master -f rollback_version=PREVIOUS_UUID
```

Retire this Go-specific workflow and boundary with the Go runtime; do not use
it to deploy the Rust replacement. It grants no learner cutover authority.

`mistystep-prod` and `production` are source definitions, not authorization to
deploy. Do not attach a custom domain, change DNS, copy the production database,
restart the existing service, or activate production until a separate cutover
stage has a fresh verified snapshot, write pause/drain, product credentials,
Access application/audience, rollback plan, and explicit approval. Missing
configuration must stop the deployment or boot; it never authorizes a substitute.
