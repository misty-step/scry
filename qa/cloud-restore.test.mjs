// Synthetic current-Rust snapshot -> private bridge -> exact candidate -> R2.
import assert from 'node:assert/strict';
import test from 'node:test';
import {createHash} from 'node:crypto';
import {createRequire} from 'node:module';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {createRuntime, internalKey} from './runtime-helper.mjs';
import {reconcile} from '../recovery/cloud-restore.mjs';
const require = createRequire(import.meta.url);
let runtime;
try {runtime = require('miniflare');} catch {runtime = createRequire(resolve(process.env.SCRY_WRANGLER_PACKAGE))('miniflare');}
const artifact = resolve(process.env.SCRY_WORKER_ARTIFACT || 'build');
const bridgePath = resolve('recovery/cloud-restore.mjs');
const token = 'synthetic-restore-key-at-least-thirty-two-chars';
const transfer = 'synthetic-transfer-key-at-least-thirty-two-chars';
const space = 'recovery-synthetic-only';
const sourceKey = 'snapshots/synthetic-source/1793130000000-daily-' + 'a'.repeat(48) + '.json';
const options = input => runtime.convertV4MiniflareOptions ? runtime.convertV4MiniflareOptions(input) : input;
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const cache = new Map();
async function sourceArchive(mode = 'plain') {
  if (cache.has(mode)) return structuredClone(cache.get(mode));
  const name = 'synthetic-recovery-source', origin = 'http://localhost:8787', sourceSpace = 'synthetic-source';
  const bindings = {SCRY_ENV: 'development', SPACE_NAME: sourceSpace, CANONICAL_ORIGIN: origin, INTERNAL_KEY: internalKey};
  const local = await createRuntime({name, bindings, inspect: true});
  const request = (path, init = {}) => local.worker.fetch(origin + path, {redirect: 'manual', ...init});
  const post = (path, fields) => request(path, {method: 'POST', headers: {'X-Scry-Test-Origin': origin,
    'Content-Type': 'application/x-www-form-urlencoded'}, body: new URLSearchParams(fields)});
  const snapshot = async () => {
    const ns = await local.mf.getDurableObjectNamespace('LEARNING_SPACE');
    const response = await ns.get(ns.idFromName(sourceSpace)).fetch('https://internal.invalid/__internal/daily',
      {method: 'POST', headers: {'x-scry-internal-key': internalKey, 'x-scry-owner': 'private-owner'}});
    assert.equal(response.status, 200);
    const bucket = await local.mf.getR2Bucket('SCRY_BACKUPS');
    const list = await bucket.list();
    return (await bucket.get(list.objects.sort((a, b) => b.uploaded - a.uploaded)[0].key)).json();
  };
  try {
    const html = await (await request('/notebook')).text();
    const form = /<form[^>]*action="\/__fixture"[^>]*>([\s\S]*?)<\/form>/.exec(html)?.[1];
    assert.ok(form);
    const fields = Object.fromEntries([...form.matchAll(/name="([^"]+)" value="([^"]*)"/g)].map(m => [m[1], m[2]]));
    assert.equal((await post('/__fixture', fields)).status, 303);
    let archive = await snapshot();
    if (mode !== 'plain') {
      const o = archive.app.occurrence;
      assert.equal((await post(`/questions/${o.question_id}/archive`, {csrf: archive.app.csrf, operation_id: 'synthetic-archive-choice'})).status, 303);
      await request('/notebook'); archive = await snapshot();
      assert.equal(archive.app.occurrence.presentation.kind, 'recall');
      assert.equal((await post('/review/answer', {csrf: archive.app.csrf, operation_id: 'synthetic-saved-answer',
        occurrence_id: archive.app.occurrence.id, answer: 'SYNTHETIC_PRIVATE remembered response'})).status, 303);
      archive = await snapshot();
      const assessment = Object.values(archive.app.assessments)[0];
      assert.equal(assessment.status, 'queued');
      if (mode === 'sent') {
        archive.format = 'scry-rust-durable-sqlite-attributed-spend-v1';
        assessment.status = 'sent'; assessment.lease = 'synthetic-paid-lease'; assessment.started_ms = Date.now();
        archive.app.spend.push({id: assessment.lease, work_id: assessment.id, created_ms: assessment.started_ms,
          reserved_micros: 500000, cost_micros: null, status: 'unknown', model: 'synthetic-grader', purpose: 'grade.jev',
          response_hash: null, response_model: null, raw_response: null, response_bytes: null, response_outcome: null});
      }
    }
    cache.set(mode, archive); return structuredClone(archive);
  } finally {await local.mf.dispose();}
}
async function fixture(fn, {mode = 'plain', change, env = {}, target = {}, legacy = false} = {}) {
  const archive = await sourceArchive(mode); change?.(archive);
  const bytes = Buffer.from(JSON.stringify(archive));
  const bounds = {TRANSFER_ISSUED_MS: String(Date.now() - 1000), TRANSFER_UNTIL_MS: String(Date.now() + 300000)};
  let bindings = {TRANSFER_KEY: transfer, INTERNAL_KEY: internalKey, RESTORE_TOKEN: token, SPACE_NAME: space,
    SOURCE_KEY: sourceKey, SOURCE_ETAG: createHash('md5').update(bytes).digest('hex'), SOURCE_BYTES: String(bytes.length), ...bounds, ...env};
  let egress = 0;
  const outboundService = async () => {egress++; throw Error('Synthetic egress forbidden');};
  const script = readFileSync(legacy ? 'revival/restore.mjs' : bridgePath, 'utf8');
  const make = () => options({unsafeInspectDurableObjects: true, workers: [
    {name: 'bridge', compatibilityDate: '2026-09-10', cf: false, modulesRoot: resolve('recovery'),
      modules: [{type: 'ESModule', path: bridgePath, contents: script}], bindings, outboundService,
      durableObjects: {...(legacy ? {} : {GATE: {className: 'RecoveryBridge', useSQLite: true}}),
        TARGET: {className: 'LearningSpace', scriptName: 'target', useSQLite: true}},
      r2Buckets: {SOURCE: 'source', BACKUPS: 'backups'}},
    {name: 'target', compatibilityDate: '2026-09-10', cf: false, modulesRoot: artifact, outboundService,
      modules: [{type: 'ESModule', path: resolve(artifact, 'recovery-adapter.mjs'), contents:
        readFileSync('recovery/target-adapter.mjs', 'utf8')},
        {type: 'ESModule', path: resolve(artifact, 'index.js'), contents: readFileSync(resolve(artifact, 'index.js'), 'utf8')},
        {type: 'CompiledWasm', path: resolve(artifact, 'index_bg.wasm'), contents: readFileSync(resolve(artifact, 'index_bg.wasm'))}],
      bindings: {SCRY_ENV: 'restore', SPACE_NAME: space, INTERNAL_KEY: internalKey, RESTORE_TOKEN: token, ...bounds, ...target},
      durableObjects: {LEARNING_SPACE: {className: 'LearningSpace', useSQLite: true}},
      r2Buckets: {SCRY_ASSETS: 'assets', SCRY_BACKUPS: 'backups'}},
  ]});
  const mf = new runtime.Miniflare(make());
  try {
    const source = await mf.getR2Bucket('SOURCE', 'bridge');
    const original = await source.put(sourceKey, bytes);
    assert.equal(original.etag, bindings.SOURCE_ETAG.replaceAll('"', ''));
    const request = async (init = {}) => {
      const response = await (await mf.getWorker('bridge')).fetch('https://private-preview.invalid/restore',
        {method: 'POST', ...init, headers: {Authorization: 'Bearer ' + transfer, ...init.headers}});
      const text = await response.text();
      assert.doesNotMatch(text, /SYNTHETIC_PRIVATE|Cache freshness|remembered response|synthetic-paid-lease/);
      for (const secret of [transfer, token, internalKey]) assert.ok(!text.includes(secret));
      return {status: response.status, body: JSON.parse(text)};
    };
    const rows = async () => {
      const sql = await mf.unsafeGetDurableObjectStorage('target', 'LearningSpace', {name: space});
      const tables = await sql.exec("SELECT name FROM sqlite_master WHERE name='records'");
      return tables.length ? sql.exec('SELECT key,value FROM records ORDER BY key') : [];
    };
    await fn({request, mf, rows, bytes, archive, source, rebindTarget: async next => {Object.assign(target, next); await mf.setOptions(make());}, rebind: async next => {bindings = {...bindings, ...next}; await mf.setOptions(make());}});
    assert.deepEqual(Buffer.from(await (await (await mf.getR2Bucket('SOURCE', 'bridge')).get(sourceKey)).arrayBuffer()), bytes);
    assert.equal(egress, 0);
  } finally {await mf.dispose();}
}

