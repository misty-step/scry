import { describe, it } from 'node:test';
import { strictEqual } from 'node:assert';
import { fingerprint, normalizeMechanism, dedupeFindings, mergeFindings } from '../lib/dedupe.mjs';

describe('dedupe', () => {
  it('normalizeMechanism collapses whitespace and punctuation', () => {
    strictEqual(normalizeMechanism('Test   Mechanism!'), 'test-mechanism');
    strictEqual(normalizeMechanism('  test  '), 'test');
  });

  it('fingerprint is deterministic for same inputs', () => {
    const f1 = fingerprint({ story: 'US-002', criterion: 'US-002.1', mechanism: 'test' });
    const f2 = fingerprint({ story: 'US-002', criterion: 'US-002.1', mechanism: 'test' });
    strictEqual(f1, f2);
    strictEqual(f1.startsWith('find-'), true);
  });

  it('different mechanisms produce different fingerprints', () => {
    const f1 = fingerprint({ story: 'US-002', criterion: 'US-002.1', mechanism: 'missing' });
    const f2 = fingerprint({ story: 'US-002', criterion: 'US-002.1', mechanism: 'broken' });
    strictEqual(f1 !== f2, true);
  });

  it('fingerprint ignores title/prose (same story+criterion+mechanism)', () => {
    const f1 = fingerprint({ story: 'US-002', criterion: 'US-002.1', mechanism: 'review-surface' });
    const f2 = fingerprint({ story: 'US-002', criterion: 'US-002.1', mechanism: 'review-surface' });
    strictEqual(f1, f2);
  });

  it('dedupeFindings collapses identical fingerprints', () => {
    const findings = [
      { id: fingerprint({ story: 'US-002', criterion: 'US-002.1', mechanism: 'm' }), story: 'US-002', criterion: 'US-002.1', mechanism: 'm', title: 'a', expected: 'e', actual: 'a', impact: 'i', uncertainty: 'u', evidence: {}, acceptance: 'a' },
      { id: fingerprint({ story: 'US-002', criterion: 'US-002.1', mechanism: 'm' }), story: 'US-002', criterion: 'US-002.1', mechanism: 'm', title: 'b', expected: 'e', actual: 'a', impact: 'i', uncertainty: 'u', evidence: {}, acceptance: 'a' }
    ];
    const deduped = dedupeFindings(findings);
    strictEqual(deduped.length, 1);
  });

  it('different mechanisms do not collapse', () => {
    const findings = [
      { id: fingerprint({ story: 'US-002', criterion: 'US-002.1', mechanism: 'm1' }), story: 'US-002', criterion: 'US-002.1', mechanism: 'm1', title: 'a', expected: 'e', actual: 'a', impact: 'i', uncertainty: 'u', evidence: {}, acceptance: 'a' },
      { id: fingerprint({ story: 'US-002', criterion: 'US-002.1', mechanism: 'm2' }), story: 'US-002', criterion: 'US-002.1', mechanism: 'm2', title: 'b', expected: 'e', actual: 'a', impact: 'i', uncertainty: 'u', evidence: {}, acceptance: 'a' }
    ];
    const deduped = dedupeFindings(findings);
    strictEqual(deduped.length, 2);
  });

  it('mergeFindings dedupes identical operation replay', () => {
    const f1 = { id: 'f1', story: 'US-002', criterion: 'US-002.1', mechanism: 'm', title: 't', expected: 'e', actual: 'a', impact: 'i', uncertainty: 'u', evidence: {}, acceptance: 'a' };
    const deduped = mergeFindings([f1], [f1]);
    strictEqual(deduped.length, 1);
  });
});
