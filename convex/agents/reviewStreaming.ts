import { createThread, listUIMessages, syncStreams, vStreamArgs } from '@convex-dev/agent';
import { paginationOptsValidator } from 'convex/server';
import { v } from 'convex/values';
import { components, internal } from '../_generated/api';
import { action, internalAction, mutation, query } from '../_generated/server';
import { requireUserFromClerk } from '../clerk';
import { reviewAgent } from './reviewAgent';

// Create a new review thread
export const createReviewThread = mutation({
  args: {},
  handler: async (ctx) => {
    await requireUserFromClerk(ctx);
    const threadId = await createThread(ctx, components.agent);
    return { threadId };
  },
});

// Send a message and trigger async streaming response (recommended pattern)
export const sendMessage = mutation({
  args: {
    threadId: v.string(),
    prompt: v.string(),
  },
  handler: async (ctx, { threadId, prompt }) => {
    await requireUserFromClerk(ctx);
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
    await requireUserFromClerk(ctx);
    const streams = await syncStreams(ctx, components.agent, {
      threadId: args.threadId,
      streamArgs: args.streamArgs,
    });
    const paginated = await listUIMessages(ctx, components.agent, args);
    return { ...paginated, streams };
  },
});

// One-shot action for starting a session (simpler, no optimistic updates)
export const startSession = action({
  args: { threadId: v.string() },
  handler: async (ctx, { threadId }) => {
    await reviewAgent.streamText(
      ctx,
      { threadId },
      { prompt: 'Start my review session. Fetch the first concept and present it.' },
      { saveStreamDeltas: true }
    );
  },
});
