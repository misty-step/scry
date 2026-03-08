import { z } from 'zod';

export const contentTypeEnum = z.enum(['verbatim', 'enumerable', 'conceptual', 'mixed']);
export type ContentType = z.infer<typeof contentTypeEnum>;

export const intentSchema = z.object({
  content_type: contentTypeEnum,
  goal: z.enum(['memorize', 'understand', 'apply']),
  atomic_units: z.array(z.string()),
  synthesis_ops: z.array(z.string()),
  confidence: z.number().min(0).max(1),
});
export type Intent = z.infer<typeof intentSchema>;

export const conceptIdeaSchema = z.object({
  title: z.string(),
  description: z.string(),
  whyItMatters: z.string(),
  contentType: contentTypeEnum,
  originIntent: z.string(),
});
export type ConceptIdea = z.infer<typeof conceptIdeaSchema>;

export const conceptIdeasSchema = z.object({
  concepts: z.array(conceptIdeaSchema).min(1),
});

export const generatedPhrasingSchema = z.object({
  question: z.string(),
  explanation: z.string(),
  type: z.enum(['multiple-choice', 'true-false']),
  options: z.array(z.string()).min(2).max(4),
  correctAnswer: z.string(),
});
export type GeneratedPhrasing = z.infer<typeof generatedPhrasingSchema>;

export const phrasingBatchSchema = z.object({
  phrasings: z.array(generatedPhrasingSchema).min(1),
});
