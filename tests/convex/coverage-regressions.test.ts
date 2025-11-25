import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Id } from '@/convex/_generated/dataModel';
import { createMany } from '@/convex/concepts';
import { enforcePerUserLimit, generateEmbedding } from '@/convex/embeddings';
import { bulkDelete } from '@/convex/questionsBulk';
import { saveBatch } from '@/convex/questionsCrud';
import { createMockCtx, createMockDb, makeConcept } from '../helpers';

const getHandler = (fn: unknown) => (fn as any).handler ?? (fn as any)._handler;

vi.mock('@/convex/clerk', () => ({
  requireUserFromClerk: vi.fn().mockResolvedValue({ _id: 'users_1' }),
}));

const mockUpdateStatsCounters = vi.fn();
const mockTrackEvent = vi.fn();
const mockUpsertEmbeddingForQuestion = vi.fn();
const mockValidateBulkOwnership = vi.fn();

const mockConceptLogger = vi.hoisted(() => ({
  info: vi.fn(),
  warn: vi.fn(),
  error: vi.fn(),
}));

vi.mock('@/convex/lib/analytics', () => ({
  trackEvent: (...args: unknown[]) => mockTrackEvent(...args),
}));
vi.mock('@/convex/lib/userStatsHelpers', () => ({
  updateStatsCounters: (...args: unknown[]) => mockUpdateStatsCounters(...args),
}));
vi.mock('@/convex/lib/embeddingHelpers', () => ({
  upsertEmbeddingForQuestion: (...args: unknown[]) => mockUpsertEmbeddingForQuestion(...args),
  deleteEmbeddingForQuestion: vi.fn(),
}));
vi.mock('@/convex/lib/validation', () => ({
  validateBulkOwnership: (...args: unknown[]) => mockValidateBulkOwnership(...args),
}));

const schedulerInitialize = vi.fn(() => ({ state: 'new', nextReview: 123 }));
vi.mock('@/convex/scheduling', () => ({
  getScheduler: () => ({
    initializeCard: schedulerInitialize,
  }),
}));

vi.mock('@/convex/lib/logger', () => ({
  createConceptsLogger: () => mockConceptLogger,
  generateCorrelationId: vi.fn(),
  logConceptEvent: vi.fn(),
}));

const mockEmbed = vi.fn();
const mockCreateGoogleGenerativeAI = vi.fn((_options?: unknown) => ({
  textEmbedding: vi.fn().mockReturnValue('model'),
}));

vi.mock('@ai-sdk/google', () => ({
  createGoogleGenerativeAI: (options: unknown) => mockCreateGoogleGenerativeAI(options),
}));

vi.mock('ai', () => ({
  embed: (...args: unknown[]) => mockEmbed(...args),
}));

