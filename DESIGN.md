# DESIGN.md - LLM Operations Infrastructure

## Architecture Overview

**Selected Approach**: Promptfoo-Centric Evaluation + Langfuse Observability

**Rationale**: Leverages battle-tested Promptfoo for evaluation (YAML configs, CI/CD, red-teaming) while using existing Langfuse integration for observability. Avoids building custom eval runner—complexity moved to configuration, not code.

**Core Modules**:
- `convex/lib/langfuse.ts`: Observability singleton (✅ complete)
- `convex/lib/prompts.ts`: Prompt resolution with fallback (✅ complete)
- `convex/lib/aiProviders.ts`: Google + OpenRouter providers (✅ complete)
- `evals/promptfoo.yaml`: Evaluation configuration (new)
- `evals/assertions/`: Custom LLM-as-judge assertions (new)
- `scripts/eval.ts`: CLI wrapper for `pnpm eval:*` commands (new)

**Data Flow**:
```
Developer/Agent → pnpm eval:run → Promptfoo CLI → AI Provider → LLM-as-judge
                                        ↓
                               Langfuse (traces)
                                        ↓
                               Results (JSON/console)
```

**Key Design Decisions**:
1. **Promptfoo over custom runner**: Proven tool, YAML-native, CI/CD friendly
2. **LLM-as-judge via assertions**: Custom assertion types for quality/coverage
3. **No Convex eval storage**: Results in Langfuse + git (YAML results files)
4. **Provider-agnostic**: Same evals run against any OpenRouter model

---

## Alternative Architectures Considered

### Alternative A: Promptfoo-Centric (SELECTED)
```
promptfoo.yaml → Promptfoo CLI → OpenRouter → Results
                       ↓
                 Langfuse traces
```
- **Pros**: Battle-tested, YAML configs, CI/CD native, red-teaming built-in
- **Cons**: External dependency, learning curve for YAML syntax
- **Ousterhout Analysis**: Deep module—simple CLI interface hides complex evaluation logic

### Alternative B: Convex-Native Evaluation
```
Convex tables → Convex action → OpenRouter → Convex results
                      ↓
                Langfuse traces
```
- **Pros**: Everything in one place, type-safe, reactive
- **Cons**: Building eval runner from scratch, no CI/CD integration, reinventing wheel
- **Ousterhout Analysis**: Shallow module—we'd expose similar complexity to what we implement

### Alternative C: Hybrid (Promptfoo + Convex Storage)
```
Convex datasets → Promptfoo → OpenRouter → Convex results
                       ↓
                 Langfuse traces
```
- **Pros**: Persistent datasets, query history
- **Cons**: Complex integration, data sync issues, dual storage
- **Ousterhout Analysis**: Information leakage—Convex schema bleeds into Promptfoo config

**Selected**: Alternative A because it minimizes code while maximizing capability. Promptfoo handles the hard parts; we configure, not build.

---

## Module Design

### Module: Promptfoo Configuration

**Responsibility**: Define evaluation test suites, providers, and assertions in YAML.

**File Structure**:
```
evals/
├── promptfoo.yaml           # Main config (providers, default assertions)
├── datasets/
│   ├── core.yaml            # Core test cases (NATO, planets, etc.)
│   ├── coverage.yaml        # Coverage-focused tests
│   └── edge-cases.yaml      # Edge case tests
├── assertions/
│   ├── quality-judge.ts     # LLM-as-judge for factuality/clarity
│   └── coverage-score.ts    # Semantic coverage assertion
└── prompts/
    └── scry-generation.txt  # Prompt template for evaluation
```

**Core Configuration** (`evals/promptfoo.yaml`):
```yaml
description: "Scry Quiz Generation Evaluation"

providers:
  - id: google-gemini
    config:
      apiKey: ${GOOGLE_AI_API_KEY}
      model: gemini-2.5-pro

  - id: openrouter-claude
    config:
      apiKeyEnvar: OPENROUTER_API_KEY
      model: anthropic/claude-3.5-sonnet

prompts:
  - file://prompts/scry-generation.txt

defaultTest:
  assert:
    - type: javascript
      value: file://assertions/quality-judge.ts
    - type: javascript
      value: file://assertions/coverage-score.ts

tests: file://datasets/core.yaml
```

