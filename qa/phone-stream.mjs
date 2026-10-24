// Authored synthetic text and native HTTP/CLI clients of the built Worker.
import assert from 'node:assert/strict';
import {feedbackArrival,assertFeedbackArrival,feedbackVariants,fullAnswer} from './feedback-arrival.mjs';
import {randomUUID,createHash} from 'node:crypto';
import {spawn} from 'node:child_process';
import {mkdtemp,writeFile,rm} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {createRuntime,internalKey} from './runtime-helper.mjs';
export const phoneText='We conceived a patient garden by the water.\n';
export const previousText='The path begins in our synthetic notebook.\n';
export const phoneAnswer='We concieved a tired garden the water extra.';
const recitalSections=[{heading:'At the garden gate',units:['We conceived a patient garden.\n','A gentle silver orchard.\n']},{heading:'Beside the stream',units:['Lanterns guard our quiet path.\n']}];
export async function phoneFixture({port=0,recitation}={}) {
 const temp=await mkdtemp(join(process.env.HOME,'.cache/tmp/scry-phone-'));
 const name='phone-'+randomUUID(),space='phone-'+randomUUID(),config=join(temp,'login.json');
 let egress=0;
 const base={SCRY_ENV:'development',CANONICAL_ORIGIN:`http://127.0.0.1:${port}`,SPACE_NAME:space,INTERNAL_KEY:internalKey};
 let runtime=await createRuntime({port,name,inspect:true,resourcePersistencePath:join(temp,"state"),bindings:base,egress:async()=>{egress++;throw Error('No egress in phone proof.');}});
 try{
 const origin=(await runtime.mf.ready).origin;
 runtime={...runtime,...await runtime.rebind({...base,CANONICAL_ORIGIN:origin})};
 const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
 const rows=async()=>Object.fromEntries((await storage.exec('SELECT key,value FROM records ORDER BY key')).map(r=>[r.key,JSON.parse(r.value)]));
 const record=async key=>JSON.parse((await storage.exec('SELECT value FROM records WHERE key=?',key))[0].value);
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
 const authored=recitation?(recitation.overcapacity?[{heading:'A bounded long Section',units:['orchard '.repeat(500),'willow '.repeat(500)]},{heading:'A short final Section',units:['A smaller final sentence.\n']}]:recitalSections):[{heading:'Synthetic first article',units:[previousText,phoneText]}];
 const texts=authored.flatMap(section=>section.units),title=recitation?'Synthetic garden recital':"Apostles' Creed",edition=recitation?'Authored phone recitation edition':'Roman Missal 2011 metadata; synthetic wording';
 const file=join(temp,'text.txt'),sections=join(temp,'sections.json');await writeFile(file,texts.join(''));await writeFile(sections,JSON.stringify(authored));
 const vid=(await cli(['version','add','--title',title,'--kind','text','--name',edition,'--source','Authored synthetic test, not canonical text','--text-file',file,'--sections-file',sections])).result.entity_id;
 const version=recitation?(await cli(['version','show',vid])).version:null;
 const sid=(await cli(['learn',vid])).practice.study.id;
 let current=(await cli(['next'])).practice;
 let cid;
 const introduced=recitation?.incomplete?1:texts.length;
 for(const [index,text] of texts.slice(0,introduced).entries()){
  cid=(await record('meta')).practice.current.targets[0].card_id;
  // Fade via actual assisted answers. No direct SQLite writes or test clock.
  for(let i=0;i<6;i++){
   assert.equal(current.prompt.cue_level,['read','read','first_letters','first_letters','first_letter','first_letter'][i]);
   const attempt=(await cli(['answer',current.prompt.id,'--text-stdin'],text)).practice.attempt;
   assert.equal(attempt.assisted,true);assert.ok(attempt.effects.every(effect=>!effect.scheduled));
   current=(await cli(['next','--after',current.prompt.id,'--card',cid])).practice;
  }
  assert.equal(current.prompt.cue_level,'previous_line');
  if(index<introduced-1||recitation){
   const attempt=(await cli(['answer',current.prompt.id,'--text-stdin'],text)).practice.attempt;
   assert.equal(attempt.assisted,false);assert.ok(attempt.effects.every(effect=>effect.scheduled));
   if(index<introduced-1)current=(await cli(['next','--after',current.prompt.id])).practice;
  }
 }
 if(!recitation)assert.equal(current.prompt.context,previousText);
 return {...runtime,origin,cli,rows,record,vid,sid,cid,prompt:current.prompt,version,title,edition,
  post:(path,fields)=>runtime.worker.fetch(origin+path,{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(fields)}),
  get egress(){return egress;},
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
 }catch(error){await runtime.mf.dispose();await rm(temp,{recursive:true,force:true});throw error;}
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
   await page.goto(f.origin+'/');await page.locator('[data-state="exact-prompt"]').waitFor();
   assert.equal(await page.locator('textarea').count(),1);assert.equal(await page.getByRole('button',{name:'Check my answer',exact:true}).count(),1);
   assert.deepEqual(await page.locator('.masthead nav a').allTextContents(),['Library']);
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
   await page.getByRole('button',{name:'I was right',exact:true}).click();await page.getByText('Owner Correction',{exact:false}).waitFor();
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
   assert.equal(f.egress,0);
  }catch(e){row.status='fail';row.reason=e.message;await record('failure');}
  finally{await clone?.close();await context.close();await f.close();}
 }
 return results;
}

