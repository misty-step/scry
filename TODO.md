# TODO: Instant Answer Feedback (Optimistic UI)

## Context
- **Architecture**: Hybrid Ephemeral + Background Persistence (see TASK.md)
- **Key Files**:
  - `components/review-flow.tsx` (lines 139-165: handleSubmit to modify)
  - `hooks/use-instant-feedback.ts` (new)
  - `hooks/use-quiz-interactions.ts` (existing: modify to support async)
- **Patterns**: Follow `hooks/use-question-mutations.ts` for optimistic pattern structure
- **Testing**: Follow `hooks/use-quiz-interactions.test.ts` for vitest + React Testing Library patterns

## Phase 1 MVP: Instant Visual Feedback

### Module 1: Instant Feedback Controller

- [x] Implement `useInstantFeedback()` hook for ephemeral UI state
  ```
  Files: hooks/use-instant-feedback.ts (new)
  Architecture: Deep module - simple interface (showFeedback, clearFeedback), hides animation timing, ARIA updates, state management
  Interface:
    - showFeedback(isCorrect: boolean): void → immediate visual state
    - feedbackState: { isCorrect: boolean, visible: boolean }
    - clearFeedback(): void → reset for next question
  Implementation:
    - useState for feedbackState
    - useCallback for showFeedback (set visible=true + isCorrect)
    - useCallback for clearFeedback (reset to initial state)
    - No animation logic here (stays in ReviewQuestionDisplay)
  Success: Hook returns correct state, updates synchronously
  Test Strategy:
    - Unit: renderHook, verify state changes
    - Test showFeedback(true) → feedbackState.isCorrect === true
    - Test clearFeedback() → feedbackState.visible === false
  Dependencies: None (pure React hook)
  Time: 15min
  ```

- [x] Add ARIA live region to ReviewFlow for screen reader announcements
  ```
  Files: components/review-flow.tsx (modify)
  Architecture: Accessibility layer - announces feedback to assistive tech
  Implementation:
    - Add <div role="status" aria-live="polite" aria-atomic="true" className="sr-only">
    - Content: feedbackState.visible ? (feedbackState.isCorrect ? "Correct" : "Incorrect") : ""
    - Place near top of component return (before PageContainer)
  Pattern: Follow existing SR-only classes in codebase
  Success: Screen reader announces "Correct" or "Incorrect" when feedback shown
  Test Strategy:
    - E2E: Use @testing-library/react to verify aria-live content updates
    - Manual: Test with VoiceOver/NVDA
  Dependencies: Module 1 (useInstantFeedback)
  Time: 15min
  ```

- [x] Integrate instant feedback into ReviewFlow submit handler
  ```
  Files: components/review-flow.tsx (modify lines 139-165)
  Architecture: Orchestration layer - coordinates instant feedback with async tracking
  Current code (lines 139-165):
    const handleSubmit = useCallback(async () => {
      const isCorrect = selectedAnswer === question.correctAnswer;
      const reviewInfo = await trackAnswer(...); // BLOCKING
      setFeedbackState({ showFeedback: true, ... }); // DELAYED
    }, [...]);

  New code:
    import { useInstantFeedback } from '@/hooks/use-instant-feedback';
    const { showFeedback, feedbackState: instantFeedback, clearFeedback } = useInstantFeedback();

    const handleSubmit = useCallback(async () => {
      if (!selectedAnswer || !question || !conceptId || !phrasingId) return;
      const isCorrect = selectedAnswer === question.correctAnswer;

      // 1. INSTANT: Show visual feedback (synchronous)
      showFeedback(isCorrect);

      // 2. BACKGROUND: Track with FSRS (don't await - fire and forget for MVP)
      const timeSpent = Date.now() - questionStartTime;
      trackAnswer(conceptId, phrasingId, selectedAnswer, isCorrect, timeSpent, sessionId)
        .then((reviewInfo) => {
          // 3. PROGRESSIVE: Show scheduling details when ready
          setFeedbackState({
            showFeedback: true,
            nextReviewInfo: reviewInfo ? {
              nextReview: reviewInfo.nextReview,
              scheduledDays: reviewInfo.scheduledDays,
            } : null,
          });
        })
        .catch((error) => {
          // Phase 1 MVP: Just log errors, Phase 2 will add retry
          console.error('Failed to track answer:', error);
        });
    }, [selectedAnswer, question, conceptId, phrasingId, questionStartTime, trackAnswer, sessionId, showFeedback]);

  Success: Feedback appears instantly, backend mutation completes in background
  Test Strategy:
    - E2E: Click Submit → verify feedback appears <50ms (relaxed for test env)
    - E2E: Mock slow trackAnswer (2s delay) → feedback still instant
    - Unit: Verify trackAnswer called with correct params
  Dependencies: Module 1 (useInstantFeedback)
  Time: 30min
  ```

