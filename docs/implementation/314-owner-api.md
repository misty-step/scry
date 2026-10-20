# Owner token / API / CLI foundation (#314)

Fixed stack base: `4ea47e6331a626842bddac9fca5eeeafc02a89bd`
(PR #312, initially `docs/exact-recall-spec`; requester changed the target to
`master` after #312 merged). Behavior: SPEC US-014.1–8,
AG1, S06.1–4, S09.1–3, S10 and D5. Content APIs follow in #316/#317.

Interface sketch (before implementation):

```text
Access owner -> Settings token form (Origin + CSRF)
Owner bearer -> /api/v1 -> verifier row lookup -> authenticated owner
  -> engine::mutate_as(actor, canonical path, fields, operation ID)
     -> existing receipt/payload hash, domain fences, atomic next-state
     -> acting-token receipt; queued work retains originating token
  -> LearningSpace::save(expected revision), existing SQLite transaction

OwnerToken = public metadata + SHA-256 verifier; no stored secret
MutationActor = browser | verified Owner token
API = whoami, tokens create/list/revoke, shared mutation adapters,
      history/export (existing assistance gates)
CLI = Rust HTTP only; stdin login, private local config; no local engine/store
```

Invalid bearer requests may query only the addressed verifier row. They must
not initialize storage, load/recover learner state, arm alarms, or update last
use. Successful authorization can update token last use. Token creation's
operation receipt never contains the secret: only its first response can show
it; replay returns metadata and the committed result. Lost creation responses
require revoking that token and deliberately minting another.

Paid authorization happens before committing a request for paid work and at
reservation/transmission. Browser writes keep Origin/CSRF, including JSON API
writes authenticated through Access. No live tokens, provider sends, deployment,
production changes, store imports or historical-data edits belong to this work.
