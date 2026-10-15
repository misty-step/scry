# ADR 003: Loopback development identity and isolated authored fixture

Status: adopted for the retained Go implementation at
[`c6ba395:cmd/scry/main.go`](https://github.com/misty-step/scry/blob/c6ba395a63814c1f973d8cda5057d54fbb587e9b/cmd/scry/main.go).
The current Rust candidate's development boundary is [ADR 004](004-rust-cloudflare-candidate.md)
and [Scry QA](../qa/system.md#local-authored-fixture), not the commands below.

`serve --dev` grants a development identity only when listening on explicit loopback. It does not disable inherited model or backup settings. `seed-fixture --db <new path>` refuses an existing SQLite database and authors DNS/TLS material without external calls. Browser proof must start under an `env -i` allowlist, with backups confined to the run-owned directory. Neither this identity nor synthetic data is permission to access production, claim provider quality, or reuse existing user data.
