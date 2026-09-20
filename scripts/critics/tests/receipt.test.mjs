import { describe, it } from 'node:test';
import { strictEqual, throws } from 'node:assert';
import { newReceipt, validateReceipt, finalizeReceipt, FORMAT } from '../lib/receipt.mjs';

describe('receipt', () => {
  it('valid minimal receipt', () => {
    const r = newReceipt({
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      candidate: { revision: 'a'.repeat(40), origin: 'http://127.0.0.1:18080', kind: 'isolated-synthetic', seed: 'authored-test-fixture', binary_sha256: 'b'.repeat(64) },
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      checks: [],
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    });
    const v = validateReceipt(r);
    strictEqual(v.ok, true, 'receipt should be valid');
    strictEqual(r.format, FORMAT);
  });

  it('pass check requires notes', () => {
    const r = newReceipt({
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      candidate: { revision: 'a'.repeat(40), origin: 'http://127.0.0.1:18080', kind: 'isolated-synthetic', seed: 'authored-test-fixture', binary_sha256: 'b'.repeat(64) },
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      checks: [{ id: 'check1', surface: '/', status: 'pass', expected: 'x', observed: 'y', evidence: { screenshots: [{ name: 's', path: '/tmp/s.png' }], notes: [] } }],
      coverage: { exercised: ['check1'], skipped: [] },
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    });
    const v = validateReceipt(r, { fileExists: () => true });
    strictEqual(v.ok, false);
    strictEqual(v.errors.some(e => e.includes('evidence.notes')), true);
  });

  it('unexercised checks cannot be pass', () => {
    const r = newReceipt({
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      candidate: { revision: 'a'.repeat(40), origin: 'http://127.0.0.1:18080', kind: 'isolated-synthetic', seed: 'authored-test-fixture', binary_sha256: 'b'.repeat(64) },
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      checks: [{ id: 'check1', surface: '/', status: 'pass', expected: 'x', observed: 'y', evidence: { screenshots: [{ name: 's', path: '/tmp/s.png' }], notes: ['note'] } }],
      coverage: { exercised: [], skipped: [] },
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    });
    const v = validateReceipt(r);
    strictEqual(v.ok, false);
    strictEqual(v.errors.some(e => e.includes('not exercised')), true);
  });

  it('finalize sets end and duration', () => {
    const r = newReceipt({
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      candidate: { revision: 'a'.repeat(40), origin: 'http://127.0.0.1:18080', kind: 'isolated-synthetic', seed: 'authored-test-fixture', binary_sha256: 'b'.repeat(64) },
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    });
    const final = finalizeReceipt(r, '2026-09-19T00:00:10.000Z', 10);
    strictEqual(final.run.ended_at, '2026-09-19T00:00:10.000Z');
    strictEqual(final.run.duration_s, 10);
  });

  it('no_findings must match findings length', () => {
    const r = newReceipt({
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      candidate: { revision: 'a'.repeat(40), origin: 'http://127.0.0.1:18080', kind: 'isolated-synthetic', seed: 'authored-test-fixture', binary_sha256: 'b'.repeat(64) },
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      findings: [{ id: 'f1', category: 'missing', story: 'US-002', criterion: 'US-002.1', mechanism: 'test', title: 't', expected: 'e', actual: 'a', impact: 'i', uncertainty: 'u', evidence: {}, acceptance: 'a' }],
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    });
    const v = validateReceipt(r);
    strictEqual(v.ok, false);
  });

  it('bound candidate requires revision, digest, and handle', () => {
    const base = {
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      checks: [],
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    };
    const bound = newReceipt({
      ...base,
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), binary_sha256: 'b'.repeat(64), kind: 'external-isolated-synthetic', seed: 'declared-unverified', origin: 'http://127.0.0.1:18080' }
    });
    strictEqual(validateReceipt(bound).ok, true, 'a fully bound candidate is valid');

    const noDigest = newReceipt({
      ...base,
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), binary_sha256: null, kind: 'external-isolated-synthetic', seed: 'declared-unverified', origin: 'http://127.0.0.1:18080' }
    });
    const v = validateReceipt(noDigest);
    strictEqual(v.ok, false, 'bound without a digest must fail validation');
    strictEqual(v.errors.some(e => e.includes('binary_sha256')), true);
  });

  it('unbound candidate must not claim identity', () => {
    const claimed = newReceipt({
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      candidate: { bound: false, handle: null, revision: 'a'.repeat(40), binary_sha256: null, kind: 'external-isolated-synthetic', seed: 'declared-unverified', origin: 'http://127.0.0.1:18080' },
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      checks: [],
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    });
    const v = validateReceipt(claimed);
    strictEqual(v.ok, false);
    strictEqual(v.errors.some(e => e.includes('must not claim a revision')), true);
  });
});
