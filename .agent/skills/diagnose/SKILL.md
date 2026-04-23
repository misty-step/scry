---
name: diagnose
description: |
  scry-specific diagnosis for bugs, test failures, production incidents,
  Sentry/Vercel/Convex triage, Langfuse cost or quality anomalies, prompt eval
  regressions, and durable issue logging. Reproduce first, prove root cause,
  then fix and verify against the scry gate.
  Use for: "diagnose", "debug", "why is this broken", "production down",
  "is production ok", "Sentry issue", "Convex deploy failed", "migration
  failed", "generation quality dropped", "cost spike", "prompt eval failed",
  "audit", "triage", "log issues".
  Trigger: /diagnose.
argument-hint: <symptoms or domain> e.g. "production health" or "prompt quality regression"
---

# /diagnose

Preserve the story before changing it. Reproduce the failure, trace the layer
that owns it, prove the root cause, fix only that cause, and leave durable
evidence.

## scry Source Of Truth

Use this gate language exactly when diagnosis turns into repair:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Operational invariants:

- `pnpm` only.
- Backend before frontend: schema, Convex query/mutation/action, generated API readiness, then UI.
- Pure FSRS is non-negotiable. A large due queue is not a bug unless the FSRS state, selection query, or interaction recording is wrong.
- Runtime Convex paths must use indexes and bounded `.take()` or pagination. No unbounded `.collect()` in production paths.
- Destructive mutations need a reverse: `archive`/`unarchive`, `softDelete`/`restore`; hard delete requires explicit confirmation UX.

## Routing

| User intent | Route |
| --- | --- |
| Local bug, test failure, type/build failure | Use the four-phase loop below |
| Flaky or intermittent test | `references/flaky-test-investigation.md`, then this file for scry verification |
| Production down, alert, postmortem | `references/triage.md`, plus the scry signal order below |
| Domain audit such as Stripe, auth, quality, cost | `references/audit.md`, then file durable issues |
| Audit then repair one issue | `references/fix.md`, then verify with the scry gate |
| Convert findings into issues | `references/log-issues.md`, with scry issue requirements below |

If the argument names Sentry, Vercel, Convex, Langfuse, production, migration,
deployment, prompt quality, or evals, stay in this file and use the scry signal
order before delegating to a generic reference.

**Symptoms:** $ARGUMENTS

## Iron Law: Reproduce Before Fix

No fix until one of these is true:

- You reproduced the failure locally, in preview, or through a read-only
  production signal.
- You have a concrete external artifact: Sentry issue URL, Vercel deployment
  log, Convex function error/log line, Langfuse trace/report, promptfoo result,
  or user-provided reproduction.
- The issue is not reproducible yet, and you have written the next smallest
  instrumentation or data-gathering step instead of changing behavior.

Never stack experiments. One hypothesis, one discriminating check, one result.

## scry Signal Order

For production or cross-service diagnosis, inspect signals in this order unless
the user supplied a single local stack trace that clearly scopes the problem.

1. **App route:** Read the live code path first. `app/api/health/route.ts`
   defines a Node.js, force-dynamic, no-revalidate health route. `GET` returns
   `createHealthSnapshot()` with `HEALTH_RESPONSE_HEADERS`; failures return
   `503` and `{ status: "unhealthy" }`. `HEAD` returns `X-Health-Status` and
   `X-Health-Timestamp`. This proves the Next route can execute; it does not by
   itself prove Convex generation, review scheduling, or Langfuse health.
2. **Health endpoints:** Check the public endpoint before dashboards:
   `curl -fsS https://scry-o08qcl16e-moomooskycow.vercel.app/api/health` and,
   for uptime probes, `curl -I https://scry-o08qcl16e-moomooskycow.vercel.app/api/health`.
   `/api/performance?action=health` is an authenticated internal monitoring
   endpoint. `scripts/check-deployment-health.sh` validates Convex connectivity,
   critical functions, core schema tables/indexes, and `health:check`, but it
   follows the configured Convex target. State the target before running it.
3. **Sentry:** Use Sentry for Next.js frontend errors, React errors, API route
   errors, build/source-map issues, traces, and error-triggered replays. Convex
   backend errors are not covered by Sentry because Convex runs in V8 isolates.
   Inspect issue details, tags, release, breadcrumbs, user feedback, and
   similar issues. `.github/workflows/sentry-triage.yml` can create GitHub
   issues from `repository_dispatch` payloads with `bug`, `sentry`, and
   `needs-triage` labels.
