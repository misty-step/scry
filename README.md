# Scry

[![CI](https://github.com/misty-step/scry/actions/workflows/ci.yml/badge.svg)](https://github.com/misty-step/scry/actions/workflows/ci.yml)

Scry is the consumer product: **Remember everything.** It helps people learn and
memorize anything through a quiz-driven review loop. This repository contains
Scry's Rust engine, not a separate product category or a generic agent-memory
store.

The product has five faces over one capability system:

- **PWA** — the primary, phone-first human surface
- **CLI** — direct operator and power-user access
- **skill** — a product-facing agent workflow
- **MCP** — typed tools for agents and applications
- **API** — the service boundary used by the PWA and other clients

Beta access is invite-gated with a visible waitlist. Human sign-in uses magic
links only; there is no OAuth path. Machine faces use operator-gated service
sessions. Subscription is the intended business model. Public
signup remains closed until generation costs are bounded and privacy, reliability,
and Stripe billing are proven. Fast and smooth are product bars: p95 acknowledgement
below 100 ms, p95 graded-visible feedback below 300 ms, and first quiz visible below
20 s. See [VISION.md](./VISION.md) for the canonical product contract.

Scry keeps a pure Rust learning kernel and explicit boundary
crates. The kernel owns deterministic scheduling, grading, progression, queue
selection, and learning invariants. Boundary crates own persistence, generation,
sessions, identity, API routes, rendering, deployment, and QA.

The production destination is a Rust WebAssembly Worker on Cloudflare:
`memory-engine-cloudflare`, one SQLite-backed `Scry` Durable Object, leased
jobs/reminders driven by alarms, private R2 recovery, and Resend over Worker Fetch.
The pure learning engine and shared Rust renderer remain the implementation.
The approved interim origin is `https://scry.misty-step.workers.dev`; registrar
or custom-domain access is not a prerequisite for that origin.
**Live cutover is not implied by this source change.** The native
DigitalOcean/Postgres service remains authoritative until Main records the
stopped-source-writer barrier, final import/readback, recovery, activation, and
public proof. Keep the old host and its backups intact until then.
Preserved browser-session records do not move host-scoped cookies: users need
a fresh browser sign-in on workers.dev. Resend acceptance is not inbox delivery.
See [docs/runbook.md](./docs/runbook.md).

## What It Owns

- Canonical learning-domain types
- FSRS state transitions
- Deterministic grading
- Progression and queue primitives
- Recitation grading
- Async rubric grading contracts
- Vendor-neutral rubric adapter interfaces
- Fixture corpora for contract and interface tests
- Evals and benchmarks for learning-behavior regressions
- Experimental clients that consume the API outside the reusable kernel

The core runtime in `crates/memory-engine-core` stays framework-free: no
filesystem, network, UI, logging, model clients, or persistence. Service,
storage, UI, auth, content parsing, and deployment experiments live in dedicated
boundary crates until dogfood evidence proves a stable reusable contract.

## Status

The Rust migration is complete for the main runtime:

- canonical types
- FSRS scheduler wrapper
- deterministic grader
- progression metadata and eligibility helpers
- queue candidate filtering and selection
- deterministic recitation grading
- async rubric grading surface
- facade adapter/testkit modules
- service, persistence, generation, study, and local HTTP app hosts
- Rust QA and benchmark receipt runners

Cloudflare staging (`scry-staging`) and production (`scry`) have separate Durable
Object namespaces and recovery buckets. No custom-domain route is configured
before a separately reviewed DNS change. `memory-engine-api` remains a native
reference/compatibility test surface, not a deployment destination.
Fresh Worker actors start paused: learner traffic, readiness, and background
work remain fenced until the fingerprint-guarded `release:traffic --enable`.
Bootstrap verifies health/assets/schema and the pause, not learner readiness.
Production requires the same immutable bundle activated and publicly exercised
in staging, then Main's source barrier, imported-state and recovery evidence.
Runtime activation does not create another Worker version or rebuild.
Migration, mail delivery, and public cutover remain pending until actual
receipts are recorded in the runbook.

Current strategy and verification docs:

- [SPEC.md](./SPEC.md)
- [docs/qa/system.md](./docs/qa/system.md)
- [docs/runbook.md](./docs/runbook.md)
- [docs/rust-migration.md](./docs/rust-migration.md)

Historical extraction packets, retained as boundary evidence rather than
active delivery oracles:

- [SLICE-1-KERNEL.md](./SLICE-1-KERNEL.md)
- [SLICE-2-PROGRESSION.md](./SLICE-2-PROGRESSION.md)
- [SLICE-3-RUBRIC.md](./SLICE-3-RUBRIC.md)
- [SLICE-4-SERVICE-PROTOTYPE.md](./SLICE-4-SERVICE-PROTOTYPE.md)
- [exemplars.md](./exemplars.md)

Work starts from the current operator request, checked against live code and
overlapping work. [GitHub Issues](https://github.com/misty-step/scry/issues)
preserve historical context and evidence; an issue is optional for direct work.
Record ownership, the result, and verification evidence in the session or PR.

## Usage

Rust consumers should use the facade crate:

```rust
use memory_engine::{next, ExactPrompt, ExactPromptKind, GradeContext, Grader, Prompt, ReviewUnitId};

let prompt = Prompt::Exact(ExactPrompt {
    kind: ExactPromptKind::ShortAnswer,
    review_unit_id: ReviewUnitId::new("latin-1"),
    prompt: "Translate poena".to_owned(),
    accepted_answers: vec!["punishment".to_owned()],
    equivalence_groups: Vec::new(),
    ignored_tokens: Vec::new(),
});

let grade = Grader::new().grade(
    &prompt,
    "Punishment",
    GradeContext {
        response_time_ms: 3_200,
        prior_reps: 3,
    },
);

let next_state = next(None, grade.rating, 1_779_465_600_000).expect("schedule");
```

Rubric grading stays adapter-backed; the Rust core owns normalization and
dispatch, while callers own any model client:

```rust
use memory_engine::{
    AsyncGrader, GradeContext, GradeablePrompt, RubricAssessment, RubricCriterion,
    RubricCriterionResult, RubricCriterionVerdict, RubricDefinition, RubricPrompt,
    ReviewUnitId, StaticRubricGrader,
};

let prompt = RubricPrompt {
    review_unit_id: ReviewUnitId::new("rubric-1"),
    prompt: "Continue the prayer.".to_owned(),
    rubric: RubricDefinition {
        answer_guide: vec!["Continue with the next line.".to_owned()],
        passing_score: 1,
        criteria: vec![RubricCriterion {
            name: "continuation".to_owned(),
            description: "Gives the next line.".to_owned(),
            required: true,
        }],
    },
};
let grader = AsyncGrader::with_rubric_grader(StaticRubricGrader::new(RubricAssessment {
    model: Some("fixture".to_owned()),
    confidence: 1.0,
    feedback: "Strong answer.".to_owned(),
    criterion_results: vec![RubricCriterionResult {
        name: "continuation".to_owned(),
        verdict: RubricCriterionVerdict::Pass,
        evidence: "Supplied the continuation.".to_owned(),
    }],
}));
let grade = grader.grade_prompt(
    GradeablePrompt::Rubric(&prompt),
    "Strong answer.",
    GradeContext {
        response_time_ms: 6_000,
        prior_reps: 0,
    },
).expect("rubric grade");
```

Test fixtures for contract and interface tests:

```rust
use memory_engine::testkit::{grading_fixtures, scheduler_fixtures};
```

## Quickstart

Prerequisites: Rust **1.94.0**, Node **22+**, Python **3.11+**, and Bun. Dagger
and a container engine are required for the full ship-parity gate.

Install the repository-pinned toolchain, then build and run the exact Worker:

```sh
bun run worker:tools
bun run worker:build --out target/cloudflare/bundle
bun run worker:smoke --artifact target/cloudflare/bundle \
  --receipt target/cloudflare/workerd-proof.json
bun run dev:isolated --artifact target/cloudflare/bundle --port 8787
```

`worker:tools` installs `worker-build 0.8.5`, matching `wasm-bindgen 0.2.125`,
and the npm lockfile's `Wrangler 4.129.0`/`esbuild 0.28.1`. Builds target
`wasm32-unknown-unknown` with locked dependencies and no default features.
The artifact contains exact source/module hashes, revision, tools, configuration,
and the SQLite migration ledger. Artifact and receipt paths cannot be overwritten.

Local workerd uses a unique private SQLite/R2 directory and local mail outbox,
never a deployed binding or inherited provider credential. First start reads
the private actor's fingerprint and activates it through the authenticated
runtime API before observing readiness. Restarts read the persisted runtime
state without repeating activation. The smoke creates an allowlisted service
session, captures LocalOnly material, waits for actual alarm-driven generation,
explicitly keeps a draft, grades an answer, and checks persistence/idempotency
over restarts.
That is runtime evidence when executed, not an email or model-quality claim.
The signed-out browser accepts `dev@example.test` in this isolated environment.

```sh
git config core.hooksPath .githooks
bun run ci:local
bun run ci:full
```

The fast gate retains browser/native Rust/recovery contracts, formatting,
Clippy, rustdoc, and action-latency budgets, then builds and exercises Wasm.
Dagger adds live Postgres reference tests and Gitleaks and runs the same Worker
command. Neither gate deploys. Explicit verified staging-to-production version
promotion, fingerprint-guarded pause/activation, and schema-safe rollback live
in [the runbook](./docs/runbook.md). Only Main may stop old writers or retire
the old-host reverse proxy/redirect after live proof.

## License

MIT
