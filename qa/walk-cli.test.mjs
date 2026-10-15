import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {cpSync,existsSync,mkdtempSync,mkdirSync,readFileSync,rmSync,symlinkSync,writeFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {test} from 'node:test';
import {specs} from './walk-specs.mjs';

const repo=resolve(import.meta.dirname,'..');
const source=readFileSync(join(repo,'SPEC.md'),'utf8');
const require=createRequire(import.meta.url);
const modules=require.resolve.paths('miniflare').find(path=>existsSync(join(path,'miniflare')));
const usage='usage: qa/walk --all | --stories "US-002 US-003"';
const walkIds=Object.keys(specs);

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
test('invalid selection preserves prior owned proof',()=>probe(['--stories',''],source,{ownedOutput:true}));
for(const argv of [[],['--stories'],['--all','extra'],['--stories','US-005'],['--stories','US-012'],['--stories','US-999'],['--stories','US-002 US-999']])test(`reject invalid CLI ${JSON.stringify(argv)}`,()=>probe(argv));
for(const argv of [['--all'],['--stories','US-002 US-013'],['--stories',' US-004\tUS-004\n ']])test(`accept selector ${JSON.stringify(argv)} (guard-only)`,()=>probe(argv,source,{accepted:true}));
for(const [name,spec,live] of [
 ['no story headings',source.replace(/^## (US-\d{3})/gm,'### $1'),[]],
 ['trailing retirement wording',source+'\n## Change log\n\nSuperseded wording for an unrelated change.\n',walkIds.slice(0,-1)],
 ['unknown active story',source+'\n## US-999 Unknown\n\nCriteria:\n1. WHEN tested, THE SYSTEM SHALL fail closed.\n',[...walkIds,'US-999']],
 ['duplicate replacing a live story',source.replace('## US-013 Create','## US-011 Create'),walkIds.with(-1,'US-011')]
])for(const argv of [['--all'],['--stories','US-002']])test(`reject discovery drift: ${name} ${argv[0]}`,()=>probe(argv,spec,{expectedError:`story discovery drift: SPEC live IDs [${live.join(', ')}] do not match walk-specs IDs [${walkIds.join(', ')}]`}));
