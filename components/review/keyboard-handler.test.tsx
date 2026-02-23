import { fireEvent } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ReviewKeyboardHandler } from './keyboard-handler';
import { renderWithSession } from './review-session-test-utils';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock('sonner', () => ({
  toast: { info: vi.fn() },
}));

describe('ReviewKeyboardHandler', () => {
  it('renders no UI', () => {
    const { container } = renderWithSession(<ReviewKeyboardHandler />);
    expect(container.firstChild).toBeNull();
  });

  it('calls handleSubmit when Enter is pressed with selected answer and no feedback', () => {
    const { session } = renderWithSession(<ReviewKeyboardHandler />, {
      selectedAnswer: '4',
      feedbackState: { showFeedback: false },
    });

    fireEvent.keyDown(window, { key: 'Enter' });
    expect(session.handleSubmit).toHaveBeenCalledTimes(1);
  });

  it('calls handleNext when Enter is pressed while feedback is showing', () => {
    const { session } = renderWithSession(<ReviewKeyboardHandler />, {
      feedbackState: { showFeedback: true },
      isTransitioning: false,
    });

    fireEvent.keyDown(window, { key: 'Enter' });
    expect(session.handleNext).toHaveBeenCalledTimes(1);
  });

  it('saves edits when escape-pressed event fires', () => {
    const { session } = renderWithSession(<ReviewKeyboardHandler />, {
      unifiedEdit: { isEditing: true },
    });

    fireEvent(window, new CustomEvent('escape-pressed'));
    expect(session.unifiedEdit.save).toHaveBeenCalledTimes(1);
  });
});
