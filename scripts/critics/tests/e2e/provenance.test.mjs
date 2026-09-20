import { describe, it, after } from 'node:test';
import { strictEqual, ok } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync, mkdtempSync, mkdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer as createNetServer } from 'node:net';
import { resolveBrowser } from '../../lib/browser.mjs';
import { readBinaryProvenance, sourceStateFromProvenance } from '../../lib/provenance.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const CRITICS_DIR = join(__dirname, '..', '..');
const REPO_ROOT = join(__dirname, '..', '..', '..', '..');
const RUN = join(CRITICS_DIR, 'run.mjs');
const REQUIRE_BROWSER = process.env.SCRY_CRITICS_REQUIRE_BROWSER === '1';
// Test artifacts stay OUT of the repository tree (the gate re-inventories it).
const TMP_ROOT = mkdtempSync(join(tmpdir(), 'scry-critics-provenance-e2e-'));
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

function headRevision() {
  const r = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: REPO_ROOT, encoding: 'utf8' });
  return r.status === 0 ? r.stdout.trim() : null;
}

function goAvailable() {
  return spawnSync('go', ['version'], { encoding: 'utf8' }).status === 0;
}

function buildScry(name, flags = []) {
  const out = join(TMP_ROOT, name);
  const r = spawnSync('go', ['build', '-mod=readonly', ...flags, '-o', out, './cmd/scry'],
    { cwd: REPO_ROOT, encoding: 'utf8', timeout: 120000 });
  if (r.status !== 0) throw new Error('go build failed: ' + (r.stderr || r.error?.message || 'unknown'));
  return out;
}

function runCandidateUp(args) {
  return spawnSync('node', [RUN, 'candidate', 'up', ...args], { encoding: 'utf8', timeout: 90000 });
}

function runCandidateDown(dir) {
  return spawnSync('node', [RUN, 'candidate', 'down', '--dir', dir, '--out', join(dir, 'candidate.json')],
    { encoding: 'utf8', timeout: 30000 });
}

function runWalk(args) {
  return spawnSync('node', [RUN, 'human', '--goal', 'practice-review', ...args],
    { encoding: 'utf8', timeout: 90000 });
}

