/**
 * AI Generation Action Module
 *
 * Processes background question generation jobs using validated AI generation.
 * This module handles the complete lifecycle from job initialization through
 * question generation with schema validation and completion.
 *
 * ARCHITECTURE: 1-Phase Learning Science Approach
 * - Single comprehensive prompt incorporating all learning science principles
 * - GPT-5 with high reasoning effort for optimal quality
 * - Structured outputs via Zod schema validation
 */

import { generateObject } from 'ai';
import { v } from 'convex/values';
import { z } from 'zod';
import { internal } from './_generated/api';
import type { Doc, Id } from './_generated/dataModel';
import { internalAction } from './_generated/server';
import { initializeProvider, type ProviderClient } from './lib/aiProviders';
import { trackEvent } from './lib/analytics';
import { TARGET_PHRASINGS_PER_CONCEPT } from './lib/conceptConstants';
import { MAX_CONCEPTS_PER_GENERATION } from './lib/constants';
import {
  createConceptsLogger,
  generateCorrelationId,
  logConceptEvent,
  type LogContext,
} from './lib/logger';
import { buildConceptSynthesisPrompt, buildPhrasingGenerationPrompt } from './lib/promptTemplates';
import { generateObjectWithResponsesApi } from './lib/responsesApi';

// Logger for this module
const conceptsLogger = createConceptsLogger({
  module: 'aiGeneration',
});

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

// Zod schema for concept synthesis (Stage A)
const conceptIdeaSchema = z.object({
  title: z.string(),
  description: z.string(),
  whyItMatters: z.string(),
});

const conceptIdeasSchema = z.object({
  concepts: z.array(conceptIdeaSchema).min(1),
});

type ConceptIdea = z.infer<typeof conceptIdeaSchema>;

const generatedPhrasingSchema = z.object({
  question: z.string(),
  explanation: z.string(),
  type: z.enum(['multiple-choice', 'true-false']),
  options: z.array(z.string()).min(2).max(4),
  correctAnswer: z.string(),
});

const phrasingBatchSchema = z.object({
  phrasings: z.array(generatedPhrasingSchema).min(1),
});

type GeneratedPhrasing = z.infer<typeof generatedPhrasingSchema>;

type GenerationErrorCode = 'SCHEMA_VALIDATION' | 'RATE_LIMIT' | 'API_KEY' | 'NETWORK' | 'UNKNOWN';

class GenerationPipelineError extends Error {
  constructor(
    message: string,
    public readonly code: GenerationErrorCode,
    public readonly retryable: boolean
  ) {
    super(message);
  }
}

export type ConceptPreparationStats = {
  totalIdeas: number;
  accepted: number;
  skippedEmptyTitle: number;
  skippedEmptyDescription: number;
  skippedDuplicate: number;
  fallbackUsed: boolean;
};

export function prepareConceptIdeas(
  ideas: ConceptIdea[],
  fallbackPrompt?: string
): {
  concepts: Array<{ title: string; description: string }>;
  stats: ConceptPreparationStats;
} {
  const normalizedConcepts: Array<{ title: string; description: string }> = [];
  const seenTitles = new Set<string>();
  const stats: ConceptPreparationStats = {
    totalIdeas: ideas.length,
    accepted: 0,
    skippedEmptyTitle: 0,
    skippedEmptyDescription: 0,
    skippedDuplicate: 0,
    fallbackUsed: false,
  };

  for (const idea of ideas) {
    const title = idea.title?.trim() ?? '';
    const description = idea.description?.trim() ?? '';

    if (!title) {
      stats.skippedEmptyTitle += 1;
      continue;
    }

    if (!description) {
      stats.skippedEmptyDescription += 1;
      continue;
    }

    const titleKey = title.toLowerCase();
    if (seenTitles.has(titleKey)) {
      stats.skippedDuplicate += 1;
      continue;
    }

    seenTitles.add(titleKey);
    normalizedConcepts.push({ title, description });
  }

  if (normalizedConcepts.length === 0 && ideas.length > 0) {
    const fallbackIdea = ideas[0];
    const fallbackTitle = fallbackIdea.title?.trim() || fallbackPrompt?.trim() || 'Learner Concept';
    const fallbackDescription =
      fallbackIdea.description?.trim() ||
      fallbackIdea.whyItMatters?.trim() ||
      (fallbackPrompt
        ? `Deepening understanding of "${fallbackPrompt}".`
        : 'User-specified topic.');

    normalizedConcepts.push({
      title: fallbackTitle,
      description: fallbackDescription,
    });
    stats.fallbackUsed = true;
  }

  stats.accepted = normalizedConcepts.length;

  return { concepts: normalizedConcepts, stats };
}

