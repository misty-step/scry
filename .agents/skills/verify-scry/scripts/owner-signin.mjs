#!/usr/bin/env node
// Sign in to production as the real owner through Cloudflare Access and the
// "Misty Step password" IdP, then prove an owner-only page renders.
//   owner-signin.mjs [--width 390|1280]
// Loads only /create after sign-in: it renders no notebook content. Like any
// owner GET it may freeze his next concept question, as opening Scry does, but
// unlike `/` it freezes no exact Prompt and marks no exposure.
// No browser state is persisted. Exit 0 PROVEN, 1 FAIL, 3 UNPROVEN.
import {join} from 'node:path';
import {ACCESS_TEAM,ORIGIN,OWNER_EMAIL,ENTRIES,secret,outDir,reporter,launch,scrub,settle} from './lib.mjs';

const width=Number(process.argv[process.argv.indexOf('--width')+1]||390)||390;
const dir=outDir('owner-signin');
const {log,done}=reporter(dir);
const browser=await launch();
const context=await browser.newContext({viewport:{width,height:900},...(width<768?{isMobile:true,hasTouch:true}:{})});
const page=await context.newPage();page.setDefaultTimeout(20000);
const hops=[];
page.on('response',r=>{if(r.request().isNavigationRequest())hops.push(`${r.status()} ${scrub(r.url())}`);});
const shot=async name=>{await settle(page);await page.screenshot({path:join(dir,name),fullPage:true});return name;};
try{
 // 1. Signed out: Access sends the browser to the owner's IdP.
 await page.goto(ORIGIN+'/create');
 await page.waitForURL(u=>u.host==='auth-canary.mistystep.io');
 await page.getByRole('heading',{name:'Welcome back'}).waitFor();
 log('signin.redirect','PASS',`signed-out GET /create → Access → ${new URL(page.url()).host} "Welcome back"`,{evidence:await shot(`signin-idp-${width}.png`)});

 // 2. Credential: the owner's Misty Step password, by pass entry name only.
 const password=secret(ENTRIES.OWNER_PASSWORD,{optional:true});
 if(!password){
  log('signin.owner','UNPROVEN',`needs pass entry ${ENTRIES.OWNER_PASSWORD} holding the ${OWNER_EMAIL} Misty Step password (normal sign-in has no email code or link)`);
 }else{
  await page.getByLabel('Email').fill(OWNER_EMAIL);
  await page.getByLabel('Password',{exact:true}).fill(password);
  await page.getByRole('button',{name:'Sign in'}).click();
  // A first OIDC authorization can show the consent step.
  const consent=page.getByRole('button',{name:'Continue',exact:true});
  await Promise.race([page.waitForURL(u=>u.origin===ORIGIN),consent.waitFor().then(()=>consent.click())]);
  await page.waitForURL(u=>u.origin===ORIGIN&&u.pathname==='/create');
  const main=await page.locator('main').innerText().catch(()=>'');
  const title=await page.title();
  if(title!=='Create · Scry'||/Owner authorization required/.test(await page.content()))throw Error(`after sign-in: "${title}" ${main.slice(0,80)}`);
  await page.locator('[data-state="create"]').waitFor();
  log('signin.owner','PASS',`${OWNER_EMAIL} signed in; owner-only ${ORIGIN}/create rendered "${title}"`,{evidence:await shot(`signin-owner-create-${width}.png`)});

  // 3. Expired session: drop the app session cookie; Access must re-authenticate.
  const kept=(await context.cookies()).filter(c=>!(c.name==='CF_Authorization'&&ORIGIN.endsWith(c.domain.replace(/^\./,''))));
  await context.clearCookies();await context.addCookies(kept);
  // Access must either send the browser back through the IdP or re-issue the
  // app session from its own session; either way /create renders for the owner.
  const reauth=[];page.on('response',r=>{if(r.request().isNavigationRequest())reauth.push(new URL(r.url()).host);});
  await page.goto(ORIGIN+'/create');
  const viaAccess=reauth.includes(ACCESS_TEAM);
  const signIn=new URL(page.url()).host==='auth-canary.mistystep.io'&&await page.getByRole('heading',{name:'Welcome back'}).isVisible();
  const owner=new URL(page.url()).origin===ORIGIN&&await page.title()==='Create · Scry'&&await page.locator('[data-state="create"]').count()===1;
  if(!viaAccess||!(signIn||owner))throw Error(`without CF_Authorization: via Access ${viaAccess}, landed on ${scrub(page.url())} "${await page.title()}"`);
  log('signin.expired','PASS',`without the app session cookie GET /create → Access → ${signIn?'Misty Step sign-in again':'session re-issued; owner-only /create rendered'}`);
 }
}catch(e){
 log('signin.owner','FAIL',e.message,{evidence:await shot(`signin-failure-${width}.png`).catch(()=>null)});
}finally{
 await import('node:fs').then(fs=>fs.writeFileSync(join(dir,'navigation.txt'),hops.join('\n')+'\n'));
 await context.close();await browser.close();
}
console.log(`evidence: ${dir}`);
process.exit(done());
