# DESIGN.md - Skip Button Architecture

## Architecture Overview

**Selected Approach**: Pure Client-Side State in `useReviewFlow`

**Rationale**: Skip is session-scoped, non-persistent, and tightly coupled with "get next question" logic. Integrating into the existing `useReviewFlow` reducer keeps the feature co-located with related state transitions while avoiding backend changes.

**Core Modules**:
- `hooks/use-review-flow.ts`: Skip state + filtering logic (primary change)
- `components/review/review-actions-dropdown.tsx`: UI entry point (minor change)
- `components/review-flow.tsx`: Wire skip handler, toast feedback (minor change)

**Data Flow**:
```
User clicks "Skip for Now"
  → onSkip() callback fires
  → dispatch({ type: 'SKIP_CONCEPT', payload: conceptId })
  → skippedConceptIds.add(conceptId)
  → REVIEW_COMPLETE dispatched (triggers next question fetch)
  → Poll returns same/different concept
  → If in skippedConceptIds AND !allSkipped → auto-skip, try next
  → If allSkipped → clear skippedConceptIds, show cycled concepts
```

**Key Design Decisions**:
1. **Reducer-based state**: Aligns with existing pattern; skippedConceptIds as `Set<Id<'concepts'>>` in state
2. **Client-side filtering**: Skip check happens when poll data arrives, not in query
3. **Session-scoped**: No persistence—refresh = reset (intentional behavior per PRD)
4. **No limits**: Natural consequences (cycling) teach sustainable habits

---

## Module: useReviewFlow Hook

**Responsibility**: Manages review session state including skip queue. Hides complexity of polling, deduplication, and skip cycling from consuming components.

**Public Interface** (additions):
```typescript
interface UseReviewFlowReturn {
  // ... existing fields
  skippedCount: number;                    // Count of currently skipped concepts
  handlers: {
    onReviewComplete: () => Promise<void>;
    onSkipConcept: () => void;             // NEW: Skip current concept to end of queue
  };
}
```

**Internal State Changes**:
```typescript
interface ReviewModeState {
  // ... existing fields
  skippedConceptIds: Set<Id<'concepts'>>;  // NEW: Session-scoped skip set
}

type ReviewAction =
  // ... existing actions
  | { type: 'SKIP_CONCEPT'; payload: Id<'concepts'> }
  | { type: 'CLEAR_SKIPPED' }
  | { type: 'AUTO_SKIP'; payload: { conceptId: Id<'concepts'>; reason: string } };
```

**Error Handling**:
- Skip with no current concept: No-op (guard in handler)
- Skip when transitioning: No-op (guard in handler)

---

## Module: ReviewActionsDropdown

**Responsibility**: Renders action menu with skip option. No business logic—purely presentational.

**Public Interface** (additions):
```typescript
interface ReviewActionsDropdownProps {
  // ... existing props
  onSkip: () => void;                      // NEW: Skip handler
  canSkip?: boolean;                       // NEW: Disable during transition (default true)
}
```

**UI Placement** (per PRD):
```
┌─────────────────────┐
│ 📝 Edit             │ ← Least destructive
├─────────────────────┤
│ 🔄 Skip for Now     │ ← NEW: After edit, before archive
├─────────────────────┤
│ 📁 Archive Phrasing │
│ 📁 Archive Concept  │ ← Most destructive
└─────────────────────┘
```

**Icon**: `RotateCcw` from lucide-react (suggests "comes back around")

---

## Module: ReviewFlow Component

**Responsibility**: Wire skip handler from hook to dropdown, show toast + a11y feedback.

**Changes**:
1. Destructure `skippedCount` and `handlers.onSkipConcept` from `useReviewFlow()`
2. Pass `onSkip={handleSkip}` to `ReviewActionsDropdown`
3. Add toast on skip: `"Skipped. You'll see this again shortly."`
4. Announce via LiveRegion: `"Concept skipped. Will reappear shortly."`

---

## Core Algorithm: Skip Filtering

