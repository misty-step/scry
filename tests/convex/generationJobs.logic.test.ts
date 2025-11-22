import { beforeEach, describe, expect, it, vi } from 'vitest';
import { internal } from '@/convex/_generated/api';
import type { Id } from '@/convex/_generated/dataModel';
import { requireUserFromClerk } from '@/convex/clerk';
import {
  advancePendingConcept,
  cancelJob,
  completeJob,
  createJob,
  failJob,
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
        (createJob as any)._handler(ctx, { prompt: 'short', ipAddress: undefined })
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
});
