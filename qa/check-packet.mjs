#!/usr/bin/env node
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {deriveStories} from './executor-contract.mjs';
import {createHash} from 'node:crypto';
import {readFile} from 'node:fs/promises';
import {join, resolve} from 'node:path';

export async function checkPacket(directory, expectedCandidate, expectedSpecHash) {
  assert.match(expectedCandidate || '', /^[a-f0-9]{40}$/, 'an exact current candidate is required');
  assert.match(expectedSpecHash || '', /^[a-f0-9]{64}$/, 'the independently frozen spec checksum is required');
  const output = resolve(directory);
  const packet = JSON.parse(await readFile(join(output, 'packet.json')));
  assert.equal(packet.schema, 'scry-rule-b-qa/1');
  assert.equal(packet.candidate, expectedCandidate, 'new code invalidates the QA verdict');
  assert.equal(packet.frozen_spec_sha256, expectedSpecHash, 'the frozen spec changed');
  assert.equal(packet.verdict, 'PASS', 'QA did not pass');
  assert.equal(packet.verifier?.role, 'independent-qa');
  assert.equal(packet.verifier?.builder, false);
  assert.ok(packet.verifier.id);
  assert.equal(packet.browser.channel, 'chrome');
  assert.ok(packet.required_stories.length, 'no required stories recorded');
  const hash = bytes => createHash('sha256').update(bytes).digest('hex');
  const specBytes = await readFile(join(output, 'frozen-spec.json'));
  assert.equal(hash(specBytes), expectedSpecHash);
  const spec = JSON.parse(specBytes), manifest = JSON.parse(await readFile(join(output, 'artifact.json')));
  assert.equal(manifest.candidate, packet.candidate);
  assert.equal(manifest.tree, packet.tree);
  assert.equal(manifest.files.find(f => f.path === 'scry').sha256, packet.artifact_sha256);
  for (const id of spec.required_stories) assert.ok(packet.required_stories.includes(id), `missing required story ${id}`);
  for (const selection of packet.selection) for (const id of selection.stories) assert.ok(packet.required_stories.includes(id), `missing diff-required story ${id}`);
  const changes = execFileSync('git', ['diff', '--name-status', spec.base, expectedCandidate], {encoding: 'utf8'}).trim().split('\n').filter(Boolean);
  const derived = deriveStories(spec, changes);
  assert.deepEqual(packet.required_stories, derived.required_stories, 'diff-required story set changed');
  assert.deepEqual(packet.selection, derived.selection, 'diff selection changed');
  const refs = new Set();
  for (const evidence of packet.evidence) {
    assert.ok(!refs.has(evidence.path), `duplicate evidence ${evidence.path}`);
    assert.ok(/^(screens|video|logs)\/[a-zA-Z0-9@_.-]+$/.test(evidence.path) || ['artifact.json','frozen-spec.json','candidate.diff'].includes(evidence.path), 'unsafe evidence path');
    refs.add(evidence.path);
    const bytes = await readFile(join(output, evidence.path));
    assert.ok(bytes.length, `empty evidence ${evidence.path}`);
    assert.equal(hash(bytes), evidence.sha256, `evidence changed: ${evidence.path}`);
  }
  for (const id of packet.required_stories) {
    assert.ok(spec.stories[id]?.length, `no frozen criteria for ${id}`);
    for (let i = 0; i < spec.stories[id].length; i++) {
      const rows = packet.rows.filter(r => r.story === id && r.criterion === i + 1);
      assert.equal(rows.length, 1, `missing or repeated ${id}/${i + 1}`);
      const row = rows[0];
      assert.equal(row.status, 'PASS', `failed, skipped, or blocked ${id}/${i + 1}`);
      assert.equal(row.contract, spec.stories[id][i], 'criterion changed');
      assert.ok(row.action.length && row.expected && row.observed.length, 'missing behavior evidence');
      assert.ok(row.evidence.some(ref => ref.startsWith('screens/')), 'missing screenshot');
      assert.ok(row.evidence.some(ref => ref.startsWith('video/')), 'missing video');
      for (const ref of row.evidence) assert.ok(refs.has(ref), `missing evidence ${ref}`);
    }
  }
  assert.ok(packet.rows.every(row => row.status === 'PASS'), 'required path did not pass');
  return packet;
}
if (process.argv[1] === new URL(import.meta.url).pathname) {
  try {await checkPacket(...process.argv.slice(2)); console.log('PASS: complete packet for the expected candidate and frozen spec');}
  catch (error) {console.error(`FAIL: ${error.message}`); process.exitCode = 1;}
}
