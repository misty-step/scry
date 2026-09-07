# Scry QA System


## Purpose

The QA system is the repeatable proof path for Scry. It is designed
to answer two questions:

1. Does the public API still execute the learning semantics consumers depend on?
2. Where can quality improve beyond pass/fail bug finding?

The executable entrypoint is:

```sh
cargo run -p memory-engine-qa -- --local
cargo run -p memory-engine-qa -- --full
```

`cargo run -p memory-engine-qa -- --local` is the inner loop.
`cargo run -p memory-engine-qa -- --full` is the handoff path and ends with the
full `bun run ci:full` Dagger gate.

## Quality Model

QA evidence is organized around product quality, not implementation folders:

- API integrity: Rust facade exports compose without private crate imports.
- Learning semantics: scheduling, grading, progression, and queue behavior stay
  stable against fixtures and regression corpus cases.
- Contract usefulness: testkit fixtures and adapter doubles remain valid
  consumer-facing contracts.
- Boundary clarity: the Rust service boundary and dogfood clients keep persistence,
  UI, authored content, confidence, and session choreography outside the kernel.
- Drift detection: evals and benchmark receipts expose behavior and performance
  changes before clients absorb them.
- Science traceability: adopted learning-science principles remain tied to
  cited doctrine plus executable tests or benchmark receipts in
  `docs/science/README.md`.
- Handoff confidence: the Bun browser lifecycle contract, retained recovery
  boundaries, Rust formatting/tests/Clippy/rustdoc, latency budgets, the exact
  Wasm build, actual local workerd smoke, and Gitleaks all pass. `bun run ci:local`
  runs the local lanes plus Worker build/smoke; `bun run ci:full` repeats them
  under Dagger and binds Postgres 16 with `MEMORY_ENGINE_POSTGRES_TEST_URL`.
  Native Postgres remains a consequential compatibility/migration reference,
  not the production destination. Neither gate has deployment credentials.

## Executable Lanes

`crates/memory-engine-qa` runs these lanes in a fixed order and prints a
receipt after each lane:

| Lane | Surface | Purpose |
|---|---|---|
| `static.rustfmt` | all Rust crates | keep checked-in Rust in canonical format |
| `static.clippy` | all Rust targets | catch correctness, maintainability, and API-shape warnings |
| `api.facade` | `memory-engine` facade crate | prove consumers can compose root, modular, testkit, and dogfood surfaces |
| `kernel.core` | `memory-engine-core` | protect pure learning semantics, queue deferral semantics, and adapter contracts |
| `service.prototype` | `memory-engine-service` command boundary | prove command flow, injected persistence, and failure semantics |
| `persistence.beta-store` | `memory-engine-persistence` durable beta store | prove persisted snapshots, restart, conflict, and validation semantics |
| `generation.beta` | `memory-engine-generation` deterministic generation probe | prove source parsing, provenance, draft validation, and promotion behavior |
| `study.beta-session` | `memory-engine-study` session/API boundary | prove source, generation, approval, reveal, answer, post-answer feedback, concept health, skip/snooze, reference, bridge, queue, and resume flow |
| `app.beta-http` | `memory-engine-beta-app` local HTTP routes | prove mobile routes and validation run through the Rust study session |
| `api.v1-contract` | versioned public JSON contract and consumer proof binary | run the Scry-facing client against a local HTTP API and prove contract fixtures stay executable |
| `dogfood.rust-receipts` | Rust CLI, import probe, web shell | exercise migrated dogfood clients through the Rust facade and service crates |
| `docs.rustdoc` | all public Rust crates | prove public API documentation compiles |
| `performance.benchmarks` | Rust facade, scheduler, queue, service, science receipts | expose migrated-runtime and learning-policy drift without brittle thresholds |
| `ci.full` | Dagger CI | prove browser/recovery contracts, native file/Postgres tests, Rust fmt/Clippy/doc, action-latency budgets, actual Wasm/workerd, and Gitleaks together |

All lanes are gating except `performance.benchmarks`, which is receipt-only
until the project has enough historical data to define stable budgets.

### Cloudflare runtime proof

The production destination is `memory-engine-cloudflare`, not an Axum process.
A green native suite cannot establish Worker/SQLite/alarm/R2/mail behavior.
The repo-owned executable gate is:

```sh
bun run worker:tools
bun run worker:gate
# Or retain an explicitly named immutable candidate and proof:
bun run worker:build --out target/cloudflare/candidate
bun run worker:smoke --artifact target/cloudflare/candidate \
  --receipt target/cloudflare/candidate-workerd-proof.json
```

