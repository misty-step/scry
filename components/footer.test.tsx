import { useUser } from '@clerk/nextjs';
import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { Footer } from './footer';

const mockUsePathname = vi.fn();
const mockUseKeyboardShortcuts = vi.fn();

vi.mock('next/navigation', () => ({
  usePathname: () => mockUsePathname(),
}));

vi.mock('@clerk/nextjs', () => ({
  useUser: vi.fn(),
}));

vi.mock('@/hooks/use-keyboard-shortcuts', () => ({
  useKeyboardShortcuts: () => mockUseKeyboardShortcuts(),
}));

vi.mock('@/components/keyboard-indicator', () => ({
  KeyboardIndicator: ({ onClick }: { onClick: () => void }) => (
    <button onClick={onClick} type="button">
      Shortcuts
    </button>
  ),
}));

vi.mock('@/components/keyboard-shortcuts-help', () => ({
  KeyboardShortcutsHelp: () => null,
}));

describe('Footer', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUsePathname.mockReturnValue('/');
    mockUseKeyboardShortcuts.mockReturnValue({ shortcuts: [] });
    (useUser as Mock).mockReturnValue({ isSignedIn: true });
  });

  it('stays hidden on the home review route', () => {
    render(<Footer />);

    expect(screen.queryByText('Feedback')).not.toBeInTheDocument();
  });

  it('renders on non-review routes', () => {
    mockUsePathname.mockReturnValue('/concepts');

    render(<Footer />);

    expect(screen.getByText('Feedback')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Shortcuts' })).toBeInTheDocument();
  });
});
