#!/usr/bin/env node
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {createServer} from 'node:net';
import {existsSync,readFileSync} from 'node:fs';
import {mkdir,readFile,rm,writeFile} from 'node:fs/promises';
import {createRequire} from 'node:module';
import {resolve,join} from 'node:path';
import {specs} from './walk-specs.mjs';
import {createSyntheticRuntime} from './runtime-helper.mjs';
import {recoveryWalk} from './browser-recovery.mjs';
import {createWalk} from './browser-create.mjs';
const repo=resolve(import.meta.dirname,'..');process.chdir(repo);
if(process.env.SCRY_SKIP_BUILD!=='1')execFileSync('worker-build',['--release','--locked'],{stdio:'inherit',env:process.env});
const source=readFileSync('USER_STORIES.md','utf8');
const sections=[...source.matchAll(/^## (US-\d{3})\b[^\n]*$/gm)];
const live=sections.filter((m,i)=>!/^Superseded|^Retired|\(retired\)/m.test(source.slice(m.index,sections[i+1]?.index))).map(m=>m[1]);
const argv=process.argv.slice(2);
const selected=argv[0]==='--all'&&argv.length===1?live:argv[0]==='--stories'&&argv.length===2?[...new Set(argv[1].split(/\s+/).filter(Boolean))]:null;
if(!selected||selected.some(id=>!live.includes(id)))throw Error('usage: qa/walk --all | --stories "US-002 US-003"');
for(const id of live){const i=sections.findIndex(m=>m[1]===id);const block=source.slice(sections[i].index,sections[i+1]?.index);const count=[...block.matchAll(/^\d+\. (?:WHEN|WHILE|WHERE|IF|THE\b|EACH|Export)/gm)].length;if(count!==specs[id]?.length)throw Error(`${id}: ${specs[id]?.length} walks for ${count} active criteria`);}
const require=createRequire(import.meta.url);
let playwright;
try{playwright=require(process.env.SCRY_CRITICS_PLAYWRIGHT_PATH||'playwright');}catch{throw Error('Install lockfile-pinned Playwright or set SCRY_CRITICS_PLAYWRIGHT_PATH.');}
const output=join(repo,'target/walk'),marker=join(output,'.scry-walk-owned');
if(existsSync(output)&&!existsSync(marker))throw Error('Refusing non-owned story-walk output.');
if(existsSync(marker))await rm(output,{recursive:true});
await mkdir(join(output,'screens'),{recursive:true});await mkdir(join(output,'logs'),{recursive:true});await writeFile(marker,'Run-owned synthetic proof only.\n');
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
const metadata=process.env.SCRY_SOURCE_META?JSON.parse(readFileSync(process.env.SCRY_SOURCE_META)):null;
const git=(...args)=>execFileSync('git',args,{encoding:'utf8'}).trim();
const receipt={schema:'foundation-walk-receipt/1',check:'scry-story-walk',run:process.env.GITHUB_RUN_ID||`local-${Date.now()}`,head:metadata?.git_revision||git('rev-parse','HEAD'),tree:metadata?.git_tree||git('rev-parse','HEAD^{tree}'),base:process.env.WALK_BASE||null,source_state:metadata?.source_state||'worktree',source_sha256:metadata?.source_sha256||null,artifact_sha256:sha(readFileSync('build/index_bg.wasm')),started_at:new Date().toISOString(),finished_at:'',exit:1,stories:[],artifacts:[],limits:['Synthetic workerd/SQLite/local R2 and authored HTTP-caching material; actual browser events.','US-001.1 runs a separate pinned retained-Go migration test; it is not Rust migration acceptance. Superseded US-005 remains historical.','Policy/test mechanics do not establish live generation/critic/holdout quality, S11 useful real material, real phone, production private ingress, or remote recovery.']};
const register=async path=>{receipt.artifacts.push({path,sha256:sha(await readFile(join(output,path)))});};
const native=execFileSync('cargo',['test','--locked','--all-targets'],{encoding:'utf8',env:process.env});await writeFile(join(output,'logs/rust-tests.txt'),native);await register('logs/rust-tests.txt');
const policy=(...names)=>{for(const name of names)assert.ok(native.includes(`test ${name} ... ok`),`Native test did not pass: ${name}`);};
let retainedGo;
if(selected.includes('US-001')){
 execFileSync('python3',['qa/retained-go-migration.py',join(output,'logs')],{stdio:'inherit',env:process.env});
 retainedGo=JSON.parse(await readFile(join(output,'logs/retained-go-migration.json'),'utf8'));
 await register('logs/retained-go-migration.txt');await register('logs/retained-go-migration.json');
}
const retainedGoMigration=()=>{assert.equal(retainedGo?.status,'pass');assert.equal(retainedGo?.target,'retained-go-compatibility');};
const freePort=async()=>{const s=createServer();await new Promise((ok,bad)=>{s.once('error',bad);s.listen(0,'127.0.0.1',ok);});const p=s.address().port;await new Promise(ok=>s.close(ok));return p;};
let browser;
try{
 browser=await playwright.chromium.launch({headless:true,...(process.env.SCRY_CRITICS_CHROMIUM_PATH?{executablePath:process.env.SCRY_CRITICS_CHROMIUM_PATH}:{}),args:['--no-sandbox']});
 for(const id of selected){
  const runtime=await createSyntheticRuntime({port:await freePort(),name:`walk-${id.toLowerCase()}`});
  const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true});const page=await context.newPage();page.setDefaultTimeout(12000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));page.on('console',message=>{if(message.type()==='error')errors.push(message.text());});
  const result={id,status:'pass',criteria:[]};receipt.stories.push(result);
  try{
   await page.goto(runtime.url+'/');
   if(id!=='US-013'){await page.getByRole('button',{name:'Explore an authored demo'}).click();await page.locator('[data-state="question"]').waitFor();}
   const c={page,url:path=>runtime.url+path,policy,retainedGoMigration};
   for(let i=0;i<specs[id].length;i++){
    const legacy=id==='US-001'&&i===0;
    const criterion={n:i+1,status:'pass',target:legacy?'retained-go-compatibility':'rust-worker',source_revision:legacy?retainedGo.source_revision:receipt.head,evidence:legacy?['logs/retained-go-migration.txt','logs/retained-go-migration.json']:['logs/rust-tests.txt']};result.criteria.push(criterion);
    try{await specs[id][i](c);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Horizontal overflow on390px');}
    catch(error){criterion.status='fail';result.status='fail';criterion.reason=error.message;console.error(`${id}.${i+1}: ${error.message}`);}
    if(!legacy){const path=`screens/${id}-${i+1}${criterion.status==='fail'?'-failure':''}.png`;await page.screenshot({path:join(output,path),fullPage:true});await register(path);criterion.evidence.push(path);}
   }
   assert.deepEqual(errors,[],'Browser console/page errors');
  }catch(error){result.status='fail';result.error=error.message;console.error(`${id}: ${error.message}`);}
  finally{await context.close();await runtime.mf.dispose();}
 }
 receipt.browser_recovery=await recoveryWalk(browser,async(page,name)=>{const path=`screens/${name}`;await page.screenshot({path:join(output,path),fullPage:true});await register(path);return path;});
 receipt.create_journey=await createWalk(browser,async(page,name)=>{const path=`screens/${name}`;await page.screenshot({path:join(output,path),fullPage:true});await register(path);return path;});
 receipt.exit=receipt.stories.every(s=>s.status==='pass')&&receipt.browser_recovery.every(s=>s.status==='pass')&&receipt.create_journey.status==='pass'?0:1;
}catch(error){receipt.infrastructure_error=error.message;console.error(error);}
finally{await browser?.close();receipt.finished_at=new Date().toISOString();await writeFile(join(output,'walk-receipt.json'),JSON.stringify(receipt,null,2)+'\n');console.log(`Rust Worker story walk ${receipt.stories.filter(s=>s.status==='pass').length}/${selected.length}: ${output}/walk-receipt.json`);}
process.exitCode=receipt.exit;
