#!/usr/bin/env node
// Synthetic transport adapter only. The Rust Worker remains responsible for
// routes, CSRF, atomic SQLite state, generation and private R2. All external
// egress is denied by runtime-helper. No operator environment is read.
import {createServer} from 'node:http';
import {parseArgs} from 'node:util';
import {readFile,writeFile} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {createRuntime,internalKey} from '../../qa/runtime-helper.mjs';
import {artifactIdentity} from './lib/worker-artifact.mjs';

const {values}=parseArgs({options:{directory:{type:'string'},port:{type:'string'}}});
const directory=resolve(values.directory);
const port=Number(values.port);
const origin='http://127.0.0.1:'+port;
const manifest=JSON.parse(await readFile(join(directory,'launch.json'),'utf8'));
const identity=await artifactIdentity(manifest.artifact.directory);
if(identity.sha256!==manifest.artifact.sha256)throw Error('Artifact changed before Worker startup');
process.chdir(manifest.artifact.directory);
console.log('Starting isolated workerd with the frozen Rust Worker');
const {mf,worker}=await createRuntime({
  artifact:manifest.artifact.directory,name:'scry-critic-'+manifest.id,
  bindings:{SCRY_ENV:'development',CANONICAL_ORIGIN:origin,SPACE_NAME:'synthetic-critic-only',INTERNAL_KEY:internalKey}
});
let stopping=false;
const server=createServer(async(req,res)=>{
  try {
    const url=new URL(req.url,origin);
    if(req.method==='GET'&&url.pathname==='/__critic/identity') {
      res.writeHead(200,{'Content-Type':'application/json','Cache-Control':'no-store'});
      res.end(JSON.stringify({id:manifest.id,pid:process.pid,token:manifest.attestation,artifact_sha256:identity.sha256,source_sha256:manifest.source_sha256,nonce:url.searchParams.get('nonce')}));
      return;
    }
    const chunks=[];let length=0;
    for await(const chunk of req) {
      length+=chunk.length;
      if(length>5*1024*1024){res.writeHead(413);res.end('Request too large');return;}
      chunks.push(chunk);
    }
    const headers=new Headers();
    for(let i=0;i<req.rawHeaders.length;i+=2)headers.append(req.rawHeaders[i],req.rawHeaders[i+1]);
    // Restore the real incoming synthetic HTTP transport fields after the
    // in-process Miniflare dispatch. Client test headers are never trusted.
    headers.delete('X-Scry-Test-Host');headers.delete('X-Scry-Test-Origin');
    headers.set('X-Scry-Test-Host',req.headers.host||new URL(origin).host);
    if(req.headers.origin)headers.set('X-Scry-Test-Origin',req.headers.origin);
    const response=await worker.fetch(url.href,{method:req.method,headers,redirect:'manual',...(chunks.length?{body:Buffer.concat(chunks)}:{})});
    res.writeHead(response.status,Object.fromEntries(response.headers.entries()));
    res.end(Buffer.from(await response.arrayBuffer()));
  } catch(error) {
    console.error('Synthetic Worker request failed:',error.message);
    if(!res.headersSent)res.writeHead(503,{'Content-Type':'text/plain'});
    res.end('Synthetic Worker unavailable');
  }
});
async function stop() {
  if(stopping)return;stopping=true;
  server.closeAllConnections();
  await new Promise(done=>server.close(done));
  await mf.dispose();
  process.exit(0);
}
process.on('SIGTERM',stop);process.on('SIGINT',stop);
try {
  // Seed through the ordinary CSRF/operation contract, never by assigning state.
  const first=await worker.fetch(origin+'/');
  if(first.status!==200)throw Error('Fresh Worker GET failed: '+first.status);
  const html=await first.text();
  const form=html.match(/<form[^>]+action="\/__fixture"[^>]*>([\s\S]*?)<\/form>/);
  if(!form)throw Error('Fresh synthetic fixture action missing');
  const fields=new URLSearchParams();
  for(const [,name,value]of form[1].matchAll(/<input[^>]+name="([^"]+)"[^>]+value="([^"]*)"[^>]*>/g))fields.set(name,value);
  const seeded=await worker.fetch(origin+'/__fixture',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:fields.toString(),redirect:'manual'});
  if(seeded.status!==303&&seeded.status!==200)throw Error('Synthetic fixture POST failed: '+seeded.status+' '+await seeded.text());
  await new Promise((done,reject)=>{server.once('error',reject);server.listen(port,'127.0.0.1',done);});
  await writeFile(join(directory,'ready.json'),JSON.stringify({pid:process.pid,token:manifest.attestation,artifact_sha256:identity.sha256})+'\n');
  console.log('Synthetic Worker ready at '+origin);
} catch(error) {
  console.error(error.message);await mf.dispose();process.exit(2);
}
