# Context Packet: Phrasing Generation Evals

## Spec

### Integration Into promptfoo.yaml

The current `evals/promptfoo.yaml` is a single-prompt config targeting `evals/prompts/concept-synthesis.txt`. Phrasing generation uses a different prompt with different input variables, so it cannot share the same config file without restructuring.

**Approach: separate config file.**

Create `evals/promptfoo-phrasing.yaml` alongside the existing `evals/promptfoo.yaml`. The `pnpm eval` script should be updated (or a parallel `pnpm eval:phrasing` added) so both suites run. The CI workflow already handles phrasing-generation in its comparison loop (line 83 of `prompt-eval.yml`).

Rationale: concept-synthesis uses `intentJson` as its sole input variable and a transform that extracts the top-level JSON object. Phrasing generation uses five different template variables (`conceptTitle`, `contentType`, `originIntent`, `existingQuestions`, `targetCount`) and produces `{"phrasings": [...]}`. Mixing them in one file would require conditional transforms and convoluted defaultTest logic. Two files is cleaner.

### Input Format

Each test case provides these `vars` to the prompt template `evals/prompts/phrasing-generation.txt`:

| Variable | Type | Description |
|----------|------|-------------|
| `conceptTitle` | string | The concept being quizzed (e.g. "Confirmation Bias") |
| `contentType` | string | One of: `verbatim`, `enumerable`, `conceptual`, `mixed` |
| `originIntent` | string | Stringified intent JSON (provides broader context) |
| `existingQuestions` | string | Newline-separated list of existing questions, or `"None (generate first phrasings for this concept)"` |
| `targetCount` | string | Number of phrasings to generate (typically "3" or "5") |

### Output Schema

The LLM returns JSON matching `phrasingBatchSchema` from `convex/lib/generationContracts.ts`:

```json
{
  "phrasings": [
    {
      "question": "string",
      "explanation": "string",
      "type": "multiple-choice" | "true-false",
      "options": ["string", ...],  // 2-4 items
      "correctAnswer": "string"    // must be one of options
    }
  ]
}
```

### Output Transform

The `defaultTest.options.transform` must extract the JSON object from possible markdown/thinking wrapping:

```yaml
transform: |
  const jsonMatch = output.match(/\{[\s\S]*\}/);
  return jsonMatch ? jsonMatch[0].trim() : output.trim();
```

This is identical to the concept-synthesis transform.

### Assertion Strategy

Every test case gets two layers of assertions:

1. **Structural (javascript):** Validates schema conformance mechanically. These are non-negotiable pass/fail.
2. **Quality (llm-rubric):** Evaluates subjective dimensions using the same four criteria from `convex/lib/scoring.ts`. These use `threshold: 0.6` (maps to ~3/5 on the scoring scale).

---

## Eval Design

### Structural Assertions (applied to ALL test cases via `defaultTest.assert`)

```yaml
defaultTest:
  options:
    transform: |
      const jsonMatch = output.match(/\{[\s\S]*\}/);
      return jsonMatch ? jsonMatch[0].trim() : output.trim();
  assert:
    # S1: Valid JSON
    - type: is-json

    # S2: Latency budget
    - type: latency
      threshold: 60000

    # S3: Has phrasings array with correct count
    - type: javascript
      value: |
        const data = JSON.parse(output);
        if (!data.phrasings || !Array.isArray(data.phrasings)) return false;
        const target = parseInt(context.vars.targetCount, 10);
        return data.phrasings.length >= target;

    # S4: Each phrasing has required fields with correct types
    - type: javascript
      value: |
        const data = JSON.parse(output);
        const validTypes = ['multiple-choice', 'true-false'];
        return data.phrasings.every(p =>
          typeof p.question === 'string' && p.question.length > 0 &&
          typeof p.explanation === 'string' && p.explanation.length > 0 &&
          validTypes.includes(p.type) &&
          Array.isArray(p.options) &&
          p.options.length >= 2 && p.options.length <= 4 &&
          typeof p.correctAnswer === 'string' &&
          p.options.includes(p.correctAnswer)
        );

    # S5: MC questions have 3-4 options, TF have exactly 2
    - type: javascript
      value: |
        const data = JSON.parse(output);
        return data.phrasings.every(p => {
          if (p.type === 'multiple-choice') return p.options.length >= 3 && p.options.length <= 4;
          if (p.type === 'true-false') return p.options.length === 2 && p.options.includes('True') && p.options.includes('False');
          return false;
        });

    # S6: Question stems under 200 chars
    - type: javascript
      value: |
        const data = JSON.parse(output);
        return data.phrasings.every(p => p.question.length <= 200);
```

