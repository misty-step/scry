import { describe, it, after } from 'node:test';
import { strictEqual, ok } from 'node:assert';
import { spawn, spawnSync } from 'node:child_process';
import { existsSync, readFileSync, mkdtempSync, writeFileSync, mkdirSync, rmSync, chmodSync, renameSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { createServer as createNetServer } from 'node:net';
import { resolveBrowser } from '../../lib/browser.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const CRITICS_DIR = join(__dirname, '..', '..');
const REPO_ROOT = join(__dirname, '..', '..', '..', '..');
const RUN = join(CRITICS_DIR, 'run.mjs');
const REQUIRE_BROWSER = process.env.SCRY_CRITICS_REQUIRE_BROWSER === '1';
// Test artifacts stay OUT of the repository tree (the gate re-inventories it).
const TMP_ROOT = mkdtempSync(join(tmpdir(), 'scry-critics-binding-'));
const children = [];
after(() => {
  for (const child of children) { try { child.kill(); } catch (_) {} }
  rmSync(TMP_ROOT, { recursive: true, force: true });
});

// Closed at walk time and not on Chromium's unsafe-port list.
function freePort() {
  return new Promise((resolve, reject) => {
    const server = createNetServer();
    server.listen(0, '127.0.0.1', () => {
      const port = server.address().port;
      server.close(() => resolve(port));
    });
    server.on('error', reject);
  });
}

function headRevision() {
  const r = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: REPO_ROOT, encoding: 'utf8' });
  return r.status === 0 ? r.stdout.trim() : null;
}

function sha256File(path) {
  return createHash('sha256').update(readFileSync(path)).digest('hex');
}

function writeHandle(dir, opts) {
  const { revision, sha, url, id, sourceState = 'clean', pid = 999999, binary = null } = opts;
  // Default: a candidate-up-shaped handle declares the binary's own revision.
  // `binaryRevision: null` writes an undeterminable provenance; `undefined`
  // omits the field (a pre-fix handle shape).
  const binaryRevision = 'binaryRevision' in opts ? opts.binaryRevision : revision;
  const path = join(dir, 'candidate.json');
  const handle = {
    format: 'scry-critic-candidate-v1',
    id: id ?? 'cand-test000000',
    pid,
    url,
    binary: binary ?? undefined,
    binary_sha256: sha,
    revision,
    source_state: sourceState,
    seed: { model: 'authored-test-fixture', source: 'test' },
  };
  if (binaryRevision !== undefined) handle.binary_revision = binaryRevision;
  writeFileSync(path, JSON.stringify(handle, null, 2));
  return path;
}

// A pid that is already gone — stands in for a stopped candidate.
function deadPid() {
  return new Promise((resolve) => {
    const child = spawn('/bin/sh', ['-c', 'exit 0'], { stdio: 'ignore' });
    child.on('exit', () => resolve(child.pid));
  });
}

// A live process whose command line references its own script path — the
// smallest honest stand-in for the serving candidate at the walk's checks.
function startKeepalive(dir, name = 'keepalive.sh') {
  const script = join(dir, name);
  writeFileSync(script, '#!/bin/sh\nwhile true; do sleep 60; done\n');
  chmodSync(script, 0o755);
  const child = spawn(script, [], { stdio: 'ignore' });
  children.push(child);
  return {
    pid: child.pid,
    binary: script,
    sha: sha256File(script),
    stop: () => new Promise(done => { child.once('exit', done); child.kill(); }),
  };
}

// Any other artifact on the port, served by a child process (the walker runs
// under a blocking spawnSync, so an in-process server could never answer).
function startSwappedServer(html) {
  return new Promise((resolve, reject) => {
    const child = spawn('node', ['-e', `
      const http = require('http');
      const server = http.createServer((req, res) => {
        res.setHeader('Content-Type', 'text/html');
        res.end(process.env.SWAPPED_HTML);
      });
      server.listen(0, '127.0.0.1', () => console.log('PORT ' + server.address().port));
    `], { stdio: ['ignore', 'pipe', 'inherit'], env: { ...process.env, SWAPPED_HTML: html } });
    children.push(child);

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
      if (!settled) { settled = true; reject(new Error('swapped server exited before listening')); }
    });
    child.on('error', reject);
  });
}