const formFields=form=>form.locator('input[type="hidden"]').evaluateAll(nodes=>Object.fromEntries(nodes.map(node=>[node.name,node.value])));
const recitalUnits=f=>f.version.sections.flatMap(section=>section.units);
const recitalForm=(page,f,scope,section=0)=>page.locator(`details.exact-recitation [data-recitation-study="${f.sid}"] form:has(input[name="recite"][value="${scope}"])${scope==='section'?`:has(input[name="section_id"][value="${f.version.sections[section].id}"])`:''}`);
const nativeAction=async(page,control,keyboard=false)=>{
 if(keyboard){await control.focus();await page.keyboard.press('Enter');}
 else if(page.viewportSize().width<768)await control.tap();
 else await control.click();
};
const schedules=async f=>Object.fromEntries(Object.entries(await f.rows()).filter(([key,value])=>key.startsWith('practice_cards/')&&value.study_id===f.sid).map(([key,value])=>[key,value.state.schedule]));
const attemptCount=async f=>Object.keys(await f.rows()).filter(key=>key.startsWith('practice_attempts/')).length;
const practiceState=async f=>{const rows=await f.rows();return {current:rows.meta.practice.current,records:Object.fromEntries(Object.entries(rows).filter(([key])=>key.startsWith('practice_')))};};
const recitationLayout=async(page,zoom)=>{
 if(zoom)await page.evaluate(()=>{for(const node of document.querySelectorAll('[data-qa-text-size]')){node.style.removeProperty('font-size');delete node.dataset.qaTextSize;}for(const node of document.querySelectorAll('body,body *'))node.dataset.qaTextSize=String(parseFloat(getComputedStyle(node).fontSize)*2);for(const node of document.querySelectorAll('[data-qa-text-size]'))node.style.fontSize=node.dataset.qaTextSize+'px';});
 await page.evaluate(()=>document.fonts.ready);
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Recitation wraps without horizontal overflow, including at 200% text.');
};

