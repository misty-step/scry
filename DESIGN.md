# DESIGN.md - Test Coverage 51% → 70%

## Architecture Overview

**Selected Approach**: Pure Function Extraction + Ratcheted Thresholds

**Rationale**: Code is hard to test because pure logic is entangled with I/O. Extract pure functions into testable modules, then tests become trivial. Ratcheting prevents regression.

**Core Modules**:
- `convex/lib/conceptHelpers.ts`: Pure concept validation/filtering logic extracted from concepts.ts
- `hooks/lib/reviewFlowHelpers.ts`: Pure reducer and session logic from use-review-flow.ts
- `convex/lib/iqcHelpers.ts`: Already exists - pure merge candidate logic
- `lib/utils/*.ts`: Already pure - just need test files

**Data Flow**:
```
God Object (concepts.ts) → Extract Pure Functions → convex/lib/conceptHelpers.ts
                        → Thin Mutation keeps orchestration
                        → Tests cover pure functions (easy, fast, deterministic)
```

**Key Design Decisions**:
1. **Co-located tests**: Tests live next to source files (`foo.ts` → `foo.test.ts`)
2. **No test infrastructure changes**: Use existing vitest setup, CI already has coverage reports
3. **Ratcheting thresholds**: Update vitest.config.ts after each improvement phase
4. **No mocking internal functions**: Test public interfaces only

## Phase 1: Infrastructure Updates

### Module: vitest.config.ts Thresholds

**Responsibility**: Enforce coverage floors, prevent regression

**Current State**:
```typescript
thresholds: {
  lines: 43.8,
  functions: 38.3,
  branches: 34.8,
  statements: 43.6,
}
```

**Target State** (after all phases):
```typescript
thresholds: {
  lines: 70,
  functions: 65,
  branches: 55,
  statements: 70,
}
```

**Ratcheting Schedule**:
| Phase | Lines | Functions | Branches | Statements |
|-------|-------|-----------|----------|------------|
| Current | 43.8 | 38.3 | 34.8 | 43.6 |
| After Phase 2 | 55 | 50 | 45 | 55 |
| After Phase 3 | 62 | 58 | 50 | 62 |
| After Phase 4 | 70 | 65 | 55 | 70 |

**Excludes to Add** (thin wrappers, untestable):
```typescript
exclude: [
  // ... existing excludes
  'lib/test-utils/**',
  'lib/sentry.ts',        // Third-party wrapper
]
```

## Phase 2: Extract Pure Functions from God Objects

### Module: convex/lib/conceptHelpers.ts

**Responsibility**: Hide concept validation, filtering, and score calculation complexity

**Functions to Extract from `convex/concepts.ts`**:

```typescript
// convex/lib/conceptHelpers.ts

/**
 * Validate page size within bounds
 * @pure
 */
export function clampPageSize(
  pageSize: number | null | undefined,
  defaults: { min: number; max: number; default: number }
): number

/**
 * Check if concept matches library view filter
 * @pure
 */
export function matchesConceptView(
  concept: ConceptDoc,
  nowMs: number,
  view: ConceptLibraryView
): boolean

/**
 * Calculate thinScore based on phrasing count
 * @pure
 */
export function computeThinScoreFromCount(
  count: number,
  targetPhrasings: number
): number | undefined

/**
 * Prioritize concepts by retrievability with urgency tier shuffling
 * @pure (deterministic when passed seeded random)
 */
export function prioritizeConcepts(
  concepts: ConceptDoc[],
  now: Date,
  getRetrievability: (fsrs: FsrsState, now: Date) => number,
  random?: () => number
): Array<{ concept: ConceptDoc; retrievability: number }>
```

**Data Structures**:
```typescript
type ConceptLibraryView = 'all' | 'due' | 'thin' | 'tension' | 'archived' | 'deleted';

interface ConceptDoc {
  fsrs: { nextReview: number; state?: string };
  deletedAt?: number;
  archivedAt?: number;
  thinScore?: number;
  conflictScore?: number;
  phrasingCount: number;
}
```

