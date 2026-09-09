---
name: scry-qa
description: >
  Exercise the changed Scry surface against reality: Go learning/storage,
  private HTTP/browser interaction, generation, recovery, or release smoke.
  Use for QA, verification, smoke tests, or checking the app.
argument-hint: "[learning|ui|generation|recovery|gate|prod-smoke]"
---

# Scry QA

Scry is one private Go/SQLite/HTMX application on exe.dev, with an append/read
Cloudflare Worker over private R2 for recovery. Read `docs/qa/system.md` and the
relevant `docs/runbook.md` section. The old Rust Workers and native Postgres are
frozen recovery material, not current application test/deployment targets.

## Choose meaningful proof

| Changed surface | Existing checks | Real-world proof |
| --- | --- | --- |
| Learning/storage | `go test ./internal/learning ./internal/store` | Durable event/schedule agreement, exact retry, restart |
| Web/auth | `go test ./internal/web` | Real browser events, rendered state, actual private ingress and access loss |
| Generation | `go test ./internal/generation` | One bounded authorized live exercise; inspect content/provenance/coverage and provider spend |
| Recovery | `go test ./internal/recovery` | Remote checksum readback, independent restore, restored service/UI when claimed |
| Release | `bun run ci` or `bun run ci:full` | Inspect and deploy the exact source-bound smoke-tested binary |
| Historical recovery | `bun run test:recovery` | Corresponding old format/store only; no Go parity claim |

The full release command is:

```sh
bun run ci:full -- --out target/ci-release --require-committed
```

It uses pinned Dagger tooling, freezes actual source, runs shared checks and
redacted Gitleaks, and exports the same binary exercised by the synthetic smoke.
Worktree-labeled artifacts are not committed release proof. Never rebuild
between smoke and deployment or bypass protected activation/remote backup.

## UI and private production

Use an isolated app with synthetic data for mutation experiments:

```sh
go run ./cmd/scry serve --dev --db data/scry.sqlite --addr 127.0.0.1:8080
```

Open a real browser. Exercise answer → held feedback → deliberate Next, Add and
saved generation state, Library/correction, refresh/background/reconnect, and
loss of a response after commit. Check persisted state as well as the screen.
Use native pointer/keyboard input. A DOM `.click()` bypass is not touch proof.
Diagnose a stalled harness or start a fresh isolated browser; do not erase the
acceptance gap. Physical-phone acceptance belongs to the operator.

Use only approved login or VM-scoped authority on the deployed private origin.
Keep credentials out of URLs, argv, screenshots, logs, and committed receipts.
Verify anonymous/forged access, wrong Host/peer, cross-site or stale mutations,
and private content after provider-access loss. Alternate hosts redirect reads,
never replay writes. Browser logout does not revoke independent VM API tokens.

Health/readiness are plaintext `ok`/`ready`; they do not prove fresh off-VM
backup, model usefulness, or learning. Settings exposes backup status. There is
no Go `/statusz`, public `/v1`, or maintained Rust CLI/MCP contract.

## Recovery and report

Restore a completed independently retrieved archive and compatible binary into
an unused isolated environment. Never overwrite the live database, attach live
integrations to a preview, or restart uncertain paid work automatically. A data
restore is not full service recovery: measure activation, private HTTPS and UI
when claiming those; name omitted provisioning or DNS steps. Stop the rehearsal
when finished. Local-only backup allowance is synthetic-only, never production.

Reuse valid evidence and keep dated receipts truthful. Return PASS, FAIL, or
UNVERIFIED with exact source/artifact, environment, behavior exercised, observed
results, and limitations. Tests, live AI, phone acceptance, delayed recall,
provider mail delivery, and availability are separate claims. Do not turn old
Rust fixtures or receipts into proof of the current Go product.