The same `scripts/scry-cloudflare gate` runs in Dagger's `worker` function.
`worker-build 0.8.5`, `wasm-bindgen 0.2.125`, `esbuild 0.28.1`, and
`Wrangler 4.129.0` are pinned; Cargo and npm dependency graphs are locked.
No synthetic server or substitute provider stands in for the application.
The exact packaged JS/Wasm runs in local workerd with isolated Durable Object
SQLite and R2 state, local mail mode, and no inherited model/Cloudflare secrets.
The gate first activates its new private actor through authenticated
`GET /internal/migration/fingerprint` and fingerprint-guarded
`POST /internal/runtime`, then observes real readiness. On restart it reads the
persisted active state without another activation. It exercises the shared
`/static/app.js` asset path, anonymous rejection, service-session auth,
alarm-driven LocalOnly structured generation, explicit draft acceptance,
isolation between two real accounts, review resume, assisted grading across
restart, durable idempotent replay without extra history/exposure, and the schema
fingerprint. Its privacy-safe receipt binds observations to
the exact artifact hash and source revision. It does not prove mail inbox
placement, paid model quality, production data continuity, or browser timing.

Export the Dagger-built bundle and its workerd proof when needed:

```sh
dagger call worker --source=. --git-sha="$(git rev-parse HEAD)" \
  export --path=target/cloudflare/dagger-proof
```

Remote bootstrap has a different acceptance boundary: health/static assets,
authenticated schema access, `maintenance: true`, and 503 learner/readiness
fences. A paused bootstrap receipt cannot authorize production. Staging must
first run `release:traffic --enable`; its verified receipt must record
`runtime_proof.maintenance: false`, `readiness: "ready"`, and
`public_smoke: "passed"` for the exact bundle and still-deployed staging version.
Traffic receipts check the immutable version before/after the application-state
mutation; no secret update, rebuild, or version deployment implements activation.
Production additionally requires the explicit source-quiesced declaration,
verified PostgreSQL primary-import provenance, and matched imported-state
fingerprint. Main must exercise pause/activation, busy-operation rejection,
alarm fencing, and independent recovery readback against the actual Worker.

**Production cutover proof remains pending.** Staging Worker
`https://scry-staging.misty-step.workers.dev` is live and isolated
(2026-09-07 receipts in the runbook). The native DigitalOcean/Postgres
service remains authoritative until Main records the source writer barrier,
final import, recovery, activation, and public proof. The approved free
production origin is `https://scry.misty-step.workers.dev`, without a
registrar prerequisite. Preserved records do not migrate browser cookies
across hostnames. Never relabel staging smoke, Resend API acceptance, or a
reachable hostname as deployed production migration or inbox-delivery
evidence.

The capture-anything path adds a focused generation receipt:

```sh
cargo run -p memory-engine-bench -- generation
```

That receipt is still local and deterministic, but it is not a raw performance
benchmark: its `shape` column must stay green for the intent fixtures that
cover verbatim memorization, concept understanding, fact recall, and
procedure/process capture, its `variants` column scores same-concept same-stage
phrasing variety without answer leakage, its `dup` column uses the same
near-duplicate predicate as source generation, and its bridge fixture must stay
green for lower-stage, same-concept, non-duplicate bridge material. Live model
quality comparisons stay explicit and write dated receipts under `docs/evals/`:

```sh
cargo run -p memory-engine-bench -- generation \
  --model google/gemini-3.7-flash \
  --prompt principled \
  --judge anthropic/claude-sonnet-4.6 \
  --out docs/evals/generation-gemini-3.7-flash-judged-$(date +%F).md
```

### Observability and external monitor proof

Canary was retired by Estate ADR 0003's 2026-08-30 amendment; no live
replacement telemetry-ingest endpoint exists. The supported implementation is
Worker-native logging in `crates/memory-engine-cloudflare/src/telemetry.rs`
and the independent repository-owned `scripts/scry-monitor`. Neither points
at the service prototype as a production runtime.

Worker logging reconstructs allowlisted, content-free browser receipts,
bounded performance aggregates, and observed actor/recovery health. Account,
cookie, CSRF, source, answer, and feedback content do not enter these telemetry
records. A valid authenticated `POST /app/performance/submit` returns an empty
**202** response with `x-scry-telemetry-delivery: runtime_logged` and
`x-scry-telemetry-attempts: 0`: the receipt was logged by the runtime, with zero
external delivery attempts. Isolate-local aggregation is best effort. This is
not remote ingest acceptance, durable telemetry storage, or external readback.

The external monitor observes three public GET witnesses without learner,
admin, or Worker deployment credentials:

| Witness | Healthy observation | Boundary |
| --- | --- | --- |
| `/healthz` | 2xx JSON with `status: "ok"` | SQLite accessibility; can remain healthy while paused. |
| `/readyz` | 2xx JSON with `status: "ready"` | Learner readiness; a paused actor returns 503. |
| `/statusz` | 200 JSON with `schema: "memory_engine.runtime_health.v1"`, `status: "healthy"`, `maintenance: false`, and integer `backupAgeMs` from 0 through 90,000,000 ms (25 hours), inclusive | Active actor plus recovery freshness. Paused, missing, future-dated, or stale backup evidence yields 503 with `status: "degraded"`; `backupAgeMs` is an integer or null. |

All three routes are observational: they do not schedule alarms, wake jobs, or
repair the backup being checked. The monitor rejects redirects, malformed
health responses, and missing or invalid recovery freshness rather than
treating an HTTP response alone as health.

