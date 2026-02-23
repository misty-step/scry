import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { InlineEditor } from './inline-editor';
import { renderWithSession } from './review-session-test-utils';

describe('InlineEditor', () => {
  it('renders nothing when not editing', () => {
    const { container } = renderWithSession(<InlineEditor />, {
      unifiedEdit: { isEditing: false },
    });

    expect(container.firstChild).toBeNull();
  });

  it('shows Save Changes and Cancel when editing', () => {
    renderWithSession(<InlineEditor />, {
      unifiedEdit: { isEditing: true },
    });

    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
  });

  it('disables Save button when not dirty', () => {
    renderWithSession(<InlineEditor />, {
      unifiedEdit: { isEditing: true, isDirty: false },
    });

    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled();
  });

  it('calls cancel when Cancel is clicked', async () => {
    const user = userEvent.setup();
    const { session } = renderWithSession(<InlineEditor />, {
      unifiedEdit: { isEditing: true },
    });

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(session.unifiedEdit.cancel).toHaveBeenCalledTimes(1);
  });
});