test('historical converted-Go bridge rejects the current-Rust snapshot (regression control)', () => fixture(async ({request, rows}) => {
  assert.deepEqual(await request(), {status: 503, body: {error: 'INVALID_PIN'}});
  assert.deepEqual(await rows(), []);
}, {legacy: true}));
for (const mode of ['plain', 'queued', 'sent']) test(`current ${mode} snapshot restores once and compares the full native archive`, () => fixture(async ({request, mf, bytes, archive}) => {
  const response = await request(); assert.equal(response.status, 200, JSON.stringify(response.body));
  assert.equal(response.body.target_page_size, 4096); assert.ok(response.body.target_database_bytes < 32 * 1024 * 1024);
  assert.equal(response.body.source_sha256, hash(bytes)); assert.equal(response.body.canonical_application_equal, true);
  const bucket = await mf.getR2Bucket('BACKUPS', 'bridge'); const list = await bucket.list(); assert.equal(list.objects.length, 1);
  const actual = await (await bucket.get(list.objects[0].key)).json();
  assert.deepEqual(actual.app.spend, archive.app.spend);
  assert.deepEqual(actual.app.events, archive.app.events);
  assert.deepEqual(actual.app.operations, archive.app.operations);
  if (mode !== 'plain') {
    assert.equal(Object.values(actual.app.assessments)[0].status, mode === 'sent' ? 'unknown' : 'paused');
    assert.equal(actual.app.occurrence.phase, 'self-check');
    assert.equal(actual.app.occurrence.answer, archive.app.occurrence.answer);
  }
  assert.deepEqual(await request(), {status: 409, body: {error: 'ATTEMPT_ALREADY_SPENT'}});
  assert.equal((await bucket.list()).objects.length, 1);
}, {mode}));
test('concurrent authenticated requests admit exactly one restore and backup', () => fixture(async ({request, mf}) => {
  const results = await Promise.all([request(), request(), request()]);
  assert.deepEqual(results.map(r => r.status).sort(), [200, 409, 409]);
  assert.equal((await (await mf.getR2Bucket('BACKUPS', 'bridge')).list()).objects.length, 1);
}));
test('authorization, body, expiry and excessive lifetime refuse before learner writes', () => fixture(async ({request, rows, rebind}) => {
  assert.equal((await request({headers: {Authorization: 'Bearer invalid'}})).status, 403);
  assert.equal((await request({method: 'GET'})).status, 405);
  assert.equal((await request({body: 'SYNTHETIC_PRIVATE'})).status, 400);
  await rebind({TRANSFER_UNTIL_MS: String(Date.now() - 1)}); assert.equal((await request()).status, 403);
  await rebind({TRANSFER_UNTIL_MS: String(Date.now() + 16 * 60000)}); assert.equal((await request()).status, 403);
  assert.deepEqual(await rows(), []);
}));
test('changed source ETag refuses, and uncertain failure cannot retry mutations', () => fixture(async ({request, rebind, rows}) => {
  await rebind({SOURCE_ETAG: '0'.repeat(32)});
  assert.deepEqual(await request(), {status: 503, body: {error: 'SOURCE_CHANGED'}});
  assert.deepEqual(await rows(), []);
  assert.deepEqual(await request(), {status: 409, body: {error: 'ATTEMPT_ALREADY_SPENT'}});
}));
test('provider-enabled target refuses restore and never sends work', () => fixture(async ({request, rows}) => {
  assert.deepEqual(await request(), {status: 503, body: {error: 'STORAGE_BOUND'}});
  assert.deepEqual(await rows(), []);
}, {target: {JEV_API_KEY: 'synthetic-not-live'}}));
test('nonempty backup target refuses before restore, not by pruning existing data', () => fixture(async ({request, mf, rows}) => {
  const bucket = await mf.getR2Bucket('BACKUPS', 'bridge'); await bucket.put('snapshots/' + space + '/keep', 'synthetic-owned');
  assert.deepEqual(await request(), {status: 503, body: {error: 'TARGET_BACKUPS_NOT_EMPTY'}});
  assert.deepEqual(await rows(), []); assert.equal(await (await bucket.get('snapshots/' + space + '/keep')).text(), 'synthetic-owned');
}));
test('reconciliation rejects unrelated state changes, wrong revision and arbitrary exposure normalization', async () => {
  const source = await sourceArchive('queued'), now = Date.now();
  const actual = structuredClone(source); actual.created_ms = now;
  actual.app.revision++; actual.app.csrf = 'f'.repeat(48); actual.app.restored_paused = true;
  actual.app.backup = {completed_ms: null, key: null, sha256: null, error: null};
  const a = Object.values(actual.app.assessments)[0]; a.status = 'paused'; a.error = 'The check is paused after restore. Its saved answer can be checked by you.';
  actual.app.occurrence.phase = 'self-check'; actual.app.concepts[actual.app.occurrence.concept_id].exposure_ms = now;
  reconcile(source, actual, space, now - 1, now + 1);
  for (const mutate of [
    a => {a.app.revision++;}, a => {a.app.preferences.pace = 'intense';},
    a => {a.app.occurrence.answer += 'changed';}, a => {a.app.occurrence.assisted = !a.app.occurrence.assisted;},
    a => {a.app.concepts[a.app.occurrence.concept_id].exposure_ms = now + 2;},
    a => {Object.values(a.app.questions)[0].card.due_ms++;}, a => {a.app.spend.push({unexpected: true});},
    a => {a.app.operations = {};}, a => {a.app.unexpected = true;}, a => {a.unexpected = true;},
  ]) {const changed = structuredClone(actual); mutate(changed); assert.throws(() => reconcile(source, changed, space, now - 1, now + 1));}
});

