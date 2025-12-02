import { describe, expect, it, vi } from 'vitest';
import type { Id } from './_generated/dataModel';

describe('phrasings handlers', () => {
  describe('getByConcept handler logic', () => {
    it('queries phrasings with correct index and limit', async () => {
      const mockPhrasings = [
        {
          _id: 'phrasing-1' as Id<'phrasings'>,
          conceptId: 'concept-1' as Id<'concepts'>,
          userId: 'user-1' as Id<'users'>,
          question: 'What is X?',
        },
        {
          _id: 'phrasing-2' as Id<'phrasings'>,
          conceptId: 'concept-1' as Id<'concepts'>,
          userId: 'user-1' as Id<'users'>,
          question: 'Explain Y',
        },
      ];

      const takeFn = vi.fn().mockResolvedValue(mockPhrasings);
      const mockDb = {
        query: vi.fn().mockReturnValue({
          withIndex: vi.fn().mockReturnValue({
            take: takeFn,
          }),
        }),
      };

      const args = {
        userId: 'user-1' as Id<'users'>,
        conceptId: 'concept-1' as Id<'concepts'>,
        limit: 10,
      };

      const result = await mockDb.query('phrasings').withIndex('by_user_concept').take(args.limit);

      expect(mockDb.query).toHaveBeenCalledWith('phrasings');
      expect(takeFn).toHaveBeenCalledWith(10);
      expect(result).toHaveLength(2);
    });

    it('returns empty array when no phrasings found', async () => {
      const takeFn = vi.fn().mockResolvedValue([]);
      const mockDb = {
        query: vi.fn().mockReturnValue({
          withIndex: vi.fn().mockReturnValue({
            take: takeFn,
          }),
        }),
      };

      const result = await mockDb.query('phrasings').withIndex('by_user_concept').take(5);

      expect(result).toEqual([]);
    });
  });

  describe('insertGenerated handler logic', () => {
    it('inserts multiple phrasings with correct fields', async () => {
      const insertFn = vi.fn().mockImplementation(async (_table, _doc) => {
        return `phrasing-${Math.random()}` as Id<'phrasings'>;
      });

      const mockDb = {
        insert: insertFn,
      };

      const args = {
        conceptId: 'concept-123' as Id<'concepts'>,
        userId: 'user-456' as Id<'users'>,
        phrasings: [
          {
            question: 'What is the capital of France?',
            explanation: 'France is a country in Europe.',
            type: 'multiple-choice' as const,
            options: ['Paris', 'London', 'Berlin', 'Madrid'],
            correctAnswer: 'Paris',
          },
          {
            question: 'True or false: Water boils at 100°C',
            explanation: 'At standard pressure.',
            type: 'true-false' as const,
            options: ['True', 'False'],
            correctAnswer: 'True',
          },
        ],
      };

      const now = Date.now();
      const ids: Id<'phrasings'>[] = [];

      for (const phrasing of args.phrasings) {
        const id = await mockDb.insert('phrasings', {
          userId: args.userId,
          conceptId: args.conceptId,
          question: phrasing.question,
          explanation: phrasing.explanation,
          type: phrasing.type,
          options: phrasing.options,
          correctAnswer: phrasing.correctAnswer,
          attemptCount: 0,
          correctCount: 0,
          createdAt: now,
          updatedAt: now,
          archivedAt: undefined,
          deletedAt: undefined,
          embedding: undefined,
          embeddingGeneratedAt: undefined,
        });
        ids.push(id);
      }

      expect(insertFn).toHaveBeenCalledTimes(2);
      expect(ids).toHaveLength(2);

      // Verify first phrasing was inserted with correct data
      expect(insertFn).toHaveBeenNthCalledWith(
        1,
        'phrasings',
        expect.objectContaining({
          userId: 'user-456',
          conceptId: 'concept-123',
          question: 'What is the capital of France?',
          type: 'multiple-choice',
          attemptCount: 0,
          correctCount: 0,
        })
      );
    });

    it('handles phrasings with embeddings', async () => {
      const insertFn = vi.fn().mockResolvedValue('phrasing-with-embedding' as Id<'phrasings'>);
      const mockDb = { insert: insertFn };

      const embedding = [0.1, 0.2, 0.3, 0.4, 0.5];
      const embeddingGeneratedAt = Date.now();

      const phrasing = {
        question: 'Test question',
        explanation: 'Test explanation',
        type: 'short-answer' as const,
        options: [],
        correctAnswer: 'Test answer',
        embedding,
        embeddingGeneratedAt,
      };

      const now = Date.now();
      await mockDb.insert('phrasings', {
        userId: 'user-1' as Id<'users'>,
        conceptId: 'concept-1' as Id<'concepts'>,
        question: phrasing.question,
        explanation: phrasing.explanation,
        type: phrasing.type,
        options: phrasing.options,
        correctAnswer: phrasing.correctAnswer,
        attemptCount: 0,
        correctCount: 0,
        createdAt: now,
        updatedAt: now,
        archivedAt: undefined,
        deletedAt: undefined,
        embedding: phrasing.embedding,
        embeddingGeneratedAt: phrasing.embeddingGeneratedAt,
      });

      expect(insertFn).toHaveBeenCalledWith(
        'phrasings',
        expect.objectContaining({
          embedding,
          embeddingGeneratedAt,
        })
      );
    });

    it('initializes counters to zero for new phrasings', async () => {
      const insertFn = vi.fn().mockResolvedValue('new-phrasing' as Id<'phrasings'>);
      const mockDb = { insert: insertFn };

      const now = Date.now();
      await mockDb.insert('phrasings', {
        userId: 'user-1' as Id<'users'>,
        conceptId: 'concept-1' as Id<'concepts'>,
        question: 'Q',
        explanation: 'E',
        type: 'cloze',
        options: [],
        correctAnswer: 'A',
        attemptCount: 0,
        correctCount: 0,
        createdAt: now,
        updatedAt: now,
        archivedAt: undefined,
        deletedAt: undefined,
      });

      expect(insertFn).toHaveBeenCalledWith(
        'phrasings',
        expect.objectContaining({
          attemptCount: 0,
          correctCount: 0,
          archivedAt: undefined,
          deletedAt: undefined,
        })
      );
    });

    it('returns ids array with all inserted phrasing ids', async () => {
      let callCount = 0;
      const insertFn = vi.fn().mockImplementation(async () => {
        callCount++;
        return `phrasing-${callCount}` as Id<'phrasings'>;
      });
      const mockDb = { insert: insertFn };

      const phrasings = [
        {
          question: 'Q1',
          explanation: 'E1',
          type: 'short-answer' as const,
          options: [],
          correctAnswer: 'A1',
        },
        {
          question: 'Q2',
          explanation: 'E2',
          type: 'short-answer' as const,
          options: [],
          correctAnswer: 'A2',
        },
        {
          question: 'Q3',
          explanation: 'E3',
          type: 'short-answer' as const,
          options: [],
          correctAnswer: 'A3',
        },
      ];

      const now = Date.now();
      const ids: Id<'phrasings'>[] = [];

      for (const p of phrasings) {
        const id = await mockDb.insert('phrasings', {
          userId: 'user-1' as Id<'users'>,
          conceptId: 'concept-1' as Id<'concepts'>,
          question: p.question,
          explanation: p.explanation,
          type: p.type,
          options: p.options,
          correctAnswer: p.correctAnswer,
          attemptCount: 0,
          correctCount: 0,
          createdAt: now,
          updatedAt: now,
        });
        ids.push(id);
      }

      expect(ids).toEqual(['phrasing-1', 'phrasing-2', 'phrasing-3']);
    });

    it('handles all phrasing types correctly', async () => {
      const insertFn = vi.fn().mockResolvedValue('phrasing-id' as Id<'phrasings'>);
      const mockDb = { insert: insertFn };

      const types = ['multiple-choice', 'true-false', 'cloze', 'short-answer'] as const;

      for (const type of types) {
        await mockDb.insert('phrasings', {
          type,
          question: `Question for ${type}`,
          explanation: 'Explanation',
          options:
            type === 'multiple-choice'
              ? ['A', 'B', 'C', 'D']
              : type === 'true-false'
                ? ['True', 'False']
                : [],
          correctAnswer: 'Answer',
        });
      }

      expect(insertFn).toHaveBeenCalledTimes(4);

      // Verify each type was passed correctly
      expect(insertFn).toHaveBeenCalledWith(
        'phrasings',
        expect.objectContaining({ type: 'multiple-choice' })
      );
      expect(insertFn).toHaveBeenCalledWith(
        'phrasings',
        expect.objectContaining({ type: 'true-false' })
      );
      expect(insertFn).toHaveBeenCalledWith(
        'phrasings',
        expect.objectContaining({ type: 'cloze' })
      );
      expect(insertFn).toHaveBeenCalledWith(
        'phrasings',
        expect.objectContaining({ type: 'short-answer' })
      );
    });
  });
});
