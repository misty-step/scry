// Shared verify-scry helpers. Secrets come from pass by entry name and never
// reach argv, stdout, evidence or URLs.
import {spawnSync} from 'node:child_process';
import {mkdirSync,writeFileSync,appendFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {createRequire} from 'node:module';

export const repo=resolve(import.meta.dirname,'../../../..');
export const ACCOUNT='b069014f6a46558ea9146fb6c4ff8f6c';
export const ORIGIN=process.env.SCRY_ORIGIN||'https://scry.mistystep.io';
export const OWNER_EMAIL=process.env.SCRY_OWNER_EMAIL||'phaedrus@mistystep.io';
export const OWNER_APP='Scry production owner';
export const OWNER_IDP='Misty Step password';
export const WORKER='scry-rust-production';
// doctor.mjs checks this against the Worker's ACCESS_TEAM and the account's Access team.
export const ACCESS_TEAM=process.env.SCRY_ACCESS_TEAM||'misty-step-pantry.cloudflareaccess.com';
// Entry names only. Override with the env var of the same name.
export const ENTRIES={
 ACCESS_READ:process.env.SCRY_VERIFY_ACCESS_ENTRY||'workstation/CLOUDFLARE_API_TOKEN',
 WORKER_READ:process.env.SCRY_VERIFY_WORKER_ENTRY||'workstation/SCRY_CLOUDFLARE_DEPLOY_TOKEN',
 QA_ID:'misty-step/SCRY_QA_ACCESS_CLIENT_ID',QA_SECRET:'misty-step/SCRY_QA_ACCESS_CLIENT_SECRET',
 MACHINE_ID:'misty-step/SCRY_ACCESS_CLIENT_ID',MACHINE_SECRET:'misty-step/SCRY_ACCESS_CLIENT_SECRET',
 // Not provisioned as of 2026-10-20; see features/sign-in.md Gotchas.
 OWNER_PASSWORD:process.env.SCRY_OWNER_PASSWORD_ENTRY||'workstation/MISTY_STEP_OWNER_PASSWORD',
};

// `pass show` keeps the stored trailing newline. `pass-env run -e` passes the
// whole plaintext, newline included, so Access sees a different client secret
// and treats the service token as absent. Use the first line only.
export function secret(entry,{optional=false}={}){
 const r=spawnSync('pass',['show',entry],{encoding:'utf8',stdio:['ignore','pipe','pipe']});
 if(r.status!==0){if(optional)return null;throw Error(`pass entry unavailable: ${entry}`);}
 const value=r.stdout.split('\n')[0].trim();
 if(!value){if(optional)return null;throw Error(`pass entry empty: ${entry}`);}
 return value;
}

export async function cf(path,entry){
 const r=await fetch('https://api.cloudflare.com/client/v4'+path,{headers:{Authorization:'Bearer '+secret(entry)}});
 const body=await r.json();
 if(!body.success)throw Error(`Cloudflare ${path}: ${JSON.stringify(body.errors?.map(e=>e.code))}`);
 return body;
}

export function outDir(name){
 const stamp=new Date().toISOString().replace(/[:.]/g,'').slice(0,17)+'Z';
 const dir=resolve(process.env.SCRY_VERIFY_OUT||join(repo,'target/verify-scry',`${stamp}-${name}`));
 mkdirSync(dir,{recursive:true});
 return dir;
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
