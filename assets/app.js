/* Presentation and durable-response reconciliation only. No offline mutation queue. */
(() => {
  'use strict';
  document.addEventListener('pointerdown', event => {
    const details = event.target.closest?.('#nato-progress > summary')?.parentElement;
    if (details) details.classList.toggle('animate-open', !details.open);
  }, { passive: true });
  document.addEventListener('keydown', () => document.querySelector('#nato-progress')?.classList.remove('animate-open'));
  const enablePickerReturn = () => document.querySelectorAll('[data-picker-close]').forEach(button => { button.hidden = false; });
  document.addEventListener('click', event => {
    const button = event.target.closest?.('[data-picker-close]');
    if (!button) return;
    const picker = button.closest('.lesson-picker');
    if (!picker) return;
    picker.open = false;
    const target = document.querySelector('#answer') || document.querySelector('[data-next] button') || picker.querySelector('summary');
    target?.focus({ preventScroll: true });
    (document.querySelector('.lesson-stage, .nato-stage') || target)?.scrollIntoView({ block: 'start', behavior: 'instant' });
  });
  enablePickerReturn();
  let pending = null;
  let pollTimer = null;
  let pollRequest = null;
  let pageSequence = 0;
  const scope = document.documentElement.dataset.notebookScope;
  let epoch = 0;
  let suspended = false;
  let disposed = false;
  let validation = null;
  let savedFocus = null;
  let savedSelection = null;
  let savedTitle = '';
  const inactive = () => document.hidden || document.prerendering;
  const scoped = /^[a-f0-9]{64}$/.test(scope || '');
  const current = (generation, flight) => scoped && !disposed && !suspended && !inactive() && epoch === generation && (!flight || flight.active);
  const expectedHeaders = { 'X-Scry-Expected-Scope': scope || '' };
  const readScope = async response => {
    if (!(response.headers.get('content-type') || '').includes('application/json') || !response.body) throw new Error('The scope check had no JSON body.');
    const reader = response.body.getReader();
    const chunks = [];
    let length = 0;
    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        length += value.byteLength;
        if (length > 512) { reader.cancel().catch(() => {}); throw new Error('The scope check was too large.'); }
        chunks.push(value);
      }
    } finally { reader.releaseLock(); }
    const bytes = new Uint8Array(length);
    let offset = 0;
    chunks.forEach(chunk => { bytes.set(chunk, offset); offset += chunk.byteLength; });
    const parsed = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes));
    if (!parsed || parsed.version !== 1 || typeof parsed.scope !== 'string' || !/^[a-f0-9]{64}$/.test(parsed.scope) || Object.keys(parsed).length !== 2) throw new Error('The scope check was incomplete.');
    return parsed.scope;
  };
  const dirtyForms = new WeakSet();
  const unsentAnswer = () => [...document.querySelectorAll('form.answer-form textarea[name=answer], form.answer-form input[type=text][name=answer]')].find(input => !input.disabled && input.value.length > 0);
  const warnDeparture = event => {
    if (disposed || pending || !unsentAnswer()) return;
    event.preventDefault();
    event.returnValue = 'You have an unsent answer.';
  };
  let departureWarningAttached = false;
  const refreshDepartureWarning = () => {
    const needed = !disposed && !pending && !!unsentAnswer();
    if (needed === departureWarningAttached) return;
    if (needed) window.addEventListener('beforeunload', warnDeparture);
    else window.removeEventListener('beforeunload', warnDeparture);
    departureWarningAttached = needed;
  };
  document.addEventListener('input', event => { if (event.target.form) dirtyForms.add(event.target.form); refreshDepartureWarning(); });
  document.addEventListener('change', event => { if (event.target.form) dirtyForms.add(event.target.form); refreshDepartureWarning(); });
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
    if (disposed) return;
    disposed = true;
    refreshDepartureWarning();
    epoch += 1;
    suspended = true;
    validation?.abort();
    clearTimeout(pollTimer);
    pollRequest?.abort();
    pending?.flight?.controller.abort();
    pending = null;
    savedFocus = null;
    savedSelection = null;
    savedTitle = '';
    document.title = 'Scry';
    document.querySelector('.page-frame')?.replaceChildren();
    document.getElementById('scope-recovery')?.remove();
    document.body.classList.add('private-hidden');
  };
  const verifyScope = actual => {
    if (typeof actual !== 'string' || !/^[a-f0-9]{64}$/.test(actual)) throw new Error('The response did not confirm its notebook.');
    if (actual !== scope) { conceal(); return false; }
    return true;
  };
  const safePage = async (response, valid) => {
    if (!valid()) return null;
    if (response.status === 401 || response.status === 403 || (response.status === 409 && response.headers.get('X-Scry-Scope-Mismatch') === '1') || new URL(response.url, location.href).origin !== location.origin) { conceal(); return null; }
    if (!verifyScope(response.headers.get('X-Scry-Scope'))) return null;
    const type = response.headers.get('content-type') || '';
    if (!type.includes('text/html')) throw new Error('The server returned an unexpected response.');
    const html = await response.text();
    if (!valid()) return null;
    const doc = new DOMParser().parseFromString(html, 'text/html');
    if (!verifyScope(doc.documentElement.dataset.notebookScope)) return null;
    if (!doc.querySelector('#main') || !doc.querySelector('.masthead')) throw new Error('The saved page could not be read.');
    return doc;
  };
  const install = (doc, response, navigation = true, preserveDrafts = false) => {
    const main = doc.querySelector('#main');
    const previousMain = document.getElementById('main');
    if (!main || !previousMain) return;
    const focused = document.activeElement;
    const selection = focused?.selectionStart == null ? null : [focused.selectionStart, focused.selectionEnd];
    if (preserveDrafts) {
      // Keep the actual draft form, including its operation identity. Only a
      // goal's read-only preparation/reference changes while the learner types.
      [...previousMain.querySelectorAll('form')].filter(form => dirtyForms.has(form)).forEach(form => {
        const replacement = [...main.querySelectorAll('form')].find(next => next.getAttribute('action') === form.getAttribute('action'));
        replacement?.replaceWith(form);
      });
    }
    // Keep the read-only inspector steady while the server updates a review.
    // It never owns a grade, a schedule or a request of its own.
    const oldProgress = previousMain.querySelector('#nato-progress');
    const newProgress = main.querySelector('#nato-progress');
    if (oldProgress && newProgress) {
      newProgress.open = oldProgress.open;
      const selected = oldProgress.querySelector('.progress-radio:checked')?.value;
      const nextSelected = [...newProgress.querySelectorAll('.progress-radio')].find(radio => radio.value === selected);
      if (nextSelected && oldProgress.open) nextSelected.checked = true;
    }
    previousMain.replaceWith(main);
    refreshDepartureWarning();
    enablePickerReturn();
    if (preserveDrafts && focused?.isConnected) {
      focused.focus({ preventScroll: true });
      if (selection) focused.setSelectionRange(...selection);
    }
    const header = doc.querySelector('.masthead');
    if (header) document.querySelector('.masthead')?.replaceWith(header);
    document.title = doc.title;
    doc.querySelectorAll('meta[name="theme-color"]').forEach(meta => {
      const current = document.querySelector(`meta[name="theme-color"][media="${meta.media}"]`);
      if (current) current.content = meta.content;
    });
    pageSequence += 1;
    if (navigation) {
      const url = new URL(response.url, location.href);
      if (url.pathname + url.search !== location.pathname + location.search) history.pushState(null, '', url.pathname + url.search);
      if (main.querySelector('.nato-screen')) {
        // A repeated review action is immediate, not a page entrance.
        const target = main.querySelector('#answer') || main.querySelector('[data-next] button') || main;
        target.focus({ preventScroll: true });
        // Lessons can change height substantially between prompt and feedback.
        // Start at the phrase identity, rather than leaving it above the viewport.
        main.querySelector('.lesson-stage')?.scrollIntoView({ block: 'start', behavior: 'instant' });
      } else {
        main.classList.add('new-stage');
        window.scrollTo({ top: 0, behavior: 'instant' });
        main.focus({ preventScroll: true });
      }
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
    if (request.sending || suspended || inactive() || !scoped) return;
    if (!navigator.onLine) {
      if (!request.attempted && pending === request) {
        pending = null;
        refreshDepartureWarning();
      }
      announce('You are offline. Your draft is still here.', [], 'Study pauses until you reconnect. Nothing is queued to send automatically.');
      return;
    }
    request.sending = true;
    if (!request.inputLocks) {
      const inputs = [...(request.form?.querySelectorAll('textarea,input:not([type=hidden]),select') || [])];
      if (request.departureDraft && !inputs.includes(request.departureDraft)) inputs.push(request.departureDraft);
      request.inputLocks = inputs.map(input => [input, input.readOnly, input.disabled]);
      request.inputLocks.forEach(([input]) => { if ('readOnly' in input && input.type !== 'file' && input.type !== 'radio' && input.type !== 'checkbox') input.readOnly = true; else input.disabled = true; });
    }
    request.form?.setAttribute('aria-busy', 'true');
    request.form?.querySelectorAll('button').forEach(button => { button.disabled = true; });
    announce('Saving…');
    const controller = new AbortController();
    const flight = { controller, active: true };
    request.flight = flight;
    const generation = epoch;
    const sequence = pageSequence;
    const valid = () => pending === request && request.flight === flight && sequence === pageSequence && current(generation, flight);
    const timer = setTimeout(() => controller.abort(), 20000);
    try {
      request.attempted = true;
      const response = await fetch(request.action, {
        method: 'POST', body: request.data, credentials: 'same-origin', redirect: 'follow',
        headers: { Accept: 'text/html', 'X-Scry-Request': 'page', ...expectedHeaders }, signal: controller.signal
      });
      if (!valid()) return;
      if (response.status === 401 || response.status === 403 || (response.status === 409 && response.headers.get('X-Scry-Scope-Mismatch') === '1') || (response.url && new URL(response.url, location.href).origin !== location.origin)) { conceal(); return; }
      if (!verifyScope(response.headers.get('X-Scry-Scope'))) return;
      if (response.status === 409) {
        if ((response.headers.get('content-type') || '').includes('text/html')) {
          const text = await response.text();
          if (!valid()) return;
          const doc = new DOMParser().parseFromString(text, 'text/html');
          if (!verifyScope(doc.documentElement.dataset.notebookScope)) return;
        }
        pending = null;
        unlock(request);
        refreshDepartureWarning();
        announce('This changed in another tab. Your draft is still here.', [['Open saved progress', () => location.reload()]], 'Reload deliberately to see the current question or saved result.');
        return;
      }
      if ([400, 410, 413, 415, 422].includes(response.status)) {
        // A definite rejection may be an HTML page or a plain transport error.
        const text = await response.text();
        if (!valid()) return;
        let error = text.trim().slice(0, 1000);
        if ((response.headers.get('content-type') || '').includes('text/html')) {
          const doc = new DOMParser().parseFromString(text, 'text/html');
          if (!verifyScope(doc.documentElement.dataset.notebookScope)) return;
          error = doc.querySelector('.notice, .empty-stage .lede')?.textContent?.trim() || '';
        }
        if (!valid()) return;
        pending = null;
        unlock(request);
        refreshDepartureWarning();
        announce(error || 'This could not be saved. Check your draft and try again.', [['Continue editing', hideStatus]]);
        return;
      }
      if (!response.ok) throw new Error('The response did not confirm what was saved.');
      const doc = await safePage(response, valid);
      if (!doc || !valid()) return;
      pending = null;
      install(doc, response);
    } catch {
      if (!valid()) return;
      // Access can redirect to another origin before the Worker returns 401.
      // A read-only manual-redirect probe distinguishes that from response loss.
      if (navigator.onLine) {
        try {
          const probe = await fetch('/session/scope', { credentials: 'same-origin', redirect: 'manual', cache: 'no-store', headers: expectedHeaders, signal: AbortSignal.timeout(5000) });
          if (!valid()) return;
          if ([401, 403, 409].includes(probe.status) || probe.type === 'opaqueredirect' || (probe.url && new URL(probe.url, location.href).origin !== location.origin)) { conceal(); return; }
          if (probe.status === 200) {
            const actual = await readScope(probe);
            if (!valid()) return;
            if (actual !== scope) { conceal(); return; }
          }
        } catch { /* Unavailable is still unknown; retain the frozen attempt. */ }
      }
      if (!valid()) return;
      // A lost response may follow a committed write. Freeze the exact payload.
      announce('The response did not arrive. Your draft is still here.', [
        ['Reconcile this attempt', () => send(request)],
        ['Open saved progress', () => location.reload()]
      ], 'Reconciliation sends the same saved operation and answer. It cannot record a second attempt.');
    } finally {
      clearTimeout(timer);
      if (request.flight === flight) { flight.active = false; request.sending = false; }
    }
  };
  document.addEventListener('submit', event => {
    const form = event.target;
    if (!(form instanceof HTMLFormElement) || form.method.toLowerCase() !== 'post') return;
    if (suspended || inactive() || !scoped) { event.preventDefault(); return; }
    if (typeof fetch !== 'function' || typeof FormData !== 'function') {
      if (unsentAnswer()?.form === form && departureWarningAttached) {
        window.removeEventListener('beforeunload', warnDeparture);
        departureWarningAttached = false;
      }
      return;
    }
    event.preventDefault();
    if (pending) { announce('An earlier response still needs to be reconciled.', [['Reconcile that attempt', () => send(pending)], ['Open saved progress', () => location.reload()]], 'Only one answer can be unresolved at a time.'); return; }
    if (!navigator.onLine) { announce('You are offline. Your draft is still here.', [], 'Study pauses until you reconnect. Nothing is queued to send automatically.'); return; }
    const photo = form.querySelector('input[type=file]')?.files?.[0];
    if (photo && photo.size > 4 * 1024 * 1024) { announce('Choose a photo smaller than 4 MB. Your written draft is still here.'); return; }
    const intent = form.querySelector('textarea[name=intent]');
    if (intent && photo && new TextEncoder().encode(intent.value).length > 1024) { announce('A photo caption can contain up to 1 KB of text. Your draft is still here.'); return; }
    if (intent && new TextEncoder().encode(intent.value).length > 32768) { announce('Your request is over the 32 KB limit. Shorten it and keep the rest separately; your draft is still here.'); return; }
    const answer = form.querySelector('textarea[name=answer],input[name=answer]');
    const answerLimit = Number(form.dataset.answerLimit || 4000);
    if (form.classList.contains('answer-form') && answer && new TextEncoder().encode(answer.value).length > answerLimit) { announce(`Keep your answer under ${answerLimit.toLocaleString('en-US')} bytes. Your draft is still here.`); return; }
    const submitter = event.submitter;
    const draft = unsentAnswer();
    if (draft && draft.form !== form) {
      const poll = pollRequest;
      pollRequest = null;
      poll?.abort();
      if (!window.confirm('Leave this answer? It will not be saved.')) return;
      if (disposed || suspended || inactive() || pending || !form.isConnected) return;
      if (!navigator.onLine) {
        announce('You are offline. Your draft is still here.', [], 'Study pauses until you reconnect. Nothing is queued to send automatically.');
        return;
      }
    }
    const previousPoll = pollRequest;
    pollRequest = null;
    previousPoll?.abort();
    const data = new FormData(form);
    if (submitter?.name) data.append(submitter.name, submitter.value);
    pending = { form, data, action: submitter?.hasAttribute('formaction') ? submitter.formAction : form.action, departureDraft: draft && draft.form !== form ? draft : null };
    refreshDepartureWarning();
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
      if (target.matches('textarea#answer:not([data-multiline])') && event.key === 'Enter' && !event.shiftKey && matchMedia('(pointer: fine)').matches) {
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
  const mayInstallPoll = (doc, preserveDrafts) => {
    const dirty = [...document.forms].filter(form => dirtyForms.has(form));
    const focused = document.activeElement;
    const editing = focused?.matches('textarea,input,select');
    if (!preserveDrafts) return !editing && dirty.length === 0;
    if (editing && !dirtyForms.has(focused.form)) return false;
    const next = doc.querySelector('#main');
    if (!next) return false;
    const forms = [...next.querySelectorAll('form')];
    return dirty.every(form => dirty.filter(candidate => candidate.getAttribute('action') === form.getAttribute('action')).length === 1 && forms.filter(candidate => candidate.getAttribute('action') === form.getAttribute('action')).length === 1);
  };
  const schedulePoll = (delay = 4000) => {
    clearTimeout(pollTimer);
    if (!document.querySelector('#main[data-poll], [data-poll]')) return;
    pollTimer = setTimeout(async () => {
      const currentState = document.querySelector('[data-state]')?.getAttribute('data-state');
      const preserveDrafts = currentState === 'goal';
      if (pending || suspended || !navigator.onLine || inactive() || document.querySelector('form[aria-busy]') || (!preserveDrafts && (document.activeElement?.matches('textarea,input,select') || [...document.forms].some(form => dirtyForms.has(form))))) { schedulePoll(); return; }
      const sequence = pageSequence;
      const generation = epoch;
      const controller = new AbortController();
      pollRequest = controller;
      const valid = () => pollRequest === controller && current(generation) && sequence === pageSequence;
      const timeout = setTimeout(() => controller.abort(), 8000);
      try {
        const response = await fetch(location.href, { credentials: 'same-origin', headers: { Accept: 'text/html', 'X-Scry-Request': 'poll', ...expectedHeaders }, cache: 'no-store', signal: controller.signal });
        const doc = await safePage(response, valid);
        if (doc && response.ok && !pending && valid() && mayInstallPoll(doc, preserveDrafts)) {
          // Never replace an editable page or an unanswered question just to poll.
          if (['concept-home', 'checking', 'checking-reference', 'checking-paused', 'pending', 'preparing', 'preparation-paused', 'preparation-stopped', 'goal', 'caught-up', 'edit'].includes(currentState)) install(doc, response, false, preserveDrafts);
        }
      } catch { /* Poll reads never retry paid work or imply success. */ }
      finally { clearTimeout(timeout); if (pollRequest === controller) pollRequest = null; }
      if (current(generation)) schedulePoll();
    }, delay);
  };
  const recovery = () => {
    let panel = document.getElementById('scope-recovery');
    if (!panel) {
      panel = document.createElement('div');
      panel.id = 'scope-recovery';
      panel.setAttribute('role', 'status');
      const message = document.createElement('p');
      message.textContent = 'Reconnect to open your private notebook.';
      const button = document.createElement('button');
      button.type = 'button';
      button.textContent = 'Try again';
      button.addEventListener('click', () => resume());
      panel.append(message, button);
      document.body.append(panel);
      button.focus();
    }
  };
  const suspend = () => {
    if (disposed) return;
    if (!suspended) {
      savedTitle = document.title;
      savedFocus = document.activeElement;
      savedSelection = savedFocus?.selectionStart == null ? null : [savedFocus.selectionStart, savedFocus.selectionEnd];
    }
    document.title = 'Scry';
    epoch += 1;
    suspended = true;
    validation?.abort();
    clearTimeout(pollTimer);
    pollRequest?.abort();
    if (pending?.flight) {
      pending.flight.active = false;
      pending.flight.controller.abort();
      pending.flight = null;
      pending.sending = false;
    }
    document.body.classList.add('private-suspended');
    const frame = document.querySelector('.page-frame');
    if (frame) frame.inert = true;
  };
  const resume = async () => {
    if (!scoped || disposed || inactive() || !suspended) return;
    const generation = epoch;
    const sequence = pageSequence;
    const controller = new AbortController();
    validation?.abort();
    validation = controller;
    const timeout = setTimeout(() => controller.abort(), 8000);
    try {
      const response = await fetch('/session/scope', { credentials: 'same-origin', redirect: 'manual', cache: 'no-store', headers: expectedHeaders, signal: controller.signal });
      if (validation !== controller || epoch !== generation || pageSequence !== sequence || inactive()) return;
      if ([401, 403, 409].includes(response.status) || response.type === 'opaqueredirect' || (response.url && new URL(response.url, location.href).origin !== location.origin)) { conceal(); return; }
      if (response.status !== 200) { recovery(); return; }
      const actual = await readScope(response);
      if (validation !== controller || epoch !== generation || pageSequence !== sequence || inactive()) return;
      if (actual !== scope) { conceal(); return; }
      suspended = false;
      document.getElementById('scope-recovery')?.remove();
      document.body.classList.remove('private-suspended');
      const frame = document.querySelector('.page-frame');
      if (frame) frame.inert = false;
      document.title = savedTitle;
      if (savedFocus?.isConnected) {
        savedFocus.focus({ preventScroll: true });
        if (savedSelection) {
          try { savedFocus.setSelectionRange(...savedSelection); } catch { /* This control does not support selection. */ }
        }
      }
      savedFocus = null;
      savedSelection = null;
      savedTitle = '';
      if (pending) announce('The response did not arrive. Your draft is still here.', [['Reconcile this attempt', () => send(pending)], ['Open saved progress', () => location.reload()]], 'Reconciliation sends the same saved operation and answer. It cannot record a second attempt.');
      else schedulePoll();
    } catch { if (validation === controller && epoch === generation && pageSequence === sequence && !inactive()) recovery(); }
    finally { clearTimeout(timeout); if (validation === controller) validation = null; }
  };
  const lifecycleChange = () => {
    if (inactive()) suspend();
    else resume();
  };
  document.addEventListener('visibilitychange', lifecycleChange);
  document.addEventListener('prerenderingchange', lifecycleChange);
  window.addEventListener('offline', () => { if (!suspended) announce('You are offline. Your draft is still here.', [], 'Study pauses until you reconnect. Nothing is queued to send automatically.'); });
  window.addEventListener('online', () => {
    if (suspended) { resume(); return; }
    if (pending) announce('You are connected again. Reconcile your saved attempt.', [['Reconcile this attempt', () => send(pending)], ['Open saved progress', () => location.reload()]]);
    else { hideStatus(); schedulePoll(); }
  });
  window.addEventListener('pagehide', conceal);
  window.addEventListener('pageshow', event => { if (event.persisted) location.reload(); });
  window.addEventListener('popstate', () => location.reload());
  if (!scoped) conceal();
  else if (inactive()) suspend();
  else schedulePoll();
  refreshDepartureWarning();
  if (!navigator.onLine && !suspended) announce('You are offline. Study is paused until you reconnect.');
})();
