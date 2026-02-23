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
import { internalAction, mutation, query } from '../_generated/server';
import { requireUserFromClerk } from '../clerk';
import { enforceRateLimit } from '../rateLimit';
import { reviewAgent } from './reviewAgent';

// Create a new review thread and auto-start the session
export const createReviewThread = mutation({
  args: {},
  handler: async (ctx) => {
    const user = await requireUserFromClerk(ctx);
    await enforceRateLimit(ctx, user._id.toString(), 'default', false);
    const threadId = await createThread(ctx, components.agent, { userId: user._id });
    await ctx.scheduler.runAfter(0, internal.agents.reviewStreaming.startReviewSession, {
      threadId,
    });
    return { threadId };
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
