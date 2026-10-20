import assert from 'node:assert/strict';
import test from 'node:test';
import {spawn} from 'node:child_process';
import {createHash, randomUUID} from 'node:crypto';
import {mkdir, mkdtemp, rm, writeFile} from 'node:fs/promises';
import {join, resolve} from 'node:path';
import {createRuntime, internalKey} from './runtime-helper.mjs';

const text = 'Copper lanterns shelter quiet gardens.';
const secondText = 'Amber streams carry patient wishes.';
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const observations = [];
const cliPath = resolve(process.env.SCRY_STUDY_CLI || 'target/debug/scry');

async function fixture(t, twoUnits = false) {
  const temp = await mkdtemp(join(process.env.HOME, '.cache/tmp/study-integrity-'));
  const config = join(temp, 'login.json');
  const name = 'study-integrity-' + randomUUID();
  const bindings = {SCRY_ENV: 'development', CANONICAL_ORIGIN: 'http://127.0.0.1:0', SPACE_NAME: name, INTERNAL_KEY: internalKey};
  let egress = 0;
  let runtime = await createRuntime({port: 0, name, inspect: true, bindings, egress: async () => {egress++; throw Error('No egress permitted.');}});
  const origin = (await runtime.mf.ready).origin;
  runtime = {...runtime, ...await runtime.rebind({...bindings, CANONICAL_ORIGIN: origin})};
  t.after(async () => {await runtime.mf.dispose(); await rm(config, {force: true}); assert.equal(egress, 0);});
  const sql = await runtime.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name});
  const rows = () => sql.exec('SELECT key,value FROM records ORDER BY key');
  const record = async key => JSON.parse((await sql.exec('SELECT value FROM records WHERE key=?', key))[0].value);
  const html = await (await runtime.worker.fetch(origin + '/settings')).text();
  const form = /<form[^>]*action="\/tokens\/create"[^>]*>([\s\S]*?)<\/form>/.exec(html);
  const fields = Object.fromEntries([...form[1].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(m => [m[1], m[2]]));
  const issued = await runtime.worker.fetch(origin + '/tokens/create', {method: 'POST', headers: {'X-Scry-Test-Origin': origin, 'Content-Type': 'application/x-www-form-urlencoded'}, body: new URLSearchParams({...fields, name: 'Study integrity fixture'})});
  const secret = /<textarea[^>]*>([^<]+)<\/textarea>/.exec(await issued.text())[1];
  const request = (path, payload) => runtime.worker.fetch(origin + path, {redirect: 'manual', ...(payload ? {method: 'POST', body: JSON.stringify(payload)} : {}), headers: {Authorization: 'Bearer ' + secret, 'Content-Type': 'application/json'}});
  const api = async (path, payload, status = 200) => {
    const response = await request(path, payload);
    const body = await response.json();
    assert.equal(response.status, status, JSON.stringify(body));
    return body;
  };
  const cli = (args, input = '', exit = 0) => new Promise((done, reject) => {
    const child = spawn(cliPath, [...args, '--json', '--config', config], {env: {PATH: process.env.PATH, HOME: temp}, stdio: ['pipe', 'pipe', 'pipe']});
    let stdout = '', stderr = '';
    child.stdout.on('data', b => stdout += b);
    child.stderr.on('data', b => stderr += b);
    child.on('error', reject);
    child.on('exit', code => {try {
      assert.ok(!stdout.includes(secret) && !stderr.includes(secret), 'CLI never prints the credential.');
      const body = JSON.parse(stdout);
      assert.equal(code, exit, JSON.stringify(body));
      done(body);
    } catch (error) {reject(error);}});
    child.stdin.end(input);
  });
  await cli(['login', '--url', origin, '--token-stdin'], secret + '\n');
  const unitTexts = twoUnits ? [text + '\n', secondText] : [text];
  const added = await api('/api/v1/library/versions', {operation_id: randomUUID(), data: {title: 'Authored copper stanza', name: 'Synthetic integrity fixture', kind: 'text', text: unitTexts.join(''), sections: [{heading: 'Authored stanza', units: unitTexts}]}});
  const version = (await api('/api/v1/library/versions/' + added.result.entity_id)).version;
  const learned = await cli(['learn', version.id]);
  const sid = learned.practice.study.id;
  const next = async card => {
    const current = (await record('meta')).practice.current;
    return (await cli(['next', ...(current?.attempt_id ? ['--after', current.prompt.id] : []), '--study', sid, ...(card ? ['--card', card] : [])])).practice;
  };
  const answer = async input => {
    const current = (await record('meta')).practice.current;
    return (await cli(['answer', current.prompt.id, '--text-stdin'], input)).practice.attempt;
  };
  return {runtime, origin, sql, rows, record, api, cli, sid, version, next, answer};
}

