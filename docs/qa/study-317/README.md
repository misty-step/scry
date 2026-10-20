# Text Study builder proof

Accepted scope: [#317](https://github.com/misty-step/scry/issues/317), under
[#313](https://github.com/misty-step/scry/issues/313). Stories US-016, US-017,
US-007 and the real used-reference boundary in US-015.7.

Fixed parent is `17ae38bf07f5d062e06ba3065642f149e5b323a0` from
[spend PR #342](https://github.com/misty-step/scry/pull/342). No parent fix or
integration is included. Root owns the later `1ed9158` QA transport fix and
bottom-first integration. All implementation commits are retained.

[proof.json](proof.json) identifies application source, verification source,
exact Worker and CLI paths with SHA-256, checks and limits.
[walk-receipt.json](walk-receipt.json) retains every numbered criterion and
original capture hashes. [cli.jsonl](cli.jsonl) contains command/result records.
Large Library read outputs have explicit hash/size summaries; Study prompts,
answers, Corrections and refusals retain their captured output. Synthetic Owner
token identifiers use consistent `owner-1` and `owner-2` labels. The journey
asserts the real acting-token identity before capture; original capture hashes
remain in the receipt.
[SHA256SUMS](SHA256SUMS) checks the retained files.

The local run uses authored synthetic text with Apostles' Creed and Roman Missal
2011 metadata. It contains no full canonical private text or live credentials.
The clock exists only in the QA JavaScript adapter. The built production Worker
accepts no client-controlled time.

## Observed results

- The compiled frozen-parent CLI refuses `learn` as unsupported. See
  [before.json](before.json). Separate parent Worker probes stalled during setup
  and were stopped; they establish no completed Worker baseline journey.
- Native tests pass, including seven Study tests, exact-v2 invariants and all
  260 pinned scheduler trajectories. Native and WASM Clippy and Worker build pass.
- Compiled CLI → built Worker → SQLite/local R2 passes the 31 selected runtime
  tests. The existing story runner passes US-015 8/8, US-016 11/11 and US-017 10/10.
- The journey covers assisted schedule preservation, cue fading and lapses,
  six introductions per rolling day in order, Free recall once per UTC day,
  same-day extra practice, Section/whole per-Unit grading, immutable Correction,
  stale/parallel operations, a discarded committed response followed by replay,
  initialized database byte stability, restart and complete fresh local restore.
- Practice's egress counter, provider jobs, assessments and spend all stay zero.
  A changed answer under the same operation conflicts and adds no Attempt.
- Existing runtime regression checks pass 62/63. The failure is the known frozen
  parent US-014.3 `fetch failed` at `qa/owner-api.mjs:82`. No parent fix is copied.
- Story-discovery guards pass 26/26 after US-016/017 become live. Pending US-018
  and discovery drift still fail before build or proof output.
- The full pinned container gate is blocked by Docker registry DNS. Mirrodin's
  hostname also failed to resolve. [The raw failure](full-gate-failure.txt) is
  retained; no gate is weakened or reported green.

## Repeat the owned checks

Use Node 22.22.0, Rust 1.98.1, worker-build 0.8.7 and locked dependencies.
Build/runtime processes use the repository's synthetic runtime helper with no
private env files or production capabilities. Scratch is under `~/.cache/tmp`.

```sh
cargo test --locked --all-targets
cargo fmt --all -- --check
cargo clippy --locked --all-targets -- -D warnings
cargo clippy --locked --target wasm32-unknown-unknown -- -D warnings
worker-build --release --locked
node --test qa/library.test.mjs qa/study.test.mjs qa/spend.test.mjs
node --test qa/walk-cli.test.mjs
SCRY_SKIP_BUILD=1 qa/walk --stories 'US-015 US-016 US-017'
```

The broader run uses `qa/runtime.test.mjs`, `qa/owner-api.test.mjs`,
`qa/revival-preserve.test.mjs` and `qa/rust-telemetry.test.mjs`.
Its known parent failure remains in [runtime-regression.tap](runtime-regression.tap).
Gitleaks 8.30.1 scans the final committed source, including this evidence, before
push with no suppressions. The PR reports that final scan separately.

## Limits

Source-only proof. Phone, complete private Missal wording, hosted API admission
under #335 and independent hosted restore remain UNVERIFIED. There is no
production deployment, activation, historical write or real provider spend.
Initialized SQLite/WAL/SHM bytes are stable on reads. Literal cold-filesystem
stability remains FAIL under the parent ruling because framework activation
can allocate those files. Populated practice requires the compatible study-v1
writer/archive/restore; empty old states retain their bytes. The edit grid has
a 1,000,000-cell bound and the existing archive cap is 16 MiB.
