# Spend reads

`scry spend --by day|purpose|model --json` reads the same projection as
`GET /api/v1/spend?by=day|purpose|model`. The selector is required. An Owner
token (including an unpaid token) or authenticated Access owner may read it.
There is no caller-write or hold-release endpoint. CLI errors remain JSON with a nonzero exit;
the client retains its private login file, origin check and redirect refusal.

The response contains `by`, `currency: "USD"`, `day_basis: "UTC"`, `totals`,
sorted `groups` (`key`, `totals`), and ledger-ordered `entries`. All monetary
fields are integer millionths of a US dollar. A group's totals are the sums
of precisely those entries whose selected `day`, `purpose`, or `model` equals
its key; global totals are the sums across every entry and every group.

| Field | Meaning |
| --- | --- |
| `entries` / `unknown_entries` in totals | Entry count / count with no reported cost |
| `reserved_micros` | Original reservation bound, including entries subsequently measured; this is not a bill or an outstanding balance |
| Entry `cost_micros` | Provider-reported amount, or `null` when unknown; a reported zero remains zero |
| `known_cost_micros` in totals | Sum of reported amounts only; unknown entries contribute no measured amount and remain counted separately |
| `unknown_held_micros` | Original reservations for entries whose cost remains unknown; zero for known entries |

`reserved_micros` overlaps the other monetary columns: never add all three
as a bill. The retained rolling-day guard counts recent measured costs and all unresolved
reservations, regardless of age. It is an additional restrictive guard, not the
monthly hard-budget admission described below.
Report days instead use the reservation's `created_ms` UTC calendar date,
including midnight, with no rolling-day filter. Reporting rejects arithmetic
overflow instead of silently truncating a sum.

Entries expose only cost metadata: ledger `id` (send lease), existing `work_id`,
`work_kind` (`concept_job`, `assessment`, or `unattributed` when `work_id`
matches neither), optional `goal_id` (`null` when unattributed), `created_ms`,
`day`, `purpose`, `model`, `status`, and the entry monetary fields above. New
entries can also include their non-secret `quote` and an
`enforcement_breach: true` marker.
Use `work_id` to sum the calls for one existing job or assessment; `goal_id`
connects generation, transcription and critic jobs to the saved request.
These are older notebook identities, not Library Work or future Generation run
identities. Raw provider responses, prompts, answer text, titles, token metadata,
verifiers and credentials are excluded. `model` is the reserved/requested model;
the provider's raw response/model receipt remains private in the full archive.

Historical reservations use `grade.jev`, `critic`, `generate.concepts`, or
`transcribe.photo`. The reviewed budget contract currently admits only
`generate.concepts`; other paid request shapes are explicitly refused. Concept generation is not `generate.version`; #322 owns
Generation agent calls and their run identities. Missing or empty historical
purpose/model display as `unattributed`, without inference or writeback.
Nonempty historical values are retained verbatim.

Spend GET/HEAD authenticates synchronously before reading records. It bypasses
the ordinary load/save path: no table/store initialization, token last-use,
revision, operation receipt, recovery, reservation, alarm or provider effect.
`whoami` and other token metadata behavior are unchanged.

## Compatibility and rollback

Missing attribution fields deserialize as absent and remain omitted on write.
Existing model strings and explicit empty strings round-trip; amounts, statuses,
provider receipts and retained Go custody bytes are unchanged. Budget-aware archives use `scry-rust-durable-sqlite-budget-v1` when quote or
settlement-time metadata is present. Optional fields missing
from old records remain absent; old writers must not attach to a populated
budget-aware SQLite store. New complete archives containing only a purpose use
`scry-rust-durable-sqlite-attributed-spend-v1`, including Library/custody state
when present. Old restore builds refuse this format. Archives without purpose
retain their previous format and bytes on encode/decode.

This is not a populated-state rollback to the parent binary. Its SQLite reader
does not understand purpose and can silently discard it on a later write; it
also cannot read a missing model. The archive marker guards restores, not direct
attachment of an old writer to current SQLite. Retain the compatible source,
Worker/assets and complete verified archive. A populated rollback needs a build
that understands these optional fields, or a forward fix preserving them.
Never restore an earlier snapshot over acknowledged work to roll code back.

