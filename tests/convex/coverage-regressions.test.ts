import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Id } from '@/convex/_generated/dataModel';
import { createMany } from '@/convex/concepts';
import { enforcePerUserLimit, generateEmbedding } from '@/convex/embeddings';
import { createMockCtx, createMockDb, makeConcept } from '../helpers';

const getHandler = (fn: unknown) => (fn as any).handler ?? (fn as any)._handler;

vi.mock('@/convex/clerk', () => ({
  requireUserFromClerk: vi.fn().mockResolvedValue({ _id: 'users_1' }),
}));

const mockUpdateStatsCounters = vi.fn();
const mockTrackEvent = vi.fn();
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
// Question embedding helpers and validation removed with questions table.
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

      expect(mockCreateGoogleGenerativeAI).toHaveBeenCalledWith(
        expect.objectContaining({ apiKey: 'key-123' })
      );
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
      expect(enforcePerUserLimit(items, 1).map((i: Item) => i.value)).toEqual([1, 3]);
      expect(enforcePerUserLimit(items, 2).map((i: Item) => i.value)).toEqual([1, 2, 3]);
    });
  });
});
