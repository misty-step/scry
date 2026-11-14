import {enterMemory,enterMemoryPage,assertStudySource} from './study-review.mjs';
// Default cross-subject review through real native forms and stored schedules.
// Clock changes are explicit fixture transport only; no learner-row setters.
import assert from 'node:assert/strict';
import {htmlStructure} from './html-structure.mjs';
import {randomUUID} from 'node:crypto';
import {starterFixture,nativeFields} from './nato-starter.mjs';
const DAY=86400000,START=(Math.floor(Date.now()/DAY)+1)*DAY+12*3600000;
const current=rows=>rows.meta.practice.current;
const domains=rows=>Object.fromEntries(Object.entries(rows).filter(([k])=>k!=='meta'));
function lessonFields(html,id){const part=html.split('<form').find(p=>p.startsWith(' method="post" action="/practice/lesson/start"')&&p.split('</form>')[0].includes(`name="starter" value="${id}"`));assert.ok(part);return nativeFields('<form'+part,'/practice/lesson/start');}
async function post(f,path,fields){const r=await f.post(path,fields);assert.equal(r.status,303,await r.text());return r;}
function expected(rows,p){const v=rows['library_versions/'+p.prompt.version_id];const units=v.sections.flatMap(s=>s.units).filter(u=>p.prompt.unit_ids.includes(u.id));return units.map(u=>v.kind==='keyed_mapping'?u.text.split('\t')[1].trim():u.text.trim()).join('\n');}
export async function mixedReviewFixture(){
 const f=await starterFixture({nowMs:START});
 try{
  const empty=await f.get('/'),before=await f.rows();assert.match(empty,/data-state="mixed-home"/);assert.doesNotMatch(empty,/No reviews due/);assert.match(empty,/action="\/practice\/review\/start"/);assert.doesNotMatch(empty,/<details[^>]*class="lesson-picker"[^>]* open/);assert.doesNotMatch(empty,/From Alfa/);
  await f.get('/');assert.deepEqual(await f.rows(),before,'Default reads do not enroll catalog content.');
  const studies=[];
  for(const id of ['nato','rosary-joyful','rosary-sorrowful']){
   await post(f,'/practice/lesson/start',lessonFields(await f.get('/lessons'),id));
   const rows=await f.rows(),p=current(rows);studies.push(p.prompt.study_id);
   // A new letter or Rosary set is studied first; the miss answers its Quiz.
   assertStudySource(await f.get('/'),expected(rows,p));
   await post(f,'/practice/answer',{...nativeFields(await enterMemory(f),'/practice/answer'),answer:'A deliberate synthetic miss'});
  }
  await post(f,'/practice/lesson/start',lessonFields(await f.get('/lessons'),'st-michael-en'));
  await enterMemory(f);const guided=current(await f.rows());await post(f,'/practice/answer',{...nativeFields(await f.get('/'),'/practice/answer'),answer:expected(await f.rows(),guided)});
  const introduced=await f.rows();assert.equal(Object.keys(introduced).filter(k=>k.startsWith('practice_studies/')).length,4,'Unused catalog subjects not enrolled');
  await f.clock(START+61000);
  await post(f,'/practice/review/start',{...nativeFields(await f.get('/'),'/practice/review/start'),after:current(await f.rows()).prompt.id});
  assert.equal(current(await f.rows()).prompt.study_id,studies[0],'Least-recently-served due subject first');
  return {f,studies};
 }catch(error){await f.close();throw error;}
}
export async function mixedReviewJourney(){
 const {f,studies}=await mixedReviewFixture();
 try{
  const seen=[];
  for(const sid of studies){
   const before=await f.rows(),p=current(before);assert.equal(p.prompt.study_id,sid);assert.equal(before.meta.practice.mixed_review,true);seen.push(sid);
   const html=await f.get('/');assert.match(html,/All subjects/);assert.deepEqual(await f.rows(),before,'Held refresh neither selects nor advances');
   const fields={...nativeFields(html,'/practice/answer'),answer:expected(before,p)};
   const bad=await f.post('/practice/answer',{...fields,operation_id:randomUUID(),grade:'Good'});assert.equal(bad.status,422);assert.deepEqual(await f.rows(),before);
   await post(f,'/practice/answer',fields);const answered=await f.rows(),a=answered['practice_attempts/'+current(answered).attempt_id];
   assert.equal(a.assisted,false);
   assert.ok(p.targets.every(t=>answered['practice_cards/'+t.card_id].state.schedule.due_ms>before['practice_cards/'+t.card_id].state.schedule.due_ms),'Every genuinely due same-day component advances its persisted schedule');
   await post(f,'/practice/answer',fields);assert.deepEqual(domains(await f.rows()),domains(answered),'Replay never duplicates schedule/evidence');
   const next=nativeFields(await f.get('/'),'/practice/review/next');await post(f,'/practice/review/next',next);const advanced=await f.rows();
   await post(f,'/practice/review/next',next);assert.deepEqual(domains(await f.rows()),domains(advanced),'Next replay preserves exact selected scope');
  }
  const after=await f.rows();assert.ok(current(after),'After due reviews, default continues with new or unfinished learning');
  assert.ok(!studies.includes(current(after).prompt.study_id),'Least-served unstarted subject precedes more from already reviewed subjects');
  assert.doesNotMatch(await f.get('/'),/No reviews due/);await f.get('/');assert.deepEqual(await f.rows(),after);
  assert.equal(f.egress,0);return {status:'pass',studies:seen.length,source_learning_eligible:true,same_day_due_advances:true,held_refresh:true,replay_stable:true,untouched_catalog_not_enrolled:true,continues_after_due:true,egress:0};
 }finally{await f.close();}
}
export async function mixedReviewWalk(browser,capture){
 const rows=[];
 for(const [width,noJS]of [[390,false],[320,true],[1280,false]]){
  const {f,studies}=await mixedReviewFixture(),context=await browser.newContext({viewport:{width,height:844},isMobile:width<1000,hasTouch:width<1000,javaScriptEnabled:!noJS,colorScheme:noJS?'dark':'light'});
  const row={feature:'mixed-due-review',width,noJS,status:'pass',evidence:[]};rows.push(row);
  try{
   const page=await context.newPage();page.setDefaultTimeout(12000);await page.goto(f.origin+'/');await page.locator('#answer').fill('Draft survives browsing');
   const held=current(await f.rows());await page.locator('.lesson-picker>summary').click();assert.equal(await page.locator('#answer').inputValue(),'Draft survives browsing');if(noJS)await page.locator('.lesson-picker>summary').click();else await page.locator('[data-picker-close]').click();assert.deepEqual(current(await f.rows()),held);
   for(const sid of studies){const saved=await f.rows(),p=current(saved);assert.equal(p.prompt.study_id,sid);await page.locator('#answer').fill(expected(saved,p));await page.locator('form[action="/practice/answer"] button').click();await page.locator('[data-state="exact-result"]').waitFor();assert.equal(await page.locator('[data-result-progress]').isVisible(),true);row.evidence.push(await capture(page,`mixed-review-${width}-${noJS?'native':'enhanced'}-${studies.indexOf(sid)}.png`));await page.locator('form[action="/practice/review/next"] button').click();await enterMemoryPage(page);}
   await page.locator('#answer').waitFor();assert.ok(current(await f.rows()),'New learning follows due reviews without a dead end');assert.equal(await page.getByRole('heading',{name:'No reviews due',exact:true}).count(),0);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);row.evidence.push(await capture(page,`mixed-review-${width}-${noJS?'native':'enhanced'}-continues-learning.png`));
  }catch(error){row.status='fail';row.reason=error.message;}
  finally{await context.close();await f.close();}
 }
 return rows;
}

