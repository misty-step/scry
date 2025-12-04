# Skip Button for Review Flow

## Executive Summary

**Problem**: Learners occasionally hit cognitive fatigue mid-review and need to defer a specific concept without permanently archiving it.

**Solution**: "Skip for Now" button moves concept to end of session queue. No algorithm modification, no persistence—pure client-side state.

**User Value**: Provides breathing room without creating escape hatches. Natural consequences preserved: skipped cards return in same session.

**Success Criteria**: Feature ships with zero FSRS modification; skipped concepts reappear after exhausting queue.

---

## User Context

**Who**: Learners mid-session who encounter a concept they're not mentally prepared for right now.

**Problem Solved**: "I need a break from THIS card" → Skip moves to back of queue, returns shortly.

**What Skip is NOT**:
- Not a way to defer until tomorrow (that's quitting the session)
- Not a way to avoid consequences (cards return same session)
- Not a comfort feature (just reorders, doesn't reduce workload)

---

## Requirements

### Functional

**FR-1: Skip (Back of Queue)**
- User can skip current concept to end of session queue
- Skipped concept remains due today
- Re-appears after other due concepts exhausted
- No limit on skips (user can skip everything—they'll cycle back)
- No FSRS state modification
- No persistence (session-scoped, clears on refresh)

**FR-2: UI Placement**
- Action in ReviewActionsDropdown (burger menu)
- Icon: RotateCcw (suggests "I'll see this again")
- Label: "Skip for Now"
- Positioned above archive actions (less destructive = higher)
- Keyboard shortcut: `s` (Phase 2)

**FR-3: Feedback**
- Brief toast: "Skipped. You'll see this again shortly."
- Screen reader announcement via LiveRegion

### Non-Functional

**NFR-1: Performance**
- Skip operation: <50ms (client-side state only)
- getDue query: unchanged (skip filtering is client-side)

**NFR-2: Philosophy Compliance**
- Zero modification to FSRS algorithm
- Natural consequences preserved (skipped cards return same session)
- No daily limits, no artificial comfort

---

## Architecture

### Selected Approach: Pure Client State

**Mechanism**: Client-side `Set<Id<'concepts'>>` tracking skipped IDs this session. When getDue returns a concept in the skip set, immediately fetch next. When queue exhausts to only skipped concepts, clear skip set and cycle through them.

**Why this is elegant**:
- Zero schema change
- Zero backend modification
- Zero persistence complexity
- Session-scoped is correct behavior (refresh = intentional reset)

### Module Boundaries

**hooks/use-review-flow.ts**:
- Add `skippedConceptIds: Set<Id<'concepts'>>` to state
- Add `skipConcept(conceptId)` handler
- Modify fetch logic to skip concepts in set
- Clear set when all remaining concepts are skipped

**components/review/review-actions-dropdown.tsx**:
- Add "Skip for Now" menu item
- New prop: `onSkip`

---

## Implementation

### Phase 1: MVP
1. Add skip state to use-review-flow hook
2. Add skip handler with client-side filtering
3. Add menu item to ReviewActionsDropdown
4. Add toast feedback + screen reader announcement
5. Tests for skip behavior

### Phase 2: Polish
- Keyboard shortcut (`s` key, refactor existing conflict)
- "X cards skipped" indicator (optional)
- Analytics event for skip actions

---

## Test Scenarios

### Skip Behavior
- [ ] Skip moves concept to end of due queue
- [ ] Skipped concept reappears after exhausting other due concepts
- [ ] Multiple skips accumulate correctly
- [ ] Skip state clears on page refresh
- [ ] Skip all concepts → cycles through skip queue
- [ ] Skip works when only 1 concept due (shows same concept again immediately)

### Edge Cases
- [ ] Skip + answer another → skipped concepts still in queue
- [ ] getDue returns null (no due concepts) → skip button disabled/hidden
- [ ] Archived concept cannot be skipped (already excluded from getDue)

### Accessibility
- [ ] Screen reader announces "Concept skipped. Will reappear shortly."
- [ ] Menu item keyboard accessible (Tab + Enter)

---

## Key Decisions

| Decision | Alternatives | Rationale |
|----------|--------------|-----------|
| Skip only (no "Not Today") | Two-tier postponement | Jobs: "Not Today" is comfort feature that violates Pure FSRS Philosophy |
| Client-side state only | Persist to DB | Session-scoped is correct; refresh = intentional reset |
| Menu placement (not quick button) | Visible button alongside Next | Friction by design; shouldn't be easiest action |
| No limits on usage | Max 5 skips | Pure FSRS: natural consequences (cycling through skips) |
| RotateCcw icon | SkipForward | Suggests "comes back around" vs "move past" |
| "Skip for Now" label | "Skip" / "Later" | Explicit temporality sets correct expectation |

---

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Users confused when skipped cards return | Low | Low | Toast explains "will reappear shortly" |
| Skip state lost mid-session | Low | Low | Acceptable—refresh is intentional reset |
| Users skip everything repeatedly | Low | Low | Natural consequence: infinite loop until they engage |

---

## UX Polish Checklist

From UX review, include in Phase 1:

- [x] Error handling: Skip is client-side only, no failure modes
- [x] Loading state: Instant (<50ms), no loading needed
- [ ] Screen reader: Add LiveRegion announcement
- [ ] Keyboard: Phase 2 (resolve `s` key conflict first)
- [ ] Toast feedback: "Skipped. You'll see this again shortly."

---

## Files to Modify

| File | Change |
|------|--------|
| `hooks/use-review-flow.ts` | Add skip state + handler |
| `components/review/review-actions-dropdown.tsx` | Add Skip menu item |
| `components/review/review-actions-dropdown.test.tsx` | Test skip rendering |
| `tests/review-flow.test.ts` | Test skip behavior |
