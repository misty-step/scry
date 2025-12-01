'use client';

import { useMemo } from 'react';
import { shuffle } from '@/lib/utils/shuffle';

/**
 * Shuffle answer options using true randomness.
 *
 * - Non-deterministic: order changes across renders/sessions.
 * - Memoized per options array identity to avoid reshuffling within a render pass.
 * - Uses crypto-strength randomness when available; falls back to Math.random.
 *
 * @param options Array of answer options to shuffle.
 * @returns Shuffled array of options (memoized on the input array).
 */
export function useShuffledOptions(options: string[]): string[] {
  return useMemo(() => shuffle(options), [options]);
}
