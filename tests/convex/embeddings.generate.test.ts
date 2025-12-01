import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { embed } from 'ai';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { generateEmbedding } from '@/convex/embeddings';

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
