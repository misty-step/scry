// Real workerd/R2; only explicitly labeled opaque synthetic bytes, not Go proof.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {createRequire} from 'node:module';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import test from 'node:test';
const require = createRequire(import.meta.url);
let runtime;
try { runtime = require('miniflare'); }
catch {
  if (!process.env.SCRY_WRANGLER_PACKAGE) throw new Error('Install pinned workerd QA dependencies.');
  runtime = createRequire(resolve(process.env.SCRY_WRANGLER_PACKAGE))('miniflare');
}
const sourcePath = resolve('revival/preserve.mjs');
const script = readFileSync(sourcePath, 'utf8');
const privateBytes = Buffer.from('Synthetic opaque archive canary; not a valid Go backup.\x00\xff');
const sourceKey = 'scry-synthetic-20261015.scry-backup.zip';
const credential = 'synthetic-preservation-only-key-more-than32chars';
const sha256 = value => createHash('sha256').update(value).digest('hex');
const destinationKey = bytes => 'preserved-go/' + sha256(bytes) + '/' + sourceKey;
async function fixture(fn) {
  let egress = 0;
  let bindings = {PRESERVE_KEY: credential, PRESERVE_UNTIL_MS: String(Date.now() + 300_000),
    SOURCE_KEY: sourceKey, SOURCE_ETAG: '0'.repeat(32), SOURCE_BYTES: String(privateBytes.length)};
  const options = () => {
    const v4 = {name: 'scry-preserve-synthetic', modulesRoot: resolve('revival'),
      modules: [{type: 'ESModule', path: sourcePath, contents: script}],
      cf: false, compatibilityDate: '2026-09-10', r2Buckets: ['SOURCE', 'DESTINATION'], bindings,
      outboundService: async () => {egress++; throw new Error('No external egress is authorized.');}};
    return runtime.convertV4MiniflareOptions ? runtime.convertV4MiniflareOptions(v4) : v4;
  };
  const mf = new runtime.Miniflare(options());
  try {
    await mf.ready;
    // Every setOptions invalidates native stubs, including R2 objects. Resolve
    // buckets per operation and retain only plain metadata across a rebind.
    const bucket = name => new Proxy({}, {get: (_, method) => async (...args) => (await mf.getR2Bucket(name))[method](...args)});
    const source = bucket('SOURCE');
    const destination = bucket('DESTINATION');
    const created = await source.put(sourceKey, privateBytes);
    const original = {etag: created.etag, version: created.version};
    bindings = {...bindings, SOURCE_ETAG: original.etag};
    await mf.setOptions(options());
    const rebind = async changes => {bindings = {...bindings, ...changes}; await mf.setOptions(options());};
    const request = async (init = {}, path = '/preserve') => {
      const worker = await mf.getWorker();
      const response = await worker.fetch('https://custody-synthetic.invalid' + path,
        {method: 'POST', ...init, headers: {Authorization: 'Bearer ' + credential, ...init.headers}, redirect: 'manual'});
      const text = await response.text();
      assert.doesNotMatch(text, /Synthetic opaque archive canary/);
      assert.ok(!text.includes(credential));
      assert.match(response.headers.get('Cache-Control'), /no-store/);
      return {status: response.status, body: JSON.parse(text)};
    };
    await fn({source, destination, request, rebind, original});
    assert.equal(egress, 0);
  } finally {await mf.dispose();}
}

test('authorization, route and method rejection cannot read or create records', () => fixture(async ({request, destination, rebind}) => {
  assert.equal((await request({headers: {Authorization: ''}})).status, 403);
  assert.equal((await request({headers: {Authorization: 'Bearer forged'}})).status, 403);
  assert.equal((await request({method: 'GET'})).status, 405);
  assert.equal((await request({}, '/export')).status, 404);
  assert.equal((await request({}, '/preserve?key=another')).status, 404);
  await rebind({PRESERVE_KEY: 'short'});
  assert.equal((await request()).status, 503);
  assert.equal((await destination.list()).objects.length, 0);
}));

test('complete exact-byte preservation, independent readback and safe replay keep source immutable', () => fixture(async ({source, destination, request, original}) => {
  const first = await request();
  assert.equal(first.status, 200);
  assert.deepEqual(first.body, {format: 'scry-preserved-go-archive-v1', verified: true,
    source_key: sourceKey, source_etag: original.etag, bytes: privateBytes.length,
    sha256: sha256(privateBytes), destination_key: destinationKey(privateBytes)});
  const stored = await destination.get(first.body.destination_key);
  assert.deepEqual(Buffer.from(await stored.arrayBuffer()), privateBytes);
  const version = stored.version;
  assert.equal((await request()).status, 200);
  assert.equal((await destination.head(first.body.destination_key)).version, version);
  assert.equal((await source.head(sourceKey)).version, original.version);
  assert.deepEqual(Buffer.from(await (await source.get(sourceKey)).arrayBuffer()), privateBytes);
}));

