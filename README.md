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
fixture, not a correctness assessment. Tests inject close/missed results;
SCRY_TEST_JUDGMENTS=close,missed,right hands out that verdict sequence in turn
(answer-independent) so the running app can show close/missed and “Why?”.
The test explainer sets the two answers side by side; it assesses nothing.
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
| SCRY_TEST_JUDGMENTS | Test mode only: comma-separated verdict script (right, close, missed) |
| SCRY_TEST_EXPLAIN | Test mode only: fail or slow exercises the explanation fallback |
| SCRY_TEST_CLOCK | Test mode only: start the app clock at an RFC 3339 instant (zone kept) |
| SCRY_TEST_CHARGE_DELAY | Test mode only: hold each fake top-up charge (e.g. 4s) |
| OPENROUTER_API_KEY | Live: the one AI key. When set it carries all four AI capabilities (Jev grading, Claude drafting, the “Why?” explainer, speech) and no ANTHROPIC_API_KEY or OPENAI_API_KEY is needed or read. A JWT-shaped value stops the boot. |
| TYPESAFE_API_KEY, ANTHROPIC_API_KEY, OPENAI_API_KEY | Live, optional direct fallbacks used only when OPENROUTER_API_KEY is unset: Jev on Typesafe, drafting and explanations on Anthropic, speech on OpenAI. Each missing one is named at boot; a JWT-shaped one stops the boot. |
| SCRY_DRAFT_MODEL | Live: drafting model. Default anthropic/claude-sonnet-5.5 on OpenRouter (must be a vendor/model slug there), claude-opus-5-5 on Anthropic |
| SCRY_EXPLAIN_MODEL | Live: model for on-demand “Why?” explanations. Default anthropic/claude-sonnet-5.5 on OpenRouter (vendor/model slug), claude-opus-5-5 with low effort on Anthropic |
| SCRY_SPEECH_MODEL | Live: transcription model. Default google/gemini-2.5-flash on OpenRouter (vendor/model slug; audio sent as input_audio), gpt-4o-mini-transcribe on OpenAI |
| LOG_LEVEL | debug enables successful health-check request logs |
| BACKUP_DIR | Empty disables automatic snapshots; image uses /backups |
| BACKUP_INTERVAL | 6h when backups enabled |
| BACKUP_KEEP | 14 snapshots (minimum effective retention 1) |
| ALERT_WEBHOOK_URL | Ignored in test mode; alerts go to JSON logs |
| SCRY_STRIPE_MODE | Optional: unset, live or test (anything else stops the boot). Unset follows SCRY_MODE. In live mode, test is the explicit staged-cutover switch to Stripe test keys (/healthz says billing_mode stripe-test and warns). In test mode, live is refused. |
| SCRY_JUDGE_MODE | Optional: unset, live or test (anything else stops the boot). In test mode, live grades with real Jev (OPENROUTER_API_KEY or TYPESAFE_API_KEY required, SCRY_JUDGE_MODEL honoured) while drafting, explanations, speech and billing stay fake. In live mode, test is refused. |
| SCRY_AI_MODE | Optional: unset, live or test (anything else stops the boot). In test mode, live wires all four real AI adapters (OPENROUTER_API_KEY, or every direct fallback key) while auth and billing follow their own rules; /healthz warns. In live mode, test is refused: live AI is never stubbed. SCRY_JUDGE_MODE=test with SCRY_AI_MODE=live is refused. |
| SCRY_AUTH_MODE | Optional: unset, live, test or clerk-test (anything else stops the boot). In live mode, test/clerk-test is the staged-cutover switch to a Clerk development instance (both Clerk keys must be test keys); live requires both keys to be live keys. In test mode, live is refused and test/clerk-test requires both Clerk test keys. Live mode on Clerk test keys always shows a /healthz warning, even with SCRY_AUTH_MODE unset. Clerk keys from different instances (one live, one test) never boot. |

