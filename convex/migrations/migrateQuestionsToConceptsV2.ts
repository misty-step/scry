/**
 * Migration V2: Questions → Concepts/Phrasings
 *
 * One-time migration to transform orphaned questions into the concepts/phrasings system.
 * Uses semantic clustering to group related questions, synthesizes unified concept titles,
 * and preserves all FSRS state and interaction history.
 *
 * SAFETY: Supports dry-run mode for validation before actual migration.
 * ATOMICITY: Questions remain untouched (only conceptId field added).
 * ROLLBACK: Can delete concepts/phrasings and clear conceptId fields if needed.
 */

import { v } from 'convex/values';
import { internal } from '../_generated/api';
import { internalAction, internalMutation, query } from '../_generated/server';
import { clusterQuestionsBySimilarity } from './clusterQuestions';
import { synthesizeConceptFromQuestions } from './synthesizeConcept';

/**
 * Migrate orphaned questions to concepts/phrasings system
 *
 * Process:
 * 1. Query all questions without conceptId (orphaned questions)
 * 2. Cluster questions by semantic similarity (0.85 threshold)
 * 3. Synthesize concept title/description for each cluster via GPT-5-mini
 * 4. Create concepts + phrasings, link questions to concepts
 * 5. Preserve all FSRS state from most-reviewed question in cluster
 *
 * @param dryRun - If true, logs intended actions without mutations (default: true)
 * @returns Migration statistics
 */
export const migrateQuestionsToConceptsV2 = internalAction({
  args: {
    dryRun: v.optional(v.boolean()),
  },
  handler: async (ctx, args) => {
    const dryRun = args.dryRun ?? true;

    console.warn(`[Migration V2] Starting with dryRun=${dryRun}`);
    console.warn(`[Migration V2] Deployment: ${process.env.CONVEX_CLOUD_URL || 'local'}`);

    // 1. Find orphaned questions (no conceptId)
    const orphanedQuestions = await ctx.runQuery(internal.migrations.getOrphanedQuestions);

    console.warn(`[Migration V2] Found ${orphanedQuestions.length} orphaned questions`);

    if (orphanedQuestions.length === 0) {
      console.warn('[Migration V2] No orphaned questions. Migration complete.');
      return {
        clustersFormed: 0,
        conceptsCreated: 0,
        phrasingsCreated: 0,
        questionsLinked: 0,
      };
    }

    // 2. Cluster by semantic similarity
    console.warn('[Migration V2] Clustering questions by similarity...');
    const clusters = await clusterQuestionsBySimilarity(ctx, orphanedQuestions);

    console.warn(`[Migration V2] Formed ${clusters.length} clusters`);
    console.warn(
      `[Migration V2] Cluster sizes: ${clusters
        .map((c) => c.questions.length)
        .sort((a, b) => b - a)
        .slice(0, 10)
        .join(', ')}${clusters.length > 10 ? '...' : ''}`
    );

    const stats = {
      clustersFormed: clusters.length,
      conceptsCreated: 0,
      phrasingsCreated: 0,
      questionsLinked: 0,
    };

    // 3. Process each cluster
    for (let i = 0; i < clusters.length; i++) {
      const cluster = clusters[i];

      // Synthesize concept title/description
      const conceptData = await synthesizeConceptFromQuestions(ctx, cluster.questions);

      if (dryRun) {
        console.warn(
          `[DRY RUN] Cluster ${i + 1}/${clusters.length}: ` +
            `Would create concept "${conceptData.title}" ` +
            `with ${cluster.questions.length} phrasing(s) ` +
            `(avg similarity: ${cluster.avgSimilarity.toFixed(2)})`
        );
        continue;
      }

      // Create concept + phrasings + links (single mutation, all DB ops)
      await ctx.runMutation(internal.migrations.createConceptFromCluster, {
        questions: cluster.questions,
        title: conceptData.title,
        description: conceptData.description,
      });

      stats.conceptsCreated++;
      stats.phrasingsCreated += cluster.questions.length;
      stats.questionsLinked += cluster.questions.length;

      console.warn(
        `[Migration V2] ${i + 1}/${clusters.length}: ` +
          `Created concept "${conceptData.title}" ` +
          `with ${cluster.questions.length} phrasing(s)`
      );
    }

    console.warn('[Migration V2] Complete:', stats);
    return stats;
  },
});

/**
 * Helper query: Get all orphaned questions (no conceptId)
 */
