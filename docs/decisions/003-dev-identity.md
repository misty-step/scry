# 003: Keep loopback identity and isolated authored fixtures

- **Status:** accepted
- **Date:** 2026-10-13 (migration; original adoption date not recorded)
- **Primitives:** private-worker, foundation-gate
- **Powder:** not recorded

## Context
[Development](../../README.md#develop) and [Scry QA](../qa/system.md) require
isolated synthetic proof. The original `cmd/scry/main.go` and Go commands remain
available in Git at `c6ba395`, not as current Rust development entrypoints.
The current [QA skill](../../.agents/skills/scry-qa/SKILL.md) owns verification procedures.

## Decision
The retained Go `serve --dev` grants identity only on explicit loopback; it does
not disable inherited model or backup settings. `seed-fixture --db <new path>`
refuses an existing SQLite database and authors DNS/TLS material without external
calls. Browser proof must start under an `env -i` allowlist, with backups confined
to the run-owned directory.

## Consequences
Development identity and synthetic data never permit production access,
provider-quality claims, or reuse of existing user data. The Rust replacement
uses its documented isolated launcher, without treating old command evidence
as replacement proof.
Revisit if: the isolated development mechanism changes; preserve explicit
loopback, fresh data, capability isolation, and honest evidence boundaries.
