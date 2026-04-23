---
name: deploy
description: |
  Deploy scry to its real production surface: Convex backend first,
  validation second, Vercel frontend last. This skill is approval-gated
  for every command that can mutate non-local Convex or Vercel state.
  It stops at a healthy deploy receipt and hands ongoing observation to
  /monitor.
  Use when: "deploy scry", "ship to production", "release", "run the
  production deploy", "deploy this merged ref".
  Trigger: /deploy, /ship-it, /release.
argument-hint: "[prod|dev] [--version <ref>] [--manual] [--rollback]"
---

# /deploy

Deploy scry. Do not perform generic platform detection here: this repo is
Next.js on Vercel with Convex as the backend.

The required order is:

1. Convex backend
2. Validation
3. Vercel frontend
4. Health receipt
5. Handoff to `/monitor`

Production Convex target: `uncommon-axolotl-639`.
Development Convex target: `amicable-lobster-935`.

If a production deploy logs or resolves to `amicable-lobster-935`, abort.
That is the development target, not production.

## Approval Boundary

An invocation of `/deploy` is intent, not approval. Before running any
deploying or state-mutating command, ask for explicit operator approval
that names the command and target. Example:

```text
Approve running ./scripts/deploy-production.sh against production Convex
uncommon-axolotl-639 and Vercel production?
```

Do not run these without explicit approval in this repo:

- `./scripts/deploy-production.sh`
- `pnpm build:local`
- `pnpm build:prod`
- `pnpm convex:deploy`
- `npx convex deploy`
- `vercel --prod`
- `vercel promote`
- `npx convex env set`
- production migration scripts, including `./scripts/run-migration.sh`
- any command that changes non-local Convex or Vercel state

Read-only validation may run before approval: `git`, `gh` inspection,
`pnpm lint`, `pnpm tsc --noEmit`, `pnpm test:ci`,
`pnpm test:contract`, `pnpm build`, and read-only health checks.

## Pre-Deploy Gate

The repo brief is the pre-deploy contract:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Do not deploy an unverified ref. Confirm the GitHub Actions
`Quality Checks` `merge-gate` is green for the exact SHA, then run local
parity if the operator asks for local evidence or the CI signal is not
available.

If `convex/**` changed, `pnpm test:contract` is required. If
`package.json`, `pnpm-lock.yaml`, `vercel.json`, `next.config.ts`, or
`.github/workflows/**` changed, `pnpm build` is required. If dependencies
or the lockfile changed, `pnpm audit --audit-level=critical` is required.

## Source of Truth

Use these live files over older prose:

- `package.json` scripts
- `vercel.json`
- `scripts/vercel-build.sh`
- `scripts/deploy-production.sh`
- `scripts/check-deployment-health.sh`
- `docs/runbooks/production-deployment.md`

`docs/operations/deployment-checklist.md` contains useful operational
history, but it also references scripts such as `pnpm convex:deploy:dev`,
`pnpm convex:deploy:prod`, and `pnpm convex:codegen:prod` that are not in
the current `package.json`. Do not run scripts that do not exist.

## Target Verification

Never `source .env.production`. The production env file is Vercel-style
configuration, not guaranteed shell syntax. Parse only the keys needed for
the command.

For production, load and verify the deploy key like this:

```bash
export CONVEX_DEPLOY_KEY=$(grep '^CONVEX_DEPLOY_KEY=' .env.production | cut -d= -f2-)
export NEXT_PUBLIC_CONVEX_URL=$(grep '^NEXT_PUBLIC_CONVEX_URL=' .env.production | cut -d= -f2-)

target=$(printf '%s' "$CONVEX_DEPLOY_KEY" | cut -d: -f2 | cut -d'|' -f1)
test "$target" = "uncommon-axolotl-639"
printf '%s' "$CONVEX_DEPLOY_KEY" | grep -q '^prod:'
test "$NEXT_PUBLIC_CONVEX_URL" = "https://uncommon-axolotl-639.convex.cloud"
```

For development, verify `amicable-lobster-935`. Do not use
`./scripts/deploy-production.sh` for development. If the operator asks for
a dev deploy, require the same explicit approval boundary and verify the
target before mutation.

## Production Protocol

### 1. Pin the ref

Resolve the exact SHA:

```bash
git rev-parse --verify HEAD
git status --short
```

If there are uncommitted app changes, stop unless the operator explicitly
confirms the deploy target is a committed SHA and the dirty worktree is
irrelevant. Do not deploy speculative local edits.

### 2. Verify the gate

Confirm GitHub Actions `Quality Checks` `merge-gate` is green for the
SHA. Local parity commands are:

```bash
pnpm lint
pnpm tsc --noEmit
pnpm test:ci
```

Add checks by touched surface:

```bash
pnpm test:contract
pnpm build
pnpm audit --audit-level=critical
```

Use only the applicable additive checks from the pre-deploy gate above.

### 3. Check Convex migration risk

If `convex/schema.ts` removes or tightens fields, do not deploy directly.
Use the three-phase Convex migration pattern:

1. make the field optional and deploy compatible backend
2. run dry-run and approved migration, then diagnostics
3. remove the field and deploy the clean schema

Production migrations require their own explicit approval. The dry-run
count and the actual mutation count must match.

### 4. Run the production deploy

Preferred path, after explicit approval:

```bash
export CONVEX_DEPLOY_KEY=$(grep '^CONVEX_DEPLOY_KEY=' .env.production | cut -d= -f2-)
export NEXT_PUBLIC_CONVEX_URL=$(grep '^NEXT_PUBLIC_CONVEX_URL=' .env.production | cut -d= -f2-)
./scripts/deploy-production.sh
```

What this script does, in order:

