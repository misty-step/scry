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
export async function starterWalk(browser,capture){
 const rows=[];
 for(const [width,noJS,scheme]of [[390,false,'light'],[320,true,'dark']]){
  const f=await starterFixture(),context=await browser.newContext({viewport:{width,height:844},isMobile:true,hasTouch:true,javaScriptEnabled:!noJS,colorScheme:scheme});
  try{
   const page=await context.newPage();page.setDefaultTimeout(12000);await page.goto(f.origin+'/library#nato-starter');
   const add=page.getByRole('button',{name:'Add NATO alphabet',exact:true});await add.waitFor();const entry=await capture(page,`starter-${width}-${noJS?'native':'enhanced'}-entry.png`);await add.tap();await page.getByRole('button',{name:'Learn',exact:true}).waitFor();
   await page.getByRole('button',{name:'Learn',exact:true}).tap();await page.getByLabel('Type your answer',{exact:true}).waitFor();assert.doesNotMatch(await page.content(),/Alfa|Bravo/);
   await page.getByLabel('Type your answer',{exact:true}).fill('Alfa');await page.getByRole('button',{name:'Check my answer',exact:true}).tap();await page.locator('[data-verdict="Good"]').waitFor();
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);const shot=await capture(page,`starter-${width}-${noJS?'native':'enhanced'}-good.png`);
   await page.getByRole('button',{name:'Next',exact:true}).tap();await page.getByLabel('Type your answer',{exact:true}).waitFor();assert.equal((await page.locator('.exact-cue .reading').innerText()).trim(),'B');
   rows.push({status:'pass',width,noJS,scheme,explicit_owner_starter:true,evidence:[entry,shot]});
  }finally{await context.close();await f.close();}
 }
 return rows;
}
