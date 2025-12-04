# TODO: Gemini 3 Pro Migration

## Context
- Architecture: Atomic Provider Removal (see DESIGN.md)
- Pattern: Replace `provider === 'openai'` branching with direct `generateObject()` + `providerOptions`
- Critical: Every `generateObject` call needs `providerOptions: { google: { thinkingConfig: { thinkingBudget: 8192, includeThoughts: true } } }`

## Implementation Tasks

These tasks can largely be done in parallel. The only hard dependency is Task 1 (delete responsesApi.ts) must complete before Tasks 2-5 since they import from it.

- [x] Delete `convex/lib/responsesApi.ts` entirely
  ```
  Files: convex/lib/responsesApi.ts (DELETE - 149 lines)
  Approach: `rm convex/lib/responsesApi.ts`
  Success: File no longer exists, no import errors after other tasks complete
  Test: N/A (deletion)
  Dependencies: None (do first)
  Time: 1min
  ```

- [x] Simplify `aiProviders.ts` to Google-only
  ```
  Files: convex/lib/aiProviders.ts
  Approach:
    - Remove OpenAI import: `import OpenAI from 'openai'`
    - Remove `initializeOpenAIProvider()` function (~lines 83-106)
    - Remove `normalizeProvider()` function (~lines 139-150)
    - Rename `initializeProvider` → `initializeGoogleProvider`
    - Simplify interface: remove `openaiClient`, remove `provider` field
    - Keep: diagnostics, logging, error handling structure
  Pseudocode: DESIGN.md "Module: aiProviders.ts (Simplified)"
  Success:
    - Function exports `initializeGoogleProvider(modelName, options)` only
    - Returns `{ model, diagnostics }` (no `provider`, no `openaiClient`)
    - ~35 lines total
  Test: `pnpm test convex/lib/aiProviders.test.ts` after test updates
  Dependencies: None
  Time: 15min
  ```

- [x] Migrate `aiGeneration.ts` to direct generateObject
  ```
  Files: convex/aiGeneration.ts
  Approach:
    - Remove import: `import { generateObjectWithResponsesApi } from './lib/responsesApi'`
    - Change import: `initializeProvider` → `initializeGoogleProvider`
    - Remove env vars: `AI_PROVIDER`, `AI_REASONING_EFFORT`, `AI_VERBOSITY`
    - Change default: `AI_MODEL` default from 'gpt-5.1' to 'gemini-3-pro-preview'
    - In `processJob()`:
      * Remove `requestedProvider`, `reasoningEffort`, `verbosity` variables
      * Remove `openaiClient`, `provider` tracking
      * Call `initializeGoogleProvider(modelName, ...)` instead of `initializeProvider(...)`
      * Replace intent extraction branching (~line 403-421) with direct call
      * Replace concept synthesis branching (~line 459-477) with direct call
      * Add `providerOptions: { google: { thinkingConfig: { thinkingBudget: 8192, includeThoughts: true } } }` to each
    - In `generatePhrasingsForConcept()`:
      * Same pattern: remove branching, add providerOptions
      * Replace phrasing generation branching (~line 793-811)
  Pseudocode: DESIGN.md "Module: aiGeneration.ts (Core Generation)"
  Success:
    - Zero `provider === 'openai'` conditionals
    - All 3 generateObject calls have `providerOptions.google.thinkingConfig: { thinkingBudget: 8192, includeThoughts: true }`
    - No import from responsesApi
  Test: `pnpm test tests/convex/aiGeneration.process.test.ts`
  Dependencies: Task 1 (responsesApi deleted)
  Time: 30min
  ```

