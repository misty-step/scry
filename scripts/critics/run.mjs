#!/usr/bin/env node
/**
 * scripts/critics/run.mjs
 * Entry point: plain node, no new npm deps.
 */

import { spawnSync, spawn } from 'node:child_process';
import { rm, writeFile, readFile, mkdir, open, access, constants } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { join, dirname, delimiter } from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import os from 'node:os';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const REPO_ROOT = join(__dirname, '..', '..');

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
  const r = spawnSync('git', ['rev-parse', 'HEAD'], { cwd: REPO_ROOT, encoding: 'utf8' });
  if (r.status !== 0) return null;
  return r.stdout.trim();
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

// === playwright resolution ===
function tryRequirePlaywright() {
  const nodePath = process.env.NODE_PATH;
  if (nodePath) {
    const paths = nodePath.split(delimiter);
    for (const p of paths) {
      if (existsSync(join(p, 'playwright'))) {
        try {
          const require = createRequire(import.meta.url);
          const mod = require(p + '/playwright');
          return { mod, source: 'NODE_PATH' };
        } catch (_) {}
      }
    }
  }

  const pwEnv = process.env.SCRY_CRITICS_PLAYWRIGHT;
  if (pwEnv) {
    try {
      const require = createRequire(import.meta.url);
      const mod = require(pwEnv);
      return { mod, source: 'SCRY_CRITICS_PLAYWRIGHT' };
    } catch (_) {}
    try {
      const require = createRequire(import.meta.url);
      const mod = require(join(REPO_ROOT, pwEnv));
      return { mod, source: 'SCRY_CRITICS_PLAYWRIGHT (relative)' };
    } catch (_) {}
  }

  try {
    const require = createRequire(import.meta.url);
    const mod = require('playwright');
    return { mod, source: 'cwd-require' };
  } catch (_) {}

  const homedir = os.homedir();
  const homedirPaths = [
    join(homedir, '.local/share/mise/installs/npm-playwright/latest/node_modules/playwright'),
    join(homedir, 'node_modules/playwright'),
    join(homedir, '.npm-global/lib/node_modules/playwright'),
  ];
  for (const p of homedirPaths) {
    if (existsSync(p)) {
      try {
        const require = createRequire(import.meta.url);
        const mod = require(p);
        return { mod, source: 'homedir-fallback' };
      } catch (_) {}
    }
  }

  return null;
}

// === chromium resolution ===
async function tryFindChromium() {
  const candidates = [
    process.env.CHROMIUM_PATH,
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
    '/usr/bin/google-chrome',
    '/usr/bin/google-chrome-stable',
  ].filter(Boolean);

  for (const p of candidates) {
    try {
      await access(p, constants.X_OK);
      return p;
    } catch (_) {}
  }

  return null;
}

// === candidate commands ===
async function candidateUp(args) {
  const { parseArgs } = await import('node:util');
  const { values } = parseArgs({
    args,
    options: {
      dir: { type: 'string' },
      port: { type: 'string' },
      out: { type: 'string' },
      json: { type: 'boolean' },
    }
  });

  const dir = values.dir || join(REPO_ROOT, 'target/critics/candidate');
  const port = values.port ? parseInt(values.port) : 18080;
  const outPath = values.out || join(dir, 'candidate.json');
  const dbPath = join(dir, 'synthetic.sqlite');
  const binaryPath = join(dir, 'scry');
  const backupsDir = join(dir, 'backups');

  const homeDir = os.homedir();
  const goBin = join(homeDir, '.local/share/mise/installs/go/1.27.1/bin');
  const goModCache = process.env.GOMODCACHE || join(homeDir, '.go-modcache');

  const env = {
    GOPATH: "",
    PATH: goBin + ':/usr/bin:/bin',
    HOME: homeDir,
    LANG: 'C.UTF-8',
    SCRY_BACKUP_DIR: backupsDir,
    GOMODCACHE: goModCache
  };
  await mkdir(goModCache, { recursive: true });

  try {
    await readFile(outPath);
    process.stderr.write('Candidate handle exists. Use fresh dir: ' + dir + '\n');
    process.exit(2);
  } catch (_) {}

  await rm(dir, { recursive: true, force: true });
  await mkdir(dir);
  await mkdir(backupsDir);

  const buildR = spawnSync('go', ['build', '-mod=readonly', '-o', binaryPath, './cmd/scry'], {
    cwd: REPO_ROOT,
    env, encoding: 'utf8'
  });
  if (buildR.status !== 0) {
    process.stderr.write('Build failed: ' + (buildR.stderr || 'unknown error') + '\n');
    process.exit(2);
  }

  const seedR = spawnSync(binaryPath, ['seed-fixture', '--db', dbPath], { env, encoding: 'utf8' });
  if (seedR.status !== 0) {
    process.stderr.write('Seed failed: ' + seedR.stderr + '\n');
    process.exit(2);
  }
  const seed = JSON.parse(seedR.stdout);

  const logPath = join(dir, 'serve.log');
  const logFd = await open(logPath, 'w');
  const serveCmd = spawn(binaryPath, ['serve', '--dev', '--db', dbPath, '--addr', `127.0.0.1:${port}`], {
    env,
    detached: true,
    stdio: ['ignore', logFd.fd, logFd.fd]
  });
  serveCmd.unref();

  let ready = false;
  const url = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 30; i++) {
    try {
      const { request } = await import('node:http');
      await new Promise((resolve, reject) => {
        request(url + '/readyz', res => {
          let data = '';
          res.on('data', chunk => data += chunk);
          res.on('end', () => res.statusCode === 200 && data.trim() === 'ready' ? resolve() : reject(new Error('not ready')));
        }).on('error', reject).end();
      });
      ready = true;
      break;
    } catch (_) {
      await sleep(500);
    }
  }
  if (!ready) {
    process.stderr.write('Server did not become ready\n');
    serveCmd.kill();
    process.exit(2);
  }

  const revision = getGitRevision();
  const binarySha = sha256File(binaryPath);
  const pid = serveCmd.pid;
  const started_at = new Date().toISOString();

  const handle = {
    format: 'scry-critic-candidate-v1',
    pid, url, dir, port, db: dbPath, binary: binaryPath, binary_sha256: binarySha,
    seed, revision, kind: 'isolated-synthetic', started_at
  };

  await writeFile(outPath, JSON.stringify(handle, null, 2));

  if (values.json) jsonOutput(handle);
  else process.stdout.write('Candidate ready: ' + url + '\n');
}

