/**
 * Query pattern tests - lightweight version of bandwidth regression tests.
 *
 * Tests the same behavioral invariant ("uses pagination, not .collect()")
 * with small fixture sizes. If pagination logic works for 20 items,
 * it works for 20,000 - the count is irrelevant to the invariant.
 *
 * For large-scale performance testing, see tests/perf/bandwidth-regressions.perf.ts
 */
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Doc, Id } from '@/convex/_generated/dataModel';
import { deleteUser } from '@/convex/clerk';
import { __quizStatsTest, getQuizInteractionStats } from '@/convex/questionsLibrary';
import { checkEmailRateLimit, recordRateLimitAttempt } from '@/convex/rateLimit';
import { reconcileUserStats } from '@/convex/userStats';

afterEach(() => {
  vi.clearAllMocks();
});

describe('Query patterns (pagination invariants)', () => {
  describe('userStats reconciliation', () => {
    it('uses pagination, not .collect()', async () => {
      // Small fixtures - enough to exercise pagination logic
      const users = generateUsers(5);
      const questions = generateQuestions({ userId: users[0]._id, count: 25 });
      const ctx = createUserStatsCtx(users, questions);

      // @ts-expect-error - Accessing private _handler for testing
      const result = await reconcileUserStats._handler(ctx as any, {
        sampleSize: 100,
        driftThreshold: 0,
      });

      expect(result.stats.usersChecked).toBeGreaterThan(0);
      expect(ctx.flags.collectCalled).toBe(false);
    });
  });

  describe('rate limit helpers', () => {
    it('caps read batches - pagination not .collect()', async () => {
      const entries = generateRateLimitEntries({
        identifier: 'user@example.com',
        count: 20,
        startTimestamp: Date.now() - 10_000,
      });
      const ctx = createRateLimitCtx(entries);

      const result = await checkEmailRateLimit(ctx as any, 'user@example.com', 'magicLink');
      // With 20 entries, should hit rate limit
      expect(result.allowed).toBe(false);
      expect(ctx.flags.collectCalled).toBe(false);
    });

    it('cleans history incrementally - pagination not .collect()', async () => {
      const entries = generateRateLimitEntries({
        identifier: 'ip-123',
        count: 15,
        startTimestamp: Date.now() - 3_600_000 * 5, // Old entries
      });
      const ctx = createRateLimitCtx(entries);

      await recordRateLimitAttempt(ctx as any, 'ip-123', 'default');
      expect(ctx.flags.collectCalled).toBe(false);
      // Old entries should be cleaned up
      expect(ctx.db.tables.rateLimits.length).toBe(1);
    });
  });

  describe('quiz interaction stats', () => {
    it('streams interactions - pagination not .collect()', async () => {
      const interactions = generateInteractions({
        userId: 'user_stream' as Id<'users'>,
        sessionId: 'session-stream',
        count: 30,
      });
      const ctx = createInteractionsCtx('user_stream', interactions);

      // @ts-expect-error - Accessing private _handler for testing
      const stats = await getQuizInteractionStats._handler(ctx as any, {
        sessionId: 'session-stream',
      });

      expect(stats.totalInteractions).toBe(30);
      expect(ctx.flags.collectCalled).toBe(false);
    });
  });

  describe('clerk deleteUser cleanup', () => {
    it('patches questions via batches - pagination not .collect()', async () => {
      const questions = generateQuestions({ userId: 'user_delete' as Id<'users'>, count: 25 });
      const ctx = createDeleteCtx(questions);

      // @ts-expect-error - Accessing private _handler for testing
      await deleteUser._handler(ctx as any, { clerkId: 'clerk_delete' });

      expect(ctx.db.patch).toHaveBeenCalledTimes(questions.length);
      expect(ctx.flags.collectCalled).toBe(false);
    });
  });
});

// ---------------------------------------------------------------------
// Lightweight fixture generators (small counts)

function generateUsers(count: number): Array<Doc<'users'>> {
  return Array.from({ length: count }, (_, i) => ({
    _id: `user_${i}` as Id<'users'>,
    _creationTime: Date.now() - i * 1000,
    email: `user${i}@example.com`,
    clerkId: `clerk_${i}`,
    createdAt: Date.now() - i * 1000,
  }));
}

