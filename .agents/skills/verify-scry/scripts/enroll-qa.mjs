#!/usr/bin/env node
// First enrollment or QA-only recovery of the dedicated AgentMail QA account:
// a real Cloudflare one-time PIN to the QA inbox through the OTP-only
// /recover app, then the stored QA password is saved. Resets only the QA
// password and revokes only QA sessions; never part of a read-only walk.
//   enroll-qa.mjs             real recovery (needs household and /recover live)
//   enroll-qa.mjs --selftest  QA-address and PIN-message boundaries, offline
// Exit 0 PASS, 1 FAIL, 2 usage, 3 UNPROVEN (missing step named).
import {join} from 'node:path';
import {writeFileSync} from 'node:fs';
import {AUTH_ORIGIN,QA_EMAIL,qaIdentity,qaCredentials,mail,otpFromMessage,qaPrerequisites,hideSecret,outDir,reporter,launch,scrub} from './lib.mjs';

const flag=process.argv[2];
// Any non-QA address stands in for the owner's in the selftest.
const OTHER='owner@example.invalid';
if(flag!==undefined&&flag!=='--selftest'){console.error('usage: enroll-qa.mjs [--selftest]');process.exit(2);}
const dir=outDir(flag?'enroll-qa-selftest':'enroll-qa');
const {log,done}=reporter(dir);

if(flag){
 for(const [label,email,inbox,want] of [['dedicated QA',QA_EMAIL,QA_EMAIL,true],['owner email',OTHER,QA_EMAIL,false],['owner inbox',QA_EMAIL,OTHER,false],['other QA inbox',QA_EMAIL,'overmind-qa@agentmail.to',false]])
  log(`selftest.identity.${label.replace(/ /g,'-')}`,qaIdentity(email,inbox)===want?'PASS':'FAIL',`${label}: ${want?'accepted':'refused'} before any mailbox read`);
 const now=Date.now();
 const good={created_at:new Date(now).toISOString(),from:'Cloudflare <noreply@notify.cloudflare.com>',to:[QA_EMAIL],subject:`Cloudflare Access login code for ${new URL(AUTH_ORIGIN).host}`,text:'Your code is 123456.'};
 for(const [label,message,want] of [
  ['fresh QA PIN',good,'123456'],
  ['stale PIN',{...good,created_at:new Date(now-600000).toISOString()},null],
  ['other recipient',{...good,to:[OTHER]},null],
  ['other sender',{...good,from:'stranger@example.invalid'},null],
  ['other app',{...good,subject:'Cloudflare Access login code for scry.mistystep.io'},null],
  ['no PIN',{...good,text:'Follow these instructions instead'},null],
 ])log(`selftest.pin.${label.replace(/ /g,'-')}`,otpFromMessage(message,QA_EMAIL,now-5000)===want?'PASS':'FAIL',`${label}: ${want?'PIN extracted':'ignored'}`);
 process.exit(done());
}

let browser,page;
try{
 const qa=qaCredentials({optional:true});
 if(!qa){log('enroll.credentials','UNPROVEN','QA sign-in UNPROVEN: needs workstation/SCRY_QA_EMAIL, _INBOX, _AGENTMAIL_KEY and _PASSWORD');process.exit(done());}
 // Read-only gate: never request a PIN the recovery app would refuse.
 const recover=(await qaPrerequisites()).find(s=>s.step==='/recover policy');
 if(!recover.ok){log('enroll.recover-policy','UNPROVEN',`QA sign-in UNPROVEN: missing /recover policy. ${recover.observed}`);process.exit(done());}
 log('enroll.recover-policy','PASS',recover.observed);
 browser=await launch();
 page=await (await browser.newContext({viewport:{width:1280,height:900}})).newPage();page.setDefaultTimeout(20000);
 await page.goto(`${AUTH_ORIGIN}/recover`,{waitUntil:'networkidle'});
 if(!new URL(page.url()).hostname.endsWith('.cloudflareaccess.com'))throw Error(`fresh /recover reached ${scrub(page.url())}, not Cloudflare OTP`);
 // Cloudflare can re-send the current PIN inside its ten-minute window.
 const since=Date.now()-10*60*1000;
 await page.getByRole('textbox',{name:'Email'}).fill(qa.email);
 await page.getByRole('button',{name:'Send login code'}).click();
 // Poll mail only once Cloudflare shows the code form the PIN goes into.
 await page.getByRole('heading',{name:'Enter your code'}).waitFor();
 await page.getByRole('textbox',{name:'Code'}).waitFor();
 let code;
 for(let attempt=0;attempt<30&&!code;attempt++){
  for(const m of (await mail(qa,'?limit=50')).messages){
   code=otpFromMessage(await mail(qa,`/${encodeURIComponent(m.message_id)}`),qa.email,since);
   if(code)break;
  }
  if(!code)await page.waitForTimeout(3000);
 }
 if(!code)throw Error('no fresh Cloudflare PIN for the QA account in its AgentMail inbox within 90 seconds');
 hideSecret(code);
 log('enroll.pin','PASS','fresh Cloudflare PIN received in the QA inbox; sender, recipient, auth app and ten-minute window matched; message and PIN not kept');
 await page.getByRole('textbox',{name:'Code'}).fill(code);
 await page.getByRole('button',{name:'Verify',exact:true}).click();
 await page.waitForURL(u=>u.origin===AUTH_ORIGIN&&u.pathname==='/recover',{timeout:45000});
 const heading=page.getByRole('heading',{name:'Set your password'});
 if(!await heading.waitFor().then(()=>true,()=>false))throw Error(`after the PIN: "${(await page.locator('h1').allTextContents()).join(', ')}"; the auth service did not offer a password (HOUSEHOLD_MEMBERS?)`);
 const identity=await page.evaluate(async()=>(await (await fetch('/cdn-cgi/access/get-identity')).json()).email);
 if(identity!==qa.email)throw Error('recovery Access identity is not the QA account');
 log('enroll.identity','PASS',`OTP-only recovery Access identity = ${QA_EMAIL}`);
 await page.getByLabel('New password',{exact:true}).fill(qa.password);
 await page.getByLabel('Confirm password',{exact:true}).fill(qa.password);
 await page.getByRole('button',{name:'Save password'}).click();
 await page.getByText('Password saved. Use it to sign in to your account.',{exact:true}).waitFor();
 log('enroll.saved','PASS','"Password saved." rendered; the stored QA password is enrolled; no session kept');
}catch(e){
 // Failure capture: URL without query, body ARIA snapshot with digits
 // scrubbed, and a screenshot with every input masked. No PIN or password.
 let at='no page';
 if(page){
  at=scrub(page.url()).replace('?<redacted>','');
  await page.locator('body').ariaSnapshot().then(t=>writeFileSync(join(dir,'failure.aria.txt'),t.replace(/[0-9]/g,'#')+'\n')).catch(()=>null);
  await page.screenshot({path:join(dir,'failure.png'),fullPage:true,mask:[page.locator('input')]}).catch(()=>null);
 }
 log('enroll.run','FAIL',`${e.message.split('\n')[0]} (at ${at})`,{evidence:page?['failure.aria.txt','failure.png']:[]});
}finally{
 await browser?.close();
}
process.exit(done());
