# Spend reads

`scry spend --by day|purpose|model --json` reads the same projection as
`GET /api/v1/spend?by=day|purpose|model`. The selector is required. An Owner
token (including an unpaid token) or authenticated Access owner may read it.
There is no spend-write endpoint. CLI errors remain JSON with a nonzero exit;
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
as a bill. Existing allowance enforcement is unchanged: `App.spent(now)` sums
reported amounts or held reservations for entries newer than `now - 24h`.
Report days instead use the reservation's `created_ms` UTC calendar date,
including midnight, with no rolling-day filter. Reporting rejects arithmetic
overflow instead of silently truncating a sum.

Entries expose only cost metadata: ledger `id` (send lease), existing `work_id`,
`work_kind` (`concept_job`, `assessment`, or `unattributed` when `work_id`
matches neither), optional `goal_id` (`null` when unattributed), `created_ms`,
`day`, `purpose`, `model`, `status`, and the entry monetary fields above.
Use `work_id` to sum the calls for one existing job or assessment; `goal_id`
connects generation, transcription and critic jobs to the saved request.
These are older notebook identities, not Library Work or future Generation run
identities. Raw provider responses, prompts, answer text, titles, token metadata,
verifiers and credentials are excluded. `model` is the reserved/requested model;
the provider's raw response/model receipt remains private in the full archive.

New reservations use `grade.jev`, `critic`, `generate.concepts`, or
`transcribe.photo`. Concept generation is not `generate.version`; #322 owns
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
provider receipts and retained Go custody bytes are unchanged. New complete
archives containing a purpose use
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