The POSIX CLI below observes the canonical staging Worker and may send real
operator mail. `bun run ops:monitor` is the same entry point. Supply
`RESEND_API_KEY`, `MEMORY_ENGINE_MAIL_FROM`, and `MEMORY_ENGINE_ALERT_TO`
through the operator environment, never committed values:

```sh
python3 scripts/scry-monitor --environment staging \
  --state-file target/scry-monitor/staging/state.json \
  --receipt-file target/scry-monitor/staging/receipt.json
# Explicit master-only workflow invocation; the label requests a real mail drill:
gh workflow run production-health.yml --ref master \
  -f environment=staging -f delivery_drill=staging-mail-proof
```

`--environment production` selects the canonical production origin. State and
receipt paths must differ; state is private notification bookkeeping and the
`scry.monitor.receipt.v1` receipt is sanitized JSON. Local mail configuration
is validated on every invocation, even an initially healthy run with no mail.
An initial healthy observation is quiet; a sustained unhealthy incident and
its recovery each send once while notification state survives. Unaccepted
mail retains its original payload and idempotency key for retry; the old
observation is delivered before current health is reconciled.
The optional CLI `--delivery-drill LABEL` or workflow `delivery_drill` input
labels a mail drill without asserting an incident. Repeating the latest
accepted drill label is quiet while its state survives.

| Receipt | What it establishes |
| --- | --- |
| `status: "healthy"` | All three public health checks passed; it says nothing about mail. |
| `result: "ok"` | Health checks and local configuration/state/notification processing completed without a recorded error. A quiet run is not provider-acceptance proof. |
| `delivery: "provider_accepted"` with `receiptId` | A Resend 2xx JSON response contained a valid acceptance ID; not inbox delivery. |
| `delivery: "already_provider_accepted"` | The latest drill's earlier acceptance was found in notification state; no new provider request or inbox observation. |
| `delivery: "acceptance_unconfirmed"` | A mail request was attempted but no valid provider-acceptance receipt was obtained; acceptance or delivery must not be inferred. |
| `delivery: "not_attempted"` | Notification failed before a provider request, such as invalid local configuration or a changed pending envelope. |

`.github/workflows/production-health.yml` checks out `master` only and
serializes runs per target environment. Restrict its `production-monitor`
GitHub environment to `master`; only that environment supplies the three
operator mail secrets above, not learner, admin, magic-link, or Worker
deployment keys. A `master` manual dispatch can select staging or production
and bypasses `SCRY_MONITOR_ENABLED`; automatic five-minute scheduling requires
that repository variable to be `true`. It remains `false` until Main's
cutover. A disabled/skipped probe is not production monitoring.

GitHub cron can be delayed, dropped, or disabled and is not an availability
SLA. Environment-scoped default-branch cache is best effort, not durable alert
history: expiry/loss can duplicate alerts or lose recovery context, and
Resend idempotency expires after 24 hours. Delayed retries describe the
original observation, not necessarily current health. Workflow artifacts
retain sanitized receipts for 14 days; neither a green workflow nor provider
acceptance establishes inbox delivery or continuous monitoring.

Current evidence is local: real workerd exercised browser identity isolation,
paused SSE fencing, R2 retrieval/separate-object restore, and `/statusz`
recovery health. The loopback HTTP checks in `scripts/scry-monitor.test.py`
passed missing-backup, failed-acceptance stable-retry, sustained-incident and
recovery deduplication, and redirect-refusal scenarios. These are not live
Worker, external telemetry readback, or real mail-delivery receipts.
Final full gates for this revision and deployed logging, alert acceptance,
separate inbox proof, and scheduled production observations remain Main-owned
evidence to record in [the runbook](../runbook.md), not claims made here.

## Operating Procedure

Use this sequence for QA work:

```sh
cargo run -p memory-engine-qa -- --local
cargo run -p memory-engine-qa -- --full
```

For a focused change, run the affected surface first, then finish with the QA
harness. Examples:

```sh
cargo test -p memory-engine-core
cargo test -p memory-engine
cargo test -p memory-engine-study
cargo test -p memory-engine-api review_escape_hatches
cargo test -p memory-engine-api post_answer_feedback
cargo test -p memory-engine-openrouter
cargo run -p memory-engine-bench -- generation
```

Report QA evidence with exact commands, final status, surfaces exercised, and
any unrun ticket-required proof oracle. Do not claim beta/product proof from
local package tests alone.

## Improvement Review

After every full QA pass, update `docs/qa/quality-register.md` when the run
reveals a quality opportunity. A register item does not need to be a bug. Good
items include missing scenario coverage, unclear API ergonomics, weak fixture
corpora, benchmark gaps, consumer-proof gaps, or dogfood friction.

Register entries should name:

- quality dimension
- current evidence
- improvement
- trigger for creating a shaped GitHub issue

Do not use the register for vague wishes. If an item is actionable now and
blocks the active work, fix it instead of recording it.
