// Native reading/disclosure only, against an already-restored built Worker.
// The caller owns the isolated runtime, typed fixture and SQLite/R2 inspection.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {existsSync} from 'node:fs';
import {mkdir, readFile, rm, writeFile} from 'node:fs/promises';
import {createRequire} from 'node:module';
import {join, resolve} from 'node:path';

const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const domText = text => text.replace(/\r\n?/g, '\n'); // HTML parsing normalizes CRLF; raw HTTP proof retains original bytes.
const markerText = 'Run-owned synthetic Material browser proof only.\n';

async function textLayout(page, doubled) {
  const sizes = await page.evaluate(doubled => {
    // Recalculate from original styles after navigation rather than doubling an
    // inherited size twice. This is text-only 200%, not viewport/CSS zoom.
    for (const node of document.querySelectorAll('[data-qa-text-size]')) {
      node.style.removeProperty('font-size'); delete node.dataset.qaTextSize;
    }
    const nodes = [...document.querySelectorAll('body,body *')];
    const original = nodes.map(node => parseFloat(getComputedStyle(node).fontSize));
    if (doubled) {
      nodes.forEach((node, i) => {node.dataset.qaTextSize = String(original[i] * 2);});
      nodes.forEach(node => {node.style.fontSize = node.dataset.qaTextSize + 'px';});
    }
    return nodes.map((node, i) => ({before: original[i], after: parseFloat(getComputedStyle(node).fontSize)}));
  }, doubled);
  for (const size of sizes) assert.ok(Math.abs(size.after - size.before * (doubled ? 2 : 1)) < 0.1,
    'Every computed text size must be exactly the requested 100% or 200%.');
  await page.evaluate(() => document.fonts.ready);
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false,
    'Home, current text and expanded earlier text must not overflow horizontally.');
  for (const text of await page.locator('[data-material-revision-id] .reading').all()) {
    if (!(await text.isVisible())) continue;
    assert.equal(await text.evaluate(node => getComputedStyle(node).whiteSpace), 'pre-wrap');
    assert.equal(await text.evaluate(node => node.scrollWidth > node.clientWidth + 1), false, 'Material wording must not be horizontally clipped.');
  }
}

async function exactMaterials(page, app, conceptId, expanded) {
  const detail = page.locator('article[data-state="concept"]');
  await detail.waitFor();
  assert.equal(await detail.locator('h1').textContent(), domText(app.concepts[conceptId].name));
  assert.equal(await detail.locator('form,button,input,textarea').count(), 0, 'Authored reading offers no mutation controls.');
  const study = detail.locator('section.concept-study[aria-label="Study materials"]');
  const materials = Object.values(app.materials.items).filter(material => material.concept_id === conceptId);
  assert.deepEqual((await study.locator('article[data-material-id]').evaluateAll(nodes => nodes.map(node => node.dataset.materialId))).sort(),
    materials.map(material => material.id).sort());
  for (const material of materials) {
    const article = study.locator(`article[data-material-id="${material.id}"]`);
    const revisions = Object.values(app.materials.revisions).filter(revision => revision.material_id === material.id)
      .sort((a, b) => a.ordinal - b.ordinal);
    const earlier = revisions.filter(revision => revision.id !== material.current_revision_id);
    assert.deepEqual(await article.locator('section[data-material-revision-id]').evaluateAll(nodes => nodes.map(node => node.dataset.materialRevisionId)),
      [material.current_revision_id, ...earlier.map(revision => revision.id)]);
    for (const revision of revisions) {
      const section = article.locator(`section[data-material-revision-id="${revision.id}"]`);
      assert.equal(await section.locator('h3').textContent(), domText(revision.title));
      const wording = section.locator('div.reading.preserve');
      assert.equal(await wording.textContent(), domText(revision.text), 'Browser wording preserves accents, spacing, tabs and trailing newlines.');
      assert.equal(await wording.evaluate(node => node.childElementCount), 0, 'Supplied wording remains plaintext, never rendered markup.');
      assert.equal(await wording.isVisible(), expanded || revision.id === material.current_revision_id);
    }
    assert.equal(await article.locator('details.saved-input').count(), earlier.length ? 1 : 0);
    if (earlier.length) {
      const details = article.locator('details.saved-input');
      assert.equal(await details.locator('summary').textContent(), 'Earlier revisions');
      assert.equal(await details.evaluate(node => node.open), expanded);
    }
  }
}

