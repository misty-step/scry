#!/usr/bin/env node
/**
 * scripts/critics/run.mjs
 * Entry point: plain node, no new npm deps.
 */

import { spawnSync, spawn } from 'node:child_process';
import { writeFile, readFile, mkdir, open, lstat, readdir } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { join, dirname, isAbsolute } from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { connect } from 'node:net';
import os from 'node:os';
import {randomBytes} from 'node:crypto';
import {artifactIdentity,snapshotSource,verifyWorkerIdentity} from './lib/worker-artifact.mjs';
import {sourceProvenance} from './lib/source-provenance.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const REPO_ROOT = join(__dirname, '..', '..');

// Supported walk goals. An unknown goal is a usage error, never a silent
// practice-review run.
const GOALS = ['practice-review'];

// === utilities ===
function jsonOutput(data) {
  process.stdout.write(JSON.stringify(data, null, 2) + '\n');
}

function sha256File(path) {
  const { readFileSync } = createRequire(import.meta.url)('fs');
  const hash = createHash('sha256');
  hash.update(readFileSync(path));
  return hash.digest('hex');
}

function getGitRevision() {
  try { return sourceProvenance(REPO_ROOT).revision; }
  catch (_) { return null; }
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function fail(message, code = 2) {
  process.stderr.write(message + '\n');
  process.exit(code);
}

function positiveInt(value, flag) {
  if (value === undefined) return null;
  const n = Number(value);
  if (!Number.isInteger(n) || n < 1) fail(`usage: ${flag} must be a positive integer, got "${value}"`);
  return n;
}

// === candidate commands ===
function isPortBusy(port) {
  return new Promise((resolve) => {
    let settled = false;
    let timer = null;
    const finish = (busy) => {
      if (settled) return;
      settled = true;
      if (timer) clearTimeout(timer);
      socket.destroy();
      resolve(busy);
    };
    const socket = connect({ host: '127.0.0.1', port });
    socket.once('connect', () => finish(true));
    socket.once('error', () => finish(false));
    timer = setTimeout(() => finish(false), 800);
  });
}

async function candidateUp(args) {
  const {parseArgs}=await import('node:util');
  let values;
  try {
    ({values}=parseArgs({args,options:{
      dir:{type:'string'},port:{type:'string'},out:{type:'string'},
      binary:{type:'string'},json:{type:'boolean'}
    }}));
  } catch(error) {fail('usage: candidate up: '+error.message);}
  const {resolve}=await import('node:path');
  const dir=resolve(values.dir||join(REPO_ROOT,'target/critics/candidate'));
  const port=values.port===undefined?18080:Number(values.port);
  if(!Number.isInteger(port)||port<1||port>65535)fail('usage: --port must be a port number 1-65535');
  const outPath=resolve(values.out||join(dir,'candidate.json'));
  if(existsSync(outPath))fail('Candidate handle exists. Use fresh dir: '+dir);
  let state='missing';
  try {
    const info=await lstat(dir);
    state=info.isDirectory()&&!info.isSymbolicLink()?(await readdir(dir)).length?'non-empty':'empty':'not-a-directory';
  } catch(error) {if(error.code!=='ENOENT')fail('Candidate dir is not fresh: '+error.message);}
  if(state!=='missing'&&state!=='empty')fail('Candidate dir is not fresh: '+dir+'; refusing to delete existing content. Use a fresh dir');
  if(await isPortBusy(port))fail('Port '+port+' is already in use; refusing to start a candidate that cannot bind');
  if(values.binary)fail('--binary is a historical Go option and is unsupported for the Rust Worker. Build a fresh source-bound candidate.');
  const provenance=sourceProvenance(REPO_ROOT);
  const revision=provenance.revision;
  if(!/^([0-9a-f]{40}|[0-9a-f]{64})$/.test(revision||''))fail('Git revision unavailable; refusing unverifiable candidate provenance');
  const sourceState=provenance.source_state;
  await mkdir(dir,{recursive:true});
  const sourceDirectory=join(dir,'source');
  const source=await snapshotSource(REPO_ROOT,sourceDirectory);
  // Only tool-cache capabilities enter this build. The compiled source is a
  // frozen copy, so another collaborator's changes cannot relabel this build.
  const buildEnv={PATH:process.env.PATH,HOME:os.homedir(),LANG:'C.UTF-8'};
  for(const key of ['CARGO_HOME','RUSTUP_HOME'])if(process.env[key])buildEnv[key]=process.env[key];
  buildEnv.CARGO_TARGET_DIR=join(REPO_ROOT,'target/critic-rust-build');
  const built=spawnSync('worker-build',['--release','--locked'],{
    cwd:sourceDirectory,env:buildEnv,encoding:'utf8',timeout:600000,maxBuffer:16*1024*1024
  });
  await writeFile(join(dir,'build.log'),(built.stdout||'')+(built.stderr||''));
  if(built.status!==0)fail('Rust Worker build failed (see '+join(dir,'build.log')+'): '+(built.error?.message||built.stderr||'unknown error'));
  const artifact={directory:join(sourceDirectory,'build'),...await artifactIdentity(join(sourceDirectory,'build'))};
  const url='http://127.0.0.1:'+port;
  const id='cand-'+createHash('sha256').update(revision+'|'+source.sha256+'|'+url+'|'+artifact.sha256).digest('hex').slice(0,12);
  const attestation=randomBytes(24).toString('hex');
  const runner=join(__dirname,'worker-server.mjs');
  await writeFile(join(dir,'launch.json'),JSON.stringify({id,attestation,artifact,source_sha256:source.sha256})+'\n');
  const log=await open(join(dir,'serve.log'),'w');
  // No inherited app/provider/cloud env and no dotenv loading. The helper
  // supplies only synthetic bindings and denies every outbound service call.
  const runtimeEnv={PATH:process.env.PATH,HOME:dir,LANG:'C.UTF-8',WRANGLER_SEND_METRICS:'false',CLOUDFLARE_LOAD_DEV_VARS_FROM_DOT_ENV:'false'};
  for(const key of ['SCRY_WRANGLER_PACKAGE'])if(process.env[key])runtimeEnv[key]=process.env[key];
  const child=spawn(process.execPath,[runner,'--directory',dir,'--port',String(port)],{
    cwd:artifact.directory,env:runtimeEnv,detached:true,stdio:['ignore',log.fd,log.fd]
  });
  child.unref();await log.close();
  const handle={
    format:'scry-critic-worker-candidate-v2',id,pid:child.pid,url,dir,port,
    binary:runner,binary_sha256:artifact.sha256,runner_sha256:sha256File(runner),
    revision,binary_revision:revision,binary_revision_source:'rust-source-snapshot',
    source_state:sourceState,source_state_source:provenance.state_source,
    gate_source_sha256:provenance.gate_source_sha256,
    source_sha256:source.sha256,source_inventory:join(sourceDirectory,'critic-source.json'),
    artifact,attestation,kind:'isolated-synthetic',
    seed:{model:'authored-test-fixture',source:'src/engine.rs::seed_fixture'},
    storage:'disposable workerd SQLite/R2; no restart or hosted recovery claim',
    started_at:new Date().toISOString()
  };
  const {readyAttempt}=await import('./lib/ready.mjs');
  let ready=false;
  for(let attempt=0;attempt<30;attempt++) {
    if(child.exitCode!==null)break;
    try {
      const marker=JSON.parse(await readFile(join(dir,'ready.json'),'utf8'));
      if(marker.pid!==child.pid||marker.token!==attestation||marker.artifact_sha256!==artifact.sha256)throw Error('Ready marker does not identify this child');
      await readyAttempt(url,{timeoutMs:2000});
      await verifyWorkerAttestation(handle);
      ready=true;break;
    } catch {await sleep(500);}
  }
  if(!ready) {
    try{process.kill(-child.pid,'SIGTERM');}catch{}
    await sleep(300);
    try{process.kill(-child.pid,'SIGKILL');}catch{}
    const detail=(await readFile(join(dir,'serve.log'),'utf8').catch(()=>'' )).slice(-4000);
    fail('Worker exited or did not become ready (see '+join(dir,'serve.log')+')'+(detail?'\n'+detail:''));
  }
  await mkdir(dirname(outPath),{recursive:true});
  await writeFile(outPath,JSON.stringify(handle,null,2)+'\n',{flag:'wx'});
  if(values.json)jsonOutput(handle);else process.stdout.write('Synthetic Rust Worker ready: '+url+'\n');
}

async function verifyWorkerAttestation(handle) {
  const nonce=randomBytes(16).toString('hex');
  const response=await fetch(handle.url+'/__critic/identity?nonce='+nonce,{signal:AbortSignal.timeout(2000),redirect:'error'});
  if(!response.ok)throw Error('Serving Worker attestation unavailable');
  const actual=await response.json();
  if(actual.id!==handle.id||actual.pid!==handle.pid||actual.token!==handle.attestation||
     actual.artifact_sha256!==handle.binary_sha256||actual.source_sha256!==handle.source_sha256||actual.nonce!==nonce)
    throw Error('Serving Worker does not match the source/artifact handle');
}

async function candidateDown(args) {
  const { parseArgs } = await import('node:util');
  let values;
  try {
    ({ values } = parseArgs({
      args,
      options: { dir: { type: 'string' }, out: { type: 'string' }, json: { type: 'boolean' } }
    }));
  } catch (err) {
    fail('usage: candidate down: ' + err.message);
  }

  const dir = values.dir || join(REPO_ROOT, 'target/critics/candidate');
  const outPath = values.out || join(dir, 'candidate.json');

  let handle;
  try { handle = JSON.parse(await readFile(outPath, 'utf8')); }
  catch (_) { process.stderr.write('Handle not found: ' + outPath + '\n'); process.exit(2); }

  const ps = spawnSync('ps', ['-o', 'args=', '-p', handle.pid], { encoding: 'utf8' });
  if (ps.status !== 0 || !ps.stdout.includes(handle.binary) ||
      (handle.format === 'scry-critic-worker-candidate-v2' && !ps.stdout.includes(handle.dir))) {
    process.stderr.write('PID does not match our binary; refusing to kill\n');
    process.exit(2);
  }

  try {
    process.kill(handle.format === 'scry-critic-worker-candidate-v2' ? -handle.pid : handle.pid, 'SIGTERM');
    await sleep(2000);
    if (spawnSync('ps', ['-p', handle.pid], { encoding: 'utf8' }).status === 0) {
      process.kill(handle.format === 'scry-critic-worker-candidate-v2' ? -handle.pid : handle.pid, 'SIGKILL');
    }
  } catch (err) {
    process.stderr.write('Failed to kill process ' + handle.pid + ': ' + err.message + '\n');
  }

  handle.stopped_at = new Date().toISOString();
  await writeFile(outPath, JSON.stringify(handle, null, 2));

  if (values.json) jsonOutput(handle);
  else process.stdout.write('Candidate stopped\n');
}

// === human walk ===

// Verify that a loopback candidate handle still identifies the process that
// serves the walk target. A handle outlives the candidate: without this check
// a stale handle (candidate stopped, port re-occupied by another server) or a
// rebuilt binary behind the same path binds a receipt to an artifact that was
// not served. Mirrors the `candidate down` guard (pid alive + command line
// references the binary) and additionally checks the binary digest. Relative
// binary paths resolve against the repo root. Fails closed on any mismatch.
async function verifyServedArtifact(handle) {
  const { classifyOrigin } = await import('./lib/guards.mjs');
  const classified = classifyOrigin(String(handle.url ?? ''));
  if (!classified.ok || classified.kind !== 'loopback') return handle.format === 'scry-critic-worker-candidate-v2'
    ? {ok:false,checked:false,reason:'Worker v2 handles require their run-owned loopback origin'}
    : {ok:true,checked:false};

  const pid = Number(handle.pid);
  if (!Number.isInteger(pid) || pid < 1) {
    return { ok: false, checked: true, reason: 'handle pid is missing or invalid; cannot verify the serving process' };
  }

  const ps = spawnSync('ps', ['-o', 'args=', '-p', String(pid)], { encoding: 'utf8' });
  if (ps.status !== 0 || !ps.stdout.trim()) {
    return { ok: false, checked: true, reason: `serving process ${pid} is not running; the candidate may have stopped and another artifact may hold the port` };
  }

  const binary = typeof handle.binary === 'string' && handle.binary.length > 0 ? handle.binary : null;
  if (!binary) {
    return { ok: false, checked: true, reason: 'handle binary path is missing; cannot verify the serving process' };
  }
  const binaryPath = isAbsolute(binary) ? binary : join(REPO_ROOT, binary);
  if (!ps.stdout.includes(binary) && !ps.stdout.includes(binaryPath)) {
    return { ok: false, checked: true, reason: `process ${pid} does not run the handle binary ${binaryPath}; refusing to bind to another artifact` };
  }

  if (handle.format === 'scry-critic-worker-candidate-v2') {
    try {
      if (!ps.stdout.includes(handle.dir)) throw Error('Serving process is not the run-owned Worker');
      if (sha256File(binaryPath) !== handle.runner_sha256) throw Error('Synthetic transport runner changed since startup');
      await verifyWorkerIdentity(handle);
      await verifyWorkerAttestation(handle);
      if (handle.binary_revision !== handle.revision) throw Error('Worker source revision does not match its claimed revision');
      return {ok:true,checked:true};
    } catch(error) {return {ok:false,checked:true,reason:error.message};}
  }
  // v1 is retained only for isolated historical binding/negative batteries.
  let actual;
  try { actual = sha256File(binaryPath); }
  catch (err) { return { ok: false, checked: true, reason: `handle binary ${binaryPath} is unreadable: ${err.message}` }; }
  const declared = String(handle.binary_sha256).toLowerCase();
  if (actual !== declared) {
    return { ok: false, checked: true, reason: `handle binary ${binaryPath} changed since candidate up (sha256 ${actual.slice(0, 12)}…, handle declares ${declared.slice(0, 12)}…)` };
  }

  // Build provenance: the handle must record the served binary's own revision
  // (`binary_revision`) and it must equal the revision the receipt would
  // claim. A --binary candidate whose provenance could not be determined
  // records binary_revision: null and cannot bind a revision-claiming
  // receipt; the checkout revision is never substituted for it.
  const binaryRevision = typeof handle.binary_revision === 'string' ? handle.binary_revision.toLowerCase() : null;
  if (!/^([0-9a-f]{40}|[0-9a-f]{64})$/.test(binaryRevision ?? '')) {
    const declared = handle.binary_revision === undefined ? 'missing' : JSON.stringify(handle.binary_revision);
    return { ok: false, checked: true, reason: `handle records no determinable binary revision (binary_revision: ${declared}); the served artifact's provenance cannot be verified` };
  }
  const claimedRevision = String(handle.revision ?? '').toLowerCase();
  if (binaryRevision !== claimedRevision) {
    return { ok: false, checked: true, reason: `binary revision ${binaryRevision} does not match handle revision ${handle.revision}; refusing to bind a revision the binary was not built from` };
  }

  return { ok: true, checked: true };
}

// Load and validate a candidate handle for receipt binding. Fails closed:
// unreadable handle, missing identity fields, an artifact that is not the
// walk target, a revision that is not the walking checkout, or (loopback) a
// serving process that no longer matches the handle all reject.
async function loadCandidateHandle(handlePath, targetUrl) {
  const block = {
    bound: false, handle: handlePath, handle_id: null, revision: null,
    binary_revision: null, binary_sha256: null, source_state: null, seed: 'declared-unverified'
  };

  let raw;
  try { raw = JSON.parse(await readFile(handlePath, 'utf8')); }
  catch (err) { return { ok: false, reason: 'handle unreadable: ' + err.message, identityBlock: block }; }

  if (!raw || typeof raw !== 'object') return { ok: false, reason: 'handle is not an object', identityBlock: block };

  if (typeof raw.id === 'string') block.handle_id = raw.id;
  if (typeof raw.revision === 'string') block.revision = raw.revision;
  // A receipt must never carry a contradictory identity pair: when the
  // handle's own `revision` and `binary_revision` disagree, the binding is
  // rejected below and the receipt records the mismatch in `observed` with
  // `binary_revision` left null.
  if (typeof raw.binary_revision === 'string' &&
      String(raw.revision ?? '').toLowerCase() === raw.binary_revision.toLowerCase()) {
    block.binary_revision = raw.binary_revision;
  }
  if (typeof raw.binary_sha256 === 'string') block.binary_sha256 = raw.binary_sha256;
  if (typeof raw.source_state === 'string') block.source_state = raw.source_state;
  if (raw.format === 'scry-critic-worker-candidate-v2') {
    block.runtime = 'Rust Worker/workerd'; block.source_sha256 = raw.source_sha256;
    block.artifact = raw.artifact; block.identity_format = raw.format;
    block.gate_source_sha256 = raw.gate_source_sha256 || null;
  }
  if (raw.seed && typeof raw.seed === 'object' && typeof raw.seed.model === 'string') block.seed = raw.seed.model;

  if (!['scry-critic-candidate-v1','scry-critic-worker-candidate-v2'].includes(raw.format)) return { ok: false, reason: 'handle format mismatch: ' + raw.format, identityBlock: block };
  if (!/^([0-9a-f]{40}|[0-9a-f]{64})$/i.test(String(raw.revision ?? ''))) return { ok: false, reason: 'handle revision missing or invalid', identityBlock: block };
  if (!/^[0-9a-f]{64}$/i.test(String(raw.binary_sha256 ?? ''))) return { ok: false, reason: 'handle binary_sha256 missing or invalid', identityBlock: block };

  // A valid identity exists on the handle from here on.
  block.bound = true;

  const target = targetUrl || 'http://127.0.0.1:18080';
  let tOrigin = null, hOrigin = null;
  try { tOrigin = new URL(target).origin; } catch (_) {}
  try { hOrigin = new URL(String(raw.url ?? '')).origin; } catch (_) {}
  if (!tOrigin || !hOrigin || tOrigin !== hOrigin) {
    return { ok: false, reason: `handle describes ${hOrigin || 'an unknown origin'} but the walk target is ${tOrigin || 'unknown'}`, identityBlock: block };
  }

  // Served-artifact verification (loopback handles): the handle must still
  // identify the running process/artifact, not only the revision. Runs before
  // the checkout-revision comparison — it depends on neither the checkout nor
  // the browser, so the boundary is enforced wherever the walk runs (host or
  // bare CI tree without .git).
  const served = await verifyServedArtifact(raw);
  if (!served.ok) {
    return { ok: false, reason: served.reason, identityBlock: block };
  }

  const checkout = getGitRevision();
  if (!checkout) return { ok: false, reason: 'checkout revision unavailable (not a git checkout); cannot verify the binding', identityBlock: block };
  if (checkout !== raw.revision) {
    return { ok: false, reason: `handle revision ${raw.revision} does not match checkout HEAD ${checkout}`, identityBlock: block };
  }

  return { ok: true, reason: null, identityBlock: block };
}

async function humanWalk(args) {
  const { parseArgs } = await import('node:util');
  let values;
  try {
    ({ values } = parseArgs({
      args,
      options: {
        goal: { type: 'string' },
        candidate: { type: 'string' },
        out: { type: 'string' },
        timeout: { type: 'string' },
        'max-steps': { type: 'string' },
        'max-screenshots': { type: 'string' },
        'allow-origin': { type: 'string' },
        handle: { type: 'string' },
        json: { type: 'boolean' }
      }
    }));
  } catch (err) {
    fail('usage: ' + err.message);
  }

  if (!values.goal) fail('usage: --goal is required; supported goals: ' + GOALS.join(', '));
  if (!GOALS.includes(values.goal)) fail(`usage: unknown goal "${values.goal}"; supported goals: ` + GOALS.join(', '));
  const maxSteps = positiveInt(values['max-steps'], '--max-steps') ?? 24;
  const timeoutS = positiveInt(values.timeout, '--timeout') ?? 120;
  const maxScreenshots = positiveInt(values['max-screenshots'], '--max-screenshots') ?? 12;

  const outDir = values.out || join(REPO_ROOT, 'target/critics/run1');
  const url = values.candidate;
  const startedAt = Date.now();

  await mkdir(outDir, { recursive: true });

  const { validateReceipt } = await import('./lib/receipt.mjs');
  const { classifyOrigin, Budget, BudgetError } = await import('./lib/guards.mjs');
  const { resolveBrowser } = await import('./lib/browser.mjs');
  const { discoverControls, parseAriaSnapshot } = await import('./lib/discover.mjs');
  const { fingerprint, dedupeFindings } = await import('./lib/dedupe.mjs');

  const budget = new Budget({ maxSteps, timeoutS, maxScreenshots });

  // Origin safety: mutating walks pass only for loopback or an explicitly
  // allowed non-loopback origin; production origins are always refused.
  if (url) {
    const result = classifyOrigin(url, { mutating: true, allowOrigin: values['allow-origin'] || null });
    if (!result.ok) {
      process.stderr.write('Blocked: ' + result.reason + '\n');
      process.exit(2);
    }
  }

  // Candidate binding (D1): receipts carry the identity of the tested artifact
  // only from its handle. A mismatch is a blocked walk with a receipt.
  let binding = {
    bound: false, handle: null, handle_id: null, revision: null,
    binary_revision: null, binary_sha256: null, source_state: null, seed: 'declared-unverified'
  };
  if (values.handle) {
    const loaded = await loadCandidateHandle(values.handle, url);
    if (!loaded.ok) {
      await writeBlockedReceipt('candidate binding rejected: ' + loaded.reason, loaded.identityBlock);
      fail('Blocked: candidate binding rejected: ' + loaded.reason);
    }
    binding = loaded.identityBlock;
  } else {
    process.stderr.write('note: no --handle passed; receipt candidate identity will be unbound\n');
  }

  const shotsDir = join(outDir, 'shots');
  await mkdir(shotsDir, { recursive: true });

  const checks = [];
  const findings = [];
  const coverage = { exercised: [], skipped: [] };

  // Authored fixture answers (src/engine.rs seed_fixture), plus historical
  // DNS/TLS prompts used by negative batteries. These are fixture-only oracles.
  const FIXTURE_ANSWERS = {
    'what type of address does a dns a record map a hostname to?': 'IPv4 address',
    'what protocol does https use to encrypt http?': 'TLS',
    'which response directive sets how long a cached response may be reused without validation?': 'max-age',
    'a stored response has passed its freshness lifetime. what does that tell you?': 'It may need validation before reuse',
    'what http status tells a browser its cached representation can be reused after a conditional request?': '304',
  };

  function getNetworkType(inputUrl) {
    if (!inputUrl) return 'local-loopback';
    try {
      const parsed = new URL(inputUrl);
      if (parsed.hostname === '127.0.0.1' || parsed.hostname === 'localhost') {
        return 'local-loopback';
      }
      return 'declared-external';
    } catch (_) {
      return 'unknown';
    }
  }

  function candidateBlock() {
    return {
      bound: binding.bound,
      handle: binding.handle,
      handle_id: binding.handle_id,
      revision: binding.revision,
      binary_revision: binding.binary_revision,
      origin: url || 'localhost:18080',
      kind: url ? 'external-isolated-synthetic' : 'isolated-synthetic',
      seed: binding.seed,
      binary_sha256: binding.binary_sha256,
      source_state: binding.source_state,
      ...(binding.artifact ? {runtime:binding.runtime,artifact:binding.artifact,source_sha256:binding.source_sha256,gate_source_sha256:binding.gate_source_sha256,identity_format:binding.identity_format} : {})
    };
  }

  // Helper to scan DOM and build aria-like structure
  async function scanPage() {
    const domList = await p.evaluate(() => {
      const result = [];
      const visible = (el) => {
        const style = window.getComputedStyle(el);
        return style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0';
      };
      const inDetails = (el) => {
        let parent = el.parentElement;
        while (parent) {
          if (parent.tagName === 'DETAILS') return true;
          parent = parent.parentElement;
        }
        return false;
      };
      const traverse = (el, depth) => {
        if (!visible(el)) return;
        const role = el.getAttribute('role') || el.tagName.toLowerCase();
        const name = el.getAttribute('aria-label') || el.innerText?.slice(0, 100) || '';
        const fieldName = el.getAttribute('name') || '';
        const type = el.getAttribute('type') || '';
        const className = el.getAttribute('class') || '';
        const dataset = el.dataset ? Object.fromEntries(Object.entries(el.dataset)) : undefined;
        result.push({ tag: el.tagName.toLowerCase(), role, type, name, fieldName, depth, visible: visible(el), inDetails: inDetails(el), class: className, dataset });
        for (const child of el.children) traverse(child, depth + 1);
      };
      traverse(document.body, 0);
      return result;
    });

    // Real accessibility tree via ARIA snapshot — independent of the DOM scan.
    // Falls back to an empty list (methods-unavailable) rather than a DOM projection.
    let ariaList = [];
    try {
      const ariaYaml = await p.locator('body').ariaSnapshot();
      ariaList = parseAriaSnapshot(ariaYaml);
    } catch (_) {
      ariaList = [];
    }

    return { domList, ariaList };
  }

  function makeReceipt(discovery, checks, findings, coverage, elapsedS, useragent, networkType, bindingOverride = null) {
    return {
      format: 'scry-critic-receipt-v1',
      no_findings: findings.length === 0,
      run: { id: 'crit-' + Date.now(), started_at: new Date(startedAt).toISOString(), ended_at: new Date().toISOString(), duration_s: elapsedS, budget: budget.toJSON() },
      candidate: (() => {
        if (!bindingOverride) return candidateBlock();
        return {
          bound: bindingOverride.bound,
          handle: bindingOverride.handle,
          handle_id: bindingOverride.handle_id,
          revision: bindingOverride.revision,
          binary_revision: bindingOverride.binary_revision,
          origin: url || 'localhost:18080',
          kind: url ? 'external-isolated-synthetic' : 'isolated-synthetic',
          seed: bindingOverride.seed,
          binary_sha256: bindingOverride.binary_sha256,
          source_state: bindingOverride.source_state,
          ...(bindingOverride.artifact ? {runtime:bindingOverride.runtime,artifact:bindingOverride.artifact,source_sha256:bindingOverride.source_sha256,gate_source_sha256:bindingOverride.gate_source_sha256,identity_format:bindingOverride.identity_format} : {})
        };
      })(),
      environment: { viewport: '390x844', user_agent: useragent, network: networkType, data: 'synthetic-authored' },
      story_bindings: [{ story: 'US-002', criteria: ['US-002.1', 'US-002.2'] }],
      checks,
      findings: dedupeFindings(findings).map(f => ({ ...f, fingerprint: fingerprint({ story: f.story, criterion: f.criterion, mechanism: f.mechanism }) })),
      coverage,
      advisory: { jevs: [], notes: [] },
      limits: []
    };
  }

  async function writeBlockedReceipt(observed, bindingOverride = null) {
    const elapsedS = Math.round((Date.now() - startedAt) / 1000);
    const receipt = makeReceipt(
      { question: { name: '', status: 'unverified' }, answerAffordance: { type: null, controls: [], status: 'unverified' }, state: 'unknown', notes: [] },
      [{ id: 'walk-execution', surface: '/', status: 'unverified', expected: 'complete review walk', observed, evidence: { screenshots: [], notes: ['blocked: ' + observed] } }],
      [],
      { exercised: ['walk-execution'], skipped: [] },
      elapsedS, '', getNetworkType(url), bindingOverride
    );
    const validation = validateReceipt(receipt);
    if (validation.ok) await writeFile(join(outDir, 'receipt.json'), JSON.stringify(receipt, null, 2));
  }

  // Browser availability is part of the environment contract: a missing
  // browser is a blocked run, and blocked runs still write a receipt.
  const browser = await resolveBrowser();
  if (!browser.ok) {
    if (!browser.playwright) {
      process.stderr.write('Playwright unavailable. Use one of:\n');
      process.stderr.write('  NODE_PATH=/path/to/playwright\n');
      process.stderr.write('  SCRY_CRITICS_PLAYWRIGHT_PATH=/path/to/playwright\n');
      process.stderr.write('  or add playwright to project package.json\n');
      await writeBlockedReceipt('playwright unavailable');
    } else {
      process.stderr.write('No Chromium found. Set SCRY_CRITICS_CHROMIUM_PATH or install:\n');
      process.stderr.write('  /usr/bin/chromium\n');
      process.stderr.write('  /usr/bin/google-chrome\n');
      await writeBlockedReceipt('chromium unavailable');
    }
    process.exit(2);
  }

  // Browser launch failure is a blocked environment, not a findings exit: it
  // must preserve a blocked receipt.
  let b = null;
  let p = null;
  try {
    b = await browser.playwright.mod.chromium.launch({ executablePath: browser.chromium, headless: true, args: browser.launchArgs });
    p = await b.newPage({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });
  } catch (err) {
    if (b) { try { await b.close(); } catch (_) {} }
    process.stderr.write('Browser launch failed: ' + err.message + '\n');
    await writeBlockedReceipt('browser launch failed: ' + err.message);
    process.exit(2);
  }

  // Budget enforcement (D3): every step and elapsed time is counted; a limit
  // stop is a blocked walk with a receipt, never a false pass.
  const checkBudget = (stage) => {
    const step = budget.checkStep();
    if (!step.ok) throw new BudgetError(`max steps exceeded (limit ${maxSteps}; stage: ${stage})`);
    const t = budget.checkTimeout();
    if (!t.ok) throw new BudgetError(`timeout exceeded (limit ${timeoutS}s; stage: ${stage})`);
  };

  try {
    checkBudget('initial navigation');
    await p.goto(url || 'http://127.0.0.1:18080/', { waitUntil: 'load', timeout: 15000 });
    await p.waitForTimeout(500);

    // Resume-safe: a persistent candidate can carry a leftover graded state
    // (feedback shown, waiting for Next). Advance via Next, bounded.
    const readState = async () => {
      const s = await scanPage();
      const h = await p.content();
      const obs = { url: p.url(), title: await p.title(), html: h, aria: s.ariaList, dom: s.domList };
      return { obs, discovery: discoverControls(obs) };
    };

    let { obs: observation, discovery } = await readState();
    if (await p.$('[data-state="intro"]')) {
      checkBudget('acknowledge authored introduction');
      const intro = await p.$('form[action="/review/intro"] button');
      if (!intro) throw Error('Authored concept introduction has no locatable acknowledgment');
      await intro.click();
      await p.waitForTimeout(1200);
      ({obs:observation,discovery}=await readState());
    }
    let normalizeSteps = 0;
    while (discovery.state === 'graded' && normalizeSteps < 5) {
      checkBudget('normalize ' + (normalizeSteps + 1));
      const nb = await p.$('form[data-next] button');
      if (!nb) break;
      await nb.click();
      await p.waitForTimeout(1200);
      ({ obs: observation, discovery } = await readState());
      normalizeSteps++;
    }

    const takeShot = async (name) => {
      const c = budget.checkScreenshot();
      if (!c.ok) throw new BudgetError('budget exhausted: ' + c.reason);
      await p.screenshot({ path: join(shotsDir, name + '.png') });
    };

    // Scan page state 1 (answerable)
    const html = observation.html;
    await takeShot('ungraded');
    const useragent = await p.evaluate(() => navigator.userAgent);
    const networkType = getNetworkType(url);

    // Check 1: answerable surface — state-aware classification.
    // A 'missing' finding requires: question confirmed AND no affordance by
    // both methods AND a state that is not graded/empty.
    if (discovery.state !== 'ungraded' || discovery.answerAffordance.status !== 'confirmed') {
      const questionConfirmed = discovery.question.status === 'confirmed';
      const isMissing = discovery.state === 'unknown' && questionConfirmed && discovery.answerAffordance.status === 'missing';
      if (isMissing) {
        findings.push({ 
          id: 'US-002.1', 
          category: 'missing', 
          story: 'US-002', 
          criterion: 'US-002.1', 
          mechanism: 'answer-affordance-absent', 
          title: 'No answer affordance', 
          expected: 'choice buttons or recall+submit', 
          actual: 'none found', 
          impact: 'walk blocked', 
          uncertainty: null, 
          evidence: { screenshots: [{ name: 'ungraded', path: join(shotsDir, 'ungraded.png') }], notes: ['question confirmed; both methods agree no affordance'] }, 
          acceptance: 'needs fix' 
        });
      } else {
        const reason = discovery.state === 'empty'
          ? 'empty state: no question due'
          : (discovery.state === 'graded'
            ? 'graded state persisted after Next attempts'
            : 'discovery status: ' + discovery.answerAffordance.status + ', state: ' + discovery.state);
        checks.push({ 
          id: 'US-002.1', 
          surface: '/', 
          status: 'unverified', 
          expected: 'answerable review question', 
          observed: reason, 
          evidence: { screenshots: [{ name: 'ungraded', path: join(shotsDir, 'ungraded.png') }], notes: ['no product claim without a confirmed postcondition'] } 
        });
        coverage.exercised.push('US-002.1');
      }

      await b.close();
      const elapsedS = Math.round((Date.now() - startedAt) / 1000);
      const receipt = makeReceipt(discovery, checks, findings, coverage, elapsedS, useragent, networkType);
      const validation = validateReceipt(receipt);
      if (validation.ok) await writeFile(join(outDir, 'receipt.json'), JSON.stringify(receipt, null, 2));
      
      if (findings.length > 0) process.exit(1);
      if (checks.some(c => c.status === 'unverified')) process.exit(3);
      process.exit(2);
    }

    checks.push({ 
      id: 'US-002.1', 
      surface: '/', 
      status: 'pass', 
      expected: 'answer affordance', 
      observed: discovery.answerAffordance.type + ' confirmed', 
      evidence: { screenshots: [{ name: 'ungraded', path: join(shotsDir, 'ungraded.png') }], notes: ['discovery confirmed: ' + discovery.answerAffordance.type] } 
    });
    coverage.exercised.push('US-002.1');

    // Answer using the discovered affordance (choice: pick the fixture's
    // correct choice when known, else the first; recall: fill the fixture
    // answer when known, else a placeholder).
    const affordanceType = discovery.answerAffordance.type;
    const fixtureAnswer = FIXTURE_ANSWERS[discovery.question.name];
    let answerAction = null;

    checkBudget('answer');

    if (affordanceType === 'choice') {
      const buttons = await p.$$('form.answer-form button[name="answer"]');
      let target = buttons[0] || null;
      if (target && fixtureAnswer) {
        for (const btn of buttons) {
          if ((await btn.getAttribute('value')) === fixtureAnswer) { target = btn; break; }
        }
      }
      if (target) { await target.click(); answerAction = 'choice'; }
    } else if (affordanceType === 'recall') {
      const ta = await p.$('form.answer-form textarea[name="answer"]');
      if (ta) {
        if (!fixtureAnswer) throw Error('Unsupported authored recall prompt; no answer oracle for this journey');
        await ta.fill(fixtureAnswer);
        await p.click('form.answer-form button:not([type="button"])');
        answerAction = 'recall';
      }
    }

    if (!answerAction) {
      checks.push({ 
        id: 'US-002.1-loc', 
        surface: '/', 
        status: 'unverified', 
        expected: 'answer control present', 
        observed: 'discovered type=' + affordanceType + ' but no locatable control', 
        evidence: { screenshots: [{ name: 'ungraded', path: join(shotsDir, 'ungraded.png') }], notes: ['runner-defect: cannot locate discovered control'] } 
      });
      coverage.exercised.push('US-002.1-loc');
      await b.close();
      const elapsedS = Math.round((Date.now() - startedAt) / 1000);
      const receipt = makeReceipt(discovery, checks, findings, coverage, elapsedS, useragent, networkType);
      const validation = validateReceipt(receipt);
      if (validation.ok) await writeFile(join(outDir, 'receipt.json'), JSON.stringify(receipt, null, 2));
      process.exit(3);
    }

    await p.waitForTimeout(1200);

    // Recall fallback: an unclear match leaves the surface ungraded ("Not
    // graded"); use the reveal path to reach a graded (helped) state.
    if (affordanceType === 'recall') {
      const probe = await readState();
      if (probe.discovery.state !== 'graded') {
        const isOpen = await p.evaluate(() => document.querySelector('details.overflow')?.open ?? false);
        if (!isOpen) await p.click('details.overflow summary').catch(() => {});
        const revealBtn = await p.$('form[action="/review/reveal"] button');
        if (revealBtn) {
          await revealBtn.click();
          await p.waitForTimeout(1200);
        }
      }
    }

    checkBudget('feedback scan');
    await takeShot('feedback');

    // Scan page state 2 (after answer) - RE-SCAN to avoid stale data
    const scan2 = await scanPage();
    const html2 = await p.content();
    const observation2 = {
      url: p.url(),
      title: await p.title(),
      html: html2,
      aria: scan2.ariaList,
      dom: scan2.domList
    };
    const discovery2 = discoverControls(observation2);

    // Check 2: feedback/Next state - graded state = status + next form, NO answer affordance
    if (discovery2.state !== 'graded') {
      if (discovery2.state === 'unknown') {
        checks.push({ 
          id: 'US-002.2', 
          surface: '/', 
          status: 'unverified', 
          expected: 'graded state after answer', 
          observed: 'discovery state: ' + discovery2.state, 
          evidence: { screenshots: [{ name: 'feedback', path: join(shotsDir, 'feedback.png') }], notes: ['runner-defect: cannot confirm graded state'] } 
        });
      } else {
        findings.push({ 
          id: 'US-002.2', 
          category: 'missing', 
          story: 'US-002', 
          criterion: 'US-002.2', 
          mechanism: 'feedback-absent', 
          title: 'No feedback or Next', 
          expected: 'status+next after answer', 
          actual: 'state=' + discovery2.state, 
          impact: 'walk blocked', 
          uncertainty: null, 
          evidence: { screenshots: [{ name: 'feedback', path: join(shotsDir, 'feedback.png') }], notes: ['discovery state: ' + discovery2.state] }, 
          acceptance: 'needs fix' 
        });
      }
      coverage.exercised.push('US-002.2');
      await b.close();
      const elapsedS = Math.round((Date.now() - startedAt) / 1000);
      const receipt = makeReceipt(discovery, checks, findings, coverage, elapsedS, useragent, networkType);
      const validation = validateReceipt(receipt);
      if (validation.ok) await writeFile(join(outDir, 'receipt.json'), JSON.stringify(receipt, null, 2));
      
      if (findings.length > 0) process.exit(1);
      if (checks.some(c => c.status === 'unverified')) process.exit(3);
      process.exit(2);
    }

    checks.push({ 
      id: 'US-002.2', 
      surface: '/', 
      status: 'pass', 
      expected: 'feedback+next', 
      observed: 'state=' + discovery2.state, 
      evidence: { screenshots: [{ name: 'feedback', path: join(shotsDir, 'feedback.png') }], notes: ['discovery confirmed graded state'] } 
    });
    coverage.exercised.push('US-002.2');

    // Press Next using correct locator: form[data-next] button
    const nextBtn = await p.$('form[data-next] button');
    if (!nextBtn) {
      checks.push({ 
        id: 'US-002.2-next', 
        surface: '/', 
        status: 'unverified', 
        expected: 'next button present', 
        observed: 'not found', 
        evidence: { screenshots: [{ name: 'feedback', path: join(shotsDir, 'feedback.png') }], notes: ['runner-defect: cannot locate next button'] } 
      });
      coverage.exercised.push('US-002.2-next');
      await b.close();
      const elapsedS = Math.round((Date.now() - startedAt) / 1000);
      const receipt = makeReceipt(discovery, checks, findings, coverage, elapsedS, useragent, networkType);
      const validation = validateReceipt(receipt);
      if (validation.ok) await writeFile(join(outDir, 'receipt.json'), JSON.stringify(receipt, null, 2));
      process.exit(3);
    }

    checkBudget('next');
    await nextBtn.click();
    await p.waitForTimeout(1200);
    await takeShot('next');

    // Scan page state 3 (after Next) - RE-SCAN to avoid stale data
    const scan3 = await scanPage();
    const html3 = await p.content();
    const observation3 = {
      url: p.url(),
      title: await p.title(),
      html: html3,
      aria: scan3.ariaList,
      dom: scan3.domList
    };
    const discovery3 = discoverControls(observation3);

    // Check 3: verify transition
    if (html2 === html3 || discovery3.state === discovery2.state) {
      findings.push({ 
        id: 'US-002.2-next', 
        category: 'broken', 
        story: 'US-002', 
        criterion: 'US-002.2', 
        mechanism: 'next-no-transition', 
        title: 'Next did not advance', 
        expected: 'different question or end state', 
        actual: 'state unchanged (html identical, state=' + discovery3.state + ')', 
        impact: 'stuck', 
        uncertainty: null, 
        evidence: { screenshots: [{ name: 'next', path: join(shotsDir, 'next.png') }], notes: ['html identical before/after', 'state unchanged'] }, 
        acceptance: 'needs fix' 
      });
      coverage.exercised.push('US-002.2-next');
    } else {
      checks.push({ 
        id: 'US-002.2-next', 
        surface: '/', 
        status: 'pass', 
        expected: 'advance to next', 
        observed: 'state=' + discovery3.state, 
        evidence: { screenshots: [{ name: 'next', path: join(shotsDir, 'next.png') }], notes: ['html differs before/after', 'state changed'] } 
      });
      coverage.exercised.push('US-002.2-next');
    }

    await b.close();
    const elapsedS = Math.round((Date.now() - startedAt) / 1000);

    const receipt = makeReceipt(discovery, checks, findings, coverage, elapsedS, useragent, networkType);

    const validation = validateReceipt(receipt);
    if (!validation.ok) {
      process.stderr.write('Receipt validation failed: ' + JSON.stringify(validation.errors) + '\n');
      await writeFile(join(outDir, 'receipt.json'), JSON.stringify({ ...receipt, validation_errors: validation.errors }, null, 2));
      process.exit(2);
    }

    await writeFile(join(outDir, 'receipt.json'), JSON.stringify(receipt, null, 2));

    // Exit semantics: 0=pass, 1=findings, 2=blocked/validation, 3=unverified
    const hasUnverified = checks.some(c => c.status === 'unverified');
    if (findings.length > 0) process.exit(1);
    else if (hasUnverified) process.exit(3);
    else process.exit(0);

  } catch (err) {
    try { if (b) await b.close(); } catch (_) {}
    process.stderr.write('Walk failed: ' + err.message + '\n');
    const elapsedS = Math.round((Date.now() - startedAt) / 1000);
    const networkType = getNetworkType(url);
    const note = (err && err.budgetStop) ? 'budget-stop: ' + err.message : 'runner-defect: walk execution failed';

    const catchChecks = [
      { id: 'walk-execution', surface: '/', status: 'unverified', expected: 'complete review walk', observed: err.message || 'exception', evidence: { screenshots: [], notes: [note] } }
    ];

    const receipt = makeReceipt(
      { question: { name: '', status: 'unverified' }, answerAffordance: { type: null, controls: [], status: 'unverified' }, state: 'unknown', notes: [] },
      catchChecks,
      [],  // findings (empty - no completed checks to report)
      { exercised: ['walk-execution'], skipped: [] },  // coverage
      elapsedS,
      '',
      networkType
    );
    const validation = validateReceipt(receipt);
    if (validation.ok) await writeFile(join(outDir, 'receipt.json'), JSON.stringify(receipt, null, 2));
    process.exit(2);  // blocked/environment failure -> exit 2
  }
}

async function main() {
  const [,, cmd, ...args] = process.argv;

  if (cmd === 'candidate') {
    const [op, ...rest] = args;
    if (op === 'up') await candidateUp(rest);
    else if (op === 'down') await candidateDown(rest);
    else { process.stderr.write('unknown candidate op: ' + op + '\n'); process.exit(2); }
  }
  else if (cmd === 'human') await humanWalk(args);
  else { process.stderr.write('unknown command: ' + cmd + '\n'); process.exit(2); }
}

main().catch(error => fail('Blocked: '+error.message));
