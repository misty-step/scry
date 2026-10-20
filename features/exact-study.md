# Exact text practice

Behavior authority: [US-016](../SPEC.md#us-016-learn-a-version-unit-by-unit),
[US-017](../SPEC.md#us-017-get-an-exact-verdict-and-a-word-by-word-diff),
and [US-007](../SPEC.md#us-007-correct-a-grade-in-one-tap).

Build the Rust CLI and isolated Worker using [Scry QA](../.agents/skills/scry-qa/SKILL.md).
Follow the [command and API reference](../docs/owner-api.md#text-study-and-practice)
with caller-authored text. Search, supply a Version, learn it, answer its saved
Prompt, inspect held feedback, then advance with `next --after PROMPT`.
Use explicit Section and whole Recitation, assistance and Correction. Pause,
resume and drop the Study, then inspect history and deletion refusal.

`qa/walk --stories "US-016 US-017"` drives every numbered criterion through the
compiled CLI and built Worker with actual SQLite and local R2. It needs no
browser. `node --test qa/study.test.mjs` reuses that journey. Native Study,
exact-v2 and pinned FSRS tests cover policy and reconstruction. The local clock
exists only in the QA adapter, outside the production Worker.

Source: `src/study.rs`, shared `src/engine.rs`, `src/api.rs`, `src/runtime.rs`,
`src/persistence.rs`, and `src/bin/scry.rs`. The existing grader and scheduler
remain in `src/grading` and `src/learning.rs`.

The fixture contains authored synthetic clauses under Creed and Missal metadata.
It establishes no private canonical wording or physical-phone acceptance. Hosted
API admission under #335 and independent hosted restore remain unverified.
