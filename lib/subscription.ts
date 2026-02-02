/**
 * Subscription access control logic.
 * Pure function for both client and server use.
 */

export type SubscriptionStatus =
  | 'trialing'
  | 'active'
  | 'past_due'
  | 'canceled'
  | 'unpaid'
  | 'incomplete'
  | 'incomplete_expired';

export type Subscription = {
  status: SubscriptionStatus;
  trialEnd?: number | null;
  currentPeriodEnd: number;
  cancelAtPeriodEnd: boolean;
};

export type SubscriptionAccess = {
  hasAccess: boolean;
  status: 'active' | 'trialing' | 'expired' | 'canceled' | 'past_due' | 'none';
  trialDaysRemaining?: number;
  currentPeriodEnd?: number;
  message?: string;
};

/**
 * Check if user has access based on subscription state.
 *
 * Access rules:
 * - active: full access
 * - trialing: access until trial ends
 * - past_due: access (grace period)
 * - canceled: access until period ends
 * - everything else: no access (hard gate)
 */
export function checkAccess(subscription: Subscription | null | undefined): SubscriptionAccess {
  // No subscription = no access
  if (!subscription) {
    return {
      hasAccess: false,
      status: 'none',
      message: 'Start your free trial to access all features',
    };
  }

  const now = Date.now();

  // Active subscription - full access
  if (subscription.status === 'active') {
    return {
      hasAccess: true,
      status: 'active',
      currentPeriodEnd: subscription.currentPeriodEnd,
    };
  }

  // Trialing - access if trial hasn't expired
  if (subscription.status === 'trialing') {
    const trialEnd = subscription.trialEnd;

    if (trialEnd && trialEnd > now) {
      const trialDaysRemaining = Math.ceil((trialEnd - now) / (1000 * 60 * 60 * 24));
      return {
        hasAccess: true,
        status: 'trialing',
        trialDaysRemaining,
        currentPeriodEnd: trialEnd,
        message:
          trialDaysRemaining === 1
            ? 'Trial ends tomorrow'
            : `${trialDaysRemaining} days left in trial`,
      };
    }

    // Trial expired
    return {
      hasAccess: false,
      status: 'expired',
      message: 'Your trial has ended. Subscribe to continue.',
    };
  }

  // Past due - grace period, warn user
  if (subscription.status === 'past_due') {
    return {
      hasAccess: true,
      status: 'past_due',
      currentPeriodEnd: subscription.currentPeriodEnd,
      message: 'Payment failed. Please update your payment method.',
    };
  }

  // Canceled but period not ended - access until period ends
  if (subscription.status === 'canceled') {
    if (subscription.currentPeriodEnd > now) {
      const daysRemaining = Math.ceil(
        (subscription.currentPeriodEnd - now) / (1000 * 60 * 60 * 24)
      );
      return {
        hasAccess: true,
        status: 'canceled',
        currentPeriodEnd: subscription.currentPeriodEnd,
        message: `Subscription canceled. Access ends in ${daysRemaining} day${daysRemaining === 1 ? '' : 's'}.`,
      };
    }

    // Period ended
    return {
      hasAccess: false,
      status: 'expired',
      message: 'Your subscription has ended. Resubscribe to continue.',
    };
  }

  // All other statuses (unpaid, incomplete, incomplete_expired) = no access
  return {
    hasAccess: false,
    status: 'expired',
    message: 'Your subscription is inactive. Please update your payment method.',
  };
}
