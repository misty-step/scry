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
