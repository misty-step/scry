---
name: monitor
description: |
  Scry post-deploy signal watch. Observe health, Sentry, Vercel,
  Convex, Langfuse, prompt eval, and Playwright surfaces through a
  bounded grace window. File crisp alerts, then hand incidents to
  /diagnose. Watcher, not fixer.
  Use when: "monitor signals", "watch the deploy", "is the deploy ok",
  "post-deploy watch", "watch production", "check Scry after deploy".
  Trigger: /monitor.
argument-hint: "[<deploy-url-or-run-ref>] [--grace <duration>] [--preview]"
---

# /monitor

Watch Scry after a deployment. Preserve the story of what the system is
doing, but do not rewrite it mid-watch.

`/monitor` observes deployed behavior and files an actionable alert when
signals trip. `/diagnose` owns incident analysis and fixes. Do not patch
code, tune thresholds, roll back, run deploy commands, or "just check one
more thing" after a terminal result.

## Scry Contract

Emit exactly one terminal outcome:

- `monitor.done`: Scry stayed green through the grace window.
- `monitor.alert`: at least one confirmed signal tripped; file or link the
  GitHub/backlog alert and hand off to `/diagnose`.
- `phase.failed`: monitoring itself could not run because credentials,
  dashboards, or URLs were unavailable.

Exit code semantics:

- `0`: clean grace window.
- `2`: alert filed; this is an escalation, not tool failure.
- `1`: monitor tooling failed before it could judge the deploy.

## Inputs

| Input | Source | Default |
|---|---|---|
| Deploy target | positional URL, Vercel deployment URL, or GitHub Actions run | production `https://scry.vercel.app` when absent |
| Grace window | `--grace` | 30 minutes for production, 10 minutes for preview |
| Health mode | `--preview` or preview URL detection | preview URLs use `/api/health/preview`; production uses `/api/health` first |
| Poll interval | built-in cadence | 60 seconds for health, dashboard checks at start/mid/end |

The monitoring setup guide says post-deployment monitoring should run for
30 minutes after production deployment. Keep the window bounded: the
interval decides retrieval; a watcher that never stops becomes noise.

## Signal Surfaces

### 1. Health Endpoints

Public health is the first hard gate:

```bash
curl -fsS "$DEPLOY_URL/api/health"
curl -fsSI "$DEPLOY_URL/api/health"
```

Expected production JSON comes from `app/api/health/route.ts` and
`lib/health.ts`: HTTP 200, `status: "healthy"`, no-store headers,
timestamp, uptime, memory, environment, and version.

Preview health is deeper:

```bash
curl -fsS "$PREVIEW_URL/api/health/preview"
```

`/api/health/preview` checks environment detection, `VERCEL_URL`, Convex
connection, Convex schema compatibility, Google AI key functionality,
session creation, and embedding coverage. Treat `status: "error"` as a
hard trip. Treat `status: "warning"` as an alert when it names schema,
Google AI, or embeddings, because preview deployments currently share the
production Convex backend and those problems can affect real data.

### 2. Sentry

Use Sentry for frontend, Next.js API route, build/source-map, performance
trace, and error-triggered replay signals. The runbook thresholds are the
operational defaults:

- New unresolved production issue after the deploy: alert.
- Error rate above 10 events/hour: alert.
- High frequency above 50 events/hour or auth/payment/data loss symptoms:
  P0/P1 alert.
- Crash-free release health below 98%: alert.

Sentry can dispatch `sentry-issue` events into
`.github/workflows/sentry-triage.yml`, which creates GitHub issues labeled
`bug`, `sentry`, and `needs-triage`. Prefer linking that issue when it
already exists; otherwise file one manually with the Sentry issue URL,
count, affected users, release, culprit, and first/last seen timestamps.

### 3. Vercel

Check Vercel immediately after deploy and again at the end of the grace
window:

- Deployments: target deployment is ready, assigned to the expected
  production or preview URL, and no newer failed deployment supersedes it.
- Analytics: page views are not collapsing relative to the pre-deploy
  baseline.
- Speed Insights/Core Web Vitals: p75 LCP under 2.5s, INP under 200ms,
  CLS under 0.1, and no obvious post-deploy regression.
- Function/API timing: no p95/p99 spike for API routes, especially health,
  Stripe, auth, generation, and webhook endpoints.

Vercel is a signal source, not a remediation console. Do not redeploy or
roll back from `/monitor`.

### 4. Convex Dashboard And Logs

Convex owns Scry's schema, queries, mutations, actions, FSRS state,
generation jobs, embeddings, IQC, and subscription webhooks. Monitor the
Convex dashboard/logs for:

