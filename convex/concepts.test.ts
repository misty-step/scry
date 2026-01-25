import { describe, expect, it, vi } from 'vitest';

const MAX_PHRASINGS_PER_BATCH = 50;
const MAX_BATCH_ITERATIONS = 100;

type PhrasingDoc = {
  _id: string;
  isArchived: boolean;
};

type PhrasingFilter = (doc: PhrasingDoc) => boolean;

type MockDb = {
  query: (table: string) => {
    withIndex: (
      index: string,
      ..._rest: unknown[]
    ) => {
      filter: (filterFn: PhrasingFilter) => {
        take: (limit: number) => Promise<PhrasingDoc[]>;
      };
    };
  };
  patch: (id: string, patch: Record<string, unknown>) => Promise<void>;
};

type MockCtx = { db: MockDb };

const makePhrasings = (count: number, isArchived = false, prefix = ''): PhrasingDoc[] =>
  Array.from({ length: count }, (_, index) => ({
    _id: `phrasing-${prefix}${index + 1}`,
    isArchived,
  }));

const createMockCtx = (docs: PhrasingDoc[]) => {
  let currentFilter: PhrasingFilter | null = null;

  const take = vi.fn(async (limit: number) => {
    const filtered = docs.filter((doc) => (currentFilter ? currentFilter(doc) : true));
    return filtered.slice(0, limit);
  });

  const filter = vi.fn((filterFn: PhrasingFilter) => {
    currentFilter = filterFn;
    return { take };
  });

  const withIndex = vi.fn(() => ({ filter }));
  const query = vi.fn(() => ({ withIndex }));
  const patch = vi.fn(async (id: string, patchData: Record<string, unknown>) => {
    const doc = docs.find((candidate) => candidate._id === id);
    if (doc) {
      Object.assign(doc, patchData);
    }
  });

  return { ctx: { db: { query, patch } } as MockCtx, take, patch, query, withIndex, filter };
};

const updatePhrasingsBatchedSim = async (
  ctx: MockCtx,
  userId: string,
  conceptId: string,
  filter: PhrasingFilter,
  patch: Record<string, unknown>
): Promise<number> => {
  let processed = 0;
  let iterations = 0;

  while (iterations < MAX_BATCH_ITERATIONS) {
    iterations++;
    const batch = await ctx.db
      .query('phrasings')
      .withIndex('by_user_concept', () => ({ userId, conceptId }))
      .filter(filter)
      .take(MAX_PHRASINGS_PER_BATCH);

    if (batch.length === 0) {
      break;
    }

    for (const phrasing of batch) {
      await ctx.db.patch(phrasing._id, patch);
      processed++;
    }
  }

  if (iterations >= MAX_BATCH_ITERATIONS) {
    console.error(`updatePhrasingsBatched: Hit MAX_BATCH_ITERATIONS for concept ${conceptId}`);
  }

  return processed;
};

