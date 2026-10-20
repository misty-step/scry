---
name: verify-scry
description: Verify Scry end to end as its owner from a shell. Sign in to https://scry.mistystep.io as phaedrus@mistystep.io, check owner identity configuration, probe signed-out, expired, wrong-identity and alternate-host states, and drive the notebook, Owner token/CLI, Library and phone exact-Study flows on a disposable local Worker. Use it after any change to verify user-visible behavior or sign-in.
---

# Verify Scry

Start with [the feature map](../../../features/README.md) and the SPEC stories
it names. It is the navigation inventory for verification and fitness, not
another acceptance contract. Claude Code reads this file through
`.claude/skills -> ../.agents/skills`; Codex reads it here.

## Credentials

Read by pass entry name; never print or commit values. The helpers read the
first line of `pass show` themselves. `pass-env run -e` keeps the stored
trailing newline, and Access then treats service tokens as absent.

| Use | Entry |
| --- | --- |
| Access apps, IdPs, users, logs (read) | `workstation/CLOUDFLARE_API_TOKEN` |
| Worker settings and deployments (read) | `workstation/SCRY_CLOUDFLARE_DEPLOY_TOKEN` |
| Isolated QA identity (own `scry-qa` space) | `misty-step/SCRY_QA_ACCESS_CLIENT_{ID,SECRET}` |
| Machine health `/healthz` | `misty-step/SCRY_ACCESS_CLIENT_{ID,SECRET}` |
| Owner sign-in (**not provisioned**) | `workstation/MISTY_STEP_OWNER_PASSWORD` (`SCRY_OWNER_PASSWORD_ENTRY`) |

## Launch

Production is `scry-rust-production`, behind the Access app "Scry production
owner". Read the [runbook](../../../docs/runbook.md) for the dated serving
source; it can lag master. Production needs no launch and is read-only.

Local write flows need the built artifacts once per source change:

```sh
export PATH="$HOME/.cargo/bin:$HOME/.local/share/node-22.22.0/bin:$HOME/.local/bin:$PATH"
npm ci --ignore-scripts --no-audit --no-fund
worker-build --release --locked
cargo build --locked --bin scry
```

`local-journeys.mjs` starts its own workerd SQLite/R2 space on a free loopback
port with no egress and stops it when it finishes. Hermes workers run builds
and browsers on Mirrodin under `/srv/hermes-chief-workspaces/<task-id>`. For manual browsing,
`bun run dev` ([scry-qa](../scry-qa/SKILL.md)) prints a run-owned loopback URL;
an empty notebook offers "Explore an authored demo".

## Doctor

```sh
.agents/skills/verify-scry/scripts/doctor.mjs prod
.agents/skills/verify-scry/scripts/doctor.mjs local http://127.0.0.1:8787
```

`prod` confirms Worker vars, Access team = `ACCESS_TEAM`, app AUD =
`ACCESS_AUDIENCE`, the "Misty Step password" IdP, an exact-email policy for
phaedrus@mistystep.io, his Access `user_uuid` (the JWT `sub`), anonymous
redirect and machine health. `owner.subject` is always UNPROVEN, because
`OWNER_SUBJECT` cannot be read back; only `owner-signin.mjs` proves admission.
Exit 0 all PASS, 1 any FAIL, 3 only UNPROVEN. Run doctor first, and again after
anything surprising.

## Drive

Run in order. Each helper prints one line per check and exits 0/1/3 like Doctor.

1. `.agents/skills/verify-scry/scripts/owner-signin.mjs [--width 1280]`:
   [sign-in](../../../features/sign-in.md). Signed out → Access → Misty Step
   "Welcome back" → (password entry) → owner-only `/create`, then expired-cookie
   re-authentication.
2. `.agents/skills/verify-scry/scripts/prod-states.mjs`: signed out,
   expired/forged session, wrong identity (QA and machine), alternate host,
   390px, and hosted CLI admission (#335).
3. `.agents/skills/verify-scry/scripts/local-journeys.mjs [--width 1280]`:
   [empty notebook and concept flow](../../../features/study.md),
   [Owner token and CLI login](../../../features/owner-tokens.md),
   [Library find and keep](../../../features/library.md),
   [phone exact Study](../../../features/exact-study.md), token revocation and
   backend down.
4. For the stories you changed, run `qa/walk --stories "US-0NN ..."` (or `--all`).

Never write to production beyond what sign-in creates. Every owner page GET can
freeze the next question, so the owner journey reads only `/create`.

## Walk as Phaedrus

After the scripts pass, judge as a learner on a short phone break. Ask whether
you know what to do next, can read and type comfortably, learn anything from
feedback, and understand a failure without knowing the backend. Record the
moment you hesitated and the exact screen or action. Inspect screenshots for
clipped controls, obscured feedback, focus and long text; a script PASS is not that judgment.
Synthetic demos do not prove useful model output or lasting recall, and
emulation is not physical-phone acceptance.

## Evidence

Each helper writes `checks.jsonl`, `summary.json`, screenshots and, where
relevant, `navigation.txt` (query strings redacted) and `cli-transcript.json`
(stdin redacted) to `target/verify-scry/<stamp>-<helper>/`. Set
`SCRY_VERIFY_OUT` to choose a directory. To retain a run, copy the reviewed
files to `docs/qa/evidence/verify-scry/<date>/` with source SHA, target,
data mode and limits in its README. Never commit cookies, tokens, auth state,
traces, email contents or personal notebook content; owner screenshots stay on `/create`.
UNVERIFIED and UNPROVEN are not PASS.

## Cleanup

Helpers close their browsers and dispose of their workerd spaces and temporary
login files themselves, including on failure. A `bun run dev` you started
forks wrangler and workerd children that outlive the launcher PID. Stop exactly
those children with `pgrep -af <printed run directory>` and `kill <PIDs>`, then remove
that directory. `target/verify-scry/` is evidence and is never removed by
cleanup. Never delete an existing SQLite namespace, R2 bucket or historical store.

## Helpers

| Script | Purpose |
| --- | --- |
| `scripts/doctor.mjs prod\|local URL` | Read-only readiness and owner identity configuration |
| `scripts/owner-signin.mjs` | Real owner sign-in to production |
| `scripts/prod-states.mjs` | Production breaking states, read-only |
| `scripts/local-journeys.mjs` | Write flows on a disposable local Worker |
| `scripts/lib.mjs` | Shared constants, pass reads, reporter and Playwright launch |

Use `maintain-verification-skill` for map drift; never redefine SPEC to make a walk pass.
