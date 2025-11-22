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

  const record = (level: LogLevel) =>
    vi.fn((message: unknown, context?: unknown) => {
      calls.push({ level, message, context });
    });

  const logger = {
    debug: record('debug'),
    info: record('info'),
    warn: record('warn'),
    error: record('error'),
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