describe('updatePhrasingsBatched simulation', () => {
  it('processes all phrasings when count < 50', async () => {
    const docs = makePhrasings(12);
    const { ctx, take, patch } = createMockCtx(docs);

    const processed = await updatePhrasingsBatchedSim(
      ctx,
      'user-1',
      'concept-1',
      (doc) => !doc.isArchived,
      { isArchived: true }
    );

    expect(processed).toBe(12);
    expect(patch).toHaveBeenCalledTimes(12);
    expect(docs.every((doc) => doc.isArchived)).toBe(true);
    expect(take).toHaveBeenCalledTimes(2);
  });

  it('handles empty result set gracefully', async () => {
    const docs: PhrasingDoc[] = [];
    const { ctx, take, patch } = createMockCtx(docs);

    const processed = await updatePhrasingsBatchedSim(
      ctx,
      'user-1',
      'concept-1',
      (doc) => !doc.isArchived,
      { isArchived: true }
    );

    expect(processed).toBe(0);
    expect(patch).not.toHaveBeenCalled();
    expect(take).toHaveBeenCalledTimes(1);
  });

  it.each([
    { count: 50, expectedTakeCalls: 2, label: 'exactly one full batch' },
    { count: 75, expectedTakeCalls: 3, label: 'two batches' },
    { count: 150, expectedTakeCalls: 4, label: 'three batches' },
  ])('processes $count phrasings across $label', async ({ count, expectedTakeCalls }) => {
    const docs = makePhrasings(count);
    const { ctx, take, patch } = createMockCtx(docs);

    const processed = await updatePhrasingsBatchedSim(
      ctx,
      'user-1',
      'concept-1',
      (doc) => !doc.isArchived,
      { isArchived: true }
    );

    expect(processed).toBe(count);
    expect(patch).toHaveBeenCalledTimes(count);
    expect(docs.every((doc) => doc.isArchived)).toBe(true);
    expect(take).toHaveBeenCalledTimes(expectedTakeCalls);
  });

  it('respects MAX_BATCH_ITERATIONS limit', async () => {
    const docs = makePhrasings(MAX_PHRASINGS_PER_BATCH);
    const { ctx, take, patch } = createMockCtx(docs);
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});

    const processed = await updatePhrasingsBatchedSim(ctx, 'user-1', 'concept-1', () => true, {});

    expect(processed).toBe(MAX_PHRASINGS_PER_BATCH * MAX_BATCH_ITERATIONS);
    expect(take).toHaveBeenCalledTimes(MAX_BATCH_ITERATIONS);
    expect(patch).toHaveBeenCalledTimes(MAX_PHRASINGS_PER_BATCH * MAX_BATCH_ITERATIONS);
    expect(consoleError).toHaveBeenCalledTimes(1);
    consoleError.mockRestore();
  });

  it('can process up to 5000 phrasings', async () => {
    const docs = makePhrasings(MAX_PHRASINGS_PER_BATCH * MAX_BATCH_ITERATIONS);
    const { ctx, take, patch } = createMockCtx(docs);
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});

    const processed = await updatePhrasingsBatchedSim(
      ctx,
      'user-1',
      'concept-1',
      (doc) => !doc.isArchived,
      { isArchived: true }
    );

    expect(processed).toBe(MAX_PHRASINGS_PER_BATCH * MAX_BATCH_ITERATIONS);
    expect(patch).toHaveBeenCalledTimes(MAX_PHRASINGS_PER_BATCH * MAX_BATCH_ITERATIONS);
    expect(take).toHaveBeenCalledTimes(MAX_BATCH_ITERATIONS);
    expect(docs.every((doc) => doc.isArchived)).toBe(true);
    consoleError.mockRestore();
  });

  it('excludes already-patched documents via filter-based pagination', async () => {
    const docs = [...makePhrasings(20, true, 'archived-'), ...makePhrasings(100, false, 'active-')];
    const archivedIds = new Set(docs.filter((doc) => doc.isArchived).map((doc) => doc._id));
    const { ctx, take, patch } = createMockCtx(docs);

    const processed = await updatePhrasingsBatchedSim(
      ctx,
      'user-1',
      'concept-1',
      (doc) => !doc.isArchived,
      { isArchived: true }
    );

    const patchedIds = new Set(patch.mock.calls.map(([id]) => id as string));

    expect(processed).toBe(100);
    expect(patch).toHaveBeenCalledTimes(100);
    expect(take).toHaveBeenCalledTimes(3);
    expect(docs.every((doc) => doc.isArchived)).toBe(true);
    for (const archivedId of archivedIds) {
      expect(patchedIds.has(archivedId)).toBe(false);
    }
  });

  it('handles unarchive (reverse operation)', async () => {
    const docs = makePhrasings(30, true);
    const { ctx, take, patch } = createMockCtx(docs);

    const processed = await updatePhrasingsBatchedSim(
      ctx,
      'user-1',
      'concept-1',
      (doc) => doc.isArchived,
      { isArchived: false }
    );

    expect(processed).toBe(30);
    expect(patch).toHaveBeenCalledTimes(30);
    expect(take).toHaveBeenCalledTimes(2);
    expect(docs.every((doc) => !doc.isArchived)).toBe(true);
  });
});
