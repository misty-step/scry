import test from 'node:test';
import {mkdir, readFile, writeFile, mkdtemp, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {execFileSync} from 'node:child_process';
import {join, resolve} from 'node:path';
import {buildCli} from './owner-api.mjs';
import {materialJourney} from './materials.mjs';
import {publishedArtifact, loadFloorPin} from './material-authoring.mjs';

buildCli();
// Keep every Phase A absent-writer/read-only assertion on its exact published
// bytes, while the Phase B writer is qualified separately against current build.
const temp = await mkdtemp(join(tmpdir(), 'scry-floor-qualification-'));
let journey;
const receipt = {
  format: 'scry-published-material-floor-proof-v1',
  proof_subject: 'published-phase-a-floor', status: 'running', floor: null,
  driver_source: process.env.SCRY_SOURCE_META
    ? JSON.parse(await readFile(resolve(process.env.SCRY_SOURCE_META), 'utf8'))
    : {git_revision: execFileSync('git', ['rev-parse', 'HEAD'], {encoding: 'utf8'}).trim()},
  cli_role: 'Phase B candidate CLI sends real commands to the pinned floor; it must not enable the floor writer.',
  criteria: [], transcript: [],
};
try {
  const pin = await loadFloorPin();
  receipt.floor = pin;
  const floorArtifact = await publishedArtifact(pin, join(temp, 'floor'), process.env.SCRY_MATERIAL_FLOOR_WORKER_ZIP);
  journey = await materialJourney({artifact: floorArtifact, cliWriter: true});
  receipt.transcript = journey.transcript;
  for (let i = 0; i < journey.criteria.length; i++) {
    await test(`Published Phase A floor compatibility ${i + 1}: candidate CLI → pinned floor Worker → SQLite/R2`, async () => {
      try {await journey.criteria[i](); receipt.criteria.push({criterion: i + 1, status: 'pass'});}
      catch (error) {receipt.criteria.push({criterion: i + 1, status: 'fail', reason: error.message}); throw error;}
    });
  }
  receipt.status = receipt.criteria.every(item => item.status === 'pass') ? 'pass' : 'fail';
} catch (error) {
  receipt.status = error.message.startsWith('UNVERIFIED:') ? 'unverified' : 'fail';
  receipt.reason = error.message;
  throw error;
} finally {
  try {
    await mkdir('target', {recursive: true});
    await writeFile('target/materials-runtime.json', JSON.stringify(receipt, null, 2) + '\n');
    console.log('Sanitized published-floor compatibility transcript: target/materials-runtime.json');
  } finally {await journey?.close(); await rm(temp, {recursive:true, force:true});}
}
