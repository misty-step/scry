// Exact staged Worker in real workerd/R2 vs independent native SQLite oracle.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {spawnSync} from 'node:child_process';
import test from 'node:test';
import * as runtime from 'miniflare';

const root = resolve(process.env.SCRY_INSPECT_ARTIFACT || 'target/revival-inspect');
const key = 'synthetic-inspection-key-more-than-32-characters';
const sha256 = value => createHash('sha256').update(value).digest('hex');
const fixtures = new Map();
function data(mode = 'valid') {
  if (!fixtures.has(mode)) {
    const result = spawnSync('python3', ['qa/revival-inspect-fixture.py', mode], {maxBuffer: 8 * 1024 * 1024});
    assert.equal(result.status, 0, 'Native synthetic SQLite oracle failed: ' + result.stderr.toString());
    const fixture = JSON.parse(result.stdout);
    fixture.bytes = Buffer.from(fixture.bytes, 'base64');
    fixtures.set(mode, fixture);
  }
  return fixtures.get(mode);
}
async function fixture(mode, fn) {
  let egress = 0;
  const input = data(mode);
  let bindings = {INSPECT_KEY: key, INSPECT_UNTIL_MS: String(Date.now() + 300_000),
    SOURCE_BYTES: String(input.bytes.length), SOURCE_SHA256: sha256(input.bytes),
    SOURCE_KEY: 'preserved-go/' + sha256(input.bytes) + '/scry-synthetic.scry-backup.zip'};
  const options = () => {
    const v4 = {name: 'revival-inspect-synthetic', modulesRoot: root, modules: readdirSync(root)
      .filter(name => /\.(?:js|mjs|wasm|sql)$/.test(name)).sort((a, b) => Number(b === 'inspect.js') - Number(a === 'inspect.js')).map(name => ({path: resolve(root, name),
        type: name.endsWith('.wasm') ? 'CompiledWasm' : name.endsWith('.sql') ? 'Text' : 'ESModule'})),
      cf: false, compatibilityDate: '2026-09-10', r2Buckets: ['SOURCE'], bindings,
      outboundService: async () => {egress++; throw new Error('External egress is not authorized.');}};
    return runtime.convertV4MiniflareOptions ? runtime.convertV4MiniflareOptions(v4) : v4;
  };
  const mf = new runtime.Miniflare(options());
  try {
    await mf.ready;
    const bucket = () => mf.getR2Bucket('SOURCE');
    const source = await (await bucket()).put(bindings.SOURCE_KEY, input.bytes);
    const sourceVersion = source.version;
    const sourceKey = bindings.SOURCE_KEY;
    const rebind = async changes => {bindings = {...bindings, ...changes}; await mf.setOptions(options());};
    const request = async (init = {}, path = '/inspect') => {
      const response = await (await mf.getWorker()).fetch('https://synthetic.invalid' + path,
        {method: 'POST', ...init, headers: {Authorization: 'Bearer ' + key, ...init.headers}, redirect: 'manual'});
      const text = await response.text();
      assert.doesNotMatch(text, /SYNTHETIC_PRIVATE|SYNTHETIC_SECRET|SYNTHETIC_PHOTO|fixture-source|fixture-goal|fixture-operation/);
      assert.ok(!text.includes(key));
      assert.match(response.headers.get('Cache-Control'), /no-store/);
      return {status: response.status, body: JSON.parse(text)};
    };
    await fn({request, rebind, input, bucket, sourceKey});
    const stored = await (await bucket()).get(sourceKey);
    if (stored) {
      assert.equal(stored.version, sourceVersion);
      assert.deepEqual(Buffer.from(await stored.arrayBuffer()), input.bytes);
    }
    assert.equal(egress, 0);
  } finally {await mf.dispose();}
}

