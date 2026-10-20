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

## Limits

Synthetic local data and emulated viewport only. No provider spend, live token,
production credential, deployment, canonical-origin change, live-space archive,
physical phone, hosted Access-policy change or independent remote recovery was
performed. Library/Study operations remain #316/#317. The CLI currently targets
Unix hosts. Parent-owned independent QA/review and protected exact-head CI are
separate requirements before merge; this proof does not authorize activation.