### LLM Rubric Assertions (per quality dimension)

These four rubrics mirror the dimensions in `convex/lib/scoring.ts` (`buildScoringPrompt`). Each is applied via `defaultTest.assert` so every test case is evaluated on all four.

```yaml
    # Q1: Standalone clarity
    - type: llm-rubric
      value: |
        Evaluate whether each quiz question in this JSON output can be understood
        WITHOUT any external context. A good standalone question explicitly
        identifies its subject/topic in the stem. A poor question uses pronouns
        like "it" or "this" without referent, or assumes the reader knows what
        concept is being tested.

        The concept being tested is: "{{conceptTitle}}" (content type: {{contentType}}).

        Score 0.0 if questions are completely context-dependent.
        Score 0.5 if questions are mostly clear but occasionally vague.
        Score 1.0 if every question explicitly identifies its topic and is
        fully self-contained.
      threshold: 0.6
      weight: 2

    # Q2: Distractor quality
    - type: llm-rubric
      value: |
        Evaluate the quality of wrong answer options (distractors) in the
        multiple-choice questions in this JSON output.

        Good distractors are semantically adjacent to the correct answer -
        they represent common misconceptions, related-but-wrong concepts,
        or plausible confusions. Bad distractors are obviously wrong,
        unrelated to the topic, or differ only in formatting/punctuation.

        For true-false questions, evaluate whether the statement tests a
        meaningful distinction rather than trivial phrasing.

        The concept is: "{{conceptTitle}}" ({{contentType}}).

        Score 0.0 if distractors are random or trivially wrong.
        Score 0.5 if some distractors are plausible but others are obvious.
        Score 1.0 if all distractors test genuine misconceptions and are
        semantically adjacent to the correct answer.
      threshold: 0.6
      weight: 2

    # Q3: Explanation value
    - type: llm-rubric
      value: |
        Evaluate whether the explanation field in each phrasing teaches WHY
        the correct answer is right, rather than merely restating it.

        A good explanation provides reasoning, addresses why common wrong
        answers fail, and helps the learner build understanding. A poor
        explanation just says "The answer is X" or repeats the question.

        Score 0.0 if explanations just restate the answer.
        Score 0.5 if explanations state the correct fact but lack depth.
        Score 1.0 if explanations teach reasoning and address misconceptions.
      threshold: 0.6
      weight: 1

    # Q4: Difficulty calibration
    - type: llm-rubric
      value: |
        Evaluate whether the questions are at an appropriate difficulty level -
        neither trivially easy nor impossibly obscure.

        Good questions require genuine understanding or recall of the concept.
        Bad questions either ask something any person would know (too easy)
        or require niche expertise beyond the concept scope (too hard).

        The concept is: "{{conceptTitle}}" ({{contentType}}).

        Score 0.0 if questions are all trivial or all unanswerable.
        Score 0.5 if difficulty is uneven or slightly miscalibrated.
        Score 1.0 if questions appropriately test meaningful knowledge.
      threshold: 0.6
      weight: 1
```

### Complete Test Case List (17 cases)

#### Enumerable Content (3 cases)

| # | Description | conceptTitle | contentType | targetCount | Extra assertions |
|---|------------|--------------|-------------|-------------|------------------|
| E1 | NATO Phonetic Alphabet letter | Alpha (NATO Phonetic Alphabet) | enumerable | 3 | Question mentions "NATO" or "phonetic alphabet" |
| E2 | Solar System planet | Jupiter | enumerable | 3 | Question involves planetary facts, not trivia |
| E3 | Periodic Table element | Helium | enumerable | 3 | Distractors are other elements, not random words |

#### Conceptual Content (3 cases)

| # | Description | conceptTitle | contentType | targetCount | Extra assertions |
|---|------------|--------------|-------------|-------------|------------------|
| C1 | Cognitive bias | Confirmation Bias | conceptual | 3 | Mentions decision-making or belief persistence |
| C2 | ML fundamental | Gradient Descent | conceptual | 3 | Technically accurate, not trivialized |
| C3 | Philosophy concept | Trolley Problem (Ethics) | conceptual | 3 | Tests ethical reasoning, not factual recall |

#### Verbatim Content (3 cases)

| # | Description | conceptTitle | contentType | targetCount | Extra assertions |
|---|------------|--------------|-------------|-------------|------------------|
| V1 | Gettysburg Address line | "Four score and seven years ago" (Gettysburg Address, Lincoln) | verbatim | 3 | References Lincoln or Gettysburg |
| V2 | Shakespeare quote | "To be, or not to be" (Hamlet, Act 3 Scene 1) | verbatim | 3 | References Hamlet or Shakespeare |
| V3 | Constitutional text | First Amendment (US Constitution) | verbatim | 3 | Accurate to amendment content |

