#!/usr/bin/env node
// Exercises the exact built Rust/WASM in real workerd with isolated SQLite/R2.
// Egress uses synthetic RSA issuer keys and explicitly controlled model
// responses. No account, owner credential, real model, or remote bucket is used.
import assert from 'node:assert/strict';
import { generateKeyPairSync, sign, createHash } from 'node:crypto';
import test from 'node:test';
import {createRuntime,Response,internalKey} from './runtime-helper.mjs';
import { resolve } from 'node:path';
import { readFileSync } from 'node:fs';

const artifact = resolve(process.env.SCRY_WORKER_ARTIFACT || 'build');
const origin = 'https://scry-synthetic.invalid';
const team = 'synthetic.cloudflareaccess.com';
const owner = 'synthetic-owner-only';
const audience = 'synthetic-audience-only';
const operatorKey = 'synthetic-operator-only-key-at-least-32chars';
const restoreToken = 'synthetic-restore-only-key-at-least-32chars';
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
const baseBindings = {SCRY_ENV:'preview',CANONICAL_ORIGIN:origin,SPACE_NAME:'synthetic-only',ACCESS_TEAM:team,ACCESS_AUDIENCE:audience,OWNER_SUBJECT:owner,INTERNAL_KEY:internalKey,OPERATOR_KEY:operatorKey,ALTERNATE_HOSTS:'alternate-synthetic.invalid'};
let checks=0;
const check = (label,fn)=>test(label,async()=>{await fn();checks++;});
let {mf:preview,worker}=await createRuntime({bindings:baseBindings,name:'scry-auth-smoke',artifact,egress});
let restored;
try {
  const request = (path='/', token=jwt(), init={}) => {const headers={'Cf-Access-Jwt-Assertion':token,...init.headers};if(headers.Origin){headers['X-Scry-Test-Origin']=headers.Origin;delete headers.Origin;}return worker.fetch(`${origin}${path}`,{...init,redirect:'manual',headers});};
  await check('real RS256 owner signature and private response headers',async()=>{
    const response=await request();assert.equal(response.status,200);assert.match(await response.text(),/Create/);assert.match(response.headers.get('Cache-Control'),/no-store/);assert.match(response.headers.get('Content-Security-Policy'),/frame-ancestors 'none'/);assert.equal(response.headers.get('Referrer-Policy'),'same-origin');
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
  const html=await (await request('/create')).text();
  const csrf=/name="csrf" value="([^"]+)"/.exec(html)?.[1];assert.ok(csrf);
  const operation=/name="operation_id" value="([^"]+)"/.exec(html)?.[1];assert.ok(operation);
  const revision=/name="revision" value="([^"]+)"/.exec(html)?.[1];assert.ok(revision);
  const fields={csrf,operation_id:operation,revision,intent:'Learn cache validation'};
  const post=(path,body,headers={})=>request(path,jwt(),{method:'POST',body:new URLSearchParams(body),headers:{Origin:origin,'Content-Type':'application/x-www-form-urlencoded',...headers}});
  await check('cross-site Origin, forged CSRF and body-length bounds',async()=>{
    assert.equal((await post('/create',fields,{Origin:'https://attacker.invalid'})).status,403);
    assert.equal((await post('/create',fields,{Origin:'null'})).status,403);
    assert.equal((await post('/create',fields,{'Sec-Fetch-Site':'cross-site'})).status,403);
    assert.equal((await post('/create',{...fields,csrf:'forged'})).status,403);
    assert.equal((await request('/create',jwt(),{method:'POST',body:'x'.repeat(5*1024*1024),headers:{Origin:origin,'Content-Type':'application/x-www-form-urlencoded'}})).status,413);
  });
  let goalPath;
  await check('atomic durable capture and exact operation replay',async()=>{
    const saved=await post('/create',fields);assert.equal(saved.status,303);goalPath=saved.headers.get('Location');assert.match(goalPath,/^\/goals\//);
    const replay=await post('/create',fields);assert.equal(replay.status,303);assert.equal(replay.headers.get('Location'),goalPath);
    const conflict=await post('/create',{...fields,intent:'different'});assert.equal(conflict.status,409);
  });
  await check('no implicit paid request or synthetic fallback without model configuration',async()=>{
    await new Promise(resolve=>setTimeout(resolve,1300));const html=await (await request(goalPath)).text();assert.match(html,/not configured|needs your attention|retry|Try again/i);assert.equal(certRequests,1);
  });
  await check('422 retains escaped no-JavaScript Create draft',async()=>{
    const fresh=await(await request('/create')).text();const current=Object.fromEntries([...fresh.matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));const text='x'.repeat(32769);const response=await post('/create',{...current,intent:text});assert.equal(response.status,422);assert.ok((await response.text()).includes(text));
  });
  const photoBytes=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=','base64');
  await check('photo type fence, durable exact capture and complete asset export',async()=>{
    const fresh=await(await request('/create')).text();const current=Object.fromEntries([...fresh.matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
    const multipart=bytes=>{const form=new FormData();for(const [key,value]of Object.entries({...current,intent:'Understand the image'}))form.set(key,value);form.set('photo',new Blob([bytes],{type:'image/png'}),'synthetic.png');return form;};
    const submit=async bytes=>{const encoded=new globalThis.Request(origin,{method:'POST',body:multipart(bytes)});return request('/create',jwt(),{method:'POST',body:Buffer.from(await encoded.arrayBuffer()),headers:{Origin:origin,'Content-Type':encoded.headers.get('Content-Type')}});};
    const invalid=await submit(Buffer.from('<svg onload="alert(1)">'));assert.equal(invalid.status,422);assert.match(await invalid.text(),/Understand the image/);
    const saved=await submit(photoBytes);assert.equal(saved.status,303);const replay=await submit(photoBytes);assert.equal(replay.status,303);assert.equal(replay.headers.get('Location'),saved.headers.get('Location'));
    const exported=await(await request('/export')).json();assert.equal(Object.keys(exported.app.goals).length,2);assert.equal(exported.photos.length,1);assert.deepEqual(Buffer.from(exported.photos[0].bytes,'base64'),photoBytes);
  });
  let archiveBytes,hash;
  await check('complete R2 backup acknowledged only after exact checksum readback',async()=>{
    const result=await request('/operator/backup',jwt(),{method:'POST',headers:{Origin:origin,'x-scry-operator-key':operatorKey}});assert.equal(result.status,200);
    const bucket=await preview.getR2Bucket('SCRY_BACKUPS');const list=await bucket.list({prefix:'snapshots/synthetic-only/'});assert.ok(list.objects.length>=1);
    const object=await bucket.get(list.objects.at(-1).key);archiveBytes=Buffer.from(await object.arrayBuffer());hash=createHash('sha256').update(archiveBytes).digest('hex');const archive=JSON.parse(archiveBytes);assert.equal(Object.keys(archive.app.goals).length,2);assert.equal(archive.algorithm.startsWith('go-fsrs/v4.0.0'),true);
  });
  let recoverySends=0;
  const recoveryEgress=async request=>{if(request.url===`https://${team}/cdn-cgi/access/certs`)return egress(request);assert.equal(request.url,'https://openrouter.ai/api/v1/chat/completions');recoverySends++;return new Response('Controlled malformed recovery response',{status:200});};
  const restoreRuntime=await createRuntime({bindings:{SCRY_ENV:'restore',CANONICAL_ORIGIN:origin,SPACE_NAME:'fresh-isolated-restore-only',INTERNAL_KEY:internalKey,RESTORE_TOKEN:restoreToken},name:'scry-restore-smoke',artifact,egress:recoveryEgress});
  restored=restoreRuntime.mf;
  const restoreWorker=restoreRuntime.worker;
  const restore=(body,checksum=hash,token=restoreToken)=>restoreWorker.fetch(`${origin}/operator/restore`,{method:'POST',headers:{'x-scry-restore-token':token,'x-scry-sha256':checksum},body});
  await check('restore authorization and archive checksum fail before writes',async()=>{
    assert.equal((await restore(archiveBytes,hash,'wrong')).status,403);assert.equal((await restore(archiveBytes,'0'.repeat(64))).status,422);
    const conflicting=JSON.parse(archiveBytes);const withoutPhoto=Object.values(conflicting.app.goals).find(goal=>!goal.photo);withoutPhoto.photo={...conflicting.photos[0].photo,sha256:'0'.repeat(64)};const inconsistent=Buffer.from(JSON.stringify(conflicting));assert.equal((await restore(inconsistent,createHash('sha256').update(inconsistent).digest('hex'))).status,422);assert.equal((await(await restored.getR2Bucket('SCRY_ASSETS')).list()).objects.length,0,'Conflicting photo metadata must fail before any recovery write.');
  });
  await check('independent complete restore is paused and refuses overwrite',async()=>{
    const response=await restore(archiveBytes);assert.equal(response.status,200);assert.match(await response.text(),/paused/);assert.equal((await restore(archiveBytes)).status,409);
    const ns=await restored.getDurableObjectNamespace('LEARNING_SPACE');const stub=ns.get(ns.idFromName('fresh-isolated-restore-only'));const readback=await stub.fetch(`${origin}/export`,{headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});assert.equal(readback.status,200);const recovered=await readback.json();assert.equal(recovered.app.restored_paused,true);assert.equal(recovered.app.preferences.pace,'steady');assert.equal(Object.keys(recovered.app.goals).length,2);assert.equal(recovered.photos.length,1);assert.deepEqual(Buffer.from(recovered.photos[0].bytes,"base64"),photoBytes);assert.notEqual(recovered.photos[0].photo.key,JSON.parse(archiveBytes).photos[0].photo.key);assert.deepEqual(recovered.app.events,JSON.parse(archiveBytes).app.events);assert.deepEqual(recovered.app.operations,JSON.parse(archiveBytes).app.operations);
  });
  await check('reviewed private recovery resumes only future explicit work',async()=>{
    const activeBindings={...baseBindings,SPACE_NAME:'fresh-isolated-restore-only',OPENROUTER_API_KEY:'synthetic-recovery-generator-key',OPENROUTER_MODEL:'synthetic-recovery-model'};
    const active=await restoreRuntime.rebind(activeBindings);const req=(path,init={})=>active.worker.fetch(`${origin}${path}`,{...init,redirect:'manual',headers:{'Cf-Access-Jwt-Assertion':jwt(),...init.headers}});
    const ns=await restored.getDurableObjectNamespace('LEARNING_SPACE');const stub=ns.get(ns.idFromName(activeBindings.SPACE_NAME));const state=async()=>{const response=await stub.fetch(`${origin}/export`,{headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});return(await response.json()).app;};
    const before=await state();assert.equal(before.restored_paused,true);const oldJobs=structuredClone(before.jobs),oldAssessments=structuredClone(before.assessments);
    const html=await(await req('/create')).text();const fields=Object.fromEntries([...html.matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));const created=await req('/create',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,intent:'A new deliberate request after recovery review'})});assert.equal(created.status,303);const newGoal=created.headers.get('Location').split('/').at(-1);await new Promise(r=>setTimeout(r,1200));assert.equal(recoverySends,0);
    assert.equal((await req('/operator/resume-work',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'x-scry-operator-key':'wrong'}})).status,403);assert.equal((await req('/operator/resume-work',{method:'POST',headers:{'X-Scry-Test-Origin':'https://attacker.invalid','x-scry-operator-key':operatorKey}})).status,403);
    const resumed=await req('/operator/resume-work',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'x-scry-operator-key':operatorKey}});assert.equal(resumed.status,200);assert.equal((await req('/operator/resume-work',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'x-scry-operator-key':operatorKey}})).status,409);
    const deadline=Date.now()+5000;let after;do{after=await state();if(recoverySends&&after.goals[newGoal].status==='failed')break;await new Promise(r=>setTimeout(r,100));}while(Date.now()<deadline);assert.equal(recoverySends,1);assert.equal(after.restored_paused,false);assert.equal(after.goals[newGoal].status,'failed');for(const[id,job]of Object.entries(oldJobs))assert.deepEqual(after.jobs[id],job,'Historical uncertain jobs must not be requeued.');assert.deepEqual(after.assessments,oldAssessments);assert.match(after.spend.at(-1).raw_response,/Controlled malformed recovery response/);
  });
  await check('complete asset capacity rejects capture atomically and removes its orphan',async()=>{
    const localOrigin='http://127.0.0.1:8789';
    const local=await createRuntime({bindings:{SCRY_ENV:'development',CANONICAL_ORIGIN:localOrigin,SPACE_NAME:'synthetic-capacity-only',INTERNAL_KEY:internalKey},name:'scry-capacity-smoke',artifact});
    const bytes=Buffer.concat([photoBytes,Buffer.alloc(4*1024*1024-photoBytes.length)]);
    try {
      for(let i=0;i<3;i++){
        const html=await(await local.worker.fetch(`${localOrigin}/create`)).text();const fields=Object.fromEntries([...html.matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
        const form=new FormData();for(const[key,value]of Object.entries({...fields,intent:`Capacity photo ${i}`}))form.set(key,value);form.set('photo',new Blob([bytes],{type:'image/png'}),`capacity-${i}.png`);
        const encoded=new globalThis.Request(localOrigin,{method:'POST',body:form});const response=await local.worker.fetch(`${localOrigin}/create`,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':localOrigin,'Content-Type':encoded.headers.get('Content-Type')},body:Buffer.from(await encoded.arrayBuffer())});
        assert.equal(response.status,i<2?303:413);if(i===2){const rejected=await response.text();assert.match(rejected,/Capacity photo 2/);assert.match(rejected,/archive exceeds its bounded size limit/);}
      }
      const assets=await local.mf.getR2Bucket('SCRY_ASSETS');assert.equal((await assets.list()).objects.length,2,'The rejected photo must not become an orphan asset.');
      const archive=await(await local.worker.fetch(`${localOrigin}/export`)).json();assert.equal(Object.keys(archive.app.goals).length,2);assert.equal(archive.photos.length,2);assert.equal(Object.keys(archive.app.operations).length,2,'Rejected captures must not commit an exact-retry receipt.');
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
      let html=await(await req('/create')).text();assert.equal((await post('/__fixture',fields(html,'/create'))).status,303);let state=await peek();let occurrence=state.occurrence;const cold=occurrence.concept_id,goal=state.concepts[cold].goal_id,oldExposure=state.concepts[cold].exposure_ms;assert.equal(occurrence.assisted,false);
      if(occurrence.presentation.kind==='choice'){assert.equal((await post(`/questions/${occurrence.question_id}/archive`,{csrf:state.csrf,operation_id:'synthetic-archive-choice-only'})).status,303);await req('/');state=await peek();occurrence=state.occurrence;}assert.equal(occurrence.presentation.kind,'recall');assert.equal(occurrence.assisted,false);
      const other=Object.keys(state.concepts).find(id=>id!==cold);assert.equal((await req(`/concepts/${other}`)).status,200);state=await peek();assert.ok(state.concepts[other].exposure_ms>oldExposure);assert.equal(state.concepts[cold].exposure_ms,oldExposure);
      const active=await fixture.rebind({...baseBindings,SPACE_NAME:space,JEV_API_KEY:'synthetic-exposure-jev-key',JEV_MODEL:'synthetic-jev',JEV_URL:'https://openrouter.ai/api/alpha/decisions'});current=active.worker;currentOrigin=origin;html=await(await req('/')).text();assert.equal((await post('/review/answer',{...fields(html,'/review/answer'),answer:'A deliberately non-exact remembered response'})).status,303);await Promise.race([sending,new Promise((_,reject)=>setTimeout(()=>reject(new Error('Synthetic meaning check did not start.')),5000))]);
      for(const path of [`/concepts/${cold}`,`/goals/${goal}`,`/questions/${occurrence.question_id}/edit`,'/export']){const gated=await(await req(path)).text();assert.match(gated,/data-state="checking-reference"/);assert.doesNotMatch(gated,/A cache stores a response/);assert.doesNotMatch(gated,/action="\/review\/help"/,'Submitted checking answers cannot be retroactively marked helped.');}
      state=await peek();assert.equal(state.occurrence.phase,'checking');assert.equal(state.occurrence.assisted,false);assert.equal(state.concepts[cold].exposure_ms,oldExposure);release();const deadline=Date.now()+5000;do{state=await peek();if(state.occurrence.phase==='result')break;await new Promise(r=>setTimeout(r,100));}while(Date.now()<deadline);assert.equal(state.events.length,1);assert.equal(state.events[0].assisted,false);const event=structuredClone(state.events[0]);await req(`/concepts/${cold}`);state=await peek();assert.ok(state.concepts[cold].exposure_ms>oldExposure);assert.deepEqual(state.events[0],event,'Later reference reads must preserve original answer evidence.');
    } finally {release?.();await fixture.mf.dispose();}
  });
  await check('bounded synthetic generation, critic, Jev, photo, stale-paid and unknown transport lifecycle',async()=>{
    const batch={title:'Conditional cache validation',complete:true,concepts:[{key:'conditional',name:'Conditional validation',summary:'A validator lets the origin confirm saved content.',note:'A stored representation can be validated by sending its ETag. If the validator still matches, a 304 response lets the browser reuse its body without downloading it again.',basis:'general',quotes:[],prerequisites:[],questions:[{prompt:'Which HTTP response status confirms a conditional cache request can reuse its stored representation?',kind:'recall',answer:'304',variants:[],choices:[],explanation:'304 Not Modified confirms that the saved validator still matches the selected representation.',basis:'general',quotes:[]}]}]};
    let instance,mode='normal',blockedStarted,releaseBlocked,transmissions=0;
    const jsonResponse=value=>new Response(JSON.stringify(value),{status:200,headers:{'Content-Type':'application/json'}});
    const provider=async req=>{
      if(req.url===`https://${team}/cdn-cgi/access/certs`)return egress(req);
      assert.ok(['https://openrouter.ai/api/v1/chat/completions','https://openrouter.ai/api/alpha/decisions'].includes(req.url));
      const payload=await req.json();const chat=req.url.endsWith('/chat/completions');
      assert.equal(req.headers.get('Authorization'),`Bearer ${chat?'synthetic-generator-key':'synthetic-jev-key'}`);transmissions++;
      const state=await readState();const reservation=state.app.spend.at(-1);assert.equal(reservation.reserved_micros,500000);assert.equal(reservation.status,'unknown');assert.equal(reservation.cost_micros,null);
      if(chat){
        if(mode==='network-error')throw new Error('Synthetic interrupted provider transport');
        if(mode==='oversize')return new Response('x'.repeat(256*1024+1),{status:200});
        if(mode==='http-error')return new Response('{}',{status:429});
        if(mode==='http-success-shaped')return new Response(JSON.stringify({model:'synthetic-generator',usage:{cost:0.000456},choices:[{message:{content:JSON.stringify(batch)}}]}),{status:429});
        if(mode==='malformed')return new Response('Controlled malformed JSON',{status:200});
        if(mode==='delayed'){blockedStarted();await new Promise(resolve=>{releaseBlocked=resolve;});}
        const parts=payload.messages.at(-1).content;
        if(Array.isArray(parts)){
          assert.match(parts.find(p=>p.type==='text').text,/Understand the image/);
          assert.equal(parts.find(p=>p.type==='image_url').image_url.url,`data:image/png;base64,${photoBytes.toString('base64')}`);
          return jsonResponse({model:'synthetic-generator',usage:{cost:0.000123},choices:[{message:{content:JSON.stringify({text:'ETag validates a representation without repeating its body.'})}}]});
        }
        return jsonResponse({model:'synthetic-generator',usage:{cost:mode==='over-allowance'?2000:0.000123},choices:[{message:{content:JSON.stringify(batch)}}]});
      }
      if(payload.questions.verdict)return jsonResponse({model:'synthetic-jev',usage:{cost:0.000345},answers:{verdict:{choice:'accept',probabilities:{accept:0.99,reject:0.005,unsure:0.005}},identity:{noul:0.01},injection:{noul:0.01}}});
      assert.equal(Object.values(state.app.jobs).some(j=>j.status==='critic-sent'&&j.candidate_json),true,'Candidates must be durably stored before criticism.');
      return jsonResponse({model:'synthetic-jev',usage:{cost:0.000234},answers:Object.fromEntries(Object.keys(payload.questions).map(key=>[key,{type:'noul',noul:0.01}]))});
    };
    const newBindings={...baseBindings,SPACE_NAME:'synthetic-provider-only',OPENROUTER_API_KEY:'synthetic-generator-key',OPENROUTER_MODEL:'synthetic-generator',JEV_API_KEY:'synthetic-jev-key',JEV_MODEL:'synthetic-jev',JEV_URL:'https://openrouter.ai/api/alpha/decisions'};
    instance=await createRuntime({bindings:newBindings,name:'scry-provider-smoke',artifact,egress:provider});
    const ns=await instance.mf.getDurableObjectNamespace('LEARNING_SPACE');const stub=ns.get(ns.idFromName(newBindings.SPACE_NAME));
    async function readState(){const response=await stub.fetch(`${origin}/export`,{headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});assert.equal(response.status,200);return response.json();}
    const req=(path,init={})=>instance.worker.fetch(`${origin}${path}`,{...init,redirect:'manual',headers:{'Cf-Access-Jwt-Assertion':jwt(),...init.headers}});
    const formFields=(html,action)=>{const form=[...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m=>m[1]===action);assert.ok(form,`Missing real form ${action}`);return Object.fromEntries([...form[2].matchAll(/name="([^"]+)" value="([^"]*)"/g)].map(m=>[m[1],m[2]]));};
    const submit=(path,fields)=>req(path,{method:'POST',body:new URLSearchParams(fields),headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'}});
    const wait=async predicate=>{const end=Date.now()+12000;do{const state=await readState();if(predicate(state.app))return state;await new Promise(r=>setTimeout(r,100));}while(Date.now()<end);throw new Error('Synthetic work did not reach its expected durable state.');};
    async function capture(intent){const fields=formFields(await(await req('/create')).text(),'/create');const response=await submit('/create',{...fields,intent});assert.equal(response.status,303);return response.headers.get('Location').split('/').at(-1);}
    try {
      const goal=await capture('Learn conditional caching');
      let prepared=await wait(app=>app.goals[goal].status==='ready');assert.equal(Object.keys(prepared.app.questions).length,1);assert.equal(prepared.app.spend.length,2);assert.equal(prepared.app.spend.every(s=>s.status==='known'&&s.cost_micros>0),true);assert.ok(prepared.app.jobs[prepared.app.goals[goal].job_id].critic_json);
      let html=await(await req('/')).text();assert.match(html,/data-state="intro"/);const intro=formFields(html,'/review/intro');assert.equal((await submit('/review/intro',{...intro,known:'false'})).status,303);
      html=await(await req('/')).text();const answer=formFields(html,'/review/answer');const answered={...answer,answer:'304 Not Modified'};assert.equal((await submit('/review/answer',answered)).status,303);
      let graded=await wait(app=>app.occurrence?.phase==='result');assert.equal(graded.app.events.length,1);assert.equal(graded.app.events[0].result.authority,'jev');const recorded=graded.app.events[0];const sends=transmissions;
      assert.equal((await submit('/review/answer',answered)).status,303);await req('/');graded=await readState();assert.equal(graded.app.events.length,1);assert.deepEqual(graded.app.events[0],recorded);assert.equal(transmissions,sends);
      const fresh=formFields(await(await req('/create')).text(),'/create');const upload=new FormData();for(const[key,value]of Object.entries({...fresh,intent:'Understand the image'}))upload.set(key,value);upload.set('photo',new Blob([photoBytes],{type:'image/png'}),'image.png');const encoded=new globalThis.Request(origin,{method:'POST',body:upload});const photoResponse=await req('/create',{method:'POST',body:Buffer.from(await encoded.arrayBuffer()),headers:{'X-Scry-Test-Origin':origin,'Content-Type':encoded.headers.get('Content-Type')}});assert.equal(photoResponse.status,303);const photoGoal=photoResponse.headers.get('Location').split('/').at(-1);const transcribed=await wait(app=>app.goals[photoGoal].status==='ready');assert.match(transcribed.app.goals[photoGoal].transcript,/ETag/);assert.equal(transcribed.photos.length,1);
      mode='delayed';const started=new Promise(resolve=>{blockedStarted=resolve;});const staleGoal=await capture('A deliberately stale paid preparation');await started;const archivePath=`/goals/${staleGoal}/archive`;const archiveFields=formFields(await(await req(`/goals/${staleGoal}`)).text(),archivePath);assert.equal((await submit(archivePath,archiveFields)).status,303);releaseBlocked();const stale=await wait(app=>app.jobs[app.goals[staleGoal].job_id].raw_response!==null);const staleJob=stale.app.jobs[stale.app.goals[staleGoal].job_id];assert.equal(staleJob.status,'superseded');assert.ok(staleJob.raw_response);const charge=stale.app.spend.find(s=>s.work_id===staleJob.id);assert.equal(charge.status,'known');assert.ok(charge.cost_micros>0);assert.equal(stale.app.goals[staleGoal].concept_ids.length,0);
      for(const failure of ['network-error','oversize','http-error','malformed','http-success-shaped']){mode=failure;const failedGoal=await capture(`Controlled ${failure} request`);const failed=await wait(app=>['unknown','failed'].includes(app.goals[failedGoal].status));const failedJob=failed.app.jobs[failed.app.goals[failedGoal].job_id];const paid=failed.app.spend.find(s=>s.work_id===failedJob.id);assert.equal(paid.status,failure==='http-success-shaped'?'known':'unknown');assert.equal(paid.cost_micros,failure==='http-success-shaped'?456:null);assert.equal(paid.reserved_micros,500000);assert.equal(failed.app.goals[failedGoal].concept_ids.length,0,'Rejected or malformed transport must not publish any content.');if(['http-error','malformed','http-success-shaped'].includes(failure)){assert.ok(paid.raw_response);assert.equal(paid.response_hash,createHash('sha256').update(paid.raw_response).digest('hex'));assert.equal(paid.response_bytes,Buffer.byteLength(paid.raw_response));assert.equal(paid.response_outcome,failure==='malformed'?'received':'rejected');}const count=transmissions;await new Promise(r=>setTimeout(r,1200));assert.equal(transmissions,count,'Unknown paid requests must never resend automatically.');}
      mode='over-allowance';const expensive=await capture('Controlled measured expense beyond the daily allowance');const over=await wait(app=>app.goals[expensive].status==='failed');const expensiveJob=over.app.jobs[over.app.goals[expensive].job_id];const measured=over.app.spend.find(s=>s.work_id===expensiveJob.id);assert.equal(measured.status,'known');assert.equal(measured.cost_micros,2_000_000_000);assert.ok(expensiveJob.raw_response);assert.ok(expensiveJob.candidate_json);const stopped=transmissions;const blocked=await capture('Save this intent after the measured allowance is exhausted');await wait(app=>app.goals[blocked].status==='failed');assert.equal(transmissions,stopped,'Measured over-budget expense must stop further reservations and sends.');
    } finally {releaseBlocked?.();await instance.mf.dispose();}
  });
  console.log(JSON.stringify({status:checks===18?'PASS':'FAIL',checks,artifact_sha256:createHash('sha256').update(readFileSync(resolve(artifact,'index_bg.wasm'))).digest('hex'),limits:'Synthetic workerd mechanics and cryptography only; no live ingress, provider quality, physical phone, remote R2 or activation proof.'}));
} finally {await restored?.dispose();await preview.dispose();}
