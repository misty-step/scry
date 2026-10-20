# Owner token foundation development proof (#314)

Initial fixed base: `4ea47e6331a626842bddac9fca5eeeafc02a89bd` (PR #312).
After #312 merged and its branch was deleted, the requester selected `master`.
The implementation was rebased before its first push onto
`fd7e51fcf2c45f53b6af662c9aadf64822d56ae4`; no spec contracts were changed.
The PR retains the final committed-source gate and exact head.

## Observed journeys

- `runtime-owner-api.txt`: eight US-014 criteria PASS against the built
  Rust/WASM Worker in real workerd with synthetic RS256 Access, SQLite and local
  R2. The compiled Rust CLI uses the actual loopback TCP listener. Its sanitized
  transcript records login/whoami, token create/list/revoke, creation replay,
  payload conflict, refused revoked login, self-revocation and subsequent refusal.
  Issuance secrets are removed from the retained transcript.
- Refusal probes cover missing, malformed, never-issued, wrong-secret, expired
  and revoked tokens, including malformed credentials alongside valid Access,
  oversized unauthenticated input and unreadable learner metadata. Refusals are
  identical; SQLite rows stay unchanged. Only verifier lookup is admitted.
- Shared browser/API operation replay and payload conflict, history/export
  acting-token attribution, default paid refusal before any job/reservation,
  cookie CSRF on every exposed foundation write and cross-origin refusal are
  exercised by the same runtime journey.
- `native-owner-api.txt`: focused native transition tests PASS. The final source
  additionally checks UTC expiry, exact token-free historical row readback,
  archive compatibility, and revocation between staging and reservation.
- `phone-token-metadata.png` and `phone-token-revoked.png`: inspected collaborative
  browser screenshots at 390×844 CSS pixels after actual create → secret →
  return → revoke actions. The secret page was observed without retaining its
  value or taking a screenshot. Metadata and revoked state fit without horizontal
  overflow. The desktop journey was also driven at 1280×800.

These initial logs/screenshots are worktree development observations. The final
PR's committed-source gate supersedes them for exact-source verification.
No historical converter, recovery receipt or live store was edited. Empty token
state remains absent from legacy serialization; populated token state uses new
verifier rows and defaulted attribution fields.

## Paid grant regression (2026-10-20)

[Review finding](https://github.com/misty-step/scry/pull/332#pullrequestreview-5587633931):
the original US-014.7 check covered direct paid work but missed an unpaid token
minting a paid child. The following proof supersedes that criterion's initial
receipt and extends US-014.2 and .8. Unchanged browser, historical restore and
other gate observations retain their original source and limits.

Tested implementation: `7339de8ba0c88818bb890086845251af8910d59f`.
Fix base: `f6721b8187c94cf32efccef8d439512b0d1c84bc`; original PR base:
`fd7e51fcf2c45f53b6af662c9aadf64822d56ae4`. This receipt-only follow-up changes
no executable source or tests. Mirrodin, Rust 1.98.1, worker-build 0.8.7,
Node 22.22.0, pinned Miniflare/workerd, disposable synthetic SQLite and local R2.
Synthetic RS256 Access admits the Owner; outbound requests permit only its
synthetic JWKS. The CLI uses the real loopback TCP Worker.

Before the engine fix, the new native grant test failed because token creation
returned `Ok` instead of 403. The built original Worker failed US-014.7 with
`Unpaid tokens cannot grant paid permission: 200 !== 403`; US-014.8 failed
because `scry token create ... --paid --json` exited 0. The other six live
criteria passed. The native test bytes are unchanged between the failing and
passing runs. The CLI assertion was corrected to expect the existing generic
403 message; its old-source nonzero-exit assertion still failed first.

After the fix, these checks passed:

- `cargo test --locked --test owner_api --test application`: 4 + 18 tests.
- `cargo test --locked --lib persistence`: 4 tests.
- `cargo fmt --all -- --check`, native all-target Clippy and WASM-target Clippy,
  both with `--locked` and `-D warnings`.
- `worker-build --release --locked`.
- `node --test qa/owner-api.test.mjs`: all eight live criterion functions.
- `node --test qa/walk-cli.test.mjs`: 26 discovery checks, retained from the
  combined run before correcting the CLI message assertion.

The API sequence verifies unpaid-to-unpaid creation/list/revoke, unpaid parent
and ordinary child refusing a paid grant, Access Owner-to-paid and paid-to-paid
issuance, and the authorized child's successful `/api/v1/create`. Refusals leave
every SQLite row unchanged, including tokens, operation receipts, last use and
the spend ledger. The CLI paid grant exits 1 with `{"error":"Request refused."}`
and unchanged rows. Issuance secrets and seeds are absent from persisted rows.
The credential-free Worker keeps the spend ledger empty. Separately, the native
engine claims the authorized child's work, reserves exactly 500000 synthetic
microdollars and passes transmission authorization without sending anything.

Artifact SHA-256, original and fixed respectively:

```text
953526c9550e0d305d1a42788ba7c95bbb0250f7807aac0affb2db574b7b255d  old index_bg.wasm
dd3d7552d1af315e25e4df0560bb8de1759e5f1531d77e7679838cac37c840f1  fixed index_bg.wasm
379232ce8dd9b6b2c08dc392ae9d2727e9a050ab3eea079b307f811483c9583a  fixed src/engine.rs
0ed6779b3e732e99fdfa9c1f667d6f8e922bb176687307b9f97af75acd89c242  tests/owner_api.rs
0f5f15632c06e39f966695eb758cf9e42a2d57dbaf114c0d0a1812e09c6a81ea  fixed qa/owner-api.mjs
```

No provider transmission/spend, production data or credential change, private
environment read, hosted policy change, deployment or merge occurred. Hosted
admission remains UNVERIFIED; [the documented follow-up](../../owner-api.md#hosted-admission-follow-up)
belongs to #313/#314. Final independent delta QA, review, judgment and protected
CI remain required at the resulting PR head.

## Limits

Synthetic local data and emulated viewport only. No provider spend, live token,
production credential, deployment, canonical-origin change, live-space archive,
physical phone, hosted Access-policy change or independent remote recovery was
performed. Library/Study operations remain #316/#317. The CLI currently targets
Unix hosts. Parent-owned independent QA/review and protected exact-head CI are
separate requirements before merge; this proof does not authorize activation.
