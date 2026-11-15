// Custody on the exact built Worker in workerd with isolated SQLite/R2:
// storage capacity from the browser and the API, a failed backup in Settings,
// the verification space and backups, cross-site reads and the expiry an Owner
// token may grant. SPEC § Backup and recovery, § Privacy, § Owner tokens.
// Synthetic RSA issuer keys only; no account, provider or remote bucket.
import assert from 'node:assert/strict';
import {generateKeyPairSync,sign,randomUUID,createHash} from 'node:crypto';
import test from 'node:test';
import {resolve} from 'node:path';
import {createRuntime,Response,internalKey} from './runtime-helper.mjs';
import {historicalMutation} from './historical-fixture.mjs';

const artifact=resolve(process.env.SCRY_WORKER_ARTIFACT||'build');
const origin='https://scry-custody-synthetic.invalid';
const team='custody-synthetic.cloudflareaccess.com',audience='synthetic-custody-audience',owner='synthetic-custody-owner';
const qaEmail='custody-qa@synthetic.invalid';
const MAX=16*1024*1024;
const CAPACITY='Storage is full. This would make the notebook too large for one complete backup, so nothing was saved.';
const {publicKey,privateKey}=generateKeyPairSync('rsa',{modulusLength:2048});
const publicJwk={...publicKey.export({format:'jwk'}),kid:'synthetic-key',alg:'RS256',use:'sig'};
const b64=value=>Buffer.from(JSON.stringify(value)).toString('base64url');
function jwt(changes={}){
  const epoch=Math.floor(Date.now()/1000);
  const encoded=`${b64({alg:'RS256',kid:'synthetic-key',typ:'JWT'})}.${b64({iss:`https://${team}`,sub:owner,aud:[audience],iat:epoch,nbf:epoch-1,exp:epoch+600,...changes})}`;
  return `${encoded}.${sign('RSA-SHA256',Buffer.from(encoded),privateKey).toString('base64url')}`;
}
const qa=()=>jwt({sub:'synthetic-custody-qa-person',email:qaEmail});
const unescape=text=>text.replaceAll('&quot;','"').replaceAll('&#39;',"'").replaceAll('&lt;','<').replaceAll('&gt;','>').replaceAll('&amp;','&');
// Everything a page shows outside closed native disclosures.
const outsideDisclosures=html=>html.replace(/<details[\s\S]*?<\/details>/g,'');

