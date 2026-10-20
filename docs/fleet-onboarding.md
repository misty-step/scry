# Fleet onboarding

This record maps Scry's external integration boundaries.
[The runbook's dated serving observation](runbook.md#rust-production-serving--observed-2026-10-20)
owns production serving state and observed schedules. The runbook owns live
origins, release, and recovery. Historical Go and Rust Worker/VM receipts are
not current runtime evidence.

| Surface | Current authority | Decision |
| --- | --- | --- |
| Application | [Dated serving observation](runbook.md#rust-production-serving--observed-2026-10-20) | Consult the operational record before selecting a runtime or deployment target. Retained source and recovery material do not establish serving state. |
| Recovery | [Runbook](runbook.md) and [replacement release guide](rust-cloudflare.md) | Select procedures compatible with the observed runtime and retained archive. Historical Go and Rust Worker/Postgres recovery stays separate. |
| Monitoring | [Dated serving observation](runbook.md#rust-production-serving--observed-2026-10-20) and runtime-specific recovery receipts | No external availability/mail monitor is claimed. Retained cron configuration and historical backup monitoring do not establish active application schedules. |
| Model capability | Runtime configuration and the runbook | Establish provider capability separately from serving state. Keep provider keys in private runtime bindings; build/recovery workspaces do not inherit them. |
| Canary | Estate ADR 0003, amended 2026-08-30 | Retired, not a shipped Scry integration. No replacement telemetry-ingest endpoint exists. |
| Work context | Current operator request, `AGENTS.md`, [README work route](../README.md#start-or-resume-work) | Glass scope `misty-step/scry` tracks current operational state; keep ownership and evidence in the session/PR. Legacy MIS identities, Linear links and dated receipts retain provenance, not execution permission; migration reconciliation is separately owned. |
| Landmark | `.landmark.yml` plus Landmark CLI | Existing manifest/synthesis-only scope. No deployment-secret or release-mutating authority is added. |
| Architecture | [ADRs](adr/README.md) and corresponding source | SPEC owns behavior, not architecture. ADRs distinguish retained Go from the current Rust candidate; the old `docs/architecture` workbench/map is retired Rust context, not fleet/runtime authority. |

## Boundaries

Private Cloudflare Access ingress and an exact owner-subject check are required.
A repository name, agent identity, build result, or VM clone grants no
application, model, backup, or deployment capability. Keep production
credentials out of agents and build containers. Do not clone an active
scheduler.

Use [the dated serving observation](runbook.md#rust-production-serving--observed-2026-10-20)
for runtime and schedule evidence. Historical Go, Rust Worker and Postgres
stores and backups remain separate recovery material. No old data import/deletion
was authorized. Never restart an old writer to "repair" stale fleet inventory.

The retained Go `/healthz` and `/readyz` are plaintext liveness/readiness
contracts, not remote backup freshness, notification delivery, learning quality,
or an SLA. Settings and recovery evidence answer the backup question.
[The QA contract](qa/system.md)
defines proof by surface. Source maps and manifest dry-runs establish metadata
only; they do not prove live state.
