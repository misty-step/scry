// Phase B proof: new content is written only through real engine-backed HTTP
// operations. A typed historical baseline is recovery input, never a successful
// authoring fixture. Local workerd/R2 is not deployed or independent recovery.
import assert from 'node:assert/strict';
import {createHash, randomUUID} from 'node:crypto';
import {execFileSync, spawn} from 'node:child_process';
import {mkdtemp, mkdir, readFile, writeFile, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';

export const FORMAT = 'scry-rust-durable-sqlite-materials-v1';
export const MARKER = 'material_capability/v1';
export const TEXT = '  Café e\u0301 <script>alert("not executable")</script>\r\n\tLiteral https://example.invalid/private-source?q=unchanged\n\n';
export const REVISED = '  Révision <img src=x onerror="notExecutable()">\r\n\tRetain the previous exact wording.\n';
export const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const clone = value => structuredClone(value);
const escapeHtml = text => text.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&#39;');
const emptyBackup = () => ({completed_ms: null, key: null, sha256: null, error: null});
const meta = rows => JSON.parse(rows.find(row => row.key === 'meta').value);
const record = (rows, key) => JSON.parse(rows.find(row => row.key === key).value);
const rowMap = rows => Object.fromEntries(rows.map(row => [row.key, row.value]));
const material = (app, conceptId) => Object.values(app.materials.items).find(item => item.concept_id === conceptId);
const revision = (app, item) => app.materials.revisions[item.current_revision_id];
const operation = extra => ({operation_id: randomUUID(), ...extra});

// Do not normalize entire histories away: compare every existing row byte for
// byte and enumerate the only metadata/token changes authoring may cause.
export function assertHistoricalRetention(before, after, tokenId) {
  const a = rowMap(before), b = rowMap(after);
  for (const [key, value] of Object.entries(a)) {
    if (key === 'meta' || key === `owner_tokens/${tokenId}`) continue;
    assert.equal(b[key], value, `Historical row changed or disappeared: ${key}`);
  }
  const beforeMeta = meta(before), afterMeta = meta(after);
  assert.ok(afterMeta.revision >= beforeMeta.revision);
  afterMeta.revision = beforeMeta.revision;
  assert.deepEqual(afterMeta, beforeMeta, 'Only revision may change in historical metadata.');
  if (tokenId) {
    const oldToken = record(before, `owner_tokens/${tokenId}`), newToken = record(after, `owner_tokens/${tokenId}`);
    newToken.last_used_ms = oldToken.last_used_ms;
    assert.deepEqual(newToken, oldToken, 'Authoring cannot alter token permissions, expiry or verifier.');
  }
}

export function assertRestoreEquivalent(source, actual, space) {
  const expected = clone(source.app);
  assert.match(actual.csrf, /^[a-f0-9]{48}$/);
  assert.notEqual(actual.csrf, source.app.csrf);
  expected.csrf = actual.csrf;
  expected.revision++;
  expected.restored_paused = true;
  expected.backup = emptyBackup();
  for (const job of Object.values(expected.jobs)) {
    if (['sent', 'critic-sent'].includes(job.status)) {
      job.status = 'unknown';
      job.error = 'Restored request has an uncertain paid outcome. Its usage is retained.';
    } else if (['queued', 'candidates'].includes(job.status)) {
      job.status = 'paused';
      job.error = 'Preparation is paused in this restored learning space.';
    }
  }
  assert.ok(Object.values(expected.assessments).every(item => !['sent', 'queued'].includes(item.status)),
    'Baseline must have settled assessments; do not silently discard assessment history.');
  for (const [id, goal] of Object.entries(expected.goals)) {
    if (!goal.photo) continue;
    const key = actual.goals[id].photo.key;
    assert.ok(key.startsWith(`photos/${space}/${goal.photo.sha256}-`));
    assert.notEqual(key, goal.photo.key);
    goal.photo.key = key;
  }
  assert.deepEqual(actual, expected, 'Restore preserves all fields except explicit operational changes.');
}

// Floor pin is intentionally external until Phase A is qualified and published.
// No source checkout, branch, locally rebuilt Worker, or pre-floor release is a
// substitute for the functioning, retained compatibility-writer artifact.
export function validateArtifactPin(pin) {
  assert.ok(pin && /^v\d+\.\d+\.\d+(?:[-.][\w.-]+)?$/.test(pin.release), 'Pin requires a published version.');
  assert.equal(pin.url, `https://github.com/misty-step/scry/releases/download/${pin.release}/scry-worker.zip`);
  for (const key of ['source', 'tree']) assert.match(pin[key] ?? '', /^[a-f0-9]{40}$/, `Pin ${key}`);
  for (const key of ['sourceDigest', 'zip', 'js', 'wasm']) assert.match(pin[key] ?? '', /^[a-f0-9]{64}$/, `Pin ${key}`);
  assert.ok(Number.isSafeInteger(pin.bytes) && pin.bytes > 0, 'Pin requires exact published ZIP length.');
  return pin;
}

export async function loadFloorPin(path = process.env.SCRY_MATERIAL_FLOOR_PIN || 'qa/material-floor-pin.json') {
  let contents;
  try {contents = await readFile(resolve(path), 'utf8');}
  catch (error) {
    if (error.code !== 'ENOENT') throw error;
    throw new Error('UNVERIFIED: the qualified published Phase A release pin is missing. Set SCRY_MATERIAL_FLOOR_PIN or commit qa/material-floor-pin.json after actual release qualification. A source revision or refusing pre-floor artifact is not rollback proof.');
  }
  return validateArtifactPin(JSON.parse(contents));
}

const PRE_FLOOR = Object.freeze({
  release: 'v0.8.1', url: 'https://github.com/misty-step/scry/releases/download/v0.8.1/scry-worker.zip',
  source: '62a7716e6f619c10fbda60630d86461ede33d8b7', tree: '492a98237708f6dc3a50368cd5e1b123e0547f2c',
  sourceDigest: '0a167c12e28116e8bac720c1c493896b2749bde23757e2cd4df2da2505471ad8', bytes: 97855915,
  zip: '7dbd628db9ddffff629b58459cb1f7d6c24fc34a7f7ed51723cc102e852715f8',
  js: '18f375bc666a3c61a2cd91650be8e7b46f8bcc4239b4975e60eb70200afc5c7e',
  wasm: 'b0271b77b3c439e0b3957342e4f3da25dfacd996a4223786b939582bae1d70f8',
});

export async function publishedArtifact(pin, directory, localZip) {
  validateArtifactPin(pin);
  const bytes = localZip ? await readFile(resolve(localZip)) : await (async () => {
    const response = await fetch(pin.url, {signal: AbortSignal.timeout(120000)});
    assert.equal(response.status, 200, 'Pinned public release must be available; downgrade proof is mandatory.');
    return Buffer.from(await response.arrayBuffer());
  })();
  assert.equal(bytes.length, pin.bytes, 'Published ZIP length.');
  assert.equal(sha256(bytes), pin.zip, 'Published ZIP SHA-256.');
  const {unzipSync} = await import('fflate');
  const wanted = ['source.json', 'worker/index.js', 'worker/index_bg.wasm'];
  const entries = unzipSync(bytes, {filter: item => wanted.includes(item.name)});
  assert.deepEqual(Object.keys(entries).sort(), wanted.sort());
  const source = JSON.parse(Buffer.from(entries['source.json']).toString('utf8'));
  assert.equal(source.source_state, 'committed');
  assert.equal(source.git_revision, pin.source); assert.equal(source.revision, pin.source);
  assert.equal(source.git_tree, pin.tree); assert.equal(source.source_sha256, pin.sourceDigest);
  assert.equal(sha256(entries['worker/index.js']), pin.js);
  assert.equal(sha256(entries['worker/index_bg.wasm']), pin.wasm);
  await mkdir(directory);
  await writeFile(join(directory, 'index.js'), entries['worker/index.js']);
  await writeFile(join(directory, 'index_bg.wasm'), entries['worker/index_bg.wasm']);
  return directory;
}

export function parseNativeFields(html, action) {
  const forms = [...html.matchAll(/<form\b[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)];
  const form = forms.find(match => match[1] === action);
  assert.ok(form, `Expected native form ${action}.`);
  return Object.fromEntries([...form[2].matchAll(/<input\b[^>]*name="([^"]+)"[^>]*value="([^"]*)"[^>]*>/g)]
    .map(match => [match[1], match[2]]));
}

async function makeHarness(temp, timestamp, transcript) {
  const {createRuntime, internalKey} = await import('./runtime-helper.mjs');
  const active = new Set();
  let egressAttempts = 0;
  const denyEgress = async () => {egressAttempts++; throw new Error('External calls are forbidden in supplied-text authoring proof.');};
  async function open(label, {artifact, secret, mode = 'development'} = {}) {
    const name = `authoring-${label}-${randomUUID()}`, space = `synthetic-authoring-${randomUUID()}`;
    const resourcePersistencePath = join(temp, name);
    const restoreToken = 'synthetic-authoring-restore-only-capability-32chars';
    let bindings = {SCRY_ENV: mode, CANONICAL_ORIGIN: 'http://127.0.0.1:0', SPACE_NAME: space,
      INTERNAL_KEY: internalKey, RESTORE_TOKEN: restoreToken, SCRY_TEST_NOW_MS: String(timestamp)};
    let runtime, storage;
    const launch = async () => {
      runtime = await createRuntime({port: 0, name, artifact, bindings, inspect: true, testClock: true, resourcePersistencePath, egress: denyEgress});
      bindings = {...bindings, CANONICAL_ORIGIN: (await runtime.mf.ready).origin};
      runtime = {...runtime, ...await runtime.rebind(bindings)};
      storage = await runtime.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name: space});
    };
    await launch();
    const context = {
      space, name,
      get url() {return runtime.url;},
      get secret() {return secret;},
      set secret(value) {secret = value;},
      rows: async () => {
        const tables = await storage.exec("SELECT name FROM sqlite_master WHERE type='table' AND name='records'");
        return tables.length ? storage.exec('SELECT key,value FROM records ORDER BY key') : [];
      },
      bind: async mode => {
        bindings = {...bindings, SCRY_ENV: mode};
        runtime = {...runtime, ...await runtime.rebind(bindings)};
        storage = await runtime.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name: space});
      },
      restart: async () => {await runtime.mf.dispose(); await launch();},
      request: (path, payload, init = {}) => runtime.worker.fetch(runtime.url + path, {
        redirect: 'manual', ...(payload === undefined ? {} : {method: 'POST', body: JSON.stringify(payload)}), ...init,
        headers: {...(secret ? {Authorization: `Bearer ${secret}`} : {}), 'Content-Type': 'application/json', ...init.headers},
      }),
      page: (path, init = {}) => runtime.worker.fetch(runtime.url + path, {redirect: 'manual', ...init}),
      form: (path, fields, init = {}) => runtime.worker.fetch(runtime.url + path, {
        method: 'POST', redirect: 'manual', ...init,
        headers: {'X-Scry-Test-Origin': runtime.url, 'Content-Type': 'application/x-www-form-urlencoded', ...init.headers},
        body: new URLSearchParams(fields),
      }),
      restore: async source => {
        const bytes = Buffer.isBuffer(source) ? source : Buffer.from(JSON.stringify(source));
        return runtime.worker.fetch(runtime.url + '/operator/restore', {method: 'POST',
          headers: {'x-scry-restore-token': restoreToken, 'x-scry-sha256': sha256(bytes)}, body: bytes});
      },
      internal: async (path, {method = 'POST', principal = space} = {}) => {
        const ns = await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE');
        return ns.get(ns.idFromName(principal)).fetch('https://internal.invalid' + path, {
          method, headers: {'x-scry-internal-key': internalKey, 'x-scry-owner': principal === 'scry-qa' ? 'private-qa' : 'private-owner'},
        });
      },
      principalRows: async principal => {
        const other = await runtime.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name: principal});
        const tables = await other.exec("SELECT name FROM sqlite_master WHERE type='table' AND name='records'");
        return tables.length ? other.exec('SELECT key,value FROM records ORDER BY key') : [];
      },
      bucket: bucket => runtime.mf.getR2Bucket(bucket),
      buckets: async () => {
        const snapshot = {};
        for (const name of ['SCRY_ASSETS', 'SCRY_BACKUPS']) {
          const bucket = await runtime.mf.getR2Bucket(name), entries = [];
          for (const item of (await bucket.list()).objects) {
            entries.push({key: item.key, sha256: sha256(Buffer.from(await (await bucket.get(item.key)).arrayBuffer()))});
          }
          snapshot[name] = entries.sort((a, b) => a.key.localeCompare(b.key));
        }
        return snapshot;
      },
      // Compatibility store input is a byte-for-byte copy of rows produced by
      // real Phase B operations, never fabricated content or receipts.
      insertAcceptedRows: async rows => {
        assert.deepEqual(await context.rows(), []);
        await storage.exec('CREATE TABLE records (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID');
        for (const {key, value} of rows) await storage.exec('INSERT INTO records(key,value) VALUES (?,?)', key, value);
      },
      cli: (args, input = '') => new Promise((done, reject) => {
        const binary = resolve(process.env.SCRY_MATERIAL_CLI || 'target/debug/scry');
        const proc = spawn(binary, [...args, '--json', '--config', join(temp, name + '-login.json')], {
          env: {PATH: process.env.PATH, HOME: temp}, stdio: ['pipe', 'pipe', 'pipe'],
        });
        let stdout = '', stderr = '';
        proc.stdout.on('data', bytes => stdout += bytes); proc.stderr.on('data', bytes => stderr += bytes);
        proc.on('error', reject); proc.stdin.on('error', error => {if (error.code !== 'EPIPE') reject(error);});
        proc.on('exit', code => {
          try {
            assert.ok(!secret || (!stdout.includes(secret) && !stderr.includes(secret)), 'CLI must not echo login secrets.');
            const value = JSON.parse(stdout);
            transcript.push({command: args[0] === 'login' ? ['scry', 'login', '--token-stdin'] : ['scry', ...args],
              exit: code, output_sha256: sha256(stdout), limit: 'Synthetic output hash; credential stdin omitted.'});
            done({code, value});
          } catch (error) {reject(error);}
        });
        proc.stdin.end(input);
      }),
      close: async () => {active.delete(context); await runtime.mf.dispose();},
    };
    active.add(context);
    return context;
  }
  return {open, get egressAttempts() {return egressAttempts;}, close: async () => {await Promise.all([...active].map(context => context.close()));}};
}

