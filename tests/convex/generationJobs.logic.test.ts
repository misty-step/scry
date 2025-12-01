import { beforeEach, describe, expect, it, vi } from 'vitest';
import { internal } from '@/convex/_generated/api';
import type { Id } from '@/convex/_generated/dataModel';
import { requireUserFromClerk } from '@/convex/clerk';
import {
  advancePendingConcept,
  cancelJob,
  cleanup,
  completeJob,
  createJob,
  failJob,
  getJobById,
  getJobByIdInternal,
  getRecentJobs,
  setConceptWork,
  updateProgress,
} from '@/convex/generationJobs';
import { enforceRateLimit } from '@/convex/rateLimit';
import { JOB_CONFIG } from '@/lib/constants/jobs';
import { createMockDb, makeGenerationJob } from '@/tests/helpers';

vi.mock('@/convex/clerk', () => ({
  requireUserFromClerk: vi.fn(),
}));

vi.mock('@/convex/rateLimit', () => ({
  enforceRateLimit: vi.fn(),
}));

vi.mock('@/convex/_generated/api', () => ({
  internal: {
    aiGeneration: {
      processJob: vi.fn(),
    },
  },
}));

const mockedRequireUser = vi.mocked(requireUserFromClerk);
const mockedEnforceRateLimit = vi.mocked(enforceRateLimit);

