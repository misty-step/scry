// Saved History journeys against the built Worker and compiled CLI. All text is
// synthetic; exact History and native disclosures must not mutate Practice.
// Existing legacy History reads may still record concept exposure.
import assert from 'node:assert/strict';
import {mkdtemp,writeFile,rm} from 'node:fs/promises';
import {join} from 'node:path';
import {tmpdir} from 'node:os';

const entries = 'article.history-entry.exact-history-entry';
const gateText = 'History is hidden while you answer from memory.';
// HTML parsing canonicalizes CRLF even though the saved answer retains its bytes.
const htmlText = text => text.replace(/\r\n?/g,'\n');
const status = effect => effect.assisted ? 'assisted' : effect.extra_practice ? 'extra' : effect.scheduled ? 'scheduled' : 'unchanged';
const word = part => ({kind:part.kind,unit_index:part.unit_index,text:part.kind==='correct'||part.kind==='missing'?part.expected.text:part.kind==='extra'?part.typed.text:part.typed.text+' → '+part.expected.text});
const savedOrder = history => Object.values(history.attempts).sort((a,b)=>b.sequence-a.sequence);
const exactHistoryState = rows => ({
  practice:rows.meta.practice,
  rows:Object.fromEntries(Object.entries(rows).filter(([key])=>key.startsWith('practice_')||key.startsWith('study_version_refs/')||key.startsWith('operations/'))),
});
const legacyEvents = rows => Object.fromEntries(Object.entries(rows).filter(([key])=>key.startsWith('events/')));
const assertExactReadOnly = (before,after) => {
  assert.deepEqual(exactHistoryState(after),exactHistoryState(before),'History leaves exact Practice/current, immutable payloads, Corrections, Study references and mutation receipts unchanged.');
  assert.deepEqual(legacyEvents(after),legacyEvents(before),'Legacy concept exposure never rewrites original legacy events.');
};
const entry = (page,id) => page.locator(`${entries}[data-attempt-id="${id}"]`);
const activate = async (page,locator,key='Enter') => {await locator.focus(); await page.keyboard.press(key);};
const openHistory = async page => {
  await activate(page,page.locator('footer').getByRole('link',{name:'History',exact:true}));
  await page.locator('[data-state="history"]').waitFor();
  assert.equal(new URL(page.url()).pathname,'/history');
};
const disclosure = async (page,id,open=true) => {
  const details=entry(page,id).locator('details').first();
  assert.equal(await details.locator('summary').evaluate(node=>[...node.childNodes].filter(child=>child.nodeType!==1||child.getAttribute('aria-hidden')!=='true').map(child=>child.textContent).join('').trim()),'See this attempt');
  if ((await details.getAttribute('open')!==null)!==open) await activate(page,details.locator('summary'),open?'Space':'Enter');
  assert.equal(await details.getAttribute('open')!==null,open,'Native History disclosure follows keyboard activation.');
  assert.equal(await details.locator('summary').evaluate(node=>node===document.activeElement),true);
  return details;
};

async function historyParity(f) {
  const before=await f.rows();
  const history=await f.cli(['history']), exported=await f.cli(['export']);
  for(const key of ['studies','cards','attempts','corrections']) assert.deepEqual(history[key],exported.app.practice[key],`Compiled CLI history/export retain the same ${key}.`);
  for(const [id,version] of Object.entries(history.versions)) assert.deepEqual(version,exported.app.library.versions[id]);
  assert.deepEqual(exported.app.spend,[],'Exact History does not create provider spend.');
  assert.deepEqual(await f.rows(),before,'CLI History and export do not update receipts, schedules or last-use.');
  return history;
}

