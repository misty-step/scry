# Phrasing Generation Evals

Priority: high
Status: ready
Estimate: M

## Goal

The core product output — quiz questions generated from user content — has **zero evals**. Add a comprehensive promptfoo eval suite for phrasing generation covering distractor quality, standalone clarity, explanation value, and difficulty calibration.

## Non-Goals

- Evaluating cloze or free-response (those types don't exist yet — item 006)
- Building new eval infrastructure (promptfoo already works)
- Changing the generation pipeline (eval first, then improve)

## Oracle

- [ ] `evals/promptfoo.yaml` includes a `phrasing-generation` eval config section using `evals/prompts/phrasing-generation.txt` as the prompt
- [ ] Minimum 15 phrasing generation test cases covering:
  - Enumerable content (NATO alphabet, planets) → 3 cases
  - Conceptual content (cognitive biases, ML fundamentals) → 3 cases
  - Verbatim content (Gettysburg Address, Shakespeare) → 3 cases
  - Mixed content (WWII history) → 2 cases
  - Edge cases (single concept, very short input, very long input) → 4 cases
- [ ] Each test case asserts on:
  - `is-json` — valid JSON output
  - `llm-rubric` for **distractor quality** ("MC options test plausible misconceptions, not obviously wrong answers") — score >= 3/5
  - `llm-rubric` for **standalone clarity** ("question is understandable without external context") — score >= 3/5
  - `llm-rubric` for **explanation value** ("explanation teaches WHY the answer is correct, not just restates it") — score >= 3/5
  - `javascript` for structural validation (correct fields, option count, type field)
- [ ] `pnpm eval` runs the full suite including phrasing generation
- [ ] `prompt-eval.yml` CI workflow triggers on changes to phrasing generation prompt template
- [ ] Baseline results captured — current pass rate and average LLM scores documented

## Notes

**Why this is urgent:** Concept synthesis has 20 eval cases. Phrasing generation has ZERO. But phrasings are what users actually interact with — if the questions are bad, the product is bad regardless of how good concept extraction is.

**Existing scoring system:** `convex/lib/scoring.ts` already has an LLM-as-judge that evaluates standalone, distractors, explanation, and difficulty. The promptfoo evals should use similar rubrics for consistency.

**Eval structure:**
- Input: a concept (title + description + content type)
- Output: array of phrasings (question, options, correctAnswer, explanation, type)
- Assertions: structural validity + quality rubrics

**Sequence:**
1. Write phrasing generation test cases in promptfoo.yaml
2. Run baseline eval to capture current quality
3. Add phrasing prompt to CI trigger paths in prompt-eval.yml
4. Document baseline scores as the floor to improve against
