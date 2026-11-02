// Default cross-subject review through real native forms and stored schedules.
// Clock changes are explicit fixture transport only; no learner-row setters.
import assert from 'node:assert/strict';
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
  const empty=await f.get('/'),before=await f.rows();assert.match(empty,/data-state="mixed-home"/);assert.match(empty,/No reviews due/);assert.doesNotMatch(empty,/From Alfa/);
  await f.get('/');assert.deepEqual(await f.rows(),before,'Default reads do not enroll catalog content.');
  const studies=[];
  for(const id of ['nato','rosary-joyful','rosary-sorrowful']){
   await post(f,'/practice/lesson/start',lessonFields(await f.get('/lessons'),id));
   const rows=await f.rows(),p=current(rows);studies.push(p.prompt.study_id);
   await post(f,'/practice/answer',{...nativeFields(await f.get('/'),'/practice/answer'),answer:'A deliberate synthetic miss'});
  }
  await post(f,'/practice/lesson/start',lessonFields(await f.get('/lessons'),'st-michael-en'));
  const guided=current(await f.rows());await post(f,'/practice/answer',{...nativeFields(await f.get('/'),'/practice/answer'),answer:expected(await f.rows(),guided)});
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
  const after=await f.rows();assert.equal(current(after),null,'Unseen introduced lesson units are not mixed review work');
  assert.match(await f.get('/'),/No reviews due/);assert.match(await f.get('/'),/Next review/);await f.get('/');assert.deepEqual(await f.rows(),after);
  assert.equal(Object.keys(after).filter(k=>k.startsWith('practice_studies/')).length,4);
  assert.equal(f.egress,0);return {status:'pass',studies:seen.length,guided_only_excluded:true,same_day_due_advances:true,held_refresh:true,replay_stable:true,untouched_catalog_not_enrolled:true,unseen_units_not_reviewed:true,caught_up:true,egress:0};
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
   for(const sid of studies){const saved=await f.rows(),p=current(saved);assert.equal(p.prompt.study_id,sid);await page.locator('#answer').fill(expected(saved,p));await page.locator('form[action="/practice/answer"] button').click();await page.locator('[data-state="exact-result"]').waitFor();assert.equal(await page.locator('[data-result-progress]').isVisible(),true);row.evidence.push(await capture(page,`mixed-review-${width}-${noJS?'native':'enhanced'}-${studies.indexOf(sid)}.png`));await page.locator('form[action="/practice/review/next"] button').click();}
   await page.locator('[data-state="mixed-home"]').waitFor();await page.getByRole('heading',{name:'No reviews due',exact:true}).waitFor();assert.equal(current(await f.rows()),null);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);row.evidence.push(await capture(page,`mixed-review-${width}-${noJS?'native':'enhanced'}-caught-up.png`));
  }catch(error){row.status='fail';row.reason=error.message;}
  finally{await context.close();await f.close();}
 }
 return rows;
}
