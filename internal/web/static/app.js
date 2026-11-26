// Scry's progressive enhancement. Every page works without this file;
// with it, the keyboard does more and the study session never reloads.

// Client telemetry: a blocked resource, a script error or an unhandled
// rejection is reported to /api/client-error so it reaches the operator
// instead of failing silently. Reports carry fixed codes and error class
// names, never message text, query strings, fragments or page content; they
// are de-duplicated and capped per page load. The server maps every field
// onto its own closed allowlists again before logging.
(function () {
  "use strict";
  var endpoint = "/api/client-error", max = 5, sent = 0, seen = {};
  var keywords = { "inline": 1, "eval": 1, "wasm-eval": 1, "self": 1, "trusted-types-policy": 1, "trusted-types-sink": 1 };

  // clean keeps scheme, host and path: never userinfo, query or fragment.
  function clean(uri) {
    if (!uri) return "";
    uri = String(uri);
    if (keywords[uri]) return uri;
    try {
      var u = new URL(uri, location.href);
      if (u.protocol === "http:" || u.protocol === "https:" || u.protocol === "ws:" || u.protocol === "wss:") {
        return u.protocol + "//" + u.host + u.pathname;
      }
      return u.protocol.replace(":", "");
    } catch (err) {
      return uri.split(/[?#]/)[0].slice(0, 200);
    }
  }

  // errorClass is the error's standard class name (error.name, or the
  // prefix of a browser's "Uncaught TypeError: ..." line), or fallback.
  // The message itself is never sent.
  var classes = /^(TypeError|ReferenceError|SyntaxError|RangeError|URIError|SecurityError|NetworkError|AbortError|DOMException|Error)$/;
  function errorClass(v, line, fallback) {
    var name = "";
    try { if (v && typeof v === "object" && typeof v.name === "string") name = v.name; } catch (err) { /* opaque */ }
    if (!classes.test(name)) {
      var m = /^(?:Uncaught\s+)?([A-Za-z]+)(?::|$)/.exec(typeof line === "string" ? line : "");
      name = m ? m[1] : "";
    }
    if (!classes.test(name)) {
      try { if (typeof DOMException !== "undefined" && v instanceof DOMException) name = "DOMException"; } catch (err) { /* ignore */ }
    }
    return classes.test(name) ? name : fallback;
  }

  function send(report) {
    report.route = location.pathname;
    var key = JSON.stringify(report);
    if (seen[key] || sent >= max) return;
    seen[key] = 1;
    sent++;
    var body = JSON.stringify(report);
    try {
      if (navigator.sendBeacon && navigator.sendBeacon(endpoint, new Blob([body], { type: "application/json" }))) return;
    } catch (err) { /* fall through to fetch */ }
    try {
      fetch(endpoint, { method: "POST", body: body, keepalive: true, credentials: "same-origin",
        headers: { "Content-Type": "application/json" } }).catch(function () {});
    } catch (err) { /* nothing else to try */ }
  }

  var blocked = {}; // URIs the policy blocked: their load errors are not news

  document.addEventListener("securitypolicyviolation", function (e) {
    blocked[clean(e.blockedURI)] = 1;
    send({
      kind: "csp_violation", violatedDirective: e.violatedDirective || "", effectiveDirective: e.effectiveDirective || "",
      blockedURI: clean(e.blockedURI), disposition: e.disposition || ""
    });
  });

  window.addEventListener("error", function (e) {
    var el = e.target;
    if (el && el !== window && el.tagName) {
      // A script, stylesheet or image that failed to load (capture phase).
      // The violation event can arrive just after the load error, so wait a
      // moment before deciding this is a separate failure.
      var src = clean(el.src || el.href || "");
      setTimeout(function () {
        if (!blocked[src]) send({ kind: "resource_error", code: "resource_load_failed", sourceFile: src });
      }, 100);
      return;
    }
    // Cross-origin scripts we don't control report only "Script error.".
    if (!e.filename && /^Script error\.?$/.test(e.message || "")) return;
    if (/^(chrome|moz|safari-web)-extension:/.test(e.filename || "")) return;
    send({ kind: "js_error", errorClass: errorClass(e.error, e.message, "UnknownError"), sourceFile: clean(e.filename),
      line: e.lineno || 0, column: e.colno || 0 });
  }, true);

  window.addEventListener("unhandledrejection", function (e) {
    send({ kind: "unhandled_rejection", errorClass: errorClass(e.reason, "", "UnhandledRejection") });
  });

  // For Scry's own scripts (stripe.js) to report an integration failure
  // by fixed code.
  window.scryReport = function (kind, code) { send({ kind: kind, code: String(code) }); };

  // S113-2: a destination switch cross-fades as a cross-document view
  // transition. One the browser skips (a quick second navigation, a resize)
  // rejects its promises; that is not a failure, so it is handled here
  // rather than reported.
  var settled = function (e) {
    var t = e.viewTransition;
    if (!t) return;
    [t.ready, t.finished, t.updateCallbackDone].forEach(function (p) { if (p) p.catch(function () {}); });
  };
  window.addEventListener("pagereveal", settled);
  window.addEventListener("pageswap", settled);
})();
(function () {
  "use strict";

  var doc = document;
  var $ = function (sel, root) { return (root || doc).querySelector(sel); };
  var $$ = function (sel, root) { return Array.prototype.slice.call((root || doc).querySelectorAll(sel)); };
  var typing = function (el) {
    return el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT" || el.isContentEditable);
  };

  // ---- Focus ring: for the keyboard only --------------------------------

  // DESIGN.md (Color): the 2 px accent ring shows while the learner moves
  // by keyboard (Tab, arrows, Enter or Space on a control). A control Scry
  // focuses by script after a tap, a click or typing in a field ("Next
  // card" when a review opens) shows no ring: on iPhone a tapped button
  // never takes focus, so the browser's own :focus-visible carried the text
  // field's keyboard state over to Next card and drew a ring around its
  // fill. Without script, the browser's :focus-visible rule applies.
  var root = doc.documentElement;
  root.classList.add("js-focus");
  var navKeys = { Tab: 1, ArrowUp: 1, ArrowDown: 1, ArrowLeft: 1, ArrowRight: 1, Enter: 1, " ": 1 };
  doc.addEventListener("keydown", function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey || !navKeys[e.key]) return;
    if (e.key === "Tab" || !typing(e.target)) root.classList.add("kbd-nav");
  }, true);
  ["pointerdown", "mousedown", "touchstart"].forEach(function (ev) {
    doc.addEventListener(ev, function () { root.classList.remove("kbd-nav"); }, { capture: true, passive: true });
  });

  // ---- Waits, status lines and errors (S113-2) ---------------------------

  // DESIGN.md (Waits, errors and recovery): the control that starts a wait
  // is disabled and marked busy at once, so a second press does nothing; its
  // word (Meta mono) appears once the wait passes 400 ms, in a polite live
  // region; a failure says what happened in words, nudges the control once
  // and leaves everything typed or chosen as it was, to retry in place. A
  // page restored by Back gives every control back (pageshow, below).
  var still = window.matchMedia ? window.matchMedia("(prefers-reduced-motion: reduce)") : { matches: false };
  var WAIT_DELAY = 400;

  // restart re-runs a one-shot CSS animation class.
  function restart(el, cls) {
    el.classList.remove(cls);
    void el.offsetWidth;
    el.classList.add(cls);
  }

  // nudge moves the field or control a failure names 4 px to the side and
  // back, once (180 ms; nothing under reduced motion).
  function nudge(el) {
    if (!el) return;
    restart(el, "is-nudged");
    clearTimeout(el.nudgeTimer);
    el.nudgeTimer = setTimeout(function () { el.classList.remove("is-nudged"); }, 240);
  }

  // tell sets a status line's words (a string or nodes): they fade in with
  // a 4 px rise; an empty line fades out (120 ms) and is cleared.
  function tell(el, content) {
    if (!el) return;
    clearTimeout(el.tellTimer);
    el.classList.remove("is-out");
    if (!content) {
      el.removeAttribute("data-failed");
      if (!el.textContent) return;
      el.classList.remove("is-in");
      if (still.matches) { el.textContent = ""; return; }
      el.classList.add("is-out");
      el.tellTimer = setTimeout(function () { el.textContent = ""; el.classList.remove("is-out"); }, 120);
      return;
    }
    if (typeof content === "string") el.textContent = content;
    else el.replaceChildren.apply(el, content);
    restart(el, "is-in");
  }

  // waitWord says a wait's word in a status line. A later word ("still
  // checking", "transcribing") cross-fades in over the last, in place.
  function waitWord(el, word, pulse) {
    if (!el) return;
    clearTimeout(el.tellTimer);
    el.classList.remove("is-out", "is-in");
    el.removeAttribute("data-failed");
    var box = el.firstElementChild && el.firstElementChild.classList.contains("wait-words") ? el.firstElementChild : null;
    if (!box) {
      box = doc.createElement("span");
      box.className = "wait-words";
      el.replaceChildren(box);
    }
    var old = box.lastElementChild;
    var w = doc.createElement("span");
    w.className = "wait-word" + (pulse ? " is-pulsing" : "");
    w.setAttribute("data-wait-word", word);
    w.textContent = word;
    box.appendChild(w);
    if (old) {
      old.classList.remove("is-pulsing");
      old.classList.add("is-leaving");
      old.setAttribute("aria-hidden", "true");
      setTimeout(function () { old.remove(); }, still.matches ? 0 : 120);
    }
  }

  // labelNode is the control's own word: its last text node with letters
  // (an icon may sit before or after it).
  function labelNode(control) {
    for (var n = control.lastChild; n; n = n.previousSibling) {
      if (n.nodeType === 3 && n.nodeValue.trim()) return n;
    }
    return null;
  }

  // hold marks the control that started a wait: disabled (a link is
  // aria-disabled) and aria-busy at once, [data-waiting] while it lasts.
  // With a word, the control's label reads it after 400 ms (opts.now: at
  // once), and opts.live (a polite status line) says it too.
  function hold(control, word, opts) {
    if (!control) return;
    opts = opts || {};
    var h = { label: labelNode(control), shown: null, live: opts.live || null, timer: 0,
      hadFocus: doc.activeElement === control };
    control.scryHold = h;
    control.setAttribute("aria-busy", "true");
    control.setAttribute("data-waiting", word || "");
    if (control.tagName === "A") control.setAttribute("aria-disabled", "true");
    else control.disabled = true;
    if (!word) return;
    var show = function () {
      if (control.scryHold !== h) return;
      if (h.label) {
        h.shown = doc.createElement("span");
        h.shown.className = "wait-word";
        h.shown.setAttribute("data-wait-word", word);
        h.shown.textContent = word;
        h.label.replaceWith(h.shown);
      }
      if (h.live) {
        h.sr = doc.createElement("span");
        h.sr.className = "sr";
        h.sr.textContent = word;
        tell(h.live, [h.sr]);
      }
    };
    if (opts.now) show(); else h.timer = setTimeout(show, WAIT_DELAY);
  }

  // release gives a held control back as it was, focus included.
  function release(control) {
    var h = control && control.scryHold;
    if (!h) return;
    control.scryHold = null;
    clearTimeout(h.timer);
    if (h.shown && h.label) h.shown.replaceWith(h.label);
    if (h.sr && h.sr.isConnected) tell(h.live, "");
    control.removeAttribute("aria-busy");
    control.removeAttribute("data-waiting");
    if (control.tagName === "A") control.removeAttribute("aria-disabled");
    else control.disabled = false;
    if (h.hadFocus && (doc.activeElement === doc.body || !doc.activeElement)) control.focus({ preventScroll: true });
  }
  function held(control) { return !!(control && control.hasAttribute("data-waiting")); }

  // Others a wait sets aside (Snooze, Delete, Check, Next card while one of
  // them runs) are only disabled, and marked so they can be given back.
  function setAside(controls) {
    var taken = controls.filter(function (b) { return b && !b.disabled; });
    taken.forEach(function (b) { b.disabled = true; b.setAttribute("data-set-aside", ""); });
    return function () { taken.forEach(function (b) { b.disabled = false; b.removeAttribute("data-set-aside"); }); };
  }

  // A page brought back by Back (the back/forward cache) gives every
  // control back as it was, and the answer field re-measures.
  window.addEventListener("pageshow", function (e) {
    if (e.persisted) {
      $$("[data-waiting]").forEach(release);
      $$("[data-set-aside]").forEach(function (b) { b.disabled = false; b.removeAttribute("data-set-aside"); });
      $$("[data-import-progress]").forEach(function (p) { p.hidden = true; });
      $$("[data-status] .wait-words").forEach(function (w) { w.remove(); });
      $$("[data-answer-input]").forEach(function (i) { i.readOnly = false; });
    }
    if (growAnswer) growAnswer();
  });

  // ---- Swap <main> from a fetched page -----------------------------------

  // opts.top: this swap is the answer-submit -> review transition. Focusing
  // the answer field below a long question scrolls the waiting page down (a
  // phone keyboard does), and swapping <main> in place would keep that
  // offset and clip the question off the top; the review is reframed at
  // the top instead. Every other swap (Ask Why, Edit card, ...) keeps the
  // learner's own scroll position.
  //
  // opts.vt (S113-2): how the card moves. "verdict": the waiting card fades
  // out while the review's own marks draw in; "card": the review fades out
  // and the next card rises in; "leave": a snoozed or deleted card fades out
  // moving left, and the next one rises in. Focus moves as soon as the new
  // card is in; nothing waits on the animation.
  function swap(html, url, opts) {
    var next = new DOMParser().parseFromString(html, "text/html");
    var main = $("main", next);
    if (!main) { location.href = url; return; }
    var apply = function () {
      $("main").innerHTML = main.innerHTML;
      doc.title = next.title;
      doc.body.className = next.body.className;
      // The active underline draws in only when the destination changes.
      var nav = $(".nav", next), here = $(".nav");
      if (nav && here && nav.innerHTML !== here.innerHTML) here.innerHTML = nav.innerHTML;
      if (url && url !== location.href) history.replaceState(null, "", url);
      init();
      if (opts && opts.top && $("[data-review]")) window.scrollTo(0, 0);
      if (frameReview()) return;
      var focus = $("main [autofocus]");
      if (focus) focus.focus();
    };
    move(opts && opts.vt, apply);
  }

  // move runs a card swap inside a same-document view transition (styled in
  // app.css by html[data-vt]); without one, or under reduced motion, the
  // swap is immediate and the next card still rises in (.is-arriving).
  function move(kind, apply) {
    if (!kind || still.matches) { apply(); return; }
    if (!doc.startViewTransition || doc.hidden) {
      apply();
      var card = kind !== "verdict" && $("main .current, main .done, main .empty-state");
      if (card) card.classList.add("is-arriving");
      return;
    }
    root.setAttribute("data-vt", kind);
    var clear = function () { root.removeAttribute("data-vt"); };
    var t;
    try { t = doc.startViewTransition(apply); } catch (err) { clear(); apply(); return; }
    t.finished.then(clear, clear);
    t.ready.catch(function () {});
    if (t.updateCallbackDone) t.updateCallbackDone.catch(function () {});
  }

  // The post-answer review stays framed at the question and its judgment
  // (DESIGN.md, Post-answer review): "Next card" takes focus without
  // scrolling, so loud Judgment type never pushes the question or the top
  // bar out of view. The button carries no HTML autofocus (a browser
  // without script would scroll a sentence-length review down to it); it
  // is marked data-autofocus-no-scroll and focused only here.
  function frameReview() {
    var review = $("[data-review]"), next = $("[data-next][data-autofocus-no-scroll]");
    if (!review || !next) return false;
    next.focus({ preventScroll: true });
    return true;
  }

  // post sends a form by fetch and resolves with the page that came back
  // (and whether it was a success); timeout (ms) gives up on a request
  // that never answers.
  function post(form, extra, timeout) {
    var body = new URLSearchParams(new FormData(form));
    if (extra) Object.keys(extra).forEach(function (k) { body.set(k, extra[k]); });
    var ctl = timeout && window.AbortController ? new AbortController() : null;
    var timer = ctl ? setTimeout(function () { ctl.abort(); }, timeout) : 0;
    return fetch(form.action, {
      method: "POST", body: body, credentials: "same-origin",
      headers: { "X-Requested-With": "fetch" }, signal: ctl ? ctl.signal : undefined
    }).then(function (res) {
      return res.text().then(function (html) {
        clearTimeout(timer);
        return { html: html, url: res.url, ok: res.ok, status: res.status };
      });
    }, function (err) { clearTimeout(timer); throw err; });
  }

  // ---- Global keys -------------------------------------------------------

  var go = { s: "/study", l: "/library", n: "/learn", b: "/billing" };
  var pendingG = 0;
  doc.addEventListener("keydown", function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (typing(e.target)) {
      if (e.key === "Escape") { e.preventDefault(); e.target.blur(); }
      return;
    }
    if (!$(".nav a[href='/study']")) return; // signed out
    var next = $("[data-next]"), answer = $("[data-answer-form]"), again = $("[data-again]");
    if ((e.key === "a" || e.key === "A") && again) {
      e.preventDefault(); again.click(); return;
    }
    // Buttons, links and the Edit card toggle retain their native
    // Enter/Space activation.
    var nativeControl = e.target.closest("button, a, input, select, textarea, summary");
    if (e.key === "Enter" && !nativeControl && (next || answer)) {
      e.preventDefault();
      if (next) next.click(); else answer.requestSubmit();
      return;
    }
    // In the post-answer review, Space also moves on to the next card.
    if (e.key === " " && !nativeControl && next) {
      e.preventDefault(); next.click(); return;
    }
    if (e.key === " " && !nativeControl && $("[data-mic]")) {
      e.preventDefault(); $("[data-mic]").click(); return;
    }
    if (pendingG && Date.now() - pendingG < 1200 && go[e.key]) {
      pendingG = 0;
      e.preventDefault();
      location.href = go[e.key];
      return;
    }
    pendingG = e.key === "g" ? Date.now() : 0;
    if (e.key === "/") {
      // On phones the outline (and its search) is hidden; the collapsible
      // idea picker holds the search instead, so open it and focus that.
      var search = $("[data-search]"), picker = $("[data-idea-picker]");
      if (search && search.offsetParent === null && picker && picker.offsetParent !== null) {
        search = $("[data-picker-search]", picker);
        if (search) picker.open = true;
      }
      if (search && search.offsetParent !== null) { e.preventDefault(); search.focus(); search.select(); }
    }
  });

  // ---- Time zone ---------------------------------------------------------

  function initTZ() {
    var tz = "";
    try { tz = Intl.DateTimeFormat().resolvedOptions().timeZone || ""; } catch (err) { /* keep UTC */ }
    $$("[data-tz-form] input[name=tz]").forEach(function (el) { el.value = tz; });
  }

  // ---- Starter shelf: the adding state -----------------------------------

  // Adding a set confirms in place: "+ Add" becomes "Adding…" and then
  // the "Added" tag, and the "Start studying" bar appears, without a page
  // load, so the window never jumps back to the top. If the request fails,
  // the form posts the plain way.
  function initShelf() {
    var bar = $("[data-shelf-bar]"), summary = $("[data-shelf-summary]"), tpl = $("[data-added-template]");
    $$("form[data-add]").forEach(function (form) {
      form.addEventListener("submit", function (e) {
        var b = $("button", form), label = $("[data-add-label]", form) || b, tile = form.closest("[data-tile]");
        if (b.disabled) { e.preventDefault(); return; }
        label.textContent = "Adding…";
        if (!window.fetch || !tpl || !tpl.content || !tile) {
          // Disable after the submit has been dispatched.
          setTimeout(function () { b.disabled = true; }, 0);
          return;
        }
        e.preventDefault();
        b.disabled = true;
        b.setAttribute("aria-busy", "true");
        fetch(form.action, {
          method: "POST", body: new URLSearchParams(new FormData(form)), credentials: "same-origin",
          headers: { "Accept": "application/json" }
        }).then(function (res) {
          if (!res.ok) throw new Error("add failed");
          return res.json();
        }).then(function (j) {
          var tag = tpl.content.firstElementChild.cloneNode(true);
          tile.classList.add("is-added");
          form.replaceWith(tag);
          tag.tabIndex = -1;
          tag.focus({ preventScroll: true });
          if (summary) summary.textContent = j.summary || "";
          if (bar) bar.hidden = false;
        }).catch(function () { form.submit(); });
      });
    });
  }

  // ---- Card and billing forms: the processing state ----------------------

  // Submitting a card or a top-up shows at once that it is being confirmed:
  // the button is disabled (no double submit), a spinner and plain words
  // appear with a progress line. stripe.js ends it again if the processor
  // refuses the card; a page brought back from the back cache starts clean.
  //
  // Each submission is one attempt, numbered on the form
  // (form.dataset.submitAttempt). Starting the state and ending it (an
  // error, the 30 second timeout, the back cache) both move the number on,
  // so work still in flight for an earlier attempt (stripe.js waiting on
  // the card processor) can tell it has been superseded and must not touch
  // the form or submit it.
  function busyAttempt(form) {
    return Number(form.dataset.submitAttempt) || 0;
  }
  function busySupersede(form) {
    var next = busyAttempt(form) + 1;
    form.dataset.submitAttempt = String(next);
    return next;
  }
  function busyStart(form) {
    busySupersede(form);
    var button = $("button:not([type=button])", form), box = $("[data-busy]", form);
    form.classList.add("is-busy");
    form.setAttribute("aria-busy", "true");
    if (button) {
      button.setAttribute("aria-busy", "true");
      // Disable after the submit has been dispatched.
      setTimeout(function () { if (form.classList.contains("is-busy")) button.disabled = true; }, 0);
    }
    if (box) {
      var text = $("[data-busy-text]", box);
      if (text && form.dataset.busyText) text.textContent = form.dataset.busyText;
      box.hidden = false;
    }
    // If the page is still here long after (the request was stopped or
    // never left), give the form back rather than leave it locked. That
    // supersedes the attempt: whatever it was waiting for is ignored.
    clearTimeout(form.busyTimer);
    form.busyTimer = setTimeout(function () { busyStop(form); }, 30000);
  }
  function busyStop(form) {
    var button = $("button:not([type=button])", form), box = $("[data-busy]", form);
    busySupersede(form);
    clearTimeout(form.busyTimer);
    form.classList.remove("is-busy");
    form.removeAttribute("aria-busy");
    if (button) { button.removeAttribute("aria-busy"); button.disabled = false; }
    if (box) box.hidden = true;
  }
  window.scryBusy = { start: busyStart, stop: busyStop, attempt: busyAttempt };

  function initBusy() {
    $$("form[data-busy-form]").forEach(function (form) {
      form.addEventListener("submit", function (e) {
        if (e.defaultPrevented) return;
        if (form.classList.contains("is-busy")) { e.preventDefault(); return; }
        busyStart(form);
      });
    });
  }
  window.addEventListener("pageshow", function (e) {
    if (e.persisted) $$("form[data-busy-form].is-busy").forEach(busyStop);
  });

  // ---- Study -------------------------------------------------------------

  var voice = null;       // the recording in progress, if any
  var growAnswer = null;  // re-measures the waiting card's answer field
  var COULDNT_CHECK = "Couldn't check that just now. Your answer is still here; press Enter to try again.";

  // S111-2: the answer field grows with what is typed, pasted or dictated.
  // Its height is measured on a hidden twin (same class, one row, the same
  // width), never by collapsing the visible field, so it never flickers,
  // shrinks mid-word or moves the prompt above; the field then eases to it
  // (120 ms, [data-grown]; at once under reduced motion). Past max-height
  // (45vh) it scrolls inside itself.
  function answerField(input) {
    var twin = doc.createElement("textarea");
    twin.className = input.className + " answer-measure";
    twin.rows = 1;
    twin.tabIndex = -1;
    twin.setAttribute("aria-hidden", "true");
    twin.setAttribute("inert", "");
    input.parentNode.appendChild(twin);
    input.rows = 1;
    var last = -1;
    function grow() {
      if (!input.isConnected) return;
      var cs = getComputedStyle(input);
      var border = (parseFloat(cs.borderTopWidth) || 0) + (parseFloat(cs.borderBottomWidth) || 0);
      twin.style.width = input.offsetWidth + "px";
      twin.value = input.value;
      var h = twin.scrollHeight + border;
      var max = parseFloat(cs.maxHeight);
      if (max > 0 && h > max) h = max;
      if (h === last) return;
      last = h;
      input.style.height = h + "px";
      keepInView(input, h);
    }
    grow();
    void input.offsetHeight; // the first height is set at once, not eased
    input.setAttribute("data-grown", "");
    input.addEventListener("input", grow);
    input.addEventListener("paste", function () { setTimeout(grow, 0); });
    if (doc.fonts && doc.fonts.ready) doc.fonts.ready.then(grow);
    return grow;
  }

  // Phones: as the field grows it stays in view above the keyboard and the
  // pinned Check bar.
  function keepInView(input, h) {
    if (doc.activeElement !== input || !window.visualViewport) return;
    var vv = window.visualViewport, bar = $(".answer-actions");
    var cover = bar && getComputedStyle(bar).position === "fixed" ? bar.offsetHeight : 0;
    var bottom = input.getBoundingClientRect().top + h;
    var limit = vv.offsetTop + vv.height - cover - 8;
    if (bottom > limit) window.scrollBy(0, bottom - limit);
  }
  window.addEventListener("resize", function () { if (growAnswer) growAnswer(); });
  window.addEventListener("orientationchange", function () { if (growAnswer) growAnswer(); });

  // The field names its key hint for assistive technology: the desktop hint,
  // or "Return to check" at phone width (where the other hints are hidden).
  var phoneWidth = window.matchMedia ? window.matchMedia("(max-width: 760px)") : null;
  function pickHint() {
    var input = $("[data-answer-input]");
    if (!input || !phoneWidth) return;
    input.setAttribute("aria-describedby", (phoneWidth.matches ? "answer-hint-phone" : "answer-hint") + " status");
  }
  if (phoneWidth) {
    if (phoneWidth.addEventListener) phoneWidth.addEventListener("change", pickHint);
    else if (phoneWidth.addListener) phoneWidth.addListener(pickHint);
  }

  // Spoken words are added after what is already typed: one space between,
  // or on the current line when it ends in a line break; the caret after.
  function appendSpoken(input, text) {
    var had = input.value;
    input.value = had + (had === "" || /\s$/.test(had) ? "" : " ") + text;
    var end = input.value.length;
    input.setSelectionRange(end, end);
    input.scrollBy(0, input.scrollHeight); // the field's own scroll, not the page's
  }

  // The status line a re-rendered study page carries ("Couldn't check that
  // just now…", an allowance sentence and its link), as nodes for this page.
  function statusFrom(html) {
    var page = new DOMParser().parseFromString(html, "text/html");
    var el = $("[data-answer-form] [data-status]", page);
    if (!el || !el.textContent.trim()) return null;
    return Array.prototype.map.call(el.childNodes, function (n) { return doc.importNode(n, true); });
  }

  function initStudy() {
    var form = $("[data-answer-form]");
    var nextForm = $("[data-next-form]");
    if (nextForm) initNext(nextForm);
    initExplain();
    initEdit();
    initManage();
    growAnswer = null;
    if (!form) return;

    var input = $("[data-answer-input]", form);
    var status = $("[data-status]", form);
    var via = $("[data-via]", form);
    var mic = $("[data-mic]", form);
    var check = $("[data-check]", form);

    growAnswer = answerField(input);
    pickHint();

    function say(text) { tell(status, text); }

    input.addEventListener("input", function () {
      if (!voice) via.value = "typed";
    });

    // Enter checks; Shift+Enter starts a new line (S111-2); never while an
    // input method is composing. Space in an empty field starts listening;
    // a second press stops.
    input.addEventListener("keydown", function (e) {
      if (e.key === "Enter" && !e.shiftKey && !e.altKey && !e.isComposing && e.keyCode !== 229) {
        e.preventDefault();
        form.requestSubmit();
        return;
      }
      if (e.key === " " && !e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey && !input.readOnly && (voice || input.value === "")) {
        e.preventDefault();
        toggleVoice();
      }
    });
    mic.addEventListener("click", function () { toggleVoice(); input.focus(); });

    // Checking (S113-2): Check is disabled and busy at once, the field keeps
    // its text and height (read-only while it is judged), "checking" pulses
    // under it after 400 ms and "still checking" cross-fades in after 3 s.
    // If grading cannot finish, or no answer comes back, the line says so,
    // the field nudges once and keeps the answer exactly as written, and
    // Enter retries.
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (held(check)) return;
      hold(check);
      input.readOnly = true;
      var slow = 0, slower = 0;
      var settle = function () { clearTimeout(slow); clearTimeout(slower); };
      var fail = function (nodes) {
        settle();
        release(check);
        input.readOnly = false;
        tell(status, nodes || COULDNT_CHECK);
        status.setAttribute("data-failed", "");
        nudge(input);
        input.focus({ preventScroll: true });
        if (growAnswer) growAnswer();
      };
      var send = function () {
        tell(status, "");
        slow = setTimeout(function () { waitWord(status, "checking", true); }, WAIT_DELAY);
        slower = setTimeout(function () { waitWord(status, "still checking", true); }, 3000);
        post(form, null, 90000).then(function (r) {
          if (!r.ok) { fail(statusFrom(r.html)); return; }
          settle();
          swap(r.html, r.url, { top: true, vt: "verdict" });
        }, function () { fail(); });
      };
      if (voice) stopVoice().then(send); else send();
    });

    function toggleVoice() {
      if (voice) { stopVoice(); return; }
      if (!navigator.mediaDevices || !(window.AudioContext || window.webkitAudioContext)) {
        say("This browser can't record. Type your answer instead.");
        return;
      }
      navigator.mediaDevices.getUserMedia({ audio: true }).then(function (stream) {
        startVoice(stream);
      }).catch(function (err) {
        if (err && (err.name === "NotAllowedError" || err.name === "SecurityError")) {
          say(window.matchMedia("(pointer: coarse)").matches ? "Allow mic in site settings." : "Allow mic, then retry.");
        } else {
          say("No microphone was found. Type your answer instead.");
        }
      });
    }

    function startVoice(stream) {
      var Ctx = window.AudioContext || window.webkitAudioContext;
      // A silent sink does not depend on a working output speaker.
      var ac = new Ctx({ sinkId: { type: "none" } }), source = ac.createMediaStreamSource(stream);
      var processor = ac.createScriptProcessor(4096, 1, 1);
      var v = { stream: stream, ac: ac, source: source, processor: processor, chunks: [], done: null };
      voice = v;
      processor.onaudioprocess = function (event) {
        if (voice === v && !v.done) v.chunks.push(new Float32Array(event.inputBuffer.getChannelData(0)));
      };
      source.connect(processor); processor.connect(ac.destination);
      ac.resume();
      form.classList.add("is-listening"); mic.setAttribute("aria-pressed", "true"); waitWord(status, "listening");
      v.timer = setTimeout(stopVoice, 20000);
    }

    // Listening stops: "transcribing" until the words settle in the field
    // (the mic is busy meanwhile), then the field re-measures at once.
    function stopVoice() {
      var v = voice;
      if (!v) return Promise.resolve();
      if (v.done) return v.done;
      clearTimeout(v.timer);
      v.processor.disconnect(); v.source.disconnect();
      v.stream.getTracks().forEach(function (t) { t.stop(); });
      var count = v.chunks.reduce(function (n, c) { return n + c.length; }, 0);
      var buffer = new ArrayBuffer(44 + count * 2), data = new DataView(buffer);
      function chars(at, s) { for (var i = 0; i < s.length; i++) data.setUint8(at + i, s.charCodeAt(i)); }
      chars(0, "RIFF"); data.setUint32(4, 36 + count * 2, true); chars(8, "WAVEfmt ");
      data.setUint32(16, 16, true); data.setUint16(20, 1, true); data.setUint16(22, 1, true);
      data.setUint32(24, v.ac.sampleRate, true); data.setUint32(28, v.ac.sampleRate * 2, true);
      data.setUint16(32, 2, true); data.setUint16(34, 16, true); chars(36, "data");
      data.setUint32(40, count * 2, true);
      var at = 44;
      v.chunks.forEach(function (c) { c.forEach(function (x) {
        x = Math.max(-1, Math.min(1, x)); data.setInt16(at, x < 0 ? x * 32768 : x * 32767, true); at += 2;
      }); });
      v.ac.close();
      form.classList.remove("is-listening"); mic.setAttribute("aria-pressed", "false");
      hold(mic);
      waitWord(status, "transcribing");
      // Exactly one provider upload, after recording stops; no client duration.
      v.done = fetch("/study/speech", {
        method: "POST", body: new Blob([buffer], { type: "audio/wav" }), credentials: "same-origin",
        headers: { "Content-Type": "audio/wav", "X-CSRF-Token": form.elements.csrf.value }
      }).then(function (res) { return res.json().then(function (j) {
        // Short, empty and silent recordings come back 200 with no text.
        if (!res.ok) { say(j.error || "Scry can't listen just now. Type your answer instead."); nudge(mic); return; }
        if (j.text) {
          appendSpoken(input, j.text); via.value = "spoken";
          if (growAnswer) growAnswer();
          say("heard · enter to check");
        } else say("Didn't catch that. Try again or type it.");
        if (!j.text) nudge(mic);
      }); }).catch(function () { say("Couldn't reach Scry. Type your answer instead."); nudge(mic); })
        .finally(function () {
          voice = null;
          release(mic);
        });
      return v.done;
    }
  }

  // The on-demand "Why?": one bounded request after the verdict, shown in
  // place. It never changes the verdict, and any failure shows the fixed
  // criterion sentence instead of a dead end.
  function initExplain() {
    var form = $("[data-explain-form]");
    if (!form) return;
    var button = $("[data-explain]", form), out = $("[data-explanation]", form), note = $("[data-explain-note]", form);
    if (!button) return;
    var busy = false;
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (busy) return;
      busy = true;
      button.setAttribute("aria-busy", "true");
      button.textContent = "explaining";
      var show = function (text, fallback) {
        out.textContent = text;
        out.hidden = false;
        if (note) note.hidden = !fallback;
        button.remove();
        var next = $("[data-next]");
        if (next) next.focus({ preventScroll: true });
      };
      var why = $(".why");
      fetch(form.action, {
        method: "POST", body: new URLSearchParams(new FormData(form)), credentials: "same-origin",
        headers: { "X-Requested-With": "fetch", "Accept": "application/json" }
      }).then(function (res) { return res.json().then(function (j) {
        if (res.ok && j.text) show(j.text, !!j.fallback);
        else show(why ? why.textContent : (j.error || ""), true);
      }); }).catch(function () { show(why ? why.textContent : "", true); });
    });
  }

  // "Edit card" in the post-answer review: the question, correct answer
  // and idea are saved without leaving the review. What is on screen (the
  // question, the correct answer and the place in the rail) is updated in
  // place and "Saved" is shown; the grade and schedule are never touched.
  // A refused field is said in words under that field. While it saves,
  // "Save card" is disabled and reads "saving" (S113-2); if the save cannot
  // be made, the line beside it says so and every field keeps its text.
  function initEdit() {
    var form = $("[data-edit-form]");
    if (!form || !window.fetch) return;
    var status = $("[data-edit-status]", form), save = $("[data-edit-save]", form), failed = $("[data-edit-fail]", form);
    var errors = $$("[data-edit-error]", form);
    var clearErrors = function () {
      errors.forEach(function (el) {
        el.hidden = true; el.textContent = "";
        el.parentNode.classList.remove("has-error");
        var field = form.elements[el.getAttribute("data-edit-error")];
        if (field) field.removeAttribute("aria-invalid");
      });
    };
    form.addEventListener("input", function () { status.hidden = true; });
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (held(save)) return;
      hold(save, "saving", { live: failed });
      clearErrors();
      status.hidden = true;
      var refuse = function (text, field) {
        release(save);
        var el = $("[data-edit-error='" + field + "']", form) || errors[0];
        el.textContent = text; el.hidden = false;
        el.parentNode.classList.add("has-error");
        var input = form.elements[el.getAttribute("data-edit-error")];
        if (input) { input.setAttribute("aria-invalid", "true"); input.focus(); }
      };
      var broke = function (text) {
        release(save);
        tell(failed, text || "Couldn't save that just now. Your edits are still here.");
        nudge(save);
      };
      fetch(form.action, {
        method: "POST", body: new URLSearchParams(new FormData(form)), credentials: "same-origin",
        headers: { "Accept": "application/json" }
      }).then(function (res) { return res.json().then(function (j) {
        if (res.ok && j.saved) { saved(j); return; }
        if (res.ok && j.field) { refuse(j.error, j.field); return; }
        broke(res.status < 500 && j.error ? j.error : "");
      }, function () { broke(); }); }, function () { broke(); });
    });
    function saved(j) {
      release(save);
      tell(failed, "");
      // The text above the form may grow or shrink (a missed answer is set
      // very large): keep the form where it was in the window.
      var before = save.getBoundingClientRect().top;
      $$("[data-canonical]").forEach(function (el) { el.textContent = j.answer; });
      $$("[data-prompt]").forEach(function (el) { el.textContent = j.prompt; });
      form.elements.prompt.value = j.prompt;
      form.elements.answer.value = j.answer;
      if (j.header) $$("[data-study-header]").forEach(function (el) { el.textContent = j.header; });
      if (j.idea && form.elements.idea) {
        form.elements.idea.value = j.idea;
        // A newly named idea is offered from now on, too.
        var list = $("[data-edit-ideas]", form);
        var known = list && $$("option", list).some(function (o) { return o.value.toLowerCase() === j.idea.toLowerCase(); });
        if (list && !known) { var o = doc.createElement("option"); o.value = j.idea; list.appendChild(o); }
      }
      status.hidden = false;
      var moved = save.getBoundingClientRect().top - before;
      if (moved) window.scrollBy(0, moved);
    }
  }

  // Snooze, Delete and Restore in Study (S106-5): each acts at once and
  // the next card (or the session's end) opens in place under one quiet
  // line, as Next card does. Without script they are plain form posts.
  // While one runs (S113-2) it reads "snoozing", "removing" or
  // "restoring", and the card's other actions are disabled; if it fails,
  // the line beside it says so and the card and any typed answer stay.
  var manageWords = { snooze: ["snoozing", "Couldn't snooze that just now."], delete: ["removing", "Couldn't remove that just now."],
    restore: ["restoring", "Couldn't restore that just now."] };
  function initManage() {
    $$("form[data-manage-form]").forEach(function (form) {
      form.addEventListener("submit", function (e) {
        e.preventDefault();
        var button = $("button", form);
        if (!button || held(button)) return;
        var kind = button.hasAttribute("data-snooze") ? "snooze" : button.hasAttribute("data-delete") ? "delete" : "restore";
        var line = $("[data-manage-status]") || manageLine(form);
        tell(line, "");
        hold(button, manageWords[kind][0], { live: line });
        var giveBack = setAside($$("[data-snooze], [data-delete], [data-restore], [data-check], [data-next], [data-again], [data-edit-save]")
          .filter(function (b) { return b !== button; }));
        var fail = function () {
          release(button);
          giveBack();
          tell(line, manageWords[kind][1]);
          nudge(button);
        };
        post(form).then(function (r) {
          if (!r.ok) { fail(); return; }
          swap(r.html, r.url, { vt: kind === "restore" ? "card" : "leave" });
        }, fail);
      });
    });
  }
  // A quiet line with no status line of its own (Restore after the session
  // ended) says a failure just after its form.
  function manageLine(form) {
    var line = doc.createElement("span");
    line.className = "status-line manage-status";
    line.setAttribute("role", "status");
    form.after(line);
    return line;
  }

  // Keys: in the review E opens Edit card; S snoozes the card in its review
  // or, while it waits, once focus has left the answer line (Escape). Never
  // while typing in a field, never with a modifier, and never a key another
  // binding took ("g s" goes to Study).
  doc.addEventListener("keydown", function (e) {
    if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey || e.repeat || typing(e.target)) return;
    if (!$("[data-session]")) return;
    var key = e.key.toLowerCase();
    if (key === "e" && $("[data-review]")) {
      var edit = $("[data-edit]");
      if (!edit) return;
      e.preventDefault();
      edit.open = true;
      var field = $("#edit-prompt", edit);
      if (field) field.focus();
      return;
    }
    if (key === "s") {
      var snooze = $("[data-snooze]");
      if (snooze) { e.preventDefault(); snooze.click(); }
    }
  });

  // Next card: the review fades out and the next card rises in (S113-2).
  // If it cannot be opened, the review stays and the line under it says so.
  function initNext(form) {
    var next = $("[data-next]", form), again = $("[data-again]", form);
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (held(next)) return;
      var extra = e.submitter && e.submitter.name === "again" ? { again: "1" } : null;
      hold(next);
      var giveBack = setAside([again].concat($$("[data-snooze], [data-delete], [data-edit-save]")));
      var fail = function () {
        release(next);
        giveBack();
        tell($("[data-manage-status]"), "Couldn't open the next card just now. Try again.");
        nudge(next);
      };
      post(form, extra).then(function (r) {
        if (!r.ok) { fail(); return; }
        swap(r.html, r.url, { vt: "card" });
      }, fail);
    });
  }

  // ---- Draft rows --------------------------------------------------------

  function initRows() {
    var rows = $$("[data-row]");
    if (!rows.length) return;
    rows.forEach(function (row) {
      row.addEventListener("keydown", function (e) {
        if (e.target !== row || e.metaKey || e.ctrlKey || e.altKey) return;
        var all = $$("[data-row]");
        var i = all.indexOf(row);
        var key = e.key.toLowerCase();
        if (key === "j" || e.key === "ArrowDown") { e.preventDefault(); (all[i + 1] || row).focus(); }
        else if (key === "k" || e.key === "ArrowUp") { e.preventDefault(); (all[i - 1] || row).focus(); }
        else if (key === "a") { e.preventDefault(); decide(row, $("form[data-approve]", row), i); }
        else if (key === "d") { e.preventDefault(); decide(row, $("form[data-dismiss]", row), i); }
      });
      $$("form", row).forEach(function (form) {
        form.addEventListener("submit", function (e) {
          e.preventDefault();
          decide(row, form, $$("[data-row]").indexOf(row));
        });
      });
    });
  }

  // decide approves or dismisses a row: it collapses upward, then the
  // page is brought in line with the server.
  function decide(row, form, index) {
    if (row.dataset.busy) return;
    row.dataset.busy = "1";
    row.style.maxHeight = row.offsetHeight + "px";
    row.offsetHeight; // settle the starting height before collapsing
    row.classList.add("is-leaving");
    var wait = new Promise(function (resolve) { setTimeout(resolve, 120); });
    Promise.all([post(form), wait]).then(function (r) {
      swap(r[0].html, r[0].url);
      var all = $$("[data-row]");
      if (all.length) all[Math.min(index, all.length - 1)].focus();
    }).catch(function () { form.submit(); });
  }

  // A draft being written: bring in the ideas as they are ready.
  var polling = 0;
  function initDraft() {
    clearTimeout(polling);
    if (!$("[data-drafting]")) return;
    polling = setTimeout(function () {
      fetch(location.href, { credentials: "same-origin" }).then(function (res) { return res.text(); }).then(function (html) {
        if (!$("[data-drafting]")) return;
        var focused = doc.activeElement && doc.activeElement.closest && doc.activeElement.closest("[data-row]");
        var index = focused ? $$("[data-row]").indexOf(focused) : -1;
        swap(html, location.href);
        if (index >= 0) { var all = $$("[data-row]"); if (all[index]) all[index].focus(); }
      }).catch(function () { initDraft(); });
    }, 700);
  }

  // ---- Create: the composer and its example prompts ----------------------

  // The field is about six lines tall and grows with what is typed or
  // pasted, up to the CSS max-height (then it scrolls). Cmd/Ctrl+Enter
  // sends; a plain Enter is a new line. Tapping an example fills the field
  // (each is a plain link without script).
  function initExamples() {
    var input = $("[data-ask-input]");
    if (!input) return;
    var grow = function () {
      input.style.height = "auto";
      var max = parseFloat(getComputedStyle(input).maxHeight) || Infinity;
      var h = input.scrollHeight + (input.offsetHeight - input.clientHeight);
      input.style.height = Math.min(h, max) + "px";
    };
    input.addEventListener("input", grow);
    grow();
    input.addEventListener("keydown", function (e) {
      if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey) {
        e.preventDefault();
        if (input.form.reportValidity()) input.form.requestSubmit();
      }
    });
    $$("[data-example]").forEach(function (a) {
      a.addEventListener("click", function (e) {
        e.preventDefault();
        input.value = a.getAttribute("data-example");
        grow();
        input.focus();
        input.setSelectionRange(input.value.length, input.value.length);
      });
    });
  }

  // ---- Create: Import a deck (S106-3, S113-2) ------------------------------

  // A file over 20 MiB is refused in words before it is sent. Otherwise
  // "Import" is disabled and reads "importing", with the 2 px progress line
  // under it. With script the file is sent by fetch, so a refusal is said
  // in words under the form with the idea title and the chosen file still
  // there, ready to try again; an import that goes through opens where the
  // server sends it (Library on the idea, or the draft). Without script the
  // form posts as usual and the server keeps the idea title.
  var IMPORT_BROKE = "Couldn't import that just now. Your file and idea title are still here; try again.";
  function initImport() {
    var form = $("[data-import-form]");
    if (!form) return;
    var status = $("[data-import-status]", form);
    var button = $("[data-import-submit]", form);
    var progress = $("[data-import-progress]", form);
    var file = form.querySelector('input[type="file"]');
    var refuse = function (text) {
      var err = doc.createElement("span");
      err.className = "field-error";
      err.textContent = text;
      tell(status, [err]);
      status.setAttribute("data-refused", "");
    };
    var stop = function () {
      release(button);
      if (progress) progress.hidden = true;
    };
    form.addEventListener("submit", function (e) {
      var f = file && file.files && file.files[0];
      if (held(button)) { e.preventDefault(); return; }
      if (f && f.size > 20 * 1024 * 1024) {
        e.preventDefault();
        refuse("That file is larger than 20 MiB. Split it and import the parts.");
        nudge(button);
        return;
      }
      status.removeAttribute("data-refused");
      if (!window.fetch || !window.FormData || !window.DOMParser) {
        // The plain post: disable after the submit has been dispatched.
        setTimeout(function () { hold(button, "importing", { now: true, live: status }); if (progress) progress.hidden = false; }, 0);
        return;
      }
      e.preventDefault();
      var body = new FormData(form);
      hold(button, "importing", { now: true, live: status });
      if (progress) progress.hidden = false;
      fetch(form.action, {
        method: "POST", body: body, credentials: "same-origin", headers: { "X-Requested-With": "fetch" }
      }).then(function (res) { return res.text().then(function (html) {
        var page = new DOMParser().parseFromString(html, "text/html");
        var refused = $("[data-import-status][data-refused]", page);
        if (refused) {
          stop();
          refuse(refused.textContent.trim());
          nudge(button);
          return;
        }
        if (!res.ok) throw new Error("import failed");
        location.assign(res.url);
      }); }).catch(function () {
        stop();
        refuse(IMPORT_BROKE);
        nudge(button);
      });
    });
  }

  // ---- Settings: Download Markdown, CSV or JSON (S113-2) -------------------

  // Each link fetches its file: it reads "preparing" (after 400 ms) until
  // the download begins, and a failure says "Couldn't prepare that
  // download. Try again." beside it. Without script (or with a modifier
  // key) it is a plain download link.
  var DOWNLOAD_BROKE = "Couldn't prepare that download. Try again.";
  function initDownloads() {
    var status = $("[data-download-status]");
    $$("a[data-download]").forEach(function (a) {
      a.addEventListener("click", function (e) {
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button) return;
        if (!window.fetch || !window.URL || !URL.createObjectURL) return;
        e.preventDefault();
        if (held(a)) return;
        tell(status, "");
        hold(a, "preparing", { live: status });
        fetch(a.href, { credentials: "same-origin" }).then(function (res) {
          if (!res.ok || res.redirected) throw new Error("export failed");
          var m = /filename="?([^";]+)"?/.exec(res.headers.get("Content-Disposition") || "");
          return res.blob().then(function (blob) {
            var url = URL.createObjectURL(blob), link = doc.createElement("a");
            link.href = url;
            link.download = m ? m[1] : "scry-export";
            link.hidden = true;
            doc.body.appendChild(link);
            link.click();
            link.remove();
            setTimeout(function () { URL.revokeObjectURL(url); }, 60000);
            release(a);
          });
        }).catch(function () {
          release(a);
          tell(status, DOWNLOAD_BROKE);
          nudge(a);
        });
      });
    });
  }

  // ---- Settings: appearance applies at once ------------------------------

  // Choosing a mode changes the page immediately, and saves it as well.
  // Saves are serialized: one POST is in flight at a time, and changes made
  // while it runs coalesce into a single follow-up carrying the latest
  // selection, so rapid switching always persists the mode last chosen.
  // If the learner leaves (or hides) the page while that follow-up is still
  // waiting, it is sent at once as a keepalive request, which outlives the
  // page, so the last choice is never lost to a quick navigation.
  //
  // Each choice is numbered when it is made, with no clock involved: every
  // page load makes its own tab token and counts its choices from 1, and
  // each choice's POST freezes { theme, tab, number }. Within one tab the
  // server applies a save only when it is newer than that tab's stored one,
  // so an earlier POST held up on the way and overtaken by this page's own
  // keepalive flush cannot overwrite the later mode. Saves from other tabs,
  // other devices (whatever their clocks) and plain Saves always apply:
  // across tabs the last save to arrive wins.
  var themeFlush = null;
  function initTheme() {
    var form = $("[data-theme-form]");
    themeFlush = null;
    if (!form) return;
    var saving = false, pending = false, latest = null;
    const tabId = (crypto.randomUUID ? crypto.randomUUID() : String(Math.random()).slice(2) + "-" + String(Math.random()).slice(2));
    let seq = 0;
    // The body is built when the POST is sent, from the choice it carries.
    function body(choice) {
      var b = new URLSearchParams(new FormData(form));
      b.set("theme", choice.theme);
      b.set("theme_tab", choice.tabId);
      b.set("theme_seq", String(choice.seq));
      return b;
    }
    function saveTheme() {
      if (saving) { pending = true; return; }
      saving = true;
      pending = false;
      fetch(form.action, {
        method: "POST", body: body(latest), credentials: "same-origin",
        headers: { "X-Requested-With": "fetch" }
      }).catch(function () { /* the Save button still posts the plain way */ })
        .then(function () {
          saving = false;
          if (pending) saveTheme();
        });
    }
    themeFlush = function () {
      if (!pending) return;
      pending = false; // sent once: the in-flight save's follow-up is now moot
      try {
        fetch(form.action, {
          method: "POST", body: body(latest), keepalive: true, credentials: "same-origin",
          headers: { "X-Requested-With": "fetch" }
        }).catch(function () {});
      } catch (err) { /* nothing else to try */ }
    };
    $$("input[name=theme]", form).forEach(function (radio) {
      radio.addEventListener("change", function () {
        if (!radio.checked) return;
        if (radio.value === "system") doc.documentElement.removeAttribute("data-theme");
        else doc.documentElement.setAttribute("data-theme", radio.value);
        seq += 1;
        latest = { theme: radio.value, tabId: tabId, seq: seq };
        saveTheme();
      });
    });
  }

  window.addEventListener("pagehide", function () { if (themeFlush) themeFlush(); });
  doc.addEventListener("visibilitychange", function () {
    if (doc.visibilityState === "hidden" && themeFlush) themeFlush();
  });

  // ---- Bridge cards (Slice 75) --------------------------------------------

  // "Build bridge cards" waits on the model: the button says "building" in
  // mono and is disabled once the form is sent, so a second press does
  // nothing (like "explaining" on Ask Why?; no spinner, nothing loops).
  // In the preview, "Remove" takes a drafted bridge out in place; without
  // script it posts and the preview comes back without that row. The last
  // row cannot be removed: cancel instead.
  function initBridges() {
    $$("form[data-bridge-build]").forEach(function (form) {
      form.addEventListener("submit", function (e) {
        var button = $("[data-build-bridges]", form), label = $("[data-build-label]", form);
        if (form.dataset.sent === "1") { e.preventDefault(); return; }
        form.dataset.sent = "1";
        if (button) {
          button.setAttribute("aria-busy", "true");
          setTimeout(function () { if (form.dataset.sent === "1") button.disabled = true; }, 0);
        }
        if (label) {
          if (!label.dataset.idle) label.dataset.idle = label.textContent;
          label.textContent = "building";
        }
      });
    });
    var list = $("[data-bridge-list]");
    if (!list) return;
    function renumber() {
      var cards = $$("[data-bridge-card]", list);
      cards.forEach(function (card, i) {
        var n = String(i + 1);
        var num = $("[data-bridge-num]", card);
        if (num) num.textContent = "Bridge " + n;
        var name = $("[data-bridge-remove-name]", card);
        if (name) name.textContent = " bridge " + n;
        var drop = $("[data-bridge-remove]", card);
        if (drop) { drop.value = String(i); drop.hidden = cards.length < 2; }
        var q = $("[data-bridge-q]", card), a = $("[data-bridge-a]", card);
        var ql = $("[data-bridge-q-label]", card), al = $("[data-bridge-a-label]", card);
        if (q && ql) { q.id = "bridge-q-" + i; ql.htmlFor = q.id; }
        if (a && al) { a.id = "bridge-a-" + i; al.htmlFor = a.id; }
      });
    }
    $$("[data-bridge-remove]", list).forEach(function (button) {
      button.addEventListener("click", function (e) {
        e.preventDefault();
        var cards = $$("[data-bridge-card]", list);
        if (cards.length < 2) return;
        var card = button.closest("[data-bridge-card]");
        var index = cards.indexOf(card);
        card.parentNode.removeChild(card);
        renumber();
        var left = $$("[data-bridge-card]", list);
        var focus = left[Math.min(index, left.length - 1)];
        var target = focus && ($("[data-bridge-remove]:not([hidden])", focus) || $("[data-bridge-q]", focus));
        if (target) target.focus();
      });
    });
    renumber();
  }

  // A page restored from the back/forward cache comes back as it was left:
  // give a sent bridge form back, like the busy forms above.
  function bridgeBuildReset(form) {
    var button = $("[data-build-bridges]", form), label = $("[data-build-label]", form);
    form.removeAttribute("data-sent");
    if (button) { button.disabled = false; button.removeAttribute("aria-busy"); }
    if (label && label.dataset.idle) label.textContent = label.dataset.idle;
  }
  window.addEventListener("pageshow", function (e) {
    if (e.persisted) $$("form[data-bridge-build][data-sent]").forEach(bridgeBuildReset);
  });

  function init() {
    initTZ();
    initExamples();
    initImport();
    initDownloads();
    initTheme();
    initShelf();
    initBusy();
    initStudy();
    initRows();
    initDraft();
    initBridges();
  }

  init();
  frameReview();
})();

