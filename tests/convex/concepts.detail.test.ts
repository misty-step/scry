import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Id } from '@/convex/_generated/dataModel';
import { requireUserFromClerk } from '@/convex/clerk';
import { getDetail, setCanonicalPhrasing } from '@/convex/concepts';
import { createMockCtx, createMockDb, makeConcept, makePhrasing } from '@/tests/helpers';

vi.mock('@/convex/clerk', () => ({
  requireUserFromClerk: vi.fn(),
}));

const mockedRequireUser = vi.mocked(requireUserFromClerk);

describe('concepts.getDetail', () => {
  const userId = 'user_1' as Id<'users'>;

  beforeEach(() => {
    vi.clearAllMocks();
    mockedRequireUser.mockResolvedValue({ _id: userId } as any);
  });

  it('returns null when concept is missing or not owned by user', async () => {
    const db = createMockDb({
      get: vi.fn().mockResolvedValueOnce(null),
    });
    const ctx = createMockCtx({ db });

    const result = await (getDetail as any)._handler(ctx, {
      conceptId: 'concept_1' as Id<'concepts'>,
    });

    expect(result).toBeNull();
  });

  it('returns concept, active phrasings, and pendingGeneration flag for owned concept', async () => {
    const concept = makeConcept({ _id: 'concept_1' as Id<'concepts'>, userId });
    const phrasingA = makePhrasing({
      _id: 'phrasing_1' as Id<'phrasings'>,
      userId,
      conceptId: concept._id,
    });
    const phrasingB = makePhrasing({
      _id: 'phrasing_2' as Id<'phrasings'>,
      userId,
      conceptId: concept._id,
    });

    const phrasings = [phrasingA, phrasingB];
    const pendingJob = { _id: 'job_1', pendingConceptIds: [concept._id], conceptIds: [] } as any;

    const db = createMockDb({
      get: vi.fn().mockResolvedValue(concept),
      query: vi.fn().mockImplementation((table: string) => {
        if (table === 'phrasings') {
          const take = vi.fn().mockResolvedValue(phrasings);
          const order = vi.fn().mockReturnValue({ take });
          const filter = vi.fn().mockReturnValue({ order });
          const withIndex = vi.fn().mockReturnValue({ filter });
          return { withIndex };
        }

        if (table === 'generationJobs') {
          const take = vi.fn().mockResolvedValue([pendingJob]);
          const order = vi.fn().mockReturnValue({ take });
          const withIndex = vi.fn().mockReturnValue({ order });
          return { withIndex };
        }

        throw new Error(`Unexpected table: ${table}`);
      }),
    });

    const ctx = createMockCtx({ db });

    const result = await (getDetail as any)._handler(ctx, {
      conceptId: concept._id,
    });

    expect(result).not.toBeNull();
    expect(result.concept._id).toBe(concept._id);
    expect(result.phrasings).toHaveLength(2);
    expect(result.pendingGeneration).toBe(true);
  });
});

describe('concepts.setCanonicalPhrasing', () => {
  const userId = 'user_1' as Id<'users'>;

  beforeEach(() => {
    vi.clearAllMocks();
    mockedRequireUser.mockResolvedValue({ _id: userId } as any);
  });

  it('sets canonical phrasing when concept and phrasing belong to user', async () => {
    const concept = makeConcept({ _id: 'concept_1' as Id<'concepts'>, userId });
    const phrasing = makePhrasing({
      _id: 'phrasing_1' as Id<'phrasings'>,
      userId,
      conceptId: concept._id,
      archivedAt: undefined,
    });

    const patch = vi.fn();
    const db = createMockDb({
      get: vi.fn().mockImplementation((id) => {
        if (id === concept._id) return Promise.resolve(concept);
        if (id === phrasing._id) return Promise.resolve(phrasing);
        return Promise.resolve(null);
      }),
      patch,
    });

    const ctx = createMockCtx({ db });

    const result = await (setCanonicalPhrasing as any)._handler(ctx, {
      conceptId: concept._id,
      phrasingId: phrasing._id,
    });

    expect(patch).toHaveBeenCalledWith(
      concept._id,
      expect.objectContaining({ canonicalPhrasingId: phrasing._id, updatedAt: expect.any(Number) })
    );
    expect(result).toEqual({ canonicalPhrasingId: phrasing._id });
  });

  it('clears canonical phrasing when phrasingId is omitted', async () => {
    const concept = makeConcept({ _id: 'concept_2' as Id<'concepts'>, userId });

    const patch = vi.fn();
    const db = createMockDb({
      get: vi.fn().mockResolvedValue(concept),
      patch,
    });

    const ctx = createMockCtx({ db });

    const result = await (setCanonicalPhrasing as any)._handler(ctx, {
      conceptId: concept._id,
      // phrasingId omitted
    });

    expect(patch).toHaveBeenCalledWith(
      concept._id,
      expect.objectContaining({ canonicalPhrasingId: undefined, updatedAt: expect.any(Number) })
    );
    expect(result).toEqual({ canonicalPhrasingId: null });
  });

  it('throws when concept is missing or belongs to another user', async () => {
    const foreignConcept = makeConcept({
      _id: 'concept_3' as Id<'concepts'>,
      userId: 'other_user' as Id<'users'>,
    });

    const db = createMockDb({
      get: vi.fn().mockResolvedValue(foreignConcept),
    });
    const ctx = createMockCtx({ db });

    await expect(
      (setCanonicalPhrasing as any)._handler(ctx, {
        conceptId: foreignConcept._id,
        phrasingId: undefined,
      })
    ).rejects.toThrow('Concept not found or unauthorized');
  });

  it('throws when phrasing is not found or unauthorized', async () => {
    const concept = makeConcept({ _id: 'concept_4' as Id<'concepts'>, userId });
    const foreignPhrasing = makePhrasing({
      _id: 'phrasing_foreign' as Id<'phrasings'>,
      userId: 'other_user' as Id<'users'>,
      conceptId: concept._id,
    });

    const db = createMockDb({
      get: vi.fn().mockImplementation((id) => {
        if (id === concept._id) return Promise.resolve(concept);
        if (id === foreignPhrasing._id) return Promise.resolve(foreignPhrasing);
        return Promise.resolve(null);
      }),
    });
    const ctx = createMockCtx({ db });

    await expect(
      (setCanonicalPhrasing as any)._handler(ctx, {
        conceptId: concept._id,
        phrasingId: foreignPhrasing._id,
      })
    ).rejects.toThrow('Phrasing not found or unavailable');
  });
});
