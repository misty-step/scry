# Context Packet: Generative UI Foundation

## Spec

Replace the hardcoded `if/else` artifact renderer in `review-chat.tsx:870-922` with a schema-driven system where:

1. Each tool/mutation return value includes an optional `renderSpec` describing which component to render and with what props.
2. A typed `ComponentRegistry` maps component names to React components.
3. A `<GenerativeRenderer>` component takes a `renderSpec` + registry and renders the appropriate component, with error boundaries and fallback for unknown types.

The design is **not** `json-render` (Vercel's full generative UI framework). That framework assumes the LLM generates the spec via free-form text. In Scry, the backend tools produce deterministic structured data. We need a much thinner layer: a Zod-validated discriminated union that backend code constructs explicitly, rendered by a registry lookup on the frontend. No LLM-generated UI specs, no streaming spec compilation, no expression evaluation. Just type-safe plumbing from tool result to component.

### Decision: Thin Registry vs. json-render

| Criterion | json-render | Thin Registry (chosen) |
|---|---|---|
| Dependency | New dep (`@json-render/core`, `@json-render/react`) | Zero new deps (Zod already installed) |
| LLM generates layout | Yes (core feature) | No (backend code constructs specs) |
| Streaming spec compilation | Yes (JSON Patch) | Not needed (specs arrive complete from mutations) |
| Expression/state binding | `$state`, `$computed`, `$template` | Not needed (props are concrete values) |
| Complexity | High (catalog, schema, renderer, providers) | Low (~150 LOC total) |
| Type safety | Good (Zod catalog) | Excellent (discriminated union, registry is exhaustive) |

json-render solves a different problem (LLM free-form UI composition). Scry's review tools return structured data from deterministic mutations. The right abstraction is a discriminated union + component map.

---

## Architecture

### renderSpec Schema

```typescript
// lib/render-spec.ts
import { z } from 'zod';

// ---- Per-component prop schemas ----

const QuestionCardSpec = z.object({
  component: z.literal('QuestionCard'),
  props: z.object({
    conceptTitle: z.string().optional(),
    fsrsState: z.string().optional(),
    question: z.string(),
    type: z.string().default('multiple-choice'),
    options: z.array(z.string()).default([]),
    retrievability: z.number().optional(),
    lapses: z.number().optional(),
    reps: z.number().optional(),
    stability: z.number().optional(),
  }),
});

const FeedbackCardSpec = z.object({
  component: z.literal('FeedbackCard'),
  props: z.object({
    isCorrect: z.boolean(),
    correctAnswer: z.string(),
    userAnswer: z.string().optional(),
    explanation: z.string().optional(),
    conceptTitle: z.string().optional(),
    nextReview: z.number().optional(),
    scheduledDays: z.number().optional(),
    newState: z.string().optional(),
    totalAttempts: z.number().optional(),
    totalCorrect: z.number().optional(),
    lapses: z.number().optional(),
    reps: z.number().optional(),
  }),
});

const WeakAreasCardSpec = z.object({
  component: z.literal('WeakAreasCard'),
  props: z.object({
    generatedAt: z.number(),
    itemCount: z.number(),
    items: z.array(z.object({
      title: z.string(),
      state: z.string(),
      lapses: z.number(),
      reps: z.number(),
      dueNow: z.boolean(),
    })),
  }),
});

const RescheduledCardSpec = z.object({
  component: z.literal('RescheduledCard'),
  props: z.object({
    conceptTitle: z.string(),
    nextReview: z.number(),
    scheduledDays: z.number(),
  }),
});

const NoticeCardSpec = z.object({
  component: z.literal('NoticeCard'),
  props: z.object({
    title: z.string(),
    description: z.string(),
  }),
});

const SessionCompleteCardSpec = z.object({
  component: z.literal('SessionCompleteCard'),
  props: z.object({}).default({}),
});

const StatsCardSpec = z.object({
  component: z.literal('StatsCard'),
  props: z.object({
    conceptsDue: z.number(),
  }),
});

// ---- Discriminated union ----

export const RenderSpec = z.discriminatedUnion('component', [
  QuestionCardSpec,
  FeedbackCardSpec,
  WeakAreasCardSpec,
  RescheduledCardSpec,
  NoticeCardSpec,
  SessionCompleteCardSpec,
  StatsCardSpec,
]);

export type RenderSpec = z.infer<typeof RenderSpec>;

// Extract props type for a specific component
export type RenderSpecProps<C extends RenderSpec['component']> =
  Extract<RenderSpec, { component: C }>['props'];
```

### Example renderSpecs for Each Existing Artifact Type

**Question (from `formatDueResult`):**
```json
{
  "component": "QuestionCard",
  "props": {
    "conceptTitle": "Binary Search",
    "fsrsState": "learning",
    "question": "What is the time complexity of binary search?",
    "type": "multiple-choice",
    "options": ["O(n)", "O(log n)", "O(n log n)", "O(1)"],
    "stability": 2.5,
    "lapses": 1,
    "reps": 3
  }
}
```

**Feedback (from `buildSubmitAnswerPayload`):**
```json
{
  "component": "FeedbackCard",
  "props": {
    "isCorrect": true,
    "correctAnswer": "O(log n)",
    "userAnswer": "O(log n)",
    "explanation": "Binary search halves the search space each step.",
    "conceptTitle": "Binary Search",
    "nextReview": 1712345678000,
    "scheduledDays": 4,
    "newState": "review",
    "totalAttempts": 5,
    "totalCorrect": 4,
    "lapses": 1,
    "reps": 4
  }
}
```

**Weak Areas (from `getWeakAreasDirect`):**
```json
{
  "component": "WeakAreasCard",
  "props": {
    "generatedAt": 1712345678000,
    "itemCount": 3,
    "items": [
      { "title": "Heap Sort", "state": "relearning", "lapses": 4, "reps": 8, "dueNow": true }
    ]
  }
}
```

**Rescheduled:**
```json
{
  "component": "RescheduledCard",
  "props": {
    "conceptTitle": "Binary Search",
    "nextReview": 1712345678000,
    "scheduledDays": 3
  }
}
```

**Session Complete:**
```json
{
  "component": "SessionCompleteCard",
  "props": {}
}
```

---

### Component Registry

```typescript
// lib/component-registry.ts
import type { ComponentType } from 'react';
import type { RenderSpec, RenderSpecProps } from './render-spec';

/**
 * Maps every component name in RenderSpec to its React component.
 * TypeScript enforces exhaustiveness: if a new variant is added to
 * RenderSpec, this type errors until a component is registered.
 */
export type ComponentRegistry = {
  [C in RenderSpec['component']]: ComponentType<RenderSpecProps<C>>;
};

/**
 * Type-safe registry factory. Guarantees every RenderSpec component
 * has a corresponding React implementation at compile time.
 */
export function defineRegistry(registry: ComponentRegistry): ComponentRegistry {
  return registry;
}
```

**Registration (single file, one import per component):**
```typescript
// lib/review-registry.ts
import { defineRegistry } from './component-registry';
import { QuestionCard } from '@/components/agent/question-card';
import { FeedbackCard } from '@/components/agent/feedback-card';
import { WeakAreasCard } from '@/components/agent/weak-areas-card';
import { RescheduledCard } from '@/components/agent/rescheduled-card';
import { NoticeCard } from '@/components/agent/notice-card';
import { SessionCompleteCard } from '@/components/agent/session-complete-card';
import { StatsCard } from '@/components/agent/stats-card';

export const reviewRegistry = defineRegistry({
  QuestionCard,
  FeedbackCard,
  WeakAreasCard,
  RescheduledCard,
  NoticeCard,
  SessionCompleteCard,
  StatsCard,
});
```

**Adding a new renderable component requires exactly 2 changes:**
1. Add a variant to the `RenderSpec` discriminated union (Zod schema).
2. Add the component to the registry map (TypeScript enforces this).

No changes to `GenerativeRenderer`, no changes to the timeline rendering loop. This is the key architectural win.

---

### GenerativeRenderer Component

```typescript
// components/agent/generative-renderer.tsx
'use client';

import { ErrorBoundary } from 'react';  // React 19 built-in or custom
import type { RenderSpec } from '@/lib/render-spec';
import { RenderSpec as RenderSpecSchema } from '@/lib/render-spec';
import type { ComponentRegistry } from '@/lib/component-registry';

interface GenerativeRendererProps {
  spec: unknown;                    // Raw spec from backend — validated at render time
  registry: ComponentRegistry;
  fallback?: React.ReactNode;       // Shown when validation fails or component missing
  className?: string;
}

export function GenerativeRenderer({
  spec,
  registry,
  fallback,
  className,
}: GenerativeRendererProps) {
  // 1. Validate the spec
  const parsed = RenderSpecSchema.safeParse(spec);
  if (!parsed.success) {
    if (fallback) return <>{fallback}</>;
    return (
      <div className="rounded-2xl border border-border bg-background p-5 shadow-sm">
        <p className="text-sm text-muted-foreground">
          Unable to display this content.
        </p>
      </div>
    );
  }

  const { component, props } = parsed.data;

  // 2. Look up in registry
  const Component = registry[component] as React.ComponentType<Record<string, unknown>> | undefined;
  if (!Component) {
    if (fallback) return <>{fallback}</>;
    return (
      <div className="rounded-2xl border border-border bg-background p-5 shadow-sm">
        <p className="text-sm text-muted-foreground">
          Unknown component: {component}
        </p>
      </div>
    );
  }

  // 3. Render with error boundary
  return (
    <div className={className}>
      <ErrorBoundary fallback={
        <div className="rounded-2xl border border-destructive/30 bg-destructive/5 p-5">
          <p className="text-sm text-destructive">Failed to render {component}.</p>
        </div>
      }>
        <Component {...props} />
      </ErrorBoundary>
    </div>
  );
}
```

**Props contract:**
- `spec: unknown` -- intentionally untyped input; Zod validates at the boundary.
- `registry: ComponentRegistry` -- exhaustive map from compile-time type checking.
- `fallback?: ReactNode` -- override for graceful degradation.

**Error handling layers:**
1. Zod parse failure -> fallback UI (malformed spec from backend).
2. Missing registry entry -> "Unknown component" message (schema/registry out of sync).
3. React ErrorBoundary -> crash recovery (runtime error in component).

---

### Agent Tool Integration

The current tool handlers return `Record<string, unknown>`. The change is additive: each return value gains an optional `renderSpec` field alongside the existing data.

**Backend changes to `reviewToolHelpers.ts`:**

```typescript
// Add to formatDueResult return value:
export function formatDueResult(
  result: Record<string, unknown> | null
): Record<string, unknown> | null {
  if (!result) return null;
  // ... existing typed destructuring ...

  const data = {
    conceptId: typed.concept._id,
    conceptTitle: typed.concept.title,
    // ... all existing fields ...
  };

  return {
    ...data,
    renderSpec: {
      component: 'QuestionCard',
      props: {
        conceptTitle: typed.concept.title,
        fsrsState: typed.concept.fsrs.state ?? 'new',
        question: typed.phrasing.question,
        type: typed.phrasing.type ?? 'multiple-choice',
        options: typed.phrasing.options ?? [],
        retrievability: typed.retrievability,
        stability: typed.concept.fsrs.stability,
        lapses: typed.concept.fsrs.lapses ?? 0,
        reps: typed.concept.fsrs.reps ?? 0,
      },
    },
  };
}

// Add to buildSubmitAnswerPayload return value:
export function buildSubmitAnswerPayload(args: { ... }) {
  const data = {
    // ... all existing fields ...
  };

  return {
    ...data,
    renderSpec: {
      component: 'FeedbackCard',
      props: {
        isCorrect: args.isCorrect,
        correctAnswer: args.correctAnswer,
        userAnswer: args.userAnswer,
        explanation: args.explanation,
        conceptTitle: args.conceptTitle ?? '',
        nextReview: args.result.nextReview,
        scheduledDays: args.result.scheduledDays,
        newState: args.result.newState,
        totalAttempts: args.result.totalAttempts,
        totalCorrect: args.result.totalCorrect,
        lapses: args.result.lapses,
        reps: args.result.reps,
      },
    },
  };
}
```

Similarly for `getWeakAreasDirect`, `rescheduleConceptDirect`, and `getSessionStats` in `reviewStreaming.ts`.

**Frontend extraction pattern:**

The frontend currently receives tool results as `Record<string, unknown>` and builds `ArtifactEntry` objects with a discriminated `type` field. The migration preserves this pipeline but adds `renderSpec`:

```typescript
// Before (review-chat.tsx):
appendArtifact({
  id: questionId,
  createdAt: Date.now(),
  type: 'question',
  data: typedNext,
});

// After:
appendArtifact({
  id: questionId,
  createdAt: Date.now(),
  renderSpec: typedNext.renderSpec,  // Extract from tool result
  data: typedNext,                   // Keep raw data for non-rendering uses
});
```

The timeline renderer changes from:
```typescript
if (entry.type === 'feedback') {
  return <FeedbackCard ... />;
}
if (entry.type === 'question') {
  return <QuestionCard ... />;
}
```

To:
```typescript
if (entry.renderSpec) {
  return (
    <GenerativeRenderer
      key={item.key}
      spec={entry.renderSpec}
      registry={reviewRegistry}
    />
  );
}
```

---

## Migration Plan

### Phase 1: Foundation (no behavioral changes)

1. Create `lib/render-spec.ts` with the Zod schema.
2. Create `lib/component-registry.ts` with the registry type.
3. Create `components/agent/generative-renderer.tsx`.
4. Write tests for all three (schema validation, registry lookup, renderer behavior).

### Phase 2: Extract inline components

The `ActionPanelCard` and `PendingFeedbackCard` are currently private functions inside `review-chat.tsx`. Extract them as standalone components:

| Current location | New component file | Registry name |
|---|---|---|
| `ActionPanelCard` (lines 1068-1153) with `type: 'weak-areas'` | `components/agent/weak-areas-card.tsx` | `WeakAreasCard` |
| `ActionPanelCard` with `type: 'rescheduled'` | `components/agent/rescheduled-card.tsx` | `RescheduledCard` |
| `ActionPanelCard` with `type: 'notice'` | `components/agent/notice-card.tsx` | `NoticeCard` |
| Inline "All done" div (lines 911-921) | `components/agent/session-complete-card.tsx` | `SessionCompleteCard` |
| `PendingFeedbackCard` (lines 1155+) | Stays inline (transient state, not a renderSpec artifact) |

Existing `QuestionCard` and `FeedbackCard` are already standalone files. Their prop interfaces need minor alignment:
- `QuestionCard` currently takes `question: Record<string, unknown>` and casts internally. Change to accept the typed `RenderSpecProps<'QuestionCard'>` directly. Keep the `Record<string, unknown>` overload via a thin adapter during migration.
- `FeedbackCard` currently takes `feedback: Record<string, unknown>`. Same treatment.

### Phase 3: Add renderSpec to backend returns

Add `renderSpec` to `formatDueResult`, `buildSubmitAnswerPayload`, `getWeakAreasDirect`, `rescheduleConceptDirect`, and `getSessionStats`. These are purely additive -- existing fields remain unchanged, so the old frontend path still works.

### Phase 4: Wire up GenerativeRenderer

1. Create `lib/review-registry.ts` with all component registrations.
2. Replace the `if/else` chain in `review-chat.tsx:870-922` with `<GenerativeRenderer>`.
3. Update `ArtifactEntry` type to carry `renderSpec` instead of `type` discriminant.

### Phase 5: Remove old path

Delete the `type` field from `ArtifactEntry`. Delete the `if/else` rendering chain. The `GenerativeRenderer` is now the sole renderer.

### Validation at each phase

- After Phase 1-2: all existing tests pass, no behavioral changes.
- After Phase 3: backend returns include `renderSpec` but frontend ignores it.
- After Phase 4: dual-path -- `renderSpec` used when present, old `type` check as fallback.
- After Phase 5: single path, old code deleted.

---

## Streaming Design

### Current state: specs arrive complete

In the current architecture, artifacts are NOT streamed. The flow is:

1. User answers a question -> `submitAnswerDirect` mutation runs synchronously -> returns complete result.
2. `fetchNextQuestion` mutation runs synchronously -> returns complete question data.
3. `getWeakAreasDirect` / `rescheduleConceptDirect` -> same pattern.

The only streaming in the system is the **agent chat text** (via `useUIMessages` + `syncStreams`). Artifacts are produced by deterministic mutations, not by the streaming agent.

### Implication: no partial renderSpec problem

Since renderSpecs are constructed server-side by mutation handlers and returned as complete JSON, there is no partial/streaming spec scenario to handle. The spec is either present and complete, or absent.

### Future: agent-generated renderSpecs via tool calls

If in the future the agent's tool calls (via `reviewAgent` LLM) need to produce renderSpecs that stream, the pattern is:

1. Tool invocation results appear in `UIMessage.parts` as `type: 'tool-invocation'` with `state: 'call' | 'partial-call' | 'result'`.
2. During `state: 'partial-call'`, show a skeleton placeholder.
3. On `state: 'result'`, extract `renderSpec` from `result` and render via `GenerativeRenderer`.

Skeleton component for streaming:
```typescript
const SkeletonCardSpec = z.object({
  component: z.literal('SkeletonCard'),
  props: z.object({
    hint: z.enum(['question', 'feedback', 'stats']).optional(),
  }),
});
```

This would render a shimmer skeleton shaped like the expected component. But this is **out of scope for the initial implementation** since no current artifact uses streaming delivery.

---

## Implementation Sequence (TDD)

### Step 1: RenderSpec schema + tests
- **Red:** Test that valid specs for each component type parse successfully. Test that malformed specs (missing `component`, wrong prop types, unknown component name) fail validation.
- **Green:** Implement the Zod discriminated union in `lib/render-spec.ts`.
- **Refactor:** Ensure prop schemas match the existing component prop interfaces exactly.

### Step 2: ComponentRegistry type + defineRegistry
- **Red:** Test that a registry missing a component causes a TypeScript compile error (type-level test via `ts-expect-error`). Test that `defineRegistry` returns the input unchanged.
- **Green:** Implement `lib/component-registry.ts`.
- **Refactor:** Minimal -- this is ~15 LOC.

### Step 3: GenerativeRenderer component
- **Red:** Test rendering a valid spec produces the correct component. Test that an invalid spec shows fallback. Test that an unknown component name shows fallback. Test that a component throwing an error shows the error boundary.
- **Green:** Implement `components/agent/generative-renderer.tsx`.
- **Refactor:** Extract error boundary if React 19's built-in isn't sufficient.

### Step 4: Extract ActionPanelCard variants
- **Red:** Snapshot or structural tests for `WeakAreasCard`, `RescheduledCard`, `NoticeCard`, `SessionCompleteCard` matching current rendered output.
- **Green:** Extract from `review-chat.tsx` into standalone files.
- **Refactor:** Align prop interfaces with `RenderSpecProps<'...'>` types.

### Step 5: Adapt QuestionCard and FeedbackCard prop interfaces
- **Red:** Existing tests continue to pass. New tests verify typed props work.
- **Green:** Add typed overloads or migrate to `RenderSpecProps<'QuestionCard'>`.
- **Refactor:** Remove internal `as` casts that exist today.

### Step 6: Add renderSpec to backend tool return values
- **Red:** Contract tests verify `renderSpec` is present and valid in `formatDueResult`, `buildSubmitAnswerPayload`, etc.
- **Green:** Add `renderSpec` construction in each helper.
- **Refactor:** Extract a `buildRenderSpec` helper if patterns are repetitive.

### Step 7: Build review-registry.ts
- **Red:** Test that registry contains all RenderSpec component names.
- **Green:** Wire up imports in `lib/review-registry.ts`.
- **Refactor:** Minimal.

### Step 8: Replace if/else chain with GenerativeRenderer
- **Red:** Integration test showing the timeline renders via `GenerativeRenderer`.
- **Green:** Replace `review-chat.tsx:870-922` with `<GenerativeRenderer>`.
- **Refactor:** Remove the dead `type` discriminant code path once all specs flow through.

### Step 9: Novel composition (oracle requirement)
- **Red:** Test for a `ConceptComparisonCard` that renders two concepts side-by-side.
- **Green:** Add the spec variant + component + registry entry. Backend produces this spec when appropriate (e.g., `getWeakAreasDirect` could include a comparison view for the top 2 weak areas).
- **Refactor:** Verify this required exactly: 1 Zod variant + 1 component file + 1 registry line.

---

## Reference Implementations

### Vercel json-render (inspiration, not dependency)
- [json-render.dev](https://json-render.dev/) -- catalog/registry/renderer pattern.
- [GitHub: vercel-labs/json-render](https://github.com/vercel-labs/json-render) -- Apache 2.0, 13k+ stars.
- [InfoQ coverage](https://www.infoq.com/news/2026/03/vercel-json-render/) -- architecture overview.
- Key takeaway: the catalog-registry-renderer layering is sound, but the full framework (expressions, state binding, streaming patches, multi-framework renderers) is overkill for Scry's deterministic tool returns.

### Vercel AI SDK Generative UI
- [AI SDK docs](https://ai-sdk.dev/docs/introduction) -- `createStreamableUI` for RSC-based generative UI.
- [Vercel blog: AI SDK 3.0](https://vercel.com/blog/ai-sdk-3-generative-ui) -- pattern overview.
- [Vercel Academy: Multi-Step & Generative UI](https://vercel.com/academy/ai-sdk/multi-step-and-generative-ui) -- tutorial.
- Key takeaway: `@ai-sdk/rsc` is experimental and requires React Server Components. Scry uses client-side rendering with Convex reactive queries. Not applicable directly, but the mental model (tool call -> UI component) is the same.

### Convex Agent tool results
- [Convex Agent docs](https://docs.convex.dev/agents) -- tool definition, `createTool`, structured output.
- [Convex Agent component](https://www.convex.dev/components/agent) -- `UIMessage`, `toUIMessages`, streaming.
- Key takeaway: tool results are saved as JSON in message history. The `renderSpec` is just another field in that JSON. No special Convex plumbing needed.

### Discriminated union pattern
- Zod's `z.discriminatedUnion` provides O(1) parsing by checking the discriminant field first. This is ideal for the component dispatch pattern where we switch on `component` name.

---

## Risks

### 1. Type safety across the agent-to-frontend boundary
**Risk:** The Convex backend returns `Record<string, unknown>` from tools. The frontend must validate at runtime.
**Mitigation:** Zod `safeParse` in `GenerativeRenderer`. The schema is the single source of truth shared across backend construction and frontend validation. Backend helpers should use the same Zod schemas to construct specs (parse-don't-validate pattern), ensuring compile-time correctness on the backend side.

### 2. Prop interface drift between components and schema
**Risk:** A component's actual props diverge from its `RenderSpec` prop schema, causing runtime errors.
**Mitigation:** The `ComponentRegistry` type enforces that `registry[C]` accepts `RenderSpecProps<C>`. If the component's props change, TypeScript errors until the schema is updated (or vice versa). This is a compile-time guarantee.

### 3. Performance of dynamic rendering vs. static components
**Risk:** Zod parsing + registry lookup + ErrorBoundary wrapper adds overhead vs. direct component rendering.
**Mitigation:** Negligible. Zod discriminated union parse is O(1) on the discriminant. Registry lookup is a property access. ErrorBoundary is a standard React pattern. The timeline renders at most ~60 items (current `MAX_ARTIFACT_ENTRIES`). Benchmark if needed, but this is not a hot path.

### 4. Migration breakage during dual-path phase
**Risk:** During Phase 4, both old (`type`-based) and new (`renderSpec`-based) rendering coexist. A bug in one path could be masked by the other.
**Mitigation:** Phase 4 uses renderSpec when present, falls back to old path. Phase 5 removes old path only after all backend endpoints emit renderSpec. Integration tests cover both paths explicitly.

### 5. Bundle size from component registry
**Risk:** Importing all registry components eagerly increases the review page bundle.
**Mitigation:** All registry components are already imported in `review-chat.tsx` today (QuestionCard, FeedbackCard, ActionPanelCard). The registry just moves these imports to a separate file. No new code is loaded. If future components are heavy, use `React.lazy()` in the registry (the `ComponentType` type supports it).

### 6. `PendingFeedbackCard` is not a renderSpec artifact
**Risk:** Temptation to force all cards through renderSpec.
**Mitigation:** `PendingFeedbackCard` represents ephemeral client-side state (optimistic submission UI), not a server-returned artifact. It stays outside the registry system. The renderSpec system is for server-returned structured data only. Document this invariant.

---

## Files to Create

| File | Purpose | LOC (est.) |
|---|---|---|
| `lib/render-spec.ts` | Zod schema for all renderSpec variants | ~90 |
| `lib/render-spec.test.ts` | Schema validation tests | ~80 |
| `lib/component-registry.ts` | Registry type + `defineRegistry` | ~20 |
| `lib/component-registry.test.ts` | Type-level + runtime tests | ~30 |
| `lib/review-registry.ts` | Concrete registry for review experience | ~25 |
| `components/agent/generative-renderer.tsx` | Renderer component | ~50 |
| `components/agent/generative-renderer.test.tsx` | Renderer tests | ~80 |
| `components/agent/weak-areas-card.tsx` | Extracted from ActionPanelCard | ~40 |
| `components/agent/rescheduled-card.tsx` | Extracted from ActionPanelCard | ~25 |
| `components/agent/notice-card.tsx` | Extracted from ActionPanelCard | ~20 |
| `components/agent/session-complete-card.tsx` | Extracted from inline div | ~15 |
| `components/agent/stats-card.tsx` | New (for getSessionStats) | ~20 |

## Files to Modify

| File | Change |
|---|---|
| `convex/agents/reviewToolHelpers.ts` | Add `renderSpec` to `formatDueResult` and `buildSubmitAnswerPayload` returns |
| `convex/agents/reviewStreaming.ts` | Add `renderSpec` to `getWeakAreasDirect`, `rescheduleConceptDirect`, `getSessionStats` returns |
| `components/agent/review-chat.tsx` | Replace if/else artifact chain with `<GenerativeRenderer>`, update `ArtifactEntry` type |
| `components/agent/question-card.tsx` | Accept typed props alongside existing `Record<string, unknown>` |
| `components/agent/feedback-card.tsx` | Accept typed props alongside existing `Record<string, unknown>` |