Interfaces isolate auth, billing, model, speech, and alerts. Test adapters use
local bcrypt auth, fake cards, scripted judgments, simulated speech, and log-only
alerts. **No keys are needed or used.** Inactive provider adapters are included:
Clerk (CLERK_PUBLISHABLE_KEY and CLERK_SECRET_KEY), and the four AI capabilities on OPENROUTER_API_KEY
alone: Jev grading, Claude drafting and explanations (OpenRouter chat completions), and speech
(OpenRouter input_audio). Only without OPENROUTER_API_KEY do they fall back to direct keys: Jev on
TYPESAFE_API_KEY, Anthropic drafting and explanations on ANTHROPIC_API_KEY, OpenAI speech on
OPENAI_API_KEY. Model overrides: SCRY_JUDGE_MODEL, SCRY_DRAFT_MODEL, SCRY_EXPLAIN_MODEL,
SCRY_SPEECH_MODEL. Stripe (STRIPE_SECRET_KEY, STRIPE_PUBLISHABLE_KEY,
STRIPE_PRICE_ID, STRIPE_WEBHOOK_SECRET). Do not enable them for this delivery.
Stripe is mode-matched: a live adapter (stripe-live) accepts only sk_live_/rk_live_ and pk_live_ keys
and livemode:true webhooks; a test adapter (stripe-test) only sk_test_/rk_test_ and pk_test_ keys and
livemode:false webhooks. A key or webhook from the other mode stops the boot or is refused, loudly.
Live mode uses stripe-live unless SCRY_STRIPE_MODE=test. Never commit credentials.
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

GET /healthz verifies database writes and returns 200 or 503. It keeps
`"status":"ok"`/`"database":"ok"` (200) and `"status":"unhealthy"`/`"database":"failing"`
(503) for scry-lib.sh and scry-watch, and adds readiness: `build_sha` (set with
`-ldflags "-X main.buildSHA=<sha>"`, else the Go VCS stamp, else `dev`; `make build`
and `make docker` set it), `mode`, `db`, `grading`/`drafting`/`explainer`/`speech`
(true only when the real Jev, Claude and OpenAI adapters are wired), `billing_mode`
(stripe-live, stripe-test, test), `auth_mode` (clerk-live, clerk-test, local),
`providers` (the exact adapter label for each) and `warnings` (deliberate unusual
wirings such as the Stripe staged cutover). It never contains a key, secret, DSN
or token. Structured logs include request IDs, latency, status and user IDs
(local and Clerk auth alike), not submitted answers.
Startup checks integrity, marks interrupted drafts failed and releases allowance
reservations orphaned by an unclean stop (logged with their count).

Clerk session tokens are verified on every request (signature, expiry, issuer,
azp, sts). The signing key set is cached for 10 minutes and re-fetched early
when a token names an unknown kid (key rotation, at most once per 30 s); a
verified, active identity is cached for 60 s (at most 1024). Errors, banned or
locked accounts and unverified emails are never cached, so a ban is noticed
within 60 s. Background health checks run every 30 seconds and
renewals every minute; panics and provider/backup failures produce alerts.

**Docker health status alone does not restart unhealthy containers.** The
restart policy handles exits. Configure the deployment platform's liveness
probe to GET /healthz every 30 seconds (5-second timeout, restart after three
failures, 10-second startup allowance). Monitor the public endpoint outside the
host and route log alerts to an operator. Restarting fixes transient failures,
not corrupt disks or arbitrary bugs; use recovery procedures as needed.
No real monitoring service was contacted in this build.

