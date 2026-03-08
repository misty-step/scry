import { describe, expect, it, vi } from 'vitest';
import {
  clampPageSize,
  computeThinScoreFromCount,
  matchesConceptView,
  prioritizeConcepts,
  type ConceptDoc,
} from './conceptHelpers';

const defaults = { min: 10, max: 100, default: 25 };

const baseFsrs = {
  nextReview: Date.now(),
  stability: 1,
  difficulty: 2,
  state: 'new' as const,
  scheduledDays: 0,
  lastReview: Date.now(),
  elapsedDays: 0,
  retrievability: undefined as number | undefined,
  reps: 0,
  lapses: 0,
};

const concept = (overrides: Partial<ConceptDoc> = {}): ConceptDoc =>
  ({
    fsrs: { ...baseFsrs },
    phrasingCount: 1,
    ...overrides,
  }) as ConceptDoc;

describe('clampPageSize', () => {
  it('returns default when undefined', () => {
    expect(clampPageSize(undefined, defaults)).toBe(defaults.default);
  });

  it('clamps below minimum', () => {
    expect(clampPageSize(5, defaults)).toBe(defaults.min);
  });

  it('clamps above maximum', () => {
    expect(clampPageSize(500, defaults)).toBe(defaults.max);
  });

  it('returns value when within range', () => {
    expect(clampPageSize(50, defaults)).toBe(50);
  });
});

describe('matchesConceptView', () => {
  const now = Date.now();

  it('returns true for deleted view when concept is deleted', () => {
    expect(matchesConceptView(concept({ deletedAt: now }), now, 'deleted')).toBe(true);
  });

  it('excludes deleted concepts from active views', () => {
    expect(matchesConceptView(concept({ deletedAt: now }), now, 'due')).toBe(false);
  });

  it('returns true for archived view when archived', () => {
    expect(matchesConceptView(concept({ archivedAt: now }), now, 'archived')).toBe(true);
  });

  it('excludes archived concepts from active views', () => {
    expect(matchesConceptView(concept({ archivedAt: now }), now, 'thin')).toBe(false);
  });

  it('checks due status for due view', () => {
    const past = now - 1000;
    expect(
      matchesConceptView(
        concept({ fsrs: { ...baseFsrs, nextReview: past, retrievability: undefined } }),
        now,
        'due'
      )
    ).toBe(true);
    expect(
      matchesConceptView(
        concept({ fsrs: { ...baseFsrs, nextReview: now + 1000, retrievability: undefined } }),
        now,
        'due'
      )
    ).toBe(false);
  });

  it('checks thin and tension scores', () => {
    expect(matchesConceptView(concept({ thinScore: 1 }), now, 'thin')).toBe(true);
    expect(matchesConceptView(concept({ conflictScore: 1 }), now, 'tension')).toBe(true);
  });
});

describe('computeThinScoreFromCount', () => {
  const target = 5;

  it('returns undefined when at or above target', () => {
    expect(computeThinScoreFromCount(5, target)).toBeUndefined();
    expect(computeThinScoreFromCount(7, target)).toBeUndefined();
  });

  it('returns positive delta when below target', () => {
    expect(computeThinScoreFromCount(2, target)).toBe(3);
  });
});

describe('prioritizeConcepts', () => {
  const now = new Date();

  it('sorts by retrievability ascending and uses provided retrievability fn', () => {
    const c1 = concept({ fsrs: { ...baseFsrs, nextReview: 0 } });
    const c2 = concept({ fsrs: { ...baseFsrs, nextReview: 0 } });
    const getRetrievability = vi.fn().mockReturnValueOnce(0.8).mockReturnValueOnce(0.3);

    const result = prioritizeConcepts([c1, c2], now, getRetrievability);

    expect(result.map((r) => r.concept)).toEqual([c2, c1]);
    expect(getRetrievability).toHaveBeenCalledTimes(2);
  });

  it('shuffles urgent tier deterministically when random provided', () => {
    const c1 = concept({ fsrs: { ...baseFsrs, nextReview: 0, retrievability: 0.1 } });
    const c2 = concept({ fsrs: { ...baseFsrs, nextReview: 0, retrievability: 0.12 } });
    const c3 = concept({ fsrs: { ...baseFsrs, nextReview: 0, retrievability: 0.14 } });
    const c4 = concept({ fsrs: { ...baseFsrs, nextReview: 0, retrievability: 0.3 } });

    const randomValues = [0.5, 0];
    const random = vi.fn().mockImplementation(() => randomValues.shift() ?? 0);

    const result = prioritizeConcepts(
      [c1, c2, c3, c4],
      now,
      (fsrs) => fsrs.retrievability!,
      random
    );

    expect(result.map((r) => r.concept)).toEqual([c3, c1, c2, c4]);
  });

  it('returns empty array when no candidates', () => {
    const result = prioritizeConcepts([], now, () => 0.5);
    expect(result).toEqual([]);
  });
});
