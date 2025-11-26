# TODO: In-Review Editing & Archiving

## Context

**Architecture**: Inline editing + dropdown actions (TASK.md Section: Architecture Decision)
**Selected Approach**: No modals, inline field editing, unified "archive" semantics
**Key Design Principles**:
- Preserve FSRS state on edits
- 8-second undo window for reversible actions
- Mobile-first (44px touch targets)
- Backend-first workflow (mutations → frontend)

**Key Files**:
- Backend: `convex/concepts.ts` (add 3 new mutations)
- Frontend: `components/review-flow.tsx` (add inline editing + dropdown)
- Hooks: `hooks/use-inline-edit.ts` (new), `hooks/use-concept-actions.ts` (extend)
- Components: `components/review/review-actions-dropdown.tsx` (new)

**Existing Patterns to Follow**:
- Mutation pattern: `archivePhrasing` (line 678), `unarchiveConcept` (line 858)
- Hook pattern: `use-concept-actions.ts` (toast feedback, pending state)
- Test pattern: `tests/convex/spacedRepetition.test.ts` (mock docs, vitest)

---

## Phase 1: Backend Mutations (Backend-First)

### Core Mutations

- [x] **Implement `concepts.updateConcept` mutation**
  ```
  File: convex/concepts.ts (new export line 840)
  Architecture: Follows existing mutation pattern (requireUserFromClerk → validate → patch)
  Implemented:
    ✓ Validates user ownership (requireUserFromClerk → concept.userId check)
    ✓ Trims title, validates non-empty
    ✓ Patches concept with { title, description?, updatedAt }
    ✓ PRESERVES all FSRS fields (fsrs, phrasingCount, etc)
  Tests: tests/convex/concepts.update.test.ts (7 test cases, all pass)
    ✓ Title/description updates
    ✓ FSRS preservation verified
    ✓ Empty title rejection
    ✓ Whitespace trimming
    ✓ Unauthorized access blocked
    ✓ Non-existent concept handling
    ✓ Optional description handling
  Commit: 059383f
  Time: 45min (TDD: tests first, then implementation)
  ```

- [ ] **Implement `concepts.updatePhrasing` mutation**
  ```
  File: convex/concepts.ts (new export ~line 800)
  Architecture: Similar to updateConcept but with MC/TF validation
  Pseudocode:
    1. Validate user ownership (fetch phrasing, check userId)
    2. Trim all string fields (question, correctAnswer, explanation)
    3. Validate non-empty question and correctAnswer
    4. If options provided: validate correctAnswer exists in options
    5. Patch phrasing with { question, correctAnswer, explanation?, options?, updatedAt }
    6. PRESERVE phrasing stats (attemptCount, lastAttemptedAt)
  Success:
    - All editable fields update
    - MC validation blocks invalid correctAnswer
    - Stats fields preserved
  Test: tests/convex/concepts.update.test.ts
    - Test question/answer editing
    - Test MC validation (correctAnswer not in options → error)
    - Test TF validation
    - Test stats preservation
  Dependencies: None
  Time: 1hr
  ```

- [ ] **Implement `concepts.unarchivePhrasing` mutation (complete mutation pair)**
  ```
  File: convex/concepts.ts (new export ~line 840)
  Architecture: Reverse of archivePhrasing (line 678), follows unarchiveConcept pattern
  Pseudocode:
    1. Validate user ownership
    2. Return early if not archived (idempotent)
    3. Clear archivedAt timestamp
    4. Fetch all active phrasings (include newly unarchived)
    5. Recalculate concept.phrasingCount
    6. Recalculate thinScore using computeThinScoreFromCount()
    7. Recalculate conflictScore (like archivePhrasing line 716-722)
    8. Update concept with new counts/scores
  Success:
    - Phrasing restored (archivedAt cleared)
    - Concept phrasingCount incremented
    - Scores recalculated correctly
  Test: tests/convex/concepts.archive.test.ts
    - Test archive → unarchive cycle (idempotent)
    - Test phrasingCount updates
    - Test score recalculation
    - Test unauthorized access blocked
  Dependencies: None
  Time: 1hr
  ```

**Phase 1 Acceptance**:
- All 3 mutations export from `convex/concepts.ts`
- `pnpm convex dev` shows "Convex functions ready!"
- All tests pass: `pnpm test tests/convex/concepts.update.test.ts`
- No TypeScript errors

---

## Phase 2: Frontend Inline Editing

### Hooks