for (const scope of ['unit', 'section', 'whole']) {
  test(`Old disclosure replay refuses through CLI/API under a protected ${scope} Prompt`, async t => {
    const f = await fixture(t, scope !== 'unit');
    const nextOp = randomUUID(), revealOp = randomUUID(), answerOp = randomUUID(), heldOp = randomUUID(), correctOp = randomUUID();
    const initial = (await f.cli(['next', '--study', f.sid, '--operation-id', nextOp])).practice;
    const pid = initial.prompt.id;
    const cid = (await f.record('meta')).practice.current.targets[0].card_id;
    const revealArgs = ['reveal', pid, '--operation-id', revealOp];
    const shown = await f.cli(revealArgs);
    const afterReveal = await f.record('meta');
    assert.equal(afterReveal.practice.current.revealed, true);
    assert.deepEqual((await f.cli(revealArgs)).result, shown.result, 'Current reveal reconciles a lost response.');
    const answerArgs = ['answer', pid, '--text-stdin', '--operation-id', answerOp];
    const answered = await f.cli(answerArgs, text);
    const original = answered.practice.attempt;
    assert.deepEqual((await f.cli(answerArgs, text)).practice.attempt, original);
    await f.cli(['next', '--operation-id', heldOp]);
    const correctArgs = ['correct', pid, original.id, '--verdict', 'Again', '--operation-id', correctOp];
    const corrected = await f.cli(correctArgs);
    for (let i = 0; i < 6; i++) {await f.next(); await f.answer(text);}
    const contextReplay = [];
    if (scope !== 'unit') {
      await f.next(); await f.answer(text);
      const firstHeld = (await f.record('meta')).practice.current;
      const contextOp = randomUUID();
      const context = await f.cli(['next', '--after', firstHeld.prompt.id, '--study', f.sid, '--operation-id', contextOp]);
      assert.equal(context.practice.prompt.context.trim(), text);
      contextReplay.push(['/api/v1/next', {operation_id: contextOp, after: firstHeld.prompt.id, study_id: f.sid}]);
      await f.answer(secondText);
    }
    const held = (await f.record('meta')).practice.current;
    const selected = await f.cli(['next', '--after', held.prompt.id, '--study', f.sid, ...(scope === 'unit' ? [] : ['--recite', scope, ...(scope === 'section' ? ['--section', f.version.sections[0].id] : [])])]);
    assert.equal(selected.practice.prompt.assisted, false);
    assert.equal(selected.practice.revealed, false);
    await f.api('/api/v1/history', undefined, 409);
    const before = await f.rows();
    for (const [path, payload] of [
      ['/api/v1/next', {operation_id: nextOp, study_id: f.sid}],
      ['/api/v1/reveal', {operation_id: revealOp, prompt_id: pid}],
      ['/api/v1/answer', {operation_id: answerOp, prompt_id: pid, answer: text}],
      ['/api/v1/next', {operation_id: heldOp}],
      ...contextReplay,
    ]) {
      const refusal = await f.api(path, payload, 409);
      assert.ok(!JSON.stringify(refusal).includes(text));
      assert.deepEqual(await f.rows(), before, 'Refused replay changes no SQLite row.');
      observations.push({scope, path, status: 409, rows_unchanged: true});
    }
    await f.cli(revealArgs, '', 1);
    await f.api('/api/v1/answer', {operation_id: answerOp, prompt_id: pid, answer: 'Changed payload'}, 409);
    assert.deepEqual(await f.rows(), before);
    const safeCorrection = await f.cli(correctArgs);
    assert.deepEqual(safeCorrection.practice.correction, corrected.practice.correction);
    assert.ok(!JSON.stringify(safeCorrection).includes(text));
    const sameOp = randomUUID();
    const same = await f.cli(['next', '--operation-id', sameOp]);
    assert.deepEqual((await f.cli(['next', '--operation-id', sameOp])).result, same.result);
    const a = await f.answer(f.version.text);
    assert.equal(a.assisted, false);
    assert.equal(a.effects.some(e => e.scheduled), true);
    if (scope !== 'unit') {
      assert.equal(a.effects.length, 2);
      assert.equal(a.effects[0].extra_practice, true);
      assert.equal(a.effects[1].scheduled, true);
    }
    assert.equal((await f.record('practice_cards/' + cid)).state.schedule.reps, 1);
    assert.deepEqual((await f.record('practice_attempts/' + original.id)).revealed, true);
    await f.next(cid);
    const staleSafe = await f.cli(['next', '--operation-id', sameOp]);
    assert.deepEqual(staleSafe.result, same.result, 'A stale safe Prompt receipt remains replayable.');
    assert.ok(!JSON.stringify(staleSafe).includes(text));
  });
}

