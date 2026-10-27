import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createSyntheticRuntime} from './runtime-helper.mjs';

test('retired generation POSTs refuse before jobs, photos, receipts or spend; native imports remain available',async()=>{
 const runtime=await createSyntheticRuntime({name:'generation-retirement'});
 try {
  const request=(path,options={})=>runtime.worker.fetch(runtime.url+path,{redirect:'manual',...options});
  const read=async()=>{const response=await request('/export');assert.equal(response.status,200);return response.json();};
  const before=await read();
  const fields={csrf:before.app.csrf,operation_id:'retired-generation-attempt',intent:'Unsent synthetic material'};
  for(const path of ['/create','/add','/goals/historical/retry','/goals/historical/refine','/questions/historical/fix']) {
   const response=await request(path,{method:'POST',headers:{'X-Scry-Test-Origin':runtime.url,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(fields)});
   assert.equal(response.status,410,`${path} must be retired, not queued`);
   assert.match(await response.text(),/Native content generation is retired/);
   assert.deepEqual((await read()).app,before.app);
  }
  for(const path of ['/review/self','/review/override','/practice/correct']) {
   const response=await request(path,{method:'POST',headers:{'X-Scry-Test-Origin':runtime.url,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,correct:'true',verdict:'Good'})});
   assert.equal(response.status,410);assert.match(await response.text(),/Self-grading is retired/);
   assert.deepEqual((await read()).app,before.app);
  }
  const data=new FormData();for(const [k,v] of Object.entries(fields))data.set(k,v);data.set('photo',new Blob([new Uint8Array([137,80,78,71,13,10,26,10])],{type:'image/png'}),'synthetic.png');
  const encoded=new globalThis.Request(runtime.url,{method:'POST',body:data});
  const upload=await request('/create',{method:'POST',headers:{'X-Scry-Test-Origin':runtime.url,'Content-Type':encoded.headers.get('Content-Type')},body:Buffer.from(await encoded.arrayBuffer())});assert.equal(upload.status,410);
  assert.deepEqual((await read()).app,before.app);assert.deepEqual((await read()).photos,[]);
  const bucket=await runtime.mf.getR2Bucket('SCRY_ASSETS');assert.deepEqual((await bucket.list()).objects,[]);
  const minted=await request('/tokens/create',{method:'POST',headers:{'X-Scry-Test-Origin':runtime.url,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,operation_id:'retirement-synthetic-token',name:'Synthetic retirement proof; unpaid'})});
  assert.equal(minted.status,200);const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())?.[1];assert.ok(secret);
  const apiBefore=(await read()).app;
  const api=await request('/api/v1/create',{method:'POST',headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'},body:JSON.stringify(fields)});
  assert.equal(api.status,410);assert.match(await api.text(),/retired/);assert.deepEqual((await read()).app,apiBefore);
  const correction=await request('/api/v1/correct',{method:'POST',headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'},body:JSON.stringify({...fields,verdict:'Good'})});
  assert.equal(correction.status,410);assert.match(await correction.text(),/Self-grading is retired/);assert.deepEqual((await read()).app,apiBefore);
  let operation=0;
  const mutate=async(path,data={})=>{
   const response=await request(path,{method:'POST',headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'},body:JSON.stringify({operation_id:`native-retirement-proof-${++operation}`, ...data})});
   assert.equal(response.status,200,path);return response.json();
  };
  const manifest=JSON.parse(readFileSync(new URL('../examples/nato-alphabet.json',import.meta.url),'utf8'));
  const imported=await mutate('/api/v1/library/mappings',{data:manifest});
  await mutate('/api/v1/learn',{version_id:imported.result.entity_id});
  const prompt=(await mutate('/api/v1/next')).practice.prompt;
  const answer=(await mutate('/api/v1/answer',{prompt_id:prompt.id,answer:'Alfa'})).practice.attempt;
  assert.equal(answer.grade.verdict,'Good');assert.equal(answer.assisted,false);
  const graded=(await read()).app;assert.deepEqual(graded.spend,[]);assert.deepEqual(graded.jobs,{});assert.deepEqual(graded.goals,{});
  const home=await(await request('/')).text();assert.match(home,/data-state="review-home"/);assert.match(home,/\/library#nato-starter/);assert.doesNotMatch(home,/href="\/create"/);
 }finally{await runtime.mf.dispose();}
});