- [ ] **Create `useInlineEdit` hook for state management**
  ```
  File: hooks/use-inline-edit.ts (new)
  Architecture: Generic hook managing edit mode, optimistic updates, rollback
  Pseudocode:
    1. State: isEditing, localData (copy of initial), isSaving
    2. startEdit(): setIsEditing(true), copy initialData to localData
    3. updateField(key, value): setLocalData({ ...localData, [key]: value })
    4. save():
       - setIsSaving(true)
       - call onSave(localData) mutation
       - on success: exit edit mode
       - on error: rollback localData to initialData, show toast
    5. cancel(): setIsEditing(false), revert localData
  Success:
    - Edit mode toggle works
    - Optimistic updates immediate
    - Rollback on error
    - TypeScript generic types work
  Test: hooks/use-inline-edit.test.ts
    - Test edit mode activation
    - Test field updates
    - Test save success path
    - Test error rollback
  Dependencies: Phase 1 mutations must exist
  Time: 1hr
  ```

- [ ] **Extend `use-concept-actions.ts` with edit/archive actions**
  ```
  File: hooks/use-concept-actions.ts (modify existing)
  Architecture: Add updateConcept, updatePhrasing, and wrap archives with undo
  Pseudocode:
    1. Add useMutation for updateConcept, updatePhrasing
    2. editConcept(data): call updateConcept, toast on success/error
    3. editPhrasing(data): call updatePhrasing, toast on success/error
    4. archivePhrasingWithUndo(id): use useUndoableAction (8sec duration)
    5. archiveConceptWithUndo(id): use useUndoableAction
  Success:
    - Edit actions work with toast feedback
    - Archive actions show undo toast
    - Undo calls unarchivePhrasing/unarchiveConcept
  Test: hooks/use-concept-actions.test.ts (extend existing)
    - Test edit mutations
    - Test undo toast integration
  Dependencies: useInlineEdit, Phase 1 mutations
  Time: 1hr
  ```

### Components

- [ ] **Add inline editing to `review-flow.tsx`**
  ```
  File: components/review-flow.tsx (modify existing, ~line 100-400)
  Architecture: Integrate useInlineEdit for concept title/description and phrasing fields
  Pseudocode:
    1. Add useInlineEdit for concept data
    2. Add useInlineEdit for phrasing data
    3. Render title as editable:
       - If editing: <Input value={localTitle} onChange={...} />
       - If not editing: <h2>{title}</h2>
    4. Render question/answer as editable (same pattern)
    5. Add Save/Cancel buttons when editing (only visible in edit mode)
    6. Keyboard shortcuts:
       - E key → startEdit() (only when not already editing)
       - Esc → save() and exit
       - Blur → auto-save
  Success:
    - Click field → becomes editable
    - Save button works
    - Esc/blur auto-save
    - Visual indicators (blue border) during edit
    - Post-save: stay on edited card
  Test: components/review-flow.test.tsx (new)
    - Test edit mode activation
    - Test save/cancel
    - Test keyboard shortcuts
  Dependencies: useInlineEdit, use-concept-actions
  Time: 2.5hrs
  ```

**Phase 2 Acceptance**:
- Can edit concept title, description inline
- Can edit phrasing question, answer, explanation inline
- Save/cancel buttons functional
- Keyboard shortcuts work (E, Esc)
- Tests pass
- Mobile keyboard interactions smooth (manual test)

---

## Phase 3: Archive Actions + Undo

- [ ] **Integrate undo toasts for archive actions**
  ```
  File: components/review-flow.tsx (modify existing)
  Architecture: Replace direct archive calls with useUndoableAction
  Pseudocode:
    1. Import useUndoableAction
    2. archivePhrasingHandler():
       - await undoableAction({
           action: () => archivePhrasing(phrasingId),
           message: 'Question archived',
           undo: () => unarchivePhrasing(phrasingId),
           duration: 8000
         })
       - Navigate to next phrasing/concept
    3. archiveConceptHandler():
       - await undoableAction({
           action: () => archiveConcept(conceptId),
           message: 'Concept archived',
           undo: () => unarchiveConcept(conceptId),
           duration: 8000
         })
       - Navigate to next concept
    4. Post-archive: use handlers.onReviewComplete() pattern
  Success:
    - Archive shows undo toast (8 seconds)
    - Undo restores item
    - Review flow continues to next item
    - Toast auto-dismisses
  Test: components/review-flow.test.tsx
    - Test archive → undo cycle
    - Test navigation after archive
  Dependencies: Phase 1 mutations, useUndoableAction (existing)
  Time: 1.5hrs
  ```

