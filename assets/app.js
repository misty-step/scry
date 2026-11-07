/* Presentation and durable-response reconciliation only. No offline mutation queue. */
(() => {
  'use strict';
  document.addEventListener('pointerdown', event => {
    const details = event.target.closest?.('#nato-progress > summary')?.parentElement;
    if (details) details.classList.toggle('animate-open', !details.open);
  }, { passive: true });
  document.addEventListener('keydown', () => document.querySelector('#nato-progress')?.classList.remove('animate-open'));
  const acknowledgedDrafts = new WeakMap();
  const enableDraftSave = () => {
    if (typeof fetch !== 'function' || typeof FormData !== 'function') return;
    document.querySelectorAll('[data-checkpoint-save], [data-checkpoint-custody]').forEach(node => { node.hidden = false; });
    document.querySelectorAll('[data-checkpoint-discard]').forEach(node => { node.hidden = node.dataset.checkpointPresent !== 'true'; });
    document.querySelectorAll('form.answer-form').forEach(form => {
      const input = form.querySelector('textarea[name=answer],input[type=text][name=answer]');
      const marker = form.querySelector('[data-checkpoint-generation]');
      const discard = form.querySelector('[data-checkpoint-discard]');
      if (input && marker && discard?.dataset.checkpointPresent === 'true') acknowledgedDrafts.set(form, {value: input.value, generation: marker.dataset.generation});
    });
  };
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
  enableDraftSave();
  let pending = null;
  const reconcileLabel = request => request.checkpoint === 'discard' ? 'Reconcile this discard' : request.checkpoint ? 'Reconcile this save' : 'Reconcile this attempt';
  const reconcileDetail = request => request.checkpoint === 'discard' ? 'Reconciliation sends the same discard operation. It does not send or grade your answer.' : request.checkpoint ? 'Reconciliation sends the same saved operation and text. Saving does not grade your answer.' : 'Reconciliation sends the same saved operation and answer. It cannot record a second attempt.';
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
  let checkpointAcknowledged = false;
  let autoTimer = null;
  let autoBlocked = false;
  let autoEdited = false;
  let answerUnsaved = false;
  let composing = false;
  const cancelAuto = () => { clearTimeout(autoTimer); autoTimer = null; };
  const dirtyForms = new WeakSet();
  const unsentAnswer = () => [...document.querySelectorAll('form.answer-form textarea[name=answer], form.answer-form input[type=text][name=answer]')].find(input => !input.disabled && input.value.length > 0);
  document.addEventListener('input', event => {
    if (event.target.form) dirtyForms.add(event.target.form);
    if (checkpointAcknowledged) hideStatus();
    const input = event.target;
    if (input.form?.classList.contains('answer-form') && input.matches?.('textarea[name=answer], input[type=text][name=answer]')) {
      autoEdited = true;
      answerUnsaved = true;
      if (!pending && autoBlocked === 'changed') announce('The question or saved draft changed. Your text is not saved.', [['Open saved progress', () => location.reload()]]);
      else if (!pending && autoBlocked === 'limit') announce('Automatic saving has reached its limit. Use Save draft.', [], 'Your current text is not saved yet.');
      else if (!pending && autoBlocked === 'unavailable') announce('Automatic saving is unavailable. Use Save draft.', [], 'Your current text is not saved yet.');
      else if (!pending && autoBlocked === 'capacity') announce('Automatic saving is paused. Use Save draft.', [], 'Your current text is not saved yet.');
      else if (!pending) announce('Changes not yet saved.');
      else if (pending.automatic && pending.sending) announce(pending.followupAllowed ? 'Saving earlier text. Newer changes are not yet saved.' : 'Checking the earlier save. Newer changes are not saved.');
      else if (pending.automatic) announce('An earlier save needs reconciliation. Newer changes are not saved.', [[reconcileLabel(pending), () => send(pending)], ['Open saved progress', () => location.reload()]], reconcileDetail(pending));
      if (!event.isComposing && !composing) scheduleAuto(input);
    }
  });
  document.addEventListener('compositionstart', () => { composing = true; cancelAuto(); });
  document.addEventListener('compositionend', event => { composing = false; if (autoEdited && event.target.form?.classList.contains('answer-form')) scheduleAuto(event.target); });
  document.addEventListener('change', event => { if (event.target.form) dirtyForms.add(event.target.form); });
  const status = () => document.getElementById('request-status');
  const hideStatus = () => { checkpointAcknowledged = false; const box = status(); if (box) { box.hidden = true; box.replaceChildren(); } };
  const announce = (message, actions = [], detail = '') => {
    checkpointAcknowledged = false;
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
    epoch += 1;
    suspended = true;
    validation?.abort();
    clearTimeout(pollTimer);
    cancelAuto();
    autoEdited = false;
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
    if (!preserveDrafts) { cancelAuto(); autoEdited = false; answerUnsaved = false; autoBlocked = false; }
    enablePickerReturn();
    enableDraftSave();
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
  const scheduleAuto = input => {
    cancelAuto();
    if (autoBlocked || disposed || suspended || inactive() || !navigator.onLine || composing || !input?.form?.isConnected) return;
    const acknowledged = acknowledgedDrafts.get(input.form);
    if (!pending && acknowledged && input.value === acknowledged.value && input.form.querySelector('[data-checkpoint-generation]')?.dataset.generation === acknowledged.generation) { autoEdited = false; answerUnsaved = false; announce('Saved.'); checkpointAcknowledged = true; return; }
    if (typeof crypto === 'undefined' || typeof crypto.randomUUID !== 'function') { autoBlocked = 'unavailable'; announce('Automatic saving is unavailable. Use Save draft.', [], 'Your current text is not saved yet.'); return; }
    if (new TextEncoder().encode(input.value).length > Number(input.form.dataset.answerLimit || 4000)) return;
    autoTimer = setTimeout(() => {
      autoTimer = null;
      if (!autoEdited || autoBlocked || disposed || suspended || inactive() || !navigator.onLine || composing || !input.form?.isConnected || pending) return;
      const form = input.form;
      const generationControl = form.querySelector('[data-checkpoint-generation]');
      const expectedGeneration = generationControl?.dataset.generation;
      if (!generationControl || !/^(0|[1-9][0-9]{0,19})$/.test(expectedGeneration || '')) {
        autoBlocked = 'changed';
        announce('The question or saved draft changed. Your text is not saved.', [['Open saved progress', () => location.reload()]]);
        return;
      }
      try {
        const snapshot = input.value;
        const data = new FormData(form);
        if (disposed || suspended || inactive() || !navigator.onLine || !form.isConnected || pending) return;
        if (form.querySelector('[data-checkpoint-generation]') !== generationControl || generationControl.dataset.generation !== expectedGeneration) { autoBlocked = 'changed'; announce('The question or saved draft changed. Your text is not saved.', [['Open saved progress', () => location.reload()]]); return; }
        if (input.value !== snapshot || data.get('answer') !== snapshot) { announce('Changes are not saved. Use Save draft or keep typing.'); return; }
        data.set('operation_id', `autosave-${crypto.randomUUID()}`);
        data.set('generation', expectedGeneration);
        autoEdited = false;
        const poll = pollRequest; pollRequest = null; poll?.abort();
        pending = { form, data, checkpoint: 'save', automatic: true, followupAllowed: true, snapshot, input, action: new URL('/practice/draft', location.href).href };
        send(pending);
      } catch {
        autoBlocked = 'unavailable';
        announce('Automatic saving is unavailable. Use Save draft.', [], 'Your current text is not saved yet.');
      }
    }, 5000);
  };
  const readCheckpoint = async (response, request) => {
    if (response.status !== 200 || !(response.headers.get('content-type') || '').includes('application/json') || !response.body) throw new Error('The checkpoint response was incomplete.');
    const reader = response.body.getReader(); const chunks = []; let length = 0;
    try {
      while (true) { const {done,value} = await reader.read(); if (done) break; length += value.byteLength; if (length > 2048) { reader.cancel().catch(() => {}); throw new Error('The checkpoint response was too large.'); } chunks.push(value); }
    } finally { reader.releaseLock(); }
    const bytes = new Uint8Array(length); let offset = 0; chunks.forEach(chunk => { bytes.set(chunk, offset); offset += chunk.byteLength; });
    const data = JSON.parse(new TextDecoder('utf-8', {fatal:true}).decode(bytes));
    const previous = request.data.get('generation');
    if (typeof previous !== 'string' || !/^(0|[1-9][0-9]{0,19})$/.test(previous) || BigInt(previous) >= 18446744073709551615n) throw new Error('The checkpoint generation was invalid.');
    if (!data || Object.keys(data).sort().join(',') !== 'current,generation,operation_id,prompt_id,remaining,version' || data.version !== 1 || data.operation_id !== request.data.get('operation_id') || data.prompt_id !== request.data.get('prompt_id') || !/^[a-f0-9]{32}$/.test(data.prompt_id || '') || typeof data.generation !== 'string' || !/^[1-9][0-9]{0,19}$/.test(data.generation) || data.generation !== (BigInt(previous) + 1n).toString() || typeof data.current !== 'boolean' || !Number.isInteger(data.remaining) || data.remaining < 0 || data.remaining > 512) throw new Error('The checkpoint response was invalid.');
    return data;
  };
  const send = async request => {
    if (request.sending || suspended || inactive() || !scoped) return;
    if (!navigator.onLine) {
      if (!request.attempted && pending === request) {
        pending = null;
      }
      announce('You are offline. Your draft is still here.', [], 'Study pauses until you reconnect. Nothing is queued to send automatically.');
      return;
    }
    request.sending = true;
    if (!request.automatic && !request.inputLocks) {
      const inputs = [...(request.form?.querySelectorAll('textarea,input:not([type=hidden]),select') || [])];
      if (request.departureDraft && !inputs.includes(request.departureDraft)) inputs.push(request.departureDraft);
      request.inputLocks = inputs.map(input => [input, input.readOnly, input.disabled]);
      request.inputLocks.forEach(([input]) => { if ('readOnly' in input && input.type !== 'file' && input.type !== 'radio' && input.type !== 'checkbox') input.readOnly = true; else input.disabled = true; });
    }
    request.form?.setAttribute('aria-busy', 'true');
    request.form?.querySelectorAll('button').forEach(button => { button.disabled = true; });
    announce(request.automatic && request.input.value !== request.snapshot ? 'Saving earlier text. Newer changes are not yet saved.' : 'Saving…');
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
        method: 'POST', body: request.data, credentials: 'same-origin', redirect: request.automatic ? 'manual' : 'follow',
        headers: { Accept: request.automatic ? 'application/json' : 'text/html', 'X-Scry-Request': 'page', ...(request.automatic ? { 'X-Scry-Checkpoint-Mode': 'auto-v1' } : {}), ...expectedHeaders }, signal: controller.signal
      });
      if (!valid()) return;
      if (request.automatic && response.type === 'opaqueredirect') throw new Error('The automatic save was redirected.');
      if (response.status === 401 || response.status === 403 || (response.status === 409 && response.headers.get('X-Scry-Scope-Mismatch') === '1') || (response.url && new URL(response.url, location.href).origin !== location.origin)) { conceal(); return; }
      if (!verifyScope(response.headers.get('X-Scry-Scope'))) return;
      if (request.automatic && response.status === 429) {
        pending = null; unlock(request); autoBlocked = 'limit'; cancelAuto();
        announce('Automatic saving has reached its limit. Use Save draft.', [], 'Your current text is not saved yet.');
        return;
      }
      if (request.automatic && response.status === 200) {
        const receipt = await readCheckpoint(response, request);
        if (!valid()) return;
        if (!receipt.current) {
          pending = null; unlock(request);
          autoBlocked = 'changed'; cancelAuto();
          announce('The question or saved draft changed. Your text is not saved.', [['Open saved progress', () => location.reload()]]);
          return;
        }
        const generationControl = request.form.querySelector('[data-checkpoint-generation]');
        if (!generationControl) throw new Error('The checkpoint generation was unavailable.');
        if (generationControl.dataset.generation !== request.data.get('generation')) {
          pending = null; unlock(request); autoBlocked = 'changed'; cancelAuto();
          announce('The question or saved draft changed. Your text is not saved.', [['Open saved progress', () => location.reload()]]);
          return;
        }
        if (!request.followupAllowed) { cancelAuto(); autoEdited = false; }
        pending = null; unlock(request);
        generationControl.dataset.generation = receipt.generation;
        acknowledgedDrafts.set(request.form, {value: request.snapshot, generation: receipt.generation});
        const discardControl = request.form.querySelector('[data-checkpoint-discard]');
        if (discardControl) { discardControl.dataset.checkpointPresent = 'true'; discardControl.hidden = false; }
        if (receipt.remaining === 0) autoBlocked = 'limit';
        if (request.input.value === request.snapshot) {
          cancelAuto(); autoEdited = false; answerUnsaved = false;
          announce(autoBlocked ? 'Saved. Automatic saving has reached its limit; use Save draft next time.' : 'Saved.');
          checkpointAcknowledged = true;
        } else {
          announce(autoBlocked ? 'Earlier text saved. Newer changes are not saved; use Save draft.' : 'Earlier text saved. Newer changes are not yet saved.');
          if (request.followupAllowed && autoEdited && !autoBlocked) scheduleAuto(request.input);
        }
        return;
      }
      if (response.status === 409) {
        if ((response.headers.get('content-type') || '').includes('text/html')) {
          const text = await response.text();
          if (!valid()) return;
          const doc = new DOMParser().parseFromString(text, 'text/html');
          if (!verifyScope(doc.documentElement.dataset.notebookScope)) return;
        }
        pending = null;
        unlock(request);
        if (request.automatic) { autoBlocked = 'changed'; cancelAuto(); }
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
        if (request.automatic && response.status === 413) { autoBlocked = 'capacity'; autoEdited = false; cancelAuto(); announce('Automatic saving is paused. Use Save draft.', [], 'Your current text is not saved yet.'); return; }
        announce(error || 'This could not be saved. Check your draft and try again.', [['Continue editing', hideStatus]]);
        return;
      }
      if (request.automatic) throw new Error('The automatic save did not return its expected acknowledgment.');
      if (!response.ok) throw new Error('The response did not confirm what was saved.');
      const doc = await safePage(response, valid);
      if (!doc || !valid()) return;
      pending = null;
      install(doc, response);
      if (request.checkpoint) { announce(request.checkpoint === 'discard' ? 'Discard confirmed. The latest notebook state is shown.' : 'Save confirmed. The latest notebook state is shown.'); checkpointAcknowledged = true; }
    } catch {
      if (!valid()) return;
      if (request.automatic) { request.followupAllowed = false; autoEdited = false; cancelAuto(); }
      // Access can redirect to another origin before the Worker returns 401.
      // A read-only manual-redirect probe distinguishes that from response loss.
      if (navigator.onLine) {
        try {
          const probe = await fetch('/session/scope', { credentials: 'same-origin', redirect: 'manual', cache: 'no-store', headers: expectedHeaders, signal: AbortSignal.timeout(5000) });
          if (!valid()) return;
          if (request.automatic && probe.type === 'opaqueredirect') { suspend(); recovery(); return; }
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
        [reconcileLabel(request), () => send(request)],
        ['Open saved progress', () => location.reload()]
      ], request.automatic && request.input.value !== request.snapshot ? 'Reconciliation sends the earlier snapshot. Your newer changes are not saved.' : reconcileDetail(request));
    } finally {
      clearTimeout(timer);
      if (request.flight === flight) { flight.active = false; request.sending = false; }
    }
  };
  document.addEventListener('submit', event => {
    const form = event.target;
    if (!(form instanceof HTMLFormElement) || form.method.toLowerCase() !== 'post') return;
    if (suspended || inactive() || !scoped) { event.preventDefault(); return; }
    if (typeof fetch !== 'function' || typeof FormData !== 'function') return;
    event.preventDefault();
    cancelAuto();
    if (pending) { if (pending.sending) announce('Saving…', [], 'Wait for the current response before continuing.'); else announce('An earlier response still needs to be reconciled.', [[pending.checkpoint ? reconcileLabel(pending) : 'Reconcile that attempt', () => send(pending)], ['Open saved progress', () => location.reload()]], 'Only one operation can be unresolved at a time.'); return; }
    if (!navigator.onLine) { announce('You are offline. Your draft is still here.', [], 'Study pauses until you reconnect. Nothing is queued to send automatically.'); return; }
    const photo = form.querySelector('input[type=file]')?.files?.[0];
    if (photo && photo.size > 4 * 1024 * 1024) { announce('Choose a photo smaller than 4 MB. Your written draft is still here.'); return; }
    const intent = form.querySelector('textarea[name=intent]');
    if (intent && photo && new TextEncoder().encode(intent.value).length > 1024) { announce('A photo caption can contain up to 1 KB of text. Your draft is still here.'); return; }
    if (intent && new TextEncoder().encode(intent.value).length > 32768) { announce('Your request is over the 32 KB limit. Shorten it and keep the rest separately; your draft is still here.'); return; }
    const submitter = event.submitter;
    const discarding = !!submitter?.hasAttribute('data-checkpoint-discard');
    const answer = form.querySelector('textarea[name=answer],input[name=answer]');
    const answerLimit = Number(form.dataset.answerLimit || 4000);
    if (!discarding && form.classList.contains('answer-form') && answer && new TextEncoder().encode(answer.value).length > answerLimit) { announce(`Keep your answer under ${answerLimit.toLocaleString('en-US')} bytes. Your draft is still here.`); return; }
    if (discarding) {
      const poll = pollRequest; pollRequest = null; poll?.abort();
      if (!window.confirm('Discard the saved draft and the text currently in this field?')) return;
      if (disposed || suspended || inactive() || pending || !form.isConnected) return;
      if (!navigator.onLine) { announce('You are offline. Your draft is still here.', [], 'Study pauses until you reconnect. Nothing is queued to send automatically.'); return; }
    }
    const draft = unsentAnswer();
    if (draft && draft.form !== form) {
      const poll = pollRequest;
      pollRequest = null;
      poll?.abort();
      if (!window.confirm('Leave this answer? It will be discarded.')) return;
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
    if (discarding) data.delete('answer');
    if (discarding || submitter?.hasAttribute('data-checkpoint-save')) {
      data.append('generation', form.querySelector('[data-checkpoint-generation]')?.dataset.generation || '0');
    }
    pending = { form, data, checkpoint: discarding ? 'discard' : submitter?.hasAttribute('data-checkpoint-save') ? 'save' : null, action: submitter?.hasAttribute('formaction') ? submitter.formAction : form.action, departureDraft: draft && draft.form !== form ? draft : null };
    send(pending);
  });
  document.addEventListener('click', event => {
    if (!pending) return;
    const link = event.target.closest?.('a');
    if (!link || link.origin !== location.origin) return;
    event.preventDefault();
    if (pending.sending) announce('Saving…', [], 'Wait for the current response before continuing.');
    else announce('First, reconcile the response that did not arrive.', [[reconcileLabel(pending), () => send(pending)], ['Open saved progress', () => location.reload()]], 'Your exact answer is kept on this page until you choose what to do.');
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
    cancelAuto();
    autoEdited = false;
    pollRequest?.abort();
    if (pending?.automatic) pending.followupAllowed = false;
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
      if (pending?.automatic && response.type === 'opaqueredirect') { recovery(); return; }
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
      if (pending) announce('The response did not arrive. Your draft is still here.', [[reconcileLabel(pending), () => send(pending)], ['Open saved progress', () => location.reload()]], reconcileDetail(pending));
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
  window.addEventListener('offline', () => { cancelAuto(); autoEdited = false; if (pending?.automatic) pending.followupAllowed = false; if (!suspended) announce('You are offline. Your draft is still here.', [], 'Study pauses until you reconnect. Nothing is queued to send automatically.'); });
  window.addEventListener('online', () => {
    if (suspended) { resume(); return; }
    if (pending?.sending) announce('Saving…', [], 'Wait for the current response before continuing.');
    else if (pending) announce(pending.automatic && pending.input.value !== pending.snapshot ? 'Reconcile the earlier save. Your newer changes are not saved.' : 'You are connected again. Reconcile your saved operation.', [[reconcileLabel(pending), () => send(pending)], ['Open saved progress', () => location.reload()]]);
    else { if (answerUnsaved && autoBlocked === 'changed') announce('The question or saved draft changed. Your text is not saved.', [['Open saved progress', () => location.reload()]]); else if (answerUnsaved && autoBlocked === 'limit') announce('Automatic saving has reached its limit. Use Save draft.', [], 'Your current text is not saved yet.'); else if (answerUnsaved && autoBlocked === 'unavailable') announce('Automatic saving is unavailable. Use Save draft.', [], 'Your current text is not saved yet.'); else if (answerUnsaved && autoBlocked === 'capacity') announce('Automatic saving is paused. Use Save draft.', [], 'Your current text is not saved yet.'); else if (answerUnsaved) announce('Changes are not saved. Use Save draft or keep typing.'); else hideStatus(); schedulePoll(); }
  });
  window.addEventListener('pagehide', conceal);
  window.addEventListener('pageshow', event => { if (event.persisted) location.reload(); });
  window.addEventListener('popstate', () => location.reload());
  if (!scoped) conceal();
  else if (inactive()) suspend();
  else schedulePoll();
  if (!navigator.onLine && !suspended) announce('You are offline. Study is paused until you reconnect.');
})();
