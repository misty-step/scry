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
- SeedDemo is idempotent: an ordinary demo account with over 100 items, at least 10 due, seven suggestions, 62% allowance used and an established trial/plan history. TestRule_SeededDemoData pins the fixture.
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

- **The mark's geometry.** DESIGN.md fixes the grid (24 px, stroke 1.75,
  round caps/joins), the parts (four corner brackets, a centered lens ring in
  ink, a solid accent focal point of r 2.25) and the sizes. The exact numbers
  are ours: brackets `M3 8V3h5 …` (5 px arms from the 3 px inset) and a lens
  ring of r 5.25. `internal/web/brand.go` is the one source; the static icons
  repeat its paths and `TestBrand_MarkIsOneDrawingEverywhere` pins that.
- **App icons.** `static/icon.svg` is the adaptive light/dark vector (favicon;
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
  6,000 input characters) with the parent's question and answer and up to
  three recent Incorrect answers as untrusted data. The reply is a JSON
  object of 2–3 `{q, a}` cards; repeats, empty or over-long cards are dropped
  and fewer than two is an error. It is reserved and settled as `draft`
  usage, with the same G2 rule (an outcome unknown after the call keeps the
  hold). In `SCRY_MODE=test` it is `ai.FakeBridger`: three fixed, labelled
  fixture cards built from the parent's question only, no network;
  `SCRY_TEST_BRIDGES=fail` shows the failure state. `SCRY_AI_MODE=live` on a
  test build uses the real client, like the other surfaces.
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
  last review, due date and review times back together.
- **Pre-existing test flake.** `TestReserveRacingPlanEnd` (internal/store)
  fails with SQLITE_BUSY on this sandbox's disk on master as well as here;
  with `TMPDIR=/dev/shm` the whole suite passes. Not changed by this slice.
