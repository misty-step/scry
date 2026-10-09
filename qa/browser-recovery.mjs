// Native-event browser failures in fresh, isolated workerd SQLite spaces.
import assert from 'node:assert/strict';
import {createServer} from 'node:net';
import {createSyntheticRuntime} from './runtime-helper.mjs';
const freePort=async()=>{const s=createServer();await new Promise((ok,bad)=>{s.once('error',bad);s.listen(0,'127.0.0.1',ok);});const p=s.address().port;await new Promise(ok=>s.close(ok));return p;};
export async function recoveryWalk(browser,capture){
 const results=[];
 const check=async(name,options,run)=>{
  const runtime=await createSyntheticRuntime({port:await freePort(),name:`recovery-${results.length}`});
  const context=await browser.newContext({viewport:{width:390,height:844},...options});const page=await context.newPage();page.setDefaultTimeout(12000);
  const result={name,status:'pass',evidence:[]};results.push(result);
  try{await page.goto(runtime.url+'/');await page.getByRole('button',{name:'Explore an authored demo'}).click();await page.locator('[data-state="question"]').waitFor();await run({page,context,url:runtime.url});}
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
  const other=await context.newPage();await other.goto(url+'/');await other.locator('[data-state="question"]').waitFor();await answer(page);await other.locator('.choice').nth(1).click();await other.getByText('This changed in another tab. Your draft is still here.',{exact:true}).waitFor();assert.equal(await other.locator('[data-state="result"]').count(),0);assert.equal((await exported(context,url)).app.events.length,1);await other.close();
 });
 await check('notes require durable assistance',{},async({page,context,url})=>{
  await page.locator('details.overflow>summary').click();await page.getByRole('link',{name:'Look it up in my notes'}).click();await page.locator('[data-state="gate"]').waitFor();assert.equal(await page.locator('.reading').count(),0);await page.getByRole('button',{name:'Look it up',exact:true}).click();await page.locator('[data-state="concept"]').waitFor();await page.goto(url+'/');await answer(page);const state=await exported(context,url);assert.equal(state.app.events[0].assisted,true);assert.equal(state.app.events[0].result.rating,0);assert.equal(await page.getByRole('button',{name:'Count as a miss'}).count(),0);
 });
 await check('private access loss conceals current page',{},async({page,context})=>{
  await context.route('**/review/answer',route=>route.fulfill({status:401,contentType:'text/plain',body:'Owner authorization expired.'}));await page.locator('.choice').first().click();await page.locator('body.private-hidden').waitFor({state:'attached'});assert.equal(await page.locator('#main').count(),0);assert.equal(await page.locator('.question').count(),0);
 });
 await check('320px, desktop and dark layouts',{},async({page})=>{
  for(const width of [320,1280]){await page.setViewportSize({width,height:844});await page.emulateMedia({colorScheme:width===1280?'dark':'light'});await page.evaluate(()=>document.fonts.ready);await page.waitForFunction(()=>{const l=rgb=>{const a=rgb.match(/[\d.]+/g).slice(0,3).map(x=>{x=Number(x)/255;return x<=.04045?x/12.92:((x+.055)/1.055)**2.4;});return .2126*a[0]+.7152*a[1]+.0722*a[2];};return [...document.querySelectorAll('.choice')].every(node=>{const s=getComputedStyle(node),a=l(s.color),b=l(s.backgroundColor);return (Math.max(a,b)+.05)/(Math.min(a,b)+.05)>=4.5;});});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await capture(page,`layout-${width}.png`);}
 });
 return results;
}
