// Mixed exact Study + understanding occurrence, through the real isolated Worker.
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {createRuntime, internalKey} from './runtime-helper.mjs';

export function formFields(html, action) {
  const form = [...html.matchAll(/<form[^>]*action="([^"]+)"[^>]*>([\s\S]*?)<\/form>/g)].find(m => m[1] === action);
  assert.ok(form, 'Expected native form: ' + action);
  return Object.fromEntries([...form[2].matchAll(/name="([^"]+)" value="([^"]*)"/g)].map(m => [m[1], m[2]]));
}

export async function mixedFixture() {
  const name = 'mixed-' + randomUUID();
  const bindings = {SCRY_ENV:'development', CANONICAL_ORIGIN:'http://127.0.0.1:0', SPACE_NAME:name, INTERNAL_KEY:internalKey};
  let egress = 0;
  const runtime = await createRuntime({port:0, name, inspect:true, bindings, egress:async () => {egress++; throw Error('No external effects allowed.');}});
  const origin = (await runtime.mf.ready).origin;
  const {worker} = await runtime.rebind({...bindings, CANONICAL_ORIGIN:origin});
  const sql = await runtime.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name});
  const record = async key => JSON.parse((await sql.exec('SELECT value FROM records WHERE key=?', key))[0].value);
  const html = async path => (await worker.fetch(origin + path)).text();
  const post = (path, fields) => worker.fetch(origin + path, {method:'POST', redirect:'manual', headers:{'Content-Type':'application/x-www-form-urlencoded', 'X-Scry-Test-Origin':origin}, body:new URLSearchParams(fields)});
  assert.equal((await post('/__fixture', formFields(await html('/'), '/__fixture'))).status, 303);
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
  return {origin, html, post, record, sql, practice, close:async () => {await runtime.mf.dispose(); assert.equal(egress, 0);}};
}

export async function mixedHttpProof() {
  const f = await mixedFixture();
  const observed = [];
  try {
    const original = await f.html('/notebook');
    const fields = formFields(original, '/review/answer');
    const before = await f.sql.exec('SELECT key,value FROM records ORDER BY key');
    for (const [status, extra] of [[409, {occurrence_id:'stale-occurrence', answer:'invalid'}], [422, {answer:''}]]) {
      const r = await f.post('/review/answer', {...fields, ...extra});
      assert.equal(r.status, status);
      const page = await r.text();
      assert.ok(page.includes('data-state="question"'), 'Rejected legacy answer retains its question view');
      assert.ok(!page.includes('data-state="exact-'), 'Legacy refusal must not render exact practice');
      assert.deepEqual(await f.sql.exec('SELECT key,value FROM records ORDER BY key'), before);
      observed.push({status, view:'legacy question', rows_unchanged:true});
    }
    const answer = 'The origin content definitely changed';
    const r = await f.post('/review/answer', {...fields, answer});
    assert.equal(r.status, 303);
    assert.equal(new URL(r.headers.get('location'), f.origin).pathname, '/notebook');
    assert.ok((await f.html('/notebook')).includes('data-state="result"'), 'Answer remains held');
    assert.ok((await f.html('/')).includes('data-state="exact-prompt"'), 'Exact Stream remains current');
    assert.deepEqual((await f.record('meta')).practice, f.practice);
    // Reconcile the original browser operation, including its old shared "/" receipt.
    const saved = await f.sql.exec('SELECT key,value FROM records ORDER BY key');
    const replay = await f.post('/review/answer', {...fields, answer});
    assert.equal(new URL(replay.headers.get('location'), f.origin).pathname, '/notebook');
    assert.deepEqual(await f.sql.exec('SELECT key,value FROM records ORDER BY key'), saved);
    observed.push({answer_redirect:'/notebook', held_result:true, replay_rows_unchanged:true, exact_practice_unchanged:true});
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
    const row = {noJS, status:'pass', evidence:[]};
    results.push(row);
    const shot = async name => row.evidence.push(await capture(page, `mixed-${noJS?'native':'js'}-${name}.png`));
    try {
      await page.goto(f.origin + '/notebook');
      const stale = await context.newPage();
      await stale.goto(f.origin + '/notebook');
      await page.getByRole('button', {name:'The origin content definitely changed', exact:true}).tap();
      await page.locator('[data-state="result"]').waitFor();
      assert.equal(new URL(page.url()).pathname, '/notebook');
      await page.reload();
      await page.locator('[data-state="result"]').waitFor();
      await shot('held-result');
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
      await page.getByRole('button', {name:'I was right', exact:true}).tap();
      await page.getByText('You corrected this grade', {exact:false}).waitFor();
      await shot('override');
      await page.getByRole('button', {name:'Next question', exact:false}).tap();
      await page.locator('#answer').fill('My own uncertain freshness answer');
      await page.getByRole('button', {name:'Check my answer', exact:true}).tap();
      // No provider configuration: the saved meaning check becomes self-check.
      await page.waitForTimeout(1300);
      await page.reload();
      await page.locator('[data-state="self-check"]').waitFor();
      assert.equal(new URL(page.url()).pathname, '/notebook');
      await shot('self-check');
      await page.getByRole('button', {name:'I was right', exact:true}).tap();
      await page.locator('[data-state="result"]').waitFor();
      await shot('learner-result');
      await page.getByRole('button', {name:'Next question', exact:false}).tap();
      assert.equal(new URL(page.url()).pathname, '/notebook');
      assert.deepEqual((await f.record('meta')).practice, f.practice);
      await page.goto(f.origin + '/');
      await page.locator('[data-state="exact-prompt"]').waitFor();
      assert.deepEqual((await f.record('meta')).practice, f.practice);
      await shot('exact-unchanged');
      row.observed = {held_refresh:true, override:true, self_check:true, deliberate_next:true, exact_practice_unchanged:true};
    } catch (error) {
      row.status = 'fail'; row.reason = error.message;
      await shot('failure');
    } finally {await context.close(); await f.close();}
  }
  return results;
}
