import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useInlineEdit } from './use-inline-edit';

describe('useInlineEdit', () => {
  it('should initialize with edit mode off', () => {
    const initialData = { title: 'Original Title', description: 'Original Desc' };
    const { result } = renderHook(() => useInlineEdit(initialData, vi.fn()));

    expect(result.current.isEditing).toBe(false);
    expect(result.current.isSaving).toBe(false);
    expect(result.current.localData).toEqual(initialData);
  });

  it('should start edit mode and copy initial data', () => {
    const initialData = { title: 'Original Title', description: 'Original Desc' };
    const { result } = renderHook(() => useInlineEdit(initialData, vi.fn()));

    act(() => {
      result.current.startEdit();
    });

    expect(result.current.isEditing).toBe(true);
    expect(result.current.localData).toEqual(initialData);
    expect(result.current.localData).not.toBe(initialData); // Should be a copy
  });

  it('should update field optimistically', () => {
    const initialData = { title: 'Original Title', description: 'Original Desc' };
    const { result } = renderHook(() => useInlineEdit(initialData, vi.fn()));

    act(() => {
      result.current.startEdit();
    });

    act(() => {
      result.current.updateField('title', 'Updated Title');
    });

    expect(result.current.localData).toEqual({
      title: 'Updated Title',
      description: 'Original Desc',
    });
  });

  it('should save successfully and exit edit mode', async () => {
    const initialData = { title: 'Original Title', description: 'Original Desc' };
    const onSave = vi.fn().mockResolvedValue(undefined);
    const { result } = renderHook(() => useInlineEdit(initialData, onSave));

    act(() => {
      result.current.startEdit();
    });

    act(() => {
      result.current.updateField('title', 'Updated Title');
    });

    await act(async () => {
      await result.current.save();
    });

    expect(onSave).toHaveBeenCalledWith({
      title: 'Updated Title',
      description: 'Original Desc',
    });
    expect(result.current.isEditing).toBe(false);
    expect(result.current.isSaving).toBe(false);
  });

  it('should set saving state to false after save completes', async () => {
    const initialData = { title: 'Original Title' };
    const onSave = vi.fn().mockResolvedValue(undefined);

    const { result } = renderHook(() => useInlineEdit(initialData, onSave));

    act(() => {
      result.current.startEdit();
    });

    await act(async () => {
      await result.current.save();
    });

    expect(result.current.isSaving).toBe(false);
  });

  it('should cancel edit and revert changes', () => {
    const initialData = { title: 'Original Title', description: 'Original Desc' };
    const { result } = renderHook(() => useInlineEdit(initialData, vi.fn()));

    act(() => {
      result.current.startEdit();
    });

    act(() => {
      result.current.updateField('title', 'Changed Title');
    });

    act(() => {
      result.current.cancel();
    });

    expect(result.current.isEditing).toBe(false);
    expect(result.current.localData).toEqual(initialData);
  });

  it('should handle multiple field updates', () => {
    const initialData = { title: 'Title', description: 'Desc', extra: 'Extra' };
    const { result } = renderHook(() => useInlineEdit(initialData, vi.fn()));

    act(() => {
      result.current.startEdit();
    });

    act(() => {
      result.current.updateField('title', 'New Title');
      result.current.updateField('description', 'New Desc');
    });

    expect(result.current.localData).toEqual({
      title: 'New Title',
      description: 'New Desc',
      extra: 'Extra',
    });
  });

  it('should track dirty state', () => {
    const initialData = { title: 'Original Title' };
    const { result } = renderHook(() => useInlineEdit(initialData, vi.fn()));

    // Not dirty initially
    expect(result.current.isDirty).toBe(false);

    act(() => {
      result.current.startEdit();
    });

    // Still not dirty after starting edit (no changes yet)
    expect(result.current.isDirty).toBe(false);

    act(() => {
      result.current.updateField('title', 'Updated Title');
    });

    // Now dirty after change
    expect(result.current.isDirty).toBe(true);

    // Revert to original value
    act(() => {
      result.current.updateField('title', 'Original Title');
    });

    // Not dirty anymore
    expect(result.current.isDirty).toBe(false);
  });

  it('should sync localData when save returns updated data', async () => {
    const initialData = { title: 'Original Title' };
    const serverUpdatedData = { title: 'Server Updated Title' };
    const onSave = vi.fn().mockResolvedValue(serverUpdatedData);

    const { result } = renderHook(() => useInlineEdit(initialData, onSave));

    act(() => {
      result.current.startEdit();
      result.current.updateField('title', 'Client Updated Title');
    });

    await act(async () => {
      await result.current.save();
    });

    // localData should be updated with server response
    expect(result.current.localData).toEqual(serverUpdatedData);
    expect(result.current.isEditing).toBe(false);
  });

  it('should rollback on save error and rethrow', async () => {
    const initialData = { title: 'Original Title' };
    const saveError = new Error('Save failed');
    const onSave = vi.fn().mockRejectedValue(saveError);

    const { result } = renderHook(() => useInlineEdit(initialData, onSave));

    act(() => {
      result.current.startEdit();
      result.current.updateField('title', 'Updated Title');
    });

    let thrownError: Error | null = null;
    await act(async () => {
      try {
        await result.current.save();
      } catch (error) {
        thrownError = error as Error;
      }
    });

    // Should have rolled back to initial data
    expect(result.current.localData).toEqual(initialData);
    expect(result.current.isEditing).toBe(false);
    expect(result.current.isSaving).toBe(false);
    // Should have rethrown the error
    expect(thrownError).toBe(saveError);
  });
});
