import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';

const script = await readFile(new URL('../assets/app.js', import.meta.url), 'utf8');
const scope = 'a'.repeat(64);
const deferred = () => { let resolve, reject; const promise = new Promise((r, j) => { resolve = r; reject = j; }); return { promise, resolve, reject }; };
const tick = () => new Promise(resolve => setImmediate(resolve));

function fixture({ hidden = false, prerendering = false, polling = false, pollState = 'checking', immediateTimeout = false, nativeTransport = false, fastAuto = false } = {}) {
  const listeners = {};
  const calls = [];
  const timerDelays = [];
  const classes = new Set();
  const status = { hidden: true, children: [], replaceChildren() { this.children = []; }, append(n) { this.children.push(n); } };
  const frame = { inert: false, erased: false, replaceChildren() { this.erased = true; } };
  const previousMain = { replaced: false, querySelector() { return null; }, querySelectorAll() { return []; }, replaceWith() { this.replaced = true; document.answerFields = []; } };
  const nodes = new Map();
  const element = () => ({ children: [], className: '', textContent: '', addEventListener(type, f) { this[type] = f; }, append(...items) { this.children.push(...items); }, setAttribute() {}, focus() { this.focused = true; }, remove() { nodes.delete(this.id); } });
  class Form { constructor() { this.method = 'post'; this.action = 'https://scry.test/practice/answer'; this.isConnected = true; this.dataset = {}; this.classList = { contains: () => false }; } querySelector() { return null; } querySelectorAll() { return []; } getAttribute(name) { return name === 'action' ? this.action : null; } setAttribute() {} removeAttribute() {} }
  const document = {
    hidden, prerendering, title: 'Private question · Scry', pollable: polling, activeElement: null, forms: [], answerFields: [], documentElement: { dataset: { notebookScope: scope } },
    body: { classList: { add: c => classes.add(c), remove: c => classes.delete(c) }, append(n) { nodes.set(n.id, n); } },
    addEventListener(type, f) { (listeners[type] ||= []).push(f); },
    querySelector(selector) { if (selector === '.page-frame') return frame; if (selector === '#main[data-poll], [data-poll]' && this.pollable) return {}; if (selector === '[data-state]' && this.pollable) return { getAttribute: () => pollState }; return null; },
    querySelectorAll(selector) { return selector.startsWith('form.answer-form') ? this.answerFields : []; },
    getElementById(id) { return id === 'request-status' ? status : id === 'main' ? previousMain : nodes.get(id); },
    createElement: element,
  };
  const window = {
    confirmResult: false, confirmCount: 0, scrollTo() {},
    confirm() { this.confirmCount++; return this.confirmResult; },
    addEventListener(type, f) { (listeners[`window:${type}`] ||= []).push(f); },
    removeEventListener(type, f) { listeners[`window:${type}`] = (listeners[`window:${type}`] || []).filter(listener => listener !== f); },
  };
  const fetch = (url, init) => {
    const gate = deferred();
    if (immediateTimeout && url === '/session/scope') init.signal.addEventListener('abort', () => gate.reject(new Error('aborted')));
    calls.push({ url, init, ...gate }); return gate.promise;
  };
  const clock = (fn, delay) => { timerDelays.push(delay); return setTimeout(fn, (polling && delay === 4000) || (immediateTimeout && delay === 8000) || (fastAuto && delay === 5000) ? 0 : delay); };
  class DOMParser {
    parseFromString(html) {
      const found = /data-notebook-scope=["']([^"']*)["']/.exec(html);
      const main = { classList: { add() {} }, focus() {}, querySelector() { return null; }, querySelectorAll() { return []; } };
      return { title: 'Synthetic page', documentElement: { dataset: { notebookScope: found?.[1] } }, querySelector(selector) { if (selector === '#main') return main; return selector === '.notice, .empty-stage .lede' ? {textContent:'Synthetic rejection'} : {}; }, querySelectorAll() { return []; } };
    }
  }
  const context = { document, window, fetch, DOMParser, navigator: { onLine: true }, location: { href: 'https://scry.test/', origin: 'https://scry.test', pathname: '/', search: '', reload() {} }, HTMLFormElement: Form, HTMLElement: class {}, FormData: class { constructor(form) { this.form = form; this.values = new Map(Object.entries(form.fields || {})); if (form.answerInput) this.values.set('answer',form.answerInput.value); form.onFormData?.(); } append(k,v) {this.values.set(k,v);} set(k,v) {this.values.set(k,v);} get(k) {return this.values.get(k) ?? null;} delete(k) {this.values.delete(k);} }, crypto: {randomUUID: () => `00000000-0000-4000-8000-${String(++context.uuidCount).padStart(12,'0')}`}, uuidCount:0, AbortController, AbortSignal, TextEncoder, TextDecoder, URL, setTimeout: clock, clearTimeout, matchMedia: () => ({ matches: false }), history: { pushState() {} }, console };
  if (nativeTransport) context.fetch = undefined;
  vm.runInNewContext(script, context);
  const emit = (type, event = {}) => (listeners[type] || []).forEach(f => f(event));
  const response = (statusCode, text = scope, headers = {}) => {
    const raw = /^[a-f0-9]{64}$/.test(text) ? JSON.stringify({version: 1, scope: text}) : text;
    const bytes = new TextEncoder().encode(raw); let read = false;
    return { status: statusCode, ok: statusCode >= 200 && statusCode < 300, type: 'basic', url: 'https://scry.test/session/scope', headers: { get: key => headers[key.toLowerCase()] || (key.toLowerCase() === 'content-type' ? 'application/json' : null) }, text: async () => text, body: { getReader: () => ({ read: async () => read ? {done:true} : (read = true, {done:false,value:bytes}), releaseLock() {}, cancel: async () => {} }) } };
  };
  return { document, window, navigator: context.navigator, frame, previousMain, status, classes, calls, timerDelays, emit, response, Form };
}