type PreparedPhrasing = {
  question: string;
  explanation: string;
  type: 'multiple-choice' | 'true-false';
  options: string[];
  correctAnswer: string;
};

export function prepareGeneratedPhrasings(
  generated: GeneratedPhrasing[],
  existingQuestions: string[],
  targetCount: number
): PreparedPhrasing[] {
  const normalized: PreparedPhrasing[] = [];
  const seen = new Set(existingQuestions.map((q) => q.trim().toLowerCase()));

  for (const phrasing of generated) {
    if (normalized.length >= targetCount) {
      break;
    }

    const question = phrasing.question.trim();
    const explanation = phrasing.explanation.trim();
    if (question.length < 12 || question.length > 400) {
      continue;
    }
    if (explanation.length < 12) {
      continue;
    }

    const questionKey = question.toLowerCase();
    if (seen.has(questionKey)) {
      continue;
    }

    const options = phrasing.options.map((opt) => opt.trim()).filter(Boolean);
    if (phrasing.type === 'multiple-choice') {
      if (options.length < 3 || options.length > 5) {
        continue;
      }
    } else if (phrasing.type === 'true-false') {
      if (options.length !== 2) {
        continue;
      }
    }

    if (!options.some((opt) => opt.toLowerCase() === phrasing.correctAnswer.trim().toLowerCase())) {
      continue;
    }

    const uniqueOptions = Array.from(new Set(options.map((opt) => opt.toLowerCase()))).map(
      (lower) => options.find((opt) => opt.toLowerCase() === lower) || lower
    );

    const prepared: PreparedPhrasing = {
      question,
      explanation,
      type: phrasing.type,
      options: uniqueOptions,
      correctAnswer:
        uniqueOptions.find(
          (opt) => opt.toLowerCase() === phrasing.correctAnswer.trim().toLowerCase()
        ) ?? phrasing.correctAnswer.trim(),
    };

    normalized.push(prepared);
    seen.add(questionKey);
  }

  return normalized;
}

function calculateConflictScore(questions: string[]): number | undefined {
  const normalized = questions.map((q) => q.trim().toLowerCase());
  const unique = new Set(normalized);
  const conflicts = normalized.length - unique.size;
  return conflicts > 0 ? conflicts : undefined;
}

/**
 * Classify error for appropriate handling and retry logic
 */
function classifyError(error: Error): { code: GenerationErrorCode; retryable: boolean } {
  const message = error.message.toLowerCase();
  const errorName = error.name || '';

  // Schema validation errors - AI generated invalid format
  if (
    errorName.includes('AI_NoObjectGeneratedError') ||
    message.includes('schema') ||
    message.includes('validation') ||
    message.includes('does not match validator')
  ) {
    return { code: 'SCHEMA_VALIDATION', retryable: true };
  }

  // Rate limit errors are transient and retryable
  if (message.includes('rate limit') || message.includes('429') || message.includes('quota')) {
    return { code: 'RATE_LIMIT', retryable: true };
  }

  // API key errors are permanent and not retryable
  if (message.includes('api key') || message.includes('401') || message.includes('unauthorized')) {
    return { code: 'API_KEY', retryable: false };
  }

  // Network/timeout errors are transient and retryable
  if (message.includes('network') || message.includes('timeout') || message.includes('etimedout')) {
    return { code: 'NETWORK', retryable: true };
  }

  // Unknown errors are treated as non-retryable by default
  return { code: 'UNKNOWN', retryable: false };
}

