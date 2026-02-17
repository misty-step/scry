import { Agent, createTool } from '@convex-dev/agent';
import { createOpenRouter } from '@openrouter/ai-sdk-provider';
import type { LanguageModel } from 'ai';
import { z } from 'zod';
import { api, components } from '../_generated/api';

const getModel = (): LanguageModel => {
  const apiKey = process.env.OPENROUTER_API_KEY;
  if (!apiKey?.trim()) {
    throw new Error('OPENROUTER_API_KEY not configured');
  }
  const modelId = process.env.AI_MODEL ?? 'google/gemini-3-flash-preview';
  const openrouter = createOpenRouter({ apiKey });
  return openrouter(modelId) as unknown as LanguageModel;
};

// --- Tools ---

const fetchDueConcept = createTool({
  description:
    'Fetch the next due concept for review. Returns the concept with a quiz question, or null if nothing is due.',
  args: z.object({}),
  handler: async (ctx): Promise<Record<string, unknown> | null> => {
    const result = await ctx.runQuery(api.concepts.getDue, {});
    if (!result) return null;
    return {
      conceptId: result.concept._id,
      conceptTitle: result.concept.title,
      conceptDescription: result.concept.description ?? '',
      fsrsState: result.concept.fsrs.state ?? 'new',
      stability: result.concept.fsrs.stability,
      difficulty: result.concept.fsrs.difficulty,
      lapses: result.concept.fsrs.lapses ?? 0,
      reps: result.concept.fsrs.reps ?? 0,
      retrievability: result.retrievability,
      phrasingId: result.phrasing._id,
      question: result.phrasing.question,
      type: result.phrasing.type ?? 'multiple-choice',
      options: result.phrasing.options ?? [],
      correctAnswer: result.phrasing.correctAnswer ?? '',
      explanation: result.phrasing.explanation ?? '',
      recentAttempts: result.interactions.length,
      recentCorrect: result.interactions.filter((i: { isCorrect: boolean }) => i.isCorrect).length,
    };
  },
});

const evaluateAnswer = createTool({
  description:
    'Evaluate the user answer against the correct answer. Returns whether correct and the explanation.',
  args: z.object({
    userAnswer: z.string().describe('The answer the user selected or typed'),
    correctAnswer: z.string().describe('The correct answer from the phrasing'),
  }),
  handler: async (_ctx, args): Promise<Record<string, unknown>> => {
    const isCorrect =
      args.userAnswer.trim().toLowerCase() === args.correctAnswer.trim().toLowerCase();
    return { isCorrect, correctAnswer: args.correctAnswer };
  },
});

const recordInteraction = createTool({
  description:
    'Record the review interaction and update FSRS scheduling. MUST be called after every answer evaluation. Never skip this.',
  args: z.object({
    conceptId: z.string().describe('The concept ID being reviewed'),
    phrasingId: z.string().describe('The phrasing ID that was shown'),
    userAnswer: z.string().describe('What the user answered'),
    isCorrect: z.boolean().describe('Whether the answer was correct'),
  }),
  handler: async (ctx, args): Promise<Record<string, unknown>> => {
    const result = await ctx.runMutation(api.concepts.recordInteraction, {
      conceptId: args.conceptId as never,
      phrasingId: args.phrasingId as never,
      userAnswer: args.userAnswer,
      isCorrect: args.isCorrect,
    });
    return {
      nextReview: result.nextReview,
      scheduledDays: result.scheduledDays,
      newState: result.newState,
    };
  },
});

const getSessionStats = createTool({
  description: 'Get the current due count and user statistics.',
  args: z.object({}),
  handler: async (ctx): Promise<Record<string, unknown>> => {
    const dueCount = await ctx.runQuery(api.concepts.getConceptsDueCount, {});
    return { conceptsDue: dueCount.conceptsDue };
  },
});

// --- Agent Definition ---

export const reviewAgent = new Agent(components.agent, {
  name: 'Review Tutor',
  languageModel: getModel(),
  instructions: `You are a spaced repetition tutor for Scry. Your role is to guide review sessions.

## Your Workflow
1. Call fetchDueConcept to get the next question
2. Present the question clearly to the user. Include the question text and all answer options formatted as a numbered list.
3. Wait for the user to answer
4. Call evaluateAnswer with the user's answer and the correct answer
5. Call recordInteraction to update FSRS scheduling — NEVER skip this step
6. Provide brief, helpful feedback (2-3 sentences). If wrong, explain why the correct answer is right.
7. Call fetchDueConcept for the next question and repeat

## Rules
- ALWAYS call recordInteraction after evaluating. FSRS scheduling is sacred.
- Keep feedback concise. Don't lecture.
- If fetchDueConcept returns null, tell the user all reviews are done and show stats via getSessionStats.
- Present one question at a time.
- When the user says "start", "begin", "review", or similar, begin by fetching the first concept.
- Format questions clearly with numbered options.`,
  tools: { fetchDueConcept, evaluateAnswer, recordInteraction, getSessionStats },
  maxSteps: 8,
});
