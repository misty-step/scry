import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
// Import mocked modules
import { useQuery } from 'convex/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useQuizInteractions } from '@/hooks/use-quiz-interactions';
import { useReviewFlow } from '@/hooks/use-review-flow';
import { ReviewFlow } from './review-flow';

// Mock all dependencies
vi.mock('convex/react', () => ({
  useQuery: vi.fn(),
  useMutation: vi.fn(),
}));

vi.mock('@clerk/nextjs', () => ({
  useUser: vi.fn(() => ({
    isSignedIn: true,
    user: { id: 'test-user-id' },
  })),
}));

vi.mock('@/hooks/use-shuffled-options', () => ({
  useShuffledOptions: vi.fn((options: string[]) => options),
}));

vi.mock('@/hooks/use-review-flow', () => ({
  useReviewFlow: vi.fn(),
}));

vi.mock('@/hooks/use-quiz-interactions', () => ({
  useQuizInteractions: vi.fn(),
}));

vi.mock('@/contexts/current-question-context', () => ({
  useCurrentQuestion: vi.fn(() => ({
    setCurrentQuestion: vi.fn(),
  })),
}));

vi.mock('@/hooks/use-keyboard-shortcuts', () => ({
  useReviewShortcuts: vi.fn(),
}));

vi.mock('@/hooks/use-question-mutations', () => ({
  useQuestionMutations: vi.fn(() => ({
    deleteQuestion: vi.fn(),
  })),
}));

vi.mock('@/hooks/use-confirmation', () => ({
  useConfirmation: vi.fn(() => ({
    confirm: vi.fn(),
    ConfirmationModal: () => null,
  })),
}));

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}));

