// Measure the real held-result arrival before any scroll, reload, or locator
// screenshot. This helper observes layout; it does not reposition the product.
import assert from 'node:assert/strict';

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
