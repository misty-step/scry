// Study beside the earlier concept notebook, through the real isolated Worker.
// The earlier notebook is history: each stored state is loaded from the
// fixture, shown read-only, refused every retired action and left as stored,
// while Study beside it keeps working.
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {isDeepStrictEqual} from 'node:util';
import {createRuntime, internalKey} from './runtime-helper.mjs';
import {assertStudySource} from './study-review.mjs';
import {seedEarlier,stored} from './earlier-fixture.mjs';

export function formFields(html, action) {
  const form = [...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m => m[1] === action);
  assert.ok(form, 'Expected native form: ' + action);
  return Object.fromEntries([...form[2].matchAll(/name="([^"]+)" value="([^"]*)"/g)].map(m => [m[1], m[2]]));
}

// What /notebook shows for the states these journeys load.
const VIEW={'question-cold-choice':'earlier-unanswered','result-corrected':'earlier-result','self-check-unsure':'earlier-ungraded'};
const RETIRED=/The earlier concept notebook is kept as history\./;

export async function mixedFixture({nowMs, state='question-cold-choice'} = {}) {
  const name = 'mixed-' + randomUUID();
  const bindings = {SCRY_ENV:'development', CANONICAL_ORIGIN:'http://127.0.0.1:0', SPACE_NAME:name, INTERNAL_KEY:internalKey, ...(nowMs === undefined ? {} : {SCRY_TEST_NOW_MS:String(nowMs)})};
  let egress = 0;
  const runtime = await createRuntime({port:0, name, inspect:true, bindings, testClock:nowMs !== undefined, egress:async () => {egress++; throw Error('No external effects allowed.');}});
  const origin = (await runtime.mf.ready).origin;
  const {worker} = await runtime.rebind({...bindings, CANONICAL_ORIGIN:origin});
  const sql = await runtime.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name});
  const record = async key => JSON.parse((await sql.exec('SELECT value FROM records WHERE key=?', key))[0].value);
  const html = async path => (await worker.fetch(origin + path)).text();
  const post = (path, fields) => worker.fetch(origin + path, {method:'POST', redirect:'manual', headers:{'Content-Type':'application/x-www-form-urlencoded', 'X-Scry-Test-Origin':origin}, body:new URLSearchParams(fields)});
  // The earlier notebook as an existing Owner holds it, then a Study beside it.
  await seedEarlier(sql, state, await runtime.mf.getR2Bucket('SCRY_ASSETS'));
  const minted = await post('/tokens/create', {...formFields(await html('/settings'), '/tokens/create'), name:'Mixed synthetic fixture; unpaid'});
  assert.equal(minted.status, 200);
  const secret = /<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())[1];
  const api = async (path, fields) => {
    const r = await worker.fetch(origin + '/api/v1/' + path, {method:'POST', headers:{Authorization:'Bearer ' + secret, 'Content-Type':'application/json'}, body:JSON.stringify({operation_id:randomUUID(), ...fields})});
    assert.equal(r.status, 200, 'Synthetic exact setup refused');
    return r.json();
  };
  const version = await api('library/versions', {data:{title:'Separate exact Stream', name:'Authored mixed fixture', kind:'text', text:'Copper lanterns shelter quiet gardens.'}});
  await api('learn', {version_id:version.result.entity_id, pace:'steady'});
  await api('next', {});
  const practice = structuredClone((await record('meta')).practice);
  // Every row of the earlier notebook, to compare with what was loaded.
  const EARLIER=['goals/','concepts/','questions/','events/','overrides/','resets/','feedback/','assessments/','jobs/','job_payload/','spend/','spend_payload/'];
  const earlier = async () => Object.fromEntries((await sql.exec('SELECT key,value FROM records ORDER BY key')).filter(row => EARLIER.some(prefix => row.key.startsWith(prefix))).map(row => [row.key, row.value]));
  const loaded = Object.fromEntries(Object.entries(stored(state).rows).filter(([key]) => EARLIER.some(prefix => key.startsWith(prefix))));
  const occurrence = JSON.parse(stored(state).rows.meta).occurrence;
  // The earlier notebook is exactly as loaded, however Study has moved.
  const intact = async stage => {assert.deepEqual(await earlier(), loaded, stage+': an earlier row changed');assert.deepEqual((await record('meta')).occurrence, occurrence, stage+': the held earlier question changed');};
  return {origin, state, html, post, api, record, sql, practice, intact, close:async () => {await runtime.mf.dispose(); assert.equal(egress, 0);}};
}

