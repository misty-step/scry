/**
 * Semantic Clustering for Question Migration
 *
 * Clusters ~163 orphaned questions into concept groups using cosine similarity.
 * One-time migration utility - optimized for clarity over reusability.
 *
 * Algorithm: Agglomerative clustering with 0.85 similarity threshold
 * - Starts with each question as separate cluster
 * - Iteratively merges most similar clusters
 * - Stops when no pairs exceed threshold
 */

import { internal } from '../_generated/api';
import type { Doc } from '../_generated/dataModel';
import type { ActionCtx } from '../_generated/server';

const SIMILARITY_THRESHOLD = 0.85;

export type QuestionCluster = {
  questions: Doc<'questions'>[];
  avgSimilarity: number;
};

/**
 * Cluster questions by semantic similarity
 *
 * Main entry point for migration. Ensures embeddings exist, builds similarity matrix,
 * and performs agglomerative clustering to group related questions.
 *
 * @param ctx - Action context for generating missing embeddings
 * @param questions - Questions to cluster (embeddings generated if missing)
 * @returns Array of clusters with average similarity metadata
 */
export async function clusterQuestionsBySimilarity(
  ctx: ActionCtx,
  questions: Doc<'questions'>[]
): Promise<QuestionCluster[]> {
  // Ensure all questions have embeddings
  const embeddings = await Promise.all(
    questions.map(async (q) => {
      if (q.embedding && q.embedding.length > 0) {
        return q.embedding;
      }
      // Generate embedding for question without one
      const text = q.question + ' ' + (q.explanation || '');
      return await ctx.runAction(internal.embeddings.generateEmbedding, { text });
    })
  );

  // Build similarity matrix
  const n = questions.length;
  const similarity: number[][] = [];
  for (let i = 0; i < n; i++) {
    similarity[i] = [];
    for (let j = 0; j < n; j++) {
      similarity[i][j] = i === j ? 1.0 : cosineSimilarity(embeddings[i], embeddings[j]);
    }
  }

  // Agglomerative clustering (track indices only)
  const clusters: number[][] = questions.map((_, i) => [i]);

  while (clusters.length > 1) {
    // Find most similar pair of clusters
    let bestScore = -1;
    let bestI = -1;
    let bestJ = -1;

    for (let i = 0; i < clusters.length; i++) {
      for (let j = i + 1; j < clusters.length; j++) {
        const score = averageSimilarity(clusters[i], clusters[j], similarity);
        if (score > bestScore) {
          bestScore = score;
          bestI = i;
          bestJ = j;
        }
      }
    }

    // Stop if best pair doesn't meet threshold
    if (bestScore < SIMILARITY_THRESHOLD) {
      break;
    }

    // Merge cluster j into cluster i
    clusters[bestI].push(...clusters[bestJ]);
    clusters.splice(bestJ, 1);
  }

  // Reconstruct questions from indices
  return clusters.map((indices) => ({
    questions: indices.map((i) => questions[i]),
    avgSimilarity: averageSimilarity(indices, indices, similarity),
  }));
}

/**
 * Calculate cosine similarity between two embedding vectors
 *
 * Returns value between 0 and 1 (negative values not expected for embeddings):
 * - 1.0 = identical vectors
 * - 0.0 = orthogonal vectors
 *
 * @param a - First embedding vector
 * @param b - Second embedding vector
 * @returns Cosine similarity score
 */
function cosineSimilarity(a: number[], b: number[]): number {
  if (a.length !== b.length) {
    throw new Error(`Embedding dimension mismatch: ${a.length} vs ${b.length}`);
  }

  const dotProduct = a.reduce((sum, val, i) => sum + val * b[i], 0);
  const magnitudeA = Math.sqrt(a.reduce((sum, val) => sum + val * val, 0));
  const magnitudeB = Math.sqrt(b.reduce((sum, val) => sum + val * val, 0));

  // Avoid division by zero for zero vectors
  if (magnitudeA === 0 || magnitudeB === 0) {
    return 0;
  }

  return dotProduct / (magnitudeA * magnitudeB);
}

/**
 * Calculate average similarity between two sets of questions
 *
 * Computes mean of all pairwise similarities between questions in set A and set B.
 * Used for cluster-to-cluster similarity (average linkage) and final cluster quality.
 *
 * @param indicesA - Question indices in first set
 * @param indicesB - Question indices in second set
 * @param matrix - Pre-computed similarity matrix
 * @returns Average pairwise similarity
 */
function averageSimilarity(indicesA: number[], indicesB: number[], matrix: number[][]): number {
  let sum = 0;
  let count = 0;

  for (const i of indicesA) {
    for (const j of indicesB) {
      sum += matrix[i][j];
      count++;
    }
  }

  return count > 0 ? sum / count : 1.0;
}
