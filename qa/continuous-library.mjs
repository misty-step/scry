// Supplied external Library -> continuous native Study, against real Worker HTTP.
// Synthetic owner token stays fixture-local; no state setters or remote egress.
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {starterFixture,nativeFields} from './nato-starter.mjs';
import {enterMemory,assertStudySource} from './study-review.mjs';
import {htmlStructure} from './html-structure.mjs';
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
  // The admitted pair is studied first: its own value, never the next pair or another edition.
  assert.equal(held.phase,'study');assertStudySource(await f.get('/'),'A supplied edition cedar');
  assert.doesNotMatch(await f.get('/'),/A supplied edition willow|Z alternate edition cedar/,'Source study shows only the admitted pair.');
  const admitted=domain(rows);assert.equal((await f.post('/practice/review/start',start)).status,303);assert.deepEqual(domain(await f.rows()),admitted,'Admission replay creates no duplicate studies, cards, or evidence.');
  await f.get('/');await f.get('/recall');assert.deepEqual(domain(await f.rows()),admitted,'Read holds admitted question without further introductions.');
  const sid=held.prompt.study_id,cardId=held.targets[0].card_id;
  const quiz=await enterMemory(f);assert.doesNotMatch(quiz,/A supplied edition cedar|A supplied edition willow|Z alternate edition cedar/,'Protected recall does not expose supplied answers.');
  assert.equal(records(await f.rows(),'practice_cards/').length,1,'Trying from memory introduces nothing.');
  const answer={...nativeFields(quiz,'/practice/answer'),answer:'A supplied edition cedar'};
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
  // The next subject is held as source study; leaving it uses the same native envelope.
  assert.equal(rows.meta.practice.current.phase,'study');
  const leave={...nativeFields(await f.get('/'),'/practice/quiz'),leave:rows.meta.practice.current.prompt.id};delete leave.prompt_id;
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

// SPEC Invariant 7 and "Open into study": opening practice with nothing held
// never selects a Prompt; one deliberate native action does.
export async function openingPracticeJourney(){
 const f=await starterFixture({nowMs:START});
 try{
  const minted=await f.post('/tokens/create',{...nativeFields(await f.get('/settings'),'/tokens/create'),name:'Synthetic opening-practice author'});
  assert.equal(minted.status,200);const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())?.[1];assert.ok(secret);
  const created=await fetch(f.origin+'/api/v1/library/mappings',{method:'POST',headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'},body:JSON.stringify({operation_id:randomUUID(),data:{title:'Synthetic solo signal',name:'Only edition',direction:'forward',pairs:[{key:'Solo signal',value:'solo cedar'}]}})});
  assert.equal(created.status,200);const vid=(await created.json()).result.entity_id;
  const detail=await f.get('/library/versions/'+vid);
  const start=await f.post('/practice/study/start',nativeFields(detail,'/practice/study/start'));assert.equal(start.status,303);assert.equal(start.headers.get('location'),'/recall');
  const quiz=await enterMemory(f,'/recall');
  assert.equal((await f.post('/practice/answer',{...nativeFields(quiz,'/practice/answer'),answer:'solo cedar'})).status,303);
  const result=await f.get('/recall');assert.match(result,/data-verdict="Good"/);
  assert.equal((await f.post('/practice/next',nativeFields(result,'/practice/next'))).status,303);
  const resting=await f.rows(),card=Object.entries(resting).find(([key])=>key.startsWith('practice_cards/'));
  assert.equal(resting.meta.practice.current,null);assert.ok(!resting.meta.practice.mixed_review,'Focused practice left nothing held');
  assert.ok(card[1].state.schedule.due_ms>START,'The only Card is scheduled for later');
  const opened=path=>async()=>{const dom=htmlStructure(await f.get(path));assert.equal(dom.forms('/practice/review/start').length,1,path+' offers one explicit Study action');assert.equal(dom.forms('/practice/answer').length+dom.forms('/practice/quiz').length,0,path+' presents no Prompt');assert.equal(dom.withAttr('data-prompt-id').length,0);};
  for(const path of ['/recall','/','/lessons','/library']){await f.get(path);assert.deepEqual(await f.rows(),resting,path+' GET with nothing held and nothing due writes nothing');}
  await opened('/recall')();
  await f.clock(card[1].state.schedule.due_ms+1);
  const due=await f.rows();assert.deepEqual(due,resting,'Clock advance alone writes nothing');
  for(const path of ['/recall','/','/recall']){await f.get(path);assert.deepEqual(await f.rows(),due,path+' GET with a due Card and nothing held leaves every record and the revision unchanged');}
  await opened('/recall')();await opened('/')();
  const page=await f.get('/recall');
  assert.equal((await f.post('/practice/review/start',nativeFields(page,'/practice/review/start'))).status,303);
  const taken=await f.rows();assert.equal(taken.meta.practice.current.targets[0].card_id,card[1].id,'The deliberate action takes the due Card');
  assert.ok(taken.meta.revision>due.meta.revision);
  await f.get('/recall');assert.deepEqual(await f.rows(),taken,'Reopening shows the held Prompt without advancing');
  assert.equal(f.egress,0);
  return {status:'pass',nothing_due_get_read_only:true,due_get_read_only:true,revision_unchanged_by_get:true,one_explicit_action:true,deliberate_action_takes_due_card:true,egress:0};
 }finally{await f.close();}
}
