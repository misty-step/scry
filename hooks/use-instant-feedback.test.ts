import { act, renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { useInstantFeedback } from './use-instant-feedback';

describe('useInstantFeedback', () => {
  it('should have initial state with visible=false and isCorrect=false', () => {
    const { result } = renderHook(() => useInstantFeedback());

    expect(result.current.feedbackState).toEqual({
      isCorrect: false,
      visible: false,
    });
  });

  it('should show correct feedback when showFeedback(true) is called', () => {
    const { result } = renderHook(() => useInstantFeedback());

    act(() => {
      result.current.showFeedback(true);
    });

    expect(result.current.feedbackState).toEqual({
      isCorrect: true,
      visible: true,
    });
  });

  it('should show incorrect feedback when showFeedback(false) is called', () => {
    const { result } = renderHook(() => useInstantFeedback());

    act(() => {
      result.current.showFeedback(false);
    });

    expect(result.current.feedbackState).toEqual({
      isCorrect: false,
      visible: true,
    });
  });

  it('should clear feedback when clearFeedback is called', () => {
    const { result } = renderHook(() => useInstantFeedback());

    // First show feedback
    act(() => {
      result.current.showFeedback(true);
    });

    expect(result.current.feedbackState.visible).toBe(true);

    // Then clear it
    act(() => {
      result.current.clearFeedback();
    });

    expect(result.current.feedbackState).toEqual({
      isCorrect: false,
      visible: false,
    });
  });

  it('should allow multiple showFeedback calls where last call wins', () => {
    const { result } = renderHook(() => useInstantFeedback());

    // First show incorrect
    act(() => {
      result.current.showFeedback(false);
    });

    expect(result.current.feedbackState.isCorrect).toBe(false);

    // Then show correct
    act(() => {
      result.current.showFeedback(true);
    });

    expect(result.current.feedbackState).toEqual({
      isCorrect: true,
      visible: true,
    });
  });

  it('should maintain stable function references', () => {
    const { result, rerender } = renderHook(() => useInstantFeedback());

    const firstShowFeedback = result.current.showFeedback;
    const firstClearFeedback = result.current.clearFeedback;

    // Trigger re-render
    rerender();

    // Functions should be the same reference (useCallback stability)
    expect(result.current.showFeedback).toBe(firstShowFeedback);
    expect(result.current.clearFeedback).toBe(firstClearFeedback);
  });
});
