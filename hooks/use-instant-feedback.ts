'use client';

import { useCallback, useState } from 'react';

export interface InstantFeedbackState {
  isCorrect: boolean;
  visible: boolean;
}

/**
 * Hook for managing instant answer feedback state
 * Provides immediate visual reinforcement without waiting for backend
 *
 * This is a deep module: simple interface hides state management complexity
 * Interface = showFeedback() + clearFeedback()
 * Hidden = useState, useCallback, animation timing coordination
 */
export function useInstantFeedback() {
  const [feedbackState, setFeedbackState] = useState<InstantFeedbackState>({
    isCorrect: false,
    visible: false,
  });

  /**
   * Show instant feedback for answer correctness
   * Synchronous - updates state immediately for instant visual response
   */
  const showFeedback = useCallback((isCorrect: boolean) => {
    setFeedbackState({
      isCorrect,
      visible: true,
    });
  }, []);

  /**
   * Clear feedback state when transitioning to next question
   */
  const clearFeedback = useCallback(() => {
    setFeedbackState({
      isCorrect: false,
      visible: false,
    });
  }, []);

  return {
    feedbackState,
    showFeedback,
    clearFeedback,
  };
}
