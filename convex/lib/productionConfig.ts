/**
 * Production Configuration Query
 *
 * Single source of truth for AI generation configuration.
 * Reads from Convex environment variables at runtime.
 *
 * ARCHITECTURE: This ensures Lab and production generation use
 * identical configs, preventing divergence and test/prod mismatches.
 */

import { query } from '../_generated/server';

/**
 * Get current production AI configuration
 *
 * Returns the actual runtime configuration used by production question generation.
 * Genesis Lab uses this to create its "PRODUCTION" config dynamically.
 */
export const getProductionConfig = query({
  handler: async () => {
    return {
      provider: 'openrouter' as const,
      model: process.env.AI_MODEL || 'google/gemini-3-flash-preview',
    };
  },
});
