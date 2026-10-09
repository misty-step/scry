---
name: scry-qa
description: >
  Exercise the changed Scry replacement against reality: Rust learning and
  SQLite state, Worker/private browser, generation, recovery, or release smoke.
argument-hint: "[learning|ui|generation|recovery|gate|prod-smoke]"
---

# Scry QA

The active rewrite is Rust/WASM on a plain Cloudflare Worker, one SQLite Durable
Object, and private R2. The documented deployed app remains the Go Container
behind Access and `scry-app-host`; source changes do not activate a replacement.
Read [QA](../../../docs/qa/system.md) and the relevant
[operational runbook](../../../docs/runbook.md) before selecting proof.

## Doctor — choose meaningful proof

| Surface | Existing check | Separate real observation |
| --- | --- | --- |
| Pure learning | `cargo test --locked --test learning_policy --test fsrs_golden --test concept_policy` | 260 scheduler golden trajectories protect compatibility, not efficacy |
| Durable application | `cargo test --locked --test application` | Real workerd/SQLite restart, exact replay, one event/schedule/held result |
| Web | `cargo test --locked --test web_render` | Native pointer/keyboard, no-JS forms, private DOM on access loss |
| Generation/critic | `cargo test --locked --test generation_policy --test application` | Bounded authorized real inputs, human-reviewed material, independent critic and recorded provider usage |
| Recovery | `cargo test --locked --lib` | Complete remote archive readback, independently retrieved fresh-space restore and private UI |
| Target/gate | `cargo check --locked --target wasm32-unknown-unknown`, `worker-build --release --locked`, `bun run ci` | Exact source/artifact-bound local Worker smoke; activation separately |
| Historical recovery | `bun run test:recovery` | Corresponding historical format/source only |

Native lint is `cargo clippy --all-targets -- -D warnings`. The source-snapshot
gate is `scripts/scry-ci`; committed release output requires
`--require-committed`. Worktree output is development evidence. Never rebuild
between proof and activation or lower protected release/recovery gates.

## Launch — isolated browser and Worker

`bun run dev` / `bun run rust-dev` run `scripts/rust-dev`: unused private run
directory, synthetic loopback identity, local workerd SQLite/R2, and `env -i`
with no inherited provider, remote backup, deployment, or operator authority.
The script never loads repository private env files. The explicit authored-demo
action works only in an empty synthetic notebook. Fixtures never prove real
provider quality, unaided recall, production ingress, or remote recovery.

Prefer the T3 collaborative browser: inspect preview status, open it if needed,
and use snapshot locators with actual pointer/keyboard actions. If unavailable,
use the isolated VM browser/story walk described in docs/qa/system. Do not start
workstation headless Chromium or substitute DOM `.click()` for touch evidence.
A stalled harness is a diagnosis or acceptance gap, never a passing walk.

## Drive

Exercise Create → saved material → reference/practice and actual refinement;
question → held feedback → deliberate Next; assistance; edit/fix draft;
refresh/background/reconnect; response loss after commit; competing tabs; and
loss of private access. Inspect persisted records as well as the screen.
Answers, explanations, current notes/names, and goal title/intent remain absent
before durable assistance/self-check/result. Offline work pauses and the same
frozen operation reconciles an unknown response; no mutation queue.

For private production use only the approved Cloudflare Access owner login.
Verify signature/issuer/audience/exact owner, anonymous/forged denial,
Host/origin/CSRF, alternate-host write rejection, and history after access loss.
Never copy tokens into URLs/argv/logs/captures or substitute a VM credential.
Upstream logout, provider keys, and restore/operator capabilities are separate.

## Evidence — recovery and receipts

Remote success requires complete JSON+photo archive validation and exact R2
readback. Restore an independently retrieved archive with compatible Worker
assets/configuration into an unused isolated namespace. No live integrations;
uncertain jobs/assessments stay paused. A checksum or local R2 emulator does not
prove independent hosted recovery. Measure actual private service/UI recovery
when claiming the RPO 24h/RTO 60m objectives, and name omitted steps.

Return PASS, FAIL, or UNVERIFIED with exact source/artifact, environment,
story/criterion, data provenance, actions, observed result, and limits. Keep
live AI, real phone, private ingress, and remote recovery distinct. The Go and
retired Rust receipts do not transfer to this fresh rewrite. Critics under
`scripts/critics/` remain advisory and need a current Worker observation;
`qa/walk` must produce current target receipts rather than relabel Go passes.

## Cleanup

Stop only the run-owned server/browser before cleanup. Preserve receipts and
screenshots, then remove only its verified temporary state/worktree/lease.
Never delete an existing SQLite namespace, R2 bucket, historical store, or
standing workspace to make a synthetic seed succeed. Do not activate or restart
any production/historical writer during a QA run.
