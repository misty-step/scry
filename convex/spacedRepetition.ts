/**
 * Compatibility layer for spaced repetition stats after migration to concepts/phrasings.
 *
 * Keep the public API stable (`getDueCount`, `getUserCardStats`) but source data
 * from concept-level FSRS state and cached userStats. All question-table logic
 * has been removed.
 */

import { v } from 'convex/values';
import { query } from './_generated/server';
import { requireUserFromClerk } from './clerk';

/**
 * Return counts of due + new concepts (reviewable units).
 * Uses concept-level FSRS via concepts.getConceptsDueCount logic.
 */
export const getDueCount = query({
  args: {},
  handler: async (ctx) => {
    const user = await requireUserFromClerk(ctx);
    const userId = user._id;

    const stats = await ctx.db
      .query('userStats')
      .withIndex('by_user', (q) => q.eq('userId', userId))
      .first();

    const newCount = stats?.newCount ?? 0;
    const dueNowCount = stats?.dueNowCount ?? 0;
    const reviewDueCount = Math.max(dueNowCount - newCount, 0);

    return {
      dueCount: reviewDueCount,
      newCount,
      totalReviewable: reviewDueCount + newCount,
    };
  },
});

/**
 * Basic card stats for UI empty states and health checks.
 * Relies on cached userStats maintained by concept-level scheduling.
 */
export const getUserCardStats = query({
  args: {
    _refreshTimestamp: v.optional(v.float64()),
  },
  handler: async (ctx, _args) => {
    const user = await requireUserFromClerk(ctx);
    const userId = user._id;

    const stats = await ctx.db
      .query('userStats')
      .withIndex('by_user', (q) => q.eq('userId', userId))
      .first();

    return {
      totalCards: stats?.totalCards ?? 0,
      nextReviewTime: stats?.nextReviewTime ?? null,
      learningCount: stats?.learningCount ?? 0,
      matureCount: stats?.matureCount ?? 0,
      newCount: stats?.newCount ?? 0,
    };
  },
});