test('background freezes an unknown request and never automatically sends it again', async () => {
  const f = fixture();
  const form = new f.Form();
  f.emit('submit', { target: form, preventDefault() {} });
  assert.equal(f.calls.length, 1);
  const original = f.calls[0];
  f.document.hidden = true; f.emit('visibilitychange');
  assert.equal(f.frame.inert, true); assert.ok(f.classes.has('private-suspended'));
  assert.equal(original.init.signal.aborted, true); assert.equal(f.frame.erased, false);
  f.document.hidden = false; f.emit('visibilitychange');
  assert.equal(f.calls.length, 2); assert.equal(f.calls[1].url, '/session/scope');
  assert.equal(f.calls[1].init.headers['X-Scry-Expected-Scope'], scope);
  f.calls[1].resolve(f.response(200)); await tick();
  assert.equal(f.frame.inert, false); assert.equal(f.calls.length, 2);
  const buttons = f.status.children.flatMap(n => n.children || []);
  const reconcile = buttons.find(n => n.textContent === 'Reconcile this attempt');
  assert.ok(reconcile); reconcile.click();
  assert.equal(f.calls.length, 3); assert.equal(f.calls[2].init.body, original.init.body);
  original.resolve(f.response(401)); await tick();
  assert.equal(f.frame.erased, false, 'a late denial cannot dispose a newer flight');
  f.calls[2].resolve(f.response(401)); await tick();
});

test('initially hidden document stays masked, denial disposes it', async () => {
  const f = fixture({ hidden: true });
  assert.equal(f.frame.inert, true); assert.ok(f.classes.has('private-suspended'));
  f.document.hidden = false; f.emit('visibilitychange');
  f.calls[0].resolve(f.response(409)); await tick();
  assert.equal(f.frame.erased, true); assert.ok(f.classes.has('private-hidden'));
});

test('a stale validation cannot restore a page after another background transition', async () => {
  const f = fixture();
  f.document.hidden = true; f.emit('visibilitychange');
  f.document.hidden = false; f.emit('visibilitychange');
  const first = f.calls[0];
  f.document.hidden = true; f.emit('visibilitychange');
  f.document.hidden = false; f.emit('visibilitychange');
  first.resolve(f.response(200)); await tick();
  assert.equal(f.frame.inert, true);
  f.calls[1].resolve(f.response(200)); await tick();
  assert.equal(f.frame.inert, false);
});

test('pagehide disposes the private frame and invalidates outstanding checks', async () => {
  const f = fixture();
  f.document.hidden = true; f.emit('visibilitychange');
  f.document.hidden = false; f.emit('visibilitychange');
  f.emit('window:pagehide');
  f.calls[0].resolve(f.response(200)); await tick();
  assert.equal(f.frame.erased, true); assert.ok(f.classes.has('private-hidden'));
  f.emit('window:online'); f.emit('visibilitychange'); await tick();
  assert.equal(f.calls.length, 1, 'A disposed page cannot start revalidation.');
});

test('a delayed rejection body cannot unlock a request after backgrounding', async () => {
  const f = fixture(); const form = new f.Form();
  f.emit('submit', { target: form, preventDefault() {} });
  const text = deferred();
  const response = f.response(422, '', { 'x-scry-scope': scope });
  response.text = () => text.promise;
  f.calls[0].resolve(response); await tick();
  f.document.hidden = true; f.emit('visibilitychange');
  f.document.hidden = false; f.emit('visibilitychange');
  f.calls[1].resolve(f.response(200)); await tick();
  text.resolve('late rejection'); await tick();
  assert.equal(f.frame.erased, false);
  const buttons = f.status.children.flatMap(n => n.children || []);
  assert.ok(buttons.some(n => n.textContent === 'Reconcile this attempt'));
  assert.ok(!buttons.some(n => n.textContent === 'Continue editing'));
});

for (const status of [502, 503]) {
  test(`unscoped ${status} keeps the frozen operation for manual reconciliation`, async () => {
    const f = fixture(); const form = new f.Form();
    f.emit('submit', { target: form, preventDefault() {} });
    const original = f.calls[0];
    original.resolve(f.response(status, 'Gateway unavailable')); await tick();
    assert.equal(f.frame.erased, false);
    assert.equal(f.calls.length, 2); assert.equal(f.calls[1].url, '/session/scope');
    f.calls[1].resolve(f.response(200)); await tick();
    assert.equal(f.frame.erased, false);
    const reconcile = f.status.children.flatMap(n => n.children || []).find(n => n.textContent === 'Reconcile this attempt');
    assert.ok(reconcile); assert.equal(f.calls.length, 2, 'No automatic replay.');
    reconcile.click(); assert.equal(f.calls[2].init.body, original.init.body);
    f.calls[2].resolve(f.response(401)); await tick();
  });
}

test('an explicit different response scope disposes the private page', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  f.calls[0].resolve(f.response(200, '', { 'x-scry-scope': 'b'.repeat(64) })); await tick();
  assert.equal(f.frame.erased, true); assert.equal(f.calls.length, 1);
});

