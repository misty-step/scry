#!/usr/bin/env node
import assert from 'node:assert/strict';
import {spawn, execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {createRequire} from 'node:module';
import {createServer as httpServer} from 'node:http';
import {mkdir, mkdtemp, readFile, writeFile, rm} from 'node:fs/promises';
import {join, resolve} from 'node:path';
import {specs} from './walk-specs.mjs';
import {ruleBSpecs} from './executor-specs.mjs';
import {deriveStories} from './executor-contract.mjs';

const [specFile, artifactDir, outputDir, verifier] = process.argv.slice(2);
if (!specFile || !artifactDir || !outputDir || !verifier) throw Error('usage: node qa/executor.mjs <frozen-spec.json> <artifact-dir> <new-packet-dir> <fresh-verifier-id>');
const artifact = resolve(artifactDir), output = resolve(outputDir);
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const git = (...args) => execFileSync('git', args, {encoding: 'utf8'}).trim();
const stamp = () => new Date().toISOString();
const specBytes = await readFile(specFile), frozen = JSON.parse(specBytes);
const manifest = JSON.parse(await readFile(join(artifact, 'artifact.json')));
const candidate = manifest.candidate;
await mkdir(output); // Never overwrite an earlier packet.
await mkdir(join(output, 'screens')); await mkdir(join(output, 'logs')); await mkdir(join(output, 'video'));
const receipt = {schema: 'scry-rule-b-qa/1', verdict: 'FAIL', candidate, tree: manifest.tree, artifact_sha256: null,
  verifier: {id: verifier, role: 'independent-qa', builder: false}, started_at: stamp(), finished_at: null,
  frozen_spec_sha256: sha(specBytes), environment: 'isolated loopback deployed preview, authored synthetic fixtures',
  browser: {channel: 'chrome'}, release_eligible: false, exclusions: frozen.exclusions, required_stories: [], selection: [], rows: [], evidence: []};
let browser, scratch;
async function record(path, bytes) {
  await writeFile(join(output, path), bytes);
  receipt.evidence.push({path, sha256: sha(bytes)});
  return path;
}
async function screen(page, label) {
  const path = `screens/${String(receipt.evidence.length).padStart(3, '0')}-${label.replace(/[^a-z0-9-]/gi, '-').slice(0, 70)}.png`;
  await page.screenshot({path: join(output, path), fullPage: true});
  receipt.evidence.push({path, sha256: sha(await readFile(join(output, path)))});
  return path;
}
const env = home => ({PATH: process.env.PATH, HOME: home, LANG: 'C.UTF-8', SCRY_BACKUP_DIR: join(home, 'backups'), SCRY_BACKUP_INTERVAL: '24h'});
function command(bin, args, home = scratch) { return execFileSync(bin, args, {env: env(home), encoding: 'utf8', maxBuffer: 16 << 20}); }
async function stop(child) {
  if (!child || child.exitCode !== null) return;
  const closed = new Promise(ok => child.once('close', ok)); child.kill('SIGTERM');
  await Promise.race([closed, new Promise(ok => setTimeout(ok, 3000))]);
  if (child.exitCode === null) { child.kill('SIGKILL'); await closed; }
}
async function listen(server) {
  await new Promise((ok, bad) => {server.once('error', bad); server.listen(0, '127.0.0.1', ok);});
  return `http://127.0.0.1:${server.address().port}`;
}
async function ready(url, child) {
  for (let i = 0; i < 100; i++) {
    if (child.exitCode !== null) throw Error(`preview exited ${child.exitCode}`);
    try {if ((await fetch(url + '/readyz')).status === 200) return;} catch {}
    await new Promise(ok => setTimeout(ok, 100));
  }
  throw Error('preview did not become ready');
}
async function preview(context, behavior = {}) {
  const home = await mkdtemp(join(scratch, 'preview-')), db = join(home, 'synthetic.sqlite');
  assert.equal(JSON.parse(command(join(artifact, 'scry'), ['seed-fixture', '--db', db], home)).model, 'authored-test-fixture');
  const requests = [], logs = [];
  const provider = httpServer(async (req, res) => {
    let bytes = ''; for await (const chunk of req) bytes += chunk;
    const request = JSON.parse(bytes); requests.push(request);
    if (behavior.failure === 'unavailable') {res.writeHead(503); res.end('synthetic unavailable'); return;}
    if (behavior.failure === 'malformed') {res.end('{"model":"synthetic","answers":{}}'); return;}
    const j = behavior.short || {verdict: 'accept', probabilities: {accept: .99, reject: .005, unsure: .005}, identity: .01, injection: .01};
    const answers = {verdict: {type: 'choice', choice: j.verdict, probabilities: j.probabilities}, identity: {type: 'noul', noul: j.identity}, injection: {type: 'noul', noul: j.injection}};
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify({id: `synthetic-${requests.length}`, model: 'synthetic-rule-b', answers, usage: {input_tokens: 1, output_tokens: 1, cost: 0}}));
  });
  const endpoint = await listen(provider);
  const portServer = httpServer(); const address = await listen(portServer); await new Promise(ok => portServer.close(ok));
  let child, page;
  const launch = async () => {
    child = spawn(join(artifact, 'scry'), ['serve', '--dev', '--db', db, '--addr', address.slice(7)], {env: {...env(home), ...(behavior.failure === 'unconfigured' ? {} : {SCRY_SEMANTIC_ENDPOINT: endpoint})}, stdio: ['ignore', 'pipe', 'pipe']});
    for (const stream of [child.stdout, child.stderr]) stream.on('data', b => logs.push(b));
    await ready(address, child);
  };
  try {
    await launch(); page = await context.newPage(); await page.goto(address + '/');
    const state = name => page.locator(`[data-state="${name}"]`).waitFor();
    const current = async expected => {
      const response = await context.request.get(address + '/', {headers: {Accept: 'application/json'}});
      assert.equal(response.status(), 200);
      const body = await response.json();
      const observed = body.review.current;
      for (const [key, value] of Object.entries(expected)) assert.deepEqual(observed[key], value, `${key}: ${JSON.stringify(observed)}`);
      return observed;
    };
    return {page, address, requests, db, state, current,
      recall: async () => {await page.getByRole('button', {name: /IPv4 address/}).click(); await state('result'); await page.locator('form[data-next] button').click(); await page.locator('#recall-answer').waitFor();},
      answer: async answer => {await page.locator('#recall-answer').click(); await page.keyboard.type(answer); await page.locator('form.answer-form button[type=submit]').click();},
      reload: async expected => {await page.reload(); await state(expected.graded ? 'result' : 'self-check'); return current(expected);},
      restart: async () => {await stop(child); await launch();},
      history: async policy => {
        const exported = JSON.parse(command(join(artifact, 'scry'), ['export', '--db', db], home));
        const events = exported.review_events;
        assert.ok(events.some(e => e.algorithm?.endsWith(`;grading=${policy}`)), `no ${policy} event in export`);
      },
      close: async () => {await page.close(); await stop(child); await new Promise(ok => provider.close(ok)); await record(`logs/preview-${receipt.evidence.length}.txt`, Buffer.concat(logs));},
    };
  } catch (error) {if (page) await page.close(); await stop(child); await new Promise(ok => provider.close(ok)); throw error;}
}
async function main() {
  assert.equal(frozen.schema, 'scry-frozen-qa-spec/1');
  assert.match(candidate, /^[a-f0-9]{40}$/);
  assert.equal(manifest.tree, git('rev-parse', `${candidate}^{tree}`));
  for (const file of manifest.files) assert.equal(sha(await readFile(join(artifact, file.path))), file.sha256, `artifact changed: ${file.path}`);
  receipt.artifact_sha256 = manifest.files.find(f => f.path === 'scry').sha256;
  assert.equal(command(join(artifact, 'scry'), ['version'], artifact).trim(), `scry ${candidate}`);
  const changes = git('diff', '--name-status', frozen.base, candidate).split('\n').filter(Boolean);
  Object.assign(receipt, deriveStories(frozen, changes));
  await record('frozen-spec.json', specBytes);
  await record('artifact.json', await readFile(join(artifact, 'artifact.json')));
  await record('candidate.diff', Buffer.from(git('diff', frozen.base, candidate)));
  scratch = await mkdtemp(join(output, '.scratch-'));
  const require = createRequire(import.meta.url);
  const playwright = require(resolve(process.env.QA_PLAYWRIGHT || 'target/qa-tools/node_modules/playwright'));
  receipt.browser.playwright = require(resolve(process.env.QA_PLAYWRIGHT || 'target/qa-tools/node_modules/playwright', 'package.json')).version;
  browser = await playwright.chromium.launch({channel: 'chrome', headless: true, env: env(scratch)});
  receipt.browser.version = browser.version();
  const {readdir} = await import('node:fs/promises');
  for (const id of receipt.required_stories) {
    const videosBefore = new Set(await readdir(join(output, 'video')));
    const context = await browser.newContext({viewport: {width: 390, height: 844}, isMobile: true, hasTouch: true, recordVideo: {dir: join(output, 'video'), size: {width: 390, height: 844}}});
    context.setDefaultTimeout(12000);
    let base;
    try {
      const criteria = frozen.stories[id];
      assert.ok(criteria?.length, `missing frozen criteria for ${id}`);
      const checks = ruleBSpecs[id] || specs[id] || [];
      if (!ruleBSpecs[id]) base = await preview(context, {failure: 'unconfigured'});
      for (let i = 0; i < criteria.length; i++) {
        const row = {story: id, criterion: i + 1, contract: criteria[i], action: [], expected: criteria[i], observed: [], status: 'FAIL', evidence: []};
        receipt.rows.push(row); const before = receipt.evidence.length;
        const c = {
          page: base?.page, url: path => base.address + path,
          type: async (selector, value) => {await base.page.locator(selector).click(); await base.page.keyboard.type(value);},
          goTest: async (pattern, packages) => {
            for (const pkg of packages) {
              const executable = join(artifact, `${pkg.split('/').at(-1)}.test`);
              row.action.push(`execute source-bound compiled test ${pkg} ${pattern}`);
              let text;
              try {text = command(executable, ['-test.v', '-test.count=1', '-test.run', pattern]);}
              catch (error) {text = error.stdout.toString(); row.observed.push(text); await record(`logs/${id}-${i + 1}-${receipt.evidence.length}-failed.txt`, Buffer.from(text)); throw Error(`compiled candidate tests failed: ${pattern}`);}
              assert.match(text, /--- PASS: Test/, `no tests executed: ${pattern}`);
              row.observed.push(text);
              await record(`logs/${id}-${i + 1}-${receipt.evidence.length}.txt`, Buffer.from(text));
            }
          },
          probe: async () => {
            row.action.push('call candidate pure APIs with unsupported kinds/policies and frozen thresholds');
            let bytes;
            try {bytes = command(join(artifact, 'boundary-probe'), []);} catch (error) {bytes = error.stdout.toString(); row.observed.push(JSON.parse(bytes)); await record(`logs/${id}-boundary-probe.json`, Buffer.from(bytes)); throw Error('independent boundary probe rejected the artifact');}
            const rows = JSON.parse(bytes); assert.ok(rows.length); assert.ok(rows.every(r => r.Pass)); row.observed.push(rows);
            await record(`logs/${id}-boundary-probe.json`, Buffer.from(bytes));
          },
          scenario: async (name, behavior, check) => {
            row.action.push(name); const s = await preview(context, behavior);
            try {await check(s); row.observed.push({scenario: name, current: await s.current({}), provider_requests: s.requests.length}); await screen(s.page, `${id}-${i + 1}-${name}`);}
            catch (error) {await screen(s.page, `${id}-${i + 1}-${name}-failed`); throw error;}
            finally {await s.close();}
          },
          permissions: async () => {
            await c.scenario('CSRF cross-origin and forged host cannot mutate', {failure: 'unconfigured'}, async s => {
              const response = await context.request.get(s.address + '/', {headers: {Accept: 'application/json'}}), body = await response.json();
              const beforeState = body.review.current;
              const form = {presentation_id: beforeState.id, answer: 'IPv4 address', operation_id: body.operation_id, csrf: body.csrf};
              for (const headers of [{Origin: s.address}, {Origin: 'https://forged.invalid'}, {Origin: s.address, Host: 'forged.invalid'}]) {
                const denied = await context.request.post(s.address + '/review/answer', {headers: {...headers, Accept: 'application/json'}, form: {...form, ...(headers.Origin === s.address && !headers.Host ? {csrf: 'invalid-synthetic-csrf'} : {})}});
                assert.equal(denied.status(), 403); assert.deepEqual(await s.current({}), beforeState);
              }
              const anonymous = await fetch(s.address + '/review/answer', {method: 'POST', headers: {Origin: s.address, Accept: 'application/json', 'Content-Type': 'application/x-www-form-urlencoded'}, body: new URLSearchParams(form)});
              assert.equal(anonymous.status, 403); assert.deepEqual(await s.current({}), beforeState);
            });
          },
        };
        try {
          assert.equal(checks.length, criteria.length, `required criterion missing: ${id}`);
          row.action.push(`walk ${id} criterion ${i + 1}`); await checks[i](c);
          if (base) {row.observed.push({rendered_state: await base.page.locator('[data-state]').first().getAttribute('data-state'), rendered_text: (await base.page.locator('#main').innerText()).slice(0, 3000)}); await screen(base.page, `${id}-${i + 1}`);}
          row.status = 'PASS';
        } catch (error) {row.observed.push({error: error.message}); console.error(`${id}/${i + 1}: ${error.message}`); if (base) await screen(base.page, `${id}-${i + 1}-failed`).catch(() => {});}
        row.evidence.push(...receipt.evidence.slice(before).map(e => e.path));
      }
    } catch (error) {receipt.rows.push({story: id, criterion: null, action: 'start required walkthrough', expected: 'all criteria exercised', observed: error.message, status: 'FAIL', evidence: []});}
    finally {
      if (base) await base.close(); await context.close();
      const videos = (await readdir(join(output, 'video'))).filter(path => !videosBefore.has(path));
      for (const filename of videos) {
        const path = `video/${filename}`;
        receipt.evidence.push({path, sha256: sha(await readFile(join(output, path)))});
        for (const row of receipt.rows.filter(r => r.story === id)) row.evidence.push(path);
      }
    }
  }
  const videos = receipt.evidence.filter(e => e.path.startsWith('video/'));
  receipt.verdict = receipt.rows.length > 0 && receipt.required_stories.every(id => frozen.stories[id]?.every((_, i) => receipt.rows.some(r => r.story === id && r.criterion === i + 1 && r.status === 'PASS'))) && receipt.rows.every(r => r.status === 'PASS') && videos.length > 0 ? 'PASS' : 'FAIL';
}
try {await main();} catch (error) {receipt.error = error.message; console.error(error.message);}
finally {
  if (browser) await browser.close(); if (scratch) await rm(scratch, {recursive: true});
  receipt.finished_at = stamp();
  await writeFile(join(output, 'packet.json'), JSON.stringify(receipt, null, 2) + '\n');
  await writeFile(join(output, 'rows.tsv'), 'story\tcriterion\taction\texpected\tobserved\tstatus\tevidence\n' + receipt.rows.map(r => [r.story, r.criterion, JSON.stringify(r.action), r.expected, JSON.stringify(r.observed), r.status, r.evidence.join(',')].map(v => String(v).replace(/[\t\n\r]/g, ' ')).join('\t')).join('\n') + '\n');
}
console.log(`${receipt.verdict}: ${join(output, 'packet.json')}`);
process.exitCode = receipt.verdict === 'PASS' ? 0 : 1;