**Test Strategy**:
```typescript
// convex/lib/conceptHelpers.test.ts
describe('clampPageSize', () => {
  it('returns default when undefined', () => {
    expect(clampPageSize(undefined, { min: 10, max: 100, default: 25 })).toBe(25);
  });

  it('clamps below minimum', () => {
    expect(clampPageSize(5, { min: 10, max: 100, default: 25 })).toBe(10);
  });

  it('clamps above maximum', () => {
    expect(clampPageSize(200, { min: 10, max: 100, default: 25 })).toBe(100);
  });
});

describe('matchesConceptView', () => {
  it('returns true for deleted view when concept deleted', () => {
    const concept = { deletedAt: 123, archivedAt: undefined };
    expect(matchesConceptView(concept, Date.now(), 'deleted')).toBe(true);
  });

  it('returns false for active views when archived', () => {
    const concept = { archivedAt: 123, deletedAt: undefined };
    expect(matchesConceptView(concept, Date.now(), 'due')).toBe(false);
  });

  it('returns true for due view when nextReview in past', () => {
    const now = Date.now();
    const concept = { fsrs: { nextReview: now - 1000 }, deletedAt: undefined, archivedAt: undefined };
    expect(matchesConceptView(concept, now, 'due')).toBe(true);
  });
});

describe('computeThinScoreFromCount', () => {
  it('returns undefined when at target', () => {
    expect(computeThinScoreFromCount(5, 5)).toBeUndefined();
  });

  it('returns delta when below target', () => {
    expect(computeThinScoreFromCount(2, 5)).toBe(3);
  });
});

describe('prioritizeConcepts', () => {
  it('sorts by retrievability ascending', () => {
    const concepts = [
      { fsrs: { nextReview: 0 }, phrasingCount: 1 },
      { fsrs: { nextReview: 0 }, phrasingCount: 1 },
    ];
    const mockRetrievability = vi.fn()
      .mockReturnValueOnce(0.8)
      .mockReturnValueOnce(0.3);

    const result = prioritizeConcepts(concepts, new Date(), mockRetrievability);
    expect(result[0].retrievability).toBe(0.3); // Lower retrievability = higher priority
  });

  it('shuffles urgent tier deterministically with seeded random', () => {
    // Test with seeded random for determinism
  });
});
```

**Migration Path in concepts.ts**:
```typescript
// Before: inline function
function clampPageSize(pageSize?: number | null) {
  if (!pageSize) return DEFAULT_LIBRARY_PAGE_SIZE;
  return Math.max(MIN_LIBRARY_PAGE_SIZE, Math.min(MAX_LIBRARY_PAGE_SIZE, pageSize));
}

// After: import from helper
import { clampPageSize } from './lib/conceptHelpers';

// Usage unchanged in mutation handler
const pageSize = clampPageSize(args.pageSize, {
  min: MIN_LIBRARY_PAGE_SIZE,
  max: MAX_LIBRARY_PAGE_SIZE,
  default: DEFAULT_LIBRARY_PAGE_SIZE,
});
```

### Module: hooks/lib/reviewFlowHelpers.ts

**Responsibility**: Pure review state machine logic, session ID generation

**Functions to Extract from `hooks/use-review-flow.ts`**:

```typescript
// hooks/lib/reviewFlowHelpers.ts

/**
 * Review state machine reducer (ALREADY EXPORTED - just needs tests)
 * @pure
 */
export function reviewReducer(state: ReviewModeState, action: ReviewAction): ReviewModeState

/**
 * Generate unique session ID
 * @pure (deterministic when crypto unavailable)
 */
export function generateSessionId(): string

/**
 * Determine if poll update should be ignored
 * @pure
 */
export function shouldIgnoreUpdate(
  state: ReviewModeState,
  dataChanged: boolean,
  hasLockId: boolean
): { ignore: boolean; reason?: string }

/**
 * Build SimpleQuestion from phrasing data
 * @pure
 */
export function buildQuestionFromPhrasing(phrasing: PhrasingData): SimpleQuestion
```

