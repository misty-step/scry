# 002: Keep exact private production ingress

- **Status:** accepted
- **Date:** 2026-10-13 (migration; original adoption date not recorded)
- **Primitives:** private-worker
- **Powder:** not recorded

## Context
[Production hosting](../../deploy/cloudflare-hosting/README.md) retains the Go
Container ingress contract. [SPEC](../../SPEC.md#one-application-and-one-state-authority)
and [AGENTS](../../AGENTS.md#product-privacy-and-historical-stores) distinguish it
from candidate Worker ingress and preserved recovery stores.

## Decision
Cloudflare Access gates production. `scry-app-host` checks issuer, audience,
and exact owner subject before forwarding to one Container. nginx strips
client-supplied identity and inserts the owner ID; Go checks canonical Host,
trusted peer, and owner identity. Alternate hosts redirect reads but reject writes.

## Consequences
Former Rust Workers and native Postgres remain recovery material, never active
ingress or writers. A `--dev` fixture cannot prove this protected production path.
Caller-supplied identity cannot replace authentication. The candidate Worker
contract does not silently replace this deployed boundary.
Revisit if: approved activation replaces the Go ingress with independently
verified exact-owner private ingress under [the runbook](../runbook.md).
