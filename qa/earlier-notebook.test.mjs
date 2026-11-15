#!/usr/bin/env node
// The earlier concept notebook is history: never served, never sent to Jev,
// never written by a read, and never in the way of anything else. Every state
// an existing notebook can hold is loaded from tests/fixtures/earlier-notebook.json
// into the exact built Rust/WASM Worker, in real workerd with isolated SQLite
// and R2. Jev is a synthetic endpoint inside this process that counts every
// request, so "nothing was sent" is counted, not inferred. No account,
// credential, real model or Owner data is used.
import assert from 'node:assert/strict';
import test from 'node:test';
import {createHash,generateKeyPairSync,randomUUID,sign} from 'node:crypto';
import {mkdtemp,readFile,readdir,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,relative} from 'node:path';
import {createRuntime,Response,internalKey} from './runtime-helper.mjs';
import {states,stored,storedApp,storedMeta,rawField,seedEarlier,earlierRows} from './earlier-fixture.mjs';
import {nativeFields} from './nato-starter.mjs';
import {enterMemory} from './study-review.mjs';

const team='earlier-synthetic.cloudflareaccess.com',audience='synthetic-earlier-audience',owner='synthetic-earlier-owner';
const JEV_URL='https://jev-synthetic.invalid/v1/decisions',JEV_KEY='synthetic-jev-key',JEV_MODEL='synthetic-jev';
const {publicKey,privateKey}=generateKeyPairSync('rsa',{modulusLength:2048});
const publicJwk={...publicKey.export({format:'jwk'}),kid:'synthetic-key',alg:'RS256',use:'sig'};
const b64=value=>Buffer.from(JSON.stringify(value)).toString('base64url');
const sleep=ms=>new Promise(done=>setTimeout(done,ms));
const SECOND=1000,YEAR=365*24*60*60*SECOND;
const RETIRED=/The earlier concept notebook is kept as history\. Scry no longer serves or grades its questions; everything saved is unchanged\./;
// Rows that hold the earlier concept notebook. Study, Library, tokens and
// operation receipts live under other names.
const EARLIER=['goals/','concepts/','questions/','events/','overrides/','resets/','feedback/','assessments/','jobs/','job_payload/','spend/','spend_payload/'];
const earlier=rows=>Object.fromEntries(Object.entries(rows).filter(([key])=>EARLIER.some(prefix=>key.startsWith(prefix))));

