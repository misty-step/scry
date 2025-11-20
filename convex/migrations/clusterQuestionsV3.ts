/**
 * Two-Phase Semantic Clustering for Question Migration (V3)
 *
 * Phase 1: Aggressive pre-filtering using embeddings (find candidates)
 * Phase 2: LLM-based concept identification (precise clustering)
 *
 * Key improvements over V2:
 * - Lower threshold (0.65 vs 0.85) for candidate finding
 * - Single linkage (merge if ANY pair matches) vs average linkage
 * - Question text only for embedding (no explanation dilution)
 * - LLM validates actual concept identity
 */

import OpenAI from 'openai';
import { internal } from '../_generated/api';
import type { Doc } from '../_generated/dataModel';
import type { ActionCtx } from '../_generated/server';

// Phase 1: Low threshold - trust the LLM for precision
// Embeddings find candidates, LLM identifies actual concepts
// Bitter lesson: let AI do the cognitive work
const CANDIDATE_THRESHOLD = 0.5;

// No batch size limit - send full candidate groups to LLM for maximum context quality
// GPT-5.1 can handle 200+ questions in context

export type ConceptCluster = {
  questions: Doc<'questions'>[];
  conceptName: string;
  avgSimilarity: number;
};

export type ClusteringStats = {
  totalQuestions: number;
  candidateGroups: number;
  finalConcepts: number;
  largestCluster: number;
  singletons: number;
  similarityDistribution: {
    range: string;
    count: number;
  }[];
};

/**
 * Two-phase clustering: embedding pre-filter + LLM concept identification
 */
export async function clusterQuestionsV3(
  ctx: ActionCtx,
  questions: Doc<'questions'>[]
): Promise<{ clusters: ConceptCluster[]; stats: ClusteringStats }> {
  console.warn(`[V3] Starting two-phase clustering for ${questions.length} questions`);

  // Phase 1: Generate embeddings and find candidate groups
  const { candidateGroups, similarityDistribution } = await prefilterByEmbedding(ctx, questions);

  console.warn(`[V3] Phase 1 complete: ${candidateGroups.length} candidate groups`);
  console.warn(
    `[V3] Group sizes: ${candidateGroups
      .map((g) => g.length)
      .sort((a, b) => b - a)
      .slice(0, 10)
      .join(', ')}${candidateGroups.length > 10 ? '...' : ''}`
  );

  // Phase 2: LLM refines candidate groups into actual concepts
  const clusters: ConceptCluster[] = [];

  for (let i = 0; i < candidateGroups.length; i++) {
    const group = candidateGroups[i];

    if (group.length === 1) {
      // Singleton - no LLM needed
      clusters.push({
        questions: group,
        conceptName: truncateForConcept(group[0].question),
        avgSimilarity: 1.0,
      });
      continue;
    }

    // Use LLM to identify concepts within candidate group
    const refinedClusters = await refineWithLLM(group);

    console.warn(
      `[V3] Group ${i + 1}/${candidateGroups.length}: ${group.length} questions → ${refinedClusters.length} concepts`
    );

    clusters.push(...refinedClusters);
  }

  const stats: ClusteringStats = {
    totalQuestions: questions.length,
    candidateGroups: candidateGroups.length,
    finalConcepts: clusters.length,
    largestCluster: Math.max(...clusters.map((c) => c.questions.length)),
    singletons: clusters.filter((c) => c.questions.length === 1).length,
    similarityDistribution,
  };

  console.warn(`[V3] Phase 2 complete: ${clusters.length} final concepts`);
  console.warn(`[V3] Stats:`, stats);

  return { clusters, stats };
}

/**
 * Phase 1: Pre-filter by embedding similarity
 *
 * Uses single linkage clustering with lower threshold (0.65).
 * Goal: Find ALL potentially related questions, even if loosely similar.
 * Precision comes from LLM in Phase 2.
 */