async function recitationChoices(page,f,after) {
 const selector=page.locator('details.exact-recitation');
 assert.equal(await selector.count(),1);
 assert.equal(await selector.locator('summary').textContent(),'Recite a text');
 assert.equal(await selector.getAttribute('open'),null,'Recitation is a quiet native disclosure.');
 const study=selector.locator(`[data-recitation-study="${f.sid}"]`);
 assert.equal(await selector.locator('[data-recitation-study]').count(),1);
 assert.equal(await study.locator('h2').textContent(),f.title);
 assert.equal(await study.locator('p.fineprint').textContent(),f.edition);
 assert.equal((await selector.textContent()).replace(/\s/g,''),['Recite a text',f.title,f.edition,'Whole text',...f.version.sections.map(section=>`Section: ${section.heading}`)].join('').replace(/\s/g,''),'Only Work/Version metadata and scope labels appear in chooser text.');
 assert.equal(await study.locator('form').count(),f.version.sections.length+1);
 assert.equal(await selector.locator('a,textarea,script,.reading,[data-unit-index]').count(),0,'No reference detour, answer-bearing content, or client state in the selector.');
 const html=await selector.evaluate(node=>node.outerHTML),text=await selector.textContent();
 for(const unit of recitalUnits(f))for(const content of [html,text])assert.ok(!content.includes(unit.text.trim()),'Chooser DOM, including hidden metadata, contains no Unit text.');
 const operations=[];
 for(const [scope,section] of [['whole',0],...f.version.sections.map((_,index)=>['section',index])]){
  const form=recitalForm(page,f,scope,section),fields=await formFields(form);
  assert.equal(await form.getAttribute('action'),'/practice/next');assert.equal((await form.getAttribute('method')).toLowerCase(),'post');
  assert.deepEqual(Object.keys(fields).sort(),['csrf','operation_id','revision','study_id','recite',...(scope==='section'?['section_id']:[]),...(after?['after']:[])].sort());
  for(const name of ['csrf','operation_id','revision'])assert.ok(fields[name]);
  assert.equal(fields.study_id,f.sid);assert.equal(fields.recite,scope);assert.equal(fields.after,after);
  if(scope==='section')assert.equal(fields.section_id,f.version.sections[section].id);
  assert.equal(await form.locator('button').textContent(),scope==='whole'?'Whole text':`Section: ${f.version.sections[section].heading}`);
  operations.push(fields.operation_id);
 }
 assert.equal(new Set(operations).size,operations.length,'Each scope owns its own existing form operation.');
 const before=await f.rows();
 await nativeAction(page,selector.locator('summary'));
 assert.notEqual(await selector.getAttribute('open'),null);
 assert.equal(await study.locator('button').evaluateAll(nodes=>nodes.every(node=>{const rect=node.getBoundingClientRect();return rect.height>=44&&rect.left>=0&&rect.right<=innerWidth;})),true,'Native scope choices retain unclipped 44px touch targets.');
 assert.deepEqual(await f.rows(),before,'Opening metadata never records assistance, introduces a Unit or advances.');
 return selector;
}

async function recitationPrompt(page,f,scope,section=0) {
 await page.locator('[data-state="exact-prompt"]').waitFor();
 assert.equal(await page.locator('details.exact-recitation').count(),0,'An unanswered Prompt never offers replacement practice, even after Reveal.');
 assert.equal(await page.getByLabel('Your answer',{exact:true}).count(),1);
 assert.equal(await page.getByRole('button',{name:'Check my answer',exact:true}).count(),1);
 assert.equal(await page.locator('.exact-cue .eyebrow').textContent(),scope==='whole'?'Whole text':'Section');
 assert.equal(await page.locator('.exact-cue .reading').textContent(),scope==='whole'?f.title:f.version.sections[section].heading);
 assert.equal(await page.locator('.exact-context').count(),0);
 const html=await page.content();
 for(const unit of recitalUnits(f))assert.ok(!html.includes(unit.text.trim()),'Unanswered Recitation has heading/title only, not hidden expected text.');
 const selected=scope==='whole'?recitalUnits(f):f.version.sections[section].units;
 const prompt=(await f.cli(['next','--study',f.sid,'--recite',scope,...(scope==='section'?['--section',f.version.sections[section].id]:[])])).practice.prompt;
 assert.deepEqual(prompt.scope,scope==='whole'?{kind:'whole'}:{kind:'section',section_id:f.version.sections[section].id});
 assert.deepEqual(prompt.unit_ids,selected.map(unit=>unit.id));assert.equal(prompt.context,null);assert.equal(prompt.assisted,false);
 assert.equal(prompt.cue,scope==='whole'?f.title:f.version.sections[section].heading);
 assert.equal(prompt.cue_level,scope==='whole'?'whole_title':'section_heading');
 const cards=Object.values(await f.rows()).filter(value=>value?.study_id===f.sid&&value.unit_id&&value.state);
 assert.ok(selected.every(unit=>cards.some(card=>card.unit_id===unit.id&&card.introduced_ms>0)),'Every selected Unit was introduced by the real CLI progression.');
 return {prompt,selected};
}