- [ ] **Handle "archive last phrasing" edge case**
  ```
  File: components/review-flow.tsx (modify archivePhrasingHandler)
  Architecture: Block archiving if only 1 phrasing, suggest concept archive
  Pseudocode:
    1. Before archiving phrasing, check totalPhrasings count
    2. If totalPhrasings === 1:
       - Show toast: "This is the last phrasing. Archive the entire concept instead?"
       - Return early (don't archive)
    3. Otherwise: proceed with archive
  Success:
    - Archiving last phrasing blocked
    - Clear message shown
    - User can archive concept instead
  Test: components/review-flow.test.tsx
    - Test 1 phrasing → archive blocked
    - Test 2+ phrasings → archive works
  Dependencies: Phase 3 undo integration
  Time: 30min
  ```

**Phase 3 Acceptance**:
- Archive shows 8-second undo toast
- Undo fully restores item
- Last phrasing edge case handled
- Review flow continues smoothly after archive
- Tests pass

---

## Phase 4: Dropdown Menu + Integration

- [ ] **Create `ReviewActionsDropdown` component**
  ```
  File: components/review/review-actions-dropdown.tsx (new)
  Architecture: Shadcn DropdownMenu with 4 actions, conditional rendering
  Pseudocode:
    Props: { conceptId, phrasingId, totalPhrasings, onEditConcept, onEditPhrasing, onArchiveConcept, onArchivePhrasing }
    1. DropdownMenuTrigger: ⋮ icon (MoreVertical from lucide-react)
    2. DropdownMenuContent:
       - Edit Question → onEditPhrasing()
       - Edit Concept → onEditConcept()
       - <Separator />
       - Archive Question → onArchivePhrasing() (hidden if totalPhrasings === 1)
       - Archive Concept → onArchiveConcept()
    3. Mobile: min-h-44px, min-w-44px touch targets
    4. Keyboard: accessible via tab navigation
  Success:
    - Dropdown opens/closes
    - All 4 actions trigger correctly
    - Archive Question hidden when only 1 phrasing
    - Mobile-friendly (44px targets)
  Test: components/review/review-actions-dropdown.test.tsx
    - Test all action callbacks
    - Test conditional rendering (1 phrasing case)
    - Test keyboard navigation
  Dependencies: None (uses existing shadcn/ui)
  Time: 1.5hrs
  ```

- [ ] **Replace legacy edit/delete buttons in `review-flow.tsx`**
  ```
  File: components/review-flow.tsx (modify existing, ~line 366-391)
  Architecture: Remove legacy question buttons, add ReviewActionsDropdown for concepts
  Pseudocode:
    1. Remove old Edit/Delete buttons for legacy questions (line 366-391)
    2. Add ReviewActionsDropdown:
       <ReviewActionsDropdown
         conceptId={conceptId}
         phrasingId={phrasingId}
         totalPhrasings={totalPhrasings}
         onEditConcept={() => conceptEdit.startEdit()}
         onEditPhrasing={() => phrasingEdit.startEdit()}
         onArchiveConcept={handleArchiveConcept}
         onArchivePhrasing={handleArchivePhrasing}
       />
    3. Position: top-right of review card
  Success:
    - Legacy buttons removed
    - Dropdown visible and functional
    - Actions wire to correct handlers
  Test: Visual regression test (Playwright snapshot)
  Dependencies: ReviewActionsDropdown component
  Time: 1hr
  ```

- [ ] **Add educational tooltips for FSRS preservation**
  ```
  File: components/review-flow.tsx (modify inline edit UI)
  Architecture: Info icon + Tooltip from shadcn/ui
  Pseudocode:
    1. Next to Save button, add:
       <Tooltip>
         <TooltipTrigger><Info size={16} /></TooltipTrigger>
         <TooltipContent>
           Edits preserve your learning progress.
           For major changes, archive and create a new concept.
         </TooltipContent>
       </Tooltip>
    2. Only show when in edit mode
    3. Mobile: tap to show, tap outside to dismiss
  Success:
    - Tooltip visible during edit
    - Clear educational message
    - Dismissible
  Test: Manual test (tooltip interaction)
  Dependencies: Phase 2 inline editing
  Time: 30min
  ```

**Phase 4 Acceptance**:
- Single dropdown menu houses all actions
- Legacy edit/delete removed
- FSRS tooltip educates users
- Mobile touch targets ≥44px (manual test)
- All tests pass

---

## Phase 5: Keyboard Shortcuts + Polish

