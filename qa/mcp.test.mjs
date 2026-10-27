// Real stdio subprocess + isolated loopback HTTP. No hosted credentials/egress.
import assert from 'node:assert/strict';
import test from 'node:test';
import {spawn, spawnSync} from 'node:child_process';
import {createServer} from 'node:http';
import {mkdtemp, writeFile, chmod, symlink, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {dirname, join, resolve} from 'node:path';
import {createInterface} from 'node:readline';
import {once} from 'node:events';

const binary = process.env.SCRY_MCP_BIN || resolve('target/debug/scry-mcp');
const cliBinary = join(dirname(binary),'scry');
if (!process.env.SCRY_MCP_BIN) {
  const build = spawnSync('cargo', ['build', '--locked', '--bin', 'scry', '--bin', 'scry-mcp'], {encoding:'utf8'});
  assert.equal(build.status, 0, build.stderr);
}
const secret = `scry_ot_${'1'.repeat(32)}_${'2'.repeat(48)}`;
const privateAnswer = 'SENSITIVE learner answer, never a protocol diagnostic';
const entity = 'a'.repeat(32), prompt = 'b'.repeat(32);
const version = {title:'Synthetic stanza',kind:'text',text:'One small synthetic stanza.'};
const mapping = {title:'Synthetic mapping',direction:'forward',pairs:[{key:'A',value:'Alfa'},{key:'B',value:'Bravo'}]};
const operation = name => `mcp-proof-${name}-operation`;
const mutation = (extra={}) => ({result:{redirect:'/recall',entity_id:entity,message:null},replayed:false,practice:{},...extra});

async function fixture(t, {configExists = true} = {}) {
  const directory = await mkdtemp(join(tmpdir(),'scry-mcp-'));
  const config = join(directory,'login.json');
  const seen = [];
  let handler = async (_request, response) => response.end(JSON.stringify({ok:true}));
  const http = createServer(async (request,response) => {
    const chunks = [];
    for await (const chunk of request) chunks.push(chunk);
    const body = Buffer.concat(chunks).toString();
    assert.ok(request.headers.authorization === `Bearer ${secret}`, 'Uses only the configured Owner bearer.');
    seen.push({method:request.method,path:request.url,payload:body ? JSON.parse(body) : undefined});
    response.setHeader('Content-Type','application/json');
    await handler(request,response,seen.at(-1));
  });
  http.listen(0,'127.0.0.1'); await once(http,'listening');
  const origin = `http://127.0.0.1:${http.address().port}`;
  if(configExists) await writeFile(config,JSON.stringify({origin,token:secret}),{mode:0o600});
  const child = spawn(binary,['--config',config],{env:{PATH:process.env.PATH,HOME:directory},stdio:['pipe','pipe','pipe']});
  let stderr = '', serial = 0;
  const waiting = new Map(), unsolicited = [], rawWaiters = [];
  child.stderr.on('data', data => stderr += data);
  const lines = createInterface({input:child.stdout});
  lines.on('line', line => {
    assert.ok(!line.includes(secret),'No credential in stdio output.');
    let message;
    try { message = JSON.parse(line); } catch { assert.fail('Every stdout line must be one JSON-RPC object.'); }
    assert.equal(message.jsonrpc,'2.0');
    if(waiting.has(message.id)) { waiting.get(message.id)(message); waiting.delete(message.id); }
    else if(rawWaiters.length) rawWaiters.shift()(message);
    else unsolicited.push(message);
  });
  const exit = once(child,'exit');
  const send = message => child.stdin.write(JSON.stringify(message)+'\n');
  const request = (method,params={},id=++serial) => new Promise(resolve => {
    waiting.set(id,resolve); send({jsonrpc:'2.0',id,method,params});
  });
  const call = (name,args={}) => request('tools/call',{name,arguments:args});
  const raw = bytes => {
    const response = unsolicited.length ? Promise.resolve(unsolicited.shift()) : new Promise(resolve => rawWaiters.push(resolve));
    child.stdin.write(bytes); return response;
  };
  const initialize = async (version='2025-11-25') => {
    const response = await request('initialize',{protocolVersion:version,capabilities:{},clientInfo:{name:'scry-wire-proof',version:'1'}});
    send({jsonrpc:'2.0',method:'notifications/initialized'});
    return response;
  };
  t.after(async () => {
    child.stdin.end();
    const ended = await Promise.race([exit,new Promise(resolve => setTimeout(()=>resolve(null),2000).unref())]);
    if(!ended) child.kill('SIGKILL');
    http.closeAllConnections(); await new Promise(resolve => http.close(resolve));
    await rm(directory,{recursive:true,force:true});
    assert.ok(ended,'MCP exits after stdin EOF.');
    assert.equal(ended?.[0],0);
    assert.equal(stderr,'','No runtime logging, payloads or credentials on stderr.');
    assert.equal(waiting.size,0,'Every request completed.');
  });
  return {config, directory, origin, child, seen, request, call, raw, send, initialize, unsolicited, setHandler:fn=>handler=fn};
}
const data = response => {
  assert.equal(response.error,undefined);
  assert.equal(response.result.isError,false,JSON.stringify(response.result.structuredContent));
  assert.deepEqual(JSON.parse(response.result.content[0].text),response.result.structuredContent);
  return response.result.structuredContent;
};
const error = response => {
  assert.equal(response.result.isError,true);
  const body = response.result.structuredContent;
  assert.ok(!JSON.stringify(body).includes(privateAnswer),'Never reflect submitted answer in errors.');
  return body.error;
};

test('MCP lifecycle, version negotiation, discovery, IDs and notifications', {timeout:15000}, async t => {
  const f = await fixture(t);
  assert.equal((await f.request('tools/list')).error.code,-32000);
  assert.equal((await f.request('initialize',{protocolVersion:'wrong'})).error.code,-32602);
  const initialized = await f.request('initialize',{protocolVersion:'older-client',capabilities:{},clientInfo:{name:'wire',version:'1'}});
  assert.equal(initialized.result.protocolVersion,'2025-11-25');
  assert.deepEqual(initialized.result.capabilities,{tools:{}});
  assert.equal((await f.request('tools/list')).error.code,-32000);
  f.send({jsonrpc:'2.0',method:'notifications/initialized'});
  const list = await f.request('tools/list');
  assert.equal(list.result.tools.length,11);
  assert.equal(new Set(list.result.tools.map(tool=>tool.name)).size,11);
  for(const tool of list.result.tools) {
    assert.equal(tool.inputSchema.type,'object');
    assert.equal(tool.inputSchema.additionalProperties,false);
    assert.ok(!/correct|generate|token|grade/.test(tool.name));
  }
  assert.equal((await f.request('tools/list',{cursor:'unsupported'})).error.code,-32602);
  assert.equal((await f.call('scry_correct',{verdict:'Good'})).error.code,-32602);
  assert.equal((await f.request('tools/call',{name:'scry_due',arguments:{},task:{ttl:1000}})).error.code,-32602);
  assert.equal((await f.request('resources/list')).error.code,-32601);
  assert.deepEqual((await f.request('ping',{},'string-request')).result,{});
  assert.equal((await f.request('ping',{},'string-request')).error.code,-32600);
  for(const notification of ['notifications/cancelled','unknown/notification']) f.send({jsonrpc:'2.0',method:notification,params:{requestId:'old',reason:privateAnswer}});
  await f.request('ping');
  assert.deepEqual(f.unsolicited,[],'Notifications receive no response.');
  assert.equal(f.seen.length,0,'Discovery does not contact API or require a token.');
});

test('Framing: escaped newlines, malformed/oversized JSON, invalid UTF-8, batch and invalid IDs', {timeout:15000}, async t => {
  const f = await fixture(t); await f.initialize();
  const invalid = ['{bad json}\n','[]\n',Buffer.from([0xff,10]),JSON.stringify({jsonrpc:'2.0',id:null,method:'ping'})+'\n',JSON.stringify({jsonrpc:'2.0',id:1.5,method:'ping'})+'\n'];
  for(const frame of invalid) assert.ok((await f.raw(frame)).error);
  const huge = 'x'.repeat(4*1024*1024+1)+'\n';
  assert.equal((await f.raw(huge)).error.code,-32700);
  assert.deepEqual((await f.request('ping')).result,{},'Valid frame after oversized one still works.');
  data(await f.call('scry_validate',{format:'version',data:{...version,text:'First line.\nSecond line.'}}));
  assert.equal(f.seen.length,0);
});

test('An unterminated JSON frame at EOF is rejected, not executed', {timeout:15000}, async t => {
  const f = await fixture(t); await f.initialize();
  const response=f.raw(JSON.stringify({jsonrpc:'2.0',id:'truncated',method:'tools/call',params:{name:'scry_due',arguments:{}}}));
  f.child.stdin.end();
  assert.equal((await response).error.code,-32700);
  assert.equal(f.seen.length,0);
});

test('Auth-free schema and native validation, not remote acceptance; strict authored input', {timeout:15000}, async t => {
  const f = await fixture(t,{configExists:false}); await f.initialize();
  const capabilities = data(await f.call('scry_capabilities'));
  assert.equal(capabilities.grading,'native_only');
  assert.equal(capabilities.generation,false); assert.equal(capabilities.self_grading,false);
  assert.equal(capabilities.multimedia,false); assert.equal(capabilities.hosted_admission,'unverified');
  const schemas = data(await f.call('scry_schema'));
  assert.deepEqual(schemas.mapping.properties.direction.enum,['forward']);
  for(const [format,draft] of [['version',version],['mapping',mapping]]) {
    const checked = data(await f.call('scry_validate',{format,data:draft}));
    assert.equal(checked.valid,true); assert.equal(checked.scope,'local_structure_only');
    assert.equal(checked.remote_admission_checked,false);
  }
  for(const draft of [
    {...version,title:''}, {...version,grade:'Good'},
    {...version,sections:[{heading:'Mismatch',units:['not the supplied text']}]},
    {...version,text:'😀'.repeat(65537)},
  ]) assert.equal(error(await f.call('scry_validate',{format:'version',data:draft})).code,'invalid_arguments');
  for(const draft of [
    {...mapping,direction:'reverse'}, {...mapping,pairs:[{key:'A',value:'Alfa'},{key:'a',value:'another'}]},
    {...mapping,pairs:[{key:'A',value:'Alfa\nBravo'}]}, {...mapping,media_url:'https://untrusted.invalid'},
  ]) assert.equal(error(await f.call('scry_validate',{format:'mapping',data:draft})).code,'invalid_arguments');
  assert.equal(error(await f.call('scry_due')).code,'login_required');
  assert.equal(f.seen.length,0);
});

test('Each tool forwards only its exact existing Owner API route and frozen payload', {timeout:15000}, async t => {
  const f = await fixture(t); await f.initialize();
  f.setHandler(async (_request,response,record)=>response.end(JSON.stringify(record.payload ? mutation({received:record.payload}) : {received:{path:record.path}})));
  data(await f.call('scry_search',{query:'A & B?\nUnicode Ω'}));
  assert.equal(new URL(f.seen.at(-1).path,f.origin).searchParams.get('q'),'A & B?\nUnicode Ω');
  for(const [name,args,path,payload] of [
    ['scry_show',{kind:'work',id:entity},`/api/v1/library/works/${entity}`,undefined],
    ['scry_show',{kind:'version',id:entity},`/api/v1/library/versions/${entity}`,undefined],
    ['scry_import',{operation_id:operation('import'),format:'version',data:version},'/api/v1/library/versions',{operation_id:operation('import'),data:version}],
    ['scry_import',{operation_id:operation('mapping'),format:'mapping',data:mapping},'/api/v1/library/mappings',{operation_id:operation('mapping'),data:mapping}],
    ['scry_learn',{operation_id:operation('learn'),version_id:entity,pace:'light'},'/api/v1/learn',{operation_id:operation('learn'),version_id:entity,pace:'light'}],
    ['scry_due',{},'/api/v1/due',undefined],
    ['scry_next',{operation_id:operation('next'),after:prompt,study_id:entity},'/api/v1/next',{operation_id:operation('next'),after:prompt,study_id:entity}],
    ['scry_answer',{operation_id:operation('answer'),prompt_id:prompt,answer:'Raw\nanswer Ω'},'/api/v1/answer',{operation_id:operation('answer'),prompt_id:prompt,answer:'Raw\nanswer Ω'}],
    ['scry_reveal',{operation_id:operation('reveal'),prompt_id:prompt},'/api/v1/reveal',{operation_id:operation('reveal'),prompt_id:prompt}],
  ]) {
    data(await f.call(name,args));
    assert.deepEqual(f.seen.at(-1),{path,method:payload?'POST':'GET',payload});
  }
  const before = f.seen.length;
  for(const [name,args] of [
    ['scry_answer',{operation_id:'short',prompt_id:prompt,answer:privateAnswer}],
    ['scry_answer',{operation_id:operation('answer'),prompt_id:prompt,answer:privateAnswer,verdict:'Good'}],
    ['scry_answer',{operation_id:operation('answer'),prompt_id:prompt,answer:privateAnswer,rating:4}],
    ['scry_next',{}],['scry_next',{operation_id:operation('next'),recite:'section'}],
    ['scry_show',{kind:'version',id:'../../tokens'}],
    ['scry_import',{operation_id:operation('invalid'),format:'version',data:{...version,text:''}}],
  ]) assert.equal(error(await f.call(name,args)).code,'invalid_arguments');
  assert.equal(f.seen.length,before,'Invalid input is refused before transmission.');
});

test('Lost answer: no hidden retry, same-ID replay before Next, changed payload blocked', {timeout:15000}, async t => {
  const f = await fixture(t); await f.initialize();
  const answer = {operation_id:operation('lost'),prompt_id:prompt,answer:privateAnswer};
  let first = true, changes = 0;
  f.setHandler(async (request,response,record) => {
    if(record.path === '/api/v1/answer') {
      assert.deepEqual(record.payload,answer);
      if(first) { first=false; changes++; request.socket.destroy(); return; }
      response.end(JSON.stringify(mutation({replayed:true,practice:{attempt:{id:entity}}})));
    } else response.end(JSON.stringify(record.payload ? mutation() : {ok:true}));
  });
  const lost = error(await f.call('scry_answer',answer));
  assert.equal(lost.code,'transport_error'); assert.equal(lost.outcome_unknown,true);
  assert.equal(f.seen.length,1,'Exactly one HTTP transmission, even on response loss.');
  assert.equal(error(await f.call('scry_next',{operation_id:operation('next')})).code,'reconciliation_required');
  assert.equal(error(await f.call('scry_answer',{...answer,answer:'changed'})).code,'reconciliation_required');
  assert.equal(error(await f.call('scry_answer',{...answer,operation_id:operation('replacement')})).code,'reconciliation_required');
  data(await f.call('scry_due'));
  await chmod(f.config,0o644);
  assert.equal(error(await f.call('scry_answer',answer)).outcome_unknown,true,'Login refusal cannot settle the earlier unknown write.');
  await chmod(f.config,0o600);
  await writeFile(f.config,JSON.stringify({origin:'http://127.0.0.1:1',token:secret}));
  const moved=error(await f.call('scry_answer',answer));
  assert.equal(moved.code,'login_changed'); assert.equal(moved.outcome_unknown,true);
  await writeFile(f.config,JSON.stringify({origin:f.origin,token:`scry_ot_${'3'.repeat(32)}_${'4'.repeat(48)}`}));
  const rotated=error(await f.call('scry_answer',answer));
  assert.equal(rotated.code,'login_changed'); assert.equal(rotated.outcome_unknown,true,'A different token may be a different principal even on the same origin.');
  assert.equal(error(await f.call('scry_due')).code,'login_changed');
  await writeFile(f.config,JSON.stringify({origin:f.origin,token:secret}));
  assert.equal(data(await f.call('scry_answer',answer)).replayed,true);
  data(await f.call('scry_next',{operation_id:operation('next')}));
  assert.equal(changes,1,'Replay did not create a second mock mutation.');
  assert.equal(f.seen.filter(r=>r.path==='/api/v1/answer').length,2);
});

test('Fixed structured failures, assistance-fence conflict and no redirect forwarding', {timeout:15000}, async t => {
  const f = await fixture(t); await f.initialize();
  let status = 401;
  f.setHandler(async (_request,response) => {
    response.statusCode=status;
    response.setHeader('Location',f.origin+'/do-not-follow');
    response.end(JSON.stringify({error:`${secret} ${privateAnswer}`}));
  });
  for(const [code,expected] of [[401,'unauthorized'],[403,'forbidden'],[409,'conflict'],[422,'validation_failed'],[302,'redirect_refused'],[500,'api_error']]) {
    status=code;
    const received = error(await f.call('scry_due'));
    assert.equal(received.code,expected); assert.equal(received.status,code);
    if(code===409) assert.match(received.recovery,/unaided Prompt/);
  }
  assert.ok(f.seen.every(record=>record.path==='/api/v1/due'),'Redirects never forward the credential.');
  f.setHandler(async (_request,response)=>response.end(`${secret} ${privateAnswer}`));
  assert.equal(error(await f.call('scry_due')).code,'invalid_response');
  f.setHandler(async (_request,response)=>response.end('[]'));
  assert.equal(error(await f.call('scry_due')).code,'invalid_response');
  f.setHandler(async (_request,response)=>response.end(JSON.stringify({error:privateAnswer})));
  assert.equal(error(await f.call('scry_due')).code,'invalid_response');
  f.setHandler(async (_request,response)=>response.end(JSON.stringify({ok:true,diagnostic:secret})));
  assert.equal(error(await f.call('scry_due')).code,'invalid_response');
  f.setHandler(async (_request,response)=>response.end('{"echo":"'+secret.replace(/^s/,'\\u0073')+'"}'));
  assert.equal(error(await f.call('scry_due')).code,'invalid_response','JSON escapes cannot bypass credential reflection refusal.');
});

test('Proxy errors and malformed 2xx objects never settle an unknown mutation', {timeout:15000}, async t => {
  const f = await fixture(t); await f.initialize();
  const answer={operation_id:operation('proxy'),prompt_id:prompt,answer:privateAnswer};
  f.setHandler(async (_request,response)=>{response.statusCode=503; response.end(privateAnswer);});
  assert.equal(error(await f.call('scry_answer',answer)).outcome_unknown,true);
  f.setHandler(async (_request,response)=>response.end('{}'));
  const malformed=error(await f.call('scry_answer',answer));
  assert.equal(malformed.code,'invalid_response'); assert.equal(malformed.outcome_unknown,true);
  f.setHandler(async (_request,response)=>{response.statusCode=409; response.end(privateAnswer);});
  const conflict=error(await f.call('scry_answer',answer));
  assert.equal(conflict.code,'conflict'); assert.equal(conflict.outcome_unknown,true);
  assert.equal(error(await f.call('scry_next',{operation_id:operation('next')})).code,'reconciliation_required');
  f.setHandler(async (_request,response)=>response.end(JSON.stringify(mutation({replayed:true}))));
  assert.equal(data(await f.call('scry_answer',answer)).replayed,true);
  data(await f.call('scry_next',{operation_id:operation('next')}));
});

test('Credential privacy, symlinks, corrupt origin and oversized login are refused locally', {timeout:15000}, async t => {
  const f = await fixture(t); await f.initialize();
  await chmod(f.config,0o644);
  assert.equal(error(await f.call('scry_due')).code,'login_required');
  await chmod(f.config,0o600);
  await writeFile(f.config,JSON.stringify({origin:'http://not-loopback.invalid',token:secret}));
  assert.equal(error(await f.call('scry_due')).code,'login_required');
  await writeFile(f.config,'x'.repeat(8193));
  assert.equal(error(await f.call('scry_due')).code,'login_required');
  await rm(f.config);
  const target=join(f.directory,'symlink-target.json');
  await writeFile(target,JSON.stringify({origin:f.origin,token:secret}),{mode:0o600});
  await symlink(target,f.config);
  assert.equal(error(await f.call('scry_due')).code,'login_required');
  assert.equal(f.seen.length,0);
});

test('Cancellation of an admitted blocking call cannot undo commit and causes no retry', {timeout:15000}, async t => {
  const f = await fixture(t); await f.initialize();
  let sent;
  const arrived = new Promise(resolve=>sent=resolve);
  f.setHandler(async (_request,response)=>{
    sent(); await new Promise(resolve=>setTimeout(resolve,50));
    response.end(JSON.stringify(mutation({committed:true})));
  });
  const response = f.request('tools/call',{name:'scry_answer',arguments:{operation_id:operation('cancel'),prompt_id:prompt,answer:'Alfa'}},'cancel-me');
  await arrived;
  f.send({jsonrpc:'2.0',method:'notifications/cancelled',params:{requestId:'cancel-me',reason:privateAnswer}});
  assert.equal(data(await response).committed,true,'Unsupported cancellation may complete; never promise rollback.');
  await f.request('ping');
  assert.equal(f.seen.length,1); assert.deepEqual(f.unsolicited,[]);
});

test('Extracted shared client preserves compiled CLI login, errors and one transmission', {timeout:15000}, async t => {
  const f = await fixture(t,{configExists:false}); await f.initialize();
  const cli = async (args,input='') => {
    const child = spawn(cliBinary,['--config',f.config,'--json',...args],{env:{PATH:process.env.PATH,HOME:f.directory},stdio:['pipe','pipe','pipe']});
    let stdout='',stderr='';
    child.stdout.on('data',part=>stdout+=part); child.stderr.on('data',part=>stderr+=part);
    const exit=once(child,'exit'); child.stdin.end(input); const [code]=await exit;
    assert.ok(!stdout.includes(secret) && !stderr.includes(secret),'CLI never reflects the Owner credential.');
    assert.ok(!stdout.includes(privateAnswer) && !stderr.includes(privateAnswer),'CLI errors never reflect the answer.');
    assert.equal(stderr,'');
    return {code,body:JSON.parse(stdout)};
  };
  const owner={owner:true,token:{id:'1'.repeat(32),name:'Synthetic MCP transport proof',created_ms:1,last_used_ms:null,expires_ms:null,paid:false,revoked_ms:null}};
  f.setHandler(async (_request,response)=>response.end(JSON.stringify(owner)));
  assert.deepEqual(await cli(['login','--url',f.origin,'--token-stdin'],secret+'\n'),{code:0,body:owner});
  assert.deepEqual(await cli(['whoami']),{code:0,body:owner});
  for(const status of [401,403,409,422,302]) {
    f.setHandler(async (_request,response)=>{
      response.statusCode=status; response.setHeader('Location',f.origin+'/never-follow');
      response.end(JSON.stringify({error:secret+' '+privateAnswer}));
    });
    const received=await cli(['whoami']); assert.equal(received.code,1);
    assert.equal(received.body.error,({401:'Owner authorization required.',403:'Request refused.',409:'Operation conflict.'})[status] || 'API request failed.');
  }
  const before=f.seen.length;
  f.setHandler(async request=>request.socket.destroy());
  const lost=await cli(['answer',prompt,'--text-stdin','--operation-id',operation('cli-lost')],privateAnswer);
  assert.equal(lost.code,1); assert.match(lost.body.error,/same operation ID/);
  assert.equal(f.seen.length,before+1,'CLI shares disabled HTTP retry policy.');
  assert.ok(f.seen.every(record=>record.path!=='/never-follow'));
});
