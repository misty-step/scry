import { describe, it, after } from 'node:test';
import { strictEqual } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync, chmodSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { readBinaryRevision } from '../lib/provenance.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const REPO_ROOT = join(__dirname, '..', '..', '..');
// Test artifacts stay OUT of the repository tree (the gate re-inventories it).
const TMP_ROOT = mkdtempSync(join(tmpdir(), 'scry-critics-provenance-'));
after(() => rmSync(TMP_ROOT, { recursive: true, force: true }));

const STAMP = 'a'.repeat(40);

function goAvailable() {
  return spawnSync('go', ['version'], { encoding: 'utf8' }).status === 0;
}

function headRevision() {
  const r = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: REPO_ROOT, encoding: 'utf8' });
  return r.status === 0 ? r.stdout.trim() : null;
}

function buildScry(name, flags = []) {
  const out = join(TMP_ROOT, name);
  const r = spawnSync('go', ['build', '-mod=readonly', ...flags, '-o', out, './cmd/scry'],
    { cwd: REPO_ROOT, encoding: 'utf8', timeout: 120000 });
  if (r.status !== 0) throw new Error('go build failed: ' + (r.stderr || r.error?.message || 'unknown'));
  return out;
}

describe('binary provenance (--binary must not borrow the checkout revision)', () => {
  it('reads vcs.revision from a plain build (and it equals the checkout)', (t) => {
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const head = headRevision();
    if (!head) return t.skip('not a git checkout; a plain build carries no vcs stamp here');
    const bin = buildScry('plain-scry');
    const got = readBinaryRevision(bin);
    strictEqual(got.revision, head);
    strictEqual(got.source, 'buildinfo-vcs');
  });

  it('reads a main.revision stamp from buildinfo (stamped build)', (t) => {
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const bin = buildScry('stamped-scry', ['-buildvcs=false', '-ldflags', `-X main.revision=${STAMP}`]);
    const got = readBinaryRevision(bin);
    strictEqual(got.revision, STAMP);
    strictEqual(got.source, 'buildinfo-ldflags');
  });

  it('reads the gate-export shape via the version output (trimpath hides the stamp in buildinfo)', (t) => {
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const bin = buildScry('trimpath-scry', ['-trimpath', '-buildvcs=false', '-ldflags', `-X main.revision=${STAMP}`]);
    // Premise of this test: a trimpath build does not record the ldflags in
    // buildinfo, so the binary's own version output is the only evidence.
    const info = spawnSync('go', ['version', '-m', bin], { encoding: 'utf8' });
    strictEqual(/ldflags/.test(info.stdout), false, 'trimpath build must hide the stamp in buildinfo');
    const got = readBinaryRevision(bin);
    strictEqual(got.revision, STAMP);
    strictEqual(got.source, 'version-output');
  });

  it('reports no revision for a build without vcs info or a stamp', (t) => {
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const bin = buildScry('novcs-scry', ['-buildvcs=false']);
    const got = readBinaryRevision(bin);
    strictEqual(got.revision, null);
    strictEqual(got.source, null);
  });

  it('reports no revision for a non-Go binary', () => {
    const script = join(TMP_ROOT, 'not-go.sh');
    writeFileSync(script, '#!/bin/sh\nexit 1\n');
    chmodSync(script, 0o755);
    const got = readBinaryRevision(script);
    strictEqual(got.revision, null);
    strictEqual(got.source, null);
  });
});
