import type { Id } from '../_generated/dataModel';

export const MAX_USER_ANSWER_LENGTH = 500;

function normalizeAnswer(value: string) {
  return value.trim().toLowerCase();
}

export function assertUserAnswerLength(userAnswer: string) {
  if (userAnswer.length > MAX_USER_ANSWER_LENGTH) {
    throw new Error(`Answer too long (max ${MAX_USER_ANSWER_LENGTH} characters)`);
  }
}

export function gradeAnswer(userAnswer: string, correctAnswer: string) {
  // Exact-match grading is intentional for deterministic multiple-choice flows.
  return normalizeAnswer(userAnswer) === normalizeAnswer(correctAnswer);
}

export function formatDueResult(
  result: Record<string, unknown> | null
): Record<string, unknown> | null {
  if (!result) return null;
  const typed = result as {
    concept: {
      _id: Id<'concepts'>;
      title: string;
      description?: string;
      fsrs: {
        state?: string;
        stability?: number;
        difficulty?: number;
        lapses?: number;
        reps?: number;
      };
    };
    phrasing: {
      _id: Id<'phrasings'>;
      question: string;
      type?: string;
      options?: string[];
    };
    retrievability?: number;
    interactions: Array<{ isCorrect: boolean }>;
  };

  return {
    conceptId: typed.concept._id,
    conceptTitle: typed.concept.title,
    conceptDescription: typed.concept.description ?? '',
    fsrsState: typed.concept.fsrs.state ?? 'new',
    stability: typed.concept.fsrs.stability,
    difficulty: typed.concept.fsrs.difficulty,
    lapses: typed.concept.fsrs.lapses ?? 0,
    reps: typed.concept.fsrs.reps ?? 0,
    retrievability: typed.retrievability,
    phrasingId: typed.phrasing._id,
    question: typed.phrasing.question,
    type: typed.phrasing.type ?? 'multiple-choice',
    options: typed.phrasing.options ?? [],
    recentAttempts: typed.interactions.length,
    recentCorrect: typed.interactions.filter((i) => i.isCorrect).length,
  };
}

interface InteractionResultSummary {
  conceptId?: Id<'concepts'>;
  nextReview: number;
  scheduledDays: number;
  newState: string;
  totalAttempts: number;
  totalCorrect: number;
  lapses: number;
  reps: number;
}

export function buildSubmitAnswerPayload(args: {
  result: InteractionResultSummary;
  userAnswer: string;
  correctAnswer: string;
  isCorrect: boolean;
  explanation: string;
  conceptTitle?: string;
  conceptDescription?: string;
}) {
  return {
    conceptId: args.result.conceptId,
    isCorrect: args.isCorrect,
    userAnswer: args.userAnswer,
    correctAnswer: args.correctAnswer,
    explanation: args.explanation,
    conceptTitle: args.conceptTitle ?? '',
    conceptDescription: args.conceptDescription ?? '',
    nextReview: args.result.nextReview,
    scheduledDays: args.result.scheduledDays,
    newState: args.result.newState,
    totalAttempts: args.result.totalAttempts,
    totalCorrect: args.result.totalCorrect,
    lapses: args.result.lapses,
    reps: args.result.reps,
  };
}
