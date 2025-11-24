import { describe, expect, it } from 'vitest';
import { createHealthSnapshot } from './health';

describe('createHealthSnapshot', () => {
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
});
