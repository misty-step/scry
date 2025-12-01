# TODO: 70%+ Coverage Sprint

## Context
- **Current**: ~44% coverage (up from 39%)
- **Target**: 70%+ with hard threshold enforcement
- **Approach**: Dedicated test sprint → Set 70% threshold → Enable PR comments → Update badges
- **Patterns**: Use `/tests/helpers/convexFixtures.ts`, `createMockDb()`, `createMockCtx()`

## Test Sprint Tasks

### Tier 1: Critical Infrastructure (Completed)

- [x] Test `convex/fsrs/engine.ts` (146 lines)
  - Coverage: 97.36% lines
- [x] Test `convex/spacedRepetition.ts` (275 lines)
  - Coverage: 92.45% lines
- [x] Test `convex/rateLimit.ts` (319 lines)
  - Coverage: 98.78% lines
- [x] Test `lib/ai-client.ts` (464 lines)
  - Coverage: 100% lines

### Tier 2: High-Value Modules (In Progress)

- [x] Test `convex/userStats.ts` (341 lines)
  - Coverage: 88.23% lines
- [x] Test `convex/questionsCrud.ts` (366 lines)
  - Coverage: 89.15% lines
- [x] Test `convex/aiGeneration.ts` (984 lines)
  - Coverage: 45.45% (Needs improvement)
  - Focus: Error handling, prompt construction, stage transitions
- [x] Test `convex/generationJobs.ts` (363 lines)
  - Coverage: 65.95% (Target 80%+)
- [x] Test `convex/concepts.ts` (1,247 lines)
  - Coverage: 25.72% (CRITICAL GAP)
  - This is the largest module with lowest coverage.
- [x] Test `convex/embeddings.ts` (1,048 lines)
  - Coverage: 26.93% (CRITICAL GAP)

## Infrastructure Tasks

- [x] Update `vitest.config.ts` with ratcheted thresholds
  - Current: 43.8% lines, 38.3% functions, 34.8% branches
- [x] Update `ci.yml` with vitest-coverage-report-action
- [x] Replace README badges with shields.io

## Success Criteria

- [ ] Coverage ≥70% lines/statements/functions
- [ ] Coverage ≥60% branches
- [x] PRs blocked on coverage regression (Thresholds set)
- [x] PR comments show coverage delta
- [x] README badge shows real coverage
- [x] Zero external service dependencies