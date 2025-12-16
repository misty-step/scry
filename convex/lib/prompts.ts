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

export type PromptLabel = 'latest' | 'staging' | 'dev' | 'production';

export interface PromptResult {
  text: string;
  source: 'langfuse' | 'fallback';
  version?: string;
  promptId: PromptId;
  /** The label that was actually used (may differ from requested if A/B testing) */
  label: PromptLabel;
}

// ═══════════════════════════════════════════════════════════════════════════
// A/B Testing Infrastructure
// ═══════════════════════════════════════════════════════════════════════════

interface ExperimentConfig {
  /** Human-readable experiment name for logging/analysis */
  name: string;
  /** Percentage of traffic to route to staging (0-100) */
  stagingPercent: number;
  /** Whether this experiment is active */
  enabled: boolean;
}

/**
 * A/B test configurations per prompt.
 *
 * When enabled, a percentage of requests will use the 'staging' label,
 * allowing you to test new prompt versions with real traffic.
 *
 * To start an experiment:
 * 1. Push new prompt to Langfuse with 'staging' label
 * 2. Set enabled: true and stagingPercent (e.g., 10 for 10%)
 * 3. Monitor quality scores in Langfuse filtered by promptLabel metadata
 * 4. If staging performs better, promote to 'production' label
 * 5. Set enabled: false when done
 */
const EXPERIMENTS: Partial<Record<PromptId, ExperimentConfig>> = {
  // Example: uncomment to run A/B test on phrasing generation
  // 'scry-phrasing-generation': {
  //   name: 'phrasing-v2-experiment',
  //   stagingPercent: 10,
  //   enabled: false,
  // },
};

/**
 * Select prompt label based on A/B test configuration.
 *
 * @param promptId - The prompt to check for experiments
 * @param requestedLabel - The label requested by the caller
 * @returns The label to actually use (may be 'staging' if in experiment)
 */
function selectPromptLabel(promptId: PromptId, requestedLabel: PromptLabel): PromptLabel {
  const experiment = EXPERIMENTS[promptId];

  // If no experiment or disabled, use requested label
  if (!experiment?.enabled) {
    return requestedLabel;
  }

  // Roll the dice for A/B testing
  const roll = Math.random() * 100;
  if (roll < experiment.stagingPercent) {
    // Log A/B test routing for analysis
    // eslint-disable-next-line no-console
    console.log(
      `[prompt-ab] ${promptId}: routing to staging (experiment: ${experiment.name}, roll: ${roll.toFixed(1)})`
    );
    return 'staging';
  }

  return requestedLabel;
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
 * Supports A/B testing: if an experiment is enabled for this prompt,
 * a percentage of requests will be routed to the 'staging' label.
 *
 * @param promptId - The prompt identifier
 * @param variables - Variables to compile into the prompt template
 * @param requestedLabel - Environment label (production/staging/dev), defaults to production
 * @returns Compiled prompt text with source metadata and actual label used
 */
export async function getPrompt<T extends PromptId>(
  promptId: T,
  variables: PromptVariables<T>,
  requestedLabel: PromptLabel = 'latest'
): Promise<PromptResult> {
  // Apply A/B testing logic to potentially override the label
  const label = selectPromptLabel(promptId, requestedLabel);

  // Fast path: if Langfuse not configured, use fallback immediately
  if (!isLangfuseConfigured()) {
    return {
      text: buildFallbackPrompt(promptId, variables),
      source: 'fallback',
      promptId,
      label,
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
      label,
    };
  } catch (error) {
    // Log warning but don't throw - use fallback

    console.warn(`Langfuse prompt fetch failed for ${promptId}, using fallback:`, error);

    return {
      text: buildFallbackPrompt(promptId, variables),
      source: 'fallback',
      promptId,
      label,
    };
  }
}

/**
 * Get multiple prompts in parallel.
 * Useful when a pipeline needs all prompts at once.
 */
export async function getPrompts<T extends PromptId>(
  requests: Array<{ promptId: T; variables: PromptVariables<T> }>,
  label: PromptLabel = 'latest'
): Promise<PromptResult[]> {
  return Promise.all(requests.map((req) => getPrompt(req.promptId, req.variables, label)));
}