**Dataset Format** (`evals/datasets/core.yaml`):
```yaml
- vars:
    userInput: "NATO Phonetic Alphabet"
  assert:
    - type: javascript
      value: "output.concepts.length >= 26"
    - type: llm-rubric
      value: "Each concept should represent exactly one NATO letter code (Alpha, Bravo, etc.)"

- vars:
    userInput: "The planets of the solar system"
  assert:
    - type: javascript
      value: "output.concepts.length >= 8"
    - type: llm-rubric
      value: "Should include Mercury through Neptune. Pluto optional."

- vars:
    userInput: "Isaac Asimov's Three Laws of Robotics"
  assert:
    - type: javascript
      value: "output.concepts.length >= 3"
    - type: contains
      value: "First Law"
```

---

### Module: Quality Judge Assertion

**Responsibility**: Score generated content on factuality, educational value, and clarity using LLM-as-judge pattern.

**Public Interface**:
```typescript
// evals/assertions/quality-judge.ts
interface QualityAssertionResult {
  pass: boolean;
  score: number;        // 0-100
  reason: string;
  breakdown: {
    factuality: number;       // 0-100 (40% weight)
    educationalValue: number; // 0-100 (35% weight)
    clarity: number;          // 0-100 (25% weight)
  };
}

export default async function qualityJudge(
  output: string,
  context: AssertionContext
): Promise<QualityAssertionResult>;
```

**Internal Implementation**:
```typescript
// Pseudocode for quality-judge.ts

const JUDGE_PROMPT = `
You are evaluating quiz content for a spaced repetition app.

Content to evaluate:
{{content}}

Original user prompt:
{{userInput}}

Score each dimension 0-100:

1. FACTUALITY (40%): Is the content accurate and verifiable?
   - 90-100: All facts correct, could be verified with authoritative sources
   - 70-89: Mostly correct, minor inaccuracies
   - 50-69: Some errors that could mislead learners
   - 0-49: Significant factual errors

2. EDUCATIONAL VALUE (35%): Does it test meaningful knowledge?
   - 90-100: Tests core concepts, promotes deep understanding
   - 70-89: Tests useful knowledge, some peripheral topics
   - 50-69: Tests trivia or surface-level facts
   - 0-49: Tests irrelevant or misleading information

3. CLARITY (25%): Are questions well-formed and unambiguous?
   - 90-100: Crystal clear, no ambiguity, obvious correct answer
   - 70-89: Clear with minor ambiguity
   - 50-69: Some confusion possible
   - 0-49: Ambiguous or poorly worded

Return JSON only:
{
  "factuality": <number>,
  "educationalValue": <number>,
  "clarity": <number>,
  "reason": "<one sentence explanation>"
}
`;

async function qualityJudge(output, context) {
  1. Parse output as JSON (handle both string and object)
  2. Extract concepts/questions from output
  3. Format content for judge prompt
  4. Call cheap judge model (gpt-4o-mini or gemini-flash)
  5. Parse JSON response
  6. Calculate weighted score:
     score = (factuality * 0.4) + (educationalValue * 0.35) + (clarity * 0.25)
  7. Return { pass: score >= 70, score, reason, breakdown }
```

**Error Handling**:
- JSON parse failure → return { pass: false, score: 0, reason: "Output not valid JSON" }
- Judge API error → retry once, then return { pass: false, score: 0, reason: "Judge unavailable" }
- Empty output → return { pass: false, score: 0, reason: "No content generated" }

---

### Module: Coverage Score Assertion

**Responsibility**: Measure what percentage of expected topics are covered by generated concepts.

**Public Interface**:
```typescript
// evals/assertions/coverage-score.ts
interface CoverageAssertionResult {
  pass: boolean;
  score: number;        // 0-100 (percentage covered)
  reason: string;
  breakdown: {
    expectedTopics: string[];
    coveredTopics: string[];
    missingTopics: string[];
  };
}

export default async function coverageScore(
  output: string,
  context: AssertionContext
): Promise<CoverageAssertionResult>;
```

**Internal Implementation**:
```typescript
// Pseudocode for coverage-score.ts

const TOPIC_EXTRACTION_PROMPT = `
Given this user prompt, list the key topics/items that should be covered:

