// Phase A compatibility, not a production authoring API. Every learner record
// comes from the typed/validated Rust fixture; only isolated workerd state is used.
import assert from 'node:assert/strict';
import {createHash, randomUUID} from 'node:crypto';
import {execFileSync, spawn} from 'node:child_process';
import {mkdtemp, mkdir, readFile, writeFile, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import {unzipSync} from 'fflate';
import {createRuntime, internalKey} from './runtime-helper.mjs';
import {materialBrowserJourney} from './materials-browser.mjs';

const FORMAT = 'scry-rust-durable-sqlite-materials-v1';
const MARKER = 'material_capability/v1';
// Immediately preceding published/promoted release, including the #394 repair.
// Published ZIP identity: https://github.com/misty-step/scry/pull/398#issuecomment-6388496739
const OLD = Object.freeze({
  release: 'v0.8.1',
  url: 'https://github.com/misty-step/scry/releases/download/v0.8.1/scry-worker.zip',
  source: '62a7716e6f619c10fbda60630d86461ede33d8b7',
  tree: '492a98237708f6dc3a50368cd5e1b123e0547f2c',
  sourceDigest: '0a167c12e28116e8bac720c1c493896b2749bde23757e2cd4df2da2505471ad8',
  bytes: 97855915,
  zip: '7dbd628db9ddffff629b58459cb1f7d6c24fc34a7f7ed51723cc102e852715f8',
  js: '18f375bc666a3c61a2cd91650be8e7b46f8bcc4239b4975e60eb70200afc5c7e',
  wasm: 'b0271b77b3c439e0b3957342e4f3da25dfacd996a4223786b939582bae1d70f8',
});
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const clone = value => structuredClone(value);
const emptyBackup = () => ({completed_ms: null, key: null, sha256: null, error: null});
const escapeHtml = text => text.replaceAll('&', '&amp;').replaceAll('<', '&lt;')
  .replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&#39;');

// The rendered Material contains nested <section> elements. A non-greedy
// section regex would stop after the first revision and miss leaked controls.
function htmlElement(html, tag, attribute, value) {
  const escaped = value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const opening = new RegExp(`<${tag}\\b[^>]*\\b${attribute}="${escaped}"[^>]*>`).exec(html);
  assert.ok(opening, `Expected ${tag}[${attribute}="${value}"].`);
  const tokens = new RegExp(`<\\/?${tag}\\b[^>]*>`, 'g');
  tokens.lastIndex = opening.index;
  let depth = 0, token;
  while ((token = tokens.exec(html))) {
    depth += token[0].startsWith('</') ? -1 : 1;
    if (depth === 0) return html.slice(opening.index, tokens.lastIndex);
  }
  assert.fail(`Unclosed ${tag}[${attribute}="${value}"].`);
}

function formFields(html, action) {
  const form = htmlElement(html, 'form', 'action', action);
  return Object.fromEntries([...form.matchAll(/<input\b[^>]*name="([^"]+)"[^>]*value="([^"]*)"[^>]*>/g)]
    .map(match => [match[1], match[2]]));
}

function assertStudyMaterials(html, app, conceptId) {
  const study = htmlElement(html, 'section', 'aria-label', 'Study materials');
  assert.match(study, /^<section class="concept-study"/);
  assert.doesNotMatch(study, /<(?:form|button|input|textarea)\b/, 'Study materials has no authoring controls.');
  const materials = Object.values(app.materials.items).filter(material => material.concept_id === conceptId);
  assert.deepEqual([...study.matchAll(/data-material-id="([^"]+)"/g)].map(match => match[1]).sort(),
    materials.map(material => material.id).sort());
  const notes = app.concepts[conceptId].notes;
  assert.equal([...study.matchAll(/aria-label="Saved explanation"/g)].length, notes.length ? 1 : 0,
    'A legacy Note revision chain remains one read-only projection, not multiple Materials.');
  for (const material of materials) {
    const article = htmlElement(study, 'article', 'data-material-id', material.id);
    const revisions = Object.values(app.materials.revisions)
      .filter(revision => revision.material_id === material.id).sort((a, b) => a.ordinal - b.ordinal);
    const earlier = revisions.filter(revision => revision.id !== material.current_revision_id);
    assert.deepEqual([...article.matchAll(/data-material-revision-id="([^"]+)"/g)].map(match => match[1]),
      [material.current_revision_id, ...earlier.map(revision => revision.id)],
      'Current wording appears first; earlier immutable revisions retain their own identities.');
    for (const revision of revisions) {
      const rendered = htmlElement(article, 'section', 'data-material-revision-id', revision.id);
      assert.ok(rendered.includes(`<h3>${escapeHtml(revision.title)}</h3>`));
      assert.ok(rendered.includes(`<div class="reading preserve">${escapeHtml(revision.text)}</div>`),
        'Rendered supplied text preserves exact UTF-8, whitespace, CRLF and trailing newlines.');
    }
    if (earlier.length) {
      const details = htmlElement(article, 'details', 'class', 'saved-input');
      assert.ok(details.includes('<summary>Earlier revisions</summary>'));
      assert.deepEqual([...details.matchAll(/data-material-revision-id="([^"]+)"/g)].map(match => match[1]),
        earlier.map(revision => revision.id));
    } else {
      assert.doesNotMatch(article, /<summary>Earlier revisions<\/summary>/);
    }
  }
  return study;
}

async function frozenWorker(directory) {
  // An override is a local copy of these exact published bytes, never a rebuild.
  let bytes;
  if (process.env.SCRY_MATERIAL_OLD_WORKER_ZIP) {
    bytes = await readFile(resolve(process.env.SCRY_MATERIAL_OLD_WORKER_ZIP));
  } else {
    const response = await fetch(OLD.url, {signal: AbortSignal.timeout(120000)});
    assert.equal(response.status, 200, 'The pinned public release must be available; do not skip downgrade proof.');
    bytes = Buffer.from(await response.arrayBuffer());
  }
  assert.equal(bytes.length, OLD.bytes, 'Frozen release ZIP size.');
  assert.equal(hash(bytes), OLD.zip, 'Frozen release ZIP SHA-256.');
  const wanted = new Set(['source.json', 'worker/index.js', 'worker/index_bg.wasm']);
  const entries = unzipSync(bytes, {filter: entry => wanted.has(entry.name)});
  assert.deepEqual(Object.keys(entries).sort(), [...wanted].sort());
  const source = JSON.parse(Buffer.from(entries['source.json']).toString('utf8'));
  assert.equal(source.git_revision, OLD.source);
  assert.equal(source.revision, OLD.source);
  assert.equal(source.git_tree, OLD.tree);
  assert.equal(source.source_sha256, OLD.sourceDigest);
  assert.equal(source.source_state, 'committed');
  assert.equal(hash(entries['worker/index.js']), OLD.js);
  assert.equal(hash(entries['worker/index_bg.wasm']), OLD.wasm);
  await mkdir(directory);
  await writeFile(join(directory, 'index.js'), entries['worker/index.js']);
  await writeFile(join(directory, 'index_bg.wasm'), entries['worker/index_bg.wasm']);
  return directory;
}

// The only permitted restore differences are spelled out, not deleted wholesale
// from the comparison. All history, custody, wording and provenance remain exact.
function expectedRestored(archive, actual, space, revision) {
  const expected = clone(archive.app);
  assert.match(actual.csrf, /^[a-f0-9]{48}$/);
  assert.notEqual(actual.csrf, archive.app.csrf);
  expected.csrf = actual.csrf;
  expected.revision = revision; // Explicit current/frozen compatibility contract, never normalized away.
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
  assert.ok(Object.values(expected.assessments).every(a => !['sent', 'queued'].includes(a.status)),
    'This fixture contains settled assessments; do not silently normalize assessment history.');
  for (const [id, goal] of Object.entries(expected.goals)) {
    if (!goal.photo) continue;
    const key = actual.goals[id].photo.key;
    assert.match(key, new RegExp(`^photos/${space}/${goal.photo.sha256}-[a-f0-9]{48}$`));
    assert.notEqual(key, goal.photo.key);
    goal.photo.key = key;
  }
  assert.deepEqual(actual, expected, 'Restore retains every field except the enumerated operational differences.');
  return expected;
}

function assertRowsExcept(before, after, allowed, message) {
  const a = Object.fromEntries(before.map(row => [row.key, row.value]));
  const b = Object.fromEntries(after.map(row => [row.key, row.value]));
  for (const key of new Set([...Object.keys(a), ...Object.keys(b)])) {
    if (!allowed.has(key)) assert.equal(b[key], a[key], `${message}: ${key}`);
  }
}

function assertSettledRestoreRows(sourceRows, actualRows, restoredApp) {
  const stored = Object.fromEntries(actualRows.map(row => [row.key, row.value]));
  const expected = clone(sourceRows);
  assert.deepEqual(Object.keys(stored).sort(), Object.keys(expected).sort(), 'Restore keeps every historical row.');
  const meta = JSON.parse(expected.meta);
  meta.csrf = restoredApp.csrf;
  meta.revision = restoredApp.revision;
  meta.restored_paused = true;
  meta.backup = emptyBackup();
  assert.deepEqual(JSON.parse(stored.meta), meta, 'Only the enumerated restore metadata changes.');
  delete stored.meta; delete expected.meta;
  for (const [id, goal] of Object.entries(restoredApp.goals)) {
    if (!goal.photo) continue;
    const key = `goals/${id}`, value = JSON.parse(expected[key]);
    value.photo.key = goal.photo.key;
    assert.deepEqual(JSON.parse(stored[key]), value, 'Only the restored photo key changes on its Goal.');
    delete stored[key]; delete expected[key];
  }
  // These fixtures have no active jobs/assessments. Even unknown spend and
  // opaque custody chunks must therefore survive as the exact original rows.
  assert.deepEqual(stored, expected, 'Every remaining historical row retains its exact serialized bytes.');
}

export async function materialJourney({artifact: floorArtifact, cliWriter = false} = {}) {
  const temp = await mkdtemp(join(tmpdir(), 'scry-materials-'));
  const active = new Set(), transcript = [];
  const close = async () => {
    await Promise.all([...active].map(runtime => runtime.close()));
    await rm(temp, {recursive: true, force: true});
  };
  try {
    const fixturePath = join(temp, 'typed-fixture.json');
    execFileSync('cargo', ['test', '--locked', '--test', 'materials_history',
      'export_material_fixture', '--', '--ignored', '--exact'], {
      env: {...process.env, SCRY_MATERIAL_FIXTURE_OUTPUT: fixturePath}, stdio: 'pipe',
    });
    const fixture = JSON.parse(await readFile(fixturePath, 'utf8'));
    const {archive, baseline_archive: baseline, ids, owner_token_secret: secret} = fixture;
    assert.equal(archive.format, FORMAT);
    assert.equal(fixture.rows[MARKER], 'true');
    assert.equal(fixture.marker_archive.format, FORMAT);
    assert.equal(fixture.marker_rows[MARKER], 'true');
    assert.equal(Object.keys(archive.app.materials.items).length, 2);
    assert.equal(Object.keys(archive.app.materials.revisions).length, 3);
    assert.ok(!Object.hasOwn(archive.app.concepts[ids.authored_concept], 'goal_id'));
    assert.equal(archive.app.concepts[ids.authored_concept].origin, 'owner-authored');
    assert.equal(secret.slice(8, 40), ids.owner_token);
    assert.equal(hash(secret), archive.app.owner_tokens[ids.owner_token].verifier);
    assert.ok(archive.photos.length > 0, 'Complete archive includes real photo bytes.');
    const timestamp = Math.max(Date.now(), archive.created_ms) + 60000;
    const restoreToken = 'synthetic-materials-restore-only-capability-32chars';
    const binary = resolve(process.env.SCRY_MATERIAL_CLI || 'target/debug/scry');

    async function open(label, artifact = floorArtifact) {
      const name = `materials-${label}-${randomUUID()}`, space = `materials-${randomUUID()}`;
      let bindings = {SCRY_ENV: 'restore', CANONICAL_ORIGIN: 'http://127.0.0.1:0',
        SPACE_NAME: space, INTERNAL_KEY: internalKey, RESTORE_TOKEN: restoreToken,
        SCRY_TEST_NOW_MS: String(timestamp)};
      let runtime = await createRuntime({port: 0, name, inspect: true, testClock: true, artifact, bindings});
      try {
        bindings = {...bindings, CANONICAL_ORIGIN: (await runtime.mf.ready).origin};
        runtime = {...runtime, ...await runtime.rebind(bindings)};
        let storage = await runtime.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name: space});
        const config = join(temp, `${name}-login.json`);
        const context = {
          space,
          get origin() {return runtime.url;},
          artifact: resolve(artifact || process.env.SCRY_WORKER_ARTIFACT || 'build'),
          rows: async () => {
            const tables = await storage.exec("SELECT name FROM sqlite_master WHERE type='table' AND name='records'");
            return tables.length ? storage.exec('SELECT key,value FROM records ORDER BY key') : [];
          },
          bucket: name => runtime.mf.getR2Bucket(name),
          bind: async mode => {
            bindings = {...bindings, SCRY_ENV: mode};
            runtime = {...runtime, ...await runtime.rebind(bindings)};
            storage = await runtime.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name: space});
          },
          request: (path, payload) => runtime.worker.fetch(runtime.url + path, {
            ...(payload === undefined ? {} : {method: 'POST', body: JSON.stringify(payload)}),
            headers: {Authorization: `Bearer ${secret}`, 'Content-Type': 'application/json'},
          }),
          page: path => runtime.worker.fetch(runtime.url + path),
          form: (path, fields) => runtime.worker.fetch(runtime.url + path, {
            method: 'POST', redirect: 'manual', headers: {'X-Scry-Test-Origin': runtime.url, 'Content-Type': 'application/x-www-form-urlencoded'},
            body: new URLSearchParams(fields),
          }),
          restore: async value => {
            const bytes = Buffer.isBuffer(value) ? value : Buffer.from(JSON.stringify(value));
            return runtime.worker.fetch(runtime.url + '/operator/restore', {
              method: 'POST', headers: {'x-scry-restore-token': restoreToken, 'x-scry-sha256': hash(bytes)}, body: bytes,
            });
          },
          internal: async (path, {method = 'POST', bearer = false} = {}) => {
            const ns = await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE');
            return ns.get(ns.idFromName(space)).fetch('https://internal.invalid' + path, {
              method, headers: {'x-scry-internal-key': internalKey, 'x-scry-owner': 'private-owner',
                ...(bearer ? {Authorization: `Bearer ${secret}`} : {})},
            });
          },
          insert: async rows => {
            assert.deepEqual(await context.rows(), []);
            await storage.exec('CREATE TABLE records (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID');
            for (const [key, value] of Object.entries(rows)) {
              await storage.exec('INSERT INTO records(key,value) VALUES (?,?)', key, value);
            }
          },
          cli: (args, input = '') => new Promise((done, reject) => {
            const proc = spawn(binary, [...args, '--json', '--config', config], {
              env: {PATH: process.env.PATH, HOME: temp}, stdio: ['pipe', 'pipe', 'pipe'],
            });
            let out = '', err = '';
            proc.stdout.on('data', bytes => out += bytes);
            proc.stderr.on('data', bytes => err += bytes);
            proc.on('error', reject);
            proc.stdin.on('error', error => {if (error.code !== 'EPIPE') reject(error);});
            proc.on('exit', code => {
              try {
                assert.ok(!out.includes(secret) && !err.includes(secret), 'CLI never prints the login secret.');
                const value = JSON.parse(out);
                transcript.push({command: ['scry', ...args, '--json'].join(' '), exit: code,
                  output_sha256: hash(out), limit: 'Synthetic output digest; login stdin omitted.'});
                done({code, value});
              } catch (error) {reject(error);}
            });
            proc.stdin.end(input);
          }),
          login: async () => {
            const result = await context.cli(['login', '--url', runtime.url, '--token-stdin'], secret + '\n');
            assert.equal(result.code, 0, JSON.stringify(result.value));
            assert.equal(result.value.token.id, ids.owner_token);
          },
          close: async () => {active.delete(context); await runtime.mf.dispose();},
        };
        active.add(context);
        return context;
      } catch (error) {await runtime.mf.dispose(); throw error;}
    }

    async function exported(context) {
      const response = await context.request('/api/v1/export');
      assert.equal(response.status, 200, await response.clone().text());
      return response.json();
    }
    async function pageHtml(context, path) {
      const response = await context.page(path);
      assert.equal(response.status, 200, await response.clone().text());
      return response.text();
    }
    const metaFrom = rows => JSON.parse(rows.find(row => row.key === 'meta').value);
    const recordFrom = (rows, key) => JSON.parse(rows.find(row => row.key === key).value);
    async function postNative(context, path, extra = {}) {
      const meta = metaFrom(await context.rows());
      const fields = {csrf: meta.csrf, revision: String(meta.revision), operation_id: randomUUID(), ...extra};
      const response = await context.form(path, fields);
      assert.equal(response.status, 303, `${path}: ${await response.text()}`);
      return fields;
    }
    async function assertPhotos(context, original, current) {
      const bucket = await context.bucket('SCRY_ASSETS');
      const listed = await bucket.list();
      assert.equal(listed.objects.length, original.photos.length);
      for (const archived of original.photos) {
        const goal = Object.values(current.goals).find(g => g.photo?.sha256 === archived.photo.sha256);
        assert.ok(goal);
        const photo = await bucket.get(goal.photo.key);
        assert.ok(photo);
        const bytes = Buffer.from(await photo.arrayBuffer());
        assert.deepEqual(bytes, Buffer.from(archived.bytes, 'base64'));
        assert.equal(hash(bytes), archived.photo.sha256);
      }
    }
    async function buckets(context) {
      const snapshot = {};
      for (const name of ['SCRY_ASSETS', 'SCRY_BACKUPS']) {
        const bucket = await context.bucket(name), entries = [];
        for (const item of (await bucket.list()).objects) {
          entries.push({key: item.key, sha256: hash(Buffer.from(await (await bucket.get(item.key)).arrayBuffer()))});
        }
        snapshot[name] = entries.sort((a, b) => a.key.localeCompare(b.key));
      }
      return snapshot;
    }
    async function assertFreshRestore(context, source, revision = source.app.revision + 1) {
      assert.deepEqual(await context.rows(), []);
      const response = await context.restore(source);
      assert.equal(response.status, 200, await response.text());
      await context.bind('development');
      const read = await exported(context);
      expectedRestored(source, read.app, context.space, revision);
      assert.equal(read.format, source.format);
      await assertPhotos(context, source, read.app);
      return read;
    }
    async function firstSettingsWrite(context, before, operation) {
      const priorRows = await context.rows();
      const response = await context.request('/api/v1/settings', {operation_id: operation, pace: 'light'});
      assert.equal(response.status, 200, await response.clone().text());
      const result = await response.json();
      assert.equal(result.replayed, false);
      const after = await exported(context), expected = clone(before.app);
      expected.revision++;
      expected.preferences.pace = 'light';
      expected.owner_tokens[ids.owner_token].last_used_ms = timestamp;
      expected.operations[operation] = {
        acting_token: ids.owner_token, path: '/settings', created_ms: timestamp,
        payload_hash: hash(JSON.stringify(['/settings', {operation_id: operation, pace: 'light'}])),
        result: {redirect: '/settings', entity_id: null, message: null},
      };
      assert.deepEqual(result.result, expected.operations[operation].result);
      assert.deepEqual(after.app, expected, 'First unrelated write retains the entire restored graph.');
      assertRowsExcept(priorRows, await context.rows(), new Set(['meta', `owner_tokens/${ids.owner_token}`, `operations/${operation}`]),
        'First unrelated write retains all pre-existing immutable row bytes');
      return after;
    }

    let current, restored, beforeWrite, accepted, oldArtifact;
    const criteria = [
      async () => {
        current = await open('current');
        beforeWrite = await assertFreshRestore(current, archive);
        const stored = Object.fromEntries((await current.rows()).map(row => [row.key, row.value]));
        const expectedRows = clone(fixture.restored_rows);
        assert.deepEqual(Object.keys(stored).sort(), Object.keys(expectedRows).sort());
        const meta = JSON.parse(expectedRows.meta); meta.csrf = beforeWrite.app.csrf;
        assert.deepEqual(JSON.parse(stored.meta), meta);
        delete stored.meta; delete expectedRows.meta;
        for (const [id, goal] of Object.entries(beforeWrite.app.goals)) {
          if (!goal.photo) continue;
          const value = JSON.parse(expectedRows[`goals/${id}`]); value.photo.key = goal.photo.key;
          assert.deepEqual(JSON.parse(stored[`goals/${id}`]), value);
          delete stored[`goals/${id}`]; delete expectedRows[`goals/${id}`];
        }
        assert.deepEqual(stored, expectedRows, 'Typed fixture rows survive the real fresh SQLite restore.');
        accepted = await firstSettingsWrite(current, beforeWrite, 'materials-first-settings-write');
        transcript.push({observation: 'Typed mixed historical/new archive, fresh SQLite restore, first unrelated settings write',
          rows: Object.keys(fixture.rows).length, materials: 2, immutable_revisions: 3, photos: archive.photos.length,
          material_rows_sha256: hash(JSON.stringify(accepted.app.materials))});
      },
      async () => {
        const prior = await current.rows(), expected = clone(accepted.app);
        await current.login();
        expected.revision++;
        expected.owner_tokens[ids.owner_token].last_used_ms = timestamp;
        const result = await current.cli(['export']);
        assert.equal(result.code, 0);
        assert.equal(result.value.format, FORMAT);
        assert.deepEqual(result.value.app, expected, 'Compiled CLI retains every old and new export field.');
        accepted = result.value;
        assertRowsExcept(prior, await current.rows(), new Set(['meta', `owner_tokens/${ids.owner_token}`]),
          'CLI login changes only its existing token last-use and CAS metadata');
        const readRows = await current.rows();
        const history = await current.cli(['history']);
        assert.equal(history.code, 0);
        for (const field of ['studies', 'cards', 'attempts', 'corrections']) {
          assert.deepEqual(history.value[field], accepted.app.practice[field]);
        }
        assert.deepEqual(await current.rows(), readRows, 'CLI export/history do not change persisted rows.');
      },
      async () => {
        const priorRows = await current.rows(), priorBuckets = await buckets(current);
        for (const path of ['/concepts/create', '/materials/add', '/materials/revise']) {
          const retained = Object.entries(accepted.app.operations).filter(([, receipt]) => receipt.path === path);
          assert.equal(retained.length, 1, 'Each reserved path has a retained, typed future-writer receipt.');
          const [operation, receipt] = retained[0];
          const matching = {entity_id: receipt.result.entity_id, operation_id: operation, synthetic_fixture: 'future-writer'};
          assert.equal(hash(JSON.stringify([path, matching])), receipt.payload_hash,
            'The refusal probe really matches the acknowledged payload, rather than failing its hash check.');
          for (const [kind, payload] of [
            ['matching retained payload', matching],
            ['changed retained payload', {...matching, synthetic_fixture: 'changed-future-writer'}],
            ['fresh operation', {...matching, operation_id: randomUUID()}],
          ]) {
            for (const ingress of ['native', 'api']) {
              const response = ingress === 'native'
                ? await current.form(path, {...payload, csrf: accepted.app.csrf})
                : await current.request(`/api/v1${path}`, payload);
              assert.equal(response.status, 404, `${ingress} ${path}: ${kind} must refuse before generic receipt replay.`);
              await response.text();
              assert.deepEqual(await current.rows(), priorRows, 'Reserved authoring refusal cannot change any SQLite row.');
              assert.deepEqual(await buckets(current), priorBuckets, 'Reserved authoring refusal cannot change any R2 object.');
            }
          }
        }
        // Ordinary operations keep their existing replay semantics. The
        // runtime advances CAS metadata on replay, but duplicates no receipt.
        const payload = {operation_id: 'materials-first-settings-write', pace: 'light'};
        const expected = clone(accepted.app);
        const replay = await current.request('/api/v1/settings', payload);
        assert.equal(replay.status, 200, await replay.clone().text());
        const replayed = await replay.json();
        assert.equal(replayed.replayed, true);
        assert.deepEqual(replayed.result, expected.operations[payload.operation_id].result);
        expected.revision++;
        assert.deepEqual((await exported(current)).app, expected);
        const nativeReplay = await current.form('/settings', {...payload, csrf: expected.csrf});
        assert.equal(nativeReplay.status, 303, await nativeReplay.text());
        assert.equal(new URL(nativeReplay.headers.get('location'), current.origin).pathname, '/settings');
        expected.revision++;
        accepted = await exported(current);
        assert.deepEqual(accepted.app, expected, 'Ordinary native replay preserves its original receipt and all learner history.');
        assertRowsExcept(priorRows, await current.rows(), new Set(['meta']), 'Only existing replay CAS metadata changes');
        assert.deepEqual(await buckets(current), priorBuckets);
        transcript.push({observation: 'All three reserved authoring paths refuse matching retained payloads, changed payloads, and fresh operation IDs through native/API ingress without SQLite/R2 writes; ordinary settings replay remains compatible.'});
      },
      async () => {
        const priorRows = await current.rows(), priorBuckets = await buckets(current);
        for (const path of ['/api/v1/materials', '/api/v1/materials/create', '/api/v1/materials/add', '/api/v1/materials/revise',
          '/api/v1/concepts', '/api/v1/concepts/create']) {
          const response = await current.request(path, {operation_id: randomUUID(), title: 'Not an authoring route', text: 'Synthetic'});
          assert.equal(response.status, 404, path);
          assert.deepEqual(await current.rows(), priorRows, `${path} must not write even a receipt or token last-use.`);
        }
        const commands = cliWriter ? [
          ['concept', 'create', '--name', 'Not enabled by floor', '--title', 'Synthetic', '--stdin'],
          ['material', 'add', ids.authored_concept, '--title', 'Synthetic', '--stdin'],
          ['material', 'revise', Object.keys(accepted.app.materials.items)[0], '--expected-revision-id', Object.values(accepted.app.materials.items)[0].current_revision_id, '--title', 'Synthetic', '--stdin'],
        ] : [['material', 'add'], ['material', 'revise'], ['concept', 'add']];
        for (const command of commands) {
          const response = await current.cli(command, 'Synthetic input only');
          assert.equal(response.code, 1);
          assert.match(response.value.error, cliWriter ? /^API request failed\./ : /^Usage:/);
          assert.deepEqual(await current.rows(), priorRows);
        }
        for (const path of ['/materials/create', '/materials/add', '/materials/revise', '/concepts/create']) {
          const response = await current.form(path, {operation_id: randomUUID(), csrf: accepted.app.csrf, title: 'Synthetic'});
          assert.equal(response.status, 404, path);
          assert.deepEqual(await current.rows(), priorRows);
        }
        assert.deepEqual(await buckets(current), priorBuckets);
        transcript.push({observation: 'Pinned floor refuses Concept/Material authoring through API, candidate CLI and native form probes; SQLite and R2 unchanged.'});
      },
      async () => {
        const priorRows = await current.rows();
        const backed = await current.internal('/__internal/daily');
        assert.equal(backed.status, 200, await backed.text());
        const bucket = await current.bucket('SCRY_BACKUPS');
        const list = await bucket.list({prefix: `snapshots/${current.space}/`});
        assert.equal(list.objects.length, 1);
        const bytes = Buffer.from(await (await bucket.get(list.objects[0].key)).arrayBuffer());
        const backedArchive = JSON.parse(bytes);
        assert.equal(backedArchive.format, FORMAT);
        assert.deepEqual(backedArchive.app, accepted.app, 'Actual R2 archive keeps all pre-backup application fields.');
        assert.deepEqual(backedArchive.photos, accepted.photos);
        const after = await exported(current), expected = clone(accepted.app);
        expected.revision++;
        expected.backup = {completed_ms: timestamp, key: list.objects[0].key, sha256: hash(bytes), error: null};
        assert.deepEqual(after.app, expected, 'Verified backup changes only exact backup metadata and CAS revision.');
        assertRowsExcept(priorRows, await current.rows(), new Set(['meta']), 'Backup preserves every record row');
        accepted = after;
        restored = await open('r2-restored');
        const read = await assertFreshRestore(restored, backedArchive);
        const written = await firstSettingsWrite(restored, read, 'materials-restored-settings-write');
        const restoredRows = await restored.rows();
        await restored.login();
        const cli = await restored.cli(['export']);
        assert.equal(cli.code, 0);
        const restoredExpected = clone(written.app);
        restoredExpected.revision++;
        restoredExpected.owner_tokens[ids.owner_token].last_used_ms = timestamp;
        assert.deepEqual(cli.value.app, restoredExpected, 'Restored CLI retains every field after the first settings write.');
        assertRowsExcept(restoredRows, await restored.rows(), new Set(['meta', `owner_tokens/${ids.owner_token}`]),
          'Restored CLI login/export retains all immutable rows');
        transcript.push({observation: 'Complete local R2 readback and fresh namespace restore retain all fields, then survive another first settings write and compiled CLI export',
          archive_sha256: hash(bytes), limit: 'Local emulator; independent hosted recovery and activation are not established.'});
      },
      async () => {
        for (const context of [current, restored]) {
          const rows = await context.rows(), before = await buckets(context);
          await context.bind('restore');
          for (const stale of [baseline, archive]) {
            const response = await context.restore(stale);
            assert.equal(response.status, 409, await response.text());
            assert.deepEqual(await context.rows(), rows, 'No historical archive can overwrite accepted new writes.');
            assert.deepEqual(await buckets(context), before, 'Rejected restore adds no asset or backup object.');
          }
          await context.bind('development');
        }
      },
      async () => {
        const reading = await open('authored-reading');
        try {
          const before = await assertFreshRestore(reading, archive);
          const initialRows = await reading.rows(), initialBuckets = await buckets(reading);
          const home = await pageHtml(reading, '/');
          assert.match(home, /data-state="concept-home"/);
          const card = htmlElement(home, 'a', 'href', `/concepts/${ids.authored_concept}`);
          assert.ok(card.includes(escapeHtml(before.app.concepts[ids.authored_concept].name)));
          assert.ok(card.includes('<span class="idea-status">Study material</span>'));
          assert.deepEqual(await reading.rows(), initialRows, 'Concept home starts neither practice nor fabricated exposure.');
          for (let i = 0; i < 2; i++) {
            const html = await pageHtml(reading, `/concepts/${ids.authored_concept}`);
            assertStudyMaterials(html, before.app, ids.authored_concept);
            const detail = htmlElement(html, 'article', 'data-state', 'concept');
            assert.doesNotMatch(detail, /<form\b/, 'Authored detail offers no practice, feedback, archive or Material authoring mutation.');
            assert.ok(detail.includes('No saved questions.'));
            assert.deepEqual(await reading.rows(), initialRows, 'Reading/reloading authored Materials changes no SQLite row.');
          }
          for (const action of ['practice', 'feedback', 'archive']) {
            const response = await reading.form(`/concepts/${ids.authored_concept}/${action}`, {
              csrf: before.app.csrf, operation_id: randomUUID(), text: 'Synthetic unavailable authored mutation',
            });
            assert.equal(response.status, 422, await response.text());
            assert.deepEqual(await reading.rows(), initialRows, 'Hidden authored mutation routes also refuse without writes.');
          }
          const generated = ids.generated_concepts[0];
          const html = await pageHtml(reading, `/concepts/${generated}`);
          assertStudyMaterials(html, before.app, generated);
          for (const action of ['practice', 'feedback', 'archive']) {
            htmlElement(html, 'form', 'action', `/concepts/${generated}/${action}`);
          }
          const expected = clone(before.app);
          assert.notEqual(expected.concepts[generated].exposure_ms, timestamp);
          expected.concepts[generated].exposure_ms = timestamp;
          expected.revision++;
          assert.deepEqual((await exported(reading)).app, expected,
            'Generated reference read retains its existing exposure semantics and changes nothing else.');
          assertRowsExcept(initialRows, await reading.rows(), new Set(['meta', `concepts/${generated}`]),
            'Generated reference read retains all immutable old/new row bytes');
          assert.deepEqual(await buckets(reading), initialBuckets);
          transcript.push({observation: 'Concept home and authored detail/reload preserve all rows; two supplied Materials render current and earlier exact wording with no mutation controls. Generated detail keeps its legacy projection, controls and exposure semantics.'});
        } finally {await reading.close();}
      },
      async () => {
        const reading = await open('legacy-material-assistance');
        try {
          const before = await assertFreshRestore(reading, fixture.legacy_material_archive);
          const conceptId = ids.legacy_concept, destination = `/concepts/${conceptId}`;
          const initialBuckets = await buckets(reading);
          assert.equal(before.app.concepts[conceptId].exposure_ms, null);
          // Use existing native actions, never edited SQLite or invented
          // occurrences, to make this Goal-backed Concept the next cold question.
          for (const question of Object.values(before.app.questions)) {
            if (question.concept_id !== conceptId && !question.archived) {
              await postNative(reading, `/questions/${question.id}/archive`);
            }
          }
          const held = metaFrom(await reading.rows()).occurrence;
          assert.equal(held.phase, 'result');
          await postNative(reading, '/review/next', {occurrence_id: held.id});
          const protectedRows = await reading.rows(), protectedMeta = metaFrom(protectedRows);
          assert.equal(protectedMeta.occurrence.concept_id, conceptId);
          assert.equal(protectedMeta.occurrence.phase, 'question');
          assert.equal(protectedMeta.occurrence.assisted, false);
          const gated = await pageHtml(reading, destination);
          assert.match(gated, /data-state="gate"/);
          assert.doesNotMatch(gated, /data-material-id=|data-material-revision-id=|aria-label="Study materials"|aria-label="Saved explanation"/);
          for (const revision of Object.values(before.app.materials.revisions)) {
            assert.ok(!gated.includes(escapeHtml(revision.title)));
            assert.ok(!gated.includes(escapeHtml(revision.text)));
          }
          for (const paragraph of before.app.concepts[conceptId].notes.at(-1).text.split('\n\n').filter(text => text.trim())) {
            assert.ok(!gated.includes(escapeHtml(paragraph)), 'The existing generated explanation remains behind assistance too.');
          }
          const deniedExport = await reading.request('/api/v1/export');
          assert.equal(deniedExport.status, 409);
          await deniedExport.text();
          assert.deepEqual(await reading.rows(), protectedRows, 'Protected reference and export refusal record no exposure or other write.');
          const helpFields = formFields(gated, '/review/help');
          assert.equal(helpFields.return_to, destination);
          const helped = await reading.form('/review/help', helpFields);
          assert.equal(helped.status, 303, await helped.text());
          const helpedRows = await reading.rows(), expectedMeta = clone(protectedMeta);
          expectedMeta.revision++;
          expectedMeta.occurrence.assisted = true;
          assert.deepEqual(metaFrom(helpedRows), expectedMeta, 'Assistance is persisted before reference content is returned.');
          const expectedConcept = recordFrom(protectedRows, `concepts/${conceptId}`);
          expectedConcept.exposure_ms = timestamp;
          assert.deepEqual(recordFrom(helpedRows, `concepts/${conceptId}`), expectedConcept);
          assert.equal(recordFrom(helpedRows, `operations/${helpFields.operation_id}`).path, '/review/help');
          assertRowsExcept(protectedRows, helpedRows, new Set(['meta', `concepts/${conceptId}`, `operations/${helpFields.operation_id}`]),
            'Native assistance preserves all prior history and Material rows');
          const html = await pageHtml(reading, destination);
          assertStudyMaterials(html, before.app, conceptId);
          for (const action of ['practice', 'feedback', 'archive']) {
            htmlElement(html, 'form', 'action', `${destination}/${action}`);
          }
          assert.deepEqual(await reading.rows(), helpedRows, 'Allowed reference read adds no second exposure at the same clock.');
          assert.ok(html.includes(`href="/goals/${before.app.concepts[conceptId].goal_id}"`),
            'The original material/preparation navigation remains available.');
          assert.deepEqual(await buckets(reading), initialBuckets);
          transcript.push({observation: 'A Material on an unchanged Goal-backed Concept obeys the real unaided assistance fence; native Look it up preserves history and the original generated controls/navigation.'});
        } finally {await reading.close();}
      },
      async () => {
        oldArtifact = await frozenWorker(join(temp, `frozen-${OLD.release}`));
        const reading = await open('authored-native-browser');
        let previous;
        try {
          const before = await assertFreshRestore(reading, archive);
          previous = await open('predecessor-browser-baseline', oldArtifact);
          await assertFreshRestore(previous, baseline);
          const report = await materialBrowserJourney({
            origin: reading.origin, artifact: reading.artifact, app: before.app, conceptId: ids.authored_concept,
            readRows: reading.rows, readBuckets: () => buckets(reading),
            predecessor: {origin: previous.origin, artifact: previous.artifact,
              readRows: previous.rows, readBuckets: () => buckets(previous), release: OLD.release},
          });
          transcript.push({observation: 'Actual Chromium Home/detail/reload and native disclosure preserve authored Material wording, immutable history, exposure, SQLite and R2 across phone/desktop light/dark and 200% computed text sizing.',
            report: report.path, variants: report.variants, layout_status: report.layout_status,
            limit: 'Synthetic local browser emulation, not physical-phone or hosted acceptance; inherited Home overflow is reported separately from new Material layout.'});
        } finally {await previous?.close(); await reading.close();}
      },
      async () => {
        if (!oldArtifact) oldArtifact = await frozenWorker(join(temp, `frozen-${OLD.release}`));
        const old = await open('old-baseline-control', oldArtifact);
        try {
          const first = await assertFreshRestore(old, baseline);
          const firstRows = await old.rows();
          assertSettledRestoreRows(fixture.baseline_rows, firstRows, first.app);
          const exportedAgain = await exported(old);
          assert.deepEqual(exportedAgain, first, 'Frozen predecessor export retains the complete restored baseline.');
          assert.deepEqual(await old.rows(), firstRows, 'Frozen predecessor export is read-only.');
          const backed = await old.internal('/__internal/daily');
          assert.equal(backed.status, 200, await backed.text());
          const bucket = await old.bucket('SCRY_BACKUPS');
          const list = await bucket.list({prefix: `snapshots/${old.space}/`});
          assert.equal(list.objects.length, 1);
          const bytes = Buffer.from(await (await bucket.get(list.objects[0].key)).arrayBuffer());
          const backedArchive = JSON.parse(bytes);
          assert.deepEqual(backedArchive, first, 'Actual predecessor R2 readback retains every archive field and photo byte.');
          const after = await exported(old), expected = clone(first.app);
          expected.revision++;
          expected.backup = {completed_ms: timestamp, key: list.objects[0].key, sha256: hash(bytes), error: null};
          assert.deepEqual(after.app, expected, 'Predecessor backup changes only verified backup metadata and revision.');
          assertRowsExcept(firstRows, await old.rows(), new Set(['meta']), 'Predecessor backup retains every historical row');
          await assertPhotos(old, first, after.app);

          const recovered = await open('old-baseline-r2-restored', oldArtifact);
          try {
            const second = await assertFreshRestore(recovered, backedArchive);
            assert.equal(second.app.revision, baseline.app.revision + 2,
              'Each fresh restore advances the archived revision exactly once, never resetting immutable sequence history.');
            assertSettledRestoreRows(fixture.baseline_rows, await recovered.rows(), second.app);
            assert.deepEqual(await buckets(recovered), {
              SCRY_ASSETS: second.photos.map(photo => ({key: photo.photo.key, sha256: hash(Buffer.from(photo.bytes, 'base64'))}))
                .sort((a, b) => a.key.localeCompare(b.key)),
              SCRY_BACKUPS: [],
            }, 'Fresh baseline recovery contains exactly its restored photo bytes and no cloned backup.');
          } finally {await recovered.close();}
          transcript.push({observation: 'Frozen immediately preceding release accepts the complete historical baseline at source revision +1, exports and backs it up to local R2, then restores the readback into a second fresh namespace with every historical row retained',
            release: OLD.release, source: OLD.source, tree: OLD.tree, source_sha256: OLD.sourceDigest,
            zip_sha256: OLD.zip, worker_sha256: OLD.js, wasm_sha256: OLD.wasm, archive_sha256: hash(bytes),
            limit: 'Local emulator; independent hosted recovery and activation are not established.'});
        } finally {await old.close();}
      },
      async () => {
        assert.ok(oldArtifact, 'Frozen old artifact verification must succeed before downgrade probes.');
        const old = await open('old-restore-refusal', oldArtifact);
        try {
          for (const [label, candidate] of [['mixed historical/new', archive], ['marker-only', fixture.marker_archive],
            ['Material on unchanged Goal-backed Concept', fixture.legacy_material_archive]]) {
            const response = await old.restore(candidate);
            assert.equal(response.status, 422, `${label}: ${await response.text()}`);
            assert.deepEqual(await old.rows(), [], 'Old archive refusal leaves SQLite empty.');
            assert.deepEqual(await buckets(old), {SCRY_ASSETS: [], SCRY_BACKUPS: []},
              'Old archive refusal happens before any R2 write.');
          }
        } finally {await old.close();}
      },
      async () => {
        assert.ok(oldArtifact, 'Frozen old artifact verification must succeed before downgrade probes.');
        for (const [label, rows, source] of [['mixed', fixture.rows, archive],
          ['marker-only', fixture.marker_rows, fixture.marker_archive],
          ['legacy-material', fixture.legacy_material_rows, fixture.legacy_material_archive]]) {
          const old = await open(`old-row-${label}`, oldArtifact);
          try {
            await old.insert(rows);
            const assets = await old.bucket('SCRY_ASSETS');
            for (const photo of source.photos) await assets.put(photo.photo.key, Buffer.from(photo.bytes, 'base64'));
            const priorRows = await old.rows(), priorBuckets = await buckets(old);
            await old.bind('development');
            await assert.rejects(old.internal('/api/v1/export', {method: 'GET', bearer: true}),
              /Unknown storage record\./, 'The actual old row reader must fail on the capability, not missing photos or provider access.');
            assert.deepEqual(await old.rows(), priorRows);
            assert.deepEqual(await buckets(old), priorBuckets);
            for (const [path, payload] of [['/api/v1/export', undefined],
              ['/api/v1/settings', {operation_id: 'old-material-write-must-refuse', pace: 'intense'}]]) {
              const response = await old.request(path, payload);
              assert.equal(response.status, 500, `${label} ${path}: ${await response.text()}`);
              assert.deepEqual(await old.rows(), priorRows, 'Old row-reader failure cannot rewrite or drop valid new records.');
              assert.deepEqual(await buckets(old), priorBuckets);
            }
          } finally {await old.close();}
        }
        transcript.push({observation: `Exact ${OLD.release} frozen reader refuses valid mixed, marker-only and Goal-backed Material archives/rows; SQLite and R2 unchanged on reads and attempted unrelated writes.`});
      },
    ];
    return {criteria, transcript, close};
  } catch (error) {await close(); throw error;}
}
