import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { requireUserFromClerk } from '@/convex/clerk';
import {
  deleteEmbeddingForQuestion,
  upsertEmbeddingForQuestion,
} from '@/convex/lib/embeddingHelpers';
import { updateStatsCounters } from '@/convex/lib/userStatsHelpers';
import {
  restoreQuestion,
  saveBatch,
  saveGeneratedQuestions,
  softDeleteQuestion,
  updateQuestion,
} from '@/convex/questionsCrud';

afterEach(() => {
  vi.clearAllMocks();
});

vi.mock('@/convex/clerk', () => ({
  requireUserFromClerk: vi.fn(),
}));

vi.mock('@/convex/lib/embeddingHelpers', () => ({
  deleteEmbeddingForQuestion: vi.fn(),
  upsertEmbeddingForQuestion: vi.fn(),
}));

vi.mock('@/convex/lib/userStatsHelpers', () => ({
  updateStatsCounters: vi.fn(),
}));

vi.mock('@/convex/lib/analytics', () => ({
  trackEvent: vi.fn(),
}));

vi.mock('@/convex/scheduling', () => ({
  getScheduler: () => ({
    initializeCard: () => ({ fsrsInit: true, nextReview: 123 }),
  }),
}));

const mockedRequireUser = vi.mocked(requireUserFromClerk);
const mockedDeleteEmbedding = vi.mocked(deleteEmbeddingForQuestion);
const mockedUpsertEmbedding = vi.mocked(upsertEmbeddingForQuestion);
const mockedUpdateStats = vi.mocked(updateStatsCounters);

describe('questionsCrud', () => {
  let db: ReturnType<typeof createMockDb>;
  let ctx: any;

  beforeEach(() => {
    vi.clearAllMocks();
    mockedRequireUser.mockResolvedValue({ _id: 'users_1' } as any);
    db = createMockDb();
    ctx = { db };
  });

  describe('updateQuestion', () => {
    it('rejects unauthorized access', async () => {
      db.data.set('q1', makeQuestion({ userId: 'users_2' }));
      await expect(
        (updateQuestion as any)._handler(ctx, { questionId: 'q1', question: 'New text' })
      ).rejects.toThrow('Question not found or unauthorized');
    });

    it('validates option lengths', async () => {
      db.data.set('q1', makeQuestion());
      await expect(
        (updateQuestion as any)._handler(ctx, { questionId: 'q1', options: ['only one'] })
      ).rejects.toThrow('At least 2 answer options are required');
    });

    it('clears embedding when text changes and preserves when only options change', async () => {
      db.data.set(
        'q1',
        makeQuestion({
          embedding: [0.1, 0.2],
          embeddingGeneratedAt: Date.now(),
        })
      );

      await (updateQuestion as any)._handler(ctx, {
        questionId: 'q1',
        question: 'Updated?',
      });

      expect(mockedDeleteEmbedding).toHaveBeenCalledWith(ctx, 'q1');
      expect(db.data.get('q1')?.embedding).toBeUndefined();

      // options-only change should not clear embedding
      mockedDeleteEmbedding.mockClear();
      await (updateQuestion as any)._handler(ctx, {
        questionId: 'q1',
        options: ['A', 'B', 'C'],
        correctAnswer: 'A',
      });
      expect(mockedDeleteEmbedding).not.toHaveBeenCalled();
    });

    it('ensures correctAnswer remains valid when options updated', async () => {
      db.data.set('q1', makeQuestion({ correctAnswer: 'A', options: ['A', 'B'] }));
      await expect(
        (updateQuestion as any)._handler(ctx, {
          questionId: 'q1',
          options: ['C', 'D'],
        })
      ).rejects.toThrow('Current correct answer must be included in new options');
    });
  });

  describe('softDeleteQuestion & restoreQuestion', () => {
    it('soft deletes then restores', async () => {
      db.data.set('q1', makeQuestion());

      const del = await (softDeleteQuestion as any)._handler(ctx, { questionId: 'q1' });
      expect(del.success).toBe(true);
      expect(db.data.get('q1')?.deletedAt).toBeDefined();

      const restored = await (restoreQuestion as any)._handler(ctx, { questionId: 'q1' });
      expect(restored.success).toBe(true);
      expect(db.data.get('q1')?.deletedAt).toBeUndefined();
    });

    it('prevents double delete', async () => {
      db.data.set('q1', makeQuestion({ deletedAt: Date.now() }));
      await expect((softDeleteQuestion as any)._handler(ctx, { questionId: 'q1' })).rejects.toThrow(
        'Question is already deleted'
      );
    });
  });

  describe('saveBatch & saveGeneratedQuestions', () => {
    it('inserts questions with fsrs init and updates stats', async () => {
      const result = await (saveBatch as any)._handler(ctx, {
        userId: 'users_1',
        questions: [
          { question: 'Q1', options: ['A', 'B'], correctAnswer: 'A', embedding: [0.1, 0.2] },
          { question: 'Q2', options: ['A', 'B'], correctAnswer: 'B' },
        ],
      });

      expect(result).toHaveLength(2);
      expect(db.data.size).toBe(2);
      expect(Array.from(db.data.values())[0].fsrsInit).toBe(true);
      expect(mockedUpsertEmbedding).toHaveBeenCalledTimes(1);
      expect(mockedUpdateStats).toHaveBeenCalledWith(
        ctx,
        'users_1',
        expect.objectContaining({ totalCards: 2, newCount: 2 })
      );
    });

    it('saveGeneratedQuestions uses current user and initializes FSRS', async () => {
      const res = await (saveGeneratedQuestions as any)._handler(ctx, {
        questions: [{ question: 'Q1', options: ['A', 'B'], correctAnswer: 'A' }],
      });
      expect(res.count).toBe(1);
      expect(db.data.get(res.questionIds[0])?.fsrsInit).toBe(true);
      expect(mockedUpdateStats).toHaveBeenCalledWith(
        ctx,
        'users_1',
        expect.objectContaining({ totalCards: 1 })
      );
    });
  });
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function createMockDb() {
  let counter = 0;
  const data = new Map<string, any>();
  return {
    data,
    async insert(_table: string, doc: Record<string, unknown>) {
      const id = `q_${++counter}`;
      data.set(id, { _id: id, ...doc });
      return id;
    },
    async patch(id: string, update: Record<string, unknown>) {
      const current = data.get(id);
      data.set(id, { ...current, ...update });
    },
    async get(id: string) {
      return data.get(id) ?? null;
    },
  };
}

function makeQuestion(overrides: Record<string, unknown> = {}) {
  return {
    _id: 'q1',
    userId: 'users_1',
    question: 'What is ATP?',
    options: ['A', 'B'],
    correctAnswer: 'A',
    explanation: 'Energy',
    deletedAt: undefined,
    embedding: undefined,
    embeddingGeneratedAt: undefined,
    ...overrides,
  };
}