```pseudocode
// In useReviewFlow effect that processes nextReview poll data

function processNextReview(nextReview, state):
  // Guard: If user has active lock, ignore update
  if state.lockId:
    return IGNORE_UPDATE

  // Guard: No concepts due
  if nextReview === null:
    return LOAD_EMPTY

  conceptId = nextReview.concept._id

  // Check if this concept is in skip set
  if state.skippedConceptIds.has(conceptId):
    // Check if ALL due concepts are skipped
    // We can't know total due count from client, so we track auto-skips
    // If we auto-skip the same concept twice in a row, queue is exhausted

    if state.lastAutoSkippedId === conceptId:
      // Queue exhausted to only skipped concepts → clear and show
      return CLEAR_SKIPPED  // clears set, allows concept to display
    else:
      // More concepts may exist → auto-skip this one
      dispatch AUTO_SKIP  // adds to lastAutoSkippedId tracking
      dispatch REVIEW_COMPLETE  // triggers next poll
      return  // Don't display this concept

  // Not skipped → normal flow
  // Clear lastAutoSkippedId since we found a non-skipped concept
  return QUESTION_RECEIVED with payload
```

**Why this works**:
- We don't know the full due queue from client (backend returns one at a time)
- By tracking consecutive auto-skips of same concept, we detect queue exhaustion
- When exhausted, we clear skip set—user sees their skipped concepts cyclically

**Alternative considered**: Query backend for count of due concepts not in skip set. Rejected: requires backend change, adds latency, violates "pure client-side" constraint.

---

## Implementation Pseudocode

### 1. Reducer Changes (use-review-flow.ts)

```pseudocode
// New initial state
const initialState = {
  ...existing,
  skippedConceptIds: new Set(),
  lastAutoSkippedId: null,
}

// New actions in reducer
case 'SKIP_CONCEPT':
  // Add concept to skip set, release lock to fetch next
  return {
    ...state,
    skippedConceptIds: new Set([...state.skippedConceptIds, action.payload]),
    lockId: null,
    isTransitioning: true,
    lastAutoSkippedId: null,  // Reset auto-skip tracker
  }

case 'AUTO_SKIP':
  // Track auto-skipped concept for queue exhaustion detection
  return {
    ...state,
    lastAutoSkippedId: action.payload.conceptId,
    lockId: null,
    isTransitioning: true,
  }

case 'CLEAR_SKIPPED':
  // Queue exhausted, clear skip set to cycle through
  return {
    ...state,
    skippedConceptIds: new Set(),
    lastAutoSkippedId: null,
  }

case 'QUESTION_RECEIVED':
  // Reset auto-skip tracker when displaying non-skipped concept
  return {
    ...existingLogic,
    lastAutoSkippedId: null,
  }
```

### 2. Effect Changes (use-review-flow.ts)

```pseudocode
useEffect processing nextReview:
  // ... existing guards (dataHasChanged, lockId, undefined check)

  if nextReview === null:
    dispatch LOAD_EMPTY
    return

  const conceptId = nextReview.concept._id

  // NEW: Skip filtering
  if (state.skippedConceptIds.has(conceptId)):
    if (state.lastAutoSkippedId === conceptId):
      // Queue exhausted → clear skips
      dispatch({ type: 'CLEAR_SKIPPED' })
      // Don't return - fall through to display this concept
    else:
      // Auto-skip this concept
      dispatch({ type: 'AUTO_SKIP', payload: { conceptId, reason: 'in_skip_set' } })
      // Trigger next fetch without displaying
      return

  // ... existing QUESTION_RECEIVED dispatch
```

### 3. Skip Handler (use-review-flow.ts)

```pseudocode
const handleSkipConcept = useCallback(() => {
  // Guards
  if (!state.conceptId) return
  if (state.isTransitioning) return

  dispatch({ type: 'SKIP_CONCEPT', payload: state.conceptId })
}, [state.conceptId, state.isTransitioning])
```

### 4. Dropdown UI (review-actions-dropdown.tsx)

