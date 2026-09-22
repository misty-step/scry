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
words, I want Scry to check the authored meaning rather than require one phrase,
while preserving my answer and learning history whenever that check is uncertain.

Criteria:
1. EACH quiz content version SHALL explicitly choose exact or semantic grading;
   choice, numeric, symbolic, identifier, spelling, and other deterministic
   answers SHALL remain exact.
2. WHEN an exact answer, authored variant, or case-only uncertainty resolves
   locally, THE SYSTEM SHALL NOT call the semantic assessor. OTHERWISE a semantic
   prose answer SHALL be saved as a durable pending assessment before one bounded
   external request is made outside SQL.
3. THE semantic-v1 policy SHALL record Correct only when all authored required
   ideas and the overall relation independently meet their thresholds with no
   contradiction or injection signal. It SHALL NOT record an automatic miss by
   default; unclear, malformed, unavailable, and timed-out checks remain ungraded.
4. WHEN exactly one required idea is clearly missing, THE surface SHALL show
   Almost and its authored cue only after the occurrence is durably marked
   assisted. A later correct response on that occurrence SHALL use the helped
   warm contract, not an FSRS success.
5. A failed check SHALL keep the answer, offer retry and reveal, and show no model
   probabilities. Replaying the same operation SHALL resume or return its durable
   assessment without duplicate review/schedule transitions; stale finalization
   SHALL be superseded without changing learning state.

No-gos: no liberal similarity grading, no model call inside a SQL transaction,
no change to the pinned FSRS algorithm/ratings, and no learning-efficacy claim.

Evidence: `internal/learning/semantic_test.go`, `internal/store/semantic_test.go`,
`internal/semantic/client_test.go`, `internal/web/semantic_test.go`

