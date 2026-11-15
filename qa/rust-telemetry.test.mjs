// The exact Rust/WASM entrypoint in workerd, real SQLite/R2 and a controlled
// HTTP telemetry sink. No owner, learner store or production capability.
// SPEC § Failure reporting: an unexpected failure at the application boundary
// (a stored state that cannot be loaded or read, a failed alarm or scheduled
// backup) is reported with a fixed category and the deployment identity only.
// An access denial, a refusal and an unhealthy health answer naming a known
// state are ordinary answers, not incidents.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {EventEmitter,once} from 'node:events';
import {performance} from 'node:perf_hooks';
import {generateKeyPairSync,sign} from 'node:crypto';
import test from 'node:test';
import {createRuntime,Response,internalKey} from './runtime-helper.mjs';
import {seedEarlier,storedApp} from './earlier-fixture.mjs';

const origin='http://127.0.0.1:8799';
const version='11111111-2222-4333-8444-555555555555';
const base={SCRY_ENV:'development',CANONICAL_ORIGIN:origin,SPACE_NAME:'synthetic-telemetry-only',INTERNAL_KEY:internalKey,SENTRY_ENVIRONMENT:'synthetic-staging',SCRY_VERSION:{id:version},OWNER_SUBJECT:'PRIVATE-owner-canary'};
const received=[];
let mode='ok';
const sink=createServer(async(request,response)=>{
  let body='';for await(const chunk of request)body+=chunk;
  received.push({body,headers:request.headers});
  if(mode==='hang')await new Promise(resolve=>setTimeout(resolve,3000));
  response.writeHead(mode==='reject'?429:mode==='redirect'?302:200,{'retry-after':'60',location:'https://must-not-follow.invalid/PRIVATE-redirect-canary'});
  response.end('{}');
});
sink.listen(0,'127.0.0.1');await once(sink,'listening');
async function instance(changes={},handleStructuredLogs) {
  received.length=0;
  const bindings=Object.fromEntries(Object.entries({...base,...changes}).filter(([,value])=>value!==undefined));
  return createRuntime({bindings,handleStructuredLogs,name:'scry-rust-telemetry',inspect:true,egress:async request=>{
    assert.equal(request.url,'https://o4510313803677696.ingest.us.sentry.io/api/4510313841426432/envelope/');
    assert.equal(request.method,'POST');
    assert.equal(request.headers.get('content-type'),'application/x-sentry-envelope');
    const response=await fetch(`http://127.0.0.1:${sink.address().port}/envelope`,{method:'POST',body:await request.text()});
    return new Response(await response.text(),{status:response.status,headers:response.headers});
  }});
}
const settle=()=>new Promise(resolve=>setTimeout(resolve,2300));
// A stored photo whose bytes no longer match its record: reading it is an
// unexpected failure (503), while every other page of the notebook still works.
const photoGoal=Object.values(storedApp('history-rich').goals).find(goal=>goal.photo);
const photo=`/photos/${photoGoal.id}`;
async function brokenPhoto(local) {
  const space=base.SPACE_NAME,assets=await local.mf.getR2Bucket('SCRY_ASSETS');
  await seedEarlier(await local.mf.unsafeGetDurableObjectStorage('scry-rust-telemetry','LearningSpace',{name:space}),'history-rich',assets);
  await assets.put(photoGoal.photo.key,'PRIVATE-replaced-photo-canary');
  return local;
}
const brokenInstance=async(changes,logs)=>brokenPhoto(await instance(changes,logs));
async function unexpected(local) {
  const response=await local.worker.fetch(origin+photo+'?PRIVATE-url-canary',{headers:{cookie:'PRIVATE-cookie-canary',authorization:'PRIVATE-secret-canary','x-private':'PRIVATE-header-canary'}});
  assert.equal(response.status,503);assert.equal(await response.text(),'The saved photo failed integrity verification.');
  assert.match(response.headers.get('cache-control'),/no-store/);
}
// The alarm runs now, as the platform runs it at the armed time.
async function alarm(local) {
  const ns=await local.mf.getDurableObjectNamespace('LEARNING_SPACE'),stub=ns.get(ns.idFromName(base.SPACE_NAME));
  try{return await stub.qaAlarm().then(status=>({status}),error=>({rejected:String(error.message).replace(/\s+/g,' ')}));}finally{stub[Symbol.dispose]?.();}
}
// A notebook whose stored state can no longer be loaded.
async function unloadable(local) {
  assert.equal((await local.worker.fetch(origin+'/create')).status,200);
  const sql=await local.mf.unsafeGetDurableObjectStorage('scry-rust-telemetry','LearningSpace',{name:base.SPACE_NAME});
  await sql.exec("UPDATE records SET value='{\"PRIVATE-corrupt-canary\":' WHERE key='meta'");
  return local;
}
function envelope(category,environment='synthetic-staging') {
  assert.equal(received.length,1);
  const raw=received[0].body;
  assert(!raw.includes('PRIVATE-'));assert(!raw.includes(internalKey));
  const [header,item,event]=raw.trimEnd().split('\n').map(JSON.parse);
  assert.deepEqual(item,{type:'event'});assert.equal(header.event_id,event.event_id);
  assert.match(event.event_id,/^[a-f0-9]{32}$/);
  assert.deepEqual(Object.keys(event).sort(),['event_id','timestamp','platform','level','logger','environment','release','message','fingerprint','tags'].sort());
  assert.equal(event.environment,environment);assert.equal(event.release,`scry-rust@${version}`);
  assert.equal(event.logger,'scry.rust');assert.equal(event.message,`Scry Rust ${category}`);
  assert.deepEqual(event.tags,{boundary:'rust-worker',category});
  assert.deepEqual(event.fingerprint,['scry-rust',environment,category]);
}
try {
  await test('native success, ordinary auth/input/host denials and unhealthy health answers produce no telemetry',async()=>{
    const local=await instance();
    try {
      assert.equal((await local.worker.fetch(origin+'/create')).status,200);
      assert.equal((await local.worker.fetch(origin+'/create',{method:'POST',body:'PRIVATE-input-canary',headers:{'X-Scry-Test-Origin':'https://wrong.invalid'}})).status,403);
      assert.equal((await local.worker.fetch(origin+'/',{headers:{'X-Scry-Test-Host':'wrong.invalid'}})).status,421);
      await settle();assert.equal(received.length,0);
      // Health reports a stale backup and an unverified proof itself: no incident.
      const stale=await local.worker.fetch(origin+'/healthz');
      assert.equal(stale.status,503);assert.equal(await stale.text(),'backup_stale');
      const proof=await local.worker.fetch(origin+'/healthz',{headers:{'x-scry-backup-proof':'0'.repeat(64)}});
      assert.equal(proof.status,503);assert.deepEqual(await proof.json(),{backup_proof:false});
      assert.equal((await local.worker.fetch(origin+'/healthz',{headers:{'x-scry-backup-proof':'PRIVATE-bad-proof'}})).status,400);
      await settle();assert.equal(received.length,0);
      const closed=await local.rebind({...base,SCRY_ENV:'production',CANONICAL_ORIGIN:'https://scry-telemetry.invalid'});
      assert.equal((await closed.worker.fetch('https://scry-telemetry.invalid/')).status,401);
      assert.equal((await closed.worker.fetch('https://scry-telemetry.invalid/',{headers:{'Cf-Access-Jwt-Assertion':'PRIVATE-forged-canary'}})).status,401);
      await settle();assert.equal(received.length,0);
    }finally{await local.mf.dispose();}
  });
  await test('an unexpected 503 keeps its exact response, sends one fixed private envelope and caps duplicates',async()=>{
    const local=await brokenInstance();
    try {
      await unexpected(local);
      await settle();envelope('upstream_5xx');
      assert.equal((await local.worker.fetch(origin+photo)).status,503);
      await settle();assert.equal(received.length,1);
      assert.equal((await local.worker.fetch(origin+'/create')).status,200);
    }finally{await local.mf.dispose();}
  });
  await test('health that cannot reach its stored state is an incident, reported as a fixed 5xx',async()=>{
    const local=await instance({INTERNAL_KEY:'',OWNER_SUBJECT:'PRIVATE-owner-canary'});
    try {
      const response=await local.worker.fetch(origin+'/healthz?PRIVATE-url-canary',{headers:{cookie:'PRIVATE-cookie-canary'}});
      assert.equal(response.status,503);assert.equal(await response.text(),'backup_unavailable');
      await settle();envelope('upstream_5xx');
    }finally{await local.mf.dispose();}
  });
  await test('native production grouping is distinct from a controlled staging first-seen alert',async()=>{
    const local=await brokenInstance({SENTRY_ENVIRONMENT:'production'});
    try {
      assert.equal((await local.worker.fetch(origin+photo)).status,503);
      await settle();envelope('upstream_5xx','production');
    }finally{await local.mf.dispose();}
  });
  await test('native Rust error preserves error response and reports only request_exception',async()=>{
    const local=await instance({SPACE_NAME:'PRIVATE-invalid/space-canary'});
    try {
      const response=await local.worker.fetch(origin+'/create?PRIVATE-url-canary');
      assert.equal(response.status,500);assert.equal(await response.text(),'INTERNAL SERVER ERROR');
      await settle();envelope('request_exception');
    }finally{await local.mf.dispose();}
  });
  for(const outage of ['reject','hang','redirect'])await test(`native sink ${outage} cannot delay or alter failure and next study`,async()=>{
    mode=outage;
    const local=await brokenInstance();
    try {
      assert.equal((await local.worker.fetch(origin+'/create')).status,200);
      const start=performance.now();
      const response=await local.worker.fetch(origin+photo);
      assert.equal(response.status,503);assert.equal(await response.text(),'The saved photo failed integrity verification.');
      const study=await local.worker.fetch(origin+'/create');assert.equal(study.status,200);assert.match(await study.text(),/Create/);
      const elapsed=performance.now()-start;
      console.log(JSON.stringify({sink:outage,native_failure_and_next_study_ms:elapsed}));
      assert(elapsed<1000,'Telemetry stalled study');
      await settle();envelope('upstream_5xx');
      assert.equal((await local.worker.fetch(origin+photo)).status,503);
      await settle();assert.equal(received.length,1,'No retry or redirect follows a failed send');
    }finally{await local.mf.dispose();mode='ok';}
  });
  for(const bad of [{SENTRY_ENVIRONMENT:'PRIVATE-invalid-canary'},{SCRY_VERSION:{id:'PRIVATE-invalid-canary'}},{SCRY_VERSION:undefined}])await test('invalid/missing telemetry config leaves native responses unchanged and makes no send',async()=>{
    const local=await brokenInstance(bad);
    try {
      assert.equal((await local.worker.fetch(origin+photo)).status,503);
      assert.equal((await local.worker.fetch(origin+'/create')).status,200);
      await settle();assert.equal(received.length,0);
    }finally{await local.mf.dispose();}
  });
  await test('same-isolate reporting recovers after sink rejection and the real one-minute bound',{timeout:120000},async t=>{
    const reports=new EventEmitter();
    mode='reject';const local=await brokenInstance({},log=>{
      if(log.message==='[scry-rust] telemetry unavailable; report dropped')reports.emit('rejected');
    });
    try {
      const rejected=once(reports,'rejected',{signal:t.signal});
      assert.equal((await local.worker.fetch(origin+photo)).status,503);
      // The drop warning follows 429 backoff processing and response-body cancellation.
      await rejected;envelope('upstream_5xx');
      mode='ok';await new Promise(resolve=>setTimeout(resolve,60000));
      assert.equal(received.length,1,'No automatic retry follows a rejected send');
      assert.equal((await local.worker.fetch(origin+photo)).status,503);await settle();assert.equal(received.length,2);
      assert.equal((await local.worker.fetch(origin+'/create')).status,200);
    }finally{await local.mf.dispose();mode='ok';}
  });
  // SPEC § Failure reporting: the access provider or the network failing to
  // supply its signing keys is an incident; an ordinary denial is not. The
  // answer is the same 401 either way.
  {
    const team='telemetry-synthetic.cloudflareaccess.com',audience='synthetic-telemetry-audience',host='https://scry-telemetry.invalid';
    const {publicKey,privateKey}=generateKeyPairSync('rsa',{modulusLength:2048});
    const jwk={...publicKey.export({format:'jwk'}),kid:'synthetic-key',alg:'RS256',use:'sig'};
    const b64=value=>Buffer.from(JSON.stringify(value)).toString('base64url');
    const assertion=(claims={},header={})=>{const epoch=Math.floor(Date.now()/1000);const encoded=`${b64({alg:'RS256',kid:'synthetic-key',typ:'JWT',...header})}.${b64({iss:`https://${team}`,sub:'PRIVATE-owner-canary',aud:[audience],iat:epoch,nbf:epoch-1,exp:epoch+600,...claims})}`;return `${encoded}.${sign('RSA-SHA256',Buffer.from(encoded),privateKey).toString('base64url')}`;};
    const keys=async(certs)=>{
      received.length=0;
      const bindings={...base,SCRY_ENV:'preview',CANONICAL_ORIGIN:host,ACCESS_TEAM:team,ACCESS_AUDIENCE:audience};
      return createRuntime({bindings,name:'scry-rust-telemetry',inspect:true,egress:async request=>{
        if(request.url===`https://${team}/cdn-cgi/access/certs`)return certs();
        assert.equal(request.url,'https://o4510313803677696.ingest.us.sentry.io/api/4510313841426432/envelope/');
        const response=await fetch(`http://127.0.0.1:${sink.address().port}/envelope`,{method:'POST',body:await request.text()});
        return new Response(await response.text(),{status:response.status,headers:response.headers});
      }});
    };
    const failures={
      network:async()=>{throw new Error('PRIVATE-network-canary');},
      status:async()=>new Response('PRIVATE-status-canary',{status:503}),
      malformed:async()=>new Response('{"keys":[PRIVATE-malformed-canary',{status:200}),
      'not keys':async()=>new Response(JSON.stringify({PRIVATE:'canary'}),{status:200}),
      oversized:async()=>new Response(JSON.stringify({keys:[jwk],padding:'x'.repeat(70*1024)}),{status:200}),
    };
    for(const [failure,certs] of Object.entries(failures))await test(`signing keys the provider cannot supply (${failure}) are reported once; the answer is unchanged`,async()=>{
      const local=await keys(certs);
      try {
        const start=performance.now();
        const page=await local.worker.fetch(host+'/?PRIVATE-url-canary',{headers:{'Cf-Access-Jwt-Assertion':assertion(),cookie:'PRIVATE-cookie-canary'}});
        assert.equal(page.status,401);assert.equal(await page.text(),'Owner authorization required.');
        const api=await local.worker.fetch(host+'/api/v1/studies',{headers:{'Cf-Access-Jwt-Assertion':assertion()}});
        assert.equal(api.status,401);assert.deepEqual(await api.json(),{error:'Owner authorization required.'});
        assert(performance.now()-start<2000,'Reporting never delays the answer');
        await settle();envelope('access_keys_unavailable');
      }finally{await local.mf.dispose();}
    });
    await test('ordinary access denials with readable signing keys report nothing',async()=>{
      let fetched=0;
      const local=await keys(async()=>{fetched++;return new Response(JSON.stringify({keys:[jwk]}),{status:200});});
      try {
        const forged=generateKeyPairSync('rsa',{modulusLength:2048}).privateKey;
        const [h,c]=assertion().split('.');
        for(const token of ['', 'PRIVATE-forged-canary', assertion({exp:1}), assertion({sub:'PRIVATE-other-canary'}), assertion({},{kid:'PRIVATE-unknown-kid'}),
          `${h}.${c}.${sign('RSA-SHA256',Buffer.from(`${h}.${c}`),forged).toString('base64url')}`])
          assert.equal((await local.worker.fetch(host+'/',{headers:{'Cf-Access-Jwt-Assertion':token}})).status,401);
        assert.ok(fetched>=1,'the keys were read');
        assert.equal((await local.worker.fetch(host+'/',{headers:{'Cf-Access-Jwt-Assertion':assertion()}})).status,200);
        await settle();assert.equal(received.length,0);
      }finally{await local.mf.dispose();}
    });
    await test('a hanging sink cannot delay the refusal when the signing keys are unavailable',async()=>{
      mode='hang';
      const local=await keys(failures.network);
      try {
        const start=performance.now();
        assert.equal((await local.worker.fetch(host+'/',{headers:{'Cf-Access-Jwt-Assertion':assertion()}})).status,401);
        assert(performance.now()-start<1000,'Telemetry stalled the answer');
        await settle();envelope('access_keys_unavailable');
      }finally{await local.mf.dispose();mode='ok';}
    });
  }
  await test('a successful alarm reports nothing',async()=>{
    const local=await instance();
    try {
      assert.equal((await local.worker.fetch(origin+'/create')).status,200);
      assert.deepEqual(await alarm(local),{status:200});
      await settle();assert.equal(received.length,0);
    }finally{await local.mf.dispose();}
  });
  await test('a failing alarm sends one fixed private envelope and keeps its outcome',async()=>{
    const local=await unloadable(await instance());
    try {
      const outcome=await alarm(local);
      assert.ok(outcome.rejected,'The alarm still fails as it did');assert.doesNotMatch(outcome.rejected,/telemetry|sentry/i);
      await settle();envelope('alarm_failure');
      // Pages of the unloadable notebook fail as before: one more, separate incident.
      received.length=0;
      assert.equal((await local.worker.fetch(origin+'/create')).status,500);
    }finally{await local.mf.dispose();}
  });
  // The outcome and timing the alarm has with no sink at all.
  let baseline;
  {
    const local=await unloadable(await instance({SENTRY_ENVIRONMENT:''}));
    try {const start=performance.now();baseline={...await alarm(local),ms:performance.now()-start};}finally{await local.mf.dispose();}
  }
  for(const outage of ['reject','hang','redirect'])await test(`alarm: sink ${outage} cannot delay or change the alarm's outcome`,async()=>{
    mode=outage;
    const local=await unloadable(await instance());
    try {
      const start=performance.now();
      const {ms:_,...expected}=baseline;
      const outcome=await alarm(local);
      const elapsed=performance.now()-start;
      console.log(JSON.stringify({sink:outage,alarm_ms:elapsed,baseline_ms:baseline.ms}));
      assert.deepEqual(outcome,expected);assert(elapsed<1000,'Telemetry stalled the alarm');
      await settle();envelope('alarm_failure');
    }finally{await local.mf.dispose();mode='ok';}
  });
  await test('a failing scheduled snapshot sends one fixed private envelope and still fails',async()=>{
    const local=await instance({SPACE_NAME:'PRIVATE-invalid/space-canary'});
    try {
      const start=performance.now();
      const result=await local.worker.scheduled({cron:'17 3 * * *'});
      assert.notEqual(result.outcome,'ok');assert(performance.now()-start<1000);
      await settle();envelope('scheduled_failure');
    }finally{await local.mf.dispose();}
  });
}finally{sink.closeAllConnections();sink.close();}
