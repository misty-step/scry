// Saved understanding recovery must use the same home as its native forms.
import assert from 'node:assert/strict';
import {isDeepStrictEqual} from 'node:util';
import {mixedFixture} from './mixed-notebook.mjs';

export async function legacyRecoveryLinksWalk(browser, capture) {
  const results = [];
  for (const noJS of [false, true]) {
    // The existing QA clock keeps the unpaused check observable without sending
    // work: its one-second alarm is one day ahead of the real test runner.
    const nowMs = Date.now() + 86_400_000;
    const f = await mixedFixture({nowMs});
    const context = await browser.newContext({viewport:{width:390, height:844}, hasTouch:true, isMobile:true, colorScheme:'dark', javaScriptEnabled:!noJS});
    const page = await context.newPage();
    page.setDefaultTimeout(12000);
    const row = {noJS, nowMs, status:'pass', evidence:[]};
    results.push(row);
    const shot = async name => row.evidence.push(await capture(page, `legacy-home-${noJS?'native':'js'}-${name}.png`));
    const unchanged = async () => assert.ok(isDeepStrictEqual((await f.record('meta')).practice, f.practice), 'Exact practice remains identical');
    const follow = async link => {
      const navigation = page.waitForNavigation({waitUntil:'domcontentloaded'});
      await link.tap();
      await navigation;
    };
    try {
      await page.goto(f.origin + '/notebook');
      await page.locator('details.overflow>summary').tap();
      await follow(page.getByRole('link', {name:'Look it up in my notes', exact:true}));
      await page.locator('[data-state="gate"]').waitFor();
      const keep = page.getByRole('link', {name:'Keep trying', exact:true});
      row.keep_trying_href = await keep.getAttribute('href');
      assert.equal(row.keep_trying_href, '/notebook');
      await shot('gate');
      await follow(keep);
      await page.locator('[data-state="question"]').waitFor();
      assert.equal(new URL(page.url()).pathname, '/notebook');
      await unchanged();
      await shot('kept-question');
      await page.goto(f.origin + '/');
      await page.locator('[data-state="exact-prompt"]').waitFor();
      await unchanged();
      await page.goto(f.origin + '/notebook');
      await page.getByRole('button', {name:'The origin content definitely changed', exact:true}).tap();
      await page.locator('[data-state="result"]').waitFor();
      await page.getByRole('button', {name:'Next question', exact:false}).tap();
      await page.locator('#answer').fill('My saved uncertain freshness answer');
      await page.getByRole('button', {name:'Check my answer', exact:true}).tap();
      await page.locator('[data-state="checking"]').waitFor();
      const again = page.getByRole('link', {name:'Check again', exact:true});
      row.check_again_href = await again.getAttribute('href');
      assert.equal(row.check_again_href, '/notebook');
      await shot('checking-before');
      await follow(again);
      await page.locator('[data-state="checking"]').waitFor();
      assert.equal(new URL(page.url()).pathname, '/notebook');
      assert.ok((await page.locator('main').innerText()).includes('My saved uncertain freshness answer'));
      await unchanged();
      await shot('checking-after');
      await page.goto(f.origin + '/');
      await page.locator('[data-state="exact-prompt"]').waitFor();
      await unchanged();
      await shot('exact-unchanged');
      row.observed = {keep_trying:'legacy question', check_again:'saved checking answer', exact_practice_unchanged:true};
    } catch (error) {
      row.status = 'fail'; row.reason = error.message;
      await shot('failure');
    } finally {await context.close(); await f.close();}
  }
  return results;
}
