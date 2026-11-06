import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {createRuntime,internalKey} from './runtime-helper.mjs';

const dir=process.env.SCRY_CHECKPOINT_FIXTURE_DIR;
test('complete checkpoint archive restores into an isolated notebook', {skip:dir?false:'Requires synthetic checkpoint fixture directory.'}, async()=>{
  const name='checkpoint-new-restore',token='synthetic-checkpoint-restore-token-32bytes';
  const base={CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey};
  const runtime=await createRuntime({port:0,name,bindings:{...base,SCRY_ENV:'restore',RESTORE_TOKEN:token},inspect:true});
  try {
    const origin=(await runtime.mf.ready).origin;
    let active=await runtime.rebind({...base,CANONICAL_ORIGIN:origin,SCRY_ENV:'restore',RESTORE_TOKEN:token});
    const bytes=await readFile(`${dir}/saved-archive.json`),hash=createHash('sha256').update(bytes).digest('hex');
    const response=await active.worker.fetch(`${origin}/operator/restore`,{method:'POST',headers:{'x-scry-restore-token':token,'x-scry-sha256':hash},body:bytes});
    assert.equal(response.status,200);
    active=await runtime.rebind({...base,CANONICAL_ORIGIN:origin,SCRY_ENV:'development'});
    const page=await active.worker.fetch(`${origin}/?practice=nato`);
    assert.equal(page.status,200);assert.match(await page.text(),/synthetic checkpoint/);
    const store=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
    assert.equal((await store.exec("SELECT key FROM records WHERE key='checkpoint_compatibility/v1'")).length,1);
  } finally { await runtime.mf.dispose(); }
});
