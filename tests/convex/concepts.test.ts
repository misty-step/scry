import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Doc, Id } from '@/convex/_generated/dataModel';
import * as conceptsModule from '@/convex/concepts';
import { createMockCtx, createMockDb, makePhrasing } from '@/tests/helpers';

afterEach(() => {
  vi.clearAllMocks();
});

vi.mock('@/convex/fsrs', () => ({
  defaultEngine: {
    getRetrievability: vi.fn((_state, now: Date) => (now.getTime() % 1000) / 1000),
  },
  selectPhrasingForConcept: vi.fn((phrasings: Array<Doc<'phrasings'>>) => ({
    phrasing: phrasings[0],
    reason: 'least-seen',
  })),
  initializeConceptFsrs: vi.fn(() => ({
    state: 'new',
    difficulty: 0.3,
    stability: 0.4,
    reps: 0,
    lapses: 0,
    nextReview: Date.now(),
  })),
}));

describe('concepts.createMany', () => {
  it('skips short titles and duplicates, inserts normalized unique concepts', async () => {
    const insertedIds = ['c1', 'c2'];
    const db = createMockDb(
      {
        insert: vi.fn().mockResolvedValueOnce(insertedIds[0]).mockResolvedValueOnce(insertedIds[1]),
        query: vi.fn().mockReturnValue({
          withIndex: vi.fn().mockReturnThis(),
          take: vi
            .fn()
            .mockResolvedValue([{ title: 'existing concept' }] as Array<Doc<'concepts'>>),
        }),
      },
      []
    );

    const ctx = { db } as any;
    const concepts = [
      { title: 'abc', description: 'too short' },
      { title: 'Existing Concept', description: 'duplicate normalized' },
      { title: 'Unique One', description: 'valid' },
      { title: 'Another Unique', description: 'valid' },
    ];

    const result = await (conceptsModule.createMany as any)._handler(ctx, {
      userId: 'user_1',
      jobId: null,
      concepts,
    });

    expect(result.conceptIds).toEqual(insertedIds);
    expect(db.insert).toHaveBeenCalledTimes(2);
    expect(db.insert).toHaveBeenCalledWith(
      'concepts',
      expect.objectContaining({ title: 'Unique One' })
    );
  });
});

// Skip: These tests access private unexported functions.
// TODO: Either export via __test pattern or test through public API.
describe('concepts helpers (private functions)', () => {
  const { prioritizeConcepts, selectActivePhrasing } = conceptsModule.__test as {
    prioritizeConcepts: (
      concepts: Array<Doc<'concepts'>>,
      now: Date
    ) => Array<{ concept: Doc<'concepts'>; retrievability: number }>;
    selectActivePhrasing: (ctx: any, concept: Doc<'concepts'>, userId: Id<'users'>) => Promise<any>;
  };

  it('prioritizeConcepts filters concepts without phrasings and sorts by retrievability', () => {
    const now = new Date('2025-01-16T12:00:00Z');
    const concepts: Array<Doc<'concepts'>> = [
      { _id: 'c1', phrasingCount: 0, fsrs: { retrievability: 0.9 } } as any,
      { _id: 'c2', phrasingCount: 2, fsrs: { retrievability: 0.6 } } as any,
      { _id: 'c3', phrasingCount: 3, fsrs: { retrievability: 0.4 } } as any,
    ];

    vi.spyOn(Math, 'random').mockReturnValue(0); // deterministic shuffle
    const prioritized = prioritizeConcepts(concepts, now);
    expect(prioritized.map((p) => p.concept._id)).toEqual(['c3', 'c2']);
  });

  it('selectActivePhrasing returns phrasing with index and reason', async () => {
    const phrasingA = makePhrasing({ _id: 'p1' as Id<'phrasings'> });
    const phrasingB = makePhrasing({ _id: 'p2' as Id<'phrasings'> });
    const db = createMockDb(undefined, [phrasingA, phrasingB]);
    const ctx = createMockCtx({ db });

    const concept = {
      _id: 'concept_1' as Id<'concepts'>,
      canonicalPhrasingId: phrasingA._id,
    } as Doc<'concepts'>;

    const result = await selectActivePhrasing(ctx as any, concept, 'users_1' as Id<'users'>);

    expect(result?.phrasing._id).toBe('p1');
    expect(result?.totalPhrasings).toBe(2);
    expect(result?.phrasingIndex).toBeGreaterThanOrEqual(1);
    expect(result?.selectionReason).toBe('least-seen');
  });

  it('selectActivePhrasing returns null when no active phrasings', async () => {
    const db = createMockDb(undefined, []);
    const ctx = createMockCtx({ db });
    const concept = { _id: 'concept_1' as Id<'concepts'> } as Doc<'concepts'>;

    const result = await selectActivePhrasing(ctx as any, concept, 'users_1' as Id<'users'>);
    expect(result).toBeNull();
  });
});
