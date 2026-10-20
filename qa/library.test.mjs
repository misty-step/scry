import test from 'node:test';
import {mkdir,writeFile} from 'node:fs/promises';
import {buildCli} from './owner-api.mjs';
import {libraryJourney} from './library.mjs';
buildCli();
const journey=await libraryJourney();
try {
 for(let i=0;i<journey.criteria.length;i++) await test(`US-015.${i+1}: private Library CLI → built Worker → SQLite/R2`,journey.criteria[i]);
 await mkdir('target',{recursive:true});
 await writeFile('target/library-cli.json',JSON.stringify(journey.transcript,null,2)+'\n');
 console.log('Sanitized Library CLI transcript: target/library-cli.json');
} finally { await journey.close(); }
