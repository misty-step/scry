# NOTES

## Handoff (2026-11-17, MIS-1210)

The first build lane (Claude Code, Fable 5.2) stopped on a usage limit at commit 9d2437a plus `internal/web/rules_test.go`, now committed. You continue on the same branch. The contract is `PRODUCT.md` (what Scry does) and `DESIGN.md` (how it looks and feels), and `AGENTS.md` holds the build rules. Those three files and this note are your only sources.

### Done

- One Go service in `cmd/scry` with packages under `internal/`: `srs` (scheduling), `exact` (word-for-word judging of exact texts), `starter` (the starter shelf), `store` (SQLite), `ai`, `speech`, `auth`, `billing` and `alert` (adapters with test modes), `app` (logic) and `web` (server-rendered HTML).
- Every story and business rule in PRODUCT.md has a named test (`TestStory_*` and `TestRule_*`).
- `go build ./...`, `go vet ./...` and a fresh `go test -count=1 ./...` all pass at handoff.
- A `Makefile` with `run`, `test`, `build`, `vet` and `docker` targets.

### First-lane decisions (trace handoff supplied by the user)

- Go 1.27 with toolchain go1.27.4: the original box had 1.23.4, so GOTOOLCHAIN=auto installs the selected toolchain. Pure-Go modernc SQLite keeps the binary static, without cgo.
- The model adapter uses Anthropic Go SDK v2: structured JSON judgments and streaming drafts (the continuation now disables automatic fallback/retries to bound call cost). The continuation replaced the fake's keyword matcher with scripted fixtures; exact texts alone use the word diff.
- Geist Sans and Geist Mono variable fonts were obtained from the npm geist package and are self-hosted with their OFL license.
- The 30 starter JSON sets (742 items, about 24 per set) were completed by three first-lane helpers. Set 30 is passages, learned line by line. Validation limits questions to 170 characters, answers to 140, and requires valid cross-links. They are complete and were not rewritten.
- SeedDemo is idempotent: an ordinary demo account with over 100 items, at least 10 due, seven cards its API tokens added (straight to the library since S106-4; they were seven Inbox suggestions before), 62% allowance used and an established trial/plan history. TestRule_SeededDemoData pins the fixture.
- Story/rule tests use the real HTTP handlers through a browser-like harness and test clock.
- Sessions slide on use. Billing has a fake and a Stripe adapter; Stripe remains test-only.

### First-lane dead ends (do not repeat)

- Cross-links to passages-worth-keeping failed before set 30 existed; they now validate.
- Missing template FuncMap helpers panic at startup (billing used amount); register helpers before adding templates.
- A restart left a stale server on :8080; one prior restart exited 144. Identify and stop the old process before reusing the port.
- The first lane used puppeteer-core for state screenshots under /tmp/shots. Its visual correction was start-alignment of the idea diagram; rerunning all screenshots is optional.
- rules_test.go was written at the usage-limit stall, subsequently committed as 4826372, and is now green.

### Left (original checklist; completed below)

1. A `Dockerfile` for one small image that serves on `PORT`, with a non-root user and the SQLite file on a volume. `make docker` must build it.
2. `README.md`: how to run, test and deploy it, how durability and backup work, and a story-to-test table with one row per PRODUCT.md story and business rule naming the test that proves it.
3. Prove `make run` works: it starts in test mode with seeded demo data, `/healthz` answers 200, and the sign-up and study pages render. Record what you ran and saw at the end of this note.
4. Below this handoff, add an `## Ambiguities` section listing every place PRODUCT.md or DESIGN.md left a choice open, and the choice you made.
5. Fix anything you find that misses PRODUCT.md or DESIGN.md, and add a test for each fix.

### How to verify

```sh
go build ./... && go vet ./... && go test -count=1 ./...
make run &   # then:
curl -fsS localhost:8080/healthz
curl -fsS localhost:8080/ | head
make docker
```

Done means all of these pass, the README table covers every story and rule, and this note records the results.


## Ambiguities

These are the implementation choices made where the contract left behavior,
numbers, operations, or conflicting design details open. No external Scry
implementation was consulted.

### Product, learning and content
- “About 30” means exactly 30 starter sets; scope and wording are the existing
  curated JSON shelf (742 items), including line-by-line passages. They are
  general educational prompts, not personalized professional advice.
- “About five minutes” is a session budget, not a timer: 20 seconds per ordinary
  item, 30 per exact line; additional due items wait for another session.
- Scheduling uses the existing FSRS-4.5-style implementation, 90% retention,
  right/close/missed grades, minimum one day and maximum ten years. Misses
  return tomorrow; exact “Again” is practice, not another scheduled review.
  Due-today uses the learner's timezone (browser-detected, otherwise UTC).
- Exact means ordered words, ignoring case, surrounding punctuation and curly
  apostrophe spelling; missing/swapped/extra words are counted. Any word difference is missed; only a zero-difference result is right (G4/D2).
- General answers are judged only through the model interface. With providers
  deliberately disabled, test mode returns an explicit scripted fixture
  independent of the answer; it cannot assess actual correctness. Tests inject
  close/missed fixtures. Model failures preserve the answer rather than guessing.
- Topic/book generation uses a draft outline, with each item or all approvable;
  no unapproved draft enters the library. Personalization uses the learner's
  About text and known ideas. In test mode book/topic fixtures and clearly
  labelled placeholders exercise the workflow, not arbitrary knowledge quality.
- Agent integration is a private bearer-token JSON API, not conversation
  scraping. Suggestions are default; direct add requires explicit token scope.
  Sources are learner/agent-supplied text; no link/image/video ingestion.
- Phaedrus is the first seeded ordinary customer, with no privileged role.
  Repeated seeding leaves existing data untouched.

### Accounts, money and operations
- Password minimum is eight characters; browser sessions last 30 days and renew
  after half their lifetime. Test authentication uses bcrypt locally; provider
  adapters are inactive. No password-reset or email-verification UX was specified.
- Prices are provisional USD defaults: $5/month includes $3 model cost, seven-day
  trial includes $1, $5 top-up adds $4 cost allowance. These values reserve room
  for hosting/payment overhead; “small margin” has no numerical contract.
  They are not a verified commercial cost model. Allowance is cost-based, not a
  fixed question count; metering reserves usage before provider calls.
- Billing renews on the subscription period, not necessarily the first of a
  calendar month. Top-ups carry forward; cancellation ends access at the stated
  trial/period end. Failed cards have a seven-day grace/retry window. No free tier.
- Outside services remain behind interfaces and test adapters for all execution
  here. Configuring ALERT_WEBHOOK_URL no longer bypasses test mode. Stripe JS is
  emitted only for a nonempty provider key, which the fake never exposes.
- Deployment is one static Go process, one local SQLite database, non-root
  scratch image, persistent /data and /backups, PORT configurable. The default
  container is test-only without demo seeding; make run seeds a demo.
- Privacy is application isolation and restricted storage, not end-to-end
  encryption against the operator. Tokens are hashed, mutations use CSRF.
  Whole snapshots contain sensitive account data and must be encrypted off-host.
- Automatic backup is optional on bare-metal (BACKUP_DIR), on by default in the
  image: every six hours, 14 retained. Whole recovery restores all accounts;
  selective recovery replaces owned data only within the same database lineage
  and now rejects identity collisions before deleting data. It does not restore
  passwords/profile/sessions; billing and token rollback require reconciliation.
  Removed ideas have a 30-day undo; JSON export is not a database backup.
- Reliability means integrity checks, write-probing health, JSON logs, alerting,
  process restart and platform liveness supervision. Docker HEALTHCHECK alone
  cannot restart a hung container; deploy/watchdog.sh supplies tested host-side
  recovery for unhealthy containers, and README specifies the required platform
  liveness policy and external monitoring. Arbitrary bugs/disk failures cannot
  be promised self-repair. Test mode logs alerts without contacting operators.

### Design and interaction
- Geist/Geist Mono are embedded and self-hosted. Fluid type, exact color tokens,
  square surfaces, 4px controls, 1120px frame and phone stacks follow DESIGN.
  System theme is default; explicit theme persists per learner.
- The single round mic is the explicit exception to the no-pill/square rules.
  Inbox approval uses accent outlines, not many filled primary buttons.
  Focused inbox rows use raised; diagram links and item-row links now have
  44px minimum targets. Tab/Enter provide keys for actions without shortcuts.
- The global phone layout breakpoint is 760px; phone type/section tokens switch
  at 640px. Decorative upcoming text is ghost; required reading uses body/meta.
  Non-keyboard test-mode notice uses meta, not micro.
- Scrollback shows at most five desktop/two phone entries on one visible line;
  right-answer reasons remain visible in body text (G3 overrides the earlier hiding decision). Up next is capped at three/one.
- Space starts voice only from an empty answer (or stops a recording), preserving
  normal spaces in typed answers. Recording uses PCM samples with a 20-second browser cap, then one upload at stop.
  The server counts samples (up to 120 seconds); there is no repeated live
  transcription or looping visual animation (G12 supersedes the earlier choice).
- Draft streaming is progressively revealed by short polling; without JS,
  server-rendered forms still work but voice/automatic polling do not.
  Checking appears after 400ms, not a spinner. Motion is 150–250ms and reduced
  motion suppresses transitions. Exact judgments remain in-place rather than
  using the large generic-miss treatment.
- One primary action is contextual: starter shelf changes after first add,
  approval/cancellation have plain-word feedback, and session end has no charts
  or game mechanics. Unspecified error states use plain-language inline messages.

## Continuation changes

- Added static multi-stage Dockerfile, non-root volumes, healthcheck and minimal
  build context; added README run/deploy/env/backup instructions and all 14
  story/business-rule rows with existing test names.
- Removed heuristic judging even from the fake; tests now explicitly script
  misses. Added regression proving fixtures are answer-independent.
- Isolated test-mode alerts from webhook configuration and corrected the
  healthcheck comment (Docker does not automatically restart unhealthy tasks).
- Corrected short touch targets, focused inbox surface, visible multi-line
  scrollback, and non-keyboard micro text, with regression tests.
- Rejected conflicting identities during selective recovery, with a regression
  test proving another learner remains unchanged.

- Added a tested host watchdog for plain Docker deployments: restart unhealthy
  containers only; preserve explicitly stopped containers and report unknown state.

### Final verification (2026-11-17)

- The first default-port attempt correctly failed because the first lane's
  /tmp/scry-run/scry (PID 47785) still owned :8080. First proved a detached
  SCRY_DB=/tmp/scry-verification/demo.db make run PORT=18080 on fresh storage:
  /healthz, /signup, /signin, authenticated /study, /library, /inbox, /billing,
  /settings and /shelf all returned 200. The demo account was seeded successfully.
- Identified and terminated only that stale first-lane server; then detached
  make run on the default :8080. /healthz returned 200 with database=ok,
  mode=test, status=ok; sign-up rendered 200; demo sign-in redirected to study,
  which rendered 200 with the current study form. Logs identify every adapter
  as test. The default-port process remains available in the sandbox.
- README audit: exactly 10 story rows and 4 business-rule rows; every named
  test exists (15 names in the table, plus the watchdog test in deployment docs).
- Final isolation review traced HTTP usage to the inactive auth, AI, billing,
  speech and webhook adapters. Test wiring chooses local/fake adapters only;
  ALERT_WEBHOOK_URL is ignored and regression-tested. Stripe's fake exposes no
  publishable key, so its external script is not emitted. Browser speech posts
  only to this service. No real credentials or outside service calls were used.
- Docker is available. make docker builds the static scratch image successfully.
  Runtime checks used PORT=18082, UID/GID 65532, /data and /backups volumes:
  healthcheck exited zero, HTTP health returned 200/test, demo login/study
  rendered, and a restart preserved the account and library. Automatic and
  manual snapshots were exercised. Host watchdog behavior is covered for
  healthy, starting, stopped, unhealthy and unknown states with a fake Docker CLI.
- This smoke test exposed a same-second manual/startup snapshot collision.
  Fixed it with unique snapshot filenames and private 0600 files; the new
  TestRule_Durability_BackupsInSameClockTickDoNotCollide passes. Failed partial
  snapshots are removed. Filenames now include subsecond time plus a unique
  suffix; pruning still uses the scry-*.db pattern.
- No new visual screenshot pass was required; the user's first-lane screenshot
  review is retained, and continuation design changes have regression tests.
- No product sources were fetched or browsed. Only the supplied contract,
  handoff, and local code were used; Docker fetched standard build images and
  Go module dependencies. No existing Scry repository was consulted.
- After the snapshot fix, reran go build ./..., go vet ./..., and fresh
  go test -count=1 ./...: all green. Rebuilt with make docker: successful.
  Recreated the container from the final image, manually backed up and restarted
  immediately: both snapshots succeeded, health remained 200/test, and logs had
  no ERROR events. Temporary verification container and anonymous volumes were
  removed after the checks. Runtime filesystem is approximately 35 MB.
- All original Left items are complete. README and this note document the
  operational limits honestly: external-model quality and real provider uptime
  are not tested; actual unattended host recovery requires installing the
  provided watchdog or configuring platform liveness and monitoring.

## Initial review remediation (2026-11-18; superseded by completed continuation below)

The review supersedes the earlier acceptance/completion statement. This build
must not be treated as accepted until the remaining requested gaps are fixed.

- G3: right-answer reasons are visible body text again. Inverted the design
  regression; observed it fail before removing the screen-reader-only class.
- G4/D2: exact verdicts are right for zero word differences and missed otherwise.
  Changed the regression first and observed its failure against the old threshold.
  Updated the HTTP exact-text story expectations to match; the word diff remains.
- At this initial stopping point the other gaps were outstanding; all requested
  remainder items are now addressed in the completed continuation below.
- The review correctly found that the previous reservation claim was false.
  G2 now implements actual durable atomic reservations, with bounded requests
  and transactional settlement. G4 removed the old exact-text close threshold.

## Open spec questions

All questions reserved for Phaedrus are answered; none is open. (Updated
2026-11-20; the earlier list here was stale.)
- Judge: RESOLVED by Kaylee on MIS-1210: Jev / Decisions API (below).
- Authentication: RESOLVED in PRODUCT.md: Clerk, email and password, verified email.
- Top-up limits: RESOLVED in PRODUCT.md: $5 each, up to $20 per person per UTC month.
- Password reset and verification: RESOLVED in PRODUCT.md (Clerk email flows).
- Monitoring owner: RESOLVED; see "2026-11-19 — MIS-1210 monitoring incident owner".
- Exact-text folding: RESOLVED in PRODUCT.md: ignore case, punctuation and
  apostrophes, then word for word.
- One-line explanation: RESOLVED by Phaedrus (2026-11-20 ruling): (a) on the hot
  path, the winning criterion sentence, no extra call; plus (b) only on demand,
  a "Why?" button on close/missed answers making one bounded post-verdict LLM
  call that never changes the grade. Implemented in Slice #3 below.

## Completed review remediation (2026-11-18)

All remaining items from the supplied fix brief are implemented. The five
Open spec questions above remain undecided. No existing Scry source was sought,
fetched or used. Adapter requests in verification went only to local stand-ins;
all keys in those tests are synthetic strings, not real credentials.

Each gap had a red regression before the corresponding implementation fix;
the provider-coverage bundle uncovered three SDK requests for one judge call,
and the browser pass found additional contrast/recording failures before they
were corrected. Subsequent commits retain the required model trailer.

