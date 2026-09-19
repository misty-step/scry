# Scry Cloudflare hosting (exe.dev retirement)

Standing policy: `daybook/meetings/decisions/2026-09-18-hosting-and-autonomy.md`
— public/private product Scry runs on Cloudflare, not exe.dev. Private
preview uses Cloudflare Access, not exe.dev login.

## Shape

```
scry.study (Cloudflare edge)
  └─ Worker scry-app-host (index.mjs)
       ├─ Cloudflare Access JWT gate (Cf-Access-Jwt-Assertion, jose JWKS)
       ├─ probe token: /healthz + /readyz only
       └─ Durable Object ScryContainer (container.mjs)
            └─ single container instance (Dockerfile)
                 ├─ nginx :8080 — local ingress, sets X-ExeDev-UserID,
                 │   strips all client authority headers, loopback peer
                 └─ scry serve 127.0.0.1:8081 (exact operator release binary)
                      ├─ wake: restore latest R2 snapshot via scry-go-backups
                      └─ SIGTERM: final `backup --require-remote`
```

One container instance, one writer — the same contract as the single VM.
Container disk is ephemeral; the private R2 sink (`scry-go-recovery`
through the `scry-go-backups` gateway) is the durable store.

## Files

- `Dockerfile` — alpine + nginx + the release binary. The binary is NOT
  committed: copy it from the operator release into this directory before
  `wrangler deploy` (see below). `deploy/cloudflare-hosting/scry` is
  git-ignored.
- `entrypoint.sh` — restore-on-wake, serve, final verified backup on stop.
- `nginx.conf` — ingress template; `__SCRY_OWNER_ID__` is substituted at
  boot. nginx sets the owner identity header, never accepts it.
- `index.mjs` — edge Worker: Access JWT validation + probe token.
- `container.mjs` — Container DO: passes app env at start, resolves the
  latest snapshot key from the R2 binding before each cold start.
- `wrangler.jsonc` — `staging` (workers.dev), `mistystep-prod`
  (scry.mistystep.io), `production` (scry.study + www + mistystep.io).

## Secrets (wrangler secret put, piped — never flags)

- `SCRY_OWNER_ID`, `SCRY_SECRET` — exact production values (VM env).
- `SCRY_BACKUP_REMOTE_TOKEN` — the container's gateway token
  (`SCRY_CONTAINER_BACKUP_TOKEN` secret on `scry-go-backups`).
- `SCRY_PROBE_TOKEN` — operations liveness/readiness bearer.
- `SCRY_ACCESS_AUD` / vars — after the Access application is created.

## Deploy

```sh
# 1. Copy the exact release binary into the build context and verify it:
scp scry-app.exe.xyz:/opt/scry/current/scry ./scry
sha256sum ./scry   # must equal the release receipt

# 2. Staging boot proof (synthetic env, no real data):
wrangler deploy --env staging
wrangler secret put SCRY_OWNER_ID --name scry-app-host-staging   # etc.

# 3. Production attach, one hostname at a time:
wrangler deploy --env mistystep-prod   # then --env production
```

The container image build uploads to Cloudflare's registry on first deploy
(several minutes); later deploys reuse the pushed layers.

## Boundaries

- Model generation is paused in this move: `scry-model.int.exe.xyz` is an
  exe-internal link-local endpoint unreachable from Cloudflare. The empty
  `SCRY_MODEL_ENDPOINT` leaves the generation worker idle. Resume requires
  an explicit approved endpoint; nothing auto-retries.
- Never point `SCRY_BASE_URL` at a host that is not attached to this
  Worker, and never widen `SCRY_REDIRECT_HOSTS` beyond real aliases.
- The R2 sink stays append/read-only. Restore-on-wake and final-backup are
  the only container-initiated writes, both through the gateway contract.
- Keep the release binary, SHA-256, and private configuration outside
  Cloudflare (existing operator custody) for independent recovery.
