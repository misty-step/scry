import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Id } from '@/convex/_generated/dataModel';
import { requireUserFromClerk } from '@/convex/clerk';
import { updateConcept, updatePhrasing } from '@/convex/concepts';
import { createMockCtx, createMockDb, fixedNow, makeConcept, makePhrasing } from '@/tests/helpers';

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

  it('should update concept title', async () => {
    const concept = makeConcept({
      _id: 'concept_1' as Id<'concepts'>,
      userId: mockUserId,
      title: 'Original Title',
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
    });

    expect(patchSpy).toHaveBeenCalledWith(
      concept._id,
      expect.objectContaining({
        title: 'Updated Title',
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
      })
    ).rejects.toThrow('Title cannot be empty');
  });

  it('should trim whitespace from title', async () => {
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
    });

    expect(patchSpy).toHaveBeenCalledWith(
      concept._id,
      expect.objectContaining({
        title: 'Trimmed Title',
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
});

describe('concepts.updatePhrasing', () => {
  const mockUserId = 'user_1' as Id<'users'>;

  beforeEach(() => {
    vi.clearAllMocks();
    mockedRequireUser.mockResolvedValue({
      _id: mockUserId,
      _creationTime: fixedNow,
      email: 'test@example.com',
    } as any);
  });

  it('should update phrasing question and answer', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_1' as Id<'phrasings'>,
      userId: mockUserId,
      question: 'Original question?',
      correctAnswer: 'Original answer',
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(phrasing),
      patch: patchSpy,
    });
    const ctx = createMockCtx({ db: mockDb });

    await (updatePhrasing as any)._handler(ctx, {
      phrasingId: phrasing._id,
      question: 'Updated question?',
      correctAnswer: 'Updated answer',
    });

    expect(patchSpy).toHaveBeenCalledWith(
      phrasing._id,
      expect.objectContaining({
        question: 'Updated question?',
        correctAnswer: 'Updated answer',
        updatedAt: expect.any(Number),
      })
    );
  });

  it('should preserve stats fields when updating', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_2' as Id<'phrasings'>,
      userId: mockUserId,
      attemptCount: 10,
      correctCount: 8,
      lastAttemptedAt: fixedNow - 86400000,
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(phrasing),
      patch: patchSpy,
    });
    const ctx = createMockCtx({ db: mockDb });

    await (updatePhrasing as any)._handler(ctx, {
      phrasingId: phrasing._id,
      question: 'Updated question?',
      correctAnswer: 'New answer',
    });

    // Verify stats were NOT in the patch (preserved)
    expect(patchSpy).toHaveBeenCalledWith(
      phrasing._id,
      expect.not.objectContaining({
        attemptCount: expect.anything(),
        correctCount: expect.anything(),
        lastAttemptedAt: expect.anything(),
      })
    );
  });

  it('should reject empty question', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_3' as Id<'phrasings'>,
      userId: mockUserId,
    });

    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(phrasing),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (updatePhrasing as any)._handler(ctx, {
        phrasingId: phrasing._id,
        question: '',
        correctAnswer: 'Answer',
      })
    ).rejects.toThrow('Question cannot be empty');
  });

  it('should reject empty correctAnswer', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_4' as Id<'phrasings'>,
      userId: mockUserId,
    });

    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(phrasing),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (updatePhrasing as any)._handler(ctx, {
        phrasingId: phrasing._id,
        question: 'Question?',
        correctAnswer: '',
      })
    ).rejects.toThrow('Correct answer cannot be empty');
  });

  it('should validate correctAnswer exists in options for MC questions', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_5' as Id<'phrasings'>,
      userId: mockUserId,
      type: 'multiple-choice',
      options: ['Option A', 'Option B', 'Option C'],
      correctAnswer: 'Option B',
    });

    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(phrasing),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (updatePhrasing as any)._handler(ctx, {
        phrasingId: phrasing._id,
        question: 'Updated question?',
        correctAnswer: 'Invalid Option',
        options: ['Option A', 'Option B', 'Option C'],
      })
    ).rejects.toThrow('Correct answer must be one of the provided options');
  });

  it('should accept valid MC question updates', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_6' as Id<'phrasings'>,
      userId: mockUserId,
      type: 'multiple-choice',
      options: ['Old A', 'Old B'],
      correctAnswer: 'Old A',
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(phrasing),
      patch: patchSpy,
    });
    const ctx = createMockCtx({ db: mockDb });

    await (updatePhrasing as any)._handler(ctx, {
      phrasingId: phrasing._id,
      question: 'Updated MC question?',
      correctAnswer: 'New B',
      options: ['New A', 'New B', 'New C'],
    });

    expect(patchSpy).toHaveBeenCalledWith(
      phrasing._id,
      expect.objectContaining({
        question: 'Updated MC question?',
        correctAnswer: 'New B',
        options: ['New A', 'New B', 'New C'],
      })
    );
  });

  it('should trim whitespace from all fields', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_7' as Id<'phrasings'>,
      userId: mockUserId,
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(phrasing),
      patch: patchSpy,
    });
    const ctx = createMockCtx({ db: mockDb });

    await (updatePhrasing as any)._handler(ctx, {
      phrasingId: phrasing._id,
      question: '  Trimmed question?  ',
      correctAnswer: '  Trimmed answer  ',
      explanation: '  Trimmed explanation  ',
    });

    expect(patchSpy).toHaveBeenCalledWith(
      phrasing._id,
      expect.objectContaining({
        question: 'Trimmed question?',
        correctAnswer: 'Trimmed answer',
        explanation: 'Trimmed explanation',
      })
    );
  });

  it('should throw error for unauthorized access', async () => {
    const otherUserId = 'other_user' as Id<'users'>;
    const phrasing = makePhrasing({
      _id: 'phrasing_8' as Id<'phrasings'>,
      userId: otherUserId,
    });

    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(phrasing),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (updatePhrasing as any)._handler(ctx, {
        phrasingId: phrasing._id,
        question: 'Hacked question',
        correctAnswer: 'Hacked answer',
      })
    ).rejects.toThrow('Phrasing not found or unauthorized');
  });

  it('should throw error for non-existent phrasing', async () => {
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(null),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (updatePhrasing as any)._handler(ctx, {
        phrasingId: 'nonexistent' as Id<'phrasings'>,
        question: 'Question',
        correctAnswer: 'Answer',
      })
    ).rejects.toThrow('Phrasing not found');
  });

  it('should handle optional explanation field', async () => {
    const phrasing = makePhrasing({
      _id: 'phrasing_9' as Id<'phrasings'>,
      userId: mockUserId,
      explanation: 'Old explanation',
    });

    const patchSpy = vi.fn();
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue(phrasing),
      patch: patchSpy,
    });
    const ctx = createMockCtx({ db: mockDb });

    // Update without explanation
    await (updatePhrasing as any)._handler(ctx, {
      phrasingId: phrasing._id,
      question: 'Updated question',
      correctAnswer: 'Updated answer',
    });

    expect(patchSpy).toHaveBeenCalledWith(
      phrasing._id,
      expect.objectContaining({
        question: 'Updated question',
        correctAnswer: 'Updated answer',
      })
    );

    // Explanation should not be in patch if not provided
    const patchCall = patchSpy.mock.calls[0][1];
    expect(patchCall).not.toHaveProperty('explanation');
  });
});
