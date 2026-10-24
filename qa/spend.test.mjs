import test from 'node:test';
import assert from 'node:assert/strict';
import {randomUUID,createHash,generateKeyPairSync,sign} from 'node:crypto';
import {spawn} from 'node:child_process';
import {mkdtemp,mkdir,readFile,writeFile,readdir,rm,chmod} from 'node:fs/promises';
import {join,resolve,relative} from 'node:path';
import {createServer} from 'node:http';
import {buildCli} from './owner-api.mjs';
import {createRuntime,internalKey,Response} from './runtime-helper.mjs';

buildCli();
test('US-019.7 / #321: spend CLI → built Worker → byte-identical SQLite',async()=>{
 await mkdir(join(process.env.HOME,'.cache/tmp'),{recursive:true});
 const temp=await mkdtemp(join(process.env.HOME,'.cache/tmp/scry-spend-'));
 const persistence=join(temp,'state'),config=join(temp,'login.json');
 const name='spend-'+randomUUID(),space='spend-'+randomUUID();
 const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:space,INTERNAL_KEY:internalKey};
 let runtime=await createRuntime({port:0,name,bindings,inspect:true,resourcePersistencePath:persistence});
 runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:(await runtime.mf.ready).origin})};
 const origin=runtime.url,storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
  let secret;const transcript=[];
 const request=(path,token=secret,init={})=>runtime.worker.fetch(origin+path,{redirect:'manual',...init,headers:{...(token?{Authorization:'Bearer '+token}:{}),...init.headers}});
 const rows=()=>storage.exec('SELECT key,value FROM records ORDER BY key');
 const form=(html,action)=>{const match=[...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m=>m[1]===action);assert.ok(match);return Object.fromEntries([...match[2].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));};
 const cli=(args,input='')=>new Promise((done,reject)=>{
  const proc=spawn(resolve('target/debug/scry'),[...args,'--json','--config',config],{env:{PATH:process.env.PATH,HOME:temp},stdio:['pipe','pipe','pipe']});
  let out='',err='';proc.stdout.on('data',d=>out+=d);proc.stderr.on('data',d=>err+=d);proc.on('error',reject);
  proc.on('exit',code=>{try{assert.ok(!secret||(!out.includes(secret)&&!err.includes(secret)));const value=JSON.parse(out);transcript.push({command:['scry',...args,'--json'].join(' '),exit:code,output:value});done({code,value});}catch(e){reject(e);}});proc.stdin.end(input);
 });
 const files=async()=>{
  const result=[];
  async function walk(dir){for(const entry of await readdir(dir,{withFileTypes:true})){const path=join(dir,entry.name);if(entry.isDirectory())await walk(path);else result.push({path:relative(persistence,path),sha256:createHash('sha256').update(await readFile(path)).digest('hex')});}}
  await walk(persistence);return result.sort((a,b)=>a.path.localeCompare(b.path));
 };
 try {
  const create=await(await runtime.worker.fetch(origin+'/create')).text();
  assert.equal((await runtime.worker.fetch(origin+'/__fixture',{redirect:'manual',method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(form(create,'/create'))})).status,303);
  const html=await(await runtime.worker.fetch(origin+'/settings')).text();
  const issued=await runtime.worker.fetch(origin+'/tokens/create',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...form(html,'/tokens/create'),name:'synthetic unpaid spend reader'})});assert.equal(issued.status,200);
  secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())?.[1];assert.ok(secret);
  assert.equal((await cli(['login','--url',origin,'--token-stdin'],secret+'\n')).code,0);
  const job=JSON.parse((await rows()).find(r=>r.key.startsWith('jobs/')).value);
  const raw='Synthetic private provider body must never appear in spend';
  const entries=[
   {id:'known-before-midnight',created_ms:Date.parse('2026-10-19T23:59:59.999Z'),purpose:'generate.concepts',model:'synthetic-model',cost_micros:123,status:'known'},
   {id:'unknown-at-midnight',created_ms:Date.parse('2026-10-20T00:00:00.000Z'),purpose:'generate.concepts',model:'synthetic-model',cost_micros:null,status:'unknown'},
   {id:'legacy-missing',created_ms:Date.parse('2026-10-20T01:00:00Z'),cost_micros:456,status:'known'},
   {id:'legacy-empty',created_ms:Date.parse('2026-10-20T02:00:00Z'),purpose:'',model:'',cost_micros:null,status:'unknown'},
   {id:'retained-attribution',created_ms:Date.parse('2026-10-20T03:00:00Z'),purpose:'historical.custom',model:'retained-model',cost_micros:0,status:'known'},
  ].map(e=>({work_id:job.id,reserved_micros:500000,response_hash:null,response_model:null,raw_response:null,response_bytes:null,response_outcome:null,...e}));
  entries[0]={...entries[0],raw_response:raw,response_hash:createHash('sha256').update(raw).digest('hex'),response_model:'reported-model',response_bytes:Buffer.byteLength(raw),response_outcome:'received'};
  for(let i=0;i<entries.length;i++)await storage.exec('INSERT INTO records(key,value) VALUES(?,?)',`spend/${String(i).padStart(16,'0')}`,JSON.stringify(entries[i]));
  // Capture ALL persisted resources, plus every record value and SQLite's write
  // counter. File checks include DB/WAL/SHM, alarm metadata and R2, if present.
  const before=await rows(),changes=await storage.exec('SELECT total_changes() AS n'),beforeFiles=await files();
  assert.ok(beforeFiles.some(f=>f.path.endsWith('.sqlite')),'Compare actual persisted database bytes.');
  let report;
  for(let repeat=0;repeat<2;repeat++)for(const by of ['day','purpose','model']){
   const response=await request('/api/v1/spend?by='+by);assert.equal(response.status,200);assert.equal(response.headers.get('Cache-Control'),'private, no-store, max-age=0');
   const api=await response.json();assert.equal(api.by,by);assert.equal(api.day_basis,'UTC');assert.equal(api.currency,'USD');
   const result=await cli(['spend','--by',by]);assert.equal(result.code,0,JSON.stringify(result.value));assert.deepEqual(result.value,api,'CLI syntax success must agree with real API values.');
   assert.deepEqual(api.totals,{entries:5,unknown_entries:2,reserved_micros:2500000,known_cost_micros:579,unknown_held_micros:1000000});
   for(const group of api.groups){const selected=api.entries.filter(e=>e[by]===group.key);assert.equal(group.totals.entries,selected.length);assert.equal(group.totals.reserved_micros,selected.reduce((n,e)=>n+e.reserved_micros,0));assert.equal(group.totals.known_cost_micros,selected.reduce((n,e)=>n+(e.cost_micros??0),0));assert.equal(group.totals.unknown_held_micros,selected.reduce((n,e)=>n+e.unknown_held_micros,0));}
   for(const key of Object.keys(api.totals))assert.equal(api.groups.reduce((n,g)=>n+g.totals[key],0),api.totals[key]);
   assert.equal(api.entries[0].day,'2026-10-19');assert.equal(api.entries[1].day,'2026-10-20');
   assert.equal(api.entries[2].purpose,'unattributed');assert.equal(api.entries[2].model,'unattributed');assert.equal(api.entries[2].cost_micros,456);assert.equal(api.entries[3].purpose,'unattributed');assert.equal(api.entries[3].cost_micros,null);
   assert.equal(api.entries[4].purpose,'historical.custom');assert.equal(api.entries[4].model,'retained-model');assert.equal(api.entries[4].cost_micros,0);
   assert.ok(api.entries.every(e=>e.work_id===job.id&&e.work_kind==='concept_job'&&e.goal_id===job.goal_id));
   assert.ok(!JSON.stringify(api).includes(raw));assert.ok(!JSON.stringify(api).includes(secret));assert.ok(!JSON.stringify(api).includes('raw_response'));
   assert.deepEqual(await rows(),before);assert.deepEqual(await storage.exec('SELECT total_changes() AS n'),changes);assert.deepEqual(await files(),beforeFiles);report=api;
  }
  for(const suffix of ['', '?by=invalid','?by=day&by=model'])assert.equal((await request('/api/v1/spend'+suffix)).status,422);
  assert.equal((await request('/api/v1/spend?by=day',secret,{method:'HEAD'})).status,200);
  assert.equal((await request('/api/v1/spend',secret,{method:'POST',headers:{'Content-Type':'application/json'},body:'{"operation_id":"no-spend-write"}'})).status,404);
  for(const args of [['spend'],['spend','--by','invalid'],['spend','--by','day','extra']])assert.equal((await cli(args)).code,1);
  assert.deepEqual(await rows(),before);assert.deepEqual(await files(),beforeFiles);
  const tokenKey='owner_tokens/'+secret.slice(8,40),savedToken=(await rows()).find(r=>r.key===tokenKey).value;
  const unknown='scry_ot_'+'0'.repeat(32)+'_'+'a'.repeat(48),forged=secret.slice(0,-1)+(secret.endsWith('0')?'1':'0');let refusal;
  const denied=async token=>{const response=await request('/api/v1/spend?by=purpose',token);const actual={status:response.status,body:await response.text()};assert.equal(actual.status,401);refusal??=actual;assert.deepEqual(actual,refusal);};
  for(const token of [null,'malformed',unknown,forged])await denied(token);
  for(const state of ['expired','revoked']){
   const record=JSON.parse(savedToken);if(state==='expired'){record.expires_ms=Date.now()-1;record.created_ms=Math.min(record.created_ms,record.expires_ms-1);}else record.revoked_ms=Date.now();
   await storage.exec('UPDATE records SET value=? WHERE key=?',JSON.stringify(record),tokenKey);const frozen=await rows();await denied(secret);assert.equal((await cli(['spend','--by','purpose'])).code,1);assert.deepEqual(await rows(),frozen);
   const meta=frozen.find(r=>r.key==='meta').value;
   await storage.exec("UPDATE records SET value='unreadable learner data' WHERE key='meta'");await denied(secret);
   await storage.exec("UPDATE records SET value=? WHERE key='meta'",meta);
  }
  await storage.exec('UPDATE records SET value=? WHERE key=?',savedToken,tokenKey);
  const meta=before.find(r=>r.key==='meta').value;
  await storage.exec("UPDATE records SET value='unreadable learner data' WHERE key='meta'");
  for(const token of [null,'malformed',unknown,forged])await denied(token);
  await storage.exec("UPDATE records SET value=? WHERE key='meta'",meta);
  const login=await readFile(config);await chmod(config,0o644);assert.equal((await cli(['spend','--by','day'])).code,1);await chmod(config,0o600);
  assert.equal((await cli(['spend','--by','day','--url','http://localhost:1'])).code,1);assert.deepEqual(await readFile(config),login);
  let sourceRequests=0,targetRequests=0,mode='redirect';
  const target=createServer((req,res)=>{targetRequests++;res.end('{}');});
  await new Promise(done=>target.listen(0,'127.0.0.1',done));
  const source=createServer((req,res)=>{sourceRequests++;assert.equal(req.url,'/api/v1/spend?by=day');assert.equal(req.headers.authorization,'Bearer '+secret);if(mode==='redirect')res.writeHead(302,{Location:`http://127.0.0.1:${target.address().port}/capture`});else res.writeHead(500);res.end('Synthetic proxy diagnostic '+secret);});
  await new Promise(done=>source.listen(0,'127.0.0.1',done));
  try {
   await writeFile(config,JSON.stringify({origin:`http://127.0.0.1:${source.address().port}`,token:secret}));
   for(mode of ['redirect','error']){const result=await cli(['spend','--by','day']);assert.equal(result.code,1);assert.deepEqual(result.value,{error:'API request failed.'});}
   assert.equal(sourceRequests,2);assert.equal(targetRequests,0,'Never follow a spend redirect or forward its bearer.');
  } finally {await new Promise(done=>source.close(done));await new Promise(done=>target.close(done));await writeFile(config,login);}
  const reader=JSON.parse(savedToken);assert.equal(reader.paid,false);
  const frozen=await rows();const paid=await request('/api/v1/create',secret,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({operation_id:'unpaid-spend-probe',intent:'Must refuse before reservation'})});assert.equal(paid.status,403);assert.deepEqual(await rows(),frozen);
  const reconciliationArgs=['spend','reconcile','unknown-at-midnight','--cost-micros','123','--reference','synthetic-provider-billing-001','--verified-provider-billing','--operation-id','spend-reconciliation-0001'];
  assert.equal((await cli(reconciliationArgs)).code,1,'An unpaid reader cannot release an unknown obligation.');
  const paidHtml=await(await runtime.worker.fetch(origin+'/settings')).text();
  const paidIssue=await runtime.worker.fetch(origin+'/tokens/create',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...form(paidHtml,'/tokens/create'),name:'synthetic budget reconciler',paid:'true'})});
  assert.equal(paidIssue.status,200);secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await paidIssue.text())?.[1];assert.ok(secret);
  assert.equal((await cli(['login','--url',origin,'--token-stdin'],secret+'\n')).code,0);
  const original=JSON.parse((await rows()).find(r=>r.key==='spend/0000000000000001').value);
  assert.equal((await cli(reconciliationArgs)).code,0);
  const reconciled=(await rows()).find(r=>r.key==='spend/0000000000000001').value;
  const saved=JSON.parse(reconciled);assert.equal(saved.cost_micros,null);assert.equal(saved.reconciliation.cost_micros,123);
  const {reconciliation,...preserved}=saved;assert.deepEqual(preserved,original,'Reconciliation appends evidence without rewriting the provider receipt.');
  assert.equal((await cli(reconciliationArgs)).code,0);assert.equal((await rows()).find(r=>r.key==='spend/0000000000000001').value,reconciled);
  const conflict=[...reconciliationArgs];conflict[4]='0';assert.equal((await cli(conflict)).code,1);assert.equal((await rows()).find(r=>r.key==='spend/0000000000000001').value,reconciled);
  assert.equal((await cli(['spend','reconcile','unknown-at-midnight','--cost-micros','0','--reference','synthetic-provider-billing-001'])).code,1,'Billing verification is explicit, never inferred from HTTP success.');
  const resolvedReport=await cli(['spend','--by','day']);assert.equal(resolvedReport.code,0);assert.equal(resolvedReport.value.entries.find(e=>e.id==='unknown-at-midnight').cost_micros,123);
  transcript.push({observation:'Repeated API + CLI reads preserve every SQLite row, total_changes and all persisted resource bytes',files:beforeFiles,totals:report.totals,unpaid_reader:true});
  await mkdir('target',{recursive:true});await writeFile('target/spend-cli.json',JSON.stringify(transcript,null,2)+'\n');
 } finally {await runtime.mf.dispose();await rm(temp,{recursive:true,force:true});}
});

