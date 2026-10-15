// Native SQLite -> exact staged CF converter/R2 -> actual Rust/WASM restore.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {spawnSync} from 'node:child_process';
import test from 'node:test';
import * as runtime from 'miniflare';
import {createRuntime, internalKey} from './runtime-helper.mjs';
const root = resolve(process.env.SCRY_CONVERT_ARTIFACT || 'target/revival-convert');
const key = 'synthetic-conversion-key-more-than-32-characters';
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const cache = new Map();
function data(mode = 'valid') {
  if (!cache.has(mode)) {
    const native = spawnSync('python3', ['qa/revival-convert-fixture.py', mode], {maxBuffer: 8 * 1024 * 1024});
    assert.equal(native.status, 0, native.stderr.toString());
    const result = JSON.parse(native.stdout);
    result.bytes = Buffer.from(result.bytes, 'base64');
    cache.set(mode, result);
  }
  return cache.get(mode);
}
async function fixture(mode, fn) {
  const input = data(mode);
  const sourceKey = 'preserved-go/' + sha(input.bytes) + '/scry-synthetic-conversion.scry-backup.zip';
  let bindings = {CONVERT_KEY: key, CONVERT_UNTIL_MS: String(Date.now() + 300000), SOURCE_KEY: sourceKey,
    SOURCE_SHA256: sha(input.bytes), SOURCE_BYTES: String(input.bytes.length)};
  let egress = 0;
  const options = () => {
    const v4 = {name: 'scry-conversion-synthetic', modulesRoot: root,
      modules: readdirSync(root).filter(name => /\.(js|wasm|sql)$/.test(name))
        .sort((a, b) => Number(b === 'convert.js') - Number(a === 'convert.js'))
        .map(name => ({path: resolve(root, name), type: name.endsWith('.wasm') ? 'CompiledWasm' : name.endsWith('.sql') ? 'Text' : 'ESModule'})),
      cf: false, compatibilityDate: '2026-09-10', r2Buckets: ['SOURCE', 'DESTINATION'], bindings,
      outboundService: async () => {egress++; throw Error('External egress forbidden');}};
    return runtime.convertV4MiniflareOptions ? runtime.convertV4MiniflareOptions(v4) : v4;
  };
  const mf = new runtime.Miniflare(options());
  try {
    await mf.ready;
    const source = await mf.getR2Bucket('SOURCE');
    const bucket = name => new Proxy({}, {get: (_, field) => async (...args) => (await mf.getR2Bucket(name))[field](...args)});
    const destination = bucket('DESTINATION');
    const initial = await source.put(sourceKey, input.bytes);
    const initialVersion = initial.version;
    const request = async (init = {}, path = '/convert') => {
      const response = await (await mf.getWorker()).fetch('https://synthetic.invalid' + path, {
        method: 'POST', ...init, headers: {Authorization: 'Bearer ' + key, ...init.headers}});
      const text = await response.text();
      assert.doesNotMatch(text, /SYNTHETIC_PRIVATE|source0|goal0|quiz0|original-operation|synthetic-original-model/);
      assert.ok(!text.includes(key));
      return {status: response.status, body: JSON.parse(text)};
    };
    await fn({request, source, destination, input, rebind: async changes => {
      bindings = {...bindings, ...changes}; await mf.setOptions(options());
    }});
    const final = await (await mf.getR2Bucket('SOURCE')).get(sourceKey);
    assert.equal(final.version, initialVersion);
    assert.deepEqual(Buffer.from(await final.arrayBuffer()), input.bytes);
    assert.equal(egress, 0);
  } finally {await mf.dispose();}
}
async function restore(bytes, fn) {
  const name = 'synthetic-converted-archive-' + Math.random().toString(16).slice(2);
  const token = 'synthetic-conversion-restore-key-at-least-32-chars';
  const bindings = {SCRY_ENV: 'restore', SPACE_NAME: name, CANONICAL_ORIGIN: 'https://synthetic.invalid',
    RESTORE_TOKEN: token, INTERNAL_KEY: internalKey};
  const fixture = await createRuntime({name, bindings, inspect: true});
  try {
    const send = async data => fixture.worker.fetch('https://synthetic.invalid/operator/restore', {
      method: 'POST', headers: {'x-scry-restore-token': token, 'x-scry-sha256': sha(data)}, body: data});
    const response = await send(bytes);
    await fn({fixture, response, send, name});
  } finally {await fixture.mf.dispose();}
}