async function prefilterByEmbedding(
  ctx: ActionCtx,
  questions: Doc<'questions'>[]
): Promise<{
  candidateGroups: Doc<'questions'>[][];
  similarityDistribution: { range: string; count: number }[];
}> {
  // Generate embeddings from question text ONLY (not explanation)
  const embeddings = await Promise.all(
    questions.map(async (q) => {
      if (q.embedding && q.embedding.length > 0) {
        return q.embedding;
      }
      // Generate embedding for question text only (better concept signal)
      return await ctx.runAction(internal.embeddings.generateEmbedding, {
        text: q.question,
      });
    })
  );

  // Build similarity matrix and track distribution
  const n = questions.length;
  const similarity: number[][] = [];
  const distributionBuckets = {
    '0.00-0.50': 0,
    '0.50-0.65': 0,
    '0.65-0.75': 0,
    '0.75-0.85': 0,
    '0.85-1.00': 0,
  };

  for (let i = 0; i < n; i++) {
    similarity[i] = [];
    for (let j = 0; j < n; j++) {
      if (i === j) {
        similarity[i][j] = 1.0;
        continue;
      }
      const sim = cosineSimilarity(embeddings[i], embeddings[j]);
      similarity[i][j] = sim;

      // Track distribution (only upper triangle to avoid double-counting)
      if (i < j) {
        if (sim < 0.5) distributionBuckets['0.00-0.50']++;
        else if (sim < 0.65) distributionBuckets['0.50-0.65']++;
        else if (sim < 0.75) distributionBuckets['0.65-0.75']++;
        else if (sim < 0.85) distributionBuckets['0.75-0.85']++;
        else distributionBuckets['0.85-1.00']++;
      }
    }
  }

  // Average-linkage clustering (merge based on mean similarity between clusters)
  // More balanced than single-linkage which creates mega-groups
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
    if (bestScore < CANDIDATE_THRESHOLD) {
      break;
    }

    // Merge cluster j into cluster i
    clusters[bestI].push(...clusters[bestJ]);
    clusters.splice(bestJ, 1);
  }

  const similarityDistribution = Object.entries(distributionBuckets).map(([range, count]) => ({
    range,
    count,
  }));

  return {
    candidateGroups: clusters.map((indices) => indices.map((i) => questions[i])),
    similarityDistribution,
  };
}

/**
 * Phase 2: LLM refines candidate group into actual concepts
 *
 * Takes a group of potentially related questions and asks GPT
 * to identify which ones actually test the same underlying concept.
 * Sends full candidate group to LLM for maximum context quality.
 */
