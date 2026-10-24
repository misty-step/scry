import test from 'node:test';
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {mkdtemp,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {createRuntime,Response,internalKey} from './runtime-helper.mjs';

test('US-019.5 / #375: real SQLite claim, monthly hold, restart and default-off egress',async()=>{
 const now=1790208000000,maximum=1071576;
 const temp=await mkdtemp(join(tmpdir(),'scry-budget-'));
 const name='budget-'+randomUUID(),space='budget-'+randomUUID(),origin='http://127.0.0.1:8796';
 const policy=JSON.stringify({contract:'openrouter-gpt41-mini-text-2026-10-24',monthly_micros:maximum,request_micros:maximum});
 let bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:origin,SPACE_NAME:space,INTERNAL_KEY:internalKey,
  SCRY_TEST_NOW_MS:String(now),OPENROUTER_API_KEY:'synthetic-never-remote-key',OPENROUTER_MODEL:'openai/gpt-4.1-mini'};
 let sends=0,storage,release,started;
 let sending=new Promise(resolve=>{started=resolve;});
 const egress=async request=>{
  assert.equal(request.url,'https://openrouter.ai/api/v1/chat/completions');sends++;
  const payload=await request.json();assert.deepEqual(payload.provider.only,['openai']);assert.equal(payload.provider.allow_fallbacks,false);
  assert.equal(payload.max_tokens,12000);assert.equal(payload.stream,false);assert.equal(payload.tools,undefined);
  const rows=await storage.exec('SELECT key,value FROM records ORDER BY key');
  const spend=rows.filter(r=>r.key.startsWith('spend/')).map(r=>JSON.parse(r.value));
  assert.equal(spend.length,1);assert.equal(spend[0].reserved_micros,maximum);assert.equal(spend[0].cost_micros,null);
  assert.equal(spend[0].quote.policy.monthly_micros,maximum,'The exact durable hold exists before any provider await.');
  started();await new Promise(resolve=>{release=resolve;});
  return new Response('Synthetic missing billing response',{status:200});
 };
 let runtime=await createRuntime({name,bindings,inspect:true,testClock:true,egress,resourcePersistencePath:temp});
 const request=(path,init={})=>runtime.worker.fetch(origin+path,{redirect:'manual',...init});
 const snapshot=async()=>{const response=await request('/export');assert.equal(response.status,200);return(await response.json()).app;};
 const fields=html=>Object.fromEntries([...html.matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
 const capture=async intent=>{const html=await(await request('/create')).text();const response=await request('/create',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields(html),intent})});assert.equal(response.status,303);return response.headers.get('Location').split('/').at(-1);};
 const wait=async predicate=>{const until=Date.now()+8000;do{const app=await snapshot();if(predicate(app))return app;await new Promise(r=>setTimeout(r,50));}while(Date.now()<until);throw Error('Budget transition did not settle.');};
 try {
  storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
  const disabled=await capture('Synthetic legacy key without a budget');
  const off=await wait(app=>app.goals[disabled].status==='failed');assert.equal(sends,0);assert.equal(off.spend.length,0);assert.match(off.jobs[off.goals[disabled].job_id].error,/explicit valid budget/);
  bindings={...bindings,SCRY_AI_BUDGET_POLICY:policy};runtime={...runtime,...await runtime.rebind(bindings)};
  const first=await capture('Synthetic bounded text request');await sending;
  const second=await capture('Concurrent intent must not borrow the held balance');
  const inFlight=await snapshot(),hold=structuredClone(inFlight.spend);
  const archivePath=`/goals/${first}/archive`,page=await(await request(`/goals/${first}`)).text();
  const archiveForm=[...page.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m=>m[1]===archivePath);assert.ok(archiveForm);
  assert.equal((await request(archivePath,{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(fields(archiveForm[2]))})).status,303);
  const noRelease=async()=>{const app=await snapshot();return request('/api/v1/spend/reconcile',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/json'},body:JSON.stringify({csrf:app.csrf,operation_id:'budget-no-release-001',spend_id:hold[0].id,cost_micros:0,reference:'made-up-receipt-001',verified:'provider_billing'})});};
  assert.equal((await noRelease()).status,404);assert.deepEqual((await snapshot()).spend,hold,'Archiving while fetch is delayed cannot release the dispatched obligation.');
  release();release=null;
  const settled=await wait(app=>app.jobs[app.goals[first].job_id].raw_response!==null&&app.goals[second].status==='failed');
  assert.equal(sends,1);assert.equal(settled.spend.length,1);assert.equal(settled.spend[0].cost_micros,null);assert.equal(settled.spend[0].reserved_micros,maximum);
  assert.match(settled.jobs[settled.goals[second].job_id].error,/monthly AI budget/);
  const held=structuredClone(settled.spend);
  await runtime.mf.dispose();
  // A new workerd isolate loads the actual same SQLite/R2 directory. Advance
  // across UTC month rollover without altering any stored budget record.
  bindings={...bindings,SCRY_TEST_NOW_MS:String(now+10*86400000)};
  runtime=await createRuntime({name,bindings,inspect:true,testClock:true,egress,resourcePersistencePath:temp});
  storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
  assert.deepEqual((await snapshot()).spend,held);
  const later=await capture('Unresolved old-month charge still holds the new-month budget');
  await wait(app=>app.goals[later].status==='failed');assert.equal(sends,1);assert.deepEqual((await snapshot()).spend,held);
  assert.equal((await noRelease()).status,404);assert.deepEqual((await snapshot()).spend,held);
  console.log(JSON.stringify({budget_boundary:'PASS',provider_sends:sends,reservation_micros:maximum,restart:true,old_month_hold:true,caller_release:'refused',archived_in_flight_hold:true}));
 } finally {release?.();await runtime.mf.dispose();await rm(temp,{recursive:true,force:true});}
});
