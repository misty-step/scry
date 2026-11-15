#!/usr/bin/env node
// Paid judgment (Jev) of memorization answers on the exact built Rust/WASM, in
// real workerd with isolated SQLite and R2. Jev is a synthetic endpoint inside
// this process: every request that would leave Scry arrives here and nowhere
// else, so "nothing was sent" is counted, not inferred. No account, credential,
// real model or Owner data is used.
import assert from 'node:assert/strict';
import test from 'node:test';
import {generateKeyPairSync,sign,createHash,randomUUID} from 'node:crypto';
import {createServer} from 'node:http';
import {spawn,spawnSync} from 'node:child_process';
import {createRequire} from 'node:module';
import {mkdtemp,mkdir,readFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {dirname,join,resolve} from 'node:path';
import {createInterface} from 'node:readline';
import {once} from 'node:events';
import {createRuntime,Response,internalKey} from './runtime-helper.mjs';

const artifact=resolve(process.env.SCRY_WORKER_ARTIFACT||'build');
// The previous Worker's build, for the refusal journey: a directory holding the
// index.js and index_bg.wasm built from the source before paid judgment.
const previous=process.env.SCRY_JUDGMENT_OLD_WORKER&&resolve(process.env.SCRY_JUDGMENT_OLD_WORKER);
const shots=resolve(process.env.SCRY_JUDGMENT_SHOTS||'target/judgment');
const origin='https://scry-judgment-synthetic.invalid';
const team='judgment-synthetic.cloudflareaccess.com',audience='synthetic-judgment-audience',owner='synthetic-judgment-owner';
const JEV_URL='https://jev-synthetic.invalid/v1/decisions',JEV_KEY='synthetic-jev-key',JEV_MODEL='synthetic-jev';
const JUDGED='scry-rust-durable-sqlite-study-judged-v4';
const MARKER='judgment_compatibility/v1';
const RESERVED=500000;
const {publicKey,privateKey}=generateKeyPairSync('rsa',{modulusLength:2048});
const publicJwk={...publicKey.export({format:'jwk'}),kid:'synthetic-key',alg:'RS256',use:'sig'};
const b64=value=>Buffer.from(JSON.stringify(value)).toString('base64url');
function jwt(changes={}){
  const epoch=Math.floor(Date.now()/1000);
  const encoded=`${b64({alg:'RS256',kid:'synthetic-key',typ:'JWT'})}.${b64({iss:`https://${team}`,sub:owner,aud:[audience],iat:epoch,nbf:epoch-1,exp:epoch+600,...changes})}`;
  return `${encoded}.${sign('RSA-SHA256',Buffer.from(encoded),privateKey).toString('base64url')}`;
}
const sleep=ms=>new Promise(done=>setTimeout(done,ms));
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');

// What the synthetic Jev answers. The shape is the one Scry already reads for
// the earlier concept notebook's check.
const answers=(choice,p,identity=0.02,injection=0)=>{
  const rest=(1-p)/2,probabilities={accept:rest,reject:rest,unsure:rest};probabilities[choice]=p;
  return {verdict:{choice,probabilities},identity:{noul:identity},injection:{noul:injection}};
};
const reply=(body,{cost=0.000345,model='synthetic-jev-large',status=200}={})=>new Response(JSON.stringify({model,...(cost===null?{}:{usage:{cost}}),answers:body}),{status,headers:{'Content-Type':'application/json'}});
const accept=options=>reply(answers('accept',0.98),options);
const reject=options=>reply(answers('reject',0.97,0.9),options);
const unsure=options=>reply(answers('unsure',0.6,0.1),options);

const HARBOUR=[
  {key:'Before leaving the berth',value:'check the bilge pump'},
  {key:'In fog',value:'sound the horn'},
  {key:'A person overboard',value:'never lose sight of them'},
  {key:'At anchor overnight',value:'show an all-round white light'},
  {key:'Entering harbour',value:'keep to the starboard side'},
  {key:'Refuelling',value:'switch off the engine'},
];
const MAPPING_OBJECTIVE="Recall the value that expected_answer gives for the key in prompt. Reject another key's value, an incomplete value, or a negation or reversal of the expected practice.";

// One disposable notebook on the built Worker. `capable` binds the synthetic
// Jev key, model and endpoint, which is what makes a deployment able to judge.
async function notebook({capable=true,testClock=false,extra={},worker:build=artifact}={}){
  const name='judgment-'+randomUUID(),space='synthetic-'+randomUUID();
  const jev={sent:[],elsewhere:[],unprepared:0,respond:null};
  const egress=async request=>{
    if(request.url===`https://${team}/cdn-cgi/access/certs`)return new Response(JSON.stringify({keys:[publicJwk]}),{headers:{'Content-Type':'application/json'}});
    if(request.url!==JEV_URL||request.method!=='POST'){jev.elsewhere.push(request.url);throw new Error('Only the synthetic Jev endpoint may be reached.');}
    assert.equal(request.headers.get('Authorization'),'Bearer '+JEV_KEY);
    const payload=await request.json();jev.sent.push(payload);
    if(!jev.respond){jev.unprepared++;throw new Error('The synthetic Jev was sent a request no journey prepared.');}
    return jev.respond(payload,jev.sent.length);
  };
  const base={SCRY_ENV:'preview',CANONICAL_ORIGIN:origin,SPACE_NAME:space,ACCESS_TEAM:team,ACCESS_AUDIENCE:audience,OWNER_SUBJECT:owner,INTERNAL_KEY:internalKey,...(capable?{JEV_API_KEY:JEV_KEY,JEV_MODEL,JEV_URL}:{}),...extra};
  const runtime=await createRuntime({bindings:base,name,artifact:build,egress,inspect:true,testClock});
  let worker=runtime.worker,credential=jwt;
  const n={jev,base,name,space};
  n.rebind=async bindings=>{worker=(await runtime.rebind(bindings)).worker;};
  n.as=signer=>{credential=signer;};
  n.req=(path,init={})=>worker.fetch(origin+path,{redirect:'manual',...init,headers:{'Cf-Access-Jwt-Assertion':credential(),...init.headers}});
  n.post=(path,fields,headers={})=>n.req(path,{method:'POST',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded',...headers},body:new URLSearchParams(fields)});
  n.page=async path=>{const response=await n.req(path);assert.equal(response.status,200,`GET ${path}: ${response.status}`);return response.text();};
  // The hidden fields of the native form that posts to `action`.
  n.form=(html,action)=>{
    const found=[...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m=>m[1]===action);
    return found&&Object.fromEntries([...found[2].matchAll(/<input type="hidden" name="([^"]+)" value="([^"]*)">/g)].map(m=>[m[1],m[2]]));
  };
  n.submit=async(path,action,more={})=>{
    const fields=n.form(await n.page(path),action);assert.ok(fields,`${path} offers a form for ${action}`);
    return {fields:{...fields,...more},response:await n.post(action,{...fields,...more})};
  };
  n.fresh=async()=>({csrf:/name="csrf" value="([^"]+)"/.exec(await n.page('/settings'))[1],operation_id:'judgment-'+randomUUID()});
  n.sql=()=>runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:space});
  // Runs the alarm handler now, as the platform does at the armed time.
  n.alarm=async()=>{const ns=await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE'),stub=ns.get(ns.idFromName(space));try{return await stub.qaAlarm();}finally{stub[Symbol.dispose]?.();}};
  n.rows=async()=>(await n.sql()).exec('SELECT key,value FROM records ORDER BY key');
  n.state=async()=>{
    const rows=await n.rows();
    const group=prefix=>Object.fromEntries(rows.filter(r=>r.key.startsWith(prefix+'/')).map(r=>[r.key.slice(prefix.length+1),JSON.parse(r.value)]));
    const payload=(prefix,id,field)=>{const parts=rows.filter(r=>r.key.startsWith(`${prefix}/${id}/${field}/`)).map(r=>JSON.parse(r.value));return parts.length?parts.join(''):undefined;};
    const attempts=group('practice_attempts');
    for(const [id,attempt] of Object.entries(attempts))for(const field of ['grade','effects']){const text=payload('practice_payload',id,field);if(text!==undefined)attempt[field]=JSON.parse(text);}
    const spend=rows.filter(r=>/^spend\/\d+$/.test(r.key)).map(r=>({...JSON.parse(r.value),raw_response:payload('spend_payload',r.key.slice(6),'raw_response')??null}));
    const meta=JSON.parse(rows.find(r=>r.key==='meta')?.value||'{}');
    return {rows,keys:rows.map(r=>r.key),meta,attempts,spend,checks:group('practice_checks'),cards:group('practice_cards'),studies:group('practice_studies'),operations:group('operations'),tokens:group('owner_tokens')};
  };
  // Poll the stored notebook until it satisfies `done`.
  n.settle=async(done,what,limit=12000)=>{
    const end=Date.now()+limit;let state;
    do{state=await n.state();if(done(state))return state;await sleep(60);}while(Date.now()<end);
    throw new Error(`The notebook did not reach: ${what}. Checks: ${JSON.stringify(Object.values(state.checks).map(c=>[c.status,c.reason]))}`);
  };
  n.newest=state=>Object.values(state.attempts).sort((a,b)=>b.sequence-a.sequence)[0];
  n.checksOf=(state,attempt)=>Object.values(state.checks).filter(c=>c.attempt_id===attempt).sort((a,b)=>a.sequence-b.sequence);
  n.closed=attempt=>state=>{const checks=n.checksOf(state,attempt);return checks.length>0&&checks.every(c=>!['queued','sent'].includes(c.status));};
  n.addMapping=async(title,pairs,strict=false)=>{
    const response=await n.post('/library/mapping/add',{...await n.fresh(),data:JSON.stringify({title,direction:'forward',spelling_strict:strict,pairs})});
    assert.equal(response.status,303,await response.text());
    return /\/library\/versions\/([0-9a-f]{32})/.exec(response.headers.get('Location'))[1];
  };
  n.study=async version=>{const response=await n.post('/practice/study/start',{...await n.fresh(),version_id:version});assert.equal(response.status,303,await response.text());};
  // Answer the held Prompt as the browser's native forms do: source study is
  // left through "Try from memory" first. Returns the Attempt and the form sent.
  n.answer=async text=>{
    let html=await n.page('/recall');
    if(n.form(html,'/practice/quiz')){assert.equal((await n.post('/practice/quiz',n.form(html,'/practice/quiz'))).status,303);html=await n.page('/recall');}
    const fields={...n.form(html,'/practice/answer'),answer:text};assert.ok(fields.prompt_id,'A Quiz Prompt is held.');
    const response=await n.post('/practice/answer',fields);assert.equal(response.status,303,await response.text());
    const state=await n.state(),attempt=n.newest(state);assert.equal(attempt.prompt.id,fields.prompt_id);
    return {fields,attempt:attempt.id,card:(attempt.judgment?.targets||attempt.effects)[0].card_id,state};
  };
  n.next=async()=>{const {response}=await n.submit('/recall','/practice/next');assert.equal(response.status,303,await response.text());};
  n.turnOn=async()=>{const {response}=await n.submit('/settings','/settings/judgment');assert.equal(response.status,303,await response.text());assert.equal((await n.state()).meta.preferences.paid_judgment,true);};
  n.token=async(label,paid)=>{
    const {response}=await n.submit('/settings','/tokens/create',{name:label,...(paid?{paid:'true'}:{})});assert.equal(response.status,200);
    const secret=/<textarea[^>]*>([^<]+)<\/textarea>/.exec(await response.text())?.[1];assert.ok(secret);return secret;
  };
  n.api=(path,secret,payload)=>worker.fetch(origin+path,{redirect:'manual',...(payload?{method:'POST',body:JSON.stringify(payload)}:{}),headers:{Authorization:'Bearer '+secret,'Content-Type':'application/json'}});
  n.export=async()=>{const response=await n.req('/export');assert.equal(response.status,200,await response.clone().text());const bytes=Buffer.from(await response.arrayBuffer());assert.equal(response.headers.get('X-Scry-Sha256'),sha(bytes));return {bytes,archive:JSON.parse(bytes)};};
  // A loopback listener for the browser, the CLI and the tool server. It adds
  // what the private edge adds (the signed-in Owner's assertion and Scry's own
  // origin) and forwards everything else to the Worker unchanged.
  n.listen=async()=>{
    const server=createServer(async(request,response)=>{
      try{
        const chunks=[];for await(const chunk of request)chunks.push(chunk);
        const headers={};
        for(const header of ['content-type','accept','authorization','x-scry-request','x-scry-expected-scope','x-scry-checkpoint-mode'])if(request.headers[header])headers[header]=request.headers[header];
        if(!headers.authorization)headers['Cf-Access-Jwt-Assertion']=credential();
        if(!['GET','HEAD'].includes(request.method))headers['X-Scry-Test-Origin']=origin;
        const answer=await worker.fetch(origin+request.url,{method:request.method,redirect:'manual',headers,...(chunks.length?{body:Buffer.concat(chunks)}:{})});
        const out={};answer.headers.forEach((value,key)=>{if(!['content-length','content-encoding','transfer-encoding','connection'].includes(key))out[key]=value;});
        response.writeHead(answer.status,out);response.end(Buffer.from(await answer.arrayBuffer()));
      }catch{response.writeHead(502);response.end('The synthetic listener could not reach the Worker.');}
    });
    await new Promise(done=>server.listen(0,'127.0.0.1',done));
    n.listener=server;return `http://127.0.0.1:${server.address().port}`;
  };
  n.close=async()=>{
    try{n.listener?.closeAllConnections?.();n.listener?.close();}finally{await runtime.mf.dispose();}
    assert.deepEqual(jev.elsewhere,[],'Nothing but the synthetic Jev and the sign-in keys was ever contacted.');
    assert.equal(jev.unprepared,0,'Every request the synthetic Jev received was one a journey expected.');
  };
  n.mf=runtime.mf;
  return n;
}
// A provider that holds its answer until the journey releases it.
function held(jev,answer){
  let release,started;const sending=new Promise(done=>{started=done;});
  const gate=new Promise(done=>{release=done;});
  jev.respond=async()=>{started();await gate;return answer();};
  return {sending:Promise.race([sending,sleep(8000).then(()=>{throw new Error('The check was never sent.');})]),release};
}
const unchangedCard=(state,attempt)=>{
  const target=state.attempts[attempt].judgment.targets[0],card=state.cards[target.card_id];
  assert.deepEqual(card.state,target.before,'An answer with no Verdict leaves its Card as it was.');
  assert.equal(card.revision,target.revision+1);
};

test('switch off: nothing leaves Scry and the stored result is the one before paid judgment',{timeout:60000},async()=>{
  const n=await notebook();
  try{
    const settings=await n.page('/settings');
    assert.match(settings,/data-paid-judgment="off"/);
    assert.match(settings,/Your answer and the expected text leave Scry for that check\./);
    assert.match(settings,/Each check reserves \$0\.50 of the \$3\.50 daily allowance before it is sent\./);
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    const {attempt}=await n.answer('test the bilge pump');
    await sleep(1600); // An alarm would have claimed a queued Check by now.
    const state=await n.state(),saved=state.attempts[attempt];
    assert.equal(saved.judgment,undefined);assert.equal(saved.grade.verdict,'Again');assert.equal(saved.grade.authority.name,'exact-v2');
    assert.equal(saved.effects.length,1);assert.equal(saved.effects[0].verdict,'Again');assert.equal(saved.effects[0].superseded,undefined);
    assert.deepEqual(state.cards[saved.effects[0].card_id].state,saved.effects[0].after);
    assert.ok(!state.keys.some(key=>key.startsWith('practice_checks/')||key===MARKER),'No row of paid judgment exists.');
    assert.equal(state.meta.preferences.paid_judgment,undefined);assert.deepEqual(state.spend,[]);
    const result=await n.page('/recall');
    assert.match(result,/data-state="exact-result"/);assert.match(result,/data-verdict="Again"/);assert.doesNotMatch(result,/Judged by Jev|not the Verdict|data-poll/);
    assert.notEqual((await n.export()).archive.format,JUDGED);
    assert.equal(n.jev.sent.length,0,'With the switch off no request reaches Jev.');
  }finally{await n.close();}
});

test('switch on: the wrong key is Scry\'s own Again at once, and nothing is sent',{timeout:60000},async()=>{
  const n=await notebook();
  try{
    await n.turnOn();
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    // SPEC § Keyed mappings: exactly another pair's value is the wrong key, never sent to be judged.
    const {attempt}=await n.answer('Sound the  horn');
    await sleep(1600); // An alarm would have claimed a queued Check by now.
    const state=await n.state(),saved=state.attempts[attempt];
    assert.equal(saved.judgment,undefined);assert.equal(saved.grade.verdict,'Again');assert.equal(saved.grade.authority.name,'exact-v2');
    assert.equal(saved.effects.length,1);assert.equal(saved.effects[0].verdict,'Again');
    assert.deepEqual(state.cards[saved.effects[0].card_id].state,saved.effects[0].after);
    assert.ok(!state.keys.some(key=>key.startsWith('practice_checks/')),'No Check exists.');
    assert.deepEqual(state.spend,[],'Nothing is reserved.');
    const result=await n.page('/recall');
    assert.match(result,/data-verdict="Again"/);assert.match(result,/That answer belongs to key In fog\./);assert.doesNotMatch(result,/Judged by Jev|data-poll/);
    assert.equal(n.jev.sent.length,0,'The wrong key never reaches Jev.');
  }finally{await n.close();}
});

test('the switch is the signed-in Owner\'s, replay-safe, and a token without paid permission sends nothing',{timeout:90000},async()=>{
  const n=await notebook();
  try{
    const unpaid=await n.token('Synthetic agent; no paid permission',false),paid=await n.token('Synthetic agent; paid permission',true);
    // No token reaches the switch, whatever permission it carries.
    for(const secret of [unpaid,paid])for(const path of ['/api/v1/settings/judgment','/api/v1/judgment'])assert.equal((await n.api(path,secret,{operation_id:'judgment-'+randomUUID(),paid_judgment:'on'})).status,404);
    assert.equal((await n.api('/api/v1/settings',paid,{operation_id:'judgment-'+randomUUID(),pace:'steady',paid_judgment:'on'})).status,410,'The retired pace route refuses, and turns nothing on.');
    assert.equal((await n.state()).meta.preferences.paid_judgment,undefined);
    // The browser's fences: a forged session token and another site are refused.
    const offered=n.form(await n.page('/settings'),'/settings/judgment');assert.equal(offered.paid_judgment,'on');
    assert.equal((await n.post('/settings/judgment',{...offered,csrf:'forged-session-token-of-another-notebook'})).status,403);
    assert.equal((await n.post('/settings/judgment',offered,{'X-Scry-Test-Origin':'https://elsewhere.invalid'})).status,403);
    assert.equal((await n.post('/settings/judgment',{...offered,paid_judgment:'maybe'})).status,422);
    let state=await n.state();assert.equal(state.meta.preferences.paid_judgment,undefined);assert.ok(!state.keys.includes(MARKER));
    assert.ok(!Object.values(state.operations).some(receipt=>receipt.path==='/settings/judgment'),'A refused switch leaves no receipt.');
    // On, once. The same form again is the same operation.
    const first=await n.post('/settings/judgment',offered);assert.equal(first.status,303);assert.equal(first.headers.get('Location'),'/settings');
    const replay=await n.post('/settings/judgment',offered);assert.equal(replay.status,303);
    assert.equal((await n.post('/settings/judgment',{...offered,paid_judgment:'off'})).status,409,'The operation cannot be reused for another choice.');
    state=await n.state();assert.equal(state.meta.preferences.paid_judgment,true);assert.ok(state.keys.includes(MARKER));
    assert.equal(Object.values(state.operations).filter(receipt=>receipt.path==='/settings/judgment').length,1);
    const on=await n.page('/settings');assert.match(on,/data-paid-judgment="on"/);assert.equal(n.form(on,'/settings/judgment').paid_judgment,'off');
    // A request without paid permission is graded as if the switch were off.
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    const call=async(path,payload)=>{const response=await n.api(path,unpaid,{operation_id:'judgment-'+randomUUID(),...payload});assert.equal(response.status,200,await response.clone().text());return response.json();};
    const shown=(await call('/api/v1/next',{})).practice;
    const quiz=(await call('/api/v1/quiz',{prompt_id:shown.prompt.id})).practice;
    const answered=await call('/api/v1/answer',{prompt_id:quiz.prompt.id,answer:'test the bilge pump'});
    assert.equal(answered.practice.judgment,undefined);assert.equal(answered.practice.attempt.judgment,undefined);
    assert.equal(answered.practice.attempt.grade.verdict,'Again');assert.equal(answered.practice.attempt.effects.length,1);
    await sleep(1600);
    state=await n.state();assert.deepEqual(state.checks,{});assert.deepEqual(state.spend,[]);
    assert.equal(n.jev.sent.length,0,'Without paid permission no request reaches Jev.');
    // The same notebook where Jev can no longer be reached: nothing is sent and
    // Settings says so, though the stored switch is still on.
    await n.rebind(Object.fromEntries(Object.entries(n.base).filter(([key])=>!key.startsWith('JEV_'))));
    const unavailable=await n.page('/settings');
    assert.match(unavailable,/Paid judgment is not available here\./);assert.match(unavailable,/stays unused here/);assert.equal(n.form(unavailable,'/settings/judgment'),undefined);
    await n.next();
    const local=await n.answer('sound the bell');
    assert.equal(local.state.attempts[local.attempt].judgment,undefined);assert.equal(local.state.attempts[local.attempt].grade.verdict,'Again');
    await sleep(1600);assert.deepEqual((await n.state()).checks,{});assert.equal(n.jev.sent.length,0);
    // Off again, by the Owner.
    await n.rebind(n.base);
    const {response}=await n.submit('/settings','/settings/judgment');assert.equal(response.status,303);
    assert.equal((await n.state()).meta.preferences.paid_judgment,undefined);
    assert.match(await n.page('/settings'),/data-paid-judgment="off"/);
  }finally{await n.close();}
});

test('a deployment without the Jev bindings, and a verification space, say "not available here" and send nothing',{timeout:60000},async()=>{
  const n=await notebook({capable:false});
  try{
    const settings=await n.page('/settings');
    assert.match(settings,/Paid judgment is not available here\./);assert.match(settings,/data-paid-judgment="unavailable"/);
    assert.equal(n.form(settings,'/settings/judgment'),undefined,'No control is offered.');assert.doesNotMatch(settings,/name="paid_judgment"/);
    const before=await n.rows();
    const refused=await n.post('/settings/judgment',{...await n.fresh(),paid_judgment:'on'});
    assert.equal(refused.status,409);assert.match(await refused.text(),/Paid judgment is not available here\. Nothing was changed\./);
    assert.deepEqual(await n.rows(),before,'A refused switch writes nothing.');
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    const {attempt,state}=await n.answer('test the bilge pump');
    assert.equal(state.attempts[attempt].judgment,undefined);assert.equal(state.attempts[attempt].grade.verdict,'Again');
    await sleep(1600);
    const after=await n.state();assert.deepEqual(after.checks,{});assert.deepEqual(after.spend,[]);assert.ok(!after.keys.includes(MARKER));
    assert.equal(n.jev.sent.length,0,'A deployment that cannot reach Jev sends nothing.');
  }finally{await n.close();}
  // The verification sign-in reaches its own isolated space. Jev is bound for
  // the Owner there, and still not available to that space.
  const client='synthetic-judgment-qa.access';
  const qa=await notebook({extra:{SCRY_QA_ACCESS_CLIENT_ID:client}});
  try{
    assert.match(await qa.page('/settings'),/data-paid-judgment="off"/,'The Owner of the same deployment is offered the switch.');
    qa.as(()=>jwt({sub:'',common_name:client}));
    const settings=await qa.page('/settings');
    assert.match(settings,/Paid judgment is not available here\./);assert.equal(qa.form(settings,'/settings/judgment'),undefined);
    assert.equal((await qa.post('/settings/judgment',{...await qa.fresh(),paid_judgment:'on'})).status,409);
    assert.equal(qa.jev.sent.length,0);
  }finally{await qa.close();}
});

test('Jev decides a mismatch: accepted wording, another identity, a negation, and an accept it cannot stand behind',{timeout:90000},async()=>{
  const n=await notebook();
  try{
    await n.turnOn();
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    const judged=async(text,respond)=>{
      const sent=n.jev.sent.length;n.jev.respond=respond;
      const answered=await n.answer(text);
      // Saved first, with no Verdict, its Card untouched and one Check waiting.
      const pending=answered.state.attempts[answered.attempt];
      assert.equal(pending.grade.verdict,'Again','the local comparison');assert.deepEqual(pending.effects,[]);assert.equal(pending.judgment.decided,undefined);
      assert.deepEqual(n.checksOf(answered.state,answered.attempt).map(c=>c.status),['queued']);unchangedCard(answered.state,answered.attempt);
      const state=await n.settle(n.closed(answered.attempt),'a closed Check');
      assert.equal(n.jev.sent.length,sent+1,'One request per Check.');
      return {...answered,state,saved:state.attempts[answered.attempt],request:n.jev.sent.at(-1),check:n.checksOf(state,answered.attempt)[0]};
    };
    // An unlisted but valid wording: Good, by Jev, with the comparison kept as a comparison.
    const good=await judged('test the bilge pump',()=>accept());
    assert.deepEqual(Object.keys(good.request).sort(),['model','questions','state']);assert.equal(good.request.model,JEV_MODEL);
    assert.deepEqual(good.request.state,{prompt:'Before leaving the berth',scope:{kind:'unit',position:1},objective:MAPPING_OBJECTIVE,expected_answer:'check the bilge pump',accepted_variants:[],learner_answer:'test the bilge pump'});
    assert.deepEqual(Object.keys(good.request.questions).sort(),['identity','injection','verdict']);
    const wire=JSON.stringify(good.request);
    for(const absent of [good.attempt,good.card,good.fields.prompt_id,good.fields.csrf,'sound the horn','never lose sight',owner,n.space])assert.ok(!wire.includes(absent),'Nothing else of the notebook is sent.');
    assert.equal(good.check.status,'accepted');
    assert.equal(good.saved.judgment.policy,'recall-v1');
    assert.deepEqual(good.saved.judgment.decided.authority,{name:'jev',grader_version:'recall-v1;model=synthetic-jev-large'});
    assert.equal(good.saved.judgment.decided.verdict,'Good');assert.equal(good.saved.judgment.decided.check_id,good.check.id);
    assert.equal(good.saved.grade.verdict,'Again');assert.equal(good.saved.grade.authority.name,'exact-v2','The Diff keeps its own authority and outcome.');
    assert.equal(good.saved.effects.length,1);assert.equal(good.saved.effects[0].verdict,'Good');assert.equal(good.saved.effects[0].scheduled,true);
    assert.deepEqual(good.state.cards[good.card].state,good.saved.effects[0].after);assert.ok(good.state.cards[good.card].state.schedule.due_ms>Date.now());
    assert.deepEqual(good.state.spend.map(s=>[s.work_id,s.purpose,s.model,s.status,s.cost_micros,s.reserved_micros,s.response_outcome]),[[good.check.id,'grade.recall',JEV_MODEL,'known',345,RESERVED,'received']]);
    let html=await n.page('/recall');
    assert.match(html,/data-state="exact-result"/);assert.match(html,/data-verdict="Good"/);assert.match(html,/data-authority="jev">Judged by Jev \(recall-v1; synthetic-jev-large\)/);
    assert.match(html,/Jev accepted this answer as the same recall, though it differs from the expected text in the comparison below\./);
    assert.match(html,/Word-by-word comparison, not the Verdict/);assert.match(html,/exact-v2[^<]*comparison only, not the Verdict/);assert.doesNotMatch(html,/data-poll/);
    // A related but different identity: Again.
    await n.next();
    const other=await judged('sound the bell',()=>reject());
    assert.equal(other.request.state.prompt,'In fog');assert.equal(other.check.status,'rejected');
    assert.equal(other.saved.judgment.decided.verdict,'Again');assert.equal(other.saved.effects[0].verdict,'Again');
    assert.deepEqual(other.state.cards[other.card].state,other.saved.effects[0].after);
    html=await n.page('/recall');assert.match(html,/data-verdict="Again"/);assert.match(html,/Judged by Jev/);
    // The same words with the practice negated: the objective names it, and it is Again.
    await n.next();
    const negated=await judged('lose sight of them',()=>reject());
    assert.equal(negated.request.state.expected_answer,'never lose sight of them');
    assert.match(negated.request.state.objective,/a negation or reversal of the expected practice/);
    assert.match(negated.request.questions.identity.instructions,/negated or reversed/);
    assert.equal(negated.saved.judgment.decided.verdict,'Again');
    // An accept that carries a real risk of a different identity is not a grade.
    await n.next();
    const risky=await judged('show a white light',()=>reply(answers('accept',0.97,0.6)));
    assert.equal(risky.check.status,'unsure');assert.equal(risky.saved.judgment.decided,undefined);assert.deepEqual(risky.saved.effects,[]);unchangedCard(risky.state,risky.attempt);
    html=await n.page('/recall');
    assert.match(html,/data-state="exact-ungraded"/);assert.match(html,/Saved, not graded\./);assert.match(html,/Jev could not decide this answer\. It stays saved and not graded\./);
    assert.ok(n.form(html,'/practice/check'),'Check again is offered.');assert.doesNotMatch(html,/data-verdict=/);
    // Not graded, so still due: Next asks for it again, and an exact answer is
    // Good at once without being sent. The earlier Attempt stays as it was.
    await n.next();
    const again=await n.answer('show an all-round white light');
    assert.equal(again.card,risky.card);assert.equal(again.state.attempts[again.attempt].judgment,undefined);assert.equal(again.state.attempts[again.attempt].grade.verdict,'Good');
    assert.deepEqual(again.state.attempts[risky.attempt],risky.saved);
    await n.next();
    const exact=await n.answer('keep to the starboard side');
    assert.equal(exact.state.attempts[exact.attempt].judgment,undefined);assert.equal(exact.state.attempts[exact.attempt].grade.verdict,'Good');
    // An empty answer is a miss at once and is never sent.
    await n.next();
    const empty=await n.answer('   ');
    assert.equal(empty.state.attempts[empty.attempt].judgment,undefined);assert.equal(empty.state.attempts[empty.attempt].grade.verdict,'Again');
    await sleep(1600);assert.equal(n.jev.sent.length,4);
    // History names each outcome and who decided it.
    const history=await n.page('/history');
    for(const expected of [/Judged by Jev \(recall-v1; synthetic-jev-large\)/,/Saved, not graded/,/Word-by-word comparison, not the Verdict/])assert.match(history,expected);
  }finally{await n.close();}
});

test('a Spelling-strict mismatch is Again at once and is never sent',{timeout:60000},async()=>{
  const n=await notebook();
  try{
    await n.turnOn();
    await n.study(await n.addMapping('Synthetic strict signals',[{key:'A',value:'Alfa'},{key:'B',value:'Bravo'}],true));
    const {attempt}=await n.answer('Alpha');
    await sleep(1600);
    const state=await n.state();
    assert.equal(state.attempts[attempt].judgment,undefined);assert.equal(state.attempts[attempt].grade.verdict,'Again');assert.equal(state.attempts[attempt].effects.length,1);
    assert.deepEqual(state.checks,{});assert.deepEqual(state.spend,[]);
    assert.match(await n.page('/recall'),/data-verdict="Again"/);
    assert.equal(n.jev.sent.length,0);
  }finally{await n.close();}
});

test('a response that is not a Verdict leaves the answer saved and not graded, with its reason and its ledger entry',{timeout:120000},async()=>{
  const n=await notebook();
  try{
    await n.turnOn();
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    const modes=[
      ['malformed',()=>new Response('Controlled malformed text',{status:200}),'failed',/The check could not be read\./,{status:'unknown',cost_micros:null,response_outcome:'received'}],
      ['429',()=>new Response('{}',{status:429}),'failed',/The provider rejected the request \(429\)\./,{status:'unknown',cost_micros:null,response_outcome:'rejected'}],
      ['unsure',()=>unsure(),'unsure',/Jev could not decide this answer\./,{status:'known',cost_micros:345,response_outcome:'received'}],
      ['injection',()=>reply(answers('accept',0.98,0.02,0.7)),'unsure',/Jev could not decide this answer\./,{status:'known',cost_micros:345,response_outcome:'received'}],
      ['oversize',()=>new Response('x'.repeat(256*1024+1),{status:200}),'unknown',/could not be safely read; its paid outcome is unknown/,{status:'unknown',cost_micros:null,response_outcome:null}],
    ];
    const mismatches=['test the bilge pump','sound the bell','keep them in view','show a white light','keep right'];
    for(const [index,[mode,respond,status,reason,ledger]] of modes.entries()){
      if(index>0)await n.next();
      n.jev.respond=respond;
      const {attempt}=await n.answer(mismatches[index]);
      const state=await n.settle(n.closed(attempt),`${mode}: a closed Check`);
      const check=n.checksOf(state,attempt)[0],saved=state.attempts[attempt];
      assert.equal(check.status,status,mode);assert.match(check.reason,reason,mode);
      assert.equal(saved.judgment.decided,undefined,mode);assert.deepEqual(saved.effects,[],mode);unchangedCard(state,attempt);
      const row=state.spend.find(s=>s.work_id===check.id);
      assert.equal(row.purpose,'grade.recall');assert.equal(row.reserved_micros,RESERVED);
      assert.deepEqual({status:row.status,cost_micros:row.cost_micros,response_outcome:row.response_outcome??null},ledger,mode);
      if(ledger.response_outcome)assert.equal(row.response_hash,sha(row.raw_response),`${mode}: the raw response is kept in the ledger`);
      const html=await n.page('/recall');
      assert.match(html,/data-state="exact-ungraded"/,mode);assert.match(html,/Saved, not graded\./,mode);assert.match(html,reason,mode);
      assert.match(html,/This answer has no Verdict and its schedule is unchanged\./,mode);assert.ok(n.form(html,'/practice/check'),mode);
      assert.equal(n.jev.sent.length,index+1,`${mode}: one request, never retried`);
    }
    await sleep(1600);assert.equal(n.jev.sent.length,modes.length,'Scry never retries by itself.');
    // Unknown and unconfirmed spend keeps its reservation in the report.
    const report=await(await n.req('/api/v1/spend?by=purpose')).json();
    assert.equal(report.entries.length,modes.length);
    assert.ok(report.entries.every(entry=>entry.purpose==='grade.recall'&&entry.work_kind==='study_check'),JSON.stringify(report.entries[0]));
    assert.equal(report.entries.filter(entry=>entry.status==='unknown'&&entry.unknown_held_micros===RESERVED).length,3);
  }finally{await n.close();}
});

// Until the earlier concept notebook became history, this transport boundary
// was exercised through its check. A Check crosses the same one.
test('a rejection shaped like a Verdict decides nothing and keeps its measured cost',{timeout:90000},async()=>{
  const n=await notebook();
  try{
    await n.turnOn();
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    const modes=[
      ['429 shaped as a Verdict',()=>accept({status:429,cost:0.000456}),'failed',/The provider rejected the request \(429\)\./,{status:'known',cost_micros:456,response_outcome:'rejected'}],
    ];
    for(const [index,[mode,respond,status,reason,ledger]] of modes.entries()){
      n.jev.respond=respond;
      const {attempt}=await n.answer('test the bilge pump');
      const state=await n.settle(n.closed(attempt),`${mode}: a closed Check`);
      const check=n.checksOf(state,attempt)[0],saved=state.attempts[attempt];
      assert.equal(check.status,status,mode);assert.match(check.reason,reason,mode);
      assert.equal(saved.judgment.decided,undefined,mode);assert.deepEqual(saved.effects,[],mode);unchangedCard(state,attempt);
      const row=state.spend.find(s=>s.work_id===check.id);
      assert.equal(row.purpose,'grade.recall');assert.equal(row.reserved_micros,RESERVED);
      assert.deepEqual({status:row.status,cost_micros:row.cost_micros,response_outcome:row.response_outcome??null},ledger,mode);
      assert.equal(row.response_hash,sha(row.raw_response),`${mode}: the raw response is kept in the ledger`);
      const html=await n.page('/recall');
      assert.match(html,/data-state="exact-ungraded"/,mode);assert.match(html,/Saved, not graded\./,mode);assert.ok(n.form(html,'/practice/check'),mode);
      assert.equal(n.jev.sent.length,index+1,`${mode}: one request, never retried`);
    }
    await sleep(1600);assert.equal(n.jev.sent.length,modes.length,'Scry never retries by itself.');
    const report=await(await n.req('/api/v1/spend?by=purpose')).json();
    assert.equal(report.entries.filter(entry=>entry.status==='known'&&entry.cost_micros===456&&entry.unknown_held_micros===0).length,1,JSON.stringify(report.entries));
  }finally{await n.close();}
});

test('a reached allowance refuses without sending, says why, and a later Check again succeeds',{timeout:90000},async()=>{
  const n=await notebook();
  try{
    await n.turnOn();
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    // One measured check uses 3.20 of the 3.50 allowance: no further reservation fits.
    n.jev.respond=()=>accept({cost:3.2});
    const first=await n.answer('test the bilge pump');
    let state=await n.settle(n.closed(first.attempt),'the first Verdict');
    assert.equal(state.spend[0].cost_micros,3_200_000);assert.equal(n.jev.sent.length,1);
    await n.next();
    n.jev.respond=null; // Anything sent now would be a failure of the cap.
    const capped=await n.answer('sound the bell');
    const check=n.checksOf(capped.state,capped.attempt)[0];
    assert.equal(check.status,'refused');assert.equal(check.lease,undefined);
    assert.match(check.reason,/Today's checking allowance is used\. Your answer is saved and not graded\. Check it again later\./);
    unchangedCard(capped.state,capped.attempt);assert.equal(capped.state.spend.length,1,'Nothing was reserved.');
    let html=await n.page('/recall');
    assert.match(html,/data-state="exact-ungraded"/);assert.match(html,/checking allowance is used\. Your answer is saved and not graded\./);
    // Asking again while the cap holds is refused and writes nothing.
    const before=await n.rows();
    const refused=await n.post('/practice/check',n.form(html,'/practice/check'));
    assert.equal(refused.status,429);assert.match(await refused.text(),/checking allowance is used\./);
    assert.deepEqual(await n.rows(),before);
    await sleep(1600);assert.equal(n.jev.sent.length,1,'A reached cap sends nothing.');
    // A day later the measured spend has left the rolling window. The harness
    // ages that one ledger row rather than waiting a day; nothing else is edited.
    const storage=await n.sql(),key=state.rows.find(r=>/^spend\/\d+$/.test(r.key)).key;
    const row=JSON.parse((await storage.exec('SELECT value FROM records WHERE key=?',key))[0].value);
    row.created_ms-=25*60*60*1000;
    await storage.exec('UPDATE records SET value=? WHERE key=?',JSON.stringify(row),key);
    n.jev.respond=()=>reject();
    html=await n.page('/recall');
    const again=await n.post('/practice/check',n.form(html,'/practice/check'));assert.equal(again.status,303,await again.text());
    state=await n.settle(n.closed(capped.attempt),'the later check');
    assert.deepEqual(n.checksOf(state,capped.attempt).map(c=>c.status),['refused','rejected']);
    assert.equal(state.attempts[capped.attempt].judgment.decided.verdict,'Again');assert.equal(n.jev.sent.length,2);
    assert.equal(Object.keys(state.attempts).length,2,'Check again is never a second Attempt.');
  }finally{await n.close();}
});

test('each Check is sent once: replay, a second tab, a stale response and a restart mid-send',{timeout:120000},async()=>{
  // Replay and a second tab.
  let n=await notebook();
  try{
    await n.turnOn();
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    let gate=held(n.jev,()=>unsure());
    const sent=await n.answer('test the bilge pump');
    // The same operation again, while queued or sent: the same Attempt, no new Check.
    for(let i=0;i<3;i++)assert.equal((await n.post('/practice/answer',sent.fields)).status,303);
    await gate.sending;
    assert.equal((await n.post('/practice/answer',sent.fields)).status,303);
    // Another tab: a different operation for the same Prompt, and a second check, are refused.
    assert.equal((await n.post('/practice/answer',{...sent.fields,operation_id:'judgment-'+randomUUID()})).status,409);
    const other=await n.post('/practice/check',{...await n.fresh(),attempt_id:sent.attempt});
    assert.equal(other.status,409);assert.match(await other.text(),/This answer is already being checked\./);
    let state=await n.state();
    assert.equal(Object.keys(state.attempts).length,1);assert.deepEqual(n.checksOf(state,sent.attempt).map(c=>c.status),['sent']);
    assert.deepEqual(state.spend.map(s=>[s.status,s.cost_micros,s.reserved_micros]),[['unknown',null,RESERVED]],'The reservation is saved before the request leaves.');
    gate.release();
    state=await n.settle(n.closed(sent.attempt),'the released check');
    // And again once it is closed: the receipt answers, nothing is sent.
    assert.equal((await n.post('/practice/answer',sent.fields)).status,303);
    const html=await n.page('/recall'),again=n.form(html,'/practice/check');
    gate=held(n.jev,()=>accept());
    for(let i=0;i<3;i++)assert.equal((await n.post('/practice/check',again)).status,303);
    await gate.sending;assert.equal((await n.post('/practice/check',again)).status,303);gate.release();
    state=await n.settle(n.closed(sent.attempt),'the second check');
    await sleep(1600);
    assert.equal(n.jev.sent.length,2,'Two Checks, two requests, however often each operation was repeated.');
    assert.deepEqual(n.checksOf(state,sent.attempt).map(c=>c.status),['unsure','accepted']);assert.equal(Object.keys(state.attempts).length,1);assert.equal(state.spend.length,2);
  }finally{await n.close();}
  // A response that arrives after the Card was answered again.
  n=await notebook();
  try{
    await n.turnOn();
    const secret=await n.token('Synthetic agent; no paid permission',false);
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    const gate=held(n.jev,()=>accept());
    const first=await n.answer('test the bilge pump');
    await gate.sending;
    // The Owner names that Card outright and answers it exactly before Jev replies.
    const call=async(path,payload)=>{const response=await n.api(path,secret,{operation_id:'judgment-'+randomUUID(),...payload});assert.equal(response.status,200,await response.clone().text());return(await response.json()).practice;};
    const shown=await call('/api/v1/next',{after:first.fields.prompt_id,card_id:first.card});
    assert.equal(shown.prompt.unit_ids.length,1);assert.notEqual(shown.prompt.id,first.fields.prompt_id);
    const second=(await call('/api/v1/answer',{prompt_id:shown.prompt.id,answer:'check the bilge pump'})).attempt;
    assert.equal(second.grade.verdict,'Good');assert.equal(second.judgment,undefined);assert.equal(second.effects[0].card_id,first.card);
    const newer=(await n.state()).cards[first.card];
    gate.release();
    const state=await n.settle(n.closed(first.attempt),'the stale Verdict');
    const stale=state.attempts[first.attempt];
    assert.equal(stale.judgment.decided.verdict,'Good','The Verdict is recorded.');
    assert.equal(stale.effects.length,1);assert.equal(stale.effects[0].superseded,true);assert.equal(stale.effects[0].scheduled,false);
    assert.deepEqual(stale.effects[0].before,stale.effects[0].after);
    assert.deepEqual(state.cards[first.card],newer,'The Card is exactly as the newer Attempt left it.');
    assert.deepEqual(state.cards[first.card].state,second.effects[0].after);
    assert.equal(state.spend[0].status,'known');assert.equal(n.jev.sent.length,1);
    assert.match(await n.page('/history'),/A newer answer was already recorded/);
  }finally{await n.close();}
  // A restart while the request is in flight.
  n=await notebook({testClock:true});
  try{
    await n.turnOn();
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    const gate=held(n.jev,()=>accept());
    const lost=await n.answer('test the bilge pump');
    await gate.sending;
    let state=await n.state();
    assert.deepEqual(n.checksOf(state,lost.attempt).map(c=>c.status),['sent']);assert.equal(state.spend.length,1);
    // The Worker restarts with its clock ninety-one seconds on. Nothing answers the request.
    await n.rebind({...n.base,SCRY_TEST_NOW_MS:String(Date.now()+91_000)});
    // A read recovers nothing: reading only loads and renders.
    let html=await n.page('/recall');
    assert.deepEqual(n.checksOf(await n.state(),lost.attempt).map(c=>c.status),['sent']);
    // The alarm armed before the send recovers it, once its outcome can no longer arrive.
    assert.equal(await n.alarm(),200);
    html=await n.page('/recall');
    state=await n.state();
    const check=n.checksOf(state,lost.attempt)[0];
    assert.equal(check.status,'unknown');assert.match(check.reason,/stopped before its outcome was confirmed\. Its reserved usage is retained\./);
    assert.deepEqual(state.spend.map(s=>[s.work_id,s.status,s.cost_micros,s.reserved_micros]),[[check.id,'unknown',null,RESERVED]],'The reservation is kept as unknown spend.');
    assert.equal(state.attempts[lost.attempt].judgment.decided,undefined);unchangedCard(state,lost.attempt);
    assert.match(html,/data-state="exact-ungraded"/);assert.match(html,/stopped before its outcome was confirmed/);assert.ok(n.form(html,'/practice/check'));
    const report=await(await n.req('/api/v1/spend?by=purpose')).json();
    assert.equal(report.entries[0].unknown_held_micros,RESERVED);
    // Back on the wall clock, with every alarm free to fire: still one request.
    await n.rebind(n.base);
    await n.page('/recall');await sleep(2500);await n.page('/recall');
    gate.release();await sleep(500);
    state=await n.state();
    assert.equal(n.jev.sent.length,1,'An interrupted send is never sent again.');
    assert.deepEqual(n.checksOf(state,lost.attempt).map(c=>c.status),['unknown']);assert.equal(state.spend.length,1);assert.equal(state.spend[0].status,'unknown');
  }finally{await n.close();}
});

// No read recovers a Check, so the alarm stored before the send has to: by the
// platform's own timer, across a restart, with no request of any kind after it.
test('a sent Check is recovered by its stored alarm after a restart, with no request at all',{timeout:240000},async()=>{
  const n=await notebook();
  try{
    await n.turnOn();
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    const gate=held(n.jev,()=>accept());
    const lost=await n.answer('test the bilge pump');
    await gate.sending;
    const [sent]=n.checksOf(await n.state(),lost.attempt);
    assert.equal(sent.status,'sent');
    await n.rebind(n.base);
    // Nothing is requested and nothing answers. Wait out the ninety seconds.
    await sleep(Math.max(0,sent.started_ms+90_000-Date.now())+8000);
    const state=await n.state(),[check]=n.checksOf(state,lost.attempt);
    assert.equal(check.status,'unknown','The stored alarm fired by itself and recovered the Check.');
    assert.deepEqual(state.spend.map(s=>[s.work_id,s.status,s.reserved_micros]),[[check.id,'unknown',RESERVED]],'The reservation is kept as unknown spend.');
    unchangedCard(state,lost.attempt);
    gate.release();await sleep(500);
    assert.equal(n.jev.sent.length,1,'An interrupted send is never sent again.');
  }finally{await n.close();}
});

test('Check again is a new Check of the same Attempt, gives the Verdict once, and stops at three',{timeout:120000},async()=>{
  const n=await notebook();
  try{
    await n.turnOn();
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR.slice(0,2)));
    const again=async attempt=>{
      const html=await n.page('/recall'),form=n.form(html,'/practice/check');assert.ok(form,'Check again is offered.');assert.equal(form.attempt_id,attempt);
      assert.match(html,/Check again asks Jev once more\. It reserves \$0\.50 of today's allowance\./);
      const response=await n.post('/practice/check',form);assert.equal(response.status,303,await response.text());
      return n.settle(state=>n.closed(attempt)(state)&&n.checksOf(state,attempt).at(-1).sequence>0&&!n.checksOf(state,attempt).some(c=>['queued','sent'].includes(c.status)),'the repeated check');
    };
    // Resolved on the third check: the Verdict is given once, by that Check.
    n.jev.respond=()=>unsure();
    const first=await n.answer('test the bilge pump');
    await n.settle(n.closed(first.attempt),'the first check');
    await again(first.attempt);
    n.jev.respond=()=>accept();
    let state=await again(first.attempt);
    let checks=n.checksOf(state,first.attempt);
    assert.deepEqual(checks.map(c=>c.status),['unsure','unsure','accepted']);assert.equal(new Set(checks.map(c=>c.id)).size,3);
    assert.equal(state.attempts[first.attempt].judgment.decided.check_id,checks[2].id);assert.equal(state.attempts[first.attempt].judgment.decided.verdict,'Good');
    assert.equal(Object.keys(state.attempts).length,1);assert.equal(n.jev.sent.length,3);
    assert.ok(n.jev.sent.every(sent=>JSON.stringify(sent)===JSON.stringify(n.jev.sent[0])),'The same saved answer is checked each time.');
    let html=await n.page('/recall');assert.match(html,/data-verdict="Good"/);assert.equal(n.form(html,'/practice/check'),undefined);
    let before=await n.rows();
    let refused=await n.post('/practice/check',{...await n.fresh(),attempt_id:first.attempt});
    assert.equal(refused.status,409);assert.match(await refused.text(),/This answer already has its Verdict\./);assert.deepEqual(await n.rows(),before);
    // Three checks with no Verdict: the fourth is refused and nothing is written.
    await n.next();
    n.jev.respond=()=>unsure();
    const second=await n.answer('sound the bell');
    await n.settle(n.closed(second.attempt),'the first check');
    await again(second.attempt);state=await again(second.attempt);
    assert.deepEqual(n.checksOf(state,second.attempt).map(c=>c.status),['unsure','unsure','unsure']);
    html=await n.page('/recall');
    assert.match(html,/Three checks are saved for this answer\. It stays saved and not graded\./);assert.equal(n.form(html,'/practice/check'),undefined);
    before=await n.rows();
    refused=await n.post('/practice/check',{...await n.fresh(),attempt_id:second.attempt});
    assert.equal(refused.status,422);assert.match(await refused.text(),/Three checks are saved for this answer\./);
    assert.deepEqual(await n.rows(),before);
    await sleep(1600);assert.equal(n.jev.sent.length,6);unchangedCard(await n.state(),second.attempt);
  }finally{await n.close();}
});

test('a Card whose check is open is not served again; the pending result stays instead of rest',{timeout:120000},async()=>{
  const n=await notebook();
  try{
    await n.turnOn();
    const secret=await n.token('Synthetic agent; no paid permission',false);
    const due=async()=>(await n.api('/api/v1/due',secret)).json();
    await n.study(await n.addMapping('Synthetic single practice',HARBOUR.slice(0,1)));
    const gate=held(n.jev,()=>reject());
    const pending=await n.answer('test the bilge pump');
    let html=await n.page('/recall');
    assert.match(html,/data-state="exact-checking" data-prompt-id="[0-9a-f]{32}" data-poll/);assert.match(html,/Your answer is saved/);assert.match(html,/Checking it\./);
    assert.match(html,/<a class="text-link" href="\/recall">Refresh<\/a>/);assert.doesNotMatch(html,/data-verdict=|Saved, not graded/);
    // Queued, then sent: Next presents nothing new and the same result is held.
    for(const sent of [false,true]){
      if(sent)await gate.sending;
      await n.next();
      const state=await n.state();
      assert.equal(state.meta.practice.current.prompt.id,pending.fields.prompt_id);assert.equal(state.meta.practice.current.attempt_id,pending.attempt);
      assert.equal(Object.keys(state.attempts).length,1);
      html=await n.page('/recall');
      assert.match(html,/data-state="exact-checking"/);assert.doesNotMatch(html,/Rest for now/);
      const listed=await due();
      assert.deepEqual(listed.due,[],'The waiting Card is not offered as due.');assert.deepEqual(listed.checking,[pending.card]);
    }
    // Another Study has something to give: Next goes there, never to the waiting Card.
    const other=await n.addMapping('Synthetic second practice',[{key:'Leaving the mooring',value:'cast off the stern line last'}]);
    const learned=await n.api('/api/v1/learn',secret,{operation_id:'judgment-'+randomUUID(),version_id:other});assert.equal(learned.status,200);
    await n.next();
    let state=await n.state();
    assert.notEqual(state.meta.practice.current.targets[0].card_id,pending.card);assert.equal(state.meta.practice.current.prompt.version_id,other);
    const exact=await n.answer('cast off the stern line last');
    assert.equal(exact.state.attempts[exact.attempt].grade.verdict,'Good');
    // Nothing else is due in these Studies and the first is still being checked.
    await n.next();
    state=await n.state();assert.equal(state.meta.practice.current,null);
    assert.deepEqual((await due()).checking,[pending.card]);
    assert.equal(n.jev.sent.length,1);
    // Jev rejects: the Card has its Verdict and is served again in its turn.
    gate.release();
    state=await n.settle(n.closed(pending.attempt),'the Verdict');
    assert.equal(state.attempts[pending.attempt].judgment.decided.verdict,'Again');assert.equal(state.attempts[pending.attempt].effects[0].superseded,undefined);
    assert.equal((await due()).checking,undefined);
  }finally{await n.close();}
});

test('the API, the CLI and the tool server report the judgment state and can ask for a check again',{timeout:180000},async t=>{
  const scry=process.env.SCRY_STUDY_CLI?resolve(process.env.SCRY_STUDY_CLI):resolve('target/debug/scry');
  const mcp=process.env.SCRY_MCP_BIN?resolve(process.env.SCRY_MCP_BIN):join(dirname(scry),'scry-mcp');
  if(!process.env.SCRY_STUDY_CLI&&!process.env.SCRY_MCP_BIN){const built=spawnSync('cargo',['build','--locked','--bin','scry','--bin','scry-mcp'],{encoding:'utf8'});assert.equal(built.status,0,built.stderr);}
  const directory=await mkdtemp(join(tmpdir(),'scry-judgment-agents-')),config=join(directory,'login.json');
  const n=await notebook();let child,closed;
  t.after(async()=>{
    try{if(child){child.stdin.end();await Promise.race([closed,sleep(2000)]);child.kill('SIGKILL');}}
    finally{try{await n.close();}finally{await rm(directory,{recursive:true,force:true});}}
  });
  await n.turnOn();
  const unpaid=await n.token('Synthetic agent; no paid permission',false),paid=await n.token('Synthetic agent; paid permission',true);
  const paidId=Object.values((await n.state()).tokens).find(token=>token.paid).id;
  const post=async(path,payload,secret=paid)=>{const response=await n.api(path,secret,payload);return {status:response.status,body:await response.json()};};
  const ok=async(path,payload,secret=paid)=>{const {status,body}=await post(path,{operation_id:'judgment-'+randomUUID(),...payload},secret);assert.equal(status,200,JSON.stringify(body));return body;};
  // The API.
  const version=(await ok('/api/v1/library/mappings',{data:{title:'Synthetic harbour practice',direction:'forward',spelling_strict:false,pairs:HARBOUR}})).result.entity_id;
  const study=(await ok('/api/v1/learn',{version_id:version})).result.entity_id;
  let shown=(await ok('/api/v1/next',{study_id:study})).practice;
  let quiz=(await ok('/api/v1/quiz',{prompt_id:shown.prompt.id})).practice;
  n.jev.respond=()=>unsure();
  const first={operation_id:'judgment-api-answer-'+randomUUID(),prompt_id:quiz.prompt.id,answer:'test the bilge pump'};
  let answered=await post('/api/v1/answer',first);
  assert.equal(answered.status,200);assert.equal(answered.body.replayed,false);
  const attempt=answered.body.result.entity_id;
  assert.deepEqual(Object.keys(answered.body.practice.judgment).sort(),['authority','check_id','reason','state','verdict']);
  assert.equal(answered.body.practice.judgment.state,'checking');assert.equal(answered.body.practice.judgment.verdict,null);assert.equal(answered.body.practice.judgment.authority,null);
  assert.equal(answered.body.practice.attempt.id,attempt);assert.equal(answered.body.practice.attempt.grade.verdict,'Again','grade is the local comparison');assert.deepEqual(answered.body.practice.attempt.effects,[]);
  let state=await n.settle(n.closed(attempt),'the API check');
  assert.equal(n.checksOf(state,attempt)[0].acting_token,paidId,'The Check records the token that asked for it.');
  // The same operation again reports where the answer stands now, and sends nothing.
  answered=await post('/api/v1/answer',first);
  assert.equal(answered.body.replayed,true);assert.equal(answered.body.practice.judgment.state,'ungraded');assert.match(answered.body.practice.judgment.reason,/Jev could not decide this answer/);
  const history=await(await n.api('/api/v1/history',paid)).json();
  assert.equal(Object.values(history.checks).filter(check=>check.attempt_id===attempt).length,1);assert.equal(history.attempts[attempt].judgment.policy,'recall-v1');
  // Check again needs paid permission; with it, it is a new Check of the same Attempt.
  let before=await n.rows();
  const refused=await post('/api/v1/check',{operation_id:'judgment-'+randomUUID(),attempt_id:attempt},unpaid);
  assert.equal(refused.status,403);assert.match(JSON.stringify(refused.body),/no paid permission/);
  assert.deepEqual((await n.rows()).filter(row=>!row.key.startsWith('owner_tokens/')),before.filter(row=>!row.key.startsWith('owner_tokens/')));
  n.jev.respond=()=>accept();
  const recheck={operation_id:'judgment-api-check-'+randomUUID(),attempt_id:attempt};
  let checked=await post('/api/v1/check',recheck);
  assert.equal(checked.status,200);assert.equal(checked.body.practice.judgment.state,'checking');assert.notEqual(checked.body.practice.judgment.check_id,answered.body.practice.judgment.check_id);
  await n.settle(n.closed(attempt),'the repeated API check');
  checked=await post('/api/v1/check',recheck);
  assert.equal(checked.body.replayed,true);assert.equal(checked.body.practice.judgment.state,'decided');assert.equal(checked.body.practice.judgment.verdict,'Good');
  assert.deepEqual(checked.body.practice.judgment.authority,{name:'jev',grader_version:'recall-v1;model=synthetic-jev-large'});
  assert.equal(n.jev.sent.length,2);
  // The CLI, through a loopback listener, logged in with the paid Owner token.
  const url=await n.listen();
  const cli=(args,input='')=>new Promise((done,failed)=>{
    const run=spawn(scry,[...args,'--json','--config',config],{env:{PATH:process.env.PATH,HOME:directory},stdio:['pipe','pipe','pipe']});let out='',err='';
    run.stdout.on('data',v=>out+=v);run.stderr.on('data',v=>err+=v);run.on('error',failed);
    run.on('close',code=>{try{assert.ok(!out.includes(paid)&&!err.includes(paid),'The CLI never prints the Owner token.');done({code,value:JSON.parse(out)});}catch(error){failed(error);}});
    run.stdin.on('error',error=>{if(error.code!=='EPIPE')failed(error);});run.stdin.end(input);
  });
  assert.equal((await cli(['login','--url',url,'--token-stdin'],paid+'\n')).code,0);
  shown=(await cli(['next','--after',quiz.prompt.id,'--study',study])).value.practice;
  quiz=(await cli(['quiz',shown.prompt.id])).value.practice;
  n.jev.respond=()=>unsure();
  const operation='judgment-cli-answer-'+randomUUID();
  let ran=await cli(['answer',quiz.prompt.id,'--text-stdin','--operation-id',operation],'sound the bell');
  assert.equal(ran.code,0,'A saved answer that is being checked is not a failure.');
  assert.equal(ran.value.practice.judgment.state,'checking');
  const second=ran.value.result.entity_id;
  await n.settle(n.closed(second),'the CLI check');
  ran=await cli(['answer',quiz.prompt.id,'--text-stdin','--operation-id',operation],'sound the bell');
  assert.equal(ran.code,0,'Saved and not graded is not a failure either.');assert.equal(ran.value.practice.judgment.state,'ungraded');
  n.jev.respond=()=>reject();
  ran=await cli(['check',second]);
  assert.equal(ran.code,0);assert.equal(ran.value.practice.judgment.state,'checking');assert.equal(ran.value.practice.attempt.id,second);
  await n.settle(n.closed(second),'the repeated CLI check');
  ran=await cli(['answer',quiz.prompt.id,'--text-stdin','--operation-id',operation],'sound the bell');
  assert.equal(ran.value.practice.judgment.state,'decided');assert.equal(ran.value.practice.judgment.verdict,'Again');
  ran=await cli(['check',second]);
  assert.equal(ran.code,1,'A refusal exits non-zero.');assert.ok(ran.value.error);
  assert.equal((await cli(['check'])).code,1);
  assert.equal(n.jev.sent.length,4);
  // The tool server, over stdio, with the same login.
  child=spawn(mcp,['--config',config],{env:{PATH:process.env.PATH,HOME:directory},stdio:['pipe','pipe','pipe']});closed=once(child,'close');
  let serial=0;const waiting=new Map();
  createInterface({input:child.stdout}).on('line',line=>{assert.ok(!line.includes(paid),'The tool server never emits the Owner token.');const value=JSON.parse(line);waiting.get(value.id)?.(value);waiting.delete(value.id);});
  const request=(method,params={})=>new Promise(done=>{const id=++serial;waiting.set(id,done);child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n');});
  const tool=async(name,args)=>(await request('tools/call',{name,arguments:args})).result;
  assert.equal((await request('initialize',{protocolVersion:'2025-11-25',capabilities:{},clientInfo:{name:'judgment-journey',version:'1'}})).result.protocolVersion,'2025-11-25');
  child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized'})+'\n');
  const tools=(await request('tools/list')).result.tools,listed=name=>tools.find(entry=>entry.name===name);
  assert.deepEqual(listed('scry_check').inputSchema.required,['operation_id','attempt_id']);assert.match(listed('scry_check').description,/Check again/);
  assert.deepEqual(listed('scry_answer').inputSchema.required,['operation_id','prompt_id','answer']);assert.equal(listed('scry_answer').inputSchema.properties.attempt_id,undefined);
  for(const name of ['scry_answer','scry_check'])for(const combinator of ['oneOf','anyOf','allOf','not'])assert.equal(listed(name).inputSchema[combinator],undefined,`${name} has no top-level ${combinator}`);
  const id=label=>`judgment-mcp-${label}-${randomUUID()}`;
  shown=(await tool('scry_next',{operation_id:id('next'),after:quiz.prompt.id,study_id:study})).structuredContent.practice;
  quiz=(await tool('scry_quiz',{operation_id:id('quiz'),prompt_id:shown.prompt.id})).structuredContent.practice;
  n.jev.respond=()=>unsure();
  const said={operation_id:id('answer'),prompt_id:quiz.prompt.id,answer:'lose sight of them'};
  let told=await tool('scry_answer',said);
  assert.equal(told.isError,false);assert.equal(told.structuredContent.practice.judgment.state,'checking');
  const third=told.structuredContent.result.entity_id;
  await n.settle(n.closed(third),'the tool server check');
  for(const [name,invalid] of [['scry_answer',{operation_id:id('bad'),prompt_id:quiz.prompt.id,answer:'x',attempt_id:third}],['scry_answer',{operation_id:id('bad'),attempt_id:third}],['scry_check',{operation_id:id('bad')}],['scry_check',{operation_id:id('bad'),attempt_id:third,verdict:'Good'}],['scry_check',{operation_id:id('bad'),attempt_id:third,answer:'x'}]]){
    const result=await tool(name,invalid);assert.equal(result.isError,true);assert.equal(result.structuredContent.error.code,'invalid_arguments');
  }
  n.jev.respond=()=>reject();
  told=await tool('scry_check',{operation_id:id('check'),attempt_id:third});
  assert.equal(told.isError,false);assert.equal(told.structuredContent.practice.judgment.state,'checking');assert.equal(told.structuredContent.practice.attempt.id,third);
  state=await n.settle(n.closed(third),'the repeated tool server check');
  told=await tool('scry_answer',said);
  assert.equal(told.structuredContent.replayed,true);assert.equal(told.structuredContent.practice.judgment.state,'decided');assert.equal(told.structuredContent.practice.judgment.verdict,'Again');
  assert.deepEqual(n.checksOf(state,third).map(check=>[check.status,check.acting_token]),[['unsure',paidId],['rejected',paidId]]);
  assert.equal(n.jev.sent.length,6);assert.equal(Object.keys(state.attempts).length,3);
});

// A notebook that never used the switch, and one that did: their stored rows
// and complete archives, for the round trip and for the previous Worker.
let captured;
const capturedNotebooks=()=>captured??=(async()=>{
  const take=async n=>{const {bytes,archive}=await n.export();return {bytes,archive,rows:await n.rows()};};
  let n=await notebook(),plain,judged;
  try{
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    await n.answer('test the bilge pump');
    plain=await take(n);assert.equal(n.jev.sent.length,0);
  }finally{await n.close();}
  n=await notebook();
  try{
    await n.turnOn();
    await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
    n.jev.respond=()=>accept();
    const good=await n.answer('test the bilge pump');await n.settle(n.closed(good.attempt),'an accepted answer');
    await n.next();n.jev.respond=()=>reject();
    const again=await n.answer('sound the bell');await n.settle(n.closed(again.attempt),'a rejected answer');
    await n.next();n.jev.respond=()=>unsure();
    const open=await n.answer('lose sight of them');await n.settle(n.closed(open.attempt),'an answer left not graded');
    judged=await take(n);
  }finally{await n.close();}
  return {plain,judged};
})();
const restoreToken='synthetic-judgment-restore-only-32-bytes';
const restoreInto=async(build,bytes,inspect)=>{
  const name='judgment-restore-'+randomUUID();
  const runtime=await createRuntime({name,artifact:build,inspect:true,bindings:{SCRY_ENV:'restore',CANONICAL_ORIGIN:origin,SPACE_NAME:name,INTERNAL_KEY:internalKey,RESTORE_TOKEN:restoreToken}});
  try{
    const response=await runtime.worker.fetch(`${origin}/operator/restore`,{method:'POST',headers:{'x-scry-restore-token':restoreToken,'x-scry-sha256':sha(bytes)},body:bytes});
    const storage=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
    const rows=(await storage.exec("SELECT name FROM sqlite_master WHERE type='table' AND name='records'")).length?await storage.exec('SELECT key,value FROM records ORDER BY key'):[];
    return inspect({status:response.status,text:await response.text(),rows});
  }finally{await runtime.mf.dispose();}
};

test('a notebook that used the switch is archived as judged-v4 and restores whole',{timeout:120000},async()=>{
  const {plain,judged}=await capturedNotebooks();
  assert.equal(judged.archive.format,JUDGED);assert.notEqual(plain.archive.format,JUDGED);
  assert.ok(judged.rows.some(row=>row.key===MARKER)&&judged.rows.some(row=>row.key.startsWith('practice_checks/')));
  assert.ok(!plain.rows.some(row=>row.key===MARKER||row.key.startsWith('practice_checks/')));
  const app=judged.archive.app;
  assert.equal(app.preferences.paid_judgment,true);assert.equal(Object.keys(app.practice.checks).length,3);
  assert.deepEqual(Object.values(app.practice.attempts).sort((a,b)=>a.sequence-b.sequence).map(a=>a.judgment.decided?.verdict??null),['Good','Again',null]);
  assert.deepEqual(app.spend.map(s=>s.purpose),['grade.recall','grade.recall','grade.recall']);
  const kept=row=>row.key!=='meta';
  await restoreInto(artifact,judged.bytes,({status,text,rows})=>{
    assert.equal(status,200,text);
    assert.deepEqual(rows.filter(kept),judged.rows.filter(kept),'Every Attempt, Check, Card, ledger entry and receipt is restored as it was stored.');
    const meta=JSON.parse(rows.find(row=>row.key==='meta').value),source=JSON.parse(judged.rows.find(row=>row.key==='meta').value);
    assert.deepEqual(meta.practice,source.practice);assert.equal(meta.preferences.paid_judgment,true);assert.equal(meta.restored_paused,true);
  });
  // The same content under an older format name is not a notebook Scry accepts.
  const forged=Buffer.from(JSON.stringify({...judged.archive,format:plain.archive.format}));
  await restoreInto(artifact,forged,({status,rows})=>{assert.equal(status,422);assert.deepEqual(rows,[]);});
});

test('the previous Worker refuses a notebook that used the switch, and still reads one that did not',{timeout:120000,skip:previous?false:'Set SCRY_JUDGMENT_OLD_WORKER to the build of the source before paid judgment.'},async()=>{
  const {plain,judged}=await capturedNotebooks();
  // Stored rows, read in place.
  const name='judgment-previous-'+randomUUID();
  const bindings={SCRY_ENV:'development',CANONICAL_ORIGIN:'http://127.0.0.1:0',SPACE_NAME:name,INTERNAL_KEY:internalKey};
  const runtime=await createRuntime({port:0,name,artifact:previous,bindings,inspect:true});
  try{
    const local=(await runtime.mf.ready).origin,active=await runtime.rebind({...bindings,CANONICAL_ORIGIN:local});
    const store=await runtime.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name});
    const read=()=>active.worker.fetch(`${local}/recall`,{headers:{'X-Scry-Test-Host':new URL(local).host}});
    const load=async rows=>{await store.exec('CREATE TABLE IF NOT EXISTS records (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID');await store.exec('DELETE FROM records');for(const row of rows)await store.exec('INSERT INTO records(key,value) VALUES(?,?)',row.key,row.value);};
    await load(plain.rows);
    const control=await read();assert.equal(control.status,200,'The previous Worker reads a notebook this Worker wrote with the switch off.');
    assert.match(await control.text(),/data-state="exact-result"/);
    await load(judged.rows);
    const before=await store.exec('SELECT key,value FROM records ORDER BY key');
    const response=await read();assert.ok(response.status>=500,`The previous Worker must refuse a judged notebook: ${response.status}`);
    assert.deepEqual(await store.exec('SELECT key,value FROM records ORDER BY key'),before,'A refused notebook is left exactly as it was.');
  }finally{await runtime.mf.dispose();}
  // Complete archives, restored.
  await restoreInto(previous,plain.bytes,({status,text,rows})=>{assert.equal(status,200,text);assert.ok(rows.length>0);});
  await restoreInto(previous,judged.bytes,({status,rows})=>{assert.equal(status,422);assert.deepEqual(rows,[],'Nothing of a refused archive is written.');});
});

test('phone width, with and without JavaScript: the switch, checking, Good, Again, and saved-not-graded with Check again',{timeout:240000},async()=>{
  const require=createRequire(import.meta.url),playwright=require(process.env.SCRY_CRITICS_PLAYWRIGHT_PATH||'playwright');
  const browser=await playwright.chromium.launch({headless:true,...(process.env.SCRY_CRITICS_CHROMIUM_PATH?{executablePath:process.env.SCRY_CRITICS_CHROMIUM_PATH}:{}),args:['--no-sandbox']});
  await mkdir(shots,{recursive:true});
  try{
    for(const js of [true,false]){
      const tag=js?'js':'nojs',n=await notebook();
      const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,deviceScaleFactor:2,javaScriptEnabled:js});
      try{
        const url=await n.listen(),page=await context.newPage(),errors=[];
        page.on('pageerror',error=>errors.push(String(error)));
        const capture=async name=>{
          assert.equal(await page.evaluate(()=>{window.scrollTo(0,0);return document.documentElement.scrollWidth-document.documentElement.clientWidth;}),0,`${name}: no sideways scroll at phone width`);
          await page.screenshot({path:join(shots,`${name}-${tag}.png`),fullPage:true});
        };
        // The switch, through its native form.
        await page.goto(url+'/settings');
        await page.locator('[data-paid-judgment="off"]').waitFor();
        await capture('settings-off');
        await page.getByRole('button',{name:'Turn paid judgment on'}).click();
        await page.locator('[data-paid-judgment="on"]').waitFor();
        assert.equal((await n.state()).meta.preferences.paid_judgment,true);
        await capture('settings-on');
        await n.study(await n.addMapping('Synthetic harbour practice',HARBOUR));
        const answer=async text=>{
          await page.goto(url+'/recall');
          const memory=page.getByRole('button',{name:'Try from memory'});
          if(await memory.count()){await memory.click();await page.locator('#answer').waitFor();}
          await page.locator('#answer').fill(text);
          await page.locator('form[action="/practice/answer"] button.primary').click();
          await page.locator('[data-state="exact-checking"],[data-state="exact-ungraded"],[data-state="exact-result"]').first().waitFor();
          return n.newest(await n.state()).id;
        };
        // With JavaScript the checking page looks again by itself; without, Refresh does.
        const outcome=async(attempt,selector)=>{
          await n.settle(n.closed(attempt),'the browser check');
          if(!js&&await page.locator('[data-state="exact-checking"]').count())await page.getByRole('link',{name:'Refresh'}).click();
          await page.locator(selector).waitFor({timeout:20000});
        };
        // Checking, then Good by Jev with the comparison kept as a comparison.
        let gate=held(n.jev,()=>accept());
        let attempt=await answer('test the bilge pump');
        await page.locator('[data-state="exact-checking"]').waitFor();
        assert.match(await page.locator('#main').innerText(),/YOUR ANSWER IS SAVED[\s\S]*Checking it\.[\s\S]*test the bilge pump/i);
        assert.equal(await page.locator('[data-verdict]').count(),0);assert.equal(await page.getByRole('link',{name:'Refresh'}).count(),1);
        await gate.sending;await capture('checking');
        gate.release();
        await outcome(attempt,'[data-verdict="Good"]');
        let text=await page.locator('#main').innerText();
        assert.match(text,/Good\./);assert.match(text,/Judged by Jev \(recall-v1; synthetic-jev-large\)/);assert.match(text,/Jev accepted this answer as the same recall/);
        await page.locator('details.exact-full-answer > summary').click();
        assert.match(await page.locator('details.exact-full-answer').innerText(),/Word-by-word comparison, not the Verdict/);
        await capture('decided-good');
        // Again by Jev.
        await page.getByRole('button',{name:'Next',exact:true}).click();
        await page.locator('[data-state="exact-prompt"]').waitFor();
        n.jev.respond=()=>reject();
        attempt=await answer('sound the bell');
        await outcome(attempt,'[data-verdict="Again"]');
        assert.match(await page.locator('#main').innerText(),/Again\.[\s\S]*Judged by Jev/);
        await capture('decided-again');
        // Saved, not graded, with its reason and Check again.
        await page.getByRole('button',{name:'Next',exact:true}).click();
        await page.locator('[data-state="exact-prompt"]').waitFor();
        n.jev.respond=()=>unsure();
        attempt=await answer('lose sight of them');
        await outcome(attempt,'[data-state="exact-ungraded"]');
        text=await page.locator('#main').innerText();
        assert.match(text,/Saved, not graded\./);assert.match(text,/Jev could not decide this answer/);assert.match(text,/It reserves \$0\.50 of today.s allowance/);
        assert.equal(await page.locator('[data-verdict]').count(),0);
        await capture('ungraded');
        n.jev.respond=()=>reject();
        await page.getByRole('button',{name:'Check again'}).click();
        await page.locator('[data-state="exact-checking"],[data-verdict="Again"]').first().waitFor();
        await n.settle(state=>n.checksOf(state,attempt).length===2&&n.closed(attempt)(state),'the check asked for in the browser');
        await outcome(attempt,'[data-verdict="Again"]');
        let state=await n.state();
        assert.deepEqual(n.checksOf(state,attempt).map(check=>check.status),['unsure','rejected']);
        assert.equal(Object.keys(state.attempts).length,3);assert.equal(n.jev.sent.length,4);
        // A text lesson: its result sits inside the lesson's own page state, and
        // is checked, refreshed and decided in the same way.
        const added=await n.post('/library/version/add',{...await n.fresh(),title:'Synthetic harbour verse',text:'The tide runs out at dawn,\nthe gulls wheel over the bar.\n'});
        assert.equal(added.status,303,await added.text());
        const verse=/\/library\/versions\/([0-9a-f]{32})/.exec(added.headers.get('Location'))[1];
        const started=await n.post('/practice/lesson/start',{...await n.fresh(),version_id:verse,after:state.meta.practice.current.prompt.id});
        assert.equal(started.status,303,await started.text());
        gate=held(n.jev,()=>reject());
        attempt=await answer('The tide goes out at dawn');
        await page.locator('.lesson-stage[data-state="exact-checking"][data-poll]').waitFor();
        assert.equal(await page.locator('[data-state]').first().getAttribute('data-state'),'lesson-home');
        await gate.sending;await capture('lesson-checking');
        gate.release();
        await outcome(attempt,'.lesson-stage [data-verdict="Again"]');
        assert.match(await page.locator('#main').innerText(),/Judged by Jev/);
        await capture('lesson-decided-again');
        state=await n.state();
        assert.equal(Object.keys(state.attempts).length,4);assert.equal(n.jev.sent.length,5);
        assert.deepEqual(errors,[],'No script error on any page.');
      }finally{await context.close();await n.close();}
    }
    // Where Jev is not reachable: said plainly, and no control.
    const n=await notebook({capable:false});
    const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,deviceScaleFactor:2});
    try{
      const page=await context.newPage();
      await page.goto(await n.listen()+'/settings');
      await page.locator('[data-paid-judgment="unavailable"]').waitFor();
      assert.match(await page.locator('#paid-judgment').innerText(),/Paid judgment is not available here\./);
      assert.equal(await page.getByRole('button',{name:/paid judgment/i}).count(),0);
      assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth-document.documentElement.clientWidth),0);
      await page.screenshot({path:join(shots,'settings-unavailable.png'),fullPage:true});
    }finally{await context.close();await n.close();}
  }finally{await browser.close();}
});
