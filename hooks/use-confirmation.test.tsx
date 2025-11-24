import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { ConfirmationProvider, useConfirmation } from './use-confirmation';

function TestComponent() {
  const confirm = useConfirmation();

  const handleClick = async () => {
    await confirm({
      title: 'Delete item?',
      description: 'This cannot be undone',
      confirmText: 'Delete',
      cancelText: 'Cancel',
      variant: 'destructive',
      requireTyping: 'DELETE',
    });
  };

  return (
    <button type="button" data-testid="trigger" onClick={handleClick}>
      Trigger
    </button>
  );
}

describe('ConfirmationProvider + useConfirmation', () => {
  it('requires matching typed text before confirming and restores focus', async () => {
    const user = userEvent.setup();
    render(
      <ConfirmationProvider>
        <TestComponent />
      </ConfirmationProvider>
    );

    const trigger = screen.getByTestId('trigger');
    await user.click(trigger);

    expect(screen.getByText('Delete item?')).toBeInTheDocument();
    const confirmButton = screen.getByRole('button', { name: 'Delete' });
    expect(confirmButton).toBeDisabled();

    const input = screen.getByLabelText(/type "DELETE" to confirm/i);
    await user.type(input, 'del');
    expect(confirmButton).toBeDisabled();

    await user.clear(input);
    await user.type(input, 'DELETE');
    expect(confirmButton).toBeEnabled();

    await user.click(confirmButton);

    await waitFor(() => {
      expect(screen.queryByText('Delete item?')).not.toBeInTheDocument();
    });
    expect([trigger, document.body]).toContain(document.activeElement);
  });
});
