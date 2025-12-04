# DESIGN.md - Gemini 3 Pro Migration

## Architecture Overview

**Selected Approach**: Atomic Provider Removal with Minimal Abstraction

**Rationale**: Single atomic commit removes ~1000 lines of OpenAI code while keeping a thin 30-line abstraction for centralized error handling and logging. No intermediate states, no feature flags, no gradual rollout—just clean removal.

**Core Modules**:
- `aiProviders.ts`: Single-function Google provider initialization with diagnostics
- `aiGeneration.ts`: Content generation using Vercel AI SDK's `generateObject` with `providerOptions`
- `iqc.ts`: Merge adjudication using direct `generateObject`
- `lab.ts`: Genesis Lab execution using direct `generateObject`

**Data Flow**: User → Prompt → `initializeGoogleProvider()` → `generateObject()` with `providerOptions.google.thinkingConfig: { thinkingBudget: 8192, includeThoughts: true }` → Zod validation → Response

**Key Design Decisions**:
1. **Hardcode `thinkingConfig: { thinkingBudget: 8192, includeThoughts: true }`**: Educational content always needs maximum reasoning. YAGNI on env var.
2. **Keep minimal abstraction**: 30 lines for centralized error handling beats shotgun surgery later.
3. **Remove provider branching entirely**: No dead code paths, no type union complexity.

---

## Module: aiProviders.ts (Simplified)

**Responsibility**: Centralized Google AI provider initialization with diagnostics and error handling.

**Before**: 151 lines, dual-provider with branching
**After**: ~35 lines, Google-only

**Public Interface**:
```typescript
export interface ProviderClient {
  model: LanguageModel;
  diagnostics: SecretDiagnostics;
}

export interface InitializeProviderOptions {
  logger?: ProviderLogger;
  logContext?: Record<string, unknown>;
  deployment?: string;
}

export function initializeGoogleProvider(
  modelName: string,
  options?: InitializeProviderOptions
): ProviderClient
```

**Internal Implementation**:
- Read `GOOGLE_AI_API_KEY` from environment
- Generate secret diagnostics (fingerprint, presence check)
- Create Google Generative AI instance via `@ai-sdk/google`
- Log initialization with context
- Throw descriptive error if API key missing

**Pseudocode**:
```pseudocode
function initializeGoogleProvider(modelName, options):
  1. Read GOOGLE_AI_API_KEY from env
  2. Generate diagnostics = getSecretDiagnostics(apiKey)
  3. Build log fields with context, model, diagnostics

  4. Log "Using Google AI provider"

  5. If apiKey is empty:
     - Log error "GOOGLE_AI_API_KEY not configured"
     - Throw Error with same message

  6. Create google = createGoogleGenerativeAI({ apiKey })
  7. Create model = google(modelName) as LanguageModel

  8. Return { model, diagnostics }
```

**Removed**:
- `initializeOpenAIProvider()` function (~25 lines)
- `normalizeProvider()` function (~12 lines)
- `provider` field in return type
- `openaiClient` field in return type
- OpenAI import and type references

---

## Module: aiGeneration.ts (Core Generation)

**Responsibility**: Process generation jobs and phrasing generation with Gemini 3 Pro.

**Before**: 985 lines with 8 provider branching conditionals
**After**: ~850 lines, direct `generateObject` calls with `providerOptions`

**Critical Change Pattern**:

Every `generateObject` call transforms from:
```typescript
// BEFORE (provider branching)
if (provider === 'openai' && openaiClient) {
  response = await generateObjectWithResponsesApi({
    client: openaiClient,
    model: modelName,
    input: prompt,
    schema: intentSchema,
    schemaName: 'intent',
    verbosity,
    reasoningEffort,
  });
} else if (provider === 'google' && model) {
  response = await generateObject({
    model,
    schema: intentSchema,
    prompt,
  });
} else {
  throw new Error('Provider not initialized correctly');
}
```

