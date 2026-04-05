import { useUser } from '@clerk/nextjs';
import { render, screen } from '@testing-library/react';
import { useMutation } from 'convex/react';
import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { useConceptsQuery } from '@/hooks/use-concepts-query';
import { ConceptsClient } from './concepts-client';

vi.mock('@clerk/nextjs', () => ({
  useUser: vi.fn(),
}));

vi.mock('@/hooks/use-concepts-query', () => ({
  useConceptsQuery: vi.fn(),
}));

describe('ConceptsClient', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (useUser as Mock).mockReturnValue({ isSignedIn: true });
    (useMutation as Mock).mockReturnValue(vi.fn());
    (useConceptsQuery as Mock).mockReturnValue({
      concepts: [],
      continueCursor: null,
      isDone: true,
      serverTime: Date.now(),
    });
  });

  it('shows only the reduced library tab set', () => {
    render(<ConceptsClient />);

    expect(screen.getAllByText('All').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Due').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Archived').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Trash').length).toBeGreaterThan(0);
    expect(screen.queryByText('Thin')).not.toBeInTheDocument();
    expect(screen.queryByText('Tension')).not.toBeInTheDocument();
  });
});
