# Content Type Completeness: Cloze & Free-Response

Priority: high
Status: ready
Estimate: M

## Goal

Enable AI generation and review of cloze (fill-in-the-blank) and free-response question types so users can memorize anything — not just via multiple choice and true/false.

## Non-Goals

- Multimedia content (audio, images, video — future expansion)
- Partial credit scoring (binary correct/incorrect is sufficient)
- Schema migration of existing phrasings (new types for new content only)
- LLM-based semantic grading in v1 (fuzzy string matching first; LLM grading is a follow-up)

## Oracle

- [ ] `generationContracts.ts` type enum includes `'cloze' | 'short-answer'`
- [ ] `promptTemplates.ts` includes generation guidance for cloze (`[___]` blank format) and short-answer
- [ ] `aiGeneration.ts` `prepareGeneratedPhrasings()` validates cloze (blank marker present) and short-answer (correctAnswer non-empty, options optional)
- [ ] `review-phrasing-display.tsx` renders cloze as text-input fill-in-blank; renders short-answer as textarea with submit
- [ ] `reviewToolHelpers.ts` `gradeAnswer()` dispatches by type: exact match for MC/TF/cloze, fuzzy match (case-insensitive, whitespace-normalized) for short-answer
- [ ] Promptfoo evals include cloze and short-answer test cases (NATO as cloze: "N is for [___]"; book theme as free-response)
- [ ] End-to-end: input "NATO phonetic alphabet" → generates mix of all 4 question types → all reviewable

## Notes

**Already supports all 4 types:** schema.ts, review-phrasing-display.tsx TypeScript type
**Blocks today:** generationContracts (MC/TF only), promptTemplates (MC/TF guidance only), pipeline validation (drops non-MC/TF), no cloze/short-answer UI rendering, exact-match-only grading

**Depends on:** Items 003-004 (evals for phrasing generation must exist first so we can measure quality of new question types)