describe('--binary build provenance (fail closed on a foreign or undeterminable revision)', () => {
  it('refuses a --binary whose own revision differs from the checkout (no handle written)', async (t) => {
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const dir = join(TMP_ROOT, 'e2e-prov-foreign');
    rmSync(dir, { recursive: true, force: true });
    const stamp = 'a'.repeat(40);
    const binary = buildScry('foreign-stamped-scry', ['-buildvcs=false', '-ldflags', `-X main.revision=${stamp}`]);
    const port = await freePort();

    const result = runCandidateUp(['--binary', binary, '--dir', dir, '--port', String(port), '--out', join(dir, 'candidate.json')]);

    strictEqual(result.status, 2, 'a foreign revision must fail closed, got ' + result.status + ': ' + result.stdout + result.stderr);
    ok(/refusing/.test(result.stderr), 'stderr must explain the refusal: ' + result.stderr);
    ok(result.stderr.includes(stamp), 'stderr must name the binary revision: ' + result.stderr);
    strictEqual(existsSync(join(dir, 'candidate.json')), false, 'no handle may be written for a foreign binary');
  });

  it('keeps the same-revision --binary path green (bound pass receipt cites the binary revision)', async (t) => {
    const head = headRevision();
    if (!head) return t.skip('not a git checkout; --binary provenance needs a checkout to compare');
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const browser = await resolveBrowser();
    if (!browser.ok && REQUIRE_BROWSER) throw new Error('browser required but unavailable: ' + browser.reason);
    if (!browser.ok) return t.skip('browser unavailable: ' + browser.reason);

    const dir = join(TMP_ROOT, 'e2e-prov-same');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const binary = buildScry('same-rev-scry');
    const port = await freePort();

    const up = runCandidateUp(['--binary', binary, '--dir', dir, '--port', String(port), '--out', join(dir, 'candidate.json')]);
    strictEqual(up.status, 0, 'a same-revision --binary must start: ' + up.stdout + up.stderr);
    try {
      const handle = JSON.parse(readFileSync(join(dir, 'candidate.json'), 'utf8'));
      strictEqual(handle.revision, head, 'handle must record the checkout revision');
      strictEqual(handle.binary_revision, head, 'handle must record the binary\u2019s own revision');
      strictEqual(handle.binary_revision_source, 'buildinfo-vcs');
      // The state label must come from the binary's own evidence (a clean
      // plain build in a clean checkout stays green), never from a guess
      // about the walking checkout.
      const expectedState = sourceStateFromProvenance(readBinaryProvenance(binary));
      strictEqual(handle.source_state, expectedState.state, 'handle source_state must describe the binary');
      strictEqual(handle.source_state_source, expectedState.source);

      const walk = runWalk(['--candidate', 'http://127.0.0.1:' + port, '--handle', join(dir, 'candidate.json'), '--out', join(dir, 'run')]);
      strictEqual(walk.status, 0, 'a verified same-revision candidate must stay green: ' + walk.stdout + walk.stderr);
      const receipt = JSON.parse(readFileSync(join(dir, 'run', 'receipt.json'), 'utf8'));
      strictEqual(receipt.candidate.bound, true);
      strictEqual(receipt.candidate.revision, head);
      strictEqual(receipt.candidate.binary_revision, head, 'receipt must carry the verified binary revision');
      strictEqual(receipt.candidate.source_state, expectedState.state, 'receipt must carry the binary\u2019s own state');
      ok(receipt.checks.every(c => c.status === 'pass'),
        'checks must all pass: ' + receipt.checks.map(c => c.id + ':' + c.status).join(', '));
    } finally {
      runCandidateDown(dir);
    }
  });

  it('records binary_revision: null for an undeterminable --binary and blocks the walk on it', async (t) => {
    if (!headRevision()) return t.skip('not a git checkout; the provenance rejection needs a checkout revision');
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const dir = join(TMP_ROOT, 'e2e-prov-unknown');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const binary = buildScry('novcs-scry', ['-buildvcs=false']);
    const port = await freePort();

    const up = runCandidateUp(['--binary', binary, '--dir', dir, '--port', String(port), '--out', join(dir, 'candidate.json')]);
    strictEqual(up.status, 0, 'an undeterminable binary must still record a candidate: ' + up.stdout + up.stderr);
    try {
      const handle = JSON.parse(readFileSync(join(dir, 'candidate.json'), 'utf8'));
      strictEqual(handle.binary_revision, null, 'the handle must not assert a provenance it cannot verify');
      strictEqual(handle.binary_revision_source, null);
      strictEqual(handle.source_state, 'unknown', 'the handle must not borrow the checkout\u2019s state either');
      strictEqual(handle.source_state_source, null);

      const walk = runWalk(['--candidate', 'http://127.0.0.1:' + port, '--handle', join(dir, 'candidate.json'), '--out', join(dir, 'run')]);
      strictEqual(walk.status, 2, 'the walk must fail closed, got ' + walk.status + ': ' + walk.stdout + walk.stderr);
      ok(/candidate binding rejected/i.test(walk.stderr), 'stderr must name the rejection: ' + walk.stderr);
      ok(/binary revision/.test(walk.stderr), 'stderr must explain the missing provenance: ' + walk.stderr);
      const receipt = JSON.parse(readFileSync(join(dir, 'run', 'receipt.json'), 'utf8'));
      const check = receipt.checks.find(c => c.id === 'walk-execution');
      ok(check && check.status === 'unverified', 'the walk is blocked, not certified');
      ok(/binary revision/.test(check.observed), 'observed must explain: ' + check.observed);
      strictEqual(receipt.candidate.source_state, 'unknown', 'the receipt must not claim a state the binary cannot prove');
    } finally {
      runCandidateDown(dir);
    }
  });

  it('records source_state: dirty for a --binary built from a modified tree (isolated clone) and keeps the walk bound', async (t) => {
    const head = headRevision();
    if (!head) return t.skip('not a git checkout; a modified-tree binary needs a checkout revision to compare');
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const browser = await resolveBrowser();
    if (!browser.ok && REQUIRE_BROWSER) throw new Error('browser required but unavailable: ' + browser.reason);
    if (!browser.ok) return t.skip('browser unavailable: ' + browser.reason);

    // Build at the checkout revision from a modified tree, entirely inside an
    // isolated clone: the shared checkout is never mutated.
    const clone = join(TMP_ROOT, 'e2e-prov-dirty-clone');
    rmSync(clone, { recursive: true, force: true });
    const cloned = spawnSync('git', ['clone', '-q', '--shared', REPO_ROOT, clone], { encoding: 'utf8', timeout: 60000 });
    if (cloned.status !== 0) return t.skip('cannot clone the checkout for the modified-tree fixture: ' + cloned.stderr);
    const cloneHead = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: clone, encoding: 'utf8' }).stdout.trim();
    strictEqual(cloneHead, head, 'the clone must start at the checkout revision');

    const binary = join(TMP_ROOT, 'e2e-prov-dirty-scry');
    const mainGo = join(clone, 'cmd', 'scry', 'main.go');
    writeFileSync(mainGo, readFileSync(mainGo, 'utf8') + '\n// dirty probe\n');
    const built = spawnSync('go', ['build', '-mod=readonly', '-o', binary, './cmd/scry'],
      { cwd: clone, encoding: 'utf8', timeout: 180000 });
    if (built.status !== 0) throw new Error('modified-tree build failed: ' + (built.stderr || built.error?.message));
    // Restore the clone so only the built bytes carry the modified evidence.
    spawnSync('git', ['checkout', '--', 'cmd/scry/main.go'], { cwd: clone, encoding: 'utf8' });

    const evidence = readBinaryProvenance(binary);
    strictEqual(evidence.revision, head, 'fixture binary must be built at the checkout revision');
    strictEqual(evidence.modified, true, 'fixture binary must carry vcs.modified=true');

    const dir = join(TMP_ROOT, 'e2e-prov-dirty');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    const port = await freePort();

    const up = runCandidateUp(['--binary', binary, '--dir', dir, '--port', String(port), '--out', join(dir, 'candidate.json')]);
    strictEqual(up.status, 0, 'a modified-tree --binary at the checkout revision must still start: ' + up.stdout + up.stderr);
    try {
      const handle = JSON.parse(readFileSync(join(dir, 'candidate.json'), 'utf8'));
      strictEqual(handle.binary_revision, head);
      strictEqual(handle.source_state, 'dirty', 'a binary built from a modified tree must never be labeled clean');
      strictEqual(handle.source_state_source, 'buildinfo-vcs');

      const walk = runWalk(['--candidate', 'http://127.0.0.1:' + port, '--handle', join(dir, 'candidate.json'), '--out', join(dir, 'run')]);
      strictEqual(walk.status, 0, 'the walk stays bound for an honestly labeled dirty binary: ' + walk.stdout + walk.stderr);
      const receipt = JSON.parse(readFileSync(join(dir, 'run', 'receipt.json'), 'utf8'));
      strictEqual(receipt.candidate.bound, true);
      strictEqual(receipt.candidate.source_state, 'dirty', 'the receipt must carry the binary\u2019s own state');
      ok(receipt.checks.every(c => c.status === 'pass'),
        'checks must all pass: ' + receipt.checks.map(c => c.id + ':' + c.status).join(', '));
    } finally {
      runCandidateDown(dir);
    }
  });

  it('records source_state: unknown for a stamped same-revision --binary (no tree-state evidence) and keeps the walk bound', async (t) => {
    const head = headRevision();
    if (!head) return t.skip('not a git checkout; a stamped same-revision binary needs a checkout revision');
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const browser = await resolveBrowser();
    if (!browser.ok && REQUIRE_BROWSER) throw new Error('browser required but unavailable: ' + browser.reason);
    if (!browser.ok) return t.skip('browser unavailable: ' + browser.reason);

    const dir = join(TMP_ROOT, 'e2e-prov-stamped');
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
    // Gate-export shape: a determinable revision via the stamp, no vcs
    // evidence — the tree state is undeterminable from the artifact.
    const binary = buildScry('stamped-same-scry', ['-buildvcs=false', '-ldflags', `-X main.revision=${head}`]);
    const port = await freePort();

    const up = runCandidateUp(['--binary', binary, '--dir', dir, '--port', String(port), '--out', join(dir, 'candidate.json')]);
    strictEqual(up.status, 0, 'a stamped same-revision --binary must start: ' + up.stdout + up.stderr);
    try {
      const handle = JSON.parse(readFileSync(join(dir, 'candidate.json'), 'utf8'));
      strictEqual(handle.binary_revision, head);
      strictEqual(handle.binary_revision_source, 'buildinfo-ldflags');
      strictEqual(handle.source_state, 'unknown', 'a stamp carries no tree state; unknown is the honest label');
      strictEqual(handle.source_state_source, null);

      const walk = runWalk(['--candidate', 'http://127.0.0.1:' + port, '--handle', join(dir, 'candidate.json'), '--out', join(dir, 'run')]);
      strictEqual(walk.status, 0, 'a determinable revision still binds, labeled unknown: ' + walk.stdout + walk.stderr);
      const receipt = JSON.parse(readFileSync(join(dir, 'run', 'receipt.json'), 'utf8'));
      strictEqual(receipt.candidate.bound, true);
      strictEqual(receipt.candidate.source_state, 'unknown');
      ok(receipt.checks.every(c => c.status === 'pass'),
        'checks must all pass: ' + receipt.checks.map(c => c.id + ':' + c.status).join(', '));
    } finally {
      runCandidateDown(dir);
    }
  });
});
