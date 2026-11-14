import test from 'node:test';
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {starterFixture,nativeFields} from './nato-starter.mjs';
import {enterMemory} from './study-review.mjs';
// Each fixture starts at source study; a draft belongs to the Quiz reached by Try from memory.

test('automatic checkpoint returns a non-replacing scoped acknowledgment and replay reflects current state',async()=>{
  const f=await starterFixture();
  try {
    const start=nativeFields(await f.get('/?practice=nato'),'/practice/nato/start');
    assert.equal((await f.post('/practice/nato/start',start)).status,303);
    const page=await enterMemory(f,'/?practice=nato');
    const scope=JSON.parse(await f.get('/session/scope')).scope;
    const original={...nativeFields(page,'/practice/answer'),generation:'0',answer:'Al',operation_id:`autosave-${randomUUID()}`};
    const send=()=>f.postScoped('/practice/draft',scope,original,{'X-Scry-Checkpoint-Mode':'auto-v1'});
    const response=await send();assert.equal(response.status,200);assert.match(response.headers.get('content-type'),/application\/json/);assert.equal(response.headers.get('x-scry-scope'),scope);
    const result=await response.json();assert.equal(result.version,1);assert.equal(result.current,true);assert.equal(result.generation,'1');assert.equal(result.prompt_id,original.prompt_id);
    assert.equal(JSON.stringify(result).includes('Al'),false,'Response contains no answer text.');
    const after=await f.rows();assert.equal(after.meta.practice.answer_checkpoint.draft.answer,'Al');
    assert.equal(Object.keys(after).filter(key=>key.startsWith('practice_attempts/')).length,0);
    assert.equal((await f.post('/practice/answer',{...nativeFields(await f.get('/?practice=nato'),'/practice/answer'),answer:'Alfa',operation_id:randomUUID()})).status,303);
    const afterAnswer=await f.rows();const late=await f.postScoped('/practice/draft',scope,{...original,operation_id:`autosave-${randomUUID()}`},{'X-Scry-Checkpoint-Mode':'auto-v1'});assert.equal(late.status,409);assert.deepEqual(await f.rows(),afterAnswer);
    const replay=await send();assert.equal(replay.status,200);const replayed=await replay.json();assert.equal(replayed.current,false,'A replayed receipt is not evidence the draft is still present.');
  } finally {await f.close();}
});

test('competing saves conflict and never overwrite the winning tab',async()=>{
  const f=await starterFixture();
  try {
    const start=nativeFields(await f.get('/?practice=nato'),'/practice/nato/start');assert.equal((await f.post('/practice/nato/start',start)).status,303);
    const page=await enterMemory(f,'/?practice=nato'),scope=JSON.parse(await f.get('/session/scope')).scope;
    const fields={...nativeFields(page,'/practice/answer'),generation:'0',answer:'winner',operation_id:`autosave-${randomUUID()}`};
    const headers={'X-Scry-Checkpoint-Mode':'auto-v1'};
    assert.equal((await f.postScoped('/practice/draft',scope,fields,headers)).status,200);
    const before=await f.rows();const stale=await f.postScoped('/practice/draft',scope,{...fields,answer:'loser',operation_id:`autosave-${randomUUID()}`},headers);assert.equal(stale.status,409);
    assert.deepEqual(await f.rows(),before);assert.equal(before.meta.practice.answer_checkpoint.draft.answer,'winner');
    const sameText={...fields,generation:'1',operation_id:`autosave-${randomUUID()}`};assert.equal((await f.postScoped('/practice/draft',scope,sameText,headers)).status,200);
    const old=await (await f.postScoped('/practice/draft',scope,fields,headers)).json();assert.equal(old.current,false,'Matching text from a newer generation is not the old receipt.');
  } finally {await f.close();}
});
test('discard and question switch invalidate old save receipts without revealing text',async()=>{
  const f=await starterFixture();
  try {
    const start=nativeFields(await f.get('/?practice=nato'),'/practice/nato/start');assert.equal((await f.post('/practice/nato/start',start)).status,303);
    const page=await enterMemory(f,'/?practice=nato'),scope=JSON.parse(await f.get('/session/scope')).scope,base=nativeFields(page,'/practice/answer'),headers={'X-Scry-Checkpoint-Mode':'auto-v1'};
    const first={...base,generation:'0',answer:'private synthetic',operation_id:`autosave-${randomUUID()}`};
    assert.equal((await f.postScoped('/practice/draft',scope,first,headers)).status,200);
    const discard={...base,generation:'1',operation_id:randomUUID()};assert.equal((await f.postScoped('/practice/draft/discard',scope,discard)).status,303);
    const afterDiscard=await f.rows();assert.equal((await f.postScoped('/practice/draft',scope,{...first,generation:'1',operation_id:`autosave-${randomUUID()}`},headers)).status,409);assert.deepEqual(await f.rows(),afterDiscard);
    const old=await (await f.postScoped('/practice/draft',scope,first,headers)).json();assert.equal(old.current,false);assert.doesNotMatch(JSON.stringify(old),/private synthetic/);
    const second={...base,generation:'2',answer:'new private synthetic',operation_id:`autosave-${randomUUID()}`};assert.equal((await f.postScoped('/practice/draft',scope,second,headers)).status,200);
    const lessons=await f.get('/lessons');const selected=lessons.split('<form').find(part=>part.startsWith(' method="post" action="/practice/lesson/start"')&&part.split('</form>')[0].includes('name="starter" value="rosary-joyful"'));assert.ok(selected);const switchFields=nativeFields('<form'+selected,'/practice/lesson/start');assert.equal((await f.post('/practice/lesson/start',switchFields)).status,303);
    const after=await f.rows();assert.notEqual(after.meta.practice.current.prompt.id,second.prompt_id);assert.equal(after.meta.practice.answer_checkpoint.draft,null);
    assert.equal((await f.postScoped('/practice/draft',scope,{...second,generation:'3',operation_id:`autosave-${randomUUID()}`},headers)).status,409);assert.deepEqual(await f.rows(),after);
    const oldSecond=await (await f.postScoped('/practice/draft',scope,second,headers)).json();assert.equal(oldSecond.current,false);assert.doesNotMatch(JSON.stringify(oldSecond),/new private synthetic/);
    assert.deepEqual(await f.rows(),after);assert.equal(Object.keys(after).filter(key=>key.startsWith('practice_attempts/')).length,0);
  } finally {await f.close();}
});

