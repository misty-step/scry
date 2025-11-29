import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { UnifiedEditData } from '@/lib/unified-edit-validation';
import { useUnifiedEdit } from './use-unified-edit';

describe('useUnifiedEdit', () => {
  const createTestData = (overrides?: Partial<UnifiedEditData>): UnifiedEditData => ({
    conceptTitle: 'Test Concept',
    question: 'What is 2 + 2?',
    correctAnswer: '4',
    explanation: 'Two plus two equals four',
    options: ['3', '4', '5', '6'],
    ...overrides,
  });

  let mockSaveConcept: any;
  let mockSavePhrasing: any;

  beforeEach(() => {
    mockSaveConcept = vi.fn().mockResolvedValue(undefined);
    mockSavePhrasing = vi.fn().mockResolvedValue(undefined);
  });

  describe('Initial State', () => {
    it('should initialize with edit mode off', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      expect(result.current.isEditing).toBe(false);
      expect(result.current.isSaving).toBe(false);
      expect(result.current.isDirty).toBe(false);
    });

    it('should initialize localData with provided initialData', () => {
      const testData = createTestData({ conceptTitle: 'Custom Title' });
      const { result } = renderHook(() =>
        useUnifiedEdit(testData, mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      expect(result.current.localData).toEqual(testData);
    });

    it('should have no errors initially', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      expect(result.current.errors).toEqual({});
    });
  });

  describe('Edit Mode Management', () => {
    it('should enter edit mode when startEdit is called', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
      });

      expect(result.current.isEditing).toBe(true);
    });

    it('should clear errors when startEdit is called', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      // Manually set errors (simulating previous save failure)
      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', '');
      });

      // Start edit again
      act(() => {
        result.current.startEdit();
      });

      expect(result.current.errors).toEqual({});
    });

    it('should exit edit mode when cancel is called', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
      });

      act(() => {
        result.current.cancel();
      });

      expect(result.current.isEditing).toBe(false);
    });

    it('should revert localData to initialData when cancel is called', () => {
      const initialData = createTestData();
      const { result } = renderHook(() =>
        useUnifiedEdit(initialData, mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', 'Changed Title');
        result.current.updateField('question', 'Changed Question');
      });

      act(() => {
        result.current.cancel();
      });

      expect(result.current.localData).toEqual(initialData);
    });
  });

  describe('Dirty Detection', () => {
    it('should detect concept as dirty when title changes', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', 'New Title');
      });

      expect(result.current.conceptIsDirty).toBe(true);
      expect(result.current.phrasingIsDirty).toBe(false);
      expect(result.current.isDirty).toBe(true);
    });

    it('should detect phrasing as dirty when question changes', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('question', 'New Question');
      });

      expect(result.current.conceptIsDirty).toBe(false);
      expect(result.current.phrasingIsDirty).toBe(true);
      expect(result.current.isDirty).toBe(true);
    });

    it('should detect phrasing as dirty when options change', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('options', ['a', 'b', 'c']);
      });

      expect(result.current.conceptIsDirty).toBe(false);
      expect(result.current.phrasingIsDirty).toBe(true);
      expect(result.current.isDirty).toBe(true);
    });

    it('should detect both as dirty when both change', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', 'New Title');
        result.current.updateField('question', 'New Question');
      });

      expect(result.current.conceptIsDirty).toBe(true);
      expect(result.current.phrasingIsDirty).toBe(true);
      expect(result.current.isDirty).toBe(true);
    });

    it('should not be dirty when no changes made', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
      });

      expect(result.current.conceptIsDirty).toBe(false);
      expect(result.current.phrasingIsDirty).toBe(false);
      expect(result.current.isDirty).toBe(false);
    });
  });

  describe('Save Orchestration', () => {
    it('should only call concept mutation when only concept is dirty', async () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', 'New Title');
      });

      await act(async () => {
        await result.current.save();
      });

      expect(mockSaveConcept).toHaveBeenCalledOnce();
      expect(mockSaveConcept).toHaveBeenCalledWith({
        title: 'New Title',
      });
      expect(mockSavePhrasing).not.toHaveBeenCalled();
    });

    it('should only call phrasing mutation when only phrasing is dirty', async () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('question', 'New Question');
      });

      await act(async () => {
        await result.current.save();
      });

      expect(mockSavePhrasing).toHaveBeenCalledOnce();
      expect(mockSavePhrasing).toHaveBeenCalledWith({
        question: 'New Question',
        correctAnswer: '4',
        explanation: 'Two plus two equals four',
        options: ['3', '4', '5', '6'],
      });
      expect(mockSaveConcept).not.toHaveBeenCalled();
    });

    it('should call both mutations when both are dirty', async () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', 'New Title');
        result.current.updateField('question', 'New Question');
      });

      await act(async () => {
        await result.current.save();
      });

      expect(mockSaveConcept).toHaveBeenCalledOnce();
      expect(mockSavePhrasing).toHaveBeenCalledOnce();
    });

    it('should exit edit mode after successful save', async () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', 'New Title');
      });

      await act(async () => {
        await result.current.save();
      });

      expect(result.current.isEditing).toBe(false);
    });
  });

  describe('Validation', () => {
    it('should show validation errors for invalid data', async () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', ''); // Invalid
        result.current.updateField('question', ''); // Invalid
      });

      await act(async () => {
        await result.current.save().catch(() => {
          // Expected to throw
        });
      });

      expect(result.current.errors).toHaveProperty('conceptTitle');
      expect(result.current.errors).toHaveProperty('question');
      expect(mockSaveConcept).not.toHaveBeenCalled();
      expect(mockSavePhrasing).not.toHaveBeenCalled();
    });

    it('should stay in edit mode when validation fails', async () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', '');
      });

      await act(async () => {
        await result.current.save().catch(() => {
          // Expected to throw
        });
      });

      expect(result.current.isEditing).toBe(true);
    });

    it('should clear field error when field is updated', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        // Simulate having an error
        result.current.updateField('conceptTitle', '');
      });

      // Now fix the error
      act(() => {
        result.current.updateField('conceptTitle', 'Fixed Title');
      });

      expect(result.current.errors.conceptTitle).toBeUndefined();
    });
  });

  describe('Partial Failure Handling', () => {
    it('should handle concept save success with phrasing save failure', async () => {
      mockSavePhrasing.mockRejectedValue(new Error('Phrasing save failed'));

      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', 'New Title');
        result.current.updateField('question', 'New Question');
      });

      await act(async () => {
        try {
          await result.current.save();
        } catch {
          // Expected to throw
        }
      });

      // Should show error for phrasing
      expect(result.current.errors).toHaveProperty('question');

      // Should stay in edit mode
      expect(result.current.isEditing).toBe(true);

      // Concept should no longer be dirty (succeeded)
      expect(result.current.conceptIsDirty).toBe(false);

      // Phrasing should still be dirty (failed)
      expect(result.current.phrasingIsDirty).toBe(true);
    });

    it('should allow retry after partial failure', async () => {
      // First call fails for phrasing
      mockSavePhrasing.mockRejectedValueOnce(new Error('Phrasing save failed'));

      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', 'New Title');
        result.current.updateField('question', 'New Question');
      });

      // First save - partial failure
      await act(async () => {
        try {
          await result.current.save();
        } catch {
          // Expected
        }
      });

      // Fix phrasing for retry
      mockSavePhrasing.mockResolvedValueOnce(undefined);

      // Second save - should only call phrasing (concept already succeeded)
      await act(async () => {
        await result.current.save();
      });

      expect(mockSaveConcept).toHaveBeenCalledTimes(1); // Only first attempt
      expect(mockSavePhrasing).toHaveBeenCalledTimes(2); // Both attempts
      expect(result.current.isEditing).toBe(false); // Successfully saved
    });
  });

  describe('updateField', () => {
    it('should update specific field in localData', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      act(() => {
        result.current.startEdit();
        result.current.updateField('conceptTitle', 'Updated Title');
      });

      expect(result.current.localData.conceptTitle).toBe('Updated Title');
    });

    it('should update options array correctly', () => {
      const { result } = renderHook(() =>
        useUnifiedEdit(createTestData(), mockSaveConcept, mockSavePhrasing, 'multiple-choice')
      );

      const newOptions = ['a', 'b', 'c', 'd'];

      act(() => {
        result.current.startEdit();
        result.current.updateField('options', newOptions);
      });

      expect(result.current.localData.options).toEqual(newOptions);
    });
  });
});
