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
import { internalAction, mutation, query } from '../_generated/server';
import { requireUserFromClerk } from '../clerk';
import { calculateConceptStatsDelta } from '../lib/conceptFsrsHelpers';
import { updateStatsCounters } from '../lib/userStatsHelpers';
import { enforceRateLimit } from '../rateLimit';
import { reviewAgent } from './reviewAgent';
import { buildSubmitAnswerPayload, formatDueResult, gradeAnswer } from './reviewToolHelpers';

const CHAT_INTENT_VALUES = ['general', 'explain', 'stats'] as const;
type ChatIntent = (typeof CHAT_INTENT_VALUES)[number];
const CHAT_INTENT_VALIDATOR = v.union(
  v.literal('general'),
  v.literal('explain'),
  v.literal('stats')
);

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

export const fetchNextQuestion = mutation({
  args: { threadId: v.string() },
  handler: async (ctx, { threadId }) => {
    const user = await requireUserFromClerk(ctx);
    const thread = await getThreadMetadata(ctx, components.agent, { threadId });
    if (thread.userId !== user._id) throw new Error('Unauthorized');

    const due = await ctx.runQuery(internal.concepts.getDueInternal, { userId: user._id });
    return formatDueResult(due as unknown as Record<string, unknown> | null);
  },
});

export const submitAnswerDirect = mutation({
  args: {
    threadId: v.string(),
    conceptId: v.id('concepts'),
    phrasingId: v.id('phrasings'),
    userAnswer: v.string(),
    conceptTitle: v.optional(v.string()),
    conceptDescription: v.optional(v.string()),
    recentAttempts: v.optional(v.number()),
    recentCorrect: v.optional(v.number()),
    lapses: v.optional(v.number()),
    reps: v.optional(v.number()),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    await enforceRateLimit(ctx, user._id.toString(), 'recordInteraction', false);

    const thread = await getThreadMetadata(ctx, components.agent, { threadId: args.threadId });
    if (thread.userId !== user._id) throw new Error('Unauthorized');

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

export const getWeakAreasDirect = mutation({
  args: {
    threadId: v.string(),
    limit: v.optional(v.number()),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    await enforceRateLimit(ctx, user._id.toString(), 'default', false);

    const thread = await getThreadMetadata(ctx, components.agent, { threadId: args.threadId });
    if (thread.userId !== user._id) throw new Error('Unauthorized');

    const now = Date.now();
    const limit = Math.max(1, Math.min(8, args.limit ?? 5));

    const concepts = await ctx.db
      .query('concepts')
      .withIndex('by_user_next_review', (q) =>
        q.eq('userId', user._id).eq('deletedAt', undefined).eq('archivedAt', undefined)
      )
      .take(200); // Bounded candidate set for in-memory weak-area ranking

    const ranked = concepts
      .filter((concept) => concept.phrasingCount > 0)
      .map((concept) => {
        const reps = concept.fsrs.reps ?? 0;
        const lapses = concept.fsrs.lapses ?? 0;
        const state = concept.fsrs.state ?? 'new';
        const lapseRate = reps > 0 ? lapses / reps : 0;
        const dueSoonWeight = concept.fsrs.nextReview <= now ? 1.25 : 0;
        const priority =
          lapses * 2.5 +
          lapseRate * 3 +
          getStatePriority(state) +
          dueSoonWeight +
          Math.min(reps, 10) * 0.05;

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
  args: {
    threadId: v.string(),
    conceptId: v.id('concepts'),
    days: v.optional(v.number()),
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    await enforceRateLimit(ctx, user._id.toString(), 'default', false);

    const thread = await getThreadMetadata(ctx, components.agent, { threadId: args.threadId });
    if (thread.userId !== user._id) throw new Error('Unauthorized');

    const concept = await ctx.db.get(args.conceptId);
    if (!concept || concept.userId !== user._id || concept.deletedAt || concept.archivedAt) {
      throw new Error('Concept not found');
    }

    const nowMs = Date.now();
    const days = Math.max(1, Math.min(30, Math.round(args.days ?? 1)));
    const nextReview = nowMs + days * 24 * 60 * 60 * 1000;

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
    threadId: v.string(),
    prompt: v.string(),
    intent: v.optional(CHAT_INTENT_VALIDATOR),
  },
  handler: async (ctx, { threadId, prompt, intent }) => {
    const user = await requireUserFromClerk(ctx);
    await enforceRateLimit(ctx, user._id.toString(), 'default', false);
    const thread = await getThreadMetadata(ctx, components.agent, { threadId });
    if (thread.userId !== user._id) throw new Error('Unauthorized');
    const { messageId } = await reviewAgent.saveMessage(ctx, {
      threadId,
      prompt,
      skipEmbeddings: true,
    });
    await ctx.scheduler.runAfter(0, internal.agents.reviewStreaming.streamResponse, {
      threadId,
      promptMessageId: messageId,
      intent: CHAT_INTENT_VALUES.includes((intent ?? 'general') as ChatIntent)
        ? (intent ?? 'general')
        : 'general',
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
    const resolvedIntent: ChatIntent = CHAT_INTENT_VALUES.includes(
      (intent ?? 'general') as ChatIntent
    )
      ? (intent ?? 'general')
      : 'general';
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
    threadId: v.string(),
    paginationOpts: paginationOptsValidator,
    streamArgs: vStreamArgs,
  },
  handler: async (ctx, args) => {
    const user = await requireUserFromClerk(ctx);
    const thread = await getThreadMetadata(ctx, components.agent, { threadId: args.threadId });
    if (thread.userId !== user._id) throw new Error('Unauthorized');
    const streams = await syncStreams(ctx, components.agent, {
      threadId: args.threadId,
      streamArgs: args.streamArgs,
    });
    const paginated = await listUIMessages(ctx, components.agent, args);
    return { ...paginated, streams };
  },
});
