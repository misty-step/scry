// Study is a read-only source view; only a deliberate hidden-answer transition
// begins a graded review. This exercises real Worker forms, not simulated UI.
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {starterFixture,nativeFields} from './nato-starter.mjs';
import {htmlStructure} from './html-structure.mjs';
const domain=rows=>Object.fromEntries(Object.entries(rows).filter(([key])=>key!=='meta'));
const held=rows=>rows.meta.practice.current;
const attempts=rows=>Object.keys(rows).filter(key=>key.startsWith('practice_attempts/')).length;
async function submit(f,path,fields){const response=await f.post(path,fields);assert.equal(response.status,303,await response.text());return response;}
function selectedLesson(html,id){const form=html.split('<form').find(part=>part.startsWith(' method="post" action="/practice/lesson/start"')&&part.split('</form>')[0].includes(`name="starter" value="${id}"`));assert.ok(form);return nativeFields('<form'+form,'/practice/lesson/start');}
export function primaryStudyAction(html){
 const form=html.split('<form').map(part=>part.split('</form>')[0]).find(part=>/<button[^>]*class="[^"]*\bprimary\b/.test(part));
 assert.ok(form,'Study has one deliberate primary transition');const action=/action="([^"]+)"/.exec(form)?.[1];assert.ok(action);assert.notEqual(action,'/practice/answer','Study is not a copy-answer exercise');return {path:action,fields:nativeFields('<form'+form,action)};
}
export function assertStudySource(html,text){
 const dom=htmlStructure(html);assert.ok(dom.withAttr('data-study-phase').some(node=>node.attrs['data-study-phase']==='study'));assert.equal(dom.forms('/practice/quiz').length,1,'Committed Source exposes exactly one transition for the browser phase wait');assert.equal(dom.forms('/practice/answer').length,0,'Visible source never has a gradeable answer form');assert.equal(dom.nodes.filter(node=>node.attrs.name==='answer'||node.attrs.id==='answer').length,0,'Study has no typing input');assert.equal(dom.withAttr('data-verdict').length,0,'Studying is not a graded result');assert.equal(dom.classes('primary').length,1,'One clear transition to review');assert.ok(dom.text.includes(text.trim()),'Study displays the supplied source: '+JSON.stringify(text));assert.doesNotMatch(html,/Copy the phrase|Type the answer shown|Check answer/);return primaryStudyAction(html);
}
export async function enterMemory(f,path='/'){
 let html=await f.get(path);
 if(htmlStructure(html).forms('/practice/quiz').length){
  const before=await f.rows(),action=primaryStudyAction(html);assert.equal(action.path,'/practice/quiz');assert.equal(htmlStructure(html).forms('/practice/answer').length,0);
  if(held(before).phase==='study'){const forged=await f.post('/practice/answer',{...action.fields,operation_id:randomUUID(),answer:'A source view is not a graded quiz'});assert.equal(forged.status,409);assert.deepEqual(await f.rows(),before,'Explicit Source rejects forged/stale answer forms without writes');}
  await submit(f,action.path,action.fields);const after=await f.rows();assert.equal(attempts(after),attempts(before),'Study transition is not an answer');
  for(const [key,card]of Object.entries(before).filter(([key])=>key.startsWith('practice_cards/')))assert.deepEqual(after[key],card,'Hiding source cannot change card evidence');
  html=await f.get(path);const quizDOM=htmlStructure(html);assert.equal(quizDOM.forms('/practice/answer').length,1);assert.equal(quizDOM.classes('exact-expected').length,0,'Returning from Source hides the expected answer');assert.ok(quizDOM.withAttr('data-study-phase').some(node=>node.attrs['data-study-phase']==='quiz'));
 }
 return html;
}
export async function enterMemoryPage(page){
 // A click/tap resolves before enhanced fetch rendering. Wait for the held
 // prompt's committed phase, not an instantaneous count on the prior result.
 await page.locator('form[action="/practice/quiz"] button, #answer').first().waitFor({state:'visible'});
 await page.locator('[data-state="exact-prompt"] [data-study-phase]').first().waitFor({state:'attached'});
 const form=page.locator('form[action="/practice/quiz"]');
 if(await form.count()){
  await page.locator('[data-study-phase="study"]').waitFor();
  assert.equal(await page.locator('form[action="/practice/answer"]').count(),0);
  await form.locator('button').click();
 }
 await page.locator('[data-state="exact-prompt"] [data-study-phase="quiz"]').waitFor({state:'attached'});
 await page.locator('#answer').waitFor();
 await form.waitFor({state:'detached'});
 assert.equal(await page.locator('.exact-expected').count(),0,'Hidden review contains no expected-answer disclosure');
}
export async function studyReviewJourney(){
 const f=await starterFixture();
 try{
  await submit(f,'/practice/lesson/start',selectedLesson(await f.get('/lessons'),'st-michael-en'));
  const initial=await f.rows(),p=held(initial),v=initial['library_versions/'+p.prompt.version_id],text=v.sections.flatMap(section=>section.units).find(unit=>unit.id===p.prompt.unit_ids[0]).text;
  const source=await f.get('/'),action=assertStudySource(source,text);assert.equal(attempts(initial),0);
  await f.get('/');await f.get('/recall');await f.get('/lessons');assert.deepEqual(await f.rows(),initial,'Refresh and navigation cannot record studying as recall');
  const forged={csrf:initial.meta.csrf,revision:initial.meta.revision,operation_id:randomUUID(),prompt_id:p.prompt.id,answer:text};
  const refusal=await f.post('/practice/answer',forged);assert.ok([409,422].includes(refusal.status),'An old/forged copy-answer form cannot grade the source view');assert.deepEqual(await f.rows(),initial);
  for(const extra of [{grade:'Good'},{answer:text},{phase:'quiz'}]){const response=await f.post(action.path,{...action.fields,operation_id:randomUUID(),...extra});assert.equal(response.status,422);assert.deepEqual(await f.rows(),initial,'Client transition overrides cannot manufacture evidence');}
  const starts=await Promise.all([f.post(action.path,action.fields),f.post(action.path,action.fields)]);assert.ok(starts.every(response=>response.status===303),'Double submit selects one quiz');const started=await f.rows();assert.equal(attempts(started),0,'Trying from memory selects a quiz; it does not grade');
  await submit(f,action.path,action.fields);assert.deepEqual(domain(await f.rows()),domain(started));assert.deepEqual(held(await f.rows()),held(started),'Duplicate Study transition holds one quiz');
  const quiz=await f.get('/'),quizDOM=htmlStructure(quiz);assert.equal(quizDOM.forms('/practice/quiz').length,0,'Committed Quiz removes the prior Source transition');assert.ok(quizDOM.withAttr('data-study-phase').some(node=>node.attrs['data-study-phase']==='quiz'));assert.equal(quizDOM.forms('/practice/answer').length,1);assert.ok(!quiz.includes(text),'Expected text is absent from the entire memory response, including hidden disclosures');
  const beforeRead=await f.rows();await f.get('/recall');await f.get('/');assert.deepEqual(await f.rows(),beforeRead,'Refresh/back route keeps protected memory prompt');
  const answer={...nativeFields(quiz,'/practice/answer'),answer:text};await submit(f,'/practice/answer',answer);const graded=await f.rows();assert.equal(attempts(graded),1);assert.match(await f.get('/'),/data-verdict="Good"/);
  await submit(f,'/practice/answer',answer);assert.deepEqual(domain(await f.rows()),domain(graded),'Double submit commits one native grade');
  await f.get('/');assert.deepEqual(domain(await f.rows()),domain(graded),'Result refresh preserves history');
  const next=primaryStudyAction(await f.get('/'));await submit(f,next.path,next.fields);
  let second=await f.rows(),secondHeld=held(second),secondText=second['library_versions/'+secondHeld.prompt.version_id].sections.flatMap(section=>section.units).filter(unit=>secondHeld.prompt.unit_ids.includes(unit.id)).map(unit=>unit.text).join('\n');
  let page=await f.get('/');if(!htmlStructure(page).forms('/practice/answer').length){const nextQuiz=assertStudySource(page,secondText);await submit(f,nextQuiz.path,nextQuiz.fields);page=await f.get('/');}
  const beforePeek=await f.rows(),peek=nativeFields(page,'/practice/reveal');await submit(f,'/practice/reveal',peek);const peeked=await f.rows();
  for(const target of held(beforePeek).targets)assert.deepEqual(peeked['practice_cards/'+target.card_id].state.schedule,beforePeek['practice_cards/'+target.card_id].state.schedule,'Peeking cannot earn scheduling credit');
  const studyAgain=assertStudySource(await f.get('/'),secondText);await submit(f,studyAgain.path,studyAgain.fields);
  const afterPeekQuiz=await f.get('/');assert.ok(!afterPeekQuiz.includes(secondText),'Returning from peek hides the answer again');
  const helpedAnswer={...nativeFields(afterPeekQuiz,'/practice/answer'),answer:secondText};await submit(f,'/practice/answer',helpedAnswer);const helped=await f.rows(),helpedAttempt=helped['practice_attempts/'+held(helped).attempt_id];
  assert.equal(helpedAttempt.assisted,true,'Hiding a peek does not turn assisted recall into unaided evidence');
  for(const target of held(beforePeek).targets)assert.deepEqual(helped['practice_cards/'+target.card_id].state.schedule,beforePeek['practice_cards/'+target.card_id].state.schedule,'A correct helped answer does not advance the due date');
  await submit(f,'/practice/answer',helpedAnswer);assert.deepEqual(domain(await f.rows()),domain(helped));
  assert.equal(f.egress,0);return {status:'pass',source_no_input_or_grade:true,forged_copy_grade_refused:true,explicit_memory_transition:true,source_hidden_in_review:true,readonly_refresh_navigation:true,transition_and_answer_replay_once:true,peek_stays_assisted_without_schedule_credit:true,egress:0};
 }finally{await f.close();}
}

