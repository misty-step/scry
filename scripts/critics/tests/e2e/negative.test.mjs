import { describe, it } from 'node:test';
import { strictEqual, ok } from 'node:assert';
import { spawnSync, spawn } from 'node:child_process';
import { existsSync, readFileSync, mkdirSync, rmSync, writeFileSync, chmodSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { resolveBrowser } from '../../lib/browser.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const FIXTURES_DIR = join(__dirname, 'fixtures');
const CRITICS_DIR = join(__dirname, '..', '..');
const REPO_ROOT = join(__dirname, '..', '..', '..', '..');
const REQUIRE_BROWSER = process.env.SCRY_CRITICS_REQUIRE_BROWSER === '1';
const net = await import('node:net');

// A real free ephemeral port: closed at walk time and not on Chromium's
// unsafe-port list (port 1 is refused client-side as ERR_UNSAFE_PORT).
function freePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.listen(0, '127.0.0.1', () => {
      const port = server.address().port;
      server.close(() => resolve(port));
    });
    server.on('error', reject);
  });
}

// Browser-dependent tests must run genuinely wherever the environment
// declares the browser required (the CI gate). Elsewhere an unavailable
// browser is a visible skip, never a silent pass.
async function browserGate(t) {
  const browser = await resolveBrowser();
  if (browser.ok) return browser;
  if (REQUIRE_BROWSER) {
    throw new Error('browser tests are required in this environment but the browser is unavailable: ' + browser.reason);
  }
  t.skip('browser unavailable: ' + browser.reason);
  return null;
}

function runWalk(args, env = {}) {
  return spawnSync('node', [
    join(CRITICS_DIR, 'run.mjs'), 'human', '--goal', 'practice-review', ...args
  ], { encoding: 'utf8', timeout: 60000, env: { ...process.env, ...env } });
}

// Fixtures must be served by a child process: the walker runs under a
// blocking spawnSync, so an in-process server could never answer it.
function serveFixture(htmlPath) {
  return new Promise((resolve, reject) => {
    const child = spawn('node', [
      '-e',
      `
      const http = require('http');
      const fs = require('fs');
      const html = fs.readFileSync(process.env.FIXTURE_PATH);
      const server = http.createServer((req, res) => {
        res.setHeader('Content-Type', 'text/html');
        res.end(html);
      });
      server.listen(0, '127.0.0.1', () => console.log('PORT ' + server.address().port));
      `,
    ], { stdio: ['ignore', 'pipe', 'inherit'], env: { ...process.env, FIXTURE_PATH: htmlPath } });

    let buffer = '';
    let settled = false;
    child.stdout.on('data', (chunk) => {
      buffer += chunk.toString();
      const m = buffer.match(/PORT (\d+)/);
      if (m && !settled) {
        settled = true;
        resolve({
          url: 'http://127.0.0.1:' + m[1] + '/',
          close: () => new Promise(done => { child.once('exit', done); child.kill(); }),
        });
      }
    });
    child.on('exit', () => {
      if (!settled) { settled = true; reject(new Error('fixture server exited before listening')); }
    });
    child.on('error', reject);
  });
}

function readReceipt(outDir) {
  return JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
}

