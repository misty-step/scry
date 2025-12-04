# Migrate Content Generation to Gemini 3 Pro (Remove OpenAI)

## Executive Summary

**Problem:** Currently using OpenAI GPT-5 for content generation, need to migrate entirely to Google Gemini 3 Pro Preview.

**Solution:** Single atomic commit removing all OpenAI code paths, simplifying to Google-only with `gemini-3-pro-preview` and `thinkingConfig: { thinkingBudget: 8192, includeThoughts: true }`.

**User Value:** Consolidate on single AI provider, reduce codebase complexity (~1000 lines removed), leverage Gemini 3's superior reasoning for educational content.

**Success Criteria:** All content generation uses Gemini 3 Pro, zero OpenAI code remains, schema validation >99%, automatic rollback on failure.

---

## User Context

**Who uses this:** Content generation pipeline for creating quiz concepts and phrasings.

**Problem solved:** Simplify AI provider infrastructure from dual-provider (OpenAI + Google) to single provider (Google only).

**Measurable benefits:**
- ~1000 lines of code removed (responsesApi.ts, aiProviders simplification, OpenAI conditionals, tests, types)
- One fewer dependency (`openai` package)
- Single API key to manage (GOOGLE_AI_API_KEY)
- Simpler mental model (no provider branching in 5+ files)

---

## Requirements

### Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| F1 | All content generation (concepts, phrasings, intent) uses Gemini 3 Pro | Must |
| F2 | Genesis Lab uses Gemini 3 Pro for all configurations | Must |
| F3 | IQC (Interactive Question Curation) uses Gemini 3 Pro | Must |
| F4 | Embeddings continue using Google text-embedding-004 | Must |
| F5 | Remove OpenAI SDK and all related code | Must |
| F6 | Remove `openai` package from dependencies | Must |
| F7 | Lab UI removes OpenAI as provider option | Must |

### Non-Functional Requirements

| ID | Requirement | Target |
|----|-------------|--------|
| NF1 | Generation quality | Equal or better than GPT-5 (subjective) |
| NF2 | Schema validation reliability | 100% success rate on existing schemas |
| NF3 | Latency | Within 20% of current performance |
| NF4 | Error rate | <1% schema validation failures |

---

## Architecture Decision

### Selected Approach: Single Atomic Migration

One commit removing all OpenAI code. Keep minimal provider abstraction (one function) for centralized error handling and future flexibility.

**Rationale:**
- **Simplicity:** Atomic change = no broken intermediate states
- **Craft:** Keep one-function abstraction (costs 20 lines, provides centralized errors)
- **Robustness:** Automatic rollback on error threshold

### Critical Implementation Detail: providerOptions

**BLOCKER from architecture review:** Must pass `thinkingConfig` to Gemini model:

```typescript
// In generateObject() calls - add providerOptions
const response = await generateObject({
  model,
  schema: intentSchema,
  prompt: intentPrompt,
  providerOptions: {
    google: {
      thinkingConfig: { thinkingBudget: 8192, includeThoughts: true },  // Required for Gemini 3 reasoning
    },
  },
});
```

Without this, Gemini 3 Pro uses default thinking level, defeating the purpose of migration.

### Simplified Module Structure

```
convex/lib/aiProviders.ts (simplified, ~30 lines)
├── initializeGoogleProvider() - Single function
├── Returns: { model, diagnostics }
└── Reads: AI_MODEL env var only

convex/aiGeneration.ts
├── processJob() - Uses generateObject with providerOptions
├── generatePhrasingsForConcept() - Uses generateObject with providerOptions
└── No provider branching

convex/embeddings.ts (unchanged)
└── Already Google-only (text-embedding-004)
```

### Environment Variables (Simplified)

| Var | Value | Notes |
|-----|-------|-------|
| `AI_MODEL` | `gemini-3-pro-preview` | Model selection |
| `GOOGLE_AI_API_KEY` | (secret) | Required |

**Removed:** `AI_PROVIDER`, `AI_REASONING_EFFORT`, `AI_VERBOSITY`, `AI_THINKING_LEVEL`, `OPENAI_API_KEY`

