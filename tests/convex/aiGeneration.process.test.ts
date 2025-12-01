import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { internal } from '@/convex/_generated/api';
import { generatePhrasingsForConcept, processJob } from '@/convex/aiGeneration';
import { generateObjectWithResponsesApi } from '@/convex/lib/responsesApi';
import { makeConcept, makeGenerationJob } from '@/tests/helpers';

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
      completeJob: Symbol('completeJob'),
    },
    aiGeneration: {
      generatePhrasingsForConcept: Symbol('generatePhrasingsForConcept'),
    },
    concepts: {
      createMany: Symbol('createMany'),
      getConceptById: Symbol('getConceptById'),
      applyPhrasingGenerationUpdate: Symbol('applyPhrasingGenerationUpdate'),
    },
    phrasings: {
      getByConcept: Symbol('getByConcept'),
      insertGenerated: Symbol('insertGenerated'),
    },
    embeddings: {
      generateEmbedding: Symbol('generateEmbedding'),
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

  it('advances job through stages and schedules phrasing generation on success', async () => {
    initializeProviderMock.mockResolvedValueOnce({
      provider: 'openai',
      model: 'gpt-5.1',
      openaiClient: {},
      diagnostics: { present: true, length: 10, fingerprint: 'abcd1234' },
    });

    const responsesApiMock = vi.mocked(generateObjectWithResponsesApi);

    responsesApiMock
      .mockResolvedValueOnce({
        // Intent extraction response
        object: { content_type: 'conceptual' },
        usage: { totalTokens: 0, inputTokens: 0, outputTokens: 0 },
        raw: {} as any,
      } as any)
      .mockResolvedValueOnce({
        // Concept synthesis response
        object: {
          concepts: [
            {
              title: 'Cellular respiration',
              description: 'How cells produce ATP',
              contentType: 'conceptual',
              whyItMatters: 'Core biology topic',
            },
          ],
        },
        usage: { totalTokens: 0, inputTokens: 0, outputTokens: 0 },
        raw: {} as any,
      } as any);

    const job = makeGenerationJob({
      _id: 'generationJobs_1' as any,
      status: 'processing',
      pendingConceptIds: [],
    });

    const updateProgressCalls: any[] = [];
    const schedulerRunAfter = vi.fn();

    const ctx = {
      runMutation: vi.fn(async (action, args) => {
        if (action === internal.generationJobs.updateProgress) {
          updateProgressCalls.push(args);
          return;
        }

        if (action === internal.concepts.createMany) {
          return { conceptIds: ['concept_1' as any] };
        }

        if (action === internal.generationJobs.setConceptWork) {
          return;
        }
      }),
      runQuery: vi.fn(async (action) => {
        if (action === internal.generationJobs.getJobByIdInternal) {
          return job;
        }
      }),
      scheduler: { runAfter: schedulerRunAfter },
    };

    // @ts-expect-error - access Convex internal handler for testing
    await expect(processJob._handler(ctx as any, { jobId: job._id })).resolves.toBeUndefined();

    expect(updateProgressCalls.map((call) => call.phase)).toEqual([
      'clarifying',
      'concept_synthesis',
      'phrasing_generation',
    ]);

    expect(schedulerRunAfter).toHaveBeenCalledWith(
      0,
      internal.aiGeneration.generatePhrasingsForConcept,
      {
        conceptId: 'concept_1',
        jobId: job._id,
      }
    );
  });

  it('maps schema validation failures to non-retryable user-friendly errors', async () => {
    initializeProviderMock.mockResolvedValueOnce({
      provider: 'openai',
      model: 'gpt-5.1',
      openaiClient: {},
      diagnostics: { present: true, length: 10, fingerprint: 'abcd1234' },
    });

    const responsesApiMock = vi.mocked(generateObjectWithResponsesApi);

    responsesApiMock
      .mockResolvedValueOnce({
        object: { content_type: 'conceptual' },
        usage: { totalTokens: 0, inputTokens: 0, outputTokens: 0 },
        raw: {} as any,
      } as any)
      .mockResolvedValueOnce({
        object: { concepts: [] },
        usage: { totalTokens: 0, inputTokens: 0, outputTokens: 0 },
        raw: {} as any,
      } as any);

    const job = makeGenerationJob({
      _id: 'generationJobs_2' as any,
      status: 'processing',
      pendingConceptIds: [],
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
          return job;
        }
      }),
      scheduler: { runAfter: vi.fn() },
    };

    const expectedMessage =
      'The AI proposed concepts that were too broad or redundant. Try giving a narrower prompt.';

    // @ts-expect-error - access Convex internal handler for testing
    await expect(processJob._handler(ctx as any, { jobId: job._id })).rejects.toThrow(
      expectedMessage
    );

    expect(failJob).toHaveBeenCalledWith(
      expect.objectContaining({
        jobId: job._id,
        errorCode: 'SCHEMA_VALIDATION',
        retryable: false,
        errorMessage: expectedMessage,
      })
    );
  });
});

