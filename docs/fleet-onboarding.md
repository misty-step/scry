# Fleet onboarding

This record owns Scry's current external integration boundary. The production
Go/SQLite/HTMX application runs behind Cloudflare Access, Worker `scry-app-host`,
and one Cloudflare Container; [the runbook](runbook.md) owns live origins,
release, and recovery. Historical Rust Worker/VM receipts are not current
runtime evidence.

| Surface | Current authority | Decision |
| --- | --- | --- |
| Application | `cmd/scry`, `internal/`, `deploy/cloudflare-hosting/`, and the runbook | One private `scry-app-host` Worker and singleton Container; one Go/SQLite writer. The former exe.dev app service is disabled; its schema-3 database remains recovery material. |
| Recovery | `internal/recovery`, `deploy/backup-gateway`, private R2 | Append/read-only backup Worker to private R2; scheduled 00:00/12:00 UTC remote snapshots and exact readback. Historical Rust Worker/Postgres recovery stays separate. |
| Monitoring | Worker/container logs, settings backup state, health/readiness | No external availability/mail monitor is claimed. The current Go Worker's backup cron is active; retired Rust Worker schedules are paused. Native Postgres backup monitoring remains recovery, not current application monitoring. |
| Model capability | Scry-only OpenRouter key | Direct HTTPS chat completions from the Cloudflare Container. Keep the provider key in the private runtime binding; build/recovery workspaces do not inherit it. |
| Canary | Estate ADR 0003, amended 2026-08-30 | Retired, not a shipped Scry integration. No replacement telemetry-ingest endpoint exists. |
| Work context | Current operator request, `AGENTS.md`, Misty Step Linear project Scry | Linear tracks ownership, execution and evidence; code/specifications remain canonical. GitHub issues and dated receipts preserve history, not a mandatory work queue. |
| Landmark | `.landmark.yml` plus Landmark CLI | Existing manifest/synthesis-only scope. No deployment-secret or release-mutating authority is added. |
| Architecture | `SPEC.md` and current Go source | The old `docs/architecture` workbench/map is retained historical Rust context, not current fleet/runtime authority. |

## Boundaries

Private Cloudflare Access ingress and an exact owner-subject check are required.
A repository name, agent identity, build result, or VM clone grants no
application, model, backup, or deployment capability. Keep production
credentials out of agents and build containers. Do not clone an active
scheduler.

The retired Rust production/staging Workers remain paused with their cron
triggers removed and recovery material retained. They are separate from the
current Go `scry-app-host` Worker, whose backup cron runs at 00:00 and 12:00 UTC.
The native Postgres writer is disabled while its retained backup timer remains
active. No old data import/deletion was authorized. Never restart an old writer
to "repair" stale fleet inventory.

`/healthz` and `/readyz` are plaintext Go liveness/readiness contracts, not remote
backup freshness, notification delivery, learning quality, or an SLA. Settings
and recovery evidence answer the backup question. [The QA contract](qa/system.md)
defines proof by surface. Source maps and manifest dry-runs establish metadata
only; they do not prove live state.