**Why no `AI_THINKING_LEVEL`?** Hardcode `thinkingConfig: { thinkingBudget: 8192, includeThoughts: true }` in code. YAGNI - if tuning needed later, add env var then.

---

## Dependencies & Assumptions

### External Dependencies
- `@ai-sdk/google@^2.0.40` - Already supports Gemini 3 Pro
- `ai@^5.0.98` - Vercel AI SDK core
- Google AI API key with Gemini 3 Pro access

### Assumptions
1. User accepts Gemini 3 Pro Preview limitations (no SLA, possible instability)
2. User accepts higher API costs (Gemini 3 Pro: $2.00/1M input, $12.00/1M output vs GPT-5-mini: $0.25/1M input, $2.00/1M output — approximately 6-8x more expensive)
3. Existing Zod schemas work with Gemini's structured output
4. `thinkingConfig: { thinkingBudget: 8192, includeThoughts: true }` provides equivalent quality to `reasoning_effort: 'high'`

### Integration Requirements
- Convex backend env vars must be updated before deployment
- Genesis Lab configs: Existing Google configs work unchanged. OpenAI configs in localStorage are user's dev experiments—they can delete them manually or we can add a one-time migration that removes invalid configs with console warning.

---

## Implementation: Single Atomic Commit

All changes in one PR. No phases. Tests updated alongside code.

### Files to DELETE

| File | Lines | Reason |
|------|-------|--------|
| `convex/lib/responsesApi.ts` | 149 | OpenAI Responses API helper |

### Files to MODIFY

**Backend (remove OpenAI paths, add providerOptions):**
- `convex/aiGeneration.ts` - Remove provider branching, add `providerOptions.google`
- `convex/lib/aiProviders.ts` - Remove OpenAI initialization (~120 lines removed)
- `convex/lib/productionConfig.ts` - Remove provider field
- `convex/lab.ts` - Remove OpenAI code path, OpenAI params
- `convex/iqc.ts` - Remove OpenAI branch
- `convex/evals/runner.ts` - Remove OpenAI path
- `convex/health.ts` - Audit for OpenAI references

**Types (collapse to Google-only):**
- `types/lab.ts` - Delete `OpenAIInfraConfig`, collapse `InfraConfig` type

**Frontend (remove OpenAI option from UI):**
- `components/lab/config-editor.tsx` - Remove provider select
- `components/lab/config-management-dialog.tsx` - Remove provider select
- `components/lab/config-manager.tsx` - Remove OpenAI rendering
- `app/lab/configs/_components/config-manager-page.tsx` - Remove provider select

**Tests (update mocks, remove OpenAI tests):**
- `convex/lib/aiProviders.test.ts` - Simplify to Google-only
- `tests/convex/aiGeneration.process.test.ts` - Update mocks
- `convex/iqc.test.ts` - Remove OpenAI cases
- `types/lab.test.ts` - Update type tests

**Config/Docs:**
- `package.json` - Remove `openai` dependency
- `.env.example` - Remove OpenAI vars
- `CLAUDE.md` - Update AI Provider section

### Environment Variable Changes

```bash
# Set new model
npx convex env set AI_MODEL "gemini-3-pro-preview" --prod

# Remove obsolete vars
npx convex env delete AI_PROVIDER --prod
npx convex env delete AI_REASONING_EFFORT --prod
npx convex env delete AI_VERBOSITY --prod
npx convex env delete OPENAI_API_KEY --prod
```

### Automatic Rollback (Jobs recommendation)

Add error threshold monitoring. If schema validation failures exceed 5% in 5 minutes, alert and prepare rollback:

```typescript
// In aiGeneration.ts error handler
if (code === 'SCHEMA_VALIDATION') {
  trackEvent('Schema Validation Failure', {
    model: 'gemini-3-pro-preview',
    schema: schemaName,
  });
}
// Sentry alert configured for: >5 schema failures in 5 minutes
```

**Manual rollback if needed:** Re-add OpenAI code from git history. Provider abstraction makes this straightforward.

