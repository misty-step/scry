# TODO: Unified Edit Button Implementation

**Status:** ✅ Phase 4 - Complete | Phase 5 - Manual Testing Recommended

**Completed:**
- ✅ Phase 1: Validation module + tests (26 tests passing)
- ✅ Phase 2: useUnifiedEdit hook + tests (24 tests passing)
- ✅ Phase 3: Component extraction (OptionsEditor, TrueFalseEditor, UnifiedEditForm)
- ✅ Phase 4: Full integration into review-flow.tsx
  - Unified edit hook integrated
  - Optimistic state management for both concept and phrasing
  - Display properties with 3-tier logic (editing → optimistic → real)
  - Keyboard shortcuts (E key, Escape) updated
  - All 14 review-flow tests passing
  - All 24 use-unified-edit tests passing
  - TypeScript compilation successful

**Remaining:** Manual testing in browser, optional integration tests

---

## Phase 4: Review Flow Integration

### Core Integration

- [x] **Replace dual hooks with useUnifiedEdit in review-flow.tsx**
  - Location: `components/review-flow.tsx` lines ~77-100
  - Remove: Two separate `useInlineEdit` hook instances (`conceptEdit`, `phrasingEdit`)
  - Add: Single `useUnifiedEdit` hook with callbacks to `conceptActions.editConcept` and `conceptActions.editPhrasing`
  - Initial data mapping: Extract `conceptTitle`, `conceptDescription` from concept query; `question`, `correctAnswer`, `explanation`, `options` from question/phrasing
  - Pass `question?.type || 'multiple-choice'` as questionType parameter
  - Success criteria: Single hook instance manages all edit state; no compilation errors
  ```
  COMPLETED: Replaced dual hooks with unified hook, updated all references,
  removed inline edit UI from feedback section, integrated UnifiedEditForm.
  All 14 review-flow tests passing.
  ```

- [x] **Update optimistic state management for unified edits**
  - Location: `components/review-flow.tsx` lines ~170-196
  - Current: Manual `optimisticPhrasing` state for phrasing only
  - Change: Extend to support both concept and phrasing optimistic updates
  - Add: `optimisticConcept` state alongside `optimisticPhrasing`
  - Wire: useUnifiedEdit save handler returns optimistic data for both domains
  - Cleanup: Clear optimistic state when Convex reactivity catches up (separate useEffect for each domain)
  - Success criteria: Both concept title and phrasing fields show immediate UI feedback on save; optimistic state clears when real data arrives
  ```
  COMPLETED: Added optimisticConcept state, updated concept save handler to return
  optimistic data, added cleanup effect for concept optimistic state, created
  displayConceptTitle and enhanced displayQuestion memos to handle editing state,
  optimistic updates, and real data. Updated UI to use displayConceptTitle.
  All 14 review-flow tests passing, all 24 use-unified-edit tests passing.
  ```

- [x] **Create memoized display properties for concept and phrasing**
  - Location: `components/review-flow.tsx` (new code after hook declarations)
  - Add: `displayConceptTitle` useMemo - returns `unifiedEdit.localData.conceptTitle` if editing, else `conceptTitle`
  - Add: `displayQuestion` useMemo - returns question with merged optimistic phrasing data if editing
  - Wire: Update all references to `conceptTitle` and `question` in render to use display properties
  - Success criteria: UI shows localData during editing, real data when not editing; no flicker on mode transitions
  ```
  COMPLETED: Created displayConceptTitle useMemo with 3-tier logic (editing → optimistic → real).
  Enhanced displayQuestion useMemo to also handle editing state. Updated UI to use displayConceptTitle.
  ```

- [x] **Replace edit form rendering with UnifiedEditForm**
  ```
  COMPLETED: Already done in Task #1 commit (f91712b). UnifiedEditForm imported
  and integrated at line 566. No duplicate edit UIs.
  ```

- [x] **Update ReviewActionsDropdown integration**
  ```
  COMPLETED: Already done in Task #1 commit (f91712b). Single onEdit callback
  at line 538 triggers unified edit mode.
  ```

### Keyboard Shortcuts

- [x] **Update E key handler for unified edit**
  ```
  COMPLETED: Already done in Task #1 commit (f91712b). handleStartInlineEdit
  at line 453 triggers unifiedEdit.startEdit() and shows feedback section.
  ```

- [x] **Update Escape key handler for unified save**
  ```
  COMPLETED: Already done in Task #1 commit (f91712b). Escape handler calls
  unifiedEdit.save() with error handling.
  ```

