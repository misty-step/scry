import { describe, it, after } from 'node:test';
import { strictEqual, ok } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, writeFileSync, chmodSync, mkdirSync, rmSync, readFileSync, readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { createServer } from 'node:http';
import { createServer as createNetServer } from 'node:net';
import { fileURLToPath } from 'node:url';
import { readyAttempt } from '../lib/ready.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const CRITICS_DIR = join(__dirname, '..');
// Test artifacts stay OUT of the repository tree (the gate re-inventories it).
const TMP_ROOT = mkdtempSync(join(tmpdir(), 'scry-critics-candidate-'));
after(() => rmSync(TMP_ROOT, { recursive: true, force: true }));

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

describe('candidate lifecycle fail-closed (D5)', () => {
  it('refuses a busy port instead of a false ready (exit 2, no handle, occupier untouched)', async () => {
    const occupier = createServer((req, res) => res.end(req.url === '/readyz' ? 'ready' : 'ok'));
    await new Promise(resolve => occupier.listen(0, '127.0.0.1', resolve));
    const port = occupier.address().port;

    const dir = join(TMP_ROOT, 'e2e-cand-busy');
    rmSync(dir, { recursive: true, force: true });

    const result = spawnSync('node', [
      join(CRITICS_DIR, 'run.mjs'), 'candidate', 'up',
      '--dir', dir, '--port', String(port), '--out', join(dir, 'candidate.json'),
    ], { encoding: 'utf8', timeout: 60000 });

    strictEqual(result.status, 2, 'a busy port must fail the candidate up: ' + result.stdout + result.stderr);
    ok(/already in use/i.test(result.stderr), 'stderr must name the conflict: ' + result.stderr);
    strictEqual(existsSync(join(dir, 'candidate.json')), false, 'no handle may claim a candidate that does not serve');

    // The occupier must still be serving — nothing was killed.
    const check = await fetch('http://127.0.0.1:' + port + '/readyz').then(r => r.text()).catch(() => null);
    strictEqual(check, 'ready', 'the occupier must be untouched');

    await new Promise(resolve => occupier.close(resolve));
    rmSync(dir, { recursive: true, force: true });
  });

  it('detects a server that exits before ready (exit 2, no handle)', async () => {
    const dir = join(TMP_ROOT, 'e2e-cand-dies');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });

    const fakeServer = join(dir, 'fake-scry.sh');
    writeFileSync(fakeServer, '#!/bin/sh\nif [ "$1" = "seed-fixture" ]; then echo \'{"questions": 2}\'; exit 0; fi\nexit 1\n');
    chmodSync(fakeServer, 0o755);

    const port = await freePort();
    const result = spawnSync('node', [
      join(CRITICS_DIR, 'run.mjs'), 'candidate', 'up',
      '--dir', join(dir, 'candidate'), '--binary', fakeServer,
      '--port', String(port), '--out', join(dir, 'candidate', 'candidate.json'),
    ], { encoding: 'utf8', timeout: 60000 });

    strictEqual(result.status, 2, 'a dead server must fail the candidate up: ' + result.stdout + result.stderr);
    ok(/exited before becoming ready/i.test(result.stderr), 'stderr must explain: ' + result.stderr);
    strictEqual(existsSync(join(dir, 'candidate', 'candidate.json')), false, 'no handle for a server that never served');

    rmSync(dir, { recursive: true, force: true });
  });

  it('creates missing parent directories (pristine checkout): clean failure, no ENOENT crash', async () => {
    // The documented `candidate up` runs from the repo root. In a pristine
    // checkout the candidate dir has no parents yet; a non-recursive mkdir
    // crashed with a raw ENOENT (exit 1 + stack trace) instead of reaching
    // normal failure handling.
    const dir = join(TMP_ROOT, 'pristine-family', 'nested', 'deeper', 'candidate');
    rmSync(join(TMP_ROOT, 'pristine-family'), { recursive: true, force: true });

    const fakeServer = join(TMP_ROOT, 'pristine-fake-scry.sh');
    writeFileSync(fakeServer, '#!/bin/sh\nif [ "$1" = "seed-fixture" ]; then echo \'{"questions": 2}\'; exit 0; fi\nexit 1\n');
    chmodSync(fakeServer, 0o755);

    const port = await freePort();
    const result = spawnSync('node', [
      join(CRITICS_DIR, 'run.mjs'), 'candidate', 'up',
      '--dir', dir, '--binary', fakeServer,
      '--port', String(port), '--out', join(dir, 'candidate.json'),
    ], { encoding: 'utf8', timeout: 60000 });

    strictEqual(result.status, 2, 'must fail cleanly (exit 2), got ' + result.status + ': ' + result.stderr);
    ok(!/ENOENT/.test(result.stderr), 'the parent dirs must be created, not crash: ' + result.stderr);
    ok(/exited before becoming ready/i.test(result.stderr), 'must reach the serving step: ' + result.stderr);
    ok(existsSync(dir), 'the candidate dir tree must exist');

    rmSync(join(TMP_ROOT, 'pristine-family'), { recursive: true, force: true });
    rmSync(fakeServer, { force: true });
  });

  it('bounds each readiness attempt: a server that never answers still settles', async () => {
    const sockets = new Set();
    const server = createNetServer((socket) => {
      sockets.add(socket);
      socket.on('close', () => sockets.delete(socket));
    });
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    const port = server.address().port;

    const started = Date.now();
    let settled = false;
    try {
      await readyAttempt('http://127.0.0.1:' + port, { timeoutMs: 300 });
    } catch (_) {
      settled = true;
    }
    const elapsed = Date.now() - started;

    ok(settled, 'the attempt must settle instead of staying pending forever');
    ok(elapsed < 5000, 'the attempt must be bounded by its timeout, took ' + elapsed + 'ms');

    for (const socket of sockets) socket.destroy();
    await new Promise(resolve => server.close(resolve));
  });

  it('advances past a ready attempt that never answers (the attempt bound must not stall)', async () => {
    const dir = join(TMP_ROOT, 'e2e-cand-hang');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });

    const port = await freePort();
    // The first /readyz connection hangs (accept, never answer); the second
    // answers. Without a bounded attempt the loop could never reach it.
    const fakeServer = join(dir, 'hang-then-ready.cjs');
    writeFileSync(fakeServer, '#!' + process.execPath + `
const net = require('node:net');
const args = process.argv.slice(2);
if (args[0] === 'seed-fixture') { process.stdout.write('{"questions": 2}\\n'); process.exit(0); }
if (args[0] === 'serve') {
  const addr = args[args.indexOf('--addr') + 1];
  const port = Number(addr.split(':')[1]);
  let connections = 0;
  const server = net.createServer((socket) => {
    connections += 1;
    if (connections === 1) return; // accept, never answer
    socket.end('HTTP/1.1 200 OK\\r\\nContent-Length: 5\\r\\nConnection: close\\r\\n\\r\\nready');
  });
  server.listen(port, '127.0.0.1');
  setInterval(() => {}, 60000);
}
`);
    chmodSync(fakeServer, 0o755);

    const result = spawnSync('node', [
      join(CRITICS_DIR, 'run.mjs'), 'candidate', 'up',
      '--dir', join(dir, 'candidate'), '--binary', fakeServer,
      '--port', String(port), '--out', join(dir, 'candidate', 'candidate.json'),
    ], { encoding: 'utf8', timeout: 60000 });

    strictEqual(result.status, 0, 'the candidate must become ready on a later attempt: ' + result.stdout + result.stderr);
    ok(existsSync(join(dir, 'candidate', 'candidate.json')), 'a handle must be written once ready');

    spawnSync('node', [
      join(CRITICS_DIR, 'run.mjs'), 'candidate', 'down',
      '--dir', join(dir, 'candidate'), '--out', join(dir, 'candidate', 'candidate.json'),
    ], { encoding: 'utf8', timeout: 30000 });

    rmSync(dir, { recursive: true, force: true });
  });
});