To:
```typescript
// AFTER (direct with providerOptions)
const response = await generateObject({
  model,
  schema: intentSchema,
  prompt,
  providerOptions: {
    google: {
      thinkingConfig: { thinkingBudget: 8192, includeThoughts: true },
    },
  },
});
```

**Locations requiring this transformation**:
1. `processJob()` - Intent extraction (line ~414)
2. `processJob()` - Concept synthesis (line ~470)
3. `generatePhrasingsForConcept()` - Phrasing generation (line ~804)

**Variable Cleanup**:
```typescript
// BEFORE
const requestedProvider = process.env.AI_PROVIDER || 'openai';
const modelName = process.env.AI_MODEL || 'gpt-5.1';
const reasoningEffort = process.env.AI_REASONING_EFFORT || 'high';
const verbosity = process.env.AI_VERBOSITY || 'medium';
let openaiClient: ProviderClient['openaiClient'];
let provider: ProviderClient['provider'] = 'openai';

// AFTER
const modelName = process.env.AI_MODEL || 'gemini-3-pro-preview';
```

**Import Cleanup**:
```typescript
// REMOVE
import { generateObjectWithResponsesApi } from './lib/responsesApi';

// KEEP
import { generateObject } from 'ai';
import { initializeGoogleProvider } from './lib/aiProviders';
```

**Pseudocode for processJob transformation**:
```pseudocode
function processJob(jobId):
  1. Initialize timing and metadata

  2. Read modelName from AI_MODEL env (default: 'gemini-3-pro-preview')

  3. Initialize provider:
     providerClient = initializeGoogleProvider(modelName, { logger, logContext })
     model = providerClient.model
     diagnostics = providerClient.diagnostics

  4. Extract intent:
     response = generateObject({
       model,
       schema: intentSchema,
       prompt: intentPrompt,
       providerOptions: { google: { thinkingConfig: { thinkingBudget: 8192, includeThoughts: true } } }
     })

  5. Generate concepts:
     response = generateObject({
       model,
       schema: conceptIdeasSchema,
       prompt: conceptPrompt,
       providerOptions: { google: { thinkingConfig: { thinkingBudget: 8192, includeThoughts: true } } }
     })

  6. Process concepts, schedule phrasing generation

  7. Error handling: classify, log, fail job
```

---

## Module: iqc.ts (Merge Adjudication)

**Responsibility**: IQC duplicate detection and merge decision via LLM.

**Before**: Provider branching in `adjudicateMergeCandidate()` and `scanAndPropose()`
**After**: Direct `generateObject` calls

**Key Function Transformation**:
```typescript
// BEFORE (lines 688-730)
async function adjudicateMergeCandidate({
  candidate, provider, modelName, reasoningEffort, verbosity, model, openaiClient
}): Promise<MergeDecision | null> {
  const prompt = buildMergePrompt(candidate);

  if (provider === 'google' && model) {
    const response = await generateObject({ model, prompt, schema: mergeDecisionSchema });
    return response.object;
  }

  if (provider === 'openai' && openaiClient) {
    const response = await generateObjectWithResponsesApi({...});
    return response.object;
  }

  return null;
}

// AFTER
async function adjudicateMergeCandidate({
  candidate, model
}: {
  candidate: MergeCandidate;
  model: LanguageModel;
}): Promise<MergeDecision> {
  const prompt = buildMergePrompt(candidate);

  const response = await generateObject({
    model,
    prompt,
    schema: mergeDecisionSchema,
    providerOptions: {
      google: {
        thinkingConfig: { thinkingBudget: 8192, includeThoughts: true },
      },
    },
  });

  return response.object;
}
```

**scanAndPropose() Cleanup**:
```typescript
// REMOVE these variables:
const provider = process.env.AI_PROVIDER || 'openai';
const reasoningEffort = process.env.AI_REASONING_EFFORT || 'medium';
const verbosity = process.env.AI_VERBOSITY || 'low';
let openaiClient: OpenAI | undefined;

// REMOVE these conditionals:
if (provider === 'google') { ... } else { ... }

// REPLACE with:
const modelName = process.env.AI_MODEL || 'gemini-3-pro-preview';
const { model, diagnostics } = initializeGoogleProvider(modelName, { logger });
```

