#!/usr/bin/env node
import {spawn, execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {createServer} from 'node:net';
import {existsSync, readFileSync} from 'node:fs';
import {cp, mkdir, mkdtemp, readFile, rm, writeFile} from 'node:fs/promises';
import {homedir} from 'node:os';
import {join, resolve} from 'node:path';
import {specs} from './walk-specs.mjs';

const repo = resolve(import.meta.dirname, '..');
process.chdir(repo);
const source = readFileSync('USER_STORIES.md', 'utf8');
const live = [...source.matchAll(/^## (US-\d{3})\b[^\n]*$/gm)]
  .filter(m => !/retired|superseded by/i.test(source.slice(m.index, source.indexOf('\n## ', m.index + 4) < 0 ? undefined : source.indexOf('\n## ', m.index + 4))))
  .map(m => m[1]);
function args() {
  if (process.argv.length === 3 && process.argv[2] === '--all') return live;
  if (process.argv.length === 4 && process.argv[2] === '--stories') {
    const ids = [...new Set(process.argv[3].trim().split(/\s+/).filter(Boolean))];
    for (const id of ids) if (!live.includes(id)) throw Error(`not a live story: ${id}`);
    return ids;
  }
  throw Error('usage: qa/walk --all | --stories "US-002 US-003"');
}
const git = (...argv) => execFileSync('git', argv, {cwd: repo, encoding: 'utf8'}).trim();
const stamp = () => new Date().toISOString();
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const output = join(repo, 'target/walk');
const marker = join(output, '.scry-walk-owned');
const scratchParent = join(process.env.XDG_CACHE_HOME || join(homedir(), '.cache'), 'tmp');
const safeEnv = (home, more = {}) => ({PATH: process.env.PATH || '/usr/bin:/bin', HOME: home, LANG: 'C.UTF-8', ...more});
async function command(bin, argv, opts = {}) {
  const child = spawn(bin, argv, {cwd: repo, env: opts.env || safeEnv(opts.home || homedir()), stdio: ['ignore', 'pipe', 'pipe']});
  const chunks = [];
  for (const stream of [child.stdout, child.stderr]) stream.on('data', b => chunks.push(b));
  const code = await new Promise((ok, bad) => { child.on('error', bad); child.on('close', ok); });
  const result = Buffer.concat(chunks).toString();
  if (code !== 0) throw Error(`${bin} ${argv.join(' ')} exited ${code}: ${result.slice(-1200)}`);
  return result;
}
async function port() {
  const server = createServer();
  await new Promise((ok, bad) => { server.once('error', bad); server.listen(0, '127.0.0.1', ok); });
  const number = server.address().port;
  await new Promise(ok => server.close(ok));
  return number;
}
async function stop(child) {
  if (!child || child.exitCode !== null) return;
  child.kill('SIGTERM');
  await Promise.race([new Promise(ok => child.once('close', ok)), new Promise(ok => setTimeout(ok, 5000))]);
  if (child.exitCode === null) child.kill('SIGKILL');
}
async function ready(url, child) {
  for (let i = 0; i < 120; i++) {
    if (child.exitCode !== null) throw Error(`serve exited ${child.exitCode}`);
    try { if ((await fetch(url + '/readyz')).status === 200) return; } catch {}
    await new Promise(ok => setTimeout(ok, 250));
  }
  throw Error('loopback serve did not become ready');
}
async function main() {
  const selected = args();
  const sections = [...source.matchAll(/^## (US-\d{3})\b([^\n]*)/gm)];
  for (const id of live) {
    const start = sections.find(m => m[1] === id);
    const end = sections.find(m => m.index > start.index);
    const count = [...source.slice(start.index, end?.index).matchAll(/^\d+\. (?:WHEN|WHILE|WHERE|IF|THE\b|EACH|Export)/gm)].length;
    if (specs[id]?.length && count !== specs[id].length) throw Error(`${id}: ${specs[id].length} walk criteria for ${count} story criteria`);
  }
  if (existsSync(output) && !existsSync(marker)) throw Error(`refusing non-owned output directory ${output}`);
  if (existsSync(marker)) await rm(output, {recursive: true});
  await mkdir(join(output, 'screens'), {recursive: true});
  await mkdir(join(output, 'logs'), {recursive: true});
  await writeFile(marker, 'Generated only by qa/walk; never production evidence.\n');
  await mkdir(scratchParent, {recursive: true});
  const scratch = await mkdtemp(join(scratchParent, 'scry-walk.'));
  const receipt = {schema: 'foundation-walk-receipt/1', check: 'scry-story-walk', run: process.env.GITHUB_RUN_ID || `local-${Date.now()}`, head: git('rev-parse', 'HEAD'), tree: git('rev-parse', 'HEAD^{tree}'), base: process.env.WALK_BASE || null, started_at: stamp(), finished_at: '', exit: 1, stories: [], artifacts: []};
  let browser;
  try {
    const install = join(homedir(), '.local/share/scry-walk/node_modules/playwright');
    if (!existsSync(join(install, 'package.json'))) throw Error(`Playwright 1.63.0 missing at ${install}; run bash .exe/setup.sh`);
    const {createRequire} = await import('node:module');
    const require = createRequire(import.meta.url);
    const playwright = require(install);
    if (require(join(install, 'package.json')).version !== '1.63.0') throw Error('Playwright version does not match 1.63.0');
    browser = await playwright.chromium.launch({headless: true, env: safeEnv(homedir())});
    const binary = join(scratch, 'scry');
    await command('go', ['build', '-mod=readonly', '-o', binary, './cmd/scry']);
    for (const id of selected) {
      if (!specs[id]?.length) {
        receipt.stories.push({id, status: 'unwalked', criteria: []});
        console.error(`${id}: no story walk is defined`);
        continue;
      }
      const dir = join(scratch, id);
      await mkdir(dir);
      const db = join(dir, 'synthetic.sqlite');
      const env = safeEnv(dir);
      const seeded = JSON.parse(await command(binary, ['seed-fixture', '--db', db], {env}));
      if (seeded.model !== 'authored-test-fixture' || !seeded.source) throw Error(`${id}: seed-fixture returned unexpected provenance`);
      const address = `http://127.0.0.1:${await port()}`;
      const serve = spawn(binary, ['serve', '--dev', '--db', db, '--addr', address.slice('http://'.length)], {
        cwd: repo, env: safeEnv(dir, {SCRY_BACKUP_DIR: join(dir, 'backups')}), stdio: ['ignore', 'pipe', 'pipe'],
      });
      const serverLog = [];
      for (const stream of [serve.stdout, serve.stderr]) stream.on('data', b => serverLog.push(b));
      let context;
      const result = {id, status: 'pass', criteria: []};
      receipt.stories.push(result);
      try {
        await ready(address, serve);
        context = await browser.newContext({viewport: {width: 390, height: 844}, isMobile: true, hasTouch: true});
        const page = await context.newPage();
        await page.goto(address + '/');
        const c = {
          page, url: path => address + path,
          type: async (selector, value) => { await page.locator(selector).click(); await page.keyboard.type(value); },
          goTest: async (pattern, packages) => {
            const argv = ['test', '-p', '1', '-count=1', '-mod=readonly', ...packages, '-run', pattern, '-v'];
            const text = await command('go', argv);
            if (!/--- PASS: Test/.test(text)) throw Error(`no test ran for ${pattern}`);
            const path = `logs/${id}-${result.criteria.length + 1}-${receipt.artifacts.length}.txt`;
            await record(path, Buffer.from(`$ go ${argv.join(' ')}\n${text}`), receipt);
            return path;
          },
        };
        for (let i = 0; i < specs[id].length; i++) {
          const criterion = {n: i + 1, status: 'pass', evidence: []};
          result.criteria.push(criterion);
          const before = receipt.artifacts.length;
          try {
            await specs[id][i](c);
            criterion.evidence.push(...receipt.artifacts.slice(before).map(a => a.path));
            const path = `screens/${id}-${i + 1}.png`;
            await page.screenshot({path: join(output, path), fullPage: true});
            await register(path, receipt);
            criterion.evidence.push(path);
          } catch (error) {
            criterion.status = 'fail'; result.status = 'fail';
            console.error(`${id} criterion ${i + 1}: ${error.message}`);
            try { const path = `screens/${id}-${i + 1}-failure.png`; await page.screenshot({path: join(output, path), fullPage: true}); await register(path, receipt); criterion.evidence.push(path); } catch {}
          }
        }
      } catch (error) {
        result.status = 'fail';
        result.criteria.push({n: result.criteria.length + 1, status: 'fail', evidence: []});
        console.error(`${id}: ${error.message}`);
      } finally {
        if (context) await context.close();
        await stop(serve);
        const serverOutput = Buffer.concat(serverLog).toString();
        await record(`logs/${id}-serve.txt`, Buffer.from(serverOutput), receipt);
      }
    }
    receipt.exit = receipt.stories.every(s => s.status === 'pass') ? 0 : 1;
  } catch (error) {
    console.error(`walk infrastructure: ${error.message}`);
    for (const id of selected) if (!receipt.stories.some(s => s.id === id)) receipt.stories.push({id, status: 'unwalked', criteria: []});
  } finally {
    if (browser) await browser.close();
    await rm(scratch, {recursive: true});
    receipt.finished_at = stamp();
    await writeFile(join(output, 'walk-receipt.json'), JSON.stringify(receipt, null, 2) + '\n');
    if (process.env.WS_EVIDENCE) await cp(output, join(process.env.WS_EVIDENCE, 'target/walk'), {recursive: true, force: true});
    console.log(`Scry story walk: ${receipt.stories.filter(s => s.status === 'pass').length}/${selected.length} pass; receipt ${join(output, 'walk-receipt.json')}`);
  }
  if (receipt.exit) process.exitCode = 1;
}
async function register(path, receipt) {
  receipt.artifacts.push({path, sha256: sha(await readFile(join(output, path)))});
}
async function record(path, bytes, receipt) {
  await writeFile(join(output, path), bytes);
  await register(path, receipt);
}
main().catch(error => {console.error(error); process.exitCode = 2;});
