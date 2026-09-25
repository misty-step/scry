import assert from 'node:assert/strict';

// Each numbered step cites the corresponding USER_STORIES.md criterion. A CLI
// boundary check is used only where the synthetic browser cannot observe the
// store/learning/provider policy. Real provider and production acceptance remain
// independent and are never claimed by this fixture.
const test = (pattern, packages) => async c => c.goTest(pattern, packages);
const store = (pattern) => test(pattern, ['./internal/store']);
const policy = (pattern) => test(pattern, ['./internal/learning']);
const web = (pattern) => test(pattern, ['./internal/web']);
const gen = (pattern) => test(pattern, ['./internal/generation']);
const sem = (pattern) => test(pattern, ['./internal/semantic']);
const text = async (locator, expected) => assert.match(await locator.innerText(), expected);
const map = async c => { await c.page.goto(c.url('/map')); await c.page.getByRole('heading', {name: 'Map'}).waitFor(); };
const correct = async c => { await c.page.locator('fieldset.choice-fieldset .choice', {hasText: 'IPv4 address'}).click(); await c.page.locator('[data-state=result]').waitFor(); };
const wrong = async c => { await c.page.locator('fieldset.choice-fieldset .choice', {hasText: 'Text value'}).click(); await c.page.locator('[data-state=result]').waitFor(); };
const recall = async c => { await correct(c); await c.page.locator('form[data-next] button').click(); await c.page.locator('#recall-answer').waitFor(); };
const concept = async c => { await correct(c); await map(c); await c.page.locator('.concept-list a').first().click(); await c.page.locator('.concept-stage').waitFor(); };