test('Access owner spend reads an empty store without initializing learner records',async()=>{
 const name='empty-spend-'+randomUUID(),space='empty-spend-'+randomUUID(),origin='https://empty-spend.invalid',team='spend-synthetic.cloudflareaccess.com';
 const {publicKey,privateKey}=generateKeyPairSync('rsa',{modulusLength:2048});
 const b64=v=>Buffer.from(JSON.stringify(v)).toString('base64url'),epoch=Math.floor(Date.now()/1000);
 const unsigned=b64({alg:'RS256',kid:'synthetic'})+'.'+b64({iss:`https://${team}`,sub:'synthetic-owner',aud:['synthetic'],iat:epoch,nbf:epoch-1,exp:epoch+600});
 const jwt=unsigned+'.'+sign('RSA-SHA256',Buffer.from(unsigned),privateKey).toString('base64url');
 const runtime=await createRuntime({name,inspect:true,bindings:{SCRY_ENV:'preview',CANONICAL_ORIGIN:origin,SPACE_NAME:space,INTERNAL_KEY:internalKey,ACCESS_TEAM:team,ACCESS_AUDIENCE:'synthetic',OWNER_SUBJECT:'synthetic-owner'},egress:async req=>{assert.equal(req.url,`https://${team}/cdn-cgi/access/certs`);return new Response(JSON.stringify({keys:[{...publicKey.export({format:'jwk'}),kid:'synthetic',alg:'RS256',use:'sig'}]}));}});
 try {
  const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
  const before=await storage.exec('SELECT name,sql FROM sqlite_master ORDER BY name'),changes=await storage.exec('SELECT total_changes() AS n');
  for(const by of ['day','purpose','model']){
   const response=await runtime.worker.fetch(origin+'/api/v1/spend?by='+by,{headers:{'Cf-Access-Jwt-Assertion':jwt}});assert.equal(response.status,200);
   const report=await response.json();assert.deepEqual(report.entries,[]);assert.deepEqual(report.groups,[]);assert.equal(report.totals.entries,0);
  }
  const invalid=await runtime.worker.fetch(origin+'/api/v1/spend?by=day',{headers:{'Cf-Access-Jwt-Assertion':jwt,Authorization:'Basic malformed'}});assert.equal(invalid.status,401);assert.deepEqual(await invalid.json(),{error:'Owner authorization required.'});
  assert.deepEqual(await storage.exec('SELECT name,sql FROM sqlite_master ORDER BY name'),before);assert.deepEqual(await storage.exec('SELECT total_changes() AS n'),changes);assert.ok(!before.some(r=>r.name==='records'));
 } finally {await runtime.mf.dispose();}
});
