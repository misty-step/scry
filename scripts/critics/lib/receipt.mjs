/**
 * scry-critic-receipt-v1 schema + validator
 */

import { createHash } from 'node:crypto';
import { existsSync } from 'node:fs';

export const FORMAT = 'scry-critic-receipt-v1';

export function normalizeString(s) {
  return String(s).replace(/\s+/g, ' ').trim();
}

export function validateReceipt(r, { fileExists = existsSync } = {}) {
  const errors = [];
  if (!r || typeof r !== 'object') return { ok: false, errors: ['receipt must be an object'] };

  if (r.format !== FORMAT) errors.push(`format must be "${FORMAT}", got "${r.format}"`);

  // no_findings: must match findings.length === 0
  if (typeof r.no_findings !== 'boolean') errors.push('no_findings must be boolean');
  else if (r.no_findings !== ((r.findings?.length ?? 0) === 0)) errors.push('no_findings must match findings.length');

  if (!r.run || typeof r.run !== 'object') errors.push('run is required');
  else {
    if (typeof r.run.id !== 'string') errors.push('run.id must be string');
    if (typeof r.run.started_at !== 'string') errors.push('run.started_at must be ISO string');
    if (r.run.ended_at != null && typeof r.run.ended_at !== 'string') errors.push('run.ended_at must be ISO string or null');
    if (r.run.duration_s != null && typeof r.run.duration_s !== 'number') errors.push('run.duration_s must be number or null');
    if (!r.run.budget) errors.push('run.budget is required');
  }

  if (!r.candidate || typeof r.candidate !== 'object') errors.push('candidate is required');
  else {
    if (r.candidate.revision != null && !/^([0-9a-f]{40}|[0-9a-f]{64})$/i.test(r.candidate.revision)) errors.push('candidate.revision must be 40/64-hex or null');
    if (typeof r.candidate.kind !== 'string') errors.push('candidate.kind must be string');
    if (typeof r.candidate.seed !== 'string') errors.push('candidate.seed must be string');
    if (r.candidate.binary_sha256 !== null && !/^[0-9a-f]{64}$/i.test(r.candidate.binary_sha256 ?? '')) errors.push('candidate.binary_sha256 must be 64-hex or null');
    if (r.candidate.bound !== undefined) {
      if (typeof r.candidate.bound !== 'boolean') errors.push('candidate.bound must be boolean');
      else if (r.candidate.bound === true) {
        if (!/^([0-9a-f]{40}|[0-9a-f]{64})$/i.test(String(r.candidate.revision ?? ''))) errors.push('bound candidate requires a 40/64-hex revision');
        if (!/^[0-9a-f]{64}$/i.test(String(r.candidate.binary_sha256 ?? ''))) errors.push('bound candidate requires a 64-hex binary_sha256');
        if (typeof r.candidate.handle !== 'string' || r.candidate.handle.length === 0) errors.push('bound candidate requires a handle reference');
      } else {
        if (r.candidate.revision !== null) errors.push('unbound candidate must not claim a revision');
        if (r.candidate.binary_sha256 !== null) errors.push('unbound candidate must not claim a binary_sha256');
      }
    }
    if (r.candidate.binary_revision !== undefined && r.candidate.binary_revision !== null &&
        !/^([0-9a-f]{40}|[0-9a-f]{64})$/i.test(String(r.candidate.binary_revision))) errors.push('candidate.binary_revision must be 40/64-hex or null');
    if (r.candidate.binary_revision != null && r.candidate.bound !== true) errors.push('unbound candidate must not claim a binary revision');
    if (r.candidate.handle_id !== undefined && r.candidate.handle_id !== null && typeof r.candidate.handle_id !== 'string') errors.push('candidate.handle_id must be string or null');
    if (r.candidate.source_state !== undefined && r.candidate.source_state !== null &&
        !['clean', 'dirty', 'unknown'].includes(r.candidate.source_state)) errors.push('candidate.source_state must be clean|dirty|unknown or null');
  }

  if (!r.environment || typeof r.environment !== 'object') errors.push('environment is required');
  else {
    if (typeof r.environment.viewport !== 'string') errors.push('environment.viewport must be string');
    if (typeof r.environment.user_agent !== 'string') errors.push('environment.user_agent must be string');
    if (typeof r.environment.network !== 'string') errors.push('environment.network must be string');
    if (r.environment.data !== 'synthetic-authored' && r.environment.data !== 'external-declared-synthetic') errors.push('environment.data must be "synthetic-authored" or "external-declared-synthetic"');
  }

  if (!Array.isArray(r.story_bindings)) errors.push('story_bindings must be array');
  else if (r.story_bindings.length === 0) errors.push('story_bindings must not be empty');
  else for (let i = 0; i < r.story_bindings.length; i++) {
    const b = r.story_bindings[i];
    if (!/US-\d{3}/.test(b.story)) errors.push(`story_bindings[${i}].story must match /US-\d{3}/`);
    if (!Array.isArray(b.criteria)) errors.push(`story_bindings[${i}].criteria must be array`);
  }

  if (!Array.isArray(r.checks)) errors.push('checks must be array');
  else for (let i = 0; i < r.checks.length; i++) {
    const c = r.checks[i];
    if (typeof c.id !== 'string') errors.push(`checks[${i}].id must be string`);
    if (typeof c.surface !== 'string') errors.push(`checks[${i}].surface must be string`);
    if (!['pass', 'fail', 'unverified'].includes(c.status)) errors.push(`checks[${i}].status must be pass|fail|unverified, got "${c.status}"`);
    if (c.status === 'pass') {
      if (!c.evidence || typeof c.evidence !== 'object') errors.push(`checks[${i}].evidence required for pass`);
      else if (!Array.isArray(c.evidence.notes)) errors.push(`checks[${i}].evidence.notes must be array`);
      else if (c.evidence.notes.length === 0) errors.push(`checks[${i}].evidence.notes must be non-empty for pass`);
      if (!Array.isArray(c.evidence.screenshots)) errors.push(`checks[${i}].evidence.screenshots must be array`);
      for (let j = 0; j < c.evidence.screenshots.length; j++) {
        const sh = c.evidence.screenshots[j];
        if (typeof sh.path !== 'string') errors.push(`checks[${i}].evidence.screenshots[${j}].path must be string`);
        if (!fileExists(sh.path)) errors.push(`checks[${i}].evidence.screenshots[${j}].path "${sh.path}" does not exist`);
      }
    }
  }

  if (!Array.isArray(r.findings)) errors.push('findings must be array');

  if (!r.coverage || typeof r.coverage !== 'object') errors.push('coverage is required');
  else {
    if (!Array.isArray(r.coverage.exercised)) errors.push('coverage.exercised must be array');
    if (!Array.isArray(r.coverage.skipped)) errors.push('coverage.skipped must be array');
    else for (let i = 0; i < r.coverage.skipped.length; i++) {
      if (typeof r.coverage.skipped[i].id !== 'string') errors.push(`coverage.skipped[${i}].id must be string`);
    }
    const exercisedIds = new Set(r.coverage.exercised);
    for (const c of r.checks ?? []) {
      if (c.status === 'pass' && !exercisedIds.has(c.id)) errors.push(`check "${c.id}" is pass but not exercised`);
    }
  }

  if (!r.advisory || typeof r.advisory !== 'object') errors.push('advisory is required');
  else if (!Array.isArray(r.advisory.jevs)) errors.push('advisory.jevs must be array');
  else if (!Array.isArray(r.advisory.notes)) errors.push('advisory.notes must be array');

  if (!Array.isArray(r.limits)) errors.push('limits must be array');

  return { ok: errors.length === 0, errors };
}

export function newReceipt(options) {
  const {
    storyBindings, candidate, environment, checks, findings, coverage, advisory, limits,
    runId, maxSteps, timeoutS, maxScreenshots
  } = options;
  const started_at = new Date().toISOString();
  const budget = { maxSteps, timeoutS, maxScreenshots };
  return {
    format: FORMAT,
    no_findings: true,
    run: { id: runId, started_at, ended_at: null, duration_s: null, budget },
    candidate, environment,
    story_bindings: storyBindings,
    checks,
    findings: findings ?? [],
    coverage: coverage ?? { exercised: [], skipped: [] },
    advisory: advisory ?? { jevs: [], notes: [] },
    limits: limits ?? []
  };
}

export function finalizeReceipt(receipt, endedAt, durationS) {
  receipt.run.ended_at = endedAt;
  receipt.run.duration_s = durationS;
  receipt.no_findings = (receipt.findings?.length ?? 0) === 0;
  return receipt;
}
