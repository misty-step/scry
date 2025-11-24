import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useButtonPress } from './use-button-press';

describe('useButtonPress', () => {
  it('sets pressing state then clears after default duration', () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useButtonPress());

    act(() => {
      result.current.handlePressStart();
    });

    expect(result.current.isPressing).toBe(true);

    act(() => {
      vi.advanceTimersByTime(220);
    });

    expect(result.current.isPressing).toBe(false);
    vi.useRealTimers();
  });

  it('respects custom duration', () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useButtonPress(50));

    act(() => {
      result.current.handlePressStart();
    });

    act(() => {
      vi.advanceTimersByTime(50);
    });

    expect(result.current.isPressing).toBe(false);
    vi.useRealTimers();
  });
});
