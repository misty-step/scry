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

[Product](VISION.md) · [Stories](USER_STORIES.md) · [Specification](SPEC.md) ·
[Design](DESIGN.md) · [Features](features/README.md) ·
[Verification](docs/qa/system.md) · [Current production operations](docs/runbook.md)

## Start or resume work

Read VISION → USER_STORIES → SPEC, especially US-013/S11, before scoping work.
Use the current operator request as authority; preserve overlapping work and
inspect `git status --short --branch` and `git rev-parse HEAD` in the selected
checkout. [Glass](https://mirrodin.tail5f5eb4.ts.net) in scope `misty-step/scry`
owns work state; legacy MIS and Linear identities are provenance. Link accepted
scope, exact source, tests, and remaining evidence from the session/PR.

[DESIGN](DESIGN.md) governs the redesigned curiosity notebook. Learning effort
is intentional; navigating an administration dashboard is not part of study.
Create and Map are the two masthead destinations. Results stay until Next;
notes remain reusable; help and reading never become unaided success.

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

- [src/learning.rs](src/learning.rs): pure pinned FSRS scheduling, grading,
  recall estimates, and concept evidence. No persistence, HTTP, or models.
- [src/model.rs](src/model.rs) / [src/engine.rs](src/engine.rs): typed records,
  atomic transitions, occurrence identity, immutable presentations and
  corrections, exact operation receipts, ownership, and bounded spend.
- [src/generation.rs](src/generation.rs): bounded intent-to-reference/practice,
  exact provenance, saved candidates, independent critic and meaning policies.
- [src/runtime.rs](src/runtime.rs): private Worker ingress, singleton SQLite
  Durable Object, external effects outside transactions, alarms, and R2.
- [src/persistence.rs](src/persistence.rs): complete versioned JSON archives
  with photo bytes and checksums, validation, and paused fresh-space restore.
- [src/web.rs](src/web.rs) / [assets](assets/): escaped HTML and presentation;
  no frontend build, browser storage, or offline mutation queue.

The provider allowance remains $3.50 per rolling day, with $0.50 reserved per
preparation and unknown sent outcomes accounted. The existing provider key's
$25/week cap is a separate control. Private text and pasted URLs never authorize
web search or link reading by inference.

## Release and recovery

[The Rust target release procedure](docs/rust-cloudflare.md) describes the new
namespace, frozen artifacts, private-ingress and recovery proof, and explicit
activation approval.

The new target starts in a fresh namespace, with private R2 asset/backup buckets
and a singleton SQLite Durable Object. It requires independent proof of exact
owner JWT validation, alternate-host read-only redirects, CSRF, complete remote
readback, restore into an unused space, paused uncertain work, and the actual
private browser before activation. Daily and pre-release snapshots with 30-day
retention target RPO 24 hours and RTO 60 minutes; those targets are not guarantees.
Retain the compatible Worker build and private configuration independently.

Activation, live migration, and retirement of the current Go writer require
explicit operator approval and concrete source-bound release/recovery evidence.
The existing [runbook](docs/runbook.md) and
[Container hosting notes](deploy/cloudflare-hosting/README.md) remain the
operational authority for production until that approved replacement occurs.
