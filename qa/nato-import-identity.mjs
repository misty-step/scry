// Supplied, normalized NATO content must remain usable after canonical dedupe.
// Real Owner API/native forms and SQLite, with no production or provider access.
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {randomUUID} from 'node:crypto';
import {createRuntime,internalKey} from './runtime-helper.mjs';
import {nativeFields} from './nato-starter.mjs';
const canonical=JSON.parse(await readFile(new URL('../examples/nato-alphabet.json',import.meta.url),'utf8'));
const now='1800000000000';
export async function normalizedNatoJourney(){
 const report=[];
 for(const variant of ['canonical','uppercase','x-space','normalized-keys']){
  for(const entry of ['focused','lesson']){
   const name='nato-import-'+randomUUID();let egress=0;
   const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey,SCRY_TEST_NOW_MS:now};
   let runtime=await createRuntime({name,port:0,inspect:true,testClock:true,bindings,egress:async()=>{egress++;throw Error('External calls forbidden in NATO import QA.');}});
   try{
    const origin=(await runtime.mf.ready).origin;
    runtime={...runtime,...await runtime.rebind({...bindings,CANONICAL_ORIGIN:origin})};
    const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
    const rows=()=>storage.exec('SELECT key,value FROM records ORDER BY key');
    const subset=(all,prefix)=>all.filter(row=>row.key.startsWith(prefix));
    const values=(all,prefix)=>subset(all,prefix).map(row=>JSON.parse(row.value));
    const assertReplay=(actual,expected)=>{
     assert.deepEqual(actual.filter(r=>r.key!=='meta'),expected.filter(r=>r.key!=='meta'),'Replay preserves every content, history and receipt byte.');
     const a=values(actual,'meta')[0],b=values(expected,'meta')[0];
     // The HTTP persistence boundary advances only the global revision on replay.
     assert.equal(a.revision,b.revision+1);a.revision=b.revision;assert.deepEqual(a,b);
    };
    const get=async path=>{const r=await runtime.worker.fetch(origin+path);assert.equal(r.status,200);return r.text();};
    const post=(path,fields)=>runtime.worker.fetch(origin+path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(fields)});
    const tokenForm=nativeFields(await get('/settings'),'/tokens/create');
    const issued=await post('/tokens/create',{...tokenForm,name:'Synthetic NATO import QA'});assert.equal(issued.status,200);
    const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())?.[1];assert.ok(secret);
    const api=async(path,payload,operation_id=randomUUID())=>{
     const response=await runtime.worker.fetch(origin+path,{method:'POST',headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'},body:JSON.stringify({operation_id,...payload})});
     const body=await response.json();assert.equal(response.status,200,JSON.stringify(body));return body;
    };
    const supplied=structuredClone(canonical);
    if(variant==='uppercase')for(const pair of supplied.pairs)pair.value=pair.value.toUpperCase();
    if(variant==='x-space')supplied.pairs[23].value='X ray';
    if(variant==='normalized-keys')for(const pair of supplied.pairs)pair.key=pair.key.toLowerCase()+'.';
    const imported=await api('/api/v1/library/mappings',{data:supplied});
    const vid=imported.result.entity_id,library=subset(await rows(),'library_');
    const learned=await api('/api/v1/learn',{version_id:vid,pace:'light'}),sid=learned.result.entity_id;
    const next=await api('/api/v1/next',{study_id:sid});
    const answer={prompt_id:next.practice.prompt.id,answer:'Alpha'};
    const firstAnswer=await api('/api/v1/answer',answer);
    const history=await rows(),studyBefore=values(history,'practice_studies/')[0];
    assert.equal(firstAnswer.practice.attempt.grade.verdict,'Again');
    const canonicalOp=randomUUID();
    assert.equal((await api('/api/v1/library/mappings',{data:canonical},canonicalOp)).result.entity_id,vid);
    const importedRows=await rows();
    assert.equal((await api('/api/v1/library/mappings',{data:canonical},canonicalOp)).replayed,true);
    assertReplay(await rows(),importedRows);
    assert.deepEqual(subset(importedRows,'library_'),library);
    assert.deepEqual(subset(importedRows,'practice_'),subset(history,'practice_'));
    const startFields={csrf:tokenForm.csrf,operation_id:randomUUID()};
    const start=()=>entry==='focused'?post('/practice/nato/start',startFields):api('/api/v1/lessons/start',{starter:'nato'},startFields.operation_id);
    const started=await start();if(entry==='focused')assert.equal(started.status,303);
    const startedRows=await rows();await start();assertReplay(await rows(),startedRows);
    const retained=values(startedRows,'practice_studies/');assert.equal(retained.length,1);
    assert.equal(retained[0].id,sid);assert.equal(retained[0].version_hash,studyBefore.version_hash);assert.equal(retained[0].pace,'light');assert.equal(retained[0].created_ms,studyBefore.created_ms);assert.equal(retained[0].nato_all_letters,true);
    assert.deepEqual(subset(startedRows,'practice_cards/'),subset(history,'practice_cards/'));
    assert.deepEqual(subset(startedRows,'practice_attempts/'),subset(history,'practice_attempts/'));
    assert.deepEqual(subset(startedRows,'practice_payload/'),subset(history,'practice_payload/'));
    const held=values(history,'meta')[0].practice.current;
    assert.deepEqual(values(startedRows,'meta')[0].practice.current,held);
    const page=await get('/?practice=nato');assert.match(page,/data-verdict="Again"/);
    // Exercise all 26 retained pairs through native grading, including spellings
    // that must still be wrong even though case/punctuation normalize.
    for(let i=1;i<26;i++){
     const advanced=await api('/api/v1/next',{after:values(await rows(),'meta')[0].practice.current.prompt.id});
     const prompt=advanced.practice.prompt;assert.equal(prompt.version_id,vid);assert.equal(prompt.cue,supplied.pairs[i].key);
     const wrong=i===9?'Juliet':i===23?'Xray':null;
     const op=randomUUID();const payload={prompt_id:prompt.id,answer:wrong??canonical.pairs[i].value};
     const answered=await api('/api/v1/answer',payload,op);
     const graded=await rows(),attempt=answered.practice.attempt;
     assert.equal(attempt.grade.verdict,wrong?'Again':'Good');
     assert.equal(attempt.version_id,vid);assert.equal(attempt.version_hash,studyBefore.version_hash);
     assert.equal((await api('/api/v1/answer',payload,op)).replayed,true);
     assertReplay(await rows(),graded);
    }
    const finalRows=await rows();assert.deepEqual(subset(finalRows,'library_'),library,'No Version bytes, hashes, Units or metadata are rewritten.');
    assert.equal(values(finalRows,'practice_attempts/').length,26);
    for(const row of [...subset(history,'practice_attempts/'),...subset(history,'practice_payload/')])assert.deepEqual(finalRows.find(r=>r.key===row.key),row,'Historical Attempt bytes stay unchanged.');
    assert.equal(egress,0);
    report.push({variant,entry,status:'pass',same_version:true,same_study:true,retained_library_bytes:true,retained_attempt_bytes:true,strict_grades:26,replay_content_history_receipt_bytes_unchanged:true,egress});
   }finally{await runtime.mf.dispose();}
  }
 }
 return report;
}
