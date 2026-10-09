# Scry critics

The current advisory critic runs the Rust Worker in isolated local workerd with
SQLite Durable Objects and local R2. It drives ordinary browser input and binds
receipts to a frozen Rust source snapshot and the exact compiled Worker modules.
It does not deploy, import historical data, call providers, or establish useful
real generated material, efficacy, private production ingress, physical phone
acceptance, or hosted recovery.

[QA system](system.md) owns current proof entrypoints.
[The runbook](../runbook.md) remains the deployed Go Container's operational
authority. The rewrite candidate is described in
[Rust/Cloudflare](../rust-cloudflare.md). September Go receipts remain historical.

## Prepare an isolated VM

Use the explicit isolated VM setup from the QA system; never launch workstation
headless Chromium. The pinned setup provides Rust 1.98.1/WASM, worker-build
0.8.7, Node 22.22.0, Playwright 1.63.0 and Chromium. Install the lockfile's local
Worker runtime dependencies, without private configuration or credentials:

~~~sh
npm ci --ignore-scripts --no-audit --no-fund
export SCRY_CRITICS_PLAYWRIGHT_PATH="$HOME/.local/share/scry-walk/node_modules/playwright"
export SCRY_CRITICS_CHROMIUM_PATH="$HOME/.local/bin/chromium"
~~~

Project Playwright and its bundled executable are also supported; the explicit
paths take precedence. Older NODE_PATH, SCRY_CRITICS_PLAYWRIGHT and
CHROMIUM_PATH inputs remain compatibility options. Browser launch failure
writes a blocked receipt. Browser batteries fail when
SCRY_CRITICS_REQUIRE_BROWSER=1; otherwise an unavailable browser is a visible
skip, never browser proof.

## Create, walk, and stop a candidate

~~~sh
node scripts/critics/run.mjs candidate up \
  --dir target/critics/worker-001 --port 18080 --json

node scripts/critics/run.mjs human --goal practice-review \
  --candidate http://127.0.0.1:18080 \
  --handle target/critics/worker-001/candidate.json \
  --out target/critics/worker-001/review

node scripts/critics/run.mjs candidate down \
  --dir target/critics/worker-001
~~~

Candidate up requires a missing or empty directory and an unused loopback port.
It refuses existing content without deleting it. It snapshots Cargo manifests,
the pinned toolchain, Rust source and embedded assets; it builds that frozen
copy with worker-build --release --locked. It never substitutes a walking
checkout revision for an externally supplied artifact. The retired Go
--binary option is explicitly rejected.

The child receives synthetic bindings and a private empty HOME, without
inherited provider, cloud, database or recovery capabilities. The shared
qa/runtime-helper supplies actual workerd/SQLite/R2 and denies every external
service call. A small synthetic HTTP transport adapter forwards requests into
that Worker; the Rust app still enforces routes, Host/origin, CSRF, atomic state
and assistance. It seeds src/engine.rs's authored HTTP-caching material through
the ordinary demo POST with a fresh operation ID and CSRF token. This fixture is
already-read practice, not cold-recall or provider-quality evidence.

The v2 handle records the source inventory/hash, Git revision and clean/dirty
checkout label, each Worker module's hash, the combined artifact digest, child
PID, runner hash and a run-specific nonce token. Startup requires its owned
ready marker and a bounded real readiness/identity response. Walk binding
rechecks the PID and run directory, runner and both compiled files, a fresh
nonce response from the serving adapter, and the checkout revision. A stopped,
swapped, changed or foreign candidate is blocked before browser mutation.
Mutable source can be exercised, but its receipt remains dirty development
evidence. This narrow source inventory is critic provenance, not a committed
release export or the full source-snapshot gate's proof.

Inside the gate's Git-free frozen source, SCRY_SOURCE_META supplies the
revision, committed/worktree label and complete source hash already validated
by scripts/scry-ci before and after checks. The critic records that gate hash
separately from its actual Rust build input inventory. Without explicit
metadata, a live source root must own its Git checkout; a parent repository
cannot provide a borrowed revision.