4. **Vercel:** Check deployments, build logs, function logs, Analytics, Speed
   Insights, Monitoring alerts, and Checks. Uptime checks should cover `/` and
   `/api/health`; Vercel Web Vitals targets are LCP < 2.5s, INP < 200ms, and
   CLS < 0.1. Correlate regressions with the deployment timeline before
   changing code.
5. **Convex dashboard/logs:** Use Convex for backend function failures,
   mutations, queries, actions, performance, schema validation, generation jobs,
   FSRS state, IQC, embeddings, and webhooks. The deployment health script
   expects critical functions: `generationJobs:getRecentJobs`,
   `generationJobs:createJob`, `generationJobs:cancelJob`,
   `aiGeneration:processJob`, `concepts:getDue`, and `health:check`.
6. **Langfuse cost/quality reports:** For generation cost spikes or quality
   degradation, inspect traces and run the repo scripts when credentials are
   available: `pnpm cost:report --days 7 --alert 5` and
   `pnpm quality:report --days 7 --min 3.5`. Relevant code lives in
   `convex/lib/langfuse.ts`, `convex/aiGeneration.ts`, `convex/lib/scoring.ts`,
   `scripts/langfuse-cost-report.ts`, and `scripts/langfuse-quality-report.ts`.
7. **Prompt evals:** For prompt, provider, synthesis, or phrasing changes, run
   `pnpm eval` or a targeted
   `npx promptfoo eval -c evals/promptfoo.yaml --filter "<case>"`, then
   inspect with `pnpm eval:view`. Prompt surfaces are
   `convex/lib/promptTemplates.ts`, `evals/prompts/**`, and
   `evals/promptfoo.yaml`; CI coverage is `.github/workflows/prompt-eval.yml`.

## Production Command Guardrail

Do not run production deploy, migration, rollback, promotion, or env mutation
commands without explicit operator approval in this conversation.

Forbidden without approval:

- `pnpm build:local`
- `pnpm build:prod`
- `pnpm convex:deploy`
- `./scripts/deploy-production.sh`
- `npx convex deploy`
- `vercel --prod`
- `vercel promote <deployment-url> --prod`
- `vercel env add ... production`
- `./scripts/run-migration.sh <name> production`
- Any production migration script or `npx convex run migrations:*` against prod

Read-only production checks are acceptable only when the user asked about
production health or incident response and you state the target first.

## Convex Deployment And Migration Pitfalls

Production Convex deployment is `uncommon-axolotl-639`. The deployment runbook
requires Convex backend first, validation second, Vercel frontend last.

Before any production-adjacent Convex action:

- Verify the target explicitly. Production deploy keys start with `prod:` and
  should resolve to `uncommon-axolotl-639`.
- Do not `source .env.production`; Vercel env files are not shell syntax. The
  documented export pattern is
  `export CONVEX_DEPLOY_KEY=$(grep CONVEX_DEPLOY_KEY .env.production | cut -d= -f2)`.
- Remember `scripts/check-deployment-health.sh` checks the Convex deployment
  configured in `.convex/config`; `NEXT_PUBLIC_CONVEX_URL` is validated but is
  not the deployment selector.
- If a migration "already migrated" but production still fails, suspect a dev
  target such as `amicable-lobster-935` instead of prod.
- Schema removals use the three-phase pattern: make the field optional, deploy,
  dry-run/backfill and verify diagnostics, then remove the field and deploy
  again.
- Migration code must use runtime property checks for removed fields:
  `'fieldName' in (doc as any)`, not `doc.fieldName !== undefined`, because
  TypeScript can erase unreachable checks after schema changes.
- `Object contains extra field` usually means the field was removed from the
  schema before data was migrated. Restore it as optional, migrate, then remove.
- Convex has no instant rollback. Backend rollback means revert and redeploy or
  apply an emergency schema compatibility fix, both requiring approval for prod.

## Four-Phase Debugging Loop

### Phase 1: Root Cause Investigation

1. Read the exact error: stack, line, release, deployment, failing command, and
   environment.
2. Reproduce with the smallest path:
   - Local code/test: run the specific test or command first.
   - Next route/API: hit the route or inspect Vercel/Sentry evidence.
   - Convex: identify the function, table, index, user scope, and deployment.
   - LLM quality/cost: identify trace, prompt version, model, phase, and eval.
3. Check recent changes with `git diff` and `git log --oneline -10`.
4. Trace data flow backward to the owner layer: UI route, API route, Convex
   function, schema, external provider, or prompt/eval input.
5. Write down the single current hypothesis as "X causes Y because Z".

### Phase 2: Pattern Analysis

