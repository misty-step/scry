import { describe, it } from 'node:test';
import { strictEqual } from 'node:assert';
import { classifyOrigin, Budget } from '../lib/guards.mjs';

describe('guards', () => {
  for (const host of ['scry.study', 'www.scry.study', 'scry.mistystep.io']) {
    for (const suffix of ['..', '%2e%2e', '.%2e', '%2e.']) {
      const origin = `https://${host}${suffix}`;
      for (const mutating of [false, true]) {
        for (const allowOrigin of [null, new URL(origin).origin]) {
          it(`refuses malformed ${origin}, mutating=${mutating}, allowOrigin=${allowOrigin}`, () => {
            const result = classifyOrigin(origin, { mutating, allowOrigin });
            strictEqual(result.ok, false);
            strictEqual(result.kind, 'invalid');
          });
        }
      }
    }
    for (const hostname of [host, host.toUpperCase()]) {
      for (const suffix of ['', '.']) {
        const origin = `https://${hostname}${suffix}`;
        for (const allowOrigin of [null, new URL(origin).origin]) {
          it(`classifyOrigin refuses ${origin} for mutating modes with allowOrigin=${allowOrigin}`, () => {
            const r = classifyOrigin(origin, { mutating: true, allowOrigin });
            strictEqual(r.ok, false);
            strictEqual(r.kind, 'production');
          });
        }
      }
    }
  }

  it('classifyOrigin accepts loopback', () => {
    const r = classifyOrigin('http://127.0.0.1:18080');
    strictEqual(r.ok, true);
    strictEqual(r.kind, 'loopback');
  });

  it('rejects empty labels in synthetic hosts and preserves loopback', () => {
    for (const host of ['.example.invalid', 'example..invalid', 'localhost..', 'localhost%2e%2e']) {
      strictEqual(classifyOrigin(`http://${host}`, { mutating: false }).ok, false);
    }
    for (const host of ['localhost', '127.0.0.1']) {
      strictEqual(classifyOrigin(`http://${host}:18080`, { mutating: true }).kind, 'loopback');
    }
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

  it('classifyOrigin rejects a non-matching --allow-origin', () => {
    const r = classifyOrigin('http://example.com', { mutating: true, allowOrigin: 'http://other.example' });
    strictEqual(r.ok, false);
    strictEqual(r.kind, 'non-loopback');
  });

  it('Budget reports configured limits and used counters', () => {
    const b = new Budget({ maxSteps: 5, timeoutS: 10, maxScreenshots: 2 });
    b.checkStep();
    b.checkScreenshot();
    const j = b.toJSON();
    strictEqual(j.maxSteps, 5);
    strictEqual(j.steps, 1);
    strictEqual(j.screenshots, 1);
    strictEqual(typeof j.elapsed_s, 'number');
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