test('native full-schema oracle matches every table, int64, embedded NUL, FTS5, scheduler, receipt and media byte', () => fixture('valid', async ({request, input}) => {
  const first = await request();
  assert.equal(first.status, 200);
  assert.equal(first.body.verified, true);
  assert.equal(first.body.format, 'scry-go-inventory-v1');
  assert.equal(first.body.archive_sha256, sha256(input.bytes));
  assert.equal(first.body.database_sha256, input.database_sha256);
  assert.equal(first.body.schema_sha256, input.schema_sha256);
  assert.deepEqual(first.body.tables, input.tables);
  assert.deepEqual(await request(), first);
}));

test('authorization, route, method and body deny before archive access', () => fixture('valid', async ({request, rebind}) => {
  assert.equal((await request({headers: {Authorization: ''}})).status, 403);
  assert.equal((await request({headers: {Authorization: 'Bearer forged'}})).status, 403);
  assert.equal((await request({method: 'GET'})).status, 405);
  assert.equal((await request({}, '/export')).status, 404);
  assert.equal((await request({}, '/inspect?sql=SELECT')).status, 404);
  assert.equal((await request({body: 'private-input'})).status, 400);
  await rebind({INSPECT_KEY: 'short'});
  assert.equal((await request()).status, 503);
}));

for (const mode of ['unexpected', 'missing', 'foreign-key', 'application', 'version', 'schema',
  'corrupt', 'manifest-digest', 'duplicate', 'extra', 'compressed']) {
  test('rejects ' + mode + ' without exposing diagnostics or changing source', () => fixture(mode, async ({request}) => {
    assert.deepEqual(await request(), {status: 422, body: {error: 'INSPECTION_FAILED'}});
  }));
}

for (const [mode, name] of [['changed-text', 'sources'], ['changed-media', 'capture_images'],
  ['changed-schedule', 'schedules'], ['changed-bigint', 'sources']]) {
  test('canonical digest detects ' + mode + ' without returning the changed value', () => fixture(mode, async ({request, input}) => {
    const result = await request();
    assert.equal(result.status, 200);
    assert.deepEqual(result.body.tables, input.tables);
    assert.notEqual(result.body.tables.find(t => t.name === name).sha256, data().tables.find(t => t.name === name).sha256);
  }));
}

test('pins, missing source and exact archive hash fail closed', () => fixture('valid', async ({request, rebind, input, bucket, sourceKey}) => {
  for (const changes of [{SOURCE_BYTES: '0'}, {SOURCE_BYTES: String(16 * 1024 * 1024 + 1)},
    {SOURCE_KEY: 'arbitrary'}, {SOURCE_SHA256: 'bad'}, {INSPECT_UNTIL_MS: 'NaN'}]) {
    await rebind({...{SOURCE_BYTES: String(input.bytes.length), SOURCE_KEY: sourceKey,
      SOURCE_SHA256: sha256(input.bytes), INSPECT_UNTIL_MS: String(Date.now() + 300_000)}, ...changes});
    assert.equal((await request()).status, 503);
  }
  await rebind({SOURCE_BYTES: String(input.bytes.length + 1), INSPECT_UNTIL_MS: String(Date.now() + 300_000)});
  assert.deepEqual(await request(), {status: 409, body: {error: 'SOURCE_CHANGED'}});
  await rebind({SOURCE_BYTES: String(input.bytes.length), SOURCE_KEY: 'preserved-go/' + '0'.repeat(64) + '/scry-synthetic.scry-backup.zip', SOURCE_SHA256: '0'.repeat(64)});
  assert.equal((await request()).status, 404);
  await (await bucket()).put('preserved-go/' + '0'.repeat(64) + '/scry-synthetic.scry-backup.zip', input.bytes);
  assert.deepEqual(await request(), {status: 409, body: {error: 'SOURCE_CHANGED'}});
}));

test('expired immutable capability denies while the private source remains retained', () => fixture('valid', async ({request, rebind}) => {
  await rebind({INSPECT_UNTIL_MS: String(Date.now() - 1)});
  assert.deepEqual(await request(), {status: 403, body: {error: 'CAPABILITY_EXPIRED'}});
}));