Run `cmd/scry-watch` beside the service as the single health/restart owner.
On a host, `deploy/host/scry-watch.service` runs `/usr/local/bin/scry-watch` as
`scry` with the defaults SCRY_WATCH_STATE=/srv/scry/data/watch/incidents.json,
SCRY_WATCH_LISTEN=127.0.0.1:8091 and SCRY_WATCH_INTERVAL=10s (scry.env
overrides them). With no SCRY_WATCH_HEALTH_URL it probes
http://127.0.0.1:$PORT/healthz using scry.env's PORT (default 8080).
install.sh installs the unit and copies ALERT_WEBHOOK_URL into scry.env for
operator notifications; install the binary (`make build` writes bin/scry-watch) and `systemctl enable --now scry-watch`.
It logs JSON lines: one at start, one per change of health, one on a clean
SIGTERM stop; a probe cut short by its own shutdown never counts as an outage.
Disable the old watchdog cron and competing platform health restarters when
using `SCRY_WATCH_RESTART_CMD`. The legacy watchdog script remains tested for
deployments not using the incident watcher. See NOTES.md's MIS-1210 monitoring
runbook for configuration and the end-to-end drill.

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
| Operator story: continuous monitoring, incident triage, fix proposal and fault drill | `TestStory_ContinuousMonitoringFaultDrillRestartRecovery`, `TestMonitoringDedupTriageFixQuietDurability` (internal/watch) |
| Story: signup, trial and study within a minute | TestStory_Newcomer_SignUpStartTrialAndStudyWithinAMinute |
| Story: about 30 starter sets | TestStory_StarterShelf_EmptyLibraryPicksFromAboutThirtySetsAndStarts |
| Story: any topic shaped to learner | TestStory_AskForAnyTopic_AndGetItShapedToMe |
| Story: favorite book core ideas | TestStory_TurnAFavoriteBookIntoASetOfItsCoreIdeas |
| Story: agents add/suggest; one-tap approval | TestStory_Agents_SuggestToInboxOrAdd_ApprovedWithOneTap |
| Story: due items, typing/speaking, five minutes | TestStory_OpenScry_GetWhatsDue_AnswerByTypingOrSpeaking_DoneInAboutFiveMinutes |
| Story: model judging, reason, exact word diff | TestStory_NeverGradeMyself_EveryAnswerJudgedByTheModelWithAOneLineReason; TestStory_TextsIMarkExactAreJudgedWordForWord; TestStory_JevAbstainsOnceAndExactBypassesJev; internal/ai.TestDecisionCorrection_ConfigurableModelAndGradeOnly; internal/ai.TestJev_RoutesAndUntrustedInjection; internal/ai.TestJev_EachFixedChoiceAndRouteSelection; internal/ai.TestJev_FailsClosedWithoutRetries |
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

Unknown outcomes retain durable reservations, so a retry cannot spend unmetered.
A hold is held until it is settled (cost spent, once) or released (no cost), and
is swept otherwise: at boot (`FailStaleReservations`; nothing is in flight) and
by the hourly housekeeping once it is older than `app.ReservationTTL` (1 hour,
from `reservations.created_at`). A sweep never charges, and a late settle of a
swept hold is a no-op. `Reserve` checks access (trialing, active, past_due
within grace) and budget in one atomic statement; an amount ≤ 0 is refused.
The reservation table is part of private backups and selective recovery.
Older snapshots need whole restore/migration before selective recovery.

Optional local browser regression (requires Chromium and puppeteer-core):

    SCRY_BROWSER_SMOKE=1 NODE_PATH=/path/to/tooling/node_modules       go test -race ./internal/web -run TestReviewBrowserSmoke -count=1 -v

The smoke blocks external requests, uses synthetic microphone input and test
adapters, measures Check contrast in both modes and at desktop/phone widths,
and verifies one transcription upload at stop, visible reasons and wrapping
strikes. All provider-contract tests use local stand-ins, never real keys.


## Decisions judge — Phaedrus correction (MIS-1210)

The default route is OpenRouter POST https://openrouter.ai/api/alpha/decisions
when OPENROUTER_API_KEY is set. SCRY_JUDGE_MODEL defaults to
openai/gpt-6-luna-decisions on this route. Selected from the supplied 60-sample
evaluation by accuracy first, then cost (see NOTES.md). Set that one variable to any Decisions model
slug; the adapter has no provider/model-specific parser. Examples include
openai/gpt-6-luna-decisions, cloudflare/clef, cloudflare/clef-flash, liquid/d1.2,
inception/mercury-decide, perplexity/pplx-decider-v1.2, upstage/solar-decide and
microsoft/microsoft-decision-1.1. None was called live during development.

