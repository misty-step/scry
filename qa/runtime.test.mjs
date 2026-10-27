#!/usr/bin/env node
// Exercises the exact built Rust/WASM in real workerd with isolated SQLite/R2.
// Egress uses synthetic RSA issuer keys and explicitly controlled model
// responses. No account, owner credential, real model, or remote bucket is used.
import assert from 'node:assert/strict';
import { generateKeyPairSync, sign, createHash, randomUUID } from 'node:crypto';
import test from 'node:test';
import {createRuntime,createSyntheticRuntime,Response,internalKey} from './runtime-helper.mjs';
import {createServer} from 'node:net';
import {historicalMutation} from './historical-fixture.mjs';
import { resolve } from 'node:path';
import { readFileSync } from 'node:fs';

// Retired HTTP operations are never used to populate new fixtures. Read native
// Settings transport fields for active writes and explicit refusal assertions.
async function savedRequestFields(response) {
  assert.equal(response.status,200);
  const html=await response.text();
  const fields=Object.fromEntries([...html.matchAll(/name="(csrf|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
  assert.ok(fields.csrf);assert.ok(fields.revision);
  return {...fields,operation_id:`saved-request-${randomUUID()}`};
}

// Validated lower-level historical setup only. Native engine transitions and
// archive capacity are checked before writing this test-owned SQLite/R2 space.
async function historicalCapture(fixture,name,space,values,photoBytes) {
  const sql=await fixture.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
  const operation=values.operation_id||`historical-${randomUUID()}`;
  const fields={...values,operation_id:operation};
  let photo;
  if(photoBytes){
    photo={key:`photos/${space}/${operation}-${createHash('sha256').update(photoBytes).digest('hex')}`,mime:'image/png',size:photoBytes.length,sha256:createHash('sha256').update(photoBytes).digest('hex')};
    fields.__photo=JSON.stringify(photo);
  }
  const at=Date.now();
  const captured=await historicalMutation(sql,{path:'/create',fields,now_ms:at,seed:`${space}:${operation}`});
  if(photo){const bucket=await fixture.mf.getR2Bucket('SCRY_ASSETS');await bucket.put(photo.key,photoBytes);assert.deepEqual(Buffer.from(await(await bucket.get(photo.key)).arrayBuffer()),photoBytes);}
  await historicalMutation(sql,{actions:[{kind:'claim',now_ms:at+1,lease:`lease-${operation}`,generation:false,jev:false,critic:false}],now_ms:at+1,seed:`${space}:${operation}`});
  return captured.result;
}

const artifact = resolve(process.env.SCRY_WORKER_ARTIFACT || 'build');
const origin = 'https://scry-synthetic.invalid';
const team = 'synthetic.cloudflareaccess.com';
const owner = 'synthetic-owner-only';
const audience = 'synthetic-audience-only';
const operatorKey = 'synthetic-operator-only-key-at-least-32chars';
const restoreToken = 'synthetic-restore-only-key-at-least-32chars';
const backupKey = 'synthetic-backup-trigger-only-key-at-least-32chars';
const { publicKey, privateKey } = generateKeyPairSync('rsa', { modulusLength: 2048 });
const publicJwk = { ...publicKey.export({format:'jwk'}), kid:'synthetic-key', alg:'RS256', use:'sig' };
const epoch = Math.floor(Date.now()/1000);
const claims = { iss:`https://${team}`, sub:owner, aud:[audience], iat:epoch, nbf:epoch-1, exp:epoch+600 };
const b64 = value => Buffer.from(JSON.stringify(value)).toString('base64url');
function jwt(changes={}, headerChanges={}, key=privateKey) {
  const encoded = `${b64({alg:'RS256',kid:'synthetic-key',typ:'JWT',...headerChanges})}.${b64({...claims,...changes})}`;
  return `${encoded}.${sign('RSA-SHA256',Buffer.from(encoded),key).toString('base64url')}`;
}
let certRequests=0;
const egress = async request => {
  assert.equal(request.url, `https://${team}/cdn-cgi/access/certs`, 'Only synthetic public-key fetches may leave the runtime.');
  certRequests++;
  return new Response(JSON.stringify({keys:[publicJwk]}),{status:200,headers:{'Content-Type':'application/json'}});
};
const baseBindings = {SCRY_ENV:'preview',CANONICAL_ORIGIN:origin,SPACE_NAME:'synthetic-only',ACCESS_TEAM:team,ACCESS_AUDIENCE:audience,OWNER_SUBJECT:owner,INTERNAL_KEY:internalKey,OPERATOR_KEY:operatorKey,BACKUP_TRIGGER_KEY:backupKey,ALTERNATE_HOSTS:'alternate-synthetic.invalid'};
let checks=0,failures=0;
const check = (label,fn)=>test(label,async()=>{try{await fn();checks++;}catch(error){failures++;throw error;}});
let {mf:preview,worker}=await createRuntime({bindings:baseBindings,name:'scry-auth-smoke',artifact,egress,inspect:true});
let restored;
try {
  const request = (path='/', token=jwt(), init={}) => {const headers={'Cf-Access-Jwt-Assertion':token,...init.headers};if(headers.Origin){headers['X-Scry-Test-Origin']=headers.Origin;delete headers.Origin;}return worker.fetch(`${origin}${path}`,{...init,redirect:'manual',headers});};
  await check('health fails closed before the first snapshot without initializing learner state',async()=>{
    const response=await request('/healthz','');
    assert.equal(response.status,503);
    assert.equal(await response.text(),'backup_stale');
    assert.match(response.headers.get('Cache-Control'),/no-store/);
    const storage=await preview.unsafeGetDurableObjectStorage('scry-auth-smoke','LearningSpace',{name:baseBindings.SPACE_NAME});
    assert.deepEqual(await storage.exec('SELECT key FROM records'),[]);
  });
  await check('real RS256 owner signature and private response headers',async()=>{
    const response=await request();assert.equal(response.status,200);assert.match(await response.text(),/<h1>Make it memorable\.<\/h1>/);assert.match(response.headers.get('Cache-Control'),/no-store/);assert.match(response.headers.get('Content-Security-Policy'),/frame-ancestors 'none'/);assert.equal(response.headers.get('Referrer-Policy'),'same-origin');
  });
  await check('empty initialization starts at revision one',async()=>{
    const storage=await preview.unsafeGetDurableObjectStorage('scry-auth-smoke','LearningSpace',{name:baseBindings.SPACE_NAME});
    const meta=JSON.parse((await storage.exec("SELECT value FROM records WHERE key='meta'"))[0].value);
    assert.equal(meta.revision,1);
  });
  await check('private build-bound changelog is read-only, with health version and revision',async()=>{
    assert.equal((await request('/changelog','')).status,401);
    assert.equal((await request('/changelog',jwt({sub:'wrong'}))).status,401);
    const storage=await preview.unsafeGetDurableObjectStorage('scry-auth-smoke','LearningSpace',{name:baseBindings.SPACE_NAME});
    const rows=await storage.exec('SELECT key,value FROM records ORDER BY key');
    const response=await request('/changelog');
    assert.equal(response.status,200);
    assert.match(response.headers.get('Cache-Control'),/no-store/);
    const html=await response.text();
    assert.match(html,/<h1>Changelog<\/h1>/);
    const version=/version = "([^"]+)"/.exec(readFileSync('Cargo.toml','utf8'))[1];
    assert.ok(html.includes('Version ' + version));
    assert.ok(html.includes(process.env.SCRY_SOURCE_REVISION || 'unverified-development-build'));
    assert.deepEqual(await storage.exec('SELECT key,value FROM records ORDER BY key'),rows);
    const health=await request('/healthz','');
    assert.equal(health.headers.get('X-Scry-Version'),version);
    assert.equal(health.headers.get('X-Scry-Revision'),process.env.SCRY_SOURCE_REVISION || 'unverified-development-build');
    assert.equal((await request('/changelog',jwt(),{method:'POST',headers:{Origin:origin}})).status,405);
  });
  await check('cached issuer keys still require every token signature',async()=>{
    const token=jwt();const [h,p,s]=token.split('.');const forged=`${h}.${b64({...claims,sub:'attacker'})}.${s}`;assert.equal((await request('/',forged)).status,401);assert.equal((await request()).status,200);assert.equal(certRequests,1);
  });
  await check('forged signature with valid owner claims is denied',async()=>{
    const wrong=generateKeyPairSync('rsa',{modulusLength:2048}).privateKey;assert.equal((await request('/',jwt({}, {}, wrong))).status,401);
  });
  await check('anonymous, algorithm, issuer, audience, subject, expiry and not-before fences',async()=>{
    for(const token of ['',jwt({}, {alg:'HS256'}),jwt({iss:'https://attacker.invalid'}),jwt({aud:['wrong']}),jwt({sub:'wrong'}),jwt({exp:epoch-1}),jwt({nbf:epoch+600}),jwt({exp:'tomorrow'})])assert.equal((await request('/',token)).status,401);
  });
  await check('forged identity headers cannot replace owner authorization',async()=>{
    assert.equal((await request('/','',{headers:{'x-scry-owner':'private-owner','x-scry-internal-key':internalKey,'Cf-Access-Authenticated-User-Email':'owner@example.invalid'}})).status,401);
    const namespace=await preview.getDurableObjectNamespace('LEARNING_SPACE');const stub=namespace.get(namespace.idFromName('synthetic-only'));assert.equal((await stub.fetch(origin)).status,403);
  });
  await check('canonical Host and alternate read-only redirect',async()=>{
    assert.equal((await request('/',jwt(),{headers:{'X-Scry-Test-Host':'wrong.invalid'}})).status,421);
    const read=await worker.fetch('https://alternate-synthetic.invalid/create?text=private',{redirect:'manual'});assert.equal(read.status,303);assert.equal(read.headers.get('Location'),`${origin}/create?text=private`);
    assert.equal((await worker.fetch('https://alternate-synthetic.invalid/create',{method:'POST',body:'intent=test'})).status,421);
  });
  await check('unconfigured Add material is read-only and cannot be enabled through query fields',async()=>{
    for(const path of ['/create','/create?__preparation_ready=true&text=Unsent%20material']){
      const response=await request(path);assert.equal(response.status,200);const html=await response.text();
      assert.match(html,/Native content generation is retired/);
      assert.doesNotMatch(html,/<form[^>]*action="\/create"|type="file"/);
      if(path.includes('text='))assert.match(html,/<textarea[^>]*readonly[^>]*>Unsent material<\/textarea>/);
    }
  });
  const fields={...await savedRequestFields(await request('/settings')),intent:'Learn cache validation'};
  const post=(path,body,headers={})=>request(path,jwt(),{method:'POST',body:new URLSearchParams(body),headers:{Origin:origin,'Content-Type':'application/x-www-form-urlencoded',...headers}});
  await check('cross-site Origin, forged CSRF and body-length bounds',async()=>{
    assert.equal((await post('/create',fields,{Origin:'https://attacker.invalid'})).status,403);
    assert.equal((await post('/create',fields,{Origin:'null'})).status,403);
    assert.equal((await post('/create',fields,{'Sec-Fetch-Site':'cross-site'})).status,403);
    assert.equal((await post('/create',{...fields,csrf:'forged'})).status,403);
    assert.equal((await request('/create',jwt(),{method:'POST',body:'x'.repeat(5*1024*1024),headers:{Origin:origin,'Content-Type':'application/x-www-form-urlencoded'}})).status,413);
  });
  const fixture={mf:preview};
  const sql=await preview.unsafeGetDurableObjectStorage('scry-auth-smoke','LearningSpace',{name:baseBindings.SPACE_NAME});
  await check('retired generation refuses new and repeated operations without durable writes',async()=>{
    const before=await sql.exec('SELECT key,value FROM records ORDER BY key');
    for(const values of [fields,fields,{...fields,intent:'different'}]){
      const response=await post('/create',values);assert.equal(response.status,410);assert.match(await response.text(),/Native content generation is retired/);
      assert.deepEqual(await sql.exec('SELECT key,value FROM records ORDER BY key'),before);
    }
  });
  await check('retirement creates no implicit paid request or synthetic fallback',async()=>{
    await new Promise(resolve=>setTimeout(resolve,1300));
    const app=(await(await request('/export')).json()).app;
    assert.deepEqual(app.jobs,{});assert.deepEqual(app.goals,{});assert.deepEqual(app.spend,[]);assert.equal(certRequests,1);
  });
  await check('legacy draft is preserved read-only on a definite retirement refusal',async()=>{
    const current=await savedRequestFields(await request('/settings'));const text='x'.repeat(32769);const response=await post('/create',{...current,intent:text});assert.equal(response.status,410);const html=await response.text();assert.ok(html.includes(text));assert.match(html,/<textarea[^>]*readonly/);assert.doesNotMatch(html,/<form[^>]*action="\/create"|type="file"/);
  });
  const photoBytes=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=','base64');
  await check('retired photo capture refuses before assets; historical photos remain complete',async()=>{
    const current=await savedRequestFields(await request('/settings'));
    const before=await sql.exec('SELECT key,value FROM records ORDER BY key');
    const multipart=bytes=>{const form=new FormData();for(const [key,value]of Object.entries({...current,intent:'Understand the image'}))form.set(key,value);form.set('photo',new Blob([bytes],{type:'image/png'}),'synthetic.png');return form;};
    for(const bytes of [Buffer.from('<svg onload="alert(1)">'),photoBytes]){
      const encoded=new globalThis.Request(origin,{method:'POST',body:multipart(bytes)});
      const response=await request('/create',jwt(),{method:'POST',body:Buffer.from(await encoded.arrayBuffer()),headers:{Origin:origin,'Content-Type':encoded.headers.get('Content-Type')}});
      assert.equal(response.status,410);assert.match(await response.text(),/Understand the image/);
      assert.deepEqual(await sql.exec('SELECT key,value FROM records ORDER BY key'),before);
      assert.deepEqual((await(await preview.getR2Bucket('SCRY_ASSETS')).list()).objects,[]);
    }
    await historicalCapture(fixture,'scry-auth-smoke',baseBindings.SPACE_NAME,{...fields,operation_id:'historical-capture-text-only'});
    await historicalCapture(fixture,'scry-auth-smoke',baseBindings.SPACE_NAME,{...current,operation_id:'historical-capture-photo-only',intent:'Understand the image'},photoBytes);
    const exported=await(await request('/export')).json();assert.equal(Object.keys(exported.app.goals).length,2);assert.equal(exported.photos.length,1);assert.deepEqual(Buffer.from(exported.photos[0].bytes,'base64'),photoBytes);
  });
  await check('failed scheduled photo archive reaches the scheduler without hiding retained work', async () => {
    const assets = await preview.getR2Bucket('SCRY_ASSETS');
    const objects = await assets.list();
    assert.equal(objects.objects.length, 1);
    const key = objects.objects[0].key;
    const photo = await assets.get(key);
    const bytes = await photo.arrayBuffer();
    await assets.delete(key);
    try {
      const result = await worker.scheduled({ cron: '17 3 * * *' });
      assert.notEqual(result.outcome, 'ok');
    } finally { await assets.put(key, bytes); }
    const state = await (await request('/export')).json();
    assert.equal(Object.keys(state.app.goals).length, 2);
    assert.ok(state.app.backup.error);
    const settings = await (await request('/settings')).text();
    assert.match(settings, /complete snapshot could not be verified/);
  });
  let archiveBytes,hash;
  await check('complete R2 backup acknowledged only after exact checksum readback',async()=>{
    const result=await request('/operator/backup',jwt(),{method:'POST',headers:{Origin:origin,'x-scry-operator-key':operatorKey}});assert.equal(result.status,200);
    const bucket=await preview.getR2Bucket('SCRY_BACKUPS');const list=await bucket.list({prefix:'snapshots/synthetic-only/'});assert.ok(list.objects.length>=1);
    const object=await bucket.get(list.objects.at(-1).key);archiveBytes=Buffer.from(await object.arrayBuffer());hash=createHash('sha256').update(archiveBytes).digest('hex');const archive=JSON.parse(archiveBytes);assert.equal(Object.keys(archive.app.goals).length,2);assert.equal(archive.algorithm.startsWith('go-fsrs/v4.0.0'),true);
  });
  let recoverySends=0;
  await check('chief backup capability is route-scoped, fails safely and recovers stale health only after full readback',async()=>{
    // Earlier captures own one-second work alarms. Restore into a separate,
    // paused notebook so only this check can refresh its stale backup fixture.
    const name='scry-chief-backup-smoke';
    const fixture=await createRuntime({bindings:{SCRY_ENV:'restore',CANONICAL_ORIGIN:origin,SPACE_NAME:baseBindings.SPACE_NAME,INTERNAL_KEY:internalKey,RESTORE_TOKEN:restoreToken},name,artifact,egress,inspect:true});
    try {
      const restored=await fixture.worker.fetch(`${origin}/operator/restore`,{method:'POST',body:archiveBytes,
        headers:{'x-scry-restore-token':restoreToken,'x-scry-sha256':hash}});
      assert.equal(restored.status,200);
      const {worker}=await fixture.rebind(baseBindings);
      const request=(path,token=jwt(),init={})=>worker.fetch(`${origin}${path}`,{...init,redirect:'manual',
        headers:{'Cf-Access-Jwt-Assertion':token,...init.headers}});
      assert.equal((await(await request('/export')).json()).app.restored_paused,true);
      const trigger=(key=backupKey,init={})=>request('/internal/backup','',{method:'POST',...init,headers:{'x-scry-backup-key':key,...init.headers}});
      assert.equal((await trigger()).status,200);
      assert.equal((await request('/healthz','')).status,200);
      for(const key of ['', 'wrong',internalKey,operatorKey])assert.equal((await trigger(key)).status,403);
      assert.equal((await trigger(backupKey,{method:'GET'})).status,405);
      assert.equal((await trigger(backupKey,{headers:{'X-Scry-Test-Host':'wrong.invalid'}})).status,421);
      assert.equal((await worker.fetch('https://alternate-synthetic.invalid/internal/backup',{method:'POST',headers:{'x-scry-backup-key':backupKey}})).status,421);
      assert.equal((await request('/','',{headers:{'x-scry-backup-key':backupKey}})).status,401);
      assert.equal((await request('/__internal/daily','',{method:'POST',headers:{'x-scry-backup-key':backupKey,'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}})).status,404);
      const storage=await fixture.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:baseBindings.SPACE_NAME});
      const meta=JSON.parse((await storage.exec("SELECT value FROM records WHERE key='meta'"))[0].value);
      meta.backup.completed_ms=Date.now()-27*60*60*1000;
      await storage.exec("UPDATE records SET value=? WHERE key='meta'",JSON.stringify(meta));
      const stale=await request('/healthz','');assert.equal(stale.status,503);assert.equal(await stale.text(),'backup_stale');
      const before=await(await request('/export')).json();
      assert.equal(before.app.backup.completed_ms,meta.backup.completed_ms);
      const assets=await fixture.mf.getR2Bucket('SCRY_ASSETS');const key=(await assets.list()).objects[0].key;
      const photo=Buffer.from(await(await assets.get(key)).arrayBuffer());await assets.delete(key);
      try {
        const failed=await trigger();assert.equal(failed.status,503);assert.equal(await failed.text(),'backup_failed');
        assert.equal((await request('/healthz','')).status,503);
        const failedMeta=JSON.parse((await storage.exec("SELECT value FROM records WHERE key='meta'"))[0].value);
        assert.equal(failedMeta.backup.completed_ms,meta.backup.completed_ms,'An incomplete archive cannot refresh health.');
      } finally {await assets.put(key,photo);}
      const recovered=await trigger();assert.equal(recovered.status,200);assert.equal(await recovered.text(),'Snapshot verified.');
      assert.equal((await request('/healthz','')).status,200);
      const after=await(await request('/export')).json();
      assert.ok(after.app.backup.completed_ms>meta.backup.completed_ms);
      assert.deepEqual(after.app.goals,before.app.goals);assert.deepEqual(after.app.events,before.app.events);assert.deepEqual(after.photos,before.photos);
      const bucket=await fixture.mf.getR2Bucket('SCRY_BACKUPS');const bytes=Buffer.from(await(await bucket.get(after.app.backup.key)).arrayBuffer());
      assert.equal(createHash('sha256').update(bytes).digest('hex'),after.app.backup.sha256);
      const snapshot=JSON.parse(bytes);assert.deepEqual(snapshot.app.goals,before.app.goals);assert.deepEqual(snapshot.photos,before.photos);
      for(const changes of [{BACKUP_TRIGGER_KEY:''},{BACKUP_TRIGGER_KEY:'short'},{INTERNAL_KEY:''},{SPACE_NAME:'invalid/name'},{SCRY_ENV:'restore'},{SCRY_ENV:'unconfigured'}]){
        const fixture=await createRuntime({bindings:{...baseBindings,...changes},name:'scry-trigger-closed',artifact});
        try {assert.notEqual((await fixture.worker.fetch(`${origin}/internal/backup`,{method:'POST',headers:{'x-scry-backup-key':backupKey}})).status,200);assert.equal((await(await fixture.mf.getR2Bucket('SCRY_BACKUPS')).list()).objects.length,0);}
        finally {await fixture.mf.dispose();}
      }
    } finally {await fixture.mf.dispose();}
  });
  const recoveryEgress=async request=>{if(request.url===`https://${team}/cdn-cgi/access/certs`)return egress(request);assert.equal(request.url,'https://openrouter.ai/api/v1/chat/completions');recoverySends++;return new Response('Controlled malformed recovery response',{status:200});};
  const restoreRuntime=await createRuntime({bindings:{SCRY_ENV:'restore',CANONICAL_ORIGIN:origin,SPACE_NAME:'fresh-isolated-restore-only',INTERNAL_KEY:internalKey,RESTORE_TOKEN:restoreToken},name:'scry-restore-smoke',artifact,egress:recoveryEgress,inspect:true});
  restored=restoreRuntime.mf;
  const restoreWorker=restoreRuntime.worker;
  const restore=(body,checksum=hash,token=restoreToken)=>restoreWorker.fetch(`${origin}/operator/restore`,{method:'POST',headers:{'x-scry-restore-token':token,'x-scry-sha256':checksum},body});
  await check('restore authorization and archive checksum fail before writes',async()=>{
    assert.equal((await restore(archiveBytes,hash,'wrong')).status,403);assert.equal((await restore(archiveBytes,'0'.repeat(64))).status,422);
    const conflicting=JSON.parse(archiveBytes);const withoutPhoto=Object.values(conflicting.app.goals).find(goal=>!goal.photo);withoutPhoto.photo={...conflicting.photos[0].photo,sha256:'0'.repeat(64)};const inconsistent=Buffer.from(JSON.stringify(conflicting));assert.equal((await restore(inconsistent,createHash('sha256').update(inconsistent).digest('hex'))).status,422);assert.equal((await(await restored.getR2Bucket('SCRY_ASSETS')).list()).objects.length,0,'Conflicting photo metadata must fail before any recovery write.');
  });
  await check('restore revision exhaustion refuses before SQLite or photo writes',async()=>{
    const storage=await restored.unsafeGetDurableObjectStorage('scry-restore-smoke','LearningSpace',{name:'fresh-isolated-restore-only'});
    assert.ok(JSON.parse(archiveBytes).photos.length,'The overflow fixture must include a photo to detect early R2 writes.');
    for(const revision of ['18446744073709551615','18446744073709551616']){
      const source=JSON.parse(archiveBytes);source.app.revision='EXHAUSTED_REVISION';
      const bytes=Buffer.from(JSON.stringify(source).replace('"EXHAUSTED_REVISION"',revision));
      const response=await restore(bytes,createHash('sha256').update(bytes).digest('hex'));
      assert.equal(response.status,422,await response.text());
      assert.deepEqual(await storage.exec('SELECT key,value FROM records'),[]);
      assert.equal((await(await restored.getR2Bucket('SCRY_ASSETS')).list()).objects.length,0);
      assert.equal((await(await restored.getR2Bucket('SCRY_BACKUPS')).list()).objects.length,0);
    }
  });
  await check('independent complete restore is paused and refuses overwrite',async()=>{
    const response=await restore(archiveBytes);assert.equal(response.status,200);assert.match(await response.text(),/paused/);assert.equal((await restore(archiveBytes)).status,409);
    const ns=await restored.getDurableObjectNamespace('LEARNING_SPACE');const stub=ns.get(ns.idFromName('fresh-isolated-restore-only'));const readback=await stub.fetch(`${origin}/export`,{headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});assert.equal(readback.status,200);const recovered=await readback.json();assert.equal(recovered.app.restored_paused,true);assert.equal(recovered.app.preferences.pace,'steady');assert.equal(Object.keys(recovered.app.goals).length,2);assert.equal(recovered.photos.length,1);assert.deepEqual(Buffer.from(recovered.photos[0].bytes,"base64"),photoBytes);assert.notEqual(recovered.photos[0].photo.key,JSON.parse(archiveBytes).photos[0].photo.key);assert.deepEqual(recovered.app.events,JSON.parse(archiveBytes).app.events);assert.deepEqual(recovered.app.operations,JSON.parse(archiveBytes).app.operations);
  });
  await check('learner Settings resume refuses CSRF, replays safely and preserves historical work',async()=>{
    const activeBindings={...baseBindings,SPACE_NAME:'fresh-isolated-restore-only',OPENROUTER_API_KEY:'synthetic-recovery-generator-key',OPENROUTER_MODEL:'synthetic-recovery-model'};
    const active=await restoreRuntime.rebind(activeBindings);const req=(path,init={})=>active.worker.fetch(`${origin}${path}`,{...init,redirect:'manual',headers:{'Cf-Access-Jwt-Assertion':jwt(),...init.headers}});
    const ns=await restored.getDurableObjectNamespace('LEARNING_SPACE');const stub=ns.get(ns.idFromName(activeBindings.SPACE_NAME));const state=async()=>{const response=await stub.fetch(`${origin}/export`,{headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});return(await response.json()).app;};
    const before=await state();assert.equal(before.restored_paused,true);const oldJobs=structuredClone(before.jobs),oldAssessments=structuredClone(before.assessments);
    const fields=await savedRequestFields(await req('/settings'));const created=await req('/create',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,intent:'A new request cannot reactivate retired generation'})});assert.equal(created.status,410);await new Promise(r=>setTimeout(r,1200));assert.equal(recoverySends,0);assert.deepEqual(await state(),before);
    assert.equal((await req('/operator/resume-work',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'x-scry-operator-key':'wrong'}})).status,403);assert.equal((await req('/operator/resume-work',{method:'POST',headers:{'X-Scry-Test-Origin':'https://attacker.invalid','x-scry-operator-key':operatorKey}})).status,403);
    const settings=await(await req('/settings')).text();
    const resumeForm=/<form[^>]*action="\/settings\/resume"[^>]*>([\s\S]*?)<\/form>/.exec(settings);
    assert.ok(resumeForm,'The owner must have a real native resume form, not an operator instruction.');
    const resumeFields=Object.fromEntries([...resumeForm[1].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
    const resume=(values=resumeFields,originHeader=origin)=>req('/settings/resume',{method:'POST',headers:{'X-Scry-Test-Origin':originHeader,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(values)});
    for(const [values,originHeader,status] of [[resumeFields,'https://attacker.invalid',403],[{...resumeFields,csrf:'forged'},origin,403],[{...resumeFields,operation_id:''},origin,422]]){
      assert.equal((await resume(values,originHeader)).status,status);
      assert.equal((await state()).restored_paused,true);
      assert.equal(recoverySends,0);
    }
    const resumed=await resume();assert.equal(resumed.status,303);assert.equal(resumed.headers.get('Location'),'/notebook');
    assert.equal((await resume()).status,303,'Lost-response replay reconciles the same operation.');
    assert.equal((await resume({...resumeFields,operation_id:'second-distinct-resume-operation'})).status,409);
    assert.equal((await req('/operator/resume-work',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'x-scry-operator-key':operatorKey}})).status,409);
    await new Promise(r=>setTimeout(r,1300));const after=await state();assert.equal(recoverySends,0);assert.equal(after.restored_paused,false);for(const[id,job]of Object.entries(oldJobs))assert.deepEqual(after.jobs[id],job,'Historical jobs must never be requeued.');assert.deepEqual(after.assessments,oldAssessments);assert.deepEqual(after.spend,before.spend);assert.deepEqual(after.goals,before.goals);

  });
  await check('historical complete asset capacity stays bounded and retired capture creates no orphan',async()=>{
    const localOrigin='http://127.0.0.1:8789',name='scry-capacity-smoke',space='synthetic-capacity-only';
    const local=await createRuntime({bindings:{SCRY_ENV:'development',CANONICAL_ORIGIN:localOrigin,SPACE_NAME:space,INTERNAL_KEY:internalKey},name,artifact,inspect:true});
    const bytes=Buffer.concat([photoBytes,Buffer.alloc(4*1024*1024-photoBytes.length)]);
    try {
      for(let i=0;i<2;i++){
        const fields=await savedRequestFields(await local.worker.fetch(`${localOrigin}/settings`));
        await historicalCapture(local,name,space,{...fields,intent:`Historical capacity photo ${i}`},bytes);
      }
      const sql=await local.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
      const before=await sql.exec('SELECT key,value FROM records ORDER BY key');
      const fields=await savedRequestFields(await local.worker.fetch(`${localOrigin}/settings`));
      await assert.rejects(historicalCapture(local,name,space,{...fields,intent:'Historical capacity photo 2'},bytes),error=>error.status===413&&/archive exceeds/.test(error.message));
      assert.deepEqual(await sql.exec('SELECT key,value FROM records ORDER BY key'),before,'An oversized historical fixture cannot bypass current archive limits.');
      const form=new FormData();for(const[key,value]of Object.entries({...fields,intent:'Retired capture at capacity'}))form.set(key,value);form.set('photo',new Blob([bytes],{type:'image/png'}),'capacity.png');
      const encoded=new globalThis.Request(localOrigin,{method:'POST',body:form});const response=await local.worker.fetch(`${localOrigin}/create`,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':localOrigin,'Content-Type':encoded.headers.get('Content-Type')},body:Buffer.from(await encoded.arrayBuffer())});
      assert.equal(response.status,410);assert.deepEqual(await sql.exec('SELECT key,value FROM records ORDER BY key'),before);
      const assets=await local.mf.getR2Bucket('SCRY_ASSETS');assert.equal((await assets.list()).objects.length,2,'Neither refused historical setup nor retired HTTP capture may create an orphan asset.');
      const archive=await(await local.worker.fetch(`${localOrigin}/export`)).json();assert.equal(Object.keys(archive.app.goals).length,2);assert.equal(archive.photos.length,2);assert.equal(Object.keys(archive.app.operations).length,2);
    } finally {await local.mf.dispose();}
  });
  await check('allowed reference reads mark future exposure while checking stays unaided and protected',async()=>{
    const fixtureOrigin='http://127.0.0.1:8790',space='synthetic-exposure-only';let release,started;const sending=new Promise(resolve=>{started=resolve;});
    const provider=async request=>{if(request.url===`https://${team}/cdn-cgi/access/certs`)return egress(request);assert.equal(request.url,'https://openrouter.ai/api/alpha/decisions');started();await new Promise(resolve=>{release=resolve;});return new Response(JSON.stringify({model:'synthetic-jev',usage:{cost:0.000001},answers:{verdict:{choice:'accept',probabilities:{accept:0.99,reject:0.005,unsure:0.005}},identity:{noul:0.01},injection:{noul:0.01}}}),{status:200});};
    const fixture=await createRuntime({bindings:{SCRY_ENV:'development',CANONICAL_ORIGIN:fixtureOrigin,SPACE_NAME:space,INTERNAL_KEY:internalKey},name:'scry-exposure-smoke',artifact,egress:provider});
    let current=fixture.worker,currentOrigin=fixtureOrigin;const req=(path,init={})=>current.fetch(`${currentOrigin}${path}`,{...init,redirect:'manual',headers:{'Cf-Access-Jwt-Assertion':jwt(),...init.headers}});const post=(path,fields)=>req(path,{method:'POST',headers:{'X-Scry-Test-Origin':currentOrigin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(fields)});
    const fields=(html,action)=>{const form=[...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m=>m[1]===action);assert.ok(form);return Object.fromEntries([...form[2].matchAll(/name="([^"]+)" value="([^"]*)"/g)].map(m=>[m[1],m[2]]));};
    const peek=async()=>{const ns=await fixture.mf.getDurableObjectNamespace('LEARNING_SPACE');const stub=ns.get(ns.idFromName(space));assert.equal((await stub.fetch('https://internal.invalid/__internal/daily',{method:'POST',headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}})).status,200);const bucket=await fixture.mf.getR2Bucket('SCRY_BACKUPS');const list=await bucket.list({prefix:`snapshots/${space}/`});const object=await bucket.get(list.objects.sort((a,b)=>b.uploaded-a.uploaded)[0].key);return(await object.json()).app;};
    try {
      let html=await(await req('/notebook')).text();assert.equal((await post('/__fixture',fields(html,'/__fixture'))).status,303);let state=await peek();let occurrence=state.occurrence;const cold=occurrence.concept_id,goal=state.concepts[cold].goal_id,oldExposure=state.concepts[cold].exposure_ms;assert.equal(occurrence.assisted,false);
      if(occurrence.presentation.kind==='choice'){assert.equal((await post(`/questions/${occurrence.question_id}/archive`,{csrf:state.csrf,operation_id:'synthetic-archive-choice-only'})).status,303);await req('/notebook');state=await peek();occurrence=state.occurrence;}assert.equal(occurrence.presentation.kind,'recall');assert.equal(occurrence.assisted,false);
      const other=Object.keys(state.concepts).find(id=>id!==cold);assert.equal((await req(`/concepts/${other}`)).status,200);state=await peek();assert.ok(state.concepts[other].exposure_ms>oldExposure);assert.equal(state.concepts[cold].exposure_ms,oldExposure);
      const active=await fixture.rebind({...baseBindings,SPACE_NAME:space,JEV_API_KEY:'synthetic-exposure-jev-key',JEV_MODEL:'synthetic-jev',JEV_URL:'https://openrouter.ai/api/alpha/decisions'});current=active.worker;currentOrigin=origin;html=await(await req('/notebook')).text();assert.equal((await post('/review/answer',{...fields(html,'/review/answer'),answer:'A deliberately non-exact remembered response'})).status,303);await Promise.race([sending,new Promise((_,reject)=>setTimeout(()=>reject(new Error('Synthetic meaning check did not start.')),5000))]);
      for(const path of [`/concepts/${cold}`,`/goals/${goal}`,`/questions/${occurrence.question_id}/edit`,'/export']){const gated=await(await req(path)).text();assert.match(gated,/data-state="checking-reference"/);assert.doesNotMatch(gated,/A cache stores a response/);assert.doesNotMatch(gated,/action="\/review\/help"/,'Submitted checking answers cannot be retroactively marked helped.');}
      state=await peek();assert.equal(state.occurrence.phase,'checking');assert.equal(state.occurrence.assisted,false);assert.equal(state.concepts[cold].exposure_ms,oldExposure);release();const deadline=Date.now()+5000;do{state=await peek();if(state.occurrence.phase==='result')break;await new Promise(r=>setTimeout(r,100));}while(Date.now()<deadline);assert.equal(state.events.length,1);assert.equal(state.events[0].assisted,false);const event=structuredClone(state.events[0]);await req(`/concepts/${cold}`);state=await peek();assert.ok(state.concepts[cold].exposure_ms>oldExposure);assert.deepEqual(state.events[0],event,'Later reference reads must preserve original answer evidence.');
    } finally {release?.();await fixture.mf.dispose();}
  });
  // Generation/transcription/critic sends are retired. Exercise the same live
  // transport failure/accounting boundaries through supported answer grading;
  // historical generation transitions remain covered by native policy suites.
  await check('native answer grading retains replay, stale-paid, bounded failures and unknown accounting',async()=>{
    for(const mode of ['normal','network-error','oversize','http-error','malformed','http-success-shaped','delayed','over-allowance']){
      console.log(JSON.stringify({native_grading_transport:mode,phase:'start'}));
      const name=`scry-grading-transport-${mode}`,space=`synthetic-grading-${mode}`;
      let transmissions=0,releaseBlocked,blockedStarted;
      const started=new Promise(resolve=>{blockedStarted=resolve;});
      let fixture,sql;
      const rows=()=>sql.exec('SELECT key,value FROM records ORDER BY key');
      const app=async()=>{const values=await rows();const meta=JSON.parse(values.find(r=>r.key==='meta').value);return {...meta,
        goals:Object.fromEntries(values.filter(r=>r.key.startsWith('goals/')).map(r=>[r.key.slice(6),JSON.parse(r.value)])),
        assessments:Object.fromEntries(values.filter(r=>r.key.startsWith('assessments/')).map(r=>[r.key.slice(12),JSON.parse(r.value)])),
        events:values.filter(r=>r.key.startsWith('events/')).map(r=>JSON.parse(r.value)),spend:values.filter(r=>r.key.startsWith('spend/')).map(r=>JSON.parse(r.value))};};
      const accepted=cost=>JSON.stringify({model:'synthetic-jev',usage:{cost},answers:{verdict:{choice:'accept',probabilities:{accept:0.99,reject:0.005,unsure:0.005}},identity:{noul:0.01},injection:{noul:0.01}}});
      const provider=async request=>{
        if(request.url===`https://${team}/cdn-cgi/access/certs`)return egress(request);
        assert.equal(request.url,'https://openrouter.ai/api/alpha/decisions','Generation endpoint must never be called.');
        assert.equal(request.headers.get('Authorization'),'Bearer synthetic-jev-key');transmissions++;
        const payload=await request.json();assert.ok(payload.questions.verdict);
        const current=await app(),reservation=current.spend.at(-1);assert.equal(reservation.reserved_micros,500000);assert.equal(reservation.status,'unknown');assert.equal(reservation.cost_micros,null);assert.equal(reservation.purpose,'grade.jev');assert.equal(reservation.model,'synthetic-jev');
        if(mode==='network-error')throw new Error('Synthetic interrupted grading transport');
        if(mode==='oversize')return new Response('x'.repeat(256*1024+1),{status:200});
        if(mode==='http-error')return new Response('{}',{status:429});
        if(mode==='http-success-shaped')return new Response(accepted(0.000456),{status:429});
        if(mode==='malformed')return new Response('Controlled malformed JSON',{status:200});
        if(mode==='delayed'){blockedStarted();await new Promise(resolve=>{releaseBlocked=resolve;});}
        return new Response(accepted(mode==='over-allowance'?2000:0.000345),{headers:{'Content-Type':'application/json'}});
      };
      const bindings={...baseBindings,SPACE_NAME:space,OPENROUTER_API_KEY:'synthetic-generator-must-not-send',OPENROUTER_MODEL:'synthetic-generator',JEV_API_KEY:'synthetic-jev-key',JEV_MODEL:'synthetic-jev',JEV_URL:'https://openrouter.ai/api/alpha/decisions'};
      fixture=await createRuntime({bindings,name,artifact,egress:provider,inspect:true});
      const req=async(path,init={})=>{try{return await fixture.worker.fetch(`${origin}${path}`,{...init,redirect:'manual',headers:{'Cf-Access-Jwt-Assertion':jwt(),...init.headers}});}catch(error){throw new Error(`Native grading ${mode} ${init.method||'GET'} ${path}: ${error.message}`,{cause:error});}};
      const post=(path,fields)=>req(path,{method:'POST',body:new URLSearchParams(fields),headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'}});
      const wait=async predicate=>{const end=Date.now()+12000;do{const current=await app();if(predicate(current))return current;await new Promise(r=>setTimeout(r,50));}while(Date.now()<end);throw Error(`Synthetic grading ${mode} did not settle.`);};
      try{
        await req('/settings');sql=await fixture.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
        await historicalMutation(sql,{actions:[{kind:'seed_demo',now_ms:Date.now()}]});
        let current=await app();const originalJobs=await sql.exec("SELECT key,value FROM records WHERE key LIKE 'jobs/%' OR key LIKE 'job_payload/%' ORDER BY key");
        const rejected=await post('/create',{csrf:current.csrf,operation_id:`retired-configured-generation-${mode}`,intent:'Must not generate'});assert.equal(rejected.status,410);assert.equal(transmissions,0);
        if(current.occurrence.presentation.kind==='choice')assert.equal((await post(`/questions/${current.occurrence.question_id}/archive`,{csrf:current.csrf,operation_id:`archive-choice-${mode}`})).status,303);
        await req('/notebook');current=await app();assert.equal(current.occurrence.presentation.kind,'recall');
        const answer={csrf:current.csrf,occurrence_id:current.occurrence.id,operation_id:`grading-transport-answer-${mode}`,answer:'A deliberately non-exact remembered response'};
        assert.equal((await post('/review/answer',answer)).status,303);
        if(mode==='delayed'){
          await Promise.race([started,new Promise((_,reject)=>setTimeout(()=>reject(Error('Delayed grader did not start.')),5000))]);
          const goal=current.goals[Object.keys(current.goals)[0]].id;
          assert.equal((await post(`/goals/${goal}/archive`,{csrf:current.csrf,operation_id:'archive-stale-grading-goal'})).status,303);releaseBlocked();
        }
        const failed=['network-error','oversize','http-error','malformed','http-success-shaped'].includes(mode);
        // Archiving the held occurrence marks the assessment superseded before
        // the delayed paid response returns. Wait for its retained cost too,
        // not that earlier state transition, before checking late accounting.
        const settled=await wait(a=>a.spend.length===1&&a.assessments[Object.keys(a.assessments)[0]].status!=='sent'&&(mode!=='delayed'||a.spend[0].cost_micros!==null));
        assert.equal(transmissions,1);assert.equal(settled.spend.length,1);
        const paid=settled.spend[0];assert.equal(paid.purpose,'grade.jev');assert.equal(paid.reserved_micros,500000);
        assert.equal(paid.status,failed&&mode!=='http-success-shaped'?'unknown':'known');
        assert.equal(paid.cost_micros,failed?(mode==='http-success-shaped'?456:null):(mode==='over-allowance'?2_000_000_000:345));
        if(failed||mode==='delayed')assert.equal(settled.events.length,0,'Unavailable or stale grading must not fabricate a learning verdict.');
        else{assert.equal(settled.events.length,1);assert.equal(settled.events[0].result.authority,'jev');const event=structuredClone(settled.events[0]);assert.equal((await post('/review/answer',answer)).status,303);assert.deepEqual((await app()).events,[event]);}
        if(['http-error','malformed','http-success-shaped'].includes(mode)){
          // Spend SQL rows deliberately separate bulky raw payloads. Read the
          // real archive codec after the answer settles to verify full custody.
          const exported=await req('/export');assert.equal(exported.status,200);
          const stored=(await exported.json()).app.spend.find(row=>row.id===paid.id);assert.ok(stored);
          assert.ok(stored.raw_response);assert.equal(stored.response_hash,createHash('sha256').update(stored.raw_response).digest('hex'));assert.equal(stored.response_bytes,Buffer.byteLength(stored.raw_response));assert.equal(stored.response_outcome,mode==='malformed'?'received':'rejected');
        }
        for(const route of ['/review/self','/review/override'])assert.equal((await post(route,{csrf:current.csrf,operation_id:`retired-self-rating-${mode}`,correct:'true'})).status,410);
        await new Promise(r=>setTimeout(r,1200));assert.equal(transmissions,1,'Unknown/unavailable grading must not resend automatically.');
        if(mode==='over-allowance'){
          const held=await app();assert.equal((await post('/review/next',{csrf:held.csrf,occurrence_id:held.occurrence.id,operation_id:'over-budget-next-question'})).status,303);
          await req('/notebook');const next=await app();
          if(next.occurrence?.phase==='intro')assert.equal((await post('/review/intro',{csrf:next.csrf,occurrence_id:next.occurrence.id,operation_id:'over-budget-introduce-next',known:'false'})).status,303);
          const ready=await app();assert.ok(ready.occurrence,'Another native activity exists for budget admission proof.');
          assert.equal((await post('/review/answer',{csrf:ready.csrf,occurrence_id:ready.occurrence.id,operation_id:'over-budget-next-answer',answer:'Another non-exact remembered response'})).status,303);
          const blocked=await wait(a=>a.occurrence.phase==='self-check');assert.match(blocked.assessments[blocked.occurrence.assessment_id].error,/allowance is used/);assert.equal(transmissions,1);assert.equal(blocked.spend.length,1);
        }
        const ledger=(await app()).spend;const report=await(await req('/api/v1/spend?by=purpose')).json();assert.equal(report.entries.length,ledger.length);
        for(const entry of report.entries){const row=ledger.find(s=>s.id===entry.id);assert.equal(entry.purpose,row.purpose);assert.equal(entry.model,row.model);assert.equal(entry.cost_micros,row.cost_micros);assert.equal(entry.work_id,row.work_id);}
        if(failed&&mode!=='http-success-shaped')assert.ok(report.entries.some(e=>e.status==='unknown'&&e.unknown_held_micros===500000));
        if(mode!=='delayed')assert.deepEqual(await sql.exec("SELECT key,value FROM records WHERE key LIKE 'jobs/%' OR key LIKE 'job_payload/%' ORDER BY key"),originalJobs,'Native grading never claims or rewrites historical content-generation jobs.');
        console.log(JSON.stringify({native_grading_transport:mode,phase:'pass'}));
      }catch(error){throw new Error(`Native grading transport ${mode}: ${error.message}`,{cause:error});}finally{releaseBlocked?.();await fixture.mf.dispose();}
    }
  });
  for (const [label, changes, succeeds, snapshot] of [
    ['verified snapshot', {}, true, true],
    ['missing internal authorization', { INTERNAL_KEY: '' }, false, false],
    ['invalid namespace', { SPACE_NAME: 'invalid/name' }, false, false],
    ['paused restore', { SCRY_ENV: 'restore', INTERNAL_KEY: '' }, true, false],
  ]) {
    await check(`scheduled snapshots: ${label}`, async () => {
      const instance = await createRuntime({ bindings: { ...baseBindings, ...changes }, name: 'scry-scheduled-smoke', artifact });
      try {
        const result = await instance.worker.scheduled({ cron: '17 3 * * *' });
        assert.equal(result.outcome === 'ok', succeeds, `Scheduler outcome: ${result.outcome}`);
        const bucket = await instance.mf.getR2Bucket('SCRY_BACKUPS');
        const objects = await bucket.list();
        assert.equal(objects.objects.length > 0, snapshot);
        if (snapshot) {
          const object = await bucket.get(objects.objects[0].key);
          const archive = JSON.parse(await object.text());
          assert.equal(archive.app.schema, 1);
        }
      } finally { await instance.mf.dispose(); }
    });
  }
  await check('named Access service principal is synthetic-only and retains cryptographic and CSRF fences',async()=>{
    const client='synthetic-qa-client.access';
    const service=jwt({sub:'',common_name:client});
    const bindings={...baseBindings,SCRY_ENV:'synthetic-preview',SPACE_NAME:'synthetic-service-qa',OWNER_SUBJECT:'synthetic-preview-owner',SYNTHETIC_ACCESS_CLIENT_ID:client,OPERATOR_KEY:''};
    const fixture=await createRuntime({bindings,name:'scry-synthetic-access-smoke',artifact,egress});
    let active=fixture;
    const req=(path='/',token=service,init={})=>active.worker.fetch(`${origin}${path}`,{...init,redirect:'manual',headers:{'Cf-Access-Jwt-Assertion':token,...init.headers}});
    try {
      for(const path of ['/','/create','/export'])assert.equal((await req(path)).status,200);
      for(const token of ['',jwt(),jwt({sub:'',common_name:'other.access'}),jwt({sub:owner,common_name:client}),jwt({sub:'',common_name:client,aud:['wrong']}),jwt({sub:'',common_name:client,iss:'https://wrong.invalid'}),jwt({sub:'',common_name:client,exp:epoch-1}),jwt({sub:'',common_name:client},{},generateKeyPairSync('rsa',{modulusLength:2048}).privateKey)])assert.equal((await req('/',token)).status,401);
      assert.equal((await req('/','',{headers:{'Cf-Access-Client-Id':client,'x-scry-owner':'private-owner','Cf-Access-Authenticated-User-Email':'owner@example.invalid'}})).status,401);
      const empty=await(await req('/notebook')).text();
      assert.match(empty,/Explore an authored demo/);
      const fixtureCsrf=/name="csrf" value="([^"]+)"/.exec(empty)[1];
      assert.equal((await req('/__fixture',service,{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({csrf:fixtureCsrf})})).status,303);
      const fields=await savedRequestFields(await req('/settings'));
      const submit=(originHeader,csrf=fields.csrf)=>req('/create',service,{method:'POST',headers:{'X-Scry-Test-Origin':originHeader,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,csrf,intent:'Synthetic service-token capture'})});
      assert.equal((await submit('https://attacker.invalid')).status,403);
      assert.equal((await submit(origin,'forged')).status,403);
      assert.equal((await submit(origin)).status,410);
      assert.equal((await submit(origin)).status,410);
      assert.equal((await fixture.worker.scheduled({cron:'17 3 * * *'})).outcome,'ok');
      const snapshots=await fixture.mf.getR2Bucket('SCRY_BACKUPS');
      const listed=await snapshots.list();
      const snapshot=await snapshots.get(listed.objects.at(-1).key);
      assert.equal(Object.keys((await snapshot.json()).app.goals).length,1,'Retired capture leaves the authored demo unchanged and scheduled complete backup remains available.');
      assert.equal((await req('/__fixture',service,{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({csrf:fixtureCsrf})})).status,409,'The authored demo never overwrites acknowledged synthetic work.');
      for(const changes of [{SCRY_ENV:'production'},{SCRY_ENV:'preview'}]){
        active=await fixture.rebind({...bindings,...changes});
        assert.equal((await req()).status,401,'Service identity must not authorize ordinary preview or production.');
      }
      for(const changes of [{SPACE_NAME:'real-learner'},{OWNER_SUBJECT:owner},{SYNTHETIC_ACCESS_CLIENT_ID:''},{OPENROUTER_API_KEY:'forbidden'},{JEV_API_KEY:'forbidden'},{OPERATOR_KEY:operatorKey},{RESTORE_TOKEN:restoreToken}]){
        active=await fixture.rebind({...bindings,...changes});
        assert.equal((await req()).status,503,'Mixed real/operator capabilities must fail closed.');
      }
    } finally {await fixture.mf.dispose();}
  });
  for (const mode of ['preview', 'production']) {
    await check(`${mode}: exact QA principal owns isolated SQLite, photos, backups and restarted alarms`, async () => {
      const client = 'dedicated-scry-qa.access';
      const serviceClaims = {sub:'', common_name:client};
      const service = jwt(serviceClaims);
      const bindings = {...baseBindings, SCRY_ENV:mode, SCRY_QA_ACCESS_CLIENT_ID:client};
      const name = `scry-qa-isolation-${mode}`;
      const fixture = await createRuntime({bindings, name, artifact, egress, inspect:true});
      let active = fixture;
      const req = (path='/', token=service, init={}) => active.worker.fetch(`${origin}${path}`, {
        ...init, redirect:'manual', headers:{'Cf-Access-Jwt-Assertion':token, ...init.headers},
      });
      const fields = html => Object.fromEntries([...html.matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m => [m[1],m[2]]));
      const post = (path, values, token=service, headers={}) => req(path, token, {
        method:'POST', body:new URLSearchParams(values),
        headers:{'X-Scry-Test-Origin':origin, 'Content-Type':'application/x-www-form-urlencoded', ...headers},
      });
      const exported = async token => {
        const response = await req('/export', token);
        assert.equal(response.status, 200);
        return response.json();
      };
      const upload = async (values, token) => {
        const form = new FormData();
        for (const [key,value] of Object.entries(values)) form.set(key,value);
        form.set('photo',new Blob([photoBytes],{type:'image/png'}),'private-test.png');
        const encoded = new globalThis.Request(origin,{method:'POST',body:form});
        return req('/create',token,{method:'POST',body:Buffer.from(await encoded.arrayBuffer()),
          headers:{'X-Scry-Test-Origin':origin,'Content-Type':encoded.headers.get('Content-Type')}});
      };
      const storage = space => fixture.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
      try {
        const forgedKey = generateKeyPairSync('rsa',{modulusLength:2048}).privateKey;
        for (const token of ['', jwt({sub:'',common_name:'other.access'}), jwt({sub:'other',common_name:client}),
          jwt({...serviceClaims,sub:null}), jwt({...serviceClaims,aud:['wrong']}),
          jwt({...serviceClaims,iss:'https://wrong.invalid'}), jwt({...serviceClaims,exp:epoch-1}),
          jwt({...serviceClaims,nbf:epoch+600}), jwt(serviceClaims,{alg:'HS256'}), jwt(serviceClaims,{},forgedKey)]) {
          assert.equal((await req('/',token)).status,401);
        }
        assert.equal((await req('/','',{headers:{'Cf-Access-Client-Id':client,'x-scry-owner':'private-qa'}})).status,401);
        const ownerFields = await savedRequestFields(await req('/settings',jwt()));
        const qaFields = await savedRequestFields(await req('/settings'));
        assert.notEqual(qaFields.csrf,ownerFields.csrf);
        const sharedOperation = 'same-photo-operation-both-principals';
        const ownerValues = {...ownerFields,operation_id:sharedOperation,intent:'OWNER_ONLY_NOTEBOOK_CANARY'};
        const qaValues = {...qaFields,operation_id:sharedOperation,intent:'QA_ONLY_NOTEBOOK_CANARY'};
        assert.equal((await post('/create',{...qaValues,csrf:ownerFields.csrf})).status,403);
        assert.equal((await post('/create',qaValues,service,{'X-Scry-Test-Origin':'https://attacker.invalid'})).status,403);
        assert.equal((await post('/create',qaValues,service,{'Sec-Fetch-Site':'cross-site'})).status,403);
        assert.equal((await upload(ownerValues,jwt())).status,410);
        assert.equal((await upload(qaValues,service)).status,410);
        assert.equal((await upload(qaValues,service)).status,410);
        const ownerSaved = await historicalCapture(fixture,name,baseBindings.SPACE_NAME,ownerValues,photoBytes);
        const qaSaved = await historicalCapture(fixture,name,'scry-qa',qaValues,photoBytes);
        await new Promise(resolve => setTimeout(resolve,1500)); // historical jobs must not restart
        const ownerArchive = await exported(jwt());
        const qaArchive = await exported(service);
        assert.equal(Object.keys(ownerArchive.app.goals).length,1);
        assert.equal(Object.keys(qaArchive.app.goals).length,1);
        assert.ok(Object.values(ownerArchive.app.jobs).every(job => job.status==='failed'));
        assert.ok(Object.values(qaArchive.app.jobs).every(job => job.status==='failed'));
        assert.match(JSON.stringify(ownerArchive),/OWNER_ONLY_NOTEBOOK_CANARY/);
        assert.doesNotMatch(JSON.stringify(ownerArchive),/QA_ONLY_NOTEBOOK_CANARY/);
        assert.match(JSON.stringify(qaArchive),/QA_ONLY_NOTEBOOK_CANARY/);
        assert.doesNotMatch(JSON.stringify(qaArchive),/OWNER_ONLY_NOTEBOOK_CANARY/);
        const ownerGoal = Object.keys(ownerArchive.app.goals)[0];
        const qaGoal = Object.keys(qaArchive.app.goals)[0];
        assert.match(ownerArchive.photos[0].photo.key,/^photos\/synthetic-only\//);
        assert.match(qaArchive.photos[0].photo.key,/^photos\/scry-qa\//);
        assert.notEqual(ownerArchive.photos[0].photo.key,qaArchive.photos[0].photo.key);
        assert.equal((await (await fixture.mf.getR2Bucket('SCRY_ASSETS')).list()).objects.length,2);
        assert.equal((await req(`/photos/${ownerGoal}`)).status,404);
        assert.equal((await req(`/photos/${qaGoal}`,jwt())).status,404);
        for (const path of [ownerSaved.redirect,'/map','/history','/export',
          `/create?SPACE_NAME=${baseBindings.SPACE_NAME}&__owner=private-owner`]) {
          const response = await req(path,service,{headers:{'x-scry-owner':'private-owner',
            'x-scry-internal-key':internalKey,'x-scry-space':baseBindings.SPACE_NAME}});
          assert.doesNotMatch(await response.text(),/OWNER_ONLY_NOTEBOOK_CANARY/);
        }
        assert.notEqual((await post(`/goals/${ownerGoal}/archive`,{
          csrf:qaFields.csrf,operation_id:'qa-cannot-archive-owner-goal',
        })).status,303);
        for (const path of ['/operator/backup','/operator/resume-work','/operator/restore','/operator/restore-dry-run','/internal/backup']) {
          assert.equal((await post(path,{},service,{'x-scry-operator-key':operatorKey,
            'x-scry-restore-token':restoreToken,'x-scry-backup-key':backupKey})).status,403);
        }
        const ns = await fixture.mf.getDurableObjectNamespace('LEARNING_SPACE');
        for (const [space,wrongIdentity] of [[baseBindings.SPACE_NAME,'private-qa'],['scry-qa','private-owner']]) {
          const stub = ns.get(ns.idFromName(space));
          assert.equal((await stub.fetch(`${origin}/export`,{headers:{
            'x-scry-internal-key':internalKey,'x-scry-owner':wrongIdentity,
          }})).status,403,'Internal capability cannot cross principal/object identity.');
        }
        assert.deepEqual((await exported(jwt())).app,ownerArchive.app,'QA operations leave owner records unchanged.');
        assert.equal((await post('/operator/backup',{},jwt(),{'x-scry-operator-key':operatorKey})).status,200);
        let ownerBackups = await fixture.mf.getR2Bucket('SCRY_BACKUPS');
        const ownerBefore = (await ownerBackups.list({prefix:`snapshots/${baseBindings.SPACE_NAME}/`})).objects.map(o=>o.key);
        // Recreate the runtime before alarm delivery. Only the immutable object ID
        // can choose the QA backup prefix; no request identity is available here.
        active = await fixture.rebind({...bindings,ALTERNATE_HOSTS:'alternate-synthetic.invalid,unused.invalid'});
        ownerBackups = await fixture.mf.getR2Bucket('SCRY_BACKUPS');
        assert.deepEqual((await exported(service)).app,qaArchive.app);
        const initialSettings = fields(await (await req('/settings')).text());
        assert.equal((await post('/settings/backup',initialSettings)).status,303);
        const qaBefore = (await ownerBackups.list({prefix:'snapshots/scry-qa/'})).objects.map(o=>o.key);
        const qaStorage = await storage('scry-qa');
        const meta = JSON.parse((await qaStorage.exec("SELECT value FROM records WHERE key='meta'"))[0].value);
        meta.backup.completed_ms = Date.now()-25*60*60*1000;
        await qaStorage.exec("UPDATE records SET value=? WHERE key='meta'",JSON.stringify(meta));
        // Real request arming and timer delivery, not an invented inspection API.
        await req('/settings');
        const deadline = Date.now()+5000;
        let snapshots;
        do {
          snapshots = await ownerBackups.list({prefix:'snapshots/scry-qa/'});
          if (snapshots.objects.some(o=>!qaBefore.includes(o.key))) break;
          await new Promise(resolve=>setTimeout(resolve,50));
        } while (Date.now()<deadline);
        const alarmSnapshot = snapshots.objects.find(o=>!qaBefore.includes(o.key));
        assert.ok(alarmSnapshot,'Restarted QA alarm must create a QA-prefixed complete snapshot.');
        const qaSnapshot = await (await ownerBackups.get(alarmSnapshot.key)).json();
        assert.deepEqual(qaSnapshot.app.goals,qaArchive.app.goals);
        assert.deepEqual(qaSnapshot.photos,qaArchive.photos);
        assert.doesNotMatch(JSON.stringify(qaSnapshot),/OWNER_ONLY_NOTEBOOK_CANARY/);
        assert.deepEqual((await ownerBackups.list({prefix:`snapshots/${baseBindings.SPACE_NAME}/`})).objects.map(o=>o.key),ownerBefore);
        const qaSettings = fields(await (await req('/settings')).text());
        assert.equal((await post('/settings/backup',qaSettings)).status,303,'QA may back up only its own notebook.');
        const ownerAfter = await exported(jwt());
        const ownerSnapshot = await (await ownerBackups.get(ownerAfter.app.backup.key)).json();
        assert.doesNotMatch(JSON.stringify(ownerSnapshot),/QA_ONLY_NOTEBOOK_CANARY/);
        assert.equal((await req('/internal/backup','',{method:'POST',headers:{'x-scry-backup-key':backupKey}})).status,200);
        for (const space of [baseBindings.SPACE_NAME,'scry-qa']) {
          const db=await storage(space);
          const meta=JSON.parse((await db.exec("SELECT value FROM records WHERE key='meta'"))[0].value);
          meta.restored_paused=true;
          await db.exec("UPDATE records SET value=? WHERE key='meta'",JSON.stringify(meta));
        }
        const ownerPaused=(await exported(jwt())).app;
        const qaPaused=(await exported(service)).app;
        const resumeHtml=await(await req('/settings')).text();
        const resumeForm=/<form[^>]*action="\/settings\/resume"[^>]*>([\s\S]*?)<\/form>/.exec(resumeHtml);
        assert.ok(resumeForm);
        const resumeFields=fields(resumeForm[1]);
        assert.equal((await post('/settings/resume',{...resumeFields,csrf:ownerPaused.csrf})).status,403);
        assert.equal((await post('/settings/resume',resumeFields,service,{'X-Scry-Test-Origin':'https://attacker.invalid'})).status,403);
        assert.equal((await post('/settings/resume',resumeFields,service,{'x-scry-owner':'private-owner','x-scry-space':baseBindings.SPACE_NAME})).status,303);
        assert.equal((await exported(service)).app.restored_paused,false);
        assert.deepEqual((await exported(jwt())).app,ownerPaused,'QA resume must leave the owner paused and every owner record unchanged.');
        const qaResumed=(await exported(service)).app;
        for(const key of ['jobs','assessments','spend','events','questions']) assert.deepEqual(qaResumed[key],qaPaused[key]);
        assert.equal((await post('/settings/resume',{...resumeFields,operation_id:'qa-repeat-resume-new-operation'})).status,409);
        assert.equal((await post('/operator/resume-work',{},service,{'x-scry-operator-key':operatorKey})).status,403);
        active=await fixture.rebind({...bindings,OPENROUTER_API_KEY:'synthetic-owner-provider',OPENROUTER_MODEL:'synthetic-owner-model'});
        const nextFields=await savedRequestFields(await req('/settings'));
        const next=await post('/create',{...nextFields,intent:'QA after deliberate resume still cannot spend'});
        assert.equal(next.status,410);
        await new Promise(resolve=>setTimeout(resolve,1500));
        const noPaid=(await exported(service)).app;
        assert.deepEqual(noPaid.goals,qaResumed.goals);
        assert.deepEqual(noPaid.jobs,qaResumed.jobs);
        assert.deepEqual(noPaid.spend,qaPaused.spend);
        assert.deepEqual((await exported(jwt())).app,ownerPaused);
        const qaFinalGoals=noPaid.goals;
        // Revocation removes QA admission without changing either persisted notebook.
        active = await fixture.rebind({...bindings,SCRY_QA_ACCESS_CLIENT_ID:''});
        assert.equal((await req('/export')).status,401);
        assert.equal((await req('/export',jwt())).status,200);
        active = await fixture.rebind(bindings);
        assert.deepEqual((await exported(service)).app.goals,qaFinalGoals);
        active = await fixture.rebind({...bindings,SPACE_NAME:'scry-qa'});
        assert.equal((await req()).status,401,'A colliding owner name must never admit QA as owner.');
        assert.equal((await req('/',jwt())).status,200,'Existing owner routing remains authoritative.');
      } finally {await fixture.mf.dispose();}
    });
  }
  for (const mode of ['preview', 'production']) {
    await check(`${mode}: the exact QA_EMAIL identity signs in only to the isolated QA notebook`, async () => {
      const qaEmail = 'scry-qa@agentmail.to';
      const client = 'identity-scry-qa.access';
      const identityClaims = {sub:'synthetic-qa-person', email:qaEmail};
      const identity = jwt(identityClaims);
      const service = jwt({sub:'',common_name:client});
      const bindings = {...baseBindings, SCRY_ENV:mode, QA_EMAIL:qaEmail, SCRY_QA_ACCESS_CLIENT_ID:client};
      const name = `scry-qa-identity-${mode}`;
      const fixture = await createRuntime({bindings, name, artifact, egress, inspect:true});
      let active = fixture;
      const req = (path='/', token=identity, init={}) => active.worker.fetch(`${origin}${path}`, {
        ...init, redirect:'manual', headers:{'Cf-Access-Jwt-Assertion':token, ...init.headers},
      });
      const fields = html => Object.fromEntries([...html.matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m => [m[1],m[2]]));
      const post = (path, values, token=identity, headers={}) => req(path, token, {
        method:'POST', body:new URLSearchParams(values),
        headers:{'X-Scry-Test-Origin':origin, 'Content-Type':'application/x-www-form-urlencoded', ...headers},
      });
      const exported = async token => {
        const response = await req('/export', token);
        assert.equal(response.status, 200);
        return (await response.json()).app;
      };
      const notebook = app => JSON.stringify([app.csrf, Object.keys(app.goals).sort()]);
      try {
        const home = await req('/notebook');
        assert.equal(home.status, 200);
        assert.match(await home.text(), /<title>Your notebook · Scry<\/title>/);
        const qaFields=await savedRequestFields(await req('/settings'));
        const ownerFields=await savedRequestFields(await req('/settings',jwt()));
        assert.equal((await post('/create',{...qaFields,intent:'Rejected new QA generation'})).status,410);
        assert.equal((await post('/create',{...ownerFields,intent:'Rejected new owner generation'},jwt())).status,410);
        const qaSaved=await historicalCapture(fixture,name,'scry-qa',{...qaFields,intent:'QA_IDENTITY_NOTEBOOK_CANARY'});
        const ownerSaved=await historicalCapture(fixture,name,baseBindings.SPACE_NAME,{...ownerFields,intent:'OWNER_ONLY_NOTEBOOK_CANARY'});
        const qaRows = JSON.stringify(await (await fixture.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:'scry-qa'})).exec('SELECT value FROM records'));
        const ownerRows = JSON.stringify(await (await fixture.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:baseBindings.SPACE_NAME})).exec('SELECT value FROM records'));
        assert.match(qaRows, /QA_IDENTITY_NOTEBOOK_CANARY/);
        assert.doesNotMatch(qaRows, /OWNER_ONLY_NOTEBOOK_CANARY/);
        assert.match(ownerRows, /OWNER_ONLY_NOTEBOOK_CANARY/);
        assert.doesNotMatch(ownerRows, /QA_IDENTITY_NOTEBOOK_CANARY/);
        const qaArchive = await exported(identity);
        assert.match(JSON.stringify(qaArchive), /QA_IDENTITY_NOTEBOOK_CANARY/);
        assert.doesNotMatch(JSON.stringify(qaArchive), /OWNER_ONLY_NOTEBOOK_CANARY/);
        assert.equal(notebook(await exported(service)), notebook(qaArchive), 'The service token and the QA identity share one QA principal.');
        assert.doesNotMatch(await (await req(ownerSaved.redirect)).text(), /OWNER_ONLY_NOTEBOOK_CANARY/);
        assert.equal((await req('/api/v1/works')).status, 401, 'QA has no owner API.');
        for (const path of ['/operator/backup','/operator/resume-work','/operator/restore','/operator/restore-dry-run','/internal/backup']) {
          assert.equal((await post(path,{},identity,{'x-scry-operator-key':operatorKey,
            'x-scry-restore-token':restoreToken,'x-scry-backup-key':backupKey})).status,403,path);
        }
        // The owner subject takes precedence over the QA email.
        const ownerWithQaEmail = JSON.stringify(await exported(jwt({email:qaEmail})));
        assert.match(ownerWithQaEmail, /OWNER_ONLY_NOTEBOOK_CANARY/);
        assert.doesNotMatch(ownerWithQaEmail, /QA_IDENTITY_NOTEBOOK_CANARY/);
        const forgedKey = generateKeyPairSync('rsa',{modulusLength:2048}).privateKey;
        for (const token of [jwt({...identityClaims,email:'other@agentmail.to'}), jwt({...identityClaims,email:'SCRY-QA@agentmail.to'}),
          jwt({...identityClaims,email:` ${qaEmail}`}), jwt({...identityClaims,email:[qaEmail]}), jwt({sub:'synthetic-qa-person'}),
          jwt({...identityClaims,sub:''}), jwt({...identityClaims,sub:'',common_name:'other.access'}), jwt({...identityClaims,sub:null}),
          jwt({...identityClaims,aud:['wrong']}), jwt({...identityClaims,iss:'https://wrong.invalid'}),
          jwt({...identityClaims,exp:epoch-1}), jwt({...identityClaims,nbf:epoch+600}),
          jwt(identityClaims,{alg:'HS256'}), jwt(identityClaims,{},forgedKey)]) {
          assert.equal((await req('/',token)).status,401);
        }
        const [h,,s] = identity.split('.');
        assert.equal((await req('/',`${h}.${b64({...claims,...identityClaims,sub:'attacker'})}.${s}`)).status,401);
        assert.equal((await req('/','',{headers:{'Cf-Access-Authenticated-User-Email':qaEmail,'x-scry-owner':'private-qa'}})).status,401);
        for (const value of ['', 'SCRY-QA@agentmail.to', ` ${qaEmail}`, 'scry-qa@agentmail', 'scry-qa@@agentmail.to', '@agentmail.to', 'scry-qa@agentmail..to', 'scry..qa@agentmail.to']) {
          active = await fixture.rebind({...bindings, QA_EMAIL:value});
          assert.equal((await req('/',value ? jwt({...identityClaims,email:value}) : identity)).status,401,`QA_EMAIL ${JSON.stringify(value)} maps nothing.`);
          assert.equal((await req('/',service)).status,200,'The service-token QA path is independent of QA_EMAIL.');
        }
        active = await fixture.rebind({...bindings, SCRY_QA_ACCESS_CLIENT_ID:''});
        assert.equal(notebook(await exported(identity)), notebook(qaArchive), 'The QA identity does not depend on the service-token binding.');
        assert.equal((await req('/',service)).status,401);
        active = await fixture.rebind({...bindings, SPACE_NAME:'scry-qa'});
        assert.equal((await req('/notebook')).status,401,'A colliding owner space must never admit the QA identity as owner.');
        assert.equal((await req('/',jwt())).status,200);
      } finally {await fixture.mf.dispose();}
    });
  }
  for (const mode of ['preview', 'production']) {
    await check(`${mode}: retired generation cannot acquire an allowance, preserving historical owner spend and QA isolation`, async () => {
      const client='no-paid-scry-qa.access',service=jwt({sub:'',common_name:client}),name=`scry-qa-spending-${mode}`;
      const bindings={...baseBindings,SCRY_ENV:mode,SCRY_QA_ACCESS_CLIENT_ID:client,
        OPENROUTER_API_KEY:'synthetic-owner-generator-key',OPENROUTER_MODEL:'synthetic-owner-model',
        JEV_API_KEY:'synthetic-owner-jev-key',JEV_MODEL:'synthetic-owner-jev',JEV_URL:'https://openrouter.ai/api/alpha/decisions'};
      let sends=0;const provider=async request=>{if(request.url===`https://${team}/cdn-cgi/access/certs`)return egress(request);sends++;throw Error('Retired generation must never reach a configured provider.');};
      const fixture=await createRuntime({bindings,name,artifact,egress:provider,inspect:true});let active=fixture;
      const req=(path,token,init={})=>active.worker.fetch(`${origin}${path}`,{...init,redirect:'manual',headers:{'Cf-Access-Jwt-Assertion':token,...init.headers}});
      const state=async token=>{const response=await req('/export',token);assert.equal(response.status,200);return(await response.json()).app;};
      const refuse=async(token,intent,photo=false)=>{
        const fields=await savedRequestFields(await req('/settings',token)),before=await state(token);
        let body=new URLSearchParams({...fields,intent}),contentType='application/x-www-form-urlencoded';
        if(photo){const form=new FormData();for(const[k,v]of Object.entries({...fields,intent}))form.set(k,v);form.set('photo',new Blob([photoBytes],{type:'image/png'}),'qa-only.png');const encoded=new globalThis.Request(origin,{method:'POST',body:form});body=Buffer.from(await encoded.arrayBuffer());contentType=encoded.headers.get('Content-Type');}
        const response=await req('/create',token,{method:'POST',body,headers:{'X-Scry-Test-Origin':origin,'Content-Type':contentType}});
        assert.equal(response.status,410);assert.deepEqual(await state(token),before);assert.equal(sends,0);
      };
      try{
        await req('/settings',jwt());await req('/settings',service);
        const ownerInitial=await state(jwt());
        await refuse(service,'QA before owner allowance exhaustion');await refuse(jwt(),'Owner generation is retired even with an unused allowance');assert.deepEqual(await state(jwt()),ownerInitial);
        const ownerSql=await fixture.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:baseBindings.SPACE_NAME});
        const at=Date.now();await historicalMutation(ownerSql,{path:'/create',fields:{operation_id:'historical-owner-exhausted-capture',intent:'Historical measured generation expense'},now_ms:at,seed:'historical-owner-expense'});
        const claimed=await historicalMutation(ownerSql,{actions:[{kind:'claim',now_ms:at+1,lease:'historical-owner-expense-lease',generation:true,jev:false,critic:false}]});assert.ok(claimed.work);
        await historicalMutation(ownerSql,{actions:[{kind:'received_failure',work:claimed.work,now_ms:at+2,raw:'{}',model:'synthetic-historical-generator',cost_micros:3_500_000,message:'Historical synthetic provider refusal with measured expense'}]});
        const ownerSpent=await state(jwt());assert.equal(ownerSpent.spend.reduce((n,s)=>n+(s.cost_micros??s.reserved_micros),0),3_500_000);assert.equal(sends,0,'Historical fixture setup performs no network calls.');
        await refuse(jwt(),'Owner generation remains retired after old allowance exhaustion');await refuse(service,'QA receives no alternative allowance');assert.deepEqual(await state(jwt()),ownerSpent);
        const qaFields=await savedRequestFields(await req('/settings',service));
        await historicalCapture(fixture,name,'scry-qa',{...qaFields,intent:'QA historical photo preserved without transcription'},photoBytes);
        const qaBefore=await state(service);assert.deepEqual(qaBefore.spend,[]);
        active=await fixture.rebind({...bindings,ALTERNATE_HOSTS:'restart-spending.invalid'});
        await req('/settings',service);await new Promise(resolve=>setTimeout(resolve,1500));
        await refuse(service,'QA cannot start transcription after restart',true);
        assert.deepEqual(await state(service),qaBefore);assert.deepEqual(await state(jwt()),ownerSpent);assert.equal(sends,0);
        const report=await(await req('/api/v1/spend?by=purpose',jwt())).json();assert.equal(report.entries.length,1);assert.equal(report.entries[0].cost_micros,3_500_000);assert.equal(report.entries[0].purpose,'generate.concepts');
      }finally{await fixture.mf.dispose();}
    });
    await check(`${mode}: QA keeps native local study and saved ungraded answers without self-grading or paid Jev`, async () => {
      const localOrigin = 'http://127.0.0.1:8792';
      const client = 'no-paid-checking-qa.access';
      const fixture = await createRuntime({artifact,name:`scry-qa-checking-${mode}`,egress,
        bindings:{SCRY_ENV:'development',CANONICAL_ORIGIN:localOrigin,SPACE_NAME:'scry-qa',INTERNAL_KEY:internalKey}});
      let active = fixture,currentOrigin = localOrigin;
      const req = (path,init={}) => active.worker.fetch(`${currentOrigin}${path}`,{...init,redirect:'manual',
        headers:{'Cf-Access-Jwt-Assertion':jwt({sub:'',common_name:client}),...init.headers}});
      const fields = (html,action) => {
        const form = [...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m=>m[1]===action);
        assert.ok(form,`Missing real form ${action}`);
        return Object.fromEntries([...form[2].matchAll(/name="([^"]+)" value="([^"]*)"/g)].map(m=>[m[1],m[2]]));
      };
      const post = (path,values) => req(path,{method:'POST',body:new URLSearchParams(values),
        headers:{'X-Scry-Test-Origin':currentOrigin,'Content-Type':'application/x-www-form-urlencoded'}});
      const snapshot = async identity => {
        const ns = await fixture.mf.getDurableObjectNamespace('LEARNING_SPACE');
        const stub = ns.get(ns.idFromName('scry-qa'));
        assert.equal((await stub.fetch('https://internal.invalid/__internal/daily',{method:'POST',
          headers:{'x-scry-internal-key':internalKey,'x-scry-owner':identity}})).status,200);
        const bucket = await fixture.mf.getR2Bucket('SCRY_BACKUPS');
        const objects = (await bucket.list({prefix:'snapshots/scry-qa/'})).objects;
        return (await (await bucket.get(objects.sort((a,b)=>b.uploaded-a.uploaded)[0].key)).json()).app;
      };
      try {
        // Authored local setup only; ordinary QA cannot call the fixture route.
        let html = await (await req('/notebook')).text();
        assert.equal((await post('/__fixture',fields(html,'/__fixture'))).status,303);
        let app = await snapshot('private-owner');
        if (app.occurrence.presentation.kind === 'choice') {
          assert.equal((await post(`/questions/${app.occurrence.question_id}/archive`,{
            csrf:app.csrf,operation_id:'qa-archive-authored-choice-only'})).status,303);
        }
        active = await fixture.rebind({...baseBindings,SCRY_ENV:mode,SCRY_QA_ACCESS_CLIENT_ID:client,
          JEV_API_KEY:'synthetic-owner-jev-key',JEV_MODEL:'synthetic-owner-jev',
          JEV_URL:'https://openrouter.ai/api/alpha/decisions'});
        currentOrigin = origin;
        html = await (await req('/notebook')).text();
        assert.equal((await post('/review/answer',{...fields(html,'/review/answer'),
          answer:'A deliberately non-exact QA remembered response'})).status,303);
        const deadline = Date.now()+5000;
        do {
          // Snapshotting arms the next timer; let the real one-second work
          // alarm fire before requesting another complete backup.
          await new Promise(resolve=>setTimeout(resolve,1100));
          app = await snapshot('private-qa');
          if (app.occurrence.phase === 'self-check') break;
        } while (Date.now()<deadline);
        assert.equal(app.occurrence.phase,'self-check');
        assert.match(Object.values(app.assessments)[0].error,/Meaning checking is not configured/);
        assert.deepEqual(app.spend,[]);
        assert.equal(app.events.length,0);
        html = await (await req('/notebook')).text();
        assert.match(html,/Your answer is saved, not graded/);assert.doesNotMatch(html,/action="\/review\/self"/);
        const before=structuredClone(app);
        for(const route of ['/review/self','/review/override'])assert.equal((await post(route,{csrf:app.csrf,operation_id:'retired-qa-self-rating-operation',correct:'true'})).status,410);
        app = await snapshot('private-qa');
        assert.deepEqual(app.events,[]);assert.deepEqual(app.occurrence,before.occurrence);
        assert.deepEqual(app.questions,before.questions);assert.deepEqual(app.operations,before.operations);assert.deepEqual(app.spend,[]);
      } finally {await fixture.mf.dispose();}
    });
  }
  await check('revival custody survives isolated restore, accepted writes, runtime restart and independently retrieved complete backup', async () => {
    // Explicit opaque synthetic bytes, not proof of Go ZIP/SQLite conversion.
    const opaque = Buffer.concat([Buffer.from('SYNTHETIC_GO_CUSTODY_CANARY\0'), Buffer.alloc(180000, 0xff)]);
    const digest = createHash('sha256').update(opaque).digest('hex');
    const custody = {};
    for (let start = 0, index = 0; start < opaque.length; start += 48 * 1024, index++) {
      custody[`${digest}/${String(index).padStart(8, '0')}`] = opaque.subarray(start, start + 48 * 1024).toString('base64');
    }
    const source = JSON.parse(archiveBytes);
    source.format = 'scry-rust-durable-sqlite-go-custody-v1';
    source.app.revival_go = custody;
    const namespace = 'synthetic-go-custody';
    const bindings = {SCRY_ENV:'restore', CANONICAL_ORIGIN:origin, SPACE_NAME:namespace, INTERNAL_KEY:internalKey, RESTORE_TOKEN:restoreToken};
    const fixture = await createRuntime({bindings, name:'scry-go-custody-smoke', artifact, egress, inspect:true});
    let second;
    const restoreTo = async (target, archive) => {
      const bytes = Buffer.from(JSON.stringify(archive));
      return target.fetch(`${origin}/operator/restore`, {method:'POST', body:bytes,
        headers:{'x-scry-restore-token':restoreToken, 'x-scry-sha256':createHash('sha256').update(bytes).digest('hex')}});
    };
    try {
      for (const mode of ['wrong-format', 'corrupt', 'gap']) {
        const invalid = structuredClone(source);
        if (mode === 'wrong-format') invalid.format = 'scry-rust-durable-sqlite-v1';
        if (mode === 'corrupt') invalid.app.revival_go[Object.keys(custody)[1]] = Buffer.alloc(48 * 1024, 1).toString('base64');
        if (mode === 'gap') delete invalid.app.revival_go[Object.keys(custody)[0]];
        assert.equal((await restoreTo(fixture.worker, invalid)).status, 422);
        const storage = await fixture.mf.unsafeGetDurableObjectStorage('scry-go-custody-smoke', 'LearningSpace', {name:namespace});
        assert.deepEqual(await storage.exec('SELECT key FROM records'), [], 'Reject invalid custody before ANY state/photo writes.');
        assert.equal((await (await fixture.mf.getR2Bucket('SCRY_ASSETS')).list()).objects.length, 0);
      }
      assert.equal((await restoreTo(fixture.worker, source)).status, 200);
      assert.equal((await restoreTo(fixture.worker, source)).status, 409);
      const activeBindings = {...baseBindings, SPACE_NAME:namespace};
      let active = await fixture.rebind(activeBindings);
      const req = (path, init={}) => active.worker.fetch(`${origin}${path}`, {...init, redirect:'manual',
        headers:{'Cf-Access-Jwt-Assertion':jwt(), ...init.headers}});
      const before = await (await req('/export')).json();
      assert.deepEqual(before.app.revival_go, custody);
      assert.deepEqual(before.app.events, source.app.events);
      assert.deepEqual(before.app.operations, source.app.operations);
      assert.equal((await active.worker.fetch(`${origin}/export`, {headers:{'x-scry-backup-key':backupKey}})).status, 401);
      const fields = await savedRequestFields(await req('/settings'));
      const submit = () => req('/settings', {method:'POST', headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},
        body:new URLSearchParams({...fields, pace:'light'})});
      const saved = await submit();
      assert.equal(saved.status, 303);
      assert.equal((await submit()).headers.get('Location'), saved.headers.get('Location'));
      active = await fixture.rebind(activeBindings);
      const after = await (await req('/export')).json();
      assert.deepEqual(after.app.revival_go, custody);
      assert.deepEqual(after.app.goals,before.app.goals);assert.equal(after.app.preferences.pace,'light');
      const response = await active.worker.fetch(`${origin}/internal/backup`, {method:'POST', headers:{'x-scry-backup-key':backupKey}});
      assert.equal(response.status, 200);
      const state = await (await req('/export')).json();
      const remote = await (await fixture.mf.getR2Bucket('SCRY_BACKUPS')).get(state.app.backup.key);
      const bytes = Buffer.from(await remote.arrayBuffer());
      assert.equal(createHash('sha256').update(bytes).digest('hex'), state.app.backup.sha256);
      const snapshot = JSON.parse(bytes);
      assert.deepEqual(snapshot.app.revival_go, custody);
      assert.deepEqual(Buffer.concat(Object.values(snapshot.app.revival_go).map(chunk => Buffer.from(chunk, 'base64'))), opaque);
      second = await createRuntime({bindings:{...bindings, SPACE_NAME:'synthetic-go-custody-independent'}, name:'scry-go-custody-independent', artifact});
      assert.equal((await restoreTo(second.worker, snapshot)).status, 200);
      const ns = await second.mf.getDurableObjectNamespace('LEARNING_SPACE');
      const stub = ns.get(ns.idFromName('synthetic-go-custody-independent'));
      const recovered = await (await stub.fetch(`${origin}/export`, {headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}})).json();
      assert.deepEqual(recovered.app.revival_go, custody);
      for (const goal of Object.values(recovered.app.goals)) {
        if (goal.photo) {
          assert.notEqual(goal.photo.key, snapshot.app.goals[goal.id].photo.key);
          assert.deepEqual({...goal.photo, key:snapshot.app.goals[goal.id].photo.key}, snapshot.app.goals[goal.id].photo);
          goal.photo.key = snapshot.app.goals[goal.id].photo.key;
        }
      }
      assert.deepEqual(recovered.app.goals, snapshot.app.goals);
      assert.deepEqual(recovered.photos.map(photo => photo.bytes), snapshot.photos.map(photo => photo.bytes));
      assert.deepEqual(recovered.app.operations, snapshot.app.operations);
    } finally {await second?.mf.dispose(); await fixture.mf.dispose();}
  });
  console.log(JSON.stringify({status:failures?'FAIL':'PASS',checks,failures,artifact_sha256:createHash('sha256').update(readFileSync(resolve(artifact,'index_bg.wasm'))).digest('hex'),limits:'Synthetic workerd mechanics and cryptography only; no live ingress, provider quality, physical phone, remote R2 or activation proof.'}));
} finally {await restored?.dispose();await preview.dispose();}

