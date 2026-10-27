// Frozen published predecessor, not a rebuild of old source. The host download
// is bounded and checksum-pinned; synthetic Workers still deny every egress.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {createRequire} from 'node:module';
import {mkdtemp,readFile,writeFile,mkdir,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {resolve,join,dirname} from 'node:path';
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
export async function prepareMappingPredecessor(){
 const pin=JSON.parse(await readFile(new URL('./mapping-predecessor-pin.json',import.meta.url),'utf8'));
 const verify=async artifact=>{
  const proof=JSON.parse(await readFile(join(dirname(artifact),'proof.json'),'utf8'));
  assert.equal(proof.source_state,'committed');assert.equal(proof.revision,pin.revision);assert.equal(proof.source_sha256,pin.source_sha256);
  for(const [name,digest]of Object.entries(pin.worker)){assert.equal(proof.worker[name],digest);assert.equal(sha(await readFile(join(artifact,name))),digest,`Exact published predecessor ${name}`);}
 };
 if(process.env.SCRY_MAPPING_PREDECESSOR_ARTIFACT){const artifact=resolve(process.env.SCRY_MAPPING_PREDECESSOR_ARTIFACT);await verify(artifact);return {artifact,pin,close:async()=>{}};}
 const response=await fetch(pin.url,{signal:AbortSignal.timeout(120000)});assert.equal(response.status,200,'Pinned predecessor release download must succeed.');
 const chunks=[];let size=0;for await(const chunk of response.body){size+=chunk.byteLength;assert.ok(size<=pin.zip_bytes,'Predecessor download exceeds pinned byte count.');chunks.push(chunk);}
 const bytes=Buffer.concat(chunks);assert.equal(bytes.length,pin.zip_bytes);assert.equal(sha(bytes),pin.zip_sha256);
 const require=createRequire(import.meta.url);let unzipSync;try{({unzipSync}=require('fflate'));}catch{assert.ok(process.env.SCRY_WRANGLER_PACKAGE,'Pinned fflate dependency required.');({unzipSync}=createRequire(resolve(process.env.SCRY_WRANGLER_PACKAGE))('fflate'));}
 const wanted=new Set(['proof.json',...Object.keys(pin.worker).map(name=>'worker/'+name)]);
 const files=unzipSync(bytes,{filter:file=>wanted.has(file.name)});assert.deepEqual(Object.keys(files).sort(),[...wanted].sort());
 const root=await mkdtemp(join(tmpdir(),'scry-mapping-predecessor-'));
 try{for(const [name,data]of Object.entries(files)){const path=join(root,name);await mkdir(dirname(path),{recursive:true});await writeFile(path,data);}const artifact=join(root,'worker');await verify(artifact);return {artifact,pin,close:()=>rm(root,{recursive:true,force:true})};}
 catch(error){await rm(root,{recursive:true,force:true});throw error;}
}