test('a poll begun before submission cannot conceal the pending mutation', async () => {
  const f = fixture({ polling: true });
  await new Promise(resolve => setTimeout(resolve, 10));
  assert.equal(f.calls.length, 1); const poll = f.calls[0];
  f.document.pollable = false;
  f.emit('submit', { target: new f.Form(), preventDefault() {} });
  assert.equal(f.calls.length, 2); assert.equal(poll.init.signal.aborted, true);
  poll.resolve(f.response(401)); await tick();
  assert.equal(f.frame.erased, false, 'The obsolete poll cannot dispose a pending operation.');
  f.calls[1].resolve(f.response(503)); await tick();
  f.calls[2].resolve(f.response(200)); await tick();
  const actions = f.status.children.flatMap(n => n.children || []);
  assert.ok(actions.some(n => n.textContent === 'Reconcile this attempt'));
});

test('a stalled scope check times out and offers retry while keeping the form masked', async () => {
  const f = fixture({ hidden: true, immediateTimeout: true });
  f.document.hidden = false; f.emit('visibilitychange');
  await new Promise(resolve => setTimeout(resolve, 10));
  assert.equal(f.calls[0].init.signal.aborted, true);
  assert.equal(f.frame.inert, true); assert.equal(f.frame.erased, false);
  const panel = f.document.getElementById('scope-recovery');
  assert.ok(panel); assert.ok(panel.children.some(n => n.textContent === 'Try again' && n.focused));
});

test('an unscoped success requires a fresh scope probe and conceals a different identity', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  f.calls[0].resolve(f.response(200, 'not a scoped Scry page')); await tick();
  assert.equal(f.frame.erased, false); assert.equal(f.calls[1].url, '/session/scope');
  f.calls[1].resolve(f.response(200, 'b'.repeat(64))); await tick();
  assert.equal(f.frame.erased, true);
});

test('a malformed scope check retains the masked pending operation until retry', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  const mutation = f.calls[0];
  f.document.hidden = true; f.emit('visibilitychange');
  f.document.hidden = false; f.emit('visibilitychange');
  f.calls[1].resolve(f.response(200, '<html>temporarily unavailable</html>')); await tick();
  assert.equal(f.frame.erased, false); assert.equal(f.frame.inert, true);
  const retry = f.document.getElementById('scope-recovery').children.find(n => n.textContent === 'Try again');
  assert.ok(retry); retry.click(); f.calls[2].resolve(f.response(200)); await tick();
  assert.equal(f.frame.inert, false);
  assert.ok(f.status.children.flatMap(n => n.children || []).some(n => n.textContent === 'Reconcile this attempt'));
  mutation.resolve(f.response(401)); await tick();
  assert.equal(f.frame.erased, false);
});

test('a stalled scope response body times out and keeps the page masked', async () => {
  const f = fixture({ hidden: true, immediateTimeout: true });
  f.document.hidden = false; f.emit('visibilitychange');
  const call = f.calls[0]; const response = f.response(200);
  response.body = { getReader: () => ({ read: () => new Promise((resolve, reject) => call.init.signal.addEventListener('abort', () => reject(new Error('body aborted')))), releaseLock() {}, cancel: async () => {} }) };
  call.resolve(response);
  await new Promise(resolve => setTimeout(resolve, 10));
  assert.equal(call.init.signal.aborted, true);
  assert.equal(f.frame.erased, false); assert.equal(f.frame.inert, true);
  assert.ok(f.document.getElementById('scope-recovery'));
});

test('malformed scope probe after an unscoped failure retains the frozen operation', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  f.calls[0].resolve(f.response(502)); await tick();
  f.calls[1].resolve(f.response(200, 'bad gateway')); await tick();
  assert.equal(f.frame.erased, false);
  assert.ok(f.status.children.flatMap(n => n.children || []).some(n => n.textContent === 'Reconcile this attempt'));
});

test('prerender stays inert without requests until activation', async () => {
  const f = fixture({ prerendering: true });
  assert.equal(f.frame.inert, true); assert.equal(f.calls.length, 0);
  assert.equal(f.document.title, 'Scry');
  f.document.prerendering = false; f.emit('prerenderingchange');
  assert.equal(f.calls[0].url, '/session/scope');
  f.calls[0].resolve(f.response(200)); await tick();
  assert.equal(f.frame.inert, false); assert.equal(f.document.title, 'Private question · Scry');
});

test('focus and selection return only after successful scope validation', async () => {
  const f = fixture();
  const focus = { isConnected: true, selectionStart: 1, selectionEnd: 4, focus() { this.focused = true; }, setSelectionRange(a,b) { this.selection = [a,b]; } };
  f.document.activeElement = focus;
  f.document.hidden = true; f.emit('visibilitychange');
  assert.equal(f.document.title, 'Scry');
  f.document.hidden = false; f.emit('visibilitychange');
  assert.equal(focus.focused, undefined);
  f.calls[0].resolve(f.response(200)); await tick();
  assert.equal(focus.focused, true); assert.deepEqual(focus.selection, [1,4]);
  assert.equal(f.document.title, 'Private question · Scry');
});

test('an oversized scope response is refused without exposing the page', async () => {
  const f = fixture({ hidden: true });
  f.document.hidden = false; f.emit('visibilitychange');
  f.calls[0].resolve(f.response(200, 'x'.repeat(1024))); await tick();
  assert.equal(f.frame.erased, false); assert.equal(f.frame.inert, true);
  assert.ok(f.document.getElementById('scope-recovery'));
});

test('a delayed poll body cannot replace or abort a newer mutation', async () => {
  const f = fixture({ polling: true }); await new Promise(resolve => setTimeout(resolve, 10));
  f.document.pollable = false;
  const poll = f.calls[0]; const body = deferred();
  const response = f.response(200, '', { 'x-scry-scope': scope, 'content-type': 'text/html' });
  response.text = () => body.promise;
  poll.resolve(response); await tick();
  f.emit('submit', { target: new f.Form(), preventDefault() {} });
  assert.equal(f.calls.length, 2); body.resolve('<html>late page</html>'); await tick();
  assert.equal(f.frame.erased, false); assert.equal(f.calls[1].init.signal.aborted, false);
  f.calls[1].resolve(f.response(503)); await tick(); f.calls[2].resolve(f.response(200)); await tick();
  assert.ok(f.status.children.flatMap(n => n.children || []).some(n => n.textContent === 'Reconcile this attempt'));
});

