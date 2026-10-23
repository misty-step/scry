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
    const title = `Admission sibling ${words} words`, text = 'orchard '.repeat(499);
    const v = await add(title, text);
    await add(title, 'cedar '.repeat(words));
    const section = (await ok(['version', 'show', v])).version.sections[0].id;
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
  transcript.push({observation: 'Recitation admission before replacing held Prompt', exact_words: {999: 'Good, durable Attempt', 1000: '422 JSON, CLI nonzero'}, sibling_cells: {1000000: 'Good', 1000500: '422 JSON'}, punctuation_units: {4000: '413 JSON storage bound'}, refused_rows: 'byte-for-byte unchanged'});
}
