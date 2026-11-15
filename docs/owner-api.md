# Owner API and CLI

Current policy: external agents author content; Scry grades answers natively.
Because generation and self-grading are [SPEC non-goals](../SPEC.md#purpose-and-non-goals),
native content-generation and learner-supplied Verdict/Correction writes return
410, regardless of paid permission. Existing material, Attempts, Corrections,
receipts and usage remain readable under the unchanged assistance/auth boundaries.

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
| GET | `/api/v1/export` | Complete existing archive, including token verifiers; refused (409) only while a Study Prompt awaits an unaided answer |
| POST | `/api/v1/settings` | Retired with the earlier concept notebook's pace: 410 before receipts or writes; a stored pace stays in export |
| POST | `/api/v1/create` | Retired: 410 before jobs, reservations, provider calls or receipts |

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
its Work revision. A correction always creates a new Version that supersedes
the old one, even when only case, punctuation or spacing changed; the old
Version and every Attempt against it stay unchanged. A correction identical in
every field is refused (422) and stores nothing. A Version is corrected once:
correct the newer Version instead (409).

Version flags include `--rights private_only|public_domain|licensed`,
`--provenance supplied|model_from_memory|checked_against_sources`,
`--part-of WORK`, `--related-to WORK`,
`--spelling-strict`, `--sections-file PATH`, and `--ordered-aliases-file PATH`. Rights default to private-only.
Provenance says how the text was obtained: supplied by the Owner or his agent
(the default), written by a model from memory, or checked against the cited
sources, which then need at least one `--source`. Scry records the claim; it
never checks or fetches a source.
`--part-of` and `--related-to` repeat and name a Work already in the Library by
its title or alias. A missing, repeated or self-naming Work refuses the whole
add (422), as does a part-of that would make a Work part of itself. A Relation
appears on both Works it links; one that already exists is not repeated.
`--kind` accepts `text`, `ordered_list`, and `keyed_mapping`. Authored ordered-list
lessons and supplied keyed mappings are described below. For `keyed_mapping`,
`--direction forward|both` and `--safety-bearing PAIR_POSITION` (zero-based,
repeatable) declare what the mapping manifest's `direction` and pair
`safety_bearing` declare. Text files must be UTF-8, at most 256 KiB.
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
`API request failed.` with a nonzero exit. A text that equals an existing
Version, ignoring case, punctuation and spacing, returns that Version and
creates nothing, whatever Work the supplied title or aliases name; its
original bytes, metadata and Relations are kept, and the draft's edition name,
sources, Rights status, Provenance and Relations are not recorded. The response
then carries `"existing": true` (`false` for a new Version), and the CLI's
human output adds `That text was already in the Library. Nothing was added.`
The same text with a different Kind, Units, ordered aliases, mapping pairs or
Spelling-strict setting is refused (409) with nothing written, and the message
names what differs, for example `That text is already in the Library in a
Version with different Units. Nothing was saved; to change that Version,
correct it.` Text supplied without Sections matches a stored text Version
whatever its Units, since Scry divides such text itself.
That refusal applies to what the Owner or an agent adds, never to the Starter
shelf, which is always startable. A lesson, NATO or Study entry for a starter
uses a stored Version the same as the starter's manifest, preferring one with a
Study; otherwise a Version with the same text in any structure that already
has a Study of any status (NATO keeps only one it can practise as the
alphabet), which a lesson start resumes and Study never bypasses; otherwise
the starter's own Version, added beside any copy, which stays unchanged. A new text attaches to
the Work its title or alias names; names that point to different Works conflict.
Search matches titles, aliases, and the first 240 characters of each Version,
and lists matching Works by title. An empty query lists the Library. No matches returns
`{"matches":[],"empty":true,"message":"Nothing matches."}`.

| Method | Route | Payload / result |
| --- | --- | --- |
| GET | `/api/v1/library/search?q=QUERY` | Matching Works by title, with Versions and Relations, explicit empty result |
| GET | `/api/v1/library/works/ID` | Work metadata, Versions with Sections and Units, Relations |
| GET | `/api/v1/library/versions/ID` | Version, owning Work, its version-of and supersedes Relations and its Work's part-of and related-to Relations |
| GET | `/api/v1/library/export` | Only `works`, `versions`, and `relations` |
| POST | `/api/v1/library/versions` | `operation_id`, `data` Version draft |
| POST | `/api/v1/library/versions/supersede` | `operation_id`, `version_id`, Work `revision`, `data` Version draft |
| POST | `/api/v1/library/versions/delete` | `operation_id`, `version_id`, Work `revision` |
| POST | `/api/v1/library/works/edit` | `operation_id`, `data` with `work_id`, `revision`, optional `title`, `aliases`, `context` |
| POST | `/api/v1/library/works/delete` | `operation_id`, `work_id`, `revision` |

The Version draft is an object with `title`, `kind`, `text`, optional `name`,
`aliases`, `context`, `sources`, `rights`, `provenance`, `relations`,
`spelling_strict`, and `sections`. `relations` is an array of at most 32
`{"kind":"part_of"|"related_to","work":"TITLE OR ALIAS"}`.
Each Section has `heading` and an ordered `units` array of exact strings.
Only Library `data` supports nested JSON; other foundation API fields remain
scalar. Mutations return the shared `result.entity_id`, `redirect`, and
`replayed` fields; Version and mapping adds also return `existing`. Show the returned identity to read the committed content.
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
`name`, `aliases`, `context`, `sources`, `rights`, `provenance`, `relations`
and `spelling_strict` (defaults to true), with the same meaning as for a
Version draft. Rights default to private-only. Sources are retained, not fetched.
`direction` is required: `forward` asks each pair from its key, and `both`
also asks it in reverse, showing the value and asking for its key. With `both`,
values must be unique too (each value is then a question), and no pair may be
safety-bearing. A pair may carry `"safety_bearing": true`, its author's flag
that the value is a safe practice the answer must state itself; keep the
critical safety condition in the key. Unknown fields (including generation,
grades and media) are refused rather than silently ignored. A raw
`keyed_mapping` Version draft declares the same with `direction` and
`safety_bearing` (zero-based pair positions); other Kinds refuse both. The CLI reads at most 1 MiB of manifest JSON; the stored Version retains
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
Each pair has one Card per declared direction, introduced together: the Pace
counts the pair once, as one Unit. A reverse Card has its own Source study,
then a Quiz whose Prompt has `cue_level: "mapping_value"`, the value as `cue`
and the title `Recall the matching key`; its answer is graded against the key.
The other direction of the pair just answered is served only when nothing else
is eligible. A Prompt for a safety-bearing pair carries `safety_bearing: true`.
After an answer, `mapping_feedback` names `kind` (`correct`, `wrong_key`,
`spelling`, `mismatch` or `ungraded`), `key` and `expected`. An answer that
is another pair's value is `wrong_key` with `answered_key`; in reverse, another
pair's key is `wrong_key` with `answered_value` and `direction: "reverse"`.
Such an answer is never accepted: when paid judgment would judge it and Jev
accepts it, the check closes unsure and the answer stays saved and not graded.

A value that is a year (one to four digits, alone or with AD or CE before or
after, with or without stops) accepts the same digits with or without AD or CE.
It is graded as an exact number: another year, or a year before the era such
as `44 BC`, is a miss, and with paid judgment on a year-shaped wrong answer is
never sent. An Attempt the year rule decided records grader version
`1;unicode=17.0.0;mapping=1;year=1`; every other mapping Attempt keeps
`;mapping=1`, and replay regrades each under the version it recorded. A
safety-bearing pair never uses the year rule. With paid judgment on, Jev judges
a safety-bearing pair against an objective that asks for the same safe
practice and rejects a dangerous alternative, a negation, a reversal, an
incomplete practice or added unsafe words.
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
text Corrections remain preserved under their original contract. A mapping is
never recited as a whole set: `next --recite` and the join, whole and middle
lesson modes refuse a mapping. Media are not provided.
Hosted Owner-token admission is still separately unverified (#335).

## Authored sequence and phrase lessons

Lessons reuse immutable supplied Versions and native grading. No content is
created by an AI service. Any supplied Version can be a lesson: a text, an
ordered list or a mapping, of any length the Library admits. Import a Version
through `version add`, `mapping add` or their routes, then deliberately start it:

```sh
./target/debug/scry lesson start --version VERSION_ID \
  --operation-id retained-lesson-start-operation --json
# Or use one bundled authored edition:
./target/debug/scry lesson start --starter st-michael-en \
  --operation-id retained-prayer-start-operation --json
./target/debug/scry answer PROMPT_ID --text-file /private/typed-answer.txt \
  --operation-id retained-prayer-answer-operation --json
./target/debug/scry lesson next --after ANSWERED_PROMPT_ID --mode join \
  --operation-id retained-prayer-join-operation --json
```

Answer files contain the learner's typed response; never supply a success grade. Bundled IDs are `rosary-joyful`, `rosary-luminous`,
`rosary-sorrowful`, `rosary-glorious`, `st-michael-en`,
`st-michael-en-lasance-1911`, `st-michael-la-traditional`, and `nato`.
`st-michael-en` is the supplied eight-phrase English wording. The historical
Lasance 1911 edition remains distinct and accessible; neither its content nor
its saved attempts are replaced or transferred to the new edition.

| Route | JSON fields beyond `operation_id` |
| --- | --- |
| `POST /api/v1/lessons/start` | Exactly one `starter` or `version_id`; optional `after` **or** `leave` |
| `POST /api/v1/lessons/next` | Required `after`; optional `mode`: `phrases`, `retry`, `join`, `whole`, `middle` |

`lesson next --mode phrases` serves the next phrase in order. A text that Study's
mix reached first, and whose lesson has not been entered, introduces its
phrases at its Pace: a next phrase without a Card is refused (409, `That phrase
has not been introduced yet.`) with nothing written; use `next` or `mix`, or
start the lesson, which makes every phrase available.

`after` must identify the currently held **answered** Prompt. Switching away
from an unanswered Prompt requires explicit `leave` with that exact Prompt ID
(CLI `--leave PROMPT_ID`). This abandons the unsent draft, not the learner's
history: it records no Attempt, grade or schedule update. Never infer consent to
leave from browsing the lesson shelf. Do not supply both `after` and `leave`.
Same-Version start resumes the held Prompt; stale identifiers conflict.

Text starts with a meaningful authored phrase. `phrases` cycles the clauses;
`retry` shows the missed phrase again as source study, with its help recorded;
it creates a new Prompt but no Attempt until the Quiz that follows is answered.
`join` checks the current phrase's authored Section; `whole` checks the complete
Version from its title alone; `middle` starts a later phrase. Author Sections around
meaningful groups, not screen-width line wraps; their Unit strings must
concatenate to the exact Version text. Learning is typed-text recall only, not
speech recognition or pronunciation assessment. Keep language, edition/version,
source citations and rights explicit in the existing `name`, `context`,
`sources` and `rights` metadata. Do not silently blend translations.

A text or list lesson deliberately introduces every Unit at once without
changing retained Pace or rewriting past schedules; a mapping lesson, starter or
supplied, introduces its pairs at the Study's Pace and `lesson next` continues
it one pair at a time (`mode` `phrases` or none). Existing local grading and
storage bounds still apply: a lesson too large for the archive is refused
before it commits. Assistance never counts as unaided recall or changes the due schedule.
Phrase, joined-Section
and whole-text Attempts retain their different scopes. Isolated phrase success
does not establish complete-text recall.

An `ordered_list` lesson presents the complete sequence. Submit one item per
line; Scry checks item identity and order, not a caller-supplied rating. Optional
`ordered_aliases` maps zero-based flattened Unit positions to explicitly
authored names, for example `{"0":["The Annunciation"]}`. CLI authors pass that
object with `--ordered-aliases-file PATH`. Each item permits at most 16 aliases;
normalized aliases/canonical names must not collide across items. Aliases are
not fuzzy matches. Omission, misplaced items, repeated entries and unknown
entries remain distinct feedback; only the complete correct sequence earns a
correct sequence result. New aliases are immutable Version content, not an edit
to already-recorded evidence. An empty alias map preserves old serialized and
reimport identity.

These routes use the same retained operation-ID replay contract as other
mutations. Their source availability does not prove hosted token admission or
authenticated learner acceptance.

## Text Study and practice

`learn` returns the one retained Study for an immutable text Version. Pace
applies to new Units. Pause keeps due Cards available. Drop stops presentation
and keeps all history and the Version reference. Explicit resume reactivates
that same Study. Learning again returns the current Study without changing its
Pace or status.

An owner or authorized notebook token can explicitly checkpoint the current
unanswered prompt. `scry draft` reads the saved text and generation;
`scry save-draft PROMPT --generation N --text-stdin --operation-id ID` saves a
new value; `scry discard-draft PROMPT --generation N --operation-id ID` removes
the saved draft. Both require an explicit operation ID; retain it and the exact
input to reconcile a lost response.
The generation is a compare-and-swap: a stale save conflicts. The mutation receipt
contains no answer text. The draft belongs to the notebook, not an individual
token, and its acknowledged text is included in complete recovery archives.
Drafts are limited to 32 KiB; JSON and CLI carriage returns and NUL are refused,
as are line feeds in a single-line mapping answer. Browser form CRLF is decoded
to the textarea’s LF representation; bare carriage returns remain invalid.
Saving does not grade or advance. Only acknowledged text can be relied on as
durable; a lost response may follow a commit. Typing or editing alone is not
saved automatically.

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

`reveal PROMPT` saves assistance before returning the target Units. The revealed
Prompt becomes source study: `quiz PROMPT` hides it again and the answer that
follows is assisted. A Quiz Prompt's JSON carries its title, section heading or
mapping key and nothing answer-bearing. History, complete export, and Library
reads refuse while an unanswered Free recall Prompt protects its target. Due and Study list
remain available. These reads never record help or consume a Prompt.

```sh
./target/debug/scry next --after PROMPT --study STUDY \
  --recite section --section SECTION --json
./target/debug/scry next --after PROMPT --study STUDY --recite whole --json
./target/debug/scry reveal PROMPT --json
./target/debug/scry quiz PROMPT --operation-id RETAINED_OPERATION_ID --json
./target/debug/scry study pause STUDY --json
./target/debug/scry study resume STUDY --json
./target/debug/scry study drop STUDY --json
./target/debug/scry study pace STUDY --pace light --json
```

`study pace` chooses an existing Study's Pace: `light` (3 new Units per
rolling day), `steady` (6, where every Study starts) or `intense` (12). It
changes only how many new Units arrive; Cards, schedules and status are
unchanged, and a dropped or paused Study keeps the chosen Pace for when it is
resumed. `pace` is required; an unknown value or any other field is refused
(422).

### Study's mix

The app's **Study** and **Next** serve Study's mix: genuinely due, previously
practised material first, interleaved across Studies by least recent service,
then unfinished Studies and Works not yet started. Agents take the same action:

```sh
./target/debug/scry mix --operation-id RETAINED_OPERATION_ID --json
./target/debug/scry mix --after ANSWERED_PROMPT --json
./target/debug/scry mix --leave UNANSWERED_PROMPT --json
```

Without `--after` or `--leave` it returns the held Prompt or result, without
choosing another. `next` and `lesson next`, except a phrase retry, leave the
mix, as choosing a Study or a Lesson does in the app. `--after`
releases the answered Prompt; `--leave` closes the identified unanswered Prompt
without a grade. When it is the turn of a Work not yet started, the mix starts
that one Study and nothing else. With nothing eligible it returns
`{"prompt":null,"due":...}`. `due`, `study list` and Library reads never
start it. Responses carry the same `practice` as `next`, under the same
assistance fence.

Recitations require every Unit in the selected scope to have been introduced.
Section prompts contain the heading alone. Whole prompts contain the title
alone. Each target Unit gets its own Verdict and schedule effect.
A Verdict cannot be overturned: `/api/v1/correct` is retired and returns 410
before any receipt or write, and the CLI has no `correct` command.
Historical Corrections retain their original meaning: an Owner override for the
held Attempt, recomputed from the original Card state and answer time, without
changing the original Attempt. Read them through history/export; no new
learner-supplied Verdict is accepted.

| Method | Route | Payload or result |
| --- | --- | --- |
| POST | `/api/v1/learn` | `operation_id`, `version_id`, optional `pace` |
| GET | `/api/v1/studies` | All Studies, including paused and dropped |
| POST | `/api/v1/studies/pause`, `/api/v1/studies/resume`, `/api/v1/studies/drop` | `operation_id`, `study_id` |
| POST | `/api/v1/studies/pace` | `operation_id`, `study_id`, `pace`: `light`, `steady` or `intense` |
| POST | `/api/v1/mix` | `operation_id`, optional `after` **or** `leave`; Study's mix |
| POST | `/api/v1/next` | `operation_id`, optional `after`, `study_id`, `card_id`, `recite`, `section_id` |
| POST | `/api/v1/answer` | `operation_id`, `prompt_id`, raw `answer` |
| POST | `/api/v1/reveal` | `operation_id`, `prompt_id` |
| POST | `/api/v1/quiz` | `operation_id`, `prompt_id`; explicitly hide source and begin a memory quiz |
| POST | `/api/v1/correct` | Retired: 410; existing Corrections remain readable through history/export |
| POST | `/api/v1/check` | `operation_id`, `attempt_id`; one more paid check of a saved answer that has no Verdict |
| GET | `/api/v1/due` | Presented and upcoming Card schedules, due identities and next due time; `checking` names Cards whose answer is still with Jev |
| GET | `/api/v1/history` | Studies, Cards, immutable Attempts and Corrections, referenced Versions and operation receipts |
| GET | `/api/v1/export` | Complete compatible private archive, including the understanding notebook and historical custody |

Practice has two forms for every Study and every client, however the Study
was started: `practice.phase: "study"` (source study: the supplied material,
no answer and no grade) and `"quiz"` (source hidden, raw answer graded
natively). A Unit not yet practised is presented as study first, whatever its
Kind; one already answered is presented as a Quiz. An ordered list is always
presented whole, however many of its Cards exist.

In study, `prompt.cue_level` is `read` and `prompt.cue` is the supplied text of
the Units in scope: a text phrase, a mapping pair as its key, a tab and its
value, or every item of an ordered list on its own line. POST `/api/v1/quiz`
with that `prompt_id` creates a separately fenced Quiz of the same Units and
scope, without creating an Attempt or changing schedules. An answer or a draft
sent against a study Prompt is refused with 409 from every client. Revealing a
Quiz records help and returns it to study; returning to Quiz preserves that
help, so the answer is assisted.

Scry never fades cues, shows first letters or uses a Card's stored cue level
to choose a Prompt. Cue levels appear only in records made before this rule.
A Prompt such a record still holds, with `practice.phase` absent, can be
answered once and is recorded as it would have been; `quiz` also applies to
its visible source. Every Prompt after it has a phase. Existing Attempts and
their original assistance and schedule evidence are never reinterpreted.

Rollback safety: a Study keeps `source_quiz` once one of its Prompts has been
held, even when no Prompt is held now. Its archives use the phase-v3 format and
older readers must refuse them. A held study Prompt for a mapping pair or a
whole list is also refused by any earlier build's validation. This archive
fence does not make an older binary safe against the same live SQLite store:
do not downgrade that store to an earlier runtime.

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
the Prompt as presented, raw answer, Diff, Verdict and per-Card scheduling effects.

An Attempt whose answer was left for Jev also carries `judgment`. Its `grade`
is then the local comparison only: the Diff, never the Verdict. The Verdict and
its Authority are `judgment.decided`, absent while the answer is being checked
or is saved and not graded, and an Attempt with no Verdict has no scheduling
effect. Each check of that answer is a `checks` record in history and export. A
practice response that names such an Attempt adds `practice.judgment`: `state`
(`checking`, `ungraded` or `decided`), `verdict`, `authority`, `reason` and
`check_id`. An archive that holds any of these records is named
`scry-rust-durable-sqlite-study-judged-v4`, and earlier readers refuse it.
A notebook whose Library holds a Provenance other than supplied, or a
correction whose normalized text equals another Version's, is named
`scry-rust-durable-sqlite-library-v5` instead, which every earlier reader
refuses.

### Paid judgment (Jev)

Paid judgment is off in every notebook until the signed-in Owner turns it on in
Settings. That switch is a browser form (`POST /settings/judgment`); no API
route, CLI command or tool reaches it, and an Owner token is refused whatever
permission it carries. Asking for the setting it already has is refused with
nothing written (409). Settings offers it only where this Scry has Jev's key,
model and endpoint bound; elsewhere it says paid judgment is not available.

An answer leaves Scry to be judged only when all three hold: this Scry can
reach Jev, the switch is on, and the request carries paid permission (the
signed-in Owner, or an Owner token created with `--paid`). Otherwise it is
graded by the local comparison exactly as before and nothing is sent or
reserved. An exact or alias match, an empty answer, any answer to
Spelling-strict content and an answer too large to send (32 KiB with its
expected content) are always graded locally.

One check is one request to Jev. Besides the model name it carries, for that
answer only: the answer as typed; the expected content of the Prompt's scope;
what names the question (the Work's title and the place in it, or a mapping's
key, or a Section's heading); the Learning objective; and, for an ordered
list, the authored aliases of its items. It carries no identifier, no other
content of the Library, no token and nothing that identifies the Owner.
Settings says the same before the Owner turns the switch on.

When an answer is left for Jev, `answer` returns at once with
`practice.judgment.state: "checking"` and `scry answer` exits 0: the answer is
saved, it has no Verdict yet and its Card is unchanged. One request is sent,
about a second later. Read the outcome by repeating the same operation, or from
history:

| `state` | Meaning |
| --- | --- |
| `checking` | A Check is queued or sent. `verdict` and `authority` are null. |
| `decided` | Jev accepted (`Good`) or rejected (`Again`). `authority.name` is `jev`. |
| `ungraded` | Saved, not graded. `reason` says why: Jev was unsure, failed, was refused, could not be reached, or the allowance was used. |

An ungraded answer keeps its Card due, so Next serves that Card again. While a
Check is queued or sent, `next`, a lesson's Next and Study do not choose that
Card (`due.checking` lists it and `due.due` omits it); when nothing else is
eligible they return the held result still being checked. A Card or Recitation
named outright (`--card`, `--recite`) is still presented; a Verdict that then
arrives for the earlier answer is recorded with `superseded: true` on its
effect and leaves the schedule as the newer Attempt set it.

```sh
./target/debug/scry check ATTEMPT --json
```

`check` asks for one more check of the same saved answer. It never creates a
second Attempt. It needs paid permission (403 without), and is refused with
nothing written when Scry graded the answer itself (409), the switch is off or
Jev is not reachable (409), a check is already open (409), the answer already has its Verdict (409), the Study was
dropped (409), the daily allowance is used (429) or three checks are already
saved for that answer (422). Each Check is sent at most once. A send that is
interrupted becomes `ungraded` with its 0.50 reservation kept as unknown spend;
Scry never retries by itself.

Free recall schedules once per UTC calendar day. Extra practice records an
Attempt but leaves FSRS unchanged. New introductions use
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

Mappings use the existing Library/Study archive families. A notebook that
holds a Version declared `both` or with safety-bearing pairs, a reverse Card,
or an Attempt the year rule decided is written with the row
`mapping_compatibility/v1` and archived as
`scry-rust-durable-sqlite-mapping-v6`, which earlier Workers refuse; a notebook
using none of these keeps its exact rows and archive. Their manifest
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
