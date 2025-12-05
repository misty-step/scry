import { act, renderHook } from '@testing-library/react';
import { toast } from 'sonner';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useUndoableAction } from './use-undoable-action';

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

describe('useUndoableAction', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns a function', () => {
    const { result } = renderHook(() => useUndoableAction());
    expect(typeof result.current).toBe('function');
  });

  it('executes action and shows success toast with undo button', async () => {
    const { result } = renderHook(() => useUndoableAction());
    const mockAction = vi.fn().mockResolvedValue(undefined);
    const mockUndo = vi.fn().mockResolvedValue(undefined);

    await act(async () => {
      await result.current({
        action: mockAction,
        message: 'Item archived',
        undo: mockUndo,
      });
    });

    expect(mockAction).toHaveBeenCalledTimes(1);
    expect(toast.success).toHaveBeenCalledWith('Item archived', {
      action: expect.objectContaining({
        label: 'Undo',
        onClick: expect.any(Function),
      }),
      duration: 5000,
    });
  });

  it('uses custom duration when provided', async () => {
    const { result } = renderHook(() => useUndoableAction());
    const mockAction = vi.fn().mockResolvedValue(undefined);
    const mockUndo = vi.fn().mockResolvedValue(undefined);

    await act(async () => {
      await result.current({
        action: mockAction,
        message: 'Done',
        undo: mockUndo,
        duration: 3000,
      });
    });

    expect(toast.success).toHaveBeenCalledWith('Done', {
      action: expect.any(Object),
      duration: 3000,
    });
  });

  it('shows error toast when action fails', async () => {
    const { result } = renderHook(() => useUndoableAction());
    const actionError = new Error('Network failed');
    const mockAction = vi.fn().mockRejectedValue(actionError);
    const mockUndo = vi.fn().mockResolvedValue(undefined);

    await expect(
      act(async () => {
        await result.current({
          action: mockAction,
          message: 'Should not see this',
          undo: mockUndo,
        });
      })
    ).rejects.toThrow('Network failed');

    expect(toast.error).toHaveBeenCalledWith('Action failed', {
      description: 'Network failed',
    });
    expect(toast.success).not.toHaveBeenCalled();
  });

  it('uses custom error message when action fails', async () => {
    const { result } = renderHook(() => useUndoableAction());
    const mockAction = vi.fn().mockRejectedValue(new Error('Oops'));
    const mockUndo = vi.fn().mockResolvedValue(undefined);

    await expect(
      act(async () => {
        await result.current({
          action: mockAction,
          message: 'Success',
          undo: mockUndo,
          errorMessage: 'Could not archive item',
        });
      })
    ).rejects.toThrow();

    expect(toast.error).toHaveBeenCalledWith('Could not archive item', {
      description: 'Oops',
    });
  });

  it('handles non-Error objects in action failure', async () => {
    const { result } = renderHook(() => useUndoableAction());
    const mockAction = vi.fn().mockRejectedValue('string error');
    const mockUndo = vi.fn().mockResolvedValue(undefined);

    await expect(
      act(async () => {
        await result.current({
          action: mockAction,
          message: 'Success',
          undo: mockUndo,
        });
      })
    ).rejects.toBe('string error');

    expect(toast.error).toHaveBeenCalledWith('Action failed', {
      description: undefined,
    });
  });

  describe('undo callback behavior', () => {
    it('undo callback shows success toast on success', async () => {
      const { result } = renderHook(() => useUndoableAction());
      const mockAction = vi.fn().mockResolvedValue(undefined);
      const mockUndo = vi.fn().mockResolvedValue(undefined);

      await act(async () => {
        await result.current({
          action: mockAction,
          message: 'Archived',
          undo: mockUndo,
        });
      });

      // Get the onClick callback that was passed to toast.success
      const successCall = (toast.success as any).mock.calls[0];
      const undoCallback = successCall[1].action.onClick;

      // Clear mocks before testing undo
      vi.clearAllMocks();

      // Execute the undo callback
      await act(async () => {
        await undoCallback();
      });

      expect(mockUndo).toHaveBeenCalledTimes(1);
      expect(toast.success).toHaveBeenCalledWith('Action undone');
    });

    it('undo callback shows error toast on failure', async () => {
      const { result } = renderHook(() => useUndoableAction());
      const mockAction = vi.fn().mockResolvedValue(undefined);
      const undoError = new Error('Undo failed');
      const mockUndo = vi.fn().mockRejectedValue(undoError);

      await act(async () => {
        await result.current({
          action: mockAction,
          message: 'Archived',
          undo: mockUndo,
        });
      });

      const successCall = (toast.success as any).mock.calls[0];
      const undoCallback = successCall[1].action.onClick;

      vi.clearAllMocks();

      await act(async () => {
        await undoCallback();
      });

      expect(mockUndo).toHaveBeenCalledTimes(1);
      expect(toast.error).toHaveBeenCalledWith('Failed to undo action', {
        description: 'Undo failed',
      });
    });

    it('undo callback uses custom undo error message', async () => {
      const { result } = renderHook(() => useUndoableAction());
      const mockAction = vi.fn().mockResolvedValue(undefined);
      const mockUndo = vi.fn().mockRejectedValue(new Error('DB error'));

      await act(async () => {
        await result.current({
          action: mockAction,
          message: 'Deleted',
          undo: mockUndo,
          undoErrorMessage: 'Could not restore item',
        });
      });

      const successCall = (toast.success as any).mock.calls[0];
      const undoCallback = successCall[1].action.onClick;

      vi.clearAllMocks();

      await act(async () => {
        await undoCallback();
      });

      expect(toast.error).toHaveBeenCalledWith('Could not restore item', {
        description: 'DB error',
      });
    });

    it('undo callback handles non-Error objects', async () => {
      const { result } = renderHook(() => useUndoableAction());
      const mockAction = vi.fn().mockResolvedValue(undefined);
      const mockUndo = vi.fn().mockRejectedValue('plain string error');

      await act(async () => {
        await result.current({
          action: mockAction,
          message: 'Done',
          undo: mockUndo,
        });
      });

      const successCall = (toast.success as any).mock.calls[0];
      const undoCallback = successCall[1].action.onClick;

      vi.clearAllMocks();

      await act(async () => {
        await undoCallback();
      });

      expect(toast.error).toHaveBeenCalledWith('Failed to undo action', {
        description: undefined,
      });
    });
  });
});