export async function mixedHttpProof() {
  const observed = [];
  for (const state of Object.keys(VIEW)) {
    const f = await mixedFixture({state});
    try {
      const heldHome=async(stage)=>{
        const before=await f.sql.exec('SELECT key,value FROM records ORDER BY key'),meta=await f.record('meta'),held=meta.practice.current;
        const html=await f.html('/');assert.ok(html.includes('data-state="'+(held.attempt_id?'exact-result':'exact-prompt')+'"'));
        const mainText=/<main\b[^>]*>([\s\S]*?)<\/main>/.exec(html)[1].replace(/<[^>]*>/g,'');
        for(const hidden of [meta.occurrence.presentation.answer,meta.occurrence.presentation.explanation])assert.ok(!mainText.includes(hidden),stage+': Study never shows the earlier notebook: '+hidden);
        if(!held.attempt_id){assert.equal(held.prompt.assisted,true);assert.ok(html.includes('Copper lanterns shelter quiet gardens.'));assertStudySource(html,'Copper lanterns shelter quiet gardens.');}
        const countClass=name=>[...html.matchAll(/<[a-z][^>]*class="([^"]*)"[^>]*>/g)].filter(m=>m[1].split(/\s+/).includes(name)).length;
        const pairs=held.attempt_id?1:0;assert.equal(countClass('explanation'),0,stage+': no earlier explanation');assert.equal(countClass('answer-pair'),pairs,stage+': only graded exact answers have a pair');assert.equal(countClass('exact-full-answer'),pairs,stage+': exact result owns the answer disclosure');
        if(held.attempt_id)assert.match(html,/<details class="exact-full-answer"><summary>Full answer<\/summary>[\s\S]*?<div class="answer-pair"><span>You answered<\/span><p>Copper lanterns shelter quiet gardens\.<\/p>[\s\S]*?<\/details>/,'Exact answer pair is contained in its initially closed disclosure');
        observed.push({state,stage,exact_answer_pairs:pairs,earlier_explanations:0,full_answer_disclosures:pairs});
        await f.html('/');assert.deepEqual(await f.sql.exec('SELECT key,value FROM records ORDER BY key'),before,'Default held home reads preserve every record');
        await f.intact(stage);
      };
      await heldHome('both-held');
      // Earlier practice shows what is held, read-only.
      const before = await f.sql.exec('SELECT key,value FROM records ORDER BY key');
      const view = await f.html('/notebook');
      assert.ok(view.includes(`data-state="${VIEW[state]}"`), `${state}: /notebook shows ${VIEW[state]}`);
      assert.doesNotMatch(/<main\b[^>]*>([\s\S]*?)<\/main>/.exec(view)[1], /<form|data-poll|data-choice/, `${state}: /notebook is read-only`);
      // Every retired action is refused on that page, with a current, a stale
      // and an empty answer alike, and changes no row.
      const meta = await f.record('meta'), envelope = () => ({csrf:meta.csrf, operation_id:randomUUID(), occurrence_id:meta.occurrence.id});
      for (const [path, extra] of [['/review/answer', {answer:meta.occurrence.presentation.answer}], ['/review/answer', {occurrence_id:'stale-occurrence', answer:'invalid'}], ['/review/answer', {answer:''}], ['/review/next', {}], ['/review/retry', {}], ['/review/reveal', {}], [`/concepts/${meta.occurrence.concept_id}/practice`, {}]]) {
        const r = await f.post(path, {...envelope(), ...extra});
        assert.equal(r.status, 410, `${state}: ${path}`);
        const page = await r.text();
        assert.match(page, RETIRED);
        assert.ok(page.includes(`data-state="${VIEW[state]}"`), 'A refusal is shown on Earlier practice with what is held there');
        assert.ok(!page.includes('data-state="exact-'), 'A refusal never renders Study');
        assert.ok(isDeepStrictEqual(await f.sql.exec('SELECT key,value FROM records ORDER BY key'), before), 'Refusal changes no rows');
      }
      for (const path of ['/review/self', '/review/override']) {
        const r = await f.post(path, {...envelope(), correct:'true'});
        assert.equal(r.status, 410);assert.match(await r.text(), /Self-grading is retired\./);
        assert.ok(isDeepStrictEqual(await f.sql.exec('SELECT key,value FROM records ORDER BY key'), before));
      }
      observed.push({state, view:VIEW[state], retired_actions:410, rows_unchanged:true});
      assert.ok((await f.html('/recall')).includes('data-state="exact-prompt"'), 'Exact Stream remains current');
      assert.deepEqual((await f.record('meta')).practice, f.practice);
      // Study goes on beside it: source, Try from memory, a graded answer.
      const beforeQuiz=await f.sql.exec('SELECT key,value FROM records ORDER BY key'),action=assertStudySource(await f.html('/'),'Copper lanterns shelter quiet gardens.');
      assert.equal((await f.post(action.path,action.fields)).status,303);
      const quiz=await f.html('/'),quizCurrent=(await f.record('meta')).practice.current;
      assert.notEqual(quizCurrent.prompt.id,f.practice.current.prompt.id);
      assert.doesNotMatch(quiz,/Copper lanterns shelter quiet gardens\./,'Explicit memory quiz hides supplied wording');
      assert.equal((await f.sql.exec("SELECT key FROM records WHERE key LIKE 'practice_attempts/%'")).length,0,'Studying and starting memory creates no attempt');
      const schedules=rows=>rows.filter(r=>r.key.startsWith('practice_cards/')).map(r=>[r.key,JSON.parse(r.value).state.schedule]);
      assert.deepEqual(schedules(await f.sql.exec('SELECT key,value FROM records ORDER BY key')),schedules(beforeQuiz),'Source-to-quiz does not advance a schedule');
      assert.equal((await f.post('/practice/answer',{...formFields(quiz,'/practice/answer'),answer:'Copper lanterns shelter quiet gardens.'})).status,303);
      await heldHome('exact-held');
      const savedAttempt=(await f.api('next',{})).practice.attempt;
      assert.equal(savedAttempt.assisted,false,'Hidden-source response is recall, not copied-source success');
      assert.equal(savedAttempt.effects[0].scheduled,true);
      await f.intact('after Study');
      assert.ok((await f.html('/notebook')).includes(`data-state="${VIEW[state]}"`));
      observed.push({state,source_no_answer_input:true,source_to_quiz_schedule_unchanged:true,quiz_source_hidden:true,graded_recall_scheduled:true,held_home_read_only:true,earlier_notebook_as_loaded:true});
    } finally {await f.close();}
  }
  return observed;
}

