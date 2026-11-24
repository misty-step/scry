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

  // ---- Swap <main> from a fetched page -----------------------------------

  // opts.top: this swap is the answer-submit -> review transition. Focusing
  // the answer field below a long question scrolls the waiting page down (a
  // phone keyboard does), and swapping <main> in place would keep that
  // offset and clip the question off the top; the review is reframed at
  // the top instead. Every other swap (Ask Why, Edit card, ...) keeps the
  // learner's own scroll position.
  function swap(html, url, opts) {
    var next = new DOMParser().parseFromString(html, "text/html");
    var main = $("main", next);
    if (!main) { location.href = url; return; }
    $("main").innerHTML = main.innerHTML;
    doc.title = next.title;
    doc.body.className = next.body.className;
    var nav = $(".nav", next), here = $(".nav");
    if (nav && here) here.innerHTML = nav.innerHTML;
    if (url && url !== location.href) history.replaceState(null, "", url);
    init();
    if (opts && opts.top && $("[data-review]")) window.scrollTo(0, 0);
    if (frameReview()) return;
    var focus = $("main [autofocus]");
    if (focus) focus.focus();
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

  function post(form, extra) {
    var body = new URLSearchParams(new FormData(form));
    if (extra) Object.keys(extra).forEach(function (k) { body.set(k, extra[k]); });
    return fetch(form.action, {
      method: "POST", body: body, credentials: "same-origin",
      headers: { "X-Requested-With": "fetch" }
    }).then(function (res) {
      return res.text().then(function (html) { return { html: html, url: res.url }; });
    });
  }

  // ---- Global keys -------------------------------------------------------

  var go = { s: "/study", l: "/library", i: "/inbox", n: "/learn", b: "/billing" };
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

  // Signed in, the learner's stored time zone follows the browser's, so
  // the review's dates are on their own clock. Sent once per page load,
  // and only when the two differ.
  function syncTZ() {
    var body = doc.body;
    if (!body || !body.getAttribute) return;
    var stored = body.getAttribute("data-tz"), csrf = body.getAttribute("data-csrf");
    if (stored === null || !csrf || !window.fetch || !window.URLSearchParams) return;
    var tz = "";
    try { tz = Intl.DateTimeFormat().resolvedOptions().timeZone || ""; } catch (err) { return; }
    if (!tz || tz === stored) return;
    body.setAttribute("data-tz", tz);
    fetch("/settings/tz", {
      method: "POST", credentials: "same-origin", body: new URLSearchParams({ csrf: csrf, tz: tz }),
      headers: { "X-Requested-With": "fetch", "X-CSRF-Token": csrf }
    }).catch(function () {});
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

  var voice = null; // the recording in progress, if any

  function initStudy() {
    var form = $("[data-answer-form]");
    var nextForm = $("[data-next-form]");
    if (nextForm) initNext(nextForm);
    initExplain();
    initEdit();
    if (!form) return;

    var input = $("#answer", form);
    var line = $("[data-answer-line]", form);
    var caret = $("[data-caret]", form);
    var status = $("[data-status]", form);
    var via = $("[data-via]", form);
    var mic = $("[data-mic]", form);
    var busy = false;

    // Block caret: a block that sits just after the text while the cursor
    // is at the end; otherwise the native caret takes over.
    var ruler = doc.createElement("span");
    ruler.className = "display";
    ruler.style.cssText = "position:absolute;visibility:hidden;white-space:pre;left:0;top:0";
    line.appendChild(ruler);
    doc.documentElement.classList.add("has-caret");
    function placeCaret() {
      var atEnd = input.selectionStart === input.value.length && input.selectionEnd === input.value.length;
      ruler.textContent = input.value;
      var x = ruler.getBoundingClientRect().width;
      var fits = x < input.clientWidth - 8;
      line.classList.toggle("caret-off", !(atEnd && fits));
      caret.style.left = x + "px";
    }
    ["input", "keyup", "click", "focus", "select"].forEach(function (ev) { input.addEventListener(ev, placeCaret); });
    if (doc.fonts && doc.fonts.ready) doc.fonts.ready.then(placeCaret);
    placeCaret();

    function say(text) { status.textContent = text || ""; }

    input.addEventListener("input", function () {
      if (!voice) via.value = "typed";
    });

    // Space on an empty answer line starts listening; a second press stops.
    input.addEventListener("keydown", function (e) {
      if (e.key === " " && (voice || input.value === "")) {
        e.preventDefault();
        toggleVoice();
      }
    });
    mic.addEventListener("click", function () { toggleVoice(); input.focus(); });

    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (busy) return;
      var send = function () {
        busy = true;
        // Checking is instant; the tag only appears if it takes over 400ms.
        var slow = setTimeout(function () { say("checking"); }, 400);
        post(form).then(function (r) {
          clearTimeout(slow);
          swap(r.html, r.url, { top: true });
        }).catch(function () {
          clearTimeout(slow);
          busy = false;
          say("Couldn't reach Scry. Check your connection and press Enter to try again.");
        });
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
      form.classList.add("is-listening"); mic.setAttribute("aria-pressed", "true"); say("listening");
      v.timer = setTimeout(stopVoice, 20000);
    }

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
      // Exactly one provider upload, after recording stops; no client duration.
      v.done = fetch("/study/speech", {
        method: "POST", body: new Blob([buffer], { type: "audio/wav" }), credentials: "same-origin",
        headers: { "Content-Type": "audio/wav", "X-CSRF-Token": form.elements.csrf.value }
      }).then(function (res) { return res.json().then(function (j) {
        // Short, empty and silent recordings come back 200 with no text.
        if (!res.ok) { say(j.error || "Scry can't listen just now. Type your answer instead."); return; }
        if (j.text) { input.value = j.text; via.value = "spoken"; placeCaret(); say("heard · enter to check"); }
        else say("Didn't catch that. Try again or type it.");
      }); }).catch(function () { say("Couldn't reach Scry. Type your answer instead."); })
        .finally(function () {
          voice = null; form.classList.remove("is-listening"); mic.setAttribute("aria-pressed", "false");
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

  // "Edit card" in the post-answer review: the question and correct answer
  // are saved without leaving the review. What is on screen is updated in
  // place and "Saved" is shown; the grade and schedule are never touched.
  function initEdit() {
    var form = $("[data-edit-form]");
    if (!form || !window.fetch) return;
    var status = $("[data-edit-status]", form), error = $("[data-edit-error]", form), save = $("[data-edit-save]", form);
    var busy = false;
    form.addEventListener("input", function () { status.hidden = true; });
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (busy) return;
      busy = true;
      save.setAttribute("aria-busy", "true");
      error.hidden = true;
      status.hidden = true;
      var done = function () { busy = false; save.removeAttribute("aria-busy"); };
      var fail = function (text) { done(); error.textContent = text; error.hidden = false; };
      fetch(form.action, {
        method: "POST", body: new URLSearchParams(new FormData(form)), credentials: "same-origin",
        headers: { "Accept": "application/json" }
      }).then(function (res) { return res.json().then(function (j) {
        if (!res.ok || !j.saved) { fail(j.error || "Couldn't save that. Try again."); return; }
        done();
        // The text above the form may grow or shrink (a missed answer is set
        // very large): keep the form where it was in the window.
        var before = save.getBoundingClientRect().top;
        $$("[data-canonical]").forEach(function (el) { el.textContent = j.answer; });
        $$("[data-prompt]").forEach(function (el) { el.textContent = j.prompt; });
        form.elements.prompt.value = j.prompt;
        form.elements.answer.value = j.answer;
        status.hidden = false;
        var moved = save.getBoundingClientRect().top - before;
        if (moved) window.scrollBy(0, moved);
      }); }).catch(function () { fail("Couldn't reach Scry. Check your connection and try again."); });
    });
  }

  function initNext(form) {
    var again = $("[data-again]", form);
    var busy = false;
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (busy) return;
      busy = true;
      var extra = e.submitter && e.submitter.name === "again" ? { again: "1" } : null;
      post(form, extra).then(function (r) { swap(r.html, r.url); }).catch(function () { form.submit(); });
    });
  }

  // ---- Inbox and drafts --------------------------------------------------

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
    var wait = new Promise(function (resolve) { setTimeout(resolve, 160); });
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

  // ---- Create: example prompts -------------------------------------------

  // Tapping an example fills the field (it is a plain link without script).
  function initExamples() {
    var input = $("#q.ask-input");
    if (!input) return;
    $$("[data-example]").forEach(function (a) {
      a.addEventListener("click", function (e) {
        e.preventDefault();
        input.value = a.getAttribute("data-example");
        input.focus();
        input.setSelectionRange(input.value.length, input.value.length);
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

  function init() {
    initTZ();
    initExamples();
    initTheme();
    initShelf();
    initBusy();
    initStudy();
    initRows();
    initDraft();
  }

  init();
  frameReview();
  syncTZ();
})();
