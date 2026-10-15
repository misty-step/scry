# Scry

Scry is a private, prompt-first learning notebook: **Create → express what you
want to understand → useful reference and practice → refine through real
answers and feedback**. A word, phrase, or dictated ramble is enough. There is
no setup questionnaire, compulsory import, or diagnostic quiz before material.

The application in this checkout is a ground-up **Rust + Cloudflare Workers**
rewrite, authorized on October 7, 2026. A plain WASM Worker validates private
Access ingress and forwards to one SQLite-backed Durable Object. Rust owns
learning, immutable history, generation, and recovery; server-rendered HTML,
self-hosted fonts, and a small vanilla controller deliver the notebook. There
is no Container or frontend framework in the replacement architecture.

**Production has not been replaced by source changes.** The documented live
application remains the Go/SQLite Container behind `scry-app-host` at
https://scry.study. [The operational runbook](docs/runbook.md) alone owns its
activation, origins, recovery, and retirement evidence. The old Go source is
retained exactly in Git at `c6ba395`; historical Rust/Postgres/Worker stores and
backups remain separate recovery material. This rewrite does not import,
reactivate, rename, or erase those stores.

[Product intent](VISION.md) · [Stories and specification](SPEC.md) ·
[Design](DESIGN.md) · [Features](features/README.md) ·
[Verification](docs/qa/system.md) · [Current production operations](docs/runbook.md)

## Start or resume work

Read SPEC, especially US-013/S11, before scoping work. VISION explains intent;
USER_STORIES is only a compatibility index. Technical decisions live in ADRs.
Use the current operator request as authority; preserve overlapping work and
inspect `git status --short --branch` and `git rev-parse HEAD` in the selected
checkout. [Glass](https://mirrodin.tail5f5eb4.ts.net) in scope `misty-step/scry`
owns work state; legacy MIS and Linear identities are provenance. Link accepted
scope, exact source, tests, and remaining evidence from the session/PR.

[DESIGN](DESIGN.md) governs visual decisions; [SPEC](SPEC.md) alone governs
behavior. Do not synchronize requirements into work instructions or feature maps.

## Develop

Use the pinned Rust 1.98.1 toolchain and `wasm32-unknown-unknown` target,
`worker-build` 0.8.7, Node 22.22.0, Bun, and lockfile-pinned dev dependencies.

```sh
rustup target add wasm32-unknown-unknown
cargo install worker-build --version 0.8.7 --locked
npm ci --ignore-scripts --no-audit --no-fund
cargo test --locked
bun run dev
```

`bun run dev` / `bun run rust-dev` run [scripts/rust-dev](scripts/rust-dev),
which builds WASM and launches real local workerd with fresh run-owned SQLite
and R2 state. Its private temporary configuration and `env -i` keep inherited
operator/model/deployment capabilities out. It listens only on loopback; set
`SCRY_DEV_PORT` to an unused unprivileged port if necessary. It never loads
repository `.env` or `.dev.vars` files. The printed directory owns that run's
state; stop the process before removing only that directory.

The empty notebook offers an explicitly authored demo only in isolated
development. The demo uses no provider and is never advertised in production.
Without configured providers, new requests remain saved with an honest stopped
preparation state. Local fixture mechanics do not prove live model usefulness,
private HTTPS, remote R2 recovery, or phone acceptance.

[.dev.vars.example](.dev.vars.example) documents synthetic local values only.
Production secrets belong in Worker bindings: `OWNER_SUBJECT`, `INTERNAL_KEY`,
`OPENROUTER_API_KEY`, `JEV_API_KEY`, and independently scoped `OPERATOR_KEY` /
`RESTORE_TOKEN` where required. Public configuration includes the canonical
origin, exact Access issuer/audience, singleton namespace, and explicit model
choices. Never put secrets in source, flags, logs, screenshots, or artifacts.

## Check and build

```sh
cargo test --locked
cargo clippy --all-targets -- -D warnings
cargo check --locked --target wasm32-unknown-unknown
worker-build --release --locked
bun run ci
```

The native suite covers application transitions, generation/critic policy,
immutable content, response replay, recovery archives, web secrecy, and pure
learning. The scheduler port compares 260 full-state trajectories against the
exact pinned Go FSRS adapter. That is scheduler compatibility evidence, not
proof of model calibration or learning efficacy.

`bun run ci` / `bun run ci:local` use the source-snapshot gate in
[scripts/scry-ci](scripts/scry-ci). Release evidence must bind committed source,
the exact WASM Worker assets exercised, lockfiles, and smoke results. Use
`--require-committed` when producing release artifacts; worktree-labeled output
is development evidence. Never rebuild between proof and activation. Retained
historical recovery tools have their own `bun run test:recovery` contract.

Browser QA uses the product-native collaborative preview when available and
isolated VM browser/story walks for release receipts. Exercise real pointer and
keyboard events, held results, no-JavaScript forms, both color schemes, narrow
screens, offline drafts, access loss, and identical-operation reconciliation.
Read [Scry QA](.agents/skills/scry-qa/SKILL.md) before verification. Unrun cases
remain UNVERIFIED; old Go/Worker receipts do not transfer to this rewrite.

The replacement's [operator procedure](docs/rust-cloudflare.md#independent-fresh-space-restore)
documents isolated restore and separately authorized future-work resumption.
These candidate endpoints do not replace the deployed Go recovery runbook.

## Durable boundaries

[ADR 004](docs/adr/004-rust-cloudflare-candidate.md) owns architecture and module
boundaries. [SPEC](SPEC.md#adopted-behavioral-policies) owns learning, privacy,
spend and recovery policies; this entrypoint is not another copy.

## Release and recovery

[The Rust target release procedure](docs/rust-cloudflare.md) describes the new
namespace, frozen artifacts, and verifiable private-ingress, recovery and
cutover conditions. Routine activation needs no separate operator approval
once those conditions pass.

Changed private-data, credential, spend or irreversible-action scope still
requires its own authorization. The existing [runbook](docs/runbook.md) and
[Container hosting notes](deploy/cloudflare-hosting/README.md) remain the
operational authority for production until the verified replacement occurs.
