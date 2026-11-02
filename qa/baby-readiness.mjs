import {htmlStructure} from './html-structure.mjs';
import {enterMemory,assertStudySource} from './study-review.mjs';
// Original, sourced baby content: real Worker/native forms and synthetic history only.
import assert from 'node:assert/strict';
import {mkdir,writeFile} from 'node:fs/promises';
import {starterFixture,nativeFields} from './nato-starter.mjs';
const current=rows=>rows.meta.practice.current;
function select(html,id){const body=html.split('<form').find(p=>p.startsWith(' method="post" action="/practice/lesson/start"')&&p.split('</form>')[0].includes(`name="starter" value="${id}"`));assert.ok(body);return nativeFields('<form'+body,'/practice/lesson/start');}
async function send(f,path,fields){const r=await f.post(path,fields);assert.equal(r.status,303,await r.text());return r.headers.get('location');}
export async function babyJourney(){
 const f=await starterFixture();try{
  const chooser=await f.get('/lessons'),before=await f.rows();assert.match(chooser,/Baby care/);await f.get('/lessons');assert.deepEqual(await f.rows(),before);
  let dest=await send(f,'/practice/lesson/start',select(chooser,'baby-soothing'));assert.equal(dest,'/?practice=baby-soothing');
  let page=await f.get(dest),rows=await f.rows(),p=current(rows);const sid=p.prompt.study_id;assert.equal(rows['practice_studies/'+sid].lesson??false,false);assert.match(page,/Stop at any signs of trying to roll/);assert.match(page,/unweighted/);assert.match(page,/mapping-study/);assert.doesNotMatch(page,/nato-letter|>swaddle</);
  await mkdir('target/keyed-mapping',{recursive:true});await writeFile('target/keyed-mapping/baby-protected.html',page);
  const unchanged=await f.rows();await f.get(dest);assert.deepEqual(await f.rows(),unchanged,'Refresh does not advance or write evidence');
  dest=await send(f,'/practice/answer',{...nativeFields(page,'/practice/answer'),answer:'not swaddle'});assert.equal(dest,'/?practice=baby-soothing');page=await f.get(dest);assert.match(page,/data-verdict="Again"/);assert.match(page,/data-result-progress/);
  const next=nativeFields(page,'/practice/next');assert.equal(next.study_id,sid);dest=await send(f,'/practice/next',next);assert.equal(dest,'/?practice=baby-soothing');page=await f.get(dest);rows=await f.rows();p=current(rows);assert.equal(p.prompt.study_id,sid);
  const reveal=nativeFields(page,'/practice/reveal');await send(f,'/practice/reveal',reveal);page=await f.get(dest);rows=await f.rows();p=current(rows);const card=rows['practice_cards/'+p.targets[0].card_id],v=rows['library_versions/'+p.prompt.version_id],text=v.sections.flatMap(s=>s.units).find(u=>u.id===p.prompt.unit_ids[0]).text;const value=text.split('\t')[1];assert.ok(value);assertStudySource(page,value);assert.ok(htmlStructure(page).text.includes(p.prompt.cue),'Study preserves the full safety-bearing mapping cue, not only the answer');
  page=await enterMemory(f,dest);await send(f,'/practice/answer',{...nativeFields(page,'/practice/answer'),answer:value});page=await f.get(dest);assert.match(page,/With help/);const helped=await f.rows();assert.deepEqual(helped['practice_cards/'+card.id].state.schedule,card.state.schedule);
  const detail=await f.get('/library/versions/'+p.prompt.version_id);await writeFile('target/keyed-mapping/baby-source-notes.html',detail);assert.match(detail,/healthychildren.org/);assert.match(detail,/happiestbaby.com/);assert.match(detail,/not medical advice|not a medical|not medical training/i);
  const history=Object.fromEntries(Object.entries(helped).filter(([key])=>key.startsWith('practice_attempts/')));
  dest=await send(f,'/practice/lesson/start',select(page,'baby-montessori'));assert.equal(dest,'/?practice=baby-montessori');await f.get(dest);const after=await f.rows();assert.notEqual(current(after).prompt.study_id,sid);for(const [k,v]of Object.entries(history))assert.deepEqual(after[k],v);assert.equal(f.egress,0);
  return {status:'pass',items:18,native_forms:true,scoped_next:true,strict_negation:true,assistance_not_scheduled:true,history_preserved:true,sources_visible:true,egress:0};
 }finally{await f.close();}
}
export async function babyWalk(browser,capture){
 const results=[];for(const [width,noJS] of [[320,true],[390,false],[1280,false]]){
  const f=await starterFixture(),context=await browser.newContext({viewport:{width,height:844},javaScriptEnabled:!noJS});try{const page=await context.newPage();await page.goto(f.origin+'/lessons');await page.locator('form[action="/practice/lesson/start"]').filter({has:page.locator('input[value="baby-soothing"]')}).locator('button').click();await page.locator('#answer').waitFor();assert.match(await page.locator('.exact-cue').innerText(),/unweighted.*Stop at any signs of trying to roll/s);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await capture(page,`baby-${width}-${noJS?'native':'enhanced'}-cue.png`);await page.locator('#answer').fill('swaddle');await page.locator('form[action="/practice/answer"] button').click();await page.locator('[data-verdict="Good"]').waitFor();await capture(page,`baby-${width}-${noJS?'native':'enhanced'}-result.png`);results.push({status:'pass',width,noJS,safety_cue_visible:true});}finally{await context.close();await f.close();}
 }return results;
}