export async function mixedBrowserWalk(browser, capture) {
  const results = [];
  for (const noJS of [false, true]) for (const state of Object.keys(VIEW)) {
    const f = await mixedFixture({state});
    const context = await browser.newContext({viewport:{width:390, height:844}, isMobile:true, hasTouch:true, colorScheme:noJS?'dark':'light', javaScriptEnabled:!noJS});
    const page = await context.newPage();
    page.setDefaultTimeout(12000);
    const row = {noJS, state, status:'pass', stage:'initial', evidence:[]};
    results.push(row);
    const shot = async name => row.evidence.push(await capture(page, `mixed-${noJS?'native':'js'}-${state}-${name}.png`));
    const earlierView = () => page.locator(`[data-state="${VIEW[state]}"]`);
    const homeRoundTrip = async stage => {
      row.stage='home '+stage;
      const meta = await f.record('meta');
      const frozen = {occurrence:meta.occurrence, practice:meta.practice};
      assert.ok(frozen.occurrence && frozen.practice.current, 'Both the earlier notebook and Study hold something');
      const unchanged = async () => {const current=await f.record('meta');assert.deepEqual({occurrence:current.occurrence,practice:current.practice},frozen, 'Navigation preserves both');await f.intact(stage);};
      const exactState=frozen.practice.current.attempt_id?'exact-result':'exact-prompt';
      const home = () => page.locator('[data-state="'+exactState+'"]');
      const openHome = async () => {await page.getByRole('link',{name:'Scry, home',exact:true}).tap();await home().waitFor();await unchanged();};
      await openHome();
      const content=await page.locator('main').innerText();
      for(const hidden of [frozen.occurrence.presentation.answer,frozen.occurrence.presentation.explanation])assert.ok(!content.includes(hidden), 'Home never shows the earlier notebook');
      if(!frozen.practice.current.attempt_id){
        const assisted=frozen.practice.current.prompt.assisted||frozen.practice.current.revealed;
        if(assisted){assert.ok(content.includes('Copper lanterns shelter quiet gardens.'),'Held guided reading retains its supplied text');assertStudySource(await page.content(),'Copper lanterns shelter quiet gardens.');}
        else assert.ok(!content.includes('Copper lanterns shelter quiet gardens.'),'Held unaided exact prompt keeps its answer hidden');
      }
      const pairs=frozen.practice.current.attempt_id?1:0;
      assert.equal(await page.locator('.explanation').count(),0,stage+': no earlier explanation');
      assert.equal(await page.locator('.answer-pair').count(),pairs,stage+': only the held exact result has an answer pair');
      assert.equal(await home().locator('.exact-full-answer .answer-pair').count(),pairs,stage+': answer belongs to exact full-answer disclosure');
      assert.equal(await home().locator('.exact-full-answer').count(),pairs,stage+': result disclosure count');
      if(pairs){assert.equal(await home().locator('.exact-full-answer').getAttribute('open'),null,stage+': full answer starts collapsed');assert.equal(await home().locator('.exact-full-answer .answer-pair p').textContent(),'Copper lanterns shelter quiet gardens.');}
      await shot(`home-${stage}`);
      await page.reload();await home().waitFor();await unchanged();
      // Read-only default home resumes the held exact stream, not a new selection.
      await page.goto(f.origin+'/recall');await home().waitFor();await unchanged();
      await page.goBack();await home().waitFor();await unchanged();
      await page.goForward();await home().waitFor();await unchanged();
      // Earlier practice is reached from the menu and shows what is held there.
      for(let visit=0;visit<2;visit++){
        await page.locator('.account-menu>summary').tap();await page.getByRole('link',{name:'Earlier practice',exact:true}).tap();
        await page.waitForURL(f.origin+'/notebook');await earlierView().waitFor();await unchanged();
        if(visit===0)await openHome();
      }
    };
    try {
      await page.goto(f.origin + '/notebook');
      row.stage='earlier practice is read-only';
      await earlierView().waitFor();
      assert.equal(await page.locator('main form, main button, main textarea, main [data-poll], main [data-choice]').count(),0,'Earlier practice offers nothing to submit');
      assert.equal(await page.getByRole('button',{name:/Next question|Check my answer|^Check$|I was right|Retry/}).count(),0);
      if(state==='question-cold-choice'){await page.getByRole('heading',{name:'Not answered.',exact:true}).waitFor();assert.equal(await page.locator('main ol li').count(),3,'The choices that were presented are shown as text');}
      if(state==='result-corrected'){await page.getByText('Your Correction',{exact:true}).waitFor();await page.locator('.answer-pair').waitFor();}
      if(state==='self-check-unsure'){await page.getByRole('heading',{name:'Your answer is saved, not graded.',exact:true}).waitFor();await page.getByText('Check recorded: inconclusive.',{exact:true}).waitFor();}
      // Enter, Space and digits are not answers: nothing changes and nothing navigates.
      if(!noJS){await page.getByRole('heading',{level:1}).click();for(const key of ['1','2','Enter',' '])await page.keyboard.press(key);await page.waitForTimeout(300);assert.equal(new URL(page.url()).pathname,'/notebook');}
      await shot('earlier');
      await homeRoundTrip('both-held');
      row.stage='exact stream unchanged, source study, and explicit memory quiz';
      await page.goto(f.origin + '/recall');
      await page.locator('[data-state="exact-prompt"]').waitFor();
      assert.deepEqual((await f.record('meta')).practice, f.practice);
      const action=assertStudySource(await page.content(),'Copper lanterns shelter quiet gardens.');
      await page.locator(`form[action="${action.path}"] .primary`).tap();
      await page.getByLabel('Your answer',{exact:true}).waitFor();
      assert.ok(!(await page.content()).includes('Copper lanterns shelter quiet gardens.'));
      assert.equal((await f.sql.exec("SELECT key FROM records WHERE key LIKE 'practice_attempts/%'")).length,0);
      await page.getByLabel('Your answer',{exact:true}).fill('Copper lanterns shelter quiet gardens.');
      await page.getByRole('button',{name:'Check',exact:true}).tap();await page.locator('[data-state="exact-result"]').waitFor();
      await homeRoundTrip('exact-held');
      await f.intact('after the walk');
      row.observed = {earlier_view:VIEW[state], earlier_practice_read_only:true, earlier_rows_as_loaded:true, exact_practice_unchanged:true, home_both_held_unchanged:true, home_exact_held_unchanged:true, earlier_practice_from_menu:true};
    } catch (error) {
      row.status = 'fail'; row.reason = error.message; row.stack=error.stack;
      console.error('Mixed notebook failure: '+JSON.stringify({noJS,state,stage:row.stage,reason:row.reason,stack:row.stack}));
      await shot('failure');
    } finally {await context.close(); await f.close();}
  }
  return results;
}
