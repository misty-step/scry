// Native supplied text -> source -> quiz -> authored Whole challenge. Legacy
// snapshots come from real pre-fix mutations, never a test-only state setter.
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createHash,randomUUID} from 'node:crypto';
import {createRuntime,internalKey} from './runtime-helper.mjs';
import {nativeFields} from './nato-starter.mjs';
const now=1800000000000;
const legacy=JSON.parse(await readFile(new URL('../tests/fixtures/split-unit-legacy.json',import.meta.url),'utf8'));
async function fixture(state,run){
 const name='split-text-'+randomUUID();let egress=0;
 const bindings={SCRY_ENV:state?'restore':'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey,SCRY_TEST_NOW_MS:String(now),...(state?{RESTORE_TOKEN:'synthetic-split-unit-restore-32chars'}:{})};
 let runtime=await createRuntime({name,port:0,inspect:true,testClock:true,bindings,egress:async()=>{egress++;throw Error('External egress forbidden.');}});
 try{
  const origin=(await runtime.mf.ready).origin;
  runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin})};
  if(state){
   const archive={format:'scry-rust-durable-sqlite-study-phase-v3',schema:1,algorithm:'go-fsrs/v4.0.0;defaults-v1;retention=.9;fuzz=false;steps=1m,10m;relearn=10m;grading=exact-v1',created_ms:now,app:legacy[state],photos:[]};
   const bytes=Buffer.from(JSON.stringify(archive));
   const restored=await runtime.worker.fetch(origin+'/operator/restore',{method:'POST',headers:{'x-scry-restore-token':bindings.RESTORE_TOKEN,'x-scry-sha256':createHash('sha256').update(bytes).digest('hex')},body:bytes});
   assert.equal(restored.status,200,await restored.text());
   const {RESTORE_TOKEN,...development}=bindings;
   runtime={...runtime,...await runtime.rebind({...development,SCRY_ENV:'development',CANONICAL_ORIGIN:origin})};
  }
  const store=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
  const rows=()=>store.exec('SELECT key,value FROM records ORDER BY key');
  const post=(path,fields,headers={})=>runtime.worker.fetch(origin+path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded',...headers},body:new URLSearchParams(fields)});
  const get=async path=>{const response=await runtime.worker.fetch(origin+path);assert.equal(response.status,200);return response.text();};
  await run({runtime,origin,rows,post,get});
 }finally{await runtime.mf.dispose();assert.equal(egress,0);}
}
export async function splitTextJourney(){
 const report=[];
 for(const text of ['Use 3.14 for pi.','Wait...really?','Alpha,beta.','Alpha, beta.','Straße,Äpfel.\nUse ٣.١٤.','\n...Alpha,beta. Next; last.']){
  await fixture(null,async({runtime,origin,rows,post,get})=>{
   const issued=await post('/tokens/create',{...nativeFields(await get('/settings'),'/tokens/create'),name:'Synthetic split Unit QA'});
   assert.equal(issued.status,200);
   const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())?.[1];assert.ok(secret);
   const api=async(path,payload,operation_id=randomUUID(),status=200)=>{
    const response=await runtime.worker.fetch(origin+path,{method:'POST',headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'},body:JSON.stringify({operation_id,...payload})});
    const body=await response.json();assert.equal(response.status,status,JSON.stringify(body));return body;
   };
   const version_id=(await api('/api/v1/library/versions',{data:{title:'Synthetic boundaries',kind:'text',text}})).result.entity_id;
   const version=(await rows()).find(r=>r.key==='library_versions/'+version_id);
   const units=JSON.parse(version.value).sections.flatMap(s=>s.units).map(u=>u.text);
   assert.equal(units.join(''),text);
   const source=(await api('/api/v1/lessons/start',{version_id})).practice;
   assert.equal(source.phase,'study');
   const beforeSourceAnswer=await rows();
   await api('/api/v1/answer',{prompt_id:source.prompt.id,answer:source.prompt.cue},randomUUID(),409);
   assert.deepEqual(await rows(),beforeSourceAnswer);
   const quiz=(await api('/api/v1/quiz',{prompt_id:source.prompt.id})).practice;
   const first=(await api('/api/v1/answer',{prompt_id:quiz.prompt.id,answer:source.prompt.cue})).practice.attempt;
   assert.equal(first.grade.verdict,'Good');
   const whole=(await api('/api/v1/lessons/next',{after:quiz.prompt.id,mode:'whole'})).practice;
   const operation=randomUUID(),payload={prompt_id:whole.prompt.id,answer:text};
   const result=await api('/api/v1/answer',payload,operation),attempt=result.practice.attempt;
   assert.equal(attempt.grade.verdict,'Good');assert.equal(attempt.assisted,false);
   assert.deepEqual(attempt.grade.authority,{name:'exact-v2',grader_version:'1;unicode=17.0.0'});
   assert.ok(attempt.grade.units.every(u=>u.verdict==='Good'));assert.ok(attempt.effects.every(e=>e.verdict==='Good'));
   assert.ok(attempt.grade.diff.every(d=>d.kind==='correct'));
   const saved=await rows(),practice=saved.filter(r=>r.key.startsWith('practice_'));
   const replay=await api('/api/v1/answer',payload,operation);assert.equal(replay.replayed,true);
   assert.equal(replay.practice.attempt.id,attempt.id);
   assert.deepEqual((await rows()).filter(r=>r.key.startsWith('practice_')),practice);
   assert.deepEqual((await rows()).find(r=>r.key===version.key),version);
   const before=await rows();
   const invalid={title:'Invalid authored boundaries',kind:'text',text:'Alpha,beta.',sections:[{heading:'One',units:['Alpha,']},{heading:'Two',units:['beta.']}]};
   await api('/api/v1/library/versions',{data:invalid},randomUUID(),422);
   assert.deepEqual(await rows(),before);
   report.push({text,units,sourceAnswerRefused:true,firstPhrase:'Good',whole:'Good',unaided:true,explicitAcrossSectionsRejected:true,replayPreserved:true});
  });
 }
 for(const state of ['source','quiz','answered','whole_quiz','whole_answered']){
  await fixture(state,async({rows,post,get})=>{
   let before=await rows(),meta=JSON.parse(before.find(r=>r.key==='meta').value),prompt=meta.practice.current.prompt;
   let draft;
   if(state==='quiz'){
    draft='Keep this decimal answer: 3.14';
    const scope=JSON.parse(await get('/session/scope')).scope;
    const saved=await post('/practice/draft',{csrf:meta.csrf,operation_id:randomUUID(),prompt_id:prompt.id,generation:'0',answer:draft},{'X-Scry-Request':'page','X-Scry-Expected-Scope':scope});
    assert.equal(saved.status,303,await saved.text());
    before=await rows();meta=JSON.parse(before.find(r=>r.key==='meta').value);
   }
   const page=await get('/recall');
   assert.match(page,/data-state="practice-unavailable"/);assert.match(page,/boundaries split a word/);
   assert.doesNotMatch(page,/no words to recall|action="\/practice\/(answer|quiz)"/);
   if(draft)assert.ok(page.includes(`<textarea id="unavailable-draft" readonly>${draft}</textarea>`));
   assert.deepEqual(await rows(),before);
   const fields={csrf:meta.csrf,operation_id:randomUUID(),revision:String(meta.revision),prompt_id:prompt.id};
   const answered=state==='answered'||state==='whole_answered';
   const checks=answered?[
    ['/practice/lesson/next',{...fields,after:prompt.id,mode:'whole'}],
    ['/practice/next',{...fields,after:prompt.id,study_id:prompt.study_id,recite:'section',section_id:legacy[state].library.versions[prompt.version_id].sections[0].id}],
   ]:[['/practice/answer',{...fields,answer:'Use 3.14 for pi.'}],['/practice/reveal',fields],...(state==='source'?[['/practice/quiz',fields]]:[])];
   for(const [path,extra] of checks){
    const request={...extra};if(path.endsWith('/next'))delete request.prompt_id;
    const response=await post(path,request);assert.equal(response.status,422,`${state} ${path}: ${await response.text()}`);
    assert.deepEqual(await rows(),before,`${state} ${path} must not write any row.`);
   }
   const requestName={source:'start',quiz:'quiz',answered:'answer',whole_quiz:'whole',whole_answered:'whole_answer'}[state];
   const replayPath={source:'/practice/lesson/start',quiz:'/practice/quiz',answered:'/practice/answer',whole_quiz:'/practice/lesson/next',whole_answered:'/practice/answer'}[state];
   const replay=await post(replayPath,{...legacy.requests[requestName],csrf:meta.csrf});assert.equal(replay.status,303,await replay.text());
   const replayed=await rows();assert.deepEqual(replayed.filter(r=>r.key!=='meta'),before.filter(r=>r.key!=='meta'));
   const nextMeta=JSON.parse(replayed.find(r=>r.key==='meta').value);assert.equal(nextMeta.revision,meta.revision+1);nextMeta.revision=meta.revision;assert.deepEqual(nextMeta,meta);
   if(state==='whole_answered'){
    const historical=legacy[state].practice.attempts[legacy[state].practice.current.attempt_id];
    assert.equal(historical.grade.verdict,'Again');assert.ok(historical.effects.some(e=>e.scheduled&&e.verdict==='Again'));
   }
   const saved=await rows(),escape=nativeFields(await get('/recall'),'/practice/review/start');
   assert.equal(escape[answered?'after':'leave'],prompt.id);
   assert.equal((await post('/practice/review/start',escape)).status,303);
   const after=await rows();assert.notEqual(JSON.parse(after.find(r=>r.key==='meta').value).practice.current.prompt.version_id,prompt.version_id);
   for(const row of saved.filter(r=>r.key.startsWith('library_')||r.key.startsWith('practice_attempts/')||r.key.startsWith('practice_payload/')||r.key.startsWith('practice_cards/')))assert.deepEqual(after.find(r=>r.key===row.key),row);
   report.push({legacyState:state,restored:true,rejectedWithoutWrites:checks.length,receiptReplayed:true,originalEvidencePreserved:true,nativeEscape:true,...(draft?{scopedDraftPreserved:true}:{})});
  });
 }
 return {sourceFixture:legacy.source_commit,cases:report,externalCalls:0};
}

export async function splitTextWalk(browser,capture){
 const report=[];
 for(const [width,noJS]of [[320,true],[1280,false]]){
  await fixture('quiz',async({origin,rows,post,get})=>{
   const meta=JSON.parse((await rows()).find(r=>r.key==='meta').value),prompt=meta.practice.current.prompt;
   const draft='Keep this decimal answer: 3.14',scope=JSON.parse(await get('/session/scope')).scope;
   assert.equal((await post('/practice/draft',{csrf:meta.csrf,operation_id:randomUUID(),prompt_id:prompt.id,generation:'0',answer:draft},{'X-Scry-Request':'page','X-Scry-Expected-Scope':scope})).status,303);
   const before=await rows(),context=await browser.newContext({viewport:{width,height:844},javaScriptEnabled:!noJS,isMobile:noJS,hasTouch:noJS,colorScheme:noJS?'dark':'light'});
   try{
    const page=await context.newPage();page.setDefaultTimeout(12000);
    await page.goto(origin+'/recall');
    await page.locator('[data-state="practice-unavailable"]').waitFor();
    assert.match(await page.locator('[data-state="practice-unavailable"]').innerText(),/boundaries split a word/);
    assert.equal(await page.locator('form[action="/practice/answer"],form[action="/practice/quiz"]').count(),0);
    assert.equal(await page.locator('#unavailable-draft').inputValue(),draft);
    assert.equal(await page.locator('#unavailable-draft').getAttribute('readonly'),'');
    assert.equal(await page.locator('form[action="/practice/review/start"] input[name="leave"]').inputValue(),prompt.id);
    assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
    await capture(page,`split-unit-unavailable-${width}${noJS?'-native':''}.png`);
    await page.reload();assert.equal(await page.locator('#unavailable-draft').inputValue(),draft);
    const button=page.getByRole('button',{name:'Study other material'});
    if(noJS)await button.tap();else{await button.focus();await page.keyboard.press('Enter');}
    await page.waitForFunction(()=>!document.querySelector('[data-state="practice-unavailable"]'));
    const after=await rows();assert.notEqual(JSON.parse(after.find(r=>r.key==='meta').value).practice.current.prompt.id,prompt.id);
    for(const row of before.filter(r=>r.key.startsWith('library_')||r.key.startsWith('practice_attempts/')||r.key.startsWith('practice_payload/')||r.key.startsWith('practice_cards/')))assert.deepEqual(after.find(r=>r.key===row.key),row);
    assert.equal(after.filter(r=>r.key.startsWith('practice_attempts/')).length,0);
    await page.reload();assert.equal(await page.locator('[data-state="practice-unavailable"]').count(),0);
    await capture(page,`split-unit-escaped-${width}${noJS?'-native':''}.png`);
    report.push({status:'pass',width,noJS,copyableDraft:true,nativeFence:true,escapeWithoutAttempt:true,reload:true});
   }finally{await context.close();}
  });
 }
 return report;
}
