import { describe, expect, it } from 'vitest';
import type { Id } from '../_generated/dataModel';
import { selectPhrasingForConcept, type PhrasingDoc } from './selectionPolicy';

const makePhrasing = (overrides: Partial<PhrasingDoc> = {}): PhrasingDoc => ({
  _id: `phrasing-${Math.random().toString(36).slice(2)}` as Id<'phrasings'>,
  _creationTime: Date.now(),
  userId: 'user-1' as Id<'users'>,
  conceptId: 'concept-1' as Id<'concepts'>,
  question: 'Test question',
  explanation: 'Test explanation',
  type: 'multiple-choice',
  options: ['A', 'B', 'C', 'D'],
  correctAnswer: 'A',
  attemptCount: 0,
  correctCount: 0,
  createdAt: Date.now(),
  updatedAt: Date.now(),
  archivedAt: undefined,
  deletedAt: undefined,
  embedding: undefined,
  embeddingGeneratedAt: undefined,
  lastAttemptedAt: undefined,
  ...overrides,
});

describe('selectPhrasingForConcept', () => {
  describe('empty input handling', () => {
    it('returns null phrasing with reason "none" when phrasings array is empty', () => {
      const result = selectPhrasingForConcept([]);
      expect(result.phrasing).toBeNull();
      expect(result.reason).toBe('none');
    });
  });

  describe('filtering logic', () => {
    it('filters out deleted phrasings', () => {
      const deleted = makePhrasing({ deletedAt: Date.now() });
      const active = makePhrasing({ deletedAt: undefined });

      const result = selectPhrasingForConcept([deleted, active]);

      expect(result.phrasing?._id).toBe(active._id);
    });

    it('filters out archived phrasings', () => {
      const archived = makePhrasing({ archivedAt: Date.now() });
      const active = makePhrasing({ archivedAt: undefined });

      const result = selectPhrasingForConcept([archived, active]);

      expect(result.phrasing?._id).toBe(active._id);
    });

    it('filters out excluded phrasing by ID', () => {
      const excluded = makePhrasing({ _id: 'excluded-phrasing' as Id<'phrasings'> });
      const active = makePhrasing({ _id: 'active-phrasing' as Id<'phrasings'> });

      const result = selectPhrasingForConcept([excluded, active], {
        excludePhrasingId: 'excluded-phrasing' as Id<'phrasings'>,
      });

      expect(result.phrasing?._id).toBe('active-phrasing');
    });

    it('returns none when all phrasings are filtered out', () => {
      const deleted = makePhrasing({ deletedAt: Date.now() });
      const archived = makePhrasing({ archivedAt: Date.now() });

      const result = selectPhrasingForConcept([deleted, archived]);

      expect(result.phrasing).toBeNull();
      expect(result.reason).toBe('none');
    });
  });

  describe('canonical phrasing selection', () => {
    it('returns canonical phrasing when specified and exists', () => {
      const canonical = makePhrasing({ _id: 'canonical-phrasing' as Id<'phrasings'> });
      const other = makePhrasing({ _id: 'other-phrasing' as Id<'phrasings'> });

      const result = selectPhrasingForConcept([other, canonical], {
        canonicalPhrasingId: 'canonical-phrasing' as Id<'phrasings'>,
      });

      expect(result.phrasing?._id).toBe('canonical-phrasing');
      expect(result.reason).toBe('canonical');
    });

    it('falls through to least-seen when canonical is deleted', () => {
      const canonical = makePhrasing({
        _id: 'canonical-phrasing' as Id<'phrasings'>,
        deletedAt: Date.now(),
      });
      const active = makePhrasing({
        _id: 'active-phrasing' as Id<'phrasings'>,
        attemptCount: 0,
      });

      const result = selectPhrasingForConcept([canonical, active], {
        canonicalPhrasingId: 'canonical-phrasing' as Id<'phrasings'>,
      });

      expect(result.phrasing?._id).toBe('active-phrasing');
      expect(result.reason).toBe('least-seen');
    });

    it('falls through when canonical ID does not exist', () => {
      const phrasing = makePhrasing({ attemptCount: 5 });

      const result = selectPhrasingForConcept([phrasing], {
        canonicalPhrasingId: 'nonexistent' as Id<'phrasings'>,
      });

      expect(result.phrasing?._id).toBe(phrasing._id);
      expect(result.reason).toBe('least-seen');
    });
  });

  describe('least-seen selection (default)', () => {
    it('selects phrasing with lowest attempt count', () => {
      const highAttempts = makePhrasing({ attemptCount: 10 });
      const lowAttempts = makePhrasing({ attemptCount: 2 });
      const medAttempts = makePhrasing({ attemptCount: 5 });

      const result = selectPhrasingForConcept([highAttempts, lowAttempts, medAttempts]);

      expect(result.phrasing?._id).toBe(lowAttempts._id);
      expect(result.reason).toBe('least-seen');
    });

    it('breaks attempt count tie with lastAttemptedAt (earlier first)', () => {
      const recentAttempt = makePhrasing({
        attemptCount: 3,
        lastAttemptedAt: Date.now(),
      });
      const olderAttempt = makePhrasing({
        attemptCount: 3,
        lastAttemptedAt: Date.now() - 86400000, // 1 day ago
      });

      const result = selectPhrasingForConcept([recentAttempt, olderAttempt]);

      expect(result.phrasing?._id).toBe(olderAttempt._id);
      expect(result.reason).toBe('least-seen');
    });

    it('breaks lastAttemptedAt tie with creation time (earlier first)', () => {
      const newer = makePhrasing({
        attemptCount: 3,
        lastAttemptedAt: 1000,
        _creationTime: Date.now(),
      });
      const older = makePhrasing({
        attemptCount: 3,
        lastAttemptedAt: 1000,
        _creationTime: Date.now() - 86400000,
      });

      const result = selectPhrasingForConcept([newer, older]);

      expect(result.phrasing?._id).toBe(older._id);
    });

    it('handles undefined attemptCount (treats as 0)', () => {
      const withCount = makePhrasing({ attemptCount: 1 });
      const withoutCount = makePhrasing({ attemptCount: undefined as any });

      const result = selectPhrasingForConcept([withCount, withoutCount]);

      // undefined ?? 0 = 0, which is less than 1
      expect(result.phrasing?._id).toBe(withoutCount._id);
    });

    it('handles undefined lastAttemptedAt (treats as 0)', () => {
      const withTime = makePhrasing({
        attemptCount: 1,
        lastAttemptedAt: 1000,
      });
      const withoutTime = makePhrasing({
        attemptCount: 1,
        lastAttemptedAt: undefined,
      });

      const result = selectPhrasingForConcept([withTime, withoutTime]);

      // undefined ?? 0 = 0, which is less than 1000
      expect(result.phrasing?._id).toBe(withoutTime._id);
    });
  });

  describe('random selection', () => {
    it('uses random selection when preferLeastSeen is false', () => {
      const phrasings = [
        makePhrasing({ _id: 'p1' as Id<'phrasings'>, attemptCount: 1 }),
        makePhrasing({ _id: 'p2' as Id<'phrasings'>, attemptCount: 2 }),
        makePhrasing({ _id: 'p3' as Id<'phrasings'>, attemptCount: 3 }),
      ];

      // Use deterministic random that returns 0.5 (selects middle item)
      const result = selectPhrasingForConcept(phrasings, {
        preferLeastSeen: false,
        random: () => 0.5,
      });

      expect(result.reason).toBe('random');
      // Math.floor(0.5 * 3) = 1, so p2
      expect(result.phrasing?._id).toBe('p2');
    });

    it('selects first item when random returns 0', () => {
      const phrasings = [
        makePhrasing({ _id: 'p1' as Id<'phrasings'> }),
        makePhrasing({ _id: 'p2' as Id<'phrasings'> }),
      ];

      const result = selectPhrasingForConcept(phrasings, {
        preferLeastSeen: false,
        random: () => 0,
      });

      expect(result.phrasing?._id).toBe('p1');
    });

    it('selects last item when random returns 0.999', () => {
      const phrasings = [
        makePhrasing({ _id: 'p1' as Id<'phrasings'> }),
        makePhrasing({ _id: 'p2' as Id<'phrasings'> }),
      ];

      const result = selectPhrasingForConcept(phrasings, {
        preferLeastSeen: false,
        random: () => 0.999,
      });

      // Math.floor(0.999 * 2) = 1
      expect(result.phrasing?._id).toBe('p2');
    });

    it('uses Math.random by default when random option not provided', () => {
      const phrasings = [makePhrasing()];

      const result = selectPhrasingForConcept(phrasings, {
        preferLeastSeen: false,
      });

      // Should not throw and should select the only available phrasing
      expect(result.phrasing).not.toBeNull();
      expect(result.reason).toBe('random');
    });
  });

  describe('combined options', () => {
    it('prioritizes canonical over least-seen', () => {
      const canonical = makePhrasing({
        _id: 'canonical' as Id<'phrasings'>,
        attemptCount: 100, // Higher attempts but canonical
      });
      const leastSeen = makePhrasing({
        _id: 'least-seen' as Id<'phrasings'>,
        attemptCount: 0,
      });

      const result = selectPhrasingForConcept([canonical, leastSeen], {
        canonicalPhrasingId: 'canonical' as Id<'phrasings'>,
      });

      expect(result.phrasing?._id).toBe('canonical');
      expect(result.reason).toBe('canonical');
    });

    it('handles exclusion of canonical phrasing', () => {
      const canonical = makePhrasing({ _id: 'canonical' as Id<'phrasings'> });
      const other = makePhrasing({ _id: 'other' as Id<'phrasings'> });

      const result = selectPhrasingForConcept([canonical, other], {
        canonicalPhrasingId: 'canonical' as Id<'phrasings'>,
        excludePhrasingId: 'canonical' as Id<'phrasings'>,
      });

      // Canonical is excluded, so should fall through
      expect(result.phrasing?._id).toBe('other');
      expect(result.reason).toBe('least-seen');
    });
  });
});
