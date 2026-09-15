/* Presentation only. HTMX sends forms; Go owns occurrence identity, assistance,
   grading, scheduling and every durable receipt. No storage or offline queue. */
(() => {
  'use strict';
  const body = document.body;
  const banner = document.getElementById('request-state');
  const message = document.getElementById('request-message');
  const recovery = document.getElementById('request-recovery');
  const retryButton = document.getElementById('retry-request');
  const recoverLink = document.getElementById('recover-request');
  const cover = document.getElementById('session-cover');
  const retrySessionButton = document.getElementById('retry-session');
  const scopeReopen = document.getElementById('scope-reopen');
  const scopeRetryButton = document.getElementById('scope-retry-request');
  const requests = new WeakMap();
  const submitters = new WeakMap();
  let epoch = 0;
  let pending = null;
  let replayPermit = false;
  let pressed = null;
  let checkingSession = null;
  let scopeTimer = null;
  let scopeDrafts = null;
  let sessionConfirmed = false;
  let readingCovered = false;

  function announce(text, recover = false, canRetry = false) {
    message.textContent = text;
    banner.hidden = false;
    recovery.hidden = !recover;
    retryButton.hidden = !canRetry;
  }

  function removePress() {
    if (pressed) pressed.classList.remove('is-pressed');
    pressed = null;
  }
  document.addEventListener('pointerdown', (event) => {
    const button = event.target.closest('button');
    if (!button || button.disabled || event.button !== 0) return;
    removePress();
    pressed = button;
    button.classList.add('is-pressed');
  }, { passive: true });
  for (const name of ['pointerup', 'pointercancel', 'lostpointercapture']) {
    document.addEventListener(name, removePress, { passive: true });
  }
  document.addEventListener('scroll', removePress, { passive: true, capture: true });
  document.addEventListener('submit', (event) => submitters.set(event.target, event.submitter), true);

  function freeze() {
    const controls = [];
    for (const element of document.querySelectorAll('#main input, #main textarea, #main select, #main button')) {
      if (element.type === 'hidden') continue;
      controls.push([element, element.disabled, element.readOnly]);
      if (element.matches('textarea, input:not([type="checkbox"]):not([type="radio"])')) element.readOnly = true;
      else element.disabled = true;
      element.setAttribute('aria-disabled', 'true');
    }
    document.getElementById('content')?.setAttribute('aria-busy', 'true');
    return controls;
  }

  function restorePreview() {
    document.querySelector('.preview-stage')?.remove();
    const stage = document.querySelector('.review-stage');
    if (stage) stage.hidden = false;
  }

  function release() {
    if (pending) {
      for (const [element, disabled, readOnly] of pending.controls) {
        element.disabled = disabled;
        if (readOnly !== undefined) element.readOnly = readOnly;
        element.removeAttribute('aria-disabled');
        element.classList.remove('is-selected');
      }
    }
    pending = null;
    document.getElementById('content')?.removeAttribute('aria-busy');
    restorePreview();
    banner.hidden = true;
    refreshReadingControls();
  }

  function unknown() {
    if (!pending) return;
    pending.unknown = true;
    restorePreview();
    announce('Save status unknown. Your input is still here. Retry this exact request, or check what the server saved.', true, true);
    refreshReadingControls();
  }

  document.addEventListener('htmx:beforeRequest', (event) => {
    const detail = event.detail;
    const config = detail.requestConfig;
    const mutation = config.verb.toLowerCase() !== 'get';
    const poll = detail.target?.hasAttribute('data-status-poll') || detail.target?.id === 'job-status';
    if (pending && !replayPermit) {
      event.preventDefault();
      if (!poll) announce(pending.unknown ? 'Resolve the unknown save before starting another action.' : 'One request is being saved. Wait for its result before continuing.', pending.unknown, pending.unknown);
      return;
    }
    if (mutation && navigator.onLine === false) {
      event.preventDefault();
      announce('You are offline. Nothing was sent. Reconnect, then choose your action again.');
      return;
    }
    if (!poll) epoch += 1;
    const meta = { epoch, mutation, poll, main: document.getElementById('main'), startedAt: performance.now() };
    requests.set(detail.xhr, meta);
    if (!mutation) return;
    if (replayPermit && pending) {
      pending.unknown = false;
      pending.xhr = detail.xhr;
    } else {
      const form = config.elt.closest('form');
      const button = form ? submitters.get(form) : null;
      const values = {};
      for (const [name, value] of Object.entries(config.parameters)) values[name] = Array.isArray(value) ? value.slice() : value;
      pending = { path: config.path, verb: config.verb, values, controls: freeze(), xhr: detail.xhr, unknown: false };
      if (button?.classList.contains('choice')) button.classList.add('is-selected');
      recoverLink.href = form?.dataset.recoverTo || (config.path === '/add' ? '/library' : config.path.startsWith('/review/') ? '/' : window.location.pathname + window.location.search);
      if (form?.hasAttribute('data-next')) {
        const preview = document.getElementById('next-preview');
        const stage = document.querySelector('.review-stage');
        if (preview && stage && preview.dataset.after === stage.dataset.presentation) {
          stage.after(preview.content.cloneNode(true));
          stage.hidden = true;
        }
      }
      pending.label = button?.dataset.pending || 'Saving your change…';
    }
    replayPermit = false;
    announce(pending.label || 'Retrying the same request…');
    refreshReadingControls();
  });

  // Reject obsolete responses before HTMX can process response headers, a
  // redirect, or a swap. A read never supersedes an unresolved mutation.
  document.addEventListener('htmx:beforeOnLoad', (event) => {
    const meta = requests.get(event.detail.xhr);
    if (!meta) return;
    if (meta.epoch !== epoch || meta.main !== document.getElementById('main')) {
      event.preventDefault();
      return;
    }
    const xhr = event.detail.xhr;
    if (xhr.getResponseHeader('HX-Redirect')) {
      hidePrivate();
      release();
      return;
    }
    if (meta.mutation && xhr.status >= 200 && xhr.status < 300 && xhr.getResponseHeader('HX-Location')) release();
  });

  document.addEventListener('htmx:beforeSwap', (event) => {
    const xhr = event.detail.xhr;
    const meta = requests.get(xhr);
    if (meta && meta.epoch !== epoch) { event.preventDefault(); return; }
    if (xhr.status >= 500) {
      event.preventDefault();
      if (meta?.mutation) unknown();
      else if (!meta?.poll) announce('This page could not be loaded. Your current page is unchanged. Try the link again.');
      return;
    }
    if (xhr.status === 403 && !xhr.getResponseHeader('HX-Redirect')) {
      event.preventDefault();
      release();
      announce('This private form expired or was rejected. Nothing changed in this request. Copy any unsaved text, then reload.', true, false);
      return;
    }
    if (xhr.status === 413) {
      event.preventDefault();
      release();
      announce('The input is too large. Nothing was saved. Use at most 32 KiB; split a long passage into smaller sections.');
      return;
    }
    if (meta?.mutation && xhr.status >= 200 && xhr.status < 300) {
      const response = new DOMParser().parseFromString(xhr.responseText, 'text/html');
      if (response.querySelector('#main[data-view="gate"]') && meta.main?.dataset.scopeKind) {
        // A correction rejected after its reading scope expired must not
        // replace the form with a gate and discard the retained draft.
        event.preventDefault();
        release();
        readingExpired(meta.main);
        return;
      }
    }
    if ([400, 404, 409, 422, 429].includes(xhr.status)) {
      if (meta?.mutation) {
        // A definite rejection is not an unknown save. Keep every in-page
        // draft, including structured corrections and planning choices, instead
        // of replacing it with an error page or a competing tab's new form.
        event.preventDefault();
        const response = new DOMParser().parseFromString(xhr.responseText, 'text/html');
        const reason = response.querySelector('.error-message p')?.textContent;
        release();
        if (readingCovered) readingExpired(meta.main);
        announce(reason || 'This change was not accepted. Your input is still here. Check it, or reload the saved state before trying again.', true, false);
      } else {
        event.detail.shouldSwap = true;
        event.detail.isError = false;
      }
    }
  });

  document.addEventListener('htmx:afterRequest', (event) => {
    const meta = requests.get(event.detail.xhr);
    if (!meta || meta.epoch !== epoch) return;
    const status = event.detail.xhr.status;
    if (meta.mutation && pending?.xhr === event.detail.xhr) {
      if (status === 0 || status >= 500) unknown();
      else release();
    } else if (status === 0 && !meta.poll) {
      announce('Connection lost. Your current page is unchanged; try loading it again.');
    }
  });

  retryButton.addEventListener('click', () => {
    if (!pending?.unknown || navigator.onLine === false) return;
    replayPermit = true;
    // Values are the original encoded form fields, including operation and
    // occurrence IDs. This never edits an answer or invents a new operation.
    const request = htmx.ajax(pending.verb, pending.path, {
      source: body, target: '#main', select: '#main', swap: 'outerHTML', values: pending.values,
    });
    replayPermit = false;
    request?.catch(() => unknown());
  });

  function localTimes(root) {
    for (const element of root.querySelectorAll('time[data-local-time]')) {
      const date = new Date(element.dateTime);
      if (!Number.isFinite(date.getTime())) continue;
      element.textContent = new Intl.DateTimeFormat(undefined, { year: 'numeric', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', timeZoneName: 'short' }).format(date);
    }
  }
  document.addEventListener('htmx:afterSwap', (event) => {
    const meta = requests.get(event.detail.xhr);
    localTimes(document);
    if (meta?.poll) return;
    const main = document.getElementById('main');
    if (!main) return;
    restoreScopeDrafts(main);
    armReadingScope(main, performance.now() - (meta?.startedAt || 0));
    if (body.classList.contains('private-hidden')) checkSession();
    document.title = `${main.dataset.title} · Scry`;
    // Focus is a presentation change, not an advance. Keep long results in
    // natural scroll flow and never focus a speculative answer control.
    const focus = main.querySelector('[data-focus]');
    focus?.focus({ preventScroll: true });
    if (focus && !pending?.unknown) focus.scrollIntoView({ block: 'start', behavior: 'instant' });
  });
  localTimes(document);
  armReadingScope(document.getElementById('main'), performance.now());

  function captureScopeDrafts(main) {
    if (pending || scopeDrafts) return;
    const forms = [];
    for (const form of main.querySelectorAll('form')) {
      const controls = [...form.querySelectorAll('input, textarea, select')];
      const modified = controls.some(control => control.name && control.type !== 'hidden' && (control.matches('select') ? [...control.options].some(option => option.selected !== option.defaultSelected) : control.type === 'checkbox' || control.type === 'radio' ? control.checked !== control.defaultChecked : control.value !== control.defaultValue));
      if (!modified) continue;
      const draft = form.cloneNode(true);
      const copied = draft.querySelectorAll('input, textarea, select');
      controls.forEach((control, index) => {
        copied[index].value = control.value;
        if (control.type === 'checkbox' || control.type === 'radio') copied[index].checked = control.checked;
      });
      forms.push({ action: form.getAttribute('action'), draft });
    }
    scopeDrafts = { kind: main.dataset.scopeKind, id: main.dataset.scopeId, view: main.dataset.view, forms };
  }

  function restoreScopeDrafts(main) {
    if (!scopeDrafts || !main.dataset.scopeKind) return;
    const drafts = scopeDrafts;
    scopeDrafts = null;
    if (drafts.kind !== main.dataset.scopeKind || drafts.id !== main.dataset.scopeId || drafts.view !== main.dataset.view) return;
    for (const saved of drafts.forms) {
      const form = [...main.querySelectorAll('form')].find(item => item.getAttribute('action') === saved.action);
      if (!form) continue;
      // Preserve the whole draft, including repeated coverage rows, selected
      // relationship identity and expected versions. Only authorization and
      // the new unsent operation token come from the freshly inspected form.
      for (const name of ['csrf', 'operation_id']) {
        const fresh = form.elements.namedItem(name);
        const retained = saved.draft.elements.namedItem(name);
        if (fresh && retained) retained.value = fresh.value;
      }
      form.replaceWith(saved.draft);
      const details = saved.draft.closest('details');
      if (details) details.open = true;
      htmx.process(saved.draft);
    }
  }

  function readingExpired(main, tokens = null) {
    if (!main || main !== document.getElementById('main')) return;
    captureScopeDrafts(main);
    hidePrivate();
    readingCovered = true;
    retrySessionButton.hidden = true;
    refreshReadingControls();
    scopeReopen.elements.csrf.value = tokens?.csrf || main.dataset.scopeCsrf;
    scopeReopen.elements.operation_id.value = tokens?.operation_id || main.dataset.scopeOperation;
    scopeReopen.elements.kind.value = main.dataset.scopeKind;
    scopeReopen.elements.id.value = main.dataset.scopeId;
    scopeReopen.elements.return_to.value = main.dataset.scopeReturn;
    scopeReopen.dataset.recoverTo = main.dataset.scopeReturn;
  }

  function refreshReadingControls() {
    if (!readingCovered) return;
    cover.querySelector('p').textContent = pending ? 'Reading is paused and a save still needs confirmation. Resolve that request before opening more content; your input is retained.' : 'Continue reading when you are ready. Opening again records inspection and marks related unanswered targets helped, not as a failed review. Your unsaved edits are retained.';
    scopeReopen.hidden = Boolean(pending);
    scopeRetryButton.hidden = !pending?.unknown;
  }

  function armReadingScope(main, elapsed = 0, access = null) {
    clearTimeout(scopeTimer);
    if (!main?.dataset.scopeKind) return true;
    const expires = Number(access?.expires_at ?? main.dataset.scopeExpires);
    const checked = Number(access?.checked_at ?? main.dataset.scopeChecked);
    // An active committed presentation stays readable until deliberate
    // retirement. Periodic read-only checks discover a different tab's Next.
    if (expires === 0 && main.dataset.scopeKind === 'presentation') return true;
    const remaining = expires - checked - Math.max(0, elapsed) - 100;
    if (!Number.isFinite(remaining) || expires <= 0 || remaining <= 0) {
      readingExpired(main);
      return false;
    }
    scopeTimer = setTimeout(() => readingExpired(main), remaining);
    return true;
  }

  function hidePrivate() {
    body.classList.add('private-hidden');
    sessionConfirmed = false;
    cover.hidden = false;
  }
  async function checkSession(options = {}) {
    if (checkingSession) return checkingSession;
    const main = document.getElementById('main');
    const startedAt = performance.now();
    if (!options.quiet) hidePrivate();
    readingCovered = false;
    retrySessionButton.hidden = true;
    scopeReopen.hidden = true;
    scopeRetryButton.hidden = true;
    cover.querySelector('p').textContent = 'Checking private access…';
    checkingSession = (async () => {
      try {
        const query = new URLSearchParams();
        if (main?.dataset.scopeKind) {
          query.set('kind', main.dataset.scopeKind);
          query.set('id', main.dataset.scopeId);
        }
        const response = await fetch(`/session?${query}`, { credentials: 'same-origin', cache: 'no-store', redirect: 'error', headers: { Accept: 'application/json' }, signal: AbortSignal.timeout(10000) });
        if (!response.ok) throw new Error('private access unavailable');
        const result = await response.json();
        if (result.authenticated !== true) throw new Error('private access unavailable');
        if (main !== document.getElementById('main')) return;
        sessionConfirmed = true;
        if (main?.dataset.scopeKind && (!result.access?.allowed || result.access?.requires_assistance)) {
          readingExpired(main, result);
          sessionConfirmed = true;
          return;
        }
        if (!armReadingScope(main, performance.now() - startedAt, result.access)) {
          sessionConfirmed = true;
          return;
        }
        if (document.visibilityState !== 'hidden') {
          readingCovered = false;
          body.classList.remove('private-hidden');
          cover.hidden = true;
        }
      } catch {
        readingCovered = false;
        hidePrivate();
        epoch += 1;
        if (pending) unknown();
        cover.querySelector('p').textContent = 'Private access could not be confirmed. Your unsaved input is still here. Reconnect or sign in with the approved exe account, then retry private access.';
        retrySessionButton.hidden = false;
      } finally {
        checkingSession = null;
        if (main !== document.getElementById('main') && body.classList.contains('private-hidden')) checkSession();
      }
    })();
    return checkingSession;
  }
  setInterval(() => {
    if (document.visibilityState === 'visible' && !body.classList.contains('private-hidden') && document.getElementById('main')?.dataset.scopeKind) checkSession({ quiet: true });
  }, 30000);
  scopeRetryButton.addEventListener('click', async () => {
    await checkSession();
    if (sessionConfirmed) retryButton.click();
  });
  retrySessionButton.addEventListener('click', checkSession);
  window.addEventListener('pagehide', hidePrivate);
  window.addEventListener('blur', hidePrivate);
  window.addEventListener('pageshow', (event) => { if (event.persisted) { hidePrivate(); window.location.reload(); } });
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') hidePrivate();
    else if (body.classList.contains('private-hidden')) checkSession();
  });
  window.addEventListener('focus', () => { if (body.classList.contains('private-hidden')) checkSession(); });
  window.addEventListener('offline', () => {
    if (pending) unknown();
    else announce('You are offline. Review pauses here; no answers are queued.');
  });
  window.addEventListener('online', () => {
    if (body.classList.contains('private-hidden')) checkSession();
    else if (!pending) banner.hidden = true;
  });
})();
