# Concepts, notes and goals

Stories: US-001, US-006, US-009, US-010
Source: internal/web/concepts.go, internal/web/templates/library.html, internal/store/*.go, internal/learning/concept*.go, internal/learning/selection*.go

## Sub-features

Additive schema-v5 history, saved notes and provenance, goal focus/pause, accessible concept list and decorative constellation, separate observations and labeled recall estimate, and prerequisite-first introductions.

## How to get to it (user POV)

Choose Map (`/map`), follow `/concepts/{id}` to a note and related questions, and follow Original input to `/sources/{id}`. Use `/goals/{id}`'s Focus on this, Pause, or Resume. Settings (`/settings`) offers Light, Steady, and Intense pace. A graded Stream concept chip links to the same concept page; an ungraded question's Look it up route shows an assistance gate first.

## Driving it

`qa/walk --stories "US-001 US-006 US-009 US-010"` inspects authored Map, Concept, notes, status words, pace, and focus/pause. Existing store/learning migration and selection checks provide non-browser-observable foundation-row preservation, prerequisite order, rolling-day caps, and one-source/one-goal identity. Use `go test -p 1 ./internal/store ./internal/learning` for those boundaries; fixture activity is not an unaided memory measurement.

## Gotchas

The authored fixture pre-acknowledges intros and has topic-basis notes only. A live source-backed note needs exact quoted evidence and provenance; a general-knowledge note cannot invent a citation. Decorative `svg.constellation` is `aria-hidden`; `ul.concept-list` carries the accessible state. Opening a note must not advance an occurrence. Old foundation rows remain historical after migration, not fresh Map concepts. Status and recall percentage are estimates, not mastery guarantees.
