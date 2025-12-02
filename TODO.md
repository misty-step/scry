# TODO: Test Coverage 43.8% → 70%

## Context
- Architecture: Pure Function Extraction + Ratcheted Thresholds (DESIGN.md)
- Key Files: convex/concepts.ts (god object), hooks/use-review-flow.ts, lib/utils/*.ts
- Patterns: Co-located tests, vitest with vi.useFakeTimers, describe/it structure
- No infrastructure tasks needed - CI coverage reporting already works

## Phase 1: Easy Pure Functions (0% → 100%)

- [x] Add tests for lib/utils/shuffle.ts
  ```
  Files: lib/utils/shuffle.test.ts (new)
  Pattern: Follow lib/format-review-time.test.ts structure
  Tests: empty array, single element, same length, same elements, no mutation, statistical variance
  Success: pnpm test shuffle.test passes, coverage shows 100%
  Dependencies: None (parallel-safe)
  Time: 15min
  ```

- [ ] Add tests for lib/utils/date-format.ts
  ```
  Files: lib/utils/date-format.test.ts (new)
  Pattern: Use vi.useFakeTimers() like format-review-time.test.ts
  Tests: formatShortRelativeTime (now, minutes, hours, days, weeks, months, future)
         formatCardDate (with/without ago suffix)
         formatDueTime (now, minutes, hours, tomorrow, days, date format)
  Success: pnpm test date-format.test passes, coverage shows 100%
  Dependencies: None (parallel-safe)
  Time: 30min
  ```

## Phase 2: Extract Pure Functions from concepts.ts

- [ ] Create convex/lib/conceptHelpers.ts with extracted functions
  ```
  Files: convex/lib/conceptHelpers.ts (new), convex/concepts.ts (modify)
  Extract from concepts.ts:
    - clampPageSize (line 524) → add explicit defaults param
    - matchesConceptView (line 531) → export ConceptLibraryView type
    - computeThinScoreFromCount (line 819) → add targetPhrasings param
    - prioritizeConcepts (line 300) → add optional random param for testability
  Success: convex/concepts.ts imports from ./lib/conceptHelpers, no behavior change
  Test: Run existing app, verify review flow works
  Dependencies: None
  Time: 45min
  ```

- [ ] Add tests for convex/lib/conceptHelpers.ts
  ```
  Files: convex/lib/conceptHelpers.test.ts (new)
  Tests per DESIGN.md:
    - clampPageSize: undefined→default, below min, above max, valid range
    - matchesConceptView: deleted/archived filtering, due check, thin/tension scores
    - computeThinScoreFromCount: at target, below target, above target
    - prioritizeConcepts: sorts by retrievability, handles empty array
  Success: pnpm test conceptHelpers.test passes, 100% coverage on new file
  Dependencies: conceptHelpers.ts extraction complete
  Time: 45min
  ```

## Phase 3: Test Review Flow Reducer (already exported)

- [ ] Add tests for hooks/use-review-flow.ts reducer
  ```
  Files: hooks/use-review-flow.test.ts (new)
  Import: reviewReducer from './use-review-flow'
  Tests per DESIGN.md:
    - LOAD_START: sets loading, clears transitioning
    - LOAD_EMPTY: clears all state, sets empty phase
    - LOAD_TIMEOUT: sets error phase with message
    - QUESTION_RECEIVED: populates from payload, sets reviewing phase
    - REVIEW_COMPLETE: releases lockId, marks transitioning
    - IGNORE_UPDATE: returns unchanged state
  Success: pnpm test use-review-flow.test passes
  Dependencies: None (reducer already exported)
  Time: 30min
  ```

## Phase 4: Expand IQC Pure Function Tests

- [ ] Export and test additional IQC helpers
  ```
  Files: convex/iqc.ts (modify exports), convex/iqc.test.ts (expand)
  Export: tokenizeTitle, accumulateStatDelta (already internal)
  New tests:
    - tokenizeTitle: handles punctuation, lowercase, short words filtered
    - accumulateStatDelta: null delta, partial delta, full merge
  Success: iqc.ts coverage increases from 13% to 40%+
  Dependencies: None
  Time: 30min
  ```

## Phase 5: Push Near-100% Files to 100%

- [ ] Add edge case tests for convex/fsrs/engine.ts
  ```
  Files: convex/fsrs/engine.test.ts (expand)
  Missing coverage:
    - getRetrievability(null) → -1
    - getRetrievability({ state: 'new', reps: 0 }) → -1
    - isDue({ nextReview: undefined }) → true
  Success: fsrs/engine.ts coverage 97% → 100%
  Dependencies: None
  Time: 15min
  ```

## Phase 6: Ratchet Thresholds

- [ ] Update vitest.config.ts thresholds to final targets
  ```
  Files: vitest.config.ts
  Change thresholds to:
    lines: 70
    functions: 65
    branches: 55
    statements: 70
  Success: pnpm test:coverage passes with new thresholds
  Dependencies: All previous tasks complete
  Time: 5min
  ```

## Verification

After all tasks:
```bash
pnpm test:coverage
# Should show: lines ≥70%, functions ≥65%, branches ≥55%, statements ≥70%
```

## Not in Scope (per DESIGN.md)
- Testing hooks with useEffect/timers (flaky)
- Testing mutations directly (need Convex test infra)
- Badge generation (maintenance burden)
- E2E coverage expansion