**Test Strategy**:
```typescript
// hooks/lib/reviewFlowHelpers.test.ts
describe('reviewReducer', () => {
  const initialState: ReviewModeState = {
    phase: 'loading',
    question: null,
    interactions: [],
    conceptId: null,
    conceptTitle: null,
    phrasingId: null,
    phrasingIndex: null,
    totalPhrasings: null,
    selectionReason: null,
    lockId: null,
    isTransitioning: false,
    conceptFsrs: null,
  };

  describe('LOAD_START', () => {
    it('sets phase to loading', () => {
      const result = reviewReducer(initialState, { type: 'LOAD_START' });
      expect(result.phase).toBe('loading');
      expect(result.isTransitioning).toBe(false);
    });
  });

  describe('LOAD_EMPTY', () => {
    it('clears all state and sets phase to empty', () => {
      const stateWithData = { ...initialState, conceptId: 'some-id' as any };
      const result = reviewReducer(stateWithData, { type: 'LOAD_EMPTY' });
      expect(result.phase).toBe('empty');
      expect(result.conceptId).toBeNull();
    });
  });

  describe('QUESTION_RECEIVED', () => {
    it('populates state from payload', () => {
      const payload = {
        question: { question: 'Q?', options: [], correctAnswer: 'A' },
        interactions: [],
        conceptId: 'concept-1' as any,
        conceptTitle: 'Title',
        phrasingId: 'phrasing-1' as any,
        phrasingStats: { index: 1, total: 3 },
        selectionReason: 'canonical',
        lockId: 'lock-123',
        conceptFsrs: { state: 'review' as const, reps: 5 },
      };
      const result = reviewReducer(initialState, { type: 'QUESTION_RECEIVED', payload });
      expect(result.phase).toBe('reviewing');
      expect(result.question).toEqual(payload.question);
      expect(result.lockId).toBe('lock-123');
    });
  });

  describe('REVIEW_COMPLETE', () => {
    it('releases lock but keeps question visible', () => {
      const reviewing = { ...initialState, phase: 'reviewing' as const, lockId: 'lock-123' };
      const result = reviewReducer(reviewing, { type: 'REVIEW_COMPLETE' });
      expect(result.phase).toBe('reviewing');
      expect(result.lockId).toBeNull();
      expect(result.isTransitioning).toBe(true);
    });
  });
});

describe('generateSessionId', () => {
  it('returns string with expected format', () => {
    const id = generateSessionId();
    expect(typeof id).toBe('string');
    expect(id.length).toBeGreaterThan(10);
  });

  it('generates unique IDs', () => {
    const ids = new Set(Array.from({ length: 100 }, generateSessionId));
    expect(ids.size).toBe(100);
  });
});
```

### Module: convex/lib/iqcHelpers.ts

**Responsibility**: Merge candidate evaluation logic

**Status**: Already exported from iqc.ts, already tested in iqc.test.ts

**Existing Pure Functions**:
- `buildProposalKey(a, b)` - ✅ tested
- `computeTitleSimilarity(a, b)` - ✅ tested
- `shouldConsiderMerge(vector, title, src, tgt)` - ✅ tested

**Additional Extraction** (not tested):
```typescript
// Extract from iqc.ts line 838
export function snapshotConcept(concept: ConceptDoc): ConceptSnapshot

// Extract from iqc.ts line 850
export function accumulateStatDelta(target: StatDeltas, delta?: StatDeltas | null): void

// Extract from iqc.ts line 781
export function tokenizeTitle(title: string): Set<string>
```

## Phase 3: Test Easy Pure Functions

### Module: lib/utils/date-format.ts

**Status**: 0% coverage, 79 lines, all pure functions

**Functions**:
- `formatShortRelativeTime(timestamp)` - returns "12d", "3h", "now"
- `formatCardDate(timestamp, includeAgo)` - adds "ago" suffix
- `formatDueTime(dueTimestamp)` - returns "Due now", "Due in 3h"

**Test Strategy**:
```typescript
// lib/utils/date-format.test.ts
describe('formatShortRelativeTime', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-08T12:00:00'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "now" for recent timestamps', () => {
    const recent = Date.now() - 30 * 1000; // 30 seconds ago
    expect(formatShortRelativeTime(recent)).toBe('now');
  });

  it('returns minutes for < 1 hour', () => {
    const thirtyMin = Date.now() - 30 * 60 * 1000;
    expect(formatShortRelativeTime(thirtyMin)).toBe('30m');
  });

  it('returns hours for < 1 day', () => {
    const fiveHours = Date.now() - 5 * 60 * 60 * 1000;
    expect(formatShortRelativeTime(fiveHours)).toBe('5h');
  });

  it('returns days for < 1 week', () => {
    const threeDays = Date.now() - 3 * 24 * 60 * 60 * 1000;
    expect(formatShortRelativeTime(threeDays)).toBe('3d');
  });

  it('handles future dates gracefully', () => {
    const future = Date.now() + 2 * 60 * 60 * 1000;
    expect(formatShortRelativeTime(future)).toBe('in 2h');
  });
});

describe('formatDueTime', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-08T12:00:00'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "Due now" for past timestamps', () => {
    expect(formatDueTime(Date.now() - 1000)).toBe('Due now');
  });

  it('returns minutes for near future', () => {
    expect(formatDueTime(Date.now() + 30 * 60 * 1000)).toBe('Due in 30m');
  });

  it('returns "Due tomorrow" for next day', () => {
    expect(formatDueTime(Date.now() + 36 * 60 * 60 * 1000)).toBe('Due tomorrow');
  });
});
```

