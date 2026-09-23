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
  const requests = new WeakMap();
  const submitters = new WeakMap();
  let epoch = 0;
  let pending = null;
  let replayPermit = false;
  let pressed = null;
  let checkingSession = null;

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
  }

  function unknown() {
    if (!pending) return;
    pending.unknown = true;
    restorePreview();
    announce('Save status unknown. Your input is still here. Retry this exact request, or check what the server saved.', true, true);
  }

  document.addEventListener('htmx:beforeRequest', (event) => {
    const detail = event.detail;
    const config = detail.requestConfig;
    const mutation = config.verb.toLowerCase() !== 'get';
    const poll = detail.target?.id === 'job-status';
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
    const meta = { epoch, mutation, poll, main: document.getElementById('main') };
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
      const multipart = form?.enctype === 'multipart/form-data';
      const payload = multipart ? new FormData(form) : null;
      pending = { path: config.path, verb: config.verb, values, payload, controls: freeze(), xhr: detail.xhr, unknown: false };
      const outgoing = document.querySelector('.review-stage');
      if (outgoing && !form?.hasAttribute('data-next')) {
        outgoing.classList.add('is-outgoing');
        setTimeout(() => outgoing.classList.remove('is-outgoing'), 150);
      }
      if (button?.classList.contains('choice')) button.classList.add('is-selected');
      recoverLink.href = config.path === '/add' ? '/map' : config.path.startsWith('/review/') ? '/' : window.location.pathname;
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
      announce('The input is too large. Nothing was saved. Shorten the text or choose a smaller photo.');
      return;
    }
    if ([400, 404, 409, 422, 429].includes(xhr.status)) {
      event.detail.shouldSwap = true;
      event.detail.isError = false;
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

  retryButton.addEventListener('click', async () => {
    if (!pending?.unknown || navigator.onLine === false) return;
    if (pending.payload) {
      const attempt = pending;
      attempt.unknown = false;
      announce('Retrying the same request…');
      try {
        const response = await fetch(attempt.path, { method: attempt.verb, body: attempt.payload, credentials: 'same-origin', cache: 'no-store' });
        if (response.status >= 500) { unknown(); return; }
        if (response.ok) {
          if (new URL(response.url).origin !== location.origin) throw new Error('Unexpected destination');
          location.assign(response.url);
          return;
        }
        release();
        location.reload();
      } catch { unknown(); }
      return;
    }
    replayPermit = true;
    // Values include the original operation and occurrence IDs.
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
    document.title = `${main.dataset.title} · Scry`;
    // Focus is a presentation change, not an advance. Keep long results in
    // natural scroll flow and never focus a speculative answer control.
    const focus = main.querySelector('[data-focus]');
    focus?.focus({ preventScroll: true });
    if (focus && !pending?.unknown) focus.scrollIntoView({ block: 'start', behavior: 'instant' });
  });
  localTimes(document);

  function hidePrivate() {
    body.classList.add('private-hidden');
    cover.hidden = false;
  }
  async function checkSession() {
    if (checkingSession) return checkingSession;
    hidePrivate();
    retrySessionButton.hidden = true;
    cover.querySelector('p').textContent = 'Checking private access…';
    checkingSession = (async () => {
      try {
        const response = await fetch('/session', { credentials: 'same-origin', cache: 'no-store', redirect: 'error', headers: { Accept: 'application/json' }, signal: AbortSignal.timeout(10000) });
        if (!response.ok || (await response.json()).authenticated !== true) throw new Error('private access unavailable');
        if (document.visibilityState !== 'hidden') {
          body.classList.remove('private-hidden');
          cover.hidden = true;
        }
      } catch {
        epoch += 1;
        if (pending) unknown();
        cover.querySelector('p').textContent = 'Private access could not be confirmed. Your unsaved input is still here. Reconnect or sign in with the approved exe account, then retry private access.';
        retrySessionButton.hidden = false;
      } finally {
        checkingSession = null;
      }
    })();
    return checkingSession;
  }
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
  function enhance(root) {
    const recall = root.querySelector('#recall-answer');
    const recallButton = root.querySelector('.recall-submit');
    const updateLabel = () => { if (recallButton) recallButton.textContent = recall?.value.trim() ? 'Check' : 'Show me'; };
    if (recall) {
      recall.addEventListener('input', updateLabel);
      updateLabel();
      recall.addEventListener('keydown', (event) => {
        if (event.key === 'Enter' && !event.shiftKey && !event.isComposing && matchMedia('(pointer: fine)').matches) {
          event.preventDefault();
          recall.form.requestSubmit(recallButton);
        }
      });
    }
    const capture = root.querySelector('[data-capture]');
    if (capture) {
      const text = capture.querySelector('#capture-text');
      const modes = [...capture.querySelectorAll('input[name="mode"]')];
      let explicit = modes.some((mode) => mode.checked);
      // Only private modes may be chosen for the learner. Topic and Link send
      // material to web research, so they are selected by the learner alone.
      const choose = (value) => { if (!explicit) modes.find((mode) => mode.value === value).checked = true; };
      modes.forEach((mode) => mode.addEventListener('change', () => { explicit = true; }));
      text.addEventListener('paste', (event) => {
        const pasted = event.clipboardData?.getData('text')?.trim() || '';
        if (pasted && !/^https?:\/\/\S+$/i.test(pasted)) choose('text');
      });
      const photo = capture.querySelector('#capture-photo');
      photo?.addEventListener('change', async () => {
        if (photo.files.length !== 1) return;
        if (!explicit) choose('photo');
        const source = photo.files[0];
        if (!source.type.startsWith('image/')) return;
        try {
          const image = await createImageBitmap(source);
          const scale = Math.min(1, 1600 / Math.max(image.width, image.height));
          if (scale === 1 && source.size <= 4 * 1024 * 1024) { image.close(); return; }
          const canvas = document.createElement('canvas');
          canvas.width = Math.round(image.width * scale);
          canvas.height = Math.round(image.height * scale);
          canvas.getContext('2d').drawImage(image, 0, 0, canvas.width, canvas.height);
          image.close();
          const blob = await new Promise((resolve) => canvas.toBlob(resolve, 'image/jpeg', .82));
          if (!blob) return;
          const transfer = new DataTransfer();
          transfer.items.add(new File([blob], source.name.replace(/\.[^.]+$/, '') + '.jpg', { type: 'image/jpeg' }));
          photo.files = transfer.files;
        } catch { /* The original file remains selected for server validation. */ }
      });
    }
    const Recognition = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (Recognition) for (const button of root.querySelectorAll('[data-voice-target]')) {
      button.hidden = false;
      button.addEventListener('click', () => {
        const target = root.querySelector('#' + button.dataset.voiceTarget);
        if (!target) return;
        const speech = new Recognition();
        speech.lang = document.documentElement.lang || 'en';
        speech.onresult = (event) => {
          target.value = [target.value, event.results[0][0].transcript].filter(Boolean).join(' ');
          target.dispatchEvent(new Event('input', { bubbles: true }));
          target.focus();
        };
        speech.start();
      });
    }
  }
  document.addEventListener('keydown', (event) => {
    if (event.altKey || event.ctrlKey || event.metaKey || event.repeat || event.target.closest('input, textarea, select, button, a, summary, [contenteditable]')) return;
    const stage = document.querySelector('.review-stage');
    if (!stage || pending) return;
    if (stage.dataset.graded === 'true' && (event.key === ' ' || event.key === 'Enter')) {
      event.preventDefault();
      stage.querySelector('.next-form button')?.click();
    } else if (/^[1-6]$/.test(event.key)) {
      const choice = stage.querySelectorAll('.choice-fieldset button')[Number(event.key) - 1];
      if (choice) { event.preventDefault(); choice.click(); }
    }
  });
  document.addEventListener('htmx:beforeSwap', (event) => {
    if (event.detail.target?.id === 'main' && !pending?.unknown) event.detail.target.classList.add('is-outgoing');
  });
  document.addEventListener('htmx:afterSwap', () => {
    const root = document.getElementById('main');
    if (root) {
      root.classList.remove('is-outgoing');
      root.classList.add('is-surfacing');
      setTimeout(() => root.classList.remove('is-surfacing'), 220);
      enhance(root);
      if (root.querySelector('.feedback-correct')) {
        const chip = root.querySelector('.concept-chip .star');
        chip?.classList.add('chip-glow');
        if (chip) setTimeout(() => chip.classList.remove('chip-glow'), 610);
      }
    }
  });
  enhance(document);
})();
