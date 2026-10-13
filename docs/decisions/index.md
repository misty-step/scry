# Decisions

A steering veto list. Open it before proposing a new primitive, surface, or
storage home. Write a record after deciding *not* to build something the next
agent will otherwise re-propose. Do not write one per PR. Start from
[0000-template.md](0000-template.md); the primitives map is
[primitives.yaml](../architecture/primitives.yaml).

## Steering

- [004](004-foundation-story-walk.md): keep the pinned Foundation contract and required story-walk gate; synthetic proof does not certify real-world acceptance.

## Historical

Former ADRs, unchanged; their numbers stay reserved. These describe historical
architectures or proposals, not authority to activate or extend the replacement.

- [001](historical/001-go-sqlite-authority.md): retained Go and SQLite state authority.
- [002](historical/002-access-ingress.md): retained Go Container private ingress.
- [003](historical/003-dev-identity.md): retained Go loopback development identity and authored fixture.
- [005](historical/005-production-shell-boundary.md): superseded Fly/Postgres production shell (formerly architecture ADR-001; renumbered to avoid 001).
- [006](historical/006-volatile-knowledge-two-clocks.md): proposed volatile-knowledge model (formerly architecture ADR-002; renumbered to avoid 002).
