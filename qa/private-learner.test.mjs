#!/usr/bin/env node
// Isolated synthetic JWTs, SQLite and local R2 only. No real identity/config.
import assert from 'node:assert/strict';
import test from 'node:test';
import {generateKeyPairSync,sign,createHash,randomUUID} from 'node:crypto';
import {createRuntime,Response,internalKey} from './runtime-helper.mjs';
import {nativeFields} from './nato-starter.mjs';
function selectedFields(html,action,key,value){
 const body=html.split('<form').find(part=>part.startsWith(` method="post" action="${action}"`)&&part.split('</form>')[0].includes(`name="${key}" value="${value}"`));
 assert.ok(body,`Native ${action} ${key}=${value} form`);return nativeFields('<form'+body,action);
}

const origin='https://private-learner.invalid',team='synthetic-learner.cloudflareaccess.com';
const owner='synthetic-owner',learner='synthetic-learner',audience='synthetic-audience';
const space='existing-owner-space';
const learnerSpace=`scry-learner-${createHash('sha256').update(learner).digest('hex')}`;
const {publicKey,privateKey}=generateKeyPairSync('rsa',{modulusLength:2048});
const jwk={...publicKey.export({format:'jwk'}),kid:'test-key',alg:'RS256',use:'sig'};
const b64=value=>Buffer.from(JSON.stringify(value)).toString('base64url');
const epoch=Math.floor(Date.now()/1000);
function jwt(changes={},key=privateKey){
 const header=b64({alg:'RS256',kid:'test-key'});
 const claims=b64({iss:`https://${team}`,sub:learner,aud:[audience],iat:epoch,nbf:epoch-1,exp:epoch+600,...changes});
 const body=`${header}.${claims}`;return `${body}.${sign('RSA-SHA256',Buffer.from(body),key).toString('base64url')}`;
}
const bindings={SCRY_ENV:'preview',CANONICAL_ORIGIN:origin,SPACE_NAME:space,ACCESS_TEAM:team,ACCESS_AUDIENCE:audience,OWNER_SUBJECT:owner,LEARNER_SUBJECT:learner,INTERNAL_KEY:internalKey,QA_EMAIL:'qa@example.invalid'};
const egress=async req=>{assert.equal(req.url,`https://${team}/cdn-cgi/access/certs`);return new Response(JSON.stringify({keys:[jwk]}),{headers:{'Content-Type':'application/json'}});};

