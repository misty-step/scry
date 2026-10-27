// Actual compiled MCP -> Owner API -> built Worker/SQLite, no browser or egress.
import assert from 'node:assert/strict';
import test from 'node:test';
import {spawn,spawnSync} from 'node:child_process';
import {createHash,randomUUID} from 'node:crypto';
import {mkdtemp,readFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {dirname,join,resolve} from 'node:path';
import {createInterface} from 'node:readline';
import {once} from 'node:events';
import {isDeepStrictEqual} from 'node:util';
import {createRuntime,internalKey} from './runtime-helper.mjs';

const binary=process.env.SCRY_MCP_BIN || resolve('target/debug/scry-mcp');
if(!process.env.SCRY_MCP_BIN){
 const built=spawnSync('cargo',['build','--locked','--bin','scry','--bin','scry-mcp'],{encoding:'utf8'});
 assert.equal(built.status,0,built.stderr);
}
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
function successfulReplay(before,after){
 assert.ok(isDeepStrictEqual(after.filter(row=>row.key!=='meta'),before.filter(row=>row.key!=='meta')),'Replay preserves every non-meta row under the fixed fixture clock.');
 const previous=JSON.parse(before.find(row=>row.key==='meta').value),current=JSON.parse(after.find(row=>row.key==='meta').value);
 assert.equal(current.revision,previous.revision+1,'A successful replay advances the transport CAS once.');
 assert.ok(isDeepStrictEqual({...current,revision:previous.revision},previous),'No domain, schedule, receipt or other meta field changes.');
}

test('Compiled MCP authors NATO, learns, grades/replays once, and preserves native assistance fences in actual SQLite', {timeout:120000}, async t=>{
 const directory=await mkdtemp(join(tmpdir(),'scry-mcp-worker-'));
 const config=join(directory,'login.json'),name='mcp-worker-'+randomUUID();
 const at=(Math.floor(Date.now()/86400000)+1)*86400000+12*60*60*1000;
 const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey,SCRY_TEST_NOW_MS:String(at)};
 const artifact=resolve(process.env.SCRY_WORKER_ARTIFACT||'build');
 const wasmHash=sha(await readFile(join(artifact,'index_bg.wasm'))),mcpHash=sha(await readFile(binary));
 let egress=0,runtime,child,closed;
 t.after(async()=>{
  let done;
  try {if(child){child.stdin.end();done=await Promise.race([closed,new Promise(resolve=>setTimeout(()=>resolve(null),2000).unref())]);if(!done){child.kill('SIGKILL');await closed;}}}
  finally {try {await runtime?.mf.dispose();} finally {await rm(directory,{recursive:true,force:true});}}
  if(child){assert.ok(done,'MCP exits on EOF.');assert.equal(done[0],0);}
  assert.equal(egress,0,'No external calls from supplied-content/native grading.');
 });
 runtime=await createRuntime({port:0,name,inspect:true,testClock:true,bindings,egress:async()=>{egress++;throw Error('No external egress in MCP proof.');}});
 const origin=(await runtime.mf.ready).origin;
 runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin})};
 const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
 const rows=()=>storage.exec('SELECT key,value FROM records ORDER BY key');

 // Reuse the native local Owner-token form and compiled CLI login. No row seed,
 // browser cookie, hosted credential, or hand-built domain record is substituted.
 const settings=await(await runtime.worker.fetch(origin+'/settings')).text();
 const form=/<form[^>]*action="\/tokens\/create"[^>]*>([\s\S]*?)<\/form>/.exec(settings);assert.ok(form);
 const fields=Object.fromEntries([...form[1].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
 const issued=await runtime.worker.fetch(origin+'/tokens/create',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,name:'Synthetic MCP proof; unpaid'})});assert.equal(issued.status,200);
 const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())?.[1];assert.ok(secret);
 const login=spawn(join(dirname(binary),'scry'),['login','--url',origin,'--token-stdin','--config',config,'--json'],{env:{PATH:process.env.PATH,HOME:directory},stdio:['pipe','pipe','pipe']});
 let loginOutput='',loginError='';login.stdout.on('data',v=>loginOutput+=v);login.stderr.on('data',v=>loginError+=v);
 const loginClosed=once(login,'close');login.stdin.end(secret+'\n');assert.equal((await loginClosed)[0],0);
 assert.equal(loginError,'');assert.ok(!loginOutput.includes(secret));assert.equal(JSON.parse(loginOutput).owner,true);

 child=spawn(binary,['--config',config],{env:{PATH:process.env.PATH,HOME:directory},stdio:['pipe','pipe','pipe']});closed=once(child,'close');
 let serial=0,stderr='';const pending=new Map();child.stderr.on('data',v=>stderr+=v);
 createInterface({input:child.stdout}).on('line',line=>{
  assert.ok(!line.includes(secret),'MCP never emits the Owner credential.');
  const value=JSON.parse(line);assert.equal(value.jsonrpc,'2.0');assert.ok(pending.has(value.id),'One response per outstanding MCP request.');
  pending.get(value.id)(value);pending.delete(value.id);
 });
 const send=value=>child.stdin.write(JSON.stringify(value)+'\n');
 const request=(method,params={})=>new Promise(done=>{const id=++serial;pending.set(id,done);send({jsonrpc:'2.0',id,method,params});});
 const call=async(name,args={})=>{
  const response=await request('tools/call',{name,arguments:args});assert.equal(response.error,undefined);
  assert.deepEqual(JSON.parse(response.result.content[0].text),response.result.structuredContent);
  return response.result;
 };
 const ok=async(name,args={})=>{const r=await call(name,args);assert.equal(r.isError,false,JSON.stringify(r.structuredContent));return r.structuredContent;};
 const refused=async(name,args,code)=>{const before=await rows();const r=await call(name,args);assert.equal(r.isError,true);assert.equal(r.structuredContent.error.code,code);assert.ok(isDeepStrictEqual(await rows(),before),'Refusal leaves every SQLite row unchanged.');};
 const initialized=await request('initialize',{protocolVersion:'2025-11-25',capabilities:{},clientInfo:{name:'scry-workerd-proof',version:'1'}});assert.equal(initialized.result.protocolVersion,'2025-11-25');
 send({jsonrpc:'2.0',method:'notifications/initialized'});

 const data=JSON.parse(await readFile('examples/nato-alphabet.json','utf8'));
 assert.equal((await ok('scry_search',{query:'NATO'})).empty,true);
 assert.equal((await ok('scry_validate',{format:'mapping',data})).units,26);
 const imported=await ok('scry_import',{operation_id:'mcp-worker-import-operation',format:'mapping',data});
 const vid=imported.result.entity_id,shown=await ok('scry_show',{kind:'version',id:vid});
 assert.equal(shown.version.kind,'keyed_mapping');assert.equal(shown.version.provenance,'supplied');
 const units=shown.version.sections.flatMap(section=>section.units);assert.equal(units.length,26);
 assert.equal(units.map(unit=>unit.text.split('\t')[0]).join(''),'ABCDEFGHIJKLMNOPQRSTUVWXYZ');
 const sid=(await ok('scry_learn',{operation_id:'mcp-worker-learn-operation',version_id:vid,pace:'intense'})).practice.study.id;
 const first=(await ok('scry_next',{operation_id:'mcp-worker-next-operation',study_id:sid})).practice;
 assert.equal(first.prompt.cue,'A');assert.equal(first.prompt.assisted,false);assert.ok(!JSON.stringify(first).includes('Alfa'));
 const answer={operation_id:'mcp-worker-answer-operation',prompt_id:first.prompt.id,answer:'Alpha'};
 const answered=await ok('scry_answer',answer),attempt=answered.practice.attempt;
 assert.equal(attempt.grade.verdict,'Again');assert.equal(attempt.grade.authority.name,'exact-v2');
 assert.ok(attempt.grade.authority.grader_version.endsWith(';mapping=1'));
 assert.equal(attempt.assisted,false);assert.equal(attempt.effects[0].scheduled,true);
 const frozen=await rows(),replayed=await ok('scry_answer',answer);
 assert.equal(replayed.replayed,true);assert.deepEqual(replayed.practice.attempt,attempt);successfulReplay(frozen,await rows());
 await refused('scry_answer',{...answer,answer:'Alfa'},'conflict');
 await refused('scry_answer',{...answer,verdict:'Good'},'invalid_arguments');
 const held=(await ok('scry_next',{operation_id:'mcp-worker-held-operation'})).practice;
 assert.equal(held.attempt.id,attempt.id,'No automatic advance after judgment.');

 const second=(await ok('scry_next',{operation_id:'mcp-worker-second-operation',after:first.prompt.id,study_id:sid})).practice;
 assert.equal(second.prompt.cue,'B');assert.equal(second.prompt.assisted,false);
 await refused('scry_answer',answer,'conflict'); // #349: old disclosure cannot bypass new unaided Prompt.
 await refused('scry_show',{kind:'version',id:vid},'conflict');
 const reveal=(await ok('scry_reveal',{operation_id:'mcp-worker-reveal-operation',prompt_id:second.prompt.id})).practice;
 assert.equal(reveal.assisted,true);assert.ok(reveal.expected.some(unit=>unit.text==='Bravo'));
 const helped=(await ok('scry_answer',{operation_id:'mcp-worker-helped-operation',prompt_id:second.prompt.id,answer:'Bravo'})).practice.attempt;
 assert.equal(helped.grade.verdict,'Good');assert.equal(helped.assisted,true);assert.equal(helped.effects[0].scheduled,false);
 assert.deepEqual((await ok('scry_answer',answer)).practice.attempt,attempt,'Old answer replay works once current Prompt is answered.');
 const due=await ok('scry_due');assert.ok(due.cards.length>=2);assert.ok(due.cards.every(card=>card.direction==='forward'));
 const exported=await runtime.worker.fetch(origin+'/api/v1/export',{headers:{Authorization:'Bearer '+secret}});assert.equal(exported.status,200);
 const app=(await exported.json()).app;
 assert.deepEqual(Object.keys(app.practice.attempts).sort(),[attempt.id,reveal.disclosure_attempt_id,helped.id].sort());
 const disclosure=app.practice.attempts[reveal.disclosure_attempt_id];
 assert.equal(disclosure.disclosure,true);assert.equal(disclosure.assisted,true);assert.equal(disclosure.revealed,true);
 assert.equal(disclosure.effects[0].scheduled,false);assert.equal(disclosure.raw_answer,'');
 assert.deepEqual(app.practice.attempts[attempt.id],attempt);assert.deepEqual(app.practice.attempts[helped.id],helped);
 assert.deepEqual(app.practice.corrections,{});
 assert.deepEqual(app.jobs,{});assert.deepEqual(app.assessments,{});assert.deepEqual(app.spend,[]);
 assert.equal(stderr,'');assert.equal(pending.size,0);
 assert.equal(sha(await readFile(join(artifact,'index_bg.wasm'))),wasmHash);assert.equal(sha(await readFile(binary)),mcpHash);
 console.log('MCP Worker receipt: '+JSON.stringify({format:'scry-mcp-workerd-proof-v1',mcp_sha256:mcpHash,worker_wasm_sha256:wasmHash,units:26,answer_attempts:2,disclosure_attempts:1,wrong_spelling:'Again',revealed_correct:'assisted Good',answer_replay_domain_and_receipts_unchanged:true,replay_cas_revision_increment:1,protected_replay_and_library_reads:409,client_grade_refused_before_http:true,external_egress:egress,limits:['synthetic local Owner token','loopback workerd/SQLite','no hosted or physical-phone claim']}));
});
