# ADR 003: Loopback development identity and isolated authored fixture

Status: adopted in [README § Develop](../../README.md#develop), [Scry QA § Local authored fixture](../qa/system.md#local-authored-fixture), and [`cmd/scry/main.go`](../../cmd/scry/main.go).

`serve --dev` grants a development identity only when listening on explicit loopback. It does not disable inherited model or backup settings. `seed-fixture --db <new path>` refuses an existing SQLite database and authors DNS/TLS material without external calls. Browser proof must start under an `env -i` allowlist, with backups confined to the run-owned directory. Neither this identity nor synthetic data is permission to access production, claim provider quality, or reuse existing user data.
