# Concepts, references, and goals

Stories: US-001, US-006, US-009, US-010

Source: src/learning.rs, src/model.rs, src/engine.rs, src/web.rs, assets/*

## Sub-features

Durable notes and provenance, prerequisite links, Map and goal focus/pause,
separate practice evidence, labeled recall estimates, and 3/6/12 new-idea pace.
## How to get to it (user POV)

Map (`/map`) offers readable goal sections, ordinary concept links, and one
decorative prerequisite constellation. Goal reference (`/goals/{id}`) retains
intent and preparation. Concept reference (`/concepts/{id}`) retains current
notes, questions, provenance, prerequisites, and practice. It does not consume a
held occurrence. An unaided current concept/goal opens behind an assistance
POST before answer-bearing reference is served.

Focus prioritizes a goal's new concepts. Pause excludes its new material while
retaining notes/history and ordinary due work. Resume restores eligible new
material. Settings offers light/steady/intense caps of 3/6/12 new concepts per
rolling day. Status, due indications, separate unaided/helped/missed evidence,
and weighted recall are stated honestly; percentages are estimates, not mastery.
Reading, navigation, or “I know this already” never manufactures cold recall.

Archive stops future selection, retains old content/history, and invalidates
stale work. The target starts in a fresh namespace: historical Go v4→v5
foundation preservation remains a contract of that historical migration, not an
import step or foundation route in this rewrite. Old stores remain separate.

## Driving it

`cargo test --locked --test concept_policy --test application --test web_render`
checks pure evidence, prerequisite/pace selection, goal controls, and reference
secrecy. Use a real synthetic Worker browser to exercise Map/focus/pause,
reference/assistance, accessible states, archive, and refresh. `svg.constellation`
is decorative; `ol.concept-list` carries the usable navigation/state. The authored
demo is not an unaided memory observation. Live source-backed references and
independent recovery need their own proof.

## Gotchas

Synthetic data proves mechanics only. Follow the current QA recipe, retain
source-bound real observations, and keep protected reference/answer material
behind durable assistance. Never attach live integrations or import historical
stores to make a preview appear complete.
