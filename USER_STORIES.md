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
2. WHEN I submit an answer, THE review surface SHALL replace the answer control
   with the result heading, the expected answer, and one Next control. When the
   question is a choice, THE tap on a choice SHALL grade the answer without any
   separate submit.
3. WHEN a review is graded, THE review surface MUST NOT show kind or due-count
   chrome, schedule lectures, Flag-a-problem, or an inspect accordion, and
   reveal, edit, and archive MUST be reachable only from one overflow control.
4. THE review document in every state, including the empty state, MUST NOT
   contain "Too advanced", "Saved foundations", "Fix or inspect", or "Stop
   reviewing this question", and MUST NOT remove reveal, edit, or archive from
   the review surface.
5. THE review session's navigation SHALL be one set (Review, Add, Library,
   History, Settings) and no second navigation set SHALL appear on the review
   surface.

No-gos: no MIS-59 foundations UX rebuild, no grading or scheduler behavior
change, no removal of foundations data or routes, no restyle of pages outside
the review surface.

Evidence: `internal/web/review_test.go`
