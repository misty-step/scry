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
