import assert from 'node:assert/strict';
import {randomUUID,createHash} from 'node:crypto';
import {spawn} from 'node:child_process';
import {mkdtemp,mkdir,writeFile,readFile,readdir,rm} from 'node:fs/promises';
import {join,resolve,relative} from 'node:path';
import {createRuntime,internalKey} from './runtime-helper.mjs';
import {recitationAdmission} from './recitation-admission.mjs';
const DAY=86400000,START=Math.floor(Date.now()/DAY)*DAY+12*60*60*1000;
const lines=['We conceived a patient garden,','with gentle silver branches.','The lantern guards our quiet path,','and keeps the morning bright.','We carry woven baskets,','beside the tranquil stream.','The orchard welcomes every guest,','and shelters every seed.'];
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
export async function studyJourney(){
 const temp=await mkdtemp(join(process.env.HOME,'.cache/tmp/scry-study-'));
 const name='study-'+randomUUID(),space='study-'+randomUUID(),config=join(temp,'login.json'),persist=join(temp,'state');
 let at=START,egress=0,secret,tokenId,runtime,storage,origin,bindings;
 const base={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:space,INTERNAL_KEY:internalKey};
 const transcript=[],fullTranscript=[];
 const start=async()=>{runtime=await createRuntime({port:0,name,inspect:true,testClock:true,resourcePersistencePath:persist,bindings:{...base,SCRY_TEST_NOW_MS:String(at)},egress:async()=>{egress++;throw Error('No provider egress permitted.');}});origin=(await runtime.mf.ready).origin;bindings={...base,CANONICAL_ORIGIN:origin,SCRY_TEST_NOW_MS:String(at)};runtime={...runtime,...await runtime.rebind(bindings)};storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});};
 await start();
 const rows=()=>storage.exec('SELECT key,value FROM records ORDER BY key');
 const record=async key=>JSON.parse((await storage.exec('SELECT value FROM records WHERE key=?',key))[0].value);
 const state=async()=>Object.fromEntries((await rows()).map(r=>[r.key,JSON.parse(r.value)]));
 const req=(path,payload,token=secret)=>runtime.worker.fetch(origin+path,{...(payload?{method:'POST',body:JSON.stringify(payload)}:{}),headers:{...(token?{Authorization:'Bearer '+token}:{}),'Content-Type':'application/json'}});
 const cli=(args,input='')=>new Promise((done,reject)=>{
  const p=spawn(resolve(process.env.SCRY_STUDY_CLI||'target/debug/scry'),[...args,'--json','--config',config],{env:{PATH:process.env.PATH,HOME:temp},stdio:['pipe','pipe','pipe']});let out='',err='';p.stdout.on('data',d=>out+=d);p.stderr.on('data',d=>err+=d);p.on('error',reject);p.on('exit',code=>{try{
   assert.ok(!secret||(!out.includes(secret)&&!err.includes(secret)));const value=JSON.parse(out);
   const command=['scry',...args.map(a=>a.startsWith(temp)?a.replace(temp,'<fixture>'):a),'--json'].join(' ');
   fullTranscript.push({command,exit:code,output:value});
   const a=value.practice?.attempt,c=value.practice?.correction;
   const output=a?{attempt:{id:a.id,prompt:a.prompt,raw_answer:a.raw_answer,grade:a.grade,acting_token:a.acting_token,assisted:a.assisted,effects:a.effects.map(e=>({unit_id:e.unit_id,cue_level:e.cue_level,verdict:e.verdict,assisted:e.assisted,extra_practice:e.extra_practice,scheduled:e.scheduled,before_due:e.before.schedule.due_ms,after_due:e.after.schedule.due_ms,after_cue:e.after.cue}))}}:c?{correction:c}:args[0]==='export'?{sha256:hash(out),format:value.format,studies:Object.keys(value.app?.practice?.studies||{}).length,attempts:Object.keys(value.app?.practice?.attempts||{}).length}:args[0]==='history'?{sha256:hash(out),studies:Object.keys(value.studies||{}).length,attempts:Object.keys(value.attempts||{}).length}:value;
   transcript.push({command,exit:code,output});done({code,value});
  }catch(e){reject(e);}});p.stdin.end(input);
 });
 const ok=async(args,input='')=>{const {code,value}=await cli(args,input);assert.equal(code,0,JSON.stringify(value));return value;};
 const file=async(name,data)=>{const path=join(temp,name);await writeFile(path,data);return path;};
 const html=await(await runtime.worker.fetch(origin+'/settings')).text(),form=/<form[^>]*action="\/tokens\/create"[^>]*>([\s\S]*?)<\/form>/.exec(html);assert.ok(form);
 const fields=Object.fromEntries([...form[1].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
 const minted=await runtime.worker.fetch(origin+'/tokens/create',{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...fields,name:'Study synthetic unpaid agent'})});assert.equal(minted.status,200);secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())?.[1];assert.ok(secret);tokenId=secret.slice(8,40);
 await ok(['login','--url',origin,'--token-stdin'],secret+'\n');
 let current,sid,vid,wid,version,cards=[],firstScheduled,caseStudy,caseCard,caseAttempt;
 const next=async(extra=[])=>{const args=['next',...(current?.attempt_id?['--after',current.prompt.id]:[]),...extra];const v=await ok(args);current=v.practice;return current;};
 const answer=async(text,extra=[])=>{const v=await ok(['answer',current.prompt.id,'--text-stdin',...extra],text);current={...current,attempt_id:v.practice.attempt.id};return v.practice.attempt;};
 const stopCurrent=async()=>{if(current?.prompt&&!current.attempt_id)await answer('');};
 const card=cid=>record('practice_cards/'+cid);
 const attempt=async id=>(await(await req('/api/v1/history')).json()).attempts[id];
 const attemptRows=async id=>(await rows()).filter(r=>r.key==='practice_attempts/'+id||r.key.startsWith('practice_payload/'+id+'/'));
 const clock=async value=>{at=value;bindings={...bindings,SCRY_TEST_NOW_MS:String(at)};runtime={...runtime,...await runtime.rebind(bindings)};storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});};
 const add=async(title,text,sections,strict=false)=>{const path=await file(randomUUID()+'.txt',text),sectionFile=sections?await file(randomUUID()+'.json',JSON.stringify(sections)):null;
  const v=await ok(['version','add','--title',title,'--kind','text','--text-file',path,'--name','Roman Missal 2011 metadata; caller-authored synthetic wording','--source','Authored test text; no canonical wording claimed',...(sectionFile?['--sections-file',sectionFile]:[]),...(strict?['--spelling-strict']:[])]);return v.result.entity_id;};
 const prepare=async(cid,text)=>{ // Observable Ladder journey, never editing rows.
  for(let i=0;i<6;i++){
   const cue=current.prompt.cue_level;assert.equal(cue,['read','read','first_letters','first_letters','first_letter','first_letter'][i]);
   if(cue!=='read'){assert.ok(!JSON.stringify(current.prompt).includes(text.trim()));assert.ok(!('version' in current.prompt));}
   const a=await answer(text);assert.equal(a.assisted,true);assert.ok(a.effects.every(e=>!e.scheduled));
   await next(['--card',cid]);
  }
  assert.equal(current.prompt.cue_level,'previous_line');assert.equal(current.prompt.cue,'');assert.equal(current.prompt.assisted,false);
 };
 const freshCase=async(title='Exact grader example',strict=false)=>{await stopCurrent();const tail='Synthetic '+title+' tail.\n';const v=await add(title,'We conceived a patient garden.\n'+tail,[{heading:'Synthetic clause',units:['We conceived a patient garden.\n']},{heading:'Fixture identity',units:[tail]}],strict);const s=(await ok(['learn',v])).practice.study.id;await next(['--study',s]);const p=current.prompt;const c=Object.values(await state()).find(r=>r.study_id===s&&r.unit_id&&r.state)?.id;return {s,c,p,v};};
 const practice=async(cid,text)=>{await next(['--card',cid]);return answer(text);};
 const studyCriteria=[
  async()=>{
   assert.equal((await ok(['search','apostles creed'])).empty,true);
   const sections=[{heading:'Synthetic first article',units:lines.slice(0,4).map(s=>s+'\n')},{heading:'Synthetic second article',units:lines.slice(4).map(s=>s+'\n')}];
   vid=await add("Apostles' Creed",lines.join('\n')+'\n',sections);const shown=await ok(['version','show',vid]);version=shown.version;wid=shown.work.id;
   sid=(await ok(['learn',vid,'--pace','steady'])).practice.study.id;assert.equal((await ok(['learn',vid])).practice.study.id,sid);
   assert.equal(Object.keys((await ok(['study','list'])).studies).length,1);
   await next(['--study',sid]);assert.equal(current.prompt.cue,lines[0]+'\n');await answer(lines[0]);
   await ok(['due']);await ok(['history']);await ok(['export']);assert.deepEqual((await ok(['spend','--by','day'])).entries,[]);
   const beforeRecite=await rows();assert.notEqual((await cli(['next','--after',current.prompt.id,'--study',sid,'--recite','whole'])).code,0);assert.deepEqual(await rows(),beforeRecite,'Recitation cannot bypass introductions/Pace.');
  },
  async()=>{
   // Complete each new Unit through real cues to its first Free recall answer.
   const all=version.sections.flatMap(s=>s.units);
   let first=Object.values(await state()).find(r=>r.study_id===sid&&r.unit_id&&r.state);cards.push(first.id);
   await next(['--card',first.id]); // one read success already committed
   await answer(lines[0]);await next(['--card',first.id]);
   for(let i=0;i<4;i++){await answer(lines[0]);await next(['--card',first.id]);}
   assert.equal(current.prompt.cue_level,'previous_line');await answer(lines[0]);
   for(let i=1;i<6;i++){
    await next(['--study',sid]);assert.equal(current.prompt.unit_ids[0],all[i].id);
    const c=Object.values(await state()).find(r=>r.study_id===sid&&r.unit_id===all[i].id&&r.state);cards.push(c.id);
    assert.equal(c.introduced_ms,START);await prepare(c.id,lines[i]);await answer(lines[i]);
   }
   assert.equal((await next(['--study',sid])).prompt,null);
   await clock(START+DAY-1);
   // Clear due work without creating new Units; the cap remains six.
   for(const [i,cid]of cards.entries()){await next(['--card',cid]);await answer(lines[i]);}
   assert.equal((await next(['--study',sid])).prompt,null);assert.equal((await rows()).filter(r=>r.key.startsWith('practice_cards/')).length,6);
   await clock(START+DAY);
   for(let i=6;i<8;i++){await next(['--study',sid]);assert.equal(current.prompt.unit_ids[0],all[i].id);const c=Object.values(await state()).find(r=>r.unit_id===all[i].id&&r.state);cards.push(c.id);await prepare(c.id,lines[i]);await answer(lines[i]);}
   transcript.push({observation:'Steady Pace: exactly six introductions before rolling 24h boundary; Units seven/eight only at boundary, all in immutable reading order.'});
  },
  async()=>{
   await next(['--card',cards[1]]);assert.equal(current.prompt.cue_level,'section_heading');assert.equal(current.prompt.cue,version.sections[0].heading);assert.equal(current.prompt.context,null);
   await answer(lines[1]);await next(['--card',cards[1]]);await answer(lines[1]);await next(['--card',cards[1]]);assert.equal(current.prompt.cue_level,'whole_title');assert.equal(current.prompt.cue,"Apostles' Creed");assert.equal(current.prompt.context,null);await answer(lines[1]);
   for(const route of ['/api/v1/next','/api/v1/answer','/api/v1/reveal','/api/v1/correct'])assert.equal((await req(route,{operation_id:randomUUID()},'invalid')).status,401);
  },
  async()=>{
   await next(['--study',sid,'--recite','section','--section',version.sections[0].id]);assert.equal(current.prompt.cue,version.sections[0].heading);assert.equal(current.prompt.context,null);assert.equal(current.prompt.unit_ids.length,4);assert.ok(!JSON.stringify(current.prompt).includes('conceived'));
   const a=await answer(lines.slice(0,4).join(' '));assert.ok(a.grade.units.every(u=>u.verdict==='Good'));assert.equal(a.effects.length,4);
   await next(['--study',sid,'--recite','whole']);assert.equal(current.prompt.cue,"Apostles' Creed");assert.equal(current.prompt.context,null);assert.equal(current.prompt.unit_ids.length,8);
   const bad=await answer(lines.join(' ').replace('lantern','mountain'));assert.equal(bad.grade.units.filter(u=>u.verdict==='Again').length,1);assert.equal(bad.grade.units[2].verdict,'Again');assert.ok(bad.grade.diff.some(d=>d.kind==='wrong'&&d.unit_index===2));
   await recitationAdmission({add,ok,cli,next,answer,stopCurrent,rows,record,req,sql:(...args)=>storage.exec(...args),transcript});
  },
  async()=>{
   await next(['--card',cards[4]]);const cid=cards[4],before=(await card(cid)).state.schedule;assert.equal(current.prompt.assisted,false);
   const protectedRows=await rows();for(const command of [['history'],['export'],['version','show',vid]]){assert.notEqual((await cli(command)).code,0);assert.deepEqual(await rows(),protectedRows);}
   const shown=await ok(['reveal',current.prompt.id]);assert.equal(shown.practice.assisted,true);assert.equal((await record('meta')).practice.current.revealed,true,'Assistance persisted before reveal response.');
   const a=await answer(lines[4]);assert.equal(a.assisted,true);assert.deepEqual((await card(cid)).state.schedule,before);
  },
  async()=>{
   const x=await freshCase('Once daily Free recall');caseStudy=x.s;caseCard=x.c;await prepare(x.c,'We conceived a patient garden.');const before=(await card(x.c)).state.schedule;
   const first=await answer('We conceived a patient garden.');assert.equal(first.effects[0].scheduled,true);assert.equal(first.effects[0].after.schedule.reps,before.reps+1);firstScheduled=(await card(x.c)).state.schedule;
   const extra=await practice(x.c,'We conceived a patient garden.');assert.equal(extra.assisted,false);assert.equal(extra.effects[0].extra_practice,true);assert.equal(extra.effects[0].scheduled,false);assert.deepEqual((await card(x.c)).state.schedule,firstScheduled);
  },
  async()=>{
   assert.equal((await card(caseCard)).state.cue,'section_heading');const bad=await practice(caseCard,'We imagined a patient garden.');assert.equal(bad.grade.verdict,'Again');assert.equal((await card(caseCard)).state.cue,'previous_line');assert.deepEqual((await card(caseCard)).state.schedule,firstScheduled);
   await clock(START+2*DAY);const nextDay=await practice(caseCard,'We imagined a patient garden.');assert.equal(nextDay.effects[0].scheduled,true);assert.equal(nextDay.effects[0].after.schedule.reps,firstScheduled.reps+1);assert.equal((await card(caseCard)).state.cue,'first_letter');
  },
  async()=>{
   const before=await rows();
   const disk=async()=>{const found=[];async function walk(dir){for(const e of await readdir(dir,{withFileTypes:true})){const path=join(dir,e.name);if(e.isDirectory())await walk(path);else found.push({path:relative(persist,path),sha256:hash(await readFile(path))});}}await walk(persist);return found.sort((a,b)=>a.path.localeCompare(b.path));};
   const bytes=await disk();for(let i=0;i<2;i++){await ok(['due']);await ok(['history']);await ok(['export']);await ok(['study','list']);await ok(['spend','--by','purpose']);}
   assert.deepEqual(await rows(),before,'All rows stable, including last-use, revision and operation receipts.');assert.deepEqual(await disk(),bytes,'Initialized SQLite/WAL/SHM and local R2 bytes stable.');
   transcript.push({observation:'Read-only due/history/export/list/spend',rows_sha256:hash(JSON.stringify(before)),files:bytes,limit:'Initialized object. Cold framework SQLite/WAL/SHM allocation is accepted parent behavior; literal cold-filesystem guarantee FAIL.'});
  },
  async()=>{
   const earlier=(await ok(['history'])).attempts;const original=earlier[Object.keys(earlier)[0]];
   await ok(['study','pause',caseStudy]);assert.equal((await next(['--study',caseStudy])).prompt,null,'Paused Study cannot introduce its waiting second Unit.');await ok(['study','resume',caseStudy]);await next(['--study',caseStudy]);assert.equal(current.prompt.scope.position,2);await answer('Synthetic Once daily Free recall tail.');
   await ok(['study','pause',sid]);await next(['--card',cards[0]]);await answer(lines[0]); // paused due/explicit Cards remain usable
   await ok(['study','resume',sid]);await ok(['study','drop',sid]);current=null;
   for(const command of [['version','delete',vid,'--revision','1'],['work','delete',wid,'--revision','1']]){const before=await rows();assert.notEqual((await cli(command)).code,0);assert.deepEqual(await rows(),before);}
   assert.equal((await record('study_version_refs/'+sid)),vid);assert.equal((await ok(['learn',vid])).practice.study.id,sid);assert.equal((await ok(['study','list'])).studies[sid].status,'dropped');assert.deepEqual(await attempt(original.id),original);
   assert.equal((await next(['--study',sid])).prompt,null);
  },
  async()=>{
   const history=await ok(['history']),full=await ok(['export']);assert.equal(full.format,'scry-rust-durable-sqlite-study-v1');assert.deepEqual(full.app.practice.studies,history.studies);assert.deepEqual(full.app.practice.cards,history.cards);assert.deepEqual(full.app.practice.attempts,history.attempts);assert.equal(history.versions[vid].normalized_hash,version.normalized_hash);assert.ok(Object.values(history.attempts).every(a=>a.acting_token===tokenId&&a.grade.authority.name==='exact-v2'&&a.grade.authority.grader_version));
   const before=full.app.practice;
   await runtime.mf.dispose();await start();await ok(['login','--url',origin,'--token-stdin'],secret+'\n');assert.deepEqual((await ok(['export'])).app.practice,before,'Real runtime restart retains all acknowledged practice rows.');
   const ns=await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE'),stub=ns.get(ns.idFromName(space));assert.equal((await stub.fetch('https://internal.invalid/__internal/daily',{method:'POST',headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}})).status,200);
   const bucket=await runtime.mf.getR2Bucket('SCRY_BACKUPS'),list=await bucket.list({prefix:`snapshots/${space}/`});const bytes=Buffer.from(await(await bucket.get(list.objects.at(-1).key)).arrayBuffer());const archive=JSON.parse(bytes);assert.deepEqual(archive.app.practice,before);
   const restoreToken='synthetic-study-isolated-restore-only-32chars',restored=await createRuntime({name:'restore-'+randomUUID(),bindings:{SCRY_ENV:'restore',CANONICAL_ORIGIN:'https://restore-study.invalid',SPACE_NAME:'unused-study-restore',INTERNAL_KEY:internalKey,RESTORE_TOKEN:restoreToken}});
   try{const response=await restored.worker.fetch('https://restore-study.invalid/operator/restore',{method:'POST',headers:{'x-scry-restore-token':restoreToken,'x-scry-sha256':hash(bytes)},body:bytes});const restoredBody=await response.text();assert.equal(response.status,200,restoredBody);const restoreNs=await restored.mf.getDurableObjectNamespace('LEARNING_SPACE');const restoreStub=restoreNs.get(restoreNs.idFromName('unused-study-restore'));const read=await restoreStub.fetch('https://restore-study.invalid/api/v1/export',{headers:{Authorization:'Bearer '+secret,'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});assert.equal(read.status,200);assert.deepEqual((await read.json()).app.practice,before);}finally{await restored.mf.dispose();}
   transcript.push({observation:'Full local R2 readback and fresh isolated restore retain populated Studies/Cards/Attempts',archive_sha256:hash(bytes),limits:'Local emulator; independent hosted restore UNVERIFIED.'});
  },
  async()=>{
   const full=await ok(['export']);assert.deepEqual(full.app.spend,[]);assert.deepEqual(full.app.jobs,{});assert.deepEqual(full.app.assessments,{});assert.equal(egress,0);for(const by of ['day','purpose','model']){const report=await ok(['spend','--by',by]);assert.deepEqual(report.entries,[]);}
   transcript.push({observation:'Practice egress boundary',outbound_requests:egress,spend_rows:0,jobs:0,assessments:0});
  }
 ];
 const graderCriteria=[
  async()=>{const x=await freshCase();caseStudy=x.s;caseCard=x.c;const a=await answer('WE, CONCEIVED—A\nPATIENT “GARDEN”!!!');assert.equal(a.grade.verdict,'Good');caseAttempt=a;},
  async()=>{for(const text of ['We imagined a patient garden.','We conceived a garden.','We conceived a very patient garden.','We conceived patient a garden.']){const a=await practice(caseCard,text);assert.equal(a.grade.verdict,'Again');assert.ok(a.grade.diff.some(d=>['wrong','missing','extra'].includes(d.kind)));}
   await stopCurrent();const text='orchard '.repeat(1100),v=await add('Bounded local alignment',text,[{heading:'Synthetic long Unit',units:[text]}]);const s=(await ok(['learn',v])).practice.study.id;await next(['--study',s]);const before=await rows();assert.notEqual((await cli(['answer',current.prompt.id,'--text-stdin'],text)).code,0);assert.deepEqual(await rows(),before);await answer('');},
  async()=>{const a=await practice(caseCard,'We concieved a patient garden.');assert.equal(a.grade.verdict,'Good');assert.ok(a.grade.diff.some(d=>d.kind==='slip'&&d.expected.text==='conceived'&&d.typed.text==='concieved'));},
  async()=>{const x=await freshCase('Strict spelling example',true);caseCard=x.c;const a=await answer('We concieved a patient garden.');assert.equal(a.grade.verdict,'Again');assert.ok(a.grade.diff.some(d=>d.kind==='slip'));},
  async()=>{const a=await practice(caseCard,'We concieved a harsh orchard extra.');assert.equal(a.grade.verdict,'Again');assert.ok(a.grade.diff.some(d=>d.kind==='slip'));assert.ok(a.grade.diff.some(d=>d.kind==='wrong'));const held=await ok(['next']);assert.deepEqual(held.practice.attempt,a);assert.equal(held.practice.prompt.id,current.prompt.id);caseAttempt=a;},
  async()=>{await stopCurrent();const x=await freshCase('Two wordings of one Work');caseCard=x.c;await answer('We conceived a patient garden.');const sibling=await add('Two wordings of one Work','We imagined a patient garden.\n');const a=await practice(x.c,'We imagined a patient garden.');assert.equal(a.grade.verdict,'Again');assert.equal(a.grade.wrong_version_hint.id,sibling);assert.ok(a.grade.wrong_version_hint.name);},
  async()=>{const x=await freshCase('Dictated text is typed text');caseCard=x.c;const one=await answer('We concieved a patient garden.');const two=await practice(x.c,'We concieved a patient garden.');assert.deepEqual(one.grade,two.grade);caseAttempt=two;},
  async()=>{assert.equal(caseAttempt.grade.authority.name,'exact-v2');assert.equal(caseAttempt.grade.authority.grader_version,'1;unicode=17.0.0');const again=await practice(caseCard,'We concieved a patient garden.');assert.deepEqual(again.grade,caseAttempt.grade);},
  async()=>{
   const original=await practice(caseCard,'We imagined a patient garden.');const prompt=current.prompt.id;
   const immutableRows=await attemptRows(original.id);const corrected=await ok(['correct',prompt,original.id,'--verdict','Good','--operation-id','study-correction-replay-operation']);assert.equal(corrected.practice.correction.authority,'owner');assert.deepEqual(await attempt(original.id),original);assert.deepEqual(await attemptRows(original.id),immutableRows);
   const replay=await ok(['correct',prompt,original.id,'--verdict','Good','--operation-id','study-correction-replay-operation']);assert.deepEqual(replay.practice,corrected.practice);assert.equal(replay.replayed,true);
   assert.notEqual((await cli(['correct',prompt,original.id,'--verdict','Good'])).code,0);
   const advancing=['next','--after',current.prompt.id,'--card',caseCard];const advanced=await Promise.all([cli(advancing),cli(advancing)]);assert.deepEqual(advanced.map(r=>r.code).sort(),[0,1],'Only one competing deliberate Next can advance.');current=advanced.find(r=>r.code===0).value.practice;const newPrompt=current.prompt.id,op='study-answer-parallel-replay-id';
   const before=(await rows()).filter(r=>r.key.startsWith('practice_attempts/')).length;
   const args=['answer',newPrompt,'--text-stdin','--operation-id',op];
   const lost=await req('/api/v1/answer',{operation_id:op,prompt_id:newPrompt,answer:'We concieved a patient garden.'});assert.equal(lost.status,200);assert.ok(lost.body);await lost.body.cancel(); // Discard the committed response body, then reconcile through the CLI.
   const [a,b]=await Promise.all([cli(args,'We concieved a patient garden.'),cli(args,'We concieved a patient garden.')]);assert.equal(a.code,0);assert.equal(b.code,0);assert.deepEqual(a.value.practice,b.value.practice);assert.equal((await rows()).filter(r=>r.key.startsWith('practice_attempts/')).length,before+1);
   current.attempt_id=a.value.practice.attempt.id;
   const replayed=await ok(args,'We concieved a patient garden.');assert.deepEqual(replayed.practice,a.value.practice);assert.equal(replayed.replayed,true);
   const saved=await rows();assert.notEqual((await cli(args,'A changed answer.')).code,0);assert.deepEqual(await rows(),saved);assert.notEqual((await cli(['answer',prompt,'--text-stdin'],'late answer')).code,0);assert.deepEqual(await rows(),saved);
   await next(['--card',caseCard]);assert.notEqual((await cli(['correct',prompt,original.id,'--verdict','Again'])).code,0);await answer('We conceived a patient garden.');
   assert.equal(egress,0);assert.deepEqual((await ok(['spend','--by','model'])).entries,[]);transcript.push({observation:'Parallel and lost-response answer replay',attempts_added:1,same_result:true,changed_payload:'409/nonzero',stale_prompt:'409/nonzero',original_attempt_unchanged:true});
  }
 ];
 // Recitation edit attribution through a separate active two-Unit Study.
 graderCriteria.splice(6,0,async()=>{
  await stopCurrent();const text='We conceived a patient garden,\na gentle silver orchard.\n';const v=await add('Multi Unit attribution',text,[{heading:'Synthetic orchard',units:['We conceived a patient garden,\n','a gentle silver orchard.\n']}]);const s=(await ok(['learn',v])).practice.study.id;
  for(const line of ['We conceived a patient garden,','a gentle silver orchard.']){await next(['--study',s]);const c=Object.values(await state()).find(r=>r.study_id===s&&r.unit_id===current.prompt.unit_ids[0]&&r.state);await prepare(c.id,line);await answer(line);}
  const shown=await ok(['version','show',v]);await next(['--study',s,'--recite','section','--section',shown.version.sections[0].id]);const a=await answer('We concieved a patient garden a harsh silver orchard.');assert.deepEqual(a.grade.units.map(u=>u.verdict),['Good','Again']);assert.equal(a.effects.length,2);assert.ok(a.grade.diff.some(d=>d.kind==='slip'&&d.unit_index===0));assert.ok(a.grade.diff.some(d=>d.kind==='wrong'&&d.unit_index===1));
  const before=a.effects.map(e=>e.after.schedule);
  const corrected=await ok(['correct',current.prompt.id,a.id,'--unit',a.effects[1].unit_id,'--verdict','Good']);assert.deepEqual(await attempt(a.id),a);assert.equal(corrected.practice.correction.unit_id,a.effects[1].unit_id);
  for(let i=0;i<2;i++)assert.deepEqual((await card(a.effects[i].card_id)).state.schedule,before[i],'Same-day Recitation/Correction cannot reschedule either Card.');
 });
 return {studyCriteria,graderCriteria,transcript,close:async()=>{await mkdir('target',{recursive:true});await writeFile('target/study-cli-full.json',JSON.stringify(fullTranscript,null,2)+'\n');await runtime.mf.dispose();await rm(temp,{recursive:true});}};
}