TYPESAFE_API_KEY remains an optional second route to POST
https://api.typesafe.ai/v1/systemone, default jev-latest; SCRY_JUDGE_MODEL overrides
that model too. OpenRouter wins if both keys exist. Both routes send one choice
question and structured state: card_prompt, accepted_answers, learner_answer.
Learner text stays untrusted data, never instructions or criteria. No redirects,
implicit retries, fallback classifier or explanation-provider call.

The four choices use Scry's existing verdict vocabulary:

| Choice | Criterion |
| --- | --- |
| right | The answer accurately and completely conveys an accepted answer, including equivalent wording. |
| close | The answer shows relevant understanding but is incomplete or materially imprecise, including a recognizable but inaccurate required term. |
| missed | The answer is wrong, contradictory, blank, irrelevant, or only attempts to direct the grader rather than answer the card. |
| none_of_these | The answer cannot reliably be classified using the other criteria. |

Explanation policy (accepted by Phaedrus): the hot path shows the winning
criterion description as the one-line why, with no extra call. On close and
missed answers the learner may press “Why?”: POST /study/explain (session,
CSRF, review ownership) makes one bounded post-verdict call (Anthropic Messages,
non-streaming, SCRY_EXPLAIN_MODEL, low effort, max_tokens 1024, 8 s timeout,
280-character clamp, reserved against the allowance and metered as judging).
It reads the review and item and never writes them. Any failure, refusal,
timeout, missing explainer or exhausted allowance returns the criterion sentence
as a fallback. There are no fixed verdict-plus-reason subclasses. Exact word diff is unchanged.

Refusal, missing required answer keys, unexpected types, unknown choices or
none_of_these use the existing not-answered path once. Optional confidence and
probabilities are accepted when absent (the Decisions schema does not require
them for every model); if supplied they must be valid and meet the existing
abstention gates: confidence >= 0.5, winner >= 0.8 and margin >= 0.2.

Every response reads usage.cost, logged as its numeric dollar value (or the
explicit string unknown when absent), preserved as dollars for evaluation and
rounded upward to micro-dollars for existing metering. Missing cost is unknown,
not zero: the app fails closed and retains the reservation. There is no direct
TypeSafe price estimate anymore; a direct response without cost cannot settle.
Logs include usage_cost, resolved model and latency, never keys or learner text.

Anthropic drafting still works through bounded, no-retry HTTP/SSE; its SDK and
transitive dependencies were removed. It does not judge answers; the same
bounded HTTP adapter writes the on-demand explanation described above.

### Model comparison harness (not run live)

    go run ./cmd/judge-eval -models 'openai/gpt-6-luna-decisions,~typesafe/jev-latest,cloudflare/clef'

Requires OPENROUTER_API_KEY. This command sends the sample and incurs provider
costs: run it intentionally. -data selects another JSONL file; model slugs can
also be supplied as positional arguments. One request per sample/model, no
retries. All tests inject a local stand-in; development did not execute this
command against a provider.

testdata/judge-eval.jsonl has 60 labeled variants of cards from three starter
sets, with source paths and accepted answers. Labels are correct/close/incorrect;
the harness maps right/close/missed to those labels. It includes correct answers,
partial answers, synonyms, inaccurate vocabulary spellings, wrong answers,
blanks and injection attempts. These are starter evaluation labels, not an
empirically validated benchmark; review borderline labels before comparing
production quality.

One JSON report per model includes accuracy over all samples, confusion counts
(actual -> predicted, including not_answered), total_usage_cost in dollars,
known_usage_cost, unknown_cost_calls, and nearest-rank p50_ms/p95_ms.
total_usage_cost is null if any call's cost is unknown, never silently free.

