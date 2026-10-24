import assert from 'node:assert/strict';
import test from 'node:test';
import {execFileSync} from 'node:child_process';
import {mkdir, readFile, writeFile} from 'node:fs/promises';
import {resolve, join} from 'node:path';
import {
  assertHistoricalRetention, assertRestoreEquivalent, parseNativeFields,
  validateArtifactPin, loadFloorPin, materialAuthoringJourney, sha256, TEXT,
} from './material-authoring.mjs';

// These small negative controls exercise proof comparators, not application
// success. SCRY_MATERIAL_AUTHORING_LIGHTWEIGHT=1 explicitly skips all runtime,
// browser, CLI build and artifact work; it must never be reported as that proof.
const rows = () => [
  {key: 'meta', value: JSON.stringify({revision: 3, occurrence: {phase: 'result'}, restored_paused: true})},
  {key: 'owner_tokens/token', value: JSON.stringify({verifier: 'unchanged-verifier', paid: false, last_used_ms: 1})},
  {key: 'concepts/generated', value: '{"id":"generated","notes":[{"text":"Old\\r\\nwording"}]}'},
  {key: 'events/000', value: '{"answer":"immutable"}'},
  {key: 'revival_go/000', value: '"AP8A"'},
];
const changedRows = () => {
  const after = rows(); after[0].value = JSON.stringify({...JSON.parse(after[0].value), revision: 4});
  after[1].value = JSON.stringify({...JSON.parse(after[1].value), last_used_ms: 2});
  after.push({key: 'materials/new', value: '{"id":"new"}'}); return after;
};
await test('Proof comparator permits only enumerated historical metadata and token last-use changes', () => {
  assertHistoricalRetention(rows(), changedRows(), 'token');
});
for (const [name, change] of [
  ['missing history', after => after.splice(3, 1)],
  ['rewritten Note bytes', after => {after[2].value += ' ';}],
  ['rewritten revival custody bytes', after => {after[4].value = '"AAAA"';}],
  ['mutated current occurrence', after => {after[0].value = JSON.stringify({...JSON.parse(after[0].value), occurrence: null});}],
  ['changed token permission', after => {after[1].value = JSON.stringify({...JSON.parse(after[1].value), paid: true});}],
]) {
  await test(`Proof comparator catches ${name}`, () => {
    const after = changedRows(); change(after); assert.throws(() => assertHistoricalRetention(rows(), after, 'token'));
  });
}

const source = () => ({app: {csrf: 'original-csrf', revision: 4, restored_paused: false,
  backup: {completed_ms: 10, key: 'old', sha256: 'old', error: null}, jobs: {}, assessments: {}, goals: {},
  events: [{answer: 'Exact old answer'}], materials: {items: {one: {id: 'one'}}, revisions: {}}}});
const restored = () => ({...source().app, csrf: 'f'.repeat(48), revision: 5, restored_paused: true,
  backup: {completed_ms: null, key: null, sha256: null, error: null}});
await test('Restore comparator allows only fresh CSRF, exact revision increment, pause and backup reset', () => {
  assertRestoreEquivalent(source(), restored(), 'synthetic-test');
  for (const key of ['events', 'materials']) {
    const actual = restored(); actual[key] = {};
    assert.throws(() => assertRestoreEquivalent(source(), actual, 'synthetic-test'));
  }
  const reset = restored(); reset.revision = 1;
  assert.throws(() => assertRestoreEquivalent(source(), reset, 'synthetic-test'));
});
await test('Restore comparator refuses silently normalized active assessments', () => {
  const archived = source(); archived.app.assessments = {pending: {status: 'sent'}};
  const actual = restored(); actual.assessments = {pending: {status: 'unknown'}};
  assert.throws(() => assertRestoreEquivalent(archived, actual, 'synthetic-test'), /settled assessments/);
});

const pin = () => ({release: 'v9.9.9', url: 'https://github.com/misty-step/scry/releases/download/v9.9.9/scry-worker.zip',
  source: 'a'.repeat(40), tree: 'b'.repeat(40), sourceDigest: 'c'.repeat(64), bytes: 123,
  zip: 'd'.repeat(64), js: 'e'.repeat(64), wasm: 'f'.repeat(64)});
