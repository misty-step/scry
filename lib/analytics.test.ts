import { beforeEach, describe, expect, it, vi } from 'vitest';

const clientTrackMock = vi.fn();
const serverTrackMock = vi.fn().mockResolvedValue(undefined);
const captureExceptionMock = vi.fn();
const setUserMock = vi.fn();

vi.mock('@vercel/analytics', () => ({
  track: clientTrackMock,
}));

vi.mock('@vercel/analytics/server', () => ({
  track: serverTrackMock,
}));

vi.mock('@sentry/nextjs', () => ({
  captureException: captureExceptionMock,
  setUser: setUserMock,
}));

describe('analytics wrapper', () => {
  beforeEach(() => {
    // Fully reset mocks to initial state
    clientTrackMock.mockReset();
    serverTrackMock.mockReset().mockResolvedValue(undefined);
    captureExceptionMock.mockReset();
    setUserMock.mockReset();

    vi.unstubAllEnvs();
  });

  it('skips tracking in development environment by default', async () => {
    vi.stubEnv('NODE_ENV', 'development');
    vi.stubEnv('VERCEL_ENV', 'development');

    vi.resetModules();
    const { trackEvent } = await import('./analytics');

    trackEvent('Quiz Generation Started', { jobId: 'job-123' });

    expect(clientTrackMock).not.toHaveBeenCalled();
  });

  it('does not throw when the analytics SDK throws', async () => {
    vi.stubEnv('NODE_ENV', 'production');
    vi.stubEnv('VERCEL_ENV', 'production');

    clientTrackMock.mockImplementation(() => {
      throw new Error('sdk failure');
    });

    vi.resetModules();
    const { trackEvent } = await import('./analytics');

    expect(() =>
      trackEvent('Quiz Generation Started', {
        jobId: 'job-123',
      })
    ).not.toThrow();
  });

  it('skips Sentry reporting when disabled', async () => {
    vi.stubEnv('NODE_ENV', 'test');
    delete process.env.SENTRY_DSN;
    delete process.env.NEXT_PUBLIC_SENTRY_DSN;

    vi.resetModules();
    const { reportError } = await import('./analytics');

    reportError(new Error('noop'));

    expect(captureExceptionMock).not.toHaveBeenCalled();
  });
});
