# In-Review Editing & Archiving

## Executive Summary

Enable learners to edit and archive concepts and phrasings directly from the review screen without breaking flow. All destructive actions are soft deletes (archive), allowing casual confirmations with undo toasts. Inline editing optimized for mobile provides immediate feedback while preserving FSRS scheduling state.

**User Value**: Learners can fix typos, improve question clarity, and manage their deck without leaving review mode—eliminating context switches and maintaining study momentum.

**Success Criteria**:
- Zero friction for typo fixes (<3 taps on mobile)
- 100% reversible operations (8-second undo window)
- FSRS schedules preserved (no accidental resets)
- Mobile-first UX (44px touch targets, inline editing)

---

## User Context

**Who**: Active learners in the middle of review sessions who encounter:
- Typos or unclear wording in questions
- Poorly phrased alternatives they want to remove
- Concepts that need refinement mid-study

**Problems Being Solved**:
1. **Context switching friction**: Currently must exit review → navigate to library → find concept → edit → return to review (5+ clicks, lost position)
2. **Momentum loss**: Breaking review flow disrupts spaced repetition effectiveness
3. **Mobile limitations**: No way to quickly fix errors on mobile during commute/downtime study
4. **Irreversible fear**: Users avoid cleaning up deck because delete feels permanent

**Measurable Benefits**:
- Time saved: 90% reduction in edit workflow (15 clicks → 2 taps)
- Completion rate: Higher review session completion (no mid-session exits)
- Deck quality: More frequent corrections = better learning materials
- User confidence: Reversible actions reduce decision anxiety

---

## Requirements

### Functional Requirements

**Edit Actions**:
1. **Edit Phrasing**: Modify question text, correct answer, explanation, answer options
2. **Edit Concept**: Modify title and description
3. Both preserve FSRS scheduling state (with educational tooltip)

**Archive Actions**:
1. **Archive Phrasing**: Soft delete single phrasing (reversible via unarchive)
2. **Archive Concept**: Soft delete concept + all phrasings atomically (reversible)
3. All show 8-second undo toast

**UI/UX Requirements**:
1. **Dropdown Menu**: Single ⋮ icon houses all actions (Edit Phrasing, Edit Concept, Archive Phrasing, Archive Concept)
2. **Inline Editing**: Click/tap field → becomes editable → save on blur or explicit save button
3. **Mobile-First**: 44px minimum touch targets, large tap areas, easy keyboard access
4. **Keyboard Shortcuts**: `E` for edit, `#` or `Backspace` for archive
5. **Post-Edit Behavior**: Stay on edited card for final review of changes

**Educational Requirements**:
1. Tooltip/info icon explaining FSRS preservation on edit
2. Guidance: "For substantial changes, archive and create new concept instead"
3. Clear undo toast messaging: "Phrasing archived — Undo"

### Non-Functional Requirements

**Performance**:
- Inline edit activation: <100ms
- Save operation: <300ms optimistic update
- Mobile tap response: <50ms visual feedback

**Accessibility**:
- Keyboard navigation for all actions
- Screen reader announcements for state changes
- Focus management (return to trigger after modal close)

**Mobile UX**:
- Touch targets ≥44px
- Inline editing works with mobile keyboards
- Dropdown menu accessible with one thumb
- Undo toast reachable without thumb gymnastics

**Data Integrity**:
- Atomic archive (concept + all phrasings)
- Validation before save (non-empty fields, valid answer options)
- Optimistic updates with rollback on error
- Concurrent edit handling via Convex reactivity

---

## Architecture Decision

### Selected Approach: Inline Editing + Dropdown Actions

**Rationale**:
1. **Simplicity**: No modals = fewer components, faster interactions
2. **User Value**: Inline editing keeps context visible (60% faster than modal workflow)
3. **Explicitness**: Dropdown menu clearly shows available actions without clutter
4. **Mobile-First**: Inline editing works naturally with mobile keyboards

### Module Design

#### Backend Mutations (Convex)

**New Mutations Required**:

