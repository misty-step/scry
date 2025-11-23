import { vi } from 'vitest';

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export type LogCall = {
  level: LogLevel;
  message: unknown;
  context?: unknown;
};

/**
 * Minimal structured logger stub that records calls for assertions.
 */
export function createLoggerStub() {
  const calls: LogCall[] = [];

  const recordLog = (level: Exclude<LogLevel, 'error'>) =>
    vi.fn((message: string, context?: unknown) => {
      calls.push({ level, message, context });
    });

  const recordError = () =>
    vi.fn((message: string, error?: Error | unknown, context?: unknown) => {
      calls.push({
        level: 'error',
        message,
        context: context ? { ...context, error } : { error },
      });
    });

  const logger = {
    debug: recordLog('debug'),
    info: recordLog('info'),
    warn: recordLog('warn'),
    error: recordError(),
    /**
     * Returns all recorded calls, optionally filtered by level.
     */
    getCalls(level?: LogLevel) {
      return level ? calls.filter((c) => c.level === level) : calls;
    },
    reset() {
      calls.splice(0, calls.length);
    },
  };

  return logger;
}
