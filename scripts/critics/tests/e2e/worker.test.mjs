import {it,after} from 'node:test';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {mkdtempSync,rmSync,readFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {tmpdir} from 'node:os';
import {createServer} from 'node:net';
import {resolveBrowser} from '../../lib/browser.mjs';
import {validateReceipt} from '../../lib/receipt.mjs';
const root=resolve(import.meta.dirname,'../../../..'),runner=join(root,'scripts/critics/run.mjs');
const temporary=mkdtempSync(join(tmpdir(),'scry-critic-browser-worker-'));
after(()=>rmSync(temporary,{recursive:true,force:true}));
const run=(...args)=>spawnSync(process.execPath,[runner,...args],{encoding:'utf8',timeout:600000});
it('current compiled Worker: real browser answer, held feedback and deliberate Next with exact artifact binding',async t=>{
  const browser=await resolveBrowser();
  if(!browser.ok){
    if(process.env.SCRY_CRITICS_REQUIRE_BROWSER==='1')throw Error('Browser required: '+browser.reason);
    t.skip('browser unavailable: '+browser.reason);return;
  }
  const server=createServer();await new Promise(done=>server.listen(0,'127.0.0.1',done));
  const port=server.address().port;await new Promise(done=>server.close(done));
  const directory=join(temporary,'candidate'),output=join(temporary,'review');
  const up=run('candidate','up','--dir',directory,'--port',String(port),'--json');
  assert.equal(up.status,0,up.stdout+up.stderr);
  const handle=JSON.parse(up.stdout);
  try{
    const walked=run('human','--goal','practice-review','--candidate',handle.url,'--handle',join(directory,'candidate.json'),'--out',output);
    assert.equal(walked.status,0,walked.stdout+walked.stderr);
    const receipt=JSON.parse(readFileSync(join(output,'receipt.json')));
    assert.equal(validateReceipt(receipt).ok,true);
    assert.equal(receipt.candidate.bound,true);
    assert.equal(receipt.candidate.identity_format,'scry-critic-worker-candidate-v2');
    assert.equal(receipt.candidate.binary_sha256,handle.artifact.sha256);
    assert.equal(receipt.candidate.source_sha256,handle.source_sha256);
    for(const id of ['US-002.1','US-002.2','US-002.2-next'])assert.equal(receipt.checks.find(check=>check.id===id)?.status,'pass');
    assert.equal(receipt.findings.length,0);
  }finally{
    const down=run('candidate','down','--dir',directory);
    assert.equal(down.status,0,down.stderr);
  }
});