// Drive every available default step using real native commands and deterministic
// server grading. No direct learner-row writes, no forged mastery, no catalog
// bulk enrollment on GET. Constant fixture time isolates new-content pacing.
export async function continuousStudyJourney(){
 const f=await starterFixture({nowMs:START});
 try{
  const initial=await f.get('/'),before=await f.rows();
  assert.match(initial,/data-state="mixed-home"/);assert.doesNotMatch(initial,/No reviews due/);
  const entry=htmlStructure(initial);assert.equal(entry.forms('/practice/review/start').length,1);assert.equal(entry.classes('primary').length,1);assert.equal(entry.nodes.filter(n=>n.tag==='h1').length,1);assert.equal(entry.classes('lesson-picker').length,1);assert.ok(!('open' in entry.classes('lesson-picker')[0].attrs),'Browse is closed in native parsed HTML');assert.equal(entry.forms('/create').length,0);
  const start=nativeFields(initial,'/practice/review/start');
  await f.get('/');await f.get('/lessons');await f.get('/library');assert.deepEqual(await f.rows(),before);
  await post(f,'/practice/review/start',start);let rows=await f.rows();
  assert.equal(Object.keys(rows).filter(k=>k.startsWith('practice_studies/')).length,1,'One action enrolls only selected subject');
  const first=current(rows);await post(f,'/practice/review/start',start);assert.deepEqual(domains(await f.rows()),domains(rows));assert.deepEqual(current(await f.rows()),first);
  const visits=[],guided=new Set(),cold=new Set(),firstVisits=new Set();let lastAnswer;
  for(let step=0;step<240;step++){
   rows=await f.rows();let p=current(rows);if(!p)break;
   if(htmlStructure(await f.get('/')).forms('/practice/quiz').length){guided.add(p.prompt.study_id);assertStudySource(await f.get('/'),expected(rows,p));await enterMemory(f);rows=await f.rows();p=current(rows);}
   const html=await f.get('/');assert.deepEqual(await f.rows(),rows,'Holding a prompt is read-only');
   const promptDOM=htmlStructure(html);assert.equal(promptDOM.forms('/practice/answer').length,1);assert.equal(promptDOM.withAttr('data-result-progress').length,0);assert.equal(promptDOM.nodes.filter(n=>n.attrs.id==='answer').length,1);assert.equal(promptDOM.nodes.filter(n=>n.tag==='input'&&['grade','verdict','rating'].includes(n.attrs.name)).length,0,'Client cannot submit a grade');if(!p.prompt.assisted)assert.equal(promptDOM.classes('exact-expected').length,0,'Unaided response does not expose expected answer');assert.ok(!('open' in promptDOM.classes('lesson-picker')[0].attrs),'Choosing another subject stays secondary');
   visits.push(p.prompt.study_id);firstVisits.add(p.prompt.study_id);
   if(!p.prompt.assisted)cold.add(p.prompt.study_id);
   const fields={...nativeFields(html,'/practice/answer'),answer:expected(rows,p)};
   await post(f,'/practice/answer',fields);const graded=await f.rows(),attempt=graded['practice_attempts/'+current(graded).attempt_id];
   assert.ok(attempt.id,'Server persisted one immutable attempt');
   if(p.prompt.assisted)for(const t of p.targets)assert.deepEqual(graded['practice_cards/'+t.card_id].state.schedule,rows['practice_cards/'+t.card_id].state.schedule,'Guided progress must not grant recall scheduling credit');
   const result=await f.get('/');assert.match(result,/data-result-progress/);assert.match(result,/data-verdict="Good"/,'Canonical answer must be graded by server');const resultDOM=htmlStructure(result);assert.equal(resultDOM.forms('/practice/review/next').length,1);assert.equal(resultDOM.withAttr('data-result-progress').length,1);assert.equal(resultDOM.forms('/practice/answer').length,0);
   await f.get('/');assert.deepEqual(await f.rows(),graded,'Result reload preserves evidence');
   await post(f,'/practice/answer',fields);assert.deepEqual(domains(await f.rows()),domains(graded));
   const next=nativeFields(await f.get('/'),'/practice/review/next');await post(f,'/practice/review/next',next);const advanced=await f.rows();
   await post(f,'/practice/review/next',next);assert.deepEqual(domains(await f.rows()),domains(advanced));assert.deepEqual(current(await f.rows()),current(advanced));
   if(step===0){assert.ok(first.targets.every(t=>graded['practice_cards/'+t.card_id].state.schedule.due_ms>START),'Existing work is future-due when unstarted subjects remain');assert.ok(current(advanced));assert.notEqual(current(advanced).prompt.study_id,first.prompt.study_id,'All-future due plus unseen library continues to a different subject');}
   if(lastAnswer){const stale=await f.post('/practice/answer',{...lastAnswer,operation_id:randomUUID(),revision:advanced.meta.revision});assert.equal(stale.status,409,'Old prompt cannot grade a newly held card');assert.deepEqual(domains(await f.rows()),domains(advanced));}
   lastAnswer=fields;
  }
  rows=await f.rows();assert.equal(current(rows),null,'Default eventually honors every per-study new-card budget');
  assert.equal(firstVisits.size,9,'All canonical catalog subjects are included, not historical/Latin variants');
  assert.equal(new Set(visits.slice(0,9)).size,9,'Unstarted subjects precede repeated new material from already-served subjects');
  assert.ok(guided.size>0);for(const sid of guided)assert.ok(cold.has(sid),'Studying source progresses to protected memory review');
  const studies=Object.entries(rows).filter(([k])=>k.startsWith('practice_studies/')).map(([,s])=>s);
  const counts=studies.map(s=>({kind:rows['library_versions/'+s.version_id].kind,count:Object.values(rows).filter(c=>c.study_id===s.id&&c.unit_index!==undefined).length}));
  assert.ok(counts.some(c=>c.kind==='keyed_mapping'&&c.count===6),'Continuous NATO respects six-card pace, not focused all-26 flag');
  assert.ok(counts.every(c=>c.kind==='ordered_list'||c.count<=6),'Non-sequence subjects cannot exceed daily introductions');
  const exhausted=await f.get('/');const doneDOM=htmlStructure(exhausted);assert.equal(doneDOM.forms('/practice/review/start').length,0);assert.equal(doneDOM.classes('lesson-picker').length,1);assert.ok(!('open' in doneDOM.classes('lesson-picker')[0].attrs));assert.doesNotMatch(exhausted,/action="\/practice\/review\/start"/);assert.doesNotMatch(exhausted,/No reviews due/);assert.match(exhausted,/<h1>Rest for now<\/h1>/);assert.doesNotMatch(exhausted,/All learned|All done|All set|Next review in/,'Paced rest is not a claim that all Library material is learned, and shows no countdown');
  assert.equal(doneDOM.withAttr('data-next-review').length,1);assert.match(exhausted,/data-next-review>Next review [A-Z][a-z]{2} \d{1,2}, \d{4} · \d{2}:\d{2} UTC\.<\/p>/,'Rest states the next scheduled review as a static time');
  const restOf=html=>html.slice(html.indexOf('data-state="rest"'),html.indexOf('</summary>',html.indexOf('data-state="rest"')));
  assert.ok(restOf(exhausted).includes('<span>Browse</span>'));assert.equal(restOf(await f.get('/recall')),restOf(exhausted),'Opening practice shows the same rest state as Home');
  await f.get('/');await f.get('/recall');assert.deepEqual(await f.rows(),rows,'Paced completion GET preserves all history');assert.equal(f.egress,0);
  return {status:'pass',prompts:visits.length,subjects:firstVisits.size,fresh_one_action:true,parsed_html_every_prompt_and_result:true,due_future_continues_new:true,source_to_memory_review:true,per_study_pacing:true,canonical_editions:true,true_paced_exhaustion:true,held_get_readonly:true,answer_and_next_idempotent:true,stale_grade_fenced:true,egress:0};
 }finally{await f.close();}
}

