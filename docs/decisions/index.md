# Decisions

A steering veto list. Open it before proposing a new primitive, surface, or
storage home. Write a record after deciding *not* to build something the next
agent will otherwise re-propose. Do not write one per PR. Start from
[0000-template.md](0000-template.md); the primitives map is
[primitives.yaml](../architecture/primitives.yaml).

## Steering

- [001](001-go-sqlite-authority.md): retain one production Go/SQLite authority until approved replacement activation.
- [002](002-access-ingress.md): retain exact private ingress, read-only alternate-host redirects, and paused historical writers.
- [003](003-dev-identity.md): keep loopback-only identity, fresh authored fixtures, and isolated capabilities.
- [004](004-foundation-story-walk.md): keep the pinned Foundation contract and required story-walk gate; synthetic proof does not certify real-world acceptance.

## Historical

Former superseded/proposed ADRs; their numbers stay reserved. They do not
authorize activation or new replacement features. Records 001–003 retain live
vetoes in Steering, with their original Go-specific constraints preserved.
Historical source paths are provenance, not instructions to recreate retired code.

- [005](historical/005-production-shell-boundary.md): superseded Fly/Postgres production shell (formerly architecture ADR-001; renumbered to avoid 001).
- [006](historical/006-volatile-knowledge-two-clocks.md): proposed volatile-knowledge model (formerly architecture ADR-002; renumbered to avoid 002).
