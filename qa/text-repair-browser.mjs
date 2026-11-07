// Isolated test-only build. Normal/release Worker admission remains closed.
import assert from 'node:assert/strict';
import {cp,mkdir,mkdtemp,readFile,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {resolve,join,relative} from 'node:path';
import {execFileSync} from 'node:child_process';
import {createHash,generateKeyPairSync,sign,randomUUID} from 'node:crypto';
import {createRuntime,Response,internalKey} from './runtime-helper.mjs';
const origin='https://repair-http.invalid',space='synthetic-repair-http-only',team='repair-synthetic.cloudflareaccess.com',audience='synthetic-repair-audience',owner='synthetic-repair-owner';
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
async function candidateBuild(){
 const output=resolve('target/keyed-mapping/text-repair');await mkdir(output,{recursive:true});
 const normal=hash(await readFile('build/index_bg.wasm'));
 const temporary=await mkdtemp(join(tmpdir(),'scry-repair-browser-'));
 try{
  const root=resolve('.'),source=join(temporary,'source');
  await cp(root,source,{recursive:true,filter:path=>{const p=relative(root,path).split('/');return !['.git','target','build','node_modules','.wrangler','.tmp','data'].includes(p[0])&&!p.some(x=>x==='.env'||x.startsWith('.env.')||x==='.dev.vars'||x.startsWith('.dev.vars.'));}});
  let log;try{log=execFileSync('worker-build',['--release','--locked'],{cwd:source,env:{...process.env,CARGO_TARGET_DIR:resolve('target/text-repair-wasm'),RUSTFLAGS:'--cfg scry_synthetic_repair_http'},maxBuffer:32*1024*1024});}catch(error){await writeFile(join(output,'build.log'),Buffer.concat([error.stdout||Buffer.alloc(0),error.stderr||Buffer.alloc(0)]));throw error;}
  await writeFile(join(output,'build.log'),log);await cp(join(source,'build'),join(output,'worker'),{recursive:true});
  assert.equal(hash(await readFile('build/index_bg.wasm')),normal,'test build must not replace normal artifact');
  await writeFile(join(output,'artifact.json'),JSON.stringify({normalWasmSha256:normal,testWasmSha256:hash(await readFile(join(output,'worker/index_bg.wasm'))),compileFlag:'scry_synthetic_repair_http',normalAdmission:'closed'},null,2)+'\n');
  return join(output,'worker');
 }finally{await rm(temporary,{recursive:true,force:true});}
}
export async function textRepairWalk(browser,capture){
 const artifact=await candidateBuild(),report=[];
 const different=JSON.parse(await readFile('target/text-repair/different-held-archive.json','utf8'));
 const same=structuredClone(different);const held=same.app.practice.current.prompt.version_id;
 for(const [javaScriptEnabled,other]of [[false,false],[true,false],[false,true],[true,true]]){
  const label=`repair-${javaScriptEnabled?'enhanced':'native'}-${other?'other':'held'}`,name=label+'-'+randomUUID();
  const archive=structuredClone(same),original=other?Object.keys(archive.app.library.versions).find(id=>id!==held):held,bytes=Buffer.from(JSON.stringify(archive));
  const {publicKey,privateKey}=generateKeyPairSync('rsa',{modulusLength:2048});const jwk={...publicKey.export({format:'jwk'}),kid:'synthetic-repair-key',alg:'RS256',use:'sig'};
  const b64=v=>Buffer.from(JSON.stringify(v)).toString('base64url');const jwt=()=>{const now=Math.floor(Date.now()/1000),value=b64({alg:'RS256',kid:jwk.kid,typ:'JWT'})+'.'+b64({iss:`https://${team}`,sub:owner,aud:[audience],iat:now,nbf:now-1,exp:now+600});return value+'.'+sign('RSA-SHA256',Buffer.from(value),privateKey).toString('base64url');};
  let bindings={SCRY_ENV:'restore',CANONICAL_ORIGIN:origin,SPACE_NAME:space,INTERNAL_KEY:internalKey,RESTORE_TOKEN:'synthetic-repair-browser-restore-32chars'};
  let rt=await createRuntime({name,artifact,port:0,inspect:true,bindings,egress:async request=>{assert.equal(request.url,`https://${team}/cdn-cgi/access/certs`);return new Response(JSON.stringify({keys:[jwk]}),{headers:{'Content-Type':'application/json'}});}});
  const result={name:label,status:'pass',javaScriptEnabled,otherVersion:other,evidence:[]};report.push(result);let context,page;
  try{
   let response=await rt.worker.fetch(origin+'/operator/restore',{method:'POST',headers:{'x-scry-restore-token':bindings.RESTORE_TOKEN,'x-scry-sha256':hash(bytes)},body:bytes});assert.equal(response.status,200,await response.text());
   bindings={SCRY_ENV:'preview',CANONICAL_ORIGIN:origin,SPACE_NAME:space,INTERNAL_KEY:internalKey,ACCESS_TEAM:team,ACCESS_AUDIENCE:audience,OWNER_SUBJECT:owner};rt={...rt,...await rt.rebind(bindings)};
   const rows=async(notebook=space)=>Object.fromEntries((await(await rt.mf.unsafeGetDurableObjectStorage(name,'LearningSpace',{name:notebook})).exec('SELECT key,value FROM records ORDER BY key')).map(r=>[r.key,r.value]));
   const before=await rows(),practice=JSON.parse(before.meta).practice;
   context=await browser.newContext({viewport:{width:javaScriptEnabled?390:320,height:844},javaScriptEnabled,isMobile:true,hasTouch:true,colorScheme:javaScriptEnabled?'light':'dark'});page=await context.newPage();page.setDefaultTimeout(15000);
   let lose=javaScriptEnabled,posts=[];
   // Browser owns native navigation, forms, JS and History. The transport-only
   // route supplies synthetic Access identity, verifies origin, and has no egress.
   await context.route('**/*',async route=>{
    const request=route.request();if(!request.url().startsWith(origin+'/')){await route.abort();throw Error('Unexpected browser egress');}
    const browserHeaders=await request.allHeaders();const headers={...browserHeaders,'Cf-Access-Jwt-Assertion':jwt()};delete headers['content-length'];
    // Preserve the browser's actual Origin through Miniflare's loopback-only
    // Origin guard. runtime-helper restores it before Rust sees the request.
    // Never fabricate an Origin for a request that did not carry one.
    if(browserHeaders.origin!==undefined){headers['X-Scry-Test-Origin']=browserHeaders.origin;delete headers.origin;}
    const response=await rt.worker.fetch(request.url(),{method:request.method(),headers,...(request.method()==='POST'?{body:request.postDataBuffer()}:{}),redirect:'manual'});
    if(request.method()==='POST'&&new URL(request.url()).pathname==='/library/version/repair'){
     posts.push({url:request.url(),body:request.postData(),scope:browserHeaders['x-scry-expected-scope']||null,status:response.status});
     if(response.status!==303&&response.status!==409){
      let csrfMatches=false,formReadable=false;try{const form=await new Request(request.url(),{method:'POST',headers:browserHeaders,body:request.postDataBuffer()}).formData();formReadable=true;csrfMatches=form.get('csrf')===JSON.parse((await rows()).meta).csrf;}catch{}
      result.refusal={status:response.status,origin:browserHeaders.origin||null,fetchSite:browserHeaders['sec-fetch-site']||null,contentType:browserHeaders['content-type']?.split(';')[0]||null,formReadable,csrfMatches,reason:(await response.clone().text()).slice(0,180)};
     }
     if(lose&&response.status===303){lose=false;await route.abort('failed');return;}
    }
    const body=Buffer.from(await response.arrayBuffer());const responseHeaders=Object.fromEntries(response.headers);delete responseHeaders['content-length'];await route.fulfill({status:response.status,headers:responseHeaders,body});
   });
   const url=origin+'/library/repair?version_id='+original;
   await page.goto(origin+'/recall');assert.equal(await page.locator('#unavailable-draft').inputValue(),practice.answer_checkpoint.draft.answer);
   await page.goto(url);await page.locator('[data-state="repair-recovery"]').waitFor();
   assert.equal(await page.locator('[data-repair-other-prompt]').count(),other?1:0);assert.ok(!(await page.locator('#main').innerText()).includes(archive.app.library.versions[original].text));
   assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
   await page.getByRole('link',{name:'Close and return to saved prompt'}).click();await page.locator('[data-state="practice-unavailable"]').waitFor();assert.deepEqual(await rows(),before);
   await page.goBack();await page.locator('[data-state="repair-recovery"]').waitFor();await page.reload();assert.deepEqual(await rows(),before);
   // Retain the exact server-rendered document to represent a cached native
   // form after a lost response. No fields or application state are invented.
   const originalHtml=await page.content();
   await page.getByRole('button',{name:'Create repaired copy',exact:true}).click();
   if(javaScriptEnabled){await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();assert.equal(posts.length,1);await page.waitForTimeout(250);assert.equal(posts.length,1);await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).click();}
   await page.locator('[data-state="practice-unavailable"]').waitFor();
   const repaired=await rows();assert.deepEqual(JSON.parse(repaired.meta).practice,practice);const repair=JSON.parse(repaired['library_repairs/'+original]);assert.equal(Object.keys(repaired).filter(k=>k.startsWith('library_versions/')).length,Object.keys(before).filter(k=>k.startsWith('library_versions/')).length+1);
   if(!javaScriptEnabled){
    // Actual no-JS browser controls submit the unchanged cached native form.
    const retained=await context.newPage();await retained.route(url,route=>route.fulfill({status:200,contentType:'text/html',body:originalHtml}));await retained.goto(url);await retained.getByRole('button',{name:'Create repaired copy',exact:true}).click();await retained.locator('[data-state="practice-unavailable"]').waitFor();await retained.close();assert.deepEqual(await rows(),repaired);result.nativeRetry='unchanged server-rendered cached document, actual no-JS submit';
   }
   assert.equal(posts.length,2);const operation=body=>body.includes('name="operation_id"')?/name="operation_id"\r\n\r\n([^\r]+)/.exec(body)?.[1]:new URLSearchParams(body).get('operation_id');assert.ok(operation(posts[0].body));assert.equal(operation(posts[0].body),operation(posts[1].body));if(!javaScriptEnabled)assert.ok(posts.every(p=>p.scope===null));
   await page.goto(url);await page.locator('[data-repair-copy]').waitFor();assert.equal(await page.locator('[data-repair-copy]').innerText(),repair.copy);assert.equal(await page.locator('[data-repair-create]').count(),0);await page.reload();assert.deepEqual(await rows(),repaired);
   await page.getByRole('button',{name:'Enroll copy without switching'}).click();await page.locator('[data-state="practice-unavailable"]').waitFor();const enrolled=await rows();assert.deepEqual(JSON.parse(enrolled.meta).practice,practice);assert.equal(Object.entries(enrolled).filter(([k,v])=>k.startsWith('practice_studies/')&&JSON.parse(v).version_id===repair.copy).length,1);
   await page.goto(url);await page.locator('[data-repair-enrolled]').waitFor();assert.equal(await page.locator('[data-repair-other-prompt]').count(),other?1:0);assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));result.evidence.push(await capture(page,label+'-enrolled.png'));
   // A retained old document must not cross notebook identity after routing changes.
   await page.route(url,route=>route.fulfill({status:200,contentType:'text/html',body:originalHtml}));await page.goto(url);const otherSpace='synthetic-repair-other-notebook';rt={...rt,...await rt.rebind({...bindings,SPACE_NAME:otherSpace})};const otherBefore=await rows(otherSpace);
   await page.getByRole('button',{name:'Create repaired copy',exact:true}).click();if(javaScriptEnabled)await page.locator('body.private-hidden').waitFor({state:'attached'});else await page.getByText('Notebook changed.',{exact:true}).waitFor();assert.deepEqual(await rows(),enrolled);assert.deepEqual(await rows(otherSpace),otherBefore);assert.equal(posts.at(-1).status,409);result.staleNotebookRefused=true;result.heldPromptAndDraftPreserved=true;
  }catch(error){result.status='fail';result.reason=error.message;if(page)result.evidence.push(await capture(page,label+'-failure.png'));}
  finally{if(context)await context.close();await rt.mf.dispose();}
 }
 await writeFile('target/keyed-mapping/text-repair/browser.json',JSON.stringify(report,null,2)+'\n');console.log('Synthetic repair browser results: '+JSON.stringify(report));return report;
}
