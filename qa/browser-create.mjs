// Real native browser journey through the compiled Worker with explicit,
// authored provider responses. This verifies orchestration, never AI quality.
import assert from 'node:assert/strict';
import {generateKeyPairSync,sign,createHash} from 'node:crypto';
import {createServer} from 'node:https';
import {execFileSync} from 'node:child_process';
import {mkdtempSync,readFileSync,rmSync} from 'node:fs';
import {join} from 'node:path';
import {tmpdir} from 'node:os';
import {createRuntime,Response,internalKey} from './runtime-helper.mjs';
export async function createWalk(browser,capture){
 const team='synthetic-browser.cloudflareaccess.com';
 const temporary=mkdtempSync(join(tmpdir(),'scry-browser-https-'));
 execFileSync('openssl',['req','-x509','-newkey','rsa:2048','-nodes','-sha256','-days','1','-subj','/CN=127.0.0.1','-keyout',join(temporary,'key.pem'),'-out',join(temporary,'cert.pem')],{stdio:'ignore'});
 let runtime;const transport=[];
 const server=createServer({key:readFileSync(join(temporary,'key.pem')),cert:readFileSync(join(temporary,'cert.pem'))},async(req,res)=>{
  try{const headers={...req.headers,'Cf-Access-Jwt-Assertion':jwt,'X-Scry-Test-Host':req.headers.host};if(headers.origin){headers['X-Scry-Test-Origin']=headers.origin;delete headers.origin;}const chunks=[];let size=0;for await(const chunk of req){size+=chunk.length;assert.ok(size<=5*1024*1024);chunks.push(chunk);}const response=await runtime.worker.fetch(origin+req.url,{method:req.method,headers,...(chunks.length?{body:Buffer.concat(chunks)}:{}),redirect:'manual'});const received=Buffer.from(await response.arrayBuffer());transport.push({method:req.method,path:req.url,status:response.status,origin:headers['X-Scry-Test-Origin']||null});if(response.status>=400)console.error('Browser Create transport:',req.method,req.url,response.status,received.toString().slice(0,500));res.writeHead(response.status,Object.fromEntries(response.headers));res.end(received);}
  catch(error){console.error('Browser Create adapter:',error.message);res.writeHead(503);res.end('Synthetic browser adapter unavailable');}
 });
 await new Promise((done,reject)=>{server.once('error',reject);server.listen(0,'127.0.0.1',done);});
 const origin='https://127.0.0.1:'+server.address().port;
 const {publicKey,privateKey}=generateKeyPairSync('rsa',{modulusLength:2048});
 const b64=value=>Buffer.from(JSON.stringify(value)).toString('base64url');const at=Math.floor(Date.now()/1000);
 const encoded=`${b64({alg:'RS256',kid:'browser-synthetic-only'})}.${b64({iss:`https://${team}`,aud:['browser-synthetic'],sub:'browser-owner-synthetic',iat:at,nbf:at-1,exp:at+600})}`;
 const jwt=`${encoded}.${sign('RSA-SHA256',Buffer.from(encoded),privateKey).toString('base64url')}`;
 let preparations=0,critics=0,checks=0;const requests=[];
 let releasePreparation;const firstPreparation=new Promise(done=>{releasePreparation=done;});
 let releaseRefinement;const refinementPreparation=new Promise(done=>{releaseRefinement=done;});
 const provider=async request=>{
  if(request.url===`https://${team}/cdn-cgi/access/certs`)return new Response(JSON.stringify({keys:[{...publicKey.export({format:'jwk'}),kid:'browser-synthetic-only',alg:'RS256',use:'sig'}]}),{headers:{'Content-Type':'application/json'}});
  const body=await request.json();requests.push(body);
  if(request.url==='https://openrouter.ai/api/v1/chat/completions'){
   preparations++;const input=JSON.parse(body.messages[1].content);assert.equal(input.intent,'Understand HTTP cache validation with a practical browser example.');
   if(preparations===1)await firstPreparation;
   if(preparations===2)await refinementPreparation;
   const refined=input.action==='refine';if(refined){assert.ok(input.observed_attempts.length);assert.match(JSON.stringify(input.feedback),/changed image/);}
   const batch={title:'Understand cache validation',complete:true,concepts:[{key:input.existing_ideas[0]?.key||'cache-validator',name:'Representation validators',summary:'A validator identifies a saved representation.',note:refined?'A changed image receives a new entity tag. If the browser sends the old validator, the server sends the new image body instead of confirming the old one.':'An entity tag identifies a representation.\nA browser uses its saved tag in a conditional request.\nA matching tag allows the server to return a short unchanged response instead of another full body.',basis:'general',quotes:[],prerequisites:[],questions:[{prompt:refined?'Which identifier lets a server compare the browser’s saved representation with its current one?':'Which response header carries the representation identifier used in conditional validation?',kind:'recall',answer:'ETag',variants:[],choices:[],explanation:'The ETag response header provides an opaque representation identifier for conditional requests.',basis:'general',quotes:[],required_ideas:[],contradictions:[]}]}]};
   return new Response(JSON.stringify({model:'authored-browser-generator',usage:{cost:0.012},choices:[{message:{content:JSON.stringify(batch)}}]}),{headers:{'Content-Type':'application/json'}});
  }
  assert.equal(request.url,'https://openrouter.ai/api/alpha/decisions');assert.ok(Object.values(body.questions).every(q=>typeof q.instructions==='string'));
  if(body.questions.verdict){checks++;return new Response(JSON.stringify({model:'authored-browser-jev',usage:{cost:0.00002},answers:{verdict:{type:'choice',choice:'accept',probabilities:{accept:0.99,reject:0.005,unsure:0.005}},identity:{type:'noul',noul:0},injection:{type:'noul',noul:0}}}),{headers:{'Content-Type':'application/json'}});}
  critics++;return new Response(JSON.stringify({model:'authored-browser-critic',usage:{cost:0.00003},answers:Object.fromEntries(Object.keys(body.questions).map(key=>[key,{type:'noul',noul:0.01}]))}),{headers:{'Content-Type':'application/json'}});
 };
 const bindings={SCRY_ENV:'preview',CANONICAL_ORIGIN:origin,SPACE_NAME:'browser-provider-isolated',ACCESS_TEAM:team,ACCESS_AUDIENCE:'browser-synthetic',OWNER_SUBJECT:'browser-owner-synthetic',INTERNAL_KEY:internalKey,OPENROUTER_API_KEY:'synthetic-no-live-provider-key',OPENROUTER_MODEL:'authored-browser-generator',JEV_API_KEY:'synthetic-no-live-jev-key',JEV_MODEL:'authored-browser-jev',JEV_URL:'https://openrouter.ai/api/alpha/decisions'};
 let context,page;const result={name:'Restored pause → unavailable Add material → Settings Resume → native ready Create → reference → meaning check → refinement',status:'pass',evidence:[],scope:'Fresh isolated restore and authored provider responses through real model transport; usefulness/calibration not evaluated.'};
 try{
 runtime=await createRuntime({bindings:{...bindings,SPACE_NAME:'browser-before-restore',OPENROUTER_API_KEY:'',OPENROUTER_MODEL:''},name:'scry-browser-create',egress:provider});
 // Retain a real earlier saved request through the existing POST contract.
 // This is explicit historical setup, not a claim that unavailable Create submits.
 const settings=await(await runtime.worker.fetch(origin+'/settings',{headers:{'Cf-Access-Jwt-Assertion':jwt}})).text();
 const savedFields=Object.fromEntries([...settings.matchAll(/name="(csrf|revision)" value="([^"]+)"/g)].map(m=>[m[1],m[2]]));
 const saved=await runtime.worker.fetch(origin+'/create',{method:'POST',redirect:'manual',headers:{'Cf-Access-Jwt-Assertion':jwt,'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({...savedFields,operation_id:'earlier-stopped-browser-request',intent:'Understand an earlier cache question.'})});
 assert.equal(saved.status,303);const savedPath=saved.headers.get('Location');
 const deadline=Date.now()+5000;let stopped=false;
 // Rendered goal reads re-arm the one-second work alarm. Poll the existing
 // export readback instead so setup observes, rather than postpones, that alarm.
 do{
  const response=await runtime.worker.fetch(origin+'/export',{headers:{'Cf-Access-Jwt-Assertion':jwt}});assert.equal(response.status,200);
  const app=(await response.json()).app;const goal=app.goals[savedPath.split('/').at(-1)];const job=app.jobs[goal.job_id];
  stopped=goal.status==='failed'&&job.status==='failed';
  if(stopped){assert.match(job.error,/Preparation is not configured/);assert.deepEqual(app.spend,[]);}
  else await new Promise(done=>setTimeout(done,100));
 }while(!stopped&&Date.now()<deadline);
 assert.ok(stopped,'The earlier saved request must stop without a configured provider');assert.deepEqual({preparations,critics,checks},{preparations:0,critics:0,checks:0});
 const archive=Buffer.from(await(await runtime.worker.fetch(origin+'/export',{headers:{'Cf-Access-Jwt-Assertion':jwt}})).arrayBuffer());
 const restoreToken='browser-synthetic-restore-token-at-least-32chars';
 let active=await runtime.rebind({SCRY_ENV:'restore',CANONICAL_ORIGIN:origin,SPACE_NAME:bindings.SPACE_NAME,INTERNAL_KEY:internalKey,RESTORE_TOKEN:restoreToken});
 const restored=await active.worker.fetch(origin+'/operator/restore',{method:'POST',headers:{'x-scry-restore-token':restoreToken,'x-scry-sha256':createHash('sha256').update(archive).digest('hex')},body:archive});
 assert.equal(restored.status,200);
 active=await runtime.rebind(bindings);runtime.worker=active.worker;
 context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,ignoreHTTPSErrors:true});page=await context.newPage();page.setDefaultTimeout(20000);
 const ns=await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE');const stub=ns.get(ns.idFromName(bindings.SPACE_NAME));
 const readState=async()=>{const r=await stub.fetch(origin+'/export',{headers:{'x-scry-internal-key':internalKey,'x-scry-owner':'private-owner'}});assert.equal(r.status,200);return(await r.json()).app;};
  // Native HTTPS requests/redirects traverse Rust ingress with a synthetic
  // owner assertion. The self-signed loopback certificate is test-only.
  await page.goto(origin+'/settings');await page.getByRole('button',{name:'Resume preparation',exact:true}).waitFor();result.evidence.push(await capture(page,'paused-settings.png'));
  await page.goto(origin+'/create');
  await page.getByText('Preparation is unavailable right now.',{exact:true}).waitFor();
  assert.equal(await page.locator('form[action="/create"],input[type="file"]').count(),0);assert.equal(preparations,0);result.evidence.push(await capture(page,'paused-add-material-unavailable.png'));
  await page.goto(origin+savedPath);await page.getByRole('heading',{name:'Preparation is paused.',exact:true}).waitFor();
  assert.equal(await page.getByRole('button',{name:'Try preparation again'}).count(),0);result.evidence.push(await capture(page,'paused-saved-request.png'));
  await page.goto(origin+'/notebook');await page.locator('[data-state="preparation-paused"]').waitFor();result.evidence.push(await capture(page,'paused-stream.png'));
  await page.locator('.empty-stage').getByRole('link',{name:'Resume preparation'}).tap();await page.getByRole('button',{name:'Resume preparation',exact:true}).tap();
  await page.locator('[data-state="preparation-stopped"]').waitFor();assert.equal((await readState()).restored_paused,false);assert.equal(preparations,0,'Resume must not retry historical stopped work');
  await page.goto(origin+'/create');
  await page.getByRole('button',{name:'Prepare material'}).waitFor();assert.match(await page.locator('#intent-hint').innerText(),/paid AI/);assert.equal(await page.locator('input[type="file"]').count(),1);
  await page.locator('#intent').fill('Understand HTTP cache validation with a practical browser example.');result.evidence.push(await capture(page,'ready-add-material.png'));
  await page.getByRole('button',{name:'Prepare material'}).tap();await page.locator('[data-state="goal"]').waitFor();const goalUrl=page.url();
  await page.locator('.preparation-panel').waitFor();
  const draft='Show how a changed image affects the next conditional request.';
  await page.locator('#feedback').fill(draft);const operation=await page.locator('form[action$="/refine"] input[name="operation_id"]').inputValue();
  let lostPoll=false;
  await context.route('**/goals/*',async route=>{if(!lostPoll&&route.request().headers()['x-scry-request']==='poll'){lostPoll=true;await route.abort('timedout');}else await route.continue();});
  releasePreparation();await page.locator('.reference-idea').waitFor();assert.ok(lostPoll,'The lost progress response was exercised');
  assert.equal(await page.locator('#feedback').inputValue(),draft);assert.equal(await page.locator('form[action$="/refine"] input[name="operation_id"]').inputValue(),operation);assert.equal(await page.locator('#feedback').evaluate(node=>node===document.activeElement),true);
  assert.match(await page.locator('.reference-idea').innerText(),/entity tag/i);assert.ok((await page.locator('.reference-idea .reading p').first().innerText()).includes('\nA browser uses'),'Reference preserves authored line breaks');result.evidence.push(await capture(page,'create-generated-reference.png'));
  await page.getByRole('link',{name:'Go to practice'}).click();await page.locator('[data-state="intro"]').waitFor();await page.getByRole('button',{name:'Start practicing'}).click();await page.locator('#answer').fill('the entity tag header');await page.getByRole('button',{name:'Check my answer'}).click();await page.locator('[data-state="result"]').waitFor();assert.match(await page.locator('.feedback').innerText(),/Checked the meaning/);const before=await readState();assert.equal(before.events.length,1);assert.equal(before.events[0].result.authority,'jev');const original=before.events[0];result.evidence.push(await capture(page,'create-generated-practice.png'));
  await page.goto(goalUrl);await page.locator('#feedback').fill('Show how a changed image affects the next conditional request.');await page.getByRole('button',{name:'Refine my material'}).click();await page.locator('.preparation-panel').waitFor();
  await page.locator('.masthead nav').getByRole('link',{name:'Concepts',exact:true}).tap();await page.locator('[data-state="concept-home"]').waitFor();
  const refinementReceipt=page.locator(`.preparation-receipt[href="${new URL(goalUrl).pathname}"]`);await refinementReceipt.waitFor();
  const historicalReceipt=page.locator(`.preparation-receipt[href="${savedPath}"]`);await historicalReceipt.getByText('Preparation needs your attention',{exact:true}).waitFor();
  const pendingHome=await readState();assert.deepEqual(pendingHome.occurrence,before.occurrence);assert.deepEqual(pendingHome.practice,before.practice);
  releaseRefinement();await refinementReceipt.waitFor({state:'detached'});assert.equal(new URL(page.url()).pathname,'/');await historicalReceipt.getByText('Preparation needs your attention',{exact:true}).waitFor();
  const readyHome=await readState();assert.deepEqual(readyHome.occurrence,pendingHome.occurrence);assert.deepEqual(readyHome.practice,pendingHome.practice);assert.deepEqual(readyHome.events,pendingHome.events);assert.equal(Object.values(readyHome.concepts)[0].notes.length,2);result.evidence.push(await capture(page,'concept-home-poll-ready.png'));
  await page.goto(goalUrl);await page.getByText('A changed image receives a new entity tag.',{exact:false}).waitFor();const after=await readState();assert.deepEqual(after.events[0],original);assert.equal(after.feedback.length,1);assert.equal(Object.values(after.concepts)[0].notes.length,2);assert.equal(preparations,2);assert.equal(critics,2);assert.equal(checks,1);assert.ok(after.spend.every(s=>s.status==='known'));result.evidence.push(await capture(page,'create-refined-reference.png'));
 }catch(error){result.status='fail';result.reason=error.message;result.transport=transport;result.provider_calls={preparations,critics,checks};console.error(`Browser Create: ${error.message}`);if(page)result.evidence.push(await capture(page,'create-failure.png'));}
 finally{releasePreparation();releaseRefinement();await context?.close();server.closeAllConnections();await new Promise(done=>server.close(done));await runtime?.mf.dispose();rmSync(temporary,{recursive:true,force:true});}
 return result;
}
