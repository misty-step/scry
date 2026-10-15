# ADR 002: Cloudflare Access and exact private ingress

Status: adopted for the retained Go implementation in
[Cloudflare hosting notes](../../deploy/cloudflare-hosting/README.md). Current
candidate ingress is [ADR 004](004-rust-cloudflare-candidate.md); this historical
decision is not a fresh observation of serving state.

Cloudflare Access gates production. `scry-app-host` checks issuer, audience and the exact owner subject before forwarding to one Container. nginx strips client-supplied identity and inserts the owner ID; Go checks canonical Host, trusted peer, and owner identity. Alternate hosts redirect reads but reject writes. The former Rust Workers and native Postgres are preserved recovery material, not active ingress or writers. A fixture on `--dev` does not prove this protected production path.
