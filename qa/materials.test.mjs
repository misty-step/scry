import test from 'node:test';
import {mkdir, writeFile, mkdtemp, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {buildCli} from './owner-api.mjs';
import {materialJourney} from './materials.mjs';
import {publishedArtifact, loadFloorPin} from './material-authoring.mjs';

buildCli();
// Keep every Phase A absent-writer/read-only assertion on its exact published
// bytes, while the Phase B writer is qualified separately against current build.
const temp = await mkdtemp(join(tmpdir(), 'scry-floor-qualification-'));
let journey;
try {
  const pin = await loadFloorPin();
  const floorArtifact = await publishedArtifact(pin, join(temp, 'floor'), process.env.SCRY_MATERIAL_FLOOR_WORKER_ZIP);
  journey = await materialJourney({artifact: floorArtifact, cliWriter: true});
  for (let i = 0; i < journey.criteria.length; i++) {
    await test(`Concept/Material compatibility ${i + 1}: compiled CLI → built Worker → SQLite/R2`, journey.criteria[i]);
  }
  await mkdir('target', {recursive: true});
  await writeFile('target/materials-runtime.json', JSON.stringify(journey.transcript, null, 2) + '\n');
  console.log('Sanitized Material compatibility transcript: target/materials-runtime.json');
} finally {await journey?.close(); await rm(temp, {recursive:true, force:true});}
