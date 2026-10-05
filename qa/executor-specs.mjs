import assert from 'node:assert/strict';
import {specs} from './walk-specs.mjs';

const short = (verdict, probability, identity = .01, injection = .01) => ({
  verdict, probabilities: {accept: verdict === 'accept' ? probability : 0, reject: verdict === 'reject' ? probability : 0, unsure: verdict === 'unsure' ? probability : 0}, identity, injection,
});
const rows = [
  ['accept at boundary', short('accept', .85, .35, .20), 'correct'],
  ['accept below boundary', short('accept', .84), 'self-check'],
  ['identity above boundary', short('accept', .99, .36), 'self-check'],
  ['accept injection above boundary', short('accept', .99, .01, .21), 'self-check'],
  ['reject at boundary', short('reject', .90, .99, .20), 'wrong'],
  ['reject below boundary', short('reject', .89), 'self-check'],
  ['reject injection above boundary', short('reject', .99, .01, .21), 'self-check'],
  ['unsure', short('unsure', .99), 'self-check'],
];
export const ruleBSpecs = {
  'US-003': specs['US-003'].map((original, index) => async c => {
    if (index === 3) await c.goTest('^TestSemanticFailurePreservesAnswerAndAllowsNewOperation$', ['./internal/store']);
    else await original(c);
    if (index === 0) for (const answer of ['TLS', 'Transport Layer Security']) await c.scenario('local key or authored variant: ' + answer, {}, async s => {
      await s.recall(); await s.answer(answer); await s.state('result');
      await s.current({graded: true, outcome: 'correct', authority: 'exact'});
      assert.equal(s.requests.length, 0); await s.reload({graded: true, outcome: 'correct', authority: 'exact'});
    });
    if (index === 2) for (const [name, semantic, graded] of [
      ['semantic equivalent', {ideas: [.99, .99], contradiction: .01, relation: 'equivalent'}, true],
      ['semantic incomplete remains shadow', {ideas: [.99, .10], contradiction: .01, relation: 'partial'}, false],
      ['semantic incorrect remains shadow', {ideas: [.10, .10], contradiction: .99, relation: 'different'}, false],
    ]) await c.scenario(name, {semantic}, async s => {
      await s.recall(); await s.answer('TLS'); await s.state('result');
      await s.page.locator('form[data-next] button').click(); await s.page.locator('#recall-answer').waitFor();
      await s.answer('synthetic prose about certificate trust'); await s.state(graded ? 'result' : 'self-check');
      await s.current(graded ? {graded: true, outcome: 'correct', authority: 'jev'} : {graded: false, self_check: true, assisted: false});
      assert.ok(s.requests[0].questions.relation); assert.ok(s.requests[0].questions.idea_0);
      await s.reload(graded ? {graded: true, outcome: 'correct', authority: 'jev'} : {graded: false, self_check: true});
    });
    if (index === 3) await c.scenario('failed check requires a deliberate retry', {failure: 'malformed'}, async s => {
      await s.recall(); await s.answer('synthetic saved retry answer'); await s.state('self-check');
      await s.replay(); assert.equal(s.requests.length, 1);
      await s.page.getByRole('button', {name: 'Retry check', exact: true}).click(); await s.state('self-check');
      assert.equal(s.requests.length, 2); await s.current({graded: false, answer: 'synthetic saved retry answer'});
    });
    if (index === 4) await c.scenario('one paid assessment and event survive exact replay and restart', {short: short('accept', .99)}, async s => {
      await s.recall(); await s.answer('a synthetic equivalent TLS answer'); await s.state('result');
      await s.replay(); assert.equal(s.requests.length, 1); await s.history('short-v1');
      await s.restart(); await s.reload({graded: true, outcome: 'correct', authority: 'jev'});
      assert.equal(s.requests.length, 1);
    });
  }),
  'US-008': [
    async c => {
      await c.scenario('different wording uses the battery', {short: short('accept', .97)}, async s => {
        await s.recall(); await s.answer('the transport layer security protocol');
        await s.state('result');
        assert.equal(s.requests.length, 1);
        assert.deepEqual(Object.keys(s.requests[0].questions).sort(), ['identity', 'injection', 'verdict']);
        assert.equal(s.requests[0].questions.verdict.type, 'choice');
        assert.equal(s.requests[0].questions.identity.type, 'noul');
        assert.equal(s.requests[0].questions.injection.type, 'noul');
        assert.equal(s.requests[0].state.learner_answer, 'the transport layer security protocol');
        await s.current({outcome: 'correct', authority: 'jev', graded: true});
        await s.reload({outcome: 'correct', authority: 'jev', graded: true});
      });
    },
    async c => {
      for (const [name, judgment, outcome] of rows) await c.scenario(name, {short: judgment}, async s => {
        await s.recall(); await s.answer('synthetic non-exact TLS answer');
        await s.state(outcome === 'self-check' ? outcome : 'result');
        await s.current(outcome === 'self-check' ? {graded: false, self_check: true, answer: 'synthetic non-exact TLS answer'} : {graded: true, outcome, authority: 'jev'});
        await s.reload(outcome === 'self-check' ? {graded: false, self_check: true} : {graded: true, outcome, authority: 'jev'});
        assert.equal(s.requests.length, 1);
      });
      await c.probe();
    },
    async c => {
      for (const failure of ['unavailable', 'malformed', 'unconfigured']) await c.scenario(failure, {failure}, async s => {
        await s.recall(); const answer = 'my saved synthetic answer'; await s.answer(answer);
        await s.state('self-check');
        await s.current({graded: false, self_check: true, answer});
        await s.page.getByRole('button', {name: 'Retry check', exact: true}).waitFor();
        await s.reload({graded: false, self_check: true, answer});
        const sends = s.requests.length;
        await s.page.getByRole('button', {name: 'I was right', exact: true}).click();
        await s.state('result');
        await s.current({graded: true, outcome: 'self_correct', authority: 'learner', answer});
        await s.reload({graded: true, outcome: 'self_correct', authority: 'learner', answer});
        await s.restart(); await s.reload({graded: true, outcome: 'self_correct', authority: 'learner', answer});
        assert.equal(s.requests.length, sends);
        await s.history('learner-v1');
      });
      await c.permissions();
    },
  ],
};
