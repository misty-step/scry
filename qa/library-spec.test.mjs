// SPEC § Library through the built Worker and every client: the compiled CLI,
// the Owner API, the app's native forms and pages, and the compiled tool
// server. Synthetic content only; no browser, account or egress.
import assert from 'node:assert/strict';
import test from 'node:test';
import {spawn,spawnSync} from 'node:child_process';
import {randomUUID} from 'node:crypto';
import {mkdtemp,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {dirname,join,resolve} from 'node:path';
import {createInterface} from 'node:readline';
import {once} from 'node:events';
import {isDeepStrictEqual} from 'node:util';
import {createRuntime,internalKey} from './runtime-helper.mjs';
import {cliProcess} from './cli-process.mjs';

const mcpBinary=process.env.SCRY_MCP_BIN||resolve('target/debug/scry-mcp');
const cliBinary=join(dirname(mcpBinary),'scry');
if(!process.env.SCRY_MCP_BIN){
 const built=spawnSync('cargo',['build','--locked','--bin','scry','--bin','scry-mcp'],{encoding:'utf8'});
 assert.equal(built.status,0,built.stderr);
}
const MARKER='library_compatibility/v1';

test('SPEC § Library: duplicates, corrections, Provenance and Relations through CLI, API, app and tool server',{timeout:180000},async t=>{
 const directory=await mkdtemp(join(tmpdir(),'scry-library-spec-'));
 const config=join(directory,'login.json'),name='library-spec-'+randomUUID();
 const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey};
 let egress=0,runtime,child,closed;
 t.after(async()=>{
  try {if(child){child.stdin.end();const done=await Promise.race([closed,new Promise(r=>setTimeout(()=>r(null),2000).unref())]);if(!done){child.kill('SIGKILL');await closed;}}}
  finally {try {await runtime?.mf.dispose();} finally {await rm(directory,{recursive:true,force:true});}}
  assert.equal(egress,0,'Library work makes no external call.');
 });
 runtime=await createRuntime({port:0,name,inspect:true,bindings,egress:async()=>{egress++;throw Error('No egress.');}});
 const origin=(await runtime.mf.ready).origin;
 runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin})};
 const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
 const rows=()=>storage.exec('SELECT key,value FROM records ORDER BY key');
 const unchanged=async(action,label)=>{const before=await rows();await action();assert.ok(isDeepStrictEqual(await rows(),before),label);};
 const page=async path=>{const r=await runtime.worker.fetch(origin+path);return {status:r.status,html:await r.text()};};
 const formFields=(html,action)=>{const form=new RegExp(`<form[^>]*action="${action.replaceAll('/','\\/')}"[^>]*>([\\s\\S]*?)</form>`).exec(html);assert.ok(form,`form ${action}`);return Object.fromEntries([...form[1].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));};
 const post=(path,body)=>runtime.worker.fetch(origin+path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(body)});

 const settings=(await page('/settings')).html;
 const issued=await post('/tokens/create',{...formFields(settings,'/tokens/create'),name:'Synthetic Library spec agent; unpaid'});
 assert.equal(issued.status,200);
 const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())?.[1];assert.ok(secret);
 const api=(path,payload)=>runtime.worker.fetch(origin+path,{...(payload?{method:'POST',body:JSON.stringify(payload)}:{}),headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'}});
 const apiJson=async(path,payload)=>{const r=await api(path,payload);return {status:r.status,body:await r.json()};};
 const cli=async(args,input='')=>{const {code,out,err}=await cliProcess(cliBinary,[...args,'--json','--config',config],{env:{PATH:process.env.PATH,HOME:directory},input});assert.ok(!out.includes(secret)&&!err.includes(secret));return {code,value:JSON.parse(out)};};
 const ok=async args=>{const r=await cli(args);assert.equal(r.code,0,JSON.stringify(r.value));return r.value;};
 const file=async(file,contents)=>{const path=join(directory,file);await writeFile(path,contents);return path;};
 assert.equal((await cli(['login','--url',origin,'--token-stdin'],secret+'\n')).code,0);
 const library=async()=>(await apiJson('/api/v1/library/export')).body;

 child=spawn(mcpBinary,['--config',config],{env:{PATH:process.env.PATH,HOME:directory},stdio:['pipe','pipe','pipe']});closed=once(child,'close');
 let serial=0;const pending=new Map();
 createInterface({input:child.stdout}).on('line',line=>{assert.ok(!line.includes(secret));const value=JSON.parse(line);pending.get(value.id)(value);pending.delete(value.id);});
 const rpc=(method,params={})=>new Promise(done=>{const id=++serial;pending.set(id,done);child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n');});
 const tool=async(name,args)=>{const r=await rpc('tools/call',{name,arguments:args});assert.equal(r.error,undefined);return r.result;};
 const toolOk=async(name,args)=>{const r=await tool(name,args);assert.equal(r.isError,false,JSON.stringify(r.structuredContent));return r.structuredContent;};
 assert.equal((await rpc('initialize',{protocolVersion:'2025-11-25',capabilities:{},clientInfo:{name:'scry-library-spec',version:'1'}})).result.protocolVersion,'2025-11-25');
 child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized'})+'\n');

 await t.test('a Version whose text is already in the Library is returned, under any Work name, by every client',async()=>{
  const first=(await ok(['version','add','--title','Spec first Work','--kind','text','--text-file',await file('first.txt','Shared spec words, in order.\n'),'--operation-id','spec-qa-first-add'])).result.entity_id;
  await ok(['version','add','--title','Spec second Work','--kind','text','--text-file',await file('second.txt','Other spec words.\n')]);
  const before=await library();
  const variant=await file('variant.txt','SHARED spec words in order!!');
  const viaCli=await ok(['version','add','--title','Spec second Work','--kind','text','--text-file',variant,'--operation-id','spec-qa-dup-cli-op']);
  assert.equal(viaCli.result.entity_id,first);assert.equal(viaCli.existing,true);
  const human=await cliProcess(cliBinary,['version','add','--title','Spec second Work','--kind','text','--text-file',variant,'--config',config],{env:{PATH:process.env.PATH,HOME:directory}});
  assert.equal(human.code,0);assert.match(human.err,/^That text was already in the Library\. Nothing was added\.$/m);
  const replay=await ok(['version','add','--title','Spec second Work','--kind','text','--text-file',variant,'--operation-id','spec-qa-dup-cli-op']);
  assert.equal(replay.replayed,true);assert.equal(replay.result.entity_id,first);
  await unchanged(async()=>{const reuse=await cli(['version','add','--title','Spec second Work','--kind','text','--text-file',await file('other.txt','Different.\n'),'--operation-id','spec-qa-dup-cli-op']);assert.notEqual(reuse.code,0);},'Reusing an operation with another payload changes nothing.');
  const viaApi=await apiJson('/api/v1/library/versions',{operation_id:'spec-qa-dup-api-operation',data:{title:'Spec second Work',kind:'text',text:'shared   spec words, in order.',aliases:['Spec first Work']}});
  assert.equal(viaApi.status,200);assert.equal(viaApi.body.result.entity_id,first);assert.equal(viaApi.body.existing,true);
  const add=(await page('/library/add')).html;
  const viaApp=await post('/library/version/add',{...formFields(add,'/library/version/add'),title:'Spec second Work',text:'Shared spec words; in order.\n'});
  assert.equal(viaApp.status,303);assert.equal(viaApp.headers.get('Location'),'/library/versions/'+first+'?existing=1');
  assert.match((await page(viaApp.headers.get('Location'))).html,/<p class="notice" role="status">That text was already in the Library\. Nothing was added\.<\/p>/);
  const viaTool=await toolOk('scry_import',{operation_id:'spec-qa-dup-tool-operation',format:'version',data:{title:'Spec second Work',kind:'text',text:'Shared Spec Words In Order'}});
  assert.equal(viaTool.result.entity_id,first);assert.equal(viaTool.existing,true);
  assert.deepEqual(await library(),before,'No client created or attached anything.');
  // The same text with other Units, aliases or Spelling-strict setting is refused, and says why.
  await unchanged(async()=>{const r=await apiJson('/api/v1/library/versions',{operation_id:'spec-qa-units-operation',data:{title:'Spec first Work',kind:'text',text:'Shared spec words, in order.\n',sections:[{heading:'One',units:['Shared spec words, in order.\n']}]}});assert.equal(r.status,409);assert.match(r.body.error,/different Units\. Nothing was saved; to change that Version, correct it\./);},'Different Units write nothing.');
  await unchanged(async()=>{const r=await cli(['version','add','--title','Spec first Work','--kind','text','--text-file',variant,'--spelling-strict']);assert.notEqual(r.code,0);},'A different Spelling-strict setting writes nothing.');
  const listed=await apiJson('/api/v1/library/versions',{operation_id:'spec-qa-list-operation',data:{title:'Spec list',kind:'ordered_list',text:'Red gate\nBlue bridge\n'}});assert.equal(listed.status,200);assert.equal(listed.body.existing,false);
  await unchanged(async()=>{const r=await toolOk('scry_validate',{format:'version',data:{title:'Spec other list',kind:'ordered_list',text:'Red gate\nBlue bridge\n',ordered_aliases:{'0':['Scarlet gate']}}});assert.equal(r.valid,true);const t2=await tool('scry_import',{operation_id:'spec-qa-list-alias-tool',format:'version',data:{title:'Spec other list',kind:'ordered_list',text:'Red gate\nBlue bridge\n',ordered_aliases:{'0':['Scarlet gate']}}});assert.equal(t2.isError,true);assert.equal(t2.structuredContent.error.status,409);},'Different aliases write nothing.');
 });

 await t.test('a correction always creates a new Version that supersedes the old one, which stays unchanged',async()=>{
  const old=(await ok(['version','add','--title','Spec corrected Work','--kind','text','--text-file',await file('c1.txt','Spec corrected words, kept.\n'),'--name','Spec edition'])).result.entity_id;
  const original=(await ok(['version','show',old])).version;
  const revision=(await ok(['version','show',old])).work.revision;
  assert.ok(!(await rows()).some(r=>r.key===MARKER));
  const args=['version','supersede',old,'--revision',String(revision),'--title','Spec corrected Work','--kind','text','--text-file',await file('c2.txt','spec CORRECTED words; kept!\n'),'--name','Spec edition, corrected','--operation-id','spec-qa-case-correction'];
  const created=(await ok(args)).result.entity_id;assert.notEqual(created,old);
  const shownOld=await ok(['version','show',old]);
  assert.deepEqual(shownOld.version,original,'The old Version keeps every byte.');
  assert.ok(shownOld.relations.some(r=>r.kind==='supersedes'&&r.version_id===created&&r.previous_id===old));
  const replay=await ok(args);assert.equal(replay.replayed,true);assert.equal(replay.result.entity_id,created);
  await unchanged(async()=>{const again=await cli([...args.slice(0,-1),'spec-qa-second-correction']);assert.notEqual(again.code,0);},'Correcting an already corrected Version changes nothing.');
  const next=(await ok(['version','show',created])).work.revision;
  await unchanged(async()=>{
   const same=await apiJson('/api/v1/library/versions/supersede',{operation_id:'spec-qa-identical-correction',version_id:created,revision:next,data:{title:'Spec corrected Work',kind:'text',text:'spec CORRECTED words; kept!\n',name:'Spec edition, corrected'}});
   assert.equal(same.status,422);
  },'An identical correction changes nothing.');
  assert.equal((await rows()).find(r=>r.key===MARKER)?.value,'1','The notebook is named for the same-text correction.');
  const oldPage=(await page('/library/versions/'+old)).html;
  assert.ok(oldPage.includes(`Corrected by <a class="text-link" href="/library/versions/${created}">Spec edition, corrected</a>`));
  assert.ok((await page('/library/versions/'+created)).html.includes(`Corrects <a class="text-link" href="/library/versions/${old}">Spec edition</a>`));
  // The complete archive the built Worker writes carries the new format name.
  const ns=await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE');const stub=ns.get(ns.idFromName(name));
  const backed=await stub.fetch('https://internal.invalid/__internal/daily',{method:'POST',headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});assert.equal(backed.status,200);
  const bucket=await runtime.mf.getR2Bucket('SCRY_BACKUPS');const list=await bucket.list({prefix:`snapshots/${name}/`});
  const archive=JSON.parse(Buffer.from(await(await bucket.get(list.objects.at(-1).key)).arrayBuffer()));
  assert.equal(archive.format,'scry-rust-durable-sqlite-library-v5');assert.deepEqual(archive.app.library,await library());
 });

 await t.test('Provenance is recorded and shown, through CLI, API and tool server',async()=>{
  const recalled=(await ok(['version','add','--title','Spec recalled Work','--kind','text','--text-file',await file('p1.txt','A model recalled these spec words.\n'),'--provenance','model_from_memory'])).result.entity_id;
  assert.equal((await ok(['version','show',recalled])).version.provenance,'model_from_memory');
  await unchanged(async()=>{assert.notEqual((await cli(['version','add','--title','Spec bad','--kind','text','--text-file',await file('p0.txt','x.\n'),'--provenance','invented'])).code,0);},'An unknown Provenance is refused before any request.');
  await unchanged(async()=>{const r=await apiJson('/api/v1/library/versions',{operation_id:'spec-qa-uncited-operation',data:{title:'Spec checked Work',kind:'text',text:'Checked spec words.\n',provenance:'checked_against_sources'}});assert.equal(r.status,422);},'Checked against cited sources without a source stores nothing.');
  const checked=await apiJson('/api/v1/library/versions',{operation_id:'spec-qa-cited-operation',data:{title:'Spec checked Work',kind:'text',text:'Checked spec words.\n',provenance:'checked_against_sources',sources:['Synthetic citation']}});
  assert.equal(checked.status,200);
  const viaTool=await toolOk('scry_import',{operation_id:'spec-qa-tool-provenance',format:'mapping',data:{title:'Spec recalled pairs',direction:'forward',provenance:'model_from_memory',pairs:[{key:'North',value:'Up'},{key:'South',value:'Down'}]}});
  assert.equal((await toolOk('scry_show',{kind:'version',id:viaTool.result.entity_id})).version.provenance,'model_from_memory');
  assert.ok((await page('/library/versions/'+recalled)).html.includes('Written by a model from memory · Private only'));
  assert.ok((await page('/library/versions/'+checked.body.result.entity_id)).html.includes('Checked against cited sources · Private only'));
 });

 await t.test('manifest Relations appear on both Works they link, through CLI, API, app and tool server',async()=>{
  const rosary=(await apiJson('/api/v1/library/versions',{operation_id:'spec-qa-rosary-operation',data:{title:'Spec Rosary',kind:'text',text:'The whole spec devotion.\n'}})).body.result.entity_id;
  const rosaryWork=(await ok(['version','show',rosary])).work.id;
  await unchanged(async()=>{assert.notEqual((await cli(['version','add','--title','Spec Glorious','--kind','text','--text-file',await file('g0.txt','Glorious spec words.\n'),'--part-of','No such spec Work'])).code,0);},'A Relation to a missing Work refuses the whole manifest.');
  const glorious=(await ok(['version','add','--title','Spec Glorious','--kind','text','--text-file',await file('g1.txt','Glorious spec words.\n'),'--part-of','spec rosary','--operation-id','spec-qa-glorious-add'])).result.entity_id;
  const gloriousWork=(await ok(['version','show',glorious])).work.id;
  const draft={title:'Spec Sorrowful',kind:'text',text:'Sorrowful spec words.\n',relations:[{kind:'part_of',work:'Spec Rosary'},{kind:'related_to',work:'Spec Glorious'}]};
  const validated=await toolOk('scry_validate',{format:'version',data:draft});
  assert.equal(validated.valid,true);assert.equal(validated.relations,2);
  const bad=await tool('scry_validate',{format:'version',data:{...draft,relations:[{kind:'part_of',work:'Spec Rosary'},{kind:'part_of',work:'SPEC ROSARY'}]}});
  assert.equal(bad.isError,true);
  const sorrowful=(await toolOk('scry_import',{operation_id:'spec-qa-sorrowful-tool',format:'version',data:draft})).result.entity_id;
  const sorrowfulWork=(await toolOk('scry_show',{kind:'version',id:sorrowful})).work.id;
  const rosaryShown=await ok(['work','show',rosaryWork]);
  for(const child of [gloriousWork,sorrowfulWork]) assert.ok(rosaryShown.relations.some(r=>r.kind==='part_of'&&r.work_id===child&&r.parent_work_id===rosaryWork));
  assert.ok((await toolOk('scry_show',{kind:'work',id:gloriousWork})).relations.some(r=>r.kind==='related_to'));
  const rosaryPage=(await page('/library/works/'+rosaryWork)).html;
  assert.ok(rosaryPage.includes(`Includes <a class="text-link" href="/library/works/${gloriousWork}">Spec Glorious</a>`));
  assert.ok(rosaryPage.includes(`Includes <a class="text-link" href="/library/works/${sorrowfulWork}">Spec Sorrowful</a>`));
  const gloriousPage=(await page('/library/works/'+gloriousWork)).html;
  assert.ok(gloriousPage.includes(`Part of <a class="text-link" href="/library/works/${rosaryWork}">Spec Rosary</a>`));
  assert.ok(gloriousPage.includes(`Related to <a class="text-link" href="/library/works/${sorrowfulWork}">Spec Sorrowful</a>`));
  assert.ok((await page('/library/versions/'+glorious)).html.includes(`Part of <a class="text-link" href="/library/works/${rosaryWork}">Spec Rosary</a>`));
  // Showing a Version shows its Work's Relations too, through the CLI and the tool server.
  const gloriousVersion=await ok(['version','show',glorious]);
  assert.ok(gloriousVersion.relations.some(r=>r.kind==='part_of'&&r.work_id===gloriousWork&&r.parent_work_id===rosaryWork));
  assert.ok(gloriousVersion.relations.some(r=>r.kind==='related_to'&&r.related_work_id===gloriousWork));
  assert.ok(gloriousVersion.relations.some(r=>r.kind==='version_of'));
  const sorrowfulVersion=await toolOk('scry_show',{kind:'version',id:sorrowful});
  assert.ok(sorrowfulVersion.relations.some(r=>r.kind==='part_of'&&r.work_id===sorrowfulWork));
  assert.ok(sorrowfulVersion.relations.some(r=>r.kind==='related_to'&&r.work_id===sorrowfulWork));
  // Replaying the CLI add returns the same Version and creates no Relation.
  const relations=Object.keys((await library()).relations).length;
  assert.equal((await ok(['version','add','--title','Spec Glorious','--kind','text','--text-file',await file('g1.txt','Glorious spec words.\n'),'--part-of','spec rosary','--operation-id','spec-qa-glorious-add'])).replayed,true);
  assert.equal(Object.keys((await library()).relations).length,relations);
  const exported=await library();assert.deepEqual(Object.keys(exported).sort(),['relations','versions','works']);
 });

 await t.test('while a Prompt awaits an unaided answer, no Library page, Relation or Provenance is reachable',async()=>{
  const vid=(await apiJson('/api/v1/library/versions',{operation_id:'spec-qa-protected-operation',data:{title:'Spec protected Work',kind:'text',text:'Protected spec answer words.\n',provenance:'model_from_memory',relations:[{kind:'part_of',work:'Spec Rosary'}]}})).body.result.entity_id;
  const work=(await ok(['version','show',vid])).work.id;
  const study=(await ok(['learn',vid])).practice.study;
  const source=(await ok(['next','--study',study.id])).practice;
  await ok(['quiz',source.prompt.id]);
  for(const path of ['/library','/library/works/'+work,'/library/versions/'+vid]) {
   const {html}=await page(path);assert.match(html,/data-state="library-gate"/,path);
   for(const hidden of ['answer words','Spec protected Work','Spec Rosary','Includes','Written by a model']) assert.ok(!html.includes(hidden),`${path} shows ${hidden}`);
  }
  for(const path of ['/api/v1/library/works/'+work,'/api/v1/library/versions/'+vid,'/api/v1/library/search?q=spec','/api/v1/library/export']) assert.equal((await api(path)).status,409,path);
  const shown=await tool('scry_show',{kind:'work',id:work});assert.equal(shown.isError,true);assert.equal(shown.structuredContent.error.code,'conflict');
 });
});

// A starter, lesson or Study entry that already has a Study keeps its Version
// after a same-text correction: one Study, V1's Prompt, V1's schedule.
for(const entry of ['nato','lesson','mix']) test(`SPEC § Open into study: the ${entry} entry keeps the studied Version after a same-text correction`,{timeout:120000},async t=>{
 const directory=await mkdtemp(join(tmpdir(),'scry-library-entry-'));
 const config=join(directory,'login.json'),name=`library-entry-${entry}-`+randomUUID();
 let now=Date.parse('2026-11-16T12:00:00Z');
 const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey,SCRY_TEST_NOW_MS:String(now)};
 let runtime=await createRuntime({port:0,name,inspect:true,testClock:true,bindings});
 t.after(async()=>{await runtime.mf.dispose();await rm(directory,{recursive:true,force:true});});
 const origin=(await runtime.mf.ready).origin;
 runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin})};
 let storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
 const rows=()=>storage.exec('SELECT key,value FROM records ORDER BY key');
 const meta=async()=>JSON.parse((await rows()).find(r=>r.key==='meta').value);
 const prefixed=async prefix=>Object.fromEntries((await rows()).filter(r=>r.key.startsWith(prefix)).map(r=>[r.key,r.value]));
 const page=async path=>(await(await runtime.worker.fetch(origin+path)).text());
 const hidden=(html,action)=>{const form=new RegExp(`<form[^>]*action="${action.replaceAll('/','\\/')}"[^>]*>([\\s\\S]*?)</form>`).exec(html);assert.ok(form,`form ${action}`);return Object.fromEntries([...form[1].matchAll(/<input type="hidden" name="([^"]+)" value="([^"]*)"/g)].map(m=>[m[1],m[2]]));};
 const post=(path,body)=>runtime.worker.fetch(origin+path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(body)});
 const issued=await post('/tokens/create',{...hidden(await page('/settings'),'/tokens/create'),name:'Synthetic entry agent; unpaid'});
 const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())?.[1];assert.ok(secret);
 const api=async(path,payload)=>{const r=await runtime.worker.fetch(origin+path,{...(payload?{method:'POST',body:JSON.stringify({operation_id:randomUUID(),...payload})}:{}),headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'}});return {status:r.status,body:await r.json()};};
 const cli=async args=>{const {code,out}=await cliProcess(cliBinary,[...args,'--json','--config',config],{env:{PATH:process.env.PATH,HOME:directory}});return {code,value:JSON.parse(out)};};
 assert.equal((await cliProcess(cliBinary,['login','--url',origin,'--token-stdin','--json','--config',config],{env:{PATH:process.env.PATH,HOME:directory},input:secret+'\n'})).code,0);
 // The NATO screen offers Start or Continue only with nothing held: practise to rest.
 const rest=async()=>{
  for(let i=0;i<200;i++){
   const c=(await meta()).practice?.current;if(!c)return;
   if(c.attempt_id) assert.equal((await api('/api/v1/next',{after:c.prompt.id})).status,200);
   else if(c.phase==='study') assert.equal((await api('/api/v1/quiz',{prompt_id:c.prompt.id})).status,200);
   else assert.equal((await api('/api/v1/answer',{prompt_id:c.prompt.id,answer:'synthetic miss'})).status,200);
  }
  assert.fail('Practice reached rest.');
 };
 const enter=async()=>{
  const current=(await meta()).practice?.current;
  if(entry==='nato') return post('/practice/nato/start',hidden(await page('/?practice=nato'),'/practice/nato/start'));
  if(entry==='lesson') {const r=await api('/api/v1/lessons/start',{starter:'st-michael-en',...(current?.attempt_id?{after:current.prompt.id}:{})});assert.equal(r.status,200,JSON.stringify(r.body));return r;}
  const home=await page('/');const action=current?.attempt_id?'/practice/review/next':'/practice/review/start';
  return post(action,hidden(home,action));
 };
 const first=await enter();assert.ok([200,303].includes(first.status));
 let held=(await meta()).practice.current;const v1=held.prompt.version_id;
 if(held.phase==='study'){assert.equal((await api('/api/v1/quiz',{prompt_id:held.prompt.id})).status,200);held=(await meta()).practice.current;}
 assert.equal((await api('/api/v1/answer',{prompt_id:held.prompt.id,answer:'synthetic miss'})).status,200);
 const cards=await prefixed('practice_cards/'),studies=await prefixed('practice_studies/');
 assert.equal(Object.keys(studies).length,1);
 // The Owner's agent corrects V1 with its own text and Units, adding a source.
 const shown=(await api('/api/v1/library/versions/'+v1)).body;const v=shown.version;
 const file=async(f,c)=>{const p=join(directory,f);await writeFile(p,c);return p;};
 const corrected=await cli(['version','supersede',v1,'--revision',String(shown.work.revision),'--title',shown.work.title,'--kind',v.kind,'--text-file',await file('v1.txt',v.text),'--sections-file',await file('v1.json',JSON.stringify(v.sections.map(s=>({heading:s.heading,units:s.units.map(u=>u.text)})))),'--name',v.name,...v.sources.flatMap(x=>['--source',x]),'--source','Synthetic added citation','--rights',v.rights,...(v.spelling_strict?['--spelling-strict']:[])]);
 assert.equal(corrected.code,0,JSON.stringify(corrected.value));const v2=corrected.value.result.entity_id;assert.notEqual(v2,v1);
 now+=2*86400000;runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin,SCRY_TEST_NOW_MS:String(now)})};
 storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
 if(entry==='nato'){
  // Practise V1 to rest before the correction's clock jump would matter.
  now-=2*86400000;runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin,SCRY_TEST_NOW_MS:String(now)})};
  storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
  await rest();
  Object.assign(cards,await prefixed('practice_cards/'));
  now+=2*86400000;runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin,SCRY_TEST_NOW_MS:String(now)})};
  storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
  assert.match(await page('/?practice=nato'),/action="\/practice\/nato\/start"/);
 }
 const again=await enter();assert.ok([200,303].includes(again.status),String(again.status));
 const after=(await meta()).practice.current;
 assert.equal(after.prompt.version_id,v1,'The Prompt is on the Version the Owner chose.');
 const nowStudies=Object.values(await prefixed('practice_studies/')).map(JSON.parse);
 assert.equal(nowStudies.length,1,'No second Study.');assert.equal(nowStudies[0].version_id,v1);
 const nowCards=await prefixed('practice_cards/');
 for(const [key,value] of Object.entries(cards)) assert.equal(nowCards[key],value,'V1\'s schedule is unchanged.');
});