### Module: lib/utils/shuffle.ts

**Status**: 0% coverage, 24 lines, pure function

**Test Strategy**:
```typescript
// lib/utils/shuffle.test.ts
describe('shuffle', () => {
  it('returns empty array for empty input', () => {
    expect(shuffle([])).toEqual([]);
  });

  it('returns single element unchanged', () => {
    expect(shuffle([1])).toEqual([1]);
  });

  it('returns array of same length', () => {
    const input = [1, 2, 3, 4, 5];
    expect(shuffle(input)).toHaveLength(5);
  });

  it('contains same elements', () => {
    const input = [1, 2, 3, 4, 5];
    expect(shuffle(input).sort()).toEqual([1, 2, 3, 4, 5]);
  });

  it('does not mutate original array', () => {
    const input = [1, 2, 3, 4, 5];
    const original = [...input];
    shuffle(input);
    expect(input).toEqual(original);
  });

  it('produces different orderings (statistical)', () => {
    const input = [1, 2, 3, 4, 5];
    const results = new Set<string>();
    // Run 100 times - should get at least 50 unique orderings
    for (let i = 0; i < 100; i++) {
      results.add(JSON.stringify(shuffle(input)));
    }
    expect(results.size).toBeGreaterThan(50);
  });
});
```

## Phase 4: Push High-Coverage Files to 100%

### Module: convex/fsrs/engine.ts

**Status**: 97% coverage, needs edge case tests

**Missing Coverage** (identified from code review):
- `getRetrievability` when state is null → returns -1
- `isDue` when nextReview undefined → returns true
- `mapDbStateToFsrs` default case
- `mapFsrsStateToDb` default case

**Test Additions**:
```typescript
// convex/fsrs/engine.test.ts (additions)
describe('getRetrievability edge cases', () => {
  it('returns -1 for null state', () => {
    expect(engine.getRetrievability(null)).toBe(-1);
  });

  it('returns -1 for new concepts with no reps', () => {
    const state = { state: 'new', reps: 0, nextReview: Date.now() };
    expect(engine.getRetrievability(state)).toBe(-1);
  });
});

describe('isDue edge cases', () => {
  it('returns true for undefined nextReview', () => {
    expect(engine.isDue({ nextReview: undefined })).toBe(true);
  });
});
```

### Module: lib/smart-polling.ts

**Status**: 96% coverage, needs one edge case

## File Organization

```
convex/
  lib/
    conceptHelpers.ts          # NEW: Pure concept logic
    conceptHelpers.test.ts     # NEW: Tests for above
    iqcHelpers.ts             # EXISTS: Already has exports
    iqc.test.ts               # EXISTS: Already has some tests

hooks/
  lib/
    reviewFlowHelpers.ts       # NEW: Pure review logic
    reviewFlowHelpers.test.ts  # NEW: Tests for above

lib/
  utils/
    date-format.ts            # EXISTS: 0% coverage
    date-format.test.ts       # NEW: Tests for above
    shuffle.ts                # EXISTS: 0% coverage
    shuffle.test.ts           # NEW: Tests for above
```

## Implementation Pseudocode

### Extract clampPageSize

```pseudocode
1. Create convex/lib/conceptHelpers.ts
2. Copy clampPageSize function from concepts.ts
3. Add explicit parameter types:
   - pageSize: number | null | undefined
   - defaults: { min: number; max: number; default: number }
4. Export function
5. In concepts.ts:
   - Add import from './lib/conceptHelpers'
   - Replace inline function call with imported function
   - Pass constants as config object
6. Create conceptHelpers.test.ts with tests
7. Run tests, verify passing
8. Run coverage, verify increase
```

