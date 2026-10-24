import test from 'node:test';
import {mkdir, writeFile} from 'node:fs/promises';
import {buildCli} from './owner-api.mjs';
import {materialJourney} from './materials.mjs';

buildCli();
const journey = await materialJourney();
try {
  for (let i = 0; i < journey.criteria.length; i++) {
    await test(`Concept/Material compatibility ${i + 1}: compiled CLI → built Worker → SQLite/R2`, journey.criteria[i]);
  }
  await mkdir('target', {recursive: true});
  await writeFile('target/materials-runtime.json', JSON.stringify(journey.transcript, null, 2) + '\n');
  console.log('Sanitized Material compatibility transcript: target/materials-runtime.json');
} finally {await journey.close();}