#### Mixed Content (2 cases)

| # | Description | conceptTitle | contentType | targetCount | Extra assertions |
|---|------------|--------------|-------------|-------------|------------------|
| M1 | WWII event | D-Day (Operation Overlord, June 1944) | mixed | 3 | Historically accurate |
| M2 | Renaissance figure | Leonardo da Vinci | mixed | 5 | Covers art AND science contributions |

#### Edge Cases (4 cases)

| # | Description | conceptTitle | contentType | targetCount | Extra assertions |
|---|------------|--------------|-------------|-------------|------------------|
| X1 | Single-word concept | Democracy | conceptual | 3 | Still produces meaningful questions |
| X2 | Very long concept title | The relationship between mitochondrial DNA inheritance patterns and maternal lineage tracing in forensic genetics | conceptual | 3 | Questions are concise despite long title |
| X3 | Highly technical | Monadic Composition in Haskell | conceptual | 3 | Questions are self-contained even for niche topics |
| X4 | Large batch request | Photosynthesis | conceptual | 5 | All 5 phrasings are distinct (no repetition) |

#### Deduplication (2 cases)

| # | Description | conceptTitle | contentType | targetCount | existingQuestions | Extra assertions |
|---|------------|--------------|-------------|-------------|-------------------|------------------|
| D1 | Avoids repeating existing | Confirmation Bias | conceptual | 3 | "1. What cognitive bias causes people to favor information confirming existing beliefs?" | None of the new questions repeat the existing one |
| D2 | Multiple existing phrasings | Photosynthesis | conceptual | 2 | "1. What process converts CO2 and water into glucose using sunlight?\n2. In photosynthesis, what gas is released as a byproduct?" | New questions cover different aspects |

---

## Example Test Cases

### Example 1: Enumerable — NATO Phonetic Alphabet Letter

```yaml
- description: "Enumerable: NATO phonetic alphabet letter"
  vars:
    conceptTitle: "Alpha (NATO Phonetic Alphabet)"
    contentType: "enumerable"
    originIntent: '{"content_type":"enumerable","goal":"memorize","atomic_units":["Alpha","Bravo","Charlie","Delta","Echo"],"synthesis_ops":["letter-to-word mapping"],"confidence":0.95}'
    existingQuestions: "None (generate first phrasings for this concept)"
    targetCount: "3"
  assert:
    # Topic identification: questions should reference NATO or phonetic alphabet
    - type: javascript
      value: |
        const data = JSON.parse(output);
        const text = JSON.stringify(data).toLowerCase();
        return text.includes('nato') || text.includes('phonetic alphabet');
```

### Example 2: Conceptual — Confirmation Bias

```yaml
- description: "Conceptual: cognitive bias with existing phrasings"
  vars:
    conceptTitle: "Confirmation Bias"
    contentType: "conceptual"
    originIntent: '{"content_type":"conceptual","goal":"understand","atomic_units":["confirmation bias","anchoring bias","availability heuristic"],"synthesis_ops":["how they affect decision making"],"confidence":0.9}'
    existingQuestions: "1. What cognitive bias causes people to favor information that confirms their existing beliefs?"
    targetCount: "3"
  assert:
    # Deduplication: new questions must not repeat the existing one
    - type: javascript
      value: |
        const data = JSON.parse(output);
        const existing = "What cognitive bias causes people to favor information that confirms their existing beliefs?";
        return data.phrasings.every(p => {
          // Fuzzy check: no phrasing should be >70% similar to existing
          const words = p.question.toLowerCase().split(/\s+/);
          const existingWords = existing.toLowerCase().split(/\s+/);
          const overlap = words.filter(w => existingWords.includes(w)).length;
          return overlap / Math.max(words.length, existingWords.length) < 0.7;
        });
    # Should reference bias or decision-making
    - type: javascript
      value: |
        const data = JSON.parse(output);
        const text = JSON.stringify(data).toLowerCase();
        return text.includes('bias') || text.includes('belief') || text.includes('decision');
```

### Example 3: Verbatim — Gettysburg Address