describe('generationJobs mutations', () => {
  let db: ReturnType<typeof createMockDb>;
  let ctx: any;

  beforeEach(() => {
    db = createMockDb();
    ctx = { db, scheduler: { runAfter: vi.fn() } };
    vi.clearAllMocks();
    mockedRequireUser.mockResolvedValue({ _id: 'user_1', email: 'a@example.com' } as any);
  });

  describe('createJob', () => {
    it('rejects prompts outside length bounds', async () => {
      await expect(
        (createJob as any)._handler(ctx, { prompt: 'ab', ipAddress: undefined })
      ).rejects.toThrow(
        `Prompt too short. Minimum ${JOB_CONFIG.MIN_PROMPT_LENGTH} characters required.`
      );

      const longPrompt = 'x'.repeat(JOB_CONFIG.MAX_PROMPT_LENGTH + 1);
      await expect(
        (createJob as any)._handler(ctx, { prompt: longPrompt, ipAddress: undefined })
      ).rejects.toThrow(
        `Prompt too long. Maximum ${JOB_CONFIG.MAX_PROMPT_LENGTH} characters allowed.`
      );
    });

    it('enforces concurrent processing limit', async () => {
      db.query.mockReturnValue({
        withIndex: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        collect: vi
          .fn()
          .mockResolvedValue(
            new Array(JOB_CONFIG.MAX_CONCURRENT_PER_USER).fill({ status: 'processing' })
          ),
      });

      await expect(
        (createJob as any)._handler(ctx, { prompt: 'valid prompt text', ipAddress: undefined })
      ).rejects.toThrow(
        `Too many concurrent jobs. Maximum ${JOB_CONFIG.MAX_CONCURRENT_PER_USER} jobs allowed.`
      );
    });

    it('creates job, applies rate limit, and schedules processing', async () => {
      db.query.mockReturnValue({
        withIndex: vi.fn().mockReturnThis(),
        eq: vi.fn().mockReturnThis(),
        collect: vi.fn().mockResolvedValue([]),
      });
      db.insert.mockResolvedValue('job_123');

      const result = await (createJob as any)._handler(ctx, {
        prompt: 'valid prompt text',
        ipAddress: '127.0.0.1',
      });

      expect(mockedEnforceRateLimit).toHaveBeenCalledWith(
        ctx,
        '127.0.0.1',
        'questionGeneration',
        false
      );
      expect(db.insert).toHaveBeenCalledWith(
        'generationJobs',
        expect.objectContaining({
          userId: 'user_1',
          status: 'pending',
          phase: 'clarifying',
        })
      );
      expect(ctx.scheduler.runAfter).toHaveBeenCalledWith(0, internal.aiGeneration.processJob, {
        jobId: 'job_123',
      });
      expect(result).toEqual({ jobId: 'job_123' });
    });
  });

  describe('cancelJob', () => {
    it('throws when user does not own job', async () => {
      db.get.mockResolvedValue({ userId: 'other_user', status: 'pending' });
      await expect((cancelJob as any)._handler(ctx, { jobId: 'job_1' })).rejects.toThrow(
        'Job not found or access denied'
      );
    });

    it('cancels pending job', async () => {
      db.get.mockResolvedValue({ _id: 'job_1', userId: 'user_1', status: 'pending' });
      db.patch.mockResolvedValue(undefined);
      const res = await (cancelJob as any)._handler(ctx, { jobId: 'job_1' });
      expect(res).toEqual({ success: true });
      expect(db.patch).toHaveBeenCalledWith(
        'job_1',
        expect.objectContaining({ status: 'cancelled' })
      );
    });
  });

  describe('updateProgress', () => {
    it('sets startedAt and status when transitioning from pending', async () => {
      db.get.mockResolvedValue(makeGenerationJob({ status: 'pending' }));
      db.patch.mockResolvedValue(undefined);
      await (updateProgress as any)._handler(ctx, {
        jobId: 'job_1',
        phase: 'generating',
        questionsGenerated: 2,
      });
      expect(db.patch).toHaveBeenCalledWith(
        'job_1',
        expect.objectContaining({
          status: 'processing',
          startedAt: expect.any(Number),
          phase: 'generating',
          questionsGenerated: 2,
        })
      );
    });

    it('patches only provided fields when job already processing', async () => {
      db.get.mockResolvedValue(makeGenerationJob({ status: 'processing', startedAt: 1 }));
      db.patch.mockResolvedValue(undefined);

      await (updateProgress as any)._handler(ctx, {
        jobId: 'job_proc',
        questionsGenerated: 5,
      });

      expect(db.patch).toHaveBeenCalledWith('job_proc', { questionsGenerated: 5 });
    });
  });

  describe('setConceptWork & advancePendingConcept', () => {
    it('sets pending concept ids', async () => {
      db.patch.mockResolvedValue(undefined);
      const conceptIds = ['c1', 'c2'] as Array<Id<'concepts'>>;
      await (setConceptWork as any)._handler(ctx, { jobId: 'job_1', conceptIds });
      expect(db.patch).toHaveBeenCalledWith('job_1', { conceptIds, pendingConceptIds: conceptIds });
    });

    it('advances pending concepts and flips phase when empty', async () => {
      db.get.mockResolvedValue(
        makeGenerationJob({
          pendingConceptIds: ['c1' as Id<'concepts'>],
          questionsGenerated: 1,
          questionsSaved: 1,
        })
      );
      db.patch.mockResolvedValue(undefined);
      const result = await (advancePendingConcept as any)._handler(ctx, {
        jobId: 'job_1',
        conceptId: 'c1' as Id<'concepts'>,
        questionsGeneratedDelta: 2,
        questionsSavedDelta: 1,
      });

      expect(result).toEqual({ pendingCount: 0, questionsGenerated: 3, questionsSaved: 2 });
      expect(db.patch).toHaveBeenCalledWith(
        'job_1',
        expect.objectContaining({
          pendingConceptIds: [],
          phase: 'finalizing',
          questionsGenerated: 3,
          questionsSaved: 2,
        })
      );
    });

    it('returns zero counts when job is missing', async () => {
      db.get.mockResolvedValue(null);

      const result = await (advancePendingConcept as any)._handler(ctx, {
        jobId: 'missing_job',
        conceptId: 'c1' as Id<'concepts'>,
        questionsGeneratedDelta: 1,
        questionsSavedDelta: 1,
      });

      expect(result).toEqual({ pendingCount: 0, questionsGenerated: 0, questionsSaved: 0 });
      expect(db.patch).not.toHaveBeenCalled();
    });
  });

  describe('completeJob & failJob', () => {
    it('marks job completed with counts and timing', async () => {
      db.patch.mockResolvedValue(undefined);
      await (completeJob as any)._handler(ctx, {
        jobId: 'job_1',
        topic: 'Biology',
        questionIds: ['q1', 'q2'],
        conceptIds: ['c1' as any],
        durationMs: 5000,
      });
      expect(db.patch).toHaveBeenCalledWith(
        'job_1',
        expect.objectContaining({
          status: 'completed',
          topic: 'Biology',
          questionsSaved: 2,
          durationMs: 5000,
          pendingConceptIds: [],
        })
      );
    });

    it('marks job failed with error metadata', async () => {
      db.patch.mockResolvedValue(undefined);
      await (failJob as any)._handler(ctx, {
        jobId: 'job_1',
        errorMessage: 'boom',
        errorCode: 'RATE_LIMIT',
        retryable: true,
      });
      expect(db.patch).toHaveBeenCalledWith(
        'job_1',
        expect.objectContaining({
          status: 'failed',
          errorMessage: 'boom',
          errorCode: 'RATE_LIMIT',
          retryable: true,
        })
      );
    });
  });

  describe('getRecentJobs query', () => {
    it('clamps pageSize to minimum 10 and uses null cursor by default', async () => {
      const paginate = vi.fn().mockResolvedValue({
        page: [makeGenerationJob({ _id: 'job_1' as any })],
        continueCursor: 'cursor_next',
        isDone: false,
      });
      const order = vi.fn().mockReturnValue({ paginate });
      const withIndex = vi.fn().mockReturnValue({ order });

      db.query.mockReturnValue({ withIndex });

      const result = await (getRecentJobs as any)._handler(ctx, {
        cursor: undefined,
        pageSize: 5,
      });

      expect(order).toHaveBeenCalledWith('desc');
      expect(paginate).toHaveBeenCalledWith({ numItems: 10, cursor: null });
      expect(result).toEqual({
        results: expect.arrayContaining([expect.objectContaining({ _id: expect.anything() })]),
        continueCursor: 'cursor_next',
        isDone: false,
      });
    });

    it('caps pageSize at 100 and forwards cursor', async () => {
      const paginate = vi.fn().mockResolvedValue({
        page: [],
        continueCursor: null,
        isDone: true,
      });
      const order = vi.fn().mockReturnValue({ paginate });
      const withIndex = vi.fn().mockReturnValue({ order });

      db.query.mockReturnValue({ withIndex });

      await (getRecentJobs as any)._handler(ctx, {
        cursor: 'cursor_1',
        pageSize: 500,
      });

      expect(paginate).toHaveBeenCalledWith({ numItems: 100, cursor: 'cursor_1' });
    });
  });

  describe('getJobById queries', () => {
    it('returns job when owned by authenticated user', async () => {
      const job = makeGenerationJob({ _id: 'job_1' as any, userId: 'user_1' as any });
      db.get.mockResolvedValue(job);

      const result = await (getJobById as any)._handler(ctx, { jobId: 'job_1' as any });

      expect(mockedRequireUser).toHaveBeenCalledTimes(1);
      expect(db.get).toHaveBeenCalledWith('job_1');
      expect(result).toBe(job);
    });

    it('returns null when job missing or owned by another user', async () => {
      db.get.mockResolvedValueOnce(null);

      const missing = await (getJobById as any)._handler(ctx, {
        jobId: 'missing_job' as any,
      });
      expect(missing).toBeNull();

      const foreignJob = makeGenerationJob({ _id: 'job_2' as any, userId: 'other_user' as any });
      db.get.mockResolvedValueOnce(foreignJob);

      const foreign = await (getJobById as any)._handler(ctx, {
        jobId: 'job_2' as any,
      });
      expect(foreign).toBeNull();
    });

    it('getJobByIdInternal bypasses auth and returns raw job', async () => {
      const job = makeGenerationJob({ _id: 'job_internal' as any });
      db.get.mockResolvedValue(job);

      const result = await (getJobByIdInternal as any)._handler(ctx, {
        jobId: 'job_internal' as any,
      });

      expect(mockedRequireUser).toHaveBeenCalledTimes(0);
      expect(db.get).toHaveBeenCalledWith('job_internal');
      expect(result).toBe(job);
    });
  });

  describe('cleanup internal mutation', () => {
    it('deletes completed and failed jobs returned by queries', async () => {
      const completedJobs = [{ _id: 'completed_1' as any }, { _id: 'completed_2' as any }];
      const failedJobs = [{ _id: 'failed_1' as any }];

      const completedCollect = vi.fn().mockResolvedValue(completedJobs);
      const failedCollect = vi.fn().mockResolvedValue(failedJobs);

      db.query
        .mockReturnValueOnce({
          withIndex: vi.fn().mockReturnValue({
            filter: vi.fn().mockReturnValue({ collect: completedCollect }),
          }),
        })
        .mockReturnValueOnce({
          withIndex: vi.fn().mockReturnValue({
            filter: vi.fn().mockReturnValue({ collect: failedCollect }),
          }),
        });

      db.delete.mockResolvedValue(undefined);

      const result = await (cleanup as any)._handler(ctx, {});

      expect(completedCollect).toHaveBeenCalledTimes(1);
      expect(failedCollect).toHaveBeenCalledTimes(1);
      expect(db.delete).toHaveBeenCalledTimes(3);
      expect(db.delete).toHaveBeenCalledWith('completed_1');
      expect(db.delete).toHaveBeenCalledWith('completed_2');
      expect(db.delete).toHaveBeenCalledWith('failed_1');
      expect(result).toEqual({ deletedCompleted: 2, deletedFailed: 1, total: 3 });
    });
  });
});
