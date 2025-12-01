/**
 * Simple Fisher-Yates shuffle using crypto RNG when available, otherwise Math.random.
 * Intended for option shuffling; true randomness is acceptable (no determinism).
 */
export function shuffle<T>(array: T[]): T[] {
  if (array.length <= 1) return [...array];

  const result = [...array];
  const random = () => {
    if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
      const buf = new Uint32Array(1);
      crypto.getRandomValues(buf);
      return buf[0] / 0xffffffff;
    }
    return Math.random();
  };

  for (let i = result.length - 1; i > 0; i--) {
    const j = Math.floor(random() * (i + 1));
    [result[i], result[j]] = [result[j], result[i]];
  }

  return result;
}
