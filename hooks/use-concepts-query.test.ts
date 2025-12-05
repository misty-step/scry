import { renderHook } from '@testing-library/react';
import { useQuery } from 'convex/react';
import { beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { useConceptsQuery, type ConceptsSort, type ConceptsView } from './use-concepts-query';

vi.mock('convex/react', () => ({
  useQuery: vi.fn(),
}));

vi.mock('@/convex/_generated/api', () => ({
  api: {
    concepts: {
      listForLibrary: { _functionPath: 'concepts:listForLibrary' },
    },
  },
}));

describe('useConceptsQuery', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('passes "skip" when enabled is false', () => {
    renderHook(() =>
      useConceptsQuery({
        enabled: false,
        cursor: null,
        pageSize: 20,
        view: 'all' as ConceptsView,
        search: '',
        sort: 'recent' as ConceptsSort,
      })
    );

    expect(useQuery).toHaveBeenCalledWith(expect.anything(), 'skip');
  });

  it('passes query args when enabled is true', () => {
    renderHook(() =>
      useConceptsQuery({
        enabled: true,
        cursor: null,
        pageSize: 20,
        view: 'all' as ConceptsView,
        search: '',
        sort: 'recent' as ConceptsSort,
      })
    );

    expect(useQuery).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({
        cursor: undefined, // null converted to undefined
        pageSize: 20,
        view: 'all',
        search: undefined, // empty string converted to undefined
        sort: 'recent',
      })
    );
  });

  it('includes cursor when not null', () => {
    renderHook(() =>
      useConceptsQuery({
        enabled: true,
        cursor: 'abc123',
        pageSize: 20,
        view: 'due' as ConceptsView,
        search: '',
        sort: 'nextReview' as ConceptsSort,
      })
    );

    expect(useQuery).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({
        cursor: 'abc123',
      })
    );
  });

  it('includes search when not empty', () => {
    renderHook(() =>
      useConceptsQuery({
        enabled: true,
        cursor: null,
        pageSize: 10,
        view: 'all' as ConceptsView,
        search: 'biology',
        sort: 'recent' as ConceptsSort,
      })
    );

    expect(useQuery).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({
        search: 'biology',
      })
    );
  });

  it('handles all view types', () => {
    const views: ConceptsView[] = ['all', 'due', 'thin', 'tension', 'archived', 'deleted'];

    views.forEach((view) => {
      vi.clearAllMocks();
      renderHook(() =>
        useConceptsQuery({
          enabled: true,
          cursor: null,
          pageSize: 20,
          view,
          search: '',
          sort: 'recent' as ConceptsSort,
        })
      );

      expect(useQuery).toHaveBeenCalledWith(expect.anything(), expect.objectContaining({ view }));
    });
  });

  it('handles both sort types', () => {
    const sorts: ConceptsSort[] = ['recent', 'nextReview'];

    sorts.forEach((sort) => {
      vi.clearAllMocks();
      renderHook(() =>
        useConceptsQuery({
          enabled: true,
          cursor: null,
          pageSize: 20,
          view: 'all' as ConceptsView,
          search: '',
          sort,
        })
      );

      expect(useQuery).toHaveBeenCalledWith(expect.anything(), expect.objectContaining({ sort }));
    });
  });

  it('returns useQuery result', () => {
    const mockData = {
      concepts: [{ _id: 'c1', title: 'Test' }],
      continueCursor: null,
      isDone: true,
      serverTime: Date.now(),
      mode: 'standard' as const,
    };
    (useQuery as Mock).mockReturnValue(mockData);

    const { result } = renderHook(() =>
      useConceptsQuery({
        enabled: true,
        cursor: null,
        pageSize: 20,
        view: 'all' as ConceptsView,
        search: '',
        sort: 'recent' as ConceptsSort,
      })
    );

    expect(result.current).toEqual(mockData);
  });
});
