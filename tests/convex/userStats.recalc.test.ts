import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Doc, Id } from '@/convex/_generated/dataModel';
import { __test } from '@/convex/userStats';

afterEach(() => {
  vi.clearAllMocks();
});

const { recalculateUserStatsFromQuestions, applyQuestionsToAccumulator } = __test;

function createQuestionQuery(data: Array<Doc<'questions'>>) {
  return {
    withIndex: () => createQuestionQuery(data),
    filter: () => createQuestionQuery(data),
    order: () => createQuestionQuery(data),
    async paginate({ numItems, cursor }: { numItems: number; cursor: string | null }) {
      const start = cursor ? JSON.parse(cursor).index : 0;
      const page = data.slice(start, start + numItems);
      const nextIndex = start + page.length;
      return {
        page,
        continueCursor: nextIndex < data.length ? JSON.stringify({ index: nextIndex }) : null,
        isDone: nextIndex >= data.length,
      };
    },
  };
}

function makeCtx(questions: Array<Doc<'questions'>>) {
  return {
    db: {
      query: (table: string) => {
        if (table === 'questions') {
          return createQuestionQuery(questions);
        }
        throw new Error(`Unexpected table ${table}`);
      },
    },
  } as any;
}

describe('userStats recalc helpers', () => {
  it('recalculates counts and earliest nextReview across paginated batches', async () => {
    const userId = 'users_1' as Id<'users'>;
    const questions: Array<Doc<'questions'>> = [
      { _id: 'q1' as Id<'questions'>, userId, state: 'new', nextReview: undefined } as any,
      {
        _id: 'q2' as Id<'questions'>,
        userId,
        state: 'learning',
        nextReview: 50,
      } as any,
      { _id: 'q3' as Id<'questions'>, userId, state: 'relearning', nextReview: 30 } as any,
      { _id: 'q4' as Id<'questions'>, userId, state: 'review', nextReview: 80 } as any,
    ];

    const ctx = makeCtx(questions);
    const stats = await recalculateUserStatsFromQuestions(ctx, userId);

    expect(stats.totalCards).toBe(4);
    expect(stats.newCount).toBe(1);
    expect(stats.learningCount).toBe(2); // learning + relearning
    expect(stats.matureCount).toBe(1);
    expect(stats.nextReviewTime).toBe(30); // earliest nextReview
  });

  it('applyQuestionsToAccumulator aggregates state counts', () => {
    const questions: Array<Doc<'questions'>> = [
      { state: 'new' },
      { state: 'learning' },
      { state: 'relearning' },
      { state: 'review' },
    ] as any;

    const acc = {
      totalCards: 0,
      newCount: 0,
      learningCount: 0,
      matureCount: 0,
      nextReviewTime: undefined,
    };

    applyQuestionsToAccumulator(questions, acc);

    expect(acc.totalCards).toBe(4);
    expect(acc.newCount).toBe(1);
    expect(acc.learningCount).toBe(2);
    expect(acc.matureCount).toBe(1);
  });
});
