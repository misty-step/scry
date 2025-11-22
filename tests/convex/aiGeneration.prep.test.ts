import { describe, expect, it } from 'vitest';
import {
  __test,
  prepareConceptIdeas,
  prepareGeneratedPhrasings,
  type ConceptPreparationStats,
} from '@/convex/aiGeneration';

describe('prepareConceptIdeas', () => {
  const baseIdea = { title: 'ATP', description: 'Energy currency', whyItMatters: 'Life' };

  it('filters empty titles/descriptions and removes duplicates (case-insensitive)', () => {
    const input = [
      { ...baseIdea, title: '   ' },
      { ...baseIdea, description: '   ' },
      baseIdea,
      { ...baseIdea, title: 'atp' }, // duplicate
    ];

    const result = prepareConceptIdeas(input);
    expect(result.concepts).toEqual([{ title: 'ATP', description: 'Energy currency' }]);
    expect(result.stats).toMatchObject({
      totalIdeas: 4,
      skippedEmptyTitle: 1,
      skippedEmptyDescription: 1,
      skippedDuplicate: 1,
      accepted: 1,
      fallbackUsed: false,
    } satisfies ConceptPreparationStats);
  });

  it('uses fallback prompt when all concepts filtered', () => {
    const input = [{ title: '   ', description: '', whyItMatters: '' }];
    const result = prepareConceptIdeas(input, 'Photosynthesis');

    expect(result.concepts).toEqual([
      {
        title: 'Photosynthesis',
        description: 'Deepening understanding of "Photosynthesis".',
      },
    ]);
    expect(result.stats.fallbackUsed).toBe(true);
  });
});

describe('prepareGeneratedPhrasings', () => {
  const base = {
    question: 'What is ATP used for in cells?',
    explanation: 'It stores and transfers energy.',
    type: 'multiple-choice' as const,
    options: ['Energy transfer', 'Structure', 'Signaling'],
    correctAnswer: 'Energy transfer',
  };

  it('drops invalid lengths, duplicates, and bad option counts', () => {
    const generated = [
      { ...base, question: 'Too short?', explanation: base.explanation },
      { ...base, explanation: 'short' }, // too short explanation
      base,
      { ...base, question: 'What is ATP used for in cells?   ' }, // duplicate after trim
      { ...base, options: ['A', 'B'], correctAnswer: 'A' }, // only 2 options MCQ -> reject
    ];
    const result = prepareGeneratedPhrasings(generated as any, [], 3);

    expect(result).toHaveLength(1);
    expect(result[0].question).toBe(base.question);
  });

  it('enforces correct answer presence and unique options', () => {
    const generated = [
      {
        ...base,
        options: ['Energy transfer', 'Energy transfer', 'Structure'],
        correctAnswer: 'Energy transfer',
      },
      {
        ...base,
        question: 'True/false variant',
        type: 'true-false' as const,
        options: ['True', 'False', 'Extra'], // invalid count
        correctAnswer: 'True',
      },
      {
        ...base,
        question: 'No correct option',
        correctAnswer: 'Nonexistent',
      },
    ];

    const result = prepareGeneratedPhrasings(generated as any, [], 5);
    expect(result).toHaveLength(1);
    expect(result[0].options).toEqual(['Energy transfer', 'Structure']);
    expect(result[0].correctAnswer).toBe('Energy transfer');
  });

  it('respects targetCount and skips existing questions', () => {
    const generated = Array.from({ length: 5 }, (_, i) => ({
      ...base,
      question: `Question ${i}`,
    }));

    const result = prepareGeneratedPhrasings(generated as any, ['question 0', 'question 1'], 2);
    expect(result).toHaveLength(2);
    expect(result.map((p) => p.question)).toEqual(['Question 2', 'Question 3']);
  });
});

describe('calculateConflictScore', () => {
  it('returns undefined when no duplicates', () => {
    expect(__test.calculateConflictScore(['a', 'b', 'c'])).toBeUndefined();
  });

  it('counts duplicate collisions', () => {
    expect(__test.calculateConflictScore(['a', 'a', 'A', 'b'])).toBe(2);
  });
});

describe('classifyError', () => {
  it('maps schema errors to SCHEMA_VALIDATION retryable', () => {
    const err = new Error('Schema does not match validator');
    expect(__test.classifyError(err)).toEqual({ code: 'SCHEMA_VALIDATION', retryable: true });
  });

  it('maps rate limit to retryable', () => {
    const err = new Error('429 rate limit exceeded');
    expect(__test.classifyError(err)).toEqual({ code: 'RATE_LIMIT', retryable: true });
  });

  it('maps API key to non-retryable', () => {
    const err = new Error('API key missing');
    expect(__test.classifyError(err)).toEqual({ code: 'API_KEY', retryable: false });
  });

  it('maps network to retryable', () => {
    const err = new Error('Network timeout');
    expect(__test.classifyError(err)).toEqual({ code: 'NETWORK', retryable: true });
  });

  it('defaults to UNKNOWN non-retryable', () => {
    const err = new Error('Unexpected failure');
    expect(__test.classifyError(err)).toEqual({ code: 'UNKNOWN', retryable: false });
  });
});
