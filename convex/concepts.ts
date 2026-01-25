import type { Expression, FilterBuilder } from 'convex/server';
import { v } from 'convex/values';
import type { ConceptBulkAction } from '../types/concepts';
import { internal } from './_generated/api';
import type { DataModel, Doc, Id } from './_generated/dataModel';
import { internalMutation, internalQuery, mutation, query } from './_generated/server';
import type { MutationCtx, QueryCtx } from './_generated/server';
import { requireUserFromClerk } from './clerk';
import {
  defaultEngine as conceptEngine,
  initializeConceptFsrs,
  scheduleConceptReview,
  selectPhrasingForConcept,
} from './fsrs';
import { TARGET_PHRASINGS_PER_CONCEPT } from './lib/conceptConstants';
import { calculateConceptStatsDelta } from './lib/conceptFsrsHelpers';
import {
  clampPageSize,
  computeThinScoreFromCount,
  matchesConceptView,
  prioritizeConcepts,
  type ConceptLibraryView,
} from './lib/conceptHelpers';
import { buildInteractionContext } from './lib/interactionContext';
import { calculateStateTransitionDelta, updateStatsCounters } from './lib/userStatsHelpers';

type ConceptDoc = Doc<'concepts'>;
type PhrasingDoc = Doc<'phrasings'>;
type PhrasingFilter = (q: FilterBuilder<DataModel['phrasings']>) => Expression<boolean>;

type SelectionResult = {
  concept: ConceptDoc;
  phrasing: PhrasingDoc;
  selectionReason: ReturnType<typeof selectPhrasingForConcept>['reason'];
  totalPhrasings: number;
  phrasingIndex: number;
};

const MAX_CONCEPT_CANDIDATES = 25;
const MAX_PHRASINGS = 50;
const MAX_PHRASINGS_PER_BATCH = 50; // Batch size for paginated phrasing updates (issue #121)
const MAX_BATCH_ITERATIONS = 100; // Safety limit: 100 * 50 = 5000 phrasings max
const MAX_INTERACTIONS = 10;
const MAX_EXISTING_TITLES = 250;
const DEFAULT_LIBRARY_PAGE_SIZE = 25;
const MAX_LIBRARY_PAGE_SIZE = 100;
const MIN_LIBRARY_PAGE_SIZE = 10;
type ConceptLibrarySort = 'recent' | 'nextReview';

/**
 * Paginate through all phrasings for a concept and apply an update.
 * Prevents unbounded queries while ensuring ALL phrasings are processed.
 *
 * PAGINATION PATTERN: Each iteration patches matching phrasings, which changes
 * their state (e.g., isArchived: false → true). Since the filter excludes
 * already-patched documents, the next query returns the NEXT batch of unpatched
 * phrasings. This is "filter-based pagination" - no cursor needed because the
 * filter condition itself advances through the dataset.
 *
 * Safety cap (MAX_BATCH_ITERATIONS) prevents infinite loops if filter doesn't
 * properly exclude patched documents.
 */