### Cleanup

- [x] **Remove unused conceptEdit and phrasingEdit state**
  ```
  COMPLETED: Already done in Task #1 commit (f91712b). No references to
  conceptEdit or phrasingEdit remain in the codebase.
  ```

- [x] **Update imports in review-flow.tsx**
  ```
  COMPLETED: Already done in Task #1 commit (f91712b). useUnifiedEdit and
  UnifiedEditForm imported, old imports removed.
  ```

---

## Phase 5: Testing & Validation

### Integration Tests (Optional - Good coverage from unit tests)

**Note:** Core functionality is well-tested through:
- ✅ 24 use-unified-edit unit tests (all smart dirty detection, save orchestration, validation, partial failure handling)
- ✅ 14 review-flow tests (instant feedback, state management, UI interactions)
- ✅ 26 unified-edit-validation tests (all validation logic)

Integration tests below would provide additional end-to-end coverage but are not critical for shipping:

- [ ] **Write review-flow integration test for concept-only edit**
  - Test: User edits only concept title → verify single `updateConcept` mutation called
  - Already covered by: use-unified-edit.test.ts "Save Orchestration" tests
  - Value: End-to-end confirmation with full component tree

- [ ] **Write review-flow integration test for phrasing-only edit**
  - Test: User edits only question text → verify single `updatePhrasing` mutation called
  - Already covered by: use-unified-edit.test.ts "Save Orchestration" tests
  - Value: End-to-end confirmation with full component tree

- [ ] **Write review-flow integration test for both concept and phrasing edit**
  - Test: User edits both → verify both mutations called in parallel
  - Already covered by: use-unified-edit.test.ts "Save Orchestration" tests
  - Value: Parallel execution timing verification in full context

- [ ] **Write review-flow integration test for partial failure handling**
  - Test: Concept saves, phrasing fails → verify error handling
  - Already covered by: use-unified-edit.test.ts "Partial Failure Handling" tests (2 tests)
  - Value: End-to-end retry behavior verification

- [ ] **Write review-flow integration test for validation errors**
  - Test: User submits empty fields → verify validation errors shown
  - Already covered by: use-unified-edit.test.ts "Validation" tests (3 tests)
  - Value: Full form error UI verification

### Manual Testing Checklist

- [ ] **Test concept-only edit flow**
  - Manual steps: Open review → click Edit → modify only concept title → save
  - Verify: Single mutation called (check Network tab); title updates immediately; edit mode exits; FSRS state preserved
  - Edge cases: Empty title shows validation error; whitespace-only title rejected
  - Success criteria: Concept edits work correctly; validation prevents invalid input; optimistic updates smooth

- [ ] **Test phrasing-only edit flow**
  - Manual steps: Open review → click Edit → modify only question text → save
  - Verify: Single mutation called; question updates immediately; edit mode exits
  - Edge cases: Empty question shows error; MC question with correctAnswer not in options fails validation
  - Success criteria: Phrasing edits work correctly; validation comprehensive; user experience smooth

- [ ] **Test both concept and phrasing edit flow**
  - Manual steps: Open review → click Edit → modify both concept title and question → save
  - Verify: Both mutations called in parallel (check Network tab timing); both update immediately
  - Edge cases: One field invalid shows field-level error; both fields invalid shows all errors
  - Success criteria: Combined edits work correctly; parallel mutations faster than sequential; all fields update together

- [ ] **Test keyboard shortcuts**
  - Manual steps: Press E key → verify edit mode activates; modify fields; press Escape → verify save
  - Verify: E disabled during edit mode; Escape saves and exits on success; Escape stays in edit mode on validation failure
  - Edge cases: E while typing in another input doesn't trigger edit; Escape with no changes exits cleanly
  - Success criteria: Keyboard shortcuts work as expected; no interference with normal typing

- [ ] **Test options array editing (multiple-choice)**
  - Manual steps: Edit MC question → add option → remove option → change correct answer → save
  - Verify: Options array mutations work; removing correct option auto-selects first; changing option text updates correctAnswer if it was selected
  - Edge cases: Cannot remove below 2 options; cannot add above 6 options; validation prevents empty options
  - Success criteria: Options editor works correctly; edge cases handled gracefully; validation comprehensive

- [ ] **Test true-false editing**
  - Manual steps: Edit TF question → change correct answer → save
  - Verify: Radio buttons work; only "True" or "False" accepted; saves correctly
  - Edge cases: Invalid value rejected by validation
  - Success criteria: True-false editor simple and functional

