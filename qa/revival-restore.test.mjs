// Native Go SQLite -> exact converter -> CF-only bridge -> real Rust SQLite/R2.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync, readdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {spawnSync} from 'node:child_process';
import test from 'node:test';
import * as runtime from 'miniflare';
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const convertRoot = resolve(process.env.SCRY_CONVERT_ARTIFACT || 'target/revival-convert');
const artifact = resolve(process.env.SCRY_WORKER_ARTIFACT || 'build');
const bridgePath = resolve('revival/restore.mjs');
const bridgeScript = readFileSync(bridgePath, 'utf8');
const internal = 'synthetic-only-internal-key-more-than32characters';
const token = 'synthetic-only-restore-key-more-than32characters';
const key = 'synthetic-only-transfer-key-more-than32characters';
const space = 'synthetic-private-restore';
const deny = async () => {throw Error('No external egress authorized');};
const options = input => runtime.convertV4MiniflareOptions ? runtime.convertV4MiniflareOptions(input) : input;
const cache = new Map();
async function converted(mode) {
  if (cache.has(mode)) return cache.get(mode);
  const native = spawnSync('python3', ['qa/revival-convert-fixture.py', mode], {maxBuffer: 8 * 1024 * 1024});
  assert.equal(native.status, 0, native.stderr.toString());
  const bytes = Buffer.from(JSON.parse(native.stdout).bytes, 'base64');
  const sourceKey = 'preserved-go/' + sha(bytes) + '/scry-synthetic-bridge.scry-backup.zip';
  const mf = new runtime.Miniflare(options({name: 'converter', compatibilityDate: '2026-09-10', cf: false,
    modulesRoot: convertRoot, modules: readdirSync(convertRoot).filter(name => /\.(js|wasm|sql)$/.test(name))
      .sort((a, b) => Number(b === 'convert.js') - Number(a === 'convert.js'))
      .map(name => ({path: resolve(convertRoot, name), type: name.endsWith('.wasm') ? 'CompiledWasm' : name.endsWith('.sql') ? 'Text' : 'ESModule'})),
    r2Buckets: ['SOURCE', 'DESTINATION'], outboundService: deny,
    bindings: {CONVERT_KEY: key, CONVERT_UNTIL_MS: String(Date.now() + 300000), SOURCE_KEY: sourceKey,
      SOURCE_BYTES: String(bytes.length), SOURCE_SHA256: sha(bytes)}}));
  try {
    await (await mf.getR2Bucket('SOURCE')).put(sourceKey, bytes);
    const response = await (await mf.getWorker()).fetch('https://synthetic.invalid/convert',
      {method: 'POST', headers: {Authorization: 'Bearer ' + key}});
    assert.equal(response.status, 200);
    const receipt = await response.json();
    const candidate = Buffer.from(await (await (await mf.getR2Bucket('DESTINATION')).get(receipt.destination_key)).arrayBuffer());
    assert.equal(sha(candidate), receipt.destination_sha256);
    const result = {candidate, receipt}; cache.set(mode, result); return result;
  } finally {await mf.dispose();}
}
async function fixture(fn, mode = 'valid') {
  const {candidate, receipt} = await converted(mode);
  let bindings = {TRANSFER_KEY: key, TRANSFER_UNTIL_MS: String(Date.now() + 300000),
    SOURCE_KEY: receipt.destination_key, SOURCE_SHA256: sha(candidate), SOURCE_BYTES: String(candidate.length),
    SPACE_NAME: space, INTERNAL_KEY: internal, RESTORE_TOKEN: token};
  let targetBindings = {SCRY_ENV: 'restore', SPACE_NAME: space, INTERNAL_KEY: internal, RESTORE_TOKEN: token};
  let backupsEnabled = true;
  let egress = 0;
  const outboundService = async () => {egress++; return deny();};
  const makeOptions = () => options({unsafeInspectDurableObjects: true, workers: [
    {name: 'bridge', compatibilityDate: '2026-09-10', cf: false, modulesRoot: resolve('revival'),
      modules: [{type: 'ESModule', path: bridgePath, contents: bridgeScript}], bindings,
      r2Buckets: {SOURCE: 'candidate', BACKUPS: 'backups'},
      durableObjects: {TARGET: {className: 'LearningSpace', scriptName: 'target', useSQLite: true}}, outboundService},
    {name: 'target', compatibilityDate: '2026-09-10', cf: false, modulesRoot: artifact,
      modules: [{type: 'ESModule', path: resolve(artifact, 'bridge-adapter.mjs'), contents:
        `import {DurableObject} from 'cloudflare:workers';import App,{LearningSpace as Rust} from './index.js';export default App;export class LearningSpace extends DurableObject{constructor(ctx,env){super(ctx,env);this.inner=new Rust(ctx,env);}fetch(req){return this.inner.fetch(req);}alarm(){return this.inner.alarm();}}`},
      {type: 'ESModule', path: resolve(artifact, 'index.js'), contents: readFileSync(resolve(artifact, 'index.js'), 'utf8')},
      {type: 'CompiledWasm', path: resolve(artifact, 'index_bg.wasm'), contents: readFileSync(resolve(artifact, 'index_bg.wasm'))}],
      bindings: targetBindings, durableObjects: {LEARNING_SPACE: {className: 'LearningSpace', useSQLite: true}},
      r2Buckets: {SCRY_ASSETS: 'assets', ...(backupsEnabled ? {SCRY_BACKUPS: 'backups'} : {})}, outboundService},
  ]});
  const mf = new runtime.Miniflare(makeOptions());
  const bucket = async (name, worker = 'bridge') => mf.getR2Bucket(name, worker);
  try {
    const source = await bucket('SOURCE');
    const initialVersion = (await source.put(receipt.destination_key, candidate)).version;
    const request = async (path = '/restore', init = {}) => {
      const response = await (await mf.getWorker('bridge')).fetch('https://bridge.invalid' + path,
        {method: 'POST', ...init, headers: {Authorization: 'Bearer ' + key, ...init.headers}});
      const text = await response.text();
      assert.doesNotMatch(text, /SYNTHETIC_PRIVATE|source0|goal0|quiz0|original-operation|synthetic-original-model/);
      for (const secret of [key, token, internal]) assert.ok(!text.includes(secret));
      return {status: response.status, body: JSON.parse(text)};
    };
    const rows = async () => {
      const storage = await mf.unsafeGetDurableObjectStorage('target', 'LearningSpace', {name: space});
      const tables = await storage.exec("SELECT name FROM sqlite_master WHERE type='table' AND name='records'");
      return tables.length ? storage.exec('SELECT key,value FROM records ORDER BY key') : [];
    };
    const rebind = async (changes, target = {}, withBackups = true) => {
      bindings = {...bindings, ...changes}; targetBindings = {...targetBindings, ...target};
      backupsEnabled = withBackups; await mf.setOptions(makeOptions());
    };
    await fn({mf, request, bucket, rows, rebind, candidate, receipt});
    const final = await (await bucket('SOURCE')).get(receipt.destination_key);
    assert.equal(final.version, initialVersion);
    assert.deepEqual(Buffer.from(await final.arrayBuffer()), candidate);
    assert.equal(egress, 0);
  } finally {await mf.dispose();}
}

