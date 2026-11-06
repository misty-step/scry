// Native-event browser failures in fresh, isolated workerd SQLite spaces.
import assert from 'node:assert/strict';
import {createSyntheticRuntime,internalKey} from './runtime-helper.mjs';
import {createHash} from 'node:crypto';
export async function recoveryWalk(browser,capture){
 const results=[];
 const check=async(name,options,run,seed=true)=>{
  const runtime=await createSyntheticRuntime({name:`recovery-${results.length}`});
  const context=await browser.newContext({viewport:{width:390,height:844},...options});const page=await context.newPage();page.setDefaultTimeout(12000);
  const result={name,status:'pass',evidence:[]};results.push(result);
  try{await page.goto(runtime.url+'/notebook');if(seed){await page.getByRole('button',{name:'Explore an authored demo'}).click();await page.locator('[data-state="question"]').waitFor();}await run({page,context,url:runtime.url,runtime});}
  catch(error){result.status='fail';result.reason=error.message;console.error(`Browser recovery ${name}: ${error.message}`);}
  finally{result.evidence.push(await capture(page,`recovery-${results.length}-${result.status}.png`));await context.close();await runtime.mf.dispose();}
 };
 const answer=async page=>{await page.locator('.choice').first().click();await page.locator('[data-state="result"]').waitFor();};
 const exported=async(context,url)=>{const r=await context.request.get(url+'/export');assert.equal(r.status(),200);return r.json();};
 await check('native forms with JavaScript disabled',{javaScriptEnabled:false},async({page,context,url})=>{await answer(page);await page.reload();await page.locator('[data-state="result"]').waitFor();assert.equal((await exported(context,url)).app.events.length,1);});
 await check('keyboard choice and held result',{},async({page,context,url})=>{await page.getByRole('heading',{level:1}).click();await page.keyboard.press('1');await page.locator('[data-state="result"]').waitFor();assert.equal((await exported(context,url)).app.events.length,1);await page.reload();await page.locator('[data-state="result"]').waitFor();});
 await check('response loss after commit reconciles exact attempt',{},async({page,context,url})=>{
  let first=true;let sends=0;const payloads=[];
  await context.route('**/review/answer',async route=>{if(route.request().method()!=='POST')return route.continue();sends++;payloads.push(route.request().postData());if(first){first=false;const committed=await route.fetch({maxRedirects:0});assert.equal(committed.status(),303);await route.abort('failed');}else await route.continue();});
  await page.locator('.choice').first().click();await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();assert.equal(await page.locator('[data-state="result"]').count(),0);await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).click();await page.locator('[data-state="result"]').waitFor();assert.equal(sends,2);const field=body=>body.match(/name="operation_id"\r\n\r\n([^\r]+)/)?.[1];assert.ok(field(payloads[0]));assert.equal(field(payloads[0]),field(payloads[1]));assert.equal((await exported(context,url)).app.events.length,1);
 });
 await check('offline pauses without automatic writes',{},async({page,context,url})=>{
  await answer(page);await page.getByRole('button',{name:'Next question'}).click();await page.locator('#answer').fill('max-age');let sends=0;context.on('request',request=>{if(request.method()==='POST'&&request.url().endsWith('/review/answer'))sends++;});
  await context.setOffline(true);await page.getByRole('button',{name:'Check my answer'}).click();assert.equal(sends,0);assert.equal(await page.locator('#answer').inputValue(),'max-age');await context.setOffline(false);await page.waitForTimeout(400);assert.equal(sends,0);await page.getByRole('button',{name:'Check my answer'}).click();await page.locator('[data-state="result"]').waitFor();assert.equal(sends,1);assert.equal((await exported(context,url)).app.events.length,2);
 });
 await check('competing tabs reject stale answer',{},async({page,context,url})=>{
  const other=await context.newPage();await other.goto(url+'/notebook');await other.locator('[data-state="question"]').waitFor();await answer(page);await other.locator('.choice').nth(1).click();await other.getByText('This changed in another tab. Your draft is still here.',{exact:true}).waitFor();assert.equal(await other.locator('[data-state="result"]').count(),0);assert.equal((await exported(context,url)).app.events.length,1);await other.close();
 });
 await check('notes require durable assistance',{},async({page,context,url})=>{
  await page.locator('details.overflow>summary').click();await page.getByRole('link',{name:'Look it up in my notes'}).click();await page.locator('[data-state="gate"]').waitFor();assert.equal(await page.locator('.reading').count(),0);await page.getByRole('button',{name:'Look it up',exact:true}).click();await page.locator('[data-state="concept"]').waitFor();await page.goto(url+'/notebook');await answer(page);const state=await exported(context,url);assert.equal(state.app.events[0].assisted,true);assert.equal(state.app.events[0].result.rating,0);assert.equal(await page.getByRole('button',{name:'Count as a miss'}).count(),0);
 });
 await check('private access loss conceals current page',{},async({page,context})=>{
  await context.route('**/review/answer',route=>route.fulfill({status:401,contentType:'text/plain',body:'Owner authorization expired.'}));await page.locator('.choice').first().click();await page.locator('body.private-hidden').waitFor({state:'attached'});assert.equal(await page.locator('#main').count(),0);assert.equal(await page.locator('.question').count(),0);
 });
 await check('background preserves a dirty form and revalidates before revealing it',{},async({page,context,url})=>{
  await answer(page);await page.getByRole('button',{name:'Next question'}).click();
  await page.locator('#answer').fill('A retained synthetic answer');
  let release;const barrier=new Promise(resolve=>{release=resolve});let reached;
  const waiting=new Promise(resolve=>{reached=resolve});
  await context.route('**/session/scope',async route=>{reached();await barrier;await route.continue();});
  await page.evaluate(()=>{Object.defineProperty(document,'hidden',{configurable:true,value:true});document.dispatchEvent(new Event('visibilitychange'));});
  assert.equal(await page.locator('.page-frame').evaluate(el=>el.inert),true);
  await page.evaluate(()=>{Object.defineProperty(document,'hidden',{configurable:true,value:false});document.dispatchEvent(new Event('visibilitychange'));});
  await waiting;assert.equal(await page.locator('.page-frame').evaluate(el=>el.inert),true);
  assert.equal(await page.locator('#answer').inputValue(),'A retained synthetic answer');
  release();await page.waitForFunction(()=>!document.querySelector('.page-frame').inert);
  assert.equal(await page.locator('#answer').inputValue(),'A retained synthetic answer');
 });
 await check('backgrounded mutation stays frozen until manual reconciliation',{},async({page,context,url})=>{
  let sends=0;let release;let reached;
  const barrier=new Promise(resolve=>{release=resolve});
  const committed=new Promise(resolve=>{reached=resolve});
  await context.route('**/review/answer',async route=>{
   if(route.request().method()!=='POST')return route.continue();
   sends++;
   if(sends!==1)return route.continue();
   const response=await route.fetch({maxRedirects:0});assert.equal(response.status(),303);reached();
   await barrier;try{await route.fulfill({response});}catch{};
  });
  await page.locator('.choice').first().click();await committed;
  await page.evaluate(()=>{Object.defineProperty(document,'hidden',{configurable:true,value:true});document.dispatchEvent(new Event('visibilitychange'));});
  await page.evaluate(()=>{Object.defineProperty(document,'hidden',{configurable:true,value:false});document.dispatchEvent(new Event('visibilitychange'));});
  await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();
  release();assert.equal(sends,1,'No mutation is resent automatically.');
  await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).click();
  await page.locator('[data-state="result"]').waitFor();assert.equal(sends,2);
  assert.equal((await exported(context,url)).app.events.length,1);
 });
 await check('scope retry stays reachable after a simulated background on a long page',{},async({page,context})=>{
  await context.route('**/session/scope',route=>route.fulfill({status:503,body:'temporarily unavailable'}));
  await page.evaluate(()=>{const spacer=document.createElement('div');spacer.style.height='3000px';document.querySelector('#main').append(spacer);window.scrollTo(0,document.body.scrollHeight);});
  await page.evaluate(()=>{Object.defineProperty(document,'hidden',{configurable:true,value:true});document.dispatchEvent(new Event('visibilitychange'));Object.defineProperty(document,'hidden',{configurable:true,value:false});document.dispatchEvent(new Event('visibilitychange'));});
  const retry=page.getByRole('button',{name:'Try again',exact:true});await retry.waitFor();
  assert.equal(await retry.evaluate(el=>el===document.activeElement),true);
  const bounds=await retry.boundingBox();assert.ok(bounds && bounds.y>=0 && bounds.y+bounds.height<=844);
  assert.equal(await page.locator('.page-frame').evaluate(el=>el.inert),true);
 });
 await check('scope mismatch on return disposes private state',{},async({page,context})=>{
  await context.route('**/session/scope',route=>route.fulfill({status:409,body:'Notebook changed.'}));
  await page.evaluate(()=>{Object.defineProperty(document,'hidden',{configurable:true,value:true});document.dispatchEvent(new Event('visibilitychange'));Object.defineProperty(document,'hidden',{configurable:true,value:false});document.dispatchEvent(new Event('visibilitychange'));});
  await page.locator('body.private-hidden').waitFor({state:'attached'});
  assert.equal(await page.locator('#main').count(),0);
 });
 await check('malformed gateway scope preserves a frozen answer for manual reconciliation',{},async({page,context,url})=>{
  const payloads=[];let first=true;
  await context.route('**/review/answer',async route=>{
   if(route.request().method()!=='POST')return route.continue();
   payloads.push(route.request().postData());
   if(first){first=false;return route.fulfill({status:503,headers:{'X-Scry-Scope':'malformed'},body:'Synthetic gateway failure'});}
   return route.continue();
  });
  await page.locator('.choice').first().click();
  await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();
  assert.equal(await page.locator('body.private-hidden').count(),0);assert.equal(payloads.length,1);
  await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).click();
  await page.locator('[data-state="result"]').waitFor();
  assert.equal(payloads.length,2);
  const field=(body,name)=>body.match(new RegExp('name="'+name+'"\\r\\n\\r\\n([^\\r]+)'))?.[1];
  for(const name of ['operation_id','occurrence_id','revision','answer']){assert.ok(field(payloads[0],name));assert.equal(field(payloads[0],name),field(payloads[1],name));}
  assert.equal((await exported(context,url)).app.events.length,1);
 });
 await check('HTML rejection missing embedded scope preserves uncertain operation',{},async({page,context,url})=>{
  const scope=await page.locator('html').getAttribute('data-notebook-scope');let sends=0;
  await context.route('**/review/answer',route=>{if(route.request().method()!=='POST')return route.continue();sends++;if(sends===1)return route.fulfill({status:422,headers:{'X-Scry-Scope':scope,'Content-Type':'text/html'},body:'<html><div class="notice">incomplete response</div></html>'});return route.continue();});
  await page.locator('.choice').first().click();await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();
  assert.equal(await page.locator('body.private-hidden').count(),0);assert.equal(sends,1);
  await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).click();
  await page.locator('[data-state="result"]').waitFor();assert.equal(sends,2);assert.equal((await exported(context,url)).app.events.length,1);
 });
 await check('conflicting embedded scope on HTML rejection disposes private state',{},async({page,context})=>{
  const scope=await page.locator('html').getAttribute('data-notebook-scope');
  await context.route('**/review/answer',route=>route.fulfill({status:422,headers:{'X-Scry-Scope':scope,'Content-Type':'text/html'},body:`<html data-notebook-scope="${'b'.repeat(64)}"><div class="notice">wrong notebook</div></html>`}));
  await page.locator('.choice').first().click();await page.locator('body.private-hidden').waitFor({state:'attached'});
  assert.equal(await page.locator('#main').count(),0);
  assert.equal(await page.getByRole('button',{name:'Continue editing',exact:true}).count(),0);
 });
 await check('320px, desktop and dark layouts',{},async({page})=>{
  for(const width of [320,1280]){await page.setViewportSize({width,height:844});await page.emulateMedia({colorScheme:width===1280?'dark':'light'});await page.evaluate(()=>document.fonts.ready);await page.waitForFunction(()=>{const l=rgb=>{const a=rgb.match(/[\d.]+/g).slice(0,3).map(x=>{x=Number(x)/255;return x<=.04045?x/12.92:((x+.055)/1.055)**2.4;});return .2126*a[0]+.7152*a[1]+.0722*a[2];};return [...document.querySelectorAll('.choice')].every(node=>{const s=getComputedStyle(node),a=l(s.color),b=l(s.backgroundColor);return (Math.max(a,b)+.05)/(Math.min(a,b)+.05)>=4.5;});});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await capture(page,`layout-${width}.png`);}
 });
 await check('retired preparation preserves the unsent draft without false progress',{isMobile:true,hasTouch:true},async({page,context,url})=>{
  const before=await exported(context,url);const intent='How do Durable Objects serialize writes across an external API await?';
  await page.goto(url+'/create?text='+encodeURIComponent(intent));
  await page.getByText('Native content generation is retired.',{exact:true}).waitFor();
  assert.equal(await page.getByLabel('Your unsent material',{exact:true}).inputValue(),intent);
  assert.equal(await page.locator('form[action="/create"],input[type="file"]').count(),0);
  const response=await context.request.post(url+'/create',{form:{csrf:before.app.csrf,operation_id:'earlier-retired-recovery-request',intent},headers:{Origin:url},maxRedirects:0});assert.equal(response.status(),410);
  assert.deepEqual((await exported(context,url)).app,before.app);
  await capture(page,'retired-preparation-draft.png');
 },false);
 // A synthetic pre-upgrade tab retains its native form while the real current
 // Worker refuses the write. This is a browser fixture, never a runtime route.
 await check('retired write in an earlier tab unlocks and preserves its exact draft',{},async({page,context,url})=>{
  const before=(await exported(context,url)).app;
  const response=await context.request.get(url+'/create');assert.equal(response.status(),200);
  const form=`<form method="post" action="/create" id="earlier-create"><input type="hidden" name="csrf" value="${before.csrf}"><input type="hidden" name="operation_id" value="earlier-tab-retired-operation"><label for="earlier-intent">Earlier unsent request</label><textarea id="earlier-intent" name="intent"></textarea><button>Save earlier request</button></form>`;
  const body=(await response.text()).replace('</main>',form+'</main>');
  await page.route(url+'/create?stale-tab=1',route=>route.fulfill({response,body}),{times:1});
  await page.goto(url+'/create?stale-tab=1');
  const draft='Keep this unsent earlier-tab draft exactly.';
  await page.getByLabel('Earlier unsent request',{exact:true}).fill(draft);
  let sends=0;page.on('request',request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/create')sends++;});
  const rejected=page.waitForResponse(response=>response.request().method()==='POST'&&new URL(response.url()).pathname==='/create');
  await page.getByRole('button',{name:'Save earlier request',exact:true}).focus();await page.keyboard.press('Enter');
  assert.equal((await rejected).status(),410);
  await page.locator('#request-status').getByText(/Native content generation is retired/).waitFor();
  assert.equal(await page.getByLabel('Earlier unsent request',{exact:true}).inputValue(),draft);
  assert.equal(await page.getByLabel('Earlier unsent request',{exact:true}).isEditable(),true);
  assert.equal(await page.locator('#earlier-create [name="operation_id"]').inputValue(),'earlier-tab-retired-operation');
  assert.equal(await page.locator('#earlier-create[aria-busy]').count(),0);
  assert.equal(await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).count(),0);
  await page.getByRole('button',{name:'Continue editing',exact:true}).click();await page.waitForTimeout(400);
  assert.equal(sends,1,'A definite retirement refusal is not queued or automatically retried.');
  assert.deepEqual((await exported(context,url)).app,before);
 },false);
 await check('restored meaning check resumes through native Settings without JavaScript',{isMobile:true,hasTouch:true,javaScriptEnabled:false},async({page,context,url,runtime})=>{
  await answer(page);
  const archive=Buffer.from(JSON.stringify(await exported(context,url)));
  const restoreToken='synthetic-browser-restore-only-token-32chars';
  let active=await runtime.rebind({SCRY_ENV:'restore',CANONICAL_ORIGIN:url,SPACE_NAME:'restored-meaning-check',INTERNAL_KEY:internalKey,RESTORE_TOKEN:restoreToken});
  const restored=await active.worker.fetch(url+'/operator/restore',{method:'POST',headers:{'x-scry-restore-token':restoreToken,'x-scry-sha256':createHash('sha256').update(archive).digest('hex')},body:archive});assert.equal(restored.status,200);
  await runtime.rebind({SCRY_ENV:'development',CANONICAL_ORIGIN:url,SPACE_NAME:'restored-meaning-check',INTERNAL_KEY:internalKey});
  await page.goto(url+'/notebook');await page.getByRole('button',{name:'Next question'}).tap();await page.locator('#answer').fill('I remember the cache freshness header');await page.getByRole('button',{name:'Check my answer'}).tap();
  await page.getByRole('heading',{name:'Your meaning check is paused.',exact:true}).waitFor();assert.equal(await page.getByRole('link',{name:'Check again',exact:true}).count(),0);await capture(page,'paused-meaning-check.png');
  await page.getByRole('link',{name:'Resume meaning checks',exact:false}).tap();await page.getByRole('button',{name:'Resume meaning checks',exact:true}).tap();
  await page.waitForTimeout(1300);await page.reload();await page.getByRole('heading',{name:'Your answer is saved, not graded.',exact:true}).waitFor();await capture(page,'resumed-meaning-self-check.png');
  assert.equal(await page.getByRole('button',{name:'I was right',exact:true}).count(),0);
  const state=(await exported(context,url)).app;assert.equal(state.restored_paused,false);assert.equal(state.events.length,1);assert.equal(state.occurrence.answer,'I remember the cache freshness header');assert.equal(state.spend.length,0);
 });
 return results;
}