**Import Cleanup**:
```typescript
// REMOVE
import OpenAI from 'openai';
import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { generateObjectWithResponsesApi } from './lib/responsesApi';

// ADD/KEEP
import { generateObject } from 'ai';
import { initializeGoogleProvider } from './lib/aiProviders';
```

---

## Module: lab.ts (Genesis Lab)

**Responsibility**: Execute infrastructure configs for prompt testing.

**Before**: Provider branching for both text and questions output
**After**: Direct Vercel AI SDK calls

**executeConfig() Transformation**:
```typescript
// BEFORE (lines 212-235)
if (provider === 'openai' && openaiClient) {
  response = await generateObjectWithResponsesApi({
    client: openaiClient,
    model: args.model,
    input: prompt,
    schema: questionsSchema,
    ...
  });
} else if (provider === 'google' && model) {
  response = await generateObject({
    model,
    schema: questionsSchema,
    prompt,
    ...
  });
}

// AFTER
const response = await generateObject({
  model,
  schema: questionsSchema,
  prompt,
  ...(args.temperature !== undefined && { temperature: args.temperature }),
  ...(args.maxTokens !== undefined && { maxTokens: args.maxTokens }),
  ...(args.topP !== undefined && { topP: args.topP }),
  providerOptions: {
    google: {
      thinkingConfig: { thinkingBudget: 8192, includeThoughts: true },
    },
  },
});
```

**Args Schema Simplification**:
```typescript
// REMOVE these args:
reasoningEffort: v.optional(...),
verbosity: v.optional(...),
maxCompletionTokens: v.optional(...),

// KEEP:
provider: v.string(),  // Still needed for config storage, but always 'google'
model: v.string(),
temperature: v.optional(v.number()),
maxTokens: v.optional(v.number()),
topP: v.optional(v.number()),
```

---

## Module: evals/runner.ts

**Responsibility**: Run evaluation cases against AI generation.

**Before**: Provider branching
**After**: Direct `generateObject`

**Transformation**:
```typescript
// BEFORE
const providerName = process.env.AI_PROVIDER || 'openai';
const { provider, model, openaiClient } = providerClient;

if (provider === 'openai' && openaiClient) {
  response = await generateObjectWithResponsesApi({...});
} else if (provider === 'google' && model) {
  response = await generateObject({...});
}

// AFTER
const modelName = process.env.AI_MODEL || 'gemini-3-pro-preview';
const { model } = initializeGoogleProvider(modelName, { logContext: { source: 'evals' } });

const response = await generateObject({
  model,
  schema: conceptIdeasSchema,
  prompt,
  providerOptions: {
    google: {
      thinkingConfig: { thinkingBudget: 8192, includeThoughts: true },
    },
  },
});
```

---

## Module: types/lab.ts (Type Definitions)

**Responsibility**: TypeScript types for Genesis Lab.

**Before**: Dual-provider union type
**After**: Google-only type

**Type Transformation**:
```typescript
// REMOVE
export type AIProvider = 'google' | 'openai';

export interface OpenAIInfraConfig extends BaseInfraConfig {
  provider: 'openai';
  model: string;
  reasoningEffort?: 'minimal' | 'low' | 'medium' | 'high';
  verbosity?: 'low' | 'medium' | 'high';
  maxCompletionTokens?: number;
  temperature?: number;
}

export type InfraConfig = GoogleInfraConfig | OpenAIInfraConfig;

// AFTER
export type InfraConfig = GoogleInfraConfig;
// AIProvider type removed entirely (or simplified to just 'google' literal)
```

