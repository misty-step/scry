import { describe, it } from 'node:test';
import { strictEqual } from 'node:assert';
import { classifyOrigin, Budget } from '../lib/guards.mjs';

describe('guards', () => {
  it('classifyOrigin rejects production origins for mutating modes', () => {
    const r = classifyOrigin('https://scry.study', { mutating: true });
    strictEqual(r.ok, false);
    strictEqual(r.kind, 'production');
  });

  it('classifyOrigin accepts loopback', () => {
    const r = classifyOrigin('http://127.0.0.1:18080');
    strictEqual(r.ok, true);
    strictEqual(r.kind, 'loopback');
  });

  it('classifyOrigin requires --allow-origin for non-loopback mutating', () => {
    const r = classifyOrigin('http://example.com', { mutating: true });
    strictEqual(r.ok, false);
    strictEqual(r.kind, 'non-loopback');
  });

  it('classifyOrigin accepts non-loopback with valid --allow-origin', () => {
    const r = classifyOrigin('http://example.com', { mutating: true, allowOrigin: 'http://example.com' });
    strictEqual(r.ok, true);
  });

  it('Budget tracks steps', () => {
    const b = new Budget({ maxSteps: 3 });
    strictEqual(b.checkStep().ok, true);
    strictEqual(b.checkStep().ok, true);
    strictEqual(b.checkStep().ok, true);
    strictEqual(b.checkStep().ok, false);
  });

  it('Budget checks timeout', () => {
    const b = new Budget({ timeoutS: 0.0001 });
    // Give it time to pass the threshold
    const start = Date.now();
    while ((Date.now() - start) < 5); // busy wait
    strictEqual(b.checkTimeout().ok, false);
  });

  it('Budget toJSON', () => {
    const b = new Budget({ maxSteps: 24, timeoutS: 120, maxScreenshots: 12 });
    const j = b.toJSON();
    strictEqual(j.maxSteps, 24);
    strictEqual(j.timeoutS, 120);
  });
});
