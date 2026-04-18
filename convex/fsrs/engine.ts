import {
  Rating as MemoryEngineRating,
  next as scheduleNext,
  type ScheduleState,
} from 'memory-engine';
import { Card, createEmptyCard, FSRS, generatorParameters, State, type Grade } from 'ts-fsrs';
import {
  conceptFsrsStateToScheduleState,
  mapDbStateToFsrs,
  mapFsrsStateToDb,
  scheduleStateToConceptFsrsState,
  type ConceptFsrsState,
  type ConceptState,
} from './memoryEngineAdapter';

export type { ConceptDoc, ConceptFsrsState, ConceptState } from './memoryEngineAdapter';

const DEFAULT_PARAMS = generatorParameters({
  maximum_interval: 365,
  enable_fuzz: true,
  enable_short_term: true,
});

export interface ScheduleArgs {
  state?: ConceptFsrsState | null;
  isCorrect: boolean;
  now?: Date;
}

export interface ScheduleResult {
  state: ConceptFsrsState;
  card: Card;
  rating: Grade;
}

export class FsrsEngine {
  private readonly fsrs: FSRS;

  constructor() {
    this.fsrs = new FSRS(DEFAULT_PARAMS);
  }

  initializeState(now: Date = new Date()): ConceptFsrsState {
    const card = createEmptyCard(now);
    return this.cardToState(card);
  }

  schedule({ state, isCorrect, now = new Date() }: ScheduleArgs): ScheduleResult {
    const rating = this.ratingFromCorrectness(isCorrect);
    const updatedScheduleState = scheduleNext(
      conceptFsrsStateToScheduleState(state, now),
      rating,
      now.getTime()
    );
    const updatedCard = this.scheduleStateToCard(updatedScheduleState);
    const nextState = scheduleStateToConceptFsrsState(updatedScheduleState);

    nextState.retrievability = this.fsrs.get_retrievability(updatedCard, now, false) as number;

    return {
      state: nextState,
      card: updatedCard,
      rating,
    };
  }

  getRetrievability(state?: ConceptFsrsState | null, now: Date = new Date()): number {
    if (!state || state.nextReview === undefined) {
      return -1;
    }

    // New concepts (no reps yet) are always highest priority
    if (!state.state || state.state === 'new' || (state.reps ?? 0) === 0) {
      return -1;
    }

    const card = this.stateToCard(state, now);
    return this.fsrs.get_retrievability(card, now, false) as number;
  }

  isDue(state?: ConceptFsrsState | null, now: Date = new Date()): boolean {
    if (!state || state.nextReview === undefined) {
      return true;
    }

    return state.nextReview <= now.getTime();
  }

  private ratingFromCorrectness(isCorrect: boolean): Grade {
    return isCorrect ? MemoryEngineRating.Good : MemoryEngineRating.Again;
  }

  private stateToCard(state?: ConceptFsrsState | null, now: Date = new Date()): Card {
    const scheduleState = conceptFsrsStateToScheduleState(state, now);

    if (scheduleState === null) {
      return createEmptyCard(now);
    }

    return this.scheduleStateToCard(scheduleState);
  }

  private scheduleStateToCard(state: ScheduleState): Card {
    const { last_review, ...rest } = state;

    if (last_review === null) {
      return {
        ...rest,
        due: new Date(state.due),
        learning_steps: 0,
      };
    }

    return {
      ...rest,
      due: new Date(state.due),
      last_review: new Date(last_review),
      learning_steps: 0,
    };
  }

  private cardToState(card: Card): ConceptFsrsState {
    return scheduleStateToConceptFsrsState({
      ...card,
      due: card.due.getTime(),
      last_review: card.last_review?.getTime() ?? null,
    });
  }

  private mapDbStateToFsrs(state: ConceptState): State {
    return mapDbStateToFsrs(state);
  }

  private mapFsrsStateToDb(state: State): ConceptState {
    return mapFsrsStateToDb(state);
  }
}

export const defaultEngine = new FsrsEngine();