describe('negative e2e failure scenarios', () => {
  it('production target https://scry.study -> exit 2, no receipt', async () => {
    const outDir = join(REPO_ROOT, 'target/critics/e2e-negative-prod');
    rmSync(outDir, { recursive: true, force: true });
    mkdirSync(outDir, { recursive: true });

    const result = runWalk(['--candidate', 'https://scry.study', '--out', outDir]);

    strictEqual(result.status, 2, 'expected exit code 2 for production origin');
    ok(result.stderr.includes('Blocked') || result.stderr.includes('production'), 'should mention blocked');
    strictEqual(existsSync(join(outDir, 'receipt.json')), false, 'a refused target must not produce a receipt');

    rmSync(outDir, { recursive: true, force: true });
  });

  it('closed-port candidate -> exit 2 with a blocked receipt from a real network failure', async (t) => {
    const browser = await browserGate(t);
    if (!browser) return;

    const outDir = join(REPO_ROOT, 'target/critics/e2e-negative-blocked');
    rmSync(outDir, { recursive: true, force: true });
    mkdirSync(outDir, { recursive: true });

    const port = await freePort();
    const result = runWalk(['--candidate', 'http://127.0.0.1:' + port + '/', '--out', outDir]);

    strictEqual(result.status, 2, 'expected exit code 2 for blocked env, got ' + result.status);
    ok(existsSync(join(outDir, 'receipt.json')), 'receipt should be written');
    const receipt = readReceipt(outDir);
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && check.status === 'unverified', 'walk-execution must be unverified');
    ok(!/playwright unavailable|chromium unavailable/i.test(check.observed),
      'the browser must actually launch; observed: ' + check.observed);
    ok(/ERR_CONNECTION|ECONNREFUSED/i.test(check.observed),
      'expected a connection failure, observed: ' + check.observed);

    rmSync(outDir, { recursive: true, force: true });
  });

  it('no-op Next fixture -> exit 1 with the next-no-transition finding only', async (t) => {
    const browser = await browserGate(t);
    if (!browser) return;

    const outDir = join(REPO_ROOT, 'target/critics/e2e-negative-noop');
    rmSync(outDir, { recursive: true, force: true });
    mkdirSync(outDir, { recursive: true });

    const fixture = await serveFixture(join(FIXTURES_DIR, 'next-no-op.html'));
    const result = runWalk(['--candidate', fixture.url, '--out', outDir]);
    await fixture.close();

    strictEqual(result.status, 1, 'expected exit 1 for a finding, got ' + result.status);
    const receipt = readReceipt(outDir);
    const statuses = receipt.checks.map(c => c.id + ':' + c.status);
    strictEqual(statuses.includes('US-002.1:pass'), true, 'US-002.1 must pass: ' + statuses.join(', '));
    strictEqual(statuses.includes('US-002.2:pass'), true, 'US-002.2 must pass: ' + statuses.join(', '));
    strictEqual(receipt.findings.length, 1, 'exactly one finding expected');
    const finding = receipt.findings[0];
    strictEqual(finding.mechanism, 'next-no-transition');
    strictEqual(finding.criterion, 'US-002.2');
    ok(/^find-US-002-[0-9a-f]{8}$/.test(finding.fingerprint), 'fingerprint must be pinned: ' + finding.fingerprint);
    ok(receipt.coverage.exercised.includes('US-002.2-next'), 'US-002.2-next must be exercised');

    rmSync(outDir, { recursive: true, force: true });
  });

  it('missing feedback/Next fixture -> exit 1 with the feedback-absent finding', async (t) => {
    const browser = await browserGate(t);
    if (!browser) return;

    const outDir = join(REPO_ROOT, 'target/critics/e2e-negative-missing');
    rmSync(outDir, { recursive: true, force: true });
    mkdirSync(outDir, { recursive: true });

    const fixture = await serveFixture(join(FIXTURES_DIR, 'missing-feedback.html'));
    const result = runWalk(['--candidate', fixture.url, '--out', outDir]);
    await fixture.close();

    strictEqual(result.status, 1, 'expected exit 1 for a finding, got ' + result.status);
    const receipt = readReceipt(outDir);
    const statuses = receipt.checks.map(c => c.id + ':' + c.status);
    strictEqual(statuses.includes('US-002.1:pass'), true, 'US-002.1 must pass: ' + statuses.join(', '));
    strictEqual(receipt.findings.length, 1, 'exactly one finding expected');
    strictEqual(receipt.findings[0].mechanism, 'feedback-absent');

    rmSync(outDir, { recursive: true, force: true });
  });

  it('browser launch failure -> exit 2 with a blocked receipt (not a findings exit)', async (t) => {
    const browser = await resolveBrowser();
    if (!browser.playwright) {
      if (REQUIRE_BROWSER) throw new Error('browser tests are required but the playwright module is unavailable');
      t.skip('playwright module unavailable: ' + browser.reason);
      return;
    }

    const outDir = join(REPO_ROOT, 'target/critics/e2e-negative-launch');
    rmSync(outDir, { recursive: true, force: true });
    mkdirSync(outDir, { recursive: true });
    const fakeChromium = join(REPO_ROOT, 'target/critics/e2e-fake-chromium');
    writeFileSync(fakeChromium, '#!/bin/sh\nexit 127\n');
    chmodSync(fakeChromium, 0o755);

    const port = await freePort();
    const result = runWalk(['--candidate', 'http://127.0.0.1:' + port + '/', '--out', outDir], { CHROMIUM_PATH: fakeChromium });

    strictEqual(result.status, 2, 'expected exit 2 for a blocked launch, got ' + result.status);
    ok(existsSync(join(outDir, 'receipt.json')), 'launch failure must preserve a receipt');
    const receipt = readReceipt(outDir);
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && check.status === 'unverified', 'walk-execution must be unverified');
    ok(/browser launch failed/i.test(check.observed), 'observed must name the launch failure: ' + check.observed);

    rmSync(outDir, { recursive: true, force: true });
  });

  it('--max-steps is enforced: impossible budget stops the walk (exit 2, receipt)', async (t) => {
    const browser = await browserGate(t);
    if (!browser) return;

    const outDir = join(REPO_ROOT, 'target/critics/e2e-negative-maxsteps');
    rmSync(outDir, { recursive: true, force: true });
    mkdirSync(outDir, { recursive: true });

    const fixture = await serveFixture(join(FIXTURES_DIR, 'next-no-op.html'));
    const result = runWalk(['--candidate', fixture.url, '--max-steps', '1', '--out', outDir]);
    await fixture.close();

    strictEqual(result.status, 2, 'a budget stop must be blocked (exit 2), got ' + result.status);
    const receipt = readReceipt(outDir);
    strictEqual(receipt.run.budget.maxSteps, 1, 'receipt must record the configured budget');
    ok(receipt.run.budget.steps >= 1, 'steps must be counted');
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && /max steps exceeded/i.test(check.observed), 'observed must name the limit: ' + check.observed);

    rmSync(outDir, { recursive: true, force: true });
  });

  it('--timeout is enforced: impossible timeout stops the walk (exit 2, receipt)', async (t) => {
    const browser = await browserGate(t);
    if (!browser) return;

    const outDir = join(REPO_ROOT, 'target/critics/e2e-negative-timeout');
    rmSync(outDir, { recursive: true, force: true });
    mkdirSync(outDir, { recursive: true });

    const fixture = await serveFixture(join(FIXTURES_DIR, 'next-no-op.html'));
    const result = runWalk(['--candidate', fixture.url, '--timeout', '1', '--out', outDir]);
    await fixture.close();

    strictEqual(result.status, 2, 'a budget stop must be blocked (exit 2), got ' + result.status);
    const receipt = readReceipt(outDir);
    strictEqual(receipt.run.budget.timeoutS, 1, 'receipt must record the configured budget');
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && /timeout exceeded/i.test(check.observed), 'observed must name the limit: ' + check.observed);

    rmSync(outDir, { recursive: true, force: true });
  });

  it('--allow-origin is accepted, validated, and never silently ignored', async () => {
    const dir = join(REPO_ROOT, 'target/critics/e2e-negative-allow-origin');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });

    // Non-loopback without --allow-origin: refused.
    const missing = runWalk(['--candidate', 'http://example.com/', '--out', join(dir, 'a')]);
    strictEqual(missing.status, 2, 'non-loopback without --allow-origin must be refused');
    ok(/allow-origin/i.test(missing.stderr), 'refusal must name --allow-origin: ' + missing.stderr);
    ok(!/ERR_PARSE_ARGS/.test(missing.stderr), 'the flag must parse: ' + missing.stderr);

    // Non-loopback with a non-matching --allow-origin: refused.
    const mismatch = runWalk(['--candidate', 'http://example.com/', '--allow-origin', 'http://other.example/', '--out', join(dir, 'b')]);
    strictEqual(mismatch.status, 2, 'mismatched --allow-origin must be refused');
    ok(/does not match/i.test(mismatch.stderr), 'refusal must explain the mismatch: ' + mismatch.stderr);
    ok(!/ERR_PARSE_ARGS/.test(mismatch.stderr), 'the flag must parse: ' + mismatch.stderr);

    // Production origins stay refused even with a matching --allow-origin.
    const prod = runWalk(['--candidate', 'https://scry.study/', '--allow-origin', 'https://scry.study', '--out', join(dir, 'c')]);
    strictEqual(prod.status, 2, 'production must stay refused');
    ok(/production/i.test(prod.stderr), 'refusal must name production: ' + prod.stderr);

    rmSync(dir, { recursive: true, force: true });
  });

  it('--goal is validated (unknown or missing goal exits 2 without a walk)', async () => {
    const dir = join(REPO_ROOT, 'target/critics/e2e-negative-goal');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });

    const unknown = spawnSync('node', [
      join(CRITICS_DIR, 'run.mjs'), 'human', '--goal', 'definitely-not-a-goal',
      '--candidate', 'http://127.0.0.1:1/', '--out', join(dir, 'a')
    ], { encoding: 'utf8', timeout: 30000 });
    strictEqual(unknown.status, 2, 'unknown goal must exit 2, got ' + unknown.status);
    ok(/unknown goal/.test(unknown.stderr), 'must name the unknown goal: ' + unknown.stderr);
    strictEqual(existsSync(join(dir, 'a', 'receipt.json')), false, 'no walk, no receipt');

    const missing = spawnSync('node', [
      join(CRITICS_DIR, 'run.mjs'), 'human', '--candidate', 'http://127.0.0.1:1/', '--out', join(dir, 'b')
    ], { encoding: 'utf8', timeout: 30000 });
    strictEqual(missing.status, 2, 'missing --goal must exit 2, got ' + missing.status);
    ok(/--goal is required/.test(missing.stderr), 'must require --goal: ' + missing.stderr);

    const badNumber = runWalk(['--candidate', 'http://127.0.0.1:1/', '--max-steps', '0', '--out', join(dir, 'c')]);
    strictEqual(badNumber.status, 2, 'invalid numeric flag must exit 2');
    ok(/positive integer/.test(badNumber.stderr), 'must explain the numeric constraint: ' + badNumber.stderr);

    rmSync(dir, { recursive: true, force: true });
  });

  it('required-browser mode fails (not passes) when the browser is masked', async (t) => {
    if (REQUIRE_BROWSER) {
      t.skip('meta-check runs only in optional-browser mode');
      return;
    }
    const maskedHome = join(REPO_ROOT, 'target/critics/e2e-masked-home');
    rmSync(maskedHome, { recursive: true, force: true });
    mkdirSync(maskedHome, { recursive: true });

    const result = spawnSync('node', ['--test', join(__dirname, 'negative.test.mjs')], {
      encoding: 'utf8',
      timeout: 90000,
      env: {
        PATH: process.env.PATH,
        HOME: maskedHome,
        NODE_PATH: '',
        SCRY_CRITICS_PLAYWRIGHT: '',
        SCRY_CRITICS_REQUIRE_BROWSER: '1',
      },
    });

    const output = (result.stdout || '') + (result.stderr || '');
    ok(result.status !== 0, 'the suite must fail when a required browser is unavailable');
    ok(/required in this environment/.test(output), 'failure must name the requirement: ' + output.slice(0, 400));

    rmSync(maskedHome, { recursive: true, force: true });
  });
});