for (const mode of ['valid', 'selfcheck-repeated', 'legacy-operation-identities', 'legacy-generation-reservations', 'critic-lifecycles', 'legacy-critic-reservations']) {
  test(mode + ': bridge restores and strictly reconciles full state and independent complete snapshot', () => fixture(async ({request, bucket, rows, receipt}) => {
    const first = await request();
    assert.equal(first.status, 200, JSON.stringify(first.body));
    assert.equal(first.body.canonical_application_equal, true);
    assert.equal(first.body.photos_full_bytes_verified, receipt.counts.photos);
    const snapshot = await (await bucket('BACKUPS')).get(first.body.snapshot_key);
    const bytes = Buffer.from(await snapshot.arrayBuffer());
    assert.equal(bytes.length, first.body.snapshot_bytes);
    assert.equal(sha(bytes), first.body.snapshot_sha256);
    const state = JSON.parse(bytes).app;
    assert.equal(state.restored_paused, true);
    assert.equal(state.questions && Object.keys(state.questions).length, receipt.counts.questions);
    assert.equal(state.events.length, receipt.counts.events);
    const before = await rows();
    assert.deepEqual(await request(), {status: 409, body: {error: 'RESTORE_REFUSED'}});
    assert.deepEqual(await rows(), before, 'Replay cannot overwrite accepted state');
    assert.equal((await request('/verify')).status, 200);
  }, mode));
}
test('wrong capability/method/body/pins/expiry fail without initializing target or writing assets/backups', () => fixture(async ({request, bucket, rows, rebind}) => {
  assert.equal((await request('/restore', {headers: {Authorization: ''}})).status, 403);
  assert.equal((await request('/restore', {headers: {Authorization: 'Bearer wrong'}})).status, 403);
  assert.equal((await request('/restore?source=anything')).status, 404);
  assert.equal((await request('/restore', {method: 'GET'})).status, 405);
  assert.equal((await request('/restore', {body: 'SYNTHETIC_PRIVATE_INPUT'})).status, 400);
  assert.deepEqual(await request('/verify'), {status: 409, body: {error: 'VERIFY_TARGET_UNAVAILABLE'}});
  await rebind({SOURCE_SHA256: '0'.repeat(64)});
  assert.equal((await request()).status, 409);
  await rebind({TRANSFER_UNTIL_MS: String(Date.now() - 1)});
  assert.deepEqual(await request(), {status: 403, body: {error: 'CAPABILITY_EXPIRED'}});
  assert.equal((await rows()).length, 0);
  assert.equal((await (await bucket('BACKUPS')).list()).objects.length, 0);
  assert.equal((await (await bucket('SCRY_ASSETS', 'target')).list()).objects.length, 0);
}));
test('wrong native restore token is denied; restoring it recovers with no bypass or overwrite', () => fixture(async ({request, rows, rebind}) => {
  await rebind({RESTORE_TOKEN: key});
  assert.deepEqual(await request(), {status: 403, body: {error: 'RESTORE_REFUSED'}});
  assert.equal((await rows()).length, 0);
  await rebind({RESTORE_TOKEN: token});
  assert.equal((await request()).status, 200);
}));
test('same immutable bridge expires on actual clock before touching a target', () => fixture(async ({request, rebind, rows}) => {
  const until = Date.now() + 1500;
  await rebind({TRANSFER_UNTIL_MS: String(until)});
  await new Promise(resolve => setTimeout(resolve, Math.max(0, until - Date.now()) + 30));
  assert.deepEqual(await request(), {status: 403, body: {error: 'CAPABILITY_EXPIRED'}});
  assert.equal((await rows()).length, 0);
}));
test('failed backup after committed restore recovers through verify without overwriting the target', () => fixture(async ({request, rebind, rows}) => {
  await rebind({}, {}, false);
  assert.deepEqual(await request(), {status: 503, body: {error: 'TRANSFER_FAILED'}});
  const accepted = (await rows()).filter(row => row.key !== 'meta');
  assert.ok(accepted.length > 0, 'Native restore already committed before the dependency failure');
  await rebind({});
  assert.deepEqual(await request(), {status: 409, body: {error: 'RESTORE_REFUSED'}});
  assert.equal((await request('/verify')).status, 200);
  assert.deepEqual((await rows()).filter(row => row.key !== 'meta'), accepted);
}));
test('post-restore accepted changes are detected, never normalized away or overwritten', () => fixture(async ({request, mf, rows}) => {
  assert.equal((await request()).status, 200);
  const storage = await mf.unsafeGetDurableObjectStorage('target', 'LearningSpace', {name: space});
  const metaRow = (await rows()).find(row => row.key === 'meta');
  const meta = JSON.parse(metaRow.value); meta.preferences.pace = 'intense';
  await storage.exec("UPDATE records SET value=? WHERE key='meta'", JSON.stringify(meta));
  assert.deepEqual(await request('/verify'), {status: 409, body: {error: 'RECONCILIATION_FAILED'}});
  assert.equal(JSON.parse((await rows()).find(row => row.key === 'meta').value).preferences.pace, 'intense');
}));

