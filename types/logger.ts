export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface LogContext {
  correlationId?: string;
  event?: string;
  context?: string;
  [key: string]: unknown;
}

export interface ILogger {
  debug: (message: string, context?: LogContext) => void;
  info: (message: string, context?: LogContext) => void;
  warn: (message: string, context?: LogContext) => void;
  error: (message: string, error?: Error | unknown, context?: LogContext) => void;
}

export function generateCorrelationId(prefix = 'log'): string {
  let uuid: string;
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    // Browser or Node.js with Web Crypto API
    uuid = crypto.randomUUID();
  } else {
    // Legacy fallback - should never execute in modern Next.js (Node 16+/modern browsers)
    // Uses Math.random() which is not cryptographically secure; correlation ID collisions are more likely
    uuid = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
      const r = (Math.random() * 16) | 0;
      const v = c === 'x' ? r : (r & 0x3) | 0x8;
      return v.toString(16);
    });
  }
  return `${prefix}-${uuid}`;
}
