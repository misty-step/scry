import {historicalMutation} from './historical-fixture.mjs';
// Mixed exact Study + understanding occurrence, through the real isolated Worker.
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {isDeepStrictEqual} from 'node:util';
import {createRuntime, internalKey} from './runtime-helper.mjs';
import {assertStudySource} from './study-review.mjs';

export function formFields(html, action) {
  const form = [...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m => m[1] === action);
  assert.ok(form, 'Expected native form: ' + action);
  return Object.fromEntries([...form[2].matchAll(/name="([^"]+)" value="([^"]*)"/g)].map(m => [m[1], m[2]]));
}

export async function mixedFixture({nowMs} = {}) {
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
  assert.equal((await post('/__fixture', formFields(await html('/notebook'), '/__fixture'))).status, 303);
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
  return {origin, html, post, api, record, sql, practice, close:async () => {await runtime.mf.dispose(); assert.equal(egress, 0);}};
}

export async function mixedHttpProof() {
  const f = await mixedFixture();
  const observed = [];
  try {
    const heldHome=async(stage)=>{
      const before=await f.sql.exec('SELECT key,value FROM records ORDER BY key'),meta=await f.record('meta'),held=meta.practice.current;
      const html=await f.html('/');assert.ok(html.includes('data-state="'+(held.attempt_id?'exact-result':'exact-prompt')+'"'));
      const mainText=/<main\b[^>]*>([\s\S]*?)<\/main>/.exec(html)[1].replace(/<[^>]*>/g,'');
      for(const hidden of [meta.occurrence.presentation.answer,meta.occurrence.presentation.explanation])assert.ok(!mainText.includes(hidden),stage+': default home cannot reveal unrelated legacy text: '+hidden);
      if(!held.attempt_id){assert.equal(held.prompt.assisted,true);assert.ok(html.includes('Copper lanterns shelter quiet gardens.'));assertStudySource(html,'Copper lanterns shelter quiet gardens.');}
      const countClass=name=>[...html.matchAll(/<[a-z][^>]*class="([^"]*)"[^>]*>/g)].filter(m=>m[1].split(/\s+/).includes(name)).length;
      const pairs=held.attempt_id?1:0;assert.equal(countClass('explanation'),0,stage+': no unrelated legacy explanation');assert.equal(countClass('answer-pair'),pairs,stage+': only graded exact answers have a pair');assert.equal(countClass('exact-full-answer'),pairs,stage+': exact result owns the answer disclosure');
      if(held.attempt_id)assert.match(html,/<details class="exact-full-answer"><summary>Full answer<\/summary>[\s\S]*?<div class="answer-pair"><span>You answered<\/span><p>Copper lanterns shelter quiet gardens\.<\/p>[\s\S]*?<\/details>/,'Exact answer pair is contained in its initially closed disclosure');
      observed.push({stage,exact_answer_pairs:pairs,legacy_explanations:0,full_answer_disclosures:pairs});
      await f.html('/');assert.deepEqual(await f.sql.exec('SELECT key,value FROM records ORDER BY key'),before,'Default held home reads preserve every record');
    };
    await heldHome('both-current');
    const original = await f.html('/notebook');
    const fields = formFields(original, '/review/answer');
    const before = await f.sql.exec('SELECT key,value FROM records ORDER BY key');
    for (const [status, extra] of [[409, {occurrence_id:'stale-occurrence', answer:'invalid'}], [422, {answer:''}]]) {
      const r = await f.post('/review/answer', {...fields, ...extra});
      assert.equal(r.status, status);
      const page = await r.text();
      assert.ok(page.includes('data-state="question"'), 'Rejected legacy answer retains its question view');
      assert.ok(!page.includes('data-state="exact-'), 'Legacy refusal must not render exact practice');
      assert.ok(isDeepStrictEqual(await f.sql.exec('SELECT key,value FROM records ORDER BY key'), before), 'Refusal changes no rows');
      observed.push({status, view:'legacy question', rows_unchanged:true});
    }
    const answer = 'The origin content definitely changed';
    const r = await f.post('/review/answer', {...fields, answer});
    assert.equal(r.status, 303);
    assert.equal(new URL(r.headers.get('location'), f.origin).pathname, '/notebook');
    assert.ok((await f.html('/notebook')).includes('data-state="result"'), 'Answer remains held');
    assert.ok((await f.html('/recall')).includes('data-state="exact-prompt"'), 'Exact Stream remains current');
    assert.deepEqual((await f.record('meta')).practice, f.practice);
    // Reconcile the original browser operation, including its old shared "/" receipt.
    const saved = await f.sql.exec('SELECT key,value FROM records ORDER BY key');
    const replay = await f.post('/review/answer', {...fields, answer});
    assert.equal(new URL(replay.headers.get('location'), f.origin).pathname, '/notebook');
    const after = await f.sql.exec('SELECT key,value FROM records ORDER BY key');
    const withoutRevision = rows => rows.map(({key, value}) => {
      const record = JSON.parse(value);
      if (key === 'meta') delete record.revision;
      return {key, value:record};
    });
    // The existing browser save increments the storage revision even on replay.
    assert.ok(isDeepStrictEqual(withoutRevision(after), withoutRevision(saved)), 'Replay changes no learning records or receipts');
    await heldHome('concept-held');
    const stale=await f.post('/review/answer',{...fields,answer:'The browser must delete it forever'});assert.equal(stale.status,409);
    const staleHtml=await stale.text();assert.match(staleHtml,/data-state="result"/);assert.doesNotMatch(staleHtml,/data-state="exact-/);assert.match(staleHtml,/changed|another tab/i);
    const historical=async path=>{
      const meta=await f.record('meta'),rows=await f.sql.exec('SELECT key,value FROM records ORDER BY key'),historicalFields={csrf:meta.csrf,operation_id:randomUUID(),occurrence_id:meta.occurrence.id,correct:'true'};
      assert.equal((await f.post(path,historicalFields)).status,410);assert.deepEqual(await f.sql.exec('SELECT key,value FROM records ORDER BY key'),rows);
      await historicalMutation(f.sql,{path,fields:historicalFields,now_ms:Date.now(),seed:historicalFields.operation_id});assert.match(await f.html('/notebook'),/data-state="result"/);
    };
    const nextLegacy=async()=>assert.equal((await f.post('/review/next',formFields(await f.html('/notebook'),'/review/next'))).status,303);
    await historical('/review/override');assert.match(await f.html('/notebook'),/You corrected this grade/);await nextLegacy();
    assert.equal((await f.post('/review/answer',{...formFields(await f.html('/notebook'),'/review/answer'),answer:'My own uncertain freshness answer'})).status,303);
    await new Promise(resolve=>setTimeout(resolve,1300));
    const meaning=await f.html('/notebook');assert.match(meaning,/data-state="self-check"/);assert.match(meaning,/Your answer is saved, not graded\./);assert.doesNotMatch(meaning,/>I was right</);
    await historical('/review/self');await nextLegacy();assert.doesNotMatch(await f.html('/notebook'),/data-state="result"/);assert.deepEqual((await f.record('meta')).practice,f.practice);
    await heldHome('legacy-history-complete-exact-guided');
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
    observed.push({source_no_answer_input:true,source_to_quiz_schedule_unchanged:true,quiz_source_hidden:true,graded_recall_scheduled:true,held_home_read_only:true});
    observed.push({answer_redirect:'/notebook',held_result:true,replay_learning_rows_unchanged:true,exact_practice_unchanged_until_explicit_quiz:true});
    return observed;
  } finally {await f.close();}
}

export async function mixedBrowserWalk(browser, capture) {
  const results = [];
  for (const noJS of [false, true]) {
    const f = await mixedFixture();
    const context = await browser.newContext({viewport:{width:390, height:844}, isMobile:true, hasTouch:true, colorScheme:noJS?'dark':'light', javaScriptEnabled:!noJS});
    const page = await context.newPage();
    page.setDefaultTimeout(12000);
    const row = {noJS, status:'pass', stage:'initial', evidence:[]};
    results.push(row);
    const shot = async name => row.evidence.push(await capture(page, `mixed-${noJS?'native':'js'}-${name}.png`));
    const historicalGrade=async(path)=>{
      row.stage='historical '+path;
      const meta=await f.record('meta'),before=await f.sql.exec('SELECT key,value FROM records ORDER BY key');
      const fields={csrf:meta.csrf,operation_id:randomUUID(),occurrence_id:meta.occurrence.id,correct:'true'};
      assert.equal((await f.post(path,fields)).status,410);assert.deepEqual(await f.sql.exec('SELECT key,value FROM records ORDER BY key'),before);
      await historicalMutation(f.sql,{path,fields,now_ms:Date.now(),seed:fields.operation_id});
      await page.reload();await page.locator('[data-state="result"]').waitFor();
    };
    const homeRoundTrip = async stage => {
      row.stage='home '+stage;
      const meta = await f.record('meta');
      const frozen = {occurrence:meta.occurrence, practice:meta.practice};
      assert.ok(frozen.occurrence && frozen.practice.current, 'Both practice streams must be populated');
      const unchanged = async () => {const current=await f.record('meta');assert.deepEqual({occurrence:current.occurrence,practice:current.practice},frozen, 'Home navigation must preserve both frozen practice streams');};
      const exactState=frozen.practice.current.attempt_id?'exact-result':'exact-prompt';
      const home = () => page.locator('[data-state="'+exactState+'"]');
      const openHome = async () => {await page.getByRole('link',{name:'Scry, home',exact:true}).tap();await home().waitFor();await unchanged();};
      await openHome();
      const content=await page.locator('main').innerText();
      for(const hidden of [frozen.occurrence.presentation.answer,frozen.occurrence.presentation.explanation])assert.ok(!content.includes(hidden), 'Home never leaks the unrelated legacy stream');
      if(!frozen.practice.current.attempt_id){
        const assisted=frozen.practice.current.prompt.assisted||frozen.practice.current.revealed;
        if(assisted){assert.ok(content.includes('Copper lanterns shelter quiet gardens.'),'Held guided reading retains its supplied text');assertStudySource(await page.content(),'Copper lanterns shelter quiet gardens.');}
        else assert.ok(!content.includes('Copper lanterns shelter quiet gardens.'),'Held unaided exact prompt keeps its answer hidden');
      }
      const pairs=frozen.practice.current.attempt_id?1:0;
      assert.equal(await page.locator('.explanation').count(),0,stage+': no unrelated legacy explanation');
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
      await page.locator('.account-menu>summary').tap();await page.getByRole('link',{name:'Earlier practice',exact:true}).tap();
      await page.waitForURL(f.origin+'/notebook');await page.locator('[data-state="'+frozen.occurrence.phase+'"]').waitFor();await unchanged();
      await openHome();await page.locator('.account-menu>summary').tap();await page.getByRole('link',{name:'Earlier practice',exact:true}).tap();await page.waitForURL(f.origin+'/notebook');await page.locator('[data-state="'+frozen.occurrence.phase+'"]').waitFor();await unchanged();
    };
    try {
      await page.goto(f.origin + '/notebook');
      await homeRoundTrip('both-current');
      row.stage='legacy answer and stale tab';
      const stale = await context.newPage();
      await stale.goto(f.origin + '/notebook');
      await page.getByRole('button', {name:'The origin content definitely changed', exact:true}).tap();
      await page.locator('[data-state="result"]').waitFor();
      assert.equal(new URL(page.url()).pathname, '/notebook');
      await page.reload();
      await page.locator('[data-state="result"]').waitFor();
      await shot('held-result');
      await homeRoundTrip('concept-held');
      row.stage='stale legacy answer';
      await stale.getByRole('button', {name:'The browser must delete it forever', exact:true}).tap();
      if (noJS) {
        await stale.locator('[data-state="result"]').waitFor();
        assert.equal(new URL(stale.url()).pathname, '/review/answer');
        assert.match(await stale.locator('main').innerText(), /changed|another tab/i);
      } else {
        await stale.getByText('This changed in another tab. Your draft is still here.', {exact:true}).waitFor();
        assert.equal(new URL(stale.url()).pathname, '/notebook');
      }
      assert.equal(await stale.locator('[data-state^="exact-"]').count(), 0);
      row.error = {status:409, legacy_context:true};
      row.evidence.push(await capture(stale, `mixed-${noJS?'native':'js'}-409.png`));
      await stale.close();
      assert.equal(await page.getByRole('button', {name:'I was right', exact:true}).count(),0);
      await historicalGrade('/review/override');
      await page.getByText('You corrected this grade', {exact:false}).waitFor();
      await shot('override');
      await page.getByRole('button', {name:'Next question', exact:false}).tap();
      row.stage='saved legacy meaning check';
      await page.locator('#answer').fill('My own uncertain freshness answer');
      await page.getByRole('button', {name:'Check my answer', exact:true}).tap();
      // No provider configuration: the saved meaning check becomes self-check.
      await page.waitForTimeout(1300);
      await page.reload();
      await page.locator('[data-state="self-check"]').waitFor();
      assert.equal(new URL(page.url()).pathname, '/notebook');
      await shot('self-check');
      assert.equal(await page.getByRole('button', {name:'I was right', exact:true}).count(),0);
      await page.getByRole('heading',{name:'Your answer is saved, not graded.',exact:true}).waitFor();
      await historicalGrade('/review/self');
      await shot('learner-result');
      await page.getByRole('button', {name:'Next question', exact:false}).tap();
      await page.locator('[data-state="result"]').waitFor({state:'detached'});
      assert.equal(new URL(page.url()).pathname, '/notebook');
      assert.deepEqual((await f.record('meta')).practice, f.practice);
      row.stage='exact stream unchanged, source study, and explicit memory quiz';
      await page.goto(f.origin + '/recall');
      await page.locator('[data-state="exact-prompt"]').waitFor();
      assert.deepEqual((await f.record('meta')).practice, f.practice);
      await shot('exact-unchanged');
      const action=assertStudySource(await page.content(),'Copper lanterns shelter quiet gardens.');
      await page.locator(`form[action="${action.path}"] .primary`).tap();
      await page.getByLabel('Your answer',{exact:true}).waitFor();
      assert.ok(!(await page.content()).includes('Copper lanterns shelter quiet gardens.'));
      assert.equal((await f.sql.exec("SELECT key FROM records WHERE key LIKE 'practice_attempts/%'")).length,0);
      await page.getByLabel('Your answer',{exact:true}).fill('Copper lanterns shelter quiet gardens.');
      await page.getByRole('button',{name:'Check my answer',exact:true}).tap();await page.locator('[data-state="exact-result"]').waitFor();
      await homeRoundTrip('exact-held');
      row.observed = {held_refresh:true, historical_override_preserved:true, historical_self_check_preserved:true,new_self_rating_refused:410, deliberate_next:true, exact_practice_unchanged:true,home_both_current_unchanged:true,home_concept_held_unchanged:true,home_exact_held_unchanged:true,explicit_resume_and_text_practice:true};
    } catch (error) {
      row.status = 'fail'; row.reason = error.message; row.stack=error.stack;
      console.error('Mixed notebook failure: '+JSON.stringify({noJS,stage:row.stage,reason:row.reason,stack:row.stack}));
      await shot('failure');
    } finally {await context.close(); await f.close();}
  }
  return results;
}
