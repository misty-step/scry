import {
  createThread,
  getThreadMetadata,
  listUIMessages,
  syncStreams,
  vStreamArgs,
} from '@convex-dev/agent';
import { paginationOptsValidator } from 'convex/server';
import { v } from 'convex/values';
import { components, internal } from '../_generated/api';
import type { Id } from '../_generated/dataModel';
import { internalAction, mutation, query, type MutationCtx, type QueryCtx } from '../_generated/server';
import { requireUserFromClerk } from '../clerk';
import { calculateConceptStatsDelta } from '../lib/conceptFsrsHelpers';
import { updateStatsCounters } from '../lib/userStatsHelpers';
import { enforceRateLimit } from '../rateLimit';
import { reviewAgent } from './reviewAgent';
import {
  assertChatPromptLength,
  assertUserAnswerLength,
  buildSubmitAnswerPayload,
  formatDueResult,
  gradeAnswer,
} from './reviewToolHelpers';
import * as dtos from './dtos';

/**
 * Validates that the current user owns the specified thread.
 * Returns the thread metadata if valid, otherwise throws an Unauthorized error.
 */
async function requireThreadOwnership(
  ctx: QueryCtx | MutationCtx,
  userId: Id<'users'>,
  threadId: string
) {
  let thread;
  try {
    thread = await getThreadMetadata(ctx, components.agent, { threadId });
  } catch (error) {
    if (error instanceof Error && error.message === 'Thread not found') {
      throw new Error('Unauthorized');
    }
    throw error;
  }
  if (thread.userId !== userId) {
    throw new Error('Unauthorized');
  }
  return thread;
}

const CHAT_INTENT_VALUES = ['general', 'explain', 'stats'] as const;
type ChatIntent = (typeof CHAT_INTENT_VALUES)[number];
const CHAT_INTENT_VALIDATOR = v.union(
  v.literal('general'),
  v.literal('explain'),
  v.literal('stats')
);

function resolveIntent(intent?: string): ChatIntent {
  return CHAT_INTENT_VALUES.includes((intent ?? 'general') as ChatIntent)
    ? ((intent ?? 'general') as ChatIntent)
    : 'general';
}

const WEAK_AREA_SAMPLE_SIZE = 200;
const WEAK_AREA_LAPSES_WEIGHT = 2.5;
const WEAK_AREA_LAPSE_RATE_WEIGHT = 3;
const WEAK_AREA_DUE_NOW_WEIGHT = 1.25;
const WEAK_AREA_REPS_BONUS_CAP = 10;
const WEAK_AREA_REPS_BONUS_WEIGHT = 0.05;
const RESCHEDULE_MIN_DAYS = 1;
const RESCHEDULE_MAX_DAYS = 30;
const MS_PER_DAY = 24 * 60 * 60 * 1000;

function getStatePriority(state?: string) {
  if (state === 'relearning') return 4;
  if (state === 'learning') return 3;
  if (state === 'review') return 2;
  return 1;
}

// Create a new review thread (no auto-start model call; quiz flow is deterministic)
export const createReviewThread = mutation({
  args: {},
  handler: async (ctx) => {
    const user = await requireUserFromClerk(ctx);
    await enforceRateLimit(ctx, user._id.toString(), 'default', false);
    const threadId = await createThread(ctx, components.agent, { userId: user._id });
    return { threadId };
  },
});

// Intentionally mutation-based: this is an imperative, one-shot UI action with
// thread ownership checks (non-reactive fetch, not a subscribed query surface).
export const fetchNextQuestion = mutation({
  args: dtos.fetchNextQuestionArgs,
  handler: async (ctx, { threadId }) => {
    const user = await requireUserFromClerk(ctx);
    await requireThreadOwnership(ctx, user._id, threadId);

    const due = await ctx.runQuery(internal.concepts.getDueInternal, { userId: user._id });
    return formatDueResult(due as unknown as Record<string, unknown> | null);
  },
});

