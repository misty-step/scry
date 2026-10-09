/* Presentation and durable-response reconciliation only. No offline mutation queue. */
(() => {
  'use strict';
  let pending = null;
  let pollTimer = null;
  let pageSequence = 0;
  const dirtyForms = new WeakSet();
  document.addEventListener('input', event => { if (event.target.form) dirtyForms.add(event.target.form); });
  document.addEventListener('change', event => { if (event.target.form) dirtyForms.add(event.target.form); });
  const status = () => document.getElementById('request-status');
  const hideStatus = () => { const box = status(); if (box) { box.hidden = true; box.replaceChildren(); } };
  const announce = (message, actions = [], detail = '') => {
    const box = status(); if (!box) return;
    box.replaceChildren();
    const copy = document.createElement('p'); copy.textContent = message; box.append(copy);
    if (detail) { const note = document.createElement('p'); note.className = 'request-detail'; note.textContent = detail; box.append(note); }
    if (actions.length) {
      const row = document.createElement('div'); row.className = 'status-actions';
      actions.forEach(([label, action]) => { const button = document.createElement('button'); button.type = 'button'; button.textContent = label; button.addEventListener('click', action); row.append(button); });
      box.append(row);
    }
    box.hidden = false;
  };
  const conceal = () => {
    clearTimeout(pollTimer);
    pending = null;
    document.querySelector('.page-frame')?.replaceChildren();
    document.body.classList.add('private-hidden');
  };
  const safePage = async response => {
    if (response.status === 401 || response.status === 403) { conceal(); return null; }
    if (new URL(response.url, location.href).origin !== location.origin) { conceal(); return null; }
    const type = response.headers.get('content-type') || '';
    if (!type.includes('text/html')) throw new Error('The server returned an unexpected response.');
    const html = await response.text();
    const doc = new DOMParser().parseFromString(html, 'text/html');
    if (!doc.querySelector('#main') || !doc.querySelector('.masthead')) throw new Error('The saved page could not be read.');
    return doc;
  };
  const install = (doc, response, navigation = true) => {
    const main = doc.querySelector('#main');
    const previousMain = document.getElementById('main');
    if (!main || !previousMain) return;
    previousMain.replaceWith(main);
    const header = doc.querySelector('.masthead');
    if (header) document.querySelector('.masthead')?.replaceWith(header);
    document.title = doc.title;
    pageSequence += 1;
    if (navigation) {
      const url = new URL(response.url, location.href);
      if (url.pathname + url.search !== location.pathname + location.search) history.pushState(null, '', url.pathname + url.search);
      main.classList.add('new-stage');
      window.scrollTo({ top: 0, behavior: 'instant' });
      main.focus({ preventScroll: true });
    }
    hideStatus();
    schedulePoll();
  };
  const unlock = request => {
    request.form?.querySelectorAll('button').forEach(button => { button.disabled = false; });
    request.form?.removeAttribute('aria-busy');
    request.inputLocks?.forEach(([input, readOnly, disabled]) => { input.readOnly = readOnly; input.disabled = disabled; });
  };
  const send = async request => {
    if (request.sending) return;
    if (!navigator.onLine) {
      announce('You are offline. Your draft is still here.', [], 'Study pauses until you reconnect. Nothing is queued to send automatically.');
      return;
    }
    request.sending = true;
    if (!request.inputLocks) {
      request.inputLocks = [...(request.form?.querySelectorAll('textarea,input:not([type=hidden]),select') || [])].map(input => [input, input.readOnly, input.disabled]);
      request.inputLocks.forEach(([input]) => { if ('readOnly' in input && input.type !== 'file' && input.type !== 'radio' && input.type !== 'checkbox') input.readOnly = true; else input.disabled = true; });
    }
    request.form?.setAttribute('aria-busy', 'true');
    request.form?.querySelectorAll('button').forEach(button => { button.disabled = true; });
    announce('Saving…');
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 20000);
    try {
      const response = await fetch(request.action, {
        method: 'POST', body: request.data, credentials: 'same-origin', redirect: 'follow',
        headers: { Accept: 'text/html', 'X-Scry-Request': 'page' }, signal: controller.signal
      });
      if (pending !== request) return;
      if (response.status === 401 || response.status === 403) { conceal(); return; }
      if (response.status === 409) {
        pending = null;
        unlock(request);
        announce('This changed in another tab. Your draft is still here.', [['Open saved progress', () => location.reload()]], 'Reload deliberately to see the current question or saved result.');
        return;
      }
      if ([400, 413, 415, 422].includes(response.status)) {
        // A definite rejection may be an HTML page or a plain transport error.
        const text = await response.text();
        let error = text.trim().slice(0, 1000);
        if ((response.headers.get('content-type') || '').includes('text/html')) {
          const doc = new DOMParser().parseFromString(text, 'text/html');
          error = doc.querySelector('.notice, .empty-stage .lede')?.textContent?.trim() || '';
        }
        if (pending !== request) return;
        pending = null;
        unlock(request);
        announce(error || 'This could not be saved. Check your draft and try again.', [['Continue editing', hideStatus]]);
        return;
      }
      if (!response.ok) throw new Error('The response did not confirm what was saved.');
      const doc = await safePage(response);
      if (!doc || pending !== request) return;
      pending = null;
      install(doc, response);
    } catch {
      if (pending !== request) return;
      // Access can redirect to another origin before the Worker returns 401.
      // A read-only manual-redirect probe distinguishes that from response loss.
      if (navigator.onLine) {
        try {
          const probe = await fetch('/readyz', { credentials: 'same-origin', redirect: 'manual', cache: 'no-store', signal: AbortSignal.timeout(5000) });
          if ([401, 403].includes(probe.status) || probe.type === 'opaqueredirect' || (probe.url && new URL(probe.url, location.href).origin !== location.origin)) { conceal(); return; }
        } catch { /* Unavailable is still unknown; retain the frozen attempt. */ }
      }
      if (pending !== request) return;
      // A lost response may follow a committed write. Freeze the exact payload.
      announce('The response did not arrive. Your draft is still here.', [
        ['Reconcile this attempt', () => send(request)],
        ['Open saved progress', () => location.reload()]
      ], 'Reconciliation sends the same saved operation and answer. It cannot record a second attempt.');
    } finally { clearTimeout(timer); request.sending = false; }
  };
  document.addEventListener('submit', event => {
    const form = event.target;
    if (!(form instanceof HTMLFormElement) || form.method.toLowerCase() !== 'post') return;
    if (typeof fetch !== 'function' || typeof FormData !== 'function') return;
    event.preventDefault();
    if (pending) { announce('An earlier response still needs to be reconciled.', [['Reconcile that attempt', () => send(pending)], ['Open saved progress', () => location.reload()]], 'Only one answer can be unresolved at a time.'); return; }
    if (!navigator.onLine) { announce('You are offline. Your draft is still here.', [], 'Study pauses until you reconnect. Nothing is queued to send automatically.'); return; }
    const photo = form.querySelector('input[type=file]')?.files?.[0];
    if (photo && photo.size > 4 * 1024 * 1024) { announce('Choose a photo smaller than 4 MB. Your written draft is still here.'); return; }
    const intent = form.querySelector('textarea[name=intent]');
    if (intent && photo && new TextEncoder().encode(intent.value).length > 1024) { announce('A photo caption can contain up to 1 KB of text. Your draft is still here.'); return; }
    if (intent && new TextEncoder().encode(intent.value).length > 32768) { announce('Your request is over the 32 KB limit. Shorten it and keep the rest separately; your draft is still here.'); return; }
    const answer = form.querySelector('textarea[name=answer]');
    if (form.classList.contains('answer-form') && answer && new TextEncoder().encode(answer.value).length > 4000) { announce('Keep your answer under 4,000 bytes. Your draft is still here.'); return; }
    const submitter = event.submitter;
    const data = new FormData(form);
    if (submitter?.name) data.append(submitter.name, submitter.value);
    pending = { form, data, action: submitter?.hasAttribute('formaction') ? submitter.formAction : form.action };
    send(pending);
  });
  document.addEventListener('click', event => {
    if (!pending) return;
    const link = event.target.closest?.('a');
    if (!link || link.origin !== location.origin) return;
    event.preventDefault();
    announce('First, reconcile the response that did not arrive.', [['Reconcile this attempt', () => send(pending)], ['Open saved progress', () => location.reload()]], 'Your exact answer is kept on this page until you choose what to do.');
  });
  document.addEventListener('keydown', event => {
    if (pending || event.altKey || event.ctrlKey || event.metaKey || event.repeat || event.isComposing) return;
    const target = event.target;
    const editing = target instanceof HTMLElement && (target.matches('input, textarea, select') || target.isContentEditable);
    if (editing) {
      if (target.matches('textarea#answer') && event.key === 'Enter' && !event.shiftKey && matchMedia('(pointer: fine)').matches) {
        event.preventDefault(); target.form?.requestSubmit();
      }
      return;
    }
    if (target instanceof HTMLElement && target.closest('a,button,summary')) return;
    if (/^[1-9]$/.test(event.key)) {
      const choice = document.querySelector(`[data-choice="${event.key}"]`);
      if (choice) { event.preventDefault(); choice.form?.requestSubmit(choice); }
    } else if (event.key === 'Enter' || event.key === ' ') {
      const next = document.querySelector('form[data-next]');
      if (next) { event.preventDefault(); next.requestSubmit(); }
    }
  });
  const schedulePoll = () => {
    clearTimeout(pollTimer);
    if (!document.querySelector('#main[data-poll], [data-poll]')) return;
    pollTimer = setTimeout(async () => {
      if (pending || !navigator.onLine || document.hidden || document.activeElement?.matches('textarea,input,select') || document.querySelector('form[aria-busy]') || [...document.forms].some(form => dirtyForms.has(form))) { schedulePoll(); return; }
      const sequence = pageSequence;
      try {
        const response = await fetch(location.href, { credentials: 'same-origin', headers: { Accept: 'text/html', 'X-Scry-Request': 'poll' }, cache: 'no-store' });
        const doc = await safePage(response);
        if (doc && response.ok && !pending && sequence === pageSequence) {
          // Never replace an editable page or an unanswered question just to poll.
          const currentState = document.querySelector('[data-state]')?.getAttribute('data-state');
          if (['checking', 'pending', 'preparing', 'goal', 'caught-up', 'edit'].includes(currentState)) install(doc, response, false);
        }
      } catch { /* Poll reads never retry paid work or imply success. */ }
      schedulePoll();
    }, 4000);
  };
  window.addEventListener('offline', () => announce('You are offline. Your draft is still here.', [], 'Study pauses until you reconnect. Nothing is queued to send automatically.'));
  window.addEventListener('online', () => {
    if (pending) announce('You are connected again. Reconcile your saved attempt.', [['Reconcile this attempt', () => send(pending)], ['Open saved progress', () => location.reload()]]);
    else { hideStatus(); schedulePoll(); }
  });
  window.addEventListener('pagehide', conceal);
  window.addEventListener('pageshow', event => { if (event.persisted) location.reload(); });
  window.addEventListener('popstate', () => location.reload());
  schedulePoll();
  if (!navigator.onLine) announce('You are offline. Study is paused until you reconnect.');
})();