```pseudocode
// After Edit menu item, before separator
<DropdownMenuItem
  onClick={onSkip}
  className="gap-2"
  disabled={!canSkip}
>
  <RotateCcw className="h-4 w-4" />
  Skip for Now
</DropdownMenuItem>
<DropdownMenuSeparator />
```

### 5. Component Integration (review-flow.tsx)

```pseudocode
const { skippedCount, handlers } = useReviewFlow()

const handleSkip = useCallback(() => {
  handlers.onSkipConcept()

  // Toast feedback
  toast('Skipped. You\'ll see this again shortly.', {
    duration: 2000,
  })
}, [handlers])

// In LiveRegion (add skip announcement state)
const [skipAnnouncement, setSkipAnnouncement] = useState('')
// After handleSkip:
setSkipAnnouncement('Concept skipped. Will reappear shortly.')
setTimeout(() => setSkipAnnouncement(''), 5000)

// Pass to dropdown
<ReviewActionsDropdown
  ...existing
  onSkip={handleSkip}
  canSkip={!isTransitioning}
/>
```

---

## File Organization

```
hooks/
  use-review-flow.ts              # Add skip state + SKIP_CONCEPT action + effect filter

components/
  review/
    review-actions-dropdown.tsx   # Add Skip menu item
    review-actions-dropdown.test.tsx  # Test skip item renders + callback

  review-flow.tsx                 # Wire handler, toast, LiveRegion

tests/
  hooks/
    use-review-flow.test.ts       # NEW: Unit tests for skip behavior
```

**New Files**: `tests/hooks/use-review-flow.test.ts`
**Modified Files**: 3 (hook, dropdown component, flow component)

---

## Testing Strategy

### Unit Tests: useReviewFlow (new file)

**Test boundaries**: Public API (returned state, handlers)
**Coverage target**: 90% (critical state machine)
**Mocking**: Mock `useSimplePoll` return values

```pseudocode
describe('useReviewFlow skip behavior')

  it('skips concept and fetches next when onSkipConcept called')
    // Setup: render hook with mock poll returning conceptA
    // Act: call handlers.onSkipConcept()
    // Assert: skippedConceptIds contains conceptA, isTransitioning true

  it('auto-skips when poll returns already-skipped concept')
    // Setup: skippedConceptIds = {conceptA}
    // Act: mock poll returns conceptA
    // Assert: another poll triggered (REVIEW_COMPLETE dispatched)

  it('clears skip set when queue exhausted to only skipped concepts')
    // Setup: skippedConceptIds = {conceptA}, lastAutoSkippedId = conceptA
    // Act: mock poll returns conceptA again
    // Assert: skippedConceptIds cleared, concept displays

  it('maintains skip set across multiple concepts')
    // Setup: skip conceptA, answer conceptB, skip conceptC
    // Assert: skippedConceptIds = {conceptA, conceptC}

  it('clears skip set on session end (empty phase)')
    // Setup: skip conceptA
    // Act: mock poll returns null
    // Assert: phase = 'empty', skippedConceptIds cleared
```

### Unit Tests: ReviewActionsDropdown (extend existing)

```pseudocode
it('renders Skip for Now menu item')
  // Assert: menu contains "Skip for Now" with RotateCcw icon

it('triggers onSkip callback when clicked')
  // Act: click "Skip for Now"
  // Assert: onSkip called once

it('disables skip when canSkip=false')
  // Render with canSkip={false}
  // Assert: menu item has aria-disabled
```

### Integration Test Scenario

```pseudocode
describe('skip flow integration')

  it('skip → answer others → skipped concepts return')
    // Given: 3 concepts due (A, B, C)
    // When: skip A, answer B correctly, answer C correctly
    // Then: A reappears
    // When: answer A
    // Then: empty state (all concepts answered)
```

---

## State Diagram

