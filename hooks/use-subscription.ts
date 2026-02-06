'use client';

import { useQuery } from 'convex/react';
import { api } from '@/convex/_generated/api';
import { checkAccess, type SubscriptionAccess } from '@/lib/subscription';

/**
 * Hook to get current user's subscription and access status.
 *
 * Usage:
 * ```tsx
 * const { subscription, access, isLoading } = useSubscription();
 *
 * if (!access.hasAccess) {
 *   return <UpgradePrompt message={access.message} />;
 * }
 * ```
 */
export function useSubscription() {
  const subscription = useQuery(api.subscriptions.getMySubscription);

  const isLoading = subscription === undefined;

  const access: SubscriptionAccess = isLoading
    ? { hasAccess: false, status: 'none' } // Optimistic: no access while loading
    : checkAccess(subscription);

  return {
    subscription,
    access,
    isLoading,

    // Convenience booleans
    hasAccess: access.hasAccess,
    isTrialing: access.status === 'trialing',
    isActive: access.status === 'active',
    isCanceled: access.status === 'canceled',
    isPastDue: access.status === 'past_due',

    // For UI
    trialDaysRemaining: access.trialDaysRemaining,
    statusMessage: access.message,
  };
}