- Function errors in mutations, queries, and actions after the deploy.
- Spikes in execution time or database operations.
- Schema mismatch messages surfaced by `/api/health/preview`.
- Generation, embedding, or IQC failures that would corrupt the learning
  loop even if the Next.js shell is healthy.

Convex runtime errors do not reliably flow through Sentry; the runbook is
explicit that backend errors remain in Convex dashboard for troubleshooting.

### 5. Langfuse Cost And Quality

Scry's LLM behavior is observable through Langfuse reports. Run these when
the deploy touches generation, prompt templates, model configuration,
evals, IQC, embeddings, or any LLM-facing Convex action:

```bash
pnpm cost:report --days 1 --alert 5
pnpm quality:report --days 1 --min 3.5
```

The scripts require `LANGFUSE_SECRET_KEY`, `LANGFUSE_PUBLIC_KEY`, and
optionally `LANGFUSE_HOST` (default `https://us.cloud.langfuse.com`).
`scripts/langfuse-cost-report.ts` reports total cost by model, phase, and
day; `scripts/langfuse-quality-report.ts` reports average score, score
distribution, score types, daily trend, and low-score trace IDs.

The scheduled `.github/workflows/llm-ops-monitor.yml` stores
`llm-ops-reports-<run_number>` artifacts containing `cost-report.txt` and
`quality-report.txt`. A cost or quality `ALERT` is a monitor alert; trace
IDs are evidence for `/diagnose`, not a request to tune prompts here.

### 6. Prompt Evals

Prompt behavior is guarded by promptfoo:

```bash
pnpm eval
pnpm eval:view
```

`.github/workflows/prompt-eval.yml` runs on changes to
`convex/lib/promptTemplates.ts`, `evals/**`, and
`.claude/skills/langfuse-prompts/**`. It uploads `promptfoo-results`
containing `eval-results.json`, comments on PRs with pass rate, LLM score,
and failures, and treats failures as non-blocking workflow signal.

For `/monitor`, a failing prompt eval after prompt or generation changes is
an alert even if the GitHub workflow remains green. File the failed cases
and artifact link; `/diagnose` decides whether the fix is prompt,
schema/contract, model, or evaluator.

### 7. Playwright And Nightly Artifacts

Use E2E artifacts as user-journey evidence:

- `pnpm test:e2e:smoke` covers smoke scenarios locally.
- `.github/workflows/preview-smoke-test.yml` checks preview `/api/health`
  and uploads `playwright-preview-smoke-artifacts`.
- `.github/workflows/nightly-e2e.yml` runs daily at 06:00 UTC against
  `https://scry.vercel.app`, runs `pnpm test:coverage`, runs the full
  Playwright suite with HTML reporter, and uploads `coverage-nightly` and
  `playwright-nightly` artifacts (`playwright-report/**` and
  `test-results/**`).

Fresh post-deploy Playwright failures, new traces/screenshots/videos, or
nightly failures that begin after the deploy are alert evidence. Do not
debug selectors or repair tests inside `/monitor`.

## Trip Rules

Hard trips alert immediately:

- `/api/health` or `/api/health/preview` returns 5xx, times out, refuses
  connection, or returns invalid JSON.
- Production `/api/health` is not HTTP 200 with `status: "healthy"`.
- Preview health reports `status: "error"`.
- Vercel deployment is failed, canceled, superseded by a failed deploy, or
  not serving the expected URL.
- Sentry shows a new P0/P1 issue, auth/payment/data-loss symptom, or high
  error rate.
- Convex logs show schema mismatch, generation-wide failures, embedding
  failure bursts, or mutation/query/action errors tied to the deploy.
- Langfuse cost exceeds alert budget or quality average falls below
  threshold.
- Prompt eval artifacts show new failures for changed prompts/generation
  contracts.
- Playwright smoke or nightly artifacts show a new failure on a primary
  user journey.

Slow-burn trips require two samples or dashboard confirmation:

- Web Vitals or Speed Insights regression.
- Function/API latency p95/p99 spike.
- Sentry low-frequency P2/P3 issue.
- Convex isolated function error without user impact.
- Langfuse low-score trace count increase without average quality breach.

Do not reset the grace window after a flap. Record the flap and continue
until the original deadline or confirmed trip.

## Alert Filing

Every `monitor.alert` must leave a durable handle before handoff:

1. Prefer an existing GitHub issue when Sentry triage already created one
   through `.github/workflows/sentry-triage.yml`.
2. Otherwise create a GitHub issue for operational incidents that need
   immediate engineering attention.
3. Use `backlog.d/` for shaped, non-urgent follow-up that is real but not
   incident response.

Issue/backlog body template:

