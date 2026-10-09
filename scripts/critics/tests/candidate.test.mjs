import {describe,it,after} from 'node:test';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {existsSync,mkdtempSync,writeFileSync,mkdirSync,rmSync,readFileSync,readdirSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';
import {createServer} from 'node:http';
import {createServer as netServer} from 'node:net';
import {readyAttempt} from '../lib/ready.mjs';
import {verifyWorkerIdentity} from '../lib/worker-artifact.mjs';
const root=resolve(import.meta.dirname,'../../..');
const runner=join(root,'scripts/critics/run.mjs');
const temporary=mkdtempSync(join(tmpdir(),'scry-worker-critic-'));
after(()=>rmSync(temporary,{recursive:true,force:true}));
const run=(...args)=>spawnSync(process.execPath,[runner,...args],{encoding:'utf8',timeout:600000});
async function freePort(){
  const server=netServer();
  await new Promise(done=>server.listen(0,'127.0.0.1',done));
  const port=server.address().port;
  await new Promise(done=>server.close(done));return port;
}

describe('Rust Worker candidate lifecycle and data boundaries',()=>{
  it('refuses a busy port before building or writing a handle and leaves its owner serving',async()=>{
    const server=createServer((_,response)=>response.end('existing-owner'));
    await new Promise(done=>server.listen(0,'127.0.0.1',done));
    const directory=join(temporary,'busy');
    try{
      const result=run('candidate','up','--dir',directory,'--port',String(server.address().port));
      assert.equal(result.status,2,result.stderr);assert.match(result.stderr,/already in use/);
      assert.equal(existsSync(directory),false);
      assert.equal(await(await fetch('http://127.0.0.1:'+server.address().port)).text(),'existing-owner');
    }finally{await new Promise(done=>server.close(done));}
  });
  it('refuses a nonempty directory without altering unrelated content',()=>{
    const directory=join(temporary,'precious');mkdirSync(directory);
    writeFileSync(join(directory,'keep.txt'),'historical material\n');
    const before=readdirSync(directory);
    const result=run('candidate','up','--dir',directory);
    assert.equal(result.status,2);assert.match(result.stderr,/not fresh/);
    assert.deepEqual(readdirSync(directory),before);
    assert.equal(readFileSync(join(directory,'keep.txt'),'utf8'),'historical material\n');
  });
  it('refuses the retired Go binary option before creating state',async()=>{
    const directory=join(temporary,'retired');
    const result=run('candidate','up','--dir',directory,'--port',String(await freePort()),'--binary','/bin/true');
    assert.equal(result.status,2);assert.match(result.stderr,/historical Go option/);
    assert.equal(existsSync(directory),false);
  });
  it('bounds readiness when a process accepts a connection without answering',async()=>{
    const sockets=new Set();
    const server=netServer(socket=>{sockets.add(socket);socket.once('close',()=>sockets.delete(socket));});
    await new Promise(done=>server.listen(0,'127.0.0.1',done));
    const started=Date.now();
    try{
      await assert.rejects(readyAttempt('http://127.0.0.1:'+server.address().port,{timeoutMs:250}));
      assert.ok(Date.now()-started<5000);
    }finally{for(const socket of sockets)socket.destroy();await new Promise(done=>server.close(done));}
  });
  it('builds frozen Rust source, seeds real workerd SQLite, binds served module bytes and stops only its own process',async()=>{
    const directory=join(temporary,'fresh','candidate'),port=await freePort();
    const result=run('candidate','up','--dir',directory,'--port',String(port),'--json');
    assert.equal(result.status,0,result.stdout+result.stderr);
    const handle=JSON.parse(result.stdout);
    try{
      assert.equal(handle.format,'scry-critic-worker-candidate-v2');
      assert.equal(handle.binary_revision,handle.revision);
      assert.equal(handle.seed.model,'authored-test-fixture');
      assert.match(handle.source_sha256,/^[a-f0-9]{64}$/);
      await verifyWorkerIdentity(handle);
      const response=await fetch(handle.url+'/');
      assert.equal(response.status,200);
      const html=await response.text();assert.match(html,/data-state="question"/);
      assert.match(html,/cached response|freshness lifetime|conditional request/);
      const nonce='current-worker-test';
      const identity=await(await fetch(handle.url+'/__critic/identity?nonce='+nonce)).json();
      assert.equal(identity.nonce,nonce);assert.equal(identity.pid,handle.pid);
      assert.equal(identity.artifact_sha256,handle.binary_sha256);
      // File replacement must fail even while the original module stays
      // loaded in the serving Worker.
      const js=join(handle.artifact.directory,'index.js');
      writeFileSync(js,readFileSync(js,'utf8')+'\n// replacement probe\n');
      await assert.rejects(verifyWorkerIdentity(handle),/changed since candidate up/);
      const denied=run('human','--goal','practice-review','--candidate',handle.url,'--handle',join(directory,'candidate.json'),'--out',join(temporary,'replaced'));
      assert.equal(denied.status,2);assert.match(denied.stderr,/artifact changed/);
      const receipt=JSON.parse(readFileSync(join(temporary,'replaced','receipt.json')));
      assert.equal(receipt.checks[0].status,'unverified');
    }finally{
      const down=run('candidate','down','--dir',directory);
      assert.equal(down.status,0,down.stderr);
      assert.ok(JSON.parse(readFileSync(join(directory,'candidate.json'))).stopped_at);
    }
  });
});
