import { v } from 'convex/values';

export const SubmitAnswerDirectArgs = {
  threadId: v.string(),
  conceptId: v.id('concepts'),
  phrasingId: v.id('phrasings'),
  userAnswer: v.string(),
  conceptTitle: v.optional(v.string()),
  conceptDescription: v.optional(v.string()),
  recentAttempts: v.optional(v.number()),
  recentCorrect: v.optional(v.number()),
  lapses: v.optional(v.number()),
  reps: v.optional(v.number()),
};

export const GetWeakAreasDirectArgs = {
  threadId: v.string(),
  limit: v.optional(v.number()),
};

export const RescheduleConceptDirectArgs = {
  threadId: v.string(),
  conceptId: v.id('concepts'),
  days: v.optional(v.number()),
};

export const FetchNextQuestionArgs = {
  threadId: v.string(),
};

export const SendMessageArgs = {
  threadId: v.string(),
  prompt: v.string(),
  // Note: intent validator imported where used
};

export const ListMessagesArgs = {
  threadId: v.string(),
};
