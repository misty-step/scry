#!/usr/bin/env node
// Write flows against a run-owned synthetic Worker (built WASM, workerd SQLite
// and local R2, no egress) with the compiled CLI and a real browser:
// empty notebook → concept flow → Owner token + CLI login → Library find and
// keep → exact Study on the 390px phone Stream → token revocation → backend down.
//   local-journeys.mjs [--width 390]
// Needs `worker-build --release --locked` (build/) and
// `cargo build --locked --bin scry` (target/debug/scry). Exit 0 PASS, 1 FAIL,
// 2 usage or missing build.
import {spawn} from 'node:child_process';
import {mkdtemp,mkdir,rm,writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {existsSync} from 'node:fs';
import {join} from 'node:path';
import {repo,outDir,reporter,launch,settle} from './lib.mjs';

process.chdir(repo);
const {createSyntheticRuntime}=await import(join(repo,'qa/runtime-helper.mjs'));
const {feedbackArrival,assertFeedbackArrival}=await import(join(repo,'qa/feedback-arrival.mjs'));
const width=Number(process.argv[process.argv.indexOf('--width')+1]||390)||390;
const cliPath=join(repo,process.env.SCRY_STUDY_CLI||'target/debug/scry');
for(const [path,build] of [['build/index_bg.wasm','worker-build --release --locked'],[cliPath,'cargo build --locked --bin scry']])
 if(!existsSync(path)){console.error(`missing ${path}; run: ${build}`);process.exit(2);}

const dir=outDir('local-journeys');
const {log,done}=reporter(dir);
// Private run directory: holds the run-owned login.json with the Owner token.
const scratch=process.env.HOME?join(process.env.HOME,'.cache/tmp'):tmpdir();await mkdir(scratch,{recursive:true});
const temp=await mkdtemp(join(scratch,'verify-scry-local-'));
const config=join(temp,'login.json');
// Authored synthetic wording only, shared with qa/phone-stream.mjs.
const {previousText,phoneText}=await import(join(repo,'qa/phone-stream.mjs'));
let secret='';
const cli=(args,input='')=>new Promise((ok,fail)=>{
 const child=spawn(cliPath,[...args,'--json','--config',config],{env:{PATH:process.env.PATH,HOME:temp},stdio:['pipe','pipe','pipe']});
 let out='',err='';child.stdout.on('data',d=>out+=d);child.stderr.on('data',d=>err+=d);child.on('error',fail);
 child.on('close',code=>{if(secret&&(out+err).includes(secret))return fail(Error('CLI echoed the token'));let value=null;try{value=JSON.parse(out);}catch{}ok({code,value,err:err.trim().slice(0,160)});});
 child.stdin.end(input);
});
const transcript=[];
const run=async(args,input)=>{const r=await cli(args,input);transcript.push({cmd:['scry',...args,'--json'].join(' ').replaceAll(temp,'<temp>'),stdin:input?'<redacted>':undefined,exit:r.code,stdout:r.value});return r;};

let runtime,browser,page,origin;
// Full-page captures paint the sticky result actions at the first viewport's
// bottom, over later content. `at` captures the viewport around one element.
const shot=async(name,mask=[],at=null)=>{await settle(page);if(at)await at.evaluate(e=>e.scrollIntoView({block:'center'}));await page.screenshot({path:join(dir,`${name}-${width}.png`),fullPage:!at,mask});return `${name}-${width}.png`;};
const step=async(id,fn)=>{try{await fn();}catch(e){log(id,'FAIL',e.message.split('\n')[0],{evidence:await shot(`${id}-failure`).catch(()=>null)});}};
let alive=false;
try{
 // Inside try: a failed Worker or Chromium launch still cleans up below.
 runtime=await createSyntheticRuntime({name:'verify-scry-local'});alive=true;
 origin=runtime.url;
 browser=await launch();
 const context=await browser.newContext({viewport:{width,height:844},...(width<768?{isMobile:true,hasTouch:true}:{})});
 page=await context.newPage();page.setDefaultTimeout(15000);
 await step('notebook.empty',async()=>{
  await page.goto(origin+'/notebook');
  const h1=await page.getByRole('heading',{level:1}).innerText();
  if(h1!=='Follow your curiosity.')throw Error(`empty notebook heading "${h1}"`);
  log('notebook.empty','PASS',`fresh notebook /notebook → "${h1}" with Create and the authored-demo action`,{evidence:await shot('notebook-empty')});
 });
 await step('notebook.concept',async()=>{
  await page.getByRole('button',{name:'Explore an authored demo'}).click();
  await page.locator('[data-state="question"]').waitFor();
  await page.locator('.choice').first().click();
  await page.locator('[data-state="result"]').waitFor();
  await page.reload();await page.locator('[data-state="result"]').waitFor();
  const held=await shot('notebook-held-result');
  await page.getByRole('button',{name:'Next question'}).click();
  await page.locator('[data-state="question"]').waitFor();
  await page.getByRole('link',{name:'Map',exact:true}).click();await page.locator('[data-state="map"]').waitFor();
  await page.locator('a[href^="/goals/"]').first().click();await page.locator('[data-state="goal"]').waitFor();
  await page.locator('main a[href^="/concepts/"]').first().click();
  await page.locator('[data-state="concept"],[data-state="gate"]').first().waitFor();
  const gate=await page.locator('[data-state="gate"]').count();
  log('notebook.concept','PASS',`demo question → choice → held result survives reload → Next question → Map → goal → concept (${gate?'assistance gate before reference':'reference'})`,{evidence:[held,await shot('notebook-concept')]});
 });
 await step('token.create',async()=>{
  await page.goto(origin+'/settings#owner-tokens');
  await page.locator('#token-name').fill('verify-scry local CLI');
  await page.getByRole('button',{name:'Create Owner token'}).click();
  const box=page.getByRole('textbox',{name:'Owner token secret'});
  secret=(await box.inputValue()).trim();
  if(!secret)throw Error('no secret shown');
  const once=await shot('token-shown-once',[box]);
  await page.goto(origin+'/settings#owner-tokens');
  const listed=await page.locator('#owner-tokens').innerText();
  if(!listed.includes('verify-scry local CLI')||(await page.content()).includes(secret))throw Error('token list missing name or exposing secret');
  log('token.create','PASS','Settings → Create Owner token shows the secret once (masked in evidence); list shows name and metadata only',{evidence:[once,await shot('token-list',[],page.locator('#owner-tokens'))]});
 });
 await step('cli.login',async()=>{
  const login=await run(['login','--url',origin,'--token-stdin'],secret+'\n');
  const who=await run(['whoami']);
  if(login.code||who.code||who.value?.token?.name!=='verify-scry local CLI')throw Error(`login ${login.code} whoami ${who.code} ${who.err}`);
  log('cli.login','PASS',`scry login --token-stdin then whoami → token "${who.value.token.name}"`);
 });
 let vid;
 await step('library.find-keep',async()=>{
  const empty=await run(['search','synthetic creed']);
  if(empty.value?.empty!==true)throw Error('Library not empty before add');
  const file=join(temp,'text.txt'),sections=join(temp,'sections.json');
  await writeFile(file,previousText+phoneText);
  await writeFile(sections,JSON.stringify([{heading:'Synthetic first article',units:[previousText,phoneText]}]));
  const add=['version','add','--title','Synthetic Creed','--kind','text','--name','verify-scry synthetic wording','--alias','Symbolum Syntheticum','--source','Authored synthetic text, not canonical','--text-file',file,'--sections-file',sections];
  const first=await run(add);vid=first.value?.result?.entity_id;
  const found=await run(['search','symbolum syntheticum']);
  const again=await run(add);
  if(!vid||found.value?.matches?.[0]?.versions?.[0]?.id!==vid||again.value?.result?.entity_id!==vid)throw Error(`add ${first.code} ${first.err}; search ${JSON.stringify(found.value)?.slice(0,120)}; repeat ${again.value?.result?.entity_id}`);
  log('library.find-keep','PASS',`search → "Nothing matches."; version add → ${vid}; alias search finds it; identical add returns the same Version`);
 });
 await step('study.phone',async()=>{
  const learn=await run(['learn',vid]);
  if(learn.code)throw Error(`learn ${learn.err}`);
  await page.goto(origin+'/recall');await page.locator('[data-state="exact-prompt"]').waitFor();
  if(await page.locator('textarea').count()!==1)throw Error('phone Stream must show one answer field');
  const prompt=await shot('study-prompt');
  await page.getByLabel('Your answer',{exact:true}).fill('The path begins in a synthetic notebook.');
  await page.getByRole('button',{name:'Check my answer',exact:true}).click();
  await page.locator('[data-state="exact-result"]').waitFor();
  const arrivalProof=await feedbackArrival(page,async (page,name,options)=>{await page.screenshot({path:join(dir,name),...options});return name;},`study-result-arrival-${width}.png`,false);
  await writeFile(join(dir,'study-result-arrival.json'),JSON.stringify(arrivalProof,null,2));
  assertFeedbackArrival(arrivalProof);
  await page.reload();await page.locator('[data-state="exact-result"]').waitFor();
  const verdict=await page.locator('[data-verdict]').getAttribute('data-verdict');
  const kinds=await page.locator('[data-diff-kind]').evaluateAll(n=>n.map(x=>x.dataset.diffKind));
  const api=(await run(['next'])).value?.practice?.attempt;
  if(verdict!==api?.grade?.verdict||JSON.stringify(kinds)!==JSON.stringify(api.grade.diff.map(d=>d.kind)))throw Error(`phone ${verdict} ${kinds} ≠ CLI ${api?.grade?.verdict}`);
  const arrival=arrivalProof.evidence;
  await page.getByText('Full answer',{exact:true}).click();
  const diff=await shot('study-diff',[],page.locator('.exact-full-answer .exact-diff'));
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth);
  if(overflow)throw Error('horizontal overflow at phone width');
  await page.getByRole('button',{name:'Next',exact:true}).click();
  await page.locator('[data-state="exact-prompt"],[data-state="exact-caught-up"]').first().waitFor();
  log('study.phone','PASS',`scry learn → /recall opens the exact Prompt (one field); typed answer → ${verdict} with Diff [${[...new Set(kinds)].join(', ')}] held across reload and equal to scry next; Next advances. On arrival the first changed word and replacement are fully visible and unoccluded`,{evidence:[prompt,arrival,diff,await shot('study-next')]});
 });
 await step('notebook.legacy-link',async()=>{
  await page.goto(origin+'/notebook');
  await page.locator('[data-state="question"],[data-state="result"],[data-state="intro"]').first().waitFor();
  log('notebook.legacy-link','PASS','/notebook still opens the concept notebook while exact Studies own /',{evidence:await shot('notebook-legacy')});
 });
 await step('token.revoke',async()=>{
  await page.goto(origin+'/settings#owner-tokens');
  // Awaited together: a failed click becomes this step's FAIL, and the pending
  // wait can never reject unhandled.
  const [revoked]=await Promise.all([
   page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname==='/tokens/revoke'),
   page.locator('#owner-tokens').getByRole('button',{name:/Revoke/}).first().click(),
  ]);
  if(revoked.status()>=400)throw Error('revoke request refused');
  await page.waitForLoadState();
  const who=await run(['whoami']);
  if(who.code===0)throw Error('revoked token still admitted');
  log('token.revoke','PASS',`Settings → Revoke; next scry whoami exits ${who.code}: ${who.err||'refused'}`);
 });
 await step('state.backend-down',async()=>{
  // Answer the open notebook question after stopping the Worker. Never click a
  // link here: navigating to a dead origin shows the browser's error page.
  await page.goto(origin+'/notebook');await page.locator('[data-state="question"]').waitFor();
  const choices=await page.locator('.choice').count();
  if(!choices)await page.locator('#answer').fill('synthetic answer while the backend is down');
  await runtime.mf.dispose();alive=false;
  if(choices)await page.locator('.choice').first().click();
  else await page.getByRole('button',{name:'Check my answer',exact:true}).click();
  const message=page.getByText('The response did not arrive. Your draft is still here.',{exact:true});
  await message.waitFor();
  if(await page.locator('[data-state="result"]').count())throw Error('a result appeared with the backend down');
  await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();
  log('state.backend-down','PASS',`Worker stopped, then ${choices?'choice tap':'typed answer'} → no result; "The response did not arrive. Your draft is still here." with Reconcile this attempt`,{evidence:await shot('backend-down')});
 });
}catch(e){
 log('local.launch','FAIL',e.message.split('\n')[0]);
}finally{
 await writeFile(join(dir,'cli-transcript.json'),JSON.stringify(transcript,null,1)+'\n');
 await browser?.close();
 if(alive)await runtime.mf.dispose();
 await rm(temp,{recursive:true,force:true});
}
process.exit(done());
