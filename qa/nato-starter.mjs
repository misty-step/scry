// Explicit, supplied starter through the same real native Library/Study forms.
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {createRuntime,internalKey} from './runtime-helper.mjs';
export function nativeFields(html,action){
 const body=html.split('<form').find(part=>part.startsWith(` method="post" action="${action}"`))?.split('</form>')[0];assert.ok(body,`Native ${action} form exists`);
 return Object.fromEntries([...body.matchAll(/<input\b[^>]*>/g)].filter(([tag])=>tag.includes('type="hidden"')||tag.includes(' checked')).map(([tag])=>[ /name="([^"]+)"/.exec(tag)[1], /value="([^"]*)"/.exec(tag)[1] ]));
}
export async function starterFixture(){
 const name='nato-starter-'+randomUUID(),bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey};let egress=0;
 let runtime=await createRuntime({port:0,name,inspect:true,bindings,egress:async()=>{egress++;throw Error('No egress in supplied starter proof.');}});
 try{
  const origin=(await runtime.mf.ready).origin;runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin})};
  const sql=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
  const rows=async()=>Object.fromEntries((await sql.exec('SELECT key,value FROM records ORDER BY key')).map(r=>[r.key,JSON.parse(r.value)]));
  const get=async path=>{const r=await runtime.worker.fetch(origin+path);assert.equal(r.status,200);return r.text();};
  const post=(path,fields)=>runtime.worker.fetch(origin+path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(fields)});
  return {origin,rows,get,post,get egress(){return egress;},close:()=>runtime.mf.dispose()};
 }catch(e){await runtime.mf.dispose();throw e;}
}
export async function starterJourney(){
 const f=await starterFixture();
 try{
  const html=await f.get('/library'),initial=await f.rows();assert.match(html,/Add NATO alphabet/);assert.doesNotMatch(html,/Alfa|Bravo/);
  await f.get('/library');assert.deepEqual(await f.rows(),initial,'Read never imports content.');
  const add=nativeFields(html,'/library/mapping/add');assert.equal(add.starter,'nato-v1');assert.ok(add.revision);assert.ok(!('data'in add));
  for(const override of [{starter:'nato-v2'},{data:'{}'},{grade:'Good'},{generate:'true'}]){const before=await f.rows();const r=await f.post('/library/mapping/add',{...add,...override,operation_id:randomUUID()});assert.equal(r.status,422);assert.deepEqual(await f.rows(),before);}
  const missing={...add};delete missing.csrf;assert.equal((await f.post('/library/mapping/add',missing)).status,403);
  const saved=await f.post('/library/mapping/add',add);assert.equal(saved.status,303);const path=saved.headers.get('location');assert.match(path,/^\/library\/versions\/[a-f0-9]{32}$/);
  const detail=await f.get(path);assert.match(detail,/<dt>A<\/dt><dd>Alfa<\/dd>/);assert.equal([...detail.matchAll(/<dt>([A-Z])<\/dt><dd>/g)].map(match=>match[1]).join(''),'ABCDEFGHIJKLMNOPQRSTUVWXYZ');const afterAdd=await f.rows();
  assert.equal((await f.post('/library/mapping/add',add)).headers.get('location'),path);
  assert.deepEqual(Object.fromEntries(Object.entries(await f.rows()).filter(([key])=>key!=='meta')),Object.fromEntries(Object.entries(afterAdd).filter(([key])=>key!=='meta')));
  const learn=await f.post('/practice/learn',nativeFields(detail,'/practice/learn'));assert.equal(learn.status,303);assert.equal(learn.headers.get('location'),'/recall');
  const prompt=await f.get('/recall');assert.match(prompt,/data-state="exact-prompt"/);assert.doesNotMatch(prompt,/Alfa|Bravo/);
  const answer=nativeFields(prompt,'/practice/answer');assert.equal((await f.post('/practice/answer',{...answer,answer:'Alfa'})).status,303);
  const result=await f.get('/recall');assert.match(result,/data-verdict="Good"/);assert.match(result,/Alfa/);assert.doesNotMatch(result,/I was right|Count as a miss/);
  const domain=await f.rows(),again=nativeFields(await f.get('/library'),'/library/mapping/add');assert.equal((await f.post('/library/mapping/add',again)).headers.get('location'),path);
  const repeated=await f.rows();for(const [key,value]of Object.entries(domain))if(key!=='meta')assert.deepEqual(repeated[key],value,`Repeated Add retains ${key}`);
  const content=rows=>Object.fromEntries(Object.entries(rows).filter(([key])=>key.startsWith('library_')||key.startsWith('practice_')));assert.deepEqual(content(repeated),content(domain));assert.deepEqual(repeated.meta.practice,domain.meta.practice);assert.equal(f.egress,0);
  return {status:'pass',native_forms:true,explicit_add:true,pairs:26,repeat_preserves_history:true,client_override_refused:true,egress:0};
 }finally{await f.close();}
}
// The main product journey now begins on Home, not in Library/Version setup.
export async function focusedJourney(){
 const f=await starterFixture();
 try{
  const initial=await f.get('/'),before=await f.rows();assert.match(initial,/data-state="nato-home"/);assert.match(initial,/Start practice/);
  await f.get('/');assert.deepEqual(await f.rows(),before,'Home reads do not import or select.');
  const start=nativeFields(initial,'/practice/nato/start');
  for(const extra of [{grade:'Good'},{answer:'Alfa'},{pace:'light'}]){const saved=await f.rows();assert.equal((await f.post('/practice/nato/start',{...start,...extra,operation_id:randomUUID()})).status,422);assert.deepEqual(await f.rows(),saved);}
  const missing={...start};delete missing.csrf;assert.equal((await f.post('/practice/nato/start',missing)).status,403);
  const starts=await Promise.all([f.post('/practice/nato/start',start),f.post('/practice/nato/start',start)]);assert.ok(starts.every(r=>r.status===303));
  const prompt=await f.get('/?practice=nato');assert.match(prompt,/data-state="exact-prompt"/);assert.doesNotMatch(prompt,/Alfa|Bravo|Alphabet reference/);
  const started=await f.rows();assert.equal(Object.keys(started).filter(k=>k.startsWith('practice_studies/')).length,1);assert.equal(Object.keys(started).filter(k=>k.startsWith('library_versions/')).length,1);
  assert.equal((await f.post('/practice/nato/start',{...start,operation_id:randomUUID()})).status,303);const resumed=await f.rows();assert.deepEqual(resumed.meta.practice,started.meta.practice);
  const answer={...nativeFields(prompt,'/practice/answer'),answer:'Alpha'};
  const answers=await Promise.all([f.post('/practice/answer',answer),f.post('/practice/answer',answer)]);assert.ok(answers.every(r=>r.status===303));
  const result=await f.get('/');assert.match(result,/data-verdict="Again"/);assert.match(result,/Alfa/);assert.match(result,/1<span class="nato-muted"> \/ 26 practiced/);
  const graded=await f.rows();assert.equal(Object.keys(graded).filter(k=>k.startsWith('practice_attempts/')).length,1);
  await f.get('/');await f.get('/?practice=nato');assert.deepEqual((await f.rows()).meta.practice,graded.meta.practice,'Reload holds result.');
  // Real wall clock: reading feedback beyond Again's one-minute due time
  // must not send the learner straight back to Alfa. No synthetic clock override.
  await new Promise(resolve=>setTimeout(resolve,65_000));
  assert.equal((await f.post('/practice/next',nativeFields(result,'/practice/next'))).status,303);
  const next=await f.get('/?practice=nato');assert.match(next,/aria-label="Letter B"/);assert.doesNotMatch(next,/Alfa|Bravo|Alphabet reference/);
  assert.equal((await f.post('/practice/reveal',nativeFields(next,'/practice/reveal'))).status,303);
  const disclosed=await f.get('/?practice=nato');assert.match(disclosed,/Bravo/);assert.match(disclosed,/1<span class="nato-muted"> \/ 26 practiced/);
  assert.equal((await f.post('/practice/answer',{...nativeFields(disclosed,'/practice/answer'),answer:'Bravo'})).status,303);
  const assisted=await f.get('/?practice=nato');assert.match(assisted,/With help/);assert.match(assisted,/1<span class="nato-muted"> \/ 26 practiced/);
  assert.equal((await f.post('/practice/next',nativeFields(assisted,'/practice/next'))).status,303);
  const afterHelp=await f.get('/?practice=nato');assert.match(afterHelp,/aria-label="Letter C"/);assert.doesNotMatch(afterHelp,/Bravo|Charlie/);
  assert.equal(f.egress,0);return {status:'pass',delayed_next_advances:true,assisted_next_advances:true,one_action_start:true,simultaneous_start_and_answer_idempotent:true,strict_server_grading:true,held_on_refresh:true,assistance_does_not_count:true,actual_unique_practiced:1,egress:0};
 }finally{await f.close();}
}
export async function starterWalk(browser,capture){
 const rows=[];
 for(const [width,noJS,scheme]of [[390,false,'light'],[1280,false,'light'],[320,true,'dark']]){
  const f=await starterFixture(),context=await browser.newContext({viewport:{width,height:844},isMobile:width<1000,hasTouch:width<1000,javaScriptEnabled:!noJS,colorScheme:scheme,reducedMotion:'reduce'});
  const row={status:'pass',width,noJS,scheme,reduced_motion:true,evidence:[]};rows.push(row);
  let page;
  try{
   page=await context.newPage();page.setDefaultTimeout(12000);const errors=[];page.on('pageerror',error=>errors.push(error.message));
   const shot=async name=>row.evidence.push(await capture(page,`nato-focus-${width}-${noJS?'native':'enhanced'}-${name}.png`));
   const layout=async()=>{await page.evaluate(()=>document.fonts.ready);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'No horizontal overflow');};
   const banner=async state=>{
    const geometry=await page.locator('#request-status').evaluate(node=>{const r=node.getBoundingClientRect();return {top:r.top,bottom:r.bottom,height:r.height,viewport:innerHeight};});
    assert.ok(geometry.height>0&&geometry.height<geometry.viewport/2&&geometry.top>geometry.viewport/3,`The ${state} banner is content-sized, not a full-height overlay: ${JSON.stringify(geometry)}`);
    row.banners={...row.banners,[state]:geometry};
   };
   const activate=async button=>width<1000?button.tap():button.click();
   await page.goto(f.origin+'/');await page.getByRole('heading',{name:'From Alfa to Zulu.',exact:true}).waitFor();await layout();await shot('entry');
   await activate(page.getByRole('button',{name:'Start practice',exact:true}));const input=page.getByLabel('Type your answer',{exact:true});await input.waitFor();assert.doesNotMatch(await page.content(),/Alfa|Bravo|Alphabet reference/);await shot('question');
   if(!noJS)assert.equal(await input.evaluate(node=>node===document.activeElement),true,'New prompt receives focus');
   await input.fill('Alpha');
   if(width>=1000)await page.keyboard.press('Enter');else await activate(page.getByRole('button',{name:/^Check/}));
   await page.locator('[data-verdict="Again"]').waitFor();await page.getByText('Alfa',{exact:true}).waitFor();await layout();await shot('correction');
   const graded=await f.rows();assert.equal(Object.keys(graded).filter(k=>k.startsWith('practice_attempts/')).length,1);
   await page.reload();await page.locator('[data-verdict="Again"]').waitFor();assert.deepEqual((await f.rows()).meta.practice,graded.meta.practice);
   if(width===390)await new Promise(resolve=>setTimeout(resolve,65_000));
   if(width>=1000){await page.keyboard.press('Enter');}else await activate(page.getByRole('button',{name:/^Next/}));
   await input.waitFor();assert.equal(await page.locator('.nato-letter').innerText(),'B');assert.doesNotMatch(await page.content(),/Alfa|Bravo/);
   if(!noJS)assert.equal(await input.evaluate(node=>node===document.activeElement),true,'Next restores input focus');
   // Progress inspection is entirely read-only and must preserve a typed draft.
   const beforeInspect=await f.rows();await input.fill('Bra');
   await activate(page.locator('#nato-progress > summary'));
   await activate(page.locator('label[for="progress-A"]'));
   await page.locator('#letter-progress-A').waitFor();
   assert.equal(await page.locator('#letter-progress-A [data-metric="reviews"]').innerText(),'1');
   assert.equal(await page.locator('#letter-progress-A [data-metric="accuracy"]').innerText(),'0%');
   assert.doesNotMatch(await page.content(),/Alfa|Bravo/);
   assert.equal(await input.inputValue(),'Bra');await layout();await shot('progress');
   await activate(page.locator('label[for="progress-Z"]'));
   assert.equal(await page.locator('#letter-progress-Z [data-metric="accuracy"]').innerText(),'—');
   assert.equal(await page.locator('#letter-progress-Z [data-metric="due"]').innerText(),'Not scheduled');
   await activate(page.locator('#nato-progress > summary'));
   assert.equal(await input.inputValue(),'Bra');assert.deepEqual(await f.rows(),beforeInspect,'Inspecting progress never writes or changes the held prompt');
   if(!noJS){
    await input.focus();
    await input.fill('Bravo');const saved=await f.rows();await context.setOffline(true);await activate(page.getByRole('button',{name:/^Check/}));await page.getByText('You are offline. Your draft is still here.',{exact:true}).waitFor();assert.deepEqual(await f.rows(),saved);assert.equal(await input.inputValue(),'Bravo');assert.equal(await page.locator('[data-verdict]').count(),0);await banner('offline');await shot('offline');await context.setOffline(false);
    await page.keyboard.press('Enter');await page.locator('[data-verdict="Good"]').waitFor();assert.equal(Object.keys(await f.rows()).filter(k=>k.startsWith('practice_attempts/')).length,2);
    await page.keyboard.press('Enter');await input.waitFor();assert.equal(await page.locator('.nato-letter').innerText(),'C');
    await input.fill('Not the code word '.repeat(100));await page.keyboard.press('Enter');await page.locator('[data-verdict="Again"]').waitFor();await page.getByText('Charlie',{exact:true}).waitFor();await layout();await shot('long-wrong-answer');
    // A reduced viewport models browser keyboard occlusion, not a physical phone.
    await page.keyboard.press('Enter');await input.waitFor();await page.setViewportSize({width,height:480});await layout();await shot('keyboard-viewport');await page.setViewportSize({width,height:844});
    // Lose only the browser response AFTER the real Worker commits. A retry
    // must reconcile the frozen operation, not invent optimistic success.
    const attemptsBefore=Object.keys(await f.rows()).filter(k=>k.startsWith('practice_attempts/')).length;
    await page.route('**/practice/answer',async route=>{await route.fetch();await page.getByText('Saving…',{exact:true}).waitFor();await banner('saving');await shot('saving');await route.abort('failed');});
    await input.fill('Delta');await page.keyboard.press('Enter');
    await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();
    assert.equal(await page.locator('[data-verdict]').count(),0);assert.equal(await input.inputValue(),'Delta');assert.equal(await input.getAttribute('readonly'),'');
    assert.equal(Object.keys(await f.rows()).filter(k=>k.startsWith('practice_attempts/')).length,attemptsBefore+1);await banner('reconcile');await shot('response-lost');
    await page.unroute('**/practice/answer');await activate(page.getByRole('button',{name:'Reconcile this attempt',exact:true}));
    await page.locator('[data-verdict="Good"]').waitFor();assert.equal(Object.keys(await f.rows()).filter(k=>k.startsWith('practice_attempts/')).length,attemptsBefore+1);row.response_loss_reconciles_once=true;
    await page.keyboard.press('Enter');await input.waitFor();assert.equal(await page.locator('.nato-letter').innerText(),'E');
    await activate(page.getByRole('button',{name:'Show answer',exact:true}));await page.getByText('Echo',{exact:true}).waitFor();
    await input.fill('Echo');await page.keyboard.press('Enter');await page.getByText('With help',{exact:true}).waitFor();
    await page.keyboard.press('Enter');await input.waitFor();assert.equal(await page.locator('.nato-letter').innerText(),'F');await shot('after-assisted-next');
   }else{await input.fill('Bravo');await activate(page.getByRole('button',{name:/^Check/}));await page.locator('[data-verdict="Good"]').waitFor();await shot('good');}
   assert.deepEqual(errors,[]);assert.equal(f.egress,0);row.native_grading=true;row.reload_preserves_result=true;row.egress=0;
  }catch(error){row.status='fail';row.reason=error.stack||error.message;if(page)row.evidence.push(await capture(page,`nato-focus-${width}-${noJS?'native':'enhanced'}-failure.png`));}
  finally{await context.close();await f.close();}
 }
 return rows;
}

