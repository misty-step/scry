import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {cpSync,existsSync,mkdtempSync,mkdirSync,readFileSync,rmSync,symlinkSync,writeFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {test} from 'node:test';
import {specs,capabilities} from './walk-specs.mjs';

const repo=resolve(import.meta.dirname,'..');
const source=readFileSync(join(repo,'SPEC.md'),'utf8');
const require=createRequire(import.meta.url);
const modules=require.resolve.paths('miniflare').find(path=>existsSync(join(path,'miniflare')));
const usage='usage: qa/walk --all | --stories "US-002 US-003"';
const walkIds=Object.keys(specs);
const drift=ids=>`spec binding drift: no SPEC capability heading or walk for [${ids.map(id=>`${id} "${capabilities[id]}"`).join(', ')}]`;

function probe(argv,spec=source,{accepted=false,ownedOutput=false,expectedError=usage}={}){
 const fixture=mkdtempSync(join(tmpdir(),'scry-walk-cli-'));
 try{
  cpSync(join(repo,'qa'),join(fixture,'qa'),{recursive:true});
  symlinkSync(modules,join(fixture,'node_modules'),'dir');
  writeFileSync(join(fixture,'SPEC.md'),spec);
  mkdirSync(join(fixture,'tools'));
  writeFileSync(join(fixture,'tools/worker-build'),'#!/bin/sh\nprintf invoked > build-invoked\nexit 91\n',{mode:0o755});
  if(ownedOutput){mkdirSync(join(fixture,'target/walk'),{recursive:true});writeFileSync(join(fixture,'target/walk/.scry-walk-owned'),'existing proof\n');}
  const env={PATH:join(fixture,'tools'),HOME:fixture,LANG:'C.UTF-8',SCRY_CRITICS_PLAYWRIGHT_PATH:join(fixture,'missing-playwright')};
  if(accepted)env.SCRY_SKIP_BUILD='1';
  const result=spawnSync('/bin/bash',[join(fixture,'qa/walk'),...argv],{cwd:fixture,env:{...env,PATH:join(fixture,'tools')+':'+process.execPath.slice(0,process.execPath.lastIndexOf('/'))+':/usr/bin:/bin'},encoding:'utf8'});
  assert.ifError(result.error);
  assert.equal(result.status,1,result.stderr);
  assert.equal(result.stdout,'');
  const error=result.stderr.match(/^Error: (.+)$/m)?.[1];
  assert.equal(error,accepted?'Install lockfile-pinned Playwright or set SCRY_CRITICS_PLAYWRIGHT_PATH.':expectedError,result.stderr);
  assert.equal(existsSync(join(fixture,'build-invoked')),false,'must validate before building');
  if(ownedOutput)assert.equal(readFileSync(join(fixture,'target/walk/.scry-walk-owned'),'utf8'),'existing proof\n');
  else assert.equal(existsSync(join(fixture,'target')),false,'must not create proof output');
  console.log(JSON.stringify({argv,error,status:result.status,build_invoked:false,proof_output:ownedOutput?'preserved':'absent'}));
 }finally{rmSync(fixture,{recursive:true,force:true});}
}

for(const value of ['', ' ', '\t\n\r '])test(`reject empty selection ${JSON.stringify(value)} before build/proof`,()=>probe(['--stories',value]));
test('standalone story walk builds the historical executable before starting a journey',()=>{
 const fixture=mkdtempSync(join(tmpdir(),'scry-walk-helper-build-'));
 try{
  cpSync(join(repo,'qa'),join(fixture,'qa'),{recursive:true});
  symlinkSync(modules,join(fixture,'node_modules'),'dir');
  writeFileSync(join(fixture,'SPEC.md'),source);
  mkdirSync(join(fixture,'build'));writeFileSync(join(fixture,'build/index_bg.wasm'),'synthetic artifact; never executed');
  writeFileSync(join(fixture,'source.json'),JSON.stringify({git_revision:'a'.repeat(40),git_tree:'b'.repeat(40),source_state:'worktree'}));
  mkdirSync(join(fixture,'tools'));
  writeFileSync(join(fixture,'tools/cargo'),'#!/bin/sh\nprintf "%s\\n" "$*" >> cargo-commands\nif [ "$1" = test ]; then exit 0; fi\nexit 91\n',{mode:0o755});
  const env={PATH:join(fixture,'tools')+':'+process.execPath.slice(0,process.execPath.lastIndexOf('/'))+':/usr/bin:/bin',HOME:fixture,LANG:'C.UTF-8',SCRY_SKIP_BUILD:'1',SCRY_SOURCE_META:join(fixture,'source.json')};
  const result=spawnSync('/bin/bash',[join(fixture,'qa/walk'),'--stories','US-016'],{cwd:fixture,env,encoding:'utf8',timeout:15000});
  assert.ifError(result.error);assert.equal(result.status,1,result.stderr);
  assert.deepEqual(readFileSync(join(fixture,'cargo-commands'),'utf8').trim().split('\n'),['test --locked --all-targets','build --locked --example qa-historical-fixture']);
  assert.match(result.stderr,/qa-historical-fixture/,'A failed helper build stops before starting any runtime.');
 }finally{rmSync(fixture,{recursive:true,force:true});}
});
test('invalid selection preserves prior owned proof',()=>probe(['--stories',''],source,{ownedOutput:true}));
for(const argv of [[],['--stories'],['--all','extra'],['--stories','US-005'],['--stories','US-012'],['--stories','US-019'],['--stories','US-999'],['--stories','US-002 US-999']])test(`reject invalid CLI ${JSON.stringify(argv)}`,()=>probe(argv));
for(const argv of [['--all'],['--stories','US-002 US-013'],['--stories',' US-004\tUS-004\n ']])test(`accept selector ${JSON.stringify(argv)} (guard-only)`,()=>probe(argv,source,{accepted:true}));
test('every journey is bound to one SPEC capability heading',()=>assert.deepEqual(Object.keys(capabilities),walkIds));
for(const [name,spec,ids] of [
 ['no capability headings',source.replace(/^### /gm,'#### '),walkIds],
 ['renamed capability',source.replace(/^### Owner tokens$/m,'### Access tokens'),['US-014']],
 ['removed capability',source.replace(/^### Retired generation records$/m,''),['US-004','US-013']]
])for(const argv of [['--all'],['--stories','US-002']])test(`reject spec binding drift: ${name} ${argv[0]}`,()=>probe(argv,spec,{expectedError:drift(ids)}));
