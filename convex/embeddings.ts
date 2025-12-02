/**
 * Embeddings Service (concepts & phrasings only)
 *
 * Provides:
 * - generateEmbedding(text): action to get 768-dim vector from Google text-embedding-004
 * - syncMissingEmbeddings(): internal mutation used by cron to backfill concepts/phrasings
 * - Helpers to fetch/save concept and phrasing embeddings
 *
 * Question tables were removed; this module no longer touches questionIds.
 */

import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { embed } from 'ai';
import { v } from 'convex/values';
import { internal } from './_generated/api';
import type { Doc, Id } from './_generated/dataModel';
import { action, internalAction, internalMutation, internalQuery } from './_generated/server';
import { createConceptsLogger, type LogContext } from './lib/logger';

const conceptsLogger = createConceptsLogger({ module: 'embeddings' });

const logger = {
  info(context: LogContext = {}, message = '') {
    conceptsLogger.info(message, context);
  },
  warn(context: LogContext = {}, message = '') {
    conceptsLogger.warn(message, context);
  },
  error(context: LogContext = {}, message = '') {
    conceptsLogger.error(message, context);
  },
};

const EMBEDDING_SYNC_CONFIG = {
  ttlMs: 1000 * 60 * 60 * 6, // 6 hours
  conceptLimit: 40,
  phrasingLimit: 80,
  batchSize: 10,
  batchDelayMs: 1000,
} as const;

export function enforcePerUserLimit<T extends { userId: Id<'users'> }>(
  items: T[],
  perUserLimit: number
): T[] {
  if (perUserLimit <= 0) {
    return [];
  }

  const seenCounts = new Map<string, number>();
  const result: T[] = [];

  for (const item of items) {
    const key = item.userId.toString();
    const current = seenCounts.get(key) ?? 0;
    if (current >= perUserLimit) {
      continue;
    }
    seenCounts.set(key, current + 1);
    result.push(item);
  }

  return result;
}

type SecretDiagnostics = {
  present: boolean;
  length: number;
  fingerprint: string | null;
};

export function getSecretDiagnostics(value: string | undefined): SecretDiagnostics {
  if (!value) {
    return { present: false, length: 0, fingerprint: null };
  }
  let hash = 0x811c9dc5;
  for (let i = 0; i < value.length; i += 1) {
    hash ^= value.charCodeAt(i);
    hash = (hash * 0x01000193) >>> 0;
  }
  return { present: true, length: value.length, fingerprint: hash.toString(16).slice(0, 8) };
}

export const generateEmbedding = internalAction({
  args: { text: v.string() },
  handler: async (_ctx, args): Promise<number[]> => {
    const startTime = Date.now();
    const apiKey = process.env.GOOGLE_AI_API_KEY;
    const keyDiagnostics = getSecretDiagnostics(apiKey);

    if (!apiKey) {
      logger.error(
        { event: 'embeddings.generation.missing-key', keyDiagnostics },
        'GOOGLE_AI_API_KEY not configured in Convex environment'
      );
      throw new Error('GOOGLE_AI_API_KEY not configured');
    }

    const google = createGoogleGenerativeAI({ apiKey });

    try {
      const { embedding } = await embed({
        model: google.textEmbedding('text-embedding-004'),
        value: args.text,
      });

      logger.info(
        {
          event: 'embeddings.generation.success',
          duration: Date.now() - startTime,
          dimensions: embedding.length,
          textLength: args.text.length,
          model: 'text-embedding-004',
        },
        'Successfully generated embedding'
      );

      return embedding;
    } catch (error) {
      const err = error as Error;
      logger.error(
        {
          event: 'embeddings.generation.failure',
          duration: Date.now() - startTime,
          errorMessage: err.message,
          stack: err.stack,
        },
        'Failed to generate embedding'
      );
      throw err;
    }
  },
});

export const getConceptsWithoutEmbeddings = internalQuery({
  args: { limit: v.optional(v.number()) },
  handler: async (ctx, args) => {
    const limit = Math.min(Math.max(args.limit ?? EMBEDDING_SYNC_CONFIG.conceptLimit, 1), 200);
    return await ctx.db
      .query('concepts')
      .filter((q) => q.eq(q.field('embedding'), undefined))
      .take(limit);
  },
});