export async function phoneRecitationWalk(browser,capture) {
 const results=[];
 for(const [width,scheme,noJS,zoom] of feedbackVariants){
  const f=await phoneFixture({recitation:{}});
  const context=await browser.newContext({viewport:{width,height:844},colorScheme:scheme,javaScriptEnabled:!noJS,...(width<768?{isMobile:true,hasTouch:true}:{})});
  const page=await context.newPage();page.setDefaultTimeout(12000);
  const row={width,scheme,noJS,text_zoom:zoom?'200%':'100%',status:'pass',criteria:[],scopes:[],evidence:[]};results.push(row);
  const shot=async name=>row.evidence.push(await capture(page,`phone-recitation-${width}-${scheme}-${zoom?200:100}-${name}.png`));
  const versionVisits=[],externalRequests=[];page.on('request',request=>{const url=new URL(request.url());if(url.pathname.startsWith('/library/versions/'))versionVisits.push(request.url());if(url.origin!==f.origin)externalRequests.push(request.url());});
  let clone;
  try{
   await page.goto(f.origin+'/');await page.locator('[data-state="exact-result"]').waitFor();
   let after=(await f.record('meta')).practice.current.prompt.id;
   const baseline=await schedules(f);
   assert.equal(Object.keys(baseline).length,3);
   for(const scope of ['whole','section']){
    await recitationLayout(page,zoom);
    await recitationChoices(page,f,after);
    await recitationLayout(page,zoom);
    await shot(scope+'-choices');
    await nativeAction(page,recitalForm(page,f,scope).getByRole('button'),scope==='section');
    const {prompt,selected}=await recitationPrompt(page,f,scope);
    await recitationLayout(page,zoom);
    await shot(scope+'-prompt');
    // The comparison clone receives exactly this frozen presentation, not a
    // separately generated Prompt. Other variants still compare CLI next/history.
    if(width===390&&scheme==='light'&&!noJS&&!zoom)clone=await f.fork();
    const answer=selected.map(unit=>unit.text).join('').replace('conceived','concieved').replace('gentle','harsh');
    await page.getByLabel('Your answer',{exact:true}).fill(answer);
    await nativeAction(page,page.getByRole('button',{name:'Check my answer',exact:true}),true);
    await page.locator('[data-state="exact-result"]').waitFor();
    // Arrival must be measured before a screenshot, reload, scroll or disclosure.
    const arrival=await feedbackArrival(page,capture,`phone-recitation-${width}-${scheme}-${zoom?200:100}-${scope}-arrival.png`,zoom);
    row.evidence.push(arrival.evidence);assertFeedbackArrival(arrival);
    const attempt=(await f.cli(['next'])).practice.attempt;
    assert.deepEqual(attempt.prompt,prompt);assert.equal(attempt.raw_answer,answer);assert.equal(attempt.assisted,false);
    assert.deepEqual(attempt.grade.units.map(unit=>unit.verdict),scope==='whole'?['Good','Again','Good']:['Good','Again']);
    assert.deepEqual(attempt.effects.map(effect=>effect.unit_id),selected.map(unit=>unit.id));
    assert.deepEqual(attempt.effects.map(effect=>effect.verdict),attempt.grade.units.map(unit=>unit.verdict));
    assert.ok(attempt.effects.every(effect=>effect.extra_practice&&!effect.scheduled&&!effect.assisted));
    assert.ok(attempt.grade.diff.some(word=>word.kind==='slip'&&word.unit_index===0));
    assert.ok(attempt.grade.diff.some(word=>word.kind==='wrong'&&word.unit_index===1));
    assert.equal(await page.locator('[data-verdict]').getAttribute('data-verdict'),attempt.grade.verdict);
    assert.deepEqual(await page.locator('[data-unit-verdict]').evaluateAll(nodes=>nodes.map(node=>({unit_index:Number(node.dataset.unitIndex),verdict:node.dataset.unitVerdict}))),attempt.grade.units.map(unit=>({unit_index:unit.unit_index,verdict:unit.verdict})));
    assert.deepEqual(await schedules(f),baseline,'Same-day Recitation leaves every schedule unchanged.');
    assert.deepEqual((await f.cli(['history'])).attempts[attempt.id],attempt);
    const parity={prompt,answer,phone_grade:attempt.grade,history_attempt:attempt.id};
    if(clone){
     assert.deepEqual((await clone.cli(['next'])).practice.prompt,prompt);
     const cliAttempt=(await clone.cli(['answer',prompt.id,'--text-stdin'],answer)).practice.attempt;
     assert.deepEqual(cliAttempt.grade,attempt.grade);assert.deepEqual(cliAttempt.effects,attempt.effects);
     parity.cli_grade=cliAttempt.grade;parity.snapshot_sha256=clone.sha256;
     await clone.close();clone=null;
    }
    const closeFull=await fullAnswer(page,f,attempt,selected.map(unit=>unit.text).join(''));
    assert.equal(await page.getByRole('list',{name:'Unit Verdicts',exact:true}).isVisible(),true);
    await shot(scope+'-full-answer');await closeFull();
    await page.reload();await page.locator('[data-state="exact-result"]').waitFor();
    assert.deepEqual((await f.cli(['next'])).practice.attempt,attempt,'Reload holds the complete Recitation result.');
    const corrections=[];
    for(const [index,verdict] of [[1,'Good'],[0,'Again']]){
     const form=page.locator(`form[action="/practice/correct"]:has(input[name="unit_id"][value="${selected[index].id}"])`);
     if(verdict==='Again')await nativeAction(page,form.locator('xpath=..').locator('summary'));
     await nativeAction(page,form.getByRole('button',{name:verdict==='Good'?'I was right':'Count as a miss',exact:true}));
     await page.getByText(`Unit ${index+1}: Owner Correction · ${verdict}. Original Verdict retained.`,{exact:true}).waitFor();
     const history=await f.cli(['history']);assert.deepEqual(history.attempts[attempt.id],attempt,'A per-Unit Correction never edits the original Attempt or Verdicts.');
     const saved=Object.values(history.corrections).filter(correction=>correction.attempt_id===attempt.id);
     const correction=saved.find(value=>value.unit_id===selected[index].id);
     assert.ok(correction);assert.equal(correction.verdict,verdict);assert.equal(correction.authority,'owner');
     for(const earlier of corrections)assert.deepEqual(saved.find(value=>value.id===earlier.id),earlier,'Earlier Corrections remain immutable.');
     corrections.push(correction);assert.equal(saved.length,corrections.length);
     assert.deepEqual(await schedules(f),baseline);
    }
    // Reveal the same selected scope through its ordinary native form. The
    // disclosure receipt must exist before answer-bearing bytes are installed.
    await recitationLayout(page,zoom);
    await recitationChoices(page,f,prompt.id);
    await nativeAction(page,recitalForm(page,f,scope).getByRole('button'),true);
    const shownPrompt=(await recitationPrompt(page,f,scope)).prompt;
    await recitationLayout(page,zoom);
    const beforeReveal=await attemptCount(f);let durableBeforeReveal=false;
    await context.route('**/practice/reveal',async route=>{
     const response=await route.fetch({maxRedirects:0});assert.equal(response.status(),303);
     durableBeforeReveal=Object.values(await f.rows()).some(value=>value?.disclosure===true&&value?.assisted===true&&value?.prompt?.id===shownPrompt.id);
     await route.fulfill({response});
    });
    await nativeAction(page,page.locator('details.overflow>summary'));
    await nativeAction(page,page.getByRole('button',{name:'Show me',exact:true}));
    await page.getByText('Assistance is recorded.',{exact:true}).waitFor();
    assert.equal(durableBeforeReveal,true);await context.unroute('**/practice/reveal');
    assert.equal(await attemptCount(f),beforeReveal+1);assert.deepEqual(await schedules(f),baseline);
    assert.equal(await page.locator('details.exact-recitation').count(),0);
    assert.equal((await page.locator('.exact-disclosure .exact-expected').allTextContents()).join(''),selected.map(unit=>unit.text).join(''));
    await page.getByLabel('Your answer',{exact:true}).fill(selected.map(unit=>unit.text).join(''));
    await nativeAction(page,page.getByRole('button',{name:'Check my answer',exact:true}),true);
    await page.locator('[data-state="exact-result"]').waitFor();
    const assisted=(await f.cli(['next'])).practice.attempt;
    assert.equal(assisted.assisted,true);assert.equal(assisted.grade.verdict,'Good');
    assert.ok(assisted.effects.every(effect=>effect.assisted&&!effect.scheduled));assert.deepEqual(await schedules(f),baseline);
    await nativeAction(page,page.getByRole('button',{name:'Next',exact:true}));
    await page.locator('[data-state="exact-caught-up"]').waitFor();
    assert.equal(await page.locator('textarea').count(),0);assert.ok((await f.cli(['due'])).next_due_ms>Date.now());
    after=undefined;
    row.scopes.push({scope,unit_ids:selected.map(unit=>unit.id),arrival,parity,correction_ids:corrections.map(correction=>correction.id),held_reload:true,assistance_durable_before_reveal:true,schedules_unchanged:true});
   }
   await recitationLayout(page,zoom);await recitationChoices(page,f,undefined);await recitationLayout(page,zoom);await shot('caught-up');
   assert.deepEqual(versionVisits,[],'Native choices never detour through an answer-bearing Version page.');
   assert.deepEqual(externalRequests,[],'Phone Recitation makes no external browser request.');
   const complete=await f.cli(['export']);assert.deepEqual(complete.app.spend,[]);assert.deepEqual(complete.app.jobs,{});assert.deepEqual(complete.app.assessments,{});assert.equal(f.egress,0);
   row.criteria.push(7);row.egress=0;
  }catch(error){row.status='fail';row.reason=error.message;await shot('failure');}
  finally{await clone?.close();await context.close();await f.close();}
 }
 return results;
}

