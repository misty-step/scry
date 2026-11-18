// SPEC § Keyed mappings, § Baby-care readiness and the mapping parts of
// § Grading through the built Worker and every client: the compiled CLI, the
// Owner API, the app's native forms and pages, and the compiled tool server.
// Synthetic content only; no account or egress. With Playwright, it also
// keeps 390 px light and dark screenshots of each changed screen.
import assert from 'node:assert/strict';
import test from 'node:test';
import {spawn,spawnSync} from 'node:child_process';
import {randomUUID} from 'node:crypto';
import {mkdtemp,mkdir,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {dirname,join,resolve} from 'node:path';
import {createInterface} from 'node:readline';
import {createRequire} from 'node:module';
import {once} from 'node:events';
import {isDeepStrictEqual} from 'node:util';
import {createRuntime,internalKey} from './runtime-helper.mjs';
import {cliProcess,stdinStopped} from './cli-process.mjs';

const mcpBinary=process.env.SCRY_MCP_BIN||resolve('target/debug/scry-mcp');
const cliBinary=join(dirname(mcpBinary),'scry');
if(!process.env.SCRY_MCP_BIN){
 const built=spawnSync('cargo',['build','--locked','--bin','scry','--bin','scry-mcp'],{encoding:'utf8'});
 assert.equal(built.status,0,built.stderr);
}
const MARKER='mapping_compatibility/v1';
const FORMAT='scry-rust-durable-sqlite-mapping-v6';
const YEAR='1;unicode=17.0.0;mapping=1;year=1';
const shots=process.env.SCRY_MAPPING_SHOTS||'target/mapping-spec';

test('SPEC § Keyed mappings: directions, wrong key, years and safety-bearing pairs through CLI, API, app and tool server',{timeout:300000},async t=>{
 const directory=await mkdtemp(join(tmpdir(),'scry-mapping-spec-'));
 const config=join(directory,'login.json'),name='mapping-spec-'+randomUUID();
 const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey};
 let egress=0,runtime,child,closed,browser;
 t.after(async()=>{
  try {await browser?.close();if(child){child.stdin.end();const done=await Promise.race([closed,new Promise(r=>setTimeout(()=>r(null),2000).unref())]);if(!done){child.kill('SIGKILL');await closed;}}}
  finally {try {await runtime?.mf.dispose();} finally {await rm(directory,{recursive:true,force:true});}}
  assert.equal(egress,0,'Mapping practice makes no external call.');
 });
 runtime=await createRuntime({port:0,name,inspect:true,bindings,egress:async()=>{egress++;throw Error('No egress.');}});
 const origin=(await runtime.mf.ready).origin;
 runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin})};
 const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
 const rows=()=>storage.exec('SELECT key,value FROM records ORDER BY key');
 const marked=async()=>(await rows()).some(r=>r.key===MARKER);
 const unchanged=async(action,label)=>{const before=await rows();await action();assert.ok(isDeepStrictEqual(await rows(),before),label);};
 const page=async path=>{const r=await runtime.worker.fetch(origin+path);return {status:r.status,html:await r.text()};};
 const formFields=(html,action)=>{const form=new RegExp(`<form[^>]*action="${action.replaceAll('/','\\/')}"[^>]*>([\\s\\S]*?)</form>`).exec(html);assert.ok(form,`form ${action}`);return Object.fromEntries([...form[1].matchAll(/name="(csrf|operation_id|revision|prompt_id|after|study_id)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));};
 const post=(path,body)=>runtime.worker.fetch(origin+path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(body)});

 const settings=(await page('/settings')).html;
 const issued=await post('/tokens/create',{...formFields(settings,'/tokens/create'),name:'Synthetic mapping spec agent; unpaid'});
 assert.equal(issued.status,200);
 const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())?.[1];assert.ok(secret);
 const api=(path,payload)=>runtime.worker.fetch(origin+path,{...(payload?{method:'POST',body:JSON.stringify(payload)}:{}),headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'}});
 let serial=0;const op=label=>`mapping-spec-${label}-${String(++serial).padStart(4,'0')}`;
 const apiJson=async(path,payload)=>{const r=await api(path,payload&&{operation_id:op('api'),...payload});return {status:r.status,body:await r.json()};};
 const apiOk=async(path,payload)=>{const r=await apiJson(path,payload);assert.equal(r.status,200,JSON.stringify(r.body));return r.body;};
 const cli=async(args,input='')=>{const {code,out,err}=await cliProcess(cliBinary,[...args,'--json','--config',config],{env:{PATH:process.env.PATH,HOME:directory},input});assert.ok(!out.includes(secret)&&!err.includes(secret));return {code,value:JSON.parse(out)};};
 const ok=async(args,input)=>{const r=await cli(args,input);assert.equal(r.code,0,JSON.stringify(r.value));return r.value;};
 const file=async(file,contents)=>{const path=join(directory,file);await writeFile(path,contents);return path;};
 assert.equal((await cli(['login','--url',origin,'--token-stdin'],secret+'\n')).code,0);

 child=spawn(mcpBinary,['--config',config],{env:{PATH:process.env.PATH,HOME:directory},stdio:['pipe','pipe','pipe']});closed=once(child,'close');
 let rpcSerial=0;const pending=new Map(),stopped=stdinStopped(child);
 createInterface({input:child.stdout}).on('line',line=>{assert.ok(!line.includes(secret));const value=JSON.parse(line);pending.get(value.id)(value);pending.delete(value.id);});
 const rpc=(method,params={})=>Promise.race([stopped,new Promise(done=>{const id=++rpcSerial;pending.set(id,done);child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n');})]);
 const tool=async(name,args)=>{const r=await rpc('tools/call',{name,arguments:args});assert.equal(r.error,undefined);return r.result;};
 const toolOk=async(name,args)=>{const r=await tool(name,args);assert.equal(r.isError,false,JSON.stringify(r.structuredContent));return r.structuredContent;};
 assert.equal((await rpc('initialize',{protocolVersion:'2025-11-25',capabilities:{},clientInfo:{name:'scry-mapping-spec',version:'1'}})).result.protocolVersion,'2025-11-25');
 child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized'})+'\n');

 // Screens, kept only where Playwright is present.
 let shoot=async()=>{};
 const playwrightPath=process.env.SCRY_CRITICS_PLAYWRIGHT_PATH;
 if(playwrightPath){
  const playwright=createRequire(import.meta.url)(playwrightPath);
  browser=await playwright.chromium.launch({headless:true,...(process.env.SCRY_CRITICS_CHROMIUM_PATH?{executablePath:process.env.SCRY_CRITICS_CHROMIUM_PATH}:{}),args:['--no-sandbox']});
  await mkdir(shots,{recursive:true});
  shoot=async(label,path='/recall')=>{for(const scheme of ['light','dark']){const context=await browser.newContext({viewport:{width:390,height:844},colorScheme:scheme,javaScriptEnabled:false});const p=await context.newPage();await p.goto(origin+path);const width=await p.evaluate(()=>document.documentElement.scrollWidth);assert.ok(width<=390,`${label} ${scheme} scrolls sideways: ${width}`);await p.screenshot({path:join(shots,`${label}-${scheme}.png`),fullPage:true});await context.close();}};
 }
 // The held Prompt's Card, from the complete history once nothing is protected.
 const held=async()=>(await apiOk('/api/v1/next',{})).practice;
 // Switching lessons: leave an unanswered Prompt, or continue after a result.
 const away=practice=>practice?.prompt?(practice.attempt_id?{after:practice.prompt.id}:{leave:practice.prompt.id}):{};

 const sky={title:'Synthetic sky',direction:'both',pairs:[{key:'North star',value:'Polaris'},{key:'Red planet',value:'Mars'},{key:'Ringed planet',value:'Saturn'}]};

 await t.test('a mapping declared both ways is practised forward and in reverse, by every client, and the reverse answer stays protected',async()=>{
  // Refusals by each client change nothing.
  await unchanged(async()=>{
   const dup=await apiJson('/api/v1/library/mappings',{data:{...sky,pairs:[{key:'One',value:'Same'},{key:'Two',value:'same'}]}});
   assert.equal(dup.status,422);assert.match(dup.body.error,/needs a different value for every pair/);
   const safe=await cli(['mapping','add','--file',await file('safe-both.json',JSON.stringify({title:'Synthetic both safety',direction:'both',pairs:[{key:'A synthetic hazard',value:'step back',safety_bearing:true}]}))]);
   assert.notEqual(safe.code,0);
   assert.equal((await tool('scry_import',{operation_id:op('tool'),format:'mapping',data:{...sky,direction:'reverse'}})).isError,true);
  },'Refused manifests write nothing.');
  assert.equal(await marked(),false);
  const added=await ok(['mapping','add','--file',await file('sky.json',JSON.stringify(sky)),'--operation-id','mapping-spec-sky-add-op']);
  const vid=added.result.entity_id;
  assert.equal((await ok(['mapping','add','--file',await file('sky.json',JSON.stringify(sky)),'--operation-id','mapping-spec-sky-add-op'])).replayed,true);
  const shown=await toolOk('scry_show',{kind:'version',id:vid});assert.equal(shown.version.direction,'both');
  assert.equal(await marked(),true,'A Version declared both ways names the notebook for it.');
  assert.match((await page('/library/versions/'+vid)).html,/Practised both ways: from each key to its value, and from each value to its key\./);
  const study=(await ok(['learn',vid])).practice.study.id;
  // Forward, through the CLI.
  let source=(await ok(['next','--study',study])).practice;assert.equal(source.phase,'study');
  let quiz=(await ok(['quiz',source.prompt.id])).practice;assert.equal(quiz.prompt.cue_level,'mapping_key');assert.equal(quiz.prompt.cue,'North star');
  let answered=(await ok(['answer',quiz.prompt.id,'--text-stdin'],'Polaris')).practice;assert.equal(answered.attempt.grade.verdict,'Good');
  // The next pair, forward, through the tool server.
  source=(await toolOk('scry_next',{operation_id:op('tool'),after:quiz.prompt.id,study_id:study})).practice;
  assert.equal(source.prompt.unit_ids[0],shown.version.sections[0].units[1].id);
  quiz=(await toolOk('scry_quiz',{operation_id:op('tool'),prompt_id:source.prompt.id})).practice;assert.equal(quiz.prompt.cue,'Red planet');
  answered=(await toolOk('scry_answer',{operation_id:op('tool'),prompt_id:quiz.prompt.id,answer:'Mars'})).practice;assert.equal(answered.attempt.grade.verdict,'Good');
  // Reverse, through the Owner API: its own Source study, led by the value.
  source=(await apiOk('/api/v1/next',{after:quiz.prompt.id,study_id:study})).practice;
  assert.equal(source.phase,'study');assert.equal(source.prompt.unit_ids[0],shown.version.sections[0].units[0].id);
  assert.match((await page('/recall')).html,/<h1>Polaris<\/h1>[\s\S]*<p class="reading exact-expected">North star<\/p>/);
  await shoot('reverse-source-study');
  quiz=(await apiOk('/api/v1/quiz',{prompt_id:source.prompt.id})).practice;
  assert.equal(quiz.prompt.cue_level,'mapping_value');assert.equal(quiz.prompt.cue,'Polaris');assert.equal(quiz.prompt.title,'Recall the matching key');
  // Protected recall: the key is answer-bearing on every page, route and tool.
  assert.ok(!JSON.stringify(quiz).includes('North star'));
  const recall=(await page('/recall')).html;
  assert.ok(!recall.includes('North star'));assert.match(recall,/Recall the matching key/);assert.match(recall,/<p class="eyebrow">Value<\/p><p class="reading">Polaris<\/p>/);
  for(const path of ['/history','/library','/library/versions/'+vid]) assert.ok(!(await page(path)).html.includes('North star'),path);
  for(const path of ['/api/v1/history','/api/v1/library/versions/'+vid,'/api/v1/export']) assert.equal((await api(path)).status,409,path);
  assert.ok(!JSON.stringify((await apiJson('/api/v1/due')).body).includes('North star'));
  assert.equal((await tool('scry_show',{kind:'version',id:vid})).isError,true);
  await shoot('reverse-prompt');
  // The app's native form answers it: the wrong pair's key is named.
  const form=formFields(recall,'/practice/answer');
  const result=await post('/practice/answer',{...form,answer:'Red planet'});
  assert.equal(result.status,303);
  const resultPage=(await page('/recall')).html;
  assert.match(resultPage,/data-verdict="Again"/);
  assert.match(resultPage,/<span>Polaris<\/span> <span aria-hidden="true">→<\/span> <strong>North star<\/strong>/);
  assert.match(resultPage,/That answer is the key for Mars\./);
  await shoot('reverse-wrong-key-result');
  const history=(await apiJson('/api/v1/history')).body;
  const reverse=Object.values(history.cards).filter(c=>c.direction==='reverse');
  assert.equal(reverse.length,2,'each introduced pair has its reverse Card');
  const attempt=Object.values(history.attempts).find(a=>a.prompt.cue_level==='mapping_value');
  assert.equal(attempt.raw_answer,'Red planet');assert.equal(attempt.grade.verdict,'Again');
  assert.match((await page('/history')).html,/Unit 1, in reverse/);
  // A Recitation of a mapping is refused by every client.
  const prompt=(await held()).prompt;
  await unchanged(async()=>{
   assert.equal((await apiJson('/api/v1/next',{after:prompt.id,study_id:study,recite:'whole'})).status,422);
   assert.notEqual((await cli(['next','--after',prompt.id,'--study',study,'--recite','whole'])).code,0);
   assert.equal((await tool('scry_lesson_next',{operation_id:op('tool'),after:prompt.id,mode:'whole'})).isError,true);
  },'A mapping is never recited as a whole set.');
  // The complete archive the built Worker writes is named for it.
  const ns=await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE');const stub=ns.get(ns.idFromName(name));
  const backed=await stub.fetch('https://internal.invalid/__internal/daily',{method:'POST',headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});assert.equal(backed.status,200);
  const bucket=await runtime.mf.getR2Bucket('SCRY_BACKUPS');const list=await bucket.list({prefix:`snapshots/${name}/`});
  const archive=JSON.parse(Buffer.from(await(await bucket.get(list.objects.at(-1).key)).arrayBuffer()));
  assert.equal(archive.format,FORMAT);
  // Leave the held result for the next journey.
  await apiOk('/api/v1/mix',{after:(await held()).prompt.id});
 });

 await t.test('a year is accepted with or without AD or CE as an exact number, and another pair\'s value is the wrong key',async()=>{
  const vid=(await apiOk('/api/v1/library/mappings',{data:{title:'Synthetic dates',direction:'forward',pairs:[{key:'Synthetic battle',value:'1066'},{key:'Synthetic coronation',value:'AD 800'}]}})).result.entity_id;
  const lesson=(await apiOk('/api/v1/lessons/start',{version_id:vid,...away(await held())})).practice;
  assert.equal(lesson.phase,'study');
  // Through the app's native forms: Try from memory, then answer.
  const study=formFields((await page('/recall')).html,'/practice/quiz');
  assert.equal((await post('/practice/quiz',study)).status,303);
  const quizPage=(await page('/recall')).html;assert.match(quizPage,/Synthetic battle/);
  assert.equal((await post('/practice/answer',{...formFields(quizPage,'/practice/answer'),answer:'1066 AD'})).status,303);
  assert.match((await page('/recall')).html,/data-verdict="Good"/);
  await shoot('year-result');
  const attempt=Object.values((await apiJson('/api/v1/history')).body.attempts).find(a=>a.raw_answer==='1066 AD');
  assert.equal(attempt.grade.authority.grader_version,YEAR);
  // Another year, through the CLI, is a miss.
  const prompt=(await held()).prompt;
  const card=Object.values((await apiJson('/api/v1/history')).body.cards).find(c=>c.unit_id===prompt.unit_ids[0]);
  const again=(await ok(['next','--after',prompt.id,'--card',card.id])).practice;
  const miss=(await ok(['answer',again.prompt.id,'--text-stdin'],'1067 AD')).practice;
  assert.equal(miss.attempt.grade.verdict,'Again');assert.equal(miss.attempt.grade.authority.grader_version,'1;unicode=17.0.0;mapping=1');
  // The year rule is recorded only where it turns a miss into Good: a miss keeps the plain grade.
  assert.deepEqual(miss.attempt.grade.diff.map(d=>d.kind),['extra','wrong']);
  // Another pair's value, through the tool server, is the wrong key.
  const next=(await toolOk('scry_lesson_next',{operation_id:op('tool'),after:again.prompt.id})).practice;
  const quiz=next.phase==='study'?(await toolOk('scry_quiz',{operation_id:op('tool'),prompt_id:next.prompt.id})).practice:next;
  assert.equal(quiz.prompt.cue,'Synthetic coronation');
  const wrong=(await toolOk('scry_answer',{operation_id:op('tool'),prompt_id:quiz.prompt.id,answer:'1066'})).practice;
  assert.equal(wrong.attempt.grade.verdict,'Again');assert.equal(wrong.mapping_feedback.kind,'wrong_key');assert.equal(wrong.mapping_feedback.answered_key,'Synthetic battle');
  assert.match((await page('/recall')).html,/That answer belongs to key Synthetic battle\./);
  await shoot('wrong-key-result');
 });

 await t.test('a safety-bearing pair keeps its question and safety cue in study and Quiz, and a negation is a miss',async()=>{
  const draft={title:'Synthetic workshop safety',direction:'forward',pairs:[{key:'Synthetic lathe running, loose sleeve. What first?',value:'stop the lathe',safety_bearing:true},{key:'Synthetic bench colour',value:'grey'}]};
  assert.equal((await toolOk('scry_validate',{format:'mapping',data:draft})).valid,true);
  const vid=(await toolOk('scry_import',{operation_id:op('tool'),format:'mapping',data:draft})).result.entity_id;
  assert.deepEqual((await ok(['version','show',vid])).version.safety_bearing,[0]);
  const lesson=(await toolOk('scry_lesson_start',{operation_id:op('tool'),version_id:vid,...away(await held())})).practice;
  assert.equal(lesson.phase,'study');assert.equal(lesson.prompt.safety_bearing,true);
  const studyPage=(await page('/recall')).html;
  assert.match(studyPage,/Synthetic lathe running, loose sleeve\. What first\?/);
  assert.match(studyPage,/Safety practice: answer with the safe practice itself\./);
  await shoot('safety-source-study');
  const quiz=(await apiOk('/api/v1/quiz',{prompt_id:lesson.prompt.id})).practice;
  assert.equal(quiz.prompt.safety_bearing,true);
  const quizPage=(await page('/recall')).html;
  assert.match(quizPage,/Safety practice: answer with the safe practice itself\./);assert.ok(!quizPage.includes('stop the lathe'));
  assert.equal((await post('/practice/answer',{...formFields(quizPage,'/practice/answer'),answer:'do not stop the lathe'})).status,303);
  const missed=(await page('/recall')).html;
  assert.match(missed,/data-verdict="Again"/);
  assert.match(missed,/A safety answer states the safe practice itself: no other practice, negation or added words\./);
  await shoot('safety-result');
  assert.match((await page('/library/versions/'+vid)).html,/stop the lathe <span class="fineprint">· Safety practice<\/span>/);
  await shoot('safety-version','/library/versions/'+vid);
 });
});
