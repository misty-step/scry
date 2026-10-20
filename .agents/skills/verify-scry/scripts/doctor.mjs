#!/usr/bin/env node
// Read-only: is this Scry worth driving, and would the real owner be admitted?
//   doctor.mjs prod            deployed owner identity and private ingress
//   doctor.mjs local <url>     a run-owned loopback Worker (bun run dev)
// Exit 0 all PASS, 1 any FAIL, 3 only UNPROVEN left.
import {createHash} from 'node:crypto';
import {ACCOUNT,ORIGIN,OWNER_EMAIL,OWNER_APP,OWNER_IDP,WORKER,ENTRIES,cf,secret,outDir,reporter} from './lib.mjs';

const [mode='prod',target]=process.argv.slice(2);
const dir=outDir(`doctor-${mode}`);
const {log,done}=reporter(dir);
const check=async(id,fn)=>{try{await fn();}catch(e){log(id,'FAIL',e.message);}};
const fp=v=>createHash('sha256').update(v).digest('hex').slice(0,12);

if(mode==='local'){
 const url=new URL(target||'http://127.0.0.1:8787');
 await check('local.loopback',async()=>{
  if(!['127.0.0.1','localhost','[::1]'].includes(url.hostname))throw Error(`${url.host} is not loopback; drive production only through doctor prod`);
  log('local.loopback','PASS',url.origin);
 });
 await check('local.page',async()=>{
  const r=await fetch(url.origin+'/',{redirect:'manual'});const html=await r.text();
  const title=/<title>([^<]*)<\/title>/.exec(html)?.[1];
  if(r.status!==200||!title?.endsWith('· Scry'))throw Error(`GET / → ${r.status} ${title||'no title'}`);
  log('local.page','PASS',`GET / → 200 "${title}"`);
 });
 await check('local.changelog',async()=>{
  const html=await(await fetch(url.origin+'/changelog')).text();
  log('local.changelog','PASS',/Version [0-9.]+/.exec(html)?.[0]||'version not rendered');
 });
 process.exit(done());
}
if(mode!=='prod'){console.error('usage: doctor.mjs prod | doctor.mjs local <url>');process.exit(2);}