test('native source -> CF-only projection -> real Rust restore preserves content/history/schedules/custody/media', () => fixture('valid', async ({request, destination, input}) => {
  const first = await request();
  assert.equal(first.status, 200);
  assert.deepEqual(first.body.counts, {quizzes: 3, questions: 3, created_legacy_concepts: 2,
    created_legacy_notes: 2, goals: 2, concepts: 3, notes: 3, content_versions: 4, events: 1,
    overrides: 0, receipts: 2, photos: 1, spend: 2});
  assert.equal(first.body.rust_restore_verified, false);
  const object = await destination.get(first.body.destination_key);
  const bytes = Buffer.from(await object.arrayBuffer());
  assert.equal(bytes.length, first.body.destination_bytes);
  assert.equal(sha(bytes), first.body.destination_sha256);
  const archive = JSON.parse(bytes), app = archive.app;
  assert.deepEqual(await request(), first);
  const concurrent = await Promise.all([request(), request()]);
  assert.ok(concurrent.every(result => JSON.stringify(result) === JSON.stringify(first)));
  assert.equal((await destination.head(first.body.destination_key)).version, object.version);
  assert.ok(Object.values(app.questions).every(q => !q.archived));
  assert.equal(Object.keys(app.questions).length, 3);
  assert.equal(app.questions['legacy-import-quiz-quiz0'].versions[0].prompt, input.original_content.prompt);
  assert.notEqual(app.questions['legacy-import-quiz-quiz0'].versions[1].prompt, input.original_content.prompt);
  assert.deepEqual(app.questions['legacy-import-quiz-quiz0'].card, input.transition.after);
  assert.deepEqual(app.events[0].before_card, input.transition.before);
  assert.deepEqual(app.events[0].after_card, input.transition.after);
  assert.equal(app.events[0].result.rating, input.transition.rating);
  assert.equal(app.occurrence.phase, 'result');
  assert.equal(app.occurrence.presentation.version, 1);
  assert.equal(app.events.length, 1, 'No synthesized review events');
  assert.equal(app.concepts['legacy-import-quiz-concept-quiz1'].introduced_ms, null);
  assert.equal(app.concepts['legacy-import-quiz-concept-quiz1'].exposure_ms, null);
  assert.equal(app.concepts['legacy-import-quiz-concept-quiz1'].already_known, false);
  assert.match(app.concepts['legacy-import-quiz-concept-quiz1'].name, /Legacy import/);
  assert.equal(app.concepts['legacy-import-quiz-concept-quiz1'].notes[0].model, 'legacy-import');
  assert.ok(!Object.keys(app.concepts).some(id => id.includes('foundation')));
  assert.deepEqual(Buffer.concat(Object.entries(app.revival_go).sort().map(([, chunk]) => Buffer.from(chunk, 'base64'))), input.bytes);
  assert.equal(sha(Buffer.from(archive.photos[0].bytes, 'base64')), input.photo_sha256);
  for (const [section, hash] of Object.entries(first.body.projection_sha256)) assert.equal(sha(JSON.stringify(app[section])), hash);
  await restore(bytes, async ({response, fixture, send, name}) => {
    assert.equal(response.status, 200, await response.text());
    assert.equal((await send(bytes)).status, 409, 'Never overwrite acknowledged imported state');
    const storage = await fixture.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name});
    const rows = await storage.exec('SELECT key,value FROM records');
    assert.equal(rows.filter(row => row.key.startsWith('questions/')).length, 3);
    assert.equal(rows.filter(row => row.key.startsWith('events/')).length, 1);
    assert.equal(rows.filter(row => row.key.startsWith('revival_go/')).length, Object.keys(app.revival_go).length);
    const q = JSON.parse(rows.find(row => row.key === 'questions/legacy-import-quiz-quiz0').value);
    assert.deepEqual(q.card, input.transition.after);
    const meta = JSON.parse(rows.find(row => row.key === 'meta').value);
    assert.equal(meta.revision, 1);
    assert.equal(meta.restored_paused, true);
    assert.notEqual(meta.csrf, app.csrf);
    const assets = await fixture.mf.getR2Bucket('SCRY_ASSETS');
    const storedPhotos = await assets.list();
    assert.equal(storedPhotos.objects.length, 1);
    assert.equal(sha(Buffer.from(await (await assets.get(storedPhotos.objects[0].key)).arrayBuffer())), input.photo_sha256);
    const origin = 'http://127.0.0.1';
    const active = await fixture.rebind({SCRY_ENV: 'development', SPACE_NAME: name,
      CANONICAL_ORIGIN: origin, INTERNAL_KEY: internalKey});
    const exported = await active.worker.fetch(origin + '/export');
    assert.equal(exported.status, 200);
    const readback = await exported.json();
    for (const section of ['questions', 'concepts', 'events', 'operations', 'revival_go', 'revival_schedule_origins'])
      assert.deepEqual(readback.app[section], app[section]);
    const html = await (await active.worker.fetch(origin + '/create')).text();
    const fields = Object.fromEntries([...html.matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)]
      .map(match => [match[1], match[2]]));
    assert.ok(fields.csrf && fields.operation_id);
    const capture = () => active.worker.fetch(origin + '/create', {method: 'POST', redirect: 'manual',
      headers: {'X-Scry-Test-Origin': origin, 'Content-Type': 'application/x-www-form-urlencoded'},
      body: new URLSearchParams({...fields, intent: 'Synthetic accepted write after Go projection'})});
    assert.equal((await capture()).status, 303);
    assert.equal((await capture()).status, 303);
    const accepted = await (await active.worker.fetch(origin + '/export')).json();
    assert.equal(Object.keys(accepted.app.goals).length, 3);
    assert.equal(accepted.app.events.length, 1);
    assert.deepEqual(accepted.app.revival_schedule_origins, app.revival_schedule_origins);
    const snapshot = await active.worker.scheduled({cron: '17 3 * * *'});
    assert.equal(snapshot.outcome, 'ok');
    const backups = await fixture.mf.getR2Bucket('SCRY_BACKUPS');
    const list = await backups.list({prefix: 'snapshots/' + name + '/'});
    assert.equal(list.objects.length, 1);
    const independent = Buffer.from(await (await backups.get(list.objects[0].key)).arrayBuffer());
    const recovered = JSON.parse(independent);
    assert.equal(recovered.format, 'scry-rust-durable-sqlite-go-projection-v1');
    assert.equal(Object.keys(recovered.app.goals).length, 3);
    assert.deepEqual(recovered.app.revival_schedule_origins, app.revival_schedule_origins);
    await restore(independent, async ({response}) => assert.equal(response.status, 200, await response.text()));
  });
}));