export const submitAnswerDirect = mutation({
  args: dtos.submitAnswerDirectArgs,
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    await enforceRateLimit(ctx, user._id.toString(), 'recordInteraction', false);
    assertUserAnswerLength(args.userAnswer);
    await requireThreadOwnership(ctx, user._id, args.threadId);

    const phrasing = await ctx.runQuery(internal.phrasings.getPhrasingInternal, {
      userId: user._id,
      phrasingId: args.phrasingId,
    });
    if (!phrasing) throw new Error('Phrasing not found');

    const correctAnswer = phrasing.correctAnswer ?? '';
    const isCorrect = gradeAnswer(args.userAnswer, correctAnswer);

    const result = await ctx.runMutation(internal.concepts.recordInteractionInternal, {
      userId: user._id,
      conceptId: args.conceptId,
      phrasingId: args.phrasingId,
      userAnswer: args.userAnswer,
      isCorrect,
    });

    return buildSubmitAnswerPayload({
      result: result as {
        conceptId?: Id<'concepts'>;
        nextReview: number;
        scheduledDays: number;
        newState: string;
        totalAttempts: number;
        totalCorrect: number;
        lapses: number;
        reps: number;
      },
      userAnswer: args.userAnswer,
      correctAnswer,
      isCorrect,
      explanation: phrasing.explanation ?? '',
      conceptTitle: args.conceptTitle,
      conceptDescription: args.conceptDescription,
    });
  },
});

// Intentionally mutation-based: invoked on demand from UI and rate-limited as an
// action endpoint (not a reactive query that auto-re-runs on document changes).
export const getWeakAreasDirect = mutation({
  args: dtos.getWeakAreasDirectArgs,
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    await enforceRateLimit(ctx, user._id.toString(), 'default', false);
    await requireThreadOwnership(ctx, user._id, args.threadId);

    const now = Date.now();
    const limit = Math.max(1, Math.min(8, args.limit ?? 5));

    const concepts = await ctx.db
      .query('concepts')
      .withIndex('by_user_next_review', (q) =>
        q.eq('userId', user._id).eq('deletedAt', undefined).eq('archivedAt', undefined)
      )
      // Intentional due-first snapshot: bounded sample for low-latency ranking UI.
      // This is a triage hint list, not a full global weak-area scan.
      .take(WEAK_AREA_SAMPLE_SIZE);

    const ranked = concepts
      .filter((concept) => concept.phrasingCount > 0)
      .map((concept) => {
        const reps = concept.fsrs.reps ?? 0;
        const lapses = concept.fsrs.lapses ?? 0;
        const state = concept.fsrs.state ?? 'new';
        const lapseRate = reps > 0 ? lapses / reps : 0;
        const dueNowWeight = concept.fsrs.nextReview <= now ? WEAK_AREA_DUE_NOW_WEIGHT : 0;
        // UI-only weak-area heuristic (ranking hints only). This does NOT change FSRS
        // scheduling/interval calculations, which remain exclusively in recordInteractionInternal.
        const priority =
          lapses * WEAK_AREA_LAPSES_WEIGHT +
          lapseRate * WEAK_AREA_LAPSE_RATE_WEIGHT +
          getStatePriority(state) +
          dueNowWeight +
          Math.min(reps, WEAK_AREA_REPS_BONUS_CAP) * WEAK_AREA_REPS_BONUS_WEIGHT;

        return {
          conceptId: concept._id,
          title: concept.title,
          state,
          lapses,
          reps,
          lapseRate,
          nextReview: concept.fsrs.nextReview,
          dueNow: concept.fsrs.nextReview <= now,
          priority,
        };
      })
      .sort((a, b) => b.priority - a.priority)
      .slice(0, limit);

    return {
      generatedAt: now,
      itemCount: ranked.length,
      items: ranked,
    };
  },
});