```yaml
- description: "Verbatim: Gettysburg Address memorization"
  vars:
    conceptTitle: '"Four score and seven years ago" (Gettysburg Address, Lincoln)'
    contentType: "verbatim"
    originIntent: '{"content_type":"verbatim","goal":"memorize","atomic_units":["Four score and seven years ago","government of the people, by the people, for the people"],"synthesis_ops":["historical context","rhetorical devices"],"confidence":0.95}'
    existingQuestions: "None (generate first phrasings for this concept)"
    targetCount: "3"
  assert:
    # Must reference Lincoln, Gettysburg, or Civil War for context
    - type: javascript
      value: |
        const data = JSON.parse(output);
        const text = JSON.stringify(data).toLowerCase();
        return text.includes('lincoln') || text.includes('gettysburg') || text.includes('civil war');
    # Verbatim questions should test recall of specific text
    - type: llm-rubric
      value: |
        These questions test memorization of the Gettysburg Address by Abraham Lincoln.
        Evaluate whether the questions test RECALL of specific phrases or sequences
        from the speech, rather than general historical knowledge about the Civil War.
        Good verbatim questions ask "what comes next after..." or "which phrase appears in..."
        Score 0.0 if questions are generic history trivia.
        Score 0.5 if questions reference the speech but don't test specific text recall.
        Score 1.0 if questions directly test recall of specific lines/phrases.
      threshold: 0.6
```

### Example 4: Edge Case — Large Batch Distinctness

```yaml
- description: "Edge: large batch request - all phrasings must be distinct"
  vars:
    conceptTitle: "Photosynthesis"
    contentType: "conceptual"
    originIntent: '{"content_type":"conceptual","goal":"understand","atomic_units":["photosynthesis"],"synthesis_ops":["light reactions","Calvin cycle","chloroplast structure"],"confidence":0.9}'
    existingQuestions: "None (generate first phrasings for this concept)"
    targetCount: "5"
  assert:
    # All 5 questions must be semantically distinct
    - type: javascript
      value: |
        const data = JSON.parse(output);
        const questions = data.phrasings.map(p => p.question.toLowerCase());
        // Check all pairs for high overlap
        for (let i = 0; i < questions.length; i++) {
          for (let j = i + 1; j < questions.length; j++) {
            const wordsA = new Set(questions[i].split(/\s+/));
            const wordsB = new Set(questions[j].split(/\s+/));
            const intersection = [...wordsA].filter(w => wordsB.has(w)).length;
            const similarity = intersection / Math.min(wordsA.size, wordsB.size);
            if (similarity > 0.8) return { pass: false, score: 0, reason: `Questions ${i+1} and ${j+1} are too similar (${(similarity*100).toFixed(0)}% word overlap)` };
          }
        }
        return true;
    # Should cover different aspects of photosynthesis
    - type: llm-rubric
      value: |
        These 5 questions should test DIFFERENT aspects of photosynthesis
        (e.g., inputs/outputs, light reactions, Calvin cycle, chloroplast
        structure, comparison to respiration). They should not all ask
        the same thing rephrased slightly differently.
        Score 0.0 if all questions test the same narrow fact.
        Score 0.5 if there is some variety but significant overlap.
        Score 1.0 if each question tests a distinct aspect.
      threshold: 0.6
```

### Example 5: Highly Technical — Monadic Composition

```yaml
- description: "Edge: highly technical niche topic"
  vars:
    conceptTitle: "Monadic Composition in Haskell"
    contentType: "conceptual"
    originIntent: '{"content_type":"conceptual","goal":"understand","atomic_units":["monadic composition","bind operator","Kleisli arrows"],"synthesis_ops":["relationship to functors","practical use in IO"],"confidence":0.8}'
    existingQuestions: "None (generate first phrasings for this concept)"
    targetCount: "3"
  assert:
    # Standalone check: questions must be understandable to someone who knows Haskell
    # (not just "what is this?" without context)
    - type: javascript
      value: |
        const data = JSON.parse(output);
        const text = JSON.stringify(data).toLowerCase();
        return text.includes('haskell') || text.includes('monad') || text.includes('functional');
    # Technical accuracy
    - type: llm-rubric
      value: |
        These questions test knowledge of monadic composition in Haskell.
        Evaluate technical accuracy: do the questions, correct answers,
        and explanations correctly represent how monads work in Haskell?
        Common errors: confusing monads with functors, incorrect bind
        operator semantics, wrong type signatures.
        Score 0.0 if there are factual errors about monads.
        Score 0.5 if technically correct but superficial.
        Score 1.0 if technically accurate and tests genuine understanding.
      threshold: 0.6
```

---

## CI Integration

### Changes to `prompt-eval.yml`

The CI workflow at `.github/workflows/prompt-eval.yml` already triggers on `evals/**` and `convex/lib/promptTemplates.ts` changes. The new config file `evals/promptfoo-phrasing.yaml` will be picked up by the `evals/**` glob.

However, the eval run step (line 74) only runs `evals/promptfoo.yaml`. It must be updated to also run the phrasing config.