test('polling resumes on its normal cadence after scope validation', async () => {
  const f = fixture({ prerendering: true, polling: true });
  f.document.prerendering = false; f.emit('prerenderingchange');
  f.calls[0].resolve(f.response(200)); await tick();
  assert.ok(f.timerDelays.includes(4000));
  assert.ok(!f.timerDelays.includes(0), 'Resume must not trigger an immediate poll.');
  await new Promise(resolve => setTimeout(resolve, 10));
  f.document.pollable = false;
  assert.ok(f.calls[1]); f.calls[1].resolve(f.response(503)); await tick();
});

async function reconcileAfterUncertainResponse(f, original) {
  await tick();
  assert.equal(f.frame.erased, false);
  assert.equal(f.calls.length, 2); assert.equal(f.calls[1].url, '/session/scope');
  f.calls[1].resolve(f.response(200)); await tick();
  const actions = f.status.children.flatMap(n => n.children || []);
  const reconcile = actions.find(n => n.textContent === 'Reconcile this attempt');
  assert.ok(reconcile); assert.ok(!actions.some(n => n.textContent === 'Continue editing'));
  reconcile.click(); assert.equal(f.calls[2].init.body, original.init.body);
  f.calls[2].resolve(f.response(401)); await tick();
}

test('a malformed scope header on a gateway response preserves uncertainty', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  const original = f.calls[0];
  original.resolve(f.response(503, 'gateway unavailable', { 'x-scry-scope': 'not-a-scope' }));
  await reconcileAfterUncertainResponse(f, original);
});

test('a scoped HTML success without its embedded scope preserves uncertainty', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  const original = f.calls[0];
  original.resolve(f.response(200, '<html><main>incomplete</main></html>', { 'x-scry-scope': scope, 'content-type': 'text/html' }));
  await reconcileAfterUncertainResponse(f, original);
});

test('a scoped HTML rejection with a conflicting embedded scope disposes the page', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  f.calls[0].resolve(f.response(422, `<html data-notebook-scope="${'b'.repeat(64)}"><div class="notice">wrong notebook</div></html>`, { 'x-scry-scope': scope, 'content-type': 'text/html' }));
  await tick(); assert.equal(f.frame.erased, true);
  assert.ok(!f.status.children.flatMap(n => n.children || []).some(n => n.textContent === 'Continue editing'));
});

test('a scoped HTML rejection with malformed embedded scope preserves uncertainty', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  const original = f.calls[0];
  original.resolve(f.response(422, '<html data-notebook-scope="broken"><div class="notice">incomplete</div></html>', { 'x-scry-scope': scope, 'content-type': 'text/html' }));
  await reconcileAfterUncertainResponse(f, original);
});

test('a consistent scoped HTML rejection unlocks the form', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  f.calls[0].resolve(f.response(422, `<html data-notebook-scope="${scope}"><div class="notice">refused</div></html>`, { 'x-scry-scope': scope, 'content-type': 'text/html' }));
  await tick(); assert.equal(f.frame.erased, false); assert.equal(f.calls.length, 1);
  assert.ok(f.status.children.flatMap(n => n.children || []).some(n => n.textContent === 'Continue editing'));
});

test('a mismatch marker on a gateway error is not treated as an authenticated mismatch', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  const original = f.calls[0];
  original.resolve(f.response(503, 'gateway error', { 'x-scry-scope-mismatch': '1' }));
  await reconcileAfterUncertainResponse(f, original);
});

test('a late successful poll cannot replace a form dirtied during the request', async () => {
  const f = fixture({ polling: true }); const form = new f.Form();
  f.document.forms.push(form); await new Promise(resolve => setTimeout(resolve, 10));
  assert.equal(f.calls.length, 1); f.document.pollable = false;
  form.value = 'unfinished answer'; f.emit('input', { target: { form } });
  f.calls[0].resolve(f.response(200, `<html data-notebook-scope="${scope}"><main></main></html>`, { 'x-scry-scope': scope, 'content-type': 'text/html' }));
  await tick();
  assert.equal(f.previousMain.replaced, false); assert.equal(form.value, 'unfinished answer');
});

test('a successful poll can still update a clean page', async () => {
  const f = fixture({ polling: true }); await new Promise(resolve => setTimeout(resolve, 10));
  assert.equal(f.calls.length, 1); f.document.pollable = false;
  f.calls[0].resolve(f.response(200, `<html data-notebook-scope="${scope}"><main></main></html>`, { 'x-scry-scope': scope, 'content-type': 'text/html' }));
  await tick(); assert.equal(f.previousMain.replaced, true);
});

test('a goal poll retains a dirty form when the replacement omits it', async () => {
  const f = fixture({ polling: true, pollState: 'goal' }); const form = new f.Form();
  f.document.forms.push(form); await new Promise(resolve => setTimeout(resolve, 10));
  f.document.pollable = false; form.value = 'unfinished goal'; f.emit('input', { target: { form } });
  f.calls[0].resolve(f.response(200, `<html data-notebook-scope="${scope}"><main></main></html>`, { 'x-scry-scope': scope, 'content-type': 'text/html' }));
  await tick(); assert.equal(f.previousMain.replaced, false); assert.equal(form.value, 'unfinished goal');
});

