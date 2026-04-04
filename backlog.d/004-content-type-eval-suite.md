# Content Type Eval Suite

Priority: high
Status: ready
Estimate: M

## Goal

Add eval coverage for every content type the user cares about — poetry, prayers, book analysis, vocabulary, trivia — across the full generation pipeline (intent extraction → concept synthesis → phrasing generation). Currently only enumerable, conceptual, and verbatim content have eval cases, and none of the user's specific domains are represented.

## Non-Goals

- Multimedia content evals (audio, images, video — future)
- Changing the generation pipeline (eval gaps inform future changes)
- Building a custom eval framework (use promptfoo)

## Oracle

- [ ] **Poetry evals** — 2 cases: one short poem (e.g., Frost "The Road Not Taken"), one longer (e.g., Shelley "Ozymandias"). Asserts: verbatim lines preserved, questions test specific line recall and thematic understanding
- [ ] **Prayer evals** — 2 cases: Hail Mary, Our Father. Asserts: exact wording preserved in concepts, questions test word-perfect recall (not paraphrase)
- [ ] **Book analysis evals** — 2 cases: one fiction (themes of a novel), one non-fiction (key arguments of a book). Asserts: concepts capture themes/arguments not just plot summary, questions test understanding not trivia
- [ ] **Vocabulary evals** — 2 cases: one foreign language vocabulary set, one English SAT-level words. Asserts: definitions accurate, questions test usage in context not just definition recall
- [ ] **NATO phonetic alphabet eval** — 1 case: full 26-letter alphabet. Asserts: each letter-word pairing preserved as individual concept, questions test both directions (letter→word and word→letter)
- [ ] **Trivia/facts evals** — 2 cases: historical dates, science facts. Asserts: facts accurate, questions test recall with plausible distractors
- [ ] All evals run end-to-end through concept synthesis AND phrasing generation stages
- [ ] Pass rate >= 80% on first run (baseline capture; improve from there)
- [ ] Results documented: which content types perform well vs. poorly with current prompts

## Notes

**Current eval coverage:**
- Enumerable: 3 cases (NATO, planets, presidents) — but only concept synthesis, not phrasing generation
- Conceptual: 3 cases (quantum, biases, ML)
- Verbatim: 2 cases (Gettysburg, Shakespeare)
- Poetry: 0 cases
- Prayers: 0 cases
- Book analysis: 0 cases
- Vocabulary: 0 cases
- Trivia: 0 cases

**Key quality signals per content type:**
- Verbatim (poetry, prayers): Must preserve exact wording. Paraphrasing = failure.
- Enumerable (NATO, vocabulary): Must create one concept per item. Lumping = failure.
- Conceptual (books, analysis): Must extract themes/arguments. Surface-level = failure.
- Trivia: Must have factually accurate distractors. Made-up facts = failure.

**Depends on:** Item 003 (phrasing generation evals establish the eval pattern; this extends it to all content types)
