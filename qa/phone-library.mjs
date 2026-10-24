// First exact Study through native browser forms, backed by the compiled Worker,
// real SQLite and local R2. All wording is authored synthetic text; no provider.
import assert from 'node:assert/strict';
import {feedbackArrival,assertFeedbackArrival,feedbackVariants,fullAnswer} from './feedback-arrival.mjs';
import {randomUUID, createHash} from 'node:crypto';
import {spawn} from 'node:child_process';
import {mkdtemp, writeFile, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import {createRuntime, internalKey} from './runtime-helper.mjs';

const title = 'The quiet lantern garden';
const alias = 'Lantern garden exercise';
const edition = 'First supplied edition';
const source = 'Authored synthetic phone exercise, not canonical wording';
const firstUnit = 'Copper lanterns shelter a quiet garden.';
const text = firstUnit + ' Silver rain marks the sheltered path.';
const otherEdition = 'Second supplied edition';
const otherText = 'Bronze lanterns shelter a patient orchard.';
const variants = feedbackVariants;
const sha = value => createHash('sha256').update(value).digest('hex');
const count = async (f, prefix) => Object.keys(await f.rows()).filter(key => key.startsWith(prefix)).length;
const form = page => page.locator('form[action="/library/version/add"]');
const addButton = page => form(page).getByRole('button');
const versionPath = id => '/library/versions/' + id;
const fields = async locator => {
  await locator.waitFor({state:'visible'});
  return locator.evaluate(form => Object.fromEntries([...form.querySelectorAll('input[name]')].filter(node => !['radio','checkbox'].includes(node.type) || node.checked).map(node => [node.name, node.value])));
};

async function libraryFixture() {
  const temp = await mkdtemp(join(tmpdir(), 'scry-phone-library-'));
  const name = 'phone-library-' + randomUUID();
  const bindings = {SCRY_ENV:'development', CANONICAL_ORIGIN:'http://127.0.0.1:0', SPACE_NAME:name, INTERNAL_KEY:internalKey};
  let runtime, secret = '', egress = 0;
  const transcript = [];
  try {
    runtime = await createRuntime({port:0, name, inspect:true, resourcePersistencePath:join(temp,'state'), bindings, egress:async () => {egress++; throw Error('No egress in supplied-text phone proof.');}});
    const origin = (await runtime.mf.ready).origin;
    runtime = {...runtime, ...await runtime.rebind({...bindings, CANONICAL_ORIGIN:origin})};
    const sql = await runtime.mf.unsafeGetDurableObjectStorage(name, 'LearningSpace', {name});
    const rows = async () => Object.fromEntries((await sql.exec('SELECT key,value FROM records ORDER BY key')).map(row => [row.key, JSON.parse(row.value)]));
    const record = async key => JSON.parse((await sql.exec('SELECT value FROM records WHERE key=?', key))[0].value);
    const post = (path, data) => runtime.worker.fetch(origin + path, {method:'POST', redirect:'manual', headers:{'X-Scry-Test-Origin':origin, 'Content-Type':'application/x-www-form-urlencoded'}, body:new URLSearchParams(data)});
    const settings = await (await runtime.worker.fetch(origin + '/settings')).text();
    const tokenForm = /<form[^>]*action="\/tokens\/create"[^>]*>([\s\S]*?)<\/form>/.exec(settings);
    assert.ok(tokenForm, 'Native synthetic Owner token form exists.');
    const tokenFields = Object.fromEntries([...tokenForm[1].matchAll(/name="(csrf|operation_id|revision)" value="([^"]+)"/g)].map(match => [match[1],match[2]]));
    const minted = await post('/tokens/create', {...tokenFields, name:'Supplied-text phone proof; unpaid'});
    assert.equal(minted.status, 200);
    secret = /<textarea[^>]*>([^<]+)<\/textarea>/.exec(await minted.text())?.[1];
    assert.ok(secret, 'Synthetic Owner token issued once.');
    const cli = (args, input = '') => new Promise((done, reject) => {
      const child = spawn(resolve(process.env.SCRY_STUDY_CLI || 'target/debug/scry'), [...args, '--json', '--config', join(temp,'login.json')], {env:{PATH:process.env.PATH, HOME:temp}, stdio:['pipe','pipe','pipe']});
      let output = '', error = '';
      child.stdout.on('data', data => output += data); child.stderr.on('data', data => error += data);
      child.on('error', reject);
      child.on('close', code => {
        try {
          assert.ok(!output.includes(secret) && !error.includes(secret), 'CLI must never echo a token.');
          assert.equal(code, 0, 'Compiled CLI refused synthetic operation.');
          const value = JSON.parse(output);
          transcript.push({command:['scry', ...args.map(arg => arg.replaceAll(temp,'<fixture>')), '--json'].join(' '), exit:code, output_sha256:sha(output)});
          done(value);
        } catch (error) {reject(error);}
      });
      child.stdin.end(input);
    });
    await cli(['login','--url',origin,'--token-stdin'], secret + '\n');
    const file = async (name, content) => {const path = join(temp,name); await writeFile(path,content); return path;};
    const api = path => runtime.worker.fetch(origin + path, {headers:{Authorization:'Bearer ' + secret, Accept:'application/json'}});
    const backup = async () => {
      const namespace = await runtime.mf.getDurableObjectNamespace('LEARNING_SPACE');
      const response = await namespace.get(namespace.idFromName(name)).fetch('https://internal.invalid/__internal/daily', {method:'POST', headers:{'x-scry-internal-key':internalKey, 'x-scry-owner':'private-owner'}});
      assert.equal(response.status, 200);
      const bucket = await runtime.mf.getR2Bucket('SCRY_BACKUPS');
      const listed = await bucket.list({prefix:`snapshots/${name}/`});
      assert.ok(listed.objects.length, 'Built Worker wrote a complete local archive.');
      const bytes = Buffer.from(await (await bucket.get(listed.objects.at(-1).key)).arrayBuffer());
      return {archive:JSON.parse(bytes), sha256:sha(bytes), bytes:bytes.length};
    };
    return {origin, cli, file, api, post, rows, record, backup, transcript, get egress() {return egress;}, close:async () => {await runtime.mf.dispose(); await rm(temp,{recursive:true,force:true});}};
  } catch (error) {await runtime?.mf.dispose(); await rm(temp,{recursive:true,force:true}); throw error;}
}

async function fillDraft(page, overrides = {}) {
  await page.getByLabel('Title',{exact:true}).fill(overrides.title ?? title);
  await page.getByLabel('Text',{exact:true}).fill(overrides.text ?? text);
  const details = page.locator('details').filter({has:page.getByLabel('Edition',{exact:true})});
  if (!(await details.getAttribute('open')) && !(await page.getByLabel('Edition',{exact:true}).isVisible())) await details.locator('summary').click();
  await page.getByLabel(/^Edition/).fill(overrides.edition ?? edition);
  await page.getByLabel(/^Source/).fill(overrides.source ?? source);
}

async function layout(page, zoom) {
  if (zoom) await page.evaluate(() => {
    // Restore our previous sizing before measuring. AJAX replaces only the main
    // and header, so retaining an already-doubled body would turn inherited
    // sizes in the new main into 400% instead of the requested 200%.
    for (const node of document.querySelectorAll('[data-qa-text-size]')) {
      node.style.removeProperty('font-size'); delete node.dataset.qaTextSize;
    }
    const nodes = [...document.querySelectorAll('body,body *')];
    for (const node of nodes) node.dataset.qaTextSize = String(parseFloat(getComputedStyle(node).fontSize) * 2);
    for (const node of nodes) node.style.fontSize = node.dataset.qaTextSize + 'px';
  });
  await page.evaluate(() => document.fonts.ready);
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, 'No horizontal overflow.');
  for (const locator of [page.getByLabel('Find a text',{exact:true}), page.getByLabel('Your answer',{exact:true})]) {
    if (await locator.count()) assert.ok(await locator.isVisible(), 'The primary field is visible.');
  }
}

async function search(page, query) {
  const field = page.getByLabel('Find a text',{exact:true});
  await field.fill(query);
  await Promise.all([
    page.waitForURL(url => url.pathname === '/library' && url.searchParams.get('q') === query,{waitUntil:'domcontentloaded'}),
    field.press('Enter')
  ]);
}

export async function phoneLibraryWalk(browser, capture) {
  const results = [];
  for (const [width, scheme, noJS, zoom] of variants) {
    const f = await libraryFixture();
    const context = await browser.newContext({viewport:{width,height:900}, colorScheme:scheme, javaScriptEnabled:!noJS, ...(width < 768 ? {isMobile:true,hasTouch:true} : {})});
    const page = await context.newPage(); page.setDefaultTimeout(12000);
    const row = {width,scheme,noJS,text_zoom:zoom?'200%':'100%',status:'pass',criteria:[],evidence:[]};
    results.push(row);
    const shot = async name => {await layout(page,zoom); row.evidence.push(await capture(page,`phone-library-${width}-${scheme}-${zoom?200:100}-${name}.png`));};
    const activate = async locator => {if (width < 768) await locator.tap(); else {await locator.focus(); await page.keyboard.press('Enter');}};
    const selectVersion = async id => {
      await Promise.all([
        page.waitForURL(f.origin + versionPath(id),{waitUntil:'domcontentloaded'}),
        activate(page.locator(`main a[href="${versionPath(id)}"]`))
      ]);
      await page.locator('[data-state="library-version"]').waitFor();
      await page.locator('form[action="/practice/learn"]').waitFor({state:'visible'});
    };
    try {
      await page.goto(f.origin + '/');
      await page.getByLabel('Find a text',{exact:true}).waitFor();
      assert.deepEqual(await page.locator('.masthead nav a').allTextContents(), ['Library']);
      assert.equal(await page.locator('.masthead nav a').first().getAttribute('href'), '/library');
      assert.equal(await page.getByRole('link',{name:'Understanding',exact:true}).getAttribute('href'), '/notebook');
      assert.equal(await count(f,'library_versions/'), 0);
      assert.equal(await count(f,'practice_studies/'), 0);
      await shot('empty');
      await search(page, 'an unknown synthetic text');
      await page.getByText('Nothing matches.',{exact:true}).waitFor();
      await activate(page.locator('main a[href="/library/add"]').first());
      await fillDraft(page, {title:'!!!'});
      const invalid = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/library/version/add');
      await activate(addButton(page));
      assert.equal((await invalid).status(), 422, 'Server validation, not only HTML required fields.');
      await page.getByLabel('Title',{exact:true}).waitFor();
      assert.equal(await page.getByLabel('Title',{exact:true}).inputValue(), '!!!');
      assert.equal(await page.getByLabel('Text',{exact:true}).inputValue(), text);
      assert.equal(await page.getByLabel(/^Edition/).inputValue(), edition);
      assert.equal(await page.getByLabel(/^Source/).inputValue(), source);
      assert.equal(await count(f,'library_versions/'), 0);
      await shot('retained-invalid-draft');
      await page.getByLabel('Title',{exact:true}).fill(title);
      await activate(addButton(page));
      await page.waitForURL(/\/library\/versions\/[^/?]+$/);
      const vid = new URL(page.url()).pathname.split('/').at(-1);
      await page.getByRole('heading',{name:title,level:1,exact:true}).waitFor();
      assert.ok((await page.locator('main').innerText()).includes(edition));
      assert.ok((await page.locator('main').innerText()).includes(source));
      assert.ok((await page.locator('main').innerText()).includes(firstUnit));
      const original = await f.cli(['version','show',vid]);
      assert.equal(original.version.text,text);
      assert.equal(original.version.name,edition);
      assert.equal(original.version.provenance,'supplied');
      assert.equal(original.version.rights,'private_only');
      assert.equal(original.version.sections.flatMap(section => section.units).map(unit => unit.text).join(''),text);
      await shot('version');
      await page.goto(f.origin + '/library/add'); await fillDraft(page); await activate(addButton(page));
      await page.waitForURL(f.origin + versionPath(vid));
      assert.equal(await count(f,'library_versions/'),1);
      assert.equal(await count(f,'library_works/'),1);
      assert.deepEqual((await f.cli(['version','show',vid])).version,original.version);
      // Alias and alternate edition are domain fixtures added by the compiled
      // CLI, not extra web-only writers or fake browser search results.
      await f.cli(['work','edit',original.work.id,'--revision',String(original.work.revision),'--title',title,'--alias',alias]);
      const second = await f.cli(['version','add','--title',alias,'--kind','text','--name',otherEdition,'--source',source,'--text-file',await f.file('second.txt',otherText)]);
      const secondId = second.result.entity_id;
      assert.equal(await count(f,'library_works/'),1);
      for (const query of [title,alias,'Copper lanterns']) {
        await page.goto(f.origin + '/library'); await search(page,query);
        assert.equal(await page.locator(`main a[href="${versionPath(vid)}"]`).count(),1);
        assert.ok((await page.locator('main').innerText()).includes(edition));
      }
      await search(page,alias);
      assert.equal(await page.locator(`main a[href="${versionPath(secondId)}"]`).count(),1);
      assert.ok((await page.locator('main').innerText()).includes(otherEdition));
      await selectVersion(secondId);
      await page.getByRole('heading',{name:title,level:1,exact:true}).waitFor();
      assert.ok((await page.locator('main').innerText()).includes(otherText));
      await page.goto(f.origin + '/library');
      // Edition metadata identifies a result; domain search remains title,
      // alias and opening words. Do not invent a metadata/semantic search.
      await search(page,otherEdition); await page.getByText('Nothing matches.',{exact:true}).waitFor();
      await shot('honest-miss');
      await search(page,alias); await selectVersion(vid);
      const learnFields = await fields(page.locator('form[action="/practice/learn"]'));
      await Promise.all([
        page.waitForURL(f.origin + '/',{waitUntil:'domcontentloaded'}),
        activate(page.getByRole('button',{name:'Learn',exact:true}))
      ]);
      await page.locator('[data-state="exact-prompt"]').waitFor();
      assert.equal(new URL(page.url()).pathname,'/');
      assert.equal(await page.getByLabel('Your answer',{exact:true}).count(),1);
      assert.equal(await page.getByRole('button',{name:'Check my answer',exact:true}).count(),1);
      assert.equal(await count(f,'practice_studies/'),1);
      const prompt = (await f.record('meta')).practice.current.prompt;
      const study = (await f.cli(['learn',vid])).practice.study;
      const replay = await f.post('/practice/learn',learnFields);
      assert.equal(replay.status,303);
      assert.equal(new URL(replay.headers.get('location'),f.origin).pathname,'/');
      assert.equal(await count(f,'practice_studies/'),1);
      assert.deepEqual((await f.record('meta')).practice.current.prompt,prompt);
      await shot('first-prompt');
      await page.setViewportSize({width,height:844});
      await page.getByLabel('Your answer',{exact:true}).fill('Copper lanterns shelter a tired garden.');
      await activate(page.getByRole('button',{name:'Check my answer',exact:true}));
      await page.locator('[data-state="exact-result"]').waitFor();
      row.arrival=await feedbackArrival(page,capture,`phone-library-${width}-${scheme}-${zoom?200:100}-arrival.png`,zoom);
      row.evidence.push(row.arrival.evidence);
      assertFeedbackArrival(row.arrival);
      await page.setViewportSize({width,height:900});
      const attempt = (await f.cli(['next'])).practice.attempt;
      assert.equal(attempt.prompt.id,prompt.id);
      assert.equal(attempt.assisted,true,'First answer-bearing cue is not unaided recall.');
      assert.equal(await page.locator('[data-verdict]').getAttribute('data-verdict'),attempt.grade.verdict);
      assert.deepEqual(await page.locator('[data-diff-kind]').evaluateAll(nodes => nodes.map(node => node.dataset.diffKind)),attempt.grade.diff.map(part => part.kind));
      await page.reload(); await page.locator('[data-state="exact-result"]').waitFor();
      assert.equal((await f.record('meta')).practice.current.attempt_id,attempt.id);
      const closeFull=await fullAnswer(page,f,attempt,original.version.sections[0].units[0].text);
      await shot('held-diff');await closeFull();
      const heldRows=await f.rows();
      await page.goto(f.origin+'/library');await page.goBack();
      await page.locator('[data-state="exact-result"]').waitFor();
      assert.equal((await f.record('meta')).practice.current.attempt_id,attempt.id);
      assert.deepEqual(await f.rows(),heldRows,'Back and read-only Library navigation preserve the held Attempt.');
      await activate(page.getByRole('button',{name:'Next',exact:true}));
      await page.locator('[data-state="exact-prompt"]').waitFor();
      assert.notEqual((await f.record('meta')).practice.current.prompt.id,prompt.id);
      await shot('next');
      for (const [action,status] of [['Pause','paused'],['Resume','active'],['Drop','dropped']]) {
        await page.goto(f.origin + versionPath(vid));
        await activate(page.getByText('Manage Study',{exact:true}));
        await Promise.all([
          page.waitForURL(f.origin + '/',{waitUntil:'domcontentloaded'}),
          activate(page.getByRole('button',{name:action,exact:true}))
        ]);
        await page.locator(status==='dropped'?'[data-state="library"]':'[data-state="exact-prompt"]').waitFor();
        assert.equal((await f.record('practice_studies/'+study.id)).status,status);
        assert.equal(await count(f,'practice_attempts/'),1,'Study controls retain the first Attempt.');
        assert.deepEqual((await f.cli(['version','show',vid])).version,original.version);
      }
      await page.getByLabel('Find a text',{exact:true}).waitFor();
      await shot('dropped-return');
      const complete = await f.cli(['export']);
      assert.deepEqual(complete.app.spend,[]); assert.deepEqual(complete.app.jobs,{}); assert.deepEqual(complete.app.assessments,{});
      const backup = await f.backup();
      assert.deepEqual(backup.archive.app.library,complete.app.library);
      assert.deepEqual(backup.archive.app.practice,complete.app.practice);
      assert.equal(f.egress,0);
      row.criteria.push(6);
      row.observed = {version:vid,study:study.id,attempt:attempt.id,duplicate_versions:0,duplicate_studies:0,first_attempt_assisted:true,held_reload:true,deliberate_next:true,search:'title, alias, opening words; edition-only honest miss',study_controls:['pause','resume','drop'],local_r2:{sha256:backup.sha256,bytes:backup.bytes},egress:0,spend:0};
    } catch (error) {row.status='fail'; row.reason=error.message; row.evidence.push(await capture(page,`phone-library-${width}-${scheme}-${zoom?200:100}-failure.png`));}
    finally {row.cli=f.transcript; await context.close(); await f.close();}
  }
  return results;
}

export async function phoneLibraryRecovery(browser, capture) {
  const results = [];
  const check = async (name, options, run) => {
    const f = await libraryFixture();
    const context = await browser.newContext({viewport:{width:390,height:844},isMobile:true,hasTouch:true,...options});
    const page = await context.newPage(); page.setDefaultTimeout(12000);
    const row = {name,status:'pass',evidence:[]}; results.push(row);
    try {await run({f,context,page,row}); assert.equal(f.egress,0);}
    catch (error) {row.status='fail'; row.reason=error.message;}
    finally {row.evidence.push(await capture(page,`phone-library-recovery-${results.length}.png`)); row.cli=f.transcript; await context.close(); await f.close();}
  };
  const openDraft = async (f,page) => {await page.goto(f.origin+'/library/add'); await fillDraft(page);};
  const payloadField = (body,name) => body.match(new RegExp('name="'+name+'"\\r\\n\\r\\n([\\s\\S]*?)\\r\\n--'))?.[1];
  await check('lost add response reconciles one Version',{},async ({f,context,page,row}) => {
    await openDraft(f,page);
    const payloads=[]; let first=true;
    await context.route('**/library/version/add',async route => {
      if(route.request().method()!=='POST') return route.continue();
      payloads.push(route.request().postData());
      if(first) {first=false; assert.equal((await route.fetch({maxRedirects:0})).status(),303); await route.abort('failed');}
      else await route.continue();
    });
    await addButton(page).tap(); await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).waitFor();
    assert.equal(await count(f,'library_versions/'),1);
    assert.equal(await page.getByLabel('Text',{exact:true}).isEditable(),false);
    await page.getByRole('button',{name:'Reconcile this attempt',exact:true}).tap();
    await page.waitForURL(/\/library\/versions\/[^/?]+$/);
    assert.equal(payloads.length,2);
    for(const name of ['operation_id','revision','title','text']) {assert.ok(payloadField(payloads[0],name)); assert.equal(payloadField(payloads[0],name),payloadField(payloads[1],name));}
    assert.equal(await count(f,'library_versions/'),1); assert.equal(await count(f,'library_works/'),1);
    row.observed={sends:2,versions:1,works:1,frozen_payload_equal:true};
  });
  await check('competing add tabs converge on one Version',{},async ({f,context,page,row}) => {
    await openDraft(f,page); const other=await context.newPage(); await openDraft(f,other);
    const firstFields=await fields(form(page)), secondFields=await fields(form(other));
    assert.ok(firstFields.operation_id && secondFields.operation_id);
    assert.notEqual(firstFields.operation_id,secondFields.operation_id,'Each tab owns its own operation.');
    const firstResponse=page.waitForResponse(response => response.request().method()==='POST' && new URL(response.url()).pathname==='/library/version/add');
    await Promise.all([page.waitForURL(/\/library\/versions\/[^/?]+$/,{waitUntil:'domcontentloaded'}),addButton(page).tap()]);
    assert.equal((await firstResponse).status(),303);
    await page.locator('[data-state="library-version"]').waitFor();
    const destination=page.url(), vid=new URL(destination).pathname.split('/').at(-1);
    const original=(await f.cli(['version','show',vid])).version;
    const secondResponse=other.waitForResponse(response => response.request().method()==='POST' && new URL(response.url()).pathname==='/library/version/add');
    await Promise.all([other.waitForURL(destination,{waitUntil:'domcontentloaded'}),addButton(other).tap()]);
    assert.equal((await secondResponse).status(),303);
    await other.locator('[data-state="library-version"]').waitFor();
    assert.equal(await count(f,'library_works/'),1); assert.equal(await count(f,'library_versions/'),1); assert.equal(await count(f,'practice_studies/'),0);
    assert.deepEqual((await f.cli(['version','show',vid])).version,original,'Competing supplied-text adds preserve the original Version.');
    row.evidence.push(await capture(other,'phone-library-competing-add.png')); await other.close();
    row.observed={version:vid,works:1,versions:1,studies:0,responses:[303,303],distinct_operations:true,original_version_unchanged:true};
  });
  await check('offline Library draft pauses without automatic writes',{},async ({f,context,page,row}) => {
    await openDraft(f,page); let sends=0;
    context.on('request',request => {if(request.method()==='POST'&&new URL(request.url()).pathname==='/library/version/add') sends++;});
    await context.setOffline(true); await addButton(page).tap();
    assert.equal(sends,0); assert.equal(await page.getByLabel('Text',{exact:true}).inputValue(),text);
    await context.setOffline(false); await page.waitForTimeout(400); assert.equal(sends,0);
    assert.equal(await count(f,'library_versions/'),0);
    await addButton(page).tap(); await page.waitForURL(/\/library\/versions\/[^/?]+$/);
    assert.equal(sends,1); assert.equal(await count(f,'library_versions/'),1);
    row.observed={offline_sends:0,reconnect_sends:0,deliberate_sends:1};
  });
  await check('long title and unbroken words remain readable in native disclosure',{viewport:{width:320,height:844},colorScheme:'dark',javaScriptEnabled:false},async ({f,page,row}) => {
    const longTitle='A deliberately long supplied title keeps every word available without displacing the correction from its learning moment';
    const longWord='counterrevolutionary'.repeat(4), longAnswer='misremembered'.repeat(6);
    const added=await f.cli(['version','add','--title',longTitle,'--kind','text','--name',edition,'--source',source,'--text-file',await f.file('long.txt',longWord+'.')]);
    await f.cli(['learn',added.result.entity_id]);
    await page.goto(f.origin+'/');
    await page.getByLabel('Your answer',{exact:true}).fill(longAnswer);
    await page.getByRole('button',{name:'Check my answer',exact:true}).tap();
    await page.locator('[data-state="exact-result"]').waitFor();
    await layout(page,true);
    const original=(await f.cli(['next'])).practice.attempt;
    assert.equal(await page.locator('[data-verdict]').getAttribute('data-verdict'),original.grade.verdict);
    row.evidence.push(await capture(page,'phone-library-long-words-320-dark-200.png'));
    const closeFull=await fullAnswer(page,f,original,longWord+'.');
    assert.equal(await page.locator('.exact-full-answer>h2').first().textContent(),longTitle);
    await layout(page,true);
    for(const selector of ['.exact-full-answer>h2','.diff-text','.exact-expected']) {
      assert.equal(await page.locator(selector).evaluateAll(nodes=>nodes.every(node=>{const rect=node.getBoundingClientRect();return rect.left>=0&&rect.right<=innerWidth&&getComputedStyle(node).overflow!=='hidden';})),true,'Full words wrap rather than clip.');
    }
    row.evidence.push(await capture(page,'phone-library-long-full-answer-320-dark-200.png'));
    await closeFull();
    assert.deepEqual((await f.cli(['next'])).practice.attempt,original);
    row.observed={width:320,text_zoom:'200%',noJS:true,complete_title:true,complete_long_words:true,overflow:false,held_attempt_unchanged:true};
  });
  for (const noJS of [false,true]) await check(`protected Library gates HTML, JSON and Back (${noJS?'native forms':'JavaScript'})`,{javaScriptEnabled:!noJS},async ({f,context,page,row}) => {
    const added=await f.cli(['version','add','--title',title,'--kind','text','--name',edition,'--source',source,'--text-file',await f.file('protected.txt',text)]);
    const vid=added.result.entity_id, shown=await f.cli(['version','show',vid]);
    await f.cli(['learn',vid]);
    await page.goto(f.origin+versionPath(vid));
    assert.ok((await page.locator('main').innerText()).includes(firstUnit));
    await page.goto(f.origin+'/'); await page.locator('[data-state="exact-prompt"]').waitFor();
    let current=(await f.cli(['next'])).practice;
    const cid=(await f.record('meta')).practice.current.targets[0].card_id;
    for(let i=0;i<6;i++) {await f.cli(['answer',current.prompt.id,'--text-stdin'],firstUnit); current=(await f.cli(['next','--after',current.prompt.id,'--card',cid])).practice;}
    assert.equal(current.prompt.cue_level,'previous_line'); assert.equal(current.prompt.assisted,false);
    await page.reload(); await page.locator('[data-state="exact-prompt"]').waitFor();
    assert.ok(!(await page.content()).includes(firstUnit));
    const before=await f.rows(), priorAttempts=await count(f,'practice_attempts/'), card=(await f.record('practice_cards/'+cid)).state;
    await page.goBack(); await page.locator('[data-state="library-gate"]').waitFor();
    assert.ok(!(await page.content()).includes(firstUnit),'Back cannot restore an answer-bearing cached Library page.');
    const paths=['/library','/library?q='+encodeURIComponent(alias),'/library/add',versionPath(vid)];
    for(const path of paths) {
      await page.goto(f.origin+path); await page.locator('[data-state="library-gate"]').waitFor();
      assert.equal(await page.getByRole('button',{name:'Open Library with help',exact:true}).count(),1);
      assert.ok(!(await page.content()).includes(firstUnit));
      assert.ok(!(await page.content()).includes(otherText));
      assert.equal(await page.getByLabel('Find a text',{exact:true}).count(),0);
    }
    for(const path of ['/api/v1/library/search?q=lantern','/api/v1/library/versions/'+vid,'/api/v1/library/works/'+shown.work.id,'/api/v1/library/export']) {
      const response=await f.api(path); assert.equal(response.status,409);
      assert.ok(!(await response.text()).includes(firstUnit));
    }
    const exported=await context.request.get(f.origin+'/export'); assert.equal(exported.status(),409); assert.ok(!(await exported.text()).includes(firstUnit));
    assert.deepEqual(await f.rows(),before,'Protected HTML/JSON reads write no assistance or learning state.');
    const destination=versionPath(vid);
    await page.goto(f.origin+destination); await page.locator('[data-state="library-gate"]').waitFor();
    row.evidence.push(await capture(page,`phone-library-${noJS?'native':'js'}-protected-gate.png`));
    const revealFields=await fields(page.locator('form[action="/practice/reveal"]'));
    let durableBeforeRedirect=false;
    await context.route('**/practice/reveal',async route => {
      const response=await route.fetch({maxRedirects:0});
      assert.equal(response.status(),303);
      const rows=await f.rows();
      durableBeforeRedirect=Object.values(rows).some(value => value?.disclosure===true&&value?.assisted===true&&value?.prompt?.id===current.prompt.id);
      await route.fulfill({response});
    });
    await page.getByRole('button',{name:'Open Library with help',exact:true}).tap();
    await page.locator('[data-state="library-version"]').waitFor();
    assert.equal(new URL(page.url()).pathname+new URL(page.url()).search,destination);
    assert.equal(durableBeforeRedirect,true,'Assistance is durable before the answer-bearing redirect.');
    assert.equal(await count(f,'practice_attempts/'),priorAttempts+1);
    assert.deepEqual((await f.record('practice_cards/'+cid)).state,card);
    await page.goto(f.origin+versionPath(vid)); assert.ok((await page.locator('main').innerText()).includes(firstUnit));
    assert.equal((await f.api('/api/v1/library/versions/'+vid)).status,200);
    await page.reload(); assert.ok((await page.locator('main').innerText()).includes(firstUnit));
    const invalidReturns=['https://outside.invalid/library','//outside.invalid/library','/settings','/library/versions/../settings','/library?next=outside'];
    for(const returnTo of invalidReturns) {
      const response=await f.post('/practice/reveal',{...revealFields,operation_id:randomUUID(),revision:String((await f.record('meta')).revision),return_to:returnTo});
      assert.equal(response.status,303);
      assert.equal(new URL(response.headers.get('location'),f.origin).href,f.origin+'/','Unsupported assistance destinations return only to practice.');
    }
    assert.equal(await count(f,'practice_attempts/'),priorAttempts+1,'Repeated disclosure does not create another Attempt.');
    row.observed={noJS,html_routes:paths,json_refusals:4,export_refusal:409,back_revalidated:true,durable_before_redirect:true,disclosure_attempts_added:1,schedule_unchanged:true,validated_return:destination,invalid_returns_refused:invalidReturns.length};
  });
  return results;
}