export async function continuousStudyWalk(browser,capture){
 const results=[];
 for(const [width,noJS]of [[390,false],[320,true],[1280,false]]){
  const f=await starterFixture({nowMs:START}),context=await browser.newContext({viewport:{width,height:844},javaScriptEnabled:!noJS,isMobile:width<1000,hasTouch:width<1000,colorScheme:noJS?'dark':'light'});
  const row={feature:'continuous-study-entry',width,noJS,status:'pass',evidence:[]};results.push(row);
  try{
   const page=await context.newPage();page.setDefaultTimeout(12000);await page.goto(f.origin+'/');
   const home=page.locator('[data-state="mixed-home"]');await home.waitFor();
   assert.equal(await home.locator('.primary').count(),1);assert.equal(await home.getByRole('button',{name:'Study',exact:true}).count(),1);
   assert.equal(await home.locator('.lesson-picker').getAttribute('open'),null);assert.match(await home.locator('.lesson-picker>summary').innerText(),/Browse/);
   assert.equal(await home.getByRole('heading',{name:'No reviews due',exact:true}).count(),0);
   const before=await f.rows();await page.reload();assert.deepEqual(await f.rows(),before);
   row.evidence.push(await capture(page,`continuous-study-${width}-${noJS?'native':'enhanced'}-entry.png`));
   // Browse opens the lesson shelf and the Library, and changes nothing.
   await home.locator('.lesson-picker>summary').click();assert.equal(await home.locator('.lesson-picker .lesson-shelf').isVisible(),true);
   const library=home.locator('.lesson-picker a[href="/library"]');assert.equal(await library.count(),1);assert.equal(await library.isVisible(),true);assert.equal((await library.innerText()).trim(),'Library');
   row.evidence.push(await capture(page,`continuous-study-${width}-${noJS?'native':'enhanced'}-browse.png`));
   await library.click();await page.locator('h1').first().waitFor();assert.equal(new URL(page.url()).pathname,'/library');assert.deepEqual(await f.rows(),before,'Browsing to the Library writes nothing');
   await page.goto(f.origin+'/');await home.waitFor();assert.equal(await home.locator('.lesson-picker').getAttribute('open'),null);
   // Study's first subject is new: it is shown as source study before its Quiz.
   await home.getByRole('button',{name:'Study',exact:true}).click();await page.locator('[data-study-phase="study"]').waitFor();
   const studied=await f.rows();assert.equal(current(studied).phase,'study');assert.equal(await page.locator('#answer,form[action="/practice/answer"]').count(),0);assert.ok((await page.locator('[data-study-phase="study"]').innerText()).includes(expected(studied,current(studied))));
   row.evidence.push(await capture(page,`continuous-study-${width}-${noJS?'native':'enhanced'}-first-study.png`));
   await enterMemoryPage(page);await page.locator('#answer').waitFor();
   const first=current(await f.rows());await page.locator('#answer').fill('My held draft');await page.locator('.lesson-picker>summary').click();assert.equal(await page.locator('#answer').inputValue(),'My held draft');await page.locator('.lesson-picker>summary').click();assert.deepEqual(current(await f.rows()),first);
   let retried=false;
   for(let n=0;n<10;n++){
    let rows=await f.rows(),p=current(rows);
    if(n>0&&!retried&&rows['library_versions/'+p.prompt.version_id].kind==='text'&&p.prompt.scope.kind==='unit'){
     // A missed phrase under Study's mix: expected phrase at once, guided retry first, Next second.
     retried=true;const phrase=expected(rows,p);
     await page.locator('#answer').fill('Not the phrase that was asked for');await page.locator('form[action="/practice/answer"] button').click();await page.locator('[data-verdict="Again"]').waitFor();
     const missed=await f.rows();assert.equal(missed.meta.practice.mixed_review,true);
     assert.equal((await page.locator('.lesson-target .exact-expected').innerText()).trim(),phrase.trim());assert.equal(await page.locator('.lesson-target').isVisible(),true);
     assert.equal(await page.locator('form[data-next] button').count(),1);assert.equal((await page.locator('form[data-next] button').innerText()).trim(),'Try this phrase again');
     const onward=page.locator('form[action="/practice/review/next"] button');assert.equal(await onward.count(),1);assert.equal((await onward.innerText()).trim(),'Next');assert.equal(await onward.evaluate(node=>node.classList.contains('primary')),false,'Continuing is the secondary choice');
     row.evidence.push(await capture(page,`continuous-study-${width}-${noJS?'native':'enhanced'}-missed-phrase.png`));
     await page.locator('form[data-next] button').click();await page.locator('[data-study-phase="study"]').waitFor();
     const offered=await f.rows();assert.deepEqual(Object.keys(offered).filter(k=>k.startsWith('practice_attempts/')),Object.keys(missed).filter(k=>k.startsWith('practice_attempts/')),'A retry records nothing until it is submitted');
     assert.equal(offered.meta.practice.mixed_review,true,'The retry stays under Study');assert.deepEqual(current(offered).prompt.unit_ids,p.prompt.unit_ids);
     await enterMemoryPage(page);assert.ok(!(await page.locator('main').innerText()).includes(phrase.trim()),'The retry is answered with the source hidden again');
     await page.locator('#answer').fill(phrase);await page.locator('form[action="/practice/answer"] button').click();await page.locator('[data-verdict="Good"]').waitFor();
     const helped=await f.rows(),attempt=helped['practice_attempts/'+current(helped).attempt_id];assert.equal(attempt.assisted,true,'A guided retry is never unaided credit');
     for(const t of p.targets)assert.deepEqual(helped['practice_cards/'+t.card_id].state,missed['practice_cards/'+t.card_id].state,'A guided retry leaves the schedule alone');
     assert.equal((await page.locator('form[data-next] button').innerText()).trim(),'Next');
     await page.locator('form[action="/practice/review/next"] button').click();await enterMemoryPage(page);continue;
    }
    await page.locator('#answer').fill(expected(rows,p));await page.locator('form[action="/practice/answer"] button').click();await page.locator('[data-state="exact-result"]').waitFor();assert.equal(await page.locator('[data-result-progress]').isVisible(),true);
    if(n===0)row.evidence.push(await capture(page,`continuous-study-${width}-${noJS?'native':'enhanced'}-graded.png`));
    await page.locator('form[action="/practice/review/next"] button').click();await enterMemoryPage(page);
    if(n===0){assert.notEqual(current(await f.rows()).prompt.study_id,first.prompt.study_id);row.evidence.push(await capture(page,`continuous-study-${width}-${noJS?'native':'enhanced'}-next-subject.png`));}
   }
   assert.equal(retried,true,'Study served a text phrase to miss and retry');
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);assert.equal(f.egress,0);
  }catch(error){row.status='fail';row.reason=error.stack||error.message;}
  finally{await context.close();await f.close();}
 }
 return results;
}
