# Owner tokens, API and CLI

Behavior authority: [US-014 / AG1](../SPEC.md#us-014-drive-scry-with-an-owner-token).

Open Settings → Owner tokens. Enter a name, optionally an expiry in UTC, and explicitly allow paid work if needed. Create Owner token,
save the secret once, then return to Settings. The list shows metadata only.
Revoke a named token and verify its next request is refused.

Build the native client with `cargo build --locked --bin scry`. Pass the secret
through stdin, never an argument or URL. See [CLI/API usage](../docs/owner-api.md).
`qa/walk --stories "US-014"` exercises each criterion; the built Worker and
native CLI walks are shared with `node --test qa/owner-api.test.mjs`.

Source: `src/owner_token.rs`, `src/api.rs`, `src/engine.rs`, `src/runtime.rs`,
`src/web.rs`, `src/bin/scry.rs`. The [Library](library.md) uses this API boundary; Study actions follow in #317. Synthetic proofs do not establish deployed ingress.

US-019.7's spend slice (#321): use `scry spend --by day|purpose|model --json`
or `GET /api/v1/spend?by=day|purpose|model` with an Owner token. Run
`node --test qa/spend.test.mjs` against the built Worker for CLI/API values,
UTC grouping, historical attribution, private errors and byte-identical store
reads. The existing `qa/runtime.test.mjs` controlled provider lifecycle checks
reservation attribution and measured/unknown costs. See the
[projection/compatibility notes](../docs/spend.md) and
[builder evidence](../docs/qa/spend-321/README.md). This partial proof leaves
US-019 pending for #322; it adds no new story or acceptance gate.
