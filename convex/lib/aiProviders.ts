import { createGoogleGenerativeAI } from '@ai-sdk/google';
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
