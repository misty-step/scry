# PRD: Instant Answer Feedback (Optimistic UI)

## Executive Summary

Replace blocking answer submission with instant visual feedback. Currently, users wait 200-500ms for backend mutation (FSRS calculation + DB writes) before seeing if they answered correctly. This breaks the learning feedback loop. Solution: Show correct/incorrect immediately using client-side comparison, run backend tracking asynchronously, load scheduling details progressively. Strategic win: Decouples UI responsiveness from backend complexity, enabling future FSRS optimizations without UI changes.

## User Context

**Who**: Users actively reviewing flashcards in spaced repetition sessions

**Problem**: Delayed feedback interrupts the dopamine loop critical for learning. Brain expects immediate reinforcement (correct=pleasure, incorrect=correction). Current 200-500ms delay creates uncertainty: "Did my click register? Was I right?"

**Measurable Benefits**:
- **Perceived performance**: Instant feedback (0ms perceived latency)
- **Learning psychology**: Immediate reinforcement strengthens memory formation
- **Session flow**: Faster reviews = higher throughput without sacrificing accuracy
- **Backend flexibility**: FSRS calculations can take longer without degrading UX

## Requirements

### Functional

1. **Instant Visual Feedback**: Show correct/incorrect state immediately on submit (green border + checkmark OR red border + X icon)
2. **Client-Side Correctness**: Compare `selectedAnswer === question.correctAnswer` without backend round-trip
3. **Background Persistence**: Execute `recordConceptInteraction` mutation asynchronously
4. **Progressive Enhancement**: Load scheduling info ("Next review: In 3 days") when backend responds
5. **Accessibility**: Announce feedback to screen readers via ARIA live regions
6. **Error Recovery**: Retry failed mutations in background, allow user to continue on persistent failure

### Non-Functional

- **Performance**: <16ms time-to-feedback (single frame, imperceptible)
- **Reliability**: 99.9% background mutation success with transparent retry
- **Maintainability**: Clear separation between UI feedback and data persistence
- **Bandwidth**: Zero increase (optimistic updates are local-only)
- **Backend-First**: Preserve pure FSRS philosophy (all scheduling logic stays backend)

## Architecture Decision

### Selected Approach: **Hybrid Ephemeral + Background Persistence**

**Core Pattern**: Instant ephemeral UI state for feedback, background mutation for persistent FSRS tracking, progressive loading for enrichment data.

**Why This Approach**:
- **Deep modules**: Simple interface (`showFeedback(isCorrect)`) hides complex retry/progressive-load logic
- **Information hiding**: UI doesn't know FSRS internals, FSRS doesn't know UI timing
- **Right abstractions**: Feedback (transient) vs. Tracking (persistent) are correctly separated
- **Strategic investment**: Enables independent evolution of UI and backend

### Alternatives Considered

| Approach | User Value | Simplicity | Risk | Why Not Chosen |
|----------|-----------|-----------|------|----------------|
| **Pure Client State** | High (instant) | High (minimal code) | High (loses data on failure) | Violates backend-first, no error handling |
| **Convex `.withOptimisticUpdate()`** | Medium (framework overhead) | Medium (new pattern) | Low (framework support) | Feedback is ephemeral UI state, not query data - abstraction mismatch |
| **Hybrid (SELECTED)** | High (instant + reliable) | Medium (3 clear modules) | Low (proven pattern) | Best balance of UX and reliability |

### Module Boundaries

#### Module 1: Instant Feedback Controller
**Interface**:
```typescript
function useInstantFeedback() {
  return {
    showFeedback: (isCorrect: boolean) => void,
    feedbackState: { isCorrect: boolean, visible: boolean },
    clearFeedback: () => void
  };
}
```

**Responsibility**: Immediate visual reinforcement + accessibility announcements

**Hidden Complexity**:
- Animation timing (checkmark/X scale-in)
- Color transitions (neutral → success/error)
- ARIA live region updates
- Icon component selection

#### Module 2: Background Tracker
**Interface**:
```typescript
function useBackgroundTracker() {
  return {
    trackAnswer: (params: TrackingParams) => Promise<SchedulingInfo>,
    trackingState: { status: 'idle' | 'pending' | 'success' | 'error', error?: Error },
    retryTracking: () => void
  };
}
```

**Responsibility**: Persistent FSRS state management with retry logic

**Hidden Complexity**:
- Mutation execution with exponential backoff retry
- Error classification (transient vs. critical)
- Queue for offline/failed mutations
- Telemetry for tracking failures

#### Module 3: Progressive Data Loader
**Interface**:
```typescript
function useProgressiveDetails() {
  return {
    details: { nextReview: Date, scheduledDays: number, explanation: string } | null,
    isLoading: boolean
  };
}
```

**Responsibility**: Non-critical enrichment data (explanation, history, next review)

**Hidden Complexity**:
- Coordination with backend mutation lifecycle
- Caching to prevent duplicate fetches
- Fallback states for slow networks

### Abstraction Layers