```
                    ┌─────────────────────────────────────────┐
                    │              reviewing                   │
                    │  ┌──────────────────────────────────┐   │
                    │  │ skippedConceptIds: Set<Id>       │   │
   SKIP_CONCEPT     │  │ lastAutoSkippedId: Id | null     │   │
 ┌─────────────────►│  └──────────────────────────────────┘   │
 │                  │                                          │
 │                  │   poll returns concept                   │
 │                  │           │                              │
 │                  │           ▼                              │
 │                  │   ┌───────────────┐                      │
 │                  │   │ in skip set?  │                      │
 │                  │   └───────────────┘                      │
 │                  │     │yes      │no                        │
 │                  │     ▼         ▼                          │
 │                  │ ┌─────────┐ ┌─────────────────┐         │
 │                  │ │ last == │ │ QUESTION_RECEIVED│         │
 │                  │ │ current?│ │ (display concept)│         │
 │                  │ └─────────┘ └─────────────────┘         │
 │                  │   │yes  │no                              │
 │                  │   ▼     ▼                                │
 │                  │ ┌───────┐ ┌─────────────┐                │
 │                  │ │CLEAR_ │ │ AUTO_SKIP   │                │
 │                  │ │SKIPPED│ │ (try next)  │                │
 │                  │ └───────┘ └─────────────┘                │
 │                  │                                          │
 └──────────────────┴──────────────────────────────────────────┘
```

---

## Performance Considerations

**Target**: <50ms skip operation (per NFR-1)

**Why this is achievable**:
- Skip is pure client-side state mutation (Set.add)
- No network call required
- Poll already running on interval; skip just releases lock
- Toast is async UI, doesn't block

**No performance concerns**:
- Set operations are O(1)
- Skip set size bounded by session due count (typically <100)
- No additional queries introduced

---

## Security Considerations

**Threat model**: N/A for this feature
- No new inputs from user (skip is boolean action)
- No data leaves client (skip state is session-scoped)
- No new queries or mutations

---

## Accessibility

**Screen reader support**:
- LiveRegion announcement: "Concept skipped. Will reappear shortly."
- Menu item keyboard accessible (inherits from DropdownMenuItem)

**Keyboard shortcut**: Phase 2 (`s` key—requires resolving existing conflict)

---

## Alternative Architectures Considered

### Alternative A: Skip state in ReviewFlow component
- **Pros**: Simpler hook, no reducer changes
- **Cons**: Skip filtering logic in component (wrong layer), harder to test
- **Verdict**: Rejected—mixes presentation with business logic

### Alternative B: Separate useSkipQueue hook
- **Pros**: Isolated, testable
- **Cons**: Adds indirection; skip is tightly coupled with "next question" logic anyway
- **Verdict**: Rejected—unnecessary abstraction for simple feature

### Alternative C: Backend-persisted skip set
- **Pros**: Survives refresh
- **Cons**: Violates "session-scoped" requirement, adds complexity, needs schema change
- **Verdict**: Rejected—refresh = intentional reset per PRD

**Selected**: Integrate into useReviewFlow (Option A with reducer pattern)
- **Justification**: Skip is state machine behavior (skip → transition → next question). Co-locating with existing state machine is simplest and most maintainable.

---

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Auto-skip loop on single-concept queue | Low | Low | lastAutoSkippedId detection clears immediately |
| Skip set grows unbounded | Very Low | Low | Bounded by due count; cleared on empty phase |
| Race condition: skip during transition | Low | Low | Guard: `if (isTransitioning) return` |

---

## Summary

This architecture delivers the "Skip for Now" feature with:
- **Zero backend changes** (pure client-side state)
- **Minimal code changes** (~100 lines across 3 files)
- **Full test coverage** strategy
- **Preserved FSRS philosophy** (natural consequences via cycling)

**Implementation sequence**:
1. Add reducer actions + state to `use-review-flow.ts`
2. Add skip filtering in poll processing effect
3. Add `onSkipConcept` handler export
4. Add "Skip for Now" menu item to dropdown
5. Wire handler + toast in `review-flow.tsx`
6. Add LiveRegion skip announcement
7. Write tests

**Next**: Run `/plan` to convert this architecture into atomic implementation tasks.
