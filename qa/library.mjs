import assert from 'node:assert/strict';
import {randomUUID,createHash} from 'node:crypto';
import {spawn} from 'node:child_process';
import {mkdtemp,mkdir,writeFile,rm} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {createRuntime,internalKey} from './runtime-helper.mjs';

// Authored, caller-supplied text. Creed labels exercise metadata, not canonical
// Roman Missal wording or the owner's future full-text private-input acceptance.
const text='Synthetic opening clause, next clause.\r\nA second authored article; final clause.\n';
export async function libraryJourney() {
 const space='library-'+randomUUID(),name='library-'+randomUUID();
 const base={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:space,INTERNAL_KEY:internalKey};
 let runtime=await createRuntime({port:0,name,inspect:true,bindings:base});
 runtime={...runtime,...await runtime.rebind({...base,CANONICAL_ORIGIN:(await runtime.mf.ready).origin})};
 const origin=runtime.url;
 let storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
 await mkdir(join(process.env.HOME,'.cache/tmp'),{recursive:true});
 const temp=await mkdtemp(join(process.env.HOME,'.cache/tmp/scry-library-'));
 const config=join(temp,'login.json'),binary=resolve(process.env.SCRY_LIBRARY_CLI||'target/debug/scry');
 const transcript=[];let secret,tokenId,workId,oldId,secondId,newId,original;
 const rows=()=>storage.exec('SELECT key,value FROM records ORDER BY key');
 const request=(path,payload,token=secret)=>runtime.worker.fetch(origin+path,{...(payload?{method:'POST',body:JSON.stringify(payload)}:{}),headers:{...(token?{Authorization:'Bearer '+token}:{}),'Content-Type':'application/json'}});
 const file=async(name,contents)=>{const path=join(temp,name);await writeFile(path,contents);return path;};
 const originalFile=await file('supplied.txt',text);
 const cli=(args,input='')=>new Promise((done,reject)=>{
  const proc=spawn(binary,[...args,'--json','--config',config],{env:{PATH:process.env.PATH,HOME:temp},stdio:['pipe','pipe','pipe']});
  let out='',err='';proc.stdout.on('data',d=>out+=d);proc.stderr.on('data',d=>err+=d);proc.on('error',reject);
  proc.on('exit',code=>{try{
   assert.ok(!secret||(!out.includes(secret)&&!err.includes(secret)),'No secret in CLI output.');
   const value=JSON.parse(out);transcript.push({command:['scry',...args.map(a=>a.startsWith(temp)?a.replace(temp,'<fixture>'):a),'--json'].join(' '),exit:code,output:value});done({code,value});
  }catch(error){reject(error);}});proc.stdin.end(input);
 });
 const ok=async args=>{const result=await cli(args);assert.equal(result.code,0,JSON.stringify(result.value));return result.value;};
 const show=async()=>ok(['work','show',workId]);
 const lib=async()=>ok(['export','--library']);
 const addArgs=(path=originalFile,title="Apostles' Creed")=>['version','add','--title',title,'--kind','text','--text-file',path,'--name','English, Roman Missal 2011 (synthetic fixture)','--alias','Symbolum Apostolorum','--source','Caller-authored synthetic excerpt; no canonical wording claimed'];
 const noSpend=async()=>{const full=await(await request('/api/v1/export')).json();assert.deepEqual(full.app.spend,[]);assert.deepEqual(full.app.jobs,{});assert.deepEqual(full.app.assessments,{});return full;};
 const html=await(await runtime.worker.fetch(origin+'/settings')).text();
 const form=/<form[^>]*action="\/tokens\/create"[^>]*>([\s\S]*?)<\/form>/.exec(html);assert.ok(form);
 const fields=Object.fromEntries([...form[1].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
 const minted=await runtime.worker.fetch(origin+'/tokens/create',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,name:'Library synthetic unpaid agent'})});
 assert.equal(minted.status,200);secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())?.[1];assert.ok(secret);tokenId=secret.slice(8,40);
 assert.equal((await cli(['login','--url',origin,'--token-stdin'],secret+'\n')).code,0);
 const criteria=[
  async()=>{
   const empty=await ok(['search','apostles creed']);assert.deepEqual(empty,{matches:[],empty:true,message:'Nothing matches.'});
   for(const route of ['/api/v1/library/search?q=apostles','/api/v1/library/export']) {
    assert.equal((await request(route,undefined,null)).status,401);
    assert.equal((await request(route,undefined,'invalid')).status,401);
   }
  },
  async()=>{
   const added=await ok([...addArgs(),'--operation-id','library-initial-add-operation']);oldId=added.result.entity_id;
   const version=await ok(['version','show',oldId]);original=version.version;workId=version.work.id;
   assert.equal(version.work.title,"Apostles' Creed");assert.equal(original.name,'English, Roman Missal 2011 (synthetic fixture)');
   assert.equal(original.provenance,'supplied');assert.equal(original.rights,'private_only');assert.equal(original.kind,'text');
   assert.equal(original.text,text);assert.equal(original.sections.length,2);assert.ok(original.sections.every(s=>s.units.length>=2));
   assert.equal(original.sections.flatMap(s=>s.units).map(u=>u.text).join(''),text,'Exact submitted UTF-8/CRLF/trailing-newline byte join.');
   const exported=await lib();assert.equal(Object.keys(exported.works).length,1);assert.equal(Object.keys(exported.versions).length,1);
   for(const query of ['apostles creed','Symbolum Apostolorum','synthetic opening clause']) {
    const result=await ok(['search',query]);assert.equal(result.empty,false);assert.equal(result.matches[0].work.id,workId);assert.equal(result.matches[0].versions[0].id,oldId);
   }
   const keys=(await rows()).map(r=>r.key);assert.ok(keys.some(k=>k.startsWith('library_works/')));assert.ok(keys.some(k=>k.startsWith('library_versions/')));assert.ok(!JSON.parse((await rows()).find(r=>r.key==='meta').value).library);
   await noSpend();
  },
  async()=>{
   const before=await lib();
   const replay=await ok([...addArgs(),'--operation-id','library-initial-add-operation']);assert.equal(replay.replayed,true);assert.equal(replay.result.entity_id,oldId);
   const exact=await ok(addArgs());assert.equal(exact.result.entity_id,oldId);
   const punct=await file('punctuation.txt','SYNTHETIC OPENING CLAUSE NEXT CLAUSE\n A SECOND AUTHORED ARTICLE FINAL CLAUSE!!!');
   assert.equal((await ok(addArgs(punct))).result.entity_id,oldId);
   assert.deepEqual(await lib(),before,'Dedupe creates no Library records or edits.');
   const conflict=await cli([...addArgs(),'--context','changed payload','--operation-id','library-initial-add-operation']);assert.notEqual(conflict.code,0);assert.match(conflict.value.error,/conflict/);
   const same='library-concurrent-add-operation';const [a,b]=await Promise.all([cli([...addArgs(),'--operation-id',same]),cli([...addArgs(),'--operation-id',same])]);assert.equal(a.code,0);assert.equal(b.code,0);assert.equal(a.value.result.entity_id,b.value.result.entity_id);assert.ok(a.value.replayed||b.value.replayed);
  },
  async()=>{
   const path=await file('second.txt','Synthetic alternate opening, a second wording.\n');
   const second=await ok(addArgs(path,'Symbolum Apostolorum'));secondId=second.result.entity_id;assert.notEqual(secondId,oldId);
   const work=await show();assert.equal(work.versions.length,2);assert.ok(work.versions.every(v=>v.work_id===workId));assert.equal(Object.keys((await lib()).works).length,1);
  },
  async()=>{
   const bad=await file('bad-sections.json',JSON.stringify([{heading:'Article',units:['Missing exact bytes.']}]));
   const before=await rows();const refused=await cli([...addArgs(),'--title','Must never be stored','--sections-file',bad]);
   assert.notEqual(refused.code,0);assert.deepEqual(await rows(),before,'Refusal changes no SQLite row, operation receipt or last-use time.');
   const valid=await file('sections.json',JSON.stringify([{heading:'Synthetic article one',units:['Supplied first clause,',' second clause.\n']} ]));
   const path=await file('explicit.txt','Supplied first clause, second clause.\n');
   const accepted=await ok([...addArgs(path,'Explicit Sections'),'--alias','Explicit alias','--sections-file',valid]);
   const version=await ok(['version','show',accepted.result.entity_id]);assert.equal(version.version.sections[0].heading,'Synthetic article one');
   // This input deliberately shares an alias with the Creed: it must attach.
   assert.equal(version.work.id,workId);
  },
  async()=>{
   const rev=(await show()).work.revision;const path=await file('corrected.txt','Synthetic corrected opening clause, next clause.\n');
   const args=['version','supersede',oldId,...addArgs(path).slice(2),'--revision',String(rev),'--operation-id','library-supersede-operation'];
   newId=(await ok(args)).result.entity_id;assert.notEqual(newId,oldId);
   assert.deepEqual((await ok(['version','show',oldId])).version,original,'Old Version bytes, metadata and Unit identities stay unchanged.');
   assert.ok((await ok(['version','show',oldId])).relations.some(r=>r.kind==='supersedes'&&r.version_id===newId&&r.previous_id===oldId));
   const replay=await ok(args);assert.equal(replay.replayed,true);assert.equal(replay.result.entity_id,newId);
   const stale=await cli([...args.slice(0,-2),'--operation-id','library-stale-supersede-op']);assert.notEqual(stale.code,0);
   // Actual complete local R2 archive, produced by the built Worker; no operator or live capability.
   const ns=await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE');const stub=ns.get(ns.idFromName(space));
   const backed=await stub.fetch('https://internal.invalid/__internal/daily',{method:'POST',headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});assert.equal(backed.status,200);
   const bucket=await runtime.mf.getR2Bucket('SCRY_BACKUPS');const list=await bucket.list({prefix:`snapshots/${space}/`});assert.ok(list.objects.length);
   const bytes=Buffer.from(await(await bucket.get(list.objects.at(-1).key)).arrayBuffer());const archive=JSON.parse(bytes);
   assert.equal(archive.format,'scry-rust-durable-sqlite-library-v1');assert.deepEqual(archive.app.library,await lib());
   transcript.push({observation:'Built Worker complete local R2 readback',archive_sha256:createHash('sha256').update(bytes).digest('hex'),versions:Object.keys(archive.app.library.versions).length,limit:'Local emulator; no independent hosted recovery.'});
  },
  async()=>{
   const path=await file('unused.txt','A separate unused synthetic text.\n');
   const added=await ok(['version','add','--title','Unused Work','--kind','text','--text-file',path]);const version=await ok(['version','show',added.result.entity_id]);
   const wid=version.work.id,vid=version.version.id;
   await ok(['work','edit',wid,'--revision','1','--title','Renamed unused Work','--alias','Unused alias','--context','Caller-supplied context']);
   const edited=await ok(['work','show',wid]);assert.equal(edited.work.title,'Renamed unused Work');assert.deepEqual(edited.work.aliases,['Unused alias']);assert.equal(edited.work.context,'Caller-supplied context');assert.deepEqual(edited.versions[0],version.version);
   const before=await rows();assert.notEqual((await cli(['version','delete',vid,'--revision','1'])).code,0);assert.deepEqual(await rows(),before);
   await ok(['version','delete',vid,'--revision','2']);assert.notEqual((await cli(['version','show',vid])).code,0);
   await ok(['work','delete',wid,'--revision','3']);assert.notEqual((await cli(['work','show',wid])).code,0);
   const another=await ok(['version','add','--title','Whole unused Work','--kind','text','--text-file',path]);const w=await ok(['version','show',another.result.entity_id]);
   await ok(['work','delete',w.work.id,'--revision','1']);assert.notEqual((await cli(['version','show',w.version.id])).code,0);
   transcript.push({observation:'Unused Version and Work deletion passed through CLI/Worker/SQLite.',used_study:'Unit reference-seam proof only in supersede_retains_original_and_study_reference_refuses_deletion; real Study/Attempt journey waits for #317.'});
  },
  async()=>{
   const exported=await lib();assert.deepEqual(Object.keys(exported).sort(),['relations','versions','works']);
   const forbidden=new Set(['study','studies','cards','attempts','requests','owner_tokens','tokens','operations','spend','jobs','assessments','study_version_refs']);
   const check=v=>{if(v&&typeof v==='object'){for(const [k,x]of Object.entries(v)){assert.ok(!forbidden.has(k),`No private study/audit field ${k}`);check(x);}}};check(exported);
   assert.ok(!JSON.stringify(exported).includes(secret));
   const full=await noSpend();assert.ok(full.app.owner_tokens[tokenId]);
   const receipts=Object.values(full.app.operations).filter(r=>r.path?.startsWith('/library/'));assert.ok(receipts.length>=6);assert.ok(receipts.every(r=>r.acting_token===tokenId));
   assert.ok((await(await request('/api/v1/history')).json()).operations['library-supersede-operation'].acting_token===tokenId);
   // Rebind the same actual SQLite object to a fresh Worker instance configuration.
   const next=await runtime.rebind({...base,CANONICAL_ORIGIN:origin,RELEASE_REVISION:'library-readback'});runtime={...runtime,...next};
   storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
   assert.deepEqual(await lib(),exported);assert.deepEqual((await ok(['version','show',oldId])).version,original);
   const json=JSON.stringify(transcript);assert.ok(!json.includes(secret));assert.ok(!JSON.stringify(await rows()).includes(secret));
  }
 ];
 return {criteria,transcript,close:async()=>{await runtime.mf.dispose();await rm(temp,{recursive:true});}};
}
