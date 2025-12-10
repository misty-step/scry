import { createOpenRouter } from '@openrouter/ai-sdk-provider';
import type { LanguageModel } from 'ai';
import type { Logger } from 'pino';
import { getSecretDiagnostics } from './envDiagnostics';

type SecretDiagnostics = ReturnType<typeof getSecretDiagnostics>;

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
