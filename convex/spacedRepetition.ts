/**
 * Spaced Repetition System - Pure FSRS Implementation
 *
 * This module implements the Free Spaced Repetition Scheduler (FSRS) algorithm
 * without modifications or comfort features. The system respects memory science
 * absolutely - no daily limits, no artificial interleaving, no comfort features.
 *
 * Queue Priority System (lower number = higher priority):
 *
 * 1. Ultra-fresh new questions (< 1 hour old): -2.0 to -1.37
 *    - Highest priority for immediate encoding into memory
 *    - Exponentially decays toward standard new priority
 *
 * 2. Fresh new questions (1-24 hours old): -1.37 to -1.0
 *    - Still prioritized but with diminishing boost
 *    - Prevents stale new questions from blocking reviews
 *
 * 3. Standard new questions (> 24 hours old): -1.0
 *    - Regular FSRS new card priority
 *    - Must be learned before reviews
 *
 * 4. Due review questions: 0.0 to 1.0
 *    - Based on FSRS retrievability calculation
 *    - Lower retrievability = higher priority
 *    - 0.0 = completely forgotten, needs immediate review
 *    - 1.0 = perfect recall, can wait
 *
 * Key Principles:
 * - The forgetting curve doesn't care about comfort
 * - If 300 cards are due, show 300 cards
 * - Natural consequences teach sustainable habits
 * - Every "improvement" that adds comfort reduces effectiveness
 *
 * @module spacedRepetition
 */

import { v } from 'convex/values';
import type { Doc } from './_generated/dataModel';
import { query } from './_generated/server';
import { requireUserFromClerk } from './clerk';
import { defaultEngine as conceptEngine } from './fsrs';
import { questionToConceptFsrsState } from './lib/conceptFsrsHelpers';

// Export for testing
export { calculateFreshnessDecay, calculateRetrievabilityScore };

/**
 * Calculate freshness priority with exponential decay over 24 hours
 *
 * @param hoursSinceCreation - Hours since the question was created
 * @returns A priority boost from 0 to 1 (1 = maximum freshness)
 */
function calculateFreshnessDecay(hoursSinceCreation: number): number {
  if (hoursSinceCreation < 0) {
    // Gracefully handle minor clock skew by treating as maximum freshness
    // This prevents crashes when client/server times are slightly misaligned
    return 1.0;
  }

  // Exponential decay with 24-hour half-life
  // At 0 hours: 1.0 (maximum freshness)
  // At 24 hours: ~0.37 (e^-1)
  // At 48 hours: ~0.14 (e^-2)
  // After 72 hours: effectively 0
  return Math.exp(-hoursSinceCreation / 24);
}

/**
 * Calculate enhanced retrievability score for queue prioritization
 *
 * Implements Pure FSRS with fresh question priority and exponential decay:
 * - Ultra-fresh questions (0-24 hours): -2 to -1 with exponential decay
 * - Regular new questions (>24 hours): -1 (standard new priority)
 * - Reviewed questions: 0-1 (based on FSRS calculation)
 *
 * The freshness decay ensures newly generated questions get immediate priority
 * but gradually lose that boost over 24 hours, preventing stale new questions
 * from indefinitely blocking important reviews.
 *
 * @param question - The question document to calculate priority for
 * @param now - Current date/time for calculation (defaults to now)
 * @returns Priority score: -2 to -1 for new questions, 0 to 1 for reviewed questions
 */
function calculateRetrievabilityScore(question: Doc<'questions'>, now: Date = new Date()): number {
  // Check if question has never been reviewed
  // Note: After CRUD refactor, new questions have FSRS fields initialized on creation,
  // so we check state === 'new' instead of relying solely on undefined nextReview.
  // This ensures newly created cards still receive the -2 to -1 freshness boost.
  if (question.state === 'new' || question.nextReview === undefined || question.reps === 0) {
    // New question - apply freshness decay
    const hoursSinceCreation = (now.getTime() - question._creationTime) / 3600000;

    // Calculate freshness boost (1.0 at creation, decays to ~0 after 72 hours)
    const freshnessBoost = calculateFreshnessDecay(hoursSinceCreation);

    // Map freshness to priority range: -2 (ultra-fresh) to -1 (standard new)
    // freshnessBoost of 1.0 gives -2, freshnessBoost of 0 gives -1
    return -1 - freshnessBoost;
  }

  // Reviewed question - reuse concept FSRS engine for consistent scoring
  const state = questionToConceptFsrsState(question, now.getTime());
  return conceptEngine.getRetrievability(state, now);
}

// NOTE: scheduleReview and getNextReview functions were removed in v2.4.0
// Use concepts.recordInteraction and concepts.getDue instead

