// Supplied canonical content through the compiled CLI, built Worker and its
// actual SQLite/local-R2 boundary. No row writes, generation or external egress.
import assert from 'node:assert/strict';
import {createHash,randomUUID} from 'node:crypto';
import {spawn} from 'node:child_process';
import {mkdtemp,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {createRuntime,internalKey} from './runtime-helper.mjs';
import {feedbackArrival,assertFeedbackArrival} from './feedback-arrival.mjs';
const DAY=86400000;
// Always later than initialization's wall clock, including the afternoon UTC.
// All changes below use this explicit fixture clock, not sleeps or row edits.
const START=(Math.floor(Date.now()/DAY)+1)*DAY+12*60*60*1000;
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
const manifest=resolve('examples/nato-alphabet.json');
// A successful authenticated API replay saves the transport CAS revision. It
// must not duplicate or change any domain row, receipt, schedule or other meta.
function successfulReplay(before,after){
 assert.deepEqual(after.filter(row=>row.key!=='meta'),before.filter(row=>row.key!=='meta'));
 const previous=JSON.parse(before.find(row=>row.key==='meta').value),current=JSON.parse(after.find(row=>row.key==='meta').value);
 assert.equal(current.revision,previous.revision+1);
 assert.deepEqual({...current,revision:previous.revision},previous);
 return {before_revision:previous.revision,after_revision:current.revision,domain_and_receipts_unchanged:true};
}
export async function keyedMappingFixture({capturePredecessor=false}={}){
 const temp=await mkdtemp(join(tmpdir(),'scry-keyed-mapping-'));
 const name='mapping-'+randomUUID(),space=name,config=join(temp,'login.json');
 const base={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:space,INTERNAL_KEY:internalKey};
 let at=START,egress=0,runtime,origin,storage,secret='';
 const transcript=[];
 const start=async()=>{
  runtime=await createRuntime({port:0,name,inspect:true,testClock:true,resourcePersistencePath:join(temp,'state'),bindings:{...base,SCRY_TEST_NOW_MS:String(at)},egress:async()=>{egress++;throw Error('No external egress in mapping proof.');}});
  origin=(await runtime.mf.ready).origin;
  runtime={...runtime,...await runtime.rebind({...base,CANONICAL_ORIGIN:origin,SCRY_TEST_NOW_MS:String(at)})};
  storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
 };
 try{
  await start();
  const rows=()=>storage.exec('SELECT key,value FROM records ORDER BY key');
  const record=async key=>JSON.parse((await storage.exec('SELECT value FROM records WHERE key=?',key))[0].value);
  const api=(path,payload)=>runtime.worker.fetch(origin+path,{...(payload?{method:'POST',body:JSON.stringify(payload)}:{}),headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'}});
  const cli=(args,input='',login=config)=>new Promise((done,reject)=>{
   const child=spawn(resolve(process.env.SCRY_STUDY_CLI||'target/debug/scry'),[...args,'--json','--config',login],{env:{PATH:process.env.PATH,HOME:temp},stdio:['pipe','pipe','pipe']});let out='',err='';
   child.stdout.on('data',s=>out+=s);child.stderr.on('data',s=>err+=s);child.on('error',reject);child.on('close',code=>{try{assert.ok(!out.includes(secret)&&!err.includes(secret));const value=JSON.parse(out);transcript.push({command:['scry',...args.map(s=>s.replaceAll(temp,'<fixture>')), '--json'].join(' '),exit:code,output_sha256:sha(out)});done({code,value});}catch(e){reject(e);}});child.stdin.end(input);
  });
  const ok=async(args,input='',login=config)=>{const r=await cli(args,input,login);assert.equal(r.code,0,JSON.stringify(r.value));return r.value;};
  const settings=await(await runtime.worker.fetch(origin+'/settings')).text();
  const tokenForm=/<form[^>]*action="\/tokens\/create"[^>]*>([\s\S]*?)<\/form>/.exec(settings);assert.ok(tokenForm);
  const fields=Object.fromEntries([...tokenForm[1].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
  const minted=await runtime.worker.fetch(origin+'/tokens/create',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,name:'Synthetic mapping proof; unpaid'})});assert.equal(minted.status,200);
  secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())?.[1];assert.ok(secret);
  await ok(['login','--url',origin,'--token-stdin'],secret+'\n');
  const archives={};
  if(capturePredecessor){
   const path=join(temp,'predecessor-control.txt');await writeFile(path,'Synthetic predecessor control.\n');
   const version=(await ok(['version','add','--title','Predecessor positive control','--kind','text','--text-file',path])).result.entity_id;
   const study=(await ok(['learn',version])).practice.study.id,prompt=(await ok(['next'])).practice.prompt;
   await ok(['answer',prompt.id,'--text-stdin'],'Synthetic predecessor control.');await ok(['study','drop',study]);
   const response=await api('/api/v1/export');assert.equal(response.status,200);archives.baseline=Buffer.from(await response.arrayBuffer());
  }
  const imported=await ok(['mapping','add','--file',manifest,'--operation-id','mapping-fixture-import-operation']);
  const vid=imported.result.entity_id,shown=await ok(['version','show',vid]);
  if(capturePredecessor){const response=await api('/api/v1/export');assert.equal(response.status,200);archives.mappingLibrary=Buffer.from(await response.arrayBuffer());}
  const sid=(await ok(['learn',vid,'--pace','intense'])).practice.study.id;
  const r2=async()=>{
   const result={};
   for(const bucketName of ['SCRY_ASSETS','SCRY_BACKUPS']){const bucket=await runtime.mf.getR2Bucket(bucketName);const objects=await bucket.list();result[bucketName]=[];for(const object of objects.objects){const bytes=Buffer.from(await(await bucket.get(object.key)).arrayBuffer());result[bucketName].push([object.key,sha(bytes)]);}}
   return result;
  };
  const clock=async value=>{at=value;runtime={...runtime,...await runtime.rebind({...base,CANONICAL_ORIGIN:origin,SCRY_TEST_NOW_MS:String(at)})};storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});};
  const restore=async()=>{
   const before=(await ok(['export'])).app;
   const ns=await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE');
   const backed=await ns.get(ns.idFromName(space)).fetch('https://internal.invalid/__internal/daily',{method:'POST',headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});assert.equal(backed.status,200);
   const bucket=await runtime.mf.getR2Bucket('SCRY_BACKUPS'),list=await bucket.list({prefix:`snapshots/${space}/`});assert.ok(list.objects.length);
   const bytes=Buffer.from(await(await bucket.get(list.objects.at(-1).key)).arrayBuffer());const archive=JSON.parse(bytes);
   assert.deepEqual(archive.app.library,before.library);assert.deepEqual(archive.app.practice,before.practice);
   const restoreToken='synthetic-mapping-restore-capability-32chars',cloneName='mapping-restore-'+randomUUID();
   const bindings={...base,SCRY_ENV:'restore',SPACE_NAME:cloneName,RESTORE_TOKEN:restoreToken,SCRY_TEST_NOW_MS:String(at)};
   let clone=await createRuntime({port:0,name:cloneName,inspect:true,testClock:true,bindings});
   try{
    const url=(await clone.mf.ready).origin;clone={...clone,...await clone.rebind({...bindings,CANONICAL_ORIGIN:url})};
    const request=()=>clone.worker.fetch(url+'/operator/restore',{method:'POST',headers:{'x-scry-restore-token':restoreToken,'x-scry-sha256':sha(bytes)},body:bytes});
    assert.equal((await request()).status,200);
    const sql=await clone.mf.unsafeGetDurableObjectStorage(cloneName,'LearningSpace',{name:cloneName});const restoredRows=()=>sql.exec('SELECT key,value FROM records ORDER BY key');
    const occupied=await restoredRows();assert.equal((await request()).status,409);assert.deepEqual(await restoredRows(),occupied);
    clone={...clone,...await clone.rebind({...bindings,SCRY_ENV:'development',CANONICAL_ORIGIN:url})};
    const login=join(temp,'restored-login.json');await ok(['login','--url',url,'--token-stdin'],secret+'\n',login);
    const after=(await ok(['export'],'',login)).app;
    assert.deepEqual(after.library,before.library);assert.deepEqual(after.practice,before.practice);
    const write=await clone.worker.fetch(url+'/api/v1/settings',{method:'POST',headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'},body:JSON.stringify({operation_id:'mapping-restore-unrelated-write',pace:'light'})});assert.equal(write.status,200);
    const retained=(await ok(['export'],'',login)).app;assert.deepEqual(retained.library,before.library);assert.deepEqual(retained.practice,before.practice);
    transcript.push({observation:'Complete local R2 archive, fresh restore, occupied-target refusal, compiled CLI and first unrelated write retain immutable mapping history',archive_sha256:sha(bytes),archive_bytes:bytes.length});
   }finally{await clone.mf.dispose();}
  };
  return {get origin(){return origin;},get worker(){return runtime.worker;},get at(){return at;},get egress(){return egress;},cli,ok,api,rows,record,r2,clock,restore,vid,sid,shown,transcript,archives,
   restart:async()=>{await runtime.mf.dispose();await start();await ok(['login','--url',origin,'--token-stdin'],secret+'\n');},
   close:async()=>{await runtime.mf.dispose();await rm(temp,{recursive:true,force:true});}};
 }catch(error){await runtime?.mf.dispose();await rm(temp,{recursive:true,force:true});throw error;}
}

export async function keyedMappingJourney({predecessorArtifact}={}){
 const f=await keyedMappingFixture({capturePredecessor:true}),transcript=f.transcript;
 let current,cards=new Map(),original;
 const next=async(extra=[])=>{current=(await f.ok(['next',...(current?.attempt_id?['--after',current.prompt.id]:[]),...extra])).practice;return current;};
 const answer=async(text,operation)=>{const result=await f.ok(['answer',current.prompt.id,'--text-stdin',...(operation?['--operation-id',operation]:[])],text);current.attempt_id=result.practice.attempt.id;return result.practice.attempt;};
 const practice=async(key,text)=>{await next(['--card',cards.get(key)]);return answer(text);};
 const refuse=async(path,payload,status)=>{const before=await f.rows(),objects=await f.r2();const r=await f.api(path,{operation_id:randomUUID(),...payload});assert.equal(r.status,status);assert.deepEqual(await f.rows(),before);assert.deepEqual(await f.r2(),objects);};
 const criteria=[
  ['26 canonical externally supplied pairs and idempotent import',async()=>{
   const v=f.shown.version,units=v.sections.flatMap(s=>s.units);assert.equal(units.length,26);assert.equal(units.map(u=>u.text.split('\t')[0]).join(''),'ABCDEFGHIJKLMNOPQRSTUVWXYZ');
   assert.equal(v.text,units.map(u=>u.text).join(''));assert.equal(v.kind,'keyed_mapping');assert.equal(v.provenance,'supplied');assert.equal(v.spelling_strict,true);
   const before=await f.rows(),replay=await f.ok(['mapping','add','--file',manifest,'--operation-id','mapping-fixture-import-operation']);assert.equal(replay.replayed,true);assert.equal(replay.result.entity_id,f.vid);transcript.push({observation:'Import receipt replay',...successfulReplay(before,await f.rows())});
   assert.equal((await f.ok(['mapping','add','--file',manifest])).result.entity_id,f.vid);assert.equal((await f.ok(['learn',f.vid])).practice.study.id,f.sid);
   await next();assert.equal(current.prompt.cue,'A');assert.equal(current.prompt.assisted,false);assert.ok(!JSON.stringify(current).includes('Alfa'));
   const snapshot=await f.rows();assert.equal((await f.api('/api/v1/library/versions/'+f.vid)).status,409);assert.deepEqual(await f.rows(),snapshot);
   original=await answer('Alpha');assert.equal(original.grade.verdict,'Again');assert.equal(original.effects[0].scheduled,true);assert.equal(original.assisted,false);assert.equal(original.grade.authority.name,'exact-v2');assert.ok(original.grade.authority.grader_version.endsWith(';mapping=1'));assert.equal(original.grade.diff[0].expected.text,'Alfa');
   cards.set('A',original.effects[0].card_id);
  }],
  ['one forward Card per pair and exact native grading',async()=>{
   // Introduce via actual next/answer operations under the rolling Pace limit.
   // Advancing the fixture clock is deliberate; no state is directly edited.
   for(let count=0;cards.size<26&&count<200;count++){
    await next();if(!current.prompt){await f.clock(f.at+DAY+1);continue;}
    const index=current.prompt.scope.position-1,unit=f.shown.version.sections.flatMap(s=>s.units)[index];const [key,value]=unit.text.trimEnd().split('\t');
    assert.equal(current.prompt.cue,key);assert.equal(current.prompt.assisted,false);
    const a=await answer(value);assert.equal(a.grade.verdict,'Good');cards.set(key,a.effects[0].card_id);
   }
   assert.equal(cards.size,26);const due=await f.ok(['due']);assert.equal(due.cards.length,26);assert.ok(due.cards.every(c=>c.direction==='forward'));
   const slip=await practice('J','Juliet');assert.equal(slip.grade.verdict,'Again');assert.ok(slip.grade.diff.some(d=>d.kind==='slip'&&d.expected.text==='Juliett'));assert.equal((await f.ok(['next'])).practice.mapping_feedback.kind,'spelling');
   const wrong=await practice('A','Bravo');assert.equal(wrong.grade.verdict,'Again');assert.ok(wrong.grade.diff.some(d=>d.kind==='wrong'));const feedback=(await f.ok(['next'])).practice.mapping_feedback;assert.equal(feedback.kind,'wrong_key');assert.equal(feedback.answered_key,'B');
   for(const text of ['Alfa',' ALFA! ','alfa'])assert.equal((await practice('A',text)).grade.verdict,'Good');
   for(const text of ['X-ray','x ray'])assert.equal((await practice('X',text)).grade.verdict,'Good');
   assert.equal((await practice('X','Xray')).grade.verdict,'Again');
   assert.equal((await practice('A','a.l.f.a')).grade.verdict,'Again');
   transcript.push({observation:'Native exact-v2 cases',cases:{Alpha:'Again',Alfa:'Good',Juliet:'Again/slip',Bravo_for_A:'Again/wrong','X-ray':'Good','x ray':'Good',Xray:'Again','a.l.f.a':'Again'},cards:26});
  }],
  ['client grades, corrections, malformed mappings and generation fields refuse without writes',async()=>{
   await next(['--card',cards.get('A')]);
   for(const key of ['verdict','grade','rating','expected'])await refuse('/api/v1/answer',{prompt_id:current.prompt.id,answer:'wrong',[key]:'Good'},422);
   const a=await answer('Alpha');await refuse('/api/v1/correct',{prompt_id:current.prompt.id,attempt_id:a.id,verdict:'Good'},422);
   await refuse('/api/v1/library/mappings',{data:{title:'Bad supplied pairs',direction:'forward',pairs:[{key:'A',value:'Alfa'},{key:'a',value:'Bravo'}]}},422);
   await refuse('/api/v1/library/mappings',{data:{title:'No generator',direction:'forward',pairs:[{key:'A',value:'Alfa'}],generate:true}},422);
   const full=(await f.ok(['export'])).app;assert.deepEqual(full.jobs,{});assert.deepEqual(full.assessments,{});assert.deepEqual(full.spend,[]);assert.deepEqual(full.practice.corrections,{});assert.equal(f.egress,0);
  }],
  ['revealed answer remains assisted and replay commits once',async()=>{
   await next(['--card',cards.get('A')]);const cid=cards.get('A'),before=(await f.record('practice_cards/'+cid)).state.schedule;
   const revealed=await f.ok(['reveal',current.prompt.id]);assert.equal(revealed.practice.assisted,true);assert.ok(revealed.practice.expected.some(u=>u.text==='Alfa'));
   const assisted=await answer('Alfa');assert.equal(assisted.grade.verdict,'Good');assert.equal(assisted.assisted,true);assert.equal(assisted.effects[0].scheduled,false);assert.deepEqual((await f.record('practice_cards/'+cid)).state.schedule,before);
   await next(['--card',cid]);const pid=current.prompt.id,op='mapping-answer-replay-operation',payload={operation_id:op,prompt_id:pid,answer:'Alfa'};
   const response=await f.api('/api/v1/answer',payload);assert.equal(response.status,200);await response.body.cancel();
   const frozen=await f.rows(),a=await f.ok(['answer',pid,'--text-stdin','--operation-id',op],'Alfa');assert.equal(a.replayed,true);const firstReplay=await f.rows();transcript.push({observation:'Lost answer response reconciled once',...successfulReplay(frozen,firstReplay)});current.attempt_id=a.practice.attempt.id;
   const b=await f.ok(['answer',pid,'--text-stdin','--operation-id',op],'Alfa');assert.deepEqual(b.practice,a.practice);const secondReplay=await f.rows();transcript.push({observation:'Repeated answer receipt remains immutable',...successfulReplay(firstReplay,secondReplay)});
   assert.equal((await f.api('/api/v1/answer',{...payload,answer:'Alpha'})).status,409);assert.deepEqual(await f.rows(),secondReplay);
  }],
  ['published predecessor restores its baseline and refuses mapping Library/Study without writes',async()=>{
   assert.ok(predecessorArtifact,'Exact frozen predecessor is mandatory.');
   const response=await f.api('/api/v1/export');assert.equal(response.status,200);const studyBytes=Buffer.from(await response.arrayBuffer());
   for(const [label,bytes,accept]of [['baseline',f.archives.baseline,true],['mapping-library',f.archives.mappingLibrary,false],['mapping-study',studyBytes,false]]){
    const result=await predecessorRestore(predecessorArtifact,bytes,accept,f.at);transcript.push({observation:'Published v0.8.1 predecessor '+label,...result});
   }
  }],
  ['held result, immutable history, runtime restart and complete local-R2 restore',async()=>{
   const held=await f.ok(['next']),history=await f.ok(['history']);assert.deepEqual(history.attempts[original.id],original);
   assert.deepEqual((await f.ok(['export'])).app.practice.attempts,history.attempts);
   const page=await(await f.worker.fetch(f.origin+'/recall')).text();assert.match(page,/data-state="exact-result"/);assert.doesNotMatch(page,/I was right|Count as a miss/);
   const before=(await f.ok(['export'])).app.practice;await f.restart();assert.deepEqual((await f.ok(['export'])).app.practice,before);assert.deepEqual((await f.ok(['next'])).practice,held.practice);
   await f.restore();assert.equal(f.egress,0);
  }],
  ['rendered native forms preserve transport fields and server grading',()=>nativeMappingForms(transcript)]
 ];
 return {criteria,transcript,close:f.close};
}

// This submits the emitted HTML form contract through real HTTP. It does not
// substitute for browser rendering, keyboard/touch or phone-layout acceptance.
async function nativeMappingForms(transcript){
 const f=await keyedMappingFixture();
 const get=async()=>{const response=await f.worker.fetch(f.origin+'/recall');assert.equal(response.status,200);return response.text();};
 const post=async(html,action,extra={})=>{
  const form=[...html.matchAll(/<form\b[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(match=>match[1]===action);assert.ok(form,'The requested form is rendered.');
  const fields=Object.fromEntries([...form[2].matchAll(/<input type="hidden" name="([^"]+)" value="([^"]*)">/g)].map(match=>[match[1],match[2]]));
  for(const field of ['csrf','operation_id','revision'])assert.ok(fields[field],`Native form carries ${field}.`);
  const response=await f.worker.fetch(f.origin+action,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':f.origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,...extra})});
  assert.equal(response.status,303,`Native ${action} must accept its emitted fields; HTTP ${response.status}.`);
  return fields;
 };
 try{
  const front=await get();assert.match(front,/data-state="exact-prompt"/);assert.doesNotMatch(front,/Alfa|Bravo/);assert.match(front,/<input id="answer" name="answer" type="text"/);
  const first=await post(front,'/practice/answer',{answer:'Alpha'}),miss=(await f.ok(['next'])).practice;
  assert.equal(miss.prompt.id,first.prompt_id);assert.equal(miss.attempt.grade.verdict,'Again');assert.equal(miss.attempt.assisted,false);assert.equal(miss.attempt.effects[0].scheduled,true);
  const held=await get();assert.match(held,/data-state="exact-result"/);assert.match(held,/Alfa/);assert.doesNotMatch(held,/I was right|Count as a miss/);
  await post(held,'/practice/next');
  const next=await get();assert.match(next,/data-state="exact-prompt"/);assert.doesNotMatch(next,/Alfa|Bravo/);
  const frozen=(await f.record('meta')).practice.current;assert.equal(frozen.prompt.cue,'B');assert.notEqual(frozen.prompt.id,first.prompt_id);
  await post(next,'/practice/answer',{answer:'Bravo'});const correct=(await f.ok(['next'])).practice.attempt;
  assert.equal(correct.grade.verdict,'Good');assert.equal(correct.assisted,false);assert.equal(correct.effects[0].scheduled,true);assert.equal(f.egress,0);
  const result=await get();assert.match(result,/data-state="exact-result"/);assert.match(result,/Bravo/);
  transcript.push({observation:'Actual rendered native form HTTP contract',answer_transport_fields:['csrf','operation_id','revision','prompt_id','answer'],miss:'Again',explicit_next:true,correct:'Good',scheduled:true,browser_observation:false});
 }finally{await f.close();}
}

async function predecessorRestore(artifact,bytes,accept,at){
 const name='mapping-predecessor-'+randomUUID(),restoreToken='synthetic-predecessor-restore-only-32chars';
 const bindings={SCRY_ENV:'restore',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey,RESTORE_TOKEN:restoreToken,SCRY_TEST_NOW_MS:String(at)};
 let runtime=await createRuntime({artifact,port:0,name,inspect:true,testClock:true,bindings});
 try{
  const origin=(await runtime.mf.ready).origin;runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin})};
  const sql=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
  const schema=()=>sql.exec("SELECT type,name,tbl_name,sql FROM sqlite_master WHERE name NOT LIKE '_cf_%' ORDER BY type,name");
  const initialSchema=await schema();
  // The released runtime allocates its empty records table on first health or
  // restore. Make this explicit and separate from the archive-refusal snapshot.
  const ns=await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE'),stub=ns.get(ns.idFromName(name));
  const headers={'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'};
  const health=await stub.fetch('https://internal.invalid/__internal/health',{headers});assert.equal(health.status,503);assert.equal(await health.text(),'backup_stale');
  const initializedSchema=await schema(),rows=()=>sql.exec('SELECT key,value FROM records ORDER BY key');
  const initialization={control:'read-only health before refusal snapshot',http_status:503,records_empty:true,schema_before:initialSchema,schema_after:initializedSchema};
  const objects=async()=>{const out={};for(const bucketName of ['SCRY_ASSETS','SCRY_BACKUPS']){const bucket=await runtime.mf.getR2Bucket(bucketName);out[bucketName]=[];for(const item of (await bucket.list()).objects)out[bucketName].push([item.key,sha(Buffer.from(await(await bucket.get(item.key)).arrayBuffer()))]);}return out;};
  const before=await rows(),r2=await objects();assert.deepEqual(before,[]);
  const result=await runtime.worker.fetch(origin+'/operator/restore',{method:'POST',headers:{'x-scry-restore-token':restoreToken,'x-scry-sha256':sha(bytes)},body:bytes});
  assert.equal(result.status,accept?200:422,await result.text());
  if(!accept){assert.deepEqual(await rows(),before);assert.deepEqual(await schema(),initializedSchema);assert.deepEqual(await objects(),r2);return {status:'refused',http_status:422,initialization,sqlite_records_and_schema_unchanged:true,r2_unchanged:true,archive_sha256:sha(bytes)};}
  const expected=JSON.parse(bytes).app;
  const read=await stub.fetch('https://internal.invalid/api/v1/export',{headers});assert.equal(read.status,200);const restored=(await read.json()).app;
  assert.deepEqual(restored.library,expected.library);assert.deepEqual(restored.practice,expected.practice);
  assert.ok(Object.keys(restored.practice.attempts).length,'Baseline contains an actual text Attempt, not an empty decoder smoke.');
  const written=await stub.fetch('https://internal.invalid/api/v1/settings',{method:'POST',headers:{...headers,'Content-Type':'application/json'},body:JSON.stringify({csrf:restored.csrf,operation_id:'mapping-predecessor-baseline-write',pace:'light'})});assert.equal(written.status,200);
  const backed=await stub.fetch('https://internal.invalid/__internal/daily',{method:'POST',headers});assert.equal(backed.status,200);
  const bucket=await runtime.mf.getR2Bucket('SCRY_BACKUPS'),items=await bucket.list({prefix:`snapshots/${name}/`});assert.ok(items.objects.length);
  const saved=JSON.parse(Buffer.from(await(await bucket.get(items.objects.at(-1).key)).arrayBuffer()));assert.deepEqual(saved.app.library,expected.library);assert.deepEqual(saved.app.practice,expected.practice);
  return {status:'accepted',http_status:200,initialization,history_retained:true,unrelated_write_and_backup:true,archive_sha256:sha(bytes)};
 }finally{await runtime.mf.dispose();}
}

async function typography(page,zoom){
 if(zoom)await page.evaluate(()=>{for(const node of document.querySelectorAll('[data-qa-text-size]')){node.style.removeProperty('font-size');delete node.dataset.qaTextSize;}const nodes=[...document.querySelectorAll('body,body *')];for(const node of nodes)node.dataset.qaTextSize=String(parseFloat(getComputedStyle(node).fontSize)*2);for(const node of nodes)node.style.fontSize=node.dataset.qaTextSize+'px';});
 await page.evaluate(()=>document.fonts.ready);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'No horizontal overflow.');
}
export async function keyedMappingWalk(browser,capture){
 const rows=[];
 for(const [width,scheme,noJS,zoom] of [[390,'light',false,false],[390,'dark',false,false],[320,'light',false,false],[390,'light',false,true],[390,'dark',true,true],[320,'dark',true,false]]){
  const f=await keyedMappingFixture(),context=await browser.newContext({viewport:{width,height:844},colorScheme:scheme,javaScriptEnabled:!noJS,isMobile:true,hasTouch:true});
  const page=await context.newPage();page.setDefaultTimeout(12000);const row={width,scheme,noJS,text_zoom:zoom?'200%':'100%',status:'pass',evidence:[]};rows.push(row);
  const shot=async label=>row.evidence.push(await capture(page,`mapping-${width}-${scheme}-${noJS?'native':'enhanced'}-${zoom?200:100}-${label}.png`));
  try{
   await page.goto(f.origin+'/recall');await page.locator('[data-state="exact-prompt"]').waitFor();
   assert.equal(await page.locator('input[name="answer"]').count(),1);assert.equal(await page.locator('textarea').count(),0);
   assert.equal((await page.locator('.exact-cue .reading').innerText()).trim(),'A');
   assert.doesNotMatch(await page.content(),/Alfa|Bravo/,'The whole unanswered HTML, including hidden inputs and metadata, carries no canonical answer.');
   assert.doesNotMatch(await page.locator('input[type="hidden"],meta').evaluateAll(nodes=>nodes.map(node=>node.outerHTML).join('')),/Alfa|Bravo/);
   await typography(page,zoom);await shot('prompt');
   await page.getByLabel('Type your answer',{exact:true}).fill('Alpha');
   await page.getByRole('button',{name:'Check my answer',exact:true}).focus();await page.keyboard.press('Enter');
   await page.locator('[data-state="exact-result"]').waitFor();
   row.arrival=await feedbackArrival(page,capture,`mapping-${width}-${scheme}-${noJS?'native':'enhanced'}-${zoom?200:100}-arrival.png`,zoom);assertFeedbackArrival(row.arrival);
   const a=(await f.ok(['next'])).practice.attempt;assert.equal(a.grade.verdict,'Again');assert.equal(await page.locator('[data-verdict]').getAttribute('data-verdict'),'Again');
   assert.equal(await page.getByRole('button',{name:'I was right',exact:true}).count(),0);assert.equal(await page.getByRole('button',{name:'Count as a miss',exact:true}).count(),0);
   assert.ok((await page.locator('.mapping-result').innerText()).includes('Alfa'));
   await page.reload();await page.locator('[data-state="exact-result"]').waitFor();assert.equal((await f.ok(['next'])).practice.attempt.id,a.id);
   await typography(page,zoom);await shot('held-reload');
   await page.locator('details.exact-full-answer>summary').tap();assert.equal(await page.locator('.exact-expected').textContent(),'Alfa');
   await page.locator('details.exact-full-answer>summary').tap();
   await page.getByRole('button',{name:'Next',exact:true}).tap();await page.locator('[data-state="exact-prompt"]').waitFor();
   assert.equal((await page.locator('.exact-cue .reading').innerText()).trim(),'B');
   const next=(await f.record('meta')).practice.current.prompt.id;await page.reload();await page.locator('[data-state="exact-prompt"]').waitFor();assert.equal((await f.record('meta')).practice.current.prompt.id,next);
   await page.goto(f.origin+'/library');await page.goBack();await page.locator('[data-state="exact-prompt"]').waitFor();assert.equal((await f.record('meta')).practice.current.prompt.id,next);
   assert.doesNotMatch(await page.content(),/Alfa|Bravo/,'Next removes the held answer and does not preload the current answer in any HTML.');
   await typography(page,zoom);await shot('next');
   await page.getByLabel('Type your answer',{exact:true}).fill('Bravo');await page.getByRole('button',{name:'Check my answer',exact:true}).tap();await page.locator('[data-verdict="Good"]').waitFor();
   const correct=(await f.ok(['next'])).practice.attempt;assert.equal(correct.assisted,false);assert.equal(correct.effects[0].scheduled,true);
   await typography(page,zoom);assert.ok((await page.locator('.mapping-result').innerText()).includes('Bravo'));
   row.good=await page.locator('[data-verdict="Good"]').evaluate(node=>{const r=node.getBoundingClientRect();return {visible:r.top>=0&&r.bottom<=innerHeight&&r.left>=0&&r.right<=innerWidth,verdict:node.dataset.verdict};});assert.equal(row.good.visible,true);
   row.evidence.push(await capture(page,`mapping-${width}-${scheme}-${noJS?'native':'enhanced'}-${zoom?200:100}-good-arrival.png`,{fullPage:false}));
   await page.getByRole('button',{name:'Next',exact:true}).tap();await page.locator('[data-state="exact-prompt"]').waitFor();assert.equal((await page.locator('.exact-cue .reading').innerText()).trim(),'C');
   assert.doesNotMatch(await page.content(),/Charlie/);
   await page.locator('details.overflow>summary').tap();const before=(await f.record('meta')).practice.current.targets[0].before.schedule;
   await page.getByRole('button',{name:'Show me',exact:true}).tap();await page.getByText('Assistance is recorded.',{exact:true}).waitFor();
   await page.getByLabel('Type your answer',{exact:true}).fill('Charlie');await page.getByRole('button',{name:'Check my answer',exact:true}).tap();await page.locator('[data-state="exact-result"]').waitFor();
   const shown=(await f.ok(['next'])).practice.attempt;assert.equal(shown.assisted,true);assert.equal(shown.effects[0].scheduled,false);assert.deepEqual(shown.effects[0].after.schedule,before);
   await typography(page,zoom);await shot('assisted');assert.equal(f.egress,0);
  }catch(e){row.status='fail';row.reason=e.message;await shot('failure');}
  finally{await context.close();await f.close();}
 }
 return rows;
}

export async function keyedMappingRecovery(browser,capture){
 const results=[];
 const check=async(name,run)=>{
  const f=await keyedMappingFixture(),context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true}),page=await context.newPage();page.setDefaultTimeout(12000);
  const row={name,status:'pass'};results.push(row);
  try{await page.goto(f.origin+'/recall');await page.locator('[data-state="exact-prompt"]').waitFor();await run({f,context,page,row});assert.equal(f.egress,0);}
  catch(error){row.status='fail';row.reason=error.message;}
  finally{row.evidence=await capture(page,`mapping-recovery-${results.length}.png`);await context.close();await f.close();}
 };
 const answer=async page=>{await page.getByLabel('Type your answer',{exact:true}).fill('Alfa');await page.getByRole('button',{name:'Check my answer',exact:true}).tap();};
 const count=async f=>(await f.rows()).filter(row=>row.key.startsWith('practice_attempts/')).length;
 await check('lost answer response locks input and reconciles the same operation once',async({f,context,page,row})=>{
  const before=await count(f),payloads=[];let first=true;
  await context.route('**/practice/answer',async route=>{if(route.request().method()!=='POST')return route.continue();payloads.push(route.request().postData());if(first){first=false;assert.equal((await route.fetch({maxRedirects:0})).status(),303);await route.abort('failed');}else await route.continue();});
  await answer(page);await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();assert.equal(await page.getByLabel('Type your answer',{exact:true}).isEditable(),false);
  await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).tap();await page.locator('[data-state="exact-result"]').waitFor();
  const field=(body,name)=>new RegExp(`name="${name}"\\r\\n\\r\\n([^\\r]*)`).exec(body)?.[1];
  assert.equal(payloads.length,2);for(const name of ['operation_id','prompt_id','answer']){assert.ok(field(payloads[0],name));assert.equal(field(payloads[0],name),field(payloads[1],name));}
  assert.equal(await count(f),before+1);row.observed={sends:2,attempts_added:1,frozen_fields_equal:true};
 });
 await check('offline preserves typed input and reconnect never auto-sends',async({context,page,row})=>{
  let sends=0;context.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/practice/answer'))sends++;});
  await page.getByLabel('Type your answer',{exact:true}).fill('Alfa');await context.setOffline(true);await page.getByRole('button',{name:'Check my answer',exact:true}).tap();
  assert.equal(sends,0);assert.equal(await page.getByLabel('Type your answer',{exact:true}).inputValue(),'Alfa');await context.setOffline(false);await page.waitForTimeout(400);assert.equal(sends,0);
  await page.getByRole('button',{name:'Check my answer',exact:true}).tap();await page.locator('[data-state="exact-result"]').waitFor();assert.equal(sends,1);row.observed={offline_sends:0,reconnect_sends:0,deliberate_sends:1};
 });
 await check('competing tabs retain exactly one answered Prompt',async({f,context,page,row})=>{
  const before=await count(f),other=await context.newPage();await other.goto(f.origin+'/recall');await answer(page);await page.locator('[data-state="exact-result"]').waitFor();
  await answer(other);await other.getByText('This changed in another tab. Your draft is still here.',{exact:true}).waitFor();assert.equal(await count(f),before+1);row.observed={attempts_added:1,stale_refusal:409};await other.close();
 });
 return results;
}

