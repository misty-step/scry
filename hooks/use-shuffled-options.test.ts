import { renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useShuffledOptions } from './use-shuffled-options';

// Mock the shuffle function to control randomness in tests
vi.mock('@/lib/utils/shuffle', () => ({
  shuffle: vi.fn((arr: string[]) => [...arr].reverse()), // Predictable: reverse the array
}));

describe('useShuffledOptions', () => {
  it('returns shuffled array', () => {
    const { result } = renderHook(() => useShuffledOptions(['A', 'B', 'C', 'D']));

    // With our mock, shuffle reverses the array
    expect(result.current).toEqual(['D', 'C', 'B', 'A']);
  });

  it('returns empty array for empty input', () => {
    const { result } = renderHook(() => useShuffledOptions([]));
    expect(result.current).toEqual([]);
  });

  it('returns single-element array unchanged', () => {
    const { result } = renderHook(() => useShuffledOptions(['Only']));
    expect(result.current).toEqual(['Only']);
  });

  it('memoizes result for same input array reference', () => {
    const options = ['A', 'B', 'C'];
    const { result, rerender } = renderHook(({ opts }) => useShuffledOptions(opts), {
      initialProps: { opts: options },
    });

    const firstResult = result.current;

    // Rerender with same array reference
    rerender({ opts: options });

    expect(result.current).toBe(firstResult);
  });

  it('recalculates when input array reference changes', () => {
    const { result, rerender } = renderHook(({ opts }) => useShuffledOptions(opts), {
      initialProps: { opts: ['A', 'B'] },
    });

    const firstResult = result.current;

    // Rerender with new array reference (same content but different object)
    rerender({ opts: ['A', 'B'] });

    // useMemo dependency is the array reference, so new array = new shuffle
    expect(result.current).not.toBe(firstResult);
    expect(result.current).toEqual(['B', 'A']); // Reversed by our mock
  });

  it('handles array with duplicate values', () => {
    const { result } = renderHook(() => useShuffledOptions(['A', 'A', 'B', 'B']));
    expect(result.current).toEqual(['B', 'B', 'A', 'A']);
  });
});
