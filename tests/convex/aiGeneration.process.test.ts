import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { internal } from '@/convex/_generated/api';
import { processJob } from '@/convex/aiGeneration';

const initializeProviderMock = vi.fn();

vi.mock('@/convex/lib/aiProviders', () => ({
  initializeProvider: (...args: unknown[]) => initializeProviderMock(...args),
}));

vi.mock('@/convex/lib/responsesApi', () => ({
  generateObjectWithResponsesApi: vi.fn(),
}));

vi.mock('ai', () => ({
  generateObject: vi.fn(),
}));

// Mock must be inline (vi.mock is hoisted, can't reference variables)
vi.mock('@/convex/_generated/api', () => ({
  internal: {
    generationJobs: {
      updateProgress: Symbol('updateProgress'),
      getJobByIdInternal: Symbol('getJobByIdInternal'),
      failJob: Symbol('failJob'),
      setConceptWork: Symbol('setConceptWork'),
      advancePendingConcept: Symbol('advancePendingConcept'),
    },
    aiGeneration: {
      generatePhrasingsForConcept: Symbol('generatePhrasingsForConcept'),
    },
    concepts: {
      createMany: Symbol('createMany'),
      getConceptById: Symbol('getConceptById'),
    },
  },
}));

describe('processJob failure handling', () => {
  const originalEnv = {
    AI_PROVIDER: process.env.AI_PROVIDER,
    AI_MODEL: process.env.AI_MODEL,
    AI_REASONING_EFFORT: process.env.AI_REASONING_EFFORT,
    AI_VERBOSITY: process.env.AI_VERBOSITY,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    process.env.AI_PROVIDER = 'openai';
    process.env.AI_MODEL = 'gpt-5.1';
    process.env.AI_REASONING_EFFORT = 'high';
    process.env.AI_VERBOSITY = 'medium';
  });

  afterEach(() => {
    process.env.AI_PROVIDER = originalEnv.AI_PROVIDER;
    process.env.AI_MODEL = originalEnv.AI_MODEL;
    process.env.AI_REASONING_EFFORT = originalEnv.AI_REASONING_EFFORT;
    process.env.AI_VERBOSITY = originalEnv.AI_VERBOSITY;
  });

  it('marks job failed when provider initialization throws', async () => {
    initializeProviderMock.mockRejectedValueOnce(new Error('Provider not configured'));

    const failJob = vi.fn();
    const ctx = {
      runMutation: vi.fn(async (action, args) => {
        if (action === internal.generationJobs.failJob) {
          failJob(args);
        }
      }),
      runQuery: vi.fn(),
      scheduler: { runAfter: vi.fn() },
    };

    // @ts-expect-error - access Convex internal handler for testing
    await expect(processJob._handler(ctx as any, { jobId: 'job_1' as any })).rejects.toThrow(
      'Provider not configured'
    );

    expect(failJob).toHaveBeenCalledWith(
      expect.objectContaining({
        jobId: 'job_1',
        errorCode: 'API_KEY',
        retryable: false,
      })
    );
  });

  it('fails gracefully when job is missing', async () => {
    initializeProviderMock.mockResolvedValueOnce({
      provider: 'openai',
      model: 'gpt-5.1',
      openaiClient: {},
      diagnostics: { present: true, length: 10, fingerprint: 'abcd1234' },
    });

    const failJob = vi.fn();
    const ctx = {
      runMutation: vi.fn(async (action, args) => {
        if (action === internal.generationJobs.failJob) {
          failJob(args);
        }
      }),
      runQuery: vi.fn(async (action) => {
        if (action === internal.generationJobs.getJobByIdInternal) {
          return null;
        }
      }),
      scheduler: { runAfter: vi.fn() },
    };

    // @ts-expect-error - access Convex internal handler for testing
    await expect(processJob._handler(ctx as any, { jobId: 'job_2' as any })).rejects.toThrow(
      'Job not found'
    );

    expect(failJob).toHaveBeenCalledWith(
      expect.objectContaining({
        jobId: 'job_2',
        errorCode: 'UNKNOWN',
        retryable: false,
        errorMessage: 'Job not found',
      })
    );
  });
});
