/**
 * Browser-stack resolution shared by the walker and the test suite.
 *
 * A missing browser is an environment block: the walker refuses to run and
 * writes a blocked receipt; the test suite must either skip visibly or fail
 * when the environment declares the browser required (CI gate).
 */

import { existsSync } from 'node:fs';
import { access, constants } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { join, dirname, delimiter } from 'node:path';
import { fileURLToPath } from 'node:url';
import os from 'node:os';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const REPO_ROOT = join(__dirname, '..', '..', '..');

// === playwright resolution ===
export function tryRequirePlaywright() {
  const nodePath = process.env.NODE_PATH;
  if (nodePath) {
    const paths = nodePath.split(delimiter);
    for (const p of paths) {
      if (existsSync(join(p, 'playwright'))) {
        try {
          const require = createRequire(import.meta.url);
          const mod = require(p + '/playwright');
          return { mod, source: 'NODE_PATH' };
        } catch (_) {}
      }
    }
  }

  const pwEnv = process.env.SCRY_CRITICS_PLAYWRIGHT;
  if (pwEnv) {
    try {
      const require = createRequire(import.meta.url);
      const mod = require(pwEnv);
      return { mod, source: 'SCRY_CRITICS_PLAYWRIGHT' };
    } catch (_) {}
    try {
      const require = createRequire(import.meta.url);
      const mod = require(join(REPO_ROOT, pwEnv));
      return { mod, source: 'SCRY_CRITICS_PLAYWRIGHT (relative)' };
    } catch (_) {}
  }

  try {
    const require = createRequire(import.meta.url);
    const mod = require('playwright');
    return { mod, source: 'cwd-require' };
  } catch (_) {}

  const homedir = os.homedir();
  const homedirPaths = [
    join(homedir, '.local/share/mise/installs/npm-playwright/latest/node_modules/playwright'),
    join(homedir, 'node_modules/playwright'),
    join(homedir, '.npm-global/lib/node_modules/playwright'),
  ];
  for (const p of homedirPaths) {
    if (existsSync(p)) {
      try {
        const require = createRequire(import.meta.url);
        const mod = require(p);
        return { mod, source: 'homedir-fallback' };
      } catch (_) {}
    }
  }

  return null;
}

// === chromium resolution ===
export async function tryFindChromium() {
  const candidates = [
    process.env.CHROMIUM_PATH,
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
    '/usr/bin/google-chrome',
    '/usr/bin/google-chrome-stable',
  ].filter(Boolean);

  for (const p of candidates) {
    try {
      await access(p, constants.X_OK);
      return p;
    } catch (_) {}
  }

  return null;
}

// Container/CI runs as root: Chromium needs --no-sandbox there. The walker
// never weakens the sandbox outside that condition.
export function chromiumLaunchArgs() {
  const args = [];
  if (process.env.SCRY_CRITICS_NO_SANDBOX === '1') args.push('--no-sandbox');
  else if (typeof process.getuid === 'function' && process.getuid() === 0) args.push('--no-sandbox');
  return args;
}

export function browserRequired() {
  return process.env.SCRY_CRITICS_REQUIRE_BROWSER === '1';
}

/**
 * Resolve the full browser stack.
 * Returns { ok, reason, playwright, chromium, launchArgs }.
 *  - playwright: { mod, source } | null
 *  - chromium: path | null
 */
export async function resolveBrowser() {
  const playwright = tryRequirePlaywright();
  if (!playwright) {
    return { ok: false, reason: 'playwright module unavailable', playwright: null, chromium: null, launchArgs: [] };
  }
  const chromium = await tryFindChromium();
  if (!chromium) {
    return { ok: false, reason: 'chromium executable unavailable', playwright, chromium: null, launchArgs: [] };
  }
  return { ok: true, reason: null, playwright, chromium, launchArgs: chromiumLaunchArgs() };
}