---

## Risks & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Preview model instability | Medium | High | Monitor Sentry errors, have manual rollback ready |
| Schema validation failures | Low | High | Test all 3 schemas (`intentSchema`, `conceptIdeasSchema`, `phrasingBatchSchema`) in dev first |
| Quality regression | Low | Medium | Compare sample outputs before/after |
| Cost spike | High | Medium | Accepted by user; add cost tracking |
| API rate limits | Medium | Medium | Existing retry logic should handle |

---

## Key Decisions

### 1. Keep Minimal Abstraction (Jobs recommendation)

**Decision:** Keep `aiProviders.ts` as single-function abstraction (~30 lines), remove only OpenAI implementation.

**Alternatives:** Delete entirely and inline (Simplicity reviewer), Keep full dual-provider

**Rationale:** One-function abstraction costs almost nothing but provides centralized error handling, logging, and future provider flexibility. Re-creating the abstraction later is far more expensive than maintaining it.

**Tradeoffs:** 30 extra lines vs shotgun surgery if provider changes needed.

### 2. Hardcode thinkingConfig (YAGNI)

**Decision:** Hardcode `thinkingConfig: { thinkingBudget: 8192, includeThoughts: true }` in code, no env var.

**Alternatives:** `AI_THINKING_LEVEL` env var for tuning

**Rationale:** Educational content generation always needs maximum reasoning. If tuning needed later, add env var then. Fewer configuration surfaces = fewer failure modes.

### 3. Atomic Commit (All reviewers)

**Decision:** All changes in single PR/commit. No phases.

**Alternatives:** 4-phase rollout, Feature flag gradual migration

**Rationale:** Phases add coordination overhead with zero safety benefit for a full removal. Atomic change is tested together, avoiding broken intermediate states.

### 4. providerOptions Required (Architecture blocker)

**Decision:** All `generateObject` calls must include `providerOptions.google.thinkingConfig`.

**Rationale:** Without this, Gemini 3 Pro uses default thinking level. This is the **entire reason** for migrating to Gemini 3 Pro. Missing this defeats the purpose.

---

## Test Scenarios

### Happy Path
- [ ] Generate concepts with `gemini-3-pro-preview` and `thinkingConfig: { thinkingBudget: 8192 }`
- [ ] Generate phrasings for concept batch
- [ ] Lab experiment with Gemini 3 config
- [ ] IQC merge operation with Gemini

### Error Conditions
- [ ] Missing GOOGLE_AI_API_KEY returns clear error
- [ ] Rate limit (429) triggers retry with backoff
- [ ] Schema validation failure returns user-friendly message
- [ ] Network timeout retries correctly

### Edge Cases
- [ ] Very long prompts (near context limit)
- [ ] Unicode/special characters in content
- [ ] Concurrent generation jobs don't conflict
- [ ] Lab configs saved pre-migration still work (Google configs)

### Regression Tests
- [ ] All existing tests pass with OpenAI mocks removed
- [ ] Contract tests validate API response shapes
- [ ] Coverage threshold maintained (70%)

---

## Pre-Migration Validation (Jobs recommendation)

Before deploying, validate schema compatibility in dev environment:

```bash
# Test all 3 schemas with Gemini 3 Pro
npx convex dev  # In separate terminal

# Run contract tests against Gemini
pnpm test:contract

# Generate sample concepts and compare quality
# (Manual quality review of 10-20 samples)
```

**Go/No-Go Criteria:**
- Schema validation success rate > 99%
- Latency within 20% of baseline
- Sample output quality subjectively acceptable

---

## Success Metrics

| Metric | Current | Target | Measurement |
|--------|---------|--------|-------------|
| Lines of OpenAI code | ~1000 | 0 | `rg -c openai` |
| Provider conditionals | ~15 | 0 | grep for `provider ===` |
| Dependencies | openai + google | google only | package.json |
| Schema validation | 100% | >99% | Sentry tracking |
| Test pass rate | 100% | 100% | CI |
| Coverage | 70% | 70% | Vitest |
