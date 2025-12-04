'use client';

import { useCallback, useEffect, useReducer, useRef } from 'react';
import { api } from '@/convex/_generated/api';
import type { Doc, Id } from '@/convex/_generated/dataModel';
import { useDataHash } from '@/hooks/use-data-hash';
import { useSimplePoll } from '@/hooks/use-simple-poll';
import { useTrackEvent } from '@/hooks/use-track-event';
import { LOADING_TIMEOUT_MS, POLLING_INTERVAL_MS } from '@/lib/constants/timing';

type SimpleQuestion = {
  question: string;
  type?: 'multiple-choice' | 'true-false' | 'cloze' | 'short-answer';
  options: string[];
  correctAnswer: string;
  explanation?: string;
};

// State machine definition
interface ReviewModeState {
  phase: 'loading' | 'empty' | 'reviewing' | 'error';
  question: SimpleQuestion | null;
  interactions: Doc<'interactions'>[];
  conceptId: Id<'concepts'> | null;
  conceptTitle: string | null;
  phrasingId: Id<'phrasings'> | null;
  phrasingIndex: number | null;
  totalPhrasings: number | null;
  selectionReason: string | null;
  lockId: string | null;
  isTransitioning: boolean;
  errorMessage?: string;
  conceptFsrs: {
    state?: 'new' | 'learning' | 'review' | 'relearning';
    reps?: number;
  } | null;
  // Skip feature: session-scoped, client-side only
  skippedConceptIds: Set<Id<'concepts'>>;
  lastAutoSkippedId: Id<'concepts'> | null;
}

// Action types for state machine
type ReviewAction =
  | { type: 'LOAD_START' }
  | { type: 'LOAD_EMPTY' }
  | { type: 'LOAD_TIMEOUT' }
  | {
      type: 'QUESTION_RECEIVED';
      payload: {
        question: SimpleQuestion;
        interactions: Doc<'interactions'>[];
        conceptId: Id<'concepts'>;
        conceptTitle: string;
        phrasingId: Id<'phrasings'>;
        phrasingStats: { index: number; total: number } | null;
        selectionReason: string | null;
        lockId: string;
        conceptFsrs: {
          state?: 'new' | 'learning' | 'review' | 'relearning';
          reps?: number;
        };
      };
    }
  | { type: 'REVIEW_COMPLETE' }
  | { type: 'IGNORE_UPDATE'; reason: string }
  // Skip feature actions
  | { type: 'SKIP_CONCEPT'; payload: Id<'concepts'> }
  | { type: 'AUTO_SKIP'; payload: { conceptId: Id<'concepts'>; reason: string } }
  | { type: 'CLEAR_SKIPPED' };

// Initial state
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
  skippedConceptIds: new Set(),
  lastAutoSkippedId: null,
};

// Reducer function to manage state transitions
export function reviewReducer(state: ReviewModeState, action: ReviewAction): ReviewModeState {
  switch (action.type) {
    case 'LOAD_START':
      return { ...state, phase: 'loading', isTransitioning: false };

    case 'LOAD_EMPTY':
      return {
        ...state,
        phase: 'empty',
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
        errorMessage: undefined,
        conceptFsrs: null,
        // Clear skip state on session end
        skippedConceptIds: new Set(),
        lastAutoSkippedId: null,
      };

    case 'LOAD_TIMEOUT':
      return {
        ...state,
        phase: 'error',
        errorMessage:
          'Loading is taking longer than expected. Please refresh the page to try again.',
      };

    case 'QUESTION_RECEIVED':
      return {
        phase: 'reviewing',
        question: action.payload.question,
        interactions: action.payload.interactions,
        conceptId: action.payload.conceptId,
        conceptTitle: action.payload.conceptTitle,
        phrasingId: action.payload.phrasingId,
        phrasingIndex: action.payload.phrasingStats?.index ?? null,
        totalPhrasings: action.payload.phrasingStats?.total ?? null,
        selectionReason: action.payload.selectionReason,
        lockId: action.payload.lockId,
        isTransitioning: false, // Clear transitioning state
        errorMessage: undefined,
        conceptFsrs: action.payload.conceptFsrs,
        // Preserve skip set across questions; reset auto-skip tracker
        skippedConceptIds: state.skippedConceptIds,
        lastAutoSkippedId: null,
      };

    case 'REVIEW_COMPLETE':
      // Optimistic UI: Keep current question visible during transition
      // This prevents flicker by maintaining layout stability while next question loads
      // Only release lock to allow new question to load
      return {
        ...state,
        phase: 'reviewing', // Stay in reviewing phase
        lockId: null, // Release lock so new question can load
        isTransitioning: true, // Mark as transitioning for UI feedback
      };

    case 'IGNORE_UPDATE':
      // No state change
      return state;

    // Skip feature: move concept to end of session queue
    case 'SKIP_CONCEPT':
      return {
        ...state,
        skippedConceptIds: new Set([...state.skippedConceptIds, action.payload]),
        lockId: null, // Release lock to fetch next
        isTransitioning: true,
        // Don't reset lastAutoSkippedId—let AUTO_SKIP handle queue exhaustion detection
      };

    case 'AUTO_SKIP':
      // Track auto-skipped concept for queue exhaustion detection
      return {
        ...state,
        lastAutoSkippedId: action.payload.conceptId,
        lockId: null,
        isTransitioning: true,
      };

    case 'CLEAR_SKIPPED':
      // Queue exhausted to only skipped concepts—clear and cycle through
      return {
        ...state,
        skippedConceptIds: new Set(),
        lastAutoSkippedId: null,
      };

    default:
      return state;
  }
}