await test('listener allocation survives deterministic contention and concurrent isolated starts',async t=>{
  // Reclaim a probe's released port before workerd starts: the old race,
  // controlled without timing, retries, a global lock or another service.
  const probe=createServer();
  await new Promise((ok,bad)=>{probe.once('error',bad);probe.listen(0,'127.0.0.1',ok);});
  const port=probe.address().port;
  await new Promise(ok=>probe.close(ok));
  const contender=createServer();
  await new Promise((ok,bad)=>{contender.once('error',bad);contender.listen(port,'127.0.0.1',ok);});
  t.after(()=>new Promise(ok=>contender.close(ok)));
  await assert.rejects(createSyntheticRuntime({port,artifact}),{code:'ERR_ADDRESS_IN_USE'});

  // Identical names/bindings must still own distinct listeners and stores.
  const starts=await Promise.allSettled(Array.from({length:4},(_,i)=>
    createSyntheticRuntime({artifact,...(i===0?{port:0}:{})}).then(local=>{t.after(()=>local.mf.dispose());return local;})));
  for(const start of starts)assert.equal(start.status,'fulfilled',start.reason?.stack);
  const runtimes=starts.map(start=>start.value);
  assert.equal(new Set(runtimes.map(local=>local.url)).size,runtimes.length);
  for(const local of runtimes){
    const listener=(await local.mf.ready).origin;
    assert.equal(local.url,listener,'Canonical origin must match the retained native listener.');
    assert.equal(new URL(listener).hostname,'127.0.0.1');
    assert.notEqual(new URL(listener).port,'0');
    assert.notEqual(new URL(listener).port,String(port));
    const response=await fetch(listener+'/create');
    assert.equal(response.status,200);
    const html=await response.text();assert.match(html,/Native content generation is retired/);
    const fields=await savedRequestFields(await fetch(listener+'/settings'));
    assert.ok(fields.csrf);
    const submit=origin=>fetch(listener+'/settings',{method:'POST',redirect:'manual',headers:{Origin:origin},body:new URLSearchParams({...fields,pace:'light'})});
    assert.equal((await submit('https://foreign.invalid')).status,403);
    assert.equal((await submit(runtimes.find(other=>other!==local).url)).status,403);
    assert.equal((await fetch(listener+'/',{headers:{'X-Scry-Test-Host':'foreign.invalid'}})).status,421);
    assert.equal(Object.keys((await(await fetch(listener+'/export')).json()).app.goals).length,0);
    assert.equal((await submit(listener)).status,303,'A rejected write must not consume the operation.');
    assert.equal((await submit(listener)).status,303,'An exact retry must remain idempotent.');
    const saved=(await(await fetch(listener+'/export')).json()).app;assert.equal(saved.preferences.pace,'light');assert.equal(Object.keys(saved.operations).length,1);assert.deepEqual(saved.goals,{});
  }
  console.log(JSON.stringify({listener_allocation:'PASS',reclaimed_probe_port:port,concurrent_origins:runtimes.map(local=>local.url),isolated_receipts_per_runtime:1,foreign_origin_status:403,foreign_host_status:421,canonical_write_and_replay_status:303}));
});
