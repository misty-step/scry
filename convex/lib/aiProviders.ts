import { createGoogleGenerativeAI } from '@ai-sdk/google';
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

export function initializeGoogleProvider(
  modelName: string,
  options: InitializeProviderOptions = {}
): ProviderClient {
  const apiKey = process.env.GOOGLE_AI_API_KEY;
  const diagnostics = getSecretDiagnostics(apiKey);
  const logFields = {
    ...(options.logContext ?? {}),
    provider: 'google',
    model: modelName,
    keyDiagnostics: diagnostics,
    deployment: options.deployment ?? process.env.CONVEX_CLOUD_URL ?? 'unknown',
  };

  options.logger?.info?.(logFields, 'Using Google AI provider');

  if (!apiKey?.trim()) {
    const errorMessage = 'GOOGLE_AI_API_KEY not configured in Convex environment';
    options.logger?.error?.(logFields, errorMessage);
    throw new Error(errorMessage);
  }

  const google = createGoogleGenerativeAI({ apiKey });
  const model = google(modelName) as unknown as LanguageModel;

  return { model, diagnostics };
}

/**
 * Initialize OpenRouter provider for multi-model access
 *
 * Supports 150+ models via single API: google/gemini-2.5-pro, anthropic/claude-3.5-sonnet, etc.
 * Requires OPENROUTER_API_KEY env var in Convex dashboard.
 */
export function initializeOpenRouterProvider(
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

/**
 * Initialize AI provider based on AI_PROVIDER env var
 *
 * Routes to Google (default) or OpenRouter based on configuration.
 * Allows switching providers without code changes.
 *
 * @param modelId - Model identifier (e.g., 'gemini-3-pro-preview' for Google, 'google/gemini-2.5-pro' for OpenRouter)
 * @param options - Provider initialization options (logger, context, deployment)
 * @returns ProviderClient with model and diagnostics
 */
export function initializeProvider(
  modelId: string,
  options: InitializeProviderOptions = {}
): ProviderClient {
  const provider = process.env.AI_PROVIDER || 'google';

  if (provider === 'openrouter') {
    return initializeOpenRouterProvider(modelId, options);
  }

  return initializeGoogleProvider(modelId, options);
}
