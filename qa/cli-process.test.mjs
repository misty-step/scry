import assert from 'node:assert/strict';
import test from 'node:test';
import {cliProcess} from './cli-process.mjs';

const run=(script,input='')=>cliProcess(process.execPath,['-e',script],{input});

test('empty input uses no pipe even when a fast child closes stdin',async()=>{
 const result=await run(`const fs=require('node:fs'); const stat=fs.fstatSync(0);const pipe=stat.isFIFO()||stat.isSocket(); fs.closeSync(0); process.stdout.write(JSON.stringify({pipe}));`);
 assert.equal(result.code,0);assert.deepEqual(JSON.parse(result.out),{pipe:false});
});

test('completion drains stdout and stderr after the child exit event',async()=>{
 const descendant=`setTimeout(()=>{process.stdout.write('{"late":true}');process.stderr.write('late stderr');},80);`;
 const result=await run(`require('node:child_process').spawn(process.execPath,['-e',${JSON.stringify(descendant)}],{stdio:['ignore',1,2]});process.exit(0);`);
 assert.equal(result.code,0);assert.equal(result.out,'{"late":true}');assert.equal(result.err,'late stderr');
});

test('real input is delivered exactly and successful output remains required',async()=>{
 const input='Synthetic supplied answer — Alfa\n'.repeat(10000);
 const result=await run(`let value='';process.stdin.setEncoding('utf8');process.stdin.on('data',x=>value+=x);process.stdin.on('end',()=>process.stdout.write(JSON.stringify({value})));`,input);
 assert.equal(result.code,0);assert.equal(JSON.parse(result.out).value,input);assert.equal(result.stdinError,null);
});

test('early refusal preserves its actual nonzero exit and JSON despite EPIPE',async()=>{
 const result=await run(`require('node:fs').closeSync(0);process.stdout.write('{"error":"synthetic refusal"}');process.exitCode=2;`,'x'.repeat(4*1024*1024));
 assert.equal(result.code,2);assert.deepEqual(JSON.parse(result.out),{error:'synthetic refusal'});assert.equal(result.stdinError,'EPIPE');
});

test('EPIPE can never turn a truncated input into successful delivery',async()=>{
 await assert.rejects(run(`require('node:fs').closeSync(0);process.stdout.write('{"ok":true}');`,'x'.repeat(4*1024*1024)),error=>error.code==='EPIPE'&&error.exitCode===0&&error.message==='CLI stdin delivery failed.');
});

test('spawn failure remains a failure rather than a CLI refusal',async()=>{
 await assert.rejects(cliProcess('/nonexistent-scry-qa-binary',[]),{code:'ENOENT'});
});

test('a signal is not a successful exit or a structured refusal',async()=>{
 await assert.rejects(run(`process.kill(process.pid,'SIGTERM');`),/CLI terminated before completion/);
});
