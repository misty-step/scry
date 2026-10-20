# Owner sign-in

The owner, phaedrus@mistystep.io, opens https://scry.mistystep.io on phone or
computer, signs in once through Cloudflare Access with his Misty Step password,
and lands in his own notebook. Everyone else is refused before any content.
Intent: [S09](../SPEC.md#s09--keep-the-application-private); the dated
configuration is in the [runbook](../docs/runbook.md#owner-subject-re-pointed--2026-10-20).

## Sub-features

- `signin-owner` signs in as phaedrus@mistystep.io and renders an owner-only page.
- `signin-signed-out` sends anonymous requests to the Access login, never the Worker.
- `signin-expired` refuses an expired or forged session and re-authenticates.
- `signin-wrong-identity` keeps the isolated QA and machine identities out of the owner notebook.
- `signin-alternate-host` redirects `scry.study` reads to the canonical origin and refuses its writes.
- `signin-config` (Doctor) checks deployed Access and Worker identity configuration against the owner's real identity.

## How to get to it (user POV)

- Open https://scry.mistystep.io (or `scry.study`, which redirects) in any browser.
- Access redirects automatically to the "Misty Step password" sign-in at
  `auth-canary.mistystep.io` ("Welcome back"): email, password, Sign in.
  Normal sign-in has no email code or magic link. "Set or reset your password"
  is recovery, which verifies the mailbox through an OTP-only Access app.
- After sign-in Access returns to the requested page. The session lasts 720h.

## Driving it with verify-scry helpers

Preconditions:

- Pinned Node and `npm ci --ignore-scripts` (Playwright and its Chromium).
- `pass` unlocked with the read entries named in [verify-scry](../.agents/skills/verify-scry/SKILL.md#credentials).
- `doctor.mjs prod` has no FAIL.

- **Configuration.** Run `.agents/skills/verify-scry/scripts/doctor.mjs prod`.
  PASS lines show: Worker `SCRY_ENV=production` and `CANONICAL_ORIGIN`; the
  account's Access team equals `ACCESS_TEAM`; "Scry production owner" AUD
  equals `ACCESS_AUDIENCE` and covers all three hosts; its only allowed IdP is
  "Misty Step password"; an exact-email allow policy includes
  phaedrus@mistystep.io; his Access user exists, with a `user_uuid`
  fingerprint (the JWT `sub`). `owner.subject` stays UNPROVEN: `OWNER_SUBJECT`
  is a secret and cannot be read, so only the owner end state proves admission.
- **Owner sign-in.** Run `.agents/skills/verify-scry/scripts/owner-signin.mjs`
  (`--width 1280` for desktop). Signed out, `/create` goes Access → Misty Step
  "Welcome back". With the password entry present, it signs in as
  phaedrus@mistystep.io and requires `Create · Scry` with `[data-state="create"]`
  on https://scry.mistystep.io/create, then drops the app cookie to show
  re-authentication. Exit 0 PROVEN, 3 UNPROVEN (credential missing), 1 FAIL.
- **Breaking states.** Run `.agents/skills/verify-scry/scripts/prod-states.mjs`.
  Anonymous `/`, `/create`, `/settings` and `/api/v1/library`; an expired cookie;
  a forged assertion; and a forged service token all get 302 to the Access login.
  The QA identity reaches only its `scry-qa` space, with owner API 401 and
  operator 403. The machine identity cannot reach `/create`. `scry.study` GET
  gives 303 to canonical, and POST gives 421. The QA view at 390px has no
  overflow. A Bearer token to `/api/v1/whoami` still meets Access (#335).
- **Proof.** Keep each run's `target/verify-scry/<stamp>-*/checks.jsonl`,
  `navigation.txt` (query strings redacted) and screenshots.

## Gotchas

- **Owner sign-in is UNPROVEN for agents as of 2026-10-20: needs the owner's
  Misty Step password in pass entry `workstation/MISTY_STEP_OWNER_PASSWORD`**
  (override with `SCRY_OWNER_PASSWORD_ENTRY`). The IdP is password-only, and
  [misty-step/auth](https://github.com/misty-step/auth) currently says agents
  never read passwords or mailbox codes. Enabling the existing "One-time PIN
  login" IdP on the owner app, with codes read through Fastmail JMAP
  (`workstation/FASTMAIL_API_TOKEN`; mistystep.io MX is Fastmail), is the other
  route. Both are the owner's decision; do not change Access or reset his password.
- Recovery ("Set or reset your password") revokes his identity sessions. It is
  never a verification path.
- The JWT `sub` is the Access `user_uuid`, one per email and not per IdP.
  Misty Step password emits one canonical email for both owner addresses
  (misty-step/auth SPEC), so either address should reach the same Access user.
  An Access user for another email, such as one from an older OTP login, has a
  different `user_uuid` and is refused by design. That refusal is not yet observed.
- The Access user's last-seen identity is account-wide. A One-time PIN login to
  any other Misty Step app changes it, so it says nothing about Scry's IdP.
- No owner page GET is write-free. Every one, `/create` included, may freeze
  his next concept question exactly as opening Scry does. `/` also freezes the
  next exact Prompt, and `/history`, `/goals/*` and `/concepts/*` mark exposure.
  `/create` is the least-writing owner page, with none of those extras, and it
  renders no notebook content. That is why the end state uses it; it is the only owner page agents load.
- Production serves the code in the runbook's serving observation and can lag master.
  On 2026-10-20 it served `69435af`, where `/notebook` is "A page has gone astray".
- `pass-env run -e` injects an entry's trailing newline, so Access treats the
  service token as absent. The helpers read the first line of `pass show`.
- A redirect to Access, a 200 for the QA identity or an Access "allowed" log is
  not owner sign-in. Only `signin.owner` PASS is.
