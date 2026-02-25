import { describe, expect, it } from 'vitest';
import type { Id } from '@/convex/_generated/dataModel';
import {
  buildSubmitAnswerPayload,
  formatDueResult,
  gradeAnswer,
} from '@/convex/agents/reviewToolHelpers';

describe('reviewToolHelpers', () => {
  describe('gradeAnswer', () => {
    it('matches answers case-insensitively with trimmed whitespace', () => {
      expect(gradeAnswer('  Paris ', 'paris')).toBe(true);
      expect(gradeAnswer('PARIS', ' paris  ')).toBe(true);
      expect(gradeAnswer('Lyon', 'Paris')).toBe(false);
    });

    it('handles blank and partial-answer edge cases deterministically', () => {
      expect(gradeAnswer('', '')).toBe(true);
      expect(gradeAnswer('   ', '')).toBe(true);
      expect(gradeAnswer('Paris, France', 'Paris')).toBe(false);
      expect(gradeAnswer('Naïve', 'naive')).toBe(false);
    });
  });

  describe('formatDueResult', () => {
    it('maps due query output into tool payload shape', () => {
      const result = formatDueResult({
        concept: {
          _id: 'concepts_1' as Id<'concepts'>,
          title: 'Photosynthesis',
          description: 'Plant energy conversion',
          fsrs: {
            state: 'review',
            stability: 12.3,
            difficulty: 4.1,
            lapses: 2,
            reps: 8,
          },
        },
        phrasing: {
          _id: 'phrasings_1' as Id<'phrasings'>,
          question: 'What does chlorophyll absorb?',
          type: 'multiple-choice',
          options: ['Light', 'Water'],
        },
        retrievability: 0.77,
        interactions: [{ isCorrect: true }, { isCorrect: false }, { isCorrect: true }],
      });

      expect(result).toEqual({
        conceptId: 'concepts_1',
        conceptTitle: 'Photosynthesis',
        conceptDescription: 'Plant energy conversion',
        fsrsState: 'review',
        stability: 12.3,
        difficulty: 4.1,
        lapses: 2,
        reps: 8,
        retrievability: 0.77,
        phrasingId: 'phrasings_1',
        question: 'What does chlorophyll absorb?',
        type: 'multiple-choice',
        options: ['Light', 'Water'],
        recentAttempts: 3,
        recentCorrect: 2,
      });
    });

    it('returns null when no due result exists', () => {
      expect(formatDueResult(null)).toBeNull();
    });
  });

  describe('buildSubmitAnswerPayload', () => {
    it('builds feedback payload from mutation result and grading context', () => {
      const payload = buildSubmitAnswerPayload({
        result: {
          conceptId: 'concepts_1' as Id<'concepts'>,
          nextReview: 1735689600000,
          scheduledDays: 3,
          newState: 'learning',
          totalAttempts: 4,
          totalCorrect: 3,
          lapses: 1,
          reps: 4,
        },
        userAnswer: 'Light',
        correctAnswer: 'Light',
        isCorrect: true,
        explanation: 'Chlorophyll absorbs light energy.',
        conceptTitle: 'Photosynthesis',
        conceptDescription: 'Plant energy conversion',
      });

      expect(payload).toEqual({
        conceptId: 'concepts_1',
        isCorrect: true,
        userAnswer: 'Light',
        correctAnswer: 'Light',
        explanation: 'Chlorophyll absorbs light energy.',
        conceptTitle: 'Photosynthesis',
        conceptDescription: 'Plant energy conversion',
        nextReview: 1735689600000,
        scheduledDays: 3,
        newState: 'learning',
        totalAttempts: 4,
        totalCorrect: 3,
        lapses: 1,
        reps: 4,
      });
    });
  });
});
