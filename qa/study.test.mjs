import test from 'node:test';
import {mkdir,writeFile} from 'node:fs/promises';
import {buildCli} from './owner-api.mjs';
import {studyJourney} from './study.mjs';
buildCli();
// Preserve chronological historical setup in US-016; US-017 retains all of
// its dependent grader criteria in a separate fresh synthetic learning space.
// US-016 still restores the full accumulated old/current history together.
const transcripts=[];
for(const story of ['US-016','US-017']){
 const journey=await studyJourney({seedHistorical:story==='US-016'});
 try {
  const criteria=story==='US-016'?journey.studyCriteria:journey.graderCriteria;
  for(let i=0;i<criteria.length;i++)await test(`${story}.${i+1}: compiled CLI → built Worker → SQLite/R2`,criteria[i]);
 }finally{transcripts.push(...journey.transcript.map(row=>({story,...row})));await mkdir('target',{recursive:true});await writeFile('target/study-cli.json',JSON.stringify(transcripts,null,2)+'\n');await journey.close();}
}