- [x] Migrate `iqc.ts` to direct generateObject
  ```
  Files: convex/iqc.ts
  Approach:
    - Remove imports: `OpenAI from 'openai'`, `createGoogleGenerativeAI`, `generateObjectWithResponsesApi`
    - Add import: `initializeGoogleProvider from './lib/aiProviders'`
    - In `scanAndPropose()` (~line 143-165):
      * Remove `provider`, `reasoningEffort`, `verbosity` variables
      * Remove OpenAI/Google branching for client init
      * Call `initializeGoogleProvider(modelName, { logger })`
    - Simplify `adjudicateMergeCandidate()` (~line 688-730):
      * Remove all parameters except `candidate` and `model`
      * Remove provider branching
      * Single `generateObject()` call with `providerOptions.google.thinkingConfig: { thinkingBudget: 8192, includeThoughts: true }`
  Pseudocode: DESIGN.md "Module: iqc.ts (Merge Adjudication)"
  Success:
    - No `from 'openai'` import
    - `adjudicateMergeCandidate()` takes only `{ candidate, model }`
    - Returns `MergeDecision` (not `MergeDecision | null`)
  Test: `pnpm test convex/iqc.test.ts`
  Dependencies: Task 1 (responsesApi deleted)
  Time: 20min
  ```

- [x] Migrate `lab.ts` to direct generateObject
  ```
  Files: convex/lab.ts
  Approach:
    - Remove import: `generateObjectWithResponsesApi from './lib/responsesApi'`
    - Change import: `initializeProvider` → `initializeGoogleProvider`
    - Remove args from validator (~line 87-91):
      * `reasoningEffort` arg
      * `verbosity` arg
      * `maxCompletionTokens` arg
    - In `executeConfig()` handler:
      * Change `initializeProvider(args.provider, ...)` → `initializeGoogleProvider(args.model, ...)`
      * Remove `openaiClient` destructure
      * Replace questions output branching (~line 212-235):
        - Remove OpenAI path
        - Keep Google path, add `providerOptions.google.thinkingConfig: { thinkingBudget: 8192, includeThoughts: true }`
      * Remove reasoning token extraction (OpenAI-specific, ~line 250-265)
  Pseudocode: DESIGN.md "Module: lab.ts (Genesis Lab)"
  Success:
    - No `generateObjectWithResponsesApi` import
    - No `reasoningEffort`/`verbosity` in args schema
    - All generateObject calls have providerOptions
  Test: Manual Lab UI test (dev-only feature)
  Dependencies: Task 1 (responsesApi deleted)
  Time: 20min
  ```

- [x] Migrate `evals/runner.ts` to direct generateObject
  ```
  Files: convex/evals/runner.ts
  Approach:
    - Remove import: `generateObjectWithResponsesApi from '../lib/responsesApi'`
    - Change import: `initializeProvider` → `initializeGoogleProvider`
    - Remove `providerName` variable (was defaulting to 'openai')
    - Change `AI_MODEL` default from 'gpt-5.1' to 'gemini-3-pro-preview'
    - Change provider init: `initializeGoogleProvider(modelName, { logContext })`
    - Replace branching (~line 40-60) with single `generateObject()` + providerOptions
  Pseudocode: DESIGN.md "Module: evals/runner.ts"
  Success:
    - No provider branching
    - Direct generateObject call with providerOptions
  Test: `npx convex run evals/runner:run` (manual)
  Dependencies: Task 1 (responsesApi deleted)
  Time: 10min
  ```

- [x] Simplify `types/lab.ts` to Google-only
  ```
  Files: types/lab.ts
  Approach:
    - Remove: `AIProvider` type (line 12)
    - Remove: `OpenAIInfraConfig` interface (~lines 63-71)
    - Simplify: `InfraConfig = GoogleInfraConfig` (was union type, line 75)
    - Simplify: `isValidConfig()` - remove OpenAI validation branch (~lines 140-152)
  Pseudocode: DESIGN.md "Module: types/lab.ts (Type Definitions)"
  Success:
    - `InfraConfig` is just `GoogleInfraConfig`
    - No `OpenAIInfraConfig` export
    - No `AIProvider` type export
  Test: `pnpm tsc --noEmit` (type check)
  Dependencies: None (types only)
  Time: 10min
  ```

