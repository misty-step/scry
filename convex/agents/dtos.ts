import { v } from 'convex/values';

export const submitAnswerDirectArgs = {
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

export const getWeakAreasDirectArgs = {
  threadId: v.string(),
  limit: v.optional(v.number()),
};

export const rescheduleConceptDirectArgs = {
  threadId: v.string(),
  conceptId: v.id('concepts'),
  days: v.optional(v.number()),
};

export const fetchNextQuestionArgs = {
  threadId: v.string(),
};

export const sendMessageArgs = {
  threadId: v.string(),
  prompt: v.string(),
  // intent is intentionally excluded — it is specific to sendMessage and not shared
};

export const listMessagesArgs = {
  threadId: v.string(),
};
