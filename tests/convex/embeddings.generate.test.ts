import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { embed } from 'ai';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  generateEmbedding,
  getQuestionForEmbedding,
  saveConceptEmbedding,
  saveEmbedding,
  savePhrasingEmbedding,
} from '@/convex/embeddings';
import { createMockCtx, createMockDb } from '@/tests/helpers';

vi.mock('@ai-sdk/google', () => ({
  createGoogleGenerativeAI: vi.fn(),
}));

vi.mock('ai', () => ({
  embed: vi.fn(),
}));

vi.mock('@/convex/lib/logger', () => ({
  createConceptsLogger: () => ({
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  }),
  generateCorrelationId: vi.fn().mockReturnValue('corr'),
  logConceptEvent: vi.fn(),
}));

const mockedCreateGoogleGenerativeAI = vi.mocked(createGoogleGenerativeAI);
const mockedEmbed = vi.mocked(embed);

describe('generateEmbedding', () => {
  const originalKey = process.env.GOOGLE_AI_API_KEY;

  beforeEach(() => {
    vi.clearAllMocks();
    process.env.GOOGLE_AI_API_KEY = 'test-key';
    mockedCreateGoogleGenerativeAI.mockReturnValue({
      textEmbedding: (model: string) => model,
    } as any);
  });

  afterEach(() => {
    process.env.GOOGLE_AI_API_KEY = originalKey;
  });

  it('throws when API key missing and logs error', async () => {
    process.env.GOOGLE_AI_API_KEY = '';

    // @ts-expect-error - access Convex internal handler for testing
    await expect(generateEmbedding._handler({} as any, { text: 'hello' })).rejects.toThrow(
      'GOOGLE_AI_API_KEY not configured'
    );
    expect(mockedCreateGoogleGenerativeAI).not.toHaveBeenCalled();
  });

  it('returns embedding on success', async () => {
    mockedEmbed.mockResolvedValue({
      embedding: [0.1, 0.2, 0.3],
      value: 'hello world',
      usage: { tokens: 3, inputTokens: 3, outputTokens: 0 },
    } as Awaited<ReturnType<typeof embed>>);

    // @ts-expect-error - access Convex internal handler for testing
    const result = await generateEmbedding._handler({} as any, { text: 'hello world' });

    expect(result).toEqual([0.1, 0.2, 0.3]);
    expect(mockedCreateGoogleGenerativeAI).toHaveBeenCalledWith({ apiKey: 'test-key' });
    expect(mockedEmbed).toHaveBeenCalledWith({
      model: 'text-embedding-004',
      value: 'hello world',
    });
  });

  it('classifies rate-limit errors with name', async () => {
    mockedEmbed.mockRejectedValue(new Error('429 rate limit exceeded'));

    // @ts-expect-error - access Convex internal handler for testing
    await expect(generateEmbedding._handler({} as any, { text: 'hello' })).rejects.toMatchObject({
      name: 'rate-limit-error',
      errorType: 'rate-limit-error',
    });
  });
});

describe('embeddings persistence helpers', () => {
  it('saveEmbedding throws when question missing and does not call upsert helper', async () => {
    const db = createMockDb({
      get: vi.fn().mockResolvedValue(null),
    });

    const ctx = createMockCtx({ db });

    await expect(
      // @ts-expect-error - internal Convex handler
      saveEmbedding._handler(ctx as any, {
        questionId: 'questions_1' as any,
        embedding: [0.1, 0.2],
        embeddingGeneratedAt: Date.now(),
      })
    ).rejects.toThrow('Question not found');
  });

  it('saveEmbedding delegates to upsertEmbeddingForQuestion with userId and timestamp', async () => {
    const question = {
      _id: 'questions_1',
      userId: 'users_1',
    } as any;

    const db = createMockDb({
      get: vi.fn().mockResolvedValue(question),
    });
    const ctx = createMockCtx({ db });

    const embedding = Array(768).fill(0.1);
    const timestamp = 123456;

    await expect(
      (saveEmbedding as any)._handler(ctx, {
        questionId: question._id,
        embedding,
        embeddingGeneratedAt: timestamp,
      })
    ).resolves.toBeUndefined();
  });

  it('saveConceptEmbedding patches concept with embedding and timestamp', async () => {
    const patch = vi.fn();
    const db = createMockDb({ patch });
    const ctx = createMockCtx({ db });

    const embedding = [0.1, 0.2];
    const timestamp = 123;

    await (saveConceptEmbedding as any)._handler(ctx, {
      conceptId: 'concepts_1' as any,
      embedding,
      embeddingGeneratedAt: timestamp,
    });

    expect(patch).toHaveBeenCalledWith('concepts_1', {
      embedding,
      embeddingGeneratedAt: timestamp,
    });
  });

  it('savePhrasingEmbedding patches phrasing with embedding and timestamp', async () => {
    const patch = vi.fn();
    const db = createMockDb({ patch });
    const ctx = createMockCtx({ db });

    const embedding = [0.5];
    const timestamp = 999;

    await (savePhrasingEmbedding as any)._handler(ctx, {
      phrasingId: 'phrasings_1' as any,
      embedding,
      embeddingGeneratedAt: timestamp,
    });

    expect(patch).toHaveBeenCalledWith('phrasings_1', {
      embedding,
      embeddingGeneratedAt: timestamp,
    });
  });

  it('getQuestionForEmbedding returns question when present', async () => {
    const question = { _id: 'questions_1' } as any;
    const db = createMockDb({ get: vi.fn().mockResolvedValue(question) });
    const ctx = createMockCtx({ db });

    const result = await (getQuestionForEmbedding as any)._handler(ctx, {
      questionId: 'questions_1' as any,
    });

    expect(result).toBe(question);
  });
});
