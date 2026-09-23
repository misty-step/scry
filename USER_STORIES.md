# Stories

<!-- Root artifact: what users must be able to do. One file, ids never
reused, criteria a check can fail on. skill://user-stories guides edits.
Authoritative chain: VISION.md (intent) -> this file (root stories) -> SPEC.md
(binding acceptance ids S##.##, behavior, architecture) -> docs/qa/system.md and
docs/qa/critics.md (verification). Linear owns work state. Journeys and receipts
cite both US ids and S ids. -->

## Capability: Concept-centered study

## US-001 Map saved foundations onto concepts and references with search and reuse

Statement: When I explore what I am learning, I want my saved foundations organized into concepts and reusable references that I can search and revisit, so that I can understand prerequisites across questions without losing my study history.

Criteria:
1. WHEN the database upgrades from schema version 2, THE SYSTEM SHALL preserve all existing foundation rows and map units to concepts, materials to references, and foundation links to concept relations.
2. WHEN I search concepts or references by text, THE SYSTEM SHALL return matching concepts and references with their linked counterparts.
3. WHEN I inspect a concept, THE SYSTEM SHALL return its linked references, quizzes, and prerequisites.
4. IF a search query matches no concepts or references, THE SYSTEM SHALL return an empty result without error.

No-gos: no manual graph editor, no relationship-role vocabulary, no automatic replanner.

Evidence: `internal/store/concept_test.go`

## Capability: Study loop

## US-002 Study with a focused review surface: question, answer, check, next

Statement: When I am reviewing, I want the screen to be the question with exactly
one way to answer it, and after I submit I want the result and Next, so that I
can keep studying without scanning a page to decide what to do next.

Criteria:
1. WHEN the current review is ungraded, THE review surface SHALL render the
   question as the dominant heading, one answer control (choice buttons that
   submit by tap, or a short recall field plus one submit), and no other answer
   control or secondary row.
2. WHEN a submitted answer is graded, THE review surface SHALL replace the
   answer control with the result heading, the expected answer, and one Next
   control, and a choice tap SHALL grade the answer without any separate
   submit. WHEN a recall submission is unclear and stays ungraded, THE surface
   SHALL keep the question and the answer form for another attempt.
3. THE review surface MUST NOT show kind or due-count chrome, schedule lectures,
   Flag-a-problem, or an inspect accordion in any state. WHILE the review is
   ungraded, reveal, edit, and archive SHALL be reachable only from the one More
   overflow; WHEN the review is graded, reveal SHALL be hidden while edit and
   archive SHALL remain reachable from the More overflow.
4. THE review document in every state, including the empty state, MUST NOT
   contain "Too advanced", "Saved foundations", "Fix or inspect", or "Stop
   reviewing this question". Reveal SHALL remain reachable while the review is
   ungraded, and edit and archive SHALL remain reachable in every state that
   presents a question.
5. WHEN I am reviewing, THE review document MUST NOT show Review, Add,
   Library, History, or Settings as always-visible links or bars; those five
   destinations MUST be reachable from at most two punch-out controls (a Menu
   control beside the wordmark) on the review surface.

No-gos: no MIS-59 foundations UX rebuild, no grading or scheduler behavior
change, no removal of foundations data or routes, no restyle of pages outside
the review surface.

Evidence: `internal/web/review_test.go`

## US-003 Answer meaning-sensitive recall without invented certainty

Statement: When a prose recall question can be answered correctly in different
words, I want Scry to check the meaning rather than require one phrase, without
asking me to pick a grading mode or write a rubric, while preserving my answer
and learning history whenever that check is uncertain.

Criteria:
1. EACH quiz content version SHALL explicitly record exact or semantic grading.
   Ordinary generation SHALL author required ideas in the same generation
   request only for conceptual prose recall; choice, exact-text, and
   complete-set output SHALL stay exact, and a rubric on them SHALL be rejected.
   THE SYSTEM SHALL NOT infer or override the mode from digits, symbols,
   keywords, or answer length, and SHALL NOT offer the learner a grading control
   or rubric field. A learner edit that changes the prompt, expected answer,
   quoted evidence, or response style SHALL publish an exact version rather than
   keep a stale rubric; earlier versions and review history SHALL NOT be
   rewritten.
2. WHEN an exact answer, authored variant, or case-only uncertainty resolves
   locally, THE SYSTEM SHALL NOT call the semantic assessor. OTHERWISE a semantic
   prose answer SHALL be saved as a durable pending assessment before one bounded
   external request is made outside SQL.
3. THE semantic-v1 policy SHALL record Correct only when all authored required
   ideas and the overall relation (frozen threshold 0.85) independently meet
   their thresholds with no contradiction or injection signal. Under the frozen
   policy incomplete and incorrect SHALL be recorded as shadow classes and
   rendered as ungraded; unclear, malformed, unavailable, and timed-out checks
   remain ungraded.
4. WHEN a class that shows authored help is enabled and applies, THE surface
   SHALL show the cue or feedback only after the occurrence is marked assisted
   and an exposure record is written in the same transaction. A later correct
   response on that content within 24 hours, on any occurrence, SHALL use the
   helped warm contract, not an FSRS success.
5. A failed check SHALL keep the answer, offer retry and reveal, and show no model
   probabilities. EXACTLY one send lease SHALL exist per assessment: a concurrent
   duplicate SHALL be refused, an exact replay SHALL reconcile the durable
   assessment without resending, an interrupted send SHALL become a definite
   failure that keeps its reservation as unknown spend, and the reservation
   SHALL be enforced against the shared daily allowance before any request
   leaves the process. Stale finalization SHALL be superseded without changing
   learning state.

No-gos: no liberal similarity grading, no model call inside a SQL transaction,
no change to the pinned FSRS algorithm/ratings, and no learning-efficacy claim.

Evidence: `internal/learning/semantic_test.go`, `internal/store/semantic_test.go`,
`internal/semantic/client_test.go`, `internal/web/semantic_test.go`,
`internal/generation/meaning_test.go`, `internal/store/meaning_edit_test.go`,
`internal/web/meaning_test.go`

## US-004 Check generated candidates before publication

Statement: I want generated quizzes checked for critical defects before they
enter review. A failed check must retain candidates and paid usage.

Criteria:
1. WHEN the critic is configured, THE SYSTEM SHALL save validated candidates
   before any critic request. EACH batch SHALL contain at most twelve candidates.
2. THE critic-v1 policy SHALL reject any applicable hard defect at probability
   0.80 or higher. Missing or malformed judgments SHALL remain ungraded.
   Explanation teaching value SHALL rank only, never reject.
3. EACH assessment SHALL have one transmission lease and an atomic reservation
   against the shared generation and semantic allowance. Interrupted sends SHALL
   retain unknown usage. A retry SHALL use a new assessment, never resend a row.
4. WHEN a critic fails, THE SYSTEM SHALL preserve candidates and retry only the
   critic. Judged candidates SHALL be reused. Publication SHALL recheck the saved
   judgments, source revision, and job ownership in one transaction.
5. WHEN no critic endpoint is configured, THE SYSTEM SHALL record skipped
   criticism without reserving allowance or changing existing publication.
6. Export and content history SHALL retain candidate attempts, defect reasons,
   model attribution, raw responses, and known or unknown usage.

No-gos: no generator-v4 change, contrast candidates, pairwise duplicate checks,
practice-coverage UI, production activation, or general accuracy claim.

Evidence: `internal/learning/critic_test.go`,
`internal/semantic/critic_request_test.go`, `internal/store/critic_test.go`,
`internal/store/critic_lifecycle_test.go`, `internal/generation/critic_test.go`.
