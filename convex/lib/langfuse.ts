/**
 * Langfuse Observability Singleton
 *
 * Provides tracing, spans, and generations for LLM observability.
 * Designed for Convex serverless environment - MUST call flushLangfuse() at action end.
 *
 * @example
 * ```ts
 * const trace = getLangfuse().trace({ name: 'quiz-generation', userId });
 * const span = trace.span({ name: 'intent-extraction' });
 * const gen = span.generation({ name: 'extract-intent', model: 'gemini-3-pro' });
 * // ... LLM call ...
 * gen.end({ output, usage: { promptTokens, completionTokens } });
 * span.end();
 * trace.update({ output: { conceptCount: 5 } });
 * await flushLangfuse(); // CRITICAL: Call at action end
 * ```
 */
import { Langfuse } from 'langfuse';

let langfuseInstance: Langfuse | null = null;

/**
 * Get or create Langfuse singleton instance.
 *
 * Requires env vars: LANGFUSE_SECRET_KEY, LANGFUSE_PUBLIC_KEY
 * Optional: LANGFUSE_HOST (defaults to EU cloud)
 */
export function getLangfuse(): Langfuse {
  if (!langfuseInstance) {
    const secretKey = process.env.LANGFUSE_SECRET_KEY;
    const publicKey = process.env.LANGFUSE_PUBLIC_KEY;

    if (!secretKey || !publicKey) {
      throw new Error(
        'Langfuse not configured: LANGFUSE_SECRET_KEY and LANGFUSE_PUBLIC_KEY required'
      );
    }

    langfuseInstance = new Langfuse({
      secretKey,
      publicKey,
      baseUrl: process.env.LANGFUSE_HOST ?? 'https://cloud.langfuse.com',
      flushAt: 1, // Flush immediately in serverless (batching unreliable)
    });
  }

  return langfuseInstance;
}

/**
 * Flush pending events to Langfuse.
 * MUST be called at the end of every Convex action that uses tracing.
 *
 * In serverless, the runtime may terminate before batched events are sent.
 * This ensures all traces/spans/generations are persisted.
 */
export async function flushLangfuse(): Promise<void> {
  if (langfuseInstance) {
    await langfuseInstance.flushAsync();
  }
}

/**
 * Check if Langfuse is configured (env vars present).
 * Use for conditional tracing - skip if not configured.
 */
export function isLangfuseConfigured(): boolean {
  return Boolean(process.env.LANGFUSE_SECRET_KEY && process.env.LANGFUSE_PUBLIC_KEY);
}