async function notebook({traceObjectFetch=false}={}){
  const name='custody-'+randomUUID(),space='synthetic-custody-'+randomUUID();
  const egress=async request=>{
    assert.equal(request.url,`https://${team}/cdn-cgi/access/certs`,'Only synthetic public keys may be fetched.');
    return new Response(JSON.stringify({keys:[publicJwk]}),{headers:{'Content-Type':'application/json'}});
  };
  const bindings={SCRY_ENV:'preview',CANONICAL_ORIGIN:origin,SPACE_NAME:space,ACCESS_TEAM:team,ACCESS_AUDIENCE:audience,OWNER_SUBJECT:owner,INTERNAL_KEY:internalKey,QA_EMAIL:qaEmail};
  const runtime=await createRuntime({bindings,name,artifact,egress,inspect:true,traceObjectFetch});
  const n={runtime,space,name};
  n.req=(path,{token=jwt,headers={},...init}={})=>runtime.worker.fetch(origin+path,{redirect:'manual',...init,headers:{'Cf-Access-Jwt-Assertion':token(),...headers}});
  n.post=(path,fields,{token=jwt,headers={}}={})=>n.req(path,{token,method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded',...headers},body:new URLSearchParams(fields)});
  n.page=async(path,token=jwt)=>{const response=await n.req(path,{token});assert.equal(response.status,200,`GET ${path}: ${response.status}`);return response.text();};
  n.form=(html,action)=>{
    const found=[...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m=>m[1]===action);
    return found&&Object.fromEntries([...found[2].matchAll(/<input type="hidden" name="([^"]+)" value="([^"]*)">/g)].map(m=>[m[1],m[2]]));
  };
  n.submit=async(path,action,more={},token=jwt)=>{
    const fields=n.form(await n.page(path,token),action);assert.ok(fields,`${path} offers ${action}`);
    return n.post(action,{...fields,...more},{token});
  };
  n.sql=(which=space)=>runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:which});
  n.rows=async(which=space)=>(await n.sql(which)).exec('SELECT key,value FROM records ORDER BY key');
  n.meta=async(which=space)=>JSON.parse((await n.rows(which)).find(r=>r.key==='meta').value);
  n.backups=async()=>(await (await runtime.mf.getR2Bucket('SCRY_BACKUPS')).list()).objects.map(o=>o.key);
  n.alarm=async which=>{const ns=await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE'),stub=ns.get(ns.idFromName(which));try{return {status:await stub.qaAlarm(),next:await stub.qaAlarmTime()};}finally{stub[Symbol.dispose]?.();}};
  n.api=(path,secret,payload)=>runtime.worker.fetch(origin+path,{redirect:'manual',...(payload?{method:'POST',body:JSON.stringify(payload)}:{}),headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'}});
  n.token=async(fields={})=>{
    const response=await n.submit('/settings','/tokens/create',{name:'custody agent',...fields});assert.equal(response.status,200);
    const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await response.text())?.[1]?.trim();assert.ok(secret);return secret;
  };
  // The complete archive's measured size: the export's bytes, with the
  // widest creation time the capacity rule allows for.
  n.size=async()=>{const response=await n.req('/export');assert.equal(response.status,200,'export');return (await response.arrayBuffer()).byteLength+'-9223372036854775808'.length-String(Date.now()).length;};
  // Test-owned receipts, each within one row's bound, bring the notebook to
  // exactly MAX - room bytes, as the Worker capacity tests fill it.
  n.fill=async room=>{
    const sql=await n.sql();
    const put=(i,pad)=>sql.exec('INSERT INTO records(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value',`operations/custody-capacity-${String(i).padStart(4,'0')}`,
      JSON.stringify({acting_token:null,path:null,created_ms:null,payload_hash:'a'.repeat(64),result:{redirect:'/',entity_id:null,message:null,data:{synthetic:'x'.repeat(pad)}}}));
    const start=await n.size();
    await put(0,900000);const one=await n.size()-start;
    let i=1;for(let total=start+one;MAX-room-total>one+50000;total+=one)await put(i++,900000);
    const before=await n.size();await put(i,1000);const overhead=await n.size()-before-1000;
    await put(i,MAX-room-before-overhead);
    assert.equal(await n.size(),MAX-room,'filled to the exact margin');
  };
  return n;
}

// NATO practice held at its first letter's source study, with its Quiz form.
async function heldNato(n){
  assert.equal((await n.submit('/?practice=nato','/practice/nato/start')).status,303);
}
async function quiz(n){
  assert.equal((await n.submit('/?practice=nato','/practice/quiz')).status,303);
  return n.form(await n.page('/?practice=nato'),'/practice/answer');
}

test('an answer past capacity is refused by one rule from the browser and the API, and a backup still succeeds',async()=>{
  const n=await notebook();
  try{
    const secret=await n.token();
    await heldNato(n);
    // Room for the Quiz and a backup request with its status, not an answer.
    await n.fill(2500);
    const answer=await quiz(n);
    const before=await n.rows();
    const typed='CAPACITY_TYPED_ALFA';
    const refused=await n.post('/practice/answer',{...answer,answer:typed});
    assert.equal(refused.status,413);
    const html=await refused.text();
    assert.ok(unescape(html).includes(CAPACITY),'the plain capacity refusal is shown');
    assert.ok(html.includes(typed),'the typed answer stays on the page');
    assert.deepEqual(await n.rows(),before,'a refused answer writes nothing');
    // The API answers by the same rule, with the same words.
    const api=await n.api('/api/v1/answer',secret,{operation_id:'custody-api-answer-'+randomUUID(),prompt_id:answer.prompt_id,answer:typed});
    assert.equal(api.status,413);assert.deepEqual(await api.json(),{error:CAPACITY});
    assert.deepEqual(await n.rows(),before,'a refused API answer writes nothing');
    // A backup still fits and is verified.
    const backup=await n.submit('/settings','/settings/backup');
    assert.equal(backup.status,303);assert.equal(backup.headers.get('Location'),'/settings');
    const meta=await n.meta();
    assert.equal(meta.backup.error,null);assert.ok(meta.backup.completed_ms);
    assert.equal((await n.backups()).filter(key=>key.startsWith(`snapshots/${n.space}/`)).length,1);
    assert.equal((await n.req('/healthz',{token:()=>''})).status,200);
  }finally{await n.runtime.mf.dispose();}
});

test('a notebook past capacity stays readable, says so, keeps everything and shows a failed backup',async()=>{
  const n=await notebook();
  try{
    const secret=await n.token();
    await heldNato(n);
    await n.fill(0);
    // Paid work and test-owned rows can leave a notebook past the bound.
    await (await n.sql()).exec('INSERT INTO records(key,value) VALUES(?,?)','operations/custody-capacity-over',
      JSON.stringify({acting_token:null,path:null,created_ms:null,payload_hash:'a'.repeat(64),result:{redirect:'/',entity_id:null,message:null,data:{synthetic:'x'.repeat(100000)}}}));
    const keys=(await n.rows()).map(r=>r.key);
    for(const path of ['/','/?practice=nato','/history','/library'])assert.equal((await n.req(path)).status,200,path);
    const settings=await n.page('/settings');
    assert.match(outsideDisclosures(settings),/<p data-capacity="full"><strong>Storage is full\.<\/strong>/);
    const exported=await n.req('/export');
    assert.equal(exported.status,413);assert.match(await exported.text(),/^Storage is full: the notebook is too large for one complete export\. Nothing has been deleted/);
    const apiExport=await n.api('/api/v1/export',secret);assert.equal(apiExport.status,413);
    assert.equal((await n.api('/api/v1/history',secret)).status,200,'history stays readable');
    assert.equal((await n.api('/api/v1/studies',secret)).status,200);
    // Asking for a backup adds to the notebook, so it is refused like any
    // growth, with Settings and its reason, and nothing is written.
    const rows=await n.rows();
    const request=await n.submit('/settings','/settings/backup');
    assert.equal(request.status,413);
    const refusedPage=unescape(await request.text());
    assert.ok(refusedPage.includes(CAPACITY));assert.match(refusedPage,/data-state="settings"/);
    assert.deepEqual(await n.rows(),rows);
    // The daily backup fails visibly in Settings; study goes on.
    assert.equal((await n.alarm(n.space)).status,200);
    const failed=outsideDisclosures(await n.page('/settings'));
    assert.match(failed,/<p data-backup="failed"><strong>The last backup failed\.<\/strong> No verified backup exists\.<\/p>/);
    assert.ok((await n.meta()).backup.error);
    assert.deepEqual((await n.backups()).filter(key=>key.startsWith(`snapshots/${n.space}/`)),[],'nothing is advertised as a backup');
    assert.equal((await n.req('/?practice=nato')).status,200,'study stays available');
    const after=(await n.rows()).map(r=>r.key);
    for(const key of keys)assert.ok(after.includes(key),`${key} was kept`);
    assert.equal((await n.req('/healthz',{token:()=>''})).status,503);
  }finally{await n.runtime.mf.dispose();}
});

test('a failed backup request returns Settings showing the failure beside the last verified backup',async()=>{
  const n=await notebook();
  try{
    const csrf=n.form(await n.page('/settings'),'/settings/backup').csrf;
    // A saved photo, as the retired capture stored one.
    const photo=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=','base64');
    const hash=createHash('sha256').update(photo).digest('hex');
    const record={key:`photos/${n.space}/custody-photo-${hash}`,mime:'image/png',size:photo.length,sha256:hash};
    await historicalMutation(await n.sql(),{path:'/create',fields:{csrf,operation_id:'custody-historical-photo',intent:'Synthetic saved photo',__photo:JSON.stringify(record)},now_ms:Date.now(),seed:'custody-historical-photo'});
    const assets=await n.runtime.mf.getR2Bucket('SCRY_ASSETS');await assets.put(record.key,photo);
    assert.equal((await n.submit('/settings','/settings/backup')).status,303);
    const verified=(await n.meta()).backup;assert.equal(verified.error,null);assert.ok(verified.completed_ms);
    assert.doesNotMatch(await n.page('/settings'),/data-backup="failed"/);
    // The photo goes missing: the next backup cannot be complete.
    await assets.delete(record.key);
    const failed=await n.submit('/settings','/settings/backup');
    assert.equal(failed.status,303,'a failed backup is not a raw error');
    assert.equal(failed.headers.get('Location'),'/settings');
    const meta=await n.meta();
    assert.ok(meta.backup.error);assert.equal(meta.backup.completed_ms,verified.completed_ms);assert.equal(meta.backup.key,verified.key);
    const at=new Date(verified.completed_ms),pad=v=>String(v).padStart(2,'0');
    const date=`${['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'][at.getUTCMonth()]} ${at.getUTCDate()}, ${at.getUTCFullYear()} · ${pad(at.getUTCHours())}:${pad(at.getUTCMinutes())} UTC`;
    const shown=outsideDisclosures(await n.page('/settings'));
    assert.ok(shown.includes(`<p data-backup="failed"><strong>The last backup failed.</strong> Last verified backup: ${date}.</p>`),shown);
    assert.equal((await n.backups()).length,1,'only the verified snapshot exists');
    assert.equal((await n.req('/')).status,200,'study stays available');
  }finally{await n.runtime.mf.dispose();}
});

test('the verification space never writes, reads or prunes backups; the Owner is unaffected',async()=>{
  const n=await notebook();
  try{
    const settings=await n.page('/settings',qa);
    assert.match(settings,/<p data-backup="not-applicable">Backups do not apply to this verification space\.<\/p>/);
    assert.doesNotMatch(settings,/action="\/settings\/backup"/);
    // A crafted request is refused before anything is written.
    const fields=n.form(settings,'/tokens/create');
    const before=await n.rows('scry-qa');
    const refused=await n.post('/settings/backup',{csrf:fields.csrf,revision:fields.revision,operation_id:'custody-qa-backup-'+randomUUID()},{token:qa});
    assert.equal(refused.status,403);
    const refusedPage=unescape(await refused.text());
    assert.match(refusedPage,/Backups do not apply to this verification space\. Nothing was saved\./);
    assert.match(refusedPage,/data-state="settings"/);assert.match(refusedPage,/data-backup="not-applicable"/);
    assert.deepEqual(await n.rows('scry-qa'),before);
    // Its alarm takes no daily backup and arms nothing for one.
    const alarm=await n.alarm('scry-qa');
    assert.equal(alarm.status,200);assert.equal(alarm.next,null);
    assert.deepEqual(await n.backups(),[],'nothing in the backup bucket');
    assert.deepEqual(await n.rows('scry-qa'),before);
    // The Owner's space backs up and is healthy as before.
    assert.equal((await n.req('/healthz',{token:()=>''})).status,503);
    assert.equal((await n.submit('/settings','/settings/backup')).status,303);
    assert.deepEqual((await n.backups()).map(key=>key.split('/').slice(0,2).join('/')),[`snapshots/${n.space}`]);
    assert.equal((await n.req('/healthz',{token:()=>''})).status,200);
    assert.ok(n.form(await n.page('/settings'),'/settings/backup'));
    assert.equal((await n.alarm('scry-qa')).status,200);
    assert.equal((await n.backups()).length,1);
    assert.equal((await n.req('/healthz',{token:()=>''})).status,200);
  }finally{await n.runtime.mf.dispose();}
});

test('a cross-site read is served only as a top-level navigation, before any state is read',async()=>{
  const n=await notebook({traceObjectFetch:true});
  try{
    const sites=[undefined,'same-origin','same-site','none','cross-site'];
    const modes=[undefined,'navigate','no-cors','cors'];
    const dests=[undefined,'document','image','iframe','empty','script'];
    // Node's fetch rewrites Sec-Fetch-Mode; the test adapter restores this one.
    const metadata=(site,mode,dest)=>({'X-Scry-Test-Fetch-Mode':mode??'',...Object.fromEntries([['Sec-Fetch-Site',site],['Sec-Fetch-Dest',dest]].filter(([,v])=>v!==undefined))});
    const refusedCombination=(site,mode,dest)=>site==='cross-site'&&!(mode==='navigate'&&dest==='document');
    const sentinel=await n.runtime.mf.getR2Bucket('SCRY_ASSETS');
    // Before any page is read the notebook does not exist; refusals keep it so.
    for(const path of ['/settings','/export','/photos/'+'0'.repeat(32),'/api/v1/studies'])for(const method of ['GET','HEAD'])
      for(const mode of modes)for(const dest of dests){
        if(!refusedCombination('cross-site',mode,dest))continue;
        const response=await n.req(path,{method,headers:metadata('cross-site',mode,dest)});
        assert.equal(response.status,403,`${method} ${path} ${mode}/${dest}`);
        assert.match(response.headers.get('Cache-Control'),/no-store/);
      }
    assert.equal(await sentinel.get('__qa_scope_do_contact'),null,'a refused read never reaches the notebook');
    // The sign-in redirect back from the access provider: a cross-site
    // top-level navigation with a fresh assertion.
    assert.equal((await n.req('/',{headers:metadata('cross-site','navigate','document')})).status,200);
    const expected={'/settings':200,'/export':200,['/photos/'+'0'.repeat(32)]:404,'/api/v1/studies':200};
    for(const [path,status] of Object.entries(expected))for(const method of ['GET','HEAD'])
      for(const site of sites)for(const mode of modes)for(const dest of dests){
        const response=await n.req(path,{method,headers:metadata(site,mode,dest)});
        assert.equal(response.status,refusedCombination(site,mode,dest)?403:status,`${method} ${path} ${site}/${mode}/${dest}`);
      }
    // A bearer API read without fetch metadata, as the CLI sends it, is unchanged.
    const secret=await n.token();
    assert.equal((await n.api('/api/v1/studies',secret)).status,200);
    const crossBearer=await n.runtime.worker.fetch(origin+'/api/v1/studies',{headers:{Authorization:'Bearer '+secret,'Sec-Fetch-Site':'cross-site','Sec-Fetch-Dest':'empty'}});
    assert.equal(crossBearer.status,403);
    // Writes keep their existing cross-site refusal.
    const fields=n.form(await n.page('/settings'),'/settings/backup');
    assert.equal((await n.post('/settings/backup',fields,{headers:{'X-Scry-Test-Origin':'https://elsewhere.invalid'}})).status,403);
  }finally{await n.runtime.mf.dispose();}
});

test('an Owner token cannot create a token that outlives it',async()=>{
  const n=await notebook();
  try{
    // The signed-in Owner sets any expiry, or none.
    const lasting=await n.token();
    const limitDate=new Date(Date.now()+2*24*60*60_000);limitDate.setUTCSeconds(0,0);
    const expiring=await n.token({expires_at:limitDate.toISOString().slice(0,16)});
    const limit=limitDate.getTime();
    for(const expires of [undefined,limit+60_000]){
      const before=await n.rows();
      const response=await n.api('/api/v1/tokens',expiring,{operation_id:'custody-child-'+randomUUID(),name:'child',...(expires===undefined?{}:{expires_ms:expires})});
      assert.equal(response.status,403,String(expires));
      assert.deepEqual(await response.json(),{error:'This Owner token expires, so a token it creates must expire no later than it does. Nothing was created.'});
      assert.deepEqual(await n.rows(),before,'a refusal writes nothing');
    }
    for(const expires of [limit,limit-60_000]){
      const response=await n.api('/api/v1/tokens',expiring,{operation_id:'custody-child-'+randomUUID(),name:'child',expires_ms:expires});
      assert.equal(response.status,200);assert.equal((await response.json()).token.expires_ms,expires);
    }
    const unbounded=await n.api('/api/v1/tokens',lasting,{operation_id:'custody-child-'+randomUUID(),name:'unbounded child'});
    assert.equal(unbounded.status,200);assert.equal((await unbounded.json()).token.expires_ms,null);
  }finally{await n.runtime.mf.dispose();}
});

test('a restore leaves retired generation jobs saved as queued and sent byte for byte',async()=>{
  const n=await notebook();
  const restoreToken='synthetic-custody-restore-key-at-least-32chars';
  try{
    const csrf=n.form(await n.page('/settings'),'/settings/backup').csrf;
    const sql=await n.sql(),at=Date.now();
    for(const [i,intent] of ['First historical request','Second historical request'].entries())
      await historicalMutation(sql,{path:'/create',fields:{csrf,operation_id:`custody-historical-${i}-request`,intent},now_ms:at+i,seed:`custody-historical-${i}`});
    const claimed=await historicalMutation(sql,{actions:[{kind:'claim',now_ms:at+5,lease:'custody-historical-generation-lease',generation:true,jev:false,critic:false}],now_ms:at+5,seed:'custody-historical-claim'});
    assert.ok(claimed.work);
    const sourceRows=(await n.rows()).filter(r=>r.key.startsWith('jobs/'));
    const statuses=sourceRows.map(r=>JSON.parse(r.value).status).sort();
    assert.deepEqual(statuses,['queued','sent']);
    const exported=await n.req('/export');assert.equal(exported.status,200);
    const bytes=Buffer.from(await exported.arrayBuffer());
    const name='custody-restore-'+randomUUID(),space='synthetic-custody-restored';
    const target=await createRuntime({bindings:{SCRY_ENV:'restore',CANONICAL_ORIGIN:origin,SPACE_NAME:space,INTERNAL_KEY:internalKey,RESTORE_TOKEN:restoreToken},name,artifact,inspect:true});
    try{
      const restored=await target.worker.fetch(origin+'/operator/restore',{method:'POST',body:bytes,headers:{'x-scry-restore-token':restoreToken,'x-scry-sha256':createHash('sha256').update(bytes).digest('hex')}});
      assert.equal(restored.status,200,await restored.clone().text());
      const rows=await (await target.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space})).exec('SELECT key,value FROM records ORDER BY key');
      assert.equal(JSON.parse(rows.find(r=>r.key==='meta').value).restored_paused,true,'the restored notebook is paused');
      assert.deepEqual(rows.filter(r=>r.key.startsWith('jobs/')),sourceRows,'every retired generation job is stored byte for byte');
      // And its complete export carries them unchanged.
      const ns=await target.mf.getDurableObjectNamespace('LEARNING_SPACE'),stub=ns.get(ns.idFromName(space));
      try{
        const readback=await stub.fetch(origin+'/export',{headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});
        assert.equal(readback.status,200);
        assert.equal(JSON.stringify((await readback.json()).app.jobs),JSON.stringify(JSON.parse(bytes).app.jobs));
      }finally{stub[Symbol.dispose]?.();}
    }finally{await target.mf.dispose();}
  }finally{await n.runtime.mf.dispose();}
});