// The Starter shelf is Scry's own content and always startable: a copy of a
// starter's text never blocks it, and a studied copy is kept.
test('SPEC § Lessons and § Library: a copy of a starter\'s text never blocks the starter, and a studied copy is kept',{timeout:180000},async t=>{
 const directory=await mkdtemp(join(tmpdir(),'scry-library-shelf-'));
 const config=join(directory,'login.json'),name='library-shelf-'+randomUUID();
 const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey};
 let runtime=await createRuntime({port:0,name,inspect:true,bindings});
 t.after(async()=>{await runtime.mf.dispose();await rm(directory,{recursive:true,force:true});});
 const origin=(await runtime.mf.ready).origin;
 runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin})};
 const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
 const rows=async()=>Object.fromEntries((await storage.exec('SELECT key,value FROM records ORDER BY key')).map(r=>[r.key,r.value]));
 const meta=async()=>JSON.parse((await rows()).meta);
 const page=async path=>(await(await runtime.worker.fetch(origin+path)).text());
 // The native form whose hidden fields include every given name and value.
 const form=(html,action,want={})=>{
  const found=[...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].filter(m=>m[1]===action).map(m=>Object.fromEntries([...m[2].matchAll(/<input type="hidden" name="([^"]+)" value="([^"]*)"/g)].map(x=>[x[1],x[2]]))).find(f=>Object.entries(want).every(([k,v])=>f[k]===v));
  assert.ok(found,`form ${action} ${JSON.stringify(want)}`);return found;
 };
 const post=(path,body)=>runtime.worker.fetch(origin+path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(body)});
 const issued=await post('/tokens/create',{...form(await page('/settings'),'/tokens/create'),name:'Synthetic shelf agent; unpaid'});
 const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())?.[1];assert.ok(secret);
 const api=async(path,payload)=>{const r=await runtime.worker.fetch(origin+path,{...(payload?{method:'POST',body:JSON.stringify({operation_id:randomUUID(),...payload})}:{}),headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'}});return {status:r.status,body:await r.json()};};
 const cli=async args=>{const {code,out}=await cliProcess(cliBinary,[...args,'--json','--config',config],{env:{PATH:process.env.PATH,HOME:directory}});return {code,value:JSON.parse(out)};};
 assert.equal((await cliProcess(cliBinary,['login','--url',origin,'--token-stdin','--json','--config',config],{env:{PATH:process.env.PATH,HOME:directory},input:secret+'\n'})).code,0);
 const lessons=JSON.parse(await (await import('node:fs/promises')).readFile('assets/lessons.json','utf8'));
 const starter=id=>lessons.find(l=>l.id===id).input;
 const appAdd=async(title,text)=>{const r=await post('/library/version/add',{...form(await page('/library/add'),'/library/version/add'),title,text});assert.equal(r.status,303);return r.headers.get('Location').split('/').at(-1).split('?')[0];};
 const copyRows=async id=>{const all=await rows();const v=JSON.parse(all['library_versions/'+id]);return [all['library_versions/'+id],all['library_works/'+v.work_id]];};
 const shelf=async id=>{const current=(await meta()).practice?.current;const want={starter:id};if(current?.attempt_id)want.after=current.prompt.id;return post('/practice/lesson/start',form(await page('/lessons'),'/practice/lesson/start',want));};
 const answer=async()=>{let c=(await meta()).practice.current;if(c.phase==='study'){assert.equal((await api('/api/v1/quiz',{prompt_id:c.prompt.id})).status,200);c=(await meta()).practice.current;}assert.equal((await api('/api/v1/answer',{prompt_id:c.prompt.id,answer:'synthetic miss'})).status,200);};
 const michael=starter('st-michael-en');

 // (a) NATO start for an alphabet pasted into the add form, with nothing held.
 const alphabet=JSON.parse(await (await import('node:fs/promises')).readFile('examples/nato-alphabet.json','utf8')).pairs.map(p=>`${p.key}\t${p.value}\n`).join('');
 const pasted=await appAdd('My alphabet',alphabet);
 const natoBefore=await copyRows(pasted);
 const natoPage=await page('/?practice=nato');
 const nato=await post('/practice/nato/start',form(natoPage,'/practice/nato/start'));assert.ok([200,303].includes(nato.status),String(nato.status));
 const natoVersion=(await meta()).practice.current.prompt.version_id;assert.notEqual(natoVersion,pasted);
 assert.equal((await api('/api/v1/library/versions/'+natoVersion)).body.version.kind,'keyed_mapping');
 assert.deepEqual(await copyRows(pasted),natoBefore);

 await answer();
 // (a) The Owner pasted the English Saint Michael prayer under his own title.
 const copy=await appAdd('My Saint Michael',michael.text);
 const before=await copyRows(copy);
 const started=await shelf('st-michael-en');assert.equal(started.status,303,await started.text());
 const own=(await meta()).practice.current.prompt.version_id;assert.notEqual(own,copy);
 const shown=(await api('/api/v1/library/versions/'+own)).body.version;assert.equal(shown.text,michael.text);assert.equal(shown.name,michael.name);
 assert.deepEqual(await copyRows(copy),before,'The copy is unchanged.');
 // (d) Repeating the entry returns the same Version and Study.
 await answer();const studies=Object.keys(await rows()).filter(k=>k.startsWith('practice_studies/'));
 // The shelf shows a current lesson without a form; an agent repeats it.
 const repeated=await api('/api/v1/lessons/start',{starter:'st-michael-en',after:(await meta()).practice.current.prompt.id});assert.equal(repeated.status,200,JSON.stringify(repeated.body));assert.equal((await meta()).practice.current.prompt.version_id,own);
 assert.deepEqual(Object.keys(await rows()).filter(k=>k.startsWith('practice_studies/')),studies);
 // The starter's own Version shares the copy's normalized text: the notebook is named for it.
 assert.equal((await rows())['library_compatibility/v1'],'1');
 // (e) What an agent adds is still refused for the same text with another structure.
 const before2=await rows();
 const refused=await api('/api/v1/library/versions',{data:{title:'Agent Saint Michael',kind:'text',text:michael.text,sections:[{heading:'All',units:[michael.text]}]}});
 assert.equal(refused.status,409);assert.match(refused.body.error,/Nothing was saved/);
 const after2=await rows();delete before2.meta;delete after2.meta;assert.deepEqual(after2,before2);

 // (b) A studied copy is kept, active, paused or dropped, and resumed by the shelf.
 const joy=starter('rosary-joyful');
 const listCopy=(await api('/api/v1/library/versions',{data:{title:'My Joyful set',kind:'ordered_list',text:joy.text,sections:joy.sections}})).body.result.entity_id;
 const study=(await cli(['learn',listCopy])).value.practice.study;
 for(const status of ['active','pause','drop']){
  if(status!=='active')assert.equal((await cli(['study',status,study.id])).code,0);
  const versions=Object.keys(await rows()).filter(k=>k.startsWith('library_versions/')).length;
  const count=Object.keys(await rows()).filter(k=>k.startsWith('practice_studies/')).length;
  let c=(await meta()).practice.current;if(c&&!c.attempt_id)await answer();
  const r=await shelf('rosary-joyful');assert.equal(r.status,303,status);
  c=(await meta()).practice.current;assert.equal(c.prompt.version_id,listCopy,status);assert.equal(c.prompt.study_id,study.id,status);
  assert.equal(Object.keys(await rows()).filter(k=>k.startsWith('library_versions/')).length,versions,`${status}: no second Version`);
  assert.equal(Object.keys(await rows()).filter(k=>k.startsWith('practice_studies/')).length,count,`${status}: no second Study`);
 }
});

test('SPEC § Open into study: on the starter\'s turn, Study never refuses or bypasses the Owner\'s dropped or paused copy',{timeout:180000},async t=>{
 for(const status of ['drop','pause']){
  const directory=await mkdtemp(join(tmpdir(),'scry-library-turn-'));
  const config=join(directory,'login.json'),name='library-turn-'+randomUUID();
  const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey};
  let runtime=await createRuntime({port:0,name,inspect:true,bindings});
  try{
   const origin=(await runtime.mf.ready).origin;
   runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin})};
   const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
   const rows=async()=>Object.fromEntries((await storage.exec('SELECT key,value FROM records ORDER BY key')).map(r=>[r.key,r.value]));
   const page=async path=>(await(await runtime.worker.fetch(origin+path)).text());
   const hidden=(html,action)=>{const m=[...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m=>m[1]===action);assert.ok(m,action);return Object.fromEntries([...m[2].matchAll(/<input type="hidden" name="([^"]+)" value="([^"]*)"/g)].map(x=>[x[1],x[2]]));};
   const post=(path,body)=>runtime.worker.fetch(origin+path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(body)});
   const issued=await post('/tokens/create',{...hidden(await page('/settings'),'/tokens/create'),name:'Synthetic turn agent; unpaid'});
   const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())?.[1];assert.ok(secret);
   const api=async(path,payload)=>{const r=await runtime.worker.fetch(origin+path,{...(payload?{method:'POST',body:JSON.stringify({operation_id:randomUUID(),...payload})}:{}),headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'}});const body=await r.json();assert.equal(r.status,200,JSON.stringify(body));return body;};
   const lessons=JSON.parse(await (await import('node:fs/promises')).readFile('assets/lessons.json','utf8'));
   const michael=lessons.find(l=>l.id==='st-michael-en').input.text;
   const copy=await post('/library/version/add',{...hidden(await page('/library/add'),'/library/version/add'),title:'My Saint Michael',text:michael});
   const copyId=copy.headers.get('Location').split('/').at(-1);
   const fillers=['NATO spelling alphabet','Joyful Mysteries','Luminous Mysteries','Sorrowful Mysteries','Glorious Mysteries','Safe soothing','Everyday readiness','Montessori-informed care'];
   const ids=[copyId];
   for(const [i,title] of fillers.entries()) ids.push((await api('/api/v1/library/versions',{data:{title,kind:'text',text:`Filler words number ${i}.\n`}})).result.entity_id);
   for(const [i,id] of ids.entries()){const s=(await api('/api/v1/learn',{version_id:id})).practice.study;await api(i===0&&status==='pause'?'/api/v1/studies/pause':'/api/v1/studies/drop',{study_id:s.id});}
   const versions=Object.keys(await rows()).filter(k=>k.startsWith('library_versions/')).length;
   const studies=Object.keys(await rows()).filter(k=>k.startsWith('practice_studies/')).length;
   // Where Home offers Study, pressing it never answers a refusal.
   const home=await page('/');
   if(home.includes('action="/practice/review/start"')){
    const r=await post('/practice/review/start',hidden(home,'/practice/review/start'));
    assert.equal(r.status,303,`${status}: Study answers without a refusal`);
   }
   assert.equal(Object.keys(await rows()).filter(k=>k.startsWith('library_versions/')).length,versions,`${status}: no second Version`);
   assert.equal(Object.keys(await rows()).filter(k=>k.startsWith('practice_studies/')).length,studies,`${status}: no second Study`);
   assert.match(await page('/'),/Rest for now/,`${status}: nothing is due and the copy is not bypassed`);
  } finally {await runtime.mf.dispose();await rm(directory,{recursive:true,force:true});}
 }
});
