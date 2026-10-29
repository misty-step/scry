import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createSyntheticRuntime} from './runtime-helper.mjs';

export async function releaseWalk(browser, capture) {
  const runtime = await createSyntheticRuntime({name:'release-notes-walk'});
  const version = /version = "([^"]+)"/.exec(readFileSync('Cargo.toml','utf8'))[1];
  const rows=[];
  try {
    for (const [width, scheme] of [[320,'light'],[390,'dark'],[1280,'light'],[1280,'dark']]) {
      const context = await browser.newContext({viewport:{width,height:900},colorScheme:scheme});
      const page = await context.newPage();
      const errors=[];
      page.on('pageerror', e=>errors.push(e.message));
      try {
        await page.goto(runtime.url);
        // NATO Home deliberately hides the generic footer. Reach retained release
        // notes through the visible account menu and Settings, not a hidden click.
        await page.locator('.account-menu>summary').click();
        await page.locator('.account-menu').getByRole('link',{name:'Settings',exact:true}).click();
        assert.equal(new URL(page.url()).pathname,'/settings');
        const changelog=page.locator('footer').getByRole('link',{name:'Changelog',exact:true});
        assert.equal(await changelog.isVisible(),true);
        await capture(page,`release-${width}-${scheme}-entry.png`);
        await changelog.click();
        await page.getByRole('heading',{name:'Changelog',level:1,exact:true}).waitFor();
        assert.equal(new URL(page.url()).pathname,'/changelog');
        const main=await page.locator('main').innerText();
        assert.ok(main.includes('Version '+version));
        assert.ok(main.includes(process.env.SCRY_SOURCE_REVISION || 'unverified-development-build'));
        assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
        await capture(page,`release-${width}-${scheme}.png`);
        assert.deepEqual(errors,[]);
        rows.push({width,scheme,status:'pass',version});
      } finally {await context.close();}
    }
  } finally {await runtime.mf.dispose();}
  return rows;
}
