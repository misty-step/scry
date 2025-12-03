import { act, renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { hashString, useDataHash } from './use-data-hash';

describe('hashString', () => {
  it('returns consistent hash for same input', () => {
    const hash1 = hashString('hello world');
    const hash2 = hashString('hello world');
    expect(hash1).toBe(hash2);
  });

  it('returns different hash for different input', () => {
    const hash1 = hashString('hello');
    const hash2 = hashString('world');
    expect(hash1).not.toBe(hash2);
  });

  it('returns unsigned 32-bit integer', () => {
    const hash = hashString('test');
    expect(hash).toBeGreaterThanOrEqual(0);
    expect(hash).toBeLessThanOrEqual(0xffffffff);
  });

  it('handles empty string', () => {
    const hash = hashString('');
    expect(typeof hash).toBe('number');
    expect(hash).toBe(5381); // djb2 initial value
  });

  it('handles long strings', () => {
    const longString = 'a'.repeat(10000);
    const hash = hashString(longString);
    expect(typeof hash).toBe('number');
  });

  it('handles unicode characters', () => {
    const hash = hashString('你好世界');
    expect(typeof hash).toBe('number');
  });

  it('handles special characters', () => {
    const hash = hashString('!@#$%^&*()_+-=[]{}|;:,.<>?');
    expect(typeof hash).toBe('number');
  });

  it('is sensitive to character order', () => {
    const hash1 = hashString('ab');
    const hash2 = hashString('ba');
    expect(hash1).not.toBe(hash2);
  });

  it('is sensitive to single character differences', () => {
    const hash1 = hashString('test');
    const hash2 = hashString('tesu');
    expect(hash1).not.toBe(hash2);
  });
});

describe('useDataHash', () => {
  it('returns hasChanged=true on first render with data', () => {
    const { result } = renderHook(() => useDataHash({ foo: 'bar' }));
    expect(result.current.hasChanged).toBe(true);
    expect(result.current.currentHash).not.toBeNull();
  });

  it('returns hasChanged=false when data unchanged on rerender', () => {
    const data = { foo: 'bar' };
    const { result, rerender } = renderHook(({ data }) => useDataHash(data), {
      initialProps: { data },
    });

    // First render - changed
    expect(result.current.hasChanged).toBe(true);

    // Rerender with same data
    rerender({ data });

    // Should not change
    expect(result.current.hasChanged).toBe(false);
  });

  it('returns hasChanged=true when data changes', () => {
    const { result, rerender } = renderHook(({ data }) => useDataHash(data), {
      initialProps: { data: { value: 1 } },
    });

    expect(result.current.hasChanged).toBe(true);
    const firstHash = result.current.currentHash;

    // Rerender with different data
    rerender({ data: { value: 2 } });

    expect(result.current.hasChanged).toBe(true);
    expect(result.current.currentHash).not.toBe(firstHash);
  });

  it('handles null data', () => {
    const { result } = renderHook(() => useDataHash(null));

    expect(result.current.hasChanged).toBe(false);
    expect(result.current.currentHash).toBeNull();
  });

  it('handles undefined data', () => {
    const { result } = renderHook(() => useDataHash(undefined));

    expect(result.current.hasChanged).toBe(false);
    expect(result.current.currentHash).toBeNull();
  });

  it('detects change from data to null', () => {
    const { result, rerender } = renderHook(({ data }) => useDataHash(data), {
      initialProps: { data: { foo: 'bar' } as { foo: string } | null },
    });

    expect(result.current.hasChanged).toBe(true);

    rerender({ data: null });

    expect(result.current.hasChanged).toBe(true);
    expect(result.current.currentHash).toBeNull();
  });

  it('detects change from null to data', () => {
    const { result, rerender } = renderHook(({ data }) => useDataHash(data), {
      initialProps: { data: null as { foo: string } | null },
    });

    // Previous hash is null initially
    expect(result.current.hasChanged).toBe(false);

    rerender({ data: { foo: 'bar' } });

    expect(result.current.hasChanged).toBe(true);
    expect(result.current.currentHash).not.toBeNull();
  });

  it('provides update function that syncs hashes', () => {
    const { result, rerender } = renderHook(({ data }) => useDataHash(data), {
      initialProps: { data: { value: 1 } },
    });

    expect(typeof result.current.update).toBe('function');

    // First render sets up the initial state
    expect(result.current.hasChanged).toBe(true);

    // Rerender with same data - useEffect should have updated previousHash
    rerender({ data: { value: 1 } });

    // Now manually call update
    act(() => {
      result.current.update();
    });

    // The update function should sync previousHash to currentHash
    // Both should be non-null now
    expect(result.current.currentHash).not.toBeNull();
  });

  it('handles arrays', () => {
    const { result, rerender } = renderHook(({ data }) => useDataHash(data), {
      initialProps: { data: [1, 2, 3] },
    });

    expect(result.current.hasChanged).toBe(true);

    rerender({ data: [1, 2, 3] });
    expect(result.current.hasChanged).toBe(false);

    rerender({ data: [1, 2, 4] });
    expect(result.current.hasChanged).toBe(true);
  });

  it('handles nested objects', () => {
    const { result, rerender } = renderHook(({ data }) => useDataHash(data), {
      initialProps: { data: { nested: { deep: 'value' } } },
    });

    expect(result.current.hasChanged).toBe(true);

    rerender({ data: { nested: { deep: 'value' } } });
    expect(result.current.hasChanged).toBe(false);

    rerender({ data: { nested: { deep: 'changed' } } });
    expect(result.current.hasChanged).toBe(true);
  });

  it('treats different object references with same content as unchanged', () => {
    const obj1 = { foo: 'bar' };
    const obj2 = { foo: 'bar' };

    const { result, rerender } = renderHook(({ data }) => useDataHash(data), {
      initialProps: { data: obj1 },
    });

    expect(result.current.hasChanged).toBe(true);

    // Different reference, same content
    rerender({ data: obj2 });

    expect(result.current.hasChanged).toBe(false);
  });
});