async function updatePhrasingsBatched(
  ctx: MutationCtx,
  userId: Id<'users'>,
  conceptId: Id<'concepts'>,
  filter: PhrasingFilter,
  patch: Record<string, unknown>
): Promise<number> {
  let processed = 0;
  let iterations = 0;

  while (iterations < MAX_BATCH_ITERATIONS) {
    iterations++;
    const batch = await ctx.db
      .query('phrasings')
      .withIndex('by_user_concept', (q) => q.eq('userId', userId).eq('conceptId', conceptId))
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
}

export const createMany = internalMutation({
  args: {
    userId: v.id('users'),
    jobId: v.optional(v.id('generationJobs')),
    concepts: v.array(
      v.object({
        title: v.string(),
        description: v.optional(v.string()),
        contentType: v.optional(
          v.union(
            v.literal('verbatim'),
            v.literal('enumerable'),
            v.literal('conceptual'),
            v.literal('mixed')
          )
        ),
        originIntent: v.optional(v.string()),
      })
    ),
  },
  handler: async (ctx, args) => {
    if (args.concepts.length === 0) {
      return { conceptIds: [] };
    }

    const now = Date.now();
    const createdIds: Id<'concepts'>[] = [];
    const normalizedExistingTitles = new Set<string>();

    const existingConcepts = await ctx.db
      .query('concepts')
      .withIndex('by_user', (q) => q.eq('userId', args.userId))
      .take(MAX_EXISTING_TITLES);

    for (const concept of existingConcepts) {
      normalizedExistingTitles.add(concept.title.trim().toLowerCase());
    }

    for (const concept of args.concepts) {
      const title = concept.title.trim();
      const description = concept.description?.trim();

      if (title.length < 5) {
        continue;
      }

      const titleKey = title.toLowerCase();
      if (normalizedExistingTitles.has(titleKey)) {
        continue;
      }

      normalizedExistingTitles.add(titleKey);

      const fsrs = initializeConceptFsrs(new Date(now));

      const conceptId = await ctx.db.insert('concepts', {
        userId: args.userId,
        title,
        description,
        contentType: concept.contentType,
        originIntent: concept.originIntent,
        fsrs,
        phrasingCount: 0,
        conflictScore: undefined,
        thinScore: undefined,
        qualityScore: undefined,
        canonicalPhrasingId: undefined,
        embedding: undefined,
        embeddingGeneratedAt: undefined,
        createdAt: now,
        updatedAt: now,
        generationJobId: args.jobId,
      });

      createdIds.push(conceptId);
    }

    return { conceptIds: createdIds };
  },
});

export const getDue = query({
  args: {
    _refreshTimestamp: v.optional(v.number()),
  },
  handler: async (ctx) => {
    const user = await requireUserFromClerk(ctx);
    const userId = user._id;
    const now = new Date();
    const nowMs = now.getTime();

    // Filter out concepts without phrasings at DB level to avoid N+1 queries
    // in the loop below (each selectActivePhrasing call was a wasted query)
    const dueConcepts = await ctx.db
      .query('concepts')
      .withIndex('by_user_next_review', (q) =>
        q
          .eq('userId', userId)
          .eq('deletedAt', undefined)
          .eq('archivedAt', undefined)
          .lte('fsrs.nextReview', nowMs)
      )
      .filter((q) => q.gt(q.field('phrasingCount'), 0))
      .take(MAX_CONCEPT_CANDIDATES);

    let candidates = dueConcepts;

    if (candidates.length === 0) {
      const newConcepts = await ctx.db
        .query('concepts')
        .withIndex('by_user_next_review', (q) =>
          q.eq('userId', userId).eq('deletedAt', undefined).eq('archivedAt', undefined)
        )
        .filter((q) => q.and(q.eq(q.field('fsrs.state'), 'new'), q.gt(q.field('phrasingCount'), 0)))
        .take(MAX_CONCEPT_CANDIDATES);

      if (newConcepts.length === 0) {
        // Never return future-scheduled concepts—breaks FSRS intervals.
        return null;
      }

      candidates = newConcepts;
    }

    const prioritized = prioritizeConcepts(candidates, now, (fsrs, date) =>
      conceptEngine.getRetrievability(fsrs, date)
    );
    for (const candidate of prioritized) {
      const phrasingSelection = await selectActivePhrasing(ctx, candidate.concept, userId);
      if (!phrasingSelection) {
        continue;
      }

      const interactions = await ctx.db
        .query('interactions')
        .withIndex('by_user_phrasing', (q) =>
          q.eq('userId', userId).eq('phrasingId', phrasingSelection.phrasing._id)
        )
        .order('desc')
        .take(MAX_INTERACTIONS);

      return {
        concept: candidate.concept,
        phrasing: phrasingSelection.phrasing,
        selectionReason: phrasingSelection.selectionReason,
        phrasingStats: {
          total: phrasingSelection.totalPhrasings,
          index: phrasingSelection.phrasingIndex,
        },
        retrievability: candidate.retrievability,
        interactions,
        serverTime: nowMs,
      };
    }

    return null;
  },
});

export const getConceptsDueCount = query({
  args: {},
  handler: async (ctx) => {
    const user = await requireUserFromClerk(ctx);
    const userId = user._id;
    const nowMs = Date.now();

    // Count due concepts with phrasings (DB-level filtering)
    const dueConcepts = await ctx.db
      .query('concepts')
      .withIndex('by_user_next_review', (q) =>
        q
          .eq('userId', userId)
          .eq('deletedAt', undefined)
          .eq('archivedAt', undefined)
          .lte('fsrs.nextReview', nowMs)
      )
      .filter((q) => q.gt(q.field('phrasingCount'), 0))
      .take(1000); // Safety limit for large collections

    return {
      conceptsDue: dueConcepts.length,
    };
  },
});

export const recordInteraction = mutation({
  args: {
    conceptId: v.id('concepts'),
    phrasingId: v.id('phrasings'),
    userAnswer: v.string(),
    isCorrect: v.boolean(),
    timeSpent: v.optional(v.number()),
    sessionId: v.optional(v.string()),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const userId = user._id;

    const concept = await ctx.db.get(args.conceptId);
    if (!concept || concept.userId !== userId) {
      throw new Error('Concept not found or unauthorized');
    }

    const phrasing = await ctx.db.get(args.phrasingId);
    if (!phrasing || phrasing.userId !== userId || phrasing.conceptId !== concept._id) {
      throw new Error('Phrasing not found or unauthorized');
    }

    const nowMs = Date.now();
    const now = new Date(nowMs);

    const scheduleResult = scheduleConceptReview(concept, args.isCorrect, { now });

    const interactionContext = buildInteractionContext({
      sessionId: args.sessionId,
      scheduledDays: scheduleResult.scheduledDays,
      nextReview: scheduleResult.nextReview,
      fsrsState: scheduleResult.state,
    });

    const interactionId = await ctx.db.insert('interactions', {
      userId,
      conceptId: concept._id,
      phrasingId: phrasing._id,
      userAnswer: args.userAnswer,
      isCorrect: args.isCorrect,
      attemptedAt: nowMs,
      timeSpent: args.timeSpent,
      context: interactionContext,
    });

    await ctx.db.patch(phrasing._id, {
      attemptCount: (phrasing.attemptCount ?? 0) + 1,
      correctCount: (phrasing.correctCount ?? 0) + (args.isCorrect ? 1 : 0),
      lastAttemptedAt: nowMs,
    });

    await ctx.db.patch(concept._id, {
      fsrs: scheduleResult.fsrs,
      updatedAt: nowMs,
    });

    const statsDelta = calculateConceptStatsDelta({
      oldState: concept.fsrs.state ?? 'new',
      newState: scheduleResult.state,
      oldNextReview: concept.fsrs.nextReview,
      newNextReview: scheduleResult.nextReview,
      nowMs,
    });

    if (statsDelta) {
      await updateStatsCounters(ctx, userId, statsDelta);
    }

    return {
      conceptId: concept._id,
      phrasingId: phrasing._id,
      interactionId,
      nextReview: scheduleResult.nextReview,
      scheduledDays: scheduleResult.scheduledDays,
      newState: scheduleResult.state,
    };
  },
});

/**
 * Record user feedback on question quality.
 * Used for LLM observability - helps identify questions that need improvement.
 *
 * Note: This intentionally overwrites any existing feedback on the interaction.
 * Users may change their mind after seeing the explanation or thinking more.
 */
export const recordFeedback = mutation({
  args: {
    interactionId: v.id('interactions'),
    feedbackType: v.union(
      v.literal('helpful'),
      v.literal('unhelpful'),
      v.literal('unclear'),
      v.literal('incorrect')
    ),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);

    const interaction = await ctx.db.get(args.interactionId);
    if (!interaction || interaction.userId !== user._id) {
      throw new Error('Interaction not found or unauthorized');
    }

    await ctx.db.patch(args.interactionId, {
      feedback: {
        type: args.feedbackType,
        givenAt: Date.now(),
      },
    });

    return { success: true };
  },
});

