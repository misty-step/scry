import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
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

function SimpleTestComponent({ onResult }: { onResult?: (confirmed: boolean) => void }) {
  const confirm = useConfirmation();

  const handleClick = async () => {
    const confirmed = await confirm({
      title: 'Simple confirm',
      description: 'Are you sure?',
    });
    onResult?.(confirmed);
  };

  return (
    <button type="button" data-testid="simple-trigger" onClick={handleClick}>
      Simple Trigger
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

  it('handles cancel button click', async () => {
    const user = userEvent.setup();
    const onResult = vi.fn();
    render(
      <ConfirmationProvider>
        <SimpleTestComponent onResult={onResult} />
      </ConfirmationProvider>
    );

    await user.click(screen.getByTestId('simple-trigger'));
    expect(screen.getByText('Simple confirm')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    await waitFor(() => {
      expect(screen.queryByText('Simple confirm')).not.toBeInTheDocument();
    });
    expect(onResult).toHaveBeenCalledWith(false);
  });

  it('handles confirm without requireTyping', async () => {
    const user = userEvent.setup();
    const onResult = vi.fn();
    render(
      <ConfirmationProvider>
        <SimpleTestComponent onResult={onResult} />
      </ConfirmationProvider>
    );

    await user.click(screen.getByTestId('simple-trigger'));

    // Confirm button should be enabled without typing requirement
    const confirmButton = screen.getByRole('button', { name: 'Confirm' });
    expect(confirmButton).toBeEnabled();

    await user.click(confirmButton);

    await waitFor(() => {
      expect(screen.queryByText('Simple confirm')).not.toBeInTheDocument();
    });
    expect(onResult).toHaveBeenCalledWith(true);
  });

  it('accepts case-insensitive typing match', async () => {
    const user = userEvent.setup();
    render(
      <ConfirmationProvider>
        <TestComponent />
      </ConfirmationProvider>
    );

    await user.click(screen.getByTestId('trigger'));

    const input = screen.getByLabelText(/type "DELETE" to confirm/i);
    await user.type(input, 'delete'); // lowercase

    const confirmButton = screen.getByRole('button', { name: 'Delete' });
    expect(confirmButton).toBeEnabled();
  });

  it('throws when used outside provider', () => {
    // Suppress console.error for this test
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    expect(() => {
      render(<TestComponent />);
    }).toThrow('useConfirmation must be used within a ConfirmationProvider');

    consoleSpy.mockRestore();
  });
});
