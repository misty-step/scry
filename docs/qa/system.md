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

The deployed boundary is `memory-engine-cloudflare`, not an Axum process. A
green native suite cannot establish Worker/SQLite/alarm/R2/mail behavior.
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

**Cutover proof remains pending until executed.** The approved free origin is
`https://scry.misty-step.workers.dev`, without a registrar prerequisite.
Main owns the source writer barrier, final import, separate-object restore,
activation, browser UI/timing, real generation cost/quality, Resend acceptance
and separate inbox/delivery proof, production smoke, and old-host
reverse-proxy/redirect retirement. Preserved records do not migrate browser
cookies across hostnames: prove a fresh workers.dev sign-in as well as imported
machine sessions and learning history. Record exact versions, source/bundle
hashes, commands, and actual results; never relabel local smoke or Resend API
acceptance as deployed migration or inbox-delivery evidence.

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