test('forged/expired/body/alternate-path requests cannot create a projection', () => fixture('valid', async ({request, rebind, destination}) => {
  assert.equal((await request({headers: {Authorization: ''}})).status, 403);
  assert.equal((await request({headers: {Authorization: 'Bearer forged'}})).status, 403);
  assert.equal((await request({body: 'private'})).status, 400);
  assert.equal((await request({}, '/inspect')).status, 404);
  assert.equal((await request({}, '/convert?source=other')).status, 404);
  assert.equal((await request({method: 'GET'})).status, 405);
  await rebind({CONVERT_UNTIL_MS: String(Date.now() - 1)});
  assert.deepEqual(await request(), {status: 403, body: {error: 'CAPABILITY_EXPIRED'}});
  assert.equal((await destination.list()).objects.length, 0);
}));
for (const mode of ['unsupported-algorithm', 'inexact-timestamp', 'unsupported-web', 'unsafe-integer',
  'zero-rating', 'unsupported-correction', 'unsupported-assessment']) {
  test('refuses ' + mode + ' before destination writes without private diagnostics', () => fixture(mode, async ({request, destination}) => {
    assert.deepEqual(await request(), {status: 422, body: {error: 'INSPECTION_FAILED'}});
    assert.equal((await destination.list()).objects.length, 0);
  }));
}
test('a conflicting existing destination is never overwritten', () => fixture('valid', async ({request, destination}) => {
  const first = await request();
  assert.equal(first.status, 200);
  const key = first.body.destination_key;
  const existing = await destination.put(key, 'SYNTHETIC_PRIVATE_CONFLICT');
  assert.deepEqual(await request(), {status: 422, body: {error: 'INSPECTION_FAILED'}});
  assert.equal((await destination.head(key)).version, existing.version);
  assert.equal(await (await destination.get(key)).text(), 'SYNTHETIC_PRIVATE_CONFLICT');
}));
test('preparation is not Rust acceptance: actual restore rejects malformed content with zero SQL/assets writes', () => fixture('bad-content', async ({request, destination}) => {
  const result = await request();
  assert.equal(result.status, 200);
  assert.equal(result.body.rust_restore_verified, false);
  const bytes = Buffer.from(await (await destination.get(result.body.destination_key)).arrayBuffer());
  await restore(bytes, async ({response, fixture, name}) => {
    assert.equal(response.status, 422);
    const storage = await fixture.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name});
    assert.deepEqual(await storage.exec('SELECT key FROM records'), []);
    assert.equal((await (await fixture.mf.getR2Bucket('SCRY_ASSETS')).list()).objects.length, 0);
  });
}));