User prompt: "{{userInput}}"

Return JSON array of topic strings. Be specific.
Example for "NATO Phonetic Alphabet": ["Alpha", "Bravo", "Charlie", ...]
Example for "US Presidents": ["George Washington", "John Adams", ...]

Return: ["topic1", "topic2", ...]
`;

const COVERAGE_CHECK_PROMPT = `
Check if this generated content covers the expected topic.

Expected topic: "{{topic}}"
Generated concepts: {{concepts}}

Does any concept substantially cover this topic?
Return JSON: { "covered": true/false, "matchingConcept": "concept title or null" }
`;

async function coverageScore(output, context) {
  1. Parse output to extract concept titles
  2. If expectedTopics provided in context.vars, use those
     Else call LLM to extract expected topics from userInput
  3. For each expected topic:
     - Check if any concept title matches (fuzzy string match)
     - If no match, call LLM to check semantic coverage
     - Track covered vs missing
  4. Calculate coverage = (coveredCount / expectedCount) * 100
  5. Return { pass: coverage >= 80, score: coverage, reason, breakdown }
```

**Optimization**:
- Batch coverage checks in single LLM call (up to 10 topics at once)
- Use embedding similarity for fast pre-filter before LLM check
- Cache topic extractions for repeated test runs

---

### Module: CLI Wrapper

**Responsibility**: Provide `pnpm eval:*` commands that wrap Promptfoo CLI with project defaults.

**Public Interface** (`scripts/eval.ts`):
```bash
# Run evaluation
pnpm eval:run                           # Run all tests with default provider
pnpm eval:run --provider=openrouter     # Specific provider
pnpm eval:run --dataset=coverage        # Specific dataset
pnpm eval:run --model=claude-3.5-sonnet # Specific model

# Compare models
pnpm eval:compare gemini claude         # A/B test two models

# View results
pnpm eval:view                          # Open Promptfoo web UI
pnpm eval:results                       # Print latest results JSON
```

**Implementation**:
```typescript
// scripts/eval.ts
import { exec } from 'child_process';

const commands = {
  run: (args) => {
    const provider = args.provider || 'google-gemini';
    const dataset = args.dataset || 'core';
    const cmd = `npx promptfoo eval -c evals/promptfoo.yaml --providers ${provider}`;
    exec(cmd, ...);
  },

  compare: (modelA, modelB) => {
    const cmd = `npx promptfoo eval -c evals/promptfoo.yaml --providers ${modelA},${modelB}`;
    exec(cmd, ...);
  },

  view: () => exec('npx promptfoo view'),

  results: () => {
    // Read latest output from .promptfoo/output/
    const latest = findLatestResults();
    console.log(JSON.stringify(latest, null, 2));
  }
};
```

---

## Integration Points

### Environment Variables

```bash
# Required for evaluation
GOOGLE_AI_API_KEY=...           # For Gemini provider
OPENROUTER_API_KEY=...          # For OpenRouter provider (optional)

# For LLM-as-judge (uses same keys)
# Judge model: gpt-4o-mini or gemini-2.0-flash (cheap, fast)

# Langfuse (for tracing eval runs)
LANGFUSE_SECRET_KEY=...
LANGFUSE_PUBLIC_KEY=...
```

### Langfuse Integration

Eval runs should appear in Langfuse for observability:

```typescript
// In eval runner (automatic via Promptfoo + Langfuse SDK)
// Each eval case creates a trace with:
// - name: "eval-{dataset}-{timestamp}"
// - tags: ["eval", dataset, provider]
// - input: userInput
// - output: generated concepts
// - scores: { quality, coverage }
```

### CI/CD Integration

```yaml
# .github/workflows/eval.yml
name: LLM Evaluation

on:
  pull_request:
    paths:
      - 'convex/lib/promptTemplates.ts'
      - 'evals/**'
  workflow_dispatch:

jobs:
  eval:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v2
      - run: pnpm install
      - run: pnpm eval:run
        env:
          GOOGLE_AI_API_KEY: ${{ secrets.GOOGLE_AI_API_KEY }}
      - name: Check results
        run: |
          PASS_RATE=$(pnpm eval:results | jq '.stats.successes / .stats.total')
          if (( $(echo "$PASS_RATE < 0.8" | bc -l) )); then
            echo "Eval pass rate $PASS_RATE is below 80% threshold"
            exit 1
          fi
```

