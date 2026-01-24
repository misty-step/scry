/**
 * Prompt Resolution
 *
 * Provides compiled prompts from hardcoded templates.
 * Templates are the source of truth; evolutionary optimization
 * happens offline via scripts/evolve-prompts.ts.
 *
 * Prompt IDs:
 * - scry-intent-extraction
 * - scry-concept-synthesis
 * - scry-phrasing-generation
 *
 * @example
 * ```ts
 * const prompt = await getPrompt('scry-intent-extraction', { userInput: 'NATO alphabet' });
 * // Returns compiled prompt text from templates
 * ```
 */
import {
  buildConceptSynthesisPrompt,
  buildIntentExtractionPrompt,
  buildPhrasingGenerationPrompt,
} from './promptTemplates';

export type PromptId =
  | 'scry-intent-extraction'
  | 'scry-concept-synthesis'
  | 'scry-phrasing-generation';

export type PromptLabel = 'production';

export interface PromptResult {
  text: string;
  source: 'template';
  version?: string;
  promptId: PromptId;
  label: PromptLabel;
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
 * Build prompt from templates
 */
function buildPrompt<T extends PromptId>(promptId: T, variables: PromptVariables<T>): string {
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
 * Get a prompt by ID.
 *
 * Returns compiled prompt text from hardcoded templates.
 * Evolutionary optimization is handled offline by scripts/evolve-prompts.ts.
 *
 * @param promptId - The prompt identifier
 * @param variables - Variables to compile into the prompt template
 * @returns Compiled prompt text with metadata
 */
export async function getPrompt<T extends PromptId>(
  promptId: T,
  variables: PromptVariables<T>
): Promise<PromptResult> {
  return {
    text: buildPrompt(promptId, variables),
    source: 'template',
    promptId,
    label: 'production',
  };
}

/**
 * Get multiple prompts in parallel.
 */
export async function getPrompts<T extends PromptId>(
  requests: Array<{ promptId: T; variables: PromptVariables<T> }>
): Promise<PromptResult[]> {
  return Promise.all(requests.map((req) => getPrompt(req.promptId, req.variables)));
}
