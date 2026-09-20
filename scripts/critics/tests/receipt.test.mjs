import { describe, it } from 'node:test';
import { strictEqual, throws } from 'node:assert';
import { newReceipt, validateReceipt, finalizeReceipt, FORMAT } from '../lib/receipt.mjs';

describe('receipt', () => {
  it('valid minimal receipt', () => {
    const r = newReceipt({
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), origin: 'http://127.0.0.1:18080', kind: 'isolated-synthetic', seed: 'authored-test-fixture', binary_sha256: 'b'.repeat(64) },
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
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), origin: 'http://127.0.0.1:18080', kind: 'isolated-synthetic', seed: 'authored-test-fixture', binary_sha256: 'b'.repeat(64) },
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
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), origin: 'http://127.0.0.1:18080', kind: 'isolated-synthetic', seed: 'authored-test-fixture', binary_sha256: 'b'.repeat(64) },
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
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), origin: 'http://127.0.0.1:18080', kind: 'isolated-synthetic', seed: 'authored-test-fixture', binary_sha256: 'b'.repeat(64) },
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
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), origin: 'http://127.0.0.1:18080', kind: 'isolated-synthetic', seed: 'authored-test-fixture', binary_sha256: 'b'.repeat(64) },
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

  it('candidate.binary_revision must be 40/64-hex or null', () => {
    const base = {
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      checks: [],
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    };
    const good = newReceipt({
      ...base,
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), binary_revision: 'a'.repeat(40), binary_sha256: 'b'.repeat(64), kind: 'external-isolated-synthetic', seed: 'declared-unverified', origin: 'http://127.0.0.1:18080' }
    });
    strictEqual(validateReceipt(good).ok, true, 'a matching binary revision is valid');

    const bad = newReceipt({
      ...base,
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), binary_revision: 'zz', binary_sha256: 'b'.repeat(64), kind: 'external-isolated-synthetic', seed: 'declared-unverified', origin: 'http://127.0.0.1:18080' }
    });
    const v = validateReceipt(bad);
    strictEqual(v.ok, false, 'a malformed binary revision must fail validation');
    strictEqual(v.errors.some(e => e.includes('binary_revision')), true);
  });

  it('unbound candidate must not claim a binary revision', () => {
    const r = newReceipt({
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      candidate: { bound: false, handle: null, revision: null, binary_revision: 'a'.repeat(40), binary_sha256: null, kind: 'external-isolated-synthetic', seed: 'declared-unverified', origin: 'http://127.0.0.1:18080' },
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      checks: [],
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    });
    const v = validateReceipt(r);
    strictEqual(v.ok, false);
    strictEqual(v.errors.some(e => e.includes('must not claim a binary revision')), true);
  });

  it('never throws on malformed pass-check evidence shapes (total validator)', () => {
    const base = () => ({
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), binary_sha256: 'b'.repeat(64), kind: 'external-isolated-synthetic', seed: 'declared-unverified', origin: 'http://127.0.0.1:18080' },
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    });
    const shapes = [
      [undefined, 'evidence required for pass'],
      [{}, 'evidence.notes'],
      [{ notes: ['note'] }, 'evidence.screenshots'],
    ];
    for (const [evidence, expected] of shapes) {
      const r = newReceipt({
        ...base(),
        checks: [{ id: 'c1', surface: '/', status: 'pass', expected: 'e', observed: 'o', evidence }],
        coverage: { exercised: ['c1'], skipped: [] }
      });
      let v;
      try {
        v = validateReceipt(r, { fileExists: () => true });
      } catch (err) {
        throw new Error('validateReceipt threw on a malformed pass check: ' + err.message);
      }
      strictEqual(v.ok, false, 'malformed pass evidence must not validate: ' + JSON.stringify(v.errors));
      strictEqual(v.errors.some(e => e.includes(expected)), true,
        'errors must include "' + expected + '": ' + JSON.stringify(v.errors));
    }
  });

  it('requires candidate.bound to be a boolean (identity without it is rejected)', () => {
    const r = newReceipt({
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      candidate: { revision: 'a'.repeat(40), binary_sha256: 'b'.repeat(64), kind: 'external-isolated-synthetic', seed: 'declared-unverified', origin: 'http://127.0.0.1:18080' },
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      checks: [],
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    });
    const v = validateReceipt(r);
    strictEqual(v.ok, false, 'a receipt without candidate.bound must not validate');
    strictEqual(v.errors.some(e => e.includes('candidate.bound must be boolean')), true, 'errors: ' + JSON.stringify(v.errors));
  });

  it('rejects a binary_revision that differs from revision (contradictory identity)', () => {
    const base = {
      storyBindings: [{ story: 'US-002', criteria: ['US-002.1'] }],
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      checks: [],
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    };
    const conflicting = newReceipt({
      ...base,
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), binary_revision: 'c'.repeat(40), binary_sha256: 'b'.repeat(64), kind: 'external-isolated-synthetic', seed: 'declared-unverified', origin: 'http://127.0.0.1:18080' }
    });
    const v = validateReceipt(conflicting);
    strictEqual(v.ok, false, 'a contradictory binary_revision must not validate');
    strictEqual(v.errors.some(e => e.includes('must match candidate.revision')), true, 'errors: ' + JSON.stringify(v.errors));

    const sameRevision = newReceipt({
      ...base,
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), binary_revision: 'A'.repeat(40), binary_sha256: 'b'.repeat(64), kind: 'external-isolated-synthetic', seed: 'declared-unverified', origin: 'http://127.0.0.1:18080' }
    });
    strictEqual(validateReceipt(sameRevision).ok, true, 'the same revision in different case is not a contradiction');
  });

  it('is total on malformed container entries (nulls are rejected, not thrown)', () => {
    const r = newReceipt({
      storyBindings: [null],
      candidate: { bound: true, handle: 'target/critics/candidate.json', revision: 'a'.repeat(40), binary_sha256: 'b'.repeat(64), kind: 'external-isolated-synthetic', seed: 'declared-unverified', origin: 'http://127.0.0.1:18080' },
      environment: { viewport: '390x844', user_agent: 'test', network: 'local-loopback', data: 'synthetic-authored' },
      checks: [null],
      coverage: { exercised: [], skipped: [null] },
      runId: 'crit-test', maxSteps: 24, timeoutS: 120, maxScreenshots: 12
    });
    let v;
    try {
      v = validateReceipt(r);
    } catch (err) {
      throw new Error('validateReceipt threw on null container entries: ' + err.message);
    }
    strictEqual(v.ok, false);
    strictEqual(v.errors.some(e => e.includes('story_bindings[0] must be an object')), true, 'errors: ' + JSON.stringify(v.errors));
    strictEqual(v.errors.some(e => e.includes('checks[0] must be an object')), true, 'errors: ' + JSON.stringify(v.errors));
    strictEqual(v.errors.some(e => e.includes('coverage.skipped[0] must be an object')), true, 'errors: ' + JSON.stringify(v.errors));
  });
});
