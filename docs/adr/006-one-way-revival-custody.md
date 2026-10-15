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
- Restore only into an unused isolated namespace, with separate private R2 and
  no paid integrations. Review all expected transformations explicitly. The
  runtime initializes a new namespace concurrency revision at one, creates a
  fresh CSRF token and photo keys, clears inherited backup receipts and pauses
  uncertain work. Content/schedule/history versions do not reset with that epoch.
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

On 2026-10-15 the independently reviewed `41c647ac5ef71559bce2912d229d794db0822ef1`
master artifact was downloaded from CI run `38788688431`, artifact `12000250186`.
The ZIP digest is
`5e39bd638dbd5cfe64553b71264c7080ed3f49d57512871c81051ea9976a9c98`.
All 64 manifest members (including hidden files), 371 source entries and 51
story-artifact hashes verified. The exact downloaded WASM passed 24 fresh
workerd tests and all three deployed runtime module hashes matched readback.

The isolated `scry-synth-20261015.mistystep.io` preview proved actual service
access, anonymous/forged denial, CSRF rejection, exact capture replay/conflict,
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
