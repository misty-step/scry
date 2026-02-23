import { act, render, screen } from '@testing-library/react';
import { useMutation, useQuery } from 'convex/react';
import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { useConceptActions } from '@/hooks/use-concept-actions';
import { useInstantFeedback } from '@/hooks/use-instant-feedback';
import { useQuizInteractions } from '@/hooks/use-quiz-interactions';
import { useReviewFlow } from '@/hooks/use-review-flow';
import { useUnifiedEdit } from '@/hooks/use-unified-edit';
import { ReviewSessionProvider, useReviewSession } from './session-context';

vi.mock('convex/react', () => ({
  useMutation: vi.fn(),
  useQuery: vi.fn(),
}));

vi.mock('sonner', () => ({
  toast: vi.fn(),
}));

vi.mock('@/hooks/use-review-flow', () => ({
  useReviewFlow: vi.fn(),
}));

vi.mock('@/hooks/use-instant-feedback', () => ({
  useInstantFeedback: vi.fn(),
}));

vi.mock('@/hooks/use-concept-actions', () => ({
  useConceptActions: vi.fn(),
}));

vi.mock('@/hooks/use-quiz-interactions', () => ({
  useQuizInteractions: vi.fn(),
}));

vi.mock('@/hooks/use-unified-edit', () => ({
  useUnifiedEdit: vi.fn(),
}));

vi.mock('@/convex/_generated/api', () => ({
  api: {
    concepts: {
      getConceptsDueCount: { _functionPath: 'concepts:getConceptsDueCount' },
      recordFeedback: { _functionPath: 'concepts:recordFeedback' },
    },
  },
}));

function SessionProbe() {
  const session = useReviewSession();

  return (
    <div>
      <p data-testid="selection-label">{session.selectionReasonLabel ?? 'none'}</p>
      <p data-testid="position-label">{session.phrasingPositionLabel ?? 'none'}</p>
      <p data-testid="due-count">{session.cachedDueCount}</p>
      <p data-testid="selected-answer">{session.selectedAnswer || 'none'}</p>
      <p data-testid="show-feedback">{String(session.feedbackState.showFeedback)}</p>
      <button type="button" onClick={() => session.handleAnswerSelect('A')}>
        pick-a
      </button>
      <button type="button" onClick={session.handleStartInlineEdit}>
        start-edit
      </button>
    </div>
  );
}

describe('ReviewSessionProvider', () => {
  const trackAnswer = vi.fn();
  const recordFeedback = vi.fn();
  let isEditing = false;
  const startEdit = vi.fn(() => {
    isEditing = true;
  });

  beforeEach(() => {
    vi.clearAllMocks();
    isEditing = false;

    (useReviewFlow as unknown as Mock).mockReturnValue({
      phase: 'reviewing',
      question: {
        question: 'What is A?',
        type: 'multiple-choice',
        options: ['A', 'B', 'C', 'D'],
        correctAnswer: 'A',
        explanation: 'Because A',
      },
      conceptTitle: 'Alpha Concept',
      conceptId: 'concept_1',
      phrasingId: 'phrasing_1',
      phrasingIndex: 2,
      totalPhrasings: 4,
      selectionReason: 'least-seen',
      interactions: [],
      isTransitioning: false,
      conceptFsrs: null,
      skippedCount: 0,
      handlers: {
        onReviewComplete: vi.fn(),
        onSkipConcept: vi.fn(),
      },
    });

    (useInstantFeedback as unknown as Mock).mockReturnValue({
      feedbackState: { visible: false, isCorrect: false },
      showFeedback: vi.fn(),
      clearFeedback: vi.fn(),
    });

    (useConceptActions as unknown as Mock).mockReturnValue({
      archivePhrasingWithUndo: vi.fn(),
      archiveConceptWithUndo: vi.fn(),
      editConcept: vi.fn(),
      editPhrasing: vi.fn(),
    });

    (useQuizInteractions as unknown as Mock).mockReturnValue({
      trackAnswer,
    });

    (useUnifiedEdit as unknown as Mock).mockImplementation(() => ({
      isEditing,
      isSaving: false,
      isDirty: false,
      localData: {
        conceptTitle: 'Alpha Concept',
        question: 'What is A?',
        correctAnswer: 'A',
        explanation: 'Because A',
        options: ['A', 'B', 'C', 'D'],
      },
      errors: {},
      startEdit,
      save: vi.fn(),
      cancel: vi.fn(),
      updateField: vi.fn(),
    }));

    (useMutation as unknown as Mock).mockReturnValue(recordFeedback);
    (useQuery as unknown as Mock).mockReturnValue({ conceptsDue: 7 });
    trackAnswer.mockResolvedValue(null);
    recordFeedback.mockResolvedValue(undefined);
  });

  it('throws when useReviewSession is used outside provider', () => {
    expect(() => render(<SessionProbe />)).toThrow('useReviewSession must be used within');
  });

  it('provides derived labels and due count', () => {
    render(
      <ReviewSessionProvider>
        <SessionProbe />
      </ReviewSessionProvider>
    );

    expect(screen.getByTestId('selection-label')).toHaveTextContent('Least practiced');
    expect(screen.getByTestId('position-label')).toHaveTextContent('Phrasing 2 of 4');
    expect(screen.getByTestId('due-count')).toHaveTextContent('7');
  });

  it('updates selected answer only before feedback is shown', () => {
    render(
      <ReviewSessionProvider>
        <SessionProbe />
      </ReviewSessionProvider>
    );

    act(() => {
      screen.getByRole('button', { name: 'pick-a' }).click();
    });

    expect(screen.getByTestId('selected-answer')).toHaveTextContent('A');

    act(() => {
      screen.getByRole('button', { name: 'start-edit' }).click();
    });

    expect(screen.getByTestId('show-feedback')).toHaveTextContent('true');
    expect(startEdit).toHaveBeenCalledTimes(1);

    act(() => {
      screen.getByRole('button', { name: 'pick-a' }).click();
    });

    expect(screen.getByTestId('selected-answer')).toHaveTextContent('A');
  });
});
