# PR #343 judge fixes

Story: US-016, US-017, US-007, US-015

This round implements [the judge's A/C blockers](https://github.com/misty-step/scry/pull/343#issuecomment-6298136225)
and [diagnostic defect #346](https://github.com/misty-step/scry/issues/346).
Starting HEAD is `3344c8b16bf23f6f1a242c052b4e0840850d231b`; fixed base is
`294f99808a352e066ca6a59cfaa8320eb37d2ee7`. The original author receipts and
independent reports retain their original source and meaning.

## Changed behavior

The shared engine receipt return checks the current protected Prompt before
returning saved data or resolving an Attempt. Replaying an old read cue,
previous-Unit context, reveal or held/answer Attempt returns 409. It changes no
SQLite row. Safe Study metadata, Corrections and current or stale permitted
Prompts still replay. The same ID with changed payload still conflicts.
Current reveal/answer retries retain one immutable Attempt and its schedule.

Attempts now require an explicit `revealed` fact, separate from cue assistance.
An unrevealed Good still counts at every rung. A revealed Good breaks the
success run; a revealed Again does not lapse. Answer, Correction reconstruction
and archive validation use the same transition. The original Attempt remains
immutable. Study validation checks the Version's Work before rendering any
frozen Prompt. Missing Work/Version/Section/Unit or malformed target references
produce a refusal instead of a panic. Ordinary restore keeps its existing
refusal and writes zero learner rows.

## Red and green observations

- [Native before](logs/native-before.txt): eight passed, three failed for the
  replay disclosure, revealed-copy fade and missing-Work panic. [Native after](logs/native-after.txt):
  12 Study tests pass, including old Free recall context disclosure, every rung,
  reveal/Again, Corrections, archive reconstruction and missing references.
- The identical [CLI/API/Worker regression file](../../../../qa/study-integrity.test.mjs)
  runs against both frozen artifact sets under Node 22.22.0.
  [Before](logs/runtime-before.tap): four passed, five failed. Unit, two-Unit
  Section and two-Unit whole Recitation replay returns 200 with protected text;
  a revealed copy fades Read; missing-Work dry-run traps and reaches the bounded
  five-second deadline. [After](logs/runtime-after.tap): nine passed, zero failed.
  [Observed results](observations.json) include 409 refusals with unchanged rows,
  every rung's success reset, and ordinary/dry-run restore 422 with zero rows.
  A valid populated archive with revealed Attempts and Corrections also restores.
- [Nearby native checks](logs/native-nearby.txt) pass 76 tests, including the
  then-current 11 Study tests. With the additional context regression, 77 unique
  native tests pass. The unchanged Unicode conformance test remains ignored.
  These include application, Library, archive, custody, schedule history and
  the 260 pinned FSRS trajectories.
- [Nearby runtime checks](logs/runtime-nearby.txt) pass 49/49 across Library,
  Study, retained custody and Rust telemetry. This supplemental run used Node
  26.12.0; the decisive before/after comparison above uses pinned Node 22.22.0.
- [Empty-state comparison](compatibility.json): a newly compiled native helper
  emits rows and archive bytes identical to the retained fixed-parent outputs.
  [Fmt](logs/fmt.txt), [native Clippy](logs/clippy-native.txt),
  [WASM Clippy](logs/clippy-wasm.txt), [WASM check](logs/wasm-check.txt),
  [locked Worker build](logs/wasm-build.txt) and [CLI build](logs/cli-build.txt) pass.

The [first candidate harness run](logs/runtime-harness-initial-failure.txt)
reached the correct replay refusals, then failed four cases by asking for due
work after a scheduled answer and using the absent Prompt. The harness now
requests the retained Card explicitly. That failed run is retained, not counted
as a pass. The earlier Node 26 red/green runs and run-owned state remain under
`~/.cache/tmp/scry-343-fix/`.

## Reproduce

Use the clean isolated environment described by [Scry QA](../../../../.agents/skills/scry-qa/SKILL.md).
No private env file, provider capability or remote credential enters these runs.
The new fixtures use authored text, actual compiled CLI, Worker, SQLite and
local restore, with zero provider egress and no paid work. The existing clock
adapter and post-Rust body drain are unchanged.

```sh
cargo test --locked --test study
cargo test --locked --test application --test archive_integrity --test schedule_history --test learning_policy --test fsrs_golden --test concept_policy --test library --test revival_custody --test revival_receipts --lib
cargo fmt --all -- --check
cargo clippy --locked --all-targets -- -D warnings
cargo clippy --locked --target wasm32-unknown-unknown -- -D warnings
cargo check --locked --target wasm32-unknown-unknown
cargo build --locked --bin scry
worker-build --release --locked
node --test qa/study-integrity.test.mjs
node --test qa/study.test.mjs qa/library.test.mjs qa/revival-preserve.test.mjs qa/rust-telemetry.test.mjs
```

For the frozen comparison, set `SCRY_WORKER_ARTIFACT` and `SCRY_STUDY_CLI` to
the matching artifact set. `SCRY_STUDY_PROOF_DIR` selects the observation output.
The new regression file is included in the existing source-snapshot CI gate.
[Source and artifact evidence](source-artifact.json) records compile inputs and
both artifact sets as arrays of paths and SHA-256 values. The retained pre-fix
Worker was built from `f10e170`, whose complete tree equals `3344c8b`; that is
source-behavior carry, not a newly built release artifact.

## Limits and child integration

These are builder development observations from the worktree, not independent
QA, review, a judge ruling, protected CI or release-artifact proof. Root owns
those next steps. No merge, deployment, Home action, provider call, production
write or historical-store change occurred. The source Gitleaks receipt is
retained in [its log](logs/gitleaks-source.txt): normal Gitleaks 8.30.1
`dir --redact --no-banner` passes, with the repository's existing policy and
no new ignore file, override or allowance.

Empty old states retain their bytes. Pre-fix populated `study-v1` Attempts lack
explicit Reveal history and are refused rather than guessed from `assisted`.
New populated practice requires a compatible forward writer; rollback to an
older writer is unsafe. No migration or activation is claimed.

The phone child [PR #345](https://github.com/misty-step/scry/pull/345) was read,
not edited or copied. Its disclosure Attempt initializer must set `revealed`
to true. Normal answer initializers retain the frozen presentation's value.
Its validator must pass `a.revealed` to `effect`, whose boolean now means explicit
Reveal, and retain its separate disclosure-effect branch. Merge those model and
validation changes ordinarily after the parent, then repeat child QA.

Literal cold-filesystem DB/WAL/SHM nonallocation remains FAIL under the accepted
parent exception. Physical phone, private canonical wording, hosted ingress,
real-provider usefulness, independent hosted recovery and activation remain
UNVERIFIED. The earlier DNS-blocked gate and #344 backup flake are not green.
Long Recitation capacity remains the separate #347 follow-up.
