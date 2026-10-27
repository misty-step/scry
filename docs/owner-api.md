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
unknown response, reconcile with the same ID and payload before advancing the
practice Prompt. A newer unanswered Free recall Prompt may refuse old
answer-bearing replay; see the practice replay boundary below. Creation replay returns the
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
routes below. Text Study actions are documented below. Source support does not establish
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
`--kind` accepts `text`, `ordered_list`, and `keyed_mapping`. Ordered-list
practice remains unavailable; supplied forward mappings are described below. Text files must be UTF-8, at most 256 KiB.
Each Version also allows at most 4,096 Sections and 8,192 Units in total,
including caller-supplied Sections. Default Sections follow lines, with clauses
as Units, so text below the byte limit can still exceed either count limit.
Recitation has a separate admission bound: its exact answer must fit the
1,000,000-cell local grader, `(expected normalized words + 1) × (answer
normalized words + 1)`, also for each matching sibling Version scope. Without
longer siblings, that admits up to 999 normalized words, not 1,000. Section
siblings match by Section position; whole Recitations compare whole Versions.
`next --recite whole|section` refuses before replacing the current Prompt with
JSON HTTP 422 naming the grading bound; the CLI exits nonzero. Refusal preserves
the held Prompt, Cards, history and operation receipts. No smaller scope is
selected automatically. Answers longer than the admitted exact text remain
subject to the same grading bound at submission. This grading check does not
establish that a large Prompt fits the separate serialized-storage limit;
storage admission remains unresolved in [#347](https://github.com/misty-step/scry/issues/347).

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
both, including active, paused and dropped Studies.
Until that slice lands, used-Study refusal has native unit proof only. Library
CRUD needs no paid permission, makes no provider call, and reserves no allowance.
The hosted Access admission limitation above also applies to these routes.

## Supplied keyed mappings

External agents author the content; Scry validates and stores it without any
content generation, provider call, spend, or fabricated learning observation.
Import the included 26-pair NATO spelling alphabet, inspect it, then explicitly
start its Study:

```sh
./target/debug/scry mapping add --file examples/nato-alphabet.json \
  --operation-id retained-nato-import-operation --json
./target/debug/scry version show VERSION_ID --json
./target/debug/scry learn VERSION_ID --pace steady --json
```

`POST /api/v1/library/mappings` takes `operation_id` and a `data` manifest:

```json
{
  "title": "NATO spelling alphabet",
  "direction": "forward",
  "spelling_strict": true,
  "pairs": [{"key": "A", "value": "Alfa"}, {"key": "J", "value": "Juliett"}]
}
```

This short example is not the complete alphabet; `examples/nato-alphabet.json`
contains all 26 official words and source links. Manifest optional fields are
`name`, `aliases`, `context`, `sources`, `rights`, and `spelling_strict` (defaults
to true). Rights default to private-only. Sources are retained, not fetched.
The only accepted direction is the explicitly declared `forward`. Unknown
fields (including generation, grades and media) are refused rather than silently
ignored. The CLI reads at most 1 MiB of manifest JSON; the stored Version retains
the existing 256 KiB text and 8,192 Unit bounds.

**Bundled starter, explicit action only.** Library offers **Add NATO alphabet**,
then the existing **Learn** form. The button posts to the same mapping mutation
using `starter=nato-v1`; its HTML does not contain the canonical words. No GET
imports content and no token setup is required for an already authenticated
browser owner. Agents may select the same supplied content without another API
route:

```json
{"operation_id":"retained-nato-starter-operation","starter":"nato-v1"}
```

Send that object to `POST /api/v1/library/mappings`, instead of `data`. Exactly
one of `starter` or `data` is required; other selectors and client overrides are
refused. `nato-v1` pins the included manifest; changing its content requires a
new selector. Repeated Add and ordinary manifest imports reuse the same Version
and preserve Study/Card history. The browser's `revision` is transport metadata,
not permission to replace content or supply a grade. CLI users can continue to
import the same file shown above; no new alias or generation command is needed.

Keys and values must be nonempty, without surrounding whitespace, control
characters, tabs or line breaks. Keys are unique under the pinned mapping Unicode/punctuation-boundary
normalization; case and compatibility-equivalent keys
cannot create duplicate Cards. Keys are at most 240 bytes, values 4,096 bytes.
Pair order is preserved. The adapter creates an ordinary immutable keyed-mapping
Version with one Section and one Unit per pair. Each Unit is `key<TAB>value<LF>`,
and the Units concatenate to `Version.text` exactly. Raw `version add --kind
keyed_mapping` accepts this same representation; only the final LF may be omitted.
Explicit Sections may group pairs but may not split one pair across Units.

Exact-operation replay returns the committed Version without duplicating content
or changing Study state (the storage CAS revision may advance); a new operation with identical normalized pairs returns the existing Version
without resetting any Study. Different pair boundaries or spelling policy with
the same legacy content hash conflict instead of silently reusing the wrong
Version. Existing historical metadata-only mappings remain readable/exportable;
malformed pair content cannot start a Study.

Forward practice presents the key and grades only the value on the server.
Supplied titles can contain answers, so active mapping Prompts use a neutral
label in JSON and HTML; the original metadata remains in Library/history.
Held restored Prompts are displayed safely without rewriting their saved title,
and an older answer-bearing Prompt receipt cannot replay during protected recall.
Mapping grading is pinned as `exact-v2` with grader-version suffix `;mapping=1`:
Unicode case/compatibility normalization is shared, while punctuation is a word
boundary rather than silently removed inside a word. Thus `X-ray` and `X ray`
agree, but `Xray` and `a.l.f.a` are not accepted as `X-ray` or `Alfa`.
Clients submit an answer, never a grade. Native reveal records assistance before
showing the value; a subsequently copied answer cannot advance the schedule as
unaided recall. Mapping verdict corrections/self-rating are refused. Historical
text Corrections remain preserved under their original contract. Reverse/year/
whole-set exercises, media and MCP are not provided by this mapping import.
Hosted Owner-token admission is still separately unverified (#335).

## Text Study and practice

`learn` returns the one retained Study for an immutable text Version. Pace
applies to new Units. Pause keeps due Cards available. Drop stops presentation
and keeps all history and the Version reference. Explicit resume reactivates
that same Study. Learning again returns the current Study without changing its
Pace or status.

```sh
./target/debug/scry learn VERSION --pace steady --json
./target/debug/scry study list --json
./target/debug/scry next --study STUDY --operation-id retained-prompt-operation --json
./target/debug/scry answer PROMPT --text-file /private/answer.txt \
  --operation-id retained-answer-operation --json
./target/debug/scry next --json
./target/debug/scry next --after PROMPT --json
./target/debug/scry due --json
./target/debug/scry history --json
./target/debug/scry export --json
./target/debug/scry spend --by day --json
```

An ordinary `next` returns the saved Prompt or held result. Only `--after`
releases an answered Prompt. `next --card CARD` requests deliberate extra
practice after the held result is released. `--text-stdin` accepts an answer
without an input file. Empty answers count as misses. Dictated text uses the
same answer field and grader.

`reveal PROMPT` saves assistance before returning the target Units. A cued
Prompt already records assistance. Lower-help Prompt JSON contains only its
allowed cue and context. History, complete export, and Library reads refuse
while an unanswered Free recall Prompt protects its target. Due and Study list
remain available. These reads never record help or consume a Prompt.

```sh
./target/debug/scry next --after PROMPT --study STUDY \
  --recite section --section SECTION --json
./target/debug/scry next --after PROMPT --study STUDY --recite whole --json
./target/debug/scry reveal PROMPT --json
./target/debug/scry correct PROMPT ATTEMPT --verdict Good --unit UNIT --json
./target/debug/scry study pause STUDY --json
./target/debug/scry study resume STUDY --json
./target/debug/scry study drop STUDY --json
```

Recitations require every Unit in the selected scope to have been introduced.
Section prompts contain the heading alone. Whole prompts contain the title
alone. Each target Unit gets its own Verdict and schedule effect.
Correction accepts `Good` or `Again`; `--unit` is required for a Recitation.
It appends an Owner override for the held Attempt and recomputes that Card from
its original state and answer time. The original Attempt remains unchanged.
A second override of that Unit, or a Correction after Next, conflicts.

| Method | Route | Payload or result |
| --- | --- | --- |
| POST | `/api/v1/learn` | `operation_id`, `version_id`, optional `pace` |
| GET | `/api/v1/studies` | All Studies, including paused and dropped |
| POST | `/api/v1/studies/pause`, `/api/v1/studies/resume`, `/api/v1/studies/drop` | `operation_id`, `study_id` |
| POST | `/api/v1/next` | `operation_id`, optional `after`, `study_id`, `card_id`, `recite`, `section_id` |
| POST | `/api/v1/answer` | `operation_id`, `prompt_id`, raw `answer` |
| POST | `/api/v1/reveal` | `operation_id`, `prompt_id` |
| POST | `/api/v1/correct` | `operation_id`, `prompt_id`, `attempt_id`, `verdict`, optional `unit_id` |
| GET | `/api/v1/due` | Presented and upcoming Card schedules, due identities and next due time |
| GET | `/api/v1/history` | Studies, Cards, immutable Attempts and Corrections, referenced Versions and operation receipts |
| GET | `/api/v1/export` | Complete compatible private archive, including the understanding notebook and historical custody |

Practice mutations return `practice` beside the shared `result` and `replayed`
fields. The engine receipt freezes each response reference. Reconcile an
unknown answer or reveal response with the same operation ID and payload
**before Next**. Replay returns the saved immutable Attempt only when the
current assistance boundary permits that disclosure. Once Next has selected a
new unanswered Free recall Prompt, an older answer/reveal replay returns 409
rather than leaking answer-bearing content. Answer or deliberately reveal the
current Prompt before reading earlier Attempts through history; do not create
a new operation to recover a response that may already have committed. Changed
payload under an operation ID conflicts. Every new Attempt names `exact-v2`
and its grader version, the acting token, Version identity and normalized hash,
actual Cue, raw answer, Diff, Verdict and per-Card scheduling effects.

Free recall schedules once per UTC calendar day. Extra practice records an
Attempt and can change cues, but leaves FSRS unchanged. New introductions use
a rolling 24-hour window. The existing FSRS adapter identity remains pinned;
its historical `ALGORITHM` string includes `exact-v1`, while the Attempt's
separate `grade.authority` names the actual `exact-v2` grader.

Answers allow at most 256 KiB. The local edit grid allows at most 1,000,000
cells for the target and each sibling comparison. Capacity refusal preserves
the current Prompt, every Card and all receipts. Smaller Sections avoid that
bound. Complete archives retain the existing 16 MiB limit.

Due, history, export, Study list and spend do not change application rows,
revision, token last-use, receipts or alarms. On first Durable Object activation,
the platform can allocate SQLite, WAL and SHM files before application SQL.
Initialized read stability does not establish a literal cold-filesystem guarantee.
The local synthetic journey does not establish hosted API admission under #335,
physical-phone behavior, private Missal wording or independent hosted recovery.

### Mapping history and rollback

Mappings use the existing Library/Study archive families. Their manifest
operation receipt and `mapping_key` Cue are not understood by v0.8.1. A retained
older build must refuse these records rather than drop keys, regrade values as
whole text, discard history or write a truncated snapshot. After a mapping is
saved, rollback requires a verified compatible artifact retaining all mappings
and Attempts. The old v0.8.1 artifact is a refusal control, not a usable rollback
writer. Never restore an earlier snapshot over acknowledged owner writes.

Release acceptance must retain the tested source/Worker and exact checksums,
a working baseline old-reader restore control, mapping refusal with no writes,
and candidate full-history export/fresh-restore/first-unrelated-write proof.
Local emulator results do not establish off-machine recovery or hosted admission.
