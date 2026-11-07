import {enterMemory,enterMemoryPage,assertStudySource} from './study-review.mjs';
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
  const shelf=await f.get('/lessons'),initial=await f.rows();assert.match(shelf,/Rosary mysteries/);assert.match(shelf,/>Joyful</);assert.match(shelf,/Lasance/);assert.doesNotMatch(shelf,/Annunciation|Visitation|Holy Michael, Archangel/);
  await f.get('/lessons');assert.deepEqual(await f.rows(),initial,'Browsing never imports or schedules.');
  const start=selectedFields(shelf,'/practice/lesson/start','starter','rosary-joyful');
  for(const override of [{grade:'Good'},{answer:'known'},{pace:'intense'}]) {const before=await f.rows();assert.equal((await f.post('/practice/lesson/start',{...start,...override,operation_id:randomUUID()})).status,422);assert.deepEqual(await f.rows(),before);}
  await submit(f,'/practice/lesson/start',start);let prompt=await f.get('/');let held=await f.rows();assert.equal(current(held).prompt.scope.kind,'whole');assert.equal(current(held).prompt.assisted,false);assert.doesNotMatch(prompt,/Annunciation|Visitation/);
  const checkpoint={...nativeFields(prompt,'/practice/answer'),generation:'0',answer:'\nFirst line\nSecond line'};
  const scope=JSON.parse(await f.get('/session/scope')).scope;const saved=await f.postScoped('/practice/draft',scope,checkpoint);assert.equal(saved.status,303,await saved.text());
  held=await f.rows();assert.equal(held.meta.practice.answer_checkpoint.draft.answer,checkpoint.answer,'Multipart form newline normalization preserves the textarea value.');
  assert.equal(Object.keys(held).filter(k=>k.startsWith('practice_attempts/')).length,0);
  assert.equal((await f.postScoped('/practice/draft',scope,checkpoint)).status,303);assert.deepEqual((await f.rows()).meta.practice,held.meta.practice,'Exact multipart replay does not change the checkpoint.');
  prompt=await f.get('/');assert.match(prompt,/\n\nFirst line\nSecond line<\/textarea>/);
  const browse=await f.get('/lessons');assert.doesNotMatch(browse,/Annunciation|Visitation/);assert.deepEqual((await f.rows()).meta.practice,held.meta.practice);assert.deepEqual(domain(await f.rows()),domain(held));
  await submit(f,'/practice/answer',{...nativeFields(prompt,'/practice/answer'),answer:'Annunciation\nNativity\nPresentation\nFinding in the Temple'});
  let result=await f.get('/');assert.match(result,/data-verdict="Again"/);assert.match(result,/4 of 5 items matched/);
  const afterMiss=await f.rows();await f.get('/');assert.deepEqual(domain(await f.rows()),domain(afterMiss),'Result reload creates no new evidence.');
  await submit(f,'/practice/lesson/next',selectedFields(result,'/practice/lesson/next','mode','whole'));
  await submit(f,'/practice/answer',{...nativeFields(await f.get('/'),'/practice/answer'),answer:'The Annunciation\nVisitation\nBirth of Jesus\nPresentation\nFinding of Jesus in the Temple'});
  result=await f.get('/');assert.match(result,/data-verdict="Good"/);assert.match(result,/5 of 5 items matched/);
  const prayerStart=selectedFields(result,'/practice/lesson/start','starter','st-michael-en');await submit(f,'/practice/lesson/start',prayerStart);
  const study=await f.get('/'),sourceRows=await f.rows(),sourcePrompt=current(sourceRows),v=version(sourceRows,sourcePrompt.prompt.version_id),line=v.sections.flatMap(section=>section.units)[0].text;
  assertStudySource(study,line);assert.match(study,/Phrase 1 \/ 8/);assert.match(study,/English · supplied wording/);
  const prayer=await enterMemory(f),beforePrayer=await f.rows(),p=current(beforePrayer);assert.equal(p.prompt.scope.kind,'unit');assert.equal(p.prompt.assisted,false);assert.ok(!prayer.includes(line));
  await submit(f,'/practice/answer',{...nativeFields(prayer,'/practice/answer'),answer:v.sections[0].units[1].text});
  result=await f.get('/');assert.match(result,/Not quite/);assert.match(result,/class="lesson-target"/);assert.match(result,/Try this phrase again/);
  const missed=await f.rows(),retry=selectedFields(result,'/practice/lesson/next','mode','retry');await f.get('/');assert.deepEqual(domain(await f.rows()),domain(missed));
  await submit(f,'/practice/lesson/next',retry);const retried=await f.rows();assert.notEqual(current(retried).prompt.id,p.prompt.id);assert.deepEqual(current(retried).prompt.unit_ids,p.prompt.unit_ids);
  await submit(f,'/practice/lesson/next',retry);assert.deepEqual(domain(await f.rows()),domain(retried),'Duplicate retry holds one new prompt.');
  assert.equal((await f.post('/practice/lesson/next',{...selectedFields(result,'/practice/lesson/next','mode','phrases'),operation_id:randomUUID(),revision:retried.meta.revision})).status,409);
  assert.equal(Object.keys(retried).filter(k=>k.startsWith('practice_attempts/')).length,Object.keys(missed).filter(k=>k.startsWith('practice_attempts/')).length,'Retry creates no answer or grade.');
  await enterMemory(f);await submit(f,'/practice/answer',{...nativeFields(await f.get('/'),'/practice/answer'),answer:line});result=await f.get('/');assert.match(result,/data-verdict="Good"/);assert.match(result,/That matches · with help/,'Explicit retry after feedback remains assisted');
  await submit(f,'/practice/lesson/next',selectedFields(result,'/practice/lesson/next','mode','join'));let joined=await f.rows();assert.equal(current(joined).prompt.scope.kind,'section');
  const section=v.sections.find(s=>s.id===current(joined).prompt.scope.section_id);await submit(f,'/practice/answer',{...nativeFields(await f.get('/'),'/practice/answer'),answer:section.units.map(u=>u.text).join('')});
  await submit(f,'/practice/lesson/next',selectedFields(await f.get('/'),'/practice/lesson/next','mode','whole'));const whole=await f.rows();assert.equal(current(whole).prompt.scope.kind,'whole');assert.equal(current(whole).prompt.assisted,false);
  await submit(f,'/practice/answer',{...nativeFields(await f.get('/'),'/practice/answer'),answer:v.text});assert.match(await f.get('/'),/data-verdict="Good"/);assert.equal(f.egress,0);
  return {status:'pass',catalog_read_only:true,ordered_omission_then_correct:true,source_view_not_copy_graded:true,wrong_continuation_guided_retry:true,retry_replay_and_stale_fenced:true,joined_section:true,cold_whole:true,egress:0};
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
   await page.locator('#answer').waitFor();const beforeTyped=await f.rows();await page.locator('#answer').fill('Annunciation\nNativity');assert.deepEqual(await f.rows(),beforeTyped,'Typing alone never saves or creates evidence.');
   if(!noJS){
    await page.locator('#answer').fill('\nAnnunciation\nNativity');
    const promptBefore=current(await f.rows()).prompt.id;
    const countBefore=Object.keys(await f.rows()).filter(k=>k.startsWith('practice_attempts/')).length;
    await activate(page.getByRole('button',{name:'Save draft',exact:true}));
    await page.getByText('Save confirmed. The latest notebook state is shown.',{exact:true}).waitFor();
    await page.reload();
    assert.equal(await page.locator('#answer').inputValue(),'\nAnnunciation\nNativity');
    assert.equal(current(await f.rows()).prompt.id,promptBefore);
    assert.equal(Object.keys(await f.rows()).filter(k=>k.startsWith('practice_attempts/')).length,countBefore);
    const cancelledDialog=page.waitForEvent('dialog');const cancelDiscard=activate(page.getByRole('button',{name:'Discard saved draft',exact:true}));
    await (await cancelledDialog).dismiss();await cancelDiscard;
    assert.equal(await page.locator('#answer').inputValue(),'\nAnnunciation\nNativity');
    const discardDialog=page.waitForEvent('dialog');const discard=activate(page.getByRole('button',{name:'Discard saved draft',exact:true}));
    await (await discardDialog).accept();await discard;
    await page.getByText('Discard confirmed. The latest notebook state is shown.',{exact:true}).waitFor();
    assert.equal(await page.locator('#answer').inputValue(),'');assert.equal((await f.rows()).meta.practice.answer_checkpoint.draft,null);
    assert.equal(Object.keys(await f.rows()).filter(k=>k.startsWith('practice_attempts/')).length,countBefore);
    await page.locator('#answer').fill('Annunciation\nNativity');
   } else { assert.equal(await page.getByRole('button',{name:'Save draft',exact:true}).count(),0); }
   const saved=current(await f.rows());await activate(page.locator('.lesson-picker > summary'));assert.equal(await page.locator('#answer').inputValue(),'Annunciation\nNativity');assert.deepEqual(current(await f.rows()),saved);await shot('sequence-draft');
   assert.equal(await page.locator('[data-current-lesson="rosary-joyful"]').count(),1);
   assert.equal(await page.locator('[data-picker-close]').isVisible(),!noJS,'The enhanced return control is hidden without JavaScript.');
   assert.equal(await page.locator('form[action="/practice/lesson/start"] input[value="rosary-joyful"]').count(),0,'Current lesson cannot be restarted by the chooser.');
   if(noJS)await activate(page.locator('.lesson-picker > summary'));else await activate(page.locator('[data-picker-close]'));
   assert.equal(await page.locator('.lesson-picker').getAttribute('open'),null);
   assert.equal(await page.locator('#answer').inputValue(),'Annunciation\nNativity');assert.deepEqual(current(await f.rows()),saved);
   if(!noJS)assert.equal(await page.locator('#answer').evaluate(el=>el===document.activeElement),true,'Returning restores answer focus without navigation.');
   if(!noJS&&width===1280){await page.locator('#answer').press('End');await page.locator('#answer').press('Enter');assert.equal(await page.locator('[data-state="exact-result"]').count(),0,'Enter inserts a line, not a grade.');}
   await page.locator('#answer').fill('Annunciation\nNativity\nPresentation\nFinding in the Temple');await activate(page.locator('form[action="/practice/answer"] button'));await page.locator('[data-verdict="Again"]').waitFor();assert.equal(await page.locator('[data-result-progress]').isVisible(),true);assert.equal(await page.locator('[data-result-metric="recall"]').innerText(),'0 / 1');assert.match(await page.locator('[data-result-progress]').innerText(),/This sequence/);await shot('ordered-missing');
   assert.equal(await page.locator('.sequence-feedback li').count(),5);
   assert.match(await page.locator('.sequence-feedback [data-item-status="missing"]').innerText(),/Visitation\s*Not matched/);
   assert.equal(await page.getByText('Selected wording',{exact:true}).count(),0,'Canonical sequence is not repeated beneath the result.');
   const sequenceRows=domain(await f.rows());
   await activate(page.getByText('Your submitted answer',{exact:true}));
   assert.equal(await page.locator('.lesson-answer .reading').isVisible(),true);
   await shot('ordered-answer-open');
   await activate(page.getByText('Your submitted answer',{exact:true}));
   assert.equal(await page.locator('.lesson-answer .reading').isVisible(),false);
   await page.reload();
   assert.deepEqual(domain(await f.rows()),sequenceRows,'Opening, dismissing and reloading feedback preserves learning evidence.');
   await activate(page.locator('form[data-next] button'));
   const longUnknown='Unrecognized'+ 'word'.repeat(60);
   await page.locator('#answer').fill('Visitation\nAnnunciation\nNativity\n'+longUnknown+'\nFinding in the Temple\nAnnunciation');
   await activate(page.locator('form[action="/practice/answer"] button'));
   await page.locator('[data-verdict="Again"]').waitFor();
   assert.match(await page.locator('.sequence-extras').innerText(),/Not recognized/);
   assert.match(await page.locator('.sequence-extras').innerText(),/Repeated/);
   assert.equal(await page.locator('.sequence-extras li').count(),2);
   await shot('ordered-long-extra');
   await activate(page.locator('.lesson-picker > summary'));await activate(page.locator('form[action="/practice/lesson/start"]').filter({has:page.locator('input[name="starter"][value="st-michael-en"]')}).locator('button'));
   assert.equal(await page.locator('#answer').count(),0);await enterMemoryPage(page);assert.match(await page.locator('.lesson-instruction').innerText(),/from memory/);assert.match(await page.locator('.lesson-position').innerText(),/Phrase 1 \/ 8/);assert.equal(await page.locator('.practice-heading h1').count(),1);assert.equal(await page.locator('.lesson-position').count(),1);assert.equal(await page.locator('.lesson-stage h1').count(),0);assert.equal(await page.locator('.practice-tools > details').count(),2);await activate(page.locator('.lesson-evidence > summary'));assert.equal(await page.locator('.lesson-evidence').getAttribute('open'),'');await activate(page.locator('.lesson-picker > summary'));assert.equal(await page.locator('.lesson-evidence').getAttribute('open'),null);await activate(page.locator('.lesson-picker > summary'));await shot('prayer-guided');const r=await f.rows(),p=current(r),v=version(r,p.prompt.version_id);
   // An understandable learner mistake: continuing with the following phrase.
   await page.locator('#answer').fill(v.sections[0].units[1].text);await activate(page.locator('form[action="/practice/answer"] button'));await page.locator('[data-verdict="Again"]').waitFor();
   assert.equal((await page.locator('.lesson-target .exact-expected').innerText()).trim(),v.sections[0].units[0].text.trim());assert.equal(await page.locator('.exact-changes').isVisible(),false,'Detailed token corrections stay secondary.');assert.match(await page.locator('form[data-next] button').innerText(),/Try this phrase again/);await shot('prayer-mismatch');
   const miss=current(await f.rows());await page.reload();assert.deepEqual(current(await f.rows()),miss);assert.equal(await page.locator('.lesson-target').isVisible(),true);
   await activate(page.locator('form[data-next] button'));await enterMemoryPage(page);const retry=current(await f.rows());assert.notEqual(retry.prompt.id,p.prompt.id);assert.deepEqual(retry.prompt.unit_ids,p.prompt.unit_ids);assert.equal(retry.prompt.assisted,false);assert.match(await page.locator('.lesson-position').innerText(),/Phrase 1 \/ 8/);await shot('prayer-retry');
   await page.locator('#answer').fill(v.sections[0].units[0].text);await activate(page.locator('form[action="/practice/answer"] button'));await page.locator('[data-verdict="Good"]').waitFor();assert.match(await page.locator('.lesson-verdict').innerText(),/with help/);assert.equal(await page.locator('[data-result-metric="recall"]').innerText(),'0 / 1','Helped retry must retain the earlier failed unaided review');assert.notEqual(await page.locator('[data-result-metric="due"]').innerText(),'Not scheduled');await shot('prayer-evidence');
   await activate(page.locator('form[data-next] button'));await enterMemoryPage(page);assert.equal(current(await f.rows()).prompt.scope.position,2);assert.match(await page.locator('.lesson-position').innerText(),/Phrase 2 \/ 8/);assert.equal(await page.locator('.lesson-context').count(),0,'Memory quiz does not disclose prior source text');await shot('prayer-second');
   await page.locator('#answer').fill(v.sections[0].units[1].text);await activate(page.locator('form[action="/practice/answer"] button'));await page.locator('[data-verdict="Good"]').waitFor();
   const challenge=page.locator('details').filter({has:page.locator('form[action="/practice/lesson/next"] input[name="mode"][value="whole"]')});if(await challenge.count())await activate(challenge.locator('summary').first());
   await activate(page.locator('form[action="/practice/lesson/next"]').filter({has:page.locator('input[name="mode"][value="whole"]')}).locator('button'));await page.locator('#answer').waitFor();assert.equal(current(await f.rows()).prompt.assisted,false);assert.equal(await page.locator('.practice-edition').isVisible(),true);assert.match(await page.locator('.practice-edition').innerText(),/English · supplied wording/);await shot('prayer-whole');
   if(!noJS&&width===1280){await page.locator('#answer').fill('First typed phrase');await page.locator('#answer').press('Enter');assert.equal(await page.locator('[data-state="exact-result"]').count(),0,'Full text Enter inserts a line, not a grade.');}
   const beforeLeave=await f.rows(),attemptCount=Object.keys(beforeLeave).filter(k=>k.startsWith('practice_attempts/')).length;
   await page.locator('#answer').fill('Unfinished synthetic draft');await activate(page.locator('.lesson-picker > summary'));assert.equal(await page.locator('#answer').inputValue(),'Unfinished synthetic draft');assert.deepEqual(current(await f.rows()),current(beforeLeave));
   if(!noJS){
    const prompt=await page.locator('form[action="/practice/answer"] input[name="prompt_id"]').inputValue();
    const operation=await page.locator('form[action="/practice/answer"] input[name="operation_id"]').inputValue();
    const switchDialog=page.waitForEvent('dialog');const cancelled=activate(page.locator('form[action="/practice/lesson/start"]').filter({has:page.locator('input[name="starter"][value="nato"]')}).locator('button'));
    const confirmation=await switchDialog;assert.equal(confirmation.type(),'confirm');await confirmation.dismiss();await cancelled;
    assert.equal(await page.locator('#answer').inputValue(),'Unfinished synthetic draft');
    assert.equal(await page.locator('form[action="/practice/answer"] input[name="prompt_id"]').inputValue(),prompt);
    assert.equal(await page.locator('form[action="/practice/answer"] input[name="operation_id"]').inputValue(),operation);
    assert.deepEqual(current(await f.rows()),current(beforeLeave));
   }
   if(!noJS&&width===390){
    await context.setOffline(true);
    await activate(page.locator('form[action="/practice/lesson/start"]').filter({has:page.locator('input[name="starter"][value="nato"]')}).locator('button'));
    await page.getByText('You are offline. Your draft is still here.',{exact:true}).waitFor();
    assert.deepEqual(current(await f.rows()),current(beforeLeave));assert.equal(await page.locator('#answer').inputValue(),'Unfinished synthetic draft');
    await context.setOffline(false);
    await page.route('**/practice/lesson/start',async route=>{await route.fetch();await route.abort('failed');});
   }
   if(!noJS)page.once('dialog',dialog=>dialog.accept());
   await activate(page.locator('form[action="/practice/lesson/start"]').filter({has:page.locator('input[name="starter"][value="nato"]')}).locator('button'));
   if(!noJS&&width===390){
    await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();
    const committed=current(await f.rows());
    assert.equal(await page.locator('#answer').inputValue(),'Unfinished synthetic draft');
    await page.unroute('**/practice/lesson/start');
    await activate(page.getByRole('button',{name:'Reconcile this attempt',exact:true}));
    await page.locator('.nato-letter').waitFor();assert.deepEqual(current(await f.rows()),committed,'Switch response reconciliation keeps one prompt.');
   }
   await page.locator('#answer').waitFor();
   const afterLeave=await f.rows();assert.equal(Object.keys(afterLeave).filter(k=>k.startsWith('practice_attempts/')).length,attemptCount,'Explicit leave creates no synthetic miss or success.');assert.notEqual(current(afterLeave).prompt.id,current(beforeLeave).prompt.id);await shot('explicit-switch');
   // Inspect secondary surfaces after a real saved answer, never bypassing protection.
   await page.locator('#answer').fill('Alfa');await activate(page.locator('form[action="/practice/answer"] button'));await page.locator('[data-state="exact-result"]').waitFor();
   await page.goto(f.origin+'/library');assert.equal(await page.locator('#nato-starter').count(),0);assert.equal(await page.locator('footer').count(),0);await shot('library');
   await activate(page.locator('.version-link').first());await page.locator('[data-state="library-version"]').waitFor();assert.equal(await page.locator('.version-details').getAttribute('open'),null);await shot('version');
   await page.goto(f.origin+'/history');assert.equal(await page.locator('h1').innerText(),'History');await shot('history');
   if(!noJS){await page.goBack();await page.locator('[data-state="library-version"]').waitFor();await page.goForward();assert.equal(await page.locator('h1').innerText(),'History');}
   await page.goto(f.origin+'/settings');assert.equal(await page.locator('h1').innerText(),'Settings');assert.equal(await page.locator('#owner-tokens').getAttribute('open'),null);await shot('settings');
   await activate(page.locator('#owner-tokens > summary'));assert.equal(await page.locator('#token-name').isVisible(),true);await shot('settings-access');await activate(page.locator('#owner-tokens > summary'));
   assert.deepEqual(errors,[]);assert.equal(f.egress,0);
  }catch(e){row.status='fail';row.error=e.stack;}finally{await context.close();await f.close();}
 }
 return rows;
}
