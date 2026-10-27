// The former Create journey now proves its explicit retirement, not model
// orchestration. Historical generation policy remains in the native suite.
import assert from 'node:assert/strict';
import {createSyntheticRuntime} from './runtime-helper.mjs';

export async function createWalk(browser,capture) {
 const result={name:'generation-and-self-grading-retired',status:'pass',evidence:[],scope:'Synthetic existing content, no provider calls or production state.'};
 for(const javaScriptEnabled of [true,false]) {
  const runtime=await createSyntheticRuntime({name:`retirement-browser-${javaScriptEnabled}`});
  const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,javaScriptEnabled});
  const page=await context.newPage();page.setDefaultTimeout(12000);
  const read=async() => {const response=await context.request.get(runtime.url+'/export');assert.equal(response.status(),200);return(await response.json()).app;};
  try {
   await page.goto(runtime.url+'/notebook');
   await page.getByRole('button',{name:'Explore an authored demo',exact:true}).tap();
   await page.locator('[data-state="question"]').waitFor();
   await page.locator('.choice').first().tap();await page.locator('[data-state="result"]').waitFor();
   const before=await read();assert.equal(before.events.length,1);assert.equal(before.events[0].result.authority,'exact');
   await page.goto(runtime.url+'/create?text=An%20unsent%20legacy%20draft');
   await page.getByText('Native content generation is retired.',{exact:true}).waitFor();
   assert.equal(await page.getByLabel('Your unsent material',{exact:true}).inputValue(),'An unsent legacy draft');
   assert.equal(await page.locator('form[action="/create"],input[type="file"]').count(),0);
   const goal=Object.keys(before.goals)[0],question=Object.keys(before.questions)[0];
   for(const path of ['/create','/add',`/goals/${goal}/retry`,`/goals/${goal}/refine`,`/questions/${question}/fix`,'/review/self','/review/override','/practice/correct']) {
    const response=await context.request.post(runtime.url+path,{form:{csrf:before.csrf,operation_id:'browser-retired-action',intent:'Never generate',feedback:'Never refine',correct:'true',verdict:'Good'},headers:{Origin:runtime.url},maxRedirects:0});
    assert.equal(response.status(),410,path);assert.match(await response.text(),/retired/);
    assert.deepEqual(await read(),before,'Refusal cannot change earlier material, result, receipt or schedule');
   }
   await page.goto(runtime.url+'/notebook');await page.locator('[data-state="result"]').waitFor();
   assert.equal(await page.locator('form[action="/review/override"],form[action="/review/self"]').count(),0);
   await page.getByRole('button',{name:'Next question',exact:false}).tap();
   await page.locator('#answer').fill('max-age');await page.getByRole('button',{name:'Check my answer',exact:false}).tap();
   await page.locator('[data-state="result"]').waitFor();
   const after=await read();assert.equal(after.events.length,2);assert.equal(after.events.at(-1).result.authority,'exact');
   assert.deepEqual(after.jobs,before.jobs);assert.deepEqual(after.spend,[]);assert.deepEqual(after.overrides,[]);
   result.evidence.push(await capture(page,`retired-generation-native-grading-${javaScriptEnabled?'js':'no-js'}.png`));
  }catch(error){result.status='fail';result.reason=error.message;result.evidence.push(await capture(page,`retired-generation-failure-${javaScriptEnabled}.png`));}
  finally{await context.close();await runtime.mf.dispose();}
 }
 return result;
}