async function selectActivePhrasing(
  ctx: QueryCtx,
  concept: ConceptDoc,
  userId: Id<'users'>
): Promise<SelectionResult | null> {
  const phrasings = await ctx.db
    .query('phrasings')
    .withIndex('by_user_concept', (q) => q.eq('userId', userId).eq('conceptId', concept._id))
    .filter((q) =>
      q.and(q.eq(q.field('archivedAt'), undefined), q.eq(q.field('deletedAt'), undefined))
    )
    .take(MAX_PHRASINGS);

  if (phrasings.length === 0) {
    return null;
  }

  const totalPhrasings = phrasings.length;

  const selection = selectPhrasingForConcept(phrasings, {
    canonicalPhrasingId: concept.canonicalPhrasingId,
  });
  if (!selection.phrasing) {
    return null;
  }

  const zeroBasedIndex = phrasings.findIndex((p) => p._id === selection.phrasing?._id);
  const phrasingIndex = zeroBasedIndex === -1 ? 1 : zeroBasedIndex + 1;

  return {
    concept,
    phrasing: selection.phrasing,
    selectionReason: selection.reason,
    totalPhrasings,
    phrasingIndex,
  };
}

export const getConceptById = internalQuery({
  args: {
    conceptId: v.id('concepts'),
  },
  handler: async (ctx, args) => {
    return await ctx.db.get(args.conceptId);
  },
});