test('a non-200 successful scope response does not reveal a suspended page', async () => {
  const f = fixture({ hidden: true }); f.document.hidden = false; f.emit('visibilitychange');
  f.calls[0].resolve(f.response(201)); await tick();
  assert.equal(f.frame.erased, false); assert.equal(f.frame.inert, true);
  assert.ok(f.document.getElementById('scope-recovery'));
});

test('a scoped HTML conflict with a conflicting embedded scope cannot unlock the form', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  f.calls[0].resolve(f.response(409, `<html data-notebook-scope="${'b'.repeat(64)}"></html>`, { 'x-scry-scope': scope, 'content-type': 'text/html' }));
  await tick(); assert.equal(f.frame.erased, true);
});

test('a scoped HTML conflict with a missing embedded scope preserves uncertainty', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  const original = f.calls[0];
  original.resolve(f.response(409, '<html><main>incomplete</main></html>', { 'x-scry-scope': scope, 'content-type': 'text/html' }));
  await reconcileAfterUncertainResponse(f, original);
});

test('a cross-origin rejection cannot unlock the private form', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  const response = f.response(422, '', { 'x-scry-scope': scope }); response.url = 'https://other.invalid/';
  f.calls[0].resolve(response); await tick(); assert.equal(f.frame.erased, true);
});

test('a scoped HTML rejection missing its embedded scope preserves uncertainty', async () => {
  const f = fixture(); f.emit('submit', { target: new f.Form(), preventDefault() {} });
  const original = f.calls[0];
  original.resolve(f.response(422, '<html><div class="notice">incomplete</div></html>', { 'x-scry-scope': scope, 'content-type': 'text/html' }));
  await reconcileAfterUncertainResponse(f, original);
});

function practiceDraft(f, value = 'unfinished synthetic answer') {
  const form = new f.Form(); form.operationId = 'operation-unchanged'; form.promptId = 'prompt-unchanged';
  const answer = { form, value, readOnly: false, disabled: false, type: 'textarea' }; f.document.answerFields.push(answer);
  f.emit('input', { target: answer }); return { form, answer };
}

test('unsent answers do not install an unload warning', () => {
  const f = fixture(); practiceDraft(f);
  const event = { prevented: false, preventDefault() { this.prevented = true; } };
  f.emit('window:beforeunload', event); assert.equal(event.prevented, false);
});

test('canceling an in-page switch preserves answer, prompt and operation without a request', () => {
  const f = fixture(); const { form, answer } = practiceDraft(f);
  const switching = new f.Form(); let prevented = false;
  f.emit('submit', { target: switching, preventDefault() { prevented = true; } });
  assert.equal(prevented, true); assert.equal(f.window.confirmCount, 1); assert.equal(f.calls.length, 0);
  assert.equal(answer.value, 'unfinished synthetic answer'); assert.equal(form.promptId, 'prompt-unchanged');
  assert.equal(form.operationId, 'operation-unchanged');
});

test('confirming an in-page switch sends one deliberate request and keeps it pending', async () => {
  const f = fixture(); const { answer } = practiceDraft(f); f.window.confirmResult = true;
  f.emit('submit', { target: new f.Form(), preventDefault() {} });
  assert.equal(answer.readOnly, true, 'Freeze the answer while its switch is unresolved.');
  assert.equal(f.window.confirmCount, 1); assert.equal(f.calls.length, 1);
  f.calls[0].resolve(f.response(503)); await tick(); f.calls[1].resolve(f.response(200)); await tick();
  assert.ok(f.status.children.flatMap(n => n.children || []).some(n => n.textContent === 'Reconcile this attempt'));
  assert.equal(f.calls.length, 2, 'No automatic resubmission.');
});

test('answer submission skips the switch confirmation and a definite rejection restores editing', async () => {
  const f = fixture(); const { form, answer } = practiceDraft(f);
  f.emit('submit', { target: form, preventDefault() {} });
  assert.equal(f.window.confirmCount, 0); assert.equal(f.calls.length, 1);
  f.calls[0].resolve(f.response(422, 'Rejected', { 'x-scry-scope': scope, 'content-type': 'text/plain' })); await tick();
  assert.equal(answer.readOnly, false);
});

test('a successful answer replaces the form without switch confirmation', async () => {
  const f = fixture(); const { form } = practiceDraft(f);
  f.emit('submit', { target: form, preventDefault() {} });
  const response = f.response(200, `<html data-notebook-scope="${scope}"><main></main></html>`, { 'x-scry-scope': scope, 'content-type': 'text/html' });
  response.url = 'https://scry.test/'; f.calls[0].resolve(response); await tick();
  assert.equal(f.previousMain.replaced, true); assert.equal(f.window.confirmCount, 0);
});

test('pagehide still invokes privacy concealment', () => {
  const f = fixture(); practiceDraft(f);
  f.emit('window:pagehide'); assert.equal(f.frame.erased, true); assert.ok(f.classes.has('private-hidden'));
});

test('background validation preserves the in-memory answer', async () => {
  const f = fixture(); const { answer, form } = practiceDraft(f);
  f.document.hidden = true; f.emit('visibilitychange');
  f.document.hidden = false; f.emit('visibilitychange'); f.calls[0].resolve(f.response(200)); await tick();
  assert.equal(answer.value, 'unfinished synthetic answer'); assert.equal(form.operationId, 'operation-unchanged');
  assert.equal(f.calls.length, 1, 'Only the existing scope check ran.');
});

test('an authentication denial conceals the page', async () => {
  const f = fixture(); const { form } = practiceDraft(f);
  f.emit('submit', { target: form, preventDefault() {} });
  f.calls[0].resolve(f.response(401)); await tick();
  assert.equal(f.frame.erased, true); assert.ok(f.classes.has('private-hidden'));
});