describe('convex regression coverage', () => {
  beforeEach(() => {
    vi.useFakeTimers().setSystemTime(new Date('2025-02-01T00:00:00Z'));
    mockUpdateStatsCounters.mockReset();
    mockTrackEvent.mockReset();
    mockUpsertEmbeddingForQuestion.mockReset();
    mockValidateBulkOwnership.mockReset();
    schedulerInitialize.mockClear();
    mockEmbed.mockReset();
    mockConceptLogger.info.mockReset();
    mockConceptLogger.warn.mockReset();
    mockConceptLogger.error.mockReset();
    mockCreateGoogleGenerativeAI.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('concepts.createMany', () => {
    it('returns empty array when no concepts provided', async () => {
      const db = createMockDb();
      const ctx = createMockCtx({ db });

      const handler = getHandler(createMany);
      const result = await handler(ctx as never, {
        userId: 'users_1' as never,
        jobId: undefined,
        concepts: [],
      });

      expect(result).toEqual({ conceptIds: [] });
      expect(db.insert).not.toHaveBeenCalled();
    });

    it('skips short/duplicate titles and trims input', async () => {
      const existing = [makeConcept({ title: 'Existing Title' })];
      const insertedIds = ['concepts_a', 'concepts_b'];
      const db = createMockDb(
        {
          insert: vi
            .fn()
            .mockResolvedValueOnce(insertedIds[0])
            .mockResolvedValueOnce(insertedIds[1]),
        },
        existing
      );

      const ctx = createMockCtx({ db });

      const handler = getHandler(createMany);
      const result = await handler(ctx as never, {
        userId: 'users_1' as never,
        jobId: 'generationJobs_1' as never,
        concepts: [
          { title: ' ex ', description: ' short ' }, // short -> skip
          { title: 'Existing Title', description: 'duplicate' }, // duplicate -> skip
          { title: 'New Concept  ', description: '  desc ' }, // trimmed + created
          { title: 'Another Concept', description: undefined }, // created
        ],
      });

      expect(db.insert).toHaveBeenCalledTimes(2);
      expect(db.insert).toHaveBeenNthCalledWith(
        1,
        'concepts',
        expect.objectContaining({
          title: 'New Concept',
          description: 'desc',
          userId: 'users_1',
          generationJobId: 'generationJobs_1',
          fsrs: expect.any(Object),
        })
      );
      expect(db.insert).toHaveBeenNthCalledWith(
        2,
        'concepts',
        expect.objectContaining({
          title: 'Another Concept',
        })
      );
      expect(result.conceptIds).toEqual(insertedIds);
    });
  });

  describe('questionsCrud.saveBatch', () => {
    it('persists embeddings only when provided and updates stats', async () => {
      const db = createMockDb({
        insert: vi.fn().mockResolvedValueOnce('questions_1').mockResolvedValueOnce('questions_2'),
      });
      const ctx = createMockCtx({ db });

      const handler = getHandler(saveBatch);
      const result = await handler(ctx as never, {
        userId: 'users_1' as never,
        questions: [
          {
            question: 'Q1',
            options: ['a'],
            correctAnswer: 'a',
            explanation: 'e1',
            embedding: [0.1, 0.2],
            embeddingGeneratedAt: 123,
          },
          {
            question: 'Q2',
            options: ['b'],
            correctAnswer: 'b',
            explanation: 'e2',
          },
        ],
      });

      expect(result).toEqual(['questions_1', 'questions_2']);
      expect(schedulerInitialize).toHaveBeenCalledTimes(1);
      expect(mockUpsertEmbeddingForQuestion).toHaveBeenCalledTimes(1);
      expect(mockUpsertEmbeddingForQuestion).toHaveBeenCalledWith(
        ctx,
        'questions_1',
        'users_1',
        [0.1, 0.2],
        123
      );
      expect(mockUpdateStatsCounters).toHaveBeenCalledWith(ctx, 'users_1', {
        totalCards: 2,
        newCount: 2,
      });
    });
  });

  describe('questionsBulk.bulkDelete', () => {
    it('applies stats deltas based on question states', async () => {
      const db = createMockDb({
        patch: vi.fn().mockResolvedValue(undefined),
      });
      const ctx = createMockCtx({ db });

      mockValidateBulkOwnership.mockResolvedValue([
        { _id: 'q1', state: 'new' },
        { _id: 'q2', state: 'learning' },
        { _id: 'q3', state: 'review' },
      ]);

      const handler = getHandler(bulkDelete);
      const result = await handler(ctx as never, {
        questionIds: ['q1', 'q2', 'q3'] as never,
      });

      expect(db.patch).toHaveBeenCalledTimes(3);
      expect(mockUpdateStatsCounters).toHaveBeenCalledWith(ctx, 'users_1', {
        totalCards: -3,
        newCount: -1,
        learningCount: -1,
        matureCount: -1,
      });
      expect(result).toEqual({ deleted: 3 });
    });
  });

  describe('embeddings.generateEmbedding', () => {
    const originalKey = process.env.GOOGLE_AI_API_KEY;

    afterEach(() => {
      process.env.GOOGLE_AI_API_KEY = originalKey;
    });

    it('throws and logs when API key missing', async () => {
      process.env.GOOGLE_AI_API_KEY = '';
      const handler = getHandler(generateEmbedding);
      await expect(handler({} as never, { text: 'hi' })).rejects.toThrow(
        'GOOGLE_AI_API_KEY not configured'
      );
      expect(mockConceptLogger.error).toHaveBeenCalledWith(
        expect.stringContaining('GOOGLE_AI_API_KEY not configured'),
        expect.objectContaining({ event: 'embeddings.generation.missing-key' })
      );
      expect(mockEmbed).not.toHaveBeenCalled();
    });

    it('generates embedding and logs success', async () => {
      process.env.GOOGLE_AI_API_KEY = 'key-123';
      mockEmbed.mockResolvedValue({ embedding: [1, 2, 3] });

      const handler = getHandler(generateEmbedding);
      const result = await handler({} as never, { text: 'hello' });

      expect(mockCreateGoogleGenerativeAI).toHaveBeenCalledWith({ apiKey: 'key-123' });
      expect(mockEmbed).toHaveBeenCalledWith({
        model: 'model',
        value: 'hello',
      });
      expect(mockConceptLogger.info).toHaveBeenCalledWith(
        expect.stringContaining('Successfully generated embedding'),
        expect.objectContaining({
          event: 'embeddings.generation.success',
          dimensions: 3,
          textLength: 5,
          model: 'text-embedding-004',
        })
      );
      expect(result).toEqual([1, 2, 3]);
    });
  });

  describe('embeddings.enforcePerUserLimit', () => {
    it('caps items per user id and ignores non-positive limits', () => {
      type Item = { userId: Id<'users'>; value: number };
      const items: Item[] = [
        { userId: 'users_1' as Id<'users'>, value: 1 },
        { userId: 'users_1' as Id<'users'>, value: 2 },
        { userId: 'users_2' as Id<'users'>, value: 3 },
      ];

      expect(enforcePerUserLimit(items, 0)).toEqual([]);
      expect(enforcePerUserLimit(items, 1).map((i) => i.value)).toEqual([1, 3]);
      expect(enforcePerUserLimit(items, 2).map((i) => i.value)).toEqual([1, 2, 3]);
    });
  });
});