### Extract matchesConceptView

```pseudocode
1. In convex/lib/conceptHelpers.ts
2. Copy matchesConceptView function
3. Define ConceptLibraryView type
4. Define minimal ConceptDoc interface for parameter
5. Export function and types
6. In concepts.ts:
   - Import function
   - Replace inline usage
7. Add tests covering:
   - deleted view with deleted concept
   - deleted view with non-deleted concept
   - archived view with archived concept
   - active views exclude deleted
   - active views exclude archived
   - due view with past nextReview
   - due view with future nextReview
   - thin view with positive thinScore
   - tension view with positive conflictScore
```

### Extract reviewReducer (already exported, add tests)

```pseudocode
1. Create hooks/lib/reviewFlowHelpers.test.ts
2. Import reviewReducer from '../use-review-flow'
3. Define initialState constant
4. Test each action type:
   - LOAD_START: sets loading, clears transitioning
   - LOAD_EMPTY: clears all state, sets empty
   - LOAD_TIMEOUT: sets error with message
   - QUESTION_RECEIVED: populates from payload
   - REVIEW_COMPLETE: releases lock, marks transitioning
   - IGNORE_UPDATE: returns same state
5. Verify state transitions are correct
```

## Coverage Targets by Module

| Module | Current | Target | Strategy |
|--------|---------|--------|----------|
| convex/lib/conceptHelpers.ts | N/A | 100% | New pure module |
| hooks/lib/reviewFlowHelpers.ts | N/A | 100% | New pure module |
| lib/utils/date-format.ts | 0% | 100% | Add test file |
| lib/utils/shuffle.ts | 0% | 100% | Add test file |
| convex/fsrs/engine.ts | 97% | 100% | Edge cases |
| lib/smart-polling.ts | 96% | 100% | Edge case |
| convex/iqc.ts | 13% | 40%+ | Test more pure helpers |

## Error Handling Strategy

**Pure functions throw for invalid input**:
```typescript
export function clampPageSize(pageSize: number | null | undefined, defaults: Config): number {
  if (defaults.min > defaults.max) {
    throw new Error('Invalid config: min > max');
  }
  // ... rest of logic
}
```

**Tests verify error cases**:
```typescript
it('throws for invalid config', () => {
  expect(() => clampPageSize(50, { min: 100, max: 10, default: 50 }))
    .toThrow('Invalid config');
});
```

## Testing Strategy

**Unit Tests** (all new tests):
- Pure functions with various inputs
- Edge cases (null, undefined, empty)
- Boundary conditions
- Error cases

**No Integration Tests Added**:
- Mutations already have implicit integration tests via app usage
- Adding integration tests would require Convex test infrastructure
- Focus on pure function extraction provides better ROI

**Mocking Strategy**:
- Mock `Date.now()` via `vi.useFakeTimers()` for time-dependent tests
- No mocking of internal functions
- No mocking of database or Convex ctx

## Alternative Architectures Considered

### Alternative A: Backfill Tests for Existing Code

- **Pros**: No refactoring needed
- **Cons**: Tests brittle, code stays hard to test, maintenance burden
- **Verdict**: Rejected - TASK.md explicitly says no backfilling

### Alternative B: Full Module Extraction

- **Pros**: Complete separation of concerns
- **Cons**: Over-engineering for coverage goal, breaks existing imports
- **Verdict**: Rejected - Too much change for 70% target

### Alternative C: Test Mutations with Mocked Convex ctx

- **Pros**: Tests actual mutation handlers
- **Cons**: Complex setup, flaky, requires Convex test infra
- **Verdict**: Rejected - Pure extraction is simpler

**Selected**: Pure Function Extraction (current approach)
- Minimal refactoring
- Easy, fast, deterministic tests
- Improves code design as side effect
- Matches TDD philosophy

## Success Criteria

1. ✅ Coverage ≥70% lines (from 43.8%)
2. ✅ Thresholds enforced in CI (ratchet pattern)
3. ✅ PR comments show coverage delta (already working)
4. ✅ No new flaky tests (pure functions only)
5. ✅ God objects refactored into testable pure functions

## What's NOT in Scope

- Testing hooks with useEffect/timers (flaky)
- Testing mutations directly (need Convex infra)
- Badge generation (maintenance burden)
- E2E coverage expansion
- Analytics test expansion (already flaky)
