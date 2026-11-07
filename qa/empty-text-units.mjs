// Actual authoring -> source -> native quiz, plus pre-fix synthetic archive
// restore. No private notebook, provider call, or client-supplied grade.
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createHash,randomUUID} from 'node:crypto';
import {createRuntime,internalKey} from './runtime-helper.mjs';
import {nativeFields} from './nato-starter.mjs';
const now=1800000000000;
const legacy=JSON.parse(await readFile(new URL('../tests/fixtures/empty-unit-legacy.json',import.meta.url),'utf8'));
async function fixture(state,run){
 const name='empty-text-'+randomUUID();let egress=0;
 const bindings={SCRY_ENV:state?'restore':'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey,SCRY_TEST_NOW_MS:String(now),...(state?{RESTORE_TOKEN:'synthetic-empty-unit-restore-32chars'}:{})};
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
  const post=(path,fields)=>runtime.worker.fetch(origin+path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(fields)});
  const get=async path=>{const response=await runtime.worker.fetch(origin+path);assert.equal(response.status,200);return response.text();};
  await run({runtime,origin,rows,post,get});
 }finally{await runtime.mf.dispose();assert.equal(egress,0);}
}
export async function emptyTextJourney(){
 const report=[];
 for(const text of ['\nAlpha beta.','... Alpha beta.','Alpha... beta?!']){
  await fixture(null,async({runtime,origin,rows,post,get})=>{
   const issued=await post('/tokens/create',{...nativeFields(await get('/settings'),'/tokens/create'),name:'Synthetic empty Unit QA'});
   assert.equal(issued.status,200);
   const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())?.[1];assert.ok(secret);
   const api=async(path,payload,operation_id=randomUUID(),status=200)=>{
    const response=await runtime.worker.fetch(origin+path,{method:'POST',headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'},body:JSON.stringify({operation_id,...payload})});
    const body=await response.json();assert.equal(response.status,status,JSON.stringify(body));return body;
   };
   const version_id=(await api('/api/v1/library/versions',{data:{title:'Synthetic separators',kind:'text',text}})).result.entity_id;
   const source=(await api('/api/v1/lessons/start',{version_id})).practice;
   assert.equal(source.phase,'study');assert.match(source.prompt.cue,/Alpha/);
   const quiz=(await api('/api/v1/quiz',{prompt_id:source.prompt.id})).practice;
   const operation=randomUUID(),payload={prompt_id:quiz.prompt.id,answer:''};
   const result=await api('/api/v1/answer',payload,operation);
   assert.equal(result.practice.attempt.grade.verdict,'Again');
   assert.equal(result.practice.attempt.assisted,false);
   assert.ok(result.practice.attempt.effects.every(e=>e.after.successes===0));
   const saved=await rows(),practice=saved.filter(r=>r.key.startsWith('practice_'));
   const replay=await api('/api/v1/answer',payload,operation);assert.equal(replay.replayed,true);
   assert.equal(replay.practice.attempt.id,result.practice.attempt.id);
   assert.deepEqual((await rows()).filter(r=>r.key.startsWith('practice_')),practice);
   const before=await rows();
   const invalid={title:'Invalid authored boundaries',kind:'text',text:' \nGamma delta.',sections:[{heading:'Passage',units:[' \n','Gamma delta.']}]};
   await api('/api/v1/library/versions',{data:invalid},randomUUID(),422);
   assert.deepEqual(await rows(),before);
   report.push({text,verdict:'Again',unaided:true,attempts:practice.filter(r=>r.key.startsWith('practice_attempts/')).length,explicitRejected:true,replayPreserved:true});
  });
 }
 for(const state of ['source','quiz','whole_quiz','answered']){
  await fixture(state,async({rows,post,get})=>{
   const before=await rows(),meta=JSON.parse(before.find(r=>r.key==='meta').value),prompt=meta.practice.current.prompt;
   const heldPage=await get('/recall');
   assert.match(heldPage,/data-state="practice-unavailable"/);assert.ok(heldPage.includes(prompt.id));
   assert.doesNotMatch(heldPage,/action="\/practice\/(answer|quiz)"/);
   assert.deepEqual(await rows(),before,'Reading the restored held prompt does not repair it.');
   const fields={csrf:meta.csrf,operation_id:randomUUID(),revision:String(meta.revision),prompt_id:prompt.id};
   const checks=state==='source'?[
    ['/practice/quiz',fields],['/practice/reveal',fields],['/practice/next',{...fields,prompt_id:undefined}],
   ]:state==='answered'?[
    ['/practice/lesson/next',{...fields,after:prompt.id,mode:'whole'}],
    ['/practice/next',{...fields,after:prompt.id,study_id:prompt.study_id,recite:'section',section_id:legacy[state].library.versions[prompt.version_id].sections[0].id}],
   ]:[['/practice/answer',{...fields,answer:''}],['/practice/answer',{...fields,answer:'Alpha beta.'}],['/practice/reveal',fields]];
   for(const [path,extra] of checks){
    const request=Object.fromEntries(Object.entries(extra).filter(([,v])=>v!==undefined));
    // Next accepts only its actual scope/fence fields, not prompt_id.
    if(path.endsWith('/next'))delete request.prompt_id;
    const response=await post(path,request);assert.equal(response.status,422,`${state} ${path}: ${await response.text()}`);
    assert.deepEqual(await rows(),before,`${state} ${path} must not write any row.`);
   }
   if(state==='answered'){
    const replay=await post('/practice/answer',{...legacy.requests.answer,csrf:meta.csrf});assert.equal(replay.status,303);
    const after=await rows();
    assert.deepEqual(after.filter(r=>r.key!=='meta'),before.filter(r=>r.key!=='meta'));
    const nextMeta=JSON.parse(after.find(r=>r.key==='meta').value);assert.equal(nextMeta.revision,meta.revision+1);nextMeta.revision=meta.revision;assert.deepEqual(nextMeta,meta);
   }
   const saved=await rows();
   const escape=nativeFields(await get('/recall'),'/practice/review/start');
   assert.equal(escape[state==='answered'?'after':'leave'],prompt.id);
   const escaped=await post('/practice/review/start',escape);assert.equal(escaped.status,303);
   const after=await rows(),current=JSON.parse(after.find(r=>r.key==='meta').value).practice.current;
   assert.notEqual(current.prompt.version_id,prompt.version_id);
   for(const row of saved.filter(r=>r.key.startsWith('library_')||r.key.startsWith('practice_attempts/')||r.key.startsWith('practice_payload/')||r.key.startsWith('practice_cards/')))assert.deepEqual(after.find(r=>r.key===row.key),row,'Escape preserves old content, cards and attempts.');
   report.push({legacyState:state,restored:true,rejectedWithoutWrites:checks.length,historicalReplay:state==='answered',nativeEscape:true});
  });
 }
 return {sourceFixture:legacy.source_commit,cases:report,externalCalls:0};
}

export async function emptyTextWalk(browser,capture){
 const report=[];
 for(const [width,noJS]of [[320,true],[1280,false]]){
  await fixture('quiz',async({origin,rows,post})=>{
   const meta=JSON.parse((await rows()).find(r=>r.key==='meta').value),prompt=meta.practice.current.prompt;
   const draft='Keep my saved answer before switching.';
   assert.equal((await post('/practice/draft',{csrf:meta.csrf,operation_id:randomUUID(),prompt_id:prompt.id,generation:'0',answer:draft})).status,303);
   const before=await rows(),context=await browser.newContext({viewport:{width,height:844},javaScriptEnabled:!noJS,isMobile:noJS,hasTouch:noJS,colorScheme:noJS?'dark':'light'});
   try{
    const page=await context.newPage();page.setDefaultTimeout(12000);
    await page.goto(origin+'/recall');
    await page.locator('[data-state="practice-unavailable"]').waitFor();
    assert.equal(await page.locator('form[action="/practice/answer"],form[action="/practice/quiz"]').count(),0);
    assert.equal(await page.locator('#unavailable-draft').inputValue(),draft);
    assert.equal(await page.locator('#unavailable-draft').getAttribute('readonly'),'');
    assert.equal(await page.locator('form[action="/practice/review/start"] input[name="leave"]').inputValue(),prompt.id);
    assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'No horizontal overflow.');
    await capture(page,`empty-unit-unavailable-${width}${noJS?'-native':''}.png`);
    const button=page.getByRole('button',{name:'Study other material'});
    if(noJS)await button.tap();else{await button.focus();await page.keyboard.press('Enter');}
    await page.waitForFunction(()=>!document.querySelector('[data-state="practice-unavailable"]'));
    const after=await rows();
    assert.notEqual(JSON.parse(after.find(r=>r.key==='meta').value).practice.current.prompt.id,prompt.id);
    for(const row of before.filter(r=>r.key.startsWith('library_')||r.key.startsWith('practice_attempts/')||r.key.startsWith('practice_payload/')||r.key.startsWith('practice_cards/')))assert.deepEqual(after.find(r=>r.key===row.key),row);
    assert.equal(after.filter(r=>r.key.startsWith('practice_attempts/')).length,0);
    await page.reload();assert.equal(await page.locator('[data-state="practice-unavailable"]').count(),0);
    await capture(page,`empty-unit-escaped-${width}${noJS?'-native':''}.png`);
    report.push({status:'pass',width,noJS,copyableDraft:true,nativeFence:true,escapeWithoutAttempt:true,reload:true});
   }finally{await context.close();}
  });
 }
 return report;
}
