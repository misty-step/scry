import { describe, it } from 'node:test';
import { strictEqual, ok } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync, mkdirSync, rmSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer as createNetServer } from 'node:net';
import { resolveBrowser } from '../../lib/browser.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const CRITICS_DIR = join(__dirname, '..', '..');
const REPO_ROOT = join(__dirname, '..', '..', '..', '..');
const RUN = join(CRITICS_DIR, 'run.mjs');

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

function writeHandle(dir, { revision, sha, url, id, sourceState = 'clean' }) {
  const path = join(dir, 'candidate.json');
  writeFileSync(path, JSON.stringify({
    format: 'scry-critic-candidate-v1',
    id: id ?? 'cand-test000000',
    pid: 999999,
    url,
    binary_sha256: sha,
    revision,
    source_state: sourceState,
    seed: { model: 'authored-test-fixture', source: 'test' },
  }, null, 2));
  return path;
}

function runWalk(args, env = {}) {
  return spawnSync('node', [RUN, 'human', '--goal', 'practice-review', ...args],
    { encoding: 'utf8', timeout: 60000, env: { ...process.env, ...env } });
}

describe('candidate handle binding (D1)', () => {
  it('rejects a mismatched handle revision; the receipt carries the handle identity, not the checkout', async () => {
    const dir = join(REPO_ROOT, 'target/critics/e2e-bind-mismatch');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const handlePath = writeHandle(dir, { revision: 'f'.repeat(40), sha: 'a'.repeat(64), url: 'http://127.0.0.1:1' });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    const result = runWalk(['--candidate', 'http://127.0.0.1:1/', '--handle', handlePath, '--out', outDir]);

    strictEqual(result.status, 2, 'binding rejection must exit 2, got ' + result.status);
    ok(/candidate binding rejected/i.test(result.stderr), 'stderr must name the rejection: ' + result.stderr);
    ok(existsSync(join(outDir, 'receipt.json')), 'a binding rejection must preserve a receipt');
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    strictEqual(receipt.candidate.bound, true);
    strictEqual(receipt.candidate.revision, 'f'.repeat(40), 'receipt must carry the tested artifact revision');
    strictEqual(receipt.candidate.binary_sha256, 'a'.repeat(64), 'receipt must carry the tested artifact digest');
    strictEqual(receipt.candidate.handle, handlePath);
    const check = receipt.checks.find(c => c.id === 'walk-execution');
    ok(check && check.status === 'unverified', 'the walk is blocked, not certified');
    ok(/revision/.test(check.observed), 'observed must explain the rejection: ' + check.observed);

    rmSync(dir, { recursive: true, force: true });
  });

  it('rejects a missing handle file (exit 2, receipt, unbound candidate)', async () => {
    const dir = join(REPO_ROOT, 'target/critics/e2e-bind-missing');
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
    const dir = join(REPO_ROOT, 'target/critics/e2e-bind-target');
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

    const dir = join(REPO_ROOT, 'target/critics/e2e-bind-valid');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const sha = 'c'.repeat(64);
    const port = await freePort();
    const handlePath = writeHandle(dir, { revision, sha, url: 'http://127.0.0.1:' + port });
    const outDir = join(dir, 'out');
    mkdirSync(outDir);

    // The target is unreachable, so the walk is blocked — but the receipt must
    // be bound to the handle identity, and the block must come from the real
    // network failure (not a missing browser), proving the binding path ran.
    const result = runWalk(['--candidate', 'http://127.0.0.1:' + port + '/', '--handle', handlePath, '--out', outDir]);

    strictEqual(result.status, 2, 'expected blocked exit 2, got ' + result.status);
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    strictEqual(receipt.candidate.bound, true);
    strictEqual(receipt.candidate.revision, revision, 'revision must come from the handle');
    strictEqual(receipt.candidate.binary_sha256, sha, 'digest must come from the handle');
    strictEqual(receipt.candidate.handle_id, 'cand-test000000');
    if (browser.ok) {
      const check = receipt.checks.find(c => c.id === 'walk-execution');
      ok(/ERR_CONNECTION|ECONNREFUSED/i.test(check.observed), 'expected the real navigation failure: ' + check.observed);
    } else if (process.env.SCRY_CRITICS_REQUIRE_BROWSER === '1') {
      throw new Error('browser required but unavailable: ' + browser.reason);
    }

    rmSync(dir, { recursive: true, force: true });
  });
});