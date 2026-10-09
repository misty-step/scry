import {describe,it,after} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,mkdir,writeFile,readFile,rm,symlink} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {artifactIdentity,snapshotSource,verifyWorkerIdentity} from '../lib/worker-artifact.mjs';
import {sourceProvenance} from '../lib/source-provenance.mjs';
import {spawnSync} from 'node:child_process';
const temporary=await mkdtemp(join(tmpdir(),'scry-critic-worker-identity-'));
after(()=>rm(temporary,{recursive:true,force:true}));
describe('Rust source and Worker artifact provenance',()=>{
  it('changes artifact identity when either compiled module changes',async()=>{
    const directory=join(temporary,'artifact');await mkdir(directory);
    await writeFile(join(directory,'index.js'),'export default {}');
    await writeFile(join(directory,'index_bg.wasm'),Buffer.from([0,97,115,109]));
    const original=await artifactIdentity(directory);
    assert.equal(original.files.length,2);
    assert.match(original.sha256,/^[0-9a-f]{64}$/);
    const handle={binary_sha256:original.sha256,artifact:{directory,...original}};
    await verifyWorkerIdentity(handle);
    await writeFile(join(directory,'index_bg.wasm'),Buffer.from([0,97,115,109,1]));
    await assert.rejects(verifyWorkerIdentity(handle),/changed/);
    const changed=await artifactIdentity(directory);
    assert.notEqual(changed.sha256,original.sha256);
    await writeFile(join(directory,'index.js'),'export default {changed:true}');
    assert.notEqual((await artifactIdentity(directory)).sha256,changed.sha256);
  });
  it('freezes exact Rust runtime/assets without importing credentials or the caller development state',async()=>{
    const source=join(temporary,'checkout');await mkdir(join(source,'src'),{recursive:true});
    await mkdir(join(source,'assets'));
    for(const name of ['Cargo.toml','Cargo.lock','rust-toolchain.toml'])await writeFile(join(source,name),name+'\n');
    await writeFile(join(source,'src/lib.rs'),'pub fn original() {}');
    await writeFile(join(source,'assets/app.css'),'body {color:ink}');
    await writeFile(join(source,'.dev.vars'),'PRIVATE_DO_NOT_COPY=secret');
    const copy=join(temporary,'snapshot');
    const identity=await snapshotSource(source,copy);
    assert.equal(identity.files.length,5);assert.ok(!identity.files.some(file=>file.path.includes('vars')));
    await writeFile(join(source,'src/lib.rs'),'pub fn changed() {}');
    assert.equal(await readFile(join(copy,'src/lib.rs'),'utf8'),'pub fn original() {}');
    assert.notEqual((await snapshotSource(source,join(temporary,'changed'))).sha256,identity.sha256);
    await assert.rejects(readFile(join(copy,'.dev.vars')));
  });
  it('refuses source symlinks that could silently import operator material',async()=>{
    const source=join(temporary,'symlink-source');await mkdir(join(source,'src'),{recursive:true});await mkdir(join(source,'assets'));
    await symlink(join(temporary,'missing-private'),join(source,'src/secret'));
    await assert.rejects(snapshotSource(source,join(temporary,'symlink-snapshot')),/symlinks are unsupported/);
  });
  it('uses explicit validated frozen metadata without requiring Git and rejects invalid source identity',async()=>{
    const metadata=join(temporary,'source.json');
    await writeFile(metadata,JSON.stringify({git_revision:'a'.repeat(40),source_sha256:'b'.repeat(64),source_state:'worktree',files:[]}));
    const identity=sourceProvenance(temporary,metadata);
    assert.equal(identity.revision,'a'.repeat(40));
    assert.equal(identity.source_state,'dirty');
    assert.equal(identity.state_source,'validated-gate-source-metadata');
    assert.equal(identity.gate_source_sha256,'b'.repeat(64));
    await writeFile(metadata,JSON.stringify({git_revision:'a'.repeat(40),source_sha256:'invalid',source_state:'committed',files:[]}));
    assert.throws(()=>sourceProvenance(temporary,metadata),/Invalid frozen source/);
  });
  it('never borrows a parent repository revision for a bare source subdirectory',async()=>{
    const parent=join(temporary,'parent-git'),child=join(parent,'bare-source');
    await mkdir(child,{recursive:true});
    const initialized=spawnSync('git',['init','--quiet',parent],{encoding:'utf8'});
    assert.equal(initialized.status,0,initialized.stderr);
    assert.equal(sourceProvenance(child,null).revision,null);
  });
});
