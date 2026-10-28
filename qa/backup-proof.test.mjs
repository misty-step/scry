// Existing /healthz, exact native Worker, synthetic SQLite/R2 only; no egress.
import assert from 'node:assert/strict';
import test from 'node:test';
import {createRequire} from 'node:module';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {internalKey} from './runtime-helper.mjs';
const require = createRequire(import.meta.url);
let runtime;
try {runtime = require('miniflare');} catch {runtime = createRequire(resolve(process.env.SCRY_WRANGLER_PACKAGE))('miniflare');}
const artifact = resolve(process.env.SCRY_WORKER_ARTIFACT || 'build');
const DAY = 24 * 60 * 60_000;
const hash = 'a'.repeat(64);
const origin = 'https://synthetic-backup.invalid';
const space = 'synthetic-backup-proof';
const adapter = `import App,{LearningSpace as Native} from './index.js';import {DurableObject} from 'cloudflare:workers';
export class LearningSpace extends DurableObject {
 constructor(ctx,env) {
  super(ctx,env);this.control={};this.reads=0;this.alarmReads=0;this.fetches=0;
  const wrap=(target,override)=>new Proxy(target,{get:(t,k)=>k in override?override[k]:(typeof Reflect.get(t,k,t)==='function'?Reflect.get(t,k,t).bind(t):Reflect.get(t,k,t))});
  const sql=wrap(ctx.storage.sql,{exec:(...args)=>{this.reads++;return ctx.storage.sql.exec(...args);}});
  const storage=wrap(ctx.storage,{sql,getAlarm:async(...args)=>{
    this.alarmReads++;const actual=await ctx.storage.getAlarm(...args);
    if(this.control.throwAlarm)throw Error('SYNTHETIC_PRIVATE-alarm-error');
    if(this.control.race||this.control.raceBackup){const m=JSON.parse(ctx.storage.sql.exec("SELECT value FROM records WHERE key='meta'").toArray()[0].value);if(this.control.race)m.revision++;else m.backup.error='SYNTHETIC_PRIVATE-race';ctx.storage.sql.exec("UPDATE records SET value=? WHERE key='meta'",JSON.stringify(m));}
    return this.control.alarmOverride===undefined?actual:this.control.alarmOverride;
  }});
  this.native=new Native(wrap(ctx,{storage}),env);
 }
 fetch(request){this.fetches++;return this.native.fetch(request);}
 alarm(){return this.native.alarm();}
 async fixture(input={}) {
  this.control=input.control||{};
  if(input.meta!==undefined){this.ctx.storage.sql.exec('CREATE TABLE IF NOT EXISTS records (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID');this.ctx.storage.sql.exec("INSERT INTO records(key,value) VALUES('meta',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",JSON.stringify(input.meta));}
  if(input.raw!==undefined)this.ctx.storage.sql.exec("UPDATE records SET value=? WHERE key='meta'",input.raw);
  if(input.alarm===null)await this.ctx.storage.deleteAlarm();else if(input.alarm!==undefined)await this.ctx.storage.setAlarm(input.alarm);
  this.reads=0;this.alarmReads=0;this.fetches=0;
 }
 state(){const tables=this.ctx.storage.sql.exec("SELECT name FROM sqlite_master WHERE type='table' AND name='records'").toArray();return{rows:tables.length?this.ctx.storage.sql.exec('SELECT key,value FROM records ORDER BY key').toArray():[],reads:this.reads,alarmReads:this.alarmReads,fetches:this.fetches};}
}
// Miniflare's host RPC drops empty-valued headers; restore this synthetic
// empty-header case before Rust, just as Host is restored by runtime-helper.
export default class extends App{fetch(req){const headers=new Headers(req.headers);headers.set('Host',new URL(req.url).host);if(headers.has('x-fixture-empty-proof')){headers.set('x-scry-backup-proof','');headers.delete('x-fixture-empty-proof');}return super.fetch(new Request(req,{headers}));}}`;
async function fixture(fn) {
  let egress = 0;
  const options = {name:'backup-proof',compatibilityDate:'2026-09-10',cf:false,modulesRoot:artifact,
    modules:[{type:'ESModule',path:resolve(artifact,'proof-fixture.mjs'),contents:adapter},
      {type:'ESModule',path:resolve(artifact,'index.js'),contents:readFileSync(resolve(artifact,'index.js'),'utf8')},
      {type:'CompiledWasm',path:resolve(artifact,'index_bg.wasm'),contents:readFileSync(resolve(artifact,'index_bg.wasm'))}],
    bindings:{SCRY_ENV:'development',CANONICAL_ORIGIN:origin,SPACE_NAME:space,INTERNAL_KEY:internalKey},
    durableObjects:{LEARNING_SPACE:{className:'LearningSpace',useSQLite:true}},r2Buckets:['SCRY_ASSETS','SCRY_BACKUPS'],
    outboundService:async()=>{egress++;throw Error('Synthetic egress forbidden');}};
  const mf = new runtime.Miniflare(runtime.convertV4MiniflareOptions?runtime.convertV4MiniflareOptions(options):options);
  try {
    const worker = await mf.getWorker();const ns = await mf.getDurableObjectNamespace('LEARNING_SPACE');
    const stub = ns.get(ns.idFromName(space));
    const request = (proof=hash, path='/healthz') => worker.fetch(origin+path,{headers:proof===undefined?{}:proof===''?{'x-fixture-empty-proof':'1'}:{'x-scry-backup-proof':proof}});
    const result = async proof => {const response=await request(proof);const text=await response.text();assert.doesNotMatch(text,/SYNTHETIC_PRIVATE|snapshots\/|[a-f0-9]{64}/);assert.match(response.headers.get('Cache-Control'),/no-store/);return{status:response.status,body:JSON.parse(text)};};
    const completed = Date.now()-1000;
    const meta = {revision:42,backup:{completed_ms:completed,key:'snapshots/SYNTHETIC_PRIVATE',sha256:hash,error:null}};
    await fn({mf,worker,stub,ns,request,result,completed,meta});assert.equal(egress,0);
  } finally {await mf.dispose();}
}
test('matching snapshot status and a persisted next-day alarm produce only a true verdict',()=>fixture(async({stub,result,meta,completed})=>{
  await stub.fixture({meta,alarm:completed+DAY});const before=(await stub.state()).rows;
  assert.deepEqual(await result(hash),{status:200,body:{backup_proof:true}});
  const after=await stub.state();assert.deepEqual(after.rows,before);assert.equal(after.alarmReads,1);
}));
test('invalid proof header fails before object/storage access; missing state stays absent',()=>fixture(async({stub,result})=>{
  for(const bad of ['', 'A'.repeat(64),'a'.repeat(63),'a'.repeat(65),'x'.repeat(64),'a'.repeat(32)+','+'b'.repeat(31)]){
    await stub.fixture();assert.deepEqual(await result(bad),{status:400,body:{backup_proof:false}});
    const state=await stub.state();assert.equal(state.fetches,0);assert.equal(state.reads,0);assert.equal(state.alarmReads,0);assert.deepEqual(state.rows,[]);
  }
  assert.deepEqual(await result(hash),{status:503,body:{backup_proof:false}});assert.deepEqual((await stub.state()).rows,[]);
}));
test('proof refuses wrong hash, error, future/stale status, missing/past/late alarm and corrupt metadata',()=>fixture(async({stub,result,meta,completed})=>{
  for(const update of [{sha256:'b'.repeat(64)},{error:'SYNTHETIC_PRIVATE'},{completed_ms:Date.now()+DAY},{completed_ms:Date.now()-27*60*60_000},{completed_ms:null}]){
    await stub.fixture({meta:{...meta,backup:{...meta.backup,...update}},alarm:completed+DAY});const before=(await stub.state()).rows;
    assert.deepEqual(await result(hash),{status:503,body:{backup_proof:false}});assert.deepEqual((await stub.state()).rows,before);
  }
  for(const alarm of [null,completed+DAY+1]){
    await stub.fixture({meta,alarm});const before=(await stub.state()).rows;
    assert.deepEqual(await result(hash),{status:503,body:{backup_proof:false}});assert.deepEqual((await stub.state()).rows,before);
  }
  for(const control of [{alarmOverride:Date.now()-1},{throwAlarm:true}]){
    await stub.fixture({meta,alarm:completed+DAY,control});const before=(await stub.state()).rows;
    assert.deepEqual(await result(hash),{status:503,body:{backup_proof:false}});assert.deepEqual((await stub.state()).rows,before);
  }
  await stub.fixture({raw:'SYNTHETIC_PRIVATE-corrupt'});assert.deepEqual(await result(hash),{status:503,body:{backup_proof:false}});
}));
test('revision change while awaiting persisted alarm fails closed without overwriting it',()=>fixture(async({stub,result,meta,completed})=>{
  for(const control of [{race:true},{raceBackup:true}]){
    await stub.fixture({meta,alarm:completed+DAY,control});
    assert.deepEqual(await result(hash),{status:503,body:{backup_proof:false}});
    const after=await stub.state();const saved=JSON.parse(after.rows[0].value);
    assert.equal(saved.revision,control.race?43:42);
    assert.equal(saved.backup.error,control.raceBackup?'SYNTHETIC_PRIVATE-race':null);assert.equal(after.alarmReads,1);
  }
}));
test('direct internal path remains404 and QA object cannot inspect owner backup proof',()=>fixture(async({stub,ns,worker})=>{
  const response=await worker.fetch(origin+'/__internal/health',{headers:{'x-scry-backup-proof':hash}});assert.equal(response.status,404);
  for(const headers of [{},{'x-scry-internal-key':'invalid','x-scry-owner':'private-owner'},{'x-scry-internal-key':internalKey,'x-scry-owner':'private-qa'}]){
    const denied=await stub.fetch('https://internal.invalid/__internal/health',{headers:{...headers,'x-scry-backup-proof':hash}});
    assert.equal(denied.status,403);assert.equal((await stub.state()).reads,0);
  }
  const qa=ns.get(ns.idFromName('scry-qa'));
  const denied=await qa.fetch('https://internal.invalid/__internal/health',{headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-qa','x-scry-backup-proof':hash}});
  assert.equal(denied.status,403);assert.deepEqual(await denied.json(),{backup_proof:false});assert.equal((await qa.state()).reads,0);assert.equal((await stub.state()).reads,0);
}));
test('without proof header existing native backup health semantics remain unchanged',()=>fixture(async({stub,worker})=>{
  let response=await worker.fetch(origin+'/healthz');assert.equal(response.status,503);assert.equal(await response.text(),'backup_stale');
  const daily=await stub.fetch('https://internal.invalid/__internal/daily',{method:'POST',headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});assert.equal(daily.status,200);
  response=await worker.fetch(origin+'/healthz');assert.equal(response.status,200);assert.equal(await response.text(),'ok');
  const state=await stub.state();const backup=JSON.parse(state.rows.find(row=>row.key==='meta').value).backup;
  response=await worker.fetch(origin+'/healthz',{headers:{'x-scry-backup-proof':backup.sha256}});assert.equal(response.status,200);assert.deepEqual(await response.json(),{backup_proof:true});
}));
