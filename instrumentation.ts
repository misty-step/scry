import * as Sentry from '@sentry/nextjs';

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
    await import('./sentry.server.config');
  }

  if (process.env.NEXT_RUNTIME === 'edge') {
    await import('./sentry.edge.config');
  }
}

export const onRequestError = Sentry.captureRequestError;

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

  Sentry.captureException(normalizedError, {
    tags: {
      domain: 'concepts',
      phase: metadata.phase,
    },
    extra: metadata,
  });
}
