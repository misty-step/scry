/**
 * Tests for Convex logger with Sentry integration
 */

import * as Sentry from '@sentry/nextjs';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createLogger, errorWithSentry, logger, type LogContext } from './logger';

// Mock Sentry module
vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(),
}));

describe('errorWithSentry', () => {
  const originalEnv = process.env;

  beforeEach(() => {
    vi.clearAllMocks();
    // Reset environment before each test
    process.env = { ...originalEnv };
  });

  afterEach(() => {
    // Restore original environment
    process.env = originalEnv;
  });

  it('logs error via existing logger when SENTRY_DSN not set', () => {
    // Arrange
    delete process.env.SENTRY_DSN;
    const mockLogger = {
      error: vi.fn(),
    };
    const testError = new Error('Test error');
    const context: LogContext = {
      event: 'test.event',
      correlationId: 'test-123',
    };

    // Act
    errorWithSentry(mockLogger, 'Operation failed', testError, context);

    // Assert
    expect(mockLogger.error).toHaveBeenCalledWith('Operation failed', testError, context);
    expect(Sentry.captureException).not.toHaveBeenCalled();
  });

  it('forwards error to Sentry when SENTRY_DSN is set', () => {
    // Arrange
    process.env.SENTRY_DSN = 'https://test@sentry.io/123';
    const mockLogger = {
      error: vi.fn(),
    };
    const testError = new Error('Sentry error');
    const context: LogContext = {
      event: 'concepts.stage_a.failed',
      context: 'concepts',
      correlationId: 'xyz-456',
    };

    // Act
    errorWithSentry(mockLogger, 'Failed to generate concept', testError, context);

    // Assert
    expect(mockLogger.error).toHaveBeenCalledWith('Failed to generate concept', testError, context);
    expect(Sentry.captureException).toHaveBeenCalledWith(testError, {
      tags: {
        runtime: 'convex',
        event: 'concepts.stage_a.failed',
        context: 'concepts',
      },
      extra: expect.objectContaining({
        event: 'concepts.stage_a.failed',
        context: 'concepts',
        correlationId: 'xyz-456',
      }),
    });
  });

  it('includes deployment in extra data', () => {
    // Arrange
    process.env.SENTRY_DSN = 'https://test@sentry.io/123';
    const mockLogger = {
      error: vi.fn(),
    };
    const testError = new Error('Deploy error');
    const context: LogContext = {
      event: 'deploy.failed',
      deployment: 'https://custom.convex.cloud',
    };

    // Act
    errorWithSentry(mockLogger, 'Deployment failed', testError, context);

    // Assert
    expect(Sentry.captureException).toHaveBeenCalledWith(
      testError,
      expect.objectContaining({
        extra: expect.objectContaining({
          deployment: 'https://custom.convex.cloud',
        }),
      })
    );
  });

  it('handles non-Error objects', () => {
    // Arrange
    process.env.SENTRY_DSN = 'https://test@sentry.io/123';
    const mockLogger = {
      error: vi.fn(),
    };
    const nonError = { message: 'Not an error' };
    const context: LogContext = {
      event: 'non-error.event',
    };

    // Act
    errorWithSentry(mockLogger, 'Unexpected failure', nonError, context);

    // Assert
    expect(mockLogger.error).toHaveBeenCalledWith('Unexpected failure', nonError, context);
    expect(Sentry.captureException).toHaveBeenCalledWith(nonError, expect.any(Object));
  });

  it('preserves existing logger behavior', () => {
    // Arrange
    process.env.SENTRY_DSN = 'https://test@sentry.io/123';
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const testError = new Error('Logger test');
    const context: LogContext = {
      event: 'logger.test',
    };

    // Act
    errorWithSentry(logger, 'Test operation failed', testError, context);

    // Assert
    expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('Test operation failed'));
    expect(Sentry.captureException).toHaveBeenCalled();

    // Cleanup
    consoleSpy.mockRestore();
  });

  it('works with createLogger instances', () => {
    // Arrange
    process.env.SENTRY_DSN = 'https://test@sentry.io/123';
    const contextLogger = createLogger({ context: 'test-module' });
    const testError = new Error('Module error');
    const additionalContext: LogContext = {
      event: 'module.failed',
      context: 'test-module', // Must explicitly include merged context
      correlationId: 'mod-789',
    };

    // Act
    errorWithSentry(contextLogger, 'Module initialization failed', testError, additionalContext);

    // Assert
    expect(Sentry.captureException).toHaveBeenCalledWith(
      testError,
      expect.objectContaining({
        tags: expect.objectContaining({
          runtime: 'convex',
          event: 'module.failed',
          context: 'test-module',
        }),
        extra: expect.objectContaining({
          context: 'test-module',
          correlationId: 'mod-789',
        }),
      })
    );
  });

  it('captures exception exactly once per call', () => {
    // Arrange
    process.env.SENTRY_DSN = 'https://test@sentry.io/123';
    const mockLogger = {
      error: vi.fn(),
    };
    const testError = new Error('Single capture');

    // Act
    errorWithSentry(mockLogger, 'Single operation failed', testError);

    // Assert
    expect(Sentry.captureException).toHaveBeenCalledTimes(1);
  });

  it('does not capture when DSN is empty string', () => {
    // Arrange
    process.env.SENTRY_DSN = '';
    const mockLogger = {
      error: vi.fn(),
    };
    const testError = new Error('No capture');

    // Act
    errorWithSentry(mockLogger, 'No capture operation', testError);

    // Assert
    expect(mockLogger.error).toHaveBeenCalled();
    expect(Sentry.captureException).not.toHaveBeenCalled();
  });

  it('includes all context metadata as extra data', () => {
    // Arrange
    process.env.SENTRY_DSN = 'https://test@sentry.io/123';
    const mockLogger = {
      error: vi.fn(),
    };
    const testError = new Error('Metadata test');
    const richContext: LogContext = {
      event: 'rich.event',
      context: 'test',
      correlationId: 'rich-123',
      conceptIds: ['id1', 'id2'],
      actionCardId: 'card-456',
      domains: ['concepts', 'ai'],
      customField: 'custom-value',
    };

    // Act
    errorWithSentry(mockLogger, 'Operation with rich context failed', testError, richContext);

    // Assert
    expect(Sentry.captureException).toHaveBeenCalledWith(
      testError,
      expect.objectContaining({
        extra: expect.objectContaining({
          event: 'rich.event',
          context: 'test',
          correlationId: 'rich-123',
          conceptIds: ['id1', 'id2'],
          actionCardId: 'card-456',
          domains: ['concepts', 'ai'],
          customField: 'custom-value',
        }),
      })
    );
  });
});
