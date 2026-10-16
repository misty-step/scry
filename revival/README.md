# Revival-only archive custody

`preserve.mjs` is a small, temporary operator Worker for ADR 006. It protects a
pinned Go recovery archive from the source bucket's 30-day expiry before the
separate Go-to-Rust conversion. It is not an importer, SQLite checker, backup
scheduler, learner endpoint or production release engine.

## Exact contract and checks

Only authenticated `POST /preserve` with an empty body is accepted. Deployment
pins one `SOURCE_KEY`, `SOURCE_ETAG` and `SOURCE_BYTES` (positive, at most 16 MiB),
plus an absolute `PRESERVE_UNTIL_MS` capability expiry checked before bucket access.
The Worker reads that exact R2 object conditionally, hashes it, conditionally
creates `preserved-go/<sha256>/<source-key>` in a separate private bucket, then
reads and compares every destination byte before acknowledging success. Replay
and concurrent calls verify the existing object without overwriting it. Missing,
changed, oversized or conflicting objects fail closed. It never lists, deletes,
returns archive bytes, logs private content or fetches an external endpoint.

Run on the authorized build host with the repository's pinned Node and lockfile:

    node --test qa/revival-preserve.test.mjs

The existing `scripts/scry-ci` gate also runs this real-workerd/R2 suite. Fixtures
are explicitly opaque synthetic bytes: these tests do not certify Go ZIP or
SQLite integrity, canonical conversion, a final stopped-writer snapshot, learner
reconciliation or a production cutover. The normal Rust app and its bindings
are unchanged.

## Bounded operator procedure

Independent review precedes any learner-data binding. Use the exact module
from the verified CI source archive, not a local rebuild. Keep its source hash,
CI identity and source metadata receipt, never private object bodies.

1. Read only the source bucket listing to select the exact archive key, ETag
   and byte count. This does not establish that it is the final consistent
   archive; the one-writer cutover must make that determination separately.
2. Provision a new private custody R2 bucket in the same Cloudflare account.
   Read back that it differs from the source bucket, public access is disabled,
   and no object-deletion lifecycle applies. Do not change source retention.
3. Bind only `SOURCE` and `DESTINATION` to those two buckets and the three
   source metadata pins above. Set a dedicated `PRESERVE_KEY` secret and
   `PRESERVE_UNTIL_MS` no more than 15 minutes ahead on the operator's clock. No owner,
   synthetic-QA, model, production-writer or general Cloudflare API credentials
   belong in this Worker. Keep API/operator credentials on the approved chief
   path; do not send them or learner bytes to build hosts or CI.
4. Invoke through a short-lived Cloudflare-authenticated remote-preview session,
   additionally presenting the preservation key. Do not publish a custom domain,
   enable workers.dev/version URLs, reuse the synthetic QA principal, or add an
   Access app/policy without its own authorization. Verify anonymous and wrong-key
   refusal first with synthetic bindings. If that private invocation path cannot
   be proven, report the exact capability rather than expose a public endpoint.
5. Accept only a verified count/hash/key receipt. Independently invoke again and
   verify the same digest/key, plus destination object metadata and no-expiry
   lifecycle. Archive bytes and readback stay inside Cloudflare. Do not download
   them to inspect, attach them, or put them in logs/screenshots.
6. Observe an invocation of that same immutable preview token fail with
   `CAPABILITY_EXPIRED` after its actual deadline, and verify no published Worker
   or routes exist. Do not claim that replacing a preview upload revokes old
   tokens: a real synthetic drill proved the old token still runs its old module.
   Never log or retain these ephemeral keys/tokens; record only expiry and safe
   receipts. Preserve the custody bucket/object. An ambiguous invocation before
   expiry is safe to retry; an existing corrupt destination must never be
   overwritten or deleted as a fix. Stop if expiry/anonymous refusal cannot be
   demonstrated with synthetic bindings before learner binding.

This procedure is source-ready, not a claim that a learner archive has been
copied. A reviewed converter must still inspect the retained ZIP/SQLite inside
Cloudflare, preserve/reconcile every record and media byte, restore into unused
isolated bindings and retain post-cutover writes during rollback.

## Read-only conversion inventory

`inspect.mjs` is the next temporary operator tool, not a converter. It only reads
one pinned preserved archive and returns safe counts/digests. It never exports
records/media, writes R2/SQLite, claims a job, calls a provider or binds a learner
app. `POST /inspect` requires an empty body, a separate `INSPECT_KEY` (at least
32 characters), and absolute `INSPECT_UNTIL_MS` checked before storage access.
Pin `SOURCE_KEY`, `SOURCE_BYTES` and `SOURCE_SHA256` at deployment; bind only
`SOURCE` to the private custody bucket. No caller chooses SQL, a key or a record.