**Validation Function Simplification**:
```typescript
// BEFORE
export function isValidConfig(config: InfraConfig): boolean {
  const baseValid = ...;
  if (!baseValid) return false;

  if (config.provider === 'google') {
    return (/* google validation */);
  } else if (config.provider === 'openai') {
    return (/* openai validation */);
  }
  return false;
}

// AFTER
export function isValidConfig(config: InfraConfig): boolean {
  const baseValid = ...;
  if (!baseValid) return false;

  return (
    (config.temperature === undefined || (config.temperature >= 0 && config.temperature <= 2)) &&
    (config.maxTokens === undefined || (config.maxTokens >= 1 && config.maxTokens <= 65536)) &&
    (config.topP === undefined || (config.topP >= 0 && config.topP <= 1))
  );
}
```

---

## Frontend: config-editor.tsx

**Responsibility**: Form for creating/editing lab configs.

**Before**: Provider select dropdown with google/openai options
**After**: No provider select (hardcoded google)

**Changes**:
1. **Remove provider select** (lines 208-223):
```tsx
// REMOVE this entire block
<div>
  <Label htmlFor="config-provider">Provider *</Label>
  <Select
    value={provider}
    onValueChange={(value) => setProvider(value as AIProvider)}
    disabled={config?.isProd}
  >
    <SelectTrigger id="config-provider" className="w-full">
      <SelectValue />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value="google">Google</SelectItem>
      <SelectItem value="openai">OpenAI</SelectItem>
    </SelectContent>
  </Select>
</div>
```

2. **Simplify state**:
```typescript
// REMOVE
const [provider, setProvider] = useState<AIProvider>(config?.provider || 'google');

// Config is always Google now, no provider state needed
```

3. **Simplify handleSave**:
```typescript
// BEFORE
if (provider === 'google') {
  const googleConfig: GoogleInfraConfig = {...};
  newConfig = googleConfig;
} else {
  const openaiConfig: OpenAIInfraConfig = {...};
  newConfig = openaiConfig;
}

// AFTER
const newConfig: InfraConfig = {
  ...baseFields,
  provider: 'google',
  temperature: tempNum,
  maxTokens: tokensNum,
  topP: topPNum,
};
```

---

## File: responsesApi.ts (DELETE)

**Action**: Delete entire file (149 lines)

**Reason**: OpenAI Responses API helper is no longer needed. All generation uses Vercel AI SDK's `generateObject` directly.

---

## Test Updates

### aiProviders.test.ts

**Before**: Tests for both Google and OpenAI initialization
**After**: Tests for Google-only initialization

**Remove**:
- `it('initializes OpenAI provider and returns client with diagnostics')`
- `it('throws when OPENAI_API_KEY is missing and logs error diagnostics')`
- OpenAI mock setup

**Update**:
- `it('initializes Google provider...')` - function name change to `initializeGoogleProvider`
- Remove `openaiClient` assertions
- Remove `provider` field assertions

**Simplify mock setup**:
```typescript
// REMOVE
vi.mock('openai', { spy: true });
const mockOpenAIConstructor = vi.mocked(OpenAI);
```

### aiGeneration.process.test.ts

**Update mocks**:
```typescript
// BEFORE
vi.mock('./lib/responsesApi', () => ({
  generateObjectWithResponsesApi: vi.fn(),
}));

// AFTER (remove entirely, just mock generateObject)
vi.mock('ai', () => ({
  generateObject: vi.fn(),
}));
```

### iqc.test.ts

**Remove OpenAI test cases** if any exist for adjudication paths.

---

## Environment Variable Changes

**Production Deployment**:
```bash
# Set new model
npx convex env set AI_MODEL "gemini-3-pro-preview" --prod

# Remove obsolete vars
npx convex env delete AI_PROVIDER --prod
npx convex env delete AI_REASONING_EFFORT --prod
npx convex env delete AI_VERBOSITY --prod
npx convex env delete OPENAI_API_KEY --prod
```

**Development (.env.local)**:
```bash
# Keep
GOOGLE_AI_API_KEY=your-key

# Remove
OPENAI_API_KEY=
AI_PROVIDER=
AI_REASONING_EFFORT=
AI_VERBOSITY=
```

---

## Package.json Dependency Removal

**Remove**:
```json
"openai": "^6.9.1"
```

