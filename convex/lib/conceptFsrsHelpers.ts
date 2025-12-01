import type { ConceptState } from '../fsrs';
import { calculateStateTransitionDelta, type StatDeltas } from './userStatsHelpers';

interface StatsDeltaArgs {
  oldState: ConceptState;
  newState?: ConceptState;
  oldNextReview?: number;
  newNextReview?: number;
  nowMs: number;
}

export function calculateConceptStatsDelta({
  oldState,
  newState,
  oldNextReview,
  newNextReview,
  nowMs,
}: StatsDeltaArgs): StatDeltas | null {
  const deltas = calculateStateTransitionDelta(oldState, newState) ?? {};
  const result: StatDeltas = { ...deltas };

  if (oldNextReview !== undefined && newNextReview !== undefined) {
    const wasDue = oldNextReview <= nowMs;
    const isDueNow = newNextReview <= nowMs;

    if (wasDue && !isDueNow) {
      result.dueNowCount = (result.dueNowCount ?? 0) - 1;
    } else if (!wasDue && isDueNow) {
      result.dueNowCount = (result.dueNowCount ?? 0) + 1;
    }
  }

  return Object.keys(result).length > 0 ? result : null;
}