const host=new URL(ORIGIN).host;
let vars={},app,idp,user;
await check('worker.config',async()=>{
 const s=(await cf(`/accounts/${ACCOUNT}/workers/scripts/${WORKER}/settings`,ENTRIES.WORKER_READ)).result;
 for(const b of s.bindings)vars[b.name]=b.type==='plain_text'?b.text:`<${b.type}>`;
 const want={SCRY_ENV:'production',CANONICAL_ORIGIN:ORIGIN,OWNER_SUBJECT:'<secret_text>'};
 const bad=Object.entries(want).filter(([k,v])=>vars[k]!==v).map(([k])=>k);
 if(!vars.ACCESS_TEAM||!vars.ACCESS_AUDIENCE)bad.push('ACCESS_TEAM/ACCESS_AUDIENCE');
 if(bad.length)throw Error(`unexpected ${bad.join(', ')}`);
 log('worker.config','PASS',`SCRY_ENV=production, CANONICAL_ORIGIN=${ORIGIN}, ACCESS_TEAM=${vars.ACCESS_TEAM}, OWNER_SUBJECT bound (value unreadable)`);
});
await check('worker.version',async()=>{
 const d=(await cf(`/accounts/${ACCOUNT}/workers/scripts/${WORKER}/deployments`,ENTRIES.WORKER_READ)).result.deployments;
 const code=d.find(x=>x.annotations?.['workers/triggered_by']==='deployment');
 const subject=d.find(x=>x.annotations?.['workers/triggered_by']==='secret');
 log('worker.version','PASS',`serving deployment ${d[0].id} at ${d[0].created_on} (${d[0].annotations?.['workers/triggered_by']}); code ${code?.annotations?.['workers/message']||'unknown'}; last secret write ${subject?.created_on||'none listed'}`,{last_secret_write:subject?.created_on||null});
});
await check('access.team',async()=>{
 const org=(await cf(`/accounts/${ACCOUNT}/access/organizations`,ENTRIES.ACCESS_READ)).result;
 if(org.auth_domain!==vars.ACCESS_TEAM)throw Error(`Access team ${org.auth_domain} ≠ ACCESS_TEAM ${vars.ACCESS_TEAM}`);
 log('access.team','PASS',`Access team ${org.auth_domain} = ACCESS_TEAM`);
});
await check('access.app',async()=>{
 app=(await cf(`/accounts/${ACCOUNT}/access/apps?per_page=100`,ENTRIES.ACCESS_READ)).result.find(a=>a.name===OWNER_APP);
 if(!app)throw Error(`no Access app named "${OWNER_APP}"`);
 if(app.aud!==vars.ACCESS_AUDIENCE)throw Error(`app AUD ${app.aud.slice(0,12)}… ≠ ACCESS_AUDIENCE ${String(vars.ACCESS_AUDIENCE).slice(0,12)}…`);
 if(!app.self_hosted_domains.includes(host))throw Error(`${host} not covered by "${OWNER_APP}"`);
 log('access.app','PASS',`"${OWNER_APP}" AUD = ACCESS_AUDIENCE; covers ${app.self_hosted_domains.join(', ')}; session ${app.session_duration}`);
});
await check('access.idp',async()=>{
 const idps=(await cf(`/accounts/${ACCOUNT}/access/identity_providers`,ENTRIES.ACCESS_READ)).result;
 idp=idps.find(i=>i.name===OWNER_IDP);
 if(!idp)throw Error(`no identity provider named "${OWNER_IDP}"`);
 const allowed=app.allowed_idps.map(id=>idps.find(i=>i.id===id)?.name||id);
 if(!app.allowed_idps.includes(idp.id))throw Error(`"${OWNER_APP}" does not allow "${OWNER_IDP}" (allows ${allowed.join(', ')})`);
 log('access.idp','PASS',`allowed IdPs: ${allowed.join(', ')} (${idp.type} at ${new URL(idp.config.auth_url).host}); auto-redirect ${app.auto_redirect_to_identity}`);
});
await check('access.policy',async()=>{
 const allow=app.policies.filter(p=>p.decision==='allow');
 if(!allow.some(p=>p.include.some(r=>r.email?.email===OWNER_EMAIL)))throw Error(`no allow policy includes ${OWNER_EMAIL}`);
 const wide=allow.flatMap(p=>p.include).filter(r=>r.everyone||r.email_domain||r.login_method);
 if(wide.length)throw Error(`allow policy is wider than exact emails: ${JSON.stringify(wide.map(Object.keys))}`);
 log('access.policy','PASS',`allow policy includes ${OWNER_EMAIL} (exact emails only); non-identity policies: ${app.policies.filter(p=>p.decision==='non_identity').map(p=>p.name).join(', ')||'none'}`);
});
await check('owner.identity',async()=>{
 user=(await cf(`/accounts/${ACCOUNT}/access/users?per_page=100`,ENTRIES.ACCESS_READ)).result.find(u=>u.email===OWNER_EMAIL);
 if(!user)throw Error(`no Access user for ${OWNER_EMAIL}; the owner has never signed in through Access`);
 const seen=(await cf(`/accounts/${ACCOUNT}/access/users/${user.id}/last_seen_identity`,ENTRIES.ACCESS_READ)).result;
 // The JWT sub is this user_uuid. It is per email, not per IdP, and the
 // last-seen identity is account-wide: any app's login (OTP included) updates it.
 if(seen.user_uuid!==user.id)throw Error('last-seen identity user_uuid differs from the Access user id');
 log('owner.identity','PASS',`${OWNER_EMAIL} has Access user_uuid fp:${fp(user.id)} (JWT sub), created ${user.created_at}; account-wide last login ${user.last_successful_login} via ${seen.idp?.id===idp.id?`"${OWNER_IDP}"`:seen.idp?.type}`,{user_uuid_fp:fp(user.id)});
});
await check('owner.subject',async()=>{
 const since=new Date(Date.now()-30*86400e3).toISOString().slice(0,19)+'Z';
 const logs=(await cf(`/accounts/${ACCOUNT}/access/logs/access_requests?limit=1000&since=${since}&email=${encodeURIComponent(OWNER_EMAIL)}`,ENTRIES.ACCESS_READ)).result.filter(l=>l.app_uid===app.uid||l.app_uid===app.id);
 const last=logs.sort((a,b)=>b.created_at.localeCompare(a.created_at))[0];
 // OWNER_SUBJECT is a secret and cannot be read back. Config alone cannot
 // prove equality; only an owner page rendered for this identity can.
 log('owner.subject','UNPROVEN',`OWNER_SUBJECT is unreadable; admission of fp:${fp(user.id)} is proven only by owner-signin.mjs reaching an owner page. Owner's last Access login to this app: ${last?`${last.created_at} via ${last.connection}, allowed=${last.allowed}`:'none in 30 days'}`);
});
await check('ingress.anonymous',async()=>{
 const r=await fetch(ORIGIN+'/create',{redirect:'manual'});const loc=r.headers.get('location')||'';
 const u=loc?new URL(loc):null;
 if(r.status!==302||u?.host!==vars.ACCESS_TEAM||u.pathname!==`/cdn-cgi/access/login/${host}`||u.searchParams.get('kid')!==vars.ACCESS_AUDIENCE)throw Error(`anonymous GET /create → ${r.status} ${u?.origin||''}${u?.pathname||''}`);
 log('ingress.anonymous','PASS',`anonymous GET /create → 302 ${u.origin}${u.pathname} (kid = ACCESS_AUDIENCE)`);
});
await check('ingress.health',async()=>{
 const r=await fetch(ORIGIN+'/healthz',{headers:{'CF-Access-Client-Id':secret(ENTRIES.MACHINE_ID),'CF-Access-Client-Secret':secret(ENTRIES.MACHINE_SECRET)},redirect:'manual'});
 const body=(await r.text()).trim().slice(0,40);
 if(r.status!==200)throw Error(`machine GET /healthz → ${r.status} ${body}`);
 log('ingress.health','PASS',`machine GET /healthz → 200 ${body}`);
});
console.log(`evidence: ${dir}`);
process.exit(done());
