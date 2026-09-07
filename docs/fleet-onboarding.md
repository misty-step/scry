# Fleet onboarding

This is the repo-local decision record for the current fleet integration
boundary. It points at current source and operator authorities, not proof of
live Worker cutover or scheduled production monitoring.

## Current decisions

| Surface | Current authority | Decision and proof |
| --- | --- | --- |
| Canary | Estate ADR 0003, amended 2026-08-30 | Retired, not a shipped Scry integration. No live replacement telemetry-ingest endpoint exists; deployment credentials, health checks, and notifications do not belong here. |
| Worker-native logs | `crates/memory-engine-cloudflare/src/telemetry.rs`, `wrangler.jsonc`, and `docs/runbook.md` | The supported runtime emits bounded, content-free browser/performance/health records. Browser receipt 202 with `runtime_logged` and attempts `0` means logging only, not remote acceptance or durable telemetry readback. |
| External monitoring | `scripts/scry-monitor`, `.github/workflows/production-health.yml`, and `docs/qa/system.md` | Repo-owned public health probes and Resend unhealthy/recovery mail. Source is present; scheduled production monitoring remains disabled with `SCRY_MONITOR_ENABLED=false` until Main's cutover. |
| Work context | `AGENTS.md`, the current operator request, and [`misty-step/scry` issues](https://github.com/misty-step/scry/issues) | Direct work uses current scope, overlap checks, and session or PR evidence. GitHub Issues preserves history; no issue or maintained backlog is required. |
| Landmark | `.landmark.yml` plus Landmark CLI | Adopted in manifest-only/synthesis-only mode. The repo has no release-secret authority, so no release-mutating workflow is added. Preview with `landmark setup --repo-root . --dry-run` and `landmark run --provider local --repo-root . --dry-run` from a Landmark checkout. |
| Project map | `docs/architecture/memory-engine.map.json` and `workbench.html` | This is the live repo-local architecture-map successor for project structure. It is static, diffable, and explicitly separate from Landmark release intelligence. |

## Monitoring boundary

The native DigitalOcean/Postgres service remains authoritative until Main
records the source writer barrier, final import/readback, recovery, activation,
and public proof. No live Worker is deployed yet. Local workerd and loopback
monitor evidence are not live cutover, external telemetry readback, Resend
acceptance, inbox delivery, or scheduled production monitoring.

The POSIX CLI is `python3 scripts/scry-monitor` (also `bun run ops:monitor`),
with required `--environment staging|production`, `--state-file`, and
`--receipt-file` arguments. It observes public `GET /healthz`, `/readyz`, and
`/statusz` without waking jobs or repairing the recovery evidence it checks.
`/statusz` is healthy only for an active actor with integer `backupAgeMs`
from 0 through 90,000,000 ms (25 hours), inclusive; paused, missing,
future-dated, or stale recovery evidence is degraded.

The `production-health.yml` workflow checks out only `master` and uses the
`production-monitor` environment. Restrict that environment to `master` and
keep only the operator mail envelope there: `RESEND_API_KEY`,
`MEMORY_ENGINE_MAIL_FROM`, and `MEMORY_ENGINE_ALERT_TO`. No learner, admin,
magic-link, or Worker deployment key is needed. Every CLI invocation validates
local mail configuration, even when healthy and sending no mail.
An explicit `master` dispatch bypasses automatic enablement; its optional
`delivery_drill` input labels a real mail drill without simulating an incident.

Five-minute GitHub cron is best effort: runs may be delayed, dropped, or
disabled. Notification cache is not durable alert history; expiry or loss can
duplicate alerts or lose recovery context. Retries preserve their original
observation, and Resend idempotency lasts only 24 hours. A skipped/disabled
probe is not monitoring, and `provider_accepted` is not inbox delivery.
Exact commands and receipt semantics are in
[the QA system](./qa/system.md#observability-and-external-monitor-proof);
[the runbook](./runbook.md) owns setup and executed production evidence.

## Verification

```sh
python3 -m json.tool docs/architecture/memory-engine.map.json >/dev/null
test -f docs/architecture/workbench.html
```

These checks establish map syntax and workbench presence only, not runtime,
notification, or deployed-monitor proof.

The canonical project map is intentionally not generated from Landmark or
runtime code. The Rust crates and tests remain runtime truth.