test('every protected record family and media stays in the canonical comparison', () => fixture(async ({request, bucket, candidate, rebind, receipt}) => {
  assert.equal((await request()).status, 200);
  const mutations = [
    app => {Object.values(app.goals)[0].intent += 'SYNTHETIC_PRIVATE';},
    app => {Object.values(app.concepts)[0].notes[0].text += 'SYNTHETIC_PRIVATE';},
    app => {Object.values(app.questions)[0].versions[0].prompt += 'SYNTHETIC_PRIVATE';},
    app => {Object.values(app.questions)[0].card.due_ms++;},
    app => {app.events[0].answer += 'SYNTHETIC_PRIVATE';},
    app => {app.events[0].assisted = !app.events[0].assisted;},
    app => {Object.values(app.operations)[0].payload_hash = 'f'.repeat(64);},
    app => {app.spend[0].reserved_micros++;},
    app => {app.spend[0].created_ms++;},
    app => {app.revival_go[Object.keys(app.revival_go)[0]] = 'SYNTHETIC_PRIVATE';},
    app => {app.revival_schedule_origins[Object.keys(app.questions)[0]]++;},
    app => {app.preferences.unexpected = true;},
  ];
  for (const mutate of mutations) {
    const baseline = JSON.parse(candidate); mutate(baseline.app);
    const bytes = Buffer.from(JSON.stringify(baseline)), digest = sha(bytes);
    const pin = 'converted-go/' + '0'.repeat(64) + '/' + digest + '.json';
    await (await bucket('SOURCE')).put(pin, bytes);
    await rebind({SOURCE_KEY: pin, SOURCE_BYTES: String(bytes.length), SOURCE_SHA256: digest});
    assert.deepEqual(await request('/verify'), {status: 409, body: {error: 'RECONCILIATION_FAILED'}});
  }
  const baseline = JSON.parse(candidate); baseline.photos[0].bytes = 'SYNTHETIC_PRIVATE';
  const bytes = Buffer.from(JSON.stringify(baseline)), digest = sha(bytes);
  const pin = 'converted-go/' + '0'.repeat(64) + '/' + digest + '.json';
  await (await bucket('SOURCE')).put(pin, bytes);
  await rebind({SOURCE_KEY: pin, SOURCE_BYTES: String(bytes.length), SOURCE_SHA256: digest});
  assert.deepEqual(await request('/verify'), {status: 409, body: {error: 'RECONCILIATION_FAILED'}});
  await rebind({SOURCE_KEY: receipt.destination_key, SOURCE_BYTES: String(candidate.length), SOURCE_SHA256: sha(candidate)});
  assert.equal((await request('/verify')).status, 200, 'Exact original pin recovers without rewriting any record');
}));

