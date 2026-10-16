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
async function fixture(mode, fn, {readOnly = false} = {}) {
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
      cf: false, compatibilityDate: '2026-09-10', r2Buckets: readOnly ? ['SOURCE'] : ['SOURCE', 'DESTINATION'], bindings,
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
  let egress = 0;
  const fixture = await createRuntime({name, bindings, inspect: true,
    egress: async () => {egress++; throw Error('External egress forbidden');}});
  try {
    const send = async data => fixture.worker.fetch('https://synthetic.invalid/operator/restore', {
      method: 'POST', headers: {'x-scry-restore-token': token, 'x-scry-sha256': sha(data)}, body: data});
    const response = await send(bytes);
    await fn({fixture, response, send, name});
  } finally {await fixture.mf.dispose(); assert.equal(egress, 0);}
}

for (const [mode, expected] of [
  ['valid', {}],
  ['selfcheck-repeated', {}],
  ['critic-lifecycles', {}],
  ['unsupported-algorithm', {'scheduler-algorithm': 3}],
  ['diagnostic-multiclass', {'content-grading': 1, 'event-transition': 1,
    'scheduler-algorithm': 3,
    'unsupported-assessments-or-corrections': 2, 'unsupported-web-documents': 2}],
  ['diagnostic-same-row', {'event-due': 1, 'event-observation': 1, 'event-observation-card': 1, 'event-transition': 1}],
  ['diagnostic-event-version', {'event-content': 1}],
  ['diagnostic-event-grading', {'content-grading': 1}],
  ['diagnostic-event-combined', {'content-grading': 1, 'event-content': 1}],
  ['diagnostic-event-nested', {'content-basis': 1, 'content-grading': 1, 'content-kind': 1,
    'content-quotes': 1, 'event-content': 1}],
  ['diagnostic-event-middle', {'content-basis': 2, 'content-grading': 2, 'content-kind': 1,
    'content-quotes': 1, 'event-content': 2}],
]) {
  test(mode + ': full-pass diagnostic returns only exact class/counts with no destination capability', () => fixture(mode, async ({request}) => {
    const first = await request({}, '/dry-run');
    assert.deepEqual(first, {status: 200, body: expected});
    assert.deepEqual(await request({}, '/dry-run'), first);
    assert.ok(Object.values(first.body).every(value => Number.isSafeInteger(value) && value > 0));
    assert.deepEqual(Object.keys(first.body), Object.keys(first.body).sort());
    assert.equal((await request()).status, 422, 'Real conversion still needs a separate destination capability');
  }, {readOnly: true}));
}
test('mid-dataset version mismatch collects nested and later refusals but conversion still writes nothing', () => fixture('diagnostic-event-middle', async ({request, destination}) => {
  assert.deepEqual(await request({}, '/dry-run'), {status: 200,
    body: {'content-basis': 2, 'content-grading': 2, 'content-kind': 1, 'content-quotes': 1, 'event-content': 2}});
  assert.deepEqual(await request(), {status: 422,
    body: {error: 'UNSUPPORTED_GO_PROJECTION', reason: 'content-grading'}});
  assert.equal((await destination.list()).objects.length, 0);
}));
test('dry-run refuses unknown/partial traversal instead of advertising a complete aggregate', () => fixture('private-shape-error', async ({request, destination}) => {
  assert.deepEqual(await request({}, '/dry-run'), {status: 422, body: {error: 'INSPECTION_FAILED'}});
  assert.equal((await destination.list()).objects.length, 0);
}));
test('dry-run never changes an existing destination and conversion still fails closed', () => fixture('diagnostic-multiclass', async ({request, destination}) => {
  const before = await destination.put('synthetic-existing', 'SYNTHETIC_PRIVATE_DESTINATION');
  assert.equal((await request({}, '/dry-run')).status, 200);
  assert.deepEqual(await request(), {status: 422,
    body: {error: 'UNSUPPORTED_GO_PROJECTION', reason: 'unsupported-assessments-or-corrections'}});
  const final = await destination.get('synthetic-existing');
  assert.equal(final.version, before.version);
  assert.equal(await final.text(), 'SYNTHETIC_PRIVATE_DESTINATION');
  assert.equal((await destination.list()).objects.length, 1);
}));
test('dry-run shares exact authentication, expiry, pin, method, body and route refusals', () => fixture('valid', async ({request, rebind, destination}) => {
  assert.equal((await request({headers: {Authorization: ''}}, '/dry-run')).status, 403);
  assert.equal((await request({headers: {Authorization: 'Bearer forged'}}, '/dry-run')).status, 403);
  assert.equal((await request({body: 'SYNTHETIC_PRIVATE'}, '/dry-run')).status, 400);
  assert.equal((await request({method: 'GET'}, '/dry-run')).status, 405);
  assert.equal((await request({}, '/dry-run?source=other')).status, 404);
  await rebind({SOURCE_BYTES: '1'});
  assert.equal((await request({}, '/dry-run')).status, 409);
  await rebind({CONVERT_UNTIL_MS: String(Date.now() - 1)});
  assert.deepEqual(await request({}, '/dry-run'), {status: 403, body: {error: 'CAPABILITY_EXPIRED'}});
  assert.equal((await destination.list()).objects.length, 0);
}));
test('same immutable dry-run capability expires on the actual clock with zero writes', () => fixture('valid', async ({request, rebind, destination}) => {
  const until = Date.now() + 1800;
  await rebind({CONVERT_UNTIL_MS: String(until)});
  assert.deepEqual(await request({}, '/dry-run'), {status: 200, body: {}});
  await new Promise(resolve => setTimeout(resolve, Math.max(0, until - Date.now()) + 50));
  assert.deepEqual(await request({}, '/dry-run'), {status: 403, body: {error: 'CAPABILITY_EXPIRED'}});
  assert.equal((await destination.list()).objects.length, 0);
}));

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
for (const [mode, expectedMicros, criticCharges] of [
  ['settled-generation', 1000, 0], ['unknown-generation', 1000000, 0],
  ['critic-spend', 1200000, 2], ['critic-lifecycles', 1700000, 6], ['critic-budget', 3600000, 2],
  ['legacy-generation-reservations', 2400000, 0], ['legacy-critic-reservations', 2400000, 4],
  ['legacy-critic-budget', 4000001, 4],
]) {
  test(mode + ': original monetary obligations survive CF projection, Rust restore and independent backup', () => fixture(mode, async ({request, destination, input}) => {
    const result = await request();
    assert.equal(result.status, 200);
    assert.equal(input.conservative_spent_micros, expectedMicros, 'Independent native SQL conservative allowance oracle');
    assert.ok(expectedMicros >= input.spent_micros, 'Never discard a retained monetary obligation');
    const bytes = Buffer.from(await (await destination.get(result.body.destination_key)).arrayBuffer());
    const original = JSON.parse(bytes).app;
    const selected = records => records.map(({id, work_id, created_ms, reserved_micros, cost_micros, status, model}) =>
      ({id, work_id, created_ms, reserved_micros, cost_micros, status, model}));
    const spent = app => app.spend.filter(s => s.created_ms > input.spend_at_ms - 86400000)
      .reduce((sum, s) => sum + (s.cost_micros ?? s.reserved_micros), 0);
    assert.equal(spent(original), expectedMicros);
    const critics = original.spend.filter(s => s.id.startsWith('legacy-import-critic-spend-'));
    assert.equal(critics.length, criticCharges);
    assert.ok(critics.every(s => s.work_id === 'legacy-import-job-job0' && s.model === 'synthetic-critic-model'));
    if (mode.startsWith('legacy-')) {
      const imported = mode === 'legacy-generation-reservations' ? original.spend : critics;
      assert.deepEqual(imported.map(s => s.reserved_micros), [500000, 500000, 500000,
        mode === 'legacy-critic-budget' ? 2500001 : 900000]);
      assert.ok(imported.every(s => s.cost_micros === null && s.status === 'unknown' && s.created_ms === input.spend_at_ms));
    } else if (mode === 'settled-generation') {
      assert.deepEqual(original.spend.map(s => s.cost_micros), [1000, 0]);
      assert.ok(original.spend.every(s => s.status === 'known'));
    } else if (mode === 'unknown-generation') {
      assert.ok(original.spend.every(s => s.cost_micros === null && s.status === 'unknown' && s.reserved_micros === 500000));
    } else {
      assert.equal(critics[0].cost_micros, mode === 'critic-budget' ? 3100000 : 700000);
      assert.equal(critics[1].cost_micros, null);
      assert.equal(critics[1].status, 'unknown');
      assert.equal(critics[1].reserved_micros, 500000);
      assert.equal(critics[1].created_ms, input.spend_at_ms, 'Never restart the unknown-charge clock');
      if (mode === 'critic-lifecycles') {
        assert.equal(critics[2].cost_micros, 0, 'Measured zero is not unknown');
        assert.equal(critics[3].status, 'unknown', 'Pending sent work never becomes a new runnable assessment');
        assert.equal(critics[4].created_ms, input.spend_at_ms - 86400001, 'Old costs stay historical');
        assert.equal(critics[5].cost_micros, null, 'Old unknown reservations remain inspectable');
      }
    }
    await restore(bytes, async ({response, fixture, name}) => {
      assert.equal(response.status, 200, await response.text());
      const origin = 'http://127.0.0.1';
      let active = await fixture.rebind({SCRY_ENV: 'development', SPACE_NAME: name, CANONICAL_ORIGIN: origin, INTERNAL_KEY: internalKey});
      const exported = await active.worker.fetch(origin + '/export');
      assert.equal(exported.status, 200);
      const readback = await exported.json();
      assert.deepEqual(selected(readback.app.spend), selected(original.spend));
      assert.equal(spent(readback.app), expectedMicros);
      assert.deepEqual(readback.app.assessments, {}, 'Never recreate critic send leases');
      assert.ok(Object.values(readback.app.jobs).every(j => j.lease === null && !['queued', 'sent', 'critic-sent'].includes(j.status)));
      const settings = await active.worker.fetch(origin + '/settings');
      assert.equal(settings.status, 200);
      assert.ok((await settings.text()).includes('$' + (expectedMicros / 1000000).toFixed(2)));
      if (mode === 'critic-budget' || mode === 'legacy-critic-budget') {
        // Exercise the actual Rust capture/claim boundary through its internal
        // authenticated DO interface. Outer Access ingress is tested separately.
        const configuredOrigin = 'https://synthetic.invalid';
        const operatorKey = 'synthetic-import-resume-key-at-least-32-chars';
        active = await fixture.rebind({SCRY_ENV: 'preview', SPACE_NAME: name,
          CANONICAL_ORIGIN: configuredOrigin, INTERNAL_KEY: internalKey, OPERATOR_KEY: operatorKey,
          OPENROUTER_API_KEY: 'synthetic-import-budget-key', OPENROUTER_MODEL: 'synthetic-import-model'});
        const ns = await fixture.mf.getDurableObjectNamespace('LEARNING_SPACE');
        const stub = ns.get(ns.idFromName(name));
        const request = (path, init = {}) => stub.fetch(configuredOrigin + path, {...init,
          headers: {'x-scry-internal-key': internalKey, 'x-scry-owner': 'private-owner', ...init.headers}});
        assert.equal((await request('/operator/resume-work', {method: 'POST',
          headers: {'x-scry-operator-key': operatorKey}})).status, 200);
        const page = await request('/create');
        assert.equal(page.status, 200);
        const html = await page.text();
        const fields = Object.fromEntries([...html.matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)]
          .map(match => [match[1], match[2]]));
        assert.ok(fields.csrf && fields.operation_id);
        const saved = await request('/create', {method: 'POST', redirect: 'manual',
          headers: {'Content-Type': 'application/x-www-form-urlencoded'},
          body: new URLSearchParams({...fields, intent: 'Synthetic deliberate new work after import'}).toString()});
        assert.equal(saved.status, 303, await saved.text());
        const goalId = saved.headers.get('Location').split('/').at(-1);
        const deadline = Date.now() + 5000;
        let app;
        do {
          app = (await (await request('/export')).json()).app;
          if (app.goals[goalId].status === 'failed') break;
          await new Promise(resolve => setTimeout(resolve, 50));
        } while (Date.now() < deadline);
        assert.equal(app.goals[goalId].status, 'failed');
        assert.match(app.jobs[app.goals[goalId].job_id].error, /allowance/);
        assert.deepEqual(selected(app.spend), selected(original.spend), 'No new reserve or paid send beyond imported allowance');
      }
      assert.equal((await active.worker.scheduled({cron: '17 3 * * *'})).outcome, 'ok');
      const backups = await fixture.mf.getR2Bucket('SCRY_BACKUPS');
      const list = await backups.list({prefix: 'snapshots/' + name + '/'});
      assert.ok(list.objects.length >= 1);
      const snapshot = Buffer.from(await (await backups.get(list.objects.at(-1).key)).arrayBuffer());
      await restore(snapshot, async ({response, fixture, name}) => {
        assert.equal(response.status, 200, await response.text());
        const active = await fixture.rebind({SCRY_ENV: 'development', SPACE_NAME: name, CANONICAL_ORIGIN: origin, INTERNAL_KEY: internalKey});
        const recovered = await (await active.worker.fetch(origin + '/export')).json();
        assert.deepEqual(selected(recovered.app.spend), selected(original.spend));
        assert.equal(spent(recovered.app), expectedMicros);
        assert.deepEqual(Buffer.concat(Object.entries(recovered.app.revival_go).sort().map(([, chunk]) => Buffer.from(chunk, 'base64'))), input.bytes);
      });
    });
  }));
}
for (const mode of ['zero-rating', 'zero-rating-wrong', 'zero-rating-recall-wrong', 'zero-rating-chain']) {
  test(mode + ': Go assisted observations retain original schedule epochs through Rust and second restore', () => fixture(mode, async ({request, destination, input}) => {
    const prepared = await request();
    assert.equal(prepared.status, 200);
    const bytes = Buffer.from(await (await destination.get(prepared.body.destination_key)).arrayBuffer());
    const app = JSON.parse(bytes).app;
    const question = app.questions['legacy-import-quiz-quiz0'];
    const chained = mode === 'zero-rating-chain';
    assert.equal(question.schedule_version, chained ? 2 : 1);
    assert.deepEqual(question.card, chained ? input.transition.after : input.transition.before);
    const observations = app.events.filter(e => e.result.rating === 0);
    assert.deepEqual(observations.map(e => e.schedule_version), chained ? [1, 2, 2] : [1]);
    assert.equal(app.events.length, chained ? 4 : 1);
    assert.ok(observations.every(e => e.assisted && e.result.outcome === (mode.endsWith('-wrong') ? 'warm_wrong' : 'warm_correct') &&
      e.result.due_ms === e.after_card.due_ms && JSON.stringify(e.before_card) === JSON.stringify(e.after_card)));
    await restore(bytes, async ({response, fixture, name}) => {
      assert.equal(response.status, 200, await response.text());
      const origin = 'http://127.0.0.1';
      const active = await fixture.rebind({SCRY_ENV: 'development', SPACE_NAME: name, CANONICAL_ORIGIN: origin, INTERNAL_KEY: internalKey});
      const read = async () => (await active.worker.fetch(origin + '/export')).json();
      const restored = (await read()).app;
      assert.deepEqual(restored.events, app.events);
      assert.deepEqual(restored.questions, app.questions);
      assert.deepEqual(restored.occurrence, app.occurrence);
      assert.deepEqual(restored.revival_go, app.revival_go);
      // Deliberate Next dismisses the imported held result, not its history.
      const fields = {csrf: restored.csrf, occurrence_id: restored.occurrence.id,
        operation_id: 'synthetic-next-after-go-noop'};
      const next = await active.worker.fetch(origin + '/review/next', {method: 'POST', redirect: 'manual',
        headers: {'X-Scry-Test-Origin': origin, 'Content-Type': 'application/x-www-form-urlencoded'},
        body: new URLSearchParams(fields)});
      assert.equal(next.status, 303);
      const snapshot = await active.worker.scheduled({cron: '17 3 * * *'});
      assert.equal(snapshot.outcome, 'ok');
      const backups = await fixture.mf.getR2Bucket('SCRY_BACKUPS');
      const list = await backups.list({prefix: 'snapshots/' + name + '/'});
      assert.equal(list.objects.length, 1);
      const independent = Buffer.from(await (await backups.get(list.objects[0].key)).arrayBuffer());
      await restore(independent, async ({response, fixture, name}) => {
        assert.equal(response.status, 200, await response.text());
        const recovered = await fixture.rebind({SCRY_ENV: 'development', SPACE_NAME: name, CANONICAL_ORIGIN: origin, INTERNAL_KEY: internalKey});
        const saved = (await (await recovered.worker.fetch(origin + '/export')).json()).app;
        assert.deepEqual(saved.events, app.events);
        assert.deepEqual(saved.questions, app.questions);
        assert.deepEqual(saved.revival_go, app.revival_go);
      });
    });
  }));
}
test('historical helped recall misses do not relax current identities, successful grades or assistance guards', () => fixture('zero-rating-recall-wrong', async ({request, destination}) => {
  const prepared = await request();
  assert.equal(prepared.status, 200);
  const bytes = Buffer.from(await (await destination.get(prepared.body.destination_key)).arrayBuffer());
  for (const mode of ['new-event', 'new-presentation', 'no-custody', 'no-original-epoch', 'unassisted', 'forged-success', 'authored-match']) {
    const archive = JSON.parse(bytes);
    const app = archive.app, event = app.events[0];
    if (mode === 'new-event') {
      event.id = 'new-native-event';
      event.result.event_id = event.id;
      app.occurrence.result.event_id = event.id;
    }
    if (mode === 'new-presentation') event.occurrence_id = app.occurrence.id = 'new-native-presentation';
    if (mode === 'no-custody') app.revival_go = {};
    if (mode === 'no-original-epoch') app.revival_schedule_origins = {};
    if (mode === 'unassisted') event.assisted = app.occurrence.assisted = false;
    if (mode === 'forged-success') event.result.outcome = app.occurrence.result.outcome = 'warm_correct';
    if (mode === 'authored-match') event.answer = app.occurrence.answer = 'freshness';
    await restore(Buffer.from(JSON.stringify(archive)), async ({response, fixture, name}) => {
      assert.equal(response.status, 422, mode);
      const sql = await fixture.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name});
      assert.deepEqual(await sql.exec('SELECT key FROM records'), [], mode);
      assert.equal((await (await fixture.mf.getR2Bucket('SCRY_ASSETS')).list()).objects.length, 0, mode);
    });
  }
}));
for (const mode of ['selfcheck', 'selfcheck-helped', 'selfcheck-history', 'selfcheck-repeated']) {
  test(mode + ': historical ungraded attempts remain ungraded across Rust, deliberate judgment and recovery', () => fixture(mode, async ({request, destination}) => {
    const prepared = await request();
    assert.equal(prepared.status, 200);
    const bytes = Buffer.from(await (await destination.get(prepared.body.destination_key)).arrayBuffer());
    const original = JSON.parse(bytes).app;
    const pending = original.events.filter(e => e.result.authority === 'ungraded');
    assert.equal(pending.length, mode === 'selfcheck-repeated' ? 2 : 1);
    assert.ok(pending.every(e => e.result.rating === 0 && e.schedule_version === 1 &&
      e.answer === 'FRESHNESS' && JSON.stringify(e.before_card) === JSON.stringify(e.after_card)));
    const historical = mode === 'selfcheck-history' || mode === 'selfcheck-repeated';
    assert.equal(original.occurrence.phase, historical ? 'result' : 'self-check');
    assert.equal(original.occurrence.assessment_id, null, 'Never invent an assessor invocation');
    await restore(bytes, async ({response, fixture, name}) => {
      assert.equal(response.status, 200, await response.text());
      const origin = 'http://127.0.0.1';
      const active = await fixture.rebind({SCRY_ENV: 'development', SPACE_NAME: name, CANONICAL_ORIGIN: origin, INTERNAL_KEY: internalKey});
      const read = async () => (await (await active.worker.fetch(origin + '/export')).json()).app;
      const restored = await read();
      assert.deepEqual(restored.events, original.events);
      assert.deepEqual(restored.questions, original.questions);
      const history = await (await active.worker.fetch(origin + '/history')).text();
      assert.ok(history.includes('Ungraded') && history.includes('Awaiting self-check'));
      const concept = await (await active.worker.fetch(origin + '/concepts/legacy-import-concept-concept0')).text();
      assert.equal((concept.match(/class="tally-/g) || []).length, historical ? 1 : 0,
        'Ungraded is neither a miss, unaided recall nor completed helped practice');
      if (!historical) {
        const fields = {csrf: restored.csrf, occurrence_id: restored.occurrence.id,
          operation_id: 'synthetic-self-after-import', correct: 'false'};
        const judge = () => active.worker.fetch(origin + '/review/self', {method: 'POST', redirect: 'manual',
          headers: {'X-Scry-Test-Origin': origin, 'Content-Type': 'application/x-www-form-urlencoded'}, body: new URLSearchParams(fields)});
        assert.equal((await judge()).status, 303);
        assert.equal((await judge()).status, 303);
        const judged = await read();
        assert.equal(judged.events.length, original.events.length + 1);
        assert.deepEqual(judged.events.slice(0, original.events.length), original.events);
        assert.equal(judged.events.at(-1).result.authority, 'learner');
        assert.equal(judged.questions['legacy-import-quiz-quiz0'].schedule_version, 2);
      }
      assert.equal((await active.worker.scheduled({cron: '17 3 * * *'})).outcome, 'ok');
      const backups = await fixture.mf.getR2Bucket('SCRY_BACKUPS');
      const listing = await backups.list({prefix: 'snapshots/' + name + '/'});
      const independent = Buffer.from(await (await backups.get(listing.objects[0].key)).arrayBuffer());
      const saved = await read();
      await restore(independent, async ({response, fixture, name}) => {
        assert.equal(response.status, 200, await response.text());
        const recovered = await fixture.rebind({SCRY_ENV: 'development', SPACE_NAME: name, CANONICAL_ORIGIN: origin, INTERNAL_KEY: internalKey});
        const app = (await (await recovered.worker.fetch(origin + '/export')).json()).app;
        assert.deepEqual(app.events, saved.events);
        assert.deepEqual(app.questions, saved.questions);
        assert.deepEqual(app.revival_go, saved.revival_go);
      });
    });
  }));
}
test('imported ungraded history refuses forged state before any SQL or photo writes', () => fixture('selfcheck', async ({request, destination}) => {
  const prepared = await request();
  assert.equal(prepared.status, 200);
  const original = JSON.parse(await (await destination.get(prepared.body.destination_key)).text());
  for (const mode of ['origin', 'custody', 'marker', 'authority', 'card', 'epoch', 'known-answer', 'current-answer', 'current-assistance', 'current-phase']) {
    const archive = structuredClone(original), app = archive.app, event = app.events[0];
    if (mode === 'origin') app.revival_schedule_origins = {};
    if (mode === 'custody') app.revival_go = {};
    if (mode === 'marker') {event.id = 'ordinary-event'; event.result.event_id = event.id;}
    if (mode === 'authority') event.result.authority = 'exact';
    if (mode === 'card') event.after_card.due_ms++;
    if (mode === 'epoch') event.schedule_version++;
    if (mode === 'known-answer') event.answer = event.presentation.answer;
    if (mode === 'current-answer') app.occurrence.answer = 'A different answer';
    if (mode === 'current-assistance') app.occurrence.assisted = !app.occurrence.assisted;
    if (mode === 'current-phase') app.occurrence.phase = 'checking';
    await restore(Buffer.from(JSON.stringify(archive)), async ({response, fixture, name}) => {
      assert.equal(response.status, 422, mode);
      const sql = await fixture.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name});
      assert.deepEqual(await sql.exec('SELECT key FROM records'), []);
      assert.equal((await (await fixture.mf.getR2Bucket('SCRY_ASSETS')).list()).objects.length, 0);
    });
  }
}));
for (const [mode, reason] of [
  ['selfcheck-changing-card', 'event-observation-card'],
  ['selfcheck-changing-version', 'event-transition'],
  ['selfcheck-bad-policy', 'event-observation'],
  ['unsupported-algorithm', 'scheduler-algorithm'],
  ['inexact-timestamp', 'timestamp-precision'],
  ['private-shape-error', null], // Unexpected TypeError stays private/generic.
  ['unsupported-web', 'unsupported-web-documents'],
  ['unsafe-integer', null], // Reader failure, not a projection refusal.
  ['zero-rating-changing-card', 'event-observation-card'],
  ['zero-rating-changing-version', 'event-transition'],
  ['zero-rating-unassisted', 'event-observation'],
  ['zero-rating-bad-outcome', 'event-observation'],
  ['zero-rating-bad-due', 'event-due'],
  ['unsupported-correction', 'unsupported-assessments-or-corrections'],
  ['unsupported-assessment', 'unsupported-assessments-or-corrections'],
  ['unsupported-old-active', 'old-pending-usage'],
  ['unsupported-critic-purpose', 'critic-policy'],
  ['unsupported-critic-owner', 'critic-ownership'],
  ['unsupported-critic-unsent', 'invariant'],
  ['unsupported-critic-old-pending', 'old-pending-usage'],
]) {
  test('refuses ' + mode + ' before destination writes without private diagnostics', () => fixture(mode, async ({request, destination}) => {
    assert.deepEqual(await request(), {status: 422, body: reason ?
      {error: 'UNSUPPORTED_GO_PROJECTION', reason} : {error: 'INSPECTION_FAILED'}});
    assert.equal((await destination.list()).objects.length, 0);
  }));
}
test('larger unknown imported reservations cannot relax ordinary archive accounting', () => fixture('legacy-critic-reservations', async ({request, destination}) => {
  const prepared = await request();
  assert.equal(prepared.status, 200);
  const original = JSON.parse(await (await destination.get(prepared.body.destination_key)).text());
  for (const mode of ['custody', 'spend-marker', 'job-marker', 'known', 'below-floor']) {
    const archive = structuredClone(original), app = archive.app;
    const spend = app.spend.find(s => s.reserved_micros === 900000);
    assert.ok(spend);
    if (mode === 'custody') app.revival_go = {};
    if (mode === 'spend-marker') spend.id = 'ordinary-paid-attempt';
    if (mode === 'job-marker') {
      const old = spend.work_id, job = app.jobs[old];
      delete app.jobs[old]; job.id = 'ordinary-job'; app.jobs[job.id] = job;
      for (const goal of Object.values(app.goals)) if (goal.job_id === old) goal.job_id = job.id;
      for (const charge of app.spend) if (charge.work_id === old) charge.work_id = job.id;
    }
    if (mode === 'known') {spend.cost_micros = 0; spend.status = 'known';}
    if (mode === 'below-floor') spend.reserved_micros = 499999;
    await restore(Buffer.from(JSON.stringify(archive)), async ({response, fixture, name}) => {
      assert.equal(response.status, 422, mode);
      const storage = await fixture.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name});
      assert.deepEqual(await storage.exec('SELECT key FROM records'), []);
      assert.equal((await (await fixture.mf.getR2Bucket('SCRY_ASSETS')).list()).objects.length, 0);
    });
  }
}));
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
