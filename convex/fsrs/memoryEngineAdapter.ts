import type { ScheduleState } from 'memory-engine';
import { State } from 'ts-fsrs';
import type { Doc } from '../_generated/dataModel';

export type ConceptDoc = Doc<'concepts'>;
export type ConceptFsrsState = ConceptDoc['fsrs'];
export type ConceptState = NonNullable<ConceptFsrsState['state']>;

export function conceptFsrsStateToScheduleState(
  state?: ConceptFsrsState | null,
  _now?: Date
): ScheduleState | null {
  if (!state || state.nextReview === undefined) {
    return null;
  }

  return {
    due: state.nextReview,
    stability: state.stability ?? 0,
    difficulty: state.difficulty ?? 0,
    elapsed_days: state.elapsedDays ?? 0,
    scheduled_days: state.scheduledDays ?? 0,
    reps: state.reps ?? 0,
    lapses: state.lapses ?? 0,
    state: mapDbStateToFsrs(state.state ?? 'new'),
    last_review: state.lastReview ?? null,
  };
}

export function scheduleStateToConceptFsrsState(state: ScheduleState): ConceptFsrsState {
  return {
    stability: state.stability,
    difficulty: state.difficulty,
    lastReview: state.last_review ?? undefined,
    nextReview: state.due,
    elapsedDays: state.elapsed_days,
    retrievability: undefined,
    scheduledDays: state.scheduled_days,
    reps: state.reps,
    lapses: state.lapses,
    state: mapFsrsStateToDb(state.state),
  };
}

export function mapDbStateToFsrs(state: ConceptState): State {
  switch (state) {
    case 'new':
      return State.New;
    case 'learning':
      return State.Learning;
    case 'review':
      return State.Review;
    case 'relearning':
      return State.Relearning;
    default:
      return State.New;
  }
}

export function mapFsrsStateToDb(state: State): ConceptState {
  switch (state) {
    case State.New:
      return 'new';
    case State.Learning:
      return 'learning';
    case State.Review:
      return 'review';
    case State.Relearning:
      return 'relearning';
    default:
      return 'new';
  }
}