// ---- Landing: the Study Specimen ---------------------------------------
//
// DESIGN.md (Landing, Motion): the one ambient demonstration. The server
// renders the Judged stage and inert step controls, so without this script
// the Specimen is complete and still. With it the steps work, and the
// stages move on (Prompt, Answer, Judged, again) every 4 s with a 200 ms
// cross-fade drawn by app.css, holding while the Specimen is hovered or
// holds focus, and stopping for good once a step is chosen. Under reduced
// motion it never moves on by itself: it rests on Judged.
(function () {
  "use strict";
  var fig = document.querySelector("[data-specimen]");
  if (!fig) return;
  var steps = Array.prototype.slice.call(fig.querySelectorAll("[data-specimen-step]"));
  var stages = Array.prototype.slice.call(fig.querySelectorAll("[data-specimen-stage]"));
  var order = steps.map(function (b) { return b.getAttribute("data-specimen-step"); });
  if (!steps.length || steps.length !== stages.length) return;
  var still = window.matchMedia ? window.matchMedia("(prefers-reduced-motion: reduce)") : { matches: false };
  var every = 4000, timer = 0, chosen = false, hovered = false, focused = false, current = order.length - 1;
  stages.forEach(function (s) {
    if (s.classList.contains("is-shown")) current = Math.max(0, order.indexOf(s.getAttribute("data-specimen-stage")));
  });

  function show(i) {
    current = i;
    stages.forEach(function (s) { s.classList.toggle("is-shown", s.getAttribute("data-specimen-stage") === order[i]); });
    steps.forEach(function (b, j) { b.setAttribute("aria-pressed", j === i ? "true" : "false"); });
  }
  function hold() {
    if (timer) clearTimeout(timer);
    timer = 0;
  }
  function run() {
    hold();
    if (chosen || hovered || focused || still.matches || document.hidden) return;
    timer = setTimeout(function () {
      timer = 0;
      show((current + 1) % order.length);
      run();
    }, every);
  }

  steps.forEach(function (b, i) {
    b.disabled = false;
    b.addEventListener("click", function () { chosen = true; hold(); show(i); });
  });
  fig.addEventListener("mouseenter", function () { hovered = true; hold(); });
  fig.addEventListener("mouseleave", function () { hovered = false; run(); });
  fig.addEventListener("focusin", function () { focused = true; hold(); });
  fig.addEventListener("focusout", function (e) {
    if (e.relatedTarget && fig.contains(e.relatedTarget)) return;
    focused = false;
    run();
  });
  document.addEventListener("visibilitychange", run);
  if (still.addEventListener) still.addEventListener("change", run);
  run();
})();
