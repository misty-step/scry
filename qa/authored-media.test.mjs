// Synthetic authored bytes only. Run against exact built CLI/Worker artifacts;
// this file never installs dependencies, builds, fetches media or calls providers.
import nodeTest from 'node:test';
import assert from 'node:assert/strict';
import {createHash,randomUUID} from 'node:crypto';
import {spawn} from 'node:child_process';
import {mkdtemp,readFile,writeFile,mkdir,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {createRequire} from 'node:module';
import {createRuntime,internalKey} from './runtime-helper.mjs';
import {prepareMappingPredecessor} from './mapping-predecessor.mjs';
import {nativeFields} from './nato-starter.mjs';
const sha=b=>createHash('sha256').update(b).digest('hex');
const png=await readFile('fixtures/authored-media/square.png'),wav=await readFile('fixtures/authored-media/tone.wav');
const mapping={title:'Synthetic authored media proof',direction:'forward',pairs:[{key:'A',value:'Alfa'},{key:'B',value:'Bravo'}]};
const upload=(bytes,mime,name,alt,transcript)=>({unit_index:0,side:'answer',file_name:name,alt,transcript,source:'Repository-authored synthetic fixture',rights:'public_domain',license:'CC0 fixture',mime,sha256:sha(bytes),bytes_base64:bytes.toString('base64')});
const data={mapping,attachments:[upload(png,'image/png','answer-alfa.png','Answer image: Alfa',''),upload(wav,'audio/wav','answer-alfa.wav','','Answer transcript: Alfa; synthetic tone, not speech')]};
const out=process.env.SCRY_MEDIA_PROOF||'target/authored-media';await mkdir(out,{recursive:true});
const transcript=[];
const test=(name,run)=>nodeTest(name,async()=>{await run();transcript.push({check:name,status:'pass'});});
const predecessor=process.env.SCRY_MEDIA_BASELINE_ONLY==='1'?null:process.env.SCRY_MEDIA_PREDECESSOR_ARTIFACT?{artifact:resolve(process.env.SCRY_MEDIA_PREDECESSOR_ARTIFACT),close:async()=>{}}:await prepareMappingPredecessor();
async function fixture(){
 const temp=await mkdtemp(join(tmpdir(),'scry-authored-media-')),name='media-'+randomUUID(),space=name,config=join(temp,'login.json');
 const at=(Math.floor(Date.now()/86400000)+1)*86400000;
 const base={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:space,INTERNAL_KEY:internalKey,SCRY_TEST_NOW_MS:String(at)};
 let runtime,storage,origin,secret='',egress=0;
 const start=async()=>{runtime=await createRuntime({port:0,name,inspect:true,testClock:true,testR2Faults:true,resourcePersistencePath:join(temp,'state'),bindings:base,egress:async()=>{egress++;throw Error('Media fixture forbids external egress');}});origin=(await runtime.mf.ready).origin;runtime={...runtime,...await runtime.rebind({...base,CANONICAL_ORIGIN:origin})};storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});};
 const cli=(args,input='',login=config)=>new Promise((done,reject)=>{const child=spawn(resolve(process.env.SCRY_STUDY_CLI||'target/debug/scry'),[...args,'--json','--config',login],{env:{PATH:process.env.PATH,HOME:temp},stdio:['pipe','pipe','pipe']});let stdout='',stderr='';child.stdout.on('data',s=>stdout+=s);child.stderr.on('data',s=>stderr+=s);child.on('error',reject);child.on('close',code=>{try{if(secret)assert.ok(!stdout.includes(secret)&&!stderr.includes(secret));done({code,value:JSON.parse(stdout)});}catch(e){reject(e);}});child.stdin.end(input);});
 const ok=async(args,input='',login=config)=>{const r=await cli(args,input,login);assert.equal(r.code,0,JSON.stringify(r.value));return r.value;};
 try{
  await start();
  const html=await(await runtime.worker.fetch(origin+'/settings')).text(),form=/<form[^>]*action="\/tokens\/create"[^>]*>([\s\S]*?)<\/form>/.exec(html);assert.ok(form);
  const fields=Object.fromEntries([...form[1].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
  const minted=await runtime.worker.fetch(origin+'/tokens/create',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,name:'Synthetic media proof; unpaid'})});assert.equal(minted.status,200);
  secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())?.[1];assert.ok(secret);await ok(['login','--url',origin,'--token-stdin'],secret+'\n');
  const api=(path,payload)=>runtime.worker.fetch(origin+path,{...(payload?{method:'POST',body:JSON.stringify(payload)}:{}),headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'}});
  const rows=()=>storage.exec('SELECT key,value FROM records ORDER BY key');
  const bucket=()=>runtime.mf.getR2Bucket('SCRY_ASSETS');
  const objects=async()=>{const b=await bucket(),o=await b.list();return Promise.all(o.objects.map(async a=>[a.key,sha(Buffer.from(await(await b.get(a.key)).arrayBuffer()))]));};
  const file=join(temp,'manifest.json');await writeFile(file,JSON.stringify(data));
  return {get runtime(){return runtime;},get origin(){return origin;},get egress(){return egress;},space,at,base,temp,file,config,api,rows,bucket,objects,cli,ok,
   fetch:(path,init)=>runtime.worker.fetch(origin+path,init),
   faults:async values=>{runtime={...runtime,...await runtime.rebind({...base,CANONICAL_ORIGIN:origin,...values})};storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});},
   restart:async()=>{await runtime.mf.dispose();await start();await ok(['login','--url',origin,'--token-stdin'],secret+'\n');},
   restore:async(bytes,artifact=process.env.SCRY_WORKER_ARTIFACT||'build')=>{
    const name='media-restore-'+randomUUID(),token='synthetic-media-restore-capability-32chars';
    const bindings={...base,SPACE_NAME:name,SCRY_ENV:'restore',RESTORE_TOKEN:token};let target=await createRuntime({port:0,name,artifact,inspect:true,testClock:true,bindings});
    try{const url=(await target.mf.ready).origin;target={...target,...await target.rebind({...bindings,CANONICAL_ORIGIN:url})};
     const restore=()=>target.worker.fetch(url+'/operator/restore',{method:'POST',headers:{'x-scry-restore-token':token,'x-scry-sha256':sha(bytes)},body:bytes});
     const response=await restore();const status=response.status;
     const sql=await target.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});const rows=()=>sql.exec('SELECT key,value FROM records ORDER BY key');
     if(status===200){const saved=await rows();assert.equal((await restore()).status,409);assert.deepEqual(await rows(),saved);target={...target,...await target.rebind({...bindings,SCRY_ENV:'development',CANONICAL_ORIGIN:url})};const login=join(temp,'restore-login.json');await ok(['login','--url',url,'--token-stdin'],secret+'\n',login);const restored=await ok(['export'],'',login);const source=JSON.parse(bytes);assert.deepEqual(restored.app.library,source.app.library);assert.deepEqual(restored.app.practice,source.app.practice);assert.deepEqual(restored.media,source.media);const b=await target.mf.getR2Bucket('SCRY_ASSETS');for(const media of source.media||[]){const got=await b.get(`media/${name}/${media.asset.sha256}`);assert.ok(got);assert.equal(sha(Buffer.from(await got.arrayBuffer())),media.asset.sha256);}}
     else assert.deepEqual(await rows(),[],'Refused restore must keep target unused');
     return status;
    }finally{await target.mf.dispose();}
   },
   close:async()=>{await runtime.mf.dispose();await rm(temp,{recursive:true,force:true});}
  };
 }catch(e){await runtime?.mf.dispose();await rm(temp,{recursive:true,force:true});throw e;}
}
const f=await fixture();let vid,prompt,sid,archive,mediaRows,mediaArchive;
const mediaFreeArchive=Buffer.from(await(await f.api('/api/v1/export')).arrayBuffer());
const mediaFreeRows=await f.rows();
try{
 if(process.env.SCRY_MEDIA_BASELINE_ONLY!=='1')await test('media: failed R2 verification leaves no partial records; identical retry reuses bounded orphan bytes',async()=>{
  const before=await f.rows();const failingHash=[sha(png),sha(wav)].sort().at(-1);
  await f.faults({SCRY_TEST_R2_MISSING_HASH:failingHash});
  const r=await f.api('/api/v1/library/media',{operation_id:'media-fixture-admission',data});assert.ok(r.status>=400);
  assert.deepEqual(await f.rows(),before);const orphanCount=(await f.objects()).length;assert.ok(orphanCount>0&&orphanCount<=2,'The fault must occur after a real bounded R2 put');transcript.push({observation:'Failed R2 readback after real put leaves no committed Library rows',orphan_objects:orphanCount});
  await f.faults({});
 });
 await test('media: actual endpoint admits all validated bytes and immutable references',async()=>{
  const r=await f.api('/api/v1/library/media',{operation_id:'media-fixture-admission',data});
  assert.equal(r.status,200,'Baseline RED: the pre-media Worker refuses this route; the candidate must admit it.');
  vid=(await r.json()).result.entity_id;const shown=(await f.ok(['version','show',vid])).version;
  assert.equal(shown.text,'A\tAlfa\nB\tBravo\n');assert.equal(shown.media.length,2);assert.equal(shown.media_hash.length,64);
  assert.equal((await f.objects()).length,2);archive=Buffer.from(await(await f.api('/api/v1/export')).arrayBuffer());assert.equal(JSON.parse(archive).media.length,2);
  mediaArchive=archive;mediaRows=await f.rows();
  transcript.push({observation:'Atomic native API media admission',version:vid,archive_sha256:sha(archive)});
 });
 if(process.env.SCRY_MEDIA_BASELINE_ONLY==='1'){
  // This intentionally fails at the same first assertion on the frozen baseline.
 }else{
 await test('media: compiled CLI dedupe/replay/conflict and invalid data keep exact content',async()=>{
  const before=(await f.ok(['export'])).app.library;
  const r=await f.ok(['media','add','--file',f.file,'--operation-id','media-cli-import-operation']);assert.equal(r.result.entity_id,vid);
  const again=await f.ok(['media','add','--file',f.file,'--operation-id','media-cli-import-operation']);assert.equal(again.replayed,true);assert.deepEqual((await f.ok(['export'])).app.library,before);
  for(const corrupt of [(()=>{const d=structuredClone(data);d.attachments[1].sha256='0'.repeat(64);return d;})(),(()=>{const d=structuredClone(data);d.attachments[1].url='https://example.invalid/audio';return d;})()]){
   const rows=await f.rows(),objects=await f.objects();const r=await f.api('/api/v1/library/media',{operation_id:randomUUID(),data:corrupt});assert.equal(r.status,422);assert.deepEqual(await f.rows(),rows);assert.deepEqual(await f.objects(),objects);
  }
  const changed=structuredClone(data);changed.attachments[0].alt='Different metadata';const beforeRows=await f.rows();const r2=await f.objects();
  assert.equal((await f.api('/api/v1/library/media',{operation_id:'media-fixture-admission',data:changed})).status,409);assert.deepEqual(await f.rows(),beforeRows);assert.deepEqual(await f.objects(),r2);
  assert.equal((await f.api('/api/v1/library/media',{operation_id:randomUUID(),data:changed})).status,409);
  const over=await f.api('/api/v1/library/media',{operation_id:randomUUID(),data,padding:'x'.repeat(4*1024*1024)});assert.equal(over.status,413);assert.deepEqual(await f.rows(),beforeRows);assert.deepEqual(await f.objects(),r2);
 });
 await test('media: compiled CLI explicitly supersedes media without altering an existing Study or Attempt',async()=>{
  const old=vid,study=(await f.ok(['learn',old])).practice.study.id;
  const p=(await f.ok(['next'])).practice.prompt;await f.ok(['answer',p.id,'--text-stdin'],'Alfa');
  await f.ok(['study','drop',study]);
  const before=(await f.ok(['export'])).app,shown=await f.ok(['version','show',old]);
  const changed=structuredClone(data);changed.attachments[0].source='Explicitly superseded synthetic source';const file=join(f.temp,'supersede.json');await writeFile(file,JSON.stringify(changed));
  vid=(await f.ok(['media','supersede',old,'--revision',String(shown.work.revision),'--file',file,'--operation-id','media-cli-explicit-supersession'])).result.entity_id;
  assert.notEqual(vid,old);const after=(await f.ok(['export'])).app;assert.deepEqual(after.practice,before.practice);assert.deepEqual(after.library.versions[old],before.library.versions[old]);
  assert.equal(after.library.versions[vid].normalized_hash,after.library.versions[old].normalized_hash);assert.notEqual(after.library.versions[vid].media_hash,after.library.versions[old].media_hash);
 });
 await test('media: bundled NATO starter upgrades explicitly and preserves native history and retirement',async()=>{
  const starter=await fixture();
  try{
   const post=(path,fields)=>starter.fetch(path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':starter.origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(fields)});
   const added=await post('/library/mapping/add',nativeFields(await(await starter.fetch('/library')).text(),'/library/mapping/add'));assert.equal(added.status,303);
   const old=added.headers.get('location').split('/').at(-1);await starter.ok(['learn',old]);const p=(await starter.ok(['next'])).practice.prompt;
   const graded=await starter.ok(['answer',p.id,'--text-stdin'],'Alfa');assert.equal(graded.practice.attempt.grade.verdict,'Good');
   const before=(await starter.ok(['export'])).app,original=before.library.versions[old];assert.equal(original.media,undefined);
   const changed=structuredClone(data);changed.mapping=JSON.parse(await readFile('examples/nato-alphabet.json','utf8'));
   const file=join(starter.temp,'nato-media.json');await writeFile(file,JSON.stringify(changed));
   const next=(await starter.ok(['media','supersede',old,'--revision',String(before.library.works[original.work_id].revision),'--file',file,'--operation-id','media-starter-supersession'])).result.entity_id;
   const after=(await starter.ok(['export'])).app;assert.notEqual(next,old);assert.deepEqual(after.practice,before.practice);assert.deepEqual(after.library.versions[old],original);
   assert.equal(after.library.versions[next].text,original.text);assert.equal(after.library.versions[next].normalized_hash,original.normalized_hash);assert.equal(after.library.versions[next].media.length,2);
   const again=await post('/library/mapping/add',nativeFields(await(await starter.fetch('/library')).text(),'/library/mapping/add'));assert.equal(again.headers.get('location'),`/library/versions/${old}`);
   const repeated=(await starter.ok(['export'])).app;assert.deepEqual(repeated.library,after.library);assert.deepEqual(repeated.practice,after.practice);
   const rows=await starter.rows();assert.equal((await starter.api('/api/v1/correct',{operation_id:randomUUID(),attempt_id:graded.practice.attempt.id,grade:'Good'})).status,410);assert.deepEqual(await starter.rows(),rows);
   assert.equal(starter.egress,0);
  }finally{await starter.close();}
 });
 await test('media: complete private archive, current restore and predecessor format refusal',async()=>{
  archive=Buffer.from(await(await f.api('/api/v1/export')).arrayBuffer());assert.equal(await f.restore(archive),200);
  for(const broken of [(()=>{const a=JSON.parse(archive);a.media.pop();return a;})(),(()=>{const a=JSON.parse(archive);a.media[0].bytes='AAAA';return a;})()])assert.equal(await f.restore(Buffer.from(JSON.stringify(broken))),422);
  assert.equal(await f.restore(mediaFreeArchive,predecessor.artifact),200);assert.equal(await f.restore(mediaArchive,predecessor.artifact),422);
  transcript.push({observation:'Actual predecessor accepts media-free archive and refuses populated-media archive',wasm_sha256:sha(await readFile(join(predecessor.artifact,'index_bg.wasm')))});
  {
   const name='media-old-reader-'+randomUUID(),bindings={...f.base,SPACE_NAME:name};
   let old=await createRuntime({port:0,name,artifact:predecessor.artifact,inspect:true,testClock:true,bindings});
   try{
    const origin=(await old.mf.ready).origin;old={...old,...await old.rebind({...bindings,CANONICAL_ORIGIN:origin})};
    const sql=await old.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
    await sql.exec('CREATE TABLE records (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID');
    for(const row of mediaFreeRows)await sql.exec('INSERT INTO records(key,value) VALUES (?,?)',row.key,row.value);
    assert.equal((await old.worker.fetch(origin+'/readyz')).status,200,'Predecessor positive-control rows are readable');
    await sql.exec('DELETE FROM records');const populated=mediaRows;for(const row of populated)await sql.exec('INSERT INTO records(key,value) VALUES (?,?)',row.key,row.value);
    assert.ok((await old.worker.fetch(origin+'/readyz')).status>=400,'Pre-media SQL reader must refuse explicit populated-media marker');
    assert.deepEqual(await sql.exec('SELECT key,value FROM records ORDER BY key'),populated);
    transcript.push({observation:'Actual predecessor accepts media-free rows and refuses populated media rows without discarding them'});
   }finally{await old.mf.dispose();}
  }
  await f.restart();assert.deepEqual((await f.ok(['export'])).media,JSON.parse(archive).media);
 });
 await test('media: answer metadata and direct bytes are absent before durable reveal',async()=>{
  sid=(await f.ok(['learn',vid])).practice.study.id;prompt=(await f.ok(['next'])).practice.prompt;
  const safe=(await f.ok(['next'])).practice;assert.ok(!('media'in safe));
  const html=await(await f.fetch('/recall')).text();for(const secret of ['answer-alfa','Answer image: Alfa','Answer transcript','CC0 fixture','/media/'])assert.ok(!html.includes(secret),secret);
  for(const p of [`/api/v1/media/${vid}/0`,`/api/v1/media/${vid}/1`])assert.equal((await f.api(p)).status,409);
  assert.equal((await f.fetch(`/media/${vid}/0`)).status,409);
  const denied=await f.fetch(`/api/v1/media/${vid}/0`,{headers:{Authorization:'Bearer not-an-owner-token'}});assert.equal(denied.status,401);
  const reveal=await f.ok(['reveal',prompt.id,'--operation-id','media-fixture-reveal']);assert.equal(reveal.practice.media.length,2);
 });
 await test('media: private image/audio delivery verifies bytes and creates no review evidence',async()=>{
  const rows=await f.rows();
  for(const [index,bytes]of [[0,png],[1,wav]]){
   const r=await f.api(`/api/v1/media/${vid}/${index}`);assert.equal(r.status,200);assert.match(r.headers.get('Cache-Control'),/no-store/);assert.equal(r.headers.get('X-Content-Type-Options'),'nosniff');assert.equal(r.headers.get('X-Scry-Sha256'),sha(bytes));assert.deepEqual(Buffer.from(await r.arrayBuffer()),bytes);
  }
  assert.deepEqual(await f.rows(),rows);
  const b=await f.bucket(),key=`media/${f.space}/${sha(png)}`;await b.put(key,Buffer.from('corrupt fixture'));
  assert.equal((await f.api(`/api/v1/media/${vid}/0`)).status,503);assert.ok((await f.api('/api/v1/export')).status>=400);assert.deepEqual(await f.rows(),rows);
  await b.put(key,png);await b.delete(key);assert.equal((await f.api(`/api/v1/media/${vid}/0`)).status,503);assert.deepEqual(await f.rows(),rows);await b.put(key,png);
 });
 await test('media: native phone image/audio controls, accessible text and no autoplay',async()=>{
  const practiceBefore=(await f.ok(['export'])).app.practice;
  const require=createRequire(import.meta.url),playwright=require(process.env.SCRY_CRITICS_PLAYWRIGHT_PATH||'playwright');
  const browser=await playwright.chromium.launch({headless:true,...(process.env.SCRY_CRITICS_CHROMIUM_PATH?{executablePath:process.env.SCRY_CRITICS_CHROMIUM_PATH}:{}),args:['--no-sandbox']});
  try{for(const colorScheme of ['light','dark']){
   const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,colorScheme});const page=await context.newPage();await page.goto(f.origin+'/recall');
   const image=page.getByAltText('Answer image: Alfa');await image.waitFor();assert.equal(await image.evaluate(e=>e.naturalWidth),8);
   const audio=page.locator('audio');assert.equal(await audio.getAttribute('preload'),'none');assert.equal(await audio.getAttribute('autoplay'),null);assert.equal(await audio.evaluate(e=>e.paused),true);
   await page.getByText('Transcript',{exact:true}).click();await page.getByText('Answer transcript: Alfa; synthetic tone, not speech',{exact:true}).waitFor();
   await audio.click({position:{x:20,y:20}});await page.waitForFunction(()=>document.querySelector('audio').currentTime>0);
   assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await page.screenshot({path:join(out,`media-${colorScheme}-390.png`),fullPage:true});
   await context.close();
  }}finally{await browser.close();}
  assert.deepEqual((await f.ok(['export'])).app.practice,practiceBefore,'Native image/audio rendering and playback must create no learning evidence.');
  const answer=await f.ok(['answer',prompt.id,'--text-stdin'],'Alfa');assert.equal(answer.practice.attempt.grade.verdict,'Good');assert.equal(answer.practice.attempt.assisted,true);assert.ok(answer.practice.attempt.effects.every(e=>!e.scheduled));
  const nextBrowser=await playwright.chromium.launch({headless:true,...(process.env.SCRY_CRITICS_CHROMIUM_PATH?{executablePath:process.env.SCRY_CRITICS_CHROMIUM_PATH}:{}),args:['--no-sandbox']});
  try{
   const page=await nextBrowser.newPage({viewport:{width:390,height:844}});await page.goto(f.origin+'/recall');
   const oldAudio=await page.locator('audio').elementHandle();await page.locator('audio').click({position:{x:20,y:20}});await page.waitForFunction(()=>document.querySelector('audio').currentTime>0);
   await f.faults({SCRY_TEST_R2_DELAY_MS:'1500'});
   const pending=f.api(`/api/v1/media/${vid}/0`);
   await new Promise(done=>setTimeout(done,150));
   await page.getByRole('button',{name:'Next',exact:true}).click();await page.locator('[data-state="exact-prompt"]').waitFor();
   assert.equal(await oldAudio.evaluate(e=>e.paused),true);assert.equal(await oldAudio.getAttribute('src'),null,'Next discards source and stops detached audio');
   assert.equal((await pending).status,409,'An in-flight answer asset must be withheld after Next protects a new Prompt.');
  }finally{await nextBrowser.close();await f.faults({});}
  assert.equal((await f.api(`/api/v1/media/${vid}/0`)).status,409);assert.equal((await f.fetch(`/media/${vid}/1`)).status,409);
 });
 await test('media: revoked token during awaited private read receives no bytes',async()=>{
  await f.ok(['reveal',(await f.ok(['next'])).practice.prompt.id]);
  const response=await f.api('/api/v1/tokens',{operation_id:randomUUID(),name:'Synthetic delayed media reader'});assert.equal(response.status,200);const child=await response.json();
  await f.faults({SCRY_TEST_R2_DELAY_MS:'1500'});
  try{
   const pending=f.fetch(`/api/v1/media/${vid}/0`,{headers:{Authorization:'Bearer '+child.secret}});
   await new Promise(done=>setTimeout(done,150));
   assert.equal((await f.api('/api/v1/tokens/revoke',{operation_id:randomUUID(),token_id:child.token.id})).status,200);
   const denied=await pending;assert.equal(denied.status,401);assert.ok(!Buffer.from(await denied.arrayBuffer()).equals(png));
  }finally{await f.faults({});}
 });
 await test('media: complete budget refusal precedes every R2 and row mutation',async()=>{
  const bounded=await fixture();
  try{
   const large=(marker)=>{const bytes=Buffer.alloc(1024*1024,marker);wav.copy(bytes,0,0,44);bytes.writeUInt32LE(bytes.length-8,4);bytes.writeUInt32LE(48000,24);bytes.writeUInt32LE(96000,28);bytes.writeUInt32LE(bytes.length-44,40);return bytes;};
   for(let batch=0;batch<6;batch++){
    const uploads=[0,1].map(unit=>({...upload(large(batch*2+unit),'audio/wav',`budget-${batch}-${unit}.wav`,'','Synthetic budget tone'),unit_index:unit}));
    const data={mapping:{title:`Synthetic budget ${batch}`,direction:'forward',pairs:[{key:`A${batch}`,value:`Alfa${batch}`},{key:`B${batch}`,value:`Bravo${batch}`}]},attachments:uploads};
    const before=await bounded.rows(),objects=await bounded.objects();
    const response=await bounded.api('/api/v1/library/media',{operation_id:randomUUID(),data});
    if(batch<5)assert.equal(response.status,200);
    else{assert.equal(response.status,413);assert.deepEqual(await bounded.rows(),before);assert.deepEqual(await bounded.objects(),objects);assert.equal(objects.length,10);}
   }
   const exported=Buffer.from(await(await bounded.api('/api/v1/export')).arrayBuffer());assert.ok(exported.length<16*1024*1024);assert.equal(JSON.parse(exported).media.length,10);assert.equal(bounded.egress,0);
  }finally{await bounded.close();}
 });
 await test('media: no provider, paid reservation or synthetic playback success',async()=>{
  assert.equal(f.egress,0);await f.ok(['reveal',(await f.ok(['next'])).practice.prompt.id]);const app=(await f.ok(['export'])).app;
  assert.deepEqual(app.jobs,{});assert.deepEqual(app.assessments,{});assert.deepEqual(app.spend,[]);assert.ok(Object.values(app.practice.attempts).every(a=>a.grade.authority.name==='exact-v2'));
 });
 }
}finally{await writeFile(join(out,'runtime.json'),JSON.stringify(transcript,null,2)+'\n');await f.close();await predecessor?.close();}