test('Explicit reveal breaks the cue success run at all rungs through the compiled CLI', async t => {
  const f = await fixture(t);
  await f.next();
  const cid = (await f.record('meta')).practice.current.targets[0].card_id;
  const rungs = ['read', 'first_letters', 'first_letter', 'previous_line', 'section_heading', 'whole_title'];
  for (const [i, cue] of rungs.entries()) {
    await f.answer(text);
    await f.next(cid);
    let current = (await f.record('meta')).practice.current;
    await f.cli(['reveal', current.prompt.id]);
    const copied = await f.answer(text);
    let card = await f.record('practice_cards/' + cid);
    assert.equal(card.state.cue, cue, 'A copied Good does not fade the cue.');
    assert.equal(card.state.successes, 0);
    assert.equal(copied.revealed, true);
    assert.equal(copied.effects[0].scheduled, false);
    await f.next(cid); await f.answer(text); await f.next(cid);
    current = (await f.record('meta')).practice.current;
    await f.cli(['reveal', current.prompt.id]);
    const missed = await f.answer('Wrong words');
    const correction = await f.cli(['correct', current.prompt.id, missed.id, '--verdict', 'Good']);
    card = await f.record('practice_cards/' + cid);
    assert.equal(card.state.cue, cue);
    assert.equal(card.state.successes, 0);
    assert.equal(correction.practice.correction.after.successes, 0);
    for (let j = 0; j < 2; j++) {await f.next(cid); await f.answer(text);}
    card = await f.record('practice_cards/' + cid);
    assert.equal(card.state.cue, rungs[Math.min(i + 1, rungs.length - 1)]);
    observations.push({cue, revealed_good_successes: 0, revealed_again_corrected_successes: 0, next_cue: card.state.cue});
    if (i + 1 < rungs.length) await f.next(cid);
  }
  await f.cli(['study', 'drop', f.sid]);
  const archive = await f.api('/api/v1/export');
  const bytes = Buffer.from(JSON.stringify(archive));
  const restore = await restoreFixture(t);
  const response = await restore.post('/operator/restore', bytes);
  assert.equal(response.status, 200, await response.text());
  const restored = await restore.sql.exec('SELECT value FROM records WHERE key=?', 'practice_cards/' + cid);
  assert.deepEqual(JSON.parse(restored[0].value), await f.record('practice_cards/' + cid));
});

async function restoreFixture(t) {
  const name = 'study-diagnostic-' + randomUUID(), origin = 'https://study-diagnostic.invalid';
  const credential = 'synthetic-study-diagnostic-restore-only-32chars';
  const runtime = await createRuntime({name, inspect: true, bindings: {SCRY_ENV: 'restore', CANONICAL_ORIGIN: origin, SPACE_NAME: name, INTERNAL_KEY: internalKey, RESTORE_TOKEN: credential}});
  t.after(() => runtime.mf.dispose());
  const sql = await runtime.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name});
  const post = async (path, bytes) => {
    let timer;
    try {return await Promise.race([
      runtime.worker.fetch(origin + path, {method: 'POST', headers: {'x-scry-restore-token': credential, 'x-scry-sha256': hash(bytes)}, body: bytes}),
      new Promise((_, reject) => {timer = setTimeout(() => reject(Error('Restore request did not complete within 5000ms.')), 5000);}),
    ]);} finally {clearTimeout(timer);}
  };
  return {sql, post};
}

for (const missing of ['work', 'version', 'section', 'unit', 'target']) {
  test(`Restore dry-run completes a refusal for a missing ${missing} reference and writes zero rows`, async t => {
    const f = await fixture(t);
    await f.next(); await f.answer(text);
    await f.cli(['study', 'drop', f.sid]);
    const archive = await f.api('/api/v1/export');
    const version = Object.values(archive.app.library.versions)[0];
    if (missing === 'work') archive.app.library.works = {};
    if (missing === 'version') archive.app.library.versions = {};
    if (missing === 'section') version.sections = [];
    if (missing === 'unit') version.sections[0].units = [];
    if (missing === 'target') Object.values(archive.app.practice.attempts)[0].effects[0].card_id = 'missing-card';
    const bytes = Buffer.from(JSON.stringify(archive));
    const restore = await restoreFixture(t);
    for (const path of ['/operator/restore', '/operator/restore-dry-run']) {
      const response = await restore.post(path, bytes);
      const body = await response.text();
      assert.equal(response.status, 422, body);
      if (path.endsWith('dry-run')) assert.equal(body, 'INCOMPLETE_IMPORT_REPORT');
      assert.deepEqual(await restore.sql.exec('SELECT key FROM records'), []);
      observations.push({missing, path, status: 422, rows: 0});
    }
  });
}

test.after(async () => {
  const out = process.env.SCRY_STUDY_PROOF_DIR || 'target';
  await mkdir(out, {recursive: true});
  await writeFile(join(out, 'study-integrity.json'), JSON.stringify(observations, null, 2) + '\n');
});

test('Mixed notebook legacy errors, held feedback and replay preserve exact practice', async () => {
  const {mixedHttpProof} = await import('./mixed-notebook.mjs');
  await mixedHttpProof();
});
