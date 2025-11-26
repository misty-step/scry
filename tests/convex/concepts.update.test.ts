import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Id } from '@/convex/_generated/dataModel';
import { requireUserFromClerk } from '@/convex/clerk';
import { updateConcept } from '@/convex/concepts';
import { createMockCtx, createMockDb, fixedNow, makeConcept } from '@/tests/helpers';

vi.mock('@/convex/clerk', () => ({
  requireUserFromClerk: vi.fn(),
}));

const mockedRequireUser = vi.mocked(requireUserFromClerk);

describe('concepts.updateConcept', () => {
  const mockUserId = 'user_1' as Id<'users'>;

  beforeEach(() => {
    vi.clearAllMocks();
    mockedRequireUser.mockResolvedValue({
      _id: mockUserId,
      _creationTime: fixedNow,
      email: 'test@example.com',
    } as any);
  });

  it('should update concept title and description', async () => {
    const concept = makeConcept({
      _id: 'concept_1' as Id<'concepts'>,
      userId: mockUserId,
      title: 'Original Title',
      description: 'Original description',
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(concept),
      patch: patchSpy,
    });
    const ctx = createMockCtx({ db: mockDb });

    await (updateConcept as any)._handler(ctx, {
      conceptId: concept._id,
      title: 'Updated Title',
      description: 'Updated description',
    });

    expect(patchSpy).toHaveBeenCalledWith(
      concept._id,
      expect.objectContaining({
        title: 'Updated Title',
        description: 'Updated description',
        updatedAt: expect.any(Number),
      })
    );
  });

  it('should preserve FSRS state when updating', async () => {
    const originalFsrs = {
      stability: 2.5,
      difficulty: 6.0,
      nextReview: fixedNow + 172800000,
      state: 'review' as const,
      reps: 5,
      lapses: 1,
      elapsedDays: 3,
    };

    const concept = makeConcept({
      _id: 'concept_2' as Id<'concepts'>,
      userId: mockUserId,
      title: 'FSRS Test',
      phrasingCount: 3,
      fsrs: originalFsrs,
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(concept),
      patch: patchSpy,
    });
    const ctx = createMockCtx({ db: mockDb });

    await (updateConcept as any)._handler(ctx, {
      conceptId: concept._id,
      title: 'FSRS Test Updated',
    });

    // Verify FSRS was NOT in the patch (preserved)
    expect(patchSpy).toHaveBeenCalledWith(
      concept._id,
      expect.not.objectContaining({
        fsrs: expect.anything(),
        phrasingCount: expect.anything(),
      })
    );

    // Only title and updatedAt should be patched
    expect(patchSpy).toHaveBeenCalledWith(
      concept._id,
      expect.objectContaining({
        title: 'FSRS Test Updated',
        updatedAt: expect.any(Number),
      })
    );
  });

  it('should reject empty title', async () => {
    const concept = makeConcept({
      _id: 'concept_3' as Id<'concepts'>,
      userId: mockUserId,
    });

    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(concept),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (updateConcept as any)._handler(ctx, {
        conceptId: concept._id,
        title: '',
        description: 'Some description',
      })
    ).rejects.toThrow('Title cannot be empty');
  });

  it('should trim whitespace from title and description', async () => {
    const concept = makeConcept({
      _id: 'concept_4' as Id<'concepts'>,
      userId: mockUserId,
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(concept),
      patch: patchSpy,
    });
    const ctx = createMockCtx({ db: mockDb });

    await (updateConcept as any)._handler(ctx, {
      conceptId: concept._id,
      title: '  Trimmed Title  ',
      description: '  Trimmed Description  ',
    });

    expect(patchSpy).toHaveBeenCalledWith(
      concept._id,
      expect.objectContaining({
        title: 'Trimmed Title',
        description: 'Trimmed Description',
      })
    );
  });

  it('should throw error for unauthorized access', async () => {
    const otherUserId = 'other_user' as Id<'users'>;
    const concept = makeConcept({
      _id: 'concept_5' as Id<'concepts'>,
      userId: otherUserId,
    });

    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(concept),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (updateConcept as any)._handler(ctx, {
        conceptId: concept._id,
        title: 'Hacked Title',
      })
    ).rejects.toThrow('Concept not found or unauthorized');
  });

  it('should throw error for non-existent concept', async () => {
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(null),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (updateConcept as any)._handler(ctx, {
        conceptId: 'nonexistent' as Id<'concepts'>,
        title: 'New Title',
      })
    ).rejects.toThrow('Concept not found');
  });

  it('should handle optional description field', async () => {
    const concept = makeConcept({
      _id: 'concept_6' as Id<'concepts'>,
      userId: mockUserId,
      description: 'Original description',
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(concept),
      patch: patchSpy,
    });
    const ctx = createMockCtx({ db: mockDb });

    // Update only title
    await (updateConcept as any)._handler(ctx, {
      conceptId: concept._id,
      title: 'Updated Title',
    });

    expect(patchSpy).toHaveBeenCalledWith(
      concept._id,
      expect.objectContaining({
        title: 'Updated Title',
      })
    );

    // Description should not be in patch if not provided
    const patchCall = patchSpy.mock.calls[0][1];
    expect(patchCall).not.toHaveProperty('description');
  });
});
