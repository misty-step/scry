import { createOpenRouter } from '@openrouter/ai-sdk-provider';
import type { Logger } from 'pino';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { initializeProvider } from './aiProviders';
import { getSecretDiagnostics } from './envDiagnostics';

vi.mock('@openrouter/ai-sdk-provider', () => ({
  createOpenRouter: vi.fn(),
}));

const mockCreateOpenRouter = vi.mocked(createOpenRouter);

const createLogger = (): Logger =>
  ({
    info: vi.fn(),
    error: vi.fn(),
  }) as unknown as Logger;

const originalEnv = { ...process.env };

describe('initializeProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    process.env = { ...originalEnv };
    delete process.env.OPENROUTER_API_KEY;
    process.env.CONVEX_CLOUD_URL = 'https://test.convex';
  });

  it('initializes OpenRouter provider with diagnostics and logging context', () => {
    process.env.OPENROUTER_API_KEY = 'openrouter-key';
    const mockModel = { id: 'google/gemini-3-flash-preview' } as any;
    mockCreateOpenRouter.mockReturnValue((() => mockModel) as any);
    const logger = createLogger();

    const result = initializeProvider('google/gemini-3-flash-preview', {
      logger,
      logContext: { configId: 'cfg-123' },
      deployment: 'https://custom.convex',
    });

    expect(mockCreateOpenRouter).toHaveBeenCalledWith({ apiKey: 'openrouter-key' });
    expect(result.model).toBe(mockModel);
    expect(result.diagnostics).toEqual(getSecretDiagnostics('openrouter-key'));

    expect(logger.info).toHaveBeenCalledWith(
      expect.objectContaining({
        configId: 'cfg-123',
        provider: 'openrouter',
        model: 'google/gemini-3-flash-preview',
        keyDiagnostics: getSecretDiagnostics('openrouter-key'),
        deployment: 'https://custom.convex',
      }),
      'Using OpenRouter provider'
    );
  });

  it('throws when OPENROUTER_API_KEY is missing and logs error diagnostics', () => {
    const logger = createLogger();

    expect(() =>
      initializeProvider('google/gemini-3-flash-preview', {
        logger,
      })
    ).toThrow('OPENROUTER_API_KEY not configured in Convex environment');

    expect(logger.error).toHaveBeenCalledWith(
      expect.objectContaining({ provider: 'openrouter' }),
      'OPENROUTER_API_KEY not configured in Convex environment'
    );
    expect(mockCreateOpenRouter).not.toHaveBeenCalled();
  });

  it('uses default deployment from CONVEX_CLOUD_URL when not provided', () => {
    process.env.OPENROUTER_API_KEY = 'openrouter-key';
    const mockModel = { id: 'google/gemini-3-flash-preview' } as any;
    mockCreateOpenRouter.mockReturnValue((() => mockModel) as any);
    const logger = createLogger();

    initializeProvider('google/gemini-3-flash-preview', { logger });

    expect(logger.info).toHaveBeenCalledWith(
      expect.objectContaining({
        deployment: 'https://test.convex',
      }),
      'Using OpenRouter provider'
    );
  });
});
