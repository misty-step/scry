import test from 'node:test';
import {ownerJourney,buildCli} from './owner-api.mjs';
buildCli();
const journey=await ownerJourney();
try {
  for(let i=0;i<journey.criteria.length;i++) {
    await test(`US-014.${i+1}: built Worker and Owner token / CLI journey`,journey.criteria[i]);
  }
  console.log('Sanitized CLI transcript: '+JSON.stringify(journey.transcript));
} finally {await journey.close();}