async function assertHistory(page,history) {
  assert.equal(await page.getByText('Your first practice will appear here.',{exact:true}).count(),0,'Saved exact Attempts are not an empty History.');
  const rendered=await page.locator(entries).evaluateAll(nodes=>nodes.map(node=>({
    id:node.dataset.attemptId,disclosure:node.dataset.disclosure,
    title:node.querySelector('h2')?.textContent,
    meta:node.querySelector('.history-meta')?.textContent,
    version:node.querySelector('h2 + .fineprint')?.textContent,
    context:node.querySelector('.exact-context .reading')?.textContent??null,
    cue:node.querySelector('.exact-cue .reading')?.textContent??'',
    text:node.textContent,
    answer:node.querySelector('.exact-history-answer')?.textContent,
    expected:[...node.querySelectorAll('.exact-expected')].map(p=>p.textContent),
    authority:node.querySelector('.exact-authority')?.textContent,
    diff:[...node.querySelectorAll('[data-diff-kind]')].map(part=>({kind:part.dataset.diffKind,unit_index:Number(part.dataset.unitIndex),text:part.querySelector('.diff-text')?.textContent})),
    effects:[...node.querySelectorAll('li[data-schedule-effect]')].map(part=>({unit_index:Number(part.dataset.unitIndex),verdict:part.dataset.verdict??null,status:part.dataset.scheduleEffect,text:part.textContent})),
    corrections:[...node.querySelectorAll('[data-correction-id]')].map(part=>({id:part.dataset.correctionId,text:part.textContent})),
    forms:node.querySelectorAll('form').length,
  })));
  const ordered=savedOrder(history);
  assert.deepEqual(rendered.map(row=>row.id),ordered.map(attempt=>attempt.id),'History orders saved sequence newest first, not random IDs or wall-clock ties.');
  for(const [index,a] of ordered.entries()) {
    const row=rendered[index],shown=a.disclosure===true;
    assert.equal(row.disclosure,String(shown));
    assert.equal(row.title,a.prompt.title,'History uses the saved Prompt title.');
    assert.ok(row.meta.includes(shown?'Shown before answering':a.grade.verdict));
    if(shown) assert.ok(!row.meta.includes('Again'),'A Show me disclosure is not a submitted miss.');
    assert.equal(row.answer,shown?'No answer submitted.':a.raw_answer===''?'No words submitted.':htmlText(a.raw_answer));
    const version=history.versions[a.prompt.version_id];
    assert.ok(row.version?.includes(version.name.trim()?version.name:'Supplied text'));
    assert.equal(row.context,a.prompt.context===null?null:htmlText(a.prompt.context));
    assert.equal(row.cue,htmlText(a.prompt.cue));
    assert.ok(row.text.includes(a.version_id)); assert.ok(row.text.includes(a.version_hash)); assert.ok(row.text.includes(a.algorithm));
    const units=new Map(version.sections.flatMap(section=>section.units).map(unit=>[unit.id,unit.text]));
    assert.deepEqual(row.expected,a.prompt.unit_ids.map(id=>htmlText(units.get(id))),'Expected wording comes from the saved Version/Unit IDs in Prompt order.');
    assert.ok(row.authority?.includes(a.grade.authority.name));
    assert.ok(row.authority?.includes(a.grade.authority.grader_version));
    assert.deepEqual(row.diff,a.grade.diff.map(word),'Every saved Diff edit and Unit attribution survives in History.');
    assert.deepEqual(row.effects.map(({text,...effect})=>effect),a.effects.map(effect=>({unit_index:effect.unit_index,verdict:shown?null:effect.verdict,status:status(effect)})),'Scheduling status belongs to each Unit, not the Attempt as a whole.');
    for(const effect of row.effects) {
      assert.ok(effect.text.includes(`Unit ${effect.unit_index+1}`));
      if(shown) assert.ok(!effect.text.includes('Again'),'Disclosure effects must not invent a submitted Verdict.');
      else assert.ok(effect.text.includes(effect.verdict));
    }
    const corrections=Object.values(history.corrections).filter(c=>c.attempt_id===a.id).sort((a,b)=>a.sequence-b.sequence);
    assert.deepEqual(row.corrections.map(c=>c.id),corrections.map(c=>c.id));
    for(const [i,c] of corrections.entries()) {
      assert.ok(row.corrections[i].text.includes('Correction'));
      assert.ok(row.corrections[i].text.includes(c.verdict));
      assert.ok(row.corrections[i].text.includes(c.authority),'A Correction retains its separate saved Authority.');
      const effect=a.effects.find(effect=>effect.unit_id===c.unit_id);
      assert.ok(row.corrections[i].text.includes(`Unit ${effect.unit_index+1}`));
    }
    assert.equal(row.forms,0,'Historical results are read-only, not live correction forms.');
  }
  return rendered;
}

