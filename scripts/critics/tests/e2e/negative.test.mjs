import { describe, it } from 'node:test';
import { strictEqual, ok } from 'node:assert';
import { spawnSync } from 'node:child_process';
import { existsSync, readFileSync, mkdirSync, rmSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { createServer } from 'node:http';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const FIXTURES_DIR = join(__dirname, '..', 'fixtures');
const CRITICS_DIR = join(__dirname, '..', '..');
const REPO_ROOT = join(__dirname, '..', '..', '..', '..', '..');

describe('negative e2e failure scenarios', () => {
  it('production target https://scry.study -> exit 2, no browser launched', async () => {
    const outDir = join(REPO_ROOT, 'target/critics/e2e-negative-prod');
    mkdirSync(outDir, { recursive: true });
    
    const result = spawnSync('node', [
      join(CRITICS_DIR, 'run.mjs'),
      'human',
      '--goal', 'practice-review',
      '--candidate', 'https://scry.study',
      '--out', outDir
    ], { encoding: 'utf8', timeout: 30000 });
    
    strictEqual(result.status, 2, 'expected exit code 2 for production origin');
    ok(result.stderr.includes('Blocked') || result.stderr.includes('production'), 'should mention blocked');
    
    rmSync(outDir, { recursive: true, force: true });
  });

  it('blocked env (closed port) -> exit 2 or 3 (runner cannot confirm), receipt written', async () => {
    const outDir = join(REPO_ROOT, 'target/critics/e2e-negative-blocked');
    mkdirSync(outDir, { recursive: true });
    
    const result = spawnSync('node', [
      join(CRITICS_DIR, 'run.mjs'),
      'human',
      '--goal', 'practice-review',
      '--candidate', 'http://127.0.0.1:1/',
      '--out', outDir
    ], { encoding: 'utf8', timeout: 30000 });
    
    ok(result.status === 2 || result.status === 3, 'expected exit code 2 or 3 for blocked env, got ' + result.status);
    ok(existsSync(join(outDir, 'receipt.json')), 'receipt should be written');
    
    const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
    ok(receipt.run.duration_s !== undefined, 'duration should be recorded');
    
    rmSync(outDir, { recursive: true, force: true });
  });

  it('no-op answer fixture -> not pass (exit 1 or 2)', async () => {
    try {
      const outDir = join(REPO_ROOT, 'target/critics/e2e-negative-noop');
      mkdirSync(outDir, { recursive: true });
      
      const fixturePath = join(FIXTURES_DIR, 'next-no-op.html');
      const server = createServer((req, res) => {
        res.setHeader('Content-Type', 'text/html');
        res.end(readFileSync(fixturePath, 'utf8'));
      });
      
      await new Promise((resolve, reject) => {
        server.on('error', reject);
        server.listen(0, '127.0.0.1', resolve);
      });
      
      const port = server.address().port;
      const result = spawnSync('node', [
        join(CRITICS_DIR, 'run.mjs'),
        'human',
        '--goal', 'practice-review',
        '--candidate', 'http://127.0.0.1:' + port + '/',
        '--out', outDir
      ], { encoding: 'utf8', timeout: 30000 });
      
      server.close();
      await new Promise(resolve => server.on('close', resolve));
      
      ok(result.status !== 0, 'expected non-zero exit, got ' + result.status);
      
      if (existsSync(join(outDir, 'receipt.json'))) {
        const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
        ok(!receipt.checks.some(c => c.status === 'pass'), 'no checks should pass');
      }
      
      rmSync(outDir, { recursive: true, force: true });
    } catch (err) {
      if (err.message.includes('ENOENT') || err.message.includes('Playwright')) {
        console.log('skipping (playwright unavailable)');
        return;
      }
      throw err;
    }
  });

  it('missing feedback/Next fixture -> finding or unverified (not pass)', async () => {
    try {
      const outDir = join(REPO_ROOT, 'target/critics/e2e-negative-missing');
      mkdirSync(outDir, { recursive: true });
      
      const fixtureHtml = '<!DOCTYPE html><html><body><div class="review-stage"><h1>Test?</h1><button class="choice" name="answer" value="A">A</button></div></body></html>';
      const server = createServer((req, res) => {
        res.setHeader('Content-Type', 'text/html');
        res.end(fixtureHtml);
      });
      
      await new Promise((resolve, reject) => {
        server.on('error', reject);
        server.listen(0, '127.0.0.1', resolve);
      });
      
      const port = server.address().port;
      const result = spawnSync('node', [
        join(CRITICS_DIR, 'run.mjs'),
        'human',
        '--goal', 'practice-review',
        '--candidate', 'http://127.0.0.1:' + port + '/',
        '--out', outDir
      ], { encoding: 'utf8', timeout: 30000 });
      
      server.close();
      await new Promise(resolve => server.on('close', resolve));
      
      ok(result.status !== 0, 'expected non-zero exit, got ' + result.status);
      
      if (existsSync(join(outDir, 'receipt.json'))) {
        const receipt = JSON.parse(readFileSync(join(outDir, 'receipt.json'), 'utf8'));
        ok(receipt.findings.length > 0 || receipt.checks.some(c => c.status !== 'pass'), 'should have findings or non-pass checks');
      }
      
      rmSync(outDir, { recursive: true, force: true });
    } catch (err) {
      if (err.message.includes('ENOENT') || err.message.includes('Playwright')) {
        console.log('skipping (playwright unavailable)');
        return;
      }
      throw err;
    }
  });
});
