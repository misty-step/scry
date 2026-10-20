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
`src/web.rs`, `src/bin/scry.rs`. Later Library and Study slices add their actions
to this API boundary. Synthetic proofs do not establish deployed ingress.
