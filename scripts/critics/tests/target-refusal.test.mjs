import { test } from 'node:test';
import { strictEqual, match } from 'node:assert';
import { mkdtempSync, writeFileSync, existsSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

test('malformed synthetic targets are refused before browser resolution or receipt', () => {
  const dir = mkdtempSync(join(tmpdir(), 'scry-target-refusal-'));
  try {
    const marker = join(dir, 'browser-loaded');
    const fake = join(dir, 'playwright.cjs');
    writeFileSync(fake, `require('node:fs').writeFileSync(${JSON.stringify(marker)}, 'loaded'); module.exports = {};`);
    const runner = fileURLToPath(new URL('../run.mjs', import.meta.url));
    for (const suffix of ['..', '%2e%2e', '.%2e']) {
      const target = `http://example.invalid${suffix}`;
      const out = join(dir, `out-${suffix}`);
      const result = spawnSync(process.execPath, [runner, 'human', '--goal', 'practice-review',
        '--candidate', target, '--allow-origin', new URL(target).origin, '--out', out], {
        encoding: 'utf8', env: { PATH: process.env.PATH, HOME: dir, SCRY_CRITICS_PLAYWRIGHT_PATH: fake }
      });
      strictEqual(result.status, 2, result.stderr);
      match(result.stderr, /hostname contains an empty label/);
      strictEqual(existsSync(marker), false, 'browser resolver must not load Playwright');
      strictEqual(existsSync(join(out, 'receipt.json')), false);
      strictEqual(existsSync(join(out, 'shots')), false);
    }
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
