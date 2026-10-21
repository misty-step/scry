#!/usr/bin/env node
// Read-only: is this Scry worth driving, and can the QA account sign in?
//   doctor.mjs prod            deployed Access, identity and QA configuration
//   doctor.mjs local <url>     a run-owned loopback Worker (bun run dev)
// Exit 0 all PASS, 1 any FAIL, 2 usage, 3 only UNPROVEN left.
import {createHash} from 'node:crypto';
import {ACCOUNT,ACCESS_TEAM,ORIGIN,QA_EMAIL,OWNER_APP,OWNER_IDP,AUTH_ORIGIN,WORKER,ENTRIES,cf,secret,qaCredentials,mail,qaPrerequisites,outDir,reporter} from './lib.mjs';

const [mode='prod',target]=process.argv.slice(2);
if(!['prod','local'].includes(mode)){console.error('usage: doctor.mjs prod | doctor.mjs local <url>');process.exit(2);}
const dir=outDir(`doctor-${mode}`);
const {log,done}=reporter(dir);
const check=async(id,fn)=>{try{await fn();}catch(e){log(id,'FAIL',e.message);}};
const fp=v=>createHash('sha256').update(v).digest('hex').slice(0,12);

if(mode==='local'){
 const url=new URL(target||'http://127.0.0.1:8787');
 // Never send a request to a host that is not loopback.
 if(!['127.0.0.1','localhost','[::1]'].includes(url.hostname)){
  log('local.loopback','FAIL',`${url.host} is not loopback; use doctor.mjs prod for production`);
  process.exit(done());
 }
 log('local.loopback','PASS',url.origin);
 await check('local.page',async()=>{
  const r=await fetch(url.origin+'/',{redirect:'manual'});const html=await r.text();
  const title=/<title>([^<]*)<\/title>/.exec(html)?.[1];
  if(r.status!==200||!title?.endsWith('· Scry'))throw Error(`GET / → ${r.status} ${title||'no title'}`);
  log('local.page','PASS',`GET / → 200 "${title}"`);
 });
 await check('local.changelog',async()=>{
  const r=await fetch(url.origin+'/changelog');const version=/Version [0-9.]+/.exec(await r.text())?.[0];
  if(r.status!==200||!version)throw Error(`GET /changelog → ${r.status} ${version||'no version rendered'}`);
  log('local.changelog','PASS',version);
 });
 process.exit(done());
}