export const getOrphanedQuestions = query({
  args: {},
  handler: async (ctx) => {
    const allQuestions = await ctx.db.query('questions').collect();
    return allQuestions.filter((q) => q.conceptId === undefined);
  },
});

/**
 * Helper mutation: Create concept and phrasings from cluster
 *
 * Single atomic mutation that creates concept, all phrasings, and links questions.
 * Uses FSRS state from most-reviewed question in cluster.
 */
export const createConceptFromCluster = internalMutation({
  args: {
    questions: v.array(v.any()), // Doc<'questions'>[] passed from action
    title: v.string(),
    description: v.string(),
  },
  handler: async (ctx, args) => {
    // Find most-reviewed question (highest reps) for FSRS state
    const mostReviewedQuestion = args.questions.reduce(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (best: any, curr: any) => ((curr.reps ?? 0) > (best.reps ?? 0) ? curr : best),
      args.questions[0]
    );

    // Create concept
    const conceptId = await ctx.db.insert('concepts', {
      userId: mostReviewedQuestion.userId,
      title: args.title,
      description: args.description,
      fsrs: {
        stability: mostReviewedQuestion.stability ?? 1,
        difficulty: mostReviewedQuestion.fsrsDifficulty ?? 5,
        lastReview: mostReviewedQuestion.lastReview,
        nextReview: mostReviewedQuestion.nextReview ?? Date.now(),
        elapsedDays: mostReviewedQuestion.elapsedDays,
        retrievability: mostReviewedQuestion.retrievability,
        scheduledDays: mostReviewedQuestion.scheduledDays,
        reps: mostReviewedQuestion.reps,
        lapses: mostReviewedQuestion.lapses,
        state: mostReviewedQuestion.state ?? 'new',
      },
      phrasingCount: args.questions.length,
      embedding: mostReviewedQuestion.embedding,
      embeddingGeneratedAt: mostReviewedQuestion.embeddingGeneratedAt,
      createdAt: Date.now(),
    });

    // Create phrasings and link questions
    for (const question of args.questions) {
      await ctx.db.insert('phrasings', {
        userId: question.userId,
        conceptId,
        question: question.question,
        explanation: question.explanation,
        type: question.type,
        options: question.options,
        correctAnswer: question.correctAnswer,
        attemptCount: question.attemptCount,
        correctCount: question.correctCount,
        lastAttemptedAt: question.lastAttemptedAt,
        createdAt: question.generatedAt ?? Date.now(),
        embedding: question.embedding,
        embeddingGeneratedAt: question.embeddingGeneratedAt,
      });

      // Link question to concept
      await ctx.db.patch(question._id, { conceptId });
    }
  },
});

/**
 * Diagnostic query: Check migration status
 *
 * Returns count of total questions, orphaned questions, and linked questions.
 * Use to verify migration completion (orphaned should be 0 when complete).
 */
export const checkMigrationStatus = query({
  args: {},
  handler: async (ctx) => {
    const totalQuestions = await ctx.db.query('questions').collect();
    const orphaned = totalQuestions.filter((q) => !q.conceptId);
    const linked = totalQuestions.filter((q) => q.conceptId);

    return {
      totalQuestions: totalQuestions.length,
      orphaned: orphaned.length,
      linked: linked.length,
      percentMigrated:
        totalQuestions.length > 0 ? (linked.length / totalQuestions.length) * 100 : 0,
    };
  },
});

/**
 * Diagnostic query: Sample created concepts for manual review
 *
 * Returns sample of concepts with their phrasings for quality validation.
 * Use after migration to spot-check clustering and synthesis quality.
 */
export const sampleConcepts = query({
  args: {
    limit: v.optional(v.number()),
  },
  handler: async (ctx, args) => {
    const limit = args.limit ?? 10;

    const concepts = await ctx.db.query('concepts').order('desc').take(limit);

    // Fetch phrasings for each concept
    const samplesWithPhrasings = await Promise.all(
      concepts.map(async (concept) => {
        const phrasings = await ctx.db
          .query('phrasings')
          .withIndex('by_concept', (q) => q.eq('conceptId', concept._id))
          .collect();

        return {
          conceptId: concept._id,
          title: concept.title,
          description: concept.description,
          phrasingCount: concept.phrasingCount,
          fsrsState: concept.fsrs.state,
          fsrsReps: concept.fsrs.reps,
          phrasings: phrasings.map((p) => ({
            phrasingId: p._id,
            question: p.question,
            attemptCount: p.attemptCount,
            correctCount: p.correctCount,
          })),
        };
      })
    );

    return samplesWithPhrasings;
  },
});
