import { next, Rating } from 'memory-engine';
import { describe, expect, it } from 'vitest';
import { FsrsEngine } from '../../convex/fsrs/engine';
import {
  conceptFsrsStateToScheduleState,
  scheduleStateToConceptFsrsState,
  type ConceptFsrsState,
} from '../../convex/fsrs/memoryEngineAdapter';

const fixedNow = new Date('2025-01-16T12:00:00Z');

const reviewState: ConceptFsrsState = {
  stability: 3.2,
  difficulty: 2.4,
  lastReview: fixedNow.getTime() - 86_400_000,
  nextReview: fixedNow.getTime() + 3_600_000,
  elapsedDays: 1,
  retrievability: undefined,
  scheduledDays: 2,
  reps: 4,
  lapses: 1,
  state: 'review',
};

describe('memory-engine canary adapter', () => {
  it('round-trips Scry concept state through memory-engine schedule state', () => {
    const scheduleState = conceptFsrsStateToScheduleState(reviewState, fixedNow);

    expect(scheduleState).not.toBeNull();
    expect(scheduleState).toMatchObject({
      due: reviewState.nextReview,
      last_review: reviewState.lastReview,
      elapsed_days: reviewState.elapsedDays,
      scheduled_days: reviewState.scheduledDays,
      reps: reviewState.reps,
      lapses: reviewState.lapses,
    });
    expect(scheduleStateToConceptFsrsState(scheduleState!)).toEqual(reviewState);
  });

  it('schedules via memory-engine and preserves Scry state shape', () => {
    const engine = new FsrsEngine();
    const expected = scheduleStateToConceptFsrsState(
      next(conceptFsrsStateToScheduleState(reviewState, fixedNow), Rating.Good, fixedNow.getTime())
    );
    const actual = engine.schedule({ state: reviewState, isCorrect: true, now: fixedNow }).state;

    expect({ ...actual, retrievability: undefined }).toEqual(expected);
  });
});
