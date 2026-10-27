# Owner tokens, API and CLI

The owner, signed in through Access, mints a named Owner token in Settings. He
saves its secret once, logs the `scry` CLI in with it, and later revokes it.
Behavior authority: [US-014 / AG1](../SPEC.md#us-014-drive-scry-with-an-owner-token).

## Sub-features

- `token-create` shows the secret once and afterwards lists only name, creation, last use, expiry and paid permission.
- `token-login` saves a CLI login from stdin (`scry login --token-stdin`) after verifying identity.
- `token-act` lets an unexpired, unrevoked token act as the Owner for every API action.
- `token-refuse` refuses missing, malformed, expired and revoked tokens identically.
- `token-revoke` refuses the token's next request.
- `token-paid` refuses paid work for tokens without paid permission.
- `mcp-supplied` exposes supplied-content schema, validation/import and native practice through [stdio MCP](../docs/mcp.md), using the existing private CLI login; no generation or caller grade.
- `token-spend` reports spend through `scry spend --by day|purpose|model` and `GET /api/v1/spend` (US-019.7 slice, #321).

## How to get to it (user POV)

- Settings → Owner tokens (`/settings#owner-tokens`): Name, optional
  "Expires (UTC, optional)" and "Allow paid work", then Create Owner token.
  "Save your Owner token" shows the secret; Return to Owner tokens; Revoke.
- Terminal: `scry login --url ORIGIN --token-stdin`, `scry whoami` and
  `scry token create|list|revoke` (see the [CLI/API usage](../docs/owner-api.md)).

## Driving it with verify-scry helpers

Preconditions:

- Built Worker and CLI, as in [study.md](study.md#driving-it-with-verify-scry-helpers).

- **Mint, log in, revoke.** Run `.agents/skills/verify-scry/scripts/local-journeys.mjs`.
  `token.create` fills `#token-name` and chooses Create Owner token. It reads the
  "Owner token secret" textbox into memory only, masks it in the screenshot,
  and requires that the list shows the name but never the secret. `cli.login`
  pipes the secret to `scry login --token-stdin` and requires that `scry whoami`
  names the token. `token.revoke` chooses Revoke in Settings, and the next
  `scry whoami` must exit non-zero.
- **MCP wire contract.** `node --test qa/mcp.test.mjs` builds `scry-mcp` and drives its real stdio pipes against a local HTTP mock: discovery, bounded framing, native draft validation, exact route/payload forwarding, response loss/replay, credential/error privacy and optional cancellation. This is not built-Worker persistence or hosted ingress proof.
- **MCP native integration.** `node --test qa/mcp-worker.test.mjs` uses the actual built Worker/SQLite and compiled MCP: external NATO authoring, native raw-answer grading, immutable replay with transport CAS accounting, Reveal disclosure, and assistance fences. The sanitized receipt binds component hashes; no hosted or phone claim.
- **Hosted.** `.agents/skills/verify-scry/scripts/prod-states.mjs` →
  `cli.hosted-admission`: a Bearer request to `/api/v1/whoami` meets the Access
  login. **Hosted API admission (#335) is uncommissioned and UNPROVEN.**
- **Story walk.** `qa/walk --stories "US-014"` exercises each criterion.
  `node --test qa/owner-api.test.mjs`, `node --test qa/spend.test.mjs` and
  `tests/owner_api.rs` cover refusal parity, replay, paid refusal, history and
  export attribution.
- **Proof.** `token-shown-once-390.png` (secret masked), `token-list-390.png`
  and `cli-transcript.json` under the `<stamp>-local-journeys` evidence directory.

## Gotchas

- Never put a token in argv, a URL, a log or a screenshot. Pipe it on stdin and mask the textbox.
- Creation replay returns metadata without the secret. If the first response
  was lost, revoke that token and mint another with a new operation ID.
- A bearer never bypasses Access. Until #335 arranges route-specific admission,
  the CLI cannot reach https://scry.mistystep.io.
- US-019 stays pending for #322. The spend slice adds no story or gate.
  See the [spend notes](../docs/spend.md) and [builder evidence](../docs/qa/spend-321/README.md).
- Source: `src/owner_token.rs`, `src/api.rs`, `src/engine.rs`, `src/runtime.rs`,
  `src/web.rs`, `src/bin/scry.rs`.