await test('configured learner has a private persisted notebook without owner capabilities',async()=>{
 const name='private-learner-isolation';const fixture=await createRuntime({bindings,name,egress,inspect:true});let active=fixture;
 const req=(path='/',token=jwt(),init={})=>active.worker.fetch(origin+path,{...init,redirect:'manual',headers:{'Cf-Access-Jwt-Assertion':token,...init.headers}});
 const fields=html=>Object.fromEntries([...html.matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
 const post=(path,values,token=jwt())=>req(path,token,{method:'POST',body:new URLSearchParams({...values,operation_id:randomUUID()}),headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'}});
 const rows=async target=>{const db=await fixture.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:target});return await db.exec('SELECT key,value FROM records ORDER BY key');};
 try {
  assert.equal((await req('/settings',jwt({sub:owner}))).status,200);
  const ownerBefore=await rows(space);
  const ownerCsrf=JSON.parse(ownerBefore.find(row=>row.key==='meta').value).csrf;
  const home=await req();assert.equal(home.status,200);const html=await home.text();
  const settings=await req('/settings');assert.equal(settings.status,200);const settingsHtml=await settings.text();
  assert.match(settingsHtml,/private to your sign-in/);assert.match(settingsHtml,/href="\/cdn-cgi\/access\/logout"/);
  assert.doesNotMatch(settingsHtml,/Owner tokens|\/tokens\/create|Usage allowance/);
  const ownExport=await req('/export?space='+space);assert.equal(ownExport.status,200);
  assert.notEqual((await ownExport.json()).app.csrf,ownerCsrf,'Client query cannot select the owner archive.');
  for(const path of ['/operator/restore','/operator/resume-work','/tokens/create','/tokens/revoke','/internal/backup','/api/v1/works','/api/v1/lessons/start','/api/v1/lessons/next']){
   assert.equal((await post(path,{})).status,403,path);
  }
  assert.equal((await req('/api/v1/works',jwt(),{headers:{Authorization:'Bearer forged'}})).status,401);
  assert.equal((await req('/__internal/daily')).status,404);
  assert.equal((await post('/practice/nato/start',fields(html))).status,303);
  assert.ok((await rows(learnerSpace)).some(row=>row.key.startsWith('practice_studies/')));
  // Real member browser forms use their own native grading/history, never the owner API.
  const notebook=async()=>Object.fromEntries((await rows(learnerSpace)).map(row=>[row.key,JSON.parse(row.value)]));
  const page=async path=>{const response=await req(path);assert.equal(response.status,200);return response.text();};
  const startLesson=async starter=>{
   const shelf=await page('/lessons');
   assert.equal((await post('/practice/lesson/start',selectedFields(shelf,'/practice/lesson/start','starter',starter))).status,303);
  };
  await startLesson('st-michael-en-lasance-1911');
  const prayer=await page('/'),beforePrayer=await notebook(),prompt=beforePrayer.meta.practice.current.prompt;
  assert.equal(prompt.scope.kind,'unit');assert.equal(prompt.assisted,true);
  const version=beforePrayer['library_versions/'+prompt.version_id];
  const line=version.sections.flatMap(section=>section.units)[0].text;
  assert.equal((await post('/practice/answer',{...nativeFields(prayer,'/practice/answer'),answer:line})).status,303);
  assert.match(await page('/'),/No unaided credit or schedule change/);
  const afterPrayer=await notebook();
  for(const [key,card] of Object.entries(beforePrayer).filter(([key])=>key.startsWith('practice_cards/'))){
   assert.deepEqual(afterPrayer[key].state.schedule,card.state.schedule);
  }
  const attempts=Object.entries(afterPrayer).filter(([key])=>key.startsWith('practice_attempts/'));
  assert.equal(attempts.length,1,'Member answer produces its own persisted evidence.');
  await startLesson('rosary-joyful');
  const sequence=await page('/');
  assert.equal((await post('/practice/answer',{...nativeFields(sequence,'/practice/answer'),answer:'The Annunciation\nVisitation\nBirth of Jesus\nPresentation\nFinding of Jesus in the Temple'})).status,303);
  assert.match(await page('/'),/data-verdict="Good"/);
  assert.equal(Object.keys(await notebook()).filter(key=>key.startsWith('practice_attempts/')).length,2);
  const learnerRows=await rows(learnerSpace);
  assert.deepEqual(await rows(space),ownerBefore,'Learner import cannot touch owner history.');
  const ownerSpoof=await req('/settings',jwt({sub:owner}),{headers:{'x-scry-owner':'private-learner'}});
  assert.equal(ownerSpoof.status,200);assert.match(await ownerSpoof.text(),/Owner tokens/);
  const learnerSpoof=await req('/settings',jwt(),{headers:{'x-scry-owner':'private-owner'}});
  assert.equal(learnerSpoof.status,200);assert.doesNotMatch(await learnerSpoof.text(),/Owner tokens/);
  assert.match(await (await req('/settings?__private_learner=true',jwt({sub:owner}))).text(),/Owner tokens/);
  assert.doesNotMatch(await (await req('/settings?__private_learner=false')).text(),/Owner tokens/);
  const badKey=generateKeyPairSync('rsa',{modulusLength:2048}).privateKey;
  for(const token of [jwt({sub:'other'}),jwt({sub:''}),jwt({sub:null}),jwt({sub:'',common_name:learner}),jwt({aud:['wrong']}),jwt({iss:'https://wrong.invalid'}),jwt({exp:epoch-1}),jwt({nbf:epoch+600}),jwt({},badKey)]){
   assert.equal((await req('/',token)).status,401);
  }
  // Rebind disposes/recreates runtime instances; identity must not depend on a prior request.
  active=await fixture.rebind({...bindings});assert.equal((await req('/')).status,200);
  assert.deepEqual(await rows(learnerSpace),learnerRows,'Restart preserves private content and held prompt.');
  active=await fixture.rebind({...bindings,LEARNER_SUBJECT:''});
  assert.equal((await req()).status,401);assert.equal((await req('/settings',jwt({sub:owner}))).status,200);
  assert.deepEqual(await rows(learnerSpace),learnerRows,'Revocation never deletes learner history.');
  active=await fixture.rebind({...bindings,LEARNER_SUBJECT:owner});
  assert.equal((await req()).status,401);assert.equal((await req('/settings',jwt({sub:owner}))).status,200);
  active=await fixture.rebind({...bindings,SPACE_NAME:learnerSpace});assert.equal((await req()).status,401);
  active=await fixture.rebind({...bindings});
  const qa=await req('/settings',jwt({sub:learner,email:bindings.QA_EMAIL}));
  assert.equal(qa.status,200);assert.match(await qa.text(),/Owner tokens/,'Existing exact QA mapping retains its isolated QA role, never private learner.');
  assert.deepEqual(await rows(learnerSpace),learnerRows);
 }finally{await fixture.mf.dispose();}
});
