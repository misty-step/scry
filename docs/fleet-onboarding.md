# Fleet onboarding

This record owns Scry's current external integration boundary. The personal
application is Go/SQLite/HTMX on private exe.dev ingress; [the runbook](runbook.md)
owns live origins, deployment and recovery. Historical Rust/Worker receipts are
not current-runtime evidence.

| Surface | Current authority | Decision |
| --- | --- | --- |
| Application | `cmd/scry`, `internal/`, `deploy/`, and the runbook | One private unprivileged process and SQLite database on `scry-app`; no second production writer. |
| Recovery | `internal/recovery`, `deploy/backup-gateway`, private R2 | Daily/pre-release off-VM snapshots; exact checksum readback and independent restored-service proof. Historical Worker/Postgres recovery stays separate. |
| Monitoring | Settings backup state, process journal, health/readiness | No new external availability/mail monitor is claimed. The old Worker workflow is disabled and its enablement variable is false; do not repoint it to Go response formats. Native backup monitoring is retained recovery, not current application monitoring. |
| Model capability | Scry-only `scry-model` exe integration | Provider key remains outside the VM. Build/recovery workspaces do not inherit it. |
| Canary | Estate ADR 0003, amended 2026-08-30 | Retired, not a shipped Scry integration. No replacement telemetry-ingest endpoint exists. |
| Work context | Current operator request, `AGENTS.md`, Misty Step Linear project Scry | Linear tracks ownership, execution and evidence; code/specifications remain canonical. GitHub issues and dated receipts preserve history, not a mandatory work queue. |
| Landmark | `.landmark.yml` plus Landmark CLI | Existing manifest/synthesis-only scope. No deployment-secret or release-mutating authority is added. |
| Architecture | `SPEC.md` and current Go source | The old `docs/architecture` workbench/map is retained historical Rust context, not current fleet/runtime authority. |

## Boundaries

Private ingress and an exact owner check are both required. A repository name,
agent identity, build result, or VM clone does not grant application, model,
backup or deployment capability. Keep production credentials out of agents and
build containers by default. Do not clone an active scheduler.

Production and staging Workers are paused with final verified recovery copies
and no cron triggers. The native Postgres writer is disabled while its retained
backup timer remains active. No old data import/deletion was authorized. Never
"repair" stale fleet inventory by restarting an old writer.

`/healthz` and `/readyz` are plaintext Go liveness/readiness contracts, not remote
backup freshness, notification delivery, learning quality, or an SLA. Settings
and recovery evidence answer the backup question. [The QA contract](qa/system.md)
defines proof by surface. Source maps and manifest dry-runs establish metadata
only; they do not prove live state.
