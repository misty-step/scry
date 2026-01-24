/**
 * LLM-as-Judge Scoring for Quiz Phrasings
 *
 * Evaluates generated quiz questions for quality using the same LLM.
 * Scores are attached to Langfuse traces for data-driven prompt iteration.
 *
 * Cost: ~500 tokens per evaluation ≈ $0.001 per concept
 */

import { generateObject } from 'ai';
import { z } from 'zod';
import { getReasoningOptions, type ProviderClient } from './aiProviders';
import type { GeneratedPhrasing } from './generationContracts';

/**
 * Schema for LLM-as-judge evaluation response
 */
const scoringResultSchema = z.object({
  overall: z.number().min(0).max(5),
  standalone: z.number().min(0).max(5),
  distractors: z.number().min(0).max(5),
  explanation: z.number().min(0).max(5),
  difficulty: z.number().min(0).max(5),
  reasoning: z.string(),
  issues: z.array(z.string()),
});

export type ScoringResult = z.infer<typeof scoringResultSchema>;

/**
 * Build the scoring prompt for LLM-as-judge evaluation.
 * Evaluates a batch of phrasings for a single concept.
 */
function buildScoringPrompt(conceptTitle: string, phrasings: GeneratedPhrasing[]): string {
  const phrasingsJson = JSON.stringify(phrasings, null, 2);

  return `You are evaluating quiz question quality for the concept: "${conceptTitle}"

PHRASINGS TO EVALUATE:
${phrasingsJson}

EVALUATION CRITERIA (score each 0-5):

1. STANDALONE (0-5): Can each question be understood without external context?
   - 5: Crystal clear, identifies topic explicitly, no ambiguity
   - 3: Understandable but could be clearer
   - 1: Requires context to understand what's being asked
   - 0: Completely dependent on external context

2. DISTRACTORS (0-5): For multiple-choice, are wrong answers plausible but clearly wrong?
   - 5: Distractors test genuine misconceptions, semantically adjacent
   - 3: Reasonable distractors but some are obvious
   - 1: Distractors are trivially wrong (formatting, typos) or random
   - 0: No real distractors (true-false only gets max 3 here)

3. EXPLANATION (0-5): Does the explanation teach WHY the answer is correct?
   - 5: Explains reasoning, addresses why others are wrong
   - 3: States the correct answer but lacks depth
   - 1: Just restates the question
   - 0: Missing or incorrect explanation

4. DIFFICULTY (0-5): Is the difficulty appropriate (not trivial, not impossible)?
   - 5: Requires understanding, tests meaningful knowledge
   - 3: Somewhat easy or somewhat hard
   - 1: Too trivial (asks obvious facts) or too obscure
   - 0: Either completely trivial or completely unanswerable

OVERALL: Average of the four scores, rounded to nearest 0.5

Return JSON with your evaluation. Be critical - we use these scores to improve prompts.`;
}

/**
 * Evaluate phrasing quality using LLM-as-judge.
 *
 * @param phrasings - Generated quiz phrasings to evaluate
 * @param conceptTitle - The concept these phrasings test
 * @param model - The AI model instance (reuse from generation)
 * @returns ScoringResult with scores and issues
 */
export async function evaluatePhrasingQuality(
  phrasings: GeneratedPhrasing[],
  conceptTitle: string,
  model: ProviderClient['model']
): Promise<ScoringResult> {
  if (phrasings.length === 0) {
    return {
      overall: 0,
      standalone: 0,
      distractors: 0,
      explanation: 0,
      difficulty: 0,
      reasoning: 'No phrasings to evaluate',
      issues: ['Empty phrasing batch'],
    };
  }

  try {
    const prompt = buildScoringPrompt(conceptTitle, phrasings);

    const result = await generateObject({
      model,
      schema: scoringResultSchema,
      prompt,
      // Use light preset for evaluation (faster, cheaper)
      ...getReasoningOptions('light'),
    });

    return result.object;
  } catch (error) {
    // On evaluation failure, return neutral scores with error info
    // Don't fail the whole generation just because scoring failed
    const errorMessage = error instanceof Error ? error.message : String(error);
    return {
      overall: -1, // Sentinel value indicating evaluation failed
      standalone: -1,
      distractors: -1,
      explanation: -1,
      difficulty: -1,
      reasoning: `Evaluation failed: ${errorMessage}`,
      issues: ['Scoring evaluation failed - scores are invalid'],
    };
  }
}

/**
 * Check if scoring is enabled (can be disabled via env var for cost control)
 */
export function isScoringEnabled(): boolean {
  return process.env.SKIP_SCORING !== 'true';
}