Contracts: [TypeSafe System One](https://docs.typesafe.ai/api/system-one),
[OpenRouter Decisions](https://openrouter.ai/docs/api/api-reference/alphadecisions/submit-a-decisions-request),
and [drafting SSE](https://platform.claude.com/docs/en/build-with-claude/streaming).

| Verifier design regressions D2/D3/D5/D7/D10 | TestVerifierDesignRegressions |
| Trial drafting refusal and truthful spendable allowance (F1) | TestF1_TrialDraftRefusalAgreesWithBilling |
| Truthful repeated top-up confirmation (F2) | TestF2_RepeatedTopupIsTruthful |
| Exact folding and near-miss diff | TestAcceptedFolding |
| Monthly top-up cap and rollover; concurrent dedup | TestAcceptedTopupMonthCap; TestF2_ConcurrentTopupsReportOneCredit |
| Top-up form token: cross-minute duplicate charges once; distinct forms up to the cap | TestF2_CrossMinuteDuplicateFormChargesOnce; TestF2_DistinctFormsWorkUpToTheMonthlyCap |
| PaymentIntent reconciliation after a lost reply or restart; decline release | TestF2_LostReplyIsReconciledWithExactlyOneCredit; TestF2_RestartMidChargeReconcilesCleanly; TestF2_PendingTopupIsShownThenSettled; TestF2_DeclineReleasesTheReservation; TestF2_StripeChargeKeyAndFindCharge |
| Late PaymentIntent success, incomplete lookups and a changed customer never drop a paid top-up; processing is pending, not declined; PaymentIntent webhooks settle top-ups once | TestF2_LateSuccessAfterLostReplyIsNeverDropped; TestF2_IncompletePaginationStaysRecoverable; TestF2_CustomerChangedWhilePendingStillCreditsTheOriginalCharge; TestF2_ProcessingChargeStaysPendingThenSettles; TestF2_TopupWebhooksOnlySettleTheirOwnAttempt; TestF2_StripeIncompleteScanIsNotAbsence; TestF2_StripeProcessingChargeIsPendingNotDeclined; TestF2_StripeFindChargeByIntentID; TestF2_StripeTopupWebhooks |
| requires_action / requires_confirmation are pending (kept by intent id, settled by lookup or webhook once); a known intent id is kept on failed attempts (402 card_error, list-discovered canceled / requires_payment_method); unconfirmed copy never claims nothing was charged | TestF2P3_NonFinalIntentStaysPendingThenSettles; TestF2P3_DeclineWithIntentKeepsChargeRef; TestF2P3_ListDiscoveredFailureKeepsChargeRef; TestF2P3_UnconfirmedCopyNeverClaimsNothingCharged; TestF2P3_IndeterminateErrorNamingIntentIsLookedUpByID; TestF2P3_FailTopupKeepsExistingChargeRef; TestF2P3_StripeNonFinalChargeIsPending; TestF2P3_StripeFindChargeNonFinalIsPending; TestF2P3_StripeErrorNamesIntent |
| signed PaymentIntent webhooks without metadata.scry_key settle the attempt by its saved charge_ref (pending or failed, credited once on delivery; failure/cancel releases; unrelated intents and wrong customers change nothing); a direct 200 canceled with no reason is released (ok=released) with no card fault | TestF2P4_IDOnlySucceededWebhookSettlesBySavedChargeRef; TestF2P4_IDOnlyFailureWebhooksSettlePendingBySavedChargeRef; TestF2P4_IDOnlyUnrelatedIntentWebhookChangesNothing; TestF2P4_IDOnlyWebhookWrongCustomerIsRefused; TestF2P4_DirectCanceledWithoutReasonReleasesCleanly; TestF2P4_DeclinesWithReasonStillReportTheReason; TestF2P4_StripeIDOnlyIntentWebhooksArePassedOn; TestF2P4_StripeDirectCanceledWithoutReasonIsNotADecline |
| No SQLite lock across the processor call (/healthz stays up) | TestD3_SlowChargeDoesNotBlockHealthOrTheDatabase |
| Plan eligibility, cap and payment details decided atomically in the reservation transaction | TestD3_PlanEndedWhileTopUpWaitsForTheWriteLockIsRefused |
| Ended plan with credit: Billing agrees with Learn; UTC month cap on any clock | TestD4_EndedPlanWithCreditBillingAgreesWithLearn; TestD4_NonUTCClockMonthCapAgreesWithTopUp |
| D2 voice: short, empty and silent recordings say "Didn't catch that" | TestD2_UncaughtRecordingsAreDidntCatchThat; TestD2_InspectFindsRecordingsWithNothingToHear; TestReviewBrowserSmoke |
| On-demand "Why?" explanation, guards and fallback | TestExplain_OnDemandAfterTheVerdictNeverChangesIt; TestExplain_GuardsAuthOwnershipCSRFAndKind; TestExplain_NotOfferedOnExactTexts; TestExplain_FallsBackToTheCriterionSentence; TestExplain_ClaudeRequestIsBoundedAndParsed; TestExplain_ExplainsTheGradedAnswerAfterTheCardIsEdited; TestExplain_LegacyReviewWithoutSnapshotFallsBack; TestReviewBrowserSmoke |

| Clerk sign-in, verified-email gate and built-in email reset | TestClerkUnconfiguredAndVerificationGate |
| Accepted decisions labels, single attempt and cost | TestJev_RoutesAndUntrustedInjection; TestDecisionUsageCostLogsValueOrUnknown |
| Monitoring reporting and operator-only drill | TestSentryErrors; TestOperatorFaultDrill |
| Verified Clerk identity and browser session | TestClerkVerifiedEmail; TestClerkSessionStandIn; TestClerkMissingKeysNoNetwork |
| Concurrent monthly cap | TestAcceptedTopupConcurrentCap |
| D1 after-stop voice text | TestG12_SpeechUploadsOnlyAtStop; TestReviewBrowserSmoke |
| D6 intrinsic missed-pair stacking | TestReviewBrowserSmoke (1280/900/600/390 geometry and screenshots) |
| D8 reading-page measure, row exemptions | TestD8_ReadingMeasure |
| D9 newest-right why lifetime | TestReviewBrowserSmoke (newest-right, next answer, Next, screenshots) |
| D11 navigation/study keys and Tab/Enter elsewhere | TestReviewBrowserSmoke; TestD11_KeyHints |
| D9 right-to-right and right-to-missed lifetime | TestD9_NewestRightReasonLifetime |
| Slice 12: orphaned allowance reservations released at boot (all users; empty table; Reserve/Settle unchanged) | internal/store.TestFailStaleReservationsReleasesOrphansAcrossUsers; internal/store.TestFailStaleReservationsEmptyTable; internal/store.TestReserveSettleUnchangedBySweep |
| Slice 13: reservation state machine (table; rapid on a real SQLite file: reserve/settle/replay/TTL sweep/crash+restart/period reset/top-up/status) | internal/store.TestReservationTransitionTable; internal/store.TestRapidReservationStateMachine |
| Slice 13: Reserve refuses ended and grace-expired plans atomically (TOCTOU), non-positive amounts; TTL sweep; created_at migration | internal/store.TestReserveGuardFollowsHasAccess; internal/store.TestReserveRefusedWhenAccessEndsAfterPlanRead; internal/store.TestReserveRacingPlanEnd; internal/store.TestReserveRejectsNonPositiveAmount; internal/store.TestSweepExpiredReservations; internal/store.TestReservationsCreatedAtMigration; internal/app.TestAppReserveFollowsAccess; internal/app.TestSweepReservationsReleasesExpiredHolds |
| Slice 13: Stripe adapter on the PaymentIntent state machine; failed invoices carry their period, stale failures ignored | internal/billing.TestStripeIntentMappingMatchesGolden; internal/billing.TestStripeFailedInvoiceCarriesItsPeriod; internal/app.TestStripeStaleInvoiceFailureIsIgnored |
| Slice 12: Clerk key set and identity cached within TTL, key rotation refresh, failures never cached, token always verified, bounded, race-safe | internal/auth.TestClerkCachesKeySetAndIdentityWithinTTL; internal/auth.TestClerkKeyRotationRefreshesKeySet; internal/auth.TestClerkNeverCachesFailures; internal/auth.TestClerkAlwaysVerifiesTokenEvenWhenCached; internal/auth.TestClerkIdentityCacheIsBounded; internal/auth.TestClerkCacheConcurrent; internal/auth.TestClerkStructLiteralUsesCachedSDKClients |
| Slice 12: Clerk request logs carry the user id | TestClerkRequestLogsCarryUserID |
| Slice 12: mode-matched Stripe keys and webhooks; SCRY_STRIPE_MODE / SCRY_JUDGE_MODE fail loud | internal/billing.TestStripeModeMatchedKeys; internal/billing.TestStripeWebhookLivemodeMatchesAdapter; cmd/scry.TestStripeModeSelection; cmd/scry.TestJudgeModeSelection |
| Slice 20: all four AI capabilities on OPENROUTER_API_KEY alone; direct fallbacks only without it; JWT-shaped keys refused loudly; SCRY_AI_MODE / SCRY_AUTH_MODE fail closed; Clerk test keys in live mode warned in /healthz | cmd/scry.TestSlice20_LiveOnOpenRouterAloneWithStagedCutover; cmd/scry.TestSlice20_ClerkTestWarningInLiveMode; cmd/scry.TestSlice20_AuthModeSelection; cmd/scry.TestSlice20_AIModeSelection; cmd/scry.TestSlice20_LiveDirectFallbacksAndKeyChecks; cmd/scry.TestLiveRequiresEveryCredential; internal/app.TestSlice20_ReadinessLabelsBothRoutes |
| Slice 20: OpenRouter drafting stream (keepalives, chunked JSONL, [DONE], usage.cost, refusal, truncation, redaction, no redirects) and explainer | internal/ai.TestSlice20_OpenRouterDraftStream; internal/ai.TestSlice20_OpenRouterDraftCostNeverInferred; internal/ai.TestSlice20_OpenRouterDraftFailures; internal/ai.TestSlice20_OpenRouterDraftStopsAtDone; internal/ai.TestSlice20_OpenRouterNoRedirectAndRedaction; internal/ai.TestSlice20_OpenRouterExplain; internal/ai.TestSlice20_OpenRouterExplainClampsAndFails; internal/ai.TestSlice20_NewClaudeWithRouterPrecedence; internal/ai.TestSlice20_DirectExplainUnchanged |
| Slice 20: OpenRouter speech (input_audio, MIME formats, empty audio, redaction, bounded reads, no redirects), JWT-shaped key rejection, NewTranscriber precedence | internal/speech.TestSlice20_OpenRouterTranscribeRequest; internal/speech.TestSlice20_OpenRouterAudioFormat; internal/speech.TestSlice20_EmptyAudioIsNothingHeard; internal/speech.TestSlice20_OpenRouterTranscriptCleanup; internal/speech.TestSlice20_OpenRouterTranscribeFailures; internal/speech.TestSlice20_JWTShapedKeys; internal/speech.TestSlice20_TranscribeRefusesJWTShapedKeys; internal/speech.TestSlice20_NewTranscriberPrecedence; internal/speech.TestSlice20_OpenAIErrorRedactedAndBounded |
| Slice 12: /healthz build SHA, mode and provider readiness, no secrets; 503 on database failure | cmd/scry.TestHealthzAcrossModes; cmd/scry.TestHealthzDatabaseFailure; cmd/scry.TestBuildSHA; TestHealthzReadinessFields |
| Slice 12: scry-watch never counts its own shutdown as an outage | internal/watch.TestCanceledProbeIsNotAnOutage |
