# Stories

<!-- Root artifact: what users must be able to do. One file, ids never
reused, criteria a check can fail on. skill://user-stories guides edits.
Authoritative chain: VISION.md (intent) -> this file (root stories) -> SPEC.md
(binding acceptance ids S##.##, behavior, architecture) -> docs/qa/system.md and
docs/qa/critics.md (verification). Linear owns work state. Journeys and receipts
cite both US ids and S ids. -->

Intent revision authority: on 2026-09-23 the operator authorized "Execute this
reimagining in full" (MIS-162). US-001–003 are revised to the approved concept-
centered experience, and US-005–012 are new, never recycled identifiers.

## Capability: Concept-centered study

## US-001 Find and reuse saved understanding

Statement: When I explore what I am learning, I want saved concepts and notes searchable and revisitable, so that I can connect prerequisites across questions without losing earlier study.

Criteria:
1. WHEN schema 4 upgrades to schema 5, THE SYSTEM SHALL preserve foundation rows and relations as historical data; foundation-origin concepts SHALL NOT appear as newly generated concepts in the Map or Stream.
2. WHEN I search the Map for a concept, note, question, or source text, THE SYSTEM SHALL return matching linked results without answer-bearing question snippets; IF none match, THEN THE SYSTEM SHALL show an empty search result without error.
3. WHEN I inspect a concept page, THE SYSTEM SHALL show its current notes, questions, prerequisites, and linked sources while preserving old presented wording and review history.

No-gos: no manual graph editor, no automatic replanner.

Evidence: `internal/store/migration_v5_test.go`, `internal/store/v5_test.go`,
`internal/web/review_test.go`

## Capability: Study loop

## US-002 Stay with one question through its result

Statement: When I review, I want one question and one way to answer, followed by the result and Next, so I can keep studying without navigating a dashboard.

Criteria:
1. WHEN a question is ungraded, THE SYSTEM SHALL show it as the dominant heading with one answer control (choice buttons that submit by tap, or one recall field and submit), and SHALL keep the answer and explanation hidden until grading or self-check.
2. WHEN an answer resolves, THE SYSTEM SHALL retain the question, result, expected answer, and explanation until I choose Next; a choice tap SHALL NOT require another submit. WHEN a check is pending, THE SYSTEM SHALL show checking without inventing a grade.
3. WHEN an automatic check reports a miss, THE SYSTEM SHALL show a quiet "I was right" link beneath Next; WHEN an automatic check reports correct, THE SYSTEM SHALL offer "Count as a miss" in More. These controls SHALL NOT auto-advance.
4. THE SYSTEM SHALL show exactly two masthead destinations, Add and Map, beside the wordmark; other maintenance actions SHALL remain in More, not an always-visible navigation bar or inspection dashboard.
5. WHEN review is empty or caught up, THE SYSTEM SHALL offer Add or an honest next-time indication, not an invented due question or foundation detour.

No-gos: no auto-advance, due-count chrome, foundation routes, or hidden answer preloading.

Evidence: `internal/web/review_test.go`

## US-003 Get an honest answer judgment

Statement: When I answer in my own words, I want correct meaning recognized where appropriate and uncertainty handed back to me, so my history reflects what I actually knew.

Criteria:
1. WHEN a choice, exact-form recall, or authored variant matches, THE SYSTEM SHALL resolve it locally without a network check; an exact-form near miss SHALL go to self-check rather than pass by similarity.
2. WHEN a flexible short recall answer needs judgment, THE SYSTEM SHALL stage one bounded Jev `short-v1` check outside SQL; it SHALL accept only at accept probability ≥0.85, exact-identity risk ≤0.35, injection risk ≤0.20, or reject only at reject probability ≥0.90 and injection risk ≤0.20. Other outcomes SHALL remain ungraded for self-check.
3. WHEN explain-level prose has an authored rubric, THE SYSTEM SHALL retain the `semantic-v1` required-ideas policy; incomplete and incorrect shadow classes SHALL remain ungraded, and answer-bearing cues SHALL count as assistance before display.
4. WHEN a check is close, unsure, unavailable, malformed, or fails, THE SYSTEM SHALL preserve the answer and offer a learner self-check; a failed check SHALL also offer Retry check without sending the identical paid assessment twice.
5. WHEN a result is recorded, THE SYSTEM SHALL name its authority as exact, Jev, learner, or reveal; it SHALL preserve original attempts and grade corrections across restart without silently rewriting history.

No-gos: no liberal string-similarity grading, model call inside a SQL transaction, learner rubric authoring, or change to pinned scheduler identity.

Evidence: `internal/learning/semantic_test.go`, `internal/store/semantic_test.go`,
`internal/store/review_test.go`, `internal/semantic/v5_request_test.go`,
`internal/web/semantic_test.go`

## US-004 Check generated candidates before publication

Statement: I want generated quizzes checked for critical defects before they
enter review. A failed check must retain candidates and paid usage.

Criteria:
1. WHEN the critic is configured, THE SYSTEM SHALL save validated candidates
   before any critic request. EACH batch SHALL contain at most 60 candidates.
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

No-gos: no generator-v4 change, pairwise duplicate checks,
practice-coverage UI, production activation, or general accuracy claim.
Contrast questions (US-012) are generated and pass the same critic fence.

Evidence: `internal/learning/critic_test.go`,
`internal/semantic/critic_request_test.go`, `internal/store/critic_test.go`,
`internal/store/critic_lifecycle_test.go`, `internal/generation/critic_test.go`.

## Capability: Capture and understand

## US-005 Capture a goal in one step

Statement: When I have something to learn, I want to add it in one step with its actual source type, so useful material can be prepared without a project setup.

Criteria:
1. WHEN I add a Topic, My text, Link, or Photo, THE SYSTEM SHALL require that explicit mode and save exactly one source and goal for an identical operation ID.
2. WHEN I choose Topic, THE SYSTEM SHALL search through Exa only if configured; WHEN I paste My text, THE SYSTEM SHALL NOT send it to web search. A Link SHALL fetch only the chosen URL, and a Photo SHALL transcribe before planning.
3. IF Topic research has no documents, THEN THE SYSTEM SHALL proceed with labeled general knowledge; IF a Link page cannot be read, THEN THE SYSTEM SHALL fail that preparation recoverably instead of planning from the URL; IF a photo is unsupported or too large, THEN THE SYSTEM SHALL reject it without creating a source.
4. WHEN generation fails, THE SYSTEM SHALL preserve captured material and show a recoverable failed preparation state rather than report ready.

No-gos: no automatic search of pasted private text, account signup, or duplicate capture on replay.

Evidence: `internal/store/v5_test.go`, `internal/generation/exa_test.go`,
`internal/generation/worker_test.go`, `internal/web/review_test.go`

## US-006 Understand a question's concept

Statement: When a question assumes an idea I do not understand, I want a note for that concept one tap away, so studying can be more than repeating the answer.

Criteria:
1. WHEN a newly generated question publishes, THE SYSTEM SHALL link exactly one primary concept and an existing standard note; a contrast question SHALL link its second concept separately.
2. WHEN I open a concept from a graded question, THE SYSTEM SHALL show its note and linked questions without changing the current review occurrence.
3. WHEN a note quotes my material or a web excerpt, THE SYSTEM SHALL keep exact evidence and inspectable provenance; general-knowledge notes SHALL be labeled and SHALL NOT claim quoted source support.

No-gos: no untrusted markup execution, fabricated citations, or reading counted as unaided recall.

Evidence: `internal/store/v5_test.go`, `internal/generation/v5_validation_test.go`,
`internal/web/review_test.go`

## Capability: Honest feedback and control

## US-007 Correct a grade in one tap

Statement: When a check gets my answer wrong, I want to correct it immediately, so my future practice does not inherit a false result.

Criteria:
1. WHEN an automatic miss is shown, THE SYSTEM SHALL offer "I was right" beneath Next; WHEN an automatic correct result is shown, THE SYSTEM SHALL offer "Count as a miss" in More.
2. WHEN I correct a grade, THE SYSTEM SHALL record one immutable override, replace the current schedule consistently, and retain the original attempt and authority in history.
3. WHEN the same override operation is retried, THE SYSTEM SHALL return the committed correction without a second schedule change; a stale correction SHALL conflict.

No-gos: no rewriting the original attempt or pretending the correction was a new cold recall.

Evidence: `internal/store/v5_test.go`, `internal/web/review_test.go`

## US-008 Answer short questions in my own words

Statement: When a short answer means the same thing in different words, I want it to count without accepting a different fact as correct.

Criteria:
1. WHEN a flexible short response does not match an authored variant locally, THE SYSTEM SHALL use the `short-v1` Jev battery with verdict, exact-identity, and injection judgments.
2. WHEN accept probability is at least 0.85 and identity risk at most 0.35 and injection risk at most 0.20, THE SYSTEM SHALL accept; WHEN reject probability is at least 0.90 and injection risk at most 0.20, THE SYSTEM SHALL reject; otherwise it SHALL ask me to self-check.
3. WHEN a check fails or its result cannot be trusted, THE SYSTEM SHALL keep my answer, name the learner's judgment as authority if I self-check, and SHALL NOT silently award success.

No-gos: no liberal similarity rule, hidden automatic retry, or claim that an untested holdout passed; policy adoption requires holdout evidence before live activation.

Evidence: `internal/learning/short_test.go`, `internal/semantic/v5_request_test.go`,
`internal/store/v5_test.go`, `internal/web/semantic_test.go`

## Capability: See and shape learning

## US-009 See what I know

Statement: When I return to study, I want to see where my concepts stand, so I can choose what needs attention without mistaking an estimate for a fact.

Criteria:
1. WHEN I open Map, THE SYSTEM SHALL show active and paused goals with concept status, due indication, and an accessible list alongside any decorative constellation.
2. WHEN I inspect a concept, THE SYSTEM SHALL distinguish new, learning, solid, and fading, show unaided/helped/missed observations separately, and label predicted recall explicitly as an estimate.
3. WHEN I search, THE SYSTEM SHALL return relevant concept, note, question, or source hits without adding a learning event.

No-gos: no guaranteed mastery score, color-only meaning, or synthetic review events from navigation.

Evidence: `internal/learning/concept_test.go`, `internal/store/v5_test.go`,
`internal/web/review_test.go`

## US-010 Say what I want to know

Statement: When my interests change, I want to pause or focus a learning goal, so the next material reflects what I choose.

Criteria:
1. WHEN capture succeeds, THE SYSTEM SHALL create one goal tied to the source; focus SHALL prioritize new concepts belonging to that goal.
2. WHEN I pause a goal, THE SYSTEM SHALL exclude its new material from ordinary selection and retain its notes/history; WHEN I resume it, THE SYSTEM SHALL make eligible material selectable again.
3. WHEN I change pace among light, steady, and intense, THE SYSTEM SHALL cap new concepts in a rolling day at 3, 6, and 12 respectively without fabricating due work.

No-gos: no invisible goal deletion or goal settings as a gate before capture.

Evidence: `internal/store/v5_test.go`, `internal/learning/selection_test.go`,
`internal/web/review_test.go`

## US-011 Meet prerequisites first

Statement: When one idea depends on another, I want a useful introduction and simpler starting point before its questions, so I am not repeatedly tested on something I have not encountered.

Criteria:
1. WHEN a goal's concepts have prerequisites, THE SYSTEM SHALL order unseen introductions prerequisite-first and show the standard note before that concept's first question.
2. WHEN I choose "I know this already" on the intro, THE SYSTEM SHALL record that observation without scoring a cold review or skipping later due questions.

No-gos: no compulsory foundation detour or reading counted as a graded success.

Evidence: `internal/learning/selection_test.go`, `internal/store/v5_test.go`,
`internal/web/review_test.go`

## US-012 Practice a real confusion

Statement: When I keep mixing two ideas up, I want a focused contrast question, so I can tell them apart.

Criteria:
1. WHEN two recorded confusions identify the same pair, THE SYSTEM SHALL queue one bounded contrast preparation for those concepts without replacing normal due review.
2. WHEN a contrast question publishes, THE SYSTEM SHALL link one primary and one contrasting concept and explain the distinction without leaking the answer in the prompt.
3. WHEN the source is busy or a preparation fails, THE SYSTEM SHALL preserve earlier questions and observations, and SHALL NOT duplicate the contrast on retry.

No-gos: no inferred confusion from one wrong answer, random pairwise quiz generation, or hidden paid retry.

Evidence: `internal/store/v5_test.go`, `internal/store/jobs_test.go`,
`internal/generation/worker_test.go`, `internal/web/review_test.go`