test('automatic save requires the original scope and valid CSRF',async()=>{
  const f=await starterFixture();
  try {
    assert.equal((await f.post('/practice/nato/start',nativeFields(await f.get('/?practice=nato'),'/practice/nato/start'))).status,303);
    const page=await enterMemory(f,'/?practice=nato'),scope=JSON.parse(await f.get('/session/scope')).scope;
    const fields={...nativeFields(page,'/practice/answer'),generation:'0',answer:'synthetic',operation_id:`autosave-${randomUUID()}`};const mode={'X-Scry-Checkpoint-Mode':'auto-v1'};const before=await f.rows();
    assert.equal((await f.post('/practice/draft',fields)).status,403);
    assert.equal((await f.postScoped('/practice/draft','b'.repeat(64),fields,mode)).status,409);
    assert.equal((await f.postScoped('/practice/draft',scope,{...fields,csrf:'invalid'},mode)).status,403);
    assert.equal((await f.postScoped('/practice/draft',scope,{...fields,answer:'x'.repeat(130*1024)},mode)).status,413);
    assert.deepEqual(await f.rows(),before);
  } finally {await f.close();}
});
test('automatic budget is shared by reloads, rejects new saves, and permits replay and manual save',{timeout:180000},async()=>{
  const f=await starterFixture();
  try {
    assert.equal((await f.post('/practice/nato/start',nativeFields(await f.get('/?practice=nato'),'/practice/nato/start'))).status,303);
    const page=await enterMemory(f,'/?practice=nato'),scope=JSON.parse(await f.get('/session/scope')).scope,base=nativeFields(page,'/practice/answer'),mode={'X-Scry-Checkpoint-Mode':'auto-v1'};let last;
    for(let i=0;i<512;i++){
      last={...base,generation:String(i),answer:`answer ${i}`,operation_id:`autosave-${randomUUID()}`};const r=await f.postScoped('/practice/draft',scope,last,mode);assert.equal(r.status,200,`save ${i}`);const receipt=await r.json();assert.equal(receipt.remaining,511-i);
      if(i===255)await f.get('/?practice=nato');
    }
    const before=await f.rows(),blocked={...last,generation:'512',answer:'blocked',operation_id:`autosave-${randomUUID()}`};assert.equal((await f.postScoped('/practice/draft',scope,blocked,mode)).status,429);assert.deepEqual(await f.rows(),before);
    assert.equal((await f.postScoped('/practice/draft',scope,{...blocked,csrf:'invalid'},mode)).status,403);assert.deepEqual(await f.rows(),before);
    const replay=await f.postScoped('/practice/draft',scope,last,mode);assert.equal(replay.status,200);assert.equal((await replay.json()).current,true);assert.deepEqual(await f.rows(),before);
    const manual={...blocked,operation_id:randomUUID(),answer:'manual remains available'};assert.equal((await f.postScoped('/practice/draft',scope,manual)).status,303);assert.equal((await f.rows()).meta.practice.answer_checkpoint.draft.answer,'manual remains available');
  } finally {await f.close();}
});

test('a maximum-length LF-heavy textarea fits the bounded multipart save',async()=>{
  const f=await starterFixture();
  try {
    const lessons=await f.get('/lessons');const selected=lessons.split('<form').find(part=>part.startsWith(' method="post" action="/practice/lesson/start"')&&part.split('</form>')[0].includes('name="starter" value="rosary-joyful"'));assert.ok(selected);assert.equal((await f.post('/practice/lesson/start',nativeFields('<form'+selected,'/practice/lesson/start'))).status,303);
    const page=await enterMemory(f),scope=JSON.parse(await f.get('/session/scope')).scope,answer='\n'.repeat(32768);
    const fields={...nativeFields(page,'/practice/answer'),generation:'0',answer,operation_id:`autosave-${randomUUID()}`};
    const saved=await f.postScoped('/practice/draft',scope,fields,{'X-Scry-Checkpoint-Mode':'auto-v1'});assert.equal(saved.status,200,await saved.text());assert.equal((await f.rows()).meta.practice.answer_checkpoint.draft.answer,answer);
  } finally {await f.close();}
});
