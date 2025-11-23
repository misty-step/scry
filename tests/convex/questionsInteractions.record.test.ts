import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Id } from '@/convex/_generated/dataModel';
import { requireUserFromClerk } from '@/convex/clerk';
import { recordInteraction } from '@/convex/questionsInteractions';
import { getScheduler } from '@/convex/scheduling';
import { createMockCtx, createMockDb } from '@/tests/helpers';

vi.mock('@/convex/clerk', () => ({
  requireUserFromClerk: vi.fn(),
}));

vi.mock('@/convex/scheduling', () => ({
  getScheduler: vi.fn(),
}));

const mockedRequireUser = vi.mocked(requireUserFromClerk);
const mockedGetScheduler = vi.mocked(getScheduler);

describe('recordInteraction mutation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockedRequireUser.mockResolvedValue({
      _id: 'user_1' as Id<'users'>,
      _creationTime: Date.now(),
      email: 'test@example.com',
    } as any);
  });

  it('rejects when question not owned by user', async () => {
    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue({ _id: 'q1', userId: 'other_user' }),
    });
    const ctx = createMockCtx({ db: mockDb });

    await expect(
      (recordInteraction as any)._handler(ctx, {
        questionId: 'q1',
        userAnswer: 'A',
        isCorrect: true,
      })
    ).rejects.toThrow('Question not found or unauthorized: q1');
  });

  it('initializes FSRS for new card and stores context/sessionId', async () => {
    const insertSpy = vi.fn();
    const patchSpy = vi.fn();
    const scheduler = {
      initializeCard: vi.fn().mockReturnValue({ state: 'new', nextReview: 123 }),
      scheduleNextReview: vi.fn().mockReturnValue({
        dbFields: { nextReview: 456, scheduledDays: 1, state: 'learning' },
      }),
    };
    mockedGetScheduler.mockReturnValue(scheduler as any);

    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue({
        _id: 'q1',
        userId: 'user_1',
        attemptCount: 0,
        correctCount: 0,
      }),
      insert: insertSpy,
      patch: patchSpy,
    });
    const ctx = createMockCtx({ db: mockDb });

    await (recordInteraction as any)._handler(ctx, {
      questionId: 'q1',
      userAnswer: 'A',
      isCorrect: true,
      sessionId: 's-1',
      timeSpent: 1200,
    });

    expect(scheduler.initializeCard).toHaveBeenCalled();
    expect(scheduler.scheduleNextReview).toHaveBeenCalledWith(
      expect.objectContaining({ _id: 'q1' }),
      true,
      expect.any(Date)
    );

    expect(insertSpy).toHaveBeenCalledWith(
      'interactions',
      expect.objectContaining({
        userId: 'user_1',
        questionId: 'q1',
        sessionId: 's-1',
        timeSpent: 1200,
        context: {
          sessionId: 's-1',
          scheduledDays: 1,
          nextReview: 456,
          fsrsState: 'learning',
        },
      })
    );

    expect(patchSpy).toHaveBeenCalledWith(
      'q1',
      expect.objectContaining({
        attemptCount: 1,
        correctCount: 1,
        nextReview: 456,
        scheduledDays: 1,
        state: 'learning',
      })
    );
  });

  it('uses existing FSRS state and updates counters on repeat', async () => {
    const insertSpy = vi.fn();
    const patchSpy = vi.fn();
    const scheduler = {
      initializeCard: vi.fn(),
      scheduleNextReview: vi.fn().mockReturnValue({
        dbFields: { nextReview: 789, scheduledDays: 3, state: 'review' },
      }),
    };
    mockedGetScheduler.mockReturnValue(scheduler as any);

    const mockDb = createMockDb({
      get: vi.fn().mockResolvedValue({
        _id: 'q1',
        userId: 'user_1',
        attemptCount: 2,
        correctCount: 1,
        state: 'learning',
      }),
      insert: insertSpy,
      patch: patchSpy,
    });
    const ctx = createMockCtx({ db: mockDb });

    await (recordInteraction as any)._handler(ctx, {
      questionId: 'q1',
      userAnswer: 'B',
      isCorrect: false,
    });

    expect(scheduler.initializeCard).not.toHaveBeenCalled();
    expect(scheduler.scheduleNextReview).toHaveBeenCalledWith(
      expect.objectContaining({ state: 'learning' }),
      false,
      expect.any(Date)
    );

    expect(patchSpy).toHaveBeenCalledWith(
      'q1',
      expect.objectContaining({
        attemptCount: 3,
        correctCount: 1,
        nextReview: 789,
        scheduledDays: 3,
        state: 'review',
      })
    );
  });
});

// Using shared createMockCtx from tests/helpers