// Independent review regressions through the actual API, not just pure policy.
export async function keyedMappingBoundaryJourney(){
 const f=await keyedMappingFixture();
 try{
  const call=async(path,payload)=>{const r=await f.api(path,{operation_id:randomUUID(),...payload});assert.equal(r.status,200,await r.clone().text());return r.json();};
  const manifest=(title,key,value,strict=true)=>({title,direction:'forward',spelling_strict:strict,pairs:[{key,value}]});
  const title='Alfa: NATO spelling alphabet';
  const vid=(await call('/api/v1/library/mappings',{data:manifest(title,'A','Alfa')})).result.entity_id;
  const sid=(await call('/api/v1/learn',{version_id:vid})).practice.study.id;
  const next=await call('/api/v1/next',{study_id:sid}),prompt=next.practice.prompt;
  assert.equal(prompt.cue,'A');assert.equal(prompt.assisted,false);assert.doesNotMatch(JSON.stringify(next),/Alfa/,'Neither result nor practice may disclose the supplied answer-bearing title.');
  assert.doesNotMatch(await(await f.worker.fetch(f.origin+'/recall')).text(),/Alfa/,'Whole active HTML hides supplied title.');
  const card=(await f.ok(['due'])).cards.find(card=>card.study_id===sid),before=(await f.record('practice_cards/'+card.id)).state.schedule;
  await call('/api/v1/reveal',{prompt_id:prompt.id});
  const assisted=(await call('/api/v1/answer',{prompt_id:prompt.id,answer:'Alfa'})).practice.attempt;
  assert.equal(assisted.grade.verdict,'Good');assert.equal(assisted.assisted,true);assert.equal(assisted.effects[0].scheduled,false);assert.deepEqual((await f.record('practice_cards/'+card.id)).state.schedule,before);
  const supplied=(await f.ok(['export'])).app;assert.equal(supplied.library.works[supplied.library.versions[vid].work_id].title,title);
  const q=(await call('/api/v1/library/mappings',{data:manifest('Letter pairs','Q','A&B')})).result.entity_id;
  const qs=(await call('/api/v1/learn',{version_id:q})).practice.study.id;
  const qp=(await call('/api/v1/next',{after:prompt.id,study_id:qs})).practice.prompt;
  assert.equal((await call('/api/v1/answer',{prompt_id:qp.id,answer:'A B'})).practice.attempt.grade.verdict,'Good');
  const original=(await f.ok(['export'])).app;
  assert.equal((await call('/api/v1/library/mappings',{data:manifest('Letter pairs','Q','A B')})).result.entity_id,q);
  assert.equal((await call('/api/v1/learn',{version_id:q})).practice.study.id,qs);
  const retained=(await f.ok(['export'])).app;assert.deepEqual(retained.library,original.library);assert.deepEqual(retained.practice,original.practice);
  const rows=await f.rows(),objects=await f.r2();assert.equal((await f.api('/api/v1/library/mappings',{operation_id:randomUUID(),data:manifest('Letter pairs','Q','A B',false)})).status,409);assert.deepEqual(await f.rows(),rows);assert.deepEqual(await f.r2(),objects);assert.equal(f.egress,0);
  return {status:'pass',supplied_title_hidden_in_json_and_html:true,metadata_retained:true,reveal_assisted_no_schedule:true,normalized_pair_reuses_version_and_study:true,original_hash_bytes_history_and_schedule_unchanged:true,policy_conflict_no_writes:true,egress:0};
 }finally{await f.close();}
}