On the authorized build host, with the pinned lockfile dependencies:

    python3 scripts/build-revival-inspect.py
    node --test qa/revival-inspect.test.mjs

The build derives the five schema definitions from the checksum-verified retained
Go source, rather than maintaining a second schema. It uses the pinned official
SQLite WASM (including FTS5) and fflate ZIP reader; neither is added to the normal
Rust app. An earlier sql.js feasibility probe lacked FTS5 and is not used.
No custom SQLite reader, shell/database server or provider integration is needed.
The existing source gate builds once, tests in workerd/R2 against independent
native SQLite fixtures, and includes the exact `revival-inspect/` modules in its
normal artifact manifest and `proof.json`. Deploy those tested modules unchanged;
do not rebuild them operator-side. The unconfigured Wrangler template has no
storage bindings or public/preview URLs and is only used for the offline build.

Validation requires exactly the two uncompressed Go format-1 ZIP entries, exact
archive/database hashes and sizes, SCRY application identity/schema 5, full exact
reference schema, SQLite integrity and foreign-key checks. No migration or repair
is attempted. Every table (including historical/operation/media and FTS tables)
contributes every SQL value to a versioned, type-tagged canonical digest. Integer
values use exact int64 decimal, real values IEEE754 big-endian bytes, text exact
UTF-8 bytes including embedded NUL, blobs every byte, and NULL its own tag. Hash
each encoded row, sort those hashes (retaining duplicates), and hash the resulting
list. The native fixture oracle independently checks the same receipts. These
are inventory fingerprints, not evidence of Rust conversion or a final writer.

Use the preservation procedure's private remote-preview path, refusal checks,
memory-only keys and at-most-15-minute immutable capability lifetime. Verify the
same preview token refuses after its actual deadline while the source remains;
never treat replacement upload as revocation. There is no published domain or
synthetic learner principal. Before learner binding, independently review this
tool and its exact CI artifact, and prove the hosted path with synthetic Go-format
fixtures. Retain only its safe summary, hashes and source identity in evidence.
Any inspection failure returns a fixed error, never SQLite or archive diagnostics.

This slice does not implement Go-to-Rust mapping, semantic content reconciliation,
legacy writer quiescence, final import, private owner acceptance or live cutover.

## Retaining original Go records in Rust recovery

`App.revival_go` carries one original recovery ZIP through existing Rust durable
rows, owner export, complete backup and isolated restore. It is empty for an
ordinary fresh space. A temporary converter supplies 48 KiB binary chunks,
standard padded base64 values, and keys `<original-sha256>/<index:08>` starting
at zero. The final chunk alone may be shorter. Archive validation rejects gaps,
mixed hashes, changed bytes, invalid/noncanonical base64 and excessive size.
Each chunk uses an existing bounded record; there is no second storage service.

Nonempty custody selects archive identity
`scry-rust-durable-sqlite-go-custody-v1`. Do not relabel it with the ordinary
`scry-rust-durable-sqlite-v1` identity: older code would silently discard an
unknown application field. Old code rejects the new archive identity and durable
row prefix; only custody-aware tested builds are compatible rollback targets.
The normal schema, scheduler identity, authorization and 16 MiB complete-archive
bound remain unchanged. Retained bytes are never jobs or recall evidence.

The original can be recovered by ordered base64 decoding and concatenation,
then verifying its SHA-256 before the read-only inspector's ZIP/SQLite checks.
For learner data all of this still runs inside the authorized Cloudflare account;
never download it to perform that extraction. The separately retained no-expiry
R2 original remains untouched. Synthetic mechanics are exercised by
`cargo test --locked --test revival_custody` and the existing runtime suite.
These checks do not implement or certify the active Go-to-Rust projection,
canonical learner reconciliation, final consistent archive or production cutover.

## Preparing the active projection

`convert.mjs` reuses the same validated read-only SQLite/ZIP reader and pins.
Its only route is authenticated empty-body `POST /convert`, with separate
`CONVERT_KEY` and `CONVERT_UNTIL_MS`. Bind `SOURCE` to the preserved original
bucket and `DESTINATION` to a physically different private isolated bucket;
verify the actual provider binding names/IDs and no public access before use.
Use the same short-lived private remote-preview procedure and actual expiry
test as above. There is no published hostname, learner principal, model secret,
app binding, restore authority, job, scheduler, or release engine in this tool.

The existing build script now builds both temporary tools. The source gate
exports `revival-convert/` with hashes in `proof.json` and the normal complete
artifact manifest. Deploy those tested modules unchanged after independent
review; never bind an unreviewed converter to learner storage.

    python3 scripts/build-revival-inspect.py
    node --test qa/revival-convert.test.mjs qa/revival-inspect.test.mjs

