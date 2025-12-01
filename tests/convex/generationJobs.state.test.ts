import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Id } from '@/convex/_generated/dataModel';
import {
  advancePendingConcept,
  completeJob,
  failJob,
  setConceptWork,
  updateProgress,
} from '@/convex/generationJobs';
import { createMockDb, makeGenerationJob } from '@/tests/helpers';

afterEach(() => {
  vi.clearAllMocks();
});

vi.mock('@/convex/_generated/api', () => ({
  internal: {
    aiGeneration: {
      processJob: vi.fn(),
    },
  },
}));

describe('generationJobs state transitions', () => {
  let db: ReturnType<typeof createMockDb>;
  let ctx: any;

  beforeEach(() => {
    db = createMockDb();
    ctx = { db, scheduler: { runAfter: vi.fn() } };
    vi.clearAllMocks();
  });

  describe('updateProgress', () => {
    it('patches only the provided progress fields', async () => {
      db.get.mockResolvedValue(makeGenerationJob({ status: 'processing', startedAt: 1 }));
      db.patch.mockResolvedValue(undefined);

      await (updateProgress as any)._handler(ctx, {
        jobId: 'job_proc' as Id<'generationJobs'>,
        phase: 'generating',
        questionsGenerated: 1,
      });

      // updateProgress only patches provided fields, doesn't copy status/startedAt
      expect(db.patch).toHaveBeenCalledWith('job_proc', {
        phase: 'generating',
        questionsGenerated: 1,
      });
    });
  });

  describe('setConceptWork', () => {
    it('overwrites pendingConceptIds and preserves existing status', async () => {
      db.patch.mockResolvedValue(undefined);
      const conceptIds = ['c1', 'c2'] as Array<Id<'concepts'>>;
      await (setConceptWork as any)._handler(ctx, { jobId: 'job1', conceptIds });

      expect(db.patch).toHaveBeenCalledWith('job1', {
        conceptIds,
        pendingConceptIds: conceptIds,
      });
    });
  });

  describe('advancePendingConcept', () => {
    it('keeps processing phase while pending concepts remain', async () => {
      db.get.mockResolvedValue(
        makeGenerationJob({
          pendingConceptIds: ['c1' as Id<'concepts'>, 'c2' as Id<'concepts'>],
          questionsGenerated: 1,
          questionsSaved: 1,
        })
      );
      db.patch.mockResolvedValue(undefined);

      const result = await (advancePendingConcept as any)._handler(ctx, {
        jobId: 'job_1',
        conceptId: 'c1' as Id<'concepts'>,
        questionsGeneratedDelta: 1,
        questionsSavedDelta: 1,
      });

      expect(result.pendingCount).toBe(1);
      expect(db.patch).toHaveBeenCalledWith(
        'job_1',
        expect.objectContaining({
          pendingConceptIds: ['c2'],
          phase: 'phrasing_generation',
        })
      );
    });
  });

  describe('completeJob', () => {
    it('sets completed status and clears pending', async () => {
      db.patch.mockResolvedValue(undefined);
      await (completeJob as any)._handler(ctx, {
        jobId: 'job_done',
        topic: 'History',
        questionIds: ['q1', 'q2'],
        conceptIds: ['c1' as any],
        durationMs: 2000,
      });

      expect(db.patch).toHaveBeenCalledWith(
        'job_done',
        expect.objectContaining({
          status: 'completed',
          pendingConceptIds: [],
          questionsSaved: 2,
          durationMs: 2000,
        })
      );
    });
  });

  describe('failJob', () => {
    it('sets failed status and retains retryable flag', async () => {
      db.patch.mockResolvedValue(undefined);
      await (failJob as any)._handler(ctx, {
        jobId: 'job_fail',
        errorMessage: 'boom',
        errorCode: 'SCHEMA_VALIDATION',
        retryable: true,
      });

      expect(db.patch).toHaveBeenCalledWith(
        'job_fail',
        expect.objectContaining({
          status: 'failed',
          errorMessage: 'boom',
          errorCode: 'SCHEMA_VALIDATION',
          retryable: true,
        })
      );
    });
  });
});
