---
name: scry-qa
description: >
  Exercise the changed Scry surface against reality: kernel, API/UI,
  generation, dogfood clients, or production smoke. Use for QA, verification,
  smoke tests, or checking the app.
argument-hint: "[api|kernel|ui|generation|gate|prod-smoke]"
---

# Scry QA

Choose the surface that changed. Production is the Rust Wasm
`memory-engine-cloudflare` Worker, one SQLite-backed Scry Durable Object,
private R2 recovery, and Resend over Worker Fetch.
A green fixture or build proves only the machinery it exercises; live API/UI
and model-backed generation need their own runs.

| Changed area | Surface and proof |
|---|---|
| `crates/memory-engine-core/**`, `crates/memory-engine/**` | `cargo test -p memory-engine-core` / `-p memory-engine`; facade composes without private-crate imports |
| `crates/memory-engine-cloudflare/**` | Run the exact-bundle local Worker procedure below; live production proof is separate and requires explicit authority. Follow `docs/runbook.md` for release and recovery gates |
| `crates/memory-engine-api/**` | Run the native compatibility API and affected shared assets; verify production-facing behavior against the Worker |
| `crates/memory-engine-generation/**`, `-openrouter/**` | `cargo run -p memory-engine-bench -- generation`; live quality needs a dated `docs/evals/` receipt |
| `crates/memory-engine-web-shell/**`, `-cli`, `-import` | `cargo run -p memory-engine-web-shell -- --receipt`; inspect the JSON receipt |
| persistence, service, study crates | Targeted crate tests; Postgres paths run under `bun run ci:full` |

## Isolated production-runtime verification

