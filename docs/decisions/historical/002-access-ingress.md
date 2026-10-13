# ADR 002: Cloudflare Access and exact private ingress

Status: adopted in [SPEC § V5 architecture](../../SPEC.md#one-application-and-one-state-authority), [AGENTS](../../AGENTS.md#product-and-runtime), and [Cloudflare hosting notes](../../deploy/cloudflare-hosting/README.md).

Cloudflare Access gates production. `scry-app-host` checks issuer, audience and the exact owner subject before forwarding to one Container. nginx strips client-supplied identity and inserts the owner ID; Go checks canonical Host, trusted peer, and owner identity. Alternate hosts redirect reads but reject writes. The former Rust Workers and native Postgres are preserved recovery material, not active ingress or writers. A fixture on `--dev` does not prove this protected production path.
