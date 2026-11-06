import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';

const script = await readFile(new URL('../assets/app.js', import.meta.url), 'utf8');
const scope = 'a'.repeat(64);
const deferred = () => { let resolve, reject; const promise = new Promise((r, j) => { resolve = r; reject = j; }); return { promise, resolve, reject }; };
const tick = () => new Promise(resolve => setImmediate(resolve));

function fixture({ hidden = false, prerendering = false, polling = false, immediateTimeout = false } = {}) {
  const listeners = {};
  const calls = [];
  const timerDelays = [];
  const classes = new Set();
  const status = { hidden: true, children: [], replaceChildren() { this.children = []; }, append(n) { this.children.push(n); } };
  const frame = { inert: false, erased: false, replaceChildren() { this.erased = true; } };
  const nodes = new Map();
  const element = () => ({ children: [], className: '', textContent: '', addEventListener(type, f) { this[type] = f; }, append(...items) { this.children.push(...items); }, setAttribute() {}, focus() { this.focused = true; }, remove() { nodes.delete(this.id); } });
  class Form { constructor() { this.method = 'post'; this.action = 'https://scry.test/practice/answer'; this.dataset = {}; this.classList = { contains: () => false }; } querySelector() { return null; } querySelectorAll() { return []; } setAttribute() {} removeAttribute() {} }
  const document = {
    hidden, prerendering, title: 'Private question · Scry', pollable: polling, activeElement: null, forms: [], documentElement: { dataset: { notebookScope: scope } },
    body: { classList: { add: c => classes.add(c), remove: c => classes.delete(c) }, append(n) { nodes.set(n.id, n); } },
    addEventListener(type, f) { (listeners[type] ||= []).push(f); },
    querySelector(selector) { if (selector === '.page-frame') return frame; if (selector === '#main[data-poll], [data-poll]' && this.pollable) return {}; return null; },
    querySelectorAll() { return []; },
    getElementById(id) { return id === 'request-status' ? status : nodes.get(id); },
    createElement: element,
  };
  const window = { addEventListener(type, f) { (listeners[`window:${type}`] ||= []).push(f); } };
  const fetch = (url, init) => {
    const gate = deferred();
    if (immediateTimeout && url === '/session/scope') init.signal.addEventListener('abort', () => gate.reject(new Error('aborted')));
    calls.push({ url, init, ...gate }); return gate.promise;
  };
  const clock = (fn, delay) => { timerDelays.push(delay); return setTimeout(fn, (polling && delay === 4000) || (immediateTimeout && delay === 8000) ? 0 : delay); };
  const context = { document, window, fetch, navigator: { onLine: true }, location: { href: 'https://scry.test/', origin: 'https://scry.test', pathname: '/', search: '', reload() {} }, HTMLFormElement: Form, HTMLElement: class {}, FormData: class { constructor(form) { this.form = form; } append() {} }, AbortController, AbortSignal, TextEncoder, TextDecoder, URL, setTimeout: clock, clearTimeout, matchMedia: () => ({ matches: false }), history: {}, console };
  vm.runInNewContext(script, context);
  const emit = (type, event = {}) => (listeners[type] || []).forEach(f => f(event));
  const response = (statusCode, text = scope, headers = {}) => {
    const raw = /^[a-f0-9]{64}$/.test(text) ? JSON.stringify({version: 1, scope: text}) : text;
    const bytes = new TextEncoder().encode(raw); let read = false;
    return { status: statusCode, ok: statusCode >= 200 && statusCode < 300, type: 'basic', url: 'https://scry.test/session/scope', headers: { get: key => headers[key.toLowerCase()] || (key.toLowerCase() === 'content-type' ? 'application/json' : null) }, text: async () => text, body: { getReader: () => ({ read: async () => read ? {done:true} : (read = true, {done:false,value:bytes}), releaseLock() {}, cancel: async () => {} }) } };
  };
  return { document, frame, status, classes, calls, timerDelays, emit, response, Form };
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