export const getPhrasingsWithoutEmbeddings = internalQuery({
  args: { limit: v.optional(v.number()) },
  handler: async (ctx, args) => {
    const limit = Math.min(Math.max(args.limit ?? EMBEDDING_SYNC_CONFIG.phrasingLimit, 1), 200);
    return await ctx.db
      .query('phrasings')
      .filter((q) => q.eq(q.field('embedding'), undefined))
      .take(limit);
  },
});

export const saveConceptEmbedding = internalMutation({
  args: {
    conceptId: v.id('concepts'),
    embedding: v.array(v.float64()),
    embeddingGeneratedAt: v.number(),
  },
  handler: async (ctx, args) => {
    const concept = await ctx.db.get(args.conceptId);
    if (!concept) return;
    await ctx.db.patch(args.conceptId, {
      embedding: args.embedding,
      embeddingGeneratedAt: args.embeddingGeneratedAt,
      updatedAt: Date.now(),
    });
  },
});

export const savePhrasingEmbedding = internalMutation({
  args: {
    phrasingId: v.id('phrasings'),
    embedding: v.array(v.float64()),
    embeddingGeneratedAt: v.number(),
  },
  handler: async (ctx, args) => {
    const phrasing = await ctx.db.get(args.phrasingId);
    if (!phrasing) return;
    await ctx.db.patch(args.phrasingId, {
      embedding: args.embedding,
      embeddingGeneratedAt: args.embeddingGeneratedAt,
      updatedAt: Date.now(),
    });
  },
});

export const countConceptsWithoutEmbeddings = internalQuery({
  args: { cutoff: v.number() },
  handler: async (ctx, args) => {
    const items = await ctx.db
      .query('concepts')
      .filter((q) =>
        q.and(q.eq(q.field('embedding'), undefined), q.gt(q.field('createdAt'), args.cutoff))
      )
      .take(1000);
    return items.length;
  },
});

export const countPhrasingsWithoutEmbeddings = internalQuery({
  args: { cutoff: v.number() },
  handler: async (ctx, args) => {
    const items = await ctx.db
      .query('phrasings')
      .filter((q) =>
        q.and(q.eq(q.field('embedding'), undefined), q.gt(q.field('createdAt'), args.cutoff))
      )
      .take(1000);
    return items.length;
  },
});

export const syncMissingEmbeddings = internalAction({
  args: {},
  handler: async (ctx) => {
    const now = Date.now();
    const concepts: Doc<'concepts'>[] = await ctx.runQuery(
      internal.embeddings.getConceptsWithoutEmbeddings,
      {
        limit: EMBEDDING_SYNC_CONFIG.conceptLimit,
      }
    );
    const phrasings: Doc<'phrasings'>[] = await ctx.runQuery(
      internal.embeddings.getPhrasingsWithoutEmbeddings,
      {
        limit: EMBEDDING_SYNC_CONFIG.phrasingLimit,
      }
    );

    const workItems: Array<
      | { kind: 'concept'; id: Id<'concepts'>; text: string }
      | { kind: 'phrasing'; id: Id<'phrasings'>; text: string }
    > = [
      ...concepts.map((c) => ({
        kind: 'concept' as const,
        id: c._id,
        text: `${c.title}\n\n${c.description ?? ''}`.trim(),
      })),
      ...phrasings.map((p) => ({
        kind: 'phrasing' as const,
        id: p._id,
        text: `${p.question}\n\n${p.explanation ?? ''}`.trim(),
      })),
    ];

    for (const item of workItems) {
      try {
        const embedding = await ctx.runAction(internal.embeddings.generateEmbedding, {
          text: item.text,
        });

        if (item.kind === 'concept') {
          await ctx.runMutation(internal.embeddings.saveConceptEmbedding, {
            conceptId: item.id,
            embedding,
            embeddingGeneratedAt: now,
          });
        } else {
          await ctx.runMutation(internal.embeddings.savePhrasingEmbedding, {
            phrasingId: item.id,
            embedding,
            embeddingGeneratedAt: now,
          });
        }
      } catch (error) {
        logger.warn(
          {
            event: 'embeddings.sync.item-failure',
            kind: item.kind,
            id: item.id,
            error: (error as Error).message,
          },
          'Failed to generate/save embedding'
        );
      }
    }

    return {
      processedConcepts: concepts.length,
      processedPhrasings: phrasings.length,
      durationMs: Date.now() - now,
    };
  },
});

export const getAuthenticatedUserId = action({
  args: {},
  handler: async (ctx) => {
    const identity = await ctx.auth.getUserIdentity();
    if (!identity) {
      throw new Error('Unauthenticated');
    }
    return identity.subject;
  },
});