```typescript
// convex/concepts.ts
export const updateConcept = mutation({
  args: {
    conceptId: v.id('concepts'),
    title: v.string(),
    description: v.optional(v.string()),
  },
  handler: async (ctx, args) => {
    // Validate ownership
    // Update concept (preserve fsrs)
    // Set updatedAt timestamp
  },
});

export const updatePhrasing = mutation({
  args: {
    phrasingId: v.id('phrasings'),
    question: v.string(),
    correctAnswer: v.string(),
    explanation: v.optional(v.string()),
    options: v.optional(v.array(v.string())),
  },
  handler: async (ctx, args) => {
    // Validate ownership
    // Validate correctAnswer in options (if MC/TF)
    // Update phrasing
    // Set updatedAt timestamp
  },
});

export const unarchivePhrasing = mutation({
  args: { phrasingId: v.id('phrasings') },
  handler: async (ctx, args) => {
    // Clear archivedAt
    // Increment concept.phrasingCount
    // Recalculate concept scores
    // Update userStats
  },
});
```

**Existing Mutations to Use**:
- `archivePhrasing` (line 678)
- `archiveConcept` (line 840)
- `unarchiveConcept` (line 858)

#### Frontend Components

**New Component**: `ReviewActionsDropdown`
```tsx
// components/review/review-actions-dropdown.tsx
<DropdownMenu>
  <DropdownMenuTrigger>⋮</DropdownMenuTrigger>
  <DropdownMenuContent>
    <DropdownMenuItem onClick={onEditPhrasing}>
      Edit Question
    </DropdownMenuItem>
    <DropdownMenuItem onClick={onEditConcept}>
      Edit Concept
    </DropdownMenuItem>
    <DropdownMenuSeparator />
    <DropdownMenuItem onClick={onArchivePhrasing}>
      Archive Question
    </DropdownMenuItem>
    <DropdownMenuItem onClick={onArchiveConcept}>
      Archive Concept
    </DropdownMenuItem>
  </DropdownMenuContent>
</DropdownMenu>
```

**Modified Component**: `review-flow.tsx`
- Replace legacy question edit/delete buttons with new dropdown
- Add inline editing state management
- Integrate undo toast for archive actions

**New Hook**: `useInlineEdit`
```typescript
// hooks/use-inline-edit.ts
export function useInlineEdit<T extends Record<string, any>>(
  initialData: T,
  onSave: (data: T) => Promise<void>
) {
  const [isEditing, setIsEditing] = useState(false);
  const [data, setData] = useState(initialData);
  const [isSaving, setIsSaving] = useState(false);

  const handleSave = async () => {
    setIsSaving(true);
    try {
      await onSave(data);
      setIsEditing(false);
    } catch (error) {
      // Rollback + show error
      setData(initialData);
      toast.error('Failed to save changes');
    } finally {
      setIsSaving(false);
    }
  };

  return { isEditing, data, setData, handleSave, startEditing, cancel };
}
```

### Alternatives Considered

| Approach | Value | Simplicity | Risk | Why Not Chosen |
|----------|-------|------------|------|----------------|
| **Modal Editing** | 7/10 | 6/10 | Low | Breaks review flow, slower on mobile |
| **Inline + Modal Hybrid** | 8/10 | 4/10 | Medium | Over-engineered for use case, confusing UX |
| **Inline Only (Selected)** | 9/10 | 9/10 | Low | Simple, fast, mobile-optimized |

### Deep Module Design

**`updatePhrasing` Mutation**:
- **Interface**: `{ phrasingId, question, correctAnswer, explanation?, options? }`
- **Hidden Complexity**: Validation, ownership check, timestamp update, option validation
- **Boundary**: Callers don't know about DB structure, just provide new values

**`ReviewActionsDropdown` Component**:
- **Interface**: `{ conceptId, phrasingId, onActionComplete }`
- **Hidden Complexity**: Mutation calls, undo toast, error handling, keyboard shortcuts
- **Boundary**: Parent just provides IDs and callback, dropdown handles all logic

---

