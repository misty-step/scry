import { beforeEach, describe, expect, it, vi } from 'vitest';

const fetchMock = vi.fn();

describe('canary transport', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.unstubAllEnvs();
    fetchMock.mockReset();
    vi.stubGlobal('fetch', fetchMock);
  });

  it('returns disabled when no Canary credentials are configured', async () => {
    vi.stubEnv('CANARY_API_KEY', '');
    vi.stubEnv('NEXT_PUBLIC_CANARY_API_KEY', '');

    const { captureCanaryException } = await import('./canary');

    await expect(captureCanaryException(new Error('boom'))).resolves.toEqual({
      status: 'disabled',
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('reports sanitized errors to Canary when configured', async () => {
    vi.stubEnv('CANARY_ENDPOINT', 'https://canary-obs.fly.dev');
    vi.stubEnv('CANARY_API_KEY', 'sk_live_test');

    fetchMock.mockResolvedValue({
      ok: true,
      json: async () => ({
        id: 'ERR-123',
        group_hash: 'hash-123',
        is_new_class: true,
      }),
    });

    const { captureCanaryException } = await import('./canary');
    const result = await captureCanaryException(new Error('boom for user@example.com'), {
      context: {
        owner: 'support@example.com',
      },
    });

    expect(result).toEqual({
      status: 'sent',
      response: {
        id: 'ERR-123',
        group_hash: 'hash-123',
        is_new_class: true,
      },
    });
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('https://canary-obs.fly.dev/api/v1/errors');

    const body = JSON.parse(String(init.body));
    expect(body.service).toBe('scry');
    expect(body.message).toBe('boom for [EMAIL_REDACTED]');
    expect(body.context).toEqual({ owner: '[EMAIL_REDACTED]' });
  });

  it('prefers server credentials when both server and browser keys are present', async () => {
    vi.stubEnv('NEXT_RUNTIME', 'nodejs');
    vi.stubEnv('CANARY_ENDPOINT', 'https://server-canary.example');
    vi.stubEnv('CANARY_API_KEY', 'sk_server_test');
    vi.stubEnv('NEXT_PUBLIC_CANARY_ENDPOINT', 'https://browser-canary.example');
    vi.stubEnv('NEXT_PUBLIC_CANARY_API_KEY', 'sk_browser_test');

    fetchMock.mockResolvedValue({
      ok: true,
      json: async () => ({
        id: 'ERR-456',
        group_hash: 'hash-456',
        is_new_class: false,
      }),
    });

    const { captureCanaryException } = await import('./canary');
    await captureCanaryException(new Error('boom'));

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('https://server-canary.example/api/v1/errors');
    expect(init.headers).toMatchObject({
      Authorization: 'Bearer sk_server_test',
    });
  });

  it('drops aborted connection reset errors', async () => {
    vi.stubEnv('CANARY_ENDPOINT', 'https://canary-obs.fly.dev');
    vi.stubEnv('CANARY_API_KEY', 'sk_live_test');

    const { captureCanaryException } = await import('./canary');
    const error = Object.assign(new Error('aborted'), { code: 'ECONNRESET' });

    await expect(captureCanaryException(error)).resolves.toEqual({ status: 'ignored' });

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('returns a failed result when Canary rejects the payload', async () => {
    vi.stubEnv('CANARY_ENDPOINT', 'https://canary-obs.fly.dev');
    vi.stubEnv('CANARY_API_KEY', 'sk_live_test');

    fetchMock.mockResolvedValue({
      ok: false,
      status: 503,
    });

    const { captureCanaryException } = await import('./canary');

    await expect(captureCanaryException(new Error('boom'))).resolves.toEqual({
      status: 'failed',
      failure: {
        reason: 'http_error',
        statusCode: 503,
      },
    });
  });
});
