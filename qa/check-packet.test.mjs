import assert from 'node:assert/strict';
import {test} from 'node:test';
import {mkdtemp, cp, readFile, writeFile, rm} from 'node:fs/promises';
import {join} from 'node:path';
import {createHash} from 'node:crypto';
import {checkPacket} from './check-packet.mjs';

// Use the real run packet. These checks protect fail-closed receipt admission.
const source = process.env.QA_TEST_PACKET;
if (!source) throw Error('QA_TEST_PACKET must point at a freshly produced packet');
const original = JSON.parse(await readFile(join(source, 'packet.json')));
const specHash = createHash('sha256').update(await readFile(join(source, 'frozen-spec.json'))).digest('hex');
test('the real complete packet is admitted', async () => {
  assert.equal((await checkPacket(source, original.candidate, specHash)).verdict, 'PASS');
});
for (const [name, mutate] of [
  ['removed diff-required story', p => {p.required_stories = ['US-008']; p.selection = []; p.rows = p.rows.filter(r => r.story === 'US-008');}],
  ['missing criterion', p => p.rows.pop()],
  ['skipped criterion', p => p.rows[0].status = 'SKIPPED'],
  ['blocked criterion', p => p.rows[0].status = 'BLOCKED'],
  ['failed criterion', p => p.rows[0].status = 'FAIL'],
  ['missing video', p => p.rows[0].evidence = p.rows[0].evidence.filter(r => !r.startsWith('video/'))],
  ['missing screenshot', p => p.rows[0].evidence = p.rows[0].evidence.filter(r => !r.startsWith('screens/'))],
  ['missing action', p => p.rows[0].action = []],
  ['missing observed behavior', p => p.rows[0].observed = []],
  ['builder self-approval', p => p.verifier.builder = true],
]) test(`${name} cannot manufacture green`, async () => {
  const directory = await mkdtemp('/tmp/scry-packet-check-');
  try {
    await cp(source, directory, {recursive: true});
    const packet = structuredClone(original); mutate(packet);
    await writeFile(join(directory, 'packet.json'), JSON.stringify(packet));
    await assert.rejects(checkPacket(directory, original.candidate, specHash));
  } finally {await rm(directory, {recursive: true});}
});
test('a new candidate invalidates an old passing packet', async () => {
  await assert.rejects(checkPacket(source, 'a'.repeat(40), specHash), /new code invalidates/);
});
test('changing the frozen spec invalidates a passing packet', async () => {
  await assert.rejects(checkPacket(source, original.candidate, 'a'.repeat(64)), /frozen spec changed/);
});