## Test Scenarios

### Happy Path

1. **Edit phrasing during review**
   - User answers question → sees feedback
   - Clicks ⋮ → "Edit Question" → fields become editable
   - Changes "What is capital of France?" → "What's the capital of France?"
   - Saves → sees edited version immediately
   - FSRS schedule preserved (verify nextReview unchanged)

2. **Edit concept title**
   - Clicks ⋮ → "Edit Concept"
   - Changes "French Geography" → "France: Geography Basics"
   - Saves → title updates in review UI
   - Phrasing content unchanged

3. **Archive phrasing (multiple exist)**
   - Concept has 3 phrasings
   - Clicks ⋮ → "Archive Question"
   - Phrasing removed immediately
   - Undo toast shows "Question archived — Undo" (8 sec)
   - Next phrasing from same concept appears

4. **Archive concept**
   - Clicks ⋮ → "Archive Concept"
   - Concept + all phrasings removed from queue
   - Undo toast shows "Concept archived — Undo"
   - Next due concept appears
   - UserStats updated (totalCards -1)

5. **Undo archive**
   - Archive phrasing → click "Undo" in toast
   - Phrasing restored to exact state
   - UserStats restored

### Edge Cases

6. **Archive last phrasing**
   - Concept has 1 phrasing
   - Attempt to archive → either:
     - Suggest archiving whole concept instead
     - Or allow (concept becomes 0-phrasing concept, hidden from review)
   - Decision needed: Block or allow?

7. **Edit validation failure**
   - Edit question → leave blank → try to save
   - Error: "Question cannot be empty"
   - Field stays in edit mode
   - Changes not saved

8. **Multiple-choice validation**
   - Edit MC question
   - Change correct answer to "Paris"
   - Correct answer NOT in options array
   - Error: "Correct answer must be one of the options"

9. **Concurrent edit from another device**
   - User editing on mobile
   - Background: concept archived from desktop
   - Convex reactivity: concept disappears
   - Toast: "This item was removed from your queue"
   - Load next card

10. **Network error during save**
    - Edit phrasing → save
    - Network fails
    - Optimistic update reverts
    - Error toast: "Failed to save — try again"
    - Fields stay in edit mode

11. **Undo after toast dismissed**
    - Archive phrasing
    - Wait 9 seconds (toast dismissed)
    - Undo not available
    - Must unarchive from library

12. **Edit immediately after answering wrong**
    - Answer incorrectly → FSRS updates to "relearning"
    - Edit question → save
    - Verify FSRS state unchanged (still "relearning")
    - Tooltip visible explaining preservation

### Mobile-Specific

13. **Touch editing on mobile**
    - Tap ⋮ dropdown (44px target)
    - Tap "Edit Question"
    - Mobile keyboard appears
    - Edit text, tap outside → saves
    - Keyboard dismisses

14. **Thumb-reachable undo**
    - Archive on mobile
    - Undo toast appears at bottom
    - Tap "Undo" with thumb (44px target)
    - Restoration succeeds

15. **Long-press menu (future)**
    - Long-press on card → dropdown appears
    - Common mobile pattern

### Keyboard Shortcuts

16. **E key for edit**
    - Press `E` → edit mode activates
    - Focus on first editable field
    - Edit + press `Esc` → saves and exits

17. **# for archive**
    - Press `#` → archive dropdown opens
    - Or directly archives (TBD)

### FSRS Preservation

18. **Verify schedule unchanged after edit**
    - Note original `nextReview`, `stability`, `difficulty`
    - Edit question → save
    - Query concept → verify FSRS fields identical

19. **Info tooltip about FSRS**
    - Hover/tap info icon during edit
    - Tooltip: "Edits preserve your learning progress. For major changes, archive and create new concept."
    - Tooltip dismissible

### Data Integrity

20. **Atomic concept archive**
    - Concept has 5 phrasings
    - Archive concept
    - Verify ALL phrasings have `archivedAt` set
    - Verify userStats decremented by 1 (not 5)