---

## Implementation Pseudocode

### Eval Run Flow

```pseudocode
function runEvaluation(options):
  1. Load configuration
     - config = parseYAML("evals/promptfoo.yaml")
     - dataset = loadDataset(options.dataset || "core")
     - provider = resolveProvider(options.provider || config.defaultProvider)

  2. Initialize tracing (if Langfuse configured)
     - trace = langfuse.trace({ name: "eval-run", tags: ["eval", dataset.name] })

  3. For each test case in dataset:
     a. Build prompt
        - prompt = loadPrompt("evals/prompts/scry-generation.txt")
        - compiled = interpolate(prompt, testCase.vars)

     b. Call AI provider
        - startTime = now()
        - response = await provider.generateObject({
            prompt: compiled,
            schema: conceptIdeasSchema
          })
        - latency = now() - startTime

     c. Run assertions
        - qualityResult = await qualityJudge(response, testCase)
        - coverageResult = await coverageScore(response, testCase)
        - customResults = await runCustomAssertions(response, testCase.assert)

     d. Record result
        - passed = qualityResult.pass && coverageResult.pass && customResults.allPass
        - results.push({
            vars: testCase.vars,
            output: response,
            pass: passed,
            scores: { quality: qualityResult.score, coverage: coverageResult.score },
            latency,
            assertions: [qualityResult, coverageResult, ...customResults]
          })

     e. Update trace
        - trace.span({ name: testCase.vars.userInput, output: response })

  4. Aggregate results
     - summary = {
         total: results.length,
         passed: results.filter(r => r.pass).length,
         failed: results.filter(r => !r.pass).length,
         avgQuality: mean(results.map(r => r.scores.quality)),
         avgCoverage: mean(results.map(r => r.scores.coverage)),
         avgLatency: mean(results.map(r => r.latency))
       }

  5. Output results
     - writeJSON(".promptfoo/output/eval-{timestamp}.json", { summary, results })
     - console.log(formatSummary(summary))
     - trace.update({ output: summary })
     - await flushLangfuse()

  6. Return exit code
     - return summary.passed === summary.total ? 0 : 1
```

### Quality Judge Flow

```pseudocode
function qualityJudge(output, context):
  1. Parse and validate output
     - concepts = parseJSON(output).concepts
     - if !concepts or concepts.length === 0:
         return { pass: false, score: 0, reason: "No concepts generated" }

  2. Format content for judge
     - content = concepts.map(c => `${c.title}: ${c.description}`).join("\n")

  3. Build judge prompt
     - prompt = JUDGE_PROMPT
         .replace("{{content}}", content)
         .replace("{{userInput}}", context.vars.userInput)

  4. Call judge model
     - judgeModel = getJudgeModel()  // gpt-4o-mini or gemini-flash
     - response = await judgeModel.generate(prompt)

  5. Parse judge response
     - scores = parseJSON(response)
     - if !scores.factuality or !scores.educationalValue or !scores.clarity:
         return { pass: false, score: 0, reason: "Judge returned invalid format" }

  6. Calculate weighted score
     - weightedScore = (scores.factuality * 0.4) +
                       (scores.educationalValue * 0.35) +
                       (scores.clarity * 0.25)

  7. Return result
     - return {
         pass: weightedScore >= 70,
         score: weightedScore,
         reason: scores.reason,
         breakdown: {
           factuality: scores.factuality,
           educationalValue: scores.educationalValue,
           clarity: scores.clarity
         }
       }
```

---

## File Organization

```
scry/
├── evals/
│   ├── promptfoo.yaml              # Main Promptfoo configuration
│   ├── datasets/
│   │   ├── core.yaml               # Core test cases (NATO, planets, etc.)
│   │   ├── coverage.yaml           # Coverage-focused tests
│   │   └── edge-cases.yaml         # Edge case tests
│   ├── assertions/
│   │   ├── quality-judge.ts        # LLM-as-judge quality scorer
│   │   ├── coverage-score.ts       # Semantic coverage checker
│   │   └── index.ts                # Export all assertions
│   └── prompts/
│       └── scry-generation.txt     # Prompt template for testing
├── scripts/
│   └── eval.ts                     # CLI wrapper (pnpm eval:*)
├── convex/
│   └── lib/
│       ├── langfuse.ts             # ✅ Complete
│       ├── prompts.ts              # ✅ Complete
│       └── aiProviders.ts          # ✅ Complete
└── package.json                    # Add eval:* scripts
```

