// The source-snapshot gate has no .git. Its validated metadata is explicit;
// otherwise a live checkout must own its Git root rather than borrow a parent.
import {readFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {resolve} from 'node:path';
export function sourceProvenance(root,metadataPath=process.env.SCRY_SOURCE_META) {
  if(metadataPath) {
    let metadata;
    try {metadata=JSON.parse(readFileSync(metadataPath,'utf8'));}
    catch {throw Error('Invalid frozen source provenance metadata');}
    if(!/^([0-9a-f]{40}|[0-9a-f]{64})$/.test(metadata.git_revision||'')||
       !/^[0-9a-f]{64}$/.test(metadata.source_sha256||'')||
       !['committed','worktree'].includes(metadata.source_state)||
       !Array.isArray(metadata.files))throw Error('Invalid frozen source provenance metadata');
    return {revision:metadata.git_revision,source_state:metadata.source_state==='committed'?'clean':'dirty',
      state_source:'validated-gate-source-metadata',gate_source_sha256:metadata.source_sha256};
  }
  const git=(...args)=>spawnSync('git',args,{cwd:root,encoding:'utf8'});
  const top=git('rev-parse','--show-toplevel');
  if(top.status!==0||resolve(top.stdout.trim())!==resolve(root))
    return {revision:null,source_state:'unknown',state_source:null,gate_source_sha256:null};
  const head=git('rev-parse','HEAD'),status=git('status','--porcelain');
  if(head.status!==0||status.status!==0)
    return {revision:null,source_state:'unknown',state_source:null,gate_source_sha256:null};
  return {revision:head.stdout.trim(),source_state:status.stdout.trim()?'dirty':'clean',
    state_source:'source-checkout',gate_source_sha256:null};
}
