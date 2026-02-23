import { Agent, createTool } from '@convex-dev/agent';
import type { LanguageModel } from 'ai';
import { z } from 'zod';
import { components, internal } from '../_generated/api';
import type { Id } from '../_generated/dataModel';
import { initializeProvider } from '../lib/aiProviders';

const DEFAULT_MODEL = 'google/gemini-3-flash-preview';

// Convex module analysis runs at deploy time WITHOUT env vars — initializeProvider()
// would throw. Proxy defers initialization to first runtime call when env vars are set.
let _model: LanguageModel | undefined;
const model = new Proxy({} as Record<string | symbol, unknown>, {
  get(_, prop) {
    _model ??= initializeProvider(process.env.AI_MODEL ?? DEFAULT_MODEL).model;
    return (_model as unknown as Record<string | symbol, unknown>)[prop];
  },
}) as unknown as LanguageModel;

// --- Tools ---

const fetchDueConcept = createTool({
  description:
    'Fetch the next due concept for review. Returns the concept with a quiz question, or null if nothing is due. The UI renders this as an interactive card automatically.',
  args: z.object({}),
  handler: async (ctx): Promise<Record<string, unknown> | null> => {
    const result = await ctx.runQuery(internal.concepts.getDueInternal, {
      userId: ctx.userId as Id<'users'>,
    });
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
      explanation: result.phrasing.explanation ?? '',
      recentAttempts: result.interactions.length,
      recentCorrect: result.interactions.filter((i: { isCorrect: boolean }) => i.isCorrect).length,
    };
  },
});

const submitAnswer = createTool({
  description:
    'Evaluate the user answer, record the interaction, and update FSRS scheduling. Call this after the user answers. The UI renders the result as a rich feedback card automatically.',
  args: z.object({
    conceptId: z.string().describe('The concept ID'),
    phrasingId: z.string().describe('The phrasing ID'),
    userAnswer: z.string().describe('The answer the user selected or typed'),
    conceptTitle: z.string().optional().describe('The concept title'),
    recentAttempts: z.number().optional().describe('Recent attempt count from fetchDueConcept'),
    recentCorrect: z.number().optional().describe('Recent correct count from fetchDueConcept'),
    lapses: z.number().optional().describe('Lapse count from fetchDueConcept'),
    reps: z.number().optional().describe('Rep count from fetchDueConcept'),
  }),
  handler: async (ctx, args): Promise<Record<string, unknown>> => {
    // Fetch correct answer server-side — never trust client-supplied value
    const phrasing = await ctx.runQuery(internal.phrasings.getPhrasingInternal, {
      userId: ctx.userId as Id<'users'>,
      phrasingId: args.phrasingId as Id<'phrasings'>,
    });
    if (!phrasing) throw new Error('Phrasing not found');

    const correctAnswer = phrasing.correctAnswer ?? '';
    const isCorrect = args.userAnswer.trim().toLowerCase() === correctAnswer.trim().toLowerCase();

    const result = await ctx.runMutation(internal.concepts.recordInteractionInternal, {
      userId: ctx.userId as Id<'users'>,
      conceptId: args.conceptId as Id<'concepts'>,
      phrasingId: args.phrasingId as Id<'phrasings'>,
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

const getSessionStats = createTool({
  description: 'Get the current due count and user statistics.',
  args: z.object({}),
  handler: async (ctx): Promise<Record<string, unknown>> => {
    const dueCount = await ctx.runQuery(internal.concepts.getConceptsDueCountInternal, {
      userId: ctx.userId as Id<'users'>,
    });
    return { conceptsDue: dueCount.conceptsDue };
  },
});

// --- Agent Definition ---

export const reviewAgent = new Agent(components.agent, {
  name: 'Review Tutor',
  languageModel: model,
  instructions: `You are a spaced repetition tutor for Scry. Your role is to guide review sessions.

## Your Workflow
1. Call fetchDueConcept to get the next question
2. The UI automatically renders an interactive quiz card — do NOT repeat the question text or options in your message
3. Wait for the user to answer
4. Call submitAnswer with the user's answer plus context from fetchDueConcept (conceptId, phrasingId, conceptTitle, recentAttempts, recentCorrect, lapses, reps)
5. The UI automatically renders a rich feedback card — do NOT repeat correctness or explanation in your message
6. Immediately call fetchDueConcept for the next question

## Critical Rules
- NEVER write text that duplicates what the tool cards show. No question text, no options list, no "Correct!" or "Incorrect" text.
- After submitAnswer, go directly to fetchDueConcept. Do not add commentary between questions.
- If fetchDueConcept returns null, tell the user all reviews are done and show stats via getSessionStats.
- When the user says "start", "begin", "review", or similar, fetch the first concept.
- Your text messages should only contain information NOT shown in cards (e.g., encouragement at session end, clarifying ambiguous answers).`,
  tools: { fetchDueConcept, submitAnswer, getSessionStats },
  maxSteps: 10,
});
