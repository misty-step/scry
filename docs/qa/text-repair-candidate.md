# Isolated legacy text repair candidate

Draft qualification only. Normal HTTP admission remains closed. No production migration, automatic enrollment, merge or deployment is part of this candidate. This follows the unsafe-text quarantine in #465/#466; it does not resume parked Materials, spend, recitation, media or private-learner work.

## Contract

One additive repaired copy per unsafe Text Version. Preserve exact UTF-8 and every immutable original, historical Attempt/Correction, schedule and acknowledged draft. Pin the repair partition and normalization dependency closure (exact-v2/Unicode 17/Rust 1.98.1). Recompute the canonical preview fingerprint on the server, including original identity, bytes, boundaries, metadata and proposal. Exact operation replay is unchanged; a new operation must validate current Work revision and fingerprint even when a copy already exists. Full App/row/archive capacity is checked before commit. New lineage, SQL compatibility marker, archive envelope and Library export envelope make actual predecessor readers refuse rather than silently discard metadata. A pre-repair snapshot is not a safe rollback after later acknowledged history.

Preview, result, receipt and recovery UI are metadata-only. Existing assistance refuses unsafe originals; that fence is not bypassed. Copy enrollment is an explicit separate action. The recovery UI requires a held prompt; if it belongs to another Version, the screen identifies that fact and preserves it. No-current state offers no repair/enrollment controls and closes to Library. Native forms bind the current notebook scope in their action URL; enhanced/API requests retain scope headers, with independent authentication, Origin and CSRF checks.

CLI repair-preview returns immutable Version, Work revision, canonical fingerprint, supported policy and bearer scope. Repair requires all of those input preconditions plus a retained explicit operation ID. Transport/5xx/unreadable/invalid success responses preserve outcome_unknown and direct exact complete-argument retry. Token values and source text are never reflected. Acting-token audit timestamps are separate from repair replay; preview's whoami preflight also advances App revision.

## Qualification

`tests/text_repair.rs` covers preservation/capacity/refusal/replay, historical state matrix, malformed envelopes, policy and fingerprint goldens, real native enrollment and multi-Section/multi-Unit Good/Again grading, no-current recovery, and a held prompt from another Version. Fixtures remain synthetic. Native tests emit bounded run-owned fixtures under target/text-repair.

The existing keyed-mapping CI/browser gate runs `textRepairWalk`. It builds a second test-only Worker in a temporary source copy with `scry_synthetic_repair_http`, verifies the normal Worker bytes were not replaced, and stores the test artifact separately. The opt-in runtime additionally requires an exact synthetic origin, space, Access issuer/audience/owner and absent provider/restore secrets. This is not an activation switch for production.

Hosted Chromium covers mobile-width native JavaScript-disabled and enhanced controls, held versus different-Version prompts, source hiding, acknowledged draft, Close/Back/reload, repeated native form and enhanced post-commit response-loss replay, separate enrollment, and stale-notebook rejection. Native replay uses an unchanged server-rendered cached document and an actual no-JS form submission. Browser requests are routed into real workerd with an ephemeral signed synthetic Access identity; no production credentials or learner data are used. This is not real external Access/TLS commissioning or physical-phone acceptance.

Screenshots and JSON belong to the exact CI run under target/keyed-mapping/text-repair and the usual keyed-mapping artifact. Test source alone is not a pass. The local Chromium route was not retried or bypassed. Before draft publication, 19 focused native tests and all-target native Clippy passed; hosted results are pending. Full standard CI and independent review remain required. No merge/deploy is authorized by this document.
