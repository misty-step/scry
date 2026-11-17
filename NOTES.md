# NOTES

## Handoff (2026-11-17, MIS-1210)

The first build lane (Claude Code, Fable 5.2) stopped on a usage limit at commit 9d2437a plus `internal/web/rules_test.go`, now committed. You continue on the same branch. The contract is `PRODUCT.md` (what Scry does) and `DESIGN.md` (how it looks and feels), and `AGENTS.md` holds the build rules. Those three files and this note are your only sources.

### Done

- One Go service in `cmd/scry` with packages under `internal/`: `srs` (scheduling), `exact` (word-for-word judging of exact texts), `starter` (the starter shelf), `store` (SQLite), `ai`, `speech`, `auth`, `billing` and `alert` (adapters with test modes), `app` (logic) and `web` (server-rendered HTML).
- Every story and business rule in PRODUCT.md has a named test (`TestStory_*` and `TestRule_*`).
- `go build ./...`, `go vet ./...` and a fresh `go test -count=1 ./...` all pass at handoff.
- A `Makefile` with `run`, `test`, `build`, `vet` and `docker` targets.

### Left

1. A `Dockerfile` for one small image that serves on `PORT`, with a non-root user and the SQLite file on a volume. `make docker` must build it.
2. `README.md`: how to run, test and deploy it, how durability and backup work, and a story-to-test table with one row per PRODUCT.md story and business rule naming the test that proves it.
3. Prove `make run` works: it starts in test mode with seeded demo data, `/healthz` answers 200, and the sign-up and study pages render. Record what you ran and saw at the end of this note.
4. Below this handoff, add an `## Ambiguities` section listing every place PRODUCT.md or DESIGN.md left a choice open, and the choice you made.
5. Fix anything you find that misses PRODUCT.md or DESIGN.md, and add a test for each fix.

### How to verify

```sh
go build ./... && go vet ./... && go test -count=1 ./...
make run &   # then:
curl -fsS localhost:8080/healthz
curl -fsS localhost:8080/ | head
make docker
```

Done means all of these pass, the README table covers every story and rule, and this note records the results.