async function json(response, status = 200) {
  assert.equal(response.status, status, await response.clone().text());
  return response.json();
}
async function html(context, path) {
  const response = await context.page(path);
  assert.equal(response.status, 200, await response.clone().text());
  return response.text();
}
async function exported(context) {return json(await context.request('/api/v1/export'));}
async function ok(context, path, fields) {return json(await context.request('/api/v1' + path, fields));}
async function native(context, path, fields) {
  const response = await context.form(path, {csrf: meta(await context.rows()).csrf, ...operation(fields)});
  assert.equal(response.status, 303, await response.text());
}
async function assertPhotos(context, source, actual) {
  const bucket = await context.bucket('SCRY_ASSETS');
  assert.equal((await bucket.list()).objects.length, source.photos.length);
  for (const photo of source.photos) {
    const goal = Object.values(actual.goals).find(goal => goal.photo?.sha256 === photo.photo.sha256);
    assert.ok(goal);
    const saved = await bucket.get(goal.photo.key); assert.ok(saved);
    const bytes = Buffer.from(await saved.arrayBuffer());
    assert.deepEqual(bytes, Buffer.from(photo.bytes, 'base64')); assert.equal(sha256(bytes), photo.photo.sha256);
  }
}
async function restoreInto(context, source) {
  assert.deepEqual(await context.rows(), []);
  assert.equal((await context.restore(source)).status, 200);
  await context.bind('development');
  const actual = await exported(context);
  assertRestoreEquivalent(source, actual.app, context.space);
  assert.equal(actual.format, source.format);
  await assertPhotos(context, source, actual.app);
  return actual;
}