export const applyPhrasingGenerationUpdate = internalMutation({
  args: {
    conceptId: v.id('concepts'),
    phrasingCount: v.number(),
    thinScore: v.optional(v.number()),
    conflictScore: v.optional(v.number()),
  },
  handler: async (ctx, args) => {
    await ctx.db.patch(args.conceptId, {
      phrasingCount: args.phrasingCount,
      thinScore: args.thinScore,
      conflictScore: args.conflictScore,
      updatedAt: Date.now(),
    });
  },
});

export const listForLibrary = query({
  args: {
    cursor: v.optional(v.string()),
    pageSize: v.optional(v.number()),
    view: v.optional(
      v.union(
        v.literal('all'),
        v.literal('due'),
        v.literal('thin'),
        v.literal('tension'),
        v.literal('archived'),
        v.literal('deleted')
      )
    ),
    search: v.optional(v.string()),
    sort: v.optional(v.union(v.literal('recent'), v.literal('nextReview'))),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const now = Date.now();
    const pageSize = clampPageSize(args.pageSize, {
      min: MIN_LIBRARY_PAGE_SIZE,
      max: MAX_LIBRARY_PAGE_SIZE,
      default: DEFAULT_LIBRARY_PAGE_SIZE,
    });
    const view = (args.view ?? 'all') as ConceptLibraryView;
    const sort = (args.sort ?? 'nextReview') as ConceptLibrarySort;
    const searchTerm = args.search?.trim() ?? '';
    const cursor = args.cursor ?? null;

    // Mode 1: Search (unchanged)
    if (searchTerm.length >= 2) {
      let searchQuery = ctx.db
        .query('concepts')
        .withSearchIndex('search_concepts', (q) => q.search('title', searchTerm));

      // Filter by user AND active status
      // Note: Search doesn't support compound filters well, so we must post-filter
      // This is acceptable for search results which are usually small in number
      searchQuery = searchQuery.filter((q) =>
        q.and(
          q.eq(q.field('userId'), user._id),
          view === 'deleted'
            ? q.neq(q.field('deletedAt'), undefined)
            : q.eq(q.field('deletedAt'), undefined),
          view === 'archived'
            ? q.neq(q.field('archivedAt'), undefined)
            : q.eq(q.field('archivedAt'), undefined)
        )
      );

      const searchResults = await searchQuery.take(Math.min(pageSize, 50));

      // Apply remaining view filters (thin, tension, due) in memory
      const filteredResults = searchResults.filter((concept) =>
        matchesConceptView(concept, now, view)
      );

      return {
        concepts: filteredResults.slice(0, pageSize),
        continueCursor: null,
        isDone: true,
        serverTime: now,
        mode: 'search' as const,
      };
    }

    // Mode 2: Optimized Views (with efficient filtering)
    let baseQuery;

    // Case A: Trash View
    if (view === 'deleted') {
      baseQuery = ctx.db
        .query('concepts')
        .withIndex('by_user_active', (q) => q.eq('userId', user._id).gt('deletedAt', 0));
    }
    // Case B: Archive View
    else if (view === 'archived') {
      baseQuery = ctx.db
        .query('concepts')
        .withIndex('by_user_active', (q) =>
          q.eq('userId', user._id).eq('deletedAt', undefined).gt('archivedAt', 0)
        );
    }
    // Case C: Active Views (All, Due, Thin, Tension)
    else {
      // Standard Active View: deletedAt=undefined, archivedAt=undefined

      if (sort === 'recent') {
        // Sort by Created At
        // Use by_user_active: userId, deletedAt=undefined, archivedAt=undefined
        // This index ends with createdAt, so it naturally sorts by creation time
        baseQuery = ctx.db
          .query('concepts')
          .withIndex('by_user_active', (q) =>
            q.eq('userId', user._id).eq('deletedAt', undefined).eq('archivedAt', undefined)
          )
          .order('desc');
      } else {
        // Sort by Next Review (Default)
        // Use by_user_next_review: userId, deletedAt, archivedAt, fsrs.nextReview
        baseQuery = ctx.db
          .query('concepts')
          .withIndex('by_user_next_review', (q) =>
            q.eq('userId', user._id).eq('deletedAt', undefined).eq('archivedAt', undefined)
          );
      }
    }

    // Execute Pagination
    // Convex's paginate() efficiently handles filters by scanning until numItems is reached
    const page = await baseQuery.paginate({
      cursor,
      numItems: pageSize,
    });

    // No post-filtering needed for deleted/archived, but still needed for 'due', 'thin', 'tension'
    const concepts = page.page.filter((concept) => matchesConceptView(concept, now, view));

    return {
      concepts,
      continueCursor: page.continueCursor,
      isDone: page.isDone,
      serverTime: now,
      mode: 'standard' as const,
    };
  },
});

