import assert from 'node:assert/strict';
import {starterFixture} from './nato-starter.mjs';

// Actual browser editing over isolated synthetic workerd; no live credentials.
export async function autosaveBrowserWalk(browser,capture) {
  const f=await starterFixture(),context=await browser.newContext({viewport:{width:390,height:844}});
  const result={status:'pass',feature:'foreground-autosave',evidence:[]};
  try {
    const page=await context.newPage();page.setDefaultTimeout(15000);
    await page.goto(f.origin+'/?practice=nato');await page.getByRole('button',{name:'Start practice'}).click();
    const answer=page.locator('#answer');await answer.waitFor();
    let release,observed;const waiting=new Promise(r=>release=r),started=new Promise(r=>observed=r);let saves=0;
    await page.route('**/practice/draft',async route=>{saves++;if(saves===1){observed();await waiting;}await route.continue();});
    await answer.fill('Al');await answer.focus();await answer.evaluate(el=>{window.__syntheticAnswer=el;el.setSelectionRange(1,1);});
    await started;await answer.press('End');await answer.press('x');const selection=await answer.evaluate(el=>[el.selectionStart,el.selectionEnd]);release();
    await page.getByText('Earlier text saved. Newer changes are not yet saved.',{exact:true}).waitFor();
    assert.equal(await answer.inputValue(),'Alx');assert.deepEqual(await answer.evaluate(el=>[el.selectionStart,el.selectionEnd]),selection);assert.equal(await answer.evaluate(el=>el===window.__syntheticAnswer),true);
    await page.getByText('Saved.',{exact:true}).waitFor();assert.equal(saves,2);assert.equal(await page.getByRole('button',{name:'Discard saved draft',exact:true}).isVisible(),true);
    result.evidence.push(await capture(page,'autosave-delayed-ack.png'));
    // CDP composition sends real Blink composition events; it is not a physical IME.
    const cdp=await context.newCDPSession(page);await answer.focus();await cdp.send('Input.imeSetComposition',{text:'z',selectionStart:1,selectionEnd:1});
    await page.waitForTimeout(5500);assert.equal(saves,2,'No save during composition.');
    const imeSaved=page.waitForResponse(response=>response.url().endsWith('/practice/draft')&&response.status()===200);
    await cdp.send('Input.insertText',{text:'z'});await imeSaved;await page.getByText('Saved.',{exact:true}).waitFor();assert.equal(saves,3);assert.equal(await answer.evaluate(el=>el===window.__syntheticAnswer),true);
    result.evidence.push(await capture(page,'autosave-composition.png'));await cdp.detach();
    const acknowledged=await answer.inputValue();await answer.fill(acknowledged+'q');await answer.fill(acknowledged);await page.waitForTimeout(5500);assert.equal(saves,3,'Undo to the acknowledged text creates no receipt.');
    await page.reload();await answer.waitFor();await page.waitForTimeout(5500);assert.equal(saves,3,'Reload does not save.');assert.equal(await answer.inputValue(),acknowledged);
    await answer.fill(acknowledged+'q');await answer.fill(acknowledged);await page.waitForTimeout(5500);assert.equal(saves,3,'Undo to the reloaded checkpoint creates no receipt.');
    const delayedContext=await browser.newContext({viewport:{width:390,height:844}});
    let releaseScript=()=>{};
    try {
      const delayedPage=await delayedContext.newPage();delayedPage.setDefaultTimeout(15000);
      let scriptSeen;const scriptStarted=new Promise(resolve=>scriptSeen=resolve),scriptGate=new Promise(resolve=>releaseScript=resolve);let delayedSaves=0;
      await delayedPage.route('**/assets/app.js',async route=>{scriptSeen();await scriptGate;await route.continue();});
      await delayedPage.route('**/practice/draft',async route=>{delayedSaves++;await route.continue();});
      await delayedPage.goto(f.origin+'/?practice=nato',{waitUntil:'commit'});await scriptStarted;
      const earlyAnswer=delayedPage.locator('#answer');await earlyAnswer.waitFor();await earlyAnswer.fill('typed before script');releaseScript();await delayedPage.waitForLoadState('load');
      await delayedPage.getByText('Changes are not yet saved. Use Save draft or keep typing.',{exact:true}).waitFor();await delayedPage.waitForTimeout(5500);assert.equal(delayedSaves,0,'Deferred-script startup does not save the pre-script text.');
      const earlySave=delayedPage.waitForResponse(response=>response.url().endsWith('/practice/draft')&&response.status()===200);
      await earlyAnswer.fill('later text');await earlyAnswer.fill('typed before script');await earlySave;await delayedPage.getByText('Saved.',{exact:true}).waitFor();assert.equal(delayedSaves,1,'The pre-script edit is not treated as the server checkpoint.');
      result.evidence.push(await capture(delayedPage,'autosave-deferred-script.png'));
    } finally {releaseScript();await delayedContext.close();}
    const rows=await f.rows();assert.equal(Object.keys(rows).filter(k=>k.startsWith('practice_attempts/')).length,0);
    return [result];
  } finally {await context.close();await f.close();}
}