async function assertNoHistory(page,privateValues) {
  const html=await page.content();
  for(const value of privateValues.filter(Boolean)) assert.ok(!html.includes(value),`Protected History contains no saved content or IDs: ${value}`);
  for(const selector of ['article.history-entry','.token-history','[data-unit-index]',entries,'[data-attempt-id]','[data-correction-id]','[data-diff-kind]','.exact-history-answer','.exact-expected','script[type="application/json"]']) assert.equal(await page.locator(selector).count(),0,`No historical payload behind the gate: ${selector}`);
}

async function assertGate(page,f,privateValues) {
  await page.getByText(gateText,{exact:true}).waitFor();
  await assertNoHistory(page,privateValues);
  assert.equal(await page.locator('form[action^="/practice/"]').count(),0,'Reading History cannot record assistance.');
  const response=await page.request.get(f.origin+'/history');
  assert.equal(response.status(),200);
  assert.match(response.headers()['cache-control'],/no-store/);
  const raw=await response.text();
  for(const value of privateValues.filter(Boolean)) assert.ok(!raw.includes(value),'Protected server HTML is safe before JavaScript runs.');
}

// Called in every existing 320/390/desktop, light/dark, 200% and no-JS variant.
export async function phoneHistory({f,page,row,original,typography,record}) {
  const history=await historyParity(f), before=await f.rows();
  const shown=savedOrder(history).find(a=>a.disclosure);
  assert.ok(shown,'Existing Show me journey saved a disclosure.');
  assert.deepEqual(history.attempts[original.id],original,'The original Attempt survives its later Correction.');
  assert.equal(Object.values(history.corrections).filter(c=>c.attempt_id===original.id).length,1);
  await openHistory(page);
  await assertHistory(page,history);
  assert.equal(await page.locator(`${entries} details[open]`).count(),0);
  await typography(); await record('history');
  const details=await disclosure(page,original.id);
  assert.equal(await details.locator('.exact-history-answer').isVisible(),true);
  assert.equal(await details.locator('[data-correction-id]').isVisible(),true);
  await typography(); await record('history-original-correction');
  await disclosure(page,original.id,false);
  await page.reload(); await page.locator('[data-state="history"]').waitFor();
  assert.equal(await page.locator(`${entries} details[open]`).count(),0);
  await assertHistory(page,history);
  await disclosure(page,shown.id);
  assert.ok((await entry(page,shown.id).innerText()).includes('No answer submitted.'));
  await typography(); await record('history-disclosure');
  assert.deepEqual(await f.rows(),before,'History, reload and keyboard disclosures write no events, attempts, Corrections or schedules.');
  await activate(page,page.locator('footer').getByRole('link',{name:'Text practice',exact:true}));
  await page.getByText('Assistance is recorded.',{exact:true}).waitFor();
  assert.equal(await page.getByLabel('Your answer',{exact:true}).inputValue(),'');
  await activate(page,page.getByRole('button',{name:'Check my answer',exact:true}));
  await page.locator('[data-state="exact-result"]').waitFor();
  const submitted=(await f.cli(['next'])).practice.attempt;
  assert.equal(submitted.raw_answer,''); assert.notEqual(submitted.disclosure,true); assert.equal(submitted.grade.verdict,'Again');
  const afterEmpty=await historyParity(f), saved=await f.rows();
  assert.deepEqual(afterEmpty.attempts[original.id],original);
  assert.deepEqual(afterEmpty.attempts[shown.id],shown,'Submitting empty text does not rewrite the earlier disclosure.');
  await openHistory(page); await assertHistory(page,afterEmpty);
  await disclosure(page,submitted.id); await typography(); await record('history-empty-answer');
  assert.deepEqual(await f.rows(),saved);
  row.history={attempts:Object.keys(afterEmpty.attempts).length,original:original.id,disclosure:shown.id,empty_answer:submitted.id,cli_export_parity:true,read_only:true,native_keyboard:true,criteria:['US-016.9','US-016.10','US-017.9','US-017.10','US-007.2']};
}