// Removed collectConceptPage helper function entirely to prevent future misuse

export const getDetail = query({
  args: {
    conceptId: v.id('concepts'),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const concept = await ctx.db.get(args.conceptId);

    if (!concept || concept.userId !== user._id) {
      return null;
    }

    const phrasings = await ctx.db
      .query('phrasings')
      .withIndex('by_user_concept', (q) => q.eq('userId', user._id).eq('conceptId', concept._id))
      .filter((q) =>
        q.and(q.eq(q.field('archivedAt'), undefined), q.eq(q.field('deletedAt'), undefined))
      )
      .order('desc')
      .take(100);

    const pendingJob = await findActiveGenerationJob(ctx, user._id, concept._id);

    return {
      concept,
      phrasings,
      pendingGeneration: Boolean(pendingJob),
    };
  },
});

export const setCanonicalPhrasing = mutation({
  args: {
    conceptId: v.id('concepts'),
    phrasingId: v.optional(v.id('phrasings')),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const concept = await ctx.db.get(args.conceptId);
    if (!concept || concept.userId !== user._id) {
      throw new Error('Concept not found or unauthorized');
    }

    if (args.phrasingId) {
      const phrasing = await ctx.db.get(args.phrasingId);
      if (
        !phrasing ||
        phrasing.userId !== user._id ||
        phrasing.conceptId !== concept._id ||
        phrasing.archivedAt
      ) {
        throw new Error('Phrasing not found or unavailable');
      }
    }

    await ctx.db.patch(concept._id, {
      canonicalPhrasingId: args.phrasingId ?? undefined,
      updatedAt: Date.now(),
    });

    return { canonicalPhrasingId: args.phrasingId ?? null };
  },
});

export const archivePhrasing = mutation({
  args: {
    conceptId: v.id('concepts'),
    phrasingId: v.id('phrasings'),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const concept = await ctx.db.get(args.conceptId);
    if (!concept || concept.userId !== user._id) {
      throw new Error('Concept not found or unauthorized');
    }

    const phrasing = await ctx.db.get(args.phrasingId);
    if (!phrasing || phrasing.userId !== user._id || phrasing.conceptId !== concept._id) {
      throw new Error('Phrasing not found or unauthorized');
    }

    if (phrasing.archivedAt) {
      return { phrasingId: phrasing._id };
    }

    const now = Date.now();
    await ctx.db.patch(phrasing._id, {
      archivedAt: now,
      updatedAt: now,
    });

    // Fetch remaining active phrasings to recalculate scores
    const remainingPhrasings = await ctx.db
      .query('phrasings')
      .withIndex('by_user_concept', (q) => q.eq('userId', user._id).eq('conceptId', concept._id))
      .filter((q) => q.eq(q.field('archivedAt'), undefined))
      .collect();

    const newCount = remainingPhrasings.length;
    const thinScore = computeThinScoreFromCount(newCount, TARGET_PHRASINGS_PER_CONCEPT);

    // Recalculate conflictScore from remaining phrasings
    let conflictScore: number | undefined = undefined;
    if (newCount > 1) {
      const phrasingsNormalized = remainingPhrasings.map((p) => p.question.trim().toLowerCase());
      const uniqueQuestions = new Set(phrasingsNormalized);
      const conflictCount = phrasingsNormalized.length - uniqueQuestions.size;
      conflictScore = conflictCount > 0 ? conflictCount : undefined;
    }

    const conceptPatch: Partial<ConceptDoc> = {
      phrasingCount: newCount,
      thinScore,
      conflictScore,
      updatedAt: now,
    };

    if (concept.canonicalPhrasingId === phrasing._id) {
      conceptPatch.canonicalPhrasingId = undefined;
    }

    await ctx.db.patch(concept._id, conceptPatch);

    return { phrasingId: phrasing._id };
  },
});