describe('generatePhrasingsForConcept failure handling', () => {
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

  it('maps normalization failures to schema_validation errors and marks job retryable', async () => {
    initializeProviderMock.mockResolvedValueOnce({
      provider: 'openai',
      model: 'gpt-5.1',
      openaiClient: {},
      diagnostics: { present: true, length: 10, fingerprint: 'abcd1234' },
    });

    const responsesApiMock = vi.mocked(generateObjectWithResponsesApi);

    responsesApiMock.mockResolvedValueOnce({
      object: {
        phrasings: [
          {
            question: 'Short?',
            explanation: 'Too short',
            type: 'multiple-choice',
            options: ['A', 'B'],
            correctAnswer: 'A',
          },
        ],
      },
      usage: { totalTokens: 0, inputTokens: 0, outputTokens: 0 },
      raw: {} as any,
    } as any);

    const job = makeGenerationJob({
      _id: 'generationJobs_3' as any,
      status: 'processing',
      pendingConceptIds: [] as any,
    });

    const concept = makeConcept({
      _id: 'concepts_1' as any,
      userId: job.userId,
      phrasingCount: 0,
    });

    job.pendingConceptIds = [concept._id as any];

    const failJob = vi.fn();

    const ctx = {
      runMutation: vi.fn(async (action, args) => {
        if (action === internal.generationJobs.failJob) {
          failJob(args);
        }
      }),
      runQuery: vi.fn(async (action) => {
        if (action === internal.generationJobs.getJobByIdInternal) {
          return job;
        }

        if (action === internal.concepts.getConceptById) {
          return concept;
        }

        if (action === internal.phrasings.getByConcept) {
          return [];
        }
      }),
      runAction: vi.fn(async () => [0.1, 0.2]),
      scheduler: { runAfter: vi.fn() },
    };

    const expectedMessage =
      'The AI could not produce review-ready phrasings. Try rerunning in a few moments.';

    await expect(
      (generatePhrasingsForConcept as any)._handler(ctx as any, {
        conceptId: concept._id,
        jobId: job._id,
      })
    ).rejects.toThrow(expectedMessage);

    expect(failJob).toHaveBeenCalledWith(
      expect.objectContaining({
        jobId: job._id,
        errorCode: 'SCHEMA_VALIDATION',
        retryable: true,
        errorMessage: expectedMessage,
      })
    );
  });

  it('classifies upstream errors and surfaces rate limit friendly message', async () => {
    initializeProviderMock.mockResolvedValueOnce({
      provider: 'openai',
      model: 'gpt-5.1',
      openaiClient: {},
      diagnostics: { present: true, length: 10, fingerprint: 'abcd1234' },
    });

    const responsesApiMock = vi.mocked(generateObjectWithResponsesApi);

    responsesApiMock.mockRejectedValueOnce(new Error('Rate limit exceeded: HTTP 429'));

    const job = makeGenerationJob({
      _id: 'generationJobs_4' as any,
      status: 'processing',
      pendingConceptIds: [] as any,
    });

    const concept = makeConcept({
      _id: 'concepts_2' as any,
      userId: job.userId,
      phrasingCount: 0,
    });

    job.pendingConceptIds = [concept._id as any];

    const failJob = vi.fn();

    const ctx = {
      runMutation: vi.fn(async (action, args) => {
        if (action === internal.generationJobs.failJob) {
          failJob(args);
        }
      }),
      runQuery: vi.fn(async (action) => {
        if (action === internal.generationJobs.getJobByIdInternal) {
          return job;
        }

        if (action === internal.concepts.getConceptById) {
          return concept;
        }

        if (action === internal.phrasings.getByConcept) {
          return [];
        }
      }),
      runAction: vi.fn(async () => [0.1, 0.2]),
      scheduler: { runAfter: vi.fn() },
    };

    const expectedMessage = 'Rate limit reached. Please wait a moment and try again.';

    await expect(
      (generatePhrasingsForConcept as any)._handler(ctx as any, {
        conceptId: concept._id,
        jobId: job._id,
      })
    ).rejects.toThrow('Rate limit exceeded: HTTP 429');

    expect(failJob).toHaveBeenCalledWith(
      expect.objectContaining({
        jobId: job._id,
        errorCode: 'RATE_LIMIT',
        retryable: true,
        errorMessage: expectedMessage,
      })
    );
  });
});
