# Scry

One Go service: private libraries, model-judged study, exact texts, and agent suggestions.
Server-rendered HTML, self-hosted Geist fonts, SQLite, no frontend build step.

## Run and test

Use Go 1.27.4 (go.mod supports automatic toolchain installation).

    make run                  # test mode, seeded demo, default PORT=8080
    make run PORT=18080        # choose a free port
    make test                 # vet and tests
    go build ./...
    go vet ./...
    go test -count=1 ./...     # uncached verification
    make build                # static bin/scry

Demo: **phaedrus@example.com / know-thyself**, an ordinary account, not an admin.
Seeding is idempotent, not a reset; use a fresh SCRY_DB for a fresh demo.
Never expose the known-password demo to untrusted users or enter real personal
data. On a remote sandbox use its port-sharing facility to obtain a browser URL.

Sign up with email and a password of at least eight characters. For the card step,
use only test card 4242 4242 4242 4242, a future expiry, and CVC 123.
4000 0000 0000 0002 demonstrates a decline. No payment occurs.

**Test-mode limitations:** non-exact answers receive a labelled scripted “right”
fixture, not a correctness assessment. Tests inject close/missed results.
Exact texts use the real word diff. Drafts use built-in examples or labelled
placeholders, not general knowledge generation. Speech is simulated, though
the browser can request microphone permission and send validated PCM WAV audio
to this service once recording stops (no repeated live transcription).
SCRY_TEST_SPEECH supplies a reproducible phrase for voice demos.

## Environment and adapters

| Variable | Default / purpose |
| --- | --- |
| PORT | 8080; binds all interfaces |
| SCRY_MODE | Bare binary defaults to live and fails without credentials. **Use test for this delivery.** make run and image set test. |
| SCRY_DB | data/scry.db; make run uses data/demo.db unless overridden |
| SCRY_SEED_DEMO | 1 seeds demo, only in test mode |
| SCRY_BASE_URL | External HTTPS origin; forces Secure cookies |
| SCRY_TEST_SPEECH | Optional fixed test transcription |
| LOG_LEVEL | debug enables successful health-check request logs |
| BACKUP_DIR | Empty disables automatic snapshots; image uses /backups |
| BACKUP_INTERVAL | 6h when backups enabled |
| BACKUP_KEEP | 14 snapshots (minimum effective retention 1) |
| ALERT_WEBHOOK_URL | Ignored in test mode; alerts go to JSON logs |

Interfaces isolate auth, billing, model, speech, and alerts. Test adapters use
local bcrypt auth, fake cards, scripted judgments, simulated speech, and log-only
alerts. **No keys are needed or used.** Inactive provider adapters are included:
Identity Toolkit (IDENTITY_API_KEY), Jev (OPENROUTER_API_KEY or TYPESAFE_API_KEY),
Anthropic drafting only (ANTHROPIC_API_KEY; optional SCRY_DRAFT_MODEL), speech (OPENAI_API_KEY; optional
SCRY_SPEECH_MODEL), Stripe (STRIPE_SECRET_KEY, STRIPE_PUBLISHABLE_KEY,
STRIPE_PRICE_ID, STRIPE_WEBHOOK_SECRET). Do not enable them for this delivery.
Stripe rejects non-test secret/publishable keys. Never commit credentials.
Provider quality and actual integrations are not proved by test fixtures.

## Deploy

    make docker
    docker volume create scry-data
    docker volume create scry-backups
    docker run -d --name scry --restart unless-stopped \
      -p 8080:8080 -e PORT=8080 -e SCRY_MODE=test \
      -v scry-data:/data -v scry-backups:/backups scry
    docker exec scry /scry healthcheck
    docker logs scry

The multi-stage scratch image has a static binary, CA certificates, embedded
assets and timezone data, no shell, and UID/GID 65532. /data and /backups are
persistent volumes. Named volumes inherit image ownership. For bind mounts,
pre-create directories owned by 65532 with mode 0700. Use one replica and local
persistent storage, not an unsuitable network filesystem. Terminate TLS at a
trusted reverse proxy, set SCRY_BASE_URL, restrict backend access, and never
expose database files, snapshots or tokens.

GET /healthz verifies database writes and returns 200 or 503. Structured logs
include request IDs, latency, status and user IDs, not submitted answers.
Startup checks integrity. Background health checks run every 30 seconds and
renewals every minute; panics and provider/backup failures produce alerts.

**Docker health status alone does not restart unhealthy containers.** The
restart policy handles exits. Configure the deployment platform's liveness
probe to GET /healthz every 30 seconds (5-second timeout, restart after three
failures, 10-second startup allowance). Monitor the public endpoint outside the
host and route log alerts to an operator. Restarting fixes transient failures,
not corrupt disks or arbitrary bugs; use recovery procedures as needed.
No real monitoring service was contacted in this build.