test('complete photo bytes survive the actual native restore and backup', () => fixture(async ({request, mf}) => {
  const result = await request(); assert.equal(result.status, 200, JSON.stringify(result.body));
  assert.equal(result.body.photos_full_bytes_verified, 1);
  assert.equal((await (await mf.getR2Bucket('SCRY_ASSETS', 'target')).list()).objects.length, 1);
}, {change: archive => {
  const bytes = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);
  const photo = {key: 'photos/synthetic-source/photo', mime: 'image/png', size: bytes.length, sha256: hash(bytes)};
  Object.values(archive.app.goals)[0].photo = photo; archive.photos = [{photo, bytes: bytes.toString('base64')}];
}}));
test('photo-dominated archive above 1 MiB preserves the independent small App SQL bound', () => fixture(async ({request, mf, bytes, archive}) => {
  assert.ok(bytes.length > 1024 * 1024 && bytes.length <= 16 * 1024 * 1024);
  assert.ok(Buffer.byteLength(JSON.stringify(archive.app)) <= 1024 * 1024);
  const result = await request(); assert.equal(result.status, 200, JSON.stringify(result.body));
  assert.equal(result.body.photos_full_bytes_verified, 1);
  assert.ok(result.body.target_database_bytes < 32 * 1024 * 1024);
  const objects = await (await mf.getR2Bucket('SCRY_ASSETS', 'target')).list();
  assert.equal(objects.objects.length, 1); assert.equal(objects.objects[0].size, 4 * 1024 * 1024);
}, {change: archive => {
  const bytes = Buffer.alloc(4 * 1024 * 1024);
  bytes.set([137, 80, 78, 71, 13, 10, 26, 10]);
  const photo = {key: 'photos/synthetic-source/large-photo', mime: 'image/png', size: bytes.length, sha256: hash(bytes)};
  Object.values(archive.app.goals)[0].photo = photo; archive.photos = [{photo, bytes: bytes.toString('base64')}];
}}));
for (const [name, change] of [
  ['photo budget', a => {a.photos = Array.from({length: 33}, () => ({}));}],
  ['record budget', a => {a.app.unexpected = Array.from({length: 1001}, () => null);}],
  ['unsafe integer', a => {a.app.revision = Number.MAX_SAFE_INTEGER + 1;}],
]) test(name + ' refuses before target writes', () => fixture(async ({request, rows}) => {
  assert.deepEqual(await request(), {status: 503, body: {error: 'RECONCILIATION_FAILED'}});
  assert.deepEqual(await rows(), []);
}, {change}));