async function refineWithLLM(questions: Doc<'questions'>[]): Promise<ConceptCluster[]> {
  if (questions.length <= 2) {
    // Small groups: assume single concept (LLM overhead not worth it)
    return [
      {
        questions,
        conceptName: await synthesizeConceptName(questions),
        avgSimilarity: 1.0,
      },
    ];
  }

  // Safety valve: chunk very large groups to avoid OpenAI timeout
  // 100 questions provides good context while staying under API limits
  const MAX_GROUP_SIZE = 100;
  if (questions.length > MAX_GROUP_SIZE) {
    console.warn(
      `[V3] Very large group (${questions.length} questions), chunking into batches of ${MAX_GROUP_SIZE}`
    );
    const chunks: Doc<'questions'>[][] = [];
    for (let i = 0; i < questions.length; i += MAX_GROUP_SIZE) {
      chunks.push(questions.slice(i, i + MAX_GROUP_SIZE));
    }

    const allClusters: ConceptCluster[] = [];
    for (const chunk of chunks) {
      const chunkClusters = await refineWithLLM(chunk);
      allClusters.push(...chunkClusters);
    }
    return allClusters;
  }

  const openai = new OpenAI({ apiKey: process.env.OPENAI_API_KEY! });

  const questionsText = questions.map((q, i) => `${i + 1}. ${q.question}`).join('\n');

  const prompt = `<task>
Classify which questions test the EXACT SAME underlying fact. Your goal is to identify true paraphrases - questions where knowing the answer to one means you definitely know the answer to the other.
</task>

<decision_framework>
For each pair of questions, apply this binary test:
"If a student correctly answers Question A, can they DEFINITELY answer Question B without any additional knowledge?"
- YES → Same concept (merge)
- NO or MAYBE → Different concepts (keep separate)
</decision_framework>

<critical_rules>
SAME CONCEPT (merge these):
- Identical fact asked with different wording
- Same question in different languages
- Synonymous phrasing ("capital of France" = "France's capital city")

DIFFERENT CONCEPTS (keep separate):
- Sequential items (line 1 vs line 2, step 1 vs step 2)
- Different attributes of same subject (date vs author vs location)
- Different items in a category (NATO A vs NATO B)
- Questions requiring different memorized facts
</critical_rules>

<failure_modes_to_avoid>
DO NOT group by:
- Topic (all "Hail Mary" questions are NOT one concept)
- Category (all "NATO alphabet" questions are NOT one concept)
- Subject matter (all questions about "Paris" are NOT one concept)

Each line of a prayer = separate concept
Each letter in NATO alphabet = separate concept
Each step in a procedure = separate concept
Date vs Author vs Location = separate concepts even for same subject
</failure_modes_to_avoid>

<concrete_examples>
SAME CONCEPT:
- "What is the capital of France?" + "Name France's capital city" → Both test: Paris
- "When was the Declaration signed?" + "What year was the Declaration signed?" → Both test: 1776

DIFFERENT CONCEPTS:
- "Complete line 3 of Hail Mary" + "Complete line 4 of Hail Mary" → Different lines to memorize
- "NATO word for A" + "NATO word for B" → Different letters (Alpha vs Bravo)
- "When was X created?" + "Who created X?" → Date fact vs Person fact
- "What is the capital of France?" + "What is France's population?" → Different facts about France
</concrete_examples>

<questions>
${questionsText}
</questions>

<output_requirements>
Return JSON with this exact structure:
{
  "clusters": [
    {"concept": "Short descriptive name (max 10 words)", "questions": [1, 2, ...]},
    ...
  ]
}

Rules:
- Each question number appears in exactly ONE cluster
- Use question numbers (1-based) as shown above
- When uncertain, keep questions SEPARATE (conservative default)
- Concept names should describe the specific fact being tested
</output_requirements>`;

  let attempt = 0;
  const maxAttempts = 3;

  while (attempt < maxAttempts) {
    try {
      const response = await openai.chat.completions.create({
        model: 'gpt-5.1',
        messages: [{ role: 'user', content: prompt }],
        response_format: { type: 'json_object' },
        max_completion_tokens: 16000,
        reasoning_effort: 'high',
      });

      const content = response.choices[0].message.content;
      if (!content) throw new Error('OpenAI returned no content');

      const parsed = JSON.parse(content);
      const llmClusters: { concept: string; questions: number[] }[] = parsed.clusters;

      // Validate: every question accounted for exactly once
      const assigned = new Set<number>();
      for (const cluster of llmClusters) {
        for (const idx of cluster.questions) {
          if (assigned.has(idx)) {
            console.warn(`[V3] LLM returned duplicate assignment for question ${idx}`);
          }
          assigned.add(idx);
        }
      }

      // Check for missing questions
      for (let i = 1; i <= questions.length; i++) {
        if (!assigned.has(i)) {
          console.warn(`[V3] LLM missed question ${i}, adding as singleton`);
          llmClusters.push({
            concept: truncateForConcept(questions[i - 1].question),
            questions: [i],
          });
        }
      }

      // Convert to ConceptCluster format
      return llmClusters.map((c) => ({
        questions: c.questions.map((idx) => questions[idx - 1]),
        conceptName: c.concept,
        avgSimilarity: 1.0, // LLM-validated clusters don't need similarity score
      }));
    } catch (error) {
      attempt++;
      const errorMessage = error instanceof Error ? error.message : String(error);
      const errorStack = error instanceof Error ? error.stack : undefined;
      console.error(`[V3] LLM attempt ${attempt} error:`, errorMessage);
      if (errorStack) console.error(`[V3] Stack:`, errorStack);

      if (attempt >= maxAttempts) {
        console.error(`[V3] LLM refinement failed after ${maxAttempts} attempts:`, error);
        // Fallback: treat entire group as single concept
        return [
          {
            questions,
            conceptName: await synthesizeConceptName(questions),
            avgSimilarity: 1.0,
          },
        ];
      }
      const delay = Math.pow(2, attempt - 1) * 1000;
      console.warn(`[V3] LLM attempt ${attempt} failed, retrying in ${delay}ms...`);
      await new Promise((resolve) => setTimeout(resolve, delay));
    }
  }

  throw new Error('Unreachable');
}