test('a switch confirmation cannot send after the page becomes inactive', () => {
  const f = fixture(); const { answer, form } = practiceDraft(f);
  f.window.confirm = () => { f.document.hidden = true; f.emit('visibilitychange'); return true; };
  f.emit('submit', { target: new f.Form(), preventDefault() {} });
  assert.equal(f.calls.length, 0); assert.equal(answer.value, 'unfinished synthetic answer');
  assert.equal(form.operationId, 'operation-unchanged'); assert.equal(f.frame.inert, true);
});

test('a rejected switch returns the retained answer to editable state', async () => {
  const f = fixture(); const { answer } = practiceDraft(f); f.window.confirmResult = true;
  f.emit('submit', { target: new f.Form(), preventDefault() {} }); assert.equal(answer.readOnly, true);
  f.calls[0].resolve(f.response(422, 'Rejected', { 'x-scry-scope': scope, 'content-type': 'text/plain' })); await tick();
  assert.equal(answer.readOnly, false); assert.equal(answer.value, 'unfinished synthetic answer');
});

test('native answer submission without fetch is not intercepted', () => {
  const f = fixture({ nativeTransport: true }); const { form } = practiceDraft(f);
  let prevented = false; f.emit('submit', { target: form, preventDefault() { prevented = true; } });
  assert.equal(prevented, false);
});

test('an in-flight poll is fenced before a switch confirmation opens', async () => {
  const f = fixture({ polling: true }); await new Promise(resolve => setTimeout(resolve, 10));
  const poll = f.calls[0]; f.document.pollable = false; const { answer } = practiceDraft(f);
  f.window.confirm = () => { assert.equal(poll.init.signal.aborted, true); return false; };
  f.emit('submit', { target: new f.Form(), preventDefault() {} });
  assert.equal(answer.value, 'unfinished synthetic answer'); assert.equal(f.calls.length, 1);
  poll.resolve(f.response(401)); await tick(); assert.equal(f.frame.erased, false);
});

test('going offline during switch confirmation leaves the answer editable', async () => {
  const f = fixture(); const { form, answer } = practiceDraft(f);
  f.window.confirm = () => { f.navigator.onLine = false; return true; };
  f.emit('submit', { target: new f.Form(), preventDefault() {} });
  assert.equal(f.calls.length, 0); assert.equal(answer.readOnly, false);
  assert.equal(answer.value, 'unfinished synthetic answer'); assert.equal(form.operationId, 'operation-unchanged');
  f.navigator.onLine = true; f.emit('window:online');
  f.emit('submit', { target: form, preventDefault() {} });
  assert.equal(f.calls.length, 1, 'The first submission was never sent or left pending.');
  f.calls[0].resolve(f.response(401)); await tick();
});

test('going offline during form serialization does not create an unresolved operation', () => {
  const f = fixture(); const { form, answer } = practiceDraft(f); const switching = new f.Form();
  f.window.confirmResult = true; switching.onFormData = () => { f.navigator.onLine = false; };
  f.emit('submit', { target: switching, preventDefault() {} });
  assert.equal(f.calls.length, 0); assert.equal(answer.readOnly, false);
  assert.equal(answer.value, 'unfinished synthetic answer'); assert.equal(form.operationId, 'operation-unchanged');
});

test('offline reconciliation retains the original previously sent operation', async () => {
  const f = fixture(); const { form } = practiceDraft(f);
  f.emit('submit', { target: form, preventDefault() {} }); const original = f.calls[0];
  original.resolve(f.response(503)); await tick(); f.calls[1].resolve(f.response(200)); await tick();
  let actions = f.status.children.flatMap(n => n.children || []);
  f.navigator.onLine = false; actions.find(n => n.textContent === 'Reconcile this attempt').click();
  assert.equal(f.calls.length, 2);
  f.navigator.onLine = true; f.emit('window:online');
  actions = f.status.children.flatMap(n => n.children || []); actions.find(n => n.textContent === 'Reconcile this attempt').click();
  assert.equal(f.calls.length, 3); assert.equal(f.calls[2].init.body, original.init.body);
  f.calls[2].resolve(f.response(401)); await tick();
});

test('checkpoint acknowledgment is cleared when the answer is edited again', async () => {
  const f = fixture(); const { form, answer } = practiceDraft(f);
  form.querySelector = selector => selector === '[data-checkpoint-generation]' ? {dataset:{generation:'0'}} : selector.includes('answer') ? answer : null;
  const submitter = { formAction: 'https://scry.test/practice/draft', hasAttribute: key => key === 'formaction' || key === 'data-checkpoint-save' };
  f.emit('submit', { target: form, submitter, preventDefault() {} });
  assert.equal(f.calls.length,1); assert.equal(f.calls[0].url,'https://scry.test/practice/draft');
  const response=f.response(200, `<html data-notebook-scope="${scope}"><main></main></html>`, { 'x-scry-scope':scope, 'content-type':'text/html' });
  response.url='https://scry.test/';f.calls[0].resolve(response);await tick();
  assert.equal(f.status.hidden,false);assert.match(f.status.children[0].textContent,/Save confirmed/);
  answer.value='edited after save';f.emit('input',{target:answer});assert.equal(f.status.hidden,true);
});

