# Scry stdio MCP

`scry-mcp` is a small native adapter over the same Owner HTTP client as `scry`.
External agents author content; Scry validates, stores and grades it. It does
not generate content, accept a caller's grade, manage tokens, or expose arbitrary
HTTP/filesystem actions. Multimedia and reverse mapping authoring are not yet
supported. This is local source support, **not proof of hosted admission**;
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
| `scry_schema` | Machine-readable Version and forward mapping manifest schemas |
| `scry_search` / `scry_show` | Existing private Library reads, with the native assistance fence |
| `scry_validate` | Native structural validation in a disposable empty local Library |
| `scry_import` | Existing Version or mapping import; no automatic Study |
| `scry_learn` / `scry_due` | Start/resume the retained Study, or read due Cards |
| `scry_next` | Current Prompt/held result; deliberate `after` releases an answered Prompt |
| `scry_answer` | Raw learner answer, graded by Scry; no verdict/rating/correct field |
| `scry_reveal` | Record assistance before disclosing the target |

`scry_validate` and `scry_import` accept `format: "version" | "mapping"` and a
`data` object matching `scry_schema`. Import additionally requires
`operation_id`. Version drafts use the existing Library shape. Mapping drafts
use `title`, `direction: "forward"`, and `pairs: [{"key":"A","value":"Alfa"}]`,
plus the supported optional metadata. Both reuse native Library validation.
Validation does not query the remote Library or prove deduplication, available
storage, authorization, Study admission, learning efficacy or deployment.

Search before import; show the returned `result.entity_id` to inspect what was
stored. A new agent must not infer that the running Worker matches its binary's
schema: unknown/unsupported routes fail safely, not silently downgrade content.
Stored ordered lists are not advertised as practisable by this slice.

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