test('ETag quotes must be paired and quoted valid pins still conditionally match', () => fixture(async ({request, rows, rebind, bytes}) => {
  const etag = createHash('md5').update(bytes).digest('hex');
  for (const malformed of ['"' + etag, etag + '"']) {
    await rebind({SOURCE_ETAG: malformed});
    assert.deepEqual(await request(), {status: 403, body: {error: 'INVALID_PIN'}});
    assert.deepEqual(await rows(), []);
  }
  await rebind({SOURCE_ETAG: '"' + etag + '"'});
  assert.equal((await request()).status, 200);
}));


test('isolated target exposes only authenticated bounded metadata and two one-shot operations', () => fixture(async ({request, mf, bytes, rebindTarget, rows}) => {
  assert.equal((await (await mf.getWorker('target')).fetch('https://private.invalid/notebook')).status, 404);
  const ns = await mf.getDurableObjectNamespace('LEARNING_SPACE', 'target');
  const stub = ns.get(ns.idFromName(space));
  const storage = await stub.fetch('https://private.invalid/__recovery/storage', {headers: {'x-scry-internal-key': internalKey}});
  assert.equal(storage.status, 200);
  const size = await storage.json();
  assert.deepEqual(Object.keys(size).sort(), ['database_bytes', 'format', 'page_size']);
  assert.equal(size.page_size, 4096); assert.ok(size.database_bytes <= 65536);
  for (const path of ['/notebook', '/operator/restore-dry-run', '/__internal/health', '/__recovery/storage?extra=1']) {
    assert.equal((await stub.fetch('https://private.invalid' + path, {headers: {'x-scry-internal-key': internalKey}})).status, 403);
  }
  assert.equal((await stub.fetch('https://private.invalid/__recovery/storage')).status, 403);
  assert.equal((await request()).status, 200);
  const saved = await rows();
  assert.equal((await stub.fetch('https://private.invalid/operator/restore', {method: 'POST',
    headers: {'x-scry-internal-key': internalKey, 'x-scry-restore-token': token, 'x-scry-sha256': hash(bytes)}, body: bytes})).status, 409);
  assert.equal((await stub.fetch('https://private.invalid/__internal/daily', {method: 'POST',
    headers: {'x-scry-internal-key': internalKey}})).status, 409);
  await rebindTarget({TRANSFER_ISSUED_MS: String(Date.now() - 2000), TRANSFER_UNTIL_MS: String(Date.now() - 1000)});
  const expiredNs = await mf.getDurableObjectNamespace('LEARNING_SPACE', 'target');
  assert.equal((await expiredNs.get(expiredNs.idFromName(space)).fetch('https://private.invalid/__recovery/storage', {headers: {'x-scry-internal-key': internalKey}})).status, 403);
  assert.deepEqual(await rows(), saved);
}));