// Optional pre-token fixture setup uses the existing authored demo and native
// form routes, not SQL writes or a second production-only seed mechanism.
export async function seedHistoryNotebook({worker,origin,record}) {
  const post=(path,fields)=>worker.fetch(origin+path,{method:'POST',redirect:'manual',headers:{'X-Scry-Test-Origin':origin,'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams(fields)});
  const inputs=html=>Object.fromEntries([...html.matchAll(/name="(csrf|operation_id|revision|occurrence_id)" value="([^"]+)"/g)].map(match=>[match[1],match[2]]));
  const empty=await(await worker.fetch(origin+'/notebook')).text();
  assert.equal((await post('/__fixture',inputs(empty))).status,303);
  const notebook=await(await worker.fetch(origin+'/notebook')).text();
  const form=/<form[^>]*action="\/review\/answer"[^>]*>([\s\S]*?)<\/form>/.exec(notebook);
  assert.ok(form);
  const occurrence=(await record('meta')).occurrence;
  assert.equal((await post('/review/answer',{...inputs(form[1]),answer:occurrence.presentation.answer})).status,303);
}

export async function phoneHistoryRecovery(browser,capture,fixture,{previousText,phoneText,phoneAnswer}) {
  const results=[];
  const check=async(name,options,run)=>{
    const {legacy=false,...browserOptions}=options;
    const f=await fixture({legacy}),context=await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,...browserOptions});
    const page=await context.newPage(); page.setDefaultTimeout(12000);
    const row={name,status:'pass',evidence:[]}; results.push(row);
    const shot=async suffix=>row.evidence.push(await capture(page,`phone-history-${results.length}-${suffix}.png`));
    try {await page.goto(f.origin+'/recall'); await run({f,page,context,row,shot}); assert.equal(f.egress,0);}
    catch(error) {row.status='fail'; row.reason=error.message;}
    finally {await shot(row.status); await context.close(); await f.close();}
  };
  const answer=async(page,text)=>{await page.getByLabel('Your answer',{exact:true}).fill(text); await activate(page,page.getByRole('button',{name:'Check my answer',exact:true})); await page.locator('[data-state="exact-result"]').waitFor();};

  await check('History keeps frozen wording, mixed Unit effects and dropped Studies',{},async({f,page,row,shot})=>{
    await page.locator('details.overflow>summary').tap(); await page.getByRole('button',{name:'Show me',exact:true}).tap();
    await page.getByText('Assistance is recorded.',{exact:true}).waitFor();
    await answer(page,'');
    const empty=(await f.cli(['next'])).practice.attempt;
    const prompt=(await f.cli(['next','--after',empty.prompt.id,'--study',f.sid,'--recite','whole'])).practice.prompt;
    await page.reload(); await page.locator('[data-state="exact-prompt"]').waitFor();
    // Real textarea form serialization may save CRLF. Compare its durable bytes
    // through CLI/export, and only normalize when asserting parsed HTML text.
    await answer(page,previousText+phoneAnswer+'\n');
    const mixed=(await f.cli(['next'])).practice.attempt;
    assert.equal(mixed.prompt.id,prompt.id);
    assert.deepEqual(mixed.effects.map(status),['extra','scheduled']);
    assert.deepEqual(mixed.effects.map(e=>e.verdict),['Good','Again']);
    const original=await historyParity(f),version=(await f.cli(['version','show',f.vid]));
    const changedTitle='Renamed current Work, not the saved Prompt';
    await f.cli(['work','edit',version.work.id,'--revision',String(version.work.revision),'--title',changedTitle]);
    const work=(await f.cli(['work','show',version.work.id])).work;
    const temp=await mkdtemp(join(tmpdir(),'scry-history-wording-'));
    let replacement;
    try {
      const file=join(temp,'replacement.txt'); await writeFile(file,'Replacement wording must never rewrite a saved Attempt.\n');
      replacement=(await f.cli(['version','supersede',f.vid,'--revision',String(work.revision),'--title',changedTitle,'--kind','text','--text-file',file,'--name','Replacement synthetic edition'])).result.entity_id;
    } finally {await rm(temp,{recursive:true,force:true});}
    assert.notEqual(replacement,f.vid);
    await f.cli(['study','drop',f.sid]);
    const history=await historyParity(f), before=await f.rows();
    assert.equal(history.studies[f.sid].status,'dropped');
    assert.deepEqual(history.attempts,original.attempts); assert.deepEqual(history.versions[f.vid],version.version);
    await openHistory(page); await assertHistory(page,history); await disclosure(page,mixed.id);
    assert.equal(await entry(page,mixed.id).locator('h2').first().textContent(),mixed.prompt.title);
    assert.ok(!(await entry(page,mixed.id).textContent()).includes('Replacement wording'));
    assert.ok(!(await entry(page,mixed.id).textContent()).includes(changedTitle));
    await shot('mixed-dropped'); await page.reload(); await page.locator('[data-state="history"]').waitFor(); await assertHistory(page,history);
    assert.deepEqual(await f.rows(),before);
    row.observed={mixed_effects:['extra','scheduled'],unit_verdicts:['Good','Again'],superseded_version:replacement,frozen_version:f.vid,dropped_study:f.sid,original_attempts_unchanged:true};
  });

  for(const noJS of [false,true]) await check(`History revalidates newer protected Prompt, Back, Forward and stale tab (${noJS?'native':'JavaScript'})`,{javaScriptEnabled:!noJS},async({f,page,context,row,shot})=>{
    await answer(page,phoneText);
    const history=await historyParity(f), saved=(await f.cli(['next'])).practice.attempt;
    await openHistory(page); await assertHistory(page,history); await disclosure(page,saved.id);
    const stale=await context.newPage(); await stale.goto(f.origin+'/history'); await stale.locator('[data-state="history"]').waitFor();
    const offline=await context.newPage(); await offline.goto(f.origin+'/history'); await offline.locator('[data-state="history"]').waitFor();
    await disclosure(offline,saved.id);
    const firstCard=Object.values(history.cards).find(card=>card.unit_index===0);
    const newer=(await f.cli(['next','--after',saved.prompt.id,'--card',firstCard.id])).practice.prompt;
    assert.equal(newer.assisted,false); assert.notEqual(newer.id,saved.prompt.id);
    const before=await f.rows(), privateValues=[phoneText.trim(),previousText.trim(),phoneAnswer,...Object.keys(history.attempts),...Object.keys(history.corrections)];
    await activate(page,page.locator('footer').getByRole('link',{name:'Text practice',exact:true})); await page.locator('[data-state="exact-prompt"]').waitFor();
    await page.goBack(); await assertGate(page,f,privateValues); await shot('back-gate');
    await page.goForward(); await page.locator('[data-state="exact-prompt"]').waitFor();
    assert.equal(await page.locator('[data-state="exact-prompt"]').getAttribute('data-prompt-id'),newer.id);
    await page.goBack(); await assertGate(page,f,privateValues);
    await stale.reload(); await assertGate(stale,f,privateValues); await stale.close();
    // This separate tab still has the original expanded answer-bearing History
    // in its Back stack, rather than the safe gate installed by the online walk.
    await activate(offline,offline.locator('footer').getByRole('link',{name:'Text practice',exact:true})); await offline.locator('[data-state="exact-prompt"]').waitFor();
    await offline.waitForLoadState('load');
    let writes=0;
    const countWrite=request=>{if(request.method()==='POST')writes++;};
    context.on('request',countWrite);
    let offlineNavigation='completed';
    await context.setOffline(true);
    try {
      // Arm fresh events after the old document has loaded. The failed request
      // rejects goBack before Chromium commits its error document, so checking
      // the old document's cached load state is not a navigation barrier. Match
      // the main-frame commit and its new load without guessing the error URL.
      await Promise.all([
        offline.waitForEvent('framenavigated',{predicate:frame=>frame===offline.mainFrame()}),
        offline.waitForEvent('load'),
        (async()=>{
          try {await offline.goBack({waitUntil:'domcontentloaded'});}
          catch(error) {
            assert.match(error.message,/net::ERR_INTERNET_DISCONNECTED/,'Only the explicit offline navigation failure is expected.');
            offlineNavigation='ERR_INTERNET_DISCONNECTED';
          }
        })(),
      ]);
      await assertNoHistory(offline,privateValues);
      row.evidence.push(await capture(offline,`phone-history-${results.length}-offline-return.png`));
      assert.equal(writes,0,'Offline Back never queues an answer or assistance.');
      assert.deepEqual(await f.rows(),before);
    } finally {await context.setOffline(false);}
    // A deliberate fresh GET after reconnection must recheck the newer Prompt;
    // do not treat an offline browser error page as a saved application result.
    await offline.goto(f.origin+'/history'); await assertGate(offline,f,privateValues);
    assert.equal(writes,0,'Reconnecting and revalidating History send no mutation.');
    context.off('request',countWrite); await offline.close();
    assert.deepEqual(await f.rows(),before,'Protected navigation does not reveal, advance or reschedule.');
    await activate(page,page.locator('footer').getByRole('link',{name:'Text practice',exact:true})); await page.locator('[data-state="exact-prompt"]').waitFor();
    await answer(page,previousText);
    await openHistory(page); await assertHistory(page,await historyParity(f));
    row.observed={noJS,back_revalidated:true,forward_current_prompt:true,stale_tab_reload_gated:true,offline_return_private_history:false,offline_navigation:offlineNavigation,online_return_revalidated:true,offline_and_reconnect_writes:0,raw_html_safe:true,read_only_gate:true};
  });

  await check('History disclosures stay read-only offline and reconnect sends nothing',{},async({f,page,context,row})=>{
    await answer(page,phoneAnswer); const history=await historyParity(f), a=savedOrder(history)[0];
    await openHistory(page); const before=await f.rows();
    let writes=0; context.on('request',request=>{if(request.method()==='POST')writes++;});
    await context.setOffline(true); await page.getByText('You are offline. Your draft is still here.',{exact:true}).waitFor();
    await disclosure(page,a.id); await assertHistory(page,history); await disclosure(page,a.id,false);
    assert.equal(writes,0); assert.deepEqual(await f.rows(),before);
    await context.setOffline(false); await page.waitForTimeout(400);
    assert.equal(writes,0); await page.reload(); await page.locator('[data-state="history"]').waitFor();
    await assertHistory(page,history); assert.deepEqual(await f.rows(),before);
    row.observed={offline_disclosures:true,offline_writes:0,reconnect_writes:0,reload_same_history:true};
  });

  for(const noJS of [false,true]) await check(`History does not survive private access loss on reload or Back (${noJS?'native':'JavaScript'})`,{javaScriptEnabled:!noJS},async({f,page,context,row})=>{
    await answer(page,phoneAnswer); const history=await historyParity(f), before=await f.rows();
    await openHistory(page); await disclosure(page,savedOrder(history)[0].id);
    // Intercepted denial is explicit browser recovery proof, not hosted ingress.
    await context.route(f.origin+'/**',route=>route.request().isNavigationRequest()?route.fulfill({status:401,contentType:'text/plain',headers:{'Cache-Control':'private, no-store, max-age=0'},body:'Owner authorization expired.'}):route.continue());
    assert.equal((await page.reload()).status(),401);
    await page.getByText('Owner authorization expired.',{exact:true}).waitFor();
    assert.equal(await page.locator(entries).count(),0); assert.ok(!(await page.content()).includes(phoneAnswer));
    await page.goBack(); await page.getByText('Owner authorization expired.',{exact:true}).waitFor();
    assert.equal(await page.locator('#main').count(),0); assert.ok(!(await page.content()).includes(phoneAnswer));
    assert.deepEqual(await f.rows(),before);
    row.observed={noJS,synthetic_denial:401,private_history_after_reload:false,private_history_after_back:false};
  });

  await check('History retains legacy Attempts and gates all history during protected exact recall',{legacy:true},async({f,page,row,shot})=>{
    const protectedRows=await f.rows();
    const privateIds=Object.keys(protectedRows).filter(key=>key.startsWith('practice_attempts/')).map(key=>key.slice('practice_attempts/'.length));
    await openHistory(page); await assertGate(page,f,[phoneText.trim(),previousText.trim(),...privateIds]);
    assertExactReadOnly(protectedRows,await f.rows());
    await shot('mixed-history-gate');
    await activate(page,page.locator('footer').getByRole('link',{name:'Text practice',exact:true})); await page.locator('[data-state="exact-prompt"]').waitFor();
    await answer(page,phoneAnswer); const history=await historyParity(f), original=(await f.cli(['export'])).app.events;
    assert.equal(original.length,1);
    await openHistory(page); await assertHistory(page,history);
    const legacy=page.locator('article.history-entry:not(.exact-history-entry)');
    assert.equal(await legacy.count(),1);
    assert.equal(await legacy.locator('h2').textContent(),original[0].presentation.prompt);
    const order=await page.locator('article.history-entry').evaluateAll(nodes=>nodes.map(node=>node.classList.contains('exact-history-entry')));
    assert.deepEqual(order,[...savedOrder(history).map(()=>true),false],'Exact saved Attempts precede preserved legacy history.');
    await page.goto(f.origin+'/notebook'); await page.locator('[data-state="result"]').waitFor();
    await page.getByRole('button',{name:'Next question'}).tap(); await page.locator('[data-state="question"]').waitFor();
    // Next records fresh concept exposure: this immediate legacy question is
    // assisted, not a cold-recall fixture. Cold legacy gating has native proof.
    assert.equal((await f.record('meta')).occurrence.assisted,true);
    const before=await f.rows(); await openHistory(page); await assertHistory(page,history);
    assert.equal(await legacy.count(),1);
    assertExactReadOnly(before,await f.rows());
    await page.goto(f.origin+'/notebook'); await page.locator('#answer').fill('max-age');
    await page.getByRole('button',{name:/Check my answer/}).tap(); await page.locator('[data-state="result"]').waitFor();
    await openHistory(page); await assertHistory(page,await historyParity(f));
    assert.equal(await legacy.count(),2);
    const after=(await f.cli(['export'])).app.events;
    assert.deepEqual(after[0],original[0],'New exact and legacy practice never rewrite the first legacy Attempt.');
    row.observed={legacy_attempts_retained:2,protected_exact_gates_all_history:true,exact_entries_before_legacy:true,assisted_legacy_remains_readable:true,limit:'Cold protected legacy question is verified in native rendering tests, not this immediate-Next browser fixture.'};
  });
  return results;
}
