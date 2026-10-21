# Sign in with the QA account

The owner opens https://scry.mistystep.io on phone or computer, signs in once
through Cloudflare Access with the owner's Misty Step password, and lands in
the owner's notebook. Every other identity is refused before any
content. The one exception is the dedicated QA identities: the AgentMail
account below and the QA service token. They reach only the isolated `scry-qa`
space, never the owner's notebook.
Agents never use the owner's account. They sign in through the same production flow as
the dedicated AgentMail QA account `scry-qa@agentmail.to`, which phaedrus
authorized via Home on 2026-10-21. The Worker gives that identity the isolated
QA principal: owner capabilities inside the `scry-qa` space, and no owner
operations. Intent: [S09](../SPEC.md#s09--keep-the-application-private). The
dated configuration is in the [runbook](../docs/runbook.md#owner-subject-re-pointed--2026-10-20).

## Sub-features

- `signin-config` (Doctor) checks Access, the IdP and the policies, the owner
  identity and each QA allowlist step.
- `qa-enrollment` gives the QA account its separate password through OTP-only
  `/recover`, with the PIN read from its own AgentMail inbox.
- `qa-session` signs in through Access and Misty Step password in a fresh
  browser. `/cdn-cgi/access/get-identity` must be the QA email.
- `qa-role` checks that the Worker renders the `scry-qa` space with owner
  capabilities and refuses owner operations.
- `signin-expired` refuses an expired or forged session and re-authenticates.
- `signin-signed-out` sends anonymous requests to the Access login, never the Worker.
- `signin-wrong-identity` keeps the QA service token and the machine identity
  out of the owner notebook.
- `signin-alternate-host` redirects `scry.study` reads to the canonical origin
  and refuses its writes.

## How to get to it (user POV)

- Open https://scry.mistystep.io (or `scry.study`, which redirects) in any browser.
- Access redirects automatically to the "Misty Step password" sign-in at
  `auth-canary.mistystep.io` ("Welcome back"): email, password, Sign in.
  Normal sign-in has no email code or magic link. After sign-in Access returns
  to the requested page, and the session lasts 720h.
- First enrollment or QA password recovery: open
  `https://auth-canary.mistystep.io/recover` in a fresh browser. Request a PIN
  for the QA address, read it from the QA inbox and submit it, then choose the
  QA password.

## Driving it with verify-scry helpers

Preconditions: those in the [feature map](README.md#baseline-preconditions),
the QA pass entries and allowlists in
[verify-scry](../.agents/skills/verify-scry/SKILL.md#qa-account), and no FAIL
from `doctor.mjs prod` (its checks are listed in
[Doctor](../.agents/skills/verify-scry/SKILL.md#doctor)).

- **Enrollment, once.** Run `.agents/skills/verify-scry/scripts/enroll-qa.mjs`
  after Doctor shows `qa.recover-policy` PASS and Process has confirmed
  household membership. Its end state is `enroll.saved` PASS: "Password saved.
  Use it to sign in to your account." It refuses to request a PIN while the
  `/recover` policy lacks the QA address.
- **QA sign-in and role.** Run `.agents/skills/verify-scry/scripts/qa-signin.mjs`
  (`--width 1280` for desktop). The end state is PASS for every one of:
  - `signin.redirect`: signed-out `/create` → Access → "Welcome back", no Scry content;
  - `signin.qa`: credentials accepted, back on https://scry.mistystep.io;
  - `signin.identity`: Access identity = `scry-qa@agentmail.to`;
  - `role.create`: `Create · Scry` with `[data-state="create"]`;
  - `role.settings`: `Your rhythm · Scry` with `#owner-tokens` and the Create
    Owner token form;
  - `role.not-owner`: `GET /operator/restore` → 403 "QA cannot invoke owner
    operations." This is what proves the identity is not the owner principal.
    Only the Worker's QA principal gets this answer, and that principal's only
    space is `scry-qa`;
  - `signin.expired`: without the app cookie, Access re-authenticates.
- **Breaking states.** Run `.agents/skills/verify-scry/scripts/prod-states.mjs`.
  Anonymous `/`, `/create`, `/settings` and `/api/v1/library`, an expired
  cookie, a forged assertion and a forged service token all get 302 to the
  Access login. The QA service token reaches only its `scry-qa` space, with
  owner API 401 and operator 403. The machine identity cannot reach `/create`.
  `scry.study` GET gives 303 to canonical and POST gives 421. The QA view at
  390px has no overflow. A Bearer token to `/api/v1/whoami` still meets Access (#335).
- **Proof.** Attach the reviewed `qa-create`, `qa-settings-tokens` and
  `signin-idp` screenshots, plus the check lines, to the PR comment, as in
  [Evidence](../.agents/skills/verify-scry/SKILL.md#evidence).

## Gotchas

- **QA sign-in is UNPROVEN as of 2026-10-21.** The pass entries and the Scry
  Access QA policy exist. Still missing, in order:
  1. Process adds the address to misty-step/auth `HOUSEHOLD_MEMBERS`.
  2. Process adds it to the `/recover` allow policy.
  3. `enroll-qa.mjs` enrolls the account.
  4. PR #365 (Worker `QA_EMAIL` → isolated QA principal) is merged and
     promoted. Promotion first needs PR #361, the fix for #359, merged.

  Until step 4, a QA login that Access admits gets the Worker's 401 "Owner
  authorization required", which `role.create` reports as UNPROVEN. The
  helpers name whichever step is missing. Never weaken the journey or
  substitute another identity.
- This proves the QA identity's role, never an owner session. No helper loads
  an owner page as the owner, tests a password against the owner's account, or
  reads the owner's mail.
- `enroll-qa.mjs` resets only the QA password and revokes only QA auth
  sessions. Run it for enrollment or recovery, not on every walk. A received
  PIN alone is partial proof; the saved password and a fresh login must follow.
- `HOUSEHOLD_MEMBERS` is a secret and cannot be read. Doctor infers membership
  only from an enrolled account, and `qa-signin.mjs` proves it currently holds.
- The JWT `sub` is the Access `user_uuid`, one per email and not per IdP. The
  owner's subject equality cannot be read back. `owner.subject-age` catches
  only an `OWNER_SUBJECT` put, per the audit log, made before every owner-policy
  Access user existed. A PASS does not show the subject is the right user.
- Every page GET may freeze the next question in the space it renders; `/`
  also freezes the next exact Prompt. The QA journey's GETs therefore write
  only to `scry-qa`.
- Production serves the code in the runbook's serving observation and can lag master.
- A redirect to Access, a 200 for the QA service token or an Access "allowed"
  log is not QA sign-in. A QA login without `role.not-owner` PASS is not role proof.
