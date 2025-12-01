import { Card, State } from 'ts-fsrs';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ConceptFsrsState, ConceptState, FsrsEngine } from '../../convex/fsrs/engine';

afterEach(() => {
  vi.clearAllMocks();
});

const fixedNow = new Date('2025-01-16T12:00:00Z');

// Access private helpers via runtime shape for deeper coverage
const getInternals = (engine: FsrsEngine) =>
  engine as unknown as {
    mapDbStateToFsrs: (state: ConceptState) => State;
    mapFsrsStateToDb: (state: State) => ConceptState;
    cardToState: (card: Card) => ConceptFsrsState;
    stateToCard: (state?: ConceptFsrsState | null, now?: Date) => Card;
  };

describe('FsrsEngine.schedule', () => {
  it('uses correctness to choose rating and advances review when correct', () => {
    const engine = new FsrsEngine();
    const initial = engine.initializeState(fixedNow);

    const correct = engine.schedule({ state: initial, isCorrect: true, now: fixedNow });
    const incorrect = engine.schedule({ state: initial, isCorrect: false, now: fixedNow });

    expect(correct.rating).toBeDefined();
    expect(incorrect.rating).toBeDefined();
    expect(correct.rating).not.toEqual(incorrect.rating);
    expect(correct.state.nextReview).toBeGreaterThan(fixedNow.getTime());
    expect(correct.state.reps).toBeGreaterThanOrEqual(1);
    expect(incorrect.state.nextReview).toBeLessThanOrEqual(correct.state.nextReview);
  });

  it('round-trips card ↔ state without losing scheduling attributes', () => {
    const engine = new FsrsEngine();
    const internals = getInternals(engine);

    const card: Card = {
      due: new Date(fixedNow.getTime() + 3_600_000),
      stability: 3.2,
      difficulty: 2.4,
      elapsed_days: 1,
      scheduled_days: 2,
      reps: 4,
      lapses: 1,
      state: State.Review,
      last_review: new Date(fixedNow.getTime() - 86_400_000),
      learning_steps: 0,
    };

    // Call through internals object to preserve 'this' binding
    const state = internals.cardToState.call(engine, card);
    const roundTripped = internals.stateToCard.call(engine, state, fixedNow);

    expect(roundTripped.due.getTime()).toBe(card.due.getTime());
    expect(roundTripped.state).toBe(State.Review);
    expect(roundTripped.stability).toBeCloseTo(card.stability);
    expect(roundTripped.difficulty).toBeCloseTo(card.difficulty);
    expect(roundTripped.reps).toBe(card.reps);
    expect(roundTripped.lapses).toBe(card.lapses);
  });
});

describe('FsrsEngine.getRetrievability', () => {
  it('returns -1 for unset or new states', () => {
    const engine = new FsrsEngine();
    const emptyState = engine.initializeState(fixedNow);

    expect(engine.getRetrievability(undefined, fixedNow)).toBe(-1);
    expect(engine.getRetrievability(null, fixedNow)).toBe(-1);
    // @ts-expect-error - intentionally testing missing nextReview field
    expect(engine.getRetrievability({ ...emptyState, nextReview: undefined }, fixedNow)).toBe(-1);
    expect(engine.getRetrievability(emptyState, fixedNow)).toBe(-1);
  });

  it('returns computed score for matured cards', () => {
    const engine = new FsrsEngine();
    let state = engine.initializeState(fixedNow);

    // Promote to review state with several correct answers
    for (let i = 0; i < 5; i++) {
      const result = engine.schedule({
        state,
        isCorrect: true,
        now: new Date(fixedNow.getTime() + i * 60_000),
      });
      state = result.state;
    }

    const score = engine.getRetrievability(state, fixedNow);
    expect(score).toBeGreaterThanOrEqual(0);
    expect(score).toBeLessThanOrEqual(1);
  });
});

describe('FsrsEngine.isDue', () => {
  it('treats missing state as due and respects nextReview timestamps', () => {
    const engine = new FsrsEngine();
    const base = engine.initializeState(fixedNow);

    expect(engine.isDue()).toBe(true);
    expect(engine.isDue({ ...base, nextReview: fixedNow.getTime() + 60_000 }, fixedNow)).toBe(
      false
    );
    expect(engine.isDue({ ...base, nextReview: fixedNow.getTime() - 60_000 }, fixedNow)).toBe(true);
  });
});

describe('FsrsEngine state mapping', () => {
  it('maps DB states to FSRS states with sane defaults', () => {
    const engine = new FsrsEngine();
    const { mapDbStateToFsrs } = getInternals(engine);

    expect(mapDbStateToFsrs('new')).toBe(State.New);
    expect(mapDbStateToFsrs('learning')).toBe(State.Learning);
    expect(mapDbStateToFsrs('review')).toBe(State.Review);
    expect(mapDbStateToFsrs('relearning')).toBe(State.Relearning);
    expect(mapDbStateToFsrs('unknown' as ConceptState)).toBe(State.New);
  });

  it('maps FSRS states back to DB-friendly strings', () => {
    const engine = new FsrsEngine();
    const { mapFsrsStateToDb } = getInternals(engine);

    expect(mapFsrsStateToDb(State.New)).toBe('new');
    expect(mapFsrsStateToDb(State.Learning)).toBe('learning');
    expect(mapFsrsStateToDb(State.Review)).toBe('review');
    expect(mapFsrsStateToDb(State.Relearning)).toBe('relearning');
    expect(mapFsrsStateToDb(999 as State)).toBe('new');
  });
});