```markdown
## Monitor alert

Signal: Sentry | Vercel | /api/health | Convex | Langfuse | prompt eval | Playwright
Deploy: <url, commit, release, or Actions run>
Grace window: <start> to <end>
Severity: P0 | P1 | P2 | P3

Evidence:
- <dashboard or artifact link>
- <sample values: status, count, affected users, trace ID, workflow artifact>

Impact:
- <what user capability is broken or at risk>

Handoff:
- Route to /diagnose. Do not fix from /monitor.
- Gate for any fix: "The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes."
```

That gate sentence is quoted from `.spellbook/repo-brief.md`; cite it on
every fix handoff so the next agent does not invent a weaker exit bar.

## Control Flow

```text
/monitor [deploy-url-or-run-ref] [--grace 30m] [--preview]
  1. Resolve target URL and deploy/release/run identifiers.
  2. Start bounded grace window: 30m production, 10m preview unless explicit.
  3. Sample /api/health or /api/health/preview immediately.
  4. Check Sentry, Vercel deployments, Vercel Analytics/Speed Insights,
     Convex logs, and relevant GitHub workflow artifacts.
  5. If LLM-facing code changed, run or inspect Langfuse cost/quality
     reports and prompt eval artifacts.
  6. If E2E surfaced changed, inspect preview smoke/nightly Playwright
     artifacts or run smoke only when credentials/environment are available.
  7. Poll health every 60s; re-check dashboards at mid-window and end.
  8. On trip, file/link alert, emit monitor.alert, exit 2.
  9. On clean deadline, emit monitor.done with final samples, exit 0.
```

## Done Event

`monitor.done` should be terse and evidence-bearing:

```json
{
  "schema_version": 1,
  "kind": "monitor.done",
  "phase": "monitor",
  "agent": "monitor",
  "deploy": "https://scry.vercel.app",
  "window": {"start": "2026-04-23T18:00:00Z", "end": "2026-04-23T18:30:00Z"},
  "signals": {
    "health": {"status": 200, "body_status": "healthy"},
    "sentry": "no new P0/P1 or rate spike",
    "vercel": "deployment ready; no web vital regression observed",
    "convex": "no deploy-correlated function errors observed",
    "langfuse": "not applicable or reports under thresholds",
    "prompt_eval": "not applicable or promptfoo-results clean",
    "playwright": "not applicable or artifacts clean"
  },
  "note": "Scry stayed green through the grace window."
}
```

## Alert Event

`monitor.alert` carries facts, not theories:

```json
{
  "schema_version": 1,
  "kind": "monitor.alert",
  "phase": "monitor",
  "agent": "monitor",
  "deploy": "https://scry.vercel.app",
  "alert_ref": "https://github.com/<owner>/<repo>/issues/<n>",
  "findings": [
    {
      "signal": "/api/health/preview",
      "observed": "status:error, convexSchema:error",
      "expected": "healthy or warning without schema/AI/embedding risk",
      "first_trip_ts": "2026-04-23T18:02:13Z",
      "samples": [
        {"ts": "2026-04-23T18:02:13Z", "value": "Convex backend schema is out of sync"}
      ]
    }
  ],
  "handoff": "/diagnose",
  "note": "Alert filed with health response and deploy URL; monitor is not diagnosing root cause."
}
```

## Boundaries

- Do not run `pnpm build:local`, `pnpm build:prod`, `pnpm convex:deploy`,
  `./scripts/deploy-production.sh`, production migration scripts, or
  anything that changes non-local Convex/Vercel state.
- Do not lower Sentry, Vercel, Langfuse, prompt eval, or Playwright
  thresholds to make the watch pass.
- Do not treat prompt eval failures as harmless because the workflow is
  non-blocking; for prompt-facing deploys they are product-quality signal.
- Do not declare production clean from `/api/health` alone when the change
  touched Convex, generation, embeddings, prompts, payments, auth, or
  review flows.
- Do not diagnose in the alert. "Convex schema mismatch observed" is
  monitor output; "missing deploy step caused it" belongs to `/diagnose`.
- Do not page humans directly unless the outer loop explicitly asked for
  paging. File the alert and hand off.

## Scry-Specific Gotchas

- Preview deployments can share production Convex on the free tier. A
  preview health warning about schema, Google AI, or embeddings may be a
  production-risk signal.
- The pure FSRS review loop can be "up" while learning quality is broken.
  Langfuse quality, prompt evals, and Playwright review artifacts are
  first-class monitor inputs for generation/review changes.
- Convex backend errors are dashboard/log signals, not guaranteed Sentry
  issues.
- `README` and older docs may drift. Prefer `package.json`,
  `.github/workflows/**`, `lefthook.yml`, and `.spellbook/repo-brief.md`
  when a monitoring instruction conflicts.