// Read actual text/background colors, not token declarations. Legacy form-label
// styles must not override the compact alphabet controls, including selected hover.
async function letterContrast(page,letter){
 const result=await page.locator(`label[for="progress-${letter}"]`).evaluate(label=>{
  const text=getComputedStyle(label.querySelector('span')),surface=getComputedStyle(label);
  const luminance=color=>{
   const channels=color.match(/[\d.]+/g).slice(0,3).map(Number).map(value=>{
    const c=value/255;return c<=.04045?c/12.92:((c+.055)/1.055)**2.4;
   });
   return channels[0]*.2126+channels[1]*.7152+channels[2]*.0722;
  };
  const fg=luminance(text.color),bg=luminance(surface.backgroundColor);
  return {foreground:text.color,background:surface.backgroundColor,ratio:(Math.max(fg,bg)+.05)/(Math.min(fg,bg)+.05),marginLeft:text.marginLeft,font:text.font,fontMatches:text.font===surface.font};
 });
 assert.ok(result.ratio>=4.5,`Letter ${letter} text contrast >=4.5: ${JSON.stringify(result)}`);
 assert.equal(result.marginLeft,'0px','Letter text is centered without legacy label offset');
 assert.equal(result.fontMatches,true,'Letter text inherits the alphabet control typography');
 return result;
}
// Rich progress proof uses actual native answers, never direct database fixtures.
export async function progressWalk(browser,capture){
 const rows=[];
 const words=['Alfa','Bravo','Charlie','Delta','Echo','Foxtrot','Golf','Hotel','India','Juliett','Kilo','Lima','Mike','November','Oscar','Papa','Quebec','Romeo','Sierra','Tango','Uniform','Victor','Whiskey','X-ray','Yankee','Zulu'];
 for(const [width,noJS,scheme] of [[390,false,'light'],[1280,false,'dark'],[320,true,'light']]){
  const f=await starterFixture();const row={status:'pass',width,noJS,scheme,evidence:[]};rows.push(row);
  const context=await browser.newContext({viewport:{width,height:844},isMobile:width<1000,hasTouch:width<1000,javaScriptEnabled:!noJS,colorScheme:scheme,reducedMotion:'reduce'});let page;
  try{
   let html=await f.get('/');await f.post('/practice/nato/start',nativeFields(html,'/practice/nato/start'));
   for(let i=0;i<27;i++){
    html=await f.get('/');
    if(i===4){await f.post('/practice/reveal',nativeFields(html,'/practice/reveal'));html=await f.get('/');}
    const answer=(i%5===0&&i!==26)?'incorrect':words[i%26];
    assert.equal((await f.post('/practice/answer',{...nativeFields(html,'/practice/answer'),answer})).status,303);
    html=await f.get('/');assert.equal((await f.post('/practice/next',nativeFields(html,'/practice/next'))).status,303);
   }
   const before=await f.rows();page=await context.newPage();await page.goto(f.origin+'/');await page.getByLabel('Type your answer',{exact:true}).fill('Draft stays here');
   const activate=async locator=>width<1000?locator.tap():locator.click();
   await activate(page.locator('#nato-progress > summary'));await activate(page.locator('label[for="progress-A"]'));
   assert.equal(await page.locator('#letter-progress-A [data-metric="reviews"]').innerText(),'2');
   assert.equal(await page.locator('#letter-progress-A [data-metric="accuracy"]').innerText(),'50%');
   assert.match(await page.locator('#letter-progress-A').innerText(),/1 of 2 answers/);
   assert.equal(await page.locator('#letter-progress-A .review-mark').count(),2);
   assert.doesNotMatch(await page.content(),/Alfa|Bravo|Charlie|Zulu/);
   await page.evaluate(()=>document.fonts.ready);
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
   await page.waitForTimeout(180); // Settle the control's 140ms color transition.
   row.letter_contrast={selected:await letterContrast(page,'A'),unselected:await letterContrast(page,'B')};
   if(width>=1000){
    await page.locator('label[for="progress-A"]').hover();
    row.letter_contrast.selected_hover=await letterContrast(page,'A');
    await page.locator('label[for="progress-B"]').hover();
    await page.waitForTimeout(180);
    row.letter_contrast.unselected_hover=await letterContrast(page,'B');
    await page.mouse.move(0,0);
   }
   row.evidence.push(await capture(page,`nato-progress-${width}-${noJS?'native':'enhanced'}-${scheme}.png`));
   await activate(page.locator('label[for="progress-E"]'));assert.equal(await page.locator('#letter-progress-E [data-metric="accuracy"]').innerText(),'—');assert.match(await page.locator('#letter-progress-E').innerText(),/1 with help/);
   if(!noJS){
    await page.locator('#progress-E').focus();await page.keyboard.press('ArrowRight');assert.equal(await page.locator('#progress-F').isChecked(),true);assert.equal(await page.getByLabel('Type your answer',{exact:true}).inputValue(),'Draft stays here');
   }
   await activate(page.locator('label[for="progress-A"]'));
   // Snapshot before applying: scaling inherited values sequentially would
   // compound them. This exercises real doubled text, not an unused root size.
   await page.evaluate(()=>{
    const sizes=[...document.querySelectorAll('body, body *')].map(node=>[node,parseFloat(getComputedStyle(node).fontSize)]);
    for(const [node,size] of sizes)node.style.setProperty('font-size',`${size*2}px`,'important');
   });
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'200% text layout fits');
   row.evidence.push(await capture(page,`nato-progress-${width}-text200.png`));
   assert.deepEqual((await f.rows()).meta.practice,before.meta.practice);
   await page.reload();await activate(page.locator('#nato-progress > summary'));await activate(page.locator('label[for="progress-A"]'));
   assert.equal(await page.locator('#letter-progress-A [data-metric="reviews"]').innerText(),'2');
   assert.equal(f.egress,0);row.review_count=27;row.assisted_excluded=true;row.reload_preserves_evidence=true;row.inspection_read_only=true;
  }catch(error){row.status='fail';row.reason=error.stack||error.message;if(page)row.evidence.push(await capture(page,`nato-progress-${width}-failure.png`));}
  finally{await context.close();await f.close();}
 }
 return rows;
}
