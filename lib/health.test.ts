import { afterEach, describe, expect, it, vi } from 'vitest';
import { createHealthSnapshot } from './health';

describe('createHealthSnapshot', () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it('returns a minimal healthy snapshot', () => {
    const snapshot = createHealthSnapshot();

    expect(snapshot.status).toBe('healthy');
    expect(typeof snapshot.timestamp).toBe('string');
    expect(typeof snapshot.uptime).toBe('number');
    expect(snapshot.memory.total).toBeGreaterThan(0);
    expect(snapshot.memory.used).toBeGreaterThanOrEqual(0);
    expect(snapshot.environment).toBe(process.env.NODE_ENV || 'unknown');
    expect(snapshot.version.length).toBeGreaterThan(0);
  });

  it('falls back to defaults when environment metadata is missing', async () => {
    // Stub env vars to undefined to test fallback paths
    vi.stubEnv('NODE_ENV', undefined as unknown as string);
    vi.stubEnv('npm_package_version', undefined as unknown as string);

    // Need fresh import to re-evaluate environment reads
    vi.resetModules();
    const { createHealthSnapshot: freshSnapshot } = await import('./health');
    const snapshot = freshSnapshot();

    expect(snapshot.environment).toBe('unknown');
    expect(snapshot.version).toBe('0.1.0');
  });
});
