---
name: verify-scry
description: Verify Scry's NATO review, prayer and Rosary lessons, practice chooser, and external-agent API/CLI/MCP on a disposable real Rust Worker. Use after app changes to collect source-bound behavior and visual evidence without production credentials or learner data.
---

# Verify Scry

Read [SPEC](../../../SPEC.md) for acceptance and the [feature map](../../../features/README.md)
for entry points. This skill drives existing harnesses; it does not create a second
release gate. Native answer grading is required. Never submit client success grades
or revive native content generation. Preserve existing content and history.

## Launch

Run from the checkout root. Use the pinned Rust toolchain in `rust-toolchain.toml`
and Node version in `package.json`; install `worker-build` as documented in
[the build procedure](../../../docs/rust-cloudflare.md). Do not source `.env`,
`.dev.vars` or operator credentials.

```sh
npm ci --ignore-scripts --no-audit --no-fund
cargo build --locked --bin scry --bin scry-mcp
worker-build --release --locked
npx playwright install chromium
```

Use the repository's lockfile-pinned browser, or set `SCRY_CRITICS_CHROMIUM_PATH`
to an already installed compatible Chromium. Browser installation is unnecessary
for runtime/API-only verification. If HOME is read-only, point `npm_config_cache`
and `XDG_CACHE_HOME` at writable workspace cache directories before installation
and building; do not change HOME security permissions. No personal computer or unlocked password store
is required for local proof.

The drive commands below launch a real compiled Worker with synthetic SQLite/R2,
seed their own content, deny external egress, and dispose of their own instances.
Readiness means the seeded user journey works, not merely an open port. Do not
run two commands that write the same `target/keyed-mapping` or `target/walk`
evidence concurrently in one checkout. Use a separate worktree for each run.

For manual visual inspection, `npm run dev` builds and starts a disposable local
Worker at `http://127.0.0.1:8787`; set `SCRY_DEV_PORT` for a separate port. Its
printed temporary directory is run-owned. Start through the native forms, not
internal state setters. The scripted flows remain the reproducible proof.

## Doctor

Run this read-only check before driving, and again after a surprising failure:

```sh
node --version
cargo --version
node -e "require('miniflare'); require('playwright'); console.log('pinned runtime modules available')"
test -s build/index.js && test -s build/index_bg.wasm
test -x target/debug/scry && test -x target/debug/scry-mcp
git rev-parse HEAD
git status --short
```

For an already running manual loopback instance, use
`.agents/skills/verify-scry/scripts/doctor.mjs local http://127.0.0.1:8787`.
It refuses non-loopback hosts. A sandbox socket/browser launch failure is an
unexecuted runtime check, not an application failure or a passing substitute.
Use the existing CI pipeline when available; retain its exact commit and logs.
Do not change security settings to make a browser launch.

## Drive

Choose the affected user journey. The first command is a small real-Worker proof;
the others cover different boundaries, not interchangeable substitutes.

```sh
# Continuous Study across due/unseen subjects, read-only home, native NATO, replay.
node --test qa/nato-focus.test.mjs

# NATO + practice chooser + Rosary + prayer; real native/enhanced phone/desktop UI.
node --test qa/keyed-mapping.test.mjs

# Compiled stdio MCP -> Owner API -> real Worker/SQLite, supplied content and grades.
node --test qa/mcp-worker.test.mjs

# Full source/build/runtime/browser gate used by the existing release pipeline.
python3 scripts/scry-ci full
```

The browser suite calls `lessonWalk` in `qa/lessons.mjs`. Its stable handles include
`.lesson-picker > summary`, `#answer`, `form[action="/practice/lesson/start"]`,
`[data-state="exact-result"]`, and `form[data-next]`. It exercises browsing and
return without losing a draft, current-session identity, version selection,
same-phrase retry, native forms, keyboard, and offline/uncertain-switch recovery.
Read the corresponding feature guide before changing that flow.