Candidate down verifies the PID command belongs to its runner and run directory,
stops it, and marks the handle stopped. It retains source, logs, receipts and
screenshots. Local SQLite/R2 is disposable for that process lifetime; this
runner does not claim restart or independent hosted restore coverage.

The supported journey is practice-review: discover the live prompt using
accessibility and visible DOM observations, tap a choice or type the known
authored fixture answer, observe held feedback, then deliberately press Next.
An introduction is explicitly acknowledged when present. Leftover held feedback
can be advanced within the budget. The walker accepts the current app's implicit
submit buttons and keeps fixture-specific answer oracles in the runner.
Unknown states, uncertain discovery or an unsupported journey remain unverified.
Screenshots are ungraded, feedback and next under the output directory.

## Receipts and boundaries

The compatible scry-critic-receipt-v1 schema remains in
[scripts/critics/lib/receipt.mjs](../../scripts/critics/lib/receipt.mjs).
For a Worker v2 candidate, the legacy binary_sha256 field contains the combined
compiled module inventory digest; binary_revision names the frozen source's
Git HEAD. Candidate artifact and source_sha256 fields record the precise Worker
and source identity. They do not describe a Go executable.

A pass requires exercised coverage and notes plus existing screenshot evidence.
Finding identity is story + criterion + normalized mechanism, independent of
persona or prose. No findings with zero verified coverage is not a pass. An
unbound run has no claimed revision/digest. Without --handle its observation
cannot be cited as revision-bound evidence. Historical v1 handle validation is
retained for negative/guard fixture batteries; candidate up only emits Worker
v2 handles.

Exit codes are 0 for selected checks passing with evidence, 1 for findings,
2 for blocked environment/usage/budget, and 3 for unresolved observation.
A blocked run writes walk-execution: unverified when a receipt can be created.
Production scry.study and www.scry.study are always refused. An explicitly
authorized synthetic non-loopback target requires exact --allow-origin;
it has no local process attestation and cannot inherit local Worker proof.

Default limits are 24 steps, 120 seconds and 12 screenshots, configurable via
--max-steps, --timeout and --max-screenshots. An exhausted limit is blocked.
Navigation/discovery disagreement is unverified; a missing/broken claim needs
the expected surface and independent observation support. The critic is
criterion-bound and advisory, with no filing hooks, schedules or deployment.

## Batteries and historical evidence

~~~sh
node --test scripts/critics/tests/*.test.mjs
SCRY_CRITICS_REQUIRE_BROWSER=1 \
  node --test scripts/critics/tests/e2e/*.test.mjs
~~~

The first suite includes a real compiled Worker startup/seed/stop and artifact
replacement rejection, exact source freezing, origin/budget guards, dedupe and
receipt validation. Run it with the pinned build tools and local dev dependencies.
The browser suite includes current Worker review/Next and retained negative
HTML fixtures for missing feedback, no-op Next, process/handle mismatch,
budgets, denied origins and blocked browser environments. The fixture cases
prove critic failure behavior, not the application's behavior.

Historical Go lifecycle/buildinfo/browser-provenance tests are copied under
[scripts/critics/history](../../scripts/critics/history/README.md), excluded
from current test discovery. Their runnable original source lives at Git
c6ba395. The old provenance reader remains a historical format helper and is
never used for Rust startup. The 2026-09-19 receipts in
[critics/evidence](critics/evidence/) bind the former Go candidate only; their
passes do not transfer to this rewrite. Current proof needs a fresh Worker
receipt with its environment, source/artifact identity and explicit limits.

Keep each run's handle, build/serve logs, receipt and screenshots together.
The project's required source/runtime/story gates remain separate; an advisory
critic pass does not replace them or authorize activation.