21. **Archive then unarchive idempotency**
    - Archive concept
    - Immediately undo
    - Verify concept fully restored
    - Verify FSRS state unchanged
    - Verify userStats unchanged

---

## Dependencies & Assumptions

### Dependencies on Existing Systems

**Backend**:
- Convex mutations framework
- Existing validation helpers (`validateBulkOwnership`)
- User stats update patterns
- FSRS scheduling engine (preserve state)

**Frontend**:
- `useConfirmation` hook (not needed for casual archives, but available)
- `useUndoableAction` hook for undo toasts
- Shadcn/ui DropdownMenu component
- Existing review flow architecture

**Integrations**:
- None (fully internal feature)

### Assumptions

**Scale**:
- Users have 10-1000 concepts in review rotation
- Edit operations are infrequent (1-5% of reviews)
- Archive operations are rare (1% of reviews)

**Environment**:
- Mobile users on modern browsers (iOS Safari, Chrome Android)
- Touch screens support 44px targets
- Network latency <500ms (optimistic updates handle slow connections)

**User Behavior**:
- Users understand archive is reversible (education via tooltips)
- Users rarely need to batch edit (one-at-a-time is acceptable)
- Users trust undo toast (8 seconds sufficient)

**Constraints**:
- Must preserve FSRS state (hard requirement)
- Cannot create new phrasings from review (defer to library)
- Cannot change question type (MC → TF) (acceptable limitation)

---

## Implementation Phases

### Phase 1: Backend Mutations (4-6 hours)

**Goal**: Implement all required mutations following mutation pair pattern

**Tasks**:
1. Create `concepts.updateConcept` mutation
   - Args validation (title non-empty, max length)
   - Ownership check
   - Preserve `fsrs`, `phrasingCount`, all FSRS fields
   - Set `updatedAt` timestamp
   - Test: unit test for validation, ownership, FSRS preservation

2. Create `concepts.updatePhrasing` mutation
   - Args validation (question/answer non-empty)
   - MC/TF: validate correctAnswer in options
   - Ownership check
   - Preserve phrasing stats (`attemptCount`, `lastAttemptedAt`)
   - Set `updatedAt` timestamp
   - Test: unit test for validation, MC validation

3. Create `concepts.unarchivePhrasing` mutation (complete the pair)
   - Clear `archivedAt` timestamp
   - Increment `concept.phrasingCount`
   - Recalculate `thinScore`, `conflictScore`
   - Update userStats (reverse archive operation)
   - Test: integration test with archive/unarchive cycle

**Acceptance**:
- All 3 mutations pass unit tests
- `npx convex dev` shows "Convex functions ready!"
- No type errors
- Mutation pair pattern complete

### Phase 2: Frontend Inline Editing (6-8 hours)

**Goal**: Build inline editing UI with optimistic updates

**Tasks**:
1. Create `useInlineEdit` hook
   - State management (editing mode, temp data, saving state)
   - Optimistic updates
   - Error handling with rollback
   - Undo support
   - Test: Vitest unit tests for state transitions

2. Add inline editing to `review-flow.tsx`
   - Edit state for concept title, description
   - Edit state for phrasing question, answer, explanation, options
   - Save on blur or explicit save button
   - Visual indicators (blue border, save/cancel buttons)
   - Loading states during save
   - Test: Component test for edit mode activation

3. Keyboard shortcuts
   - `E` activates edit mode
   - `Esc` saves and exits
   - `Ctrl/Cmd+Enter` explicit save
   - Focus management
   - Test: E2E test for keyboard workflow

**Acceptance**:
- Can edit all visible fields inline
- Optimistic updates show immediately
- Network errors rollback gracefully
- Mobile keyboard interactions smooth
- Tests pass in CI

### Phase 3: Archive Actions + Undo (4-6 hours)

**Goal**: Implement archive with undo toast

**Tasks**:
1. Integrate `useUndoableAction` hook
   - Archive phrasing → undo toast with `unarchivePhrasing`
   - Archive concept → undo toast with `unarchiveConcept`
   - 8-second duration
   - Error handling for undo failures
   - Test: Integration test for archive/undo cycle

