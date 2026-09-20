import { describe, it, after } from 'node:test';
import { strictEqual } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync, mkdirSync, chmodSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { readBinaryProvenance, sourceStateFromProvenance } from '../lib/provenance.mjs';

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

function gitAvailable() {
  return spawnSync('git', ['--version'], { encoding: 'utf8' }).status === 0;
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

// A self-contained git repo with a dependency-free Go module, so clean and
// modified builds are deterministic regardless of the host checkout's state.
function miniRepo(name) {
  const dir = join(TMP_ROOT, name);
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, 'go.mod'), 'module crittest\n\ngo 1.27\n');
  writeFileSync(join(dir, 'main.go'), 'package main\n\nfunc main() {}\n');
  const git = (...args) => spawnSync('git',
    ['-c', 'user.email=crit@test', '-c', 'user.name=crit', ...args],
    { cwd: dir, encoding: 'utf8' });
  const init = git('init', '-q');
  if (init.status !== 0) throw new Error('git init failed: ' + (init.stderr || init.error?.message));
  for (const step of [['add', '-A'], ['commit', '-qm', 'init']]) {
    const r = git(...step);
    if (r.status !== 0) throw new Error('git ' + step[0] + ' failed: ' + r.stderr);
  }
  return { dir, git };
}

function buildMini(dir, name) {
  const out = join(TMP_ROOT, name);
  const r = spawnSync('go', ['build', '-o', out, '.'], { cwd: dir, encoding: 'utf8', timeout: 120000 });
  if (r.status !== 0) throw new Error('mini build failed: ' + (r.stderr || r.error?.message || 'unknown'));
  return out;
}

describe('binary provenance (--binary must not borrow the checkout\u2019s revision or state)', () => {
  it('reads vcs.revision from a plain build (and it equals the checkout)', (t) => {
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const head = headRevision();
    if (!head) return t.skip('not a git checkout; a plain build carries no vcs stamp here');
    const bin = buildScry('plain-scry');
    const got = readBinaryProvenance(bin);
    strictEqual(got.revision, head);
    strictEqual(got.source, 'buildinfo-vcs');
    strictEqual(typeof got.modified, 'boolean');
  });

  it('reads vcs.modified=false from a clean build (deterministic mini repo)', (t) => {
    if (!goAvailable() || !gitAvailable()) return t.skip('go or git unavailable');
    const { dir } = miniRepo('mini-clean');
    const head = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: dir, encoding: 'utf8' }).stdout.trim();
    const bin = buildMini(dir, 'mini-clean-bin');
    const got = readBinaryProvenance(bin);
    strictEqual(got.revision, head);
    strictEqual(got.source, 'buildinfo-vcs');
    strictEqual(got.modified, false);
  });

  it('reads vcs.modified=true from a modified-tree build (deterministic mini repo)', (t) => {
    if (!goAvailable() || !gitAvailable()) return t.skip('go or git unavailable');
    const { dir, git } = miniRepo('mini-dirty');
    const head = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: dir, encoding: 'utf8' }).stdout.trim();
    writeFileSync(join(dir, 'main.go'), 'package main\n\nfunc main() {}\n// dirty probe\n');
    const bin = buildMini(dir, 'mini-dirty-bin');
    const got = readBinaryProvenance(bin);
    strictEqual(got.revision, head);
    strictEqual(got.source, 'buildinfo-vcs');
    strictEqual(got.modified, true);
    git('checkout', '--', 'main.go');
  });

  it('reads a main.revision stamp from buildinfo (stamped build)', (t) => {
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const bin = buildScry('stamped-scry', ['-buildvcs=false', '-ldflags', `-X main.revision=${STAMP}`]);
    const got = readBinaryProvenance(bin);
    strictEqual(got.revision, STAMP);
    strictEqual(got.source, 'buildinfo-ldflags');
    strictEqual(got.modified, null, 'a stamp carries no tree-state evidence');
  });

  it('reads the gate-export shape via the version output (trimpath hides the stamp in buildinfo)', (t) => {
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const bin = buildScry('trimpath-scry', ['-trimpath', '-buildvcs=false', '-ldflags', `-X main.revision=${STAMP}`]);
    // Premise of this test: a trimpath build does not record the ldflags in
    // buildinfo, so the binary's own version output is the only evidence.
    const info = spawnSync('go', ['version', '-m', bin], { encoding: 'utf8' });
    strictEqual(/ldflags/.test(info.stdout), false, 'trimpath build must hide the stamp in buildinfo');
    const got = readBinaryProvenance(bin);
    strictEqual(got.revision, STAMP);
    strictEqual(got.source, 'version-output');
    strictEqual(got.modified, null, 'the version output carries no tree-state evidence');
  });

  it('reports no revision or state for a build without vcs info or a stamp', (t) => {
    if (!goAvailable()) return t.skip('go toolchain unavailable');
    const bin = buildScry('novcs-scry', ['-buildvcs=false']);
    const got = readBinaryProvenance(bin);
    strictEqual(got.revision, null);
    strictEqual(got.source, null);
    strictEqual(got.modified, null);
  });

  it('reports no revision for a non-Go binary', () => {
    const script = join(TMP_ROOT, 'not-go.sh');
    writeFileSync(script, '#!/bin/sh\nexit 1\n');
    chmodSync(script, 0o755);
    const got = readBinaryProvenance(script);
    strictEqual(got.revision, null);
    strictEqual(got.source, null);
    strictEqual(got.modified, null);
  });
});

describe('binary source state (clean/dirty only from the binary\u2019s own evidence)', () => {
  it('maps vcs.modified to clean/dirty; stamp and absent evidence are unknown', () => {
    strictEqual(sourceStateFromProvenance({ source: 'buildinfo-vcs', modified: true }).state, 'dirty');
    strictEqual(sourceStateFromProvenance({ source: 'buildinfo-vcs', modified: true }).source, 'buildinfo-vcs');
    strictEqual(sourceStateFromProvenance({ source: 'buildinfo-vcs', modified: false }).state, 'clean');
    strictEqual(sourceStateFromProvenance({ source: 'buildinfo-vcs', modified: false }).source, 'buildinfo-vcs');
    // vcs.revision without vcs.modified is undeterminable state, never a guess.
    strictEqual(sourceStateFromProvenance({ source: 'buildinfo-vcs', modified: null }).state, 'unknown');
    strictEqual(sourceStateFromProvenance({ source: 'buildinfo-vcs', modified: null }).source, null);
    // Stamp sources carry no tree state: unknown, not the checkout's state.
    strictEqual(sourceStateFromProvenance({ source: 'buildinfo-ldflags', modified: null }).state, 'unknown');
    strictEqual(sourceStateFromProvenance({ source: 'version-output', modified: null }).state, 'unknown');
    strictEqual(sourceStateFromProvenance({ source: null, modified: null }).state, 'unknown');
    strictEqual(sourceStateFromProvenance({ source: null, modified: null }).source, null);
  });
});
