import { useUser } from '@clerk/nextjs';
import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { Navbar } from './navbar';

const mockUsePathname = vi.fn();
const mockSetTheme = vi.fn();
const mockUseActiveJobs = vi.fn();
const mockUseClerkAppearance = vi.fn();

vi.mock('next/navigation', () => ({
  usePathname: () => mockUsePathname(),
}));

vi.mock('@clerk/nextjs', () => ({
  useUser: vi.fn(),
  UserButton: ({ customMenuItems = [] }: { customMenuItems?: Array<{ label: string }> }) => (
    <div data-testid="user-button">
      {customMenuItems.map((item) => (
        <span key={item.label}>{item.label}</span>
      ))}
    </div>
  ),
}));

vi.mock('next-themes', () => ({
  useTheme: () => ({
    setTheme: mockSetTheme,
    theme: 'light',
    systemTheme: 'light',
  }),
}));

vi.mock('@/components/generation-modal', () => ({
  GenerationModal: () => null,
}));

vi.mock('@/hooks/use-active-jobs', () => ({
  useActiveJobs: () => mockUseActiveJobs(),
}));

vi.mock('@/hooks/use-clerk-appearance', () => ({
  useClerkAppearance: () => mockUseClerkAppearance(),
}));

describe('Navbar', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUsePathname.mockReturnValue('/');
    mockUseActiveJobs.mockReturnValue({ activeCount: 2 });
    mockUseClerkAppearance.mockReturnValue({});
    (useUser as Mock).mockReturnValue({
      isLoaded: true,
      isSignedIn: true,
    });
  });

  it('keeps only generate, library, and user menu actions for signed-in users', () => {
    render(<Navbar />);

    expect(screen.getByRole('link', { name: 'Scry.' })).toHaveAttribute('href', '/');
    expect(screen.getByRole('button', { name: /Generate questions/ })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Concepts' })).toHaveAttribute('href', '/concepts');
    expect(screen.queryByRole('link', { name: 'Background Tasks' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Action Inbox' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'AI Review' })).not.toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.getByText('Settings')).toBeInTheDocument();
    expect(screen.getByText('Switch to dark theme')).toBeInTheDocument();
  });

  it('hides the navbar when auth has loaded and the user is signed out', () => {
    (useUser as Mock).mockReturnValue({
      isLoaded: true,
      isSignedIn: false,
    });

    render(<Navbar />);

    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
  });
});
