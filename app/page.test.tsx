import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Home from './page';

let isAuthenticated = true;

vi.mock('convex/react', () => ({
  Authenticated: ({ children }: { children: React.ReactNode }) =>
    isAuthenticated ? <>{children}</> : null,
  Unauthenticated: ({ children }: { children: React.ReactNode }) =>
    isAuthenticated ? null : <>{children}</>,
}));

vi.mock('@/components/agent/review-chat', () => ({
  ReviewChat: () => <div data-testid="review-chat">review chat</div>,
}));

describe('Home', () => {
  beforeEach(() => {
    isAuthenticated = true;
  });

  it('renders the review chat for authenticated users', () => {
    render(<Home />);

    expect(screen.getByTestId('review-chat')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Sign in' })).not.toBeInTheDocument();
  });

  it('renders auth actions for signed-out users', () => {
    isAuthenticated = false;

    render(<Home />);

    expect(screen.getByText('Sign in to start reviewing')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute('href', '/sign-in');
    expect(screen.getByRole('link', { name: 'Create account' })).toHaveAttribute(
      'href',
      '/sign-up'
    );
  });
});