async function candidateDown(args) {
  const { parseArgs } = await import('node:util');
  const { values } = parseArgs({
    args,
    options: { dir: { type: 'string' }, out: { type: 'string' }, json: { type: 'boolean' } }
  });

  const dir = values.dir || join(REPO_ROOT, 'target/critics/candidate');
  const outPath = values.out || join(dir, 'candidate.json');

  let handle;
  try { handle = JSON.parse(await readFile(outPath, 'utf8')); }
  catch (_) { process.stderr.write('Handle not found: ' + outPath + '\n'); process.exit(2); }

  const ps = spawnSync('ps', ['-o', 'args=', '-p', handle.pid], { encoding: 'utf8' });
  if (ps.status !== 0 || !ps.stdout.includes(handle.binary)) {
    process.stderr.write('PID does not match our binary; refusing to kill\n');
    process.exit(2);
  }

  try {
    process.kill(handle.pid, 'SIGTERM');
    await sleep(2000);
    if (spawnSync('ps', ['-p', handle.pid], { encoding: 'utf8' }).status === 0) {
      process.kill(handle.pid, 'SIGKILL');
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
async function humanWalk(args) {
  const { parseArgs } = await import('node:util');
  const { values } = parseArgs({
    args,
    options: {
      goal: { type: 'string' },
      candidate: { type: 'string' },
      out: { type: 'string' },
      timeout: { type: 'string' },
      'max-steps': { type: 'string' },
      json: { type: 'boolean' }
    }
  });

  const outDir = values.out || join(REPO_ROOT, 'target/critics/run1');
  const url = values.candidate;
  const timeout = values.timeout ? parseInt(values.timeout) : 120;

  await mkdir(outDir, { recursive: true });

  const { validateReceipt } = await import('./lib/receipt.mjs');
  const { classifyOrigin, Budget } = await import('./lib/guards.mjs');
  const { discoverControls, parseAriaSnapshot } = await import('./lib/discover.mjs');
  const { fingerprint, dedupeFindings } = await import('./lib/dedupe.mjs');

  if (url) {
    const result = classifyOrigin(url, { mutating: true });
    if (!result.ok) {
      process.stderr.write('Blocked: ' + result.reason + '\n');
      process.exit(2);
    }
  }

  const shotsDir = join(outDir, 'shots');
  await mkdir(shotsDir, { recursive: true });

  const startedAt = Date.now();
  const checks = [];
  const findings = [];
  const coverage = { exercised: [], skipped: [] };
  const budget = new Budget({});

  // Authored fixture answers (cmd/scry/main.go seed-fixture). The critic knows
  // its own fixture; unknown prompts fall back to placeholder behavior.
  const FIXTURE_ANSWERS = {
    'what type of address does a dns a record map a hostname to?': 'IPv4 address',
    'what protocol does https use to encrypt http?': 'TLS',
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

  function makeReceipt(discovery, checks, findings, coverage, elapsedS, useragent, networkType) {
    return {
      format: 'scry-critic-receipt-v1',
      no_findings: findings.length === 0,
      run: { id: 'crit-' + Date.now(), started_at: new Date(startedAt).toISOString(), ended_at: new Date().toISOString(), duration_s: elapsedS, budget: budget.toJSON() },
      candidate: { revision: getGitRevision(), origin: url || 'localhost:18080', kind: url ? 'external-isolated-synthetic' : 'isolated-synthetic', seed: url ? 'declared-unverified' : 'authored-test-fixture', binary_sha256: url ? null : sha256File(join(REPO_ROOT, 'target/critics/candidate/scry')) },
      environment: { viewport: '390x844', user_agent: useragent, network: networkType, data: 'synthetic-authored' },
      story_bindings: [{ story: 'US-002', criteria: ['US-002.1', 'US-002.2'] }],
      checks,
      findings: dedupeFindings(findings).map(f => ({ ...f, fingerprint: fingerprint({ story: f.story, criterion: f.criterion, mechanism: f.mechanism }) })),
      coverage,
      advisory: { jevs: [], notes: [] },
      limits: []
    };
  }

  async function writeBlockedReceipt(observed) {
    const elapsedS = Math.round((Date.now() - startedAt) / 1000);
    const receipt = makeReceipt(
      { question: { name: '', status: 'unverified' }, answerAffordance: { type: null, controls: [], status: 'unverified' }, state: 'unknown', notes: [] },
      [{ id: 'walk-execution', surface: '/', status: 'unverified', expected: 'complete review walk', observed, evidence: { screenshots: [], notes: ['runner-defect: ' + observed] } }],
      [],
      { exercised: ['walk-execution'], skipped: [] },
      elapsedS, '', getNetworkType(url)
    );
    const validation = validateReceipt(receipt);
    if (validation.ok) await writeFile(join(outDir, 'receipt.json'), JSON.stringify(receipt, null, 2));
  }

  // Browser availability is part of the environment contract: a missing
  // browser is a blocked run, and blocked runs still write a receipt.
  const pw = tryRequirePlaywright();
  if (!pw) {
    process.stderr.write('Playwright unavailable. Use one of:\n');
    process.stderr.write('  NODE_PATH=/path/to/playwright\n');
    process.stderr.write('  SCRY_CRITICS_PLAYWRIGHT=/path/to/playwright\n');
    process.stderr.write('  or add playwright to project package.json\n');
    await writeBlockedReceipt('playwright unavailable');
    process.exit(2);
  }

  const chromium = await tryFindChromium();
  if (!chromium) {
    process.stderr.write('No Chromium found. Set CHROMIUM_PATH or install:\n');
    process.stderr.write('  /usr/bin/chromium\n');
    process.stderr.write('  /usr/bin/google-chrome\n');
    await writeBlockedReceipt('chromium unavailable');
    process.exit(2);
  }

  const b = await pw.mod.chromium.launch({ executablePath: chromium, headless: true });
  const p = await b.newPage({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

  try {
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
    let normalizeSteps = 0;
    while (discovery.state === 'graded' && normalizeSteps < 5) {
      const nb = await p.$('form[data-next] button[type="submit"]');
      if (!nb) break;
      await nb.click();
      await p.waitForTimeout(1200);
      ({ obs: observation, discovery } = await readState());
      normalizeSteps++;
    }

    const takeShot = async (name) => {
      const c = budget.checkScreenshot();
      if (!c.ok) throw new Error('budget exhausted: ' + c.reason);
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

    { const tc = budget.checkTimeout(); if (!tc.ok) throw new Error('budget exhausted: ' + tc.reason); }

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
        await ta.fill(fixtureAnswer || 'test answer');
        await p.click('form.answer-form button[type="submit"]');
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

    // Press Next using correct locator: form[data-next] button[type=submit]
    const nextBtn = await p.$('form[data-next] button[type="submit"]');
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
    await b.close();
    process.stderr.write('Walk failed: ' + err.message + '\n');
    const elapsedS = Math.round((Date.now() - startedAt) / 1000);
    const networkType = getNetworkType(url);
    
    // Build catch receipt - FIXED: correct argument order for makeReceipt
    // makeReceipt(discovery, checks, findings, coverage, elapsedS, useragent, networkType)
    const catchChecks = [
      { id: 'walk-execution', surface: '/', status: 'unverified', expected: 'complete review walk', observed: err.message || 'exception', evidence: { screenshots: [], notes: ['runner-defect: walk execution failed'] } }
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

function main() {
  const [,, cmd, ...args] = process.argv;

  if (cmd === 'candidate') {
    const [op, ...rest] = args;
    if (op === 'up') candidateUp(rest);
    else if (op === 'down') candidateDown(rest);
    else { process.stderr.write('unknown candidate op: ' + op + '\n'); process.exit(2); }
  }
  else if (cmd === 'human') humanWalk(args);
  else { process.stderr.write('unknown command: ' + cmd + '\n'); process.exit(2); }
}

main();