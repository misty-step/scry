/**
 * Concept Synthesis for Migration
 *
 * Generates unified concept titles and descriptions from clusters of related questions.
 * Used during one-time migration from legacy questions to concepts/phrasings system.
 *
 * Singleton clusters use question text directly.
 * Multi-question clusters use GPT-5-mini to identify common concept.
 */

import OpenAI from 'openai';
import type { Doc } from '../_generated/dataModel';
import type { ActionCtx } from '../_generated/server';

/**
 * Synthesize concept title/description from question cluster
 *
 * @param ctx - Action context (unused, kept for consistency with migration patterns)
 * @param questions - Array of related questions from clustering algorithm
 * @returns Promise resolving to { title, description } for concept creation
 */
export async function synthesizeConceptFromQuestions(
  ctx: ActionCtx,
  questions: Doc<'questions'>[]
): Promise<{ title: string; description: string }> {
  // Singleton fallback: use question text directly
  if (questions.length === 1) {
    const q = questions[0];
    return {
      title: truncate(q.question, 120),
      description: q.explanation || 'Concept from question migration',
    };
  }

  // Multi-question synthesis via OpenAI GPT-5-mini
  const openai = new OpenAI({ apiKey: process.env.OPENAI_API_KEY! });
  const questionsText = questions.map((q, i) => `${i + 1}. ${q.question}`).join('\n');

  const response = await openai.chat.completions.create({
    model: 'gpt-5-mini',
    messages: [
      {
        role: 'user',
        content: `Analyze these related questions and identify the SINGLE underlying concept being tested:

${questionsText}

Requirements:
- Title (max 120 chars): Atomic noun phrase, no "and"/"vs"/"or"
- Description (max 300 chars): Brief explanation of what concept tests

Return JSON: {"title": "...", "description": "..."}

Example: ["What is X?", "When was X created?"] → {"title": "X", "description": "..."}`,
      },
    ],
    response_format: { type: 'json_object' },
    max_completion_tokens: 500,
  });

  const { title, description } = JSON.parse(response.choices[0].message.content);
  return {
    title: truncate(title.trim(), 120),
    description: truncate(description.trim(), 300),
  };
}

/**
 * Truncate text to maximum length with ellipsis
 */
function truncate(text: string, max: number): string {
  if (text.length <= max) return text;
  return text.substring(0, max - 3) + '...';
}
