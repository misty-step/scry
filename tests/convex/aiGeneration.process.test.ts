import { generateObject } from 'ai';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { internal } from '@/convex/_generated/api';
import { generatePhrasingsForConcept, processJob } from '@/convex/aiGeneration';
import { makeConcept, makeGenerationJob } from '@/tests/helpers';

const initializeGoogleProviderMock = vi.fn();

vi.mock('@/convex/lib/aiProviders', () => ({
  initializeGoogleProvider: (...args: unknown[]) => initializeGoogleProviderMock(...args),
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
    AI_MODEL: process.env.AI_MODEL,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    process.env.AI_MODEL = 'gemini-3-pro-preview';
  });

  afterEach(() => {
    process.env.AI_MODEL = originalEnv.AI_MODEL;
  });

  it('marks job failed when provider initialization throws', async () => {
    initializeGoogleProviderMock.mockImplementationOnce(() => {
      throw new Error('Provider not configured');
    });

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
    initializeGoogleProviderMock.mockReturnValueOnce({
      model: {},
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
    initializeGoogleProviderMock.mockReturnValueOnce({
      model: {},
      diagnostics: { present: true, length: 10, fingerprint: 'abcd1234' },
    });

    const generateObjectMock = vi.mocked(generateObject);

    generateObjectMock
      .mockResolvedValueOnce({
        // Intent extraction response
        object: { content_type: 'conceptual' },
        usage: { totalTokens: 0, promptTokens: 0, completionTokens: 0 },
        rawResponse: {} as any,
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
        usage: { totalTokens: 0, promptTokens: 0, completionTokens: 0 },
        rawResponse: {} as any,
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
    initializeGoogleProviderMock.mockReturnValueOnce({
      model: {},
      diagnostics: { present: true, length: 10, fingerprint: 'abcd1234' },
    });

    const generateObjectMock = vi.mocked(generateObject);

    generateObjectMock
      .mockResolvedValueOnce({
        object: { content_type: 'conceptual' },
        usage: { totalTokens: 0, promptTokens: 0, completionTokens: 0 },
        rawResponse: {} as any,
      } as any)
      .mockResolvedValueOnce({
        object: { concepts: [] },
        usage: { totalTokens: 0, promptTokens: 0, completionTokens: 0 },
        rawResponse: {} as any,
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
      })
    );
  });
});

describe('generatePhrasingsForConcept', () => {
  const originalEnv = {
    AI_MODEL: process.env.AI_MODEL,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    process.env.AI_MODEL = 'gemini-3-pro-preview';
  });

  afterEach(() => {
    process.env.AI_MODEL = originalEnv.AI_MODEL;
  });

  it('generates phrasings and schedules next concept', async () => {
    initializeGoogleProviderMock.mockReturnValueOnce({
      model: {},
      diagnostics: { present: true, length: 10, fingerprint: 'abcd1234' },
    });

    const generateObjectMock = vi.mocked(generateObject);

    generateObjectMock.mockResolvedValueOnce({
      object: {
        phrasings: [
          {
            question: 'What is cellular respiration?',
            explanation:
              'Cellular respiration is a metabolic process that produces ATP from glucose.',
            type: 'multiple-choice',
            options: ['ATP production', 'Digestion', 'Circulation', 'Excretion'],
            correctAnswer: 'ATP production',
          },
        ],
      },
      usage: { totalTokens: 0, promptTokens: 0, completionTokens: 0 },
      rawResponse: {} as any,
    } as any);

    const concept = makeConcept({
      _id: 'concept_1' as any,
      title: 'Cellular respiration',
      description: 'How cells produce ATP',
    });

    const job = makeGenerationJob({
      _id: 'generationJobs_1' as any,
      status: 'processing',
      pendingConceptIds: ['concept_1' as any, 'concept_2' as any],
    });

    const advancePendingConceptCalls: any[] = [];

    const ctx = {
      runMutation: vi.fn(async (action, args) => {
        if (action === internal.generationJobs.advancePendingConcept) {
          advancePendingConceptCalls.push(args);
          return { pendingCount: 1, phrasingGenerated: 1, phrasingSaved: 1 };
        }

        if (action === internal.phrasings.insertGenerated) {
          return { ids: ['phrasing_1' as any] };
        }

        if (action === internal.concepts.applyPhrasingGenerationUpdate) {
          return;
        }
      }),
      runQuery: vi.fn(async (action) => {
        if (action === internal.concepts.getConceptById) {
          return concept;
        }

        if (action === internal.generationJobs.getJobByIdInternal) {
          return job;
        }

        if (action === internal.phrasings.getByConcept) {
          return [];
        }
      }),
      scheduler: { runAfter: vi.fn() },
    };

    await expect(
       
      (generatePhrasingsForConcept as any)._handler(ctx as any, {
        conceptId: 'concept_1' as any,
        jobId: job._id,
      })
    ).resolves.toBeUndefined();

    expect(advancePendingConceptCalls).toHaveLength(1);
    expect(advancePendingConceptCalls[0]).toEqual({
      jobId: job._id,
      conceptId: 'concept_1',
      phrasingGeneratedDelta: 1,
      phrasingSavedDelta: 1,
    });
  });
});
