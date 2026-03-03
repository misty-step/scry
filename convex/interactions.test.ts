import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Id } from './_generated/dataModel';
import { scheduleConceptReview } from './fsrs';
import { recordInteractionCore } from './interactions';
import { calculateConceptStatsDelta } from './lib/conceptFsrsHelpers';
import { buildInteractionContext } from './lib/interactionContext';
import { updateStatsCounters } from './lib/userStatsHelpers';

vi.mock('./fsrs', () => ({
  scheduleConceptReview: vi.fn(),
}));

vi.mock('./lib/conceptFsrsHelpers', () => ({
  calculateConceptStatsDelta: vi.fn(),
}));

vi.mock('./lib/interactionContext', () => ({
  buildInteractionContext: vi.fn(),
}));

vi.mock('./lib/userStatsHelpers', () => ({
  updateStatsCounters: vi.fn(),
}));

const mockScheduleConceptReview = vi.mocked(scheduleConceptReview);
const mockCalculateConceptStatsDelta = vi.mocked(calculateConceptStatsDelta);
const mockBuildInteractionContext = vi.mocked(buildInteractionContext);
const mockUpdateStatsCounters = vi.mocked(updateStatsCounters);

describe('recordInteractionCore', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(Date, 'now').mockReturnValue(1_700_000_000_000);
  });

  it('records interaction and preserves side-effect ordering', async () => {
    const userId = 'user-1' as Id<'users'>;
    const conceptId = 'concept-1' as Id<'concepts'>;
    const phrasingId = 'phrasing-1' as Id<'phrasings'>;

    const concept = {
      _id: conceptId,
      userId,
      fsrs: { state: 'new', nextReview: undefined },
    } as any;
    const phrasing = {
      _id: phrasingId,
      userId,
      conceptId,
      attemptCount: 2,
      correctCount: 1,
    } as any;

    const mockDb = {
      get: vi.fn(async (id: string) => {
        if (id === conceptId) return concept;
        if (id === phrasingId) return phrasing;
        return null;
      }),
      insert: vi.fn(async () => 'interaction-1' as Id<'interactions'>),
      patch: vi.fn(async () => undefined),
    };

    const ctx = { db: mockDb } as any;

    mockScheduleConceptReview.mockReturnValue({
      scheduledDays: 3,
      nextReview: 1_700_000_000_000 + 86_400_000,
      state: 'learning',
      fsrs: { state: 'learning', lapses: 0, reps: 1 },
    } as any);
    mockBuildInteractionContext.mockReturnValue({ source: 'test' } as any);
    mockCalculateConceptStatsDelta.mockReturnValue({ learningCount: 1 } as any);

    const result = await recordInteractionCore(ctx, userId, {
      conceptId,
      phrasingId,
      userAnswer: 'answer',
      isCorrect: true,
      timeSpent: 4200,
      sessionId: 'session-1',
    });

    expect(mockScheduleConceptReview).toHaveBeenCalledWith(
      concept,
      true,
      expect.objectContaining({ now: expect.any(Date) })
    );
    expect(mockBuildInteractionContext).toHaveBeenCalledWith(
      expect.objectContaining({
        sessionId: 'session-1',
        scheduledDays: 3,
        nextReview: 1_700_086_400_000,
        fsrsState: 'learning',
      })
    );

    expect(mockDb.insert).toHaveBeenCalledWith(
      'interactions',
      expect.objectContaining({
        userId,
        conceptId,
        phrasingId,
        userAnswer: 'answer',
        isCorrect: true,
        attemptedAt: 1_700_000_000_000,
        timeSpent: 4200,
        context: { source: 'test' },
      })
    );
    expect(mockDb.patch).toHaveBeenNthCalledWith(
      1,
      phrasingId,
      expect.objectContaining({
        attemptCount: 3,
        correctCount: 2,
        lastAttemptedAt: 1_700_000_000_000,
      })
    );
    expect(mockDb.patch).toHaveBeenNthCalledWith(
      2,
      conceptId,
      expect.objectContaining({
        fsrs: expect.objectContaining({ state: 'learning' }),
        updatedAt: 1_700_000_000_000,
      })
    );

    expect(mockUpdateStatsCounters).toHaveBeenCalledWith(ctx, userId, { learningCount: 1 });

    const insertOrder = mockDb.insert.mock.invocationCallOrder[0];
    const firstPatchOrder = mockDb.patch.mock.invocationCallOrder[0];
    const secondPatchOrder = mockDb.patch.mock.invocationCallOrder[1];
    const statsOrder = mockUpdateStatsCounters.mock.invocationCallOrder[0];
    expect(insertOrder).toBeLessThan(firstPatchOrder);
    expect(firstPatchOrder).toBeLessThan(secondPatchOrder);
    expect(secondPatchOrder).toBeLessThan(statsOrder);

    expect(result).toEqual(
      expect.objectContaining({
        conceptId,
        phrasingId,
        interactionId: 'interaction-1',
        nextReview: 1_700_086_400_000,
        scheduledDays: 3,
        newState: 'learning',
        totalAttempts: 3,
        totalCorrect: 2,
        lapses: 0,
        reps: 1,
      })
    );
  });

  it('skips stats update when no delta is produced', async () => {
    const userId = 'user-1' as Id<'users'>;
    const conceptId = 'concept-1' as Id<'concepts'>;
    const phrasingId = 'phrasing-1' as Id<'phrasings'>;

    const concept = { _id: conceptId, userId, fsrs: { state: 'new' } } as any;
    const phrasing = { _id: phrasingId, userId, conceptId } as any;

    const mockDb = {
      get: vi.fn().mockResolvedValueOnce(concept).mockResolvedValueOnce(phrasing),
      insert: vi.fn(async () => 'interaction-1' as Id<'interactions'>),
      patch: vi.fn(async () => undefined),
    };

    mockScheduleConceptReview.mockReturnValue({
      scheduledDays: 1,
      nextReview: 1_700_000_060_000,
      state: 'learning',
      fsrs: { state: 'learning' },
    } as any);
    mockBuildInteractionContext.mockReturnValue({ source: 'test' } as any);
    mockCalculateConceptStatsDelta.mockReturnValue(null);

    await recordInteractionCore({ db: mockDb } as any, userId, {
      conceptId,
      phrasingId,
      userAnswer: 'answer',
      isCorrect: true,
    });

    expect(mockUpdateStatsCounters).not.toHaveBeenCalled();
  });

  it('throws when concept is missing', async () => {
    const mockDb = {
      get: vi.fn(async () => null),
      insert: vi.fn(),
      patch: vi.fn(),
    };

    await expect(
      recordInteractionCore({ db: mockDb } as any, 'user-1' as Id<'users'>, {
        conceptId: 'missing-concept' as Id<'concepts'>,
        phrasingId: 'phrasing-1' as Id<'phrasings'>,
        userAnswer: 'answer',
        isCorrect: true,
      })
    ).rejects.toThrow('Concept not found or unauthorized');

    expect(mockDb.insert).not.toHaveBeenCalled();
    expect(mockDb.patch).not.toHaveBeenCalled();
  });

  it('throws when concept is owned by a different user', async () => {
    const concept = {
      _id: 'concept-1' as Id<'concepts'>,
      userId: 'other-user' as Id<'users'>,
      fsrs: { state: 'new' },
    } as any;

    const mockDb = {
      get: vi.fn(async () => concept),
      insert: vi.fn(),
      patch: vi.fn(),
    };

    await expect(
      recordInteractionCore({ db: mockDb } as any, 'user-1' as Id<'users'>, {
        conceptId: 'concept-1' as Id<'concepts'>,
        phrasingId: 'phrasing-1' as Id<'phrasings'>,
        userAnswer: 'answer',
        isCorrect: true,
      })
    ).rejects.toThrow('Concept not found or unauthorized');

    expect(mockDb.insert).not.toHaveBeenCalled();
    expect(mockDb.patch).not.toHaveBeenCalled();
  });

  it('throws when phrasing does not belong to concept', async () => {
    const userId = 'user-1' as Id<'users'>;
    const conceptId = 'concept-1' as Id<'concepts'>;
    const phrasingId = 'phrasing-1' as Id<'phrasings'>;

    const concept = {
      _id: conceptId,
      userId,
      fsrs: { state: 'new' },
    } as any;
    const phrasing = {
      _id: phrasingId,
      userId,
      conceptId: 'different-concept' as Id<'concepts'>,
    } as any;

    const mockDb = {
      get: vi.fn().mockResolvedValueOnce(concept).mockResolvedValueOnce(phrasing),
      insert: vi.fn(),
      patch: vi.fn(),
    };

    await expect(
      recordInteractionCore({ db: mockDb } as any, userId, {
        conceptId,
        phrasingId,
        userAnswer: 'answer',
        isCorrect: true,
      })
    ).rejects.toThrow('Phrasing not found or unauthorized');

    expect(mockDb.insert).not.toHaveBeenCalled();
    expect(mockDb.patch).not.toHaveBeenCalled();
  });
});