test('an uncertain checkpoint is frozen and reconciled only as the same save', async () => {
  const f = fixture(); const { form, answer } = practiceDraft(f);
  form.querySelectorAll = selector => selector.startsWith('textarea') ? [answer] : [];
  form.querySelector = selector => selector === '[data-checkpoint-generation]' ? {dataset:{generation:'0'}} : selector.includes('answer') ? answer : null;
  const submitter = { formAction:'https://scry.test/practice/draft', hasAttribute:key=>key==='formaction'||key==='data-checkpoint-save' };
  f.emit('submit',{target:form,submitter,preventDefault(){}});const original=f.calls[0];
  original.resolve(f.response(503));await tick();f.calls[1].resolve(f.response(200));await tick();
  assert.equal(f.calls.length,2);assert.equal(answer.readOnly,true);
  const reconcile=f.status.children.flatMap(n=>n.children||[]).find(n=>n.textContent==='Reconcile this save');
  assert.ok(reconcile);reconcile.click();assert.equal(f.calls.length,3);
  assert.equal(f.calls[2].url,original.url);assert.equal(f.calls[2].init.body,original.init.body);
  f.calls[2].resolve(f.response(401));await tick();
});


test('discard cancellation preserves the answer and an uncertain discard reconciles exactly', async()=>{
  const f=fixture();const {form,answer}=practiceDraft(f);
  form.querySelectorAll=selector=>selector.startsWith('textarea')?[answer]:[];
  form.querySelector=selector=>selector==='[data-checkpoint-generation]'?{dataset:{generation:'1'}}:selector.includes('answer')?answer:null;
  const submitter={formAction:'https://scry.test/practice/draft/discard',hasAttribute:key=>key==='formaction'||key==='data-checkpoint-discard'};
  f.emit('submit',{target:form,submitter,preventDefault(){}});assert.equal(f.calls.length,0);assert.equal(answer.value,'unfinished synthetic answer');
  f.window.confirmResult=true;f.emit('submit',{target:form,submitter,preventDefault(){}});const original=f.calls[0];assert.equal(answer.readOnly,true);
  original.resolve(f.response(503));await tick();f.calls[1].resolve(f.response(200));await tick();
  const reconcile=f.status.children.flatMap(n=>n.children||[]).find(n=>n.textContent==='Reconcile this discard');assert.ok(reconcile);
  assert.equal(f.calls.length,2);assert.match(f.status.children[1].textContent,/does not send or grade your answer/);reconcile.click();assert.equal(f.calls[2].init.body,original.init.body);assert.equal(f.calls[2].url,original.url);
  f.calls[2].resolve(f.response(401));await tick();
});

function autosaveForm(f) {
  const form=new f.Form();form.classList={contains:name=>name==='answer-form'};form.fields={csrf:'synthetic-csrf',prompt_id:'1'.repeat(32),operation_id:'manual-operation-000001'};
  const generation={dataset:{generation:'0'}};
  const input={form,value:'first',readOnly:false,disabled:false,matches:()=>true};form.answerInput=input;
  form.querySelector=selector=>selector==='[data-checkpoint-generation]'?generation:selector.includes('answer')?input:null;
  return {form,input,generation};
}
function autosaveResponse(f,call,generation,current=true,remaining=511) {
  const json=JSON.stringify({version:1,operation_id:call.init.body.get('operation_id'),prompt_id:call.init.body.get('prompt_id'),generation:String(generation),current,remaining});
  return f.response(200,json,{'x-scry-scope':scope});
}
test('foreground autosave preserves the editor and distinguishes newer typing',async()=>{
  const f=fixture({fastAuto:true});const {form,input,generation}=autosaveForm(f);
  assert.equal(f.calls.length,0,'Loading does not save.');
  f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));
  assert.equal(f.calls.length,1);const first=f.calls[0];assert.equal(first.init.headers['X-Scry-Checkpoint-Mode'],'auto-v1');assert.equal(input.readOnly,false);
  input.value='first and later';f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,1,'Only one snapshot can be unresolved.');
  first.resolve(autosaveResponse(f,first,1));await tick();assert.equal(f.previousMain.replaced,false);assert.equal(input.value,'first and later');assert.equal(generation.dataset.generation,'1');assert.match(f.status.children[0].textContent,/Newer changes are not yet saved/);
  await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,2);assert.notEqual(f.calls[1].init.body.get('operation_id'),first.init.body.get('operation_id'));assert.equal(f.calls[1].init.body.get('generation'),'1');
  f.calls[1].resolve(autosaveResponse(f,f.calls[1],2));await tick();assert.equal(f.status.children[0].textContent,'Saved.');
});
test('an old-server redirect retains the exact automatic operation for manual reconciliation',async()=>{
  const f=fixture({fastAuto:true});const {input}=autosaveForm(f);f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));
  const first=f.calls[0];first.resolve({...f.response(0),type:'opaqueredirect'});await tick();assert.equal(f.calls[1].url,'/session/scope');f.calls[1].resolve(f.response(200));await tick();
  assert.equal(f.frame.erased,false);const reconcile=f.status.children.flatMap(x=>x.children||[]).find(x=>x.textContent==='Reconcile this save');assert.ok(reconcile);assert.equal(f.calls.length,2);reconcile.click();assert.equal(f.calls[2].init.body,first.init.body);
  f.calls[2].resolve(autosaveResponse(f,f.calls[2],1));await tick();
});
test('backgrounding cancels an unsent timer and foreground return does not save',async()=>{
  const f=fixture();const {input}=autosaveForm(f);f.emit('input',{target:input});f.document.hidden=true;f.emit('visibilitychange');f.document.hidden=false;f.emit('visibilitychange');
  assert.equal(f.calls.length,1);f.calls[0].resolve(f.response(200));await tick();assert.equal(f.calls.length,1);assert.equal(input.value,'first');
});
test('stale checkpoint acknowledgment never adopts a competing generation',async()=>{
  const f=fixture({fastAuto:true});const {input,generation}=autosaveForm(f);f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));const call=f.calls[0];call.resolve(autosaveResponse(f,call,7,false));await tick();assert.equal(generation.dataset.generation,'0');assert.match(f.status.children[0].textContent,/not saved/);
  input.value='still unsaved';f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,1);
});