/**
 * Get count of questions due for review
 *
 * Returns the REAL count - no limits, no filtering, no comfort.
 * This is your actual learning debt:
 * - newCount: Questions never reviewed (highest priority)
 * - dueCount: Questions past their optimal review time
 * - totalReviewable: The truth about what needs review
 *
 * Bandwidth optimization: Hybrid approach for correctness + efficiency
 * - New cards: Use cached newCount from userStats (time-agnostic, always due)
 * - Due cards: Query time-filtered learning/mature cards (nextReview <= now)
 * - Impact: 75-95% bandwidth reduction (vs 99.996% with pure cache)
 * - Rationale: userStats counters are state-based, NOT time-aware
 *
 * Note: Originally attempted pure O(1) cache lookup, but Codex review (PR #53)
 * correctly identified that learningCount + matureCount counts ALL cards in those
 * states, not just cards where nextReview <= now. This hybrid approach maintains
 * API correctness while achieving significant bandwidth savings.
 */
export const getDueCount = query({
  args: {},

  handler: async (ctx) => {
    const user = await requireUserFromClerk(ctx);
    const userId = user._id;

    // Get cached stats - fully reactive via mutation updates
    // No time-filtered queries needed: dueNowCount is maintained by scheduleReview
    const stats = await ctx.db
      .query('userStats')
      .withIndex('by_user', (q) => q.eq('userId', userId))
      .first();

    if (!stats) {
      console.warn(
        'Missing userStats for user',
        userId,
        '- returning zeros. This may indicate reconciliation failure.'
      );
    }

    // Use time-aware cached counters (updated by scheduleReview mutations)
    // This enables true Convex reactivity: when scheduleReview updates userStats,
    // this query automatically re-runs via WebSocket (no polling needed)
    const dueNowCount = stats?.dueNowCount || 0;
    const newCount = stats?.newCount || 0;

    // New cards are always due, so dueNowCount already includes them.
    // Subtract them out to keep dueCount aligned with "review" cards only.
    const reviewDueCount = Math.max(dueNowCount - newCount, 0);

    return {
      dueCount: reviewDueCount,
      newCount,
      totalReviewable: reviewDueCount + newCount,
    };
  },
});

/**
 * Get user's card statistics and next scheduled review time
 * Used for context-aware empty states
 *
 * Bandwidth optimization: O(1) query using cached userStats table
 * instead of O(N) collection scan. Updated incrementally on card state changes.
 */
export const getUserCardStats = query({
  args: {
    _refreshTimestamp: v.optional(v.float64()),
  },

  handler: async (ctx, _args) => {
    const user = await requireUserFromClerk(ctx);
    const userId = user._id;

    // Query cached stats (O(1) vs O(N) collection scan)
    const stats = await ctx.db
      .query('userStats')
      .withIndex('by_user', (q) => q.eq('userId', userId))
      .first();

    // Return default stats if no record exists (new user case)
    if (!stats) {
      return {
        totalCards: 0,
        nextReviewTime: null,
        learningCount: 0,
        matureCount: 0,
        newCount: 0,
      };
    }

    return {
      totalCards: stats.totalCards,
      nextReviewTime: stats.nextReviewTime ?? null,
      learningCount: stats.learningCount,
      matureCount: stats.matureCount,
      newCount: stats.newCount,
    };
  },
});

/**
 * @deprecated Use getUserCardStats instead (reads from cached userStats table)
 * This function performs O(N) collection scan and will be removed after migration
 */
export const getUserCardStats_DEPRECATED = query({
  args: {
    _refreshTimestamp: v.optional(v.float64()),
  },

  handler: async (ctx, _args) => {
    const user = await requireUserFromClerk(ctx);
    const userId = user._id;
    const now = Date.now();

    // Get all user's cards (not deleted)
    const allCards = await ctx.db
      .query('questions')
      .withIndex('by_user', (q) => q.eq('userId', userId))
      .filter((q) => q.eq(q.field('deletedAt'), undefined))
      .collect();

    const totalCards = allCards.length;

    if (totalCards === 0) {
      return {
        totalCards: 0,
        nextReviewTime: null,
        learningCount: 0,
        matureCount: 0,
        newCount: 0,
      };
    }

    // Find the earliest next review time (for cards not yet due)
    const futureReviews = allCards
      .filter((card) => card.nextReview && card.nextReview > now)
      .sort((a, b) => (a.nextReview || 0) - (b.nextReview || 0));

    const nextReviewTime = futureReviews[0]?.nextReview || null;

    // Count cards by state
    let learningCount = 0;
    let matureCount = 0;
    let newCount = 0;

    for (const card of allCards) {
      if (!card.state || card.state === 'new') {
        newCount++;
      } else if (card.state === 'learning' || card.state === 'relearning') {
        learningCount++;
      } else if (card.state === 'review') {
        matureCount++;
      }
    }

    return {
      totalCards,
      nextReviewTime,
      learningCount,
      matureCount,
      newCount,
    };
  },
});