For a plain Docker host without orchestrator liveness, install the included
deploy/watchdog.sh in a root-owned directory and run it every minute from the
Docker operator's cron (absolute path, container name as argument). It restarts
only running unhealthy containers, leaves healthy/starting/manually stopped
containers alone, and emits an error if health information is unavailable.
Capture cron output in host monitoring. Example crontab entry:

    * * * * * /opt/scry/deploy/watchdog.sh scry

Access to Docker is privileged; do not grant it to untrusted users. The watchdog
is covered by TestRule_Reliability_WatchdogRestartsOnlyUnhealthyContainers
in cmd/scry; platform liveness is preferable when available.

## Durability, privacy and backup

SQLite uses WAL, FULL synchronous writes, foreign keys, and transactions.
Learner queries are scoped by user ID. Cookies are HttpOnly/SameSite; mutations
require CSRF. Agent tokens are hashed and default to suggestions-only; direct
addition is an explicit permission. Data is private between accounts, **not
end-to-end encrypted**: host administrators and backup holders can read it.
Encrypt volumes and off-host backups and restrict operator access.

Automatic VACUUM INTO snapshots run on startup and every BACKUP_INTERVAL,
retaining BACKUP_KEEP copies. Manual backup:

    SCRY_DB=data/demo.db bin/scry backup backups
    docker exec scry /scry backup /backups

Copy completed snapshots to separate encrypted storage. A second directory on
the same disk is not disaster recovery. Default recovery-point objective is up
to six hours plus off-host-copy delay. Never copy just the live .db file while
WAL writes are active.

Whole recovery: stop the service, preserve the damaged database and its WAL/SHM
siblings, install a verified snapshot at SCRY_DB with correct ownership and
restrictive permissions, ensure no stale WAL/SHM remain at that path, then
restart and verify health, login, library and history. Rehearse on disposable
storage first. Whole snapshots include accounts, local password hashes and tokens.

Selective recovery is for the **same database lineage**. First snapshot the
destination and stop traffic/background work, then:

    SCRY_DB=data/demo.db bin/scry restore-user backups/scry-TIMESTAMP.db learner@example.com

This replaces owned rows transactionally, leaves other users untouched, and
rejects conflicting account identities. It does not restore account profile,
local passwords, or browser sessions; use whole recovery for these. Billing and
agent-token rows are restored: reconcile billing and revoke restored tokens
before reopening access. It is not an import between unrelated databases.
Settings also offers JSON export (without token hashes) and 30-day undo for
removed ideas; export is not a whole-database backup.

## Agents and keyboard

Settings creates/revokes agent tokens. Send Authorization: Bearer TOKEN:
GET /api/v1/ideas lists private ideas; POST /api/v1/suggestions queues approval;
POST /api/v1/items requires “may add”. Body example:

    {"source":"Reading notes","items":[{"question":"What is inversion?","answer":"Reason backward from failure","idea":"Mental models"}]}

Tab/Enter operate controls. Outside fields: g s study, g l library, g i inbox,
g n learn, g b billing; / focuses search. Inbox: A approve, D dismiss,
J/K or arrows move. Study: Enter checks/continues, Space on an empty answer
starts voice and another Space stops it, A repeats exact text. Settings has
system/light/dark modes; reduced motion disables transitions.

## Contract → automated proof

Each row maps one PRODUCT.md story or rule to existing tests in internal/web.
These are deterministic integration proofs, not external model quality or uptime
guarantees. Unattended restart requires the deployment liveness policy above.