test('concurrent preservation has one immutable destination version', () => fixture(async ({request, destination}) => {
  const results = await Promise.all([request(), request(), request()]);
  assert.deepEqual(results.map(r => r.status), [200, 200, 200]);
  const objects = (await destination.list()).objects;
  assert.equal(objects.length, 1);
  const version = objects[0].version;
  await request();
  assert.equal((await destination.head(objects[0].key)).version, version);
}));

test('changed or missing pinned source refuses without writing destination', () => fixture(async ({source, destination, request, rebind}) => {
  await source.put(sourceKey, 'changed synthetic bytes');
  assert.deepEqual(await request(), {status: 409, body: {error: 'SOURCE_CHANGED'}});
  await source.delete(sourceKey);
  assert.deepEqual(await request(), {status: 404, body: {error: 'SOURCE_MISSING'}});
  await source.put(sourceKey, privateBytes);
  await rebind({SOURCE_BYTES: String(privateBytes.length + 1)});
  assert.equal((await request()).status, 409);
  assert.equal((await destination.list()).objects.length, 0);
}));

test('a corrupt existing destination is never overwritten or accepted', () => fixture(async ({destination, request}) => {
  const existing = await destination.put(destinationKey(privateBytes), 'corrupt synthetic bytes');
  assert.deepEqual(await request(), {status: 409, body: {error: 'DESTINATION_UNVERIFIED'}});
  assert.equal((await destination.head(destinationKey(privateBytes))).version, existing.version);
  assert.equal(await (await destination.get(destinationKey(privateBytes))).text(), 'corrupt synthetic bytes');
}));

test('same-size destination corruption is detected by full-byte readback', () => fixture(async ({destination, request}) => {
  const corrupt = Buffer.from(privateBytes); corrupt[corrupt.length - 1] ^= 1;
  const existing = await destination.put(destinationKey(privateBytes), corrupt);
  assert.deepEqual(await request(), {status: 409, body: {error: 'DESTINATION_UNVERIFIED'}});
  assert.equal((await destination.head(destinationKey(privateBytes))).version, existing.version);
}));

test('untrusted body, source path, ETag and size pins fail closed', () => fixture(async ({request, rebind, destination, original}) => {
  assert.equal((await request({body: 'arbitrary-key-or-private-text'})).status, 400);
  for (const changes of [{SOURCE_KEY: '../another.scry-backup.zip'}, {SOURCE_KEY: 'other'},
    {SOURCE_ETAG: 'not-an-etag'}, {SOURCE_BYTES: '0'}, {SOURCE_BYTES: '-1'}, {SOURCE_BYTES: 'NaN'},
    {SOURCE_BYTES: String(16 * 1024 * 1024 + 1)}]) {
    await rebind({SOURCE_KEY: sourceKey, SOURCE_ETAG: original.etag, SOURCE_BYTES: String(privateBytes.length), ...changes});
    assert.deepEqual(await request(), {status: 503, body: {error: 'INVALID_SOURCE_PIN'}});
  }
  assert.equal((await destination.list()).objects.length, 0);
}));

test('source metadata larger than the exact bounded pin is refused before copy', () => fixture(async ({source, destination, request, rebind}) => {
  const large = await source.put(sourceKey, new Uint8Array(16 * 1024 * 1024 + 1));
  await rebind({SOURCE_ETAG: large.etag});
  assert.equal((await request()).status, 409);
  assert.equal((await destination.list()).objects.length, 0);
}));

test('immutable capability expires on the actual clock without changing or overwriting either bucket', () => fixture(async ({request, rebind, source, destination, original}) => {
  await rebind({PRESERVE_UNTIL_MS: String(Date.now() + 2000)});
  assert.equal((await request()).status, 200);
  const version = (await destination.head(destinationKey(privateBytes))).version;
  await new Promise(resolve => setTimeout(resolve, 2100));
  assert.deepEqual(await request(), {status: 403, body: {error: 'CAPABILITY_EXPIRED'}});
  assert.equal((await destination.head(destinationKey(privateBytes))).version, version);
  assert.equal((await source.head(sourceKey)).version, original.version);
  await rebind({PRESERVE_UNTIL_MS: 'not-a-time'});
  assert.deepEqual(await request(), {status: 503, body: {error: 'INVALID_SOURCE_PIN'}});
}));
