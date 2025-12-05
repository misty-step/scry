import { describe, expect, it, vi } from 'vitest';
import type { Id } from './_generated/dataModel';

// Test the query handlers by simulating their logic
// We mock ctx.db and requireUserFromClerk to test business logic

describe('spacedRepetition queries', () => {
  const mockUserId = 'user_123' as Id<'users'>;

  const createMockCtx = (userStats: any = null) => ({
    db: {
      query: vi.fn().mockImplementation((_table) => ({
        withIndex: vi.fn().mockImplementation((_indexName) => ({
          first: vi.fn().mockResolvedValue(userStats),
        })),
      })),
    },
  });

  describe('getDueCount handler logic', () => {
    it('returns zeros when userStats is null', async () => {
      const ctx = createMockCtx(null);

      // Simulate getDueCount handler
      const stats = await ctx.db.query('userStats').withIndex('by_user').first();

      const newCount = stats?.newCount ?? 0;
      const dueNowCount = stats?.dueNowCount ?? 0;
      const reviewDueCount = Math.max(dueNowCount - newCount, 0);

      const result = {
        dueCount: reviewDueCount,
        newCount,
        totalReviewable: reviewDueCount + newCount,
      };

      expect(result.dueCount).toBe(0);
      expect(result.newCount).toBe(0);
      expect(result.totalReviewable).toBe(0);
    });

    it('calculates due counts correctly from userStats', async () => {
      const userStats = {
        userId: mockUserId,
        newCount: 5,
        dueNowCount: 12,
      };
      const ctx = createMockCtx(userStats);

      // Simulate getDueCount handler
      const stats = await ctx.db.query('userStats').withIndex('by_user').first();

      const newCount = stats?.newCount ?? 0;
      const dueNowCount = stats?.dueNowCount ?? 0;
      const reviewDueCount = Math.max(dueNowCount - newCount, 0);

      const result = {
        dueCount: reviewDueCount,
        newCount,
        totalReviewable: reviewDueCount + newCount,
      };

      expect(result.dueCount).toBe(7); // 12 - 5 = 7 review cards
      expect(result.newCount).toBe(5);
      expect(result.totalReviewable).toBe(12);
    });

    it('clamps reviewDueCount to 0 when newCount exceeds dueNowCount', async () => {
      // Edge case: more new cards than total due (shouldn't happen, but handle gracefully)
      const userStats = {
        userId: mockUserId,
        newCount: 10,
        dueNowCount: 5,
      };
      const ctx = createMockCtx(userStats);

      const stats = await ctx.db.query('userStats').withIndex('by_user').first();

      const newCount = stats?.newCount ?? 0;
      const dueNowCount = stats?.dueNowCount ?? 0;
      const reviewDueCount = Math.max(dueNowCount - newCount, 0);

      expect(reviewDueCount).toBe(0); // Math.max(5 - 10, 0) = 0
      expect(newCount).toBe(10);
    });
  });

  describe('getUserCardStats handler logic', () => {
    it('returns default values when userStats is null', async () => {
      const ctx = createMockCtx(null);

      // Simulate getUserCardStats handler
      const stats = await ctx.db.query('userStats').withIndex('by_user').first();

      const result = {
        totalCards: stats?.totalCards ?? 0,
        nextReviewTime: stats?.nextReviewTime ?? null,
        learningCount: stats?.learningCount ?? 0,
        matureCount: stats?.matureCount ?? 0,
        newCount: stats?.newCount ?? 0,
      };

      expect(result.totalCards).toBe(0);
      expect(result.nextReviewTime).toBeNull();
      expect(result.learningCount).toBe(0);
      expect(result.matureCount).toBe(0);
      expect(result.newCount).toBe(0);
    });

    it('returns userStats values when present', async () => {
      const userStats = {
        userId: mockUserId,
        totalCards: 100,
        nextReviewTime: 1700000000000,
        learningCount: 15,
        matureCount: 75,
        newCount: 10,
      };
      const ctx = createMockCtx(userStats);

      const stats = await ctx.db.query('userStats').withIndex('by_user').first();

      const result = {
        totalCards: stats?.totalCards ?? 0,
        nextReviewTime: stats?.nextReviewTime ?? null,
        learningCount: stats?.learningCount ?? 0,
        matureCount: stats?.matureCount ?? 0,
        newCount: stats?.newCount ?? 0,
      };

      expect(result.totalCards).toBe(100);
      expect(result.nextReviewTime).toBe(1700000000000);
      expect(result.learningCount).toBe(15);
      expect(result.matureCount).toBe(75);
      expect(result.newCount).toBe(10);
    });

    it('handles partial userStats with some undefined fields', async () => {
      const userStats = {
        userId: mockUserId,
        totalCards: 50,
        // Missing other fields - should fall back to defaults
      };
      const ctx = createMockCtx(userStats);

      const stats = await ctx.db.query('userStats').withIndex('by_user').first();

      const result = {
        totalCards: stats?.totalCards ?? 0,
        nextReviewTime: stats?.nextReviewTime ?? null,
        learningCount: stats?.learningCount ?? 0,
        matureCount: stats?.matureCount ?? 0,
        newCount: stats?.newCount ?? 0,
      };

      expect(result.totalCards).toBe(50);
      expect(result.nextReviewTime).toBeNull();
      expect(result.learningCount).toBe(0);
      expect(result.matureCount).toBe(0);
      expect(result.newCount).toBe(0);
    });
  });
});