/**
 * Process a generation job
 *
 * This is the main entry point for background question generation.
 * It handles the complete flow from job initialization through
 * intent clarification, streaming generation, and completion.
 */
export const processJob = internalAction({
  args: {
    jobId: v.id('generationJobs'),
  },
  handler: async (ctx, args) => {
    const startTime = Date.now();
    let job: Doc<'generationJobs'> | null = null;
    const stageACorrelationId = generateCorrelationId('stage-a');
    const stageAMetadata = {
      phase: 'stage_a' as const,
      correlationId: stageACorrelationId,
      jobId: args.jobId,
    };

    // Initialize AI provider from environment configuration
    // Defaults to OpenAI for production, but can be overridden via env vars
    const requestedProvider = process.env.AI_PROVIDER || 'openai';
    const modelName = process.env.AI_MODEL || 'gpt-5.1';
    const reasoningEffort = process.env.AI_REASONING_EFFORT || 'high';
    const verbosity = process.env.AI_VERBOSITY || 'medium';

    // Declare keyDiagnostics outside conditional blocks for error handler access
    let keyDiagnostics: ProviderClient['diagnostics'] = {
      present: false,
      length: 0,
      fingerprint: null,
    };
    let model: ProviderClient['model'];
    let openaiClient: ProviderClient['openaiClient'];
    let provider: ProviderClient['provider'] = 'openai';

    try {
      const providerClient = await initializeProvider(requestedProvider, modelName, {
        logger,
        logContext: {
          ...stageAMetadata,
          jobId: args.jobId,
          ...(requestedProvider.toLowerCase() === 'openai' && { reasoningEffort }),
        },
        deployment: process.env.CONVEX_CLOUD_URL ?? 'unknown',
      });

      provider = providerClient.provider;
      model = providerClient.model;
      openaiClient = providerClient.openaiClient;
      keyDiagnostics = providerClient.diagnostics;
    } catch (error) {
      const err = error as Error;
      const errorCode = err.message.includes('not configured') ? 'API_KEY' : 'CONFIG_ERROR';

      await ctx.runMutation(internal.generationJobs.failJob, {
        jobId: args.jobId,
        errorMessage: err.message,
        errorCode,
        retryable: false,
      });

      throw err;
    }

    try {
      logger.info(
        {
          ...stageAMetadata,
          provider,
          model: modelName,
          reasoningEffort,
          verbosity,
        },
        'Starting Stage A job processing'
      );

      // Update job to processing status
      await ctx.runMutation(internal.generationJobs.updateProgress, {
        jobId: args.jobId,
        phase: 'clarifying',
      });

      // Fetch job details
      job = await ctx.runQuery(internal.generationJobs.getJobByIdInternal, {
        jobId: args.jobId,
      });

      if (!job) {
        logger.error({ ...stageAMetadata }, 'Job not found');
        throw new Error('Job not found');
      }

      // Check if already cancelled
      if (job.status === 'cancelled') {
        logger.info(
          { ...stageAMetadata, userId: job.userId },
          'Job already cancelled, exiting early'
        );
        return;
      }

      logger.info(
        {
          ...stageAMetadata,
          prompt: job.prompt,
          userId: job.userId,
        },
        'Job details fetched'
      );

      logConceptEvent(conceptsLogger, 'info', 'Stage A concept synthesis started', {
        ...stageAMetadata,
        event: 'start',
        userId: job.userId,
        provider,
        model: modelName,
        reasoningEffort,
        verbosity,
      });

      trackEvent('Quiz Generation Started', {
        jobId: args.jobId,
        userId: String(job.userId),
        provider,
      });

      await ctx.runMutation(internal.generationJobs.updateProgress, {
        jobId: args.jobId,
        phase: 'concept_synthesis',
      });

      const conceptPrompt = buildConceptSynthesisPrompt(job.prompt);

      let finalResponse;
      if (provider === 'openai' && openaiClient) {
        finalResponse = await generateObjectWithResponsesApi({
          client: openaiClient,
          model: modelName,
          input: conceptPrompt,
          schema: conceptIdeasSchema,
          schemaName: 'concepts',
          verbosity: verbosity as 'low' | 'medium' | 'high',
          reasoningEffort: reasoningEffort as 'minimal' | 'low' | 'medium' | 'high',
        });
      } else if (provider === 'google' && model) {
        finalResponse = await generateObject({
          model,
          schema: conceptIdeasSchema,
          prompt: conceptPrompt,
        });
      } else {
        throw new Error('Provider not initialized correctly');
      }

      const { object } = finalResponse;

      // Diagnostic logging for production debugging
      logger.info(
        {
          ...stageAMetadata,
          conceptCount: object.concepts.length,
          responseSize: JSON.stringify(object).length,
          rawResponseSnippet: JSON.stringify(object).slice(0, 1000),
        },
        'Raw AI response received'
      );

      const totalSuggestions = object.concepts.length;
      const preparedConceptsResult = prepareConceptIdeas(object.concepts, job.prompt);
      // Soft limit: Take top N concepts to prevent system overload while preventing validation crashes
      const preparedConcepts = preparedConceptsResult.concepts.slice(0, MAX_CONCEPTS_PER_GENERATION);

      if (preparedConcepts.length === 0) {
        throw new GenerationPipelineError(
          'The AI proposed concepts that were too broad or redundant. Try giving a narrower prompt.',
          'SCHEMA_VALIDATION',
          false
        );
      }

      logger.info(
        {
          ...stageAMetadata,
          totalSuggestions,
          acceptedConcepts: preparedConcepts.length,
          userId: job.userId,
          filterStats: preparedConceptsResult.stats,
        },
        'Concept synthesis validation complete'
      );

      const currentJob = await ctx.runQuery(internal.generationJobs.getJobByIdInternal, {
        jobId: args.jobId,
      });

      if (currentJob?.status === 'cancelled') {
        logger.info(
          { ...stageAMetadata, userId: job.userId },
          'Job cancelled by user before concept creation'
        );
        return;
      }

      const creationResult = await ctx.runMutation(internal.concepts.createMany, {
        userId: job.userId,
        jobId: args.jobId,
        concepts: preparedConcepts,
      });

      const conceptIds = creationResult.conceptIds;

      if (conceptIds.length === 0) {
        throw new GenerationPipelineError(
          'All generated concepts already exist in your library. Try prompting for different material.',
          'SCHEMA_VALIDATION',
          false
        );
      }

      await ctx.runMutation(internal.generationJobs.setConceptWork, {
        jobId: args.jobId,
        conceptIds,
      });

      await ctx.runMutation(internal.generationJobs.updateProgress, {
        jobId: args.jobId,
        phase: 'phrasing_generation',
        questionsGenerated: conceptIds.length,
        questionsSaved: 0,
        estimatedTotal: conceptIds.length * TARGET_PHRASINGS_PER_CONCEPT,
      });

      for (const conceptId of conceptIds) {
        await ctx.scheduler.runAfter(0, internal.aiGeneration.generatePhrasingsForConcept, {
          conceptId,
          jobId: args.jobId,
        });
      }

      const conceptIdStrings = conceptIds.map((id: Id<'concepts'>) => id.toString());

      logConceptEvent(conceptsLogger, 'info', 'Stage A concept synthesis completed', {
        ...stageAMetadata,
        event: 'completed',
        userId: job.userId,
        conceptIds: conceptIdStrings,
        conceptCount: conceptIds.length,
        pendingConceptIds: conceptIds.length,
      });
    } catch (error) {
      const err = error as Error;
      let code: GenerationErrorCode;
      let retryable: boolean;

      if (err instanceof GenerationPipelineError) {
        code = err.code;
        retryable = err.retryable;
      } else {
        const classification = classifyError(err);
        code = classification.code;
        retryable = classification.retryable;
      }

      // Provide user-friendly error messages for common scenarios
      let userMessage = err.message;
      if (!(err instanceof GenerationPipelineError)) {
        if (code === 'SCHEMA_VALIDATION') {
          userMessage =
            'The AI generated concepts in an unexpected format. Please try again with a slightly different prompt.';
        } else if (code === 'RATE_LIMIT') {
          userMessage = 'Rate limit reached. Please wait a moment and try again.';
        } else if (code === 'API_KEY') {
          userMessage = 'API configuration error. Please contact support.';
        } else if (code === 'NETWORK') {
          userMessage = 'Network error. Please check your connection and try again.';
        }
      }

      logConceptEvent(conceptsLogger, 'error', 'Stage A concept synthesis failed', {
        ...stageAMetadata,
        event: 'failed',
        errorCode: code,
        retryable,
        errorMessage: err.message,
        errorName: err.name,
        stack: err.stack,
        keyDiagnostics,
        userId: job ? job.userId : undefined,
        conceptIds: job?.conceptIds?.map((id) => id.toString()),
      });

      // Mark job as failed
      await ctx.runMutation(internal.generationJobs.failJob, {
        jobId: args.jobId,
        errorMessage: userMessage,
        errorCode: code,
        retryable,
      });

      const durationMs = Date.now() - startTime;
      trackEvent('Quiz Generation Failed', {
        jobId: args.jobId,
        userId: job ? String(job.userId) : 'unknown',
        provider,
        questionCount: job ? job.questionsSaved : 0,
        errorType: code,
        durationMs,
      });

      // Re-throw to signal failure
      throw error;
    }
  },
});

