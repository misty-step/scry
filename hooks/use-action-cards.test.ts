import { act, renderHook } from '@testing-library/react';
import { useMutation, useQuery } from 'convex/react';
import { toast } from 'sonner';
import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { useActionCards } from './use-action-cards';

vi.mock('convex/react', () => ({
  useQuery: vi.fn(),
  useMutation: vi.fn(),
}));

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

vi.mock('@/hooks/use-keyboard-shortcuts', () => ({
  useKeyboardShortcuts: vi.fn(),
}));

describe('useActionCards', () => {
  const mockCards = [
    {
      _id: 'card1',
      _creationTime: Date.now(),
      userId: 'user1',
      kind: 'MERGE_CONCEPTS',
      payload: {},
      createdAt: Date.now(),
      expiresAt: undefined,
      resolvedAt: undefined,
      resolution: undefined,
    },
  ];

  let applyMutation: any;
  let rejectMutation: any;

  beforeEach(() => {
    vi.clearAllMocks();
    (useQuery as Mock).mockReturnValue(mockCards);

    applyMutation = vi.fn().mockResolvedValue({});
    rejectMutation = vi.fn().mockResolvedValue({});

    // Mock useMutation to return different spies based on call order
    let mutationCallCount = 0;
    (useMutation as Mock).mockImplementation(() => {
      mutationCallCount++;
      // First call: applyCard, Second call: rejectCard
      return mutationCallCount === 1 ? applyMutation : rejectMutation;
    });
  });

  it('returns cards from query', () => {
    const { result } = renderHook(() => useActionCards());
    expect(result.current.cards).toHaveLength(1);
  });

  it('accepts selected card', async () => {
    const { result } = renderHook(() => useActionCards());
    await act(async () => {
      await result.current.acceptSelected();
    });
    expect(applyMutation).toHaveBeenCalledWith({ actionCardId: 'card1' });
    expect(toast.success).toHaveBeenCalledWith('Action applied');
  });

  it('rejects selected card', async () => {
    const { result } = renderHook(() => useActionCards());
    await act(async () => {
      await result.current.rejectSelected();
    });
    expect(rejectMutation).toHaveBeenCalledWith({ actionCardId: 'card1' });
    expect(toast.success).toHaveBeenCalledWith('Action rejected');
  });

  it('handles accept error with Error instance', async () => {
    applyMutation.mockRejectedValue(new Error('Apply failed'));

    const { result } = renderHook(() => useActionCards());
    await act(async () => {
      await result.current.acceptSelected();
    });

    expect(toast.error).toHaveBeenCalledWith('Apply failed');
  });

  it('handles accept error with non-Error', async () => {
    applyMutation.mockRejectedValue('string error');

    const { result } = renderHook(() => useActionCards());
    await act(async () => {
      await result.current.acceptSelected();
    });

    expect(toast.error).toHaveBeenCalledWith('Failed to apply action');
  });

  it('handles reject error with Error instance', async () => {
    rejectMutation.mockRejectedValue(new Error('Reject failed'));

    const { result } = renderHook(() => useActionCards());
    await act(async () => {
      await result.current.rejectSelected();
    });

    expect(toast.error).toHaveBeenCalledWith('Reject failed');
  });

  it('handles reject error with non-Error', async () => {
    rejectMutation.mockRejectedValue('string error');

    const { result } = renderHook(() => useActionCards());
    await act(async () => {
      await result.current.rejectSelected();
    });

    expect(toast.error).toHaveBeenCalledWith('Failed to reject action');
  });

  it('does nothing when accepting with no selected card', async () => {
    (useQuery as Mock).mockReturnValue([]);

    const { result } = renderHook(() => useActionCards());
    await act(async () => {
      await result.current.acceptSelected();
    });

    expect(applyMutation).not.toHaveBeenCalled();
  });

  it('does nothing when rejecting with no selected card', async () => {
    (useQuery as Mock).mockReturnValue([]);

    const { result } = renderHook(() => useActionCards());
    await act(async () => {
      await result.current.rejectSelected();
    });

    expect(rejectMutation).not.toHaveBeenCalled();
  });

  it('returns -1 for selectedIndex when no cards', () => {
    (useQuery as Mock).mockReturnValue([]);

    const { result } = renderHook(() => useActionCards());
    expect(result.current.selectedIndex).toBe(-1);
  });

  it('wraps around when selecting beyond bounds', () => {
    const multiCards = [{ _id: 'card1' }, { _id: 'card2' }, { _id: 'card3' }];
    (useQuery as Mock).mockReturnValue(multiCards);

    const { result } = renderHook(() => useActionCards());

    act(() => {
      result.current.selectOffset(-1); // Should wrap to end
    });
    expect(result.current.selectedIndex).toBe(2);

    act(() => {
      result.current.selectOffset(1); // Should wrap to start
    });
    expect(result.current.selectedIndex).toBe(0);
  });

  it('shows loading state when query is undefined', () => {
    (useQuery as Mock).mockReturnValue(undefined);

    const { result } = renderHook(() => useActionCards());
    expect(result.current.isLoading).toBe(true);
    expect(result.current.cards).toEqual([]);
  });

  it('allows setting selected index directly', () => {
    const multiCards = [{ _id: 'card1' }, { _id: 'card2' }];
    (useQuery as Mock).mockReturnValue(multiCards);

    const { result } = renderHook(() => useActionCards());

    act(() => {
      result.current.setSelectedIndex(1);
    });
    expect(result.current.selectedIndex).toBe(1);
  });
});
