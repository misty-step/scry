import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {createRuntime,internalKey} from './runtime-helper.mjs';

// Run with the prior release Worker bytes and candidate-generated synthetic rows.
const fixturePath = process.env.SCRY_CHECKPOINT_FIXTURE_DIR;
const oldArtifact = process.env.SCRY_CHECKPOINT_OLD_WORKER;
const skip = fixturePath && oldArtifact ? false : 'Requires synthetic fixtures and verified previous Worker artifact.';
if (!skip) {
  const sha = bytes => createHash('sha256').update(bytes).digest('hex');
  assert.equal(sha(await readFile(`${oldArtifact}/index.js`)), '12de1dd5eef381a6c75b8b27d483ab559af466b1658a205258122472136ccfd2');
  assert.equal(sha(await readFile(`${oldArtifact}/index_bg.wasm`)), '73d9bc59b7cb6a668935ec20053c353294b4f462fdbf76a536b2e7fe6c4c9d69');
}

for (const suffix of ['saved','cleared']) {
  await test(`previous deployed Worker refuses ${suffix} checkpoint rows without modifying them`,{skip},async()=>{
    const name=`checkpoint-old-${suffix}`;
    const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey};
    const runtime=await createRuntime({port:0,name,artifact:oldArtifact,bindings,inspect:true});
    try {
      const origin=(await runtime.mf.ready).origin;
      const active=await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin});
      const store=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
      await store.exec('CREATE TABLE IF NOT EXISTS records (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID');
      const baseline=JSON.parse(await readFile(`${fixturePath}/baseline-rows.json`,'utf8'));
      assert.ok(!('checkpoint_compatibility/v1' in baseline),'The positive control is an older-compatible notebook.');
      for(const [key,value] of Object.entries(baseline)) await store.exec('INSERT INTO records(key,value) VALUES(?,?)',key,value);
      const control=await active.worker.fetch(`${origin}/recall`,{headers:{'X-Scry-Test-Host':new URL(origin).host}});
      assert.equal(control.status,200,'The same old Worker and storage path must read a compatible notebook.');
      assert.match(await control.text(),/data-state="exact-prompt"/,'The old Worker rendered the synthetic practice prompt.');
      await store.exec('DELETE FROM records');
      const rows=JSON.parse(await readFile(`${fixturePath}/${suffix}-rows.json`,'utf8'));
      for(const [key,value] of Object.entries(rows)) await store.exec('INSERT INTO records(key,value) VALUES(?,?)',key,value);
      const before=await store.exec('SELECT key,value FROM records ORDER BY key');
      const response=await active.worker.fetch(`${origin}/recall`,{headers:{'X-Scry-Test-Host':new URL(origin).host}});
      assert.ok(response.status>=500,`Old Worker must refuse incompatible state: ${response.status}`);
      assert.deepEqual(await store.exec('SELECT key,value FROM records ORDER BY key'),before);
    } finally { await runtime.mf.dispose(); }
  });
}

for (const suffix of ['saved','cleared']) {
  await test(`previous deployed Worker refuses ${suffix} checkpoint archive`,{skip},async()=>{
    const origin='https://checkpoint-restore-synthetic.invalid',name=`checkpoint-old-restore-${suffix}`;
    const token='synthetic-checkpoint-restore-token-32bytes';
    const bindings={SCRY_ENV:'restore',CANONICAL_ORIGIN:origin,SPACE_NAME:name,INTERNAL_KEY:internalKey,RESTORE_TOKEN:token};
    const runtime=await createRuntime({name,artifact:oldArtifact,bindings,inspect:true});
    try {
      const bytes=await readFile(`${fixturePath}/${suffix}-archive.json`);
      const hash=createHash('sha256').update(bytes).digest('hex');
      const response=await runtime.worker.fetch(`${origin}/operator/restore`,{method:'POST',headers:{'x-scry-restore-token':token,'x-scry-sha256':hash},body:bytes});
      assert.equal(response.status,422);
      const store=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
      assert.deepEqual(await store.exec('SELECT key FROM records'),[]);
    } finally { await runtime.mf.dispose(); }
  });
}