await test('Artifact pin requires exact release identity, length, source tree and all hashes', () => {
  assert.deepEqual(validateArtifactPin(pin()), pin());
  for (const key of Object.keys(pin())) {
    const missing = pin(); delete missing[key]; assert.throws(() => validateArtifactPin(missing), key);
  }
});
await test('Artifact pin rejects checkout/branch URLs and unrelated published sources', () => {
  for (const changed of [{release: 'master'}, {url: 'file:///tmp/build.zip'},
    {url: 'https://example.invalid/scry-worker.zip'}, {bytes: 1.5}, {source: '5920bb0'}, {wasm: 'unverified'}]) {
    assert.throws(() => validateArtifactPin({...pin(), ...changed}));
  }
});
await test('Missing published floor pin remains an explicit UNVERIFIED failure', async () => {
  await assert.rejects(loadFloorPin('/tmp/nonexistent-scry-floor-pin-' + process.pid + '.json'), /^Error: UNVERIFIED:/);
});
await test('Native form parser reads its own form, not another operation or an injected text fragment', () => {
  const document = '<form method="post" action="/other"><input name="operation_id" value="other"></form>' +
    '<form method="post" action="/concepts/create"><input type="hidden" name="csrf" value="csrf"><input name="operation_id" value="create">' +
    '<textarea name="text">&lt;input name="operation_id" value="wrong"&gt;</textarea></form>';
  assert.deepEqual(parseNativeFields(document, '/concepts/create'), {csrf: 'csrf', operation_id: 'create'});
  assert.throws(() => parseNativeFields(document, '/missing'));
});
await test('Exact supplied-text fixture distinguishes byte, newline and Unicode normalization changes', () => {
  assert.ok(TEXT.includes('\r\n') && TEXT.includes('\n\n') && TEXT.includes('e\u0301'));
  assert.notEqual(sha256(TEXT), sha256(TEXT.replaceAll('\r\n', '\n')));
  assert.notEqual(sha256(TEXT), sha256(TEXT.normalize('NFC')));
  assert.notEqual(Buffer.byteLength('é'.repeat(101)), 'é'.repeat(101).length);
});

await test('Phase B compiled CLI / built Worker / persisted SQLite / local R2 / pinned rollback integration', {
  skip: process.env.SCRY_MATERIAL_AUTHORING_LIGHTWEIGHT === '1' ? 'Explicit lightweight-only run: runtime, browser and release qualification UNVERIFIED.' : false,
}, async t => {
  // Same gate convention as the existing owner/material suites; never invoked
  // by the lightweight-only fixture tests above.
  const {buildCli} = await import('./owner-api.mjs');
  buildCli();
  const journey = await materialAuthoringJourney();
  const receipt = {proof_subject: 'phase-b-candidate', criteria: [], transcript: journey.transcript,
    limits: ['Local isolated synthetic workerd/SQLite/R2 only.', 'Browser screenshots are recorded by qa/walk, not this runtime suite.',
      'No paid provider, deployed ingress, physical phone, or independent hosted recovery acceptance.']};
  try {
    const artifact = resolve(process.env.SCRY_WORKER_ARTIFACT || 'build');
    receipt.source = process.env.SCRY_SOURCE_META
      ? JSON.parse(await readFile(resolve(process.env.SCRY_SOURCE_META), 'utf8'))
      : {git_revision: execFileSync('git', ['rev-parse', 'HEAD'], {encoding: 'utf8'}).trim(),
        source_state: execFileSync('git', ['status', '--porcelain'], {encoding: 'utf8'}).trim() ? 'worktree' : 'committed'};
    receipt.worker_sha256 = sha256(await readFile(join(artifact, 'index.js')));
    receipt.wasm_sha256 = sha256(await readFile(join(artifact, 'index_bg.wasm')));
    for (const criterion of journey.criteria) {
      await t.test(criterion.name, async () => {
        try {await criterion.run(); receipt.criteria.push({name: criterion.name, status: 'pass'});}
        catch (error) {receipt.criteria.push({name: criterion.name, status: error.message.startsWith('UNVERIFIED:') ? 'unverified' : 'fail', reason: error.message}); throw error;}
      });
    }
  } finally {
    try {
      await mkdir('target', {recursive: true});
      await writeFile('target/material-authoring-runtime.json', JSON.stringify(receipt, null, 2) + '\n');
      console.log('Sanitized Phase B runtime proof: target/material-authoring-runtime.json');
    } finally {await journey.close();}
  }
});
