#!/usr/bin/env node
// Sign in to production as the dedicated AgentMail QA account through
// Cloudflare Access and the "Misty Step password" IdP, in a fresh browser with
// no injected Access headers, then prove its role: the Worker renders the
// isolated scry-qa space with owner capabilities and refuses owner operations.
//   qa-signin.mjs [--width 390|1280]
// GETs only: /create, /settings, /cdn-cgi/access/get-identity and a refused
// GET /operator/restore. A page GET may freeze the next question in the scry-qa
// space, never in the owner's. No browser state is kept.
// Exit 0 PROVEN, 1 FAIL, 2 usage, 3 QA sign-in UNPROVEN (missing step named).
import {join} from 'node:path';
import {writeFileSync} from 'node:fs';
import {ACCESS_TEAM,AUTH_ORIGIN,ORIGIN,QA_EMAIL,qaCredentials,qaPrerequisites,missing,outDir,reporter,launch,scrub,settle} from './lib.mjs';

const at=process.argv.indexOf('--width');
const width=at<0?390:Number(process.argv[at+1]);
if(![390,1280].includes(width)){console.error('usage: qa-signin.mjs [--width 390|1280]');process.exit(2);}
const dir=outDir('qa-signin');
const {log,done}=reporter(dir);
const hops=[];
let browser,page;
const shot=async name=>{await settle(page);await page.screenshot({path:join(dir,`${name}-${width}.png`),fullPage:true});return `${name}-${width}.png`;};
// Stops the journey at the first step that fails; later steps depend on it.
class Stop extends Error{}
const step=async(id,fn)=>{try{await fn();}catch(e){if(!(e instanceof Stop))log(id,'FAIL',e.message.split('\n')[0],{evidence:await shot(`${id}-failure`).catch(()=>null)});throw new Stop();}};
const unproven=(id,why)=>{log(id,'UNPROVEN',`QA sign-in UNPROVEN: ${why}`);throw new Stop();};
try{
 browser=await launch();
 // Fresh context: no extraHTTPHeaders, no storage state. Only Access can admit it.
 const context=await browser.newContext({viewport:{width,height:900},...(width<768?{isMobile:true,hasTouch:true}:{})});
 page=await context.newPage();page.setDefaultTimeout(20000);
 page.on('response',r=>{if(r.request().isNavigationRequest())hops.push(`${r.status()} ${scrub(r.url())}`);});

 await step('signin.redirect',async()=>{
  await page.goto(ORIGIN+'/create');
  await page.waitForURL(u=>u.origin===AUTH_ORIGIN);
  await page.getByRole('heading',{name:'Welcome back'}).waitFor();
  if(!hops.some(h=>h.includes(ACCESS_TEAM)))throw Error('sign-in was reached without passing through Access');
  if(/notebook|Owner token/i.test(await page.locator('body').innerText()))throw Error('sign-in screen shows Scry content');
  log('signin.redirect','PASS',`fresh browser GET /create → Access → ${new URL(page.url()).host} "Welcome back"; no Scry content`,{evidence:await shot('signin-idp')});
 });

 const qa=qaCredentials({optional:true});
 if(!qa)unproven('signin.qa','needs workstation/SCRY_QA_EMAIL, _INBOX, _AGENTMAIL_KEY and _PASSWORD');
 const steps=await qaPrerequisites();
 const wrong=steps.filter(s=>s.ok===false&&s.step==='Worker QA_EMAIL');
 if(wrong.length){log('qa.worker-qa-email','FAIL',wrong[0].observed);throw new Stop();}
 const enrollment=steps.find(s=>s.step==='enrollment');
 // Without an account there is nothing to sign in to: name every missing step.
 if(!enrollment.ok)unproven('signin.qa',`missing ${missing(steps).join(', ')}. ${steps.filter(s=>s.ok!==true).map(s=>`[${s.step}] ${s.observed}`).join(' ')}`);

 await step('signin.qa',async()=>{
  await page.getByLabel('Email').fill(qa.email);
  await page.getByLabel('Password',{exact:true}).fill(qa.password);
  await page.getByRole('button',{name:'Sign in'}).click();
  // A first OIDC authorization can show the consent step.
  const consent=page.getByRole('button',{name:'Continue',exact:true});
  await Promise.race([page.waitForURL(u=>u.origin===ORIGIN,{timeout:45000}),consent.waitFor({timeout:45000}).then(()=>consent.click())]);
  await page.waitForURL(u=>u.origin===ORIGIN,{timeout:45000}).catch(async()=>{throw Error(`still on ${scrub(page.url())} "${(await page.locator('h1').allTextContents()).join(', ')}": the auth service refused the QA account (HOUSEHOLD_MEMBERS, or the stored password; re-run enroll-qa.mjs)`);});
  log('signin.qa','PASS',`${QA_EMAIL} submitted at ${AUTH_ORIGIN}; Access returned to ${scrub(page.url())}`);
 });

 await step('signin.identity',async()=>{
  const email=await page.evaluate(async()=>{const r=await fetch('/cdn-cgi/access/get-identity',{credentials:'include'});return r.ok?(await r.json()).email??null:`HTTP ${r.status}`;});
  if(email!==QA_EMAIL)throw Error(`Access identity is ${email??'absent'}, not ${QA_EMAIL}`);
  log('signin.identity','PASS',`/cdn-cgi/access/get-identity email = ${QA_EMAIL} (not the owner)`);
 });

 // Required role: the isolated QA principal, owner capabilities in scry-qa only.
 await step('role.create',async()=>{
  if(new URL(page.url()).pathname!=='/create')await page.goto(ORIGIN+'/create');
  if(/Owner authorization required/.test(await page.content())){
   const worker=steps.find(s=>s.step==='Worker QA_EMAIL');
   if(!worker.ok)unproven('role.create',`Access admitted ${QA_EMAIL}, but the Worker answered 401 "Owner authorization required": missing PR #365 promotion. ${worker.observed}`);
   throw Error(`Worker refused ${QA_EMAIL} with QA_EMAIL deployed`);
  }
  const title=await page.title();
  await page.locator('[data-state="create"]').waitFor();
  if(title!=='Create · Scry')throw Error(`/create rendered "${title}"`);
  log('role.create','PASS',`Worker rendered "${title}" with [data-state="create"] for ${QA_EMAIL}`,{evidence:await shot('qa-create')});
 });
 await step('role.settings',async()=>{
  await page.goto(ORIGIN+'/settings');
  const title=await page.title();
  const tokens=page.locator('#owner-tokens');
  await tokens.waitFor();
  const create=await page.locator('form[action="/tokens/create"]').getByRole('button',{name:'Create Owner token'}).count();
  if(title!=='Your rhythm · Scry'||!create)throw Error(`/settings "${title}", Create Owner token form ${create?'present':'absent'}`);
  await tokens.scrollIntoViewIfNeeded();await settle(page);
  await page.screenshot({path:join(dir,`qa-settings-tokens-${width}.png`)});
  log('role.settings','PASS',`Settings "${title}" renders Owner tokens (#owner-tokens) with the Create Owner token form; nothing submitted`,{evidence:`qa-settings-tokens-${width}.png`});
 });
 await step('role.not-owner',async()=>{
  // Only the Worker's QA principal gets this refusal, before any learning space
  // is touched; the owner principal and an unknown identity get other answers.
  const r=await page.evaluate(async()=>{const r=await fetch('/operator/restore',{credentials:'include',redirect:'manual'});return {status:r.status,text:(await r.text()).slice(0,80)};});
  if(r.status!==403||!r.text.includes('QA cannot invoke owner operations.'))throw Error(`GET /operator/restore → ${r.status} "${r.text}"`);
  log('role.not-owner','PASS','GET /operator/restore → 403 "QA cannot invoke owner operations.": the Worker chose its QA principal, whose only space is scry-qa, not the owner principal or space');
 });

 await step('signin.expired',async()=>{
  // Drop the app session cookie; Access must re-authenticate.
  const kept=(await context.cookies()).filter(c=>!(c.name==='CF_Authorization'&&ORIGIN.endsWith(c.domain.replace(/^\./,''))));
  await context.clearCookies();await context.addCookies(kept);
  const reauth=[];page.on('response',r=>{if(r.request().isNavigationRequest())reauth.push(new URL(r.url()).host);});
  await page.goto(ORIGIN+'/create');
  const viaAccess=reauth.includes(ACCESS_TEAM);
  const signIn=new URL(page.url()).origin===AUTH_ORIGIN&&await page.getByRole('heading',{name:'Welcome back'}).isVisible();
  const again=new URL(page.url()).origin===ORIGIN&&await page.title()==='Create · Scry';
  if(!viaAccess||!(signIn||again))throw Error(`without CF_Authorization: via Access ${viaAccess}, landed on ${scrub(page.url())} "${await page.title()}"`);
  log('signin.expired','PASS',`without the app session cookie GET /create → Access → ${signIn?'Misty Step sign-in again':'session re-issued; /create rendered for QA'}`);
 });
}catch(e){
 if(!(e instanceof Stop))log('signin.run','FAIL',e.message.split('\n')[0]);
}finally{
 writeFileSync(join(dir,'navigation.txt'),hops.join('\n')+'\n');
 // Discards the context and its cookies.
 await browser?.close();
}
process.exit(done());