export const rescheduleConceptDirect = mutation({
  args: dtos.rescheduleConceptDirectArgs,
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    await enforceRateLimit(ctx, user._id.toString(), 'default', false);
    await requireThreadOwnership(ctx, user._id, args.threadId);

    const concept = await ctx.db.get(args.conceptId);
    if (!concept || concept.userId !== user._id || concept.deletedAt || concept.archivedAt) {
      throw new Error('Concept not found');
    }

    const nowMs = Date.now();
    const requestedDays =
      typeof args.days === 'number' && Number.isFinite(args.days) ? args.days : RESCHEDULE_MIN_DAYS;
    const days = Math.max(
      RESCHEDULE_MIN_DAYS,
      Math.min(RESCHEDULE_MAX_DAYS, Math.round(requestedDays))
    );
    const nextReview = nowMs + days * MS_PER_DAY;

    await ctx.db.patch(concept._id, {
      fsrs: {
        ...concept.fsrs,
        nextReview,
        scheduledDays: days,
      },
      updatedAt: nowMs,
    });

    const statsDelta = calculateConceptStatsDelta({
      // Manual postpone changes timing only; FSRS state is intentionally unchanged.
      oldState: concept.fsrs.state ?? 'new',
      newState: concept.fsrs.state ?? 'new',
      oldNextReview: concept.fsrs.nextReview,
      newNextReview: nextReview,
      nowMs,
    });

    if (statsDelta) {
      await updateStatsCounters(ctx, user._id, statsDelta);
    }

    return {
      conceptId: concept._id,
      conceptTitle: concept.title,
      nextReview,
      scheduledDays: days,
      newState: concept.fsrs.state ?? 'new',
      reps: concept.fsrs.reps ?? 0,
      lapses: concept.fsrs.lapses ?? 0,
    };
  },
});

// Send a message and trigger async streaming response
export const sendMessage = mutation({
  args: {
    ...dtos.sendMessageArgs,
    intent: v.optional(CHAT_INTENT_VALIDATOR),
  },
  handler: async (ctx, { threadId, prompt, intent }) => {
    const user = await requireUserFromClerk(ctx);
    await enforceRateLimit(ctx, user._id.toString(), 'default', false);
    assertChatPromptLength(prompt);
    await requireThreadOwnership(ctx, user._id, threadId);
    const { messageId } = await reviewAgent.saveMessage(ctx, {
      threadId,
      prompt,
      skipEmbeddings: true,
    });
    await ctx.scheduler.runAfter(0, internal.agents.reviewStreaming.streamResponse, {
      threadId,
      promptMessageId: messageId,
      intent: resolveIntent(intent),
    });
    return { messageId };
  },
});

// Internal action that streams the agent's response
export const streamResponse = internalAction({
  args: {
    threadId: v.string(),
    promptMessageId: v.string(),
    intent: v.optional(CHAT_INTENT_VALIDATOR),
  },
  handler: async (ctx, { threadId, promptMessageId, intent }) => {
    const resolvedIntent = resolveIntent(intent);
    const toolChoice = resolvedIntent === 'explain' ? 'none' : 'auto';
    const result = await reviewAgent.streamText(
      ctx,
      { threadId },
      { promptMessageId, toolChoice },
      { saveStreamDeltas: { chunking: 'word', throttleMs: 100 } }
    );
    await result.consumeStream();
  },
});

// Query for listing messages with streaming support
export const listMessages = query({
  args: {
    ...dtos.listMessagesArgs,
    paginationOpts: paginationOptsValidator,
    streamArgs: vStreamArgs,
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    await requireThreadOwnership(ctx, user._id, args.threadId);
    const streams = await syncStreams(ctx, components.agent, {
      threadId: args.threadId,
      streamArgs: args.streamArgs,
    });
    const paginated = await listUIMessages(ctx, components.agent, args);
    return { ...paginated, streams };
  },
});
