import { act, renderHook } from '@testing-library/react';
import { useMutation } from 'convex/react';
import { toast } from 'sonner';
import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { useConceptActions } from './use-concept-actions';
import { useUndoableAction } from './use-undoable-action';

vi.mock('convex/react', () => ({
  useMutation: vi.fn(),
}));

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

vi.mock('./use-undoable-action', () => ({
  useUndoableAction: vi.fn(),
}));

vi.mock('@/convex/_generated/api', () => ({
  api: {
    concepts: {
      setCanonicalPhrasing: { _functionPath: 'concepts:setCanonicalPhrasing' },
      archivePhrasing: { _functionPath: 'concepts:archivePhrasing' },
      unarchivePhrasing: { _functionPath: 'concepts:unarchivePhrasing' },
      archiveConcept: { _functionPath: 'concepts:archiveConcept' },
      unarchiveConcept: { _functionPath: 'concepts:unarchiveConcept' },
      updateConcept: { _functionPath: 'concepts:updateConcept' },
      updatePhrasing: { _functionPath: 'concepts:updatePhrasing' },
      requestPhrasingGeneration: { _functionPath: 'concepts:requestPhrasingGeneration' },
    },
  },
}));

describe('useConceptActions', () => {
  const conceptId = 'concept_1';
  let mockSetCanonical: any;
  let mockArchive: any;
  let mockUnarchive: any;
  let mockArchiveConcept: any;
  let mockUnarchiveConcept: any;
  let mockUpdateConcept: any;
  let mockUpdatePhrasing: any;
  let mockGenerate: any;
  let mockUndoableAction: any;

  beforeEach(() => {
    vi.clearAllMocks();

    mockSetCanonical = vi.fn().mockResolvedValue({});
    mockArchive = vi.fn().mockResolvedValue({});
    mockUnarchive = vi.fn().mockResolvedValue({});
    mockArchiveConcept = vi.fn().mockResolvedValue({});
    mockUnarchiveConcept = vi.fn().mockResolvedValue({});
    mockUpdateConcept = vi.fn().mockResolvedValue({});
    mockUpdatePhrasing = vi.fn().mockResolvedValue({});
    mockGenerate = vi.fn().mockResolvedValue({});
    mockUndoableAction = vi.fn().mockResolvedValue(undefined);

    (useUndoableAction as unknown as Mock).mockReturnValue(mockUndoableAction);

    (useMutation as unknown as Mock).mockImplementation((mutation: any) => {
      switch (mutation?._functionPath) {
        case 'concepts:setCanonicalPhrasing':
          return mockSetCanonical;
        case 'concepts:archivePhrasing':
          return mockArchive;
        case 'concepts:unarchivePhrasing':
          return mockUnarchive;
        case 'concepts:archiveConcept':
          return mockArchiveConcept;
        case 'concepts:unarchiveConcept':
          return mockUnarchiveConcept;
        case 'concepts:updateConcept':
          return mockUpdateConcept;
        case 'concepts:updatePhrasing':
          return mockUpdatePhrasing;
        case 'concepts:requestPhrasingGeneration':
          return mockGenerate;
        default:
          return vi.fn();
      }
    });
  });

  it('sets canonical phrasing', async () => {
    const { result } = renderHook(() => useConceptActions({ conceptId }));

    await act(async () => {
      await result.current.setCanonical('phrasing_1');
    });

    expect(mockSetCanonical).toHaveBeenCalledWith({
      conceptId,
      phrasingId: 'phrasing_1',
    });
    expect(toast.success).toHaveBeenCalledWith('Canonical phrasing updated');
  });

  it('archives phrasing', async () => {
    const { result } = renderHook(() => useConceptActions({ conceptId }));

    await act(async () => {
      await result.current.archivePhrasing('phrasing_2');
    });

    expect(mockArchive).toHaveBeenCalledWith({
      conceptId,
      phrasingId: 'phrasing_2',
    });
    expect(toast.success).toHaveBeenCalledWith('Phrasing archived');
  });

  it('requests generation', async () => {
    const { result } = renderHook(() => useConceptActions({ conceptId }));

    await act(async () => {
      await result.current.requestMorePhrasings();
    });

    expect(mockGenerate).toHaveBeenCalledWith({ conceptId });
    expect(toast.success).toHaveBeenCalledWith('Generation job started');
  });

  it('updates concept', async () => {
    const { result } = renderHook(() => useConceptActions({ conceptId }));

    await act(async () => {
      await result.current.editConcept({
        title: 'Updated Title',
      });
    });

    expect(mockUpdateConcept).toHaveBeenCalledWith({
      conceptId,
      title: 'Updated Title',
    });
    expect(toast.success).toHaveBeenCalledWith('Concept updated');
  });

  it('updates phrasing', async () => {
    const { result } = renderHook(() => useConceptActions({ conceptId }));

    await act(async () => {
      await result.current.editPhrasing({
        phrasingId: 'phrasing_1',
        question: 'Updated Question?',
        correctAnswer: 'Updated Answer',
        explanation: 'Updated Explanation',
      });
    });

    expect(mockUpdatePhrasing).toHaveBeenCalledWith({
      phrasingId: 'phrasing_1',
      question: 'Updated Question?',
      correctAnswer: 'Updated Answer',
      explanation: 'Updated Explanation',
    });
    expect(toast.success).toHaveBeenCalledWith('Phrasing updated');
  });

  it('archives phrasing with undo', async () => {
    const { result } = renderHook(() => useConceptActions({ conceptId }));

    await act(async () => {
      await result.current.archivePhrasingWithUndo('phrasing_1');
    });

    expect(mockUndoableAction).toHaveBeenCalledWith({
      action: expect.any(Function),
      message: 'Phrasing archived',
      undo: expect.any(Function),
      duration: 8000,
    });

    // Verify action callback calls archivePhrasing
    const actionFn = mockUndoableAction.mock.calls[0][0].action;
    await actionFn();
    expect(mockArchive).toHaveBeenCalledWith({
      conceptId,
      phrasingId: 'phrasing_1',
    });

    // Verify undo callback calls unarchivePhrasing
    const undoFn = mockUndoableAction.mock.calls[0][0].undo;
    await undoFn();
    expect(mockUnarchive).toHaveBeenCalledWith({
      conceptId,
      phrasingId: 'phrasing_1',
    });
  });

  it('archives concept with undo', async () => {
    const { result } = renderHook(() => useConceptActions({ conceptId }));

    await act(async () => {
      await result.current.archiveConceptWithUndo();
    });

    expect(mockUndoableAction).toHaveBeenCalledWith({
      action: expect.any(Function),
      message: 'Concept archived',
      undo: expect.any(Function),
      duration: 8000,
    });

    // Verify action callback calls archiveConcept
    const actionFn = mockUndoableAction.mock.calls[0][0].action;
    await actionFn();
    expect(mockArchiveConcept).toHaveBeenCalledWith({
      conceptId,
    });

    // Verify undo callback calls unarchiveConcept
    const undoFn = mockUndoableAction.mock.calls[0][0].undo;
    await undoFn();
    expect(mockUnarchiveConcept).toHaveBeenCalledWith({
      conceptId,
    });
  });

  describe('Error handling', () => {
    it('handles editConcept error with Error instance', async () => {
      mockUpdateConcept.mockRejectedValue(new Error('Update concept failed'));

      const { result } = renderHook(() => useConceptActions({ conceptId }));

      await act(async () => {
        try {
          await result.current.editConcept({ title: 'New Title' });
        } catch {
          // Expected
        }
      });

      expect(toast.error).toHaveBeenCalledWith('Update concept failed');
    });

    it('handles editConcept error with non-Error', async () => {
      mockUpdateConcept.mockRejectedValue('string error');

      const { result } = renderHook(() => useConceptActions({ conceptId }));

      await act(async () => {
        try {
          await result.current.editConcept({ title: 'New Title' });
        } catch {
          // Expected
        }
      });

      expect(toast.error).toHaveBeenCalledWith('Failed to update concept');
    });

    it('handles editPhrasing error with Error instance', async () => {
      mockUpdatePhrasing.mockRejectedValue(new Error('Update phrasing failed'));

      const { result } = renderHook(() => useConceptActions({ conceptId }));

      await act(async () => {
        try {
          await result.current.editPhrasing({
            phrasingId: 'phrasing_1',
            question: 'Q',
            correctAnswer: 'A',
          });
        } catch {
          // Expected
        }
      });

      expect(toast.error).toHaveBeenCalledWith('Update phrasing failed');
    });

    it('handles editPhrasing error with non-Error', async () => {
      mockUpdatePhrasing.mockRejectedValue('string error');

      const { result } = renderHook(() => useConceptActions({ conceptId }));

      await act(async () => {
        try {
          await result.current.editPhrasing({
            phrasingId: 'phrasing_1',
            question: 'Q',
            correctAnswer: 'A',
          });
        } catch {
          // Expected
        }
      });

      expect(toast.error).toHaveBeenCalledWith('Failed to update phrasing');
    });
  });
});
