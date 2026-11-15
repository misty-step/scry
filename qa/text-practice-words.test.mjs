// A text's practice screens speak of phrases, SPEC's word for a text's Units
// (SPEC § Terms), and leave the earlier notebook to the Menu (SPEC
// § Experience), through the built Worker's native forms and pages. Synthetic
// content only; no browser, account or egress.
import assert from 'node:assert/strict';
import test from 'node:test';
import {randomUUID} from 'node:crypto';
import {createRuntime,internalKey} from './runtime-helper.mjs';

const origin='http://127.0.0.1:8794';
const CANTICLE=['Still waters hold the morning light,\n','and quiet hills receive the rain;\n','the patient reed bends low at night,\n','and rises with the dawn again.\n'];

test('a text\'s Version page, Source study, Quiz and result say phrases and link no concept practice',{timeout:60000},async()=>{
 const name='text-words-'+randomUUID();
 let egress=0;
 const runtime=await createRuntime({name,bindings:{SCRY_ENV:'development',CANONICAL_ORIGIN:origin,SPACE_NAME:name,INTERNAL_KEY:internalKey},egress:async()=>{egress++;throw Error('No egress.');}});
 try{
  const get=async path=>{const response=await runtime.worker.fetch(origin+path);assert.equal(response.status,200,`GET ${path}`);return response.text();};
  const form=(html,action)=>{
   const found=[...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m=>m[1]===action);
   assert.ok(found,`a form for ${action}`);
   return Object.fromEntries([...found[2].matchAll(/<input type="hidden" name="([^"]+)" value="([^"]*)">/g)].map(m=>[m[1],m[2]]));
  };
  const post=async(path,fields)=>{
   const response=await runtime.worker.fetch(origin+path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(fields)});
   const text=await response.clone().text();assert.equal(response.status,303,`${path}: ${text.slice(text.indexOf("<main"),text.indexOf("<main")+600)}`);return response.headers.get('Location');
  };
  const csrf=/name="csrf" value="([^"]+)"/.exec(await get('/settings'))[1];
  const added=await post('/library/version/add',{csrf,operation_id:'text-words-'+randomUUID(),title:'Synthetic Canticle',name:'Synthetic v1',text:CANTICLE.join('')});
  const version=/\/library\/versions\/[0-9a-f]{32}/.exec(added)[0];
  const fresh=await get(version);
  assert.match(fresh,/<summary>Pace: 6 new phrases a day<\/summary>/);assert.match(fresh,/<legend>New phrases each day<\/legend>/);
  await post('/practice/study/start',{...form(fresh,'/practice/study/start'),pace:'steady'});
  const studied=await get(version);
  assert.match(studied,/Pause stops new phrases; due practice remains\./);assert.match(studied,/<legend>Pace · new phrases each day<\/legend>/);
  for(const page of [fresh,studied])assert.doesNotMatch(page,/new Units|New Units/);
  const source=await get('/recall');assert.match(source,/Try from memory/);
  await post('/practice/quiz',form(source,'/practice/quiz'));
  const quiz=await get('/recall');
  await post('/practice/answer',{...form(quiz,'/practice/answer'),answer:'Still waters'});
  const result=await get('/recall');assert.match(result,/data-state="exact-result"/);
  for(const page of [source,quiz,result]){
   assert.doesNotMatch(page,/Concept practice/);assert.doesNotMatch(page,/class="text-link exact-notebook"/);
   assert.match(page,/<a href="\/notebook">Earlier practice<\/a>/,'The Menu still reaches the earlier notebook.');
  }
  assert.equal(egress,0);
 }finally{await runtime.mf.dispose();}
});
