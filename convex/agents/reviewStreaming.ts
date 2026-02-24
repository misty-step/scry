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
import { enforceRateLimit } from '../rateLimit';
import { reviewAgent } from './reviewAgent';

function formatDueResult(result: Record<string, unknown> | null): Record<string, unknown> | null {
  if (!result) return null;
  const typed = result as {
    concept: {
      _id: Id<'concepts'>;
      title: string;
      description?: string;
      fsrs: {
        state?: string;
        stability?: number;
        difficulty?: number;
        lapses?: number;
        reps?: number;
      };
    };
    phrasing: {
      _id: Id<'phrasings'>;
      question: string;
      type?: string;
      options?: string[];
    };
    retrievability?: number;
    interactions: Array<{ isCorrect: boolean }>;
  };

  return {
    conceptId: typed.concept._id,
    conceptTitle: typed.concept.title,
    conceptDescription: typed.concept.description ?? '',
    fsrsState: typed.concept.fsrs.state ?? 'new',
    stability: typed.concept.fsrs.stability,
    difficulty: typed.concept.fsrs.difficulty,
    lapses: typed.concept.fsrs.lapses ?? 0,
    reps: typed.concept.fsrs.reps ?? 0,
    retrievability: typed.retrievability,
    phrasingId: typed.phrasing._id,
    question: typed.phrasing.question,
    type: typed.phrasing.type ?? 'multiple-choice',
    options: typed.phrasing.options ?? [],
    recentAttempts: typed.interactions.length,
    recentCorrect: typed.interactions.filter((i) => i.isCorrect).length,
  };
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
    const isCorrect = args.userAnswer.trim().toLowerCase() === correctAnswer.trim().toLowerCase();

    const result = await ctx.runMutation(internal.concepts.recordInteractionInternal, {
      userId: user._id,
      conceptId: args.conceptId,
      phrasingId: args.phrasingId,
      userAnswer: args.userAnswer,
      isCorrect,
    });

    return {
      isCorrect,
      userAnswer: args.userAnswer,
      correctAnswer,
      explanation: phrasing.explanation ?? '',
      conceptTitle: args.conceptTitle ?? '',
      nextReview: result.nextReview,
      scheduledDays: result.scheduledDays,
      newState: result.newState,
      totalAttempts: (args.recentAttempts ?? 0) + 1,
      totalCorrect: (args.recentCorrect ?? 0) + (isCorrect ? 1 : 0),
      lapses: args.lapses ?? 0,
      reps: (args.reps ?? 0) + 1,
    };
  },
});

// Send a message and trigger async streaming response
export const sendMessage = mutation({
  args: {
    threadId: v.string(),
    prompt: v.string(),
  },
  handler: async (ctx, { threadId, prompt }) => {
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
    });
    return { messageId };
  },
});

// Auto-start: agent fetches first concept without a visible user message
export const startReviewSession = internalAction({
  args: { threadId: v.string() },
  handler: async (ctx, { threadId }) => {
    const result = await reviewAgent.streamText(
      ctx,
      { threadId },
      { prompt: 'Start my review session. Fetch the first concept and present it.' },
      { saveStreamDeltas: { chunking: 'word', throttleMs: 100 } }
    );
    await result.consumeStream();
  },
});

// Internal action that streams the agent's response
export const streamResponse = internalAction({
  args: {
    threadId: v.string(),
    promptMessageId: v.string(),
  },
  handler: async (ctx, { threadId, promptMessageId }) => {
    const result = await reviewAgent.streamText(
      ctx,
      { threadId },
      { promptMessageId },
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