- [ ] **Test partial failure handling**
  - Manual steps: Simulate network failure for one mutation (modify code temporarily or use DevTools to block)
  - Verify: Error shown for failed field; successful field marked clean; can retry with only failed field
  - Edge cases: Both fail shows both errors; retry after fixing works
  - Success criteria: Partial failures handled gracefully; user can recover without data loss

- [ ] **Test optimistic updates and cleanup**
  - Manual steps: Edit fields → save → watch UI during Convex sync
  - Verify: Changes show immediately (optimistic); no flicker when real data arrives; optimistic state clears correctly
  - Edge cases: Quick edit → next question clears optimistic state; slow network doesn't cause UI stutter
  - Success criteria: Optimistic updates smooth; cleanup prevents stale data; UX feels instant

- [ ] **Test error display and field clearing**
  - Manual steps: Submit invalid data → see errors → fix one field → verify error for that field clears
  - Verify: Field-level errors show with aria-invalid; errors map to correct fields; typing in field clears its error
  - Edge cases: Multiple errors for same field concatenated; all errors clear on successful save
  - Success criteria: Error UX helpful and precise; users understand what to fix

- [ ] **Test FSRS preservation**
  - Manual steps: Note nextReview date before edit → edit concept/phrasing → save → verify nextReview unchanged
  - Verify: Tooltip explains FSRS preservation; all FSRS fields (stability, difficulty, nextReview, state) unchanged
  - Edge cases: Major content change still preserves FSRS (tooltip educates about archiving instead)
  - Success criteria: FSRS state preserved; tooltip provides clear guidance; users understand behavior

---

## Code Quality & Documentation

- [ ] **Add JSDoc comments to useUnifiedEdit hook**
  - Location: `hooks/use-unified-edit.ts`
  - Document: Hook purpose, parameters, return values, usage examples
  - Include: Notes about parallel mutation execution and partial failure handling
  - Success criteria: JSDoc comprehensive; developers understand hook behavior without reading implementation

- [ ] **Add JSDoc comments to UnifiedEditForm component**
  - Location: `components/review/unified-edit-form.tsx`
  - Document: Component purpose, props, UX design decisions
  - Include: Notes about field-level errors and FSRS preservation tooltip
  - Success criteria: JSDoc explains component role in unified edit flow

- [ ] **Update inline comments in review-flow.tsx**
  - Location: `components/review-flow.tsx`
  - Update: Comments referencing old conceptEdit/phrasingEdit hooks
  - Add: Comment explaining unified edit integration
  - Remove: Outdated comments about separate edit flows
  - Success criteria: Comments accurate; explain why unified approach chosen

---

## Type Safety & Validation

- [ ] **Verify TypeScript compilation with strict mode**
  - Command: `npx tsc --noEmit`
  - Check: No type errors in review-flow.tsx integration
  - Fix: Any type mismatches between UnifiedEditData and actual data structure
  - Success criteria: Zero TypeScript errors; all types correctly inferred

- [ ] **Run full test suite to verify no regressions**
  - Command: `pnpm test`
  - Verify: All existing tests pass (validation: 26 tests, hook: 24 tests)
  - Check: No test failures in other components
  - Success criteria: All tests green; no regressions introduced

- [ ] **Run linter and fix any issues**
  - Command: `pnpm lint`
  - Fix: Any linting errors in new files (unified-edit-validation.ts, use-unified-edit.ts, etc.)
  - Address: Unused variables, import order, accessibility warnings
  - Success criteria: Zero linting errors; code follows project style guide

---

## Final Verification

- [ ] **Build project and verify no errors**
  - Command: `pnpm build`
  - Verify: Next.js build succeeds; no warnings about unused code
  - Check: Bundle size reasonable (extracted components should reduce duplication)
  - Success criteria: Clean build; production-ready code

- [ ] **Test in development mode with Convex**
  - Command: `pnpm dev`
  - Verify: Hot reload works; edit flow functional in browser
  - Test: Real mutations against Convex dev instance
  - Success criteria: Full end-to-end flow works in development

---

**Estimated Remaining Effort:** 6-8 hours
- Integration: 2-3 hours
- Testing: 3-4 hours
- Documentation & Cleanup: 1 hour

**Risk Assessment:** Low
- All core components tested and working
- Integration follows established patterns
- Reversible if issues found (can revert review-flow.tsx changes)
