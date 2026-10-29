// Synthetic authored content, real native forms/Worker/storage: no fabricated grades.
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {starterFixture,nativeFields} from './nato-starter.mjs';
function selectedFields(html,action,key,value){
 const body=html.split('<form').find(part=>part.startsWith(` method="post" action="${action}"`)&&part.split('</form>')[0].includes(`name="${key}" value="${value}"`));
 assert.ok(body,`Native ${action} ${key}=${value} form`);return nativeFields('<form'+body,action);
}
const domain=rows=>Object.fromEntries(Object.entries(rows).filter(([key])=>key.startsWith('library_')||key.startsWith('practice_')));
const version=(rows,id)=>rows['library_versions/'+id];
const current=rows=>rows.meta.practice.current;
async function submit(f,path,fields){const r=await f.post(path,fields);assert.equal(r.status,303,await r.text());return r;}
export async function lessonJourney(){
 const f=await starterFixture();
 try{
  const shelf=await f.get('/lessons'),initial=await f.rows();assert.match(shelf,/Joyful Mysteries/);assert.match(shelf,/Lasance/);assert.doesNotMatch(shelf,/Annunciation|Visitation|Holy Michael, Archangel/);
  await f.get('/lessons');assert.deepEqual(await f.rows(),initial,'Browsing never imports or schedules.');
  const start=selectedFields(shelf,'/practice/lesson/start','starter','rosary-joyful');
  for(const override of [{grade:'Good'},{answer:'known'},{pace:'intense'}]) {const before=await f.rows();assert.equal((await f.post('/practice/lesson/start',{...start,...override,operation_id:randomUUID()})).status,422);assert.deepEqual(await f.rows(),before);}
  await submit(f,'/practice/lesson/start',start);const prompt=await f.get('/'),held=await f.rows();assert.equal(current(held).prompt.scope.kind,'whole');assert.equal(current(held).prompt.assisted,false);assert.doesNotMatch(prompt,/Annunciation|Visitation/);
  const browse=await f.get('/lessons');assert.doesNotMatch(browse,/Annunciation|Visitation/);assert.deepEqual((await f.rows()).meta.practice,held.meta.practice);assert.deepEqual(domain(await f.rows()),domain(held));
  await submit(f,'/practice/answer',{...nativeFields(prompt,'/practice/answer'),answer:'Annunciation\nNativity\nPresentation\nFinding in the Temple'});
  let result=await f.get('/');assert.match(result,/data-verdict="Again"/);assert.match(result,/4 of 5 (?:mysteries|items) recalled/);
  const afterMiss=await f.rows();await f.get('/');assert.deepEqual(domain(await f.rows()),domain(afterMiss),'Result reload creates no new evidence.');
  await submit(f,'/practice/lesson/next',selectedFields(result,'/practice/lesson/next','mode','whole'));
  await submit(f,'/practice/answer',{...nativeFields(await f.get('/'),'/practice/answer'),answer:'The Annunciation\nVisitation\nBirth of Jesus\nPresentation\nFinding of Jesus in the Temple'});
  result=await f.get('/');assert.match(result,/data-verdict="Good"/);assert.match(result,/5 of 5 (?:mysteries|items) recalled/);
  const prayerStart=selectedFields(result,'/practice/lesson/start','starter','st-michael-en-lasance-1911');await submit(f,'/practice/lesson/start',prayerStart);
  const prayer=await f.get('/'),beforePrayer=await f.rows(),p=current(beforePrayer);assert.equal(p.prompt.scope.kind,'unit');assert.equal(p.prompt.assisted,true);
  const v=version(beforePrayer,p.prompt.version_id),line=v.sections.flatMap(s=>s.units)[0].text;
  await submit(f,'/practice/answer',{...nativeFields(prayer,'/practice/answer'),answer:line});result=await f.get('/');assert.match(result,/No unaided credit or schedule change/);
  const afterPrayer=await f.rows();for(const [key,card]of Object.entries(beforePrayer).filter(([key])=>key.startsWith('practice_cards/')))assert.deepEqual(afterPrayer[key].state.schedule,card.state.schedule);
  await submit(f,'/practice/lesson/next',selectedFields(result,'/practice/lesson/next','mode','join'));let joined=await f.rows();assert.equal(current(joined).prompt.scope.kind,'section');
  const section=v.sections.find(s=>s.id===current(joined).prompt.scope.section_id);await submit(f,'/practice/answer',{...nativeFields(await f.get('/'),'/practice/answer'),answer:section.units.map(u=>u.text).join('')});
  await submit(f,'/practice/lesson/next',selectedFields(await f.get('/'),'/practice/lesson/next','mode','whole'));const whole=await f.rows();assert.equal(current(whole).prompt.scope.kind,'whole');assert.equal(current(whole).prompt.assisted,false);
  await submit(f,'/practice/answer',{...nativeFields(await f.get('/'),'/practice/answer'),answer:v.text});assert.match(await f.get('/'),/data-verdict="Good"/);assert.equal(f.egress,0);
  return {status:'pass',catalog_read_only:true,ordered_omission_then_correct:true,phrase_assisted_without_schedule_credit:true,joined_section:true,cold_whole:true,egress:0};
 }finally{await f.close();}
}
export async function lessonWalk(browser,capture){
 const rows=[];
 for(const [width,noJS,scheme]of [[390,false,'light'],[1280,false,'dark'],[320,true,'dark']]){
  const f=await starterFixture(),context=await browser.newContext({viewport:{width,height:844},isMobile:width<1000,hasTouch:width<1000,javaScriptEnabled:!noJS,colorScheme:scheme,reducedMotion:'reduce'});
  const row={status:'pass',feature:'authored-lessons',width,noJS,scheme,evidence:[]};rows.push(row);
  try{
   const page=await context.newPage();page.setDefaultTimeout(12000);const errors=[];page.on('pageerror',e=>errors.push(e.message));
   const activate=locator=>width<1000?locator.tap():locator.click();const shot=async name=>{assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);row.evidence.push(await capture(page,`lesson-${width}-${noJS?'native':'enhanced'}-${name}.png`));};
   await page.goto(f.origin+'/lessons');await shot('shelf');
   await activate(page.locator('form[action="/practice/lesson/start"]').filter({has:page.locator('input[name="starter"][value="rosary-joyful"]')}).locator('button'));
   await page.locator('#answer').waitFor();await page.locator('#answer').fill('Annunciation\nNativity');
   const saved=current(await f.rows());await activate(page.locator('.lesson-picker > summary'));assert.equal(await page.locator('#answer').inputValue(),'Annunciation\nNativity');assert.deepEqual(current(await f.rows()),saved);await shot('sequence-draft');await activate(page.locator('.lesson-picker > summary'));
   if(!noJS&&width===1280){await page.locator('#answer').press('End');await page.locator('#answer').press('Enter');assert.equal(await page.locator('[data-state="exact-result"]').count(),0,'Enter inserts a line, not a grade.');}
   await page.locator('#answer').fill('Annunciation\nNativity\nPresentation\nFinding in the Temple');await activate(page.locator('form[action="/practice/answer"] button'));await page.locator('[data-verdict="Again"]').waitFor();await shot('ordered-missing');
   await activate(page.locator('.lesson-picker > summary'));await activate(page.locator('form[action="/practice/lesson/start"]').filter({has:page.locator('input[name="starter"][value="st-michael-en-lasance-1911"]')}).locator('button'));
   await page.locator('#answer').waitFor();await shot('prayer-guided');const r=await f.rows(),p=current(r),v=version(r,p.prompt.version_id);await page.locator('#answer').fill(v.sections[0].units[0].text);await activate(page.locator('form[action="/practice/answer"] button'));await page.locator('[data-state="exact-result"]').waitFor();await shot('prayer-evidence');
   const challenge=page.locator('details').filter({has:page.locator('form[action="/practice/lesson/next"] input[name="mode"][value="whole"]')});if(await challenge.count())await activate(challenge.locator('summary').first());
   await activate(page.locator('form[action="/practice/lesson/next"]').filter({has:page.locator('input[name="mode"][value="whole"]')}).locator('button'));await page.locator('#answer').waitFor();assert.equal(current(await f.rows()).prompt.assisted,false);await shot('prayer-whole');
   if(!noJS&&width===1280){await page.locator('#answer').fill('First typed phrase');await page.locator('#answer').press('Enter');assert.equal(await page.locator('[data-state="exact-result"]').count(),0,'Full text Enter inserts a line, not a grade.');}
   const beforeLeave=await f.rows(),attemptCount=Object.keys(beforeLeave).filter(k=>k.startsWith('practice_attempts/')).length;
   await page.locator('#answer').fill('Unfinished synthetic draft');await activate(page.locator('.lesson-picker > summary'));assert.equal(await page.locator('#answer').inputValue(),'Unfinished synthetic draft');assert.deepEqual(current(await f.rows()),current(beforeLeave));
   await activate(page.locator('form[action="/practice/lesson/start"]').filter({has:page.locator('input[name="starter"][value="nato"]')}).locator('button'));await page.locator('#answer').waitFor();
   const afterLeave=await f.rows();assert.equal(Object.keys(afterLeave).filter(k=>k.startsWith('practice_attempts/')).length,attemptCount,'Explicit leave creates no synthetic miss or success.');assert.notEqual(current(afterLeave).prompt.id,current(beforeLeave).prompt.id);await shot('explicit-switch');
   assert.deepEqual(errors,[]);assert.equal(f.egress,0);
  }catch(e){row.status='fail';row.error=e.stack;}finally{await context.close();await f.close();}
 }
 return rows;
}
