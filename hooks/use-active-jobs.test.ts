import { useUser } from '@clerk/nextjs';
import { renderHook } from '@testing-library/react';
import { useQuery } from 'convex/react';
import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { useActiveJobs } from './use-active-jobs';

vi.mock('@clerk/nextjs', () => ({
  useUser: vi.fn(),
}));

vi.mock('convex/react', () => ({
  useQuery: vi.fn(),
}));

vi.mock('@/convex/_generated/api', () => ({
  api: {
    generationJobs: {
      getRecentJobs: { _functionPath: 'generationJobs:getRecentJobs' },
    },
  },
}));

vi.mock('@/types/generation-jobs', () => ({
  isActiveJob: vi.fn((job: any) => job.status === 'pending' || job.status === 'processing'),
}));

describe('useActiveJobs', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('skips query when user is not signed in', () => {
    (useUser as Mock).mockReturnValue({ isSignedIn: false });
    (useQuery as Mock).mockReturnValue(undefined);

    renderHook(() => useActiveJobs());

    expect(useQuery).toHaveBeenCalledWith(expect.anything(), 'skip');
  });

  it('executes query when user is signed in', () => {
    (useUser as Mock).mockReturnValue({ isSignedIn: true });
    (useQuery as Mock).mockReturnValue({ results: [] });

    renderHook(() => useActiveJobs());

    expect(useQuery).toHaveBeenCalledWith(expect.anything(), { pageSize: 50 });
  });

  it('returns default values when jobs is undefined', () => {
    (useUser as Mock).mockReturnValue({ isSignedIn: true });
    (useQuery as Mock).mockReturnValue(undefined);

    const { result } = renderHook(() => useActiveJobs());

    expect(result.current).toEqual({
      jobs: undefined,
      activeJobs: [],
      activeCount: 0,
      hasActive: false,
    });
  });

  it('returns all jobs and filters active ones', () => {
    (useUser as Mock).mockReturnValue({ isSignedIn: true });

    const mockJobs = [
      { _id: 'job1', status: 'pending' },
      { _id: 'job2', status: 'processing' },
      { _id: 'job3', status: 'completed' },
      { _id: 'job4', status: 'failed' },
    ];
    (useQuery as Mock).mockReturnValue({ results: mockJobs });

    const { result } = renderHook(() => useActiveJobs());

    expect(result.current.jobs).toEqual(mockJobs);
    expect(result.current.activeJobs).toHaveLength(2);
    expect(result.current.activeCount).toBe(2);
    expect(result.current.hasActive).toBe(true);
  });

  it('returns hasActive false when no active jobs', () => {
    (useUser as Mock).mockReturnValue({ isSignedIn: true });

    const mockJobs = [
      { _id: 'job1', status: 'completed' },
      { _id: 'job2', status: 'failed' },
    ];
    (useQuery as Mock).mockReturnValue({ results: mockJobs });

    const { result } = renderHook(() => useActiveJobs());

    expect(result.current.activeJobs).toHaveLength(0);
    expect(result.current.activeCount).toBe(0);
    expect(result.current.hasActive).toBe(false);
  });

  it('handles empty jobs array', () => {
    (useUser as Mock).mockReturnValue({ isSignedIn: true });
    (useQuery as Mock).mockReturnValue({ results: [] });

    const { result } = renderHook(() => useActiveJobs());

    expect(result.current.jobs).toEqual([]);
    expect(result.current.activeJobs).toEqual([]);
    expect(result.current.activeCount).toBe(0);
    expect(result.current.hasActive).toBe(false);
  });

  it('handles all jobs being active', () => {
    (useUser as Mock).mockReturnValue({ isSignedIn: true });

    const mockJobs = [
      { _id: 'job1', status: 'pending' },
      { _id: 'job2', status: 'processing' },
      { _id: 'job3', status: 'pending' },
    ];
    (useQuery as Mock).mockReturnValue({ results: mockJobs });

    const { result } = renderHook(() => useActiveJobs());

    expect(result.current.activeJobs).toHaveLength(3);
    expect(result.current.activeCount).toBe(3);
    expect(result.current.hasActive).toBe(true);
  });
});