/**
 * Unarchive a phrasing and recalculate concept scores.
 * Reverses archivePhrasing operation.
 */
export const unarchivePhrasing = mutation({
  args: {
    conceptId: v.id('concepts'),
    phrasingId: v.id('phrasings'),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const concept = await ctx.db.get(args.conceptId);
    if (!concept || concept.userId !== user._id) {
      throw new Error('Concept not found or unauthorized');
    }

    const phrasing = await ctx.db.get(args.phrasingId);
    if (!phrasing || phrasing.userId !== user._id || phrasing.conceptId !== concept._id) {
      throw new Error('Phrasing not found or unauthorized');
    }

    // Idempotent: return early if not archived
    if (!phrasing.archivedAt) {
      return { phrasingId: phrasing._id };
    }

    const now = Date.now();
    await ctx.db.patch(phrasing._id, {
      archivedAt: undefined,
      updatedAt: now,
    });

    // Fetch all active phrasings (including newly unarchived)
    const activePhrasings = await ctx.db
      .query('phrasings')
      .withIndex('by_user_concept', (q) => q.eq('userId', user._id).eq('conceptId', concept._id))
      .filter((q) => q.eq(q.field('archivedAt'), undefined))
      .collect();

    const newCount = activePhrasings.length;
    const thinScore = computeThinScoreFromCount(newCount, TARGET_PHRASINGS_PER_CONCEPT);

    // Recalculate conflictScore from active phrasings
    let conflictScore: number | undefined = undefined;
    if (newCount > 1) {
      const normalized = activePhrasings.map((p) => p.question.trim().toLowerCase());
      const uniqueQuestions = new Set(normalized);
      const conflictCount = normalized.length - uniqueQuestions.size;
      conflictScore = conflictCount > 0 ? conflictCount : undefined;
    }

    const conceptPatch: Partial<ConceptDoc> = {
      phrasingCount: newCount,
      thinScore,
      conflictScore,
      updatedAt: now,
    };

    await ctx.db.patch(concept._id, conceptPatch);

    return { phrasingId: phrasing._id };
  },
});

export const requestPhrasingGeneration = mutation({
  args: {
    conceptId: v.id('concepts'),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const concept = await ctx.db.get(args.conceptId);
    if (!concept || concept.userId !== user._id) {
      throw new Error('Concept not found or unauthorized');
    }

    const existingJob = await findActiveGenerationJob(ctx, user._id, concept._id);
    if (existingJob) {
      throw new Error('Generation already in progress for this concept');
    }

    const now = Date.now();
    const jobId = await ctx.db.insert('generationJobs', {
      userId: user._id,
      prompt: `Manual concept phrasing request: ${concept.title}`,
      status: 'pending',
      phase: 'phrasing_generation',
      phrasingGenerated: 0,
      phrasingSaved: 0,
      estimatedTotal: TARGET_PHRASINGS_PER_CONCEPT,
      topic: concept.title,
      conceptIds: [],
      pendingConceptIds: [],
      durationMs: undefined,
      errorMessage: undefined,
      errorCode: undefined,
      retryable: undefined,
      createdAt: now,
      startedAt: undefined,
      completedAt: undefined,
      ipAddress: undefined,
    });

    await ctx.runMutation(internal.generationJobs.setConceptWork, {
      jobId,
      conceptIds: [concept._id],
    });

    await ctx.runMutation(internal.generationJobs.updateProgress, {
      jobId,
      phase: 'phrasing_generation',
      estimatedTotal: TARGET_PHRASINGS_PER_CONCEPT,
    });

    await ctx.scheduler.runAfter(0, internal.aiGeneration.generatePhrasingsForConcept, {
      conceptId: concept._id,
      jobId,
    });

    return { jobId };
  },
});

async function findActiveGenerationJob(
  ctx: QueryCtx | MutationCtx,
  userId: Id<'users'>,
  conceptId: Id<'concepts'>
) {
  const statuses: Array<'pending' | 'processing'> = ['pending', 'processing'];

  for (const status of statuses) {
    const jobs = await ctx.db
      .query('generationJobs')
      .withIndex('by_user_status', (q) => q.eq('userId', userId).eq('status', status))
      .order('desc')
      .take(25);

    const match = jobs.find((job) => {
      const pending = job.pendingConceptIds ?? [];
      const knownConcepts = job.conceptIds ?? [];
      return pending.some((id) => id === conceptId) || knownConcepts.some((id) => id === conceptId);
    });

    if (match) {
      return match;
    }
  }

  return null;
}