- requires `CONVEX_DEPLOY_KEY`
- deploys Convex with `npx convex deploy --env-file .env.production`
- runs `./scripts/check-deployment-health.sh`
- deploys Vercel with `vercel --prod`
- exits on the first failed step

This preserves scry's backend-first invariant: the frontend must not ship
until the Convex backend is deployed and healthy.

### 5. Manual hotfix path

Use only when the operator rejects the script or asks for a manual
hotfix. Each mutating command still needs explicit approval.

```bash
export CONVEX_DEPLOY_KEY=$(grep '^CONVEX_DEPLOY_KEY=' .env.production | cut -d= -f2-)
export NEXT_PUBLIC_CONVEX_URL=$(grep '^NEXT_PUBLIC_CONVEX_URL=' .env.production | cut -d= -f2-)

target=$(printf '%s' "$CONVEX_DEPLOY_KEY" | cut -d: -f2 | cut -d'|' -f1)
test "$target" = "uncommon-axolotl-639"
printf '%s' "$CONVEX_DEPLOY_KEY" | grep -q '^prod:'
test "$NEXT_PUBLIC_CONVEX_URL" = "https://uncommon-axolotl-639.convex.cloud"
```

Then, with approval for each mutating step:

```bash
npx convex deploy --yes --cmd-url-env-var-name UNSET
./scripts/check-deployment-health.sh
vercel --prod
```

Do not run `vercel --prod` if the Convex health check fails.

### 6. Post-deploy health

After Vercel returns a deployment URL, validate:

```bash
./scripts/check-deployment-health.sh
npx convex run diagnostics:validateSchemaConsistency
curl -fsS https://scry-o08qcl16e-moomooskycow.vercel.app/api/health
```

Expected health:

- `scripts/check-deployment-health.sh` finds critical Convex functions:
  `generationJobs:getRecentJobs`, `generationJobs:createJob`,
  `generationJobs:cancelJob`, `aiGeneration:processJob`,
  `concepts:getDue`, and `health:check`.
- `health:check` reports all required environment variables present.
- `diagnostics:validateSchemaConsistency` returns
  `{ "healthy": true, "issues": [] }`.
- The production API health endpoint returns 2xx.

Also perform the runbook's manual smoke: visit the production URL, check
the browser console, test generation, test reviews, and confirm no schema
version error modal appears.

## Vercel Build Behavior

`vercel.json` uses:

```json
{
  "buildCommand": "./scripts/vercel-build.sh",
  "devCommand": "pnpm dev",
  "installCommand": "pnpm install",
  "framework": "nextjs",
  "outputDirectory": ".next"
}
```

During a Vercel build, `scripts/vercel-build.sh` requires
`CONVEX_DEPLOY_KEY`, infers routing from the key type, runs
`npx convex deploy --cmd 'pnpm build'`, and lets Convex set
`NEXT_PUBLIC_CONVEX_URL` for the build command.

Key routing:

- `prod:` deploy key routes to production backend
  `uncommon-axolotl-639`.
- `preview:` deploy key creates a branch-named isolated backend.

If Vercel build logs mention the wrong Convex target, stop and diagnose
the deploy key before retrying.

## Receipt

End `/deploy` by reporting a concise receipt:

```json
{
  "repo": "scry",
  "sha": "<full sha>",
  "env": "production",
  "convex_target": "uncommon-axolotl-639",
  "vercel_url": "<url returned by vercel>",
  "health": "healthy",
  "commands_run": ["<commands, excluding secret values>"],
  "handoff": "/monitor"
}
```

Do not include secret values. Redact deploy keys and environment values.

## Deploy vs Monitor

`/deploy` stops when the deployed ref is healthy and the receipt is
complete. It may run bounded health checks, but it does not tail logs
indefinitely, watch metrics, decide rollback from later anomalies, or
triage incidents.

After a healthy receipt, hand off to `/monitor` with the SHA, Vercel URL,
Convex target, and health evidence. `/monitor` owns ongoing log review,
anomaly detection, and rollback recommendations.

If post-deploy health fails, do not call it monitoring. Stop the deploy,
emit the failed health evidence, and route to `/diagnose` or an explicit
rollback path.

## Rollback

Rollback is also state mutation and requires explicit approval.

Vercel frontend rollback:

```bash
vercel ls
vercel promote <deployment-url> --prod
```

Convex has no instant rollback. For backend failures, either revert the
bad commit and redeploy through this same protocol, or deploy an emergency
schema compatibility fix. Schema-validation failures usually require
adding the removed field back as optional, deploying, then rerunning the
proper migration sequence.

## Gotchas

- Do not `source .env.production`; parse keys with `grep` and verify the
  target.
- `uncommon-axolotl-639` is production. `amicable-lobster-935` is dev.
- `./scripts/check-deployment-health.sh` requires
  `NEXT_PUBLIC_CONVEX_URL` in the shell and checks the Convex deployment
  configured by the CLI.
- `pnpm build:prod` and `pnpm build:local` deploy Convex as part of the
  build path; they are approval-gated.
- The frontend must not deploy if backend health fails.
- The operations checklist contains historical commands; current scripts
  in `package.json` win.
- Preview behavior follows the live `vercel.json` and
  `scripts/vercel-build.sh`, not stale references to
  `scripts/vercel-build.cjs`.
- Never paste or log a full deploy key.

## Related

- `docs/runbooks/production-deployment.md`
- `docs/operations/deployment-checklist.md`
- `scripts/deploy-production.sh`
- `scripts/check-deployment-health.sh`
- `scripts/vercel-build.sh`
- `vercel.json`
- `/monitor` for post-healthy observation
- `/diagnose` for failed health or incident triage
