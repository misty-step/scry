// Measure the real held-result arrival before any scroll, reload, or locator
// screenshot. This helper observes layout; it does not reposition the product.
import assert from 'node:assert/strict';

export const feedbackVariants = [[320,'light',false,true],[320,'dark',true,true],[390,'light',false,false],[390,'dark',false,false],[1280,'light',false,false],[1280,'dark',true,false],[320,'light',false,false],[320,'dark',false,false],[390,'light',false,true],[390,'dark',true,true]];

export async function feedbackArrival(page, capture, name, zoom) {
  if (zoom) await page.evaluate(() => {
    for (const node of document.querySelectorAll('[data-qa-text-size]')) {
      node.style.removeProperty('font-size'); delete node.dataset.qaTextSize;
    }
    const nodes = [...document.querySelectorAll('body,body *')];
    for (const node of nodes) node.dataset.qaTextSize = String(parseFloat(getComputedStyle(node).fontSize) * 2);
    for (const node of nodes) node.style.fontSize = node.dataset.qaTextSize + 'px';
  });
  await page.evaluate(() => document.fonts.ready);
  const geometry = await page.evaluate(() => {
    const observe = node => {
      if (!node) return null;
      const rect = node.getBoundingClientRect();
      const points = [[.1,.1],[.5,.5],[.9,.9]].map(([x,y]) => [rect.left + rect.width*x, rect.top + rect.height*y]);
      const visible = rect.width > 0 && rect.height > 0 && rect.top >= 0 && rect.left >= 0 && rect.bottom <= innerHeight && rect.right <= innerWidth;
      return {text:node.textContent,rect:{top:rect.top,bottom:rect.bottom,left:rect.left,right:rect.right,width:rect.width,height:rect.height},visible,unoccluded:visible && points.every(([x,y]) => node.contains(document.elementFromPoint(x,y)))};
    };
    const changed = document.querySelector('[data-feedback-kind], [data-diff-kind]:not([data-diff-kind="correct"])');
    return {viewport:{width:innerWidth,height:innerHeight},scroll:{x:scrollX,y:scrollY},overflow:document.documentElement.scrollWidth>innerWidth,verdict:observe(document.querySelector('[data-verdict]')),change:observe(changed),replacement:observe(changed?.querySelector('.diff-text') || changed?.lastElementChild),actions:observe(document.querySelector('.exact-study .result-actions'))};
  });
  const evidence = await capture(page, name, {fullPage:false});
  return {geometry,evidence};
}

export function assertFeedbackArrival(arrival) {
  assert.equal(arrival.geometry.overflow,false,'Arrival has no horizontal overflow.');
  for (const name of ['verdict','change','replacement']) {
    assert.ok(arrival.geometry[name]?.unoccluded,`Arrival ${name} is fully visible and unoccluded before scrolling: ${JSON.stringify(arrival.geometry)}`);
  }
}

// Native disclosure is presentation only, including when the controller is off.
export async function fullAnswer(page, f, attempt, expected) {
  const before = await f.rows();
  const words=attempt.grade.diff.map(d => ({kind:d.kind,unit_index:d.unit_index,text:d.kind==='correct'||d.kind==='missing'?d.expected.text:d.kind==='extra'?d.typed.text:d.typed.text+' → '+d.expected.text}));
  assert.deepEqual(await page.locator('[data-feedback-kind]').evaluateAll(nodes=>nodes.map(node=>({kind:node.dataset.feedbackKind,unit_index:Number(node.dataset.unitIndex),text:node.querySelector('.diff-text').textContent}))),words.filter(word=>word.kind!=='correct'));
  const details = page.locator('details.exact-full-answer');
  assert.equal(await details.getAttribute('open'),null);
  const summary = details.locator('summary');
  await summary.focus(); await page.keyboard.press('Space');
  assert.notEqual(await details.getAttribute('open'),null);
  assert.equal(await summary.evaluate(node => node === document.activeElement),true);
  assert.equal(await details.locator('.answer-pair p').textContent(),attempt.raw_answer);
  assert.equal((await details.locator('.exact-expected').allTextContents()).join(''),expected);
  assert.ok((await details.locator('.exact-authority').textContent()).includes(attempt.grade.authority.name));
  assert.ok((await details.locator('.exact-authority').textContent()).includes(attempt.grade.authority.grader_version));
  assert.deepEqual(await details.locator('[data-diff-kind]').evaluateAll(nodes => nodes.map(node => ({kind:node.dataset.diffKind,unit_index:Number(node.dataset.unitIndex),text:node.querySelector('.diff-text').textContent}))),words);
  assert.equal(await details.locator('[data-diff-kind="correct"] .diff-label.sr-only').count(),attempt.grade.diff.filter(d=>d.kind==='correct').length);
  if(words.some(word=>word.kind==='correct')) assert.ok((await details.ariaSnapshot()).includes('Correct'),'Matching words retain an accessible classification.');
  assert.deepEqual(await f.rows(),before,'Opening Full answer writes no event, correction or schedule.');
  return async () => {
    await summary.focus(); await page.keyboard.press('Enter');
    assert.equal(await details.getAttribute('open'),null);
    await page.keyboard.press('Tab');
    assert.equal(await page.getByRole('button',{name:'Next',exact:true}).evaluate(node=>node===document.activeElement),true,'Next follows the closed disclosure in keyboard order.');
    assert.deepEqual(await f.rows(),before,'Closing Full answer and focusing Next do not advance.');
  };
}