export async function materialAuthoringJourney() {
  const temp = await mkdtemp(join(tmpdir(), 'scry-material-authoring-')), transcript = [];
  let harness;
  const close = async () => {await harness?.close(); await rm(temp, {recursive: true, force: true});};
  try {
    const fixturePath = join(temp, 'historical-baseline.json');
    execFileSync('cargo', ['test', '--locked', '--test', 'materials_history', 'export_material_fixture', '--', '--ignored', '--exact'],
      {env: {...process.env, SCRY_MATERIAL_FIXTURE_OUTPUT: fixturePath}, stdio: 'pipe'});
    const fixture = JSON.parse(await readFile(fixturePath, 'utf8'));
    const baseline = fixture.baseline_archive, secret = fixture.owner_token_secret, tokenId = fixture.ids.owner_token;
    const timestamp = Math.max(Date.now(), baseline.created_ms) + 60000;
    harness = await makeHarness(temp, timestamp, transcript);
    const {open} = harness;
    let current, baselineRows, firstConcept, firstMaterial, firstRevision, addedMaterial, legacyMaterial, legacyPayload;
    let createPayload, addPayload, revisePayload, currentArchive;
    const criteria = [];
    const check = (name, run) => criteria.push({name, run: async () => {
      await run();
      assert.equal(harness.egressAttempts, 0, 'Authoring and source URLs trigger no provider or external fetch, including failed attempts.');
      transcript.push({observation: name, status: 'pass'});
    }});

    check('New reference reads use an uninitialized snapshot and wrong-principal requests never create rows', async () => {
      const empty = await open('empty-reads');
      try {
        for (const path of ['/api/v1/concepts', '/api/v1/concepts/missing', '/api/v1/concepts/missing/materials',
          '/api/v1/materials/missing', '/api/v1/materials/missing/revisions', '/api/v1/materials/missing/revisions/missing']) {
          const response = await empty.internal(path, {method: 'GET'});
          assert.equal(response.status, path === '/api/v1/concepts' ? 200 : 404);
          assert.deepEqual(await empty.rows(), [], 'Pure reads must not call load(), initialize CSRF, recover work or write token metadata.');
          const denied = await empty.internal(path, {method: 'GET', principal: 'scry-qa'});
          assert.equal(denied.status, 401);
          assert.deepEqual(await empty.principalRows('scry-qa'), []);
        }
        for (const path of ['/api/v1/concepts/create', '/api/v1/materials/add', '/api/v1/materials/revise']) {
          assert.equal((await empty.internal(path, {principal: 'scry-qa'})).status, 401);
          assert.deepEqual(await empty.principalRows('scry-qa'), []);
        }
      } finally {await empty.close();}
    });

    check('Complete historical baseline restores paused; unpaid authoring invents no practice, preparation, spend or history', async () => {
      current = await open('main', {secret, mode: 'restore'});
      const restored = await restoreInto(current, baseline);
      assert.equal(restored.app.restored_paused, true);
      assert.equal(restored.app.owner_tokens[tokenId].paid, false);
      assert.ok(Object.keys(restored.app.goals).length > 0);
      assert.ok(Object.values(restored.app.concepts).every(concept => concept.notes.length === 2));
      assert.equal(restored.app.events.length, 1); assert.equal(restored.app.overrides.length, 1); assert.equal(restored.app.resets.length, 1);
      assert.equal(Object.keys(restored.app.practice.attempts).length, 7); assert.equal(Object.keys(restored.app.practice.corrections).length, 1);
      assert.equal(restored.app.spend.length, 2); assert.equal(Object.keys(restored.app.revival_go).length, 3);
      baselineRows = await current.rows();
      createPayload = operation({name: 'Café <concept>', summary: 'Exact supplied references, without quizzes.', title: 'First <reference>', text: TEXT});
      const created = await ok(current, '/concepts/create', createPayload);
      assert.equal(created.replayed, false); firstConcept = created.result.entity_id;
      let app = (await exported(current)).app;
      assert.equal(app.concepts[firstConcept].name, createPayload.name);
      assert.equal(app.concepts[firstConcept].summary, createPayload.summary);
      assert.equal(app.concepts[firstConcept].origin, 'owner-authored');
      assert.ok(!Object.hasOwn(app.concepts[firstConcept], 'goal_id'));
      assert.deepEqual(app.concepts[firstConcept].notes, []); assert.deepEqual(app.concepts[firstConcept].prerequisites, []);
      for (const field of ['introduced_ms', 'exposure_ms']) assert.equal(app.concepts[firstConcept][field], null);
      firstMaterial = material(app, firstConcept).id;
      firstRevision = app.materials.items[firstMaterial].current_revision_id;
      assert.equal(app.materials.revisions[firstRevision].text, TEXT);
      assert.equal(app.operations[createPayload.operation_id].acting_token, tokenId);
      assertHistoricalRetention(baselineRows, await current.rows(), tokenId);
      const replayRows = await current.rows();
      const replay = await ok(current, '/concepts/create', createPayload);
      assert.equal(replay.replayed, true); assert.deepEqual(replay.result, created.result);
      assert.deepEqual(await current.rows(), replayRows, 'Lost-response replay never appends or writes again.');
      const crossSurface = await current.form('/concepts/create', {...createPayload, csrf: meta(replayRows).csrf});
      assert.equal(crossSurface.status, 303); assert.equal(crossSurface.headers.get('Location'), `/concepts/${firstConcept}`);
      assert.deepEqual(await current.rows(), replayRows, 'Native form/API share one canonical operation and replay without writes.');
      const collision = await current.request('/api/v1/concepts/create', {...createPayload, name: 'Changed intent'});
      assert.equal(collision.status, 409); assert.deepEqual(await current.rows(), replayRows);
      const duplicateName = await ok(current, '/concepts/create', {...createPayload, operation_id: randomUUID()});
      assert.notEqual(duplicateName.result.entity_id, firstConcept, 'Same title/name is not an identity.');
      addPayload = operation({concept_id: firstConcept, title: 'Second independent reference', text: 'Separate material.\r\n'});
      addedMaterial = (await ok(current, '/materials/add', addPayload)).result.entity_id;
      legacyPayload = operation({concept_id: fixture.ids.generated_concepts[0], title: 'Supplied legacy reference', text: TEXT});
      legacyMaterial = (await ok(current, '/materials/add', legacyPayload)).result.entity_id;
      revisePayload = operation({material_id: firstMaterial, expected_revision_id: firstRevision, title: 'Revised <reference>', text: REVISED});
      const revised = await ok(current, '/materials/revise', revisePayload);
      app = (await exported(current)).app;
      const latest = app.materials.revisions[revised.result.entity_id];
      assert.equal(latest.predecessor_id, firstRevision); assert.equal(latest.ordinal, 2); assert.equal(latest.text, REVISED);
      assert.equal(app.materials.revisions[firstRevision].text, TEXT);
      assertHistoricalRetention(baselineRows, await current.rows(), tokenId);
      transcript.push({observation: 'Baseline custody digest retained', revival_sha256: sha256(JSON.stringify(app.revival_go)),
        history_rows_sha256: sha256(JSON.stringify(baselineRows.filter(row => !['meta', `owner_tokens/${tokenId}`].includes(row.key))))});
    });

    check('Read APIs preserve all rows and return current and immutable earlier wording without duplicating legacy Notes', async () => {
      const rows = await current.rows(), app = (await exported(current)).app;
      const listed = await json(await current.request('/api/v1/concepts'));
      assert.deepEqual(listed.concepts.map(item => item.id).sort(), Object.keys(app.concepts).sort());
      const shown = await json(await current.request(`/api/v1/concepts/${firstConcept}`));
      assert.deepEqual(shown.concept, app.concepts[firstConcept]);
      const references = await json(await current.request(`/api/v1/concepts/${firstConcept}/materials`));
      assert.deepEqual(references.materials, shown.materials); assert.equal(references.materials.length, 2);
      const legacy = await json(await current.request(`/api/v1/concepts/${legacyPayload.concept_id}/materials`));
      assert.equal(legacy.materials.filter(item => item.kind === 'legacy-note').length, 1);
      for (const id of [firstMaterial, addedMaterial, legacyMaterial]) {
        for (const suffix of ['', '/revisions']) {
          const result = await json(await current.request(`/api/v1/materials/${id}${suffix}`));
          assert.deepEqual(result.material, app.materials.items[id]);
          assert.deepEqual(result.revisions, Object.values(app.materials.revisions).filter(item => item.material_id === id).sort((a, b) => a.ordinal - b.ordinal));
        }
      }
      const old = await json(await current.request(`/api/v1/materials/${firstMaterial}/revisions/${firstRevision}`));
      assert.deepEqual(old.revision, app.materials.revisions[firstRevision]);
      assert.equal((await current.request(`/api/v1/materials/${addedMaterial}/revisions/${firstRevision}`)).status, 404);
      const rendered = await html(current, `/concepts/${firstConcept}`);
      assert.ok(rendered.includes(escapeHtml(TEXT))); assert.ok(rendered.includes(escapeHtml(REVISED)));
      assert.doesNotMatch(rendered, /<script>alert|<img src=x|href="https:\/\/example.invalid/);
      assert.match(rendered, /Earlier revisions/); assert.match(rendered, /No quizzes\/questions yet/);
      assert.doesNotMatch(rendered, new RegExp(`action="/concepts/${firstConcept}/practice"`));
      assert.deepEqual(await current.rows(), rows, 'All new API reads and authored detail use a pure snapshot.');
    });

    check('Authentication, CSRF, wrong verbs and invalid UTF-8 byte boundaries refuse before any SQLite or R2 change', async () => {
      const before = await current.rows(), buckets = await current.buckets();
      const endpoints = ['/concepts/create', '/materials/add', '/materials/revise'];
      for (const path of endpoints) {
        for (const method of ['GET', 'HEAD', 'PUT', 'PATCH', 'DELETE', 'OPTIONS']) {
          const response = await current.request('/api/v1' + path, undefined, {method});
          assert.ok([404, 405].includes(response.status), `${method} ${path} must refuse.`);
          assert.deepEqual(await current.rows(), before);
          const nativeResponse = await current.page(path, {method});
          assert.ok([403, 404, 405].includes(nativeResponse.status), `Native ${method} ${path} must refuse.`);
          assert.deepEqual(await current.rows(), before);
        }
        const anonymous = await current.page('/api/v1' + path, {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(createPayload)});
        assert.equal(anonymous.status, 401);
        for (const auth of ['Bearer invalid', 'Basic invalid']) {
          const response = await current.request('/api/v1' + path, createPayload, {headers: {Authorization: auth}});
          assert.equal(response.status, 401); assert.equal(response.headers.get('X-Scry-Test-Body-Used'), 'false');
        }
        assert.equal((await current.form(path, operation({title: 'Denied', text: TEXT}))).status, 403);
        assert.equal((await current.form(path, {...createPayload, csrf: 'wrong'})).status, 403);
        assert.equal((await current.form(path, {...createPayload, csrf: meta(before).csrf},
          {headers: {'X-Scry-Test-Origin': 'https://attacker.invalid'}})).status, 403);
      }
      const currentApp = (await exported(current)).app;
      const targets = [
        ['/concepts/create', {name: 'Strict UTF-8', title: 'Strict text'}],
        ['/materials/add', {concept_id: firstConcept, title: 'Strict text'}],
        ['/materials/revise', {material_id: firstMaterial, expected_revision_id: currentApp.materials.items[firstMaterial].current_revision_id, title: 'Strict text'}],
      ];
      for (const [path, fields] of targets) {
        const payload = operation({...fields, text: 'BROKEN_UTF8'});
        const encoded = JSON.stringify(payload);
        const [prefix, suffix] = encoded.split('BROKEN_UTF8');
        for (const body of [Buffer.concat([Buffer.from(prefix), Buffer.from([0xff]), Buffer.from(suffix)]),
          encoded.replace('BROKEN_UTF8', '\\ud800')]) {
          assert.equal((await current.request('/api/v1' + path, undefined, {method: 'POST', body})).status, 422);
          assert.deepEqual(await current.rows(), before, 'Malformed UTF-8/JSON cannot be replaced into accepted wording.');
        }
        const native = {...payload, csrf: meta(before).csrf};
        const formBody = new URLSearchParams(native).toString();
        for (const body of [formBody.replace('BROKEN_UTF8', '%FF'),
          Buffer.concat(formBody.split('BROKEN_UTF8').flatMap((part, i) => i ? [Buffer.from([0xff]), Buffer.from(part)] : [Buffer.from(part)]))]) {
          assert.equal((await current.page(path, {method: 'POST', headers: {
            'X-Scry-Test-Origin': current.url, 'Content-Type': 'application/x-www-form-urlencoded'}, body})).status, 422);
          assert.deepEqual(await current.rows(), before, 'Native percent decoding cannot silently replace malformed bytes.');
        }
        const boundary = 'synthetic-authoring-utf8-boundary';
        const rawMultipart = Object.entries(native).map(([key, value]) =>
          `--${boundary}\r\nContent-Disposition: form-data; name="${key}"\r\n\r\n${value}\r\n`).join('') + `--${boundary}--\r\n`;
        const [beforeText, afterText] = rawMultipart.split('BROKEN_UTF8');
        const malformedMultipart = Buffer.concat([Buffer.from(beforeText), Buffer.from([0xff]), Buffer.from(afterText)]);
        assert.equal((await current.page(path, {method: 'POST', headers: {'X-Scry-Test-Origin': current.url,
          'Content-Type': `multipart/form-data; boundary=${boundary}`}, body: malformedMultipart})).status, 422);
        assert.deepEqual(await current.rows(), before, 'Multipart text cannot silently replace malformed bytes.');
        const multipart = new FormData();
        for (const [key, value] of Object.entries(native)) multipart.set(key, value);
        multipart.set('attachment', new Blob(['Unsupported supplied file']), 'not-supported.txt');
        assert.equal((await current.page(path, {method: 'POST', headers: {'X-Scry-Test-Origin': current.url}, body: multipart})).status, 422);
        assert.deepEqual(await current.rows(), before, 'File-valued fields cannot be dropped from the canonical payload.');
      }
      const invalid = [
        {name: 'é'.repeat(101)}, {title: 'é'.repeat(101)}, {summary: 'é'.repeat(1001)}, {text: 'é'.repeat(16385)},
        {text: 'x'.repeat(32769)}, {text: ' \r\n\t '}, {text: 'x\0y'}, {text: 'x\u0001y'}, {name: ''}, {title: ''},
        {source_url: 'https://example.invalid/must-not-fetch'}, {paid: true}, {format: 'pdf'},
      ];
      for (const changed of invalid) {
        const response = await current.request('/api/v1/concepts/create', {...createPayload, ...changed, operation_id: randomUUID()});
        assert.equal(response.status, 422, JSON.stringify(Object.keys(changed)));
        assert.deepEqual(await current.rows(), before, 'Rejected input creates no rows, receipt or token last-use.');
      }
      assert.deepEqual(await current.buckets(), buckets);
      const boundary = await ok(current, '/concepts/create', operation({name: 'é'.repeat(100), title: 'é'.repeat(100), summary: 'é'.repeat(1000), text: 'é'.repeat(16384)}));
      const app = (await exported(current)).app;
      assert.equal(Buffer.byteLength(revision(app, material(app, boundary.result.entity_id)).text), 32768);
      assertHistoricalRetention(baselineRows, await current.rows(), tokenId);
    });

    check('Competing revisions append exactly one immutable successor and identical-operation concurrency replays once', async () => {
      const prior = (await exported(current)).app, expected = prior.materials.items[firstMaterial].current_revision_id;
      const contenders = [0, 1].map(index => operation({material_id: firstMaterial, expected_revision_id: expected, title: `Race ${index}`, text: `Winner ${index}\r\n`}));
      const responses = await Promise.all(contenders.map(payload => current.request('/api/v1/materials/revise', payload)));
      assert.deepEqual(responses.map(response => response.status).sort(), [200, 409]);
      const winner = responses.findIndex(response => response.status === 200), won = await responses[winner].json();
      let app = (await exported(current)).app;
      assert.equal(Object.keys(app.materials.revisions).length, Object.keys(prior.materials.revisions).length + 1);
      assert.equal(app.materials.items[firstMaterial].current_revision_id, won.result.entity_id);
      assert.equal(app.materials.revisions[won.result.entity_id].predecessor_id, expected);
      assert.ok(!Object.hasOwn(app.operations, contenders[1 - winner].operation_id));
      for (const [id, old] of Object.entries(prior.materials.revisions)) assert.deepEqual(app.materials.revisions[id], old);
      const rows = await current.rows();
      assert.equal((await ok(current, '/materials/revise', contenders[winner])).replayed, true);
      const earlierReplay = await ok(current, '/materials/revise', revisePayload);
      assert.equal(earlierReplay.replayed, true);
      assert.equal(earlierReplay.result.entity_id, expected, 'Replaying an older accepted revision returns that revision, not the newer head.');
      assert.equal((await ok(current, '/materials/add', addPayload)).replayed, true);
      assert.equal((await current.request('/api/v1/materials/revise', {...contenders[winner], text: 'Changed same-operation content'})).status, 409);
      assert.equal((await current.request('/api/v1/materials/revise', {...contenders[1 - winner], operation_id: randomUUID()})).status, 409);
      assert.deepEqual(await current.rows(), rows);
      const same = operation({concept_id: firstConcept, title: 'Double submit', text: 'Only one durable Material.\n'});
      const double = await Promise.all([ok(current, '/materials/add', same), ok(current, '/materials/add', same)]);
      assert.deepEqual(double.map(result => result.replayed).sort(), [false, true]);
      assert.deepEqual(double[0].result, double[1].result);
      app = (await exported(current)).app;
      assertHistoricalRetention(baselineRows, await current.rows(), tokenId);
      assert.equal(app.restored_paused, true);
    });

    check('Compiled CLI creates, lists, shows, adds and revises exact file/stdin bytes through the same unpaid API', async () => {
      let result = await current.cli(['login', '--url', current.url, '--token-stdin'], secret + '\n'); assert.equal(result.code, 0);
      const input = join(temp, 'literal-input.txt'); await writeFile(input, TEXT);
      const createArgs = ['concept', 'create', '--name', 'CLI concept', '--summary', 'CLI summary', '--title', 'CLI first', '--text-file', input, '--operation-id', randomUUID()];
      result = await current.cli(createArgs); assert.equal(result.code, 0);
      const conceptId = result.value.result.entity_id;
      const app = (await exported(current)).app, item = material(app, conceptId);
      assert.equal(revision(app, item).text, TEXT);
      result = await current.cli(createArgs); assert.equal(result.code, 0); assert.equal(result.value.replayed, true);
      result = await current.cli(['material', 'add', conceptId, '--title', 'CLI stdin', '--stdin', '--operation-id', randomUUID()], TEXT);
      assert.equal(result.code, 0);
      const reviseArgs = ['material', 'revise', item.id, '--expected-revision-id', item.current_revision_id, '--title', 'CLI new wording', '--stdin', '--operation-id', randomUUID()];
      result = await current.cli(reviseArgs, REVISED); assert.equal(result.code, 0);
      const final = (await exported(current)).app; assert.equal(revision(final, final.materials.items[item.id]).text, REVISED);
      const reads = [
        [['concept', 'list'], '/api/v1/concepts'], [['concept', 'show', conceptId], `/api/v1/concepts/${conceptId}`],
        [['material', 'list', conceptId], `/api/v1/concepts/${conceptId}/materials`],
        [['material', 'show', item.id], `/api/v1/materials/${item.id}`],
        [['material', 'history', item.id], `/api/v1/materials/${item.id}/revisions`],
        [['material', 'show', item.id, '--revision-id', item.current_revision_id], `/api/v1/materials/${item.id}/revisions/${item.current_revision_id}`],
      ];
      const rows = await current.rows();
      for (const [args, path] of reads) {
        result = await current.cli(args); assert.equal(result.code, 0);
        assert.deepEqual(result.value, await json(await current.request(path)));
      }
      result = await current.cli(reviseArgs, REVISED); assert.equal(result.code, 0); assert.equal(result.value.replayed, true);
      result = await current.cli(reviseArgs, 'Changed operation'); assert.notEqual(result.code, 0);
      assert.deepEqual(await current.rows(), rows);
      assertHistoricalRetention(baselineRows, await current.rows(), tokenId);
    });

    check('SQLite restart and complete local R2 archive/fresh restore retain every accepted row, photo and operation', async () => {
      currentArchive = await exported(current);
      const rows = await current.rows(), objects = await current.buckets();
      await current.restart();
      assert.deepEqual(await current.rows(), rows, 'Real persisted SQLite survives dispose/reopen.');
      assert.deepEqual(await exported(current), currentArchive);
      assert.deepEqual(await current.buckets(), objects);
      const backed = await current.internal('/__internal/daily'); assert.equal(backed.status, 200, await backed.text());
      const bucket = await current.bucket('SCRY_BACKUPS');
      const listed = await bucket.list({prefix: `snapshots/${current.space}/`}); assert.equal(listed.objects.length, 1);
      const bytes = Buffer.from(await (await bucket.get(listed.objects[0].key)).arrayBuffer());
      const archived = JSON.parse(bytes); assert.deepEqual(archived, currentArchive);
      const after = await exported(current), expected = clone(currentArchive.app);
      expected.revision++; expected.backup = {completed_ms: timestamp, key: listed.objects[0].key, sha256: sha256(bytes), error: null};
      assert.deepEqual(after.app, expected);
      const restored = await open('fresh-r2', {secret, mode: 'restore'});
      try {
        const read = await restoreInto(restored, archived), beforeWrite = await restored.rows();
        const write = await ok(restored, '/settings', operation({pace: 'light'})); assert.equal(write.replayed, false);
        const written = await exported(restored);
        assert.deepEqual(written.app.materials, read.app.materials);
        for (const [id, receipt] of Object.entries(read.app.operations)) assert.deepEqual(written.app.operations[id], receipt);
        for (const row of beforeWrite) if (!['meta', `owner_tokens/${tokenId}`].includes(row.key)) assert.equal(rowMap(await restored.rows())[row.key], row.value);
        const prior = await restored.rows(), objects = await restored.buckets();
        await restored.bind('restore');
        for (const stale of [baseline, archived]) {
          assert.equal((await restored.restore(stale)).status, 409);
          assert.deepEqual(await restored.rows(), prior); assert.deepEqual(await restored.buckets(), objects);
        }
      } finally {await restored.close();}
      transcript.push({observation: 'Complete R2 bytes read back before fresh isolated restore', archive_sha256: sha256(bytes),
        limit: 'Local workerd/R2 only; no independent hosted recovery, deployment or physical-phone acceptance.'});
    });

    check('Protected concept and exact Prompt hide all material revisions, JSON reads, edit forms and receipt replays', async () => {
      const protectedSpace = await open('protection', {secret, mode: 'restore'});
      try {
        await restoreInto(protectedSpace, currentArchive);
        const app = (await exported(protectedSpace)).app, conceptId = legacyPayload.concept_id;
        for (const question of Object.values(app.questions)) {
          if (question.concept_id !== conceptId && !question.archived) await native(protectedSpace, `/questions/${question.id}/archive`, {});
        }
        const held = meta(await protectedSpace.rows()).occurrence;
        await native(protectedSpace, '/review/next', {occurrence_id: held.id});
        const selected = meta(await protectedSpace.rows()).occurrence;
        assert.equal(selected.concept_id, conceptId); assert.equal(selected.phase, 'question'); assert.equal(selected.assisted, false);
        const protectedRows = await protectedSpace.rows();
        const legacyRevisions = Object.values(app.materials.revisions).filter(item => item.material_id === legacyMaterial);
        for (const path of [`/concepts/${conceptId}`, `/concepts/${conceptId}/materials/new`, `/materials/${legacyMaterial}/edit`,
          `/materials/${legacyMaterial}`, ...legacyRevisions.map(item => `/materials/${legacyMaterial}/revisions/${item.id}`)]) {
          const rendered = await html(protectedSpace, path);
          for (const item of legacyRevisions) {assert.ok(!rendered.includes(escapeHtml(item.text))); assert.ok(!rendered.includes(escapeHtml(item.title)));}
          assert.doesNotMatch(rendered, /name="(?:text|expected_revision_id)"|data-material-revision-id=/);
        }
        for (const path of ['/api/v1/concepts', `/api/v1/concepts/${conceptId}`, `/api/v1/concepts/${conceptId}/materials`,
          `/api/v1/materials/${legacyMaterial}`, `/api/v1/materials/${legacyMaterial}/revisions`,
          ...legacyRevisions.map(item => `/api/v1/materials/${legacyMaterial}/revisions/${item.id}`)]) {
          const response = await protectedSpace.request(path); assert.equal(response.status, 409);
          const body = await response.text(); for (const item of legacyRevisions) assert.ok(!body.includes(item.text));
        }
        assert.equal((await protectedSpace.request('/api/v1/materials/add', legacyPayload)).status, 409, 'Previously accepted receipt cannot bypass current assistance.');
        assert.deepEqual(await protectedSpace.rows(), protectedRows);
        const gate = await html(protectedSpace, `/concepts/${conceptId}`), help = parseNativeFields(gate, '/review/help');
        assert.equal((await protectedSpace.form('/review/help', help)).status, 303);
        // Create an exact unaided Prompt via the existing real next operation.
        const current = meta(await protectedSpace.rows()).practice.current;
        const cardId = current.targets[0].card_id;
        await ok(protectedSpace, '/next', operation({card_id: cardId, after: current.prompt.id}));
        const prompt = meta(await protectedSpace.rows()).practice.current;
        assert.equal(prompt.prompt.assisted, false); assert.equal(prompt.attempt_id, null);
        const exactRows = await protectedSpace.rows();
        for (const path of ['/api/v1/concepts', `/api/v1/concepts/${firstConcept}`, `/api/v1/concepts/${firstConcept}/materials`,
          `/api/v1/materials/${firstMaterial}`, `/api/v1/materials/${firstMaterial}/revisions`, `/api/v1/materials/${firstMaterial}/revisions/${firstRevision}`]) {
          assert.equal((await protectedSpace.request(path)).status, 409);
        }
        for (const [path, fields] of [['/concepts/create', createPayload], ['/materials/add', addPayload], ['/materials/revise', revisePayload]]) {
          assert.equal((await protectedSpace.request('/api/v1' + path, fields)).status, 409);
          assert.equal((await protectedSpace.request('/api/v1' + path, {...fields, operation_id: randomUUID()})).status, 409);
        }
        for (const path of ['/concepts/new', `/concepts/${firstConcept}`, `/concepts/${firstConcept}/materials/new`, `/materials/${firstMaterial}/edit`,
          `/materials/${firstMaterial}`, `/materials/${firstMaterial}/revisions/${firstRevision}`]) {
          const rendered = await html(protectedSpace, path);
          assert.ok(!rendered.includes(escapeHtml(TEXT))); assert.ok(!rendered.includes(escapeHtml(REVISED)));
          assert.doesNotMatch(rendered, /name="(?:text|expected_revision_id)"|data-material-revision-id=/);
        }
        assert.deepEqual(await protectedSpace.rows(), exactRows, 'Gates refuse without assistance, exposure, receipt, or stream changes.');
      } finally {await protectedSpace.close();}
    });

    check('Escaping-expanded complete-archive overflow refuses before committing any partial authoring write', async () => {
      const full = await open('archive-bound', {secret, mode: 'restore'});
      try {
        await restoreInto(full, baseline);
        const made = await ok(full, '/concepts/create', operation({name: 'Archive boundary', title: 'Bounded exact text', text: 'Seed\n'}));
        const escaped = '\\'.repeat(32768);
        let refused = false;
        // Each valid 32 KiB payload expands to >64 KiB in JSON. At most 256
        // successful additions fit the 16 MiB complete archive, without photos.
        for (let count = 0; count < 260; count++) {
          const before = await full.rows(), objects = await full.buckets();
          const payload = operation({concept_id: made.result.entity_id, title: `Exact escaped text ${count}`, text: escaped});
          const response = await full.request('/api/v1/materials/add', payload);
          if (response.status === 200) {
            const saved = await response.json();
            const rows = await full.rows(), item = record(rows, `materials/${saved.result.entity_id}`);
            assert.equal(record(rows, `material_revisions/${item.current_revision_id}`).text, escaped);
            continue;
          }
          assert.equal(response.status, 413, await response.text());
          assert.deepEqual(await full.rows(), before, 'Overflow must roll back content, receipt, token metadata and revision.');
          assert.deepEqual(await full.buckets(), objects);
          const archived = await exported(full);
          assert.ok(Buffer.byteLength(JSON.stringify(archived)) <= 16 * 1024 * 1024);
          assert.ok(!Object.hasOwn(archived.app.operations, payload.operation_id));
          refused = true; transcript.push({observation: 'Real operations filled the complete archive to refusal', successful_additions: count}); break;
        }
        assert.equal(refused, true, 'Bounded archive fixture must actually reach precommit overflow.');
      } finally {await full.close();}
    });

    check('Published pre-floor artifact refuses real Phase B archives and accepted store rows without writes', async () => {
      const artifact = await publishedArtifact(PRE_FLOOR, join(temp, 'pre-floor'), process.env.SCRY_MATERIAL_OLD_WORKER_ZIP);
      const old = await open('pre-floor-restore', {secret, mode: 'restore', artifact});
      try {
        assert.equal((await old.restore(currentArchive)).status, 422);
        assert.deepEqual(await old.rows(), []); assert.deepEqual(await old.buckets(), {SCRY_ASSETS: [], SCRY_BACKUPS: []});
      } finally {await old.close();}
      const store = await open('pre-floor-store', {secret, artifact});
      try {
        const acceptedRows = await current.rows();
        await store.insertAcceptedRows(acceptedRows);
        for (const photo of currentArchive.photos) await (await store.bucket('SCRY_ASSETS')).put(photo.photo.key, Buffer.from(photo.bytes, 'base64'));
        const objects = await store.buckets();
        for (const [path, payload] of [['/api/v1/export', undefined], ['/api/v1/settings', operation({pace: 'light'})]]) {
          assert.equal((await store.request(path, payload)).status, 500);
          assert.deepEqual(await store.rows(), acceptedRows); assert.deepEqual(await store.buckets(), objects);
        }
      } finally {await store.close();}
    });

    check('Qualified published Phase A rollback writer restores Phase B records and retains them on its first unrelated write', async () => {
      const pin = await loadFloorPin();
      assert.notEqual(pin.zip, PRE_FLOOR.zip, 'The pre-floor artifact cannot be relabeled as the compatible rollback writer.');
      const artifact = await publishedArtifact(pin, join(temp, 'qualified-floor'), process.env.SCRY_MATERIAL_FLOOR_WORKER_ZIP);
      const floor = await open('qualified-floor', {secret, mode: 'restore', artifact});
      try {
        const restored = await restoreInto(floor, currentArchive), before = await floor.rows();
        const payload = operation({pace: 'light'});
        const result = await ok(floor, '/settings', payload); assert.equal(result.replayed, false);
        const actual = (await exported(floor)).app, expected = clone(restored.app);
        expected.revision++; expected.preferences.pace = 'light'; expected.owner_tokens[tokenId].last_used_ms = timestamp;
        const fields = {operation_id: payload.operation_id, pace: payload.pace};
        expected.operations[payload.operation_id] = {acting_token: tokenId, path: '/settings', created_ms: timestamp,
          payload_hash: sha256(JSON.stringify(['/settings', fields])), result: {redirect: '/settings', entity_id: null, message: null}};
        assert.deepEqual(actual, expected, 'A real published rollback writer retains every Phase B field and acknowledged receipt.');
        for (const row of before) if (!['meta', `owner_tokens/${tokenId}`].includes(row.key)) assert.equal(rowMap(await floor.rows())[row.key], row.value);
        await floor.restart(); assert.deepEqual((await exported(floor)).app, actual);
        transcript.push({observation: 'Functioning compatible rollback writer', proof_subject: 'published-phase-a-floor', pin});
      } finally {await floor.close();}
      const store = await open('qualified-floor-store', {secret, artifact});
      try {
        const accepted = await exported(current), acceptedRows = await current.rows();
        await store.insertAcceptedRows(acceptedRows);
        for (const photo of accepted.photos) await (await store.bucket('SCRY_ASSETS')).put(photo.photo.key, Buffer.from(photo.bytes, 'base64'));
        // The first learner-facing action after opening this accepted store is
        // an unrelated ordinary mutation, not a migration or a restore.
        const payload = operation({pace: 'light'}), result = await ok(store, '/settings', payload);
        assert.equal(result.replayed, false);
        const actual = (await exported(store)).app, expected = clone(accepted.app);
        expected.revision++; expected.preferences.pace = 'light'; expected.owner_tokens[tokenId].last_used_ms = timestamp;
        expected.operations[payload.operation_id] = {acting_token: tokenId, path: '/settings', created_ms: timestamp,
          payload_hash: sha256(JSON.stringify(['/settings', {operation_id: payload.operation_id, pace: payload.pace}])),
          result: {redirect: '/settings', entity_id: null, message: null}};
        assert.deepEqual(actual, expected, 'Published floor can write an already-accepted Phase B store without shedding records.');
        const writtenRows = rowMap(await store.rows());
        for (const row of acceptedRows) if (!['meta', `owner_tokens/${tokenId}`].includes(row.key)) assert.equal(writtenRows[row.key], row.value);
        await assertPhotos(store, accepted, actual);
      } finally {await store.close();}
    });
    return {criteria, transcript, close};
  } catch (error) {await close(); throw error;}
}

// Native forms are driven without JavaScript at every requested width/theme.
// A separate enhanced-browser case covers an actual committed response loss.
export async function materialAuthoringWalk(browser, capture) {
  const temp = await mkdtemp(join(tmpdir(), 'scry-material-browser-')), results = [];
  const harness = await makeHarness(temp, Date.now(), []);
  const cases = [320, 390, 1280].flatMap(width => ['light', 'dark'].map(scheme => ({width, scheme, js: false})));
  cases.push({width: 390, scheme: 'light', js: true});
  cases.push({width: 390, scheme: 'dark', js: false, paused: true});
  try {
    for (const {width, scheme, js, paused = false} of cases) {
      const name = `material-authoring-${width}-${scheme}-${paused ? 'restored-paused-' : ''}${js ? 'enhanced' : 'no-js'}`;
      const runtime = await harness.open(name, {mode: paused ? 'restore' : 'development'});
      if (paused) {
        const seed = await harness.open('empty-restore-source');
        try {
          const source = await json(await seed.internal('/api/v1/export', {method: 'GET'}));
          assert.equal((await runtime.restore(source)).status, 200);
          await runtime.bind('development');
          assert.equal(meta(await runtime.rows()).restored_paused, true);
        } finally {await seed.close();}
      }
      const context = await browser.newContext({viewport: {width, height: 844},
        colorScheme: scheme, javaScriptEnabled: js, isMobile: width < 1280, hasTouch: width < 1280});
      const page = await context.newPage(); page.setDefaultTimeout(12000);
      const result = {name, width, scheme, javascript: js, restored_paused: paused, input: width < 1280 ? 'native touch' : 'native keyboard',
        zoom: [100, 200], text_size: [100, 200], status: 'pass', evidence: [], geometry: [], scope: 'Real local Worker/native forms; CSS 200% layout zoom and doubled computed text sizes, not physical-phone or provider-quality acceptance.'};
      results.push(result);
      console.log(`Starting synthetic browser case: ${name}`);
      const errors = []; page.on('pageerror', error => errors.push(error.message));
      const shot = async suffix => {
        // Capture the whole page from its top. A negative-top fixed skip link
        // otherwise appears inside Chromium's beyond-viewport screenshot even
        // though it is outside the actual scrolled viewport.
        await page.evaluate(() => window.scrollTo(0, 0));
        // Page callbacks may not run with JavaScript disabled. Poll a
        // synchronous geometry fact from Node, with a hard readiness bound.
        let atTop = false;
        for (let attempt = 0; attempt < 40; attempt++) {
          atTop = await page.evaluate(() => scrollY === 0 && scrollX === 0);
          if (atTop) break;
          await new Promise(resolve => setTimeout(resolve, 25));
        }
        assert.ok(atTop, 'Screenshot scroll position reaches the page top within one second.');
        const skipLink = await page.locator('.skip-link').evaluate(node => ({
          focused: node === document.activeElement, bottom: node.getBoundingClientRect().bottom,
        }));
        assert.ok(skipLink.focused || skipLink.bottom <= 0, 'An unfocused skip link cannot obscure the viewport.');
        result.evidence.push(await capture(page, `${name}-${suffix}.png`));
      };
      const activate = async locator => {
        if (width < 1280) {await locator.tap(); return;}
        for (let i = 0; i < 80; i++) {
          if (await locator.evaluate(node => node === document.activeElement)) {await page.keyboard.press('Enter'); return;}
          await page.keyboard.press('Tab');
        }
        assert.fail('Control is not reachable by native keyboard tab order.');
      };
      const layout = async suffix => {
        await page.evaluate(() => document.fonts.ready);
        for (const mode of ['normal', 'zoom', 'text']) {
          const sizes = await page.evaluate(mode => {
            document.documentElement.style.zoom = mode === 'zoom' ? '2' : '1';
            for (const node of document.querySelectorAll('[data-qa-text-size]')) {
              node.style.removeProperty('font-size'); delete node.dataset.qaTextSize;
            }
            const nodes = [...document.querySelectorAll('body,body *')];
            const original = nodes.map(node => parseFloat(getComputedStyle(node).fontSize));
            if (mode === 'text') {
              nodes.forEach((node, i) => {node.dataset.qaTextSize = String(original[i] * 2);});
              nodes.forEach(node => {node.style.fontSize = node.dataset.qaTextSize + 'px';});
            }
            return nodes.map((node, i) => ({before: original[i], after: parseFloat(getComputedStyle(node).fontSize)}));
          }, mode);
          for (const size of sizes) assert.ok(Math.abs(size.after - size.before * (mode === 'text' ? 2 : 1)) < 0.1,
            'Computed text size must match the declared 100% or 200% mode.');
          const zoom = mode === 'normal' ? 1 : 2;
          const geometry = await page.evaluate(() => ({inner_width: innerWidth,
            client_width: document.documentElement.clientWidth, scroll_width: document.documentElement.scrollWidth,
            visual_width: visualViewport.width, visual_scale: visualViewport.scale}));
          result.geometry.push({page: suffix, mode, configured_width: width, ...geometry});
          assert.ok(geometry.inner_width <= width + 0.5, `${name}: mobile layout cannot silently expand its configured viewport`);
          assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, `${name} ${zoom * 100}% overflow`);
          const clipped = await page.locator('main input:not([type=hidden]), main textarea, main button, main a.button').evaluateAll(nodes =>
            nodes.filter(node => {const box = node.getBoundingClientRect(); return box.width > 0 && (box.left < -0.5 || box.right > innerWidth + 0.5);}).map(node => node.name || node.textContent));
          assert.deepEqual(clipped, [], `${name}: authoring controls fit at ${zoom * 100}%`);
          const wrappedActions = await page.locator('.material-form .actions .button').evaluateAll(nodes => nodes.filter(node => {
            const text = document.createRange(); text.selectNodeContents(node);
            return text.getClientRects().length !== 1;
          }).map(node => node.textContent));
          assert.deepEqual(wrappedActions, [], `${name}: Save/Cancel labels stay on one readable line at ${zoom * 100}%`);
          await shot(`${suffix}-${mode === 'text' ? 'text-' : ''}${zoom * 100}`);
          if (mode === 'text') result.evidence.push(await capture(page, `${name}-${suffix}-text-200-viewport.png`, {fullPage: false}));
        }
        await page.evaluate(() => {
          document.documentElement.style.zoom = '1';
          for (const node of document.querySelectorAll('[data-qa-text-size]')) {
            node.style.removeProperty('font-size'); delete node.dataset.qaTextSize;
          }
        });
      };
      const fill = async (action, values) => {
        const form = page.locator(`form[action="${action}"]`); await form.waitFor();
        assert.equal(await form.getAttribute('method'), 'post');
        assert.ok(await form.locator('input[name=csrf]').inputValue()); assert.ok(await form.locator('input[name=operation_id]').inputValue());
        for (const [name, value] of Object.entries(values)) await form.locator(`[name="${name}"]`).fill(value);
        return form;
      };
      try {
        await page.goto(runtime.url + '/'); await page.locator('[data-state="concept-home"]').waitFor();
        const empty = await runtime.rows();
        const link = page.locator('main').getByRole('link', {name: 'Create concept', exact: true});
        assert.equal(await link.getAttribute('href'), '/concepts/new');
        await layout('empty');
        const paid = page.locator('main').getByRole('link', {name: 'Prepare material with AI', exact: true});
        assert.equal(await paid.getAttribute('href'), '/create');
        await activate(paid); await page.getByRole('heading', {name: 'Prepare material', exact: true}).waitFor();
        await page.getByText('Preparation is unavailable right now.', {exact: true}).waitFor();
        assert.equal(await page.locator('form[action="/create"], input[type="file"]').count(), 0);
        assert.deepEqual(await runtime.rows(), empty, 'Unavailable paid preparation does not affect free authoring.');
        await layout('preparation-unavailable');
        await activate(page.locator('main').getByRole('link', {name: 'Create concept', exact: true}));
        let form = await fill('/concepts/create', {name: 'Cancelled concept', title: 'Unsent', text: 'Do not save this draft.'});
        await activate(page.getByRole('link', {name: 'Cancel', exact: true}));
        await page.waitForURL(runtime.url + '/'); assert.deepEqual(await runtime.rows(), empty, 'Cancel creates no content or receipt.');
        await page.goBack(); await page.locator('form[action="/concepts/create"]').waitFor();
        assert.deepEqual(await runtime.rows(), empty, 'Back to unsent form is not a save.');
        await page.reload(); assert.deepEqual(await runtime.rows(), empty, 'Reloading form is not a save.');
        form = await fill('/concepts/create', {name: 'Browser café <concept>', summary: 'Saved without preparation.', title: 'é'.repeat(101), text: TEXT});
        const draftOperation = await form.locator('input[name=operation_id]').inputValue();
        if (js) {
          await activate(form.getByRole('button', {name: 'Save', exact: true}));
          await page.getByText('Keep title within 200 UTF-8 bytes. Your draft is still here.', {exact: true}).waitFor();
        } else {
          await Promise.all([
            page.waitForResponse(response => new URL(response.url()).pathname === '/concepts/create' && response.status() === 422),
            activate(form.getByRole('button', {name: 'Save', exact: true})),
          ]);
          await page.getByText('Supply a title up to 200 bytes and nonempty text up to 32 KiB.', {exact: true}).waitFor();
        }
        form = page.locator('form[action="/concepts/create"]');
        assert.equal(await form.locator('[name=title]').inputValue(), 'é'.repeat(101));
        assert.equal(await form.locator('[name=text]').inputValue(), TEXT.replaceAll('\r\n', '\n'));
        assert.equal(await form.locator('[name=operation_id]').inputValue(), draftOperation);
        assert.deepEqual(await runtime.rows(), empty, 'Invalid UTF-8 byte length preserves the draft and creates no content or receipt.');
        await shot('invalid-draft-retained');
        form = await fill('/concepts/create', {name: 'Browser café <concept>', summary: 'Saved without preparation.', title: 'Browser <reference>', text: TEXT});
        await layout('create-form');
        await activate(form.getByRole('button', {name: 'Save', exact: true}));
        await page.locator('[data-state="concept"]').waitFor();
        const conceptId = new URL(page.url()).pathname.split('/').at(-1);
        assert.ok(conceptId && conceptId !== 'create');
        let saved = await runtime.rows();
        const concepts = saved.filter(row => row.key.startsWith('concepts/'));
        assert.equal(concepts.length, 1);
        const item = JSON.parse(saved.find(row => row.key.startsWith('materials/')).value);
        const initial = record(saved, `material_revisions/${item.current_revision_id}`);
        // HTML textarea serialization uses CRLF for line endings. Compare exact
        // submitted native-form bytes, not an assumed LF-normalized draft.
        assert.equal(initial.text, TEXT.replace(/\r?\n/g, '\r\n'));
        await page.getByRole('heading', {name: 'Materials', exact: true}).waitFor();
        await page.getByText('No quizzes/questions yet.', {exact: false}).waitFor();
        assert.equal(await page.locator(`form[action="/concepts/${conceptId}/practice"]`).count(), 0);
        assert.equal(await page.locator('main img, main script').count(), 0);
        assert.equal(await page.locator('main a[href^="https://example.invalid"]').count(), 0);
        await layout('saved'); await page.reload(); assert.deepEqual(await runtime.rows(), saved);
        await activate(page.getByRole('link', {name: '← Your concepts', exact: true}));
        await page.locator('[data-state="concept-home"]').waitFor();
        assert.equal(await page.locator('main').getByRole('link', {name: 'Prepare material with AI', exact: true}).getAttribute('href'), '/create');
        assert.equal(await page.locator('main').getByRole('link', {name: 'Create concept', exact: true}).getAttribute('href'), '/concepts/new');
        assert.equal(await page.getByRole('link', {name: /^(Start|Resume) practice$/}).count(), 0);
        await layout('populated-home');
        await activate(page.locator(`.home-concepts a[href="/concepts/${conceptId}"]`));
        await page.locator('[data-state="concept"]').waitFor();
        assert.deepEqual(await runtime.rows(), saved, 'Populated Home is only navigation, never practice or a save.');
        const add = page.getByRole('link', {name: 'Add study material', exact: true});
        assert.equal(await add.getAttribute('href'), `/concepts/${conceptId}/materials/new`); await activate(add);
        form = await fill('/materials/add', {title: 'Independent material', text: 'Another exact reference.\n'});
        assert.equal(await form.locator('input[name=concept_id]').inputValue(), conceptId);
        await layout('add-form');
        await activate(page.getByRole('link', {name: 'Cancel', exact: true})); await page.locator('[data-state="concept"]').waitFor();
        assert.deepEqual(await runtime.rows(), saved);
        await activate(page.getByRole('link', {name: 'Add study material', exact: true}));
        form = await fill('/materials/add', {title: 'Independent material', text: 'Another exact reference.\n'});
        if (js) {
          let dropped = false;
          await context.route(runtime.url + '/materials/add', async route => {
            if (route.request().method() !== 'POST' || dropped) return route.continue();
            dropped = true; const response = await route.fetch(); assert.ok(response.ok()); await route.abort('failed');
          });
          await activate(form.getByRole('button', {name: 'Save', exact: true}));
          await page.getByText('The response did not arrive. Your draft is still here.', {exact: true}).waitFor();
          saved = await runtime.rows(); assert.equal(saved.filter(row => row.key.startsWith('materials/')).length, 2);
          await shot('committed-response-lost');
          await activate(page.getByRole('button', {name: 'Reconcile saved change', exact: true}));
          await page.locator('[data-state="concept"]').waitFor();
          assert.deepEqual(await runtime.rows(), saved, 'Frozen operation reconciles once after commit, without duplicate Material.');
          await context.unroute(runtime.url + '/materials/add');
        } else {
          await activate(form.getByRole('button', {name: 'Save', exact: true})); await page.locator('[data-state="concept"]').waitFor();
        }
        saved = await runtime.rows(); assert.equal(saved.filter(row => row.key.startsWith('materials/')).length, 2);
        const revise = page.locator(`[data-material-id="${item.id}"]`).getByRole('link', {name: 'Revise material', exact: true});
        assert.equal(await revise.getAttribute('href'), `/materials/${item.id}/edit`); await activate(revise);
        form = await fill('/materials/revise', {title: 'Browser revision', text: REVISED});
        assert.equal(await form.locator('input[name=material_id]').inputValue(), item.id);
        assert.equal(await form.locator('input[name=expected_revision_id]').inputValue(), item.current_revision_id);
        await layout('revise-form');
        await activate(page.getByRole('link', {name: 'Cancel', exact: true})); await page.locator('[data-state="concept"]').waitFor();
        assert.deepEqual(await runtime.rows(), saved);
        await activate(page.locator(`[data-material-id="${item.id}"]`).getByRole('link', {name: 'Revise material', exact: true}));
        form = await fill('/materials/revise', {title: 'Browser revision', text: REVISED});
        await activate(form.getByRole('button', {name: 'Save', exact: true})); await page.locator('[data-state="concept"]').waitFor();
        let revisedRows = await runtime.rows();
        const latest = record(revisedRows, `materials/${item.id}`);
        assert.equal(record(revisedRows, `material_revisions/${latest.current_revision_id}`).text, REVISED.replace(/\r?\n/g, '\r\n'));
        assert.deepEqual(record(revisedRows, `material_revisions/${item.current_revision_id}`), initial);
        const article = page.locator(`[data-material-id="${item.id}"]`);
        await activate(article.locator('summary').filter({hasText: 'Earlier revisions'}));
        await article.locator(`[data-material-revision-id="${item.current_revision_id}"]`).waitFor();
        await layout('earlier-revisions'); await page.reload(); assert.deepEqual(await runtime.rows(), revisedRows);
        await page.goBack();
        // Enhanced history deliberately reloads on popstate. Wait for the
        // restored editor before requesting Forward, rather than interrupting
        // that privacy reload with a second concurrent navigation.
        await page.locator('form[action="/materials/revise"]').waitFor();
        assert.deepEqual(await runtime.rows(), revisedRows, 'Back to the editor never resubmits a revision.');
        await page.goForward(); await page.locator('[data-state="concept"]').waitFor();
        assert.deepEqual(await runtime.rows(), revisedRows, 'Back/Forward and refresh do not resubmit the revision.');
        if (js) {
          await activate(page.locator(`[data-material-id="${item.id}"]`).getByRole('link', {name: 'Revise material', exact: true}));
          form = await fill('/materials/revise', {title: 'Unsent competing draft', text: 'Do not overwrite the other tab.\n'});
          const staleOperation = await form.locator('input[name=operation_id]').inputValue();
          const other = await context.newPage();
          try {
            await other.goto(runtime.url + `/materials/${item.id}/edit`);
            const otherForm = other.locator('form[action="/materials/revise"]');
            await otherForm.locator('[name=title]').fill('Other tab accepted');
            await otherForm.locator('[name=text]').fill('The other tab wrote first.\n');
            await otherForm.getByRole('button', {name: 'Save', exact: true}).tap();
            await other.locator('[data-state="concept"]').waitFor();
            revisedRows = await runtime.rows();
            await activate(form.getByRole('button', {name: 'Save', exact: true}));
            await page.getByText('This changed in another tab. Your draft is still here.', {exact: true}).waitFor();
            assert.equal(await form.locator('[name=text]').inputValue(), 'Do not overwrite the other tab.\n');
            assert.equal(await form.locator('[name=operation_id]').inputValue(), staleOperation);
            assert.deepEqual(await runtime.rows(), revisedRows, 'A stale visible draft cannot overwrite another tab or create a receipt.');
            await shot('stale-revision-draft');
            await activate(page.getByRole('button', {name: 'Open saved progress', exact: true}));
            await page.locator('form[action="/materials/revise"] [name=title]').waitFor();
            assert.equal(await page.locator('[name=title]').inputValue(), 'Other tab accepted');
            await activate(page.getByRole('link', {name: 'Cancel', exact: true}));
            await page.locator('[data-state="concept"]').waitFor();
          } finally {await other.close();}
          assert.deepEqual(await runtime.rows(), revisedRows);
        }
        const state = meta(revisedRows);
        assert.equal(state.occurrence, null); assert.equal(state.practice?.current ?? null, null);
        assert.equal(state.restored_paused, paused, 'Free authoring cannot resume external work.');
        for (const prefix of ['goals/', 'jobs/', 'questions/', 'events/', 'spend/', 'practice_attempts/']) assert.equal(revisedRows.filter(row => row.key.startsWith(prefix)).length, 0);
        assert.deepEqual(errors, []);
        assert.equal(harness.egressAttempts, 0, 'Native authoring never opens supplied links or starts provider work.');
      } catch (error) {
        result.status = 'fail'; result.reason = error.message; console.error(`${name}: ${error.message}`);
        try {await shot('failure');} catch { /* Preserve original failure if the page closed. */ }
      } finally {await context.close(); await runtime.close(); console.log(`Finished synthetic browser case: ${name}: ${result.status}`);}
    }
  } finally {await harness.close(); await rm(temp, {recursive: true, force: true});}
  results.push(...await materialProtectionWalk(browser, capture));
  return results;
}