export async function materialBrowserJourney({origin, artifact, app, conceptId, readRows, readBuckets,
  output = 'target/materials-browser'}) {
  const url = new URL(origin);
  assert.ok(['127.0.0.1', 'localhost'].includes(url.hostname), 'Material browser proof must use an isolated loopback Worker.');
  assert.equal(app.concepts[conceptId].origin, 'owner-authored');
  assert.equal(app.concepts[conceptId].exposure_ms, null);
  const baselineRows = await readRows(), baselineBuckets = await readBuckets();
  const out = resolve(output), marker = join(out, '.scry-materials-browser-owned');
  if (existsSync(out)) {
    assert.equal(await readFile(marker, 'utf8'), markerText, 'Refuse to overwrite non-owned browser output.');
    await rm(out, {recursive: true});
  }
  await mkdir(join(out, 'screens'), {recursive: true});
  await writeFile(marker, markerText);
  const report = {
    format: 'scry-materials-browser-v1', status: 'running', variants: [],
    worker_sha256: sha256(await readFile(join(artifact, 'index.js'))),
    wasm_sha256: sha256(await readFile(join(artifact, 'index_bg.wasm'))),
    initial_rows_sha256: sha256(JSON.stringify(baselineRows)),
    limits: ['Typed synthetic restored fixture on local Chromium/workerd/SQLite/R2; no provider, production or physical-phone acceptance.',
      'Browser DOM follows HTML CRLF-to-LF parsing; raw response checks separately retain exact persisted UTF-8 bytes.',
      '200% means doubled computed font sizes for every element, not CSS zoom or device scaling.'],
  };
  const require = createRequire(import.meta.url);
  let browser;
  try {
    const playwright = require(process.env.SCRY_CRITICS_PLAYWRIGHT_PATH || 'playwright');
    browser = await playwright.chromium.launch({headless: true, args: ['--no-sandbox'],
      ...(process.env.SCRY_CRITICS_CHROMIUM_PATH ? {executablePath: process.env.SCRY_CRITICS_CHROMIUM_PATH} : {})});
    for (const width of [390, 1280]) for (const scheme of ['light', 'dark']) for (const doubled of [false, true]) {
      const name = `${width}-${scheme}-${doubled ? 200 : 100}`;
      // The 200% cases also prove that disclosure/navigation work with no app JS.
      const context = await browser.newContext({viewport: {width, height: 900}, colorScheme: scheme,
        javaScriptEnabled: !doubled, ...(width < 768 ? {isMobile: true, hasTouch: true} : {})});
      const page = await context.newPage(); page.setDefaultTimeout(15000);
      const errors = [], external = [], posts = [];
      page.on('pageerror', error => errors.push(error.message));
      context.on('request', request => {if (request.method() !== 'GET') posts.push(request.method());});
      await context.route('**/*', route => {
        if (new URL(route.request().url()).origin === url.origin) return route.continue();
        external.push(new URL(route.request().url()).origin); return route.abort();
      });
      const row = {width, scheme, text_size: doubled ? '200%' : '100%', javascript: !doubled,
        status: 'running', actions: [], screens: []};
      report.variants.push(row);
      const stable = async action => {
        assert.deepEqual(await readRows(), baselineRows, `${action}: authored reading changes no exposure, history or SQLite row.`);
        assert.deepEqual(await readBuckets(), baselineBuckets, `${action}: authored reading changes no R2 object.`);
        row.actions.push(action);
      };
      const capture = async label => {
        const path = `screens/${name}-${label}.png`;
        await page.screenshot({path: join(out, path), fullPage: true});
        row.screens.push({path, sha256: sha256(await readFile(join(out, path)))});
      };
      const activate = locator => width < 768 ? locator.tap() : locator.click();
      const home = () => page.locator('[data-state="concept-home"]');
      const summaries = () => page.locator('article[data-material-id] details.saved-input > summary');
      try {
        const response = await page.goto(url.origin + '/');
        assert.equal(response.status(), 200); await home().waitFor();
        const card = home().locator(`a[href="/concepts/${conceptId}"]`);
        assert.ok((await card.textContent()).includes(app.concepts[conceptId].name));
        assert.equal(await card.locator('.idea-status').textContent(), 'Study material');
        await textLayout(page, doubled); await stable('Home'); await capture('home');
        await activate(card); await page.waitForURL(url => url.pathname === `/concepts/${conceptId}`);
        await exactMaterials(page, app, conceptId, false);
        await textLayout(page, doubled); await stable('Home to authored detail'); await capture('current');
        assert.equal(await summaries().count(), 1, 'The typed two-Material/three-revision fixture has one native history disclosure.');
        await activate(summaries());
        await exactMaterials(page, app, conceptId, true);
        await textLayout(page, doubled); await stable('Pointer/touch opens native Earlier revisions'); await capture('earlier');
        await summaries().focus(); await page.keyboard.press('Enter');
        await exactMaterials(page, app, conceptId, false); await stable('Keyboard closes native Earlier revisions');
        await page.keyboard.press('Enter');
        await exactMaterials(page, app, conceptId, true); await stable('Keyboard reopens native Earlier revisions');
        await page.reload();
        await exactMaterials(page, app, conceptId, false);
        await textLayout(page, doubled); await stable('Reload retains exact current and earlier wording'); await capture('reload');
        await activate(page.locator('article[data-state="concept"] > a.back-link'));
        await page.waitForURL(url => url.pathname === '/'); await home().waitFor();
        await textLayout(page, doubled); await stable('Native Your concepts navigation');
        await page.goBack(); await exactMaterials(page, app, conceptId, false);
        await textLayout(page, doubled); await stable('Browser Back to authored detail');
        await page.goForward(); await home().waitFor(); await stable('Browser Forward to Home');
        assert.deepEqual(errors, [], 'No browser runtime errors.');
        assert.deepEqual(external, [], 'No external browser request.');
        assert.deepEqual(posts, [], 'Reading and native disclosure issue no mutations.');
        row.status = 'pass';
      } catch (error) {
        row.status = 'fail'; row.reason = String(error.message).slice(0, 1500);
        await capture('failure').catch(() => {});
      } finally {await context.close();}
    }
    assert.ok(report.variants.every(row => row.status === 'pass'),
      report.variants.filter(row => row.status !== 'pass').map(row => `${row.width}/${row.scheme}/${row.text_size}: ${row.reason}`).join('\n'));
    report.status = 'pass';
  } catch (error) {
    report.status = 'fail'; report.reason = String(error.message).slice(0, 3000); throw error;
  } finally {
    await browser?.close();
    await writeFile(join(out, 'report.json'), JSON.stringify(report, null, 2) + '\n');
  }
  return {path: `${output}/report.json`, variants: report.variants.length};
}
