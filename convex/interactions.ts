import type { Id } from './_generated/dataModel';
import type { MutationCtx } from './_generated/server';
import { scheduleConceptReview } from './fsrs';
import { calculateConceptStatsDelta } from './lib/conceptFsrsHelpers';
import { buildInteractionContext } from './lib/interactionContext';
import { updateStatsCounters } from './lib/userStatsHelpers';

export type RecordInteractionArgs = {
  conceptId: Id<'concepts'>;
  phrasingId: Id<'phrasings'>;
  userAnswer: string;
  isCorrect: boolean;
  timeSpent?: number;
  sessionId?: string;
};

/**
 * Core interaction recording pipeline extracted from `concepts.recordInteraction`.
 *
 * Invariants:
 * - Authorization checks happen before any write.
 * - FSRS scheduling and state transition decisions remain colocated here.
 * - Side effects preserve existing ordering: interaction insert → phrasing patch → concept patch → stats.
 */
export async function recordInteractionCore(
  ctx: MutationCtx,
  userId: Id<'users'>,
  args: RecordInteractionArgs
) {
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
    totalAttempts: (phrasing.attemptCount ?? 0) + 1,
    totalCorrect: (phrasing.correctCount ?? 0) + (args.isCorrect ? 1 : 0),
    lapses: scheduleResult.fsrs.lapses ?? 0,
    reps: scheduleResult.fsrs.reps ?? 0,
  };
}