// One disposable object on the built Worker, holding one stored state. It is
// the Owner's own preview space with Jev bound: the only kind of space from
// which an answer can leave Scry. `at` fixes the clock.
async function space(state,{at,persist=false}={}){
 const name='earlier-'+randomUUID(),spaceName='earlier-'+randomUUID();
 const temp=persist?await mkdtemp(join(tmpdir(),'scry-earlier-')):null;
 const jev={sent:[],elsewhere:[]};
 const egress=async request=>{
  if(request.url===`https://${team}/cdn-cgi/access/certs`)return new Response(JSON.stringify({keys:[publicJwk]}),{headers:{'Content-Type':'application/json'}});
  if(request.url===JEV_URL&&request.method==='POST'){
   jev.sent.push(await request.text());
   // A confident acceptance: a source that still sent a check would also finish it.
   return new Response(JSON.stringify({model:JEV_MODEL,usage:{cost:0.000345},answers:{verdict:{choice:'accept',probabilities:{accept:0.98,reject:0.01,unsure:0.01}},identity:{noul:0.02},injection:{noul:0}}}),{headers:{'Content-Type':'application/json'}});
  }
  jev.elsewhere.push(request.url);throw new Error('Nothing but the synthetic Jev may be reached.');
 };
 const clock=at===undefined?{}:{SCRY_TEST_NOW_MS:String(at)};
 const origin='https://scry-earlier-synthetic.invalid';
 const bindings={SCRY_ENV:'preview',CANONICAL_ORIGIN:origin,SPACE_NAME:spaceName,ACCESS_TEAM:team,ACCESS_AUDIENCE:audience,OWNER_SUBJECT:owner,INTERNAL_KEY:internalKey,JEV_API_KEY:JEV_KEY,JEV_MODEL,JEV_URL,...clock};
 const runtime=await createRuntime({bindings,name,egress,inspect:true,testClock:at!==undefined,...(temp?{resourcePersistencePath:join(temp,'state')}:{})});
 let worker=runtime.worker;
 try{
  const jwt=()=>{
   const epoch=Math.floor((at??Date.now())/1000);
   const encoded=`${b64({alg:'RS256',kid:'synthetic-key',typ:'JWT'})}.${b64({iss:`https://${team}`,sub:owner,aud:[audience],iat:epoch,nbf:epoch-1,exp:epoch+600})}`;
   return `${encoded}.${sign('RSA-SHA256',Buffer.from(encoded),privateKey).toString('base64url')}`;
  };
  const s={state,origin,jev,at,name:spaceName};
  s.sql=()=>runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:spaceName});
  s.stub=async()=>{const ns=await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE');return ns.get(ns.idFromName(spaceName));};
  s.rows=async()=>earlierRows(await s.sql());
  s.changes=async()=>(await(await s.sql()).exec('SELECT total_changes() AS n'))[0].n;
  // Runs the alarm handler now, as the platform would at the armed time.
  const rpc=async call=>{const stub=await s.stub();try{return await call(stub);}finally{stub[Symbol.dispose]?.();}};
  s.alarm=()=>rpc(stub=>stub.qaAlarm());
  s.alarmAt=()=>rpc(stub=>stub.qaAlarmTime());
  // The signed-in Owner; `s.token` is a program holding an Owner token instead.
  s.req=(path,init={})=>worker.fetch(origin+path,{redirect:'manual',...init,headers:{'Cf-Access-Jwt-Assertion':jwt(),...init.headers}});
  s.token=(path,secret,init={})=>worker.fetch(origin+path,{redirect:'manual',...init,headers:{Authorization:'Bearer '+secret,...init.headers}});
  s.get=async path=>{const response=await s.req(path);assert.equal(response.status,200,`GET ${path}: ${response.status}`);return response.text();};
  s.post=(path,fields,headers={})=>s.req(path,{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded',...headers},body:new URLSearchParams(fields)});
  // A valid form envelope: the stored cross-site value and a fresh operation.
  s.envelope=async()=>({csrf:JSON.parse((await s.rows()).meta).csrf,operation_id:'earlier-'+randomUUID()});
  s.restart=async()=>{worker=(await runtime.rebind(bindings)).worker;};
  // Every persisted byte: the object's database and its log, alarm metadata
  // and R2. A `-shm` file is SQLite's transient index of its log, rewritten
  // when the local R2 emulator is read; it holds no stored data.
  s.files=async()=>{
   const found=[];
   const walk=async dir=>{for(const entry of await readdir(dir,{withFileTypes:true})){const path=join(dir,entry.name);if(entry.isDirectory())await walk(path);else if(!path.endsWith('-shm'))found.push({path:relative(temp,path),sha256:createHash('sha256').update(await readFile(path)).digest('hex')});}};
   await walk(temp);return found.sort((a,b)=>a.path.localeCompare(b.path));
  };
  s.close=async()=>{await runtime.mf.dispose();if(temp)await rm(temp,{recursive:true,force:true});};
  await seedEarlier(await s.sql(),state,await runtime.mf.getR2Bucket('SCRY_ASSETS'));
  return s;
 }catch(error){await runtime.mf.dispose();if(temp)await rm(temp,{recursive:true,force:true});throw error;}
}

// Every page and read an Owner or his agent can open for this notebook.
function reads(state){
 const app=storedApp(state);
 return {
  pages:['/','/?practice=nato','/recall','/lessons','/library','/notebook','/map',
   ...Object.keys(app.goals).map(id=>`/goals/${id}`),
   ...Object.keys(app.concepts).map(id=>`/concepts/${id}`),
   ...Object.keys(app.questions).map(id=>`/questions/${id}/edit`),
   '/history','/settings','/gate','/no-such-page'],
  other:['/export','/readyz',...Object.values(app.goals).filter(goal=>goal.photo).map(goal=>`/photos/${goal.id}`)],
  api:['/api/v1/export','/api/v1/history','/api/v1/due','/api/v1/studies','/api/v1/draft','/api/v1/spend?by=day'],
 };
}

// What /notebook must show for a stored state, from the stored facts alone.
// With nothing held, a saved generation request whose preparation stopped still
// leads the page as before: a retired generation record, not practice.
function shown(state){
 const meta=storedMeta(state),held=meta.occurrence;
 if(!held)return {view:Object.values(storedApp(state).goals).some(goal=>!goal.archived&&!goal.paused&&goal.status==='failed')?'preparation-stopped':'earlier-empty'};
 if(held.phase==='result'&&held.result)return {view:'earlier-result'};
 if(held.answer===null)return {view:'earlier-unanswered'};
 const check=held.assessment_id&&JSON.parse(stored(state).rows['assessments/'+held.assessment_id]);
 return {view:'earlier-ungraded',check:check?check.status:'none',error:check?.error};
}
// A page is long: report what was looked for, never the page.
const has=(html,pattern,what)=>assert.ok(pattern.test(html),`Missing: ${what}`);
const lacks=(html,pattern,what)=>assert.ok(!pattern.test(html),`Present: ${what}`);
const escapeHtml=text=>text.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
// No page practises the earlier notebook: no route into it and nothing polled.
function assertNoPractice(html,path){
 lacks(html,/action="\/review\//,`${path} posts to a review route`);
 lacks(html,/action="\/concepts\/[^"]+\/practice"/,`${path} offers concept practice`);
 lacks(html,/data-choice|Look it up|href="\/gate|Practice this idea|Next question|Check my answer|Show me the answer|Retry native check/,`${path} offers an answer, Next, Retry, reveal or the reference gate`);
 lacks(html,/data-state="(intro|question|checking|self-check|result|gate|checking-reference|checking-paused)"/,`${path} renders a practice state`);
}

test('a stored check is never sent, finished or restarted, and asks for no alarm',{timeout:600000},async()=>{
 for(const state of ['checking-queued','checking-sent','queued-while-paused','self-check-paused'])for(const later of [100*SECOND,10*YEAR]){
  const fixture=stored(state),at=fixture.now_ms+later,label=`${state} +${later/SECOND}s`;
  const s=await space(state,{at});
  try{
   const check=async when=>{
    const rows=await s.rows(),where=`${label}, ${when}`;
    assert.equal(s.jev.sent.length,0,`${where}: a request reached Jev`);assert.deepEqual(s.jev.elsewhere,[],where);
    assert.deepEqual(earlier(rows),earlier(fixture.rows),`${where}: an earlier row changed`);
    assert.equal(rawField(rows.meta,'occurrence'),rawField(fixture.rows.meta,'occurrence'),`${where}: the held question changed`);
    const alarm=await s.alarmAt();
    assert.ok(alarm===null||alarm>at+5*SECOND,`${where}: the alarm is set for ${alarm===null?alarm:alarm-at} ms from now`);
   };
   assert.equal(await s.alarmAt(),null);await check('as loaded');
   assert.equal(await s.alarm(),200);await check('after the alarm fired');
   const {pages}=reads(state);
   for(const path of pages){const html=await s.get(path);assertNoPractice(html,path);}
   assert.equal((await s.req('/export')).status,200);await check('after every page was read');
   // Resuming paused judgment starts nothing that was saved before it.
   const resumed=await s.post('/settings/resume',await s.envelope());
   if(storedMeta(state).restored_paused){assert.equal(resumed.status,303);assert.equal(resumed.headers.get('Location'),'/settings');assert.equal(JSON.parse((await s.rows()).meta).restored_paused,false);}
   else assert.equal(resumed.status,409);
   await check('after resume');
   assert.equal(await s.alarm(),200);await check('after resume and the alarm');
   await s.restart();
   for(const path of pages)await s.get(path);
   assert.equal(await s.alarm(),200);await check('after a restart');
   const view=await s.get('/notebook');
   has(view,/data-state="earlier-ungraded"/,`${label}: saved, not graded`);has(view,new RegExp(`data-check="${shown(state).check}"`),`${label}: the stored status of its check`);lacks(view,/data-poll/,`${label}: a poll`);
  }finally{await s.close();}
 }
});

test('on the wall clock, with every timer free to fire, nothing is sent',{timeout:120000},async()=>{
 for(const state of ['checking-queued','queued-while-paused']){
  const fixture=stored(state),s=await space(state);
  try{
   for(const path of reads(state).pages)await s.get(path);
   const resumed=await s.post('/settings/resume',await s.envelope());
   assert.equal(resumed.status,storedMeta(state).restored_paused?303:409);
   await s.get('/notebook');await sleep(2500);await s.get('/notebook');
   const rows=await s.rows();
   assert.equal(s.jev.sent.length,0,`${state}: a request reached Jev`);
   assert.deepEqual(earlier(rows),earlier(fixture.rows),`${state}: an earlier row changed`);
   assert.equal(rawField(rows.meta,'occurrence'),rawField(fixture.rows.meta,'occurrence'));
   const alarm=await s.alarmAt();assert.ok(alarm===null||alarm>Date.now()+5*SECOND,`${state}: an alarm is set ${alarm-Date.now()} ms from now`);
  }finally{await s.close();}
 }
});

test('a read writes nothing, in every stored state',{timeout:900000},async()=>{
 for(const state of states){
  const fixture=stored(state),at=fixture.now_ms+100*SECOND;
  const s=await space(state,{at,persist:true});
  try{
   // The first request an object ever serves settles its storage files; the
   // rows, the change count and the alarm are compared from before it.
   const changes=await s.changes();
   const {pages,other,api}=reads(state);
   const unchanged=async when=>{
    assert.deepEqual(await s.rows(),fixture.rows,`${state}: a stored row changed ${when}`);
    assert.equal(await s.changes(),changes,`${state}: SQLite recorded a write ${when}`);
    assert.equal(await s.alarmAt(),null,`${state}: an alarm was set ${when}`);
   };
   assert.equal((await s.req('/healthz')).status,503);await unchanged('on a health check');
   const files=await s.files();assert.ok(files.some(file=>file.path.endsWith('.sqlite')),'The persisted database is compared.');
   for(const method of ['GET','HEAD'])for(const path of [...pages,...other,...api,'/healthz']){
    const response=await s.req(path,{method});
    assert.equal(response.status,method==='GET'&&path==='/healthz'?503:200,`${state}: ${method} ${path} answered ${response.status}`);
    if(method==='GET'&&pages.includes(path))assertNoPractice(await response.text(),path);else await response.arrayBuffer();
    await unchanged(`on ${method} ${path}`);
   }
   assert.deepEqual(await s.files(),files,`${state}: a persisted file changed on a read`);
   // /notebook shows the held question or answer as it was left, by a fixed
   // label for its stored status and never the message stored with it.
   const expected=shown(state),view=await s.get('/notebook');
   has(view,new RegExp(`data-state="${expected.view}"`),`${state}: /notebook shows ${expected.view}`);
   lacks(view,/data-poll|<textarea|name="answer"/,`${state}: /notebook polls or offers an answer field`);
   if(expected.check)has(view,new RegExp(`data-check="${expected.check}"`),`${state}: the stored status ${expected.check}`);
   if(expected.error)assert.ok(!view.includes(escapeHtml(expected.error)),`${state}: the stored message is shown`);
   if(storedMeta(state).occurrence)assert.ok(view.includes(escapeHtml(storedMeta(state).occurrence.presentation.prompt)),`${state}: the question is shown`);
   // A token read of the snapshot endpoints leaves no trace either. The one
   // read that does is the token's own last use, on the endpoints that stamp it.
   const issued=await s.post('/tokens/create',{...await s.envelope(),name:'synthetic earlier reader'});assert.equal(issued.status,200);
   const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())[1];
   const withToken=await s.rows(),tokenChanges=await s.changes(),tokenFiles=await s.files();
   assert.deepEqual(earlier(withToken),earlier(fixture.rows));
   for(const method of ['GET','HEAD'])for(const path of api){
    const response=await s.token(path,secret,{method});assert.equal(response.status,200,`${state}: token ${method} ${path}`);await response.arrayBuffer();
    assert.deepEqual(await s.rows(),withToken,`${state}: token ${method} ${path} changed a row`);assert.equal(await s.changes(),tokenChanges);
   }
   assert.deepEqual(await s.files(),tokenFiles,`${state}: a persisted file changed on a token read`);
   assert.equal((await s.token('/api/v1/whoami',secret)).status,200);
   const stamped=await s.rows(),tokenRow='owner_tokens/'+secret.slice(8,40);
   assert.deepEqual(Object.keys(stamped).filter(key=>stamped[key]!==withToken[key]).sort(),['meta',tokenRow].sort(),`${state}: only the token's last use is stamped`);
   const metaBefore=JSON.parse(withToken.meta),metaAfter=JSON.parse(stamped.meta);
   assert.deepEqual({...metaAfter,revision:0},{...metaBefore,revision:0});
   assert.deepEqual({...JSON.parse(stamped[tokenRow]),last_used_ms:null},{...JSON.parse(withToken[tokenRow]),last_used_ms:null});
   assert.equal(s.jev.sent.length,0);assert.deepEqual(s.jev.elsewhere,[]);
  }finally{await s.close();}
 }
});

test('a held unanswered question blocks no export, history or reference',{timeout:120000},async()=>{
 for(const state of ['question-cold','question-cold-choice','checking-queued','history-rich']){
  const fixture=stored(state),s=await space(state,{at:fixture.now_ms+100*SECOND});
  try{
   const app=storedApp(state),bytes=rawField(fixture.archive,'app');
   // The complete export, in the browser and through the API, holds the
   // notebook exactly as it is stored.
   const browser=await s.req('/export');assert.equal(browser.status,200,`${state}: /export`);
   const browserText=await browser.text();
   assert.equal(rawField(browserText,'app'),bytes,`${state}: /export holds the stored notebook byte for byte`);
   assert.deepEqual(JSON.parse(browserText).photos,JSON.parse(fixture.archive).photos);
   assert.equal(browser.headers.get('X-Scry-Sha256'),createHash('sha256').update(browserText).digest('hex'));
   const api=await s.req('/api/v1/export');assert.equal(api.status,200,`${state}: /api/v1/export`);
   const exported=await api.json();assert.deepEqual(exported.app,app);assert.equal(exported.format,fixture.format);
   for(const path of ['/history','/api/v1/history'])assert.equal((await s.req(path)).status,200,`${state}: ${path}`);
   // History shows every earlier answer, and nothing is masked or gated.
   const history=await s.get('/history');
   assert.equal((history.match(/class="history-entry"/g)||[]).length,app.events.length,`${state}: every earlier answer is in History`);
   lacks(history,/hidden while you/,`${state}: a masked answer in History`);
   const held=app.occurrence;
   if(held){
    const concept=await s.get(`/concepts/${held.concept_id}`);
    has(concept,/data-state="concept"/,`${state}: the Concept page`);assert.ok(concept.includes(escapeHtml(held.presentation.answer)),`${state}: the saved answer is readable on its Concept`);
    has(await s.get(`/goals/${app.concepts[held.concept_id].goal_id}`),/data-state="goal"/,`${state}: the Goal page`);
    has(await s.get(`/questions/${held.question_id}/edit`),/data-state="edit"/,`${state}: the question page`);
   }
   for(const goal of Object.values(app.goals).filter(goal=>goal.photo)){
    const photo=await s.req(`/photos/${goal.id}`);assert.equal(photo.status,200);
    assert.equal(createHash('sha256').update(Buffer.from(await photo.arrayBuffer())).digest('hex'),goal.photo.sha256);
   }
   // Study is not held back by it: the alphabet's welcome and its reference.
   const welcome=await s.get('/?practice=nato');
   has(welcome,/data-state="nato-home"/,`${state}: the alphabet's home`);
   has(welcome,/<div class="nato-specimen" aria-hidden="true">Aa<span>Alfa<\/span><\/div>/,`${state}: the welcome specimen`);
   has(welcome,/From Alfa to Zulu\./,`${state}: the welcome title`);
   has(welcome,/<summary>Alphabet reference<\/summary>/,`${state}: the alphabet reference`);
   assert.deepEqual(await s.rows(),fixture.rows,`${state}: none of this wrote`);
  }finally{await s.close();}
 }
});

test('every retired route answers 410 and leaves the notebook as stored',{timeout:120000},async()=>{
 for(const state of ['intro','question-cold','question-cold-choice','checking-queued','self-check-failed','self-check-unsure','result-held','result-jev-wrong','empty-history','history-rich']){
  const fixture=stored(state),s=await space(state,{at:fixture.now_ms+100*SECOND});
  try{
   const app=storedApp(state),held=app.occurrence,changes=await s.changes();
   const occurrence=held?{occurrence_id:held.id}:{};
   const concepts=Object.keys(app.concepts);
   const refused=[
    ['/review/intro',{...occurrence,known:'false'}],['/review/intro',{...occurrence,known:'true'}],
    ['/review/answer',{...occurrence,answer:held?.presentation.answer??'304'}],['/review/answer',{...occurrence,answer:'a nonexact synthetic answer'}],
    ['/review/reveal',occurrence],['/review/help',{...occurrence,return_to:'/export'}],['/review/retry',occurrence],['/review/next',occurrence],
    ['/review/anything-else',occurrence],
    ...concepts.map(id=>[`/concepts/${id}/practice`,{}]),
   ];
   for(const [path,fields] of refused){
    const response=await s.post(path,{...await s.envelope(),...fields}),text=await response.text();
    assert.equal(response.status,410,`${state}: POST ${path} answered ${response.status}`);
    has(text,RETIRED,`${state}: POST ${path} says why`);assertNoPractice(text,path);
   }
   // Self-grading keeps its own, older refusal.
   for(const path of ['/review/self','/review/override']){
    const response=await s.post(path,{...await s.envelope(),...occurrence,correct:'true'});
    assert.equal(response.status,410);has(await response.text(),/Self-grading is retired\./,`${state}: ${path} keeps its refusal`);
   }
   // No API action ever existed for these; none is added to refuse them.
   for(const path of ['/api/v1/review/answer','/api/v1/review/next',`/api/v1/concepts/${concepts[0]}/practice`]){
    const response=await s.req(path,{method:'POST',headers:{'Content-Type':'application/json','X-Scry-Test-Origin':s.origin},body:JSON.stringify({...await s.envelope(),...occurrence,answer:'304'})});
    assert.equal(response.status,404,`${state}: POST ${path}`);
   }
   assert.deepEqual(await s.rows(),fixture.rows,`${state}: a refusal wrote a row`);
   assert.equal(await s.changes(),changes,`${state}: a refusal wrote`);assert.equal(await s.alarmAt(),null,`${state}: a refusal set an alarm`);
  }finally{await s.close();}
 }
});

test('Study beside the earlier notebook leaves every earlier row as stored',{timeout:120000},async()=>{
 const state='history-rich',fixture=stored(state),s=await space(state,{at:fixture.now_ms+100*SECOND});
 try{
  const intact=async when=>{
   const rows=await s.rows();
   assert.deepEqual(earlier(rows),earlier(fixture.rows),`An earlier row changed ${when}`);
   assert.equal(rawField(rows.meta,'occurrence'),rawField(fixture.rows.meta,'occurrence'),`The held question changed ${when}`);
   assert.equal(rawField(rows.meta,'preferences'),rawField(fixture.rows.meta,'preferences'),`The stored pace changed ${when}`);
   return rows;
  };
  const prompt=rows=>JSON.parse(rows.meta).practice.current;
  // The Study helpers read rows as values.
  const study={get:s.get,post:s.post,rows:async()=>Object.fromEntries(Object.entries(await s.rows()).map(([key,value])=>[key,JSON.parse(value)]))};
  // Import a starter and start its Study.
  const home=await s.get('/?practice=nato');has(home,/data-state="nato-home"/,'the alphabet home');
  assert.equal((await s.post('/practice/nato/start',nativeFields(home,'/practice/nato/start'))).status,303);
  let rows=await intact('on starting a Study');
  const versions=Object.keys(rows).filter(key=>key.startsWith('library_versions/')).map(key=>key.slice('library_versions/'.length));
  assert.equal(versions.length,1);
  const served=[];
  const serving=rows=>{const current=prompt(rows);served.push(current.prompt.version_id);return current;};
  assert.equal(serving(rows).phase,'study');
  // Learn it, then answer it from memory.
  const quiz=await enterMemory(study,'/?practice=nato');rows=await intact('on Try from memory');assert.equal(serving(rows).phase,'quiz');
  assert.equal((await s.post('/practice/answer',{...nativeFields(quiz,'/practice/answer'),answer:'Alfa'})).status,303);
  rows=await intact('on an answer');
  const result=await s.get('/?practice=nato');has(result,/data-verdict="Good"/,'a Good Verdict');
  assert.equal((await s.post('/practice/next',nativeFields(result,'/practice/next'))).status,303);
  rows=await intact('on Next');serving(rows);
  const second=await enterMemory(study,'/?practice=nato');rows=await intact('on a second Try from memory');
  assert.equal((await s.post('/practice/answer',{...nativeFields(second,'/practice/answer'),answer:'not the code word'})).status,303);
  rows=await intact('on a miss');
  // Study never draws on the earlier notebook: every Prompt it served is of the starter's Version.
  assert.deepEqual([...new Set(served)],versions);
  const attempts=Object.keys(rows).filter(key=>key.startsWith('practice_attempts/')).map(key=>JSON.parse(rows[key]));
  assert.equal(attempts.length,2);assert.ok(attempts.every(attempt=>attempt.version_id===versions[0]));
  assert.equal(Object.keys(rows).filter(key=>key.startsWith('events/')).length,storedApp(state).events.length);
  // A verified backup, then the alarm: still nothing of it is touched or sent.
  const settings=await s.get('/settings');
  assert.equal((await s.post('/settings/backup',nativeFields(settings,'/settings/backup'))).status,303);
  rows=await intact('on a backup');assert.ok(JSON.parse(rows.meta).backup.completed_ms);
  assert.equal(await s.alarm(),200);await intact('on the alarm');
  const exported=await(await s.req('/api/v1/export')).json();
  for(const part of ['goals','concepts','questions','events','overrides','resets','feedback','assessments','jobs','occurrence'])assert.deepEqual(exported.app[part],storedApp(state)[part],`The export's ${part} are the stored ones`);
  has(await s.get('/notebook'),new RegExp(`data-state="${shown(state).view}"`),'the earlier notebook as stored');
  assert.equal(s.jev.sent.length,0);assert.deepEqual(s.jev.elsewhere,[]);
 }finally{await s.close();}
});
