import { describe, it } from 'node:test';
import { strictEqual, ok } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { existsSync, writeFileSync, chmodSync, mkdirSync, rmSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { createServer } from 'node:http';
import { createServer as createNetServer } from 'node:net';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const CRITICS_DIR = join(__dirname, '..');
const REPO_ROOT = join(__dirname, '..', '..', '..');

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

    const dir = join(REPO_ROOT, 'target/critics/e2e-cand-busy');
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
    const dir = join(REPO_ROOT, 'target/critics/e2e-cand-dies');
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
});