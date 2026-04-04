# Generative UI Foundation

Priority: medium
Status: ready
Estimate: L

## Goal

Replace hardcoded artifact rendering with schema-driven generative UI so the review agent can dynamically compose question layouts and feedback displays — the differentiator that makes Scry an agentic UX.

## Non-Goals

- General-purpose UI generation framework (scope to review experience)
- React Server Components migration
- Agent-generated CSS/styling

## Oracle

- [ ] Agent tools return structured `renderSpec` alongside data describing what component to render
- [ ] `<GenerativeRenderer>` component maps renderSpec objects to React components from existing ui/ library
- [ ] Artifact type system replaced with schema-driven rendering — new types don't require code changes
- [ ] Review agent renders all question types, feedback, stats, weak areas via renderSpec
- [ ] renderSpec validated with Zod; malformed specs show graceful fallback
- [ ] Streaming works: partial renderSpecs display progressive UI
- [ ] At least one novel composition exists that wasn't possible before (e.g., side-by-side concept comparison)
- [ ] Typed component registry: adding a renderable component = registry entry + component map entry, nothing else
- [ ] All existing review flows work identically through the new path

## Notes

**Current:** review-chat.tsx:870-922 hardcoded if/else: `artifact.type === 'feedback'` → `<FeedbackCard>`. Adding new UI = code change.

**Target:** Agent tool → `{ data, renderSpec: { component: 'QuestionCard', props } }` → `<GenerativeRenderer>` → registry lookup → render.

**Available building blocks:** 25 Radix + CVA components in components/ui/. Vercel AI SDK v5 installed but unused.

**Depends on:** Items 006 (all content types) + 007 (review-chat decomposed)