/**
 * Update concept title.
 * Preserves all FSRS state and other fields.
 */
export const updateConcept = mutation({
  args: {
    conceptId: v.id('concepts'),
    title: v.string(),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const concept = await ctx.db.get(args.conceptId);

    if (!concept || concept.userId !== user._id) {
      throw new Error('Concept not found or unauthorized');
    }

    // Trim and validate title
    const title = args.title.trim();
    if (!title) {
      throw new Error('Title cannot be empty');
    }

    const now = Date.now();
    const patch: Partial<ConceptDoc> = {
      title,
      updatedAt: now,
    };

    await ctx.db.patch(args.conceptId, patch);
  },
});

/**
 * Update phrasing question, answer, explanation, and options.
 * Preserves all stats (attemptCount, correctCount, lastAttemptedAt).
 */
export const updatePhrasing = mutation({
  args: {
    phrasingId: v.id('phrasings'),
    question: v.string(),
    correctAnswer: v.string(),
    explanation: v.optional(v.string()),
    options: v.optional(v.array(v.string())),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const phrasing = await ctx.db.get(args.phrasingId);

    if (!phrasing) {
      throw new Error('Phrasing not found');
    }

    if (phrasing.userId !== user._id) {
      throw new Error('Phrasing not found or unauthorized');
    }

    // Trim and validate required fields
    const question = args.question.trim();
    const correctAnswer = args.correctAnswer.trim();

    if (!question) {
      throw new Error('Question cannot be empty');
    }

    if (!correctAnswer) {
      throw new Error('Correct answer cannot be empty');
    }

    // For MC questions, validate correctAnswer exists in options
    if (args.options && args.options.length > 0) {
      if (!args.options.includes(correctAnswer)) {
        throw new Error('Correct answer must be one of the provided options');
      }
    }

    const now = Date.now();
    const patch: Partial<PhrasingDoc> = {
      question,
      correctAnswer,
      updatedAt: now,
    };

    // Only update optional fields if provided
    if (args.explanation !== undefined) {
      patch.explanation = args.explanation.trim();
    }

    if (args.options !== undefined) {
      patch.options = args.options;
    }

    await ctx.db.patch(args.phrasingId, patch);

    // Return the updated phrasing for immediate UI update
    const updated = await ctx.db.get(args.phrasingId);
    return updated;
  },
});

/**
 * Archive a concept and all its phrasings.
 * Removes it from review queues and stats.
 */
export const archiveConcept = mutation({
  args: {
    conceptId: v.id('concepts'),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const concept = await ctx.db.get(args.conceptId);
    if (!concept || concept.userId !== user._id) {
      throw new Error('Concept not found or unauthorized');
    }
    await archiveConceptDoc(ctx, user._id, concept);
  },
});

/**
 * Unarchive a concept and its phrasings.
 * Returns it to active review.
 */
export const unarchiveConcept = mutation({
  args: { conceptId: v.id('concepts') },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const concept = await ctx.db.get(args.conceptId);
    if (!concept || concept.userId !== user._id) {
      throw new Error('Concept not found or unauthorized');
    }
    await unarchiveConceptDoc(ctx, user._id, concept);
  },
});

/**
 * Soft delete a concept and all its phrasings.
 */
export const softDeleteConcept = mutation({
  args: {
    conceptId: v.id('concepts'),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const concept = await ctx.db.get(args.conceptId);
    if (!concept || concept.userId !== user._id) {
      throw new Error('Concept not found or unauthorized');
    }
    await softDeleteConceptDoc(ctx, user._id, concept);
  },
});

/**
 * Restore a soft-deleted concept and its phrasings.
 */
export const restoreConcept = mutation({
  args: { conceptId: v.id('concepts') },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const concept = await ctx.db.get(args.conceptId);
    if (!concept || concept.userId !== user._id) {
      throw new Error('Concept not found or unauthorized');
    }
    await restoreConceptDoc(ctx, user._id, concept);
  },
});