test('automatic save waits for composition and never starts on reconnect alone',async()=>{
  const f=fixture({fastAuto:true});const {input}=autosaveForm(f);f.emit('compositionstart',{target:input});f.emit('input',{target:input,isComposing:true});await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,0);
  f.emit('compositionend',{target:input});await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,1);f.calls[0].resolve(autosaveResponse(f,f.calls[0],1));await tick();
  f.navigator.onLine=false;f.emit('window:offline');input.value='offline edit';f.emit('input',{target:input});f.navigator.onLine=true;f.emit('window:online');await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,1);assert.match(f.status.children[0].textContent,/not saved/);
});
test('the automatic budget refusal leaves editing and manual saving available',async()=>{
  const f=fixture({fastAuto:true});const {form,input}=autosaveForm(f);f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));f.calls[0].resolve(f.response(429,'limit',{'x-scry-scope':scope}));await tick();assert.equal(input.readOnly,false);assert.match(f.status.children[0].textContent,/Use Save draft/);
  input.value='more';f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,1);const submitter={formAction:'https://scry.test/practice/draft',hasAttribute:key=>key==='formaction'||key==='data-checkpoint-save'};f.emit('submit',{target:form,submitter,preventDefault(){}});assert.equal(f.calls.length,2);f.calls[1].resolve(f.response(401));await tick();
});

test('backgrounded automatic save remains frozen until manual reconciliation',async()=>{
  const f=fixture({fastAuto:true});const {input}=autosaveForm(f);f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));const original=f.calls[0];f.document.hidden=true;f.emit('visibilitychange');assert.equal(original.init.signal.aborted,true);original.resolve(f.response(503));await tick();
  f.document.hidden=false;f.emit('visibilitychange');f.calls[1].resolve(f.response(200));await tick();assert.equal(f.calls.length,2);const action=f.status.children.flatMap(x=>x.children||[]).find(x=>x.textContent==='Reconcile this save');assert.ok(action);action.click();assert.equal(f.calls[2].init.body,original.init.body);f.calls[2].resolve(autosaveResponse(f,f.calls[2],1));await tick();
});
test('an identity mismatch conceals an automatic save without exposing the editor',async()=>{
  const f=fixture({fastAuto:true});const {input}=autosaveForm(f);f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));f.calls[0].resolve(f.response(200,'{}',{'x-scry-scope':'b'.repeat(64)}));await tick();assert.equal(f.frame.erased,true);assert.ok(f.classes.has('private-hidden'));
});
test('malformed automatic acknowledgment retains the exact operation',async()=>{
  const f=fixture({fastAuto:true});const {input}=autosaveForm(f);f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));const original=f.calls[0];original.resolve(f.response(200,'{}',{'x-scry-scope':scope}));await tick();assert.equal(f.calls[1].url,'/session/scope');f.calls[1].resolve(f.response(200));await tick();const action=f.status.children.flatMap(x=>x.children||[]).find(x=>x.textContent==='Reconcile this save');assert.ok(action);action.click();assert.equal(f.calls[2].init.body,original.init.body);f.calls[2].resolve(autosaveResponse(f,f.calls[2],1));await tick();
});

test('deleting text can checkpoint an empty answer without submitting it',async()=>{
  const f=fixture({fastAuto:true});const {input}=autosaveForm(f);input.value='';f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,1);assert.equal(f.calls[0].url,'https://scry.test/practice/draft');f.calls[0].resolve(autosaveResponse(f,f.calls[0],1));await tick();assert.equal(f.status.children[0].textContent,'Saved.');
});

test('an opaque old-server scope redirect masks but retains the automatic operation',async()=>{
  const f=fixture({fastAuto:true});const {input}=autosaveForm(f);f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));const original=f.calls[0];
  original.resolve({...f.response(0),type:'opaqueredirect'});await tick();assert.equal(f.calls[1].url,'/session/scope');f.calls[1].resolve({...f.response(0),type:'opaqueredirect'});await tick();
  assert.equal(f.frame.erased,false);assert.equal(f.frame.inert,true);assert.ok(f.classes.has('private-suspended'));const panel=f.document.getElementById('scope-recovery');assert.ok(panel);panel.children.find(x=>x.textContent==='Try again').click();f.calls[2].resolve({...f.response(0),type:'opaqueredirect'});await tick();assert.equal(f.frame.erased,false);assert.equal(f.frame.inert,true);assert.equal(f.calls.length,3);
});

test('automatic save refuses a missing draft-generation control',async()=>{
  const f=fixture({fastAuto:true});const {form,input}=autosaveForm(f);form.querySelector=selector=>selector.includes('answer')?input:null;
  f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,0);assert.match(f.status.children[0].textContent,/not saved/);
});

test('going offline during automatic form serialization never sends or queues a snapshot',async()=>{
  const f=fixture({fastAuto:true});const {form,input}=autosaveForm(f);form.onFormData=()=>{f.navigator.onLine=false;f.emit('window:offline');};f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,0);assert.equal(input.value,'first');f.navigator.onLine=true;f.emit('window:online');await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,0);
});
test('a synchronous form-data edit cannot be mistaken for the frozen snapshot',async()=>{
  const f=fixture({fastAuto:true});const {form,input}=autosaveForm(f);form.onFormData=()=>{input.value='newer';};f.emit('input',{target:input});await new Promise(r=>setTimeout(r,10));assert.equal(f.calls.length,0);assert.match(f.status.children[0].textContent,/not saved/);
});
