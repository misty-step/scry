// Native local-browser journeys over the compiled Worker and real SQLite.
// The sole content fixture is the existing explicit authored-demo action.
import assert from 'node:assert/strict';
import {createRuntime,createSyntheticRuntime,internalKey} from './runtime-helper.mjs';

export async function conceptHomeWalk(browser,capture){
 const results=[];
 const cases=[...[320,390,1280].flatMap(width=>['light','dark'].map(scheme=>({width,scheme,js:true}))),{width:390,scheme:'light',js:false}];
 for(const {width,scheme,js} of cases){
  const name=`concept-home-${width}-${scheme}${js?'':'-no-js'}`;
  const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:'synthetic-concept-home',INTERNAL_KEY:internalKey};
  const runtime=await createRuntime({bindings,name,port:0,inspect:true});
  const url=(await runtime.mf.ready).origin;await runtime.rebind({...bindings,CANONICAL_ORIGIN:url});
  const context=await browser.newContext({viewport:{width,height:844},colorScheme:scheme,javaScriptEnabled:js,isMobile:width<1280,hasTouch:width<1280});
  const page=await context.newPage();page.setDefaultTimeout(12000);
  const row={name,width,scheme,javascript:js,input:width<1280?'native touch':'native keyboard',zoom:[100,200],status:'pass',evidence:[],scope:'Authored demo only; no live provider, physical phone or production acceptance. 200% CSS layout zoom.'};results.push(row);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:bindings.SPACE_NAME});
  const state=async()=>{
   const records=await storage.exec('SELECT key,value FROM records ORDER BY key');
   const meta=JSON.parse(records.find(r=>r.key==='meta').value);
   return {occurrence:meta.occurrence,practice:meta.practice??null,records:records.filter(r=>/^(questions\/|concepts\/|events\/|overrides\/|resets\/|practice_|study_version_refs\/)/.test(r.key))};
  };
  const stable=async before=>assert.deepEqual(await state(),before,'Home and history navigation must not select, reveal, grade or mutate either practice stream');
  const activate=async locator=>{
   if(width<1280){await locator.tap();return;}
   // Move actual keyboard focus through the native tab order, not DOM click().
   for(let i=0;i<40;i++){
    if(await locator.evaluate(node=>node===document.activeElement)){await page.keyboard.press('Enter');return;}
    await page.keyboard.press('Tab');
   }
   assert.fail('The requested control is not reachable through the keyboard tab order');
  };
  const home=()=>page.locator('[data-state="concept-home"]');
  const headerHome=()=>page.locator('.masthead nav').getByRole('link',{name:'Concepts',exact:true});
  const layout=async()=>{
   await page.evaluate(()=>document.fonts.ready);
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,`${name}: horizontal overflow`);
   const clipped=await page.locator('.masthead a,main .button').evaluateAll(nodes=>nodes.filter(node=>{const box=node.getBoundingClientRect();return box.left<-.5||box.right>innerWidth+.5;}).map(node=>node.textContent));
   assert.deepEqual(clipped,[],`${name}: clipped primary controls`);
  };
  const shot=async suffix=>row.evidence.push(await capture(page,`${name}-${suffix}.png`));
  try{
   await page.goto(url+'/');await home().waitFor();await page.getByRole('heading',{name:'What do you want to learn?',exact:true}).waitFor();
   assert.deepEqual(await page.locator('.masthead nav a').allTextContents(),['Concepts','Create concept']);
   assert.equal(await page.locator('footer').getByRole('link',{name:'Text practice',exact:true}).getAttribute('href'),'/recall');
   const empty=await state();assert.equal(empty.occurrence,null);assert.equal(empty.practice?.current??null,null);
   await layout();await shot('empty');await page.evaluate(()=>document.documentElement.style.zoom='2');await layout();await shot('empty-200');
   await activate(home().getByRole('link',{name:'Prepare material with AI',exact:true}));
   await page.getByText('Preparation is unavailable right now.',{exact:true}).waitFor();
   assert.equal(await page.locator('form[action="/create"],input[type="file"]').count(),0);
   await page.evaluate(()=>document.documentElement.style.zoom='2');await layout();await shot('unavailable-200');
   await activate(headerHome());await home().waitFor();await stable(empty);
   await page.reload();await home().waitFor();await stable(empty);
   await page.goBack();await page.getByText('Preparation is unavailable right now.',{exact:true}).waitFor();await page.goForward();await home().waitFor();await stable(empty);
   // Explicit opt-in to authored synthetic content, never a SQL fixture write.
   await page.goto(url+'/notebook');await activate(page.getByRole('button',{name:'Explore an authored demo',exact:true}));await page.locator('[data-state="question"]').waitFor();
   const question=await state();assert.equal(question.occurrence.phase,'question');assert.equal(question.practice?.current??null,null);
   await activate(headerHome());await home().waitFor();
   const markup=await home().innerText();
   for(const hidden of [question.occurrence.presentation.answer,question.occurrence.presentation.explanation,question.occurrence.presentation.prompt])assert.ok(!markup.includes(hidden),'Home must not leak the current question, answer or explanation');
   const cold=JSON.parse(question.records.find(r=>r.key===`concepts/${question.occurrence.concept_id}`).value);
   assert.ok(!markup.includes(cold.name),'Home must protect the current unaided concept name');
   assert.equal(await home().getByRole('link',{name:'Resume practice',exact:true}).getAttribute('href'),'/notebook');
   await stable(question);await page.evaluate(()=>document.documentElement.style.zoom='2');await layout();await shot('question-home-200');
   await page.reload();await home().waitFor();await stable(question);
   await page.goBack();await page.locator('[data-state="question"]').waitFor();await stable(question);
   await page.goForward();await home().waitFor();await stable(question);
   await activate(home().getByRole('link',{name:'Resume practice',exact:true}));await page.locator('[data-state="question"]').waitFor();await stable(question);
   await activate(page.locator('.choice').first());await page.locator('[data-state="result"]').waitFor();
   const held=await state();assert.equal(held.occurrence.phase,'result');assert.equal(held.records.filter(r=>r.key.startsWith('events/')).length,1);
   const answerPair=await page.locator('.answer-pair').innerText();
   await activate(headerHome());await home().waitFor();await stable(held);await page.evaluate(()=>document.documentElement.style.zoom='2');await layout();await shot('held-home-200');
   await page.reload();await home().waitFor();await stable(held);
   await activate(home().getByRole('link',{name:'Resume practice',exact:true}));await page.locator('[data-state="result"]').waitFor();assert.equal(await page.locator('.answer-pair').innerText(),answerPair);await stable(held);
   await page.goBack();await home().waitFor();await page.goForward();await page.locator('[data-state="result"]').waitFor();await stable(held);
   await activate(headerHome());await home().waitFor();
   const concepts=held.records.filter(r=>r.key.startsWith('concepts/')).map(r=>JSON.parse(r.value));
   const dependent=concepts.find(c=>c.prerequisites.length);assert.ok(dependent,'The authored demo must retain its real prerequisite relation');
   const prerequisite=concepts.find(c=>c.id===dependent.prerequisites[0]);assert.ok(prerequisite);
   await activate(home().locator(`a[href="/concepts/${prerequisite.id}"]`));await page.locator('[data-state="concept"]').waitFor();
   for(const heading of ['Materials','Practice','Resources','Related','Used by'])await page.getByRole('heading',{name:heading,exact:true}).waitFor();
   assert.equal(await page.getByRole('link',{name:'Original material & preparation',exact:true}).getAttribute('href'),`/goals/${prerequisite.goal_id}`);
   const reverse=page.locator('.connected').getByRole('link',{name:dependent.name,exact:true});assert.equal(await reverse.getAttribute('href'),`/concepts/${dependent.id}`);
   await layout();await shot('concept-used-by');await page.evaluate(()=>document.documentElement.style.zoom='2');await layout();await shot('concept-used-by-200');
   await activate(reverse);await page.getByRole('heading',{name:'Builds on',exact:true}).waitFor();assert.equal(await page.locator('.connected').getByRole('link',{name:prerequisite.name,exact:true}).getAttribute('href'),`/concepts/${prerequisite.id}`);
   const afterReference=await state();assert.deepEqual(afterReference.occurrence,held.occurrence);assert.deepEqual(afterReference.practice,held.practice);assert.deepEqual(afterReference.records.filter(r=>r.key.startsWith('events/')),held.records.filter(r=>r.key.startsWith('events/')));
   // Archive through the real Worker command, not by rewriting Concept flags.
   await activate(page.getByRole('link',{name:'Original material & preparation',exact:true}));
   await page.locator('[data-state="goal"]').waitFor();
   await activate(page.getByRole('button',{name:'Archive this goal',exact:true}));await page.locator('[data-state="map"]').waitFor();
   const archiveBaseline=await state();
   assert.deepEqual(archiveBaseline.occurrence,held.occurrence);assert.deepEqual(archiveBaseline.practice,held.practice);
   assert.deepEqual(archiveBaseline.records.filter(r=>r.key.startsWith('events/')),held.records.filter(r=>r.key.startsWith('events/')));
   assert.equal(await page.locator(`a[href="/goals/${prerequisite.goal_id}"]`).count(),0);
   await activate(headerHome());await home().waitFor();
   assert.equal(await home().locator('.home-concepts a').count(),0);assert.equal(await home().getByRole('link',{name:'Start practice',exact:true}).count(),0);
   await stable(archiveBaseline);await shot('archived-goal-home');
   for(const concept of [prerequisite,dependent]){
    await page.goto(url+`/concepts/${concept.id}`);await page.getByRole('heading',{name:concept.name,exact:true}).waitFor();
    await page.getByText('Archived: retained for reference.',{exact:true}).waitFor();
    assert.equal(await page.getByRole('button',{name:'Practice this idea',exact:false}).count(),0);
    assert.equal(await page.locator('.connected').count(),0);
    // Deliberately reading a reference records future exposure, not practice.
    // Preserve every record field except that existing exposure timestamp.
    const withoutExposure=snapshot=>({...snapshot,records:snapshot.records.map(r=>r.key.startsWith('concepts/')?{...r,value:JSON.stringify({...JSON.parse(r.value),exposure_ms:null})}:r)});
    assert.deepEqual(withoutExposure(await state()),withoutExposure(archiveBaseline));
   }
   await page.goto(url+'/history');assert.deepEqual((await state()).records.filter(r=>r.key.startsWith('events/')),held.records.filter(r=>r.key.startsWith('events/')));
   await page.goto(url+'/notebook');await page.locator('[data-state="result"]').waitFor();
   await activate(page.getByRole('button',{name:'Next question',exact:true}));await page.locator('[data-state="empty"], [data-state="caught-up"]').waitFor();
   await activate(headerHome());await home().waitFor();assert.equal(await home().getByRole('link',{name:/^(Start|Resume) practice$/}).count(),0);
   assert.equal((await state()).occurrence,null);assert.equal((await state()).practice?.current??null,null);
   await layout();assert.deepEqual(errors,[],'Browser errors');
  }catch(error){row.status='fail';row.reason=error.message;console.error(`${name}: ${error.message}`);await shot('failure');}
  finally{await context.close();await runtime.mf.dispose();}
 }
 const row={name:'concept-home-read-only-poll',status:'pass',evidence:[],scope:'Existing saved requests and native Home polling; no provider configuration or external egress.'};results.push(row);
 const runtime=await createSyntheticRuntime({name:'concept-home-read-only-poll'});
 const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true});const page=await context.newPage();page.setDefaultTimeout(12000);
 let releasePoll;const pollRelease=new Promise(done=>{releasePoll=done;});
 try{
  const read=async()=>{const response=await context.request.get(runtime.url+'/export');assert.equal(response.status(),200);return(await response.json()).app;};
  const practice=app=>({occurrence:app.occurrence,practice:app.practice,events:app.events,concepts:app.concepts,questions:app.questions});
  const save=async operation=>{
   const settings=await(await context.request.get(runtime.url+'/settings')).text();
   const fields=Object.fromEntries([...settings.matchAll(/name="(csrf|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
   const response=await context.request.post(runtime.url+'/create',{form:{...fields,operation_id:operation,intent:'Synthetic saved request for Home status'},headers:{Origin:runtime.url},maxRedirects:0});assert.equal(response.status(),303);
  };
  await save('home-poll-stopped');await page.goto(runtime.url+'/');
  await page.getByText('Preparing your material',{exact:true}).waitFor();const before=practice(await read());
  await page.getByText('Preparation needs your attention',{exact:true}).waitFor();
  assert.equal(new URL(page.url()).pathname,'/');assert.deepEqual(practice(await read()),before);assert.deepEqual((await read()).spend,[]);
  row.evidence.push(await capture(page,'concept-home-poll-stopped.png'));
  // Hold an actual server poll response across newer native navigation.
  await save('home-poll-stale-navigation');let sawPoll;const started=new Promise(done=>{sawPoll=done;});let completedPoll;const completed=new Promise(done=>{completedPoll=done;});let deliveryError;
  await context.route(runtime.url+'/',async route=>{
   if(route.request().headers()['x-scry-request']!=='poll')return route.continue();
   const response=await route.fetch();sawPoll();await pollRelease;
   try{await route.fulfill({response});}catch(error){deliveryError=error.message;}finally{completedPoll();}
  });
  await page.goto(runtime.url+'/');await page.getByText('Preparing your material',{exact:true}).waitFor();
  let timer;try{await Promise.race([started,new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error('Home did not issue its read-only poll')),12000);})]);}finally{clearTimeout(timer);}
  await page.locator('footer').getByRole('link',{name:'Settings',exact:true}).tap();await page.waitForURL(runtime.url+'/settings');
  releasePoll();await completed;await page.waitForTimeout(250);
  if(deliveryError)assert.match(deliveryError,/closed|cancel|abort/i,'Only native navigation cancellation may prevent delivery of the old poll');
  assert.equal(new URL(page.url()).pathname,'/settings');assert.equal(await page.locator('[data-state="concept-home"]').count(),0);
  assert.deepEqual(practice(await read()),before);assert.deepEqual((await read()).spend,[]);row.evidence.push(await capture(page,'concept-home-poll-newer-navigation.png'));
 }catch(error){row.status='fail';row.reason=error.message;console.error(`${row.name}: ${error.message}`);row.evidence.push(await capture(page,'concept-home-poll-failure.png'));}
 finally{releasePoll();await context.close();await runtime.mf.dispose();}
 return results;
}
