import { v } from 'convex/values';
import { mutation, query } from './_generated/server';

export const backfillTotalPhrasings = mutation({
  args: {
    dryRun: v.boolean(),
  },
  handler: async (ctx, args) => {
    // Fetch batch of userStats
    const stats = await ctx.db.query('userStats').take(500);
    let processed = 0;

    for (const stat of stats) {
      if (stat.totalPhrasings === undefined) {
        // Count active phrasings for this user
        const phrasings = await ctx.db
          .query('phrasings')
          .withIndex('by_user_active', (q) =>
            q.eq('userId', stat.userId).eq('deletedAt', undefined).eq('archivedAt', undefined)
          )
          .collect();

        if (!args.dryRun) {
          await ctx.db.patch(stat._id, {
            totalPhrasings: phrasings.length,
          });
        }
        processed++;
      }
    }

    return { processed, dryRun: args.dryRun };
  },
});

export const diagnosticBackfillTotalPhrasings = query({
  args: {},
  handler: async (ctx) => {
    const stats = await ctx.db.query('userStats').take(1000);
    const count = stats.filter((s) => s.totalPhrasings === undefined).length;
    return count;
  },
});