export const runBulkAction = mutation({
  args: {
    conceptIds: v.array(v.id('concepts')),
    action: v.union(
      v.literal('archive'),
      v.literal('unarchive'),
      v.literal('delete'),
      v.literal('restore')
    ),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const userId = user._id;

    if (args.conceptIds.length === 0) {
      return { processed: 0, skipped: 0 };
    }

    const uniqueIds = new Set<Id<'concepts'>>(args.conceptIds);
    let processed = 0;
    let skipped = 0;

    for (const conceptId of uniqueIds) {
      const concept = await ctx.db.get(conceptId);
      if (!concept || concept.userId !== userId) {
        skipped++;
        continue;
      }

      const applied = await applyConceptBulkAction(ctx, userId, concept, args.action);
      if (applied) {
        processed++;
      } else {
        skipped++;
      }
    }

    return { processed, skipped };
  },
});

async function applyConceptBulkAction(
  ctx: MutationCtx,
  userId: Id<'users'>,
  concept: ConceptDoc,
  action: ConceptBulkAction
) {
  switch (action) {
    case 'archive':
      return archiveConceptDoc(ctx, userId, concept);
    case 'unarchive':
      return unarchiveConceptDoc(ctx, userId, concept);
    case 'delete':
      return softDeleteConceptDoc(ctx, userId, concept);
    case 'restore':
      return restoreConceptDoc(ctx, userId, concept);
    default:
      return false;
  }
}

async function archiveConceptDoc(ctx: MutationCtx, userId: Id<'users'>, concept: ConceptDoc) {
  if (concept.archivedAt) {
    return false;
  }

  const now = Date.now();

  await ctx.db.patch(concept._id, {
    archivedAt: now,
    updatedAt: now,
  });

  // Process all phrasings in batches to prevent unbounded queries (issue #121)
  await updatePhrasingsBatched(
    ctx,
    userId,
    concept._id,
    (q) => q.eq(q.field('archivedAt'), undefined),
    { archivedAt: now, updatedAt: now }
  );

  const deltas = calculateStateTransitionDelta(concept.fsrs.state, undefined);
  await updateStatsCounters(ctx, userId, {
    totalCards: -1,
    ...(deltas ?? {}),
  });

  return true;
}

async function unarchiveConceptDoc(ctx: MutationCtx, userId: Id<'users'>, concept: ConceptDoc) {
  if (!concept.archivedAt) {
    return false;
  }

  const now = Date.now();
  await ctx.db.patch(concept._id, { archivedAt: undefined, updatedAt: now });

  // Process all phrasings in batches to prevent unbounded queries (issue #121)
  await updatePhrasingsBatched(
    ctx,
    userId,
    concept._id,
    (q) => q.neq(q.field('archivedAt'), undefined),
    { archivedAt: undefined, updatedAt: now }
  );

  const deltas = calculateStateTransitionDelta(undefined, concept.fsrs.state ?? 'new');
  await updateStatsCounters(ctx, userId, {
    totalCards: 1,
    ...(deltas ?? {}),
  });

  return true;
}

async function softDeleteConceptDoc(ctx: MutationCtx, userId: Id<'users'>, concept: ConceptDoc) {
  if (concept.deletedAt) {
    return false;
  }

  const now = Date.now();

  await ctx.db.patch(concept._id, {
    deletedAt: now,
    updatedAt: now,
  });

  // Process all phrasings in batches to prevent unbounded queries (issue #121)
  await updatePhrasingsBatched(
    ctx,
    userId,
    concept._id,
    (q) => q.eq(q.field('deletedAt'), undefined),
    { deletedAt: now, updatedAt: now }
  );

  const deltas = calculateStateTransitionDelta(concept.fsrs.state, undefined);
  await updateStatsCounters(ctx, userId, {
    totalCards: -1,
    ...(deltas ?? {}),
  });

  return true;
}

async function restoreConceptDoc(ctx: MutationCtx, userId: Id<'users'>, concept: ConceptDoc) {
  if (!concept.deletedAt) {
    return false;
  }

  const now = Date.now();
  await ctx.db.patch(concept._id, { deletedAt: undefined, updatedAt: now });

  // Process all phrasings in batches to prevent unbounded queries (issue #121)
  await updatePhrasingsBatched(
    ctx,
    userId,
    concept._id,
    (q) => q.neq(q.field('deletedAt'), undefined),
    { deletedAt: undefined, updatedAt: now }
  );

  const deltas = calculateStateTransitionDelta(undefined, concept.fsrs.state ?? 'new');
  await updateStatsCounters(ctx, userId, {
    totalCards: 1,
    ...(deltas ?? {}),
  });

  return true;
}

export const __test = {
  prioritizeConcepts,
  selectActivePhrasing,
};
