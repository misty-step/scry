import { describe, expect, it } from 'vitest';
import { buildInteractionContext } from './interactionContext';

describe('buildInteractionContext', () => {
  describe('empty/minimal input', () => {
    it('returns undefined when all options are empty/null/undefined', () => {
      const result = buildInteractionContext({});

      expect(result).toBeUndefined();
    });

    it('returns undefined when options have null values', () => {
      const result = buildInteractionContext({
        sessionId: null,
        scheduledDays: null,
        nextReview: null,
        fsrsState: null,
      });

      expect(result).toBeUndefined();
    });
  });

  describe('sessionId', () => {
    it('includes sessionId when provided', () => {
      const result = buildInteractionContext({
        sessionId: 'session-123',
      });

      expect(result).toEqual({ sessionId: 'session-123' });
    });

    it('excludes sessionId when empty string', () => {
      const result = buildInteractionContext({
        sessionId: '',
      });

      expect(result).toBeUndefined();
    });
  });

  describe('isRetry', () => {
    it('includes isRetry when true', () => {
      const result = buildInteractionContext({
        isRetry: true,
      });

      expect(result).toEqual({ isRetry: true });
    });

    it('includes isRetry when false', () => {
      const result = buildInteractionContext({
        isRetry: false,
      });

      expect(result).toEqual({ isRetry: false });
    });

    it('excludes isRetry when undefined', () => {
      const result = buildInteractionContext({
        isRetry: undefined,
      });

      expect(result).toBeUndefined();
    });
  });

  describe('scheduledDays', () => {
    it('includes scheduledDays when provided as number', () => {
      const result = buildInteractionContext({
        scheduledDays: 7,
      });

      expect(result).toEqual({ scheduledDays: 7 });
    });

    it('includes scheduledDays when 0', () => {
      const result = buildInteractionContext({
        scheduledDays: 0,
      });

      expect(result).toEqual({ scheduledDays: 0 });
    });

    it('excludes scheduledDays when null', () => {
      const result = buildInteractionContext({
        scheduledDays: null,
      });

      expect(result).toBeUndefined();
    });
  });

  describe('nextReview', () => {
    it('includes nextReview when provided as number', () => {
      const timestamp = Date.now();
      const result = buildInteractionContext({
        nextReview: timestamp,
      });

      expect(result).toEqual({ nextReview: timestamp });
    });

    it('includes nextReview when 0', () => {
      const result = buildInteractionContext({
        nextReview: 0,
      });

      expect(result).toEqual({ nextReview: 0 });
    });
  });

  describe('fsrsState', () => {
    it('includes fsrsState when new', () => {
      const result = buildInteractionContext({
        fsrsState: 'new',
      });

      expect(result).toEqual({ fsrsState: 'new' });
    });

    it('includes fsrsState when learning', () => {
      const result = buildInteractionContext({
        fsrsState: 'learning',
      });

      expect(result).toEqual({ fsrsState: 'learning' });
    });

    it('includes fsrsState when review', () => {
      const result = buildInteractionContext({
        fsrsState: 'review',
      });

      expect(result).toEqual({ fsrsState: 'review' });
    });

    it('includes fsrsState when relearning', () => {
      const result = buildInteractionContext({
        fsrsState: 'relearning',
      });

      expect(result).toEqual({ fsrsState: 'relearning' });
    });

    it('excludes fsrsState when null', () => {
      const result = buildInteractionContext({
        fsrsState: null,
      });

      expect(result).toBeUndefined();
    });
  });

  describe('combined options', () => {
    it('includes all provided values in result', () => {
      const result = buildInteractionContext({
        sessionId: 'session-abc',
        isRetry: true,
        scheduledDays: 3,
        nextReview: 1700000000000,
        fsrsState: 'learning',
      });

      expect(result).toEqual({
        sessionId: 'session-abc',
        isRetry: true,
        scheduledDays: 3,
        nextReview: 1700000000000,
        fsrsState: 'learning',
      });
    });

    it('excludes null/undefined values while including valid ones', () => {
      const result = buildInteractionContext({
        sessionId: 'session-xyz',
        isRetry: null as unknown as boolean,
        scheduledDays: null,
        nextReview: 1700000000000,
        fsrsState: null,
      });

      expect(result).toEqual({
        sessionId: 'session-xyz',
        nextReview: 1700000000000,
      });
    });

    it('produces minimal payload for bandwidth optimization', () => {
      // Only sessionId - minimal valid context
      const minimal = buildInteractionContext({ sessionId: 's' });
      expect(Object.keys(minimal!)).toHaveLength(1);

      // Full context
      const full = buildInteractionContext({
        sessionId: 's',
        isRetry: false,
        scheduledDays: 1,
        nextReview: 1,
        fsrsState: 'new',
      });
      expect(Object.keys(full!)).toHaveLength(5);
    });
  });
});
