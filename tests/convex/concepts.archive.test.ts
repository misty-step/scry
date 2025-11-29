import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Id } from '@/convex/_generated/dataModel';
import { requireUserFromClerk } from '@/convex/clerk';
import { unarchivePhrasing } from '@/convex/concepts';
import { createMockCtx, createMockDb, fixedNow, makeConcept, makePhrasing } from '@/tests/helpers';

vi.mock('@/convex/clerk', () => ({
  requireUserFromClerk: vi.fn(),
}));

const mockedRequireUser = vi.mocked(requireUserFromClerk);

describe('concepts.unarchivePhrasing', () => {
  const mockUserId = 'user_1' as Id<'users'>;
  const mockConceptId = 'concept_1' as Id<'concepts'>;

  beforeEach(() => {
    vi.clearAllMocks();
    mockedRequireUser.mockResolvedValue({
      _id: mockUserId,
      _creationTime: fixedNow,
      email: 'test@example.com',
    } as any);
  });

  it('should unarchive phrasing and clear archivedAt', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_1' as Id<'phrasings'>,
      userId: mockUserId,
      conceptId: mockConceptId,
      archivedAt: fixedNow - 1000,
    });

    const concept = makeConcept({
      _id: mockConceptId,
      userId: mockUserId,
      phrasingCount: 2,
      thinScore: 1,
    });

    const otherPhrasing = makePhrasing({
      _id: 'phrasing_2' as Id<'phrasings'>,
      userId: mockUserId,
      conceptId: mockConceptId,
      question: 'Different question',
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb(
      {
        get: vi.fn((id) => {
          if (id === phrasing._id) return Promise.resolve(phrasing);
          if (id === concept._id) return Promise.resolve(concept);
          return Promise.resolve(null);
        }),
        patch: patchSpy,
      },
      [phrasing, otherPhrasing] // Query will return both after unarchive
    );

    const ctx = createMockCtx({ db: mockDb });

    await (unarchivePhrasing as any)._handler(ctx, {
      conceptId: concept._id,
      phrasingId: phrasing._id,
    });

    // Verify phrasing archivedAt cleared
    expect(patchSpy).toHaveBeenCalledWith(
      phrasing._id,
      expect.objectContaining({
        archivedAt: undefined,
        updatedAt: expect.any(Number),
      })
    );

    // Verify concept phrasingCount incremented
    expect(patchSpy).toHaveBeenCalledWith(
      concept._id,
      expect.objectContaining({
        phrasingCount: 2,
        updatedAt: expect.any(Number),
      })
    );
  });

  it('should recalculate thinScore when unarchiving', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_1' as Id<'phrasings'>,
      userId: mockUserId,
      conceptId: mockConceptId,
      archivedAt: fixedNow - 1000,
    });

    const concept = makeConcept({
      _id: mockConceptId,
      userId: mockUserId,
      phrasingCount: 0,
      thinScore: 4, // Originally had 0 phrasings (target=4, so 4-0=4)
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb(
      {
        get: vi.fn((id) => {
          if (id === phrasing._id) return Promise.resolve(phrasing);
          if (id === concept._id) return Promise.resolve(concept);
          return Promise.resolve(null);
        }),
        patch: patchSpy,
      },
      [phrasing] // Only one active phrasing after unarchive
    );

    const ctx = createMockCtx({ db: mockDb });

    await (unarchivePhrasing as any)._handler(ctx, {
      conceptId: concept._id,
      phrasingId: phrasing._id,
    });

    // After unarchiving, phrasingCount=1, so thinScore should be 3 (target 4 - 1 = 3)
    expect(patchSpy).toHaveBeenCalledWith(
      concept._id,
      expect.objectContaining({
        phrasingCount: 1,
        thinScore: 3,
      })
    );
  });

  it('should recalculate conflictScore when unarchiving', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_1' as Id<'phrasings'>,
      userId: mockUserId,
      conceptId: mockConceptId,
      question: 'What is ATP?',
      archivedAt: fixedNow - 1000,
    });

    const duplicatePhrasing = makePhrasing({
      _id: 'phrasing_2' as Id<'phrasings'>,
      userId: mockUserId,
      conceptId: mockConceptId,
      question: 'What is ATP?', // Same question - conflict!
    });

    const concept = makeConcept({
      _id: mockConceptId,
      userId: mockUserId,
      phrasingCount: 1,
      conflictScore: undefined,
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb(
      {
        get: vi.fn((id) => {
          if (id === phrasing._id) return Promise.resolve(phrasing);
          if (id === concept._id) return Promise.resolve(concept);
          return Promise.resolve(null);
        }),
        patch: patchSpy,
      },
      [phrasing, duplicatePhrasing] // Both active after unarchive - duplicate questions
    );

    const ctx = createMockCtx({ db: mockDb });

    await (unarchivePhrasing as any)._handler(ctx, {
      conceptId: concept._id,
      phrasingId: phrasing._id,
    });

    // Should detect conflict (2 questions with same text)
    expect(patchSpy).toHaveBeenCalledWith(
      concept._id,
      expect.objectContaining({
        phrasingCount: 2,
        conflictScore: 1, // 2 questions - 1 unique = 1 conflict
      })
    );
  });

  it('should be idempotent (return early if not archived)', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_1' as Id<'phrasings'>,
      userId: mockUserId,
      conceptId: mockConceptId,
      archivedAt: undefined, // NOT archived
    });

    const concept = makeConcept({
      _id: mockConceptId,
      userId: mockUserId,
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb({
      get: vi.fn((id) => {
        if (id === phrasing._id) return Promise.resolve(phrasing);
        if (id === concept._id) return Promise.resolve(concept);
        return Promise.resolve(null);
      }),
      patch: patchSpy,
    });

    const ctx = createMockCtx({ db: mockDb });

    const result = await (unarchivePhrasing as any)._handler(ctx, {
      conceptId: concept._id,
      phrasingId: phrasing._id,
    });

    // Should return early without patching
    expect(result).toEqual({ phrasingId: phrasing._id });
    expect(patchSpy).not.toHaveBeenCalled();
  });

  it('should throw error for unauthorized concept access', async () => {
    const otherUserId = 'other_user' as Id<'users'>;
    const concept = makeConcept({
      _id: mockConceptId,
      userId: otherUserId,
    });

    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(concept),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (unarchivePhrasing as any)._handler(ctx, {
        conceptId: concept._id,
        phrasingId: 'phrasing_1' as Id<'phrasings'>,
      })
    ).rejects.toThrow('Concept not found or unauthorized');
  });

  it('should throw error for unauthorized phrasing access', async () => {
    const otherUserId = 'other_user' as Id<'users'>;
    const phrasing = makePhrasing({
      _id: 'phrasing_1' as Id<'phrasings'>,
      userId: otherUserId,
      conceptId: mockConceptId,
    });

    const concept = makeConcept({
      _id: mockConceptId,
      userId: mockUserId,
    });

    const mockDb = createMockDb({
      get: vi.fn((id) => {
        if (id === phrasing._id) return Promise.resolve(phrasing);
        if (id === concept._id) return Promise.resolve(concept);
        return Promise.resolve(null);
      }),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (unarchivePhrasing as any)._handler(ctx, {
        conceptId: concept._id,
        phrasingId: phrasing._id,
      })
    ).rejects.toThrow('Phrasing not found or unauthorized');
  });

  it('should throw error for non-existent phrasing', async () => {
    const concept = makeConcept({
      _id: mockConceptId,
      userId: mockUserId,
    });

    const mockDb = createMockDb({
      get: vi.fn((id) => {
        if (id === concept._id) return Promise.resolve(concept);
        return Promise.resolve(null);
      }),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (unarchivePhrasing as any)._handler(ctx, {
        conceptId: concept._id,
        phrasingId: 'nonexistent' as Id<'phrasings'>,
      })
    ).rejects.toThrow('Phrasing not found or unauthorized');
  });
});
