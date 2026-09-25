---
name: scry-qa
description: >
  Exercise the changed Scry surface against reality: Go learning/storage,
  private HTTP/browser interaction, generation, recovery, or release smoke.
  Use for QA, verification, smoke tests, or checking the app.
argument-hint: "[learning|ui|generation|recovery|gate|prod-smoke]"
---

# Scry QA

Scry is a private Go/SQLite/HTMX app on Cloudflare: Cloudflare Access gates
Worker `scry-app-host`, which forwards to one singleton Container. An
append/read-only backup Worker exposes the private R2 recovery store. Read
`docs/qa/system.md` and the relevant `docs/runbook.md` section. The former
exe.dev app service is disabled; its retained database, the old Rust Workers,
and native Postgres are recovery material, not active application writers or
current production deployment targets.

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
between smoke and deployment or bypass Cloudflare's protected release gates or
remote-backup verification.
Scheduled or exploratory critic passes use `scripts/critics/` with run
instructions in [docs/qa/critics.md](../../../docs/qa/critics.md); treat their
output as advisory.

## UI and private production

For mutation experiments, follow the authoritative
[local authored-fixture recipe](../../../docs/qa/system.md#local-authored-fixture).
It seeds a fresh isolated database and strips inherited model/remote-backup
capabilities before serving; `--dev` alone does not provide that isolation.

Open a real browser. Exercise answer → held feedback → deliberate Next, Add and
saved generation state, Library/correction, refresh/background/reconnect, and
loss of a response after commit. Check persisted state as well as the screen.
Use native pointer/keyboard input. A DOM `.click()` bypass is not touch proof.
Diagnose a stalled harness or start a fresh isolated browser; do not erase the
acceptance gap. Physical-phone acceptance belongs to the operator.

Use only the operator-approved Cloudflare Access owner login on production.
VM-scoped credentials belong only to explicitly authorized recovery work, never
as a substitute for Access. Keep credentials out of URLs, argv, screenshots,
logs, and committed receipts.
Verify anonymous/forged Access denial, wrong Host/peer, cross-site or stale
mutations, and private content after access loss. Alternate hosts redirect reads,
never replay writes. Do not assume Access sign-out, the Go session cookie, or
independent provider/recovery credentials revoke one another; verify each at its
own authority.

Health/readiness are plaintext `ok`/`ready`; they do not prove a fresh remote R2
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

## Launch

On the project VM, use `ws init`, `ws up --task factory-foundations`, then
`ws run --task factory-foundations -- bash .exe/setup.sh` if the workspace was
not bootstrapped. Run browser/heavy checks with `ws run --task factory-foundations
-- qa/walk --all`; use `ws browser --task factory-foundations` for an interactive
headless browser over the VM's CDP tunnel. `qa/walk` builds `./cmd/scry`,
creates a new run-owned `~/.cache/tmp/scry-walk.XXXXXXXX` directory, and invokes
`seed-fixture --db` on its unused SQLite path.

## Doctor

Confirm `go version` is Go 1.27.1 linux/amd64, `node --version` is 22 or newer,
and `.exe/setup.sh` has installed test-only Playwright 1.63.0 and Chromium.
The walk waits for loopback `/readyz`; readiness is not a product or remote
backup verdict. `--dev` alone does not isolate credentials. The walk runs the
seed and server with `env -i`, an explicit PATH/HOME/LANG allowlist, and a
disposable `SCRY_BACKUP_DIR`.

## Drive

Use `qa/walk --stories "US-002 US-003"` for selected stories or `qa/walk --all`
for every live story. The runner uses real pointer/keyboard browser actions
where observable and focused CLI/Go checks for non-browser policy. No browser
or Playwright run belongs on the workstation. See
[`features/`](../../../features/README.md) for routes and selectors; do not
substitute DOM `.click()` for pointer evidence or fixture data for provider
quality.

## Evidence

Inspect `target/walk/walk-receipt.json` and `target/walk/screens/`, including
story/criterion status, checksums, source head/tree, and the run-bound base.
Pull VM evidence with `ws pull --task factory-foundations` before teardown.
`foundation-check receipt target/walk/walk-receipt.json --base <base>` checks
the current source and affected stories; `unwalked` is not a pass. Report
credential-free fixture limits separately from real-provider, production
ingress, recovery and physical-phone acceptance.

## Cleanup

The walk stops its own server and browser and removes only its own run-scoped
scratch directory after copying receipt and screenshots. Never delete an
existing SQLite path to make a seed succeed. Pull evidence, then use
`ws down --task factory-foundations` to remove the task worktree and lease,
not the standing `scry-ws` VM.
