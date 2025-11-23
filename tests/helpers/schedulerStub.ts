import { vi } from 'vitest';
import type { Doc } from '@/convex/_generated/dataModel';

type QuestionDoc = Doc<'questions'>;

export type ScheduleResult = {
  dbFields: Partial<QuestionDoc>;
  nextReviewDate?: Date;
  scheduledDays?: number;
  newState?: QuestionDoc['state'];
};

export type SchedulerStubOptions = {
  initializeState?: Partial<QuestionDoc>;
  scheduleResult?: ScheduleResult;
};

/**
 * Lightweight FSRS scheduler stub that mirrors the IScheduler shape.
 */
export function createSchedulerStub(options: SchedulerStubOptions = {}) {
  const { initializeState = {}, scheduleResult } = options;

  const resolvedSchedule: ScheduleResult = scheduleResult ?? {
    dbFields: {
      nextReview: Date.now() + 60_000,
      scheduledDays: 1,
      state: 'learning',
    },
    nextReviewDate: new Date(Date.now() + 60_000),
    scheduledDays: 1,
    newState: 'learning',
  };

  return {
    initializeCard: vi.fn(() => ({
      state: 'new',
      ...initializeState,
    })),
    scheduleNextReview: vi.fn((_question: QuestionDoc, _isCorrect: boolean) => ({
      dbFields: resolvedSchedule.dbFields ?? {},
      nextReviewDate:
        resolvedSchedule.nextReviewDate ??
        new Date((resolvedSchedule.dbFields?.nextReview as number | undefined) ?? Date.now()),
      scheduledDays:
        resolvedSchedule.scheduledDays ??
        (resolvedSchedule.dbFields?.scheduledDays as number | undefined) ??
        1,
      newState:
        resolvedSchedule.newState ?? (resolvedSchedule.dbFields?.state as QuestionDoc['state']),
    })),
    getRetrievability: vi.fn(() => 0.5),
  };
}
