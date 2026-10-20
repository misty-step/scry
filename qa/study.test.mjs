import test from 'node:test';
import {mkdir,writeFile} from 'node:fs/promises';
import {buildCli} from './owner-api.mjs';
import {studyJourney} from './study.mjs';
buildCli();
const journey=await studyJourney();
try {
 for(const [story,criteria]of [['US-016',journey.studyCriteria],['US-017',journey.graderCriteria]])
  for(let i=0;i<criteria.length;i++)await test(`${story}.${i+1}: compiled CLI → built Worker → SQLite/R2`,criteria[i]);
}finally{await mkdir('target',{recursive:true});await writeFile('target/study-cli.json',JSON.stringify(journey.transcript,null,2)+'\n');await journey.close();}
