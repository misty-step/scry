# ADR 006: One-way Go-to-Rust revival with Cloudflare-only custody

Status: accepted revival scope, 2026-10-15. Import implementation, learner-data
reconciliation and production cutover remain unverified. This decision is not
an import receipt or a declaration that the Rust replacement is live.

## Context

The original D6 fresh-start policy did not authorize importing historical
learning records. The owner decision recorded on the revival execution task
on 2026-10-15 supersedes that restriction for one revival: Rust plus a reviewed,
one-way import of the final Go dataset. It does not revive the historical API,
permit general uploads, merge unrelated stores, or accept the whole rewrite.

Production route metadata still identifies `scry-app-host`, the Go Container
predecessor, on `scry.study`, `www.scry.study` and `scry.mistystep.io`. The retained
Go source is `c6ba395a63814c1f973d8cda5057d54fbb587e9b`; source/deployment metadata
alone does not prove the running container image's bytes. The exe.dev VM,
October 9 Rust preview and archived Convex predecessor are separate custody
boundaries. Unknown Convex custody is not permission to merge or delete it.

## Decision

- Implement a reviewed, one-way conversion of a consistent final Go archive
  into the Rust candidate's validated archive. Keep the converter temporary and
  outside ordinary learner request paths; no runtime compatibility API follows.
- Learner bytes stay exclusively inside the existing Cloudflare account and
  isolated preview bindings. No learner archive, SQLite file, export, photo or
  plaintext record may travel to an operator machine, build host, CI, logs,
  screenshots, source or evidence attachment. The earlier offline-converter
  proposal is superseded. Test external evidence with synthetic fixtures.
- Preserve the source archive independently beyond the Go recovery bucket's
  30-day deletion rule. Validate its consistency and exact bytes before relying
  on it. Reconcile canonical records, content versions, full scheduler state,
  immutable attempts/corrections/receipts and every media byte across formats;
  comparing unrelated SQLite/JSON file hashes is not reconciliation.
  The [bounded preservation module](../../revival/README.md) only retains pinned
  opaque archive bytes inside Cloudflare and verifies complete remote readback;
  it does not perform conversion or grant consistency/cutover acceptance.
  The same procedure's read-only inventory verifies the retained archive with
  official SQLite WASM and returns per-table canonical counts/digests only.
  Its reference schema is derived from the exact retained Go source. It remains
  outside learner routes and is not itself the one-way conversion or live proof.
- A revived Rust space retains the original Go recovery ZIP verbatim in
  `App.revival_go`. The Go store also contains historical foundation, source
  revision and operational records that have no exact active Rust counterpart;
  a projection must not silently discard them. Retention is byte custody, not
  a substitute for mapping and reconciling learner-visible content, schedules,
  history and receipts. The temporary converter must inspect the original and
  prepare the active projection separately before any learner binding.
  Custody uses SHA-256-addressed, ordered base64 chunks in existing record
  storage and the existing complete backup/export/restore paths, with no new
  endpoint, dependency, scheduler or historical-code execution. It consumes
  the existing 16 MiB total capacity. Ordinary archives remain compatible;
  custody archives use a distinct identity so older builds reject restoration
  rather than deserialize away unknown fields. Old builds likewise reject
  the unfamiliar durable rows. They are not rollback targets after import;
  the cutover must retain a tested custody-aware compatible rollback artifact.
- The subsequent owner decision keeps all 37 retained quizzes active. A quiz
  without a current non-foundation assessing concept receives its own explicitly
  labeled `legacy-import` concept and reference note, mechanically assembled
  from the exact existing prompt, answer and explanation. No model call, invented
  introduction, mastery observation or review event is permitted. Existing
  generated concepts/notes retain their content; old foundation structures stay
  in original-byte custody. Imported identities and mechanically created notes
  identify their provenance so the projection can be found and redone later.
  Report actual quiz/question/new legacy concept/new legacy note counts without
  learner content. Source support or synthetic counts are not learner receipts.
- Go's schedule epoch begins at one, Rust's ordinary epoch at zero. Preserve
  original schedule numbers with `App.revival_schedule_origins`, restricted to
  epoch one for existing `legacy-import-quiz-` records with retained Go bytes.
  Every real transition and the current full card still require the same complete
  chain and pinned scheduler checks. Nonempty origins select the distinct
  `scry-rust-durable-sqlite-go-projection-v1` archive identity; the separate row
  prefix makes older custody-only builds refuse the state rather than drop this
  interpretation. Those older builds are not serving/rollback targets.
- Go's assisted zero-rating `warm_correct`/`warm_wrong` observations do not write
  a schedule or increment its epoch. Preserve their original epoch, immutable
  answer/assistance/authority and unchanged full card rather than renumbering
  them as transitions. Only `legacy-import-event-`/`legacy-import-presentation-`
  identities on retained Go question epochs have this interpretation. Validate
  each against the real card at that epoch, separately from the complete chain
  of scheduler writes. Go's zero `due_at` sentinel means no new deadline; Rust's
  held result names the unchanged card's real due date. The original sentinel
  stays byte-exact in custody. Ordinary/new Rust warm events still increment
  their concurrency epoch. Earlier projection builds reject these archives and
  cannot be rollback targets after this import.