`project.mjs` maps current goals, generated concepts/notes, every quiz/content
version, full pinned FSRS cards and original schedule numbers, graded history,
overrides, held feedback, receipt IDs/hashes, paid reservations and private
photos. Topic provenance becomes the candidate's `general` label; source quotes
must still validate against the saved source. Imported entity IDs begin with
`legacy-import-`; old operation IDs and payload hashes remain fenced against
reuse. Go-specific receipt kinds/result JSON and all other historical/source/
foundation/operational rows remain exactly recoverable in the original ZIP,
not executable compatibility responses or new learning evidence. Uncertain
historical jobs are paused, never resumed or sent. ADR 006 owns the mechanical
legacy-note, original-schedule-epoch and assisted zero-rating observation
decisions, including their compatibility/rollback limits. These observations
remain history, never invented scheduler writes or unaided recall.

Earlier exact-policy Go self-check attempts also remain immutable, visibly
ungraded history, including repeated submissions before a later grade on the
same presentation. They do not count as misses, helped completions or recall,
and cannot suppress due practice. A still-current saved self-check keeps its
answer and deliberate learner judgment without inventing an assessor call.
Current feedback is selected by its exact Go `review_id`, not the first event
on a presentation. Older Rust builds reject this history; retain only a proven
compatible build for rollback.

Both generator attempts and every transmitted `content_assessments` critic row
project into the existing Rust spending ledger under their original job, identity,
timestamp, request model and exact known cost or unknown reservation. Critic
retries remain separate charges, never a new send lease. Known zero remains zero;
Go's cleared settled reservation maps to Rust's nominal $0.50 reservation because
measured cost always takes precedence. Exact original fields/results remain in
the retained ZIP. Untransmitted zero-obligation critic rows stay historical only.
Unknown amounts other than $0.50, unsupported critic purposes/policies/ownership,
and old active/pending obligations that Rust's rolling window cannot represent
refuse preparation before destination writes; never reset a charge's clock or
silently release it. No spending engine or allowance policy is changed.

This deliberately bounded converter refuses unsupported active shapes rather
than inventing or silently dropping them: web documents, semantic-assessment
rows, corrections/reset rows, merged concepts, ambiguous primary concepts,
unsupported zero-rating outcomes, incompatible algorithms, unsafe integers or
submillisecond scheduler timestamps. Any such refusal must be investigated inside Cloudflare
and repaired with an independently reviewed transformation before cutover.
It does not reinterpret an unsupported dataset as a fresh start.

An unsupported projection returns HTTP 422 with `UNSUPPORTED_GO_PROJECTION`
and a source-defined `reason` code only. This identifies the refused
transformation without returning a record ID, SQL, private value, parser message
or stack. Archive/SQLite/reader and destination failures remain the generic
`INSPECTION_FAILED`; the read-only inspector's response contract is unchanged.
No destination write occurs for a projection refusal. Investigate the code's
named invariant and reproduce the shape with synthetic data before changing
that transformation; a refusal is never an import or cutover receipt.

The complete candidate is conditionally created at
`converted-go/<original-sha256>/<candidate-sha256>.json`, then read back byte for
byte before acknowledging. Replay/concurrency verify an existing object; a
conflict never overwrites it. Content-addressing permits a later reviewed
conversion to produce a different immutable candidate without erasing earlier
custody. Only safe counts, original inventory fingerprints, candidate section
hashes and object metadata leave Cloudflare. No record, archive or media body
is returned or logged. The original ZIP remains in every complete Rust archive.

Preparation explicitly returns `rust_restore_verified: false`: integrity and
exact remote bytes do not prove Rust semantic validation or live acceptance.
The synthetic suite exercises native SQLite -> real CF converter/R2 -> actual
Rust/WASM restore, exact content/card/history/media readback, accepted capture
replay, a complete independent backup and second unused-space restore. It also
proves malformed content fails Rust restoration with zero SQL/asset writes.
Native Go SQL independently checks the financial fixtures against converted,
restored and twice-recovered usage; an over-budget import blocks a new configured
preparation without a provider send. Known zero, cleared measured reservations,
unknown cost, critic retries, untransmitted work and old-window records are
separate controls, not inferred from original-byte custody.
These are local synthetic boundary proofs, not hosted learner reconciliation.
An operator must keep the prepared learner body inside Cloudflare when invoking
the existing isolated restore capability, compare runtime readback canonically,
and complete the remaining one-writer/private/live/rollback conditions before
cutover. Never download the prepared body to perform that operation.
