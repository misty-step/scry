import { describe, expect, it } from 'vitest';
import { chunkArray } from './chunkArray';

describe('chunkArray', () => {
  it('splits array into chunks of specified size', () => {
    const result = chunkArray([1, 2, 3, 4, 5, 6], 2);
    expect(result).toEqual([
      [1, 2],
      [3, 4],
      [5, 6],
    ]);
  });

  it('handles last chunk being smaller than size', () => {
    const result = chunkArray([1, 2, 3, 4, 5], 2);
    expect(result).toEqual([[1, 2], [3, 4], [5]]);
  });

  it('handles chunk size equal to array length', () => {
    const result = chunkArray([1, 2, 3], 3);
    expect(result).toEqual([[1, 2, 3]]);
  });

  it('handles chunk size larger than array length', () => {
    const result = chunkArray([1, 2], 5);
    expect(result).toEqual([[1, 2]]);
  });

  it('returns empty array for empty input', () => {
    const result = chunkArray([], 3);
    expect(result).toEqual([]);
  });

  it('handles chunk size of 1', () => {
    const result = chunkArray([1, 2, 3], 1);
    expect(result).toEqual([[1], [2], [3]]);
  });

  it('throws error when size is zero', () => {
    expect(() => chunkArray([1, 2, 3], 0)).toThrow('chunk size must be greater than 0');
  });

  it('throws error when size is negative', () => {
    expect(() => chunkArray([1, 2, 3], -1)).toThrow('chunk size must be greater than 0');
  });

  it('works with string arrays', () => {
    const result = chunkArray(['a', 'b', 'c', 'd'], 2);
    expect(result).toEqual([
      ['a', 'b'],
      ['c', 'd'],
    ]);
  });

  it('works with object arrays', () => {
    const objs = [{ id: 1 }, { id: 2 }, { id: 3 }];
    const result = chunkArray(objs, 2);
    expect(result).toEqual([[{ id: 1 }, { id: 2 }], [{ id: 3 }]]);
  });
});
