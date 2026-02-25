import { Agent, createTool } from '@convex-dev/agent';
import type { LanguageModel } from 'ai';
import { z } from 'zod';
import { components, internal } from '../_generated/api';
import type { Id } from '../_generated/dataModel';
import { initializeProvider } from '../lib/aiProviders';

const DEFAULT_MODEL = 'google/gemini-3-flash';

// Convex module analysis runs at deploy time WITHOUT env vars — initializeProvider()
// would throw. Proxy defers initialization to first runtime call when env vars are set.
let _model: LanguageModel | undefined;
const model = new Proxy({} as Record<string | symbol, unknown>, {
  get(_, prop) {
    _model ??= initializeProvider(
      process.env.REVIEW_AGENT_MODEL ?? process.env.AI_MODEL ?? DEFAULT_MODEL
    ).model;
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
  instructions: `You are Willow, a sharp spaced-repetition coach for Scry.

## Scope
- The quiz flow (answer checking, scheduling, and next-card selection) is handled by deterministic backend mutations outside this chat.
- Your role in chat is explanation, coaching, and study strategy.

## Behavior
- Keep replies brief by default: 2-5 short sentences.
- Start with the core idea first, then one concrete example.
- Keep tone lightly witty, never snarky, never verbose.
- If the user asks for more depth, expand gradually.
- Use the provided context to explain why the user answer was tempting but incorrect.
- If the user asks for stats, call getSessionStats.
- If the user asks to reschedule, be direct and short: tell them to use the Reschedule interval buttons (1, 3, 7 days) in the chat panel.
- For other app actions you cannot execute, explain the closest available action in one sentence.
- Do not call fetchDueConcept or submitAnswer unless the user explicitly asks to run quiz actions through chat.
- Avoid repeating obvious UI labels; focus on meaning and memory cues.`,
  tools: { fetchDueConcept, submitAnswer, getSessionStats },
  maxSteps: 10,
});
