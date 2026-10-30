# Private learner admission: local preparation, not activated

This draft prepares the owner's requested separate learner history. It does not
create an account, change an Access policy, configure a subject, or activate a
learner. It must receive independent security review and exact-source native,
WASM and runtime proof before publication/deployment. Activation is a separate
explicitly approved security configuration change.

## Deliberately small contract

- `LEARNER_SUBJECT` is optional and absent by default. No environment or secret
  file in this change configures it. It accepts one exact immutable Access person
  subject, not an email address, wildcard, service token or browser-selected ID.
- The existing signature, issuer, audience and lifetime checks still apply. A
  matching subject is only a candidate until that complete verification passes.
- The subject maps to `scry-learner-` plus its SHA-256. This stable name uses the
  existing Durable Object namespace. Owner `SPACE_NAME` and QA `scry-qa` remain
  unchanged; no old data is migrated, renamed or copied.
- Empty/malformed configuration, owner-subject collision and owner-space
  collision do not admit a learner. Existing exact QA identity matching has its
  existing precedence and never maps that identity into the learner notebook.
- The Durable Object independently derives its principal from its durable ID,
  including after eviction and alarms. Unknown IDs fail, never fall back to
  owner. Request headers cannot select a notebook.
- Learners can use browser learning, supplied content, private history and their
  own export. Operator, owner-token and owner API capabilities remain unavailable
  at ingress and object boundaries. Existing external-work checks permit only
  Owner, so a learner never inherits owner paid-provider capabilities.
- Browser receipts remain ordinary browser actions (`acting_token: null`) in
  their own private space. They are not attributed to an owner token.
- Learner settings omit owner-token/spending controls and provide a standard
  same-origin Cloudflare Access logout link. Actual Access logout is a provider
  capability, not a newly implemented authentication service.

## Activation requirements (not performed)

1. Identify the intended learner's exact sign-in email and verified immutable
   Access subject through an authorized setup route. Email alone is insufficient.
2. Obtain the user's specific approval for admitting that identity upstream and
   setting the optional subject binding, stating that it grants access only to
   the isolated learner space. Do not replace owner/QA configuration.
3. Verify the deployed identity, isolated private history and denied owner
   capabilities using approved identities. Synthetic JWT/workerd tests cannot
   establish authenticated live acceptance.
4. Removing the subject binding revokes this app's learner admission without
   deleting its retained data. Reinstating the same subject resumes that same
   space; a new subject gets a new space and does not silently inherit history.

## Local checks

`cargo test --locked --test private_learner` covers inert/malformed/colliding
configuration and capability paths. `node --test qa/private-learner.test.mjs`
requires the exact compiled Worker and tests synthetic signed identities,
private NATO/lesson writes and native prayer/sequence grading, owner row
preservation, spoofed headers, invalid JWTs, restart,
revocation, collisions and old QA precedence. No production identity or learner
data is used. Run the existing full runtime suite too before shipping; existing
owner/QA behavior must retain its independent regression proof.
