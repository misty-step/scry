// Current candidate identity: exact Rust source snapshot + compiled Worker
// modules. Historical Go buildinfo lives in provenance.mjs; it is never used
// to identify a Rust candidate.
import {createHash} from 'node:crypto';
import {readFile,writeFile,mkdir,readdir,lstat,copyFile} from 'node:fs/promises';
import {join,resolve,relative} from 'node:path';

export const artifactFiles=['index.js','index_bg.wasm'];
const sourceFiles=['Cargo.toml','Cargo.lock','rust-toolchain.toml'];
const digest=bytes=>createHash('sha256').update(bytes).digest('hex');

export async function artifactIdentity(directory) {
  const entries=[];
  for(const name of artifactFiles) {
    const path=join(directory,name);
    const stat=await lstat(path);
    if(!stat.isFile()||stat.isSymbolicLink())throw Error('Worker artifact must be a regular file: '+name);
    entries.push({path:name,sha256:digest(await readFile(path)),size:stat.size});
  }
  return {files:entries,sha256:digest(JSON.stringify(entries))};
}

export async function snapshotSource(root,directory) {
  root=resolve(root);
  const names=[...sourceFiles];
  async function descend(parent) {
    for(const child of (await readdir(join(root,parent))).sort()) {
      const name=join(parent,child);
      const info=await lstat(join(root,name));
      if(info.isSymbolicLink())throw Error('Source symlinks are unsupported in a critic build: '+name);
      if(info.isDirectory())await descend(name);
      else if(info.isFile())names.push(name);
      else throw Error('Unsupported source entry: '+name);
    }
  }
  await descend('src');await descend('assets');
  // Rust embeds the inherited licensed fonts at their original source paths.
  // Copy only that directory, never the historical Go tree or operator state.
  await descend('internal/web/assets/fonts');
  const entries=[];
  await mkdir(directory,{recursive:true});
  for(const name of names.sort()) {
    const original=join(root,name),copied=join(directory,name);
    if(relative(root,resolve(original)).startsWith('..'))throw Error('Source escapes checkout');
    const stat=await lstat(original);
    if(!stat.isFile()||stat.isSymbolicLink())throw Error('Source must be a regular file: '+name);
    await mkdir(join(copied,'..'),{recursive:true});
    await copyFile(original,copied);
    const copiedBytes=await readFile(copied);
    entries.push({path:name,sha256:digest(copiedBytes),size:copiedBytes.length});
  }
  const identity={format:'scry-critic-rust-source-v1',files:entries,sha256:digest(JSON.stringify(entries))};
  await writeFile(join(directory,'critic-source.json'),JSON.stringify(identity,null,2)+'\n');
  return identity;
}

export async function verifyWorkerIdentity(handle) {
  if(!handle.artifact||typeof handle.artifact.directory!=='string')throw Error('handle lacks Worker artifact identity');
  const actual=await artifactIdentity(handle.artifact.directory);
  if(actual.sha256!==handle.binary_sha256||actual.sha256!==handle.artifact.sha256)
    throw Error('Worker artifact changed since candidate up');
  if(JSON.stringify(actual.files)!==JSON.stringify(handle.artifact.files))
    throw Error('Worker module inventory does not match its handle');
  return actual;
}