**Layer 1 (Component)**: User clicks Submit → sees feedback
**Vocabulary**: "correct", "incorrect", "next question"

**Layer 2 (Hooks)**: Feedback state machine + async tracking
**Vocabulary**: "feedbackState", "trackingState", "retry"

**Layer 3 (Convex Mutation)**: FSRS calculation + database updates
**Vocabulary**: "recordInteraction", "scheduleResult", "updateStats"

Each layer transforms concepts meaningfully - no pass-through logic.

## Dependencies & Assumptions

**Dependencies**:
- Existing `recordConceptInteraction` mutation (no changes)
- `question.correctAnswer` available client-side
- Animation utilities (`animate-scaleIn`, Tailwind transitions)
- Toast notification system for error feedback

**Assumptions**:
- Correctness is deterministic (answer key is source of truth)
- FSRS mutation latency: 200-500ms typical, up to 2s acceptable
- Users review at 1 answer per 3-5 seconds pace
- Mutation failure rate <0.1% under normal conditions
- `question.correctAnswer` staleness is negligible (rare mid-review edits)

**Scale Expectations**:
- 100-1000 concurrent reviewers
- 10-50 answers per user per session
- 1000-10000 mutations per day initially

## Implementation Phases

### Phase 1 MVP: Instant Visual Feedback (1-2 days)

**Goal**: Prove the feedback loop improvement

**Scope**:
1. Create `useInstantFeedback()` hook managing ephemeral state
2. Modify `handleSubmit()` in `review-flow.tsx`:
   - Compute `isCorrect` immediately
   - Call `showFeedback(isCorrect)` synchronously
   - Fire `trackAnswer()` asynchronously (no await)
3. Update `ReviewQuestionDisplay` to use instant feedback state for button colors
4. Add ARIA live region: "Correct" / "Incorrect" announcements
5. Keep existing feedback section hidden until backend responds

**Success Criteria**:
- Feedback appears <16ms after Submit click
- Backend mutation still completes successfully
- No regressions in FSRS tracking accuracy

### Phase 2 Hardening: Error Recovery + Progressive Details (2-3 days)

**Goal**: Production-ready reliability

**Scope**:
1. Implement `useBackgroundTracker()` with retry logic:
   - Exponential backoff (100ms, 500ms, 2s)
   - Max 3 retries before showing error
   - Queue failed mutations for later retry
2. Progressive detail loading:
   - Show feedback section when backend responds
   - Fallback: "Calculating next review..." for slow responses
3. Error handling:
   - Toast notification: "Tracking failed, retrying..."
   - Allow user to continue to next question
   - Log telemetry for monitoring
4. Add Sentry error tracking for mutation failures
5. Comprehensive test coverage (optimistic flow, errors, retries)

**Success Criteria**:
- 99.9% tracking success rate (with retries)
- Users never blocked by transient failures
- Error telemetry dashboards operational

### Phase 3 Future: Optimistic Transitions + Batch Optimization (Planned)

