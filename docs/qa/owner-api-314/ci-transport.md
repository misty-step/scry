# US-014.3 CI transport diagnosis

Observed 2026-10-20 against application source
`17ae38bf07f5d062e06ba3065642f149e5b323a0`, PR [#342](https://github.com/misty-step/scry/pull/342).
The integration base is `31de4f7934ae1a1fc73b848879bdfb1ce534986d`.
This receipt covers the QA adapter delta only. [SPEC US-014.3](../../../SPEC.md#us-014-drive-scry-with-an-owner-token)
remains the authority.

## Failure and cause

[Exact-head CI job 118775049777](https://github.com/misty-step/scry/actions/runs/39465310995/job/118775049777)
passed 72/73 runtime tests. The 5 MiB unauthenticated POST in
`qa/owner-api.mjs` failed in `ProxyClientBridge.dispatchFetch`. Earlier release
head `c979` and independent release verification
[39466689664](https://github.com/misty-step/scry/actions/runs/39466689664)
also failed at this request. The deliberate listener-contention test passed;
its later bind error does not explain this failure.

The local probe exposed the nested cause:

```text
TypeError: fetch failed
  at ProxyClientBridge.dispatchFetch (.../miniflare/dist/src/index.js:83182:22)
  at .../qa/owner-api.mjs:82:23
cause: Error: write ECONNRESET
```

Limiting the probe and its workerd child to one available CPU made the race
frequent. A tiny Worker returning 401 without reading its body reproduced it
without Rust, SQLite, R2, credentials, or inspection. Both `mf.dispatchFetch`
and native Node fetch also reproduced it. This excludes application
authentication, the inspection wrapper, and a particular fetch entrypoint as
the cause.

workerd cancels an unread upload after an early response, and its connection
reset can destroy that response before the client consumes it. The upstream
[workerd report #7634](https://github.com/cloudflare/workerd/issues/7634)
describes this lifecycle, including Miniflare's service-binding hop.
[Wrangler drains unused request bodies after the handler returns](https://github.com/cloudflare/workers-sdk/blob/main/packages/wrangler/templates/middleware/middleware-ensure-req-body-drained.ts).
Applying only that post-handler drain to the tiny Worker removed the failure.

## Patch and observations

The existing QA adapter now awaits Rust's response, records whether Rust used
the forwarded request body, and discards any unread body one chunk at a time.
It returns the original response. The drain runs after application handling,
does not concatenate or retain the upload, and does not catch transport errors.
The deployed Worker, dependencies, token policy, and persistence are unchanged.

The existing US-014.3 walk still sends 5 MiB, requires exactly the ordinary 401
status and body, checks byte-identical records, and tests malformed learner
state. It additionally requires the body-used observation to be false before
the transport drain. A scratch artifact deliberately changed to consume the
body before Rust authentication fails that assertion with `true !== false`.

All counts below are diagnostic repetitions, not retries used to pass a test.
Every transport failure made its probe exit nonzero. Each request kept the
full 5 MiB body. State comparisons below used actual workerd SQL inspection.

| Probe | Before | After | Additional observation |
| --- | --- | --- | --- |
| Original journey, criteria 1–3, fresh runtime each time, unrestricted CPUs | 20/20 pass | Not repeated | Existing local success did not settle CI |
| Original journey, criteria 1–3, one CPU | 3/10 pass, 7 fetch failures | 10/10 pass | Exact original failure path; no unread-body assertion failures |
| Built Worker, repeated invalid-bearer requests, one CPU | 431/1,000 pass, 569 resets | 1,000/1,000 pass | Records byte-identical before and after |
| Tiny Worker, same QA adapter and inspection, one CPU | 44/100 pass, 56 resets | 100/100 pass with post-handler drain | Rust not involved |
| Tiny Worker, no inspection or persistence, one CPU | 52/100 pass, 48 resets | Not repeated | Inspection not required |
| Built Worker via `mf.dispatchFetch`, one CPU | 89/100 pass, 11 resets | Not repeated | Records byte-identical |
| Tiny Worker via native Node fetch, one CPU | 87/100 pass, 13 resets | Not repeated | Changing the client alone is insufficient |
| Scratch Worker with forced pre-auth body consumption | Not run | Fails unread-body assertion | The drain does not conceal application buffering |

The compiled application bytes were identical across the before/after runs:

```text
build/index.js       33e54df305c388a6e0a1ebac3d1c9974a3a278b36247bff8b394e72f6d0fb37f
build/index_bg.wasm  d949d867444b163779f42e74ce4b5f78456d2b1a7b5c1830a0efee9bb33d80ba
```

Environment: Node 22.22.0, Rust 1.98.1, worker-build 0.8.7,
lockfile-pinned Miniflare 5.20260903.0-alpha / workerd 1.20260903.1.
Builds and probes used an allowlisted `env -i`, fresh synthetic namespaces,
denied provider egress, and scratch under `~/.cache/tmp`. No private env file
or live store was used.

The builder ran the same six-file runtime group as the repository gate:

```sh
node --test qa/runtime.test.mjs qa/owner-api.test.mjs qa/library.test.mjs \
  qa/spend.test.mjs qa/revival-preserve.test.mjs qa/rust-telemetry.test.mjs
```

Result: 72/73 PASS. All eight US-014 criteria, Library and spend tests passed.
The remaining failure was the chief-backup stale-health assertion at
`qa/runtime.test.mjs:163`, which observed 200 instead of 503. A scratch copy of
the unmodified `17ae` test and helper, stopped after that backup criterion,
reproduced the same failure at the same line. Its state, auth, and backup
assertions were unchanged. The original full runtime file separately passed
35/35, so this is an unstable baseline failure; its cause remains undiagnosed
here. It is not a passing full-runtime gate. The logs are
`runtime-after.log`, `backup-baseline-1.log`, and `runtime-baseline.log` in the
retained scratch directory below.

## Reproduce

Build the Worker and install the pinned dependencies using README instructions.
With Node 22.22.0 on PATH, run this from the repository root on Linux. The
one-CPU constraint is inherited by Node and workerd. This loop stops at the
first failed assertion; it accepts no fetch exception.

```sh
scry_probe=$(mktemp "$HOME/.cache/tmp/scry-owner-refusal.XXXXXXXX.mjs")
cat > "$scry_probe" <<'JS'
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
const {ownerJourney}=await import(pathToFileURL(resolve('qa/owner-api.mjs')));
for(let i=0;i<10;i++) {
  const journey=await ownerJourney();
  try {
    await journey.criteria[0]();
    await journey.criteria[1]();
    await journey.criteria[2]();
    console.log(`US-014.3 iteration ${i+1}: PASS`);
  } finally {await journey.close();}
}
JS
env -i PATH="$PATH" HOME="$HOME" TMPDIR="$HOME/.cache/tmp" \
  WRANGLER_SEND_METRICS=false CLOUDFLARE_LOAD_DEV_VARS_FROM_DOT_ENV=false \
  python3 -c 'import os,sys; os.sched_setaffinity(0,{min(os.sched_getaffinity(0))}); os.execvp("node",["node",sys.argv[1]])' "$scry_probe"
```

The actual diagnosis runner continued after failures to count them, captured
only sanitized error fields, and exited nonzero for any failure. Its scripts,
the original CI log, and before/after transcripts remain under
`~/.cache/tmp/scry-owner-ci-fix/` and `~/.cache/tmp/scry-spend-ci/`.

## Limits and inherited evidence

This is local synthetic runtime proof. Fresh independent delta QA, review,
judge, and protected exact-head CI remain the parent task's responsibility.
There is no deployed ingress, physical-phone, provider, or hosted recovery
claim, and no release activation.

The spend implementation and its prior [independent QA](https://github.com/misty-step/scry/pull/342#issuecomment-6297030739),
[formal review](https://github.com/misty-step/scry/pull/342#pullrequestreview-5588993361),
and [judge decision at 17ae](https://github.com/misty-step/scry/pull/342#issuecomment-6297166222)
retain their original scope. The judge accepted one-time platform file
allocation while persisted application metadata/content remains read-only.
The literal cold-filesystem guarantee remains FAIL; no prewarming was added.
Populated attribution rollback remains unsafe and needs a compatible forward
fix. This patch adds no storage index or architecture and does not settle
activation or hosted recovery.
