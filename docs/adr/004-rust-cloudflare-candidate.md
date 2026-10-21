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
bound. A signed identity token whose `email` exactly equals the plain var
`QA_EMAIL` maps to the same QA principal (2026-10-21), unless its `sub` is the
owner's. The owner keeps the configured `SPACE_NAME`. The object ID determines
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
2026-10-15, daily delivery moved outside Cloudflare. The repo-owned Mirrodin
systemd user timer `scry-daily-backup.timer` replaces the archived Hermes job
at 09:30 UTC; [the installation procedure](../rust-cloudflare.md#mirrodin-triggered-daily-snapshot)
owns its post-merge handoff and verification. A separately scoped secret admits only the
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

The 2026-10-20 exact-recall decision adds a versioned `/api/v1` JSON API on
this Worker and a thin native Rust `scry` HTTP client sharing wire types.
Access remains browser identity; named Owner tokens admit programs to the same
owner space. The secret is returned only on first creation and only a SHA-256
verifier is stored, in an independently addressed row so refusal precedes
learner-state reads. Optional expiry, revocation, last use and paid permission
(default off) belong to the token. There is no account or public signup model.

Both transports use the same engine receipts, payload conflicts, operation
fences and revision-fenced SQLite commit. Access writes retain Origin and CSRF;
bearer writes skip those cookie-specific checks. Receipts retain the acting
token for history/export, and queued paid work retains it for authorization
before reservation and again before transmission. Old records deserialize with
empty tokens and absent attribution. Complete private archives retain verifier
and revocation metadata, never token secrets. Browser-owner backup/restore
capabilities remain separate. Library and Study route adapters follow in their
own slices; the historical QA object is not removed by this foundation.

No legacy API/MCP parity or old Go schema upgrade is inherited. Current route
and configuration details are inspectable in `src/runtime.rs`; operator endpoints
are documented in the candidate procedure. Browser-owner identity is not restore
or backup authority. A passing local Worker test is not deployed ingress or
independent remote-recovery proof.

The historical Go/SQLite/HTMX and private-hosting decisions remain in ADRs 001–003,
[Container hosting notes](../../deploy/cloudflare-hosting/README.md) and the
runbook. [R2 consistency](https://developers.cloudflare.com/r2/reference/consistency/)
and [S3 compatibility](https://developers.cloudflare.com/r2/api/s3/api/) describe
storage mechanics, not an assumption of object lock or independent replication.

## 2026-10-20 Library storage amendment (#316)

`src/library.rs` owns private content and its deterministic transitions. Its
interface is `search`, `show_work`, `show_version`, `add`, `edit`, `delete`, and
`validate`. `Work` holds title, aliases, Kind, context, and a revision.
`Version` holds immutable text, name, Kind, Supplied Provenance, Rights status,
citations, normalized hash, and ordered Sections containing ordered Units.
`Relation` represents version-of, supersedes, part-of, and related-to. This
slice creates version-of and supersedes; list and mapping slices own their
Work relations. No relations editor is added.

The existing singleton and SQLite transaction remain the sole writer. Content
uses separate `library_works/<id>`, `library_versions/<id>`, and
`library_relations/<id>` records. Each Version includes its Sections and Units,
with stable identities and array order. `App.library` groups only these records;
SQLite metadata carries no embedded Library. `study_version_refs/<study-id>`
records a Version reference outside the Library. The practice slice must write
that reference atomically with Study creation and retain it for dropped Studies
and their Attempts. #316 tests the populated reference contract without adding
a Study lifecycle or claiming real Attempt proof.

`engine::mutate_as` still clones, applies, attributes the acting Owner token,
records the operation receipt, and commits through the SQLite revision fence.
Work edit, delete, and Version supersede take the current Work revision. Add
resolves normalized titles and aliases before creating a Work. Text equality
uses SHA-256 of the shared #315 Unicode-normalized words with spacing and
apostrophes removed. There is no fuzzy comparison or second Unicode algorithm.
Duplicates return the existing Version without changing Library records.
A supersession to text that already exists conflicts; the caller can show that
Version. A correction never edits the old Version. Free Library operations
create no provider job, assessment, reservation, or spend.

Default segmentation groups lines into Sections and splits clauses while
retaining delimiters and whitespace. Callers can supply named Sections and
exact Unit strings. Their concatenation must equal the submitted UTF-8 bytes,
including CRLF and final whitespace, before any record is changed. Text is
bounded at 256 KiB, with the existing row and complete archive limits also
applying. Sources are optional caller-supplied citations; they are not fetched
or certified. Rights default to private-only. Lists and mappings retain their
Kind here; their practice rules remain in their own slices.

The Library export serializes only `Library`, excluding study references,
credentials, audit receipts, and the older notebook. Complete recovery archives
include all of them and use `scry-rust-durable-sqlite-library-v1` while Library
records, Study references, or Library operation receipts exist. Older builds must refuse this format instead
of silently discarding content. Empty Library and empty reference state are
omitted, so pre-Library archive bytes and SQLite rows round-trip unchanged.
Deletion retains operation receipts even when their content no longer exists.
A pre-Library build cannot read populated Library rows; rollback therefore
requires a compatible build, not removal of acknowledged content. The Owner
token foundation's verifier/revocation rollback limitations still apply.

This is a source/storage decision, not activation. No historical store is
changed, and no account, sharing, signup, or public read path is introduced.

## 2026-10-20 text Study integration (#317)

`src/study.rs` owns Study transitions and Prompt policy. Typed records distinguish
Study status and Pace, one Unit/direction Card with its Cue and pinned FSRS
state, a frozen presentation, immutable Attempts and per-Unit Corrections.
[The implementation note](../implementation/exact-study-loop.md) records the
access paths, concurrency analysis, selection order and reversible timing choices.
The CLI and JSON routes adapt to `engine::mutate_as`; they add no writer,
provider job, reservation or schedule implementation.

The App's optional `practice` field is omitted when empty. Populated maps use
`practice_studies`, `practice_cards`, `practice_attempts` and
`practice_corrections` SQLite rows. Frozen current presentation remains in
metadata. Attempt Grade and effects use the existing bounded payload-chunk
codec under `practice_payload`. Study creation writes the Library reference in
the same commit. Drop retains that reference and every Attempt.

The shared operation result can contain optional response data. Practice
receipts freeze safe Prompt responses or immutable Attempt/Correction identities.
The runtime resolves those identities after the revision-fenced save, so no
answer or reveal precedes its commit. Existing receipts omit the new field.
Due, history, export and Study list share spend's narrow SQL snapshot read.
They do not initialize application state, save last-use, recover work or arm
alarms. Framework allocation on cold activation is a separate parent behavior.

Populated practice or practice receipts require
`scry-rust-durable-sqlite-study-v1` archives. Compatible archive validation checks
Version/Unit references, deterministic Grade, per-Card effects, Corrections and
current projections. Empty pre-Study rows and archives retain their old bytes.
An older writer cannot safely handle populated practice. Rollback requires a
compatible build that retains these records, not deletion or an older binary.
Local SQLite/R2 restart and fresh restore proof do not authorize activation or
establish independent hosted recovery.
