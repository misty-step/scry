/**
 * Deterministic fingerprint for findings
 * - story: US-xxx
 * - criterion: US-xxx.n (e.g. US-002.1)
 * - mechanism: normalized string (e.g. "review-surface-missing-next-button")
 */

import { createHash } from 'node:crypto';

export function normalizeMechanism(mechanism) {
  return String(mechanism)
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(0, 60);
}

export function fingerprint({ story, criterion, mechanism }) {
  const mech = normalizeMechanism(mechanism);
  const input = `${story}|${criterion}|${mech}`;
  const hash = createHash('sha256').update(input).digest('hex').slice(0, 8);
  return `find-${story}-${hash}`;
}

export function dedupeFindings(findings) {
  const seen = new Map();
  for (const f of findings) {
    const key = fingerprint({ story: f.story, criterion: f.criterion, mechanism: f.mechanism });
    if (!seen.has(key)) seen.set(key, f);
  }
  return Array.from(seen.values());
}

export function mergeFindings(existing, incoming) {
  const deduped = dedupeFindings([...existing, ...incoming]);
  return deduped;
}