- [ ] **Add keyboard shortcuts (E for edit, # for archive)**
  ```
  File: components/review-flow.tsx (modify existing useEffect for shortcuts)
  Architecture: Extend existing useReviewShortcuts or add new useEffect
  Pseudocode:
    1. useEffect(() => {
         const handleKeyDown = (e: KeyboardEvent) => {
           // Only if not already editing and no input focused
           if (isEditing || document.activeElement?.tagName === 'INPUT') return;

           if (e.key === 'e' || e.key === 'E') {
             e.preventDefault();
             phrasingEdit.startEdit(); // Default to phrasing edit
           }

           if (e.key === '#' || e.key === 'Backspace') {
             e.preventDefault();
             // Open dropdown or trigger archive (TBD)
           }
         };
         document.addEventListener('keydown', handleKeyDown);
         return () => document.removeEventListener('keydown', handleKeyDown);
       }, [isEditing]);
  Success:
    - E key activates edit mode
    - # key triggers archive action
    - Shortcuts disabled during text input
  Test: E2E test (Playwright keyboard interaction)
  Dependencies: Phase 2 inline editing
  Time: 45min
  ```

- [ ] **Mobile UX testing & polish**
  ```
  Files: All Phase 2-4 components
  Architecture: Manual testing on real devices, refinements
  Tasks:
    1. Test on iOS Safari (iPhone 12+)
       - Inline editing with keyboard
       - Dropdown menu reachability
       - Undo toast tap target
    2. Test on Chrome Android (Pixel 5+)
       - Same tests as iOS
    3. Adjustments:
       - Increase touch targets if <44px
       - Fix keyboard focus issues
       - Adjust z-index for dropdown/toast overlap
  Success:
    - Smooth editing on mobile
    - All touch targets ≥44px
    - Keyboard dismiss works
    - Undo toast reachable with thumb
  Test: Manual testing checklist
  Dependencies: All previous phases
  Time: 1.5hrs
  ```

- [ ] **Playwright E2E tests for critical flows**
  ```
  File: tests/e2e/review-editing.spec.ts (new)
  Architecture: E2E tests covering happy path + 3 edge cases
  Test Scenarios:
    1. Edit phrasing flow:
       - Navigate to review
       - Open dropdown → Edit Question
       - Modify question text
       - Save → verify appears on card
    2. Archive with undo:
       - Archive phrasing
       - Verify undo toast appears
       - Click undo → verify restored
    3. Archive last phrasing edge case:
       - Concept with 1 phrasing
       - Attempt archive → verify blocked with message
    4. Keyboard shortcuts:
       - Press E → edit mode activates
       - Edit + Esc → saves and exits
  Success:
    - All 4 scenarios pass
    - Run in CI on PRs
    - <2min total runtime
  Test: pnpm test:e2e
  Dependencies: All previous phases
  Time: 2hrs
  ```

**Phase 5 Acceptance**:
- Keyboard shortcuts functional (E, #)
- Mobile testing complete (iOS + Android)
- E2E tests cover critical flows
- No mobile-specific bugs
- Tests pass in CI

---

## Design Iteration

**After Phase 2 (Inline Editing)**:
- Review inline edit UX: Is blur-to-save intuitive? Should we require explicit save click?
- Check mobile keyboard behavior: Does blur work correctly on iOS?
- Consider: Should edited card auto-advance after 3 seconds or require manual "Next"?

**After Phase 4 (Dropdown Integration)**:
- Review dropdown positioning: Is top-right optimal on mobile? Should it be bottom-right?
- Check tooltip clarity: Do users understand FSRS preservation?
- Measure: What % of edits are typo fixes vs substantial changes?

---

## Automation Opportunities

**Test Data Generation**:
- Script to create test concepts with varying phrasing counts (1, 3, 10)
- Use in manual testing and E2E tests
- Location: `tests/helpers/concept-fixtures.ts`

**Visual Regression Snapshots**:
- Playwright snapshots for review-flow before/after dropdown change
- Run on every PR to catch UI regressions
- Automate in CI: `pnpm test:e2e:snapshots`

---

## Non-Code TODOs (Process - NOT in this file, just for reference)

These are workflow tasks, NOT implementation tasks. They belong in BACKLOG.md or workflow docs:
- ~~Create PR after Phase 1 complete~~
- ~~Run full test suite before merge~~
- ~~Update CHANGELOG.md~~
- ~~Deploy to preview environment~~

---

## Task Summary

**Total Tasks**: 14 implementation tasks
**Estimated Time**: 20-26 hours
**Parallelizable**: Phases 1-2 (backend + hook can work in parallel after mutations done)
**Critical Path**: Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5
**Highest Risk**: Mobile keyboard interactions (Phase 2), E2E test flakiness (Phase 5)

**Modules Created**:
1. `updateConcept` mutation (deep module: hides validation, ownership, FSRS preservation)
2. `updatePhrasing` mutation (deep module: hides MC/TF validation)
3. `unarchivePhrasing` mutation (completes mutation pair)
4. `useInlineEdit` hook (deep module: hides edit state, optimistic updates, rollback)
5. `ReviewActionsDropdown` component (deep module: hides action routing, conditional rendering)

**Each module = simple interface + powerful implementation.**
