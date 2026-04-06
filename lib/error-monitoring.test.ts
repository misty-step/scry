import { beforeEach, describe, expect, it, vi } from 'vitest';

const captureCanaryExceptionMock = vi.fn();
const captureCanaryRequestErrorMock = vi.fn();
const captureExceptionMock = vi.fn();
const captureRequestErrorMock = vi.fn();
const initMock = vi.fn();
const isEnabledMock = vi.fn();

vi.mock('./canary', () => ({
  captureCanaryException: (...args: unknown[]) => captureCanaryExceptionMock(...args),
  captureCanaryRequestError: (...args: unknown[]) => captureCanaryRequestErrorMock(...args),
}));

vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => captureExceptionMock(...args),
  captureRequestError: (...args: unknown[]) => captureRequestErrorMock(...args),
  init: (...args: unknown[]) => initMock(...args),
  isEnabled: (...args: unknown[]) => isEnabledMock(...args),
}));

describe('error monitoring', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.unstubAllEnvs();
    captureCanaryExceptionMock.mockReset();
    captureCanaryRequestErrorMock.mockReset();
    captureExceptionMock.mockReset();
    captureRequestErrorMock.mockReset();
    initMock.mockReset();
    isEnabledMock.mockReset();
    isEnabledMock.mockReturnValue(false);
  });

  it('does not touch Sentry when Canary capture succeeds', async () => {
    vi.stubEnv('NODE_ENV', 'production');
    vi.stubEnv('SENTRY_DSN', 'https://dsn.example');
    captureCanaryExceptionMock.mockResolvedValue({
      status: 'sent',
      response: {
        id: 'ERR-123',
        group_hash: 'hash-123',
        is_new_class: true,
      },
    });

    const { captureRuntimeException } = await import('./error-monitoring');
    await captureRuntimeException(new Error('boom'));

    expect(initMock).not.toHaveBeenCalled();
    expect(captureExceptionMock).not.toHaveBeenCalled();
  });

  it('falls back to Sentry when Canary transport fails', async () => {
    vi.stubEnv('NODE_ENV', 'production');
    vi.stubEnv('SENTRY_DSN', 'https://dsn.example');
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    captureCanaryExceptionMock.mockResolvedValue({
      status: 'failed',
      failure: {
        reason: 'http_error',
        statusCode: 503,
      },
    });

    const { captureRuntimeException } = await import('./error-monitoring');
    await captureRuntimeException(new Error('boom'), {
      context: { route: '/' },
    });

    expect(consoleErrorSpy).toHaveBeenCalledWith(
      '[monitoring] Canary capture failed',
      expect.objectContaining({
        kind: 'exception',
        reason: 'http_error',
        statusCode: 503,
      })
    );
    expect(initMock).toHaveBeenCalledOnce();
    expect(captureExceptionMock).toHaveBeenCalledOnce();

    consoleErrorSpy.mockRestore();
  });

  it('keeps ignored errors out of Sentry', async () => {
    vi.stubEnv('NODE_ENV', 'production');
    vi.stubEnv('SENTRY_DSN', 'https://dsn.example');
    captureCanaryExceptionMock.mockResolvedValue({ status: 'ignored' });

    const { captureRuntimeException } = await import('./error-monitoring');
    await captureRuntimeException(Object.assign(new Error('aborted'), { code: 'ECONNRESET' }));

    expect(initMock).not.toHaveBeenCalled();
    expect(captureExceptionMock).not.toHaveBeenCalled();
  });

  it('falls back request errors to Sentry when Canary request capture fails', async () => {
    vi.stubEnv('NODE_ENV', 'production');
    vi.stubEnv('SENTRY_DSN', 'https://dsn.example');
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    captureCanaryRequestErrorMock.mockResolvedValue({
      status: 'failed',
      failure: {
        reason: 'network_error',
        message: 'timeout',
      },
    });

    const { captureRuntimeRequestError } = await import('./error-monitoring');
    const error = new Error('boom');
    const request = { method: 'GET', url: '/healthz' };
    await captureRuntimeRequestError(error, request, { routeType: 'render' });

    expect(initMock).toHaveBeenCalledOnce();
    expect(captureRequestErrorMock).toHaveBeenCalledOnce();
    expect(captureRequestErrorMock).toHaveBeenCalledWith(error, request, { routeType: 'render' });
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      '[monitoring] Canary capture failed',
      expect.objectContaining({
        kind: 'request',
        message: 'timeout',
        reason: 'network_error',
      })
    );

    consoleErrorSpy.mockRestore();
  });
});