export async function phoneRecovery(browser,capture) {
 const results=[];
 const check=async(name,run,{recitation,noJS=false}={})=>{
  const f=await phoneFixture({recitation}),context=await browser.newContext({viewport:{width:390,height:844},javaScriptEnabled:!noJS,...(recitation?{isMobile:true,hasTouch:true}:{})}),page=await context.newPage();page.setDefaultTimeout(12000);
  const row={name,status:'pass'};results.push(row);
  try{await page.goto(f.origin+'/');await run({f,context,page,row});assert.equal(f.egress,0);}
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
  const before=await count(f),other=await context.newPage();await other.goto(f.origin+'/');await answer(page);await page.locator('[data-state="exact-result"]').waitFor();
  await answer(other);await other.getByText('This changed in another tab. Your draft is still here.',{exact:true}).waitFor();assert.equal(await count(f),before+1);row.observed={new_attempts:1,stale_refusal:409};await other.close();
 });
 await check('offline pauses and reconnect sends nothing',async({f,context,page,row})=>{
  let sends=0;context.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/practice/answer'))sends++;});
  await page.getByLabel('Your answer',{exact:true}).fill(phoneAnswer);await context.setOffline(true);await page.getByRole('button',{name:'Check my answer',exact:true}).click();
  assert.equal(sends,0);assert.equal(await page.getByLabel('Your answer',{exact:true}).inputValue(),phoneAnswer);await context.setOffline(false);await page.waitForTimeout(400);assert.equal(sends,0);
  await page.getByRole('button',{name:'Check my answer',exact:true}).click();await page.locator('[data-state="exact-result"]').waitFor();assert.equal(sends,1);row.observed={offline_sends:0,reconnect_sends:0,deliberate_sends:1};
 });
 await check('automatic Good offers immutable Count as a miss',async({f,page,row})=>{
  await page.getByLabel('Your answer',{exact:true}).fill(phoneText);await page.getByRole('button',{name:'Check my answer',exact:true}).click();await page.locator('[data-verdict="Good"]').waitFor();
  const original=(await f.cli(['next'])).practice.attempt;
  await page.locator('details.overflow>summary').click();await page.getByRole('button',{name:'Count as a miss',exact:true}).click();await page.getByText('Owner Correction',{exact:false}).waitFor();
  assert.deepEqual((await f.cli(['next'])).practice.attempt,original);assert.equal((await f.record('meta')).practice.current.attempt_id,original.id);
  assert.equal((await f.record('practice_cards/'+f.cid)).state.schedule.reps,1);row.observed={original_verdict:'Good',owner_verdict:'Again',held_attempt_unchanged:true};
 });
 await check('access loss removes private DOM',async({context,page,row})=>{
  await context.route('**/practice/answer',route=>route.fulfill({status:401,contentType:'text/plain',body:'Owner authorization expired.'}));await answer(page);await page.locator('body.private-hidden').waitFor({state:'attached'});assert.equal(await page.locator('#main').count(),0);row.observed={private_main_present:false};
 });
 const payloadField=(body,name)=>body.match(new RegExp('name="'+name+'"\\r\\n\\r\\n([^\\r]+)'))?.[1];
 for(const scope of ['whole','section'])await check(`lost ${scope} choice reconciles the identical scope; duplicate submit holds one Prompt`,async({f,context,page,row})=>{
  const after=(await f.record('meta')).practice.current.prompt.id;
  await recitationChoices(page,f,after);
  const form=recitalForm(page,f,scope),fields=await formFields(form),beforeAttempts=await attemptCount(f),payloads=[];let first=true;
  await context.route('**/practice/next',async route=>{
   if(route.request().method()!=='POST')return route.continue();
   payloads.push(route.request().postData());
   if(first){first=false;assert.equal((await route.fetch({maxRedirects:0})).status(),303);await route.abort('failed');}
   else await route.continue();
  });
  await nativeAction(page,form.getByRole('button'));
  await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();
  const committed=await practiceState(f),receipt=(await f.rows())['operations/'+fields.operation_id];
  assert.ok(receipt,'The selected scope has one durable operation receipt.');
  assert.equal(committed.current.prompt.scope.kind,scope);assert.equal(committed.current.attempt_id,null);
  assert.notEqual(committed.current.prompt.id,after);assert.equal(await attemptCount(f),beforeAttempts);
  assert.equal(await form.getByRole('button').isEnabled(),false);
  await nativeAction(page,page.getByRole('button',{name:'Reconcile this attempt',exact:true}));
  await page.locator('[data-state="exact-prompt"]').waitFor();
  assert.equal(payloads.length,2);
  for(const name of Object.keys(fields)){assert.ok(payloadField(payloads[0],name));assert.equal(payloadField(payloads[0],name),payloadField(payloads[1],name),`Frozen ${name} survives response loss.`);}
  assert.deepEqual(await practiceState(f),committed,'Reconciliation cannot create another Prompt, Attempt, Correction or schedule.');
  const duplicates=await Promise.all([f.post('/practice/next',fields),f.post('/practice/next',fields)]);
  assert.deepEqual(duplicates.map(response=>response.status),[303,303]);
  assert.deepEqual(await practiceState(f),committed);
  assert.deepEqual((await f.rows())['operations/'+fields.operation_id],receipt);
  const beforeChanged=await f.rows(),changed=await f.post('/practice/next',{...fields,recite:scope==='whole'?'section':'whole',...(scope==='whole'?{section_id:f.version.sections[0].id}:{})});
  assert.equal(changed.status,409);assert.deepEqual(await f.rows(),beforeChanged,'A changed payload cannot reuse the saved choice operation.');
  assert.equal(await page.locator('details.exact-recitation').count(),0);
  row.observed={scope,sends:2,frozen_fields:Object.keys(fields),same_prompt:true,new_attempts:0,duplicate_responses:[303,303],changed_payload:409};
 },{recitation:{}});
 for(const noJS of [false,true])await check(`stale Recitation tab cannot replace an unanswered Prompt (${noJS?'native forms':'JavaScript'})`,async({f,context,page,row})=>{
  const after=(await f.record('meta')).practice.current.prompt.id,other=await context.newPage();
  try{
   await other.goto(f.origin+'/');await recitationChoices(page,f,after);await recitationChoices(other,f,after);
   const staleFields=await formFields(recitalForm(other,f,'section'));
   await nativeAction(page,recitalForm(page,f,'whole').getByRole('button'));await page.locator('[data-state="exact-prompt"]').waitFor();
   assert.equal(await page.locator('details.exact-recitation').count(),0);
   const before=await f.rows(),response=other.waitForResponse(response=>response.request().method()==='POST'&&new URL(response.url()).pathname==='/practice/next');
   await nativeAction(other,recitalForm(other,f,'section').getByRole('button'));
   assert.equal((await response).status(),409);
   await other.getByText('This changed in another tab. Reload to continue from your saved progress.',{exact:true}).waitFor();
   if(!noJS)assert.equal(await other.locator('[data-state="exact-result"]').count(),1,'Definite refusal does not install a competing page.');
   assert.deepEqual(await f.rows(),before);
   // A fresh operation/revision still cannot use a stale held-result fence.
   const stale=await f.post('/practice/next',{...staleFields,operation_id:randomUUID(),revision:String(before.meta.revision)});
   assert.equal(stale.status,409);assert.deepEqual(await f.rows(),before);
   await other.goto(f.origin+'/');await other.locator('[data-state="exact-prompt"]').waitFor();
   assert.equal(await other.locator('details.exact-recitation').count(),0);
   assert.equal(await other.locator('[data-prompt-id]').getAttribute('data-prompt-id'),before.meta.practice.current.prompt.id);
   row.observed={noJS,stale_response:409,fresh_operation_old_after:409,all_rows_unchanged:true,unanswered_selector:false};
  }finally{await other.close();}
 },{recitation:{},noJS});
 await check('offline Recitation selection never autosends on reconnect',async({f,context,page,row})=>{
  await recitationChoices(page,f,(await f.record('meta')).practice.current.prompt.id);
  const before=await f.rows();let sends=0;context.on('request',request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/practice/next')sends++;});
  await context.setOffline(true);await nativeAction(page,recitalForm(page,f,'section').getByRole('button'));
  await page.getByText('You are offline. Your draft is still here.',{exact:true}).waitFor();
  assert.equal(sends,0);assert.deepEqual(await f.rows(),before);
  await context.setOffline(false);await page.waitForTimeout(400);assert.equal(sends,0);assert.deepEqual(await f.rows(),before);
  await nativeAction(page,recitalForm(page,f,'section').getByRole('button'));await page.locator('[data-state="exact-prompt"]').waitFor();
  assert.equal(sends,1);assert.equal((await f.record('meta')).practice.current.prompt.scope.kind,'section');
  row.observed={offline_sends:0,reconnect_sends:0,deliberate_sends:1};
 },{recitation:{}});
 for(const [kind,status,message] of [['incomplete',409,'Introduce every Unit in this scope before Recitation.'],['overcapacity',422,'This Recitation exceeds the 1,000,000-cell local grading capacity. Choose a smaller Section.']])for(const noJS of [false,true])await check(`${kind} Recitation refusal retains the current result (${noJS?'native forms':'JavaScript'})`,async({f,page,row})=>{
  const held=(await f.record('meta')).practice.current,original=(await f.cli(['next'])).practice.attempt;
  const cards=Object.values(await f.rows()).filter(value=>value?.study_id===f.sid&&value.unit_id&&value.state);
  assert.equal(cards.length,kind==='incomplete'?1:3);
  const refusals=[];
  for(const scope of ['whole','section']){
   await page.goto(f.origin+'/');await page.locator('[data-state="exact-result"]').waitFor();
   await recitationChoices(page,f,held.prompt.id);
   const before=await f.rows(),response=page.waitForResponse(response=>response.request().method()==='POST'&&new URL(response.url()).pathname==='/practice/next');
   await nativeAction(page,recitalForm(page,f,scope).getByRole('button'));
   assert.equal((await response).status(),status);await page.getByText(message,{exact:true}).waitFor();
   assert.deepEqual(await f.rows(),before,'Admission refusal retains every persisted row, including Prompt, result, receipts and schedules.');
   if(!noJS)assert.equal(await page.locator('[data-state="exact-result"]').count(),1);
   await page.goto(f.origin+'/');await page.locator('[data-state="exact-result"]').waitFor();
   assert.deepEqual((await f.record('meta')).practice.current,held);assert.deepEqual((await f.cli(['next'])).practice.attempt,original);
   refusals.push({scope,status,explanation:message,all_rows_unchanged:true});
  }
  row.observed={kind,noJS,refusals,held_attempt:original.id};
 },{recitation:{[kind]:true},noJS});
 return results;
}
