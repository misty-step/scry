import { createOpenRouter } from '@openrouter/ai-sdk-provider';
import type { LanguageModel } from 'ai';
import type { Logger } from 'pino';
import { getSecretDiagnostics } from './envDiagnostics';

type SecretDiagnostics = ReturnType<typeof getSecretDiagnostics>;

/**
 * Token budget configuration for reasoning-enabled models.
 *
 * When using extended thinking (reasoning tokens), the model allocates
 * tokens from the total budget for internal reasoning before generating
 * visible output. This configuration ensures sufficient headroom exists
 * for both reasoning AND content generation.
 *
 * Invariant: maxOutputTokens > reasoningTokens (enforced by getReasoningOptions)
 */
export const REASONING_BUDGET = {
  /** Full budget for generation tasks (concepts, phrasings) */
  full: {
    maxOutputTokens: 16384,
    reasoningTokens: 8192,
  },
  /** Light budget for evaluation tasks (scoring, simple decisions) */
  light: {
    maxOutputTokens: 4096,
    reasoningTokens: 1024,
  },
} as const;

export type ReasoningPreset = keyof typeof REASONING_BUDGET;

/**
 * Get configured options for reasoning-enabled model calls.
 *
 * This is the ONLY correct way to configure reasoning for generateObject/generateText.
 * It returns both maxOutputTokens AND providerOptions together, ensuring the invariant
 * that maxOutputTokens > reasoning.max_tokens is always satisfied.
 *
 * @param preset - Budget preset: 'full' for generation, 'light' for evaluation
 * @returns Object with maxOutputTokens and providerOptions ready to spread into AI SDK calls
 *
 * @example
 * const response = await generateObject({
 *   model,
 *   schema,
 *   prompt,
 *   ...getReasoningOptions('full'),
 * });
 */
export function getReasoningOptions(preset: ReasoningPreset = 'full') {
  const budget = REASONING_BUDGET[preset];
  return {
    maxOutputTokens: budget.maxOutputTokens,
    providerOptions: {
      openrouter: {
        reasoning: {
          max_tokens: budget.reasoningTokens,
        },
      },
    },
  };
}

type MinimalLogger = {
  info?: (context: Record<string, unknown>, message?: string) => void;
  error?: (context: Record<string, unknown>, message?: string) => void;
};

type ProviderLogger = Pick<Logger, 'info' | 'error'> | MinimalLogger;

export interface ProviderClient {
  model: LanguageModel;
  diagnostics: SecretDiagnostics;
}

export interface InitializeProviderOptions {
  logger?: ProviderLogger;
  logContext?: Record<string, unknown>;
  deployment?: string;
}

/**
 * Initialize OpenRouter AI provider
 *
 * Supports 150+ models via single API: google/gemini-2.5-pro, anthropic/claude-3.5-sonnet, etc.
 * Requires OPENROUTER_API_KEY env var in Convex dashboard.
 *
 * @param modelId - Model identifier (e.g., 'google/gemini-2.5-pro-preview-0827', 'anthropic/claude-3.5-sonnet')
 * @param options - Provider initialization options (logger, context, deployment)
 * @returns ProviderClient with model and diagnostics
 */
export function initializeProvider(
  modelId: string,
  options: InitializeProviderOptions = {}
): ProviderClient {
  const apiKey = process.env.OPENROUTER_API_KEY;
  const diagnostics = getSecretDiagnostics(apiKey);
  const logFields = {
    ...(options.logContext ?? {}),
    provider: 'openrouter',
    model: modelId,
    keyDiagnostics: diagnostics,
    deployment: options.deployment ?? process.env.CONVEX_CLOUD_URL ?? 'unknown',
  };

  options.logger?.info?.(logFields, 'Using OpenRouter provider');

  if (!apiKey?.trim()) {
    const errorMessage = 'OPENROUTER_API_KEY not configured in Convex environment';
    options.logger?.error?.(logFields, errorMessage);
    throw new Error(errorMessage);
  }

  const openrouter = createOpenRouter({ apiKey });
  const model = openrouter(modelId) as unknown as LanguageModel;

  return { model, diagnostics };
}
