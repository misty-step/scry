import { describe, expect, it } from 'vitest';
import type { Id } from './_generated/dataModel';
import { enforcePerUserLimit, getSecretDiagnostics } from './embeddings';

describe('embeddings helpers', () => {
  describe('enforcePerUserLimit', () => {
    const makeItem = (userId: string) => ({
      userId: userId as Id<'users'>,
      _id: `item-${Math.random()}` as Id<'concepts'>,
    });

    it('returns empty array when limit is 0', () => {
      const items = [makeItem('user1'), makeItem('user1')];
      expect(enforcePerUserLimit(items, 0)).toEqual([]);
    });

    it('returns empty array when limit is negative', () => {
      const items = [makeItem('user1')];
      expect(enforcePerUserLimit(items, -1)).toEqual([]);
    });

    it('returns all items when under limit per user', () => {
      const items = [makeItem('user1'), makeItem('user2')];
      const result = enforcePerUserLimit(items, 2);
      expect(result).toHaveLength(2);
    });

    it('limits items per user correctly', () => {
      const items = [
        makeItem('user1'),
        makeItem('user1'),
        makeItem('user1'),
        makeItem('user2'),
        makeItem('user2'),
      ];
      const result = enforcePerUserLimit(items, 2);

      // Should get 2 from user1 and 2 from user2
      expect(result).toHaveLength(4);

      const user1Count = result.filter((r) => r.userId === 'user1').length;
      const user2Count = result.filter((r) => r.userId === 'user2').length;
      expect(user1Count).toBe(2);
      expect(user2Count).toBe(2);
    });

    it('preserves order and takes first N items per user', () => {
      const item1 = { userId: 'user1' as Id<'users'>, order: 1 };
      const item2 = { userId: 'user1' as Id<'users'>, order: 2 };
      const item3 = { userId: 'user1' as Id<'users'>, order: 3 };

      const result = enforcePerUserLimit([item1, item2, item3], 2);

      expect(result).toHaveLength(2);
      expect(result[0]).toBe(item1);
      expect(result[1]).toBe(item2);
    });

    it('handles empty input array', () => {
      expect(enforcePerUserLimit([], 5)).toEqual([]);
    });

    it('handles single item correctly', () => {
      const items = [makeItem('user1')];
      expect(enforcePerUserLimit(items, 1)).toHaveLength(1);
    });

    it('handles interleaved users correctly', () => {
      const items = [
        makeItem('user1'),
        makeItem('user2'),
        makeItem('user1'),
        makeItem('user2'),
        makeItem('user1'),
      ];
      const result = enforcePerUserLimit(items, 1);

      expect(result).toHaveLength(2);
      expect(result[0].userId).toBe('user1');
      expect(result[1].userId).toBe('user2');
    });
  });

  describe('getSecretDiagnostics', () => {
    it('returns not present for undefined', () => {
      const result = getSecretDiagnostics(undefined);
      expect(result.present).toBe(false);
      expect(result.length).toBe(0);
      expect(result.fingerprint).toBeNull();
    });

    it('returns not present for empty string', () => {
      const result = getSecretDiagnostics('');
      expect(result.present).toBe(false);
      expect(result.length).toBe(0);
      expect(result.fingerprint).toBeNull();
    });

    it('returns present with correct length for valid string', () => {
      const result = getSecretDiagnostics('test-api-key');
      expect(result.present).toBe(true);
      expect(result.length).toBe(12);
      expect(result.fingerprint).not.toBeNull();
    });

    it('produces consistent fingerprint for same input', () => {
      const result1 = getSecretDiagnostics('my-secret-key');
      const result2 = getSecretDiagnostics('my-secret-key');
      expect(result1.fingerprint).toBe(result2.fingerprint);
    });

    it('produces different fingerprint for different inputs', () => {
      const result1 = getSecretDiagnostics('key-one');
      const result2 = getSecretDiagnostics('key-two');
      expect(result1.fingerprint).not.toBe(result2.fingerprint);
    });

    it('returns 8-character hex fingerprint', () => {
      const result = getSecretDiagnostics('some-api-key');
      expect(result.fingerprint).toMatch(/^[0-9a-f]{1,8}$/);
    });

    it('handles long strings correctly', () => {
      const longKey = 'x'.repeat(1000);
      const result = getSecretDiagnostics(longKey);
      expect(result.present).toBe(true);
      expect(result.length).toBe(1000);
      expect(result.fingerprint).not.toBeNull();
    });

    it('handles special characters correctly', () => {
      const result = getSecretDiagnostics('key!@#$%^&*()_+-=[]{}');
      expect(result.present).toBe(true);
      expect(result.fingerprint).not.toBeNull();
    });
  });
});