test('rehearsal-only object-key, App and archive byte caps refuse before native writes', async () => {
  await fixture(async ({request, rows}) => {
    assert.deepEqual(await request(), {status: 503, body: {error: 'RECONCILIATION_FAILED'}});
    assert.deepEqual(await rows(), []);
  }, {change: a => {a.app['x'.repeat(257)] = null;}});
  await fixture(async ({request, rows}) => {
    assert.deepEqual(await request(), {status: 503, body: {error: 'RECONCILIATION_FAILED'}});
    assert.deepEqual(await rows(), []);
  }, {change: a => {a.app.unexpected = 'x'.repeat(1024 * 1024);}});
  await fixture(async ({request, rows}) => {
    assert.deepEqual(await request(), {status: 403, body: {error: 'INVALID_PIN'}});
    assert.deepEqual(await rows(), []);
  }, {change: a => {a.photos = [{bytes: 'x'.repeat(16 * 1024 * 1024)}];}});
});

test('isolated adapter independently rejects oversized App and entry count before admission or native writes', async () => {
  const code = readFileSync('recovery/target-adapter.mjs', 'utf8')
    .replace("import {DurableObject} from 'cloudflare:workers';", 'class DurableObject {}')
    .replace("import {LearningSpace as NativeLearningSpace} from './index.js';", 'class NativeLearningSpace {fetch() {globalThis.syntheticNativeCalls++; throw Error("native must not run");}}');
  const {LearningSpace} = await import('data:text/javascript;base64,' + Buffer.from(code).toString('base64'));
  for (const app of [{payload: 'x'.repeat(1024 * 1024)}, {records: Array.from({length: 1001}, () => null)}]) {
    let transactions = 0; globalThis.syntheticNativeCalls = 0;
    const object = new LearningSpace({storage: {sql: {exec: () => ({toArray: () => [{page_size: 4096}]}), databaseSize: 4096},
      transaction: () => {transactions++; throw Error('admission must not run');}}},
    {SCRY_ENV: 'restore', INTERNAL_KEY: internalKey, TRANSFER_ISSUED_MS: String(Date.now() - 1000), TRANSFER_UNTIL_MS: String(Date.now() + 1000)});
    const body = JSON.stringify({app, photos: []}); assert.ok(Buffer.byteLength(body) < 16 * 1024 * 1024);
    const result = await object.fetch(new Request('https://private.invalid/operator/restore', {method: 'POST',
      headers: {'x-scry-internal-key': internalKey}, body}));
    assert.equal(result.status, 503); assert.deepEqual(await result.json(), {error: 'RECOVERY_FAILED'});
    assert.equal(transactions, 0); assert.equal(globalThis.syntheticNativeCalls, 0); assert.equal(typeof object.alarm, 'undefined');
  }
  delete globalThis.syntheticNativeCalls;
});