export const generatePhrasingsForConcept = internalAction({
  args: {
    conceptId: v.id('concepts'),
    jobId: v.id('generationJobs'),
  },
  handler: async (ctx, args) => {
    const startTime = Date.now();
    const requestedProvider = process.env.AI_PROVIDER || 'openai';
    const modelName = process.env.AI_MODEL || 'gpt-5.1';
    const reasoningEffort = process.env.AI_REASONING_EFFORT || 'high';
    const verbosity = process.env.AI_VERBOSITY || 'medium';
    const stageBCorrelationId = generateCorrelationId('stage-b');
    const stageBMetadata = {
      phase: 'stage_b' as const,
      correlationId: stageBCorrelationId,
      jobId: args.jobId,
      conceptIds: [args.conceptId.toString()],
    };

    let job: Doc<'generationJobs'> | null = null;
    let keyDiagnostics: ProviderClient['diagnostics'] = {
      present: false,
      length: 0,
      fingerprint: null,
    };
    let model: ProviderClient['model'];
    let openaiClient: ProviderClient['openaiClient'];
    let provider: ProviderClient['provider'] = 'openai';

    try {
      job = await ctx.runQuery(internal.generationJobs.getJobByIdInternal, {
        jobId: args.jobId,
      });

      if (!job) {
        logger.error({ ...stageBMetadata }, 'Job not found for Stage B generation');
        return;
      }

      if (job.status === 'cancelled') {
        logger.info(
          { ...stageBMetadata, userId: job.userId },
          'Job cancelled before Stage B started'
        );
        return;
      }

      const concept = await ctx.runQuery(internal.concepts.getConceptById, {
        conceptId: args.conceptId,
      });
      if (!concept || concept.userId !== job.userId) {
        logger.warn(
          {
            ...stageBMetadata,
            conceptId: args.conceptId,
            userId: job.userId,
          },
          'Concept missing or unauthorized for Stage B, skipping'
        );

        if (job.pendingConceptIds?.includes(args.conceptId)) {
          await ctx.runMutation(internal.generationJobs.advancePendingConcept, {
            jobId: job._id,
            conceptId: args.conceptId,
            questionsGeneratedDelta: 0,
            questionsSavedDelta: 0,
          });
        }
        return;
      }

      if (!job.pendingConceptIds || !job.pendingConceptIds.includes(args.conceptId)) {
        logger.info(
          {
            ...stageBMetadata,
            conceptId: args.conceptId,
            userId: job.userId,
          },
          'Concept already processed for Stage B, skipping'
        );
        return;
      }

      try {
        const providerClient = await initializeProvider(requestedProvider, modelName, {
          logger,
          logContext: {
            ...stageBMetadata,
            jobId: args.jobId,
            conceptId: args.conceptId.toString(),
            ...(requestedProvider.toLowerCase() === 'openai' && { reasoningEffort }),
          },
          deployment: process.env.CONVEX_CLOUD_URL ?? 'unknown',
        });

        provider = providerClient.provider;
        model = providerClient.model;
        openaiClient = providerClient.openaiClient;
        keyDiagnostics = providerClient.diagnostics;
      } catch (error) {
        const err = error as Error;
        const errorCode = err.message.includes('not configured') ? 'API_KEY' : 'CONFIG_ERROR';

        await ctx.runMutation(internal.generationJobs.failJob, {
          jobId: args.jobId,
          errorMessage: err.message,
          errorCode,
          retryable: false,
        });

        throw err;
      }

      logConceptEvent(conceptsLogger, 'info', 'Stage B phrasing generation started', {
        ...stageBMetadata,
        event: 'start',
        userId: job.userId,
        provider,
        model: modelName,
        reasoningEffort,
        verbosity,
      });

      const existingPhrasings: Doc<'phrasings'>[] = await ctx.runQuery(
        internal.phrasings.getByConcept,
        {
          userId: concept.userId,
          conceptId: concept._id,
          limit: 20,
        }
      );

      const existingQuestions = existingPhrasings.map(
        (phrasing: Doc<'phrasings'>) => phrasing.question
      );

      const prompt = buildPhrasingGenerationPrompt({
        conceptTitle: concept.title,
        conceptDescription: concept.description ?? '',
        targetCount: TARGET_PHRASINGS_PER_CONCEPT,
        existingQuestions,
      });

      let finalResponse;
      if (provider === 'openai' && openaiClient) {
        finalResponse = await generateObjectWithResponsesApi({
          client: openaiClient,
          model: modelName,
          input: prompt,
          schema: phrasingBatchSchema,
          schemaName: 'phrasings',
          verbosity: verbosity as 'low' | 'medium' | 'high',
          reasoningEffort: reasoningEffort as 'minimal' | 'low' | 'medium' | 'high',
        });
      } else if (provider === 'google' && model) {
        finalResponse = await generateObject({
          model,
          schema: phrasingBatchSchema,
          prompt,
        });
      } else {
        throw new Error('Provider not initialized correctly');
      }

      const normalizedPhrasings = prepareGeneratedPhrasings(
        finalResponse.object.phrasings,
        existingQuestions,
        TARGET_PHRASINGS_PER_CONCEPT
      );

      if (normalizedPhrasings.length === 0) {
        throw new GenerationPipelineError(
          'The AI could not produce review-ready phrasings. Try rerunning in a few moments.',
          'SCHEMA_VALIDATION',
          true
        );
      }

      const preparedDocs: Array<
        PreparedPhrasing & {
          embedding?: number[];
          embeddingGeneratedAt?: number;
        }
      > = normalizedPhrasings.map((phrasing) => ({
        ...phrasing,
      }));

      const EMBEDDING_BATCH_SIZE = 5;
      for (let i = 0; i < preparedDocs.length; i += EMBEDDING_BATCH_SIZE) {
        const batch = preparedDocs.slice(i, i + EMBEDDING_BATCH_SIZE);
        const embeddingResults = await Promise.allSettled(
          batch.map(async (phrasing) => {
            const embeddingText = `${phrasing.question}\n\n${phrasing.explanation}`;
            const embedding = await ctx.runAction(internal.embeddings.generateEmbedding, {
              text: embeddingText,
            });
            return embedding;
          })
        );

        embeddingResults.forEach((result, index) => {
          if (result.status === 'fulfilled') {
            batch[index].embedding = result.value;
            batch[index].embeddingGeneratedAt = Date.now();
          } else {
            logger.warn(
              {
                ...stageBMetadata,
                event: 'stage-b.embedding.failure',
                jobId: args.jobId,
                conceptId: concept._id,
                error:
                  result.reason instanceof Error ? result.reason.message : String(result.reason),
              },
              'Failed to generate embedding for phrasing'
            );
          }
        });
      }

      const insertResult = await ctx.runMutation(internal.phrasings.insertGenerated, {
        conceptId: concept._id,
        userId: concept.userId,
        phrasings: preparedDocs,
      });
      const insertedIds = insertResult.ids;

      if (insertedIds.length === 0) {
        throw new GenerationPipelineError(
          'The AI responses were rejected after validation. This is usually temporary—please retry.',
          'SCHEMA_VALIDATION',
          true
        );
      }

      const newPhrasingCount = concept.phrasingCount + insertedIds.length;
      const thinScoreValue = Math.max(
        0,
        TARGET_PHRASINGS_PER_CONCEPT - Math.min(newPhrasingCount, TARGET_PHRASINGS_PER_CONCEPT)
      );
      const newQuestions = preparedDocs.map((phrasing) => phrasing.question);
      const allQuestions = existingQuestions.concat(newQuestions);
      const conflictScoreValue = calculateConflictScore(allQuestions);

      await ctx.runMutation(internal.concepts.applyPhrasingGenerationUpdate, {
        conceptId: concept._id,
        phrasingCount: newPhrasingCount,
        thinScore: thinScoreValue > 0 ? thinScoreValue : undefined,
        conflictScore: conflictScoreValue,
      });

      const progress = await ctx.runMutation(internal.generationJobs.advancePendingConcept, {
        jobId: job._id,
        conceptId: args.conceptId,
        questionsGeneratedDelta: normalizedPhrasings.length,
        questionsSavedDelta: insertedIds.length,
      });

      if (progress.pendingCount === 0) {
        const durationMs = Date.now() - (job.startedAt ?? job.createdAt);
        await ctx.runMutation(internal.generationJobs.completeJob, {
          jobId: job._id,
          topic: job.prompt,
          questionIds: [],
          conceptIds: job.conceptIds,
          durationMs,
        });

        trackEvent('Quiz Generation Completed', {
          jobId: job._id,
          userId: String(job.userId),
          provider,
          questionCount: progress.questionsSaved,
          durationMs,
        });
      }

      logConceptEvent(conceptsLogger, 'info', 'Stage B phrasing generation completed', {
        ...stageBMetadata,
        event: 'completed',
        userId: job.userId,
        phrasingsCreated: insertedIds.length,
        remainingConcepts: progress.pendingCount,
      });
    } catch (error) {
      const err = error as Error;
      const { code, retryable } =
        err instanceof GenerationPipelineError
          ? { code: err.code, retryable: err.retryable }
          : classifyError(err);
      const userMessage =
        err instanceof GenerationPipelineError
          ? err.message
          : code === 'SCHEMA_VALIDATION'
            ? 'The AI generated phrasings in an unexpected format. Please try again shortly.'
            : code === 'RATE_LIMIT'
              ? 'Rate limit reached. Please wait a moment and try again.'
              : code === 'API_KEY'
                ? 'API configuration error. Please contact support.'
                : code === 'NETWORK'
                  ? 'Network error. Please check your connection and try again.'
                  : err.message;

      await ctx.runMutation(internal.generationJobs.failJob, {
        jobId: args.jobId,
        errorMessage: userMessage,
        errorCode: code,
        retryable,
      });

      const durationMs = Date.now() - startTime;
      trackEvent('Quiz Generation Failed', {
        jobId: args.jobId,
        userId: job ? String(job.userId) : 'unknown',
        provider,
        questionCount: job ? job.questionsSaved : 0,
        errorType: code,
        durationMs,
      });

      logConceptEvent(conceptsLogger, 'error', 'Stage B phrasing generation failed', {
        ...stageBMetadata,
        event: 'failed',
        errorCode: code,
        retryable,
        errorMessage: err.message,
        errorName: err.name,
        stack: err.stack,
        keyDiagnostics,
        userId: job ? job.userId : undefined,
      });

      throw error;
    }
  },
});