function generateQuestions(opts: {
  userId: Id<'users'> | string;
  count: number;
}): Array<Doc<'questions'>> {
  return Array.from({ length: opts.count }, (_, i) => ({
    _id: `question_${i}` as Id<'questions'>,
    _creationTime: Date.now() - i * 1000,
    userId: opts.userId as Id<'users'>,
    question: `Question ${i}`,
    type: 'multiple-choice' as const,
    options: ['A', 'B', 'C', 'D'],
    correctAnswer: 'A',
    generatedAt: Date.now() - i * 1000,
    attemptCount: 0,
    correctCount: 0,
    state: 'new' as const,
    nextReview: Date.now(),
  }));
}

function generateRateLimitEntries(opts: {
  identifier: string;
  count: number;
  startTimestamp: number;
}): Array<Doc<'rateLimits'>> {
  return Array.from({ length: opts.count }, (_, i) => ({
    _id: `rateLimit_${i}` as Id<'rateLimits'>,
    _creationTime: opts.startTimestamp + i * 100,
    identifier: opts.identifier,
    operation: 'magicLink',
    timestamp: opts.startTimestamp + i * 100,
    metadata: {},
  }));
}

function generateInteractions(opts: {
  userId: Id<'users'>;
  sessionId: string;
  count: number;
}): Array<Doc<'interactions'>> {
  return Array.from({ length: opts.count }, (_, i) => ({
    _id: `interaction_${i}` as Id<'interactions'>,
    _creationTime: Date.now() - i * 1000,
    userId: opts.userId,
    questionId: `question_${i % 10}` as Id<'questions'>,
    userAnswer: 'A',
    isCorrect: i % 2 === 0,
    attemptedAt: Date.now() - i * 1000,
    sessionId: opts.sessionId,
  }));
}

// ---------------------------------------------------------------------
// Mock infrastructure (same as perf tests, just smaller data)

function createExprBuilder<T extends Record<string, unknown>>() {
  const predicates: Array<(doc: T) => boolean> = [];

  const builder = {
    field(fieldName: keyof T | string) {
      return fieldName;
    },
    eq(field: keyof T | string, value: unknown) {
      predicates.push((doc) => doc[field as keyof T] === value);
      return builder;
    },
    gt(field: keyof T | string, value: unknown) {
      predicates.push((doc) => (doc[field as keyof T] as number) > (value as number));
      return builder;
    },
    gte(field: keyof T | string, value: unknown) {
      predicates.push((doc) => (doc[field as keyof T] as number) >= (value as number));
      return builder;
    },
    lt(field: keyof T | string, value: unknown) {
      predicates.push((doc) => (doc[field as keyof T] as number) < (value as number));
      return builder;
    },
    lte(field: keyof T | string, value: unknown) {
      predicates.push((doc) => (doc[field as keyof T] as number) <= (value as number));
      return builder;
    },
    _getPredicate() {
      return (doc: T) => predicates.every((p) => p(doc));
    },
  };

  return builder;
}

class GenericQuery<T extends Record<string, unknown>> {
  private readonly tableName: string;
  private readonly rows: T[];
  private readonly flags: { collectCalled: boolean };
  private predicates: Array<(doc: T) => boolean> = [];
  private orderDirection: 'asc' | 'desc' | null = null;

  constructor(tableName: string, rows: T[], flags: { collectCalled: boolean }) {
    this.tableName = tableName;
    this.rows = rows;
    this.flags = flags;
  }

  withIndex(
    _name: string,
    builder?: (
      expr: ReturnType<typeof createExprBuilder<T>>
    ) => ReturnType<typeof createExprBuilder<T>>
  ) {
    if (builder) {
      const exprBuilder = createExprBuilder<T>();
      builder(exprBuilder);
      this.predicates.push(exprBuilder._getPredicate());
    }
    return this;
  }

  filter(
    builder: (
      expr: ReturnType<typeof createExprBuilder<T>>
    ) => ReturnType<typeof createExprBuilder<T>>
  ) {
    const exprBuilder = createExprBuilder<T>();
    builder(exprBuilder);
    this.predicates.push(exprBuilder._getPredicate());
    return this;
  }

  order(direction: 'asc' | 'desc') {
    this.orderDirection = direction;
    return this;
  }

