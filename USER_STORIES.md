# Stories

<!-- Root artifact: what users must be able to do. One file, ids never
reused, criteria a check can fail on. skill://user-stories guides edits. -->

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
