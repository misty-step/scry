'use client';

import { Analytics } from '@vercel/analytics/react';
import type { AnalyticsProps } from '@vercel/analytics/react';

/**
 * Client-side wrapper for Vercel Analytics with beforeSend filtering.
 *
 * Filters sensitive URLs from analytics tracking:
 * - Webhook endpoints
 * - URLs with tokens/keys/secrets in query params
 */
export function AnalyticsWrapper() {
  const beforeSend: AnalyticsProps['beforeSend'] = (event) => {
    // Filter sensitive URLs from analytics
    const url = event.url || '';

    // Skip tracking for sensitive paths
    if (
      url.includes('/api/webhooks') || // Webhook endpoints
      url.includes('token=') || // Query params with tokens
      url.includes('key=') || // Query params with keys
      url.includes('secret=') // Query params with secrets
    ) {
      return null; // Don't track this event
    }

    return event;
  };

  return <Analytics beforeSend={beforeSend} />;
}
