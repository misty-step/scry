import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';

const script = await readFile(new URL('../assets/app.js', import.meta.url), 'utf8');
const scope = 'a'.repeat(64);
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r; }); return { promise, resolve }; };
const tick = () => new Promise(resolve => setImmediate(resolve));

function fixture({ hidden = false } = {}) {
  const listeners = {};
  const calls = [];
  const classes = new Set();
  const status = { hidden: true, children: [], replaceChildren() { this.children = []; }, append(n) { this.children.push(n); } };
  const frame = { inert: false, erased: false, replaceChildren() { this.erased = true; } };
  const nodes = new Map();
  const element = () => ({ children: [], className: '', textContent: '', addEventListener(type, f) { this[type] = f; }, append(...items) { this.children.push(...items); }, setAttribute() {}, remove() { nodes.delete(this.id); } });
  class Form { constructor() { this.method = 'post'; this.action = 'https://scry.test/practice/answer'; this.dataset = {}; this.classList = { contains: () => false }; } querySelector() { return null; } querySelectorAll() { return []; } setAttribute() {} removeAttribute() {} }
  const document = {
    hidden, activeElement: null, forms: [], documentElement: { dataset: { notebookScope: scope } },
    body: { classList: { add: c => classes.add(c), remove: c => classes.delete(c) }, append(n) { nodes.set(n.id, n); } },
    addEventListener(type, f) { (listeners[type] ||= []).push(f); },
    querySelector(selector) { if (selector === '.page-frame') return frame; return null; },
    querySelectorAll() { return []; },
    getElementById(id) { return id === 'request-status' ? status : nodes.get(id); },
    createElement: element,
  };
  const window = { addEventListener(type, f) { (listeners[`window:${type}`] ||= []).push(f); } };
  const fetch = (url, init) => { const gate = deferred(); calls.push({ url, init, ...gate }); return gate.promise; };
  const context = { document, window, fetch, navigator: { onLine: true }, location: { href: 'https://scry.test/', origin: 'https://scry.test', pathname: '/', search: '', reload() {} }, HTMLFormElement: Form, HTMLElement: class {}, FormData: class { constructor(form) { this.form = form; } append() {} }, AbortController, AbortSignal, TextEncoder, URL, setTimeout, clearTimeout, matchMedia: () => ({ matches: false }), history: {}, console };
  vm.runInNewContext(script, context);
  const emit = (type, event = {}) => (listeners[type] || []).forEach(f => f(event));
  const response = (statusCode, text = scope, headers = {}) => ({ status: statusCode, ok: statusCode >= 200 && statusCode < 300, type: 'basic', url: 'https://scry.test/session/scope', headers: { get: key => headers[key.toLowerCase()] || null }, text: async () => text });
  return { document, frame, status, classes, calls, emit, response, Form };
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
