import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { QuestionDisplay } from './question-display';
import { renderWithSession } from './review-session-test-utils';

describe('QuestionDisplay', () => {
  it('renders question text from context', () => {
    renderWithSession(<QuestionDisplay />);

    expect(screen.getByText('What is 2+2?')).toBeInTheDocument();
  });

  it('shows FSRS learning badge when concept is learning', () => {
    renderWithSession(<QuestionDisplay />, {
      conceptFsrs: { state: 'learning', reps: 1 },
    });

    expect(screen.getByText('Learning Mode • Step 2 of 4')).toBeInTheDocument();
  });

  it('hides FSRS learning badge when concept is not learning', () => {
    renderWithSession(<QuestionDisplay />, {
      conceptFsrs: { state: 'review', reps: 2 } as never,
    });

    expect(screen.queryByText(/Learning Mode • Step/)).not.toBeInTheDocument();
  });

  it('renders edit mode form when unified edit is active', () => {
    renderWithSession(<QuestionDisplay />, {
      unifiedEdit: { isEditing: true },
    });

    expect(screen.getByPlaceholderText('Enter question text...')).toBeInTheDocument();
  });
});