test('adapter refuses unsupported page sizes and oversized storage before native writes', async () => {
  // A structural unit test complements real workerd (which uses 4096-byte pages).
  const code = readFileSync('recovery/target-adapter.mjs', 'utf8')
    .replace("import {DurableObject} from 'cloudflare:workers';", 'class DurableObject {}')
    .replace("import {LearningSpace as NativeLearningSpace} from './index.js';", 'class NativeLearningSpace {fetch() {throw Error("native must not run");}}');
  const {LearningSpace} = await import('data:text/javascript;base64,' + Buffer.from(code).toString('base64'));
  for (const [page, bytes] of [[8192, 0], [4096, 32 * 1024 * 1024 + 1], [4096, NaN]]) {
    const object = new LearningSpace({storage: {sql: {exec: () => ({toArray: () => [{page_size: page}]}), databaseSize: bytes},
      transaction: () => {throw Error('admission must not run');}}},
    {SCRY_ENV: 'restore', INTERNAL_KEY: internalKey, TRANSFER_ISSUED_MS: String(Date.now() - 1000), TRANSFER_UNTIL_MS: String(Date.now() + 1000)});
    const result = await object.fetch(new Request('https://private.invalid/__recovery/storage', {headers: {'x-scry-internal-key': internalKey}}));
    assert.deepEqual(await result.json(), {error: 'RECOVERY_FAILED'}); assert.equal(result.status, 503);
    assert.equal(typeof object.alarm, 'undefined');
  }
});

test('rehearsal storage envelope covers every supported page size and obsolete meta versions', () => {
  const persistence = readFileSync('src/persistence.rs', 'utf8');
  const native = readFileSync('src/runtime.rs', 'utf8');
  assert.match(persistence, /MAX_RECORD_BYTES: usize = 1024 \* 1024/);
  assert.match(persistence, /PAYLOAD_CHUNK_BYTES: usize = 64 \* 1024/);
  assert.match(native, /CREATE TABLE IF NOT EXISTS records \(key TEXT PRIMARY KEY, value TEXT NOT NULL\) WITHOUT ROWID/);
  const source = 1024 * 1024, entries = 1000;
  for (const path of ['recovery/cloud-restore.mjs', 'recovery/target-adapter.mjs']) {
    const code = readFileSync(path, 'utf8');
    assert.match(code, /APP_LIMIT = 1024 \* 1024/);
    assert.match(code, /new TextEncoder\(\)\.encode\(JSON\.stringify\(source\.app\)\)\.length <= APP_LIMIT/);
  }
  // Partitions, not copies: first payload chunks consume existing field entries.
  const rows = entries + Math.ceil(source / (65536 - 3)) + 1;
  assert.equal(rows, 1018);
  // JSON-as-a-string <=2x; restore errors <=128 per entry; bounded keys/headers.
  assert.ok(2 * source + 128 * entries + 512 * rows < 4 * 1024 * 1024);
  // Even UTF-16 encoding (2x), plus two obsolete/new 1MiB meta values (2x).
  const allPayloadVersions = 12 * 1024 * 1024, allRows = rows + 2;
  for (const page of [512, 1024, 2048, 4096]) {
    // One B-tree page/row plus one rounding page per overflow chain. Charge
    // all meta versions, so freed pages are included; SQLite reuses freelists.
    const pages = 2 * allRows + 3 + Math.ceil(allPayloadVersions / (page - 259));
    // Maximum 255 reserved bytes; conservatively one pointer-map per51pages.
    // Fresh64KiB database + additional64KiB schema/KV/gate allowance.
    const bytes = (pages + Math.ceil(pages / 51)) * page + 2 * 65536;
    assert.ok(bytes <= 27161600 && bytes < 32 * 1024 * 1024);
  }
});
