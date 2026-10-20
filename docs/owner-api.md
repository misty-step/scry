# Owner API and CLI

The one Owner signs into Settings through Access to mint the first named
Owner token. Save its secret on creation: lists, history, export and replay
cannot recover it. An optional expiry is a Unix timestamp in milliseconds.
Paid permission defaults to off. Any Owner token holder can mint, list and
revoke ordinary tokens. Granting paid permission requires the Access owner or
an Owner token that already has paid permission; an unpaid grant attempt is
refused before creating a token or operation receipt. Owner tokens do not
authorize operator backup or restore.

## Use the CLI

Build with the pinned Rust toolchain:

```sh
cargo build --locked --bin scry
./target/debug/scry login --url https://YOUR-CANONICAL-ORIGIN --token-stdin --json
./target/debug/scry whoami --json
./target/debug/scry token create 'build agent' --json
./target/debug/scry token list --json
./target/debug/scry token revoke TOKEN_ID --json
```

Pipe the token from a private credential source into login's stdin. Never put
it in an argument, URL, shell history, log or screenshot. Login verifies identity
before saving to `$XDG_CONFIG_HOME/scry/login.json` (or
`$HOME/.config/scry/login.json`) with mode 0600. `--config PATH` selects a separate
credential file. HTTPS is required except for isolated loopback HTTP. Redirects
are refused. A different origin requires a separate login.

All commands accept `--json`, output one JSON value, and exit nonzero on refusal
or failure. Creation accepts `--expires-ms TIMESTAMP` and `--paid`. Mutations
accept `--operation-id ID` (16–128 ASCII letters, digits, hyphens or underscores).
For a safely repeatable automated mutation, supply and retain that ID. On an
unknown response, reuse the same ID and payload. Creation replay returns the
committed result and metadata without the secret. If its first response was
lost, revoke that token and deliberately create another with a new ID.

## Call the API

All routes below are private, return JSON, and use `Cache-Control: no-store`.
Present `Authorization: Bearer <Owner token>` over HTTPS. Browser Access identity
is also admitted; cookie-authenticated writes require exact Origin and `csrf`.
Bearer admission takes precedence when an Authorization header is present,
including malformed credentials. Tokens are never accepted in query strings.

| Method | Route | Payload / result |
| --- | --- | --- |
| GET | `/api/v1/whoami` | Owner identity and acting token metadata |
| GET | `/api/v1/tokens` | `tokens` metadata list, including revocation |
| POST | `/api/v1/tokens` | `operation_id`, `name`, optional `expires_ms`, `paid` |
| POST | `/api/v1/tokens/revoke` | `operation_id`, `token_id` |
| GET | `/api/v1/history` | Operation receipts with `acting_token`, path and time |
| GET | `/api/v1/export` | Complete existing archive, including token verifiers; assistance gate retained |
| POST | `/api/v1/settings` | `operation_id`, `pace`; same engine as Settings |
| POST | `/api/v1/create` | `operation_id`, `intent`; existing preparation, requires paid permission |

Mutation results contain `result`, `replayed`, and token metadata for creation.
Only first creation includes `secret`. JSON fields are scalar strings, numbers
or booleans; internal fields are refused. Payload conflicts return 409. Invalid
tokens return the same 401 JSON refusal. Paid refusal is 403 before reservation.
Token-authenticated changes appear by token ID in receipts/history/export.

These are the #314 foundation routes. Library and Study actions belong to
#316/#317; they must adapt to this shared engine and durable boundary. This
change does not switch canonical origin, archive a live space, enable providers,
change Access deployment configuration, or activate an API in production.

## Hosted admission follow-up

A Scry bearer does **not** bypass the upstream Cloudflare Access policy. The
Worker can verify it only after Access admits the request; the CLI supplies
only that bearer and cannot satisfy an owner browser login policy by itself.

Before claiming hosted CLI/API access, the parent of [#314](https://github.com/misty-step/scry/issues/314),
[#313](https://github.com/misty-step/scry/issues/313), must arrange reviewed
route-specific Access admission/provisioning and retain hosted positive and
negative proof: an admitted Owner token reaches the canonical API and works,
missing/invalid/revoked tokens are refused, and browser routes retain their
owner Access protection. That policy work and its independent verification are
parent-owned follow-up. Local synthetic Worker checks do not establish hosted
admission; it remains unverified. This change does not alter live Access policy
or expose the application.