  async first() {
    const data = this.buildResult();
    return data[0] ?? null;
  }

  async take(limit: number) {
    return this.buildResult().slice(0, limit);
  }

  async paginate({ numItems, cursor }: { numItems: number; cursor: string | null }) {
    const data = this.buildResult();
    const start = cursor ? parseInt(cursor, 10) : 0;
    const page = data.slice(start, start + numItems);
    const nextIndex = start + page.length;
    return {
      page,
      continueCursor: nextIndex < data.length ? String(nextIndex) : null,
      isDone: nextIndex >= data.length,
    };
  }

  collect() {
    this.flags.collectCalled = true;
    throw new Error('.collect() should not be used - use pagination');
  }

  private buildResult() {
    let result = [...this.rows];
    for (const predicate of this.predicates) {
      result = result.filter(predicate);
    }

    const key = this.orderDirection ? getSortKey(result[0]) : null;
    if (key) {
      result.sort((a, b) => ((a as any)[key] ?? 0) - ((b as any)[key] ?? 0));
      if (this.orderDirection === 'desc') {
        result.reverse();
      }
    }

    return result;
  }
}

function getSortKey(row: Record<string, unknown> | undefined) {
  if (!row) return null;
  if ('attemptedAt' in row) return 'attemptedAt';
  if ('generatedAt' in row) return 'generatedAt';
  if ('timestamp' in row) return 'timestamp';
  if ('createdAt' in row) return 'createdAt';
  if ('_creationTime' in row) return '_creationTime';
  return null;
}

// ---------------------------------------------------------------------
// Context factories

function createUserStatsCtx(users: Array<Doc<'users'>>, questions: Array<Doc<'questions'>>) {
  const flags = { collectCalled: false };

  const db = {
    query: (table: string) => {
      if (table === 'users') return new GenericQuery(table, users, flags);
      if (table === 'questions') return new GenericQuery(table, questions, flags);
      if (table === 'userStats') return new GenericQuery(table, [], flags);
      throw new Error(`Unknown table ${table}`);
    },
    insert: vi.fn(),
    patch: vi.fn(),
  };

  return { db, flags };
}

function createRateLimitCtx(entries: Array<Doc<'rateLimits'>>) {
  const flags = { collectCalled: false };
  const tables = { rateLimits: entries };

  const db = {
    tables,
    query: (table: string) => {
      if (table !== 'rateLimits') throw new Error(`Unknown table ${table}`);
      return new GenericQuery(table, tables.rateLimits, flags);
    },
    insert: (_table: string, doc: Doc<'rateLimits'>) => {
      tables.rateLimits.push(doc);
    },
    delete: (id: string) => {
      tables.rateLimits = tables.rateLimits.filter((row) => row._id !== id);
    },
  };

  return { db, flags };
}

function createInteractionsCtx(userId: string, interactions: Array<Doc<'interactions'>>) {
  const flags = { collectCalled: false };

  const mockUser = {
    _id: userId as Id<'users'>,
    _creationTime: Date.now(),
    email: 'test@example.com',
    clerkId: 'clerk_test_user',
    createdAt: Date.now(),
  };

  const db = {
    query: (table: string) => {
      if (table === 'interactions') return new GenericQuery(table, interactions, flags);
      if (table === 'users') return new GenericQuery(table, [mockUser], flags);
      throw new Error(`Unknown table ${table}`);
    },
  };

  const auth = {
    getUserIdentity: vi.fn().mockResolvedValue({
      subject: 'clerk_test_user',
      email: 'test@example.com',
      name: 'Test User',
    }),
  };

  return { db, auth, flags };
}

function createDeleteCtx(questions: Array<Doc<'questions'>>) {
  const flags = { collectCalled: false };
  const questionQuery = new GenericQuery('questions', questions, flags);

  const db = {
    patch: vi.fn().mockResolvedValue(undefined),
    query: (table: string) => {
      if (table === 'users') {
        return {
          withIndex: () => ({
            first: () => Promise.resolve({ _id: 'user_delete' }),
          }),
        };
      }
      if (table === 'questions') return questionQuery;
      throw new Error(`Unknown table ${table}`);
    },
  };

  return { db, flags };
}