describe('candidate dir freshness (data safety: never delete content the tool did not create)', () => {
  // A candidate must become ready: `seed-fixture` returns JSON, `serve` answers
  // /readyz. A child process survives the blocking spawnSync CLI.
  function writeReadyFakeServer(path) {
    writeFileSync(path, '#!' + process.execPath + `
const http = require('node:http');
const args = process.argv.slice(2);
if (args[0] === 'seed-fixture') { process.stdout.write('{"questions": 2}\\n'); process.exit(0); }
if (args[0] === 'serve') {
  const addr = args[args.indexOf('--addr') + 1];
  const port = Number(addr.split(':')[1]);
  http.createServer((req, res) => { res.end(req.url === '/readyz' ? 'ready' : 'ok'); }).listen(port, '127.0.0.1');
}
`);
    chmodSync(path, 0o755);
  }

  it('refuses a pre-existing non-empty dir: exit 2, content untouched, no handle', async () => {
    const family = join(TMP_ROOT, 'nonfresh');
    const dir = join(family, 'precious');
    rmSync(family, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    writeFileSync(join(dir, 'keep.txt'), 'keep-me\n');
    writeFileSync(join(dir, 'evidence-receipt.json'), '{"format":"evidence"}\n');
    const before = readdirSync(dir).sort();

    const fakeServer = join(TMP_ROOT, 'nonfresh-fake-scry.sh');
    writeFileSync(fakeServer, '#!/bin/sh\nif [ "$1" = "seed-fixture" ]; then echo \'{"questions": 2}\'; exit 0; fi\nexit 1\n');
    chmodSync(fakeServer, 0o755);

    const port = await freePort();
    const result = spawnSync('node', [
      join(CRITICS_DIR, 'run.mjs'), 'candidate', 'up',
      '--dir', dir, '--binary', fakeServer,
      '--port', String(port), '--out', join(dir, 'candidate.json'),
    ], { encoding: 'utf8', timeout: 60000 });

    strictEqual(result.status, 2, 'a non-fresh dir must be refused: ' + result.stdout + result.stderr);
    ok(/not fresh/i.test(result.stderr), 'stderr must say the dir is not fresh: ' + result.stderr);
    strictEqual(readFileSync(join(dir, 'keep.txt'), 'utf8'), 'keep-me\n', 'unrelated content must survive');
    strictEqual(existsSync(join(dir, 'evidence-receipt.json')), true, 'the unrelated evidence file must survive');
    strictEqual(readdirSync(dir).sort().join(','), before.join(','), 'the refused dir must be untouched');
    strictEqual(existsSync(join(dir, 'candidate.json')), false, 'no handle may be written for a refused dir');

    rmSync(family, { recursive: true, force: true });
    rmSync(fakeServer, { force: true });
  });

  it('creates a missing dir and reuses an existing empty dir (fresh paths work)', async () => {
    const family = join(TMP_ROOT, 'fresh-paths');
    rmSync(family, { recursive: true, force: true });
    mkdirSync(family, { recursive: true });

    const fakeServer = join(family, 'ready-fake-scry.cjs');
    writeReadyFakeServer(fakeServer);

    const missingDir = join(family, 'missing', 'candidate');
    const emptyDir = join(family, 'empty', 'candidate');
    mkdirSync(emptyDir, { recursive: true });

    for (const dir of [missingDir, emptyDir]) {
      const port = await freePort();
      const result = spawnSync('node', [
        join(CRITICS_DIR, 'run.mjs'), 'candidate', 'up',
        '--dir', dir, '--binary', fakeServer,
        '--port', String(port), '--out', join(dir, 'candidate.json'),
      ], { encoding: 'utf8', timeout: 60000 });
      strictEqual(result.status, 0, 'fresh path must work (' + dir + '): ' + result.stdout + result.stderr);
      strictEqual(existsSync(join(dir, 'candidate.json')), true, 'a handle must be written for ' + dir);

      const down = spawnSync('node', [
        join(CRITICS_DIR, 'run.mjs'), 'candidate', 'down',
        '--dir', dir, '--out', join(dir, 'candidate.json'),
      ], { encoding: 'utf8', timeout: 30000 });
      strictEqual(down.status, 0, 'candidate down must stop the candidate: ' + down.stderr);
    }

    rmSync(family, { recursive: true, force: true });
  });
});