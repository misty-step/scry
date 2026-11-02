// Supplied external Library -> continuous native Study, against real Worker HTTP.
// Synthetic owner token stays fixture-local; no state setters or remote egress.
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {starterFixture,nativeFields} from './nato-starter.mjs';
const DAY=86400000,START=(Math.floor(Date.now()/DAY)+1)*DAY+12*3600000;
const domain=rows=>Object.fromEntries(Object.entries(rows).filter(([key])=>key!=='meta'));
const records=(rows,prefix)=>Object.values(Object.fromEntries(Object.entries(rows).filter(([key])=>key.startsWith(prefix))));
export async function continuousLibraryJourney(){
 const f=await starterFixture({nowMs:START});
 try{
  const minted=await f.post('/tokens/create',{...nativeFields(await f.get('/settings'),'/tokens/create'),name:'Synthetic continuous Library author'});
  assert.equal(minted.status,200);const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())?.[1];assert.ok(secret);
  const api=async(path,payload)=>{const r=await fetch(f.origin+path,{method:'POST',headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'},body:JSON.stringify({operation_id:randomUUID(),...payload})});const text=await r.text();assert.equal(r.status,200,text);return JSON.parse(text);};
  const mapping=async name=>(await api('/api/v1/library/mappings',{data:{title:'Synthetic private signals',name,direction:'forward',pairs:[{key:'First signal',value:name+' cedar'},{key:'Second signal',value:name+' willow'}]}})).result.entity_id;
  const canonical=await mapping('A supplied edition'),alternate=await mapping('Z alternate edition');
  assert.notEqual(canonical,alternate);
  assert.equal((await fetch(f.origin+'/api/v1/library/versions/'+canonical)).status,401,'Supplied private Library is not anonymously readable.');
  const overCapacity=(await api('/api/v1/library/versions',{data:{title:'Synthetic oversized passage',kind:'text',text:'capacity '.repeat(1100),sections:[{heading:'One supplied unit',units:['capacity '.repeat(1100)]}]}})).result.entity_id;
  const home=await f.get('/'),before=await f.rows();await f.get('/');await f.get('/recall');
  assert.deepEqual(await f.rows(),before,'Home/Recall GET do not enroll supplied versions or catalog starters.');
  assert.equal(records(before,'practice_studies/').length,0,'External authoring never enrolls by itself.');
  const start=nativeFields(home,'/practice/review/start');
  const started=await Promise.all([f.post('/practice/review/start',start),f.post('/practice/review/start',start)]);assert.ok(started.every(r=>r.status===303));
  let rows=await f.rows(),held=rows.meta.practice.current;
  assert.equal(held.prompt.version_id,canonical,'Default uses one deterministic current edition of supplied Work before importing catalog.');
  assert.equal(records(rows,'practice_studies/').length,1);assert.equal(records(rows,'practice_cards/').length,1,'Admission introduces one Unit, not an entire deck.');
  assert.doesNotMatch(await f.get('/'),/A supplied edition cedar|A supplied edition willow|Z alternate edition cedar/,'Protected recall does not expose supplied answers.');
  const admitted=domain(rows);assert.equal((await f.post('/practice/review/start',start)).status,303);assert.deepEqual(domain(await f.rows()),admitted,'Admission replay creates no duplicate studies, cards, or evidence.');
  await f.get('/');await f.get('/recall');assert.deepEqual(domain(await f.rows()),admitted,'Read holds admitted question without further introductions.');
  const sid=held.prompt.study_id,cardId=held.targets[0].card_id;
  const answer={...nativeFields(await f.get('/'),'/practice/answer'),answer:'A supplied edition cedar'};
  assert.equal((await f.post('/practice/answer',answer)).status,303);
  rows=await f.rows();const attempts=records(rows,'practice_attempts/');assert.equal(attempts.length,1);assert.match(await f.get('/'),/data-verdict="Good"/);
  const evidence=Object.fromEntries(Object.entries(rows).filter(([key])=>key.startsWith('practice_attempts/')||key.startsWith('practice_payload/')));
  await api('/api/v1/studies/pause',{study_id:sid});
  const paused=await f.rows();assert.equal(paused['practice_studies/'+sid].status,'paused');
  const next=nativeFields(await f.get('/'),'/practice/review/next');assert.equal((await f.post('/practice/review/next',next)).status,303);
  rows=await f.rows();assert.notEqual(rows.meta.practice.current.prompt.study_id,sid,'Paused Study introduces no second Unit.');
  assert.equal(records(rows,'practice_cards/').filter(c=>c.study_id===sid).length,1);
  assert.ok(!records(rows,'practice_studies/').some(s=>s.version_id===alternate||s.version_id===overCapacity),'Paused Work cannot be re-entered through an alternate; oversized text is not auto-admitted.');
  await f.clock(rows['practice_cards/'+cardId].state.schedule.due_ms+1);
  const leave={...nativeFields(await f.get('/'),'/practice/answer'),leave:rows.meta.practice.current.prompt.id};delete leave.prompt_id;
  // Use only the standard native operation envelope when leaving an unanswered prompt.
  const envelope=Object.fromEntries(Object.entries(leave).filter(([key])=>['csrf','revision','operation_id','leave'].includes(key)));
  assert.equal((await f.post('/practice/review/start',envelope)).status,303);
  rows=await f.rows();assert.equal(rows.meta.practice.current.prompt.study_id,sid,'Already scheduled due work remains reviewable while paused.');
  assert.equal(rows.meta.practice.current.targets[0].card_id,cardId);assert.equal(rows['practice_studies/'+sid].status,'paused','Due review does not resume introductions.');
  await api('/api/v1/studies/drop',{study_id:sid});
  const dropped=await f.rows();assert.equal(dropped['practice_studies/'+sid].status,'dropped');
  await f.get('/');await f.get('/recall');assert.deepEqual(domain(await f.rows()),domain(dropped),'GET after dropping held question never re-enrolls or advances.');
  assert.equal((await f.post('/practice/review/start',nativeFields(await f.get('/'),'/practice/review/start'))).status,303);
  rows=await f.rows();assert.notEqual(rows.meta.practice.current.prompt.study_id,sid,'Dropped Study is excluded even when due.');
  assert.equal(records(rows,'practice_studies/').filter(s=>[canonical,alternate].includes(s.version_id)).length,1,'Dropped Work remains blocked across editions.');
  for(const [key,value] of Object.entries(evidence))assert.deepEqual(rows[key],value,'Pause/drop/admission preserve immutable grading history, including sharded payloads.');
  assert.equal(f.egress,0);
  return {status:'pass',external_authoring_admitted:true,one_current_edition:true,private_library_and_recall:true,oversized_excluded:true,gets_read_only:true,admission_replay_once:true,paused_new_blocked_due_allowed:true,dropped_work_not_reactivated:true,history_preserved:true,egress:0};
 }finally{await f.close();}
}