**Deferred to 6-month horizon**:
- Optimistic "Next Question" transition (don't wait for mutation)
- Batch multiple answers if user goes rapid-fire
- Offline-first: Queue answers when offline, sync when online
- Predictive FSRS: Pre-compute likely next states client-side

**Why Deferred**: Premature optimization. Wait for usage data showing these are valuable.

## Risks & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| **Stale answer key** | Very Low | Low | Accept risk - editing questions mid-review is rare, consequence minor (one wrong feedback) |
| **Backend mutation fails silently** | Low | High | Retry logic + error telemetry + user notification on persistent failure |
| **Race condition: user clicks Next before mutation completes** | Medium | Medium | Queue mutation, block hard navigation (browser close) with beforeunload warning if pending |
| **Incorrect correctness logic** | Very Low | High | Mirror backend logic exactly, add integration tests comparing client vs server |
| **Visual feedback misleading on slow networks** | Low | Low | Progressive loader shows "Saving..." if >1s, reassures user |
| **FSRS tracking data loss** | Low | Critical | Mutation retry queue with localStorage persistence, periodic background sync |

## Key Decisions

### Decision 1: Ephemeral UI State vs. Optimistic Query Cache
**Choice**: Ephemeral UI state (React useState)
**Alternatives**: Convex `.withOptimisticUpdate()` on query cache
**Rationale**:
- **User Value**: Equal (both instant)
- **Simplicity**: Ephemeral is simpler - no query to update, feedback is transient by nature
- **Explicitness**: Clear separation - feedback is UI concern, tracking is data concern
**Tradeoffs**: Can't use Convex's automatic rollback, but we don't want rollback (correctness is certain)

### Decision 2: Block Next Question vs. Allow Progression
**Choice**: Allow progression with background retry
**Alternatives**: Block until mutation succeeds
**Rationale**:
- **User Value**: Don't interrupt flow for infrastructure issues
- **Simplicity**: Retry queue is standard pattern
- **Explicitness**: User sees toast if problem persists
**Tradeoffs**: Slight risk of data loss if browser crashes with pending mutations (mitigated by localStorage queue)

### Decision 3: Retry Strategy
**Choice**: 3 retries with exponential backoff (100ms, 500ms, 2s)
**Alternatives**: Infinite retries, no retries, user-triggered retry
**Rationale**:
- **User Value**: Handles transient network blips transparently
- **Simplicity**: Standard pattern, well-understood behavior
- **Explicitness**: Clear max attempts prevents infinite loops
**Tradeoffs**: Adds 2.6s worst-case latency, but runs in background so non-blocking

### Decision 4: Wait for Second Use Case Before Abstracting
**Choice**: Implement for answer submission only, no generic "optimistic action" abstraction
**Alternatives**: Build generic `useOptimisticAction()` hook upfront
**Rationale**: Ousterhout's "write it twice" principle - wait for 2-3 use cases before abstracting
**Future**: If we add optimistic edit/delete/archive, extract shared pattern then

## Test Scenarios

### Happy Path
- [ ] User selects answer, clicks Submit → feedback appears instantly (<16ms)
- [ ] Correct answer shows green border, checkmark icon, success color
- [ ] Incorrect answer shows red border, X icon, error color
- [ ] Screen reader announces "Correct" or "Incorrect"
- [ ] Backend mutation completes within 500ms
- [ ] Feedback section appears with explanation and next review schedule
- [ ] User clicks Next, transitions to new question smoothly

### Error Conditions
- [ ] Backend mutation fails (500 error) → retry 3 times → success
- [ ] Backend mutation fails after 3 retries → toast notification, user can continue
- [ ] Network offline → mutation queued, syncs when online
- [ ] User closes browser with pending mutation → localStorage preserves queue
- [ ] Mutation timeout (>5s) → show "Slow network" indicator, keep retrying

### Edge Cases
- [ ] User clicks Submit multiple times rapidly → only one mutation fires
- [ ] User clicks Next before mutation completes → mutation completes in background
- [ ] Question correctAnswer is null/undefined → graceful degradation, no feedback shown
- [ ] Extremely slow mutation (10s+) → user can continue, mutation completes eventually
- [ ] Browser beforeunload with 5+ pending mutations → warning dialog

### Accessibility
- [ ] ARIA live region announces feedback correctly
- [ ] Screen reader users hear feedback without visual cues
- [ ] Keyboard-only: focus stays on Next button after feedback appears
- [ ] High contrast mode: feedback colors meet WCAG AAA
- [ ] Reduced motion: animations disabled, feedback still clear

### Performance
- [ ] Time-to-feedback: <16ms (measured via Performance API)
- [ ] Animation smoothness: 60fps for checkmark/X scale-in
- [ ] No layout shift when feedback section appears
- [ ] Memory: no leaks from queued mutations (test 1000+ answers)

### Concurrency
- [ ] Two mutations in flight → both complete, no race condition
- [ ] Mutation completes during "Next" transition → state updates correctly
- [ ] Component unmounts before mutation completes → no state updates on unmounted component

## Success Metrics

**Immediate (Phase 1)**:
- Feedback appears in <16ms (measured client-side)
- Zero regression in FSRS tracking accuracy (compare mutation logs)
- User sessions complete 10-15% faster (fewer "did that work?" pauses)

**Short-term (Phase 2, 1 week post-launch)**:
- Mutation success rate >99.9% (with retries)
- Error rate <0.1% reaching user-facing toast
- Zero data loss incidents (all mutations eventually succeed)

**Long-term (3 months)**:
- Review session completion rate +5-10% (better flow = more finished sessions)
- User satisfaction feedback mentions "snappier" or "faster" UI
- Backend can safely increase FSRS calculation time without UX impact

---

## Next Steps

1. **Review this PRD** for any missed requirements or architectural concerns
2. **Run `/plan`** to break down Phase 1 MVP into implementation tasks
3. **After Phase 1**: User test with 5-10 reviewers, gather feedback on perceived speed
4. **Before Phase 2**: Review telemetry from Phase 1 to inform error handling strategy

---

**Architecture Quality Checklist**:
- ✅ **Deep modules**: Simple hooks hide complex retry/progressive-load logic
- ✅ **Information hiding**: UI and FSRS logic fully decoupled
- ✅ **Different abstractions**: Each layer transforms vocabulary meaningfully
- ✅ **Strategic design**: Enables independent evolution of UI and backend
- ✅ **Design twice**: Evaluated 3 approaches, chose best balance
- ✅ **YAGNI**: Deferred batch optimization and offline-first to Phase 3

**Complexity Assessment**: Medium

- **New concepts**: 3 hooks, retry queue, progressive loading
- **Integration points**: 1 (modify `handleSubmit` in `review-flow.tsx`)
- **Lines of code estimate**: ~300 lines (100 per hook)
- **Testing surface**: Moderate (async timing, error conditions)

This is worthwhile complexity because it solves a core UX problem and establishes a pattern for future optimistic interactions.