const host=new URL(ORIGIN).host;
let vars={},app,apps,idp,ownerEmails=[],owners=[],lastSecretWrite;
await check('worker.config',async()=>{
 const s=(await cf(`/accounts/${ACCOUNT}/workers/scripts/${WORKER}/settings`,ENTRIES.WORKER_READ)).result;
 for(const b of s.bindings)vars[b.name]=b.type==='plain_text'?b.text:`<${b.type}>`;
 const want={SCRY_ENV:'production',CANONICAL_ORIGIN:ORIGIN,ACCESS_TEAM,OWNER_SUBJECT:'<secret_text>'};
 const bad=Object.entries(want).filter(([k,v])=>vars[k]!==v).map(([k])=>k);
 if(!vars.ACCESS_AUDIENCE)bad.push('ACCESS_AUDIENCE');
 if(bad.length)throw Error(`unexpected ${bad.join(', ')}`);
 log('worker.config','PASS',`SCRY_ENV=production, CANONICAL_ORIGIN=${ORIGIN}, ACCESS_TEAM=${vars.ACCESS_TEAM}, OWNER_SUBJECT bound (value unreadable)`);
});
await check('worker.version',async()=>{
 const d=(await cf(`/accounts/${ACCOUNT}/workers/scripts/${WORKER}/deployments`,ENTRIES.WORKER_READ)).result.deployments;
 const code=d.find(x=>x.annotations?.['workers/triggered_by']==='deployment');
 lastSecretWrite=d.find(x=>x.annotations?.['workers/triggered_by']==='secret')?.created_on;
 log('worker.version','PASS',`serving deployment ${d[0].id} at ${d[0].created_on} (${d[0].annotations?.['workers/triggered_by']}); code ${code?.annotations?.['workers/message']||'unknown'}; last secret write ${lastSecretWrite||'none listed'}`);
});
await check('access.team',async()=>{
 const org=(await cf(`/accounts/${ACCOUNT}/access/organizations`,ENTRIES.ACCESS_READ)).result;
 if(org.auth_domain!==vars.ACCESS_TEAM)throw Error(`Access team ${org.auth_domain} ≠ ACCESS_TEAM ${vars.ACCESS_TEAM}`);
 log('access.team','PASS',`Access team ${org.auth_domain} = ACCESS_TEAM`);
});
await check('access.app',async()=>{
 apps=(await cf(`/accounts/${ACCOUNT}/access/apps?per_page=200`,ENTRIES.ACCESS_READ)).result;
 app=apps.find(a=>a.name===OWNER_APP);
 if(!app)throw Error(`no Access app named "${OWNER_APP}"`);
 if(app.aud!==vars.ACCESS_AUDIENCE)throw Error(`app AUD ${app.aud.slice(0,12)}… ≠ ACCESS_AUDIENCE ${String(vars.ACCESS_AUDIENCE).slice(0,12)}…`);
 if(!app.self_hosted_domains.includes(host))throw Error(`${host} not covered by "${OWNER_APP}"`);
 log('access.app','PASS',`"${OWNER_APP}" AUD = ACCESS_AUDIENCE; covers ${app.self_hosted_domains.join(', ')}; session ${app.session_duration}`);
});
await check('access.idp',async()=>{
 const idps=(await cf(`/accounts/${ACCOUNT}/access/identity_providers`,ENTRIES.ACCESS_READ)).result;
 idp=idps.find(i=>i.name===OWNER_IDP);
 const allowed=(app.allowed_idps??[]).map(id=>idps.find(i=>i.id===id)?.name||id);
 // Password-only: exactly this OIDC provider, so no PIN or other IdP can sign in.
 if(!idp||app.allowed_idps?.length!==1||app.allowed_idps[0]!==idp.id||idp.type!=='oidc'||!idp.config?.auth_url?.startsWith(AUTH_ORIGIN+'/'))throw Error(`"${OWNER_APP}" allows ${allowed.join(', ')||'every configured IdP'}; want only "${OWNER_IDP}" (oidc at ${AUTH_ORIGIN})`);
 log('access.idp','PASS',`password-only: the one allowed IdP is "${OWNER_IDP}" (oidc at ${new URL(idp.config.auth_url).host}); auto-redirect ${app.auto_redirect_to_identity}`);
});
await check('access.policies',async()=>{
 const emails=p=>p.include.map(r=>r.email?.email?.toLowerCase());
 const allow=app.policies.filter(p=>p.decision==='allow');
 const qa=allow.filter(p=>emails(p).includes(QA_EMAIL));
 // The owner policy is the other allow policy. Its addresses are checked, never printed.
 const owner=allow.filter(p=>!emails(p).includes(QA_EMAIL));
 const exact=e=>/^[^\s@*]+@[^\s@*]+\.[^\s@*]+$/.test(e??'');
 const problems=[];
 // Include rules are ORed: any selector other than an exact email could admit someone else.
 for(const p of allow)if(p.include.some(r=>Object.keys(r).join()!=='email'||!exact(r.email?.email)))problems.push(`"${p.name}" includes a non-email, wildcard or domain selector`);
 for(const p of app.policies.filter(p=>p.decision==='non_identity'))if(p.include.some(r=>Object.keys(r).join()!=='service_token'))problems.push(`"${p.name}" admits more than service tokens`);
 if(owner.length!==1||!owner[0].include.length||emails(owner[0]).some(e=>e?.endsWith('@agentmail.to')))problems.push(`owner policy must be one exact-email policy without QA addresses (found ${owner.length})`);
 if(qa.length!==1||emails(qa[0]).length!==1)problems.push(`QA policy must be one allow policy whose only include is ${QA_EMAIL} (found ${qa.map(p=>`"${p.name}" with ${p.include.length} includes`).join(', ')||'none'})`);
 if(problems.length)throw Error(problems.join('; '));
 ownerEmails=emails(owner[0]);
 log('access.policies','PASS',`owner policy "${owner[0].name}": ${ownerEmails.length} exact emails, no wildcard, domain, everyone or QA address; QA policy "${qa[0].name}": exactly ${QA_EMAIL}; non-identity: ${app.policies.filter(p=>p.decision==='non_identity').map(p=>`"${p.name}" (service tokens)`).join(', ')||'none'}`);
});
await check('access.bypass',async()=>{
 const hosts=app.self_hosted_domains;
 const domains=a=>[a.domain,...(a.self_hosted_domains??[])].filter(Boolean);
 const covering=apps.filter(a=>domains(a).some(d=>hosts.some(h=>d===h||d.startsWith(h+'/'))));
 const bad=covering.flatMap(a=>a.policies.filter(p=>!['allow','non_identity'].includes(p.decision)||[...p.include,...(p.require??[])].some(r=>'everyone' in r)).map(p=>`"${a.name}"/"${p.name}" (${p.decision})`));
 if(bad.length)throw Error(`bypass or everyone rules on Scry hosts: ${bad.join(', ')}`);
 log('access.bypass','PASS',`${covering.length} Access apps cover Scry hosts (${covering.map(a=>`"${a.name}"`).join(', ')}); every policy is allow or non_identity, none bypass or everyone`);
});
await check('owner.identity',async()=>{
 owners=(await cf(`/accounts/${ACCOUNT}/access/users?per_page=100`,ENTRIES.ACCESS_READ)).result.filter(u=>ownerEmails.includes(u.email.toLowerCase())).sort((a,b)=>a.created_at.localeCompare(b.created_at));
 if(!owners.length)throw Error('no Access user for any owner-policy email; the owner has never signed in through Access');
 log('owner.identity','PASS',`${owners.length} Access users for owner-policy emails: ${owners.map(u=>`fp:${fp(u.id)} created ${u.created_at}`).join(', ')} (user_uuid is the JWT sub)`,{user_uuid_fps:owners.map(u=>fp(u.id))});
});
await check('owner.subject-age',async()=>{
 // OWNER_SUBJECT is unreadable. The account audit log names the secret on
 // each put, so its newest put can be dated. A value put before every
 // owner-policy Access user existed cannot be any of their uuids.
 const first=owners[0].created_at,last=owners.at(-1).created_at;
 const puts=[];let cursor;
 for(let page=0;page<20;page++){
  const q=new URLSearchParams({since:new Date(Date.parse(first)-90*86400e3).toISOString(),before:new Date().toISOString(),limit:'1000',resource_type:'scripts.secrets',resource_id:WORKER,...(cursor?{cursor}:{})});
  const body=await cf(`/accounts/${ACCOUNT}/logs/audit?${q}`,ENTRIES.ACCESS_READ);
  puts.push(...body.result.filter(e=>e.resource?.id===WORKER&&e.resource?.request?.name==='OWNER_SUBJECT'&&e.raw?.method==='PUT'&&e.action?.result==='success').map(e=>e.action.time));
  cursor=body.result_info?.cursor;if(!cursor||!body.result.length)break;
 }
 const put=puts.sort((a,b)=>Date.parse(a)-Date.parse(b)).at(-1);
 if(!put)return log('owner.subject-age','UNPROVEN',`no OWNER_SUBJECT put on ${WORKER} in the audit log since 90 days before the first owner Access user; its write time is unknown`);
 if(Date.parse(put)<Date.parse(first))throw Error(`newest OWNER_SUBJECT put ${put} predates every owner-policy Access user (first created ${first}): it cannot be any of their uuids`);
 if(Date.parse(put)<Date.parse(last))return log('owner.subject-age','UNPROVEN',`newest OWNER_SUBJECT put ${put} falls between the owner-policy Access users' creation (${first}, ${last}); which one Access issues for the owner is not readable here`);
 log('owner.subject-age','PASS',`newest OWNER_SUBJECT put ${put} (audit log) follows every owner-policy Access user's creation (latest ${last}); this rules out only a subject written before those users existed, and equality stays unreadable`);
});
await check('qa.credentials',async()=>{
 const qa=qaCredentials({optional:true});
 if(!qa)return log('qa.credentials','UNPROVEN',`QA sign-in UNPROVEN: needs ${ENTRIES.QA_EMAIL}, _INBOX, _AGENTMAIL_KEY and _PASSWORD`);
 const box=await mail(qa,'?limit=1');
 if(!Array.isArray(box.messages))throw Error('AgentMail QA inbox answered without a message list');
 log('qa.credentials','PASS',`QA pass entries readable; address and inbox are ${QA_EMAIL}; AgentMail inbox readable (no message content read)`);
});
await check('qa.prerequisites',async()=>{
 const steps=await qaPrerequisites();
 for(const s of steps)log(`qa.${s.step.replace(/[^a-z]+/gi,'-').replace(/^-|-$/g,'').toLowerCase()}`,s.ok===true?'PASS':s.ok===false?'FAIL':'UNPROVEN',s.observed);
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
process.exit(done());
