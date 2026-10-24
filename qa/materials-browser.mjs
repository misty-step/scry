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
  // innerWidth can expand with mobile content. Configured-width fit is measured
  // separately below, never inferred from scrollWidth <= innerWidth.
  for (const text of await page.locator('[data-material-revision-id] .reading').all()) {
    if (!(await text.isVisible())) continue;
    assert.equal(await text.evaluate(node => getComputedStyle(node).whiteSpace), 'pre-wrap');
    assert.equal(await text.evaluate(node => node.scrollWidth > node.clientWidth + 1), false, 'Material wording must not be horizontally clipped.');
  }
}

async function geometry(page) {
  const configured = page.viewportSize();
  return {configured_viewport: configured, ...await page.evaluate(() => {
    const dimensions = node => ({scrollWidth: node.scrollWidth, clientWidth: node.clientWidth, offsetWidth: node.offsetWidth});
    const vv = visualViewport;
    const rect = node => {
      const r = node.getBoundingClientRect(), style = getComputedStyle(node);
      const range = document.createRange(); range.selectNodeContents(node);
      const text = range.getBoundingClientRect();
      return {tag: node.tagName, class: node.getAttribute('class'), href: node.getAttribute('href'),
        material_id: node.getAttribute('data-material-id'), revision_id: node.getAttribute('data-material-revision-id'),
        text: (node.textContent || '').slice(0, 100), ...dimensions(node),
        rect: {x:r.x,y:r.y,left:r.left,right:r.right,top:r.top,bottom:r.bottom,width:r.width,height:r.height},
        text_bounds: {left:text.left,right:text.right,top:text.top,bottom:text.bottom,width:text.width,height:text.height},
        position: style.position, font_size: style.fontSize, white_space: style.whiteSpace, overflow_wrap: style.overflowWrap,
        focused: node === document.activeElement,
        intersects_visual_viewport: r.width > 0 && r.height > 0 && r.right > vv.offsetLeft && r.left < vv.offsetLeft + vv.width
          && r.bottom > vv.offsetTop && r.top < vv.offsetTop + vv.height};
    };
    return {innerWidth, innerHeight, scrollX, scrollY, devicePixelRatio,
      document: dimensions(document.documentElement), body: dimensions(document.body),
      visualViewport: {width:vv.width,height:vv.height,scale:vv.scale,offsetLeft:vv.offsetLeft,offsetTop:vv.offsetTop,pageLeft:vv.pageLeft,pageTop:vv.pageTop},
      focused_element: document.activeElement ? {tag:document.activeElement.tagName,class:document.activeElement.getAttribute('class'),
        text:(document.activeElement.textContent || '').slice(0,100)} : null,
      elements: [...document.querySelectorAll('.skip-link,.masthead,.masthead nav,.home-concepts a,.idea-status,article[data-state="concept"],article[data-state="concept"] h1,[data-material-id],[data-material-revision-id],[data-material-revision-id] .reading,details.saved-input>summary')].map(rect)};
  })};
}

function configuredOverflow(measurement) {
  return Math.max(0, measurement.document.scrollWidth, measurement.body.scrollWidth) - measurement.configured_viewport.width;
}

function assertMaterialViewport(measurement) {
  assert.ok(configuredOverflow(measurement) <= 1, 'New Material detail must fit the configured viewport, not a content-expanded innerWidth.');
  for (const element of measurement.elements.filter(element => element.material_id || element.revision_id)) {
    if (!element.rect.width || !element.rect.height) continue; // Closed native details have no rendered rectangle.
    assert.ok(element.rect.left >= -1 && element.rect.right <= measurement.configured_viewport.width + 1,
      'Each rendered current/earlier Material rectangle fits the configured viewport.');
  }
  const skip = measurement.elements.find(element => element.class === 'skip-link');
  assert.ok(skip && !skip.focused && !skip.intersects_visual_viewport,
    'An unfocused skip link must not obstruct the actual Material viewport.');
}

async function homeMarkup(page) {
  return Object.fromEntries(await page.locator('.home-concepts a').evaluateAll(nodes => nodes.map(node => [node.getAttribute('href'), node.outerHTML])));
}

