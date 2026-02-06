import { v } from 'convex/values';
import { mutation, query } from './_generated/server';
import { requireUserFromClerk } from './clerk';

// Status type matching schema
const subscriptionStatus = v.union(
  v.literal('trialing'),
  v.literal('active'),
  v.literal('past_due'),
  v.literal('canceled'),
  v.literal('unpaid'),
  v.literal('incomplete'),
  v.literal('incomplete_expired')
);

/**
 * Mutation to sync subscription state from Stripe webhooks.
 * Called by the webhook handler in Next.js API routes.
 * Protected by webhook secret validation.
 */
export const syncSubscription = mutation({
  args: {
    webhookSecret: v.string(),
    clerkUserId: v.string(),
    stripeCustomerId: v.string(),
    stripeSubscriptionId: v.string(),
    stripePriceId: v.string(),
    status: subscriptionStatus,
    trialStart: v.optional(v.number()),
    trialEnd: v.optional(v.number()),
    currentPeriodStart: v.number(),
    currentPeriodEnd: v.number(),
    cancelAtPeriodEnd: v.boolean(),
    canceledAt: v.optional(v.number()),
  },
  handler: async (ctx, args) => {
    // Validate webhook secret
    const expectedSecret = process.env.CONVEX_WEBHOOK_SECRET;
    if (!expectedSecret || args.webhookSecret !== expectedSecret) {
      throw new Error('Invalid webhook secret');
    }

    // Remove webhookSecret from args before using
    const { webhookSecret: _, ...subscriptionData } = args;

    const now = Date.now();

    // Find user by clerkId
    const user = await ctx.db
      .query('users')
      .withIndex('by_clerk_id', (q) => q.eq('clerkId', subscriptionData.clerkUserId))
      .first();

    if (!user) {
      console.error('User not found for clerkUserId:', subscriptionData.clerkUserId);
      return null;
    }

    // Check if subscription exists
    const existing = await ctx.db
      .query('subscriptions')
      .withIndex('by_stripe_subscription', (q) =>
        q.eq('stripeSubscriptionId', subscriptionData.stripeSubscriptionId)
      )
      .first();

    if (existing) {
      // Update existing subscription
      await ctx.db.patch(existing._id, {
        status: subscriptionData.status,
        stripePriceId: subscriptionData.stripePriceId,
        trialStart: subscriptionData.trialStart,
        trialEnd: subscriptionData.trialEnd,
        currentPeriodStart: subscriptionData.currentPeriodStart,
        currentPeriodEnd: subscriptionData.currentPeriodEnd,
        cancelAtPeriodEnd: subscriptionData.cancelAtPeriodEnd,
        canceledAt: subscriptionData.canceledAt,
        updatedAt: now,
      });
      return existing._id;
    }

    // Create new subscription
    const subscriptionId = await ctx.db.insert('subscriptions', {
      userId: user._id,
      clerkUserId: subscriptionData.clerkUserId,
      stripeCustomerId: subscriptionData.stripeCustomerId,
      stripeSubscriptionId: subscriptionData.stripeSubscriptionId,
      stripePriceId: subscriptionData.stripePriceId,
      status: subscriptionData.status,
      trialStart: subscriptionData.trialStart,
      trialEnd: subscriptionData.trialEnd,
      currentPeriodStart: subscriptionData.currentPeriodStart,
      currentPeriodEnd: subscriptionData.currentPeriodEnd,
      cancelAtPeriodEnd: subscriptionData.cancelAtPeriodEnd,
      canceledAt: subscriptionData.canceledAt,
      createdAt: now,
      updatedAt: now,
    });

    return subscriptionId;
  },
});

/**
 * Get current user's subscription.
 */
export const getMySubscription = query({
  args: {},
  handler: async (ctx) => {
    const user = await requireUserFromClerk(ctx);

    const subscription = await ctx.db
      .query('subscriptions')
      .withIndex('by_user', (q) => q.eq('userId', user._id))
      .first();

    return subscription;
  },
});

/**
 * Get subscription by Stripe subscription ID (for webhook lookups).
 */
export const getByStripeSubscriptionId = query({
  args: { stripeSubscriptionId: v.string() },
  handler: async (ctx, { stripeSubscriptionId }) => {
    return await ctx.db
      .query('subscriptions')
      .withIndex('by_stripe_subscription', (q) =>
        q.eq('stripeSubscriptionId', stripeSubscriptionId)
      )
      .first();
  },
});
