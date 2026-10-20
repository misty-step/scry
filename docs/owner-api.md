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

The foundation routes share the engine and durable boundary with the Library
routes below. Study actions belong to #317. Source support does not establish
production API activation or Access admission.

## Hosted admission follow-up

A Scry bearer does **not** bypass the upstream Cloudflare Access policy. The
Worker can verify it only after Access admits the request; the CLI supplies
only that bearer and cannot satisfy an owner browser login policy by itself.

Before claiming hosted CLI/API access, the parent of [#314](https://github.com/misty-step/scry/issues/314),
[#313](https://github.com/misty-step/scry/issues/313), must arrange reviewed
route-specific Access admission/provisioning and retain hosted positive and
negative proof. Confirm that an admitted Owner token can use the canonical API.
Requests without a token or with an invalid or revoked token must be refused.
Browser routes must retain owner Access protection. That policy work and its independent verification are
parent-owned follow-up. Local synthetic Worker checks do not establish hosted
admission; it remains unverified. This change does not alter live Access policy
or expose the application.

## Private Library

Search before adding supplied text:

```sh
./target/debug/scry search 'apostles creed' --json
./target/debug/scry version add --title "Apostles' Creed" --kind text \
  --text-file /private/supplied.txt --name 'English, Roman Missal 2011' \
  --alias 'Symbolum Apostolorum' --source 'Caller-supplied citation' \
  --operation-id retained-library-operation --json
./target/debug/scry version show VERSION_ID --json
./target/debug/scry work show WORK_ID --json
./target/debug/scry work edit WORK_ID --revision 1 \
  --alias 'Symbolum Apostolorum' --context 'Private context note' --json
./target/debug/scry version supersede VERSION_ID --revision 2 \
  --title "Apostles' Creed" --kind text --text-file /private/corrected.txt --json
./target/debug/scry export --library --json
./target/debug/scry version delete UNUSED_VERSION_ID --revision 3 --json
./target/debug/scry work delete UNUSED_WORK_ID --revision 4 --json
```

Use the current `work.revision` from show for edit, delete, and supersede.
The numbers above illustrate successive revisions, not values to reuse blindly.
`--alias` and `--source` repeat. Work edit replaces aliases when supplied;
`--clear-aliases` removes them. Omitted title, aliases, and context stay unchanged.
Kind belongs to the Work and is fixed; Version text and metadata are immutable.
Supersede takes the same draft flags as add, with an existing Version ID and
its Work revision. A correction to wording that already exists conflicts.

Version flags include `--rights private_only|public_domain|licensed`,
`--spelling-strict`, and `--sections-file PATH`. Rights default to private-only
and Provenance is always supplied. Sources are optional and never fetched.
`--kind` accepts `text`, `ordered_list`, and `keyed_mapping`; this slice stores
them and adds no practice behavior. Text files must be UTF-8, at most 256 KiB.
Each Version also allows at most 4,096 Sections and 8,192 Units in total,
including caller-supplied Sections. Default Sections follow lines, with clauses
as Units, so text below the byte limit can still exceed either count limit.
To name articles or stanzas,
pass a JSON file such as:

```json
[{"heading":"Synthetic article","units":["First supplied clause,"," second clause.\n"]}]
```

Every byte must match the text file when Units are joined, including line endings
and final whitespace. A mismatch refuses the whole add. The API returns HTTP 422
with `Version Units must join to the submitted text exactly.` for a join mismatch,
`A Version supports at most 4096 Sections.` for excess Sections, or
`A Version supports at most 8192 Units.` for excess Units. These refusals store
nothing. Supplied Sections must satisfy the same limits and exact byte join;
they do not bypass capacity checks. The CLI reports these refusals as
`API request failed.` with a nonzero exit. Case, punctuation,
and spacing differences return the existing Version, preserving its original
bytes and metadata. Titles and aliases resolve one Work; ambiguous names conflict.
Search matches titles, aliases, and the first 240 characters of each Version.
An empty query lists the Library. No matches returns
`{"matches":[],"empty":true,"message":"Nothing matches."}`.

| Method | Route | Payload / result |
| --- | --- | --- |
| GET | `/api/v1/library/search?q=QUERY` | Matching Works with Versions and Relations, explicit empty result |
| GET | `/api/v1/library/works/ID` | Work metadata, Versions with Sections and Units, Relations |
| GET | `/api/v1/library/versions/ID` | Version, owning Work, incoming and outgoing Relations |
| GET | `/api/v1/library/export` | Only `works`, `versions`, and `relations` |
| POST | `/api/v1/library/versions` | `operation_id`, `data` Version draft |
| POST | `/api/v1/library/versions/supersede` | `operation_id`, `version_id`, Work `revision`, `data` Version draft |
| POST | `/api/v1/library/versions/delete` | `operation_id`, `version_id`, Work `revision` |
| POST | `/api/v1/library/works/edit` | `operation_id`, `data` with `work_id`, `revision`, optional `title`, `aliases`, `context` |
| POST | `/api/v1/library/works/delete` | `operation_id`, `work_id`, `revision` |

The Version draft is an object with `title`, `kind`, `text`, optional `name`,
`aliases`, `context`, `sources`, `rights`, `spelling_strict`, and `sections`.
Each Section has `heading` and an ordered `units` array of exact strings.
Only Library `data` supports nested JSON; other foundation API fields remain
scalar. Mutations return the shared `result.entity_id`, `redirect`, and
`replayed` fields. Show the returned identity to read the committed content.
Every Library mutation retains token attribution in private history and complete
export. The Library-only export excludes that attribution and all study state.

Unused Versions and Works can be deleted; a recorded Study reference refuses
both. #317 must create and retain references with real Studies and Attempts.
Until that slice lands, used-Study refusal has native unit proof only. Library
CRUD needs no paid permission, makes no provider call, and reserves no allowance.
The hosted Access admission limitation above also applies to these routes.