2. Post-action navigation
   - Archive phrasing → load next phrasing or next concept
   - Archive concept → load next due concept
   - Handle empty queue gracefully
   - Test: Review flow continues correctly

3. Keyboard shortcut for archive
   - `#` or `Backspace` triggers archive dropdown
   - Or direct archive (TBD during implementation)
   - Test: E2E test for keyboard archive

**Acceptance**:
- Archive actions show undo toast
- Undo restores exact state
- Review flow continues smoothly
- Tests pass

### Phase 4: Dropdown Menu + Integration (4-6 hours)

**Goal**: Build unified dropdown menu, polish UX

**Tasks**:
1. Create `ReviewActionsDropdown` component
   - DropdownMenu with 4 actions
   - Keyboard navigation
   - Mobile-friendly touch targets (44px)
   - Icon positioning (top-right of card)
   - Test: Component test for all actions

2. Replace legacy edit/delete buttons in `review-flow.tsx`
   - Remove old legacy question edit/delete UI
   - Add new dropdown for concepts/phrasings
   - Conditional actions (hide "Archive Phrasing" if only 1 phrasing?)
   - Test: Visual regression test

3. Educational tooltips
   - Info icon next to edit actions
   - Tooltip: "Edits preserve learning progress"
   - Link to help doc with guidance on substantial changes
   - Test: Tooltip appears, dismisses correctly

**Acceptance**:
- Single dropdown houses all actions
- Mobile touch targets ≥44px
- Tooltips provide FSRS education
- Legacy edit/delete removed
- Tests pass

### Phase 5: Mobile UX Polish + Testing (3-4 hours)

**Goal**: Ensure mobile experience is top-tier

**Tasks**:
1. Mobile touch optimizations
   - Increase tap areas
   - Test on iOS Safari, Chrome Android
   - Keyboard handling (dismiss, focus, scroll)
   - Undo toast reachability

2. Playwright E2E tests
   - Full review → edit → save flow
   - Archive → undo flow
   - Mobile viewport tests
   - Keyboard shortcut tests

3. Edge case handling
   - Concurrent edits
   - Network errors
   - Validation failures
   - Empty queue after archive

**Acceptance**:
- Manual testing on real mobile devices passes
- E2E tests cover all happy paths + 5 edge cases
- No mobile-specific bugs
- Lighthouse accessibility score ≥95

---

## Risks & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| **Inline editing breaks on mobile keyboards** | Medium | High | Test on real iOS/Android devices early; add explicit save button as fallback |
| **Undo toast dismissed before user notices** | Medium | Medium | 8-second duration (research-backed); clear messaging; document undo from library |
| **Users confused archive is reversible** | Medium | Medium | Educational tooltip + help link; rename "Archive" to "Archive (Undo from Library)"? |
| **FSRS preservation not obvious** | High | Low | Prominent tooltip with info icon; user education in onboarding |
| **Validation errors frustrate users** | Low | Medium | Clear error messages, field-level validation, prevent invalid states |
| **Concurrent edits cause data loss** | Low | High | Convex reactivity shows latest state; warn if data changed; optimistic updates with rollback |
| **Archive last phrasing leaves 0-phrasing concept** | Medium | Low | Decide: block with message suggesting concept archive, or allow |
| **Mobile dropdown hard to reach** | Low | Medium | Position dropdown top-right; test thumb reachability on large phones |

---

## Key Decisions

### 1. Archive Unifies Delete Semantics

**Decision**: Remove all "delete" terminology; everything is "archive" (soft delete with reversibility).

**Rationale**:
- Reduces user anxiety (100% reversible)
- Simplifies UX (one action instead of two)
- Matches backend reality (all deletes are soft)
- Enables casual confirmations (undo toast vs confirmation dialog)

**Tradeoffs**:
- Users may not realize archived items can be restored → mitigate with tooltip
- No hard delete option for truly unwanted content → acceptable (storage cost negligible)

---

