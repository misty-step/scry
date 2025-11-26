import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ReviewActionsDropdown } from './review-actions-dropdown';

const defaultHandlers = () => ({
  onEditPhrasing: vi.fn(),
  onEditConcept: vi.fn(),
  onArchivePhrasing: vi.fn(),
  onArchiveConcept: vi.fn(),
});

describe('ReviewActionsDropdown', () => {
  it('triggers callbacks for all menu actions', async () => {
    const user = userEvent.setup();
    const handlers = defaultHandlers();

    render(<ReviewActionsDropdown totalPhrasings={3} {...handlers} />);

    // Test Edit Phrasing
    await user.click(screen.getByRole('button', { name: /review actions/i }));
    await user.click(screen.getByText('Edit Phrasing'));
    expect(handlers.onEditPhrasing).toHaveBeenCalledTimes(1);

    // Test Edit Concept
    await user.click(screen.getByRole('button', { name: /review actions/i }));
    await user.click(screen.getByText('Edit Concept'));
    expect(handlers.onEditConcept).toHaveBeenCalledTimes(1);

    // Test Archive Phrasing
    await user.click(screen.getByRole('button', { name: /review actions/i }));
    await user.click(screen.getByText('Archive Phrasing'));
    expect(handlers.onArchivePhrasing).toHaveBeenCalledTimes(1);

    // Test Archive Concept
    await user.click(screen.getByRole('button', { name: /review actions/i }));
    await user.click(screen.getByText('Archive Concept'));
    expect(handlers.onArchiveConcept).toHaveBeenCalledTimes(1);
  });

  it('hides archive phrasing when only one phrasing exists', async () => {
    const user = userEvent.setup();

    render(<ReviewActionsDropdown totalPhrasings={1} {...defaultHandlers()} />);

    await user.click(screen.getByRole('button', { name: /review actions/i }));
    expect(screen.queryByText('Archive Phrasing')).not.toBeInTheDocument();
  });

  it('shows archive phrasing when more than one phrasing exists', async () => {
    const user = userEvent.setup();

    render(<ReviewActionsDropdown totalPhrasings={2} {...defaultHandlers()} />);

    await user.click(screen.getByRole('button', { name: /review actions/i }));
    expect(screen.getByText('Archive Phrasing')).toBeInTheDocument();
  });
});
