# TODO: LLM Evaluation Framework

## Context
- Architecture: DESIGN.md (Promptfoo-Centric) - SIMPLIFIED per Grug review
- Key Files: `evals/promptfoo.yaml`, `evals/prompts/concept-synthesis.txt`
- Patterns: Use Promptfoo built-in assertions first; custom only when proven necessary
- Grug Wisdom: Run ONE eval first. Learn. Then add complexity.

## Phase 1: Make One Thing Work ✅

- [x] Install Promptfoo and create minimal config
  ```
  Files: package.json (devDep), evals/promptfoo.yaml, evals/prompts/concept-synthesis.txt

  Work Log:
  - Installed promptfoo 0.120.1
  - Required pnpm rebuild better-sqlite3 for native module
  - Added pnpm.onlyBuiltDependencies config
  - Created minimal YAML config with 3 inline test cases
  - Added transform to strip markdown code blocks from output
  - All 3 tests pass (100% pass rate)

  Learning: Model wrapped JSON in markdown code blocks - needed transform
  ```

- [x] Migrate existing test cases to Promptfoo YAML format
  ```
  Merged with above - test cases are inline in promptfoo.yaml for simplicity
  ```

- [x] Add minimal package.json script for convenience
  ```
  Added: "eval", "eval:view" scripts
  ```

- [x] Run first evaluation and document findings
  ```
  First eval: 3/3 tests pass (100%)
  Duration: 36s, ~9700 tokens

  KEY LEARNINGS:

  1. MARKDOWN WRAPPING: Gemini wraps JSON in ```json blocks
     - Fix: Added defaultTest.options.transform to strip blocks
     - Applies to all test cases automatically

  2. FALSE POSITIVE DETECTED: Asimov test passed but generated WRONG content!
     - Input: "First Law, Second Law, Third Law" (Asimov's Robotics)
     - Output: "First Law of Thermodynamics" (Physics!)
     - The simple "count >= 3" assertion passed despite wrong domain
     - LEARNING: Need semantic checks, not just count checks

  3. TRANSFORM WORKED: JSON parsing now reliable after stripping blocks

  4. TOKEN COST: ~3200 tokens/test case (affordable for iteration)

  NEXT ACTIONS (Phase 2 candidates):
  - Add domain-specific assertions ("contains: Asimov" or "contains: robot")
  - Consider llm-rubric for semantic correctness
  - The Asimov false positive proves Grug right: start simple, find real gaps
  ```

## Phase 2: Add What's Actually Needed (after 20+ manual runs)

These tasks are DEFERRED until Phase 1 learnings prove need:

- [x] Fix Asimov false positive with domain-specific assertion
  ```
  Files: evals/promptfoo.yaml
  Trigger: PROVEN - first eval showed wrong domain (thermodynamics vs robotics)
  Approach: Add "contains: robot" or "contains: Asimov" assertion

  Work Log:
  - Root cause: vague atomic_units ("First Law") without domain context
  - Fix 1: Made atomic_units explicit ("First Law of Robotics")
  - Fix 2: Added synthesis_op "Asimov science fiction" for context
  - Fix 3: Added `icontains: robot` assertion as domain check
  - Result: 3/3 pass, output now correctly about robotics not thermodynamics

  Learning: Input clarity > output assertions. Better prompts beat more tests.
  ```

- [ ] Add LLM-as-judge quality assertion (IF string matching proves insufficient)
  ```
  Files: evals/assertions/quality-judge.ts
  Trigger: When >20% of evals give false positives/negatives with simple assertions
  Approach: Start with pass/fail (no weighted scoring), use cheap model (gemini-flash)
  Deferred: Weighted scoring (40/35/25) - add only if single score proves insufficient
  ```

- [ ] Add coverage assertion (IF contains-based checks prove insufficient)
  ```
  Files: evals/assertions/coverage-score.ts
  Trigger: When topic coverage clearly needs semantic matching (not string)
  Approach: Simple keyword extraction first, LLM semantic match only if needed
  ```

## Phase 3: Scale What Works (after 100+ runs)

- [ ] CI integration (IF manual eval proves valuable and stable)
- [ ] Model comparison workflow
- [ ] Cost tracking

## Not Doing (Grug-approved deletions)

| Original Plan | Grug Verdict | Why |
|--------------|--------------|-----|
| TypeScript CLI wrapper | ❌ Skip | `npx promptfoo` works fine |
| Weighted quality scores | ❌ Defer | No data to tune weights |
| LLM semantic coverage | ❌ Defer | Try string match first |
| CI/CD on day 1 | ❌ Defer | Don't automate unproven tool |
| Multiple datasets | ❌ Defer | Start with 3 cases, add as needed |

## Success Criteria

**Phase 1 complete when:** ✅
- [x] Can run `pnpm eval` successfully
- [x] 3 test cases from existing cases.ts work
- [x] Have documented learnings from first run

**Phase 2 complete when:**
- [ ] Have run 20+ evals manually
- [ ] Added only the complexity that real failures demanded

**Phase 3 complete when:**
- [ ] Eval runs are stable (<5% flake rate)
- [ ] Value proven (eval catches real issues)
- [ ] CI integration makes sense (cost/time acceptable)
