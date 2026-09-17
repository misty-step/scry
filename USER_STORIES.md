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
