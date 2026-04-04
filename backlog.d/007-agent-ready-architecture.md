# Agent-Ready Architecture

Priority: medium
Status: ready
Estimate: L

## Goal

Split the four largest modules into focused, independently-testable sub-modules so AI coding agents can safely modify one concern without risking cascades. Target <400 lines per file.

## Non-Goals

- Rewriting business logic (extract, don't rewrite)
- Changing public Convex API signatures
- Over-decomposing into micro-modules

## Oracle

- [ ] `convex/concepts.ts` (1295 lines) → <400 lines + extracted lib modules for CRUD, FSRS scheduling, phrasing management, lifecycle state machines
- [ ] `convex/aiGeneration.ts` (1202 lines) → <400 lines + extracted lib modules for pipeline orchestration, phrasing generation, validation
- [ ] `convex/iqc.ts` (827 lines) → <400 lines + extracted lib modules for candidate selection, merge adjudication
- [ ] `components/agent/review-chat.tsx` (1389 lines) → <400 lines + extracted hooks and sub-components
- [ ] Zero circular imports between extracted modules
- [ ] All existing tests pass without modification
- [ ] New unit tests for extracted pure functions
- [ ] Coverage exclusions for concepts.ts, aiGeneration.ts, iqc.ts removed from vitest.config.ts

## Notes

Follows existing `convex/lib/` patterns (conceptHelpers.ts, conceptFsrsHelpers.ts, userStatsHelpers.ts).

**Depends on:** Item 005 (quality gates must be hard before large refactors)