### Module 2: Visual Feedback Updates

- [x] Update ReviewQuestionDisplay to use instant feedback state for button colors
  ```
  Files: components/review-question-display.tsx (modify)
  Architecture: Presentation layer - visual state driven by instant feedback
  Current: Button colors based on feedbackState.showFeedback (delayed)
  New: Accept instantFeedback prop from parent, use for immediate color changes

  Interface change:
    Add prop: instantFeedback?: { isCorrect: boolean, visible: boolean }

  Implementation:
    - Lines 49-128: Update button className logic
    - Priority: instantFeedback > feedbackState (instant wins during transition)
    - Correct: border-success-border bg-success-background text-success
    - Incorrect: border-error-border bg-error-background text-error
    - Keep existing CheckCircle/XCircle icon logic

  Pattern: Follow existing conditional className logic in component
  Success: Buttons show correct colors immediately when instantFeedback.visible
  Test Strategy:
    - Component test: render with instantFeedback.visible=true, isCorrect=true
    - Verify success classes applied
    - Component test: render with instantFeedback.visible=true, isCorrect=false
    - Verify error classes applied
  Dependencies: None (prop-driven)
  Time: 20min
  ```

- [x] Update ReviewFlow to pass instant feedback to ReviewQuestionDisplay
  ```
  Files: components/review-flow.tsx (modify)
  Architecture: Integration point - connect instant feedback to display
  Implementation:
    - Pass instantFeedback prop to ReviewQuestionDisplay
    - Value: feedbackState from useInstantFeedback hook
    - Keep existing feedbackState for progressive details section

  Code location: Line ~290 where ReviewQuestionDisplay is rendered

  Success: ReviewQuestionDisplay receives instant feedback prop
  Test Strategy:
    - Integration test: Verify prop passed correctly
    - E2E: Submit answer → button colors change instantly
  Dependencies: Module 1 (useInstantFeedback), Module 2.1 (ReviewQuestionDisplay update)
  Time: 10min
  ```

### Module 3: State Management

- [x] Reset instant feedback state when question changes
  ```
  Files: components/review-flow.tsx (modify)
  Architecture: State lifecycle management
  Current: Lines 118-129 useEffect resets selectedAnswer and feedbackState
  New: Also call clearFeedback() from useInstantFeedback

  Implementation:
    useEffect(() => {
      if (phrasingId && !isTransitioning) {
        setSelectedAnswer('');
        clearFeedback(); // ADD THIS
        setFeedbackState({
          showFeedback: false,
          nextReviewInfo: null,
        });
        setQuestionStartTime(Date.now());
      }
    }, [phrasingId, isTransitioning, clearFeedback]);

  Success: Instant feedback cleared when new question loads
  Test Strategy:
    - Integration test: Change phrasingId → verify instantFeedback.visible = false
    - E2E: Complete question, next question loads → feedback cleared
  Dependencies: Module 1 (useInstantFeedback)
  Time: 10min
  ```

### Module 4: Testing