function runWalk(args, env = {}) {
  return spawnSync('node', [RUN, 'human', '--goal', 'practice-review', ...args],
    { encoding: 'utf8', timeout: 60000, env: { ...process.env, ...env } });
}

describe('candidate handle binding (D1)', () => {
  it('rejects a handle whose revision cannot be verified against the walking checkout', async () => {
    const dir = join(TMP_ROOT, 'e2e-bind-mismatch');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    // A live, matching process: only the revision differs (or, in a bare tree
    // without .git, cannot be compared).
    const keep = startKeepalive(dir);
    const handlePath = writeHandle(dir, {
      revision: 'f'.repeat(40), sha: keep.sha, url: 'http://127.0.0.1:1',
      pid: keep.pid, binary: keep.binary,
    });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    let result;
    try {
      result = runWalk(['--candidate', 'http://127.0.0.1:1/', '--handle', handlePath, '--out', outDir]);
    } finally {
      await keep.stop();
    }

    strictEqual(result.status, 2, 'binding rejection must exit 2, got ' + result.status);
    ok(/candidate binding rejected/i.test(result.stderr), 'stderr must name the rejection: ' + result.stderr);
    ok(existsSync(join(outDir, 'receipt.json')), 'a binding rejection must preserve a receipt');
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    strictEqual(receipt.candidate.bound, true);
    strictEqual(receipt.candidate.revision, 'f'.repeat(40), 'receipt must carry the tested artifact revision');
    strictEqual(receipt.candidate.binary_sha256, keep.sha, 'receipt must carry the tested artifact digest');
    strictEqual(receipt.candidate.handle, handlePath);
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && check.status === 'unverified', 'the walk is blocked, not certified');
    ok(/revision/.test(check.observed), 'observed must explain the rejection: ' + check.observed);
    if (headRevision()) {
      ok(/does not match checkout HEAD/.test(check.observed), 'observed must name the mismatch: ' + check.observed);
    } else {
      ok(/checkout revision unavailable/.test(check.observed), 'observed must name the missing checkout: ' + check.observed);
    }

    rmSync(dir, { recursive: true, force: true });
  });

  it('rejects a missing handle file (exit 2, receipt, unbound candidate)', async () => {
    const dir = join(TMP_ROOT, 'e2e-bind-missing');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    const result = runWalk(['--candidate', 'http://127.0.0.1:1/', '--handle', join(dir, 'nope.json'), '--out', outDir]);

    strictEqual(result.status, 2, 'missing handle must exit 2, got ' + result.status);
    ok(/unreadable/i.test(result.stderr), 'stderr must explain: ' + result.stderr);
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    strictEqual(receipt.candidate.bound, false, 'no identity may be claimed');
    strictEqual(receipt.candidate.revision, null);
    strictEqual(receipt.candidate.binary_sha256, null);

    rmSync(dir, { recursive: true, force: true });
  });

  it('rejects a handle that describes a different target than the walk', async () => {
    const dir = join(TMP_ROOT, 'e2e-bind-target');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const handlePath = writeHandle(dir, { revision: headRevision() ?? 'a'.repeat(40), sha: 'b'.repeat(64), url: 'http://127.0.0.1:19999' });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    const result = runWalk(['--candidate', 'http://127.0.0.1:1/', '--handle', handlePath, '--out', outDir]);

    strictEqual(result.status, 2, 'target mismatch must exit 2, got ' + result.status);
    ok(/candidate binding rejected/i.test(result.stderr), 'stderr must name the rejection: ' + result.stderr);
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && /binding rejected/i.test(check.observed), 'observed: ' + (check && check.observed));

    rmSync(dir, { recursive: true, force: true });
  });

  it('binds a valid handle: the receipt cites the handle revision and digest', async (t) => {
    const revision = headRevision();
    if (!revision) {
      t.skip('not a git checkout; cannot verify the positive binding path');
      return;
    }
    const browser = await resolveBrowser();

    const dir = join(TMP_ROOT, 'e2e-bind-valid');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const keep = startKeepalive(dir);
    const port = await freePort();
    const handlePath = writeHandle(dir, { revision, sha: keep.sha, url: 'http://127.0.0.1:' + port, pid: keep.pid, binary: keep.binary });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    // The target is unreachable, so the walk is blocked — but the receipt must
    // be bound to the handle identity, and the block must come from the real
    // network failure (not a missing browser), proving the binding path ran.
    let result;
    try {
      result = runWalk(['--candidate', 'http://127.0.0.1:' + port + '/', '--handle', handlePath, '--out', outDir]);
    } finally {
      await keep.stop();
    }

    strictEqual(result.status, 2, 'expected blocked exit 2, got ' + result.status);
    ok(!/candidate binding rejected/i.test(result.stderr), 'a live handle must pass verification: ' + result.stderr);
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    strictEqual(receipt.candidate.bound, true);
    strictEqual(receipt.candidate.revision, revision, 'revision must come from the handle');
    strictEqual(receipt.candidate.binary_revision, revision, 'binary revision must come from the handle');
    strictEqual(receipt.candidate.binary_sha256, keep.sha, 'digest must come from the handle');
    strictEqual(receipt.candidate.handle_id, 'cand-test000000');
    if (browser.ok) {
      const check = receipt.checks.find(c => c.id === 'walk-execution');
      ok(/ERR_CONNECTION|ECONNREFUSED/i.test(check.observed), 'expected the real navigation failure: ' + check.observed);
    } else if (REQUIRE_BROWSER) {
      throw new Error('browser required but unavailable: ' + browser.reason);
    }

    rmSync(dir, { recursive: true, force: true });
  });

  it('rejects a stopped handle whose port serves a swapped-in artifact (served-artifact boundary)', async () => {
    const dir = join(TMP_ROOT, 'e2e-bind-stopped');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    // The candidate is stopped; another server occupies its port.
    const swapped = await startSwappedServer('<html><body><h1>swapped-in artifact</h1></body></html>');
    try {
      const handlePath = writeHandle(dir, {
        revision: headRevision() ?? 'a'.repeat(40),
        sha: 'a'.repeat(64),
        url: swapped.url.replace(/\/$/, ''),
        pid: await deadPid(),
        binary: join(dir, 'scry'),
      });

      const result = runWalk(['--candidate', swapped.url, '--handle', handlePath, '--out', outDir]);

      strictEqual(result.status, 2, 'a stopped handle must fail closed, got ' + result.status);
      ok(/candidate binding rejected/i.test(result.stderr), 'stderr must name the rejection: ' + result.stderr);
      ok(/not running/i.test(result.stderr), 'stderr must explain the stopped process: ' + result.stderr);
      const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
      strictEqual(receipt.candidate.bound, true, 'the declared handle identity is kept');
      const check = receipt.checks.find(c => c.id === 'walk-execution');
      ok(check && check.status === 'unverified', 'the walk is blocked, not certified');
      ok(/not running/i.test(check.observed), 'observed must explain: ' + check.observed);
      strictEqual(receipt.findings.length, 0, 'no finding may be attributed to the swapped artifact');

      // Nothing may be disturbed: the swapped-in server is still serving.
      const probe = await fetch(swapped.url).then(r => r.status).catch(() => null);
      strictEqual(probe, 200, 'the swapped-in server must be untouched');
    } finally {
      await swapped.close();
    }

    rmSync(dir, { recursive: true, force: true });
  });

  it('rejects a handle whose binary was replaced after candidate up (digest check)', async () => {
    const dir = join(TMP_ROOT, 'e2e-bind-replaced');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    const keep = startKeepalive(dir, 'candidate-script.sh');
    let result;
    try {
      const port = await freePort();
      const handlePath = writeHandle(dir, {
        revision: headRevision() ?? 'a'.repeat(40),
        sha: keep.sha,
        url: 'http://127.0.0.1:' + port,
        pid: keep.pid,
        binary: keep.binary,
      });
      // A rebuild lands as new bytes behind the same path; the running process
      // keeps the old image.
      const replacement = join(dir, 'candidate-script.rebuilt.sh');
      writeFileSync(replacement, '#!/bin/sh\n# rebuilt after candidate up\n');
      renameSync(replacement, keep.binary);

      result = runWalk(['--candidate', 'http://127.0.0.1:' + port + '/', '--handle', handlePath, '--out', outDir]);
    } finally {
      await keep.stop();
    }

    strictEqual(result.status, 2, 'a replaced binary must fail closed, got ' + result.status);
    ok(/candidate binding rejected/i.test(result.stderr), 'stderr must name the rejection: ' + result.stderr);
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && /changed since candidate up/i.test(check.observed), 'observed must explain the digest change: ' + (check && check.observed));

    rmSync(dir, { recursive: true, force: true });
  });

  it('rejects a live pid that does not run the handle binary (command-line check)', async () => {
    const dir = join(TMP_ROOT, 'e2e-bind-foreign-pid');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    const binaryPath = join(dir, 'candidate-binary.sh');
    writeFileSync(binaryPath, '#!/bin/sh\nexit 0\n');
    chmodSync(binaryPath, 0o755);
    const port = await freePort();
    // The pid is alive (this test process) but it does not run the handle
    // binary; the digest matches, so only the command-line check can reject.
    const handlePath = writeHandle(dir, {
      revision: headRevision() ?? 'a'.repeat(40),
      sha: sha256File(binaryPath),
      url: 'http://127.0.0.1:' + port,
      pid: process.pid,
      binary: binaryPath,
    });

    const result = runWalk(['--candidate', 'http://127.0.0.1:' + port + '/', '--handle', handlePath, '--out', outDir]);

    strictEqual(result.status, 2, 'a foreign pid must fail closed, got ' + result.status);
    ok(/candidate binding rejected/i.test(result.stderr), 'stderr must name the rejection: ' + result.stderr);
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && /does not run the handle binary/i.test(check.observed), 'observed must explain: ' + (check && check.observed));

    rmSync(dir, { recursive: true, force: true });
  });

  it('rejects a loopback handle that records no determinable binary revision (binary_revision null)', async () => {
    const dir = join(TMP_ROOT, 'e2e-bind-null-revision');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    const keep = startKeepalive(dir);
    let result;
    try {
      const port = await freePort();
      const handlePath = writeHandle(dir, {
        revision: headRevision() ?? 'a'.repeat(40),
        sha: keep.sha,
        url: 'http://127.0.0.1:' + port,
        pid: keep.pid,
        binary: keep.binary,
        binaryRevision: null,
      });

      result = runWalk(['--candidate', 'http://127.0.0.1:' + port + '/', '--handle', handlePath, '--out', outDir]);
    } finally {
      await keep.stop();
    }

    strictEqual(result.status, 2, 'an undeterminable binary revision must fail closed, got ' + result.status);
    ok(/candidate binding rejected/i.test(result.stderr), 'stderr must name the rejection: ' + result.stderr);
    ok(/binary revision/.test(result.stderr), 'stderr must explain the missing provenance: ' + result.stderr);
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    strictEqual(receipt.candidate.bound, true, 'the declared handle identity is kept');
    strictEqual(receipt.candidate.binary_revision, null, 'no provenance may be claimed');
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && check.status === 'unverified', 'the walk is blocked, not certified');
    ok(/binary revision/.test(check.observed), 'observed must explain: ' + check.observed);

    rmSync(dir, { recursive: true, force: true });
  });

  it('rejects a pre-fix loopback handle that omits binary_revision entirely', async () => {
    const dir = join(TMP_ROOT, 'e2e-bind-missing-revision');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    const keep = startKeepalive(dir);
    let result;
    try {
      const port = await freePort();
      const handlePath = writeHandle(dir, {
        revision: headRevision() ?? 'a'.repeat(40),
        sha: keep.sha,
        url: 'http://127.0.0.1:' + port,
        pid: keep.pid,
        binary: keep.binary,
        binaryRevision: undefined,
      });

      result = runWalk(['--candidate', 'http://127.0.0.1:' + port + '/', '--handle', handlePath, '--out', outDir]);
    } finally {
      await keep.stop();
    }

    strictEqual(result.status, 2, 'a handle without a binary revision must fail closed, got ' + result.status);
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && /binary revision/.test(check.observed), 'observed must explain: ' + (check && check.observed));

    rmSync(dir, { recursive: true, force: true });
  });

  it('rejects a handle whose binary revision differs from the claimed revision', async () => {
    const dir = join(TMP_ROOT, 'e2e-bind-foreign-revision');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    const keep = startKeepalive(dir);
    let result;
    try {
      const port = await freePort();
      const handlePath = writeHandle(dir, {
        revision: headRevision() ?? 'a'.repeat(40),
        sha: keep.sha,
        url: 'http://127.0.0.1:' + port,
        pid: keep.pid,
        binary: keep.binary,
        binaryRevision: 'e'.repeat(40),
      });

      result = runWalk(['--candidate', 'http://127.0.0.1:' + port + '/', '--handle', handlePath, '--out', outDir]);
    } finally {
      await keep.stop();
    }

    strictEqual(result.status, 2, 'a foreign binary revision must fail closed, got ' + result.status);
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    strictEqual(receipt.candidate.bound, true, 'the declared handle identity is kept');
    strictEqual(receipt.candidate.binary_revision, null, 'the blocked receipt must not carry the contradictory pair');
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && /does not match handle revision/i.test(check.observed), 'observed must explain: ' + (check && check.observed));

    rmSync(dir, { recursive: true, force: true });
  });

  it('does not process-verify non-loopback handles (--allow-origin path unaffected)', async () => {
    const dir = join(TMP_ROOT, 'e2e-bind-external');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    const revision = headRevision();
    const handlePath = writeHandle(dir, {
      revision: revision ?? 'a'.repeat(40),
      sha: 'd'.repeat(64),
      url: 'http://example.invalid:9',
      pid: 999999,
      binary: '/nonexistent/scry',
    });

    const result = runWalk([
      '--candidate', 'http://example.invalid:9/', '--allow-origin', 'http://example.invalid:9',
      '--handle', handlePath, '--out', outDir,
    ]);

    strictEqual(result.status, 2, 'blocked by the unreachable target, got ' + result.status);
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    const check = receipt.checks.find(c => c.id === 'walk-execution') ?? {};

    // The loopback process check must never fire for a non-loopback handle:
    // its dead pid and missing binary may not surface as the rejection.
    const combined = (result.stderr || '') + ' | ' + (check.observed ?? '');
    ok(!/not running|does not run the handle binary|changed since candidate up/i.test(combined),
      'the loopback process check must not apply: ' + combined);

    if (revision) {
      // With a checkout the binding is accepted; the walk proceeds and is
      // blocked by the unreachable target, not by binding.
      ok(!/candidate binding rejected/i.test(result.stderr), 'binding must be accepted: ' + result.stderr);
      strictEqual(receipt.candidate.bound, true);
      ok(check.status === 'unverified' && !/binding rejected/i.test(check.observed),
        'observation must come from the walk attempt: ' + check.observed);
    } else {
      // A bare tree cannot compare revisions, so the walk rejects at the
      // revision check — never through the process check.
      ok(/checkout revision unavailable/i.test(check.observed ?? ''), 'observed: ' + check.observed);
    }

    rmSync(dir, { recursive: true, force: true });
  });
});