Default Study also runs `continuousStudyJourney` and `continuousLibraryJourney`: due-first
selection, all nine canonical subjects, custom/current Library versions, explicit pause/drop,
held results, 24-hour pacing, and read-only GET. Inspect `continuous-study-runtime.json`
and `continuous-library-runtime.json`; the phone/desktop continuous-study walk must
show one primary Study action, closed Browse, and no false no-reviews dead end.

For retained exact-text, Library or historical compatibility changes, use the
existing guide and `qa/walk --stories "US-016 US-017"` (or the appropriate SPEC IDs).
`qa/walk --all` discovers the active numbered stories. Do not treat an old
concept/generation journey as current product intent.

## Evidence

Before running, create a fresh directory outside the checkout and record source:

```sh
proof=$(mktemp -d "${TMPDIR:-/tmp}/scry-proof.XXXXXXXX")
git rev-parse HEAD > "$proof/source.txt"
git diff --binary HEAD > "$proof/source.patch"
git ls-files --others --exclude-standard > "$proof/untracked.txt"
sha256sum build/index.js build/index_bg.wasm target/debug/scry target/debug/scry-mcp > "$proof/artifacts.sha256"
# Example; save both output and the actual exit code, including on failure.
if node --test qa/nato-focus.test.mjs > "$proof/nato.tap" 2>&1; then
  result=0
else
  result=$?
fi
printf '%s\n' "$result" > "$proof/nato.exit"
cp target/keyed-mapping/focused-runtime.json "$proof/" 2>/dev/null || true
printf 'Evidence: %s; exit: %s\n' "$proof" "$result"
```

Review `untracked.txt`: record hashes and contents of any relevant untracked source
without copying private files, or commit the intended source and rerun. HEAD plus
a diff summary is not a source identity. A missing receipt or nonzero exit is not
PASS. Preserve failed output as well.
For browser proof, copy `target/keyed-mapping/` to the evidence directory after
completion; retain `browser.json`, screenshots and runtime transcripts together.
Story proof lives in `target/walk/`. MCP proof prints a source-artifact hash receipt
in stdout. Capture the actual command, exit, action and observed result. Confirm
stored effects through the harness's independent SQLite/CLI reads, not just a
final screenshot. No test-only setters may replace the user action under test.

Inspect screenshots as a learner: legible controls, visual hierarchy, useful
feedback, meaningful focus and no overflow at tested sizes. A scripted PASS is
not that judgment. State separately: native tests, local real Worker, browser
emulation, deployed byte identity, authenticated learner acceptance, and physical
phone. None proves the others. Review artifacts for secrets before any public PR
attachment; use only synthetic content. Keep the existing CI receipt rather than
inventing another release process.

## Cleanup

Scripted harnesses close their own browsers, subprocesses, temporary login files
and Worker stores in `finally`/test cleanup hooks, including failed iterations.
For a manual `npm run dev`, stop only its recorded launcher/child processes and
verify its port closes before removing its printed run directory. Never kill by
process name or remove an existing data store. Keep the external proof directory
and confirm its transcript/receipts still exist after cleanup.

## Helpers

No new runner is installed by this skill. Existing helpers are:

- `qa/nato-focus.test.mjs`: atomic native NATO journey and JSON receipt.
- `qa/keyed-mapping.test.mjs`: mappings, lessons and browser screenshots.
- `qa/mcp-worker.test.mjs`: compiled MCP authoring-to-graded-review proof.
- `qa/walk --stories "US-0NN"`: SPEC-indexed retained journeys.
- `scripts/rust-dev`: optional disposable manual loopback session.
- `scripts/scry-ci full`: existing aggregate source and runtime gate.

Production is a separate, explicitly scoped observation. Existing QA helpers
(`doctor.mjs prod`, `qa-signin.mjs`, `prod-states.mjs`) require their documented
credentials and current authorization; this skill grants neither. Never run
`enroll-qa.mjs`, reset credentials, alter Access, contact another operator, or
sign in as the owner as a setup prerequisite. See [sign-in](../../../features/sign-in.md)
and [the runbook](../../../docs/runbook.md). Missing access is UNPROVEN.

Use pstack's `maintain-verification-skill` when the feature map drifts. Preserve
SPEC and user intent instead of weakening a journey to turn it green.
