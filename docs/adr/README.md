# Architecture decisions

These records own technical decisions. [SPEC](../../SPEC.md) alone owns stories
and behavior, [DESIGN](../../DESIGN.md) owns visuals, and [the runbook](../runbook.md)
owns operational records. A decision is not proof of deployment or serving state.

On 2026-10-20 SPEC made Scry exact-recall-first for one owner
([Exact recall first](../../SPEC.md#exact-recall-first-2026-10-20)). Until the
build tickets amend them, read ADR 004's "no API/CLI parity", "no generic agent
layer" and QA-identity statements, and ADR 006's revival custody, as superseded
in intent by SPEC AG1, US-014, US-019, retired S09.4 and amended D6.

- [ADR 004: Rust Cloudflare candidate and one durable authority](004-rust-cloudflare-candidate.md): current candidate, not activation.
- [ADR 005: Synthetic preview Access service identity](005-synthetic-preview-access.md): revocable machine QA, never production owner access.
- [ADR 006: One-way Go-to-Rust revival with Cloudflare-only custody](006-one-way-revival-custody.md): the bounded D6 exception, remaining data/cutover gates and executed synthetic checkpoint.
- [ADR 007: Generated releases for the Rust application](007-generated-releases.md): historical generated-release lane, superseded by direct exact-artifact shipping.
- [ADR 001: One Go and SQLite state authority](001-go-sqlite-authority.md): retained Go implementation.
- [ADR 002: Cloudflare Access and exact private ingress](002-access-ingress.md): retained Go ingress.
- [ADR 003: Loopback development identity and isolated authored fixture](003-dev-identity.md): retained Go development.