Read [`docs/qa/system.md`](../../../docs/qa/system.md#cloudflare-runtime-proof)
for the existing executable oracle; do not substitute the native compatibility
server for the Worker. From the repository root, require Python 3, Node 22+,
Bun, Rust/rustup, and network access to the pinned package registries.
`bun run worker:tools` installs npm dependencies in this checkout, tools under
`target/cloudflare-tools`, and the pinned Wasm target in the user's rustup
installation. Reuse installed tools; obtain approval before changing a shared
toolchain. No Cloudflare login, `.env`, provider key, or production account is
needed. Do not source `.env` for ordinary verification.

Use a new run directory (never overwrite another run):

```sh
run_dir=$(mktemp -d "${TMPDIR:-/tmp}/scry-qa.XXXXXX")
bun run worker:build --out "$run_dir/bundle"
bun run worker:smoke --artifact "$run_dir/bundle" --receipt "$run_dir/workerd-proof.json"
bun run dev --artifact "$run_dir/bundle" --state "$run_dir/browser" --port 0
```

With harness tools, launch the final command through a supervised process
handle, not a detached shell. Wait for `local Worker http://127.0.0.1:<port>`
and inspect `/readyz` for `status: "ready"` before opening that exact origin.
Startup is bounded to 60 seconds. State must not already exist; an occupied
fixed port is a failure, not permission to reuse a different application.

At 390×844, use a fresh isolated browser profile (not the user's logged-in
browser), open `/app/account`, submit `dev@example.test`, and follow the
local-outbox link rendered by this instance. Confirm Library opens; replay
the consumed link in a separate fresh browser context and require rejection.
Keep tokens, links, cookies, and the private Worker log out of screenshots.
Inspect the actual mobile layout and reload the authenticated Library.
This establishes rendered local sign-in/session behavior, not inbox delivery.
Without an explicitly authorized provider file, browser AI capture cannot
establish successful generation: do not claim otherwise or enable a paid
provider to make the run green.

The smoke command separately exercises real workerd with its own disposable
accounts and structured NATO/ALFA source: alarm-driven publication, cross-account
403 rejection, reveal followed by restart and submission yielding
`revealed`/rating 1, and idempotent replay without new learning history.
It also exercises provider redirect rejection through a local HTTP boundary
fixture. Inspect the JSON receipt's `status`, `checks`, `revision`,
`source_sha256`, and `bundle_sha256` against this run's manifest; exit zero or
an old receipt alone is not proof. These assertions already reject plausible
identity leaks, assisted answers graded as correct recall, and duplicate
review writes. They do not establish external model quality.

Stop the dev process with Ctrl-C through its owning handle and close only the
browser contexts opened for this run. The helper stops its child process
group (10-second grace, then kill) but `--state` deliberately retains private
state and logs. Smoke removes its temporary state automatically and retains a
private diagnostic log beside the receipt on failure. Inspect/sanitize required
evidence before removing only the recorded `run_dir` with
`rm -rf -- "$run_dir"`; do not remove shared caches, `.env`, or other worktrees.
For a failed run, start with a new directory, not retained credentials/state.

## Skill discovery

The canonical entry point is `.agents/skills/scry-qa/SKILL.md`; existing
`.pi/settings.json` loads `.agents/skills/*`. For runners with ambient skills
disabled, explicitly read this file from the candidate checkout before
choosing proof. `AGENTS.md` and the verifier's gate instructions point here;
this requires only existing read authority, not additional tools or secrets.
Discovery proof is a fresh intended-runner session that locates this skill
and identifies the local Worker command, its no-provider boundary, and cleanup
without conversation-only setup. A file listing is not runner-loading proof.

## Native compatibility API (not production)

The native API needs a store, an allowlisted auth email, an outbox/mailer, and
`MEMORY_ENGINE_RETURN_UNSUBSCRIBE_SECRET`. The file-store path is local/dev
only:

```sh
MEMORY_ENGINE_ENVIRONMENT=development \
MEMORY_ENGINE_ENABLE_FILE_STORE=true \
MEMORY_ENGINE_API_STORE_DIR=.tmp/api-dev \
MEMORY_ENGINE_AUTH_ALLOWED_EMAILS=owner@example.com \
MEMORY_ENGINE_AUTH_LINK_OUTBOX_PATH=.tmp/api-dev/outbox.tsv \
MEMORY_ENGINE_AUTH_EXPOSE_DEBUG_LINKS=true \
MEMORY_ENGINE_RETURN_UNSUBSCRIBE_SECRET=local-dev-unsubscribe-secret \
HOST=127.0.0.1 PORT=18080 cargo run -p memory-engine-api
```

With the process running, check health, home, and the anonymous mutation
boundary:

```sh
curl -fsS http://127.0.0.1:18080/healthz
curl -fsS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18080/
# Expect anonymous rejection; curl -f exits nonzero on an HTTP error.
curl -fsS -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1:18080/app/generate
```

On this loopback instance, the explicit `development` environment above enables
the local-only account bootstrap; it does not need an operator admin token:

```sh
umask 077
curl -fsS http://127.0.0.1:18080/v1/accounts \
  -H 'content-type: application/json' \
  --data '{"email":"owner@example.com"}' \
  --output .tmp/api-dev/local-session.json
```

The private response contains `accountId` and `sessionToken`; do not print or
commit it. This anonymous bootstrap is disabled in production and staging;
those environments require operator-provisioned service sessions instead.
Then exercise source capture, queued `POST .../generation-jobs`, bounded polling of
`GET .../generation-jobs/{jobId}`, and review-next. Walk sign-in via the debug
link, source capture, generation, `/app/next`, reveal, and submit. The legacy
synchronous generate route returns HTTP 409 when Postgres is configured.

Generation without `OPENROUTER_API_KEY` uses structured-block parsing. Keep
ordinary verification credential-free; live model proof requires explicit
authorization for the key, model, source disclosure, and spend. The fixture
receipt cannot prove model quality.

## Gates and production

```sh
bun run ci
bun run ci:full
cargo run -p memory-engine-qa -- --local
cargo run -p memory-engine-qa -- --full
```

For a live model comparison, write one dated receipt and do not loop:

```sh
cargo run -p memory-engine-bench -- generation --model <m> --judge <m> --out docs/evals/<name>-$(date +%F).md
```

Production is `https://scry.misty-step.workers.dev`. The legacy `scry.study`
and `www.scry.study` origins proxy to that Worker; the native service is
disabled and its Postgres database is frozen recovery material. Check
`/healthz`, `/readyz`, and `/statusz` on the canonical origin and legacy ingress.
Only `release:traffic` may activate or pause the primary object; source,
immutable artifact, import, and recovery guards stay enforced. Do not restart
the native writer as a rollback after Worker writes.
Use explicitly approved QA identities, not the learner's account. Read
`docs/runbook.md` for real mail, session, migration, and monitoring proof.
Postgres compatibility contract tests remain part of `bun run ci:full`.

### Reproducible live proof

The isolated `bun run worker:smoke -- --artifact <bundle> --receipt <proof.json>`
command proves the exact bundle under local workerd, **not production**.
For live public health, run from outside Cloudflare and require HTTP success
plus healthy/ready JSON on every route:

```sh
for origin in https://scry.misty-step.workers.dev https://scry.study https://www.scry.study; do
  for route in healthz readyz statusz; do
    printf '%s/%s\n' "$origin" "$route"
    curl --fail --silent --show-error --max-time 20 "$origin/$route"
    printf '\n'
  done
done
```

For authenticated `/v1`, use an operator-provisioned disposable QA account with
an authorized public source and at least one published, due question. Put its
`baseUrl`, `accountId`, and `sessionToken` in the existing owner-only mode-0600
`credentials.json` format under a **separate QA home**. Do not use the learner's
home or put tokens in argv. The following real clients use that credential file;
the explicit environment removals prevent ambient credentials taking precedence:

```sh
qa_home=/private/scry-qa
env -u MEMORY_ENGINE_ACCOUNT_ID -u MEMORY_ENGINE_SESSION_TOKEN \
  MEMORY_ENGINE_HOME="$qa_home" \
  cargo run -p memory-engine-review -- review --max-cards 1

printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"scry-qa","version":"1"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"review_next","arguments":{}}}' |
  env -u MEMORY_ENGINE_ACCOUNT_ID -u MEMORY_ENGINE_SESSION_TOKEN \
    -u MEMORY_ENGINE_MCP_BASE_URL MEMORY_ENGINE_HOME="$qa_home" \
    cargo run -p memory-engine-mcp
```

Set the file's `baseUrl` to the canonical production origin. The CLI must
successfully call authenticated `POST /v1/accounts/{accountId}/review/next`
and `POST /v1/accounts/{accountId}/review/{reviewUnitId}/submit`;
answer the public QA question and inspect the resulting
grade. MCP must initialize and return an authenticated review-next result, not
an error. For legacy bearer/POST preservation, repeat the CLI against each
legacy origin using `--base-url https://scry.study` and then
`--base-url https://www.scry.study`, with another due QA question as needed.
Keep private receipts; revoke the disposable machine session after proof.

For live `/app/*`, open an actual browser at 390×844 or use a physical phone:

1. Request a fresh magic link for the approved QA mailbox; inspect real delivery,
   sign in from that message, and confirm the consumed-link replay is rejected.
2. Capture a word, phrase, or public passage and observe automatic publication
   into review without an approval action. Answer a cold question, choose Next,
   then “I don’t know yet”; verify Correct/Good and Revealed/Again respectively.
   Reload must hold the grade; inspect narrow-screen layout and deliberate Next.
3. Check missing/mismatched CSRF rejection. Use independently scoped cookie jars
   to prove logout-all revokes both browser sessions, not the machine session.
4. Receive a real due-count reminder, follow its signed unsubscribe link, confirm
   reminders off, and verify replay rejection. Keep URLs/tokens out of captures.

[`docs/qa/production-cutover-20260908.json`](../../../docs/qa/production-cutover-20260908.json)
records the executed cutover's revision/version, data continuity, recovery,
public health, browser/client/auth/mail, timing, and monitoring evidence; the
linked screenshots show actual phone-sized UI. It is a dated receipt, not a
substitute for exercising a newly changed surface. Do not infer physical-phone
retention, a long-term latency SLO, or platform PITR from that receipt.

## Report

Return `PASS`, `FAIL`, or `UNVERIFIED`; exact commands; surfaces exercised
(machinery, live API/UI, generation brain); artifacts inspected; uncovered
surfaces; and any Worker health or external-monitor signal. Canary is retired.