**Keep** (still used):
```json
"@ai-sdk/google": "^2.0.40",
"@ai-sdk/openai": "^2.0.71",  // May be used elsewhere? Check.
"ai": "^5.0.98"
```

**Check**: Verify `@ai-sdk/openai` isn't used anywhere else before removing.

---

## Implementation Sequence

**Single atomic commit with this order**:

1. **Delete** `convex/lib/responsesApi.ts`
2. **Modify** `convex/lib/aiProviders.ts` - simplify to Google-only
3. **Modify** `convex/aiGeneration.ts` - remove branching, add providerOptions
4. **Modify** `convex/iqc.ts` - remove branching, add providerOptions
5. **Modify** `convex/lab.ts` - remove branching, add providerOptions
6. **Modify** `convex/evals/runner.ts` - remove branching
7. **Modify** `types/lab.ts` - remove OpenAIInfraConfig
8. **Modify** `components/lab/config-editor.tsx` - remove provider select
9. **Update tests** - remove OpenAI test cases
10. **Update** `package.json` - remove `openai` dependency
11. **Update** `.env.example` - remove OpenAI vars
12. **Update** `CLAUDE.md` - update AI Provider section

---

## Error Handling Strategy

**Error Categories** (unchanged):
1. `SCHEMA_VALIDATION` - AI generated invalid format → retryable
2. `RATE_LIMIT` - 429 errors → retryable with backoff
3. `API_KEY` - Missing/invalid key → not retryable
4. `NETWORK` - Timeout/connection → retryable

**New Monitoring**:
```typescript
// Track schema validation failures for rollback decision
if (code === 'SCHEMA_VALIDATION') {
  trackEvent('Schema Validation Failure', {
    model: 'gemini-3-pro-preview',
    schema: schemaName,
  });
}
```

**Sentry Alert**: Configure for >5 schema failures in 5 minutes.

---

## Testing Strategy

**Unit Tests**:
- `initializeGoogleProvider()` with mocked `@ai-sdk/google`
- Error handling for missing API key
- Diagnostics generation

**Contract Tests**:
- All 3 schemas (`intentSchema`, `conceptIdeasSchema`, `phrasingBatchSchema`) with Gemini 3 Pro
- Run `pnpm test:contract` before deployment

**Integration Tests**:
- Full generation flow in dev environment
- Genesis Lab execution with Gemini config

**Pre-Migration Validation**:
```bash
# 1. Set env vars in dev
npx convex env set AI_MODEL "gemini-3-pro-preview" --dev

# 2. Run contract tests
pnpm test:contract

# 3. Manual quality review (10-20 samples)
```

---

## Rollback Plan

**If issues post-deployment**:
1. Re-add OpenAI code from git history
2. Set `AI_PROVIDER=openai` and `OPENAI_API_KEY`
3. Deploy

**Provider abstraction makes this straightforward** - reason for keeping ~30 lines of abstraction.

---

## Estimated Lines Changed

| File | Before | After | Delta |
|------|--------|-------|-------|
| responsesApi.ts | 149 | 0 | -149 |
| aiProviders.ts | 151 | ~35 | -116 |
| aiGeneration.ts | 985 | ~850 | -135 |
| iqc.ts | 865 | ~800 | -65 |
| lab.ts | 371 | ~320 | -51 |
| evals/runner.ts | 103 | ~80 | -23 |
| types/lab.ts | 191 | ~120 | -71 |
| config-editor.tsx | 430 | ~380 | -50 |
| aiProviders.test.ts | 144 | ~70 | -74 |
| **Total** | | | **~-734** |

Plus OpenAI-related lines in other test files and configs = **~1000 lines removed**.

---

## Success Criteria

- [ ] `rg -c openai convex/` returns 0
- [ ] `rg -c "provider ===" convex/` returns 0
- [ ] All tests pass (`pnpm test`)
- [ ] Coverage maintained at 70%
- [ ] Contract tests pass with Gemini 3 Pro
- [ ] `openai` package removed from package.json