| Gap | Fix and regression proof |
| --- | --- |
| G1 | Both operation keys and confirmed payment references are deduplicated. Ledger insertion, operation marker and credit increment commit together; retry after a local failure remains safe. TestG1_TopUpCreditsOncePerPayment first reproduced 12,000,000 credit from one 4,000,000-credit payment; TestG1_OnePaymentReferenceCannotCreditTwice also rejects a replay under a different operation key. The existing per-minute idempotency window is unchanged; this is not a new top-up limit. (Superseded 2026-11-20: the per-minute key is replaced by a per-form operation token; see Slice #3.) |
| G2 | SQLite reservations atomically require enough unreserved allowance BEFORE judge, draft or speech calls. Actual usage and reservation removal settle in one transaction. TestG2_ConcurrentDraftsCannotSpendLastMicro first admitted 25 calls, now none. TestG2_ReservationHeldUntilProviderCompletes covers overlapping requests and refund to actual cost. TestG2_UnknownProviderOutcomeKeepsReservation covers ambiguous errors. |
| G3 | Previously fixed: right reasons visible in body text. Browser smoke now verifies actual dimensions, not merely template presence. |
| G4/D2 | Previously fixed: exact is right only for zero differences, missed otherwise; no tuned close threshold. Diff and difference counts remain visible. |
| G5 | Webhook subscription/ledger effects and processed marker share one transaction. A failed write rolls back the marker too. TestG5_FailedEventCanBeRedelivered injects a SQLite failure then redelivers, and checks duplicate replay adds no extra ledger entry. |
| G6 | Removed checkbox-label class from Check. Removed accent opacity/background transitions that transiently lowered contrast. TestG6_CheckUsesAccentForeground plus real Chromium measurements: 5.2501:1 light, 5.8932:1 dark, at both desktop and phone widths. |
| G7 | Native text-decoration strikes wrap with the answer, no duplicate overlay. TestG7_MissedStrikeFollowsText plus desktop/phone screenshots and computed-style assertions. The old overlay draw animation is intentionally superseded by the brief's native decoration requirement. |
| G8 | All four provider adapters redact their configured key at the error boundary, before application logs/alerts. TestG8_ProviderKeyNeverLeavesAdapter first demonstrated a transport error containing the key; it now returns a redacted error. |
| G10/D3 | Empty non-exact answers follow the same one-call judge path. TestG10_EmptyAnswerCallsJudgeOnce. No local empty-answer verdict remains. |
| G11 | Local tests exercise signed Stripe events (valid, tampered, missing, stale, future, live-mode refusal), retired auth adapter request/error mapping, Anthropic structured judgment parsing/request shape/streaming draft bounds, and speech multipart upload. TestG11_* tests use no real service. SDK automatic retries disabled: one invocation sends one request. Installed SDK bearer-auth header is asserted. |
| G12 | Browser captures mono PCM WAV to a silent audio sink, uploads once at stop. Server validates RIFF/PCM framing, sample rate/alignment/length, counts samples, ignores client duration, rejects unsupported/malformed audio. TestG12_SpeechDurationComesFromAudio first billed 1 micro instead of 50 for one second; now 50. TestG12_ValidatedSampleDuration tests malformed/truncated headers. TestG12_SpeechUploadsOnlyAtStop and the browser's real Web Audio/fake-device flow prove no uploads while listening and one at stop. |
| G14 | Removed inline-link height exemption. TestG14_InlineLinksKeepTouchTargets plus browser measurements of visible links on library, inbox, billing and settings. |
| G15 | Plan, trial, cancellation and ledger dates all use learner timezone, without mutating stored timestamps. TestG15_AllBillingDatesUseLearnerTimezone crosses a UTC/local midnight boundary. |
| G16 | Cancel, card changes and failure messages mutate a freshly read row under one write transaction; top-up uses an atomic increment; events already run transactionally; opening a plan preserves current credit and refuses to replace an active plan. TestG16_CancelCannotOverwriteConcurrentUsage injects metering during the processor call and proves usage survives. |
| G17 | Long answers are cut at a UTF-8 rune boundary, still within 2,000 bytes. TestG17_LongAnswerNeverSplitsRune first passed invalid UTF-8 to the spy judge, now preserves valid text. |
| G19 | go mod tidy declares the three direct imports correctly and resolves dependency checksums. TestG19_DirectDependenciesAreDeclaredDirect failed on all three former indirect declarations. |

### Allowance and recording operational details

- Model requests have explicit input-byte/output-token ceilings: judge
  8,192/1,024; drafts 32,000/16,000. Reservations use the highest configured
  price rates, plus 1,024 input tokens of framing margin: 114,688 micro-dollars
  for a judgment and 904,192 for a draft. This intentionally refuses a request
  when remaining allowance cannot cover its worst case, even if it is not zero.
  Unused budget is released when actual usage is known.
- Automatic SDK retries and draft fallback are disabled, so they cannot multiply
  an authorized request's cost. The largest known input/output prices bound
  configured models; the rate table still needs explicit maintenance before a
  future price/model change. No claim of live-provider pricing verification.
- Uncertain provider outcomes (lost responses, panic, process death) retain
  their reservation instead of silently forgiving potentially incurred costs.
  Known actual usage is charged without clamping away excess. Operators must
  reconcile held reservations against provider records before releasing them;
  there is no automatic expiry or invented monitoring agent. Pre-provider
  draft creation/profile failures release the unused reservation.
- Reservations are durable, per-user, and included in snapshots/export/selective
  recovery. Snapshots predating this table need whole restore/migration before
  selective restore; never restore over running provider work.
- The new audio path does not provide live transcription during listening:
  the fix brief's one-upload-at-stop requirement supersedes that earlier design
  implementation. The listening state remains; heard text appears before Check.
  Browser cap is 20 seconds; server accepts validated PCM up to 120 seconds and
  6 MiB, reserving at most 6,000 micro-dollars. Unsupported codecs fail closed.
  Test-only say: fixtures remain restricted to the test speech adapter.
- Chromium smoke found that a speakerless host could stall the recording graph.
  A silent sink fixes that without a real microphone/provider. Unsupported audio
  environments still allow typing.

### Reproducible browser smoke

The optional TestReviewBrowserSmoke starts the real HTTP app with scripted
judgments and a fake speech provider, then drives Chromium via puppeteer-core.
It exercises signup, test-card trial, shelf, typed and spoken study, right/missed
judgments, library, inbox, billing and settings. Network interception allows only
that local test server. Chrome's synthetic microphone drives actual Web Audio,
not a stubbed upload. No production-only test hooks were added.

    SCRY_BROWSER_SMOKE=1 NODE_PATH=/tmp/shots/node_modules       go test -race ./internal/web -run TestReviewBrowserSmoke -count=1 -v

For another environment install puppeteer-core in a tooling directory, set
NODE_PATH accordingly and optionally CHROME_BIN. Default Go tests skip this
opt-in browser test. Screenshots are in /tmp/scry-remediation-smoke; inspected the
phone/light multi-line miss to verify the strike follows each wrapped line.

### Final verification of completed remediation

- go build ./...: PASS.
- go vet ./...: PASS.
- go test -race -count=1 ./...: PASS, with fresh execution (the opt-in browser
  smoke is skipped by this command and run separately).
- SCRY_BROWSER_SMOKE=1 NODE_PATH=/tmp/shots/node_modules go test -race
  ./internal/web -run TestReviewBrowserSmoke -count=1 -v: PASS. No external
  browser requests or JavaScript errors. Checks at 1280 and 390 pixels in both
  themes; measured button contrast 5.2501 and 5.8932, 44px targets, no horizontal
  overflow, native multiline strike, visible right reason, single audio upload
  only at stop. Also checked sign-up/card/shelf and authenticated main pages.
- make run PORT=18086 with fresh SCRY_DB=/tmp/scry-review-final/demo.db:
  detached successfully in test mode; /healthz was 200/ok/test, /signup was
  200, demo login redirected to an authenticated /study with the form rendered.
- make docker: PASS on the available Docker daemon, rebuilding the static
  non-root image with the updated code, embedded JS/CSS and tidied modules.
- Raw verification logs: /tmp/scry-remediation-final-race.log,
  /tmp/scry-remediation-final-browser.log, /tmp/scry-remediation-final-docker.log.
  Red regression logs are /tmp/G*-red (including G1-ref-red and G2-unknown-red).
- No remaining requested recompiler misses. The unresolved owner questions stay
  in Open spec questions; real-provider integration, actual model quality and
  production monitoring ownership have not been asserted or silently decided.

## Initial Jev judge swap (MIS-1210; SUPERSEDED by Phaedrus correction below)

Scope: judging only. Auth, top-up policy, password reset/verification,
monitoring ownership and exact folding remain OPEN and were not changed.
Previous references to an Anthropic study judge in historical notes are
superseded by this section.

### Contract and architecture

Read the authorized public TypeSafe llms.txt index, System One/Choice/confidence
and model documentation, plus OpenRouter's official Decisions API schema.
References:
- https://docs.typesafe.ai/llms.txt
- https://docs.typesafe.ai/api/system-one
- https://docs.typesafe.ai/primitives/choice
- https://docs.typesafe.ai/confidence
- https://docs.typesafe.ai/models
- https://openrouter.ai/docs/api/api-reference/alphadecisions/submit-a-decisions-request

Both current schemas use named questions with type=choice, instructions and
criteria, and answers with type, choice, probabilities and confidence. No
options field and no chat/messages endpoint. State is JSON data; learner text
cannot modify the instructions or criteria structure.

The existing application Model is satisfied by ai.Split: Jev implements the
judge side; Claude implements only drafting. Removed all Haiku judge code,
configuration and pricing entries. The Anthropic SDK is deliberately retained
because the existing streaming drafter still uses it; go mod tidy was run.
No new generative reason-writing call exists. Test mode still uses scripted
fixtures; no keys were read for live requests and tests never contact providers.

OpenRouter key takes precedence when both env keys exist; otherwise direct
TypeSafe is used. The aliases and endpoints are exactly those in the ruling.
No redirect following, provider failover, SDK retries or second question.

### Provisional option table (Open spec question: awaiting Phaedrus)

The single source of truth is judgeOptions in internal/ai/jev.go. Criteria
descriptions and user-facing reasons are identical strings.

| Choice | Verdict | Fixed description |
| --- | --- | --- |
| correct | right | Your answer conveys the reference answer's meaning. |
| correct_minor_slip | right | Your answer conveys the right meaning despite a minor spelling or wording slip. |
| partially_correct_missing_key_part | close | Your answer has the core idea but is missing a key part. |
| partially_correct_inaccurate_detail | close | Your answer has the core idea but includes an inaccurate detail. |
| incorrect_wrong_concept | missed | Your answer gives a different or contradictory concept. |
| incorrect_blank_or_irrelevant | missed | Your answer is blank or does not address the question. |
| none_of_these | unavailable; no grade | None of these descriptions reliably fits your answer. |

none_of_these never creates a review; the existing failure copy is shown instead.
Jev cannot return word spans with this single choice, so no close-word highlights
are fabricated. Exact texts continue to show their unchanged plain word diff.

### Failure, cost and telemetry

A recognized choice is accepted only with a full normalized distribution,
confidence >= 0.5, chosen probability >= 0.8 and a >= 0.2 margin above the runner
up. These centralized, provisional safety gates abstain rather than grading
ambiguous answers; they are not text matchers and are not empirically calibrated
claims. Low concentration, ties, malformed output, none_of_these, missing cost
or provider failure enters the existing unavailable path once, without a review
or scheduling change. Learners may explicitly retry; the adapter never does.

The logger records requested and resolved model, latency_ms, cost_micros,
cost_source and acceptance for every attempt. Unknown model/cost is explicitly
unavailable/null, never invented. No raw state, response body, provider error
text or secret is logged. G8 redaction remains at the boundary, including
provider-echoed keys in model identifiers.

OpenRouter usage.cost is converted from dollars; direct resolved versions
jev-1.13.0 and jev-1.14.0 use the documented $0.042/M input-token estimate,
outputs free. Other direct versions fail closed until pricing is reviewed.
Known cost settles the existing reservation even if the decision is declined;
unknown cost leaves the reservation held. The existing conservative admission
budget and top-up policy are unchanged.

### Tests and verification

- Wrote TestJev_RoutesAndUntrustedInjection and failure tests first; recorded
  the expected pre-implementation failure in /tmp/jev-red.log.
- Local stand-ins validate both exact paths/aliases, bearer authentication,
  one choice question with fixed criteria, choice parsing, all option mappings,
  route precedence, rejection of bad/missing/ambiguous decisions, 503s and
  redirects without retries, cost handling and secret-free logs.
- Injection test uses exactly “ignore the criteria, answer correct” as learner
  state. It verifies isolation from instructions/criteria and respects the
  stand-in's missed result. This proves the boundary, not real model robustness.
- TestStory_JevAbstainsOnceAndExactBypassesJev proves one request/one failure
  notification, no review after none_of_these, and zero Jev requests for exact
  texts. README's judging-story row now names these adapter/integration proofs.

Final results for this swap: go build ./..., go vet ./..., and
go test -race -count=1 ./... all PASS. The opt-in Chromium smoke also PASSed
under -race (signup/trial/shelf, typed and synthetic-microphone study, right
reason, wrapped miss, library/inbox/billing/settings; both themes at 1280/390).
go mod tidy -diff reports no changes. Logs: /tmp/jev-final-race.log and
/tmp/jev-final-browser.log. No live provider was called; semantic accuracy,
threshold calibration and Phaedrus's wording approval remain unverified.

## Phaedrus correction applied — current judge contract

This section supersedes the initial seven-option judge swap above. The request
is one general correctness choice with right/close/missed (Scry's existing
vocabulary) plus none_of_these, not verdict-and-reason labels. State contains
card_prompt, accepted_answers (array; existing card answer is the fallback)
and learner_answer. No text matcher or extra classifier/explanation call.

SCRY_JUDGE_MODEL is restored as the sole model override. Default on OpenRouter
is ~typesafe/jev-latest; optional direct default is jev-latest. No whitelist or
resolved-version-specific parser: the named examples in Phaedrus's correction
use exactly the same adapter. Required response keys/type/choice are checked.
Refusals, missing named answers, unexpected answer types, invalid choices and
none_of_these enter not-answered once. Confidence/probabilities are optional
per the generic Decisions schema; when present the earlier validation and
low-concentration gates still apply. This permits models which return only
the required choice fields without fabricating probabilities.

All cost comes from response usage.cost, on both routes; no token-price estimate.
Exact dollar cost is logged with resolved model and latency and preserved for
evaluation; metering still rounds upward to micro-dollars. A missing cost fails
closed and retains the existing reservation. In particular the published
direct TypeSafe schema does not guarantee cost: deployments whose direct
responses omit it must use OpenRouter for metered judging. No key or raw
provider body reaches logs. No live calls were made to test this.

Explanation alternatives were recorded neutrally in Open spec questions and are
now resolved (Slice #3): the criterion description is the hot-path why, and an
on-demand "Why?" makes the only explanation call. See README for the four criteria.
Auth, top-up limits, reset/verification, monitoring and exact folding remain OPEN
and unchanged.

### SDK removal and preserved drafting

Removed the Anthropic SDK entirely (including transitive dependencies via
go mod tidy). Drafting retains its existing provider/model, prompt, JSON-lines
outline behavior, input/output caps and no-retry policy using net/http and a
bounded SSE reader. The local stand-in verifies both rejection and streaming
head/item delivery with token accounting. No generative judging was reintroduced.

### Offline sample and comparison command

Added testdata/judge-eval.jsonl: 60 variants from Kitchen numbers, NATO alphabet,
and Mental models, labeled correct/close/incorrect with explicit source paths.
Includes partial/synonym/vocabulary-typo close examples, correct alternatives,
wrong answers, blanks and the injection attempt. Labels are an initial,
reviewable evaluation set, not a claim that a real model was benchmarked.

cmd/judge-eval takes comma-separated -models or positional model slugs and
optional -data. It sends each sample once per model using the same Decisions
adapter. Prints per-model JSON: accuracy, confusion counts, total exact
usage.cost (null if incomplete), known cost and unknown-cost count, and
nearest-rank p50/p95 latency. Not-answered counts in the accuracy denominator.
There is no live evaluation output: only httptest-server metrics tests ran.

### Regression-first evidence and verification

The new configurable-model/cost test failed before Model/CostUSD existed
(/tmp/correction-red.log); the SDK removal test failed on the old dependency
(/tmp/sdk-red.log); evaluation tests failed before the harness existed
(/tmp/eval-red.log). Added cases for refusal, missing answer key and unexpected
type; retained injection, both route shapes, exact bypass, no retry, redaction
and ordinary failure-path tests. Harness tests cover model iteration, exact cost
sum, accuracy/confusion/latency, refusals and missing-cost accounting locally.

Final correction results: go build ./..., go vet ./... and
go test -race -count=1 ./... all PASS. The separate opt-in Chromium smoke under
-race also PASSed at desktop/phone sizes in both themes, including synthetic
microphone recording. go mod tidy -diff is clean; go.mod/go.sum contain no
Anthropic SDK. testdata/judge-eval.jsonl has exactly 60 rows. Logs are
/tmp/correction-race.log and /tmp/correction-browser.log. No comparison was run
against any network provider; no live model accuracy or robustness is claimed.


## Supplied live judge evaluation and default update

**Evaluation date:** 2026-11-17 PT. **Dataset:** testdata/judge-eval.jsonl,
**60 samples per evaluated model**. Results were supplied by the owner in
/home/user/judge-eval-results.json; this follow-up did not run live requests.
The JSON reports requested slugs; resolved identifiers below are from the
owner's accompanying report, not inferred from aliases.

| Requested model | Accuracy | Total usage.cost / 60 (USD) | p50 ms | p95 ms | Notes |
| --- | ---: | ---: | ---: | ---: | --- |
| ~typesafe/jev-latest | 0.833 | 0.001305444 | 228.701 | 278.190 | Resolved typesafe/jev-1.14-20261014 |
| openai/gpt-6-luna-decisions | 0.900 | 0.001744800 | 192.322 | 380.339 | Resolved version suffix -20261006, as supplied |
| cloudflare/clef | 0.750 | 0.004296096 | 291.863 | 768.259 | |
| cloudflare/clef-flash | 0.100 | 0.000508003 | 310.815 | 895.439 | Mostly not_answered (54/60) |
| liquid/d1.2 | 0.700 | 0.000273900 | 310.888 | 667.579 | All 15 close samples were not_answered |
| inception/mercury-decide | — | — | — | — | Not listed among OpenRouter Decisions models; not run |

The selection rule is **accuracy first, then cost**. Luna achieved the highest
sample accuracy (54/60), so openai/gpt-6-luna-decisions is now the default for the
OpenRouter route. It is not the cheapest model and has higher p95 latency than
Jev; the decision follows the requested priority, not a universal performance
claim. This small labeled sample does not establish general production accuracy.

SCRY_JUDGE_MODEL remains the sole override. The optional TypeSafe direct route
still defaults to jev-latest. The explanation policy, exact folding and all
other open spec questions are unchanged.

Fixed the judge decision log's usage_cost field: dereference a known cost to a
numeric scalar; use the string unknown when nil. This avoids pointer addresses
with slog's text handler and makes absence explicit in both text and JSON.
No key/state logging was added.

Regression-first proof:
- Updated TestJev_RoutesAndUntrustedInjection to expect the new OpenRouter
  default, observed it fail against the previous default, then changed routing.
  Direct-route and explicit-model-override tests still pass.
- TestDecisionUsageCostLogsValueOrUnknown reproduced the pointer address and
  nil representations before the fix; now verifies numeric/unknown values for
  both text and JSON handlers, also checking logs contain no key.

Verification for this follow-up: go build ./..., go vet ./..., and
fresh go test -race -count=1 ./... all PASS. Output is in
/tmp/judge-default-final-tests.log; initial regressions are recorded in
/tmp/default-change-red.log and /tmp/cost-log-red.log. No live evaluation was
rerun. The optional Chromium smoke was not rerun for this routing/log-only change.

Verifier follow-up (partial): D2 maps invalid/too-short audio to the specified
retry sentence; D5 uses concise pointer-appropriate permission guidance.
D3 applies the existing one-shot 200ms strike animation (static with reduced
motion). D7 gives wordmark/navigation/Again minimum 44px targets. D10 gives
allowance-refused drafts a real Display heading. Regression assertions were
run failing before implementation. F1 and F2 remain outstanding.

F1: Keep the conservative draft reservation and atomic cap unchanged rather
than risk under-reserving live costs. Billing now counts worst-case draft
reservations from remaining spendable credit, subtracting in-flight holds;
it explains the $0.90 start requirement and actual-cost settlement. Learn
and refused drafts explicitly say not enough allowance left for a draft.
The trial-until-refused HTTP regression failed before the copy change.

F2: Preserve minute-key double-click deduplication. The credit transaction now
reports whether it added credit (including payment-reference deduplication);
the HTTP handler shows success only for an actual credit, otherwise an
explicit already-processed/no-additional-credit notice. Existing internal
TopUp callers retain their idempotent error-only API. The repeat HTTP
regression failed before implementation.

Final F1/F2 verification: go build ./..., go vet ./..., and
go test -race -count=1 ./... pass. Chromium TestReviewBrowserSmoke passes
against its isolated httptest server (SCRY_BROWSER_SMOKE=1). The F2 race
regression also confirms eight concurrent requests report exactly one credit.
No preview files or process on port 18080 were touched.

MIS-1210 ruling 6: remove all punctuation, including apostrophes, within
whitespace-delimited words before comparing; preserve original words in the
plain diff. No fuzzy matching.

MIS-1210 ruling 4: $5 top-ups, four per UTC calendar month. PRODUCT did not
state credit amount: preserve $4 credit per $5. Serialize the charge and credit
under the existing billing transaction, with a second cap check at credit.
Billing disables top-ups and states the reset date; minute dedup remains.

MIS-1210 rulings 2 and 3: Clerk replaces the former password provider entirely.
The official Go SDK verifies each browser session; only a verified primary
email can resolve to a Scry account. Agent bearer tokens are unchanged.
No Clerk keys means a visible sign-in-not-configured page, not an auth startup
failure. Configure CLERK_PUBLISHABLE_KEY and CLERK_SECRET_KEY in the environment.
In Clerk Dashboard enable email/password, require email verification and enable
email password reset; disable social and passwordless sign-in. Built-in SignUp
and SignIn components provide verification and Forgot password flows. Real key
provisioning, allowed origins and delivery configuration remain operator work.
SDK/component references: https://clerk.com/docs/go/getting-started/quickstart
and https://clerk.com/docs/js-frontend/getting-started/quickstart .

MIS-1210 ruling 1: PRODUCT used no grade labels, so learner labels are correct,
almost correct, incorrect. Stored scheduling grades stay stable. The one choice
question uses plain instructions and label-only criteria (the API's fixed
choice set), not the old rubric. Explanation seam and its existing strings are
unchanged; refusals/missing/none_of_these still abstain once with cost logged.

## docs/runbook — MIS-1210 monitoring fault drill

Set SENTRY_DSN in the service environment; unset is a no-op. Error-level
structured events (including recovered panics, background failures and every
HTTP 5xx) report sanitized event names to Sentry. /healthz checks SQLite.
Run `go build -o /tmp/scry-watch ./cmd/scry-watch` then
`/tmp/scry-watch` under a host supervisor. Set SCRY_WATCH_HEALTH_URL,
SCRY_WATCH_STATE, SCRY_WATCH_LISTEN, SCRY_WATCH_INTERVAL, SCRY_WATCH_QUIET,
SCRY_WATCH_RETRY, SCRY_WATCH_RESTART_CMD, SCRY_WATCH_AGENT_CMD,
SCRY_DEPLOY_COMMIT and SCRY_WATCH_LOG_FILE as appropriate. Defaults probe
loopback every ten seconds, detect three failures, retry restart every minute,
and close after two minutes without signals. State is a private, atomic,
fsynced JSON file separate from SQLite; one process holds its lock.
`/tmp/scry-watch incidents` lists evidence and lifecycle timestamps.

Error source: signed Sentry-style alert webhook (HMAC-SHA256 raw body in
Sentry-Hook-Signature; action=triggered). Set SCRY_WATCH_WEBHOOK_SECRET on
the watcher and configure Sentry alert delivery to its listener through TLS.
Unset secret disables intake. For dependable local drill delivery without a
Sentry account, set SCRY_WATCH_WEBHOOK_URL and SCRY_WATCH_WEBHOOK_SECRET on
the app too: every error mirrors a content-free signed signal, with three
bounded delivery attempts. Keep the listener private; no payload/user text is
stored. SCRY_MODE=test disables outside notifications and restricts app mirror
and health requests to loopback. Use local stand-ins for restart/fixer commands.

ALERT_WEBHOOK_URL configures operator notification; unset uses local records.
The fixer receives the brief on stdin and SCRY_INCIDENT_ID in its environment.
Its contract is to open a PR, deduplicating by that ID, and exit zero only after
accepting that responsibility. It must be trusted, sandboxed by the operator,
and must not deploy. A successful handoff is timestamped, not a claim of PR
merge. Without it the record/notification says "fix proposal not configured".
Commands time out after thirty seconds; outputs are never persisted.
Disable watchdog cron/competing liveness restart owners when using the watcher.
Use a restart command that starts stopped services as well as replacing hung ones.

Against a disposable running test-mode preview (NOT port 18080), enable
SCRY_FAULT_DRILL and SCRY_FAULT_DRILL_TOKEN on the app; configure the local
signed mirror, watcher, and a fixer stand-in that writes stdin to a file.
Export SCRY_DRILL_URL, SCRY_FAULT_DRILL_TOKEN, SCRY_WATCH_STATE and
SCRY_DRILL_KILL_CMD (a command killing only that preview process). Run exactly:

    python3 deploy/fault-drill.py

The script requires no preexisting open incident, posts the real drill, checks
one incident and fix handoff, kills the process, and verifies watcher-driven
recovery and quiet-window resolution. Afterwards disable the drill settings.
Automated isolated equivalent:

    go test -race -count=1 ./internal/watch -run TestStory_ContinuousMonitoringFaultDrillRestartRecovery -v

Accepted-rulings verification supplement: Clerk identity tests reject unverified
and banned users and exercise verified/unverified browser stand-ins without
network. Pending sessions are rejected. The component does not redirect an
unverified user into a study/sign-in loop. Concurrent cap testing starts with
three paid top-ups and confirms twelve racing requests add exactly one more.
Live Clerk delivery and a real Sentry-to-agent drill require external account
configuration; these cannot be proven with local stand-ins.

Final accepted-rulings checks: go mod tidy, go build ./..., go vet ./...,
go test -race -count=1 ./..., and the real Chromium TestReviewBrowserSmoke
all pass. Smoke uses an isolated httptest server, not the preview. Logs:
 /tmp/rulings-final.log and /tmp/rulings-smoke.log.
The capped top-up transaction also commits decline records before returning
the decline, preserving the existing card-failure behavior.

Accepted chat:phaedrus/t3579u, D1: preserve one transcription after stop.
DESIGN now describes the unchanged answer line and listening label, not live
transcription. Chromium smoke asserts no upload or new words while listening.

D6: preserve intrinsic Judgment widths rather than shrinking flex children
before wrapping. If the two answers do not fit, they stack left-aligned with
the phone spacing; long individual answers still wrap without overflow.
Smoke checks the real missed state at 1280, 900, 600 and 390 pixels.

D8: 68ch applies to reading pages, not inbox/draft/scrollback rows. Existing
reading paragraphs and ledes already use the 68ch measure; no CSS change.
TestD8_ReadingMeasure locks the existing reading-page constraints.

D9: retain the newest right answer's why only until another answer arrives.
Existing behavior already handled right-to-right and right-to-pending-miss;
fixed the remaining case where Next on a miss resurrected that miss's reason
in scrollback. Chromium verifies visibility, removal and non-resurrection,
with desktop/mobile screenshots of both retained and receded states.

D11: dedicated keys cover the five primary navigation destinations, search,
and study Check/Next, mic start/stop and exact Again. Settings and other
controls remain Tab/Enter, as explicitly accepted. Escape leaves an input to
make navigation available without stealing typed characters. Study shortcuts
now also work outside their forms; native focused-button activation is kept.
Footer and study hints document the keys. Inbox/draft extras are unchanged.

Desktop/mobile check found two D6 details: the flex pair needs min-width:0
inside its grid track to prevent intrinsic-width overflow, and the 200ms
strike animation must release its clip afterwards so wrapped lines remain
visible. Both are corrected without changing timing or reduced motion.
Screenshots under /home/user/verify/accepted-design show full long answers
stacking, and the newest right reason disappearing after the next answer.
The mobile fixed Next behavior is unchanged (D12 is outside this work).
Smoke also exercises primary navigation, search, Escape, Space start/stop,
Enter Check/Next and exact-text Again with A on a real isolated session.

Final t3579u verification: go build ./..., go vet ./..., and fresh
go test -race -count=1 ./... all PASS (/tmp/design-final-tests.log).
Chromium smoke PASS (/tmp/design-smoke.log), including actual keyboard flows.
Visually checked D6 long/short pairs and D9 newest/receded states at 1280x800
and 390x844; screenshots are in /home/user/verify/accepted-design/.
D6 geometry also passed at 900 and 600 pixels. Reading-page paragraph measure
checks passed on Billing and Settings. The preview and port 18080 were not used.

## MIS-1210 starter accuracy pass

- "Every major subject" has no enumerated syllabus. Kept the existing 30 sets;
  coverage limitations and all 742-item review results are in AUDIT-STARTER.md.
- Exact texts need an edition choice. Used 1662 BCP, modern-spelling KJV
  (1769 tradition), 1787 engrossed Preamble, 1864 Bliss Gettysburg copy,
  Hutchinson 1914 Shelley, and the 1956 NATO alphabet. Details in the audit.
- Standardized ambiguous prompts by naming the relevant measurement convention,
  theorem assumptions, historical period or usage sense rather than demanding
  that the semantic grader infer unstated conditions.

## 2026-11-18 — MIS-1210 per-adapter preview selection

Test mode now selects Clerk only with both Clerk keys and Stripe only with all
four Stripe settings; incomplete sets fall back independently to local auth
or fake billing. Model and speech remain fake even when keys are present.
Stripe still rejects non-test keys. Startup logs still name every adapter.
Live mode requires every adapter credential, including both Clerk keys (this
supersedes the earlier no-Clerk-keys/configuration-page startup exception).
Failed builds close the database.

With Clerk, SCRY_SEED_DEMO skips the entire demo fixture, not just the account;
the demo login hint stays hidden. Local auth retains the existing seed behavior.
Test mode suppresses Sentry initialization/reporting as well as webhook alerts.

Clerk's existing SignUp/SignIn components own signup, required verification and
Forgot password; its script loads from the publishable key's domain. Local HTTP
and JavaScript stand-ins cover rendering, component mounting, verification and
pending-session gates, verified redirects and Clerk sign-out. Dashboard email/
password, required verification, reset and allowed-origin configuration are
still operator responsibilities. No credentials were read and no live provider
or email delivery was tested; real Clerk and Stripe flows remain unverified.

Selection tables, missing-live-key checks and non-test Stripe rejection tests
were added. go build ./..., go vet ./... and go test -race -count=1 ./... pass.
The preview and port 18080 were not touched.


## 2026-11-19 — MIS-1210 monitoring incident owner
- Added cmd/scry-watch and internal/watch. One open incident groups concurrent
  symptoms until quiet; repeat signals update bounded evidence, not new pages.
- Chose signed webhook intake, plus content-free local mirror for credential-free
  drills; Sentry reporting remains optional and independent.
- Strict structural log allowlist discards messages/attributes rather than
  attempting unreliable secret redaction. Commit accepts hexadecimal only.
- Durable records precede side effects; command delivery is at-least-once after
  crashes, so fix commands must deduplicate by incident ID. No PR provider or
  production credentials were used; local stand-ins prove the handoff.
- Host supervision, external-host outage monitoring, TLS ingress and real
  coding-agent PR permissions remain deployment responsibilities.
- Verification: go build ./..., go vet ./..., and go test -race -count=1 ./...
  passed, including the real isolated app/drill/kill/restart test and failed
  fixer retry. No existing preview, production service, Sentry account, or
  external coding agent was contacted.

## 2026-11-19 — MIS-1210 auth page copy
- DESIGN specifies the sign-up layout and trial terms, but no account heading.
  Chose “Sign up” with “Create your account with email and password.” for
  account creation; Clerk also explains verification before study or billing.
- Sign-in retains its own heading and password-reset guidance. Clerk hash
  verification/reset steps update Scry's heading, title and lede without
  remounting the component. Unconfigured sign-up now names sign-up, not sign-in.
  The card step already had the correct “Add a card” title and trial terms.
- Pending sign-up persistence and the brief verification URL before the card
  redirect remain Clerk-owned; no navigation or session behavior was changed.
  HTTP behavior tests cover local/Clerk/unconfigured pages and error renders;
  offline JavaScript stand-ins cover verification/reset and hash transitions.
  No credentials or live Clerk flows were accessed.
- Verification: go build ./..., go vet ./..., and go test -race -count=1 ./...
  passed. The existing preview and port 18080 were not touched.

## 2026-11-20 — MIS-1210 Slice #3: Cold Critic defects and on-demand "Why?"

Starting point 3108995 (PR #493). Every item has a named regression test (README table).
- D2 voice: speech.Inspect classifies a valid WAV with no samples, under
  0.25 s, or digitally silent (peak <= 16) as ErrNoSpeech; POST /study/speech
  answers 200 {"text":"","caught":false} without a provider call or metering.
  Malformed uploads are 415 and >120 s are 413, each with its own sentence.
  app.js shows DESIGN's "Didn't catch that. Try again or type it." for any empty
  result and no longer keys it to HTTP 400.
- F2 top-ups: the per-minute idempotency key is gone. Each rendered Billing
  form carries a nonce bound to learner and session by HMAC (topup_token); the
  processor key is topup-<user>-<nonce>. A topups table (user_id, token) holds
  pending/succeeded/failed attempts. Duplicate submissions of one form, at any
  distance in time, are one charge and one credit; separate forms are separate
  top-ups up to the cap.
- PaymentIntent reconciliation: Stripe charges carry metadata[scry_key];
  Processor.FindCharge lists the customer's PaymentIntents since the attempt
  (strongly consistent list, not search) and matches the key. Pending attempts
  are reconciled on resubmission, on every Billing view and each minute in
  App.Run: succeeded -> credit once; failed/cancelled -> release with reason;
  not found -> keep pending for 2 minutes (TopupSettleWindow), then release.
  Reconciliation never creates or confirms a charge.
- D3 lock: App.TopUp is three steps: short TX (token, UTC cap counting paid +
  pending, reserve pending row), the processor call outside any transaction
  (detached from client cancellation, 30 s bound), short TX to finalize. An
  in-process claim per key makes a double click wait for the first request.
- D4: an ended plan (or past grace) never claims spendable drafts on Billing;
  it says study/judging/drafting need an active plan and leftover credit is kept
  for a restart, matching /learn's redirect. The cap month comes from one helper,
  store.MonthBounds (UTC), used by App.TopUp and Billing.
- Explanation ruling: Judgment.Close and automatic close highlights removed.
  ai.Explainer (Claude adapter, raw HTTP like drafting; the SDK stays removed),
  App.Explain and POST /study/explain. Test mode: ai.FakeExplainer
  (SCRY_TEST_EXPLAIN=fail|slow for the fallback) and SCRY_TEST_JUDGMENTS for an
  answer-independent verdict script on the running binary.
- Decisions taken here: explanation usage is metered as kind "judge" (the usage
  table CHECK predates it); an errored explanation releases its reservation
  rather than holding it, since the call is optional and bounded (~$0.06 worst
  case); default explanation model is claude-opus-5-5 at low effort with no
  server-side fallbacks (a refusal falls back to the criterion sentence, in line
  with the existing no-fallback cost policy). No live provider was called.

## Slice #3, Pass 2 (Cold Critic remediation)

- F2, absence is never proof: a PaymentIntent lookup that finds nothing,
  however long after the attempt, leaves the attempt pending (it keeps its
  slot under that UTC month's cap). Past TopupSettleWindow the learner is
  told the top-up could not be confirmed yet, that its credit is added
  automatically if the charge went through, and that it holds a monthly
  slot until confirmed (ok=unconfirmed; Pass 3 removed the old "nothing has
  been charged" claim, which an unseen success could make false). Only a definitive processor answer settles it:
  succeeded -> credit once; canceled / requires_payment_method / a decline ->
  failed. Billing and the minute sweep keep asking for TopupWatchWindow
  (35 days); after that the attempt still stays pending and its webhook
  still settles it. Choice: holding the cap slot (rather than releasing it)
  means the $20 cap cannot be exceeded by a late success; the cost is that a
  request Stripe never recovers uses one slot until the month ends.
- Lookups: Stripe.FindCharge retrieves GET /v1/payment_intents/:id when the
  attempt knows its intent id; otherwise it lists the attempt's own recorded
  customer's intents, up to 20 pages, and a list still has_more after that
  is billing.ErrLookupIncomplete (inconclusive), never "not found".
- The attempt row records customer_ref and payment_ref inside the
  reservation transaction, and charge_ref (the intent id) as soon as it is
  known. Charge and every lookup use those, never the current subscription
  (a resubscription opens a new customer). Rows from before these columns
  fall back to the current plan's customer.
- A 200 PaymentIntent in processing or requires_capture is
  billing.PendingError: the id is kept, the attempt stays pending and the
  learner sees "being confirmed", not a decline.
- Webhooks: payment_intent.succeeded / payment_failed / canceled carrying
  metadata.scry_key are matched to the attempt by charge_ref or idempotency
  key (and refused if the customer differs), marked once by event id inside
  the billing transaction, and settle it. A success always credits exactly
  once, even after a lookup saw nothing or the attempt was marked failed
  (the processor is authoritative; the "card failed" line is replaced by the
  paid one, which in that rare case can take a month past the cap).
  Intents without Scry's key (plan invoices) are ignored.
- D3: whether the plan may be charged (App.HasAccess: trialing, active, or
  past_due within grace) is decided inside TX1 against the row under the
  write lock, together with the token, the cap and the captured customer and
  card. The earlier App.Plan read only renews and refuses early. No network
  call is made inside any transaction.
- Explain: reviews snapshot graded_prompt and graded_expected when the
  verdict is recorded; "Why?" explains those, so editing the card later does
  not change what is explained. A review from before the snapshot gets the
  verdict's criterion sentence, since its card may have been edited.
- Migration: added columns are applied with ALTER TABLE on open, and
  RestoreUser copies by shared column names, so a snapshot taken before the
  columns existed still restores.

## Slice #3, Pass 3 (Cold Critic remediation)

- F2, non-final PaymentIntents: only succeeded is paid, and only canceled
  and requires_payment_method are final refusals. processing,
  requires_capture, requires_action, requires_confirmation (and any status
  this adapter does not know) are ChargePending in both Stripe.Charge
  (id + billing.PendingError) and Stripe.FindCharge, so the attempt stays
  pending with charge_ref set, holds its cap slot, redirects ok=pending,
  and settles by GET /v1/payment_intents/:id or by webhook, once. Choice:
  an off-session intent left in requires_action is never declined early;
  it holds its slot until Stripe cancels it (canceled webhook or lookup) or
  the month ends.
- F2, a known intent id is never dropped: a 4xx/5xx error body's
  error.payment_intent.id is surfaced on billing.DeclineError.Ref (card
  errors) or billing.ProcessorError.Ref (anything else, indeterminate:
  noted on the still-pending attempt). Store.FailTopup takes the charge id
  and records it unless the attempt already has one (empty never clears
  it); finishCharge, reconcile (list- or id-discovered failures) and the
  failure/cancel webhook all pass it. A late success webhook still matches
  by charge_ref or key and credits once.
- A canceled intent with no last_payment_error releases the slot with no
  card fault (ok=released); requires_payment_method without a sentence is
  "Your card was declined.", matching the payment_failed webhook.

## Slice #3, Pass 4 (Cold Critic remediation)

- F2, ID-only webhooks: a signed payment_intent.succeeded, payment_failed
  or canceled whose metadata has no scry_key is no longer dropped by
  Stripe.ParseWebhook. It becomes a top-up event with the intent's id as
  ChargeRef, an empty key, the customer and any reason; applyTopupEvent
  finds the attempt by saved charge_ref (Store.TopupByCharge never matches
  an empty key), applies the customer guard, and settles it on delivery: a
  pending or failed attempt is credited once on succeeded, a pending one is
  released on payment_failed/canceled. Choice: an intent no attempt
  recorded (a plan's invoice PaymentIntent) is acknowledged with 204 and
  changes nothing, not even the processed-event marker.
- Direct HTTP 200 canceled with no last_payment_error is a
  billing.CanceledError, not a decline: finishCharge fails the attempt with
  an empty reason (charge_ref kept, slot released, no card fault, no failed
  history line) and redirects ok=released, exactly like a lookup that finds
  the intent canceled. canceled with a processor reason, and
  requires_payment_method, are still declines.

## Slice #3, Pass 4b (Cold Critic edge fixes)

- A payment_intent.canceled webhook keeps last_payment_error.message as
  the reason (card fault + one failed history line), like a direct canceled
  charge; with no message it still releases fault-free.
- Stripe.ParseWebhook ignores a PaymentIntent event whose object has an
  empty id, even with metadata.scry_key. Store.SucceedTopup never clears a
  saved charge_ref with an empty one (it uses the saved id) and, with no id
  at all, returns store.ErrNoChargeRef without changing the attempt or the
  ledger; applyTopupEvent ignores such a paid event before marking it. So a
  paid ledger line never has a blank ref.

## Slice #12 — prod go-live hardening

Choices where the brief was silent:

- Reservation sweep: `Store.FailStaleReservations` deletes every row at boot
  (before serving; Scry is one process and `scry-deploy` restarts rather than
  overlaps). An orphan never charges usage: the sweep only releases the hold.
- Clerk: the SDK's `jwks.Client` does not cache (each `Get` is an HTTPS call),
  so Scry caches the fetched `JSONWebKeySet` (10 min) and passes the matching
  key to `jwt.Verify` as `JWK`, which still checks signature, expiry, issuer
  and azp; sts is checked after. A kid missing from a fresh set refetches at
  most once per 30 s so made-up kids cannot turn each request into a Clerk
  call. A key-set fetch failure is not cached and stale keys are not served.
  Identities are cached 60 s, max 1024, oldest-expiring evicted; only
  verified active identities whose id equals the token subject are cached. A
  ban therefore takes effect within 60 s even if the session token is still
  valid (Clerk also revokes the user's sessions on ban).
- Stripe: `NewStripe` stays as the test-mode constructor (tests and test-mode
  wiring); `NewStripeWithMode(live, …)` is the explicit one. A webhook with no
  `livemode` field decodes as false, so a live adapter refuses it.
  `SCRY_STRIPE_MODE=test` in test mode without all four Stripe settings stops
  the boot (an explicit request for Stripe is never silently the fake).
- `SCRY_JUDGE_MODE=live` in test mode wraps the fake model as the drafter
  (`ai.Split`), so drafts, explanations, speech and billing stay fake; the
  scripted `SCRY_TEST_JUDGMENTS` no longer apply because Jev grades.
  `ai.Split.Name()` now names both halves (`jev+anthropic-drafts` in live,
  `jev+test-drafts` here).
- /healthz: `providers.grading` names the Jev route (`jev-openrouter` or
  `jev-typesafe`). A `warnings` array (always present) carries the staged
  Stripe cutover in live mode and the real grader in test mode; status stays
  "ok" because both are deliberate. The 503 body carries the same readiness
  fields with `db:false`.
- Build SHA: `-X main.buildSHA` wins; otherwise the 7-char `vcs.revision`
  from `debug.ReadBuildInfo()`; otherwise `dev` (Docker builds have no .git, so
  the Dockerfile takes `--build-arg BUILD_SHA`).
- scry-watch: JSON logs to stdout; invalid durations now exit with an error
  instead of a panic; a probe cut short by the watcher's own SIGTERM is not
  counted; SIGTERM shuts the webhook listener down gracefully and exits 0.
- install.sh now installs (does not enable) `scry-watch.service` and also
  copies OPENROUTER_/TYPESAFE_ keys and the SCRY_*_MODE/model/watch settings
  into scry.env; live mode already needed a Jev key that the old filter
  dropped.

## Slice #13 — explicit state machines (reservations, Stripe)

Choices where the brief was silent:

- Reservations are an explicit machine (`store.TransitionReservation`):
  none → held → settled | released | swept. A settle that finds no row is a
  replay and returns nil without spending. Only `held` is stored; the
  terminal states are the row's absence, so the store cannot tell a
  never-reserved id from a finished one. Ids are random tokens, so this is
  not a real ambiguity.
- `Reserve` checks access (trialing, active, or past_due within grace) and
  the budget in the same `INSERT … SELECT`. The app calls `ReserveAt` with
  its own clock and `GraceDays`. A refusal on access is
  `store.ErrReservationNoAccess`, which the app reports as `ErrPlanEnded`
  (redirect to Billing). `Reserve` without a clock uses the wall clock and
  the shipped 7-day grace. An amount ≤ 0 is `ErrInvalidReservationAmount`.
- `reservations.created_at`: added by `addColumn` on open. Old rows read 0
  and are released by the next TTL sweep. Housekeeping runs
  `SweepExpiredReservations(now − 1h)` hourly (`app.ReservationTTL`; the
  longest provider call is the 5-minute draft). This also releases G2's
  deliberately held unknown-outcome holds after an hour, instead of only at
  the next restart.
- Settle debits the clamped actual cost (allowance first, then top-up,
  never below zero), not `min(cost, reserved)` as the Quint spec's `step`
  models. This keeps the Transcribe rule that a recording's known cost is
  never forgiven. The spec's `inv_debit_within_reservation` and
  `inv_reserved_within_available` are therefore not Go invariants. The
  rapid test checks the non-negative balances, recovery and idempotence
  invariants instead.
- Stripe `invoice.payment_failed` now carries the period from its first
  line, as `invoice.paid` already did. A failure for a period that is not
  newer than the plan's current one is ignored (`applyEvent`).

## Slice 20: OpenRouter for everything

Choices where the brief was silent:

- Precedence is NewJev's everywhere (`ai.NewClaudeWithRouter`,
  `speech.NewTranscriber`, `cmd/scry.realAI`): when OPENROUTER_API_KEY is set
  it carries grading, drafting, explanations and speech, and the direct keys
  are not read at all. A stale JWT-shaped OPENAI_API_KEY beside a good
  OPENROUTER_API_KEY therefore boots. Each fallback key is checked only when
  it is the route in use.
- TestLiveRequiresEveryCredential and two /healthz expectations changed with
  that rule: ANTHROPIC_API_KEY and OPENAI_API_KEY are no longer required when
  OPENROUTER_API_KEY is set, so the test now checks both sides (covered with
  the router key; each one named when it is unset). The direct-fallback
  /healthz case was added next to the full-live one.
- The JWT-shape check (`safeerr.LooksLikeJWT`: three base64url segments whose
  header starts eyJ or decodes to a JSON object) runs at boot on every AI key
  in use (OpenRouter, or Typesafe/Anthropic/OpenAI on the fallback) and again
  inside both speech adapters' Transcribe. Errors name the variable, never
  the value.
- On OpenRouter a model override must be a vendor/model slug. A leftover
  direct name such as SCRY_DRAFT_MODEL=claude-opus-5-5 stops the boot
  instead of failing every call.
- OpenRouter explanations drop Anthropic's `output_config` (effort low);
  the default there is anthropic/claude-sonnet-5.5 for both drafting and
  explanations. Budgets keep the existing conservative price ceilings; the
  exact `usage.cost` is recorded when OpenRouter returns a sane one (0–$100)
  and never inferred.
- OpenRouter speech sends the recording as `input_audio` (format from the
  MIME type, default webm) to google/gemini-2.5-flash with temperature 0 and
  512 output tokens. A transcript wrapped in one pair of quotation marks is
  unwrapped; words are never changed. An empty recording is "nothing heard"
  with no call, on both speech adapters.
- SCRY_AUTH_MODE accepts test and clerk-test as the same thing. Unset keeps
  the old behaviour (live mode boots on Clerk test keys) but /healthz now
  always warns about it. A Clerk key pair from different instances never
  boots. In test mode an explicit test/clerk-test needs both Clerk keys,
  like SCRY_STRIPE_MODE=test needs all four Stripe settings.
- SCRY_AI_MODE=live in test mode wires the real adapters through the same
  `realAI` as live mode. Combined with SCRY_JUDGE_MODE=test it is refused as
  a contradiction. The /healthz warning lists each real capability and its
  route.

Pass 2 (cold critic remediation):

- G2 on OpenRouter: `App.runDraft` settles a reservation only when
  `Usage.Model` is set. The OpenRouter drafting stream now sets it only once
  a usage chunk arrives (or on a clean `[DONE]` completion whose usage chunk
  upstream omitted). Any failure before usage (truncated stream, stream
  error object, `finish_reason` error, `content_filter` without usage,
  invalid chunk, sink error, disconnect) returns an empty model, so the
  reservation stays held until reconciled or swept, as on the direct route.
  Usage reported on a chunk that also fails is kept and settles at its cost.
  TestG2_OpenRouterDraftTruncatedWithoutUsageKeepsReservation covers it end
  to end on the real adapter.
- The OpenRouter explainer follows the same rule at the adapter (empty model
  without reported usage). `App.Explain` is unchanged on purpose: its small
  reservation is released on failure, which
  TestExplain_FallsBackToTheCriterionSentence already requires.
- `finish_reason: "error"` is an explicit error on the explainer and on
  OpenRouter speech; partial text is never returned.
- One slug check, `safeerr.IsRouterSlug`, for SCRY_DRAFT_MODEL,
  SCRY_EXPLAIN_MODEL and SCRY_SPEECH_MODEL on OpenRouter: at least
  vendor/model, no whitespace or control characters, no leading, trailing
  or doubled slash.
- SCRY_JUDGE_MODE=live in test mode checks the key Jev will use
  (OPENROUTER_API_KEY, else TYPESAFE_API_KEY) like realAI, so a JWT-shaped
  key stops the boot.
- Test mode refuses live Clerk keys (pk_live_/sk_live_) whatever
  SCRY_AUTH_MODE says, as it refuses live Stripe keys, even when only one is
  set. SCRY_AUTH_MODE=test/clerk-test now also refuses a Clerk key with
  neither prefix.

## Slice 30: Stripe card CSP, client telemetry, live-route verifier

- Root cause: `secure()` assigned `frame-src` per integration, so with Clerk
  configured `frame = "https://challenges.cloudflare.com"` replaced Stripe's
  `https://js.stripe.com`, and Chrome drew a broken-document icon in the card
  field. The policy is now accumulated per directive (deduplicated, fixed
  order) in `internal/web/csp.go`.
- A second live defect showed up while verifying against production: Clerk JS
  v5 creates `blob:` workers on every page and the live policy had no
  `worker-src`, so each page logged two `worker-src` violations. The Clerk set
  now includes `worker-src 'self' blob:`.
- `https://*.js.stripe.com` is added beside `https://js.stripe.com` in
  `script-src` and `frame-src`, following Stripe's current CSP guidance; the
  brief's list is otherwise used exactly.
- The Stripe card Element is created with 16px text on a 24px line. DESIGN.md
  is silent on the card field; Stripe's default made its frame 17px tall, and
  16px also stops phones zooming into it. No other look changes.
- When Stripe.js is missing, the Element fails to mount or never signals ready
  within 15s, the card step says "The card form couldn't load. Check your
  connection, allow js.stripe.com in any content blocker, then reload this
  page." under the field, refuses to post the form without a card, and
  reports `integration_error` to /api/client-error. Onboarding, billing and
  trial logic are unchanged.
- /api/client-error logs at error level (forwarded to Sentry, and to
  scry-watch when configured) once per distinct sanitized message per 10
  minutes; repeats and reports blamed on browser extensions are warn-level
  local logs. Sentry receives only the message line, which is built from
  closed allowlists alone; `script-sample`, `original-policy`, `referrer`
  and any `message` field are never read. Routes are reduced to Scry's
  route templates or `/other` (the brief's list plus `/library/texts/:id`; a
  trailing slash is ignored, case is not; `/api/...` is `/other`).
  Directives outside the standard list become `other-directive`. URIs are a
  CSP keyword, `extension`, the bare path of a file embedded under
  `static/` (only when relative or on SCRY_BASE_URL's origin,
  scry.mistystep.io on its default port, or 127.0.0.1/localhost on any
  port; the request Host header never counts), `https://<host>` for an
  allowlisted integration host on its default port (`http:` is reported as
  `https:`), the fixed wildcard `https://*.js.stripe.com` or
  `https://*.protect.clerk.com` for any of their subdomains (the label is
  never kept), or `external`. A Scry page URL that is not a static asset is
  `external` too, as the brief's closed list has no other value for it.
  `*.clerk.accounts.dev` is no longer named; the configured Clerk Frontend
  API host is. For a CSP violation only route, directive, effective
  directive, blocked URI, `disposition=report` (when report-only) and build
  are kept. `js_error`/`unhandled_rejection` carry an allowlisted error
  class (app.js takes `error.name`, else the `Uncaught X:` prefix) and a
  line/column only when the source is a first-party asset or allowlisted
  origin, not `external`. Reports of a known kind are accepted without a
  message, since none is read.
- Sentry events from the operator log are sent at level `error`
  (`sentry.CaptureEvent`), not CaptureMessage's default `info`, so Sentry
  error-level alert rules fire.
- Per-address limits use the connection address, like the sign-in limiter;
  behind a proxy that is one shared budget, which also bounds Sentry volume.
- `scripts/verify-live.mjs` treats `net::ERR_ABORTED` as benign only for
  cancelled navigations and for requests a cross-origin child frame (Stripe's
  hCaptcha logo) cancels itself; requests Scry's policy blocks fail as
  `ERR_BLOCKED_BY_CSP` and always count.

## Slice 36: empirical model winners, OpenRouter audio/transcriptions, 7-day no-card trial

Owner-approved spec change (2026-11-21). Choices where the brief left room:

- **Defaults.** Grading `typesafe/jev-1.14` (Decisions route), explainer
  `openai/gpt-6-luna`, drafting `openai/gpt-6.2-sol`, speech
  `microsoft/mai-transcribe-2`, all on `OPENROUTER_API_KEY`. Each
  `SCRY_*_MODEL` overrides its surface; `SCRY_JUDGE_MODEL` is now checked to be a
  vendor/model slug when OpenRouter is the route, like the other three.
- **Speech routing** is by slug (`speech.UsesTranscriptionsRoute`): anything
  from `elevenlabs/`, `deepgram/` or `assemblyai/`, `mistralai/voxtral*`, any
  `*whisper*`, `*transcribe*` or `*scribe*` model, and `*-stt*` names go to `POST /api/v1/audio/transcriptions` as multipart `model` + `file`
  (`audio.<ext>` from `AudioFormat`, with a matching audio Content-Type); every
  other slug is treated as an audio-input chat model on chat completions with
  `input_audio`. No `language` field is sent (the eval forced `en`), so an
  answer in another language is still written down as spoken. Scry converts
  every recording to WAV before transcribing, so production uploads are
  `audio.wav`; the other extensions are covered by unit tests.
  `SCRY_SPEECH_ROUTE` (pass 2) overrides the name rule for a model it gets
  wrong: `auto` or unset (the rule), `transcriptions` or `chat` (forced). Any
  other value stops the boot in every mode, and `chat` without
  `OPENROUTER_API_KEY` does too (the direct OpenAI route has no chat audio).
  `/healthz` `speech_route` reports the effective route.
- **Pricing.** Catalog prices were read from the public
  `https://openrouter.ai/api/v1/models` listing on 2026-11-21: gpt-6.2-sol
  $2/$10 and gpt-6-luna $0.10/$0.50 per million tokens. jev-1.14 is not in the
  chat catalog; NOTES' $0.042/M input, free output is priced here at $0.10/$1 as
  a margin. mai-transcribe-2 lists `"0.1"` with no unit (Slice #29 read it as per
  second and skipped it). Read as per hour, which matches mai-transcribe-1.5's
  `0.36`, it is about 28 micros a second, so Scry keeps metering speech at 50
  micros a second ($0.003 a minute). It also meters a recording at the
  provider's own `usage.cost` whenever that is higher, so a wrong reading of the
  unit is metered truthfully rather than absorbed.
- **Model-aware reservations.** A hold is the worst case at the configured
  model's rate (`app.BudgetMicros`), and the old $8/$40 ceiling for any model
  without a price. A dated resolved id (`typesafe/jev-1.14-20261014`) is priced
  as its model. The old `ai.*BudgetMicros` constants remain as the ceiling.
- **Trial cap.** `Cfg.TrialMicros = $0.50` (no card). One worst-case draft
  ($0.226), judgment, explanation and longest recording fit inside it at once.
  The cap is enforced at admission, and that is all an admission rule can
  promise (pass 2): a hold is the most one call can draw, because
  `store.SettleMetered` debits `min(cost, hold, available)` (the Quint spec's
  "cap at reservation"), so a call billed over its hold never draws on
  allowance backing another hold in flight. The rest is written off, kept
  in full on the usage row, added to the plan's `period_overage_micros`, and
  logged as a warning ("usage cost over its reservation; overage written
  off"). On a no-card trial the overage counts as spent: `ReserveAt` admits a
  hold only while `allowance - used - overage - held >= amount`, so once what
  the trial actually cost reaches $0.50 nothing more is admitted. What it
  actually cost can still pass $0.50 by the overage of calls already
  admitted; no rule can bound a third party's bill after the fact, so the
  holds are sized to make that rare. Speech holds are
  `max(ceil(seconds) x 250, 15,000)` micros (`app.SpeechBudgetMicros`, $0.03
  for a 2-minute clip), 5x the meter, to cover what dedicated STT models
  (mai-transcribe, whisper, scribe) and audio chat models (gemini) report as
  `usage.cost`; the old 6,000-micro hold let 77 recordings billed $0.0065
  each reach $0.5005. On a card-backed plan overage is recorded but does not
  gate admission (the margin absorbs it).
  Adding a card raises the trial allowance to `Cfg.CardTrialMicros = $1.00`
  (the previous card-up-front trial allowance), and what was spent stays spent.
- **Upgrade inside the window** keeps the trial's end. Stripe is asked for
  `trial_end = period_end`, nothing is charged that day, and the first $5 is on
  the original end date, so adding a card early costs no free days. With under
  an hour left (`app.MinCardTrialLeft`) adding a card starts the paid plan at
  once, charged today, so Stripe is never asked for a trial ending within the
  hour. After the end it is the existing restart path: charged today, full
  allowance, no second trial.
- **Consent to the charge (pass 2).** The card step's form carries what its
  page said, `billing_terms` = `trial` ("nothing charged today") or
  `immediate` ("$5 today"); `app.CardTerms` decides it and the copy from the
  same plan state. `StartTrial` charges today only when the submitted form
  said `immediate`. A page loaded with over an hour left and submitted with
  less (or after the end), or a post with no or unknown terms, is refused
  before the processor is called: nothing is charged, the trial is
  unchanged, and the card step is shown again (409) saying the trial ends
  within the hour (or has ended) and a card now charges $5 today, with
  `billing_terms=immediate`, so the next submit is the learner's consent.
- **Nothing changes before the processor does (pass 2).** The trial row is
  never ended or rewritten before `Subscribe` succeeds. A declined card (402)
  or processor error leaves the no-card trial exactly as it was, usable until
  its own `period_end`. On success `store.StartPaidFromNoCard` turns the
  no-card row (trialing in its last hour, or ended) into the active paid plan
  in one transaction (full monthly allowance, nothing used, no overage), and
  refuses, with an operator alert, if a card was added to it in between.
- **One subscription per confirmed upgrade (pass 3).** The per-minute
  idempotency key is gone here too. `subscription_attempts` holds one pending
  attempt per learner (a partial unique index), claimed before the processor
  is called, with its key (`sub-<user>-<random>`) and the parameters sent
  under it (card token, email, trial end). A second card step in the same
  process while one is in flight gets 409 and sends nothing. An attempt whose
  outcome was never learned is repeated with its own key and parameters, so
  Stripe returns what it already did; a newly entered card is not substituted,
  because Stripe refuses a key with other parameters and the first card may
  already be charged (it stays the card on file). A definitive refusal (a
  decline, or `invalid_request_error`) marks the attempt failed, and the card
  just entered is then tried under a new key. The plan, its ledger line and the
  attempt's end are one transaction. Stripe forgets keys after 24 hours, so
  from 23 hours (or once a trial attempt's trial end has passed) an attempt is
  looked up by `metadata[scry_key]` and never sent again: recorded if it
  exists, released only if a complete lookup has nothing or it is over.
  `specs/subscription_claim.qnt` models it, with the old code's double charge
  as tests. An attempt is settled when the learner next submits a card; there
  is no background sweep. A learner who never returns after a lost reply is
  covered by the operator alert the failure raised.
- **Expiry** is enforced three times: `HasAccess` (trialing with
  `customer_ref = ''` only while `now < period_end`); the `ReserveAt` statement
  (`period_end > floor(now)`, the same predicate, since period_end is stored in
  whole seconds); and the move to `ended` (`Plan` on the learner's next request,
  plus `EndExpiredNoCardTrials` on the minute renewal tick for every processor,
  not just the test one). No-card rows are excluded from `SubscriptionsDue`, so
  the test processor never renews one into a paid plan, and `applyEvent`
  ignores any processor event naming one.
- **Legacy accounts** with no plan row (signed up before this slice and never
  added a card) start the no-card trial on their next visit (`Plan`), so no one
  is left on the old forced card step. A trial already had (a row exists, live
  or ended) is never given again.
- **Chrome indicator.** DESIGN.md said the trial end date is "never a banner
  elsewhere". The owner asked for a calm indicator in the chrome, so it is one
  line of micro mono text after the wordmark, not a banner. It is only on
  non-bare pages and only for no-card plans.
- **Cancel** is hidden on a no-card trial and `SetCancel` is a no-op for one:
  nothing would be charged, and there is no processor subscription to cancel.
- **Abuse.** One trial per account. A person can still open many local
  accounts, each capped at $0.50 of AI spend. With Clerk, each needs a verified
  email.

## Slice 41: dogfooding overhaul (post-answer review, spoilers, shelf, Create, icons)

Choices made where the brief, PRODUCT.md or DESIGN.md were silent or pulled two ways:

- **Every answer is held, in the state machine too.** A right answer used to
  pass straight on (`EventAnswerPassed`). The pair `(Asking, AnswerPassed)` is
  gone from the session table, so the store refuses an unheld scored answer
  rather than merely not being asked for one; the event stays defined so the
  refusal has a name and a test. `specs/study_session.qnt` models the same:
  CommitAnswer always holds and never ends the session; only Next does.
- **"Next card" reads "Finish session" on the last card.** Calling the last
  step "Next card" would promise a card that does not exist.
- **Ask Why? on correct answers** explains why the answer holds; the explainer
  prompt says so. Exact texts still have none: the word diff is the explanation.
- **Telemetry is FSRS's own fields.** Scry schedules with FSRS, which has
  stability and difficulty, not an "ease factor", so the review shows those
  (stability in days, difficulty out of 10) with the interval (last review to
  next due) and the counts from the reviews table. Practice attempts ("Again")
  are counted apart and say that the schedule did not move.
- **Inline edit changes text only.** `POST /study/edit` keeps the card's
  exact flag, schedule and history; the review on screen keeps the grade it was
  given against the old text (the graded snapshot on the review row), and the
  edit applies from the next review. A refused edit answers the page's script
  with 200 and `saved: false` (a 4xx would put an error in the console for an
  ordinary empty field); the plain form post still answers 422.
- **What counts as a spoiler.** The Study rail never shows an idea's own title
  while its card waits. It shows the top-level topic and "Set N", and each
  candidate label ("topic · Set N", topic, "Set N", "Current set", nothing) is
  checked whole-word against the answers of the card and its siblings; the
  first clean one wins. So "Set 3" gives way beside the answer "3 teaspoons".
  Matching is on whole words: "alphabet" does not show "Alpha". The "coming up"
  list also holds back a cue that quotes the owed answer (the next line of an
  exact text quotes the line before it), which the brief did not list but is
  the same leak.
- **Starter titles.** Besides the NATO sections ("Letters A–I", "J–R", "S–Z")
  two more named a whole answer and were renamed: "First principles and
  inversion" (answer "Inversion") and "Anchoring and framing" (answer
  "Anchoring"). `starter.Validate` now refuses a section title that contains a
  whole short answer. Topical overlap with long answers ("Compound interest" /
  "Interest earned on...") is not a title giving an answer away and is left;
  the rail guard covers it on the Study screen regardless. The set itself is
  now titled "NATO phonetic alphabet". Libraries that already hold these sets
  are renamed on open (by origin and old title only, idempotent).
- **Shelf order** is the file order (NN- prefix). The first four are NATO,
  Mental models, Kitchen numbers, World geography; the Creeds and Passages
  worth keeping are last. No set was removed.
- **API tokens.** New tokens always suggest through the inbox (`can_add` is
  ignored when posted). `/api/v1/items` still works for tokens created earlier
  with it. PRODUCT.md's story was reworded to match.
- **"Confirming your card with Stripe…"** is shown where Stripe is configured;
  with the test-mode processor it reads "Confirming your card…", which is true.
- **Icons vs "no icon-card grids".** DESIGN.md's Never list forbade icon-card
  grids. Tiles stay typographic hairline tiles; the icon is a small glyph on
  the title line. The rule now forbids filled or colored icon tiles and emoji.
- **The spinner loops.** DESIGN.md said nothing loops; the processing spinner
  and progress line are now the two named exceptions, still under reduced motion.
- **The browser smoke moved from puppeteer to Playwright**, which the repo
  already depends on for verify-live, so it runs with `npm --prefix scripts ci`.
- **Pass 2: what counts as a title giving a line away (exact texts).** The
  brief said "per the spoiler check". The rail's check (any shared word) would
  strip the title from shipped texts ("Preamble to the United States
  Constitution" shares words with its first line; "Ozymandias" with line 10),
  leaving "how does it begin?" with no text named, which cannot be answered.
  So the exact-text label and prompt use the stricter-to-trigger rule: the
  title is replaced by "Exact text" only when it holds the whole waiting line,
  word for word (case, punctuation and a leading article aside). The rail
  keeps its own any-shared-word rule. `TestSlice41_Spoiler_EveryStarterExactTextKeepsItsTitle`
  pins that no starter text loses its title.
- **Pass 2: the neutral cue.** A text named for its line is asked for as
  "Exact text: how does it begin?" / "Exact text: what follows “…”" (the stored
  cue with the title swapped), and labelled "Exact text · line 1". A
  hand-written prompt on such a text that still quotes the line falls back to
  "Line N of this exact text". The stored prompt is not changed; the review shows it.
- **Pass 2: the cue of a repeated line is left alone.** The cue for a line is
  the line before it. Where a text repeats a line, that cue is the same words
  as the answer; it is still the text's own cue, so the waiting card shows it.
- **Pass 2: short answers.** An answer of any length now holds back a cue that
  quotes it as whole words, so a one-word answer such as "A" can hold back an
  unrelated cue that happens to contain that word until it is answered. The
  list then shows the cues after it; nothing is lost.
- **Pass 2: the scrollback follows the same rule** as "coming up" while a card
  waits (a row whose cue quotes the answer owed is held back or shown under
  the neutral title). This matters after "Again", when the line's own row is
  above the card.
- **Pass 2: "Coming up" is Meta type in ink-muted** (13 px mono, 5.0:1 light,
  7.2:1 dark). ink-ghost now colors hairlines and the dot grid only.
- **Pass 2: a retry always confirms the card in the field.** Besides ignoring
  a late answer for a superseded attempt, stripe.js no longer re-posts a
  payment method left in the form by an earlier attempt whose post brought no
  page back: after the form is given back, the next submit confirms the card
  again. Otherwise a learner who changed the card after the timeout would post
  the earlier one.


## Slice 54: brand mark, landing specimen and craft pass (Folio 8bf4475e)

Reconciles PRODUCT.md and DESIGN.md as changed in 92350ed3..8bf4475e.
Choices where the Folio left room:

- **The mark's geometry.** (Superseded by [S113-3]: the lens is retired.) DESIGN.md fixes the grid (24 px, stroke 1.75,
  round caps/joins), the parts (four corner brackets, a centered lens ring in
  ink, a solid accent focal point of r 2.25) and the sizes. The exact numbers
  are ours: brackets `M3 8V3h5 …` (5 px arms from the 3 px inset) and a lens
  ring of r 5.25. `internal/web/brand.go` is the one source; the static icons
  repeat its paths and `TestBrand_MarkIsOneDrawingEverywhere` pins that.
- **App icons.** (Superseded by [S113-3].) `static/icon.svg` was the adaptive light/dark vector (favicon;
  `/favicon.ico` redirects to it). `static/icon-dark.svg` is the dark-ground
  icon (#12101A, white aperture, #4D8DFF focal point); the 180/192/512 PNGs are
  rendered from it with `rsvg-convert -w N -h N icon-dark.svg -o icon-N.png`
  and committed (no build step). The 512 PNG is the link-preview image
  (`og:image`, absolute: SCRY_BASE_URL when set, else the request origin).
- **Home screen.** `static/manifest.webmanifest` (served as
  application/manifest+json) has `start_url` `/study` and `display`
  standalone, so a home-screen launch opens daily recall (signed-out it
  passes through Sign in, as any /study visit does).
- **The Study Specimen** is static type, not a live session: a real starter
  card (Mental models · The razors, "Hanlon's razor"), an answer that differs
  only in case and punctuation, the verdict "Correct" and the grader's real
  criterion sentence (`ai.CriterionSentence`). Its schedule shows relative
  facts only ("in 3 days"), so the page never shows a stale date.
- **Public footer.** "Made by Misty Step · Your library and history are yours
  alone: nobody else can see them." on signed-out pages only; it restates the
  PRODUCT.md privacy story and claims nothing further.
- **Library on phones.** PRODUCT.md says the opened idea sits "immediately
  above a compact collapsible outline"; DESIGN.md says it "renders first
  beneath a compact collapsible idea picker". We follow DESIGN.md (it owns
  layout) in a way that satisfies both readings of the goal: one collapsed
  44 px picker row (naming the open idea) sits above the idea, and the full
  outline lives inside it, so the idea and its cards come before any outline
  rows. The picker is a separate phone-only `<details>` that repeats the
  outline (desktop keeps its pane); one of the two is always `display: none`.
- **Review framing.** "Next card" is focused with `preventScroll`, both after
  an answer is swapped in and on a full page load. Nothing scrolls the window
  programmatically (the slice-41 no-jump rule stands); a review the learner
  scrolled stays where they put it. The button carries no HTML `autofocus`
  (a browser without script scrolled a sentence-length Incorrect review down
  to it, question out of view); it is marked `data-autofocus-no-scroll` and
  focused by app.js only. Without script, Next card is therefore not focused
  (Enter/Space shortcuts need script anyway). A sentence-length Incorrect
  answer is struck at Judgment size, so on a 1440x900 window the verdict line
  can sit below the fold; the review still opens at the top with the question
  in view, and the layout is unchanged.
- **Settings appearance** applies the mode to the page on change and posts
  the same form in the background; "Save appearance" still works without
  script. Background saves are serialized: one POST is in flight at a time,
  and changes made meanwhile coalesce into a single follow-up that reads the
  form when it is sent, so rapid switching always persists the last choice
  A follow-up still waiting when the learner leaves or hides the page
  (`pagehide`, or `visibilitychange` to hidden) is sent at once as a
  url-encoded `keepalive` fetch (a few dozen bytes, far under the 64 KiB
  keepalive budget) and cleared, so it is sent exactly once.
- **Theme save ordering: per-tab `(theme_tab, theme_seq)`.** The keepalive
  flush can reach Scry before the same page's earlier save (that request held
  up on the way). Each Settings page load makes one tab token
  (`crypto.randomUUID()`, with a random fallback) and counts its choices from
  1; every scripted save, the keepalive flush included, carries the pair
  frozen when the choice was made. `users.theme_tab` / `users.theme_seq`
  store the last scripted save's pair, and `SetUserTheme` applies a scripted
  save only when it comes from a different tab or carries a strictly higher
  number than that tab's stored one, so within one tab an older or replayed
  save is ignored (and answered with the usual 303). A plain no-JS Save sends
  neither field, always applies and clears the pair. Nothing uses a clock,
  `sessionStorage` or `localStorage`, so devices with skewed clocks number
  their saves as well as any other.
  Why per tab, not account-wide (Pass 5 used a server-seeded account-wide
  sequence): two Settings tabs opened at the same time are, on the wire,
  indistinguishable from one delayed request, so an account-wide number
  rejected the second tab's later, genuine choice (Tab A saves Dark, Tab B,
  already open, picks Light: Dark was kept). The ordering we must protect is
  within a tab (a page's own `fetch` vs its own `keepalive` flush); across
  tabs, devices and no-JS saves Scry uses ordinary HTTP last-writer-wins, so
  a request from one tab held up behind another tab's later save can still
  land last. That is the accepted boundary.
- **"/" on phones.** The Library outline (and its search) is hidden on
  phones, so "/" opens the idea picker and focuses the picker's own search.
- **Long answers on phones.** The block caret's hidden measuring ruler is
  clipped inside the answer line (`overflow: hidden`), so a typed sentence no
  longer widens the page (it was ~929-1903 px) or pushes Check off screen.
- **Hover** rules (fills and colour changes) now live in one
  `@media (hover: hover)` block; focus styles are unchanged.
- **Password reset in test mode.** The reset flow is Clerk's (live and
  clerk-test auth). Local test-mode auth has no reset, as before; its Sign in
  page is unchanged, and the new "email first, then Forgot password on the
  password step" copy is on the Clerk page and in clerk.js.
- **Clerk theming** reads Scry's resolved colour tokens at mount time (so dark
  mode gets dark panels) and passes them as Clerk appearance variables and
  elements, with shadows and background images turned off.
- **Waiting screen at 1440×900.** The session no longer reserves a
  viewport-tall minimum height on desktop and the study page's section padding
  is 48 px; with the margin top-aligned there is nothing to centre. Measured
  on the running app: scrollHeight 900 at a 900 px window (was 918).

### Slice 106 [S106-1]: the landing Specimen steps

- **The prompt "Hanlon's razor".** DESIGN.md names the prompt; the starter
  card asks the razor's definition and answers "Hanlon's razor". The Specimen
  asks it the other way round: the correct answer is the starter card's own
  wording ("Never attribute to malice what is adequately explained by
  carelessness or stupidity"), the typed answer says the same in other words,
  and the why is the grader's plain sentence for Correct. The place is the
  spoiler-safe "Mental models · Set 1" a session shows.
- **Step controls without script.** They are rendered (3 · Judged marked
  current, aria-pressed) but `disabled` until app.js wires them, so no page
  shows a control that does nothing; with script they are ordinary buttons
  reached by Tab.
- **Timing.** One stage every 4 s (inside 3.5–4.5 s), 200 ms ease-out
  cross-fade. Hover or focus within holds the loop; leaving resumes it; a
  chosen step stops it for the page's life. The loop also rests while the tab
  is hidden. Under reduced motion it never advances, but the steps still work.
- **Calibrated ticks.** A short 8 px tick every 22 px (the session's) plus a
  long 14 px tick every fifth (110 px), `static/tick-major.svg`.
- **Prompt and Answer stages** keep the Judged stage's height (one grid
  cell); their one-line hint sits at the foot of the cell, so the panel never
  changes size while it steps.


## Slice 70: iPhone dogfooding polish on the Study review (Folio 54beb48 → this PR)

PRODUCT.md and DESIGN.md were updated first in this PR (the intent commit);
the code follows them. Choices where the brief or the Folio left room:

- **Skips stay graded (G10 kept).** The brief asked that blank cards say "No
  answer given." with feedback such as "Skipped — review the answer above."
  The G10/D3 remediation removed every local empty-answer verdict (AGENTS.md:
  no heuristic decides correctness), so an empty answer still goes to the
  grader once. Only the words change: "No answer given." where the answer
  would be, and, when the verdict is Incorrect, "Skipped — review the answer
  above." as the why (exact texts answered with nothing read the same way).
  A skip keeps Ask Why? (it was model-graded), as PRODUCT.md promises for
  every model-graded answer.
- **Plain whys, stored criteria.** The grader still returns, and Scry still
  stores, its criterion sentence on the review (the explainer is sent the
  question, expected answer, answer and verdict, not the criterion). The
  review, the Ask Why? fallback, the card history and the landing specimen
  show a fixed plain sentence per verdict (`ai.PlainWhy`), chosen in
  DESIGN.md's table. A stored reason that is not a criterion (an exact text's
  word count, seeded or legacy reasons) is shown as before.
- **Current card only.** In the review the scrollback and the "coming up"
  list are not rendered at all (server-side), not hidden with CSS, so no
  earlier or next prompt is in the page. While a card waits they are as
  before, and the spoiler rules still apply to them.
- **One clock for the schedule.** The observed "Nov 23 · today" beside "1
  day" came from mixing a calendar-day relative label with an elapsed-time
  interval (most likely a review left open and reopened the next day, or a
  learner whose stored zone was UTC: Clerk sign-ups never stored one). Both
  are now measured from the stored instants on one clock (`spanLabel`),
  rounded to the nearest hour first: minutes under half an hour, hours from
  1 to 21 rounded hours, whole days (rounded) from 22 rounded hours, that
  is from 21 h 30 min. The event reads "Tue, Nov 24 · in 1 day" (or "in 4 hours", "due
  now"); the interval reads "1 day after this answer" ("after the last graded
  answer" after a practice attempt). `<time datetime>` now carries the full
  RFC 3339 instant.
- **The stored time zone stays the basis; no page view writes it.** The
  first pass added a background `POST /settings/tz` from every signed-in
  page whose browser zone differed. The Cold Critic showed it rewrote the
  account-wide zone from whichever device viewed a page last, flipping the
  review's date and the Study queue (`app.EndOfDay`) between a Chicago and
  a Tokyo browser. It is removed (route, handler, layout attributes and
  script). The dates use the zone stored at sign-up or when the study
  profile is saved, as before; the elapsed labels need no zone at all.
  TestSlice70_NoPageViewChangesTheTimeZoneOrTheQueue pins it. A learner
  with no stored zone (Clerk sign-ups) sees UTC dates until they save the
  study profile; a display-only or explicit zone setting would be its own
  slice.
- **"Review streak".** PRODUCT.md rules out streaks, so the "review streak"
  telemetry is the card's factual verdict trail: up to its last eight graded
  verdicts, oldest first, as 14 px verdict icons with their words for screen
  readers, beside the existing counts. No run is counted.
- **Next card in ink.** The electric blue (#2563eb as observed on the phone;
  the accent token is #0562EF) is replaced on Next card by an ink-filled
  control (`btn-ink`: ink ground, page text, both modes). Check keeps the
  accent: it was not part of the report and stays the waiting screen's one
  accent action.
- **Focus ring.** iOS never focuses a tapped button, so the answer field's
  keyboard-style focus state carried over to the programmatically focused
  Next card and WebKit drew the accent ring around its fill (the "autofocus
  double ring"). app.js now marks keyboard navigation (`html.kbd-nav`: Tab,
  or arrows/Enter/Space outside a field; cleared by any pointer or touch) and
  the ring is drawn only then; without script the browser's `:focus-visible`
  rule applies. Text fields show one 2 px ink edge, never a border plus an
  accent ring.
- **Phone nav.** All six destinations sit in one 52 px row of equal columns
  (16 px icon over an 11 px mono label), under a 48 px brand row: 101 px in
  all, about the same as before, with nothing off-screen. On the study
  session the rail's "Study / Recall what's due today" is hidden on phones
  (the nav already says Study); the place line and count stay. Empty
  Study's "Study · Recall what's due today" kicker is visually hidden on
  phones too (kept for screen readers); its h1 "No cards to study yet."
  stays visible. Desktop keeps
  its 196 px rail unchanged.
- **Bar clearance.** The phone's pinned Check / Next card bar is 77 px plus
  the safe-area inset; the session page now pads its body by that plus 24 px
  (it padded `.session` by 96 px, which left the footer under the bar).
- **WebKit CSP reports in automation.** Playwright's WebKit screenshot injects
  an inline `<style>` (even with `caret: "initial"`), which Scry's
  `style-src 'self'` refuses and reports; one report per screenshot, none
  outside screenshots. The policy is right; the verifier sets those aside as
  tooling.


## Slice 75: the Strugglometer (bridge cards for repeated misses)

Owner brief (2026-11-24). Choices where the brief left room:

- **Detection.** Deterministic, from stored state only: FSRS `lapses >= 3`,
  or the latest two graded reviews (practice attempts on exact texts left
  out) both Incorrect (`missed`). Almost Correct breaks a run of misses.
  `app.Struggling` holds the rule; tests pin both boundaries.
- **Where the cue shows.** "Cards detail/deck list" are Scry's card page
  (/library/items/{id}) and the idea's item rows in the Library (and the same
  rows in search results). The post-answer review shows it after the answer
  that made the card struggle; the waiting Study screen never does (no
  spoilers, no nudge before answering).
- **Not offered.** Exact texts (word-for-word lines have no prerequisites;
  "Again" is their remedy), a card already held, and an active bridge card
  (bridges do not nest). `Struggle.Offer` holds the rule; the routes refuse
  the rest (a crafted POST cannot spend allowance on a calm card).
- **Generation.** `ai.Bridger`. In production it is the same drafting client
  as Create (`*ai.Claude`; on OpenRouter the Slice 36 empirical winner
  `openai/gpt-6.2-sol`, `SCRY_DRAFT_MODEL` overrides; or the direct Anthropic
  drafting model), one bounded non-streaming call (45 s, 1,200 output tokens,
  6,000 input bytes) with the parent's question and answer and up to
  three recent Incorrect answers as untrusted data. The reply is a JSON
  object of 2–3 `{q, a}` cards; repeats, empty or over-long cards are dropped
  and fewer than two is an error. It is reserved and settled as `draft`
  usage, with the same G2 rule (an outcome unknown after the call keeps the
  hold). In `SCRY_MODE=test` it is `ai.FakeBridger`: three fixed, labelled
  fixture cards built from the parent's question only, no network;
  `SCRY_TEST_BRIDGES=fail` shows the failure state. The fixture is
  unconditional in test mode: `SCRY_AI_MODE=live` (or `SCRY_JUDGE_MODE=live`)
  on a test build moves grading, drafting, explanations and speech to the
  real providers but never bridges (pass 2; `cmd/scry`
  `TestSlice75_TestModeBridgerIsAlwaysTheFixture` traps the network).
- **Spoilers.** The production parser (`ai.ParseBridges`, on both drafting
  routes) drops, before the 2–3 card cut, any drafted card whose question
  repeats the parent's question or whose question or answer contains the
  parent's whole answer, both compared as normalized words (case folded,
  punctuation and spacing ignored, whole words in order: "Bravo" is found in
  "B is Bravo." but not in "Bravos"). Partial overlaps stay: a bridge may
  share a word with the card it leads to. Fewer than two safe cards is the
  plain failure, never a spoiler. The learner's own edits are not filtered.
  An answer with no letter or digit (a code or math operator: "!=", "->",
  "≠") has no words to compare, so it is matched literally instead (pass 3):
  the trimmed answer, inner spacing collapsed, anywhere in the bridge's
  question or answer is a spoiler, so "Which programming languages use != to
  mean not equal?" is dropped for a parent whose answer is "!=". Answers with
  any letter or digit keep whole-word matching unchanged.
- **Long text.** A bridge question may be 400 characters with no space in
  it; every surface showing one (bridge links, the bridge's page title, deck
  rows, Study) wraps it in place (`overflow-wrap: anywhere`, `minmax(0, 1fr)`
  tracks). `.title`, `.item-q` and `.item-a` wrap the same way everywhere,
  which changes nothing for text with break opportunities.
- **Preview state.** The drafted cards are not stored: they ride in the
  preview form, which carries a random accept key. Reloading the preview
  drafts again (and is metered again); the learner's edits are the cards.
- **Persistence.** One additive table, `item_bridges` (created with
  `CREATE TABLE IF NOT EXISTS` on every open, plus one index; no existing
  table or column is altered, so a pre-slice database opens with its rows and
  schema unchanged). One row per bridge: parent, bridge item, accept key and
  position (unique per learner), `ended_at` and `end_reason`
  (`mastered`, `released`, `removed`). It is a learner table
  (`store.userTables`: export, per-user restore and the privacy walk include
  it). Accepting runs in one `BEGIN IMMEDIATE` transaction: it checks the
  parent is the learner's own live card, replays an already-applied key as a
  duplicate (nothing written), refuses a second set for a held card or a
  bridge-of-a-bridge, then inserts the items and rows. Concurrent accepts of
  one preview write it once; of two previews, one wins.
- **Hold.** A card is held while it has an active row whose bridge item and
  its idea are not removed. Every due path applies the same `notHeld` SQL:
  `DueItems`, `StartSession`, an open session's entries (so a hold made
  mid-session takes the card out of it), the answer guard, the due count and
  `NextDue`. The parent's row in `items` is never written by a hold or its
  end, so it is due exactly when its schedule says.
- **Release.** Each bridge's first non-practice Correct or Almost Correct
  ends its row (`mastered`) in the same transaction that records the review;
  the parent is free when none is left. "Release hold" ends all its rows
  (`released`); the bridges stay as ordinary cards. Removing a bridge card
  ends its row (`removed`); removing a held parent ends its hold. Trashing
  the idea (which holds both) pauses the hold, since a trashed bridge does
  not count; restoring it restores the hold of bridges not yet mastered.
- **Bridges' place.** Bridge cards join the parent's idea (so ownership,
  outline, export and Trash all follow it) with source `bridge`; as new
  cards they are due at once and come after reviews in the queue, oldest
  first, like any new card.
- **Mobile width.** The brief's ">= 366 px card width at 390 px" is met by
  the bridge panels and cards running 8 px into the 16 px phone gutter (374
  px wide at 390 px); the page column itself stays 358 px like every other
  page.
- **Verification data.** `scripts/verify-slice75-strugglometer.mjs` starts
  its own test-mode server per configuration on a fresh SQLite file with a
  minimal environment (no keys), signs up through the UI and seeds two cards'
  history with `node:sqlite` (a synthetic past); "time travel" moves a card's
  last review, due date and review times back together. Its CSP gate is a
  run without screenshots: Playwright's WebKit screenshotter always inserts
  an inline `<style>` (its animation sync; no caret, animations, scale or
  fullPage option avoids it), which Scry's CSP refuses. A media run
  (`--shots`) captures Chromium under the enforced CSP (zero violations
  required) and WebKit with Playwright's `bypassCSP`. `--traces` writes
  scoped JSON only (routes, statuses, timings, assertion results, counters),
  checked free of the run's email, random password, cookie values, form
  tokens and auth field names before it is written; no Playwright archive.
- **Pass 4 (CodeRabbit review on db9624d, each reproduced before fixing).**
  The reply is decoded one JSON object at a time (`json.Decoder` from each
  `{`, at most 32 tries), so a cards object followed by another object
  parses; the first object with a `cards` list wins. A failed parse settles
  only usage the provider reported: the fallback model name is set on a
  clean success only, so an unparseable reply with no usage keeps its
  reservation pending (G2). The request is fitted before any budget is
  reserved (`ai.FitBridgeInput`: at most three misses, the oldest dropped
  until it fits 6,000 bytes; the question and answer always fit within the
  card limits), and a request refused before dispatch (`ai.ErrBridgeNotSent`)
  releases its reservation, while an ambiguous failure after dispatch still
  keeps it. Drafted cards are kept only within 400 characters and the card
  limits in bytes (1,000 for a question, 2,000 for an answer, pinned to
  `app.MaxPromptLen`/`MaxAnswerLen` by a test), so every previewed card can
  be accepted unchanged; acceptance copy names bytes. "Release hold" ends
  only bridges that hold the card now (the hold predicate, in the same
  statement), so a stale release after trashing the idea is a no-op and the
  hold returns on restore. A Build form restored from the back/forward cache
  is given back (pageshow `persisted`, like the busy forms). The verifier
  refuses an unknown or empty `--only`, and the README commands run as
  written. Not changed: CodeRabbit's note on screenshot errors being set
  aside was about 4c15945; since pass 2 nothing raised during a capture is
  set aside (WebKit media runs use `bypassCSP` instead and say so).
- **Pre-existing test flake.** `TestReserveRacingPlanEnd` (internal/store)
  fails with SQLITE_BUSY on this sandbox's disk on master as well as here;
  with `TMPDIR=/dev/shm` the whole suite passes. Not changed by this slice.

### Slice 75: independent follow-up

- An exact, whitespace-normalized repeat of a symbol-only parent question is still a repeat; simply using the operator in a different question is not. The production OpenRouter adapter is covered by a loopback regression.
- Omitting `--only` runs all browser configurations; explicitly supplying no name (including an empty string) or supplying the option twice is a usage error.

### Slice 75: the bridge review names its parent

- DESIGN.md (Held) says "A bridge card's page and review say 'Bridge card · leads up to …'". The card's page linked the parent; the post-answer Study review built its bridge view without it and said only "Bridge card." Study now builds the view with the same `bridgeViewFor` the card page uses (a read through the learner's scoped store), so an active bridge's review links the parent card's question to its page. Only the question is shown, never the parent's answer.
- Only after the answer: the waiting bridge carries no bridge note and no parent link, as before (the bridge view is built only for a judged card).
- An *active* bridge only. Once a bridge is answered Correct or Almost Correct it has ended, and its review says "Bridge cleared · …" (what clearing it changed) instead, as DESIGN.md describes, without the "leads up to" line. A practice re-attempt on an active bridge shows the same line as its first review.
- No schema or persistence change: no migration. The parent is read with the existing scoped `Item` query, so another learner's card can never be named (that read is a not-found for them) and the parent's page is a 404 for anyone else.

### Slice 106: No Inbox (S106-4), Create from Markdown or prose (S106-2)

- **No Inbox.** The Inbox page, its template, its count badge, its icon and `g i` are gone; navigation is Study, Library, Create, Billing, Settings (five equal columns on phones). `/inbox` and everything under it (`POST /inbox/approve-all` included) answer 303 to `/library` for any method, signed in or not (Library itself sends a signed-out visitor to Sign in). The "API tokens" note under Slice 41 above is superseded.
- **Old suggestions.** Every agent suggestion still pending (`draft_id = 0`, `status = 'pending'`) is approved into its learner's library when the store opens (`store.MoveInboxToLibrary`, called from the idempotent `migrateAdded` path), one learner per transaction, with the same `approve` logic as a tap on Approve (idea created or joined, source line kept). Each learner moved is logged at Info ("moved pending inbox suggestions into the library", user, count). A learner whose move fails is logged at Error (which alerts) and retried on the next open; it never stops Scry from starting. Dismissed suggestions stay dismissed; draft items (`draft_id ≠ 0`) still wait in their outline, and `/suggestions/{id}/approve|dismiss` keep working for them (an old draft-0 suggestion, if one were ever left, returns to Library).
- **Every token adds.** `POST /api/v1/items` and `POST /api/v1/suggestions` are one handler: 1–50 items, all or none in one transaction, `201 {"added": N, "ids": [...], "status": "in the library"}`. The old `"suggested"` response shape is not kept: the alias answers exactly as `/items` does, because the cards are in the library, not waiting. `can_add` is no longer read anywhere (old `can_add = 0` tokens add); new tokens write `can_add = 1` only so the column never says otherwise. `ErrAgentMayNotAdd` and `AgentSuggest` are removed. Refusals are 422 with a reason naming the item ("item 3 needs an answer", "send at most 50 items per request; this one had 51"); a body that is not the expected JSON stays a 400 showing the shape.
- **Settings → API Tokens** has the DESIGN.md copy, "Token name", "Create token" and a `curl` example for `POST /api/v1/items`. The example carries the new token only on the response that created it (inside the "Copy it now" box); otherwise it shows `Bearer scry_…`. A stored token is never printed (Scry keeps only its hash). Token rows say "created … · never used / last used …" instead of "suggests to inbox".
- **Demo data.** The seeded demo's seven agent suggestions are now cards those tokens added to the library (through the same `AddFromAgent` path), so the demo never shows a dead queue.
- **Create textarea.** One `<textarea>` (`rows="6"`, `maxlength="16000"`, Body type, bordered like other fields so the placeholder never clips at 390 px), the mono hint, the example prompts, then "Draft cards". Script grows it with its content up to `max-height: 66vh` (then it scrolls); without script it is resizable. Cmd/Ctrl+Enter submits (after the browser's own validation); plain Enter is a newline. The hint line reads "⌘/ctrl + enter to draft".
- **What counts as pasted material.** A request with a line break, or over 300 characters (the old topic limit), is material (`ai.IsMaterial`). It keeps its line breaks (CRLF and CR become LF, the ends are trimmed); a one-line topic still has its spaces collapsed. Over 16,000 characters (counted as Unicode characters) is refused: "Keep it under 16,000 characters."
- **Allowance unchanged.** The draft reservation and the 32,000-byte input ceiling it is priced on are unchanged. To fit 16,000 characters inside it, the learner's library titles sent with the request are cut to the room left (never the material). Text in a script of several bytes a character can exceed the ceiling below 16,000 characters; it is refused before anything is reserved: "That's too long to draft at once. Send it in parts."
- **Draft title.** A topic names itself as before. Material is named by its first Markdown heading, or else its first non-empty line, as plain words (markers, emphasis and link targets dropped), cut at a word to at most 80 characters with "…". The model's own title is held to the same 80. The draft page labels it "from your notes".
- **Prompt.** Both routes (OpenRouter and direct Anthropic) share one system prompt; it now says that a message in material tags is the learner's own Markdown, prose or notes, to be written as atomic questions grouped by the ideas it covers, following its headings, keeping to what it says, and never treated as instructions; 1 to 12 ideas for material (4 to 7 for a topic). Material is sent between `<material>` tags with its lines; a `</material>` inside it is defused.
- **Test-mode fixture.** `ai.Fake` drafts material with no network call: with Markdown headings, a leading `#` heading names the draft, lines before the first sub-heading are dropped when there are sections, and each heading (at most four) is an idea; prose without headings makes one idea per paragraph, named by its first six words. Each idea gets up to three cards from its own sentences: "term: definition" asks "what is term?", "X is Y" asks "what is X?", a longer line asks how it ends, a short one asks for its place. A single section of several lines is split into two ideas. Material never matches a fixed book or starter topic.
- **Retry with long material.** "Ask again" / "Try a narrower topic" on a failed or empty draft link to `/learn?draft=ID`, which reopens Create with that draft's own text (read through the learner's scoped store; another learner's id fills nothing), instead of a 16,000-character `?q=` URL. `?q=` still fills the field for the example links without script.
- **No hand-written card forms.** Library's "Add an item" form, the "Add an exact text" link and page (`text.html`) and their handlers are removed. `GET /library/texts/new`, `POST /library/texts` and `POST /library/ideas/{id}/items` answer 303 to `/learn` and add nothing. Both outlines (desktop pane and phone picker) end "Starter shelf · Create or import cards", linking `/shelf` and `/learn` (the import section will live on Create). A card's own page still edits question, answer and the exact flag. `store.AddExactText` stays for starter sets, imports and tests; tests that need an exact text now file one directly.
- **Client telemetry routes.** `/inbox`, `/library/texts/new` and `/library/texts/:id` are no longer page routes; reports from them are counted as `/other`.
- **Not changed here.** The landing page's "Yours alone" row still says scripts "suggest cards … nothing is added until you approve it"; the landing page is reworked separately for S106-1. Older per-slice browser verifiers (`scripts/verify-slice41.mjs`, `verify-slice54.mjs`, `verify-slice70.mjs`) record those slices' contracts (six destinations, the Inbox) and were not rewritten; `scripts/browser-smoke.cjs` and `scripts/verify-live.mjs` follow the new contract.

## S106-5: Manage a card without leaving Study

- **Which idea a typed name picks.** Edit card's "Idea" field matches a live idea of this learner case-insensitively (spaces collapsed). When several match, the card's own idea wins, then one under the same top-level idea (the card's topic), then the oldest. With no match a new idea is created: inside the same top-level idea when the card's current idea sits inside one, otherwise as a new top-level idea (ideas nest one level, so no child is created under a top-level card's own idea). A form post without an `idea` field (an older page) keeps the card's idea, so `/study/edit` stays compatible.
- **Snooze while waiting** records no review and changes only `due_at`, to the start of the learner's next local day (`EndOfDay` in their stored time zone) if it was sooner. The card's unanswered `session_items` row in the open session is deleted, so the session's count shrinks and the "n / N" position is the card's place among the session's current entries (no gap). **Snooze in the review** keeps the recorded review and sets the next review to max(scheduled due, tomorrow's start), then clears the review as Next card does. Every graded answer is due at least 24 hours later (`srs.MinInterval`), so the max only moves a review left open across a local midnight.
- **Delete** soft-deletes the card (`deleted_at`). A waiting card's session row is kept, so Restore from the quiet line puts it straight back in the session; a card deleted in its review keeps its review in the session's count. Deleting, snoozing or restoring twice, or a card outside the open session, is harmless; another learner's card is a 404.
- **The quiet line** travels as a query parameter (`/study?note=snoozed`, `note=removed&card=ID`, `note=restored`) after a PRG redirect; "Restore" is shown only while that card is the learner's and still removed. When Snooze or Delete empties a session with no answer in it, `/study/done` hands the line on to `/study`, which shows the next session's card, "Nothing due." or the empty-library page with the line: never a loop. "Card restored." is the line after Restore (DESIGN.md is silent on it).
- **Recently removed** lists removed ideas and cards removed on their own (whose idea is live) in one list, newest first; cards removed with their idea come back with it and are listed under the idea. Cards are kept 30 days by the same sweep that purges ideas (`Store.Sweep`). Restoring a card uses the existing `POST /settings/restore/{id}` with `kind=card`.
- **Keys.** E (review only) opens Edit card and focuses Question; S snoozes in the review, and while a card waits only when focus is outside a field (after Escape), so typing an "s" in the answer is never taken. Neither acts with a modifier or after another binding claimed the key ("g s" still goes to Study). The hint line stays "enter or space for next card" as DESIGN.md writes it; the buttons carry `aria-keyshortcuts`.
- **Layout.** Snooze is the `snooze` icon (a crescent moon), Delete is `bin`. On phones Edit card, Snooze and Delete share one row of equal columns in the flow, above the pinned Next card bar's clearance; when Edit card is open it takes its own row and Snooze and Delete sit together under the form beside Next card. The edit form uses `novalidate` so an empty field is refused in Scry's words under it rather than by a browser bubble.

## S106-3: Import and export decks (web wiring)

- **Where.** "Import a deck" is a section of Create (`/learn#import`, `POST /learn/import`, multipart), after a 1 px rule; Library's outline ends "Starter shelf · Create or import cards" linking `/learn#import`, and Settings → Your data links to it.
- **Idea title.** Optional. When given it is the idea for every card that does not name its own: CSV rows of two columns, `---` card blocks, and an Anki package's cards (which otherwise take their deck's name). CSV rows with an idea column and Scry's Markdown export keep their own ideas. With no title and no idea in the file, the file's name is the idea ("spanish-verbs.csv" → "spanish verbs").
- **Unstructured Markdown** goes through the same path as text pasted into Create (`App.Learn`): a draft outline, drawn from the allowance like any draft; over 16,000 characters it is refused in words. The draft page says "Sent to drafting: …" once.
- **Size.** A request whose Content-Length is over 20 MiB + 1 MiB is answered 303 to `/learn?import=toolarge#import` before anything reads it (a redirect has no side effects, so it needs no CSRF token); bodies of unknown length are capped as they are read. The page script refuses a file over 20 MiB in words before sending it.
- **Refusals** are shown in words under the form with status 200, not 422: an expected outcome, and a 4xx document logs an error in the browser console (the same choice as Edit card's JSON refusals).
- **Downloads.** `GET /settings/export?format=md|csv|json` (JSON stays the default for `/settings/export`), available on any plan state (`authed`, not `member`). Markdown and CSV are every live card of a live idea in outline order; a card in a sub-idea carries the sub-idea's title, so a re-import files it under a top-level idea of that name. Importing an export into the same library adds the cards again (no de-duplication).

## S111-3 / S113-1: Model-only grading chain

- **Two kinds of "no verdict".** Jev now classes every failure: `ai.ErrDecisionUnavailable` (unreachable, any non-200, unreadable JSON, missing usage or cost, refusal or another answer type, an unknown choice, confidence or probabilities out of range or not summing to 1) sends the chain to the next decisions model; `ai.ErrDecisionUncertain` (confidence < 0.5, the chosen probability < 0.8, margin < 0.2, or `none_of_these`) escalates straight to the LLM. A failure before any request left (no key, state over 8 KiB) is also `ai.ErrNotSent`, so it costs nothing. With the 0.8 probability bar and probabilities that sum to 1 the margin bar can only bind together with it; it is kept for models that report probabilities without a winner bar.
- **The orchestrator is the same in every mode.** `ai.Chain` (primary, fallbacks, escalation) runs in production and, in test mode, inside `ai.Fake` over fixture steps, so the SCRY_TEST_JUDGE_PATHS paths exercise the real ordering and escalation rules. The fixture path and verdict are chosen per submission before the chain starts and never from the answer; the fixture's token counts still scale with the input length, as before, for metering only.
- **Escalation when the escalation is unsure.** The LLM is the last step, so a reply it cannot give cleanly (unreadable, refused, cut off, outside the three verdicts) is no verdict; the chain never asks a second LLM or loops. With no escalation configured (SCRY_JUDGE_MODE=live on Typesafe without ANTHROPIC_API_KEY) the chain ends with the decisions models, with a boot warning.
- **Unset versus empty.** SCRY_JUDGE_FALLBACK_MODELS unset means the two defaults; set but empty (or only commas and spaces) means no fallbacks. SCRY_JUDGE_ESCALATION_MODEL empty means the default. Entries are trimmed; blank entries are skipped.
- **Without OpenRouter.** The fallback and escalation settings are OpenRouter slugs, so on the direct route they are ignored with a warning rather than stopping the boot: the chain is Jev on Typesafe alone and the escalation is the drafting model (SCRY_DRAFT_MODEL, default claude-opus-5-5) on Anthropic, sent with `output_config: {effort: "low", format: json_schema}`. Fallback decisions models share the primary's OpenRouter key, transport and log.
- **Escalation request.** OpenRouter chat completions with `response_format: json_schema` (strict, enum of the three verdict ids), `provider.require_parameters: true` so only a provider that honours the schema serves it, `max_tokens` 1024, and the same JSON state Jev gets (card_prompt, accepted_answers, learner_answer) as an untrusted user message. The rubric is built from the same criteria sentences; the reason recorded is the chosen criterion's fixed sentence, as for Jev.
- **Timeouts.** Each decisions model has 8 s and the escalation 15 s; the app's judging deadline rose from 25 s to 45 s so the whole chain fits (the server's write timeout is 90 s). A request interrupted mid-chain stops the chain at once.
- **One charge.** `app.JudgeBudget` is the sum of every model in the chain at its rate (one reservation of the costliest path), and the chain returns one Usage with every attempt (`Usage.Attempts`). `app.Cost` sums them: reported cost when known, nothing for an attempt never sent, and the model's worst case for an attempt whose reply was lost (sent, no usage), so a lost reply is never unmetered. If the chain fails with any such attempt the whole hold is kept (G2), as for a bare Jev. A single attempt is reported exactly as before. `Usage.Attempts` is a pointer so `ai.Usage` stays comparable.
- **Prices not read from the catalog.** No network was used, so the default fallback models (openai/gpt-6-luna-decisions, google/gemini-decision-1) are priced at $0.10/$1 like jev-1.14's margin, and google/gemini-4-hydrogen at $1/$5; the default chain reserves 20,174 micros (about $0.02). The four test fixture models are priced like test-judge, so a test-mode hold is 5,736 micros (TestSlice36_TrialGradesWithAModelAwareHold now leaves 6,000).
- **/healthz.** A new `grading_chain` object (`primary`, `fallbacks`, `escalation`, `escalation_provider`); `models.grading` stays the primary, `providers.grading` stays jev-openrouter / jev-typesafe / test, and `grading: true` still means the primary is real Jev.
- **Logs.** Each attempt logs `judge attempt` (step, position, requested and resolved model, latency, cost, tokens, outcome) and each walk `judge chain` (path, why it escalated, which model gave the verdict); Jev's own `judge decision` line gained `outcome`. None carries the answer, question, a provider error body or a key.
- **"Couldn't check" status code.** When every path fails Study still renders the full study page with the copy and the kept answer at HTTP 503, as before (existing tests rely on it, and the client swaps the page whatever the status). No fallback or escalation path produces a 503.
- **Lost response.** Already held: a recorded review leaves the session in PhasePendingReview, so a repeated submit is ErrStale before anything is reserved or judged, and Study shows that review. Now proved by test.
- **Line breaks (S111-2, server side).** `app.NormalizeAnswer` keeps line breaks: CRLF and CR become LF, spaces and tabs within a line collapse, runs of blank lines become one, the whole is trimmed and cut at 2,000 bytes on a rune boundary. The exact-text diff still splits on all whitespace, so a line break is a space there. How the review displays them is the template's concern.
## S111-1: Punch in and study, one interleaved session across every deck
- **Deck.** A deck is a top-level idea (a starter set, book or topic); a card's deck is its idea's parent, or its idea when that is top-level (`COALESCE(parent_id, id)`). Cards the API or an import files under a new top-level idea make that idea its own deck.
- **Which cards are eligible.** A due review is a card reviewed at least once (`reps > 0`) and due before the end of the learner's day, as before. A new card "I am ready to see" is one never reviewed (`reps = 0`) whose `due_at` has come by the end of the day: so a snoozed new card waits for tomorrow. Cards held by bridges, removed, or in a removed idea are never eligible (the `notHeld` guard as before).
- **The 200 candidates.** SQL cannot rank by FSRS retrievability, so the 200 candidates are the 200 earliest-due reviews; within them the plan ranks by current retrievability (`srs.State.RetrievabilityAt`, the FSRS-4.5 forgetting curve at the time since the last review), then earliest due, then id. A review due after the first 200 waits even if its retrievability is lower. A reviewed card with no stored last review or stability reads retrievability 0 (most urgent).
- **Filling the budget.** Room goes to due reviews by urgency, then to new cards oldest first (`created_at`, then id). A card that no longer fits the remaining budget is passed over and the next one that fits is taken (an ordinary 20 s card can take the last 20 s an exact 30 s line could not); the first card always fits, so a tiny configured budget still studies one card. A new card never takes room a due review could use, so "a new card used to break a streak never takes a review's place" holds by construction.
- **Ordering.** Next is the most urgent remaining review from a deck other than the last card's; else the oldest remaining new card from another deck; else (no other deck has a card left) the last deck again, its reviews first. The order is fixed when the session starts and stored as positions, so the existing session state machine (Answer, Next, Snooze, Delete, Edit, the guards) is unchanged.
- **Exact texts keep their order.** Lines of one exact text are those with the same idea and text title. When a line wins a slot, the earliest remaining line of the same text takes that slot instead, so the lines that are in the session always come in line order, even when a later line is more urgent.
- **Counting first presentations (the 10 a day).** A new table `first_presentations (user_id, item_id, day, session_id, at)`, keyed by learner, local day (`YYYY-MM-DD` in the stored time zone) and card. `StartStudy` writes a row in the same transaction that places a new card in a session, recounts the day inside that transaction and skips new cards over the limit, so two tabs starting sessions at once cannot exceed 10; a card already presented that day is free (no second row). A row is taken back only for a card the learner never saw: when a session is replaced (Study this idea, Study all decks) its new cards still waiting behind the card on screen are released, so switching sessions does not spend the day on unseen cards; the card on screen, and any card answered, snoozed or deleted, stays counted. A row keeps the session that first presented the card, so a later session replacing one that re-placed it never releases it. Rows cascade with their card and learner, are listed in `store.UserTables` (exported, restored per learner), and the table is additive (`CREATE TABLE IF NOT EXISTS`). The count covers focused and interleaved sessions alike; the day turns at the learner's own midnight. Sessions started with the older `StartSession` (tests) record nothing.
- **When today's new cards are spent.** If nothing is eligible but cards are due (only new cards are left over the limit), the rest page says "That's today's 10 new cards. More come tomorrow." instead of "Your next review is due now." (PRODUCT.md is silent on the wording).
- **Focused sessions and `/study`.** "Study this idea" is a quiet text link beside the idea's title in Library (the Study icon and the words), `GET /study/idea/{id}`: it starts a session over that idea and the ideas inside it (scope stored on the session, `study_sessions.scope_idea_id`) and redirects to `/study`; another learner's idea is a 404; with nothing eligible the idea's page says "Nothing in this idea is due today." and the open session is left alone. Plain `/study` (Study in the navigation, `g s`, and the focused rail line's "Study all decks") always opens every deck interleaved, as DESIGN.md says: a focused session open from Library gives way to one across every deck (its unseen new cards are released, so the daily limit is not spent on them). Study's own actions (answer, Next, Snooze, Delete, Restore, Edit card, Ask Why?, bridge cards) redirect to `/study?on=1`, which goes on with the open session whatever its scope, so a focused session is never dropped mid-card by the app itself; a finished focused session is followed by an interleaved one. `/study?on=1` is internal: it only continues, it never opens a focused session.
- **The deck tag.** The waiting card's tag is its existing spoiler-safe place line in the rail (on phones above the prompt), now marked `data-deck-tag`. In an interleaved session the cards beside it come from other decks, so the tag is also checked against the answers of the cards listed as coming up (at most three): a topic holding a word of one falls back to the neutral set number, "Current set", or no tag. The focused line is checked against every answer in the focused idea and the session, and reads "Focused" alone when the topic is not safe.
- **Bridge cards** are new cards like any other and count toward the 10 a day; once today's limit is spent they come the next day.
- **Demo seed.** The demo's kitchen numbers set now has two due reviews (one due today, one missed yesterday) among its new cards, so due reviews are spread unevenly over three decks (NATO, mental models, kitchen numbers) and new cards over several (those three, the Creeds and the API-added ideas).
- **Quint.** `specs/study_session.qnt` models the session state machine (phases, answer and advance races) with "every due item" as the session's contents; the selection rules above change only which items a session starts with and in what order, so the spec was not changed. Modelling the plan (budget, urgency, deck alternation, the daily count across sessions) is left for a later pass.
- **Headed verification** (xvfb-run, desktop Chromium at 1440×900 and 390×844) was not run in this change; the rules are proved by the Go tests named in README's story table.
## S113-3: One mark everywhere, the S3-A deck of cards
(Superseded by [S125-1] below: the S3-A deck is retired; these notes record what it was.)
- **One source, redrawn by a script.** `internal/web/brand.go` holds the 24 px header drawing; `scripts/render-brand.py` writes every other vector (served ones into `internal/web/static/`, raster-only sources into `internal/web/brand/`) and rasterizes them with `rsvg-convert` (PNGs stripped of timestamps, so a re-run is byte-stable; `/favicon.ico` is ImageMagick's pack of the two hinted PNGs). The PNGs are committed; there is no build step.
- **"Outline 1.5 px" on an "identical 14 x 10" card.** A centred SVG stroke would make the receding cards 15.5 x 11.5 outside and put every edge on a half pixel. Each receding card is instead an ink-step 14 x 10 card (rx 1.5) with a `page` face inset by 1.5 px (square inner corners), so the three cards share one outline and every edge is on a whole device pixel at 2x and above. The rectangles in the markup are DESIGN's numbers exactly.
- **Ink steps as opaque colours.** "ink-muted" and "ink at 35%" are mixed over `page` (`color-mix` in the header, precomputed hex in files: light #6D7073 / #AAABAD, dark #A09FA3 / #65646A), so a receding card really hides the one behind and the favicon stays opaque on a transparent tab.
- **Prompt bar.** "2 px tall with round caps, from x 5.5 to 12" is read as the bar's whole extent including the caps: a 6.5 x 2 rect with rx 1 at y 13.5, centred on y 14.5.
- **Pixel hinting.** At 16 and 32 px (and the header on a 1x screen) the deck is redrawn on its own pixel grid rather than scaled, with `shape-rendering="crispEdges"` and integer coordinates so no pixel is anti-aliased (the test checks every pixel is a palette colour or transparent). Small corners are one notched pixel. 16 px: cards 10 x 7 stepped 2 px, 1 px outlines with 1 px gaps, bar 5 x 1 three pixels clear of the card's top and bottom. 32 px: cards 18 x 13 stepped 3 px, 2 px outlines 1 px apart, bar 9 x 3 with notched caps. 24 px at 1x: cards 14 x 10 stepped 3 px, 2 px outlines 1 px apart, bar 7 x 2 from x 5 to 12. The deck sits one pixel high where its height is odd (lighter top, heavier bottom card).
- **Header at 1x.** DESIGN's 2.5 px steps fall between pixels on a 1x screen and blur, so the inline SVG carries both drawings and `@media (max-resolution: 1dppx)` shows the hinted one; every denser screen (and a browser without the style sheet) shows the exact one.
- **Favicon links.** `<link rel="icon" href="/favicon.ico" sizes="32x32">` (light, hinted 16 + 32 PNGs, for browsers without SVG favicons) then `/static/favicon.svg`: the hinted 16 px drawing with a `prefers-color-scheme` palette, crisp at 16 and 32. `/favicon.ico` is now served (image/x-icon), no longer a redirect. The new file name also retires browsers' cached lens favicon. Dark PNG favicons are not linked: `media` on icon links is unevenly supported and could outrank the adaptive SVG.
- **App icon.** 64 px, a sampled superellipse (|x|^5 + |y|^5, 96 points; continuous curvature, no arcs) filling the canvas; light `#FFFFFF` with a 1 px `line` edge (#E2E2E3, ink 12% over white) drawn just inside; dark `#12101A`. The mark is 40 px (scale 5/3), hinted to whole pixels at 64: cards 24 x 17 stepped 4 px, 2 px outlines, rx 2.5, bar 11 x 3. `app-icon.svg`, `app-icon-dark.svg` and their 64 px PNGs are served; the dark SVG is the manifest's vector icon.
- **Home screen, maskable, apple-touch, social preview.** The "any" icons (192, 512) are the dark squircle on transparency. The maskable icons (192, 512), the 180 px apple-touch icon and `og:image` use the full-bleed dark ground (`internal/web/brand/icon-maskable.svg`): platforms apply their own mask (iOS fills transparency with black), and link previews crop to their own shape. The mark spans at most ~20.6/64 of the radius from centre, inside the 25.6/64 safe circle; the test checks every mark pixel.
- **Study icon.** Unchanged: the `cards` line glyph (two outlined cards, `fill="none"`, `currentColor`), never the brand's three filled-and-outlined cards.
## S111-2 and S113-2: the multiline answer field, waits and motion (front end)
- **Measuring without collapsing.** The answer field's height is read from a hidden twin `<textarea class="answer-input answer-measure" rows="1">` beside it (same class, the field's own pixel width, `height: 0`, `visibility: hidden`, `inert`, no name): height = twin `scrollHeight` + the field's top and bottom borders, capped at the field's computed `max-height` (45vh). The visible field never gets `height: auto`, so it never flickers. The first measure is set at once; only then does `[data-grown]` switch on the 120 ms height transition, so a page load never animates from three rows to one. Re-measured on input, paste, a dictated insert, window resize and orientation change, `pageshow` and when the fonts finish loading.
- **Where "phone" starts for the hint.** The field's `aria-describedby` names the visible hint: `answer-hint` (desktop) above 760 px, `answer-hint-phone` ("Return to check", under the field) at 760 px and below, the same breakpoint that hides the other key hints. Without script it names the desktop hint. The desktop hint is set as DESIGN.md writes it, capitalised ("Enter to check · Shift+Enter for new line · Space to start/stop speaking (empty answer)"); the review's hint line keeps its lowercase "enter or space for next card".
- **Keys.** Enter (and Ctrl/Cmd+Enter) checks; Shift+Enter and Alt+Enter are left to the field; nothing submits while an input method composes (`isComposing` or key code 229). Space starts or stops listening only with no modifier and an empty field (or while listening).
- **Dictation spacing.** Spoken words are appended after a single space unless the typed text is empty or already ends in whitespace (a space or a line break), then the caret goes to the end and the field scrolls to it.
- **A draft's leading line break.** The template writes a newline right after `<textarea …>`, because the HTML parser drops the first one; a draft that itself starts with a line break comes back intact on "couldn't check".
- **Couldn't check with script.** A non-2xx answer to `/study/answer` is not swapped in: the status line takes the words from the page that came back (the server's own sentence, or the allowance sentence and its link) and the field keeps its text untouched. A network failure, or no answer within 90 s, says the DESIGN.md sentence. While it checks the field is read-only (so the text being judged is the text kept) and Check is disabled.
- **Card motion.** Card swaps run inside a same-document View Transition (`document.startViewTransition`): the card element (`.current`, `.done` or `.empty-state`) is named `card` and its group does not morph (no scaling). The kind is set on `html[data-vt]`: `verdict` (the waiting card fades out in 120 ms while the review's own marks draw: the strike and the 4 px rise), `card` (Next card: out 120 ms, the next card in with an 8 px rise, 200 ms) and `leave` (Snooze/Delete: out moving 8 px left, 180 ms). Without the API the next card still rises in (`.is-arriving`). A snoozed or deleted card leaves only once the server has answered, so a failure leaves it in place.
- **Destination switch.** `@view-transition { navigation: auto; }` cross-fades the page (160 ms) under a header named `top-bar` whose transition is suppressed, so the header and navigation stay still. The active navigation underline is now a 1 px `::after` line under the label (instead of `text-decoration`) so it can draw in by clip (160 ms); it is drawn again only when the destination changes (a study swap that keeps the same navigation leaves it alone).
- **Skipped destination transitions.** A cross-document view transition the browser skips (a second navigation or a resize during the 160 ms) rejects its `ready` promise in the page it reveals. app.js handles those promises (`pagereveal` and `pageswap`), and the script tag is `defer blocking="render"` so that handler is in place before the first paint; without it a skipped transition surfaced as an uncaught "Transition was skipped" page error. Parsing still never waits on the script.
- **Edit card drawer.** Opening and closing animate the `<details>` content (`::details-content`, `interpolate-size: allow-keywords`, height and opacity in 200 ms, `content-visibility` allow-discrete). Browsers without these open and close at once.
- **When each word shows.** "checking" (and every button word: "saving", "snoozing", "removing", "restoring", "preparing") appears after 400 ms; "listening" and "transcribing" at once (the voice section sets its own timing); "importing" and its 2 px progress line at once, as the processing marks of the card and billing forms they share. A word that lives on a button is also said in the nearby polite status line (visually hidden), so screen readers hear it.
- **Others set aside.** While Snooze, Delete or Restore runs, the card's other actions (Snooze, Delete, Restore, Check, Next card, Again, Save card) are disabled and given back on failure or Back. Next card sets Again, Snooze, Delete and Save aside. Restore after a session has ended has no status line of its own; a failure is said just after its form.
- **Failures.** Save card: "Couldn't save that just now. Your edits are still here." beside Save (a refused field is still said under that field; a 4xx with a reason says the reason). Next card (not in DESIGN's table): "Couldn't open the next card just now. Try again." Restore: "Couldn't restore that just now." Import: a refusal is read from the page the server returned and said in place, with the idea title and the chosen file kept; a network or server failure says "Couldn't import that just now. Your file and idea title are still here; try again." Download: a failed, or redirected (signed out), export is a failure. Every failure nudges the control it names once.
- **Downloads.** With script each Download link fetches its file and hands it to the browser through an object URL named from `Content-Disposition`; a modified click (new tab, save as) and no-script use the plain link.
- **Forced failures for the headed proof.** The front end treats any failed or non-2xx request as the failure of that flow, so a walkthrough can force each one by intercepting its request in the browser (for example Playwright `page.route(…, r => r.abort())`); no server knob was added for it.
- **Reduced motion.** Every animation and transition is removed (final states), `:active` presses do not move, view transitions are off (`@view-transition { navigation: none; }`, and no same-document transition is started), and the remaining static `translate(-50%)` centring (trial chip touch target, the search key) was rewritten without transforms, so nothing is translated at all.
- **Repeats.** The checking pulse is the third loop in the sheet (`breathe`, 1.6 s, ease-in-out, opacity 1 ↔ 0.6), beside the spinner and the progress line; the earlier test that allowed exactly two loops now allows these three.
- **Server side.** The review shows `white-space: pre-wrap` for the learner's answer; keeping line breaks in the stored answer is `app.Answer`'s job (changed separately, see S113-1).

## S125-1: the exact L3-S3 mark, restored from the canonical SVG
- **One geometry, read from DESIGN.md.** DESIGN.md's two ```svg blocks are committed byte for byte as `internal/web/brand/mark-light.svg` and `mark-dark.svg`; `TestDesign_S125_1_CanonicalSVGIsDesignVerbatim` reads DESIGN.md itself and fails if the blocks, the layer table, the header mark, the SVG favicon, the app icons or the maskable source draw anything else. `scripts/render-brand.py` writes every other file from the same numbers.
- **Header.** The inline SVG is the canonical drawing (`viewBox="0 0 32 32"`, the same rects, the same `rotate(10 19 14)` group, bar `opacity="0.85"`) at `width="24" height="24"`, with a class per layer. The fill attributes are the light pair (also the no-CSS fallback); the style sheet sets `fill: var(--mark-*)`, and the `--mark-*` custom properties are declared beside the theme tokens in all three places the mode is decided (light `:root`, the system-dark media block unless Settings says light, and Settings' dark), so the active mode picks the pair. They are not the theme tokens: the dark top card is `#3B82F6`, not `accent` `#4D8DFF`.
- **No 1x twin any more.** The interim deck carried a second, pixel-hinted header drawing for 1x screens. DESIGN.md now says every surface uses the canonical SVG "scaled whole" and lists hinting only for the PNG favicons, so the header draws the canonical SVG at every density.
- **PNG favicon hinting.** "The upright cards' edges land on whole pixels": at 32 px one grid unit is a pixel, so only the middle card's y 7.5 moves (to 8; height 19 kept). At 16 px a unit is half a pixel: both upright cards snap to even units, back (4, 10) and middle (6, 8), and their height 19 snaps to 20 (8 x 10 px), so every upright edge is a whole pixel, the cards stay portrait and the one-pixel fan up and right is kept; nothing moves more than half a pixel. The tilted top card and its prompt bar cannot sit on a pixel grid and are drawn exactly as the canonical SVG (so they are anti-aliased; DESIGN.md forbids straightening the tilt). The test checks the pixels on both sides of each upright edge, the portrait shape, the tilt (the cobalt card's left edge leans) and the lighter bar.
- **SVG favicon.** One 32 x 32 grid file with the light fills as attributes and a `prefers-color-scheme: dark` style block switching every layer to the dark pair, as DESIGN.md asks; the earlier hinted 16 px drawing is gone from it.
- **App icons.** The canonical drawing inside `<g transform="translate(12 12) scale(1.25)">`: the 32 grid at 62.5% (40 px of 64), centred as a grid on the icon. The superellipse grounds, the light icon's 1 px `line` edge (#E2E2E3), the dark "any" icons (192, 512) and the full-bleed dark maskable/apple-touch (180)/social preview icons are unchanged in role. Every mark pixel lies within ~21.3/64 of the centre, inside the 25.6/64 safe circle (checked per pixel at 64, 180, 192 and 512).
- **Retired everywhere.** The interim S3-A drawings (`mark-24-*.svg` sources, the `mark-far`/`mark-near`/`mark-face` classes, the 1x `mark-hinted` twin, the landscape 14 x 10 cards) are deleted, and `TestDesign_S125_1_NoLensOrInterimDeckRemains` fails if any template, static file, script or Go source draws either retired mark.
- **Light back card.** The canonical light back card (`#CBD5E1`) is faint on the white page and in the light favicon; it is DESIGN.md's colour and is kept exactly.

## S135-1: the landing page is clean on a phone
- **Desktop Specimen.** DESIGN.md says desktop is unchanged by [S135-1] apart from the single hero action and the terms line, while Prompt and Answer on phones are drawn as the phone's Study screen. Both pictures are in the markup: the desktop answer line and hint (`.specimen-desk`) show above 760 px, and the phone's picture of Study (`.specimen-study`: field, Return to check, mic, Check, one card coming up) shows at 760 px and below. The phone picture is `aria-hidden` apart from the typed answer, and holds no focusable element.
- **Terms line breaks.** Each phrase is a `white-space: nowrap` span, and each " ·" separator is its own nowrap span with no space before it, so the only break opportunities are the spaces after a separator. The text reads exactly "7 days free · no card needed · $5/month after".
- **Subject header on the Specimen ([S131-1]).** The Specimen shows the S131-1 header (layers icon, "Mental models", "· Set 1", "4 / 11"; screen readers hear "Card 4 of 11"), which this slice's Landing text requires. This slice does not add the S131-1 header to real Study cards (the deck icons per starter set, the neutral tag icon and the cross-fade). That still needs its own reconcile.
- **Upcoming card.** The Specimen's one upcoming card is the literal "NATO alphabet: K" from DESIGN.md. It is a picture, not a real queue.

