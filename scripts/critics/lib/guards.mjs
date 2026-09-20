/**
 * Safety guards: origin classification, budgets
 */

import { URL } from 'node:url';

// production origins per spec
const PRODUCTION_ORIGINS = new Set(['scry.study', 'www.scry.study']);

export function classifyOrigin(rawUrl, { mutating = false, allowOrigin = null } = {}) {
  if (!rawUrl) return { ok: false, kind: 'invalid', reason: 'no url provided' };

  let parsed;
  try { parsed = new URL(rawUrl); }
  catch (_) { return { ok: false, kind: 'invalid', reason: 'malformed url' }; }

  const { protocol, hostname, port } = parsed;
  const isHttps = protocol === 'https:';
  const isHttp = protocol === 'http:';

  if (!isHttp && !isHttps) {
    return { ok: false, kind: 'invalid', reason: 'protocol must be http or https' };
  }

  if (PRODUCTION_ORIGINS.has(hostname)) {
    if (mutating) {
      return { ok: false, kind: 'production', reason: 'production origins are forbidden for mutating modes' };
    }
    return { ok: false, kind: 'production', reason: 'production origins forbidden' };
  }

  const isLoopback = hostname === '127.0.0.1' || hostname === 'localhost';
  if (isLoopback) {
    return { ok: true, kind: 'loopback', origin: parsed.origin };
  }

  // non-loopback
  if (mutating && !allowOrigin) {
    return { ok: false, kind: 'non-loopback', reason: 'non-loopback mutating requires --allow-origin' };
  }
  if (mutating && allowOrigin !== parsed.origin) {
    return { ok: false, kind: 'non-loopback', reason: 'allow-origin does not match' };
  }

  return { ok: true, kind: 'non-loopback', origin: parsed.origin };
}

export class Budget {
  constructor({ maxSteps = 24, timeoutS = 120, maxScreenshots = 12 } = {}) {
    this.maxSteps = maxSteps;
    this.timeoutS = timeoutS;
    this.maxScreenshots = maxScreenshots;
    this.steps = 0;
    this.screenshots = 0;
    this.startedAt = Date.now();
  }

  checkStep() {
    if (this.steps >= this.maxSteps) {
      return { ok: false, reason: 'max steps exceeded' };
    }
    this.steps++;
    return { ok: true };
  }

  checkScreenshot() {
    if (this.screenshots >= this.maxScreenshots) {
      return { ok: false, reason: 'max screenshots exceeded' };
    }
    this.screenshots++;
    return { ok: true };
  }

  checkTimeout() {
    if ((Date.now() - this.startedAt) / 1000 >= this.timeoutS) {
      return { ok: false, reason: 'timeout exceeded' };
    }
    return { ok: true };
  }

  get exhausted() {
    return this.steps >= this.maxSteps || this.screenshots >= this.maxScreenshots ||
           (Date.now() - this.startedAt) / 1000 >= this.timeoutS;
  }

  toJSON() {
    return {
      maxSteps: this.maxSteps, timeoutS: this.timeoutS, maxScreenshots: this.maxScreenshots,
      steps: this.steps, screenshots: this.screenshots,
      elapsed_s: Math.round((Date.now() - this.startedAt) / 1000)
    };
  }
}

// Thrown when a budget limit stops the walk. Distinct from runner defects so
// receipts can classify the stop honestly (blocked stop, not a product finding).
export class BudgetError extends Error {
  constructor(message) {
    super(message);
    this.name = 'BudgetError';
    this.budgetStop = true;
  }
}