| Contract | Test |
| --- | --- |
| Story: signup, trial and study within a minute | TestStory_Newcomer_SignUpStartTrialAndStudyWithinAMinute |
| Story: about 30 starter sets | TestStory_StarterShelf_EmptyLibraryPicksFromAboutThirtySetsAndStarts |
| Story: any topic shaped to learner | TestStory_AskForAnyTopic_AndGetItShapedToMe |
| Story: favorite book core ideas | TestStory_TurnAFavoriteBookIntoASetOfItsCoreIdeas |
| Story: agents add/suggest; one-tap approval | TestStory_Agents_SuggestToInboxOrAdd_ApprovedWithOneTap |
| Story: due items, typing/speaking, five minutes | TestStory_OpenScry_GetWhatsDue_AnswerByTypingOrSpeaking_DoneInAboutFiveMinutes |
| Story: model judging, reason, exact word diff | TestStory_NeverGradeMyself_EveryAnswerJudgedByTheModelWithAOneLineReason; TestStory_TextsIMarkExactAreJudgedWordForWord; TestStory_JevAbstainsOnceAndExactBypassesJev; internal/ai.TestJev_RoutesAndUntrustedInjection; internal/ai.TestJev_EachFixedChoiceAndRouteSelection; internal/ai.TestJev_FailsClosedWithoutRetries |
| Story: connected ideas; each item belongs to an idea | TestStory_SeeHowIdeasConnect_AndEachItemBelongsToAnIdea |
| Story: private library/history | TestStory_MyLibraryAndHistoryAreMineAlone |
| Story: clear price/allowance, top-up/cancel | TestStory_PayingUser_KnowsThePriceAndAllowance_CanTopUpOrCancelAnyTime |
| Rule: no free tier, 7-day trial, card up front | TestRule_NoFreeTier_SevenDayTrialWithACardUpFront |
| Rule: small base, capped cost allowance, top-ups | TestRule_Prices_SmallBaseCappedAllowanceAndTopUpsBeyondIt |
| Rule: Phaedrus first with no special powers | TestRule_PhaedrusIsTheFirstCustomerWithNoSpecialPowers |
| Rule: outages noticed/fixed without prompting | TestRule_Reliability_HealthzStructuredLogsAndAlertsWithoutBeingAsked |

Additional regression tests cover snapshot/per-user recovery, conflicting restore identities, test-mode alert isolation, scripted judgments, and design details.


## Review-remediation verification and allowance safety

Provider cost is now reserved atomically before calls; insufficient unreserved
budget stops work even if a small positive balance remains. Requests are bounded
(judge: 8,192 input bytes / 1,024 output tokens; draft: 32,000 / 16,000).
Conservative reservations include framing margin and the highest configured
rates; actual usage settlement refunds unused budget. Automatic model retries
and fallback are disabled. Review the price table before changing model pricing.

Unknown outcomes retain durable reservations. Reconcile them against provider
usage before releasing holds; do not blindly delete reservations after a crash.
The reservation table is part of private backups and selective recovery.
Older snapshots need whole restore/migration before selective recovery.

Optional local browser regression (requires Chromium and puppeteer-core):

    SCRY_BROWSER_SMOKE=1 NODE_PATH=/path/to/tooling/node_modules       go test -race ./internal/web -run TestReviewBrowserSmoke -count=1 -v

The smoke blocks external requests, uses synthetic microphone input and test
adapters, measures Check contrast in both modes and at desktop/phone widths,
and verifies one transcription upload at stop, visible reasons and wrapping
strikes. All provider-contract tests use local stand-ins, never real keys.


## Jev judge (MIS-1210 ruling)

Non-exact study answers use Jev's typed decision API, not a generative judge.
OPENROUTER_API_KEY selects POST https://openrouter.ai/api/alpha/decisions with
model ~typesafe/jev-latest; otherwise TYPESAFE_API_KEY selects POST
https://api.typesafe.ai/v1/systemone with model jev-latest. OpenRouter wins when
both keys are configured. No automatic provider failover or retries. Both routes
use one choice question with criteria and a JSON state containing the question,
reference and untrusted learner answer. SCRY_JUDGE_MODEL is no longer used.
ANTHROPIC_API_KEY remains necessary only for the separate drafting provider.
Test mode stays entirely local regardless of configured keys.

The small judgeOptions table in internal/ai/jev.go supplies both verdicts and
fixed user-facing descriptions. Kaylee recommended this design; Phaedrus has
not yet approved the wording. No generated prose or fabricated close-word spans.
An unknown/none choice, low concentration or invalid/provider output returns
the existing unavailable-judge path without a review or schedule change.
Acceptance requires confidence >= 0.5, selected probability >= 0.8 and a
runner-up margin >= 0.2, a valid normalized distribution and a recognized choice.
These are provisional abstention gates, not answer-matching heuristics.

Every attempt logs resolved model (unavailable if unknown), latency and cost
(null if unknown), never learner state, raw provider bodies or credentials.
OpenRouter cost comes from usage.cost; missing cost fails closed. Direct Jev
1.13/1.14 cost is estimated from reported input tokens at $0.042/M, with free
outputs; unknown direct model versions fail closed pending a price review.
Charges round upward to micro-dollars. Existing conservative reservations
remain: known usage settles even on abstention, unknown cost retains the hold.
Exact-text word diff and folding are unchanged.

Contracts: [TypeSafe System One](https://docs.typesafe.ai/api/system-one),
[Choice](https://docs.typesafe.ai/primitives/choice),
[pricing](https://docs.typesafe.ai/models), and
[OpenRouter Decisions](https://openrouter.ai/docs/api/api-reference/alphadecisions/submit-a-decisions-request).
Local tests verify wire behavior and isolation, not live Jev accuracy or
resistance to semantic prompt injection in the real model.
