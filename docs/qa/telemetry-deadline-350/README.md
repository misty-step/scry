# Telemetry recovery deadline control

[Issue #350](https://github.com/misty-step/scry/issues/350), under
[plan #313](https://github.com/misty-step/scry/issues/313). Story: none.
This checks the test deadline against SPEC's
[private runtime failure reporting](../../../SPEC.md#private-runtime-failure-reporting)
contract and S09.3 synthetic isolation. It does not change application behavior.

The old test schedules its second request at `started + 61,500 ms`, with
`started` recorded before the first request. Startup consumes that wait, while
the reporter starts its one-minute cap at send time and its 429 backoff when
it processes the response. The fixed test observes the existing Worker drop
warning, which follows 429 backoff processing and response-body cancellation,
then waits a full 60,000 ms. Miniflare's structured-log callback observes that
warning without altering the frozen Worker. Sink receipt or the Node egress
callback's return would precede Worker processing and are unsuitable anchors.
The test timeout cancels a missing observation instead of waiting indefinitely.

`reproduce.py` runs only the existing same-isolate recovery test and retains its
original two-send and subsequent Create response assertions. It delays the
first request by 2,200 ms, after the old anchor, and holds the first 429 response
in the Node bridge for 800 ms. The reporter still uses real time, its original
one-minute policy, its two-second fetch timeout, and the same SQLite/R2 actor.
The control observes sink receipt, bridge response/return, the completed-drop
warning, and the second request. It asserts their order and the original
failure or corrected recovery. These delays force the mechanism; they do not
estimate the frequency of naturally slow startup.

From an isolated checkout with README's pinned tools and lockfile dependencies
installed, run each control once against the same built artifact:

```sh
proof_dir="$(mktemp -d "$HOME/.cache/tmp/scry-350-XXXXXX")"
clean() {
  env -i HOME="$HOME" TMPDIR="$proof_dir" \
    PATH="$HOME/.local/share/mise/installs/node/22.22.0/bin:$HOME/.cargo/bin:/usr/local/bin:/usr/bin:/bin" \
    WRANGLER_SEND_METRICS=false CLOUDFLARE_LOAD_DEV_VARS_FROM_DOT_ENV=false "$@"
}
clean env SCRY_SOURCE_REVISION="$(git rev-parse HEAD)" worker-build --release --locked
clean python3 docs/qa/telemetry-deadline-350/reproduce.py \
  --source . --artifact build --test-revision 99415ae0336aa38fe08ae9532c8be82bc3b4024f \
  --expect red --out "$proof_dir/before"
clean python3 docs/qa/telemetry-deadline-350/reproduce.py \
  --source . --artifact build --expect green --out "$proof_dir/after"
```

The base control uses the current helper's observation hook. Its deadline and
recovery assertion come from the named base revision. Generated probes, complete
logs, times, hashes, and verdicts remain in each output directory. The runner
passes only PATH, HOME, TMPDIR and explicit local test settings to Node; the
fixture permits only the controlled loopback telemetry sink. It never loads
private env files, providers, live credentials, operator configuration, or a live
notebook. Each invocation disposes only its own Miniflare runtime and sink.

[Recorded evidence](evidence.json) binds the observed controls and normal shared
runtime group. Before: expected two reports, observed one; the second request
was 56,368 ms after completed rejection. After: two reports, with the second
request 60,010 ms after completed rejection. Syntax, formatting, walk-runner,
release, promotion, daily-backup, and retained historical recovery checks passed.

The fresh 0.3.0 eight-file group is **102/103 FAIL**, zero skips, one run.
Recovery passed; the unchanged reject-sink no-delay check failed its
`elapsed < 1000` assertion. Its elapsed value is logged only after the assertion,
so the red does not retain the exact duration or establish its cause. There was
no full-group retry and no relaxation of that assertion.

One separate diagnostic delayed the local 429 reply by 1,500 ms, logged timing
before the existing assertion, and asserted that failure and subsequent Create
finish before the sink replies. It passed the original no-delay/privacy/retry
assertions: failure response at `1792526170602`, subsequent Create at
`1792526170638`, sink reply at `1792526172118`; measured failure plus Create
was 48.34 ms. Its generated script, log, hashes and events remain in
`post-release/async-reject-diagnostic.*`. This proves asynchronous reporting in
that control, not the cause of the group red. Attribution to load, version or
another change remains unverified. The root judge owns its disposition.

This qualification freshly built ordinary merge
`31285a46f946fda16646b360cde1b20fb0f78146`, froze its Worker, and reused those
exact bytes for both controls and the group. That merge preserves the original
implementation commit and brings in release
`69435af34fe7a8be7b5683f8b5d3e6b27852e177`, whose six files change only version
and changelog information. The Worker reports the merge revision and version
0.3.0. The change affects only QA and these receipts;
`git diff 31285a46 -- . ':!qa' ':!docs'` is empty, including every Rust, asset,
loader source, manifest, lockfile, and toolchain input. This is compile-input
equivalence, not a fresh build of the final PR revision. Protected CI must build
that committed source afresh; the root agent owns CI, independent QA/review,
judgment and merging.

The earlier 0.2.0 controls and 103/103 group remain at
[the original implementation commit](https://github.com/misty-step/scry/blob/34f9d34c9bec16f97ad7ebcbe4f71cd28671fa2c/docs/qa/telemetry-deadline-350/evidence.json)
and `~/.cache/tmp/scry-350-builder/`. They are not relabeled as 0.3.0 proof.
The new frozen Worker, controls and group are under its `post-release/` directory.

The historical 72/73 group red and earlier controlled red remain recorded with
their original hashes. The historical group lacks actual send timestamps, so
these controls do not diagnose it or attribute it to backup work or load.
This synthetic local proof does not certify owned remote alert delivery, phone
experience, deployed ingress, provider usefulness, remote recovery, or release.
