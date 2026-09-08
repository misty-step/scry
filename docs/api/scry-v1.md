# Scry v1 API handoff

Scry's Rust engine owns memory science: source ingestion, validated automatic
quiz publication, optional editing/removal, due queue selection, reveal, grading,
scheduling, and source archival. Scry owns the product experience: account UI,
study layout, navigation, copy, reminders, and client-side state.

External clients use only `/v1/...` JSON routes. Browser-only CSRF fields and
HTML forms are not part of this contract. Token clients authenticate with:

```text
Authorization: Bearer <sessionToken>
```

The session token comes from an invite magic-link or an operator-issued
pre-provisioned account session. The token is secret; receipts and demos must
never print it. Anonymous account creation is not part of this contract.

## Contract Files

- `docs/api/openapi.v1.json` is the checked-in v1 contract.
- `GET /v1/openapi.json` serves the same contract from the API.
- `cargo test -p memory-engine-api v1_ -- --nocapture` proves the route table,
  OpenAPI methods, Bearer auth, and full machine-to-machine loop agree.
- `cargo test -p memory-engine-contract` starts the real API router on
  localhost, runs the external consumer runner over HTTP, fetches the served
  OpenAPI contract, completes the full loop, archives its source, and proves
  receipts redact credentials.
- `docs/qa/scry-v1-production-contract.md` is historical native-host evidence;
  current shipment and recovery proof belongs with `docs/runbook.md`.

## Consumer Demo

Provision a disposable local or staging account through the invite magic-link
or operator service-session flow. Then run the contract against that account
without printing the session token:

```sh
MEMORY_ENGINE_ACCOUNT_ID=acct_... \
MEMORY_ENGINE_SESSION_TOKEN="$SESSION_TOKEN" \
cargo run -p memory-engine-contract -- \
  --base-url http://127.0.0.1:18080
```

The production demo uses the same pre-provisioned account contract. Production
account creation is allowlist-gated. The default origin is
`https://scry.misty-step.workers.dev`; there is no native-provider fallback.
Existing machine credentials keep their recorded origin. After the verified
cutover, explicitly update that origin or pass `--base-url` for the new host;
the migrated Bearer token remains valid until its existing expiry or revocation.

```sh
MEMORY_ENGINE_ACCOUNT_ID=acct_... \
MEMORY_ENGINE_SESSION_TOKEN="$SESSION_TOKEN" \
cargo run -p memory-engine-contract
```

The runner creates a disposable source, enqueues generation, and polls it to a
bounded terminal state. Successful generation publishes reviewable quizzes without
an approval request; the runner selects
a review, reveals and submits the answer, archives the source, and emits a
redacted receipt with the source absent from the active list. A revealed answer
records assisted exposure (`Revealed`/`Again`), never successful unaided recall.
The synchronous `/generate` route uses the same admission/trust policy, but
queued generation is the preferred interface for restart-safe clients.

## Scry Integration Notes

Scry should treat `ReviewUnitId` as opaque. It should not infer concept,
phrasing, or scheduling meaning from IDs. The client can keep its own view
state, but the current due item, projected multiple-choice choices, revealed
answer, grade, attempt count, post-answer feedback, item history, concept
health rollup, and schedule result come from the engine response.

Generated material exposes source spans and provider/model/prompt-version
provenance. `approved` means published, not manually endorsed; `learnerDecision`
stays null until an actual edit or removal. Historical drafts are separate from
the live `queue` inventory. Rejected quizzes retain their history but leave review.
Source `title` is optional and inferred when omitted.

`GET .../review/next` inspects live inventory while preserving a held graded
card. `POST .../review/next` deliberately advances. Reload, listing, capture, and
editing must not consume feedback or repeat a schedule update. Correct maps to
Good; Close, Wrong, and Revealed map to Again, independent of response speed.

After a submit, `current.feedback` carries human-language result text, the
expected answer, this item's attempt history (`lastSeen` plus
`lastSeenSummary`, success trend, and response-time trend), and the matching
concept rollup. `current.choices` is already projected in the engine response;
clients should not pin answer position across reviews.
`conceptProgress` is the management-surface list of attempted concepts sorted
weakest first. It is derived from the existing attempt log and review-unit
concept metadata; clients should not build a separate analytics store for v1.

Use the OpenAPI file as Scry's source of truth for routes and schemas. If the
engine adds prompt enums, grader verdicts, or queue semantics, update the
contract, API tests, contract runner, and this handoff doc in the same change.
