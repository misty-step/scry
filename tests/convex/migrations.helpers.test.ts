import { describe, expect, it, vi } from 'vitest';
import type { Id } from '@/convex/_generated/dataModel';
import {
  backfillInteractionSessionId,
  backfillUserCreatedAt,
  removeTopicFromQuestions,
} from '@/convex/migrations';
import { createMockDb } from '@/tests/helpers';

const makeId = <T extends string>(table: T, suffix: number) => `${table}_${suffix}` as Id<any>;

describe('migrations helpers', () => {
  describe('backfillInteractionSessionId', () => {
    it('copies sessionId from context when missing', async () => {
      const interactions = [
        { _id: makeId('interactions', 1), context: { sessionId: 'sess-1' }, sessionId: undefined },
      ];
      const paginate = makePaginate(interactions);
      const db = createMockDb({
        query: vi.fn().mockReturnValue({
          filter: vi.fn().mockReturnThis(),
          order: vi.fn().mockReturnThis(),
          paginate,
        }),
        patch: vi.fn().mockResolvedValue(undefined),
      });

      // @ts-expect-error internal handler exposed for tests
      const res = await backfillInteractionSessionId._handler({ db } as any, {
        batchSize: 10,
        dryRun: false,
      });

      expect(res.status).toBe('completed');
      expect(db.patch).toHaveBeenCalledWith(interactions[0]._id, { sessionId: 'sess-1' });
    });

    it('counts missing context without patching', async () => {
      const interactions = [{ _id: makeId('interactions', 2), context: {}, sessionId: undefined }];
      const paginate = makePaginate(interactions);
      const db = createMockDb({
        query: vi.fn().mockReturnValue({
          filter: vi.fn().mockReturnThis(),
          order: vi.fn().mockReturnThis(),
          paginate,
        }),
        patch: vi.fn(),
      });

      // @ts-expect-error internal handler exposed for tests
      const res = await backfillInteractionSessionId._handler({ db } as any, {
        batchSize: 10,
        dryRun: false,
      });

      expect(res.stats.missingSessionContext).toBe(1);
      expect(db.patch).not.toHaveBeenCalled();
    });
  });

  describe('backfillUserCreatedAt', () => {
    it('skips users with createdAt and patches missing ones', async () => {
      const users = [
        { _id: makeId('users', 1), createdAt: 123, _creationTime: 111 },
        { _id: makeId('users', 2), _creationTime: 222 },
      ];
      const paginate = makePaginate(users);
      const db = createMockDb({
        query: vi.fn().mockReturnValue({ paginate }),
        patch: vi.fn().mockResolvedValue(undefined),
      });

      // @ts-expect-error internal handler exposed for tests
      const res = await backfillUserCreatedAt._handler({ db } as any, {
        batchSize: 10,
        dryRun: false,
      });

      expect(res.stats.alreadyHadCreatedAt).toBe(1);
      expect(res.stats.updated).toBe(1);
      expect(db.patch).toHaveBeenCalledWith(users[1]._id, { createdAt: users[1]._creationTime });
    });
  });

  describe('removeTopicFromQuestions', () => {
    it('removes topic field via replace and reports counts', async () => {
      const questions = [
        {
          _id: makeId('questions', 1),
          topic: 'bio',
          _creationTime: 1,
          question: 'Q1',
          userId: makeId('users', 1),
        },
        {
          _id: makeId('questions', 2),
          question: 'Q2',
          userId: makeId('users', 1),
          _creationTime: 2,
        },
      ];
      const paginate = makePaginate(questions);
      const db = createMockDb({
        query: vi.fn().mockReturnValue({ paginate }),
        replace: vi.fn().mockResolvedValue(undefined),
      });

      // @ts-expect-error internal handler exposed for tests
      const res = await removeTopicFromQuestions._handler({ db } as any, {
        batchSize: 10,
        dryRun: false,
      });

      expect(res.stats.updated).toBe(1);
      expect(res.stats.alreadyMigrated).toBe(1);
      expect(db.replace).toHaveBeenCalledWith(questions[0]._id, {
        question: 'Q1',
        userId: questions[0].userId,
      });
    });
  });
});

function makePaginate<T>(items: T[]) {
  return vi.fn().mockReturnValue({
    page: items,
    continueCursor: null,
    isDone: true,
  });
}
