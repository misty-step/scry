import {historicalMutation} from './historical-fixture.mjs';
// Authored synthetic text and native HTTP/CLI clients of the built Worker.
import assert from 'node:assert/strict';
import {feedbackArrival,assertFeedbackArrival,feedbackVariants,fullAnswer} from './feedback-arrival.mjs';
import {phoneHistory,phoneHistoryRecovery,seedHistoryNotebook} from './phone-history.mjs';
import {randomUUID,createHash} from 'node:crypto';
import {spawn} from 'node:child_process';
import {mkdir,mkdtemp,writeFile,rm} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {createRuntime,internalKey} from './runtime-helper.mjs';
export const phoneText='We conceived a patient garden by the water.\n';
export const previousText='The path begins in our synthetic notebook.\n';
export const phoneAnswer='We concieved a tired garden the water extra.';
export async function phoneFixture({port=0,legacy=false}={}) {
 await mkdir(join(process.env.HOME,'.cache/tmp'),{recursive:true});
 const temp=await mkdtemp(join(process.env.HOME,'.cache/tmp/scry-phone-'));
 const name='phone-'+randomUUID(),space='phone-'+randomUUID(),config=join(temp,'login.json');
 let egress=0;
 const base={SCRY_ENV:'development',CANONICAL_ORIGIN:`http://127.0.0.1:${port}`,SPACE_NAME:space,INTERNAL_KEY:internalKey};
 let runtime=await createRuntime({port,name,inspect:true,resourcePersistencePath:join(temp,"state"),bindings:base,egress:async()=>{egress++;throw Error('No egress in phone proof.');}});
 const origin=(await runtime.mf.ready).origin;
 runtime={...runtime,...await runtime.rebind({...base,CANONICAL_ORIGIN:origin})};
 const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
 const rows=async()=>Object.fromEntries((await storage.exec('SELECT key,value FROM records ORDER BY key')).map(r=>[r.key,JSON.parse(r.value)]));
 const record=async key=>JSON.parse((await storage.exec('SELECT value FROM records WHERE key=?',key))[0].value);
 if(legacy)await seedHistoryNotebook({worker:runtime.worker,origin,record});
 const page=await(await runtime.worker.fetch(origin+'/settings')).text();
 const form=/<form[^>]*action="\/tokens\/create"[^>]*>([\s\S]*?)<\/form>/.exec(page)[1];
 const fields=Object.fromEntries([...form.matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
 const minted=await runtime.worker.fetch(origin+'/tokens/create',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,name:'Synthetic phone proof; unpaid'})});
 assert.equal(minted.status,200);
 const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())[1];
 const cli=(args,input='',login=config)=>new Promise((done,reject)=>{
  const child=spawn(resolve(process.env.SCRY_STUDY_CLI||'target/debug/scry'),[...args,'--json','--config',login],{env:{PATH:process.env.PATH,HOME:temp},stdio:['pipe','pipe','pipe']});let output='',error='';
  child.stdout.on('data',s=>output+=s);child.stderr.on('data',s=>error+=s);child.on('error',reject);child.on('exit',code=>{try{assert.equal(code,0,'Compiled CLI refused synthetic operation.');assert.ok(!output.includes(secret)&&!error.includes(secret));done(JSON.parse(output));}catch(e){reject(e);}});child.stdin.end(input);
 });
 await cli(['login','--url',origin,'--token-stdin'],secret+'\n');
 const file=join(temp,'text.txt'),sections=join(temp,'sections.json');await writeFile(file,previousText+phoneText);await writeFile(sections,JSON.stringify([{heading:'Synthetic first article',units:[previousText,phoneText]}]));
 const vid=(await cli(['version','add','--title',"Apostles' Creed",'--kind','text','--name','Roman Missal 2011 metadata; synthetic wording','--source','Authored synthetic test, not canonical text','--text-file',file,'--sections-file',sections])).result.entity_id;
 const sid=(await cli(['learn',vid])).practice.study.id;
 let current=(await cli(['next'])).practice;
 let cid;
 for(const text of [previousText,phoneText]){
  cid=(await record('meta')).practice.current.targets[0].card_id;
  // Fade via actual assisted answers. No direct SQLite writes or test clock.
  for(let i=0;i<6;i++){
   await cli(['answer',current.prompt.id,'--text-stdin'],text);
   current=(await cli(['next','--after',current.prompt.id,'--card',cid])).practice;
  }
  assert.equal(current.prompt.cue_level,'previous_line');
  if(text===previousText){await cli(['answer',current.prompt.id,'--text-stdin'],text);current=(await cli(['next','--after',current.prompt.id])).practice;}
 }
 assert.equal(current.prompt.context,previousText);
 const historicalCorrection=async fields=>{
  const payload={operation_id:randomUUID(),...fields};const before=await rows();
  const response=await runtime.worker.fetch(origin+'/api/v1/correct',{method:'POST',headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'},body:JSON.stringify(payload)});
  assert.equal(response.status,410);assert.deepEqual(await rows(),before,'Retired correction cannot write');
  return historicalMutation(storage,{path:'/practice/correct',fields:payload,now_ms:Date.now(),seed:payload.operation_id});
 };
 return {...runtime,origin,cli,historicalCorrection,rows,record,vid,sid,cid,prompt:current.prompt,get egress(){return egress;},
  fork:async()=>{
   const ns=await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE');
   const backed=await ns.get(ns.idFromName(space)).fetch('https://internal.invalid/__internal/daily',{method:'POST',headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});
   assert.equal(backed.status,200);
   const bucket=await runtime.mf.getR2Bucket('SCRY_BACKUPS'),list=await bucket.list({prefix:`snapshots/${space}/`});
   const bytes=Buffer.from(await(await bucket.get(list.objects.at(-1).key)).arrayBuffer());
   const sha256=createHash('sha256').update(bytes).digest('hex');
   const restoreToken='synthetic-phone-isolated-restore-only-32chars';
   const bindings={SCRY_ENV:'restore',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:'phone-clone-'+randomUUID(),INTERNAL_KEY:internalKey,RESTORE_TOKEN:restoreToken};
   let clone=await createRuntime({port:0,name:'phone-clone-'+randomUUID(),bindings});
   const url=(await clone.mf.ready).origin;
   clone={...clone,...await clone.rebind({...bindings,CANONICAL_ORIGIN:url})};
   const r=await clone.worker.fetch(url+'/operator/restore',{method:'POST',headers:{'x-scry-restore-token':restoreToken,'x-scry-sha256':sha256},body:bytes});assert.equal(r.status,200);
   clone={...clone,...await clone.rebind({...bindings,SCRY_ENV:'development',CANONICAL_ORIGIN:url})};
   const login=join(temp,'clone-login.json');await cli(['login','--url',url,'--token-stdin'],secret+'\n',login);
   return {cli:(args,input)=>cli(args,input,login),sha256,close:()=>clone.mf.dispose()};
  },
  close:async()=>{await runtime.mf.dispose();await rm(temp,{recursive:true,force:true});}};
}

export async function phoneWalk(browser,capture) {
 const results=[];
 for(const [width,scheme,noJS,zoom] of feedbackVariants){
  const f=await phoneFixture();
  const context=await browser.newContext({viewport:{width,height:900},colorScheme:scheme,javaScriptEnabled:!noJS,...(width<768?{isMobile:true,hasTouch:true}:{})});
  const page=await context.newPage();page.setDefaultTimeout(12000);
  let clone;
  const row={width,scheme,noJS,text_zoom:zoom?'200%':'100%',status:'pass',criteria:[],evidence:[]};results.push(row);
  const record=async(name)=>row.evidence.push(await capture(page,`phone-${width}-${scheme}-${zoom?200:100}-${name}.png`));
  const typography=async()=>{
   if(zoom)await page.evaluate(()=>{for(const node of document.querySelectorAll('[data-qa-text-size]')){node.style.removeProperty('font-size');delete node.dataset.qaTextSize;}for(const el of document.querySelectorAll('body,body *')){const size=parseFloat(getComputedStyle(el).fontSize);el.dataset.qaTextSize=String(size*2);}for(const el of document.querySelectorAll('[data-qa-text-size]'))el.style.fontSize=el.dataset.qaTextSize+'px';});
   await page.evaluate(()=>document.fonts.ready);
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Horizontal overflow');
  };
  try{
   await page.goto(f.origin+'/recall');await page.locator('[data-state="exact-prompt"]').waitFor();
   assert.equal(await page.locator('textarea').count(),1);assert.equal(await page.getByRole('button',{name:'Check my answer',exact:true}).count(),1);
   assert.deepEqual(await page.locator('.masthead nav a').allTextContents(),['Practice','Choose a lesson','Library','History','Settings','Earlier practice']);
   assert.ok(!(await page.locator('main').innerText()).includes(phoneText.trim()));assert.equal((await page.locator('.exact-context .reading').innerText()).trim(),previousText.trim());
   await typography();await record('prompt');row.criteria.push(1);
   clone=await f.fork();
   await page.setViewportSize({width,height:844});
   await page.getByLabel('Your answer',{exact:true}).fill(phoneAnswer);
   // Submit with a real keyboard, including the no-JavaScript path.
   await page.getByRole('button',{name:'Check my answer',exact:true}).focus();await page.keyboard.press('Enter');
   await page.locator('[data-state="exact-result"]').waitFor();
   row.arrival=await feedbackArrival(page,capture,`phone-${width}-${scheme}-${zoom?200:100}-arrival.png`,zoom);
   row.evidence.push(row.arrival.evidence);
   assertFeedbackArrival(row.arrival);
   await page.setViewportSize({width,height:900});
   const held=(await f.record('meta')).practice.current;
   const a=(await f.cli(['next'])).practice.attempt;
   assert.equal(a.id,held.attempt_id);assert.equal(a.prompt.version_id,f.vid);assert.deepEqual(a.prompt,f.prompt);
   assert.equal(await page.locator('[data-verdict]').getAttribute('data-verdict'),a.grade.verdict);
   assert.deepEqual(await page.locator('[data-diff-kind]').evaluateAll(nodes=>nodes.map(n=>n.dataset.diffKind)),a.grade.diff.map(d=>d.kind));
   assert.deepEqual(new Set(a.grade.diff.map(d=>d.kind)),new Set(['correct','slip','wrong','missing','extra']));
   const cp=(await clone.cli(['next'])).practice.prompt;
   assert.deepEqual(cp,f.prompt);
   const ca=(await clone.cli(['answer',cp.id,'--text-stdin'],phoneAnswer)).practice.attempt;
   assert.deepEqual(ca.prompt,a.prompt);assert.deepEqual(ca.grade,a.grade);
   row.parity={version:f.vid,version_hash:f.prompt.version_hash,unit_ids:f.prompt.unit_ids,prompt:f.prompt,answer:phoneAnswer,phone_grade:a.grade,cli_grade:ca.grade,snapshot_sha256:clone.sha256,method:'Exact frozen Prompt restored from complete local R2 archive into isolated clone; compiled scry answer --json.'};
   await clone.close();clone=null;
   await page.reload();await page.locator('[data-state="exact-result"]').waitFor();
   assert.equal((await f.record('meta')).practice.current.attempt_id,a.id);
   await typography();
   const closeFull=await fullAnswer(page,f,a,phoneText);
   await record('diff');await closeFull();row.criteria.push(2);
   assert.equal(await page.getByRole('button',{name:'I was right',exact:true}).count(),0);
   await f.historicalCorrection({prompt_id:a.prompt.id,attempt_id:a.id,verdict:'Good'});await page.reload();await page.getByText('Owner Correction',{exact:false}).waitFor();
   assert.deepEqual((await f.cli(['next'])).practice.attempt,a,'Original Attempt stays immutable.');
   assert.equal(Object.keys(await f.rows()).filter(k=>k.startsWith('practice_corrections/')).length,1);row.criteria.push(5);
   await page.getByRole('button',{name:'Next',exact:true}).click();
   await page.locator('[data-state="exact-caught-up"]').waitFor();
   const due=(await f.cli(['due'])).next_due_ms;
   assert.ok(due>Date.now());assert.ok((await page.locator('main').innerText()).includes('Next due at'));
   assert.equal(await page.locator('textarea').count(),0);await record('caught-up');row.criteria.push(4);
   await f.cli(['next','--card',f.cid]);await page.reload();await page.locator('[data-state="exact-prompt"]').waitFor();
   const before=(await f.record('practice_cards/'+f.cid)).state;
   const attempts=Object.keys(await f.rows()).filter(k=>k.startsWith('practice_attempts/')).length;
   await page.locator('details.overflow>summary').click();await page.getByRole('button',{name:'Show me',exact:true}).click();
   await page.getByText('Assistance is recorded.',{exact:true}).waitFor();
   assert.deepEqual((await f.record('practice_cards/'+f.cid)).state,before);
   assert.equal(Object.keys(await f.rows()).filter(k=>k.startsWith('practice_attempts/')).length,attempts+1);
   assert.ok(Object.values(await f.rows()).some(v=>v?.disclosure===true&&v?.assisted===true));
   await record('shown');row.criteria.push(3);
   await phoneHistory({f,page,row,original:a,typography,record});
   assert.equal(f.egress,0);
  }catch(e){row.status='fail';row.reason=e.message;await record('failure');}
  finally{await clone?.close();await context.close();await f.close();}
 }
 return results;
}

export async function phoneRecovery(browser,capture) {
 const results=[];
 const check=async(name,run)=>{
  const f=await phoneFixture(),context=await browser.newContext({viewport:{width:390,height:844}}),page=await context.newPage();page.setDefaultTimeout(12000);
  const row={name,status:'pass'};results.push(row);
  try{await page.goto(f.origin+'/recall');await run({f,context,page,row});assert.equal(f.egress,0);}
  catch(e){row.status='fail';row.reason=e.message;}
  finally{row.evidence=await capture(page,`phone-recovery-${results.length}.png`);await context.close();await f.close();}
 };
 const answer=async page=>{await page.getByLabel('Your answer',{exact:true}).fill(phoneAnswer);await page.getByRole('button',{name:'Check my answer',exact:true}).click();};
 const count=async f=>Object.keys(await f.rows()).filter(k=>k.startsWith('practice_attempts/')).length;
 await check('unknown response reconciles one frozen answer',async({f,context,page,row})=>{
  const before=await count(f),payloads=[];let first=true;
  await context.route('**/practice/answer',async route=>{if(route.request().method()!=='POST')return route.continue();payloads.push(route.request().postData());if(first){first=false;assert.equal((await route.fetch({maxRedirects:0})).status(),303);await route.abort('failed');}else await route.continue();});
  await answer(page);await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();
  assert.equal(await page.getByLabel('Your answer',{exact:true}).isEditable(),false);
  assert.equal(await count(f),before+1);
  await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).click();await page.locator('[data-state="exact-result"]').waitFor();
  const field=(body,name)=>body.match(new RegExp('name="'+name+'"\\r\\n\\r\\n([^\\r]+)'))?.[1];
  for(const name of ['operation_id','prompt_id','revision','answer']){assert.ok(field(payloads[0],name));assert.equal(field(payloads[0],name),field(payloads[1],name));}
  assert.equal(await count(f),before+1);row.observed={sends:payloads.length,new_attempts:1,frozen_payload_equal:true};
 });
 await check('competing stale tabs keep one Attempt',async({f,context,page,row})=>{
  const before=await count(f),other=await context.newPage();await other.goto(f.origin+'/recall');await answer(page);await page.locator('[data-state="exact-result"]').waitFor();
  await answer(other);await other.getByText('This changed in another tab. Your draft is still here.',{exact:true}).waitFor();assert.equal(await count(f),before+1);row.observed={new_attempts:1,stale_refusal:409};await other.close();
 });
 await check('offline pauses and reconnect sends nothing',async({f,context,page,row})=>{
  let sends=0;context.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/practice/answer'))sends++;});
  await page.getByLabel('Your answer',{exact:true}).fill(phoneAnswer);await context.setOffline(true);await page.getByRole('button',{name:'Check my answer',exact:true}).click();
  assert.equal(sends,0);assert.equal(await page.getByLabel('Your answer',{exact:true}).inputValue(),phoneAnswer);await context.setOffline(false);await page.waitForTimeout(400);assert.equal(sends,0);
  await page.getByRole('button',{name:'Check my answer',exact:true}).click();await page.locator('[data-state="exact-result"]').waitFor();assert.equal(sends,1);row.observed={offline_sends:0,reconnect_sends:0,deliberate_sends:1};
 });
 await check('automatic Good refuses new self-rating and retains a historical Correction fixture',async({f,page,row})=>{
  await page.getByLabel('Your answer',{exact:true}).fill(phoneText);await page.getByRole('button',{name:'Check my answer',exact:true}).click();await page.locator('[data-verdict="Good"]').waitFor();
  const original=(await f.cli(['next'])).practice.attempt;
  assert.equal(await page.getByRole('button',{name:'Count as a miss',exact:true}).count(),0);
  await f.historicalCorrection({prompt_id:original.prompt.id,attempt_id:original.id,verdict:'Again'});await page.reload();await page.getByText('Owner Correction',{exact:false}).waitFor();
  assert.deepEqual((await f.cli(['next'])).practice.attempt,original);assert.equal((await f.record('meta')).practice.current.attempt_id,original.id);
  assert.equal((await f.record('practice_cards/'+f.cid)).state.schedule.reps,1);row.observed={original_verdict:'Good',historical_owner_verdict:'Again',new_self_rating_refused:410,held_attempt_unchanged:true};
 });
 await check('access loss removes private DOM',async({context,page,row})=>{
  await context.route('**/practice/answer',route=>route.fulfill({status:401,contentType:'text/plain',body:'Owner authorization expired.'}));await answer(page);await page.locator('body.private-hidden').waitFor({state:'attached'});assert.equal(await page.locator('#main').count(),0);row.observed={private_main_present:false};
 });
 results.push(...await phoneHistoryRecovery(browser,capture,phoneFixture,{previousText,phoneText,phoneAnswer}));
 return results;
}
