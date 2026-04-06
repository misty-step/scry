import { captureRuntimeException, captureRuntimeRequestError } from './lib/error-monitoring';
import { shouldEnableSentry } from './lib/sentry';

type ConceptTelemetryMetadata = {
  event?: string;
  phase?: 'stage_a' | 'stage_b' | 'iqc_scan' | 'iqc_apply' | 'embeddings_sync' | 'migration';
  correlationId?: string;
  conceptIds?: string[];
  actionCardId?: string;
  [key: string]: unknown;
};

export async function register() {
  if (process.env.NEXT_RUNTIME === 'nodejs') {
    if (shouldEnableSentry(process.env.SENTRY_DSN || process.env.NEXT_PUBLIC_SENTRY_DSN)) {
      await import('./sentry.server.config');
    }
  }

  if (process.env.NEXT_RUNTIME === 'edge') {
    if (shouldEnableSentry(process.env.SENTRY_DSN || process.env.NEXT_PUBLIC_SENTRY_DSN)) {
      await import('./sentry.edge.config');
    }
  }
}

export async function onRequestError(...args: unknown[]) {
  await captureRuntimeRequestError(...args);
}

export function captureConceptsTelemetryFailure(
  error: unknown,
  metadata: ConceptTelemetryMetadata
) {
  const normalizedError =
    error instanceof Error
      ? error
      : new Error(
          typeof error === 'string' ? error : `Concept pipeline failure: ${JSON.stringify(error)}`
        );

  const event = metadata.event || 'concepts.failure';
  const logPayload = {
    ...metadata,
    event,
    errorName: normalizedError.name,
    errorMessage: normalizedError.message,
  };

  // Use console directly - instrumentation.ts runs in Edge context where pino isn't available
  console.error('[concepts.failure]', logPayload);

  void captureRuntimeException(normalizedError, {
    context: {
      ...metadata,
      domain: 'concepts',
      phase: metadata.phase,
    },
  });
}