describe('ReviewFlow - Instant Feedback Integration', () => {
  let mockTrackAnswer: ReturnType<typeof vi.fn>;
  let mockHandleNext: ReturnType<typeof vi.fn>;

  const mockQuestion = {
    question: 'What is grace?',
    options: ['Unmerited favor', 'Earned reward', 'Good luck', 'Hard work'],
    correctAnswer: 'Unmerited favor',
    type: 'multiple-choice' as const,
  };

  const mockReviewFlowState = {
    phase: 'reviewing' as const,
    question: mockQuestion,
    conceptTitle: 'Grace',
    conceptId: 'concept-1',
    phrasingId: 'phrasing-1',
    phrasingIndex: 1,
    totalPhrasings: 3,
    legacyQuestionId: null,
    selectionReason: 'canonical',
    interactions: [],
    isTransitioning: false,
    conceptFsrs: { state: 'review' as const, reps: 5 },
    handlers: {
      handleNext: vi.fn(),
      handleRefresh: vi.fn(),
    },
  };

  beforeEach(() => {
    vi.clearAllMocks();

    // Mock trackAnswer with configurable delay
    mockTrackAnswer = vi.fn();
    (useQuizInteractions as any).mockReturnValue({
      trackAnswer: mockTrackAnswer,
    });

    // Mock useReviewFlow
    mockHandleNext = vi.fn();
    (useReviewFlow as any).mockReturnValue({
      ...mockReviewFlowState,
      handlers: {
        handleNext: mockHandleNext,
        handleRefresh: vi.fn(),
      },
    });

    // Mock useQuery for due count
    (useQuery as any).mockReturnValue({ total: 10 });
  });

  describe('Instant Visual Feedback', () => {
    it('shows instant feedback immediately when answer is submitted', async () => {
      // Mock trackAnswer with 500ms delay to test fire-and-forget
      mockTrackAnswer.mockImplementation(
        () =>
          new Promise((resolve) => {
            setTimeout(
              () =>
                resolve({
                  nextReview: new Date('2025-01-25'),
                  scheduledDays: 5,
                  newState: 'review',
                }),
              500
            );
          })
      );

      render(<ReviewFlow />);

      // Select an answer
      const firstOption = screen.getByText('Unmerited favor');
      fireEvent.click(firstOption);

      // Submit the answer
      const submitButton = screen.getByText('Submit');
      fireEvent.click(submitButton);

      // Instant feedback should appear immediately (synchronously)
      // The selected button should show feedback colors instantly
      await waitFor(
        () => {
          const selectedButton = screen.getByText('Unmerited favor').closest('button');
          expect(selectedButton).toHaveClass('border-success-border');
          expect(selectedButton).toHaveClass('bg-success-background');
        },
        { timeout: 100 }
      ); // Very short timeout to verify it's instant

      // trackAnswer should still be called
      expect(mockTrackAnswer).toHaveBeenCalledWith(
        'concept-1',
        'phrasing-1',
        'Unmerited favor',
        true,
        expect.any(Number),
        expect.any(String)
      );
    });

    it('shows success feedback for correct answer', async () => {
      mockTrackAnswer.mockResolvedValue({
        nextReview: new Date('2025-01-25'),
        scheduledDays: 5,
        newState: 'review',
      });

      render(<ReviewFlow />);

      // Select correct answer
      fireEvent.click(screen.getByText('Unmerited favor'));
      fireEvent.click(screen.getByText('Submit'));

      // Verify success colors (multiple-choice layout doesn't have text-success)
      await waitFor(() => {
        const correctButton = screen.getByText('Unmerited favor').closest('button');
        expect(correctButton).toHaveClass('border-success-border');
        expect(correctButton).toHaveClass('bg-success-background');
      });

      // Verify CheckCircle icon appears (using stable SVG presence check)
      const correctButton = screen.getByText('Unmerited favor').closest('button');
      expect(correctButton?.querySelector('svg')).toBeInTheDocument();
    });

    it('shows error feedback for incorrect answer', async () => {
      mockTrackAnswer.mockResolvedValue({
        nextReview: new Date('2025-01-20'),
        scheduledDays: 1,
        newState: 'relearning',
      });

      render(<ReviewFlow />);

      // Select incorrect answer
      fireEvent.click(screen.getByText('Earned reward'));
      fireEvent.click(screen.getByText('Submit'));

      // Verify error colors on selected (wrong) answer (multiple-choice layout doesn't have text-error)
      await waitFor(() => {
        const incorrectButton = screen.getByText('Earned reward').closest('button');
        expect(incorrectButton).toHaveClass('border-error-border');
        expect(incorrectButton).toHaveClass('bg-error-background');
      });

      // Verify correct answer still shows success colors
      await waitFor(() => {
        const correctButton = screen.getByText('Unmerited favor').closest('button');
        expect(correctButton).toHaveClass('border-success-border');
      });

      // Verify XCircle icon appears on wrong answer
      const xIcons = document.querySelectorAll('.lucide-circle-x');
      expect(xIcons.length).toBeGreaterThan(0);
    });
  });

  describe('ARIA Live Region Announcements', () => {
    it('announces "Correct" when correct answer is submitted', async () => {
      mockTrackAnswer.mockResolvedValue({
        nextReview: new Date(),
        scheduledDays: 5,
        newState: 'review',
      });

      render(<ReviewFlow />);

      // Select correct answer and submit
      fireEvent.click(screen.getByText('Unmerited favor'));
      fireEvent.click(screen.getByText('Submit'));

      // Check ARIA live region
      await waitFor(() => {
        const liveRegion = screen.getByRole('status');
        expect(liveRegion).toHaveAttribute('aria-live', 'polite');
        expect(liveRegion).toHaveAttribute('aria-atomic', 'true');
        expect(liveRegion).toHaveTextContent('Correct');
      });
    });

    it('announces "Incorrect" when wrong answer is submitted', async () => {
      mockTrackAnswer.mockResolvedValue({
        nextReview: new Date(),
        scheduledDays: 1,
        newState: 'relearning',
      });

      render(<ReviewFlow />);

      // Select incorrect answer and submit
      fireEvent.click(screen.getByText('Earned reward'));
      fireEvent.click(screen.getByText('Submit'));

      // Check ARIA live region
      await waitFor(() => {
        const liveRegion = screen.getByRole('status');
        expect(liveRegion).toHaveTextContent('Incorrect');
      });
    });
  });

  describe('Progressive Data Loading', () => {
    it('shows scheduling details after backend mutation completes', async () => {
      // Mock slow backend with 500ms delay
      mockTrackAnswer.mockImplementation(
        () =>
          new Promise((resolve) => {
            setTimeout(
              () =>
                resolve({
                  nextReview: new Date('2025-01-30T10:00:00Z'),
                  scheduledDays: 10,
                  newState: 'review',
                }),
              500
            );
          })
      );

      render(<ReviewFlow />);

      // Select and submit
      fireEvent.click(screen.getByText('Unmerited favor'));
      fireEvent.click(screen.getByText('Submit'));

      // Instant feedback should appear immediately
      await waitFor(
        () => {
          expect(screen.getByText('Unmerited favor').closest('button')).toHaveClass(
            'border-success-border'
          );
        },
        { timeout: 100 }
      );

      // Wait for backend mutation to complete and scheduling details to appear
      await waitFor(
        () => {
          expect(screen.getByText(/Next review:/i)).toBeInTheDocument();
        },
        { timeout: 1000 }
      );
    });

    it('handles backend mutation failure gracefully', async () => {
      const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

      // Mock trackAnswer to reject
      mockTrackAnswer.mockRejectedValue(new Error('Network error'));

      render(<ReviewFlow />);

      // Select and submit
      fireEvent.click(screen.getByText('Unmerited favor'));
      fireEvent.click(screen.getByText('Submit'));

      // Instant feedback should still appear
      await waitFor(() => {
        expect(screen.getByText('Unmerited favor').closest('button')).toHaveClass(
          'border-success-border'
        );
      });

      // Error should be logged (Phase 1 MVP: just log, no retry)
      await waitFor(() => {
        expect(consoleErrorSpy).toHaveBeenCalledWith('Failed to track answer:', expect.any(Error));
      });

      consoleErrorSpy.mockRestore();
    });
  });

  describe('State Management', () => {
    it('clears feedback when new question loads', async () => {
      mockTrackAnswer.mockResolvedValue({
        nextReview: new Date(),
        scheduledDays: 5,
        newState: 'review',
      });

      const { rerender } = render(<ReviewFlow />);

      // Submit first answer
      fireEvent.click(screen.getByText('Unmerited favor'));
      fireEvent.click(screen.getByText('Submit'));

      // Verify feedback shown
      await waitFor(() => {
        expect(screen.getByText('Unmerited favor').closest('button')).toHaveClass(
          'border-success-border'
        );
      });

      // Simulate new question by updating phrasingId
      const newQuestion = {
        question: 'What is faith?',
        options: ['Trust in God', 'Hope for future', 'Religious duty', 'Blind belief'],
        correctAnswer: 'Trust in God',
        type: 'multiple-choice' as const,
      };

      (useReviewFlow as any).mockReturnValue({
        ...mockReviewFlowState,
        question: newQuestion,
        phrasingId: 'phrasing-2', // Different phrasingId triggers reset
        handlers: {
          handleNext: mockHandleNext,
          handleRefresh: vi.fn(),
        },
      });

      // Rerender with new question
      rerender(<ReviewFlow />);

      // Wait for new question to render
      await waitFor(() => {
        expect(screen.getByText('What is faith?')).toBeInTheDocument();
      });

      // Verify ARIA live region is cleared
      const liveRegion = screen.getByRole('status');
      expect(liveRegion).toHaveTextContent('');

      // Verify new options don't have feedback colors
      const newOption = screen.getByText('Trust in God').closest('button');
      expect(newOption).not.toHaveClass('border-success-border');
      expect(newOption).not.toHaveClass('border-error-border');
    });

    it('prevents submission without selected answer', () => {
      render(<ReviewFlow />);

      // Try to submit without selecting
      const submitButton = screen.getByText('Submit');
      fireEvent.click(submitButton);

      // trackAnswer should not be called
      expect(mockTrackAnswer).not.toHaveBeenCalled();

      // No feedback should appear
      const liveRegion = screen.getByRole('status');
      expect(liveRegion).toHaveTextContent('');
    });
  });

  describe('Bug Fix: Feedback Section and Next Button', () => {
    it('shows Next button immediately after submission (not waiting for backend)', async () => {
      // Mock trackAnswer with LONG delay (2 seconds)
      mockTrackAnswer.mockImplementation(
        () =>
          new Promise((resolve) => {
            setTimeout(
              () =>
                resolve({
                  nextReview: new Date('2025-01-25'),
                  scheduledDays: 5,
                  newState: 'review',
                }),
              2000
            );
          })
      );

      render(<ReviewFlow />);

      // Select and submit
      fireEvent.click(screen.getByText('Unmerited favor'));
      fireEvent.click(screen.getByText('Submit'));

      // CRITICAL: Next button should appear immediately (not after 2 seconds!)
      await waitFor(
        () => {
          expect(screen.getByText('Next')).toBeInTheDocument();
        },
        { timeout: 100 }
      ); // Very short timeout proves it's instant

      // Submit button should be gone
      expect(screen.queryByText('Submit')).not.toBeInTheDocument();
    });

    it('shows feedback section immediately without waiting for backend', async () => {
      // Mock trackAnswer with LONG delay
      mockTrackAnswer.mockImplementation(
        () =>
          new Promise((resolve) => {
            setTimeout(
              () =>
                resolve({
                  nextReview: new Date('2025-01-25'),
                  scheduledDays: 5,
                  newState: 'review',
                }),
              2000
            );
          })
      );

      // Add explanation to question
      const mockQuestionWithExplanation = {
        ...mockQuestion,
        explanation: 'Grace is unmerited favor from God',
      };

      (useReviewFlow as any).mockReturnValue({
        ...mockReviewFlowState,
        question: mockQuestionWithExplanation,
      });

      render(<ReviewFlow />);

      // Select and submit
      fireEvent.click(screen.getByText('Unmerited favor'));
      fireEvent.click(screen.getByText('Submit'));

      // Explanation should appear immediately
      await waitFor(
        () => {
          expect(screen.getByText('Grace is unmerited favor from God')).toBeInTheDocument();
        },
        { timeout: 100 }
      );
    });

    it('progressively shows scheduling details after backend responds', async () => {
      // Mock trackAnswer with moderate delay
      mockTrackAnswer.mockImplementation(
        () =>
          new Promise((resolve) => {
            setTimeout(
              () =>
                resolve({
                  nextReview: new Date('2025-01-30T10:00:00Z'),
                  scheduledDays: 5,
                  newState: 'review',
                }),
              500
            );
          })
      );

      render(<ReviewFlow />);

      // Select and submit
      fireEvent.click(screen.getByText('Unmerited favor'));
      fireEvent.click(screen.getByText('Submit'));

      // Next button appears immediately
      await waitFor(
        () => {
          expect(screen.getByText('Next')).toBeInTheDocument();
        },
        { timeout: 100 }
      );

      // Scheduling details NOT present yet
      expect(screen.queryByText(/Next review:/i)).not.toBeInTheDocument();

      // Wait for backend to respond
      await waitFor(
        () => {
          expect(screen.getByText(/Next review:/i)).toBeInTheDocument();
        },
        { timeout: 1000 }
      );

      // Verify scheduling details filled in
      expect(screen.getByText(/In 5 days/i)).toBeInTheDocument();
    });

    it('shows Next button even when backend fails', async () => {
      // Mock trackAnswer to reject
      mockTrackAnswer.mockRejectedValue(new Error('Network error'));

      render(<ReviewFlow />);

      // Select and submit
      fireEvent.click(screen.getByText('Unmerited favor'));
      fireEvent.click(screen.getByText('Submit'));

      // Next button should still appear (not blocked by error)
      await waitFor(
        () => {
          expect(screen.getByText('Next')).toBeInTheDocument();
        },
        { timeout: 100 }
      );

      // User can click Next and advance
      const nextButton = screen.getByText('Next');
      expect(nextButton).not.toBeDisabled();
    });

    it('shows concept title immediately after submission', async () => {
      // Mock trackAnswer with LONG delay
      mockTrackAnswer.mockImplementation(
        () =>
          new Promise((resolve) => {
            setTimeout(
              () =>
                resolve({
                  nextReview: new Date('2025-01-25'),
                  scheduledDays: 5,
                  newState: 'review',
                }),
              2000
            );
          })
      );

      render(<ReviewFlow />);

      // Select and submit
      fireEvent.click(screen.getByText('Unmerited favor'));
      fireEvent.click(screen.getByText('Submit'));

      // Concept title should appear immediately
      await waitFor(
        () => {
          expect(screen.getByText('Grace')).toBeInTheDocument(); // mockReviewFlowState.conceptTitle
        },
        { timeout: 100 }
      );
    });
  });
});
