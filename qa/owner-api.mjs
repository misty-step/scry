import assert from 'node:assert/strict';
import {generateKeyPairSync,sign,randomUUID} from 'node:crypto';
import {spawn,execFileSync} from 'node:child_process';
import {mkdtemp,readFile,rm,stat} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {createRuntime,internalKey,Response} from './runtime-helper.mjs';

// Each function is the actual US-014 criterion walk, reused by the runtime gate
// and qa/walk. Assertions and output deliberately never render bearer values.
export async function ownerJourney() {
  const name='owner-api-'+randomUUID(), space='synthetic-'+randomUUID();
  const team='owner-token-synthetic.cloudflareaccess.com';
  const origin='https://owner-token-synthetic.invalid';
  const {publicKey,privateKey}=generateKeyPairSync('rsa',{modulusLength:2048});
  const epoch=Math.floor(Date.now()/1000),b64=v=>Buffer.from(JSON.stringify(v)).toString('base64url');
  const unsigned=b64({alg:'RS256',kid:'synthetic'})+'.'+b64({iss:`https://${team}`,sub:'synthetic-owner',aud:['synthetic'],iat:epoch,nbf:epoch-1,exp:epoch+600});
  const jwt=unsigned+'.'+sign('RSA-SHA256',Buffer.from(unsigned),privateKey).toString('base64url');
  const bindings={SCRY_ENV:'preview',CANONICAL_ORIGIN:origin,SPACE_NAME:space,INTERNAL_KEY:internalKey,ACCESS_TEAM:team,ACCESS_AUDIENCE:'synthetic',OWNER_SUBJECT:'synthetic-owner'};
  let runtime=await createRuntime({port:0,name,bindings,inspect:true,egress:async req=>{
    assert.equal(req.url,`https://${team}/cdn-cgi/access/certs`,'No provider or other egress permitted.');
    return new Response(JSON.stringify({keys:[{...publicKey.export({format:'jwk'}),kid:'synthetic',alg:'RS256',use:'sig'}]}));
  }});
  const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
  const temp=await mkdtemp(join(tmpdir(),'scry-owner-cli-'));
  const transcript=[];
  let root,child,revoked,expired,csrf,formFields,creationResponse;
  const access=(path,init={})=>runtime.worker.fetch(origin+path,{redirect:'manual',...init,headers:{'Cf-Access-Jwt-Assertion':jwt,...init.headers}});
  const bearer=(path,token=root?.secret,payload)=>runtime.worker.fetch(origin+path,{redirect:'manual',...(payload?{method:'POST',body:JSON.stringify(payload)}:{}),headers:{...(token?{Authorization:'Bearer '+token}:{}),'Content-Type':'application/json'}});
  const rows=()=>storage.exec('SELECT key,value FROM records ORDER BY key');
  const postForm=(path,fields)=>access(path,{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(fields)});
  const create=async(name,extra={})=>{
    const response=await bearer('/api/v1/tokens',root.secret,{operation_id:randomUUID(),name,...extra});assert.equal(response.status,200);
    const value=await response.json();assert.ok(typeof value.secret==='string','First issuance returns a secret.');return value;
  };
  const criteria=[
    async()=>{
      const html=await(await access('/settings')).text();
      const form=/<form[^>]*action="\/tokens\/create"[^>]*>([\s\S]*?)<\/form>/.exec(html);assert.ok(form,'Access owner has a native token form.');
      formFields=Object.fromEntries([...form[1].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));csrf=formFields.csrf;
      creationResponse=await postForm('/tokens/create',{...formFields,name:'builder'});assert.equal(creationResponse.status,200);
      const minted=await creationResponse.text();const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(minted)?.[1];assert.ok(secret,'Secret appears in first response only.');
      root={secret,token:{id:secret.slice(8,40)}};
      const replay=await postForm('/tokens/create',{...formFields,name:'builder'});assert.equal(replay.status,303);
      const listed=await(await bearer('/api/v1/tokens')).json();root.token=listed.tokens[0];
      assert.equal(root.token.name,'builder');assert.equal(root.token.paid,false);
      assert.ok(root.token.created_ms>0);assert.ok(root.token.last_used_ms>0);assert.equal(root.token.expires_ms,null);
      assert.ok(!JSON.stringify(listed).includes(secret),'Token list never returns secret.');
      assert.ok(!JSON.stringify(await rows()).includes(secret),'Only verifier is persisted.');
      const settings=await(await access('/settings')).text();assert.ok(!settings.includes(secret),'Settings cannot recover secret.');
    },
    async()=>{
      const who=await(await bearer('/api/v1/whoami')).json();assert.equal(who.token.name,'builder');
      child=await create('delegated agent',{expires_ms:Date.now()+600000,paid:true});assert.equal(child.token.paid,true);
      const response=await bearer('/api/v1/tokens',child.secret);assert.equal(response.status,200);
      assert.equal((await response.json()).tokens.length,2);
      const other=await create('rotation');
      assert.equal((await bearer('/api/v1/tokens/revoke',child.secret,{operation_id:randomUUID(),token_id:other.token.id})).status,200);
    },
    async()=>{
      revoked=await create('revoked probe');
      await bearer('/api/v1/tokens/revoke',root.secret,{operation_id:randomUUID(),token_id:revoked.token.id});
      expired=await create('expiry probe',{expires_ms:Date.now()+100000});
      const row=(await storage.exec('SELECT value FROM records WHERE key=?',`owner_tokens/${expired.token.id}`))[0];
      const record=JSON.parse(row.value);
      // Force expiry without sleeps or modifying acknowledged learner content.
      record.expires_ms=Date.now()-1;record.created_ms=Math.min(record.created_ms,record.expires_ms-1);
      await storage.exec('UPDATE records SET value=? WHERE key=?',JSON.stringify(record),`owner_tokens/${expired.token.id}`);
      const unknown='scry_ot_'+'0'.repeat(32)+'_'+'a'.repeat(48);
      const forged=root.secret.slice(0,-1)+(root.secret.endsWith('0')?'1':'0');
      const probes=[undefined,'bad',unknown,forged,revoked.secret,expired.secret];let refusal;
      const before=await rows();
      for(const token of probes) {
        // Explicitly omit header for missing token (avoid the root default).
        const response=await runtime.worker.fetch(origin+'/api/v1/whoami',{headers:token?{Authorization:'Bearer '+token}:{}});
        const observed={status:response.status,body:await response.text()};assert.equal(observed.status,401);
        refusal??=observed;assert.deepEqual(observed,refusal);
        assert.deepEqual(await rows(),before,'Refusal changes no rows.');
      }
      const malformed=await runtime.worker.fetch(origin+'/api/v1/whoami',{headers:{Authorization:'Basic invalid','Cf-Access-Jwt-Assertion':jwt}});
      assert.deepEqual({status:malformed.status,body:await malformed.text()},refusal,'Malformed bearer cannot fall back to an Access session.');
      const oversized=await runtime.worker.fetch(origin+'/api/v1/settings',{method:'POST',headers:{Authorization:'Bearer '+unknown,'Content-Type':'application/json'},body:'x'.repeat(5*1024*1024)});
      assert.deepEqual({status:oversized.status,body:await oversized.text()},refusal,'Authentication precedes body buffering and parsing.');
      assert.deepEqual(await rows(),before);
      // Learner records are unreadable while verifier lookup still works: a bad
      // bearer must return 401, rather than reaching malformed learner JSON.
      const meta=before.find(r=>r.key==='meta').value;
      await storage.exec("UPDATE records SET value='unreadable-learner-state' WHERE key='meta'");
      try {
        for(const token of probes) {
          const response=await runtime.worker.fetch(origin+'/api/v1/settings',{method:'POST',headers:{'Content-Type':'application/json',...(token?{Authorization:'Bearer '+token}:{})},body:JSON.stringify({operation_id:randomUUID(),pace:'light'})});
          assert.equal(response.status,401);assert.deepEqual({status:response.status,body:await response.text()},refusal);
        }
      } finally { await storage.exec("UPDATE records SET value=? WHERE key='meta'",meta); }
    },
    async()=>{
      const value=await create('immediate revocation');
      assert.equal((await bearer('/api/v1/whoami',value.secret)).status,200);
      assert.equal((await bearer('/api/v1/tokens/revoke',root.secret,{operation_id:randomUUID(),token_id:value.token.id})).status,200);
      assert.equal((await bearer('/api/v1/whoami',value.secret)).status,401);
    },
    async()=>{
      const request={operation_id:randomUUID(),pace:'light'};
      const first=await(await bearer('/api/v1/settings',root.secret,request)).json();
      const replay=await(await bearer('/api/v1/settings',root.secret,request)).json();
      assert.deepEqual(replay.result,first.result);assert.equal(replay.replayed,true);
      assert.equal((await bearer('/api/v1/settings',root.secret,{...request,pace:'intense'})).status,409);
      // Browser/API share the same operation receipts and canonical payload.
      assert.equal((await postForm('/settings',{...request,csrf})).status,303);
      assert.equal((await postForm('/settings',{...request,pace:'intense',csrf})).status,409);
      const createPayload={operation_id:randomUUID(),name:'lost response'};
      const issued=await(await bearer('/api/v1/tokens',root.secret,createPayload)).json();
      const again=await(await bearer('/api/v1/tokens',root.secret,createPayload)).json();
      assert.ok(typeof issued.secret==='string');assert.ok(!('secret' in again));assert.equal(again.token.id,issued.token.id);
    },
    async()=>{
      const history=await(await bearer('/api/v1/history')).json();
      const mutations=Object.values(history.operations).filter(r=>r.acting_token===root.token.id);assert.ok(mutations.length>0);
      const exported=await(await bearer('/api/v1/export')).json();assert.ok(Object.values(exported.app.operations).some(r=>r.acting_token===root.token.id));
      assert.ok(!JSON.stringify(exported).includes(root.secret),'Export never contains token secret.');
      const html=await(await access('/history')).text();assert.match(html,/Owner token builder/);
    },
    async()=>{
      const before=await rows();
      const response=await bearer('/api/v1/create',root.secret,{operation_id:randomUUID(),intent:'synthetic paid request'});assert.equal(response.status,403);
      assert.match((await response.json()).error,/paid permission/);assert.deepEqual(await rows(),before,'No learner write or allowance reservation on paid refusal.');
      const exported=await(await bearer('/api/v1/export')).json();assert.deepEqual(exported.app.spend,[]);assert.deepEqual(exported.app.jobs,{});
    },
    async()=>{
      for(const path of ['/tokens/create','/tokens/revoke','/settings','/create']) {
        const denied=await postForm(path,{operation_id:randomUUID(),name:'denied',token_id:child.token.id,pace:'light',intent:'denied'});assert.equal(denied.status,403);
      }
      for(const path of ['/api/v1/tokens','/api/v1/tokens/revoke','/api/v1/settings','/api/v1/create']) {
        const denied=await access(path,{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/json'},body:JSON.stringify({operation_id:randomUUID(),name:'denied',pace:'light'})});assert.equal(denied.status,403);
      }
      const validCookie=await access('/api/v1/settings',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/json'},body:JSON.stringify({operation_id:randomUUID(),pace:'steady',csrf})});assert.equal(validCookie.status,200);
      const cross=await access('/api/v1/tokens',{method:'POST',headers:{'X-Scry-Test-Origin':'https://attacker.invalid','Content-Type':'application/json'},body:'{}'});assert.equal(cross.status,403);
      // Same actual SQLite object, rebound to an isolated loopback environment
      // so the native client drives the real TCP Worker without a proxy/stub.
      const local=(await runtime.mf.ready).origin;
      const next=await runtime.rebind({SCRY_ENV:'development',CANONICAL_ORIGIN:local,SPACE_NAME:space,INTERNAL_KEY:internalKey});runtime={...runtime,...next};
      const binary=resolve('target/debug/scry'),config=join(temp,'login.json');
      const cli=(args,input='')=>new Promise((resolveResult,reject)=>{
        const proc=spawn(binary,[...args,'--json','--config',config],{env:{PATH:process.env.PATH,HOME:temp},stdio:['pipe','pipe','pipe']});
        let out='',err='';proc.stdout.on('data',data=>out+=data);proc.stderr.on('data',data=>err+=data);proc.on('error',reject);
        proc.on('exit',code=>{assert.ok(!out.includes(root.secret)&&!err.includes(root.secret),'CLI never echoes login secret.');try{const value=JSON.parse(out);const safe=structuredClone(value);delete safe.secret;transcript.push({command:['scry',...args,'--json'].join(' '),exit:code,output:safe});resolveResult({code,value,err});}catch{reject(Error('CLI must return JSON.'));}});proc.stdin.end(input);
      });
      assert.equal((await cli(['login','--url',local,'--token-stdin'],root.secret+'\n')).code,0);
      assert.equal((await stat(config)).mode&0o777,0o600);
      const identity=await cli(['whoami']);assert.equal(identity.code,0);assert.equal(identity.value.token.name,'builder');
      const op=randomUUID();const issued=await cli(['token','create','cli agent','--operation-id',op]);assert.equal(issued.code,0);assert.equal(issued.value.token.paid,false);
      const replay=await cli(['token','create','cli agent','--operation-id',op]);assert.equal(replay.code,0);assert.ok(!('secret' in replay.value));
      assert.notEqual((await cli(['token','create','changed payload','--operation-id',op])).code,0);
      assert.equal((await cli(['token','list'])).code,0);
      assert.equal((await cli(['token','revoke',issued.value.token.id])).code,0);
      assert.notEqual((await cli(['login','--url',local,'--token-stdin'],issued.value.secret+'\n')).code,0);
      assert.equal((await cli(['token','revoke',root.token.id])).code,0);
      assert.notEqual((await cli(['whoami'])).code,0);
      assert.notEqual((await cli(['unknown'])).code,0);
      assert.notEqual((await cli(['login','--url','http://external.invalid','--token-stdin'],root.secret)).code,0);
      const stored=JSON.parse(await readFile(config,'utf8'));assert.ok(stored.token===root.secret,'Login persists exactly the supplied token privately.');
    }
  ];
  return {criteria,transcript,close:async()=>{await runtime.mf.dispose();await rm(temp,{recursive:true});}};
}
export function buildCli() { execFileSync('cargo',['build','--locked','--bin','scry'],{stdio:'pipe'}); }
