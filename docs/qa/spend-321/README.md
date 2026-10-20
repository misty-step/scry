# Spend slice builder evidence

Controlled, local proof for [#321](https://github.com/misty-step/scry/issues/321),
under [#313](https://github.com/misty-step/scry/issues/313), scoped to US-019.7.
This is implementation-author proof. Root owns independent QA, review and judge.
US-019 retains `Evidence: pending` for #322.

The branch began at frozen Library source
`42778fa0466a28338dea81d94e4a366886d02e8b`. Implementation commit `c2eec92`
is preserved. After [PR340](https://github.com/misty-step/scry/pull/340) merged,
ordinary merge `9c7d14e` integrated master `31de4f7`. The four overlapping
Library conflict blocks retained the spend additions; upstream operational docs
are preserved. There was no reset, rebase, force-push or parent-branch edit.

[receipt.json](receipt.json) binds that clean committed source export, lockfiles,
changed files and exact Worker/CLI artifacts to the checks. This evidence-only
commit changes no tested implementation. Checksum records have separate `path`
and `sha256` fields. The retained local artifact directory is recorded there.

## Observations

- [Before](before.json): the fixed-base built Worker returned 500 when loading
  the mixed ledger's missing model, and 404 for an Access owner's empty spend
  read. The same candidate probe failed both tests.
- [After](cli.json): all three grouping selectors succeed through the compiled
  CLI and the real Worker API with equal JSON. Five authored entries total
  2,500,000 reserved micros, 579 known micros, and 1,000,000 unknown-held micros.
  Group sums equal entry sums. The 23:59:59.999Z and 00:00:00.000Z reservations
  fall on separate UTC dates. Unknown costs stay `null`; reported zero stays zero.
- Repeated API and CLI reads preserve every ordered SQLite key/value, SQLite's
  `total_changes()`, and all 12 persisted resource files, including database,
  WAL, SHM, metadata and local R2 files. This covers revision, token last use and
  operation receipts. Access reads of an empty store create no learner table.
- The same journey observes uniform 401 for missing, malformed, forged,
  unknown, expired and revoked credentials, including unreadable learner state.
  An unpaid token can read spend and receives 403 for paid create before any
  reservation. Invalid selectors return 422; spend POST returns 404. CLI errors
  exit nonzero, keep JSON private, refuse redirects and another origin, and
  reject a non-private login file. Controlled proxy errors cannot echo a bearer.
- [Controlled costs](controlled.json): the existing provider-boundary fixture
  records `generate.concepts`, `critic`, `grade.jev` and `transcribe.photo` at
  reservation, with the requested model and provider-reported cost. The existing
  transport's upward rounding records `$0.000123` as 124 micros. Interrupted
  outcomes retain reservations; replay does not resend; measured over-allowance
  cost blocks another send. These are intercepted responses, with no provider
  transmission or actual spend.
- `tests/spend.rs` validates historical missing/empty/nonempty attribution,
  exact row/archive roundtrips and opaque retained custody bytes, plus attributed
  archive format refusal, inconsistent statuses, overflow and the unchanged
  rolling allowance. Full native checks pass 128 tests; one existing Unicode
  corpus test remains ignored. CLI integration passes 18 tests; runtime passes
  35. Native/WASM Clippy, target compilation and the release Worker build pass.

## Repeat

Use README's pinned Rust 1.98.1, Node 22.22.0 and worker-build 0.8.7 in an
isolated environment with no private env files or capabilities. Export the
`source_commit` in the receipt with `git archive` to an unused directory, then
run the same focused commands:

```sh
npm ci --ignore-scripts --no-audit --no-fund
cargo fmt --all -- --check
cargo test --locked --all-targets
cargo clippy --locked --all-targets -- -D warnings
cargo clippy --locked --target wasm32-unknown-unknown -- -D warnings
cargo check --locked --target wasm32-unknown-unknown
worker-build --release --locked
node --test qa/spend.test.mjs qa/owner-api.test.mjs qa/library.test.mjs
node --test qa/runtime.test.mjs
```

`qa/spend.test.mjs` writes a sanitized transcript to `target/spend-cli.json`.
The existing runtime suite prints its controlled cost observation and exact
WASM hash. The retained CLI receipt omits duplicate successful reads and login
output. Both repetitions still ran and asserted byte equality.

No claim covers live provider usefulness, hosted ingress, physical phones,
independent hosted recovery, deployment, activation, or the full browser gate.
There are no production or historical data writes. Populated rollback requires
an attribution-aware writer; [compatibility notes](../../spend.md#compatibility-and-rollback)
describe why the parent binary is unsafe for current SQLite.
