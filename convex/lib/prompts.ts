/**
 * Prompt Resolution with Langfuse Integration
 *
 * Fetches prompts from Langfuse when configured, falls back to hardcoded templates.
 * Provides a unified API regardless of prompt source.
 *
 * Prompt IDs (Langfuse naming convention):
 * - scry-intent-extraction
 * - scry-concept-synthesis
 * - scry-phrasing-generation
 *
 * @example
 * ```ts
 * const prompt = await getPrompt('scry-intent-extraction', { userInput: 'NATO alphabet' });
 * // Returns compiled prompt text, from Langfuse or fallback
 * ```
 */
import { getLangfuse, isLangfuseConfigured } from './langfuse';
import {
  buildConceptSynthesisPrompt,
  buildIntentExtractionPrompt,
  buildPhrasingGenerationPrompt,
} from './promptTemplates';

export type PromptId =
  | 'scry-intent-extraction'
  | 'scry-concept-synthesis'
  | 'scry-phrasing-generation';

export type PromptLabel = 'production' | 'staging' | 'dev';

export interface PromptResult {
  text: string;
  source: 'langfuse' | 'fallback';
  version?: string;
  promptId: PromptId;
}

interface IntentExtractionVars {
  userInput: string;
}

interface ConceptSynthesisVars {
  intentJson: string;
}

interface PhrasingGenerationVars {
  conceptTitle: string;
  contentType?: 'verbatim' | 'enumerable' | 'conceptual' | 'mixed';
  originIntent?: string;
  targetCount: number;
  existingQuestions: string[];
}

type PromptVariables<T extends PromptId> = T extends 'scry-intent-extraction'
  ? IntentExtractionVars
  : T extends 'scry-concept-synthesis'
    ? ConceptSynthesisVars
    : T extends 'scry-phrasing-generation'
      ? PhrasingGenerationVars
      : never;

/**
 * Build fallback prompt using hardcoded templates
 */
function buildFallbackPrompt<T extends PromptId>(
  promptId: T,
  variables: PromptVariables<T>
): string {
  switch (promptId) {
    case 'scry-intent-extraction': {
      const vars = variables as IntentExtractionVars;
      return buildIntentExtractionPrompt(vars.userInput);
    }
    case 'scry-concept-synthesis': {
      const vars = variables as ConceptSynthesisVars;
      return buildConceptSynthesisPrompt(vars.intentJson);
    }
    case 'scry-phrasing-generation': {
      const vars = variables as PhrasingGenerationVars;
      return buildPhrasingGenerationPrompt(vars);
    }
    default:
      throw new Error(`Unknown prompt ID: ${promptId}`);
  }
}

/**
 * Get a prompt by ID, with Langfuse lookup and fallback.
 *
 * @param promptId - The prompt identifier
 * @param variables - Variables to compile into the prompt template
 * @param label - Environment label (production/staging/dev), defaults to production
 * @returns Compiled prompt text with source metadata
 */
export async function getPrompt<T extends PromptId>(
  promptId: T,
  variables: PromptVariables<T>,
  label: PromptLabel = 'production'
): Promise<PromptResult> {
  // Fast path: if Langfuse not configured, use fallback immediately
  if (!isLangfuseConfigured()) {
    return {
      text: buildFallbackPrompt(promptId, variables),
      source: 'fallback',
      promptId,
    };
  }

  try {
    const langfuse = getLangfuse();
    const prompt = await langfuse.getPrompt(promptId, undefined, { label });

    // Compile template with variables
    // Langfuse uses {{variable}} syntax, same as our fallbacks
    // Cast through unknown to handle complex variable types
    const varsRecord = Object.fromEntries(
      Object.entries(variables).map(([k, v]) => [k, typeof v === 'string' ? v : JSON.stringify(v)])
    );
    const compiled = prompt.compile(varsRecord);

    return {
      text: compiled,
      source: 'langfuse',
      version: prompt.version?.toString(),
      promptId,
    };
  } catch (error) {
    // Log warning but don't throw - use fallback
     
    console.warn(`Langfuse prompt fetch failed for ${promptId}, using fallback:`, error);

    return {
      text: buildFallbackPrompt(promptId, variables),
      source: 'fallback',
      promptId,
    };
  }
}

/**
 * Get multiple prompts in parallel.
 * Useful when a pipeline needs all prompts at once.
 */
export async function getPrompts<T extends PromptId>(
  requests: Array<{ promptId: T; variables: PromptVariables<T> }>,
  label: PromptLabel = 'production'
): Promise<PromptResult[]> {
  return Promise.all(requests.map((req) => getPrompt(req.promptId, req.variables, label)));
}
