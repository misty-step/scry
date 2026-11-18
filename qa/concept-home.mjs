// Native local-browser journeys over the compiled Worker and real SQLite.
// The earlier concept notebook is loaded from its fixture, one stored state per
// stage, and is only ever read.
import assert from 'node:assert/strict';
import {createRuntime,createSyntheticRuntime,internalKey} from './runtime-helper.mjs';
import {seedEarlier} from './earlier-fixture.mjs';

export async function conceptHomeWalk(browser,capture){
 const results=[];
 const cases=[...[320,390,1280].flatMap(width=>['light','dark'].map(scheme=>({width,scheme,js:true}))),{width:390,scheme:'light',js:false}];
 for(const {width,scheme,js} of cases){
  const name=`concept-home-${width}-${scheme}${js?'':'-no-js'}`;
  const row={name,width,scheme,javascript:js,input:width<1280?'native touch':'native keyboard',zoom:[100,200],status:'pass',evidence:[],scope:'Synthetic fixture only; no live provider, physical phone or production acceptance. 200% CSS layout zoom.'};results.push(row);
  // One isolated space per stage: unused, or holding one stored state of the
  // earlier concept notebook loaded from its fixture.
  const stage=async(label,seed,run)=>{
   const worker=`${name}-${label}`;
   const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:'synthetic-concept-home',INTERNAL_KEY:internalKey};
   const runtime=await createRuntime({bindings,name:worker,port:0,inspect:true});
   const url=(await runtime.mf.ready).origin;await runtime.rebind({...bindings,CANONICAL_ORIGIN:url});
   const context=await browser.newContext({viewport:{width,height:844},colorScheme:scheme,javaScriptEnabled:js,isMobile:width<1280,hasTouch:width<1280});
   const page=await context.newPage();page.setDefaultTimeout(12000);
   const errors=[];page.on('pageerror',error=>errors.push(error.message));
   const storage=await runtime.mf.unsafeGetDurableObjectStorage(worker,'LearningSpace',{name:bindings.SPACE_NAME});
   if(seed)await seedEarlier(storage,seed,await runtime.mf.getR2Bucket('SCRY_ASSETS'));
   const rows=async()=>Object.fromEntries((await storage.exec('SELECT key,value FROM records ORDER BY key')).map(r=>[r.key,r.value]));
   const state=async()=>{
    const records=await storage.exec('SELECT key,value FROM records ORDER BY key');
    const meta=JSON.parse(records.find(r=>r.key==='meta').value);
    return {occurrence:meta.occurrence,practice:meta.practice??null,records:records.filter(r=>/^(goals\/|questions\/|concepts\/|events\/|overrides\/|resets\/|assessments\/|practice_|study_version_refs\/)/.test(r.key))};
   };
   const stable=async before=>assert.deepEqual(await state(),before,'Home, reference and history navigation must not select, reveal, grade or mutate anything');
   const activate=async locator=>{
    if(width<1280){await locator.tap();return;}
    // Move actual keyboard focus through the native tab order, not DOM click().
    for(let i=0;i<40;i++){
     if(await locator.evaluate(node=>node===document.activeElement)){await page.keyboard.press('Enter');return;}
     await page.keyboard.press('Tab');
    }
    assert.fail('The requested control is not reachable through the keyboard tab order');
   };
   const home=()=>page.locator('[data-state="mixed-home"]');
   const headerHome=()=>page.getByRole('link',{name:'Scry, home',exact:true});
   const earlier=async()=>{await activate(page.locator('.account-menu>summary'));await activate(page.getByRole('link',{name:'Earlier practice',exact:true}));};
   const layout=async()=>{
    await page.evaluate(()=>document.fonts.ready);
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,`${name}: horizontal overflow`);
    // Closed native details retain descendant boxes in Chromium; only the summary
    // is a visible control until expanded. Expanded links are checked below too.
    const clipped=await page.locator('.masthead>a,.masthead summary,.masthead details[open] a,main .button').evaluateAll(nodes=>nodes.map(node=>{const box=node.getBoundingClientRect();return {text:node.textContent,left:box.left,right:box.right,viewport:innerWidth,zoom:getComputedStyle(document.documentElement).zoom};}).filter(box=>box.left<-.5||box.right>box.viewport+.5));
    assert.deepEqual(clipped,[],`${name}: clipped primary controls`);
   };
   const shot=async suffix=>row.evidence.push(await capture(page,`${name}-${suffix}.png`));
   // Earlier practice and its references offer nothing to answer, dismiss,
   // archive, edit, flag, focus or pause: no form at all.
   const readOnly=async()=>{assert.equal(await page.locator('main form,main button,main textarea,main [data-choice],main [data-poll]').count(),0);assert.equal(await page.getByRole('button',{name:/Next question|Check my answer|^Check$|Practice this idea|Look it up|Archive|Focus|Pause/}).count(),0);};
   try{await run({page,url,rows,state,stable,activate,home,headerHome,earlier,layout,shot,readOnly});assert.deepEqual(errors,[],'Browser errors');}
   catch(error){row.status='fail';row.reason=`${label}: ${error.message}`;console.error(`${name} ${label}: ${error.message}`);await shot(`${label}-failure`);}
   finally{await context.close();await runtime.mf.dispose();}
  };
  await stage('empty',null,async({page,url,state,stable,activate,home,headerHome,layout,shot})=>{
   const menuLayout=async suffix=>{
    const menu=page.locator('.account-menu');
    assert.equal(await menu.getAttribute('open'),null,'Menu starts closed');
    await activate(menu.locator('summary'));
    assert.notEqual(await menu.getAttribute('open'),null,'Native menu opens');
    const links=menu.getByRole('link');
    assert.equal(await links.count(),7,'All retained menu destinations are exposed');
    for(const link of await links.all())assert.equal(await link.isVisible(),true,'Expanded menu link is visible');
    await layout();await shot(suffix);
    await activate(menu.locator('summary'));
    assert.equal(await menu.getAttribute('open'),null,'Native menu closes');
   };
   await page.goto(url+'/');await home().waitFor();await page.getByRole('button',{name:'Study',exact:true}).waitFor();assert.equal(await page.locator('.lesson-picker').getAttribute('open'),null);assert.equal(await home().getByRole('heading').count(),1);
   assert.deepEqual(await page.locator('.masthead nav a').allTextContents(),['Practice','Lessons','Library','History','Settings','Earlier practice','Changelog']);
   assert.equal(await page.locator('footer').count(),0);
   const empty=await state();assert.equal(empty.occurrence,null);assert.equal(empty.practice?.current??null,null);
   await layout();await shot('empty');await menuLayout('menu');
   await page.evaluate(()=>document.documentElement.style.zoom='2');await layout();await shot('empty-200');await menuLayout('menu-200');
   await activate(page.locator('.account-menu>summary'));await activate(page.getByRole('link',{name:'Library',exact:true}));
   await page.locator('[data-state="library"]').waitFor();
   assert.equal(await page.locator('form[action="/create"],input[type="file"]').count(),0);
   await page.evaluate(()=>document.documentElement.style.zoom='2');await layout();await shot('unavailable-200');
   await activate(headerHome());await home().waitFor();await stable(empty);
   await page.reload();await home().waitFor();await stable(empty);
   await page.goBack();await page.locator('[data-state="library"]').waitFor();await page.goForward();await home().waitFor();await stable(empty);
   // An unused notebook's Earlier practice says so and selects nothing.
   await page.goto(url+'/notebook');await page.locator('[data-state="earlier-empty"]').waitFor();await page.getByText('Nothing is saved in the earlier concept notebook.',{exact:true}).waitFor();await stable(empty);
  });
  // A question that was left unanswered: shown as unanswered, never on Home,
  // and nothing about it is hidden or changed by looking.
  await stage('unanswered','question-cold-choice',async({page,url,rows,state,stable,activate,home,headerHome,earlier,layout,shot,readOnly})=>{
   const loaded=await rows();
   await page.goto(url+'/notebook');await page.locator('[data-state="earlier-unanswered"]').waitFor();await readOnly();
   const question=await state();assert.equal(question.occurrence.phase,'question');assert.equal(question.practice?.current??null,null);
   await page.getByRole('heading',{name:'Not answered.',exact:true}).waitFor();assert.equal(await page.locator('main ol li').count(),question.occurrence.presentation.choices.length);
   await layout();await shot('unanswered');
   await activate(headerHome());await home().waitFor();
   const markup=await home().innerText();
   for(const hidden of [question.occurrence.presentation.answer,question.occurrence.presentation.explanation,question.occurrence.presentation.prompt])assert.ok(!markup.includes(hidden),'Home never shows the earlier notebook');
   assert.equal(await page.getByRole('link',{name:'Earlier practice',exact:true,includeHidden:true}).getAttribute('href'),'/notebook');
   await stable(question);await page.evaluate(()=>document.documentElement.style.zoom='2');await layout();await shot('question-home-200');
   await page.reload();await home().waitFor();await stable(question);
   await page.goBack();await page.locator('[data-state="earlier-unanswered"]').waitFor();await stable(question);
   await page.goForward();await home().waitFor();await stable(question);
   await earlier();await page.locator('[data-state="earlier-unanswered"]').waitFor();await stable(question);
   // Its Concept is one tap away and shows the saved answer: nothing is gated.
   await activate(page.locator('main a.concept-chip'));await page.locator('[data-state="concept"]').waitFor();await readOnly();
   assert.ok((await page.locator('main').innerText()).includes(question.occurrence.presentation.answer)||await page.locator('.question-reference').count()>0);
   await page.goto(url+'/history');await page.locator('[data-state="history"]').waitFor();
   assert.deepEqual(await rows(),loaded,'Reading the earlier notebook writes nothing');
  });
  // A held result with its references: prerequisite links both ways.
  await stage('result','result-held',async({page,url,rows,state,stable,activate,home,headerHome,earlier,layout,shot,readOnly})=>{
   const loaded=await rows();
   await page.goto(url+'/notebook');await page.locator('[data-state="earlier-result"]').waitFor();await readOnly();
   const held=await state();assert.equal(held.occurrence.phase,'result');
   const answerPair=await page.locator('.answer-pair').innerText();
   await activate(headerHome());await home().waitFor();await stable(held);await page.evaluate(()=>document.documentElement.style.zoom='2');await layout();await shot('held-home-200');
   await page.reload();await home().waitFor();await stable(held);
   await earlier();await page.locator('[data-state="earlier-result"]').waitFor();assert.equal(await page.locator('.answer-pair').innerText(),answerPair);await stable(held);
   await page.goBack();await home().waitFor();await page.goForward();await page.locator('[data-state="earlier-result"]').waitFor();await stable(held);
   const concepts=held.records.filter(r=>r.key.startsWith('concepts/')).map(r=>JSON.parse(r.value));
   const dependent=concepts.find(c=>c.prerequisites.length);assert.ok(dependent,'The fixture retains its real prerequisite relation');
   const prerequisite=concepts.find(c=>c.id===dependent.prerequisites[0]);assert.ok(prerequisite);
   await page.goto(url+'/map');await page.locator('[data-state="map"]').waitFor();await activate(page.locator(`a[href="/concepts/${prerequisite.id}"]`).first());await page.locator('[data-state="concept"]').waitFor();await readOnly();
   for(const heading of ['Notes','Recorded answers','Resources','Related','Used by'])await page.getByRole('heading',{name:heading,exact:true}).waitFor();
   assert.equal(await page.getByRole('link',{name:'Original material & preparation',exact:true}).getAttribute('href'),`/goals/${prerequisite.goal_id}`);
   const reverse=page.locator('.connected').getByRole('link',{name:dependent.name,exact:true});assert.equal(await reverse.getAttribute('href'),`/concepts/${dependent.id}`);
   await layout();await shot('concept-used-by');await page.evaluate(()=>document.documentElement.style.zoom='2');await layout();await shot('concept-used-by-200');
   await activate(reverse);await page.getByRole('heading',{name:'Builds on',exact:true}).waitFor();assert.equal(await page.locator('.connected').getByRole('link',{name:prerequisite.name,exact:true}).getAttribute('href'),`/concepts/${prerequisite.id}`);
   await activate(page.getByRole('link',{name:'Original material & preparation',exact:true}));
   await page.locator('[data-state="goal"]').waitFor();await readOnly();assert.equal(await page.getByRole('link',{name:/Go to practice/}).count(),0);
   await page.goto(url+'/history');await page.locator('[data-state="history"]').waitFor();
   assert.equal(await page.locator('.history-entry').count(),held.records.filter(r=>r.key.startsWith('events/')).length);
   await page.goto(url+'/notebook');await page.locator('[data-state="earlier-result"]').waitFor();
   // Reading references records nothing: every row is exactly as loaded.
   await stable(held);assert.deepEqual(await rows(),loaded,'Reading the earlier notebook writes nothing');
   // Archiving a Goal is retired: its page offers no control, and the same
   // POST through the real Worker answers 410 and changes no row.
   await page.goto(url+`/goals/${prerequisite.goal_id}`);await page.locator('[data-state="goal"]').waitFor();await readOnly();
   const csrf=JSON.parse(loaded.meta).csrf;
   const refused=await page.request.post(url+`/goals/${prerequisite.goal_id}/archive`,{form:{csrf,operation_id:'concept-home-archive-'+Date.now()},headers:{Origin:url},maxRedirects:0});
   assert.equal(refused.status(),410);assert.match(await refused.text(),/The earlier concept notebook is kept as history\./);
   assert.deepEqual(await rows(),loaded,'A refused archive writes nothing');
   await page.goto(url+'/map');await page.locator('[data-state="map"]').waitFor();
   assert.ok(await page.locator(`a[href="/goals/${prerequisite.goal_id}"]`).count()>0,'The Goal is still on the Map');
  });
  // A held result whose Goal was archived afterwards keeps its references.
  await stage('archived','result-then-archived',async({page,url,rows,state,stable,activate,home,headerHome,shot,readOnly})=>{
   const loaded=await rows(),held=await state();
   const goals=held.records.filter(r=>r.key.startsWith('goals/')).map(r=>JSON.parse(r.value)).filter(goal=>goal.archived);assert.equal(goals.length,1);
   await page.goto(url+'/map');await page.locator('[data-state="map"]').waitFor();
   // It stays on the Map, after the live Goals and labelled archived.
   const archived=page.locator('.goal-map').filter({has:page.locator(`h2 a[href="/goals/${goals[0].id}"]`)});
   assert.equal(await archived.count(),1);assert.match(await archived.locator('.goal-meta').innerText(),/ · Archived/);
   await activate(headerHome());await home().waitFor();
   assert.equal(await home().locator('.home-concepts a').count(),0);assert.equal(await home().getByRole('link',{name:'Start practice',exact:true}).count(),0);
   await stable(held);await shot('archived-goal-home');
   for(const concept of held.records.filter(r=>r.key.startsWith('concepts/')).map(r=>JSON.parse(r.value))){
    await page.goto(url+`/concepts/${concept.id}`);await page.getByRole('heading',{name:concept.name,exact:true}).waitFor();
    await page.getByText('Archived: retained for reference.',{exact:true}).waitFor();await readOnly();
    // Its links to the other archived Concepts stay, each labelled archived.
    const links=await page.locator('.connected a.concept-chip').count();
    assert.equal(await page.locator('.connected .related-archived').count(),links);
   }
   await page.goto(url+'/notebook');await page.locator('[data-state="earlier-result"]').waitFor();await readOnly();
   assert.deepEqual(await rows(),loaded,'Reading the earlier notebook writes nothing');
  });
 }
 // Home intentionally does not poll retired generation or select either stream.
 const runtime=await createSyntheticRuntime({name:'review-home-refusal'});
 const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true});
 const page=await context.newPage();
 const row={name:'review-home-retired-request',status:'pass',evidence:[]};results.push(row);
 try {
  const read=async()=>(await(await context.request.get(runtime.url+'/export')).json()).app;
  const before=await read();
  const response=await context.request.post(runtime.url+'/create',{form:{csrf:before.csrf,operation_id:'home-retired-request',intent:'Unsent material'},headers:{Origin:runtime.url},maxRedirects:0});
  assert.equal(response.status(),410);assert.deepEqual(await read(),before);
  await page.goto(runtime.url+'/create?text=Unsent%20material');
  await page.getByText('Native content generation is retired.',{exact:true}).waitFor();
  assert.equal(await page.getByLabel('Your unsent material',{exact:true}).inputValue(),'Unsent material');
  assert.equal(await page.locator('form[action="/create"],input[type="file"]').count(),0);
  await page.getByRole('link',{name:'Scry, home',exact:true}).tap();
  await page.locator('[data-state="mixed-home"]').waitFor();assert.equal(await page.locator('[data-poll]').count(),0);
  assert.deepEqual(await read(),before);row.evidence.push(await capture(page,'review-home-retired-request.png'));
 }catch(error){row.status='fail';row.reason=error.message;}
 finally{await context.close();await runtime.mf.dispose();}
 return results;
}