Verification is in `tests/spend.rs`, `qa/spend.test.mjs`, and the existing
controlled provider lifecycle in `qa/runtime.test.mjs`. See the
[feature navigation](../features/owner-tokens.md) and
[builder receipt](qa/spend-321/README.md). This is US-019.7 only; US-019's
`Evidence: pending` remains until the Generation agent slice completes it.


## Disabled-by-default monthly admission (#375)

A legacy model/key setting does not enable paid work. `SCRY_AI_BUDGET_POLICY`
must be explicit JSON with `contract`, `monthly_micros`, and `request_micros`.
There is no default amount, and this change supplies no production binding.
The contract identifier is `openrouter-gpt41-mini-text-2026-10-24`; it refuses
new sends on or after 2026-11-30 UTC pending a new contract review.

The current narrow contract is OpenRouter Chat Completions, model
`openai/gpt-4.1-mini`, OpenAI provider only, no fallback, mandatory parameter
support, 12,000 maximum output tokens, text-only messages and JSON output.
Tools, search, plugins, images, files, alternate providers, service tiers and
Jev are not admitted. It is not the US-019 Library Generation agent.

The request sets exact integer ceilings of $1/million input tokens and
$2/million output tokens, and zero request/image fees. These are route price
ceilings, not an estimated bill or an approved spend allocation. The reservation
is the entire published 1,047,576-token context at the input ceiling plus the
output limit at its ceiling: 1,071,576 microdollars. No tokenizer heuristic or
fixed per-call guess is used. This deliberately pessimistic maximum includes
all input framing; excess context must be rejected by the pinned provider.
For this non-reasoning model, cache reads are discounted and there is no
separate cache-write/long-context fee in the reviewed pricing contract.

Primary contract sources, reviewed 2026-10-24:
[OpenRouter model/context/pricing](https://openrouter.ai/openai/gpt-4.1-mini/performance),
[OpenAI model contract](https://developers.openai.com/api/docs/models/gpt-4.1-mini),
[OpenRouter price and parameter routing](https://openrouter.ai/docs/guides/routing/provider-selection).
These documented controls are not proof that any current key has a cap.

Admission sums measured costs in the UTC reservation month, plus costs settled
in the current month, plus every unresolved reservation from all
months. Arithmetic overflow or a charge above its quote blocks further paid work. The full measured charge
is retained, never clamped to its quote. A blocked job keeps its input and
explicit error; exact practice continues without a provider.

The runtime commits the exact request hash, contract, monthly/per-request caps,
period and reservation in the existing Spend row before its first await.
Concurrent claims share the singleton's synchronous revision-fenced SQLite
transaction. Immediately before send it rechecks the current policy, month,
lease, source, paid-token authority and both the retained rolling-day and
monthly limits. The exact quote must fit every limit, not the legacy provisional
$0.50 reserve. No automatic retry follows an unknown
outcome. Cancellation, timeout, missing usage, malformed responses and HTTP
errors do not prove a free request and therefore cannot erase a saved hold.

## Unknown outcomes are not caller-releasable

This unit has no reconciliation API, CLI command or caller-supplied billing
metadata. A paid token permits bounded work, not permission to declare its
unknown charge zero. Browser owners cannot assert away a hold either.
Archiving a Goal changes learning-publication state; it does not prove the
provider request ended or release any reserved money. Restart and month rollover
retain the same obligation.

Only the existing authoritative provider-result path can settle a measured
charge and preserve its complete bounded response receipt. A late result can
record actual spend even when stale learning is rejected. Exact response replay
is idempotent and conflicting response bytes/cost remain refused. If a received
response had no cost, its immutable receipt and full hold remain: a separate
verified provider-billing reconciliation capability is deferred, not simulated
with an operator string or extra token authority.

## Activation remains separate

Before activation, obtain explicit owner monthly/per-request amounts and verify
the dedicated provider key's actual settings, UTC/reset and concurrency
behavior. Provider budget settings are not an absolute spending guarantee:
[OpenRouter documents in-flight overage](https://openrouter.ai/docs/guides/features/workspaces/workspace-budgets).
Scry's own pre-send reservation is the bound, conditional on the reviewed model,
context/token and price contract being enforced for every billed dimension.
Confirm no BYOK/upstream invoice, shared-key traffic, extra fees or additional
billable dimension can evade that contract, then run separately authorized
bounded live acceptance. If any assumption cannot be verified, keep paid
generation off. This source change supplies no production budget amount and
makes no claim of a live billing guarantee.
