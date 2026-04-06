import { captureCanaryException, captureCanaryRequestError } from './canary';
import {
  createSentryOptions,
  shouldEnableSentry,
  shouldEnableSentryFallback,
  type SentryTarget,
} from './sentry';

type SentryModule = typeof import('@sentry/nextjs');

export interface RuntimeCaptureOptions {
  context?: Record<string, unknown>;
  fingerprint?: string[];
  severity?: 'error' | 'warning' | 'info';
}

let sentryPromise: Promise<SentryModule | null> | null = null;

function resolveSentryDsn(): string | undefined {
  return process.env.SENTRY_DSN || process.env.NEXT_PUBLIC_SENTRY_DSN;
}

function loadSentry(): Promise<SentryModule | null> {
  if (!sentryPromise) {
    const isBrowser = typeof window !== 'undefined';
    const isTest = process.env.NODE_ENV === 'test';
    const importPromise = isTest
      ? import('@sentry/nextjs')
      : isBrowser
        ? import('@sentry/nextjs')
        : import(/* webpackIgnore: true */ ('@sentry' + '/' + 'nextjs') as '@sentry/nextjs');

    sentryPromise = importPromise.then((module) => module).catch(() => null);
  }

  return sentryPromise;
}

function resolveSentryTarget(): SentryTarget {
  if (typeof window !== 'undefined') {
    return 'client';
  }

  if (process.env.NEXT_RUNTIME === 'edge') {
    return 'edge';
  }

  return 'server';
}

function logCanaryFailure(
  kind: 'exception' | 'request',
  failure: {
    message?: string;
    reason: 'http_error' | 'network_error';
    statusCode?: number;
  }
): void {
  console.error('[monitoring] Canary capture failed', {
    kind,
    message: failure.message,
    reason: failure.reason,
    statusCode: failure.statusCode,
  });
}

async function ensureSentryInitialized(
  Sentry: SentryModule,
  mode: 'default' | 'fallback'
): Promise<boolean> {
  const options = createSentryOptions(resolveSentryTarget(), mode);

  if (!options.enabled) {
    return false;
  }

  if (!Sentry.isEnabled()) {
    Sentry.init(options);
  }

  return true;
}

export async function captureRuntimeException(
  error: unknown,
  options: RuntimeCaptureOptions = {}
): Promise<void> {
  const canaryResult = await captureCanaryException(error, options);
  if (canaryResult.status === 'sent' || canaryResult.status === 'ignored') {
    return;
  }

  const dsn = resolveSentryDsn();
  const sentryMode = canaryResult.status === 'failed' ? 'fallback' : 'default';
  const canUseSentry =
    sentryMode === 'fallback' ? shouldEnableSentryFallback(dsn) : shouldEnableSentry(dsn);

  if (canaryResult.status === 'failed') {
    logCanaryFailure('exception', canaryResult.failure);
  }

  if (!canUseSentry) {
    return;
  }

  const Sentry = await loadSentry();
  if (!Sentry || !(await ensureSentryInitialized(Sentry, sentryMode))) {
    return;
  }

  Sentry.captureException(
    error instanceof Error ? error : new Error(typeof error === 'string' ? error : String(error)),
    options.context ? { extra: options.context } : undefined
  );
}

export async function captureRuntimeRequestError(...args: unknown[]): Promise<void> {
  const canaryResult = await captureCanaryRequestError(args[0], args[1]);
  if (canaryResult.status === 'sent' || canaryResult.status === 'ignored') {
    return;
  }

  const dsn = resolveSentryDsn();
  const sentryMode = canaryResult.status === 'failed' ? 'fallback' : 'default';
  const canUseSentry =
    sentryMode === 'fallback' ? shouldEnableSentryFallback(dsn) : shouldEnableSentry(dsn);

  if (canaryResult.status === 'failed') {
    logCanaryFailure('request', canaryResult.failure);
  }

  if (!canUseSentry) {
    return;
  }

  const Sentry = await loadSentry();
  if (!Sentry?.captureRequestError || !(await ensureSentryInitialized(Sentry, sentryMode))) {
    return;
  }

  await Sentry.captureRequestError(...(args as Parameters<typeof Sentry.captureRequestError>));
}
