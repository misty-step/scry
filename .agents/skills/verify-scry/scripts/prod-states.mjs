#!/usr/bin/env node
// Read-only production breaking states: signed out, forged/expired session,
// wrong identity, alternate host, and 390px phone. No owner data is read and
// nothing in the owner space is written: every request is refused or an
// isolated-QA read, which may freeze a question in the scry-qa space only.
//   prod-states.mjs
// Exit 0 PASS, 1 FAIL, 3 UNPROVEN left.
import {join} from 'node:path';
import {writeFileSync} from 'node:fs';
import {ACCESS_TEAM,ORIGIN,ENTRIES,secret,outDir,reporter,launch,settle} from './lib.mjs';

const dir=outDir('prod-states');
const {log,done}=reporter(dir);
const host=new URL(ORIGIN).host;
const check=async(id,fn)=>{try{await fn();}catch(e){log(id,'FAIL',e.message);}};
const get=(url,headers={},method='GET')=>fetch(url,{method,headers,redirect:'manual'});
// The redirect must go to this account's Access team, not just any login path.
const toAccess=r=>{if(r.status!==302)return false;const u=new URL(r.headers.get('location')||'http://x');return u.origin===`https://${ACCESS_TEAM}`&&u.pathname===`/cdn-cgi/access/login/${host}`;};
const qa=()=>({'CF-Access-Client-Id':secret(ENTRIES.QA_ID),'CF-Access-Client-Secret':secret(ENTRIES.QA_SECRET)});
// Structurally valid, unsigned Access assertion with an expired exp.
const b64=o=>Buffer.from(JSON.stringify(o)).toString('base64url');
const forged=`${b64({alg:'RS256',kid:'forged'})}.${b64({sub:'forged',exp:1,iss:'https://forged.cloudflareaccess.com'})}.${Buffer.from('sig').toString('base64url')}`;

await check('state.signed-out',async()=>{
 for(const path of ['/','/create','/settings','/api/v1/library']){
  const r=await get(ORIGIN+path);
  if(!toAccess(r))throw Error(`anonymous ${path} → ${r.status}`);
 }
 log('state.signed-out','PASS','anonymous GET / /create /settings /api/v1/library → 302 Access login; no Worker content');
});
await check('state.expired-or-forged',async()=>{
 const a=await get(ORIGIN+'/create',{Cookie:`CF_Authorization=${forged}`});
 const b=await get(ORIGIN+'/create',{'Cf-Access-Jwt-Assertion':forged});
 const c=await get(ORIGIN+'/create',{'CF-Access-Client-Id':'forged.access','CF-Access-Client-Secret':'forged'});
 if(!toAccess(a)||!toAccess(b)||!toAccess(c))throw Error(`expired cookie ${a.status}, forged header ${b.status}, forged service token ${c.status}`);
 log('state.expired-or-forged','PASS','expired CF_Authorization cookie, forged Cf-Access-Jwt-Assertion and forged service token → 302 Access login');
});
await check('state.wrong-identity',async()=>{
 const r=await get(ORIGIN+'/create',qa());const html=await r.text();
 const title=/<title>([^<]*)/.exec(html)?.[1];
 if(r.status!==200||title!=='Create · Scry')throw Error(`QA identity /create → ${r.status} ${title}`);
 const api=await get(ORIGIN+'/api/v1/library',qa());
 const op=await get(ORIGIN+'/operator/restore',qa(),'POST');
 if(api.status!==401||op.status!==403)throw Error(`QA identity API ${api.status}, operator ${op.status}`);
 const machine=await get(ORIGIN+'/create',{'CF-Access-Client-Id':secret(ENTRIES.MACHINE_ID),'CF-Access-Client-Secret':secret(ENTRIES.MACHINE_SECRET)});
 if(machine.status===200)throw Error('machine health identity reached /create');
 log('state.wrong-identity','PASS',`isolated QA service identity → 200 its own scry-qa space only; owner API → ${api.status}; operator → ${op.status}; machine health identity /create → ${machine.status}`);
});
await check('state.alternate-host',async()=>{
 const r=await get('https://scry.study/create',qa());
 const w=await get('https://scry.study/create',qa(),'POST');
 if(r.status!==303||r.headers.get('location')!==ORIGIN+'/create'||w.status!==421)throw Error(`alternate GET ${r.status} ${r.headers.get('location')}, POST ${w.status}`);
 log('state.alternate-host','PASS',`scry.study GET → 303 ${ORIGIN}/create; POST → 421`);
});
await check('state.phone-390',async()=>{
 const browser=await launch();
 try{
  const context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,extraHTTPHeaders:qa()});
  const page=await context.newPage();
  const r=await page.goto(ORIGIN+'/create');
  await page.locator('[data-state="create"]').waitFor();
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth);
  if(r.status()!==200||overflow)throw Error(`390px QA service identity /create → ${r.status()}, horizontal overflow ${overflow}`);
  await settle(page);await page.screenshot({path:join(dir,'qa-identity-create-390.png'),fullPage:true});
  log('state.phone-390','PASS','QA service identity /create at 390px touch: 200, no horizontal overflow',{evidence:'qa-identity-create-390.png'});
 }finally{await browser.close();}
});
await check('cli.hosted-admission',async()=>{
 // #335: Access answers before the Worker can verify an Owner token.
 const r=await get(ORIGIN+'/api/v1/whoami',{Authorization:'Bearer scry_not_a_real_token'});
 if(!toAccess(r))throw Error(`Bearer /api/v1/whoami → ${r.status}; hosted admission changed, re-verify #335`);
 log('cli.hosted-admission','UNPROVEN','Bearer /api/v1/whoami → 302 Access login: hosted Owner-token admission (#335) is not commissioned');
});
log('state.backend-down','UNPROVEN','not inducible on production read-only; local-journeys.mjs stops its own Worker');
writeFileSync(join(dir,'README.txt'),'Production breaking states. QA identity reads its isolated scry-qa space; no owner data.\n');
process.exit(done());
