import { describe, expect, it, vi } from 'vitest';
import { shuffle } from './shuffle';

describe('shuffle', () => {
  it('returns an empty array for empty input', () => {
    const input: number[] = [];
    const result = shuffle(input);

    expect(result).toEqual([]);
    expect(result).not.toBe(input);
  });

  it('returns a copy for single-element arrays', () => {
    const input = [42];
    const result = shuffle(input);

    expect(result).toEqual([42]);
    expect(result).not.toBe(input);
  });

  it('does not mutate the original array', () => {
    const input = [1, 2, 3, 4];
    const original = [...input];

    const result = shuffle(input);

    expect(input).toEqual(original);
    expect(result).not.toBe(input);
  });

  it('preserves length and elements', () => {
    const input = ['a', 'b', 'c', 'd'];
    const result = shuffle(input);

    expect(result).toHaveLength(input.length);
    expect(result.sort()).toEqual([...input].sort());
  });

  it('uses crypto.getRandomValues when available', () => {
    const getRandomValuesSpy = vi.spyOn(globalThis.crypto, 'getRandomValues');
    const mathRandomSpy = vi.spyOn(Math, 'random');

    getRandomValuesSpy.mockImplementation((array) => {
      if (!array) return array;
      const uintArray = array as Uint32Array;
      uintArray[0] = 0x7fffffff; // ~0.5 when divided by 0xffffffff
      return uintArray;
    });

    shuffle([1, 2, 3]);

    expect(getRandomValuesSpy).toHaveBeenCalled();
    expect(mathRandomSpy).not.toHaveBeenCalled();

    getRandomValuesSpy.mockRestore();
    mathRandomSpy.mockRestore();
  });

  it('falls back to Math.random when crypto.getRandomValues is unavailable', () => {
    const originalGetRandomValues = globalThis.crypto?.getRandomValues;
    // @ts-expect-error mutate for test
    globalThis.crypto.getRandomValues = undefined;

    const mathRandomSpy = vi
      .spyOn(Math, 'random')
      .mockReturnValueOnce(0.5) // i = 3 → j = 2
      .mockReturnValueOnce(0.25) // i = 2 → j = 0
      .mockReturnValueOnce(0.75); // i = 1 → j = 1

    try {
      const result = shuffle([1, 2, 3, 4]);
      expect(result).toEqual([4, 2, 1, 3]);
    } finally {
      mathRandomSpy.mockRestore();
      if (originalGetRandomValues) {
        globalThis.crypto.getRandomValues = originalGetRandomValues;
      }
    }
  });
});