async function assertVisibleSourcePage(page,text){
 const source=page.locator('[data-study-phase="study"]');await source.waitFor();
 const normalized=value=>value.replace(/\s+/g,' ').trim();
 assert.equal(normalized((await source.locator('.exact-expected').allTextContents()).join(' ')),normalized(text),'Source panel shows the exact supplied unit');
}
async function assertHiddenReviewPage(page,text){
 assert.equal(await page.locator('.exact-expected').count(),0);
 const normalized=value=>value.replace(/\s+/g,' ').trim();
 assert.ok(!normalized(await page.locator('body').textContent()).includes(normalized(text)),'Expected text is absent even from hidden DOM text');
 assert.ok(!(await page.content()).includes(text),'Expected source is absent from raw response markup');
}
export async function studyReviewWalk(browser,capture){
 const results=[];
 for(const [width,noJS]of [[390,false],[320,true],[1280,false]]){
  const f=await starterFixture(),context=await browser.newContext({viewport:{width,height:844},isMobile:width<1000,hasTouch:width<1000,javaScriptEnabled:!noJS,colorScheme:noJS?'dark':'light'}),row={feature:'source-study-memory-review',width,noJS,status:'pass',evidence:[]};results.push(row);
  try{
   await submit(f,'/practice/lesson/start',selectedLesson(await f.get('/lessons'),'st-michael-en'));
   const page=await context.newPage();page.setDefaultTimeout(12000);await page.goto(f.origin+'/');const initial=await f.rows(),p=held(initial),v=initial['library_versions/'+p.prompt.version_id],text=v.sections.flatMap(s=>s.units).find(u=>u.id===p.prompt.unit_ids[0]).text.trim();
   assert.equal(await page.locator('form[action="/practice/answer"],#answer').count(),0);await page.locator('form[action="/practice/quiz"] button').waitFor();await assertVisibleSourcePage(page,text);
   row.evidence.push(await capture(page,`study-review-${width}-${noJS?'native':'enhanced'}-source.png`));await page.reload();assert.deepEqual(await f.rows(),initial);
   await enterMemoryPage(page);const quiz=await f.rows();await assertHiddenReviewPage(page,text);assert.equal(attempts(quiz),0);
   if(noJS){await page.goBack();await page.locator('#answer').waitFor();await assertHiddenReviewPage(page,text);assert.deepEqual(await f.rows(),quiz);}
   await page.goto(f.origin+'/lessons');await page.goBack();await page.locator('#answer').waitFor();assert.deepEqual(await f.rows(),quiz,'Browser back re-renders held quiz rather than disclosing old source');await assertHiddenReviewPage(page,text);
   row.evidence.push(await capture(page,`study-review-${width}-${noJS?'native':'enhanced'}-quiz.png`));
   await page.locator('form[action="/practice/reveal"] button').click();await page.locator('[data-study-phase="study"]').waitFor();assert.equal(await page.locator('#answer').count(),0);await assertVisibleSourcePage(page,text);
   const afterPeek=await f.rows();for(const target of held(quiz).targets)assert.deepEqual(afterPeek['practice_cards/'+target.card_id].state.schedule,quiz['practice_cards/'+target.card_id].state.schedule);
   await enterMemoryPage(page);await assertHiddenReviewPage(page,text);await page.locator('#answer').fill(text);await page.locator('form[action="/practice/answer"] button').click();await page.locator('[data-verdict="Good"]').waitFor();
   const result=await f.rows(),attempt=result['practice_attempts/'+held(result).attempt_id];assert.equal(attempt.assisted,true);assert.equal(await page.locator('[data-result-progress]').isVisible(),true);
   for(const target of held(quiz).targets)assert.deepEqual(result['practice_cards/'+target.card_id].state.schedule,quiz['practice_cards/'+target.card_id].state.schedule);
   row.evidence.push(await capture(page,`study-review-${width}-${noJS?'native':'enhanced'}-helped-result.png`));await page.reload();assert.deepEqual(await f.rows(),result);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);assert.equal(f.egress,0);
  }catch(error){row.status='fail';row.reason=error.stack||error.message;}
  finally{await context.close();await f.close();}
 }
 return results;
}