- [x] Update `config-editor.tsx` - remove provider select
  ```
  Files: components/lab/config-editor.tsx
  Approach:
    - Remove `AIProvider` import from types/lab
    - Remove state: `const [provider, setProvider] = useState<AIProvider>(...)`
    - Remove provider select UI block (~lines 208-223)
    - Simplify `handleSave()`:
      * Remove provider conditional
      * Always create GoogleInfraConfig with `provider: 'google'`
  Pseudocode: DESIGN.md "Frontend: config-editor.tsx"
  Success:
    - No provider dropdown in UI
    - All saved configs have `provider: 'google'`
  Test: Manual UI test - create new config, verify no provider select
  Dependencies: Task 7 (types simplified)
  Time: 15min
  ```

- [x] Update `aiProviders.test.ts` for Google-only
  ```
  Files: convex/lib/aiProviders.test.ts
  Approach:
    - Remove OpenAI mock: `vi.mock('openai', { spy: true })`
    - Remove: `mockOpenAIConstructor` variable
    - Delete tests:
      * `it('initializes OpenAI provider...')` (~lines 65-94)
      * `it('throws when OPENAI_API_KEY is missing...')` (~lines 112-126)
    - Update remaining tests:
      * Function calls: `initializeProvider` → `initializeGoogleProvider`
      * Remove `provider` field assertions
      * Remove `openaiClient` assertions
  Pseudocode: DESIGN.md "Test Updates - aiProviders.test.ts"
  Success:
    - Only Google provider tests remain
    - Tests pass: `pnpm test convex/lib/aiProviders.test.ts`
  Test: Self-validating
  Dependencies: Task 2 (aiProviders simplified)
  Time: 15min
  ```

- [x] Remove `openai` dependency from package.json
  ```
  Files: package.json, pnpm-lock.yaml
  Approach:
    - `pnpm remove openai`
    - Verify `@ai-sdk/openai` can also be removed (not used anywhere)
    - If safe: `pnpm remove @ai-sdk/openai`
  Success:
    - `grep openai package.json` returns only @ai-sdk/google (or nothing)
    - `pnpm install` succeeds
    - `pnpm build` succeeds
  Test: `pnpm build:local` (full build)
  Dependencies: All code changes complete (Tasks 1-9)
  Time: 5min
  ```

- [x] Update `.env.example` - remove OpenAI vars
  ```
  Files: .env.example
  Approach:
    - Remove: `AI_PROVIDER=openai` line
    - Remove: `AI_MODEL=gpt-5-mini` line
    - Remove: `AI_REASONING_EFFORT=high` line
    - Remove: `OPENAI_API_KEY=sk-proj-...` line and surrounding comments
    - Update: Google AI section comments (no longer "legacy, kept for rollback")
    - Add: `AI_MODEL=gemini-3-pro-preview` in Google section
  Success:
    - No OpenAI references in .env.example
    - Google AI is primary, not "legacy"
  Test: N/A (documentation)
  Dependencies: None
  Time: 5min
  ```

- [x] Update `CLAUDE.md` - remove OpenAI AI Provider section
  ```
  Files: CLAUDE.md
  Approach:
    - Update "AI Provider Configuration" section:
      * Remove references to OpenAI as provider option
      * Update example env vars
      * Remove rollback-to-OpenAI instructions
    - Update env var table to show only:
      * `AI_MODEL` = `gemini-3-pro-preview`
      * `GOOGLE_AI_API_KEY` = (secret)
  Success:
    - CLAUDE.md reflects Google-only AI provider
    - No mention of OpenAI rollback
  Test: N/A (documentation)
  Dependencies: None
  Time: 5min
  ```

## Validation Checklist

After all tasks complete:
- [x] `rg -c openai convex/` returns 0 (no openai imports in convex)
- [x] `rg -c "provider ===" convex/` returns 0 (no provider branching)
- [x] `rg "from 'openai'" .` returns 0 (no openai imports)
- [x] `pnpm test` passes (866 tests pass)
- [x] `npx tsc --noEmit` succeeds (TypeScript compiles)

## Not In Scope (Deferred)

- Env var changes on Convex dashboard (deployment task, not code)
- Manual quality review of Gemini 3 Pro outputs (post-deployment)
- Sentry alert configuration (observability task)