export const specs = {
  'US-001': [
    store('^TestSchemaV4ToV5MigrationUS001$'),
    async c => { await concept(c); await text(c.page.locator('.note'), /DNS A record/); await c.page.getByRole('heading', {name: 'Questions'}).waitFor(); await store('^TestEditArchiveAndDisputePreserveEvidence$')(c); },
  ],
  'US-002': [
    async c => { await c.page.locator('h1.question').waitFor(); assert.equal(await c.page.locator('fieldset.choice-fieldset .choice').count(), 4); assert.equal(await c.page.locator('.expected-answer, .explanation').count(), 0); },
    async c => { await correct(c); await text(c.page.locator('.feedback'), /Correct[.]/); await text(c.page.locator('.pair-key'), /IPv4 address/); await c.page.locator('form[data-next] button').waitFor(); await c.page.reload(); await c.page.locator('[data-state=result]').waitFor(); await web('^TestSemanticFailureOffersHonestSelfCheck$')(c); },
    async c => { await c.page.locator('details.overflow summary').click(); await c.page.getByRole('button', {name: 'Count as a miss'}).waitFor(); await c.page.locator('[data-state=result]').waitFor(); await c.page.locator('details.overflow summary').click(); },
    async c => { assert.equal(await c.page.locator('nav.masthead-actions a').count(), 2); await text(c.page.locator('nav.masthead-actions'), /Add.*Map/s); },
    async c => { await c.page.locator('form[data-next] button').click(); await c.page.locator('[data-state=question]').waitFor(); await store('^TestLostResponseResumeAndStaleNext$')(c); },
  ],
  'US-003': [
    async c => { await correct(c); await web('^TestSemanticExactLocalMatchDoesNotCallClient$')(c); },
    async c => { await policy('^TestShortV1PolicyUS008$')(c); await sem('^TestShortAnswerBatteryAndParsing$')(c); },
    async c => { await policy('^TestGradeSemanticPolicy$')(c); await store('^TestSemanticShadowClassesAreRecordedButNeverApplied$')(c); },
    async c => { await c.page.locator('form[data-next] button').click(); await c.page.locator('#recall-answer').waitFor(); await c.type('#recall-answer', 'a plausible but non-exact answer'); await c.page.locator('form.answer-form button[type=submit]').click(); await c.page.locator('[data-state=self-check]').waitFor(); await text(c.page.locator('.self-check'), /You answered|Check for yourself/); await store('^TestSemanticFailurePreservesAnswerAndAllowsNewOperation$')(c); },
    store('^TestSemanticAssessmentStagesFinalizesAndReplaysIdempotently$'),
  ],
  'US-004': [
    store('^TestCriticPersistsBeforeSendAndSingleConcurrentLease$'),
    policy('^TestCriticEveryHardVetoAndFrozenBoundary$|^TestCriticMalformedIsUngraded$|^TestCriticSoftRanksButNeverRejects$'),
    store('^TestCriticCrashNeverResendsAndPreservesUnknownSpend$|^TestCriticReservationSharedWithGenerationAndSemantic$'),
    store('^TestCriticRetryReusesCandidatesAndJudgedRowsWithoutGenerationReservation$|^TestCriticMalformedAndSourceChangeCannotPublish$'),
    store('^TestCriticUnconfiguredPreservesPublicationAndReservesNothing$'),
    store('^TestCriticPublicationRevalidatesAndRecordsRejections$|^TestCriticSavedUsageCannotBeOverwrittenByCompletionOrFailure$'),
  ],
  'US-005': [
    async c => { await c.page.goto(c.url('/add')); assert.equal(await c.page.locator('input[name=mode]').count(), 4); await c.page.locator('input[name=mode][value=text]').check(); await c.type('form.capture-form textarea[name=text]', 'Synthetic private note about DNS records'); await c.page.getByRole('button', {name: 'Add to Scry'}).click(); await c.page.locator('.source-stage').waitFor(); await text(c.page.locator('.source-stage'), /Synthetic private note/); await web('^TestShareTargetNeverChoosesCaptureModeUS005$')(c); },
    async c => { await gen('^TestResearchWithoutKeyDistinguishesTopicAndUnreadableLink$')(c); await store('^TestCaptureModesUS005$')(c); },
    async c => { await web('^TestCaptureRequiresModeAndBoundsPhoto$')(c); await gen('^TestV5TranscriptionPreservesSourceAndRejectsInventedFields$')(c); },
    async c => { await map(c); await store('^TestStoppedCaptureStaysReachableOnMap$')(c); },
  ],
  'US-006': [
    store('^TestConceptChainAndIntroUS006$'),
    async c => { await concept(c); await c.page.locator('.question-list').waitFor(); await c.page.locator('.note').waitFor(); await c.page.goto(c.url('/')); await c.page.locator('[data-state=result]').waitFor(); await c.page.locator('.concept-chip a').click(); await c.page.locator('.concept-stage .note').waitFor(); },
    async c => { await text(c.page.locator('.note-foot'), /General knowledge/); await gen('^TestV5UnmatchedWebQuoteDowngradesTopicButDropsSource$|^TestV5CurlyQuoteAndWhitespaceEvidenceSnapsBeforePublication$')(c); },
  ],
  'US-007': [
    async c => { await wrong(c); await c.page.getByRole('button', {name: 'I was right'}).waitFor(); },
    async c => { await c.page.getByRole('button', {name: 'I was right'}).click(); await c.page.locator('.feedback[data-verdict=correct]').waitFor(); await text(c.page.locator('.feedback'), /You changed this to correct/); },
    store('^TestOverrideAutomaticGradeUS007$|^TestOverrideRetiresStrandedOccurrence$'),
  ],
  'US-008': [
    async c => { await recall(c); await c.type('#recall-answer', 'Transport Layer Security'); await c.page.locator('form.answer-form button[type=submit]').click(); await c.page.locator('[data-state=result]').waitFor(); await text(c.page.locator('.feedback'), /Correct/); },
    policy('^TestShortV1PolicyUS008$'),
    async c => { await store('^TestShortAnswerStagingAndFinalizationUS008$')(c); await web('^TestSemanticFailureOffersHonestSelfCheck$')(c); },
  ],
  'US-009': [
    async c => { await map(c); await c.page.locator('svg.constellation[aria-hidden=true]').waitFor(); await c.page.locator('ul.concept-list .status-label').first().waitFor(); },
    async c => { await c.page.locator('.concept-list a').first().click(); await c.page.locator('.gate-stage, .concept-stage').first().waitFor(); if (await c.page.locator('.gate-stage').count()) { await c.page.getByRole('button', {name: 'Show me and open'}).click(); } await c.page.locator('.concept-stage').waitFor(); await text(c.page.locator('.practice-panel .hint'), /estimate/i); await policy('^TestConceptStateUS009$')(c); },
  ],
  'US-010': [
    async c => { await map(c); await c.page.locator('.goal').waitFor(); await c.page.locator('.goal-actions a[href^=\"/sources/\"]').waitFor(); },
    async c => { await map(c); await c.page.getByRole('button', {name: 'Pause'}).click(); await c.page.locator('.goal-meta .badge', {hasText: 'Paused'}).waitFor(); await c.page.getByRole('button', {name: 'Resume'}).click(); await c.page.getByRole('button', {name: 'Pause'}).waitFor(); await store('^TestGoalPauseStopsNewMaterialUS010$')(c); },
    async c => { await c.page.goto(c.url('/settings')); await c.page.locator('input[name=pace][value=light]').waitFor(); await text(c.page.locator('.pace-options'), /3 new ideas.*6 new ideas.*12 new ideas/s); await policy('^TestSelectionDueGoalsAndPacingUS010$')(c); },
  ],
  'US-011': [
    policy('^TestSelectionPrerequisitesFirstUS011$'),
    store('^TestOnlyTheOfferedIntroCanBeAcknowledgedUS011$|^TestConceptChainAndIntroUS006$'),
  ],
};
