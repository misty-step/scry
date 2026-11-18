// The former Create journey now proves its explicit retirement, not model
// orchestration. Historical generation policy remains in the native suite.
import assert from 'node:assert/strict';
import {createSyntheticRuntime,syntheticSpace} from './runtime-helper.mjs';
import {seedEarlier,storedApp} from './earlier-fixture.mjs';

export async function createWalk(browser,capture) {
 const result={name:'generation-self-grading-and-earlier-practice-retired',status:'pass',evidence:[],scope:'Synthetic existing content, no provider calls or production state.'};
 for(const javaScriptEnabled of [true,false]) {
  const worker=`retirement-browser-${javaScriptEnabled}`;
  const runtime=await createSyntheticRuntime({name:worker,inspect:true});
  const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,javaScriptEnabled});
  const page=await context.newPage();page.setDefaultTimeout(12000);
  let secret='';
  // The authenticated API export is the observation behind every whole-App check.
  const read=async() => {const response=await context.request.get(runtime.url+'/api/v1/export',{headers:{Authorization:'Bearer '+secret}});assert.equal(response.status(),200);return(await response.json()).app;};
  try {
   // An existing earlier concept notebook with a held result, as stored.
   await seedEarlier(await runtime.mf.unsafeGetDurableObjectStorage(worker,'LearningSpace',{name:syntheticSpace}),'result-held',await runtime.mf.getR2Bucket('SCRY_ASSETS'));
   await page.goto(runtime.url+'/notebook');await page.locator('[data-state="earlier-result"]').waitFor();
   const settings=await(await context.request.get(runtime.url+'/settings')).text();
   const form=/<form[^>]*action="\/tokens\/create"[^>]*>([\s\S]*?)<\/form>/.exec(settings);assert.ok(form);
   const fields=Object.fromEntries([...form[1].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
   const minted=await context.request.post(runtime.url+'/tokens/create',{form:{...fields,name:'Synthetic retirement readback'},headers:{Origin:runtime.url},maxRedirects:0});assert.equal(minted.status(),200);
   secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())?.[1];assert.ok(secret);
   const before=await read();assert.deepEqual(before.events,storedApp('result-held').events);assert.equal(before.events.at(-1).result.authority,'exact');
   assert.deepEqual(await read(),before,'The authenticated observation itself must not mutate the App.');
   await page.goto(runtime.url+'/create?text=An%20unsent%20legacy%20draft');
   await page.getByText('Native content generation is retired.',{exact:true}).waitFor();
   assert.equal(await page.getByLabel('Your unsent material',{exact:true}).inputValue(),'An unsent legacy draft');
   assert.equal(await page.locator('form[action="/create"],input[type="file"]').count(),0);
   const goal=Object.keys(before.goals)[0],question=Object.keys(before.questions)[0];
   const concept=Object.keys(before.concepts)[0];
   for(const path of ['/create','/add',`/goals/${goal}/retry`,`/goals/${goal}/refine`,`/questions/${question}/fix`,'/review/self','/review/override','/practice/correct','/review/answer','/review/next','/review/reveal','/review/retry','/review/help','/review/intro',`/concepts/${concept}/practice`]) {
    const response=await context.request.post(runtime.url+path,{form:{csrf:before.csrf,operation_id:'browser-retired-action',occurrence_id:before.occurrence.id,answer:'304',known:'false',intent:'Never generate',feedback:'Never refine',correct:'true',verdict:'Good'},headers:{Origin:runtime.url},maxRedirects:0});
    assert.equal(response.status(),410,path);assert.match(await response.text(),/retired|kept as history/);
    assert.deepEqual(await read(),before,'Refusal cannot change earlier material, result, receipt or schedule');
   }
   // The held result is still shown, read-only: no Next, no Correction.
   await page.goto(runtime.url+'/notebook');await page.locator('[data-state="earlier-result"]').waitFor();
   assert.equal(await page.locator('main form,main button').count(),0);
   // Native grading goes on in Study: a starter, Try from memory, an exact answer.
   await page.goto(runtime.url+'/?practice=nato');await page.locator('form[action="/practice/nato/start"] button').tap();
   await page.locator('[data-study-phase="study"]').waitFor();await page.locator('form[action="/practice/quiz"] button').tap();
   await page.locator('#answer').fill('Alfa');await page.locator('form[action="/practice/answer"] button.primary').tap();
   await page.locator('[data-state="exact-result"] [data-verdict="Good"]').waitFor();
   const after=await read();assert.equal(Object.keys(after.practice.attempts).length,1);
   for(const part of ['events','questions','concepts','goals','assessments','occurrence','jobs','spend','overrides'])assert.deepEqual(after[part],before[part],`Study leaves the earlier notebook's ${part} as stored`);
   assert.deepEqual(after.overrides,[]);
   result.evidence.push(await capture(page,`retired-generation-native-grading-${javaScriptEnabled?'js':'no-js'}.png`));
  }catch(error){result.status='fail';result.reason=error.message;result.evidence.push(await capture(page,`retired-generation-failure-${javaScriptEnabled}.png`));}
  finally{await context.close();await runtime.mf.dispose();}
 }
 return result;
}