1. Find a working example in this repo and read it fully.
2. Compare boundaries, not just syntax: auth context, Convex index, bounded
   query, generated API type, schema validator, env var, prompt contract, and
   feature flag.
3. For FSRS behavior, inspect state transitions and interaction recording
   before questioning the algorithm.
4. For generation/IQC issues, compare prompt templates, Langfuse traces,
   scoring output, generated concepts/phrasings, and promptfoo fixtures.

### Phase 3: Hypothesis Test

Use one discriminating experiment:

- Local code: write or run the narrowest failing Vitest/Playwright case.
- Convex contract: use `pnpm test:contract` or a local/dev Convex check.
- Health: compare `/api/health`, Vercel logs, and Convex `health:check`.
- Sentry: confirm release, culprit, breadcrumbs, and affected users.
- Langfuse: compare cost by model/phase and quality score trend.
- Prompt: reproduce with `pnpm eval` or a targeted promptfoo filter.

Classify each result:

- **Disproved:** eliminate the hypothesis and choose a new one.
- **Supported:** design the next smallest check to prove the causal chain.
- **Ambiguous:** the experiment was too broad; narrow it.

### Phase 4: Fix And Verify

1. Write the failing test or artifact first.
2. Verify it fails for the right reason.
3. Make one root-cause fix. Do not refactor adjacent non-broken code.
4. Run targeted verification first, then the scry gate required by the change:
   - Default local parity: `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`.
   - `convex/**`: add `pnpm test:contract`.
   - Build/config/workflow/dependency surfaces: add `pnpm build`.
   - Dependency or lockfile changes: add `pnpm audit --audit-level=critical`.
   - Prompt/generation changes: add `pnpm eval` or targeted promptfoo evals,
     plus Langfuse report checks when diagnosing live quality/cost.
5. Demand observable proof: passing test, clean health endpoint, resolved Sentry
   issue, Convex log absence after reproduction, schema diagnostic, Langfuse
   report, or promptfoo result.

If two fixes fail, stop editing. Re-read the failing file and this diagnosis,
then delegate fresh-context investigation or ask for the missing production
artifact.

## Durable Issue Logging

File a concrete GitHub issue or backlog item for every durable problem:

- P0/P1 production issue, data loss/corruption risk, auth/payment breakage, or
  incident follow-up.
- Recurring Sentry issue, alert noise, missing dashboard, missing runbook, or
  observability blind spot.
- Convex migration/deploy hazard, schema compatibility gap, missing diagnostic,
  or missing bounded query/index.
- Langfuse cost anomaly, quality degradation, missing prompt regression case, or
  eval coverage gap.
- Any fix intentionally deferred because it is larger than the current incident.

Issue template:

```markdown
Title: [P<0-3>] [scry component] concise failure

Evidence:
- Symptom:
- Reproduction or artifact:
- Affected users/scope:
- First seen / last seen:

Root cause or leading hypothesis:

Acceptance criteria:
- Reproduction fails before the fix and passes after.
- Observable production or CI signal confirms resolution.
- Required scry gate/additive checks pass.
```

For Sentry-created work, preserve the Sentry URL, issue id, event count,
affected users, culprit, first seen, and last seen from
`.github/workflows/sentry-triage.yml`.

## Incident Work Log

For non-trivial production incidents, keep a local `INCIDENT-<timestamp>.md`
or GitHub issue timeline with:

- Timeline in UTC.
- Evidence checked in the scry signal order.
- Ranked hypotheses and result of each experiment.
- Actions taken and what each action proved.
- Root cause and fix.
- Verification and 30-minute post-fix monitoring result.
- Follow-up GitHub/backlog issues.

## Delegation Pattern

Lead model owns hypothesis ranking, root-cause declaration, production command
approval boundaries, and fix selection.

Delegate fresh-context probes when:

- Multiple plausible layers exist: Next route, Vercel, Convex, Langfuse, prompt
  evals.
- More than three tool calls would be exploratory.
- You are reviewing a fix you just wrote.

Prompt investigators with one subsystem, exact files, target evidence, and a
report shape: confirmed/disproved, evidence, next experiment, and no code edits.

## Output

End every `/diagnose` run with:

- **Root cause:** the proven causal chain, or **UNVERIFIED** with the missing
  evidence.
- **Fix:** what changed, or why no change was made.
- **Verification:** exact commands, dashboards, traces, endpoints, or issue
  links proving the outcome.
- **Durable record:** GitHub issue, backlog item, Sentry resolution, or
  incident log created for anything that must survive the session.
