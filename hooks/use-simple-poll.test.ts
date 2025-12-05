import { act, renderHook } from '@testing-library/react';
import { useQuery } from 'convex/react';
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { useSimplePoll } from './use-simple-poll';

// Mock the Convex useQuery hook
vi.mock('convex/react', () => ({
  useQuery: vi.fn(),
}));

describe('useSimplePoll', () => {
  const mockQuery = { _functionPath: 'test:query' } as any;

  beforeEach(() => {
    vi.useFakeTimers();
    (useQuery as Mock).mockReturnValue(undefined);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it('returns data, isLoading, and refetch', () => {
    const { result } = renderHook(() => useSimplePoll(mockQuery, {}, 1000));

    expect(result.current).toHaveProperty('data');
    expect(result.current).toHaveProperty('isLoading');
    expect(result.current).toHaveProperty('refetch');
    expect(typeof result.current.refetch).toBe('function');
  });

  it('isLoading is true when data is undefined and not skipped', () => {
    (useQuery as Mock).mockReturnValue(undefined);
    const { result } = renderHook(() => useSimplePoll(mockQuery, { someArg: 1 }, 1000));

    expect(result.current.isLoading).toBe(true);
  });

  it('isLoading is false when data is defined', () => {
    (useQuery as Mock).mockReturnValue({ value: 42 });
    const { result } = renderHook(() => useSimplePoll(mockQuery, {}, 1000));

    expect(result.current.isLoading).toBe(false);
    expect(result.current.data).toEqual({ value: 42 });
  });

  it('isLoading is false when args is "skip"', () => {
    (useQuery as Mock).mockReturnValue(undefined);
    const { result } = renderHook(() => useSimplePoll(mockQuery, 'skip', 1000));

    expect(result.current.isLoading).toBe(false);
  });

  it('passes "skip" to useQuery when args is "skip"', () => {
    renderHook(() => useSimplePoll(mockQuery, 'skip', 1000));

    expect(useQuery).toHaveBeenCalledWith(mockQuery, 'skip');
  });

  it('passes args with _refreshTimestamp to useQuery', () => {
    const args = { filter: 'active' };
    renderHook(() => useSimplePoll(mockQuery, args, 1000));

    expect(useQuery).toHaveBeenCalledWith(
      mockQuery,
      expect.objectContaining({
        filter: 'active',
        _refreshTimestamp: expect.any(Number),
      })
    );
  });

  it('refetch increments refreshCount causing useQuery to be called with new timestamp', () => {
    const { result, rerender } = renderHook(() => useSimplePoll(mockQuery, {}, 1000));

    const initialCalls = (useQuery as Mock).mock.calls.length;

    act(() => {
      result.current.refetch();
    });

    rerender();

    // After refetch, useQuery should be called again with updated _refreshTimestamp
    expect((useQuery as Mock).mock.calls.length).toBeGreaterThan(initialCalls);
  });

  it('does not set up interval when intervalMs is 0', () => {
    const { result } = renderHook(() => useSimplePoll(mockQuery, {}, 0));

    // Advance timers - should not cause issues
    act(() => {
      vi.advanceTimersByTime(5000);
    });

    expect(result.current.data).toBeUndefined();
  });

  it('does not set up interval when args is "skip"', () => {
    renderHook(() => useSimplePoll(mockQuery, 'skip', 1000));

    const initialCalls = (useQuery as Mock).mock.calls.length;

    // Advance past the interval
    act(() => {
      vi.advanceTimersByTime(2000);
    });

    // No additional refetches should have happened (polling disabled)
    // The hook shouldn't crash
    expect((useQuery as Mock).mock.calls.length).toBe(initialCalls);
  });

  it('cleans up interval on unmount', () => {
    const { unmount } = renderHook(() => useSimplePoll(mockQuery, {}, 1000));

    unmount();

    // Advancing timers after unmount should not cause errors
    act(() => {
      vi.advanceTimersByTime(5000);
    });
  });

  it('provides stable refetch function reference', () => {
    const { result, rerender } = renderHook(() => useSimplePoll(mockQuery, {}, 1000));

    const firstRefetch = result.current.refetch;
    rerender();
    const secondRefetch = result.current.refetch;

    expect(firstRefetch).toBe(secondRefetch);
  });

  it('handles changing intervalMs', () => {
    const { rerender } = renderHook(({ interval }) => useSimplePoll(mockQuery, {}, interval), {
      initialProps: { interval: 1000 },
    });

    // Change interval
    rerender({ interval: 2000 });

    // Should not crash
    act(() => {
      vi.advanceTimersByTime(3000);
    });
  });

  it('handles changing from skip to real args', () => {
    const { result, rerender } = renderHook<{ data: any; isLoading: boolean }, { args: any }>(
      ({ args }) => useSimplePoll(mockQuery, args, 1000),
      {
        initialProps: { args: 'skip' },
      }
    );

    expect(result.current.isLoading).toBe(false);

    // Change to real args
    rerender({ args: { filter: 'all' } });

    expect(useQuery).toHaveBeenLastCalledWith(
      mockQuery,
      expect.objectContaining({ filter: 'all' })
    );
  });
});
