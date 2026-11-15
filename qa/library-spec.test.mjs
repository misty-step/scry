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
  assert.equal(viaCli.result.entity_id,first);
  const replay=await ok(['version','add','--title','Spec second Work','--kind','text','--text-file',variant,'--operation-id','spec-qa-dup-cli-op']);
  assert.equal(replay.replayed,true);assert.equal(replay.result.entity_id,first);
  await unchanged(async()=>{const reuse=await cli(['version','add','--title','Spec second Work','--kind','text','--text-file',await file('other.txt','Different.\n'),'--operation-id','spec-qa-dup-cli-op']);assert.notEqual(reuse.code,0);},'Reusing an operation with another payload changes nothing.');
  const viaApi=await apiJson('/api/v1/library/versions',{operation_id:'spec-qa-dup-api-operation',data:{title:'Spec second Work',kind:'text',text:'shared   spec words, in order.',aliases:['Spec first Work']}});
  assert.equal(viaApi.status,200);assert.equal(viaApi.body.result.entity_id,first);
  const add=(await page('/library/add')).html;
  const viaApp=await post('/library/version/add',{...formFields(add,'/library/version/add'),title:'Spec second Work',text:'Shared spec words; in order.\n'});
  assert.equal(viaApp.status,303);assert.equal(viaApp.headers.get('Location'),'/library/versions/'+first);
  const viaTool=await toolOk('scry_import',{operation_id:'spec-qa-dup-tool-operation',format:'version',data:{title:'Spec second Work',kind:'text',text:'Shared Spec Words In Order'}});
  assert.equal(viaTool.result.entity_id,first);
  assert.deepEqual(await library(),before,'No client created or attached anything.');
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