function generateSessionId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }

  return `review-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

/**
 * Custom hook that encapsulates all review flow business logic
 * Separates data fetching, state management, and event handling from presentation
 *
 * @returns Object containing review state and handlers
 */
export function useReviewFlow() {
  const trackEvent = useTrackEvent();
  // Single state machine instead of 6 separate state variables
  const [state, dispatch] = useReducer(reviewReducer, initialState);

  // Use ref to track the last seen question ID
  const lastCandidateKeyRef = useRef<string | null>(null);

  // Use ref for loading timeout
  const loadingTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const sessionStartRef = useRef<number | null>(null);
  const questionsReviewedRef = useRef(0);

  // Query - use simple polling for time-sensitive review queries
  const { data: nextReview } = useSimplePoll(api.concepts.getDue, {}, POLLING_INTERVAL_MS);

  // Check if data has actually changed to prevent unnecessary renders
  const { hasChanged: dataHasChanged } = useDataHash(nextReview);

  const resetSessionMetrics = useCallback(() => {
    sessionIdRef.current = null;
    sessionStartRef.current = null;
    questionsReviewedRef.current = 0;
  }, []);

  const startSession = useCallback(() => {
    if (sessionIdRef.current) {
      return;
    }

    const sessionId = generateSessionId();
    sessionIdRef.current = sessionId;
    sessionStartRef.current = Date.now();
    questionsReviewedRef.current = 0;

    trackEvent('Review Session Started', {
      sessionId,
      questionsReviewed: 0,
      durationMs: 0,
    });
  }, [trackEvent]);

  const completeSession = useCallback(() => {
    if (!sessionIdRef.current || sessionStartRef.current === null) {
      return;
    }

    const durationMs = Date.now() - sessionStartRef.current;

    trackEvent('Review Session Completed', {
      sessionId: sessionIdRef.current,
      questionsReviewed: questionsReviewedRef.current,
      durationMs,
    });

    resetSessionMetrics();
  }, [resetSessionMetrics, trackEvent]);

  const abandonSession = useCallback(() => {
    if (!sessionIdRef.current || sessionStartRef.current === null) {
      return;
    }

    const durationMs = Date.now() - sessionStartRef.current;

    trackEvent('Review Session Abandoned', {
      sessionId: sessionIdRef.current,
      questionsReviewed: questionsReviewedRef.current,
      durationMs,
    });

    resetSessionMetrics();
  }, [resetSessionMetrics, trackEvent]);

  // Set up loading timeout (5 seconds)
  // Triggers for both initial loading and optimistic transitions
  useEffect(() => {
    if (state.phase === 'loading' || state.isTransitioning) {
      // Clear any existing timeout
      if (loadingTimeoutRef.current) {
        clearTimeout(loadingTimeoutRef.current);
      }

      // Set new timeout
      loadingTimeoutRef.current = setTimeout(() => {
        dispatch({ type: 'LOAD_TIMEOUT' });
      }, LOADING_TIMEOUT_MS);
    } else {
      // Clear timeout when not loading/transitioning
      if (loadingTimeoutRef.current) {
        clearTimeout(loadingTimeoutRef.current);
        loadingTimeoutRef.current = null;
      }
    }

    return () => {
      // Cleanup on unmount
      if (loadingTimeoutRef.current) {
        clearTimeout(loadingTimeoutRef.current);
      }
    };
  }, [state.phase, state.isTransitioning]);

  // Process polling data and update state
  useEffect(() => {
    if (!dataHasChanged && state.phase !== 'loading' && !state.isTransitioning) {
      dispatch({ type: 'IGNORE_UPDATE', reason: 'Data unchanged from previous poll' });
      return;
    }

    if (state.lockId) {
      dispatch({ type: 'IGNORE_UPDATE', reason: 'User is actively reviewing' });
      return;
    }

    if (nextReview === undefined) {
      if (state.phase === 'loading' && !lastCandidateKeyRef.current) {
        dispatch({ type: 'LOAD_START' });
      }
      return;
    }

    if (nextReview === null) {
      dispatch({ type: 'LOAD_EMPTY' });
      return;
    }

    const conceptId = nextReview.concept._id;

    // Skip filtering: check if this concept should be auto-skipped
    if (state.skippedConceptIds.has(conceptId)) {
      if (state.lastAutoSkippedId === conceptId && dataHasChanged) {
        // Queue exhausted: NEW poll data still returns same skipped concept—clear and show
        dispatch({ type: 'CLEAR_SKIPPED' });
        // Fall through to display this concept
      } else {
        // Auto-skip and wait for new poll data
        dispatch({ type: 'AUTO_SKIP', payload: { conceptId, reason: 'in_skip_set' } });
        return;
      }
    }

    const conceptKey = `${conceptId}:${nextReview.phrasing._id}`;
    const isNewQuestion = conceptKey !== lastCandidateKeyRef.current;

    const question: SimpleQuestion = {
      question: nextReview.phrasing.question,
      options: nextReview.phrasing.options || [],
      correctAnswer: nextReview.phrasing.correctAnswer || '',
      explanation: nextReview.phrasing.explanation || '',
      type: nextReview.phrasing.type || 'multiple-choice',
    };

    if (isNewQuestion) {
      if (process.env.NODE_ENV === 'development') {
        performance.mark('review-question-loaded');
      }

      const lockId = `${conceptKey}-${Date.now()}`;
      dispatch({
        type: 'QUESTION_RECEIVED',
        payload: {
          question,
          interactions: nextReview.interactions || [],
          conceptId: nextReview.concept._id,
          conceptTitle: nextReview.concept.title,
          phrasingId: nextReview.phrasing._id,
          phrasingStats: nextReview.phrasingStats ?? null,
          selectionReason: nextReview.selectionReason ?? null,
          lockId,
          conceptFsrs: {
            state: nextReview.concept.fsrs.state,
            reps: nextReview.concept.fsrs.reps,
          },
        },
      });
      startSession();
      lastCandidateKeyRef.current = conceptKey;
    } else if (state.isTransitioning) {
      const lockId = `${conceptKey}-${Date.now()}`;
      dispatch({
        type: 'QUESTION_RECEIVED',
        payload: {
          question,
          interactions: nextReview.interactions || [],
          conceptId: nextReview.concept._id,
          conceptTitle: nextReview.concept.title,
          phrasingId: nextReview.phrasing._id,
          phrasingStats: nextReview.phrasingStats ?? null,
          selectionReason: nextReview.selectionReason ?? null,
          lockId,
          conceptFsrs: {
            state: nextReview.concept.fsrs.state,
            reps: nextReview.concept.fsrs.reps,
          },
        },
      });
      startSession();
    } else {
      dispatch({
        type: 'IGNORE_UPDATE',
        reason: 'Poll executed but data unchanged - same concept/phrasing combination',
      });
    }
  }, [
    nextReview,
    state.lockId,
    state.phase,
    state.isTransitioning,
    state.skippedConceptIds,
    state.lastAutoSkippedId,
    dataHasChanged,
    startSession,
  ]);

  // Memoized handler for review completion
  const handleReviewComplete = useCallback(async () => {
    questionsReviewedRef.current += 1;
    // Release lock and reset state for clean transition to next question
    dispatch({ type: 'REVIEW_COMPLETE' });
    // Intentional loading state provides visual feedback during transitions,
    // especially important for FSRS immediate re-review of incorrect answers
  }, []);

  // Skip handler: move current concept to end of session queue
  const handleSkipConcept = useCallback(() => {
    if (!state.conceptId) return;
    if (state.isTransitioning) return;
    dispatch({ type: 'SKIP_CONCEPT', payload: state.conceptId });
  }, [state.conceptId, state.isTransitioning]);

  useEffect(() => {
    if (state.phase === 'empty') {
      completeSession();
    } else if (state.phase === 'error') {
      abandonSession();
    }
  }, [state.phase, abandonSession, completeSession]);

  useEffect(() => {
    return () => {
      abandonSession();
    };
  }, [abandonSession]);

  // Return stable object with all necessary data and handlers
  return {
    phase: state.phase,
    question: state.question,
    conceptId: state.conceptId,
    conceptTitle: state.conceptTitle,
    phrasingId: state.phrasingId,
    phrasingIndex: state.phrasingIndex,
    totalPhrasings: state.totalPhrasings,
    selectionReason: state.selectionReason,
    interactions: state.interactions,
    isTransitioning: state.isTransitioning,
    errorMessage: state.errorMessage,
    conceptFsrs: state.conceptFsrs,
    skippedCount: state.skippedConceptIds.size,
    handlers: {
      onReviewComplete: handleReviewComplete,
      onSkipConcept: handleSkipConcept,
    },
  };
}
