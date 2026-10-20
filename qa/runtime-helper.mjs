// Exact compiled Rust/WASM in disposable workerd + SQLite/R2. No account or
// remote capabilities are inherited by this explicit test configuration.
import {createRequire} from 'node:module';
import {resolve} from 'node:path';
import {readFileSync} from 'node:fs';
const require=createRequire(import.meta.url);
let runtime;
try {runtime=require('miniflare');}
catch {
  if(!process.env.SCRY_WRANGLER_PACKAGE)throw new Error('Install pinned dev dependencies before running workerd QA.');
  runtime=createRequire(resolve(process.env.SCRY_WRANGLER_PACKAGE))('miniflare');
}
export const Response=runtime.Response;
export const denyEgress=async()=>{throw new Error('External calls are forbidden in synthetic QA.');};
export const internalKey='synthetic-internal-private-storage-key-32chars';
export async function createRuntime({bindings,name='scry-synthetic-qa',artifact=process.env.SCRY_WORKER_ARTIFACT||'build',egress=denyEgress,port,inspect=false,resourcePersistencePath}) {
  artifact=resolve(artifact);
  // Miniflare replaces Host with its own listener. This transport-only adapter
  // restores the simulated external Host before Rust enforces its real checks.
  // worker-rs exports a non-RPC class. Only inspection fixtures wrap it in the
  // native base required by Miniflare's SQL inspection; fetch/alarm stay Rust.
  const space=inspect
    ? `import {DurableObject} from 'cloudflare:workers';export class LearningSpace extends DurableObject{constructor(ctx,env){super(ctx,env);this.inner=new RustLearningSpace(ctx,env);}fetch(request){return this.inner.fetch(request);}alarm(){return this.inner.alarm();}}`
    : `export {RustLearningSpace as LearningSpace};`;
  // Like Wrangler's drain-body middleware, discard unread transport bytes only
  // after Rust responds. Otherwise workerd can reset the upload and erase its
  // early refusal (cloudflare/workerd#7634). Never buffer them before auth.
  const adapter=`import App,{LearningSpace as RustLearningSpace} from './index.js';${space}export default class extends App{async fetch(request){const headers=new Headers(request.headers);headers.set('Host',headers.get('X-Scry-Test-Host')||new URL(request.url).host);headers.delete('X-Scry-Test-Host');if(headers.has('X-Scry-Test-Origin')){headers.set('Origin',headers.get('X-Scry-Test-Origin'));headers.delete('X-Scry-Test-Origin');}
    const forwarded=new Request(request,{headers});
    const response=await super.fetch(forwarded);
    response.headers.set('X-Scry-Test-Body-Used',String(forwarded.bodyUsed));
    if(forwarded.body!==null&&!forwarded.bodyUsed){
      const reader=forwarded.body.getReader();
      while(!(await reader.read()).done){}
    }
    return response;
  }}`;
  const modules=[
    {type:'ESModule',path:resolve(artifact,'smoke-adapter.js'),contents:adapter},
    {type:'ESModule',path:resolve(artifact,'index.js'),contents:readFileSync(resolve(artifact,'index.js'),'utf8')},
    {type:'CompiledWasm',path:resolve(artifact,'index_bg.wasm'),contents:readFileSync(resolve(artifact,'index_bg.wasm'))}
  ];
  const v4={name,modules,modulesRoot:artifact,cf:false,compatibilityDate:'2026-09-10',durableObjects:{LEARNING_SPACE:{className:'LearningSpace',useSQLite:true}},r2Buckets:['SCRY_ASSETS','SCRY_BACKUPS'],bindings,outboundService:egress,...(port!==undefined?{host:'127.0.0.1',port}:{}),...(resourcePersistencePath?{resourcePersistencePath}:{})};
  const options=runtime.convertV4MiniflareOptions?runtime.convertV4MiniflareOptions(v4):v4;
  if(inspect)options.unsafeInspectDurableObjects=true;
  const mf=new runtime.Miniflare(options);
  try {await mf.ready;return {mf,worker:await mf.getWorker(),url:bindings.CANONICAL_ORIGIN,rebind:async nextBindings=>{const next={...v4,bindings:nextBindings};const options=runtime.convertV4MiniflareOptions?runtime.convertV4MiniflareOptions(next):next;if(inspect)options.unsafeInspectDurableObjects=true;await mf.setOptions(options);return {worker:await mf.getWorker(),url:nextBindings.CANONICAL_ORIGIN};}};}
  catch(error){await mf.dispose();throw error;}
}
export async function createSyntheticRuntime({port=0,name='scry-browser-synthetic',artifact}={}) {
  const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:`http://127.0.0.1:${port}`,SPACE_NAME:'synthetic-browser-only',INTERNAL_KEY:internalKey};
  const local=await createRuntime({port,name,artifact,bindings});
  // Let workerd own port 0, then configure its actual canonical origin.
  // Miniflare retains that listener when updating bindings with port:0.
  try {return {...local,...await local.rebind({...bindings,CANONICAL_ORIGIN:(await local.mf.ready).origin})};}
  catch(error){await local.mf.dispose();throw error;}
}
