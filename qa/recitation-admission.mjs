import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';

// Run inside the existing compiled CLI → Worker → isolated SQLite journey.
// Only the large-card fixture seeds rows; all admission and answers use the API.
export async function recitationAdmission({add, ok, cli, next, answer, stopCurrent, rows, record, req, sql, transcript}) {
  const refuse = async (study, scope, section, status, bound) => {
    const held = (await record('meta')).practice.current;
    const fields = {operation_id: randomUUID(), after: held.prompt.id, study_id: study, recite: scope, ...(section ? {section_id: section} : {})};
    const before = await rows();
    const response = await req('/api/v1/next', fields);
    assert.equal(response.status, status);
    assert.match(response.headers.get('content-type'), /application\/json/);
    assert.match((await response.json()).error, bound);
    assert.deepEqual(await rows(), before, 'Refusal preserves every persisted row, including the held Prompt and receipts.');
    const result = await cli(['next', '--after', held.prompt.id, '--study', study, '--recite', scope, ...(section ? ['--section', section] : [])]);
    assert.notEqual(result.code, 0);
    assert.deepEqual(await rows(), before, 'The compiled CLI refusal also leaves SQLite unchanged.');
  };
  for (const words of [999, 1000]) {
    await stopCurrent();
    const text = 'orchard '.repeat(words);
    const v = await add(`Admission ${words} words`, text);
    const section = (await ok(['version', 'show', v])).version.sections[0].id;
    const study = (await ok(['learn', v])).practice.study.id;
    await next(['--study', study]);
    await answer('');
    for (const scope of ['whole', 'section']) {
      if (words === 1000) {
        await refuse(study, scope, scope === 'section' ? section : null, 422, /1,000,000.*grading/);
      } else {
        await next(['--study', study, '--recite', scope, ...(scope === 'section' ? ['--section', section] : [])]);
        const result = await answer(text);
        assert.equal(result.grade.verdict, 'Good');
        assert.equal(result.assisted, false);
        assert.equal(result.effects.length, 1);
        assert.equal((await record('meta')).practice.current.attempt_id, result.id);
      }
    }
  }
  for (const words of [1999, 2000]) {
    await stopCurrent();
    const title = `Admission sibling ${words} words`, text = (words === 1999 ? 'orchard ' : 'garden ').repeat(499);
    const v = await add(title, text);
    const sibling = await add(title, 'cedar '.repeat(words));
    const shown = await ok(['version', 'show', v]);
    assert.equal((await ok(['version', 'show', sibling])).work.id, shown.work.id, 'The capacity case must use a sibling of the same Work.');
    const section = shown.version.sections[0].id;
    const study = (await ok(['learn', v])).practice.study.id;
    await next(['--study', study]);
    await answer('');
    for (const scope of ['whole', 'section']) {
      if (words === 2000) {
        await refuse(study, scope, scope === 'section' ? section : null, 422, /1,000,000.*grading/);
      } else {
        await next(['--study', study, '--recite', scope, ...(scope === 'section' ? ['--section', section] : [])]);
        assert.equal((await answer(text)).grade.verdict, 'Good');
      }
    }
  }
  await stopCurrent();
  const smallText = 'maple\n.\n';
  const smallVersion = await add('Admission exact serialized completion', smallText, [{heading: 'Two Units', units: ['maple\n', '.\n']}]);
  const small = (await ok(['version', 'show', smallVersion])).version;
  const smallStudy = (await ok(['learn', smallVersion])).practice.study.id;
  await next(['--study', smallStudy]);
  const smallHeld = (await record('meta')).practice.current;
  const smallCard = await record('practice_cards/' + smallHeld.targets[0].card_id);
  await answer('');
  const second = {...smallCard, id: randomUUID().replaceAll('-', ''), unit_id: small.sections[0].units[1].id, unit_index: 1};
  await sql('INSERT INTO records (key,value) VALUES (?,?)', 'practice_cards/' + second.id, JSON.stringify(second));
  const original = await rows();
  const restore = async () => {
    // Restore only this run-owned synthetic snapshot, never another namespace.
    await sql('DELETE FROM records');
    for (const row of original) await sql('INSERT INTO records (key,value) VALUES (?,?)', row.key, row.value);
  };
  const nextFields = () => ({operation_id: randomUUID(), after: smallHeld.prompt.id, study_id: smallStudy, recite: 'whole'});
  const complete = async () => {
    const response = await req('/api/v1/next', nextFields());
    assert.equal(response.status, 200);
    const prompt = (await response.json()).practice.prompt;
    const fields = {operation_id: randomUUID(), prompt_id: prompt.id, answer: smallText};
    const answered = await req('/api/v1/answer', fields);
    assert.equal(answered.status, 200);
    assert.equal((await answered.json()).practice.attempt.grade.verdict, 'Good');
    return fields;
  };
  try {
    // Measure the real completed representation, not an assumed byte reserve.
    await complete();
    const completedBytes = Buffer.byteLength((await rows()).find(row => row.key === 'meta').value);
    const limit = 1024 * 1024;
    for (const extra of [0, 1]) {
      await restore();
      const meta = await record('meta');
      meta.csrf += 'x'.repeat(limit - completedBytes + extra);
      await sql('UPDATE records SET value=? WHERE key=?', JSON.stringify(meta), 'meta');
      if (extra === 1) {
        await refuse(smallStudy, 'whole', null, 413, /1,048,576.*storage/);
        continue;
      }
      const answered = await complete();
      assert.equal(Buffer.byteLength((await rows()).find(row => row.key === 'meta').value), limit);
      const replayMeta = await record('meta');
      replayMeta.revision = 999999999;
      const overflow = Buffer.byteLength(JSON.stringify(replayMeta)) - limit;
      replayMeta.csrf = replayMeta.csrf.slice(0, replayMeta.csrf.length - overflow);
      await sql('UPDATE records SET value=? WHERE key=?', JSON.stringify(replayMeta), 'meta');
      const beforeReplay = await rows();
      const replay = await req('/api/v1/answer', answered);
      assert.equal(replay.status, 413, 'Replay must check the actual next runtime revision.');
      assert.match((await replay.json()).error, /1,048,576.*storage/);
      assert.deepEqual(await rows(), beforeReplay);
      assert.notEqual((await cli(['answer', answered.prompt_id, '--text-stdin', '--operation-id', answered.operation_id], smallText)).code, 0);
      assert.deepEqual(await rows(), beforeReplay);
    }
  } finally {
    await restore();
  }
  const units = ['orchard\n', ...Array(3999).fill('.\n')];
  const v = await add('Admission synthetic storage fixture', units.join(''), [{heading: 'Punctuation scope', units}]);
  const version = (await ok(['version', 'show', v])).version;
  const study = (await ok(['learn', v])).practice.study.id;
  await next(['--study', study]);
  const held = (await record('meta')).practice.current;
  const template = await record('practice_cards/' + held.targets[0].card_id);
  await answer('');
  for (const [i, unit] of version.sections[0].units.entries()) {
    if (i === 0) continue;
    const card = {...template, id: i.toString(16).padStart(32, '0'), unit_id: unit.id, unit_index: i};
    await sql('INSERT INTO records (key,value) VALUES (?,?)', 'practice_cards/' + card.id, JSON.stringify(card));
  }
  for (const scope of ['whole', 'section']) {
    await refuse(study, scope, scope === 'section' ? version.sections[0].id : null, 413, /1,048,576.*storage/);
  }
  transcript.push({observation: 'Recitation admission before replacing held Prompt', exact_words: {999: 'Good, durable Attempt', 1000: '422 JSON, CLI nonzero'}, sibling_cells: {1000000: 'Good', 1000500: '422 JSON'}, completed_meta_bytes: {1048576: 'Good, durable Attempt', 1048577: '413 before presentation'}, replay_revision_growth: '413 JSON, CLI nonzero, rows unchanged', punctuation_units: {4000: '413 JSON storage bound'}, refused_rows: 'byte-for-byte unchanged'});
}
