# ADR 005: Synthetic preview Access service identity

Status: proposed implementation for the authorized isolated revival preview,
2026-10-15. Not production activation, learner-data access, or an import decision.

## Context

SPEC S09.3 permits explicitly scoped and revocable machine QA. Cloudflare Access
service-token JWTs have an empty `sub` and an authenticated `common_name` client
ID. The ordinary exact-owner check correctly rejects them; an Access Service
Auth policy alone cannot exercise the application. Copying an owner's browser
session, acquiring their PIN, or rewriting identity headers is not the solution.

## Decision

Add an explicit `SCRY_ENV=synthetic-preview` mode. It admits only the exact
`SYNTHETIC_ACCESS_CLIENT_ID` secret in a signed Access JWT's `common_name`, with
an empty subject. Existing RS256 signature, issuer, audience, expiry/not-before,
canonical origin, internal capability, CSRF and operation fences still apply.
Ordinary `preview` and `production` continue to require the exact owner subject;
setting the synthetic client ID does not broaden either mode.

Synthetic mode fails closed unless `SPACE_NAME` begins `synthetic-`,
`OWNER_SUBJECT` is the literal non-human marker `synthetic-preview-owner`, and
no model, operator or restore secret is bound. External work is disabled in
this mode, including alarm paths; daily complete snapshots remain enabled.
Pages are labeled synthetic. An operator must provision a physically separate
Worker/Durable Object namespace and private R2 buckets, containing only fixtures.
The existing deliberate authored-demo action also works here, but still refuses
any space with saved goals or events; ordinary preview/production cannot seed it.
The prefix is an extra mistake fence, not proof of physical isolation or data
provenance. This mode must never be applied to an imported learner namespace.

The chief provisions one hostname-scoped Access app, the existing approved Glass
owner identity policy, and one named Service Auth token. The token is encrypted
in the operator password store, never in CI, source, argv, screenshots or logs.
The resource receipt records IDs and delete API requests, without identity or
credential bytes. The broad account token is used only on the chief for this
bounded Access operation; CI receives only the shared narrow deploy credential.

## Consequences and proof

The runtime accepts no generic machine-owner fallback and grants this token no
production read/export/mutation or recovery capability. Token revocation is
independent of the owner policy. A separate isolated namespace is required for
learner-data restore; synthetic QA is not permission to view real records.

The real-workerd regression exercises valid service access and capture/replay,
wrong principal/subject, missing/forged/expired/wrong-issuer/wrong-audience JWTs,
spoofed headers, origin/CSRF rejection, ordinary-mode denial, and mixed-capability
configuration refusal. Deployed Access allow/anonymous denial, exact artifact
readback, screenshots and actual remote recovery need separate receipts.

Cloudflare reference:
https://developers.cloudflare.com/cloudflare-one/access-controls/service-credentials/service-tokens/