**Change the eval step** (line 74) from:

```yaml
npx promptfoo eval -c evals/promptfoo.yaml -o eval-results.json -j 2 $FILTER_ARG
```

To:

```yaml
npx promptfoo eval -c evals/promptfoo.yaml -o eval-results-synthesis.json -j 2 $FILTER_ARG
npx promptfoo eval -c evals/promptfoo-phrasing.yaml -o eval-results-phrasing.json -j 2 $FILTER_ARG
# Merge results for downstream steps
jq -s '{ results: { results: ([.[].results.results] | add), stats: .[0].results.stats } }' eval-results-synthesis.json eval-results-phrasing.json > eval-results.json
```

**Update the artifact upload** to include both individual result files:

```yaml
path: |
  eval-results.json
  eval-results-synthesis.json
  eval-results-phrasing.json
```

**Update `package.json` scripts:**

```json
{
  "eval": "promptfoo eval -c evals/promptfoo.yaml && promptfoo eval -c evals/promptfoo-phrasing.yaml",
  "eval:synthesis": "promptfoo eval -c evals/promptfoo.yaml",
  "eval:phrasing": "promptfoo eval -c evals/promptfoo-phrasing.yaml"
}
```

The `compare-prompts.ts` script (line 11) already supports `--target phrasing-generation` and handles it in its comparison loop. No changes needed there.

---

## Implementation Sequence

1. **Create `evals/promptfoo-phrasing.yaml`** with the full config:
   - `prompts` pointing to `file://prompts/phrasing-generation.txt`
   - `providers` matching production (same `openrouter:google/gemini-3-pro-preview` config as concept-synthesis)
   - `defaultTest` with transform, structural assertions (S1-S6), and quality rubrics (Q1-Q4)
   - All 17 test cases with their vars and per-case assertions

2. **Update `package.json`** scripts: add `eval:phrasing` and `eval:synthesis` commands, update `eval` to run both.

3. **Update `.github/workflows/prompt-eval.yml`** to run both eval configs and merge results.

4. **Run baseline eval locally** (`pnpm eval:phrasing`) and capture results. Record pass rate and average LLM scores in a comment at the top of the YAML file.

5. **Commit all changes** in a single PR with baseline results documented.

---

## Verification

### Local Execution

```bash
# Run just phrasing evals
pnpm eval:phrasing

# View results in browser
pnpm eval:view

# Run a single test case by description filter
npx promptfoo eval -c evals/promptfoo-phrasing.yaml --filter "NATO"

# Run with verbose output to debug assertion failures
npx promptfoo eval -c evals/promptfoo-phrasing.yaml -o results.json -j 1 --verbose
```

### Interpreting Results

Each test case produces:
- **Pass/fail** overall (all assertions must pass)
- **Component scores** for each assertion:
  - `is-json`: binary pass/fail
  - `javascript`: binary pass/fail with reason on failure
  - `llm-rubric`: 0.0-1.0 score, passes if >= threshold (0.6)
  - `latency`: binary pass/fail against 60s threshold

**Key metrics to track:**
- Overall pass rate (target: >= 80% on baseline, improve from there)
- Average LLM rubric score per dimension (standalone, distractors, explanation, difficulty)
- Latency p95

### Baseline Capture

After first run, add a comment block to the top of `promptfoo-phrasing.yaml`:

```yaml
# BASELINE (captured YYYY-MM-DD):
# Pass rate: XX/17 (XX%)
# Avg standalone: X.XX/1.0
# Avg distractors: X.XX/1.0
# Avg explanation: X.XX/1.0
# Avg difficulty: X.XX/1.0
# Latency p95: XXXXms
```

This becomes the floor. Any prompt change that regresses below baseline should fail review.

---

## Files Modified/Created

| File | Action |
|------|--------|
| `evals/promptfoo-phrasing.yaml` | **CREATE** - Full phrasing generation eval config |
| `package.json` | **EDIT** - Add `eval:phrasing`, `eval:synthesis` scripts, update `eval` |
| `.github/workflows/prompt-eval.yml` | **EDIT** - Run both eval configs, merge results |

## Key Source Files (read-only reference)

| File | Purpose |
|------|---------|
| `evals/prompts/phrasing-generation.txt` | The prompt template being evaluated |
| `convex/lib/generationContracts.ts` | Output schema (`phrasingBatchSchema`) |
| `convex/lib/scoring.ts` | LLM-as-judge dimensions to mirror |
| `convex/lib/promptTemplates.ts` | `buildPhrasingGenerationPrompt()` — canonical prompt builder |
| `evals/promptfoo.yaml` | Existing concept-synthesis evals (pattern reference) |