- Earlier Go revisions also durably recorded exact-policy `selfcheck` (and
  exposed `warm_selfcheck`) attempts with rating zero and no scheduler write,
  sometimes before a later graded attempt on the same presentation. Retain
  every original event/answer/assistance/epoch, labeled `ungraded`, not a miss,
  helped completion, recall success or invented assessor transmission. Shared
  presentation identity must keep identical content and allow at most one
  completed grade. A still-current held answer remains actionable self-check;
  choosing a grade later creates an ordinary new learner event. Earlier Rust
  builds reject this history and are not compatible rollback targets. Verbatim
  original outcomes and grading policy remain in the retained Go archive.
- Restore only into an unused isolated namespace, with separate private R2 and
  no paid integrations. Review all expected transformations explicitly. The
  runtime initializes a new namespace concurrency revision at one, creates a
  fresh CSRF token and photo keys, clears inherited backup receipts and pauses
  uncertain work. Content/schedule/history versions do not reset with that epoch.
- Diagnose the whole preserved dataset before another import, inside the same
  private Cloudflare custody with no destination capability. Collection and
  conversion use the same projection guards; a complete diagnostic releases
  only fixed refusal classes and counts, and writes nothing. An unexpected
  dependent parsing/shape failure invalidates the report rather than publishing
  a partial pass. Only the actual complete report triggers further shape-specific
  repair, batched into one independently reviewed change before real import.
- Project generator and transmitted critic usage into the existing Rust spend
  records, preserving owning job, stable per-attempt identity, original timestamp,
  model and exact known cost or unknown $0.50 reservation. All critic retries
  count independently, without reconstructing executable assessment leases.
  Rust's nominal reservation remains $0.50 after known settlement; Go's cleared
  reservation is therefore normalized only when measured cost (including zero)
  takes precedence. Keep the exact original in custody. Unsent zero-obligation
  critic rows remain historical; unsupported purpose/ownership/unknown amounts
  and old active/pending obligations Rust cannot express fail before candidate
  writes. No new spending engine or silently restarted allowance window follows.
- Synthetic service QA remains confined to ADR 005's synthetic bindings. Never
  apply that mode or principal to a namespace containing imported learner data.
  Any approved existing-owner readback is read-only; do not obtain or impersonate
  the owner's PIN or copy their browser session as an agent workaround.
- Keep one authoritative learner writer. Complete synthetic/deployed recovery,
  canonical learner reconciliation and independent pre-cutover review before
  the final consistent import and routing change. Routine rollout needs no
  additional owner approval after those objective gates; changed private-data,
  credential, money or irreversible-action scope still needs authorization.
- Preserve accepted post-cutover Rust writes during rollback. A compatible
  retained Rust version on the same namespace is the normal rollback target.
  Repointing to the old Go snapshot would lose accepted Rust writes and is not
  an authorized rollback. Retain the Go host/data cold and recoverable; VM
  deletion requires its separate ownership decision. Delete only authorized
  learner preview data after successful cutover.

## Executed synthetic checkpoint, not learner or live acceptance

On 2026-10-16 the reviewed active converter ran inside Cloudflare against the
retained learner archive and refused it before a candidate write. Its generic
failure could not distinguish an unsupported transformation from archive or
storage failure. Temporary conversion now identifies projection refusals with
source-defined invariant codes, never exception text or private values.
This changes diagnostics only, not mapping, custody, supported shapes, or the
conditions for cutover. The normal app and read-only inventory remain unchanged.

On 2026-10-15 the independently reviewed `41c647ac5ef71559bce2912d229d794db0822ef1`
master artifact was downloaded from CI run `38788688431`, artifact `12000250186`.
The ZIP digest is
`5e39bd638dbd5cfe64553b71264c7080ed3f49d57512871c81051ea9976a9c98`.
All 64 manifest members (including hidden files), 371 source entries and 51
story-artifact hashes verified. The exact downloaded WASM passed 24 fresh
workerd tests and all three deployed runtime module hashes matched readback.

The isolated `scry-synth-20261015.mistystep.io` preview admitted the QA service
principal and refused anonymous or forged requests, with CSRF rejection, exact capture replay/conflict,
draft preservation, optional private photos, held/assisted feedback, navigation,
light/dark/320px screenshots and native no-JavaScript capture. Six browser
journeys and 23 HTTP observations passed; these do not constitute every SPEC
criterion, upstream owner logout, useful live generation or physical-phone proof.

A deliberate complete R2 snapshot (26,509 bytes, SHA-256
`fafdafe159be93ce7edc7b2b230ad3102cb47f6da046108c2e01ea8704d71452`)
was independently retrieved and restored into a separate hosted Worker/namespace.
Eleven goals, two immutable events, fifteen operation receipts and two photos
reconciled after the explicit restore/reference-read changes. Recovery HTTPS/UI
was exercised and the restore capability removed. The synthetic hostname was
returned to its original namespace. A real provider version rollback then
retained a newly accepted synthetic capture exactly once and preserved all
canonical application records and photos. This proves a same-schema Rust
rollback, not a reverse Rust-to-Go migration.

Failures remain part of the execution evidence: an initial browser assertion
incorrectly excluded the legitimate preparing state; domain replacement first
returned 409 until explicit origin override; new binding/route propagation
briefly returned fail-closed 403; initial reconciliation incorrectly assumed a
cross-namespace monotonic concurrency revision. No storage was erased or
recreated to clear those failures. Natural remote cron success is still a
separate unverified requirement; deliberate backup success is not cron proof.

The native execution task retains exact operation receipts, scripts, hashes,
real synthetic screenshots and remaining gates. No learner records, production
routes, production writer, historical stores or model capabilities were touched
for this checkpoint. The later version/changelog/release lane remains separate;
this migration introduces no competing release workflow.