async function servedCss(context, origin) {
  const response = await context.request.get(origin + '/assets/app.css');
  assert.equal(response.status(), 200); return sha256(await response.body());
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

export async function materialBrowserJourney({origin, artifact, app, conceptId, readRows, readBuckets, predecessor,
  output = 'target/materials-browser'}) {
  const url = new URL(origin);
  assert.ok(['127.0.0.1', 'localhost'].includes(url.hostname), 'Material browser proof must use an isolated loopback Worker.');
  assert.equal(app.concepts[conceptId].origin, 'owner-authored');
  assert.equal(app.concepts[conceptId].exposure_ms, null);
  const baselineRows = await readRows(), baselineBuckets = await readBuckets();
  assert.ok(predecessor && ['127.0.0.1', 'localhost'].includes(new URL(predecessor.origin).hostname));
  const predecessorRows = await predecessor.readRows(), predecessorBuckets = await predecessor.readBuckets();
  const out = resolve(output), marker = join(out, '.scry-materials-browser-owned');
  if (existsSync(out)) {
    assert.equal(await readFile(marker, 'utf8'), markerText, 'Refuse to overwrite non-owned browser output.');
    await rm(out, {recursive: true});
  }
  await mkdir(join(out, 'screens'), {recursive: true});
  await writeFile(marker, markerText);
  const report = {
    format: 'scry-materials-browser-v2', status: 'running', variants: [],
    status_scope: 'Functional read/disclosure/storage assertions; measured layout status is reported separately.',
    worker_sha256: sha256(await readFile(join(artifact, 'index.js'))),
    wasm_sha256: sha256(await readFile(join(artifact, 'index_bg.wasm'))),
    initial_rows_sha256: sha256(JSON.stringify(baselineRows)),
    predecessor: {release: predecessor.release, worker_sha256: sha256(await readFile(join(predecessor.artifact, 'index.js'))),
      wasm_sha256: sha256(await readFile(join(predecessor.artifact, 'index_bg.wasm')))},
    limits: ['Typed synthetic restored fixture on local Chromium/workerd/SQLite/R2; no provider, production or physical-phone acceptance.',
      'Browser DOM follows HTML CRLF-to-LF parsing; raw response checks separately retain exact persisted UTF-8 bytes.',
      '200% means doubled computed font sizes for every element, not CSS zoom or device scaling.',
      'Full-page screenshots can include fixed content outside the currently visible viewport; paired viewport images and rectangles, not full-page pixels alone, establish obstruction.',
      'Inherited generated-Home overflow is disclosed with exact predecessor HTML/CSS/geometry comparison, not relabeled as a responsive pass.'],
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
      const options = {viewport: {width, height: 900}, colorScheme: scheme,
        javaScriptEnabled: !doubled, ...(width < 768 ? {isMobile: true, hasTouch: true} : {})};
      const context = await browser.newContext(options);
      const page = await context.newPage(); page.setDefaultTimeout(15000);
      const errors = [], external = [], posts = [];
      page.on('pageerror', error => errors.push(error.message));
      context.on('request', request => {if (request.method() !== 'GET') posts.push(request.method());});
      await context.route('**/*', route => {
        if (new URL(route.request().url()).origin === url.origin) return route.continue();
        external.push(new URL(route.request().url()).origin); return route.abort();
      });
      const row = {width, scheme, text_size: doubled ? '200%' : '100%', javascript: !doubled,
        status: 'running', layout_status: 'pending', actions: [], screens: [], measurements: []};
      report.variants.push(row);
      const stable = async action => {
        assert.deepEqual(await readRows(), baselineRows, `${action}: authored reading changes no exposure, history or SQLite row.`);
        assert.deepEqual(await readBuckets(), baselineBuckets, `${action}: authored reading changes no R2 object.`);
        row.actions.push(action);
      };
      const capture = async (label, target = page, kind = 'material') => {
        const shot = async (suffix, fullPage) => {
          const path = `screens/${name}-${label}-${suffix}.png`;
          const bytes = await target.screenshot({path: join(out, path), fullPage});
          row.screens.push({path, sha256: sha256(bytes), pixel_width: bytes.readUInt32BE(16), pixel_height: bytes.readUInt32BE(20)});
        };
        const interaction = await geometry(target);
        row.measurements.push({stage: label, capture_position: 'interaction', ...interaction});
        if (kind === 'material') assertMaterialViewport(interaction);
        await shot('interaction-viewport', false);
        await shot('full', true);
        await target.evaluate(() => {window.scrollTo(0, 0);});
        // No page-script callback is required: no-JS contexts may never invoke
        // requestAnimationFrame. Poll the actual scroll state from Node instead.
        let atTop = false;
        for (let attempt = 0; attempt < 40; attempt++) {
          atTop = await target.evaluate(() => scrollX === 0 && scrollY === 0);
          if (atTop) break;
          await new Promise(resolve => setTimeout(resolve, 50));
        }
        assert.ok(atTop, 'Viewport capture must reach scroll top within two seconds.');
        const top = await geometry(target);
        row.measurements.push({stage: label, capture_position: 'scroll-top', ...top});
        if (kind === 'material') assertMaterialViewport(top);
        await shot('top-viewport', false);
        await target.evaluate(({x,y}) => window.scrollTo(x,y), {x:interaction.scrollX,y:interaction.scrollY});
        return interaction;
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
        const currentMarkup = await homeMarkup(page), currentCss = await servedCss(context, url.origin);
        await textLayout(page, doubled); await stable('Home');
        const homeGeometry = await capture('home', page, 'home');
        const authoredCard = homeGeometry.elements.find(element => element.href === `/concepts/${conceptId}`);
        assert.ok(authoredCard);
        row.authored_home_text_overflow_px = Math.max(0, authoredCard.text_bounds.right - width, -authoredCard.text_bounds.left);
        assert.ok(row.authored_home_text_overflow_px <= 1,
          'The new authored Home card text must fit even if an existing generated status already makes the page wider.');
        if (width === 390 && doubled) {
          const previousContext = await browser.newContext(options);
          try {
            await previousContext.route('**/*', route => {
              if (new URL(route.request().url()).origin === new URL(predecessor.origin).origin) return route.continue();
              external.push(new URL(route.request().url()).origin); return route.abort();
            });
            const previousPage = await previousContext.newPage(); previousPage.setDefaultTimeout(15000);
            const response = await previousPage.goto(predecessor.origin + '/');
            assert.equal(response.status(), 200); await previousPage.locator('[data-state="concept-home"]').waitFor();
            const previousMarkup = await homeMarkup(previousPage), previousCss = await servedCss(previousContext, predecessor.origin);
            const generatedMarkup = {...currentMarkup}; delete generatedMarkup[`/concepts/${conceptId}`];
            assert.deepEqual(generatedMarkup, previousMarkup, 'Existing generated-Home rows are byte-identical to the predecessor.');
            assert.equal(currentCss, previousCss, 'The actual served CSS is unchanged from the predecessor.');
            await textLayout(previousPage, true);
            const previousGeometry = await capture('predecessor-home', previousPage, 'home');
            const statusGeometry = measurement => measurement.elements.filter(element => element.class === 'idea-status' && element.text !== 'Study material')
              .map(element => ({text:element.text,left:element.rect.left,right:element.rect.right,width:element.rect.width,
                text_left:element.text_bounds.left,text_right:element.text_bounds.right,text_width:element.text_bounds.width,
                font_size:element.font_size,white_space:element.white_space,overflow_wrap:element.overflow_wrap}));
            assert.deepEqual(statusGeometry(homeGeometry), statusGeometry(previousGeometry), 'Existing generated status horizontal geometry is unchanged.');
            assert.ok(Math.abs(configuredOverflow(homeGeometry) - configuredOverflow(previousGeometry)) <= 1,
              'Any Home overflow must match the unchanged predecessor, not silently accept a new regression.');
            row.home_comparison = {release:predecessor.release,served_css_sha256:currentCss,
              generated_rows_sha256:sha256(JSON.stringify(generatedMarkup)),generated_html_equal:true,generated_status_geometry_equal:true,
              current_overflow_px:configuredOverflow(homeGeometry),predecessor_overflow_px:configuredOverflow(previousGeometry)};
            row.layout_status = configuredOverflow(homeGeometry) > 1 ? 'inherited-generated-home-overflow; Material detail checked separately' : 'within-configured-width';
            assert.deepEqual(await predecessor.readRows(), predecessorRows, 'Predecessor Home comparison changes no historical row.');
            assert.deepEqual(await predecessor.readBuckets(), predecessorBuckets, 'Predecessor Home comparison changes no R2 object.');
          } finally {await previousContext.close();}
        } else {
          assert.ok(configuredOverflow(homeGeometry) <= 1, 'Home fits the configured viewport outside the measured inherited case.');
          row.layout_status = 'within-configured-width';
        }
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
        await textLayout(page, doubled); assertMaterialViewport(await geometry(page)); await stable('Browser Back to authored detail');
        await page.goForward(); await home().waitFor(); await stable('Browser Forward to Home');
        assert.deepEqual(errors, [], 'No browser runtime errors.');
        assert.deepEqual(external, [], 'No external browser request.');
        assert.deepEqual(posts, [], 'Reading and native disclosure issue no mutations.');
        row.status = 'pass';
      } catch (error) {
        row.status = 'fail'; row.reason = String(error.message).slice(0, 1500);
        await capture('failure', page, 'failure').catch(() => {});
      } finally {await context.close();}
    }
    assert.ok(report.variants.every(row => row.status === 'pass'),
      report.variants.filter(row => row.status !== 'pass').map(row => `${row.width}/${row.scheme}/${row.text_size}: ${row.reason}`).join('\n'));
    report.layout_status = report.variants.some(row => row.layout_status.startsWith('inherited-'))
      ? 'Inherited predecessor generated-Home overflow retained and disclosed; new Material detail fits the configured viewport.'
      : 'All measured pages fit the configured viewport.';
    report.status = 'pass';
  } catch (error) {
    report.status = 'fail'; report.reason = String(error.message).slice(0, 3000); throw error;
  } finally {
    await browser?.close();
    await writeFile(join(out, 'report.json'), JSON.stringify(report, null, 2) + '\n');
  }
  return {path: `${output}/report.json`, variants: report.variants.length, layout_status: report.layout_status};
}
