import { beforeEach, describe, expect, it, vi } from 'vitest';

describe('shouldEnableSentry', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.unstubAllEnvs();
  });

  it('disables Sentry in development by default', async () => {
    vi.stubEnv('NODE_ENV', 'development');

    const { shouldEnableSentry } = await import('./sentry');
    expect(shouldEnableSentry('https://dsn.example')).toBe(false);
  });

  it('allows development reporting when explicitly enabled', async () => {
    vi.stubEnv('NODE_ENV', 'development');
    vi.stubEnv('SENTRY_ENABLE_DEV', 'true');

    const { shouldEnableSentry } = await import('./sentry');
    expect(shouldEnableSentry('https://dsn.example')).toBe(true);
  });

  it('disables Sentry when Canary is configured', async () => {
    vi.stubEnv('NODE_ENV', 'production');
    vi.stubEnv('CANARY_ENDPOINT', 'https://canary-obs.fly.dev');
    vi.stubEnv('CANARY_API_KEY', 'sk_live_test');

    const { shouldEnableSentry } = await import('./sentry');
    expect(shouldEnableSentry('https://dsn.example')).toBe(false);
  });

  it('allows Sentry emergency fallback when Canary is configured', async () => {
    vi.stubEnv('NODE_ENV', 'production');
    vi.stubEnv('CANARY_ENDPOINT', 'https://canary-obs.fly.dev');
    vi.stubEnv('CANARY_API_KEY', 'sk_live_test');

    const { shouldEnableSentryFallback } = await import('./sentry');
    expect(shouldEnableSentryFallback('https://dsn.example')).toBe(true);
  });
});
