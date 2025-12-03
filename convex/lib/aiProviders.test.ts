import { createGoogleGenerativeAI } from '@ai-sdk/google';
import type { Logger } from 'pino';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { initializeGoogleProvider } from './aiProviders';
import { getSecretDiagnostics } from './envDiagnostics';

vi.mock('@ai-sdk/google', () => ({
  createGoogleGenerativeAI: vi.fn(),
}));

const mockCreateGoogleGenerativeAI = vi.mocked(createGoogleGenerativeAI);

const createLogger = (): Logger =>
  ({
    info: vi.fn(),
    error: vi.fn(),
  }) as unknown as Logger;

const originalEnv = { ...process.env };

describe('initializeGoogleProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    process.env = { ...originalEnv };
    delete process.env.GOOGLE_AI_API_KEY;
    process.env.CONVEX_CLOUD_URL = 'https://test.convex';
  });

  it('initializes Google provider with diagnostics and logging context', () => {
    process.env.GOOGLE_AI_API_KEY = 'google-key';
    const mockModel = { id: 'gemini-3-pro-preview' } as any;
    mockCreateGoogleGenerativeAI.mockReturnValue((() => mockModel) as any);
    const logger = createLogger();

    const result = initializeGoogleProvider('gemini-3-pro-preview', {
      logger,
      logContext: { configId: 'cfg-123' },
      deployment: 'https://custom.convex',
    });

    expect(mockCreateGoogleGenerativeAI).toHaveBeenCalledWith({ apiKey: 'google-key' });
    expect(result.model).toBe(mockModel);
    expect(result.diagnostics).toEqual(getSecretDiagnostics('google-key'));

    expect(logger.info).toHaveBeenCalledWith(
      expect.objectContaining({
        configId: 'cfg-123',
        provider: 'google',
        model: 'gemini-3-pro-preview',
        keyDiagnostics: getSecretDiagnostics('google-key'),
        deployment: 'https://custom.convex',
      }),
      'Using Google AI provider'
    );
  });

  it('throws when GOOGLE_AI_API_KEY is missing and logs error diagnostics', () => {
    const logger = createLogger();

    expect(() =>
      initializeGoogleProvider('gemini-3-pro-preview', {
        logger,
      })
    ).toThrow('GOOGLE_AI_API_KEY not configured in Convex environment');

    expect(logger.error).toHaveBeenCalledWith(
      expect.objectContaining({ provider: 'google' }),
      'GOOGLE_AI_API_KEY not configured in Convex environment'
    );
    expect(mockCreateGoogleGenerativeAI).not.toHaveBeenCalled();
  });

  it('uses default deployment from CONVEX_CLOUD_URL when not provided', () => {
    process.env.GOOGLE_AI_API_KEY = 'google-key';
    const mockModel = { id: 'gemini-3-pro-preview' } as any;
    mockCreateGoogleGenerativeAI.mockReturnValue((() => mockModel) as any);
    const logger = createLogger();

    initializeGoogleProvider('gemini-3-pro-preview', { logger });

    expect(logger.info).toHaveBeenCalledWith(
      expect.objectContaining({
        deployment: 'https://test.convex',
      }),
      'Using Google AI provider'
    );
  });
});
