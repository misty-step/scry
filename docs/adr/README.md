# Architecture decisions

These records own technical decisions. [SPEC](../../SPEC.md) alone owns stories
and behavior, [DESIGN](../../DESIGN.md) owns visuals, and [the runbook](../runbook.md)
owns operational records. A decision is not proof of deployment or serving state.

- [ADR 004: Rust Cloudflare candidate and one durable authority](004-rust-cloudflare-candidate.md): current candidate, not activation.
- [ADR 001: One Go and SQLite state authority](001-go-sqlite-authority.md): retained Go implementation.
- [ADR 002: Cloudflare Access and exact private ingress](002-access-ingress.md): retained Go ingress.
- [ADR 003: Loopback development identity and isolated authored fixture](003-dev-identity.md): retained Go development.