**Modifications to existing files**:
- `package.json` - Add scripts:
  ```json
  {
    "scripts": {
      "eval:run": "tsx scripts/eval.ts run",
      "eval:compare": "tsx scripts/eval.ts compare",
      "eval:view": "npx promptfoo view",
      "eval:results": "tsx scripts/eval.ts results"
    }
  }
  ```

---

## Testing Strategy

### Unit Tests
- `assertions/quality-judge.test.ts`: Mock LLM responses, verify scoring logic
- `assertions/coverage-score.test.ts`: Test topic extraction and matching

### Integration Tests
- Run evaluation against test fixtures (not live LLM)
- Verify YAML parsing and assertion wiring
- Test CLI wrapper commands

### Live Evaluation
- Manual runs with `pnpm eval:run`
- CI runs on prompt template changes
- Compare models with `pnpm eval:compare`

**Coverage Targets**:
- Assertions: 90% (critical scoring logic)
- CLI wrapper: 70% (mostly passthrough)

---

## Performance Considerations

**Expected Load**:
- 10-50 eval cases per run
- 2-5 runs per day during active iteration
- Cost: ~$0.10-0.50 per full eval run (depending on judge model)

**Optimizations**:
- Batch judge calls (multiple concepts in one prompt)
- Cache topic extractions for repeated inputs
- Use cheap judge model (gpt-4o-mini: $0.15/1M tokens)
- Parallel test case execution (Promptfoo default)

**Latency Targets**:
- Single test case: <10s (including judge calls)
- Full core dataset (10 cases): <60s
- Full eval with comparison: <120s

---

## Security Considerations

**Secrets Management**:
- API keys in environment variables only
- Never logged or included in eval results
- CI secrets stored in GitHub Secrets

**Eval Data**:
- Test inputs may contain sensitive examples
- Results files excluded from git (`.promptfoo/output/`)
- Langfuse traces respect existing privacy settings

---

## Migration Path

### Week 1 (Complete)
- [x] Langfuse singleton
- [x] Prompt resolution with fallback
- [x] OpenRouter provider
- [x] Basic tracing in processJob

### Week 2 (Next)
- [ ] Install Promptfoo: `pnpm add -D promptfoo`
- [ ] Create `evals/promptfoo.yaml`
- [ ] Create `evals/datasets/core.yaml` (migrate from `convex/evals/cases.ts`)
- [ ] Create basic assertions (count-based first)
- [ ] Add `pnpm eval:run` script
- [ ] Test locally

### Week 3
- [ ] Implement `quality-judge.ts` assertion
- [ ] Implement `coverage-score.ts` assertion
- [ ] Add CI workflow
- [ ] Document in README

### Week 4
- [ ] A/B comparison workflows
- [ ] Langfuse trace integration for evals
- [ ] Cost tracking
- [ ] Dashboard/reporting

---

## Open Questions (Resolved)

| Question | Decision |
|----------|----------|
| Judge model? | gpt-4o-mini (cheap, fast, sufficient quality) |
| Coverage threshold? | 80% (tune based on results) |
| Store results where? | Git-ignored `.promptfoo/output/` + Langfuse traces |
| Genesis Lab fate? | Keep for now, deprecate after Langfuse UI adoption |

---

## Summary

This design uses Promptfoo as the evaluation engine with custom LLM-as-judge assertions. The architecture is:

1. **Simple**: YAML configs + 2 custom assertions
2. **Deep**: Promptfoo hides evaluation complexity behind `pnpm eval:run`
3. **Explicit**: All test cases and assertions visible in `evals/` directory
4. **Robust**: CI integration, multiple providers, graceful fallbacks

Total new code: ~300 lines (assertions + CLI wrapper)
Total configuration: ~100 lines YAML
Reused infrastructure: Promptfoo, Langfuse, existing providers
