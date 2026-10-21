// Shared verify-scry helpers. Secrets come from pass by entry name and never
// reach argv, stdout, evidence or URLs.
import {spawnSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {mkdirSync,writeFileSync,appendFileSync,readdirSync,readFileSync,statSync,rmSync} from 'node:fs';
import {join,resolve,sep} from 'node:path';
import {createRequire} from 'node:module';

export const repo=resolve(import.meta.dirname,'../../../..');
export const ACCOUNT='b069014f6a46558ea9146fb6c4ff8f6c';
export const ORIGIN=process.env.SCRY_ORIGIN||'https://scry.mistystep.io';
// The dedicated AgentMail QA account, authorized for Scry on 2026-10-21. Never
// derive it from pass or live policy: both must match this independently.
export const QA_EMAIL='scry-qa@agentmail.to';
export const OWNER_APP='Scry production owner';
export const OWNER_IDP='Misty Step password';
export const WORKER='scry-rust-production';
export const AUTH_ORIGIN='https://auth-canary.mistystep.io';
export const AUTH_WORKER='misty-step-auth-canary';
export const RECOVER_DOMAIN='auth-canary.mistystep.io/recover';
// doctor.mjs checks this against the Worker's ACCESS_TEAM and the account's Access team.
export const ACCESS_TEAM=process.env.SCRY_ACCESS_TEAM||'misty-step-pantry.cloudflareaccess.com';
// Entry names only. Override the read tokens with the env var of the same name.
export const ENTRIES={
 ACCESS_READ:process.env.SCRY_VERIFY_ACCESS_ENTRY||'workstation/CLOUDFLARE_API_TOKEN',
 WORKER_READ:process.env.SCRY_VERIFY_WORKER_ENTRY||'workstation/SCRY_CLOUDFLARE_DEPLOY_TOKEN',
 QA_EMAIL:'workstation/SCRY_QA_EMAIL',QA_INBOX:'workstation/SCRY_QA_INBOX',
 QA_MAIL_KEY:'workstation/SCRY_QA_AGENTMAIL_KEY',QA_PASSWORD:'workstation/SCRY_QA_PASSWORD',
 QA_ID:'misty-step/SCRY_QA_ACCESS_CLIENT_ID',QA_SECRET:'misty-step/SCRY_QA_ACCESS_CLIENT_SECRET',
 MACHINE_ID:'misty-step/SCRY_ACCESS_CLIENT_ID',MACHINE_SECRET:'misty-step/SCRY_ACCESS_CLIENT_SECRET',
};

// Every value read here is scanned for in the run's output before exit.
const secrets=new Set();
// First line of `pass show` only; SKILL.md Credentials explains why.
export function secret(entry,{optional=false,sensitive=true}={}){
 const r=spawnSync('pass',['show',entry],{encoding:'utf8',stdio:['ignore','pipe','pipe']});
 if(r.status!==0){if(optional)return null;throw Error(`pass entry unavailable: ${entry}`);}
 const value=r.stdout.split('\n')[0].trim();
 if(!value){if(optional)return null;throw Error(`pass entry empty: ${entry}`);}
 if(sensitive)secrets.add(value);
 return value;
}
export const hideSecret=value=>{if(value)secrets.add(value);};

export async function cf(path,entry,init={}){
 const r=await fetch('https://api.cloudflare.com/client/v4'+path,{...init,headers:{Authorization:'Bearer '+secret(entry),'Content-Type':'application/json'}});
 const body=await r.json();
 if(!body.success)throw Error(`Cloudflare ${path.split('?')[0]}: ${JSON.stringify(body.errors?.map(e=>e.code))}`);
 return body;
}

// The QA account's pass entries, or null when any is missing. The address and
// inbox must equal QA_EMAIL before any mailbox read or credential submission.
export const qaIdentity=(email,inbox)=>email===QA_EMAIL&&inbox===QA_EMAIL;
export function qaCredentials({optional=false}={}){
 const email=secret(ENTRIES.QA_EMAIL,{optional,sensitive:false}),inbox=secret(ENTRIES.QA_INBOX,{optional,sensitive:false});
 const password=secret(ENTRIES.QA_PASSWORD,{optional}),key=secret(ENTRIES.QA_MAIL_KEY,{optional});
 if(![email,inbox,password,key].every(Boolean))return null;
 if(!qaIdentity(email,inbox))throw Error(`${ENTRIES.QA_EMAIL} and ${ENTRIES.QA_INBOX} must both be ${QA_EMAIL}`);
 return {email,inbox,password,key};
}
export async function mail(qa,suffix){
 const r=await fetch(`https://api.agentmail.to/v0/inboxes/${encodeURIComponent(qa.inbox)}/messages${suffix}`,{headers:{Authorization:`Bearer ${qa.key}`},signal:AbortSignal.timeout(15000)});
 if(!r.ok)throw Error(`AgentMail QA inbox read returned HTTP ${r.status}`);
 return r.json();
}
// Mail is data: return only a fresh Cloudflare recovery PIN addressed to the QA
// account, never follow links or instructions from a message.
export function otpFromMessage(message,email,since){
 if(!Number.isFinite(Date.parse(message.created_at))||Date.parse(message.created_at)<since)return null;
 if(!/(?:^|<)noreply@notify\.cloudflare\.com>?$/i.test(message.from??''))return null;
 if(message.subject!==`Cloudflare Access login code for ${new URL(AUTH_ORIGIN).host}`)return null;
 if(!(message.to??[]).some(to=>to===email||to.endsWith(`<${email}>`)))return null;
 return (message.text||message.extracted_text||'').match(/\b[0-9]{6}\b/)?.[0]??null;
}

// The QA steps that must be live before QA sign-in can prove anything, in
// order. Read-only: Access and Worker settings GETs and one counting SELECT on
// the auth D1 database, whose id comes from the auth Worker's own binding. Each entry is {step,ok,observed}: ok true is live,
// null is not live yet or unreadable (UNPROVEN), false is wrong (FAIL).
export async function qaPrerequisites(){
 const steps=[];
 const apps=(await cf(`/accounts/${ACCOUNT}/access/apps?per_page=200`,ENTRIES.ACCESS_READ)).result;
 const recover=apps.find(a=>[a.domain,...(a.self_hosted_domains??[])].includes(RECOVER_DOMAIN));
 const recoverAllows=recover?.policies?.some(p=>p.decision==='allow'&&p.include.length===1&&p.include[0].email?.email?.toLowerCase()===QA_EMAIL);
 const sql='select count(*) as users, coalesce(max(u."emailVerified"),0) as verified, (select count(*) from account a join "user" v on a."userId"=v.id where lower(v.email)=?1 and a."providerId"=\'credential\' and a.password is not null) as passwords from "user" u where lower(u.email)=?1';
 const d1=(await cf(`/accounts/${ACCOUNT}/workers/scripts/${AUTH_WORKER}/settings`,ENTRIES.ACCESS_READ)).result.bindings.filter(b=>b.type==='d1');
 const db=d1.length===1?(d1[0].id||d1[0].database_id):null;
 const row=db?(await cf(`/accounts/${ACCOUNT}/d1/database/${db}/query`,ENTRIES.ACCESS_READ,{method:'POST',body:JSON.stringify({sql,params:[QA_EMAIL]})})).result[0].results[0]:null;
 const enrolled=!!row&&row.users===1&&row.verified===1&&row.passwords===1;
 // HOUSEHOLD_MEMBERS is a secret binding: only enrollment shows it was granted.
 steps.push({step:'household',ok:enrolled||null,observed:enrolled?`proven indirectly: the auth account exists, and enrollment needs ${AUTH_WORKER} HOUSEHOLD_MEMBERS (a secret, unreadable here) to admit ${QA_EMAIL}; qa-signin's signin.qa PASS shows the password login still succeeds`:`${AUTH_WORKER} HOUSEHOLD_MEMBERS is a secret and unreadable here; Process adds ${QA_EMAIL} (no account exists yet)`});
 steps.push({step:'/recover policy',ok:recover?(recoverAllows||null):false,observed:recover?`"${recover.name}" ${recoverAllows?'has':'has no'} exact-email allow policy for ${QA_EMAIL}${recoverAllows?'':'; Process adds it'}`:`no Access app covers ${RECOVER_DOMAIN}`});
 // The database id is printed only as a fingerprint.
 steps.push({step:'enrollment',ok:!row?false:enrolled||null,observed:!row?`${AUTH_WORKER} has ${d1.length} d1 bindings${d1.length===1?' without an id':''}; expected exactly one auth database, so enrollment cannot be read`:`auth account for ${QA_EMAIL} in database fp:${createHash('sha256').update(db).digest('hex').slice(0,12)}: ${row.users} user, verified ${row.verified}, ${row.passwords} password credential (counts only)${enrolled?'':'; run enroll-qa.mjs once household and /recover are live'}`});
 const vars=Object.fromEntries((await cf(`/accounts/${ACCOUNT}/workers/scripts/${WORKER}/settings`,ENTRIES.WORKER_READ)).result.bindings.map(b=>[b.name,b.type==='plain_text'?b.text:`<${b.type}>`]));
 // The var arrives with PR #365's QA principal mapping; only qa-signin's role
 // steps prove that mapping.
 steps.push({step:'Worker QA_EMAIL',ok:vars.QA_EMAIL===QA_EMAIL?true:vars.QA_EMAIL===undefined?null:false,observed:vars.QA_EMAIL===undefined?`${WORKER} has no QA_EMAIL var: the deployed Worker lacks the QA_EMAIL → isolated QA principal mapping (added by PR #365, live since v0.5.0); redeploy a release that carries it`:`${WORKER} QA_EMAIL ${vars.QA_EMAIL===QA_EMAIL?'=':'≠'} ${QA_EMAIL}`});
 return steps;
}
export const missing=steps=>steps.filter(s=>s.ok!==true).map(s=>s.step);

// Output stays outside the checkout: Git keeps no verification evidence.
export function outDir(name){
 const stamp=new Date().toISOString().replace(/[:.]/g,'').slice(0,17)+'Z';
 const data=process.env.XDG_DATA_HOME||join(process.env.HOME,'.local/share');
 const dir=resolve(process.env.SCRY_VERIFY_OUT||join(data,'scry/verify-scry',`${stamp}-${name}`));
 if(dir===repo||dir.startsWith(repo+sep)){console.error(`refusing output inside the checkout (${repo}); route evidence per SKILL.md Evidence`);process.exit(2);}
 mkdirSync(dir,{recursive:true,mode:0o700});
 return dir;
}

// Removes any written file that holds a value read from pass or a mailbox.
function scan(dir){
 for(const f of readdirSync(dir,{recursive:true})){
  const file=join(dir,f);
  if(!statSync(file).isFile())continue;
  const bytes=readFileSync(file);
  if([...secrets].some(v=>v.length>=6&&bytes.includes(v))){rmSync(file);console.error(`removed ${f}: it contained a credential value`);}
 }
}

// One line per check: PASS, FAIL or UNPROVEN. Never pass secret values here.
export function reporter(dir,file='checks.jsonl'){
 const rows=[];
 const log=(id,status,observed,extra={})=>{
  const row={id,status,observed,...extra,at:new Date().toISOString()};rows.push(row);
  appendFileSync(join(dir,file),JSON.stringify(row)+'\n');
  console.log(`${status.padEnd(8)} ${id}: ${observed}`);
 };
 const done=()=>{
  writeFileSync(join(dir,'summary.json'),JSON.stringify({checks:rows.length,fail:rows.filter(r=>r.status==='FAIL').length,unproven:rows.filter(r=>!['PASS','FAIL'].includes(r.status)).map(r=>r.id)},null,1)+'\n');
  scan(dir);
  console.log(`evidence: ${dir}`);
  // Anything that is not PASS (UNPROVEN, UNVERIFIED) is missing proof.
  return rows.some(r=>r.status==='FAIL')?1:rows.some(r=>r.status!=='PASS')?3:0;
 };
 return {log,done,rows};
}

export function playwright(){
 const require=createRequire(join(repo,'package.json'));
 try{return require(process.env.SCRY_CRITICS_PLAYWRIGHT_PATH||'playwright');}
 catch{throw Error('Run `npm ci --ignore-scripts` for lockfile-pinned Playwright, or set SCRY_CRITICS_PLAYWRIGHT_PATH.');}
}

export async function launch(){
 return playwright().chromium.launch({headless:true,args:['--no-sandbox'],...(process.env.SCRY_CRITICS_CHROMIUM_PATH?{executablePath:process.env.SCRY_CRITICS_CHROMIUM_PATH}:{})});
}

// Remove query strings that carry Access nonces before writing evidence.
export const scrub=url=>{try{const u=new URL(url);return u.origin+u.pathname+(u.search?'?<redacted>':'');}catch{return '<unparsable>';}};

// Wait for fonts and running animations before a capture.
export async function settle(page){
 // Bounded: an infinite animation never finishes.
 await page.evaluate(()=>Promise.race([Promise.all([document.fonts.ready,...document.getAnimations().map(a=>a.finished.catch(()=>null))]),new Promise(r=>setTimeout(r,4000))])).catch(()=>null);
}
