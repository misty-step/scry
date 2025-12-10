# TODO: LLM Operations Infrastructure

## Overview

Infrastructure for observability, iterability, and experimentation in the AI generation pipeline.

**Key Files:**
- `convex/lib/langfuse.ts` - Observability singleton
- `convex/lib/scoring.ts` - LLM-as-judge evaluation
- `convex/lib/prompts.ts` - Langfuse prompt fetching (wired to aiGeneration)
- `convex/lib/aiProviders.ts` - OpenRouter provider (supports all models)
- `convex/aiGeneration.ts` - Generation pipeline
- `~/.claude/skills/langfuse-observability/` - Claude Code skill

---

## Phase 1: Observability Foundation ✅

- [x] Langfuse integration - traces, spans, generations
- [x] LLM-as-judge scoring (5 quality metrics per phrasing batch)
- [x] Claude Code skill for querying traces
- [x] Error logging to traces before flush
- [x] Staggered Stage B scheduling (2s delay)

---

## Phase 2: Prompt Iteration (~2 hours)

### 2a. Langfuse Prompt Management

- [ ] Create prompts in Langfuse dashboard:
  - `scry-intent-extraction` with `{{userInput}}`
  - `scry-concept-synthesis` with `{{intentJson}}`
  - `scry-phrasing-generation` with `{{conceptTitle}}`, `{{contentType}}`, etc.

- [x] Wire `prompts.ts` to `aiGeneration.ts` (3 call sites: ~409, ~476, ~861)
  ```typescript
  // Before
  const prompt = buildIntentExtractionPrompt(job.prompt);
  // After
  const result = await getPrompt('scry-intent-extraction', { userInput: job.prompt });
  ```

- [x] Add `promptVersion` to trace metadata (included in span metadata)

### 2b. Analytics Dimensions (quick win)

- [x] Add to trace metadata:
  - `contentType` - verbatim/enumerable/conceptual (Stage B)
  - `inputLength` - User input character count (Stage A)

---

## Phase 3: Multi-Provider ✅

### OpenRouter Activation

- [x] Simplified to single OpenRouter provider (supports all models including Google)
- [x] Migrated all 5 call sites to `initializeProvider(modelId)`:
  - `aiGeneration.ts` (2 sites)
  - `lab.ts` (1 site)
  - `iqc.ts` (1 site)
  - `evals/runner.ts` (1 site)

### Remaining (Operational)

- [ ] Add `OPENROUTER_API_KEY` to Convex dashboard (dev + prod)

---

## Phase 4: User Feedback Loop (~4 hours)

- [ ] Extend `interactions` schema:
  ```typescript
  feedback?: {
    type: 'helpful' | 'unhelpful' | 'unclear' | 'incorrect'
    givenAt: number
  }
  ```

- [ ] Add `recordFeedback` mutation to `concepts.ts`

- [ ] Add feedback UI to `ReviewFlow`:
  - Thumbs up/down after answer reveal
  - Wire to Langfuse trace via correlationId

---

## Phase 5: Evaluation Framework

Context: Promptfoo for offline CI testing. Grug-approved minimal approach.

- [x] Install Promptfoo, create minimal config (3 test cases)
- [x] Fix Asimov false positive with domain-specific assertion
- [ ] Add LLM-as-judge assertion (IF string matching proves insufficient)
- [ ] CI integration (IF manual eval proves valuable after 100+ runs)

---

## Not Doing (Grug-approved)

| Idea | Verdict | Why |
|------|---------|-----|
| TypeScript CLI wrapper for Promptfoo | Skip | `npx promptfoo` works |
| Weighted quality scores in evals | Defer | No data to tune weights |
| Multiple Promptfoo datasets on day 1 | Defer | Start with 3, add as needed |
| Real-time streaming UI | Skip | Batch generation is fine |

---

## Commands

```bash
# Query traces
cd ~/.claude/skills/langfuse-observability
npx tsx scripts/fetch-traces.ts --limit 10
npx tsx scripts/fetch-trace.ts <trace-id>

# Run evals
pnpm eval
pnpm eval:view

# Deploy
npx convex dev --once
```
