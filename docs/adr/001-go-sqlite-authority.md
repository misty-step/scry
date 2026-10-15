# ADR 001: One Go and SQLite state authority

Status: adopted for the retained Go implementation at Git `c6ba395`. The current
Rust candidate's implementation decision is [ADR 004](004-rust-cloudflare-candidate.md);
this historical decision does not establish which application is serving.

Scry uses one Go process and a local SQLite WAL store for learning writes, background jobs, and recovery. `internal/learning` stays pure; `internal/store` owns short atomic transitions; generation and semantic calls stay outside SQL. HTML/HTMX and embedded assets are served by that process, not a second frontend. A new writer, app Worker database, or frontend build would change the authority boundary and needs a new decision. This records the existing architecture, not a new migration approval.