/**
 * Generate concept name from questions (fallback when LLM not used)
 */
async function synthesizeConceptName(questions: Doc<'questions'>[]): Promise<string> {
  if (questions.length === 1) {
    return truncateForConcept(questions[0].question);
  }

  // For small groups without LLM, use first question as basis
  return truncateForConcept(questions[0].question);
}

/**
 * Truncate question text to concept-friendly length
 */
function truncateForConcept(text: string): string {
  if (text.length <= 100) return text;
  return text.substring(0, 97) + '...';
}

/**
 * Calculate cosine similarity between two embedding vectors
 */
function cosineSimilarity(a: number[], b: number[]): number {
  if (a.length !== b.length) {
    throw new Error(`Embedding dimension mismatch: ${a.length} vs ${b.length}`);
  }

  const dotProduct = a.reduce((sum, val, i) => sum + val * b[i], 0);
  const magnitudeA = Math.sqrt(a.reduce((sum, val) => sum + val * val, 0));
  const magnitudeB = Math.sqrt(b.reduce((sum, val) => sum + val * val, 0));

  if (magnitudeA === 0 || magnitudeB === 0) return 0;

  return dotProduct / (magnitudeA * magnitudeB);
}

/**
 * Average similarity between all pairs in two clusters (average linkage)
 */
function averageSimilarity(indicesA: number[], indicesB: number[], matrix: number[][]): number {
  let sum = 0;
  let count = 0;
  for (const i of indicesA) {
    for (const j of indicesB) {
      if (i !== j) {
        sum += matrix[i][j];
        count++;
      }
    }
  }
  return count > 0 ? sum / count : 1.0;
}

/**
 * Analyze similarity distribution for diagnostics
 *
 * Call this to understand the pairwise similarity landscape before migration.
 */
export async function analyzeSimilarityDistribution(
  ctx: ActionCtx,
  questions: Doc<'questions'>[]
): Promise<{
  distribution: { range: string; count: number; percentage: string }[];
  summary: {
    totalPairs: number;
    pairsAbove85: number;
    pairsAbove75: number;
    pairsAbove65: number;
  };
}> {
  // Generate embeddings
  const embeddings = await Promise.all(
    questions.map(async (q) => {
      if (q.embedding && q.embedding.length > 0) {
        return q.embedding;
      }
      return await ctx.runAction(internal.embeddings.generateEmbedding, {
        text: q.question,
      });
    })
  );

  // Calculate all pairwise similarities
  const n = questions.length;
  const buckets = {
    '0.00-0.50': 0,
    '0.50-0.65': 0,
    '0.65-0.75': 0,
    '0.75-0.85': 0,
    '0.85-0.90': 0,
    '0.90-0.95': 0,
    '0.95-1.00': 0,
  };

  let pairsAbove85 = 0;
  let pairsAbove75 = 0;
  let pairsAbove65 = 0;
  let totalPairs = 0;

  for (let i = 0; i < n; i++) {
    for (let j = i + 1; j < n; j++) {
      const sim = cosineSimilarity(embeddings[i], embeddings[j]);
      totalPairs++;

      if (sim >= 0.85) pairsAbove85++;
      if (sim >= 0.75) pairsAbove75++;
      if (sim >= 0.65) pairsAbove65++;

      if (sim < 0.5) buckets['0.00-0.50']++;
      else if (sim < 0.65) buckets['0.50-0.65']++;
      else if (sim < 0.75) buckets['0.65-0.75']++;
      else if (sim < 0.85) buckets['0.75-0.85']++;
      else if (sim < 0.9) buckets['0.85-0.90']++;
      else if (sim < 0.95) buckets['0.90-0.95']++;
      else buckets['0.95-1.00']++;
    }
  }

  const distribution = Object.entries(buckets).map(([range, count]) => ({
    range,
    count,
    percentage: ((count / totalPairs) * 100).toFixed(1) + '%',
  }));

  return {
    distribution,
    summary: {
      totalPairs,
      pairsAbove85,
      pairsAbove75,
      pairsAbove65,
    },
  };
}
