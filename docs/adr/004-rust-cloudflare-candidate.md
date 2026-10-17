# ADR 004: Rust Cloudflare candidate and one durable authority

Status: accepted implementation decision, authorized 2026-10-07; extracted from
SPEC during authority consolidation. This records candidate source, not serving
state, live migration approval, or product acceptance. [SPEC](../../SPEC.md)
alone owns behavior and policy; [the runbook](../runbook.md) owns operational
records; [the candidate procedure](../rust-cloudflare.md) owns build/release and
restore commands. ADRs 001–003 describe the retained Go implementation, not this
fresh replacement or the earlier retired Rust stack.

## Decision and rationale

Use one plain Rust/WASM Cloudflare Worker, one named SQLite-backed
`LearningSpace` Durable Object, and private R2. One singleton owns learning
writes and background transmission ownership; R2 holds photos and complete
recovery archives, not a second learning writer. This is a ground-up application,
not a restored Container, retired multi-app engine, or historical-data import.
Worker-build compiles the WASM and loader. No Go application dependency, HTMX,
frontend framework/build, vector database, event bus, or generic agent layer is
required. The fresh namespace preserves historical stores separately.

```text
private browser: escaped HTML + self-hosted assets + vanilla controller
  → Access → Rust/WASM Worker → internal capability → LearningSpace SQLite
                                                       ↘ bounded model/Jev HTTPS
                                                       ↘ private R2 archives/photos
```

## Module ownership

- `src/learning.rs`: pure grading, scheduler and concept evidence; no persistence,
  HTTP, authentication, UI, logging or model-client dependency. Preserve
  `learning::SCHEDULER` / `ALGORITHM` and the full pinned Go FSRS adapter state.
  `tests/fsrs_golden.rs` checks 260 complete reference trajectories. This is
  bounded scheduler compatibility, not retired-engine parity or efficacy.
- `src/model.rs` / `src/engine.rs`: typed records and domain transitions;
  `src/persistence.rs`: validated row and versioned JSON/photo archive encoding.
- `src/generation.rs`: bounded serialized requests, material/evidence validation
  and independent critic results. Model/user input is data, never instructions
  that gain runtime authority.
- `src/runtime.rs`: Access, canonical Host/origin/CSRF, SQLite durability,
  external effects, alarms, R2 assets, exact remote readback and fresh restore.
- `src/web.rs` / `assets/`: escaped HTML, self-hosted fonts/styles and presentation.
  The external vanilla controller enhances ordinary same-origin POST forms,
  reconciles server-rendered HTML and owns no durable grade/schedule/publication.
  No inline executable source, remote fonts or localStorage history is needed.
  Browser/retained recovery tools deliberately use plain JS; `.dagger/` is the
  TypeScript gate surface.

## Transactions and external effects

A revision-fenced synchronous SQLite transaction validates the operation,
resolves a local choice/exact grade or saves one pending assessment, and commits
all domain records before returning success. Form operation identities derive
from a fresh cryptographic render nonce plus occurrence/revision/action/form.
An identical committed operation returns its saved result; changed payload or
stale occurrence/content/schedule conflicts. External calls never enter SQL.

Before awaiting external work, save transmission ownership and allowance. Load
fresh state afterward and recheck lease, source/content revision, occurrence,
schedule version and lifecycle before finalization. Events retain `exact-v1`,
`short-v1`, `semantic-v1` or `learner-v1` policy attribution. Behavioral thresholds,
retry rules and durable-history obligations remain only in SPEC.

## Ingress and recovery mechanism

Verify the Access RS256 JWT signature against issuer keys, exact issuer,
audience, time claims and immutable owner subject; email policy alone is not
authentication. A separate internal capability authenticates singleton forwarding.
Same-origin/CSRF/operation fences protect writes; private responses use no-store
and strict external-script/style policy. Loopback development identity refuses
live provider/operator capabilities.

The 2026-10-16 QA authorization adds one fixed `scry-qa` object to ordinary
preview/production, not a multi-user signup system. An exact signed service
`common_name`, with empty `sub`, maps to QA only when its client ID is explicitly
bound. The owner keeps the configured `SPACE_NAME`. The object ID determines
internal principal and R2 prefixes, including after eviction and alarm delivery;
request headers never select a notebook. Owner internal forwarding cannot access
the QA object, nor vice versa. QA cannot invoke operator or owner-backup routes;
ordinary Settings backup and alarms act only on its own notebook. There is no
schema change, import or new integration. Missing QA binding leaves the path
inert. Provisioning and hosted ingress proof are separate from this source change.

QA does not inherit provider capabilities. Its separate notebook cannot safely
reserve against the owner's application ledger, so object-derived capability
checks disable all QA paid work before reservation and again at transmission.
This preserves the existing owner spending boundary without a distributed ledger
or a second allowance. Enabling QA providers requires a separately approved
allocation and implementation; Access admission alone never authorizes spend.

Complete versioned JSON archives include referenced photo bytes and metadata in
private R2. After two account-level Cron Trigger drills went unobserved on
2026-10-15, the existing chief no-agent minute job is the daily trigger instead;
no new job or scheduler is added. A separately scoped secret admits only the
canonical `POST /internal/backup`, behind the existing Access perimeter, without
granting browser-owner identity. Cron Triggers are disabled in the template;
Durable Object alarms remain a backup/work recovery path, not daily delivery
proof. All triggers and pre-release/operator backup share exact remote
size/checksum/full-byte readback and archive validation. Health reads the last
verified completion without starting work; SPEC owns its freshness threshold.
Restore uses an unused
isolated Durable Object namespace and separate scoped restore capability, with
compatible Worker/assets/configuration retained independently. Behavioral
capacity, retention, pause, spend and recovery objectives are specified in SPEC,
not independently configured by this record.

## Consequences and references

No legacy API/CLI/MCP parity or old Go schema upgrade is inherited. Current route
and configuration details are inspectable in `src/runtime.rs`; operator endpoints
are documented in the candidate procedure. Browser-owner identity is not restore
or backup authority. A passing local Worker test is not deployed ingress or
independent remote-recovery proof.

The historical Go/SQLite/HTMX and private-hosting decisions remain in ADRs 001–003,
[Container hosting notes](../../deploy/cloudflare-hosting/README.md) and the
runbook. [R2 consistency](https://developers.cloudflare.com/r2/reference/consistency/)
and [S3 compatibility](https://developers.cloudflare.com/r2/api/s3/api/) describe
storage mechanics, not an assumption of object lock or independent replication.
