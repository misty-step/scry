import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { requireUserFromClerk } from '@/convex/clerk';
import {
  getDueCount,
  getUserCardStats,
  getUserCardStats_DEPRECATED,
} from '@/convex/spacedRepetition';
import { createMockCtx, createMockDb, makeQuestion } from '@/tests/helpers';

vi.mock('@/convex/clerk', () => ({
  requireUserFromClerk: vi.fn(),
}));

const mockedRequireUser = vi.mocked(requireUserFromClerk);

describe('spacedRepetition queries', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-16T12:00:00Z'));
    mockedRequireUser.mockResolvedValue({ _id: 'users_1' } as any);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  describe('getDueCount', () => {
    it('returns zeros when stats are missing', async () => {
      const db = createMockDb(undefined, []); // first() will resolve to null
      db.query.mockReturnValue({
        withIndex: vi.fn().mockReturnThis(),
        first: vi.fn().mockResolvedValue(null),
      } as any);

      const ctx = createMockCtx({ db });
      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

      const result = await (getDueCount as any)._handler(ctx, {});

      expect(result).toEqual({ dueCount: 0, newCount: 0, totalReviewable: 0 });
      expect(consoleSpy).toHaveBeenCalled();
    });

    it('subtracts new cards from dueNowCount to compute review due count', async () => {
      const stats = { dueNowCount: 12, newCount: 5 };
      const db = createMockDb(undefined, [stats]);
      const ctx = createMockCtx({ db });

      const result = await (getDueCount as any)._handler(ctx, {});

      expect(result.dueCount).toBe(7);
      expect(result.newCount).toBe(5);
      expect(result.totalReviewable).toBe(12);
    });
  });

  describe('getUserCardStats', () => {
    it('returns defaults when no stats record exists', async () => {
      const db = createMockDb(undefined, []);
      const ctx = createMockCtx({ db });

      const result = await (getUserCardStats as any)._handler(ctx, {
        _refreshTimestamp: undefined,
      });

      expect(result).toEqual({
        totalCards: 0,
        nextReviewTime: null,
        learningCount: 0,
        matureCount: 0,
        newCount: 0,
      });
    });

    it('returns cached stats when available', async () => {
      const stats = {
        totalCards: 10,
        nextReviewTime: Date.now() + 60_000,
        learningCount: 3,
        matureCount: 4,
        newCount: 3,
      };
      const db = createMockDb(undefined, [stats]);
      const ctx = createMockCtx({ db });

      const result = await (getUserCardStats as any)._handler(ctx, {});

      expect(result).toEqual({
        totalCards: 10,
        nextReviewTime: stats.nextReviewTime,
        learningCount: 3,
        matureCount: 4,
        newCount: 3,
      });
    });
  });

  describe('getUserCardStats_DEPRECATED', () => {
    it('returns empty stats when user has no cards', async () => {
      const db = createMockDb(undefined, []);
      const ctx = createMockCtx({ db });

      const result = await (getUserCardStats_DEPRECATED as any)._handler(ctx, {});

      expect(result).toEqual({
        totalCards: 0,
        nextReviewTime: null,
        learningCount: 0,
        matureCount: 0,
        newCount: 0,
      });
    });

    it('computes counts and earliest future review time from questions', async () => {
      const base = Date.now();
      const questions = [
        makeQuestion({ state: 'new', nextReview: undefined }),
        makeQuestion({ state: 'learning', nextReview: base + 60_000 }),
        makeQuestion({ state: 'relearning', nextReview: base + 90_000 }),
        makeQuestion({ state: 'review', nextReview: base + 120_000 }),
        makeQuestion({ state: 'review', nextReview: base - 60_000 }),
      ];

      const db = createMockDb(undefined, questions);
      const ctx = createMockCtx({ db });

      const result = await (getUserCardStats_DEPRECATED as any)._handler(ctx, {});

      expect(result.totalCards).toBe(5);
      expect(result.newCount).toBe(1);
      expect(result.learningCount).toBe(2); // learning + relearning
      expect(result.matureCount).toBe(2);
      expect(result.nextReviewTime).toBe(base + 60_000); // earliest future review
    });
  });
});