for (const mode of ['valid', 'selfcheck-repeated', 'legacy-operation-identities', 'critic-lifecycles', 'legacy-generation-reservations', 'legacy-critic-reservations']) {
  test(mode + ': complete native diagnostic is read-only and agrees with actual import', () => fixture(async ({request, rows, bucket}) => {
    assert.deepEqual(await request('/diagnose'), {status: 200, body: {}});
    assert.deepEqual(await request('/diagnose'), {status: 200, body: {}});
    assert.equal((await rows()).length, 0);
    assert.equal((await (await bucket('BACKUPS')).list()).objects.length, 0);
    assert.equal((await (await bucket('SCRY_ASSETS', 'target')).list()).objects.length, 0);
    assert.equal((await request()).status, 200);
    const accepted = await rows();
    assert.deepEqual(await request('/diagnose'), {status: 200, body: {}});
    assert.deepEqual(await rows(), accepted, 'Diagnostic never changes a used space either');
  }, mode));
}
test('native full pass collects independent nested/same-row and later-record failures, import still fails closed', () => fixture(async ({request, rows, candidate, bucket, rebind}) => {
  const archive = JSON.parse(candidate);
  const questions = Object.values(archive.app.questions);
  questions[0].versions[0].model = 'x'.repeat(257);
  questions[0].versions[0].prompt = '';
  questions[1].card.lapses = 1;
  archive.app.events[0].algorithm = 'SYNTHETIC_PRIVATE_BAD_ALGORITHM';
  archive.app.events[0].answer = 'SYNTHETIC_PRIVATE_UNMATCHED';
  archive.app.spend[0].status = 'SYNTHETIC_PRIVATE_BAD_STATUS';
  const bytes = Buffer.from(JSON.stringify(archive)), digest = sha(bytes);
  const pin = 'converted-go/' + '0'.repeat(64) + '/' + digest + '.json';
  await (await bucket('SOURCE')).put(pin, bytes);
  await rebind({SOURCE_KEY: pin, SOURCE_BYTES: String(bytes.length), SOURCE_SHA256: digest});
  const result = await request('/diagnose');
  assert.equal(result.status, 200, JSON.stringify(result.body));
  for (const name of ['invalid-content-attribution-or-version', 'invalid-question-content-or-evidence',
    'invalid-complete-scheduler-state', 'an-incompatible-event-algorithm',
    'an-exact-grade-without-an-authored-answer-match', 'orphaned-or-inconsistent-paid-usage']) assert.equal(result.body[name], 1, name);
  assert.deepEqual(await request('/diagnose'), result);
  assert.deepEqual(await request(), {status: 422, body: {error: 'RESTORE_REFUSED'}});
  assert.equal((await rows()).length, 0);
  assert.equal((await (await bucket('BACKUPS')).list()).objects.length, 0);
  assert.equal((await (await bucket('SCRY_ASSETS', 'target')).list()).objects.length, 0);
}));
test('native incomplete dependency/media traversal returns no partial aggregate and recovers on original pin', () => fixture(async ({request, rows, candidate, bucket, rebind, receipt}) => {
  for (const mutate of [
    archive => {Object.values(archive.app.concepts)[0].goal_id = 'SYNTHETIC_PRIVATE_MISSING';},
    archive => {archive.photos[0].bytes = 'AAAA';},
    archive => {archive.app.events[0].result.authority = 'SYNTHETIC_PRIVATE_UNKNOWN';},
  ]) {
    const archive = JSON.parse(candidate); mutate(archive);
    const bytes = Buffer.from(JSON.stringify(archive)), digest = sha(bytes);
    const pin = 'converted-go/' + '0'.repeat(64) + '/' + digest + '.json';
    await (await bucket('SOURCE')).put(pin, bytes);
    await rebind({SOURCE_KEY: pin, SOURCE_BYTES: String(bytes.length), SOURCE_SHA256: digest});
    assert.deepEqual(await request('/diagnose'), {status: 422, body: {error: 'INCOMPLETE_IMPORT_REPORT'}});
    assert.equal((await rows()).length, 0);
  }
  await rebind({SOURCE_KEY: receipt.destination_key, SOURCE_BYTES: String(candidate.length), SOURCE_SHA256: sha(candidate)});
  assert.deepEqual(await request('/diagnose'), {status: 200, body: {}});
  assert.equal((await request()).status, 200);
}));
test('mid-dataset version failure does not hide nested content guards or later records', () => fixture(async ({request, rows, candidate, bucket, rebind}) => {
  const archive = JSON.parse(candidate), questions = Object.values(archive.app.questions);
  for (const index of [0, 1, 2]) questions[index].versions[0].model = 'x'.repeat(257);
  questions[1].versions[0].version = 0;
  questions[1].versions[0].prompt = '';
  questions[2].versions[0].prompt = '';
  const bytes = Buffer.from(JSON.stringify(archive)), digest = sha(bytes);
  const pin = 'converted-go/' + '0'.repeat(64) + '/' + digest + '.json';
  await (await bucket('SOURCE')).put(pin, bytes);
  await rebind({SOURCE_KEY: pin, SOURCE_BYTES: String(bytes.length), SOURCE_SHA256: digest});
  const result = await request('/diagnose');
  assert.equal(result.status, 200);
  assert.equal(result.body['invalid-content-attribution-or-version'], 3, 'Version and attribution count once per question');
  assert.equal(result.body['invalid-question-content-or-evidence'], 2);
  assert.equal(result.body['broken-question-versions-or-idea-references'], 1);
  assert.equal((await rows()).length, 0);
  assert.deepEqual(await request(), {status: 422, body: {error: 'RESTORE_REFUSED'}});
}));
test('native diagnostic shares capability, expiry, mode, paid-key and method fences', () => fixture(async ({request, rebind, rows}) => {
  assert.equal((await request('/diagnose', {headers: {Authorization: 'Bearer invalid'}})).status, 403);
  assert.equal((await request('/diagnose', {method: 'GET'})).status, 405);
  assert.equal((await request('/diagnose', {body: 'SYNTHETIC_PRIVATE'})).status, 400);
  await rebind({RESTORE_TOKEN: key});
  assert.deepEqual(await request('/diagnose'), {status: 422, body: {error: 'INCOMPLETE_IMPORT_REPORT'}});
  await rebind({RESTORE_TOKEN: token}, {OPENROUTER_API_KEY: 'synthetic-not-a-real-key'});
  assert.deepEqual(await request('/diagnose'), {status: 422, body: {error: 'INCOMPLETE_IMPORT_REPORT'}});
  await rebind({}, {OPENROUTER_API_KEY: '', SCRY_ENV: 'production'});
  assert.deepEqual(await request('/diagnose'), {status: 422, body: {error: 'INCOMPLETE_IMPORT_REPORT'}});
  await rebind({}, {SCRY_ENV: 'restore'});
  assert.deepEqual(await request('/diagnose'), {status: 200, body: {}});
  await rebind({TRANSFER_UNTIL_MS: String(Date.now() - 1)});
  assert.deepEqual(await request('/diagnose'), {status: 403, body: {error: 'CAPABILITY_EXPIRED'}});
  assert.equal((await rows()).length, 0);
}));
test('retained receipt allowance rejects missing custody/marker, bad hash and invalid Go text without writes', () => fixture(async ({request, rows, candidate, bucket, rebind}) => {
  for (const mode of ['custody', 'marker', 'redirect', 'entity', 'hash', 'blank', 'oversize', 'control']) {
    const archive = JSON.parse(candidate);
    const receipt = archive.app.operations.old;
    if (mode === 'custody') archive.app.revival_go = {};
    else if (mode === 'marker') receipt.result.message = null;
    else if (mode === 'redirect') receipt.result.redirect = '/';
    else if (mode === 'entity') receipt.result.entity_id = 'missing';
    else if (mode === 'hash') receipt.payload_hash = 'invalid';
    else {
      delete archive.app.operations.old;
      archive.app.operations[mode === 'blank' ? ' ' : mode === 'oversize' ? 'x'.repeat(201) : 'bad\0id'] = receipt;
    }
    const bytes = Buffer.from(JSON.stringify(archive)), digest = sha(bytes);
    const pin = 'converted-go/' + '0'.repeat(64) + '/' + digest + '.json';
    await (await bucket('SOURCE')).put(pin, bytes);
    await rebind({SOURCE_KEY: pin, SOURCE_BYTES: String(bytes.length), SOURCE_SHA256: digest});
    const result = await request('/diagnose');
    assert.equal(result.status, 200, mode);
    assert.equal(result.body['an-invalid-exactoperation-receipt'], mode === 'custody' ? 4 : 1, mode);
    assert.deepEqual(await request(), {status: 422, body: {error: 'RESTORE_REFUSED'}}, mode);
    assert.equal((await rows()).length, 0, mode);
  }
  assert.equal((await (await bucket('BACKUPS')).list()).objects.length, 0);
  assert.equal((await (await bucket('SCRY_ASSETS', 'target')).list()).objects.length, 0);
}, 'legacy-operation-identities'));
