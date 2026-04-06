const EMAIL_REDACTION_PATTERN =
  /[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(?<!\[EMAIL_REDACTED\])/g;
const DEFAULT_CANARY_ENDPOINT = 'https://canary-obs.fly.dev';
const DEFAULT_CANARY_SERVICE = 'scry';

export type CanarySeverity = 'error' | 'warning' | 'info';

export interface CanaryCaptureOptions {
  severity?: CanarySeverity;
  context?: Record<string, unknown>;
  fingerprint?: string[];
}

interface CanaryConfig {
  apiKey: string;
  endpoint: string;
  environment: string;
  service: string;
}

interface CanaryErrorPayload {
  context?: Record<string, unknown>;
  error_class: string;
  fingerprint?: string[];
  message: string;
  severity: CanarySeverity;
  stack_trace?: string;
}

export interface CanaryResponse {
  group_hash: string;
  id: string;
  is_new_class: boolean;
}

export type CanaryCaptureResult =
  | { status: 'disabled' }
  | { status: 'ignored' }
  | { status: 'sent'; response: CanaryResponse }
  | {
      status: 'failed';
      failure: {
        message?: string;
        reason: 'http_error' | 'network_error';
        statusCode?: number;
      };
    };

function scrubString(value: string): string {
  return value.replace(EMAIL_REDACTION_PATTERN, '[EMAIL_REDACTED]');
}

function scrubValue<T>(value: T): T {
  if (typeof value === 'string') {
    return scrubString(value) as T;
  }

  if (Array.isArray(value)) {
    return value.map((entry) => scrubValue(entry)) as T;
  }

  if (value && typeof value === 'object') {
    const next: Record<string, unknown> = {};
    for (const [key, entry] of Object.entries(value as Record<string, unknown>)) {
      next[key] = scrubValue(entry);
    }
    return next as T;
  }

  return value;
}

function normalizeError(error: unknown): {
  errorClass: string;
  message: string;
  stackTrace?: string;
} {
  if (error instanceof Error) {
    return {
      errorClass: error.constructor.name || 'Error',
      message: error.message,
      stackTrace: error.stack,
    };
  }

  if (typeof error === 'string') {
    return {
      errorClass: 'StringError',
      message: error,
    };
  }

  return {
    errorClass: 'UnknownError',
    message: String(error),
  };
}

function resolveCanaryConfig(): CanaryConfig | null {
  const endpoint =
    process.env.NEXT_PUBLIC_CANARY_ENDPOINT ||
    process.env.CANARY_ENDPOINT ||
    DEFAULT_CANARY_ENDPOINT;
  const apiKey = process.env.NEXT_PUBLIC_CANARY_API_KEY || process.env.CANARY_API_KEY;

  if (!endpoint || !apiKey) {
    return null;
  }

  return {
    apiKey,
    endpoint: endpoint.replace(/\/$/, ''),
    environment:
      process.env.CANARY_ENVIRONMENT ||
      process.env.NEXT_PUBLIC_CANARY_ENVIRONMENT ||
      process.env.VERCEL_ENV ||
      process.env.NODE_ENV ||
      'production',
    service:
      process.env.NEXT_PUBLIC_CANARY_SERVICE ||
      process.env.CANARY_SERVICE ||
      DEFAULT_CANARY_SERVICE,
  };
}

export function isCanaryConfigured(): boolean {
  return resolveCanaryConfig() !== null;
}

function shouldIgnoreError(error: unknown): boolean {
  if (!(error instanceof Error)) {
    return false;
  }

  const code =
    typeof (error as { code?: unknown }).code === 'string'
      ? ((error as { code?: string }).code ?? '')
      : '';
  const message = error.message.toLowerCase();

  return code === 'ECONNRESET' || message === 'aborted' || message.includes('econnreset');
}

async function sendToCanary(
  config: CanaryConfig,
  payload: CanaryErrorPayload
): Promise<CanaryCaptureResult> {
  const signal = typeof AbortSignal !== 'undefined' ? AbortSignal.timeout(2_000) : undefined;
  const response = await fetch(`${config.endpoint}/api/v1/errors`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${config.apiKey}`,
    },
    body: JSON.stringify({
      service: config.service,
      environment: config.environment,
      ...payload,
    }),
    signal,
  });

  if (!response.ok) {
    return {
      status: 'failed',
      failure: {
        reason: 'http_error',
        statusCode: response.status,
      },
    };
  }

  return {
    status: 'sent',
    response: (await response.json()) as CanaryResponse,
  };
}

export async function captureCanaryException(
  error: unknown,
  options: CanaryCaptureOptions = {}
): Promise<CanaryCaptureResult> {
  if (shouldIgnoreError(error)) {
    return { status: 'ignored' };
  }

  const config = resolveCanaryConfig();
  if (!config) {
    return { status: 'disabled' };
  }

  const normalized = normalizeError(error);

  try {
    return await sendToCanary(config, {
      context: options.context ? scrubValue(options.context) : undefined,
      error_class: normalized.errorClass,
      fingerprint: options.fingerprint,
      message: scrubString(normalized.message),
      severity: options.severity ?? 'error',
      stack_trace: normalized.stackTrace ? scrubString(normalized.stackTrace) : undefined,
    });
  } catch (captureError) {
    return {
      status: 'failed',
      failure: {
        reason: 'network_error',
        message: captureError instanceof Error ? captureError.message : String(captureError),
      },
    };
  }
}

function extractRequestInfo(request: unknown): Record<string, unknown> {
  if (!request || typeof request !== 'object') {
    return {};
  }

  const record = request as Record<string, unknown>;
  const context: Record<string, unknown> = {};

  if (typeof record.path === 'string') {
    context.path = record.path;
  } else if (typeof record.url === 'string') {
    context.path = record.url;
  }

  if (typeof record.method === 'string') {
    context.method = record.method;
  }

  return context;
}

export async function captureCanaryRequestError(
  error: unknown,
  request?: unknown,
  options: CanaryCaptureOptions = {}
): Promise<CanaryCaptureResult> {
  return captureCanaryException(error, {
    ...options,
    context: {
      ...extractRequestInfo(request),
      ...options.context,
    },
  });
}
