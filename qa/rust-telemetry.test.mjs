// The exact Rust/WASM entrypoint in workerd, real SQLite/R2 and a controlled
// HTTP telemetry sink. No owner, learner store or production capability.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {EventEmitter,once} from 'node:events';
import {performance} from 'node:perf_hooks';
import test from 'node:test';
import {createRuntime,Response,internalKey} from './runtime-helper.mjs';

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
  return createRuntime({bindings,handleStructuredLogs,name:'scry-rust-telemetry',egress:async request=>{
    assert.equal(request.url,'https://o4510313803677696.ingest.us.sentry.io/api/4510313841426432/envelope/');
    assert.equal(request.method,'POST');
    assert.equal(request.headers.get('content-type'),'application/x-sentry-envelope');
    const response=await fetch(`http://127.0.0.1:${sink.address().port}/envelope`,{method:'POST',body:await request.text()});
    return new Response(await response.text(),{status:response.status,headers:response.headers});
  }});
}
const settle=()=>new Promise(resolve=>setTimeout(resolve,2300));
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
  await test('native success and ordinary auth/input/host denials produce no telemetry',async()=>{
    const local=await instance();
    try {
      assert.equal((await local.worker.fetch(origin+'/create')).status,200);
      assert.equal((await local.worker.fetch(origin+'/create',{method:'POST',body:'PRIVATE-input-canary',headers:{'X-Scry-Test-Origin':'https://wrong.invalid'}})).status,403);
      assert.equal((await local.worker.fetch(origin+'/',{headers:{'X-Scry-Test-Host':'wrong.invalid'}})).status,421);
      await settle();assert.equal(received.length,0);
      const closed=await local.rebind({...base,SCRY_ENV:'production',CANONICAL_ORIGIN:'https://scry-telemetry.invalid'});
      assert.equal((await closed.worker.fetch('https://scry-telemetry.invalid/')).status,401);
      assert.equal((await closed.worker.fetch('https://scry-telemetry.invalid/',{headers:{'Cf-Access-Jwt-Assertion':'PRIVATE-forged-canary'}})).status,401);
      await settle();assert.equal(received.length,0);
    }finally{await local.mf.dispose();}
  });
  await test('native 503 keeps exact response, sends one fixed private envelope and caps duplicates',async()=>{
    const local=await instance();
    try {
      const response=await local.worker.fetch(origin+'/healthz?PRIVATE-url-canary',{headers:{cookie:'PRIVATE-cookie-canary',authorization:'PRIVATE-secret-canary','x-private':'PRIVATE-header-canary'}});
      assert.equal(response.status,503);assert.equal(await response.text(),'backup_stale');
      assert.match(response.headers.get('cache-control'),/no-store/);
      await settle();envelope('upstream_5xx');
      assert.equal((await local.worker.fetch(origin+'/healthz')).status,503);
      await settle();assert.equal(received.length,1);
      assert.equal((await local.worker.fetch(origin+'/create')).status,200);
    }finally{await local.mf.dispose();}
  });
  await test('native production grouping is distinct from a controlled staging first-seen alert',async()=>{
    const local=await instance({SENTRY_ENVIRONMENT:'production'});
    try {
      assert.equal((await local.worker.fetch(origin+'/healthz')).status,503);
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
    const local=await instance();
    try {
      assert.equal((await local.worker.fetch(origin+'/create')).status,200);
      const start=performance.now();
      const response=await local.worker.fetch(origin+'/healthz');
      assert.equal(response.status,503);assert.equal(await response.text(),'backup_stale');
      const study=await local.worker.fetch(origin+'/create');assert.equal(study.status,200);assert.match(await study.text(),/Create/);
      const elapsed=performance.now()-start;
      assert(elapsed<1000,'Telemetry stalled study');
      console.log(JSON.stringify({sink:outage,native_failure_and_next_study_ms:elapsed}));
      await settle();envelope('upstream_5xx');
      assert.equal((await local.worker.fetch(origin+'/healthz')).status,503);
      await settle();assert.equal(received.length,1,'No retry or redirect follows a failed send');
    }finally{await local.mf.dispose();mode='ok';}
  });
  for(const bad of [{SENTRY_ENVIRONMENT:'PRIVATE-invalid-canary'},{SCRY_VERSION:{id:'PRIVATE-invalid-canary'}},{SCRY_VERSION:undefined}])await test('invalid/missing telemetry config leaves native responses unchanged and makes no send',async()=>{
    const local=await instance(bad);
    try {
      assert.equal((await local.worker.fetch(origin+'/healthz')).status,503);
      assert.equal((await local.worker.fetch(origin+'/create')).status,200);
      await settle();assert.equal(received.length,0);
    }finally{await local.mf.dispose();}
  });
  await test('same-isolate reporting recovers after sink rejection and the real one-minute bound',{timeout:120000},async t=>{
    const reports=new EventEmitter();
    mode='reject';const local=await instance({},log=>{
      if(log.message==='[scry-rust] telemetry unavailable; report dropped')reports.emit('rejected');
    });
    try {
      const rejected=once(reports,'rejected',{signal:t.signal});
      assert.equal((await local.worker.fetch(origin+'/healthz')).status,503);
      // The drop warning follows 429 backoff processing and response-body cancellation.
      await rejected;envelope('upstream_5xx');
      mode='ok';await new Promise(resolve=>setTimeout(resolve,60000));
      assert.equal(received.length,1,'No automatic retry follows a rejected send');
      assert.equal((await local.worker.fetch(origin+'/healthz')).status,503);await settle();assert.equal(received.length,2);
      assert.equal((await local.worker.fetch(origin+'/create')).status,200);
    }finally{await local.mf.dispose();mode='ok';}
  });
}finally{sink.closeAllConnections();sink.close();}
