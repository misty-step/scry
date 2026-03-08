import { describe, expect, it } from 'vitest';
import { calculateConceptStatsDelta } from './conceptFsrsHelpers';

describe('calculateConceptStatsDelta', () => {
  describe('state transitions', () => {
    it('returns null when no state change and no due status change', () => {
      const result = calculateConceptStatsDelta({
        oldState: 'learning',
        newState: 'learning',
        oldNextReview: 2000,
        newNextReview: 3000,
        nowMs: 1000, // Both are in future
      });

      expect(result).toBeNull();
    });

    it('returns state transition deltas for new → learning', () => {
      const result = calculateConceptStatsDelta({
        oldState: 'new',
        newState: 'learning',
        nowMs: 1000,
      });

      expect(result).not.toBeNull();
      expect(result?.newCount).toBe(-1);
      expect(result?.learningCount).toBe(1);
    });

    it('returns state transition deltas for learning → review', () => {
      const result = calculateConceptStatsDelta({
        oldState: 'learning',
        newState: 'review',
        nowMs: 1000,
      });

      expect(result).not.toBeNull();
      expect(result?.learningCount).toBe(-1);
      expect(result?.matureCount).toBe(1); // 'review' maps to matureCount
    });

    it('returns state transition deltas for review → relearning', () => {
      const result = calculateConceptStatsDelta({
        oldState: 'review',
        newState: 'relearning',
        nowMs: 1000,
      });

      expect(result).not.toBeNull();
      expect(result?.matureCount).toBe(-1); // 'review' maps to matureCount
      expect(result?.learningCount).toBe(1); // 'relearning' maps to learningCount
    });

    it('returns state transition deltas for relearning → review', () => {
      const result = calculateConceptStatsDelta({
        oldState: 'relearning',
        newState: 'review',
        nowMs: 1000,
      });

      expect(result).not.toBeNull();
      expect(result?.learningCount).toBe(-1); // 'relearning' maps to learningCount
      expect(result?.matureCount).toBe(1); // 'review' maps to matureCount
    });
  });

  describe('due status changes', () => {
    it('decrements dueNowCount when moving from due to not due', () => {
      const nowMs = 1000;
      const result = calculateConceptStatsDelta({
        oldState: 'review',
        newState: 'review', // Same state
        oldNextReview: 500, // Was due (500 <= 1000)
        newNextReview: 2000, // Now not due (2000 > 1000)
        nowMs,
      });

      expect(result).not.toBeNull();
      expect(result?.dueNowCount).toBe(-1);
    });

    it('increments dueNowCount when moving from not due to due', () => {
      const nowMs = 1000;
      const result = calculateConceptStatsDelta({
        oldState: 'review',
        newState: 'review',
        oldNextReview: 2000, // Was not due (2000 > 1000)
        newNextReview: 500, // Now due (500 <= 1000)
        nowMs,
      });

      expect(result).not.toBeNull();
      expect(result?.dueNowCount).toBe(1);
    });

    it('returns null when due status unchanged (both due)', () => {
      const nowMs = 1000;
      const result = calculateConceptStatsDelta({
        oldState: 'review',
        newState: 'review',
        oldNextReview: 500, // Due
        newNextReview: 800, // Still due
        nowMs,
      });

      expect(result).toBeNull();
    });

    it('returns null when due status unchanged (both not due)', () => {
      const nowMs = 1000;
      const result = calculateConceptStatsDelta({
        oldState: 'review',
        newState: 'review',
        oldNextReview: 2000, // Not due
        newNextReview: 3000, // Still not due
        nowMs,
      });

      expect(result).toBeNull();
    });

    it('handles edge case where nextReview equals nowMs (considered due)', () => {
      const nowMs = 1000;
      const result = calculateConceptStatsDelta({
        oldState: 'review',
        newState: 'review',
        oldNextReview: 2000, // Not due
        newNextReview: 1000, // Exactly now = due
        nowMs,
      });

      expect(result).not.toBeNull();
      expect(result?.dueNowCount).toBe(1);
    });
  });

  describe('combined state and due changes', () => {
    it('returns both state and due deltas when both change', () => {
      const nowMs = 1000;
      const result = calculateConceptStatsDelta({
        oldState: 'learning',
        newState: 'review',
        oldNextReview: 500, // Was due
        newNextReview: 2000, // Now not due
        nowMs,
      });

      expect(result).not.toBeNull();
      expect(result?.learningCount).toBe(-1);
      expect(result?.matureCount).toBe(1); // 'review' maps to matureCount
      expect(result?.dueNowCount).toBe(-1);
    });
  });

  describe('undefined newState', () => {
    it('handles undefined newState (no state transition)', () => {
      const result = calculateConceptStatsDelta({
        oldState: 'review',
        newState: undefined,
        oldNextReview: 500,
        newNextReview: 2000,
        nowMs: 1000,
      });

      // Should have both due change and state transition (review -> undefined)
      expect(result).not.toBeNull();
      expect(result?.dueNowCount).toBe(-1);
      expect(result?.matureCount).toBe(-1);
    });
  });

  describe('undefined nextReview values', () => {
    it('handles undefined oldNextReview', () => {
      const result = calculateConceptStatsDelta({
        oldState: 'new',
        newState: 'learning',
        oldNextReview: undefined,
        newNextReview: 2000,
        nowMs: 1000,
      });

      // State transition only, no due calculation
      expect(result).not.toBeNull();
      expect(result?.newCount).toBe(-1);
      expect(result?.learningCount).toBe(1);
      expect(result?.dueNowCount).toBeUndefined();
    });

    it('handles undefined newNextReview', () => {
      const result = calculateConceptStatsDelta({
        oldState: 'new',
        newState: 'learning',
        oldNextReview: 500,
        newNextReview: undefined,
        nowMs: 1000,
      });

      // State transition only, no due calculation
      expect(result).not.toBeNull();
      expect(result?.newCount).toBe(-1);
      expect(result?.learningCount).toBe(1);
      expect(result?.dueNowCount).toBeUndefined();
    });
  });
});