### 2. Inline Editing Over Modal

**Decision**: Edit in place with inline fields, no modal dialog.

**Rationale**:
- **User Value**: 60% faster than modal workflow (2 taps vs 5+ clicks)
- **Simplicity**: Fewer components, simpler state management
- **Mobile-First**: Works naturally with mobile keyboards
- **Explicitness**: Context stays visible during edit

**Tradeoffs**:
- Limited space for complex edits → acceptable (not editing full forms)
- Harder to show preview → mitigate with "view edited version" post-save
- Can't edit multiple phrasings at once → defer to library for batch operations

---

### 3. Preserve FSRS on Edit

**Decision**: Edits preserve scheduling state; educate users about when to archive instead.

**Rationale**:
- **Correctness**: Typo fixes shouldn't reset learning progress
- **User Value**: Maintains hard-earned FSRS history
- **Simplicity**: No complex "reset" logic needed
- **Explicitness**: Tooltip makes behavior clear

**Tradeoffs**:
- Users editing substantially different content don't get reset → mitigate with education (tooltip says "archive and recreate for major changes")
- No easy "start over" button → acceptable (users can archive then recreate manually)

---

### 4. 8-Second Undo Window

**Decision**: Undo toast visible for 8 seconds after archive.

**Rationale**:
- Research-backed duration (Gmail uses 5-10 seconds)
- Long enough to notice mistake
- Short enough to avoid screen clutter
- Matches existing `useUndoableAction` default (5 sec) → extend to 8 sec

**Tradeoffs**:
- Power users may miss undo → mitigate with "unarchive from library" documentation
- Slower users may need more time → acceptable (can unarchive from library)

---

### 5. Dropdown Menu Houses All Actions

**Decision**: Single ⋮ icon with dropdown containing all 4 actions.

**Rationale**:
- **Simplicity**: Clean UI, no clutter
- **Explicitness**: All actions visible in menu
- **Mobile-Friendly**: Large touch target, thumb-reachable
- **Discoverable**: Standard pattern (Gmail, Notion, etc.)

**Tradeoffs**:
- Requires extra tap vs always-visible buttons → acceptable (actions are infrequent)
- Keyboard shortcuts needed for power users → mitigate with `E`, `#` shortcuts

---

### 6. Post-Edit: Review Edited Version

**Decision**: After editing, stay on the edited card for final review.

**Rationale**:
- **User Value**: Verify changes before moving on
- **Safety**: Catch mistakes immediately
- **Workflow**: Natural "edit → review → next" flow

**Tradeoffs**:
- Slows review momentum slightly → acceptable (editing is infrequent)
- User must manually advance → mitigate with clear "Next" button or auto-advance after 3 seconds

---

### 7. No New Phrasing Creation in Review

**Decision**: Cannot add new phrasings from review screen; must use library.

**Rationale**:
- **Simplicity**: Avoids complex form in review context
- **Focus**: Keep review screen focused on reviewing
- **Explicitness**: Clear separation: review = consume, library = create

**Tradeoffs**:
- Users wanting to add variation must exit review → acceptable (rare use case)
- Requires context switch → mitigate with "Add phrasing" link that opens library in new tab

---

## Infrastructure Requirements

None. Feature is fully self-contained using existing infrastructure:
- ✅ Convex mutations (existing framework)
- ✅ Shadcn/ui components (already installed)
- ✅ Confirmation/undo hooks (already implemented)
- ✅ Toast notifications (Sonner already configured)
- ✅ Mobile responsiveness (Tailwind CSS)

---

## Next Steps

Run `/architect` to transform this PRD into detailed implementation plan with:
- Exact file changes
- Component pseudocode
- Mutation signatures
- Test stubs
- Migration strategy (if needed)

---

**Estimated Total Effort**: 20-26 hours across 5 phases
**Complexity Assessment**: Medium (well-scoped, clear requirements, existing patterns)
**User Value**: High (eliminates #1 user complaint about context switching)
**Risk Level**: Low (reversible actions, incremental delivery, tested patterns)
