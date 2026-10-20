# Backup health gate diagnosis, 2026-10-20

Story: none. [Ticket #344](https://github.com/misty-step/scry/issues/344),
under [#313](https://github.com/misty-step/scry/issues/313).
Fixed base: `294f99808a352e066ca6a59cfaa8320eb37d2ee7`.
This is fixture reliability for S10.2, not a product acceptance change.

## Cause and repair

The chief-backup check reused the notebook from earlier capture checks. Its
photo capture still had a queued job and a one-second work alarm. The test
changed `meta.backup.completed_ms` directly without owning the notebook's
background writes. A due alarm reads that stale completion, makes a complete
backup, and writes a fresh completion. Health correctly returns 200 afterward.
The same alarm can also finish a photo job during the preservation comparison.

`/__internal/health` reads stored rows directly. No cache was changed. The
production alarm, backup, health, persistence, and QA transport helper are
unchanged. The check now restores the existing synthetic archive into its own
unused notebook, uses the normal restored-work pause, rebinds to preview, and
makes an initial verified backup. That arms its normal daily backup alarm,
without inheriting another check's one-second work alarm. No alarms are deleted
or disabled. The stale SQL fixture and all route, failure, complete archive,
checksum, photo, and history assertions remain. New assertions check that the
failed archive leaves the stale completion unchanged and successful recovery
advances it.

## Failing reproduction and counterfactual

[reproduce.py](reproduce.py) extracts the first 16 runtime checks and adds only
scratch inspection instrumentation. It preserves the affected assertions and
executes the actual Rust alarm through workerd's native `storage.setAlarm`.
The one controlled interleaving delivers the original capture notebook's alarm
between the SQL fixture update and health GET. An event promise waits for that
alarm's completion; there is no sleep, polling, retry, or replacement alarm
implementation. The candidate delivers the same shared notebook alarm, while
its health fixture belongs to a different notebook.

On the same compiled application bytes:

- Base: 15/16, the original stale assertion fails with `200 !== 503`.
- Candidate: 16/16, health stays `503 backup_stale`, incomplete backup stays
  503, and complete verified recovery returns 200.

The base probe observed `completed_ms=1792424088422`, revision 9, and pending
alarm `1792521289365`. Native alarm delivery then saved a different daily
snapshot at `completed_ms=1792521288518`, revision 11. Health returned 200
at `1792521288558`. The post-health SQL observation retained that same new
completion and snapshot key. In the candidate, delivery of the shared alarm
left its separate stale completion and revision unchanged; health returned 503.
[The retained signal](evidence.json) contains the actual observations and
transcript hashes. Raw command output remains under
`~/.cache/tmp/scry-344/paired-{base,fixed}/runtime.log`.

Reproduce from this checkout after installing the README's pinned tools and
building with `worker-build --release --locked`. Each output path must be new.
The base invocation is expected to exit 1; the candidate exits 0.

```sh
mkdir -p "$HOME/.cache/tmp"
scry_probe=$(mktemp -d "$HOME/.cache/tmp/scry-344-repro.XXXXXX")
env -i PATH="$HOME/.local/share/mise/installs/node/22.22.0/bin:$HOME/.cargo/bin:/usr/bin:/bin" \
  HOME="$HOME" TMPDIR="$HOME/.cache/tmp" WRANGLER_SEND_METRICS=false \
  CLOUDFLARE_LOAD_DEV_VARS_FROM_DOT_ENV=false \
  python3 docs/qa/backup-health-344/reproduce.py \
  --revision 294f99808a352e066ca6a59cfaa8320eb37d2ee7 --out "$scry_probe/base"
env -i PATH="$HOME/.local/share/mise/installs/node/22.22.0/bin:$HOME/.cargo/bin:/usr/bin:/bin" \
  HOME="$HOME" TMPDIR="$HOME/.cache/tmp" WRANGLER_SEND_METRICS=false \
  CLOUDFLARE_LOAD_DEV_VARS_FROM_DOT_ENV=false \
  python3 docs/qa/backup-health-344/reproduce.py --out "$scry_probe/candidate"
```

The controlled delivery explains an actual competing writer, rather than
claiming to recover timestamps from the older uninstrumented failure. Those
original logs remain unchanged at `~/.cache/tmp/scry-owner-ci-fix/`. The initial
natural instrumented runs were 5/5 green. Five one-CPU runs had two failures:
one at the later stale-health assertion after a recovery alarm, and one during
the goals comparison. Six concurrent first-16 probes on one CPU had one later
stale-health failure. These natural failures are retained with their actual
assertion locations; they are not relabeled as initial-stale failures.

## Validation and limits

All Worker invocations use allowlisted `env -i`, Node 22.22.0, pinned Miniflare
5.20260903.0-alpha and workerd 1.20260903.1, isolated SQLite and local R2,
synthetic RSA identity, and denied or stubbed egress. Rust is 1.98.1 and
worker-build is 0.8.7. `~/.cache/tmp/scry-344/run` retains the exact environment
wrapper. The base application WASM has SHA-256
`d949d867444b163779f42e74ce4b5f78456d2b1a7b5c1830a0efee9bb33d80ba`;
loader `index.js` has SHA-256
`33e54df305c388a6e0a1ebac3d1c9974a3a278b36247bff8b394e72f6d0fb37f`.
These are development artifacts with unverified-development build identity.
No Rust application source changes in this repair.

The first candidate full-runtime attempt failed 34/35 because its restore
setup included the synthetic operator binding. The real restore guard refused
it with 403. That setup now uses the existing restore-only binding set; the
failed transcript remains `runtime-after.log`. No guard was relaxed.

Focused native verification passed `cargo test --locked --lib --test
backup_health --test application`: 25 passed, with one existing ignored lib
test. The native CLI build, WASM target check, and native and WASM Clippy
passed. The Worker build passed on the fixed application base.

A scratch R2 proxy additionally supplies a same-size corrupt archive body on
backup GET. It leaves Rust encoding, PUT, checksum comparison, and health
untouched. The recovery check rejects the response, with health still stale;
its deliberately red transcript is `readback-recovery-control-v2.log`. Two
earlier scratch controls failed at the initial backup because the proxy bound
the native constructor and failed the Rust binding type check. Those logs are
retained; the corrected proxy preserves the constructor and reaches the actual
readback before corrupting it. This is a local fault control, not proof of remote R2 availability.

The full six-file runtime group finished **72/73 FAIL**, with all **35/35**
checks in `qa/runtime.test.mjs` passing. The distinct failure is
`qa/rust-telemetry.test.mjs:120`, "same-isolate reporting recovers after sink
rejection and the real one-minute bound": one received send, expected two.
The failed group is retained as `runtime-group.log`; it is not classified as
#344, and no telemetry source was changed. One subsequent isolated full-file
telemetry diagnostic passed 11/11 with zero skips. It does not replace the
failed group or establish the cause of that failure. No full-group retry was
run.

The final command results, bounded repeats, native test scope, and transcript
hashes are recorded in [evidence.json](evidence.json). Full protected CI and
independent critical-tier QA, Claude review, and judge remain the conductor's
responsibility. Physical phone, deployed private ingress, real providers,
independent hosted recovery, and activation are unverified here. No Study or
phone source, private env file, production data, integration, deployment, or
credential was used or changed.
