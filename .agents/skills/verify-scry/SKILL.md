---
name: verify-scry
description: Verify Scry end to end from a shell. Sign in to production https://scry.mistystep.io with the dedicated AgentMail QA account (scry-qa@agentmail.to) and prove its isolated owner role, check Access and Worker identity configuration, probe signed-out, expired, wrong-identity and alternate-host states, and drive the notebook, Owner token/CLI, Library and phone exact-Study flows on a disposable local Worker. Use it after any change to verify user-visible behavior or sign-in.
---

# Verify Scry

Current policy: the [2026-10-27 retirement amendment](../../../SPEC.md#review-first-entry-and-retired-native-generation-2026-10-27-406)
retires native generation and new self-grading writes. Retained generation or
correction journeys below are historical compatibility instructions only;
current HTTP must refuse them with 410 and no writes/provider work. Native
answer grading remains required; unavailable grades stay saved and ungraded.
Legacy `/create` retains its title/state for safe reads, but no capture form.
Historical content, private ingress, assistance boundaries and application
backup/restore validation remain unchanged. The [October 29 deployment
amendment](../../../SPEC.md#minimal-deployment-2026-10-29) retires hosted recovery
drills and custom operational gates as deployment prerequisites.


Start with [the feature map](../../../features/README.md) and the SPEC stories
it names. It is the navigation inventory for verification and fitness, not
another acceptance contract. Repository instructions live in `AGENTS.md` only.
Claude Code reads this skill through the `.claude/skills/verify-scry` symlink.

## QA account

Agents sign in to production only as the dedicated AgentMail account
`scry-qa@agentmail.to`, never as the owner. Its credentials follow the
`workstation/<PROJECT>_QA_*` convention with `<PROJECT>` = `SCRY`. phaedrus
authorized this setup via Home on 2026-10-21: the AgentMail account and inbox,
generated credentials in pass, and adding this exact address to the
allowlists below. Nothing else is authorized: no other credentials, no owner
data and no owner sign-in. Never ask for phaedrus's password or a browser
profile he signed into. Missing QA access is reported as `QA sign-in UNPROVEN`
(exit 3) with the missing step named.

**Required role.** The Worker gives this identity its isolated QA principal.
That is the owner/admin role inside the `scry-qa` learning space, never the
owner principal or the owner's space. `qa-signin.mjs` checks it in order:

1. `/cdn-cgi/access/get-identity` returns the QA email.
2. `/create` renders `Create · Scry` with `[data-state="create"]`.
3. `/settings` renders Owner tokens with the Create Owner token form.
4. `GET /operator/restore` returns 403 "QA cannot invoke owner operations."

Only the Worker's QA principal gets that refusal. The owner principal never
does, and the QA principal's only space is `scry-qa`.

**Allowlists**, each with an exact-email include:

| Where | Owner | State on 2026-10-21 |
| --- | --- | --- |
| Access app "Scry production owner", policy "scry-qa AgentMail identity" | done | live |
| misty-step/auth `HOUSEHOLD_MEMBERS` (secret on `misty-step-auth-canary`) | Process only | live; Process confirmed 19:00Z |
| Access app "Misty Step auth pilot recovery (OTP only)" (`/recover`) allow policy | Process only | live; Process confirmed 19:00Z |
| Worker `QA_EMAIL` var and its mapping to the QA principal on `scry-rust-production` | Scry PR #365 | live since v0.5.0 (deployed 2026-10-21 20:11Z) |

Never write the household list or the `/recover` policy yourself. Ask Process
for the exact address.

## Credentials

Read by pass entry name. Never print, log or commit a value. The helpers read
the first line of `pass show` themselves, because `pass-env run -e` keeps the
stored trailing newline and Access then treats service tokens as absent.

| Use | Entry |
| --- | --- |
| QA address, inbox, AgentMail API key, password | `workstation/SCRY_QA_EMAIL`, `_INBOX`, `_AGENTMAIL_KEY`, `_PASSWORD` |
| Access apps, IdPs, users, audit log, auth D1 counts (read) | `workstation/CLOUDFLARE_API_TOKEN` |
| Worker settings and deployments (read) | `workstation/SCRY_CLOUDFLARE_DEPLOY_TOKEN` |
| Isolated QA service token (`scry-qa` space) | `misty-step/SCRY_QA_ACCESS_CLIENT_{ID,SECRET}` |
| Machine health `/healthz` | `misty-step/SCRY_ACCESS_CLIENT_{ID,SECRET}` |

The QA address and inbox entries must both equal `scry-qa@agentmail.to`. The
helpers check this before any mailbox read or password submission.

## Launch

Production is `scry-rust-production`, behind the Access app "Scry production
owner". Read the [runbook](../../../docs/runbook.md) for the dated serving
source, which can lag master. Production needs no launch. It gets only GETs and
QA sign-in.

Local write flows need the built artifacts once per source change:

```sh
export PATH="$HOME/.cargo/bin:$HOME/.local/share/node-22.22.0/bin:$HOME/.local/bin:$PATH"
npm ci --ignore-scripts --no-audit --no-fund
npx playwright install chromium   # or set SCRY_CRITICS_CHROMIUM_PATH
worker-build --release --locked
cargo build --locked --bin scry
```

`local-journeys.mjs` starts its own workerd SQLite/R2 space on a free loopback
port with no egress and stops it when it finishes. Hermes workers run builds
and browsers on Mirrodin under `/srv/hermes-chief-workspaces/<task-id>`. For
manual browsing, `bun run dev` ([scry-qa](../scry-qa/SKILL.md)) prints a
run-owned loopback URL. Home offers Review and the explicit "Start with NATO"
Library entry; reading either page must not import or consume practice.

## Doctor

```sh
.agents/skills/verify-scry/scripts/doctor.mjs prod
.agents/skills/verify-scry/scripts/doctor.mjs local http://127.0.0.1:8787
```

`prod` is read-only: Cloudflare GETs, one counting SELECT on the auth D1
database, an AgentMail inbox listing (no message read), anonymous requests and
machine health. It checks:

- the Worker vars, and that the account's Access team equals `ACCESS_TEAM`;
- the app AUD equals `ACCESS_AUDIENCE`, and "Misty Step password" is its only
  IdP (password-only);
- the owner policy is exact emails only, with no wildcard, domain, everyone
  or QA address, checked without printing the addresses;
- the QA policy is exactly `scry-qa@agentmail.to`, and service-token
  policies admit only service tokens;
- no bypass or everyone rule on any app covering a Scry host;
- `owner.subject-age` dates the newest `OWNER_SUBJECT` put from the account
  audit log. It FAILs if that put predates every Access user in the owner
  policy, because then `OWNER_SUBJECT` cannot be any of their uuids. With no
  audit entry, or a put between those users' creation times, it is UNPROVEN;
- the QA pass entries and inbox are readable;
- `qa.household`, `qa.recover-policy`, `qa.enrollment` and
  `qa.worker-qa-email` each report the allowlist step above. A step not yet
  live is UNPROVEN, not FAIL. A wrong value is FAIL. The household list is
  unreadable, so `qa.household` is proven indirectly: it passes once the auth
  account exists, and `signin.qa` PASS shows the password login still succeeds.

`local` refuses a non-loopback host before sending anything. Exit codes: 0 all
PASS, 1 any FAIL, 2 usage, 3 only UNPROVEN. Run Doctor first, and again after
anything surprising.

## Drive

Run in order. Each helper prints one line per check, with a distinct id per
step, and exits like Doctor.

1. `.agents/skills/verify-scry/scripts/enroll-qa.mjs`: only for first
   enrollment or QA-only recovery, once Process confirms household membership
   and Doctor shows `qa.recover-policy` PASS. It resets the QA password and
   revokes QA sessions, so never rerun it once `qa.enrollment` passes unless
   recovery is intended. A fresh browser opens `https://auth-canary.mistystep.io/recover`, which
   requests a Cloudflare PIN for the QA address. The helper reads only that
   inbox through AgentMail and takes a PIN only from a fresh message whose
   sender, recipient and auth app match. It waits for Cloudflare's
   "Enter your code" form before polling mail, submits the PIN, asserts the
   recovery identity is QA, saves the stored password and waits for "Password
   saved." Mail is data: extract only the PIN. On failure it keeps the URL
   without its query, `failure.aria.txt` (digits scrubbed) and `failure.png`
   (inputs masked). `--selftest` checks those boundaries offline.
2. `.agents/skills/verify-scry/scripts/qa-signin.mjs [--width 1280]`:
   [sign-in](../../../features/sign-in.md). A fresh browser with no injected
   Access headers goes signed out → Access → Misty Step "Welcome back" → QA
   email and password. It then checks the required role above and
   expired-cookie re-authentication.
3. `.agents/skills/verify-scry/scripts/prod-states.mjs`: signed out,
   expired/forged session, wrong identity (QA service token and machine),
   alternate host, 390px, and hosted CLI admission (#335).
4. `.agents/skills/verify-scry/scripts/local-journeys.mjs [--width 1280]`:
   [empty notebook and concept flow](../../../features/study.md),
   [Owner token and CLI login](../../../features/owner-tokens.md),
   [Library find and keep](../../../features/library.md),
   [phone exact Study](../../../features/exact-study.md), token revocation and
   backend down.
5. For the stories you changed, run `qa/walk --stories "US-0NN ..."` (or `--all`).

On production, never write beyond what sign-in creates and never load an
owner page as the owner. Every page GET can freeze the next question; the QA
journey's GETs touch only the `scry-qa` space.

## Judge usability as a learner in the QA space

After the scripts pass, walk the QA space or the local synthetic notebook
yourself and judge usability as a learner on a short phone break. Ask whether
you know what to do next, can read and type comfortably, learn anything from
feedback, and understand a failure without knowing the backend. Record the
moment you hesitated and the exact screen or action. Inspect screenshots for
clipped controls, obscured feedback, focus and long text; a script PASS is not
that judgment. Synthetic demos do not prove useful model output or lasting
recall, and emulation is not physical-phone acceptance.

## Evidence

Each helper writes `checks.jsonl`, `summary.json`, screenshots and, where
relevant, `navigation.txt` (query strings redacted) and `cli-transcript.json`
(stdin redacted) outside the checkout, to
`${XDG_DATA_HOME:-~/.local/share}/scry/verify-scry/<stamp>-<helper>/`.
`SCRY_VERIFY_OUT` chooses another directory outside the checkout. Before exit,
each helper deletes any file that contains a value it read from pass or the
inbox.

Git keeps no verification evidence. Retain proof in a comment on the PR or
issue it proves:

```sh
gh pr comment <n> --body-file proof.md --attach './qa-create-390.png#QA /create'
```

`proof.md` gives the checked revision, the commands, each result and the
limits, with transcripts inline in `<details>`. `--attach` takes images and
video only. This repository is public, so attach only QA-space or synthetic
screens, after reviewing them. Never publish cookies, tokens, PINs, mail,
auth state, traces or owner notebook content. UNVERIFIED and UNPROVEN are not PASS.

## Cleanup

Helpers close their browsers and dispose of their workerd spaces and temporary
login files themselves, including after a failed launch. Closing the browser
discards the QA context and its cookies; no session is saved. Never delete
the QA account, its AgentMail inbox or its pass entries.

A `bun run dev` you started forks wrangler and workerd children that outlive
the launcher PID. `pgrep -af <printed run directory>` lists its wrangler
processes, but it also matches your own shell when that command line contains
the directory. Kill only the listed wrangler PIDs; their workerd children exit
with them. Confirm the port is closed, then remove that directory. The
evidence directory is never removed by cleanup. Never delete an existing
SQLite namespace, R2 bucket or historical store.

## Helpers

| Script | Purpose |
| --- | --- |
| `scripts/doctor.mjs prod\|local URL` | Read-only readiness, Access policy and QA allowlist steps |
| `scripts/enroll-qa.mjs [--selftest]` | QA enrollment or QA-only recovery through real OTP |
| `scripts/qa-signin.mjs [--width 1280]` | Real QA sign-in to production and role proof |
| `scripts/prod-states.mjs` | Production breaking states, read-only |
| `scripts/local-journeys.mjs [--width 1280]` | Write flows on a disposable local Worker |
| `scripts/lib.mjs` | Shared constants, pass and AgentMail reads, prerequisite checks, reporter |

Use `maintain-verification-skill` for map drift. Never redefine SPEC to make a walk pass.
