import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { AnswerFeedback } from './answer-feedback';
import { renderWithSession } from './review-session-test-utils';

describe('AnswerFeedback', () => {
  it('renders nothing when feedback is hidden and not editing', () => {
    const { container } = renderWithSession(<AnswerFeedback />, {
      feedbackState: { showFeedback: false },
      unifiedEdit: { isEditing: false },
    });

    expect(container.firstChild).toBeNull();
  });

  it('shows concept title when feedback is visible', () => {
    renderWithSession(<AnswerFeedback />, {
      feedbackState: { showFeedback: true },
    });

    expect(screen.getByText('Math Basics')).toBeInTheDocument();
  });

  it('shows explanation text', () => {
    renderWithSession(<AnswerFeedback />, {
      feedbackState: { showFeedback: true },
    });

    expect(screen.getByText('Basic arithmetic')).toBeInTheDocument();
  });

  it('shows next review date when available', () => {
    renderWithSession(<AnswerFeedback />, {
      feedbackState: {
        showFeedback: true,
        nextReviewInfo: {
          nextReview: new Date('2026-01-15T10:00:00Z'),
          scheduledDays: 2,
        },
      },
    });

    expect(screen.getByText(/Next review:/)).toBeInTheDocument();
    expect(screen.getByText(/In 2 days/)).toBeInTheDocument();
  });

  it('shows thumbs up/down when interaction id is set', () => {
    renderWithSession(<AnswerFeedback />, {
      feedbackState: { showFeedback: true },
      currentInteractionId: 'interaction123' as never,
    });

    expect(screen.getByRole('button', { name: 'Helpful' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Not helpful' })).toBeInTheDocument();
  });

  it('calls handleUserFeedback when thumb button is clicked', async () => {
    const user = userEvent.setup();
    const { session } = renderWithSession(<AnswerFeedback />, {
      feedbackState: { showFeedback: true },
      currentInteractionId: 'interaction123' as never,
    });

    await user.click(screen.getByRole('button', { name: 'Helpful' }));

    expect(session.handleUserFeedback).toHaveBeenCalledWith('helpful');
  });
});