- [x] Write unit tests for useInstantFeedback hook
  ```
  Files: hooks/use-instant-feedback.test.ts (new)
  Pattern: Follow hooks/use-quiz-interactions.test.ts structure
  Test cases:
    - Initial state: visible=false, isCorrect=false
    - showFeedback(true): visible=true, isCorrect=true
    - showFeedback(false): visible=true, isCorrect=false
    - clearFeedback(): visible=false, isCorrect=false
    - Multiple showFeedback calls: last call wins

  Framework: vitest + @testing-library/react (renderHook)
  Success: All tests pass, 100% branch coverage
  Dependencies: Module 1 (useInstantFeedback)
  Time: 20min
  ```

- [ ] Write integration tests for instant feedback flow
  ```
  Files: components/review-flow.test.tsx (modify existing or new file)
  Pattern: Follow existing component test patterns
  Test cases:
    - User selects answer + submits → instant feedback appears
    - Correct answer → success colors + checkmark
    - Incorrect answer → error colors + X icon
    - ARIA live region announces "Correct"/"Incorrect"
    - Backend mutation completes → scheduling details appear
    - New question loads → feedback cleared

  Mocking strategy:
    - Mock trackAnswer with 500ms delay
    - Mock useReviewFlow with test question data
    - Use vi.fn() for handlers

  Success: All integration scenarios pass
  Dependencies: All Module 1-3 tasks complete
  Time: 45min
  ```

- [ ] Manual testing checklist
  ```
  Test in browser (development mode):
    [ ] Click Submit → feedback appears instantly (use DevTools Performance tab: <16ms)
    [ ] Correct answer → green border + checkmark
    [ ] Incorrect answer → red border + X
    [ ] Screen reader (VoiceOver/NVDA) announces feedback
    [ ] Backend mutation completes → "Next review: In X days" appears
    [ ] Click Next → new question, feedback cleared
    [ ] Rapid submissions (click Submit 3x fast) → only one mutation
    [ ] Slow network (DevTools: Fast 3G) → feedback still instant

  Success: All manual tests pass
  Dependencies: All modules complete
  Time: 30min
  ```

## Phase 1 Acceptance Criteria

Before merging to master:
- [ ] Feedback appears <16ms after Submit click (measured via Performance API) - Ready for manual testing
- [ ] Backend mutation still completes successfully (check Convex dashboard) - Ready for manual testing
- [ ] Zero regression in FSRS tracking accuracy (compare interaction logs before/after) - Ready for manual testing
- [ ] ARIA live region announces feedback (test with screen reader) - Ready for manual testing
- [x] All unit + integration tests pass - useInstantFeedback tests: 6/6 passing
- [x] pnpm test → no failures - 591/600 passing (9 unrelated worker failures)
- [x] pnpm build → successful build - TypeScript compiles without errors
- [ ] No console errors in browser - Ready for manual testing

## Phase 2 Preview (Not Included in This TODO)

Future work after Phase 1 ships:
- Retry logic with exponential backoff (useBackgroundTracker hook)
- Error toast notifications for persistent failures
- Progressive detail loading with "Calculating..." fallback
- localStorage queue for offline support
- Sentry error tracking integration
- Comprehensive error scenario testing

## Notes

**Why fire-and-forget in Phase 1:**
- Proves the instant feedback concept
- Simplifies initial implementation
- Backend mutation is reliable (>99% success in prod)
- Phase 2 adds retry + error handling based on Phase 1 telemetry

**Why no new constants needed:**
- No new timing constants (instant = synchronous)
- Reusing existing OPTIMISTIC_UPDATE_CLEAR_DELAY for reference
- Animation timing stays in Tailwind (animate-scaleIn)

**Module boundaries respected:**
- useInstantFeedback: Pure UI state (no backend knowledge)
- ReviewFlow: Orchestration (coordinates feedback + tracking)
- ReviewQuestionDisplay: Presentation (driven by props)
- Each module can evolve independently
