# Scry stdio MCP

`scry-mcp` is a small native adapter over the same Owner HTTP client as `scry`.
External agents author content; Scry validates, stores and grades it. It does
not generate content, accept a caller's grade, manage tokens, or expose arbitrary
HTTP/filesystem actions. Multimedia is not supported. This is local source support, **not proof of hosted admission**;
[#335](https://github.com/misty-step/scry/issues/335) remains unverified.

## Connect

```sh
cargo build --locked --bin scry --bin scry-mcp
# First log in using your existing private Owner token source, not an argv value:
./target/debug/scry login --url https://YOUR-CANONICAL-ORIGIN --token-stdin
./target/debug/scry-mcp
```

Configure an MCP host to launch the absolute `scry-mcp` binary path. To choose
an existing private login file, set the launch arguments to `["--config",
"/private/path/login.json"]`. Otherwise the same CLI config path is used:
`$XDG_CONFIG_HOME/scry/login.json` or `$HOME/.config/scry/login.json`. Discovery
and local validation work without a login; API tools require one. The adapter
reads, but never creates or rewrites, that file. It must be a regular, non-symlink
file with no group/other permissions. HTTPS is required outside loopback.
The normalized origin **and credential** are pinned on first API use for the
lifetime of the MCP process: even a same-origin replacement token can select a
different principal. Changes are refused before transmission. Reconcile pending
operations against their original origin and principal before starting a new
process for a different login. Only a fingerprint is retained, never logged.
Redirects and HTTP retry policies are disabled. No browser cookie or Cloudflare
Access credential is borrowed. A Scry token cannot by itself bypass Access.

There are no command-line token, endpoint override, generation or grade options.
Keep credentials and real answers out of host transcripts. Successful practice
results intentionally include the data the Owner API permits, including raw
answers and native grading feedback; protocol errors never echo input or an
upstream error body.

## Surface

The `tools/list` schemas describe each exact input and reject unknown fields.
Results contain the same JSON in both `structuredContent` and a text content
block. Tool failures set `isError: true` with a bounded `error` object, including
`code`, `message`, and `outcome_unknown`, and HTTP `status` when available.

| Tool | Purpose |
| --- | --- |
| `scry_capabilities` | Adapter limits and supported subset; not a remote health check |
| `scry_schema` | Machine-readable Version and mapping manifest schemas |
| `scry_search` / `scry_show` | Existing private Library reads, with the native assistance fence |
| `scry_validate` | Native structural validation in a disposable empty local Library |
| `scry_import` | Existing Version or mapping import; no automatic Study |
| `scry_learn` / `scry_due` | Start/resume the retained Study, or read due Cards |
| `scry_next` | Current Prompt/held result; deliberate `after` releases an answered Prompt |
| `scry_mix` | Study's mix, as the app's Study and Next serve it; `after` releases an answered Prompt, `leave` closes an unanswered one ungraded |
| `scry_lesson_start` | Start/resume a lesson for any supplied Version (text, list or mapping) or a bundled starter; explicit `after` or `leave` controls a switch |
| `scry_lesson_next` | Advance phrases, join their authored Section, try the whole text, or practise from the middle |
| `scry_answer` | Raw learner answer, graded by Scry; no verdict/rating/correct field |
| `scry_check` | Check again: one more paid check of a saved answer that has no Verdict |
| `scry_reveal` | Record assistance before disclosing the target |
| `scry_quiz` | Deliberately hide the held source and start a memory quiz; no grade or schedule credit |

`scry_validate` and `scry_import` accept `format: "version" | "mapping"` and a
`data` object matching `scry_schema`. Their input schemas carry no top-level
`oneOf`, `anyOf` or `allOf`, which common tool-calling clients refuse; `data`
is described as an object and Scry's native validator checks it whole against
the schema `format` names. Import additionally requires
`operation_id`. Version drafts use the existing Library shape. Mapping drafts
use `title`, `direction` (`"forward"`, or `"both"` to practise each pair in
reverse too, which needs unique values and no safety-bearing pair), and
`pairs: [{"key":"A","value":"Alfa"}]`, where a pair may add
`"safety_bearing": true`, plus the supported optional metadata. A year value
accepts the year with or without AD or CE, as an exact number, and a year
answer is never sent to Jev. Exactly another pair's value (or key in reverse)
is the wrong key: Again at once, never sent. A reverse
Prompt has `cue_level: "mapping_value"` and shows the value; see the Owner API
guide for the result's `mapping_feedback`. Both reuse native Library validation.
Both formats take an optional `provenance` (`supplied`, the default,
`model_from_memory` or `checked_against_sources`, which needs a source) and
`relations`, an array of `{"kind":"part_of"|"related_to","work":"TITLE OR ALIAS"}`
naming Works already in the Library. Validation checks the shape of each
Relation and reports how many there are; import checks that each named Work
exists, and refuses the whole manifest otherwise. A text already in the
Library returns its Version with `existing: true` and records nothing from the
manifest, Relations included; the same text with different Units, aliases,
pairs or Spelling-strict setting is refused with what differs.
Validation does not query the remote Library or prove deduplication, related
Works, available storage, authorization, Study admission, learning efficacy or
deployment. Library corrections and deletions, and pausing, resuming,
dropping or pacing a Study, are API and CLI actions, not tools.

`scry_mix` is Study's mix, the action SPEC calls Study and Next: due reviews
across every Study first, then unfinished Studies and Works not yet started. It
takes `operation_id` and optionally `after` (the answered Prompt) or `leave`
(the unanswered Prompt to close ungraded), never both, and no Study, Card,
Pace or grade. Without either it returns the held Prompt or result.

Where the Owner has turned paid judgment on and the login's Owner token carries
paid permission, `scry_answer` may return `practice.judgment.state: "checking"`:
the answer is saved with no Verdict while Jev judges it. Repeat the same
`scry_answer` call (same `operation_id` and arguments) to read `decided` or
`ungraded`. For an `ungraded` answer, `scry_check` with `operation_id` and
`attempt_id` asks for one more paid check of the same Attempt, under the
[Owner API's rules](owner-api.md#paid-judgment-jev): each check reserves 0.50
US dollars of the daily allowance and an answer is checked at most three times.
It returns the same `practice.judgment`, in state `checking`. The adapter has
no tool that turns paid judgment on or off.

Search before import; show the returned `result.entity_id` to inspect what was
stored. A new agent must not infer that the running Worker matches its binary's
schema: unknown/unsupported routes fail safely, not silently downgrade content.
Ordered lists support whole-sequence lessons with native item/order feedback.
Version manifests can include `ordered_aliases`, an object mapping zero-based
flattened Unit positions to authored alias arrays. Empty aliases preserve old
Version serialization; nonempty aliases are immutable content. Scry validates
collisions and bounds locally; aliases never relax NATO spelling policy.

For a supplied text, call `scry_lesson_start` with `operation_id` and
`version_id`. Alternatively use `starter: "st-michael-en-lasance-1911"` or
`starter: "rosary-joyful"`. Exactly one selector is required. `after` releases
the current answered Prompt; explicit `leave` abandons the identified unanswered
Prompt without recording a grade or schedule update. Never supply both.
Browsing is not permission to abandon the learner's draft.

When `practice.phase` is `study`, show the source without an answer input.
Call `scry_quiz` with a retained `operation_id` and that `prompt_id` only when
the learner chooses to try from memory. Its new Prompt hides the source and
creates no Attempt or schedule credit. Source-phase `scry_answer` is refused.
A reveal returns to source study; hiding it again preserves assistance, so the
subsequent answer cannot count as unaided recall. `scry_next` and `scry_learn`
follow the same two forms as a lesson: a Unit not yet practised arrives as
`study`, a practised one as `quiz`, and an ordered list always arrives whole.
Scry never fades cues or shows first letters. A Prompt held without a phase
comes from an older record: it can be answered once, and `scry_quiz` applies to
its visible source.

Submit raw typed text for the quiz with `scry_answer`. Then call `scry_lesson_next` with a
new retained `operation_id`, the answered Prompt ID in `after`, and `mode`:
`phrases`, `join`, `whole`, or `middle`. `join` uses the current authored Section;
`whole` is a distinct complete-text challenge. Existing `scry_reveal` records
help before disclosure. Studying alone earns no recall credit; helped answers
do not become unaided or whole-text credit. No speech/pronunciation judgment is implied. See
[authored lesson and metadata contract](owner-api.md#authored-sequence-and-phrase-lessons).

## Frozen operation and interruption contract

For **every mutation**, create and retain an `operation_id` before sending,
along with the exact tool arguments. IDs are 16–128 ASCII letters, digits,
hyphens or underscores. The adapter never invents an ID or retries a request.
MCP JSON-RPC request IDs are separate: use a fresh request ID for each call,
while reusing the original Scry operation ID and exact payload for reconciliation.

On a lost/invalid response or uncertain proxy failure, the mutation may have
committed. Resend the **same operation and exact payload before Next**. Within
the current MCP process, an unresolved mutation blocks other mutations; due
reads and local discovery remain available. The guard stores only a digest,
not an answer, and is not durable across restarts. The calling agent remains
responsible for retaining the frozen operation and reconciling it after restart.
An authentication failure while reconciling does not clear that guard.

A changed payload under the same operation ID conflicts. Additionally, the
native assistance fence returns 409 for an older answer, reveal, held result,
or answer-bearing Prompt replay while a newer unaided Prompt is unanswered.
Do not bypass that refusal with a fresh ID or Reveal. If another client advanced
before reconciliation, finish the current Prompt through the Owner API/CLI,
then retry the original frozen operation here; the session guard stays in place
until it is settled. Prior history also becomes readable after the current
Prompt is answered. This corrects the
unsafe inference that old answer-bearing responses are always readable after
Next; see [#349](https://github.com/misty-step/scry/issues/349).

Cancellation is optional in the negotiated protocol. This synchronous adapter
cannot cancel an HTTP request once admitted; it has a 30-second overall HTTP
timeout and may still return the completed result. `notifications/cancelled`
receives no acknowledgement, never triggers a retry, and does not imply rollback.
If the host discards that result, reconcile the original operation before any
new mutation. Cancellation reasons are never logged.

## Protocol subset and verification

This implementation follows the official MCP **2025-11-25**
[stdio transport](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports),
[lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle),
[tools](https://modelcontextprotocol.io/specification/2025-11-25/server/tools), and
[optional cancellation rules](https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/cancellation).
It uses existing Rust dependencies, not an added SDK. Supported methods are
`initialize`, `ping`, `tools/list`, and `tools/call`; the client must send
`notifications/initialized` before tools. If another protocol version is offered,
the response proposes 2025-11-25; an incompatible client must disconnect.
No resources, prompts, sampling, elicitation, subscriptions, task augmentation,
pagination, HTTP MCP transport or concurrent call execution are advertised.

Input/output is UTF-8 newline-delimited JSON-RPC, never Content-Length framing
or batches. Incoming frames are bounded at 4 MiB and API responses at 16 MiB;
unterminated, oversized, malformed and invalid UTF-8 frames are rejected without
echoing them. The session retains at most 10,000 unique request IDs (strings up
to 256 bytes or integers); reconnect after that bound, retaining operation IDs.
EOF closes the process after any admitted synchronous request completes.

```sh
node --test qa/mcp.test.mjs
```

This gate builds the real binary and drives a wire client over pipes with an
isolated loopback HTTP mock: handshake/discovery, notifications and unsupported
methods, malformed framing, schema/domain refusal, all route/payload mappings,
response loss and exact replay, refusal/redirect privacy, credential file bounds,
and cancellation without retry. It uses only synthetic content and a synthetic
token. These tests do not establish compatibility with every MCP host, a deployed
Worker, real Owner token admission, multimedia, or learning efficacy. The native
engine and built-Worker tests remain separately required; a mock does not prove
grading or durable persistence.

For built-Worker integration (after the repository's normal Worker build):

```sh
node --test qa/mcp-worker.test.mjs
```

This separate gate uses the compiled CLI/MCP, a synthetic local Owner token,
and the actual workerd SQLite runtime. It authors all 26 NATO pairs, starts a
Study, submits raw answers to native grading, checks immutable answer replay
(with the expected transport CAS revision increment), and checks assisted
Reveal plus the newer-unaided-Prompt disclosure fence. Reveal has its own
recorded disclosure Attempt; it is not a successful recall. It asserts no
provider calls, jobs, assessments, spend or caller-written grade. Its receipt
binds the tested MCP and Worker WASM hashes. It does not prove hosted admission,
phone/browser usability or general MCP-host interoperability.
