/**
 * Prompt Mutation via LLM Meta-Prompts
 *
 * Uses an LLM to generate variations of a prompt template.
 * The meta-prompt instructs the LLM on mutation strategies.
 */

import { createOpenRouter } from '@openrouter/ai-sdk-provider';
import { generateText } from 'ai';

const MAX_RETRIES = 3;
const RETRY_DELAY_MS = 1000;

async function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

const MUTATION_META_PROMPT = `You are an expert prompt engineer. Given a prompt template, generate {{count}} variations that might perform better for the task.

ORIGINAL PROMPT:
---
{{original}}
---

MUTATION STRATEGIES to consider:
1. Restructure: Reorder sections, change heading styles, adjust hierarchy
2. Tone shift: More formal/casual, direct/exploratory, concise/detailed
3. Constraint emphasis: Stricter output format requirements, clearer boundaries
4. Example injection: Add/remove/modify few-shot examples if appropriate
5. Role framing: Change the persona, expertise level, or perspective
6. Chain-of-thought: Add or remove reasoning scaffolding
7. Negative constraints: Add "do not" instructions to prevent common errors
8. Simplification: Remove unnecessary words, combine redundant instructions
9. Specificity: Make vague instructions more concrete and actionable

REQUIREMENTS:
- Each variant must be SUBSTANTIALLY different (not just word swaps or minor rephrasing)
- Preserve the core task objective and expected output format
- Keep the same input variable placeholders (e.g., {{intentJson}}, {{userInput}})
- Variants should explore DIFFERENT strategies, not variations of the same strategy
- Output ONLY a valid JSON array of prompt strings, no explanation or markdown

Generate {{count}} variations as a JSON array:`;

interface MutationOptions {
  /** Number of variants to generate (default: 5) */
  count?: number;
  /** Model to use for mutation (default: google/gemini-2.0-flash-001) */
  model?: string;
  /** Temperature for generation (default: 0.9 for diversity) */
  temperature?: number;
}

/**
 * Generate prompt variations using LLM meta-prompts.
 *
 * @param original - The original prompt to mutate
 * @param options - Mutation configuration
 * @returns Array of mutated prompt strings
 */
export async function mutatePrompt(
  original: string,
  options: MutationOptions = {}
): Promise<string[]> {
  const { count = 5, model = 'google/gemini-2.0-flash-001', temperature = 0.9 } = options;

  const apiKey = process.env.OPENROUTER_API_KEY;
  if (!apiKey) {
    throw new Error('OPENROUTER_API_KEY environment variable is required');
  }

  const openrouter = createOpenRouter({ apiKey });

  const prompt = MUTATION_META_PROMPT.replace('{{original}}', original).replace(
    /{{count}}/g,
    String(count)
  );

  // Retry loop for API failures
  let text: string;
  let lastError: Error | undefined;

  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    try {
      const result = await generateText({
        model: openrouter(model),
        prompt,
        temperature,
      });
      text = result.text;
      break;
    } catch (error) {
      lastError = error instanceof Error ? error : new Error(String(error));
      console.error(`Mutation API attempt ${attempt}/${MAX_RETRIES} failed:`, lastError.message);

      if (attempt < MAX_RETRIES) {
        const delay = RETRY_DELAY_MS * attempt; // Exponential backoff
        console.log(`Retrying in ${delay}ms...`);
        await sleep(delay);
      }
    }
  }

  if (!text!) {
    throw new Error(`Mutation API failed after ${MAX_RETRIES} attempts: ${lastError?.message}`);
  }

  // Parse JSON response - handle potential markdown code blocks
  let jsonText = text.trim();
  if (jsonText.startsWith('```')) {
    jsonText = jsonText.replace(/^```(?:json)?\n?/, '').replace(/\n?```$/, '');
  }

  try {
    const variants = JSON.parse(jsonText);
    if (!Array.isArray(variants)) {
      throw new Error('Expected JSON array of prompt strings');
    }
    return variants.filter((v): v is string => typeof v === 'string');
  } catch (error) {
    console.error('Failed to parse mutation response:', text);
    throw new Error(`Failed to parse mutation response: ${error}`);
  }
}

/**
 * Generate a single mutation with a specific strategy focus.
 */
export async function mutateWithStrategy(
  original: string,
  strategy: string,
  options: Omit<MutationOptions, 'count'> = {}
): Promise<string> {
  const variants = await mutatePrompt(original, {
    ...options,
    count: 1,
  });
  return variants[0] || original;
}
