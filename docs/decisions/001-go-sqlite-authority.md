# 001: One production Go and SQLite state authority

- **Status:** accepted
- **Date:** 2026-10-13 (migration; original adoption date not recorded)
- **Primitives:** durable-learning-state, private-worker
- **Powder:** not recorded

## Context
The deployed Go application remains governed by [the runbook](../runbook.md);
[the replacement specification](../../SPEC.md#one-application-and-one-state-authority)
and [architecture boundaries](../../AGENTS.md#architecture-and-boundaries) distinguish
candidate source from production activation. The original Go source is retained at
`c6ba395`; this record does not approve a migration.

## Decision
Until explicitly approved replacement activation, keep one Go process and a local
SQLite WAL store as production authority for learning writes, background jobs,
and recovery. `internal/learning` stays pure; `internal/store` owns short atomic
transitions; generation and semantic calls stay outside SQL. Serve HTML/HTMX
and embedded assets from that process, without a second frontend.

## Consequences
A new writer, app Worker database, or frontend build changes that production
authority boundary and requires a new decision. The separately authorized Rust
candidate does not authorize activation, import, or two live writers.
Revisit if: the operator approves replacement activation with migration and
recovery proof under the runbook.
