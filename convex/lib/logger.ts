/**
 * Convex-compatible structured logger
 *
 * Since Convex functions run in a constrained environment without access to Node.js modules,
 * this provides a simple structured logging utility that works within Convex's runtime.
 */

import * as Sentry from '@sentry/nextjs';

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface LogContext {
  event?: string;
  context?: string;
  correlationId?: string;
  domains?: string[];
  deployment?: string;
  [key: string]: unknown;
}

const DEFAULT_DEPLOYMENT = process.env.CONVEX_CLOUD_URL ?? 'unknown';

/**
 * Determines if logging should be enabled based on environment
 */
function shouldLog(level: LogLevel): boolean {
  // In production, only log info and above
  if (process.env.NODE_ENV === 'production') {
    return level !== 'debug';
  }
  return true;
}

/**
 * Formats log message with structured data
 */
function formatLog(level: LogLevel, message: string, context?: LogContext): string {
  const timestamp = new Date().toISOString();
  const logData = {
    timestamp,
    level,
    message,
    ...context,
  };

  // In development, pretty print for readability
  if (process.env.NODE_ENV === 'development') {
    return JSON.stringify(logData, null, 2);
  }

  // In production, use compact JSON for log aggregation
  return JSON.stringify(logData);
}

/**
 * Convex-compatible logger
 */
export const logger = {
  debug(message: string, context?: LogContext) {
    if (shouldLog('debug')) {
      console.log(formatLog('debug', message, context));
    }
  },

  info(message: string, context?: LogContext) {
    if (shouldLog('info')) {
      console.log(formatLog('info', message, context));
    }
  },

  warn(message: string, context?: LogContext) {
    if (shouldLog('warn')) {
      console.warn(formatLog('warn', message, context));
    }
  },

  error(message: string, error?: Error | unknown, context?: LogContext) {
    if (shouldLog('error')) {
      const errorContext: LogContext = {
        ...context,
        event: context?.event || 'error',
      };

      if (error instanceof Error) {
        errorContext.errorName = error.name;
        errorContext.errorMessage = error.message;
        errorContext.errorStack = error.stack;
      } else if (error) {
        errorContext.error = error;
      }

      console.error(formatLog('error', message, errorContext));
    }
  },
};

/**
 * Create a logger with a specific context
 */
export function createLogger(defaultContext: Partial<LogContext>) {
  return {
    debug: (message: string, context?: LogContext) =>
      logger.debug(message, { ...defaultContext, ...context }),
    info: (message: string, context?: LogContext) =>
      logger.info(message, { ...defaultContext, ...context }),
    warn: (message: string, context?: LogContext) =>
      logger.warn(message, { ...defaultContext, ...context }),
    error: (message: string, error?: Error | unknown, context?: LogContext) =>
      logger.error(message, error, { ...defaultContext, ...context }),
  };
}

const BASE_CONCEPTS_CONTEXT: LogContext = {
  context: 'concepts',
  domains: ['concepts', 'ai', 'database'],
  deployment: DEFAULT_DEPLOYMENT,
};

export function createConceptsLogger(defaultContext: Partial<LogContext> = {}) {
  return createLogger({
    ...BASE_CONCEPTS_CONTEXT,
    ...defaultContext,
  });
}

type ConceptLogLevel = 'debug' | 'info' | 'warn' | 'error';

export type ConceptLogPhase =
  | 'stage_a'
  | 'stage_b'
  | 'iqc_scan'
  | 'iqc_apply'
  | 'embeddings_sync'
  | 'migration';

export type ConceptLogMetadata = LogContext & {
  phase: ConceptLogPhase;
  event: string;
  correlationId: string;
  conceptIds?: string[];
  actionCardId?: string;
};

type ConceptsLoggerInstance = ReturnType<typeof createConceptsLogger>;

export type ConceptsLogger = Pick<ConceptsLoggerInstance, ConceptLogLevel>;

export function logConceptEvent(
  structuredLogger: ConceptsLogger,
  level: ConceptLogLevel,
  message: string,
  metadata: ConceptLogMetadata
) {
  const eventName = metadata.event.startsWith('concepts.')
    ? metadata.event
    : `concepts.${metadata.phase}.${metadata.event}`;

  structuredLogger[level](message, {
    ...metadata,
    event: eventName,
  });
}

export function generateCorrelationId(prefix = 'concepts'): string {
  const randomSegment = Math.random().toString(36).slice(2, 8);
  return `${prefix}-${Date.now().toString(36)}-${randomSegment}`;
}

/**
 * Forward error to Sentry with context while preserving existing logging
 *
 * @param structuredLogger - Logger instance to use for structured logging
 * @param message - Contextual message describing what failed (e.g., "Failed to generate concept")
 * @param error - Error instance to forward
 * @param context - Additional context to attach as tags/extra data
 *
 * Usage:
 * ```typescript
 * try {
 *   await riskyOperation();
 * } catch (error) {
 *   errorWithSentry(logger, 'Failed to generate concept', error, {
 *     event: 'concepts.stage_a.generation_failed',
 *     correlationId: 'xyz-123',
 *     conceptIds: ['id1', 'id2']
 *   });
 * }
 * ```
 *
 * Side effects:
 * - Logs error via existing logger (always)
 * - Captures exception in Sentry when SENTRY_DSN configured (conditional)
 * - Attaches context as tags (runtime=convex) and extra metadata
 * - Returns void (fire-and-forget)
 */
export function errorWithSentry(
  structuredLogger: ConceptsLogger | ReturnType<typeof createLogger> | typeof logger,
  message: string,
  error?: Error | unknown,
  context?: LogContext
): void {
  // Always log via existing logger
  structuredLogger.error(message, error, context);

  // Forward to Sentry if DSN configured
  if (!process.env.SENTRY_DSN) return;

  Sentry.captureException(error, {
    tags: {
      runtime: 'convex',
      event: context?.event,
      context: context?.context,
    },
    extra: {
      ...context,
      deployment: context?.deployment || DEFAULT_DEPLOYMENT,
    },
  });
}
