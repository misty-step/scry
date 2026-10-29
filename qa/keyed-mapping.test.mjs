import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdir,writeFile} from 'node:fs/promises';
import {createRequire} from 'node:module';
import {join} from 'node:path';
import {buildCli} from './owner-api.mjs';
import {prepareMappingPredecessor} from './mapping-predecessor.mjs';
import {keyedMappingJourney,keyedMappingWalk,keyedMappingRecovery} from './keyed-mapping.mjs';
import {keyedMappingBoundaryJourney} from './keyed-mapping.mjs';
import {starterWalk,progressWalk} from './nato-starter.mjs';
const output='target/keyed-mapping';await mkdir(output,{recursive:true});
buildCli();
const predecessor=await prepareMappingPredecessor();
const journey=await keyedMappingJourney({predecessorArtifact:predecessor.artifact});
try{for(const [name,run]of journey.criteria)await test(`NATO mapping: ${name}`,run);}
finally{await writeFile(join(output,'runtime.json'),JSON.stringify(journey.transcript,null,2)+'\n');await journey.close();await predecessor.close();}
await test('NATO mapping: protected title and normalized pair identity',async()=>{
 const result=await keyedMappingBoundaryJourney();await writeFile(join(output,'review-boundaries.json'),JSON.stringify(result,null,2)+'\n');
});
await test('NATO mapping: actual mobile native/enhanced browser loop',async()=>{
 const require=createRequire(import.meta.url),playwright=require(process.env.SCRY_CRITICS_PLAYWRIGHT_PATH||'playwright');
 const browser=await playwright.chromium.launch({headless:true,...(process.env.SCRY_CRITICS_CHROMIUM_PATH?{executablePath:process.env.SCRY_CRITICS_CHROMIUM_PATH}:{}),args:['--no-sandbox']});
 try{
  const capture=async(page,name,options={})=>{const path=join(output,name);await page.screenshot({path,fullPage:true,...options});return name;};
  const rows=[...await keyedMappingWalk(browser,capture),...await keyedMappingRecovery(browser,capture),...await starterWalk(browser,capture),...await progressWalk(browser,capture)];
  await writeFile(join(output,'browser.json'),JSON.stringify(rows,null,2)+'\n');
  assert.ok(rows.every(row=>row.status==='pass'),JSON.stringify(rows.filter(row=>row.status!=='pass')));
 }finally{await browser.close();}
});