async function materialProtectionWalk(browser, capture) {
  const {phoneFixture, phoneText} = await import('./phone-stream.mjs');
  const results = [];
  for (const js of [false, true]) {
    const name = `material-protected-history-${js ? 'enhanced' : 'no-js'}`;
    const result = {name, javascript: js, status: 'pass', evidence: [],
      scope: 'Real existing exact-Study setup and CLI-created Materials; no fixture injection or automatic assistance.'};
    results.push(result);
    let fixture, context, page;
    const shot = async (target, suffix) => {
      await target.evaluate(() => window.scrollTo(0, 0));
      result.evidence.push(await capture(target, `${name}-${suffix}.png`));
    };
    try {
      fixture = await phoneFixture();
      // Explicitly answer the existing synthetic Prompt before authoring.
      // This setup is separate from the authoring-only no-practice assertions.
      await fixture.cli(['answer', fixture.prompt.id, '--text-stdin'], phoneText);
      const created = await fixture.cli(['concept', 'create', '--name', 'Protected reference concept',
        '--title', 'PRIVATE_MATERIAL_OLD_TITLE', '--stdin'], TEXT);
      const conceptId = created.result.entity_id;
      const shown = await fixture.cli(['concept', 'show', conceptId]);
      const supplied = shown.materials.find(value => value.kind === 'supplied').material;
      await fixture.cli(['material', 'revise', supplied.id, '--expected-revision-id', supplied.current_revision_id,
        '--title', 'PRIVATE_MATERIAL_NEW_TITLE', '--stdin'], REVISED);
      context = await browser.newContext({viewport: {width: 390, height: 844}, colorScheme: 'dark',
        javaScriptEnabled: js, isMobile: true, hasTouch: true});
      page = await context.newPage(); page.setDefaultTimeout(12000);
      const oldPath = `/materials/${supplied.id}/revisions/${supplied.current_revision_id}`;
      await page.goto(fixture.origin + oldPath);
      await page.getByRole('heading', {name: 'PRIVATE_MATERIAL_OLD_TITLE', exact: true}).waitFor();
      const staleRead = await context.newPage(), editor = await context.newPage();
      await staleRead.goto(fixture.origin + oldPath);
      await staleRead.getByRole('heading', {name: 'PRIVATE_MATERIAL_OLD_TITLE', exact: true}).waitFor();
      await editor.goto(fixture.origin + `/materials/${supplied.id}/edit`);
      const form = editor.locator('form[action="/materials/revise"]');
      await form.waitFor();
      assert.equal(await form.locator('[name=title]').inputValue(), 'PRIVATE_MATERIAL_NEW_TITLE');
      await form.locator('[name=text]').fill('UNSENT_PRIVATE_MATERIAL_DRAFT');
      const next = await fixture.cli(['next', '--after', fixture.prompt.id, '--card', fixture.cid]);
      assert.equal(next.practice.prompt.assisted, false);
      const protectedRows = await fixture.rows();
      assert.equal(protectedRows.meta.practice.current.attempt_id, null);
      const gate = async target => {
        await target.locator('[data-state="material-gate"]').waitFor();
        const text = await target.locator('body').textContent();
        for (const privateText of [TEXT, REVISED, 'PRIVATE_MATERIAL_OLD_TITLE', 'PRIVATE_MATERIAL_NEW_TITLE', 'UNSENT_PRIVATE_MATERIAL_DRAFT']) {
          assert.ok(!text.includes(privateText.replace(/\r\n?/g, '\n')), 'Protected Material pages do not keep reference or draft payloads in the DOM.');
        }
        assert.equal(await target.locator('textarea, [data-material-revision-id], form[action="/review/help"]').count(), 0);
        assert.deepEqual(await fixture.rows(), protectedRows, 'A Material gate cannot record assistance, exposure, a revision or a receipt.');
      };
      await page.locator('footer').getByRole('link', {name: 'Text practice', exact: true}).tap();
      await page.locator('[data-state="exact-prompt"]').waitFor();
      await page.goBack(); await gate(page); await shot(page, 'old-revision-back-gate');
      await page.goForward(); await page.locator('[data-state="exact-prompt"]').waitFor();
      assert.equal(await page.locator('[data-state="exact-prompt"]').getAttribute('data-prompt-id'), next.practice.prompt.id);
      await page.goBack(); await gate(page);
      await staleRead.reload(); await gate(staleRead);
      await form.getByRole('button', {name: 'Save', exact: true}).tap();
      await gate(editor); await shot(editor, 'stale-editor-protected');
      await editor.reload(); await gate(editor);
      assert.equal(fixture.egress, 0);
    } catch (error) {
      result.status = 'fail'; result.reason = error.message; console.error(`${name}: ${error.message}`);
      if (page) try {await shot(page, 'failure');} catch { /* Preserve original failure. */ }
    } finally {await context?.close(); await fixture?.close();}
  }
  return results;
}
